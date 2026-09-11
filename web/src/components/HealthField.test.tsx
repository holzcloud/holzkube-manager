import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { Field } from '@/api'
import { ago, HealthField, StageBadge } from '@/components/HealthField'

/**
 * The three states of a fact, on screen (D-14).
 *
 * The middle one is the whole test. A stale value that is hidden looks exactly
 * like a value that does not exist, and that is how the empty dashboard INV-08
 * forbids comes about -- during the outage, on the screen that exists for it.
 */

function field<T>(over: Partial<Field<T>>): Field<T> {
  return { level: 'node', available: false, unavailable_reason: '', ...over } as Field<T>
}

describe('HealthField', () => {
  it('shows a confirmed value plainly', () => {
    render(<HealthField field={field({ value: 'v1.13.9', available: true })} />)
    expect(screen.getByText('v1.13.9')).toBeInTheDocument()
  })

  it('keeps a stale value and says how old it is', () => {
    const since = new Date(Date.now() - 4 * 60 * 1000).toISOString()
    render(
      <HealthField
        field={field({
          value: 'v1.13.9',
          stale_since: since,
          unavailable_reason: 'the node could not be reached',
        })}
      />,
    )

    // The value survives. Dropping it would be indistinguishable from null.
    expect(screen.getByText('v1.13.9')).toBeInTheDocument()
    expect(screen.getByText(/as of 4m ago/)).toBeInTheDocument()
  })

  it('shows the reason instead of a bare dash when nothing was ever read', () => {
    render(
      <HealthField
        field={field({
          value: '',
          unavailable_reason: 'the node reports no kubelet',
        })}
      />,
    )
    expect(screen.getByText('the node reports no kubelet')).toBeInTheDocument()
    expect(screen.getByText('not known')).toBeInTheDocument()
  })

  it('treats an empty array as nothing to show rather than as a value', () => {
    render(
      <HealthField
        field={field<string[]>({
          value: [],
          available: true,
          unavailable_reason: 'the node has not reported its disks',
        })}
      />,
    )
    expect(screen.getByText('the node has not reported its disks')).toBeInTheDocument()
  })
})

describe('ago', () => {
  it('rounds to the largest unit that is still unambiguous', () => {
    const now = new Date('2026-09-11T12:00:00Z')
    expect(ago('2026-09-11T11:59:30Z', now)).toBe('30s ago')
    expect(ago('2026-09-11T11:55:00Z', now)).toBe('5m ago')
    expect(ago('2026-09-11T09:00:00Z', now)).toBe('3h ago')
    expect(ago('2026-09-08T12:00:00Z', now)).toBe('3d ago')
  })

  it('does not render a negative age for a clock that ran ahead', () => {
    const now = new Date('2026-09-11T12:00:00Z')
    expect(ago('2026-09-11T12:00:30Z', now)).toBe('0s ago')
  })
})

describe('StageBadge', () => {
  it('distinguishes degraded from down', () => {
    const { rerender } = render(<StageBadge stage="degraded" />)
    expect(screen.getByText('degraded')).toBeInTheDocument()

    rerender(<StageBadge stage="down" />)
    expect(screen.getByText('down')).toBeInTheDocument()
  })

  it('calls the watching stage healthy, because that is what it means', () => {
    render(<StageBadge stage="watching" />)
    expect(screen.getByText('healthy')).toBeInTheDocument()
  })
})
