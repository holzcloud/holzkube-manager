import { useQuery } from '@tanstack/react-query'
import { createRoute, Link } from '@tanstack/react-router'
import { ArrowRight } from 'lucide-react'
import { api } from '@/api'
import { DataTable } from '@/components/DataTable'
import { HealthField, StageBadge } from '@/components/HealthField'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useSystemStatus } from '@/hooks/useSession'
import { cn } from '@/lib/utils'
import { authenticatedRoute } from '@/routes/__root'

/**
 * The fleet overview (D-29), in the 2026 shell (docs/mockups/webui-2026.html).
 *
 * One card per fact an operator opens the dashboard for -- the fleet, the
 * machines, the instance's own record-keeping -- with the number large and
 * the detail one line under it, the way the mockup draws a card. What the
 * mockup fills with sparklines is left out here: this page has no series
 * endpoint for these numbers, and a line drawn from three points is
 * decoration pretending to be data.
 *
 * The instance status stays in a supporting role and deliberately does not
 * disappear. The audit chain-break warning is the one thing on this page that
 * says holzkube-manager's own record-keeping cannot be trusted, and a page
 * that dropped it while gaining cluster tiles would have traded the more
 * important fact for the more interesting one (D-15 from phase 1).
 */

/** One metric card: the label, the big number, one line of detail. */
function MetricCard({
  title,
  badge,
  value,
  unit,
  detail,
  href,
  state = 'default',
}: {
  title: string
  badge?: string
  value: string
  unit?: string
  detail: string
  href: string
  state?: 'default' | 'warn' | 'danger'
}) {
  return (
    <Link
      to={href}
      className="group flex flex-col rounded-2xl border border-border bg-card p-5 transition-colors hover:border-primary/45 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
    >
      <div className="flex items-center gap-2">
        <h3 className="text-sm font-semibold">{title}</h3>
        {badge && (
          <Badge
            variant="outline"
            className={cn(
              'ml-auto',
              state === 'warn' && 'border-amber-600/40 text-amber-700 dark:text-amber-300',
              state === 'danger' && 'border-red-600/40 text-red-700 dark:text-red-300',
            )}
          >
            {badge}
          </Badge>
        )}
      </div>
      <p className="mt-3 font-heading text-4xl font-bold tracking-tight tabular-nums">
        {value}
        {unit && <span className="ml-1.5 text-sm font-semibold text-muted-foreground">{unit}</span>}
      </p>
      <p className="mt-1 text-xs break-words text-muted-foreground">{detail}</p>
    </Link>
  )
}

/** A titled panel that fills the height it is given and scrolls inside itself. */
function Panel({
  title,
  hint,
  to,
  toLabel,
  className,
  children,
}: {
  title: string
  hint: string
  to: string
  toLabel: string
  className?: string
  children: React.ReactNode
}) {
  return (
    <section
      className={cn(
        'flex min-h-0 flex-col rounded-2xl border border-border bg-card p-5',
        className,
      )}
    >
      <div className="flex items-baseline justify-between gap-2">
        <div>
          <h2 className="font-heading text-base font-semibold">{title}</h2>
          <p className="text-xs text-muted-foreground">{hint}</p>
        </div>
        <Button asChild variant="ghost" size="sm" className="text-muted-foreground">
          <Link to={to}>
            {toLabel}
            <ArrowRight aria-hidden="true" className="size-4" />
          </Link>
        </Button>
      </div>
      <div className="mt-4 min-h-0 flex-1 overflow-auto">{children}</div>
    </section>
  )
}

