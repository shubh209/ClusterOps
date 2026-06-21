// cmd/simulator is the self-contained cluster data generator.
// It publishes synthetic node, job, GPU, and alert events to Kafka
// with no external cluster dependency.
//
// Usage:
//
//	./simulator --brokers localhost:9092
//	./simulator --brokers kafka:9092 --nodes 10 --gpus 8
package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/clusterops/backend/internal/kafka"
	"github.com/clusterops/backend/internal/simulator"
	"go.uber.org/zap"
)

func main() {
	// ── flags ────────────────────────────────────────────────────────────
	brokers := flag.String("brokers", envOr("KAFKA_BROKERS", "localhost:9092"),
		"Comma-separated Kafka broker addresses")
	nodeCount := flag.Int("nodes", 5, "Number of simulated cluster nodes")
	gpusPerNode := flag.Int("gpus", 8, "GPUs per node")
	flag.Parse()

	// ── logger ───────────────────────────────────────────────────────────
	log, _ := zap.NewProduction()
	defer log.Sync()

	// ── kafka producer ───────────────────────────────────────────────────
	brokerList := strings.Split(*brokers, ",")
	producer := kafka.NewProducer(kafka.ProducerConfig{Brokers: brokerList}, log)
	defer producer.Close()

	// ── simulator config ─────────────────────────────────────────────────
	cfg := simulator.DefaultConfig()
	cfg.NodeCount = *nodeCount
	cfg.GPUsPerNode = *gpusPerNode

	// Speed up for quick demo: first job submits within 5 seconds.
	cfg.JobSubmitMinInterval = 5 * time.Second
	cfg.JobSubmitMaxInterval = 30 * time.Second

	// ── run ──────────────────────────────────────────────────────────────
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	sim := simulator.New(cfg, producer, log)
	sim.Run(ctx)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
