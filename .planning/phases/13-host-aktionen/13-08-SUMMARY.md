---
phase: 13-host-aktionen
plan: 08
subsystem: ui
tags: [host-actions, react, radix-dialog, sudo, vitest, playwright, 390px]
status: complete

requires:
  - phase: 13-host-aktionen
    plan: 01
    provides: HostActions tracer (update button, dialog, status box), api.hostActions, HOST_ACTION_PATHS
  - phase: 13-host-aktionen
    plan: 05
    provides: the order state withdrawn (10-s pickup timeout)
  - phase: 13-host-aktionen
    plan: 06
    provides: actions.available / missing / install_commands in GET /api/v1/host
provides:
  - hostSchema.actions with available, missing (script / path-unit / not-enabled), install_commands; order state enum pending / picked-up / withdrawn
  - HostActions (four buttons, disabledReason, the dialog table), HOST_ACTIONS, REASON
  - orderPhase (nine phases), followedOrder, isFinal, HostOrderStatus (Dismiss status, focus on placement)
  - outcomeSentence exported from HostActions (host.tsx UpdateCheck imports it)
  - HostHelperNotice
  - HostView sessionRole prop; HostPage reads useSession().me?.role
  - the waiting notice in the stale notice's place
affects: [13-09, 14]

actuals:
  tokens: 26100
  tasks: 3
  commits: 3
plan_head_before: 4a84aab6bb4a24e138d2f09397b0dce2840c8466
plan_head_after: 614a660

tech-stack:
  added: []
  patterns:
    - "One reason line for a button group: disabledReason returns the first applicable reason, rendered as p#host-actions-reason and wired by aria-describedby only when set"
    - "Order phases derived from server fields only (order state, helper result for the id, process start, observed_at minus uptime, update checked_at)"
    - "428 tested through the real client and the real SudoDialog, with only fetch faked"

key-files:
  created: []
  modified:
    - web/src/api.ts
    - web/src/components/HostActions.tsx
    - web/src/components/HostActions.test.tsx
    - web/src/routes/host.tsx
    - web/src/routes/host.test.tsx
    - web/src/routes/host.browser.test.tsx

key-decisions:
  - "The role prop is sessionRole, not role: biome's a11y/useValidAriaRole reads a JSX role= as an ARIA role and fails lint"
  - "Reason 2 (helper) stands for the server's available=false once the container is ruled out, so the buttons are off exactly when the routes would refuse"
  - "The held order stays until dismissed; the daemon's order and the helper's result are followed only within 15 minutes of the reading (observed_at, the server's clock)"
  - "An order known only from the helper's result says 'recorded {time}' in the meta line, not 'placed': the placement time is not in the answer"
  - "While waiting, the status box keeps the last known sentence (started, or picked up); the waiting notice says the page is waiting"
  - "update-finished and back are checked before the failed poll, so a final phase stays final when a later poll fails"
  - "The status box takes focus only when this page placed the order (followed.from === 'held'), never for an order it merely found"

patterns-established:
  - "HostView tests pass sessionRole; without one the actions are off with the reader reason"

requirements-completed: [HACT-01, HACT-02, HACT-03, HACT-04, HACT-05, HACT-07]

