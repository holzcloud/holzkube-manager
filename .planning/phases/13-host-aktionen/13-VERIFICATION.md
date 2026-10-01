---
phase: 13-host-aktionen
verified: 2026-10-01T13:12:51Z
status: human_needed
score: 5/5 must-haves verified
covered_files:
  - ".github/workflows/ci.yml"
  - ".goreleaser.yaml"
  - ".planning/phases/13-host-aktionen/13-01-PLAN.md"
  - ".planning/phases/13-host-aktionen/13-01-SUMMARY.md"
  - ".planning/phases/13-host-aktionen/13-02-PLAN.md"
  - ".planning/phases/13-host-aktionen/13-02-SUMMARY.md"
  - ".planning/phases/13-host-aktionen/13-03-PLAN.md"
  - ".planning/phases/13-host-aktionen/13-03-SUMMARY.md"
  - ".planning/phases/13-host-aktionen/13-04-PLAN.md"
  - ".planning/phases/13-host-aktionen/13-04-SUMMARY.md"
  - ".planning/phases/13-host-aktionen/13-05-PLAN.md"
  - ".planning/phases/13-host-aktionen/13-05-SUMMARY.md"
  - ".planning/phases/13-host-aktionen/13-06-PLAN.md"
  - ".planning/phases/13-host-aktionen/13-06-SUMMARY.md"
  - ".planning/phases/13-host-aktionen/13-07-PLAN.md"
  - ".planning/phases/13-host-aktionen/13-07-SUMMARY.md"
  - ".planning/phases/13-host-aktionen/13-08-PLAN.md"
  - ".planning/phases/13-host-aktionen/13-08-SUMMARY.md"
  - ".planning/phases/13-host-aktionen/13-09-PLAN.md"
  - ".planning/phases/13-host-aktionen/13-09-SUMMARY.md"
  - ".planning/phases/13-host-aktionen/13-10-PLAN.md"
  - ".planning/phases/13-host-aktionen/13-10-SUMMARY.md"
  - ".planning/phases/13-host-aktionen/13-11-PLAN.md"
  - ".planning/phases/13-host-aktionen/13-11-SUMMARY.md"
  - ".planning/phases/13-host-aktionen/13-12-PLAN.md"
  - ".planning/phases/13-host-aktionen/13-12-SUMMARY.md"
  - ".planning/phases/13-host-aktionen/13-13-PLAN.md"
  - ".planning/phases/13-host-aktionen/13-13-SUMMARY.md"
  - ".planning/phases/13-host-aktionen/13-14-PLAN.md"
  - ".planning/phases/13-host-aktionen/13-14-SUMMARY.md"
  - ".planning/phases/13-host-aktionen/13-15-PLAN.md"
  - ".planning/phases/13-host-aktionen/13-15-SUMMARY.md"
  - "README.md"
  - "cmd/holzkube-managerd/budget_test.go"
  - "cmd/holzkube-managerd/main.go"
  - "deploy/HOST-HELPER.md"
  - "deploy/holzkube-manager-host.path"
  - "deploy/holzkube-manager-host.service"
  - "deploy/holzkube-manager-host.sh"
  - "deploy/holzkube-manager-update-check.service"
  - "docs/api-contract.md"
  - "docs/guide.md"
  - "docs/screenshots/host.png"
  - "internal/audit/redact.go"
  - "internal/host/actions_test.go"
  - "internal/host/collector.go"
  - "internal/host/collector_test.go"
  - "internal/host/health.go"
  - "internal/host/health_test.go"
  - "internal/host/host.go"
  - "internal/host/hostaction/helper.go"
  - "internal/host/hostaction/helper_owner_other.go"
  - "internal/host/hostaction/helper_owner_unix.go"
  - "internal/host/hostaction/helper_test.go"
  - "internal/host/hostaction/hostaction.go"
  - "internal/host/hostaction/hostaction_test.go"
  - "internal/host/hostaction/result.go"
  - "internal/host/hostaction/result_test.go"
  - "internal/host/hostaction/script_test.go"
  - "internal/host/hostaction/testdata/holzkube-manager-host-before-check.sh"
  - "internal/host/hostaction/units_test.go"
  - "internal/host/sensors.go"
  - "internal/host/sensors_test.go"
  - "internal/host/updatestatus/script_test.go"
  - "internal/httpapi/endtoend_test.go"
  - "internal/httpapi/handlers/confirm_test.go"
  - "internal/httpapi/handlers/host.go"
  - "internal/httpapi/handlers/wall_host_test.go"
  - "internal/httpapi/hostactionsapi_test.go"
  - "internal/httpapi/hostgatesapi_test.go"
  - "internal/httpapi/hosthelperapi_test.go"
  - "internal/httpapi/problem.go"
  - "internal/httpapi/router.go"
  - "internal/jobs/confirm.go"
  - "internal/jobs/confirm_test.go"
  - "internal/processguard_test.go"
  - "internal/store/fsstore/atomic.go"
  - "internal/store/fsstore/place_test.go"
  - "web/fixtures/demo.json"
  - "web/fixtures/host-helper-installed.json"
  - "web/scripts/readme-images.mjs"
  - "web/src/api.nulls.test.ts"
  - "web/src/api.ts"
  - "web/src/components/HostActions.test.tsx"
  - "web/src/components/HostActions.tsx"
  - "web/src/components/charts/Sensors.test.tsx"
  - "web/src/components/charts/Sensors.tsx"
  - "web/src/fixtures.test.ts"
  - "web/src/routes/host.browser.test.tsx"
  - "web/src/routes/host.test.tsx"
  - "web/src/routes/host.tsx"
  - "web/src/routes/wall.test.tsx"
