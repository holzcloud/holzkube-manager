import { useMutation, useQuery } from '@tanstack/react-query'
import { createRoute } from '@tanstack/react-router'
import { Activity, AlertTriangle, Download, Info, ShieldCheck } from 'lucide-react'
import { type FormEvent, type ReactNode, useState } from 'react'
import { api, roleAtLeast } from '@/api'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { WallLinks } from '@/components/WallLinks'
import { authenticatedRoute } from '@/routes/__root'
import { AccountsCard } from '@/routes/accounts'

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
/**
 * Renders its children only for an admin session.
 *
 * It is presentation and never enforcement: every route behind it refuses on
 * its own, and a client that ignored this would meet 403 rather than get
 * anywhere. What it buys is that an operator does not see a panel whose every
 * button answers "you may not".
 *
 * While the identity is still loading it renders nothing rather than
 * optimistically showing the panel, because a panel that appears and then
 * vanishes reads as a bug.
 */
function AdminOnly({ children }: { children: ReactNode }) {
  const me = useQuery({ queryKey: ['me'], queryFn: () => api.me() })
  if (!me.data || !roleAtLeast(me.data.role, 'admin')) {
    return null
  }
  return <>{children}</>
}

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
      <AdminOnly>
        <AccountsCard />
      </AdminOnly>
      <AdminOnly>
        <WallLinksCard />
      </AdminOnly>
      <SupportCard />
      <MetricsCard />
      <BackupCard />
    </section>
  )
}

/* ---------------------------------------------------------------------- */

/**
 * The links a screen in a corridor is left open on.
 *
 * The cluster is pinned into the link because a kiosk link opens the wall route
 * and nothing else -- a screen showing one cannot list the clusters to pick the
 * first, so the address has to name it.
 */
export function WallLinksCard() {
  const clusters = useQuery({ queryKey: ['clusters'], queryFn: api.clusters.list })

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Wall links</CardTitle>
      </CardHeader>
      <CardContent>
        <WallLinks clusterID={clusters.data?.[0]?.id ?? ''} />
      </CardContent>
    </Card>
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
/**
 * Where the Prometheus endpoint is, because nothing else says.
 *
 * `/metrics` needs no session — a scraper cannot log in — so it is reachable
 * and completely undiscoverable: there is no link to it, and an operator who
 * does not already know the convention has no way to learn this instance
 * exports anything at all. That is the shape of a feature that ships and is
 * never used.
 *
 * It is a card of prose rather than a link, deliberately. Following it in a
 * browser produces a page of text nobody wants to read; what an operator needs
 * is the address to paste into a scrape config, and the boundary this product
 * keeps.
 */
export function MetricsCard() {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Metrics</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3 text-sm">
        <p className="flex items-center gap-2">
          <Activity aria-hidden="true" className="size-4" />
          <span>
            Prometheus can scrape <span className="font-mono">/metrics</span> on this instance.
          </span>
        </p>

        <pre className="overflow-x-auto rounded-md border border-border p-3 font-mono text-xs">
          {`scrape_configs:
  - job_name: holzkube-manager
    scheme: https
    static_configs:
      - targets: ['${typeof window === 'undefined' ? 'localhost:8443' : window.location.host}']`}
        </pre>

        <p className="max-w-prose text-muted-foreground">
          It needs no session, because a scraper has none. What guards it is the same host allowlist
          that guards every other route here, and a listener that stays on loopback unless you moved
          it.
        </p>

        <p className="max-w-prose text-muted-foreground">
          What it exports is what this instance knows and nothing else has: nodes per stage, seconds
          left on each cluster's client certificate — negative once it has expired, because an
          expired certificate is a state and not a missing number — job records by kind, confirmed
          etcd members, and whether the audit chain verified at startup. No node ever appears as a
          label, so the number of series follows the number of clusters rather than the size of the
          fleet. This exports; it does not alert, and it keeps no history — that is the scraper's
          job.
        </p>
      </CardContent>
    </Card>
  )
}

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
