import { type UseQueryResult, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import type { History, HistoryRange } from '@/api'
import { LIVE_WINDOW_MS, type Point, type Series } from '@/hooks/useLiveSeries'

/**
 * Which stretch of time the charts on a page show, and the points for it
 * (2026-09-26).
 *
 * The operator's words: the curve must not start when the page is opened, and
 * the last 24 hours must be visible. So a chart is the daemon's recorded
 * history for the chosen range with the page's own live readings appended after
 * its last point -- the history says what happened, the live readings keep the
 * right-hand edge moving every few seconds without asking the server for a
 * day's worth of points each time.
 *
 * "Live" is the last five minutes, and it too starts from history: opening the
 * page shows the five minutes before it was opened, not an empty chart.
 */

export type ChartRange = 'live' | HistoryRange

export const CHART_RANGES: { value: ChartRange; label: string; ms: number }[] = [
  { value: 'live', label: 'Live', ms: LIVE_WINDOW_MS },
  { value: '1h', label: '1 h', ms: 60 * 60_000 },
  { value: '6h', label: '6 h', ms: 6 * 60 * 60_000 },
  { value: '24h', label: '24 h', ms: 24 * 60 * 60_000 },
]

export function windowOf(range: ChartRange): number {
  return CHART_RANGES.find((r) => r.value === range)?.ms ?? LIVE_WINDOW_MS
}

/**
 * merge is the history for a key, then the live points after it, both cut to
 * the window ending at `now`. Exported for the test.
 */
export function merge(
  history: History | undefined,
  live: Series,
  windowMs: number,
  now: number,
): Series {
  const out: Series = {}
  const keys = new Set([...Object.keys(history?.series ?? {}), ...Object.keys(live)])
  const cutoff = now - windowMs
  for (const key of keys) {
    const recorded: Point[] = (history?.series[key] ?? []).map(([t, v]) => ({ t, v }))
    const lastRecorded = recorded[recorded.length - 1]?.t ?? Number.NEGATIVE_INFINITY
    const fresh = (live[key] ?? []).filter((p) => p.t > lastRecorded)
    out[key] = [...recorded, ...fresh].filter((p) => p.t >= cutoff)
  }
  return out
}

/**
 * useChartRange holds the chosen range and fetches the history for it. The
 * live range reads the hour's history once, for the five minutes before the
 * page was opened; the others refresh while the page is open.
 */
export function useChartRange(
  key: unknown[],
  fetch: (range: HistoryRange) => Promise<History>,
  enabled = true,
): {
  range: ChartRange
  setRange: (r: ChartRange) => void
  history: UseQueryResult<History>
  windowMs: number
} {
  const [range, setRange] = useState<ChartRange>('live')
  const asked: HistoryRange = range === 'live' ? '1h' : range
  const history = useQuery({
    queryKey: [...key, 'history', asked],
    queryFn: () => fetch(asked),
    enabled,
    retry: false,
    refetchInterval: range === 'live' ? false : range === '1h' ? 60_000 : 5 * 60_000,
  })
  return { range, setRange, history, windowMs: windowOf(range) }
}
