import { useState } from 'react'
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Legend } from 'recharts'
import { Server, X, Thermometer, MemoryStick, Zap } from 'lucide-react'
import { useQuery } from '@/hooks/useQuery'
import { api } from '@/lib/api'
import { Card, CardHeader, CardTitle } from '@/components/shared/Card'
import { Badge } from '@/components/shared/Badge'
import { GPUBar } from '@/components/shared/GPUBar'
import { PageSpinner } from '@/components/shared/Spinner'
import { EmptyState } from '@/components/shared/EmptyState'
import { cn, nodeStatusBg, formatRelative, formatBytes } from '@/lib/utils'
import type { Node, GPUTimeSeries } from '@/types'
import { format } from 'date-fns'

// ─── Node Card ───────────────────────────────────────────────────────────────

function NodeCard({ node, onClick }: { node: Node; onClick: () => void }) {
  const avgUtil = node.gpu_utilization?.length
    ? node.gpu_utilization.reduce((a, b) => a + b, 0) / node.gpu_utilization.length
    : 0

  return (
    <div
      onClick={onClick}
      className="rounded-lg border border-border bg-surface-1 p-4 cursor-pointer hover:border-accent/50 hover:bg-surface-2 transition-colors"
    >
      <div className="flex items-start justify-between mb-3">
        <div>
          <p className="text-sm font-semibold text-gray-200 font-mono">
            {node.hostname.split('.')[0]}
          </p>
          <p className="text-[10px] text-muted mt-0.5">{node.hostname}</p>
        </div>
        <Badge className={cn(nodeStatusBg(node.status))}>{node.status}</Badge>
      </div>

      {/* GPU utilization bars */}
      <div className="space-y-1 mb-3">
        {(node.gpu_utilization ?? []).map((u, i) => (
          <GPUBar key={i} utilization={u} label={`G${i}`} height="h-1.5" showValue={false} />
        ))}
      </div>

      <div className="grid grid-cols-3 gap-2 text-[10px] border-t border-border pt-2.5 mt-2.5">
        <div>
          <p className="text-muted">Avg Util</p>
          <p className="font-mono text-gray-300">{Math.round(avgUtil)}%</p>
        </div>
        <div>
          <p className="text-muted">Alloc</p>
          <p className="font-mono text-gray-300">{node.allocated_gpus}/{node.gpu_count} GPUs</p>
        </div>
        <div>
          <p className="text-muted">Seen</p>
          <p className="font-mono text-gray-300">{formatRelative(node.last_seen)}</p>
        </div>
      </div>
    </div>
  )
}

// ─── Node Drilldown Panel ────────────────────────────────────────────────────

