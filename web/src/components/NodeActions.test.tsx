import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { describe, expect, it } from 'vitest'
import type { Machine } from '@/api'
import { NodeActions } from '@/components/NodeActions'

/**
 * Removing a node from its cluster (UPG-13).
 *
 * The route shipped in phase 9 with nothing to click, which made the whole of
 * that phase's etcd work reachable only with curl — the same gap the password
 * form had, and the support bundle after it. What these tests hold is the two
 * things the dialog says that nothing else does: that this product cannot
 * cordon or drain, and that a removal is not a reset.
 */

function wrap(node: ReactNode) {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
  return render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
}

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
  stage: 'watching',
  lost_addr: false,
  certificate_expired: false,
  locked: false,
  lock_reason: '',
  unsupported_version: false,
  pre_release: false,
  version_notice: '',
  adopted_at: '2026-09-01T00:00:00Z',
  watch: { live: true, since: '', reason: '', restarts: 0 },
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

describe('removing a node from its cluster', () => {
  it('is offered at all, which it was not for two milestones', () => {
    wrap(<NodeActions machine={machine} />)
    expect(screen.getByRole('button', { name: /remove from cluster/i })).toBeInTheDocument()
  })

  it('is not offered for a machine that belongs to no cluster', () => {
    // A node in maintenance mode has nothing to be removed from, and a button
    // that answers "this machine belongs to no cluster" teaches the operator
    // to ignore what buttons say.
    wrap(<NodeActions machine={{ ...machine, cluster: '' }} />)
    expect(screen.queryByRole('button', { name: /remove from cluster/i })).not.toBeInTheDocument()
  })

  it('says that nothing is cordoned or drained, before anything happens', () => {
    wrap(<NodeActions machine={machine} />)
    fireEvent.click(screen.getByRole('button', { name: /remove from cluster/i }))

    // The single most expensive misunderstanding available on this screen: an
    // operator who expects the workloads to move first is an operator whose
    // pods get killed.
    expect(screen.getByText(/cannot\s+cordon or drain/i)).toBeInTheDocument()
    expect(screen.getByText(/kubectl drain/)).toBeInTheDocument()
  })

  it('says that a removal is not a wipe', () => {
    wrap(<NodeActions machine={machine} />)
    fireEvent.click(screen.getByRole('button', { name: /remove from cluster/i }))

    // Somebody expecting a reset would leave a machine on the network still
    // holding the cluster's secrets.
    expect(screen.getByText(/this is not a reset/i)).toBeInTheDocument()
  })

  it('warns when the node is an etcd member, because the quorum moves', () => {
    wrap(<NodeActions machine={machine} />)
    fireEvent.click(screen.getByRole('button', { name: /remove from cluster/i }))

    expect(screen.getByText(/lowers the cluster's voting count/i)).toBeInTheDocument()
  })

  it('refuses to submit until the hostname is typed exactly', () => {
    wrap(<NodeActions machine={machine} />)
    fireEvent.click(screen.getByRole('button', { name: /remove from cluster/i }))

    const submit = screen.getAllByRole('button', { name: /remove from cluster/i }).at(-1)
    expect(submit).toBeDisabled()

    fireEvent.change(screen.getByLabelText(/Type cp-1 to confirm/i), { target: { value: 'cp-' } })
    expect(submit).toBeDisabled()

    fireEvent.change(screen.getByLabelText(/Type cp-1 to confirm/i), { target: { value: 'cp-1' } })
    expect(submit).toBeEnabled()
  })

  it('asks for the machine id when the node has never reported a hostname', () => {
    // Still specific to this machine. A generic word would confirm only that
    // somebody can type.
    wrap(<NodeActions machine={{ ...machine, hostname: field('') }} />)
    fireEvent.click(screen.getByRole('button', { name: /remove from cluster/i }))

    expect(
      screen.getByLabelText(new RegExp(`Type ${machine.id} to confirm`, 'i')),
    ).toBeInTheDocument()
  })
})

/**
 * What happens after the removal succeeds.
 *
 * The record is gone by then, so the screen this component sits on is about a
 * machine that no longer exists. Navigating away the instant the call returns
 * would be the obvious fix and the wrong one: the dialog is the only place the
 * cordon-and-drain notice appears, and taking it off the screen before it is
 * read is how an operator finds out about their pods afterwards.
 */
describe('after a node has been removed', () => {
  it('does not leave the caller on the page until the operator closes the dialog', () => {
    let left = 0
    wrap(
      <NodeActions
        machine={machine}
        onRemoved={() => {
          left += 1
        }}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: /remove from cluster/i }))
    expect(left).toBe(0)

    // Cancelling is not a removal, so it must not navigate either.
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(left).toBe(0)
  })
})
