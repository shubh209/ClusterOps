// cmd/server is the ClusterOps REST API server.
// It reads from PostgreSQL and Redis; it never writes — that's the ingestion service's job.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/clusterops/backend/internal/api"
	"github.com/clusterops/backend/internal/assistant"
	"github.com/clusterops/backend/internal/cache"
	"github.com/clusterops/backend/internal/store"
	"github.com/clusterops/backend/internal/telemetry"
	"go.uber.org/zap"
)

func main() {
	log, _ := zap.NewProduction()
	defer log.Sync()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// ── OpenTelemetry ─────────────────────────────────────────────────────
	otelEndpoint := envOr("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")
	tp, err := telemetry.Init(ctx, "clusterops-api", otelEndpoint)
	if err != nil {
		log.Warn("otel init failed (traces disabled)", zap.Error(err))
	} else {
		defer tp.Shutdown(ctx)
	}

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

	// ── Rule-based assistant ──────────────────────────────────────────────
	asst := assistant.NewEngine(log)

	// ── HTTP server ───────────────────────────────────────────────────────
	cfg := api.DefaultConfig()
	cfg.Addr = envOr("API_ADDR", ":8080")

	srv := api.New(db, cacheClient, asst, log, cfg)
	if err := srv.Start(ctx); err != nil {
		log.Error("server stopped", zap.Error(err))
	}
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
