import { type AnyRoute, createMemoryHistory, createRouter } from '@tanstack/react-router'
import { describe, expect, it } from 'vitest'
import { routeTree } from '@/routeTree'
import entries from '../scripts/layout-routes.json'

/**
 * The layout audit measures every route the router has.
 *
 * The audit (web/scripts/layout-audit.mjs) opens the routes listed in
 * web/scripts/layout-routes.json, and this holds that list equal to the
 * router's own leaves. It runs ahead of the browser in `npm run test:layout`,
 * so a route added to the router without an audit entry ends the layout gate
 * before anything is measured.
 *
 * Why this exists: ledger 153 -- three screens the audit had never opened, while
 * it reported a clean run. The list was typed by hand and the router grew
 * beside it. A guard that measures the screens it was told about says nothing
 * about the ones it was not.
 *
 * Why the router is the truth, and not something cheaper: CONTEXT rejected
 * reading the `path:` strings out of the route files with a regex (the pathless
 * authenticated layout, the '/' overview under /kubernetes and the NAV_AREAS
 * placeholders are all things only the router's own init resolves), and it
 * rejected a debug global in the shipped bundle that the audit could read.
 * Building a router here costs nothing and asks the one component that knows.
 *
 * `fullPath` exists only once createRouter has run each route's init; before
 * that every key would read "undefined" and the comparison would be about
 * nothing. The third test holds that.
 */

type Entry = { route: string; path: string; when?: string; note?: string }

const KEYS = new Set(['route', 'path', 'when', 'note'])
const WHENS = new Set(['before-account', 'before-session'])
const LIST = 'web/scripts/layout-routes.json'

/** Leaves, by walking children: routesByPath merges /kubernetes with its '/' overview. */
function leaves(route: AnyRoute): AnyRoute[] {
  const kids = (route.children ?? []) as AnyRoute[]
  return kids.length === 0 ? [route] : kids.flatMap(leaves)
}

/** The overview's fullPath is '/kubernetes/'; the list names it '/kubernetes'. */
const key = (fullPath: string) => (fullPath.length > 1 ? fullPath.replace(/\/$/, '') : fullPath)

describe('the layout audit measures every route the router has', () => {
  // Building the router is what gives each route its fullPath (route.init).
  const router = createRouter({
    routeTree,
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  const inRouter = new Map(leaves(routeTree).map((r) => [key(String(r.fullPath)), r]))
  const list = entries as Entry[]

  it('lists every router leaf once, names none that is gone, and gives each parameter an example', () => {
    const problems: string[] = []
    const listed = new Map<string, Entry>()
    for (const e of list) {
      if (listed.has(e.route)) problems.push(`${e.route} is listed twice in ${LIST}.`)
      listed.set(e.route, e)
      const odd = Object.keys(e).filter((k) => !KEYS.has(k))
      if (e.when !== undefined && !WHENS.has(e.when)) odd.push(`when: ${e.when}`)
      if (odd.length > 0)
        problems.push(
          `${e.route} in ${LIST} has a key or \`when\` the audit does not read: ${odd.join(', ')}.`,
        )
    }
    for (const r of inRouter.keys())
      if (!listed.has(r))
        problems.push(
          `${r} is in the router but not in ${LIST}: the layout audit would never open it.`,
        )
    for (const r of listed.keys())
      if (!inRouter.has(r)) problems.push(`${r} is in ${LIST} but not in the router.`)
    for (const [r, e] of listed)
      if (r.includes('$') && (typeof e.path !== 'string' || e.path === '' || e.path.includes('$')))
        problems.push(`${r} has a parameter but no example path in ${LIST}.`)
    // Every line, not "expected false to be true".
    expect(problems).toEqual([])
  })

  it('opens each example path on the route it is listed for', () => {
    const wrong: string[] = []
    for (const e of list) {
      const leaf = inRouter.get(e.route)
      if (leaf === undefined) continue // reported by the test above
      const last = router.matchRoutes(e.path).at(-1)
      if (last?.routeId !== leaf.id)
        wrong.push(`${e.path} opens ${last?.routeId ?? 'nothing'}, not ${e.route} (${leaf.id}).`)
    }
    expect(wrong).toEqual([])
  })

  it('read the tree it compares against', () => {
    // A guard that saw no tree passes everything: no leaf, nothing missing.
    expect(inRouter.size).toBeGreaterThanOrEqual(20)
    expect([...inRouter.keys()].filter((k) => k === 'undefined' || k === '')).toEqual([])
  })
})
