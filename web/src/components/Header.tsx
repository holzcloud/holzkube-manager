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

  return (
    <header className="flex h-14 shrink-0 items-center justify-end gap-3 border-b border-border px-4">
      {me && (
        <span className="text-sm text-muted-foreground">
          Signed in as <span className="text-foreground">{me.username}</span>
        </span>
      )}

      <Separator orientation="vertical" className="h-5" />

      <ThemeToggle />

      <Button
        type="button"
        variant="ghost"
        size="sm"
        disabled={loggingOut}
        onClick={() => {
          void logout()
        }}
      >
        <LogOut aria-hidden="true" className="size-4" />
        {loggingOut ? 'Signing out…' : 'Sign out'}
      </Button>
    </header>
  )
}
