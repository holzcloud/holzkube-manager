import { useQuery } from '@tanstack/react-query'
import { Link, useRouterState } from '@tanstack/react-router'
import {
  ArrowUpCircle,
  Boxes,
  Container,
  Cpu,
  Disc3,
  Ellipsis,
  FileCog,
  HardDriveDownload,
  LayoutDashboard,
  ListChecks,
  type LucideIcon,
  MonitorDot,
  ScrollText,
  Server,
  Settings,
} from 'lucide-react'
import { api, type Host } from '@/api'
import { AlphaBadge, AlphaNotice } from '@/components/Alpha'
import { HostStateMark } from '@/components/HostState'
import { SourceNotice } from '@/components/SourceNotice'
import { WhatsNew } from '@/components/WhatsNew'
import { cn } from '@/lib/utils'

/**
 * Every area this product has, as one list.
 *
 * The 2026 redesign splits it in two: the five areas an operator touches
 * during an incident (PRIMARY_NAV, the mockup's choice) are on the rail and
 * the phone's bottom bar; the rest (MORE_NAV) is behind the "More" button
 * -- the rail's ellipsis and the phone's menu -- which opens the drawer that
 * carries the full list, every area, with labels.
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
    path: '/wall',
    label: 'Wall',
    icon: MonitorDot,
    phase: null,
    description:
      'One screen for the IT office: every node and every workload as a tile, how full the cluster is, and the most recent warnings. It says how old its answer is and goes visibly stale rather than leaving a confident green screen up during an outage.',
  },
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
    phase: null,
    description:
      'Every machine holzkube-manager knows about, with its Talos version, its role and an honest health state — including the machines that are not answering.',
  },
  {
    path: '/host',
    label: 'Host',
    icon: Cpu,
    phase: null,
    description:
      'The machine holzkube-manager itself runs on: what it is, how the service is doing, and its live readings — processor, memory, filesystems, temperatures and network.',
  },
  {
    path: '/clusters',
    label: 'Clusters',
    icon: Boxes,
    phase: null,
    description:
      'Imported clusters, their control planes, their etcd members and the certificate expiry dates that decide whether any of it still works next month.',
  },
  {
    path: '/kubernetes',
    label: 'Kubernetes',
    icon: Container,
    phase: null,
    description:
      'What the cluster’s own API server says: its nodes as Kubernetes sees them, its pods, and why a workload is not running. It is allowed to disagree with the Nodes screen — that one reads the machine API.',
  },
  {
    path: '/config',
    label: 'Config',
    icon: FileCog,
    phase: null,
    description:
      'Machine configuration: view it with secrets redacted on the server, diff it, patch it, and see which apply mode a change actually needs.',
  },
  {
    path: '/jobs',
    label: 'Jobs',
    icon: ListChecks,
    phase: null,
    description:
      'Long-running and dangerous operations as persisted jobs that survive a restart of holzkube-manager itself.',
  },
  {
    path: '/provision',
    label: 'Provision',
    icon: HardDriveDownload,
    phase: null,
    description:
      'A blank machine becomes a cluster node: find it, confirm it is the one you mean, choose the disk, and watch it come back.',
  },
  {
    path: '/upgrades',
    label: 'Upgrades',
    icon: ArrowUpCircle,
    phase: null,
    description:
      'Rolling Talos and Kubernetes upgrades behind a health gate that would rather refuse than strand a cluster.',
  },
  {
    path: '/images',
    label: 'Images',
    icon: Disc3,
    phase: null,
    description:
      'Image Factory schematics: the system extensions, kernel arguments and META values a machine boots with, and the exact ISO, PXE and installer references they produce.',
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
    phase: null,
    description:
      'Backup and restore of the data directory, the supported Talos version range, and the rest of the operational settings.',
  },
]

const byPath = (path: string): NavArea => {
  const area = NAV_AREAS.find((a) => a.path === path)
  if (area === undefined) throw new Error(`No navigation area named ${path}`)
  return area
}

/** The five areas the rail and the phone's tab bar carry (the mockup's choice). */
export const PRIMARY_NAV: NavArea[] = ['/', '/clusters', '/host', '/jobs', '/settings'].map(byPath)

