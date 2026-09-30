---
phase: 14-telefon-tippziele
plan: 03
subsystem: testing
tags: [layout-audit, playwright, fixtures, zod, tap-targets]
status: complete

requires:
  - "14-01: scoped finders, opener runner, request monitor, the Sudo dialog opener"
provides:
  - "web/fixtures/host-helper-installed.json: the /host actions object with the helper installed, parsed with hostSchema"
  - "demo.json /api/v1/machines/m-cp-1/reset-preview, as the handler builds it, parsed with resetPreviewSchema"
  - "layout-audit.mjs: nine openers (17 runs), disabled controls measured, [data-slot data-size] in each SMALL detail line, per-path page-route overrides, open-less states with an enabled-control requirement, per-opener check and named close"
  - "the phase's starting finding: $HOME/.cache/holzkube-manager-layout/14-03-first-red.log (outside the repository)"
affects: [14-04, 14-05, 14-06, 14-07]

actuals:
  tokens: 6200
  tasks: 2
  commits: 2
plan_head_before: da20115510d6d6d6551e6e80988255a3bfd73347
plan_head_after: fb2a3b3f18f81dd080960355468a544f2dcfb6ab

tech-stack:
  added: []
  patterns:
    - "Opener override: one page.route per API path with a URL predicate (pathname ===), installed before goto, page.unroute after the close; non-GET falls back"
    - "Open steps are waited for (first().waitFor attached, 5 s) rather than counted at once, so a menu item rendered by the previous click is found"
    - "An opener's `open` may be a function of the width (What's new opens the drawer first at 390)"

key-files:
  created:
    - web/fixtures/host-helper-installed.json
  modified:
    - web/fixtures/demo.json
    - web/src/fixtures.test.ts
    - web/scripts/layout-audit.mjs

key-decisions:
  - "No new exemption. The skips are exactly visibility hidden, display none, aria-hidden on the element, a zero-sized box, and a link in running text, each with its reason in findSmallTargets' doc comment"
  - "The primitive shown is the element's own data-slot/data-size, falling back to its label's when the label is the target"
  - "A state with no open steps skips the close check and fails as OPENER when no enabled control is inside `expect`"

requirements-completed: []
requirements-partial: [MOB-02, MOB-03]

duration: 20min
completed: 2026-09-30
---

# Phase 14 Plan 03: The remaining openers and the first extended run Summary

**The layout audit now opens and measures nine states, 17 runs across both widths: the drawer, What's new, the Power menu and its confirmation, the Reset dialog with its disk rows, a Select list, the sudo dialog, and /host's action group and dialog with the helper installed. It also measures disabled controls, and each small-target line names the primitive it came from. The first run is red with 7 findings and nothing else: no OPENER, EMPTY or EXECUTED line, and the only non-GET requests were the sudo opener's two POSTs, both answered 428.**

## Where it ran

On the operator's Pi (aarch64). Node came from nvm and Go from `~/.local/go`. No `-race` run. Nothing was installed, pushed or released. `holzkube-manager.service` and `/var/lib/holzkube-manager` were not addressed. Every audit run used its own daemon on 127.0.0.1 with a temporary data directory.

## Performance

- **Duration:** about 20 min (09:45 to 10:05 UTC)
- **Tasks:** 2, each committed
- **Files:** 1 created, 3 modified

## Task 1: fixtures, red first

- **Red run 1** (the new tests written, neither file present): `vitest run --project jsdom src/fixtures.test.ts` gave **exit=1**, `Failed to resolve import "../fixtures/host-helper-installed.json"`.
- **Red run 2** (helper file created, reset preview key still missing): **exit=1**. The two reset-preview cases failed and 36 passed.
- **Green:** 38 of 38 passed, exit 0. `go test -count=1 ./internal/publicrepo/` exited 0 with the new file staged; the gate reads `git ls-files`.
- **Fault reinstated:** `"available": false` put back into host-helper-installed.json made the helper-installed case fail (exit=1). The file was then restored.
- **The plan's three `node -e` checks** all exit 0, including the check that the default `/api/v1/host` entry is still helper-missing.
- **Where the values come from:** every value in the reset preview comes from the m-cp-1 fixture (hostname, confirm phrase, both disks) or from `resetPreview` in `handlers/jobs.go` (modes, defaults, the talosctl warning). The helper-installed `result.message` is the default demo entry's own sentence.

