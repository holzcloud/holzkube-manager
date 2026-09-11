import { useEffect, useRef } from 'react'
import type { StreamLine, StreamState } from '@/hooks/useStream'
import { cn } from '@/lib/utils'

/**
 * One stream, rendered.
 *
 * Two things here are not decoration.
 *
 * A gap is drawn as a rule across the panel with the count in it, because the
 * alternative — leaving it out — is indistinguishable from the log simply not
 * having had those lines. An operator diagnosing an outage from a log with an
 * invisible hole in it reaches a confident wrong conclusion.
 *
 * The state line says which of the four causes of silence this is. "Nothing is
 * arriving" is not one condition: the service may be quiet, we may be
 * reconnecting, the node may be rebooting, or it may be gone, and each calls
 * for something different from the person reading.
 */

const STATE_TEXT: Record<StreamState, { label: string; className: string }> = {
  live: { label: 'live', className: 'text-emerald-700 dark:text-emerald-300' },
  reconnecting: { label: 'reconnecting', className: 'text-amber-700 dark:text-amber-300' },
  rebooting: { label: 'node rebooting', className: 'text-amber-700 dark:text-amber-300' },
  disconnected: { label: 'disconnected', className: 'text-red-700 dark:text-red-300' },
}

export interface LogPanelProps {
  title: string
  lines: StreamLine[]
  /** The connection, as opposed to this topic's state. */
  connection: 'connecting' | 'open' | 'closed'
  className?: string
}

export function LogPanel({ title, lines, connection, className }: LogPanelProps) {
  const box = useRef<HTMLDivElement>(null)
  const pinned = useRef(true)

  // Follow the tail, but only while the reader has not scrolled up. Yanking
  // somebody back to the bottom while they are reading something is worse than
  // not following at all.
  //
  // The dependency is the line *count* rather than the array: the effect only
  // has to run when something was appended, and the array identity changes on
  // every state update whether or not it gained anything.
  const count = lines.length
  // biome-ignore lint/correctness/useExhaustiveDependencies: the effect body reads only refs, so the analysis concludes it needs no dependencies -- but running it when a line arrives is the entire purpose. Removing `count` leaves a panel that never follows the tail.
  useEffect(() => {
    const el = box.current
    if (el && pinned.current) {
      el.scrollTop = el.scrollHeight
    }
  }, [count])

  const latestState = [...lines].reverse().find((l) => l.state)?.state
  const stateReason = [...lines].reverse().find((l) => l.state)?.reason

  return (
    <div className={cn('flex flex-col rounded-md border border-border', className)}>
      <div className="flex items-center justify-between gap-2 border-b border-border px-3 py-1.5">
        <span className="font-mono text-xs font-medium">{title}</span>
        <span className="flex items-center gap-2 text-xs">
          {latestState && (
            <span className={STATE_TEXT[latestState].className} title={stateReason}>
              {STATE_TEXT[latestState].label}
            </span>
          )}
          {connection !== 'open' && <span className="text-muted-foreground">({connection})</span>}
        </span>
      </div>

      <div
        ref={box}
        onScroll={(e) => {
          const el = e.currentTarget
          pinned.current = el.scrollHeight - el.scrollTop - el.clientHeight < 32
        }}
        className="h-64 overflow-auto bg-muted/30 p-2 font-mono text-xs leading-relaxed"
      >
        {lines.length === 0 && (
          <p className="text-muted-foreground">
            Nothing yet. The panel fills as the node sends output.
          </p>
        )}

        {lines.map((entry) => {
          if (entry.gap) {
            return (
              <div
                key={entry.id}
                className="my-1 flex items-center gap-2 text-amber-700 dark:text-amber-300"
                title="Your browser could not keep up and these lines were dropped rather than queued. They are not missing from the node."
              >
                <span className="h-px flex-1 bg-current opacity-40" />
                <span>{entry.gap.missed} lines dropped</span>
                <span className="h-px flex-1 bg-current opacity-40" />
              </div>
            )
          }
          if (entry.state) {
            return (
              <div key={entry.id} className={cn('my-0.5', STATE_TEXT[entry.state].className)}>
                — {STATE_TEXT[entry.state].label}
                {entry.reason ? `: ${entry.reason}` : ''} —
              </div>
            )
          }
          return (
            <div key={entry.id} className="whitespace-pre-wrap break-all">
              {entry.line}
            </div>
          )
        })}
      </div>
    </div>
  )
}
