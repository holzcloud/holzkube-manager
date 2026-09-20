import { createRoute } from '@tanstack/react-router'
import { ApplyManifest } from '@/components/ApplyManifest'
import { ClusterResources } from '@/components/ClusterResources'
import { KubernetesShell, useClusterSelection } from '@/components/KubernetesSection'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { kubernetesRoute } from '@/routes/kubernetes'

/**
 * ConfigMaps, Secrets, claims, Ingresses, autoscalers and budgets.
 *
 * The manifest form is on this page too: it is how any of them is changed, and
 * putting the reading and the writing of the same objects apart would make
 * somebody navigate between two pages to do one thing.
 */
export function KubernetesConfigPage() {
  const { selected, namespace } = useClusterSelection()

  return (
    <KubernetesShell>
      <Card>
        <CardHeader>
          <CardTitle>Configuration and routing</CardTitle>
        </CardHeader>
        <CardContent>
          <ClusterResources clusterID={selected} namespace={namespace} />
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
    </KubernetesShell>
  )
}

export const kubernetesConfigRoute = createRoute({
  getParentRoute: () => kubernetesRoute,
  path: 'config',
  component: KubernetesConfigPage,
})
