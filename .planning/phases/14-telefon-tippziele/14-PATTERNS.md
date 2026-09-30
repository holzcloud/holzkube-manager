# Phase 14: Telefon — Tippziele - Pattern Map

**Mapped:** 2026-09-30
**Files analyzed:** 17
**Analogs found:** 16 / 17

All paths are git-tracked source under `web/`. Line numbers are from HEAD `b45eff3`.

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|---|---|---|---|---|
| `web/src/routeTree.ts` (NEW) | config/module | transform | `web/src/App.tsx:42-76` (the tree itself, moved verbatim) | exact (move) |
| `web/src/App.tsx` (MOD) | provider | — | itself: keep `createRouter` + `Register` (78-87) | exact |
| `web/scripts/layout-routes.json` (NEW) | config | — | `ROUTES` in `web/scripts/layout-audit.mjs:68-103` + `/setup`, `/login` | exact (move) |
| `web/src/layoutRoutes.test.ts` (NEW) | test (jsdom) | transform | `web/src/fixtures.test.ts` (JSON import, one-line-per-problem style) | role-match |
| `web/package.json` `test:layout` (MOD) | config | — | line 16 `"test:layout": "node scripts/layout-audit.mjs"` | exact |
| `web/scripts/layout-audit.mjs` (MOD) | test tooling (Playwright) | request-response / event-driven | itself: `findSmallTargets` 274-300+, `measure()` 564-580, fixture `context.route` ~550-560 | exact |
| `web/fixtures/host-helper-installed.json` (NEW) or key in demo.json | fixture | — | `web/fixtures/demo.json:3341-3358` + shape from `web/src/routes/host.browser.test.tsx:45-54` | exact |
| `web/src/fixtures.test.ts` (MOD) | test | — | its own case at 150-163 ("shows /host with the helper not installed") | exact |
| toast backstop, e.g. `web/src/components/ui/sonner.browser.test.tsx` (NEW) | test (vitest browser) | — | `web/src/components/WhatsNew.browser.test.tsx` | role-match |
| `web/src/components/ui/button.tsx` (MOD) | component | — | its own `default`/`icon` variants (30-39) | exact |
| `web/src/components/ui/dialog.tsx` (MOD) | component | — | `button.tsx` `max-md:` pattern; lines 53, 59-66, 72-76 | exact |
| `web/src/components/ui/dropdown-menu.tsx` (MOD) | component | — | `button.tsx`; Item line 65, CheckboxItem/RadioItem/SubTrigger | exact |
| `web/src/components/ui/select.tsx` (MOD) | component | — | SelectTrigger's `max-md:data-[size=default]:h-11` in the same file | exact |
| `web/src/components/ui/sonner.tsx` (MOD) | component | — | its own `closeButton` 44-45 (`size-6!` suffix form) | exact |
| `web/src/components/Sidebar.tsx`, `WhatsNew.tsx` (MOD) | component | — | `HostActions.tsx:252` (`max-md:h-auto max-md:min-h-11`) | role-match |
| `web/src/components/NodeActions.tsx` (MOD, Reset dialog) | component | — | `web/src/components/NodeSchedulingActions.tsx:84-102` | exact |
| `web/src/components/HostActions.tsx` (MOD, carried 13-UI-REVIEW 1-4) | component | event-driven | itself | exact |
| `README.md`, `docs/guide.md` (MOD, "On a phone" 1621-1656) | docs | — | existing sections | exact |

## Pattern Assignments

### `web/src/routeTree.ts` (NEW) + `web/src/App.tsx`

Move lines 1-~40 imports (route imports only) and `const routeTree = rootRoute.addChildren([...])` (App.tsx:42-76) verbatim, `export`ed. App.tsx keeps:

```tsx
import { createRouter, RouterProvider } from '@tanstack/react-router'
import { routeTree } from '@/routeTree'
const router = createRouter({ routeTree, defaultPreload: 'intent' })
declare module '@tanstack/react-router' { interface Register { router: typeof router } }
```
Name it `routeTree.ts`, not `.gen.ts` (no router-plugin). No side effect in the module (Pitfall 12: run the new test alone first).

### `web/scripts/layout-routes.json` (NEW)

