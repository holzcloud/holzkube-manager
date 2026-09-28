---
phase: "11"
slug: "host-seite"
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
# audit-milestone §5.5 distinguishes NOT-VALIDATED (draft) from PARTIAL (validated + nyquist_compliant: false) (#2117)
status: draft
nyquist_compliant: false
wave_0_complete: false
created: "2026-09-28"
---

# Phase 11 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (Go 1.26.7 via GOTOOLCHAIN) + vitest (web) |
| **Config file** | Taskfile.yml (test, test:web, test:layout); web/vite.config.ts (vitest projects jsdom + browser) |
| **Quick run command** | `GOTOOLCHAIN=go1.26.7 go test ./internal/host/... ./internal/httpapi/...` |
| **Full suite command** | `./bin/task ci` |
| **Estimated runtime** | ~11 seconds |

---

## Sampling Rate

- **After every task commit:** Run the quick run command
- **After every plan wave:** Run the full suite command
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 11 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 11-01-T1 (tracer) | 01 | 1 | HOST-01 (D-01, D-02, D-10, D-11) | T-11-01, T-11-04, T-11-05 | 401 without session, reader 200, no audit record, bounded reads | unit + HTTP integration + component | `GOTOOLCHAIN=go1.26.7 go test ./internal/host -count=1 -run 'TestTracerReadsUname\|TestUnreadableUnameHasNoValue\|TestEveryReadingIsWellFormed\|TestNoNulls'` · `go test ./internal/httpapi -run TestHostAPI` · `go test ./cmd/holzkube-managerd -run TestEveryRouteThatReachesUpstreamHasABudgetRow` · `npm --prefix web run test -- --project jsdom src/routes/host.test.tsx` | ❌ created by the task | ⬜ |
| 11-01-T2 | 01 | 1 | HOST-01 (D-04, D-17) | T-11-03 | PID 1 environ reduced to a boolean, never serialised | unit + live kernel + component | `GOTOOLCHAIN=go1.26.7 go test ./internal/host -count=1 -run 'TestIdentity\|TestCountCPUList\|TestOSRelease\|TestContainer\|TestLiveHostMatchesTheKernel'` · `npm --prefix web run test -- --project jsdom src/routes/host.test.tsx` | ❌ created by the task | ⬜ |
| 11-01-T3 | 01 | 1 | HOST-01 | — | — | fixture schema + layout e2e | `npm --prefix web run test -- --project jsdom src/fixtures.test.ts` · `./bin/task test:layout` | ✅ extend | ⬜ |
| 11-02-T1 | 02 | 2 | HOST-03 (D-16) | T-11-09 | 4 KiB cap, strict field validation, nothing synthesised | unit | `GOTOOLCHAIN=go1.26.7 go test ./internal/host/updatestatus -count=1 -run TestReadStatus` | ❌ created by the task | ⬜ |
| 11-02-T2 | 02 | 2 | HOST-03 (D-14, D-15) | T-11-06, T-11-07, T-11-08, T-11-10 | symlinked / foreign-owned dir refused as root; exit codes unchanged; JSON via json.dump | integration (bash, stubs, user namespace) | `bash -n deploy/holzkube-manager-update.sh` · `GOTOOLCHAIN=go1.26.7 go test ./internal/host/updatestatus -count=1 -run 'TestUpdateScript\|TestReadStatus'` | ❌ created by the task | ⬜ |
| 11-03-T1 | 03 | 2 | HMON-06, HMON-01 (D-03, D-05) | T-11-12 | mountinfo read capped at 1 MiB | unit | `GOTOOLCHAIN=go1.26.7 go test ./internal/host -count=1 -run 'TestMountinfo\|TestProcSubsetPid\|TestProcStat\|TestMeminfo\|TestLoadavg\|TestLoadFromSysinfo\|TestClassifyNeedsTheMountAsProof'` | ❌ created by the task | ⬜ |
| 11-03-T2 | 03 | 2 | HMON-06, HMON-01 (D-02, D-05, D-12) | T-11-11 | a hidden value carries no number; guard seen red against a reinstated 0 | unit + real kernel (unshare, subset=pid) | `GOTOOLCHAIN=go1.26.7 go test ./internal/host -count=1 -run 'TestHardeningHidesCPUAndMemory\|TestHiddenValuesCarryNoNumber\|TestLivePi5FirstRead\|TestRates\|TestCoreCountChange\|TestLoadUnreadable\|TestRealKernelProcSubset'` · `go test ./internal/inventory` | ❌ created by the task | ⬜ |
| 11-03-T3 | 03 | 2 | HMON-06, HMON-01 (D-06, D-10, D-12) | T-11-11 | no 0 / 0 % / 0 B rendered for a hidden value | component | `npm --prefix web run test -- --project jsdom src/routes/host.test.tsx src/fixtures.test.ts` · `npm --prefix web run typecheck` · `npm --prefix web run lint` | ✅ extend | ⬜ |
| 11-04-T1 | 04 | 3 | HMON-02 (D-07) | T-11-14 | 60 s cache, singleflight, 5 s walk deadline | unit (+ du comparison on linux) | `GOTOOLCHAIN=go1.26.7 go test ./internal/host -count=1 -run 'TestFilesystems\|TestStatfsMatchesDf\|TestFilesystemStatfsFails\|TestFilesystemKeysCoverInventory\|TestDataDirSize'` · darwin/arm64 build | ❌ created by the task | ⬜ |
| 11-04-T2 | 04 | 3 | HOST-02, HOST-03 (D-16) | T-11-16 | absolute status path required; strict reader | unit + config guards | `GOTOOLCHAIN=go1.26.7 go test ./internal/host -count=1 -run 'TestServiceSection\|TestUpdateStatusReadings'` · `go test ./internal/config ./cmd/holzkube-managerd ./internal/host/updatestatus` | ❌ / ✅ extend | ⬜ |
| 11-04-T3 | 04 | 3 | HOST-02, HOST-03, HMON-02 | T-11-15 | — | component + layout | `npm --prefix web run test -- --project jsdom src/routes/host.test.tsx src/fixtures.test.ts` · `./bin/task test:layout` | ✅ extend | ⬜ |
| 11-05-T1 | 05 | 4 | HMON-03 (D-08, D-13) | T-11-17 | chip/input caps, bounded reads | unit | `GOTOOLCHAIN=go1.26.7 go test ./internal/talos -count=1` · `go test ./internal/host -run 'TestSensors\|TestThermalTwinRule'` | ❌ / ✅ extend | ⬜ |
| 11-05-T2 | 05 | 4 | HMON-03 (D-13) | — | — | component (NodeHardware unchanged) | `npm --prefix web run test -- --project jsdom src/components/NodeHardware.test.tsx` · `npm --prefix web run typecheck` | ✅ | ⬜ |
| 11-05-T3 | 05 | 4 | HMON-03 (D-08) | T-11-18 | — | component + layout | `npm --prefix web run test -- --project jsdom src/routes/host.test.tsx src/fixtures.test.ts src/components/NodeHardware.test.tsx` · `./bin/task test:layout` | ✅ extend | ⬜ |
| 11-06-T1 | 06 | 5 | HMON-04 (D-09, D-12) | T-11-19, T-11-20 | no MAC read; 512-entry cap | unit | `GOTOOLCHAIN=go1.26.7 go test ./internal/host -count=1 -run 'TestNetwork\|TestLinkKeysMatchInventory\|TestRates'` | ❌ created by the task | ⬜ |
| 11-06-T2 | 06 | 5 | HMON-04 (D-09, D-17) | — | — | component + layout (tap target) | `npm --prefix web run test -- --project jsdom src/routes/host.test.tsx src/fixtures.test.ts` · `./bin/task test:layout` | ✅ extend | ⬜ |
| 11-07-T1 | 07 | 6 | all (contract, guide) | T-11-21 | no real identifiers in docs | unit (contract/readme/publicrepo guards) | `GOTOOLCHAIN=go1.26.7 go test ./internal/httpapi -run 'TestEveryProblemCodeIsInTheContract\|TestEveryProblemCodeIsEmitted'` · `go test ./internal/config ./internal/publicrepo` | ✅ | ⬜ |
| 11-07-T2 | 07 | 6 | HOST-01, HMON-03, HMON-04 (UI-SPEC backstops) | — | — | browser (390 px) | `npm --prefix web run test:browser -- src/routes/host.browser.test.tsx` | ❌ created by the task | ⬜ |
| 11-07-T3 | 07 | 6 | all eight | T-11-21, T-11-22 | production service untouched; comparison output anonymised | gate + on-Pi comparison vs shell + real subset=pid run | `./bin/task ci` · linux/amd64 and darwin/arm64 builds | ✅ | ⬜ |

