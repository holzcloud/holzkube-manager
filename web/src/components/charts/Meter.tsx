import { AlertTriangle, Flame } from 'lucide-react'
import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

/**
 * One reading against its ceiling, drawn (2026-09-26).
 *
 * The fill carries severity and nothing else: it is --viz-ok until the value
 * crosses `warn`, then --viz-warn, then --viz-danger. Severity is never colour
 * alone -- past `warn` an icon and a word ride beside the figure, so a reader
 * who cannot tell amber from green still reads "hot".
 *
 * The figure is text in text tokens beside the bar, never inside it: a bar at
 * 3% has no room for a label and a label that only fits sometimes is a label
 * that moves.
 */

export type Severity = 'ok' | 'warn' | 'danger'

export function severityOf(value: number, warn?: number, danger?: number): Severity {
  if (danger !== undefined && value >= danger) return 'danger'
  if (warn !== undefined && value >= warn) return 'warn'
  return 'ok'
}

const FILL: Record<Severity, string> = {
  ok: 'var(--viz-ok)',
  warn: 'var(--viz-warn)',
  danger: 'var(--viz-danger)',
}

export interface MeterProps {
  label: ReactNode
  /** The figure beside the bar, already formatted. */
  display: string
  value: number
  max: number
  warn?: number
  danger?: number
  /** A second line under the bar: "5.4 GiB of 55 GiB". */
  detail?: ReactNode
  className?: string
  /** Compact rows for lists of cores or sensors. */
  dense?: boolean
}

export function Meter({
  label,
  display,
  value,
  max,
  warn,
  danger,
  detail,
  className,
  dense,
}: MeterProps) {
  const severity = severityOf(value, warn, danger)
  const width = max > 0 ? Math.min(100, Math.max(0, (value / max) * 100)) : 0

  return (
    <div className={cn('min-w-0', className)} data-severity={severity}>
      <div className="flex items-baseline justify-between gap-2">
        <span className={cn('min-w-0 truncate', dense ? 'text-xs' : 'text-sm')}>{label}</span>
        <span
          className={cn(
            'flex shrink-0 items-center gap-1 tabular-nums',
            dense ? 'text-xs' : 'text-sm font-medium',
          )}
        >
          {severity === 'danger' && (
            <Flame aria-hidden="true" className="size-3.5 text-[color:var(--viz-danger)]" />
          )}
          {severity === 'warn' && (
            <AlertTriangle aria-hidden="true" className="size-3.5 text-[color:var(--viz-warn)]" />
          )}
          {display}
          {severity !== 'ok' && (
            <span className="sr-only">{severity === 'danger' ? ' (critical)' : ' (high)'}</span>
          )}
        </span>
      </div>
      <div
        aria-hidden="true"
        className={cn('w-full overflow-hidden rounded-[2px]', dense ? 'mt-0.5 h-1.5' : 'mt-1 h-2')}
        style={{ background: 'var(--viz-track)' }}
      >
        <div
          className="h-full rounded-r-[4px] transition-[width] duration-500"
          style={{ width: `${width}%`, background: FILL[severity] }}
        />
      </div>
      {detail !== undefined && (
        <p className="mt-1 text-muted-foreground text-xs tabular-nums">{detail}</p>
      )}
    </div>
  )
}

/**
 * StackedBar is a whole split into named parts: memory as used, cache and free.
 *
 * Parts are separated by a 2px gap in the track colour rather than a border,
 * and the legend beside it names each with its swatch and figure -- identity
 * is never colour alone.
 */
export function StackedBar({
  parts,
  total,
  label,
}: {
  parts: { key: string; label: string; value: number; color: string; display: string }[]
  total: number
  label: string
}) {
  return (
    <div>
      <div
        role="img"
        aria-label={`${label}: ${parts.map((p) => `${p.label} ${p.display}`).join(', ')}`}
        className="flex h-3 w-full gap-[2px] overflow-hidden rounded-[4px]"
        style={{ background: 'var(--viz-track)' }}
      >
        {parts.map((p) =>
          total > 0 && p.value > 0 ? (
            <div
              key={p.key}
              className="h-full transition-[width] duration-500"
              style={{ width: `${(p.value / total) * 100}%`, background: p.color }}
            />
          ) : null,
        )}
      </div>
      <ul className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs">
        {parts.map((p) => (
          <li key={p.key} className="flex items-center gap-1.5">
            <span
              aria-hidden="true"
              className="inline-block size-2.5 rounded-[2px]"
              style={{ background: p.color }}
            />
            <span className="text-muted-foreground">{p.label}</span>
            <span className="tabular-nums">{p.display}</span>
          </li>
        ))}
      </ul>
    </div>
  )
}

/** StatTile is a single figure with its label: the chart is the number. */
export function StatTile({
  label,
  value,
  hint,
  className,
}: {
  label: string
  value: ReactNode
  hint?: ReactNode
  className?: string
}) {
  return (
    <div className={cn('min-w-0 rounded-md border px-3 py-2', className)}>
      <p className="text-muted-foreground text-xs">{label}</p>
      <p className="truncate font-semibold text-xl">{value}</p>
      {hint !== undefined && <p className="truncate text-muted-foreground text-xs">{hint}</p>}
    </div>
  )
}
