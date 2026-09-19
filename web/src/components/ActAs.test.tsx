import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, type KubeIdentity } from '@/api'
import { ActAs } from '@/components/ActAs'

/**
 * Whose name the cluster sees.
 *
 * The claim worth pinning is the preview: storing a name the cluster's RBAC has
 * never heard of breaks every Kubernetes screen at once, and the failure looks
 * like the product is broken rather than like a setting is wrong. So nothing is
 * stored that has not been checked against the cluster.
 */

function wrap(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
}

const asProduct: KubeIdentity = {
  user: '',
  groups: [],
  describes: "holzkube-manager (this product's own certificate)",
  permissions: [],
  missing: 0,
  notice: 'This is what the cluster says.',
}

const allowed: KubeIdentity = {
  user: 'holz@holzcloud.ch',
  groups: [],
  describes: 'holz@holzcloud.ch',
  permissions: [{ verb: 'list', resource: 'pods', namespace: '', allowed: true, reason: '' }],
  missing: 0,
  notice: 'This is what the cluster says.',
}

const refused: KubeIdentity = {
  ...allowed,
  permissions: [
    { verb: 'list', resource: 'pods', namespace: '', allowed: true, reason: '' },
    {
      verb: 'patch',
      resource: 'nodes',
      namespace: '',
      allowed: false,
      reason: 'no RBAC policy allows "holz@holzcloud.ch" to patch nodes',
    },
  ],
  missing: 1,
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('choosing whose name the cluster sees', () => {
  it('will not store a name that has not been checked', async () => {
    vi.spyOn(api.kubernetes, 'identity').mockResolvedValue(asProduct)
    const save = vi.spyOn(api.kubernetes, 'setIdentity').mockResolvedValue(undefined)

    wrap(<ActAs clusterID="c-1" />)
    await userEvent.type(screen.getByLabelText(/act as/i), 'holz@holzcloud.ch')

    expect(screen.getByRole('button', { name: 'Use it' })).toBeDisabled()
    expect(save).not.toHaveBeenCalled()
  })

  it('shows the cluster’s own refusals before anything is stored', async () => {
    const identity = vi
      .spyOn(api.kubernetes, 'identity')
      .mockImplementation(async (_cluster, opts) => (opts?.as ? refused : asProduct))

    wrap(<ActAs clusterID="c-1" />)
    await userEvent.type(screen.getByLabelText(/act as/i), 'holz@holzcloud.ch')
    await userEvent.click(screen.getByRole('button', { name: 'Check it' }))

    await waitFor(() => expect(screen.getByText(/refuses 1 of 2/i)).toBeInTheDocument())
    // The authoriser's own sentence, not this product's paraphrase of somebody
    // else's RBAC.
    expect(screen.getByText(/no RBAC policy allows/i)).toBeInTheDocument()
    expect(identity).toHaveBeenCalledWith('c-1', { as: 'holz@holzcloud.ch' })
  })

  it('stores exactly the name that was checked', async () => {
    vi.spyOn(api.kubernetes, 'identity').mockImplementation(async (_cluster, opts) =>
      opts?.as ? allowed : asProduct,
    )
    const save = vi.spyOn(api.kubernetes, 'setIdentity').mockResolvedValue(undefined)

    wrap(<ActAs clusterID="c-1" />)
    const field = screen.getByLabelText(/act as/i)
    await userEvent.type(field, 'holz@holzcloud.ch')
    await userEvent.click(screen.getByRole('button', { name: 'Check it' }))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Use it' })).toBeEnabled())

    await userEvent.click(screen.getByRole('button', { name: 'Use it' }))
    expect(save).toHaveBeenCalledWith('c-1', 'holz@holzcloud.ch')
  })

  it('throws the check away when the name is edited afterwards', async () => {
    vi.spyOn(api.kubernetes, 'identity').mockImplementation(async (_cluster, opts) =>
      opts?.as ? allowed : asProduct,
    )
    const save = vi.spyOn(api.kubernetes, 'setIdentity').mockResolvedValue(undefined)

    wrap(<ActAs clusterID="c-1" />)
    const field = screen.getByLabelText(/act as/i)
    await userEvent.type(field, 'holz@holzcloud.ch')
    await userEvent.click(screen.getByRole('button', { name: 'Check it' }))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Use it' })).toBeEnabled())

    // A check belongs to the text it was made for, the same rule the manifest
    // plan follows.
    await userEvent.type(field, 'x')

    expect(screen.getByRole('button', { name: 'Use it' })).toBeDisabled()
    expect(save).not.toHaveBeenCalled()
  })

  it('says what acting as this product’s own certificate costs', async () => {
    vi.spyOn(api.kubernetes, 'identity').mockResolvedValue(asProduct)

    wrap(<ActAs clusterID="c-1" />)

    expect(await screen.findByText(/system:masters/)).toBeInTheDocument()
    expect(screen.getByText(/audit log records that name rather than yours/i)).toBeInTheDocument()
  })

  it('warns when the identity in use cannot do everything, and says it will not retry as admin', async () => {
    vi.spyOn(api.kubernetes, 'identity').mockResolvedValue(refused)

    wrap(<ActAs clusterID="c-1" />)

    expect(await screen.findByText(/will not retry them as an administrator/i)).toBeInTheDocument()
  })
})
