import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { type ClusterStorage as Answer, api } from '@/api'
import { ClusterStorage } from '@/components/ClusterStorage'

/**
 * The cluster's storage.
 *
 * The rows worth testing are the ones that say something no claim knows: whether
 * deleting destroys the data, who is using it, and why a Pending claim is pending.
 */

function wrap(node: ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
}

const answer: Answer = {
  volumes: [
    {
      name: 'pvc-bound',
      capacity: '20Gi',
      phase: 'Bound',
      claim: 'db/postgres-data',
      storage_class: 'longhorn',
      reclaim_policy: 'Delete',
      access_modes: ['RWO'],
      driver: 'driver.longhorn.io',
      notice: '',
      created_at: '',
    },
    {
      name: 'pvc-orphan',
      capacity: '100Gi',
      phase: 'Released',
      claim: '',
      storage_class: 'longhorn',
      reclaim_policy: 'Retain',
      access_modes: ['RWO'],
      driver: 'driver.longhorn.io',
      notice: 'Its claim is gone and the data is still on the disk.',
      created_at: '',
    },
  ],
  claims: [
    {
      namespace: 'db',
      name: 'postgres-data',
      phase: 'Bound',
      requested: '20Gi',
      capacity: '20Gi',
      storage_class: 'longhorn',
      volume: 'pvc-bound',
      access_modes: ['RWO'],
      used_by: ['db/postgres-0'],
      expandable: true,
      notice: '',
      created_at: '',
    },
    {
      namespace: 'default',
      name: 'forgotten',
      phase: 'Bound',
      requested: '5Gi',
      capacity: '5Gi',
      storage_class: 'longhorn',
      volume: 'pvc-old',
      access_modes: ['RWO'],
      used_by: [],
      expandable: true,
      notice: 'Bound, and no running pod mounts it.',
      created_at: '',
    },
  ],
  classes: [
    {
      name: 'longhorn',
      provisioner: 'driver.longhorn.io',
      default: true,
      reclaim_policy: 'Delete',
      binding_mode: 'Immediate',
      allows_expansion: true,
      created_at: '',
    },
    {
      name: 'local-path',
      provisioner: 'rancher.io/local-path',
      default: false,
      reclaim_policy: 'Delete',
      binding_mode: 'WaitForFirstConsumer',
      allows_expansion: false,
      created_at: '',
    },
  ],
  notice: 'A capacity here is what was provisioned, not how full the filesystem on it is.',
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('the cluster’s storage', () => {
  it('says what happens to the data rather than naming the policy', async () => {
    vi.spyOn(api.kubernetes, 'storage').mockResolvedValue(answer)

    wrap(<ClusterStorage clusterID="c-1" namespace="" />)

    // "Delete" and "Retain" under a heading "Reclaim policy" is the most
    // consequential field in the table and the least readable.
    expect(await screen.findByText('data destroyed')).toBeInTheDocument()
    expect(screen.getByText('data kept')).toBeInTheDocument()
  })

  it('shows a volume whose claim is gone and whose data is not', async () => {
    vi.spyOn(api.kubernetes, 'storage').mockResolvedValue(answer)

    wrap(<ClusterStorage clusterID="c-1" namespace="" />)

    expect(await screen.findByText('pvc-orphan')).toBeInTheDocument()
    expect(screen.getByText('Released')).toBeInTheDocument()
  })

  it('says who is using a claim, and says nothing when nothing is', async () => {
    vi.spyOn(api.kubernetes, 'storage').mockResolvedValue(answer)

    wrap(<ClusterStorage clusterID="c-1" namespace="" />)

    expect(await screen.findByText('db/postgres-0')).toBeInTheDocument()
    // A bound claim nothing mounts is storage being paid for and not used, and
    // that is a real answer rather than a blank cell.
    expect(screen.getAllByText('nothing').length).toBeGreaterThan(0)
  })

  it('says which class binds only when a pod needs it', async () => {
    vi.spyOn(api.kubernetes, 'storage').mockResolvedValue(answer)

    wrap(<ClusterStorage clusterID="c-1" namespace="" />)

    // The mode that makes a claim sit Pending for a correct reason, which looks
    // exactly like a broken provisioner.
    expect(await screen.findByText('when a pod needs it')).toBeInTheDocument()
  })

  it('says a cluster with no storage class will hold every claim Pending', async () => {
    vi.spyOn(api.kubernetes, 'storage').mockResolvedValue({ ...answer, classes: [] })

    wrap(<ClusterStorage clusterID="c-1" namespace="" />)

    // An empty screen is a claim (INV-08), and this one has a consequence.
    expect(await screen.findByText(/every claim in it will stay Pending/)).toBeInTheDocument()
  })

  it('carries the sentence about what a capacity is not', async () => {
    vi.spyOn(api.kubernetes, 'storage').mockResolvedValue(answer)

    wrap(<ClusterStorage clusterID="c-1" namespace="" />)

    expect(await screen.findByText(/not how full the filesystem on it is/)).toBeInTheDocument()
  })
})
