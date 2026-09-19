import { useQuery } from '@tanstack/react-query'
import { api } from '@/api'
import { DataTable } from '@/components/DataTable'
import { Problem } from '@/components/Problem'

/**
 * What is actually being used (2026-09-19).
 *
 * The node detail reports what pods REQUESTED, because that is what the
 * scheduler reserves. This is the other half. Both are needed and neither
 * substitutes for the other: a node with no room and 5% usage is over-reserved,
 * a node with room and 95% usage is about to fall over, and those are opposite
 * repairs.
 *
 * Most clusters have no metrics-server — Talos does not ship one — so the
 * ordinary answer here is "nobody is collecting this", said in words rather than
 * shown as zeroes.
 */
export function ClusterUsage({ clusterID, namespace }: { clusterID: string; namespace: string }) {
  const usage = useQuery({
    queryKey: ['kubernetes', 'usage', clusterID, namespace],
    queryFn: () => api.kubernetes.usage(clusterID, namespace),
    refetchInterval: 30_000,
  })

  if (usage.error) return <Problem error={usage.error} />
  if (!usage.data) return <p className="text-muted-foreground text-sm">Reading…</p>

  if (!usage.data.collecting) {
    return <p className="text-muted-foreground text-sm">{usage.data.notice}</p>
  }

  // Busiest first: the question is "what is using it", and a list sorted by name
  // makes somebody read all of it to find out.
  const pods = [...usage.data.pods].sort((a, b) => b.cpu_millis - a.cpu_millis).slice(0, 10)

  return (
    <div className="space-y-3">
      <DataTable
        label="Node usage"
        rows={usage.data.nodes}
        keyOf={(u) => u.name}
        empty="The metrics API answered, and it has nothing about these nodes."
        columns={[
          {
            key: 'name',
            label: 'Node',
            role: 'identity',
            render: (u) => <span className="break-all font-mono text-xs">{u.name}</span>,
          },
          { key: 'cpu', label: 'CPU', render: (u) => u.cpu },
          { key: 'memory', label: 'Memory', render: (u) => u.memory },
        ]}
      />

      {pods.length > 0 && (
        <DataTable
          label="Busiest pods"
          phone="rows"
          rows={pods}
          keyOf={(u) => `${u.namespace}/${u.name}`}
          empty="The metrics API answered, and it has nothing about these pods."
          columns={[
            {
              key: 'name',
              label: 'Pod',
              role: 'identity',
              render: (u) => (
                <span className="break-all font-mono text-xs">
                  {u.namespace}/{u.name}
                </span>
              ),
            },
            { key: 'cpu', label: 'CPU', render: (u) => u.cpu },
            { key: 'memory', label: 'Memory', render: (u) => u.memory },
          ]}
        />
      )}

      {/* The distinction this panel exists beside, said rather than implied. */}
      <p className="text-muted-foreground text-xs">{usage.data.notice}</p>
    </div>
  )
}