coverage:
  - id: D1
    description: "Four buttons in the header, harmless first, machine-wide pair apart and red; 2 x 2 grid with visible labels below md"
    requirement: HACT-01
    verification:
      - kind: unit
        ref: "web/src/components/HostActions.test.tsx#offers all four to an operator with the helper installed, harmless first, and says nothing"
        status: pass
      - kind: automated_ui
        ref: "web/src/routes/host.browser.test.tsx#lays the four host actions out as two rows of two, each a thumb wide and tall"
        status: pass
    human_judgment: false
  - id: D2
    description: "The one reason the buttons are off: container, helper missing, reader, no hostname, not answering, update running, order waiting"
    requirement: HACT-07
    verification:
      - kind: unit
        ref: "web/src/components/HostActions.test.tsx#HostActions: the four buttons and the one reason they are off"
        status: pass
      - kind: unit
        ref: "web/src/routes/host.test.tsx#hands the session's role to the host actions: a reader is told why they are off"
        status: pass
    human_judgment: false
  - id: D3
    description: "One dialog per action with typed hostname, Keep running, destructive confirm for the machine-wide pair; 428 opens SudoDialog on top and replays; refusals keep the typed text"
    requirement: HACT-05
    verification:
      - kind: unit
        ref: "web/src/components/HostActions.test.tsx#HostActions: one dialog per action"
        status: pass
      - kind: unit
        ref: "web/src/components/HostActions.test.tsx#HostActions: the password step"
        status: pass
    human_judgment: false
  - id: D4
    description: "Order status from placed to back / update finished, Dismiss status per id, focus after placement"
    requirement: HACT-02
    verification:
      - kind: unit
        ref: "web/src/components/HostActions.test.tsx#orderPhase: every phase from server fields"
        status: pass
      - kind: unit
        ref: "web/src/components/HostActions.test.tsx#HostOrderStatus"
        status: pass
      - kind: unit
        ref: "web/src/routes/host.test.tsx#the order status and the waiting notice"
        status: pass
    human_judgment: false
  - id: D5
    description: "Waiting notice instead of the stale one while a started or picked-up order's host does not answer, also on a page that did not place it"
    requirement: HACT-03
    verification:
      - kind: unit
        ref: "web/src/routes/host.test.tsx#says waiting on a page that did not place the order: the second operator's page"
        status: pass
    human_judgment: true
    rationale: "The UI-SPEC backstop: the real two-session reboot on the Pi is not something a component test can do; the unit test holds the derivation"
  - id: D6
    description: "Helper notice: missing pieces in the server's order, install commands in a self-scrolling pre, nothing wider than 390 px"
    requirement: HACT-07
    verification:
      - kind: unit
        ref: "web/src/routes/host.test.tsx#the helper notice"
        status: pass
      - kind: automated_ui
        ref: "web/src/routes/host.browser.test.tsx#fits the helper notice: three missing pieces, the commands scroll inside their own box"
        status: pass
    human_judgment: false
  - id: D7
    description: "A 40-character hostname wraps inside the host action dialog at 390 px"
    requirement: HACT-04
    verification:
      - kind: automated_ui
        ref: "web/src/routes/host.browser.test.tsx#wraps a 40-character hostname inside the Restart host dialog"
        status: pass
    human_judgment: false

duration: 24min
completed: 2026-09-29
---

# Phase 13 Plan 08: Host actions on the page Summary

**`/host` now carries all four host actions with one dialog each (typed hostname, "Keep running", the sudo step through the real SudoDialog), a single line saying why they are off, an order status box that walks placed -> picked up -> started -> back or update finished from server fields alone, a slate waiting notice in place of the amber stale one while the host is gone, and a helper notice listing exactly what is missing with the commands that install it.**

## Where this ran

The operator's Raspberry Pi 5 (aarch64). Vitest's jsdom project and its browser project (Playwright Chromium, already installed under `~/.cache/ms-playwright`), Node v22 via nvm; the Go route guards with Go 1.26.7 via GOTOOLCHAIN. No package installed, `web/package.json` and `web/package-lock.json` unchanged. The production service, its data directory and `/usr/local/bin` were not touched.

## Performance

- **Duration:** ~24 min
- **Started:** 2026-09-29T02:43Z
- **Completed:** 2026-09-29T03:08Z
- **Tasks:** 3
- **Files:** 6 modified

## Accomplishments

