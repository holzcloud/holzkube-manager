import { createRouter, RouterProvider } from '@tanstack/react-router'
import { routeTree } from '@/routeTree'

/** Router wiring, and nothing else. The tree itself lives in routeTree.ts. */
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
