import { useMutation, useQuery } from '@tanstack/react-query'
import { createRoute } from '@tanstack/react-router'
import { AlertTriangle, Download, Info, ShieldCheck } from 'lucide-react'
import { type FormEvent, useState } from 'react'
import { api } from '@/api'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { authenticatedRoute } from '@/routes/__root'

/**
 * Settings: the operator's own account, and what this installation is.
 *
 * The password form is the reason this screen exists rather than the other way
 * round. `POST /api/v1/account/password` and the 428 sudo flow have been in
 * place since phase 1 and there was no way to reach either from the interface
 * — gap G-01-1 in `01-UAT.md`, deferred to this phase because the entry point
 * belongs to the screen this phase builds.
 *
 * The form's behaviour under the sudo dialog is the part worth being careful
 * about. Changing a password is destructive, so the server answers 428 and the
 * dialog appears — and when it is cancelled or the password is wrong, **what
 * was typed here is still here**. That works because the request is a promise
 * the caller is still awaiting: nothing unmounts, and this component keeps its
 * own state throughout.
 */
export function SettingsPage() {
  return (
    <section className="space-y-5">
      <div>
        <h1 className="font-heading text-xl font-semibold tracking-tight">Settings</h1>
        <p className="max-w-prose text-sm text-muted-foreground">
          This operator's account, and what this installation supports.
        </p>
      </div>

      <PasswordCard />
      <SupportCard />
      <BackupCard />
    </section>
  )
}

/* ---------------------------------------------------------------------- */

export function PasswordCard() {
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [confirm, setConfirm] = useState('')
  const [message, setMessage] = useState('')

  const change = useMutation({
    mutationFn: () => api.changePassword(current, next),
    onSuccess: () => {
      setMessage('Your password has been changed.')
      setCurrent('')
      setNext('')
      setConfirm('')
    },
  })

  function submit(e: FormEvent) {
    e.preventDefault()
    setMessage('')

    // Checked here and not only on the server, because the server cannot see
    // the confirmation field at all — it takes one new password. A mismatch is
    // a typing mistake, and catching it before the request is what stops
    // somebody changing their password to something they cannot reproduce.
    if (next !== confirm) {
      setMessage('The two new passwords do not match.')
      return
    }
    change.mutate()
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Password</CardTitle>
      </CardHeader>
      <CardContent>
        <form className="max-w-md space-y-3" onSubmit={submit}>
          <div className="space-y-1.5">
            <Label htmlFor="current-password">Current password</Label>
            <Input
              id="current-password"
              type="password"
              autoComplete="current-password"
              value={current}
              onChange={(e) => setCurrent(e.target.value)}
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="new-password">New password</Label>
            <Input
              id="new-password"
              type="password"
              autoComplete="new-password"
              value={next}
              onChange={(e) => setNext(e.target.value)}
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="confirm-password">New password again</Label>
            <Input
              id="confirm-password"
              type="password"
              autoComplete="new-password"
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
            />
          </div>

          <p className="text-xs text-muted-foreground">
            Changing a password is a destructive action, so the server asks for your password again
            before it goes through. Cancelling that dialog leaves what you have typed here exactly
            as it is.
          </p>

          <Button type="submit" disabled={change.isPending || !current || !next}>
            {change.isPending ? 'Changing…' : 'Change password'}
          </Button>

          {message && <p className="text-sm">{message}</p>}
          {change.error ? (
            <p className="text-sm text-destructive">
              {change.error instanceof Error ? change.error.message : String(change.error)}
            </p>
          ) : null}
        </form>
      </CardContent>
    </Card>
  )
}

/* ---------------------------------------------------------------------- */

/**
 * OPS-03, as a statement about this build.
 *
 * The range is a claim this build makes about itself, so it is served rather
 * than written here: a copy in the bundle is a copy that drifts from the
 * constants that enforce it.
 */
