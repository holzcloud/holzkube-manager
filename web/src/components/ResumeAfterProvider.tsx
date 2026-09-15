import { useNavigate } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { Button } from '@/components/ui/button'

/**
 * What an operator was doing before the identity provider took the page away.
 *
 * The provider round trip is a full navigation: the page is unloaded, so the
 * request waiting behind the sudo dialog cannot survive it and is settled as
 * refused before leaving. Without this, an operator re-authenticated, landed on
 * the dashboard, and found the thing they had asked for still sitting there
 * undone -- which reads exactly like the action having failed. That happened to
 * the operator this was built for, on the first cluster they tried to remove.
 *
 * WHAT IS REMEMBERED IS THE INTENT AND NEVER THE REQUEST. The 428 flow replays
 * the original request unchanged, and carrying that across a navigation would
 * mean putting its body in sessionStorage -- where, for the password change that
 * is also gated this way, it would be the password. So this carries the
 * sentence the dialog showed and the screen it was shown on, and offers the way
 * back. The operator presses the action again; the window is open by then, so
 * it runs without asking.
 */

const KEY = 'holzkube.sudo-intent'

/**
 * How long an intent stays worth offering. Ten minutes is longer than a
 * provider round trip and shorter than a lunch break: the point is that a
 * banner from an hour ago, about something the operator has long since dealt
 * with another way, is noise that teaches them to dismiss banners.
 */
const MAX_AGE_MS = 10 * 60 * 1000

export interface SudoIntent {
  /** The sentence the dialog showed, so the banner names the same thing. */
  action: string
  /** Where it was started, so the way back is one click rather than a hunt. */
  path: string
  at: number
}

/** Remembers what was being attempted, just before the page is handed over. */
export function rememberSudoIntent(action: string, path: string): void {
  try {
    const intent: SudoIntent = { action, path, at: Date.now() }
    sessionStorage.setItem(KEY, JSON.stringify(intent))
  } catch {
    // A browser with storage refused is a browser where this banner does not
    // appear. It is a convenience on top of an action the operator can repeat
    // by hand, so failing quietly is the right shape -- the alternative is an
    // error about a reminder.
  }
}

function readSudoIntent(): SudoIntent | null {
  try {
    const raw = sessionStorage.getItem(KEY)
    if (raw === null) {
      return null
    }
    const intent = JSON.parse(raw) as Partial<SudoIntent>
    if (
      typeof intent.action !== 'string' ||
      typeof intent.path !== 'string' ||
      typeof intent.at !== 'number' ||
      Date.now() - intent.at > MAX_AGE_MS
    ) {
      sessionStorage.removeItem(KEY)
      return null
    }
    return intent as SudoIntent
  } catch {
    return null
  }
}

export function forgetSudoIntent(): void {
  try {
    sessionStorage.removeItem(KEY)
  } catch {
    // See rememberSudoIntent.
  }
}

/**
 * The banner. It states plainly that nothing was done, because the one thing an
 * operator must not have to guess at is whether a destructive action ran.
 */
/**
 * What the three sudo refusals mean, and who fixes each.
 *
 * They used to be one sentence behind a problem document the browser rendered
 * as raw JSON in the address bar. An operator met that three times in a row on
 * their own cluster, each time on a page they had no reason to read as an
 * error, and could not tell which refusal they were looking at -- the two that
 * matter need repairs from different places.
 */
const SUDO_ERRORS: Record<string, { title: string; detail: string }> = {
  'oidc.no-auth-time': {
    title: 'Your identity provider does not say when it last authenticated you',
    detail:
      'Confirming a destructive action means proving you were asked again just now, and the only ' +
      'thing that proves it is the auth_time claim in the ID token. It is optional in OIDC, and ' +
      'your provider is not sending it — so this confirmation cannot succeed however many times ' +
      'you try. Enable auth_time in the provider’s ID-token claims for this application. Until ' +
      'then, sign in with the local account on the local network to confirm destructive actions.',
  },
  'oidc.not-fresh': {
    title: 'Your identity provider did not ask you again',
    detail:
      'It was asked to re-authenticate you and answered with an older session instead. Accepting ' +
      'that would make this confirmation a redirect with nothing behind it. Sign out at the ' +
      'provider first and try again, or allow it to re-prompt for this application.',
  },
  'oidc.other-identity': {
    title: 'A different account came back',
    detail:
      'The re-authentication was completed by an account other than the one signed in here. ' +
      'Nothing was confirmed. Sign out at the provider and sign in as the same account.',
  },
}

/** Reads ?sudo_error= once and takes it out of the address bar. */
function useSudoError(): string | null {
  const [code, setCode] = useState<string | null>(null)

  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const found = params.get('sudo_error')
    if (found === null) {
      return
    }
    setCode(found)
    // Taken out of the URL so a reload does not resurrect a refusal the
    // operator has already read and acted on.
    params.delete('sudo_error')
    const rest = params.toString()
    window.history.replaceState({}, '', window.location.pathname + (rest ? `?${rest}` : ''))
  }, [])

  return code
}

export function SudoFailureNotice({ className }: { className?: string }) {
  const code = useSudoError()
  const [dismissed, setDismissed] = useState(false)

  if (code === null || dismissed) {
    return null
  }

  const known = SUDO_ERRORS[code]

  return (
    <div
      className={`rounded-md border border-destructive/40 bg-destructive/10 p-3 text-sm ${className ?? ''}`}
    >
      <p className="font-medium text-foreground">
        {known?.title ?? 'The confirmation with your identity provider was refused'}
      </p>
      <p className="mt-1 text-muted-foreground">
        {known?.detail ??
          `The provider’s answer was refused with the code ${code}. Nothing was confirmed and ` +
            'nothing ran. The server log carries the reason.'}
      </p>
      <Button size="sm" variant="ghost" className="mt-2" onClick={() => setDismissed(true)}>
        Dismiss
      </Button>
    </div>
  )
}

export function ResumeAfterProvider({ className }: { className?: string }) {
  const navigate = useNavigate()
  const [intent, setIntent] = useState<SudoIntent | null>(null)

  useEffect(() => {
    setIntent(readSudoIntent())
  }, [])

  if (intent === null) {
    return null
  }

  const dismiss = () => {
    forgetSudoIntent()
    setIntent(null)
  }

  return (
    <div
      className={`rounded-md border border-amber-600/40 bg-amber-500/10 p-3 text-sm ${className ?? ''}`}
    >
      <p className="text-foreground">
        You were re-authenticated.{' '}
        <strong className="font-medium">{intent.action} did not run</strong> — the action was not
        carried across the trip to your identity provider. Run it again and it will not ask a second
        time.
      </p>
      <div className="mt-2 flex gap-2">
        <Button
          size="sm"
          variant="outline"
          onClick={() => {
            const to = intent.path
            dismiss()
            void navigate({ to })
          }}
        >
          Back to where you were
        </Button>
        <Button size="sm" variant="ghost" onClick={dismiss}>
          Dismiss
        </Button>
      </div>
    </div>
  )
}
