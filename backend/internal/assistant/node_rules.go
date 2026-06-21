package assistant

import (
	"fmt"

	"github.com/clusterops/backend/internal/models"
)

// ─── node rules ───────────────────────────────────────────────────────────────

func (e *Engine) analyzeDegradedNode(a *Analysis, node *models.Node) {
	// Look for the most likely cause by inspecting GPU telemetry.
	highTemp := maxFloat(node.GPUTemperature) > 85
	highMem := anyAbove(node.GPUMemoryUsedGB, 78) // >78 GB of 80 GB

	a.Severity = "warning"
	a.RootCause = classifyNodeDegradation(highTemp, highMem)
	a.Headline = fmt.Sprintf(
		"Node %s is degraded — ECC errors detected; still serving traffic with reduced reliability.",
		node.Hostname,
	)
	a.Summary = buildNodeDegradedSummary(node, highTemp, highMem)

	steps := []Step{
		{
			Order:       1,
			Title:       "Check ECC error counts",
			Description: "Count volatile and aggregate ECC errors per GPU. Any uncorrectable errors require immediate action.",
			Command:     fmt.Sprintf(`ssh %s "nvidia-smi --query-gpu=index,ecc.errors.uncorrected.volatile.total --format=csv"`, node.Hostname),
		},
		{
			Order:       2,
			Title:       "Check GPU temperatures",
			Description: fmt.Sprintf("Current max temperature is %.0f°C. Sustained operation above 87°C triggers throttling.", maxFloat(node.GPUTemperature)),
			Command:     fmt.Sprintf(`ssh %s "nvidia-smi --query-gpu=index,temperature.gpu,clocks_throttle_reasons.hw_thermal_slowdown --format=csv"`, node.Hostname),
		},
		{
			Order:       3,
			Title:       "Inspect kernel logs",
			Description: "Check dmesg for Xid errors, PCIe errors, or NVLink faults.",
			Command:     fmt.Sprintf(`ssh %s "dmesg | grep -E 'Xid|NVRM|ECC|NVLink' | tail -20"`, node.Hostname),
		},
		{
			Order:       4,
			Title:       "Run DCGM health check",
			Description: "If DCGM is available, run a comprehensive GPU health diagnostic.",
			Command:     fmt.Sprintf(`ssh %s "dcgmi health -g 0 -j"`, node.Hostname),
		},
		{
			Order:       5,
			Title:       "Drain node if errors persist",
			Description: "If ECC errors continue, cordon the node to prevent new scheduling and drain existing workloads.",
			Command:     fmt.Sprintf(`kubectl cordon %s && kubectl drain %s --ignore-daemonsets --delete-emptydir-data`, node.ID, node.ID),
		},
	}

	if highTemp {
		steps = append(steps, Step{
			Order:       6,
			Title:       "Check cooling system",
			Description: "High temperatures may indicate a fan failure, blocked airflow, or data center cooling issue. Check physical infrastructure.",
		})
	}

	a.DebuggingSteps = steps
	a.PreventionTips = []string{
		"Enable DCGM health monitoring with automated node drain policies on uncorrectable ECC errors.",
		"Set GPU temperature alert thresholds at 80°C for early warning.",
		"Schedule quarterly GPU firmware and driver updates.",
	}
	a.Confidence = 0.82
}

