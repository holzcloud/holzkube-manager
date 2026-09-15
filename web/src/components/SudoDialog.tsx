import { useQuery } from '@tanstack/react-query'
import { type FormEvent, useCallback, useEffect, useRef, useState } from 'react'
import { api, type Me, oidcPath, onSessionExpired, onSudoRequired, type SudoChallenge } from '@/api'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { SESSION_QUERY_KEY } from '@/hooks/useSession'
import { messageFor, type Problem, ProblemError, presentationFor, waitMessage } from '@/lib/problem'

/**
 * The client half of the 428 flow (D-05).
 *
 * A destructive route answers `428 sudo.required` when the five-minute window
 * is shut -- including for a session that just signed in, because logging in
 * does not open the window; only POST /api/v1/auth/sudo does. This dialog is
 * how that refusal becomes a question instead of an error.
 *
 * It is presentation only. The window lives in the session on the server and
 * the server decides; nothing here grants anything (threat T-01-33). Cancelling
 * abandons the action and leaves the operator's form exactly as it was, because
 * the pending request is a promise the caller is still awaiting -- no component
 * unmounts and no state is thrown away.
 */
export function SudoDialog() {
  // Whether this session was established through the identity provider. An
  // operator who signed in that way has no local password to type here, and on
  // a host the operator declared SSO-only the password routes are refused
  // outright -- so the dialog that only ever offered a password field was a
  // dead end: every destructive action in the product, unreachable, with a
  // form that could not be completed and a refusal that read like a mistake.
  //
  // Read with the same query key the settings screen uses, so the answer is the
  // one already in the cache rather than another round trip at the moment an
  // operator is waiting on a dialog.
  // Read from the cache the shell already fills and NEVER fetch: enabled:false
  // subscribes to the identity without asking for it. This dialog says of
  // itself that it is presentation only, and a version that issued its own
  // request the moment a destructive action was refused would have made that
  // false -- it also broke every one of this file's own tests, which is how it
  // was caught. Before the shell has an answer there is none here either, and
  // the password form is the right fallback: it is what a local account uses.
  const { data: me } = useQuery<Me>({
    queryKey: SESSION_QUERY_KEY,
    queryFn: api.me,
    enabled: false,
  })
  const throughProvider = me?.sso === true

  const [challenge, setChallenge] = useState<SudoChallenge | null>(null)
  const [password, setPassword] = useState('')
  const [message, setMessage] = useState('')
  const [busy, setBusy] = useState(false)

  // A challenge must be settled exactly once: twice would resolve a promise
  // the pipeline already moved past, never would hang the caller forever.
  const settled = useRef(false)

  useEffect(() => {
    onSudoRequired((next) => {
      setChallenge((current) => {
        // A challenge that is being displaced is a challenge that was refused.
        // Resetting `settled` without settling the outgoing one first left its
        // `askForSudo` promise unresolved forever: `send` never returned, the
        // caller's `await` hung for the life of the page, and there was no
        // error and no toast to show for it -- only a spinner that never
        // stopped. One destructive route exists today, so a single page cannot
        // reach this yet; phase 6's node actions gate through this same dialog.
        if (current !== null && !settled.current) {
          current.settle(false)
        }
        settled.current = false
        return next
      })
      setPassword('')
      setMessage('')
    })
    return () => onSudoRequired(null)
  }, [])

  const settle = useCallback(
    (granted: boolean) => {
      if (challenge !== null && !settled.current) {
        settled.current = true
        challenge.settle(granted)
      }
      setChallenge(null)
      setPassword('')
      setBusy(false)
    },
    [challenge],
  )

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setMessage('')
    setBusy(true)
    try {
      await api.sudo(password)
      settle(true)
    } catch (error) {
      // A wrong password here is 401: the session is fine, the credential was
      // not. The dialog stays open so the operator can try again without
      // losing the action that is still waiting behind it.
      if (error instanceof ProblemError && presentationFor(error.problem) === 'wait') {
        setMessage(waitMessage(error.retryAfterSeconds))
      } else {
        setMessage(messageFor(error))
      }
      setPassword('')
      setBusy(false)
    }
  }

  const open = challenge !== null

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) {
          settle(false)
        }
      }}
    >
      <DialogContent showCloseButton={false}>
        <DialogHeader>
          <DialogTitle>
            {throughProvider ? 'Confirm with your identity provider' : 'Confirm your password'}
          </DialogTitle>
          <DialogDescription>
            {challenge?.action ?? 'This destructive action'}{' '}
            {challenge?.because ??
              (throughProvider
                ? 'changes something that cannot simply be undone, so holzkube-manager asks your ' +
                  'identity provider to confirm it is still you'
                : 'changes something that cannot simply be undone, so holzkube-manager asks for your ' +
                  'password again before it runs')}
            .
          </DialogDescription>
        </DialogHeader>

        {throughProvider ? (
          <div className="space-y-4">
            <p className="text-sm text-muted-foreground">
              You signed in through your identity provider, so there is no password here to type.
              Confirming takes you there and back.{' '}
              <strong className="font-medium text-foreground">
                This action is not carried across
              </strong>{' '}
              — when you return, run it again. It will not ask a second time.
            </p>

            <DialogFooter>
              <Button type="button" variant="ghost" onClick={() => settle(false)}>
                Cancel
              </Button>
              <Button
                type="button"
                onClick={() => {
                  // Settle the waiting request FIRST. Navigating away with it
                  // unsettled leaves the caller's promise pending for the life
                  // of the page -- the defect this file already records once,
                  // with no error and no toast, only a spinner that never
                  // stops. Refused is the honest answer: nothing was confirmed.
                  settle(false)
                  window.location.assign(oidcPath.reauthenticate)
                }}
              >
                Continue to your provider
              </Button>
            </DialogFooter>
          </div>
        ) : (
          <form onSubmit={submit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="sudo-password">Password</Label>
              <Input
                id="sudo-password"
                type="password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                autoComplete="current-password"
                required
              />
            </div>

            {message !== '' && <p className="text-sm text-destructive">{message}</p>}

            <DialogFooter>
              <Button type="button" variant="ghost" onClick={() => settle(false)} disabled={busy}>
                Cancel
              </Button>
              <Button type="submit" disabled={busy}>
                {busy ? 'Confirming…' : 'Confirm'}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
}

/**
 * The client half of the 401 flow.
 *
 * Sessions are 24 hours absolute (D-07), so an operator meets this during
 * normal work rather than as an exception. `onExpired` receives the decoded
 * problem so the destination can say why the session ended instead of
 * presenting an empty login form.
 */
export function SessionExpiryWatcher({ onExpired }: { onExpired: (problem: Problem) => void }) {
  useEffect(() => {
    onSessionExpired(onExpired)
    return () => onSessionExpired(null)
  }, [onExpired])

  return null
}
