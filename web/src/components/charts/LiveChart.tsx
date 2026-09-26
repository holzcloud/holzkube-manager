import { type KeyboardEvent, type PointerEvent, useEffect, useRef, useState } from 'react'
import type { Point } from '@/hooks/useLiveSeries'
import { LIVE_WINDOW_MS } from '@/hooks/useLiveSeries'

/**
 * The last few minutes of a live reading, as a line (2026-09-26).
 *
 * Built by hand in SVG rather than from a charting library: the product has
 * none, one curve type does not justify a dependency, and every rule this has
 * to keep -- one axis, 2px lines, a hairline grid, a crosshair that finds the
 * time rather than making the reader hit a line -- is a few lines of SVG.
 *
 * At most two series, and always the same unit: two measures of different
 * scale are two charts here, never one chart with two axes.
 *
 * Every value the tooltip shows is also in the table under "Show as table";
 * the tooltip enhances and never gates.
 */

export interface LiveSeries {
  key: string
  label: string
  /** Categorical slot: 1 is brass, 2 is blue. Validated as a pair. */
  slot: 1 | 2
  points: Point[]
}

interface LiveChartProps {
  /** What is plotted, for the accessible name and the table caption. */
  title: string
  series: LiveSeries[]
  format: (value: number) => string
  /** A fixed ceiling, e.g. 100 for a percentage. Otherwise it follows the data. */
  yMax?: number
  windowMs?: number
  height?: number
}

const GUTTER = 48
const PAD_TOP = 8
const PAD_BOTTOM = 18

