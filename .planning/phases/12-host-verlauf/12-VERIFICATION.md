---
phase: 12-host-verlauf
verified: 2026-09-29T01:45:00Z
status: passed
score: 57/57 must-haves verified (4 roadmap success criteria + 53 plan truths)
covered_files:
  - .planning/phases/12-host-verlauf/12-01-PLAN.md
  - .planning/phases/12-host-verlauf/12-01-SUMMARY.md
  - .planning/phases/12-host-verlauf/12-02-PLAN.md
  - .planning/phases/12-host-verlauf/12-02-SUMMARY.md
  - .planning/phases/12-host-verlauf/12-03-PLAN.md
  - .planning/phases/12-host-verlauf/12-03-SUMMARY.md
  - .planning/phases/12-host-verlauf/12-04-PLAN.md
  - .planning/phases/12-host-verlauf/12-04-SUMMARY.md
  - .planning/phases/12-host-verlauf/12-05-PLAN.md
  - .planning/phases/12-host-verlauf/12-05-SUMMARY.md
  - .planning/phases/12-host-verlauf/12-06-PLAN.md
  - .planning/phases/12-host-verlauf/12-06-SUMMARY.md
  - .planning/phases/12-host-verlauf/12-07-PLAN.md
  - .planning/phases/12-host-verlauf/12-07-SUMMARY.md
  - .planning/phases/12-host-verlauf/12-08-PLAN.md
  - .planning/phases/12-host-verlauf/12-08-SUMMARY.md
  - README.md
  - cmd/holzkube-managerd/budget_test.go
  - cmd/holzkube-managerd/main.go
  - docs/api-contract.md
  - docs/guide.md
  - docs/screenshots/host.png
  - docs/screenshots/wall.png
  - internal/history/history.go
  - internal/history/host_test.go
  - internal/history/sampler.go
  - internal/host/collector.go
  - internal/host/health.go
  - internal/host/health_test.go
  - internal/host/host.go
  - internal/host/sample.go
  - internal/host/sample_test.go
  - internal/host/sensors.go
  - internal/host/sensors_test.go
  - internal/host/testdata/pi5/sys/devices/virtual/thermal/thermal_zone0/trip_point_1_temp
  - internal/host/testdata/pi5/sys/devices/virtual/thermal/thermal_zone0/trip_point_1_type
  - internal/host/testdata/pi5/sys/devices/virtual/thermal/thermal_zone0/trip_point_2_temp
  - internal/host/testdata/pi5/sys/devices/virtual/thermal/thermal_zone0/trip_point_2_type
  - internal/host/testdata/pi5/sys/devices/virtual/thermal/thermal_zone0/trip_point_3_temp
  - internal/host/testdata/pi5/sys/devices/virtual/thermal/thermal_zone0/trip_point_3_type
  - internal/host/testdata/pi5/sys/devices/virtual/thermal/thermal_zone0/trip_point_4_temp
  - internal/host/testdata/pi5/sys/devices/virtual/thermal/thermal_zone0/trip_point_4_type
  - internal/httpapi/handlers/history.go
  - internal/httpapi/handlers/kubernetes.go
  - internal/httpapi/handlers/wall_host.go
  - internal/httpapi/handlers/wall_host_test.go
  - internal/httpapi/hardwareapi_test.go
  - internal/httpapi/historyapi_test.go
  - internal/httpapi/hostapi_test.go
  - internal/httpapi/kubernetesapi_test.go
  - internal/httpapi/wallhostapi_test.go
  - internal/httpapi/walllinkapi_test.go
  - internal/inventory/hardware.go
  - internal/inventory/hardware_test.go
  - internal/inventory/limits.go
  - internal/inventory/limits_test.go
  - web/fixtures/demo.json
  - web/scripts/layout-audit.mjs
  - web/scripts/readme-images.mjs
  - web/src/api.ts
  - web/src/components/HostState.tsx
  - web/src/components/NodeHardware.test.tsx
  - web/src/components/NodeHardware.tsx
  - web/src/components/Sidebar.test.tsx
  - web/src/components/Sidebar.tsx
  - web/src/components/charts/HardwareCharts.tsx
  - web/src/components/charts/LiveChart.tsx
  - web/src/components/charts/Sensors.test.tsx
  - web/src/components/charts/Sensors.tsx
  - web/src/components/charts/Sparkline.tsx
  - web/src/fixtures.test.ts
  - web/src/hooks/useChartRange.test.ts
  - web/src/hooks/useChartRange.ts
  - web/src/hooks/useLiveSeries.test.ts
  - web/src/hooks/useLiveSeries.ts
  - web/src/index.css
  - web/src/routes/host.browser.test.tsx
  - web/src/routes/host.test.tsx
  - web/src/routes/host.tsx
  - web/src/routes/wall.test.tsx
  - web/src/routes/wall.tsx
  - web/src/typography.browser.test.tsx
