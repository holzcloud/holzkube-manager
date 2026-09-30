# Phase 14: Telefon — Tippziele, die ein Daumen trifft - Research

**Researched:** 2026-09-30
**Domain:** Tailwind v4 `max-md:` sizing of shadcn/Radix primitives; extending a Playwright layout audit (route list from the TanStack Router tree, opened states, request guard)
**Confidence:** HIGH (every in-repo value below was read this session; three items are [ASSUMED] and listed)
**Where this ran:** the operator's Pi (aarch64). Read-only: no source changed, no audit run, the production service not addressed.

## Summary

The phase is two jobs of very different weight. The sizing job is small and mechanical: `ui/button.tsx` already shows the pattern (`h-8 max-md:h-11`, `size-8 max-md:size-11`), and what is still small at 390 px is concentrated in a handful of primitives that the audit has never seen, because it never opens anything: the dialog close X (`icon-sm`, 28 px), dropdown and select items (`py-1`, about 28 px), the toast's dismiss chip (`size-6!`), and the drawer's navigation links (`py-1.5`, about 32 px). The audit misses them for two reasons. Its route pass skips the closed drawer, which is `max-md:invisible`. And menus and dialogs only exist after a click, which the audit never makes.

The guard job is where the planning risk lies. Six findings from the code change how the openers have to be built:

1. **`fullPath` is undefined until a router is built.** Walking `routeTree` needs `createRouter(...)` first.
2. **Dialogs animate in at `zoom-in-95`.** A 44 px button measured mid-animation reads about 42 px.
3. **The drawer does not close on Escape.** Its backdrop's centre is covered by the drawer, so a plain `click()` on it is intercepted.
4. **`countTargets` does not count `role="menuitem"`/`role="option"`.** An "EMPTY" rule built on it would fail the Power menu while the menu is fine.
5. **Node power routes pass through `ClusterLock` before the sudo gate.** Against the audit's empty data directory they answer 404, never 428. So the sudo dialog has to come from a route without a cluster scope. `POST /api/v1/users` (the "New account" form on `/settings`) is one.
6. **The route guard belongs in the npm script, not only in the Taskfile.** CI runs `npm --prefix web run test:layout` directly.

The safety argument for the audit rests on verified server facts, not on the audit's good behaviour. A login never opens the sudo window, and the sudo gate answers 428 before any destructive handler runs. A host order would also land in the audit daemon's own temporary data directory, which no helper watches. The audit should still never type into `#host-action-confirm`. Since disabled controls are now measured, it does not need to.

**Primary recommendation:** Build it in this order:
1. Move `routeTree` into its own module and add the route guard, chained in the npm `test:layout` script before the browser run.
2. Extend `layout-audit.mjs`: routes read from JSON, disabled controls no longer skipped, openers measured in scope with the animation settled, a request monitor, `LAYOUT_DUMP`.
3. Take the 1280 dump before touching any class, then run the expected-red first measurement.
4. Fix the primitives in `ui/*`, then the named single places, until green.
5. Do the three red checks and the second dump, then the README/guide update.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

#### Routen vollständig (MOB-02, Kriterium 2)

- **D-01:** Die Routenliste zieht aus `layout-audit.mjs` in eine geteilte Datei
  `web/scripts/layout-routes.json`: jede Route des Routers mit einem konkreten
  Pfad; Parameter-Routen (`/nodes/$uuid`, `apps/$namespace/$kind/$name`) mit
  einem Beispiel aus `web/fixtures/demo.json`.
