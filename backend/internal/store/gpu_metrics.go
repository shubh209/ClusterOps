package store

import (
	"context"
	"fmt"
	"time"

	"github.com/clusterops/backend/internal/models"
)

// InsertGPUMetric appends a single telemetry snapshot.
func (db *DB) InsertGPUMetric(ctx context.Context, m *models.GPUMetric) error {
	q := `
		INSERT INTO gpu_metrics (
			node_id, gpu_index, recorded_at,
			utilization_pct, memory_used_gb, memory_total_gb,
			temperature_c, power_watts
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`

	_, err := db.pool.Exec(ctx, q,
		m.NodeID, m.GPUIndex, m.Timestamp,
		m.UtilizationPct, m.MemoryUsedGB, m.MemoryTotalGB,
		m.TemperatureC, m.PowerWatts,
	)
	if err != nil {
		return fmt.Errorf("InsertGPUMetric node=%s gpu=%d: %w", m.NodeID, m.GPUIndex, err)
	}
	return nil
}

// GetGPUTimeSeries returns metric snapshots for a node+GPU pair within a window.
// Points are returned oldest-first for charting.
func (db *DB) GetGPUTimeSeries(
	ctx context.Context,
	nodeID string,
	gpuIndex int,
	from, to time.Time,
) ([]models.GPUMetric, error) {
	q := `
		SELECT id, node_id, gpu_index, recorded_at,
		       utilization_pct, memory_used_gb, memory_total_gb,
		       temperature_c, power_watts
		FROM gpu_metrics
		WHERE node_id = $1 AND gpu_index = $2
		  AND recorded_at BETWEEN $3 AND $4
		ORDER BY recorded_at ASC`

	rows, err := db.pool.Query(ctx, q, nodeID, gpuIndex, from, to)
	if err != nil {
		return nil, fmt.Errorf("GetGPUTimeSeries: %w", err)
	}
	defer rows.Close()

	var pts []models.GPUMetric
	for rows.Next() {
		var m models.GPUMetric
		if err := rows.Scan(
			&m.ID, &m.NodeID, &m.GPUIndex, &m.Timestamp,
			&m.UtilizationPct, &m.MemoryUsedGB, &m.MemoryTotalGB,
			&m.TemperatureC, &m.PowerWatts,
		); err != nil {
			return nil, fmt.Errorf("GetGPUTimeSeries scan: %w", err)
		}
		pts = append(pts, m)
	}
	return pts, rows.Err()
}

// GetNodeGPUTimeSeries returns all GPUs for a node in one query,
// grouped into per-GPU time series. Used by the node drilldown panel.
func (db *DB) GetNodeGPUTimeSeries(
	ctx context.Context,
	nodeID string,
	from, to time.Time,
) ([]models.GPUTimeSeries, error) {
	q := `
		SELECT id, node_id, gpu_index, recorded_at,
		       utilization_pct, memory_used_gb, memory_total_gb,
		       temperature_c, power_watts
		FROM gpu_metrics
		WHERE node_id = $1 AND recorded_at BETWEEN $2 AND $3
		ORDER BY gpu_index, recorded_at ASC`

	rows, err := db.pool.Query(ctx, q, nodeID, from, to)
	if err != nil {
		return nil, fmt.Errorf("GetNodeGPUTimeSeries: %w", err)
	}
	defer rows.Close()

	byGPU := map[int]*models.GPUTimeSeries{}
	for rows.Next() {
		var m models.GPUMetric
		if err := rows.Scan(
			&m.ID, &m.NodeID, &m.GPUIndex, &m.Timestamp,
			&m.UtilizationPct, &m.MemoryUsedGB, &m.MemoryTotalGB,
			&m.TemperatureC, &m.PowerWatts,
		); err != nil {
			return nil, fmt.Errorf("GetNodeGPUTimeSeries scan: %w", err)
		}
		ts, ok := byGPU[m.GPUIndex]
		if !ok {
			ts = &models.GPUTimeSeries{NodeID: nodeID, GPUIndex: m.GPUIndex}
			byGPU[m.GPUIndex] = ts
		}
		ts.Points = append(ts.Points, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make([]models.GPUTimeSeries, 0, len(byGPU))
	for _, ts := range byGPU {
		result = append(result, *ts)
	}
	return result, nil
}

// ClusterGPUSummary computes aggregated GPU stats from the most recent snapshot
// per node/GPU pair (within last 30 seconds).
func (db *DB) ClusterGPUSummary(ctx context.Context) (*models.ClusterGPUSummary, error) {
	q := `
		WITH latest AS (
			SELECT DISTINCT ON (node_id, gpu_index)
				node_id, gpu_index, utilization_pct, memory_used_gb, memory_total_gb, temperature_c
			FROM gpu_metrics
			WHERE recorded_at > NOW() - INTERVAL '30 seconds'
			ORDER BY node_id, gpu_index, recorded_at DESC
		)
		SELECT
			COUNT(*)                          AS total_gpus,
			AVG(utilization_pct)              AS avg_util,
			AVG(memory_used_gb)               AS avg_mem_used,
			AVG(temperature_c)                AS avg_temp,
			SUM(CASE WHEN utilization_pct < 10 THEN 1 ELSE 0 END) AS wasted_gpus
		FROM latest`

	s := &models.ClusterGPUSummary{}
	var totalGPUs int64
	var wastedGPUs int64
	err := db.pool.QueryRow(ctx, q).Scan(
		&totalGPUs, &s.AvgUtilization, &s.AvgMemoryUsedGB, &s.AvgTemperatureC, &wastedGPUs,
	)
	if err != nil {
		return nil, fmt.Errorf("ClusterGPUSummary: %w", err)
	}
	s.TotalGPUs = int(totalGPUs)
	s.WastedGPUs = int(wastedGPUs)
	if s.TotalGPUs > 0 {
		s.WastePercent = float64(s.WastedGPUs) / float64(s.TotalGPUs) * 100
	}
	return s, nil
}

// PruneOldMetrics deletes gpu_metrics older than the retention window.
func (db *DB) PruneOldMetrics(ctx context.Context, retention time.Duration) (int64, error) {
	cutoff := time.Now().Add(-retention)
	tag, err := db.pool.Exec(ctx,
		`DELETE FROM gpu_metrics WHERE recorded_at < $1`, cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("PruneOldMetrics: %w", err)
	}
	return tag.RowsAffected(), nil
}
