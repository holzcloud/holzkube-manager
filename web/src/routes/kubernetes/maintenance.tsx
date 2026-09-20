import { createRoute } from '@tanstack/react-router'
import {
  KubernetesAnswer,
  KubernetesShell,
  useClusterSelection,
} from '@/components/KubernetesSection'
import { ReachService } from '@/components/ReachService'
import { TidyUp } from '@/components/TidyUp'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { kubernetesRoute } from '@/routes/kubernetes'

/**
 * The two things done TO a cluster rather than read from it.
 *
 * Clearing out what is finished, and reaching a service to see what it answers.
 * Neither belongs on a page somebody opens to find something out, and both are
 * easier to find when they are not the fourteenth card down.
 */
export function KubernetesMaintenancePage() {
  const { selected, namespace } = useClusterSelection()

  return (
    <KubernetesShell>
      <Card>
        <CardHeader>
          <CardTitle>Clear out what is finished</CardTitle>
        </CardHeader>
        <CardContent>
          <TidyUp clusterID={selected} namespace={namespace} />
        </CardContent>
      </Card>

      <KubernetesAnswer>
        {(overview) => (
          <Card>
            <CardHeader>
              <CardTitle>Reach a service</CardTitle>
            </CardHeader>
            <CardContent>
              <ReachService clusterID={selected} services={overview.services} />
            </CardContent>
          </Card>
        )}
      </KubernetesAnswer>
    </KubernetesShell>
  )
}

export const kubernetesMaintenanceRoute = createRoute({
  getParentRoute: () => kubernetesRoute,
  path: 'maintenance',
  component: KubernetesMaintenancePage,
})
