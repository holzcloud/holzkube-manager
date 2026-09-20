import { createRoute } from '@tanstack/react-router'
import { KubernetesShell, useClusterSelection } from '@/components/KubernetesSection'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Workloads } from '@/components/Workloads'
import { kubernetesRoute } from '@/routes/kubernetes'

/**
 * Everything that runs, whatever kind it is -- and the buttons that act on it.
 */
export function KubernetesWorkloadsPage() {
  const { selected, namespace } = useClusterSelection()

  return (
    <KubernetesShell>
      <Card>
        <CardHeader>
          <CardTitle>Workloads</CardTitle>
        </CardHeader>
        <CardContent>
          <Workloads clusterID={selected} namespace={namespace} />
        </CardContent>
      </Card>
    </KubernetesShell>
  )
}

export const kubernetesWorkloadsRoute = createRoute({
  getParentRoute: () => kubernetesRoute,
  path: 'workloads',
  component: KubernetesWorkloadsPage,
})
