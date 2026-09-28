---
phase: 12-host-verlauf
plan: 05
subsystem: web
tags: [react, charts, live-series, host, hardening, vitest]
status: complete

requires:
  - phase: 12-host-verlauf
    provides: "12-01: useChartRange(['host'], api.hostHistory), useLiveSeries('host'), merge, RangePicker on /host; 12-03: host.HistoryValues with cpu, core:n, memory, rx/tx; 12-04: Sensors on the server's lines"
provides:
  - "web/src/components/charts/HardwareCharts.tsx: ProcessorChart, CoreList, MemoryChart, NetworkChart, shared by NodeHardware and /host"
  - "useLiveSeries append keeps a missing key's points cut to the window, drops the key once they aged out"
  - "/host: the three charts in their cards, CoreList instead of the dense per-core meters, HistorySlot ('Not readable — no history'), the live pick mirroring host.HistoryValues"
  - "/host copy: 'Readings' (host-readings), the 15 s / 24 h line, footer 'History is sampled every 15 s.', notice 'and are not recorded'"
  - "Processor card: load in the figure slot when usage is hidden, MissingValue for an unreadable load, '1 core'; filesystem caption 'of {used + available} usable'"
affects: [12-06, 12-07, 12-08]

actuals:
  tokens: 11200
  tasks: 3
  commits: 3
plan_head_before: 5304835718ba923913111670a7902041ce7ff905
plan_head_after: d71a0d3656d38bde40c570871573880dca722a67

tech-stack:
  added: []
  patterns:
    - "One set of chart blocks for a node and the host: a page places them, it does not own a copy"
    - "A chart slot decides between the chart and 'Not readable — no history' from the current reading and the points of its keys in the window; rate.no-baseline is pending, not unreadable"
    - "Reading guards skip chart axis labels (outsideAxes): an axis starting at 0% is a scale, not a reading"

key-files:
  created:
    - web/src/components/charts/HardwareCharts.tsx
    - web/src/hooks/useLiveSeries.test.ts
  modified:
    - web/src/components/NodeHardware.tsx
    - web/src/hooks/useLiveSeries.ts
    - web/src/routes/host.tsx
    - web/src/routes/host.test.tsx

key-decisions:
  - "The hardening notice says 'is not recorded' for a single hidden reading and 'are not recorded' for several, the same is/are switch the sentence already had"
  - "Usage hidden and load unreadable too: two MissingValue rows, usage first, no figure (no readable number exists)"
  - "The load-as-figure layout applies whenever usage is not readable, rate.no-baseline included: the first poll after a restart also shows 'load 7.29' big and 'Waiting for a second reading' below it"
  - "rx/tx in the page's live pick also need rates_over_seconds non-null and at least one physical link, as physicalRates in Go does"
  - "The misplaced MissingValue doc comment (above hostHistoryValues since 12-01) moved back above MissingValue"

requirements-completed: [HMON-05]

coverage:
  - id: D1
    description: "The node page draws from the shared module, unchanged: NodeHardware.test and Sensors.test pass without an edit"
    requirement: HMON-05
    verification:
      - kind: automated_ui
        ref: "web/src/components/NodeHardware.test.tsx, web/src/components/charts/Sensors.test.tsx"
        status: pass
  - id: D2
    description: "append keeps a missing key's points and drops it once they aged out"
    requirement: HMON-05
    verification:
      - kind: unit
        ref: "web/src/hooks/useLiveSeries.test.ts"
        status: pass
  - id: D3
    description: "/host draws processor, memory, network and sensor curves; under ProcSubset=pid with nothing recorded, processor and memory say 'Not readable — no history' with no axis; a gap is two runs; the first poll keeps its chart; two hardened polls add no point"
    requirement: HMON-05
    verification:
      - kind: automated_ui
        ref: "web/src/routes/host.test.tsx#the charts"
        status: pass
  - id: D4
    description: "Load in the figure slot when usage is hidden, MissingValue for load, '1 core', the filesystem caption's denominator"
    requirement: HMON-05
    verification:
      - kind: automated_ui
        ref: "web/src/routes/host.test.tsx#the Processor card figure, #the filesystem caption"
        status: pass

duration: ~14 min
completed: 2026-09-28
---

# Phase 12 Plan 05: The host's curves Summary

**/host now draws the host's processor, memory and network history over Live / 1 h / 6 h / 24 h with the node page's own chart blocks, moved into `charts/HardwareCharts.tsx` rather than copied. A value the hardening hides and never recorded says "Not readable — no history" instead of drawing an empty axis or a 0 % line. The page no longer claims that nothing is stored.**

