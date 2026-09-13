import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createRoute } from '@tanstack/react-router'
import { Lock, LockOpen, ShieldQuestion } from 'lucide-react'
import { useState } from 'react'
import { api, type Cluster } from '@/api'
import { ClusterScalePanel } from '@/components/ClusterScale'
import { ClusterTemplatePanel } from '@/components/ClusterTemplate'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { authenticatedRoute } from '@/routes/__root'

/**
 * Clusters, and the wizard that adopts one (D-29).
 *
 * The adoption is two steps and the split is the point (D-03): step one reads
 * the certificate the node is presenting and shows its fingerprint; step two
 * sends it back alongside the talosconfig. Nothing trusts the node between the
 * two, so what the operator confirms is the certificate that will actually be
 * used.
 *
 * The talosconfig is uploaded or pasted. There is deliberately no field for a
 * path on the server: that would be an arbitrary file read under the server's
 * uid (D-06).
 */
export function ClustersPage() {
  const { data, isLoading, error } = useQuery({
    queryKey: ['clusters'],
    queryFn: () => api.clusters.list(),
    refetchInterval: 30_000,
  })

  return (
    <section className="space-y-5">
      <div>
        <h1 className="font-heading text-xl font-semibold tracking-tight">Clusters</h1>
        <p className="max-w-prose text-sm text-muted-foreground">
          An imported cluster is one you already depend on, so it is adopted read-only. Unlock it
          when you want holzkube-manager to be able to change it.
        </p>
      </div>

      {isLoading && <p className="text-sm text-muted-foreground">Loading clusters…</p>}
      {error && (
        <p className="text-sm text-destructive">
          The cluster list could not be read: {(error as Error).message}
        </p>
      )}

      <div className="grid gap-4 lg:grid-cols-2">
        {(data ?? []).map((c) => (
          <ClusterCard key={c.id} cluster={c} />
        ))}
      </div>

      <ImportWizard />
      <CreateCluster />
      <ClusterTemplatePanel clusterID={(data ?? [])[0]?.id} />
    </section>
  )
}

export function ClusterCard({ cluster }: { cluster: Cluster }) {
  const queryClient = useQueryClient()

  const setLock = useMutation({
    mutationFn: (locked: boolean) => api.clusters.setLock(cluster.id, locked),
    onSettled: () => void queryClient.invalidateQueries({ queryKey: ['clusters'] }),
  })

  return (
    <Card>
      <CardHeader className="flex flex-row items-start justify-between gap-2">
        <div>
          <CardTitle className="text-base">{cluster.name}</CardTitle>
          <p className="font-mono text-xs text-muted-foreground">{cluster.endpoint}</p>
        </div>
        <div className="flex items-center gap-2">
          <Badge variant="outline">{cluster.origin}</Badge>
          <Badge
            variant="outline"
            className={
              cluster.locked
                ? 'border-amber-600/40 text-amber-700 dark:text-amber-300'
                : 'border-border'
            }
          >
            {cluster.locked ? 'read-only' : 'writable'}
          </Badge>
        </div>
      </CardHeader>

      <CardContent className="space-y-3 text-sm">
        <dl className="grid grid-cols-[10rem_1fr] gap-x-4 gap-y-1">
          <dt className="text-muted-foreground">Nodes</dt>
          <dd>
            {cluster.nodes} ({cluster.control_plane} control plane, {cluster.workers} worker)
          </dd>

          <dt className="text-muted-foreground">Condition</dt>
          <dd>
            <span className="text-emerald-700 dark:text-emerald-300">
              {cluster.healthy} healthy
            </span>
            {', '}
            <span className="text-amber-700 dark:text-amber-300">{cluster.degraded} degraded</span>
            {', '}
            <span className="text-red-700 dark:text-red-300">{cluster.down} not answering</span>
          </dd>

          <dt className="text-muted-foreground">Client certificate</dt>
          <dd>
            {/*
              The badge rung of the ladder (D-23). The banner rungs live in the
              app shell, because at 30 days and below the fact is about every
              page rather than about this card.
            */}
            {cluster.client_cert_days_left <= 0 ? (
              <span className="text-red-700 dark:text-red-300">
                expired {new Date(cluster.client_cert_not_after).toLocaleDateString()}
              </span>
            ) : (
              <span
                className={
                  cluster.certificate_urgency === 'none'
                    ? undefined
                    : 'text-amber-700 dark:text-amber-300'
                }
              >
                {cluster.client_cert_days_left} days left (
                {new Date(cluster.client_cert_not_after).toLocaleDateString()})
              </span>
            )}
          </dd>
        </dl>

        <div className="flex flex-col gap-1">
          <a
            href={api.clusters.talosconfigPath(cluster.id)}
            className="inline-block text-sm underline"
            title="An admin client configuration for talosctl. The certificate in it is minted on demand and is not the one holzkube-manager dials with, so losing your copy does not affect this instance's access."
          >
            Download a talosconfig
          </a>

          {/*
            The answer to "how do I run kubectl against this", which this
            product could not give until now: the only way was talosctl, which
            is the tool it exists to replace.

            It is rendered by a control-plane node rather than by this
            installation, so unlike the talosconfig above it can fail — and it
            is the one download here that is audited, because what comes back
            is system:masters on the cluster.
          */}
          <a
            href={api.clusters.kubeconfigPath(cluster.id)}
            className="inline-block text-sm underline"
            title="Admin credentials for this cluster's Kubernetes, rendered by a control-plane node. It is system:masters and it is not revocable from here — a Kubernetes CA rotation is what withdraws it. The request is recorded in the audit log."
          >
            Download a kubeconfig
          </a>

          {/*
            The support bundle's way in. The route was built on the argument
            that during an incident the operator is in a browser and not on a
            console — and then shipped without anything to click, which made
            the argument false. This is that gap closed, in the same shape as
            the link above: a plain anchor, because the response is a file to
            keep and the browser downloads it better than we can.

            No confirmation and no destructive marking: it reads the cluster
            and changes nothing. What it is worth saying is what is *in* it,
            because the operator is about to send it to somebody.
          */}
          <a
            href={api.clusters.supportBundlePath(cluster.id)}
            className="inline-block text-sm underline"
            title="One archive with what somebody debugging this cluster would otherwise collect by hand: per node the facts, services, versions, disks, links, extensions, etcd status, recent logs and the machine configuration with every secret removed — plus the audit tail. Nodes that do not answer are listed with the reason rather than left out."
          >
            Download a support bundle
          </a>
        </div>

        <Button
          size="sm"
          variant="outline"
          disabled={setLock.isPending}
          onClick={() => setLock.mutate(!cluster.locked)}
          title={
            cluster.locked
              ? 'Unlocking is itself a destructive action: it is what makes every other destructive action on this cluster reachable.'
              : 'Lock this cluster so that nothing can change it.'
          }
        >
          {cluster.locked ? (
            <>
              <LockOpen aria-hidden="true" className="size-4" /> Unlock
            </>
          ) : (
            <>
              <Lock aria-hidden="true" className="size-4" /> Lock
            </>
          )}
        </Button>

        <ClusterScalePanel clusterID={cluster.id} />
      </CardContent>
    </Card>
  )
}

