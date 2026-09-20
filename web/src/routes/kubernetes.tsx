import { useQuery } from '@tanstack/react-query'
import { createRoute } from '@tanstack/react-router'
import { useState } from 'react'
import { api } from '@/api'
import { ActAs } from '@/components/ActAs'
import { ApplyManifest } from '@/components/ApplyManifest'
import { ClusterCapacityPanel } from '@/components/ClusterCapacityPanel'
import { ClusterResources } from '@/components/ClusterResources'
import { ClusterUsage } from '@/components/ClusterUsage'
import { DataTable } from '@/components/DataTable'
import { NodeSchedulingActions } from '@/components/NodeSchedulingActions'
import { NodeWhy } from '@/components/NodeWhy'
import { PodDiagnosis } from '@/components/PodDiagnosis'
import { Problem } from '@/components/Problem'
import { ReachService } from '@/components/ReachService'
import { TidyUp } from '@/components/TidyUp'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { RestartPodButton } from '@/components/WorkloadActions'
import { Workloads } from '@/components/Workloads'
import { authenticatedRoute } from '@/routes/__root'

/**
 * What Kubernetes says about a cluster (milestone v1.17, slice 2).
 *
 * This screen exists because the inventory cannot answer the question somebody
 * actually arrives with. The inventory knows what the machine API says about a
 * machine: it is reachable, it runs this Talos version, its etcd is a member.
 * None of that says whether the kubelet registered, whether the scheduler will
 * place anything there, or why a workload is not running — and those are the
 * facts a person is looking for at that moment.
 *
 * Two things it refuses to do, both learned one layer down:
 *
 * It does not render an empty list when the API server did not answer. That is
 * the claim INV-08 forbids — an empty screen reads as "nothing is running", and
 * the operator goes looking for a workload instead of an API server. A refusal
 * is shown as a problem to read.
 *
 * It shows a pod's phase AND its readiness. `Running` with 1 of 2 ready is the
 * most misread state in Kubernetes: the phase is the pod's own claim, the ready
 * count is whether its containers pass their probes, and a screen that showed
 * only the first would call a broken workload healthy.
 */
