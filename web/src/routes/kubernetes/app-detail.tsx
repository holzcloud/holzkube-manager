import { useQuery } from '@tanstack/react-query'
import { createRoute, Link } from '@tanstack/react-router'
import { ChevronRight } from 'lucide-react'
import { Fragment, useState } from 'react'
import { type AppPod, api } from '@/api'
import { LiveChart } from '@/components/charts/LiveChart'
import { StatTile } from '@/components/charts/Meter'
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

/** The pods, each opening onto its containers. */
function PodTable({ pods, machineByHost }: { pods: AppPod[]; machineByHost: Map<string, string> }) {
  const [open, setOpen] = useState<Record<string, boolean>>({})
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="text-left text-muted-foreground text-xs">
            <th className="py-1 font-normal">Pod</th>
            <th className="py-1 font-normal">Node</th>
            <th className="py-1 font-normal">State</th>
            <th className="py-1 text-right font-normal">CPU</th>
            <th className="py-1 text-right font-normal">Memory</th>
          </tr>
        </thead>
        <tbody>
          {pods.map((p) => {
            const machine = machineByHost.get(p.node)
            const expanded = open[p.name] === true
            return (
              <Fragment key={p.name}>
                <tr className="border-t align-top">
                  <td className="py-1.5">
                    <button
                      type="button"
                      aria-expanded={expanded}
                      className="flex items-center gap-1 text-left"
                      onClick={() => setOpen((o) => ({ ...o, [p.name]: !expanded }))}
                    >
                      <ChevronRight
                        aria-hidden="true"
                        className={
                          expanded
                            ? 'size-3.5 rotate-90 transition-transform'
                            : 'size-3.5 transition-transform'
                        }
                      />
                      <span className="break-all font-mono text-xs">{p.name}</span>
                    </button>
                    {p.ip && <p className="pl-5 font-mono text-muted-foreground text-xs">{p.ip}</p>}
                  </td>
                  <td className="py-1.5">
                    {machine ? (
                      <Link
                        to="/nodes/$uuid"
                        params={{ uuid: machine }}
                        className="font-mono text-xs underline-offset-2 hover:underline"
                      >
                        {p.node}
                      </Link>
                    ) : (
                      <span className="font-mono text-xs">{p.node || '—'}</span>
                    )}
                  </td>
                  <td className="py-1.5 text-xs">
                    {p.phase} · {p.ready}
                    {p.restarts > 0 && (
                      <span className="text-amber-700 dark:text-amber-300">
                        {' '}
                        · {p.restarts} restarts
                      </span>
                    )}
                  </td>
                  <td className="py-1.5 text-right tabular-nums">
                    {p.usage_known ? formatCores(p.cpu_millis) : '—'}
                  </td>
                  <td className="py-1.5 text-right tabular-nums">
                    {p.usage_known ? formatBytes(p.memory_bytes) : '—'}
                  </td>
                </tr>
                {expanded &&
                  p.containers.map((c) => (
                    <tr key={`${p.name}/${c.name}`} className="text-xs">
                      <td className="py-1 pl-5" colSpan={2}>
                        <span className="font-medium">{c.name}</span>{' '}
                        <span className="break-all font-mono text-muted-foreground">{c.image}</span>
                      </td>
                      <td className="py-1">
                        {c.state}
                        {c.restarts > 0 && ` · ${c.restarts} restarts`}
                      </td>
                      <td className="py-1 text-right tabular-nums">
                        {c.usage_known ? formatCores(c.cpu_millis) : '—'}
                        <p className="text-muted-foreground">
                          {c.cpu_limit_millis > 0
                            ? `limit ${formatCores(c.cpu_limit_millis)}`
                            : c.cpu_request_millis > 0
                              ? `req ${formatCores(c.cpu_request_millis)}`
                              : 'no request'}
                        </p>
                      </td>
                      <td className="py-1 text-right tabular-nums">
                        {c.usage_known ? formatBytes(c.memory_bytes) : '—'}
                        <p className="text-muted-foreground">
                          {c.memory_limit_bytes > 0
                            ? `limit ${formatBytes(c.memory_limit_bytes)}`
                            : c.memory_request_bytes > 0
                              ? `req ${formatBytes(c.memory_request_bytes)}`
                              : 'no request'}
                        </p>
                      </td>
                    </tr>
                  ))}
              </Fragment>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

export const appDetailRoute = createRoute({
  getParentRoute: () => kubernetesRoute,
  path: 'apps/$namespace/$kind/$name',
  component: AppDetailPage,
})