- **D-02:** Ein vitest-Test (`web/src/layoutRoutes.test.ts`) läuft den
  **echten** `routeTree` ab (dafür wird der Baum aus `App.tsx` in ein
  seiteneffektfreies Modul gezogen) und schlägt fehl, wenn eine Router-Route
  in der JSON fehlt, eine JSON-Route im Router nicht existiert oder eine
  Parameter-Route kein Beispiel hat. `task test:layout` führt diesen Test
  **vor** dem Browser-Lauf aus, sodass Kriterium 3 („Route aus der Liste
  genommen → `task test:layout` Exit ≠ 0") am Befehl selbst abgelesen wird.
  Platzhalter-Routen aus `NAV_AREAS` zählen mit; `/setup`, `/login` und
  `/wall` werden wie heute gesondert gemessen und in der JSON als solche
  markiert.

#### Geöffnete Zustände (MOB-02, Kriterium 2)

- **D-03:** Eine ausdrückliche Liste von **Öffnern** im Audit:
  `{ route, name, open: <Schritte>, expect: <Selektor des Geöffneten> }`.
  Mindestens: mobile Navigation („Open the navigation", auf einer Route
  genügt — die Navigation ist überall dieselbe), das Power-Menü auf
  `/nodes/<beispiel>`, der Bestätigungsdialog einer Knotenaktion, der
  Sudo-Dialog, auf `/host` der Aktionsdialog mit Tippfeld (Phase 13). Nach dem
  Öffnen wird dieselbe Tippziel-Messung auf das Geöffnete angewendet.
- **D-04:** Ein Öffner, dessen Auslöser oder dessen `expect` nicht gefunden
  wird, ist **rot**, nie übersprungen („Empty output is not green"). Die
  Anzahl gemessener Elemente je Öffner wird ausgegeben, damit ein leerer Dialog
  auffällt.
- **D-05:** Den Sudo-Dialog öffnet der Wächter über den echten Weg: das
  Audit-Konto hat nach dem Login kein offenes Sudo-Fenster bzw. es wird
  geschlossen, und eine destruktive Aktion gegen den echten Daemon antwortet
  `428 sudo.required` — der Dialog erscheint, ohne dass ein POST je etwas
  ausführt (der Wächter bestätigt nichts, er misst und schließt mit Escape).
  Lässt sich das Fenster nicht zuverlässig schließen, darf eine
  Audit-Fixture-Antwort `428` liefern; das entscheidet der Plan nach einem
  Versuch.

#### Größen setzen, Desktop unverändert (MOB-01)

- **D-06:** Größen wachsen **nur** über `max-md:`-Varianten, wie es
  `ui/button.tsx` schon tut (`h-8 max-md:h-11`, `size-8 max-md:size-11`).
  Korrekturen zuerst in den geteilten Bausteinen (`ui/button`,
  `ui/dropdown-menu`-Items, `ui/dialog`-Schließen, `ui/select`-Trigger,
  `ui/input`), erst dann an einzelnen Stellen.
- **D-07:** Kleine Symbolknöpfe, die optisch klein bleiben sollen, bekommen die
  Tippfläche über Innenabstand oder ein unsichtbares Trefferfeld
  (`max-md:` Pseudo-Element/Padding) — nicht über ein größeres Symbol.
- **D-08:** „Desktop unverändert" wird einmal in der Phase gemessen: der
  Wächter schreibt bei 1280 px Größe und Lage jedes Bedienelements; der Lauf
  vor der ersten Änderung und der nach der letzten werden verglichen und das
  Ergebnis in die SUMMARY geschrieben. Dauerhaft hält es die
  `max-md:`-Disziplin, keine eingecheckte Baseline.
- **D-09:** Die vorhandenen Ausnahmen des Wächters bleiben, wie sie begründet
  sind (verstecktes natives `<select>` hinter Radix, Link im Fließtext nach
  WCAG 2.5.8); neue Ausnahmen nur mit Begründung im Code und einzeln
  aufgezählt.

#### Beweis (Kriterium 3, 4)

- **D-10:** Drei Rot-Checks, einzeln, Exit-Code vom Befehl selbst gelesen (nicht
  durch eine Pipe): (a) ein Element auf unter 44 px zurückgesetzt (z. B.
  `max-md:h-11` am Button entfernt), (b) ein Dialogknopf unter 44 px, (c) eine
  Route aus `layout-routes.json` genommen.
- **D-11:** `/host` mit Aktionen, Bestätigungsdialog und Tippfeld besteht bei
  390 px; dazu ein Handtest auf einem Telefon (Aktion bis zur getippten
  Bestätigung, **ohne** abzusenden), dessen Ergebnis in der SUMMARY steht — ist
  kein Telefon erreichbar, steht das dort als nicht ausgeführt, nicht als
  bestanden.

### Claude's Discretion

- Die genaue Form der Öffner-Liste und der Ausgabe.
- Welche Bausteine zuerst angepasst werden — Reihenfolge ergibt sich aus der
  ersten roten Messung des erweiterten Wächters.
- Ob `task test:layout` die vitest-Prüfung als eigenen Task-Schritt oder über
  ein npm-Skript aufruft.

### Ohne Rückfrage entschieden (from CONTEXT.md, verbatim)

Gewählt wurde jeweils die empfohlene Antwort; verworfen:

- *Routen per Regex aus den Quelldateien lesen* — verworfen, eine Liste aus
  `path: '…'`-Treffern sieht Kind-Routen und `NAV_AREAS`-Platzhalter falsch
  und scheitert still; der echte `routeTree` ist die Wahrheit.
- *Router zur Laufzeit über ein `window`-Global auslesen* — verworfen, das
  hieße ein Debug-Global im ausgelieferten Bundle.
- *Automatisch jeden Knopf anklicken, um Menüs/Dialoge zu finden* — verworfen,
  klickt auch destruktive Aktionen an und ist nicht deterministisch; eine
  ausdrückliche Öffner-Liste, die rot wird, wenn ein Öffner nichts findet, ist
  prüfbar.
- *Eingecheckte Desktop-Baseline als Dauer-Wächter* — verworfen, jede
  gewollte Desktop-Änderung müsste sie neu schreiben, und eine Baseline, die
  man ohne Lesen neu erzeugt, bewacht nichts.
- *Tabellen auf dem Telefon als Karten* — verworfen vom Betreiber (2026-09-17).
- *Größere Tippziele auch auf dem Desktop* — verworfen, Entscheidung des
  Betreibers vom 2026-09-17 (kompakter Desktop).

### Deferred Ideas (OUT OF SCOPE)

- Weitere Breiten (z. B. 360 px, Tablet 768 px).
- Messung auf der Wand bei Fernseh-Auflösung als Wächter (heute nur Bild).
- Tabellen als Karten — vom Betreiber verworfen, nur der Vollständigkeit halber.

### Binding from the approved 14-UI-SPEC.md (summarised; the spec is the authority)

- No pseudo-element hit areas. The box itself grows (`min-h`, `size`, padding), because the audit measures `getBoundingClientRect()`. UI-SPEC "How a target grows" rule 4, which narrows D-07.
- Dialog close X: `max-md:size-11` via `icon-sm`, plus `max-md:top-0 max-md:right-0`. `DialogHeader` gets `max-md:pr-8`. Toast close: `max-md:size-11! max-md:top-0! max-md:right-0! max-md:border-transparent!`.
- The `findSmallTargets` skip for `disabled` is removed.
- Measurement inside an opener is scoped to the `expect` element.
- Zero measured controls in an opener is red (`EMPTY`).
- Any non-GET during an opener is printed. A 2xx on an action path is `EXECUTED` and fails the run.
- "Carried from 13-UI-REVIEW" items 1–4 (focus on cancel, open dialog follows the reason line, "up since ." clause, minors) are **binding for the planner in this phase**.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| MOB-01 | At 390 px every control has a tap area of at least 44 × 44 px (below `md`; desktop unchanged) | Primitive inventory with verified class strings (§Code Examples). `max-md:` is the only lever (Tailwind default `md` = 48rem). The D-08 dump design and its pitfalls (animation, relative times) |
| MOB-02 | A guard in the layout check measures tap-target sizes on all routes and goes red as soon as one falls below | Route guard via `createRouter` + tree walk (Pattern 1). Openers with scoped measurement, settled animations and Escape/backdrop close (Pattern 2). Sudo via `POST /api/v1/users` (Pattern 3). Request monitor. npm-script chaining so CI gets the guard too |
| MOB-03 | The new host page meets MOB-01 and can be used on a phone | Helper-installed override via `page.route` (page routes beat context routes, CITED). Disabled buttons measured in the helper-missing state. Never type into `#host-action-confirm`. Prerequisites and blockers of the hand test (Open Question 2) |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **Public repository:** nothing identifying the operator's installation goes into tests, fixtures, the helper-installed override, screenshots, the SUMMARY or commit messages. Use documentation values (192.168.1.10, homeserver, example.com). `internal/publicrepo` fails the gate on known values.
- **README rule:** a feature updates the README in the same change. Screenshots are rendered with `task build && node web/scripts/readme-images.mjs`, never taken from the real cluster. Depth goes into `docs/guide.md`. No release happens in this phase, so no changelog entry is needed unless one is cut.
- **Alpha stays declared.** Nothing here touches `Alpha.tsx`.
- **Where it ran, said in the SUMMARY:** the Pi (aarch64). `go test -race` is not possible there (CI only). Replacing, restarting or copying the production service is the operator's call.
- **Guards seen red against the reinstated fault**, with the exit code read from the command itself and never through a pipe (memory: "Empty output is not green").
- **Every question to the operator is a choice** via `AskUserQuestion` (2–4 named options, recommended first).
- **Release only on request.** Run `./bin/task ci` locally before any push: there are no push-CI minutes since 2026-09-28.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Tap-target sizes below `md` | Browser / Client (Tailwind classes in `web/src/components/ui/*`) | — | Purely presentational. A `max-md:` media variant keeps 1280 unchanged by construction |
| Route list = router leaves | Build/test tooling (vitest jsdom, `web/src/layoutRoutes.test.ts`) | Client (the `routeTree` module) | The router is the truth (D-02). The test imports it; the bundle carries no debug global |
| Opened-state measurement | Test tooling (Playwright in `web/scripts/layout-audit.mjs`) | Client (Radix portals) | Needs a real browser, a real session and the whole shell |
| Sudo 428 that opens the prompt | API / Backend (sudo middleware, innermost in `wrapRoute`) | Client (`api.ts` 428 interceptor → `SudoDialog`) | The server answers before the handler, so the audit cannot execute anything through this path |
| Fixture data (GET only) | Test tooling (Playwright `context.route` / `page.route`) | — | Data is replaced. Session, setup and login are the real daemon's |

## Standard Stack

No new dependency. Everything needed is installed and pinned in `web/package.json` [VERIFIED: web/package.json read this session]:

| Library | Version (pinned) | Purpose in this phase |
|---------|------------------|-----------------------|
| playwright | 1.62.1 | the audit's browser driver; Chromium build `1234` (151.0.7922.34) is installed on the Pi [VERIFIED: `web/node_modules/playwright-core/browsers.json`, `~/.cache/ms-playwright/chromium-1234`] |
| @tanstack/react-router | 1.170.32 (resolves `@tanstack/router-core` 1.171.27) | `createRouter`, `createMemoryHistory` for the route guard [VERIFIED: node_modules package.json] |
| vitest | 4.1.11 (projects `jsdom` and `browser`) | the route guard (jsdom) and the toast backstop (browser, `page.viewport`) |
| tailwindcss / @tailwindcss/vite | 4.3.3 | `max-md:` = `@media (width < 48rem)`, default breakpoints |
| tailwind-merge | 3.6.0 | `cn()` in the primitives. Variant-prefixed classes do not conflict with unprefixed ones |
| sonner | ^2.0.8 | toast close via `toastOptions.classNames.closeButton` |

**Installation:** none.

## Package Legitimacy Audit

Not applicable: this phase installs no external package. No `npm install`, no `go get`.

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| (none) | — | — | — | — | — | — |

**Packages removed due to [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

## Architecture Patterns

### System Architecture Diagram

```
 ./bin/task test:layout  (deps: build:go → build:web)
        │
        ▼
 npm --prefix web run test:layout          ◄── CI runs exactly this line (.github/workflows/ci.yml:140-141)
        │
        ├─(1) vitest run --project jsdom src/layoutRoutes.test.ts
        │        │ import { routeTree } from '@/routeTree'  (side-effect-free module)
        │        │ createRouter({ routeTree, history: createMemoryHistory() })  → init() fills fullPath
        │        │ walk leaves ──► compare with web/scripts/layout-routes.json
        │        └─ mismatch? ──► print one line per mismatch, exit 1  ──► STOP (browser never starts)
        │
        └─(2) node scripts/layout-audit.mjs
                 │ builds bundle + daemon, spawns daemon (temp data dir, --insecure-http, random port)
                 │ context.route('**/api/v1/**'): GET → demo.json by pathname; non-GET → real daemon
                 │
                 ├─ /setup (no account) ─► POST /api/v1/setup ─► /login (no session) ─► login
                 │
                 ├─ ROUTE PASS: for each JSON entry without `when`, at 390 and 1280
                 │     measure(document): CLIPPED / SIDEWAYS / OFFSCREEN / SMALL (390 only)
                 │     LAYOUT_DUMP set & width 1280 → one JSON line per control
                 │
                 └─ OPENER PASS: for each opener × its widths (fresh page.goto per opener)
                       optional page.route override (e.g. /api/v1/host helper installed)
                       response monitor ON (non-GET → REQUEST line; 2xx on action path → EXECUTED)
                       open steps ─► trigger missing? OPENER red
                       wait expect visible ≤5 s ─► not visible? OPENER red
                       wait until animations inside expect have finished
                       measure(scope = expect): SMALL / CLIPPED / sideways; count(scope) = 0 → EMPTY red
                       close (Escape, or backdrop click for the drawer) ─► still there? OPENER red
                 │
                 ▼
        failures > 0 → exit 1, else the green summary line with N routes and M opened states
```

### Recommended file layout

```
web/
├── scripts/
│   ├── layout-audit.mjs        # reads routes from JSON; OPENERS list; scoped measure; dump; request monitor
│   └── layout-routes.json      # NEW: one entry per router leaf ({route, path, when?})
├── src/
│   ├── routeTree.ts            # NEW: `export const routeTree = rootRoute.addChildren([...])` moved from App.tsx
│   ├── App.tsx                 # keeps createRouter + `declare module ... Register` + <RouterProvider>
│   ├── layoutRoutes.test.ts    # NEW: the route guard (jsdom project)
│   ├── fixtures.test.ts        # + parse the helper-installed host variant with hostSchema
│   └── components/ui/*.tsx     # the max-md: fixes
└── package.json                # "test:layout": "vitest run --project jsdom src/layoutRoutes.test.ts && node scripts/layout-audit.mjs"
```

Name the module `routeTree.ts`, **not** `routeTree.gen.ts`. There is no `@tanstack/router-plugin` in this project, and `.gen` is that generator's convention. [VERIFIED: web/package.json has no router-plugin]

### Pattern 1: The route guard walks the real tree after `createRouter`

**What:** `route.fullPath` is a getter over `_fullPath`, and that field is set only in `route.init()`, which `createRouter` calls through `buildRouteTree()`. Before a router exists, every `fullPath` is `undefined`.
[VERIFIED: `web/node_modules/@tanstack/router-core/dist/esm/route.js:17-19` (`get fullPath() { return this._fullPath; }`) and `:37-41` (`const fullPath = id === "__root__" ? "/" : joinPaths([this.parentRoute.fullPath, path]); … this._fullPath = fullPath;`), and `router.js:159-163` (`this.buildRouteTree = () => { const result = processRouteTree(this.routeTree, …, (route, i) => { route.init({ originalIndex: i }); });`)]

**Leaves, not `routesByPath`:** `routesByPath` merges the `/kubernetes` layout and its `'/'` overview child under one trimmed key, and the overview (fullPath `/kubernetes/`) wins. Walk `children` and take the routes whose `children` is empty or absent.
[VERIFIED: `new-process-route-tree.js:343-345`: `const trimmedFullPath = trimPathRight(route.fullPath); if (!routesByPath[trimmedFullPath] || route.fullPath.endsWith("/")) routesByPath[trimmedFullPath] = route;`]

**The current tree's leaves** [VERIFIED: `web/src/App.tsx:42-76`]: `setupRoute`, `loginRoute`, `wallRoute`; under the pathless `authenticatedRoute` (`id: 'authenticated'`, `__root.tsx:76-80`): `indexRoute`, `auditRoute`, `imagesRoute`, `nodesRoute`, `nodeDetailRoute`, `hostRoute`, `clustersRoute`; the 12 children of `kubernetesRoute`; then `jobsRoute`, `configRoute`, `provisionRoute`, `upgradesRoute`, `settingsRoute`, `...placeholderRoutes`. `placeholderRoutes` is **empty today**: every `NAV_AREAS` entry has `phase: null` [VERIFIED: `web/src/components/Sidebar.tsx:54-157`; `placeholders.tsx:15` filters `area.phase !== null`]. That makes 27 leaves. The audit's hand list has 25 routes plus `/setup` and `/login`, also 27 [VERIFIED: `layout-audit.mjs:68-103`]. **The first guard run should therefore pass.** It is red check (c) that proves the guard.

**Example:**

```typescript
// web/src/layoutRoutes.test.ts  (jsdom project)
import { createMemoryHistory, createRouter, type AnyRoute } from '@tanstack/react-router'
import { describe, expect, it } from 'vitest'
import { routeTree } from '@/routeTree'
import entries from '../scripts/layout-routes.json'   // same pattern fixtures.test.ts uses for ../fixtures/demo.json

type Entry = { route: string; path: string; when?: 'before-account' | 'before-session' }

function leaves(route: AnyRoute): AnyRoute[] {
  const kids = (route.children ?? []) as AnyRoute[]
  return kids.length === 0 ? [route] : kids.flatMap(leaves)
}
const key = (fullPath: string) => (fullPath.length > 1 ? fullPath.replace(/\/$/, '') : fullPath)

describe('the layout audit measures every route the router has', () => {
  // Building the router is what gives each route its fullPath (route.init).
  const router = createRouter({ routeTree, history: createMemoryHistory({ initialEntries: ['/'] }) })
  const inRouter = new Map(leaves(routeTree).map((r) => [key(r.fullPath), r]))
  const listed = new Map((entries as Entry[]).map((e) => [e.route, e]))

  it('lists no route twice, misses none and names none that is gone', () => {
    const problems: string[] = []
    for (const r of inRouter.keys())
      if (!listed.has(r)) problems.push(`${r} is in the router but not in web/scripts/layout-routes.json: the layout audit would never open it.`)
    for (const r of listed.keys())
      if (!inRouter.has(r)) problems.push(`${r} is in web/scripts/layout-routes.json but not in the router.`)
    for (const [r, e] of listed)
      if (r.includes('$') && (e.path === '' || e.path.includes('$')))
        problems.push(`${r} has a parameter but no example path in web/scripts/layout-routes.json.`)
    expect(problems).toEqual([])          // prints every line, not "expected true"
  })

  it('opens each example path on the route it is listed for', () => {
    for (const [r, e] of listed) {
      const last = router.matchRoutes(e.path).at(-1)
      expect(last?.routeId, e.path).toBe(inRouter.get(r)?.id)
    }
  })
})
```

The copy of the three messages is verbatim from 14-UI-SPEC "Copywriting Contract". `router.matchRoutes(pathname)` exists on the router [VERIFIED: `router.js:237-243`]. That a match's `.routeId` equals the leaf's `.id` is [ASSUMED] (A1). The fallback is `router.getMatchedRoutes(path)[2]?.id`, which returns `[branch, rawParams, route]` [VERIFIED: `router.js:244-253`].

### Pattern 2: An opener is measured scoped, settled, and closed

**What:** the same passes, run on `expect` rather than on `document`. The in-page functions (`findSmallTargets`, `findClipped`, `findSidewaysPanes`, count) take a root, either `document` or the `expect` element. Playwright passes an `ElementHandle` into `page.evaluate` / `locator.evaluate` as the element.

**Four facts decide how this has to be written:**

1. **Animations.** `DialogContent` carries `duration-100 … data-open:animate-in data-open:fade-in-0 data-open:zoom-in-95` [VERIFIED: `web/src/components/ui/dialog.tsx:53`], and `DropdownMenuContent` carries `data-open:zoom-in-95` plus a slide [VERIFIED: `dropdown-menu.tsx:37`]. `getBoundingClientRect()` includes transforms, so a 44 px button measured at the start of the animation reads 41.8 px. Wait for the animations to finish before measuring: `await expectLocator.evaluate(el => Promise.all(el.getAnimations({ subtree: true }).map(a => a.finished)))`. The drawer is a CSS transition (`max-md:transition-transform max-md:duration-200`) [VERIFIED: `Sidebar.tsx:207`], and `getAnimations()` covers CSS transitions too. Also require `nav.getBoundingClientRect().left === 0`.
2. **The drawer is not a Radix dialog.** Nothing closes it on Escape. The only closers are the backdrop button, a link, and the header [VERIFIED: `AppShell.tsx:81-88`: `aria-label="Close the navigation"`, `className="fixed inset-0 z-40 bg-background/70 md:hidden"`, and no key handler anywhere in AppShell]. The backdrop covers the whole viewport, but the drawer (`max-md:z-50`, `w-56` = 224 px, from the left edge) sits on top of the backdrop's centre (195, 422). Playwright's `click()` on the backdrop would therefore fail its actionability check ("element intercepts pointer events"). Give each opener a `close` step, defaulting to `Escape`. The Navigation opener closes with `page.getByRole('button', { name: 'Close the navigation' }).click({ position: { x: 370, y: 400 } })` and then expects the nav to be hidden again (it goes back to `visibility: hidden`, which `toBeHidden`/`waitFor({state:'hidden'})` recognises). **Do not add an Escape handler to the drawer in this phase.** That would be a behaviour change beyond tap targets ("kein Umbau über die Tippziele hinaus").
3. **Counting.** `countTargets` selects only `'button,a[href],summary,input,select,textarea,[role="button"]'` [VERIFIED: `layout-audit.mjs:393-402`]. Radix menu items are `div[role="menuitem"]` and select options are `div[role="option"]`. A Power menu or Select list would count 0 and be falsely `EMPTY`. The opener count has to use `findSmallTargets`' SELECTOR [VERIFIED: `layout-audit.mjs:275-290`, which includes `'[role="menuitem"]'`, `'[role="option"]'`, `'[role="checkbox"]'`]. Best: `findSmallTargets` returns `{ small, measured }` so both come from one loop.
4. **Radix leaves the page in the DOM behind a modal** (and marks outside siblings `aria-hidden`). The skip `el.getAttribute('aria-hidden') === 'true'` only looks at the element itself [VERIFIED: `layout-audit.mjs:295`]. So a document-wide pass while a dialog is open re-measures the page. This is the reason the spec scopes measurement to `expect`, and the scoping must be real (`root.querySelectorAll`), not a filter applied afterwards.

**Fresh state per opener:** start every opener with `page.goto(base + route)` + `networkidle` + 400 ms, as `measure()` does [VERIFIED: `layout-audit.mjs:564-573`]. After What's new at 390 the drawer is still open, and after the sudo opener a Problem is showing. State must not leak into the next opener.

**Disabled:** delete line 296 `if (el.hasAttribute('disabled')) continue` [VERIFIED: `layout-audit.mjs:296`]. The removal also applies to the route pass. Disabled `<button>`s anywhere (for example the four `/host` buttons in the helper-missing fixture) become measured, which is intended. Expect new SMALL lines from them in the first red run.

### Pattern 3: The sudo opener through a route without a cluster scope

**Verified chain:**
- A login never opens the window. Only `POST /api/v1/auth/sudo` (and the OIDC re-auth) calls `OpenSudoWindow` [VERIFIED: `web/src/api.ts:24-26`: "Note that a fresh login does NOT open the sudo window: only POST /api/v1/auth/sudo does."; `internal/auth/sudo.go:23-26`; `OpenSudoWindow(` is called only at `handlers/auth.go:251` and `handlers/oidc.go:489`].
- Per-route chain, outermost first: `csrf -> authn -> audit -> sudo` [VERIFIED: `internal/httpapi/router.go:402-403`], with Authz and **ClusterLock between audit and sudo** [VERIFIED: `router.go:447-483`]. ClusterLock answers `NotFound("notfound.cluster", …)` for a cluster that does not exist [VERIFIED: `router.go:476-480`].
- **Node power is the wrong trigger.** `POST /api/v1/machines/{id}/power/<a>` has `Destructive: a.Sudo()` (only the two forced actions) and `ClusterScope: machineClusterFromPath(d)` [VERIFIED: `handlers/power.go:42-46, 91-99`]. The audit daemon's data directory is empty, so the lock link answers before sudo, and a non-forced action is not behind sudo at all. Pressing a Power confirmation would send a request to the handler. **Never press confirm on the Power confirmation opener.**
- **`POST /api/v1/users` is the right trigger.** It is `MinRole: model.RoleAdmin, Destructive: true, Action: "user.create"` and has no `ClusterScope` [VERIFIED: `internal/httpapi/handlers/users.go:39-47`]. The audit account is the setup account, which is admin. The UI routes it through the sudo dialog: "Every mutation here is destructive, so every one of them goes through the existing sudo dialog." [VERIFIED: `web/src/routes/accounts.tsx:35-36`]. The form fields are `#new-username`, `#new-password`, `#new-role` [VERIFIED: grep of `accounts.tsx` lines 347, 357, 369]. The client maps any `sudo.` code to `'sudo-prompt'` [VERIFIED: `web/src/lib/problem.ts:106`: `['sudo.', 'sudo-prompt']`].
- Opener: on `/settings`, fill `#new-username` / `#new-password` with throwaway values, submit. The monitor prints `REQUEST POST /api/v1/users -> 428`. Expect `[role="dialog"]` with the title "Confirm your password" [VERIFIED: `SudoDialog.tsx:136-138`]. Measure, Escape (`onOpenChange(false)` → `settle(false)`, `SudoDialog.tsx:128-132`). **Never fill `#sudo-password`.** The audit knows the account password, and submitting it would open the window for the rest of that context.
- The spec says "The audit never presses a confirm button". Read that as: never a confirm inside a *confirmation* dialog, and never the sudo dialog. Submitting the New-account form is how the 428 is produced, and the `EXECUTED` rule (a 2xx on `POST /api/v1/users`) is the proof that nothing ran. Record this reading in the plan (Open Question 1).
- D-05 fallback, if the real path turns out flaky: `page.route('**/api/v1/users', r => r.request().method() === 'POST' ? r.fulfill({ status: 428, contentType: 'application/problem+json', body: <the server's own 428 body> }) : r.fallback())`. Take the body from a real 428 observed once and printed by the monitor, never hand-written.

### Pattern 4: The `/host` helper-installed override

- The default fixture is helper-**missing** (`"available": false`, `missing` = script + path-unit) [VERIFIED: `web/fixtures/demo.json:3341-3358`]. `fixtures.test.ts:150-163` **pins** that ("shows /host with the helper not installed, the widest the page gets"). The default entry must not change, because the route pass and the README picture depend on it.
- The override goes through `page.route`, installed before `page.goto`. "Page routes (set up with page.route()) take precedence over browser context routes when request matches both handlers." [CITED: https://playwright.dev/docs/api/class-browsercontext#browser-context-route]. `/host` polls every 3 s, so the override has to stay in place for the whole opener, and `page.unroute` comes after the close.
- Shape to use, from the existing browser test [VERIFIED: `web/src/routes/host.browser.test.tsx:45-54`]:
  `{ order: null, result: { readable: false, reason: { code: 'host-action.no-result', message: 'The helper has recorded nothing yet.' } }, available: true, missing: [], install_commands: [] }`. The override is `{ ...FIXTURES['/api/v1/host'], actions: <that> }`.
- Hold the variant to `hostSchema` in `fixtures.test.ts`. Keep it in a sibling file (`web/fixtures/host-helper-installed.json`) or a non-path key in `demo.json`, so the `.mjs` audit and the zod test read the same bytes. A variant defined only inside the `.mjs` would be the one fixture no schema checks, which is the ledger-149 shape.
- The buttons are enabled when `!container && actions.available && role ≥ operator && hostname readable && !pollFailed && no order` [VERIFIED: `HostActions.tsx:171-204`]. The admin audit account meets the role rule.
- **Never type into `#host-action-confirm`.** The confirm button is measured while disabled, which is allowed since the disabled skip is gone. The action flow is `api.hostActions.confirm(action, typed)` then `place(action, token)` [VERIFIED: `HostActions.tsx:303-304`]. It is layered safe even so: the confirm route answers 409 when the helper is missing on the audit machine, the place route is sudo-gated, and an order lands "in the data directory" [VERIFIED: `handlers/host.go:44-47, 81-93`], which is the audit's temporary directory and not one a helper watches. Not typing removes the whole chain. 13-11 found the helper absent on the Pi (`/usr/local/sbin/holzkube-manager-host` absent), but that is the operator's to change.

### Anti-Patterns to Avoid

- **An unprefixed size, padding or position class** in any primitive. It changes 1280. Every added class starts with `max-md:`.
- **A class written flush against `${`.** Tailwind's scanner cannot read it, so no rule is generated. That was the root cause of G-13-3 (13-11-SUMMARY). Every class goes into `cn()` as a whitespace-bounded literal.
- **A `::after` hit area.** It is invisible to `getBoundingClientRect()`, so it would need an exemption, which D-09 forbids. (UI-SPEC rule 4.)
- **Measuring the whole document while a modal is open** (Radix leaves the page in place).
- **Measuring before `a.finished`** (zoom-in-95).
- **Reading "green" from a pipe or filtered output:** `./bin/task test:layout; echo "exit=$?"`, never `| tail`.
- **A per-opener fixture the zod schemas never see.**

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Knowing which routes exist | a regex over `path: '…'` (explicitly rejected in CONTEXT) | `createRouter` + walking `routeTree.children` | Kind routes, the pathless layout, `NAV_AREAS` placeholders and the `'/'` overview under `/kubernetes` are all handled by the router's own `init()` |
| Matching an example path to its route | string comparison with `$param` replaced | `router.matchRoutes(path)` | Parameter and splat semantics stay the router's |
| Waiting for "opened" | fixed `waitForTimeout` | `locator.waitFor({ state: 'visible', timeout: 5000 })` + `getAnimations().finished` | A fixed wait is the Pi flake the audit already records (`layout-audit.mjs:566-572`) |
| Per-opener data | editing the shared `demo.json` entry | `page.route` override (beats the context route) | The default fixture is pinned by tests and by the README renderer |
| Toast measurement | a harmless trigger in the audit | a vitest **browser** test at `page.viewport(390, 844)` that renders `<Toaster/>` and calls `notify.info(...)` | UI-SPEC backstop. There is no side-effect-free way to raise a toast in the running product |

## Common Pitfalls

### Pitfall 1: `fullPath` is undefined in the route guard
**What goes wrong:** the test walks `routeTree` without building a router. Every key is `undefined`, and the guard reports every route as missing, or with a careless comparison, none.
**How to avoid:** `createRouter({ routeTree, history: createMemoryHistory(...) })` first (Pattern 1).
**Warning sign:** keys like `"undefined"` in the failure list.

### Pitfall 2: The guard lives only in the Taskfile
**What goes wrong:** CI's step runs `npm --prefix web run test:layout` [VERIFIED: `.github/workflows/ci.yml:140-141`], so a Taskfile-only step leaves CI unguarded.
**How to avoid:** chain it in the npm script: `vitest run --project jsdom src/layoutRoutes.test.ts && node scripts/layout-audit.mjs`. `&&` stops before the browser (D-10c: "browser run not reached"). The Taskfile target keeps `deps: ["build:go"]` and its one `npm --prefix web run test:layout` line [VERIFIED: `Taskfile.yml:116-125`].

### Pitfall 3: Measuring mid-animation
**What goes wrong:** a 44 px button reads 41.8 px during `zoom-in-95`. The run is red on the Pi and green on a fast machine, or the reverse.
**How to avoid:** await `getAnimations({ subtree: true })` → `finished` on the `expect` element.

### Pitfall 4: The drawer never closes
**What goes wrong:** Escape does nothing, and `click()` on the backdrop times out because the drawer covers its centre.
**How to avoid:** a `close` step per opener. The drawer's is a positioned click at x 370 on "Close the navigation".

### Pitfall 5: False `EMPTY` on menus and lists
**What goes wrong:** the count uses the narrow `countTargets` selector.
**How to avoid:** one SELECTOR for measuring and counting.

### Pitfall 6: The sudo opener does not get 428
**What goes wrong:** a node power action is used; ClusterLock answers 404 for the empty inventory, or a non-forced action is not sudo-gated at all.
**How to avoid:** `POST /api/v1/users` from `/settings` (Pattern 3). The monitor line `REQUEST POST /api/v1/users -> 428` is the evidence.

### Pitfall 7: The request monitor flags login as `EXECUTED`
**What goes wrong:** the monitor is on during setup and login (`POST /api/v1/setup`, `POST /api/v1/auth/login`, both 2xx).
**How to avoid:** attach it only for the span of each opener, and additionally allowlist `/api/v1/auth/` and `/api/v1/setup`. The spec's "not a confirm/token path" also covers `POST /api/v1/host/confirm` and `/api/v1/machines/{id}/confirm`. These issue tokens but still count as `REQUEST`.

### Pitfall 8: The drawer overflows at 390 × 844 once links are 44 px
**Numbers:** 13 NAV_AREAS entries [VERIFIED: `Sidebar.tsx:54-157`; the UI-SPEC's "Fourteen" is off by one], 13 × 44 + 12 × 4 gap = 620 px, plus the brand block, the What's-new button (44 once grown), the alpha notice, the source notice and 24 px padding. That lands at roughly 844 px, so it is borderline.
**How to avoid:** `max-md:overflow-y-auto` on the `nav` (UI-SPEC), and the Navigation opener's `CUT OFF` check (`scrollHeight > clientHeight` and `overflow-y` not `auto`/`scroll`).

### Pitfall 9: The D-08 dump differs for reasons that are not layout
**What goes wrong:** relative times ("placed 10:31", "3 min ago", `toLocaleTimeString`) change the accessible name between runs, and live `/host` values change widths. The two runs then "differ" with no class changed.
**How to avoid:** compare `route · opener · index · tag · x, y, width, height`. Keep the name as information only. Run both dumps on the same machine with the same fixture. `/wall` refreshes `generated_at` to `now` by design [VERIFIED: `layout-audit.mjs:550-552`]. If a box differs, read it rather than regenerate it.

### Pitfall 10: The `!` important modifier together with a variant
**What goes wrong:** `max-md:size-11!` is written but no rule is generated. The Tailwind docs show the trailing `!` [CITED: tailwindcss.com/docs/styling-with-utility-classes, "add `!` to the end of the class name"] but not combined with a variant, so the combination is [ASSUMED] (A2).
**How to avoid:** after `./bin/task build:web`, run `grep -c 'max-md\\:size-11\\!' internal/httpapi/dist/assets/index-*.css` and expect ≥ 1. 13-11 used the same built-CSS check.

### Pitfall 11: Caller classes override the primitive
**What goes wrong:** a caller passes a `max-md:` height of its own, and `twMerge` keeps the caller's. `HostActions` deliberately passes `max-md:h-auto max-md:min-h-11` [VERIFIED: `HostActions.tsx:252`], which is fine. A caller `h-8`/`size-8` without a prefix does **not** remove `max-md:h-11`, because the variant group differs. `audit.tsx:204` (`<SelectTrigger id="audit-action" className="h-8 w-56">`) still reaches 44 at 390 through `max-md:data-[size=default]:h-11` [VERIFIED: `select.tsx` trigger class].
**How to avoid:** when a SMALL line names a primitive that already has the `max-md:` size, look at the call site's `className` first.

### Pitfall 12: The route guard imports every route module under jsdom
**What goes wrong:** a module that touches `matchMedia` or `window` at import time would crash, because `setup.ts` stubs those only in `beforeEach`. No such top-level use was seen (theme work happens in `useEffect`, `__root.tsx:33-35`), but the whole set has never been imported in one file before. [ASSUMED] (A3)
**How to avoid:** the first task runs the new test on its own before anything else depends on it.

### Pitfall 13: Phase 13 is still open underneath
**What goes wrong:** STATE.md (2026-09-30) records that HACT-04 needs a **fifth host action, "check for updates only"**, planned as 13-12 and not yet planned [VERIFIED: `.planning/STATE.md` "Phase 13 offen"; no `13-12-*` file exists]. That changes `HOST_ACTIONS` and the 2 × 2 grid.
**How to avoid:** the audit measures whatever buttons exist, so the guard itself is unaffected. The `/host` hand test (D-11) and the D-08 "after" dump have to run after 13-12 lands, or be repeated if 13-12 lands later.

## Code Examples

### The primitives as they are today (verbatim, the lines to change)

`web/src/components/ui/button.tsx:30-39` [VERIFIED]:
```
default: 'h-8 max-md:h-11 gap-1.5 px-2.5 max-md:px-3.5 has-data-[icon=inline-end]:pr-2 has-data-[icon=inline-start]:pl-2',
xs: "h-6 gap-1 rounded-[min(var(--radius-md),10px)] px-2 text-xs in-data-[slot=button-group]:rounded-lg has-data-[icon=inline-end]:pr-1.5 has-data-[icon=inline-start]:pl-1.5 [&_svg:not([class*='size-'])]:size-3",
sm: "h-7 max-md:h-11 gap-1 rounded-[min(var(--radius-md),12px)] px-2.5 max-md:px-3 text-[0.8rem] …",
lg: 'h-9 max-md:h-11 gap-1.5 px-2.5 max-md:px-3.5 …',
icon: 'size-8 max-md:size-11',
'icon-xs': "size-6 rounded-[min(var(--radius-md),10px)] in-data-[slot=button-group]:rounded-lg [&_svg:not([class*='size-'])]:size-3",
'icon-sm': 'size-7 rounded-[min(var(--radius-md),12px)] in-data-[slot=button-group]:rounded-lg',
'icon-lg': 'size-9',
```
Target: `xs` + `max-md:h-11 max-md:px-3`; `icon-xs`/`icon-sm`/`icon-lg` + `max-md:size-11`. Red check (a) removes `max-md:h-11` from `default`. Red check (b) removes `max-md:size-11` from `icon-sm`. Both are one-token edits, and the restore is checked with `cmp` against `git show HEAD:<file>`, as 13-11 did.

`web/src/components/ui/dialog.tsx:59-66, 72-76` [VERIFIED]:
```
<Button variant="ghost" className="absolute top-2 right-2" size="icon-sm">
…
<div data-slot="dialog-header" className={cn('flex flex-col gap-2', className)} {...props} />
```
Target: `className="absolute top-2 right-2 max-md:top-0 max-md:right-0"`; header `'flex flex-col gap-2 max-md:pr-8'`.

`web/src/components/ui/sonner.tsx:44-45` [VERIFIED]:
```
closeButton:
  'top-3! right-3! left-auto! size-6! translate-x-0! translate-y-0! border-border bg-transparent',
```
Target: append `max-md:size-11! max-md:top-0! max-md:right-0! max-md:border-transparent!` (Pitfall 10).

`web/src/components/ui/dropdown-menu.tsx` Item [VERIFIED: line 65 contains `rounded-md px-1.5 py-1 text-sm`]; CheckboxItem, RadioItem [VERIFIED: `py-1 pr-8 pl-1.5`]; SubTrigger [VERIFIED: `px-1.5 py-1`]; `select.tsx` SelectItem [VERIFIED: `py-1 pr-8 pl-1.5`]. Target for each: + `max-md:min-h-11 max-md:py-2`.

### The opener list (shape recommended; names and routes from 14-UI-SPEC)

```js
// web/scripts/layout-audit.mjs
const OPENERS = [
  { name: 'Navigation', route: '/', widths: [390],
    open: [(p) => p.getByRole('button', { name: 'Open the navigation' }).click()],
    expect: 'nav[aria-label="Main navigation"]',
    close: (p) => p.getByRole('button', { name: 'Close the navigation' }).click({ position: { x: 370, y: 400 } }),
    drawer: true /* adds the CUT OFF check */ },
  { name: 'Power menu', route: '/nodes/m-cp-1', widths: [390, 1280],
    open: [(p) => p.getByRole('button', { name: /^Power: / }).click()], expect: '[role="menu"]' },
  { name: 'Sudo dialog', route: '/settings', widths: [390, 1280],
    open: [(p) => p.fill('#new-username', 'layout-audit-probe'),
           (p) => p.fill('#new-password', 'a-long-enough-passphrase-that-is-never-used-2'),
           /* submit the New account form */],
    expect: '[role="dialog"]:has-text("Confirm your password")' },
  { name: 'Host action dialog', route: '/host', widths: [390, 1280],
    fixture: { '/api/v1/host': HOST_HELPER_INSTALLED },
    open: [(p) => p.getByRole('button', { name: 'Restart host' }).click()],
    expect: '[role="dialog"]:has(#host-action-confirm)' },
  // What's new, Power confirmation, Reset dialog, Select list: as in 14-UI-SPEC
]
```
`aria-label={`Power: ${target.name}`}` [VERIFIED: `PowerMenu.tsx:158`]. "Open the navigation" [VERIFIED: `Header.tsx:26`]. "Restart host" = `HOST_ACTION_LABEL.reboot` [VERIFIED: `HostActions.tsx:55`]. `#host-action-confirm` [VERIFIED: `HostActions.tsx:366`]. `#audit-action` [VERIFIED: `audit.tsx:204`]. The submit control of the New account form was not read, so its locator is left for the plan.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Hand list `ROUTES` in the audit | router-derived leaves checked against a JSON list | this phase | a new route without an audit entry is red before the browser starts |
| Route pass only, nothing clicked | explicit openers, scoped | this phase | menus, dialogs and the drawer are measured for the first time |
| Disabled controls skipped | measured | this phase | a control that turns enabled later has already been sized |
| Tailwind `!size-6` (prefix, v3) | `size-6!` (suffix, v4) | Tailwind v4 | the repository already uses the suffix form (`sonner.tsx:45`) |

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `router.matchRoutes(path).at(-1).routeId` equals the leaf route's `.id` | Pattern 1 | the "example opens its route" test fails spuriously. Fallback: `getMatchedRoutes(path)[2]?.id` (verified signature) |
| A2 | Tailwind 4.3.3 generates `max-md:size-11!` (variant + trailing `!`) | Pitfall 10, Code Examples | the toast close stays 24 px at 390. Detected by the built-CSS grep and the toast backstop |
| A3 | Importing every route module in one jsdom test file has no import-time side effect that crashes | Pitfall 12 | the guard test errors at import. Detected by the first task's run |
| A4 | Sonner renders its dismiss button with a `data-close-button` attribute that a browser test can select | Don't Hand-Roll (toast backstop) | the backstop selects nothing and reports nothing. Must assert count ≥ 1 |

## Open Questions (RESOLVED)

1. **Does the spec's "the audit never presses a confirm button" admit submitting the New-account form?** (RESOLVED)
   - What we know: that submit is the only verified real path to a 428 in the audit (Pattern 3), and the server makes it non-executing by construction.
   - Recommendation: the plan states the reading explicitly ("never a confirm in a confirmation dialog, never the sudo dialog"), keeps `EXECUTED` as the proof, and falls back to the D-05 fixture 428 only after one failed attempt. This is a planner decision inside D-05's latitude, not an operator question.
   - **RESOLVED (14-01 Task 2):** the Sudo dialog opener submits the New account form on /settings and gets the daemon's real `428 sudo.required`. The reading goes into the opener's comment. The sudo password field is never filled (a grep gate checks this). `EXECUTED` fails any 2xx to an action request, and it is seen red in 14-01 Task 3. The D-05 fixture 428 is a fallback, used only after two consecutive failed runs and only with a body a real run printed. The SUMMARY says which path was taken.
2. **Can the D-11 hand test reach the typed confirmation at all?** (RESOLVED)
   - What we know: the buttons need `actions.available`, which the server derives from the helper files on the machine. 13-11 found them absent on the Pi. The production daemon is excluded by the UI-SPEC. A phone cannot use `--insecure-http` (Secure cookies are not sent over http, HANDOVER §3.1), so a dev daemon on the LAN needs its self-signed TLS.
   - Recommendation: put the prerequisite to the operator as a choice (AskUserQuestion) at the end of the phase: (a) the operator installs the helper on the Pi (a Phase 13 UAT item anyway), then the hand test runs against a dev daemon with TLS on a LAN port. Recommended: it serves both phases. (b) record "not performed" per D-11. Nothing may be done silently. Installing the helper and addressing production are the operator's call.
   - **RESOLVED (14-07 Task 3):** the executor installs nothing and asks no open question. It records the hand test as performed (device, browser, result) or as "not performed" together with its three prerequisites. Until the hand test is performed, ROADMAP criterion 4 stays **open (human_needed)** in the SUMMARY: its measured half is met by 14-06, and its phone half is never marked passed by default. At verification the prerequisite goes to the operator as a choice: (A, recommended) install the helper and run the test against a dev daemon with TLS; (B) keep "not performed", with criterion 4 left human_needed.
3. **Which of the dialogs' own contents turn out small in the first red run** (Reset dialog disk rows, What's new chips, `WhatsNew` version button)? (RESOLVED)
   - Recommendation: the plan treats the first extended run as the finding (CONTEXT "Specific Ideas"). The UI-SPEC single-place table already names the likely ones.
   - **RESOLVED (14-03 Task 2 → 14-05, 14-06):** the first extended run is recorded as the finding in `$HOME/.cache/holzkube-manager-layout/14-03-first-red.log`. Each SMALL detail line now names its primitive by data-slot, so the SUMMARY sorts every finding into primitives (fixed in 14-05) or single places (fixed in 14-06). A finding in a file 14-06 does not list is fixed there the same way and named in its SUMMARY.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Node | vitest, audit | ✓ (via `. ~/.nvm/nvm.sh`) | v22.23.3 | — |
| Playwright Chromium | audit, browser tests | ✓ | build 1234 (matches playwright 1.62.1) | — |
| Go | `build:go` in `test:layout` | ✓ `~/.local/go` | go1.27.1 local; the release toolchain is fetched via `GOTOOLCHAIN` as in 13-11 | — |
| `./bin/task` | `task ci`, `task test:layout` | ✓ | repo-local | — |
| A phone on the LAN | D-11 hand test | unknown | — | record "not performed" |
| Host helper installed on the Pi | D-11 hand test reaching the dialog | ✗ (13-11: absent) | — | operator choice (Open Question 2) |
| `go test -race` | CI only | ✗ on the Pi (ThreadSanitizer vs. a 47-bit address space) | — | not needed for this phase (web-only) |

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | vitest 4.1.11 (projects `jsdom`, `browser`) + the Playwright audit script |
| Config file | `web/vite.config.ts` (`test.projects`) |
| Quick run command | `npm --prefix web exec -- vitest run --project jsdom src/layoutRoutes.test.ts src/fixtures.test.ts` |
| Full suite command | `./bin/task ci` (lint:web → test:web → build → lint:go → test → test:layout → test:next) |
| Layout gate | `./bin/task test:layout; echo "exit=$?"` (never piped) |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | What makes it red | File Exists? |
|--------|----------|-----------|-------------------|-------------------|-------------|
| MOB-01 | every control ≥ 44 × 44 at 390 on every route | audit (e2e) | `./bin/task test:layout` | red check (a): `max-md:h-11` removed from Button `default` → `SMALL 390px <route>` lines, exit ≠ 0 | ✅ audit exists; extend |
| MOB-01 | 1280 unchanged | one-off measurement | `LAYOUT_DUMP=/tmp/…/before.jsonl npm --prefix web run test:layout` before the first class change, `after.jsonl` after the last; compare by key | any box differing → SUMMARY states count ≠ 0 | ❌ Wave 0 (dump flag) |
| MOB-02 | opened states measured | audit (e2e) | `./bin/task test:layout` | red check (b): `max-md:size-11` removed from `icon-sm` → `SMALL 390px <route> · <opener>`, exit ≠ 0. A missing trigger → `OPENER`; zero measured → `EMPTY` | ❌ Wave 0 (openers) |
| MOB-02 | a route in the router is missing from the audit | unit (jsdom) | `npm --prefix web run test:layout` (the guard runs first) | red check (c): one entry deleted from `layout-routes.json` → "… is in the router but not in …", exit ≠ 0, browser not started | ❌ Wave 0 (`layoutRoutes.test.ts`, `routeTree.ts`, JSON) |
| MOB-02 | the audit executes nothing | audit (e2e) | same | a 2xx on a non-GET action path → `EXECUTED` line, exit ≠ 0 | ❌ Wave 0 (monitor) |
| MOB-02 | the helper-installed variant is a valid host answer | unit (jsdom) | `vitest run --project jsdom src/fixtures.test.ts` | variant not parseable by `hostSchema` | ❌ Wave 0 |
| MOB-01 | toast dismiss ≥ 44 at 390 | browser component | `npm --prefix web run test:browser -- <toast test file>` | remove `max-md:size-11!` → expect 24 to be ≥ 44; also assert close count ≥ 1 | ❌ Wave 0 |
| MOB-03 | `/host` + host action dialog at 390 | audit (e2e) | `./bin/task test:layout` | the dialog close X at 28 px (before the fix) → `SMALL 390px /host · Host action dialog` | ❌ Wave 0 (opener + override) |
| MOB-03 | a thumb reaches the typed confirmation on a phone | manual | — | recorded in the SUMMARY as performed/not performed, never "passed" by default | manual-only (D-11) |

### Sampling Rate
- **Per task commit:** the quick run command, plus `./bin/task test:layout` whenever a class or the audit changed.
- **Per wave merge:** `./bin/task test:layout`.
- **Phase gate:** `./bin/task ci` exit 0, read from the command; the three red checks each followed by a restore (`cmp` against `git show HEAD:`) and a green run.

### Wave 0 Gaps
- [ ] `web/src/routeTree.ts`: the tree moved out of `App.tsx`, which keeps `createRouter` and the `Register` declaration [VERIFIED: `App.tsx:78-87`]
- [ ] `web/scripts/layout-routes.json`: 27 entries today (Pattern 1)
- [ ] `web/src/layoutRoutes.test.ts`: the route guard
- [ ] `web/package.json` `test:layout` chained (Pitfall 2)
- [ ] `layout-audit.mjs`: JSON routes, scoped measure with a shared SELECTOR/count, disabled not skipped, OPENERS, close steps, animation settle, request monitor, `LAYOUT_DUMP`
- [ ] helper-installed host variant + its `fixtures.test.ts` case
- [ ] toast backstop browser test

## Security Domain

`security_enforcement` is enabled (absent = enabled; `.planning/config.json` has `"security_enforcement": true`, ASVS level 1).

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no change | the audit account is created and signed in through the real routes; its password lives only in the audit script, as today (`layout-audit.mjs:141-142`) |
| V3 Session Management | no change | — |
| V4 Access Control | yes (test harness) | the sudo gate (428 before the handler) is what makes the sudo opener safe; `EXECUTED` proves the harness never passed it |
| V5 Input Validation | no | no new input surface |
| V6 Cryptography | no | — |

### Known Threat Patterns

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| The audit places a real host order on the operator's Pi (the audit runs where production runs) | Tampering / DoS | never type into `#host-action-confirm`; confirm refused (409) without a helper; place is sudo-gated; the order file lands in the audit's temp data dir [VERIFIED: `handlers/host.go:44-47`]; `EXECUTED` fails any 2xx on an action path |
| The audit opens the sudo window with its known password | Elevation | never fill `#sudo-password`; close with Escape only |
| Real identifiers enter fixtures, screenshots or the SUMMARY (public repo) | Information disclosure | documentation values only; `internal/publicrepo` in the gate; the hand-test record names device and browser, not the LAN address or hostname |
| A `LAYOUT_DUMP` file is committed as a baseline | — (process) | write it under the scratchpad or `/tmp`; D-08 forbids a checked-in baseline |

## Sources

### Primary (HIGH confidence, read this session)
- `web/scripts/layout-audit.mjs` (whole file), `web/src/App.tsx`, `web/src/routes/__root.tsx`, `web/src/routes/placeholders.tsx`, `web/src/components/{Sidebar,AppShell,Header,HostActions,SudoDialog,PowerMenu,NodeActions}.tsx`, `web/src/components/ui/{button,dialog,dropdown-menu,select,input,sonner}.tsx`, `web/src/components/Toaster.tsx`, `web/src/routes/{audit,accounts}.tsx` (parts), `web/src/fixtures.test.ts`, `web/src/routes/host.browser.test.tsx` (head), `web/fixtures/demo.json` (head, host entry), `web/vite.config.ts`, `web/tsconfig.json`, `web/package.json`, `Taskfile.yml`, `.github/workflows/ci.yml`
- `internal/httpapi/router.go:398-520`, `internal/auth/sudo.go`, `internal/httpapi/handlers/{power,users,host}.go` (route tables and headers)
- `web/node_modules/@tanstack/router-core/dist/esm/{route,router,new-process-route-tree}.js`
- `.planning/phases/14-telefon-tippziele/{14-CONTEXT,14-UI-SPEC}.md`, `13-11-SUMMARY.md`, `STATE.md`, `ROADMAP.md`, `REQUIREMENTS.md`, `HANDOVER.md` §3.2, `README.md`, `docs/guide.md` "On a phone"

### Secondary (MEDIUM)
- Playwright docs, BrowserContext.route: page routes take precedence (https://playwright.dev/docs/api/class-browsercontext)
- Tailwind docs, important modifier `!` suffix (https://tailwindcss.com/docs/styling-with-utility-classes)

## README and guide (CLAUDE.md rule)

- `README.md:110` "**On a phone.** Every screen, one-handed." Extend it to menus and dialogs. `docs/screenshots/phone.png` is rendered from `/nodes/m-cp-1`, `/kubernetes/apps`, `/clusters` at 390 [VERIFIED: `readme-images.mjs:295-300`]. Growing controls change it, so re-render with `task build && node web/scripts/readme-images.mjs`.
- `docs/guide.md` "On a phone" (lines 1621-1656) describes the audit's passes. Add the route guard and the opened states. Its "Below 768px a table is not a table" paragraph is out of scope here and stays as it is.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH. Nothing new is installed, and versions were read from package files.
- Architecture: HIGH. Router internals, the middleware order and the route tables were read from source.
- Pitfalls: HIGH for 1–9 and 11–13 (read from code). MEDIUM for 10 (Tailwind variant + `!`, to be verified on the built CSS).

**Research date:** 2026-09-30
**Valid until:** 2026-10-30, or until 13-12 lands (it changes `/host`)
