package assistant

import (
	"fmt"

	"github.com/clusterops/backend/internal/models"
)

// ─── failed job rules ─────────────────────────────────────────────────────────

func (e *Engine) analyzeFailedJob(a *Analysis, job *models.Job, reason models.FailureReason) {
	switch reason {
	case models.FailureReasonOOM:
		e.ruleOOM(a, job)
	case models.FailureReasonHardwareFault:
		e.ruleHardwareFault(a, job)
	case models.FailureReasonTimeout:
		e.ruleTimeout(a, job)
	case models.FailureReasonPreemption:
		e.analyzePreemptedJob(a, job)
	case models.FailureReasonUserError:
		e.ruleUserError(a, job)
	default:
		e.analyzeUnknownFailure(a, job)
	}
}

func (e *Engine) ruleOOM(a *Analysis, job *models.Job) {
	a.Severity = "critical"
	a.RootCause = "GPU Out-of-Memory (OOM)"
	a.Headline = fmt.Sprintf(
		"Job %s crashed with a GPU OOM error during %s training on %s.",
		job.Name, job.Framework, job.ModelName,
	)
	a.Summary = fmt.Sprintf(
		"The training process requested more GPU memory than was available on the assigned node(s). "+
			"This typically happens when the batch size is too large for the model's activation memory footprint, "+
			"or when gradient accumulation buffers exceed available HBM. "+
			"The job consumed %.0f seconds of compute before failing.",
		job.DurationSeconds(),
	)
	a.DebuggingSteps = []Step{
		{
			Order: 1,
			Title: "Confirm OOM in logs",
			Description: "Look for 'CUDA out of memory' or 'RuntimeError: CUDA error' in the job log tail. " +
				"Note the allocation size reported.",
			Command: fmt.Sprintf(`kubectl logs -l job-name=%s --tail=50 | grep -i "out of memory"`, job.ID),
		},
		{
			Order:       2,
			Title:       "Check peak memory usage",
			Description: "Use nvidia-smi to see the maximum memory consumption on the node at time of failure.",
			Command:     fmt.Sprintf(`nvidia-smi --query-gpu=memory.used,memory.free --format=csv -l 1`),
		},
		{
			Order: 3,
			Title: "Reduce batch size",
			Description: fmt.Sprintf(
				"Start with halving the per-GPU batch size. For %s, try --batch-size 8 if currently at 16.",
				job.ModelName,
			),
		},
		{
			Order:       4,
			Title:       "Enable gradient checkpointing",
			Description: "Trades compute for memory. For PyTorch: model.gradient_checkpointing_enable(). For JAX: use jax.checkpoint.",
		},
		{
			Order:       5,
			Title:       "Enable mixed-precision training",
			Description: "bf16 training halves activation memory. Set --bf16 true (Accelerate) or torch.autocast('cuda', dtype=torch.bfloat16).",
		},
		{
			Order:       6,
			Title:       "Try ZeRO-3 / FSDP",
			Description: "Distribute optimizer state, gradients, and parameters across GPUs with DeepSpeed ZeRO Stage 3 or PyTorch FSDP.",
		},
	}
	a.PreventionTips = []string{
		"Set --max-memory-mb in your training config and run a dry-run with a single batch before full training.",
		"Use memory profilers (torch.profiler, jax.profiler) to identify peak allocation sites.",
		"Configure GPU memory fraction limits to leave headroom: torch.cuda.set_per_process_memory_fraction(0.9).",
	}
	a.Confidence = 0.95
}

