import { Navigate, Outlet } from '@tanstack/react-router'
import { useState } from 'react'
import { CertificateBanner } from '@/components/CertificateBanner'
import { ChainBannerContainer } from '@/components/ChainBanner'
import { DryRunBannerContainer } from '@/components/DryRunBanner'
import { Header } from '@/components/Header'
import { ResumeAfterProvider, SudoFailureNotice } from '@/components/ResumeAfterProvider'
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

  // The drawer's state lives here because two things open it and three close
  // it: the header's button, the backdrop, and following a link. Owned by one
  // of them, the others would each need their own rule.
  const [navOpen, setNavOpen] = useState(false)

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

      {/* Alongside it, for the same reason and at the same level: which mode
          the process is in is a fact about every screen, not about one
          (FOUND-12). Both can apply at once and both are then shown. */}
      <DryRunBannerContainer />

      {/* And the third fact that is about every screen rather than one: a
          cluster whose client certificate is about to expire. When it does,
          every node in that cluster goes unreachable in the same second, and
          the operator who was not warned reads that as a dead cluster (D-23). */}
      <CertificateBanner className="px-4 pt-2" />

      {/* And a fourth: what the operator was doing before the identity
          provider took the page away. It belongs here rather than on the
          screen the action started on, because the provider hands the browser
          back to the dashboard and that is where they are standing. */}
      {/* The refusal comes first: it says why nothing happened, and the
          banner below only says what it was. Reading them the other way round
          would tell the operator to try again before telling them it cannot
          currently work. */}
      <SudoFailureNotice className="mx-4 mt-2" />
      <ResumeAfterProvider className="mx-4 mt-2" />

      <div className="flex min-h-0 flex-1">
        <Sidebar open={navOpen} onNavigate={() => setNavOpen(false)} />

        {/* The backdrop, below md and only while the drawer is open. It is a
            button rather than a div so that closing the drawer is reachable
            without a pointer, and it carries a label because "" is what a
            screen reader would otherwise read out. */}
        {navOpen && (
          <button
            type="button"
            aria-label="Close the navigation"
            className="fixed inset-0 z-40 bg-background/70 md:hidden"
            onClick={() => setNavOpen(false)}
          />
        )}

        <div className="flex min-w-0 flex-1 flex-col">
          <Header onOpenNav={() => setNavOpen(true)} />
          {/* 24px of padding on each side is 12% of a 390px phone. */}
          <main className="min-h-0 flex-1 overflow-auto p-4 md:p-6">
            <Outlet />
          </main>
        </div>
      </div>
    </div>
  )
}
