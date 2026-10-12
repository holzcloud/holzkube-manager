import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, type Cluster } from '@/api'
import { CertificateBanner } from '@/components/CertificateBanner'
import { NodeCertificatesRow } from '@/components/NodeCertificates'

type Health = NonNullable<Cluster['node_certificates']>

function health(overrides: Partial<Health> = {}): Health {
  return {
    checked_at: '2026-10-12T08:00:00Z',
    read_nodes: 3,
    control_planes: 3,
    urgency: 'none',
    warning: '',
    items: [
      {
        node: 'cp-1',
        name: 'etcd peer',
        kind: 'issued',
        not_after: '2027-06-01T00:00:00Z',
        days_left: 232,
        urgency: 'none',
      },
    ],
    ...overrides,
  }
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('the cluster certificates row', () => {
  it('says "not read yet" rather than showing nothing', () => {
    render(<NodeCertificatesRow health={null} />)
    expect(screen.getByText('not read yet')).toBeInTheDocument()
  })

  it('names the soonest certificate, its node and what the two kinds mean', () => {
    render(<NodeCertificatesRow health={health()} />)
    expect(
      screen.getByText(/1 certificates, soonest: etcd peer on cp-1, 232 days left/),
    ).toBeInTheDocument()
    expect(screen.getByText(/never renewed by Talos/)).toBeInTheDocument()
  })

  it('says when only some control planes answered', () => {
    render(<NodeCertificatesRow health={health({ read_nodes: 2 })} />)
    expect(screen.getByText(/2 of 3 control planes answered/)).toBeInTheDocument()
  })

  it('shows an expired certificate as expired, not as a negative count', () => {
    const items: Health['items'] = [
      {
        node: 'cp-2',
        name: 'etcd authority',
        kind: 'authority',
        not_after: '2026-10-01T00:00:00Z',
        days_left: -11,
        urgency: 'critical',
      },
    ]
    render(<NodeCertificatesRow health={health({ urgency: 'critical', items })} />)
    expect(screen.getByText(/etcd authority on cp-2, expired/)).toBeInTheDocument()
    expect(screen.queryByText(/-11/)).toBeNull()
  })
})

describe('the banner for certificates inside a cluster', () => {
  function clusterWith(node_certificates: Health | null, id = 'c-1') {
    return {
      id,
      name: 'homelab',
      client_cert_days_left: 300,
      client_cert_not_after: '2027-08-01T00:00:00Z',
      certificate_warning: '',
      certificate_urgency: 'none',
      node_certificates,
    } as unknown as Cluster
  }

  function show(clusters: Cluster[]) {
    vi.spyOn(api.clusters, 'list').mockResolvedValue(clusters)
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    return render(
      <QueryClientProvider client={client}>
        <CertificateBanner />
      </QueryClientProvider>,
    )
  }

  it('shows the warning sentence from a banner or critical rung', async () => {
    show([
      clusterWith(
        health({
          urgency: 'critical',
          warning: 'The etcd peer certificate on cp-1 (homelab) expires in 2 days.',
        }),
      ),
    ])
    expect(await screen.findByText(/etcd peer certificate on cp-1/)).toBeInTheDocument()
  })

  it('stays quiet at the badge rung and for a cluster not read yet', async () => {
    // The critical cluster is in the same answer so there is something to wait
    // for: asserting absence before the data has rendered would pass whatever
    // the component did.
    show([
      clusterWith(health({ urgency: 'badge', warning: 'should not appear' }), 'c-1'),
      clusterWith(null, 'c-2'),
      clusterWith(health({ urgency: 'critical', warning: 'this one appears' }), 'c-3'),
    ])
    expect(await screen.findByText('this one appears')).toBeInTheDocument()
    expect(screen.queryByText('should not appear')).toBeNull()
  })
})
