---
phase: "13"
slug: "host-aktionen"
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
# audit-milestone §5.5 distinguishes NOT-VALIDATED (draft) from PARTIAL (validated + nyquist_compliant: false) (#2117)
status: draft
nyquist_compliant: true
wave_0_complete: false
created: "2026-09-29"
---

# Phase 13 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (Go 1.26.7 via GOTOOLCHAIN) + vitest (web) + shell script tests in a user namespace |
| **Config file** | Taskfile.yml (test, test:web, test:layout); web/vite.config.ts |
| **Quick run command** | `GOTOOLCHAIN=go1.26.7 go test ./internal/host/... ./internal/httpapi/...` |
| **Full suite command** | `./bin/task ci` |
| **Estimated runtime** | ~13 seconds |

---

## Sampling Rate

- **After every task commit:** Run the quick run command
- **After every plan wave:** Run the full suite command
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 13 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 13-01-T1 (tracer) | 01 | 1 | HACT-04, HACT-05, HACT-06 (D-01..D-09, D-15) | T-13-01..T-13-09 | token bound to `host.update` + `@host`; one link-exclusive 0600 order; script consumes before acting, fixed argv, records `started`; strict result reader | HTTP integration (real routes → Box → fsstore → root script under unshare → reader → GET /api/v1/host) + route-table guards + component | `GOTOOLCHAIN=go1.26.7 go test ./internal/httpapi -count=1 -run 'TestHostActionRoundTrip' -v` · `go test ./cmd/holzkube-managerd -run 'TestEveryRouteThatReachesUpstreamHasABudgetRow\|TestEveryRouteIsReachableFromTheInterface\|TestEveryRouteTheClientIssuesIsCalledByAScreen\|TestEveryAuditedActionIsInTheAllowlist\|TestEveryAllowlistedActionIsAReachableRoute\|TestOnlyTheWallAcceptsAWallLink'` · `cd web && npx vitest run --project jsdom src/components/HostActions.test.tsx src/routes/host.test.tsx` · `bash -n deploy/holzkube-manager-host.sh` | ❌ created by the task | ⬜ |
| 13-01-T2 | 01 | 1 | HACT-05 (D-09) | T-13-02 | host table: four keys, all true, disjoint from the node table; a host route without Check turns the scan red | unit (table + AST scan) | `GOTOOLCHAIN=go1.26.7 go test ./internal/httpapi/handlers -count=1 -run 'TestEveryConfirmableActionDecidesOnTypedPhrase\|TestEveryConfirmedActionIsInTheTable\|TestEveryHostActionRequiresTyping' -v` | ✅ extend | ⬜ |
| 13-02-T1 | 02 | 1 | HOST-04, HMON-07 (checker resolutions 4, 5) | T-13-11, T-13-12 | an untyped fallback zone is never Healthy; the wall sentence never carries a mount path | unit (MapFS + tables) | `GOTOOLCHAIN=go1.26.7 go test ./internal/host ./internal/httpapi/handlers -count=1 -run 'TestSensorsThatFailAreNotNone\|TestZonesOnlyWithoutCPUChip\|TestAssess\|TestUnreadableIsNeverHealthy\|TestTheWallsHost' -v` | ✅ extend | ⬜ |
| 13-02-T2 | 02 | 1 | HOST-04, HMON-07 (checker resolution 4) | T-13-13 | ▲ colour from the server-rated severity | component | `cd web && npx vitest run --project jsdom src/components/charts/Sensors.test.tsx src/routes/wall.test.tsx` | ✅ extend | ⬜ |
| 13-03-T1 | 03 | 2 | HACT-01..05 (D-07, D-08, D-10) | T-13-14, T-13-15 | 428 without sudo, 403 for a reader, 403 for no / foreign / node / machine-bound token, nothing placed | HTTP integration | `GOTOOLCHAIN=go1.26.7 go test ./internal/httpapi -count=1 -run 'TestHostActionGates\|TestARouteWithNoRoleCannotBeRegistered' -v` | ❌ created by the task | ⬜ |
| 13-03-T2 | 03 | 2 | HACT-05 (D-07, D-08, D-10) | T-13-16, T-13-17, T-13-18 | typed hostname exact; host.<action> audited, typed and confirmation never archived | HTTP integration + allowlist guards | `GOTOOLCHAIN=go1.26.7 go test ./internal/httpapi -count=1 -run 'TestHostConfirmGates\|TestHostActionGates' -v` · `go test ./cmd/holzkube-managerd -run 'TestEveryAuditedActionIsInTheAllowlist\|TestEveryAllowlistedActionIsAReachableRoute'` | ❌ created by the task | ⬜ |
| 13-04-T1 | 04 | 2 | HACT-01..04, HACT-06 (D-03..D-06, R4, R5, R6) | T-13-19..T-13-25 | 4 orders run fixed argv after consumption; 18+ shapes rejected without their bytes in the journal; symlink / FIFO / stale / future refused | integration (bash + stand-in, root under unshare) | `GOTOOLCHAIN=go1.26.7 go test ./internal/host/hostaction -count=1 -run 'TestHostScript' -v` · `bash -n deploy/holzkube-manager-host.sh` | ❌ created by the task | ⬜ |
| 13-04-T2 | 04 | 2 | HACT-06 (D-05, R6) | T-13-26 | result reader refuses all but the script's format, never quotes bytes | unit (MapFS table) | `GOTOOLCHAIN=go1.26.7 go test ./internal/host/hostaction -count=1 -run 'TestReadResult' -v` | ❌ created by the task | ⬜ |
| 13-05-T1 | 05 | 2 | HACT-06 (D-02, R2, R8) | T-13-30, T-13-31 | one slot (link), 0600, no temp left; claim is one rename | unit (real temp dir) | `GOTOOLCHAIN=go1.26.7 go test ./internal/store/... -count=1 -run 'TestPlaceNew\|TestClaim\|TestNoDirectFileAccessOutsideFsstore' -v` | ❌ created by the task | ⬜ |
| 13-05-T2 | 05 | 2 | HACT-07 (D-13, R8, R9) | T-13-28, T-13-29, T-13-32 | order withdrawn after 10 s and at start; an old timer never claims a newer order | unit (manual timers) | `GOTOOLCHAIN=go1.26.7 go test ./internal/host/hostaction -count=1 -run 'TestPlace\|TestWithdraw\|TestSweep\|TestConcurrentPlace\|TestClose' -v` | ❌ created by the task | ⬜ |
| 13-05-T3 | 05 | 2 | HACT-06 (D-11, R1) | T-13-27 | the daemon starts no process (own imports + AST, 15-file floor, negative control) | guard | `GOTOOLCHAIN=go1.26.7 go test ./internal -count=1 -run 'TestTheDaemonStartsNoProcess\|TestProcessGuardRecognisesAProcessStart' -v` | ❌ created by the task | ⬜ |
| 13-06-T1 | 06 | 2 | HACT-07 (D-12) | T-13-34 | script root-owned, executable, not g/o-writable; units present; enabled | unit (MapFS) | `GOTOOLCHAIN=go1.26.7 go test ./internal/host/hostaction -count=1 -run 'TestHelper' -v` | ❌ created by the task | ⬜ |
| 13-06-T2 | 06 | 2 | HACT-07 (D-12, D-14) | T-13-35 | available false in a container or with anything missing; lists never null | unit | `GOTOOLCHAIN=go1.26.7 go test ./internal/host/... -count=1 -run 'TestReadCarriesActions\|TestContainer\|TestNoNulls' -v` | ❌ created by the task | ⬜ |
| 13-06-T3 | 06 | 2 | HACT-07 (D-12, D-14) | T-13-33, T-13-35 | helper missing / container → 409 before Check and Place; nothing in the data dir | HTTP integration + contract gates | `GOTOOLCHAIN=go1.26.7 go test ./internal/httpapi -count=1 -run 'TestHostActionsNeedTheHelper\|TestHostActionsInAContainer\|TestEveryProblemCodeIsInTheContract\|TestEveryProblemCodeIsEmitted' -v` | ❌ created by the task | ⬜ |
| 13-07-T1 | 07 | 3 | HACT-08 (D-17, D-18, D-19) | T-13-42..T-13-45 | units verify with empty output; nothing installed on the host | tool gate | `systemd-analyze verify --man=no` on copies with a stub ExecStart (rc 0 and empty output) · `go test ./internal/publicrepo/` | ❌ created by the task | ⬜ |
| 13-07-T2 | 07 | 3 | HACT-08 (D-17, D-18, D-19, R7) | T-13-38..T-13-41, T-13-44 | verify gated on output with its own negative control; guide = InstallCommands; daemon hardening named as unchanged; updater never ships the helper | guard | `GOTOOLCHAIN=go1.26.7 go test ./internal/host/hostaction -count=1 -run 'TestUnitsVerify\|TestUnitsAgree\|TestInstallCommandsMatchTheGuide\|TestGuideKeepsTheDaemonsHardening\|TestTheArchiveCarriesTheHelper\|TestTheUpdateScriptDoesNotShipTheHelper' -v` | ❌ created by the task | ⬜ |
| 13-08-T1 | 08 | 3 | HACT-01..05 (D-08, D-09, D-14, D-16) | T-13-46, T-13-48, T-13-49 | buttons off with the one reason; typed field attributes; reader never offered | component | `cd web && npx vitest run --project jsdom src/components/HostActions.test.tsx src/routes/host.test.tsx` · `npm --prefix web run typecheck` | ✅ extend | ⬜ |
| 13-08-T2 | 08 | 3 | HACT-01..04 (D-05, D-13, D-15) | T-13-47 | every phase from server fields; waiting replaces stale, never both | component | `cd web && npx vitest run --project jsdom src/components/HostActions.test.tsx src/routes/host.test.tsx` | ✅ extend | ⬜ |
| 13-08-T3 | 08 | 3 | HACT-07 (D-12) | T-13-50 | helper notice lists only what is missing; 390-px grid ≥ 44 px, no overflow | component + browser (390 px) | `cd web && npx vitest run --project jsdom src/routes/host.test.tsx` · `npm --prefix web run test:browser -- src/routes/host.browser.test.tsx` | ✅ extend | ⬜ |
| 13-09-T1 | 09 | 4 | HACT-01..08 (README rule) | T-13-51, T-13-54 | fixture commands = InstallCommands; documentation values only | fixture + contract gates | `GOTOOLCHAIN=go1.26.7 go test ./internal/host/hostaction ./internal/httpapi ./internal/config ./internal/publicrepo -count=1` · `cd web && npx vitest run --project jsdom src/fixtures.test.ts` | ❌ TestTheFixtureShowsTheRealInstallCommands created by the task | ⬜ |
| 13-09-T2 | 09 | 4 | HACT-01..08 | T-13-51..T-13-53 | production service untouched (ActiveEnterTimestamp, NRestarts); helper not installed | full gate on the Pi | `./bin/task ci` · amd64 and darwin builds · `go test ./internal/publicrepo/` · `test ! -e /usr/local/sbin/holzkube-manager-host` | ✅ | ⬜ |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

