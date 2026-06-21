import { useState, useCallback } from 'react'
import {
  RadialBarChart, RadialBar, ResponsiveContainer,
  PieChart, Pie, Cell, Tooltip, AreaChart, Area, XAxis, YAxis, CartesianGrid,
} from 'recharts'
import { AlertTriangle, Server, Cpu, Zap, Activity } from 'lucide-react'
import { useSSE } from '@/hooks/useSSE'
import { useQuery } from '@/hooks/useQuery'
import { api } from '@/lib/api'
import { StatCard } from '@/components/shared/StatCard'
import { Card, CardHeader, CardTitle } from '@/components/shared/Card'
import { Badge } from '@/components/shared/Badge'
import { SeverityDot } from '@/components/shared/AlertBadge'
import { GPUHeatmapRow } from '@/components/shared/GPUBar'
import { PageSpinner } from '@/components/shared/Spinner'
import {
  cn, healthScoreColor, healthScoreLabel, formatRelative,
  alertSeverityBg, jobStatusBg, nodeStatusBg,
} from '@/lib/utils'
import type { ClusterSummary, Node, Alert } from '@/types'

const JOB_COLORS = ['#58a6ff', '#3fb950', '#f85149', '#d29922', '#768390']
const JOB_LABELS = ['Running', 'Completed', 'Failed', 'Preempted', 'Queued']

