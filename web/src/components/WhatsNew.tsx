import { useQuery } from '@tanstack/react-query'
import { CalendarDays, Sparkles } from 'lucide-react'
import { type KeyboardEvent, useEffect, useRef, useState } from 'react'
import { api, type Release } from '@/api'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  readSeenVersion,
  sameVersion,
  shouldAnnounce,
  splitHeadline,
  writeSeenVersion,
} from '@/lib/releaseNotes'
import { cn } from '@/lib/utils'

/**
 * The running version and the changelog, from the server.
 *
 * The number comes from the server rather than from the bundle: a constant
 * baked into the frontend would be a second copy of the version, and the one
 * thing this screen must never do is disagree with the binary it talks to.
 * Read once and kept; the version of a running process does not change under it.
 */
export function useVersion() {
  return useQuery({
    queryKey: ['system', 'version'],
    queryFn: () => api.version(),
    staleTime: Number.POSITIVE_INFINITY,
    retry: false,
  })
}

/** The version in the sidebar. Opens the notes. */
export function WhatsNew({ className }: { className?: string }) {
  const [open, setOpen] = useState(false)
  const info = useVersion()

  // A version that could not be read is not rendered as "unknown" or as a
  // spinner in the corner of every screen: a permanent error in the chrome
  // teaches people to ignore the chrome.
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
          // A 16px line of text is no tap target on a phone: below md the
          // button grows to 44px and keeps its text centred in it (MOB-01).
          'max-md:flex max-md:min-h-11 max-md:items-center',
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
 * Opens the notes by themselves, once, the first time this browser meets a new
 * version.
 *
 * Mounted once in the shell. The sidebar's version button cannot do this: the
 * navigation is rendered twice (rail and drawer), and two copies of this would
 * open two dialogs.
 *
 * It is marked as seen when it opens, not when it is closed: the claim is
 * "shown once", and a reload with the dialog open must not show it again.
 */
export function WhatsNewOnUpdate() {
  const info = useVersion()
  const [open, setOpen] = useState(false)
  const decided = useRef(false)

  const version = info.data?.version ?? ''
  const releases = info.data?.releases

  useEffect(() => {
    if (decided.current || !info.isSuccess || version === '') {
      return
    }
    decided.current = true
    const seen = readSeenVersion()
    const known = (releases ?? []).map((r) => r.version)
    if (shouldAnnounce(seen, version, known)) {
      setOpen(true)
    }
    writeSeenVersion(version)
  }, [info.isSuccess, version, releases])

  if (!open || releases === undefined) {
    return null
  }
  return <ReleaseNotes open onOpenChange={setOpen} version={version} releases={releases} />
}

/**
 * The screen itself: one tab per release, the running one selected, a header
 * that says what the release is, and each change as a card. Exported so a test
 * can render it without a query client, a session and a server behind it.
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
  // Opens on what the operator is running, not on whatever is first: they
  // differ the moment somebody opens this on a host that is behind.
  const running = releases.find((r) => sameVersion(r.version, version))?.version
  const [selected, setSelected] = useState<string | null>(null)
  const activeVersion = selected ?? running ?? releases[0]?.version ?? null
  const shown = releases.find((r) => r.version === activeVersion)

  const tabRefs = useRef(new Map<string, HTMLButtonElement>())

  // The active tab scrolls into view: with many releases the strip scrolls
  // sideways, and the running version may be far to the right of the edge.
  useEffect(() => {
    if (!open || activeVersion === null) {
      return
    }
    tabRefs.current.get(activeVersion)?.scrollIntoView?.({ inline: 'nearest', block: 'nearest' })
  }, [open, activeVersion])

  function onKeyDown(e: KeyboardEvent<HTMLDivElement>) {
    const index = releases.findIndex((r) => r.version === activeVersion)
    let next = index
    if (e.key === 'ArrowRight') next = Math.min(index + 1, releases.length - 1)
    else if (e.key === 'ArrowLeft') next = Math.max(index - 1, 0)
    else if (e.key === 'Home') next = 0
    else if (e.key === 'End') next = releases.length - 1
    else return
    e.preventDefault()
    const target = releases[next]
    if (target !== undefined) {
      setSelected(target.version)
      tabRefs.current.get(target.version)?.focus()
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {/* flex-col rather than the base grid, and it is the whole reason the
          list scrolls. DialogContent lays its children out in a grid with
          auto-sized rows, and an auto row takes an automatic minimum size from
          its content -- so max-h clipped the panel while the list went on
          rendering underneath it. In a column, min-h-0 on the scroller lets
          that row shrink and the overflow becomes a scrollbar. Only a browser
          shows this: jsdom has no layout. */}
      <DialogContent className="flex max-h-[88vh] flex-col gap-4 sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Sparkles aria-hidden="true" className="size-5 text-primary" />
            What's New
          </DialogTitle>
          <DialogDescription>
            This instance is running <span className="font-medium">{version}</span>.
          </DialogDescription>
        </DialogHeader>

        {releases.length > 0 && (
          <div
            role="tablist"
            aria-label="Releases"
            onKeyDown={onKeyDown}
            // Scrolls sideways instead of wrapping: fourteen tabs on two or
            // three rows would push the content off a phone.
            className="-mx-1 flex gap-1 overflow-x-auto px-1 pb-1 [scrollbar-width:thin]"
          >
            {releases.map((release) => {
              const selectedTab = release.version === activeVersion
              const isRunning = sameVersion(release.version, version)
              return (
                <button
                  key={release.version}
                  ref={(el) => {
                    if (el === null) tabRefs.current.delete(release.version)
                    else tabRefs.current.set(release.version, el)
                  }}
                  type="button"
                  role="tab"
                  id={`release-tab-${release.version}`}
                  aria-selected={selectedTab}
                  aria-controls="release-panel"
                  tabIndex={selectedTab ? 0 : -1}
                  onClick={() => setSelected(release.version)}
                  className={cn(
                    'relative shrink-0 rounded-full border px-3 py-1 text-sm tabular-nums transition-colors',
                    // 44px below md, with the side padding a phone needs.
                    'max-md:min-h-11 max-md:px-4',
                    'focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring',
                    selectedTab
                      ? 'border-primary bg-primary text-primary-foreground shadow-sm'
                      : 'border-border text-muted-foreground hover:bg-muted hover:text-foreground',
                  )}
                >
                  {release.version}
                  {isRunning && (
                    <span
                      aria-hidden="true"
                      className={cn(
                        'absolute -right-0.5 -top-0.5 size-2.5 rounded-full ring-2 ring-background',
                        selectedTab ? 'bg-emerald-400' : 'bg-emerald-500',
                      )}
                    />
                  )}
                </button>
              )
            })}
          </div>
        )}

        <div
          role="tabpanel"
          id="release-panel"
          aria-labelledby={activeVersion === null ? undefined : `release-tab-${activeVersion}`}
          className="min-h-0 flex-1 space-y-4 overflow-y-auto pr-1"
        >
          {shown !== undefined && (
            <>
              <div className="rounded-xl border border-primary/20 bg-gradient-to-br from-primary/15 via-primary/5 to-transparent p-4 sm:p-5">
                <div className="flex flex-wrap items-center gap-2">
                  <h3 className="font-heading text-2xl font-semibold tracking-tight">
                    {shown.version}
                  </h3>
                  {sameVersion(shown.version, version) && (
                    <span className="rounded-full bg-primary px-2 py-0.5 text-xs font-medium text-primary-foreground">
                      running here
                    </span>
                  )}
                </div>
                <div className="mt-1 flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-muted-foreground">
                  {shown.date !== '' && (
                    <span className="inline-flex items-center gap-1.5 tabular-nums">
                      <CalendarDays aria-hidden="true" className="size-3.5" />
                      {shown.date}
                    </span>
                  )}
                  <span>
                    {shown.changes.length}
                    {shown.changes.length === 1 ? ' change' : ' changes'}
                  </span>
                </div>
              </div>

              <ul className="space-y-3">
                {shown.changes.map((change) => {
                  const { headline, body } = splitHeadline(change.text)
                  return (
                    <li
                      key={change.text}
                      className="flex gap-3 rounded-lg border bg-card p-3 text-sm leading-relaxed shadow-xs sm:p-4"
                    >
                      <span
                        aria-hidden="true"
                        className="flex size-9 shrink-0 select-none items-center justify-center rounded-md bg-muted text-lg"
                      >
                        {change.icon}
                      </span>
                      <div className="min-w-0 space-y-1">
                        {headline !== '' && <p className="font-medium">{headline}</p>}
                        <p className={cn(headline !== '' && 'text-muted-foreground')}>{body}</p>
                      </div>
                    </li>
                  )
                })}
              </ul>
            </>
          )}

          {shown === undefined && (
            <p className="text-sm text-muted-foreground">This build carries no release notes.</p>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}
