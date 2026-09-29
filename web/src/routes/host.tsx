import { useQuery } from '@tanstack/react-query'
import { createRoute } from '@tanstack/react-router'
import { AlertTriangle } from 'lucide-react'
import { type ReactNode, useState } from 'react'
import { api, type Host, type HostHealth, type HostOrder, type Link, type Reason } from '@/api'
import {
  CoreList,
  MemoryChart,
  NetworkChart,
  ProcessorChart,
} from '@/components/charts/HardwareCharts'
import { Meter } from '@/components/charts/Meter'
import { RangePicker } from '@/components/charts/RangePicker'
import { FanList, Sensors, sensorKey } from '@/components/charts/Sensors'
import { ago } from '@/components/HealthField'
import { HostActions, HostOrderStatus, orderPhase } from '@/components/HostActions'
import { HostStateMark, STATE_WORD } from '@/components/HostState'
import { HARDWARE_POLL_INTERVAL_MS } from '@/components/NodeHardware'
import { Problem } from '@/components/Problem'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { merge, useChartRange } from '@/hooks/useChartRange'
import { type Series, useLiveSeries } from '@/hooks/useLiveSeries'
import { useSession } from '@/hooks/useSession'
import { formatBytes, formatPercent, formatRate, formatUptime } from '@/lib/format'
import { authenticatedRoute } from '@/routes/__root'

/**
 * The machine holzkube-manager itself runs on (Phase 11, HOST-01).
 *
 * Every other page in this product is about the cluster; this one is about the
 * box beside it -- on the operator's installation a Raspberry Pi. It is read
 * live, every 3 s while the page is open, and since Phase 12 the daemon also
 * records it every 15 s under host/local and keeps 24 hours of it, through a
 * restart; the curves are that record followed by this page's own readings,
 * drawn by the node page's own chart blocks (D-05).
 *
 * The rule the whole page is built around (D-02): a value the daemon could not
 * read is drawn as "Not readable" with the reason, and never as a zero or an
 * empty cell. The production unit hides /proc/stat and /proc/meminfo from the
 * daemon, so an unread value is an everyday state here, not an error -- and a
 * 0 % that is really "could not look" is the one lie this page must not tell.
 * On a curve that rule reads: an unread value is no point at all, so a stretch
 * without readings is a gap, and a value never read in the window gets a
 * sentence instead of an empty axis (HistorySlot).
 * The schema makes the renderer branch on `readable` before it can format
 * anything; MissingValue is the only way a missing value is drawn.
 */

type Reading<T> = { readable: true; value: T } | { readable: false; reason: Reason }

export function HostPage() {
  // The role decides only whether the buttons are offered (D-16); the action
  // routes' operator floor is the lock.
  const role = useSession().me?.role
  const query = useQuery({
    queryKey: ['host'],
    queryFn: api.host,
    refetchInterval: HARDWARE_POLL_INTERVAL_MS,
    // A failed poll keeps the last reading on screen, marked as old, rather
    // than replacing the page with an error (as the node hardware does).
    retry: false,
  })

  if (query.data === undefined) {
    if (query.error) {
      return (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Host</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            <p className="text-sm">The host readings could not be loaded.</p>
            <Problem error={query.error} />
            <p className="text-sm text-muted-foreground">This page keeps asking every 3 s.</p>
          </CardContent>
        </Card>
      )
    }
    return <p className="text-sm text-muted-foreground">Reading the host…</p>
  }

  return <HostView host={query.data} stale={query.error ?? null} sessionRole={role} />
}

