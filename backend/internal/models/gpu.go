package models

import "time"

// GPUMetric is a point-in-time snapshot of a single GPU's telemetry.
// Stored in PostgreSQL for history; current values live in Node.
type GPUMetric struct {
	ID            int64     `json:"id"`
	NodeID        string    `json:"node_id"`
	GPUIndex      int       `json:"gpu_index"`
	Timestamp     time.Time `json:"timestamp"`
	UtilizationPct float64  `json:"utilization_pct"`
	MemoryUsedGB  float64   `json:"memory_used_gb"`
	MemoryTotalGB float64   `json:"memory_total_gb"`
	TemperatureC  float64   `json:"temperature_c"`
	PowerWatts    float64   `json:"power_watts"`
}

// GPUTimeSeries groups metric snapshots for a single GPU for charting.
type GPUTimeSeries struct {
	NodeID    string      `json:"node_id"`
	GPUIndex  int         `json:"gpu_index"`
	Points    []GPUMetric `json:"points"`
}

// ClusterGPUSummary is the rolled-up GPU state across the whole cluster.
type ClusterGPUSummary struct {
	TotalGPUs       int     `json:"total_gpus"`
	AllocatedGPUs   int     `json:"allocated_gpus"`
	IdleGPUs        int     `json:"idle_gpus"`
	WastedGPUs      int     `json:"wasted_gpus"` // allocated but <10% util
	AvgUtilization  float64 `json:"avg_utilization_pct"`
	AvgMemoryUsedGB float64 `json:"avg_memory_used_gb"`
	AvgTemperatureC float64 `json:"avg_temperature_c"`
	WastePercent    float64 `json:"waste_percent"`
}
