import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { api, USER_ROLE_SENTENCE, USER_ROLES, type User, type UserRoleName } from '@/api'
import { Problem } from '@/components/Problem'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

/**
 * Accounts and roles (V2-AUTH-02).
 *
 * The screen shows only to an admin, because the route does. What it spends
 * its words on is the one thing an interface can get wrong here that costs
 * somebody a shell on the host: a change that leaves nobody able to manage the
 * instance. The server refuses those, and this says why *before* the refusal
 * rather than after it — an error message arriving on a click is a worse
 * teacher than a disabled button with a reason on it.
 *
 * Every mutation here is destructive, so every one of them goes through the
 * existing sudo dialog. Nothing on this screen re-implements that.
 */
export function AccountsCard() {
  const queryClient = useQueryClient()
  const users = useQuery({ queryKey: ['users'], queryFn: () => api.users.list() })

  const refresh = () => queryClient.invalidateQueries({ queryKey: ['users'] })

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Accounts</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="max-w-prose text-sm text-muted-foreground">
          Three roles. A <strong>reader</strong> looks; an <strong>operator</strong> runs the fleet;
          an <strong>admin</strong> also manages accounts and can download the credentials that make
          this instance unnecessary.
        </p>

        {users.error ? <Problem error={users.error} /> : null}
        {users.data && <AccountTable users={users.data} onChanged={refresh} />}

        <NewAccountForm onCreated={refresh} />
      </CardContent>
    </Card>
  )
}

export function AccountTable({ users, onChanged }: { users: User[]; onChanged: () => void }) {
  const admins = users.filter((u) => u.role === 'admin')

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Account</TableHead>
          <TableHead>Role</TableHead>
          <TableHead>Sign-in</TableHead>
          <TableHead />
        </TableRow>
      </TableHeader>
      <TableBody>
        {users.map((u) => (
          <AccountRow key={u.id} user={u} adminCount={admins.length} onChanged={onChanged} />
        ))}
      </TableBody>
    </Table>
  )
}

function AccountRow({
  user,
  adminCount,
  onChanged,
}: {
  user: User
  adminCount: number
  onChanged: () => void
}) {
  const [resetting, setResetting] = useState(false)
  const [password, setPassword] = useState('')

  const setRole = useMutation({
    mutationFn: (role: UserRoleName) => api.users.setRole(user.id, role),
    onSuccess: onChanged,
  })
  const reset = useMutation({
    mutationFn: () => api.users.resetPassword(user.id, password),
    onSuccess: () => {
      setResetting(false)
      setPassword('')
    },
  })
  const remove = useMutation({
    mutationFn: () => api.users.remove(user.id),
    onSuccess: onChanged,
  })

  // The two rules the server enforces, stated here so the button carries the
  // reason instead of the click producing it. They are separate because the
  // remedies are: another admin can demote this one, and nobody can remove the
  // last one.
  const lastAdmin = user.role === 'admin' && adminCount === 1
  const selfDemotion = user.self && user.role === 'admin'

  const roleReason = lastAdmin
    ? 'This is the only admin. Promote another account first, or nobody can manage this instance.'
    : selfDemotion
      ? 'An account cannot take away its own admin role. Another admin can.'
      : undefined

  return (
    <TableRow>
      <TableCell className="text-sm">
        {user.username}
        {user.self && <span className="ml-2 text-xs text-muted-foreground">(you)</span>}
      </TableCell>

      <TableCell>
        <Select
          value={user.role}
          onValueChange={(value) => setRole.mutate(value as UserRoleName)}
          disabled={roleReason !== undefined || setRole.isPending}
        >
          <SelectTrigger
            className="h-8 w-32"
            aria-label={`Role of ${user.username}`}
            title={roleReason}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {USER_ROLES.map((role) => (
              <SelectItem key={role} value={role} title={USER_ROLE_SENTENCE[role]}>
                {role}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {roleReason && (
          <p className="mt-1 max-w-prose text-xs text-muted-foreground">{roleReason}</p>
        )}
      </TableCell>

      <TableCell className="text-sm text-muted-foreground">
        {user.linked_identity ? 'password and single sign-on' : 'password'}
      </TableCell>

      <TableCell>
        <div className="flex flex-col items-end gap-1">
          <div className="flex gap-2">
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => setResetting((open) => !open)}
            >
              Reset password
            </Button>
            <Button
              type="button"
              size="sm"
              variant="destructive"
              disabled={lastAdmin || remove.isPending}
              title={
                lastAdmin
                  ? 'This is the only admin. Removing it leaves nobody able to manage this instance.'
                  : undefined
              }
              onClick={() => remove.mutate()}
            >
              Remove
            </Button>
          </div>

          {resetting && (
            <div className="flex items-center gap-2">
              <Label htmlFor={`reset-${user.id}`} className="sr-only">
                New password for {user.username}
              </Label>
              <Input
                id={`reset-${user.id}`}
                type="password"
                value={password}
                className="h-8 w-64"
                placeholder="at least 12 characters"
                onChange={(event) => setPassword(event.target.value)}
              />
              <Button
                type="button"
                size="sm"
                disabled={password.length < 12 || reset.isPending}
                onClick={() => reset.mutate()}
              >
                Set
              </Button>
            </div>
          )}

          {setRole.error ? <Problem error={setRole.error} /> : null}
          {reset.error ? <Problem error={reset.error} /> : null}
          {remove.error ? <Problem error={remove.error} /> : null}
        </div>
      </TableCell>
    </TableRow>
  )
}

function NewAccountForm({ onCreated }: { onCreated: () => void }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [role, setRole] = useState<UserRoleName>('reader')

  const create = useMutation({
    mutationFn: () => api.users.create(username, password, role),
    onSuccess: () => {
      setUsername('')
      setPassword('')
      setRole('reader')
      onCreated()
    },
  })

  // Reader, and not operator, is the default that is chosen rather than the
  // one that is convenient. An account created by accident at the default
  // should be able to do the least.
  return (
    <form
      className="flex flex-wrap items-end gap-2"
      onSubmit={(event) => {
        event.preventDefault()
        create.mutate()
      }}
    >
      <div className="space-y-1.5">
        <Label htmlFor="new-username">New account</Label>
        <Input
          id="new-username"
          value={username}
          className="h-8 w-48"
          onChange={(event) => setUsername(event.target.value)}
        />
      </div>

      <div className="space-y-1.5">
        <Label htmlFor="new-password">Password</Label>
        <Input
          id="new-password"
          type="password"
          value={password}
          className="h-8 w-56"
          placeholder="at least 12 characters"
          onChange={(event) => setPassword(event.target.value)}
        />
      </div>

      <div className="space-y-1.5">
        <Label htmlFor="new-role">Role</Label>
        <Select value={role} onValueChange={(value) => setRole(value as UserRoleName)}>
          <SelectTrigger id="new-role" className="h-8 w-32">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {USER_ROLES.map((each) => (
              <SelectItem key={each} value={each} title={USER_ROLE_SENTENCE[each]}>
                {each}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <Button
        type="submit"
        size="sm"
        disabled={username.length < 3 || password.length < 12 || create.isPending}
      >
        {create.isPending ? 'Creating…' : 'Create account'}
      </Button>

      {create.error ? <Problem error={create.error} /> : null}
    </form>
  )
}
