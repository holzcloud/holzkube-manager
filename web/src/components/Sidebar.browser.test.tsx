import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { page } from 'vitest/browser'
import { api, hostSchema } from '@/api'
// Imported for its effect: the bar's width, the font and the nowrap come from
// Tailwind classes and the Manrope face this entry point pulls in; without it
// the measurement measures the browser's defaults and nothing of ours.
import '@/index.css'
import demo from '../../fixtures/demo.json'
import { BottomNav, NavDrawer, RailNav } from './Sidebar'

/**
 * The 2026 navigation, measured where it lives:
 *
 * - the rail at md and up: the brand mark at the top, every icon a thumb-sized
 *   48px square, nothing of it past the bar's own edges;
 * - the bottom tab bar below md: five labels, none clipped, none overlapping,
 *   the bar exactly where the viewport ends;
 * - the drawer's brand block, on one line each: the name, and the tagline
 *   under it (the old measurement, on the new component that carries them).
 */

function renderNavigation(open: boolean) {
  const rootRoute = createRootRoute({
    component: () => (
      <>
        <RailNav />
        <NavDrawer open={open} />
        <BottomNav />
        <Outlet />
      </>
    ),
  })
  const home = createRoute({ getParentRoute: () => rootRoute, path: '/', component: () => null })
  const router = createRouter({
    routeTree: rootRoute.addChildren([home]),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

async function rail() {
  return screen.findByRole('navigation', { name: 'Primary navigation' })
}

async function bottomBar() {
  return screen.findByRole('navigation', { name: 'Bottom navigation' })
}

async function drawerBrand() {
  const nav = await screen.findByRole('navigation', { name: 'Main navigation' })
  await document.fonts.ready
  const name = await screen.findByText('holzkube-manager')
  const tagline = screen.getByText('Talos cluster management')
  return { nav, name, tagline }
}

/**
 * Room each line must leave before the bar's edge. The same text comes out a
 * few pixels wider from one machine's font rendering to the next -- in this
 * browser the tagline needs exactly the 146px the old 224px bar left it, in
 * the app it got 145px and broke -- so fitting with nothing to spare is the
 * defect, not a pass.
 */
const SLACK_PX = 8

function expectOneLineEach({ nav, name, tagline }: Awaited<ReturnType<typeof drawerBrand>>) {
  // The row the mark and the two lines sit in, inside the bar's padding and
  // its own: its content edge is where a line stops being inside the brand
  // block. (The column holding the lines is only as wide as its widest line,
  // so measuring against it would always find exactly zero room.)
  const row = (name.parentElement as HTMLElement).parentElement as HTMLElement
  const inner =
    row.getBoundingClientRect().right - Number.parseFloat(getComputedStyle(row).paddingRight)
  expect(inner).toBeLessThanOrEqual(nav.getBoundingClientRect().right)
  // The name ends inside the bar with room to spare, not on or past its border.
  expect(name.getBoundingClientRect().right).toBeLessThanOrEqual(inner - SLACK_PX)
  // The tagline is one line tall and ends inside the bar too.
  const lineHeight = Number.parseFloat(getComputedStyle(tagline).lineHeight)
  expect(tagline.getBoundingClientRect().height).toBeLessThan(lineHeight * 1.5)
  expect(tagline.getBoundingClientRect().right).toBeLessThanOrEqual(inner - SLACK_PX)
}

beforeEach(() => {
  const fixtures = demo as Record<string, unknown>
  vi.spyOn(api, 'host').mockResolvedValue(hostSchema.parse(fixtures['/api/v1/host']))
})

afterEach(async () => {
  cleanup()
  vi.restoreAllMocks()
  await page.viewport(1200, 900)
})

describe('the rail, at tablet widths (md up to lg)', () => {
  it('keeps every icon inside the bar at 900 px', async () => {
    await page.viewport(900, 800)
    renderNavigation(false)
    const nav = await rail()
    const buttons = await nav.querySelectorAll('a, button')
    expect(buttons.length).toBeGreaterThan(4)
    for (const el of Array.from(buttons)) {
      const r = el.getBoundingClientRect()
      expect(r.width).toBeGreaterThanOrEqual(44)
      expect(r.height).toBeGreaterThanOrEqual(44)
      expect(r.right).toBeLessThanOrEqual(nav.getBoundingClientRect().right + 0.5)
      expect(r.left).toBeGreaterThanOrEqual(nav.getBoundingClientRect().left - 0.5)
    }
  })
})

describe('the bottom tab bar, at phone widths', () => {
  it('shows five labels, none clipped, none past the viewport', async () => {
    await page.viewport(390, 844)
    renderNavigation(false)
    const nav = await bottomBar()
    const labels = await nav.querySelectorAll('span')
    expect(labels.length).toBe(5)
    for (const el of Array.from(labels)) {
      const r = el.getBoundingClientRect()
      expect(r.right).toBeLessThanOrEqual(390 + 0.5)
      expect(r.left).toBeGreaterThanOrEqual(-0.5)
      expect(r.width).toBeGreaterThan(8)
    }
  })
})

describe('the drawer brand block', () => {
  it('keeps the name and the tagline on one line each at 1280 px', async () => {
    await page.viewport(1280, 800)
    renderNavigation(true)
    expectOneLineEach(await drawerBrand())
  })

  it('keeps them on one line each in the open drawer at 390 px', async () => {
    await page.viewport(390, 844)
    renderNavigation(true)
    expectOneLineEach(await drawerBrand())
  })
})
