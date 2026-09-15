import { useQuery } from '@tanstack/react-query'
import { Sparkles } from 'lucide-react'
import { useMemo, useState } from 'react'
import { api, type Release } from '@/api'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { cn } from '@/lib/utils'

/**
 * The version this instance is running, and what changed in it.
 *
 * It exists because the answer to "which build is this host on" used to require
 * an ssh session and `holzkube-managerd --version`, which meant nobody asked --
 * and an operator who does not know what they are running cannot tell a bug
 * from a version that predates the fix.
 *
 * The number comes from the server rather than from the bundle. A constant
 * baked into the frontend at build time would be a second copy of the version,
 * and the one thing this component must never do is disagree with the binary it
 * is talking to.
 */
export function WhatsNew({ className }: { className?: string }) {
  const [open, setOpen] = useState(false)

  // Read once and kept: the version of a running process does not change under
  // it. staleTime Infinity rather than a long number, because "never refetch"
  // is the actual claim and a large number is a guess at it.
  const info = useQuery({
    queryKey: ['system', 'version'],
    queryFn: () => api.version(),
    staleTime: Number.POSITIVE_INFINITY,
    retry: false,
  })

  // A version that could not be read is not rendered as "unknown" or as a
  // spinner in the corner of every screen. The sidebar is not the place to
  // report a failed background read -- there is nothing an operator would do
  // about it from here, and a permanent error in the chrome teaches people to
  // ignore the chrome.
  if (!info.isSuccess || info.data.version === '') {
    return null
  }

  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        className={cn(
          'w-full rounded px-1 text-left text-xs text-sidebar-foreground/60',
          'hover:text-sidebar-accent-foreground hover:underline underline-offset-2',
          'focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring',
          className,
        )}
        title="What changed in this release"
      >
        {info.data.version}
      </button>

      <ReleaseNotes
        open={open}
        onOpenChange={setOpen}
        version={info.data.version}
        releases={info.data.releases}
      />
    </>
  )
}

/**
 * Whether two version strings name the same release, ignoring a leading v.
 *
 * They disagree about it, and both are right where they are. goreleaser stamps
 * the binary with its `{{ .Version }}`, which is the tag with the v stripped,
 * so a released build reports `1.16.0`; the changelog spells versions exactly
 * as the git tag does, `v1.16.0`, because that is what the release pipeline
 * checks the file against before it creates the tag. Neither can simply adopt
 * the other's spelling: deploy/holzkube-manager-update.sh compares the tag with
 * its v stripped against what the binary reports, so a binary that reported the
 * v would look out of date on every single run and reinstall itself forever.
 *
 * So the panel normalises instead of insisting. This was found by driving the
 * released artifact rather than a development build -- a dev build reports
 * `v1.16.0-3-g1234-dirty` from git describe, which matches no release and
 * therefore hid the mismatch behind a case that is supposed to match nothing.
 */
function sameVersion(a: string, b: string): boolean {
  return a.replace(/^v/, '') === b.replace(/^v/, '')
}

/**
 * The panel itself, exported so it can be rendered in a test without a query
 * client, a session and a server behind it.
 */
export function ReleaseNotes({
  open,
  onOpenChange,
  version,
  releases,
}: {
  open: boolean
  onOpenChange: (next: boolean) => void
  version: string
  releases: Release[]
}) {
  // Grouped by series, in the order the releases arrive, which is newest first.
  // Built from the server's own `series` field rather than by splitting the
  // version string again here: one implementation of "which minor is this",
  // and it is not this one.
  const series = useMemo(() => {
    const order: string[] = []
    const bySeries = new Map<string, Release[]>()
    for (const release of releases) {
      const existing = bySeries.get(release.series)
      if (existing === undefined) {
        order.push(release.series)
        bySeries.set(release.series, [release])
      } else {
        existing.push(release)
      }
    }
    return order.map((name) => ({ name, releases: bySeries.get(name) ?? [] }))
  }, [releases])

  // The series the running build belongs to, so the panel opens on what the
  // operator is actually running rather than on whatever happens to be first.
  // They differ the moment somebody opens this on a host that is behind.
  const runningSeries = releases.find((r) => sameVersion(r.version, version))?.series
  const [selected, setSelected] = useState<string | null>(null)
  const active = selected ?? runningSeries ?? series[0]?.name ?? null
  const shown = series.find((s) => s.name === active)

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {/* flex-col rather than the base grid, and it is the whole reason the
          list scrolls. DialogContent lays its children out in a grid with
          auto-sized rows, and an auto row takes an automatic minimum size from
          its content -- so max-h-[85vh] clipped the panel while the list went
          on rendering underneath it, off the bottom of the box. The last
          release in the file was simply not reachable. In a column, min-h-0 on
          the scroller lets that row shrink and the overflow becomes a
          scrollbar. Only a browser shows this: jsdom has no layout. */}
      <DialogContent className="flex max-h-[85vh] flex-col gap-4 sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Sparkles aria-hidden="true" className="size-5 text-primary" />
            What's New
          </DialogTitle>
          <DialogDescription>
            This instance is running <span className="font-medium">{version}</span>.
          </DialogDescription>
        </DialogHeader>

        {series.length > 1 && (
          <div className="flex flex-wrap gap-1.5">
            {series.map((entry) => (
              <button
                key={entry.name}
                type="button"
                onClick={() => setSelected(entry.name)}
                aria-pressed={entry.name === active}
                className={cn(
                  'rounded-md border px-2.5 py-1 text-sm',
                  'focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring',
                  entry.name === active
                    ? 'border-primary bg-primary/10 font-medium text-primary'
                    : 'border-transparent text-muted-foreground hover:bg-muted',
                )}
              >
                {entry.name}
              </button>
            ))}
          </div>
        )}

        <div className="min-h-0 flex-1 space-y-6 overflow-y-auto pr-1">
          {shown?.releases.map((release) => (
            <section key={release.version} className="space-y-3">
              <div className="flex flex-wrap items-baseline gap-2">
                <span
                  className={cn(
                    'rounded px-2 py-0.5 text-sm font-medium',
                    sameVersion(release.version, version)
                      ? 'bg-primary text-primary-foreground'
                      : 'bg-muted text-muted-foreground',
                  )}
                >
                  {release.version}
                </span>
                {sameVersion(release.version, version) && (
                  <span className="text-xs text-muted-foreground">running here</span>
                )}
                <span className="text-xs text-muted-foreground">
                  {release.changes.length}
                  {release.changes.length === 1 ? ' change' : ' changes'}
                </span>
                {release.date !== '' && (
                  <span className="ml-auto text-xs text-muted-foreground tabular-nums">
                    {release.date}
                  </span>
                )}
              </div>

              <ul className="space-y-3">
                {release.changes.map((change) => (
                  <li key={change.text} className="flex gap-3 text-sm leading-relaxed">
                    <span aria-hidden="true" className="shrink-0 select-none">
                      {change.icon}
                    </span>
                    <span>{change.text}</span>
                  </li>
                ))}
              </ul>
            </section>
          ))}

          {shown === undefined && (
            <p className="text-sm text-muted-foreground">This build carries no release notes.</p>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}
