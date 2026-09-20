import { useQuery } from '@tanstack/react-query'
import { createRoute } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { api, type Capacity, type Wall, type WallTile } from '@/api'
import { rootRoute } from '@/routes/__root'

/**
 * The wall (2026-09-20).
 *
 * A screen in the IT office, read from across the room, by somebody who is not
 * operating anything. It answers one question — is everything all right — and it
 * has to answer it in the second somebody glances up.
 *
 * Every decision here is a consequence of that:
 *
 * # It is not the product's shell
 *
 * No navigation, no sidebar, no header. Nobody is going anywhere from this page,
 * and a menu on a wall is pixels that could have been a tile.
 *
 * # It never scrolls
 *
 * A wall nobody touches cannot show what is below the fold, so the tiles shrink
 * to fit instead. Past the point where they would be unreadable the screen shows
 * the counts and the worst ones by name, which is a true answer at any size —
 * unlike a grid that silently stops at the bottom edge.
 *
 * # It goes visibly stale
 *
 * The answer says when it was true. If the last one is older than a few
 * intervals the whole screen dims and says so, because a wall that cannot go
 * stale lies during exactly the incident it exists for: a daemon that died at two
 * leaves a confident green screen up all night.
 *
 * And a failed refresh keeps the last answer on screen rather than blanking it —
 * with the age visible. The state a moment ago, labelled, beats nothing.
 *
 * # The colours mean five things, not two
 *
 * ok, warn, down, stopped, unknown. The server decides which; see internal/kube/
 * wall.go for why a CronJob between runs is not an outage and why a node nobody
 * is hearing from is never green.
 */

/** How often the screen asks. Ten seconds is what the Kubernetes screens use. */
const REFRESH_MS = 10_000

/**
 * When the screen stops believing itself.
 *
 * Three intervals: one missed refresh is a network blink, three is something
 * wrong. Below this the numbers are treated as current; above it the screen says
 * how old they are and stops looking confident.
 */
const STALE_AFTER_MS = 3 * REFRESH_MS

const TILE_COLOURS: Record<string, string> = {
  ok: 'bg-emerald-600/15 text-emerald-200 ring-emerald-500/40',
  warn: 'bg-amber-500/20 text-amber-100 ring-amber-400/50',
  down: 'bg-red-600/25 text-red-100 ring-red-500/60',
  unknown: 'bg-slate-500/20 text-slate-200 ring-slate-400/40',
  stopped: 'bg-slate-700/40 text-slate-400 ring-slate-600/40',
}

