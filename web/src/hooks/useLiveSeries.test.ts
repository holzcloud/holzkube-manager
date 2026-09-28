import { afterEach, describe, expect, it } from 'vitest'
import { append, forget } from '@/hooks/useLiveSeries'

/**
 * A live curve survives a reading that lacks one of its values (2026-09-28).
 *
 * The page writes only what it could read: a hidden CPU or a sensor that failed
 * one read is an absent key, never a 0. Such a key must keep the points it had,
 * so the chart draws a gap for the missing time instead of losing the curve.
 */

afterEach(() => forget())

describe('append', () => {
  it('keeps the points of a key the reading does not carry', () => {
    append('k', 1_000, { a: 1, b: 2 })
    const series = append('k', 4_000, { a: 3 })
    expect(series.a?.map((p) => p.v)).toEqual([1, 3])
    expect(series.b?.map((p) => p.v)).toEqual([2])
    expect(series.b?.map((p) => p.t)).toEqual([1_000])
  })

  it('drops a missing key once its last point is older than the window', () => {
    append('k', 1_000, { a: 1, b: 2 }, 10_000)
    append('k', 5_000, { a: 3 }, 10_000)
    const series = append('k', 12_000, { a: 4 }, 10_000)
    expect(series.a?.map((p) => p.v)).toEqual([3, 4])
    expect('b' in series).toBe(false)
  })

  it('records each reading once and forgets what fell out of the window', () => {
    append('k', 1_000, { a: 1 }, 10_000)
    expect(append('k', 1_000, { a: 2 }, 10_000).a?.map((p) => p.v)).toEqual([1])
    append('k', 5_000, { a: 3 }, 10_000)
    expect(append('k', 20_000, { a: 4 }, 10_000).a?.map((p) => p.v)).toEqual([4])
  })
})
