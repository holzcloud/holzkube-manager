import { createRoute } from '@tanstack/react-router'
import { useState } from 'react'
import { DataTable } from '@/components/DataTable'
import {
  KubernetesAnswer,
  KubernetesShell,
  useClusterSelection,
} from '@/components/KubernetesSection'
import { PodDiagnosis } from '@/components/PodDiagnosis'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { RestartPodButton } from '@/components/WorkloadActions'
import { kubernetesRoute } from '@/routes/kubernetes'

/**
 * The pods, and what each of them reserved.
 *
 * A pod carries both its phase AND its readiness, because `Running` with 1 of 2
 * ready is the most misread state in Kubernetes: the phase is the pod's own claim
 * about its lifecycle, the ready count is whether its containers pass their
 * probes, and a screen showing only the first would call a broken workload
 * healthy.
 *
 * The namespace filter is in the shell above rather than on this card: it narrows
 * eight pages now rather than this one.
 */
export function KubernetesPodsPage() {
  const { selected } = useClusterSelection()
  const [diagnosing, setDiagnosing] = useState<{ namespace: string; pod: string } | null>(null)

  return (
    <KubernetesShell>
      <KubernetesAnswer>
        {(overview) => (
          <Card>
            <CardHeader>
              <CardTitle>Pods</CardTitle>
            </CardHeader>
            <CardContent>
              <DataTable
                label="Pods"
                rows={overview.pods}
                keyOf={(pod) => `${pod.namespace}/${pod.name}`}
                empty={
                  <>
                    The API server answered, and there are no pods
                    {overview.namespace === '' ? '' : ` in ${overview.namespace}`}.
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
        )}
      </KubernetesAnswer>

      {diagnosing && (
        <PodDiagnosis
          clusterID={selected}
          namespace={diagnosing.namespace}
          pod={diagnosing.pod}
          onClose={() => setDiagnosing(null)}
        />
      )}
    </KubernetesShell>
  )
}

export const kubernetesPodsRoute = createRoute({
  getParentRoute: () => kubernetesRoute,
  path: 'pods',
  component: KubernetesPodsPage,
})
