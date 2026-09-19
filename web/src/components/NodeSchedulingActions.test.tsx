import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from '@/api'
import { NodeSchedulingActions } from '@/components/NodeSchedulingActions'

/**
 * Cordon, uncordon and drain.
 *
 * The panel's whole design is that the two drain flags are OFF until somebody
 * says otherwise: a pod nothing owns is gone for good once it is evicted, and a
 * pod with local storage loses that storage when it moves. kubectl refuses both
 * without being told and so does this product, so these tests pin that the
 * screen sends what was ticked rather than a convenient default.
 */

function wrap(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
}

const acceptedJob = {
  job: {
    id: 'job-1',
    kind: 'node.drain',
    state: 'pending',
    current: 0,
    steps: [{ name: 'drain cp-1', state: 'pending', verifiable: true }],
  },
  topic: 'jobs/job-1',
} as Awaited<ReturnType<typeof api.kubernetes.drain>>

afterEach(() => {
  vi.restoreAllMocks()
})

describe('a node’s scheduling actions', () => {
  it('sends the state it wants rather than a toggle, so a retry is the same request', async () => {
    const cordon = vi.spyOn(api.kubernetes, 'cordon').mockResolvedValue({
      name: 'cp-1',
      ready: 'True',
      unschedulable: true,
      roles: [],
      kubelet_version: '',
      os_image: '',
      created_at: '',
      internal_address: '',
      container_runtime: '',
    })

    wrap(<NodeSchedulingActions clusterID="c-1" node="cp-1" unschedulable={false} />)
    await userEvent.click(screen.getByRole('button', { name: 'Cordon' }))

    expect(cordon).toHaveBeenCalledWith('c-1', 'cp-1', true)
  })

  it('offers to uncordon a node that is already cordoned', async () => {
    const cordon = vi.spyOn(api.kubernetes, 'cordon').mockResolvedValue({
      name: 'cp-1',
      ready: 'True',
      unschedulable: false,
      roles: [],
      kubelet_version: '',
      os_image: '',
      created_at: '',
      internal_address: '',
      container_runtime: '',
    })

    wrap(<NodeSchedulingActions clusterID="c-1" node="cp-1" unschedulable={true} />)
    await userEvent.click(screen.getByRole('button', { name: 'Uncordon' }))

    expect(cordon).toHaveBeenCalledWith('c-1', 'cp-1', false)
  })

  it('drains with both decisions refused unless they were ticked', async () => {
    const drain = vi.spyOn(api.kubernetes, 'drain').mockResolvedValue(acceptedJob)

    wrap(<NodeSchedulingActions clusterID="c-1" node="cp-1" unschedulable={false} />)
    await userEvent.click(screen.getByRole('button', { name: 'Drain…' }))
    await userEvent.click(screen.getByRole('button', { name: /drain this node/i }))

    expect(drain).toHaveBeenCalledWith('c-1', 'cp-1', { force: false, delete_local_data: false })
  })

  it('sends what was ticked, and says what each tick costs', async () => {
    const drain = vi.spyOn(api.kubernetes, 'drain').mockResolvedValue(acceptedJob)

    wrap(<NodeSchedulingActions clusterID="c-1" node="cp-1" unschedulable={false} />)
    await userEvent.click(screen.getByRole('button', { name: 'Drain…' }))

    // The consequence is on the screen beside the box, not in a tooltip: a
    // phone has no hover, and this is the box that loses data.
    expect(screen.getByText(/evicting one means losing it/i)).toBeInTheDocument()
    expect(screen.getByText(/goes with the pod when it moves/i)).toBeInTheDocument()

    await userEvent.click(screen.getByLabelText(/no controller owns/i))
    await userEvent.click(screen.getByLabelText(/local storage/i))
    await userEvent.click(screen.getByRole('button', { name: /drain this node/i }))

    expect(drain).toHaveBeenCalledWith('c-1', 'cp-1', { force: true, delete_local_data: true })
  })

  it('says a drain is a job rather than pretending it finished', async () => {
    vi.spyOn(api.kubernetes, 'drain').mockResolvedValue(acceptedJob)

    wrap(<NodeSchedulingActions clusterID="c-1" node="cp-1" unschedulable={false} />)
    await userEvent.click(screen.getByRole('button', { name: 'Drain…' }))
    await userEvent.click(screen.getByRole('button', { name: /drain this node/i }))

    await waitFor(() => expect(screen.getByRole('status')).toBeInTheDocument())
    expect(screen.getByRole('status')).toHaveTextContent(/runs as a job/i)
  })
})