export function WallView() {
  const [now, setNow] = useState(() => Date.now())

  // The kiosk link, read from the page's own address once. It stays in the
  // address because the address IS the bookmark on the screen; from here it
  // travels into a header and nowhere else. It is never put in an API URL: a
  // URL is written to the server's log, the browser's history and whatever
  // proxy sits between, and a credential in all three outlives its screen.
  const [link] = useState(() => {
    try {
      return new URLSearchParams(window.location.search).get('k') ?? ''
    } catch {
      return ''
    }
  })

  // A clock of its own, so the age on screen keeps counting up between
  // refreshes. Without it a screen whose requests had stopped would show an age
  // frozen at the moment of the last success, which is the opposite of the
  // thing this page promises.
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(timer)
  }, [])

  // A kiosk link opens the wall route and nothing else, so a screen showing one
  // cannot list the clusters. The cluster then comes from the address too, and
  // the link somebody copies out of the settings screen already carries it.
  const [pinned] = useState(() => {
    try {
      return new URLSearchParams(window.location.search).get('cluster') ?? ''
    } catch {
      return ''
    }
  })

  const clusters = useQuery({
    queryKey: ['clusters'],
    queryFn: api.clusters.list,
    // Not asked for at all when the address names one: the request would be
    // refused, and a refusal nobody reads is a request nobody should make.
    enabled: pinned === '',
  })
  const cluster = pinned === '' ? (clusters.data?.[0]?.id ?? '') : pinned

  const wall = useQuery({
    queryKey: ['wall', cluster, link],
    queryFn: () => api.kubernetes.wall(cluster, undefined, link),
    enabled: cluster !== '',
    refetchInterval: REFRESH_MS,
    // The last answer stays on screen while a new one is fetched or fails. A
    // wall that blanked on every hiccup would spend its life blank.
    placeholderData: (previous) => previous,
    retry: false,
  })

  const data = wall.data
  const answeredAt = data?.generated_at === '' ? null : new Date(data?.generated_at ?? '')
  const ageMs =
    answeredAt === null || Number.isNaN(answeredAt.getTime()) ? null : now - answeredAt.getTime()
  const stale = ageMs === null || ageMs > STALE_AFTER_MS

  return (
    <div className="fixed inset-0 overflow-hidden bg-zinc-950 p-[2vmin] text-zinc-100">
      {data === undefined ? (
        <p className="grid h-full place-items-center text-[4vmin] text-zinc-500">
          {wall.error
            ? // Never a green screen: an unanswered question is not an answer.
              // A revoked or mistyped link lands here too, and saying which is
              // worth the sentence: one is a cluster problem and the other is a
              // link somebody has to replace.
              link !== '' && isRefusal(wall.error)
              ? 'This screen’s link is no longer valid. Make a new one in Settings.'
              : 'The cluster could not be asked.'
            : clusters.data !== undefined && cluster === ''
              ? 'No cluster has been imported yet.'
              : 'Asking the cluster…'}
        </p>
      ) : (
        <div className={`flex h-full flex-col gap-[1.5vmin] ${stale ? 'opacity-40' : ''}`}>
          <Header wall={data} stale={stale} ageMs={ageMs} />
          <Tiles title="Nodes" tiles={data.nodes} />
          <Tiles title="Workloads" tiles={data.workloads} grow />
          <Footer wall={data} />
        </div>
      )}
    </div>
  )
}

function Header({ wall, stale, ageMs }: { wall: Wall; stale: boolean; ageMs: number | null }) {
  const down = (wall.summary.down ?? 0) + (wall.summary.unknown ?? 0)
  const warn = wall.summary.warn ?? 0

  return (
    <div className="flex items-baseline justify-between gap-[2vmin]">
      <p
        className={`font-semibold text-[5vmin] leading-none ${
          down > 0 ? 'text-red-400' : warn > 0 ? 'text-amber-300' : 'text-emerald-400'
        }`}
      >
        {down > 0
          ? `${down} not running`
          : warn > 0
            ? `${warn} need attention`
            : 'Everything is running'}
      </p>
      {/* The age, always — not only when it is bad. A number that appears only
          during trouble is a number nobody has learned to read by then. */}
      <p className={`text-[2.2vmin] ${stale ? 'text-red-300' : 'text-zinc-500'}`}>
        {ageMs === null
          ? 'never answered'
          : stale
            ? `last answer ${readableAge(ageMs)} ago — this may be out of date`
            : `as of ${readableAge(ageMs)} ago`}
      </p>
    </div>
  )
}

function Tiles({ title, tiles, grow }: { title: string; tiles: WallTile[]; grow?: boolean }) {
  if (tiles.length === 0) return null

  // The tile size follows the count, so the grid fills the screen instead of
  // overflowing it. Past about sixty the names stop being readable from across
  // a room, and the screen falls back to counts plus the worst ones by name --
  // a true answer at any size, unlike a grid that stops at the bottom edge.
  const tooMany = tiles.length > 60
  const shown = tooMany ? tiles.filter((t) => t.state !== 'ok' && t.state !== 'stopped') : tiles
  const size =
    shown.length > 24 ? 'text-[1.6vmin]' : shown.length > 12 ? 'text-[2vmin]' : 'text-[2.6vmin]'

  return (
    <div className={`flex min-h-0 flex-col gap-[0.8vmin] ${grow ? 'flex-1' : ''}`}>
      <p className="text-[1.8vmin] text-zinc-500 uppercase tracking-widest">
        {title}
        {tooMany && ` — showing ${shown.length} of ${tiles.length}`}
      </p>
      <div
        className="grid min-h-0 flex-1 content-start gap-[0.8vmin]"
        style={{
          gridTemplateColumns: `repeat(auto-fill, minmax(${shown.length > 24 ? 16 : 24}vmin, 1fr))`,
        }}
      >
        {shown.map((tile) => (
          <div
            key={`${tile.kind}/${tile.namespace}/${tile.name}`}
            // Marked as a row so the layout audit can count tiles. Without it
            // the audit reported "0 items" for a full wall, which is exactly
            // the ambiguity that count exists to remove: rendered nothing, or
            // rendered something the selector cannot see?
            data-row=""
            data-state={tile.state}
            className={`overflow-hidden rounded-[1vmin] px-[1.2vmin] py-[1vmin] ring-1 ${size} ${
              TILE_COLOURS[tile.state] ?? TILE_COLOURS.unknown
            }`}
          >
            <p className="truncate font-medium">{tile.name}</p>
            <p className="truncate opacity-70">{tile.detail}</p>
          </div>
        ))}
      </div>
    </div>
  )
}

