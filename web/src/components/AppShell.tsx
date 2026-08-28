import { Navigate, Outlet } from '@tanstack/react-router'
import { ChainBannerContainer } from '@/components/ChainBanner'
import { Header } from '@/components/Header'
import { Sidebar } from '@/components/Sidebar'
import { useSession } from '@/hooks/useSession'

/**
 * The permanent shell (D-10). It wraps every authenticated route: sidebar,
 * header, and the content area later phases render into.
 *
 * It is also the single gate in front of those routes:
 *   - while no operator account exists, everything goes to /setup (D-01);
 *   - without a live session, everything goes to /login.
 *
 * Neither of those is a security decision -- the server refuses regardless.
 * This only keeps the operator from staring at a screen full of 401s.
 */
export function AppShell() {
  const { setupRequired, authenticated, loading } = useSession()

  if (setupRequired) {
    return <Navigate to="/setup" replace />
  }

  if (loading) {
    return (
      <div className="flex h-dvh items-center justify-center text-sm text-muted-foreground">
        Loading…
      </div>
    )
  }

  if (!authenticated) {
    return <Navigate to="/login" replace search={{ reason: 'required' }} />
  }

  return (
    <div className="flex h-dvh flex-col">
      {/* Above the sidebar and the header, so a chain break is visible on
          every page regardless of where the operator navigated (D-15). */}
      <ChainBannerContainer />

      <div className="flex min-h-0 flex-1">
        <Sidebar />
        <div className="flex min-w-0 flex-1 flex-col">
          <Header />
          <main className="min-h-0 flex-1 overflow-auto p-6">
            <Outlet />
          </main>
        </div>
      </div>
    </div>
  )
}
