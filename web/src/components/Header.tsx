import { useNavigate } from '@tanstack/react-router'
import { LogOut, Menu } from 'lucide-react'
import { ThemeToggle } from '@/components/ThemeToggle'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { useSession } from '@/hooks/useSession'

/**
 * The permanent header: who is signed in, the theme switch, and the way out.
 * All copy is English with no translation layer (D-09).
 */
export function Header({ onOpenNav }: { onOpenNav?: () => void }) {
  const { me, logout, loggingOut } = useSession()
  const navigate = useNavigate()

  return (
    <header className="flex h-14 shrink-0 items-center gap-3 border-b border-border px-4">
      {/* The way to the navigation on a phone, where the sidebar is a drawer.
          Below md only: above it the bar is permanently there and a button
          that opened what is already open would be a second way to do
          nothing. */}
      <Button
        type="button"
        variant="ghost"
        size="sm"
        aria-label="Open the navigation"
        className="-ml-2 md:hidden"
        onClick={onOpenNav}
      >
        <Menu aria-hidden="true" className="size-5" />
      </Button>

      {/* Pushes the rest to the right, which is what justify-end used to do
          before the menu button needed the left edge. */}
      <div className="flex-1" />

      {me && (
        // The username is the first thing to go when the bar is 390px wide:
        // it is the least useful of the three and the only one that is not a
        // control.
        <span className="hidden text-sm text-muted-foreground sm:inline">
          Signed in as <span className="text-foreground">{me.username}</span>
        </span>
      )}

      {/* UAT G-01-5: Separator's vertical variant carries `self-stretch`, and a
          stretched item with an explicit height falls back to flex-start -- so
          this rule hung off the top edge of the 56px header instead of sitting
          between the two things it separates. */}
      <Separator orientation="vertical" className="h-5 data-vertical:self-center" />

      <ThemeToggle />

      <Button
        type="button"
        variant="ghost"
        size="sm"
        disabled={loggingOut}
        onClick={() => {
          // Say why the login screen is showing. Without this the shell's own
          // guard would redirect with "a session is required", which is true
          // but reads like a failure rather than like the thing just asked for.
          void logout().then(() =>
            navigate({ to: '/login', search: { reason: 'signed-out' }, replace: true }),
          )
        }}
      >
        <LogOut aria-hidden="true" className="size-4" />
        {loggingOut ? 'Signing out…' : 'Sign out'}
      </Button>
    </header>
  )
}
