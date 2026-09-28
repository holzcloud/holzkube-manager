import { LiveChart } from '@/components/charts/LiveChart'
import { severityOf } from '@/components/charts/Meter'
import { SEVERITY_COLOR } from '@/components/charts/Sensors'
import { Sparkline } from '@/components/charts/Sparkline'
import type { Series } from '@/hooks/useLiveSeries'
import { formatPercent, formatRate } from '@/lib/format'

/**
 * The chart blocks of a machine's hardware, for a node and for the host
 * (2026-09-28).
 *
 * The node page and /host draw their processor, memory and network history
 * from these components, not from two copies of them, so the two pages cannot
 * drift apart: same titles, same series, same formats, same heights (D-05).
 * `history` is the merged series (the daemon's record followed by the page's
 * own readings); `windowMs` is the range `useChartRange` chose.
 */

interface ChartProps {
  history: Series
  windowMs: number
}

export function ProcessorChart({ history, windowMs }: ChartProps) {
  return (
    <LiveChart
      title="Processor load"
      series={[{ key: 'cpu', label: 'Load', slot: 1, points: history.cpu ?? [] }]}
      format={(v) => `${Math.round(v)}%`}
      yMax={100}
      height={160}
      windowMs={windowMs}
    />
  )
}

/** One row per core: its load now and its sparkline. */
export function CoreList({ perCore, history }: { perCore: number[]; history: Series }) {
  return (
    <ul className="grid grid-cols-2 gap-x-4 gap-y-2 sm:grid-cols-3 xl:grid-cols-6">
      {perCore.map((usage, i) => {
        const severity = severityOf(usage, 75, 90)
        return (
          // biome-ignore lint/suspicious/noArrayIndexKey: the index IS the CPU's name
          <li key={i} className="min-w-0" data-severity={severity}>
            <p className="flex justify-between text-xs">
              <span className="text-muted-foreground">CPU {i}</span>
              <span className="tabular-nums">
                {formatPercent(usage)}
                {severity !== 'ok' && <span className="text-[color:var(--viz-danger)]"> ▲</span>}
              </span>
            </p>
            <Sparkline
              label={`CPU ${i} load`}
              points={history[`core:${i}`] ?? []}
              max={100}
              width={120}
              color={SEVERITY_COLOR[severity]}
            />
          </li>
        )
      })}
    </ul>
  )
}

export function MemoryChart({ history, windowMs }: ChartProps) {
  return (
    <LiveChart
      title="Memory in use"
      series={[{ key: 'memory', label: 'Used', slot: 1, points: history.memory ?? [] }]}
      format={(v) => `${Math.round(v)}%`}
      yMax={100}
      windowMs={windowMs}
    />
  )
}

export function NetworkChart({ history, windowMs }: ChartProps) {
  return (
    <LiveChart
      title="Network throughput"
      series={[
        { key: 'rx', label: 'Inbound', slot: 2, points: history.rx ?? [] },
        { key: 'tx', label: 'Outbound', slot: 1, points: history.tx ?? [] },
      ]}
      format={formatRate}
      windowMs={windowMs}
    />
  )
}
