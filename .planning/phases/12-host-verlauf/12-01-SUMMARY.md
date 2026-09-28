---
phase: 12-host-verlauf
plan: 01
subsystem: api, ui
tags: [go, history, sampler, sysfs, hwmon, react, tanstack-query, vitest]
status: complete

requires:
  - phase: 11-host-seite
    provides: "host.Collector (readSensors, reasonFor, sensorsPath, platformUnsupported), GET /api/v1/host, /host page (HostView, SensorsCard), hostapi_test hostSys fake"
provides:
  - "history.HostSubject() = \"host/local\" and an explicit retained case"
  - "SamplerDeps.Host (optional) and sampleHost, called before the inventory's early return"
  - "host.Collector.Sample and host.HistoryValues (temp:/fan: keys so far; plan 03 adds cpu, memory, core, rx/tx)"
  - "GET /api/v1/host/history (reader, not audited, no wall link, noUpstream), wired as Host: hostCollector.Sample in main.go"
  - "api.hostHistory(range); /host: useChartRange(['host'], api.hostHistory), useLiveSeries('host'), merge, RangePicker, Sensors sparkline column"
affects: [12-03, 12-05, 12-06, 12-07, 12-08]

actuals:
  tokens: 11200
  tasks: 2
  commits: 2
plan_head_before: 8246ab98a7f32b744cbc34c3ff18c4d41158398a
plan_head_after: 37ded415fd55adf8b518b707338a0492be6e077e

tech-stack:
  added: []
  patterns:
    - "The host is one more subject of the one sampler: same pass, same instant, same file, sampled first"
    - "HistoryValues lives in internal/host so internal/history never imports it; an unread value is an absent key, never 0"
    - "Sample reads only what the history keeps; it never calls Read (rate memo, data-directory walk)"
    - "Browser tests of HostView mock api.hostHistory in beforeEach and forget() the live series in afterEach"

key-files:
  created:
    - internal/host/sample.go
    - internal/host/sample_test.go
    - internal/history/host_test.go
    - .planning/phases/12-host-verlauf/deferred-items.md
  modified:
    - internal/history/history.go
    - internal/history/sampler.go
    - internal/httpapi/handlers/history.go
    - internal/httpapi/historyapi_test.go
    - cmd/holzkube-managerd/main.go
    - cmd/holzkube-managerd/budget_test.go
    - web/src/api.ts
    - web/src/routes/host.tsx
    - web/src/routes/host.test.tsx
    - web/src/routes/host.browser.test.tsx

key-decisions:
  - "TestTheHostIsNeverForgotten pins the literal key host/local (it is what the history file carries); HostSubject returning machine/local alone cannot turn Retain red, because retained compares against HostSubject itself"
  - "The RangePicker sits in LiveSection under the Live heading; the heading, its line ('Nothing on this page is stored') and the section comment stay as they are until plan 05 rewrites them"
  - "getHistory in historyapi_test.go takes *harness rather than *inventoryHarness, so the host route (no inventory) uses the same contract check"

patterns-established:
  - "A route of the host is tested with the hostSys fake and an fstest.MapFS sysfs, run through the real sampler (sampleHostOnce)"

requirements-completed: [HMON-05]

coverage:
  - id: D1
    description: "A host temperature read from sysfs is recorded by the one sampler under host/local and served by GET /api/v1/host/history"
    requirement: HMON-05
    verification:
      - kind: integration
        ref: "internal/httpapi/historyapi_test.go#TestHostHistoryAPI/a_sensor_read_on_the_host_is_served_as_its_series"
        status: pass
  - id: D2
    description: "Retain never deletes the host; Prune ages it after 24 h; the host is sampled when the inventory fails; a failing host logs once and leaves a gap"
    requirement: HMON-05
    verification:
      - kind: unit
        ref: "internal/history/host_test.go"
        status: pass
  - id: D3
    description: "HistoryValues writes no key for an unreadable reading; Sample on an unsupported platform is an empty map"
    requirement: HMON-05
    verification:
      - kind: unit
        ref: "internal/host/sample_test.go#TestHistoryValuesSensors"
        status: pass
  - id: D4
    description: "Route refusals: 401 without a session, reader 200 and unaudited, 400 Validation field range, 502 upstream.history-unavailable (host and machine)"
    requirement: HMON-05
    verification:
      - kind: integration
        ref: "internal/httpapi/historyapi_test.go#TestHostHistoryAPI, #TestAMachineHistoryWithoutAStore"
        status: pass
  - id: D5
    description: "/host draws each sensor's sparkline from the recorded history, shows the range picker, and the note when the history request fails"
    requirement: HMON-05
    verification:
      - kind: automated_ui
        ref: "web/src/routes/host.test.tsx#the Sensors card"
        status: pass

