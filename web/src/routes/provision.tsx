import { useMutation, useQuery } from '@tanstack/react-query'
import { createRoute, Link } from '@tanstack/react-router'
import { AlertTriangle, Info, Search } from 'lucide-react'
import { useState } from 'react'
import {
  api,
  type Candidate,
  type CandidateDisk,
  type Found,
  type ProvisionPreview,
  type ProvisionRequest,
} from '@/api'
import { DiskEncryption } from '@/components/DiskEncryption'
import { Problem } from '@/components/Problem'
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
 * The provisioning wizard: a blank machine becomes a cluster node.
 *
 * Four steps, and the split between them is the point. The first three read
 * and change nothing; the fourth is the only thing in this product that can
 * wipe a disk somebody else's data is on. So the fourth is a separate screen
 * with a separate confirmation, and it shows exactly what is about to be
 * written rather than a summary of it.
 *
 * Two things on this page are not decoration and are worth saying out loud.
 *
 * A machine in maintenance mode has no cluster PKI, so nothing here has
 * verified that it is the machine anybody thinks it is. The certificate
 * fingerprint is shown next to every candidate, and the only place it can be
 * checked against is the machine's own console.
 *
 * The install image is shown, in full, before the apply. A machine that boots
 * an ISO with system extensions and then installs a stock installer comes up
 * without them -- the install succeeds, the node joins, the extensions are
 * simply gone, and nothing reports it. Reading that line is the only moment
 * anybody would catch it.
 */
export function ProvisionPage() {
  const [candidate, setCandidate] = useState<Candidate | null>(null)

  const recovery = useQuery({
    queryKey: ['provision', 'recovery'],
    queryFn: () => api.provision.recovery(),
  })

  return (
    <section className="space-y-5">
      <div>
        <h1 className="font-heading text-xl font-semibold tracking-tight">Provision</h1>
        <p className="max-w-prose text-sm text-muted-foreground">
          Find a machine waiting for a configuration, confirm it is the one you mean, and turn it
          into a node of one of your clusters.
        </p>
      </div>

      {recovery.data && recovery.data.pending.length > 0 && (
        <BootstrapRecovery
          pending={recovery.data.pending}
          guidance={recovery.data.guidance}
          onResolved={() => recovery.refetch()}
        />
      )}

      {candidate ? (
        <PlanStep candidate={candidate} onBack={() => setCandidate(null)} />
      ) : (
        <ScanStep onChoose={setCandidate} />
      )}
    </section>
  )
}

/* ---------------------------------------------------------------------- */
/* Step one and two: find a machine, then read it                          */
/* ---------------------------------------------------------------------- */

