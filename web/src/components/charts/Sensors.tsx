import { Fan as FanIcon } from 'lucide-react'
import type { ReactNode } from 'react'
import type { Fan, Temperature } from '@/api'
import { severityOf } from '@/components/charts/Meter'
import { Sparkline } from '@/components/charts/Sparkline'

/**
 * A machine's temperatures and fans, drawn the same way wherever they appear
 * (2026-09-28): a node's hardware page and the host page carry the same JSON
 * for a sensor, and one set of rules decides where it turns amber and red.
 * Moved here out of NodeHardware, not copied, so the two pages cannot drift.
 */

/**
 * Where a temperature turns amber and where it turns red, when the chip does
 * not say.
 *
 * The chip's own `high` and `crit` win whenever it reports them: Intel's
 * TjMax, an NVMe drive's warning composite temperature, are the manufacturer's
 * numbers and better than any default. These are for chips that report only a
 * reading, and they are the conventional ones -- a desktop CPU at 80 °C is
 * working hard, a drive at 60 °C is past where its life shortens.
 */
export const TEMPERATURE_DEFAULTS = {
  cpu: { warn: 80, danger: 95 },
  disk: { warn: 60, danger: 70 },
  board: { warn: 70, danger: 85 },
  gpu: { warn: 80, danger: 95 },
  other: { warn: 75, danger: 90 },
} as const satisfies Record<string, { warn: number; danger: number }>

export function temperatureLimits(t: Temperature): { warn: number; danger: number } {
  const fallback: { warn: number; danger: number } =
    TEMPERATURE_DEFAULTS[t.kind as keyof typeof TEMPERATURE_DEFAULTS] ?? TEMPERATURE_DEFAULTS.other
  const danger = t.critical_c ?? fallback.danger
  const warn = t.high_c !== null && t.high_c < danger ? t.high_c : Math.min(fallback.warn, danger)
  return { warn, danger }
}

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
  history?: Record<string, { t: number; v: number }[]>
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
              const limits = temperatureLimits(t)
              const severity = severityOf(t.celsius, limits.warn, limits.danger)
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
                      max={limits.danger}
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