duration: ~21 min
completed: 2026-09-28
---

# Phase 12 Plan 01: Host history tracer Summary

**One host sensor now travels the whole path: sysfs → `Collector.Sample` → the one sampler under `host/local` → the existing store → `GET /api/v1/host/history` → its sparkline on `/host`.**

## Where this ran

The operator's Raspberry Pi 5 (aarch64), Go 1.26.7 via `GOTOOLCHAIN`, Node through nvm. There was no `-race` run: ThreadSanitizer does not work on the Pi 5 kernel, so the race detector only runs in CI. The production service, its data directory and `/usr/local/bin` were not touched.

## Performance

- **Duration:** about 21 min (19:40Z to 20:01Z)
- **Tasks:** 2 of 2
- **Files:** 3 created and 10 modified in code and tests, plus `deferred-items.md`

## Accomplishments

- `history.HostSubject()` returns `"host/local"`. `retained` now keeps it by name, where before it was kept only by the fallback for keys it does not know.
- `SamplerDeps.Host` and `sampleHost`. The host is sampled right after `at := s.deps.Now()` and before the inventory call. It uses the same `failed`/`recovered` logging as a node. There is no ticker, goroutine or file of its own.
- `host.Collector.Sample` reads only the sensors and never calls `Read`. `host.HistoryValues` writes `temp:<chip>/<label>` and `fan:<chip>/<label>` only when the sensors are readable.
- `GET /api/v1/host/history`: reader role, not audited, no wall link, no inventory check. It is in the `noUpstream` list and wired as `Host: hostCollector.Sample` in `main.go`.
- `api.hostHistory(range)`. `/host` fetches the history with `useChartRange(['host'], api.hostHistory)`, keeps its own points with `useLiveSeries` and joins the two with `merge`. It has a `RangePicker` with the node page's note, and the Sensors card has its sparkline column.

## Task Commits

1. **Task 1: Tracer** — `b78c808` (feat). Tracer gate: every `<verify>` command was re-run and passed, so the plan continued to Task 2.
2. **Task 2: Guards and error states** — `37ded41` (test)

## Fault injections (each seen red, then restored from a saved copy)

Exit codes were read from the test command itself. After every restore, `git diff --quiet HEAD -- <file>` confirmed the production file matched HEAD. Before the Task 2 commit, `git status --porcelain` listed only the four test files, plus STATE.md, which the orchestrator had already modified before this plan started.

