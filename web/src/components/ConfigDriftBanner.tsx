import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { AlertTriangle } from 'lucide-react'
import { api, type Cluster } from '@/api'
import { cn } from '@/lib/utils'

/**
 * A configuration that has differed for a day, on every page (15-09, D-06).
 *
 * The server counts the 24 hours, from the start of the drift, and says so with
 * `banner` and the sentence in `warning`. This component computes no threshold
 * and keeps no state: it shows what the server reports and goes when the server
 * stops reporting it.
 *
 * It cannot be dismissed, on purpose. A difference that could be clicked away is
 * the one nobody sees any more, and the only way to end it is the one that ends
 * the fact: look at the node and confirm it as intended (or let it return to its
 * baseline). The one thing to click is the link to that node.
 *
 * It reads the cluster list under the same key as the certificate banner next to
 * it, so the two cost the server one request, not two.
 */
export function ConfigDriftBanner({ className }: { className?: string }) {
  const { data } = useQuery({
    queryKey: ['clusters'],
    queryFn: () => api.clusters.list(),
    // Quiet, like the certificate banner: a day is the unit here.
    refetchInterval: 5 * 60 * 1000,
    retry: false,
  })

  const warning = (data ?? []).filter((c) => c.config_drift?.banner === true)
  if (warning.length === 0) {
    return null
  }

  return (
    <div className={cn('flex flex-col gap-2', className)}>
      {warning.map((c) => (
        <DriftWarning key={c.id} cluster={c} />
      ))}
    </div>
  )
}

function DriftWarning({ cluster }: { cluster: Cluster }) {
  const target = oldest(cluster)
  return (
    <div
      role="status"
      className="flex items-start gap-2 rounded-md border border-amber-600/40 bg-amber-600/10 px-3 py-2 text-sm text-amber-700 dark:text-amber-300"
    >
      <AlertTriangle aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
      <div>
        <p>{cluster.config_drift?.warning}</p>
        <p className="mt-0.5 text-xs opacity-80">
          Cluster {cluster.name}.{' '}
          {target !== undefined && (
            <Link
              to="/nodes/$uuid"
              params={{ uuid: target }}
              className="underline max-md:inline-flex max-md:min-h-11 max-md:items-center"
            >
              Open the node
            </Link>
          )}
        </p>
      </div>
    </div>
  )
}

/** The machine whose drift began first: the one the sentence names. */
function oldest(cluster: Cluster): string | undefined {
  const drifting = (cluster.config_drift?.items ?? []).filter((i) => i.since)
  drifting.sort((a, b) => (a.since ?? '').localeCompare(b.since ?? ''))
  return drifting[0]?.machine
}
