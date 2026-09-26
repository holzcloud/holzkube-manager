import { useQuery } from '@tanstack/react-query'
import { createRoute } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import {
  api,
  type Capacity,
  type Wall,
  type WallNamespace,
  type WallTile,
  type WallWarning,
} from '@/api'
import { LiveChart } from '@/components/charts/LiveChart'
import { Sparkline } from '@/components/charts/Sparkline'
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
 * to fit instead. Nothing is ever cut off the bottom: the right-hand column
 * ROLLS UP rather than truncating, so a cluster twice this size still fits and
 * still tells the truth about its own size.
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
 * # Two halves, because there are two readers
 *
 * The same screen is read from ten metres ("is anything wrong") and from one
 * ("what, and why"). One layout cannot be optimised for both, so it is split:
 *
 *   LEFT   the answer in words at nine per cent of the screen's height, the
 *          nodes, and every workload that is not running — named, large, legible
 *          from across the room.
 *   RIGHT  the detail somebody walks up to read: how much room is left, one tile
 *          per namespace, and the cluster's latest warnings in its own words.
 *
 * This is the shape Grafana's own Kubernetes dashboards and the kube-mixin
 * boards use — a headline that needs no explanation, then labelled sections —
 * and the namespace tiles are its polystat: a group collapsed to one block
 * carrying the worst state inside it, naming the offender rather than making
 * somebody go and look.
 *
 * # The colours are solid, and mean five things
 *
 * ok, warn, down, stopped, unknown. The first version tinted them at fifteen per
 * cent opacity, which is legible on a laptop at arm's length and gone at four
 * metres on a television with the lights on. They are flat saturated fills now,
 * which is what every wall board that works actually does.
 *
 * The server decides which; see internal/kube/wall.go for why a CronJob between
 * runs is not an outage and why a node nobody is hearing from is never green.
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

/** How many warnings the right-hand column has room for at a readable size. */
const WARNINGS_SHOWN = 4

/**
 * Flat, saturated fills — not tints.
 *
 * A wall is read at four metres, off-axis, in a lit room, on a television whose
 * contrast nobody has calibrated. `bg-emerald-600/15` survives none of that: it
 * is a hint of colour on black, and the first photograph of this screen from
 * across the room showed a field of dark grey squares. Every wall board that
 * works — Grafana's Kubernetes dashboards, the kube-mixin boards, polystat —
 * uses solid colour for exactly this reason.
 *
 * `stopped` is the one that recedes on purpose: something switched off is not
 * news, and drawing it as loudly as a failure is how a wall teaches people to
 * stop looking at it.
 */
const TILE_COLOURS: Record<string, string> = {
  ok: 'bg-green-700 text-white ring-green-600',
  warn: 'bg-amber-700 text-white ring-amber-600',
  down: 'bg-red-700 text-white ring-red-500',
  unknown: 'bg-zinc-600 text-white ring-zinc-500',
  stopped: 'bg-zinc-900 text-zinc-500 ring-zinc-800',
}

/**
 * What each colour means, in words, on the screen.
 *
 * The operator asked what the green squares meant and then asked whether they
 * were pods. Having to ask is the defect: a wall is read by people nobody told
 * anything, and an answer given once in a conversation is an answer nobody
 * walking past ever gets.
 */
const STATE_WORDS: Record<string, string> = {
  ok: 'running',
  warn: 'partly running',
  down: 'not running',
  unknown: 'not reporting',
  stopped: 'switched off on purpose',
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
        it scrolls and stacks, and the television keeps its one screenful. */
    <div className="fixed inset-0 overflow-auto bg-zinc-950 p-4 text-zinc-100 md:p-[2vmin] md:overflow-hidden">
      {data === undefined ? (
        <p className="grid h-full place-items-center p-6 text-center text-lg text-zinc-500 md:p-0 md:text-[4vmin]">
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
        /* The split. Slightly wider on the left, because what is on the left is
            what gets read from ten metres and the names there have to fit. */
        <div
          className={`grid min-h-0 gap-6 md:h-full md:grid-cols-[minmax(0,1.15fr)_minmax(0,1fr)] md:gap-[2.5vmin] ${
            stale ? 'opacity-40' : ''
          }`}
        >
          <LeftHalf wall={data} stale={stale} ageMs={ageMs} />
          <RightHalf wall={data} now={now} />
        </div>
      )}
    </div>
  )
}

