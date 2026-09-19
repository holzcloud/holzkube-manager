import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createRoute, Link } from '@tanstack/react-router'
import { RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { api, type Machine } from '@/api'
import { DataTable } from '@/components/DataTable'
import { HealthField, StageBadge } from '@/components/HealthField'
import { MachineClasses } from '@/components/MachineClasses'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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
      <DataTable
        label="Machines"
        rows={machines}
        keyOf={(m) => m.id}
        empty="No machines yet."
        columns={[
          {
            key: 'host',
            label: 'Host',
            role: 'identity',
            className: 'font-medium',
            render: (m) => (
              <>
                <Link
                  to="/nodes/$uuid"
                  params={{ uuid: m.id }}
                  // A whole line on a phone: measured 172x19 before, which is
                  // under the 44px a thumb needs and was invisible to the guard
                  // until it rendered rows at all (ledger 149).
                  className="inline-flex items-center hover:underline max-md:min-h-11"
                >
                  <HealthField field={m.hostname} render={(v) => v || m.id.slice(0, 8)} />
                </Link>
                {m.lost_addr && (
                  <Badge
                    variant="outline"
                    className="ml-2 border-amber-600/40 text-amber-700 dark:text-amber-300"
                    title="A different machine answered at this one's last known address. This record was kept; the stranger got one of its own."
                  >
                    moved
                  </Badge>
                )}
              </>
            ),
          },
          { key: 'state', label: 'State', render: (m) => <MachineState machine={m} /> },
          { key: 'role', label: 'Role', className: 'text-sm', render: (m) => m.role || '—' },
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
          {
            key: 'kubernetes',
            label: 'Kubernetes',
            className: 'font-mono text-xs',
            render: (m) => <HealthField field={m.kubernetes_version} />,
          },
          {
            key: 'cluster',
            label: 'Cluster',
            className: 'text-sm',
            render: (m) =>
              m.cluster === '' ? (
                <span className="text-muted-foreground" title="This machine belongs to no cluster.">
                  —
                </span>
              ) : (
                <Link
                  to="/clusters"
                  className="inline-flex items-center hover:underline max-md:min-h-11"
                  title={`Cluster ${m.cluster}`}
                >
                  {m.cluster.slice(0, 8)}
                </Link>
              ),
          },
          {
            key: 'refresh',
            label: 'Ask now',
            role: 'actions',
            render: (m) => <RefreshButton machine={m} />,
          },
        ]}
      />

      <MachineClasses />
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

/** The badges that qualify a machine's stage. Several can be true at once. */
function MachineState({ machine }: { machine: Machine }) {
  return (
    <>
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
      {/* OPS-03. Red for outside the range and amber for a pre-release,
          because they mean opposite things about what to do: one is a node
          to change, the other is a node this instance can accept once
          somebody says so. */}
      {machine.unsupported_version && (
        <Badge
          variant="outline"
          className="ml-2 border-red-600/40 text-red-700 dark:text-red-300"
          title={machine.version_notice}
        >
          unsupported
        </Badge>
      )}
      {!machine.unsupported_version && machine.pre_release && (
        <Badge
          variant="outline"
          className="ml-2 border-amber-600/40 text-amber-700 dark:text-amber-300"
          title={machine.version_notice}
        >
          pre-release
        </Badge>
      )}
      {machine.locked && (
        <Badge
          variant="outline"
          className="ml-2"
          title={machine.lock_reason || 'Rolling operations skip this node.'}
        >
          locked
        </Badge>
      )}
    </>
  )
}

function RefreshButton({ machine }: { machine: Machine }) {
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
    <Button
      size="icon"
      variant="ghost"
      // Full width on a phone, where it sits at the bottom of the card under
      // the host it refreshes; an icon-sized square there would be a 36px
      // target floating under a name.
      className="max-md:h-11 max-md:w-full"
      aria-label={`Refresh ${machine.id}`}
      title="Ask this node now, rather than waiting for the next heartbeat."
      disabled={refresh.isPending || refreshing}
      onClick={() => {
        setRefreshing(true)
        refresh.mutate()
      }}
    >
      <RefreshCw aria-hidden="true" className="size-4" />
      <span className="md:hidden">Ask this node now</span>
    </Button>
  )
}

export const nodesRoute = createRoute({
  getParentRoute: () => authenticatedRoute,
  path: '/nodes',
  component: NodesPage,
})
