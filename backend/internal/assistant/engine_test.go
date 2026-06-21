// engine_test.go is the assistant harness.
//
// AI meta-learning note:
//   This is an *eval harness* — a table-driven test that checks whether the
//   rule engine produces the expected root cause, severity, and minimum number
//   of debugging steps for each failure scenario.
//
//   When you add a real LLM (Ollama) in Phase 5, you run the same harness
//   against the LLM output and compare. Any case where the LLM produces a
//   *worse* result than the rules (wrong severity, missing steps) is a
//   regression. The harness is your ground truth.
//
//   This pattern — deterministic rules as baseline, LLM as enhancement,
//   harness to measure delta — is how production AI systems are evaluated.
package assistant_test

import (
	"testing"
	"time"

	"github.com/clusterops/backend/internal/assistant"
	"github.com/clusterops/backend/internal/models"
	"go.uber.org/zap"
)

func makeEngine() *assistant.Engine {
	return assistant.NewEngine(zap.NewNop())
}

func ptr[T any](v T) *T { return &v }

// ─── job analysis harness ─────────────────────────────────────────────────────

func TestAnalyzeJob(t *testing.T) {
	now := time.Now()

	cases := []struct {
		name              string
		job               *models.Job
		wantRootCause     string
		wantSeverity      string
		minDebuggingSteps int
		wantConfidence    float64
	}{
		{
			name: "OOM failure",
			job: &models.Job{
				ID: "job-00001", Name: "llama-3-70b-train-1234",
				Status: models.JobStatusFailed,
				FailureReason: ptr(models.FailureReasonOOM),
				Framework: models.FrameworkPyTorch, ModelName: "llama-3-70b",
				RequestedGPUs: 8, AssignedNodes: []string{"node-01"},
				StartTime: ptr(now.Add(-10 * time.Minute)), EndTime: ptr(now),
			},
			wantRootCause:     "GPU Out-of-Memory (OOM)",
			wantSeverity:      "critical",
			minDebuggingSteps: 4,
			wantConfidence:    0.90,
		},
		{
			name: "Hardware fault",
			job: &models.Job{
				ID: "job-00002", Name: "mistral-7b-train-5678",
				Status:        models.JobStatusFailed,
				FailureReason: ptr(models.FailureReasonHardwareFault),
				Framework:     models.FrameworkPyTorch, ModelName: "mistral-7b",
				RequestedGPUs: 4, AssignedNodes: []string{"node-02"},
				StartTime: ptr(now.Add(-20 * time.Minute)), EndTime: ptr(now),
			},
			wantRootCause:     "Hardware Fault (ECC / NVLink Error)",
			wantSeverity:      "critical",
			minDebuggingSteps: 4,
			wantConfidence:    0.90,
		},
		{
			name: "Timeout",
			job: &models.Job{
				ID: "job-00003", Name: "gpt-neox-20b-train-9012",
				Status:        models.JobStatusFailed,
				FailureReason: ptr(models.FailureReasonTimeout),
				Framework:     models.FrameworkPyTorch, ModelName: "gpt-neox-20b",
				RequestedGPUs: 8, AssignedNodes: []string{"node-03"},
				StartTime: ptr(now.Add(-90 * time.Minute)), EndTime: ptr(now),
			},
			wantRootCause:     "Job Timeout",
			wantSeverity:      "warning",
			minDebuggingSteps: 4,
			wantConfidence:    0.80,
		},
		{
			name: "Preemption",
			job: &models.Job{
				ID: "job-00004", Name: "falcon-40b-train-3456",
				Status:        models.JobStatusPreempted,
				FailureReason: ptr(models.FailureReasonPreemption),
				Priority:      2,
				Framework:     models.FrameworkPyTorch, ModelName: "falcon-40b",
				RequestedGPUs: 4, AssignedNodes: []string{"node-01"},
				StartTime: ptr(now.Add(-15 * time.Minute)), EndTime: ptr(now),
			},
			wantRootCause:     "Scheduler Preemption",
			wantSeverity:      "warning",
			minDebuggingSteps: 2,
			wantConfidence:    0.90,
		},
		{
			name: "User error",
			job: &models.Job{
				ID: "job-00005", Name: "qwen-72b-train-7890",
				Status:        models.JobStatusFailed,
				FailureReason: ptr(models.FailureReasonUserError),
				Framework:     models.FrameworkPyTorch, ModelName: "qwen-72b",
				RequestedGPUs: 8, AssignedNodes: []string{"node-04"},
				StartTime: ptr(now.Add(-2 * time.Minute)), EndTime: ptr(now),
			},
			wantRootCause:     "User / Configuration Error",
			wantSeverity:      "warning",
			minDebuggingSteps: 3,
			wantConfidence:    0.80,
		},
		{
			name: "Running job",
			job: &models.Job{
				ID: "job-00006", Name: "deepseek-67b-train-2468",
				Status:        models.JobStatusRunning,
				Framework:     models.FrameworkPyTorch, ModelName: "deepseek-67b",
				RequestedGPUs: 8, AssignedNodes: []string{"node-05"},
				StartTime: ptr(now.Add(-30 * time.Minute)),
			},
			wantRootCause:     "Currently Running",
			wantSeverity:      "info",
			minDebuggingSteps: 1,
			wantConfidence:    0.95,
		},
		{
			name: "Completed job",
			job: &models.Job{
				ID: "job-00007", Name: "llama-3-70b-train-1357",
				Status:        models.JobStatusCompleted,
				Framework:     models.FrameworkPyTorch, ModelName: "llama-3-70b",
				RequestedGPUs: 8, AssignedNodes: []string{"node-01"},
				StartTime: ptr(now.Add(-45 * time.Minute)), EndTime: ptr(now),
			},
			wantRootCause:     "Completed Successfully",
			wantSeverity:      "info",
			minDebuggingSteps: 0,
			wantConfidence:    0.99,
		},
	}

	engine := makeEngine()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := engine.AnalyzeJob(tc.job, nil)

			if got.RootCause != tc.wantRootCause {
				t.Errorf("RootCause: got %q, want %q", got.RootCause, tc.wantRootCause)
			}
			if got.Severity != tc.wantSeverity {
				t.Errorf("Severity: got %q, want %q", got.Severity, tc.wantSeverity)
			}
			if len(got.DebuggingSteps) < tc.minDebuggingSteps {
				t.Errorf("DebuggingSteps: got %d, want >= %d", len(got.DebuggingSteps), tc.minDebuggingSteps)
			}
			if got.Confidence < tc.wantConfidence {
				t.Errorf("Confidence: got %.2f, want >= %.2f", got.Confidence, tc.wantConfidence)
			}
			if got.Headline == "" {
				t.Error("Headline should not be empty")
			}
			if got.Summary == "" {
				t.Error("Summary should not be empty")
			}
		})
	}
}

