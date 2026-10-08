import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { LogPanel } from '@/components/LogPanel'

/** Two entries with one id must both render and must not make React warn. */
describe('LogPanel', () => {
  it('is a log region, named after its topic, so appended lines are announced politely', () => {
    render(<LogPanel title="kubelet" connection="open" lines={[]} />)
    expect(screen.getByRole('log', { name: 'kubelet log' })).toBeInTheDocument()
  })
})

describe('LogPanel keys', () => {
  afterEach(() => vi.restoreAllMocks())

  it('renders entries that share an id without a duplicate-key warning', () => {
    const error = vi.spyOn(console, 'error').mockImplementation(() => {})
    const at = '2026-09-11T12:00:00Z'
    render(
      <LogPanel
        title="kubelet"
        connection="open"
        lines={[
          { id: 0, line: 'first', at },
          { id: 0, line: 'second', at },
        ]}
      />,
    )
    expect(screen.getByText('first')).toBeInTheDocument()
    expect(screen.getByText('second')).toBeInTheDocument()
    const warned = error.mock.calls.some((c) => String(c[0]).includes('same key'))
    expect(warned).toBe(false)
  })
})
