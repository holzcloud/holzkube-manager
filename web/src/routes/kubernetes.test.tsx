import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Cluster, KubernetesOverview } from '@/api'
import { api } from '@/api'
import { validateKubernetesSearch } from '@/components/KubernetesSection'
import { KubernetesAccessPage } from '@/routes/kubernetes/access'
import { KubernetesConfigPage } from '@/routes/kubernetes/config'
import { KubernetesEventsPage } from '@/routes/kubernetes/events'
import { KubernetesMaintenancePage } from '@/routes/kubernetes/maintenance'
import { KubernetesNamespacesPage } from '@/routes/kubernetes/namespaces'
import { KubernetesNetworkPage } from '@/routes/kubernetes/network'
import { KubernetesOverviewPage } from '@/routes/kubernetes/overview'
import { KubernetesPodsPage } from '@/routes/kubernetes/pods'
import { KubernetesStoragePage } from '@/routes/kubernetes/storage'
import { KubernetesWorkloadsPage } from '@/routes/kubernetes/workloads'

/**
 * The Kubernetes screen, and the two things it must not do.
 *
 * It must not render an empty list when the API server did not answer — that is
 * INV-08 one layer up, and it sends an operator looking for a workload instead
 * of for an API server. And it must not reduce a pod to its phase: `Running`
 * with 1 of 2 ready is a broken workload, and the phase alone calls it healthy.
 *
 * # Why this test now builds a router
 *
 * The screen was one page and is ten (2026-09-20). Which cluster and which
 * namespace live in the URL so that moving between them keeps the selection, so
 * a page cannot be rendered without a router any more — and testing it without
 * one would mean testing something the operator never sees.
 *
 * The tree here is the real one: the same layout route with the same
 * `validateSearch`, the same child paths, and the real page components. A stub
 * tree would let a link point at a page that does not exist, which is exactly
 * the kind of thing this has to catch.
 */

const PAGES: [string, () => React.JSX.Element][] = [
  ['/', KubernetesOverviewPage],
  ['workloads', KubernetesWorkloadsPage],
  ['pods', KubernetesPodsPage],
  ['storage', KubernetesStoragePage],
  ['network', KubernetesNetworkPage],
  ['config', KubernetesConfigPage],
  ['namespaces', KubernetesNamespacesPage],
  ['access', KubernetesAccessPage],
  ['events', KubernetesEventsPage],
  ['maintenance', KubernetesMaintenancePage],
]

