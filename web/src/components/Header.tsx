import { useNavigate } from '@tanstack/react-router'
import { LogOut } from 'lucide-react'
import { ThemeToggle } from '@/components/ThemeToggle'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { useSession } from '@/hooks/useSession'

/**
 * The permanent header: who is signed in, the theme switch, and the way out.
 * All copy is English with no translation layer (D-09).
 */
export function Header() {
  const { me, logout, loggingOut } = useSession()
  const navigate = useNavigate()

  return (
    <header className="flex h-14 shrink-0 items-center justify-end gap-3 border-b border-border px-4">
      {me && (
        <span className="text-sm text-muted-foreground">
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