## Task 2: the first extended run (the phase's starting finding)

`./bin/task test:layout > "$HOME/.cache/holzkube-manager-layout/14-03-first-red.log" 2>&1` gave **exit=201**, which is Task's code for a failed command. The log reports `7 finding(s) across routes, openers and widths.`

The plan's checks, with their results:

- **The run is red:** `test $? -ne 0` → 0.
- **Nothing failed to open, measured nothing, or carried out an action:** `! grep -qE '^  (OPENER|EMPTY|EXECUTED) '` → 0.
- **The host action dialog was measured:** `grep -c ' · Host action dialog'` → 2.
- **All 17 opener runs are present:** each (width, route, name) has exactly one line matching `^  (ok|SMALL|…|CUT OFF) +<w>px  <route> · <name>`.
- **The only non-GET requests are the sudo opener's:** `REQUEST` = 2 and `REQUEST   POST /api/v1/users -> 428` = 2.
- **The script never skips disabled controls or types into a confirmation field:** `grep -c "hasAttribute('disabled')"` = 0 and the `fill('#…confirm|sudo-password')` grep = 0.
- **The audit uses the helper-installed fixture:** `grep -c host-helper-installed.json` = 2.
- **The route pass is still clean:** all 54 route lines are `ok`, with 64 `ok` lines in total.

### (a) Primitives in `web/src/components/ui/*`, for 14-05

```
  SMALL      390px  / · What's new  (3)
              <button> 28x28 [dialog-close icon-sm] "Close" .group/button inline-flex shrink-0 items-center jus
  SMALL      390px  /nodes/m-cp-1 · Power confirmation  (1)
              <button> 28x28 [dialog-close icon-sm] "Close" ...
  SMALL      390px  /nodes/m-cp-1 · Reset dialog  (3)
              <button> 28x28 [dialog-close icon-sm] "Close" ...
  SMALL      390px  /host · Host action dialog  (1)
              <button> 28x28 [dialog-close icon-sm] "Close" ...
  SMALL      390px  /nodes/m-cp-1 · Power menu  (2)
              <div> 312x28 [dropdown-menu-item] "Disable" .group/dropdown-menu-item relative flex cursor-defa
              <div> 312x28 [dropdown-menu-item] "Restart" .group/dropdown-menu-item relative flex cursor-defa
  SMALL      390px  /audit · Select list  (15)
              <div> 219x28 [select-item] "Any action" .relative flex w-full cursor-default items-center g
              ... 14 more [select-item] rows at 219x28: setup.create, auth.login, auth.logout, auth.sudo,
              account.password, schematic.create, schematic.delete, cluster.fingerprint, cluster.import,
              cluster.lock, cluster.forget, machine.add, machine.refresh, machine.forget
```

These come down to three primitives:

- **The dialog close X:** `dialog.tsx`, Button `icon-sm`, 28 px. It shows up in four dialogs. The sudo dialog has no close X, so it is `ok`.
- **The dropdown menu item:** `dropdown-menu.tsx`, 28 px. Items with a reason line under them are already taller, so they pass.
- **The select item:** `select.tsx`, 28 px.

### (b) Single places, for 14-06

```
  SMALL      390px  / · Navigation  (14)
              <a> 199x32 "Wall" .flex items-center gap-2 rounded-md border-l-2 bord
              ... 12 more nav links at 199x32: Dashboard, Nodes, Host — warning, Clusters, Kubernetes,
              Config, Jobs, Provision, Upgrades, Images, Audit, Settings
              <button> 183x16 "dev" .w-full rounded px-1 text-left text-xs text-sidebar
  SMALL      390px  / · What's new  (3)
              <button> 45x30 "v0.1" .rounded-md border px-2.5 py-1 text-sm focus-visibl
              <button> 50x30 "v0.0" .rounded-md border px-2.5 py-1 text-sm focus-visibl
  SMALL      390px  /nodes/m-cp-1 · Reset dialog  (3)
              <input> 13x13 "Wipe /dev/nvme0n1" .
              <input> 13x13 "Wipe /dev/mmcblk0" .
```

- **Nav links, 32 px:** `web/src/components/Sidebar.tsx`.
- **The version button "dev", 16 px high:** `web/src/components/WhatsNew.tsx`.
- **The version chips in the release notes, 30 px:** `web/src/components/WhatsNew.tsx`, in ReleaseNotes.
- **The disk checkboxes, 13×13 and not inside a label:** `web/src/components/NodeActions.tsx`, ResetDialog, around lines 340-345. By contrast, the graceful and reboot checkboxes are measured through their labels and pass.

