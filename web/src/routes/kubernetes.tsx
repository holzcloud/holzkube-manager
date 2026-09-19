import { useQuery } from '@tanstack/react-query'
import { createRoute } from '@tanstack/react-router'
import { useState } from 'react'
import { api } from '@/api'
import { NodeSchedulingActions } from '@/components/NodeSchedulingActions'
import { Problem } from '@/components/Problem'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { DeploymentActions, RestartPodButton } from '@/components/WorkloadActions'
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
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b text-left text-muted-foreground">
                      <th className="py-1 pr-4 font-medium">Name</th>
                      <th className="py-1 pr-4 font-medium">Ready</th>
                      <th className="py-1 pr-4 font-medium">Scheduling</th>
                      <th className="py-1 pr-4 font-medium">Roles</th>
                      <th className="py-1 pr-4 font-medium">Kubelet</th>
                      <th className="py-1 pr-4 font-medium">Scheduling actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {overview.data.nodes.map((node) => (
                      <tr key={node.name} className="border-b max-md:h-11">
                        <td className="py-1 pr-4 font-mono text-xs">{node.name}</td>
                        <td className="py-1 pr-4">{node.ready}</td>
                        <td className="py-1 pr-4">
                          {node.unschedulable ? (
                            <span className="text-amber-700 dark:text-amber-300">cordoned</span>
                          ) : (
                            'schedulable'
                          )}
                        </td>
                        <td className="py-1 pr-4">{node.roles.join(', ') || '—'}</td>
                        <td className="py-1 pr-4">{node.kubelet_version || '—'}</td>
                        <td className="py-1 pr-4">
                          <NodeSchedulingActions
                            clusterID={selected}
                            node={node.name}
                            unschedulable={node.unschedulable}
                          />
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Deployments</CardTitle>
            </CardHeader>
            <CardContent>
              {overview.data.deployments.length === 0 ? (
                <p className="text-muted-foreground text-sm">
                  The API server answered, and this cluster has no deployments
                  {overview.data.namespace === '' ? '' : ` in ${overview.data.namespace}`}.
                </p>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b text-left text-muted-foreground">
                        <th className="py-1 pr-4 font-medium">Namespace</th>
                        <th className="py-1 pr-4 font-medium">Deployment</th>
                        <th className="py-1 pr-4 font-medium">Ready</th>
                        <th className="py-1 pr-4 font-medium">Image</th>
                        <th className="py-1 pr-4 font-medium">Actions</th>
                      </tr>
                    </thead>
                    <tbody>
                      {overview.data.deployments.map((deployment) => (
                        <tr
                          key={`${deployment.namespace}/${deployment.name}`}
                          className="border-b max-md:h-11"
                        >
                          <td className="py-1 pr-4 text-xs">{deployment.namespace}</td>
                          <td className="py-1 pr-4 font-mono text-xs">{deployment.name}</td>
                          <td
                            className={
                              deployment.ready === deployment.desired
                                ? 'py-1 pr-4'
                                : 'py-1 pr-4 text-red-700 dark:text-red-300'
                            }
                          >
                            {deployment.ready}/{deployment.desired}
                          </td>
                          <td className="py-1 pr-4 font-mono text-xs">{deployment.image || '—'}</td>
                          <td className="py-1 pr-4">
                            <DeploymentActions
                              clusterID={selected}
                              namespace={deployment.namespace}
                              deployment={deployment.name}
                              desired={deployment.desired}
                            />
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
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

              {overview.data.pods.length === 0 ? (
                <p className="text-muted-foreground text-sm">
                  The API server answered, and there are no pods
                  {overview.data.namespace === '' ? '' : ` in ${overview.data.namespace}`}.
                </p>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b text-left text-muted-foreground">
                        <th className="py-1 pr-4 font-medium">Namespace</th>
                        <th className="py-1 pr-4 font-medium">Pod</th>
                        <th className="py-1 pr-4 font-medium">Phase</th>
                        <th className="py-1 pr-4 font-medium">Ready</th>
                        <th className="py-1 pr-4 font-medium">Restarts</th>
                        <th className="py-1 pr-4 font-medium">Node</th>
                        <th className="py-1 pr-4 font-medium">Action</th>
                      </tr>
                    </thead>
                    <tbody>
                      {overview.data.pods.map((pod) => {
                        const healthy = pod.ready === pod.containers && pod.reason === ''
                        return (
                          <tr key={`${pod.namespace}/${pod.name}`} className="border-b max-md:h-11">
                            <td className="py-1 pr-4 text-xs">{pod.namespace}</td>
                            <td className="py-1 pr-4 font-mono text-xs">{pod.name}</td>
                            <td className="py-1 pr-4">
                              {pod.phase}
                              {pod.reason === '' ? null : (
                                <span className="ml-1 text-red-700 dark:text-red-300">
                                  ({pod.reason})
                                </span>
                              )}
                            </td>
                            <td
                              className={
                                healthy ? 'py-1 pr-4' : 'py-1 pr-4 text-red-700 dark:text-red-300'
                              }
                            >
                              {pod.ready}/{pod.containers}
                            </td>
                            <td className="py-1 pr-4 tabular-nums">{pod.restarts}</td>
                            <td className="py-1 pr-4 font-mono text-xs">{pod.node || '—'}</td>
                            <td className="py-1 pr-4">
                              <RestartPodButton
                                clusterID={selected}
                                namespace={pod.namespace}
                                pod={pod.name}
                              />
                            </td>
                          </tr>
                        )
                      })}
                    </tbody>
                  </table>
                </div>
              )}
            </CardContent>
          </Card>
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
