---
phase: 12-host-verlauf
plan: 06
subsystem: web
tags: [react, host, health, sidebar, wall, vitest, playwright]
status: complete

requires:
  - phase: 12-host-verlauf
    provides: "12-02: host.Health (state, summary, warnings, unreadable) in GET /api/v1/host; 12-03: the wall's host field (name, state, reason); 12-05: the /host page with its charts"
provides:
  - "web/src/components/HostState.tsx: HostStateMark (dot / triangle / hollow ring in one fixed box), STATE_WORD, the 'unanswered' mark state"
  - "hostSchema.health (no default on state or summary) and wallSchema.host (nullish → null)"
  - "/host: header state line (word in a polite live region) and the amber Warning notice between stale and container"
  - "Sidebar: the Host entry's mark from the ['host'] query, 30 s away from /host, none of its own on /host"
  - "wall: the host as the first tile of Nodes, data-kind=host, no trend, not counted in the headline"
  - "demo fixtures in warn: /api/v1/host (cpu_thermal 82.1 °C) and the wall's host"
affects: [12-07, 12-08]

actuals:
  tokens: 12000
  tasks: 3
  commits: 3
plan_head_before: 0b15489654b834c4b66e0f2f79d1358a16060f7e
plan_head_after: 1886bc4ed13ffba6c499b1799d466506afaee20c

tech-stack:
  added: []
  patterns:
    - "A state is drawn, never derived: the mark takes a state, not readings, so there is nothing in it to compute one from"
    - "An unanswered poll is its own mark state ('unanswered'), not the last answer's"
    - "Two observers of one query key with different intervals: the navigation's is switched off on the page that already polls"

key-files:
  created:
    - web/src/components/HostState.tsx
    - web/src/components/Sidebar.test.tsx
  modified:
    - web/src/api.ts
    - web/src/routes/host.tsx
    - web/src/routes/host.test.tsx
    - web/src/routes/host.browser.test.tsx
    - web/src/components/Sidebar.tsx
    - web/src/routes/wall.tsx
    - web/src/routes/wall.test.tsx
    - web/fixtures/demo.json
    - web/src/fixtures.test.ts

key-decisions:
  - "The line form of HostStateMark carries no sr-only word: on the /host header the visible word follows the mark, and a screen reader would otherwise say it twice (decided without asking, as the plan asked)"
  - "The sidebar's sr-only words are preceded by a space as a text node of its own: a space inside the sr-only span was trimmed out of the link's accessible name ('Host— warning'); in the link's flex row a whitespace-only text node takes no room"
  - "The 'Also not readable' line also carries break-words: with a 120-character mount path it ran to 1404 px at 390 px"
  - "The unanswered mark's title is 'holzkube-manager did not answer the latest request.' (there is no server summary to show)"

requirements-completed: [HOST-04, HMON-07]

coverage:
  - id: D1
    description: "/host header: mark, word (polite live region) and summary for ok, warn and unknown; the Warning notice only in warn, the server's sentences in the server's order, 'Also not readable', the threshold rule; position stale → warning → container → hardening"
    requirement: HMON-07
    verification:
      - kind: automated_ui
        ref: "web/src/routes/host.test.tsx#the state line and the warning notice"
        status: pass
  - id: D2
    description: "Agreement at the boundary: filesystem meter amber and the warning at 80 %, neither at 79.99 %; cpu_thermal ▲ exactly when the notice names it"
    requirement: HMON-07
    verification:
      - kind: automated_ui
        ref: "web/src/routes/host.test.tsx#agrees with its filesystem meter…, #shows the cpu_thermal ▲…"
        status: pass
  - id: D3
    description: "A 120-character mount path in a warning and in 'Also not readable' wraps at 390 px"
    requirement: HMON-07
    verification:
      - kind: automated_ui
        ref: "web/src/routes/host.browser.test.tsx#wraps a 120-character mount path…"
        status: pass
  - id: D4
    description: "Sidebar mark: none before the first answer; the three shapes with their words; ring with 'did not answer' after a failed poll; one call per 30 s away from /host, none of its own on /host"
    requirement: HOST-04
    verification:
      - kind: automated_ui
        ref: "web/src/components/Sidebar.test.tsx"
        status: pass
  - id: D5
    description: "Wall: host first in Nodes with name, 'manager · {reason}', data-state and colours; no tile when absent or null; headline counts wall.nodes; no curve on the host tile; one size class across host and nodes"
    requirement: HOST-04
    verification:
      - kind: automated_ui
        ref: "web/src/routes/wall.test.tsx#the host tile (D-14)"
        status: pass
  - id: D6
    description: "Fixtures: /host and the wall in warn, the same host, the cpu_thermal reading past its line"
    requirement: HOST-04
    verification:
      - kind: unit
        ref: "web/src/fixtures.test.ts#puts the host on the wall and on /host in warning…"
        status: pass

duration: ~15 min
completed: 2026-09-28
---

# Phase 12 Plan 06: The host's state where the operator looks Summary