export function HostView({
  host,
  stale,
  sessionRole,
}: {
  host: Host
  stale: unknown
  /** The session's role; without one the host actions are not offered. */
  sessionRole?: string
}) {
  const isStale = stale !== null && stale !== undefined
  const observed = new Date(host.observed_at)
  const d = host.device
  const hidden = hiddenByHardening(host)
  const load = host.live.cpu.load
  const rates = host.live.rates_over_seconds
  const health = host.health
  // The order this page placed, held until the page is left: the status box
  // follows it through the host answer's order and result (Phase 13).
  const [held, setHeld] = useState<HostOrder | null>(null)

  // The charts are the node page's (D-05): the daemon's recorded history for
  // the range, with this page's own readings appended after its last point.
  const chart = useChartRange(['host'], api.hostHistory)
  const live = useLiveSeries('host', host, host.observed_at, hostHistoryValues)
  const history = merge(chart.history.data, live, chart.windowMs, Date.parse(host.observed_at))

  return (
    <section className="space-y-5">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="font-heading text-xl font-semibold tracking-tight">Host</h1>
          <p className="text-sm text-muted-foreground">
            The machine holzkube-manager runs on, and the service itself.
          </p>
          {/* The server's state (D-06, D-10), drawn as it came. Only the word
              is live, so a temperature that changes every 3 s is not
              re-announced, but a change of state is. Not dimmed when stale:
              the stale notice below says how old it is. */}
          <p className="mt-2 flex flex-wrap items-center gap-x-2 gap-y-1 text-sm">
            <HostStateMark state={health.state} summary={health.summary} form="line" />
            <span aria-live="polite" className="font-semibold">
              {STATE_WORD[health.state]}
            </span>
            <span className="text-muted-foreground">{health.summary}</span>
          </p>
        </div>
        {/* The slot Phase 11 left for the host actions. */}
        <HostActions
          host={host}
          sessionRole={sessionRole}
          pollFailed={isStale}
          order={held === null ? null : { action: held.action, phase: orderPhase(held, host) }}
          onPlaced={setHeld}
        />
      </header>

      {/* Notices, in this order when they apply: the order this page placed,
          stale, warning, container, hardening. The order comes first: it is
          what the operator just did. Stale then says the rest may be old. */}
      {held !== null && <HostOrderStatus host={host} held={held} />}
      {isStale && (
        <p className="rounded-md border border-amber-600/40 bg-amber-600/10 px-3 py-2 text-sm text-amber-700 dark:text-amber-300">
          holzkube-manager did not answer the latest request. What you see is its reading from{' '}
          {observed.toLocaleTimeString()}. {stale instanceof Error ? stale.message : ''}
        </p>
      )}
      {health.state === 'warn' && <WarningNotice health={health} />}
      {host.container && (
        <p className="rounded-md border border-slate-500/40 bg-slate-500/10 px-3 py-2 text-sm text-slate-700 dark:text-slate-300">
          holzkube-manager runs in a container. Kernel, CPU, memory and temperatures are the host's;
          hostname, network and filesystems are the container's.
        </p>
      )}
      {hidden.length > 0 && (
        <div className="rounded-md border border-slate-500/40 bg-slate-500/10 px-3 py-2 text-sm text-slate-700 dark:text-slate-300">
          <p>
            <strong className="font-semibold">
              Some readings are hidden by the service's hardening.
            </strong>{' '}
            The systemd unit sets <code className="font-mono">ProcSubset=pid</code>, which hides
            /proc/stat and /proc/meminfo from holzkube-manager, so {joinList(hidden)}{' '}
            {hidden.length === 1 ? 'is' : 'are'} shown as not readable rather than as zero, and{' '}
            {hidden.length === 1 ? 'is' : 'are'} not recorded.
            {load.readable && load.value.source === 'sysinfo' && (
              <> Load is still exact; it comes from sysinfo(2).</>
            )}{' '}
            To show them, set this line in the unit's [Service] section, then reload systemd and
            restart the service:
          </p>
          <code className="mt-2 block overflow-x-auto rounded-sm bg-muted px-2 py-1 font-mono text-xs">
            ProcSubset=all
          </code>
          <p className="mt-2">
            <code className="font-mono">ProtectProc=invisible</code> can stay: other processes
            remain hidden either way.
          </p>
        </div>
      )}

      <div className={isStale ? 'space-y-5 opacity-60' : 'space-y-5'}>
        <div className="grid gap-4 lg:grid-cols-2">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Device</CardTitle>
            </CardHeader>
            <CardContent>
              <dl className="grid grid-cols-1 md:grid-cols-[10rem_minmax(0,1fr)] gap-x-4 gap-y-2 text-sm">
                <Row
                  label="Hostname"
                  reading={d.hostname}
                  render={(v) => (
                    <>
                      <Mono>{v}</Mono>
                      {host.container && (
                        <Badge variant="outline" className="ml-2">
                          container
                        </Badge>
                      )}
                    </>
                  )}
                />
                <Row label="Model" reading={d.model} render={(v) => v} />
                <Row
                  label="Architecture"
                  reading={d.arch}
                  render={(v) => <Mono>{`${v.goarch} (${v.machine})`}</Mono>}
                />
                <Row label="Processor cores" reading={d.cores} render={(v) => `${v} online`} />
                <Row label="Operating system" reading={d.os} render={(v) => v} />
                <Row label="Kernel" reading={d.kernel} render={(v) => <Mono>{v}</Mono>} />
                <Row
                  label="Up since boot"
                  reading={d.uptime_seconds}
                  render={(v) => (
                    <>
                      {formatUptime(v)}{' '}
                      <span className="text-xs text-muted-foreground">
                        since {new Date(observed.getTime() - v * 1000).toLocaleString()}
                      </span>
                    </>
                  )}
                />
              </dl>
            </CardContent>
          </Card>
          <ServiceCard host={host} observed={observed} />
        </div>

        <LiveSection host={host} chart={chart} history={history} />

        <p className="text-xs text-muted-foreground">
          Read {observed.toLocaleTimeString()}, every 3 s while this page is open.
          {rates !== null && ` Rates are over the last ${rates.toFixed(1)} s.`}
          {' History is sampled every 15 s.'}
        </p>
      </div>
    </section>
  )
}

