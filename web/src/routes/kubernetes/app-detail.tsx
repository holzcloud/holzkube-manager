import { useQuery } from '@tanstack/react-query'
import { createRoute, Link } from '@tanstack/react-router'
import { type AppPod, api } from '@/api'
import { LiveChart } from '@/components/charts/LiveChart'
import { StatTile } from '@/components/charts/Meter'
import { DataTable } from '@/components/DataTable'
import { KubernetesShell, useClusterSelection } from '@/components/KubernetesSection'
import { PowerMenu } from '@/components/PowerMenu'
import { Problem } from '@/components/Problem'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useLiveSeries } from '@/hooks/useLiveSeries'
import { formatBytes, formatCores } from '@/lib/format'
import { kubernetesRoute } from '@/routes/kubernetes'
import { APPS_POLL_INTERVAL_MS } from '@/routes/kubernetes/apps'

/**
 * One app, and everything about it that is happening now (2026-09-26).
 *
 * Its pods and where they run, what each container uses against what it asked
 * for, the services that reach it, and the events that say what went wrong
 * lately. CPU and memory are two charts, never one: they are different units,
 * and one chart with two scales invents a relationship between them.
 */
export function AppDetailPage() {
  const { namespace, kind, name } = appDetailRoute.useParams()
  const { selected } = useClusterSelection()

  const detail = useQuery({
    queryKey: ['kubernetes', 'app', selected, namespace, kind, name],
    queryFn: () => api.kubernetes.app(selected, namespace, kind, name),
    enabled: selected !== '',
    refetchInterval: APPS_POLL_INTERVAL_MS,
  })

  const machines = useQuery({ queryKey: ['machines'], queryFn: api.machines.list })
  const machineByHost = new Map(
    (machines.data ?? []).map((m) => [m.hostname.value ?? '', m.id] as const),
  )

  const d = detail.data
  const history = useLiveSeries(
    `app:${selected}/${namespace}/${kind}/${name}`,
    d,
    d?.collected_at,
    (r) => ({ cpu: r.app.cpu_millis, memory: r.app.memory_bytes }),
  )

  return (
    <KubernetesShell>
      <div className="space-y-4">
        <header>
          <p className="text-muted-foreground text-sm">
            <Link
              to="/kubernetes/apps"
              search={(previous: Record<string, unknown>) => previous}
              className="underline"
            >
              Apps
            </Link>{' '}
            / {namespace}
          </p>
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="font-semibold text-xl tracking-tight">{name}</h2>
            <Badge variant="outline">{kind}</Badge>
            {selected !== '' && (
              <span className="ml-auto">
                <PowerMenu
                  target={{ kind: 'app', cluster: selected, namespace, appKind: kind, name }}
                />
              </span>
            )}
          </div>
          {d && d.app.images.length > 0 && (
            <p className="break-all font-mono text-muted-foreground text-xs">
              {d.app.images.join(', ')}
            </p>
          )}
        </header>

        {detail.error ? <Problem error={detail.error} /> : null}
        {d === undefined && !detail.error && (
          <p className="text-muted-foreground text-sm">Asking the cluster…</p>
        )}

        {d && (
          <>
            <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
              <StatTile
                label="CPU now"
                value={d.app.usage_known ? formatCores(d.app.cpu_millis) : '—'}
                hint={
                  d.app.cpu_limit_millis > 0
                    ? `limit ${formatCores(d.app.cpu_limit_millis)}`
                    : d.app.cpu_request_millis > 0
                      ? `requests ${formatCores(d.app.cpu_request_millis)}`
                      : 'no request, no limit'
                }
              />
              <StatTile
                label="Memory now"
                value={d.app.usage_known ? formatBytes(d.app.memory_bytes) : '—'}
                hint={
                  d.app.memory_limit_bytes > 0
                    ? `limit ${formatBytes(d.app.memory_limit_bytes)}`
                    : d.app.memory_request_bytes > 0
                      ? `requests ${formatBytes(d.app.memory_request_bytes)}`
                      : 'no request, no limit'
                }
              />
              <StatTile
                label="Pods ready"
                value={`${d.app.ready}/${d.app.pods}`}
                hint={d.app.nodes.length > 0 ? `on ${d.app.nodes.join(', ')}` : 'on no node'}
              />
              <StatTile
                label="Restarts"
                value={d.app.restarts}
                hint={
                  d.created_at
                    ? `created ${new Date(d.created_at).toLocaleDateString()}`
                    : undefined
                }
              />
            </div>

            <div className="grid gap-4 lg:grid-cols-2">
              <Card>
                <CardHeader>
                  <CardTitle className="text-base">CPU</CardTitle>
                </CardHeader>
                <CardContent>
                  <LiveChart
                    title="CPU in use"
                    series={[{ key: 'cpu', label: 'CPU', slot: 1, points: history.cpu ?? [] }]}
                    format={formatCores}
                  />
                </CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardTitle className="text-base">Memory</CardTitle>
                </CardHeader>
                <CardContent>
                  <LiveChart
                    title="Memory in use"
                    series={[
                      { key: 'memory', label: 'Memory', slot: 1, points: history.memory ?? [] },
                    ]}
                    format={formatBytes}
                  />
                </CardContent>
              </Card>
            </div>

            <Card>
              <CardHeader>
                <CardTitle className="text-base">Pods</CardTitle>
              </CardHeader>
              <CardContent>
                {d.pods.length === 0 ? (
                  <p className="text-muted-foreground text-sm">No pod of this app is running.</p>
                ) : (
                  <PodTable pods={d.pods} machineByHost={machineByHost} />
                )}
              </CardContent>
            </Card>

            <div className="grid gap-4 lg:grid-cols-2">
              <Card>
                <CardHeader>
                  <CardTitle className="text-base">Reached through</CardTitle>
                </CardHeader>
                <CardContent>
                  {d.services.length === 0 ? (
                    <p className="text-muted-foreground text-sm">
                      No service selects these pods, so nothing in the cluster reaches them by name.
                    </p>
                  ) : (
                    <ul className="space-y-2 text-sm">
                      {d.services.map((s) => (
                        <li key={s.name}>
                          <span className="font-medium">{s.name}</span>{' '}
                          <span className="text-muted-foreground text-xs">
                            {s.type}
                            {s.cluster_ip && ` · ${s.cluster_ip}`}
                          </span>
                          {s.ports.length > 0 && (
                            <p className="font-mono text-xs">{s.ports.join(', ')}</p>
                          )}
                        </li>
                      ))}
                    </ul>
                  )}
                </CardContent>
              </Card>

              <Card>
                <CardHeader>
                  <CardTitle className="text-base">Recent events</CardTitle>
                </CardHeader>
                <CardContent>
                  {d.events.length === 0 ? (
                    <p className="text-muted-foreground text-sm">
                      Nothing has happened to it lately.
                    </p>
                  ) : (
                    <ul className="space-y-2 text-sm">
                      {d.events.map((e) => (
                        <li key={`${e.object}/${e.reason}/${e.last_seen}`}>
                          <p>
                            <span
                              className={
                                e.type === 'Warning'
                                  ? 'font-medium text-amber-700 dark:text-amber-300'
                                  : 'font-medium'
                              }
                            >
                              {e.reason}
                            </span>{' '}
                            <span className="text-muted-foreground text-xs">
                              {e.object}
                              {e.count > 1 && ` · ${e.count}×`}
                              {e.last_seen && ` · ${new Date(e.last_seen).toLocaleString()}`}
                            </span>
                          </p>
                          <p className="break-words text-muted-foreground text-xs">{e.message}</p>
                        </li>
                      ))}
                    </ul>
                  )}
                </CardContent>
              </Card>
            </div>

            {d.notice !== '' && <p className="text-muted-foreground text-xs">{d.notice}</p>}
          </>
        )}
      </div>
    </KubernetesShell>
  )
}