/**
 * The two-step adoption.
 *
 * Step one is a read: it opens one connection, reads the certificate and
 * throws the connection away. Step two sends the talosconfig and the
 * fingerprint together. The server refuses step two outright if the
 * fingerprint does not match what the node presents at that moment, so the
 * confirmation is a real gate and not a ceremony.
 */
function ImportWizard() {
  const queryClient = useQueryClient()
  const [endpoint, setEndpoint] = useState('')
  const [name, setName] = useState('')
  const [talosconfig, setTalosconfig] = useState('')
  const [fingerprint, setFingerprint] = useState('')
  const [failure, setFailure] = useState('')

  const probe = useMutation({
    mutationFn: () => api.clusters.fingerprint(endpoint),
    onSuccess: (r) => {
      setFingerprint(r.fingerprint)
      setFailure('')
    },
    onError: (e: Error) => setFailure(e.message),
  })

  const adopt = useMutation({
    mutationFn: () => api.clusters.import({ name, talosconfig, endpoint, fingerprint }),
    onSuccess: () => {
      setName('')
      setTalosconfig('')
      setEndpoint('')
      setFingerprint('')
      setFailure('')
      void queryClient.invalidateQueries({ queryKey: ['clusters'] })
      void queryClient.invalidateQueries({ queryKey: ['machines'] })
    },
    onError: (e: Error) => setFailure(e.message),
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Import an existing cluster</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="max-w-prose text-sm text-muted-foreground">
          Name a <strong>control-plane</strong> node and upload the talosconfig you already use with{' '}
          <code>talosctl</code>. holzkube-manager reads that node's own machine configuration to
          derive the cluster's secrets, then proves it can get back in under a certificate it issues
          itself. A worker is refused: its configuration carries the certificate authority without
          the private key.
        </p>

        <div className="grid gap-3 sm:grid-cols-2">
          <div className="space-y-1">
            <Label htmlFor="cluster-name">Name</Label>
            <Input
              id="cluster-name"
              value={name}
              placeholder="homelab"
              onChange={(e) => setName(e.target.value)}
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor="cluster-endpoint">Control-plane address</Label>
            <Input
              id="cluster-endpoint"
              value={endpoint}
              placeholder="192.168.1.41"
              onChange={(e) => {
                setEndpoint(e.target.value)
                // The fingerprint belongs to the address it was read from.
                // Keeping a stale one after the address changed would let an
                // operator confirm one node's certificate and adopt another.
                setFingerprint('')
              }}
            />
          </div>
        </div>

        <div className="space-y-1">
          <Label htmlFor="cluster-talosconfig">talosconfig</Label>
          <textarea
            id="cluster-talosconfig"
            value={talosconfig}
            rows={6}
            spellCheck={false}
            placeholder="paste the contents of ~/.talos/config, or choose the file below"
            onChange={(e) => setTalosconfig(e.target.value)}
            className="w-full rounded-md border border-input bg-transparent p-2 font-mono text-xs"
          />
          <input
            type="file"
            aria-label="Upload a talosconfig"
            className="text-xs"
            onChange={async (e) => {
              const file = e.target.files?.[0]
              if (file) {
                setTalosconfig(await file.text())
              }
            }}
          />
          <p className="text-xs text-muted-foreground">
            The file is read in your browser and sent in the request body. holzkube-manager never
            reads a path on the server.
          </p>
        </div>

        <div className="space-y-2 rounded-md border border-border p-3">
          <div className="flex items-center gap-2 text-sm font-medium">
            <ShieldQuestion aria-hidden="true" className="size-4" /> Confirm the node's certificate
          </div>
          <Button
            size="sm"
            variant="outline"
            disabled={endpoint === '' || probe.isPending}
            onClick={() => probe.mutate()}
          >
            Read the fingerprint
          </Button>
          {fingerprint !== '' && (
            <p className="break-all font-mono text-xs">
              {fingerprint}
              <span className="mt-1 block font-sans text-muted-foreground">
                Check this against <code>talosctl --nodes {endpoint} get certificate</code>, or
                against whatever you trust, before importing.
              </span>
            </p>
          )}
        </div>

        {failure !== '' && <p className="text-sm text-destructive">{failure}</p>}

        <Button
          disabled={
            adopt.isPending ||
            name === '' ||
            endpoint === '' ||
            talosconfig === '' ||
            fingerprint === ''
          }
          onClick={() => adopt.mutate()}
        >
          Import cluster
        </Button>
      </CardContent>
    </Card>
  )
}

/**
 * Creating a cluster from nothing.
 *
 * This is the only path in the product that mints a certificate authority
 * (INV-02), and it is deliberately a separate form from the import rather than
 * a mode of it. The reason is not tidiness: an import that could fall back to
 * generating its own PKI would produce a cluster record indistinguishable from
 * the real one that cannot enter a single one of that cluster's nodes. A test
 * holds the separation; this form is the half an operator sees.
 *
 * A created cluster has no nodes. Provisioning them is phase 8 — this writes
 * the secrets a joining node's configuration will be built from.
 */
function CreateCluster() {
  const queryClient = useQueryClient()
  const [name, setName] = useState('')
  const [endpoint, setEndpoint] = useState('')
  const [failure, setFailure] = useState('')

  const create = useMutation({
    mutationFn: () => api.clusters.create(name, endpoint),
    onSuccess: () => {
      setName('')
      setEndpoint('')
      setFailure('')
      void queryClient.invalidateQueries({ queryKey: ['clusters'] })
    },
    onError: (e: Error) => setFailure(e.message),
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Create a new cluster</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="max-w-prose text-sm text-muted-foreground">
          For a cluster that does not exist yet. holzkube-manager generates its certificate
          authorities and joining tokens, and keeps them. It has no nodes until you provision one. A
          created cluster is <strong>not</strong> locked — there is nothing yet to protect from a
          mistake.
        </p>

        <div className="grid gap-3 sm:grid-cols-2">
          <div className="space-y-1">
            <Label htmlFor="new-cluster-name">Name</Label>
            <Input
              id="new-cluster-name"
              value={name}
              placeholder="lab"
              onChange={(e) => setName(e.target.value)}
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor="new-cluster-endpoint">Kubernetes endpoint</Label>
            <Input
              id="new-cluster-endpoint"
              value={endpoint}
              placeholder="https://10.0.0.1:6443"
              onChange={(e) => setEndpoint(e.target.value)}
            />
          </div>
        </div>

        {failure !== '' && <p className="text-sm text-destructive">{failure}</p>}

        <Button
          variant="secondary"
          disabled={create.isPending || name === '' || endpoint === ''}
          onClick={() => create.mutate()}
        >
          Create cluster
        </Button>
      </CardContent>
    </Card>
  )
}

export const clustersRoute = createRoute({
  getParentRoute: () => authenticatedRoute,
  path: '/clusters',
  component: ClustersPage,
})
