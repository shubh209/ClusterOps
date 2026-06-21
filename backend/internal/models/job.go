package models

import "time"

// JobStatus represents the lifecycle state of a training job.
type JobStatus string

const (
	JobStatusQueued    JobStatus = "queued"
	JobStatusRunning   JobStatus = "running"
	JobStatusFailed    JobStatus = "failed"
	JobStatusCompleted JobStatus = "completed"
	JobStatusPreempted JobStatus = "preempted"
)

// FailureReason categorizes why a job failed — drives the rule-based assistant.
type FailureReason string

const (
	FailureReasonOOM          FailureReason = "oom"
	FailureReasonHardwareFault FailureReason = "hardware_fault"
	FailureReasonPreemption   FailureReason = "preemption"
	FailureReasonTimeout      FailureReason = "timeout"
	FailureReasonUserError    FailureReason = "user_error"
)

// Framework identifies the ML training framework used.
type Framework string

const (
	FrameworkPyTorch     Framework = "pytorch"
	FrameworkJAX         Framework = "jax"
	FrameworkTensorFlow  Framework = "tensorflow"
)

// Job represents a single AI training job running on the cluster.
type Job struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	Status          JobStatus      `json:"status"`
	Framework       Framework      `json:"framework"`
	ModelName       string         `json:"model_name"`
	RequestedGPUs   int            `json:"requested_gpus"`
	AssignedNodes   []string       `json:"assigned_nodes"`
	StartTime       *time.Time     `json:"start_time,omitempty"`
	EndTime         *time.Time     `json:"end_time,omitempty"`
	FailureReason   *FailureReason `json:"failure_reason,omitempty"`
	FailureMessage  string         `json:"failure_message,omitempty"`
	LogTail         []string       `json:"log_tail,omitempty"`
	Priority        int            `json:"priority"` // 1 (low) – 10 (high)
	UserID          string         `json:"user_id"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// DurationSeconds returns wall-clock seconds the job has run (or ran).
// Returns 0 if the job hasn't started yet.
func (j *Job) DurationSeconds() float64 {
	if j.StartTime == nil {
		return 0
	}
	end := time.Now()
	if j.EndTime != nil {
		end = *j.EndTime
	}
	return end.Sub(*j.StartTime).Seconds()
}

// IsTerminal returns true if the job is in a final state.
func (j *Job) IsTerminal() bool {
	return j.Status == JobStatusFailed ||
		j.Status == JobStatusCompleted ||
		j.Status == JobStatusPreempted
}
