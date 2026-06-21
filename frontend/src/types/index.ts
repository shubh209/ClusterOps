// Mirrors the Go models exactly — keep in sync with backend/internal/models/

export type NodeStatus = 'healthy' | 'degraded' | 'unavailable' | 'maintenance'
export type JobStatus = 'queued' | 'running' | 'failed' | 'completed' | 'preempted'
export type FailureReason = 'oom' | 'hardware_fault' | 'preemption' | 'timeout' | 'user_error'
export type Framework = 'pytorch' | 'jax' | 'tensorflow'
export type AlertSeverity = 'critical' | 'warning' | 'info'
export type AlertType =
  | 'node_unavailable'
  | 'node_degraded'
  | 'gpu_high_temperature'
  | 'gpu_memory_full'
  | 'job_failed'
  | 'capacity_waste'
  | 'job_timeout'
  | 'cluster_degraded'

export interface Node {
  id: string
  hostname: string
  status: NodeStatus
  gpu_count: number
  gpu_model: string
  cpu_cores: number
  memory_gb: number
  allocated_gpus: number
  gpu_utilization: number[]
  gpu_memory_used_gb: number[]
  gpu_memory_total_gb: number[]
  gpu_temperature_c: number[]
  gpu_power_watts: number[]
  labels: Record<string, string>
  last_seen: string
  created_at: string
}

export interface Job {
  id: string
  name: string
  status: JobStatus
  framework: Framework
  model_name: string
  requested_gpus: number
  assigned_nodes: string[]
  start_time?: string
  end_time?: string
  failure_reason?: FailureReason
  failure_message?: string
  log_tail?: string[]
  priority: number
  user_id: string
  created_at: string
  updated_at: string
}

export interface GPUMetric {
  id: number
  node_id: string
  gpu_index: number
  timestamp: string
  utilization_pct: number
  memory_used_gb: number
  memory_total_gb: number
  temperature_c: number
  power_watts: number
}

export interface GPUTimeSeries {
  node_id: string
  gpu_index: number
  points: GPUMetric[]
}

export interface ClusterGPUSummary {
  total_gpus: number
  allocated_gpus: number
  idle_gpus: number
  wasted_gpus: number
  avg_utilization_pct: number
  avg_memory_used_gb: number
  avg_temperature_c: number
  waste_percent: number
}

export interface ClusterSummary {
  health_score: number
  total_nodes: number
  healthy_nodes: number
  degraded_nodes: number
  unavailable_nodes: number
  active_jobs: number
  queued_jobs: number
  failed_jobs_last_1h: number
  gpu: ClusterGPUSummary
  active_alerts: number
  updated_at: string
}

export interface Alert {
  id: string
  severity: AlertSeverity
  type: AlertType
  title: string
  message: string
  node_id?: string
  job_id?: string
  triggered_at: string
  resolved_at?: string
  resolved: boolean
}

export interface CapacityRow {
  node_id: string
  hostname: string
  status: NodeStatus
  total_gpus: number
  allocated_gpus: number
  idle_gpus: number
  waste_percent: number
  avg_utilization_pct: number
}

export interface AssistantStep {
  order: number
  title: string
  description: string
  command?: string
}

export interface AssistantAnalysis {
  target_type: 'job' | 'node'
  target_id: string
  target_name: string
  headline: string
  severity: AlertSeverity
  root_cause: string
  summary: string
  debugging_steps: AssistantStep[]
  prevention_tips: string[]
  related_alert_ids: string[]
  confidence: number
  generated_at: string
}
