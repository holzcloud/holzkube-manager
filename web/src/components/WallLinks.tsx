import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { api, type WallLink } from '@/api'
import { DataTable } from '@/components/DataTable'
import { Problem } from '@/components/Problem'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

/**
 * The links a screen in a corridor is left open on (2026-09-20).
 *
 * # Why this credential exists
 *
 * A session expires. A television showing a sign-in page at three in the morning
 * is worse than no wall at all: it stopped answering the one question it was put
 * up for, and nobody notices until somebody needs it.
 *
 * # Shown once, and the screen says so before it is made
 *
 * Only the hash is stored, so a lost link is replaced rather than recovered.
 * That sentence is on the page before anybody presses the button and again
 * beside the link, because "you will not see this again" after the fact is a
 * warning that arrives too late to act on.
 *
 * # What it can reach, said plainly
 *
 * One route. It reads. It can never change anything, and it cannot see the audit
 * archive, a Secret's key names or a cluster's configuration. That is worth
 * saying on the screen rather than leaving somebody to infer it, because the
 * thing they are about to do is put a URL on a television.
 */
export function WallLinks({ clusterID }: { clusterID: string }) {
  const queryClient = useQueryClient()
  const [label, setLabel] = useState('')
  const [minted, setMinted] = useState<{ url: string; notice: string } | null>(null)

  const links = useQuery({ queryKey: ['wall-links'], queryFn: () => api.wallLinks.list() })

  const create = useMutation({
    mutationFn: (name: string) => api.wallLinks.create(name),
    onSuccess: (result) => {
      // Built here rather than on the server: the address a screen reaches this
      // installation on is the browser's, and the server knows only the Host
      // header of whoever asked. A link built from that would be right for the
      // administrator and wrong for the wall often enough to be a trap.
      const url = `${window.location.origin}/wall?k=${encodeURIComponent(result.token)}${
        clusterID === '' ? '' : `&cluster=${encodeURIComponent(clusterID)}`
      }`
      setMinted({ url, notice: result.notice })
      setLabel('')
      void queryClient.invalidateQueries({ queryKey: ['wall-links'] })
    },
  })

  const revoke = useMutation({
    mutationFn: (id: string) => api.wallLinks.revoke(id),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ['wall-links'] }),
  })

  return (
    <div className="space-y-4">
      <p className="text-muted-foreground text-sm">
        A link opens the wall and nothing else: it reads, it can never change anything, and it
        cannot see the audit archive, a Secret’s key names or a cluster’s configuration. Anybody
        holding it can see the wall, so treat it as the credential it is — and revoke it here when
        the screen comes down.
      </p>

      <form
        className="flex flex-wrap items-end gap-2"
        onSubmit={(event) => {
          event.preventDefault()
          if (label.trim() !== '') create.mutate(label.trim())
        }}
      >
        <div className="space-y-1">
          <label className="text-sm" htmlFor="wall-link-label">
            Which screen is this for?
          </label>
          <Input
            id="wall-link-label"
            className="w-72 max-md:h-11 max-md:w-full"
            value={label}
            onChange={(event) => setLabel(event.target.value)}
            placeholder="The screen in the IT office"
          />
        </div>
        <Button
          type="submit"
          className="max-md:h-11"
          disabled={label.trim() === '' || create.isPending}
        >
          {create.isPending ? 'Making…' : 'Make a link'}
        </Button>
      </form>

      {/* Said BEFORE the button is pressed as well as after: a warning that
          arrives once the link is on screen is one that came too late to act
          on. */}
      {minted === null && (
        <p className="text-muted-foreground text-xs">
          The link is shown once. Only its hash is kept, so it cannot be recovered — a lost one is
          revoked and replaced.
        </p>
      )}

      {create.error ? <Problem error={create.error} /> : null}
      {revoke.error ? <Problem error={revoke.error} /> : null}

      {minted !== null && (
        <div className="space-y-2 rounded-md border border-amber-600/40 bg-amber-500/10 p-3">
          <p className="font-medium text-sm">Open this on the screen, once, and bookmark it.</p>
          <code className="block break-all rounded bg-background/60 p-2 font-mono text-xs">
            {minted.url}
          </code>
          <p className="text-muted-foreground text-xs">{minted.notice}</p>
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="max-md:h-11"
            onClick={() => setMinted(null)}
          >
            I have it
          </Button>
        </div>
      )}

      <DataTable
        label="Wall links"
        rows={links.data?.links ?? []}
        keyOf={(link: WallLink) => link.id}
        empty="No screen has a link yet. A wall opened without one needs somebody to sign in on it, and that session will expire."
        columns={[
          {
            key: 'label',
            label: 'Screen',
            role: 'identity',
            render: (link) => <span className="break-all text-xs">{link.label}</span>,
          },
          {
            key: 'used',
            label: 'Last used',
            // The question this list exists for. A link nothing has ever used is
            // one somebody made and never put on a screen, and it is the first
            // one to revoke.
            render: (link) =>
              link.last_used_at === '' ? (
                <span className="text-muted-foreground text-xs">never used</span>
              ) : (
                <span className="text-xs tabular-nums">{link.last_used_at}</span>
              ),
          },
          {
            key: 'made',
            label: 'Made',
            role: 'detail',
            render: (link) => (
              <span className="text-xs tabular-nums">
                {link.created_at}
                {link.created_by === '' ? '' : ` by ${link.created_by}`}
              </span>
            ),
          },
          {
            key: 'revoke',
            label: 'Action',
            role: 'actions',
            render: (link) => (
              <Button
                type="button"
                size="sm"
                variant="destructive"
                className="max-md:h-11"
                disabled={revoke.isPending}
                onClick={() => revoke.mutate(link.id)}
              >
                Revoke
              </Button>
            ),
          },
        ]}
      />
      {links.error ? <Problem error={links.error} /> : null}
    </div>
  )
}
