package store

import (
	"context"
	"fmt"
	"time"

	"github.com/clusterops/backend/internal/models"
)

// UpsertNode inserts or updates a node record.
// ON CONFLICT updates all mutable columns so the row reflects the latest state.
func (db *DB) UpsertNode(ctx context.Context, n *models.Node) error {
	q := `
		INSERT INTO nodes (
			id, hostname, status, gpu_count, gpu_model, cpu_cores, memory_gb,
			allocated_gpus, gpu_utilization, gpu_memory_used_gb, gpu_memory_total_gb,
			gpu_temperature_c, gpu_power_watts, labels, last_seen, created_at
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16
		)
		ON CONFLICT (id) DO UPDATE SET
			hostname            = EXCLUDED.hostname,
			status              = EXCLUDED.status,
			allocated_gpus      = EXCLUDED.allocated_gpus,
			gpu_utilization     = EXCLUDED.gpu_utilization,
			gpu_memory_used_gb  = EXCLUDED.gpu_memory_used_gb,
			gpu_memory_total_gb = EXCLUDED.gpu_memory_total_gb,
			gpu_temperature_c   = EXCLUDED.gpu_temperature_c,
			gpu_power_watts     = EXCLUDED.gpu_power_watts,
			labels              = EXCLUDED.labels,
			last_seen           = EXCLUDED.last_seen`

	_, err := db.pool.Exec(ctx, q,
		n.ID, n.Hostname, string(n.Status),
		n.GPUCount, n.GPUModel, n.CPUCores, n.MemoryGB,
		n.AllocatedGPUs,
		n.GPUUtilization, n.GPUMemoryUsedGB, n.GPUMemoryTotalGB,
		n.GPUTemperature, n.GPUPowerWatts,
		n.Labels, n.LastSeen, n.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("UpsertNode %s: %w", n.ID, err)
	}
	return nil
}

// GetNode fetches a single node by ID.
func (db *DB) GetNode(ctx context.Context, id string) (*models.Node, error) {
	q := `
		SELECT id, hostname, status, gpu_count, gpu_model, cpu_cores, memory_gb,
		       allocated_gpus, gpu_utilization, gpu_memory_used_gb, gpu_memory_total_gb,
		       gpu_temperature_c, gpu_power_watts, labels, last_seen, created_at
		FROM nodes WHERE id = $1`

	row := db.pool.QueryRow(ctx, q, id)
	n := &models.Node{}
	var labelsJSON []byte
	err := row.Scan(
		&n.ID, &n.Hostname, &n.Status,
		&n.GPUCount, &n.GPUModel, &n.CPUCores, &n.MemoryGB,
		&n.AllocatedGPUs,
		&n.GPUUtilization, &n.GPUMemoryUsedGB, &n.GPUMemoryTotalGB,
		&n.GPUTemperature, &n.GPUPowerWatts,
		&labelsJSON, &n.LastSeen, &n.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("GetNode %s: %w", id, err)
	}
	return n, nil
}

// ListNodes returns all nodes ordered by hostname.
func (db *DB) ListNodes(ctx context.Context) ([]*models.Node, error) {
	q := `
		SELECT id, hostname, status, gpu_count, gpu_model, cpu_cores, memory_gb,
		       allocated_gpus, gpu_utilization, gpu_memory_used_gb, gpu_memory_total_gb,
		       gpu_temperature_c, gpu_power_watts, labels, last_seen, created_at
		FROM nodes ORDER BY hostname`

	rows, err := db.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("ListNodes: %w", err)
	}
	defer rows.Close()

	var nodes []*models.Node
	for rows.Next() {
		n := &models.Node{}
		var labelsJSON []byte
		if err := rows.Scan(
			&n.ID, &n.Hostname, &n.Status,
			&n.GPUCount, &n.GPUModel, &n.CPUCores, &n.MemoryGB,
			&n.AllocatedGPUs,
			&n.GPUUtilization, &n.GPUMemoryUsedGB, &n.GPUMemoryTotalGB,
			&n.GPUTemperature, &n.GPUPowerWatts,
			&labelsJSON, &n.LastSeen, &n.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("ListNodes scan: %w", err)
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

// ListNodesByStatus returns nodes filtered by a specific status.
func (db *DB) ListNodesByStatus(ctx context.Context, status models.NodeStatus) ([]*models.Node, error) {
	q := `
		SELECT id, hostname, status, gpu_count, gpu_model, cpu_cores, memory_gb,
		       allocated_gpus, gpu_utilization, gpu_memory_used_gb, gpu_memory_total_gb,
		       gpu_temperature_c, gpu_power_watts, labels, last_seen, created_at
		FROM nodes WHERE status = $1 ORDER BY hostname`

	rows, err := db.pool.Query(ctx, q, string(status))
	if err != nil {
		return nil, fmt.Errorf("ListNodesByStatus: %w", err)
	}
	defer rows.Close()

	var nodes []*models.Node
	for rows.Next() {
		n := &models.Node{}
		var labelsJSON []byte
		if err := rows.Scan(
			&n.ID, &n.Hostname, &n.Status,
			&n.GPUCount, &n.GPUModel, &n.CPUCores, &n.MemoryGB,
			&n.AllocatedGPUs,
			&n.GPUUtilization, &n.GPUMemoryUsedGB, &n.GPUMemoryTotalGB,
			&n.GPUTemperature, &n.GPUPowerWatts,
			&labelsJSON, &n.LastSeen, &n.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("ListNodesByStatus scan: %w", err)
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

// MarkNodeLastSeen updates only the last_seen timestamp — cheaper than a full upsert.
func (db *DB) MarkNodeLastSeen(ctx context.Context, id string) error {
	_, err := db.pool.Exec(ctx,
		`UPDATE nodes SET last_seen = $1 WHERE id = $2`,
		time.Now(), id,
	)
	return err
}
