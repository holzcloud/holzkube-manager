import { createRouter, RouterProvider } from '@tanstack/react-router'
import { authenticatedRoute, rootRoute } from '@/routes/__root'
import { auditRoute } from '@/routes/audit'
import { clustersRoute } from '@/routes/clusters'
import { configRoute } from '@/routes/config'
import { imagesRoute } from '@/routes/images'
import { indexRoute } from '@/routes/index'
import { jobsRoute } from '@/routes/jobs'
import { kubernetesRoute } from '@/routes/kubernetes'
import { kubernetesAccessRoute } from '@/routes/kubernetes/access'
import { appDetailRoute } from '@/routes/kubernetes/app-detail'
import { kubernetesAppsRoute } from '@/routes/kubernetes/apps'
import { kubernetesConfigRoute } from '@/routes/kubernetes/config'
import { kubernetesEventsRoute } from '@/routes/kubernetes/events'
import { kubernetesMaintenanceRoute } from '@/routes/kubernetes/maintenance'
import { kubernetesNamespacesRoute } from '@/routes/kubernetes/namespaces'
import { kubernetesNetworkRoute } from '@/routes/kubernetes/network'
import { kubernetesOverviewRoute } from '@/routes/kubernetes/overview'
import { kubernetesPodsRoute } from '@/routes/kubernetes/pods'
import { kubernetesStorageRoute } from '@/routes/kubernetes/storage'
import { kubernetesWorkloadsRoute } from '@/routes/kubernetes/workloads'
import { loginRoute } from '@/routes/login'
import { nodeDetailRoute } from '@/routes/node-detail'
import { nodesRoute } from '@/routes/nodes'
import { placeholderRoutes } from '@/routes/placeholders'
import { provisionRoute } from '@/routes/provision'
import { settingsRoute } from '@/routes/settings'
import { setupRoute } from '@/routes/setup'
import { upgradesRoute } from '@/routes/upgrades'
import { wallRoute } from '@/routes/wall'

/**
 * Router wiring, and nothing else.
 *
 * /setup and /login sit directly under the root because they are the two
 * screens that exist before there is a session to build a shell around.
 * Everything else hangs under the authenticated layout and therefore inherits
 * the permanent shell (D-10).
 */

const routeTree = rootRoute.addChildren([
  setupRoute,
  loginRoute,
  // Outside the authenticated layout: a wall has no sidebar. See wall.tsx.
  wallRoute,
  authenticatedRoute.addChildren([
    indexRoute,
    auditRoute,
    imagesRoute,
    nodesRoute,
    nodeDetailRoute,
    clustersRoute,
    kubernetesRoute.addChildren([
      kubernetesOverviewRoute,
      kubernetesAppsRoute,
      appDetailRoute,
      kubernetesWorkloadsRoute,
      kubernetesPodsRoute,
      kubernetesStorageRoute,
      kubernetesNetworkRoute,
      kubernetesConfigRoute,
      kubernetesNamespacesRoute,
      kubernetesAccessRoute,
      kubernetesEventsRoute,
      kubernetesMaintenanceRoute,
    ]),
    jobsRoute,
    configRoute,
    provisionRoute,
    upgradesRoute,
    settingsRoute,
    ...placeholderRoutes,
  ]),
])

const router = createRouter({
  routeTree,
  defaultPreload: 'intent',
})

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}

export default function App() {
  return <RouterProvider router={router} />
}