/**
 * The ten-metre half: the answer, the nodes, and whatever is wrong.
 *
 * Nothing here is ever collapsed or counted-instead-of-drawn. The first version
 * of this page hid healthy tiles past sixty and showed "SHOWING 0 OF 132" and a
 * screen of black — an empty screen claiming nothing was running, about a
 * cluster running everything (INV-08). The roll-up lives on the right instead,
 * where it is labelled with its own totals.
 */
function LeftHalf({ wall, stale, ageMs }: { wall: Wall; stale: boolean; ageMs: number | null }) {
  const attention = wall.workloads.filter(needsAttention)

  return (
    <div className="flex min-h-0 min-w-0 flex-col gap-4 md:gap-[1.8vmin]">
      <Headline wall={wall} stale={stale} ageMs={ageMs} />
      {/* The nodes are always here, all of them and named, whether or not
          anything is wrong with them. There are three of them on the operator's
          cluster and each one is a machine they can walk over to; a wall that
          only mentioned a node once it had already failed would be a wall that
          never showed the thing it is most often consulted about. */}
      <Named title="Nodes" tiles={wall.nodes} trends={wall.trends.nodes} />
      {/* And everything that is not running, named, large. Absent — not empty,
          not a zero — when there is nothing to name.
          It GROWS into whatever the column has left: a wall wants the biggest
          tiles that still fit, not the smallest that technically do. The first
          photograph of this layout had two outage tiles at the top and 55 per
          cent of the screen black underneath. */}
      {attention.length > 0 && <Named title="Needs attention" tiles={attention} grow />}
    </div>
  )
}

/** The one-metre half: how much room is left, where things are, and why. */
function RightHalf({ wall, now }: { wall: Wall; now: number }) {
  return (
    <div className="flex min-h-0 min-w-0 flex-col gap-5 md:gap-[2vmin]">
      <ClusterTrend trends={wall.trends.cluster} />
      <Section title="Room left">
        <div className="flex flex-col gap-3 md:gap-[1.2vmin]">
          <Meter label="CPU" capacity={wall.cpu} />
          <Meter label="Memory" capacity={wall.memory} />
          <Meter label="Pods" capacity={wall.pods} />
        </div>
      </Section>
      <Namespaces tiles={wall.namespaces} workloads={wall.workloads.length} />
      <Warnings warnings={wall.warnings} now={now} />
    </div>
  )
}

/**
 * The cluster's load over the last day (2026-09-26, the operator's layout A).
 *
 * The mean over the nodes that answered, per minute, from the daemon's record
 * -- so the curve is there the moment the screen is switched on, not a line
 * that starts growing when it is.
 */
function ClusterTrend({ trends }: { trends: Record<string, [number, number][]> }) {
  const cpu = (trends.cpu ?? []).map(([t, v]) => ({ t, v }))
  const memory = (trends.memory ?? []).map(([t, v]) => ({ t, v }))
  if (cpu.length === 0 && memory.length === 0) return null
  return (
    <Section title="Cluster load" aside="last 24 hours">
      <LiveChart
        title="Cluster load"
        series={[
          { key: 'cpu', label: 'CPU', slot: 1, points: cpu },
          { key: 'memory', label: 'Memory', slot: 2, points: memory },
        ]}
        format={(v) => `${Math.round(v)}%`}
        yMax={100}
        windowMs={24 * 60 * 60_000}
        height={180}
      />
    </Section>
  )
}