// ─── node analysis harness ────────────────────────────────────────────────────

func TestAnalyzeNode(t *testing.T) {
	cases := []struct {
		name          string
		node          *models.Node
		wantSeverity  string
		wantRootCause string
	}{
		{
			name: "Degraded node high temp",
			node: &models.Node{
				ID: "node-01", Hostname: "gpu-worker-01.cluster.local",
				Status: models.NodeStatusDegraded, GPUCount: 8,
				GPUTemperature:  []float64{87, 89, 86, 91, 85, 88, 90, 87},
				GPUMemoryUsedGB: []float64{40, 42, 38, 45, 39, 41, 44, 43},
			},
			wantSeverity:  "warning",
			wantRootCause: "Thermal Throttle (GPU overheating)",
		},
		{
			name: "Unavailable node",
			node: &models.Node{
				ID: "node-02", Hostname: "gpu-worker-02.cluster.local",
				Status: models.NodeStatusUnavailable, GPUCount: 8,
				GPUModel: "NVIDIA H100 80GB SXM5",
				LastSeen: time.Now().Add(-5 * time.Minute),
			},
			wantSeverity:  "critical",
			wantRootCause: "Node Unreachable",
		},
		{
			name: "Healthy node",
			node: &models.Node{
				ID: "node-03", Hostname: "gpu-worker-03.cluster.local",
				Status: models.NodeStatusHealthy, GPUCount: 8, AllocatedGPUs: 6,
				GPUUtilization: []float64{85, 87, 0, 0, 88, 86, 84, 0},
			},
			wantSeverity:  "info",
			wantRootCause: "Node Healthy",
		},
	}

	engine := makeEngine()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := engine.AnalyzeNode(tc.node)

			if got.Severity != tc.wantSeverity {
				t.Errorf("Severity: got %q, want %q", got.Severity, tc.wantSeverity)
			}
			if got.RootCause != tc.wantRootCause {
				t.Errorf("RootCause: got %q, want %q", got.RootCause, tc.wantRootCause)
			}
			if got.Headline == "" {
				t.Error("Headline should not be empty")
			}
		})
	}
}
