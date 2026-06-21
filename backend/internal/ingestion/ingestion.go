// Package ingestion consumes Kafka events from the simulator and writes them
// to PostgreSQL (durable store) and Redis (hot cache).
// It is the only writer to the database — the API server is read-only.
package ingestion

import (
	"context"
	"time"

	"github.com/clusterops/backend/internal/cache"
	"github.com/clusterops/backend/internal/kafka"
	"github.com/clusterops/backend/internal/models"
	"github.com/clusterops/backend/internal/store"
	"go.uber.org/zap"
)

// Service wires the Kafka consumer to the store and cache.
type Service struct {
	db     *store.DB
	cache  *cache.Client
	logger *zap.Logger
}

// New returns a configured ingestion Service.
func New(db *store.DB, cache *cache.Client, logger *zap.Logger) *Service {
	return &Service{db: db, cache: cache, logger: logger}
}

// Run starts the Kafka consumer and blocks until ctx is cancelled.
// It also starts a background pruning goroutine.
func (svc *Service) Run(ctx context.Context, consumerCfg kafka.ConsumerConfig) {
	consumer := kafka.NewConsumer(consumerCfg, kafka.Handlers{
		OnNodeEvent:  svc.handleNodeEvent,
		OnJobEvent:   svc.handleJobEvent,
		OnGPUMetric:  svc.handleGPUMetric,
		OnAlertEvent: svc.handleAlertEvent,
	}, svc.logger)
	defer consumer.Close()

	consumer.Start(ctx)
	svc.logger.Info("ingestion service running")

	// Prune old GPU metrics every 30 minutes.
	pruneTicker := time.NewTicker(30 * time.Minute)
	defer pruneTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			svc.logger.Info("ingestion service stopped")
			return
		case <-pruneTicker.C:
			svc.pruneMetrics(ctx)
		}
	}
}

// ─── handlers ────────────────────────────────────────────────────────────────

func (svc *Service) handleNodeEvent(ctx context.Context, evt models.NodeEvent) error {
	n := &evt.Node

	// Write to PostgreSQL.
	if err := svc.db.UpsertNode(ctx, n); err != nil {
		svc.logger.Error("upsert node", zap.String("id", n.ID), zap.Error(err))
		return err
	}

	// Invalidate and refresh Redis.
	if err := svc.cache.InvalidateNode(ctx, n.ID); err != nil {
		svc.logger.Warn("cache invalidate node", zap.String("id", n.ID), zap.Error(err))
	}
	if err := svc.cache.SetNode(ctx, n); err != nil {
		svc.logger.Warn("cache set node", zap.String("id", n.ID), zap.Error(err))
	}

	return nil
}

func (svc *Service) handleJobEvent(ctx context.Context, evt models.JobEvent) error {
	j := &evt.Job

	// Write to PostgreSQL.
	if err := svc.db.UpsertJob(ctx, j); err != nil {
		svc.logger.Error("upsert job", zap.String("id", j.ID), zap.Error(err))
		return err
	}

	// Invalidate job cache (both individual and list caches).
	if err := svc.cache.InvalidateJob(ctx, j.ID); err != nil {
		svc.logger.Warn("cache invalidate job", zap.String("id", j.ID), zap.Error(err))
	}
	// Warm the individual job cache immediately.
	if err := svc.cache.SetJob(ctx, j); err != nil {
		svc.logger.Warn("cache set job", zap.String("id", j.ID), zap.Error(err))
	}

	svc.logger.Debug("job event processed",
		zap.String("id", j.ID),
		zap.String("status", string(j.Status)),
	)
	return nil
}

func (svc *Service) handleGPUMetric(ctx context.Context, evt models.GPUMetricEvent) error {
	m := &evt.Metric

	// Write to PostgreSQL time-series.
	if err := svc.db.InsertGPUMetric(ctx, m); err != nil {
		svc.logger.Error("insert gpu metric", zap.String("node", m.NodeID), zap.Error(err))
		return err
	}

	// Refresh cluster GPU summary in Redis (best-effort, non-blocking).
	go func() {
		refreshCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if summary, err := svc.db.ClusterGPUSummary(refreshCtx); err == nil {
			_ = svc.cache.SetGPUSummary(refreshCtx, summary)
		}
	}()

	return nil
}

func (svc *Service) handleAlertEvent(ctx context.Context, evt models.AlertEvent) error {
	a := &evt.Alert

	switch evt.Type {
	case models.EventTypeAlertFired:
		if err := svc.db.InsertAlert(ctx, a); err != nil {
			svc.logger.Error("insert alert", zap.String("id", a.ID), zap.Error(err))
			return err
		}
	case models.EventTypeAlertResolved:
		if err := svc.db.ResolveAlert(ctx, a.ID); err != nil {
			svc.logger.Error("resolve alert", zap.String("id", a.ID), zap.Error(err))
			return err
		}
	}

	// Invalidate alert cache so the next API read reflects the change.
	if err := svc.cache.InvalidateAlerts(ctx); err != nil {
		svc.logger.Warn("cache invalidate alerts", zap.Error(err))
	}

	return nil
}

// pruneMetrics removes GPU metrics older than 24 hours.
func (svc *Service) pruneMetrics(ctx context.Context) {
	n, err := svc.db.PruneOldMetrics(ctx, 24*time.Hour)
	if err != nil {
		svc.logger.Error("prune gpu metrics", zap.Error(err))
		return
	}
	if n > 0 {
		svc.logger.Info("pruned old gpu metrics", zap.Int64("rows_deleted", n))
	}
}