/** One node's last hour, the size of a line of text, under its name. */
function NodeTrend({ points, name }: { points: [number, number][]; name: string }) {
  const last = points[points.length - 1]?.[1]
  return (
    <div className="mt-1 flex items-center gap-2">
      <Sparkline
        label={`${name} processor load, last hour`}
        points={points.map(([t, v]) => ({ t, v }))}
        max={100}
        width={180}
        height={28}
        color={
          last === undefined || last < 75
            ? 'var(--viz-ok)'
            : last < 90
              ? 'var(--viz-warn)'
              : 'var(--viz-danger)'
        }
      />
      {last !== undefined && <span className="tabular-nums opacity-80">{Math.round(last)}%</span>}
    </div>
  )
}

function Headline({ wall, stale, ageMs }: { wall: Wall; stale: boolean; ageMs: number | null }) {
  const down = (wall.summary.down ?? 0) + (wall.summary.unknown ?? 0)
  const warn = wall.summary.warn ?? 0

  return (
    <div>
      {/* Nine per cent of the screen's height. On a television across an office
          that is the difference between a wall somebody reads while walking past
          and a wall somebody has to stop and squint at. */}
      <p
        className={`font-semibold text-[2.6rem] leading-[0.95] md:text-[8.5vmin] tracking-tight ${
          down > 0 ? 'text-red-400' : warn > 0 ? 'text-amber-300' : 'text-emerald-400'
        }`}
      >
        {down > 0
          ? `${down} not running`
          : warn > 0
            ? `${warn} need attention`
            : 'Everything is running'}
      </p>
      {/* The age, always -- not only when it is bad. A number that appears only
          during trouble is a number nobody has learned to read by then. And the
          size of the cluster beside it, so the headline has a denominator:
          "everything is running" says much more when everything is 132 things. */}
      <p
        className={`mt-1 text-xs md:mt-[0.8vmin] md:text-[2.1vmin] ${stale ? 'text-red-300' : 'text-zinc-500'}`}
      >
        {ageMs === null
          ? 'never answered'
          : stale
            ? `last answer ${readableAge(ageMs)} ago — this may be out of date`
            : `as of ${readableAge(ageMs)} ago`}
        {' · '}
        {plural(wall.workloads.length, 'workload')}, {plural(wall.nodes.length, 'node')}
      </p>
    </div>
  )
}

/** Section is a labelled block. Every section on this screen says what it is. */
function Section({
  title,
  aside,
  children,
  grow,
}: {
  title: string
  aside?: string
  children: React.ReactNode
  grow?: boolean
}) {
  return (
    <div className={`flex min-h-0 flex-col gap-2 md:gap-[0.9vmin] ${grow ? 'md:flex-1' : ''}`}>
      <div className="flex flex-wrap items-baseline gap-x-[1.5vmin]">
        <p className="text-[0.7rem] text-zinc-500 uppercase tracking-[0.2em] md:text-[1.5vmin]">
          {title}
        </p>
        {aside !== undefined && (
          <span className="text-[0.7rem] text-zinc-600 md:text-[1.4vmin]">{aside}</span>
        )}
      </div>
      {children}
    </div>
  )
}

/**
 * Nodes and everything that needs attention: named, large, all of them.
 */
