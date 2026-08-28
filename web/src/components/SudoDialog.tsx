import { type FormEvent, useCallback, useEffect, useRef, useState } from 'react'
import { api, onSessionExpired, onSudoRequired, type SudoChallenge } from '@/api'
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
          <DialogTitle>Confirm your password</DialogTitle>
          <DialogDescription>
            {challenge?.action ?? 'This destructive action'} changes something that cannot simply be
            undone, so holzkube asks for your password again before it runs.
          </DialogDescription>
        </DialogHeader>

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