const DATA_DIR_ROLE = 'data directory'

/**
 * holzkube-manager itself (HOST-02, HOST-03): which version runs, since when,
 * where its state lives and how much room is left there, and what the update
 * script last recorded -- or that it records nothing.
 */
/**
 * What is wrong and by how much (HMON-07, D-09): every warning sentence the
 * server sent, in the server's order -- worst first -- and never re-sorted or
 * re-worded here. The per-value marks (the sensor ▲, the filesystem meter's
 * amber) hang on the same lines the server judged by, so they agree with this
 * list rather than repeat a rule of the browser's own.
 */
function WarningNotice({ health }: { health: HostHealth }) {
  const n = health.warnings.length
  return (
    <div className="rounded-md border border-amber-600/40 bg-amber-600/10 px-3 py-2 text-sm text-amber-700 dark:text-amber-300">
      <p className="flex items-center gap-2 font-semibold">
        <AlertTriangle aria-hidden="true" className="size-4 shrink-0" />
        Warning — {n} {n === 1 ? 'threshold' : 'thresholds'} crossed
      </p>
      <ul className="mt-1 space-y-1 break-words tabular-nums">
        {health.warnings.map((w, i) => (
          // biome-ignore lint/suspicious/noArrayIndexKey: the server's list, in its order; sentences may repeat
          <li key={i}>{w}</li>
        ))}
      </ul>
      {health.unreadable.length > 0 && (
        <p className="mt-1 break-words">Also not readable: {health.unreadable.join(' ')}</p>
      )}
      <p className="mt-1 text-xs">
        A temperature warns at its chip's own limit, or at its thermal zone's trip point, or at a
        default for its kind (80 °C for a processor) when neither names one; a filesystem warns at
        80% used.
      </p>
    </div>
  )
}

function ServiceCard({ host, observed }: { host: Host; observed: Date }) {
  const s = host.service
  const size = s.data_dir.size
  const dataFs = host.live.filesystems.find((f) => f.roles.includes(DATA_DIR_ROLE))
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Service</CardTitle>
      </CardHeader>
      <CardContent>
        <dl className="grid grid-cols-1 md:grid-cols-[10rem_minmax(0,1fr)] gap-x-4 gap-y-2 text-sm">
          <dt className="text-muted-foreground">Version</dt>
          <dd className="min-w-0 break-words">
            <Mono>{s.version}</Mono>
          </dd>

          <dt className="text-muted-foreground">Running for</dt>
          <dd className="min-w-0 break-words">
            {formatUptime(s.uptime_seconds)}{' '}
            <span className="text-xs text-muted-foreground">
              since {new Date(s.started_at).toLocaleString()}
            </span>
          </dd>

          <dt className="text-muted-foreground">Data directory</dt>
          <dd className="min-w-0 break-words">
            <Mono>{s.data_dir.path}</Mono>
            {/* The size is walked at most once a minute, so its age is said
                rather than implied current. */}
            {size.readable ? (
              <p className="text-xs text-muted-foreground tabular-nums">
                {formatBytes(size.value.bytes)} · measured {ago(size.value.measured_at, observed)}
              </p>
            ) : (
              <MissingValue reason={size.reason} />
            )}
          </dd>

          <dt className="text-muted-foreground">Free space</dt>
          <dd className="min-w-0 break-words">
            {dataFs === undefined ? (
              <MissingValue
                reason={{
                  code: 'read-failed',
                  message: 'The answer names no filesystem for the data directory.',
                }}
              />
            ) : dataFs.usage.readable ? (
              <span className="tabular-nums">
                {formatBytes(dataFs.usage.value.available_bytes)} free of{' '}
                {formatBytes(dataFs.usage.value.size_bytes)} on <Mono>{dataFs.mount}</Mono>
              </span>
            ) : (
              <MissingValue reason={dataFs.usage.reason} />
            )}
          </dd>

          <dt className="text-muted-foreground">Update check</dt>
          <dd className="min-w-0 break-words">
            <UpdateCheck update={s.update} version={s.version} observed={observed} />
          </dd>
        </dl>
      </CardContent>
    </Card>
  )
}

