import { createRoute } from '@tanstack/react-router'
import { useState } from 'react'
import { ClusterCapacityPanel } from '@/components/ClusterCapacityPanel'
import { ClusterUsage } from '@/components/ClusterUsage'
import { DataTable } from '@/components/DataTable'
import {
  KubernetesAnswer,
  KubernetesShell,
  useClusterSelection,
} from '@/components/KubernetesSection'
import { NodeSchedulingActions } from '@/components/NodeSchedulingActions'
import { NodeWhy } from '@/components/NodeWhy'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { kubernetesRoute } from '@/routes/kubernetes'

/**
 * The overview: the nodes as Kubernetes sees them, and how full the cluster is.
 *
 * The page somebody lands on, so it answers the two questions they arrive with —
 * is everything up, and is there room — and nothing else. Everything that was
 * below it on the old single page is a page of its own now.
 */
export function KubernetesOverviewPage() {
  const { selected, namespace } = useClusterSelection()
  const [inspecting, setInspecting] = useState<string | null>(null)

  return (
    <KubernetesShell>
      <KubernetesAnswer>
        {(overview) => (
          <div className="space-y-4">
            <p className="text-muted-foreground text-xs">API server {overview.server_version}</p>

            <Card>
              <CardHeader>
                <CardTitle>Nodes</CardTitle>
              </CardHeader>
              <CardContent>
                <DataTable
                  label="Kubernetes nodes"
                  rows={overview.nodes}
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
                    {
                      key: 'roles',
                      label: 'Roles',
                      render: (node) => node.roles.join(', ') || '—',
                    },
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
                <CardTitle>What is being used</CardTitle>
              </CardHeader>
              <CardContent>
                <ClusterUsage clusterID={selected} namespace={namespace} />
              </CardContent>
            </Card>
          </div>
        )}
      </KubernetesAnswer>

      {inspecting && (
        <NodeWhy clusterID={selected} node={inspecting} onClose={() => setInspecting(null)} />
      )}
    </KubernetesShell>
  )
}

export const kubernetesOverviewRoute = createRoute({
  getParentRoute: () => kubernetesRoute,
  path: '/',
  component: KubernetesOverviewPage,
})