/** Everything else, behind "More". */
export const MORE_NAV: NavArea[] = NAV_AREAS.filter((area) => !PRIMARY_NAV.includes(area))

/**
 * The host reading behind the Host mark, shared by the rail, the drawer and
 * nothing else. On /host the page's own 3-s poll feeds the mark and this asks
 * nothing of its own; elsewhere a 30-s refresh, no retry: a failed poll is
 * shown as one, at once (T-12-14, RESEARCH Pattern 8).
 */
export function useHostMark() {
  const onHostPage = useRouterState({ select: (s) => s.location.pathname === '/host' })
  return useQuery({
    queryKey: ['host'],
    queryFn: api.host,
    retry: false,
    refetchInterval: onHostPage ? false : 30_000,
  })
}

function HostMark({ host }: { host: { data?: Host; error: unknown } }) {
  if (host.error) {
    return (
      <HostStateMark
        state="unanswered"
        summary="holzkube-manager did not answer the latest request."
        form="sidebar"
      />
    )
  }
  if (host.data === undefined) return null
  return (
    <HostStateMark
      state={host.data.health.state}
      summary={host.data.health.summary}
      form="sidebar"
    />
  )
}

const isActive = (pathname: string, path: string) =>
  path === '/' ? pathname === '/' : pathname === path || pathname.startsWith(`${path}/`)

/**
 * The rail (md up to lg): the brand mark, one icon button per primary area, and
 * the ellipsis that opens the drawer with everything else. Tooltips come from
 * the title attribute; a hover CSS popover would be a second copy of what the
 * accessible name already says.
 */
export function RailNav({ onOpenMore, moreOpen }: { onOpenMore?: () => void; moreOpen?: boolean }) {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const host = useHostMark()
  return (
    <nav
      aria-label="Primary navigation"
      className="hidden h-full w-20 shrink-0 flex-col items-center gap-1 border-r border-sidebar-border bg-sidebar px-2 py-3 md:flex lg:hidden"
    >
      <Link
        to="/"
        aria-label="holzkube-manager — Dashboard"
        className="mb-3 flex size-11 shrink-0 items-center justify-center rounded-xl focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
      >
        <img src="/favicon.svg" alt="" className="size-9" />
      </Link>
      {PRIMARY_NAV.map((area) => {
        const active = isActive(pathname, area.path)
        return (
          <Link
            key={area.path}
            to={area.path}
            activeOptions={{ exact: area.path === '/' }}
            title={area.label}
            aria-current={active ? 'page' : undefined}
            className={cn(
              'flex size-12 shrink-0 flex-col items-center justify-center gap-1 rounded-xl border border-transparent transition-colors',
              'focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring',
              active
                ? 'border-primary/40 bg-sidebar-accent font-semibold text-sidebar-accent-foreground'
                : 'text-sidebar-foreground/70 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground',
            )}
          >
            <area.icon aria-hidden="true" className="size-5" />
            {area.path === '/host' && <HostMark host={host} />}
          </Link>
        )
      })}
      <div className="mt-auto flex flex-col items-center gap-1.5">
        <AlphaBadge />
        <button
          type="button"
          aria-label="More navigation"
          aria-expanded={moreOpen ?? false}
          onClick={onOpenMore}
          className={cn(
            'flex size-12 shrink-0 items-center justify-center rounded-xl border border-transparent transition-colors',
            'focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring',
            moreOpen
              ? 'border-primary/40 bg-sidebar-accent font-semibold text-sidebar-accent-foreground'
              : 'text-sidebar-foreground/70 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground',
          )}
        >
          <Ellipsis aria-hidden="true" className="size-5" />
        </button>
      </div>
    </nav>
  )
}

/**
 * The bottom tab bar (below md): the five primary areas with icon and label,
 * thumb-sized targets (MOB-01), the same active cue the rail uses.
 */