The Navigation opener printed **no CUT OFF**: at 390×844 with 32 px links, the drawer fits. Once the links reach 44 px, the drawer grows by 13 × 12 px, and Pitfall 8 says that is when the check matters.

The plan expected the dialog close X at 28 px to be among the findings, and it is. The `/host` buttons in the helper-missing state are now measured while disabled, and they are all at least 44 px at 390.

## Red runs for the new rules (method: the fault put back)

The exit code was read from the command itself each time. The file was restored from a saved copy and checked with `cmp` against it (`cmp=0`) before the commit.

1. **Three faults in one run, exit=201.**
   - **The drawer shifted by `translateX(-50px)` and set to `height: 300px`:**
     `  OPENER     390px  / · Navigation -- the drawer did not slide in`
     `  CUT OFF    390px  / · Navigation -- the drawer is 709px tall in 300px and does not scroll`
   - **Both /host openers given the default helper-missing answer**, at 390 and at 1280:
     `  OPENER     390px  /host · Host actions (helper installed) -- the helper-installed answer did not reach the page`
     `  OPENER     390px  /host · Host action dialog -- could not use "Restart host": locator.click: Timeout 5000ms exceeded.`
   - **The `disabled` skip put back:** `/host` dropped from 14 to 10 measured controls (its four disabled buttons), `/settings` from 25 to 19, `/upgrades` from 16 to 12, and the Reset dialog at 1280 from 11 to 10. No verdict flipped, because every disabled control on the screens measured today is already at least 44 px at 390. Today the rule shows up only in the counts.
   - **The close tap moved onto the drawer (x 100, `force`):** this was not a valid fault. The tap landed on a nav link, which navigated and closed the drawer anyway, so it proved nothing. That is why run 2 was needed.
2. **The drawer's close replaced with a no-op, exit=201:**
   `  OPENER     390px  / · Navigation -- still open after a tap on "Close the navigation" beside the drawer`

## Task Commits

1. **Task 1:** `6ba501d` (test): /host with the helper installed and m-cp-1's reset preview, held to the product's schemas
2. **Task 2:** `fb2a3b3` (feat): the audit opens nine states, measures disabled controls, names the primitive

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] The quick vitest command was run from `web/`**
- This is the same issue as in 14-01: `npm --prefix web exec -- vitest ... --project jsdom` finds no projects when run from the repository root. Running it from `web/` works, and nothing else changes.

**2. [Rule 1 - Bug] Open steps are waited for instead of counted at once**
- **Found during:** Task 2
- **Issue:** the runner checked `target.count() === 0` right after the previous click. The Power confirmation's menu item exists only once the menu has rendered and the power answer has arrived, so an immediate count could report a missing control that was about to appear.
- **Fix:** `target.first().waitFor({ state: 'attached', timeout: 5_000 })`. It prints the same OPENER line on timeout.
- **Commit:** fb2a3b3

**3. The close step became `{ name, run }`**
- An opener's close is now an object with a name and a function, so the "still open after …" line names the close that was used, whether that is Escape or the tap beside the drawer. The plan did not fix a shape for this.

### Exemptions

None added.

## Known Stubs

None.

## Threat Flags

None.

- **T-14-06 and T-14-07 are in place.** The grep gate reports 0 for any `fill` into `#host-action-confirm`, `#reset-confirm` or `#sudo-password`. No confirm button is addressed by any opener. The only REQUEST lines are the sudo opener's 428s.
- **T-14-08 is in place.** `internal/publicrepo` passes with the new file tracked, and every fixture value was copied from existing fixture bytes or from the handler source.

## Next Phase Readiness

- **14-04:** takes the 1280 before-dump before the first class change.
- **14-05:** gets the three primitive findings in (a).
- **14-06:** gets the four single places in (b).
- **After the nav links grow**, the Navigation opener's CUT OFF check becomes the one that matters (Pitfall 8).

## Self-Check: PASSED

- FOUND: web/fixtures/host-helper-installed.json, web/scripts/layout-audit.mjs, $HOME/.cache/holzkube-manager-layout/14-03-first-red.log
- FOUND: commits 6ba501d, fb2a3b3
