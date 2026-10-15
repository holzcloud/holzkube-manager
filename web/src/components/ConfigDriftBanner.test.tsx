import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, type Cluster } from '@/api'
import { CertificateBanner } from '@/components/CertificateBanner'
import { ConfigDriftBanner } from '@/components/ConfigDriftBanner'

/**
 * The configuration banner on every page (15-09, D-06).
 *
 * Three claims are worth holding. It comes from the server and only from it: a
 * cluster with drifted nodes but no `banner` is not a banner, because the 24
 * hours are the server's to count. It cannot be dismissed: a difference that
 * could be clicked away is the one nobody sees any more. And it asks nothing of
 * the server that the certificate banner does not already ask: both read the
 * cluster list under one query key.
 */

const base: Cluster = {
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
  checking: 0,
}

const SENTENCE =
  'cp-2 has differed from its baseline for more than a day, and so have 1 other node. Look at it, or confirm it as intended.'

function drifting(overrides: Partial<Cluster> = {}, banner = true): Cluster {
  return {
    ...base,
    config_drift: {
      checked_at: '2026-10-12T09:05:00Z',
      nodes: 3,
      read_nodes: 3,
      drifted: 2,
      banner,
      warning: banner ? SENTENCE : '',
      items: [
        { machine: 'm-cp-1', node: 'cp-1', state: 'ok', sentence: 'fine' },
        {
          machine: 'm-cp-3',
          node: 'cp-3',
          state: 'drift',
          since: '2026-10-11T12:00:00Z',
          sentence: 'differs',
        },
        {
          machine: 'm-cp-2',
          node: 'cp-2',
          state: 'drift',
          since: '2026-10-10T07:30:00Z',
          sentence: 'differs',
        },
      ],
    },
    ...overrides,
  }
}

function renderBanner(clusters: Cluster[], withCertificates = false) {
  const list = vi.spyOn(api.clusters, 'list').mockResolvedValue(clusters)
  const rootRoute = createRootRoute({
    component: () => (
      <>
        {withCertificates && <CertificateBanner />}
        <ConfigDriftBanner />
      </>
    ),
  })
  const home = createRoute({ getParentRoute: () => rootRoute, path: '/', component: () => null })
  const node = createRoute({
    getParentRoute: () => rootRoute,
    path: '/nodes/$uuid',
    component: () => null,
  })
  const router = createRouter({
    routeTree: rootRoute.addChildren([home, node]),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const view = render(
    <QueryClientProvider client={client}>
      {/* biome-ignore lint/suspicious/noExplicitAny: the test tree is not the
          registered one, so the router's generated types do not describe it. */}
      <RouterProvider router={router as any} />
    </QueryClientProvider>,
  )
  return { list, view }
}

/** Waits until the cluster list was asked for and its answer was drawn. */
async function settled(list: ReturnType<typeof vi.spyOn>) {
  await waitFor(() => expect(list).toHaveBeenCalled())
  await new Promise((resolve) => setTimeout(resolve, 20))
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('the configuration banner', () => {
  it('comes only from the server: drifted nodes without banner are not a banner', async () => {
    const { list } = renderBanner([drifting({}, false)])
    await settled(list)
    expect(screen.queryByRole('status')).toBeNull()
  })

  it('renders nothing for a cluster without config_drift or with banner false', async () => {
    const { list } = renderBanner([base, drifting({ id: 'c-2' }, false)])
    await settled(list)
    expect(screen.queryByRole('status')).toBeNull()
  })

  it('shows the sentence of the server, names the cluster and leads to the node with the oldest drift', async () => {
    renderBanner([drifting()])

    const banner = await screen.findByRole('status')
    expect(within(banner).getByText(SENTENCE)).toBeInTheDocument()
    expect(banner).toHaveTextContent('homelab')
    const link = within(banner).getByRole('link', { name: 'Open the node' })
    // cp-2 has run since the 10th, cp-3 only since the 11th.
    await waitFor(() => expect(link).toHaveAttribute('href', '/nodes/m-cp-2'))
    expect(link.className).toContain('max-md:min-h-11')
  })

  it('cannot be clicked away: the only thing to click is the link to the node', async () => {
    renderBanner([drifting()])
    const banner = await screen.findByRole('status')

    expect(within(banner).queryAllByRole('button')).toHaveLength(0)
    expect(within(banner).getAllByRole('link')).toHaveLength(1)
    expect(
      banner.querySelectorAll('button, input, select, textarea, [tabindex], [onclick]'),
    ).toHaveLength(0)
    for (const name of [/close/i, /dismiss/i, /acknowledge/i, /hide/i, /^x$/i]) {
      expect(within(banner).queryAllByLabelText(name)).toHaveLength(0)
      expect(within(banner).queryAllByTitle(name)).toHaveLength(0)
    }
  })

  it('shows one banner for each cluster that has one', async () => {
    renderBanner([
      drifting(),
      drifting({ id: 'c-2', name: 'second' }),
      drifting({ id: 'c-3', name: 'third' }, false),
    ])
    await waitFor(() => expect(screen.getAllByRole('status')).toHaveLength(2))
    expect(screen.getByText(/second/)).toBeInTheDocument()
    expect(screen.queryByText(/third/)).toBeNull()
  })

  it('shares the cluster query with the certificate banner and asks the server once', async () => {
    const { list } = renderBanner([drifting()], true)
    await screen.findByRole('status')
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(list).toHaveBeenCalledTimes(1)
  })
})