function NodeDrilldown({ node, onClose }: { node: Node; onClose: () => void }) {
  const now = new Date()
  const from = new Date(now.getTime() - 60 * 60 * 1000).toISOString()

  const seriesQuery = useQuery(
    (s) => api.nodes.gpuSeries(node.id, from, now.toISOString(), s),
    ['gpu-series', node.id],
    { pollMs: 10_000 }
  )

  // Flatten series for Recharts: each point has timestamps + all GPU values
  const chartData = (() => {
    if (seriesQuery.status !== 'success') return []
    const allSeries: GPUTimeSeries[] = seriesQuery.data
    if (allSeries.length === 0) return []

    const byTime: Record<string, Record<string, number>> = {}
    allSeries.forEach((ts) => {
      ts.points.forEach((p) => {
        const key = format(new Date(p.timestamp), 'HH:mm:ss')
        if (!byTime[key]) byTime[key] = { time: key as unknown as number }
        byTime[key][`GPU ${ts.gpu_index}`] = Math.round(p.utilization_pct)
      })
    })
    return Object.values(byTime).sort((a, b) =>
      String(a.time).localeCompare(String(b.time))
    )
  })()

  const gpuColors = ['#58a6ff', '#3fb950', '#d29922', '#f85149', '#a371f7', '#39d353', '#ff7b72', '#ffa657']

  return (
    <div className="fixed inset-0 z-50 flex items-stretch">
      <div className="flex-1 bg-black/50" onClick={onClose} />
      <div className="w-[640px] bg-surface-1 border-l border-border overflow-y-auto animate-fade-in">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-border p-4 sticky top-0 bg-surface-1 z-10">
          <div className="flex items-center gap-2">
            <Server className="h-4 w-4 text-accent" />
            <span className="font-mono text-sm font-semibold text-gray-200">
              {node.hostname.split('.')[0]}
            </span>
            <Badge className={cn(nodeStatusBg(node.status))}>{node.status}</Badge>
          </div>
          <button onClick={onClose} className="text-muted hover:text-gray-200">
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="p-4 space-y-5">
          {/* Node metadata */}
          <div className="grid grid-cols-3 gap-3 text-xs">
            {[
              { label: 'GPU Model', value: node.gpu_model },
              { label: 'GPU Count', value: `${node.gpu_count} × H100` },
              { label: 'Allocated', value: `${node.allocated_gpus}/${node.gpu_count}` },
              { label: 'CPU Cores', value: node.cpu_cores },
              { label: 'Memory', value: formatBytes(node.memory_gb) },
              { label: 'Last Seen', value: formatRelative(node.last_seen) },
            ].map(({ label, value }) => (
              <div key={label} className="rounded-md border border-border bg-surface-2 p-2.5">
                <p className="text-muted mb-0.5">{label}</p>
                <p className="font-mono font-medium text-gray-200 truncate">{String(value)}</p>
              </div>
            ))}
          </div>

          {/* Per-GPU current state */}
          <div>
            <h3 className="text-xs font-semibold text-muted uppercase tracking-wider mb-2">
              Per-GPU State (current)
            </h3>
            <div className="space-y-2">
              {(node.gpu_utilization ?? []).map((util, i) => (
                <div key={i} className="rounded-md border border-border bg-surface-2 p-2.5">
                  <div className="flex items-center justify-between mb-1.5">
                    <span className="text-xs font-mono text-gray-300">GPU {i}</span>
                    <div className="flex items-center gap-3 text-[10px] text-muted">
                      <span className="flex items-center gap-1">
                        <Thermometer className="h-3 w-3" />
                        {Math.round(node.gpu_temperature_c?.[i] ?? 0)}°C
                      </span>
                      <span className="flex items-center gap-1">
                        <MemoryStick className="h-3 w-3" />
                        {(node.gpu_memory_used_gb?.[i] ?? 0).toFixed(1)}/{(node.gpu_memory_total_gb?.[i] ?? 80).toFixed(0)} GB
                      </span>
                      <span className="flex items-center gap-1">
                        <Zap className="h-3 w-3" />
                        {Math.round(node.gpu_power_watts?.[i] ?? 0)}W
                      </span>
                    </div>
                  </div>
                  <GPUBar utilization={util} height="h-2" showValue />
                </div>
              ))}
            </div>
          </div>

          {/* Sparklines */}
          <div>
            <h3 className="text-xs font-semibold text-muted uppercase tracking-wider mb-2">
              GPU Utilization — Last Hour
            </h3>
            {seriesQuery.status === 'loading' && (
              <p className="text-xs text-muted">Loading time series…</p>
            )}
            {chartData.length > 0 && (
              <ResponsiveContainer width="100%" height={180}>
                <LineChart data={chartData} margin={{ top: 4, right: 4, bottom: 0, left: -20 }}>
                  <CartesianGrid strokeDasharray="3 3" stroke="#373e47" />
                  <XAxis dataKey="time" tick={{ fontSize: 9, fill: '#768390' }} interval="preserveStartEnd" />
                  <YAxis domain={[0, 100]} tick={{ fontSize: 9, fill: '#768390' }} />
                  <Tooltip
                    contentStyle={{ background: '#1c2128', border: '1px solid #373e47', borderRadius: 6, fontSize: 11 }}
                  />
                  <Legend wrapperStyle={{ fontSize: 10 }} />
                  {Array.from({ length: node.gpu_count }, (_, i) => (
                    <Line
                      key={i}
                      type="monotone"
                      dataKey={`GPU ${i}`}
                      stroke={gpuColors[i % gpuColors.length]}
                      dot={false}
                      strokeWidth={1.5}
                    />
                  ))}
                </LineChart>
              </ResponsiveContainer>
            )}
          </div>

          {/* Labels */}
          {Object.keys(node.labels ?? {}).length > 0 && (
            <div>
              <h3 className="text-xs font-semibold text-muted uppercase tracking-wider mb-2">Labels</h3>
              <div className="flex flex-wrap gap-1.5">
                {Object.entries(node.labels).map(([k, v]) => (
                  <Badge key={k} className="border-border text-muted font-mono text-[10px]">
                    {k}={v}
                  </Badge>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

// ─── Page ────────────────────────────────────────────────────────────────────

export function Nodes() {
  const [selected, setSelected] = useState<Node | null>(null)
  const query = useQuery((s) => api.nodes.list(s), 'nodes', { pollMs: 5_000 })

  if (query.status === 'loading') return <PageSpinner />
  if (query.status === 'error') return (
    <div className="p-6">
      <p className="text-danger text-sm">{query.error}</p>
    </div>
  )

  const nodes = query.data

  const byStatus = {
    healthy:     nodes.filter(n => n.status === 'healthy'),
    degraded:    nodes.filter(n => n.status === 'degraded'),
    unavailable: nodes.filter(n => n.status === 'unavailable'),
  }

  return (
    <div className="p-6 space-y-6">
      <div>
        <h1 className="text-xl font-bold text-gray-100">Nodes</h1>
        <p className="text-xs text-muted mt-0.5">
          {nodes.length} nodes · {byStatus.healthy.length} healthy ·{' '}
          {byStatus.degraded.length} degraded · {byStatus.unavailable.length} unavailable
        </p>
      </div>

      {nodes.length === 0 ? (
        <EmptyState icon={<Server className="h-8 w-8" />} title="No nodes found" description="The simulator may not be running yet." />
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5">
          {nodes.map(n => (
            <NodeCard key={n.id} node={n} onClick={() => setSelected(n)} />
          ))}
        </div>
      )}

      {selected && (
        <NodeDrilldown node={selected} onClose={() => setSelected(null)} />
      )}
    </div>
  )
}
