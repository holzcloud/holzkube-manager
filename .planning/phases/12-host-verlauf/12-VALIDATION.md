---
phase: "12"
slug: "host-verlauf"
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
# audit-milestone §5.5 distinguishes NOT-VALIDATED (draft) from PARTIAL (validated + nyquist_compliant: false) (#2117)
status: draft
nyquist_compliant: false
wave_0_complete: false
created: "2026-09-28"
---

# Phase 12 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (Go 1.26.7 via GOTOOLCHAIN) + vitest (web) |
| **Config file** | Taskfile.yml (test, test:web, test:layout); web/vite.config.ts |
| **Quick run command** | `GOTOOLCHAIN=go1.26.7 go test ./internal/host/... ./internal/history/... ./internal/httpapi/...` |
| **Full suite command** | `./bin/task ci` |
| **Estimated runtime** | ~12 seconds |

---

## Sampling Rate

- **After every task commit:** Run the quick run command
- **After every plan wave:** Run the full suite command
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 12 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 12-01-T1 (tracer) | 01 | 1 | HMON-05 (D-01..D-05) | T-12-01, T-12-03 | route behind session + reader, not a wall-link route; Sample never walks the data dir | HTTP integration (real collector → sampler → store → route) + component + route gates | `GOTOOLCHAIN=go1.26.7 go test ./internal/httpapi -count=1 -run 'TestHostHistoryAPI'` · `go test ./cmd/holzkube-managerd -run 'TestEveryRouteThatReachesUpstreamHasABudgetRow\|TestEveryRouteIsReachableFromTheInterface\|TestEveryRouteTheClientIssuesIsCalledByAScreen\|TestOnlyTheWallAcceptsAWallLink'` · `npm --prefix web exec -- vitest run --project jsdom src/routes/host.test.tsx` · `npm --prefix web run test:browser -- src/routes/host.browser.test.tsx` | ❌ created by the task | ⬜ |
| 12-01-T2 | 01 | 1 | HMON-05 (D-01, D-02, D-04) | T-12-01, T-12-02 | 401 / reader 200 / 400 range / 502; not audited | unit + HTTP integration + component | `GOTOOLCHAIN=go1.26.7 go test ./internal/history ./internal/host -count=1 -run 'TestTheHostIsNeverForgotten\|TestTheSamplerRecordsTheHost\|TestTheHostIsSampledWhenTheInventoryFails\|TestAFailingHostIsLoggedOnce\|TestHistoryValuesSensors'` · `go test ./internal/httpapi -run 'TestHostHistoryAPI'` · `vitest run --project jsdom src/routes/host.test.tsx` | ❌ created by the task | ⬜ |
| 12-02-T1 | 02 | 1 | HMON-07 (D-06) | — | — | unit (ported browser rule) | `GOTOOLCHAIN=go1.26.7 go test ./internal/inventory -count=1 -run 'TestTemperatureLimits\|TestHardware'` | ❌ created by the task | ⬜ |
| 12-02-T2 | 02 | 1 | HMON-07 (D-07) | T-12-05, T-12-06 | trip-point reads capped (16/zone) and bounded; active trips never a threshold | unit (pi5 fixture + MapFS) | `GOTOOLCHAIN=go1.26.7 go test ./internal/host -count=1 -run 'TestTripPoints\|TestSensors\|TestThermalTwinRule\|TestZonesOnlyWithoutCPUChip'` | ❌ created by the task (fixture extended) | ⬜ |
| 12-02-T3 | 02 | 1 | HMON-07, HOST-04 (D-08..D-12) | T-12-06 | unreadable rated values never ok; df denominator and >= | unit (tables) | `GOTOOLCHAIN=go1.26.7 go test ./internal/host -count=1 -run 'TestAssessWarns\|TestUnreadableIsNeverHealthy'` | ❌ created by the task | ⬜ |
| 12-03-T1 | 03 | 2 | HMON-05 (D-03) | T-12-03 | no 0 for an unreadable value; sampler's own rate window | unit + file round trip | `GOTOOLCHAIN=go1.26.7 go test ./internal/host -count=1 -run 'TestHistoryValues\|TestSampleHasItsOwnBaseline\|TestRates\|TestNetworkRates'` · `go test ./internal/history -run 'TestTheHostHistorySurvivesARestartWithItsGap'` | ❌ created by the task | ⬜ |
| 12-03-T2 | 03 | 2 | HOST-04, HMON-07 (D-06, D-10, D-11) | — | health lists never null | unit + HTTP | `GOTOOLCHAIN=go1.26.7 go test ./internal/host -count=1 -run 'TestLatest\|TestReadCarriesHealth\|TestNoNulls\|TestEveryReadingIsWellFormed'` · `go test ./internal/httpapi -run 'TestHostAPI'` | ❌ created by the task | ⬜ |
| 12-03-T3 | 03 | 2 | HOST-04 (D-14, D-11) | T-12-08, T-12-09, T-12-10, T-12-11 | wall carries name/state/fixed reason only; stale → unknown; wall link refused on /api/v1/host(/history) | unit + HTTP integration (kubesim cluster, session and wall link) | `GOTOOLCHAIN=go1.26.7 go test ./internal/httpapi/handlers -count=1 -run 'TestTheWallsHost'` · `go test ./internal/httpapi -run 'TestTheWallCarriesTheHost\|TestAWallLinkOpensTheWallAndNothingElse'` · `go test ./cmd/holzkube-managerd -run 'TestOnlyTheWallAcceptsAWallLink'` | ❌ created by the task | ⬜ |
| 12-04-T1 | 04 | 2 | HMON-07 (D-06) | — | — | component | `npm --prefix web exec -- vitest run --project jsdom src/components/NodeHardware.test.tsx src/components/charts/Sensors.test.tsx` | ✅ extend | ⬜ |
| 12-04-T2 | 04 | 2 | HMON-07 (D-06) | T-12-12 | no browser-side limit or default | component + fixture schema + browser | `vitest run --project jsdom src/components/charts/Sensors.test.tsx src/components/NodeHardware.test.tsx src/routes/host.test.tsx src/fixtures.test.ts` · `npm --prefix web run test:browser -- src/routes/host.browser.test.tsx` | ✅ extend | ⬜ |
| 12-05-T1 | 05 | 3 | HMON-05 (D-05) | — | — | component + hook unit | `vitest run --project jsdom src/components/NodeHardware.test.tsx src/components/charts/Sensors.test.tsx src/hooks/useLiveSeries.test.ts src/hooks/useChartRange.test.ts` | ❌ useLiveSeries.test.ts created by the task | ⬜ |
| 12-05-T2 | 05 | 3 | HMON-05 (D-05) | T-12-13 | no 0 drawn for an unreadable value | component + browser | `vitest run --project jsdom src/routes/host.test.tsx` · `npm --prefix web run test:browser -- src/routes/host.browser.test.tsx` | ✅ extend | ⬜ |
| 12-05-T3 | 05 | 3 | HMON-05 (checker resolutions 1, 2, 6) | — | — | component | `vitest run --project jsdom src/routes/host.test.tsx` | ✅ extend | ⬜ |
| 12-06-T1 | 06 | 4 | HMON-07, HOST-04 (D-09, D-10) | — | server sentences verbatim, in order | component + browser (390 px) | `vitest run --project jsdom src/routes/host.test.tsx src/fixtures.test.ts` · `npm --prefix web run test:browser -- src/routes/host.browser.test.tsx` | ✅ extend | ⬜ |
| 12-06-T2 | 06 | 4 | HOST-04 (D-13) | T-12-14, T-12-15 | never green after a failed poll; no poll of its own on /host | component (memory router, fake timers) | `vitest run --project jsdom src/components/Sidebar.test.tsx` | ❌ created by the task | ⬜ |
| 12-06-T3 | 06 | 4 | HOST-04 (D-14) | — | — | component + fixture schema | `vitest run --project jsdom src/routes/wall.test.tsx src/fixtures.test.ts` · `npm --prefix web run test` | ✅ extend | ⬜ |
| 12-07-T1 | 07 | 5 | HMON-07 (checker resolution 4) | — | — | browser measurement | `npm --prefix web run test:browser -- src/typography.browser.test.tsx` · `npm --prefix web run test` | ❌ created by the task | ⬜ |
| 12-07-T2 | 07 | 5 | HMON-05, HOST-04 | T-12-17 | audit and renderer never draw the real host | fixture schema + layout e2e | `vitest run --project jsdom src/fixtures.test.ts` · `./bin/task test:layout` | ✅ extend | ⬜ |
| 12-07-T3 | 07 | 5 | HMON-05, HMON-07, HOST-04 (README rule) | T-12-17 | no identifier of the real installation in pictures | public-repo gate | `GOTOOLCHAIN=go1.26.7 go test ./internal/publicrepo -count=1` | ✅ | ⬜ |
| 12-08-T1 | 08 | 6 | HMON-05, HMON-07, HOST-04 | T-12-18 | — | contract gates | `GOTOOLCHAIN=go1.26.7 go test ./internal/httpapi -count=1 -run 'TestEveryProblemCodeIsInTheContract\|TestEveryProblemCodeIsEmitted'` · `go test ./internal/config ./internal/publicrepo` | ✅ | ⬜ |
| 12-08-T2 | 08 | 6 | HMON-05, HMON-07, HOST-04 | T-12-18, T-12-19 | production service untouched (ActiveEnterTimestamp before = after) | full gate + dev daemon on the Pi (restart, subset=pid) | `./bin/task ci` · amd64 and darwin builds · `go test ./internal/publicrepo` | ✅ | ⬜ |