**The host's state is now shown in three places: the /host header, the Host entry in the navigation, and the first tile of the wall's Nodes. All three draw the server's own decision. In warning, /host lists every crossed threshold with its value and line, using the server's sentences in the server's order. After a failed poll, the navigation shows "did not answer" rather than an earlier green.**

## Where this ran

The operator's Raspberry Pi 5 (aarch64), with Node through nvm.

- Vitest ran the jsdom project.
- The browser project (Playwright Chromium) ran `host.browser.test.tsx` and was also part of the full `npm run test`.
- Go 1.26.7 ran the `publicrepo` guard and the two route checks in `cmd/holzkube-managerd`.
- The production service, its data directory and `/usr/local/bin` were not touched.

## Accomplishments

- **`health` in `hostSchema`.** Neither `state` nor `summary` has a default. An answer without them fails to parse, so it can never turn into a browser-side "ok".
- **`HostStateMark`.** The three states are three shapes, so they survive greyscale:
  - ok: a filled `--viz-ok` dot
  - warn: `AlertTriangle` in `--viz-warn`
  - unknown / unanswered: a hollow grey ring

  All three sit in one fixed `size-4` box, so a change of state does not move the text beside it. The sidebar form adds sr-only words; the line form does not.
- **/host header.** Under the muted line, the mark, the word and the server's summary. Only the word is in `aria-live="polite"`, so a temperature change every 3 s is not read out again, but a change of state is. The line is not dimmed when the reading is stale.
- **Warning notice.** It uses the P11 amber class string verbatim.
  - Heading "Warning — 1 threshold crossed" or "Warning — {n} thresholds crossed", with a triangle.
  - One `li` per server sentence, in the server's order.
  - "Also not readable: …" when the server lists unreadable values.
  - The threshold rule last.
  - No button, dismiss or link. It sits after the stale notice and before the container and hardening notices.
- **Sidebar.** It uses the same `['host']` query /host polls, with `refetchInterval: onHostPage ? false : 30_000` and `retry: false`.
  - No mark before the first answer.
  - After a failed poll, the ring with " — not readable, holzkube-manager did not answer".
  - Otherwise, the server's state with its summary as the title.
- **Wall.** `wallSchema.host` is nullish and becomes null when missing. `LeftHalf` prepends a Host tile (`manager · {reason}`) to the tiles `Named` draws, so one count and one longest name decide the section's size.
  - The tile has `data-kind="host"`.
  - It has no curve, even when a node shares its name.
  - The headline still counts `wall.nodes`, and "Needs attention" does not get the host.
- **Fixtures.** On `/api/v1/host`, `cpu_thermal` now reads 82.1 against `warn_c` 80, with `health` in warn ("cpu_thermal 82.1 °C ≥ 80 °C"). The wall carries the same host (`manager-01.homelab.example`) in warn. Both use documentation values only.

## Task Commits

1. **Task 1: state line and Warning notice on /host**: `88851b4` (feat)
2. **Task 2: state mark in the navigation**: `0231c9f` (feat)
3. **Task 3: host tile on the wall**: `1886bc4` (feat)

## Fault injections (each seen red, then restored from a saved copy)

Exit codes were read from the vitest command itself, with output redirected to a file and never piped. After each restore, `cmp` against the saved copy confirmed the file was back, and the suite was green again.

| # | Fault | Test | Observed |
|---|-------|------|----------|
| 1 | warnings re-sorted in the browser (`[...health.warnings].sort()`) | host.test "lists several warnings in the server's order, never re-sorted" | exit 1: `expected [ '/srv 97% used ≥ 80%', …(2) ] to deeply equal [ …(3) ]` |
| 2 | `break-words` removed from the warning `ul` | host.browser.test "wraps a 120-character mount path in the warning list…" (Chromium) | exit 1: `text of <li class=""> "/srv/mmmm…" ends at 1393.2 > 390.5` |
| 3 | notice rendered for every state but ok (`health.state !== 'ok'`) | host.test "unknown: the hollow ring, Not readable…, and no amber notice" | exit 1: `expected <div …(1)><p …(1)>…(1)</p>` to be null |
| 4 (extra) | `state: z.enum(…).default('ok')` | host.test "refuses an answer without health rather than calling it healthy" | exit 1: `expected true to be false` |
| 5 (found, not injected) | "Also not readable" line without `break-words`, as first written | the same 390-px case | exit 1: `text of <p class="mt-1"> "Also not readable: Usage of /srv/mmm…" ends at 1404.7 > 390.5`. Fixed (deviation 1). |
| 6 | the last answer's state drawn when `host.error` is set (`host.error && host.data === undefined`) | Sidebar.test "after a failed poll draws the ring…, never the earlier green" | exit 1: `expected 'ok' to be 'unanswered'` |
| 7 | `refetchInterval: 30_000` on every page | Sidebar.test "asks nothing of its own on /host, where the page polls" | exit 1: `expected "host" to be called 1 times, but got 2 times` |
| 8 (extra) | a ring drawn before the first answer | Sidebar.test "has no mark before the first answer" | exit 1: `expected <span …(2)>…(1)</span> to be null` |
| 9 (found, not injected) | the space inside the sr-only span, as first written | Sidebar.test warn, ok/unknown and failed-poll cases | exit 1: `Expected … Host — warning / Received: Host— warning`. Fixed (deviation 2). |
| 10 | host counted into the headline (`wall.nodes.length + 1`) | wall.test "does not count the host among the cluster's nodes" | exit 1: `Unable to find an element with the text: /^as of \d+s ago · 1 workload, 1 node · …$/` |
| 11 | the host tile draws a same-named node's trend (the `tile.kind !== 'Host'` guard removed) | wall.test "draws no curve on the host tile, even when a node shares its name" | exit 1: `expected [ SVGSVGElement…, …(1) ] to have a length of 1 but got 2` |
| 12 (extra) | host placed after the nodes | wall.test "is the first tile of Nodes…" | exit 1: `expected <div data-row data-state="ok" …> to be <div data-row …(3)>` |
| 13 (extra) | wall fixture without `host` | fixtures.test "puts the host on the wall and on /host in warning…" | exit 1: `expected null not to be null` |

