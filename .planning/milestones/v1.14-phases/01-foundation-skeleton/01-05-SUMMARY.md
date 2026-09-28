---
phase: 01-foundation-skeleton
plan: 05
subsystem: ui
tags: [react, typescript, vite, tanstack-router, tanstack-query, shadcn-ui, radix, tailwind, vitest, jsdom, testing-library, rfc9457, csrf, sudo-mode, dark-mode]

# Dependency graph
requires:
  - phase: 01-01
    provides: "the Vite/React scaffold, web/src/api.ts as the single fetch chokepoint with the CSRF preconditions, the committed package-lock.json, the //go:embed dist contract and docs/api-contract.md"
  - phase: 01-03
    provides: "GET /api/v1/audit serving the full query contract with {items, next_cursor}, the <redacted> marker and request_id in params, and the unclearable audit_chain verdict on system/status"
  - phase: 01-04
    provides: "the CSRF header checked by value, POST /api/v1/auth/sudo, the 428 on destructive routes with a shut window, and 429 + Retry-After on login and sudo"
provides:
  - "the permanent app shell: sidebar over every future area, header with the signed-in operator and sign-out, toast surface, error page, TanStack Router tree (D-10)"
  - "honest Coming-in-phase-N placeholders derived from the navigation, so a nav entry can never point at an unregistered route"
  - "dark-first theme following prefers-color-scheme, an explicit choice in localStorage, applied before the first paint (D-11)"
  - "shadcn/ui as twelve source files under web/src/components/ui/, not a framework dependency (D-12)"
  - "web/src/lib/problem.ts: RFC 9457 decoding plus a presentation for every code prefix of the closed taxonomy (FOUND-11)"
  - "web/src/api.ts: all three CSRF preconditions on every mutating request, the 401 login transition and the 428 sudo replay with an unchanged body and unchanged headers"
  - "SudoDialog: the client half of the 428 flow, with the original request replayed after re-authentication (FOUND-03, D-05)"
  - "ChainBanner: a stateless, non-closable chain-break banner above the whole shell (D-15)"
  - "the audit view: filters, cursor paging on next_cursor !== null, and a per-record detail dialog — the table pattern phase 3 inherits (D-13)"
  - "the frontend test harness: vitest inside vite.config.ts, jsdom, Testing Library, and 60 tests"
affects: [01-06, 03-inventar, 05-streaming, 06-jobs, 07-config, 08-provisioning, 09-upgrades, 10-haertung]

actuals:
  tokens: 39800
  tasks: 3
  commits: 4

tech-stack:
  added:
    - "@tanstack/react-router 1.170.32, @tanstack/react-query 5.102.6"
    - "lucide-react 1.34.0, class-variance-authority 0.7.1, clsx 2.1.1, tailwind-merge 3.6.0"
    - "radix-ui 1.6.7 (the unified primitives package), sonner 2.0.8, tw-animate-css 1.4.0, @fontsource-variable/geist 5.3.0, shadcn 4.19.0"
    - "vitest 4.1.11, @vitest/coverage-v8 4.1.11, jsdom 30.0.1, @testing-library/{react 16.3.3, user-event 14.6.6, jest-dom 7.0.1}"
  patterns:
    - "Navigation is data: NAV_AREAS drives both the sidebar and the placeholder routes, so a nav entry cannot point at a route nobody registered"
    - "A route module owns its own createRoute and imports the layout from __root; App.tsx only assembles the tree"
    - "The request init is built once and reused for the sudo replay, so 'unchanged body and headers' is structural rather than asserted"
    - "The UI branches on the problem `code`, never on the status alone; two codes already share 403"
    - "Every error path ends in a sentence: a response that is not readable problem+json still produces one"
    - "A component that must not be dismissible takes its verdict as a prop and holds no state at all, so the property is testable"
    - "vitest lives in vite.config.ts so the path alias cannot drift between bundler, tsc and tests"
    - "Tests are faked at the global fetch, which records the exact (input, init) pairs and makes request-identity claims checkable"
    - "Every behavioural test group was re-run against a deliberately broken implementation before being trusted"

key-files:
  created:
    - web/src/lib/problem.ts
    - web/src/lib/problem.test.ts
    - web/src/lib/utils.ts
    - web/src/hooks/useTheme.ts
    - web/src/hooks/useTheme.test.tsx
    - web/src/hooks/useSession.ts
    - web/src/components/AppShell.tsx
    - web/src/components/Sidebar.tsx
    - web/src/components/Header.tsx
    - web/src/components/ThemeToggle.tsx
    - web/src/components/Toaster.tsx
    - web/src/components/ComingSoon.tsx
    - web/src/components/SudoDialog.tsx
    - web/src/components/SudoDialog.test.tsx
    - web/src/components/ChainBanner.tsx
    - web/src/components/ChainBanner.test.tsx
    - web/src/components/ui/ (button, input, label, card, table, dialog, sonner, badge, select, separator, skeleton, dropdown-menu)
    - web/src/routes/__root.tsx
    - web/src/routes/index.tsx
    - web/src/routes/setup.tsx
    - web/src/routes/login.tsx
    - web/src/routes/error.tsx
    - web/src/routes/audit.tsx
    - web/src/routes/audit.test.tsx
    - web/src/routes/placeholders.tsx
    - web/src/test/setup.ts
    - web/components.json
  modified:
    - web/src/api.ts
    - web/src/App.tsx
    - web/src/index.css
    - web/index.html
    - web/package.json
    - web/package-lock.json
    - web/tsconfig.json
    - web/vite.config.ts
    - biome.json