type UpdateStatus = Extract<Host['service']['update'], { readable: true }>['value']

/**
 * The update row (HOST-03, D-16): what the script recorded, or the server's
 * sentence for why there is nothing. Every time and version on it is one the
 * status file named; a half the script could not know is left out, not filled.
 */
function UpdateCheck({
  update,
  version,
  observed,
}: {
  update: Host['service']['update']
  version: string
  observed: Date
}) {
  if (!update.readable) {
    return <MissingValue reason={update.reason} />
  }
  const u = update.value
  const attention = u.outcome === 'failed' || u.outcome === 'rolled-back'
  const mismatch = u.installed !== null && bare(u.installed) !== bare(version)
  return (
    <>
      <p className={attention ? 'text-amber-700 dark:text-amber-300' : undefined}>
        {outcomeSentence(u)}
      </p>
      <p className="text-xs text-muted-foreground">
        Checked {ago(u.checked_at, observed)} · {new Date(u.checked_at).toLocaleString()}
      </p>
      {(u.installed !== null || u.latest !== null) && (
        <p className="text-xs text-muted-foreground">
          {u.installed !== null && (
            <>
              Installed <Mono>{u.installed}</Mono>
            </>
          )}
          {u.installed !== null && u.latest !== null && ' · '}
          {u.latest !== null && (
            <>
              latest <Mono>{u.latest}</Mono>
            </>
          )}
        </p>
      )}
      {mismatch && (
        <p className="text-xs text-muted-foreground">
          The update status names {u.installed} as installed; this process runs {version}.
        </p>
      )}
    </>
  )
}

function outcomeSentence(u: UpdateStatus): string {
  switch (u.outcome) {
    case 'current':
      return 'Up to date.'
    case 'available':
      return u.latest !== null ? `${u.latest} is available.` : 'A newer release is available.'
    case 'updated':
      return u.installed !== null ? `Updated to ${u.installed}.` : 'Updated.'
    case 'rolled-back':
      return u.latest !== null && u.installed !== null
        ? `The update to ${u.latest} was rolled back; ${u.installed} is installed.`
        : 'The update was rolled back.'
    case 'failed':
      return u.installed !== null
        ? `The last update run failed; ${u.installed} is still installed.`
        : 'The last update run failed.'
  }
}

/** A version without its tag's leading "v": v0.1.0 and 0.1.0 are one release. */
function bare(v: string): string {
  return v.startsWith('v') ? v.slice(1) : v
}

/**
 * / and the data directory's filesystem (HMON-02, D-07). One meter per
 * filesystem: when the data directory lives on the root filesystem -- as on the
 * production Pi -- that is one meter with both roles, never two identical bars.
 */
