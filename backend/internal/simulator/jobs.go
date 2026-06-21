package simulator

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/clusterops/backend/internal/models"
)

// jobCounter is a monotonic counter for generating unique job IDs.
var jobCounter int

// newJobID returns the next job ID in the sequence.
func newJobID() string {
	jobCounter++
	return fmt.Sprintf("job-%05d", jobCounter)
}

// submitJob creates a new job, assigns it to available nodes, and transitions
// it to Running. Returns nil if no capacity is available (job stays queued).
func (s *Simulator) submitJob() *models.Job {
	gpusNeeded := pickGPUCount()
	now := time.Now()

	j := &models.Job{
		ID:            newJobID(),
		Name:          fmt.Sprintf("%s-train-%d", pickFrom(s.cfg.ModelNames), rand.Intn(9999)),
		Status:        models.JobStatusQueued,
		Framework:     pickFramework(),
		ModelName:     pickFrom(s.cfg.ModelNames),
		RequestedGPUs: gpusNeeded,
		Priority:      1 + rand.Intn(10),
		UserID:        pickFrom(s.cfg.Users),
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	// Try to place the job immediately.
	available := s.state.availableNodes(gpusNeeded)
	if len(available) == 0 {
		// No capacity — leave queued, will be retried on next scheduler tick.
		s.state.mu.Lock()
		s.state.jobs[j.ID] = j
		s.state.mu.Unlock()
		return j
	}

	// Assign to the first available node (simple first-fit scheduler).
	assigned := available[0]

	s.state.mu.Lock()
	node := s.state.nodes[assigned.ID]
	node.AllocatedGPUs += gpusNeeded
	j.Status = models.JobStatusRunning
	j.AssignedNodes = []string{assigned.ID}
	j.StartTime = &now
	j.LogTail = generateStartLogs(j)
	s.state.jobs[j.ID] = j
	s.state.mu.Unlock()

	return j
}

// completeJob transitions a running job to Completed and frees its GPUs.
func (s *Simulator) completeJob(jobID string) *models.Job {
	s.state.mu.Lock()
	defer s.state.mu.Unlock()

	j, ok := s.state.jobs[jobID]
	if !ok || j.Status != models.JobStatusRunning {
		return nil
	}

	now := time.Now()
	j.Status = models.JobStatusCompleted
	j.EndTime = &now
	j.UpdatedAt = now
	j.LogTail = append(j.LogTail, generateCompletionLogs(j)...)
	j.LogTail = trimLogs(j.LogTail, 50)

	s.freeGPUs(j)
	return copyJob(j)
}

// failJob transitions a running job to Failed with the given reason.
func (s *Simulator) failJob(jobID string, reason models.FailureReason, msg string) *models.Job {
	s.state.mu.Lock()
	defer s.state.mu.Unlock()

	j, ok := s.state.jobs[jobID]
	if !ok || j.Status != models.JobStatusRunning {
		return nil
	}

	now := time.Now()
	j.Status = models.JobStatusFailed
	j.FailureReason = &reason
	j.FailureMessage = msg
	j.EndTime = &now
	j.UpdatedAt = now
	j.LogTail = append(j.LogTail, generateFailureLogs(j, reason)...)
	j.LogTail = trimLogs(j.LogTail, 50)

	s.freeGPUs(j)
	return copyJob(j)
}

// preemptJob transitions a running job to Preempted (freed by a higher-priority job).
func (s *Simulator) preemptJob(jobID string) *models.Job {
	s.state.mu.Lock()
	defer s.state.mu.Unlock()

	j, ok := s.state.jobs[jobID]
	if !ok || j.Status != models.JobStatusRunning {
		return nil
	}

	now := time.Now()
	j.Status = models.JobStatusPreempted
	reason := models.FailureReasonPreemption
	j.FailureReason = &reason
	j.FailureMessage = "Job preempted by higher-priority workload"
	j.EndTime = &now
	j.UpdatedAt = now
	j.LogTail = append(j.LogTail, "[WARN]  Received SIGTERM from scheduler: preemption",
		"[INFO]  Saving checkpoint to /mnt/checkpoints/...",
		"[INFO]  Checkpoint saved. Exiting.")
	j.LogTail = trimLogs(j.LogTail, 50)

	s.freeGPUs(j)
	return copyJob(j)
}

// freeGPUs releases allocated GPU slots on all nodes assigned to a job.
// Must be called with state.mu held.
func (s *Simulator) freeGPUs(j *models.Job) {
	gpusPerNode := j.RequestedGPUs / max(len(j.AssignedNodes), 1)
	for _, nodeID := range j.AssignedNodes {
		if node, ok := s.state.nodes[nodeID]; ok {
			node.AllocatedGPUs -= gpusPerNode
			if node.AllocatedGPUs < 0 {
				node.AllocatedGPUs = 0
			}
		}
	}
}

// retryQueuedJobs tries to start any jobs that are still in Queued state.
func (s *Simulator) retryQueuedJobs() []*models.Job {
	s.state.mu.RLock()
	var queued []*models.Job
	for _, j := range s.state.jobs {
		if j.Status == models.JobStatusQueued {
			cp := *j
			queued = append(queued, &cp)
		}
	}
	s.state.mu.RUnlock()

	var started []*models.Job
	for _, j := range queued {
		available := s.state.availableNodes(j.RequestedGPUs)
		if len(available) == 0 {
			continue
		}
		now := time.Now()
		assigned := available[0]

		s.state.mu.Lock()
		live := s.state.jobs[j.ID]
		if live != nil && live.Status == models.JobStatusQueued {
			node := s.state.nodes[assigned.ID]
			node.AllocatedGPUs += j.RequestedGPUs
			live.Status = models.JobStatusRunning
			live.AssignedNodes = []string{assigned.ID}
			live.StartTime = &now
			live.UpdatedAt = now
			live.LogTail = generateStartLogs(live)
			cp := *live
			started = append(started, &cp)
		}
		s.state.mu.Unlock()
	}
	return started
}

// ─── log generators ──────────────────────────────────────────────────────────

func generateStartLogs(j *models.Job) []string {
	return []string{
		fmt.Sprintf("[INFO]  Job %s initializing on node(s): %v", j.ID, j.AssignedNodes),
		fmt.Sprintf("[INFO]  Framework: %s | Model: %s | GPUs: %d", j.Framework, j.ModelName, j.RequestedGPUs),
		"[INFO]  Loading dataset shards from /mnt/data/...",
		"[INFO]  Building model graph...",
		"[INFO]  Compiling CUDA kernels (may take 60–120s on first run)...",
		"[INFO]  Starting training loop. Step 0/50000",
	}
}

func generateCompletionLogs(j *models.Job) []string {
	return []string{
		fmt.Sprintf("[INFO]  Training complete. Final loss: %.4f", 0.05+rand.Float64()*0.15),
		"[INFO]  Saving final checkpoint...",
		fmt.Sprintf("[INFO]  Job %s finished successfully. Duration: %.0fs", j.ID, j.DurationSeconds()),
	}
}

func generateFailureLogs(j *models.Job, reason models.FailureReason) []string {
	switch reason {
	case models.FailureReasonOOM:
		return []string{
			fmt.Sprintf("[WARN]  GPU memory usage at %.1f GB / 80.0 GB", 76+rand.Float64()*3),
			"[ERROR] CUDA out of memory. Tried to allocate 2.50 GiB",
			"[ERROR] RuntimeError: CUDA out of memory.",
			"[ERROR] Process killed by OOM handler.",
		}
	case models.FailureReasonHardwareFault:
		return []string{
			"[WARN]  ECC error detected on GPU 3",
			"[ERROR] NCCL error: unhandled cuda error, CUDA driver version is insufficient",
			"[ERROR] Hardware fault detected. Node marked for maintenance.",
		}
	case models.FailureReasonTimeout:
		return []string{
			fmt.Sprintf("[WARN]  Job has been running for %.0f minutes — exceeds timeout threshold", j.DurationSeconds()/60),
			"[ERROR] Job exceeded maximum allowed duration. Terminating.",
		}
	case models.FailureReasonUserError:
		return []string{
			"[ERROR] ValueError: expected input shape (batch, seq, dim) but got (seq, batch, dim)",
			"[ERROR] Traceback (most recent call last): ...",
			"[ERROR] Job terminated due to unhandled exception.",
		}
	default:
		return []string{"[ERROR] Job terminated unexpectedly."}
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func pickGPUCount() int {
	// Weighted: most jobs use 1 or 2 nodes worth of GPUs
	choices := []int{1, 2, 4, 8}
	weights := []int{20, 35, 30, 15}
	return weightedPick(choices, weights)
}

func pickFramework() models.Framework {
	choices := []models.Framework{
		models.FrameworkPyTorch,
		models.FrameworkJAX,
		models.FrameworkTensorFlow,
	}
	weights := []int{60, 25, 15}
	return choices[weightedPickIndex(weights)]
}

func pickFrom(ss []string) string {
	return ss[rand.Intn(len(ss))]
}

func weightedPick(values []int, weights []int) int {
	return values[weightedPickIndex(weights)]
}

func weightedPickIndex(weights []int) int {
	total := 0
	for _, w := range weights {
		total += w
	}
	r := rand.Intn(total)
	cumulative := 0
	for i, w := range weights {
		cumulative += w
		if r < cumulative {
			return i
		}
	}
	return len(weights) - 1
}

func trimLogs(logs []string, max int) []string {
	if len(logs) <= max {
		return logs
	}
	return logs[len(logs)-max:]
}

func copyJob(j *models.Job) *models.Job {
	cp := *j
	return &cp
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