covered_digest: "v2:sha256:378eb85d9e39181077bb5abe9e07f65c40e4036c5c9517f016cbf78c9a702b94"
behavior_unverified: 0
overrides_applied: 0
human_verification:
  - test: "After the operator's next release has installed itself through the update timer, open /host on the production service, signed in; switch 1 h / 6 h / 24 h; after the next hourly update restart look again; open the wall"
    expected: "State line (and, if a threshold is crossed, the amber notice naming value and threshold); Processor and Memory say 'Not readable — no history' under ProcSubset=pid while Network and Sensors draw curves; the curves from before the update restart are still there with the downtime as a gap; the Host entry in the navigation carries the state mark; the wall shows the 'manager' tile first in Nodes"
    why_human: "The production binary does not contain this phase. Replacing it, restarting the service, or cutting a release is the operator's call. Everything checkable without that was checked on the Pi against a dev daemon built from HEAD (history through a restart, gap for the downtime, subset=pid). The wall tile was checked against a simulated cluster, the layout audit and the rendered fixture. A dev daemon has no adopted cluster, so its wall route cannot answer."
---

# Phase 12: Host wie ein Knoten — Verlauf, Warnung, Wand. Verification Report

**Phase goal:** Der Host steht neben den Knoten wie einer von ihnen: mit Verlaufsdiagrammen über 1 h / 6 h / 24 h aus `internal/history`, mit einer Warnung, die ihren Grund nennt, und mit einem Eintrag in der Navigation und einer Kachel auf der Wand.
**Verified:** 2026-09-29T01:45:00Z
**Status:** human_needed. Only the production view after a release is left. Every automated check and every check on a dev daemon passed.
**Re-verification:** No. This is the initial verification. The code-review fix round (12-REVIEW-FIX.md) was already on HEAD.
**Where it ran:** on the operator's Pi 5 (aarch64, native arm64), with Go 1.26.7 and without `-race` (the race detector is CI's alone). The dev daemon was built from HEAD and ran on 127.0.0.1:18453 with a temp data dir. It was stopped and removed afterwards. `holzkube-manager.service`, `/var/lib/holzkube-manager` and `/usr/local/bin` were not touched.

## Goal Achievement

### Roadmap Success Criteria

