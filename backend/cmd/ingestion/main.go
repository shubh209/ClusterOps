// cmd/ingestion is the Kafka→PostgreSQL+Redis bridge.
// It consumes all cluster event topics and persists them durably.
package main

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/clusterops/backend/internal/cache"
	"github.com/clusterops/backend/internal/ingestion"
	"github.com/clusterops/backend/internal/kafka"
	"github.com/clusterops/backend/internal/store"
	"go.uber.org/zap"
)

func main() {
	log, _ := zap.NewProduction()
	defer log.Sync()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// ── PostgreSQL ────────────────────────────────────────────────────────
	db, err := store.New(ctx, store.Config{
		Host:     envOr("POSTGRES_HOST", "localhost"),
		Port:     envInt("POSTGRES_PORT", 5432),
		User:     envOr("POSTGRES_USER", "clusterops"),
		Password: envOr("POSTGRES_PASSWORD", "clusterops"),
		DBName:   envOr("POSTGRES_DB", "clusterops"),
		SSLMode:  envOr("POSTGRES_SSLMODE", "disable"),
	}, log)
	if err != nil {
		log.Fatal("postgres connect", zap.Error(err))
	}
	defer db.Close()

	// ── Redis ─────────────────────────────────────────────────────────────
	cacheClient, err := cache.New(cache.Config{
		Addr:     envOr("REDIS_ADDR", "localhost:6379"),
		Password: envOr("REDIS_PASSWORD", ""),
		DB:       0,
	}, log)
	if err != nil {
		log.Fatal("redis connect", zap.Error(err))
	}
	defer cacheClient.Close()

	// ── Kafka consumer ────────────────────────────────────────────────────
	brokers := strings.Split(envOr("KAFKA_BROKERS", "localhost:9092"), ",")
	consumerCfg := kafka.ConsumerConfig{
		Brokers: brokers,
		GroupID: "clusterops-ingestion",
	}

	// ── run ───────────────────────────────────────────────────────────────
	svc := ingestion.New(db, cacheClient, log)
	svc.Run(ctx, consumerCfg)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n := 0
	for _, c := range v {
		if c < '0' || c > '9' {
			return fallback
		}
		n = n*10 + int(c-'0')
	}
	return n
}
