import { useQuery } from '@tanstack/react-query'
import { api } from '@/api'
import { DataTable } from '@/components/DataTable'
import { Problem } from '@/components/Problem'

/**
 * Namespaces, quotas, and the kinds this cluster has that Kubernetes does not
 * (2026-09-20).
 *
 * # Why a namespace needs rows and not only a dropdown
 *
 * It has been a filter everywhere in this product, which is what a namespace is
 * for most questions. Two things about one are findings in their own right, and
 * both read as a fault of the *workload*:
 *
 *   - Stuck in `Terminating`. Something in it has a finalizer nothing will clear,
 *     so the namespace hangs — for weeks — the name cannot be reused, and every
 *     attempt to recreate it fails with "already exists". `kubectl get ns` shows
 *     the word and nothing about what is holding it; the API server's own
 *     condition does, and that is what the row carries.
 *   - A full ResourceQuota. That is why the next pod is refused, and the refusal
 *     lands on the *pod* as "exceeded quota" in a namespace whose quota nothing on
 *     any screen showed.
 *
 * # The combination nobody expects
 *
 * A compute quota with no LimitRange refuses every pod that sets no requests — not
 * for being too big, but because the quota cannot account for a pod that asked for
 * nothing. The error says "must specify limits", which reads as the pod being
 * wrong. It is the namespace that is half-configured.
 *
 * # The cluster's own kinds
 *
 * A cluster's operators keep their state in them, and "what is a Longhorn Volume
 * called here" is not answerable from a list of pods. Objects are not counted per
 * kind: that would be one request per definition on a cluster that can have two
 * hundred.
 */
export function ClusterInventory({ clusterID }: { clusterID: string }) {
  const inventory = useQuery({
    queryKey: ['kubernetes', 'inventory', clusterID],
    queryFn: () => api.kubernetes.inventory(clusterID),
    enabled: clusterID !== '',
    refetchInterval: 60_000,
  })

  if (inventory.error) return <Problem error={inventory.error} />
  if (!inventory.data) return <p className="text-muted-foreground text-sm">Reading…</p>

  const { namespaces, custom_kinds, notice } = inventory.data

  // Broken first: the question is which namespace is the problem, and a list
  // sorted by name makes somebody read all of it to find out.
  const sorted = [...namespaces].sort((a, b) => Number(a.healthy) - Number(b.healthy))

  return (
    <div className="space-y-4">
      <DataTable
        label="Namespaces"
        rows={sorted}
        keyOf={(namespace) => namespace.name}
        empty="The API server answered, and the cluster has no namespaces — which cannot be, since every cluster has default and kube-system."
        columns={[
          {
            key: 'name',
            label: 'Namespace',
            role: 'identity',
            render: (namespace) => (
              <span className="break-all font-mono text-xs">
                {namespace.name}
                {namespace.phase === 'Terminating' ? (
                  <span className="ml-1 font-sans text-red-700 dark:text-red-300">
                    (terminating)
                  </span>
                ) : null}
              </span>
            ),
          },
          {
            key: 'pods',
            label: 'Pods',
            render: (namespace) =>
              namespace.pods_running === 0 ? (
                // Visible as empty rather than as a name.
                <span className="text-muted-foreground text-xs">empty</span>
              ) : (
                <span className="text-xs tabular-nums">{namespace.pods_running}</span>
              ),
          },
          {
            key: 'quota',
            label: 'Quota',
            render: (namespace) =>
              namespace.quotas.length === 0 ? (
                <span className="text-muted-foreground text-xs">none</span>
              ) : (
                <span className="text-xs tabular-nums">
                  {namespace.quotas
                    .map(
                      (line) =>
                        `${line.resource} ${line.used}/${line.hard}${line.percent >= 100 ? ' (full)' : ''}`,
                    )
                    .join(', ')}
                </span>
              ),
          },
          {
            key: 'defaults',
            label: 'Defaults',
            role: 'detail',
            render: (namespace) =>
              namespace.has_limit_range ? (
                <span className="text-xs">a LimitRange sets them</span>
              ) : (
                <span className="text-muted-foreground text-xs">none</span>
              ),
          },
          {
            key: 'why',
            label: 'Note',
            render: (namespace) =>
              namespace.notice === '' ? (
                '—'
              ) : (
                <span
                  className={
                    namespace.healthy
                      ? 'text-muted-foreground text-xs'
                      : 'text-amber-700 text-xs dark:text-amber-300'
                  }
                >
                  {namespace.notice}
                </span>
              ),
          },
        ]}
      />

      <DataTable
        label="This cluster’s own kinds"
        phone="rows"
        rows={custom_kinds}
        keyOf={(kind) => `${kind.group}/${kind.kind}`}
        empty="The API server answered, and the cluster defines no kinds of its own — or does not let this identity read them."
        columns={[
          {
            key: 'kind',
            label: 'Kind',
            role: 'identity',
            render: (kind) => (
              <span className="break-all font-mono text-xs">
                {kind.kind}
                <span className="text-muted-foreground">.{kind.group}</span>
                {kind.established ? null : (
                  <span className="ml-1 font-sans text-red-700 dark:text-red-300">
                    (not served)
                  </span>
                )}
              </span>
            ),
          },
          {
            key: 'versions',
            label: 'Versions',
            render: (kind) => (
              <span className="break-all font-mono text-xs">
                {kind.versions.join(', ') || '—'}
                {kind.stored === '' ? '' : ` (stores ${kind.stored})`}
              </span>
            ),
          },
          {
            key: 'scope',
            label: 'Scope',
            role: 'detail',
            render: (kind) => (
              <span className="text-xs">
                {kind.scope === 'Namespaced' ? 'in a namespace' : 'cluster-wide'}
              </span>
            ),
          },
          {
            key: 'why',
            label: 'Note',
            role: 'detail',
            render: (kind) =>
              kind.notice === '' ? (
                '—'
              ) : (
                <span className="text-amber-700 text-xs dark:text-amber-300">{kind.notice}</span>
              ),
          },
        ]}
      />

      {/* What a quota's figures are and what is deliberately not counted. */}
      <p className="text-muted-foreground text-xs">{notice}</p>
    </div>
  )
}
