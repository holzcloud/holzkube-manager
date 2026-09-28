import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { api, type Hardware, type Machine, type Temperature } from '@/api'
import {
  CoreList,
  MemoryChart,
  NetworkChart,
  ProcessorChart,
} from '@/components/charts/HardwareCharts'
import { LiveChart } from '@/components/charts/LiveChart'
import { Meter, severityOf } from '@/components/charts/Meter'
import { RangePicker } from '@/components/charts/RangePicker'
import { FanList, Sensors, sensorKey } from '@/components/charts/Sensors'
import { Problem } from '@/components/Problem'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { merge, useChartRange } from '@/hooks/useChartRange'
import { useLiveSeries } from '@/hooks/useLiveSeries'
import { formatBytes, formatCores, formatPercent, formatRate } from '@/lib/format'

/**
 * A node's hardware as it is right now (2026-09-26).
 *
 * The operator's model is Unraid's dashboard: one glance says how busy the
 * processor is and how hot everything runs, per core, per disk, per fan. So the
 * page asks the node every few seconds and draws what it says. Nothing here is
 * stored anywhere -- the curves are the readings this page has seen since it
 * was opened, and closing it forgets them.
 *
 * What the node cannot say is said as such. A board whose sensor chip has no
 * kernel driver reports no fans; that is shown as "not reported", never as a
 * row of zeroes, because 0 RPM is a real reading that means a fan has stopped.
 */

export const HARDWARE_POLL_INTERVAL_MS = 3_000

/** The hottest processor sensor: the package reading if there is one. */
export function cpuTemperature(h: Hardware): Temperature | undefined {
  const cpu = h.temperatures.filter((t) => t.kind === 'cpu')
  return (
    cpu.find((t) => /package|tctl|tdie/i.test(t.label)) ??
    [...cpu].sort((a, b) => b.celsius - a.celsius)[0]
  )
}

