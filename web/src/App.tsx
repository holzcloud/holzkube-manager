import { createRouter, RouterProvider } from '@tanstack/react-router'
import { authenticatedRoute, rootRoute } from '@/routes/__root'
import { auditRoute } from '@/routes/audit'
import { clustersRoute } from '@/routes/clusters'
import { imagesRoute } from '@/routes/images'
import { indexRoute } from '@/routes/index'
import { jobsRoute } from '@/routes/jobs'
import { loginRoute } from '@/routes/login'
import { nodeDetailRoute } from '@/routes/node-detail'
import { nodesRoute } from '@/routes/nodes'
import { placeholderRoutes } from '@/routes/placeholders'
import { setupRoute } from '@/routes/setup'

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
  authenticatedRoute.addChildren([
    indexRoute,
    auditRoute,
    imagesRoute,
    nodesRoute,
    nodeDetailRoute,
    clustersRoute,
    jobsRoute,
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
