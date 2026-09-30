---
phase: 13-host-aktionen
plan: 14
subsystem: host-actions
tags: [host-actions, update-check, root-helper, contract, web, tdd]
status: complete

requires:
  - phase: 13-host-aktionen
    plan: 13
    provides: the helper's marker line (HelperOrdersMarker), UpdateCheckUnitPath, the frozen pre-13-12 helper in testdata
provides:
  - hostaction.KnownOrders (the installed script's marker line, read bounded and only once the script is root's), Outdated, Box.Outdated, MaxHelperScriptSize, OutdatedScript, OutdatedCheckUnit
  - actions.outdated in GET /api/v1/host, never null, empty while anything is missing
  - 409 conflict.host-helper-outdated for check-update on the confirm and action routes, before a token is issued or checked and before Place
  - the contract row, the outdated field and an older helper's example in docs/api-contract.md
  - the check button off with its own reason (REASON.helperOutdated, actionReason) while the other four stay on
affects: [13-15]

actuals:
  tokens: 15060
  tasks: 2
  commits: 2
plan_head_before: 3db1eb0de79f58339beb7cc4599132c1bce58ddd
plan_head_after: f97c2dfb6e696403994b94a43fc8dbff996b51a7

tech-stack:
  added: []
  patterns:
    - "A daemon learns what root code on the host can do by reading one marker line from the installed file, bounded, only after the file passed the ownership check, and never as the lock"
    - "One reason for a whole button group plus one reason for a single button, merged into the same line, with the group reason always winning"

key-files:
  created: []
  modified:
    - internal/host/hostaction/helper.go
    - internal/host/hostaction/helper_test.go
    - internal/host/host.go
    - internal/host/collector.go
    - internal/host/actions_test.go
    - internal/httpapi/handlers/host.go
    - internal/httpapi/problem.go
    - internal/httpapi/hosthelperapi_test.go
    - internal/httpapi/hostactionsapi_test.go
    - docs/api-contract.md
    - web/src/api.ts
    - web/src/components/HostActions.tsx
    - web/src/components/HostActions.test.tsx
    - web/src/api.nulls.test.ts
    - web/fixtures/demo.json

key-decisions:
  - "KnownOrders reads the script only when Detect's rule calls it installed (regular, executable, uid 0, writable by nobody else). A script somebody else may change says nothing about what root runs, so it counts as the four orders of 13-01"
  - "A script without the marker line, with two marker lines, or larger than 64 KiB counts as the helper of 13-01 (reboot, poweroff, restart-service, update). Unknown words on the line are ignored"
  - "Outdated items reuse the Missing shape ({item, path}) with the items script-outdated and check-unit. available is unchanged: the four older orders work with an older helper"
  - "The confirm route refuses host.check-update once the body names it and before the hostname is compared. The action route refuses after the missing check and before the body and token. Order: container, missing, outdated"

requirements-completed: []

metrics:
  duration: 20min
  completed: 2026-09-30
---

# Phase 13 Plan 14: An older helper is never offered the check Summary

**The daemon now reads the installed helper's marker line (bounded, and only after the root-ownership check) and checks for the check unit. It reports what is missing as `actions.outdated`. Both host routes refuse `check-update` with 409 `conflict.host-helper-outdated` before a token is issued or checked and before anything is placed. The page turns off only Check for updates, with its own sentence, and the other four buttons stay on.**

## Where it ran

The operator's Raspberry Pi 5 (aarch64), Go from `~/.local/go`, Node through nvm, and Chromium for the one browser test. No `-race`: ThreadSanitizer refuses this kernel, so the race verdict belongs to CI.

- `holzkube-manager.service` at the start of Task 1: `ActiveEnterTimestamp=Tue 2026-09-29 20:25:38 CEST`, `NRestarts=0`
- after Task 2: `ActiveEnterTimestamp=Tue 2026-09-29 20:25:38 CEST`, `NRestarts=0`. Identical.
- Nothing was installed. `test ! -e` on `/usr/local/sbin/holzkube-manager-host`, `/etc/systemd/system/holzkube-manager-host.path` and `/etc/systemd/system/holzkube-manager-update-check.service` exited 0. Nothing was pushed, tagged or released.

## Red runs (exit codes read from the command itself)

For each injection: save the file, make one edit, run, restore from the saved copy, `cmp` (exit 0 every time), then one green run (exit 0 every time) before the next.