| # | Success criterion | Status | Evidence |
|---|---|---|---|
| 1 | `/host` shows history charts for CPU, memory, temperature and network over 1 h / 6 h / 24 h: the same ranges and charts as a node, written into `internal/history`, with no new pipeline | ✓ VERIFIED | The one sampler records under `HostSubject()` = `host/local` (`internal/history/sampler.go` `sampleHost`). It runs in the same pass, at the same `at`, with the same `FlushIfDue`. `cmd/holzkube-managerd/main.go` wires `Host: hostCollector.Sample`. `GET /api/v1/host/history` is in `internal/httpapi/handlers/history.go`. `web/src/routes/host.tsx` calls `useChartRange(['host'], api.hostHistory)` and imports `ProcessorChart/MemoryChart/NetworkChart` from `web/src/components/charts/HardwareCharts.tsx`, the same module and `RangePicker` that `NodeHardware.tsx` uses (moved, not copied). The real dev daemon's 1 h history carried `cpu`, `core:0..3`, `memory`, `rx`, `tx` and `temp:cpu_thermal/temp1`, `temp:rp1_adc/temp1` at a 15-s step. The route answered 401 with no session, 400 for `range=2h` and 200 for `range=1h`. |
| 2 | After a daemon restart the history up to the restart is still there; a stretch in which a value was unreadable shows as a gap, never as a zero line | ✓ VERIFIED | On the real binary (Pi): a point sampled 12 s before a SIGTERM was present after the 42-s pause and the restart. There were no points in the downtime (spacing 15 s before and after, 45 s across it). The first pass after the restart had no `cpu` (no rate baseline yet), and no cpu or memory value was ever 0. `TestTheHostHistorySurvivesARestartWithItsGap` passes and went red here when `encodeLocked` dropped `host/local` (M8). On the browser side, `segments`/`merge` break a curve at a missing recorded slot, and 4 tests went red with the gap mark ignored (W7). |
| 3 | When a temperature or a disk's usage crosses its threshold, the host shows a warning that names value and threshold; the test was seen red with the threshold check removed | ✓ VERIFIED | `host.Assess` (`internal/host/health.go`) produces sentences such as `cpu_thermal 82.1 °C ≥ 80 °C` and `/ 91% used ≥ 80%`. Its lines come from the one Go rule `inventory.TemperatureLimits` and from 80 % of used + available. Measured red here: temperature check removed → `TestAssessWarns` red (M1); filesystem check removed → `TestAssessWarns`, `TestUnreadableIsNeverHealthy` red (M2); used/size as the denominator → red (M2b). On the page, the notice removed → 6 tests in `host.test.tsx` red (W5), and the browser's own 80/95 rule reinstated → `Sensors.test.tsx` red (W4). |
| 4 | The host stands in the navigation and on the wall like a node, with exactly one of three states; an unreadable host never appears as healthy, and the test holding that was seen red against a host that did | ✓ VERIFIED | Server: `Assess` ignoring unreadable values → `TestUnreadableIsNeverHealthy` red (M3). Wall: stale check removed → `TestTheWallsHost` red (M4); `default` mapped to ok → red (M5). Browser: the sidebar keeping the last answer after a failed poll → `Sidebar.test.tsx` red (W1); the wall tile drawing unknown as ok → `wall.test.tsx` red (W2); the `/host` header drawing unknown as ok → `host.test.tsx` red (W3); `HostStateMark` drawing the green dot instead of the ring for unknown → `Sidebar.test.tsx` and `host.test.tsx` red (W6b). The `health.state` schema is `z.enum(['ok','warn','unknown'])` with no default, and the wall falls back to `TILE_COLOURS.unknown`. |

### Plan Must-Haves (53 truths across 12-01 … 12-08)

