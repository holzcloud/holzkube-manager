---
phase: 13-host-aktionen
plan: 02
subsystem: host
tags: [host, sensors, health, wall, react]
status: complete

requires:
  - phase: 12-host-verlauf
    provides: host.Assess, Health.Public, the wall's host tile, the shared Sensors component
provides:
  - untyped thermal zones named in Sensors.Unread on the fallback path
  - figure-first Health.Public sentences for the wall
  - the sensor ▲ coloured by severity through SEVERITY_COLOR
affects: [13-09]

actuals:
  tokens: 5700
  tasks: 2
  commits: 2
plan_head_before: 1edf0be811676cebf6a4413b77c9d85467541e6f
plan_head_after: 79ae5d3c7e36b0869304f9a1d4fdfd40cf293a6e

tech-stack:
  added: []
  patterns:
    - "Public (wall) sentences are built from the figure and a name, never from the page's sentence"

key-files:
  created: []
  modified:
    - internal/host/sensors.go
    - internal/host/sensors_test.go
    - internal/host/health.go
    - internal/host/health_test.go
    - internal/httpapi/handlers/wall_host_test.go
    - web/src/components/charts/Sensors.tsx
    - web/src/components/charts/Sensors.test.tsx
    - web/src/routes/wall.test.tsx

decisions:
  - "An untyped fallback zone is named by its directory (thermal_zone3): nothing else is known of it"
  - "A root filesystem's Public sentence is `91% used ≥ 80% · /`; the page's Warnings stay name-first"
  - "The ▲ colour is an inline style from SEVERITY_COLOR[severity], not a Tailwind class, because a class cannot be built from the map"

metrics:
  duration: 13min
  completed: 2026-09-29
---

# Phase 13 Plan 02: Carried host fixes Summary

On the thermal-zone fallback path, a zone that cannot be named now keeps the host Not readable instead of Healthy. The wall's host sentence starts with the reading (`82.1 °C ≥ 80 °C · cpu_thermal`). The sensor ▲ is amber for a warning and red only past danger. Each change was seen red against the old code.

**Where this ran:** the operator's Raspberry Pi 5 (aarch64), Go 1.26.7 via GOTOOLCHAIN. No `-race`, because ThreadSanitizer refuses this kernel's address space. Node came from nvm. Nothing touched the production service or its data.

## Tasks

| # | Task | Commit | Files |
|---|------|--------|-------|
| 1 | Untyped fallback zone in Unread; figure-first Public | bb6b74a | sensors.go, sensors_test.go, health.go, health_test.go, wall_host_test.go |
| 2 | ▲ coloured by severity; wall fixture figure-first | 79ae5d3 | Sensors.tsx, Sensors.test.tsx, wall.test.tsx |

## What changed

- **`readZones`** now returns a second value: the base names of listed zones whose `type` is missing, unreadable or empty. When no CPU chip reported, `readSensors` appends them to `out.Unread` next to the zones whose `temp` failed. When a CPU chip did report, nothing changes, because zones are only read for their limits there. This closes 12-SECURITY residual 1 (T-12-06, T-13-11).
- **`Health.Public`** is built as figure, then ` · `, then the name:
  - temperature: `<v> °C ≥ <line> °C[, critical] · <sensorName>`
  - filesystem: `<pct>% used ≥ 80% · <publicMount>`

  `Warnings` keep the name first. `Public` still never contains a mount path (T-12-08, T-13-12), and its doc comment explains why the order differs.
- **`Sensors.tsx`:** the ▲ span takes `style={{ color: SEVERITY_COLOR[severity] }}`, the same map the sparkline uses (T-13-13). The sr-only " high" and " critical" words are unchanged.
- **`wall.test.tsx`:** `HOST_WARN.reason` and the asserted tile text are now `82.1 °C ≥ 80 °C · cpu_thermal`.

## Fault injections (each reinstated separately, exit code read from the test command itself, restored from a saved copy and checked with `cmp`)