### Fault injections (each seen red, then restored; recorded in the plan's SUMMARY)

| Plan-Task | Fault reinstated | Expected red |
|-----------|------------------|--------------|
| 12-01-T2 | `HostSubject()` returns `"machine/local"` | `TestTheHostIsNeverForgotten` |
| 12-01-T2 | `sampleHost` called after the inventory's early return | `TestTheHostIsSampledWhenTheInventoryFails` |
| 12-01-T2 | sensor keys written as 0 when the reading is hidden | `TestHistoryValuesSensors` |
| 12-01-T2 | `hostHistory` ignores `range` | `TestHostHistoryAPI` 400 case |
| 12-01-T2 | RangePicker note always shown | host.test.tsx readable-history case |
| 12-02-T1 | `high < danger` guard dropped | `TestTemperatureLimits` |
| 12-02-T2 | `active` counted as high; `critical` ignored; trip cap removed | `TestTripPoints` |
| 12-02-T3 | temperature check → false; filesystem check → false; `used/size` | `TestAssessWarns` |
| 12-02-T3 | Assess returns ok whatever it could not read | `TestUnreadableIsNeverHealthy` |
| 12-03-T1 | `cpu = 0` when hidden; Virtual links in rx; sampler on `&c.prev`; encode skips `host/local` | `TestHistoryValues`, `TestSampleHasItsOwnBaseline`, `TestTheHostHistorySurvivesARestartWithItsGap` |
| 12-03-T2 | `Read` leaves `Health` zero | `TestReadCarriesHealth`, `TestHostAPI` |
| 12-03-T3 | staleness check skipped; host appended to `Wall.Nodes` | `TestTheWallsHost` |
| 12-04-T1 | drive figure on fixed 60/70; bare `animate-spin` | NodeHardware.test.tsx, Sensors.test.tsx |
| 12-04-T2 | browser rule reinstated in `Sensors` | Sensors.test.tsx "the limits are the server's" |
| 12-05-T1 | `append` drops keys absent from a reading | useLiveSeries.test.ts |
| 12-05-T2 | empty chart for an unreadable key; pick writes `cpu: 0`; `rate.no-baseline` treated as unreadable | host.test.tsx |
| 12-05-T3 | usage in the figure slot when hidden; plural-only core line | host.test.tsx |
| 12-06-T1 | warnings re-sorted in the browser; `break-words` dropped; notice for unknown | host.test.tsx, host.browser.test.tsx |
| 12-06-T2 | last answer drawn after an error; 30-s poll on /host too | Sidebar.test.tsx |
| 12-06-T3 | host counted in the headline; host tile lends a node's trend | wall.test.tsx |
| 12-07-T1 | the 12-px spacing fix removed | typography.browser.test.tsx |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

