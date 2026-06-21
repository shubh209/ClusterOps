import { cn, alertSeverityBg } from '@/lib/utils'
import { Badge } from './Badge'
import type { Alert } from '@/types'

export function AlertBadge({ alert }: { alert: Alert }) {
  return (
    <Badge className={cn(alertSeverityBg(alert.severity))}>
      {alert.severity.toUpperCase()}
    </Badge>
  )
}

export function SeverityDot({ severity }: { severity: Alert['severity'] }) {
  const color =
    severity === 'critical' ? 'bg-danger' :
    severity === 'warning'  ? 'bg-warning' : 'bg-accent'
  return <span className={cn('inline-block w-2 h-2 rounded-full', color)} />
}
