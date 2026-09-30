---
phase: 14-telefon-tippziele
plan: 01
subsystem: testing
tags: [layout-audit, playwright, tanstack-router, vitest, sudo]
status: complete

requires: []
provides:
  - "web/src/routeTree.ts: the route tree as a side-effect-free module (App.tsx and the guard import it)"
  - "web/scripts/layout-routes.json: the audited route list, 27 router leaves"
  - "web/src/layoutRoutes.test.ts: the route guard, chained ahead of the browser in npm run test:layout"
  - "layout-audit.mjs: scoped finders, opener runner (OPENER/EMPTY), request monitor (REQUEST/EXECUTED), the Sudo dialog opener"
affects: [14-03, 14-04, 14-05, 14-06, 14-07]

actuals:
  tokens: 10779
  tasks: 3
  commits: 2
plan_head_before: 567cd4c1c455e8f5cb8ea4c08bd6fd9c60ac92f8
plan_head_after: 2b7bdcba65605666243d0a54a9e574f7b8dc628f

tech-stack:
  added: []
  patterns:
    - "Route guard: createRouter first (fullPath exists only after init), walk children to leaves, one problem line per mismatch, expect(problems).toEqual([])"
    - "Opener: fresh page, steps with missing-control check, expect visible <= 5 s, finite animations finished, measurement scoped via locator.evaluate(fn) on the opened element, close, expect hidden <= 5 s"
    - "Request monitor attached only for an opener's span; 2xx to a non-GET action path is EXECUTED and red"

key-files:
  created:
    - web/src/routeTree.ts
    - web/scripts/layout-routes.json
    - web/src/layoutRoutes.test.ts
    - .planning/phases/14-telefon-tippziele/deferred-items.md
  modified:
    - web/src/App.tsx
    - web/package.json
    - web/scripts/layout-audit.mjs

key-decisions:
  - "D-05 held on the real path: the Sudo dialog opener submits the New account form on /settings and the audit's own daemon answers POST /api/v1/users with 428 at both widths; the fixture-428 fallback was not used"
  - "A1 held: router.matchRoutes(path).at(-1).routeId equals the leaf's id; getMatchedRoutes was not needed"
  - "The opener's `measured` count is taken after the visibility and size skips (inline links in running text are counted as present, then exempted from the size verdict); the route pass prints this count instead of the old narrow countTargets"
  - "The New account password field is located as `form:has(#new-username) #new-password`, because /settings renders two #new-password inputs"

patterns-established:
  - "OPENERS entries: { name, route, widths, open: [{ name, locate, fill? }], expect, close?, fixture? }"

requirements-completed: []
requirements-partial: [MOB-02]

duration: 21min
completed: 2026-09-30
---

# Phase 14 Plan 01: Route list from the router, first opener end to end Summary

**The layout audit's route list now comes from the router, and a jsdom guard holds it there before a browser starts. The audit also opens its first tapped state: the sudo dialog, reached through its own daemon's real `428 sudo.required`, measured only inside the dialog, closed with Escape, and a request monitor shows that nothing was carried out.**

## Where it ran

On the operator's Pi (aarch64). Node came from nvm and Go from `~/.local/go`. No `-race` was run; the race verdict belongs to CI. Nothing was installed. `task build:go` runs `npm ci` from the lockfile as always, and that adds no new packages. Nothing was pushed or released. `holzkube-manager.service` and `/var/lib/holzkube-manager` were not touched. Every audit run used its own daemon, with a temporary data directory, on 127.0.0.1 at a port the kernel picked.

## Performance

- **Duration:** about 21 min
- **Started:** 2026-09-30T09:09:37Z
- **Completed:** 2026-09-30T09:30Z
- **Tasks:** 3 (Task 3 changes nothing and has no commit, as the plan says)
- **Files modified:** 7

## Accomplishments

