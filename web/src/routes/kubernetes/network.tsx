import { createRoute } from '@tanstack/react-router'
import { ClusterNetwork } from '@/components/ClusterNetwork'
import { KubernetesShell, useClusterSelection } from '@/components/KubernetesSection'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { kubernetesRoute } from '@/routes/kubernetes'

/**
 * The cluster's networking, on its own page.
 *
 * Services with what is actually behind them, the policies, and the namespaces
 * that have none.
 */
export function KubernetesNetworkPage() {
  const { selected, namespace } = useClusterSelection()

  return (
    <KubernetesShell>
      <Card>
        <CardHeader>
          <CardTitle>Networking</CardTitle>
        </CardHeader>
        <CardContent>
          <ClusterNetwork clusterID={selected} namespace={namespace} />
        </CardContent>
      </Card>
    </KubernetesShell>
  )
}

export const kubernetesNetworkRoute = createRoute({
  getParentRoute: () => kubernetesRoute,
  path: 'network',
  component: KubernetesNetworkPage,
})
