import { afterEach, describe, expect, it, vi } from 'vitest'
import { formatCount, formatDate, formatDateTime, formatTime } from '@/lib/format'

describe('date formatting', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('prints the same text whatever the browser language is', () => {
    const at = '2026-10-06T14:05:09Z'
    const before = [formatDateTime(at), formatDate(at), formatTime(at)]

    vi.spyOn(Date.prototype, 'toLocaleString').mockReturnValue('10/6/2026, 2:05:09 PM')
    vi.spyOn(Date.prototype, 'toLocaleDateString').mockReturnValue('10/6/2026')
    vi.spyOn(Date.prototype, 'toLocaleTimeString').mockReturnValue('2:05:09 PM')

    expect([formatDateTime(at), formatDate(at), formatTime(at)]).toEqual(before)
    expect(formatDate(at)).toMatch(/^\d{1,2} Oct 2026$/)
    expect(formatCount(1234567)).toBe('1,234,567')
  })

  it('prints a dash, not "Invalid Date", for a value that is not a date', () => {
    expect(formatDateTime('not a date')).toBe('—')
    expect(formatDate(undefined)).toBe('—')
    expect(formatTime(null)).toBe('—')
    expect(formatDateTime('')).toBe('—')
    expect(formatDateTime(new Date(Number.NaN))).toBe('—')
  })
})