Source list: `layout-audit.mjs:68-103` (25 entries) plus `/setup` (`"when": "before-account"`) and `/login` (`"when": "before-session"`) = 27. Shape per UI-SPEC:
```json
{ "route": "/nodes/$uuid", "path": "/nodes/m-cp-1" }
{ "route": "/kubernetes/apps/$namespace/$kind/$name", "path": "/kubernetes/apps/media/Deployment/jellyfin" }
```
`/kubernetes` overview key is `/kubernetes` (trailing slash trimmed). The explanatory comments now in `ROUTES` (wall, kubernetes ten pages, ledger 153/159) move to the audit's JSON loader comment, since JSON has none.

### `web/src/layoutRoutes.test.ts` (NEW)

**Analog:** `web/src/fixtures.test.ts` — imports JSON directly:
```ts
import { describe, expect, it } from 'vitest'
import demo from '../fixtures/demo.json'   // fixtures.test.ts:32
```
Core: use RESEARCH Pattern 1 verbatim (`createRouter({ routeTree, history: createMemoryHistory(...) })` first, then walk `children` leaves; `expect(problems).toEqual([])`). Messages verbatim from UI-SPEC Copywriting "Route guard" rows. Assumption A1 fallback: `router.getMatchedRoutes(path)`.

### `web/package.json`
```json
"test:layout": "vitest run --project jsdom src/layoutRoutes.test.ts && node scripts/layout-audit.mjs"
```
CI runs `npm --prefix web run test:layout` (`.github/workflows/ci.yml:140-141`); Taskfile `test:layout` (116-125) stays as is.

### `web/scripts/layout-audit.mjs` (MOD)

**Skip block to edit** (lines ~293-296):
```js
for (const el of document.querySelectorAll(SELECTOR)) {
  const style = getComputedStyle(el)
  if (style.visibility === 'hidden' || style.display === 'none') continue
  if (el.getAttribute('aria-hidden') === 'true') continue
  if (el.hasAttribute('disabled')) continue      // DELETE (UI-SPEC "Disabled controls are measured")
```
Change `findSmallTargets = (min)` to take a root (`(min, root = document)` or pass element handle via `locator.evaluate`) and `root.querySelectorAll(SELECTOR)`; return `{ small, measured }` so count uses the same SELECTOR (Pitfall 5; `countTargets` 393-402 is too narrow for menuitem/option).

**Fresh-state/settle pattern to copy for every opener** (`measure()` 564-573):
```js
await page.goto(base + route)
await page.waitForLoadState('networkidle', { timeout: 20_000 }).catch(() => {})
await page.waitForTimeout(400)
```
Then: trigger locator by accessible name → missing = `OPENER` failure; `locator(expect).waitFor({ state: 'visible', timeout: 5000 })`; settle `el.getAnimations({ subtree: true })` → `finished`; measure scoped; close (Escape default; Navigation: positioned click x 370 on "Close the navigation"); `waitFor({state:'hidden'})`.

**Fixture override:** existing `context.route('**/api/v1/**')` fulfill (~555-560, `contentType: 'application/json; charset=utf-8'`, `body: JSON.stringify(body)`); per-opener override via `page.route` (beats context route), `page.unroute` after close.

**OPENERS list:** shape and triggers in RESEARCH "The opener list" + UI-SPEC table (8 openers). Sudo via `/settings` New account (`#new-username`, `#new-password`), never fill `#sudo-password`; never type `#host-action-confirm`.

**Request monitor:** `page.on('response')` attached only per opener; non-GET → `REQUEST` line; 2xx on action path (not `/api/v1/auth/`, `/api/v1/setup`, `.../confirm`) → `EXECUTED` failure.

**LAYOUT_DUMP:** env-var gate like existing `LAYOUT_SHOTS`; at 1280 write JSONL `route, opener, index, tag, name, x, y, width, height` to scratchpad, never committed.

Output lines: copy the existing two-space column format; exact strings in UI-SPEC Copywriting table.

### Host helper-installed fixture + `fixtures.test.ts`

