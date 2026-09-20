import { useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { replaySudoAction, type SudoReplay } from '@/api'
import { notify } from '@/components/Toaster'
import { Button } from '@/components/ui/button'
import { messageFor, ProblemError } from '@/lib/problem'

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
 * WHAT IS REMEMBERED IS NEVER A REQUEST BODY. The 428 flow replays the original
 * request unchanged, and carrying that across a navigation would mean putting
 * its body in sessionStorage -- where, for the password change that is also
 * gated this way, it would be the password.
 *
 * A METHOD AND A PATH CARRY NOTHING SECRET, so since 2026-09-20 those are
 * remembered too, for requests that have no body. The banner then offers the
 * ACTION rather than a way back to the page it started on.
 *
 * That changed because of what the operator reported: they pressed a button to
 * forget a machine, were taken to their provider and back, and met a banner
 * whose first words were "did not run" and whose only button took them to a
 * list. They read it as the deletion having failed and stopped -- the journal
 * shows the sudo window opened correctly and no second attempt was ever made.
 * The product knew what was wanted, had the window open, and asked them to go
 * and find the button again (ledger 165).
 *
 * An action WITH a body still asks them to repeat it, and the banner says so
 * rather than offering a button that would send an empty one.
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
  /** The request to re-issue, when it has no body. See the file comment. */
  replay?: SudoReplay
  at: number
}

/** Remembers what was being attempted, just before the page is handed over. */
export function rememberSudoIntent(action: string, path: string, replay?: SudoReplay): void {
  try {
    const intent: SudoIntent = { action, path, replay, at: Date.now() }
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
    // A replay is checked rather than trusted: this comes out of storage the
    // page itself wrote, but a malformed one would send a request built from
    // whatever was there. Anything that is not a method and a path is dropped,
    // and the banner falls back to offering the way back.
    const replay = intent.replay
    if (
      replay !== undefined &&
      (typeof replay.method !== 'string' ||
        typeof replay.path !== 'string' ||
        !replay.path.startsWith('/api/v1/'))
    ) {
      return { ...(intent as SudoIntent), replay: undefined }
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
  const queryClient = useQueryClient()
  const [intent, setIntent] = useState<SudoIntent | null>(null)
  const [running, setRunning] = useState(false)
  const [failure, setFailure] = useState('')

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

  const replay = intent.replay
  const run = async () => {
    if (replay === undefined) return
    setRunning(true)
    setFailure('')
    try {
      await replaySudoAction(replay)
      // Everything, rather than a guessed key: the banner cannot know which
      // screens the action changed, and a stale list after a deletion is how
      // somebody concludes it did not work -- which is the whole complaint
      // this change answers.
      await queryClient.invalidateQueries()
      notify.success(`${intent.action} ran.`)
      dismiss()
    } catch (cause) {
      // Shown here rather than thrown: the banner is the only place this
      // attempt exists, so an error that escaped it would vanish.
      setFailure(
        cause instanceof ProblemError
          ? messageFor(cause.problem)
          : cause instanceof Error
            ? cause.message
            : 'It did not run, and the reason did not survive.',
      )
      setRunning(false)
    }
  }

  return (
    <div
      className={`rounded-md border border-amber-600/40 bg-amber-500/10 p-3 text-sm ${className ?? ''}`}
    >
      <p className="text-foreground">
        You were re-authenticated, and{' '}
        <strong className="font-medium">{intent.action} has not run yet</strong> — it could not be
        carried across the trip to your identity provider.{' '}
        {replay === undefined
          ? 'Go back and run it again; it will not ask a second time.'
          : 'It will not ask a second time.'}
      </p>
      {failure !== '' && <p className="mt-1 text-destructive">{failure}</p>}
      <div className="mt-2 flex flex-wrap gap-2">
        {replay !== undefined && (
          <Button
            size="sm"
            className="max-md:h-11"
            disabled={running}
            onClick={() => {
              void run()
            }}
          >
            {running ? 'Running…' : `Run ${lowerFirst(intent.action)} now`}
          </Button>
        )}
        <Button
          size="sm"
          variant="outline"
          className="max-md:h-11"
          onClick={() => {
            const to = intent.path
            dismiss()
            void navigate({ to })
          }}
        >
          Back to where you were
        </Button>
        <Button size="sm" variant="ghost" className="max-md:h-11" onClick={dismiss}>
          Dismiss
        </Button>
      </div>
    </div>
  )
}

/**
 * lowerFirst makes "Forgetting this machine" fit inside "Run … now".
 *
 * Only the first letter, and only when the second is not also upper case: an
 * action named after something like "CA rotation" must not become "cA rotation".
 */
function lowerFirst(sentence: string): string {
  if (sentence.length < 2 || sentence[1] !== sentence[1]?.toLowerCase()) {
    return sentence
  }
  return sentence[0]?.toLowerCase() + sentence.slice(1)
}
