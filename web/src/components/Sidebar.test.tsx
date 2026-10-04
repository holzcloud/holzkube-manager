import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router'
import { act, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api, type Host, hostSchema } from '@/api'
import demo from '../../fixtures/demo.json'
import { BottomNav, NavDrawer, PRIMARY_NAV, RailNav } from './Sidebar'

/**
 * The Host entry's state mark (HOST-04, D-13), in the 2026 shell: the rail and
 * the drawer both carry it, and the rules did not change with the redesign.
 *
 * The failure this guards against is a green dot vouching for a host nobody
 * heard from: the navigation is on every screen, so a mark kept from an
 * earlier answer after the daemon stopped answering would be the one
 * reassurance on screen during exactly the outage it should show (T-12-14).
 * The second is cost: on /host the page already polls every 3 s, and a
 * navigation poll of its own there would shorten the window the page's rates
 * are computed over.
 *
 * Every state here is the server's, written out by the test.
 */
type HealthShape = { state: 'ok' | 'warn' | 'unknown'; summary: string; warnings: string[] }

function hostWith(health: HealthShape): Host {
  const fixtures = demo as Record<string, unknown>
  const raw = structuredClone(fixtures['/api/v1/host']) as Record<string, unknown>
  return hostSchema.parse({ ...raw, health: { ...health, unreadable: [] } })
}

const OK = hostWith({
  state: 'ok',
  summary: 'Temperatures and filesystems are below their thresholds.',
  warnings: [],
})
const WARN = hostWith({
  state: 'warn',
  summary: '1 threshold crossed.',
  warnings: ['cpu_thermal 82.1 °C ≥ 80 °C'],
})
const UNKNOWN = hostWith({
  state: 'unknown',
  summary: 'Temperatures could not be read: permission denied.',
  warnings: [],
})