function Named({
  title,
  tiles,
  grow,
  trends,
}: {
  title: string
  tiles: WallTile[]
  grow?: boolean
  /** A node's last hour of processor load, drawn under its name. */
  trends?: Record<string, [number, number][]>
}) {
  if (tiles.length === 0) return null

  // Size follows the count AND the longest name, and the floor is the point at
  // which a name is still readable from across a room.
  //
  // The count alone is not enough: three nodes got the largest size and then
  // "srv-rsp-prod02.holzcloud.ch" broke mid-word across three lines -- "srv-rsp-
  // prod02.holzcloud.c / h". A hostname is one word, so wrapping cannot help it;
  // only the type size can. Third time this screen has mangled a node's name
  // (ledger 170, then again on first sight of this layout).
  const longest = Math.max(...tiles.map((tile) => tile.name.length))
  const size =
    tiles.length > 12 || longest > 34
      ? 'text-sm md:text-[1.7vmin]'
      : tiles.length > 6 || longest > 22
        ? 'text-base md:text-[2.2vmin]'
        : 'text-lg md:text-[2.9vmin]'

  return (
    <Section title={title} grow={grow}>
      <div
        // auto-FIT, not auto-fill: three nodes on a television should be three
        // WIDE tiles whose names fit, not three narrow ones beside four empty
        // tracks. The first photograph of this screen showed "srv-rsp-prod02…"
        // truncated with two thirds of the row unused.
        className={`grid gap-[0.9vmin] ${
          grow ? 'min-h-0 md:h-full md:content-stretch' : 'content-start'
        } ${
          // The FLOOR follows the longest name too, not only the count. On a
          // 390px phone two 9rem tracks leave 147px of text, and
          // "srv-rsp-prod02.holzcloud.ch" does not fit in that at any size a
          // wall should use -- so it broke mid-word into "holzcloud.c / h".
          // A long name takes the whole width there and reads in one line.
          longest > 22
            ? 'grid-cols-[repeat(auto-fit,minmax(16rem,1fr))]'
            : 'grid-cols-[repeat(auto-fit,minmax(9rem,1fr))]'
        } ${
          tiles.length > 12
            ? 'md:grid-cols-[repeat(auto-fit,minmax(20vmin,1fr))]'
            : 'md:grid-cols-[repeat(auto-fit,minmax(28vmin,1fr))]'
        }`}
        style={{
          // Rows share the height evenly when the section is growing, so two
          // outages are two large blocks rather than two small ones and a void.
          gridAutoRows: grow ? 'minmax(0, 1fr)' : undefined,
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
            className={`min-w-0 overflow-hidden rounded-[1vmin] px-3 py-2 ring-1 md:px-[1.3vmin] md:py-[1.1vmin] ${size} ${
              grow ? 'flex flex-col justify-center' : ''
            } ${TILE_COLOURS[tile.state] ?? TILE_COLOURS.unknown}`}
          >
            {/* Wrapped, not truncated: "srv-rsp-prod02.ho…" is not an address
                anybody can act on, and this is the second time this screen has
                cut a node's name in half (ledger 170). */}
            <p className="break-words font-semibold leading-tight">{tile.name}</p>
            <p className="truncate opacity-80">
              {tile.namespace === '' ? tile.detail : `${tile.namespace} · ${tile.detail}`}
            </p>
            {trends?.[tile.name] !== undefined && (
              <NodeTrend points={trends[tile.name] ?? []} name={tile.name} />
            )}
          </div>
        ))}
      </div>
    </Section>
  )
}

/**
 * One tile per namespace, coloured by the worst thing in it.
 *
 * # Why not one square per workload
 *
 * That is what this page did first, and the operator asked twice what the green
 * squares were and then asked whether they were pods. A hundred and thirty-two
 * anonymous squares are not read as a hundred and thirty-two things; they are
 * read as texture. Eight tiles with names and counts are read as eight places.
 *
 * Nothing is hidden by the roll-up: every workload is inside exactly one tile,
 * each tile carries its own count, the total is printed above, and anything that
 * is not running is ALSO named in full on the left. The aggregate is a second
 * view of the same set, never a substitute for it.
 *
 * # Why the tile names the offender
 *
 * A red block with no name sends somebody to go and look, which is the whole
 * thing this screen exists to save. So a tile that is not green says which
 * workload decided that, in the cluster's own words — "postgres · 2 of 3 ready".
 */
