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
