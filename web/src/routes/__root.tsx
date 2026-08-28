import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createRootRoute, createRoute, Outlet, useNavigate } from '@tanstack/react-router'
import { useCallback, useEffect } from 'react'
import { AppShell } from '@/components/AppShell'
import { SessionExpiryWatcher, SudoDialog } from '@/components/SudoDialog'
import { notify, Toaster } from '@/components/Toaster'
import { applyTheme, resolveTheme } from '@/hooks/useTheme'
import { messageFor, type Problem, ProblemError } from '@/lib/problem'
import { ErrorPage } from '@/routes/error'

/**
 * The router root: the query client, the theme, the toast surface and the
 * global error boundary. Everything below this is a route.
 */

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      // The operator is looking at cluster state; a stale answer presented as
      // current is the UX pitfall this project cares most about avoiding.
      staleTime: 5_000,
      refetchOnWindowFocus: true,
      retry: false,
    },
  },
})

function RootLayout() {
  const navigate = useNavigate()

  // The inline script in index.html has already set the class for the first
  // paint. This keeps <html> correct if the module reloads during development.
  useEffect(() => {
    applyTheme(resolveTheme())
  }, [])

  const onExpired = useCallback(
    (problem: Problem) => {
      // Everything cached was read with a session that no longer exists.
      queryClient.clear()
      notify.info('Your session ended.', messageFor(new ProblemError(problem)))
      void navigate({ to: '/login', search: { reason: 'expired' }, replace: true })
    },
    [navigate],
  )

  return (
    <QueryClientProvider client={queryClient}>
      <Outlet />
      <SudoDialog />
      <SessionExpiryWatcher onExpired={onExpired} />
      <Toaster />
    </QueryClientProvider>
  )
}

export const rootRoute = createRootRoute({
  component: RootLayout,
  errorComponent: ErrorPage,
  notFoundComponent: () => <ErrorPage error={new Error('This page does not exist.')} />,
})

/**
 * The pathless layout every authenticated route hangs under. Its component is
 * the permanent AppShell, so a later phase adds a page and inherits the
 * sidebar, the header, the chain banner and the session gate for free (D-10).
 */
export const authenticatedRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: 'authenticated',
  component: AppShell,
})
