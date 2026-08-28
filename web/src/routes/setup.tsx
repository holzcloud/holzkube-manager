import { useQueryClient } from '@tanstack/react-query'
import { createRoute, useNavigate } from '@tanstack/react-router'
import { type FormEvent, useState } from 'react'
import { api } from '@/api'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useSystemStatus } from '@/hooks/useSession'
import { rootRoute } from '@/routes/__root'

/**
 * The setup wizard (D-01): the one place the first operator account is created,
 * without ever opening a terminal.
 *
 * Once an account exists the route stays reachable but is not usable: the
 * server answers 409 setup.already-completed, and this page shows that answer.
 * Hiding the route would be a UI convention; the refusal is the actual rule.
 */
function SetupPage() {
  const status = useSystemStatus()
  const navigate = useNavigate()
  const queryClient = useQueryClient()

  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [message, setMessage] = useState('')
  const [busy, setBusy] = useState(false)

  const alreadyDone = status.isSuccess && !status.data.setup_required

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setMessage('')

    if (password !== confirm) {
      setMessage('The two passwords do not match.')
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
      setMessage(error instanceof Error ? error.message : 'Something went wrong.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex min-h-dvh items-center justify-center p-6">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>Create the operator account</CardTitle>
          <CardDescription>
            This runs once. There is no second account and no recovery path.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {alreadyDone ? (
            <p className="text-sm text-muted-foreground">
              An operator account already exists. Setup can only run once.
            </p>
          ) : (
            <form onSubmit={submit} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="setup-username">Username</Label>
                <Input
                  id="setup-username"
                  value={username}
                  onChange={(event) => setUsername(event.target.value)}
                  autoComplete="username"
                  required
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="setup-password">Password</Label>
                <Input
                  id="setup-password"
                  type="password"
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  autoComplete="new-password"
                  required
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="setup-confirm">Repeat password</Label>
                <Input
                  id="setup-confirm"
                  type="password"
                  value={confirm}
                  onChange={(event) => setConfirm(event.target.value)}
                  autoComplete="new-password"
                  required
                />
              </div>

              {message !== '' && <p className="text-sm text-destructive">{message}</p>}

              <Button type="submit" disabled={busy} className="w-full">
                {busy ? 'Creating…' : 'Create account'}
              </Button>
            </form>
          )}
        </CardContent>
      </Card>
    </div>
  )
}

export const setupRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/setup',
  component: SetupPage,
})
