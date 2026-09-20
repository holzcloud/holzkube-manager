import { createRoute } from '@tanstack/react-router'
import { ClusterInventory } from '@/components/ClusterInventory'
import { KubernetesShell, useClusterSelection } from '@/components/KubernetesSection'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { kubernetesRoute } from '@/routes/kubernetes'

/**
 * Namespaces, their quotas, and the kinds this cluster defines itself.
 */
export function KubernetesNamespacesPage() {
  const { selected } = useClusterSelection()

  return (
    <KubernetesShell>
      <Card>
        <CardHeader>
          <CardTitle>Namespaces and the cluster's own kinds</CardTitle>
        </CardHeader>
        <CardContent>
          <ClusterInventory clusterID={selected} />
        </CardContent>
      </Card>
    </KubernetesShell>
  )
}

export const kubernetesNamespacesRoute = createRoute({
  getParentRoute: () => kubernetesRoute,
  path: 'namespaces',
  component: KubernetesNamespacesPage,
})
