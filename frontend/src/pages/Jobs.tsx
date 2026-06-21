import { useState } from 'react'
import { Briefcase, X, Terminal, AlertCircle, ChevronDown, ChevronUp } from 'lucide-react'
import { useQuery } from '@/hooks/useQuery'
import { api } from '@/lib/api'
import { Badge } from '@/components/shared/Badge'
import { Card } from '@/components/shared/Card'
import { PageSpinner } from '@/components/shared/Spinner'
import { EmptyState } from '@/components/shared/EmptyState'
import { SeverityDot } from '@/components/shared/AlertBadge'
import {
  cn, jobStatusBg, failureReasonLabel, formatDuration,
  formatRelative, alertSeverityBg,
} from '@/lib/utils'
import type { Job, JobStatus, Alert } from '@/types'

const STATUS_OPTIONS: Array<{ value: string; label: string }> = [
  { value: '',          label: 'All statuses' },
  { value: 'running',   label: 'Running' },
  { value: 'queued',    label: 'Queued' },
  { value: 'failed',    label: 'Failed' },
  { value: 'completed', label: 'Completed' },
  { value: 'preempted', label: 'Preempted' },
]

function frameworkIcon(fw: string) {
  const icons: Record<string, string> = { pytorch: 'PT', jax: 'JX', tensorflow: 'TF' }
  return icons[fw] ?? '??'
}

// ─── Job Detail Drawer ────────────────────────────────────────────────────────

