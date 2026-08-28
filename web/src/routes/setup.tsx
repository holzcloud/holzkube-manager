import { useQueryClient } from '@tanstack/react-query'
import { createRoute, useNavigate } from '@tanstack/react-router'
import { type FormEvent, useState } from 'react'
import { api } from '@/api'
import { SourceNotice } from '@/components/SourceNotice'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useSystemStatus } from '@/hooks/useSession'
import { fieldErrorsOf, messageFor, ProblemError } from '@/lib/problem'
import { rootRoute } from '@/routes/__root'

/** Matches the server's own minimum; the server is still the one that decides. */
const MIN_PASSWORD_LENGTH = 12

/**
 * The setup wizard (D-01): the one place the first operator account is created,
 * without ever opening a terminal.
 *
 * Once an account exists the route stays reachable and stops being usable. It
 * does not merely hide: the server answers `409 setup.already-completed` and
 * this page shows that answer, because a hidden route is a convention while a
 * refusal is the rule. The direct hit is what proves the difference, so the
 * page renders the server's sentence rather than an empty form.
 */
function SetupPage() {
  const status = useSystemStatus()
  const navigate = useNavigate()
  const queryClient = useQueryClient()

  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [message, setMessage] = useState('')
  const [refusal, setRefusal] = useState('')
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({})
  const [busy, setBusy] = useState(false)

  const alreadyDone = refusal !== '' || (status.isSuccess && !status.data.setup_required)

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setMessage('')
    setFieldErrors({})

    if (password.length < MIN_PASSWORD_LENGTH) {
      setFieldErrors({
        password: `Use at least ${MIN_PASSWORD_LENGTH} characters. This password is the only thing standing in front of your cluster.`,
      })
      return
    }
    if (password !== confirm) {
      setFieldErrors({ confirm: 'The two passwords do not match.' })
      return
    }

    setBusy(true)
    try {
      await api.setup(username, password)
      setPassword('')
      setConfirm('')
      await queryClient.invalidateQueries()
      await navigate({ to: '/' })
    } catch (error) {
      // 409 setup.already-completed: somebody else got here first, or this tab
      // was open across a setup in another one. Show what the server said.
      if (error instanceof ProblemError && error.problem.status === 409) {
        setRefusal(messageFor(error))
      } else {
        setFieldErrors(fieldErrorsOf(error))
        setMessage(messageFor(error))
      }
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex min-h-dvh flex-col items-center justify-center gap-6 p-6">
      {/* UAT G-01-5: the first screen of the install named nothing. The login
          card at least says "holzkube" in its own copy; setup said only
          "Create the operator account", so the very first thing an operator
          sees never identified the system they were configuring. */}
      <div className="text-center">
        <span className="font-heading text-xl font-semibold tracking-tight">holzkube</span>
        <p className="text-sm text-muted-foreground">Talos cluster management</p>
      </div>
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>Create the operator account</CardTitle>
          <CardDescription>
            This runs once. There is no second account and no recovery path.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {alreadyDone ? (
            <div className="space-y-4">
              <p role="alert" className="text-sm text-muted-foreground">
                {refusal !== ''
                  ? refusal
                  : 'An operator account already exists. Setup can only run once.'}
              </p>
              <Button
                type="button"
                variant="secondary"
                onClick={() => {
                  void navigate({ to: '/login' })
                }}
              >
                Go to sign in
              </Button>
            </div>
          ) : (
            <form onSubmit={submit} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="setup-username">Username</Label>
                <Input
                  id="setup-username"
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
                <Label htmlFor="setup-password">Password</Label>
                <Input
                  id="setup-password"
                  type="password"
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  autoComplete="new-password"
                  aria-invalid={fieldErrors.password !== undefined}
                  required
                />
                {fieldErrors.password !== undefined && (
                  <p className="text-sm text-destructive">{fieldErrors.password}</p>
                )}
              </div>

              <div className="space-y-2">
                <Label htmlFor="setup-confirm">Repeat password</Label>
                <Input
                  id="setup-confirm"
                  type="password"
                  value={confirm}
                  onChange={(event) => setConfirm(event.target.value)}
                  autoComplete="new-password"
                  aria-invalid={fieldErrors.confirm !== undefined}
                  required
                />
                {fieldErrors.confirm !== undefined && (
                  <p className="text-sm text-destructive">{fieldErrors.confirm}</p>
                )}
              </div>

              {message !== '' && (
                <p role="alert" className="text-sm text-destructive">
                  {message}
                </p>
              )}

              <Button type="submit" disabled={busy} className="w-full">
                {busy ? 'Creating…' : 'Create account'}
              </Button>
            </form>
          )}
        </CardContent>
      </Card>
      <SourceNotice className="text-xs text-muted-foreground" />
    </div>
  )
}

export const setupRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/setup',
  component: SetupPage,
})
