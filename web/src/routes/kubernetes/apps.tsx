import { useQuery } from '@tanstack/react-query'
import { createRoute, Link, useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { type App, api } from '@/api'
import { StatTile } from '@/components/charts/Meter'
import { DataTable } from '@/components/DataTable'
import { KubernetesShell, useClusterSelection } from '@/components/KubernetesSection'
import { PowerMenu } from '@/components/PowerMenu'
import { Problem } from '@/components/Problem'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { formatBytes, formatCores } from '@/lib/format'
import { kubernetesRoute } from '@/routes/kubernetes'

/**
 * Every app in the cluster and what it is using now (2026-09-26).
 *
 * "App" is what somebody deployed, not what Kubernetes happens to call the
 * pieces: a Deployment's pods are counted under the Deployment, not under the
 * ReplicaSet in between that nobody wrote. The numbers are read from each
 * node's kubelet on every refresh, so they are what is happening, not what was
 * asked for -- requests and limits are shown beside them for exactly that
 * comparison.
 *
 * Busiest first by default, because the question this page answers is "what is
 * eating the cluster", and a list sorted by name makes somebody read all of it.
 */

export const APPS_POLL_INTERVAL_MS = 5_000

type SortKey = 'cpu' | 'memory' | 'name'

export function sortApps(apps: App[], by: SortKey): App[] {
  const byName = (a: App, b: App) =>
    a.namespace.localeCompare(b.namespace) || a.name.localeCompare(b.name)
  return [...apps].sort((a, b) => {
    if (by === 'cpu') return b.cpu_millis - a.cpu_millis || byName(a, b)
    if (by === 'memory') return b.memory_bytes - a.memory_bytes || byName(a, b)
    return byName(a, b)
  })
}

export function KubernetesAppsPage() {
  const { selected, namespace } = useClusterSelection()
  const navigate = useNavigate()
  const [sort, setSort] = useState<SortKey>('cpu')

  const apps = useQuery({
    queryKey: ['kubernetes', 'apps', selected, { namespace }],
    queryFn: () => api.kubernetes.apps(selected, { namespace }),
    enabled: selected !== '',
    refetchInterval: APPS_POLL_INTERVAL_MS,
  })

  const rows = apps.data ? sortApps(apps.data.apps, sort) : []
  const maxCPU = Math.max(1, ...rows.map((a) => a.cpu_millis))
  const maxMemory = Math.max(1, ...rows.map((a) => a.memory_bytes))
  const totalCPU = rows.reduce((sum, a) => sum + a.cpu_millis, 0)
  const totalMemory = rows.reduce((sum, a) => sum + a.memory_bytes, 0)
  const totalPods = rows.reduce((sum, a) => sum + a.pods, 0)
  const unhealthy = rows.filter((a) => a.ready < a.pods).length

  const open = (a: App) =>
    void navigate({
      to: '/kubernetes/apps/$namespace/$kind/$name',
      params: { namespace: a.namespace, kind: a.kind, name: a.name },
      search: (previous: Record<string, unknown>) => previous,
    })

  return (
    <KubernetesShell>
      {apps.error ? <Problem error={apps.error} /> : null}

      {apps.data && (
        <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
          <StatTile label="Apps" value={rows.length} hint={`${totalPods} pods`} />
          <StatTile
            label="Not all pods ready"
            value={unhealthy}
            hint={unhealthy === 0 ? 'every app is complete' : 'see the list below'}
          />
          <StatTile label="CPU in use" value={formatCores(totalCPU)} hint="by the apps listed" />
          <StatTile
            label="Memory in use"
            value={formatBytes(totalMemory)}
            hint="working set, by the apps listed"
          />
        </div>
      )}

      <Card>
        <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
          <CardTitle>Apps</CardTitle>
          <fieldset className="m-0 flex gap-1 border-0 p-0">
            <legend className="sr-only">Sort by</legend>
            {(
              [
                ['cpu', 'CPU'],
                ['memory', 'Memory'],
                ['name', 'Name'],
              ] as const
            ).map(([key, label]) => (
              <Button
                key={key}
                size="sm"
                variant={sort === key ? 'secondary' : 'ghost'}
                aria-pressed={sort === key}
                onClick={() => setSort(key)}
              >
                {label}
              </Button>
            ))}
          </fieldset>
        </CardHeader>
        <CardContent className="space-y-3">
          {apps.data === undefined && !apps.error && (
            <p className="text-muted-foreground text-sm">Asking every node’s kubelet…</p>
          )}
          {apps.data && (
            <div className={apps.isFetching && apps.isStale ? 'opacity-90' : undefined}>
              <DataTable
                label="Apps"
                rows={rows}
                keyOf={(a) => `${a.namespace}/${a.kind}/${a.name}`}
                onRowClick={open}
                rowLabel={(a) => `Open ${a.kind} ${a.namespace}/${a.name}`}
                empty="Nothing runs in this namespace."
                columns={[
                  {
                    key: 'app',
                    label: 'App',
                    role: 'identity',
                    render: (a) => (
                      <div className="min-w-0">
                        <Link
                          to="/kubernetes/apps/$namespace/$kind/$name"
                          params={{ namespace: a.namespace, kind: a.kind, name: a.name }}
                          search={(previous: Record<string, unknown>) => previous}
                          className="font-medium underline-offset-2 hover:underline"
                          onClick={(e) => e.stopPropagation()}
                        >
                          {a.name}
                        </Link>
                        <p className="text-muted-foreground text-xs">
                          {a.namespace} · {a.kind}
                        </p>
                      </div>
                    ),
                  },
                  {
                    key: 'pods',
                    label: 'Pods',
                    render: (a) => (
                      <span
                        className={
                          a.ready < a.pods
                            ? 'text-amber-700 tabular-nums dark:text-amber-300'
                            : 'tabular-nums'
                        }
                      >
                        {a.ready}/{a.pods} ready
                        {a.restarts > 0 && (
                          <span className="text-muted-foreground"> · {a.restarts} restarts</span>
                        )}
                      </span>
                    ),
                  },
                  {
                    key: 'nodes',
                    label: 'Nodes',
                    role: 'detail',
                    render: (a) =>
                      a.nodes.length === 0 ? (
                        <span className="text-muted-foreground text-xs">none</span>
                      ) : (
                        <span className="font-mono text-xs">{a.nodes.join(', ')}</span>
                      ),
                  },
                  {
                    key: 'cpu',
                    label: 'CPU',
                    render: (a) => (
                      <Usage
                        known={a.usage_known}
                        value={a.cpu_millis}
                        max={maxCPU}
                        display={formatCores(a.cpu_millis)}
                        reference={
                          a.cpu_limit_millis > 0
                            ? `limit ${formatCores(a.cpu_limit_millis)}`
                            : a.cpu_request_millis > 0
                              ? `requests ${formatCores(a.cpu_request_millis)}`
                              : 'no request'
                        }
                      />
                    ),
                  },
                  {
                    key: 'memory',
                    label: 'Memory',
                    render: (a) => (
                      <Usage
                        known={a.usage_known}
                        value={a.memory_bytes}
                        max={maxMemory}
                        display={formatBytes(a.memory_bytes)}
                        reference={
                          a.memory_limit_bytes > 0
                            ? `limit ${formatBytes(a.memory_limit_bytes)}`
                            : a.memory_request_bytes > 0
                              ? `requests ${formatBytes(a.memory_request_bytes)}`
                              : 'no request'
                        }
                      />
                    ),
                  },
                  {
                    key: 'power',
                    label: '',
                    role: 'actions',
                    render: (a) => (
                      // biome-ignore lint/a11y/noStaticElementInteractions: stops the row's own click, it handles nothing itself
                      // biome-ignore lint/a11y/useKeyWithClickEvents: same
                      <span onClick={(e) => e.stopPropagation()}>
                        <PowerMenu
                          target={{
                            kind: 'app',
                            cluster: selected,
                            namespace: a.namespace,
                            appKind: a.kind,
                            name: a.name,
                          }}
                        />
                      </span>
                    ),
                  },
                ]}
              />
            </div>
          )}
          {apps.data && apps.data.notice !== '' && (
            <p className="text-muted-foreground text-xs">{apps.data.notice}</p>
          )}
        </CardContent>
      </Card>
    </KubernetesShell>
  )
}

/**
 * One app's usage beside the biggest in the list.
 *
 * The bar is length against the busiest app on this page, so the eye finds the
 * heavy ones without reading figures; the figure is the value itself, and the
 * line under it is what the app asked for, which is the comparison that says
 * whether it is behaving.
 */
function Usage({
  known,
  value,
  max,
  display,
  reference,
}: {
  known: boolean
  value: number
  max: number
  display: string
  reference: string
}) {
  if (!known) return <span className="text-muted-foreground text-xs">not reported</span>
  return (
    <div className="min-w-28">
      <span className="text-sm tabular-nums">{display}</span>
      <div
        aria-hidden="true"
        className="mt-0.5 h-1.5 w-full rounded-[2px]"
        style={{ background: 'var(--viz-track)' }}
      >
        <div
          className="h-full rounded-r-[4px] transition-[width] duration-500"
          style={{
            width: `${Math.max(1, (value / max) * 100)}%`,
            background: 'var(--viz-series-1)',
          }}
        />
      </div>
      <p className="text-muted-foreground text-xs">{reference}</p>
    </div>
  )
}

export const kubernetesAppsRoute = createRoute({
  getParentRoute: () => kubernetesRoute,
  path: 'apps',
  component: KubernetesAppsPage,
})
