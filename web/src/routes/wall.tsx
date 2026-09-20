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
    /* Fixed and never scrolling on the screen it is for: nobody touches a wall,
        so anything below the fold is gone rather than one swipe away. On a phone
        -- where the operator looked at it first -- that same rule silently CUTS
        it, which is the claim this product refuses everywhere else. So below md
        it scrolls, and the television keeps its one screenful. */
    <div className="fixed inset-0 overflow-auto bg-zinc-950 p-[2vmin] text-zinc-100 md:overflow-hidden">
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
        <div className={`flex flex-col gap-[1.5vmin] md:h-full ${stale ? 'opacity-40' : ''}`}>
          <Header wall={data} stale={stale} ageMs={ageMs} />
          <Named title="Nodes" tiles={data.nodes} />
          {/* Named and first, because these are what somebody is looking for.
              A healthy cluster has none and the section simply is not there. */}
          <Named title="Needs attention" tiles={data.workloads.filter(needsAttention)} />
          <Field tiles={data.workloads.filter((tile) => !needsAttention(tile))} />
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

/**
 * NODES and everything that needs attention, named and large.
 *
 * Always ALL of them. The first version of this hid healthy tiles past sixty and
 * drew only the ones that were not fine -- which on a cluster where everything
 * works showed "SHOWING 0 OF 132" and a screen of black. That is the claim this
 * product refuses everywhere else (INV-08): an empty screen reads as "nothing is
 * running", and here it said so about a cluster running a hundred and thirty-two
 * things.
 */
function Named({ title, tiles }: { title: string; tiles: WallTile[] }) {
  if (tiles.length === 0) return null

  // Size follows the count so a long list still fits, and the floor is the point
  // at which a name is still readable from across a room. Below that the tile
  // belongs in the field instead.
  const size =
    tiles.length > 12 ? 'text-[1.8vmin]' : tiles.length > 6 ? 'text-[2.4vmin]' : 'text-[3.2vmin]'

  return (
    <div className="flex flex-col gap-[0.8vmin]">
      <p className="text-[1.6vmin] text-zinc-500 uppercase tracking-widest">{title}</p>
      <div
        className="grid content-start gap-[0.8vmin]"
        style={{
          // auto-FIT, not auto-fill: three nodes on a television should be three
          // WIDE tiles whose names fit, not three narrow ones beside four empty
          // tracks. The first photograph of this screen showed
          // "srv-rsp-prod02…" truncated with two thirds of the row unused.
          gridTemplateColumns: `repeat(auto-fit, minmax(${tiles.length > 12 ? 24 : 34}vmin, 1fr))`,
        }}
      >
        {tiles.map((tile) => (
          <div
            key={`${tile.kind}/${tile.namespace}/${tile.name}`}
            // Marked as a row so the layout audit can count tiles: without it
            // the audit reported "0 items" for a full wall, which is exactly the
            // ambiguity that count exists to remove.
            data-row=""
            data-state={tile.state}
            className={`overflow-hidden rounded-[1vmin] px-[1.2vmin] py-[1vmin] ring-1 ${size} ${
              TILE_COLOURS[tile.state] ?? TILE_COLOURS.unknown
            }`}
          >
            <p className="truncate font-medium">{tile.name}</p>
            <p className="truncate opacity-70">
              {tile.namespace === '' ? tile.detail : `${tile.namespace} · ${tile.detail}`}
            </p>
          </div>
        ))}
      </div>
    </div>
  )
}

/**
 * Everything that is fine, as a field of colour grouped by namespace.
 *
 * # Why a field and not a list
 *
 * A hundred and thirty-two names cannot be read from four metres and nobody is
 * trying to. What a wall is read for is the SHAPE: a field of green with one red
 * square in it is understood before anybody has focused on anything. So the
 * healthy ones keep their colour and lose their names, and the ones that are not
 * healthy are named above in full.
 *
 * # Why grouped by namespace
 *
 * Without it this is a hundred and thirty-two anonymous squares. With it the
 * field has landmarks -- somebody who knows the cluster sees "db" and "monitoring"
 * as places, and a gap or a wrong colour has an address.
 */
