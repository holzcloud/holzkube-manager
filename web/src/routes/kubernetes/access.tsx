import { createRoute } from '@tanstack/react-router'
import { AccessControl } from '@/components/AccessControl'
import { ActAs } from '@/components/ActAs'
import { KubernetesShell, useClusterSelection } from '@/components/KubernetesSection'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { kubernetesRoute } from '@/routes/kubernetes'

/**
 * Who may do what, on its own page.
 *
 * Its own page rather than a card among fifteen, because "who administers this
 * cluster" is a question somebody arrives with rather than scrolls past.
 */
export function KubernetesAccessPage() {
  const { selected, namespace } = useClusterSelection()

  return (
    <KubernetesShell>
      <Card>
        <CardHeader>
          <CardTitle>Who may do what</CardTitle>
        </CardHeader>
        <CardContent>
          <AccessControl clusterID={selected} namespace={namespace} />
        </CardContent>
      </Card>

      {/* Beside it rather than elsewhere: the page says what every identity in
          the cluster may do, and this one says which of them this product is. */}
      <Card>
        <CardHeader>
          <CardTitle>Who this acts as</CardTitle>
        </CardHeader>
        <CardContent>
          <ActAs clusterID={selected} />
        </CardContent>
      </Card>
    </KubernetesShell>
  )
}

export const kubernetesAccessRoute = createRoute({
  getParentRoute: () => kubernetesRoute,
  path: 'access',
  component: KubernetesAccessPage,
})
