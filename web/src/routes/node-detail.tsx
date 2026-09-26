import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createRoute, Link, useNavigate } from '@tanstack/react-router'
import { BookX, RefreshCw } from 'lucide-react'
import { type ReactNode, useState } from 'react'
import { api, type Field, type Machine } from '@/api'
import { HealthField, StageBadge } from '@/components/HealthField'
import { LabelEditor } from '@/components/LabelEditor'
import { LogPanel } from '@/components/LogPanel'
import { NodeActions } from '@/components/NodeActions'
import { NodeHardware } from '@/components/NodeHardware'
import { PowerMenu } from '@/components/PowerMenu'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useStream } from '@/hooks/useStream'
import { authenticatedRoute } from '@/routes/__root'
import { MACHINE_POLL_INTERVAL_MS } from '@/routes/nodes'

/**
 * One node, at its own URL (D-30).
 *
 * A route and not a panel, because a node is the object people talk about
 * during an incident and a link you can paste into a message is worth more
 * than the click it saves. Phases 5, 6 and 7 hang logs, actions and
 * configuration off this page; a sheet would have been too small within three
 * phases.
 */
/**
 * When the node last answered, shown only when that is news.
 *
 * It is news in exactly one case: the node is answering and its readings are
 * not current. That is a machine which is present and cannot be read — a
 * different repair from one that is absent, and previously indistinguishable
 * from it, because an observation that failed halfway recorded nothing at all.
 *
 * When the node is healthy this says nothing. A line reading "last answered:
 * four seconds ago" on every page is a line nobody reads on the page where it
 * matters.
 */
export function LastAnswered({ machine }: { machine: Machine }) {
  if (machine.seen_at === undefined || machine.stage === 'watching') {
    return null
  }

  return (
    <p className="text-sm text-amber-700 dark:text-amber-300">
      This node last answered {new Date(machine.seen_at).toLocaleString()}. If that is recent and
      the readings below are not, the machine is reachable and something on it is not responding —
      which is a different thing from a machine that is down.
    </p>
  )
}