function JobDetail({ job, onClose }: { job: Job; onClose: () => void }) {
  const [logsOpen, setLogsOpen] = useState(true)
  const alertsQuery = useQuery((s) => api.jobs.alerts(job.id, s), ['job-alerts', job.id])

  return (
    <div className="fixed inset-0 z-50 flex items-stretch">
      <div className="flex-1 bg-black/50" onClick={onClose} />
      <div className="w-[600px] bg-surface-1 border-l border-border overflow-y-auto animate-fade-in">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-border p-4 sticky top-0 bg-surface-1 z-10">
          <div className="flex items-center gap-2 min-w-0">
            <Briefcase className="h-4 w-4 text-accent shrink-0" />
            <span className="font-mono text-sm font-semibold text-gray-200 truncate">{job.name}</span>
            <Badge className={cn(jobStatusBg(job.status))}>{job.status}</Badge>
          </div>
          <button onClick={onClose} className="text-muted hover:text-gray-200 ml-2">
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="p-4 space-y-5">
          {/* Failure banner */}
          {job.status === 'failed' && job.failure_reason && (
            <div className="flex items-start gap-2.5 rounded-md border border-danger/30 bg-danger/10 p-3">
              <AlertCircle className="h-4 w-4 text-danger mt-0.5 shrink-0" />
              <div>
                <p className="text-xs font-semibold text-danger">{failureReasonLabel(job.failure_reason)}</p>
                {job.failure_message && (
                  <p className="text-xs text-danger/80 mt-0.5">{job.failure_message}</p>
                )}
              </div>
            </div>
          )}

          {/* Metadata grid */}
          <div className="grid grid-cols-2 gap-3 text-xs">
            {[
              { label: 'Job ID',       value: job.id },
              { label: 'Framework',    value: job.framework.toUpperCase() },
              { label: 'Model',        value: job.model_name },
              { label: 'User',         value: job.user_id },
              { label: 'Priority',     value: `${job.priority}/10` },
              { label: 'GPUs',         value: `${job.requested_gpus} requested` },
              { label: 'Nodes',        value: job.assigned_nodes?.join(', ') || '—' },
              { label: 'Duration',     value: formatDuration(job.start_time, job.end_time) },
              { label: 'Started',      value: job.start_time ? formatRelative(job.start_time) : '—' },
              { label: 'Ended',        value: job.end_time ? formatRelative(job.end_time) : '—' },
            ].map(({ label, value }) => (
              <div key={label} className="rounded-md border border-border bg-surface-2 p-2.5">
                <p className="text-muted mb-0.5">{label}</p>
                <p className="font-mono font-medium text-gray-200 truncate">{value}</p>
              </div>
            ))}
          </div>

          {/* Log tail */}
          <div>
            <button
              onClick={() => setLogsOpen(v => !v)}
              className="flex items-center gap-1.5 text-xs font-semibold text-muted uppercase tracking-wider mb-2 hover:text-gray-200 transition-colors"
            >
              <Terminal className="h-3.5 w-3.5" />
              Log Tail
              {logsOpen ? <ChevronUp className="h-3 w-3" /> : <ChevronDown className="h-3 w-3" />}
            </button>
            {logsOpen && (
              <div className="rounded-md border border-border bg-surface bg-opacity-80 p-3 font-mono text-[10px] text-gray-400 max-h-48 overflow-y-auto space-y-0.5">
                {(job.log_tail ?? []).length === 0 ? (
                  <p className="text-muted">No logs available.</p>
                ) : (
                  (job.log_tail ?? []).map((line, i) => (
                    <p key={i} className={cn(
                      line.includes('[ERROR]') ? 'text-danger' :
                      line.includes('[WARN]')  ? 'text-warning' : 'text-gray-400'
                    )}>
                      {line}
                    </p>
                  ))
                )}
              </div>
            )}
          </div>

          {/* Related alerts */}
          {alertsQuery.status === 'success' && alertsQuery.data.length > 0 && (
            <div>
              <h3 className="text-xs font-semibold text-muted uppercase tracking-wider mb-2">
                Related Alerts
              </h3>
              <div className="space-y-1.5">
                {alertsQuery.data.map((a: Alert) => (
                  <div key={a.id} className="flex items-start gap-2 rounded-md border border-border p-2.5">
                    <SeverityDot severity={a.severity} />
                    <div className="flex-1 min-w-0">
                      <p className="text-xs font-medium text-gray-200">{a.title}</p>
                      <p className="text-[10px] text-muted">{formatRelative(a.triggered_at)}</p>
                    </div>
                    <Badge className={cn(alertSeverityBg(a.severity))}>{a.severity}</Badge>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export function Jobs() {
  const [statusFilter, setStatusFilter] = useState('')
  const [selected, setSelected] = useState<Job | null>(null)
  const [sortField, setSortField] = useState<'created_at' | 'priority'>('created_at')
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('desc')

  const query = useQuery(
    (s) => api.jobs.list(statusFilter ? { status: statusFilter } : undefined, s),
    ['jobs', statusFilter],
    { pollMs: 5_000 }
  )

  if (query.status === 'loading') return <PageSpinner />
  if (query.status === 'error') return (
    <div className="p-6"><p className="text-danger text-sm">{query.error}</p></div>
  )

  const jobs = [...query.data].sort((a, b) => {
    const av = sortField === 'priority' ? a.priority : new Date(a.created_at).getTime()
    const bv = sortField === 'priority' ? b.priority : new Date(b.created_at).getTime()
    return sortDir === 'desc' ? bv - av : av - bv
  })

  function toggleSort(field: typeof sortField) {
    if (sortField === field) setSortDir(d => d === 'desc' ? 'asc' : 'desc')
    else { setSortField(field); setSortDir('desc') }
  }

  const SortIcon = sortDir === 'desc' ? ChevronDown : ChevronUp

  return (
    <div className="p-6 space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-bold text-gray-100">Jobs</h1>
          <p className="text-xs text-muted mt-0.5">{jobs.length} jobs</p>
        </div>
        <select
          value={statusFilter}
          onChange={e => setStatusFilter(e.target.value)}
          className="rounded-md border border-border bg-surface-2 px-3 py-1.5 text-xs text-gray-200 focus:outline-none focus:border-accent"
        >
          {STATUS_OPTIONS.map(o => (
            <option key={o.value} value={o.value}>{o.label}</option>
          ))}
        </select>
      </div>

      {jobs.length === 0 ? (
        <EmptyState icon={<Briefcase className="h-8 w-8" />} title="No jobs found" />
      ) : (
        <div className="rounded-lg border border-border overflow-hidden">
          {/* Table header */}
          <div className="grid grid-cols-12 gap-2 border-b border-border bg-surface-2 px-4 py-2.5 text-[10px] font-semibold text-muted uppercase tracking-wider">
            <div className="col-span-4">Job</div>
            <div className="col-span-1">FW</div>
            <div className="col-span-2">Status</div>
            <div className="col-span-1">GPUs</div>
            <div
              className="col-span-1 cursor-pointer hover:text-gray-200 flex items-center gap-0.5"
              onClick={() => toggleSort('priority')}
            >
              Pri {sortField === 'priority' && <SortIcon className="h-3 w-3" />}
            </div>
            <div className="col-span-2">Duration</div>
            <div
              className="col-span-1 cursor-pointer hover:text-gray-200 flex items-center gap-0.5"
              onClick={() => toggleSort('created_at')}
            >
              Created {sortField === 'created_at' && <SortIcon className="h-3 w-3" />}
            </div>
          </div>

          {/* Rows */}
          <div className="divide-y divide-border">
            {jobs.map(j => (
              <div
                key={j.id}
                onClick={() => setSelected(j)}
                className="grid grid-cols-12 gap-2 px-4 py-2.5 text-xs cursor-pointer hover:bg-surface-2 transition-colors items-center"
              >
                <div className="col-span-4 min-w-0">
                  <p className="font-mono text-gray-200 truncate">{j.name}</p>
                  <p className="text-[10px] text-muted truncate">{j.id}</p>
                </div>
                <div className="col-span-1">
                  <span className="rounded bg-surface-3 px-1.5 py-0.5 text-[10px] font-mono text-muted">
                    {frameworkIcon(j.framework)}
                  </span>
                </div>
                <div className="col-span-2">
                  <Badge className={cn(jobStatusBg(j.status))}>{j.status}</Badge>
                  {j.failure_reason && (
                    <p className="text-[9px] text-muted mt-0.5">{failureReasonLabel(j.failure_reason)}</p>
                  )}
                </div>
                <div className="col-span-1 font-mono text-muted">{j.requested_gpus}</div>
                <div className="col-span-1 font-mono text-muted">{j.priority}</div>
                <div className="col-span-2 font-mono text-muted">
                  {formatDuration(j.start_time, j.end_time)}
                </div>
                <div className="col-span-1 text-muted">{formatRelative(j.created_at)}</div>
              </div>
            ))}
          </div>
        </div>
      )}

      {selected && <JobDetail job={selected} onClose={() => setSelected(null)} />}
    </div>
  )
}
