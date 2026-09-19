import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Cluster, KubernetesOverview } from '@/api'
import { api } from '@/api'
import { KubernetesView } from '@/routes/kubernetes'

/**
 * The Kubernetes screen, and the two things it must not do.
 *
 * It must not render an empty list when the API server did not answer — that is
 * INV-08 one layer up, and it sends an operator looking for a workload instead
 * of for an API server. And it must not reduce a pod to its phase: `Running`
 * with 1 of 2 ready is a broken workload, and the phase alone calls it healthy.
 */

function wrap(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
}

const cluster: Cluster = {
  id: 'c-1',
  name: 'homelab',
  origin: 'imported',
  endpoint: '192.168.1.110',
  locked: true,
  created_at: '2026-09-01T00:00:00Z',
  client_cert_not_after: '2027-09-01T00:00:00Z',
  client_cert_days_left: 354,
  certificate_warning: '',
  certificate_urgency: 'none',
  nodes: 1,
  control_plane: 1,
  workers: 0,
  healthy: 1,
  degraded: 0,
  down: 0,
  checking: 0,
}

const overview: KubernetesOverview = {
  cluster: 'c-1',
  server_version: 'v1.34.1',
  namespace: '',
  namespaces: ['default', 'kube-system'],
  services: [
    {
      namespace: 'default',
      name: 'api',
      type: 'ClusterIP',
      cluster_ip: '10.96.0.12',
      ports: [{ name: 'http', port: 8080, protocol: 'TCP' }],
    },
  ],
  nodes: [
    {
      name: 'holzkube-01',
      ready: 'True',
      unschedulable: false,
      roles: ['control-plane'],
      kubelet_version: 'v1.34.1',
      os_image: 'Talos (v1.14.1)',
      created_at: '2026-09-01T00:00:00Z',
      internal_address: '192.168.1.110',
      container_runtime: 'containerd://2.1.4',
    },
  ],
  deployments: [
    {
      namespace: 'default',
      name: 'api',
      desired: 3,
      ready: 1,
      updated: 1,
      available: 1,
      image: 'example/api:1.4',
      created_at: '2026-09-18T00:00:00Z',
    },
  ],
  pods: [
    {
      namespace: 'default',
      name: 'api-7c9',
      node: 'holzkube-01',
      phase: 'Running',
      ready: 1,
      containers: 2,
      restarts: 14,
      reason: 'CrashLoopBackOff',
      created_at: '2026-09-18T00:00:00Z',
    },
  ],
}

// firstNode is the fixture's node, read once so the spread below keeps every
// required field rather than making them optional.
const firstNode = overview.nodes[0] as KubernetesOverview['nodes'][number]

afterEach(() => {
  vi.restoreAllMocks()
})

describe('the Kubernetes screen', () => {
  it('shows a pod’s phase and its readiness, because the phase alone calls a broken workload healthy', async () => {
    vi.spyOn(api.clusters, 'list').mockResolvedValue([cluster])
    vi.spyOn(api.kubernetes, 'overview').mockResolvedValue(overview)

    wrap(<KubernetesView />)

    await waitFor(() => expect(screen.getByText('api-7c9')).toBeInTheDocument())
    expect(screen.getByText('1/2')).toBeInTheDocument()
    expect(screen.getByText(/Running/)).toBeInTheDocument()
    expect(screen.getByText(/CrashLoopBackOff/)).toBeInTheDocument()
    expect(screen.getByText('14')).toBeInTheDocument()

    // The node's Kubernetes side, which the Nodes screen cannot answer. The
    // name is on the screen twice on purpose -- once as a node, once as the
    // pod's node -- so this asks for both rather than for "the" one.
    expect(screen.getAllByText('holzkube-01')).toHaveLength(2)
    expect(screen.getByText('schedulable')).toBeInTheDocument()
  })

  it('says a cordoned node is cordoned rather than leaving it looking normal', async () => {
    vi.spyOn(api.clusters, 'list').mockResolvedValue([cluster])
    vi.spyOn(api.kubernetes, 'overview').mockResolvedValue({
      ...overview,
      nodes: [{ ...firstNode, unschedulable: true }],
    })

    wrap(<KubernetesView />)

    await waitFor(() => expect(screen.getByText('cordoned')).toBeInTheDocument())
  })

  it('does not present an unanswered API server as a cluster with nothing running', async () => {
    vi.spyOn(api.clusters, 'list').mockResolvedValue([cluster])
    vi.spyOn(api.kubernetes, 'overview').mockRejectedValue(
      new Error('the API server did not answer'),
    )

    wrap(<KubernetesView />)

    await waitFor(() =>
      expect(screen.getByText(/was asked and did not answer/i)).toBeInTheDocument(),
    )
    // And no pod table at all: an empty one would be the claim.
    expect(screen.queryByText(/there are no pods/i)).not.toBeInTheDocument()
  })

  it('says so when the API server answered and there is genuinely nothing', async () => {
    vi.spyOn(api.clusters, 'list').mockResolvedValue([cluster])
    vi.spyOn(api.kubernetes, 'overview').mockResolvedValue({ ...overview, pods: [] })

    wrap(<KubernetesView />)

    // The difference from the case above, in words: asked and empty is a fact,
    // unasked and empty is not.
    await waitFor(() =>
      expect(screen.getByText(/answered, and there are no pods/i)).toBeInTheDocument(),
    )
  })

  it('asks the server for one namespace rather than filtering in the browser', async () => {
    vi.spyOn(api.clusters, 'list').mockResolvedValue([cluster])
    const call = vi.spyOn(api.kubernetes, 'overview').mockResolvedValue(overview)

    wrap(<KubernetesView />)
    await waitFor(() => expect(call).toHaveBeenCalled())

    // The first read is every namespace, which is the question somebody
    // arrives with: what is broken, not what is broken in kube-system.
    expect(call).toHaveBeenCalledWith('c-1', '')
  })
})
