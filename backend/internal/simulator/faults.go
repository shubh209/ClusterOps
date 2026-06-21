package simulator

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/clusterops/backend/internal/models"
)

// faultTick runs once per minute and randomly fires fault scenarios.
// Each fault type has an independent probability drawn from Config.
// Returns the list of alerts that should be published.
func (s *Simulator) faultTick() ([]*models.Node, []*models.Job, []*models.Alert) {
	var dirtyNodes []*models.Node
	var dirtyJobs []*models.Job
	var firedAlerts []*models.Alert

	// ── 1. Node state transitions ──────────────────────────────────────────
	for _, nodeID := range s.state.nodeIDs() {
		s.state.mu.Lock()
		node := s.state.nodes[nodeID]

		switch node.Status {
		case models.NodeStatusHealthy:
			if rand.Float64() < s.cfg.FaultNodeDegradeProb {
				node.Status = models.NodeStatusDegraded
				firedAlerts = append(firedAlerts, s.makeAlert(
					models.AlertSeverityWarning,
					models.AlertTypeNodeDegraded,
					fmt.Sprintf("Node %s degraded", node.Hostname),
					"ECC errors detected; node is still serving traffic with reduced reliability.",
					nodeID, "",
				))
				cp := *node
				dirtyNodes = append(dirtyNodes, &cp)
			}

		case models.NodeStatusDegraded:
			if rand.Float64() < s.cfg.FaultNodeDownProb {
				// Degraded → unavailable: kill all jobs on this node.
				node.Status = models.NodeStatusUnavailable
				node.AllocatedGPUs = 0
				firedAlerts = append(firedAlerts, s.makeAlert(
					models.AlertSeverityCritical,
					models.AlertTypeNodeUnavailable,
					fmt.Sprintf("Node %s is unreachable", node.Hostname),
					"Node failed health checks for 3 consecutive intervals. No traffic is being sent.",
					nodeID, "",
				))
				cp := *node
				dirtyNodes = append(dirtyNodes, &cp)

				// Fail all running jobs on this node.
				for _, j := range s.state.jobs {
					if j.Status != models.JobStatusRunning {
						continue
					}
					for _, assignedNode := range j.AssignedNodes {
						if assignedNode == nodeID {
							now := time.Now()
							reason := models.FailureReasonHardwareFault
							j.Status = models.JobStatusFailed
							j.FailureReason = &reason
							j.FailureMessage = fmt.Sprintf("Node %s became unavailable during training", node.Hostname)
							j.EndTime = &now
							j.UpdatedAt = now
							j.LogTail = append(j.LogTail, generateFailureLogs(j, reason)...)
							j.LogTail = trimLogs(j.LogTail, 50)
							cp := *j
							dirtyJobs = append(dirtyJobs, &cp)

							firedAlerts = append(firedAlerts, s.makeAlert(
								models.AlertSeverityCritical,
								models.AlertTypeJobFailed,
								fmt.Sprintf("Job %s failed: hardware fault", j.Name),
								fmt.Sprintf("Training job lost due to node failure on %s.", node.Hostname),
								nodeID, j.ID,
							))
						}
					}
				}

			} else if rand.Float64() < s.cfg.FaultNodeRecoverProb {
				node.Status = models.NodeStatusHealthy
				cp := *node
				dirtyNodes = append(dirtyNodes, &cp)
			}

		case models.NodeStatusUnavailable:
			// Unavailable nodes recover slowly.
			if rand.Float64() < 0.15 {
				node.Status = models.NodeStatusDegraded
				cp := *node
				dirtyNodes = append(dirtyNodes, &cp)
			}
		}

		s.state.mu.Unlock()
	}

	// ── 2. Thermal throttle ────────────────────────────────────────────────
	for _, nodeID := range s.state.nodeIDs() {
		if rand.Float64() < s.cfg.FaultThermalThrottle {
			s.state.mu.RLock()
			node := s.state.nodes[nodeID]
			isHealthy := node.Status == models.NodeStatusHealthy
			s.state.mu.RUnlock()

			if isHealthy {
				firedAlerts = append(firedAlerts, s.makeAlert(
					models.AlertSeverityWarning,
					models.AlertTypeGPUHighTemp,
					fmt.Sprintf("GPU thermal throttle on %s", s.nodeHostname(nodeID)),
					fmt.Sprintf("GPU temperatures exceeded 88°C on node %s. Clock speeds reduced.", nodeID),
					nodeID, "",
				))
			}
		}
	}

	// ── 3. Running job faults ──────────────────────────────────────────────
	for _, j := range s.state.runningJobs() {
		// OOM
		if rand.Float64() < s.cfg.FaultOOMProb {
			if failed := s.failJob(j.ID, models.FailureReasonOOM,
				"CUDA out of memory during forward pass"); failed != nil {
				dirtyJobs = append(dirtyJobs, failed)
				firedAlerts = append(firedAlerts, s.makeAlert(
					models.AlertSeverityCritical,
					models.AlertTypeJobFailed,
					fmt.Sprintf("Job %s failed: OOM", j.Name),
					"GPU out-of-memory error during training. Consider reducing batch size or enabling gradient checkpointing.",
					firstNode(j.AssignedNodes), j.ID,
				))
			}
			continue // job is done, don't check other faults
		}

		// Preemption (only low-priority jobs, priority <= 3)
		if j.Priority <= 3 && rand.Float64() < s.cfg.FaultPreemptProb {
			if preempted := s.preemptJob(j.ID); preempted != nil {
				dirtyJobs = append(dirtyJobs, preempted)
			}
			continue
		}

		// Hardware fault
		if rand.Float64() < s.cfg.FaultHardwareProb {
			if failed := s.failJob(j.ID, models.FailureReasonHardwareFault,
				"Uncorrectable ECC error on GPU 2"); failed != nil {
				dirtyJobs = append(dirtyJobs, failed)
				firedAlerts = append(firedAlerts, s.makeAlert(
					models.AlertSeverityCritical,
					models.AlertTypeJobFailed,
					fmt.Sprintf("Job %s failed: hardware fault", j.Name),
					"ECC error detected on assigned GPU. Node has been flagged for inspection.",
					firstNode(j.AssignedNodes), j.ID,
				))
			}
			continue
		}

		// Timeout
		if j.DurationSeconds() > s.cfg.JobDurationMax.Seconds() {
			if failed := s.failJob(j.ID, models.FailureReasonTimeout,
				fmt.Sprintf("Job exceeded maximum duration of %.0fm", s.cfg.JobDurationMax.Minutes())); failed != nil {
				dirtyJobs = append(dirtyJobs, failed)
				firedAlerts = append(firedAlerts, s.makeAlert(
					models.AlertSeverityWarning,
					models.AlertTypeJobTimeout,
					fmt.Sprintf("Job %s timed out", j.Name),
					fmt.Sprintf("Job ran for %.0f minutes, exceeding the %.0f-minute limit.",
						j.DurationSeconds()/60, s.cfg.JobDurationMax.Minutes()),
					firstNode(j.AssignedNodes), j.ID,
				))
			}
			continue
		}

		// Natural completion
		if j.DurationSeconds() >= s.cfg.JobDurationMin.Seconds() {
			// Weighted: 70% chance of completing if past minimum duration.
			if rand.Float64() < 0.35 {
				if completed := s.completeJob(j.ID); completed != nil {
					dirtyJobs = append(dirtyJobs, completed)
				}
			}
		}
	}

	// ── 4. Capacity waste alert ────────────────────────────────────────────
	totalGPUs := s.cfg.NodeCount * s.cfg.GPUsPerNode
	allocatedGPUs := 0
	idleAllocatedGPUs := 0

	s.state.mu.RLock()
	for _, node := range s.state.nodes {
		allocatedGPUs += node.AllocatedGPUs
		for _, u := range node.GPUUtilization {
			if u < 10 && node.AllocatedGPUs > 0 {
				idleAllocatedGPUs++
			}
		}
	}
	s.state.mu.RUnlock()

	wastePercent := 0.0
	if totalGPUs > 0 {
		wastePercent = float64(idleAllocatedGPUs) / float64(totalGPUs) * 100
	}
	if wastePercent > 30 {
		firedAlerts = append(firedAlerts, s.makeAlert(
			models.AlertSeverityWarning,
			models.AlertTypeCapacityWaste,
			fmt.Sprintf("%.0f%% GPU capacity wasted", wastePercent),
			fmt.Sprintf("%d GPUs are allocated but running below 10%% utilization. Check for stalled jobs.", idleAllocatedGPUs),
			"", "",
		))
	}

	return dirtyNodes, dirtyJobs, firedAlerts
}

// makeAlert constructs an Alert with a generated ID and current timestamp.
func (s *Simulator) makeAlert(sev models.AlertSeverity, typ models.AlertType,
	title, msg, nodeID, jobID string) *models.Alert {
	return &models.Alert{
		ID:          fmt.Sprintf("alert-%d", time.Now().UnixNano()),
		Severity:    sev,
		Type:        typ,
		Title:       title,
		Message:     msg,
		NodeID:      nodeID,
		JobID:       jobID,
		TriggeredAt: time.Now(),
	}
}

// nodeHostname is a safe hostname lookup (returns ID if node not found).
func (s *Simulator) nodeHostname(id string) string {
	s.state.mu.RLock()
	defer s.state.mu.RUnlock()
	if n, ok := s.state.nodes[id]; ok {
		return n.Hostname
	}
	return id
}

func firstNode(nodes []string) string {
	if len(nodes) == 0 {
		return ""
	}
	return nodes[0]
}
