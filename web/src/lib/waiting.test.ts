import { afterEach, describe, expect, it } from 'vitest'
import {
  formatDuration,
  medianSeconds,
  readDurationHistory,
  recordDuration,
  secondsBetween,
} from '@/lib/waiting'

/**
 * The arithmetic behind "expected duration", which PITFALLS.md:646 names and
 * ledger entry 62 records as missing.
 *
 * The operator's decision of 2026-09-17 was that this number is MEASURED and
 * never estimated, so what these pin is that it cannot quietly become an
 * estimate: a median of nothing is null and says so, and a sample that did not
 * happen never enters the set.
 */
describe('medianSeconds', () => {
  it('has no opinion until something has happened', () => {
    expect(medianSeconds([])).toBeNull()
  })

  it('takes the middle of an odd count', () => {
    expect(medianSeconds([30, 10, 20])).toBe(20)
  })

  it('averages the two middles of an even count', () => {
    expect(medianSeconds([10, 20, 30, 40])).toBe(25)
  })

  it('ignores a run that parked for an hour rather than carrying it forever', () => {
    // The median and not the mean, and this is the case that decides it: one
    // job that waited on a person is not evidence about how long the work
    // takes, and a mean would let it dominate every later answer.
    expect(medianSeconds([18, 20, 22, 3600])).toBe(21)
    const mean = (18 + 20 + 22 + 3600) / 4
    expect(mean).toBeGreaterThan(900)
  })

  it('drops samples that are not durations', () => {
    expect(medianSeconds([Number.NaN, -5, 10, 20, 30])).toBe(20)
  })
})

describe('secondsBetween', () => {
  it('measures a finished run', () => {
    expect(secondsBetween('2026-09-17T10:00:00Z', '2026-09-17T10:00:18Z')).toBe(18)
  })

  it('refuses a run that has not finished', () => {
    expect(secondsBetween('2026-09-17T10:00:00Z', undefined)).toBeNull()
    expect(secondsBetween('2026-09-17T10:00:00Z', '')).toBeNull()
  })

  it('refuses an end before its start rather than reporting a negative wait', () => {
    expect(secondsBetween('2026-09-17T10:00:18Z', '2026-09-17T10:00:00Z')).toBeNull()
  })

  it('refuses a timestamp it cannot read', () => {
    expect(secondsBetween('not a time', '2026-09-17T10:00:00Z')).toBeNull()
  })
})

describe('formatDuration', () => {
  it('reads as a duration and not as a number of seconds', () => {
    expect(formatDuration(18)).toBe('18s')
    expect(formatDuration(59)).toBe('59s')
    expect(formatDuration(60)).toBe('1m00s')
    expect(formatDuration(192)).toBe('3m12s')
    expect(formatDuration(3600)).toBe('1h00m')
    expect(formatDuration(3900)).toBe('1h05m')
  })
})

describe('the browser-side history', () => {
  afterEach(() => localStorage.clear())

  it('remembers what actually happened and returns it in order of arrival', () => {
    recordDuration('factory.create', 20)
    recordDuration('factory.create', 18)
    expect(readDurationHistory('factory.create')).toEqual([20, 18])
    expect(medianSeconds(readDurationHistory('factory.create'))).toBe(19)
  })

  it('keeps at most eight, because last year says nothing about today', () => {
    for (let i = 1; i <= 12; i += 1) {
      recordDuration('factory.create', i)
    }
    expect(readDurationHistory('factory.create')).toEqual([5, 6, 7, 8, 9, 10, 11, 12])
  })

  it('records nothing that is not a duration', () => {
    recordDuration('factory.create', Number.NaN)
    recordDuration('factory.create', -1)
    expect(readDurationHistory('factory.create')).toEqual([])
  })

  it('survives a stored value that is not a list of numbers', () => {
    localStorage.setItem('holzkube.duration.factory.create', '{"not":"a list"}')
    expect(readDurationHistory('factory.create')).toEqual([])
  })

  it('keeps one kind of wait out of another', () => {
    recordDuration('factory.create', 20)
    expect(readDurationHistory('factory.assets')).toEqual([])
  })
})
