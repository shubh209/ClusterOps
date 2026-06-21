import { cn } from '@/lib/utils'

interface GPUBarProps {
  utilization: number   // 0–100
  label?: string
  height?: string
  showValue?: boolean
}

function utilizationColor(u: number): string {
  if (u >= 80) return 'bg-success'
  if (u >= 40) return 'bg-accent'
  if (u >= 10) return 'bg-warning'
  return 'bg-surface-4'
}

export function GPUBar({ utilization, label, height = 'h-2', showValue = false }: GPUBarProps) {
  const clamped = Math.min(100, Math.max(0, utilization))
  return (
    <div className="flex items-center gap-2 w-full">
      {label && <span className="text-xs text-muted font-mono w-8 shrink-0">{label}</span>}
      <div className={cn('flex-1 rounded-full bg-surface-4 overflow-hidden', height)}>
        <div
          className={cn('h-full rounded-full transition-all duration-700', utilizationColor(clamped))}
          style={{ width: `${clamped}%` }}
        />
      </div>
      {showValue && (
        <span className="text-xs font-mono text-muted w-8 text-right shrink-0">
          {Math.round(clamped)}%
        </span>
      )}
    </div>
  )
}

interface GPUGridProps {
  utilization: number[]
  nodeId: string
}

/** Renders an 8-cell GPU utilization heatmap for a single node. */
export function GPUHeatmapRow({ utilization, nodeId }: GPUGridProps) {
  return (
    <div className="flex gap-1">
      {utilization.map((u, i) => (
        <div
          key={`${nodeId}-gpu-${i}`}
          title={`GPU ${i}: ${Math.round(u)}%`}
          className={cn(
            'h-5 w-5 rounded-sm flex items-center justify-center text-[9px] font-mono font-bold',
            u >= 80 ? 'bg-success text-black' :
            u >= 40 ? 'bg-accent text-black' :
            u >= 10 ? 'bg-warning text-black' :
                      'bg-surface-4 text-muted'
          )}
        >
          {Math.round(u / 10)}
        </div>
      ))}
    </div>
  )
}