| # | What | Test | Exit | Failing line |
|---|---|---|---|---|
| RED 1 | Task 1's tests against 13-13's tree | `go test ./internal/host/... ./internal/httpapi/ -run 'TestKnownOrders\|TestOutdated\|TestReadCarriesActions\|TestUnsupportedPlatformOutdated\|TestHostCheckNeedsANewerHelper'` | 1 | `helper_test.go:263:76: undefined: MaxHelperScriptSize`, `undefined: KnownOrders`, `a.Outdated undefined (type Actions has no field or method Outdated)`, `undefined: hostaction.OutdatedScript` (compile) |
| (a) | KnownOrders returns Actions() whatever the marker says | TestKnownOrders, TestHostCheckNeedsANewerHelper | 1 | `KnownOrders = [reboot poweroff restart-service update check-update], want [reboot poweroff restart-service update]` (frozen, two markers, too large, uid 1000 …); `POST /api/v1/host/actions/check-update with a valid host token: 202, want 409 conflict.host-helper-outdated`; `the data directory holds host-order: a refused request placed an order` |
| (b) | Outdated refusal removed from the action route only | TestHostCheckNeedsANewerHelper | 1 | `POST /api/v1/host/actions/check-update with a valid host token: 202, want 409 …`; `after /api/v1/host/actions/check-update the data directory holds host-order` |
| (c) | Removed from the confirm route only | TestHostCheckNeedsANewerHelper | 1 | `POST /api/v1/host/confirm for host.check-update: 200, want 409 …`; `… handed out a token` |
| (d) | The contract row removed (`grep -c` then 0) | TestEveryProblemCodeIsInTheContract | 1 | `contract_codes_test.go:50: these problem codes are not mentioned anywhere in docs/api-contract.md: CodeHostHelperOutdated (conflict.host-helper-outdated)` |
| (e) | The collector asks Outdated even while Missing is not empty | TestReadCarriesActions | 1 | `nothing_installed: actions.outdated = [{script-outdated …} {check-unit …}], want []`; `the_helper_before_the_check,_not_enabled: actions.outdated = [{script-outdated …}], want []` |
| (g) | (beyond the plan) KnownOrders' ownership gate removed | TestKnownOrders | 1 | `the_shipped_script_owned_by_uid_1000: KnownOrders = [… check-update], want [reboot poweroff restart-service update]` |
| RED 2 | Task 2's rows against Task 1's tree | `npx vitest run --project jsdom src/components/HostActions.test.tsx src/api.nulls.test.ts` | 1 | 5 failed: the two older-helper rows, the 13-13-daemon parse row, the null/absent `outdated` row, the check-dialog WR-01 row |
| (f) | The check button ignores outdated (`disabled={reason !== null}`) | HostActions.test.tsx | 1 | 3 failed: `expected [ false, false, false, false, false ] to deeply equal [ true, false, false, false, false ]`, and the check-dialog focus row |

## Green

- Task 1 `<verify>` (the plan's -run list plus TestUnsupportedPlatformOutdated, `-v`): exit 0, 74 PASS lines, no SKIP or FAIL. It includes `--- PASS: TestHostCheckRoundTrip (5.00s)` (not skipped) and `--- PASS: TestHostCheckNeedsANewerHelper`.
- `go test ./internal/... ./cmd/... -count=1`: exit 0, 41 `ok`, no FAIL (publicrepo included). `GOOS=darwin GOARCH=arm64 go build ./...`: exit 0. `./bin/task lint:go`: exit 0, 0 issues.
- Task 2 `<verify>`: vitest on HostActions, api.nulls, fixtures and host.test exited 0 (4 files, 279 tests). Web lint exited 0 (the 2 pre-existing warnings). Typecheck exited 0. The full jsdom suite, run alone, exited 0 (56 files, 755 tests). `test:browser src/routes/host.browser.test.tsx` exited 0 (9 tests).
- Acceptance greps: `conflict.host-helper-outdated` appears 1 time in the contract; `CodeHostHelperOutdated` appears 2 times in handlers/host.go; `func KnownOrders` 1; `func TestHostCheckNeedsANewerHelper` 1; `helperOutdated` 2 in HostActions.tsx.

## Task Commits

1. **Task 1: older helper recognised, actions.outdated, 409 before anything is placed**: `5acd0bb` (feat)
2. **Task 2: the check's button off with its own reason**: `f97c2df` (feat)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Security] KnownOrders checks the ownership gate itself**
- **Found during:** Task 1
- **Issue:** The plan relies on the collector asking only once Missing is empty. KnownOrders and Outdated are exported, so another caller could read a script that is not root's.
- **Fix:** KnownOrders calls `scriptInstalled` first. A script that is not root's is not read and counts as the original four. A TestKnownOrders row covers this, and injection (g) proved it goes red.
- **Commit:** 5acd0bb

**2. [Rule 3 - Blocking] internal/host/actions_test.go's helper fixture read the shipped script**
- **Issue:** `helperInstalledFS` served `#!/usr/bin/env bash\n`. With Outdated in place, the "installed" rows would read as an older helper.
- **Fix:** It now serves the shipped script's bytes and the check unit (it takes `t`), and `olderHelperFS` serves the frozen script. The file was in the plan's list; the changed signature was not.

**3. [Rule 2 - Consistency] web/fixtures/demo.json carries `"outdated": []`**
- The fixture is the answer a current daemon sends and feeds the README images. This file was not in the plan's list. Nothing visible changes. The "daemon before 13-14" test drops the key explicitly.

**4. Extra tests**
- `TestUnsupportedPlatformOutdated` (unsupported platform with a Box over an older helper gives an empty `outdated`). Also a row showing the older helper with a piece missing leaves `outdated` empty, and a web row showing the restart dialog is not given the check's reason.

### Left for later plans

- The notice that names what to reinstall, with the commands, and the "four orders" wording in HostHelperNotice/README/HOST-HELPER.md are 13-15's. The reason line points at "the note below", which 13-15 extends to name the outdated pieces. HACT-04 is **not** marked complete.

## Known Stubs

None.

## Threat Flags

None beyond the plan's register. T-13-71 is mitigated by the ownership gate, the Stat size check and a LimitReader at 64 KiB+1; no error or answer quotes the file. T-13-73 is mitigated by injections (a) to (c). T-13-74: the service properties are identical before and after.

## Self-Check: PASSED

- FOUND: internal/host/hostaction/helper.go (KnownOrders), internal/httpapi/hosthelperapi_test.go (TestHostCheckNeedsANewerHelper), web/src/components/HostActions.tsx (helperOutdated)
- FOUND: 5acd0bb, f97c2df
