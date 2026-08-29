import { render, screen, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { AuditChain } from '@/api'
import { ChainBanner } from './ChainBanner'
import { DryRunBanner } from './DryRunBanner'

/**
 * The executed proof for FOUND-12's visibility half and the mitigation of
 * threat T-02-44.
 *
 * The argument is ChainBanner.test.tsx's, one degree sharper. "The indicator
 * cannot be dismissed" is a statement of intent until something counts the
 * interactive elements and finds none; and an operator who is mistaken about
 * which mode the process is in is worse off than one with no indicator at all,
 * because they will act on the belief. So this file also asserts that rendering
 * writes nothing a later render could read back as "seen".
 */

const BROKEN: AuditChain = {
  ok: false,
  broken_at_line: 3,
  file: 'audit-2026-08-29.jsonl',
}

describe('DryRunBanner', () => {
  it('renders nothing at all while the process is live', () => {
    const { container } = render(<DryRunBanner dryRun={false} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('announces the mode when dry-run is on', () => {
    render(<DryRunBanner dryRun={true} />)

    const banner = screen.getByRole('status')
    expect(banner).toHaveTextContent('Dry-run mode')
    expect(banner).toHaveTextContent('--dry-run')
  })

  it('says the refusal happens in the transport, not in the interface', () => {
    render(<DryRunBanner dryRun={true} />)

    const banner = screen.getByRole('status')
    expect(banner).toHaveTextContent('refused in the transport before it reaches a node')
    expect(banner).toHaveTextContent('Reading is unaffected')
    expect(banner).toHaveTextContent('Restart without the flag')
  })

  it('is a mode and not a fault: polite status, never an assertive alert', () => {
    render(<DryRunBanner dryRun={true} />)

    const banner = screen.getByRole('status')
    expect(banner).toHaveAttribute('aria-live', 'polite')
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('renders no interactive element of any kind', () => {
    const { container } = render(<DryRunBanner dryRun={true} />)
    const banner = within(container)

    expect(banner.queryAllByRole('button')).toHaveLength(0)
    expect(banner.queryAllByRole('link')).toHaveLength(0)
    expect(banner.queryAllByRole('checkbox')).toHaveLength(0)
    expect(banner.queryAllByRole('switch')).toHaveLength(0)
    expect(
      container.querySelectorAll('button, a, input, select, textarea, [tabindex], [onclick]'),
    ).toHaveLength(0)
  })

  it('carries no accessible name suggesting a way to close or acknowledge it', () => {
    const { container } = render(<DryRunBanner dryRun={true} />)
    const banner = within(container)

    for (const name of [
      /close/i,
      /dismiss/i,
      /acknowledge/i,
      /got it/i,
      /understood/i,
      /hide/i,
      /ok/i,
      /^x$/i,
    ]) {
      expect(banner.queryAllByRole('button', { name })).toHaveLength(0)
      expect(banner.queryAllByLabelText(name)).toHaveLength(0)
      expect(banner.queryAllByTitle(name)).toHaveLength(0)
    }
  })

  it('writes nothing to browser storage that a later render could read back', () => {
    localStorage.clear()
    sessionStorage.clear()

    const view = render(<DryRunBanner dryRun={true} />)
    view.rerender(<DryRunBanner dryRun={true} />)
    view.rerender(<DryRunBanner dryRun={true} />)

    expect(localStorage.length).toBe(0)
    expect(sessionStorage.length).toBe(0)
    expect(document.cookie).toBe('')

    // And it is still there after all of that: no render counted as a
    // dismissal.
    expect(screen.getByRole('status')).toBeInTheDocument()
  })

  it('disappears only when the mode itself turns off', () => {
    const view = render(<DryRunBanner dryRun={true} />)
    expect(screen.getByRole('status')).toBeInTheDocument()

    view.rerender(<DryRunBanner dryRun={false} />)

    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })

  it('is distinguishable from the chain-break banner when both apply', () => {
    render(
      <>
        <ChainBanner chain={BROKEN} />
        <DryRunBanner dryRun={true} />
      </>,
    )

    const chain = screen.getByRole('alert')
    const dryRun = screen.getByRole('status')

    expect(chain).toBeInTheDocument()
    expect(dryRun).toBeInTheDocument()
    expect(chain).not.toBe(dryRun)

    // A fault and a chosen mode, told apart by role rather than by reading
    // them: two banners that announced themselves identically would read as
    // one, and the operator would take the mode for part of the failure.
    expect(chain).toHaveAttribute('aria-live', 'assertive')
    expect(dryRun).toHaveAttribute('aria-live', 'polite')
    expect(chain).toHaveTextContent('audit log no longer verifies')
    expect(dryRun).toHaveTextContent('Dry-run mode')
  })
})
