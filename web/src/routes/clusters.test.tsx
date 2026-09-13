import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { describe, expect, it } from 'vitest'
import type { Cluster } from '@/api'
import { ClusterCard } from '@/routes/clusters'

/**
 * The two things on a cluster card that leave the browser as files.
 *
 * The support bundle is here because it shipped without a way in. Its route
 * was argued for on the grounds that during an incident the operator is in a
 * browser and not on a console — and then there was nothing to click, which
 * made the argument false and the route unreachable for exactly the person it
 * was built for. It is the same gap the password form had (UAT G-01-1), and
 * this test is what stops it reopening.
 */

function wrap(node: ReactNode) {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
  return render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
}

const cluster: Cluster = {
  id: 'c-1',
  name: 'homelab',
  origin: 'imported',
  endpoint: 'https://10.0.0.1:6443',
  locked: false,
  created_at: '2026-09-01T00:00:00Z',
  client_cert_not_after: '2027-09-01T00:00:00Z',
  client_cert_days_left: 354,
  certificate_warning: '',
  certificate_urgency: 'none',
  nodes: 3,
  control_plane: 3,
  workers: 0,
  healthy: 3,
  degraded: 0,
  down: 0,
}

describe('a cluster card', () => {
  it('offers the support bundle, because a route with nothing to click is a route nobody reaches', () => {
    wrap(<ClusterCard cluster={cluster} />)

    const link = screen.getByRole('link', { name: /support bundle/i })
    expect(link).toHaveAttribute('href', '/api/v1/clusters/c-1/support-bundle')
  })

  it('says what is in the bundle, because the operator is about to send it to somebody', () => {
    wrap(<ClusterCard cluster={cluster} />)

    // The archive carries every node's machine configuration. That it is
    // redacted is the reason it can be forwarded at all, so the interface has
    // to say so before somebody decides whether to attach it to a mail.
    const link = screen.getByRole('link', { name: /support bundle/i })
    expect(link.getAttribute('title')).toMatch(/secret.? removed/i)
  })

  it('still offers the talosconfig alongside it', () => {
    wrap(<ClusterCard cluster={cluster} />)

    expect(screen.getByRole('link', { name: /talosconfig/i })).toHaveAttribute(
      'href',
      '/api/v1/clusters/c-1/talosconfig',
    )
  })

  it('offers the kubeconfig, which is the answer to "how do I run kubectl"', () => {
    wrap(<ClusterCard cluster={cluster} />)

    expect(screen.getByRole('link', { name: /kubeconfig/i })).toHaveAttribute(
      'href',
      '/api/v1/clusters/c-1/kubeconfig',
    )
  })

  it('says what the kubeconfig is before somebody downloads it', () => {
    wrap(<ClusterCard cluster={cluster} />)

    // Two facts an operator needs before this file leaves the browser: it is
    // full admin on the cluster, and nothing here can take it back. The
    // talosconfig beside it is revocable in the sense that matters -- it is
    // minted on demand and is not the credential this installation dials with
    // -- and a kubeconfig is not.
    const link = screen.getByRole('link', { name: /kubeconfig/i })
    expect(link.getAttribute('title')).toMatch(/system:masters/i)
    expect(link.getAttribute('title')).toMatch(/not revocable/i)
  })

  it('escapes a cluster id in the paths it builds', () => {
    // Ids are this product's own and contain nothing exotic today. The
    // encoding is here so that the day one does, the link breaks visibly in
    // this test rather than silently in a browser.
    wrap(<ClusterCard cluster={{ ...cluster, id: 'a/b c' }} />)

    expect(screen.getByRole('link', { name: /support bundle/i })).toHaveAttribute(
      'href',
      '/api/v1/clusters/a%2Fb%20c/support-bundle',
    )
  })
})
