import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from '@/api'
import { ReachService } from '@/components/ReachService'

/**
 * Reaching a service, from the screen.
 *
 * The test that matters most is the one about markup: a workload's HTML must
 * arrive as text. If it were ever rendered, any pod in the cluster could script
 * the interface holding the operator's session — so this asserts that the script
 * tag is visible as characters rather than present as an element.
 */

function wrap(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
}

const services = [
  {
    namespace: 'default',
    name: 'api',
    type: 'ClusterIP',
    cluster_ip: '10.96.0.12',
    ports: [{ name: 'http', port: 8080, protocol: 'TCP' }],
  },
  {
    namespace: 'kube-system',
    name: 'kube-dns',
    type: 'ClusterIP',
    cluster_ip: '10.96.0.10',
    ports: [
      { name: 'dns', port: 53, protocol: 'UDP' },
      { name: 'metrics', port: 9153, protocol: 'TCP' },
    ],
  },
]

afterEach(() => {
  vi.restoreAllMocks()
})

describe('reaching a service', () => {
  it('sends the service, the port and the path that were chosen', async () => {
    const proxy = vi
      .spyOn(api.kubernetes, 'proxyService')
      .mockResolvedValue({ status: 200, body: 'ok', truncated: false })

    wrap(<ReachService clusterID="c-1" services={services} />)

    await userEvent.click(screen.getByRole('combobox', { name: 'Service' }))
    await userEvent.click(screen.getByRole('option', { name: 'default/api' }))
    await userEvent.click(screen.getByRole('button', { name: 'Fetch' }))

    // The one port a single-port service has is filled in, because making
    // somebody choose from a list of one is a question with one answer.
    expect(proxy).toHaveBeenCalledWith('c-1', 'default', 'api', '8080', '/healthz')
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent(/answered 200/i))
  })

  it('shows a workload’s markup as text and does not render it', async () => {
    vi.spyOn(api.kubernetes, 'proxyService').mockResolvedValue({
      status: 200,
      body: '<script>alert(document.cookie)</script><b>bold</b>',
      truncated: false,
    })

    const { container } = wrap(<ReachService clusterID="c-1" services={services} />)

    await userEvent.click(screen.getByRole('combobox', { name: 'Service' }))
    await userEvent.click(screen.getByRole('option', { name: 'default/api' }))
    await userEvent.click(screen.getByRole('button', { name: 'Fetch' }))

    // Visible as characters...
    await waitFor(() => expect(screen.getByText(/alert\(document\.cookie\)/)).toBeInTheDocument())
    // ...and not present as elements. Either of these would mean a pod's markup
    // had been mounted in this origin.
    expect(container.querySelector('script')).toBeNull()
    expect(container.querySelector('b')).toBeNull()
    expect(container.querySelector('iframe')).toBeNull()
  })

  it('makes a multi-port service’s port an explicit choice', async () => {
    const proxy = vi.spyOn(api.kubernetes, 'proxyService')

    wrap(<ReachService clusterID="c-1" services={services} />)

    await userEvent.click(screen.getByRole('combobox', { name: 'Service' }))
    await userEvent.click(screen.getByRole('option', { name: 'kube-system/kube-dns' }))

    // Two ports, so none is chosen: guessing 53 for a DNS service and reporting
    // what came back would be the product answering a question nobody asked.
    expect(screen.getByRole('button', { name: 'Fetch' })).toBeDisabled()
    expect(proxy).not.toHaveBeenCalled()
  })

  it('does not carry a port over to a service that may not have it', async () => {
    const proxy = vi
      .spyOn(api.kubernetes, 'proxyService')
      .mockResolvedValue({ status: 200, body: 'ok', truncated: false })

    wrap(<ReachService clusterID="c-1" services={services} />)

    await userEvent.click(screen.getByRole('combobox', { name: 'Service' }))
    await userEvent.click(screen.getByRole('option', { name: 'default/api' }))
    await userEvent.click(screen.getByRole('combobox', { name: 'Service' }))
    await userEvent.click(screen.getByRole('option', { name: 'kube-system/kube-dns' }))

    expect(screen.getByRole('button', { name: 'Fetch' })).toBeDisabled()
    expect(proxy).not.toHaveBeenCalled()
  })

  it('says when the body was cut, rather than showing half a page as a whole one', async () => {
    vi.spyOn(api.kubernetes, 'proxyService').mockResolvedValue({
      status: 200,
      body: 'x'.repeat(100),
      truncated: true,
    })

    wrap(<ReachService clusterID="c-1" services={services} />)

    await userEvent.click(screen.getByRole('combobox', { name: 'Service' }))
    await userEvent.click(screen.getByRole('option', { name: 'default/api' }))
    await userEvent.click(screen.getByRole('button', { name: 'Fetch' }))

    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent(/cut at 1 MiB/i))
  })

  it('shows the refusal the server makes about a path', async () => {
    vi.spyOn(api.kubernetes, 'proxyService').mockRejectedValue(
      new Error('"/../secrets" would be rewritten to something else before it was sent'),
    )

    wrap(<ReachService clusterID="c-1" services={services} />)

    await userEvent.click(screen.getByRole('combobox', { name: 'Service' }))
    await userEvent.click(screen.getByRole('option', { name: 'default/api' }))
    await userEvent.click(screen.getByRole('button', { name: 'Fetch' }))

    await waitFor(() => expect(screen.getByText(/would be rewritten/i)).toBeInTheDocument())
  })

  it('says so when the cluster has no services, instead of showing empty boxes', () => {
    wrap(<ReachService clusterID="c-1" services={[]} />)

    expect(screen.getByText(/no services to reach/i)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Fetch' })).toBeNull()
  })
})
