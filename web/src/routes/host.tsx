import { useQuery } from '@tanstack/react-query'
import { createRoute } from '@tanstack/react-router'
import type { ReactNode } from 'react'
import { api, type Host, type Link, type Reason } from '@/api'
import { Meter } from '@/components/charts/Meter'
import { FanList, Sensors } from '@/components/charts/Sensors'
import { ago } from '@/components/HealthField'
import { HARDWARE_POLL_INTERVAL_MS } from '@/components/NodeHardware'
import { Problem } from '@/components/Problem'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { formatBytes, formatPercent, formatRate, formatUptime } from '@/lib/format'
import { authenticatedRoute } from '@/routes/__root'

/**
 * The machine holzkube-manager itself runs on (Phase 11, HOST-01).
 *
 * Every other page in this product is about the cluster; this one is about the
 * box beside it -- on the operator's installation a Raspberry Pi. It is read
 * live, every 3 s while the page is open, and nothing on it is stored.
 *
 * The rule the whole page is built around (D-02): a value the daemon could not
 * read is drawn as "Not readable" with the reason, and never as a zero or an
 * empty cell. The production unit hides /proc/stat and /proc/meminfo from the
 * daemon, so an unread value is an everyday state here, not an error -- and a
 * 0 % that is really "could not look" is the one lie this page must not tell.
 * The schema makes the renderer branch on `readable` before it can format
 * anything; MissingValue is the only way a missing value is drawn.
 */

type Reading<T> = { readable: true; value: T } | { readable: false; reason: Reason }

export function HostPage() {
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

  return <HostView host={query.data} stale={query.error ?? null} />
}

export function HostView({ host, stale }: { host: Host; stale: unknown }) {
  const isStale = stale !== null && stale !== undefined
  const observed = new Date(host.observed_at)
  const d = host.device
  const hidden = hiddenByHardening(host)
  const load = host.live.cpu.load
  const rates = host.live.rates_over_seconds

  return (
    <section className="space-y-5">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="font-heading text-xl font-semibold tracking-tight">Host</h1>
          <p className="text-sm text-muted-foreground">
            The machine holzkube-manager runs on, and the service itself.
          </p>
        </div>
      </header>

      {/* Notices, in this order when they apply: stale, container, hardening. */}
      {isStale && (
        <p className="rounded-md border border-amber-600/40 bg-amber-600/10 px-3 py-2 text-sm text-amber-700 dark:text-amber-300">
          holzkube-manager did not answer the latest request. What you see is its reading from{' '}
          {observed.toLocaleTimeString()}. {stale instanceof Error ? stale.message : ''}
        </p>
      )}
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
            {hidden.length === 1 ? 'is' : 'are'} shown as not readable rather than as zero.
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

        <LiveSection host={host} />

        <p className="text-xs text-muted-foreground">
          Read {observed.toLocaleTimeString()}, every 3 s while this page is open.
          {rates !== null && ` Rates are over the last ${rates.toFixed(1)} s.`}
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
          const { size_bytes: size, used_bytes: used, available_bytes: free } = f.usage.value
          // df's denominator: blocks reserved for root are neither used nor
          // available, so used / size would read lower than df's Use%.
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
                  {formatBytes(used)} of {formatBytes(size)} · {formatBytes(free)} free
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
 * The Live block (HMON-01): figures and meters of the moment, no curves --
 * history is Phase 12's.
 */
function LiveSection({ host }: { host: Host }) {
  return (
    <section aria-labelledby="host-live" className="space-y-4">
      <div>
        <h2 id="host-live" className="text-base font-semibold">
          Live
        </h2>
        <p className="text-sm text-muted-foreground">
          Read every 3 s while this page is open. Nothing on this page is stored.
        </p>
      </div>
      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_20rem]">
        <div className="min-w-0 space-y-4">
          <ProcessorCard host={host} />
          <div className="grid gap-4 md:grid-cols-2">
            <MemoryCard memory={host.live.memory} />
            <FilesystemsCard filesystems={host.live.filesystems} />
          </div>
          <NetworkCard
            network={host.live.network}
            container={host.container}
            waiting={host.live.rates_over_seconds === null}
          />
        </div>
        <SensorsCard sensors={host.live.sensors} />
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
}: {
  network: Host['live']['network']
  container: boolean
  waiting: boolean
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
      <CardContent className="space-y-2">
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
 * rows (D-13) -- without the sparkline column, since this page draws no curves
 * yet. Voltages are never read, so never shown (D-08).
 */
function SensorsCard({ sensors }: { sensors: Host['live']['sensors'] }) {
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

function ProcessorCard({ host }: { host: Host }) {
  const { usage, per_core: perCore, load } = host.live.cpu
  const cores = host.device.cores
  return (
    <Card>
      <CardHeader className="flex flex-row items-start justify-between gap-3">
        <div className="min-w-0">
          <CardTitle className="text-base">Processor</CardTitle>
          {cores.readable && (
            <p className="truncate text-sm text-muted-foreground">{cores.value} cores</p>
          )}
        </div>
        <div className="text-right">
          {usage.readable ? (
            <p className="font-semibold text-xl tabular-nums">{formatPercent(usage.value)}</p>
          ) : (
            <MissingValue reason={usage.reason} align="right" />
          )}
          {/* Load survives the hardening (it comes from sysinfo(2)), so it is
              shown whenever it was read, whatever happened to usage. */}
          {load.readable ? (
            <p className="text-xs text-muted-foreground tabular-nums">
              load {load.value.load1.toFixed(2)} · {load.value.load5.toFixed(2)} ·{' '}
              {load.value.load15.toFixed(2)}
            </p>
          ) : (
            <p className="text-xs text-muted-foreground">
              Load not readable: {reasonDetail(load.reason)}
            </p>
          )}
        </div>
      </CardHeader>
      {/* Per-core meters only for numbers that were read: an unread or not yet
          computed core is left out, never drawn as an empty bar. */}
      {perCore.readable && perCore.value.length > 0 && (
        <CardContent>
          <div className="grid grid-cols-2 gap-x-4 gap-y-2 sm:grid-cols-4">
            {perCore.value.map((u, i) => (
              <Meter
                // biome-ignore lint/suspicious/noArrayIndexKey: the index IS the CPU's name
                key={i}
                dense
                label={`CPU ${i}`}
                display={formatPercent(u)}
                value={u}
                max={100}
                warn={75}
                danger={90}
              />
            ))}
          </div>
        </CardContent>
      )}
    </Card>
  )
}

function MemoryCard({ memory }: { memory: Host['live']['memory'] }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Memory</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
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
  if (reason.code === 'rate.no-baseline') {
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