/**
 * The pods, with their containers under each.
 *
 * Through DataTable, so a phone gets a card per pod rather than a table it has
 * to swipe -- the layout guard caught the first version doing exactly that.
 */
function PodTable({ pods, machineByHost }: { pods: AppPod[]; machineByHost: Map<string, string> }) {
  return (
    <DataTable
      label="Pods"
      rows={pods}
      keyOf={(p) => p.name}
      empty="No pod of this app is running."
      columns={[
        {
          key: 'pod',
          label: 'Pod',
          role: 'identity',
          render: (p) => (
            <div className="min-w-0">
              <span className="break-all font-mono text-xs">{p.name}</span>
              {p.ip && <p className="font-mono text-muted-foreground text-xs">{p.ip}</p>}
            </div>
          ),
        },
        {
          key: 'node',
          label: 'Node',
          render: (p) => {
            const machine = machineByHost.get(p.node)
            return machine ? (
              <Link
                to="/nodes/$uuid"
                params={{ uuid: machine }}
                className="break-all font-mono text-xs underline-offset-2 hover:underline max-md:inline-flex max-md:min-h-11 max-md:items-center"
              >
                {p.node}
              </Link>
            ) : (
              <span className="break-all font-mono text-xs">{p.node || '—'}</span>
            )
          },
        },
        {
          key: 'state',
          label: 'State',
          render: (p) => (
            <span className="text-xs">
              {p.phase} · {p.ready}
              {p.restarts > 0 && (
                <span className="text-amber-700 dark:text-amber-300"> · {p.restarts} restarts</span>
              )}
            </span>
          ),
        },
        {
          key: 'cpu',
          label: 'CPU',
          render: (p) => (
            <span className="tabular-nums">{p.usage_known ? formatCores(p.cpu_millis) : '—'}</span>
          ),
        },
        {
          key: 'memory',
          label: 'Memory',
          render: (p) => (
            <span className="tabular-nums">
              {p.usage_known ? formatBytes(p.memory_bytes) : '—'}
            </span>
          ),
        },
        {
          key: 'containers',
          label: 'Containers',
          role: 'detail',
          render: (p) => (
            <ul className="space-y-1 text-xs">
              {p.containers.map((c) => (
                <li key={c.name}>
                  <span className="font-medium">{c.name}</span>{' '}
                  <span className="text-muted-foreground">{c.state}</span>
                  {c.restarts > 0 && ` · ${c.restarts} restarts`}
                  <span className="block tabular-nums">
                    {c.usage_known ? formatCores(c.cpu_millis) : '—'}
                    <span className="text-muted-foreground">
                      {' '}
                      of{' '}
                      {c.cpu_limit_millis > 0
                        ? `limit ${formatCores(c.cpu_limit_millis)}`
                        : c.cpu_request_millis > 0
                          ? `request ${formatCores(c.cpu_request_millis)}`
                          : 'no request'}
                    </span>
                    {' · '}
                    {c.usage_known ? formatBytes(c.memory_bytes) : '—'}
                    <span className="text-muted-foreground">
                      {' '}
                      of{' '}
                      {c.memory_limit_bytes > 0
                        ? `limit ${formatBytes(c.memory_limit_bytes)}`
                        : c.memory_request_bytes > 0
                          ? `request ${formatBytes(c.memory_request_bytes)}`
                          : 'no request'}
                    </span>
                  </span>
                  <span className="block break-all font-mono text-muted-foreground">{c.image}</span>
                </li>
              ))}
            </ul>
          ),
        },
      ]}
    />
  )
}

export const appDetailRoute = createRoute({
  getParentRoute: () => kubernetesRoute,
  path: 'apps/$namespace/$kind/$name',
  component: AppDetailPage,
})
