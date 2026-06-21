package store

import (
	"context"
	"fmt"
	"time"

	"github.com/clusterops/backend/internal/models"
)

// ClusterSummary builds the top-level health rollup from live DB state.
// This is called by the API and its result is cached in Redis.
func (db *DB) ClusterSummary(ctx context.Context) (*models.ClusterSummary, error) {
	// Node counts by status.
	type nodeCounts struct {
		total, healthy, degraded, unavailable int
	}
	nc := nodeCounts{}
	rows, err := db.pool.Query(ctx,
		`SELECT status, COUNT(*) FROM nodes GROUP BY status`,
	)
	if err != nil {
		return nil, fmt.Errorf("ClusterSummary node counts: %w", err)
	}
	for rows.Next() {
		var status string
		var cnt int
		if err := rows.Scan(&status, &cnt); err != nil {
			rows.Close()
			return nil, err
		}
		nc.total += cnt
		switch models.NodeStatus(status) {
		case models.NodeStatusHealthy:
			nc.healthy += cnt
		case models.NodeStatusDegraded:
			nc.degraded += cnt
		case models.NodeStatusUnavailable:
			nc.unavailable += cnt
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Job counts.
	type jobCounts struct{ active, queued int }
	jc := jobCounts{}
	jobRows, err := db.pool.Query(ctx,
		`SELECT status, COUNT(*) FROM jobs WHERE status IN ('running','queued') GROUP BY status`,
	)
	if err != nil {
		return nil, fmt.Errorf("ClusterSummary job counts: %w", err)
	}
	for jobRows.Next() {
		var status string
		var cnt int
		if err := jobRows.Scan(&status, &cnt); err != nil {
			jobRows.Close()
			return nil, err
		}
		switch models.JobStatus(status) {
		case models.JobStatusRunning:
			jc.active += cnt
		case models.JobStatusQueued:
			jc.queued += cnt
		}
	}
	jobRows.Close()

	// Failed jobs in last hour.
	failedLast1h, err := db.CountFailedJobsSince(ctx, time.Now().Add(-time.Hour))
	if err != nil {
		return nil, err
	}

	// GPU summary.
	gpuSummary, err := db.ClusterGPUSummary(ctx)
	if err != nil {
		// Non-fatal: GPU metrics may not exist yet at startup.
		gpuSummary = &models.ClusterGPUSummary{}
	}

	// Active alerts.
	activeAlerts, err := db.CountActiveAlerts(ctx)
	if err != nil {
		return nil, err
	}

	// Health score: 100 minus penalties.
	score := 100.0
	if nc.total > 0 {
		score -= float64(nc.unavailable) / float64(nc.total) * 40 // up to -40 for unavailable nodes
		score -= float64(nc.degraded) / float64(nc.total) * 20    // up to -20 for degraded nodes
	}
	score -= gpuSummary.WastePercent * 0.2 // up to -20 for GPU waste
	if failedLast1h > 5 {
		score -= 10
	} else if failedLast1h > 0 {
		score -= float64(failedLast1h) * 2
	}
	if score < 0 {
		score = 0
	}

	return &models.ClusterSummary{
		HealthScore:      score,
		TotalNodes:       nc.total,
		HealthyNodes:     nc.healthy,
		DegradedNodes:    nc.degraded,
		UnavailableNodes: nc.unavailable,
		ActiveJobs:       jc.active,
		QueuedJobs:       jc.queued,
		FailedJobsLast1h: failedLast1h,
		GPU:              *gpuSummary,
		ActiveAlerts:     activeAlerts,
		UpdatedAt:        time.Now(),
	}, nil
}