covered_digest: "v2:sha256:fa2b31758f700765db3e4ae5427c972fe47516572750e845bb92b40249c65de1"
behavior_unverified: 0
overrides_applied: 0
re_verification:
  previous_status: human_needed
  previous_score: 5/5
  previous_head: 591af39
  gaps_closed:
    - "HACT-04 wording (previous human item 4): the operator decided on 2026-09-30 for a fifth, check-only action; ROADMAP criteria 1 and 3 name five actions (48db458), and 13-12..13-15 built it"
  gaps_remaining: []
  regressions: []
advisory:
  - finding: "A check and the hourly holzkube-manager-update.service can run the update script at the same time; the script has no lock and status.json is last-writer-wins (13-REVIEW-2 IN-02, not applied)"
    category: other
    reason: "A fix touches the update script, which D-19 keeps unchanged, or adds ordering to the check unit, which the allow-list guard now forbids; an operator decision"
    evidence_status: "none provided"
  - finding: "The check unit's sandbox could be tighter (SystemCallFilter=@system-service, ProtectProc=invisible, ProcSubset=pid, PrivateIPC, IPAddressDeny for the LAN) (13-REVIEW-2 IN-03, not applied)"
    category: security
    reason: "Hardening beyond what SC5 asks; would need systemd-analyze security and the allow-list extended"
    evidence_status: "none provided"
  - finding: "Outdated() does not check /usr/local/sbin/holzkube-manager-update; on a host with the helper but without the reference update script the check button is on and every check fails (13-REVIEW-2 IN-04, not applied; HOST-HELPER.md 'What it needs' states the requirement)"
    category: other
    reason: "Documented prerequisite, not detected; the failed check now names the right journal (WR-02 fixed)"
    evidence_status: "none provided"
