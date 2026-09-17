import { describe, expect, it } from 'vitest'
import type { Job } from '@/api'
import { typicalSecondsFor } from '@/routes/jobs'

/**
 * How long a job of this kind usually takes, and where that number may come
 * from.
 *
 * PITFALLS.md:164 item 5 asks for an expected duration beside the elapsed one,
 * and it is about bootstrap rather than about polish: an operator who cannot
 * tell a slow run from a stuck one clicks again, and a second bootstrap is a
 * split brain. Ledger entry 62 records that what shipped instead was the
 * server's ceiling, which is a maximum and says nothing about this run.
 *
 * The operator decided on 2026-09-17 that the figure is measured. These pin the
 * two ways that could quietly stop being true: a sample that did not happen
 * entering the set, and an outlier deciding the answer.
 */

function job(over: Partial<Job>): Job {
  return {
    id: 'j',
    kind: 'provision',
    cluster: '',
    machine: '',
    state: 'succeeded',
    steps: [],
    current: 0,
    params: {},
    cancel_requested: false,
    actor: '',
    parked_reason: '',
    created_at: '2026-09-17T10:00:00Z',
    rev: 1,
    ...over,
  }
}

const ran = (seconds: number, over: Partial<Job> = {}): Job =>
  job({
    started_at: '2026-09-17T10:00:00Z',
    finished_at: new Date(Date.parse('2026-09-17T10:00:00Z') + seconds * 1000).toISOString(),
    ...over,
  })

describe('typicalSecondsFor', () => {
  it('has nothing to say until a job of that kind has finished', () => {
    expect(typicalSecondsFor('provision', [])).toBeNull()
    expect(
      typicalSecondsFor('provision', [
        job({ state: 'running', started_at: '2026-09-17T10:00:00Z' }),
      ]),
    ).toBeNull()
  })

  it('reports the middle of the ones that finished', () => {
    expect(typicalSecondsFor('provision', [ran(100), ran(200), ran(300)])).toBe(200)
  })

  it('does not count a job that failed', () => {
    // A failure stopped early. Letting it in would report the work as faster
    // than it is, which is the direction that makes an operator think a normal
    // run has hung.
    expect(typicalSecondsFor('provision', [ran(200), ran(2, { state: 'failed' })])).toBe(200)
  })

  it('does not count a job that parked', () => {
    // Parked means it waited on a person. That is a measurement of somebody's
    // lunch break, not of the operation.
    expect(typicalSecondsFor('provision', [ran(200), ran(4000, { state: 'parked' })])).toBe(200)
  })

  it('does not count a job of another kind', () => {
    expect(typicalSecondsFor('provision', [ran(200), ran(9000, { kind: 'upgrade' })])).toBe(200)
  })

  it('is not moved by a single slow run', () => {
    // The median and not the mean: three normal runs and one that took an
    // hour should still say "about three minutes".
    expect(typicalSecondsFor('provision', [ran(170), ran(180), ran(190), ran(3600)])).toBe(185)
  })

  it('ignores a job whose timestamps cannot be read', () => {
    expect(
      typicalSecondsFor('provision', [
        ran(200),
        job({ started_at: 'nonsense', finished_at: 'also nonsense' }),
      ]),
    ).toBe(200)
  })
})
