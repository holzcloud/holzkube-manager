import { useQuery } from '@tanstack/react-query'
import { api } from '@/api'
import { DataTable } from '@/components/DataTable'
import { Problem } from '@/components/Problem'

/**
 * What the cluster reported (moved out of the route file, 2026-09-20).
 *
 * Unchanged: it was a private function in kubernetes.tsx, and splitting that
 * screen into pages left it without a home. A route file is where a route lives,
 * not a component.
 */
export function ClusterEvents({ clusterID, namespace }: { clusterID: string; namespace: string }) {
  const events = useQuery({
    queryKey: ['kubernetes', 'events', clusterID, namespace],
    queryFn: () => api.kubernetes.events(clusterID, namespace),
    refetchInterval: 15_000,
  })

  if (events.error) return <Problem error={events.error} />
  if (!events.data) return <p className="text-muted-foreground text-sm">Reading…</p>

  const warnings = events.data.events.filter((e) => e.type === 'Warning').reverse()

  return (
    <div className="space-y-2">
      {warnings.length === 0 ? (
        <p className="text-muted-foreground text-sm">{events.data.notice}</p>
      ) : (
        <>
          <DataTable
            label="Warnings the cluster reported"
            phone="rows"
            rows={warnings.slice(0, 20)}
            keyOf={(e) => `${e.object}-${e.reason}-${e.last_seen}`}
            empty={events.data.notice}
            columns={[
              {
                key: 'reason',
                label: 'Reason',
                role: 'identity',
                render: (e) => (
                  <span className="text-amber-700 dark:text-amber-300">
                    {e.reason}
                    {e.count > 1 && <span className="text-muted-foreground"> ×{e.count}</span>}
                  </span>
                ),
              },
              {
                key: 'object',
                label: 'Object',
                render: (e) => <span className="break-all font-mono text-xs">{e.object}</span>,
              },
              {
                key: 'message',
                label: 'Message',
                role: 'detail',
                render: (e) => <span className="break-words">{e.message}</span>,
              },
              {
                key: 'last_seen',
                label: 'Last seen',
                role: 'detail',
                render: (e) => <span className="tabular-nums">{e.last_seen}</span>,
              },
            ]}
          />
          <p className="text-muted-foreground text-xs">{events.data.notice}</p>
        </>
      )}
    </div>
  )
}