- **The route tree is its own module.** `web/src/routeTree.ts` contains only `export const routeTree`. `App.tsx` builds the router from it; `defaultPreload`, `Register` and `App` are unchanged.
- **`web/scripts/layout-routes.json`** has 27 entries. `/setup` is marked `when: before-account`, `/login` is marked `when: before-session`, and `/wall` carries a note. Two entries have example paths: `/nodes/$uuid` → `/nodes/m-cp-1`, and `/kubernetes/apps/$namespace/$kind/$name` → `/kubernetes/apps/media/Deployment/jellyfin`.
- **The route guard** (`web/src/layoutRoutes.test.ts`, 3 tests) reports each of these on its own line: a missing route, a stale route, a route listed twice, an unknown key or `when`, and a parameter route without an example. It also checks that each example path matches its leaf, and it fails if it sees fewer than 20 leaves or a key named "undefined".
- **The guard runs before the audit:** `test:layout` is now `vitest run --project jsdom src/layoutRoutes.test.ts && node scripts/layout-audit.mjs`. CI calls the same npm script, so CI runs the guard too.
- **`layout-audit.mjs`:**
  - It reads the route list from the JSON, and throws if the before-account or before-session entry is missing or a `when` is unknown.
  - The finders take a root element. `findSmallTargets` returns `{ small, measured }`.
  - A new opener runner reports `OPENER` and `EMPTY` lines.
  - A new request monitor reports `REQUEST` and `EXECUTED` lines.
  - The Sudo dialog opener is added.
  - The green line and the failure text were updated.

## Green gate run

`./bin/task test:layout > log 2>&1; echo "exit=$?"` gave **exit=0**, with:

```
  REQUEST   POST /api/v1/users -> 428
  ok         390px  /settings · Sudo dialog  (3 controls)
  REQUEST   POST /api/v1/users -> 428
  ok        1280px  /settings · Sudo dialog  (3 controls)

Nothing out of reach at 390px and 1280px, every control is at least 44px at 390px, on 27 routes and in 1 opened states.
```

After Task 1 alone, the gate also gave exit=0: `... on 27 routes.`, with the same 27 routes at both widths as before (54 `ok` lines). Lint and typecheck exit 0. The whole jsdom suite passes: 56 files, 700 tests, exit 0.

## Red runs (Task 3): every new rule seen red against its fault

In each run the exit code was read from the command itself (`cmd > log 2>&1; echo "exit=$?"`). The file was then restored from a saved copy and checked with `cmp` against `git show HEAD:<file>`, which returned `cmp=0` every time.

