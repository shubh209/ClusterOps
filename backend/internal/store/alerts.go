package store

import (
	"context"
	"fmt"
	"time"

	"github.com/clusterops/backend/internal/models"
)

// InsertAlert persists a newly fired alert.
func (db *DB) InsertAlert(ctx context.Context, a *models.Alert) error {
	q := `
		INSERT INTO alerts (id, severity, type, title, message, node_id, job_id, triggered_at, resolved)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (id) DO NOTHING`

	nodeID := nullableString(a.NodeID)
	jobID := nullableString(a.JobID)

	_, err := db.pool.Exec(ctx, q,
		a.ID, string(a.Severity), string(a.Type),
		a.Title, a.Message,
		nodeID, jobID,
		a.TriggeredAt, a.Resolved,
	)
	if err != nil {
		return fmt.Errorf("InsertAlert %s: %w", a.ID, err)
	}
	return nil
}

// ResolveAlert marks an alert as resolved.
func (db *DB) ResolveAlert(ctx context.Context, id string) error {
	now := time.Now()
	_, err := db.pool.Exec(ctx,
		`UPDATE alerts SET resolved = TRUE, resolved_at = $1 WHERE id = $2`,
		now, id,
	)
	return err
}

// ListAlerts returns alerts ordered by triggered_at descending.
// If activeOnly is true, only unresolved alerts are returned.
func (db *DB) ListAlerts(ctx context.Context, activeOnly bool, limit int) ([]*models.Alert, error) {
	if limit == 0 {
		limit = 200
	}

	q := `
		SELECT id, severity, type, title, message,
		       COALESCE(node_id,''), COALESCE(job_id,''),
		       triggered_at, resolved_at, resolved
		FROM alerts`
	if activeOnly {
		q += ` WHERE resolved = FALSE`
	}
	q += ` ORDER BY triggered_at DESC LIMIT $1`

	rows, err := db.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("ListAlerts: %w", err)
	}
	defer rows.Close()

	var alerts []*models.Alert
	for rows.Next() {
		a := &models.Alert{}
		var sev, typ string
		if err := rows.Scan(
			&a.ID, &sev, &typ, &a.Title, &a.Message,
			&a.NodeID, &a.JobID,
			&a.TriggeredAt, &a.ResolvedAt, &a.Resolved,
		); err != nil {
			return nil, fmt.Errorf("ListAlerts scan: %w", err)
		}
		a.Severity = models.AlertSeverity(sev)
		a.Type = models.AlertType(typ)
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}

// CountActiveAlerts returns the number of unresolved alerts.
func (db *DB) CountActiveAlerts(ctx context.Context) (int, error) {
	var count int
	err := db.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM alerts WHERE resolved = FALSE`,
	).Scan(&count)
	return count, err
}

// GetAlertsByJob returns all alerts linked to a specific job.
func (db *DB) GetAlertsByJob(ctx context.Context, jobID string) ([]*models.Alert, error) {
	q := `
		SELECT id, severity, type, title, message,
		       COALESCE(node_id,''), COALESCE(job_id,''),
		       triggered_at, resolved_at, resolved
		FROM alerts WHERE job_id = $1
		ORDER BY triggered_at DESC`

	rows, err := db.pool.Query(ctx, q, jobID)
	if err != nil {
		return nil, fmt.Errorf("GetAlertsByJob: %w", err)
	}
	defer rows.Close()

	var alerts []*models.Alert
	for rows.Next() {
		a := &models.Alert{}
		var sev, typ string
		if err := rows.Scan(
			&a.ID, &sev, &typ, &a.Title, &a.Message,
			&a.NodeID, &a.JobID,
			&a.TriggeredAt, &a.ResolvedAt, &a.Resolved,
		); err != nil {
			return nil, fmt.Errorf("GetAlertsByJob scan: %w", err)
		}
		a.Severity = models.AlertSeverity(sev)
		a.Type = models.AlertType(typ)
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}

// nullableString converts an empty string to nil (for nullable TEXT columns).
func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