## Where this ran

The operator's Raspberry Pi 5 (aarch64), Node through nvm. Vitest ran the jsdom project, and the browser project (Playwright Chromium) ran both for `host.browser.test.tsx` and for the full `npm run test`. The Go `publicrepo` guard ran with Go 1.26.7. The production service, its data directory and `/usr/local/bin` were not touched.

## Accomplishments

- **Shared chart blocks.** `HardwareCharts.tsx` exports `ProcessorChart`, `CoreList`, `MemoryChart` and `NetworkChart`, with their JSX moved verbatim out of `NodeHardware`. `NodeHardware` imports them back at the same places. Neither `NodeHardware.test.tsx` nor `Sensors.test.tsx` changed (`git show --stat` for `3327fd2` lists neither file).
- **`append` keeps a missing key.** When a reading leaves out a key, that key keeps its points, cut to the window, and gets no new point. Once all its points have aged out, the key is dropped.
- **Charts on /host:**
  - Processor card: `ProcessorChart`, then `CoreList`, which replaces Phase 11's dense per-core meters.
  - Memory card: `MemoryChart`, then the two meters.
  - Network card: `NetworkChart` (card body now `space-y-3`), then the interface list and the disclosure.
  - Filesystems has no chart.
- **The page's live pick mirrors `host.HistoryValues`.** It writes `cpu`, `core:n`, `memory`, `rx`/`tx` and the sensors, and only for values that were read.
- **`HistorySlot`.** It shows "Not readable — no history" only when the value is unreadable now, the reason code is not `rate.no-baseline`, and the key has zero points in the window. In every other case the chart is drawn.
- **Copy:**
  - Heading "Readings" (`id="host-readings"`, `aria-labelledby` matches), with the line about recording every 15 s and keeping 24 hours.
  - Footer ends with " History is sampled every 15 s."
  - The hardening notice adds "and are not recorded."
  - The file's opening comment and `LiveSection`'s comment were rewritten.
- **Processor card:**
  - When usage is hidden, the `text-xl` slot holds `load 0.52`, then `5 min 0.41 · 15 min 0.33`, then a muted `MissingValue` for usage.
  - An unreadable load is shown by a `MissingValue`.
  - The core count reads "1 core" for one core.
- **Filesystem caption.** It now reads "{used} of {used + available} usable · {available} free · {device}", the same denominator the df-style figure and the server's 80 % rule use.

## Task Commits

1. **Task 1: Shared chart blocks, and `append` keeps a missing key**: `3327fd2` (feat)
2. **Task 2: Host curves, "Not readable — no history", the copy**: `cc947b8` (feat)
3. **Task 3: Focal figure, MissingValue for load, "1 core", filesystem caption**: `d71a0d3` (fix)

## Fault injections (each seen red, then restored from a saved copy)

Exit codes were read from the vitest command itself. After each restore, `cmp` against the saved copy, or `git diff --quiet HEAD`, confirmed the file was back, and the suite was green again.

