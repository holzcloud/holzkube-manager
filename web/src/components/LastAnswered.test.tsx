import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { Machine } from '@/api'
import { LastAnswered } from '@/routes/node-detail'

/**
 * "It answered" and "it was read" are different facts, and the difference is a
 * different repair.
 *
 * An observation that connects and then fails its facts read used to record
 * nothing at all, so a node that is present and unreadable was stored exactly
 * like one that is switched off. One sends an operator to the node; the other
 * to the cable. This line is where that distinction reaches them.
 */

function field<T>(value: T) {
  return {
    value,
    level: 'node' as const,
    available: true,
    stale_since: null,
    unavailable_reason: '',
  }
}

const machine: Machine = {
  id: '4c4c4544-0043-4a10-8054-b3c04f565033',
  cluster: 'c-1',
  role: 'controlplane',
  stage: 'degraded',
  lost_addr: false,
  certificate_expired: false,
  locked: false,
  labels: {},
  lock_reason: '',
  unsupported_version: false,
  pre_release: false,
  version_notice: '',
  adopted_at: '2026-09-01T00:00:00Z',
  seen_at: '2026-09-14T06:00:00Z',
  watch: { live: false, since: '', reason: '', restarts: 0 },
  hostname: field('cp-1'),
  addr: field('10.0.0.1'),
  talos_version: field('v1.13.9'),
  kubernetes_version: field('v1.34.0'),
  schematic_id: field(''),
  manufacturer: field(''),
  product_name: field(''),
  serial_number: field(''),
  memory_mib: field(0),
  cpus: field([]),
  disks: field([]),
  interfaces: field([]),
  services: field([]),
  etcd_member: field(true),
  compatibility: field({ known: true, supported: true, headroom_minors: 1, sentence: '' }),
}

describe('when a node last answered', () => {
  it('says so for a node that is not being watched', () => {
    render(<LastAnswered machine={machine} />)

    // The sentence, not just the timestamp. A date on its own does not tell an
    // operator that a reachable machine and a dead one are different problems.
    expect(screen.getByText(/last answered/i)).toBeInTheDocument()
    expect(screen.getByText(/different thing from a machine that is down/i)).toBeInTheDocument()
  })

  it('says nothing about a healthy node', () => {
    // A line reading "last answered four seconds ago" on every page is a line
    // nobody reads on the page where it matters.
    const { container } = render(<LastAnswered machine={{ ...machine, stage: 'watching' }} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('says nothing about a node that has never answered', () => {
    // seen_at absent is a machine adopted and never observed. "Last answered:
    // never" is noise; the stage already says it.
    const { container } = render(<LastAnswered machine={{ ...machine, seen_at: undefined }} />)
    expect(container).toBeEmptyDOMElement()
  })
})