key-decisions:
  - "The dashboard shows only what the System Status Contract actually defines. The plan asked for version, bind address and data directory from GET /api/v1/system/status; that endpoint returns setup_required and audit_chain and nothing else, and inventing the fields would produce a UI that breaks against the real server"
  - "The theme is resolved from localStorage and matchMedia on every read rather than cached in a module variable, so the reload case D-11 cares about is proven by storage and not by the cache"
  - "The request init object is built once and handed to both the original call and the sudo replay, which makes 'the same body and the same headers' a structural property rather than a claim"
  - "GET /api/v1/auth/me is exempt from the 401 interceptor: its 401 means 'not signed in', which is an answer, and treating it as an expiry would greet a first-time visitor with 'your session ended'"
  - "POST /api/v1/auth/sudo is exempt from both interceptors: a wrong password there is 401 on the credential, not on the session, and must not become a login transition or a second sudo prompt"
  - "next-themes, which shadcn wires sonner.tsx to by default, was removed and the copied component now reads the project's own theme hook — D-11 forbids a second theming layer and D-12 is what makes editing the copy the right move"
  - "shadcn 4.19 ships Radix as the unified `radix-ui` package rather than scoped @radix-ui/* packages, so the plan's literal grep does not match while its stated intent does"
  - "vite re-emits internal/httpapi/dist/.gitkeep, because emptyOutDir had been silently deleting a tracked file on every web build since plan 01"
  - "The lint script keeps --config-path .., because biome.json lives at the repository root; the plan's literal `biome check src` finds no config"

patterns-established:
  - "A later phase replaces a placeholder by removing the phase number from its NAV_AREAS entry and registering a real route; it never edits the shell"
  - "A new destructive route adds one line to ACTION_LABELS in web/src/api.ts so the sudo prompt names it; without that entry the prompt still works but says 'This destructive action'"
  - "A new error code added server-side must be added to PRESENTATION_BY_CODE and to the TAXONOMY table in problem.test.ts, or the coverage test fails"
  - "A client that reads a paginated endpoint compares next_cursor against null and never for truthiness; audit.test.tsx feeds a 0 through as the regression guard"
  - "Redacted values are rendered as explicitly redacted, never as an empty field: an empty cell reads as 'nothing was sent', which is a different and wrong statement"

requirements-completed: [FOUND-01, FOUND-03, FOUND-11]