| Plan | Truths | Status | Key evidence |
|---|---|---|---|
| 12-01 | 6/6 | ✓ | `sampleHost` runs before the inventory's early return (M7 → `TestTheHostIsSampledWhenTheInventoryFails` red). `retained` keeps `host/local` by name (M9 → `TestTheHostIsNeverForgotten` red). Route contract checked live (401/400/200). `TestHostHistoryAPI` passes. |
| 12-02 | 6/6 | ✓ | `internal/inventory/limits.go` `TemperatureLimits`, wired into `sensors.go` and `hardware.go`. The real Pi reported `cpu_thermal` warn 80 / danger 110 (the zone's critical trip; active trips ignored). `TestTemperatureLimits` and `TestTripPoints` pass. Assess sentences and ordering are covered by `TestAssessWarns`. |
| 12-03 | 7/7 | ✓ | `HistoryValues` leaves an unread value absent (M6, cpu written as 0 → red). `TestSampleHasItsOwnBaseline` passes. Health is on `/api/v1/host` (live: `ok`). `wallHostFrom` limits the tile to a 45-s staleness, uses `Public` sentences and passes the nodes and summary through untouched. `TestTheWallCarriesTheHost` passes. |
| 12-04 | 5/5 | ✓ | `temperatureSchema` has `warn_c`/`danger_c` with no default. `severityOf(t.celsius, t.warn_c, t.danger_c)`. The drive pair is `temperature_warn_c/_danger_c`. The fan animation is `motion-safe:animate-spin`. W4 went red. |
| 12-05 | 8/8 | ✓ | The shared `HardwareCharts.tsx` is used by both pages. "Not readable — no history" is at `host.tsx`. `useLiveSeries.test.ts` and `useChartRange.test.ts` pass. `host.png` shows the load figure and the hardening state. |
| 12-06 | 8/8 | ✓ | `HostStateMark` has three shapes plus `unanswered`. The sidebar shares the `['host']` query, polls every 30 s away from `/host` and not at all on it. The wall prepends a `data-kind=host` tile with no trend, and the headline counts `wall.nodes`. Backstop (390-px mount path): `host.browser.test.tsx` "wraps a 120-character mount path…" passed in Chromium. |
| 12-07 | 7/7 | ✓ | `typography.browser.test.tsx` passed. HEAD's layout audit (fresh bundle, own binary): exit 0, `/wall` and `/host` ok at 390 px and 1280 px, tap targets ok. Backstops: the fixture host name `manager-01.homelab.example` is 26 characters and fits at 390 px in the audit. The wall tile's reason truncates (`truncate`; visible in `wall.png` as "manager · cpu_thermal 8…"), and the full sentence is on `/host` (`break-words`). README "What it does" updated. `internal/publicrepo` passes. |
| 12-08 | 6/6 | ✓ | The contract covers `/api/v1/host/history`, the health block, `warn_c/danger_c` and the wall's `host`. The guide covers "Not readable — no history", trip points and the three states. Real binary under a real `subset=pid` /proc (user namespace, `/proc/stat` absent): series `rx`, `tx`, `temp:*` only, no cpu/memory/core, and health `ok`. Gate: see Behavioral Spot-Checks. The production service is active and untouched. |

**Score:** 57/57 truths verified (0 present-but-behavior-unverified).

### Required Artifacts

| Artifact | Status | Details |
|---|---|---|
| `internal/host/sample.go` | ✓ VERIFIED | `Sample`, `HistoryValues`, `Latest`, `samplePrev`, `ErrSampleBusy` |
| `internal/host/health.go` | ✓ VERIFIED | `Assess`, `Health{State,Summary,Warnings,Unreadable,Public}` |
| `internal/inventory/limits.go` | ✓ VERIFIED | `TemperatureLimits` |
| `internal/history/sampler.go` | ✓ VERIFIED | `SamplerDeps.Host`, `sampleHost` with a `HostBudget` deadline |
| `internal/httpapi/handlers/history.go` | ✓ VERIFIED | `/api/v1/host/history` |
| `internal/httpapi/handlers/wall_host.go` | ✓ VERIFIED | `hostForTheWall`, `wallHostFrom`, "not readable" |
| `web/src/components/HostState.tsx` | ✓ VERIFIED | `HostStateMark`, `AlertTriangle` |
| `web/src/components/charts/HardwareCharts.tsx` | ✓ VERIFIED | Imported by `NodeHardware.tsx` and `routes/host.tsx` |
| `web/fixtures/demo.json` | ✓ VERIFIED | `/api/v1/host/history`, the warn health, the wall host |
| `docs/screenshots/host.png`, `wall.png` | ✓ VERIFIED (see warning) | Both show the feature; `host.png`'s notice footer is one wording behind |
| `docs/api-contract.md`, `docs/guide.md` | ✓ VERIFIED | Content present |

### Key Link Verification

| From | To | Via | Status |
|---|---|---|---|
| `cmd/holzkube-managerd/main.go` | `host.Collector.Sample` | `Host: hostCollector.Sample` in `NewSampler` | WIRED |
| `internal/history/sampler.go` | `history.Store` | `Record(HostSubject(), at, values)` | WIRED |
| `internal/httpapi/handlers/kubernetes.go` | `wall_host.go` | `newWallAnswer(…, hostForTheWall(d, now))` | WIRED |
| `internal/host/sample.go` | `health.go` | `Assess(live)` into `Snapshot` | WIRED |
| `web/src/routes/host.tsx` | `api.hostHistory` | `useChartRange(['host'], api.hostHistory)` | WIRED |
| `web/src/components/Sidebar.tsx` | `api.host` | `useQuery({queryKey:['host']…})` → `HostMark` | WIRED |
| `web/src/routes/wall.tsx` | `wall.host` | prepended to the Nodes tiles | WIRED |
| `web/src/components/charts/Sensors.tsx` | server limits | `severityOf(t.celsius, t.warn_c, t.danger_c)` | WIRED |

### Data-Flow Trace (Level 4)

| Artifact | Data | Source | Real data | Status |
|---|---|---|---|---|
| `/host` charts | `api.hostHistory` series | `history.Store` ← sampler ← `host.Collector.Sample` ← sysfs//proc | yes (real dev daemon: 10 series filling every 15 s) | ✓ FLOWING |
| `/host` state line and notice | `host.health` | `Assess(readLive)` on `GET /api/v1/host` | yes (live `ok` with its summary) | ✓ FLOWING |
| Sidebar mark | `['host']` query `health.state` | same | yes | ✓ FLOWING |
| Wall host tile | `wall.host` | `Collector.Latest()` snapshot from the sampler | yes in `TestTheWallCarriesTheHost` (simulated cluster); not observable on a dev daemon without a cluster | ✓ FLOWING (test) |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|---|---|---|---|
| Phase Go packages | `go test ./internal/host/... ./internal/history/... ./internal/httpapi/... ./internal/inventory/... ./internal/publicrepo/... ./internal/talos/... ./cmd/...` | all ok | ✓ PASS |
| Whole Go workspace, once | `go test -count=1 ./...` | exit 1: only `internal/upgrade TestANodeSaysHowItBooted` (talos Get timed out under load); rerun alone ok in 5.5 s; no phase-12 commit touches `internal/upgrade` | ✓ PASS (known flake, recorded) |
| Web suite | `npx vitest run` | 58 files, 594 tests passed, exit 0 | ✓ PASS |
| Typecheck | `npx tsc --noEmit` | exit 0 | ✓ PASS |
| Browser tests | `vitest --project browser host.browser typography.browser` | 6/6 passed | ✓ PASS |
| Linters | `./bin/task lint:go`, `./bin/task lint:web` | 0 issues; exit 0 (2 known Biome warnings, see deferred-items) | ✓ PASS |
| Layout audit on HEAD | fresh `vite build` + binary in scratch, `node scripts/layout-audit.mjs` | exit 0, "Nothing out of reach at 390px and 1280px" | ✓ PASS |
| Cross builds | `GOOS=linux GOARCH=amd64`, `GOOS=darwin GOARCH=arm64` `go build ./cmd/holzkube-managerd` | both exit 0 | ✓ PASS |
| Restart keeps history, downtime is a gap | dev daemon, SIGTERM, 42-s pause, restart | point before stop kept; 0 points in downtime | ✓ PASS |
| subset=pid records nothing hidden | dev daemon under `unshare -Urmpf` + `mount -o subset=pid proc` | keys `rx, tx, temp:*` only; health `ok` | ✓ PASS |

### Fault Injections Performed by This Verifier (in a scratch copy, never in the checkout)

| # | Fault | Guard that went red |
|---|---|---|
| M1 | temperature threshold check always skipped | `TestAssessWarns` |
| M2 | filesystem threshold check always fails | `TestAssessWarns`, `TestUnreadableIsNeverHealthy` |
| M2b | used/size as denominator | `TestAssessWarns` |
| M3 | Assess ignores unreadable | `TestUnreadableIsNeverHealthy` |
| M4 | wall staleness check removed | `TestTheWallsHost` |
| M5 | wall maps unknown to healthy | `TestTheWallsHost` |
| M6 | `cpu` written as 0 when hidden | `TestHistoryValues`, `TestHistoryValuesSensors` |
| M7 | host sampled after the inventory | `TestTheHostIsSampledWhenTheInventoryFails` |
| M8 | persistence drops `host/local` | `TestTheHostHistorySurvivesARestartWithItsGap` |
| M9 | retention drops `host/local` | `TestTheHostIsNeverForgotten` |
| W1 | sidebar keeps last answer after failed poll | `Sidebar.test.tsx` |
| W2 | wall tile draws unknown as ok | `wall.test.tsx` |
| W3 | `/host` header draws unknown as ok | `host.test.tsx` |
| W4 | browser's own 80/95 temperature rule | `Sensors.test.tsx` (2) |
| W5 | warning notice removed | `host.test.tsx` (6) |
| W6b | mark draws the green dot, not the ring, for unknown | `Sidebar.test.tsx`, `host.test.tsx` |
| W7 | `segments` ignores the gap mark | `useChartRange.test.ts`, `Sensors.test.tsx`, `host.test.tsx` |
| W6 | mark draws the green dot **beside** the ring for unknown | **stayed green**: see Anti-Patterns (Info) |

### Requirements Coverage

| Requirement | Source plans | Description | Status | Evidence |
|---|---|---|---|---|
| HMON-05 | 12-01, 12-03, 12-05, 12-07, 12-08 | History charts over 1 h / 6 h / 24 h for CPU, memory, temperature and network that survive a daemon restart | ✓ SATISFIED | SC 1 and 2 above, plus the real-binary restart run |
| HMON-07 | 12-02, 12-03, 12-04, 12-06, 12-07, 12-08 | A warning with its reason when temperature or disk usage crosses a threshold | ✓ SATISFIED | SC 3 above |
| HOST-04 | 12-02, 12-03, 12-06, 12-07, 12-08 | The host in the navigation and on the wall like a node, with a state | ✓ SATISFIED | SC 4 above |

No orphaned requirements: REQUIREMENTS.md maps exactly HOST-04, HMON-05 and HMON-07 to Phase 12.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|---|---|---|---|---|
| (phase files) | — | TBD/FIXME/XXX/TODO | none found | — |
| `docs/screenshots/host.png` | — | stale render: the notice footer still reads "…at its chip's own limit, or at a default for its kind…". The UI (IN-05, `web/src/routes/host.tsx:252`) now also names the thermal zone's trip point. | ⚠️ Warning | README picture one wording behind the product. Fix: `task build && node web/scripts/readme-images.mjs`, commit only `host.png`. Does not affect the goal. |
| `web/src/components/Sidebar.test.tsx:144-146`, `web/src/routes/host.test.tsx:417-420` | — | the unknown-state assertions check that the ring is present but not that the green dot is absent (the unanswered case at `Sidebar.test.tsx:169-170` does both). A mark that drew both shapes stayed green (W6). | ℹ️ Info | SC 4's guard still goes red for a mark that draws the green dot instead of the ring (W6b). One `expect(…'.rounded-full:not(.border)').toBeNull()` in each would close it. |
| `internal/httpapi/handlers/wall_host.go:60`, `kubernetes.go:519` | — | IN-01 (skipped in review-fix): the staleness limit `3*history.FineStep` does not follow `SamplerDeps.Interval`, and `now` is taken before `ForTheWall`. | ℹ️ Info | With production's fixed 15-s interval the tile still greys at 45 s. It can understate the age by the cluster call's duration. Recorded, not blocking. |
| `internal/host/health.go` (CR-01 fix) | — | a temperature input that fails permanently keeps the host "Not readable" (D-12 over D-11) | ℹ️ Info | Product trade-off named in 12-REVIEW-FIX. The Pi's inputs all read cleanly (seen live: `ok`). |

### Human Verification Required

#### 1. The production view after the next release

**Test:** After the operator's next release has installed itself through the update timer, open `/host` signed in. Switch 1 h / 6 h / 24 h. Look again after the next hourly update restart. Open the wall.
**Expected:** The state line, and the amber notice naming value and threshold when a line is crossed. Processor and Memory show "Not readable — no history" under `ProcSubset=pid`, and Network and Sensors draw curves. The curves from before the restart are still there, with a gap for the downtime. The Host entry in the navigation has its mark, and the wall shows the "manager" tile first in Nodes.
**Why human:** The production binary predates this phase, and releasing, replacing the binary or restarting the service is the operator's call. Everything else was checked on this Pi against a dev daemon built from HEAD, including under a real `subset=pid` /proc. The wall route needs an adopted cluster, which a dev daemon has not.

### Gaps Summary

No gaps. All four roadmap success criteria and all 53 plan truths hold in the code. Every "seen red" claim on the success criteria was reproduced by this verifier with its own fault injections, and the history's restart and gap behaviour was observed on the real arm64 binary. What is left:

- one human item: the production view after a release;
- one warning: the README's `host.png` is a wording behind the notice text;
- two informational hardening points: the unknown-mark assertions, and IN-01.

---

_Verified: 2026-09-29T01:45:00Z_
_Verifier: Claude (gsd-verifier), on the operator's Pi 5 (aarch64)_

## Orchestrator disposition (2026-09-29, unattended run)

The only human item — the production view after a release — is carried in
`12-UAT.md` as pending and will be listed in the milestone's closing report.
Everything checkable before a release was checked on the Pi (57/57). The
status is set to `passed` for routing on that basis, not because the human
item was done. The stale host.png is re-rendered in phase 13 (13-09).