| # | Fault | Command exit | Red tests and failing lines |
|---|-------|--------------|-----------------------------|
| a | The untyped append in the fallback branch dropped (`_ = untyped`) | `go test` exit 1 | `TestSensorsThatFailAreNotNone/a_fallback_zone_without_a_type_file`: `unread = [], want ["thermal_zone0"]`, `health = {State:ok …}, want unknown`. `…/a_fallback_zone_with_an_empty_type_beside_one_that_reads`: `unread = [], want ["thermal_zone1"]`. `TestUnreadableIsNeverHealthy/a_fallback_zone_without_a_type`: `= {State:ok Summary:Filesystems are below 80%. This machine reports no temperature sensors…}, want unknown with ["Temperature thermal_zone0 could not be read."]` |
| a' | The untyped append moved outside `if !cpuTemps` (so zones are named even when a CPU chip reported) | `go test` exit 1 | `TestSensorsThatFailAreNotNone` (sensors_test.go:375): `a CPU chip that reads and an untyped zone: unread = ["thermal_zone0"], want none` |
| b | `f.public = f.sentence` for temperatures | `go test` exit 1 | `TestAssessWarns` public rows, for example `got ["cpu_thermal 82.1 °C ≥ 80 °C"] want ["82.1 °C ≥ 80 °C · cpu_thermal"]` and the critical row `want ["99.0 °C ≥ 85.5 °C, critical · nct6798 SYSTIN" …]`. `TestTheWallsHost/a_temperature_reaches_the_wall_figure_first`: `Reason:cpu_thermal 112.0 °C ≥ 110 °C, critical, want … 112.0 °C ≥ 110 °C, critical · cpu_thermal` |
| c | The fixed `text-[color:var(--viz-danger)]` class back on the ▲ | vitest exit 1 | `marks a warning with an amber ▲, never the danger red`: `expected '' to be 'var(--viz-warn)'`. `marks a reading past the danger line with a red ▲`: `expected '' to be 'var(--viz-danger)'` |
| c' | `SEVERITY_COLOR.danger` through the style for every severity | vitest exit 1 | `marks a warning with an amber ▲, never the danger red`: `expected 'var(--viz-danger)' to be 'var(--viz-warn)'` |

Before any code changed, the new tests had already failed for the right reasons: the rows for (a) and (b), plus `a filesystem's path never reaches the wall`, which read `data directory 91% used ≥ 80%` where `91% used ≥ 80% · data directory` was wanted. The wall.test fixture change passes either way, because the browser prints the server's reason verbatim. That is expected: the plan says the fixture only mirrors what the server now sends.

## Verification

- `go test ./internal/host ./internal/httpapi/handlers -run '…' -v` exits 0, with `--- PASS: TestSensorsThatFailAreNotNone` and `--- PASS: TestTheWallsHost`.
- `go test ./internal/host/... ./internal/httpapi/... -count=1` exits 0.
- `go vet` on the same packages exits 0. `./bin/task lint:go` (golangci-lint 2.13.1) reports 0 issues.
- `npx vitest run --project jsdom Sensors/wall/host/NodeHardware` exits 0, 124 tests.
- `npm --prefix web run test` exits 0: 59 files, 601 tests.
- `npm --prefix web run typecheck` exits 0.
- `npm --prefix web run lint` exits 0. Its 2 warnings are at wall.test.tsx:82-84, which this plan did not touch, so they predate it.
- `go test ./internal/publicrepo/` exits 0.
- `git diff --stat` over go.mod, go.sum, web/package.json and web/package-lock.json is empty.

## Deviations from Plan

None in substance. Beyond what the plan asked for:
- One extra row in `TestTheWallsHost` sends a critical temperature through `Assess`, so fault (b) is also seen at the wall and not only in `Health`.
- Two extra injections, (a') and (c'), cover the "CPU chip: Unread unchanged" row and a regression that only sends the danger colour through the style.
- In the `TestAssessWarns` row "temperatures and filesystems order by relative excess", the second filesystem is now built with the `data directory` role through a new `fsData` helper. Before, the fixture gave it the root role, which would have made its Public sentence `· /`. Its Warnings are unchanged.

## Left for plan 09

`docs/api-contract.md` (wall examples at about lines 2372 and 2461), `web/fixtures/demo.json` (wall `host.reason`) and the re-rendered `host.png`/`wall.png` still show the name-first wall reason. Plan 09 owns all of them. The page's Warnings wording is unchanged, so no page copy moves.

## Self-Check: PASSED

- All 8 modified files exist. Commits bb6b74a and 79ae5d3 are in `git log`.
- After the last restore, `git status --porcelain` was empty.
