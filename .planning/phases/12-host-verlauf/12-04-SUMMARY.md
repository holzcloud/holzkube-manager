---
phase: 12-host-verlauf
plan: 04
subsystem: web
tags: [react, zod, sensors, temperature, limits, reduced-motion]
status: complete

requires:
  - phase: 12-host-verlauf
    provides: "12-02: warn_c/danger_c on every HardwareTemperature, temperature_warn_c/temperature_danger_c on HardwareDisk"
provides:
  - "temperatureSchema with required warn_c/danger_c (no default)"
  - "hardwareSchema drive temperature_warn_c/temperature_danger_c"
  - "Sensors severity and sparkline ceiling from the server's lines; the browser's limit table and function deleted"
  - "Storage card drive figure on the server's pair"
  - "fan icon motion-safe:animate-spin"
  - "demo.json limits on every host and node temperature and node drive"
affects: [12-05, 12-06, 12-08]

actuals:
  tokens: 5800
  tasks: 2
  commits: 2
plan_head_before: c0bfb5fce01349510754d4cc3ae48d002537440b
plan_head_after: 704ec1f1caa8ec782fea3854f6aec26b3d6a7f19

tech-stack:
  added: []
  patterns:
    - "The browser draws temperature lines it is sent and never works them out; test shapes write the server's numbers out per sensor, not through a helper that recomputes them"

key-files:
  created: []
  modified:
    - web/src/api.ts
    - web/src/components/charts/Sensors.tsx
    - web/src/components/charts/Sensors.test.tsx
    - web/src/components/NodeHardware.tsx
    - web/src/components/NodeHardware.test.tsx
    - web/src/routes/host.test.tsx
    - web/src/routes/host.browser.test.tsx
    - web/fixtures/demo.json

key-decisions:
  - "TemperatureFigure's warn/danger became optional: a drive whose pair is null gets no severity rather than a number invented in the browser"
  - "The Pi test shapes (host.test pi5Sensors, host.browser.test's Pi) now carry critical_c 110 / 80 / 110, as the daemon reports since 12-02, not the old null crit"

requirements-completed: [HMON-07]

duration: ~9 min
completed: 2026-09-28
---

# Phase 12 Plan 04: The browser draws the server's temperature lines Summary

**Every temperature row on the node page and on /host, and every drive figure on the Storage card, now turns amber and red at the `warn_c`/`danger_c` the server sends. The browser's own limit table and function are deleted. A temperature arriving without its lines fails to parse instead of falling back to a default. The fan icon stops spinning for readers who asked for reduced motion.**

## Where this ran

The operator's Raspberry Pi 5 (aarch64), with Node 22 via nvm. Vitest ran both the jsdom project and the browser project (Playwright Chromium). The Go publicrepo guard ran with Go 1.26.7. The production service, its data directory and `/usr/local/bin` were not touched.

## Accomplishments

- **Schemas (`api.ts`):**
  - `temperatureSchema` requires `warn_c: z.number()` and `danger_c: z.number()`. Neither has a `.default()`, and the doc comment explains why.
  - Each drive in the node `hardwareSchema` carries `temperature_warn_c`/`temperature_danger_c`, which are nullable.
- **`Sensors.tsx`:**
  - Severity is now `severityOf(t.celsius, t.warn_c, t.danger_c)`, and the sparkline's `max={t.danger_c}`.
  - The per-kind default table and the limit function are deleted. `grep -rn` over `web/src` finds neither name.
  - The opening comment now says the lines come from internal/inventory.
- **`NodeHardware.tsx`:** the Storage card's figure reads `d.temperature_warn_c ?? undefined` / `d.temperature_danger_c ?? undefined`, and the import of the drive default is gone.
- **Fan icon:** `size-3.5 shrink-0 motion-safe:animate-spin [animation-duration:2s]`.
- **`demo.json` limits** (the numbers are what `inventory.TemperatureLimits` gives for each sensor):

  | Page | Sensor or drive | warn / danger (°C) |
  |------|-----------------|--------------------|
  | host | `cpu_thermal` (`critical_c` 110) | 80 / 110 |
  | host | `rp1_adc` | 75 / 90 |
  | m-cp-1 | coretemp | 80 / 100 |
  | m-cp-1 | nct6798 | 70 / 85 |
  | m-cp-1 | nvme Composite | 82.8 / 84.8 |
  | m-cp-1 | nvme0n1 drive | 82.8 / 84.8 |
  | m-cp-1 | sda drive | null / null |