// KubernetesView is the screen's body, exported so its own test can render it
// without a router around it -- the same split the other screens use.
export function KubernetesView() {
  const [cluster, setCluster] = useState('')
  const [namespace, setNamespace] = useState('')
  const [diagnosing, setDiagnosing] = useState<{ namespace: string; pod: string } | null>(null)
  const [inspecting, setInspecting] = useState<string | null>(null)

  const clusters = useQuery({ queryKey: ['clusters'], queryFn: api.clusters.list })

  // The first cluster, until somebody picks another. A screen that made an
  // operator choose before showing anything would be asking a question it can
  // answer itself in the common case of one cluster.
  const selected = cluster === '' ? (clusters.data?.[0]?.id ?? '') : cluster

  const overview = useQuery({
    queryKey: ['kubernetes', selected, namespace],
    queryFn: () => api.kubernetes.overview(selected, namespace),
    enabled: selected !== '',
    // Kubernetes moves faster than the inventory does, and this screen is
    // usually open because something is moving right now.
    refetchInterval: 10_000,
  })

  return (
    <div className="space-y-4">
      <div>
        <h1 className="font-semibold text-2xl tracking-tight">Kubernetes</h1>
        <p className="text-muted-foreground text-sm">
          What the cluster’s own API server says: its nodes as Kubernetes sees them, and its pods.
          This is allowed to disagree with the Nodes screen — that screen reads the machine API, and
          where the two differ, the difference is the answer.
        </p>
      </div>

      {clusters.error ? <Problem error={clusters.error} /> : null}

      {clusters.data !== undefined && clusters.data.length === 0 && (
        <p className="text-muted-foreground text-sm">
          No cluster has been imported yet, so there is no Kubernetes API to ask.
        </p>
      )}

      {clusters.data !== undefined && clusters.data.length > 1 && (
        <div className="flex items-center gap-2">
          <span className="text-muted-foreground text-sm">Cluster</span>
          <Select value={selected} onValueChange={setCluster}>
            <SelectTrigger className="w-64" aria-label="Cluster">
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

      {overview.error ? (
        <>
          <Problem error={overview.error} />
          {/* Said out loud, because this is the distinction the screen exists
              to keep: no answer is not an empty cluster. */}
          <p className="text-muted-foreground text-sm">
            Nothing below is a statement about this cluster: its API server was asked and did not
            answer.
          </p>
        </>
      ) : null}

      {overview.data && (
        <>
          <p className="text-muted-foreground text-xs">API server {overview.data.server_version}</p>

          <Card>
            <CardHeader>
              <CardTitle>Nodes</CardTitle>
            </CardHeader>
            <CardContent>
              <DataTable
                label="Kubernetes nodes"
                rows={overview.data.nodes}
                keyOf={(node) => node.name}
                empty="The API server answered, and it has no nodes registered."
                columns={[
                  {
                    key: 'name',
                    label: 'Name',
                    role: 'identity',
                    render: (node) => <span className="font-mono text-xs">{node.name}</span>,
                  },
                  { key: 'ready', label: 'Ready', render: (node) => node.ready },
                  {
                    key: 'scheduling',
                    label: 'Scheduling',
                    render: (node) =>
                      node.unschedulable ? (
                        <span className="text-amber-700 dark:text-amber-300">cordoned</span>
                      ) : (
                        'schedulable'
                      ),
                  },
                  { key: 'roles', label: 'Roles', render: (node) => node.roles.join(', ') || '—' },
                  {
                    key: 'kubelet',
                    label: 'Kubelet',
                    render: (node) => node.kubelet_version || '—',
                  },
                  {
                    key: 'actions',
                    label: 'Scheduling actions',
                    role: 'actions',
                    render: (node) => (
                      <div className="flex flex-wrap gap-2 max-md:flex-col">
                        <Button
                          type="button"
                          size="sm"
                          variant="outline"
                          className="max-md:h-11"
                          onClick={() => setInspecting(node.name)}
                        >
                          Why?
                        </Button>
                        <NodeSchedulingActions
                          clusterID={selected}
                          node={node.name}
                          unschedulable={node.unschedulable}
                        />
                      </div>
                    ),
                  },
                ]}
              />
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>How full it is</CardTitle>
            </CardHeader>
            <CardContent>
              <ClusterCapacityPanel clusterID={selected} />
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Workloads</CardTitle>
            </CardHeader>
            <CardContent>
              <Workloads clusterID={selected} namespace={namespace} />
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Pods</CardTitle>
            </CardHeader>
            <CardContent className="space-y-2">
              <div className="flex items-center gap-2">
                <span className="text-muted-foreground text-sm">Namespace</span>
                <Select
                  value={namespace === '' ? 'all' : namespace}
                  onValueChange={(value) => setNamespace(value === 'all' ? '' : value)}
                >
                  <SelectTrigger className="w-64" aria-label="Namespace">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">every namespace</SelectItem>
                    {overview.data.namespaces.map((ns) => (
                      <SelectItem key={ns} value={ns}>
                        {ns}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <DataTable
                label="Pods"
                rows={overview.data.pods}
                keyOf={(pod) => `${pod.namespace}/${pod.name}`}
                empty={
                  <>
                    The API server answered, and there are no pods
                    {overview.data.namespace === '' ? '' : ` in ${overview.data.namespace}`}.
                  </>
                }
                columns={[
                  {
                    key: 'namespace',
                    label: 'Namespace',
                    render: (pod) => <span className="text-xs">{pod.namespace}</span>,
                  },
                  {
                    key: 'name',
                    label: 'Pod',
                    role: 'identity',
                    render: (pod) => (
                      <span className="break-all font-mono text-xs">{pod.name}</span>
                    ),
                  },
                  {
                    key: 'phase',
                    label: 'Phase',
                    render: (pod) => (
                      <>
                        {pod.phase}
                        {pod.reason === '' ? null : (
                          <span className="ml-1 text-red-700 dark:text-red-300">
                            ({pod.reason})
                          </span>
                        )}
                      </>
                    ),
                  },
                  {
                    key: 'ready',
                    label: 'Ready',
                    render: (pod) => (
                      <span
                        className={
                          pod.ready === pod.containers && pod.reason === ''
                            ? undefined
                            : 'text-red-700 dark:text-red-300'
                        }
                      >
                        {pod.ready}/{pod.containers}
                      </span>
                    ),
                  },
                  {
                    key: 'restarts',
                    label: 'Restarts',
                    className: 'tabular-nums',
                    render: (pod) => pod.restarts,
                  },
                  {
                    key: 'reserved',
                    label: 'Reserved',
                    // What this pod took out of its node's room. Requests are
                    // what the scheduler holds, so this is why a node is full.
                    render: (pod) =>
                      pod.cpu_request === '' && pod.memory_request === '' ? (
                        <span className="text-muted-foreground text-xs">nothing asked for</span>
                      ) : (
                        <span className="text-xs tabular-nums">
                          {pod.cpu_request || '—'} · {pod.memory_request || '—'}
                        </span>
                      ),
                  },
                  {
                    key: 'limit',
                    label: 'Limit',
                    // A detail on a phone: the limit matters when something is
                    // being throttled or killed, which is not the common read.
                    role: 'detail',
                    render: (pod) =>
                      pod.cpu_limit === '' && pod.memory_limit === '' ? (
                        <span className="text-muted-foreground text-xs">none</span>
                      ) : (
                        <span className="text-xs tabular-nums">
                          {pod.cpu_limit || '—'} · {pod.memory_limit || '—'}
                        </span>
                      ),
                  },
                  {
                    key: 'node',
                    label: 'Node',
                    render: (pod) => (
                      <span className="break-all font-mono text-xs">{pod.node || '—'}</span>
                    ),
                  },
                  {
                    key: 'action',
                    label: 'Action',
                    role: 'actions',
                    render: (pod) => (
                      <div className="flex flex-wrap gap-2 max-md:flex-col">
                        <Button
                          type="button"
                          size="sm"
                          variant="outline"
                          className="max-md:h-11"
                          onClick={() => setDiagnosing({ namespace: pod.namespace, pod: pod.name })}
                        >
                          Why?
                        </Button>
                        <RestartPodButton
                          clusterID={selected}
                          namespace={pod.namespace}
                          pod={pod.name}
                        />
                      </div>
                    ),
                  },
                ]}
              />
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>What is being used</CardTitle>
            </CardHeader>
            <CardContent>
              <ClusterUsage clusterID={selected} namespace={namespace} />
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Configuration, storage and routing</CardTitle>
            </CardHeader>
            <CardContent>
              <ClusterResources clusterID={selected} namespace={namespace} />
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Clear out what is finished</CardTitle>
            </CardHeader>
            <CardContent>
              <TidyUp clusterID={selected} namespace={namespace} />
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Who this acts as</CardTitle>
            </CardHeader>
            <CardContent>
              <ActAs clusterID={selected} />
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>What the cluster reported</CardTitle>
            </CardHeader>
            <CardContent>
              <ClusterEvents clusterID={selected} namespace={namespace} />
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Reach a service</CardTitle>
            </CardHeader>
            <CardContent>
              <ReachService clusterID={selected} services={overview.data.services} />
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Apply a manifest</CardTitle>
            </CardHeader>
            <CardContent>
              <ApplyManifest clusterID={selected} />
            </CardContent>
          </Card>
        </>
      )}

      {inspecting && (
        <NodeWhy clusterID={selected} node={inspecting} onClose={() => setInspecting(null)} />
      )}

      {diagnosing && (
        <PodDiagnosis
          clusterID={selected}
          namespace={diagnosing.namespace}
          pod={diagnosing.pod}
          onClose={() => setDiagnosing(null)}
        />
      )}
    </div>
  )
}

/**
 * What the cluster has reported, cluster-wide.
 *
 * Warnings first is deliberate and it is the only place this screen reorders
 * anything: inside one object the sequence is the story, but across a whole
 * cluster the question is "what is wrong", and a FailedScheduling buried under
 * forty Pulled events is an answer nobody finds.
 */
function ClusterEvents({ clusterID, namespace }: { clusterID: string; namespace: string }) {
  const events = useQuery({
    queryKey: ['kubernetes', 'events', clusterID, namespace],
    queryFn: () => api.kubernetes.events(clusterID, namespace),
    refetchInterval: 15_000,
  })

  if (events.error) return <Problem error={events.error} />
  if (!events.data) return <p className="text-muted-foreground text-sm">Reading…</p>

  const warnings = events.data.events.filter((e) => e.type === 'Warning').reverse()

  return (
    <div className="space-y-2">
      {warnings.length === 0 ? (
        <p className="text-muted-foreground text-sm">{events.data.notice}</p>
      ) : (
        <>
          <DataTable
            label="Warnings the cluster reported"
            phone="rows"
            rows={warnings.slice(0, 20)}
            keyOf={(e) => `${e.object}-${e.reason}-${e.last_seen}`}
            empty={events.data.notice}
            columns={[
              {
                key: 'reason',
                label: 'Reason',
                role: 'identity',
                render: (e) => (
                  <span className="text-amber-700 dark:text-amber-300">
                    {e.reason}
                    {e.count > 1 && <span className="text-muted-foreground"> ×{e.count}</span>}
                  </span>
                ),
              },
              {
                key: 'object',
                label: 'Object',
                render: (e) => <span className="break-all font-mono text-xs">{e.object}</span>,
              },
              {
                key: 'message',
                label: 'Message',
                role: 'detail',
                render: (e) => <span className="break-words">{e.message}</span>,
              },
              {
                key: 'last_seen',
                label: 'Last seen',
                role: 'detail',
                render: (e) => <span className="tabular-nums">{e.last_seen}</span>,
              },
            ]}
          />
          <p className="text-muted-foreground text-xs">{events.data.notice}</p>
        </>
      )}
    </div>
  )
}

export const kubernetesRoute = createRoute({
  getParentRoute: () => authenticatedRoute,
  path: '/kubernetes',
  component: KubernetesView,
})
