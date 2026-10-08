import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, type Job, type Machine } from '@/api'
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
  labels: {},
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

    // The system disk is wiped and nothing else: somebody expecting a full
    // reset must be told the data disks stay, and somebody expecting no wipe at
    // all must be told the system disk goes.
    expect(screen.getByText(/system disk wiped/i)).toBeInTheDocument()
    expect(screen.getByText(/this is not a full reset/i)).toBeInTheDocument()
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

/**
 * The removal is a job, and the dialog follows it.
 *
 * What these hold is that the dialog does not say "removed" when the request
 * returns — the request only accepted the job — and that each way a job can end
 * is said in words rather than left as a spinner that stops.
 */
const step = (name: string, state: Job['steps'][number]['state'], detail = '') => ({
  name,
  state,
  verifiable: true,
  detail,
})

function jobWith(state: Job['state'], steps: Job['steps'], extra: Partial<Job> = {}): Job {
  return {
    id: 'job-1',
    kind: 'node.remove-from-cluster',
    cluster: 'c-1',
    machine: machine.id,
    state,
    steps,
    current: 0,
    params: {},
    cancel_requested: false,
    actor: 'op',
    parked_reason: '',
    created_at: '2026-10-01T00:00:00Z',
    rev: 1,
    ...extra,
  } as Job
}

function typeAndSubmit() {
  fireEvent.click(screen.getByRole('button', { name: /remove from cluster/i }))
  fireEvent.change(screen.getByLabelText(/Type cp-1 to confirm/i), { target: { value: 'cp-1' } })
  fireEvent.click(screen.getAllByRole('button', { name: /remove from cluster/i }).at(-1) as Element)
}

async function submitRemoval(job: Job, notice = 'Nothing was drained.') {
  vi.spyOn(api.machines, 'confirm').mockResolvedValue({ token: 'tok', expires: '' })
  const remove = vi.spyOn(api.machines, 'removeFromCluster').mockResolvedValue({
    job,
    topic: 'job:job-1',
    notice,
  })
  const onRemoved = vi.fn()
  wrap(<NodeActions machine={machine} onRemoved={onRemoved} />)
  typeAndSubmit()
  await screen.findByRole('list', { name: 'Removal steps' }, { timeout: 5000 })
  return { remove, onRemoved }
}

const allDone = [
  'check the node answers',
  'leave etcd',
  'let etcd settle',
  'wipe the node',
  'forget the node',
].map((n) => step(n, 'done'))

describe('following the removal job', () => {
  afterEach(() => vi.restoreAllMocks())

  it('does not claim the node is removed while the job is still running', async () => {
    const running = jobWith('running', [
      step('check the node answers', 'done'),
      step('leave etcd', 'running'),
      step('let etcd settle', 'pending'),
      step('wipe the node', 'pending'),
      step('forget the node', 'pending'),
    ])
    vi.spyOn(api.jobs, 'get').mockResolvedValue(running)
    const { remove } = await submitRemoval(running)

    expect(remove).toHaveBeenCalledWith(machine.id, 'c-1', 'tok')
    expect(screen.getByText('leave etcd')).toBeInTheDocument()
    expect(screen.queryByText(/has been removed/i)).not.toBeInTheDocument()
    // The notice about cordon and drain is on screen while it runs, not only
    // after.
    expect(screen.getByText('Nothing was drained.')).toBeInTheDocument()
  })

  it('says it is removed once the job has succeeded, and only then lets the caller leave', async () => {
    vi.spyOn(api.jobs, 'get').mockResolvedValue(jobWith('succeeded', allDone))
    const { onRemoved } = await submitRemoval(jobWith('running', allDone))

    expect(await screen.findByText(/cp-1 has been removed/i)).toBeInTheDocument()
    expect(onRemoved).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Done' }))
    expect(onRemoved).toHaveBeenCalledTimes(1)
  })

  it('keeps polling until the job is over', async () => {
    const running = jobWith('running', [step('leave etcd', 'running')])
    const finished = jobWith('succeeded', [step('leave etcd', 'done')])
    const get = vi.spyOn(api.jobs, 'get').mockResolvedValueOnce(running).mockResolvedValue(finished)
    await submitRemoval(running)

    expect(
      await screen.findByText(/cp-1 has been removed/i, {}, { timeout: 6000 }),
    ).toBeInTheDocument()
    expect(get.mock.calls.length).toBeGreaterThanOrEqual(2)
  }, 10000)

  it('says what a failed job left behind, and does not navigate away', async () => {
    const failed = jobWith('failed', [
      step('check the node answers', 'done'),
      step('leave etcd', 'failed', 'only 1 of 3 voting members answer'),
    ])
    vi.spyOn(api.jobs, 'get').mockResolvedValue(failed)
    const { onRemoved } = await submitRemoval(failed)

    expect(await screen.findByText(/stopped at/i)).toHaveTextContent(/nothing was wiped/i)
    expect(screen.getByText('only 1 of 3 voting members answer')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Done' }))
    // The record is still there: leaving the node's page would be wrong.
    expect(onRemoved).not.toHaveBeenCalled()
  })

  it('hands a parked job to the operator with the reason', async () => {
    const parked = jobWith(
      'parked',
      [step('let etcd settle', 'done'), step('wipe the node', 'running')],
      { parked_reason: 'it may or may not have been wiped' },
    )
    vi.spyOn(api.jobs, 'get').mockResolvedValue(parked)
    const { onRemoved } = await submitRemoval(parked)

    await waitFor(() => expect(screen.getByText(/needs you to decide/i)).toBeInTheDocument())
    expect(screen.getByText(/may or may not have been wiped/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Done' }))
    expect(onRemoved).not.toHaveBeenCalled()
  })

  it('shows the quorum refusal as the error it is, before any job exists', async () => {
    vi.spyOn(api.machines, 'confirm').mockResolvedValue({ token: 'tok', expires: '' })
    vi.spyOn(api.machines, 'removeFromCluster').mockRejectedValue(
      new Error('only one voting member: removing it would end the cluster'),
    )
    wrap(<NodeActions machine={machine} />)
    typeAndSubmit()

    expect(await screen.findByText(/would end the cluster/i)).toBeInTheDocument()
    expect(screen.queryByRole('list', { name: 'Removal steps' })).not.toBeInTheDocument()
  })
})
