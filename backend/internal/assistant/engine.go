// Package assistant implements a rule-based failure analysis engine.
//
// Design philosophy (relevant to the AI meta-learning goal):
//   This is a deterministic "LLM-free" baseline — it encodes expert knowledge
//   as explicit rules and lookup tables rather than probabilistic inference.
//   This is the same pattern used in production systems as a harness/fallback
//   for when the LLM is unavailable, and as a regression test oracle for when
//   you add a real LLM (you can diff the LLM output against the rule output).
//
//   The structure mirrors a RAG pipeline:
//     1. Retrieve:  match failure reason / node state → select relevant playbook
//     2. Augment:   enrich with live metrics (GPU util, temp, job duration)
//     3. Generate:  produce structured Analysis (summary + steps + severity)
//
//   When you add Ollama in Phase 5, the Engine interface stays the same —
//   you just swap the Generate step for an LLM call, keeping Retrieve/Augment.
package assistant

import (
	"time"

	"github.com/clusterops/backend/internal/models"
	"go.uber.org/zap"
)

// Analysis is the structured output returned by the assistant for any target.
type Analysis struct {
	// TargetType is "job" or "node".
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
	TargetName string `json:"target_name"`

	// Headline is a one-sentence plain-English summary of the situation.
	Headline string `json:"headline"`

	// Severity mirrors alert severity for UI badge colouring.
	Severity string `json:"severity"` // "critical" | "warning" | "info"

	// RootCause is the engine's best-effort classification.
	RootCause string `json:"root_cause"`

	// Summary is 2–4 sentences explaining what happened and why.
	Summary string `json:"summary"`

	// DebuggingSteps is an ordered list of concrete next actions for the operator.
	DebuggingSteps []Step `json:"debugging_steps"`

	// PreventionTips are longer-term mitigations.
	PreventionTips []string `json:"prevention_tips"`

	// RelatedAlerts surfaces linked alert IDs for the UI to link to.
	RelatedAlerts []string `json:"related_alert_ids"`

	// Confidence is a rough 0–1 score of how well the rules matched.
	Confidence float64 `json:"confidence"`

	GeneratedAt time.Time `json:"generated_at"`
}

// Step is one debugging action with an optional command the operator can run.
type Step struct {
	Order       int    `json:"order"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Command     string `json:"command,omitempty"` // shell command, if applicable
}

// Engine is the rule-based analysis engine.
type Engine struct {
	logger *zap.Logger
}

// NewEngine creates a ready Engine.
func NewEngine(logger *zap.Logger) *Engine {
	return &Engine{logger: logger}
}

// AnalyzeJob produces a full Analysis for a completed or running job.
func (e *Engine) AnalyzeJob(job *models.Job, alerts []*models.Alert) *Analysis {
	a := &Analysis{
		TargetType:  "job",
		TargetID:    job.ID,
		TargetName:  job.Name,
		GeneratedAt: time.Now(),
	}

	// Collect related alert IDs.
	for _, al := range alerts {
		a.RelatedAlerts = append(a.RelatedAlerts, al.ID)
	}

	// Route to the appropriate rule set based on job status / failure reason.
	switch job.Status {
	case models.JobStatusRunning:
		e.analyzeRunningJob(a, job)
	case models.JobStatusFailed:
		if job.FailureReason != nil {
			e.analyzeFailedJob(a, job, *job.FailureReason)
		} else {
			e.analyzeUnknownFailure(a, job)
		}
	case models.JobStatusPreempted:
		e.analyzePreemptedJob(a, job)
	case models.JobStatusCompleted:
		e.analyzeCompletedJob(a, job)
	default:
		e.analyzeQueuedJob(a, job)
	}

	return a
}

// AnalyzeNode produces an Analysis for a degraded or unavailable node.
func (e *Engine) AnalyzeNode(node *models.Node) *Analysis {
	a := &Analysis{
		TargetType:  "node",
		TargetID:    node.ID,
		TargetName:  node.Hostname,
		GeneratedAt: time.Now(),
	}

	switch node.Status {
	case models.NodeStatusDegraded:
		e.analyzeDegradedNode(a, node)
	case models.NodeStatusUnavailable:
		e.analyzeUnavailableNode(a, node)
	default:
		e.analyzeHealthyNode(a, node)
	}

	return a
}
