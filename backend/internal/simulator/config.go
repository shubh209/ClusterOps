// Package simulator generates a fully self-contained synthetic GPU cluster.
// No Kubernetes or cloud dependencies required — it runs as a single Go binary.
//
// Design:
//   - 5 nodes, each with 8 mock H100 GPUs
//   - A job scheduler submits training jobs every 30–120s
//   - A GPU telemetry engine emits realistic utilization curves every 5s
//   - A fault injector randomly triggers node degradation, OOM kills,
//     preemptions, thermal throttling, and timeouts
//   - All state changes are published to Kafka so the ingestion service
//     and API server consume them independently
package simulator

import "time"

// Config controls the simulator's behavior. All intervals are wall-clock.
type Config struct {
	// Cluster topology
	NodeCount    int // default 5
	GPUsPerNode  int // default 8
	GPUModelName string

	// Job scheduler
	JobSubmitMinInterval time.Duration // min time between new job submissions
	JobSubmitMaxInterval time.Duration // max time between new job submissions
	JobDurationMin       time.Duration // shortest a job can run before completing
	JobDurationMax       time.Duration // longest a job can run before timing out

	// GPU telemetry
	GPUMetricInterval time.Duration // how often GPU snapshots are emitted

	// Fault injection probabilities (per tick, 0.0–1.0)
	FaultNodeDegradeProb  float64 // probability a healthy node degrades each minute
	FaultNodeRecoverProb  float64 // probability a degraded node recovers each minute
	FaultNodeDownProb     float64 // probability a degraded node goes fully down each minute
	FaultOOMProb          float64 // probability a running job OOMs each minute
	FaultPreemptProb      float64 // probability a running low-priority job is preempted each minute
	FaultHardwareProb     float64 // probability of hardware fault on a running job each minute
	FaultThermalThrottle  float64 // probability of GPU thermal throttle event per node per minute

	// Users to spread jobs across (for realism)
	Users []string

	// Model names to cycle through
	ModelNames []string
}

// DefaultConfig returns sensible defaults for the demo.
func DefaultConfig() Config {
	return Config{
		NodeCount:    5,
		GPUsPerNode:  8,
		GPUModelName: "NVIDIA H100 80GB SXM5",

		JobSubmitMinInterval: 20 * time.Second,
		JobSubmitMaxInterval: 90 * time.Second,
		JobDurationMin:       2 * time.Minute,
		JobDurationMax:       8 * time.Minute,

		GPUMetricInterval: 5 * time.Second,

		FaultNodeDegradeProb: 0.05,  // 5% per minute
		FaultNodeRecoverProb: 0.30,  // 30% per minute — nodes tend to recover
		FaultNodeDownProb:    0.10,  // 10% per minute once degraded
		FaultOOMProb:         0.08,  // 8% per running job per minute
		FaultPreemptProb:     0.06,  // 6% for low-priority jobs
		FaultHardwareProb:    0.03,  // 3% per running job per minute
		FaultThermalThrottle: 0.04,  // 4% thermal throttle per node per minute

		Users: []string{
			"alice", "bob", "charlie", "diana", "evan",
		},
		ModelNames: []string{
			"llama-3-70b", "mistral-7b", "gpt-neox-20b",
			"falcon-40b", "qwen-72b", "deepseek-67b",
		},
	}
}
