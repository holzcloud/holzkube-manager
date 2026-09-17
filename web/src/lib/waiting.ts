import { useEffect, useState } from 'react'

/**
 * What a wait has to say for itself.
 *
 * PITFALLS.md:164 item 5 names the four, and it is about the bootstrap window
 * rather than about taste: a spinner invites the second click, and a second
 * bootstrap is a split brain. "Elapsed time, current sub-step, expected
 * duration, and -- critically -- an explicitly disabled Bootstrap button with
 * the reason." PITFALLS.md:646 repeats it as the UX row: a spinner is the
 * anti-pattern, "named state, elapsed time, expected duration, current
 * sub-step, disabled action buttons with reasons" is the approach.
 *
 * Ledger entry 62 records that what shipped instead was one static sentence per
 * waiting state naming the server's own CEILING -- which is a maximum and not a
 * prediction, and tells an operator nothing about whether this run is going
 * normally.
 *
 * The expected duration here is MEASURED and never estimated. The operator's
 * decision of 2026-09-17 was explicit about the alternative: a plausible
 * constant would be an assertion with no measurement behind it, which is the
 * kind of statement this repository files as a defect rather than shipping.
 */

/** Seconds since an ISO timestamp, re-rendered once a second while live. */
export function useElapsedSeconds(since: string | undefined, live: boolean): number | null {
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    if (!live || since === undefined || since === '') {
      return
    }
    // One second, because the number is read rather than watched: a faster
    // tick spends renders on a digit nobody is looking at, and a slower one
    // makes the screen feel stopped, which is the thing this exists against.
    const timer = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(timer)
  }, [live, since])

  if (since === undefined || since === '') {
    return null
  }
  const started = Date.parse(since)
  if (Number.isNaN(started)) {
    return null
  }
  return Math.max(0, Math.round((now - started) / 1000))
}

/** "18s", "3m12s", "1h04m" -- the longest unit first and never more than two. */
export function formatDuration(seconds: number): string {
  if (seconds < 60) {
    return `${seconds}s`
  }
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) {
    return `${minutes}m${String(seconds % 60).padStart(2, '0')}s`
  }
  return `${Math.floor(minutes / 60)}h${String(minutes % 60).padStart(2, '0')}m`
}

/**
 * The middle of what has actually happened, or null when nothing has.
 *
 * The median rather than the mean, because one job that parked for an hour
 * waiting on a person is not evidence about how long the next one takes, and a
 * mean would carry it forever.
 */
export function medianSeconds(samples: readonly number[]): number | null {
  const usable = samples.filter((s) => Number.isFinite(s) && s >= 0).sort((a, b) => a - b)
  if (usable.length === 0) {
    return null
  }
  const middle = Math.floor(usable.length / 2)
  // Indexed access is possibly-undefined under noUncheckedIndexedAccess, and
  // the compiler is right to say so rather than be talked out of it: the guard
  // above establishes the length, not the type.
  const upper = usable[middle] ?? 0
  if (usable.length % 2 === 1) {
    return upper
  }
  return Math.round(((usable[middle - 1] ?? upper) + upper) / 2)
}

/** The seconds between two ISO timestamps, or null if either is unusable. */
export function secondsBetween(from: string | undefined, to: string | undefined): number | null {
  if (from === undefined || from === '' || to === undefined || to === '') {
    return null
  }
  const start = Date.parse(from)
  const end = Date.parse(to)
  if (Number.isNaN(start) || Number.isNaN(end) || end < start) {
    return null
  }
  return Math.round((end - start) / 1000)
}

/**
 * How long this kind of wait took the last few times, kept in this browser.
 *
 * Used where the server holds no history of its own -- the Image Factory build
 * is a request, not a job, so nothing but the browser that waited for it knows
 * how long it took. Where there IS a record, as with jobs, the median comes
 * from the records and this is not used: a number every browser computes the
 * same way from shared data beats one that differs per machine.
 *
 * Bounded at eight samples, because what the Factory did last year says
 * nothing about today and an unbounded list would keep saying it.
 */
const HISTORY_LIMIT = 8

export function readDurationHistory(key: string): number[] {
  try {
    const raw = localStorage.getItem(`holzkube.duration.${key}`)
    if (raw === null) {
      return []
    }
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) {
      return []
    }
    return parsed.filter((n): n is number => typeof n === 'number' && Number.isFinite(n) && n >= 0)
  } catch {
    // A browser with storage refused still gets the elapsed counter and the
    // ceiling; it just has no history to compare against. Saying so is the
    // component's job, not this one's.
    return []
  }
}

export function recordDuration(key: string, seconds: number): void {
  if (!Number.isFinite(seconds) || seconds < 0) {
    return
  }
  try {
    const next = [...readDurationHistory(key), Math.round(seconds)].slice(-HISTORY_LIMIT)
    localStorage.setItem(`holzkube.duration.${key}`, JSON.stringify(next))
  } catch {
    // See readDurationHistory.
  }
}