### Fault injections (each reinstated, run, seen red with the exit code read from the command itself, restored; recorded in the plan's SUMMARY)

| # (RESEARCH) | Plan-Task | Fault reinstated | Expected red |
|--------------|-----------|------------------|--------------|
| F1 | 13-03-T1 | `Destructive: false` on the reboot route | TestHostActionGates "sudo window" (428) |
| F2 | 13-03-T2 | the confirm route skips the typed comparison | TestHostConfirmGates wrong-hostname rows |
| F3 | 13-03-T1 | `MinRole: RoleReader` on the action routes | TestHostActionGates "reader" |
| F4 | 13-03-T2 | `Action: ""` on the poweroff route | TestHostActionGates audit subtest (route still 202) |
| F5 | 13-03-T1 | the action handler skips `Confirmer.Check` | TestHostActionGates "no confirmation" |
| F6 | 13-03-T1, 13-01-T2 | intent target not `@host`; a host action in `typedPhrase`; a host action set to false | "host token bound to a machine"; TestEveryHostActionRequiresTyping |
| F7 | 13-05-T3 | `exec.Command("systemctl", "reboot")` in internal/host | TestTheDaemonStartsNoProcess, both halves |
| F8 | 13-05-T3 | `syscall.ForkExec` in internal/host, no os/exec import | TestTheDaemonStartsNoProcess AST half only |
| F9 | 13-04-T1 | the script executes without the pattern | TestHostScriptAsRoot "halt" |
| F10 | 13-04-T1 | `dd` without nofollow and the `-L` classification removed | "symlink to a valid order" |
| F11 | 13-04-T1 | `dd` without nonblock and the not-regular classification removed | "FIFO" (5-s context) |
| F12 | 13-04-T1 | `rm` after the systemctl call | valid cases: stand-in saw the order |
| F13 | 13-04-T1 | age window removed | "stale", "future" |
| F14 | 13-06-T1, 13-06-T3 | wants check removed; the route's helper check removed | TestHelper "not enabled"; TestHostActionsNeedTheHelper "no order placed" |
| F15 | 13-05-T1 | `os.Rename` in place of `os.Link` in PlaceNew | TestPlaceNew second placement |
| F16 | 13-05-T2 | pickup timer not armed | TestWithdraw "not picked up" |
| F17 | 13-07-T2 | `ProtectHome=true` misspelt in the shipped service | TestUnitsVerify (output non-empty) |
| F18 | 13-07-T2 | one character changed in HOST-HELPER.md's install block | TestInstallCommandsMatchTheGuide |
| F19 | 13-07-T2 | the update script extracts the helper | TestTheUpdateScriptDoesNotShipTheHelper |
| F20 | 13-06-T3 (via contract gate) | a host code's contract row removed | TestEveryProblemCodeIsInTheContract |
| F21 | 13-08-T1 | the role check dropped from the reasons | HostActions.test.tsx "reader" |
| — | 13-01-T2 | `Confirmer.Check` removed from the host handler | TestEveryConfirmedActionIsInTheTable host call-site count |
| — | 13-02-T1 | untyped-zone append dropped; `public = sentence` restored | TestSensorsThatFailAreNotNone new rows; Public expectations |
| — | 13-02-T2 | the fixed danger colour on the ▲ | Sensors.test.tsx warn-level case |
| — | 13-04-T2 | `-` accepted with started; both size checks removed | TestReadResult rows |
| — | 13-06-T1, T2, T3 | owner check removed; Available without the container flag; the confirm route's container check removed | TestHelper uid-1000 row; TestReadCarriesActions container row; TestHostActionsInAContainer |
| — | 13-07-T2 | `RemainAfterExit=yes` in the service | TestUnitsAgree |
| — | 13-08-T2, T3 | waiting condition removed; `max-md:grid-cols-2` removed | host.test.tsx waiting cases; host.browser.test.tsx 390-px grid |

