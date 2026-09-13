import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { RenewCertificate } from '@/components/RenewCertificate'

/**
 * The button D-23's ladder has been counting down to.
 *
 * The case worth a test is the refusal, and specifically what it must not look
 * like. The server proves a new certificate against a node before keeping it,
 * so `conflict.certificate-rejected` means the old certificate is still in
 * place and nothing changed — the safe outcome of a renewal rather than a
 * failure of one. A screen that turned it into an alarm would be telling an
 * operator their cluster is broken at the moment this refused to break it.
 */

function wrap(node: ReactNode) {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
  return render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
}

function responds(status: number, body: unknown) {
  return vi.fn((_input: RequestInfo | URL, _init?: RequestInit) =>
    Promise.resolve(
      new Response(JSON.stringify(body), {
        status,
        headers: {
          'Content-Type': status === 200 ? 'application/json' : 'application/problem+json',
        },
      }),
    ),
  )
}

const renewed = {
  id: 'c-1',
  name: 'homelab',
  origin: 'imported',
  endpoint: 'https://10.0.0.1:6443',
  locked: false,
  created_at: '2026-09-01T00:00:00Z',
  client_cert_not_after: '2027-09-13T00:00:00Z',
  client_cert_days_left: 365,
  certificate_warning: '',
  certificate_urgency: 'none',
  nodes: 3,
  control_plane: 3,
  workers: 0,
  healthy: 3,
  degraded: 0,
  down: 0,
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('renewing a cluster certificate', () => {
  it('asks the route that renews it, and says no node was touched', async () => {
    const fetchMock = responds(200, renewed)
    vi.stubGlobal('fetch', fetchMock)

    wrap(<RenewCertificate clusterID="c-1" />)
    await userEvent.click(screen.getByRole('button', { name: /Renew/i }))

    await screen.findByRole('status')
    const [url, init] = fetchMock.mock.calls[0] ?? []
    expect(String(url)).toContain('/api/v1/clusters/c-1/client-certificate')
    expect((init as RequestInit).method).toBe('POST')

    // The one thing an operator needs to believe before pressing this on a
    // cluster they depend on.
    expect(screen.getByRole('status').textContent).toMatch(/no node was touched/i)
  })

  it('reports a rejected certificate as what it is: nothing changed', async () => {
    vi.stubGlobal(
      'fetch',
      responds(409, {
        type: '/problems/conflict',
        title: 'No node accepted the new certificate',
        status: 409,
        code: 'conflict.certificate-rejected',
        detail:
          'inventory: no node accepted the new certificate, so the old one has been kept and ' +
          'nothing has changed.',
      }),
    )

    wrap(<RenewCertificate clusterID="c-1" />)
    await userEvent.click(screen.getByRole('button', { name: /Renew/i }))

    // The server's own sentence, which is the one that says the old
    // certificate is still there.
    await screen.findByText(/nothing has changed/i)

    // And no success line beside it.
    expect(screen.queryByText(/Renewed\./)).toBeNull()
  })
})
