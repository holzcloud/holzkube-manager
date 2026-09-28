---
phase: 12-host-verlauf
plan: 07
subsystem: web
tags: [css, typography, tailwind, fixtures, layout-audit, readme, screenshots, playwright]
status: complete

requires:
  - phase: 12-host-verlauf
    provides: "12-05: the /host curves and hostHistory; 12-06: health on /host, the host tile on the wall, the warn fixtures"
provides:
  - "--text-xs--letter-spacing: normal (web/src/index.css): 12-px lines at the font's own tracking, app-wide"
  - "web/src/typography.browser.test.tsx: the measured word space of a 12-px line, and headings/14-px tracking unchanged"
  - "/api/v1/host/history in web/fixtures/demo.json, held by fixtures.test.ts (keys, gap, end)"
  - "readme-images.mjs: host branch of history(), /host times moved to now as a whole, `through` option for a tall shot"
  - "layout-audit.mjs and readme-images.mjs wait for the daemon to exit before removing its data directory"
  - "README: Host bullet and 'A look around' text; docs/screenshots/host.png and wall.png re-rendered"
affects: [12-08]

actuals:
  tokens: 9700
  tasks: 3
  commits: 3
plan_head_before: afb8afcdfde0edff30ab58be853bc32bdcb82fa7
plan_head_after: 7897eb64a3fb1b8e10eb55fea55e1883b3d3ede8

tech-stack:
  added: []
  patterns:
    - "A tracking fix through the Tailwind theme (--text-xs--letter-spacing), so the utility sets it and a tracking-* class still wins"
    - "Fixture history ends at the fixture's observed_at, because /host ends its chart window there, not at Date.now()"

key-files:
  created:
    - web/src/typography.browser.test.tsx
  modified:
    - web/src/index.css
    - web/fixtures/demo.json
    - web/src/fixtures.test.ts
    - web/scripts/readme-images.mjs
    - web/scripts/layout-audit.mjs
    - README.md
    - docs/screenshots/host.png
    - docs/screenshots/wall.png

key-decisions:
  - "Fix chosen from the measurement: letter-spacing, not word-spacing. Layout widths are the same at DPR 1 and DPR 2, so hinting plays no part; with letter-spacing normal the product's space equals the reference exactly."
  - "The rule is the theme variable --text-xs--letter-spacing: normal. It is one line, it reaches every text-xs element and its children, and a tracking-* class on the same element still wins."
  - "host.png is shot as tall as the Readings section (1440×1529). A 900-px window ends above the curves, and the plan asks for the gap to be visible."
  - "Text below 12 px in arbitrary sizes (the wall's text-[0.7rem], several text-[10px]/[11px]) is not reached by the text-xs fix. It is recorded in deferred-items.md, not fixed here."

requirements-completed: [HMON-05, HMON-07, HOST-04]

coverage:
  - id: D1
    description: "A 12-px text-xs line's word space equals the font's (within 0.1 px); an h1 with tracking-tight and 14-px text keep their tracking"
    requirement: HMON-07
    verification:
      - kind: automated_ui
        ref: "web/src/typography.browser.test.tsx"
        status: pass
  - id: D2
    description: "/api/v1/host/history fixture parses with historySchema, carries exactly rx, tx and the two Pi temperatures, has a ≥ 5-min gap in every series, and no point after observed_at"
    requirement: HMON-05
    verification:
      - kind: unit
        ref: "web/src/fixtures.test.ts#draws the host's history from what the fixture's hardening leaves readable, with a gap"
        status: pass
  - id: D3
    description: "Layout audit: /host (warn shape, history fixture) and /wall (host tile) ok at 390 and 1280 px, tap targets included"
    requirement: HOST-04
    verification:
      - kind: automated_ui
        ref: "./bin/task test:layout"
        status: pass
  - id: D4
    description: "Wall backstops: a 26-character host name fits at 390 px, and a long warning truncates on the tile"
    requirement: HOST-04
    verification:
      - kind: manual_procedural
        ref: "one-off Playwright measurement against the built daemon (see 'Wall backstops')"
        status: pass
  - id: D5
    description: "README names history, warning and state; host.png and wall.png show them from fixtures"
    requirement: HOST-04
    verification:
      - kind: manual_procedural
        ref: "pictures inspected; go test ./internal/publicrepo"
        status: pass

duration: ~30 min
completed: 2026-09-28
---

# Phase 12 Plan 07: Words in 12-px lines, the host's history as a fixture, and the README Summary

**12-px lines had their word space shrunk to 1.75 px, against the font's 2.00. The cause was the body's tracking, which is inherited as -0.24 px. `text-xs` now uses the font's own tracking, and a browser test holds that measurement. The layout audit and the README renderer now draw the host's history from a fixture, never from the machine they run on. The README names the host's history, warning and state, and two pictures are rendered from fixtures to show them.**

## Where this ran