human_verification:
  - test: "Install the helper and the check unit per deploy/HOST-HELPER.md on the Pi (operator's call), run the guide's probe: start holzkube-manager-host.service with no order, check the path unit is 'active (waiting)', then press 'Check for updates' on /host, then 'Check for updates and install'."
    expected: "Dry start: journal 'kein Auftrag', exit 0. Check for updates: the order is picked up once; /var/lib/holzkube-manager-host/last reads '<id> check-update started', then '<id> check-update done'; while it runs all five buttons are off with 'An update check is running; wait for it to finish.'; the status box ends with the newest and the installed version; nothing is installed. Check for updates and install: last reads '<id> update started'; the path unit does not loop."
    why_human: "Needs root-installed units under the real system manager; every test here runs the script under unshare with a stand-in systemctl. Installing root code on the production host is the operator's call (CLAUDE.md)."
  - test: "V-26: run the check unit under its real sandbox: sudo systemctl start holzkube-manager-update-check.service; echo exit=$?; systemctl show holzkube-manager-update-check.service -p Result -p ExecMainStatus; journalctl -u holzkube-manager-update-check.service -b -n 50; sudo -u holzkube-manager cat /var/lib/holzkube-manager-update/status.json"
    expected: "exit=0, Result=success, ExecMainStatus=0; no 'Permission denied', 'Operation not permitted', 'Could not resolve host' or 'Address family not supported' in the journal; status.json has a checked_at from just now and the daemon user can read it."
    why_human: "systemd-analyze verify and the allow-list guard prove the unit is well-formed, not that curl, python3 and bash work under RestrictAddressFamilies=, MemoryDenyWriteExecute= and ProtectSystem=strict with its StateDirectory=. Needs an installed unit and root."
  - test: "With the helper installed, restart the host from /host with two sessions open, only one of which placed the order. Separately, once: start a Check for updates and reboot (or lose power) before it ends."
    expected: "Both pages show 'Waiting for holzkube-manager to come back' (not the stale notice) while the host is down, then 'back'; the reboot order does not run a second time after boot (last shows one started line for that id). A check cut off by the reboot shows 'started, but the host restarted before the check ended ...' and actions.busy is false after boot, so the buttons are on again."
    why_human: "Only a real reboot shows the connection loss, PathExists at boot, the 60-s age window against the restored clock and the boot clause of CheckRunning against CLOCK_BOOTTIME; tests hold the derivations from readings only."
  - test: "With the helper installed, 'Restart service' and 'Shut down host' once each (operator's choice when)."
    expected: "restart-service: the daemon comes back and the page shows 'back'; poweroff: the Pi powers off via the root oneshot under NoNewPrivileges and RestrictAddressFamilies=AF_UNIX."
    why_human: "systemctl reboot/poweroff/restart from inside the helper's sandbox is not exercised by any test; the stand-in systemctl only records argv."
  - test: "D-11 hand test with five actions: on a phone at its native width, against a development daemon (never production) over TLS on the LAN, with the helper installed: open /host without zooming, read the five buttons, tap Check for updates and Restart host, read each dialog, type the host name with the phone keyboard, see confirm enable, tap Keep running."
    expected: "Five buttons readable and tappable without zoom ('Check for updates' spans the first row, the other four two by two), the dialog fits, typing works, Keep running closes it; nothing submitted."
    why_human: "Needs a real phone, TLS and an installed helper; the layout audit measures 44 px at 390 px in Chromium, not a thumb on a device."
---

# Phase 13: Host-Aktionen über einen root-eigenen Helfer -- Verification Report

