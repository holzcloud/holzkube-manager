import { describe, expect, it } from 'vitest'
import { historySchema } from '@/api'
import { segments, spanLabel } from '@/components/charts/LiveChart'
import { merge } from '@/hooks/useChartRange'

/**
 * The charts start yesterday, not when the page was opened (2026-09-26).
 *
 * A chart is the daemon's record followed by the page's own readings, cut to
 * the chosen range, and a stretch where nothing was read is a gap, not a line.
 */

const now = 1_800_000_000_000
const history = historySchema.parse({
  range: '1h',
  step_seconds: 15,
  series: {
    cpu: [
      [now - 30 * 60_000, 10],
      [now - 60_000, 20],
    ],
  },
})

describe('merge', () => {
  it('puts the record first and the live readings after its last point', () => {
    const live = {
      cpu: [
        { t: now - 90_000, v: 99 }, // older than the record's last point: already in it
        { t: now - 3_000, v: 30 },
      ],
    }
    const out = merge(history, live, 60 * 60_000, now)
    expect(out.cpu?.map((p) => p.v)).toEqual([10, 20, 30])
  })

  it('cuts everything to the chosen range', () => {
    const out = merge(history, {}, 5 * 60_000, now)
    expect(out.cpu?.map((p) => p.v)).toEqual([20])
  })

  it('still draws the live readings when nothing was recorded', () => {
    const out = merge(undefined, { cpu: [{ t: now, v: 5 }] }, 5 * 60_000, now)
    expect(out.cpu?.map((p) => p.v)).toEqual([5])
  })
})

describe('a chart with a hole in it', () => {
  it('breaks the line where readings stopped', () => {
    const points = [0, 15, 30, 45, 600, 615].map((s) => ({ t: s * 1000, v: 1 }))
    expect(segments(points).map((run) => run.length)).toEqual([4, 2])
  })

  it('keeps a steady record as one line', () => {
    const points = [0, 60, 120, 180].map((s) => ({ t: s * 1000, v: 1 }))
    expect(segments(points)).toHaveLength(1)
  })

  it('says how far back it reaches', () => {
    expect(spanLabel(5 * 60_000)).toBe('5 min')
    expect(spanLabel(60 * 60_000)).toBe('1 h')
    expect(spanLabel(24 * 60 * 60_000)).toBe('24 h')
  })
})
