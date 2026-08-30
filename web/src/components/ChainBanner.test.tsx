import { render, screen, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { AuditChain } from '@/api'
import { ChainBanner } from './ChainBanner'

/**
 * The executed proof for D-15 and the mitigation of threat T-01-34.
 *
 * Without this file, "the banner cannot be dismissed" would be a statement of
 * intent that any later refactor could quietly break -- someone adds a close
 * button because banners usually have one, and the property nobody tested is
 * gone. So the test counts the interactive elements the component renders and
 * expects none of them.
 */

const BROKEN: AuditChain = {
  ok: false,
  broken_at_line: 3,
  file: '/var/lib/holzkube-manager/audit/audit-2026-08-28.jsonl',
}

const INTACT: AuditChain = {
  ok: true,
  broken_at_line: 0,
  file: '/var/lib/holzkube-manager/audit/audit-2026-08-28.jsonl',
}

describe('ChainBanner', () => {
  it('renders nothing at all while the chain verifies', () => {
    const { container } = render(<ChainBanner chain={INTACT} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('names the affected file and the line number when the chain is broken', () => {
    render(<ChainBanner chain={BROKEN} />)

    const banner = screen.getByRole('alert')
    expect(banner).toHaveTextContent('line 3')
    expect(banner).toHaveTextContent('/var/lib/holzkube-manager/audit/audit-2026-08-28.jsonl')
  })

  it('says what the finding means and what it does not mean', () => {
    render(<ChainBanner chain={BROKEN} />)

    const banner = screen.getByRole('alert')
    expect(banner).toHaveTextContent('changed since it was written')
    expect(banner).toHaveTextContent('does not mean the cluster is compromised')
  })

  it('renders no interactive element of any kind', () => {
    const { container } = render(<ChainBanner chain={BROKEN} />)
    const banner = within(container)

    expect(banner.queryAllByRole('button')).toHaveLength(0)
    expect(banner.queryAllByRole('link')).toHaveLength(0)
    expect(banner.queryAllByRole('checkbox')).toHaveLength(0)
    expect(banner.queryAllByRole('switch')).toHaveLength(0)
    // And nothing focusable that the role queries would miss.
    expect(
      container.querySelectorAll('button, a, input, select, textarea, [tabindex], [onclick]'),
    ).toHaveLength(0)
  })

  it('carries no accessible name suggesting a way to close or acknowledge it', () => {
    const { container } = render(<ChainBanner chain={BROKEN} />)
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

  it('stays put when re-rendered with the same verdict', () => {
    const view = render(<ChainBanner chain={BROKEN} />)
    expect(screen.getByRole('alert')).toBeInTheDocument()

    view.rerender(<ChainBanner chain={BROKEN} />)
    view.rerender(<ChainBanner chain={BROKEN} />)

    expect(screen.getByRole('alert')).toBeInTheDocument()
    expect(screen.getByRole('alert')).toHaveTextContent('line 3')
  })

  it('disappears only when the verdict itself turns clean', () => {
    const view = render(<ChainBanner chain={BROKEN} />)
    expect(screen.getByRole('alert')).toBeInTheDocument()

    view.rerender(<ChainBanner chain={INTACT} />)

    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })
})
