import { segments } from '@/components/charts/LiveChart'
import type { Point } from '@/hooks/useLiveSeries'

/**
 * A reading's recent past, the size of a word (2026-09-26).
 *
 * No axis and no tooltip, deliberately: it sits beside the figure it belongs
 * to, and that figure is the value. The line only says "steady", "climbing" or
 * "spiking", which is the one thing the figure alone cannot say. `max` fixes the
 * scale where the quantity has a natural ceiling (100 %, a sensor's critical
 * temperature) so two sparklines side by side are comparable. Where readings
 * stopped it breaks, as the large chart does (segments): small is no licence
 * to draw a reading nobody took.
 */
export function Sparkline({
  points,
  max,
  color = 'var(--viz-series-1)',
  width = 96,
  height = 26,
  label,
}: {
  points: Point[]
  max?: number
  color?: string
  width?: number
  height?: number
  label: string
}) {
  const first = points[0]
  const last = points[points.length - 1]
  if (points.length < 2 || first === undefined || last === undefined) {
    return <svg role="img" aria-label={`${label}: collecting`} width={width} height={height} />
  }
  const t0 = first.t
  const span = Math.max(1, last.t - t0)
  const top = max ?? Math.max(1e-9, ...points.map((p) => p.v)) * 1.1
  const x = (t: number) => ((t - t0) / span) * width
  const y = (v: number) => height - 2 - (Math.min(v, top) / (top || 1)) * (height - 4)
  const runs = segments(points)
  const paths = runs.map((run) =>
    run.map((p, i) => `${i === 0 ? 'M' : 'L'}${x(p.t).toFixed(1)},${y(p.v).toFixed(1)}`).join(' '),
  )
  const line = paths.join(' ')
  // Each run is filled down to the baseline under itself only.
  const area = runs
    .map((run, i) => {
      const a = run[0] as Point
      const b = run[run.length - 1] as Point
      return `${paths[i]} L${x(b.t).toFixed(1)},${height} L${x(a.t).toFixed(1)},${height} Z`
    })
    .join(' ')

  return (
    <svg role="img" aria-label={label} width={width} height={height} className="shrink-0">
      <path d={area} fill={color} fillOpacity={0.1} />
      <path d={line} fill="none" stroke={color} strokeWidth={1.5} strokeLinejoin="round" />
    </svg>
  )
}
