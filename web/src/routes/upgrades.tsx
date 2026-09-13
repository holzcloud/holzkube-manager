import { useMutation, useQuery } from '@tanstack/react-query'
import { createRoute, Link } from '@tanstack/react-router'
import { AlertTriangle, CheckCircle2, Download, Info, Lock, Unlock } from 'lucide-react'
import { useState } from 'react'
import { api, type EtcdMemberList, type GateVerdict, type NodePlan, type UpgradePlan } from '@/api'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { authenticatedRoute } from '@/routes/__root'

/**
 * Rolling upgrades and etcd management.
 *
 * The thing this screen refuses to do is the point of it. There is no "upgrade
 * to latest" button, because "latest" may be two minors away and Talos does
 * not support skipping one — what replaces it is a chain, listed, each hop a
 * separate run with the health gate re-evaluated in between.
 *
 * And the gate shows its inputs whether it passes or refuses. A gate that only
 * says no is a gate an operator works around; the numbers it decided on are
 * what makes "why did it let that through" answerable too.
 */
export function UpgradesPage() {
  const [cluster, setCluster] = useState('')

  const clusters = useQuery({ queryKey: ['clusters'], queryFn: () => api.clusters.list() })
  const chosen = cluster || clusters.data?.[0]?.id || ''

  return (
    <section className="space-y-5">
      <div>
        <h1 className="font-heading text-xl font-semibold tracking-tight">Upgrades</h1>
        <p className="max-w-prose text-sm text-muted-foreground">
          Rolling Talos and Kubernetes upgrades, node by node, behind a gate that would rather
          refuse than leave the cluster without a quorum.
        </p>
      </div>

      {clusters.isSuccess && clusters.data.length === 0 && (
        <p className="text-sm text-muted-foreground">No clusters yet.</p>
      )}

      {clusters.isSuccess && clusters.data.length > 0 && (
        <div className="max-w-xs space-y-1.5">
          <Label htmlFor="upgrade-cluster">Cluster</Label>
          <Select value={chosen} onValueChange={setCluster}>
            <SelectTrigger id="upgrade-cluster">
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

      {chosen && (
        <UpgradePanel
          cluster={chosen}
          name={clusters.data?.find((c) => c.id === chosen)?.name ?? ''}
        />
      )}
      {chosen && <EtcdPanel cluster={chosen} />}
      {chosen && <LockPanel cluster={chosen} />}
    </section>
  )
}

/* ---------------------------------------------------------------------- */
/* The rolling upgrade                                                     */
/* ---------------------------------------------------------------------- */

function UpgradePanel({ cluster, name }: { cluster: string; name: string }) {
  const [kubernetes, setKubernetes] = useState(false)
  const [to, setTo] = useState('')
  const [typed, setTyped] = useState('')

  const releases = useQuery({
    queryKey: ['upgrade', 'releases'],
    queryFn: () => api.upgrades.releases(),
  })

  const plan = useMutation({
    mutationFn: () =>
      kubernetes ? api.upgrades.planKubernetes(cluster, to) : api.upgrades.plan(cluster, to),
  })

  const start = useMutation({
    mutationFn: async () => {
      const kind = kubernetes ? 'cluster.upgrade-kubernetes' : 'cluster.upgrade-talos'
      const confirmation = await api.upgrades.confirm(cluster, kind, to, typed)
      return api.upgrades.start(cluster, to, confirmation.token, kubernetes)
    },
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Roll the cluster forward</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-3 sm:grid-cols-2">
          <div className="space-y-1.5">
            <Label htmlFor="upgrade-what">What</Label>
            <Select
              value={kubernetes ? 'kubernetes' : 'talos'}
              onValueChange={(value) => setKubernetes(value === 'kubernetes')}
            >
              <SelectTrigger id="upgrade-what">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="talos">Talos</SelectItem>
                <SelectItem value="kubernetes">Kubernetes</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="upgrade-to">To version</Label>
            {kubernetes ? (
              <Input
                id="upgrade-to"
                value={to}
                onChange={(e) => setTo(e.target.value)}
                placeholder="v1.34.1"
              />
            ) : (
              <Select value={to} onValueChange={setTo}>
                <SelectTrigger id="upgrade-to">
                  <SelectValue placeholder="Choose a version" />
                </SelectTrigger>
                <SelectContent>
                  {(releases.data?.releases ?? []).map((r) => (
                    <SelectItem key={r} value={r}>
                      {r}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          </div>
        </div>

        {releases.data?.notice && <Notice text={releases.data.notice} />}

        <Button disabled={plan.isPending || !to} onClick={() => plan.mutate()}>
          {plan.isPending ? 'Checking every node…' : 'Show me what this would do'}
        </Button>
        {plan.error ? <Problem error={plan.error} /> : null}

        {plan.data && (
          <PlanView
            plan={plan.data}
            clusterName={name}
            typed={typed}
            onTyped={setTyped}
            onStart={() => start.mutate()}
            starting={start.isPending}
            startError={start.error}
            started={start.isSuccess}
          />
        )}
      </CardContent>
    </Card>
  )
}

function PlanView({
  plan,
  clusterName,
  typed,
  onTyped,
  onStart,
  starting,
  startError,
  started,
}: {
  plan: UpgradePlan
  clusterName: string
  typed: string
  onTyped: (v: string) => void
  onStart: () => void
  starting: boolean
  startError: unknown
  started: boolean
}) {
  return (
    <div className="space-y-4 border-t border-border pt-4">
      {plan.blocked && <Notice danger text={plan.block_reason} />}

      {/* UPG-05. A target more than one minor away is several runs, and the
          middle ones are not the version anybody wanted. */}
      {plan.chain && plan.chain.length > 1 && (
        <div className="space-y-1.5">
          <p className="text-sm font-medium">This takes more than one run</p>
          <ol className="space-y-1.5">
            {plan.chain.map((step) => (
              <li key={`${step.to.Major}.${step.to.Minor}.${step.to.Patch}`} className="text-sm">
                <span className="font-mono">
                  v{step.to.Major}.{step.to.Minor}.{step.to.Patch}
                </span>
                <span className="ml-2 text-muted-foreground">{step.why}</span>
              </li>
            ))}
          </ol>
        </div>
      )}

      {/* UPG-06. Shown whether or not it blocks: "this is fine and here is
          why" is the answer to the question an operator asks when the button
          is enabled. */}
      <Notice danger={plan.strand.blocked} text={plan.strand.sentence} />
      {plan.strand.remedy && <Notice danger text={plan.strand.remedy} />}

      <GatePanel gate={plan.gate_preview} />

      <NodeTable nodes={plan.nodes} />

      <div className="space-y-1.5">
        <Label htmlFor="upgrade-typed">
          Type <span className="font-mono">{clusterName}</span> to confirm
        </Label>
        <Input
          id="upgrade-typed"
          value={typed}
          onChange={(e) => onTyped(e.target.value)}
          placeholder={clusterName}
        />
        <p className="text-xs text-muted-foreground">
          A rolling upgrade is not about one machine, so there is no hostname to type. What is at
          risk is the cluster.
        </p>
      </div>

      <Button
        variant="destructive"
        disabled={plan.blocked || starting || typed.trim() !== clusterName}
        onClick={onStart}
      >
        {starting ? 'Submitting…' : 'Start the rolling upgrade'}
      </Button>
      {startError ? <Problem error={startError} /> : null}
      {started && (
        <p className="text-sm">
          Accepted as a job.{' '}
          <Link className="underline" to="/jobs">
            Watch it on the jobs page
          </Link>
          . You can stop it after the node it is on.
        </p>
      )}
    </div>
  )
}

/**
 * UPG-02's "shows its inputs", on the screen.
 *
 * The numbers are here whether the gate passed or refused. A gate that only
 * says no is a gate an operator works around, and one that says yes without
 * saying why is one nobody can check.
 */
export function GatePanel({ gate }: { gate: GateVerdict }) {
  const input = gate.input

  return (
    <div className="space-y-2 rounded-md border border-border p-3">
      <p className="flex items-center gap-2 text-sm font-medium">
        {gate.ok ? (
          <CheckCircle2 aria-hidden="true" className="size-4" />
        ) : (
          <AlertTriangle aria-hidden="true" className="size-4 text-destructive" />
        )}
        Health gate {gate.ok ? 'passes right now' : 'refuses'}
      </p>

      {gate.reason && <p className="max-w-prose text-sm text-muted-foreground">{gate.reason}</p>}

      <dl className="grid gap-2 text-sm sm:grid-cols-3">
        <div>
          <dt className="text-xs text-muted-foreground">Voting etcd members</dt>
          <dd>{input.voting}</dd>
        </div>
        <div>
          <dt className="text-xs text-muted-foreground">Largest raft lag</dt>
          <dd>{input.max_raft_lag}</dd>
        </div>
        <div>
          <dt className="text-xs text-muted-foreground">Alarms</dt>
          <dd>{input.alarms?.length ? input.alarms.map((a) => a.Type).join(', ') : 'none'}</dd>
        </div>
      </dl>

      {input.unreachable && input.unreachable.length > 0 && (
        <p className="text-sm text-destructive">Did not answer: {input.unreachable.join(', ')}</p>
      )}

      <p className="text-xs text-muted-foreground">
        This is the gate as it stands now. The gate that decides runs again before every node — the
        second node is taken down in a cluster the first one just changed.
      </p>
    </div>
  )
}

function NodeTable({ nodes }: { nodes: NodePlan[] }) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Node</TableHead>
          <TableHead>Role</TableHead>
          <TableHead>From</TableHead>
          <TableHead>Installs</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {nodes.map((n) => (
          <TableRow key={n.machine}>
            <TableCell>
              <span className="font-mono text-xs">{n.hostname || n.machine}</span>
              {n.skipped && (
                <Badge className="ml-2" variant="outline">
                  skipped
                </Badge>
              )}
              {n.blocked && (
                <Badge className="ml-2" variant="destructive">
                  blocked
                </Badge>
              )}
              {(n.skip_reason || n.block_reason) && (
                <p className="mt-1 max-w-prose text-xs text-muted-foreground">
                  {n.skip_reason || n.block_reason}
                </p>
              )}
              {/* UPG-04: both lists, so the operator can see the difference
                  rather than take it on trust. */}
              {n.kernel_args?.drifted && (
                <dl className="mt-1 space-y-0.5 text-xs">
                  {n.kernel_args.only_on_node && n.kernel_args.only_on_node.length > 0 && (
                    <div>
                      <dt className="inline text-muted-foreground">Only on the node: </dt>
                      <dd className="inline font-mono">{n.kernel_args.only_on_node.join(' ')}</dd>
                    </div>
                  )}
                  {n.kernel_args.only_in_config && n.kernel_args.only_in_config.length > 0 && (
                    <div>
                      <dt className="inline text-muted-foreground">Only in the configuration: </dt>
                      <dd className="inline font-mono">{n.kernel_args.only_in_config.join(' ')}</dd>
                    </div>
                  )}
                </dl>
              )}
            </TableCell>
            <TableCell>{n.role}</TableCell>
            <TableCell className="font-mono text-xs">{n.from || '—'}</TableCell>
            <TableCell>
              {/* UPG-03. The installer is shown per node, because its failure
                  is silent: install a stock image on a node built from a
                  Factory one and every extension it has is gone. */}
              <span className="break-all font-mono text-[11px]">{n.installer || '—'}</span>
              {n.schematic_sentence && (
                <p className="mt-1 max-w-prose text-xs text-muted-foreground">
                  {n.schematic_sentence}
                </p>
              )}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

/* ---------------------------------------------------------------------- */
/* etcd                                                                    */
/* ---------------------------------------------------------------------- */

export function EtcdPanel({ cluster }: { cluster: string }) {
  const members = useQuery({
    queryKey: ['etcd', cluster],
    queryFn: () => api.etcd.members(cluster),
  })

  const remove = useMutation({
    mutationFn: (id: string) => api.etcd.removeMember(cluster, id),
    onSuccess: () => members.refetch(),
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">etcd</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {members.error ? <Problem error={members.error} /> : null}
        {members.data && <MemberTable list={members.data} onRemove={remove.mutate} />}
        {remove.error ? <Problem error={remove.error} /> : null}

        {/* UPG-12. A link rather than a fetch: the browser streams a database
            to disk instead of holding it in memory. */}
        <a
          className="inline-flex items-center gap-2 text-sm underline"
          href={api.etcd.snapshotURL(cluster)}
        >
          <Download aria-hidden="true" className="size-4" />
          Download an etcd snapshot
        </a>
        <p className="max-w-prose text-xs text-muted-foreground">
          A snapshot through the etcd API is a consistent point-in-time copy and needs a quorum. A
          cluster that has lost one cannot answer that, so this fails there — what is left then is
          the member's own database file, copied off the node, which is not the same thing.
        </p>

        {/*
          The other half of the sentence, said here because here is where
          somebody forms the belief. A product that offers a backup button and
          no restore is a product whose operator finds out during the disaster;
          the backup card on the settings screen says the same kind of thing
          about its own tarballs, for the same reason.

          Restoring is deliberately not here rather than not yet. It rewinds
          the cluster to the snapshot's moment and discards everything after
          it, and it runs on exactly one control-plane node -- doing it on two
          produces two clusters that both believe they are the original. That
          is a decision for somebody at a console with the cluster in front of
          them, not a button in a browser during an incident.
        */}
        <p className="max-w-prose text-xs text-muted-foreground">
          <strong>holzkube-manager does not restore a snapshot.</strong> Recovery is two steps with{' '}
          <code>talosctl</code> — upload the snapshot to one control-plane node with{' '}
          <code>talosctl etcd recover</code>, then bootstrap that same node in recovery mode — and
          Talos' own documentation is the reference for the exact invocation. It is not here
          deliberately: a restore rewinds the cluster to this moment and discards everything after
          it, and running it on more than one node produces two clusters that each believe they are
          the original. Keep the file somewhere that survives the cluster.
        </p>
      </CardContent>
    </Card>
  )
}

export function MemberTable({
  list,
  onRemove,
}: {
  list: EtcdMemberList
  onRemove: (id: string) => void
}) {
  return (
    <div className="space-y-2">
      <p className="text-sm text-muted-foreground">{list.sentence}</p>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Member</TableHead>
            <TableHead>Votes</TableHead>
            <TableHead />
          </TableRow>
        </TableHeader>
        <TableBody>
          {list.members.map((m) => (
            <TableRow key={m.id}>
              {/* UPG-10: hostname first, hex id behind it. */}
              <TableCell className="text-sm">{m.name}</TableCell>
              <TableCell>
                {m.voting ? (
                  <Badge variant="secondary">voting</Badge>
                ) : (
                  <Badge variant="outline">learner</Badge>
                )}
              </TableCell>
              <TableCell className="text-right">
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={m.voting && list.tolerates === 0}
                  onClick={() => onRemove(m.id)}
                >
                  Remove
                </Button>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}

/* ---------------------------------------------------------------------- */
/* UPG-14: the per-node lock                                               */
/* ---------------------------------------------------------------------- */

function LockPanel({ cluster }: { cluster: string }) {
  const machines = useQuery({ queryKey: ['machines'], queryFn: () => api.machines.list() })
  const [reason, setReason] = useState('')

  const lock = useMutation({
    mutationFn: ({ id, locked }: { id: string; locked: boolean }) =>
      api.machineLock.set(id, locked, locked ? reason : ''),
    onSuccess: () => machines.refetch(),
  })

  const nodes = (machines.data ?? []).filter((m) => m.cluster === cluster)

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Nodes rolling operations skip</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="max-w-prose text-sm text-muted-foreground">
          A locked node is walked past rather than failing the run. The lock is a note this
          installation keeps about what it should not do, so it works on a node that is not
          answering — which is when you most want to set one.
        </p>

        <div className="max-w-md space-y-1.5">
          <Label htmlFor="lock-reason">Reason</Label>
          <Input
            id="lock-reason"
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            placeholder="the database on this one moves first"
          />
          <p className="text-xs text-muted-foreground">
            Required when locking. A lock nobody can explain is a lock the next person clears
            because it is in the way.
          </p>
        </div>

        <Table>
          <TableBody>
            {nodes.map((m) => (
              <TableRow key={m.id}>
                <TableCell className="font-mono text-xs">
                  {m.hostname.value || m.id}
                  {m.lock_reason && (
                    <p className="mt-1 text-xs text-muted-foreground">{m.lock_reason}</p>
                  )}
                </TableCell>
                <TableCell className="text-right">
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={lock.isPending || (!m.locked && !reason.trim())}
                    onClick={() => lock.mutate({ id: m.id, locked: !m.locked })}
                  >
                    {m.locked ? (
                      <>
                        <Unlock aria-hidden="true" className="size-4" /> Unlock
                      </>
                    ) : (
                      <>
                        <Lock aria-hidden="true" className="size-4" /> Lock
                      </>
                    )}
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
        {lock.error ? <Problem error={lock.error} /> : null}
      </CardContent>
    </Card>
  )
}

/* ---------------------------------------------------------------------- */

function Notice({ text, danger }: { text: string; danger?: boolean }) {
  if (!text) return null
  return (
    <p
      className={
        danger
          ? 'flex max-w-prose gap-2 rounded-md border border-destructive/50 p-2.5 text-sm'
          : 'flex max-w-prose gap-2 rounded-md border border-border p-2.5 text-sm text-muted-foreground'
      }
    >
      {danger ? (
        <AlertTriangle aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-destructive" />
      ) : (
        <Info aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
      )}
      <span>{text}</span>
    </p>
  )
}

function Problem({ error }: { error: unknown }) {
  return (
    <p className="max-w-prose text-sm text-destructive">
      {error instanceof Error ? error.message : String(error)}
    </p>
  )
}

export const upgradesRoute = createRoute({
  getParentRoute: () => authenticatedRoute,
  path: '/upgrades',
  component: UpgradesPage,
})