| # | Fault | Test | Observed |
|---|-------|------|----------|
| 0 (tracer) | `hostHistory` queries `MachineSubject("local")` | TestHostHistoryAPI/a_sensor_read… | exit 1: `temp:cpu_thermal/temp1 is <nil> (<nil>), want an array` |
| 1a | `HostSubject()` returns `"machine/local"` | TestTheHostIsNeverForgotten | **First run without the key pin: exit 0 (green)**, because `retained` compares against `HostSubject()` itself. After adding the pin: exit 1, `HostSubject() = "machine/local"; the history file carries host/local` |
| 1b | same, plus `retained`'s host case removed (the host filed like a machine) | TestTheHostIsNeverForgotten | exit 1: the pin line plus `Retain with an empty inventory deleted the host's history` |
| 2 | `sampleHost` moved below the inventory's early return | TestTheHostIsSampledWhenTheInventoryFails | exit 1: `with the inventory failing the host has [], want its point at 1.7904168e+12` |
| 3 | `HistoryValues` writes `temp:cpu_thermal/temp1 = 0` when the sensors are hidden | TestHistoryValuesSensors/hidden | exit 1: `= map[temp:cpu_thermal/temp1:0]; want no key at all, never a 0` |
| 4 | `hostHistory` without `historyRange` (always `Range1h`) | TestHostHistoryAPI | exit 1: `range=7d: 200, want 400` and `range = 1h, want 6h` |
| 5a | RangePicker note always shown | host.test.tsx "draws a sensor's recorded history…" | exit 1: `expected <span …> to be null` |
| 5b | RangePicker note never shown (extra) | host.test.tsx "says the history is missing…" | exit 1 |
| 6 | `sampleHost` records a 0 on failure (extra) | TestAFailingHostIsLoggedOnce | exit 1: `the host has [[… 0] [… 0] [… 51.5] [… 51.5]], want two points` |
| — | before `api.hostHistory` existed in web/src | TestEveryRouteIsReachableFromTheInterface | exit 1: `no file under web/src mentions these routes: GET /api/v1/host/history` |

## Verification (all on the Pi)

- `go test ./internal/history/ ./internal/host/... ./internal/httpapi/... ./cmd/holzkube-managerd/ -count=1`: exit 0
- `npm --prefix web run test`: 55 files and 539 tests pass. `npm --prefix web run typecheck`: exit 0. `test:browser src/routes/host.browser.test.tsx`: 2 of 2 pass.
- `GOOS=darwin GOARCH=arm64 go build ./...`: exit 0. `./bin/task lint:go`: 0 issues. `go test ./internal/publicrepo/`: ok.
- `git diff --stat` of go.mod, go.sum, web/package.json and web/package-lock.json against the base: empty.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Guard that could not go red] Pinned the host key in TestTheHostIsNeverForgotten**
- **Found during:** Task 2, injection 1
- **Issue:** The plan's fault, `HostSubject()` returning `"machine/local"`, left the test green. `retained`'s explicit case is `key == HostSubject()`, so whatever the key is, Retain keeps it.
- **Fix:** The test now asserts `HostSubject() == "host/local"`, because that key is what the history file on disk carries. The fault was also injected in the combined form "filed like a machine" (1b), which fails both the pin and the Retain assertion.
- **Commit:** 37ded41

**2. [Rule 3 - Blocking] The plan's vitest command does not run from the repository root**
- **Issue:** `npm --prefix web exec -- vitest run --project jsdom …` fails with `No projects matched the filter "jsdom"`, because vitest runs with the repository root as its working directory and does not find `web/vite.config.ts`.
- **Fix:** Used `npm --prefix web run test -- --project jsdom src/routes/host.test.tsx`, which runs the same projects through the package script.

**3. [Rule 2 - Coverage the acceptance asked for] TestAMachineHistoryWithoutAStore**
- The acceptance criteria require `upstream.history-unavailable` to appear at least twice in historyapi_test.go ("machine and host cases"). No machine case existed, so one was added.

**4. `getHistory` now takes `*harness`**
- The host route has no inventory harness. The existing callers now pass `c.harness`.

### Deferred

- `./bin/task lint:web` fails on `web/src/components/DataTable.tsx` and `web/src/routes/wall.test.tsx`. This plan did not touch either file, and the problem was already there at base 8246ab9. The three web files this plan changed pass `biome check`. Details are in `deferred-items.md`.

## Known Stubs

None that block this plan's goal. The history keys cover only temperatures and fans on purpose: plan 03 adds cpu, memory, core and rx/tx. The "Live" heading and the line "Nothing on this page is stored." are now inaccurate, and plan 05 replaces them ("Readings" plus the new line).

## Threat Flags

None. The new route matches T-12-01 and T-12-02 (session, reader, range parsed only through `historyRange`, no wall link). T-12-03 is addressed: `Sample` reads only the hwmon/thermal files, through `readBounded`.

## Self-Check: PASSED

- FOUND: internal/host/sample.go, internal/host/sample_test.go, internal/history/host_test.go
- FOUND: b78c808, 37ded41
