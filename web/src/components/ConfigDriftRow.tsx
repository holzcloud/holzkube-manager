import type { Cluster } from '@/api'
import { STATE_LABEL } from '@/components/NodeConfigDrift'
import { formatDateTime } from '@/lib/format'
import { cn } from '@/lib/utils'

type ConfigDrift = NonNullable<Cluster['config_drift']>
type Item = ConfigDrift['items'][number]

/**
 * The configuration of the cluster's nodes against their baselines (15-09).
 *
 * Like the certificate row beside it, it says "not read yet" while the server
 * has nothing: a card that showed nothing until the first pass would look
 * exactly like a cluster whose configuration is fine.
 *
 * What it counts as "matching" is narrow on purpose. Only a node that was
 * compared and found equal (`ok`) matches. A node nobody has read, that cannot
 * be read or whose baseline is damaged has no verdict, and is listed under its
 * own state instead of being rounded up to fine. And "differs" follows `since`,
 * not the state: a node that stops answering while it drifts still drifts, and
 * the row says its drift is the last known one.
 *
 * The server decides all of it, including the 24 hours behind the banner; no
 * threshold is computed here.
 */
export function ConfigDriftRow({ health }: { health: Cluster['config_drift'] }) {
  if (health === null || health === undefined) {
    return <span className="text-muted-foreground">not read yet</span>
  }

  const items = health.items
  const drifting = items.filter((i) => i.since)
  const rest = items.filter((i) => !i.since)
  const matching = rest.filter((i) => i.state === 'ok')
  const created = rest.filter((i) => i.state === 'baseline-created')
  const settling = rest.filter((i) => i.state === 'settling')
  const unjudged = rest.filter(
    (i) => i.state === 'not-read' || i.state === 'unreadable' || i.state === 'baseline-damaged',
  )
  const oldest = drifting
    .map((i) => i.since)
    .filter((s): s is string => !!s)
    .sort()[0]
  const partial = health.read_nodes < health.nodes

  return (
    <div className="flex flex-col gap-1">
      {drifting.length > 0 && (
        <>
          <p className="text-amber-700 dark:text-amber-300">
            {drifting.length} of {items.length} nodes {drifting.length === 1 ? 'differs' : 'differ'}{' '}
            from {drifting.length === 1 ? 'its' : 'their'} baseline:{' '}
            {drifting
              .map((i) => (i.state === 'unreadable' ? `${i.node} (last known)` : i.node))
              .join(', ')}
          </p>
          {oldest && (
            <p className="text-amber-700 dark:text-amber-300 text-xs">
              The oldest difference is from {formatDateTime(oldest)}.
            </p>
          )}
        </>
      )}
      {matching.length > 0 && (
        <p>
          {matching.length === items.length
            ? matching.length === 1
              ? '1 node matches its baseline'
              : `${matching.length} nodes match their baseline`
            : `${matching.length} of ${items.length} nodes match their baseline`}
        </p>
      )}
      {created.length > 0 && (
        <p>
          The baseline of {created.length} node{created.length === 1 ? ' was' : 's were'} just
          created.
        </p>
      )}
      {settling.length > 0 && (
        <p className="text-amber-700 dark:text-amber-300">
          {settling.length} node{settling.length === 1 ? ' is' : 's are'} settling after an apply
          from here.
        </p>
      )}
      {unjudged.length > 0 && (
        <p className="text-muted-foreground">
          {unjudged.length} node{unjudged.length === 1 ? '' : 's'} cannot be judged:{' '}
          {unjudged.map((i) => `${i.node} (${STATE_LABEL[i.state]})`).join(', ')}
        </p>
      )}
      {items.length === 0 && <p className="text-muted-foreground">no nodes were read</p>}
      {partial && (
        <p className="text-amber-700 dark:text-amber-300">
          {health.read_nodes} of {health.nodes} nodes answered; the rest show their last reading.
        </p>
      )}
      {items.length > 0 && (
        <details>
          <summary className="cursor-pointer text-sm underline max-md:flex max-md:min-h-11 max-md:items-center">
            All nodes
          </summary>
          <ul className="mt-1 flex flex-col gap-1 text-sm">
            {items.map((it) => (
              <li
                key={it.machine}
                className="grid grid-cols-1 gap-x-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]"
              >
                <span className="min-w-0 break-words">{it.node}</span>
                <span className={cn(driftClass(it))}>{STATE_LABEL[it.state]}</span>
              </li>
            ))}
          </ul>
        </details>
      )}
      <p className="text-muted-foreground text-xs">
        Last read {formatDateTime(health.checked_at)}.
      </p>
    </div>
  )
}

function driftClass(it: Item): string | undefined {
  return it.since || it.state === 'settling' ? 'text-amber-700 dark:text-amber-300' : undefined
}
