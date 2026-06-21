package simulator

import (
	"fmt"
	"sync"
	"time"

	"github.com/clusterops/backend/internal/models"
)

// clusterState holds all mutable node and job state.
// All access is protected by mu.
type clusterState struct {
	mu    sync.RWMutex
	nodes map[string]*models.Node
	jobs  map[string]*models.Job

	// gpuPhase tracks the position of each GPU's sine-wave utilization curve.
	// key = "nodeID:gpuIndex"
	gpuPhase map[string]float64
}

// newClusterState initialises the cluster with n nodes each having gpusPerNode GPUs.
func newClusterState(cfg Config) *clusterState {
	cs := &clusterState{
		nodes:    make(map[string]*models.Node, cfg.NodeCount),
		jobs:     make(map[string]*models.Job),
		gpuPhase: make(map[string]float64),
	}

	for i := 0; i < cfg.NodeCount; i++ {
		id := fmt.Sprintf("node-%02d", i+1)
		n := &models.Node{
			ID:               id,
			Hostname:         fmt.Sprintf("gpu-worker-%02d.cluster.local", i+1),
			Status:           models.NodeStatusHealthy,
			GPUCount:         cfg.GPUsPerNode,
			GPUModel:         cfg.GPUModelName,
			CPUCores:         96,
			MemoryGB:         768,
			AllocatedGPUs:    0,
			GPUUtilization:   make([]float64, cfg.GPUsPerNode),
			GPUMemoryUsedGB:  make([]float64, cfg.GPUsPerNode),
			GPUMemoryTotalGB: make([]float64, cfg.GPUsPerNode),
			GPUTemperature:   make([]float64, cfg.GPUsPerNode),
			GPUPowerWatts:    make([]float64, cfg.GPUsPerNode),
			Labels: map[string]string{
				"zone":          fmt.Sprintf("zone-%s", zoneForNode(i)),
				"instance-type": "a3-highgpu-8g",
			},
			LastSeen:  time.Now(),
			CreatedAt: time.Now(),
		}

		// Initialise GPU memory total and idle baseline.
		for g := 0; g < cfg.GPUsPerNode; g++ {
			n.GPUMemoryTotalGB[g] = 80.0 // H100 80 GB
			n.GPUTemperature[g] = 35.0   // idle temp
			n.GPUPowerWatts[g] = 70.0    // idle power

			// Stagger each GPU's phase so they don't all spike together.
			key := fmt.Sprintf("%s:%d", id, g)
			cs.gpuPhase[key] = float64(i*cfg.GPUsPerNode+g) * 0.4
		}

		cs.nodes[id] = n
	}
	return cs
}

// nodeIDs returns a stable sorted list of node IDs.
func (cs *clusterState) nodeIDs() []string {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	ids := make([]string, 0, len(cs.nodes))
	for id := range cs.nodes {
		ids = append(ids, id)
	}
	return ids
}

// getNode returns a shallow copy of a node (safe to publish without holding the lock).
func (cs *clusterState) getNode(id string) (*models.Node, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	n, ok := cs.nodes[id]
	if !ok {
		return nil, false
	}
	cp := *n
	return &cp, true
}

// runningJobs returns all jobs currently in Running state.
func (cs *clusterState) runningJobs() []*models.Job {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	var out []*models.Job
	for _, j := range cs.jobs {
		if j.Status == models.JobStatusRunning {
			cp := *j
			out = append(out, &cp)
		}
	}
	return out
}

// allJobs returns a copy of every job.
func (cs *clusterState) allJobs() []*models.Job {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	out := make([]*models.Job, 0, len(cs.jobs))
	for _, j := range cs.jobs {
		cp := *j
		out = append(out, &cp)
	}
	return out
}

// availableNodes returns nodes that are healthy and have free GPU slots.
func (cs *clusterState) availableNodes(gpusNeeded int) []*models.Node {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	var out []*models.Node
	for _, n := range cs.nodes {
		if n.Status == models.NodeStatusHealthy &&
			n.GPUCount-n.AllocatedGPUs >= gpusNeeded {
			cp := *n
			out = append(out, &cp)
		}
	}
	return out
}

// zoneForNode maps node index to a zone label for label realism.
func zoneForNode(i int) string {
	zones := []string{"a", "b", "c"}
	return zones[i%len(zones)]
}