function FilesystemsCard({ filesystems }: { filesystems: Host['live']['filesystems'] }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Filesystems</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {filesystems.length === 0 && (
          <p className="text-sm text-muted-foreground">No filesystem was reported.</p>
        )}
        {filesystems.map((f) => {
          const label = (
            <>
              <span className="font-mono">{f.mount}</span>
              {f.roles.length > 0 && (
                <span className="text-muted-foreground"> {f.roles.join(' · ')}</span>
              )}
            </>
          )
          if (!f.usage.readable) {
            return (
              <div key={f.mount} className="space-y-1">
                <p className="text-sm">{label}</p>
                <MissingValue reason={f.usage.reason} />
              </div>
            )
          }
          const { used_bytes: used, available_bytes: free } = f.usage.value
          // df's denominator: blocks reserved for root are neither used nor
          // available, so used / size would read lower than df's Use%. The
          // figure, the bar, the caption and the server's 80 % rule all divide
          // by this one number, so the caption names it ("usable").
          const ceiling = used + free
          return (
            <Meter
              key={f.mount}
              dense
              label={label}
              display={dfPercent(used, free)}
              value={used}
              max={ceiling}
              warn={ceiling * 0.8}
              danger={ceiling * 0.9}
              detail={
                <>
                  {formatBytes(used)} of {formatBytes(ceiling)} usable · {formatBytes(free)} free
                  {f.device !== '' && (
                    <>
                      {' · '}
                      <span className="font-mono">{f.device}</span>
                    </>
                  )}
                </>
              }
            />
          )
        })}
      </CardContent>
    </Card>
  )
}

/**
 * df(1)'s Use%: used / (used + available), rounded UP to a whole percent --
 * gnulib's df adds one whenever the division leaves a remainder. The figure on
 * this page is the one the operator's shell prints for the same filesystem.
 */
function dfPercent(used: number, available: number): string {
  const total = used + available
  if (!(total > 0)) return '—'
  return `${Math.ceil((used * 100) / total)}%`
}

/**
 * The Readings block (HMON-01, HMON-05): the figures and meters of the moment,
 * and over them the curves a node has -- processor, memory, network and the
 * sensors' sparklines -- over the range the RangePicker chose.
 */
function LiveSection({
  host,
  chart,
  history,
}: {
  host: Host
  chart: ReturnType<typeof useChartRange>
  history: Series
}) {
  return (
    <section aria-labelledby="host-readings" className="space-y-4">
      <div>
        <h2 id="host-readings" className="text-base font-semibold">
          Readings
        </h2>
        <p className="text-sm text-muted-foreground">
          Read every 3 s while this page is open. holzkube-manager also records them every 15 s and
          keeps the last 24 hours, through a restart.
        </p>
      </div>
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
          <ProcessorCard host={host} history={history} windowMs={chart.windowMs} />
          <div className="grid gap-4 md:grid-cols-2">
            <MemoryCard memory={host.live.memory} history={history} windowMs={chart.windowMs} />
            <FilesystemsCard filesystems={host.live.filesystems} />
          </div>
          <NetworkCard
            network={host.live.network}
            container={host.container}
            waiting={host.live.rates_over_seconds === null}
            history={history}
            windowMs={chart.windowMs}
          />
        </div>
        <SensorsCard sensors={host.live.sensors} history={history} />
      </div>
    </section>
  )
}

/**
 * The interfaces (HMON-04, D-09): every physical one as a row with its state,
 * speed and throughput; the virtual ones -- loopback, bridges, veths -- behind
 * one disclosure, counted rather than dropped. A rate the server did not
 * compute is "—": formatRate(0) would say "0 B/s", which is an idle link, not a
 * missing reading (D-02).
 */
function NetworkCard({
  network,
  container,
  waiting,
  history,
  windowMs,
}: {
  network: Host['live']['network']
  container: boolean
  waiting: boolean
  history: Series
  windowMs: number
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">
          Network
          {container && (
            <Badge variant="outline" className="ml-2">
              container
            </Badge>
          )}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <HistorySlot reading={network} points={pointsOf(history, 'rx', 'tx')}>
          <NetworkChart history={history} windowMs={windowMs} />
        </HistorySlot>
        {network.readable ? (
          <>
            {network.value.physical.length > 0 ? (
              <ul className="divide-y">
                {network.value.physical.map((l) => (
                  <LinkRow key={l.name} link={l} />
                ))}
              </ul>
            ) : (
              <p className="text-sm text-muted-foreground">No physical network interface found.</p>
            )}
            {network.value.virtual.length > 0 && (
              <details className="group">
                {/* min-h-11 is 44px at every width: the summary is the one
                    control this page has, and it has to fit a thumb. */}
                <summary className="flex min-h-11 cursor-pointer list-none items-center gap-2 text-sm text-muted-foreground marker:content-none">
                  <span
                    aria-hidden="true"
                    className="shrink-0 text-xs transition-transform group-open:rotate-90"
                  >
                    ▶
                  </span>
                  {network.value.virtual.length}{' '}
                  {network.value.virtual.length === 1 ? 'virtual interface' : 'virtual interfaces'}
                </summary>
                <ul className="divide-y">
                  {network.value.virtual.map((l) => (
                    <LinkRow key={l.name} link={l} />
                  ))}
                </ul>
              </details>
            )}
            {waiting && (
              <p className="text-xs text-muted-foreground">Waiting for a second reading</p>
            )}
          </>
        ) : (
          <MissingValue reason={network.reason} />
        )}
      </CardContent>
    </Card>
  )
}