coverage:
  - id: D1
    description: "The permanent app shell stands: sidebar over every future area, header with the operator and sign-out, toasts, error page and the router tree, so later phases hang themselves in instead of building navigation (D-10)"
    requirement: "FOUND-01"
    verification:
      - kind: other
        ref: "npm --prefix web run build && npm --prefix web run typecheck — the tree compiles and bundles; grep createRootRoute web/src/routes/__root.tsx returns 2"
        status: pass
      - kind: manual_procedural
        ref: "live binary: GET / redirects to /setup and GET /setup serves the embedded index.html referencing the built JS and CSS assets, both 200"
        status: pass
    human_judgment: true
    rationale: "The bundle builds, embeds and is served, and the routes compile — but nothing drives a real browser, so 'the sidebar is usable and the header renders the operator' is still held by reading the code. This is concern D2 from plan 01, narrowed rather than closed: component-level rendering is now tested in jsdom, browser-level rendering is not."
  - id: D2
    description: "Areas that do not exist yet show an honest Coming-in-phase-N page naming the area and its phase, instead of a dead link or an empty frame (D-10)"
    verification:
      - kind: unit
        ref: "temporary probe suite over NAV_AREAS and ComingSoon: 7 assertions, one per placeholder plus the list itself — all six placeholder routes name their area and their phase"
        status: pass
      - kind: other
        ref: "placeholders.tsx derives its routes from NAV_AREAS, so the set of nav entries and the set of registered placeholder routes cannot diverge"
        status: pass
    human_judgment: true
    rationale: "The probe suite that proved this was run and then deleted, because adding a sixth test file would have gone beyond the plan's declared files. The structural guarantee (routes derived from the nav list) is permanent; the per-page assertion is not, and a reviewer should either re-add it or accept the recorded run."
  - id: D3
    description: "The theme is dark-first, follows prefers-color-scheme by default, and an explicit choice survives a reload via localStorage (D-11)"
    verification:
      - kind: unit
        ref: "web/src/hooks/useTheme.test.tsx — 7 tests: system dark, system light, no matchMedia at all, the choice written under a fixed key, the dark class on <html>, the stored choice winning over the system on a remount, and following the system again once cleared"
        status: pass
      - kind: other
        ref: "negative control: making resolveTheme ignore storage fails 4 of the 7"
        status: pass
      - kind: manual_procedural
        ref: "live binary: the served /setup page carries the inline boot script that sets the class before React loads"
        status: pass
    human_judgment: false
  - id: D4
    description: "Every visible string is English and no i18n layer exists (D-09)"
    requirement: "FOUND-01"
    verification:
      - kind: other
        ref: "grep -icE '\"(i18next|react-i18next|react-intl|@lingui/core|next-intl)\"' web/package.json returns 0"
        status: pass
    human_judgment: true
    rationale: "The absence of a translation layer is machine-checked. That every user-visible string is English is held by review of the 26 source files written here; no linter enforces it."
  - id: D5
    description: "While no account exists every route lands on the setup wizard, and afterwards the route refuses with the server's 409 rather than showing an empty form (D-01)"
    requirement: "FOUND-01"
    verification:
      - kind: manual_procedural
        ref: "live binary: POST /api/v1/setup returns 201, a second POST returns 409 setup.already-completed, and GET / redirects to /setup while setup_required is true"
        status: pass
      - kind: other
        ref: "AppShell redirects to /setup on setup_required before rendering anything; setup.tsx renders the 409 detail in place of the form"
        status: pass
    human_judgment: true
    rationale: "The server side of D-01 is proven against a live binary. The client redirect and the 409 rendering are held by code review: no test mounts AppShell or the setup route, because the plan's declared test files do not include one."
  - id: D6
    description: "A 428 from a destructive action opens the password prompt and replays the original action after successful re-authentication, without losing any input (FOUND-03, D-05)"
    requirement: "FOUND-03"
    verification:
      - kind: unit
        ref: "web/src/components/SudoDialog.test.tsx#opens on a 428, names the action, and replays the original request unchanged — compares the recorded fetch arguments of call 1 and call 3"
        status: pass
      - kind: unit
        ref: "web/src/components/SudoDialog.test.tsx#does not replay anything when the operator cancels"
        status: pass
      - kind: unit
        ref: "web/src/components/SudoDialog.test.tsx#stays open and explains itself when the re-authentication password is wrong"
        status: pass
      - kind: unit
        ref: "web/src/components/SudoDialog.test.tsx#shows the remaining wait, and never a lockout, when re-authentication is rate limited"
        status: pass
      - kind: unit
        ref: "web/src/components/SudoDialog.test.tsx#does not prompt for a 428 when nothing is mounted to answer it"
        status: pass
      - kind: other
        ref: "negative control: removing the replay fails the replay test; drifting the CSRF header value to 'true' fails it too"
        status: pass
      - kind: manual_procedural
        ref: "live binary: POST /api/v1/account/password returns 428 sudo.required, POST /api/v1/auth/sudo returns 204, the identical password-change call then returns 204"
        status: pass
    human_judgment: false
  - id: D7
    description: "A session ending during work leads to the login screen with an explanation and without data loss (D-07)"
    verification:
      - kind: unit
        ref: "web/src/components/SudoDialog.test.tsx#turns a 401 during ordinary work into a login transition with an explanation"
        status: pass
      - kind: unit
        ref: "web/src/components/SudoDialog.test.tsx#leaves the session probe alone: a 401 from /auth/me is an answer, not an expiry"
        status: pass
      - kind: unit
        ref: "web/src/components/SudoDialog.test.tsx#stops listening once unmounted"
        status: pass
      - kind: other
        ref: "negative control: removing the /auth/me exemption fails the probe test"
        status: pass
    human_judgment: true
    rationale: "The interception, the explanation and the probe exemption are proven. 'Without data loss' is proven only for the sudo path, where the pending promise keeps the caller's state alive; for the 401 path the query cache is cleared and the operator is navigated away, so what survives is whatever a form component holds — no test mounts a form across an expiry."
  - id: D8
    description: "Every error response is rendered from problem+json: title as heading, detail as text, code for the routing — never a raw status code and never an empty toast (FOUND-11)"
    requirement: "FOUND-11"
    verification:
      - kind: unit
        ref: "web/src/lib/problem.test.ts — 13 cases, one per code of the closed taxonomy, plus a coverage assertion over the prefix set"
        status: pass
      - kind: unit
        ref: "web/src/lib/problem.test.ts#never returns an empty string, even for a problem with no text at all"
        status: pass
      - kind: unit
        ref: "web/src/lib/problem.test.ts#produces a sentence, not a bare status, for a response with no usable body"
        status: pass
      - kind: unit
        ref: "web/src/lib/problem.test.ts#falls back to title when detail is missing / present but empty"
        status: pass
      - kind: unit
        ref: "web/src/lib/problem.test.ts#phrases the wait without ever implying a locked account"
        status: pass
    human_judgment: false
  - id: D9
    description: "The audit view shows records chronologically, filters by date and action, opens a detail dialog per record, and pages on next_cursor !== null (D-13, D-14)"
    verification:
      - kind: unit
        ref: "web/src/routes/audit.test.tsx — 9 tests: delivery order, the exact contract parameters, the explained empty result, the detail dialog with seq/prev_hash/hash/request_id/truncated session/redacted params, the orphaned intent, paging with the returned cursor, no control when next_cursor is null, and the 0-cursor regression guard"
        status: pass
      - kind: other
        ref: "negative control: replacing next_cursor !== null with a truthiness check fails the 0-cursor test; dropping the hash fields fails the detail test"
        status: pass
      - kind: manual_procedural
        ref: "live binary: GET /api/v1/audit?limit=2 returns {items, next_cursor}; action=auth.sudo returns 2 items; from=2030-01-01 returns 0 items and next_cursor null; the recorded params carry <redacted> and request_id exactly as the detail dialog reads them"
        status: pass
    human_judgment: false
  - id: D10
    description: "A broken hash chain appears as a permanent banner naming the affected file and line, with no way to close, hide or confirm it away (D-15, threat T-01-34)"
    verification:
      - kind: unit
        ref: "web/src/components/ChainBanner.test.tsx — 7 tests: nothing rendered while intact, file and line named when broken, what the finding does and does not mean, zero interactive elements, no closing-or-confirming accessible name, unchanged across re-renders, gone only when the verdict turns clean"
        status: pass
      - kind: other
        ref: "grep -icE 'onClose|dismiss|setHidden|acknowledge' web/src/components/ChainBanner.tsx returns 0"
        status: pass
      - kind: other
        ref: "negative control: adding a Dismiss button fails 2 tests; rendering while ok fails 2 others"
        status: pass
      - kind: manual_procedural
        ref: "live binary: rewriting the actor on line 3 of the audit file and restarting yields audit_chain.ok=false, broken_at_line=3 on every subsequent call — the exact shape the banner renders"
        status: pass
    human_judgment: false