export function Dashboard() {
  const summaryQuery = useQuery((s) => api.cluster.summary(s), 'cluster-summary')
  const nodesQuery = useQuery((s) => api.nodes.list(s), 'node-list')
  const alertsQuery = useQuery((s) => api.alerts.list(true, s), 'active-alerts')

  // SSE overwrites the polled state for instant feel
  const [summary, setSummary] = useState<ClusterSummary | null>(null)
  const [nodes, setNodes] = useState<Node[] | null>(null)
  const [alerts, setAlerts] = useState<Alert[] | null>(null)

  useSSE({
    cluster_summary: useCallback((d) => setSummary(d as ClusterSummary), []),
    nodes: useCallback((d) => setNodes(d as Node[]), []),
    alerts: useCallback((d) => setAlerts(d as Alert[]), []),
  })

  const s = summary ?? (summaryQuery.status === 'success' ? summaryQuery.data : null)
  const nodeList = nodes ?? (nodesQuery.status === 'success' ? nodesQuery.data : null)
  const alertList = alerts ?? (alertsQuery.status === 'success' ? alertsQuery.data : null)

  if (!s) return <PageSpinner />

  const healthPct = Math.round(s.health_score)

  const jobPieData = [
    { name: 'Running',   value: s.active_jobs },
    { name: 'Failed',    value: s.failed_jobs_last_1h },
    { name: 'Queued',    value: s.queued_jobs },
  ].filter(d => d.value > 0)

  return (
    <div className="p-6 space-y-6 animate-fade-in">
      {/* ── Header ───────────────────────────────────────────────────── */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-bold text-gray-100">Cluster Dashboard</h1>
          <p className="text-xs text-muted mt-0.5">
            Last updated {s.updated_at ? formatRelative(s.updated_at) : '—'} · Live via SSE
          </p>
        </div>
        <div className="flex items-center gap-2">
          <span className="h-1.5 w-1.5 rounded-full bg-success animate-pulse-slow" />
          <span className="text-xs text-success">Live</span>
        </div>
      </div>

      {/* ── Health score + stat cards ─────────────────────────────────── */}
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-5">
        {/* Health score radial */}
        <div className="col-span-2 lg:col-span-1 rounded-lg border border-border bg-surface-1 p-4 flex flex-col items-center justify-center">
          <p className="text-xs font-medium text-muted uppercase tracking-wider mb-2">Health Score</p>
          <div className="relative h-28 w-28">
            <ResponsiveContainer width="100%" height="100%">
              <RadialBarChart
                innerRadius="70%"
                outerRadius="90%"
                data={[{ value: healthPct, fill: healthPct >= 80 ? '#3fb950' : healthPct >= 50 ? '#d29922' : '#f85149' }]}
                startAngle={90}
                endAngle={90 - 360 * (healthPct / 100)}
              >
                <RadialBar dataKey="value" background={{ fill: '#2d333b' }} cornerRadius={4} />
              </RadialBarChart>
            </ResponsiveContainer>
            <div className="absolute inset-0 flex flex-col items-center justify-center">
              <span className={cn('text-2xl font-bold font-mono', healthScoreColor(s.health_score))}>
                {healthPct}
              </span>
              <span className="text-[10px] text-muted">{healthScoreLabel(s.health_score)}</span>
            </div>
          </div>
        </div>

        <StatCard
          label="Total Nodes"
          value={s.total_nodes}
          sub={`${s.healthy_nodes} healthy · ${s.degraded_nodes} degraded · ${s.unavailable_nodes} down`}
          icon={<Server className="h-5 w-5" />}
        />
        <StatCard
          label="Active Jobs"
          value={s.active_jobs}
          sub={`${s.queued_jobs} queued · ${s.failed_jobs_last_1h} failed/hr`}
          icon={<Activity className="h-5 w-5" />}
          valueClassName={s.active_jobs > 0 ? 'text-accent' : 'text-gray-100'}
        />
        <StatCard
          label="GPU Utilization"
          value={`${Math.round(s.gpu.avg_utilization_pct)}%`}
          sub={`${s.gpu.total_gpus} total · ${s.gpu.allocated_gpus} allocated`}
          icon={<Cpu className="h-5 w-5" />}
          valueClassName={s.gpu.avg_utilization_pct >= 70 ? 'text-success' : 'text-warning'}
        />
        <StatCard
          label="GPU Waste"
          value={`${Math.round(s.gpu.waste_percent)}%`}
          sub={`${s.gpu.wasted_gpus} GPUs idle while allocated`}
          icon={<Zap className="h-5 w-5" />}
          valueClassName={s.gpu.waste_percent > 20 ? 'text-danger' : s.gpu.waste_percent > 10 ? 'text-warning' : 'text-success'}
        />
      </div>

      {/* ── Middle row ───────────────────────────────────────────────── */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        {/* GPU Heatmap */}
        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle>GPU Utilization Heatmap</CardTitle>
            <span className="text-xs text-muted">Per-GPU, per-node · darker = higher util</span>
          </CardHeader>
          <div className="space-y-3">
            {(nodeList ?? []).map((n) => (
              <div key={n.id} className="flex items-center gap-3">
                <div className="w-28 shrink-0">
                  <p className="text-xs font-mono text-gray-300 truncate">{n.hostname.split('.')[0]}</p>
                  <Badge className={cn('mt-0.5', nodeStatusBg(n.status))}>{n.status}</Badge>
                </div>
                <GPUHeatmapRow utilization={n.gpu_utilization ?? []} nodeId={n.id} />
                <span className="w-10 shrink-0 text-right text-xs font-mono text-muted">
                  {n.allocated_gpus}/{n.gpu_count}
                </span>
              </div>
            ))}
            {!nodeList && <p className="text-xs text-muted">Loading nodes…</p>}
          </div>
          <div className="mt-3 flex items-center gap-3 border-t border-border pt-3">
            {[
              { color: 'bg-success', label: '80–100%' },
              { color: 'bg-accent', label: '40–79%' },
              { color: 'bg-warning', label: '10–39%' },
              { color: 'bg-surface-4', label: '0–9%' },
            ].map(({ color, label }) => (
              <div key={label} className="flex items-center gap-1.5">
                <span className={cn('h-3 w-3 rounded-sm', color)} />
                <span className="text-[10px] text-muted">{label}</span>
              </div>
            ))}
          </div>
        </Card>

        {/* Job distribution pie */}
        <Card>
          <CardHeader><CardTitle>Job Distribution</CardTitle></CardHeader>
          {jobPieData.length > 0 ? (
            <>
              <ResponsiveContainer width="100%" height={160}>
                <PieChart>
                  <Pie
                    data={jobPieData}
                    cx="50%" cy="50%"
                    innerRadius={45} outerRadius={70}
                    paddingAngle={3}
                    dataKey="value"
                  >
                    {jobPieData.map((entry, i) => (
                      <Cell key={entry.name} fill={JOB_COLORS[i % JOB_COLORS.length]} />
                    ))}
                  </Pie>
                  <Tooltip
                    contentStyle={{ background: '#1c2128', border: '1px solid #373e47', borderRadius: 6 }}
                    labelStyle={{ color: '#cdd9e5' }}
                  />
                </PieChart>
              </ResponsiveContainer>
              <div className="space-y-1.5 mt-2">
                {jobPieData.map((d, i) => (
                  <div key={d.name} className="flex items-center justify-between text-xs">
                    <div className="flex items-center gap-2">
                      <span className="h-2 w-2 rounded-full" style={{ background: JOB_COLORS[i % JOB_COLORS.length] }} />
                      <span className="text-muted">{d.name}</span>
                    </div>
                    <span className="font-mono text-gray-300">{d.value}</span>
                  </div>
                ))}
              </div>
            </>
          ) : (
            <p className="py-8 text-center text-xs text-muted">No active jobs</p>
          )}
        </Card>
      </div>

      {/* ── Bottom row: capacity waste + alerts feed ──────────────────── */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        {/* Capacity chart */}
        <Card>
          <CardHeader>
            <CardTitle>Capacity Breakdown</CardTitle>
          </CardHeader>
          <div className="space-y-2">
            {(nodeList ?? []).map((n) => {
              const allocPct = n.gpu_count > 0 ? (n.allocated_gpus / n.gpu_count) * 100 : 0
              return (
                <div key={n.id}>
                  <div className="flex justify-between text-xs mb-1">
                    <span className="font-mono text-muted">{n.hostname.split('.')[0]}</span>
                    <span className="font-mono text-gray-300">{n.allocated_gpus}/{n.gpu_count} GPUs</span>
                  </div>
                  <div className="h-1.5 rounded-full bg-surface-4 overflow-hidden">
                    <div
                      className="h-full rounded-full bg-accent transition-all duration-700"
                      style={{ width: `${allocPct}%` }}
                    />
                  </div>
                </div>
              )
            })}
          </div>
        </Card>

        {/* Active alerts feed */}
        <Card>
          <CardHeader>
            <CardTitle>Active Alerts</CardTitle>
            {s.active_alerts > 0 && (
              <Badge className="bg-danger/10 text-danger border-danger/30">
                {s.active_alerts} active
              </Badge>
            )}
          </CardHeader>
          <div className="space-y-2 max-h-56 overflow-y-auto">
            {(alertList ?? []).length === 0 ? (
              <p className="py-6 text-center text-xs text-muted">No active alerts</p>
            ) : (
              (alertList ?? []).slice(0, 8).map((a) => (
                <div key={a.id} className="flex items-start gap-2.5 rounded-md border border-border p-2.5">
                  <SeverityDot severity={a.severity} />
                  <div className="flex-1 min-w-0">
                    <p className="text-xs font-medium text-gray-200 leading-snug">{a.title}</p>
                    <p className="text-[10px] text-muted mt-0.5">{formatRelative(a.triggered_at)}</p>
                  </div>
                  <Badge className={cn(alertSeverityBg(a.severity))}>{a.severity}</Badge>
                </div>
              ))
            )}
          </div>
        </Card>
      </div>
    </div>
  )
}
