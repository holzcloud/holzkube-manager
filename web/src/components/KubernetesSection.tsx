import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import type { ReactNode } from 'react'
import { api, type KubernetesOverview } from '@/api'
import { Problem } from '@/components/Problem'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn } from '@/lib/utils'

/**
 * The Kubernetes screen's shell, and the selection every part of it shares
 * (2026-09-20).
 *
 * # Why this exists
 *
 * The screen had grown to fifteen cards on one page: how full the cluster is,
 * every workload, every pod, storage, networking, access control, namespaces,
 * events, the manifest form. The operator's words were that you scroll yourself
 * to death, and they are right — everything was reachable and nothing was
 * findable, which is the same defect the phone tables had in a different shape.
 *
 * So each of them is its own page now, with its own URL. That is worth more than
 * tabs would be: a link to the storage view is a link somebody can send, the back
 * button does what it looks like, and a screen that fails does not take the other
 * eight down with it.
 *
 * # Why the selection lives in the URL
 *
 * Which cluster and which namespace are the two things every one of these pages
 * needs, and moving between pages must not lose them. Held in component state
 * they would reset on every navigation; held in the URL they survive, they can be
 * sent to somebody else, and the back button restores them.
 *
 * They are OPTIONAL in the URL and absent by default, so the ordinary address is
 * `/kubernetes/storage` rather than a line of query parameters nobody chose. The
 * defaults are the first cluster and every namespace.
 */

export interface KubernetesSearch {
  cluster?: string
  namespace?: string
}

/** validateKubernetesSearch is the layout route's parser, shared by its children. */
export function validateKubernetesSearch(search: Record<string, unknown>): KubernetesSearch {
  const out: KubernetesSearch = {}
  // Kept as bare strings: a cluster id and a namespace are the server's
  // vocabulary, and a page that dropped names it did not recognise would answer
  // a newer cluster with silence.
  if (typeof search.cluster === 'string' && search.cluster !== '') out.cluster = search.cluster
  if (typeof search.namespace === 'string' && search.namespace !== '') {
    out.namespace = search.namespace
  }
  return out
}

/**
 * useClusterSelection is what every Kubernetes page reads.
 *
 * The clusters list and the overview are ordinary queries, so each page asking
 * for them costs nothing: react-query answers the second caller from the cache.
 * That is why this is a hook rather than a context — a page can be rendered on
 * its own, in a test or after a deep link, without a provider above it.
 */
export function useClusterSelection() {
  const search = useSearch({ strict: false }) as KubernetesSearch
  const clusters = useQuery({ queryKey: ['clusters'], queryFn: api.clusters.list })

  // The first cluster, until somebody picks another. A screen that made an
  // operator choose before showing anything would be asking a question it can
  // answer itself in the common case of one cluster.
  const selected = search.cluster ?? clusters.data?.[0]?.id ?? ''
  const namespace = search.namespace ?? ''

  return { clusters, selected, namespace }
}

/** The pages, in the order somebody works through them. */
const SECTIONS: { to: string; label: string }[] = [
  { to: '/kubernetes', label: 'Overview' },
  { to: '/kubernetes/workloads', label: 'Workloads' },
  { to: '/kubernetes/pods', label: 'Pods' },
  { to: '/kubernetes/storage', label: 'Storage' },
  { to: '/kubernetes/network', label: 'Network' },
  { to: '/kubernetes/config', label: 'Config' },
  { to: '/kubernetes/namespaces', label: 'Namespaces' },
  { to: '/kubernetes/access', label: 'Access' },
  { to: '/kubernetes/events', label: 'Events' },
  { to: '/kubernetes/maintenance', label: 'Maintenance' },
]

/**
 * KubernetesShell is the heading, the pickers and the navigation.
 *
 * It renders its children rather than an Outlet so a page can be tested with it
 * directly, which is how every other screen here is built.
 */
