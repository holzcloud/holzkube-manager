import type { Cluster } from '@/api'
import { formatDate } from '@/lib/format'
import { cn } from '@/lib/utils'

type NodeCertificates = NonNullable<Cluster['node_certificates']>
type Item = NodeCertificates['items'][number]

/**
 * The certificates inside a cluster, from its control-plane nodes.
 *
 * Two kinds, said on the screen because they mean different things when they
 * run short: an authority is never renewed by Talos, so its date is one to
 * plan a rotation for, and an issued certificate is renewed by Talos, so a
 * small number there is a rotation that is stuck on that node.
 *
 * Nothing read yet is a sentence of its own. A card that showed nothing until
 * the first pass would look exactly like a cluster whose certificates are fine.
 */
export function NodeCertificatesRow({ health }: { health: Cluster['node_certificates'] }) {
  if (health === null || health === undefined) {
    return <span className="text-muted-foreground">not read yet</span>
  }
  const soonest = health.items[0]
  const partial = health.read_nodes < health.control_planes

  return (
    <div className="flex flex-col gap-1">
      <p className={cn(health.urgency === 'none' ? undefined : urgencyClass(health.urgency))}>
        {soonest === undefined
          ? 'no certificates were reported'
          : `${health.items.length} certificates, soonest: ${soonest.name} on ${soonest.node}, ${daysText(soonest)}`}
      </p>
      {partial && (
        <p className="text-amber-700 dark:text-amber-300">
          {health.read_nodes} of {health.control_planes} control planes answered; the rest show
          their last reading.
        </p>
      )}
      {health.items.length > 0 && (
        <details>
          <summary className="cursor-pointer text-sm underline max-md:flex max-md:min-h-11 max-md:items-center">
            All certificates
          </summary>
          <ul className="mt-1 flex flex-col gap-1 text-sm">
            {health.items.map((it) => (
              <li
                key={`${it.node}/${it.name}`}
                className="grid grid-cols-1 gap-x-3 md:grid-cols-[minmax(0,1fr)_7rem_minmax(0,1fr)]"
              >
                <span className="min-w-0 break-words">
                  {it.name} <span className="text-muted-foreground">on {it.node}</span>
                </span>
                <span className="text-muted-foreground">{it.kind}</span>
                <span className={cn(it.urgency === 'none' ? undefined : urgencyClass(it.urgency))}>
                  {daysText(it)} ({formatDate(it.not_after)})
                </span>
              </li>
            ))}
          </ul>
        </details>
      )}
      <p className="text-muted-foreground text-xs">
        Last read {formatDate(health.checked_at)}. An authority is never renewed by Talos; an issued
        certificate is, so a short time left on one means a stuck rotation.
      </p>
    </div>
  )
}

function daysText(it: Item): string {
  if (it.days_left <= 0) return 'expired'
  return `${it.days_left} day${it.days_left === 1 ? '' : 's'} left`
}

function urgencyClass(urgency: Item['urgency']): string {
  return urgency === 'critical'
    ? 'text-red-700 dark:text-red-300'
    : 'text-amber-700 dark:text-amber-300'
}
