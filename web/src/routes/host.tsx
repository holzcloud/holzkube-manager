import { useQuery } from '@tanstack/react-query'
import { createRoute } from '@tanstack/react-router'
import type { ReactNode } from 'react'
import { api, type Host, type Reason } from '@/api'
import { HARDWARE_POLL_INTERVAL_MS } from '@/components/NodeHardware'
import { Problem } from '@/components/Problem'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { formatUptime } from '@/lib/format'
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
        </div>

        <p className="text-xs text-muted-foreground">
          Read {observed.toLocaleTimeString()}, every 3 s while this page is open.
        </p>
      </div>
    </section>
  )
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
  const detail =
    reason.code === 'hardening.proc-subset'
      ? "Hidden by the unit's ProcSubset=pid — see the note at the top."
      : reason.message
  return (
    <div className={alignment}>
      <p className="text-sm text-muted-foreground">{headline}</p>
      {detail && <p className="text-xs text-muted-foreground">{detail}</p>}
    </div>
  )
}

export const hostRoute = createRoute({
  getParentRoute: () => authenticatedRoute,
  path: '/host',
  component: HostPage,
})