No separate Wave 0: every new test file is created by the task whose `<verify>` runs it, so no `<automated>` command references a file that does not exist yet.

- [ ] `internal/history/host_test.go` (12-01-T2; extended 12-03-T1)
- [ ] `internal/host/sample_test.go` (12-01-T2; extended 12-03-T1, 12-03-T2)
- [ ] `internal/inventory/limits_test.go` (12-02-T1)
- [ ] `internal/host/health_test.go` (12-02-T3)
- [ ] `internal/host/testdata/pi5/.../thermal_zone0/trip_point_{1..4}_{temp,type}` (12-02-T2)
- [ ] `internal/httpapi/handlers/wall_host_test.go`, `internal/httpapi/wallhostapi_test.go` (12-03-T3)
- [ ] `web/src/hooks/useLiveSeries.test.ts` (12-05-T1)
- [ ] `web/src/components/Sidebar.test.tsx` (12-06-T2)
- [ ] `web/src/typography.browser.test.tsx` (12-07-T1)
- [ ] extend: `internal/httpapi/historyapi_test.go` (`TestHostHistoryAPI`), `hostapi_test.go`, `walllinkapi_test.go`, `cmd/holzkube-managerd/budget_test.go` `noUpstream`, `web/src/routes/host.test.tsx`, `host.browser.test.tsx`, `wall.test.tsx`, `Sensors.test.tsx`, `NodeHardware.test.tsx`, `web/src/fixtures.test.ts`, `web/fixtures/demo.json`

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Host history and wall tile on the production service | HMON-05, HOST-04 | needs a release (operator's call) | after the next release: open /host, switch 1h/6h/24h, restart the service, check the history survived; open the wall |
| The wall's host tile against a live dev daemon | HOST-04 | a dev daemon has no adopted cluster, so its wall route cannot answer | covered by `TestTheWallCarriesTheHost` (12-03-T3) against a simulated cluster, as a session and as a wall link; stated in 12-08-SUMMARY |

*Everything else has automated verification; 12-08-T2 also exercises history persistence and the subset=pid case on the real binary and kernel.*

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references (each test file is created by the task that needs it)
- [x] No watch-mode flags (`vitest run`, `go test -count=1`)
- [ ] Feedback latency < 12s (the quick Go command is ~12 s; `internal/httpapi/handlers` alone measured 57 s in research, so a full per-task run can exceed it)
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** {pending / approved YYYY-MM-DD}
