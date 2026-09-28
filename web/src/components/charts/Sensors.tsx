import { Fan as FanIcon } from 'lucide-react'
import type { ReactNode } from 'react'
import type { Fan, Temperature } from '@/api'
import { severityOf } from '@/components/charts/Meter'
import { Sparkline } from '@/components/charts/Sparkline'
import type { Series } from '@/hooks/useLiveSeries'

/**
 * A machine's temperatures and fans, drawn the same way wherever they appear
 * (2026-09-28): a node's hardware page and the host page carry the same JSON
 * for a sensor and hand it to these components, so the two pages cannot drift.
 *
 * Where a temperature turns amber and red is not decided here. The server sends
 * both lines with every reading (`warn_c`, `danger_c`), worked out once in
 * internal/inventory from the chip's own limits or its kind's default -- the
 * same rule the host's state and its warnings are made from. This file only
 * draws them, so the ▲ beside a sensor and the warning above it cannot
 * disagree at the edge.
 */

export const KIND_LABEL: Record<string, string> = {
  cpu: 'Processor',
  board: 'Mainboard',
  disk: 'Drives',
  gpu: 'Graphics',
  other: 'Other',
}

export const SEVERITY_COLOR = {
  ok: 'var(--viz-ok)',
  warn: 'var(--viz-warn)',
  danger: 'var(--viz-danger)',
} as const

/** A sensor's identity across readings: its chip and its label. */
export function sensorKey(t: Temperature): string {
  return `${t.chip}/${t.label}`
}

/**
 * The sensor column: every temperature the machine reports, grouped by what it
 * measures, each with its figure -- and its recent past, when there is one to
 * draw. Without `history` there is no sparkline column at all, rather than a
 * column of empty curves.
 */
export function Sensors({
  temperatures,
  history,
  emptyText,
}: {
  temperatures: Temperature[]
  history?: Series
  /** The sentence shown when there is no temperature sensor at all. */
  emptyText: string
}) {
  if (temperatures.length === 0) {
    return <p className="text-muted-foreground text-sm">{emptyText}</p>
  }
  const groups = ['cpu', 'board', 'disk', 'gpu', 'other']
    .map((kind) => ({ kind, items: temperatures.filter((t) => (t.kind || 'other') === kind) }))
    .filter((g) => g.items.length > 0)
  return (
    <div className="space-y-3">
      {groups.map((g) => (
        <div key={g.kind}>
          <p className="mb-1 text-muted-foreground text-xs">{KIND_LABEL[g.kind] ?? g.kind}</p>
          <ul className="divide-y">
            {g.items.map((t) => {
              const severity = severityOf(t.celsius, t.warn_c, t.danger_c)
              return (
                <li
                  key={sensorKey(t)}
                  data-severity={severity}
                  className="flex items-center justify-between gap-2 py-1.5"
                >
                  <span className="min-w-0">
                    <span className="block truncate text-sm">{t.label || t.chip}</span>
                    <span className="block truncate text-muted-foreground text-xs">{t.chip}</span>
                  </span>
                  {history && (
                    <Sparkline
                      label={`${t.label || t.chip} temperature`}
                      points={history[`temp:${sensorKey(t)}`] ?? []}
                      max={t.danger_c}
                      width={64}
                      height={22}
                      color={SEVERITY_COLOR[severity]}
                    />
                  )}
                  <span className="w-16 shrink-0 text-right font-medium text-sm tabular-nums">
                    {t.celsius.toFixed(0)} °C
                    {severity !== 'ok' && (
                      <span className="text-[color:var(--viz-danger)]">
                        {' '}
                        ▲
                        <span className="sr-only">
                          {severity === 'danger' ? ' critical' : ' high'}
                        </span>
                      </span>
                    )}
                  </span>
                </li>
              )
            })}
          </ul>
        </div>
      ))}
    </div>
  )
}

/**
 * The fan rows: a turning fan above 0 RPM, a still one and the word "stopped"
 * at 0 -- a real reading, a fan that has stopped. An empty list is `empty`,
 * which each page words for its own machine: "no fan reported" is not the same
 * sentence on a Talos node as on the host.
 */
export function FanList({ fans, empty }: { fans: Fan[]; empty: ReactNode }) {
  if (fans.length === 0) return <>{empty}</>
  return (
    <ul className="space-y-1">
      {fans.map((f) => (
        <li
          key={`${f.chip}/${f.label}`}
          className="flex items-center justify-between gap-2 text-sm"
        >
          <span className="flex min-w-0 items-center gap-1.5">
            <FanIcon
              aria-hidden="true"
              className={
                f.rpm > 0
                  ? 'size-3.5 shrink-0 motion-safe:animate-spin [animation-duration:2s]'
                  : 'size-3.5 shrink-0 text-muted-foreground'
              }
            />
            <span className="truncate">{f.label}</span>
          </span>
          <span className="tabular-nums">
            {f.rpm > 0 ? `${f.rpm} RPM` : <span className="text-muted-foreground">stopped</span>}
          </span>
        </li>
      ))}
    </ul>
  )
}
