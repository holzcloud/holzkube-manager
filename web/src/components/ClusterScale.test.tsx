import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ClusterScalePanel } from '@/components/ClusterScale'

/**
 * The scale panel, and the two things it must not do.
 *
 * It must not read etcd until somebody asks — a cluster list that answered this
 * question per card on a timer would be a fleet overview quietly polling every
 * cluster it can see. And it must not paraphrase a refusal: the sentence beside
 * a node that cannot be removed is the server's own, because it is the sentence
 * the removal route would produce, and two accounts of one condition is how an
 * operator comes to believe the screen and the server disagree.
 */

function wrap(node: ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
}

const REFUSAL =
  'upgrade: removing this member would leave etcd without a quorum: cp-1 is the only etcd ' +
  'member. Removing it does not make the cluster smaller, it ends it'

function answer(body: unknown) {
  // The parameters are declared even though the body ignores them: without
  // them the mock's call tuple is empty and `calls[0][0]` does not typecheck,
  // which is how the URL assertion below would have been quietly dropped.
  return vi.fn((_input: RequestInfo | URL, _init?: RequestInit) =>
    Promise.resolve(
      new Response(JSON.stringify(body), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    ),
  )
}

const plan = {
  plan: {
    cluster: 'c-1',
    name: 'homelab',
    control_plane: 1,
    workers: 1,
    members_known: true,
    voting: 1,
    tolerates: 0,
    members_problem: '',
    removals: [
      { machine: 'm-1', name: 'cp-1', role: 'controlplane', allowed: false, reason: REFUSAL },
      { machine: 'm-2', name: 'w-1', role: 'worker', allowed: true, reason: '' },
    ],
    additions: [{ machine: 'm-3', name: 'new-1', ready: false, reason: 'This machine is down.' }],
    advice: ['One voting member. Add two to reach a control plane that survives losing one.'],
    sentence: 'homelab: 1 control-plane node(s) and 1 worker(s), etcd tolerating the loss of 0.',
  },
  notice: 'Nothing here changes the cluster.',
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('the cluster scale panel', () => {
  it('reads nothing until it is asked', async () => {
    const fetchMock = answer(plan)
    vi.stubGlobal('fetch', fetchMock)

    wrap(<ClusterScalePanel clusterID="c-1" />)

    expect(fetchMock).not.toHaveBeenCalled()

    await userEvent.click(screen.getByRole('button', { name: /What can this cluster spare/i }))

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1))
    const [url] = fetchMock.mock.calls[0] ?? []
    expect(String(url)).toContain('/api/v1/clusters/c-1/scale')
  })

  it("shows the server's own refusal rather than one of its own", async () => {
    vi.stubGlobal('fetch', answer(plan))

    wrap(<ClusterScalePanel clusterID="c-1" />)
    await userEvent.click(screen.getByRole('button', { name: /What can this cluster spare/i }))

    // The whole sentence, including what to do instead. A table cell that
    // showed "no" would be a screen that refused without saying why, which is
    // the screen an operator works around.
    await screen.findByText(/it ends it/)
    expect(screen.getByText(/the only etcd member/)).toBeInTheDocument()

    // The arithmetic, which is why the panel exists.
    expect(screen.getByText(/Add two/)).toBeInTheDocument()

    // And the server's statement about its own scope, not one written here.
    expect(screen.getByText('Nothing here changes the cluster.')).toBeInTheDocument()
  })

  it('lists a machine that cannot join rather than hiding it', async () => {
    vi.stubGlobal('fetch', answer(plan))

    wrap(<ClusterScalePanel clusterID="c-1" />)
    await userEvent.click(screen.getByRole('button', { name: /What can this cluster spare/i }))

    // "There is nothing to add" and "there is a machine and it is down" send
    // an operator to different places.
    await screen.findByText('new-1')
    expect(screen.getByText(/This machine is down/)).toBeInTheDocument()
  })
})