function LinkRow({ link }: { link: Link }) {
  const state =
    link.up === null
      ? ' · state not readable'
      : !link.up
        ? ' · down'
        : link.speed_mbit !== null
          ? ` · up · ${link.speed_mbit} Mbit/s`
          : ' · up'
  return (
    <li className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1 py-2 text-sm">
      <span className="min-w-0 break-words">
        <span className="font-mono">{link.name}</span>
        <span className="text-muted-foreground">{state}</span>
      </span>
      <span className="text-xs tabular-nums">
        <span className="text-muted-foreground">in </span>
        {rateOrDash(link.rx_bytes_per_sec)}
        <span className="text-muted-foreground"> · out </span>
        {rateOrDash(link.tx_bytes_per_sec)}
      </span>
    </li>
  )
}

/** A rate, or "—" when this round has none. Branches before formatting. */
function rateOrDash(bytesPerSecond: number | null): string {
  return bytesPerSecond === null ? '—' : formatRate(bytesPerSecond)
}

/**
 * The host's temperatures and fans (HMON-03), with the node page's own list and
 * rows (D-13), and its sparkline column: the recorded history of each sensor
 * spliced with this page's readings. Voltages are never read, so never shown
 * (D-08).
 */
function SensorsCard({ sensors, history }: { sensors: Host['live']['sensors']; history: Series }) {
  return (
    <Card className="h-fit">
      <CardHeader>
        <CardTitle className="text-base">Sensors</CardTitle>
        <p className="text-sm text-muted-foreground">Temperatures and fans</p>
      </CardHeader>
      <CardContent className="space-y-4">
        {sensors.readable ? (
          <>
            <Sensors
              temperatures={sensors.value.temperatures}
              history={history}
              emptyText="This machine reports no temperature sensors."
            />
            <div>
              <p className="mb-1 text-xs text-muted-foreground">Fans</p>
              <FanList
                fans={sensors.value.fans}
                empty={
                  <p className="text-sm text-muted-foreground">
                    No fan reported. Nothing under /sys/class/hwmon on this machine has a fan speed
                    input — either there is no fan, or its controller has no driver.
                  </p>
                }
              />
            </div>
          </>
        ) : (
          <MissingValue reason={sensors.reason} />
        )}
      </CardContent>
    </Card>
  )
}

function ProcessorCard({
  host,
  history,
  windowMs,
}: {
  host: Host
  history: Series
  windowMs: number
}) {
  const { usage, per_core: perCore, load } = host.live.cpu
  const cores = host.device.cores
  return (
    <Card>
      <CardHeader className="flex flex-row items-start justify-between gap-3">
        <div className="min-w-0">
          <CardTitle className="text-base">Processor</CardTitle>
          {cores.readable && (
            <p className="truncate text-sm text-muted-foreground">
              {cores.value === 1 ? '1 core' : `${cores.value} cores`}
            </p>
          )}
        </div>
        {/* One strong figure where the eye lands, in every state: usage when it
            was read; otherwise the 1-minute load, which survives the hardening
            (it comes from sysinfo(2)), with usage said once below it, muted. */}
        <div className="text-right">
          {usage.readable ? (
            <>
              <p className="font-semibold text-xl tabular-nums">{formatPercent(usage.value)}</p>
              {load.readable ? (
                <p className="text-xs text-muted-foreground tabular-nums">
                  load {load.value.load1.toFixed(2)} · {load.value.load5.toFixed(2)} ·{' '}
                  {load.value.load15.toFixed(2)}
                </p>
              ) : (
                <MissingValue reason={load.reason} align="right" />
              )}
            </>
          ) : load.readable ? (
            <>
              <p className="font-semibold text-xl tabular-nums">
                load {load.value.load1.toFixed(2)}
              </p>
              <p className="text-xs text-muted-foreground tabular-nums">
                5 min {load.value.load5.toFixed(2)} · 15 min {load.value.load15.toFixed(2)}
              </p>
              <MissingValue reason={usage.reason} align="right" />
            </>
          ) : (
            <>
              <MissingValue reason={usage.reason} align="right" />
              <MissingValue reason={load.reason} align="right" />
            </>
          )}
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        <HistorySlot reading={usage} points={pointsOf(history, 'cpu')}>
          <ProcessorChart history={history} windowMs={windowMs} />
        </HistorySlot>
        {/* The node page's per-core list, only for numbers that were read: an
            unread or not yet computed core is left out, never drawn as 0. */}
        {perCore.readable && perCore.value.length > 0 && (
          <CoreList perCore={perCore.value} history={history} />
        )}
      </CardContent>
    </Card>
  )
}