function Namespaces({ tiles, workloads }: { tiles: WallNamespace[]; workloads: number }) {
  if (tiles.length === 0) return null

  // Only the colours actually on the screen are keyed. A key to something that
  // is not there is one more thing to read past.
  const present: string[] = []
  for (const tile of tiles) {
    if (!present.includes(tile.state)) present.push(tile.state)
  }
  if (tiles.some((tile) => tile.stopped > 0) && !present.includes('stopped')) {
    present.push('stopped')
  }
  present.sort((a, b) => Object.keys(STATE_WORDS).indexOf(a) - Object.keys(STATE_WORDS).indexOf(b))

  return (
    <Section
      title={`${plural(tiles.length, 'namespace')} — ${plural(workloads, 'workload')}`}
      // Spelled out, because the operator asked twice what the squares were and
      // then asked whether they were pods. "Workload" is this product's word and
      // not theirs, and a wall that needs explaining has not been explained
      // until the explanation is ON it.
      aside="each tile takes the colour of the worst workload in it — a deployment, statefulset, daemonset, job or cronjob, not a single pod"
      grow
    >
      {/* Stretched, like the left half: the first photograph of this column had
          its eight tiles in a band at the top and a third of the screen black
          underneath. A wall wants the biggest tiles that still fit. */}
      <div
        className="grid min-h-0 grid-cols-[repeat(auto-fit,minmax(6.5rem,1fr))] gap-[0.8vmin] md:flex-1 md:grid-cols-[repeat(auto-fit,minmax(16vmin,1fr))] md:content-stretch"
        style={{ gridAutoRows: 'minmax(0, 1fr)' }}
      >
        {tiles.map((tile) => (
          <div
            key={tile.name}
            data-row=""
            data-state={tile.state}
            title={tile.worst === '' ? tile.name : `${tile.name} — ${tile.worst}`}
            className={`min-w-0 overflow-hidden rounded-[0.8vmin] px-2.5 py-2 ring-1 md:px-[1vmin] md:py-[0.8vmin] ${
              TILE_COLOURS[tile.state] ?? TILE_COLOURS.unknown
            }`}
          >
            <p className="flex items-baseline justify-between gap-[0.6vmin] text-sm md:text-[1.8vmin]">
              <span className="truncate font-semibold">{tile.name}</span>
              <span className="tabular-nums opacity-80">{tile.total}</span>
            </p>
            {/* Only when there is something to say. A tile that always carries a
                sentence is a tile whose sentence nobody reads. */}
            {tile.worst !== '' && (
              <p className="line-clamp-2 break-words text-[0.7rem] leading-tight opacity-90 md:text-[1.4vmin]">
                {tile.worst}
              </p>
            )}
            {tile.worst === '' && tile.stopped > 0 && (
              <p className="truncate text-[0.7rem] opacity-70 md:text-[1.4vmin]">
                {tile.stopped} switched off
              </p>
            )}
          </div>
        ))}
      </div>
      <div className="flex flex-wrap items-center gap-x-[1.6vmin] gap-y-[0.4vmin]">
        {present.map((state) => (
          <span
            key={state}
            className="flex items-center gap-[0.6vmin] text-[0.7rem] text-zinc-500 md:text-[1.4vmin]"
          >
            <span
              className={`h-3 w-3 rounded-[0.3vmin] ring-1 md:h-[1.3vmin] md:w-[1.3vmin] ${
                TILE_COLOURS[state] ?? TILE_COLOURS.unknown
              }`}
            />
            {STATE_WORDS[state] ?? state}
          </span>
        ))}
      </div>
    </Section>
  )
}

/**
 * The cluster's latest warnings, in the cluster's own words.
 *
 * "Failed Pod/dupl-test" names a pod and says nothing at all about what happened
 * to it — the operator asked what it meant, which is the right question to ask of
 * a line with no content in it. The message is the only part of a warning that
 * carries any, so it gets a line of its own here rather than being truncated
 * into the end of another one.
 */