The operator's Raspberry Pi 5 (aarch64), with Node through nvm.

- Playwright Chromium ran the browser project, the layout audit and the README renderer.
- Go 1.26.7 (`GOTOOLCHAIN`) built the binary and ran `internal/publicrepo`.
- The production service, `/var/lib/holzkube-manager` and `/usr/local/bin` were not touched.

## The measurement (Task 1)

I rendered `text-xs text-muted-foreground` spans in the product's body, in Manrope Variable at 12 px, after `document.fonts.load`. Each word space is the width of "a b" minus the width of "ab".

| | "a b" | "ab" | space |
|---|---|---|---|
| product, before the fix | 15.28125 | 13.53125 | **1.750** |
| reference (`letter-spacing: normal; word-spacing: normal`) | 16 | 14 | **2.000** |
| product, after the fix | 16 | 14 | 2.000 |

- **DPR makes no difference.** DPR 1 is the project default. For DPR 2 I made a one-off copy of the config with `playwright({ contextOptions: { deviceScaleFactor: 2 } })`. The four widths were identical, so layout widths do not depend on DPR and hinting is not the cause (RESEARCH A4 does not hold).
- **Cause.** The body sets `letter-spacing: -0.015em`. An em tracking is inherited as the value computed at the body's 16 px, which is -0.24 px. So a 12-px glyph loses 0.24 px, not 0.18. The computed style confirms this: a 14-px `text-sm` paragraph also reads `-0.24px`.
- **Fix.** I added one theme variable in `@theme inline`, `--text-xs--letter-spacing: normal`, with a comment giving the numbers.
  - The `text-xs` utility now sets letter-spacing through `var(--tw-tracking, …)`, so a `tracking-*` class on the same element still wins.
  - `tracking-tight` headings (`-0.5px`) and 14-px text (`-0.24px`) are unchanged.

## Fault injections (each seen red, then restored)

Exit codes came from the vitest command itself, with output redirected to a file and not piped. After each restore, `cmp` against the saved copy confirmed the file was back.

| # | Fault | Test | Observed |
|---|-------|------|----------|
| 1 | the `--text-xs--letter-spacing: normal` line removed | typography "separate their words by the space the font means" | exit 1: `expected 0.25 to be less than or equal to 0.1` |
| 2 | fix removed and the body's tracking reset to `normal` instead (a fix that reaches every size) | typography "leave the tracking of headings and of larger text alone" | exit 1: `expected 'normal' to be '-0.24px'` |
| 3 | a `cpu` series added to the history fixture | fixtures "draws the host's history…" | exit 1: `expected [ 'cpu', 'rx', …(3) ] to deeply equal [ Array(4) ]` |
| 4 | the gap filled (240 even 15-s points) | same | exit 1: `rx: expected 15000 to be greater than or equal to 300000` |
| 5 | a tx point 15 s after the last one, past `observed_at` | same | exit 1: `tx: expected 1790589615000 to be less than or equal to 1790589603000` |
| 6 | the `/api/v1/host/history` entry removed | the schema case and the history case | exit 1: both fail (2 failed, 32 passed) |
| 7 (found, not injected) | the audit removing its data directory while the daemon still runs | `./bin/task test:layout` | exit 201: every route `ok`, then `ENOTEMPTY: directory not empty, rmdir '/tmp/holzkube-layout-…'`. It did not reproduce in two isolated start/kill/rm runs, so it is a race. Fixed (deviation 1). |

## Wall backstops (left open by 12-06)

I ran a one-off Playwright script from scratch space against the built daemon, with the demo fixtures and the audit's `generated_at` refresh, at 390 and 1280 px.

- **26-character name.** `manager-01.homelab.example` is 26 characters. At 390 px the tile is 358 px wide and ends at 374. The name's `break-words` line has `scrollWidth` 334 = `clientWidth` 334, and the document is 390 px wide.
- **Long warning.** With a 117-character reason, the tile's `truncate` line has `scrollWidth` 1036 against `clientWidth` 334 at 390 px, and ends at 362. At 1280 px it is 1215 against 302. The page does not widen.
- **The full sentence on /host.** 12-06's `host.browser.test.tsx` case holds that a 120-character mount path wraps at 390 px.
- **Also seen:** at 1280 px even the fixture's short reason truncates on the tile ("manager · cpu_thermal 82.1 °C ≥ 8…", 321 against 302). In the 1600-px README picture it is also cut after "cpu_thermal 8…". This is the tile's design (truncate; the full sentence is on /host), and it is noted here.

## Task Commits

1. **Task 1: 12-px word space, measured, fixed and held**: `bff9dcb` (fix)
2. **Task 2: host history fixture, renderer host branch, audit cleanup**: `61baaaf` (feat)
3. **Task 3: README text, host.png and wall.png**: `7897eb6` (docs)

## The pictures