function Dashboard() {
  const status = useSystemStatus()
  const clusters = useQuery({
    queryKey: ['clusters'],
    queryFn: () => api.clusters.list(),
    refetchInterval: 30_000,
  })
  const machines = useQuery({
    queryKey: ['machines'],
    queryFn: () => api.machines.list(),
    refetchInterval: 30_000,
  })
  const recent = useQuery({
    queryKey: ['audit', 'recent'],
    queryFn: () => api.audit({ limit: 8 }),
  })

  const fleet = machines.data ?? []
  const unassigned = fleet.filter((m) => m.cluster === '')
  const fleetByCondition = (condition: string) => fleet.filter((m) => m.stage === condition).length

  const up = fleet.length - fleetByCondition('down')
  const down = fleetByCondition('down')

  return (
    <div className="flex min-h-0 flex-col gap-6 lg:h-full">
      <div>
        <h1 className="font-heading text-2xl font-semibold tracking-tight">Dashboard</h1>
        <p className="text-sm text-muted-foreground">The fleet, as holzkube-manager last saw it.</p>
      </div>

      {clusters.isSuccess && clusters.data.length === 0 && (
        <div className="max-w-2xl rounded-2xl border border-border bg-card p-6">
          <h2 className="font-heading text-base font-semibold">No clusters yet</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            Import the cluster you already run. holzkube-manager reads a control-plane node's own
            configuration and fills the inventory from the cluster's membership.
          </p>
          <Button asChild className="mt-4">
            <Link to="/clusters">Import a cluster</Link>
          </Button>
        </div>
      )}

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
        <MetricCard
          title="Clusters"
          value={clusters.isPending ? '…' : String(clusters.data?.length ?? 0)}
          detail={
            clusters.isSuccess && clusters.data.length > 0
              ? clusters.data.map((c) => c.name).join(' · ')
              : 'None imported yet.'
          }
          href="/clusters"
        />
        <MetricCard
          title="Machines"
          badge={down > 0 ? `${down} down` : fleet.length > 0 ? 'all up' : undefined}
          state={down > 0 ? 'danger' : 'default'}
          value={machines.isPending ? '…' : String(fleet.length)}
          unit="known"
          detail={`${up} answering · ${unassigned.length} in no cluster`}
          href="/nodes"
        />
        <MetricCard
          title="Instance"
          badge={
            status.data ? (status.data.audit_chain.ok ? 'verified' : 'chain broken') : undefined
          }
          state={status.data && !status.data.audit_chain.ok ? 'danger' : 'default'}
          value={status.isPending ? '…' : status.data?.setup_required ? 'setup' : 'ready'}
          detail="holzkube-manager's own process and audit chain."
          href="/settings"
        />
      </div>

      {/* The rest of the screen: the machines take two thirds and the
          clusters and the latest records share the third. Each panel scrolls
          inside itself, so a long fleet never pushes the others off screen. */}
      <div className="grid min-h-0 flex-1 gap-4 lg:grid-cols-3">
        <Panel
          title="Machines"
          hint="Every machine this instance knows, and how it answered last."
          to="/nodes"
          toLabel="All nodes"
          className="lg:col-span-2"
        >
          {machines.isPending && <Skeleton className="h-24 w-full" />}
          {machines.isSuccess && (
            <DataTable
              label="Machines"
              phone="rows"
              rows={fleet}
              keyOf={(m) => m.id}
              empty="No machines yet."
              columns={[
                {
                  key: 'host',
                  label: 'Host',
                  role: 'identity',
                  className: 'font-medium',
                  render: (m) => (
                    <Link
                      to="/nodes/$uuid"
                      params={{ uuid: m.id }}
                      className="inline-flex items-center hover:underline max-md:min-h-11"
                    >
                      <HealthField field={m.hostname} render={(v) => v || m.id.slice(0, 8)} />
                    </Link>
                  ),
                },
                {
                  key: 'state',
                  label: 'State',
                  render: (m) => <StageBadge stage={m.stage} />,
                },
                { key: 'role', label: 'Role', render: (m) => m.role || '—' },
                {
                  key: 'addr',
                  label: 'Address',
                  className: 'font-mono text-xs',
                  render: (m) => <HealthField field={m.addr} />,
                },
                {
                  key: 'talos',
                  label: 'Talos',
                  className: 'font-mono text-xs',
                  render: (m) => <HealthField field={m.talos_version} />,
                },
              ]}
            />
          )}
        </Panel>

        <div className="flex min-h-0 flex-col gap-4">
          <Panel
            title="Clusters"
            hint="Nodes by condition, and how long the client certificate lasts."
            to="/clusters"
            toLabel="Clusters"
          >
            {clusters.isSuccess && clusters.data.length === 0 && (
              <p className="text-sm text-muted-foreground">None imported yet.</p>
            )}
            <ul className="space-y-3">
              {(clusters.data ?? []).map((c) => (
                <li key={c.id} className="rounded-xl border border-border p-3">
                  <div className="flex items-baseline justify-between gap-2">
                    <span className="font-medium">{c.name}</span>
                    <span className="text-xs text-muted-foreground tabular-nums">
                      {c.nodes} {c.nodes === 1 ? 'node' : 'nodes'} · {c.control_plane} cp ·{' '}
                      {c.workers} {c.workers === 1 ? 'worker' : 'workers'}
                    </span>
                  </div>
                  <p className="mt-1 text-xs text-muted-foreground tabular-nums">
                    {c.healthy} healthy · {c.degraded} degraded · {c.down} not answering ·
                    certificate {c.client_cert_days_left} days left
                  </p>
                </li>
              ))}
            </ul>
          </Panel>

          <Panel
            title="Recent activity"
            hint="The latest audit records, newest first."
            to="/audit"
            toLabel="Audit log"
            className="flex-1"
          >
            {recent.isPending && <Skeleton className="h-16 w-full" />}
            {recent.isSuccess && recent.data.items.length === 0 && (
              <p className="text-sm text-muted-foreground">Nothing has been recorded yet.</p>
            )}
            {recent.isSuccess && recent.data.items.length > 0 && (
              <ul className="divide-y divide-border">
                {recent.data.items.map((record) => (
                  <li key={record.seq} className="py-2">
                    <p className="truncate font-mono text-xs">{record.action}</p>
                    <p className="text-xs text-muted-foreground tabular-nums">
                      {record.ts} · {record.actor === '' ? '—' : record.actor}
                    </p>
                  </li>
                ))}
              </ul>
            )}
          </Panel>
        </div>
      </div>
    </div>
  )
}

export const indexRoute = createRoute({
  getParentRoute: () => authenticatedRoute,
  path: '/',
  component: Dashboard,
})
