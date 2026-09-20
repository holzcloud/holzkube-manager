import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, type Inventory, type NamespaceSummary } from '@/api'
import { ClusterInventory } from '@/components/ClusterInventory'

/**
 * Namespaces and the cluster's own kinds.
 *
 * Both findings here read as a fault of the workload: a namespace stuck
 * terminating, and a quota that is full — whose refusal lands on the pod as
 * "exceeded quota".
 */

function wrap(node: ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
}

const healthy: NamespaceSummary = {
  name: 'web',
  phase: 'Active',
  pods_running: 3,
  quotas: [],
  has_limit_range: false,
  healthy: true,
  notice: '',
  created_at: '',
}

const answer: Inventory = {
  namespaces: [
    healthy,
    { ...healthy, name: 'empty-one', pods_running: 0 },
    {
      ...healthy,
      name: 'old-staging',
      phase: 'Terminating',
      pods_running: 0,
      healthy: false,
      notice:
        'It has been deleted and something in it will not go. Some content has finalizers remaining: volumes.longhorn.io in 3 resource instances Until that clears, the name cannot be reused.',
    },
    {
      ...healthy,
      name: 'ci',
      pods_running: 1,
      quotas: [
        { quota: 'build-quota', resource: 'cpu', used: '4', hard: '4', percent: 100 },
        { quota: 'build-quota', resource: 'memory', used: '6Gi', hard: '8Gi', percent: 75 },
      ],
      has_limit_range: true,
      healthy: false,
      notice: 'Its quota is full on cpu, so the next thing asking for it is refused.',
    },
  ],
  custom_kinds: [
    {
      group: 'longhorn.io',
      kind: 'Volume',
      versions: ['v1beta2'],
      stored: 'v1beta2',
      scope: 'Namespaced',
      established: true,
      notice: '',
      created_at: '',
    },
    {
      group: 'example.invalid',
      kind: 'Widget',
      versions: ['v1'],
      stored: 'v1',
      scope: 'Cluster',
      established: false,
      notice: 'The API server is not serving this kind, so every manifest naming it is refused.',
      created_at: '',
    },
  ],
  notice: 'A quota’s used figure is what the namespace has reserved.',
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('namespaces and the cluster’s own kinds', () => {
  it('puts the namespaces that are the problem first', async () => {
    vi.spyOn(api.kubernetes, 'inventory').mockResolvedValue(answer)

    wrap(<ClusterInventory clusterID="c-1" />)

    // Read from the rows themselves rather than by matching the name text: the
    // identity cell carries a suffix on exactly the rows this is about, so a text
    // matcher would silently skip them and the assertion would be about the
    // healthy ones.
    const table = await screen.findByRole('table', { name: 'Namespaces' })
    const names = [...table.querySelectorAll('tbody tr')].map(
      (row) => row.querySelector('td')?.textContent ?? '',
    )
    expect(names).toHaveLength(4)
    expect(names[0]).toMatch(/old-staging|^ci$/)
    expect(names[1]).toMatch(/old-staging|^ci$/)
    // And the healthy ones are after them, not interleaved.
    expect(names.slice(2).join(' ')).toMatch(/web/)
  })

  it('says what is holding a terminating namespace', async () => {
    vi.spyOn(api.kubernetes, 'inventory').mockResolvedValue(answer)

    wrap(<ClusterInventory clusterID="c-1" />)

    // kubectl shows the word and nothing about what is keeping it.
    expect(await screen.findByText('(terminating)')).toBeInTheDocument()
    expect(screen.getByText(/volumes.longhorn.io/)).toBeInTheDocument()
  })

  it('marks the quota line that is full, and not the one that is not', async () => {
    vi.spyOn(api.kubernetes, 'inventory').mockResolvedValue(answer)

    wrap(<ClusterInventory clusterID="c-1" />)

    // cpu is at 4 of 4 and memory at 6 of 8: one is the reason the next pod is
    // refused and the other is ordinary.
    expect(await screen.findByText(/cpu 4\/4 \(full\), memory 6Gi\/8Gi$/)).toBeInTheDocument()
  })

  it('shows an empty namespace as empty rather than as a name', async () => {
    vi.spyOn(api.kubernetes, 'inventory').mockResolvedValue(answer)

    wrap(<ClusterInventory clusterID="c-1" />)

    expect(await screen.findByText('empty-one')).toBeInTheDocument()
    expect(screen.getAllByText('empty').length).toBeGreaterThan(0)
  })

  it('marks a kind the API server is not serving', async () => {
    vi.spyOn(api.kubernetes, 'inventory').mockResolvedValue(answer)

    wrap(<ClusterInventory clusterID="c-1" />)

    // Every manifest naming it is refused, and nothing else in a cluster says so.
    expect(await screen.findByText('(not served)')).toBeInTheDocument()
  })

  it('says a cluster with no kinds of its own may simply not allow reading them', async () => {
    vi.spyOn(api.kubernetes, 'inventory').mockResolvedValue({ ...answer, custom_kinds: [] })

    wrap(<ClusterInventory clusterID="c-1" />)

    // An empty screen is a claim, and here there are two readings of it.
    expect(await screen.findByText(/does not let this identity read them/)).toBeInTheDocument()
  })
})
