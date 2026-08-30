import { useQueryClient } from '@tanstack/react-query'
import { createRoute, useNavigate } from '@tanstack/react-router'
import { type FormEvent, useEffect, useState } from 'react'
import { api } from '@/api'
import { SourceNotice } from '@/components/SourceNotice'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  fieldErrorsOf,
  messageFor,
  ProblemError,
  presentationFor,
  waitMessage,
  waitSecondsOf,
} from '@/lib/problem'
import { rootRoute } from '@/routes/__root'

/**
 * Sign-in. A session lasts 24 hours absolute (D-07), so an operator sees this
 * page roughly daily and it has to be a calm place rather than an accident.
 *
 * A wrong password and an unknown username produce the same answer from the
 * server by design, and this page repeats it verbatim rather than helpfully
 * distinguishing the two.
 *
 * Repeated failures are answered with a delay, never a lock (D-08). When the
 * server sends `429` with `Retry-After` the button is disabled for exactly that
 * long and the remaining seconds are counted down in plain sight. There is no
 * locked state, so this page must never suggest one and never offers an unlock.
 */
function LoginPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { reason } = loginRoute.useSearch()

  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [message, setMessage] = useState('')
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({})
  const [waitSeconds, setWaitSeconds] = useState(0)
  const [busy, setBusy] = useState(false)

  // Count the delay down rather than leaving the operator guessing.
  useEffect(() => {
    if (waitSeconds <= 0) {
      return
    }
    const timer = setTimeout(() => {
      setWaitSeconds((remaining) => {
        const next = remaining - 1
        if (next <= 0) {
          setMessage('')
        }
        return next
      })
    }, 1000)
    return () => clearTimeout(timer)
  }, [waitSeconds])

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setMessage('')
    setFieldErrors({})
    setBusy(true)
    try {
      await api.login(username, password)
      setPassword('')
      await queryClient.invalidateQueries()
      await navigate({ to: '/' })
    } catch (error) {
      if (error instanceof ProblemError && presentationFor(error.problem) === 'wait') {
        const seconds = waitSecondsOf(error) ?? 0
        setWaitSeconds(seconds)
        setMessage(waitMessage(seconds))
      } else {
        setFieldErrors(fieldErrorsOf(error))
        setMessage(messageFor(error))
      }
      setPassword('')
    } finally {
      setBusy(false)
    }
  }

  const waiting = waitSeconds > 0

  return (
    <div className="flex min-h-dvh flex-col items-center justify-center gap-4 p-6">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>Sign in</CardTitle>
          <CardDescription>{reasonText(reason)}</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={submit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="login-username">Username</Label>
              <Input
                id="login-username"
                value={username}
                onChange={(event) => setUsername(event.target.value)}
                autoComplete="username"
                aria-invalid={fieldErrors.username !== undefined}
                required
              />
              {fieldErrors.username !== undefined && (
                <p className="text-sm text-destructive">{fieldErrors.username}</p>
              )}
            </div>

            <div className="space-y-2">
              <Label htmlFor="login-password">Password</Label>
              <Input
                id="login-password"
                type="password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                autoComplete="current-password"
                aria-invalid={fieldErrors.password !== undefined}
                required
              />
              {fieldErrors.password !== undefined && (
                <p className="text-sm text-destructive">{fieldErrors.password}</p>
              )}
            </div>

            {message !== '' && (
              <p role="alert" className="text-sm text-destructive">
                {waiting ? waitMessage(waitSeconds) : message}
              </p>
            )}

            <Button type="submit" disabled={busy || waiting} className="w-full">
              {busy ? 'Signing in…' : waiting ? `Wait ${waitSeconds}s` : 'Sign in'}
            </Button>
          </form>
        </CardContent>
      </Card>
      <SourceNotice className="text-xs text-muted-foreground" />
    </div>
  )
}

export type LoginReason = 'required' | 'expired' | 'signed-out' | undefined

function reasonText(reason: LoginReason): string {
  switch (reason) {
    case 'expired':
      return 'Your session ended. Sessions last 24 hours; sign in again and nothing you had open is lost.'
    case 'signed-out':
      return 'You are signed out.'
    default:
      return 'holzkube-manager needs a session before it will show you anything.'
  }
}

export const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/login',
  validateSearch: (search: Record<string, unknown>): { reason?: LoginReason } => {
    const reason = search.reason
    return reason === 'required' || reason === 'expired' || reason === 'signed-out'
      ? { reason }
      : {}
  },
  component: LoginPage,
})