- **Test shapes:** every temperature in `NodeHardware.test`, `Sensors.test` and `host.test` states its lines as literal numbers. The `temp()` helper in `host.browser.test` takes `warn`/`danger` as explicit arguments.
- **`NodeHardware.test`:** the two limit cases were removed. They exist, with the same names, as rows of Go's `TestTemperatureLimits`. The `cpuTemperature` case moved to `describe('the processor temperature')`.
- **New tests:**
  - `NodeHardware.test` › "the drive figure turns at the server's lines" has three cases, all parsed through `hardwareSchema`.
  - `Sensors.test` › "the limits are the server's" has five cases: 60 at 55, 85 at 90, danger at and past, the sparkline ceiling, and schema refusal on both host and hardware with a control.
  - `Sensors.test` › "the fan icon".

## Task Commits

1. **Task 1: the drive figure takes the server's pair; the fan respects reduced motion**: `fcb690c` (feat)
2. **Task 2: Sensors draw the server's limits; the browser's rule is deleted**: `704ec1f` (feat)

## Fault injections (each seen red, then restored)

Before each fault I copied the file to the scratchpad and copied it back afterwards; `cmp` confirmed the restore. I read exit codes from the vitest command itself, with its output going to a file.

| # | Fault | Red test | Observed (exit 1) |
|---|-------|----------|-------------------|
| 1a | Drive figure fed a fixed `warn={60} danger={70}` | NodeHardware.test › "stays calm below a warning line of 70, whatever a drive default would say" | `expected 'warn' to be 'ok'` |
| 1b | Bare `animate-spin` class on the turning fan | Sensors.test › "spins only for a reader who has not asked for reduced motion" | `expected [ 'lucide', 'lucide-fan', …(4) ] to include 'motion-safe:animate-spin'` |
| 2a | The browser's old rule put back into `Sensors` (kind defaults, chip high/crit, `warn_c` ignored) | Sensors.test › "warns at 60 °C when the server says 55…" and "stays calm at 85 °C when the server says 90…" | `expected 'ok' to be 'warn'`; `expected 'warn' to be 'ok'` (2 failed, 68 passed) |
| 2b (extra) | Sparkline `max={95}` instead of `t.danger_c` | Sensors.test › "draws the sparkline up to the danger line" | `expected 'M0.0,8.6 L64.0,14.3' to be 'M0.0,2.0 L64.0,11.0'` |
| 2c (extra) | `.default(80)` / `.default(95)` on `warn_c`/`danger_c` | Sensors.test › "refuses a temperature without its lines, on both pages" | `expected true to be false` |

Injections 1a and 1b were applied together. Each one turned red only the test written for it, and no other test.

## Verification

- `vitest run --project jsdom` on Sensors, NodeHardware, host and fixtures tests: exit 0, 101 tests.
- `npm run test:browser -- src/routes/host.browser.test.tsx`: exit 0, 2 tests.
- `npm --prefix web run test` (full suite): exit 0, 55 files, 546 tests.
- `npm run typecheck`: exit 0.
- `npm run lint`: exit 0. It reports 2 warnings and 1 info, all in files this plan did not touch: `DataTable.tsx`, `wall.test.tsx`, `vite.config.ts`.
- The plan's acceptance checks all pass:
  - The demo.json assertion exits 0.
  - `grep -c` returns 1 for each of `warn_c: z.number()`, `danger_c: z.number()`, `t.warn_c` and `motion-safe:animate-spin`.
- `go test ./internal/publicrepo/`: ok.
- `web/package.json` and `web/package-lock.json` are unchanged, and no package was added.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] TemperatureFigure's props had to accept a missing line**
- **Found during:** Task 1
- **Issue:** `TemperatureFigure` typed `warn`/`danger` as `number`, so `d.temperature_warn_c ?? undefined` would not typecheck. The plan says "nothing else in the file changes".
- **Fix:** Made both props optional, with a one-line doc comment. `severityOf` already treats `undefined` as no line.
- **Commit:** fcb690c

**2. Extra tests and injections**
- The sparkline-ceiling and schema-refusal tests, and their injections 2b and 2c, go beyond the plan. The schema-refusal test uses demo.json's own host and hardware answers, with a control that parses them intact first.

## Known Stubs

None.

## Threat Flags

None. T-12-12 is mitigated: the schema has no default (2c was red with one), and the rule is deleted (2a was red with it reinstated). T-12-SC holds: no package was added.

## Self-Check: PASSED

- FOUND: web/src/components/charts/Sensors.tsx, web/src/api.ts, web/fixtures/demo.json, web/src/components/NodeHardware.tsx
- FOUND: fcb690c, 704ec1f
