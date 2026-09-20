import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { type ClusterNetwork as Answer, api, type ServiceSummary } from '@/api'
import { ClusterNetwork } from '@/components/ClusterNetwork'

/**
 * The cluster's networking.
 *
 * The claim worth the most: a Service with nothing behind it looks completely
 * healthy in every list, and this screen exists to stop it looking that way.
 */

function wrap(node: ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
}

const working: ServiceSummary = {
  namespace: 'default',
  name: 'website',
  type: 'ClusterIP',
  cluster_ip: '10.96.0.10',
  external: '',
  ports: ['http 80/TCP → 8080'],
  selector: 'app=website',
  endpoints: 3,
  ready_endpoints: 3,
  healthy: true,
  notice: '',
  created_at: '',
}

const empty: ServiceSummary = {
  ...working,
  name: 'api',
  cluster_ip: '10.96.0.11',
  selector: 'app=api',
  endpoints: 0,
  ready_endpoints: 0,
  healthy: false,
  notice: 'Nothing is behind it: no pod matches app=api.',
}

const answer: Answer = {
  services: [working, empty],
  policies: [
    {
      namespace: 'default',
      name: 'api-lockdown',
      applies: 'app=api',
      types: ['Ingress'],
      selects: 0,
      isolating: false,
      summary: 'It matches no pod in default, so it protects nothing.',
      healthy: false,
      created_at: '',
    },
  ],
  unprotected: ['monitoring', 'open'],
  ingress_classes: ['nginx'],
  default_ingress_class: 'nginx',
  notice: 'Endpoints are counted from EndpointSlices.',
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('the cluster’s networking', () => {
  it('puts a service with nothing behind it first, and says so', async () => {
    vi.spyOn(api.kubernetes, 'network').mockResolvedValue(answer)

    wrap(<ClusterNetwork clusterID="c-1" namespace="" />)

    // Broken first: the question is what is wrong, and a list sorted by name
    // makes somebody read all of it to find out.
    const rows = await screen.findAllByText(/default\/(api|website)/)
    expect(rows[0]).toHaveTextContent('default/api')
    expect(screen.getByText('nothing')).toBeInTheDocument()
    // The selector is named. "Nothing matches" without saying what does not
    // match leaves somebody where they were.
    expect(screen.getByText(/no pod matches app=api/)).toBeInTheDocument()
  })

  it('shows both endpoint numbers, because an unready one serves nothing', async () => {
    vi.spyOn(api.kubernetes, 'network').mockResolvedValue({
      ...answer,
      services: [{ ...working, endpoints: 3, ready_endpoints: 1, healthy: false }],
    })

    wrap(<ClusterNetwork clusterID="c-1" namespace="" />)

    expect(await screen.findByText('1 of 3 ready')).toBeInTheDocument()
  })

  it('says a ClusterIP is not reachable from outside', async () => {
    vi.spyOn(api.kubernetes, 'network').mockResolvedValue({ ...answer, services: [working] })

    wrap(<ClusterNetwork clusterID="c-1" namespace="" />)

    // The answer to "why can I not reach this from my laptop", which a list of
    // ports cannot give.
    expect(await screen.findByText(/inside only/)).toBeInTheDocument()
  })

  it('names the namespaces with no policy rather than counting them', async () => {
    vi.spyOn(api.kubernetes, 'network').mockResolvedValue(answer)

    wrap(<ClusterNetwork clusterID="c-1" namespace="" />)

    // "Eleven namespaces are open" is not actionable; a list is.
    expect(await screen.findByText('monitoring, open')).toBeInTheDocument()
    expect(screen.getByText(/2 namespaces have pods and no NetworkPolicy/)).toBeInTheDocument()
  })

  it('says nothing about open namespaces when none are', async () => {
    vi.spyOn(api.kubernetes, 'network').mockResolvedValue({ ...answer, unprotected: [] })

    wrap(<ClusterNetwork clusterID="c-1" namespace="" />)

    await screen.findByText('default/api')
    expect(screen.queryByText(/no NetworkPolicy, so every pod/)).not.toBeInTheDocument()
  })

  it('warns when no ingress class is the default', async () => {
    vi.spyOn(api.kubernetes, 'network').mockResolvedValue({
      ...answer,
      default_ingress_class: '',
    })

    wrap(<ClusterNetwork clusterID="c-1" namespace="" />)

    // An Ingress naming no class is then one no controller will ever pick up,
    // and it looks exactly like a working one.
    expect(await screen.findByText(/none is the default/)).toBeInTheDocument()
  })

  it('says an empty policy list is Kubernetes’s default, not a setting', async () => {
    vi.spyOn(api.kubernetes, 'network').mockResolvedValue({ ...answer, policies: [] })

    wrap(<ClusterNetwork clusterID="c-1" namespace="" />)

    // An empty screen is a claim (INV-08): whose claim, and what it means.
    expect(
      await screen.findByText(/every pod may reach every other pod, which is Kubernetes/),
    ).toBeInTheDocument()
  })
})
