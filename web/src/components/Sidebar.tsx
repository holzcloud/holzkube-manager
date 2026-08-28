import { Link } from '@tanstack/react-router'
import {
  ArrowUpCircle,
  Boxes,
  FileCog,
  LayoutDashboard,
  ListChecks,
  type LucideIcon,
  ScrollText,
  Server,
  Settings,
} from 'lucide-react'
import { cn } from '@/lib/utils'

/**
 * The permanent navigation (D-10).
 *
 * Every area this product will ever have is listed here from phase 1 on. Later
 * phases replace a placeholder page with a real one; none of them has to touch
 * the navigation, which is exactly the point -- nobody rebuilds the shell under
 * time pressure in the middle of the inventory phase.
 *
 * `phase` is null for an area that exists now. Anything else names the phase
 * that builds it, and the placeholder page says so in plain English (D-09).
 */

export interface NavArea {
  path: string
  label: string
  icon: LucideIcon
  /** The phase that builds this area, or null when it already exists. */
  phase: number | null
  /** One honest sentence about what will live here. */
  description: string
}

export const NAV_AREAS: NavArea[] = [
  {
    path: '/',
    label: 'Dashboard',
    icon: LayoutDashboard,
    phase: null,
    description: 'Instance status and the most recent audit records.',
  },
  {
    path: '/nodes',
    label: 'Nodes',
    icon: Server,
    phase: 3,
    description:
      'Every machine holzkube knows about, with its Talos version, its role and an honest health state — including the machines that are not answering.',
  },
  {
    path: '/clusters',
    label: 'Clusters',
    icon: Boxes,
    phase: 3,
    description:
      'Imported clusters, their control planes, their etcd members and the certificate expiry dates that decide whether any of it still works next month.',
  },
  {
    path: '/config',
    label: 'Config',
    icon: FileCog,
    phase: 7,
    description:
      'Machine configuration: view it with secrets redacted on the server, diff it, patch it, and see which apply mode a change actually needs.',
  },
  {
    path: '/jobs',
    label: 'Jobs',
    icon: ListChecks,
    phase: 6,
    description:
      'Long-running and dangerous operations as persisted jobs that survive a restart of holzkube itself.',
  },
  {
    path: '/upgrades',
    label: 'Upgrades',
    icon: ArrowUpCircle,
    phase: 9,
    description:
      'Rolling Talos and Kubernetes upgrades behind a health gate that would rather refuse than strand a cluster.',
  },
  {
    path: '/audit',
    label: 'Audit',
    icon: ScrollText,
    phase: null,
    description: 'Every mutation, in order, with its hash chain.',
  },
  {
    path: '/settings',
    label: 'Settings',
    icon: Settings,
    phase: 10,
    description:
      'Backup and restore of the data directory, the supported Talos version range, and the rest of the operational settings.',
  },
]

export function Sidebar() {
  return (
    <nav
      aria-label="Main navigation"
      className="flex h-full w-56 shrink-0 flex-col gap-1 border-r border-border bg-sidebar p-3"
    >
      <div className="mb-4 px-2 pt-1">
        <span className="font-heading text-lg font-semibold tracking-tight">holzkube</span>
        <p className="text-xs text-muted-foreground">Talos cluster management</p>
      </div>

      {/* UAT G-01-5: the active pill alone was a 5/255 step against the sidebar
          and the active label was neither darker nor heavier than the rest, so
          nothing reliably said which page was open. Three cues now do: a darker
          pill, a semibold label, and a left bar that survives greyscale. */}
      {NAV_AREAS.map((area) => (
        <Link
          key={area.path}
          to={area.path}
          activeOptions={{ exact: area.path === '/' }}
          className={cn(
            'flex items-center gap-2 rounded-md border-l-2 border-transparent py-1.5 pr-2 pl-1.5 text-sm text-sidebar-foreground/70',
            'hover:bg-sidebar-accent hover:text-sidebar-accent-foreground',
            'focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring',
            // The active styles ride on the router's own data-status rather than
            // on activeProps: activeProps is concatenated onto className without
            // tailwind-merge, so `text-sidebar-accent-foreground` and the base
            // `text-sidebar-foreground/70` both survived and stylesheet order --
            // not intent -- decided the colour. A variant beats the bare
            // utility on specificity, so this cannot silently lose again.
            'data-[status=active]:border-sidebar-accent-foreground data-[status=active]:bg-sidebar-accent',
            'data-[status=active]:font-semibold data-[status=active]:text-sidebar-accent-foreground',
          )}
        >
          <area.icon aria-hidden="true" className="size-4 shrink-0" />
          <span className="flex-1">{area.label}</span>
          {area.phase !== null && (
            /* The chip fill and the active row's highlight were the same
               token, so the open page's badge lost its pill while every other
               kept it. The border gives the chip an edge of its own. */
            <span
              className="rounded border border-sidebar-border bg-muted px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground"
              title={`This area is built in phase ${area.phase}.`}
            >
              P{area.phase}
            </span>
          )}
        </Link>
      ))}
    </nav>
  )
}