# Metrics
duration: 27 min
completed: 2026-08-28
status: complete
---

# Phase 1 Plan 05: Web Interface Summary

**The permanent React shell — TanStack Router tree, shadcn/ui as repository source, dark-first theme, setup wizard, login, the 428 sudo prompt that replays the original request byte for byte, and an audit view with filters, cursor paging and a non-closable chain-break banner — behind 60 tests, every group of which was first run against a deliberately broken implementation.**

## Performance

- **Duration:** 27 min
- **Started:** 2026-08-28T02:46:00Z
- **Completed:** 2026-08-28T03:13:26Z
- **Tasks:** 3
- **Files changed:** 47 (28 created, 19 modified)
- **Tests:** 60, in 5 files, from 0

## Accomplishments

- **Concern D2 from plan 01 is narrowed from "unverified" to "unverified only in a browser."** The frontend had no test harness and no assertion of any kind. It now has 60 tests over the theme, the error taxonomy, the sudo replay, the session expiry, the chain banner and the audit table — and the live binary was driven through setup, the 409 refusal, the 428 gate, the sudo grant, the replay, the audit filters and a deliberately corrupted hash chain over real HTTP.
- **The sudo replay is structurally correct, not merely asserted.** The request `init` object is built once and handed to both the original call and the retry, so "the same body and the same headers" is a property of the code rather than a promise about it. The test proves it by comparing the recorded `fetch` arguments of the first and third calls.
- **The three CSRF preconditions are set in one place and checked by value.** `X-Holzkube-CSRF` is sent as the literal `'1'`; the live server confirmed that `'true'` is refused with `X-Holzkube-CSRF must be 1`.
- **The chain-break banner has no state and no controls.** Its verdict arrives as a property, and the test counts the interactive elements and the accessible names that would suggest a way out and expects none of both — which is what turns the D-15 promise into something a later refactor cannot quietly break.
- **The navigation is data.** Sidebar entries and placeholder routes both derive from one list, so a nav entry cannot point at a route nobody registered, and a later phase replaces a placeholder by deleting a phase number.
- **A tracked file that had been silently deleted on every web build since plan 01 was found and fixed.**

## Task Commits

1. **Task 1: router tree, shadcn/ui, dark-first theme and the permanent app shell** — `da11135` (feat)
2. **Task 2: setup wizard, login, session expiry, sudo prompt and problem+json rendering** — `1ce0701` (feat)
3. **Task 3: audit view with filters, detail dialog, cursor paging and the chain banner** — `f2902e6` (feat)
4. **Sign out lands on the login screen with the reason that fits** — `eeabd47` (fix)

## Files Created/Modified

**The client and the error taxonomy**
- `web/src/lib/problem.ts` — RFC 9457 typing, `ProblemError` with `Retry-After`, a presentation per code prefix of the closed taxonomy, and the guarantee that every path out produces a sentence
- `web/src/api.ts` — the one place calling `fetch`; all three CSRF preconditions on every mutating request, the 401 login transition, the 428 sudo replay, and the audit query string
- `web/src/hooks/useSession.ts` — status, identity, login and logout over TanStack Query

