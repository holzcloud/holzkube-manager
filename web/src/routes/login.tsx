import { useQueryClient } from '@tanstack/react-query'
import { createRoute, useNavigate } from '@tanstack/react-router'
import { type FormEvent, useEffect, useState } from 'react'
import { api, oidcPath } from '@/api'
import { SourceNotice } from '@/components/SourceNotice'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useSystemStatus } from '@/hooks/useSession'
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
 *
 * Which ways in are offered comes from the server, per address. The same
 * instance can answer on a LAN address that accepts both the identity provider
 * and the local account, and on a public name that accepts only the provider --
 * so this page renders what `system/status` reports for the address it was
 * loaded from, rather than assuming one shape and failing on the other.
 */
function LoginPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { reason, sso_error: ssoError } = loginRoute.useSearch()
  const status = useSystemStatus()

  // Until the answer arrives, offer the password form: it is what every
  // deployment has, and a page that renders nothing while it waits looks broken
  // on the slowest connections, which are the ones least able to afford it.
  const ssoAvailable = status.data?.oidc_enabled === true
  const passwordAvailable = status.data?.password_login !== false

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
        <CardContent className="space-y-4">
          {ssoErrorMessage(ssoError) !== '' && (
            <p role="alert" className="text-sm text-destructive">
              {ssoErrorMessage(ssoError)}
            </p>
          )}

          {ssoAvailable && (
            <div className="space-y-2">
              <Button
                type="button"
                className="w-full"
                onClick={() => {
                  // A full navigation, not a fetch: the authorization code flow
                  // is a sequence of browser redirects, and an XHR would follow
                  // them invisibly and land the provider's login page in a
                  // response body nobody renders.
                  window.location.assign(oidcPath.signIn)
                }}
              >
                Continue with single sign-on
              </Button>
              {!passwordAvailable && (
                <p className="text-sm text-muted-foreground">
                  This address accepts single sign-on only. The local account works on the local
                  network.
                </p>
              )}
            </div>
          )}

          {ssoAvailable && passwordAvailable && (
            <div className="flex items-center gap-3" aria-hidden="true">
              <span className="h-px flex-1 bg-border" />
              <span className="text-xs text-muted-foreground">or</span>
              <span className="h-px flex-1 bg-border" />
            </div>
          )}

          {passwordAvailable && (
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

              <Button
                type="submit"
                variant={ssoAvailable ? 'outline' : 'default'}
                disabled={busy || waiting}
                className="w-full"
              >
                {busy ? 'Signing in…' : waiting ? `Wait ${waitSeconds}s` : 'Sign in'}
              </Button>
            </form>
          )}

          {!ssoAvailable && !passwordAvailable && (
            <p role="alert" className="text-sm text-destructive">
              This address offers no way to sign in. Single sign-on is not configured, and the local
              account is not accepted here.
            </p>
          )}
        </CardContent>
      </Card>
      <SourceNotice className="text-xs text-muted-foreground" />
    </div>
  )
}

export type LoginReason = 'required' | 'expired' | 'signed-out' | undefined

/**
 * Why a single sign-on attempt ended back here.
 *
 * The server sends a stable code rather than a message: those routes are
 * browser navigations, and a problem document rendered as raw JSON in the
 * address bar is where this flow used to leave people standing. The wording
 * lives here, where it can name the next step.
 */
const ssoErrorText: Record<string, string> = {
  'bind-host':
    'This account is not linked to single sign-on yet. Linking has to happen from the local network — ' +
    'sign in there once, and this address will work afterwards.',
  'setup-required':
    'This instance has no operator account yet. It has to be created from the local network.',
  'other-identity': 'This instance is linked to a different account at the identity provider.',
  denied:
    'The identity provider refused the sign-in. Check that your account is assigned to this application.',
  'provider-unreachable':
    'The identity provider could not be reached. If it is down, the local account still works on the local network.',
  'no-flow': 'That sign-in took too long or was started in another browser. Try again.',
  'state-mismatch': 'That sign-in could not be matched to the one this browser started. Try again.',
  'no-code': 'The identity provider returned no authorisation code.',
  'exchange-failed': 'The sign-in could not be completed. The server log has the detail.',
}

function ssoErrorMessage(code: string | undefined): string {
  if (code === undefined) {
    return ''
  }
  // An unknown code still says something: a newer server may send one this
  // page has not learned yet, and silence would look like the click did
  // nothing at all.
  return ssoErrorText[code] ?? 'The sign-in did not complete.'
}

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
  validateSearch: (
    search: Record<string, unknown>,
  ): { reason?: LoginReason; sso_error?: string } => {
    const reason = search.reason
    const out: { reason?: LoginReason; sso_error?: string } =
      reason === 'required' || reason === 'expired' || reason === 'signed-out' ? { reason } : {}

    // Kept as a bare string rather than a union: the server owns this
    // vocabulary, and a page that dropped codes it did not recognise would
    // answer a newer server with silence.
    if (typeof search.sso_error === 'string' && search.sso_error !== '') {
      out.sso_error = search.sso_error
    }
    return out
  },
  component: LoginPage,
})
