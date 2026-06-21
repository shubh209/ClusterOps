package models

import "time"

// NodeStatus represents the operational state of a cluster node.
type NodeStatus string

const (
	NodeStatusHealthy      NodeStatus = "healthy"
	NodeStatusDegraded     NodeStatus = "degraded"
	NodeStatusUnavailable  NodeStatus = "unavailable"
	NodeStatusMaintenance  NodeStatus = "maintenance"
)

// Node represents a physical or virtual machine in the GPU cluster.
type Node struct {
	ID              string            `json:"id"`
	Hostname        string            `json:"hostname"`
	Status          NodeStatus        `json:"status"`
	GPUCount        int               `json:"gpu_count"`
	GPUModel        string            `json:"gpu_model"`
	CPUCores        int               `json:"cpu_cores"`
	MemoryGB        float64           `json:"memory_gb"`
	AllocatedGPUs   int               `json:"allocated_gpus"`
	GPUUtilization  []float64         `json:"gpu_utilization"`  // per-GPU, 0–100
	GPUMemoryUsedGB []float64         `json:"gpu_memory_used_gb"`
	GPUMemoryTotalGB []float64        `json:"gpu_memory_total_gb"`
	GPUTemperature  []float64         `json:"gpu_temperature_c"`
	GPUPowerWatts   []float64         `json:"gpu_power_watts"`
	Labels          map[string]string `json:"labels"`
	LastSeen        time.Time         `json:"last_seen"`
	CreatedAt       time.Time         `json:"created_at"`
}

// GPUWastePercent returns the percentage of GPUs that are allocated but underutilized (<10%).
func (n *Node) GPUWastePercent() float64 {
	if n.AllocatedGPUs == 0 {
		return 0
	}
	wasted := 0
	for _, u := range n.GPUUtilization {
		if u < 10.0 {
			wasted++
		}
	}
	return float64(wasted) / float64(n.GPUCount) * 100
}

// AvgGPUUtilization returns the mean GPU utilization across all GPUs on this node.
func (n *Node) AvgGPUUtilization() float64 {
	if len(n.GPUUtilization) == 0 {
		return 0
	}
	sum := 0.0
	for _, u := range n.GPUUtilization {
		sum += u
	}
	return sum / float64(len(n.GPUUtilization))
}