Shape (from `host.browser.test.tsx:45-54`):
```ts
{ order: null, result: { readable: false, reason: { code: 'host-action.no-result', message: 'The helper has recorded nothing yet.' } }, available: true, missing: [], install_commands: [] }
```
Override = `{ ...demo['/api/v1/host'], actions: <that> }`. Test case copies 150-163 style:
```ts
const host = hostSchema.parse(<variant>)
expect(host.actions.available).toBe(true)
```
Default demo entry stays unchanged (pinned by 150-163 and README renderer). Documentation values only.

### Toast backstop browser test (NEW)

**Analog:** `web/src/components/WhatsNew.browser.test.tsx:1-7`:
```tsx
import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
// Imported for its effect: every class measured below is generated by Tailwind
import '@/index.css'
```
Add `page.viewport(390, 844)` (vitest browser), render `<Toaster/>`, `notify.info(...)`, select `[data-close-button]`, assert count ≥ 1 and `getBoundingClientRect()` ≥ 44 × 44 (A4). Comment in the analog's style: why jsdom cannot see it.

### `ui/*` primitives (MOD)

**Model** (`button.tsx:30-39`):
```
default: 'h-8 max-md:h-11 gap-1.5 px-2.5 max-md:px-3.5 ...',
icon: 'size-8 max-md:size-11',
'icon-sm': 'size-7 rounded-[min(var(--radius-md),12px)] ...',   // + max-md:size-11
```
- `xs` + `max-md:h-11 max-md:px-3`; `icon-xs`/`icon-sm`/`icon-lg` + `max-md:size-11`.
- `dialog.tsx:59-66`: `className="absolute top-2 right-2 max-md:top-0 max-md:right-0"`; header (72-76) `'flex flex-col gap-2 max-md:pr-8'`.
- `dropdown-menu.tsx` Item (65), CheckboxItem, RadioItem, SubTrigger; `select.tsx` SelectItem: + `max-md:min-h-11 max-md:py-2`.
- `sonner.tsx:44-45` closeButton: append `max-md:size-11! max-md:top-0! max-md:right-0! max-md:border-transparent!`; verify in built CSS: `grep -c 'max-md\\:size-11\\!' internal/httpapi/dist/assets/index-*.css` (A2).

### Single places

- `Sidebar.tsx` links + `max-md:min-h-11`; `nav` + `max-md:overflow-y-auto`. No Escape handler on drawer.
- `WhatsNew.tsx` version button `max-md:flex max-md:min-h-11 max-md:items-center`; chips `max-md:min-h-11 max-md:px-3`.
- `NodeActions.tsx` Reset dialog — copy `NodeSchedulingActions.tsx:84-88`:
```tsx
<label className="flex items-start gap-2 py-1 text-xs max-md:min-h-11" htmlFor={forceID}>
  <input ... className="mt-0.5 max-md:mt-2.5 max-md:size-6" />
```
  with `max-md:mt-3` (UI-SPEC checker fix; also change `NodeSchedulingActions.tsx:88,102` `mt-2.5`→`mt-3`). Disk rows become `<label>` wrapping checkbox + text.
- Caller-class override precedent: `HostActions.tsx:252` `max-md:h-auto max-md:min-h-11`.

## Shared Patterns

### `max-md:` only, whitespace-bounded literals in `cn()`
**Source:** `web/src/components/ui/button.tsx:30-39`. **Apply to:** every class change. No unprefixed size; never a class flush against `${` (G-13-3 root cause); no `::after` hit areas.

### Exit code read from the command
`./bin/task test:layout; echo "exit=$?"` — never piped. Red checks (a)(b)(c) each restored with `cmp` against `git show HEAD:<file>` (13-11 precedent).

### Public repo
Fixtures, dump files, SUMMARY: documentation values only (192.168.1.10, homeserver, example.com).

## No Analog Found

| File | Role | Data Flow | Reason |
|---|---|---|---|
| OPENERS pass / request monitor inside `layout-audit.mjs` | test tooling | event-driven | The audit has never clicked anything or watched responses; use RESEARCH Patterns 2-3 and the UI-SPEC opener table |

## Metadata

**Analog search scope:** `web/src`, `web/scripts`, `web/fixtures`, `web/package.json`
**Files scanned:** ~12 (plus RESEARCH's verified excerpts)
**Pattern extraction date:** 2026-09-30