| # | Fault | Test | Observed |
|---|-------|------|----------|
| 1 | `append` builds the next series only from the reading's keys | useLiveSeries.test "keeps the points of a key the reading does not carry" | exit 1: `expected undefined to deeply equal [ 2 ]` |
| 1b (extra) | a missing key is never dropped | "drops a missing key once its last point is older than the window" | exit 1: `expected true to be false` |
| 2 | `HistorySlot` always renders the chart (empty points for an unreadable key) | "under ProcSubset=pid with nothing recorded…" (also "hardened now…" on Memory, and "two polls") | exit 1: `Unable to find an element with the text: Not readable — no history` |
| 3 | the pick writes `cpu: 0` when usage is hidden | "adds no processor or memory point…, poll after poll" (also the hardened and no-baseline zero guards) | exit 1: 4 cases red; the zero guard `expected [ '0%' ] to deeply equal []` (the 0 row in the chart's table) |
| 4 | `rate.no-baseline` treated as unreadable | "on the first poll after a restart draws the processor chart…" | exit 1: `Unable to find an accessible element with the role "img" and name /^Processor load/` |
| 4b (extra) | `segments()` never splits | "draws a stretch without readings as a gap" | exit 1: `expected [ 'M' ] to have a length of 2 but got 1` |
| 5 | the Phase 11 layout put back for a hidden usage (usage in the figure slot) | "with usage hidden puts the 1-minute load in the figure slot…" (and two existing cases) | exit 1: `Unable to find an element with the text: load 0.52` |
| 6 | the core line in the plural only | "says 1 core for one and 4 cores for four" | exit 1: `Unable to find an element with the text: 1 core` |
| 7 (extra) | the caption divides by `size_bytes` again | "names the denominator its df-style figure divides by", "draws one meter for root and data directory…" | exit 1: both `toHaveTextContent` failed |
| 8 (extra) | the ad-hoc "Load not readable: …" line put back | "draws an unreadable load as MissingValue…" | exit 1: `Unable to find an element with the text: Could not read /proc/loadavg: gone` |

## Verification

All of this ran on the Pi.

- The Task 1 set (`NodeHardware`, `Sensors`, `useLiveSeries`, `useChartRange`): 29 tests pass.
- `host.test.tsx` and `NodeHardware.test.tsx` (jsdom): exit 0. `host.test.tsx` alone has 62 tests.
- `test:browser src/routes/host.browser.test.tsx`: 2 of 2 pass.
- `npm run test` (jsdom and browser): 56 files and 561 tests pass, exit 0.
- `npm run typecheck`: exit 0. `npm run lint`: exit 0. Its 2 warnings and 1 info are in `DataTable.tsx` and `wall.test.tsx`, which this plan did not touch.
- `GOTOOLCHAIN=go1.26.7 go test ./internal/publicrepo/`: ok.
- Acceptance greps in `host.tsx`:
  - "Not readable — no history": 1
  - `host-readings`: 2
  - "Nothing on this page is stored": 0
  - `charts/HardwareCharts`: 1
  - "History is sampled every 15 s.": 1
  - "Load not readable": 0
  - "usable": 2
- `grep -c '1 core'` in `host.test.tsx`: 3.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] The plan's `npm --prefix web exec -- vitest run --project jsdom …` from the repository root finds no project**
- **Found during:** Task 1 verify.
- **Issue:** Run from the root, vitest does not load `web/vite.config.ts`, and it fails with `No projects matched the filter "jsdom"`.
- **Fix:** Every `<automated>` vitest command ran from `web/` (`npx vitest run --project jsdom …`). The npm scripts (`typecheck`, `lint`, `test`, `test:browser`) ran unchanged through `npm run` in `web/`.

**2. [Rule 1 - Test adaptation] Existing reading guards met the new chart axes**
- **Found during:** Task 2.
- **Issue:** Each chart's axis starts at "0%" or "0 B/s", and its "Show as table" is a second `details` in the Network card. Four existing assertions tripped on these, even though no reading had been drawn as 0.
- **Fix:**
  - `expectNoDrawnZero` and the network "never 0 B/s" check skip nodes inside an `svg` (`outsideAxes`).
  - The glued-text check `not.toHaveTextContent('0 B/s')` was dropped. The per-element check still applies to the rows and the legend.
  - "no disclosure" ignores `details` inside a `figure`.
  - The readable figure is looked up as `getByText('18%', { selector: 'p' })`, because the chart's table has a matching cell.
  - Fault 3 shows these guards still catch a real 0: the chart's table row `0%` turned them red.

**3. [Rule 1] The existing processor cases now expect the load as the figure**
- **Found during:** Task 3.
- **Issue:** The hardened and no-baseline cases asserted the Phase 11 line "load a · b · c". This plan replaces that line when usage is not readable.
- **Fix:** Those cases now assert `load 0.52`, then `5 min 0.41 · 15 min 0.33`, and the same for 7.29.

### Decided without asking (within the approved UI-SPEC resolutions)

- The figure copy is `load {load1}`, and the line under it is `5 min {load5} · 15 min {load15}`. The caption is "{used} of {used + available} usable · {available} free".
- The notice uses "is not recorded" for a single hidden reading and "are" for several.
- When usage is hidden and the load is unreadable too, the card shows two `MissingValue` rows and no figure.

## Known Stubs

None.

## Threat Flags

None. No new endpoint or trust boundary. T-12-13 is mitigated as planned: faults 2 and 3 are the red runs.

## Next

- 12-06/12-08: the state mark, the warning notice and the wall tile.
- 12-07: the README and the guide for the host history. They are not part of this plan.

## Self-Check: PASSED

- FOUND: `web/src/components/charts/HardwareCharts.tsx`, `web/src/hooks/useLiveSeries.test.ts`
- FOUND commits: `3327fd2`, `cc947b8`, `d71a0d3`