- **Schema:** `actions.available`, `actions.missing[]` (`script` / `path-unit` / `not-enabled`, each with its path), `actions.install_commands`, all inside the existing `.default(...)` (older answer: not available, nothing missing); the order's `state` is the enum `pending | picked-up | withdrawn`.
- **Buttons:** Check for updates and install, Restart service, a `w-4 max-md:hidden` spacer, Restart host, Shut down host (the last two `text-destructive`) in the `<fieldset aria-label="Host actions">` from plan 01; `max-md:grid max-md:grid-cols-2` and the phone classes on each button.
- **Reasons (checker resolution 1):** `disabledReason(host, role, pollFailed, order)`: container -> `!available` (helper) -> not operator (also while the role is unknown) -> hostname unreadable -> poll failed (stale or waiting) -> update started -> order placed / picked up. All four off together, one `p#host-actions-reason`, `aria-describedby` only when it is there.
- **Dialog:** an action table with every string verbatim from the Copywriting Contract; red title with `AlertTriangle` and `variant="destructive"` confirm for the machine-wide pair, the red consequence box for Shut down host, the hostname as `font-mono break-all`, the three phone-keyboard attributes, "Keep running", "Working…".
- **Status:** `orderPhase` (nine phases), `followedOrder` (held until dismissed; else the daemon's order; else the helper's result naming an order -- the last two within 15 minutes of `observed_at`), `HostOrderStatus` with slate / emerald / red, "Dismiss status" on final phases, focus once after this page placed an order. `outcomeSentence` moved here; the Service card imports it.
- **Waiting:** in the stale notice's place, slate, the shut-down variant for poweroff; the body keeps `opacity-60`; polling unchanged.
- **Helper notice:** last in the stack when `!container && missing.length > 0`.

## Task Commits

1. **Task 1: four actions, one dialog per action, one reason** - `e36f97c` (feat)
2. **Task 2: placed, picked up, started, back, and waiting** - `51d6558` (feat)
3. **Task 3: the helper notice, and the page measured at 390 px** - `614a660` (feat)

## Fault injections (each put in from a saved copy, run, seen red from vitest's own exit code, restored and `cmp`-checked)

| ID | Injection | rc | Red tests and failing line |
|---|---|---|---|
| F21 | `disabledReason`: the role check replaced by `role === 'never' && ...` (a reader passes) | 1 | `× for a reader: all four still shown, all off, and the role named (F21)`, `× before the role is known: offers nothing` -- `Received element is not disabled` at `HostActions.test.tsx:114` (`expect(b).toBeDisabled()`); and `× hands the session's role to the host actions: a reader is told why they are off` (`host.test.tsx`) |
| T2 | host.tsx: `waitingFor` never set (`phase === 'never'`): a failed poll always shows the stale notice | 1 | six red: the four `a started {restart-service,update,reboot,poweroff} and a failed poll ...`, `a picked-up order and a failed poll: waiting too`, `says waiting on a page that did not place the order: the second operator's page` -- `Unable to find an element with the text: Waiting for holzkube-manager to come back. ...` (and the shut-down variant) at `host.test.tsx:1627`, `:1644`, `:1709` |
| T2b | `orderPhase`: `boot > placed` dropped (back as soon as an uptime is read) | 1 | `× restart host, booted before the order` -- `expected 'back' to be 'started'` |
| T3 | `max-md:grid-cols-2` removed from the group | 1 | browser project: `× lays the four host actions out as two rows of two, each a thumb wide and tall` -- `expected 164 to be 112` at `host.browser.test.tsx:490` (`expect(service.top).toBe(update.top)`); the width check alone stayed green (one column is full width), which is why the rows are asserted |
| T3b | `overflow-x-auto` removed from the commands `pre` | 1 | browser project: `× fits the helper notice ...` -- `overflowing(phone)` listed `text of <pre class="mt-2 rounded-sm bg-muted ..."> "sudo install -o root -g root -m 0755 deploy/holzkube-manager-host.sh ..." ends at 917.0 > 390.5` at `host.browser.test.tsx:531` |

## Verification (exit codes read from the command itself)

