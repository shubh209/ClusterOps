import { cn } from '@/lib/utils'
import type { ReactNode } from 'react'

interface StatCardProps {
  label: string
  value: ReactNode
  sub?: ReactNode
  icon?: ReactNode
  valueClassName?: string
  trend?: 'up' | 'down' | 'neutral'
}

export function StatCard({ label, value, sub, icon, valueClassName }: StatCardProps) {
  return (
    <div className="rounded-lg border border-border bg-surface-1 p-4">
      <div className="flex items-start justify-between">
        <div>
          <p className="text-xs font-medium text-muted uppercase tracking-wider mb-1">{label}</p>
          <p className={cn('text-2xl font-bold font-mono', valueClassName ?? 'text-gray-100')}>
            {value}
          </p>
          {sub && <p className="mt-1 text-xs text-muted">{sub}</p>}
        </div>
        {icon && (
          <div className="text-muted opacity-60">{icon}</div>
        )}
      </div>
    </div>
  )
}