**The shell**
- `web/src/routes/__root.tsx` — query client, theme, toaster, sudo dialog, session watcher, error boundary, and the pathless authenticated layout
- `web/src/components/{AppShell,Sidebar,Header,ThemeToggle,Toaster,ComingSoon}.tsx`
- `web/src/hooks/useTheme.ts` — dark-first, storage-backed, recomputed on every read
- `web/index.html` — the inline boot script that sets the theme class before the first paint
- `web/src/App.tsx` — router wiring and nothing else

**The pages**
- `web/src/routes/{setup,login,index,audit,error,placeholders}.tsx`
- `web/src/components/{SudoDialog,ChainBanner}.tsx`

**Component library and tooling**
- `web/src/components/ui/` — twelve shadcn/ui components as source in the repository (D-12)
- `web/components.json`, `web/src/lib/utils.ts`, `web/src/index.css` — design tokens with the light palette on `:root` and the dark one on `.dark`
- `web/vite.config.ts` — the path alias, the vitest block, and the plugin that re-emits `internal/httpapi/dist/.gitkeep`
- `web/tsconfig.json`, `web/package.json`, `web/package-lock.json`, `biome.json`
- `web/src/test/setup.ts` — jest-dom matchers, the `matchMedia` stub jsdom lacks, and the layout stubs Radix needs

**Tests**
- `web/src/hooks/useTheme.test.tsx` (7), `web/src/lib/problem.test.ts` (24), `web/src/components/SudoDialog.test.tsx` (8), `web/src/components/ChainBanner.test.tsx` (7), `web/src/routes/audit.test.tsx` (9), plus 5 taxonomy cases counted within problem.test.ts

## Decisions Made

- **The dashboard reports only what the contract defines.** The plan asked for version, bind address and data directory from `GET /api/v1/system/status`. That endpoint returns `setup_required` and `audit_chain`, full stop — fixed by plan 01 and kept by plan 03. Adding the fields client-side would have produced a UI that breaks against the real server; adding them server-side means editing `cmd/holzkubed/main.go`, which belongs to plan 06. The card shows setup state, the chain verdict and the audit file, and says nothing it cannot back.
- **The theme is recomputed, not cached.** A module-level cache would survive a remount, and the reload case D-11 actually cares about would then be proven by the cache rather than by `localStorage`. `useSyncExternalStore` gets a snapshot function that reads storage and `matchMedia` every time; both are deterministic, so the snapshot is stable.
- **The request `init` is built once.** Rebuilding it for the retry would still usually be identical, and would stop being identical the first time somebody makes a header depend on state. Reusing the object makes the guarantee structural.
- **`GET /api/v1/auth/me` is exempt from the 401 interceptor.** Its 401 means "not signed in", which is the answer the shell asked for. Treating it as an expiry would greet a first-time visitor with "your session ended". `POST /api/v1/auth/sudo` is exempt from both interceptors: a wrong password there is 401 on the credential, not on the session.
- **`next-themes` was removed and the copied `ui/sonner.tsx` was rewired to our own hook.** shadcn generates it against `next-themes`; D-11 specifies one theme with two values and no second theming layer. Editing the copy is exactly what D-12 buys.
- **The sudo prompt names the action from a table in `api.ts`.** One entry today (`/api/v1/account/password`). Anything unlisted still works and says "This destructive action" — degrading to a vaguer sentence rather than to a wrong one.
- **The audit page is a button, not infinite scroll.** With forensic data a page you can name and come back to is worth more than a fluid one you cannot.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] `emptyOutDir` had been deleting the tracked `internal/httpapi/dist/.gitkeep` on every web build**

- **Found during:** Task 1, at the commit
- **Issue:** `git status` showed `D internal/httpapi/dist/.gitkeep` after the first `vite build`. That file is tracked on purpose — `.gitignore` says so in a comment — because `go:embed` fails outright on a missing directory, so a fresh clone cannot `go build` without it. The deletion has been happening since plan 01 and was about to be committed as part of this plan's changes.
- **Fix:** A four-line Rollup plugin in `web/vite.config.ts` re-emits `.gitkeep` as part of the bundle, so it survives `emptyOutDir` whether the build came through the Taskfile or through a bare `npm run build`.
- **Verification:** `git status` clean after a rebuild; `ls internal/httpapi/dist/.gitkeep` present; no deletions in the diff of any of this plan's four commits.
- **Committed in:** `da11135`

**2. [Rule 3 - Blocking] `web/tsconfig.json` had to be modified although it is not in `files_modified`**

- **Found during:** Task 1
- **Issue:** The plan's own action text requires the shadcn path alias in "`vite.config.ts` und `tsconfig.json`", but `tsconfig.json` is absent from the plan's `files_modified` list. Separately, TypeScript 7.0.2 has **removed** `baseUrl` (`error TS5102`), and the `types` array was pinned to `["vite/client"]`, which excludes the vitest globals and the jest-dom matchers the plan's own tests need.
- **Fix:** `paths: { "@/*": ["./src/*"] }` without `baseUrl`; `types` extended with `vitest/globals` and `@testing-library/jest-dom`.
- **Verification:** `npm --prefix web run typecheck` exits 0.
- **Committed in:** `da11135`