- `npx vitest run --project jsdom src/components/HostActions.test.tsx src/routes/host.test.tsx src/fixtures.test.ts src/api.nulls.test.ts`: rc 0 (Task 1: 141 tests).
- `npx vitest run --project jsdom src/components/HostActions.test.tsx src/routes/host.test.tsx`: rc 0 (166 tests after Task 3).
- `npm --prefix web run test:browser -- src/routes/host.browser.test.tsx`: rc 0, 6 tests, Chromium.
- `npm --prefix web run test` (both projects): rc 0, 59 files, 693 tests.
- `npm --prefix web run typecheck`: rc 0. `npm --prefix web run lint`: rc 0 (the 2 warnings and 1 info are pre-existing: DataTable.tsx, wall.test.tsx).
- `go test ./cmd/holzkube-managerd -count=1 -run 'TestEveryRouteIsReachableFromTheInterface|TestEveryRouteTheClientIssuesIsCalledByAScreen|TestNoStaleClientExemptions'`: rc 0, three PASS.
- `git diff --stat -- web/package.json web/package-lock.json`: empty. `go test ./internal/publicrepo/`: ok.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] `role` prop renamed to `sessionRole`**
- **Found during:** Task 1 (lint)
- **Issue:** biome `lint/a11y/useValidAriaRole` fails `<HostActions role="operator">` as an invalid ARIA role.
- **Fix:** `HostActions` and `HostView` take `sessionRole`; HostPage passes `useSession().me?.role` (the plan's `me.data?.role` does not exist: `useSession` returns `me` as the data).
- **Committed in:** e36f97c

**2. [Rule 2 - Missing critical] The status box only takes focus for an order this page placed**
- **Found during:** Task 2
- **Issue:** plan 01's box focused itself for every new id; with the box now following the daemon's order too, a page load or another operator's order would pull focus.
- **Fix:** `takeFocus` prop, true only for `from === 'held'`; tested both ways (`takes focus only when told to`, and the second operator's page asserts no focus).
- **Committed in:** 51d6558

**3. [Rule 1 - Truthfulness] "recorded" instead of "placed" for an order known only from the result**
- **Found during:** Task 2
- **Issue:** after the daemon restarts, only the helper's result names the order; its time is the helper's, and "placed {time}" would state a placement time the answer does not have.
- **Fix:** the meta line says "Order {id} · recorded {time}" for that case; UI-SPEC copy unchanged otherwise.
- **Committed in:** 51d6558

### Additions beyond the plan's lists

- The 428 is tested through the real `SudoDialog` and client (only fetch faked): replay places the order, cancel leaves the host dialog open with its typed text and the confirm enabled.
- Extra injections T2b (back without a later boot) and T3b (pre without overflow-x-auto), both seen red.

**Total deviations:** 3 auto-fixed (1 blocking, 1 missing critical, 1 truthfulness). **Impact:** none on scope; all within the plan's files.

## Known Stubs

None. `web/fixtures/demo.json` still has no `actions` block (so it parses to the default: not available, nothing missing); the helper-missing fixture state, the README host line, the guide and `host.png` are plan 13-09's by design (CLAUDE.md README rule is carried there).

## Backstops (UI-SPEC)

- "A 40-character hostname wraps inside the host action dialog at 390 px": now measured in the browser project (title and label text inside the dialog, the name on more than one line).
- "A /host page that did not place the reboot order shows the waiting notice, not the stale notice": held by the unit test from the last reading; the real two-session reboot remains a manual check.

## Threat Flags

None beyond the plan's register (T-13-46..T-13-50): no new route, no new request; the buttons are display only, the routes stay the lock.

## Next Phase Readiness

- 13-09: fixture (`actions` in the helper-missing state), README "What it does" host line, guide section, contract text for `withdrawn`, `host.png` re-render. The layout audit was not run here (it measures the built fixture, which 13-09 changes).
- Phase 14: the dialog's close X (28 px) is still below 44 px on a phone, as the UI-SPEC left it.

## Self-Check: PASSED