function Field({ tiles }: { tiles: WallTile[] }) {
  if (tiles.length === 0) return null

  const groups = new Map<string, WallTile[]>()
  for (const tile of tiles) {
    const key = tile.namespace === '' ? '—' : tile.namespace
    groups.set(key, [...(groups.get(key) ?? []), tile])
  }
  // By name, so a namespace does not move between refreshes. Something jumping
  // about on a wall is read as something changing.
  const ordered = [...groups.entries()].sort((a, b) => a[0].localeCompare(b[0]))
  const running = tiles.filter((tile) => tile.state === 'ok').length
  const stopped = tiles.length - running

  // flex-1 only where the page cannot scroll. On a television the field takes
  // the slack and the footer sits at the bottom; on a phone the same rule opens
  // a black gap between the tiles and the footer, and scrolling down lands in
  // it -- which is what the operator photographed.
  return (
    <div className="flex min-h-0 flex-col gap-[0.8vmin] md:flex-1">
      <div className="flex flex-wrap items-baseline gap-x-[2vmin] gap-y-[0.4vmin]">
        <p className="text-[1.6vmin] text-zinc-500 uppercase tracking-widest">
          {tiles.length} workloads — one square each
        </p>
        {/* Spelled out, because the operator asked twice what the squares were
            and then asked whether they were pods. "Workload" is this product's
            word and not theirs, and a wall that needs explaining has not been
            explained until the explanation is ON it. */}
        <span className="text-[1.5vmin] text-zinc-600">
          a deployment, statefulset, daemonset, job or cronjob — not a single pod
        </span>
        {/* A legend, once, in small type. The operator asked what the green
            squares meant, and having to ask is the defect: a wall is read by
            people who were never told anything about it, and an answer given in
            a chat is an answer nobody walking past ever gets. Only the colours
            actually on the screen are listed -- a key to something that is not
            there is one more thing to read past. */}
        <span className="flex items-center gap-[1.2vmin] text-[1.5vmin] text-zinc-600">
          {running > 0 && (
            <span className="flex items-center gap-[0.5vmin]">
              <span
                className={`h-[1.4vmin] w-[1.4vmin] rounded-[0.3vmin] ring-1 ${TILE_COLOURS.ok}`}
              />
              running {running}
            </span>
          )}
          {stopped > 0 && (
            <span className="flex items-center gap-[0.5vmin]">
              <span
                className={`h-[1.4vmin] w-[1.4vmin] rounded-[0.3vmin] ring-1 ${TILE_COLOURS.stopped}`}
              />
              stopped on purpose {stopped}
            </span>
          )}
        </span>
      </div>
      <div className="flex min-h-0 flex-1 flex-wrap content-start gap-x-[3vmin] gap-y-[1.6vmin]">
        {ordered.map(([namespace, group]) => (
          <div key={namespace} className="flex flex-col gap-[0.4vmin]">
            <p className="text-[1.7vmin] text-zinc-500">
              {namespace} <span className="tabular-nums">{group.length}</span>
            </p>
            <div className="flex max-w-[46vmin] flex-wrap gap-[0.6vmin]">
              {group.map((tile) => (
                <div
                  key={`${tile.kind}/${tile.namespace}/${tile.name}`}
                  data-row=""
                  data-state={tile.state}
                  // The name is a title rather than text: it is there for
                  // anybody who walks up to the screen, and takes no room from
                  // the four-metre reading it would otherwise crowd out.
                  title={`${tile.name} — ${tile.detail}`}
                  className={`h-[3.4vmin] w-[3.4vmin] rounded-[0.5vmin] ring-1 ${
                    TILE_COLOURS[tile.state] ?? TILE_COLOURS.unknown
                  }`}
                />
              ))}
            </div>
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
              {/* And WHY. "Failed Pod/dupl-test" names a pod and says nothing
                  at all about what happened to it; the cluster's own sentence
                  is the only part of a warning with any content, and it was
                  fetched, carried through the API and then not drawn. */}
              {warning.message === '' ? '' : ` — ${warning.message}`}
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

/**
 * needsAttention splits the two zones.
 *
 * `stopped` is NOT here: it is a decision somebody made, and a wall that put
 * every deliberately stopped workload in the attention list would be a wall
 * asking to be ignored. It keeps its own colour in the field instead.
 */
function needsAttention(tile: WallTile): boolean {
  return tile.state === 'down' || tile.state === 'unknown' || tile.state === 'warn'
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
