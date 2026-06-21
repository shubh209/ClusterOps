package simulator

import (
	"context"
	"math/rand"
	"time"

	"github.com/clusterops/backend/internal/kafka"
	"go.uber.org/zap"
)

// Simulator orchestrates all synthetic cluster activity and publishes events
// to Kafka. It is the only writer of cluster state — the ingestion service
// and API server are read-only consumers.
type Simulator struct {
	cfg      Config
	state    *clusterState
	producer *kafka.Producer
	logger   *zap.Logger
}

// New creates a Simulator with the given config, initialises cluster state,
// and connects the Kafka producer.
func New(cfg Config, producer *kafka.Producer, logger *zap.Logger) *Simulator {
	return &Simulator{
		cfg:      cfg,
		state:    newClusterState(cfg),
		producer: producer,
		logger:   logger,
	}
}

// Run starts all simulator loops and blocks until ctx is cancelled.
//
// Three concurrent loops:
//  1. gpuLoop    — emits GPU metrics every GPUMetricInterval (default 5s)
//  2. jobLoop    — submits new jobs on a random interval (20–90s)
//  3. faultLoop  — fires fault scenarios every 60s
func (s *Simulator) Run(ctx context.Context) {
	s.logger.Info("simulator starting",
		zap.Int("nodes", s.cfg.NodeCount),
		zap.Int("gpus_per_node", s.cfg.GPUsPerNode),
	)

	// Publish initial node state so the ingestion service has a baseline.
	s.publishAllNodes(ctx)

	gpuTicker := time.NewTicker(s.cfg.GPUMetricInterval)
	faultTicker := time.NewTicker(60 * time.Second)
	jobTicker := time.NewTimer(s.nextJobInterval())

	defer gpuTicker.Stop()
	defer faultTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("simulator stopped")
			return

		case <-gpuTicker.C:
			s.runGPUTick(ctx)

		case <-faultTicker.C:
			s.runFaultTick(ctx)
			// Also retry any queued jobs on each fault tick.
			s.runQueueRetry(ctx)

		case <-jobTicker.C:
			s.runJobSubmit(ctx)
			jobTicker.Reset(s.nextJobInterval())
		}
	}
}

// ─── tick handlers ────────────────────────────────────────────────────────────

func (s *Simulator) runGPUTick(ctx context.Context) {
	metrics := s.emitGPUMetrics()
	for i := range metrics {
		if err := s.producer.PublishGPUMetric(ctx, &metrics[i]); err != nil {
			s.logger.Warn("publish gpu metric", zap.Error(err))
		}
	}
	// Also republish node snapshots with updated GPU arrays.
	s.publishAllNodes(ctx)
}

func (s *Simulator) runFaultTick(ctx context.Context) {
	dirtyNodes, dirtyJobs, alerts := s.faultTick()

	for _, n := range dirtyNodes {
		if err := s.producer.PublishNodeEvent(ctx, n); err != nil {
			s.logger.Warn("publish node event", zap.Error(err))
		}
	}
	for _, j := range dirtyJobs {
		if err := s.producer.PublishJobEvent(ctx, j); err != nil {
			s.logger.Warn("publish job event", zap.Error(err))
		}
	}
	for _, a := range alerts {
		if err := s.producer.PublishAlertFired(ctx, a); err != nil {
			s.logger.Warn("publish alert", zap.Error(err))
		}
	}

	if len(dirtyJobs) > 0 || len(dirtyNodes) > 0 {
		s.logger.Info("fault tick",
			zap.Int("dirty_nodes", len(dirtyNodes)),
			zap.Int("dirty_jobs", len(dirtyJobs)),
			zap.Int("alerts", len(alerts)),
		)
	}
}

func (s *Simulator) runJobSubmit(ctx context.Context) {
	j := s.submitJob()
	if j == nil {
		return
	}
	if err := s.producer.PublishJobEvent(ctx, j); err != nil {
		s.logger.Warn("publish job event", zap.Error(err))
	}
	s.logger.Info("job submitted",
		zap.String("id", j.ID),
		zap.String("model", j.ModelName),
		zap.String("status", string(j.Status)),
		zap.Int("gpus", j.RequestedGPUs),
	)
}

func (s *Simulator) runQueueRetry(ctx context.Context) {
	started := s.retryQueuedJobs()
	for _, j := range started {
		if err := s.producer.PublishJobEvent(ctx, j); err != nil {
			s.logger.Warn("publish queued job start", zap.Error(err))
		}
	}
}

// publishAllNodes sends a node update for every node in the cluster.
func (s *Simulator) publishAllNodes(ctx context.Context) {
	for _, id := range s.state.nodeIDs() {
		n, ok := s.state.getNode(id)
		if !ok {
			continue
		}
		if err := s.producer.PublishNodeEvent(ctx, n); err != nil {
			s.logger.Warn("publish node event", zap.String("node", id), zap.Error(err))
		}
	}
}

// nextJobInterval returns a random duration in [JobSubmitMinInterval, JobSubmitMaxInterval].
func (s *Simulator) nextJobInterval() time.Duration {
	min := s.cfg.JobSubmitMinInterval
	max := s.cfg.JobSubmitMaxInterval
	delta := max - min
	return min + time.Duration(rand.Int63n(int64(delta)))
}
