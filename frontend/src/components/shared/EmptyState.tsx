import type { ReactNode } from 'react'

interface EmptyStateProps {
  icon?: ReactNode
  title: string
  description?: string
}

export function EmptyState({ icon, title, description }: EmptyStateProps) {
  return (
    <div className="flex flex-col items-center justify-center py-16 text-center">
      {icon && <div className="mb-4 text-muted opacity-40">{icon}</div>}
      <p className="text-sm font-medium text-muted">{title}</p>
      {description && <p className="mt-1 text-xs text-muted opacity-70">{description}</p>}
    </div>
  )
}
