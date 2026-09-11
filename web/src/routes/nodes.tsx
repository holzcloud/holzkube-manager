import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createRoute, Link } from '@tanstack/react-router'
import { RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { api, type Machine } from '@/api'
import { HealthField, StageBadge } from '@/components/HealthField'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { authenticatedRoute } from '@/routes/__root'

/**
 * Every machine holzkube-manager knows about, across every cluster, in one flat table
 * (D-29).
 *
 * Flat and not grouped by cluster, because a machine that belongs to no
 * cluster is an ordinary state -- it is every machine in maintenance mode, and
 * it is every machine whose cluster was removed -- and grouping would make the
 * normal case a special one. The `cluster` column says which, and an empty one
 * says none.
 *
 * The poll interval is deliberately slow. The server holds a supervisor per
 * node that observes on its own heartbeat (D-17), so this fetch is reading a
 * read model rather than driving the observation: asking faster would produce
 * the same answer. Phase 5 replaces the poll with SSE and, because the shape
 * of the answer does not change, nothing below this line moves (D-18).
 */
export const MACHINE_POLL_INTERVAL_MS = 15_000

export function NodesPage() {
  const { data, isLoading, error } = useQuery({
    queryKey: ['machines'],
    queryFn: () => api.machines.list(),
    refetchInterval: MACHINE_POLL_INTERVAL_MS,
  })

  if (isLoading) {
    return <p className="text-sm text-muted-foreground">Loading nodes…</p>
  }
  if (error) {
    return (
      <p className="text-sm text-destructive">
        The node list could not be read: {(error as Error).message}
      </p>
    )
  }

  const machines = data ?? []

  if (machines.length === 0) {
    return (
      <section className="space-y-3">
        <Heading />
        <p className="max-w-prose text-sm text-muted-foreground">
          No machines yet. Import a cluster on the{' '}
          <Link to="/clusters" className="underline">
            Clusters
          </Link>{' '}
          page — the inventory fills itself from the cluster's own membership.
        </p>
      </section>
    )
  }

  return (
    <section className="space-y-4">
      <Heading />
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Host</TableHead>
            <TableHead>State</TableHead>
            <TableHead>Role</TableHead>
            <TableHead>Address</TableHead>
            <TableHead>Talos</TableHead>
            <TableHead>Kubernetes</TableHead>
            <TableHead>Cluster</TableHead>
            <TableHead className="w-10" />
          </TableRow>
        </TableHeader>
        <TableBody>
          {machines.map((m) => (
            <MachineRow key={m.id} machine={m} />
          ))}
        </TableBody>
      </Table>
    </section>
  )
}

function Heading() {
  return (
    <div>
      <h1 className="font-heading text-xl font-semibold tracking-tight">Nodes</h1>
      <p className="text-sm text-muted-foreground">
        Every machine in the inventory, including the ones that are not answering. A record is only
        ever removed when you remove it.
      </p>
    </div>
  )
}

function MachineRow({ machine }: { machine: Machine }) {
  const queryClient = useQueryClient()
  const [refreshing, setRefreshing] = useState(false)

  const refresh = useMutation({
    mutationFn: () => api.machines.refresh(machine.id),
    onSettled: () => {
      setRefreshing(false)
      void queryClient.invalidateQueries({ queryKey: ['machines'] })
    },
  })

  return (
    <TableRow>
      <TableCell className="font-medium">
        <Link to="/nodes/$uuid" params={{ uuid: machine.id }} className="hover:underline">
          <HealthField field={machine.hostname} render={(v) => v || machine.id.slice(0, 8)} />
        </Link>
        {machine.lost_addr && (
          <Badge
            variant="outline"
            className="ml-2 border-amber-600/40 text-amber-700 dark:text-amber-300"
            title="A different machine answered at this one's last known address. This record was kept; the stranger got one of its own."
          >
            moved
          </Badge>
        )}
      </TableCell>
      <TableCell>
        <StageBadge stage={machine.stage} />
        {machine.certificate_expired && (
          <Badge
            variant="outline"
            className="ml-2 border-red-600/40 text-red-700 dark:text-red-300"
            title="The cluster's client certificate has expired. This is not a problem with the node."
          >
            certificate
          </Badge>
        )}
      </TableCell>
      <TableCell className="text-sm">{machine.role}</TableCell>
      <TableCell className="font-mono text-xs">
        <HealthField field={machine.addr} />
      </TableCell>
      <TableCell className="font-mono text-xs">
        <HealthField field={machine.talos_version} />
      </TableCell>
      <TableCell className="font-mono text-xs">
        <HealthField field={machine.kubernetes_version} />
      </TableCell>
      <TableCell className="text-sm">
        {machine.cluster === '' ? (
          <span className="text-muted-foreground" title="This machine belongs to no cluster.">
            —
          </span>
        ) : (
          <Link to="/clusters" className="hover:underline" title={`Cluster ${machine.cluster}`}>
            {machine.cluster.slice(0, 8)}
          </Link>
        )}
      </TableCell>
      <TableCell>
        <Button
          size="icon"
          variant="ghost"
          aria-label={`Refresh ${machine.id}`}
          title="Ask this node now, rather than waiting for the next heartbeat."
          disabled={refresh.isPending || refreshing}
          onClick={() => {
            setRefreshing(true)
            refresh.mutate()
          }}
        >
          <RefreshCw aria-hidden="true" className="size-4" />
        </Button>
      </TableCell>
    </TableRow>
  )
}

export const nodesRoute = createRoute({
  getParentRoute: () => authenticatedRoute,
  path: '/nodes',
  component: NodesPage,
})