**3. [Rule 3 - Blocking] `biome.json` had to be modified so that `npm --prefix web run lint` can parse the CSS**

- **Found during:** Task 1
- **Issue:** shadcn writes Tailwind 4 directives (`@custom-variant`, `@apply`) into `web/src/index.css`. Biome 2.5.11 refuses to parse them unless `css.parser.tailwindDirectives` is on, so the plan-level verification "`npm --prefix web run lint` meldet keine Fehler" could not pass. `biome.json` lives at the repository root and is outside this plan's declared files.
- **Fix:** Enabled `css.parser.tailwindDirectives` in `biome.json`. Nothing else in that file changed.
- **Verification:** `npm --prefix web run lint` reports no findings across all 41 files.
- **Committed in:** `da11135`

**4. [Rule 3 - Blocking] The `lint` script keeps `--config-path ..`**

- **Found during:** Task 1
- **Issue:** The plan specifies `"lint": "biome check src"`. `biome.json` is at the repository root, not in `web/`, so the literal script finds no configuration — plan 01 already hit this and solved it with `--config-path ..`.
- **Fix:** Left the existing script shape and added only the three new scripts (`typecheck`, `test`, `test:watch`). The acceptance criterion checks that a `lint` script exists, which it does.
- **Committed in:** `da11135`

**5. [Rule 1 - Bug] `next-themes` removed and `ui/sonner.tsx` rewired**

- **Found during:** Task 1
- **Issue:** `shadcn add sonner` generates a component importing `useTheme` from `next-themes` and installs that package. holzkube has its own two-value theme (D-11) and adding a second theming layer would mean two sources of truth for the same class on `<html>`.
- **Fix:** The copied component now imports `@/hooks/useTheme`; `next-themes` uninstalled. The comment in the file records why the copy diverges from upstream.
- **Verification:** `grep next-themes web/package.json` finds nothing; build, typecheck and lint clean.
- **Committed in:** `da11135`

**6. [Rule 3] Task boundaries reshuffled so every commit builds**

- **Found during:** Task 1
- **Issue:** The plan puts `setup.tsx`, `login.tsx`, `error.tsx`, `Toaster.tsx` and `useSession.ts` in Task 2, while Task 1 must produce a router tree that includes `/setup` and `/login` and an error boundary pointing at `error.tsx`. Task 1 cannot compile without them.
- **Fix:** Task 1 created working versions of those five files by moving the tracer's forms out of `App.tsx`; Task 2 replaced them with the taxonomy-driven implementations (field errors, the 409 refusal, the 429 countdown, the interceptors). No file was ever committed as a stub, and every commit builds, typechecks, tests and lints on its own.
- **Committed in:** `da11135`, `1ce0701`

### Contract discrepancies, recorded rather than papered over

**7. Radix ships as the unified `radix-ui` package, so `grep -c '@radix-ui/' web/package.json` returns 0**

- **Criterion:** Task 1 requires `grep -c '@radix-ui/' web/package.json` ≥ 1, glossed as "the primitives are a real dependency, the copied components live in the repository".
- **Reality:** shadcn 4.19.0 generates components that import `{ Dialog as DialogPrimitive } from 'radix-ui'` — the unified package — and installs `radix-ui@1.6.7`. There are no scoped `@radix-ui/*` entries.
- **Handling:** Not forced. Satisfying the letter would mean rewriting the import of every generated component to scoped packages, producing worse code to make a grep happy. The stated intent holds: `grep -cE '"(radix-ui|@radix-ui/)"' web/package.json` returns 1, and all twelve components are source files under `web/src/components/ui/`.

**8. Six placeholder routes, not seven**

- **Criterion:** "jede der sieben Platzhalter-Routen rendert eine Seite, die den Bereichsnamen und die zuständige Phasennummer nennt", with `/audit` listed among them.
- **Reality:** `/audit` is a real, fully built page in this same plan (D-13, Task 3). The genuinely unbuilt areas are six: Nodes and Clusters (phase 3), Jobs (phase 6), Config (phase 7), Upgrades (phase 9) and Settings (phase 10).
- **Handling:** All six render their area name and their phase; verified by a probe suite that was run and then removed (see Known Stubs). Settings is not assigned a phase anywhere in the plan; phase 10 was taken from the ROADMAP, where backup/restore and the version range live.

**9. The dashboard cannot show version, bind address or data directory**

- **Criterion:** Task 1 asks the dashboard to show "Version, Bind-Adresse, Datenverzeichnis und Zustand der Audit-Kette aus `GET /api/v1/system/status`".
- **Reality:** `docs/api-contract.md` § *System Status Contract* and `internal/httpapi/handlers/system.go` both define exactly two fields: `setup_required` and `audit_chain`. Three of the four requested values do not exist.
- **Handling:** The card renders the two that do, plus the audit file path. Inventing the fields client-side would break against the real server; adding them server-side would edit `cmd/holzkubed/main.go`, which belongs to plan 06 and is outside this plan's scope. Flagged for plan 06 or a later phase as a possible contract extension.