**Phase Goal:** Der Betreiber kann den Host neu starten, herunterfahren, den Dienst holzkube-manager neu starten und „jetzt nach Updates suchen" auslösen -- ohne dass der Daemon Root, D-Bus oder eine Capability bekommt. Er legt einen Auftrag im Datenverzeichnis ab; eine root-eigene Path-Unit holt ihn ab und ein festes Skript führt ihn aus einer festen Liste aus.
**Verified:** 2026-10-01T13:12:51Z at 08e5443
**Status:** human_needed. All five success criteria hold in the code and in tests run here. What remains needs the helper installed, a real reboot, or a phone.
**Re-verification:** Yes. The previous report was written at 591af39. Since then: plans 13-12..13-15 (the operator's HACT-04 decision for a fifth, check-only action), 13-REVIEW-2, and three fix rounds (13-REVIEW-2-FIX rounds 1-3).

**Where this ran:** the operator's Raspberry Pi 5 (aarch64), with Go 1.27.1 from `~/.local/go`, Node 22 through nvm, and Chromium from the browser project. No `-race`: ThreadSanitizer refuses this kernel, so the race verdict is CI's. `unshare --user --map-root-user id -u` printed 0, so the root-namespace script tests ran (33 `TestHostScriptAsRoot` subtests PASS, 0 SKIP). Nothing was installed: `/usr/local/sbin/holzkube-manager-host`, `/etc/systemd/system/holzkube-manager-host.path` and `/etc/systemd/system/holzkube-manager-update-check.service` are absent. `holzkube-manager.service` read `ActiveEnterTimestamp=Tue 2026-09-29 20:25:38 CEST, NRestarts=0` and was not touched. Its data directory was not read. Every fault injection ran in a scratch copy made with `git archive HEAD`. The checkout's `git status --porcelain` was empty before this report was written.

## Re-verification scope

The ROADMAP's criteria 1 and 3 changed (48db458): they now name five actions, and "nach Updates suchen" is split into "search and install" and "search only". So every criterion was re-checked against the five-action code. A regression check alone would not have been enough. New and changed artifacts were checked in full: the check arm in the helper script, `holzkube-manager-update-check.service`, `KnownOrders`/`Outdated`, the busy rule (`CheckRunning`, `Box.Busy`, `CheckHeld`), the 409 codes `host-helper-outdated` and `host-helper-busy`, and the page's fifth button, dialog, status phases and notices.

## Goal Achievement

### Observable Truths (ROADMAP success criteria)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Five actions on /host: Restart host, Shut down host, Restart service, Check for updates and install, Check for updates (only looks). Each needs a sudo window and the typed hostname, is refused below operator, and is audited. Each of the four locks was removed singly and its test seen red | VERIFIED | `hostaction.Actions()` returns five (`hostaction.go:92-93`). `HostRoutes` builds one route per action with `MinRole: RoleOperator`, `Destructive: true` and `Action: host.<a>` (`handlers/host.go:176-186`). `hostTypedPhrase` is true for all five (`:148-154`). `redact.go:242` lists `host.check-update`. The gate tests loop over `Actions()` (`hostgatesapi_test.go:183-421`). host.png, viewed by this verifier, shows the five buttons in that order. **Red by this verifier:** with `hostTypedPhrase[CheckUpdate]` set to false, `TestEveryHostActionRequiresTyping` gave rc 1 ("a host action without the typed hostname") and `TestHostConfirmGates` gave rc 1 ("host.check-update typing \"example-hostx\": 200 ... want 400"). Restored, `cmp` 0, both rc 0. The other three locks' red runs for the check are 13-12's claims and were not repeated. |
| 2 | The daemon never executes a host action itself; a guard was seen red against a smuggled process call | VERIFIED | `TestTheDaemonStartsNoProcess` PASS (`./internal/`). The check runs in the helper (`systemctl start holzkube-manager-update-check.service`), not in the daemon. The daemon's new code reads files only: the helper's marker line, bounded (`helper.go:224-296`), and `last` (`result.go`). `main.go`'s change wires `SinceBoot` from `host.OS().BootTime` and starts no process. |
| 3 | The helper script, run against a test data dir and a stand-in systemctl, carries out exactly the known orders (five since 13-12); an unknown or malformed order is discarded and logged; the test was seen red against a script that runs it | VERIFIED | The anchored pattern and the case statement both name exactly the five words (`holzkube-manager-host.sh:201`, `:220-227`). One marker line names the same five, and `TestTheHelperNamesItsOrders` holds marker = pattern = case = `Actions()`. Root matrix: 33 subtests PASS, 0 SKIP. It covers valid runs of all five, systemctl failing for each, 19 malformed shapes (three of them check-update variants), symlink, FIFO, stale, future, directory and symlinked state dir. `TestAnOlderHelperRefusesTheCheck` PASS: the frozen 8b64a06 helper rejects `check-update <id>` and still runs `update`. **Red by this verifier (as root in a user namespace):** (a) the pattern widened to `^([a-z-]+)` and `*)` changed to run `systemctl start "$action"`: rc 1, `rejects_foreign_word` "exit = 0, want 2" and "systemctl was called, want never". (b) the `record ... done` line removed from the check arm: rc 1, `valid_check-update` "last = {... started}, want {... done}". Restored, `cmp` 0, rc 0 with 34 PASS. |
| 4 | With the helper not installed, the actions are locked, the page names what from deploy/ goes where, and no order is left in the data dir | VERIFIED | The route refusals run in order: container, missing, outdated (check only), busy. All of them come before the body, the token and `Place` (`handlers/host.go:251-291` confirm route, `:336-356` action route). `TestHostActionsNeedTheHelper`, `TestHostActionsInAContainer` and `TestHostCheckNeedsANewerHelper` PASS. The empty machine still lists `[script, path-unit]`. The install commands now install the check unit as well, and the page, HOST-HELPER.md between its markers, and demo.json carry the same bytes (`TestInstallCommandsMatchTheGuide` PASS). The page's notice says "knows exactly five orders ... the five buttons above stay off" (host.png viewed). An older helper gets its own notice with what to reinstall, and only the check's button is off. |
| 5 | deploy/ has the path unit, the service unit, the script and an install guide; `systemd-analyze verify` accepts the units; the guide asks for no less hardening on the daemon's unit | VERIFIED | deploy/ now also has `holzkube-manager-update-check.service`, and `.goreleaser.yaml:88` ships it. `TestUnitsVerify` PASS: it ran systemd-analyze over the three units with no output. `TestUnitsAgree` and `TestTheCheckUnitRunsOnlyTheCheck` PASS. After round 2 these are allow-lists: exactly one `ExecStart=/usr/local/sbin/holzkube-manager-update --check`, the four address families exactly, no other Exec*, the worst case start + 4 x stop at least 15 s under the helper's 3 min, and the helper's `TimeoutStartSec` equal to `HelperServiceLimit`. `TestGuideKeepsTheDaemonsHardening` PASS. `deploy/holzkube-manager-update.sh` is unchanged since 9f4104b (D-19). |

**Score:** 5/5 truths verified (0 present-but-behavior-unverified)

### Plan-level truths added since 591af39 (13-12..13-15 and the fix rounds)

| Truth | Status | Evidence |
|---|---|---|
| A check places exactly `check-update <16 hex>\n`; the helper consumes it before acting, calls `start holzkube-manager-update-check.service` blocking, and records `started` then `done`/`failed` | VERIFIED | `TestHostCheckRoundTrip` PASS (gates, order, helper as root, status back in GET /api/v1/host). `TestHostScriptAsRoot/valid_check-update` and the "systemctl fails" rows PASS. Red (b) above. |
| `--check` installs nothing (as root) | VERIFIED | `TestUpdateScriptAsRoot/check_installs_nothing,_as_root` PASS. |
| An older helper is recognised from its marker line; check-update is refused with 409 `conflict.host-helper-outdated` and no order, while the other four still work | VERIFIED | `TestHostCheckNeedsANewerHelper` PASS. `KnownOrders` reads only a root-owned script that no one else can write, capped at Max+1. |
| While the helper waits for a check, every order is refused with 409 `conflict.host-helper-busy` before token and placement; a refused client keeps its token; the page's buttons follow `actions.busy` | VERIFIED | This is a behaviour-dependent truth (a state transition). Named tests PASS: `TestHostActionsWaitForARunningCheck`, `TestABusyRefusalKeepsTheToken`, `TestAHelperBusyAtPlacementIsRefusedAsBusy`, `TestCheckRunning`, `TestCheckHeld`. **Red by this verifier:** (c) the action route's `Busy()` refusal removed: rc 1. `TestABusyRefusalKeepsTheToken` failed ("same token after the check: 403, want 202 -- the busy refusal spent it"), and so did `TestAHelperBusyAtPlacementIsRefusedAsBusy`. `TestHostActionsWaitForARunningCheck` alone stayed green, because `Place`'s own guard still refused with 409. (d) `Place`'s `busyLocked()` guard removed as well: rc 1, all seven "refused" rows of `TestHostActionsWaitForARunningCheck` ("202, want 409 conflict.host-helper-busy", "a refused request placed an order"). (e) Page: the `host.actions.busy` clause in `disabledReason` removed: vitest rc 1, 2 failed ("while the server says the helper is busy: all five off"). All restored, `cmp` 0 against the checkout, rc 0. |
| A failed check names the check unit's journal; a check is finished only by the helper's `done`/`failed`, not by an hourly run's status; a check cut off by a reboot says so | VERIFIED (derivation) + human | Covered by HostActions.test.tsx rows (181 PASS). The real reboot is a human item. |
| At 390 px 'Check for updates' spans the first row and the other four sit two by two, every control at least 44 px; at 1200 px all five are one line | VERIFIED | `host.browser.test.tsx` 10/10 PASS here. The CI log's layout audit ran `/host · Host actions (helper installed) (5 controls)` ok, and "every control is at least 44px at 390px". |
| README, guide, HOST-HELPER.md and the contract describe five actions; host.png shows them | VERIFIED | README:51 ("check for an update without installing anything"). guide.md:243-253. api-contract.md:1443, :1704-1745. host.png viewed. |

### Required Artifacts (new or changed since 591af39)

| Artifact | Status | Details |
|---|---|---|
| `deploy/holzkube-manager-host.sh` | VERIFIED | five-word pattern, marker line, check arm without `--no-block`, `done` record; red (a), (b) |
| `deploy/holzkube-manager-update-check.service` | VERIFIED | oneshot, only `--check`, no [Install], `TimeoutStopSec=10s`, allow-list guarded, in the release archive |
| `internal/host/hostaction/{hostaction,helper,result}.go` | VERIFIED | `CheckUpdate`, `KnownOrders`/`Outdated`, `CheckRunning`/`CheckHeld`, `Box.Busy`, `ErrBusy` |
| `internal/host/hostaction/testdata/holzkube-manager-host-before-check.sh` | VERIFIED | frozen older helper; `TestAnOlderHelperRefusesTheCheck` PASS |
| `internal/httpapi/handlers/host.go`, `problem.go` | VERIFIED | refusal order container -> missing -> outdated -> busy, before token and body; red (c), (d) |
| `web/src/components/HostActions.tsx`, `routes/host.tsx`, `api.ts` | VERIFIED | fifth button, dialog, phases, `actions.busy`, older-helper notice; red (e) |
| `web/fixtures/demo.json`, `host-helper-installed.json`, `docs/screenshots/host.png` | VERIFIED | `outdated: []` in both raw fixtures (fixtures.test.ts PASS); five buttons in host.png |

### Key Link Verification

| From | To | Via | Status |
|---|---|---|---|
| HostActions.tsx | `/api/v1/host/actions/check-update` | `HOST_ACTION_PATHS` (`api.ts:3012`) | WIRED |
| handlers/host.go | hostaction.go | route loop over `Actions()`, `hostTypedPhrase[CheckUpdate]` | WIRED |
| holzkube-manager-host.sh | update-check.service | the fixed argv of the check arm | WIRED (TestUnitsAgree, marker test) |
| update-check.service | update.sh | `ExecStart=UpdateScriptPath --check` | WIRED (allow-list) |
| `Box.Busy()` | routes + `actions.busy` -> page buttons | `collector.go` -> host answer -> `disabledReason` | WIRED (red c, d, e) |
| `Box.Outdated()` | 409 outdated + page notice | `actions.outdated` | WIRED (TestHostCheckNeedsANewerHelper) |
| demo.json | host.png / README | `readme-images.mjs` | WIRED (png shows five) |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|---|---|---|---|
| Phase-13 Go packages | `go test -count=1 -v ./internal/host/... ./internal/httpapi/ ./internal/httpapi/handlers/ ./internal/ ./cmd/holzkube-managerd/ ./internal/audit/ ./internal/store/fsstore/ ./internal/jobs/ ./internal/publicrepo/` | rc 0, 11 packages ok, 1063 PASS lines, 0 SKIP, 0 FAIL | PASS |
| Web jsdom | `vitest run --project jsdom HostActions.test.tsx host.test.tsx fixtures.test.ts api.nulls.test.ts wall.test.tsx` | rc 0, 5 files, 358 tests | PASS |
| Web browser | `npm run test:browser -- src/routes/host.browser.test.tsx` | rc 0, 10/10 | PASS |
| Whole gate | `./bin/task ci` on HEAD, run by the caller just before; log read, not re-run | rc 0 as the caller reported it (the log carries no rc line). Log: lint:web, test:web (61 files, 828 tests), build, lint:go "0 issues", test (every package `ok`, no FAIL line), test:layout "Nothing out of reach ... on 27 routes and in 9 opened states", test:next | PASS (log) |

### Fault injections by this verifier (scratch copy from `git archive HEAD`; each restored, `cmp` 0 against the checkout, then green)

| # | Injection | Test | rc |
|---|---|---|---|
| a | helper script: pattern `^([a-z-]+)`, `*)` runs `systemctl start "$action"` | `TestHostScriptAsRoot` (as root, userns) | 1 (`rejects_foreign_word`) |
| b | helper script: `record "$id" "$action" done` removed | `TestHostScriptAsRoot` | 1 (`valid_check-update`) |
| c | action route: `Busy()` refusal removed | busy API tests | 1 (`TestABusyRefusalKeepsTheToken`, `TestAHelperBusyAtPlacementIsRefusedAsBusy`) |
| d | (c) plus `Place`'s `busyLocked()` guard removed | busy API tests | 1 (7 rows of `TestHostActionsWaitForARunningCheck` as well) |
| e | page: `host.actions.busy` clause in `disabledReason` removed | HostActions.test.tsx | 1 (2 failed) |
| f | `hostTypedPhrase[CheckUpdate] = false` | `TestEveryHostActionRequiresTyping`, `TestHostConfirmGates` | 1, 1 |

Injection (c) also shows what each test does. `TestHostActionsWaitForARunningCheck` sees "no order placed" and holds Place's guard. The token-keeping test is the one that holds the route's own early refusal.

### Requirements Coverage

| Requirement | Description | Status | Evidence |
|---|---|---|---|
| HACT-01 | Restart host | SATISFIED (automated) / real run human | SC1, SC3 |
| HACT-02 | Shut down host | SATISFIED (automated) / real run human | SC1, SC3 |
| HACT-03 | Restart service | SATISFIED (automated) / real run human | SC1, SC3 |
| HACT-04 | „jetzt nach Updates suchen" | SATISFIED. The operator's decision of 2026-09-30 is built: "Check for updates" only looks, and "Check for updates and install" installs | 13-12..13-15; the previous report's decision item is closed |
| HACT-05 | sudo + typed hostname + audit + operator | SATISFIED | SC1, injection f |
| HACT-06 | daemon never executes; fixed list; unknown discarded and logged | SATISFIED | SC2, SC3, injections a, b |
| HACT-07 | helper missing -> UI says so, names what to install, no order | SATISFIED | SC4; the older-helper case added |
| HACT-08 | deploy/ units, script, guide | SATISFIED | SC5 |

No orphaned requirements. Every HACT ID is claimed by a plan.

### Anti-Patterns Found

The implementation files changed by phase-13 commits since 591af39 contain no `TBD`, `FIXME`, `XXX`, `TODO` or `HACK`.

| File | Line | Pattern | Severity | Impact |
|---|---|---|---|---|
| deploy/holzkube-manager-update.sh + update-check.service | -- | no lock between a check and the hourly update (IN-02, open) | Info | a rare overlap can overwrite status.json; see advisory |
| internal/host/hostaction/helper.go | 281-296 | `Outdated` does not look for the update script (IN-04, open) | Info | documented prerequisite, not detected |
| internal/host/hostaction/hostaction.go | busyLocked | V-22 remainder: Place's lock does not close the helper's rename between `Lstat` and the link (left as info by round 3) | Info | milliseconds window; the withdrawal then names the check unit's journal (`CheckHeld`) |
| .github/workflows/ci.yml | 121-122 | userns sysctl step never executed (no CI minutes) | Info | the root matrix is proven on the Pi only |

### Human Verification Required

1. **Install the helper and the check unit, then probe them.** Follow HOST-HELPER.md: do the dry start, check that the path unit is `active (waiting)`, then press Check for updates and then Check for updates and install. Expected: `last` walks `check-update started` -> `done`. All five buttons are off while the check runs. The box ends with the newest and the installed version. Nothing is installed by the check. The update order reads `update started`, and the path unit does not loop.
2. **V-26: the check unit under its real sandbox.** Run `sudo systemctl start holzkube-manager-update-check.service`. Expected: exit 0, `Result=success`, no permission, address-family or DNS errors in its journal, and a fresh `status.json` that the daemon user can read.
3. **A real reboot with two sessions, and a check cut off by a reboot.** Expected: both pages show the waiting notice, then "back", and the reboot order does not run again. The cut-off check says "the host restarted before the check ended", and the buttons come back on after boot.
4. **Restart service and Shut down host, once each.** Both should work from inside the helper's sandbox.
5. **D-11 phone hand test with five actions.** Against a development daemon over TLS with the helper installed: no zoom, five readable buttons, both dialogs open, typing works, and Keep running closes the dialog. Nothing is submitted.

### Gaps Summary

There are no gaps. The fifth action exists end to end. It has a button and its own dialog, goes through the same four locks (one of them re-seen red here), places a fixed one-line order, and is picked up by a helper whose pattern, case arms and marker line agree with `Actions()`. That helper blocks on a check unit that runs only `--check` and is guarded by an allow-list. The helper records `done` or `failed`, and the routes and the page refuse other orders until it does. The server is the lock: removing the route's refusal and Place's guard each turns a named test red. An older helper is told apart and refused for the check alone. The previous report's open product decision (HACT-04 wording) is closed by the operator's choice and built. What remains needs root on the real host, a real reboot, or a phone, and stays with the operator. Three review info items (IN-02, IN-03, IN-04) are recorded as advisory. None of them blocks a success criterion.

---

_Verified: 2026-10-01T13:12:51Z_
_Verifier: Claude (gsd-verifier)_
