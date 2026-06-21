package models

import "time"

// AlertSeverity classifies alert urgency.
type AlertSeverity string

const (
	AlertSeverityCritical AlertSeverity = "critical"
	AlertSeverityWarning  AlertSeverity = "warning"
	AlertSeverityInfo     AlertSeverity = "info"
)

// AlertType identifies the category of alert for rule-based routing.
type AlertType string

const (
	AlertTypeNodeUnavailable  AlertType = "node_unavailable"
	AlertTypeNodeDegraded     AlertType = "node_degraded"
	AlertTypeGPUHighTemp      AlertType = "gpu_high_temperature"
	AlertTypeGPUMemoryFull    AlertType = "gpu_memory_full"
	AlertTypeJobFailed        AlertType = "job_failed"
	AlertTypeCapacityWaste    AlertType = "capacity_waste"
	AlertTypeJobTimeout       AlertType = "job_timeout"
	AlertTypeClusterDegraded  AlertType = "cluster_degraded"
)

// Alert represents a fired threshold or event-driven notification.
type Alert struct {
	ID          string        `json:"id"`
	Severity    AlertSeverity `json:"severity"`
	Type        AlertType     `json:"type"`
	Title       string        `json:"title"`
	Message     string        `json:"message"`
	NodeID      string        `json:"node_id,omitempty"`
	JobID       string        `json:"job_id,omitempty"`
	TriggeredAt time.Time     `json:"triggered_at"`
	ResolvedAt  *time.Time    `json:"resolved_at,omitempty"`
	Resolved    bool          `json:"resolved"`
}

// ClusterSummary is the top-level health rollup served to the dashboard.
type ClusterSummary struct {
	HealthScore      float64           `json:"health_score"`      // 0–100
	TotalNodes       int               `json:"total_nodes"`
	HealthyNodes     int               `json:"healthy_nodes"`
	DegradedNodes    int               `json:"degraded_nodes"`
	UnavailableNodes int               `json:"unavailable_nodes"`
	ActiveJobs       int               `json:"active_jobs"`
	QueuedJobs       int               `json:"queued_jobs"`
	FailedJobsLast1h int               `json:"failed_jobs_last_1h"`
	GPU              ClusterGPUSummary `json:"gpu"`
	ActiveAlerts     int               `json:"active_alerts"`
	UpdatedAt        time.Time         `json:"updated_at"`
}