---

**Total deviations:** 9 (1 Rule 1 bug found and fixed, 4 Rule 3 blockers, 1 Rule 1 dependency correction, 3 contract discrepancies recorded rather than forced).
**Impact on plan:** No scope creep. Deviation 1 fixed a pre-existing defect that would otherwise have been committed under this plan's name. Deviations 7–9 are places where the plan's letter and the binding contract disagree; in each case the contract won and the disagreement is written down rather than hidden behind a satisfied grep.

## Known Stubs

| Stub | File | Reason / resolved by |
|---|---|---|
| The sudo prompt names only `/api/v1/account/password`; every other destructive route says "This destructive action" | `web/src/api.ts` (`ACTION_LABELS`) | It is the only destructive route that exists. Phase 6 (node actions) and phase 9 (etcd) each add one line. |
| The placeholder-page probe suite was run and deleted | — | Adding a sixth test file would have exceeded the plan's declared files. The structural guarantee (routes derived from `NAV_AREAS`) is permanent; the per-page assertion is recorded in coverage D2 as a run, not as a standing test. |
| No test mounts `AppShell`, `setup.tsx` or `login.tsx` | `web/src/components/AppShell.tsx`, `web/src/routes/{setup,login}.tsx` | The plan names five test files and all five exist. The setup redirect, the 409 rendering and the 429 countdown are held by code review plus the live-binary run. |
| `/settings` is assigned to phase 10 by inference | `web/src/components/Sidebar.tsx` | The plan's phase mapping names Nodes/Clusters/Config/Jobs/Upgrades but not Settings. Phase 10 is where the ROADMAP puts backup/restore and the version range. |
| No browser-level coverage of any kind | `web/` | Playwright is not in phase 1 scope; concern D2 from plan 01 stays partially open. |

None of these prevents the plan's goal: the shell stands, the error taxonomy renders, and the audit view proves `store → API → UI` on real records.

## Threat Flags

None. Every file written here is presentation over server state.

- **T-01-31** (stored strings in the audit table): held. `grep -rn dangerouslySetInnerHTML web/src` returns nothing; every value goes through a React text node.
- **T-01-32** (security state in the browser): held. `localStorage` carries the theme and nothing else; the session and the sudo window live server-side and the UI only reflects them.
- **T-01-33** (UI confirmation mistaken for a control): held. `SudoDialog` renders the server's 428 and grants nothing; the replay is refused again if the window did not actually open.
- **T-01-34** (a chain break clicked away): mitigated and now **tested**, which is what the plan asked for.
- **T-01-36** (a request without the CSRF preconditions): held. `web/src/api.ts` is still the only file calling `fetch` (`grep -rn 'fetch(' web/src --include=*.ts --include=*.tsx` outside the test files matches only `api.ts`), and the live server confirmed that a drifted header value is refused.

## Issues Encountered

- **shadcn 4.19 requires a preset and will not take `--defaults` alone.** `init -b radix -y` still prompted; `-p radix-nova` is rejected; `-p nova` works. The generated `style` in `components.json` is then `radix-nova`, which is what the `-b radix` flag actually selects.
- **TypeScript 7.0.2 removed `baseUrl`.** The first `paths` attempt failed with `TS5102`. `paths` alone, relative to the config file, is the supported form.
- **`vite.config.ts` needed `defineConfig` from `vitest/config`, not from `vite`.** The `test` block is not part of Vite's own `UserConfigExport`. A `fileURLToPath` import from `node:url` was also dropped in favour of `new URL(...).pathname`, which avoids adding `@types/node` for one line.
- **Two tests initially produced an unhandled rejection.** A promise that rejects at a click and is only observed on the next line is flagged by Node before the assertion attaches. Fixed by attaching the handler at creation and asserting on the settled value.
- **Every behavioural group was re-run against a deliberately broken implementation before being trusted**, and each control failed the expected tests and only those: theme resolution ignoring storage (4 failures), the sudo replay removed (1), the CSRF value drifted to `'true'` (1), the `/auth/me` exemption removed (1), `next_cursor` compared for truthiness (1), a Dismiss button added to the banner (2), the banner rendering while intact (2), the hash fields dropped from the detail dialog (1).

## User Setup Required

None — no external service configuration.

## Next Phase Readiness

**Ready for plan 06.** Nothing in `Taskfile.yml`, `.golangci.yml`, `.goreleaser.yaml`, `README.md`, `internal/config/`, `internal/tlsx/` or `cmd/holzkubed/main.go` was touched. The one thing plan 06 should know: `web/package-lock.json` is in sync and `npm --prefix web ci --dry-run` exits 0, so the `npm install` → `npm ci` tightening in `build:web` is now safe. `biome.json` gained one CSS parser option and is otherwise unchanged.

