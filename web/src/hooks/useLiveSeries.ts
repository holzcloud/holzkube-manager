import { useEffect, useState } from 'react'

/**
 * The recent past of a live reading, kept in the browser (2026-09-26).
 *
 * The server reads the node when asked and remembers nothing, which is the
 * operator's rule: the manager shows the cluster as it is. A curve needs the
 * last few minutes, so the page keeps them -- every reading it was handed, for
 * as long as the window is long. Opening the page starts the curve; nothing is
 * invented for the time before.
 *
 * Held in a module-level store keyed by the page's subject rather than in
 * component state, so leaving a node's page and coming back within the window
 * finds the curve where it was instead of starting again from one point.
 */

export interface Point {
  /** Milliseconds since the epoch, from the reading's own timestamp. */
  t: number
  v: number
}

export type Series = Record<string, Point[]>

const store = new Map<string, Series>()

/** The default window: five minutes, about a hundred readings at 3 s. */
export const LIVE_WINDOW_MS = 5 * 60_000

/**
 * append records one reading and drops what has fallen out of the window.
 *
 * Exported for the test; the hook is its only other caller. A reading whose
 * timestamp is not newer than the last one is ignored, because react-query hands
 * the same answer back on every render and a curve with the same point twice is
 * a curve that lies about how often it was measured.
 */
export function append(
  key: string,
  at: number,
  values: Record<string, number>,
  windowMs = LIVE_WINDOW_MS,
): Series {
  const current = store.get(key) ?? {}
  const next: Series = {}
  const cutoff = at - windowMs
  for (const [name, value] of Object.entries(values)) {
    const points = current[name] ?? []
    const last = points[points.length - 1]
    const kept = points.filter((p) => p.t >= cutoff)
    next[name] = last !== undefined && last.t >= at ? kept : [...kept, { t: at, v: value }]
  }
  store.set(key, next)
  return next
}

/** forget clears one subject's history; for tests. */
export function forget(key?: string) {
  if (key === undefined) store.clear()
  else store.delete(key)
}

/**
 * useLiveSeries returns the history of the values `pick` reads off each new
 * reading. `at` is the reading's own timestamp (ISO); `key` names the subject.
 */
export function useLiveSeries<T>(
  key: string,
  reading: T | undefined,
  at: string | undefined,
  pick: (reading: T) => Record<string, number>,
): Series {
  const [series, setSeries] = useState<Series>(() => store.get(key) ?? {})

  // `pick` is deliberately not a dependency: callers pass an inline function,
  // and the reading's timestamp is what says there is something new to record.
  // biome-ignore lint/correctness/useExhaustiveDependencies: see above
  useEffect(() => {
    if (reading === undefined || at === undefined) {
      setSeries(store.get(key) ?? {})
      return
    }
    const t = Date.parse(at)
    if (Number.isNaN(t)) return
    setSeries(append(key, t, pick(reading)))
  }, [key, at, reading])

  return series
}
