import type { ReactNode } from 'react'
import type { Field, Stage } from '@/api'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

/**
 * Rendering a fact honestly (D-14, D-15).
 *
 * Three states, three different things on screen, and the middle one is the
 * one that is easy to get wrong:
 *
 * - confirmed: the value, plainly.
 * - stale: the value, **still there**, muted, with "as of 4m ago". Dropping
 *   it would be indistinguishable from null, and that is exactly how the empty
 *   dashboard INV-08 forbids comes about.
 * - never read: an em dash and the reason. A bare dash sends an operator
 *   looking for a broken node when the answer is that nothing ever asked.
 *
 * Staleness is muted, never alarming. Alarm colour belongs to the connection
 * state (the stage badge below); a value being four minutes old is information,
 * not an emergency, and a screen that shouts about both has no way left to
 * distinguish them.
 */

/** How long ago, in the shortest form that is still unambiguous. */
export function ago(since: string, now: Date = new Date()): string {
  const then = new Date(since)
  if (Number.isNaN(then.getTime())) {
    return 'unknown'
  }
  const seconds = Math.max(0, Math.round((now.getTime() - then.getTime()) / 1000))
  if (seconds < 60) {
    return `${seconds}s ago`
  }
  const minutes = Math.round(seconds / 60)
  if (minutes < 60) {
    return `${minutes}m ago`
  }
  const hours = Math.round(minutes / 60)
  if (hours < 48) {
    return `${hours}h ago`
  }
  return `${Math.round(hours / 24)}d ago`
}

/** True when the field has a value worth rendering, stale or not. */
function hasValue<T>(field: Field<T>): boolean {
  if (field.value === undefined || field.value === null) {
    return false
  }
  if (Array.isArray(field.value)) {
    return field.value.length > 0
  }
  if (typeof field.value === 'string') {
    return field.value !== ''
  }
  return true
}

export interface HealthFieldProps<T> {
  field: Field<T>
  /** How to draw the value itself. Defaults to String(). */
  render?: (value: T) => ReactNode
  className?: string
}

export function HealthField<T>({ field, render, className }: HealthFieldProps<T>) {
  const draw = render ?? ((v: T) => String(v))

  if (field.available && hasValue(field)) {
    return <span className={className}>{draw(field.value as T)}</span>
  }

  // Stale: the value survives, muted, with its age. This is the branch that
  // keeps a dead cluster's dashboard readable.
  if (field.stale_since && hasValue(field)) {
    return (
      <span
        className={cn('text-muted-foreground', className)}
        title={field.unavailable_reason || undefined}
      >
        {draw(field.value as T)}
        <span className="ml-1.5 text-xs italic">as of {ago(field.stale_since)}</span>
      </span>
    )
  }

  // Never read, or read and genuinely empty. Either way there is nothing to
  // show but the reason, and the reason is the useful part.
  return (
    <span className={cn('text-muted-foreground', className)}>
      <span aria-hidden="true">—</span>
      <span className="sr-only">not known</span>
      {field.unavailable_reason !== '' && (
        <span className="ml-1.5 text-xs italic" title={field.unavailable_reason}>
          {field.unavailable_reason}
        </span>
      )}
    </span>
  )
}

/**
 * The observer's state, as the one thing on the row that may use alarm colour.
 *
 * `degraded` and `down` are deliberately different: a degraded node is
 * answering with one of its layers gone, a down node is answering nothing. An
 * operator who cannot tell them apart starts the wrong repair.
 */
const STAGE_PRESENTATION: Record<Stage, { label: string; className: string; title: string }> = {
  unknown: {
    label: 'unknown',
    className: 'border-border bg-muted text-muted-foreground',
    title: 'Nothing has looked at this machine yet.',
  },
  connecting: {
    label: 'connecting',
    className: 'border-border bg-muted text-muted-foreground',
    title: 'A connection to this machine is being established.',
  },
  watching: {
    label: 'healthy',
    className: 'border-emerald-600/40 bg-emerald-600/15 text-emerald-700 dark:text-emerald-300',
    title: 'This machine is answering and every layer it was asked about answered too.',
  },
  degraded: {
    label: 'degraded',
    className: 'border-amber-600/40 bg-amber-600/15 text-amber-700 dark:text-amber-300',
    title: 'This machine is answering, but one of the layers above it is not.',
  },
  down: {
    label: 'down',
    className: 'border-red-600/40 bg-red-600/15 text-red-700 dark:text-red-300',
    title: 'This machine is not answering. Its last known facts are still shown, marked stale.',
  },
}

export function StageBadge({ stage, className }: { stage: Stage; className?: string }) {
  const p = STAGE_PRESENTATION[stage]
  return (
    <Badge variant="outline" className={cn(p.className, className)} title={p.title}>
      {p.label}
    </Badge>
  )
}