export function NodeDetailPage() {
  const { uuid } = nodeDetailRoute.useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()

  const { data, isLoading, error } = useQuery({
    queryKey: ['machine', uuid],
    queryFn: () => api.machines.get(uuid),
    refetchInterval: MACHINE_POLL_INTERVAL_MS,
  })

  const refresh = useMutation({
    mutationFn: () => api.machines.refresh(uuid),
    onSettled: () => void queryClient.invalidateQueries({ queryKey: ['machine', uuid] }),
  })

  const forget = useMutation({
    mutationFn: () => api.machines.forget(uuid),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['machines'] })
      void navigate({ to: '/nodes' })
    },
  })

  if (isLoading) {
    return <p className="text-sm text-muted-foreground">Loading node…</p>
  }
  if (error || !data) {
    return (
      <p className="text-sm text-destructive">
        This node could not be read: {(error as Error | undefined)?.message ?? 'not found'}
      </p>
    )
  }

  const m = data

  return (
    <section className="space-y-5">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="font-heading text-xl font-semibold tracking-tight">
              <HealthField field={m.hostname} render={(v) => v || m.id.slice(0, 8)} />
            </h1>
            <StageBadge stage={m.stage} />
            <Badge variant="outline">{m.role}</Badge>
          </div>
          <p className="font-mono text-xs text-muted-foreground">{m.id}</p>
          <p className="text-sm text-muted-foreground">
            In the inventory since {new Date(m.adopted_at).toLocaleString()}.{' '}
            <Link to="/nodes" className="underline">
              Back to all nodes
            </Link>
          </p>
          <LastAnswered machine={m} />
        </div>

        <div className="flex flex-wrap gap-2 max-md:w-full max-md:flex-nowrap max-md:justify-between">
          {/* Harmless first, destructive last and apart: reading, power, then
              the three acts that take the machine or its record away. */}
          <Button
            variant="outline"
            size="sm"
            className="max-md:min-w-11"
            disabled={refresh.isPending}
            onClick={() => refresh.mutate()}
            title="Refresh"
            aria-label="Refresh"
          >
            <RefreshCw aria-hidden="true" className="size-4" />{' '}
            <span className="max-md:sr-only">Refresh</span>
          </Button>
          <PowerMenu
            target={{ kind: 'node', machine: m.id, name: m.hostname.value || m.id.slice(0, 8) }}
          />
          <span aria-hidden="true" className="w-4 max-md:hidden" />
          <NodeActions machine={m} onRemoved={() => void navigate({ to: '/nodes' })} />
          {/*
            Destructive, so the server answers 428 and the shared interceptor
            opens the password prompt and replays this exact request. There is
            deliberately no confirmation dialog of our own: a second one would
            train the operator to click past the first.

            It removes the record and touches the machine not at all — no
            cordon, no drain, no reset. That is phase 6's work.
          */}
          <Button
            variant="outline"
            size="sm"
            className="text-destructive max-md:min-w-11"
            disabled={forget.isPending}
            onClick={() => forget.mutate()}
            title="Forget: remove this record from the inventory. The machine itself is not touched."
            aria-label="Forget"
          >
            <BookX aria-hidden="true" className="size-4" />{' '}
            <span className="max-md:sr-only">Forget</span>
          </Button>
        </div>
      </header>

      {m.certificate_expired && (
        <p className="rounded-md border border-red-600/40 bg-red-600/10 px-3 py-2 text-sm text-red-700 dark:text-red-300">
          The cluster's client certificate has expired. That is why this node cannot be reached; the
          node itself is very probably fine.
        </p>
      )}
      {!m.watch.live && m.watch.reason && (
        <p className="rounded-md border border-slate-500/40 bg-slate-500/10 px-3 py-2 text-sm text-slate-700 dark:text-slate-300">
          Live updates for this node are off: {m.watch.reason}. Its facts are still being read on
          the heartbeat, so what you see below is at most one heartbeat old rather than missing.
        </p>
      )}
      {m.lost_addr && (
        <p className="rounded-md border border-amber-600/40 bg-amber-600/10 px-3 py-2 text-sm text-amber-700 dark:text-amber-300">
          A different machine answered at this one's last known address. This record was kept
          exactly as it was — the UUID identifies a machine, an address does not.
        </p>
      )}

      <NodeHardware machine={m} />

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Identity</CardTitle>
          </CardHeader>
          <CardContent>
            <dl className="grid grid-cols-1 md:grid-cols-[10rem_minmax(0,1fr)] gap-x-4 gap-y-2 text-sm">
              <Row label="Address" field={m.addr} mono />
              <Row label="Talos" field={m.talos_version} mono />
              <Row label="Kubernetes" field={m.kubernetes_version} mono />
              <Row
                label="Schematic"
                field={m.schematic_id}
                mono
                render={(v) => (v ? `${v.slice(0, 16)}…` : '')}
              />
              <Row label="Manufacturer" field={m.manufacturer} />
              <Row label="Product" field={m.product_name} />
              <Row label="Serial" field={m.serial_number} mono />
              <Row label="etcd member" field={m.etcd_member} render={(v) => (v ? 'yes' : 'no')} />
            </dl>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">Labels</CardTitle>
          </CardHeader>
          <CardContent>
            <LabelEditor machine={m} />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">Version compatibility</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            <HealthField
              field={m.compatibility}
              render={(c) =>
                c.known ? (
                  <span className="space-y-1">
                    <span
                      className={
                        c.supported
                          ? 'text-emerald-700 dark:text-emerald-300'
                          : 'text-red-700 dark:text-red-300'
                      }
                    >
                      {c.supported ? 'Supported' : 'Outside the supported window'}
                    </span>
                    <span className="block text-muted-foreground">{c.sentence}</span>
                    <span className="block text-xs text-muted-foreground">
                      Headroom: {c.headroom_minors} Kubernetes minor version
                      {c.headroom_minors === 1 ? '' : 's'}.
                    </span>
                  </span>
                ) : (
                  <span className="text-muted-foreground">{c.sentence}</span>
                )
              }
            />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">Hardware</CardTitle>
          </CardHeader>
          <CardContent>
            <dl className="grid grid-cols-1 md:grid-cols-[10rem_minmax(0,1fr)] gap-x-4 gap-y-2 text-sm">
              <Row
                label="Memory"
                field={m.memory_mib}
                render={(v) => `${(v / 1024).toFixed(0)} GiB`}
              />
              <Row
                label="Processors"
                field={m.cpus}
                render={(cpus) => (
                  <ul>
                    {cpus.map((c) => (
                      <li key={c.socket}>
                        {c.manufacturer} {c.product_name} — {c.cores} cores / {c.threads} threads
                      </li>
                    ))}
                  </ul>
                )}
              />
              <Row
                label="Disks"
                field={m.disks}
                render={(disks) => (
                  <ul className="break-words font-mono text-xs">
                    {disks.map((d) => (
                      <li key={d.device}>
                        {d.device} — {d.pretty_size || d.size} {d.model && `(${d.model})`}
                      </li>
                    ))}
                  </ul>
                )}
              />
              <Row
                label="Interfaces"
                field={m.interfaces}
                render={(ifaces) => (
                  <ul className="break-words font-mono text-xs">
                    {ifaces.map((i) => (
                      <li key={i.name}>
                        {i.name} — {i.up ? 'up' : 'down'}
                        {i.addresses.length > 0 && ` — ${i.addresses.join(', ')}`}
                      </li>
                    ))}
                  </ul>
                )}
              />
            </dl>
          </CardContent>
        </Card>

        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle className="text-base">Live output</CardTitle>
          </CardHeader>
          <CardContent>
            <NodeStreams machine={m} />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">Services</CardTitle>
          </CardHeader>
          <CardContent>
            <HealthField
              field={m.services}
              render={(services) => (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Service</TableHead>
                      <TableHead>State</TableHead>
                      <TableHead>Health</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {services.map((s) => (
                      <TableRow key={s.id}>
                        <TableCell className="font-mono text-xs">{s.id}</TableCell>
                        <TableCell className="text-xs">{s.state}</TableCell>
                        <TableCell className="text-xs">
                          {/*
                            "does not report health" is not "reported unhealthy".
                            Collapsing the two paints a working node red, which is
                            how a colour stops meaning anything.
                          */}
                          {s.health_unknown ? (
                            <span className="text-muted-foreground">not reported</span>
                          ) : s.healthy ? (
                            <span className="text-emerald-700 dark:text-emerald-300">healthy</span>
                          ) : (
                            <span className="text-red-700 dark:text-red-300">unhealthy</span>
                          )}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            />
          </CardContent>
        </Card>
      </div>
    </section>
  )
}

/**
 * The live panels for one node, on one connection.
 *
 * Which services to offer comes from the node's own service list rather than
 * from a hardcoded set: a node that does not run etcd has no etcd log, and a
 * panel for it would sit at "nothing yet" forever, which reads as broken. A
 * node whose service list is stale still gets the services it last reported —
 * the log may be empty, but the list is the honest one.
 */
function NodeStreams({ machine }: { machine: Machine }) {
  const services = (machine.services.value ?? [])
    .map((s) => s.id)
    .filter((id) => STREAMABLE_SERVICES.has(id))

  const [open, setOpen] = useState<string[]>(() => ['dmesg'])

  // Panel name -> topic, so the render below never indexes one array with
  // another's position. Two parallel arrays kept in step by hand is a bug
  // waiting for somebody to filter one of them.
  const topicOf = (name: string) =>
    name === 'dmesg' ? `dmesg:${machine.id}` : `logs:${machine.id}:${name}`

  const { lines, connection } = useStream(open.map(topicOf))

  const available = ['dmesg', ...services]

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap gap-2">
        {available.map((name) => (
          <Button
            key={name}
            size="sm"
            variant={open.includes(name) ? 'secondary' : 'outline'}
            onClick={() =>
              setOpen((prev) =>
                prev.includes(name) ? prev.filter((n) => n !== name) : [...prev, name],
              )
            }
          >
            {name}
          </Button>
        ))}
      </div>

      {open.length === 0 && (
        <p className="text-sm text-muted-foreground">
          No panels open. Every panel above shares one connection to this instance, and one stream
          per topic from the node — four panels on one service is one read, not four.
        </p>
      )}

      <div className="grid gap-3 xl:grid-cols-2">
        {open.map((name) => (
          <LogPanel
            key={name}
            title={name}
            lines={lines[topicOf(name)] ?? []}
            connection={connection}
          />
        ))}
      </div>
    </div>
  )
}

/**
 * The Talos services whose logs are worth offering.
 *
 * A list rather than "everything the node reports", because the service list
 * includes things whose log is empty by construction, and a panel that can
 * never fill is indistinguishable from one that is broken.
 */
const STREAMABLE_SERVICES = new Set([
  'apid',
  'containerd',
  'cri',
  'etcd',
  'kubelet',
  'machined',
  'trustd',
])

function Row<T>({
  label,
  field,
  render,
  mono,
}: {
  label: string
  field: Field<T>
  render?: (value: T) => ReactNode
  mono?: boolean
}) {
  return (
    <>
      <dt className="text-muted-foreground">{label}</dt>
      {/* break-words, because the values here are the ones that do not break on
          their own: a schematic id is 64 hex characters and a CPU model is a
          sentence with no spaces the browser likes. Measured at 390px: this row
          reached 397px and was gone, not narrow. */}
      <dd className={mono ? 'break-words font-mono text-xs' : 'break-words'}>
        <HealthField field={field} render={render} />
      </dd>
    </>
  )
}

export const nodeDetailRoute = createRoute({
  getParentRoute: () => authenticatedRoute,
  path: '/nodes/$uuid',
  component: NodeDetailPage,
})