export function SupportCard() {
  const status = useQuery({ queryKey: ['system', 'status'], queryFn: () => api.status() })

  const machines = useQuery({ queryKey: ['machines'], queryFn: () => api.machines.list() })
  const outside = (machines.data ?? []).filter((m) => m.unsupported_version)
  const preRelease = (machines.data ?? []).filter((m) => m.pre_release)

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Supported Talos versions</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="flex items-center gap-2 text-sm">
          <ShieldCheck aria-hidden="true" className="size-4" />
          {status.data?.talos_range
            ? `This build is tested against Talos ${status.data.talos_range}.`
            : 'Loading…'}
        </p>

        <p className="max-w-prose text-sm text-muted-foreground">
          A node outside that range is marked in the node list and every version-dependent action
          against it is refused. The client library would happily talk to it, which is the problem:
          an untested API surface that answers is worse than one that refuses, because the
          divergence surfaces later, on a cluster.
        </p>

        <p className="max-w-prose text-sm text-muted-foreground">
          Pre-releases are accepted only when this instance was started with{' '}
          <code className="font-mono text-xs">--allow-prerelease</code>. Everything this product
          guarantees about a node is a claim about released Talos.
        </p>

        {outside.length > 0 && (
          <p className="flex max-w-prose gap-2 rounded-md border border-destructive/50 p-2.5 text-sm">
            <AlertTriangle aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-destructive" />
            <span>
              {outside.length} node{outside.length === 1 ? '' : 's'} outside the supported range:{' '}
              {outside.map((m) => m.hostname.value || m.id).join(', ')}
            </span>
          </p>
        )}

        {preRelease.length > 0 && (
          <p className="flex max-w-prose gap-2 rounded-md border border-border p-2.5 text-sm text-muted-foreground">
            <Info aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
            <span>
              {preRelease.length} node{preRelease.length === 1 ? '' : 's'} running a pre-release:{' '}
              {preRelease.map((m) => m.hostname.value || m.id).join(', ')}
            </span>
          </p>
        )}
      </CardContent>
    </Card>
  )
}

/* ---------------------------------------------------------------------- */

/**
 * OPS-01, from the screen's side: what to run, and why it is a subcommand
 * rather than a button.
 *
 * There is deliberately no "back up now" button. A backup writes a tarball
 * containing every secret in the data directory verbatim, and a browser is the
 * wrong place to start one: the file lands on the server's disk, not the
 * operator's, so a button would produce a file somebody then has to find over
 * SSH anyway — and a button that streamed it to the browser would be an
 * endpoint that hands the cluster over to whoever has a session.
 */
export function BackupCard() {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Backup and restore</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3 text-sm">
        <p className="flex items-center gap-2">
          <Download aria-hidden="true" className="size-4" />
          Backups are a subcommand of the same binary, run on the host.
        </p>

        <pre className="overflow-x-auto rounded-md border border-border p-3 font-mono text-xs">
          {`holzkube-managerd backup            # writes a tarball into the data directory
holzkube-managerd backups           # lists what is there
holzkube-managerd restore FILE      # backs up what is there, then unpacks
holzkube-managerd verify-audit      # checks the audit hash chain`}
        </pre>

        <p className="max-w-prose text-muted-foreground">
          There is no button for this, and that is deliberate. A backup contains every secret in the
          data directory verbatim; the file lands on the server's disk rather than yours, so a
          button would produce a file you would have to go and find anyway — and one that streamed
          it to the browser would be an endpoint that hands the cluster to whoever has a session.
        </p>

        <p className="max-w-prose text-muted-foreground">
          A backup is safe to take while holzkube-manager is running: every record is written
          atomically, so a tarball taken mid-write captures either the old record or the new one and
          never half of one. A <span className="font-mono">restore</span> is not — it refuses while
          another instance holds the data directory, and backs up what is there before it starts.
        </p>
      </CardContent>
    </Card>
  )
}

export const settingsRoute = createRoute({
  getParentRoute: () => authenticatedRoute,
  path: '/settings',
  component: SettingsPage,
})