func (e *Engine) analyzeUnavailableNode(a *Analysis, node *models.Node) {
	a.Severity = "critical"
	a.RootCause = "Node Unreachable"
	a.Headline = fmt.Sprintf(
		"Node %s is unreachable — no traffic is being routed to it.",
		node.Hostname,
	)
	a.Summary = fmt.Sprintf(
		"The node has failed 3 or more consecutive health checks and has been marked unavailable. "+
			"All %d GPUs on this node (%s) are offline. Any jobs that were running on this node have been "+
			"terminated with a hardware_fault error. Last seen: %s.",
		node.GPUCount, node.GPUModel, node.LastSeen.Format("15:04:05 UTC"),
	)
	a.DebuggingSteps = []Step{
		{
			Order:       1,
			Title:       "Verify network connectivity",
			Description: "Ping and SSH to the node to determine if it is a network issue or a full system crash.",
			Command:     fmt.Sprintf(`ping -c 3 %s && ssh %s "uptime"`, node.Hostname, node.Hostname),
		},
		{
			Order:       2,
			Title:       "Check node kubelet status",
			Description: "If the node is reachable but the kubelet is down, restart it.",
			Command:     fmt.Sprintf(`ssh %s "systemctl status kubelet && journalctl -u kubelet -n 50"`, node.Hostname),
		},
		{
			Order:       3,
			Title:       "Check for kernel panic or OOM kill",
			Description: "Look for kernel panic messages or out-of-memory kills in system logs.",
			Command:     fmt.Sprintf(`ssh %s "journalctl -k -p 3 -n 30"`, node.Hostname),
		},
		{
			Order:       4,
			Title:       "Check GPU driver status",
			Description: "NVRM driver crash can take down the node. Check if the nvidia module is loaded.",
			Command:     fmt.Sprintf(`ssh %s "lsmod | grep nvidia && nvidia-smi"`, node.Hostname),
		},
		{
			Order:       5,
			Title:       "Reboot if safe",
			Description: "If the node is in a crashed state and all jobs have been rescheduled, reboot to restore service.",
			Command:     fmt.Sprintf(`ssh %s "sudo reboot"`, node.Hostname),
		},
		{
			Order:       6,
			Title:       "Remove from cluster if hardware failure confirmed",
			Description: "If a GPU or motherboard fault is confirmed, remove the node from the cluster for physical replacement.",
			Command:     fmt.Sprintf(`kubectl delete node %s`, node.ID),
		},
	}
	a.PreventionTips = []string{
		"Deploy a node problem detector (NPD) to automatically detect and report node-level failures.",
		"Use out-of-band (IPMI/BMC) management for nodes that may lose in-band connectivity.",
		"Implement automatic job rescheduling with fault-tolerant training (Torch Elastic, Jax distributed restart).",
	}
	a.Confidence = 0.90
}

func (e *Engine) analyzeHealthyNode(a *Analysis, node *models.Node) {
	a.Severity = "info"
	a.RootCause = "Node Healthy"
	a.Headline = fmt.Sprintf(
		"Node %s is healthy — %d/%d GPUs allocated, avg utilization %.0f%%.",
		node.Hostname, node.AllocatedGPUs, node.GPUCount, node.AvgGPUUtilization(),
	)
	a.Summary = fmt.Sprintf(
		"The node is operating normally. %d GPUs are allocated to active training jobs. "+
			"Average GPU utilization is %.0f%%. No hardware faults detected.",
		node.AllocatedGPUs, node.AvgGPUUtilization(),
	)
	a.DebuggingSteps = nil
	a.Confidence = 1.0
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func classifyNodeDegradation(highTemp, highMem bool) string {
	if highTemp && highMem {
		return "Thermal Throttle + High Memory Pressure"
	}
	if highTemp {
		return "Thermal Throttle (GPU overheating)"
	}
	if highMem {
		return "High GPU Memory Pressure"
	}
	return "ECC / Hardware Error"
}

func buildNodeDegradedSummary(node *models.Node, highTemp, highMem bool) string {
	base := fmt.Sprintf(
		"Node %s has entered a degraded state due to hardware errors. "+
			"It is still serving traffic but with reduced reliability. ",
		node.Hostname,
	)
	if highTemp {
		base += fmt.Sprintf(
			"GPU temperatures are elevated (max %.0f°C), which may be causing thermal throttling and clock reductions. ",
			maxFloat(node.GPUTemperature),
		)
	}
	if highMem {
		base += fmt.Sprintf(
			"GPU memory utilization is critically high (max %.0f GB / 80 GB). "+
				"A memory leak or oversized allocation may be present. ",
			maxFloat(node.GPUMemoryUsedGB),
		)
	}
	base += "Immediate investigation is recommended to prevent a full node outage."
	return base
}

func maxFloat(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	m := vals[0]
	for _, v := range vals[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

func anyAbove(vals []float64, threshold float64) bool {
	for _, v := range vals {
		if v > threshold {
			return true
		}
	}
	return false
}
