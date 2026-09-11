import { useQuery } from '@tanstack/react-query'
import { createRoute, Link } from '@tanstack/react-router'
import { api } from '@/api'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useSystemStatus } from '@/hooks/useSession'
import { authenticatedRoute } from '@/routes/__root'

/**
 * The fleet overview (D-29).
 *
 * Phase 3 turned this from an instance status card into the page an operator
 * opens during an incident: one tile per cluster, a fleet-wide count of nodes
 * by condition, and only then the instance's own status.
 *
 * The instance status moved into a supporting role and deliberately did not
 * disappear. The audit chain-break warning is the one thing on this page that
 * says holzkube-manager's own record-keeping cannot be trusted, and a page that
 * dropped it while gaining cluster tiles would have traded the more important
 * fact for the more interesting one (D-15 from phase 1).
 */
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

  const fleet = machines.data ?? []
  const unassigned = fleet.filter((m) => m.cluster === '')

  // The only real data flow phase 1 has, and therefore the proof that
  // store -> API -> UI works on records rather than on placeholders (D-13).
  const recent = useQuery({
    queryKey: ['audit', 'recent'],
    queryFn: () => api.audit({ limit: 3 }),
  })

  return (
    <div className="space-y-6">
      <div>
        <h1 className="font-heading text-2xl font-semibold tracking-tight">Dashboard</h1>
        <p className="text-sm text-muted-foreground">The fleet, as holzkube-manager last saw it.</p>
      </div>

      {clusters.isSuccess && clusters.data.length === 0 && (
        <Card className="max-w-2xl">
          <CardHeader>
            <CardTitle>No clusters yet</CardTitle>
            <CardDescription>
              Import the cluster you already run. holzkube-manager reads a control-plane node's own
              configuration and fills the inventory from the cluster's membership.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Button asChild>
              <Link to="/clusters">Import a cluster</Link>
            </Button>
          </CardContent>
        </Card>
      )}

      {clusters.isSuccess && clusters.data.length > 0 && (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {clusters.data.map((c) => (
            <Card key={c.id}>
              <CardHeader>
                <CardTitle className="flex items-center justify-between gap-2 text-base">
                  {c.name}
                  {c.locked && (
                    <Badge
                      variant="outline"
                      className="border-amber-600/40 text-amber-700 dark:text-amber-300"
                    >
                      read-only
                    </Badge>
                  )}
                </CardTitle>
                <CardDescription className="font-mono text-xs">{c.endpoint}</CardDescription>
              </CardHeader>
              <CardContent className="space-y-1 text-sm">
                <p>
                  {c.nodes} node{c.nodes === 1 ? '' : 's'} — {c.control_plane} control plane,{' '}
                  {c.workers} worker
                </p>
                {/* Three counts and not one health verdict: "the cluster is
                    degraded" hides which of the two failures it is, and the
                    difference decides what the operator does next. */}
                <p>
                  <span className="text-emerald-700 dark:text-emerald-300">
                    {c.healthy} healthy
                  </span>
                  {', '}
                  <span className="text-amber-700 dark:text-amber-300">{c.degraded} degraded</span>
                  {', '}
                  <span className="text-red-700 dark:text-red-300">{c.down} not answering</span>
                </p>
                <Button asChild variant="secondary" size="sm" className="mt-2">
                  <Link to="/clusters">Open</Link>
                </Button>
              </CardContent>
            </Card>
          ))}

          {unassigned.length > 0 && (
            <Card>
              <CardHeader>
                <CardTitle className="text-base">Not in a cluster</CardTitle>
                <CardDescription>
                  Machines holzkube-manager knows about that belong to no cluster. This is an
                  ordinary state, not an error.
                </CardDescription>
              </CardHeader>
              <CardContent>
                <p className="text-sm">
                  {unassigned.length} machine{unassigned.length === 1 ? '' : 's'}
                </p>
                <Button asChild variant="secondary" size="sm" className="mt-2">
                  <Link to="/nodes">Open the node list</Link>
                </Button>
              </CardContent>
            </Card>
          )}
        </div>
      )}

      <Card className="max-w-2xl">
        <CardHeader>
          <CardTitle>Instance</CardTitle>
          <CardDescription>Reported by this holzkube-manager process.</CardDescription>
        </CardHeader>
        <CardContent>
          {status.isPending && <Skeleton className="h-16 w-full" />}

          {status.isError && (
            <p className="text-sm text-muted-foreground">
              The status endpoint did not answer. holzkube-manager itself may be restarting.
            </p>
          )}

          {status.data && (
            <dl className="grid grid-cols-[minmax(0,10rem)_1fr] gap-x-4 gap-y-3 text-sm">
              <dt className="text-muted-foreground">Setup</dt>
              <dd>
                {status.data.setup_required
                  ? 'No operator account exists yet.'
                  : 'Operator account created.'}
              </dd>

              <dt className="text-muted-foreground">Audit chain</dt>
              <dd className="flex items-center gap-2">
                {status.data.audit_chain.ok ? (
                  <Badge variant="secondary">Verified</Badge>
                ) : (
                  <Badge variant="destructive">Broken</Badge>
                )}
                <span className="text-muted-foreground">
                  {status.data.audit_chain.ok
                    ? 'Every record verifies against its predecessor.'
                    : `First mismatch at line ${status.data.audit_chain.broken_at_line}.`}
                </span>
              </dd>

              <dt className="text-muted-foreground">Audit file</dt>
              <dd className="break-all font-mono text-xs">{status.data.audit_chain.file}</dd>
            </dl>
          )}
        </CardContent>
      </Card>

      <Card className="max-w-2xl">
        <CardHeader>
          <CardTitle>Recent activity</CardTitle>
          <CardDescription>The three most recent audit records, newest first.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {recent.isPending && <Skeleton className="h-16 w-full" />}

          {recent.isSuccess && recent.data.items.length === 0 && (
            <p className="text-sm text-muted-foreground">Nothing has been recorded yet.</p>
          )}

          {recent.isSuccess && recent.data.items.length > 0 && (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Time</TableHead>
                  <TableHead>Actor</TableHead>
                  <TableHead>Action</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {recent.data.items.slice(0, 3).map((record) => (
                  <TableRow key={record.seq}>
                    <TableCell className="tabular-nums">{record.ts}</TableCell>
                    <TableCell>{record.actor === '' ? '—' : record.actor}</TableCell>
                    <TableCell className="font-mono text-xs">{record.action}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}

          <Button asChild variant="secondary">
            <Link to="/audit">Open the audit log</Link>
          </Button>
        </CardContent>
      </Card>
    </div>
  )
}

export const indexRoute = createRoute({
  getParentRoute: () => authenticatedRoute,
  path: '/',
  component: Dashboard,
})
