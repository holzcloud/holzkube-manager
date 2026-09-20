import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from '@/api'
import { LoginPage } from '@/routes/login'

/**
 * The sign-in page on an address that refuses the local account (2026-09-20).
 *
 * It said "the local account works on the local network" and left the operator
 * to remember which address that was. The server derives it from the hosts this
 * instance answers to; this is the half that shows it — and the half that stays
 * quiet when the server has no honest answer, because a guessed address lands on
 * a connection refused and reads as the product being broken.
 */

function statusOf(overrides: Record<string, unknown>) {
  return {
    setup_required: false,
    audit_chain: { ok: true, broken_at_line: 0, file: '' },
    oidc_enabled: true,
    password_login: false,
    local_sign_in_url: '',
    talos_range: '',
    allow_prerelease: false,
    ...overrides,
  } as Awaited<ReturnType<typeof api.status>>
}

function renderLogin() {
  const rootRoute = createRootRoute({ component: Outlet })
  const login = createRoute({
    getParentRoute: () => rootRoute,
    path: '/login',
    validateSearch: () => ({}),
    component: LoginPage,
  })
  const router = createRouter({
    routeTree: rootRoute.addChildren([login]),
    history: createMemoryHistory({ initialEntries: ['/login'] }),
  })
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      {/* biome-ignore lint/suspicious/noExplicitAny: the test tree is not the
          registered one, so the router's generated types do not describe it. */}
      <RouterProvider router={router as any} />
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('the sign-in page on an SSO-only address', () => {
  it('links to the address the local account works on', async () => {
    vi.spyOn(api, 'status').mockResolvedValue(
      statusOf({ local_sign_in_url: 'https://192.168.1.30:8443' }),
    )

    renderLogin()

    const link = await screen.findByRole('link', { name: 'https://192.168.1.30:8443' })
    expect(link).toHaveAttribute('href', 'https://192.168.1.30:8443')
  })

  it('says the sentence without a link when the server has no honest answer', async () => {
    vi.spyOn(api, 'status').mockResolvedValue(statusOf({ local_sign_in_url: '' }))

    renderLogin()

    // The sentence stays — it is still true and still the thing to do.
    expect(await screen.findByText(/local account works on the local network/i)).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /https?:\/\// })).toBeNull()
  })

  it('offers no such link on an address that accepts the local account', async () => {
    vi.spyOn(api, 'status').mockResolvedValue(
      statusOf({ password_login: true, local_sign_in_url: 'https://192.168.1.30:8443' }),
    )

    renderLogin()

    // A link away from a page that already works is an invitation to leave it.
    await screen.findByLabelText(/password/i)
    expect(screen.queryByRole('link', { name: /192\.168\.1\.30/ })).toBeNull()
  })
})