Fault injections (CLAUDE.md: every guard seen red against the reinstated fault) are named per task in each PLAN's action and acceptance criteria; the set follows 11-RESEARCH.md § "Fault injections", extended with the identity, filesystem cache, sensors, network and layout guards.

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/host/testdata/` fixture trees (pi5, amd64, procsubset-pid) — per 11-RESEARCH.md; created in plan 01 (identity), plan 03 (proc, procsubset-pid), plan 05 (hwmon/thermal), plan 06 (net)
- [ ] `internal/host/helpers_test.go` — `fakeSys`, `fixtureFS` (os.OpenRoot), `marshalView` (plan 01 T1)
- [ ] `internal/host/collector_test.go` — `TestEveryReadingIsWellFormed`, `TestNoNulls` with the `nullableKeys` allowlist (plan 01 T1; extended by plans 03, 04, 05, 06)
- [ ] `internal/httpapi/hostapi_test.go` + `withHost` harness option (plan 01 T1)
- [ ] `web/src/routes/host.test.tsx` (plan 01 T1, extended by every later plan)
- [ ] `internal/host/updatestatus/status_test.go`, `script_test.go` (plan 02)
- [ ] `internal/host/realkernel_test.go` (plan 03 T2)
- [ ] `web/src/routes/host.browser.test.tsx` (plan 07 T2)
- [ ] extend: `web/src/fixtures.test.ts`, `web/fixtures/demo.json`, `web/scripts/layout-audit.mjs` `ROUTES`, `cmd/holzkube-managerd/budget_test.go` `noUpstream`, `internal/config/config_test.go` values table

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Values match the shell on the Pi | HOST-01, HMON-01..04 | needs the real host | scripted in plan 07 T3 against a dev daemon: `uname -n -r -m`, `/proc/uptime`, device-tree model, `/etc/os-release`, `/proc/meminfo`, `df -B1 /`, `du -s -B1`, `/sys/class/thermal/thermal_zone0/temp`, eth0 counters |
| Hardening answer on the real kernel | HMON-06 | needs a real `subset=pid` proc mount | plan 03 T2 `TestRealKernelProcSubset` (automated, user namespace) and plan 07 T3 dev daemon under `unshare … mount -t proc -o subset=pid` |
| /host on the operator's production service | HOST-01..03, HMON-01..04, HMON-06 | the production binary is replaced only by the operator's update timer, never by a session | after the next release installs itself: open /host, expect the hardening notice with ProcSubset=all, "Not recorded" until the new script has run once |

*If none: "All phase behaviors have automated verification."*

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references (each test file is created by the task that needs it)
- [x] No watch-mode flags (`vitest run`, `go test -count=1`)
- [ ] Feedback latency < 11s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** {pending / approved YYYY-MM-DD}