function MemoryCard({
  memory,
  history,
  windowMs,
}: {
  memory: Host['live']['memory']
  history: Series
  windowMs: number
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Memory</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <HistorySlot reading={memory} points={pointsOf(history, 'memory')}>
          <MemoryChart history={history} windowMs={windowMs} />
        </HistorySlot>
        {memory.readable ? (
          <>
            <Meter
              label="Memory"
              display={formatPercent(share(memory.value.used_bytes, memory.value.total_bytes))}
              value={memory.value.used_bytes}
              max={memory.value.total_bytes}
              detail={`${formatBytes(memory.value.used_bytes)} of ${formatBytes(memory.value.total_bytes)} · ${formatBytes(memory.value.cache_bytes)} cache · ${formatBytes(memory.value.available_bytes)} available`}
            />
            {memory.value.swap_total_bytes > 0 ? (
              <Meter
                label="Swap"
                display={formatPercent(
                  share(memory.value.swap_used_bytes, memory.value.swap_total_bytes),
                )}
                value={memory.value.swap_used_bytes}
                max={memory.value.swap_total_bytes}
                detail={`${formatBytes(memory.value.swap_used_bytes)} of ${formatBytes(memory.value.swap_total_bytes)}`}
              />
            ) : (
              <p className="text-sm text-muted-foreground">No swap configured.</p>
            )}
          </>
        ) : (
          // Memory and swap come from the same file: one reason covers both.
          <MissingValue reason={memory.reason} />
        )}
      </CardContent>
    </Card>
  )
}

/** used as a percentage of total; NaN (drawn as "—") for a total of 0. */
function share(used: number, total: number): number {
  return total > 0 ? (used / total) * 100 : Number.NaN
}

const HARDENING = 'hardening.proc-subset'

/**
 * The readings the hardening hid, named from the response rather than from a
 * fixed list, in the order the page shows them.
 */
function hiddenByHardening(host: Host): string[] {
  const byHardening = (r: Reading<unknown>) => !r.readable && r.reason.code === HARDENING
  const { cpu, memory } = host.live
  const names: string[] = []
  if (byHardening(cpu.usage) || byHardening(cpu.per_core)) names.push('CPU usage')
  if (byHardening(cpu.load)) names.push('load')
  if (byHardening(memory)) names.push('memory', 'swap')
  return names
}

/** "A", "A and B", "A, B and C". */
function joinList(items: string[]): string {
  if (items.length <= 1) return items.join('')
  return `${items.slice(0, -1).join(', ')} and ${items[items.length - 1]}`
}

function Mono({ children }: { children: ReactNode }) {
  return <span className="font-mono">{children}</span>
}

/**
 * One row of a host card's definition list. The value is rendered only when it
 * was read; otherwise the row says why not.
 */
function Row<T>({
  label,
  reading,
  render,
}: {
  label: string
  reading: Reading<T>
  render: (value: T) => ReactNode
}) {
  return (
    <>
      <dt className="text-muted-foreground">{label}</dt>
      {/* min-w-0 and break-words: a hostname, a kernel release or a board name
          is one long word as far as the browser is concerned, and at 390 px it
          has to wrap rather than widen the page. */}
      <dd className="min-w-0 break-words">
        {reading.readable ? render(reading.value) : <MissingValue reason={reading.reason} />}
      </dd>
    </>
  )
}