---

## Wave 0 Requirements

*Existing infrastructure covers all phase requirements (user-namespace root tests as in 11-02). No framework is installed; every test file below is created inside the task that needs it (tdd tasks write the test first), so no separate Wave 0 is needed:*

- [ ] `internal/httpapi/hostactionsapi_test.go` (13-01), `hostgatesapi_test.go` (13-03), `hosthelperapi_test.go` (13-06)
- [ ] `internal/host/hostaction/script_test.go`, `result_test.go` (13-04); `hostaction_test.go` (13-05); `helper_test.go` (13-06); `units_test.go` (13-07)
- [ ] `internal/store/fsstore/place_test.go`, `internal/processguard_test.go` (13-05); `internal/host/actions_test.go` (13-06)
- [ ] `web/src/components/HostActions.test.tsx` (13-01, widened in 13-08)

*Where a check cannot run here: the root cases skip visibly (never silently) without an unprivileged user namespace; `TestUnitsVerify` fails on Linux without systemd-analyze unless `HOLZKUBE_MANAGER_NO_SYSTEMD_ANALYZE=1`, and then says "SKIPPED, not verified"; the race detector is CI's alone (no -race on the Pi).*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Installing the helper and running a real reboot / shutdown / service restart / update on the Pi | HACT-01..04, HACT-06 | root on the production host is the operator's call, every time (CLAUDE.md) | follow deploy/HOST-HELPER.md, then trigger each action from /host |
| `systemctl reboot`/`poweroff` from inside the helper's sandbox (RESEARCH A1, A3) and PID 1 watching a file in the daemon's 0700 directory (A2) | HACT-01, HACT-02, HACT-06 | only provable with the system manager and a real reboot; the Go tests use a stand-in and the optional probe the user manager | HOST-HELPER.md's probe (`systemctl start holzkube-manager-host.service`, then "Check for updates and install") proves the path unit and the systemctl → PID 1 path; a first real reboot proves logind |
| A /host page that did not place the reboot order shows the waiting notice (UI-SPEC backstop) | HACT-01 | a two-session run across a real reboot | a component test covers the condition (13-08-T2); confirm once with two browsers during the first real reboot |

*If none: "All phase behaviors have automated verification."*

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies (every task carries runnable commands with `<fails_when>`)
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references (each test file is created by the task that needs it)
- [x] No watch-mode flags (`vitest run`, `go test -count=1`)
- [ ] Feedback latency < 13s (the quick command stays within it; the root script matrix and the browser project take longer and run per task, not per commit)
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** {pending / approved YYYY-MM-DD}
