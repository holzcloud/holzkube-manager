import { createRoute } from '@tanstack/react-router'
import { ClusterStorage } from '@/components/ClusterStorage'
import { KubernetesShell, useClusterSelection } from '@/components/KubernetesSection'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { kubernetesRoute } from '@/routes/kubernetes'

/**
 * The cluster's storage, on its own page.
 *
 * Volumes, claims and classes together, because the questions storage raises are
 * all answered by two of the three disagreeing -- whether deleting destroys the
 * data, who mounts a claim, why one is Pending.
 */
export function KubernetesStoragePage() {
  const { selected, namespace } = useClusterSelection()

  return (
    <KubernetesShell>
      <Card>
        <CardHeader>
          <CardTitle>Storage</CardTitle>
        </CardHeader>
        <CardContent>
          <ClusterStorage clusterID={selected} namespace={namespace} />
        </CardContent>
      </Card>
    </KubernetesShell>
  )
}

export const kubernetesStorageRoute = createRoute({
  getParentRoute: () => kubernetesRoute,
  path: 'storage',
  component: KubernetesStoragePage,
})
