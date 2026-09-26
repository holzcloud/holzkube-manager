import { Button } from '@/components/ui/button'
import { CHART_RANGES, type ChartRange } from '@/hooks/useChartRange'

/**
 * The time range for every chart on the page, in one row above them -- one
 * control for all of them, so the charts always show the same stretch of time
 * and never disagree about "now".
 */
export function RangePicker({
  value,
  onChange,
  note,
}: {
  value: ChartRange
  onChange: (range: ChartRange) => void
  /** A line beside the buttons: how far back the record goes, or why it is short. */
  note?: string
}) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <fieldset className="m-0 flex gap-1 border-0 p-0">
        <legend className="sr-only">Time range</legend>
        {CHART_RANGES.map((r) => (
          <Button
            key={r.value}
            size="sm"
            variant={value === r.value ? 'secondary' : 'ghost'}
            aria-pressed={value === r.value}
            onClick={() => onChange(r.value)}
          >
            {r.label}
          </Button>
        ))}
      </fieldset>
      {note && <span className="text-muted-foreground text-xs">{note}</span>}
    </div>
  )
}