/** at renders the Kubernetes routes at one address. */
function at(path: string) {
  const rootRoute = createRootRoute({ component: Outlet })
  const layout = createRoute({
    getParentRoute: () => rootRoute,
    path: '/kubernetes',
    validateSearch: validateKubernetesSearch,
    component: Outlet,
  })
  const tree = rootRoute.addChildren([
    layout.addChildren(
      PAGES.map(([child, component]) =>
        createRoute({ getParentRoute: () => layout, path: child, component }),
      ),
    ),
  ])
  const router = createRouter({
    routeTree: tree,
    history: createMemoryHistory({ initialEntries: [path] }),
  })

  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      {/* biome-ignore lint/suspicious/noExplicitAny: the test tree is not the
          registered one, so the router's generated types do not describe it. */}
      <RouterProvider router={router as any} />
    </QueryClientProvider>,
  )
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
      cpu_request: '250m',
      memory_request: '256Mi',
      cpu_limit: '',
      memory_limit: '512Mi',
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

    at('/kubernetes/pods')

    await waitFor(() => expect(screen.getByText('api-7c9')).toBeInTheDocument())
    expect(screen.getByText('1/2')).toBeInTheDocument()
    expect(screen.getByText(/Running/)).toBeInTheDocument()
    expect(screen.getByText(/CrashLoopBackOff/)).toBeInTheDocument()
    expect(screen.getByText('14')).toBeInTheDocument()

    // The node it runs on. It is on the PODS page once now, not twice: the
    // node table moved to the overview, which is the whole point of the split.
    expect(screen.getByText('holzkube-01')).toBeInTheDocument()
  })

  it('shows the nodes on the overview, with their scheduling state', async () => {
    vi.spyOn(api.clusters, 'list').mockResolvedValue([cluster])
    vi.spyOn(api.kubernetes, 'overview').mockResolvedValue(overview)

    at('/kubernetes')

    await waitFor(() => expect(screen.getByText('holzkube-01')).toBeInTheDocument())
    expect(screen.getByText('schedulable')).toBeInTheDocument()
  })

  it('says a cordoned node is cordoned rather than leaving it looking normal', async () => {
    vi.spyOn(api.clusters, 'list').mockResolvedValue([cluster])
    vi.spyOn(api.kubernetes, 'overview').mockResolvedValue({
      ...overview,
      nodes: [{ ...firstNode, unschedulable: true }],
    })

    at('/kubernetes')

    await waitFor(() => expect(screen.getByText('cordoned')).toBeInTheDocument())
  })

  it('does not present an unanswered API server as a cluster with nothing running', async () => {
    vi.spyOn(api.clusters, 'list').mockResolvedValue([cluster])
    vi.spyOn(api.kubernetes, 'overview').mockRejectedValue(
      new Error('the API server did not answer'),
    )

    at('/kubernetes/pods')

    await waitFor(() =>
      expect(screen.getByText(/was asked and did not answer/i)).toBeInTheDocument(),
    )
    // And no pod table at all: an empty one would be the claim.
    expect(screen.queryByText(/there are no pods/i)).not.toBeInTheDocument()
  })

  it('says so when the API server answered and there is genuinely nothing', async () => {
    vi.spyOn(api.clusters, 'list').mockResolvedValue([cluster])
    vi.spyOn(api.kubernetes, 'overview').mockResolvedValue({ ...overview, pods: [] })

    at('/kubernetes/pods')

    // The difference from the case above, in words: asked and empty is a fact,
    // unasked and empty is not.
    await waitFor(() =>
      expect(screen.getByText(/answered, and there are no pods/i)).toBeInTheDocument(),
    )
  })

  it('asks the server for one namespace rather than filtering in the browser', async () => {
    vi.spyOn(api.clusters, 'list').mockResolvedValue([cluster])
    const call = vi.spyOn(api.kubernetes, 'overview').mockResolvedValue(overview)

    at('/kubernetes/pods')
    await waitFor(() => expect(call).toHaveBeenCalled())

    // The first read is every namespace, which is the question somebody
    // arrives with: what is broken, not what is broken in kube-system.
    expect(call).toHaveBeenCalledWith('c-1', '')
  })

  it('keeps the cluster and the namespace when moving between pages', async () => {
    vi.spyOn(api.clusters, 'list').mockResolvedValue([cluster])
    const call = vi.spyOn(api.kubernetes, 'overview').mockResolvedValue(overview)
    vi.spyOn(api.kubernetes, 'storage').mockResolvedValue({
      volumes: [],
      claims: [],
      classes: [],
      notice: 'nothing here',
    })

    at('/kubernetes/pods?namespace=kube-system')
    await waitFor(() => expect(call).toHaveBeenCalledWith('c-1', 'kube-system'))

    await userEvent.click(await screen.findByRole('link', { name: 'Storage' }))

    // The selection is in the URL, so the page somebody lands on asks the same
    // question the page they left was asking. Held in component state it would
    // have reset to every namespace here, silently.
    await waitFor(() => expect(api.kubernetes.storage).toHaveBeenCalledWith('c-1', 'kube-system'))
  })

  it('offers every section as a link, so none of them is reachable only by typing', async () => {
    vi.spyOn(api.clusters, 'list').mockResolvedValue([cluster])
    vi.spyOn(api.kubernetes, 'overview').mockResolvedValue(overview)

    at('/kubernetes')

    const nav = await screen.findByRole('navigation', { name: 'Kubernetes sections' })
    // Eleven pages, eleven links -- Apps joined on 2026-09-26. A page with no
    // link in is a page that shipped and cannot be found, which is the defect
    // the reachability guard exists for one layer down.
    expect(nav.querySelectorAll('a')).toHaveLength(11)
  })
})