func (e *Engine) ruleHardwareFault(a *Analysis, job *models.Job) {
	a.Severity = "critical"
	a.RootCause = "Hardware Fault (ECC / NVLink Error)"
	a.Headline = fmt.Sprintf(
		"Job %s terminated due to a hardware fault on the assigned GPU node.",
		job.Name,
	)
	a.Summary = fmt.Sprintf(
		"An uncorrectable ECC (Error-Correcting Code) memory error was detected on one or more GPUs assigned to this job. "+
			"This is a hardware-level failure — the GPU's HBM memory reported a bit error that could not be corrected. "+
			"The job ran for %.0f seconds before the error surfaced. The node has been flagged for inspection.",
		job.DurationSeconds(),
	)
	a.DebuggingSteps = []Step{
		{
			Order:       1,
			Title:       "Check ECC error counts",
			Description: "Query volatile and aggregate ECC error counters on the affected GPU.",
			Command:     `nvidia-smi --query-gpu=ecc.errors.uncorrected.volatile.total,ecc.errors.corrected.volatile.total --format=csv`,
		},
		{
			Order:       2,
			Title:       "Review dmesg for hardware errors",
			Description: "Look for NVRM, Xid errors in the kernel log. Xid 48 = Double Bit ECC Error, Xid 79 = GPU fatal fault.",
			Command:     `dmesg | grep -i "NVRM\|Xid\|ECC" | tail -30`,
		},
		{
			Order:       3,
			Title:       "Run GPU diagnostics",
			Description: "Execute nvidia-smi diagnostic test to confirm hardware health.",
			Command:     `nvidia-smi -q -d ECC`,
		},
		{
			Order:       4,
			Title:       "Drain and cordon the node",
			Description: "Prevent new workloads from being scheduled on this node until hardware is inspected.",
			Command:     fmt.Sprintf(`kubectl cordon %s`, firstAssignedNode(job)),
		},
		{
			Order:       5,
			Title:       "Reset ECC counters and retest",
			Description: "After inspection, clear counters and run a memory test (CUDA memtest) before returning to service.",
			Command:     `nvidia-smi --ecc-config=1 && nvidia-smi -r`,
		},
		{
			Order:       6,
			Title:       "Resubmit job on healthy node",
			Description: "Once a healthy node is confirmed, resubmit the job with --exclude-node=<failed-node> or a node affinity rule.",
		},
	}
	a.PreventionTips = []string{
		"Enable GPU health checks in your cluster's node problem detector (NPD) to catch ECC errors before they kill jobs.",
		"Schedule weekly nvidia-smi ECC scrubs during maintenance windows.",
		"Configure DCGM (Data Center GPU Manager) health monitoring for automated node draining on hardware faults.",
	}
	a.Confidence = 0.92
}

func (e *Engine) ruleTimeout(a *Analysis, job *models.Job) {
	durationMin := job.DurationSeconds() / 60
	a.Severity = "warning"
	a.RootCause = "Job Timeout"
	a.Headline = fmt.Sprintf(
		"Job %s exceeded its maximum allowed runtime of %.0f minutes.",
		job.Name, durationMin,
	)
	a.Summary = fmt.Sprintf(
		"The training job ran for %.0f minutes without completing, triggering the scheduler's timeout policy. "+
			"Common causes: the model is larger than expected, the dataset has more samples than estimated, "+
			"a training loop hung waiting for a network barrier, or a GPU is throttling due to thermal issues.",
		durationMin,
	)
	a.DebuggingSteps = []Step{
		{
			Order:       1,
			Title:       "Check training throughput",
			Description: "Look at steps/sec or samples/sec in the training logs. Compare to expected throughput for this model size.",
		},
		{
			Order:       2,
			Title:       "Check for NCCL hangs",
			Description: "Distributed jobs often hang at collective operations if one rank is slower. Look for NCCL_TIMEOUT errors.",
			Command:     fmt.Sprintf(`kubectl logs -l job-name=%s --tail=100 | grep -i "NCCL\|hang\|timeout\|barrier"`, job.ID),
		},
		{
			Order:       3,
			Title:       "Profile GPU utilization",
			Description: "Low GPU utilization over a long period suggests a CPU-bound data pipeline or a stalled rank.",
			Command:     `nvidia-smi dmon -s u -d 5 -c 12`,
		},
		{
			Order:       4,
			Title:       "Check data loader throughput",
			Description: "A slow data loader can starve GPUs. Increase --num-workers or use prefetch_factor. Check I/O wait.",
			Command:     `iostat -x 2 5`,
		},
		{
			Order:       5,
			Title:       "Review checkpoint frequency",
			Description: "Frequent checkpointing to slow storage can add hours to training. Checkpoint to local NVMe, not NFS.",
		},
		{
			Order:       6,
			Title:       "Increase timeout or split job",
			Description: "If the job is correctly implemented, adjust the timeout limit or split training into phases with checkpoints.",
		},
	}
	a.PreventionTips = []string{
		"Set conservative throughput estimates and add 30% buffer to timeout values.",
		"Use training profilers (PyTorch Profiler, Nsight) to identify bottlenecks before large runs.",
		"Implement early stopping if validation loss hasn't improved in N steps.",
	}
	a.Confidence = 0.88
}

