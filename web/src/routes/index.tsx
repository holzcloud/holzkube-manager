import { useQuery } from '@tanstack/react-query'
import { createRoute, Link } from '@tanstack/react-router'
import { ArrowRight } from 'lucide-react'
import { api } from '@/api'
import { DataTable } from '@/components/DataTable'
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
    <Button
      asChild
      variant="ghost"
      className="group h-auto w-full justify-start rounded-2xl border border-border bg-card p-5 text-left hover:border-primary/45 hover:bg-card"
    >
      <Link to={href} className="block">
        <div className="flex w-full items-center gap-2">
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
          {unit && (
            <span className="ml-1.5 text-sm font-semibold text-muted-foreground">{unit}</span>
          )}
        </p>
        <p className="mt-1 text-xs text-muted-foreground">{detail}</p>
        <ArrowRight
          aria-hidden="true"
          className="mt-3 size-4 text-muted-foreground transition-transform group-hover:translate-x-0.5"
        />
      </Link>
    </Button>
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
    queryFn: () => api.audit({ limit: 3 }),
  })

  const fleet = machines.data ?? []
  const unassigned = fleet.filter((m) => m.cluster === '')
  const fleetByCondition = (condition: string) => fleet.filter((m) => m.stage === condition).length

  const up = fleet.length - fleetByCondition('down')
  const down = fleetByCondition('down')

  return (
    <div className="space-y-6">
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

      <div className="max-w-3xl rounded-2xl border border-border bg-card p-5">
        <div className="flex items-baseline justify-between gap-2">
          <div>
            <h2 className="font-heading text-base font-semibold">Recent activity</h2>
            <p className="text-xs text-muted-foreground">
              The three most recent audit records, newest first.
            </p>
          </div>
          <Button asChild variant="ghost" size="sm" className="text-muted-foreground">
            <Link to="/audit">
              Audit log
              <ArrowRight aria-hidden="true" className="size-4" />
            </Link>
          </Button>
        </div>
        {recent.isPending && <Skeleton className="mt-4 h-16 w-full" />}
        {recent.isSuccess && recent.data.items.length === 0 && (
          <p className="mt-4 text-sm text-muted-foreground">Nothing has been recorded yet.</p>
        )}
        {recent.isSuccess && recent.data.items.length > 0 && (
          <div className="mt-4">
            <DataTable
              label="Recent activity"
              phone="rows"
              rows={recent.data.items.slice(0, 3)}
              keyOf={(record) => String(record.seq)}
              empty="Nothing has been recorded yet."
              columns={[
                {
                  key: 'action',
                  label: 'Action',
                  role: 'identity',
                  render: (record) => <span className="font-mono text-xs">{record.action}</span>,
                },
                {
                  key: 'ts',
                  label: 'Time',
                  className: 'tabular-nums',
                  render: (record) => <span className="tabular-nums">{record.ts}</span>,
                },
                {
                  key: 'actor',
                  label: 'Actor',
                  render: (record) => (record.actor === '' ? '—' : record.actor),
                },
              ]}
            />
          </div>
        )}
      </div>
    </div>
  )
}

export const indexRoute = createRoute({
  getParentRoute: () => authenticatedRoute,
  path: '/',
  component: Dashboard,
})