/** The navigation in a memory router at `path`, with "/" and "/host" as routes. */
function renderAt(path: '/' | '/host') {
  const rootRoute = createRootRoute({
    component: () => (
      <>
        <RailNav />
        <NavDrawer />
        <BottomNav />
        <Outlet />
      </>
    ),
  })
  // The pages render nothing: every call of api.host counted here is the
  // navigation's own, which is what the polling cases measure.
  const home = createRoute({ getParentRoute: () => rootRoute, path: '/', component: () => null })
  const hostPage = createRoute({
    getParentRoute: () => rootRoute,
    path: '/host',
    component: () => null,
  })
  const router = createRouter({
    routeTree: rootRoute.addChildren([home, hostPage]),
    history: createMemoryHistory({ initialEntries: [path] }),
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

/** The Host link in the drawer, once the router has rendered the navigation.
 *
 * The rail's Host button and the drawer's Host link both match /^Host/, so
 * the drawer's own -- the one that carries the mark and the words -- is picked
 * from inside the "Main navigation" list. */
async function hostLink(): Promise<HTMLElement> {
  const nav = await screen.findByRole('navigation', { name: 'Main navigation' })
  return await waitFor(() => {
    const links = Array.from(nav.querySelectorAll<HTMLAnchorElement>('a'))
    const link = links.find((l) => l.textContent?.trim().startsWith('Host'))
    if (link === undefined) throw new Error('the Host link has not rendered yet')
    return link
  })
}

/** The mark inside the Host link, or null. */
function markOf(link: HTMLElement): HTMLElement | null {
  return link.querySelector<HTMLElement>('[data-host-state]')
}

beforeEach(() => {
  // shouldAdvanceTime: the fake clock also runs with the real one, so the
  // router's and testing-library's own waits still finish; the polling cases
  // then jump it forward with advanceTimersByTimeAsync.
  vi.useFakeTimers({ shouldAdvanceTime: true })
  vi.spyOn(api, 'version').mockResolvedValue({ version: '', releases: [] })
})

afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
})

describe('the shell navigation', () => {
  it('carries exactly five primary areas, Wall among the rest', () => {
    expect(PRIMARY_NAV.map((a) => a.path)).toEqual([
      '/',
      '/clusters',
      '/host',
      '/jobs',
      '/settings',
    ])
  })
})

describe('the Host entry in the navigation', () => {
  it('has no mark before the first answer', async () => {
    vi.spyOn(api, 'host').mockReturnValue(new Promise<Host>(() => undefined))
    renderAt('/')

    const link = await hostLink()
    await waitFor(() => expect(api.host).toHaveBeenCalled())
    expect(markOf(link)).toBeNull()
    expect(link).toHaveAccessibleName('Host')
  })

  it('draws the triangle for a warning, says "Host — warning", and titles it with the summary', async () => {
    vi.spyOn(api, 'host').mockResolvedValue(WARN)
    renderAt('/')

    const link = await hostLink()
    await waitFor(() => expect(markOf(link)?.dataset.hostState).toBe('warn'))
    const mark = markOf(link) as HTMLElement
    expect(mark.querySelector('svg')).not.toBeNull()
    expect(mark).toHaveAttribute('title', '1 threshold crossed.')
    expect(link).toHaveAccessibleName('Host — warning')
  })

  it('draws the dot for healthy and the ring for not readable, each with its words', async () => {
    vi.spyOn(api, 'host').mockResolvedValue(OK)
    const first = renderAt('/')
    let link = await hostLink()
    await waitFor(() => expect(markOf(link)?.dataset.hostState).toBe('ok'))
    expect(markOf(link)?.querySelector('.rounded-full:not(.border)')).not.toBeNull()
    expect(link).toHaveAccessibleName('Host — healthy')
    first.unmount()

    vi.spyOn(api, 'host').mockResolvedValue(UNKNOWN)
    renderAt('/')
    link = await hostLink()
    await waitFor(() => expect(markOf(link)?.dataset.hostState).toBe('unknown'))
    expect(markOf(link)?.querySelector('.rounded-full.border')).not.toBeNull()
    expect(link).toHaveAccessibleName('Host — not readable')
    expect(markOf(link)).toHaveAttribute(
      'title',
      'Temperatures could not be read: permission denied.',
    )
  })

  it('after a failed poll draws the ring and says holzkube-manager did not answer, never the earlier green', async () => {
    const host = vi
      .spyOn(api, 'host')
      .mockResolvedValueOnce(OK)
      .mockRejectedValue(new Error('Network down'))
    renderAt('/')

    const link = await hostLink()
    await waitFor(() => expect(markOf(link)?.dataset.hostState).toBe('ok'))

    await act(async () => {
      await vi.advanceTimersByTimeAsync(31_000)
    })
    expect(host).toHaveBeenCalledTimes(2)
    await waitFor(() => expect(markOf(link)?.dataset.hostState).toBe('unanswered'))
    const mark = markOf(link) as HTMLElement
    expect(mark.querySelector('.rounded-full.border')).not.toBeNull()
    expect(mark.querySelector('.rounded-full:not(.border)')).toBeNull()
    expect(link).toHaveAccessibleName('Host — not readable, holzkube-manager did not answer')
  })

  it('asks again after 30 s away from /host', async () => {
    const host = vi.spyOn(api, 'host').mockResolvedValue(OK)
    renderAt('/')
    const link = await hostLink()
    await waitFor(() => expect(markOf(link)?.dataset.hostState).toBe('ok'))
    expect(host).toHaveBeenCalledTimes(1)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(31_000)
    })
    expect(host).toHaveBeenCalledTimes(2)
  })

  it('asks nothing of its own on /host, where the page polls', async () => {
    const host = vi.spyOn(api, 'host').mockResolvedValue(OK)
    renderAt('/host')
    const link = await hostLink()
    await waitFor(() => expect(markOf(link)?.dataset.hostState).toBe('ok'))
    expect(host).toHaveBeenCalledTimes(1)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(31_000)
    })
    expect(host).toHaveBeenCalledTimes(1)
  })
})