func (e *Engine) ruleUserError(a *Analysis, job *models.Job) {
	a.Severity = "warning"
	a.RootCause = "User / Configuration Error"
	a.Headline = fmt.Sprintf(
		"Job %s failed due to a code or configuration error in the training script.",
		job.Name,
	)
	a.Summary = "The training process exited with an unhandled exception. " +
		"This is typically a shape mismatch, invalid configuration value, missing file, or import error. " +
		"The error is user-fixable and does not indicate a cluster infrastructure problem."
	a.DebuggingSteps = []Step{
		{
			Order:       1,
			Title:       "Read the full traceback",
			Description: "The last lines of the job log contain the Python traceback. Identify the exception type and the line number.",
			Command:     fmt.Sprintf(`kubectl logs -l job-name=%s --tail=80 | grep -A 20 "Traceback"`, job.ID),
		},
		{
			Order:       2,
			Title:       "Check input tensor shapes",
			Description: "Shape mismatches are the most common cause. Add print(tensor.shape) or use a debugger to trace the data flow.",
		},
		{
			Order:       3,
			Title:       "Validate config file",
			Description: "Confirm all required config keys are present and values are within valid ranges.",
		},
		{
			Order:       4,
			Title:       "Run locally on a single GPU",
			Description: "Reproduce the error on a single machine with a small batch before resubmitting to the cluster.",
		},
	}
	a.PreventionTips = []string{
		"Add input validation at the start of training scripts to catch shape/type errors before allocating GPU memory.",
		"Use a CI pipeline to run a 1-step sanity check on every code change before cluster submission.",
	}
	a.Confidence = 0.85
}

func (e *Engine) analyzePreemptedJob(a *Analysis, job *models.Job) {
	a.Severity = "warning"
	a.RootCause = "Scheduler Preemption"
	a.Headline = fmt.Sprintf(
		"Job %s (priority %d) was preempted to free capacity for a higher-priority workload.",
		job.Name, job.Priority,
	)
	a.Summary = fmt.Sprintf(
		"The cluster scheduler terminated this job at %.0f seconds to reclaim GPUs for a higher-priority job. "+
			"The job's checkpoint (if enabled) was saved before termination. This is expected behaviour for low-priority jobs.",
		job.DurationSeconds(),
	)
	a.DebuggingSteps = []Step{
		{
			Order:       1,
			Title:       "Verify checkpoint was saved",
			Description: "Check the log tail for 'Checkpoint saved' before the SIGTERM. If missing, the checkpoint may be corrupt.",
			Command:     fmt.Sprintf(`kubectl logs -l job-name=%s --tail=20`, job.ID),
		},
		{
			Order:       2,
			Title:       "Resume from checkpoint",
			Description: "Resubmit the job with --resume-from-checkpoint=<path>. Set a higher priority to avoid repeat preemption.",
		},
		{
			Order:       3,
			Title:       "Increase job priority",
			Description: fmt.Sprintf("Current priority is %d. Raise it above 5 to protect against preemption in most scenarios.", job.Priority),
		},
		{
			Order:       4,
			Title:       "Request a guaranteed node pool",
			Description: "If this job must not be interrupted, request a dedicated or reserved node pool with no preemption policy.",
		},
	}
	a.PreventionTips = []string{
		"Enable checkpoint-on-preemption hooks in your training framework to guarantee state is saved on SIGTERM.",
		"Use spot/preemptible instances only for fault-tolerant workloads with frequent checkpointing.",
	}
	a.Confidence = 0.97
}