function Footer({ wall }: { wall: Wall }) {
  return (
    <div className="flex items-end justify-between gap-[2vmin]">
      <div className="flex gap-[2vmin]">
        <Meter label="CPU" capacity={wall.cpu} />
        <Meter label="Memory" capacity={wall.memory} />
        <Meter label="Pods" capacity={wall.pods} />
      </div>
      {wall.warnings.length > 0 && (
        <div className="min-w-0 flex-1 text-right">
          {wall.warnings.slice(0, 3).map((warning) => (
            <p
              key={`${warning.object}/${warning.reason}/${warning.age}`}
              className="truncate text-[1.8vmin] text-amber-200/80"
            >
              <span className="font-medium">{warning.reason}</span> {warning.object}
              {/* Seen 340 times is a different situation from seen once, and
                  from four metres that count is the whole message. */}
              {warning.count > 1 ? ` ×${warning.count}` : ''}
            </p>
          ))}
        </div>
      )}
    </div>
  )
}

function Meter({ label, capacity }: { label: string; capacity: Capacity }) {
  return (
    <div className="w-[18vmin]">
      <div className="flex items-baseline justify-between text-[1.8vmin]">
        <span className="text-zinc-500">{label}</span>
        <span className="tabular-nums text-zinc-300">
          {capacity.percent < 0 ? '—' : `${capacity.percent}%`}
        </span>
      </div>
      <div className="mt-[0.4vmin] h-[1vmin] overflow-hidden rounded-full bg-zinc-800">
        {capacity.percent >= 0 && (
          <div
            className={
              capacity.percent >= 90
                ? 'h-full bg-red-500'
                : capacity.percent >= 75
                  ? 'h-full bg-amber-400'
                  : 'h-full bg-emerald-500'
            }
            style={{ width: `${Math.min(100, Math.max(0, capacity.percent))}%` }}
          />
        )}
      </div>
    </div>
  )
}

/** readableAge is what somebody four metres away can read at a glance. */
function readableAge(ms: number): string {
  const seconds = Math.max(0, Math.round(ms / 1000))
  if (seconds < 90) return `${seconds}s`
  const minutes = Math.round(seconds / 60)
  if (minutes < 90) return `${minutes} min`
  return `${Math.round(minutes / 60)} h`
}

/**
 * Hung off the ROOT and not the authenticated layout, deliberately.
 *
 * That layout's component is the AppShell: sidebar, header, chain banner. All of
 * it is right for somebody working and all of it is wrong on a wall, where
 * nobody is going anywhere and every pixel of navigation is a pixel that could
 * have been a tile.
 *
 * What it gives up is the session gate that layout carries. For now that means
 * the screen needs somebody to sign in on it once — and a session expires, so
 * this is not yet a screen that can be left up for weeks. The kiosk link is the
 * other half and is the next thing built; until it exists, this page says what
 * it is rather than pretending.
 */
export const wallRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/wall',
  component: WallView,
})

/**
 * isRefusal tells a rejected credential from an unreachable cluster.
 *
 * On a wall the difference is the whole of what somebody walking past can do
 * about it: a revoked link is replaced in Settings, an unreachable cluster is
 * not.
 */
function isRefusal(error: unknown): boolean {
  const status = (error as { problem?: { status?: number } })?.problem?.status
  return status === 401 || status === 403
}