## Verification

All of this ran on the Pi.

- `vitest run --project jsdom` on `host.test.tsx`, `fixtures.test.ts` and `Sensors.test.tsx`: 113 tests pass, exit 0. `Sensors.test` builds from the /host fixture, which now carries `health`.
- `npm run test:browser -- src/routes/host.browser.test.tsx`: 3 of 3 pass, exit 0.
- `Sidebar.test.tsx`: 6 of 6 pass. `wall.test.tsx` and `fixtures.test.ts`: 61 pass.
- `npm run test` (jsdom and browser): 57 files and 584 tests pass, exit 0.
- `npm run typecheck`: exit 0. `npm run lint`: exit 0. The 2 warnings and 1 info are in `DataTable.tsx` and `wall.test.tsx` lines 81–83, which predate this plan.
- `GOTOOLCHAIN=go1.26.7 go test ./cmd/holzkube-managerd -count=1 -run 'TestEveryRouteTheClientIssuesIsCalledByAScreen|TestEveryRouteIsReachableFromTheInterface'`: ok.
- `GOTOOLCHAIN=go1.26.7 go test ./internal/publicrepo/`: ok.
- Acceptance greps:
  - `state: z.enum(['ok', 'warn', 'unknown'])`: 1
  - `aria-live="polite"` in host.tsx: ≥1
  - "thresholds crossed" in host.tsx: ≥1
  - "Also not readable": 1
  - `queryKey: ['host']` in Sidebar.tsx: 1
  - `onHostPage ? false : 30_000`: 1
  - `^  it(` in Sidebar.test.tsx: 6
  - `data-kind` in wall.tsx: 1
  - `manager · ` in wall.tsx: 1
  - `wall.nodes.length`: 1
- The two fixture python checks both exit 0.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] "Also not readable" overflowed the phone**
- **Found during:** Task 1, the new 390-px case. Its warn shape also carries a long unreadable sentence.
- **Issue:** The plan put `break-words` on the warning `ul` only. The "Also not readable: …" paragraph with a 120-character mount path ran to 1404.7 px.
- **Fix:** `mt-1 break-words` on that paragraph. The `ul` keeps its own `break-words`, so fault 2 still turns red on its own.
- **Files modified:** web/src/routes/host.tsx
- **Commit:** 88851b4

**2. [Rule 1 - Bug] The sidebar link read "Host— warning"**
- **Found during:** Task 2
- **Issue:** The accessible name is put together per element and trimmed, so the space leading " — warning" inside the sr-only span was lost.
- **Fix:** The words now begin with "—", and the space is a text node of its own before the sr-only span. A whitespace-only node in the link's flex row takes no room.
- **Files modified:** web/src/components/HostState.tsx
- **Commit:** 0231c9f

### Smaller choices, within the plan's latitude

- `hostShape` in host.test.tsx takes `health` through its existing `overrides` object, with an ok health written out by the test (`healthOf`) as the default, rather than a third positional argument.
- The fixture test also checks that the /host hostname equals the wall host's name, and that the first warning names `cpu_thermal`.

## Known Stubs

None.

## Not measured here

- The layout audit (`web/scripts/layout-audit.mjs`), which needs a build and a server, was not run. The /host fixture it measures now has the notice, and the wall fixture has the host tile. `host.browser.test.tsx` covers the 390-px notice backstop. The two wall backstops from the UI-SPEC (a 26-character host name at 390 px, and a long warning truncating on the tile) are measured by that audit and stay open for the phase's close.
- README and the `host.png` / `wall.png` renders belong to 12-07.

## Self-Check: PASSED

- web/src/components/HostState.tsx, web/src/components/Sidebar.test.tsx: FOUND
- 88851b4, 0231c9f, 1886bc4: FOUND in `git log`