func (e *Engine) analyzeUnknownFailure(a *Analysis, job *models.Job) {
	a.Severity = "warning"
	a.RootCause = "Unknown"
	a.Headline = fmt.Sprintf("Job %s failed without a clear failure signal.", job.Name)
	a.Summary = "The job entered a failed state but no specific failure reason was recorded. " +
		"This may indicate a signal-9 kill from an external process, a node reboot, or a missing exit code handler."
	a.DebuggingSteps = []Step{
		{
			Order:       1,
			Title:       "Check node events",
			Description: "Look for node reboots, kernel panics, or OOM killer activity around the job's end time.",
			Command:     `kubectl get events --sort-by='.lastTimestamp' | tail -30`,
		},
		{
			Order:       2,
			Title:       "Check job exit code",
			Description: "The exit code may reveal the kill signal. 137 = OOM kill, 143 = SIGTERM, 1 = generic error.",
			Command:     fmt.Sprintf(`kubectl describe job %s | grep "Exit Code"`, job.ID),
		},
	}
	a.Confidence = 0.40
}

func (e *Engine) analyzeRunningJob(a *Analysis, job *models.Job) {
	a.Severity = "info"
	a.RootCause = "Currently Running"
	durationMin := job.DurationSeconds() / 60
	a.Headline = fmt.Sprintf(
		"Job %s has been running for %.0f minutes on %s.",
		job.Name, durationMin, job.ModelName,
	)
	a.Summary = fmt.Sprintf(
		"Training is in progress. The job is using %d GPUs across node(s) %v. "+
			"Monitor GPU utilization and memory to ensure training is proceeding efficiently.",
		job.RequestedGPUs, job.AssignedNodes,
	)
	a.DebuggingSteps = []Step{
		{
			Order:       1,
			Title:       "Monitor GPU utilization",
			Description: "Healthy training should sustain >80% GPU utilization. Drops below 60% suggest a data pipeline bottleneck.",
			Command:     `nvidia-smi dmon -s u -d 5`,
		},
		{
			Order:       2,
			Title:       "Check training loss",
			Description: "Monitor loss curve via TensorBoard or WandB. Diverging loss may indicate a learning rate or data issue.",
		},
	}
	a.Confidence = 0.99
}

func (e *Engine) analyzeCompletedJob(a *Analysis, job *models.Job) {
	a.Severity = "info"
	a.RootCause = "Completed Successfully"
	a.Headline = fmt.Sprintf("Job %s completed successfully in %.0f minutes.", job.Name, job.DurationSeconds()/60)
	a.Summary = fmt.Sprintf(
		"Training finished without errors. The job ran for %.0f seconds and used %d GPUs. "+
			"Check the final checkpoint and evaluation metrics.",
		job.DurationSeconds(), job.RequestedGPUs,
	)
	a.DebuggingSteps = nil
	a.Confidence = 1.0
}

func (e *Engine) analyzeQueuedJob(a *Analysis, job *models.Job) {
	a.Severity = "info"
	a.RootCause = "Waiting for Capacity"
	a.Headline = fmt.Sprintf("Job %s is queued — waiting for %d GPUs to become available.", job.Name, job.RequestedGPUs)
	a.Summary = "The scheduler has accepted the job but cannot place it yet because sufficient GPU capacity is not available. " +
		"This is normal during high cluster utilisation."
	a.DebuggingSteps = []Step{
		{
			Order:       1,
			Title:       "Check cluster capacity",
			Description: "Review the Nodes page to see how many GPUs are currently allocated vs idle.",
		},
		{
			Order:       2,
			Title:       "Reduce GPU request",
			Description: fmt.Sprintf("Job requests %d GPUs. If the model fits on fewer GPUs (e.g. with tensor parallelism), reduce the request.", job.RequestedGPUs),
		},
	}
	a.Confidence = 0.95
}

// firstAssignedNode returns the first node ID from a job's AssignedNodes, or empty string.
func firstAssignedNode(job *models.Job) string {
	if len(job.AssignedNodes) > 0 {
		return job.AssignedNodes[0]
	}
	return "<node>"
}
