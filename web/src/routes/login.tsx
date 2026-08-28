import { useQueryClient } from '@tanstack/react-query'
import { createRoute, useNavigate } from '@tanstack/react-router'
import { type FormEvent, useState } from 'react'
import { api } from '@/api'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { rootRoute } from '@/routes/__root'

/**
 * Sign-in. A session lasts 24 hours absolute (D-07), so an operator sees this
 * page roughly daily and it has to be a calm place rather than an accident.
 */
function LoginPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { reason } = loginRoute.useSearch()

  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [message, setMessage] = useState('')
  const [busy, setBusy] = useState(false)

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setMessage('')
    setBusy(true)
    try {
      await api.login(username, password)
      setPassword('')
      await queryClient.invalidateQueries()
      await navigate({ to: '/' })
    } catch (error) {
      setMessage(error instanceof Error ? error.message : 'Something went wrong.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex min-h-dvh items-center justify-center p-6">
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
                required
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="login-password">Password</Label>
              <Input
                id="login-password"
                type="password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                autoComplete="current-password"
                required
              />
            </div>

            {message !== '' && <p className="text-sm text-destructive">{message}</p>}

            <Button type="submit" disabled={busy} className="w-full">
              {busy ? 'Signing in…' : 'Sign in'}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}

export type LoginReason = 'required' | 'expired' | 'signed-out' | undefined

function reasonText(reason: LoginReason): string {
  switch (reason) {
    case 'expired':
      return 'Your session expired. Sessions last 24 hours; signing in again picks up where you left off.'
    case 'signed-out':
      return 'You are signed out.'
    default:
      return 'holzkube needs a session before it will show you anything.'
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
