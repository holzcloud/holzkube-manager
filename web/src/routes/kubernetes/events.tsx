import { createRoute } from '@tanstack/react-router'
import { ClusterEvents } from '@/components/ClusterEvents'
import { KubernetesShell, useClusterSelection } from '@/components/KubernetesSection'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { kubernetesRoute } from '@/routes/kubernetes'

/**
 * What the cluster reported.
 *
 * Its own page because it is long and because it is what somebody reaches for
 * after every other page has told them WHAT is wrong: an event says when the
 * scheduler gave up, when an image could not be pulled, when a probe failed.
 */
export function KubernetesEventsPage() {
  const { selected, namespace } = useClusterSelection()

  return (
    <KubernetesShell>
      <Card>
        <CardHeader>
          <CardTitle>What the cluster reported</CardTitle>
        </CardHeader>
        <CardContent>
          <ClusterEvents clusterID={selected} namespace={namespace} />
        </CardContent>
      </Card>
    </KubernetesShell>
  )
}

export const kubernetesEventsRoute = createRoute({
  getParentRoute: () => kubernetesRoute,
  path: 'events',
  component: KubernetesEventsPage,
})
