import { useQuery } from '@tanstack/react-query'
import { api, type Capacity, type NodeCapacity } from '@/api'
import { DataTable } from '@/components/DataTable'
import { Problem } from '@/components/Problem'

/**
 * How full the cluster is (2026-09-20).
 *
 * # Why this panel and the usage panel both exist
 *
 * The operator said they could see no resource information anywhere — not for
 * the cluster, not for a node, not for a pod. The usage panel was there, and on
 * this cluster it correctly says nobody is collecting: Talos ships no
 * metrics-server, so the honest answer was an explanatory sentence, which reads
 * exactly like nothing.
 *
 * This is the figure every cluster has: allocatable against what the pods
 * REQUESTED. It is the scheduler's own arithmetic, so it is also what decides
 * whether the next pod starts — which is the question behind "how full is it"
 * more often than consumption is.
 *
 * Neither number is called the other, here or on the server. A cluster at 90%
 * requested and 5% used is over-reserved and will refuse work it could do; one
 * at 20% requested and 95% used is about to fall over while looking empty. Those
 * are opposite repairs.
 *
 * # Why a bar and not just a percentage
 *
 * The bar is the comparison; the two quantities underneath are the answer. A bar
 * alone cannot say "1200m of 4", and on a cluster where the interesting fact is
 * which node is nearly full, the shapes are what makes that visible at a glance
 * on a phone. Both are always present, so the bar never has to be read as a
 * measurement.
 */
export function ClusterCapacityPanel({ clusterID }: { clusterID: string }) {
  const capacity = useQuery({
    queryKey: ['kubernetes', 'capacity', clusterID],
    queryFn: () => api.kubernetes.capacity(clusterID),
    enabled: clusterID !== '',
    refetchInterval: 30_000,
  })

  if (capacity.error) return <Problem error={capacity.error} />
  if (!capacity.data) return <p className="text-muted-foreground text-sm">Reading…</p>

  const { cpu, memory, pods, nodes, notice } = capacity.data

  return (
    <div className="space-y-4">
      <div className="grid gap-3 sm:grid-cols-3">
        <Meter label="CPU" capacity={cpu} />
        <Meter label="Memory" capacity={memory} />
        <Meter label="Pods" capacity={pods} />
      </div>

      <DataTable
        label="Node capacity"
        rows={nodes}
        keyOf={(node) => node.name}
        empty="The API server answered, and it lists no nodes at all."
        columns={[
          {
            key: 'name',
            label: 'Node',
            role: 'identity',
            render: (node) => (
              <span className="break-all font-mono text-xs">
                {node.name}
                {/* Said on the row, because a node that takes nothing new is
                    the reason a cluster with apparent room refuses a pod. */}
                {node.cordoned ? <NodeNote>cordoned</NodeNote> : null}
                {node.ready ? null : <NodeNote>not ready</NodeNote>}
              </span>
            ),
          },
          { key: 'cpu', label: 'CPU', render: (node) => <Cell capacity={node.cpu} /> },
          { key: 'memory', label: 'Memory', render: (node) => <Cell capacity={node.memory} /> },
          { key: 'pods', label: 'Pods', render: (node) => <Cell capacity={node.pods} /> },
        ]}
      />

      {/* What these numbers ARE. "80% full" invites exactly the wrong reading,
          and the sentence comes from the server so both halves say one thing. */}
      <p className="text-muted-foreground text-xs">{notice}</p>
      {nodes.some(excludedFromTotals) && (
        <p className="text-muted-foreground text-xs">
          A cordoned or unready node is left out of the totals above — nothing new will be placed
          there, so counting its room would report space that does not exist. The pods already on it
          are still counted, because they are still on it.
        </p>
      )}
    </div>
  )
}

function excludedFromTotals(node: NodeCapacity): boolean {
  return node.cordoned || !node.ready
}

function NodeNote({ children }: { children: string }) {
  return (
    <span className="ml-1 font-sans text-amber-700 dark:text-amber-300">{`(${children})`}</span>
  )
}

/** Meter is one quantity, large, for the cluster totals. */
function Meter({ label, capacity }: { label: string; capacity: Capacity }) {
  return (
    <div className="rounded-md border p-3">
      <div className="flex items-baseline justify-between gap-2">
        <span className="font-medium text-sm">{label}</span>
        <span className="tabular-nums text-sm">
          {capacity.percent < 0 ? 'not reported' : `${capacity.percent}%`}
        </span>
      </div>
      <Bar percent={capacity.percent} label={label} />
      <p className="mt-1 text-muted-foreground text-xs tabular-nums">
        {capacity.percent < 0
          ? 'No node has reported what it can offer.'
          : `${capacity.requested} requested of ${capacity.allocatable}`}
      </p>
    </div>
  )
}

/** Cell is the same thing inside a table row. */
function Cell({ capacity }: { capacity: Capacity }) {
  if (capacity.percent < 0) {
    return <span className="text-muted-foreground text-xs">not reported</span>
  }
  return (
    <div className="min-w-24">
      <span className="text-xs tabular-nums">
        {capacity.percent}% · {capacity.requested} of {capacity.allocatable}
      </span>
      <Bar percent={capacity.percent} label="" />
    </div>
  )
}

/**
 * Bar is the comparison, drawn.
 *
 * Over 100% is possible and is not an error: a cluster can be over-committed by
 * a manifest applied while a node was down. It is clamped so the bar stays
 * inside its track, and the figure beside it still says 115%, which is the part
 * that matters.
 *
 * `aria-hidden`, deliberately: the quantities are in the text beside it, so to a
 * screen reader this is decoration repeating what was already said.
 */
function Bar({ percent, label }: { percent: number; label: string }) {
  if (percent < 0) return null
  const width = Math.min(100, Math.max(0, percent))
  return (
    <div
      aria-hidden="true"
      data-testid={label === '' ? undefined : `capacity-bar-${label.toLowerCase()}`}
      className="mt-1 h-2 w-full overflow-hidden rounded-full bg-muted"
    >
      <div
        className={
          percent >= 90
            ? 'h-full bg-red-600 dark:bg-red-500'
            : percent >= 75
              ? 'h-full bg-amber-500'
              : 'h-full bg-emerald-600 dark:bg-emerald-500'
        }
        style={{ width: `${width}%` }}
      />
    </div>
  )
}