function Warnings({ warnings, now }: { warnings: WallWarning[]; now: number }) {
  if (warnings.length === 0) return null

  return (
    <Section title="Latest warnings">
      <div className="flex flex-col gap-3 md:gap-[0.8vmin]">
        {warnings.slice(0, WARNINGS_SHOWN).map((warning) => (
          <div
            key={`${warning.object}/${warning.reason}/${warning.last_seen}`}
            data-row=""
            className="min-w-0 overflow-hidden border-amber-600 border-l-4 pl-2 md:border-l-[0.4vmin] md:pl-[1vmin]"
          >
            <p className="flex items-baseline gap-[0.8vmin] text-sm md:text-[1.7vmin]">
              <span className="font-semibold text-amber-300">{warning.reason}</span>
              <span className="min-w-0 truncate text-zinc-400">{warning.object}</span>
              {/* Seen 340 times is a different situation from seen once, and
                  from four metres that count is the whole message. */}
              {warning.count > 1 && (
                <span className="tabular-nums text-zinc-500">×{warning.count}</span>
              )}
              {/* An AGE, not the instant the server sent: the first photograph
                  of this column read "2026-09-20T09:58:00Z" across a
                  television, which is four metres of nothing. */}
              {warning.last_seen !== '' && (
                <span className="ml-auto whitespace-nowrap text-zinc-600">
                  {sinceText(warning.last_seen, now)}
                </span>
              )}
            </p>
            {warning.message !== '' && (
              <p className="line-clamp-2 text-xs text-zinc-400 md:text-[1.5vmin]">
                {warning.message}
              </p>
            )}
          </div>
        ))}
      </div>
    </Section>
  )
}

/** Meter is a capacity bar with the number beside it, not inside it. */
function Meter({ label, capacity }: { label: string; capacity: Capacity }) {
  return (
    <div>
      <div className="flex items-baseline justify-between gap-[1vmin] text-sm md:text-[1.8vmin]">
        <span className="text-zinc-400">{label}</span>
        <span className="truncate text-xs text-zinc-600 md:text-[1.5vmin]">
          {capacity.requested === '' || capacity.allocatable === ''
            ? ''
            : `${capacity.requested} of ${capacity.allocatable}`}
        </span>
        <span className="ml-auto tabular-nums font-semibold text-zinc-200">
          {capacity.percent < 0 ? '—' : `${capacity.percent}%`}
        </span>
      </div>
      <div className="mt-1 h-2 overflow-hidden rounded-[0.2vmin] bg-zinc-800 md:mt-[0.5vmin] md:h-[1.2vmin]">
        {capacity.percent >= 0 && (
          <div
            className={
              capacity.percent >= 90
                ? 'h-full bg-red-600'
                : capacity.percent >= 75
                  ? 'h-full bg-amber-600'
                  : 'h-full bg-green-700'
            }
            style={{ width: `${Math.min(100, Math.max(0, capacity.percent))}%` }}
          />
        )}
      </div>
    </div>
  )
}

/**
 * needsAttention decides what gets named on the left.
 *
 * `stopped` is NOT here: it is a decision somebody made, and a wall that put
 * every deliberately stopped workload in the attention list would be a wall
 * asking to be ignored. It is counted in its namespace's tile instead.
 */
function needsAttention(tile: WallTile): boolean {
  return tile.state === 'down' || tile.state === 'unknown' || tile.state === 'warn'
}

/**
 * sinceText turns the instant a warning was last seen into an age.
 *
 * Unparseable instants are shown as they came rather than swallowed: a wall that
 * silently drops a field it did not understand is a wall that cannot be debugged
 * from across the room.
 */
function sinceText(instant: string, now: number): string {
  const at = new Date(instant)
  if (Number.isNaN(at.getTime())) return instant
  return `${readableAge(now - at.getTime())} ago`
}

/** readableAge is what somebody four metres away can read at a glance. */
function readableAge(ms: number): string {
  const seconds = Math.max(0, Math.round(ms / 1000))
  if (seconds < 90) return `${seconds}s`
  const minutes = Math.round(seconds / 60)
  if (minutes < 90) return `${minutes} min`
  return `${Math.round(minutes / 60)} h`
}

function plural(count: number, word: string): string {
  return `${count} ${word}${count === 1 ? '' : 's'}`
}

/**
 * Hung off the ROOT and not the authenticated layout, deliberately.
 *
 * That layout's component is the AppShell: sidebar, header, chain banner. All of
 * it is right for somebody working and all of it is wrong on a wall, where
 * nobody is going anywhere and every pixel of navigation is a pixel that could
 * have been a tile.
 *
 * What it gives up is the session gate that layout carries — which is why the
 * kiosk link exists: a credential of its own, readable and nothing else, so the
 * screen can be left up for weeks without a session expiring behind it. See
 * internal/auth/walllink.go.
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