export function KubernetesShell({ children }: { children: ReactNode }) {
  // Narrowed by hand, and this is the honest reason: the router's generated
  // types infer a navigation's search shape from the route it starts at, and
  // this component is rendered by ten of them. `useSearch({ strict: false })`
  // above has the same problem and the same answer. The shape asserted here is
  // the one `validateKubernetesSearch` guarantees, so the assertion is checked
  // by the parser rather than merely hoped for.
  const navigate = useNavigate() as (options: {
    search: (previous: KubernetesSearch) => KubernetesSearch
    replace?: boolean
  }) => Promise<void>
  const search = useSearch({ strict: false }) as KubernetesSearch
  const { clusters, selected, namespace } = useClusterSelection()

  // The namespaces come from the overview, which every page reads anyway.
  const overview = useQuery({
    queryKey: ['kubernetes', selected, namespace],
    queryFn: () => api.kubernetes.overview(selected, namespace),
    enabled: selected !== '',
    refetchInterval: 10_000,
  })

  const setSearch = (next: KubernetesSearch) => {
    // `replace` deliberately: changing the cluster or the namespace is refining
    // the same question, and filling the back stack with every filter somebody
    // tried makes the back button useless for leaving the screen.
    void navigate({
      search: (previous) => ({ ...previous, ...next }),
      replace: true,
    })
  }

  return (
    <div className="space-y-4">
      <div>
        <h1 className="font-semibold text-2xl tracking-tight">Kubernetes</h1>
        <p className="text-muted-foreground text-sm">
          What the cluster’s own API server says. This is allowed to disagree with the Nodes screen
          — that screen reads the machine API, and where the two differ, the difference is the
          answer.
        </p>
      </div>

      {clusters.error ? <Problem error={clusters.error} /> : null}

      {clusters.data !== undefined && clusters.data.length === 0 && (
        <p className="text-muted-foreground text-sm">
          No cluster has been imported yet, so there is no Kubernetes API to ask.
        </p>
      )}

      <div className="flex flex-wrap items-center gap-3">
        {clusters.data !== undefined && clusters.data.length > 1 && (
          <div className="flex items-center gap-2">
            <span className="text-muted-foreground text-sm">Cluster</span>
            <Select value={selected} onValueChange={(value) => setSearch({ cluster: value })}>
              <SelectTrigger className="w-56 max-md:h-11" aria-label="Cluster">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {clusters.data.map((c) => (
                  <SelectItem key={c.id} value={c.id}>
                    {c.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}

        {/* The namespace filter sits beside the cluster rather than inside the
            pods card, because it now narrows eight pages rather than one. */}
        {overview.data && (
          <div className="flex items-center gap-2">
            <span className="text-muted-foreground text-sm">Namespace</span>
            <Select
              value={namespace === '' ? 'all' : namespace}
              onValueChange={(value) =>
                setSearch({ namespace: value === 'all' ? undefined : value })
              }
            >
              <SelectTrigger className="w-56 max-md:h-11" aria-label="Namespace">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">every namespace</SelectItem>
                {overview.data.namespaces.map((ns) => (
                  <SelectItem key={ns} value={ns}>
                    {ns}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}
      </div>

      {/* Wrapping rather than scrolling sideways: a tab strip that scrolls hides
          half of itself on a phone, and the thing it hides is the page somebody
          is looking for. Three rows that are all visible beat one row that is
          not. */}
      <nav aria-label="Kubernetes sections" className="flex flex-wrap gap-1 border-b pb-2">
        {SECTIONS.map((section) => (
          <Link
            key={section.to}
            to={section.to}
            search={search}
            // `exact` on the overview only: every other page is a leaf, and
            // without it the overview would look active on all of them.
            activeOptions={{ exact: section.to === '/kubernetes' }}
            className={cn(
              'rounded-md px-3 py-2 text-sm transition-colors',
              'max-md:min-h-11 max-md:flex-1 max-md:text-center',
              'hover:bg-muted',
            )}
            activeProps={{ className: 'bg-muted font-medium' }}
          >
            {section.label}
          </Link>
        ))}
      </nav>

      {children}
    </div>
  )
}

/**
 * KubernetesAnswer is what each page wraps its content in.
 *
 * It carries the one refusal every page owes: an API server that did not answer
 * is not an empty cluster (INV-08). Before, that sentence lived once on a page
 * that showed everything; with ten pages it has to live in the thing they share,
 * or nine of them would quietly show an empty screen as a fact.
 */
export function KubernetesAnswer({
  children,
}: {
  children: (overview: KubernetesOverview) => ReactNode
}) {
  const { selected, namespace } = useClusterSelection()

  const overview = useQuery({
    queryKey: ['kubernetes', selected, namespace],
    queryFn: () => api.kubernetes.overview(selected, namespace),
    enabled: selected !== '',
    refetchInterval: 10_000,
  })

  if (overview.error) {
    return (
      <>
        <Problem error={overview.error} />
        <p className="text-muted-foreground text-sm">
          Nothing here is a statement about this cluster: its API server was asked and did not
          answer.
        </p>
      </>
    )
  }
  if (!overview.data) return null

  return <>{children(overview.data)}</>
}