1. **`/host` removed from `layout-routes.json`, then `./bin/task test:layout`:** exit=201 (non-zero; Task's code for a failed command).
   `"/host is in the router but not in web/scripts/layout-routes.json: the layout audit would never open it."`
   The log has **0** lines starting with `  ok ` and **0** "Nothing out of reach" lines, so the browser run was never reached.
2. **An added `/gone` entry, quick guard command:** exit=1.
   `"/gone is in web/scripts/layout-routes.json but not in the router."`
3. **The `/nodes/$uuid` path set to `/nodes/$uuid`, quick guard command:** exit=1.
   `"/nodes/$uuid has a parameter but no example path in web/scripts/layout-routes.json."`
4. **A second `/jobs` entry, quick guard command:** exit=1.
   `"/jobs is listed twice in web/scripts/layout-routes.json."`
5. **Step 3's single run, three faults in `layout-audit.mjs` (5 to 7 here):** exit=201 overall, `8 finding(s) across routes, openers and widths.` This fault was an opener whose trigger matches nothing:
   `  OPENER     390px  /settings · Missing trigger -- no control named "No such control" to open it` (and the same at 1280px)
6. **Same run, an opener whose `expect` is the sudo dialog's description paragraph:**
   `  EMPTY      390px  /settings · Empty probe -- opened, but not one control in it was measured` (and the same at 1280px)
7. **Same run, the Sudo opener's POST /api/v1/users fulfilled with 201 `{}`:**
   `  EXECUTED  POST /api/v1/users -> 201 -- the audit carried out an action; it must only open and measure`
   The same opener's own line followed, as expected:
   `  OPENER     390px  /settings · Sudo dialog -- clicked, but [role="dialog"]:has-text("Confirm your password") did not appear within 5 s`

After the restore, the gate on the tree gave exit=0, with the green line quoted above. `git status --porcelain` shows only `.planning/STATE.md`, which the orchestrator had already modified before this plan started.

## D-05: real path or fixture

**The real path held.** The first attempt did stop with an `OPENER` line, `could not use "#new-password": strict mode violation ... resolved to 2 elements`. That was a locator problem, not a missing 428. With the locator scoped to the New account form, both following runs printed `REQUEST   POST /api/v1/users -> 428` at 390 and 1280, and the dialog closed on Escape. The `page.route` fallback with a recorded 428 body was not needed, and it is not in the code.

## A1 (matchRoutes)

**Held.** For all 27 entries, `router.matchRoutes(entry.path).at(-1).routeId` equals the id of the listed leaf, and the example-path test passed on its first run. `getMatchedRoutes` was not needed. Pitfall 12 (A3) did not occur either: importing the whole route set under jsdom works without any change to the test environment.

## Task Commits

1. **Task 1: router leaves are the audit's route list, guarded before the browser:** `e38de11` (feat)
2. **Task 2: the sudo dialog through the real 428, measured in scope, closed, nothing carried out:** `2b7bdcb` (feat)
3. **Task 3: red runs:** no commit, by design (both files restored byte-identical)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] The quick guard command does not run as written**
- **Found during:** Task 1
- **Issue:** `npm --prefix web exec -- vitest run --project jsdom src/layoutRoutes.test.ts` runs vitest in the repository root. It finds no config there and stops with `No projects matched the filter "jsdom"` (exit 1).
- **Fix:** the quick command was run from `web/` instead (`cd web && npm exec -- vitest run --project jsdom src/layoutRoutes.test.ts`). The npm script `test:layout` already runs in `web/`, so the chain itself is not affected, and red run 1 proves that. Later plans that use the quick command should run it from `web/`.
- **Files modified:** none

**2. [Rule 1 - Bug] `#new-password` appears twice on /settings**
- **Found during:** Task 2
- **Issue:** the change-password card and the New account form both render `id="new-password"`, so the opener's locator broke Playwright's strict mode.
- **Fix:** the opener locates `form:has(#new-username) #new-password`. The duplicate id itself is a product accessibility defect outside the tap-target scope, and it is logged in `deferred-items.md`.
- **Files modified:** web/scripts/layout-audit.mjs
- **Commit:** 2b7bdcb

**3. Route-pass counts changed (expected, noted for comparison)**
- The route pass now prints `measured` from `findSmallTargets`' own loop instead of the narrower `countTargets`. It adds `role="tab"`, `menuitem`, `option`, `checkbox`, `switch` and `[tabindex]`, and still skips disabled controls, as this plan requires.
- Some numbers moved as a result. For example, /nodes/m-cp-1 went from 38 to 42, /settings from 27 to 19 (its disabled buttons were counted before and are skipped now), and /clusters from 37 to 31.
- `countTargets` is deleted.
- None of this changes any verdict.

## Known Stubs

None.

## Threat Flags

None. The only new surface is the audit's POST /api/v1/users against its own daemon, which the threat model already covers as T-14-01. The mitigation for T-14-01 is in place: the 428 was seen at both widths, and EXECUTED was seen red. The mitigation for T-14-02 is also in place: `grep -cE "fill\('#sudo-password'" web/scripts/layout-audit.mjs` = 0.

## Next Phase Readiness

- 14-03 can add the remaining openers to `OPENERS` using the same entry shape. The `fixture` override is already in place.
- 14-03 still has to remove the disabled skip and add the drawer's `close` step and its CUT OFF check.

## Self-Check: PASSED

- FOUND: web/src/routeTree.ts, web/scripts/layout-routes.json, web/src/layoutRoutes.test.ts, deferred-items.md
- FOUND: commits e38de11, 2b7bdcb