const NO_BASELINE = 'rate.no-baseline'

/**
 * A chart slot, or the sentence that there is nothing to chart (D-05).
 *
 * A value that is not readable now and has no point in the chosen window has
 * no curve: an empty axis would look like a chart of nothing happening, and a
 * line at 0 would be the lie HMON-06 forbids. So the slot says "Not readable —
 * no history" and reserves no box. Anything else is the chart, whose own
 * segments() draws a stretch without points as a gap. A rate still waiting for
 * its second reading (rate.no-baseline) is pending, not unreadable: its chart
 * is drawn and says the curve starts now.
 */
function HistorySlot({
  reading,
  points,
  children,
}: {
  reading: Reading<unknown>
  points: number
  children: ReactNode
}) {
  const unreadable = !reading.readable && reading.reason.code !== NO_BASELINE
  if (unreadable && points === 0) {
    return <p className="text-xs text-muted-foreground">Not readable — no history</p>
  }
  return children
}

/** How many points the keys have in the merged, window-cut series. */
function pointsOf(history: Series, ...keys: string[]): number {
  return keys.reduce((sum, key) => sum + (history[key]?.length ?? 0), 0)
}

/**
 * The page's own readings as history values: the keys the daemon records under
 * host/local (host.HistoryValues), so the two splice. A value that was not read
 * writes no key -- never a 0 -- and the curve has a gap there.
 */
function hostHistoryValues(host: Host): Record<string, number> {
  const values: Record<string, number> = {}
  const { cpu, memory, network } = host.live
  if (cpu.usage.readable) values.cpu = cpu.usage.value
  if (cpu.per_core.readable) {
    cpu.per_core.value.forEach((usage, i) => {
      values[`core:${i}`] = usage
    })
  }
  if (memory.readable && memory.value.total_bytes > 0) {
    values.memory = (memory.value.used_bytes / memory.value.total_bytes) * 100
  }
  // Physical interfaces only, and only when every one of them has both rates
  // this round: a sum over some links would be a dip that did not happen.
  const links = network.readable ? network.value.physical : []
  if (host.live.rates_over_seconds !== null && links.length > 0) {
    let rx = 0
    let tx = 0
    let rated = true
    for (const l of links) {
      if (l.rx_bytes_per_sec === null || l.tx_bytes_per_sec === null) rated = false
      else {
        rx += l.rx_bytes_per_sec
        tx += l.tx_bytes_per_sec
      }
    }
    if (rated) {
      values.rx = rx
      values.tx = tx
    }
  }
  const sensors = host.live.sensors
  if (sensors.readable) {
    for (const t of sensors.value.temperatures) values[`temp:${sensorKey(t)}`] = t.celsius
    for (const f of sensors.value.fans) values[`fan:${f.chip}/${f.label}`] = f.rpm
  }
  return values
}

/**
 * The one way a missing value is drawn on /host.
 *
 * Three shapes, by reason code: "Waiting for a second reading" for a rate with
 * no baseline yet (one line, nothing to explain), "Not recorded" for an update
 * check the installed script does not record, and "Not readable" for
 * everything else. The hardening reason gets its short form here; the full
 * explanation and the fix live in the notice at the top of the page.
 */
export function MissingValue({ reason, align }: { reason: Reason; align?: 'right' }) {
  const alignment = align === 'right' ? 'text-right' : undefined
  if (reason.code === NO_BASELINE) {
    return (
      <p className={['text-sm text-muted-foreground', alignment].filter(Boolean).join(' ')}>
        Waiting for a second reading
      </p>
    )
  }
  const headline = reason.code === 'update.not-recorded' ? 'Not recorded' : 'Not readable'
  const detail = reasonDetail(reason)
  return (
    <div className={alignment}>
      <p className="text-sm text-muted-foreground">{headline}</p>
      {detail && <p className="text-xs text-muted-foreground">{detail}</p>}
    </div>
  )
}

/** The reason's second line: the short form for the hardening, whose full
 *  explanation is the notice at the top; the server's sentence otherwise. */
function reasonDetail(reason: Reason): string {
  return reason.code === HARDENING
    ? "Hidden by the unit's ProcSubset=pid — see the note at the top."
    : reason.message
}

export const hostRoute = createRoute({
  getParentRoute: () => authenticatedRoute,
  path: '/host',
  component: HostPage,
})