export function LiveChart({
  title,
  series,
  format,
  yMax,
  windowMs = LIVE_WINDOW_MS,
  height = 128,
}: LiveChartProps) {
  const [ref, width] = useWidth<HTMLDivElement>()
  const [focus, setFocus] = useState<number | null>(null)

  const base = series[0]?.points ?? []
  const latest = Math.max(0, ...series.flatMap((s) => s.points.map((p) => p.t)))
  const start = latest - windowMs
  const top = yMax ?? niceCeiling(Math.max(0, ...series.flatMap((s) => s.points.map((p) => p.v))))

  const plotW = Math.max(10, width - GUTTER)
  const plotH = Math.max(10, height - PAD_TOP - PAD_BOTTOM)
  const x = (t: number) => GUTTER + ((t - start) / windowMs) * plotW
  const y = (v: number) => PAD_TOP + plotH - (Math.min(v, top) / top) * plotH

  const onPointerMove = (e: PointerEvent<SVGSVGElement>) => {
    if (base.length === 0) return
    const box = e.currentTarget.getBoundingClientRect()
    const t = start + ((e.clientX - box.left - GUTTER) / plotW) * windowMs
    setFocus(nearest(base, t))
  }

  const onKeyDown = (e: KeyboardEvent<SVGSVGElement>) => {
    if (base.length === 0) return
    const current = focus ?? base.length - 1
    if (e.key === 'ArrowLeft') setFocus(Math.max(0, current - 1))
    else if (e.key === 'ArrowRight') setFocus(Math.min(base.length - 1, current + 1))
    else return
    e.preventDefault()
  }

  const focused = focus !== null ? base[focus] : undefined
  const ticks = [0, top / 2, top]

  return (
    <figure className="min-w-0">
      {series.length > 1 && (
        <ul className="mb-1 flex flex-wrap gap-x-4 gap-y-1 text-xs">
          {series.map((s) => (
            <li key={s.key} className="flex items-center gap-1.5">
              <LineKey slot={s.slot} />
              <span className="text-muted-foreground">{s.label}</span>
              <span className="tabular-nums">
                {lastOf(s.points) !== undefined ? format((lastOf(s.points) as Point).v) : '—'}
              </span>
            </li>
          ))}
        </ul>
      )}

      <div ref={ref} className="relative w-full" style={{ height }}>
        <svg
          role="img"
          aria-label={`${title}, last ${Math.round(windowMs / 60_000)} minutes`}
          width={width}
          height={height}
          // biome-ignore lint/a11y/noNoninteractiveTabindex: the chart is explorable by keyboard — arrows move the readout — which is what makes it more than a picture
          tabIndex={0}
          className="block touch-none outline-none focus-visible:ring-2 focus-visible:ring-ring rounded-sm"
          onPointerMove={onPointerMove}
          onPointerLeave={() => setFocus(null)}
          onFocus={() => setFocus(base.length > 0 ? base.length - 1 : null)}
          onBlur={() => setFocus(null)}
          onKeyDown={onKeyDown}
        >
          {ticks.map((v) => (
            <g key={v}>
              <line
                x1={GUTTER}
                x2={GUTTER + plotW}
                y1={y(v)}
                y2={y(v)}
                stroke="var(--viz-grid)"
                strokeWidth={1}
                shapeRendering="crispEdges"
              />
              <text
                x={GUTTER - 6}
                y={y(v)}
                textAnchor="end"
                dominantBaseline="middle"
                className="fill-muted-foreground text-[10px] tabular-nums"
              >
                {format(v)}
              </text>
            </g>
          ))}
          <text
            x={GUTTER}
            y={height - 4}
            className="fill-muted-foreground text-[10px]"
          >{`−${Math.round(windowMs / 60_000)} min`}</text>
          <text
            x={GUTTER + plotW}
            y={height - 4}
            textAnchor="end"
            className="fill-muted-foreground text-[10px]"
          >
            now
          </text>

          {series.map((s) => {
            const pts = s.points.filter((p) => p.t >= start)
            const first = pts[0]
            const last = lastOf(pts)
            if (first === undefined || last === undefined) return null
            const line = pts.map((p, i) => `${i === 0 ? 'M' : 'L'}${x(p.t)},${y(p.v)}`).join(' ')
            const area = `${line} L${x(last.t)},${y(0)} L${x(first.t)},${y(0)} Z`
            const color = `var(--viz-series-${s.slot})`
            return (
              <g key={s.key}>
                <path d={area} fill={color} fillOpacity={0.1} stroke="none" />
                <path
                  d={line}
                  fill="none"
                  stroke={color}
                  strokeWidth={2}
                  strokeLinejoin="round"
                  strokeLinecap="round"
                />
                <circle
                  cx={x(last.t)}
                  cy={y(last.v)}
                  r={4}
                  fill={color}
                  stroke="var(--card)"
                  strokeWidth={2}
                />
              </g>
            )
          })}

          {focused && (
            <line
              x1={x(focused.t)}
              x2={x(focused.t)}
              y1={PAD_TOP}
              y2={PAD_TOP + plotH}
              stroke="currentColor"
              strokeOpacity={0.35}
              strokeWidth={1}
              shapeRendering="crispEdges"
            />
          )}
        </svg>

        {focused && (
          <div
            role="status"
            className="pointer-events-none absolute top-1 z-10 min-w-32 rounded-md border bg-popover px-2 py-1.5 text-xs shadow-md"
            style={
              x(focused.t) > width / 2
                ? { right: width - x(focused.t) + 8 }
                : { left: x(focused.t) + 8 }
            }
          >
            <p className="text-muted-foreground">{new Date(focused.t).toLocaleTimeString()}</p>
            {series.map((s) => {
              const p = s.points[focus ?? 0]
              return (
                <p key={s.key} className="flex items-center gap-1.5">
                  <LineKey slot={s.slot} />
                  <span className="font-semibold tabular-nums">
                    {p !== undefined ? format(p.v) : '—'}
                  </span>
                  <span className="text-muted-foreground">{s.label}</span>
                </p>
              )
            })}
          </div>
        )}

        {base.length < 2 && (
          <p className="absolute inset-x-0 top-1/2 -translate-y-1/2 text-center text-muted-foreground text-xs">
            The curve starts now; it fills in as readings arrive.
          </p>
        )}
      </div>

      <details className="mt-1 text-xs">
        <summary className="cursor-pointer text-muted-foreground">Show as table</summary>
        <table className="mt-1 w-full tabular-nums">
          <caption className="sr-only">{title}</caption>
          <thead>
            <tr className="text-left text-muted-foreground">
              <th className="font-normal">Time</th>
              {series.map((s) => (
                <th key={s.key} className="font-normal">
                  {s.label}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {base
              .map((p, i) => ({ p, i }))
              .slice(-20)
              .reverse()
              .map(({ p, i }) => (
                <tr key={p.t}>
                  <td>{new Date(p.t).toLocaleTimeString()}</td>
                  {series.map((s) => (
                    <td key={s.key}>{s.points[i] !== undefined ? format(s.points[i].v) : '—'}</td>
                  ))}
                </tr>
              ))}
          </tbody>
        </table>
      </details>
    </figure>
  )
}

function lastOf<T>(items: T[]): T | undefined {
  return items[items.length - 1]
}

/** A short stroke of the series colour: a line key, not a box. */
function LineKey({ slot }: { slot: 1 | 2 }) {
  return (
    <span
      aria-hidden="true"
      className="inline-block h-0.5 w-3 rounded-full"
      style={{ background: `var(--viz-series-${slot})` }}
    />
  )
}

/** nearest is the index of the point closest in time to t. */
export function nearest(points: Point[], t: number): number {
  let best = 0
  let distance = Number.POSITIVE_INFINITY
  points.forEach((p, i) => {
    if (Math.abs(p.t - t) < distance) {
      distance = Math.abs(p.t - t)
      best = i
    }
  })
  return best
}

/**
 * niceCeiling rounds a maximum up to a clean tick: 1, 2, 2.5, 5 times a power
 * of ten. A zero series still gets an axis, because a flat line at the floor of
 * a chart with no scale reads as "no data".
 */
export function niceCeiling(value: number): number {
  if (!(value > 0)) return 1
  const padded = value * 1.15
  const power = 10 ** Math.floor(Math.log10(padded))
  for (const step of [1, 2, 2.5, 5, 10]) {
    if (step * power >= padded) return step * power
  }
  return 10 * power
}

/** useWidth measures an element, so the SVG draws at its real pixel size. */
function useWidth<T extends HTMLElement>(): [(node: T | null) => void, number] {
  const [node, setNode] = useState<T | null>(null)
  const [width, setWidth] = useState(600)
  const observer = useRef<ResizeObserver | null>(null)

  useEffect(() => {
    if (node === null) return
    setWidth(node.clientWidth || 600)
    if (typeof ResizeObserver === 'undefined') return
    observer.current = new ResizeObserver((entries) => {
      const w = entries[0]?.contentRect.width
      if (w !== undefined && w > 0) setWidth(w)
    })
    observer.current.observe(node)
    return () => observer.current?.disconnect()
  }, [node])

  return [setNode, width]
}
