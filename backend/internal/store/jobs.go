package store

import (
	"context"
	"fmt"
	"time"

	"github.com/clusterops/backend/internal/models"
)

// UpsertJob inserts or updates a job record.
func (db *DB) UpsertJob(ctx context.Context, j *models.Job) error {
	var failureReason *string
	if j.FailureReason != nil {
		s := string(*j.FailureReason)
		failureReason = &s
	}

	q := `
		INSERT INTO jobs (
			id, name, status, framework, model_name, requested_gpus,
			assigned_nodes, start_time, end_time, failure_reason,
			failure_message, log_tail, priority, user_id, created_at, updated_at
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16
		)
		ON CONFLICT (id) DO UPDATE SET
			status          = EXCLUDED.status,
			assigned_nodes  = EXCLUDED.assigned_nodes,
			start_time      = EXCLUDED.start_time,
			end_time        = EXCLUDED.end_time,
			failure_reason  = EXCLUDED.failure_reason,
			failure_message = EXCLUDED.failure_message,
			log_tail        = EXCLUDED.log_tail,
			updated_at      = EXCLUDED.updated_at`

	_, err := db.pool.Exec(ctx, q,
		j.ID, j.Name, string(j.Status), string(j.Framework), j.ModelName,
		j.RequestedGPUs, j.AssignedNodes,
		j.StartTime, j.EndTime,
		failureReason, j.FailureMessage, j.LogTail,
		j.Priority, j.UserID, j.CreatedAt, j.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("UpsertJob %s: %w", j.ID, err)
	}
	return nil
}

// GetJob fetches a single job by ID.
func (db *DB) GetJob(ctx context.Context, id string) (*models.Job, error) {
	q := `
		SELECT id, name, status, framework, model_name, requested_gpus,
		       assigned_nodes, start_time, end_time, failure_reason,
		       failure_message, log_tail, priority, user_id, created_at, updated_at
		FROM jobs WHERE id = $1`

	row := db.pool.QueryRow(ctx, q, id)
	return scanJob(row)
}

// JobFilter controls which rows are returned by ListJobs.
type JobFilter struct {
	Status    string    // empty = all
	Since     time.Time // zero = no lower bound
	Until     time.Time // zero = no upper bound
	UserID    string
	Limit     int // 0 = default 100
	Offset    int
}

// ListJobs returns jobs matching the filter, newest first.
func (db *DB) ListJobs(ctx context.Context, f JobFilter) ([]*models.Job, error) {
	if f.Limit == 0 {
		f.Limit = 100
	}

	args := []interface{}{}
	where := []string{}
	i := 1

	if f.Status != "" {
		where = append(where, fmt.Sprintf("status = $%d", i))
		args = append(args, f.Status)
		i++
	}
	if !f.Since.IsZero() {
		where = append(where, fmt.Sprintf("created_at >= $%d", i))
		args = append(args, f.Since)
		i++
	}
	if !f.Until.IsZero() {
		where = append(where, fmt.Sprintf("created_at <= $%d", i))
		args = append(args, f.Until)
		i++
	}
	if f.UserID != "" {
		where = append(where, fmt.Sprintf("user_id = $%d", i))
		args = append(args, f.UserID)
		i++
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + joinStrings(where, " AND ")
	}

	q := fmt.Sprintf(`
		SELECT id, name, status, framework, model_name, requested_gpus,
		       assigned_nodes, start_time, end_time, failure_reason,
		       failure_message, log_tail, priority, user_id, created_at, updated_at
		FROM jobs %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`,
		whereClause, i, i+1,
	)
	args = append(args, f.Limit, f.Offset)

	rows, err := db.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("ListJobs: %w", err)
	}
	defer rows.Close()

	var jobs []*models.Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// CountFailedJobsSince returns the number of failed jobs in the given window.
func (db *DB) CountFailedJobsSince(ctx context.Context, since time.Time) (int, error) {
	var count int
	err := db.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM jobs WHERE status = 'failed' AND updated_at >= $1`,
		since,
	).Scan(&count)
	return count, err
}

// scanJob reads a single job row from any pgx Scanner (QueryRow or Rows).
func scanJob(s interface {
	Scan(dest ...interface{}) error
}) (*models.Job, error) {
	j := &models.Job{}
	var status, framework, failureReason string
	var frPtr *string

	err := s.Scan(
		&j.ID, &j.Name, &status, &framework, &j.ModelName,
		&j.RequestedGPUs, &j.AssignedNodes,
		&j.StartTime, &j.EndTime,
		&frPtr, &j.FailureMessage, &j.LogTail,
		&j.Priority, &j.UserID, &j.CreatedAt, &j.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scanJob: %w", err)
	}

	j.Status = models.JobStatus(status)
	j.Framework = models.Framework(framework)

	if frPtr != nil {
		failureReason = *frPtr
		fr := models.FailureReason(failureReason)
		j.FailureReason = &fr
	}
	return j, nil
}

func joinStrings(ss []string, sep string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}
