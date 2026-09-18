import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { AuthorityPreview } from '@/api'
import { api } from '@/api'
import { RotateAuthority } from '@/components/RotateAuthority'

/**
 * The gates in front of a certificate-authority rotation.
 *
 * This is the one operation in the product that can leave a cluster nobody can
 * reach, so the panel's job is to be read rather than clicked: the passes, the
 * nodes, the warnings, and a name that has to be typed. The server checks the
 * phrase again, and these tests are about the half an operator meets first.
 */

function wrap(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
}

const plan: AuthorityPreview = {
  cluster: 'c-1',
  name: 'homelab',
  confirm_phrase: 'homelab',
  nodes: [
    { id: 'uuid-1', hostname: 'cp-1', role: 'controlplane' },
    { id: 'uuid-2', hostname: '', role: 'worker' },
  ],
  in_progress: false,
  locked: false,
  passes: ['pass one', 'pass two', 'pass three', 'pass four'],
  warnings: ['Every node in this cluster has to answer.', 'Never run against real hardware.'],
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('rotating a cluster’s certificate authority', () => {
  it('reads nothing until it is asked', () => {
    const preview = vi.spyOn(api.authority, 'preview')

    wrap(<RotateAuthority clusterID="c-1" />)

    expect(preview).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: /rotate this cluster/i })).toBeInTheDocument()
  })

  it('shows the passes, the nodes and the warnings before anything can be typed', async () => {
    vi.spyOn(api.authority, 'preview').mockResolvedValue(plan)

    wrap(<RotateAuthority clusterID="c-1" />)
    await userEvent.click(screen.getByRole('button', { name: /rotate this cluster/i }))

    await waitFor(() => expect(screen.getByText('pass one')).toBeInTheDocument())
    expect(screen.getByText('pass four')).toBeInTheDocument()
    expect(screen.getByText(/Every node in this cluster has to answer/)).toBeInTheDocument()
    expect(screen.getByText(/Never run against real hardware/)).toBeInTheDocument()

    // The node with no hostname is still named, by its id: a row that showed
    // nothing would be a node the operator cannot check against the fleet.
    expect(screen.getByText(/uuid-2/)).toBeInTheDocument()
    expect(screen.getByText(/cp-1/)).toBeInTheDocument()
  })

  it('will not start until the cluster’s name is typed exactly, and says so beside the button', async () => {
    vi.spyOn(api.authority, 'preview').mockResolvedValue(plan)
    const confirm = vi.spyOn(api.authority, 'confirm')

    wrap(<RotateAuthority clusterID="c-1" />)
    await userEvent.click(screen.getByRole('button', { name: /rotate this cluster/i }))
    await waitFor(() => expect(screen.getByText('pass one')).toBeInTheDocument())

    const start = screen.getByRole('button', { name: /^rotate the authority$/i })
    expect(start).toBeDisabled()
    expect(screen.getByText(/name has to match exactly/i)).toBeInTheDocument()

    await userEvent.type(screen.getByLabelText(/to confirm/i), 'homelab-2')
    expect(start).toBeDisabled()

    await userEvent.clear(screen.getByLabelText(/to confirm/i))
    await userEvent.type(screen.getByLabelText(/to confirm/i), 'homelab')
    await waitFor(() => expect(start).toBeEnabled())
    expect(confirm).not.toHaveBeenCalled()
  })

  it('refuses a locked cluster with the reason beside the button rather than in a tooltip', async () => {
    vi.spyOn(api.authority, 'preview').mockResolvedValue({ ...plan, locked: true })

    wrap(<RotateAuthority clusterID="c-1" />)
    await userEvent.click(screen.getByRole('button', { name: /rotate this cluster/i }))
    await waitFor(() => expect(screen.getByText('pass one')).toBeInTheDocument())

    await userEvent.type(screen.getByLabelText(/to confirm/i), 'homelab')

    expect(screen.getByRole('button', { name: /^rotate the authority$/i })).toBeDisabled()
    // Beside the button, because a phone has no hover and a title attribute is
    // not a reason there.
    expect(screen.getByText(/adopted read-only/i)).toBeInTheDocument()
  })

  it('says a started rotation is continuing rather than beginning a second one', async () => {
    vi.spyOn(api.authority, 'preview').mockResolvedValue({ ...plan, in_progress: true })

    wrap(<RotateAuthority clusterID="c-1" />)
    await userEvent.click(screen.getByRole('button', { name: /rotate this cluster/i }))

    await waitFor(() =>
      expect(screen.getByText(/did not finish\. Starting it again continues/i)).toBeInTheDocument(),
    )
  })

  it('confirms and then submits, in that order, and points at the job', async () => {
    vi.spyOn(api.authority, 'preview').mockResolvedValue(plan)
    const calls: string[] = []
    vi.spyOn(api.authority, 'confirm').mockImplementation(async (_cluster, typed) => {
      calls.push(`confirm:${typed}`)
      return { token: 'tok', expires: '2026-09-18T12:00:00Z' }
    })
    vi.spyOn(api.authority, 'rotate').mockImplementation(async (_cluster, confirmation) => {
      calls.push(`rotate:${confirmation}`)
      return {
        job: {
          id: 'job-1',
          kind: 'cluster.rotate-ca',
          state: 'pending',
          current: 0,
          steps: [
            { name: 'one', state: 'pending', verifiable: true },
            { name: 'two', state: 'pending', verifiable: true },
          ],
        },
        topic: 'jobs/job-1',
      } as Awaited<ReturnType<typeof api.authority.rotate>>
    })

    wrap(<RotateAuthority clusterID="c-1" />)
    await userEvent.click(screen.getByRole('button', { name: /rotate this cluster/i }))
    await waitFor(() => expect(screen.getByText('pass one')).toBeInTheDocument())
    await userEvent.type(screen.getByLabelText(/to confirm/i), 'homelab')
    await userEvent.click(screen.getByRole('button', { name: /^rotate the authority$/i }))

    await waitFor(() => expect(screen.getByRole('status')).toBeInTheDocument())
    // The token is carried from one call into the next: a submission that sent
    // no confirmation would be refused by the server, and a screen that skipped
    // the confirm call would be a screen whose dialog authorises nothing.
    expect(calls).toEqual(['confirm:homelab', 'rotate:tok'])
    expect(screen.getByRole('status')).toHaveTextContent(/2 steps/)
  })
})