**Carried forward:**

1. **The UI is still unverified in a real browser.** Component rendering is now tested in jsdom and the request pipeline was driven against the live binary over HTTPS, but no browser is automated. Concern D2 from plan 01 is narrowed, not closed.
2. **`GET /api/v1/system/status` is thinner than the dashboard wants.** Version, bind address and data directory are not in the contract. If they are wanted, that is a deliberate contract extension, not a client-side fix.
3. **The bundle is 466 KB (143 KB gzipped) in one chunk.** It goes into the binary, so this is size rather than latency, but phase 5 onwards will add xterm and CodeMirror. Code splitting is worth a decision before then rather than after.
4. **Three plan criteria disagree with the binding contract** (deviations 7–9). None was forced; each is recorded above so a reviewer sees the disagreement rather than a satisfied grep.

---
*Phase: 01-foundation-skeleton*
*Completed: 2026-08-28*

## Self-Check: PASSED

**Files claimed as created — all present on disk:** the 26 named source, test and config files, plus all twelve `web/src/components/ui/` copies (`badge`, `button`, `card`, `dialog`, `dropdown-menu`, `input`, `label`, `select`, `separator`, `skeleton`, `sonner`, `table`) — FOUND.

**Commits — all five present in `git log`:** `da11135`, `1ce0701`, `f2902e6`, `eeabd47`, `1879732` — FOUND.

**Plan-level verification re-run at close-out:**

| Check | Result |
|---|---|
| `npm --prefix web install` | PASS |
| `npm --prefix web run build` | PASS (466 KB JS, 47 KB CSS into `internal/httpapi/dist`) |
| `npm --prefix web run typecheck` | PASS |
| `npm --prefix web test` | PASS (5 files, 60 tests) |
| `npm --prefix web run lint` | PASS (41 files, no findings) |
| `npm --prefix web ci --dry-run` | PASS — lockfile in sync |
| `git ls-files --error-unmatch web/package-lock.json` | PASS |
| `go build ./...` | PASS |
| `go test ./... -count=1` | PASS (all packages) |

**Task acceptance criteria — all re-run:**

| Criterion | Result |
|---|---|
| `web/package.json` has `typecheck`, `test`, `lint` scripts | PASS |
| `grep -c "jsdom" web/vite.config.ts` ≥ 1 | PASS (1) |
| `web/src/components/ui/` has `button`, `input`, `table`, `dialog` | PASS |
| `grep -c '@radix-ui/' web/package.json` ≥ 1 | **FAIL as written (0)** — Radix ships as the unified `radix-ui` package; `grep -cE '"(radix-ui\|@radix-ui/)"' ` returns 1. See deviation 7. |
| `web/src/routes/__root.tsx` contains `createRootRoute` | PASS (2) |
| `web/index.html` sets the theme class before the first paint | PASS |
| `grep -icE '"(i18next\|react-i18next\|react-intl\|@lingui/core\|next-intl)"' web/package.json` is 0 | PASS |
| `@biomejs/biome` still pinned at 2.5.11 | PASS |
| Each placeholder route names its area and its phase | PASS (six routes; the plan says seven and counts `/audit`, which is a real page — see deviation 8) |
| `grep -c 'retry\|replay' web/src/api.ts` ≥ 1 | PASS (3) |
| `web/src/lib/problem.ts` covers every code prefix of the taxonomy | PASS (13 codes, asserted in `problem.test.ts`) |
| `grep -c 'next_cursor !== null' web/src/routes/audit.tsx` ≥ 1 | PASS (1) |
| `grep -cE 'next_cursor\??: *number *\| *null'` over problem.ts, api.ts, audit.tsx ≥ 1 | PASS (1, in `api.ts`) |
| `grep -icE 'onClose\|dismiss\|setHidden\|acknowledge' web/src/components/ChainBanner.tsx` is 0 | PASS (0) |
| 428 opens the prompt; the replay carries an identical body and identical headers; cancel replays nothing | PASS (unit + live binary) |
| A 401 during work becomes a login transition with an explanation | PASS (unit) |
| A 429 shows the remaining wait | PASS (unit, in `SudoDialog.test.tsx`; the login countdown is code-review only) |
| Setup redirect while `setup_required`; the 409 shown on a direct hit | PASS (live binary; client side by review) |
| The audit table shows records newest-first, filters, and an explained empty result | PASS (unit + live binary) |
| A row click opens a dialog with `seq`, `prev_hash`, `hash` and the redacted parameters | PASS (unit) |
| The chain banner names file and line, has no interactive element and no closing name; renders nothing when intact | PASS (unit + live binary verdict shape) |

**Hygiene:** no file deletions in any of this plan's commits (`git diff --diff-filter=D` empty across `fdc02ce..HEAD`); `web/node_modules/` never staged; `.gsd/` and `.planning/milestone.lock` untouched and gitignored; no `--no-verify` used.

**One criterion fails as literally written** (`@radix-ui/`), for the reason recorded in deviation 7. It is reported rather than worked around.