export function NodeHardware({ machine }: { machine: Machine }) {
  const hardware = useQuery({
    queryKey: ['hardware', machine.id],
    queryFn: () => api.machines.hardware(machine.id),
    refetchInterval: HARDWARE_POLL_INTERVAL_MS,
    // A node that stopped answering keeps its last picture on screen, marked
    // as old, rather than turning the whole dashboard into an error message.
    retry: false,
  })

  const h = hardware.data
  const live = useLiveSeries(`hardware:${machine.id}`, h, h?.observed_at, (r) => {
    const values: Record<string, number> = {
      cpu: r.cpu.usage_percent,
      memory: r.memory.total_bytes > 0 ? (r.memory.used_bytes / r.memory.total_bytes) * 100 : 0,
      rx: r.network.reduce((sum, n) => sum + n.rx_bytes_per_sec, 0),
      tx: r.network.reduce((sum, n) => sum + n.tx_bytes_per_sec, 0),
      read: r.disks.reduce((sum, d) => sum + d.read_bytes_per_sec, 0),
      write: r.disks.reduce((sum, d) => sum + d.write_bytes_per_sec, 0),
    }
    r.cpu.per_core.forEach((usage, i) => {
      values[`core:${i}`] = usage
    })
    for (const t of r.temperatures) values[`temp:${sensorKey(t)}`] = t.celsius
    for (const f of r.fans) values[`fan:${f.chip}/${f.label}`] = f.rpm
    return values
  })

  const chart = useChartRange(['hardware', machine.id], (range) =>
    api.machines.hardwareHistory(machine.id, range),
  )
  const now = h ? Date.parse(h.observed_at) : Date.now()
  const history = merge(chart.history.data, live, chart.windowMs, now)

  if (h === undefined) {
    if (hardware.error) {
      return (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Live hardware</CardTitle>
          </CardHeader>
          <CardContent>
            <Problem error={hardware.error} />
          </CardContent>
        </Card>
      )
    }
    return <p className="text-muted-foreground text-sm">Asking the node for its readings…</p>
  }

  const stale = hardware.error !== null
  const mem = h.memory
  const totalRead = h.disks.reduce((sum, d) => sum + d.read_bytes_per_sec, 0)
  const totalWrite = h.disks.reduce((sum, d) => sum + d.write_bytes_per_sec, 0)
  const totalRx = h.network.reduce((sum, n) => sum + n.rx_bytes_per_sec, 0)
  const totalTx = h.network.reduce((sum, n) => sum + n.tx_bytes_per_sec, 0)

  /*
   * The layout the operator chose on 2026-09-26 ("C — Verlauf zuerst"): the
   * curves are the page, because the question at a node is "what has it been
   * doing", and the sensors ride in a column of their own beside them, where a
   * temperature climbing is visible without scrolling. Below lg the column
   * drops under the curves, which on a phone is the order a person reads.
   */
  return (
    <section aria-label="Live hardware" className={stale ? 'space-y-4 opacity-60' : 'space-y-4'}>
      {stale && (
        <p className="rounded-md border border-amber-600/40 bg-amber-600/10 px-3 py-2 text-sm text-amber-700 dark:text-amber-300">
          The node did not answer the latest request. What you see is its reading from{' '}
          {new Date(h.observed_at).toLocaleTimeString()}.{' '}
          {hardware.error instanceof Error ? hardware.error.message : ''}
        </p>
      )}

      <RangePicker
        value={chart.range}
        onChange={chart.setRange}
        note={
          chart.history.error
            ? 'No recorded history yet — the curves start with this page.'
            : undefined
        }
      />

      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_20rem]">
        <div className="min-w-0 space-y-4">
          <Card>
            <CardHeader className="flex flex-row items-start justify-between gap-3">
              <div className="min-w-0">
                <CardTitle className="text-base">Processor</CardTitle>
                <p className="truncate text-muted-foreground text-sm">
                  {h.cpu.model || 'Model not reported'}
                  {h.cpu.cores > 0 && ` — ${h.cpu.cores} cores / ${h.cpu.threads} threads`}
                </p>
              </div>
              <div className="text-right">
                <p className="font-semibold text-2xl">{formatPercent(h.cpu.usage_percent)}</p>
                <p className="text-muted-foreground text-xs tabular-nums">
                  load {h.cpu.load1.toFixed(2)} · {h.cpu.load5.toFixed(2)} ·{' '}
                  {h.cpu.load15.toFixed(2)}
                </p>
              </div>
            </CardHeader>
            <CardContent className="space-y-3">
              <ProcessorChart history={history} windowMs={chart.windowMs} />
              {h.cpu.per_core.length > 0 && <CoreList perCore={h.cpu.per_core} history={history} />}
              {h.cpu.iowait_percent >= 1 && (
                <p className="text-muted-foreground text-xs">
                  {formatPercent(h.cpu.iowait_percent)} of the time spent waiting for disks.
                </p>
              )}
            </CardContent>
          </Card>

          <div className="grid gap-4 md:grid-cols-2">
            <Card>
              <CardHeader className="flex flex-row items-start justify-between gap-3">
                <div>
                  <CardTitle className="text-base">Memory</CardTitle>
                  <p className="text-muted-foreground text-sm">of {formatBytes(mem.total_bytes)}</p>
                </div>
                <p className="font-semibold text-lg">{formatBytes(mem.used_bytes)}</p>
              </CardHeader>
              <CardContent className="space-y-3">
                <MemoryChart history={history} windowMs={chart.windowMs} />
                <p className="text-muted-foreground text-xs tabular-nums">
                  {formatBytes(mem.cache_bytes)} cache · {formatBytes(mem.available_bytes)}{' '}
                  available
                  {mem.swap_total_bytes > 0 &&
                    ` · swap ${formatBytes(mem.swap_used_bytes)} of ${formatBytes(mem.swap_total_bytes)}`}
                </p>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-base">Disks</CardTitle>
              </CardHeader>
              <CardContent className="space-y-3">
                <LiveChart
                  title="Disk throughput"
                  series={[
                    { key: 'read', label: 'Read', slot: 2, points: history.read ?? [] },
                    { key: 'write', label: 'Write', slot: 1, points: history.write ?? [] },
                  ]}
                  format={formatRate}
                  windowMs={chart.windowMs}
                />
                <p className="sr-only">
                  Now: read {formatRate(totalRead)}, write {formatRate(totalWrite)}.
                </p>
              </CardContent>
            </Card>
          </div>

          <Card>
            <CardHeader>
              <CardTitle className="text-base">Network</CardTitle>
              <p className="text-muted-foreground text-sm">
                {h.network
                  .map((n) =>
                    n.up
                      ? `${n.name} ${n.speed_mbit > 0 ? `· ${n.speed_mbit} Mbit/s` : '· up'}`
                      : `${n.name} · down`,
                  )
                  .join('   ')}
              </p>
            </CardHeader>
            <CardContent>
              <NetworkChart history={history} windowMs={chart.windowMs} />
              <p className="sr-only">
                Now: in {formatRate(totalRx)}, out {formatRate(totalTx)}.
              </p>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-base">Storage</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              <ul className="space-y-2">
                {h.disks.map((d) => (
                  <li
                    key={d.name}
                    className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1 text-sm"
                  >
                    <span className="min-w-0">
                      <span className="font-mono">{d.name}</span>
                      {d.model && <span className="text-muted-foreground"> {d.model}</span>}
                      <span className="text-muted-foreground text-xs">
                        {' '}
                        · {formatBytes(d.size_bytes)}
                      </span>
                    </span>
                    <span className="text-xs tabular-nums">
                      <span className="text-muted-foreground">read </span>
                      {formatRate(d.read_bytes_per_sec)}
                      <span className="text-muted-foreground"> · write </span>
                      {formatRate(d.write_bytes_per_sec)}
                      <span className="text-muted-foreground"> · </span>
                      {d.temperature_c === null ? (
                        <span className="text-muted-foreground">temperature not reported</span>
                      ) : (
                        <TemperatureFigure
                          celsius={d.temperature_c}
                          warn={d.temperature_warn_c ?? undefined}
                          danger={d.temperature_danger_c ?? undefined}
                        />
                      )}
                    </span>
                  </li>
                ))}
              </ul>
              {h.filesystems.length > 0 && (
                <div className="grid gap-x-6 gap-y-2 md:grid-cols-2">
                  {h.filesystems.map((f) => (
                    <Meter
                      key={`${f.device}${f.mount}`}
                      dense
                      label={
                        <>
                          <span className="font-mono">{f.mount}</span>
                          <span className="text-muted-foreground"> {f.device}</span>
                        </>
                      }
                      display={
                        f.size_bytes > 0 ? formatPercent((f.used_bytes / f.size_bytes) * 100) : '—'
                      }
                      value={f.used_bytes}
                      max={f.size_bytes}
                      warn={f.size_bytes * 0.8}
                      danger={f.size_bytes * 0.9}
                      detail={`${formatBytes(f.used_bytes)} of ${formatBytes(f.size_bytes)}`}
                    />
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </div>

        <Card className="h-fit">
          <CardHeader>
            <CardTitle className="text-base">Sensors</CardTitle>
            <p className="text-muted-foreground text-sm">Temperatures and fans, live</p>
          </CardHeader>
          <CardContent className="space-y-4">
            <Sensors
              temperatures={h.temperatures}
              history={history}
              emptyText="This node reports no temperature sensors."
            />
            {h.sensors_notice !== '' && (
              <p className="text-muted-foreground text-xs">{h.sensors_notice}</p>
            )}
            <div>
              <p className="mb-1 text-muted-foreground text-xs">Fans</p>
              <FanList
                fans={h.fans}
                empty={
                  <p className="text-muted-foreground text-sm">
                    This node reports no fans. That usually means its fan controller has no driver
                    in the Talos kernel — not that it has none.
                  </p>
                }
              />
            </div>
          </CardContent>
        </Card>
      </div>

      <NodeApps machine={machine} />

      <p className="text-muted-foreground text-xs">
        Read from the node {new Date(h.observed_at).toLocaleTimeString()}, every{' '}
        {HARDWARE_POLL_INTERVAL_MS / 1000} s while this page is open. Rates are over the last{' '}
        {h.rates_over_seconds.toFixed(1)} s.
      </p>
    </section>
  )
}

function TemperatureFigure({
  celsius,
  warn,
  danger,
}: {
  celsius: number
  /** The server's lines; a missing one is no line, never a number made up here. */
  warn?: number
  danger?: number
}) {
  const severity = severityOf(celsius, warn, danger)
  return (
    <span data-severity={severity}>
      {celsius.toFixed(0)} °C
      {severity !== 'ok' && (
        <span className="text-[color:var(--viz-danger)]">
          {severity === 'danger' ? ' — critical' : ' — high'}
        </span>
      )}
    </span>
  )
}

/**
 * What runs on this node and what it uses here -- the Kubernetes half of the
 * same question, read from this node's kubelet.
 */
function NodeApps({ machine }: { machine: Machine }) {
  const node = machine.hostname.value ?? ''
  const apps = useQuery({
    queryKey: ['kubernetes', 'apps', machine.cluster, { node }],
    queryFn: () => api.kubernetes.apps(machine.cluster, { node }),
    enabled: machine.cluster !== '' && node !== '',
    refetchInterval: 5_000,
    retry: false,
  })

  if (machine.cluster === '' || node === '') return null

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">What runs here</CardTitle>
        <p className="text-muted-foreground text-sm">
          Apps with a pod on this node, and what those pods use on it right now.
        </p>
      </CardHeader>
      <CardContent>
        {apps.error ? (
          <Problem error={apps.error} />
        ) : apps.data === undefined ? (
          <p className="text-muted-foreground text-sm">Asking the kubelet…</p>
        ) : apps.data.apps.length === 0 ? (
          <p className="text-muted-foreground text-sm">No pods run on this node.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-muted-foreground text-xs">
                  <th className="py-1 font-normal">App</th>
                  <th className="py-1 font-normal">Pods</th>
                  <th className="py-1 text-right font-normal">CPU</th>
                  <th className="py-1 text-right font-normal">Memory</th>
                </tr>
              </thead>
              <tbody>
                {apps.data.apps.map((a) => (
                  <tr key={`${a.namespace}/${a.kind}/${a.name}`} className="border-t">
                    <td className="py-1.5">
                      <Link
                        to="/kubernetes/apps/$namespace/$kind/$name"
                        params={{ namespace: a.namespace, kind: a.kind, name: a.name }}
                        search={{ cluster: machine.cluster }}
                        className="underline-offset-2 hover:underline"
                      >
                        {a.name}
                      </Link>
                      <span className="text-muted-foreground text-xs"> {a.namespace}</span>
                    </td>
                    <td className="py-1.5 tabular-nums">
                      {a.ready}/{a.pods}
                    </td>
                    <td className="py-1.5 text-right tabular-nums">
                      {a.usage_known ? formatCores(a.cpu_millis) : '—'}
                    </td>
                    <td className="py-1.5 text-right tabular-nums">
                      {a.usage_known ? formatBytes(a.memory_bytes) : '—'}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            {apps.data.notice !== '' && (
              <p className="mt-2 text-muted-foreground text-xs">{apps.data.notice}</p>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
