/**
 * The units the live views speak (2026-09-26).
 *
 * Memory and filesystems in binary units, because that is what the node and
 * Kubernetes both mean by "Gi": a node sold as 64 GB reports 62.7 GiB, and
 * showing it as "67 GB" would make every screen here disagree with `free`.
 * Throughput in decimal per second, because that is how links are sold and
 * how every other tool on the operator's desk prints a transfer rate.
 */

const BINARY = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB']
const DECIMAL = ['B/s', 'kB/s', 'MB/s', 'GB/s', 'TB/s']

/** formatBytes prints a size in binary units: 5.41 GiB. */
export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
  let unit = 0
  let value = bytes
  while (value >= 1024 && unit < BINARY.length - 1) {
    value /= 1024
    unit++
  }
  return `${trim(value)} ${BINARY[unit]}`
}

/** formatRate prints bytes per second in decimal units: 187.9 kB/s. */
export function formatRate(bytesPerSecond: number): string {
  if (!Number.isFinite(bytesPerSecond) || bytesPerSecond <= 0) return '0 B/s'
  let unit = 0
  let value = bytesPerSecond
  while (value >= 1000 && unit < DECIMAL.length - 1) {
    value /= 1000
    unit++
  }
  return `${trim(value)} ${DECIMAL[unit]}`
}

/**
 * formatCores prints CPU in the unit a limit is written in.
 *
 * Millicores below one core, because "0.004 cores" is a number nobody reads;
 * cores above it, because "2350m" makes somebody divide.
 */
export function formatCores(millis: number): string {
  if (!Number.isFinite(millis) || millis <= 0) return '0m'
  if (millis < 1000) return `${Math.round(millis)}m`
  return `${trim(millis / 1000)} cores`
}

/** formatUptime prints a duration the way a person says it: 3 d 4 h. */
export function formatUptime(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return '—'
  const d = Math.floor(seconds / 86_400)
  const h = Math.floor((seconds % 86_400) / 3_600)
  const m = Math.floor((seconds % 3_600) / 60)
  if (d > 0) return `${d} d ${h} h`
  if (h > 0) return `${h} h ${m} min`
  return `${m} min`
}

/** formatPercent prints a share with the one decimal a dashboard can use. */
export function formatPercent(value: number): string {
  if (!Number.isFinite(value)) return '—'
  return `${value < 10 ? value.toFixed(1) : Math.round(value)}%`
}

/** Three significant figures, never trailing zeros: 5.41, 44.4, 187. */
function trim(value: number): string {
  if (value >= 100) return String(Math.round(value))
  if (value >= 10) return String(Number(value.toFixed(1)))
  return String(Number(value.toFixed(2)))
}

/**
 * Dates and times, in one fixed locale.
 *
 * `toLocaleString()` with no argument prints whatever the browser's language
 * is, so the same audit row read "10/6/2026, 2:05:09 PM" on one desk and
 * "06.10.2026, 14:05:09" on the next, and a screenshot in a bug report did not
 * match the screen it was taken from. The product is English everywhere else
 * (D-09), so these are en-GB: day first, 24-hour clock, and unambiguous. The
 * time zone stays the viewer's own -- an operator reading "14:05" means their
 * clock, not UTC.
 *
 * A value that is not a date prints "—" rather than "Invalid Date", which is
 * what `new Date(undefined).toLocaleString()` produces and what a half-filled
 * API field would otherwise put on screen.
 */
const DATE_TIME = new Intl.DateTimeFormat('en-GB', { dateStyle: 'medium', timeStyle: 'medium' })
const DATE_ONLY = new Intl.DateTimeFormat('en-GB', { dateStyle: 'medium' })
const TIME_ONLY = new Intl.DateTimeFormat('en-GB', { timeStyle: 'medium' })
const COUNT = new Intl.NumberFormat('en-GB')

type DateInput = Date | string | number | null | undefined

function asDate(value: DateInput): Date | null {
  if (value === null || value === undefined || value === '') return null
  const date = value instanceof Date ? value : new Date(value)
  return Number.isNaN(date.getTime()) ? null : date
}

/** formatDateTime prints a moment as 6 Oct 2026, 14:05:09. */
export function formatDateTime(value: DateInput): string {
  const date = asDate(value)
  return date === null ? '—' : DATE_TIME.format(date)
}

/** formatDate prints a day as 6 Oct 2026. */
export function formatDate(value: DateInput): string {
  const date = asDate(value)
  return date === null ? '—' : DATE_ONLY.format(date)
}

/** formatTime prints a time of day as 14:05:09. */
export function formatTime(value: DateInput): string {
  const date = asDate(value)
  return date === null ? '—' : TIME_ONLY.format(date)
}

/** formatCount prints a whole number with grouping: 1,234,567. */
export function formatCount(value: number): string {
  return Number.isFinite(value) ? COUNT.format(value) : '—'
}
