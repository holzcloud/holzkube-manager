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
 * The phase-1 dashboard: a short status card, not a node dashboard. Nodes
 * arrive in phase 3, and inventing a preview of them here would be
 * indistinguishable from a broken real one later.
 */
function Dashboard() {
  const status = useSystemStatus()

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
        <p className="text-sm text-muted-foreground">
          Instance status. Cluster and node views arrive in phase 3.
        </p>
      </div>

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