Both were rendered by `bin/holzkube-managerd`, built with `-X main.version=v0.1.0`, because `git describe` gives `v0.0.1-…`. The sidebar reads `v0.1.0`, which matches the fixture's service version. Only fixture values appear: `manager-01.homelab.example`, the `srv-node-0x.homelab.example` nodes, and the fixture's model, OS and kernel strings. I restored the other re-rendered images (banner, social, app-detail, apps, clusters, kubernetes, node-hardware, phone, the three web/public icons) with `git checkout --`.

- **host.png** (1440×1529):
  - The header state line reads "Warning · 1 threshold crossed.", with the triangle on the Host navigation entry.
  - The amber notice has "cpu_thermal 82.1 °C ≥ 80 °C" and the threshold rule.
  - The hardening notice, then the Device and Service cards; the data directory was "measured 23 s ago".
  - Readings are at **24 h**:
    - Processor and Memory show "Not readable — no history".
    - The Network card's inbound/outbound curve has a visible gap where the service was stopped.
    - Sensors has cpu_thermal at 82 °C ▲ and rp1_adc at 55 °C with sparklines.
- **wall.png** (1600×900): the amber `manager-01.homelab.example` tile is first in Nodes, reading "manager · cpu_thermal 8…". It has no curve.

## Verification

All of this ran on the Pi.

- `npm run test:browser -- src/typography.browser.test.tsx`: 3 of 3 pass, exit 0.
- `vitest run --project jsdom src/fixtures.test.ts`: 34 pass, exit 0.
- `npm run test` (jsdom and browser): 58 files and 589 tests pass, exit 0. This was run after Task 1 (587 tests) and again after Task 3.
- `./bin/task test:layout`: exit 0, "Nothing out of reach at 390px and 1280px, and every control is at least 44px at 390px". `/host` measured 9 controls and 27 items at 390 px; `/wall` measured 0 controls and 17 items. Both were ok at 1280 px. This run included Tasks 1 and 2.
- `npm run typecheck`: exit 0. `npm run lint`: exit 0. Its 2 warnings and 1 info are the ones already listed in deferred-items (DataTable.tsx, wall.test.tsx).
- `GOTOOLCHAIN=go1.26.7 go test ./internal/publicrepo -count=1`: ok.
- Acceptance checks:
  - `import '@/index.css'` in the typography test: 1
  - The series-keys python check: exit 0
  - `/api/v1/host/history` in readme-images.mjs: 1
  - `'/host', 'host.png', { range: '24 h'`: 1
  - In the README Host bullet: "wall" 1, "history" 1, "warn" 2
  - `git show --stat` of `7897eb6`: host.png and wall.png, and no other picture
- `bin/holzkube-managerd` was rebuilt with the normal `git describe` version after the render.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] The layout audit went red after every route had passed**
- **Found during:** Task 1's audit run.
- **Issue:** Since 12-01 the daemon writes its metrics history into its data directory on SIGTERM. `layout-audit.mjs` sent SIGTERM and ran `rm -r` straight away, which failed with ENOTEMPTY (exit 201). `readme-images.mjs` had the same order.
- **Fix:** Both scripts now wait for the daemon's `exit`, bounded at 10 s, before removing the directory. The next audit run exited 0.
- **Files modified:** web/scripts/layout-audit.mjs (not in the plan's file list), web/scripts/readme-images.mjs
- **Commit:** 61baaaf

**2. [Rule 1 - Bug] The /host picture ended above the curves, and said "measured 12 h ago"**
- **Found during:** Task 3's first render.
- **Issue:** A 900-px window stops at the range buttons, so the gap and "Not readable — no history" were not in the picture. Also, refreshing only `observed_at` pushed the data directory's `measured_at` 12 hours into the past, and the service seemed to start before the machine booted.
- **Fix:** `shoot` has a `through` option: the window is made as tall as the Readings section plus 16 px. `live()` moves `observed_at`, `service.started_at` and the data directory's `measured_at` together, keeping their distances.
- **Files modified:** web/scripts/readme-images.mjs
- **Commit:** 7897eb6

### Smaller choices, within the plan's latitude

- `wall.png`'s README sentence now says the host is the first tile among the nodes, in addition to the Host paragraph.
- The typography test also checks that the span really is 12 px in Manrope Variable and that the font has loaded, so a pass cannot come from measuring the fallback font.

## Deferred

- Text below 12 px in arbitrary sizes keeps the -0.24 px tracking. Examples are the wall's `text-[0.7rem]` aside and legend, and the `text-[10px]`/`text-[11px]` in Sidebar, Alpha, LiveChart, upgrades, provision and images. In a 1280×844 wall render, "each tile takes the colour…" read with collapsed spaces. The details are in `deferred-items.md`.

## Known Stubs

None.

## Self-Check: PASSED

- web/src/typography.browser.test.tsx, docs/screenshots/host.png, docs/screenshots/wall.png: FOUND
- bff9dcb, 61baaaf, 7897eb6: FOUND in `git log`