export function BottomNav() {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  return (
    <nav
      aria-label="Bottom navigation"
      className={cn(
        'fixed inset-x-0 bottom-0 z-30 border-t border-sidebar-border bg-sidebar/95 backdrop-blur',
        'pb-[env(safe-area-inset-bottom)] md:hidden',
      )}
    >
      <div className="mx-auto grid max-w-md grid-cols-5">
        {PRIMARY_NAV.map((area) => {
          const active = isActive(pathname, area.path)
          return (
            <Link
              key={area.path}
              to={area.path}
              activeOptions={{ exact: area.path === '/' }}
              aria-current={active ? 'page' : undefined}
              className={cn(
                'flex min-h-14 flex-1 flex-col items-center justify-center gap-0.5 rounded-lg px-1 text-[10px] font-medium',
                'focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring',
                active
                  ? 'text-sidebar-accent-foreground'
                  : 'text-sidebar-foreground/70 hover:text-foreground',
              )}
            >
              <area.icon aria-hidden="true" className="size-5" />
              <span className="w-full truncate text-center">{area.label}</span>
            </Link>
          )
        })}
      </div>
    </nav>
  )
}

/**
 * The drawer: the full list, every area, with labels. Below md it slides in
 * over the page; on md and up it grows out of the rail as a second column,
 * so the "More" areas stay one tap away on a desk too.
 */
export function NavDrawer({
  open = false,
  onNavigate,
}: {
  open?: boolean
  onNavigate?: () => void
}) {
  const host = useHostMark()
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  return (
    <nav
      aria-label="Main navigation"
      className={cn(
        'flex h-full shrink-0 flex-col gap-1 bg-sidebar',
                // Below md: out of the flow, over the page, off the left edge until asked for.
        'max-lg:fixed max-lg:inset-y-0 max-lg:left-0 max-lg:z-50 max-lg:w-72 max-lg:border-r max-lg:border-sidebar-border max-lg:p-3 max-lg:shadow-xl',
        'max-lg:overflow-y-auto max-lg:transition-transform max-lg:duration-200 max-lg:ease-out',
        open ? 'max-lg:translate-x-0 max-lg:visible' : 'max-lg:-translate-x-full max-lg:invisible',
        // From lg up: the permanent column, the desk's own list, alone. The audit's desk pass reads the version button without opening
        // anything, so the list is there rather than behind the ellipsis.
        'lg:sticky lg:top-0 lg:w-60 lg:border-r lg:border-sidebar-border lg:p-3',
      )}
    >
      <div className="mb-3 flex items-center gap-2.5 px-2 pt-1">
        <img src="/favicon.svg" alt="" className="size-8 shrink-0" />
        <div>
          <span className="font-heading text-base font-semibold tracking-tight">
            holzkube-manager
          </span>
          <p className="text-xs text-muted-foreground">Talos cluster management</p>
        </div>
      </div>
      {NAV_AREAS.map((area) => (
        <Link
          key={area.path}
          to={area.path}
          activeOptions={{ exact: area.path === '/' }}
          onClick={onNavigate}
          className={cn(
            'flex items-center gap-2 rounded-lg border-l-2 border-transparent py-1.5 pr-2 pl-2 text-sm',
            // A thumb-sized row on a phone (MOB-01).
            'max-md:min-h-11',
            'focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring',
            // The active styles ride on the state this side computes, not on
            // activeProps: that is concatenated onto className without
            // tailwind-merge, so both colours survived and stylesheet order
            // decided (UAT G-01-5).
            isActive(pathname, area.path)
              ? 'border-sidebar-accent-foreground bg-sidebar-accent font-semibold text-sidebar-accent-foreground'
              : 'text-sidebar-foreground/70 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground',
          )}
        >
          <area.icon aria-hidden="true" className="size-4 shrink-0" />
          <span className="flex-1">{area.label}</span>
          {area.path === '/host' && <HostMark host={host} />}
        </Link>
      ))}
      <div className="mt-auto px-2 pt-3">
        <WhatsNew />
        <AlphaNotice className="mt-1 text-[11px] leading-snug text-sidebar-foreground/60" />
      </div>
      {/* AGPL section 13: the running instance has to offer its own source. */}
      <SourceNotice className="px-2 pt-1 text-xs text-sidebar-foreground/60" />
    </nav>
  )
}