function ScanStep({ onChoose }: { onChoose: (c: Candidate) => void }) {
  const [cidr, setCidr] = useState('')
  const [addr, setAddr] = useState('')

  const notices = useQuery({
    queryKey: ['provision', 'notices'],
    queryFn: () => api.provision.notices(),
  })

  const scan = useMutation({
    mutationFn: () => api.provision.scan(cidr.trim(), addr.trim() ? [addr.trim()] : []),
  })

  const inspect = useMutation({
    mutationFn: (found: Found) => api.provision.inspect(found.addr, found.fingerprint),
    onSuccess: onChoose,
  })

  return (
    <div className="space-y-4">
      {/* PROV-12. Before the scan, not after it: each of these produces a
          failure that looks like something else, and the second one looks
          exactly like an empty result. */}
      {(notices.data ?? []).map((notice) => (
        <Notice key={notice} text={notice} />
      ))}

      <Card>
        <CardHeader>
          <CardTitle className="text-base">1 · Find the machine</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="provision-cidr">Subnet</Label>
              <Input
                id="provision-cidr"
                placeholder="192.168.1.0/24"
                value={cidr}
                onChange={(e) => setCidr(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">
                A /23 or smaller. Larger than that cannot be scanned before the server has to
                answer.
              </p>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="provision-addr">Or one address</Label>
              <Input
                id="provision-addr"
                placeholder="192.168.1.41"
                value={addr}
                onChange={(e) => setAddr(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">
                Always available, and the way in when the machine is on another subnet.
              </p>
            </div>
          </div>

          <Button
            disabled={scan.isPending || (!cidr.trim() && !addr.trim())}
            onClick={() => scan.mutate()}
          >
            <Search aria-hidden="true" className="size-4" />
            {scan.isPending ? 'Scanning…' : 'Scan'}
          </Button>

          {scan.error && <Problem error={scan.error} />}
          {inspect.error && <Problem error={inspect.error} />}

          {scan.isSuccess && <FoundTable found={scan.data.found} onInspect={inspect.mutate} />}
        </CardContent>
      </Card>
    </div>
  )
}

export function FoundTable({
  found,
  onInspect,
}: {
  found: Found[]
  onInspect: (f: Found) => void
}) {
  if (found.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        Nothing answered on the Talos API port at any of those addresses. That is a statement about
        the addresses, not about the machines: a machine that booted the ISO without DHCP has no
        address for anything to reach.
      </p>
    )
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Address</TableHead>
          <TableHead>State</TableHead>
          <TableHead>Certificate fingerprint</TableHead>
          <TableHead />
        </TableRow>
      </TableHeader>
      <TableBody>
        {found.map((f) => (
          <TableRow key={f.addr}>
            <TableCell className="font-mono text-xs">
              {f.addr}
              {f.hostname && <span className="ml-2 text-muted-foreground">{f.hostname}</span>}
            </TableCell>
            <TableCell>
              <StateBadge found={f} />
              {f.detail && (
                <p className="mt-1 max-w-prose text-xs text-muted-foreground">{f.detail}</p>
              )}
            </TableCell>
            <TableCell className="max-w-[18rem] break-all font-mono text-[11px] text-muted-foreground">
              {f.fingerprint || '—'}
            </TableCell>
            <TableCell className="text-right">
              {f.state === 'maintenance' && (
                <Button size="sm" variant="secondary" onClick={() => onInspect(f)}>
                  Inspect
                </Button>
              )}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

/**
 * PROV-02, on the screen.
 *
 * A configured node is reported as a configured node and never as "nothing
 * found". The two lead to opposite actions: one is "check the cable", the
 * other is "there is already a machine here and provisioning it would replace
 * what is on it".
 */
function StateBadge({ found }: { found: Found }) {
  if (found.state === 'maintenance') {
    return <Badge variant="secondary">Waiting for a configuration</Badge>
  }
  return (
    <span className="flex flex-wrap items-center gap-1.5">
      <Badge variant="destructive">
        {found.state === 'configured' ? 'Already configured' : 'Something is here'}
      </Badge>
      {found.known && <Badge variant="outline">In your inventory</Badge>}
    </span>
  )
}

/* ---------------------------------------------------------------------- */
/* Step three and four: plan it, then write it                             */
/* ---------------------------------------------------------------------- */

function PlanStep({ candidate, onBack }: { candidate: Candidate; onBack: () => void }) {
  const clusters = useQuery({ queryKey: ['clusters'], queryFn: () => api.clusters.list() })

  const [cluster, setCluster] = useState('')
  const [controlPlane, setControlPlane] = useState(false)
  const [disk, setDisk] = useState(candidate.disks[0]?.device ?? '')
  const [schematic, setSchematic] = useState('')
  const [version, setVersion] = useState(candidate.talos_version)
  const [hostname, setHostname] = useState('')
  const [typed, setTyped] = useState('')
  const [secureBoot, setSecureBoot] = useState(false)
  const [encryptState, setEncryptState] = useState(false)
  const [encryptEphemeral, setEncryptEphemeral] = useState(false)
  const [encryptKind, setEncryptKind] = useState('nodeID')

  const encrypting = encryptState || encryptEphemeral

  const request: ProvisionRequest = {
    cluster,
    addr: candidate.addr,
    uuid: candidate.uuid,
    control_plane: controlPlane,
    install_disk: disk,
    schematic_id: schematic.trim(),
    talos_version: version.trim(),
    fingerprint: candidate.fingerprint,
    hostname: hostname.trim(),
    secureboot: secureBoot,
    encryption: encrypting
      ? { state: encryptState, ephemeral: encryptEphemeral, kind: encryptKind }
      : undefined,
  }

  const plan = useMutation({ mutationFn: () => api.provision.plan(request) })

  const apply = useMutation({
    mutationFn: async () => {
      const confirmation = await api.provision.confirm(request, typed)
      return api.provision.apply(request, confirmation.token)
    },
  })

  const preview: ProvisionPreview | undefined = plan.data

  return (
    <div className="space-y-4">
      <Button size="sm" variant="ghost" onClick={onBack}>
        ← Back to the scan
      </Button>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">2 · This is the machine</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3 text-sm">
          {/* PROV-03: the UUID is the key, the MAC is the identifier printed
              on a label, and the two together are how anybody tells one
              identical mini-PC from another. */}
          <dl className="grid gap-2 sm:grid-cols-2">
            <Fact label="Address" value={candidate.addr} mono />
            <Fact label="UUID" value={candidate.uuid} mono />
            <Fact label="Talos version" value={candidate.talos_version || 'not reported'} />
            <Fact label="MAC addresses" value={candidate.macs.join(', ') || 'none reported'} mono />
            <Fact label="Certificate fingerprint" value={candidate.fingerprint || '—'} mono wide />
          </dl>

          {candidate.warnings.map((w) => (
            <Notice key={w} text={w} />
          ))}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">3 · What to write</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="provision-cluster">Cluster</Label>
              <Select value={cluster} onValueChange={setCluster}>
                <SelectTrigger id="provision-cluster">
                  <SelectValue placeholder="Choose a cluster" />
                </SelectTrigger>
                <SelectContent>
                  {(clusters.data ?? []).map((c) => (
                    <SelectItem key={c.id} value={c.id}>
                      {c.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="provision-role">Role</Label>
              <Select
                value={controlPlane ? 'controlplane' : 'worker'}
                onValueChange={(v) => setControlPlane(v === 'controlplane')}
              >
                <SelectTrigger id="provision-role">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="worker">Worker</SelectItem>
                  <SelectItem value="controlplane">Control plane</SelectItem>
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="provision-version">Talos version</Label>
              <Input
                id="provision-version"
                value={version}
                onChange={(e) => setVersion(e.target.value)}
                placeholder="v1.13.9"
              />
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="provision-schematic">Image Factory schematic</Label>
              <Input
                id="provision-schematic"
                value={schematic}
                onChange={(e) => setSchematic(e.target.value)}
                placeholder="the same id as the ISO this machine booted"
              />
              <p className="text-xs text-muted-foreground">
                The installer is built from this id. Leave it empty only if the machine booted a
                stock ISO — an ISO with system extensions and a stock installer produces a node
                without them.
              </p>
            </div>

            <div className="space-y-1.5 sm:col-span-2">
              <label className="flex items-start gap-2 text-sm">
                <input
                  type="checkbox"
                  className="mt-1"
                  checked={secureBoot}
                  onChange={(e) => setSecureBoot(e.target.checked)}
                />
                <span>
                  This machine booted the <strong>SecureBoot</strong> image
                  <span className="block text-xs text-muted-foreground">
                    A schematic id does not say which variant was written to the USB stick — one id
                    builds both — so this is the only place that knows. It picks the installer, and
                    Talos requires it to match: the ordinary installer does not produce a SecureBoot
                    node.
                  </span>
                </span>
              </label>
            </div>

            <div className="space-y-1.5 sm:col-span-2">
              <Label htmlFor="provision-hostname">Hostname (optional)</Label>
              <Input
                id="provision-hostname"
                value={hostname}
                onChange={(e) => setHostname(e.target.value)}
              />
            </div>
          </div>

          <DiskPicker disks={candidate.disks} chosen={disk} onChoose={setDisk} />

          <DiskEncryption
            state={encryptState}
            ephemeral={encryptEphemeral}
            kind={encryptKind}
            secureBoot={secureBoot}
            onState={setEncryptState}
            onEphemeral={setEncryptEphemeral}
            onKind={setEncryptKind}
          />

          <Button disabled={plan.isPending || !cluster || !disk} onClick={() => plan.mutate()}>
            {plan.isPending ? 'Checking…' : 'Show me what this would do'}
          </Button>
          {plan.error && <Problem error={plan.error} />}
        </CardContent>
      </Card>

      {preview && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">4 · Apply</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <dl className="grid gap-2">
              {/* PROV-08, shown in full. This line is the only place the
                  extensions-disappear failure is visible before it happens. */}
              <Fact label="Install image" value={preview.install_image} mono wide />
              <Fact label="Install disk" value={disk} mono />
              <Fact
                label="Control-plane nodes after this"
                value={String(preview.control_plane_after)}
              />
            </dl>

            {preview.bootstrap && (
              <Notice
                danger
                text="This run initialises etcd for the cluster, because it is the first control-plane node. That happens exactly once in a cluster's life and cannot be undone."
              />
            )}

            {preview.warnings.map((w) => (
              <Notice danger key={w} text={w} />
            ))}

            <Notice text={preview.maintenance_warning} />
            <Notice text={preview.cni_notice} />

            <div className="space-y-1.5">
              <Label htmlFor="provision-typed">
                Type <span className="font-mono">{disk}</span> to confirm
              </Label>
              <Input
                id="provision-typed"
                value={typed}
                onChange={(e) => setTyped(e.target.value)}
                placeholder={disk}
              />
              <p className="text-xs text-muted-foreground">
                That disk is what gets written. Typing it is how this screen knows you mean that
                one.
              </p>
            </div>

            <Button
              variant="destructive"
              disabled={apply.isPending || typed.trim() !== disk}
              onClick={() => apply.mutate()}
            >
              {apply.isPending ? 'Submitting…' : 'Provision this machine'}
            </Button>
            {apply.error && <Problem error={apply.error} />}

            {apply.isSuccess && (
              <p className="text-sm">
                Accepted as a job. Closing this tab does not stop it —{' '}
                <Link className="underline" to="/jobs">
                  watch it on the jobs page
                </Link>
                .
              </p>
            )}
          </CardContent>
        </Card>
      )}
    </div>
  )
}

/**
 * PROV-06.
 *
 * Size, model, serial and transport, because "512 GB" on a machine with two
 * 512 GB disks is not a choice. The system disk is marked and deliberately not
 * pre-selected: on a machine that has been installed before, choosing it is
 * the ordinary thing, and on a fresh one the mark means nothing.
 */
export function DiskPicker({
  disks,
  chosen,
  onChoose,
}: {
  disks: CandidateDisk[]
  chosen: string
  onChoose: (device: string) => void
}) {
  return (
    <div className="space-y-1.5">
      <Label>Install disk</Label>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead />
            <TableHead>Device</TableHead>
            <TableHead>Size</TableHead>
            <TableHead>Model</TableHead>
            <TableHead>Serial</TableHead>
            <TableHead>Transport</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {disks.map((d) => (
            <TableRow key={d.device}>
              <TableCell>
                <input
                  type="radio"
                  name="install-disk"
                  aria-label={`Install to ${d.device}`}
                  checked={chosen === d.device}
                  onChange={() => onChoose(d.device)}
                />
              </TableCell>
              <TableCell className="font-mono text-xs">
                {d.device}
                {d.system && (
                  <Badge className="ml-2" variant="outline">
                    system disk
                  </Badge>
                )}
              </TableCell>
              <TableCell>{d.pretty_size || d.size}</TableCell>
              <TableCell>{d.model || '—'}</TableCell>
              <TableCell className="font-mono text-xs">{d.serial || '—'}</TableCell>
              <TableCell>{d.transport || '—'}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}

/* ---------------------------------------------------------------------- */
/* PROV-10: the recovery flow for the one case nothing can decide          */
/* ---------------------------------------------------------------------- */

function BootstrapRecovery({
  pending,
  guidance,
  onResolved,
}: {
  pending: { cluster: string; addr: string; machine: string; started_at: string; detail: string }[]
  guidance: string
  onResolved: () => void
}) {
  return (
    <Card className="border-destructive/50">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base text-destructive">
          <AlertTriangle aria-hidden="true" className="size-4" />
          An etcd bootstrap has no recorded outcome
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="max-w-prose text-sm">{guidance}</p>
        {pending.map((p) => (
          <PendingBootstrap key={p.cluster} pending={p} onResolved={onResolved} />
        ))}
      </CardContent>
    </Card>
  )
}

function PendingBootstrap({
  pending,
  onResolved,
}: {
  pending: { cluster: string; addr: string; machine: string; started_at: string; detail: string }
  onResolved: () => void
}) {
  const [note, setNote] = useState('')

  const resolve = useMutation({
    mutationFn: (bootstrapped: boolean) =>
      api.provision.resolve(pending.cluster, bootstrapped, note.trim()),
    onSuccess: onResolved,
  })

  return (
    <div className="space-y-3 rounded-md border border-border p-3">
      <dl className="grid gap-2 sm:grid-cols-2">
        <Fact label="Cluster" value={pending.cluster} mono />
        <Fact label="Machine" value={pending.machine || 'not recorded'} mono />
        <Fact label="Address" value={pending.addr || 'not recorded'} mono />
        <Fact label="Started" value={pending.started_at || 'not recorded'} />
      </dl>
      {pending.detail && <p className="text-sm text-muted-foreground">{pending.detail}</p>}

      <div className="space-y-1.5">
        <Label htmlFor={`note-${pending.cluster}`}>What did you find?</Label>
        <Input
          id={`note-${pending.cluster}`}
          value={note}
          onChange={(e) => setNote(e.target.value)}
          placeholder="etcd has three members"
        />
        <p className="text-xs text-muted-foreground">
          This is the only account anything will ever have of this bootstrap. There is no default
          answer, because guessing here is the operation the record exists to prevent.
        </p>
      </div>

      <div className="flex flex-wrap gap-2">
        <Button
          size="sm"
          variant="secondary"
          disabled={resolve.isPending || !note.trim()}
          onClick={() => resolve.mutate(true)}
        >
          etcd is running
        </Button>
        <Button
          size="sm"
          variant="secondary"
          disabled={resolve.isPending || !note.trim()}
          onClick={() => resolve.mutate(false)}
        >
          etcd was never started
        </Button>
      </div>
      {resolve.error && <Problem error={resolve.error} />}
    </div>
  )
}

/* ---------------------------------------------------------------------- */
/* Small pieces                                                            */
/* ---------------------------------------------------------------------- */

function Fact({
  label,
  value,
  mono,
  wide,
}: {
  label: string
  value: string
  mono?: boolean
  wide?: boolean
}) {
  return (
    <div className={wide ? 'sm:col-span-2' : undefined}>
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className={mono ? 'break-all font-mono text-xs' : 'text-sm'}>{value}</dd>
    </div>
  )
}

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

export const provisionRoute = createRoute({
  getParentRoute: () => authenticatedRoute,
  path: '/provision',
  component: ProvisionPage,
})
