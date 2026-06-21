import { NavLink, Outlet } from 'react-router-dom'
import { cn } from '@/lib/utils'
import {
  LayoutDashboard,
  Server,
  Briefcase,
  Bell,
  BrainCircuit,
  Activity,
} from 'lucide-react'

const nav = [
  { to: '/',          label: 'Dashboard',  icon: LayoutDashboard },
  { to: '/nodes',     label: 'Nodes',      icon: Server },
  { to: '/jobs',      label: 'Jobs',       icon: Briefcase },
  { to: '/alerts',    label: 'Alerts',     icon: Bell },
  { to: '/assistant', label: 'Assistant',  icon: BrainCircuit },
]

export function Layout() {
  return (
    <div className="flex h-screen overflow-hidden bg-surface">
      {/* ── Sidebar ───────────────────────────────────────────────────── */}
      <aside className="flex w-56 shrink-0 flex-col border-r border-border bg-surface-1">
        {/* Logo */}
        <div className="flex items-center gap-2 border-b border-border px-4 py-4">
          <Activity className="h-5 w-5 text-accent" />
          <span className="text-sm font-bold tracking-tight text-gray-100">ClusterOps</span>
        </div>

        {/* Nav */}
        <nav className="flex-1 space-y-0.5 p-2 pt-3">
          {nav.map(({ to, label, icon: Icon }) => (
            <NavLink
              key={to}
              to={to}
              end={to === '/'}
              className={({ isActive }) =>
                cn(
                  'flex items-center gap-2.5 rounded-md px-3 py-2 text-sm transition-colors',
                  isActive
                    ? 'bg-accent/10 text-accent font-medium'
                    : 'text-muted hover:bg-surface-3 hover:text-gray-200'
                )
              }
            >
              <Icon className="h-4 w-4 shrink-0" />
              {label}
            </NavLink>
          ))}
        </nav>

        {/* Footer */}
        <div className="border-t border-border p-3">
          <p className="text-[10px] text-muted">AI Cluster Debug Console</p>
          <p className="text-[10px] text-muted opacity-60">v0.1.0</p>
        </div>
      </aside>

      {/* ── Main content ──────────────────────────────────────────────── */}
      <main className="flex-1 overflow-y-auto">
        <Outlet />
      </main>
    </div>
  )
}
