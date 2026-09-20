import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, type ClusterCapacity, type NodeCapacity } from '@/api'
import { ClusterCapacityPanel } from '@/components/ClusterCapacityPanel'

/**
 * How full the cluster is.
 *
 * The tests here are about the two ways this screen could lie: reporting a node
 * that has said nothing as a node at 0%, and counting room on a node where
 * nothing will be placed.
 */

function wrap(node: ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
}

const busyNode: NodeCapacity = {
  name: 'cp-1',
  ready: true,
  cordoned: false,
  cpu: { allocatable: '4', requested: '1400m', percent: 35 },
  memory: { allocatable: '15Gi', requested: '14Gi', percent: 93 },
  pods: { allocatable: '110', requested: '31', percent: 28 },
  pods_running: 31,
}

const answer: ClusterCapacity = {
  cpu: { allocatable: '8', requested: '4200m', percent: 53 },
  memory: { allocatable: '30Gi', requested: '27Gi', percent: 90 },
  pods: { allocatable: '220', requested: '74', percent: 34 },
  nodes: [
    busyNode,
    {
      name: 'cp-2',
      ready: true,
      cordoned: true,
      cpu: { allocatable: '4', requested: '2800m', percent: 70 },
      memory: { allocatable: '15Gi', requested: '13Gi', percent: 87 },
      pods: { allocatable: '110', requested: '43', percent: 39 },
      pods_running: 43,
    },
    // Has reported nothing. -1, not 0.
    {
      name: 'cp-3',
      ready: false,
      cordoned: false,
      cpu: { allocatable: '', requested: '0', percent: -1 },
      memory: { allocatable: '', requested: '0', percent: -1 },
      pods: { allocatable: '', requested: '0', percent: -1 },
      pods_running: 0,
    },
  ],
  notice: 'Requested is what the pods asked for.',
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('how full the cluster is', () => {
  it('shows both quantities, not only a percentage', async () => {
    vi.spyOn(api.kubernetes, 'capacity').mockResolvedValue(answer)

    wrap(<ClusterCapacityPanel clusterID="c-1" />)

    // A bar alone cannot say "4200m of 8", and that is the answer; the bar is
    // only the comparison.
    expect(await screen.findByText('4200m requested of 8')).toBeInTheDocument()
    expect(screen.getByText('27Gi requested of 30Gi')).toBeInTheDocument()
  })

  it('does not report a node that has said nothing as a node at 0%', async () => {
    vi.spyOn(api.kubernetes, 'capacity').mockResolvedValue(answer)

    wrap(<ClusterCapacityPanel clusterID="c-1" />)

    await screen.findByText('4200m requested of 8')
    // Three per-node figures on cp-3, all of them "not reported". An empty bar
    // would read as a node with all its room free, which is the opposite of
    // what an unready node is.
    expect(screen.getAllByText('not reported')).toHaveLength(3)
    expect(screen.queryByText(/0% · 0 of/)).not.toBeInTheDocument()
  })

  it('says which nodes were left out of the totals, and why', async () => {
    vi.spyOn(api.kubernetes, 'capacity').mockResolvedValue(answer)

    wrap(<ClusterCapacityPanel clusterID="c-1" />)

    expect(await screen.findByText('(cordoned)')).toBeInTheDocument()
    expect(screen.getByText('(not ready)')).toBeInTheDocument()
    // The sentence appears only because such a node is present: a cluster whose
    // nodes all take work does not need the caveat.
    expect(screen.getByText(/left out of the totals above/)).toBeInTheDocument()
  })

  it('says nothing about exclusions when every node takes work', async () => {
    vi.spyOn(api.kubernetes, 'capacity').mockResolvedValue({
      ...answer,
      nodes: [busyNode],
    })

    wrap(<ClusterCapacityPanel clusterID="c-1" />)

    await screen.findByText('4200m requested of 8')
    expect(screen.queryByText(/left out of the totals above/)).not.toBeInTheDocument()
  })

  it('carries the server’s sentence about what these numbers are', async () => {
    vi.spyOn(api.kubernetes, 'capacity').mockResolvedValue(answer)

    wrap(<ClusterCapacityPanel clusterID="c-1" />)

    // "90% full" invites exactly the wrong reading, and the distinction between
    // reserved and used is written once, on the server.
    expect(await screen.findByText('Requested is what the pods asked for.')).toBeInTheDocument()
  })
})
