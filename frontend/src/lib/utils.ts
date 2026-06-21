import { type ClassValue, clsx } from 'clsx'
import { twMerge } from 'tailwind-merge'
import type { AlertSeverity, FailureReason, JobStatus, NodeStatus } from '@/types'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

// ─── Status colours ──────────────────────────────────────────────────────────

export function nodeStatusColor(status: NodeStatus): string {
  switch (status) {
    case 'healthy':     return 'text-success'
    case 'degraded':    return 'text-warning'
    case 'unavailable': return 'text-danger'
    case 'maintenance': return 'text-muted'
  }
}

export function nodeStatusBg(status: NodeStatus): string {
  switch (status) {
    case 'healthy':     return 'bg-success/10 text-success border-success/30'
    case 'degraded':    return 'bg-warning/10 text-warning border-warning/30'
    case 'unavailable': return 'bg-danger/10 text-danger border-danger/30'
    case 'maintenance': return 'bg-muted/10 text-muted border-muted/30'
  }
}

export function jobStatusBg(status: JobStatus): string {
  switch (status) {
    case 'running':   return 'bg-accent/10 text-accent border-accent/30'
    case 'queued':    return 'bg-muted/10 text-muted border-muted/30'
    case 'completed': return 'bg-success/10 text-success border-success/30'
    case 'failed':    return 'bg-danger/10 text-danger border-danger/30'
    case 'preempted': return 'bg-warning/10 text-warning border-warning/30'
  }
}

export function alertSeverityBg(severity: AlertSeverity): string {
  switch (severity) {
    case 'critical': return 'bg-danger/10 text-danger border-danger/30'
    case 'warning':  return 'bg-warning/10 text-warning border-warning/30'
    case 'info':     return 'bg-accent/10 text-accent border-accent/30'
  }
}

export function failureReasonLabel(reason?: FailureReason): string {
  if (!reason) return '—'
  const map: Record<FailureReason, string> = {
    oom:            'Out of Memory',
    hardware_fault: 'Hardware Fault',
    preemption:     'Preempted',
    timeout:        'Timeout',
    user_error:     'User Error',
  }
  return map[reason]
}

// ─── Formatting ──────────────────────────────────────────────────────────────

export function formatDuration(startIso?: string, endIso?: string): string {
  if (!startIso) return '—'
  const start = new Date(startIso).getTime()
  const end = endIso ? new Date(endIso).getTime() : Date.now()
  const secs = Math.floor((end - start) / 1000)
  if (secs < 60) return `${secs}s`
  const mins = Math.floor(secs / 60)
  if (mins < 60) return `${mins}m ${secs % 60}s`
  const hrs = Math.floor(mins / 60)
  return `${hrs}h ${mins % 60}m`
}

export function formatRelative(isoString: string): string {
  const diff = Date.now() - new Date(isoString).getTime()
  const secs = Math.floor(diff / 1000)
  if (secs < 60) return `${secs}s ago`
  const mins = Math.floor(secs / 60)
  if (mins < 60) return `${mins}m ago`
  const hrs = Math.floor(mins / 60)
  if (hrs < 24) return `${hrs}h ago`
  return `${Math.floor(hrs / 24)}d ago`
}

export function formatBytes(gb: number): string {
  if (gb >= 1000) return `${(gb / 1000).toFixed(1)} TB`
  return `${gb.toFixed(1)} GB`
}

export function pct(value: number, total: number): number {
  if (total === 0) return 0
  return Math.round((value / total) * 100)
}

export function healthScoreColor(score: number): string {
  if (score >= 80) return 'text-success'
  if (score >= 50) return 'text-warning'
  return 'text-danger'
}

export function healthScoreLabel(score: number): string {
  if (score >= 90) return 'Excellent'
  if (score >= 75) return 'Good'
  if (score >= 50) return 'Degraded'
  if (score >= 25) return 'Critical'
  return 'Down'
}
