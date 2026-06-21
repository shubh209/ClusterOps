import { useState, useCallback } from 'react'
import { Bell, CheckCircle } from 'lucide-react'
import { useSSE } from '@/hooks/useSSE'
import { useQuery } from '@/hooks/useQuery'
import { api } from '@/lib/api'
import { Badge } from '@/components/shared/Badge'
import { EmptyState } from '@/components/shared/EmptyState'
import { PageSpinner } from '@/components/shared/Spinner'
import { SeverityDot } from '@/components/shared/AlertBadge'
import { cn, alertSeverityBg, formatRelative } from '@/lib/utils'
import type { Alert } from '@/types'

const SEVERITY_ORDER = { critical: 0, warning: 1, info: 2 }

export function Alerts() {
  const [activeOnly, setActiveOnly] = useState(true)
  const [liveAlerts, setLiveAlerts] = useState<Alert[] | null>(null)

  const query = useQuery(
    (s) => api.alerts.list(activeOnly, s),
    ['alerts', String(activeOnly)],
    { pollMs: 5_000 }
  )

  useSSE({
    alerts: useCallback((d) => setLiveAlerts(d as Alert[]), []),
  })

  const raw = liveAlerts ?? (query.status === 'success' ? query.data : null)
  const alerts = raw
    ? [...raw]
        .filter(a => activeOnly ? !a.resolved : true)
        .sort((a, b) => {
          const sd = SEVERITY_ORDER[a.severity] - SEVERITY_ORDER[b.severity]
          if (sd !== 0) return sd
          return new Date(b.triggered_at).getTime() - new Date(a.triggered_at).getTime()
        })
    : null

  if (!alerts && query.status === 'loading') return <PageSpinner />

  const critical = alerts?.filter(a => a.severity === 'critical').length ?? 0
  const warning  = alerts?.filter(a => a.severity === 'warning').length ?? 0

  return (
    <div className="p-6 space-y-5">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-bold text-gray-100">Alerts</h1>
          <p className="text-xs text-muted mt-0.5">
            {critical > 0 && <span className="text-danger">{critical} critical </span>}
            {warning > 0 && <span className="text-warning">{warning} warning </span>}
            {(alerts?.length ?? 0)} total
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setActiveOnly(v => !v)}
            className={cn(
              'rounded-md border px-3 py-1.5 text-xs transition-colors',
              activeOnly
                ? 'border-accent bg-accent/10 text-accent'
                : 'border-border text-muted hover:text-gray-200'
            )}
          >
            {activeOnly ? 'Active only' : 'All alerts'}
          </button>
        </div>
      </div>

      {!alerts || alerts.length === 0 ? (
        <EmptyState
          icon={<CheckCircle className="h-8 w-8 text-success" />}
          title="No alerts"
          description={activeOnly ? 'All systems nominal.' : 'No historical alerts found.'}
        />
      ) : (
        <div className="space-y-2">
          {alerts.map(a => (
            <div
              key={a.id}
              className={cn(
                'rounded-lg border p-4 transition-colors',
                a.severity === 'critical' ? 'border-danger/30 bg-danger/5' :
                a.severity === 'warning'  ? 'border-warning/30 bg-warning/5' :
                                            'border-border bg-surface-1'
              )}
            >
              <div className="flex items-start gap-3">
                <div className="mt-0.5">
                  <SeverityDot severity={a.severity} />
                </div>
                <div className="flex-1 min-w-0">
                  <div className="flex items-start justify-between gap-2">
                    <p className="text-sm font-semibold text-gray-200">{a.title}</p>
                    <div className="flex items-center gap-2 shrink-0">
                      <Badge className={cn(alertSeverityBg(a.severity))}>
                        {a.severity.toUpperCase()}
                      </Badge>
                      {a.resolved && (
                        <Badge className="border-success/30 bg-success/10 text-success">RESOLVED</Badge>
                      )}
                    </div>
                  </div>
                  <p className="text-xs text-muted mt-1 leading-relaxed">{a.message}</p>
                  <div className="flex items-center gap-4 mt-2 text-[10px] text-muted">
                    <span>{formatRelative(a.triggered_at)}</span>
                    {a.node_id && <span>Node: <span className="font-mono text-gray-400">{a.node_id}</span></span>}
                    {a.job_id  && <span>Job: <span className="font-mono text-gray-400">{a.job_id}</span></span>}
                    <span className="font-mono opacity-50">{a.type}</span>
                  </div>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
