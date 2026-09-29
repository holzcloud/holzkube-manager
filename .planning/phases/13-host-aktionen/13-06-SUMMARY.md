---
phase: 13-host-aktionen
plan: 06
subsystem: host-actions
tags: [host-actions, helper-detection, container, problem-codes, api-contract]
status: complete

requires:
  - phase: 13-host-aktionen
    plan: 01
    provides: hostaction.Box, the host confirm and action routes, installedHelperFS, withHostActions/withHostOver
  - phase: 13-host-aktionen
    plan: 05
    provides: the Box's final Config (Claim, PickupTimeout, AfterFunc) and the 10-s withdrawal as the second net
provides:
  - hostaction.HelperScriptPath, PathUnitPath, ServiceUnitPath, WantsLinkPath
  - hostaction.Missing (item/path), MissingScript / MissingPathUnit / MissingNotEnabled
  - hostaction.Detect(fs.FS) and (*Box).Missing() -- never nil
  - hostaction.InstallCommands (the one copy of the four install lines)
  - actions.available / actions.missing / actions.install_commands in GET /api/v1/host
  - (*host.Collector).InContainer()
  - httpapi.CodeHostHelperMissing (409 conflict.host-helper-missing), httpapi.CodeHostInContainer (409 conflict.host-in-container)
affects: [13-07, 13-08, 13-09]

actuals:
  tokens: 10000
  tasks: 3
  commits: 3
plan_head_before: b255e29554108318c80f4769fae169a830f95361
plan_head_after: b2fd924cfbd7e23c40e65b5e79a404e89d457e89

tech-stack:
  added: []
  patterns:
    - "Helper detection by the files systemd reads (no D-Bus): fs.Stat for script and units, fs.Lstat for the wants entry, owner from *syscall.Stat_t behind a unix/!unix build split"
    - "The answer's available and the routes' refusal ask the same two questions (Collector.InContainer, Box.Missing), so page and server cannot disagree"

key-files:
  created:
    - internal/host/hostaction/helper.go
    - internal/host/hostaction/helper_owner_unix.go
    - internal/host/hostaction/helper_owner_other.go
    - internal/host/hostaction/helper_test.go
    - internal/host/actions_test.go
    - internal/httpapi/hosthelperapi_test.go
  modified:
    - internal/host/host.go
    - internal/host/collector.go
    - internal/httpapi/handlers/host.go
    - internal/httpapi/problem.go
    - docs/api-contract.md

key-decisions:
  - "A path-unit item carries the path of the first of the two unit files found wanting (.path before .service); one item for the pair, because one command installs both"
  - "Container is asked before the helper on both routes: in a container the install commands would be the wrong advice"
  - "The checks are inlined in both handlers rather than shared, so each route's check can be removed (and was, for the injections) on its own"
  - "On an unsupported platform (darwin) actions.available is false and missing is still Detect's answer (the script always reads missing there: no owner can be told)"
  - "Each new problem code is mentioned exactly once in the contract (its table row), so removing the row turns TestEveryProblemCodeIsInTheContract red"

requirements-completed: []

duration: 19min
completed: 2026-09-29
---

# Phase 13 Plan 06: Helper detection, container and helper-missing refusals Summary

**The daemon tells an installed helper from a missing one by three files it can read under ProtectSystem=strict (script owned by root and writable by nobody else, both units, the paths.target.wants entry); `GET /api/v1/host` says `available`, what is `missing` and the `install_commands`, and with the helper missing or in a container both host routes answer 409 before a token is checked or a file is written.**

## Where this ran

The operator's Raspberry Pi 5 (aarch64), Go 1.26.7 via GOTOOLCHAIN, **no -race** (ThreadSanitizer refuses the Pi 5 kernel's address space). Nothing was installed. The production service was not touched: `systemctl show holzkube-manager.service -p ActiveEnterTimestamp` read `Mon 2026-09-28 18:06:00 CEST` before (02:07Z) and after (02:26Z) the plan.

## Performance

- **Duration:** ~19 min
- **Started:** 2026-09-29T02:07:14Z
- **Completed:** 2026-09-29T02:26Z
- **Tasks:** 3
- **Files:** 11 (6 created, 5 modified)

## Accomplishments

- `hostaction.Detect(fsys)`: script via `fs.Stat` -- regular, `perm&0o111 != 0`, `perm&0o022 == 0`, owner uid 0 from `ownerUID` (`*syscall.Stat_t` on unix, never known elsewhere; unknown owner = missing); path-unit when either unit file is absent or not regular; not-enabled when `fs.Lstat(WantsLinkPath)` fails (a copied file there counts as enabled). Items in the order script, path-unit, not-enabled; never nil. `(*Box).Missing()` reads through the Box's FS.
- `InstallCommands`: the four lines of 13-RESEARCH "Install block", with the comment that plan 07 holds `deploy/HOST-HELPER.md` to it byte for byte.
- `host.Actions` gains `available`, `missing` (never null) and `install_commands` (a copy, so a View cannot change the one copy); `Collector.InContainer()`; `readUnsupported` fills them too (available false).
- Both host routes, right after the nil guards: `InContainer()` -> 409 `conflict.host-in-container`, then `len(HostActions.Missing()) > 0` -> 409 `conflict.host-helper-missing`, with UI-SPEC's server sentences verbatim; in the action route both before `Confirmer.Check` and `Place`. The file comment gives the reason for the order.
- Contract: two rows in the host-actions table, the `actions` example and bullets for `available`, `missing` (items with their paths) and `install_commands`.

## Task Commits

1. **Task 1: Is the helper installed? Three files, read the way systemd reads them** - `c83b702` (feat)
2. **Task 2: The host answer says whether actions are possible, what is missing, how to install it** - `2c8f5f3` (feat)
3. **Task 3: Refused before anything is written -- helper missing, or a container** - `b2fd924` (feat)

## Fault injections (each put in, run, seen red from `go test`'s own exit code, restored from a saved copy; `cmp` confirmed each restore)

| ID | Injection | rc | Failing line |
|---|---|---|---|
| T1 (a), F14 | `Detect`: the `fs.Lstat(WantsLinkPath)` check removed | 1 | `--- FAIL: TestHelper/both_units,_not_enabled` `helper_test.go:85: Detect = [], want [{Item:not-enabled Path:/etc/systemd/system/paths.target.wants/holzkube-manager-host.path}]` (and `TestHelper/nothing_installed`) |
| T1 (b) | `scriptInstalled`: owner check replaced by `return true` | 1 | `--- FAIL: TestHelper/script_owned_by_uid_1000` `helper_test.go:85: Detect = [], want [{Item:script Path:/usr/local/sbin/holzkube-manager-host}]` (and `TestHelper/script_with_no_owner_information`) |
| T2 | `Available: len(missing) == 0` (container flag dropped) | 1 | `--- FAIL: TestReadCarriesActions/installed,_in_a_container` `actions_test.go:69: actions.available = true, want false` |
| T3 (a), F14 route half | helper check removed from the action handler only | 1 | `--- FAIL: TestHostActionsNeedTheHelper/nothing_installed` (and path_unit_not_enabled, ...) `hosthelperapi_test.go:174: POST /api/v1/host/actions/reboot with a valid host token: 202, want 409 conflict.host-helper-missing` and `hosthelperapi_test.go:174: after /api/v1/host/actions/reboot the data directory holds host-order: a refused request placed an order` (all four actions) |
| T3 (b) | container check removed from the confirm route only | 1 | `--- FAIL: TestHostActionsInAContainer` `hosthelperapi_test.go:221: POST /api/v1/host/confirm: 200, want 409 conflict.host-in-container` and `... handed out a token` |
| T3 (c), F20 | the `conflict.host-helper-missing` row deleted from docs/api-contract.md (0 mentions left) | 1 | `--- FAIL: TestEveryProblemCodeIsInTheContract` `contract_codes_test.go:50: these problem codes are not mentioned anywhere in docs/api-contract.md: CodeHostHelperMissing (conflict.host-helper-missing)` |

The action route was handed a token from the harness's own confirmer in every refused request, so the 202 under T3 (a) is what an operator with a valid confirmation would have got, and the order file it placed is what the test found.

## Verification (exit codes read from the command itself)

- `go test ./internal/host/hostaction -run TestHelper -v`: rc 0, `--- PASS: TestHelper` with 15 rows, plus `TestHelperBoxMissing`, `TestInstallCommandsAreTheGuidesBlock`.
- `go test ./internal/host/... -run 'TestReadCarriesActions|TestHelper|TestContainer|TestNoNulls|TestEveryReadingIsWellFormed' -v`: rc 0.
- `go test ./internal/httpapi -run 'TestHostActionsNeedTheHelper|TestHostActionsInAContainer|TestHostActionRoundTrip|TestEveryProblemCodeIsInTheContract|TestEveryProblemCodeIsEmitted|TestHostAPI' -v`: rc 0; `--- PASS: TestHostActionRoundTrip`, 0 SKIP lines (the root round trip ran in a user namespace).
- `go test ./internal/... ./cmd/... -count=1`: rc 0, no FAIL line.
- `./bin/task lint:go`: 0 issues. `GOOS=darwin GOARCH=arm64 go build ./...`: rc 0.
- `git diff b255e29 -- go.mod go.sum`: empty. `go test ./internal/publicrepo/`: ok.

## Deviations from Plan

None that change behaviour. Small additions:
- `TestHostActionsNeedTheHelper` has an `installed` control subtest (confirm 200, action 202, order placed) so the refusals are shown to be the helper's and not the harness's.
- `TestInstallCommandsAreTheGuidesBlock` and `TestInstallCommandsAreNotShared` hold the install commands' text and that the answer carries a copy.
- The plan's Task 3 wording "extend the file's comment" was done as a paragraph in the file's header comment ("Refused before anything else when nothing can carry them out").

## Requirements

HACT-07 is not marked complete here: this plan is its server half. The page half (the notice listing the missing items and the install block, the disabled buttons) is 13-08, and the guide's install block held to `InstallCommands` is 13-07.

## Known Stubs

None. The web schema (`api.ts`) is not strict and simply ignores the three new keys until 13-08 reads them.

## Threat Flags

None beyond the plan's threat model (T-13-33..T-13-37): the detection reads four fixed public paths; no new route, and the two new answers are 409s on existing routes.

## Next Phase Readiness

- 13-07: `hostaction.InstallCommands` is the text `deploy/HOST-HELPER.md`'s block must equal; the four path constants are what `units_test.go` should hold the units and script defaults to.
- 13-08: `actions.available`, `actions.missing[].item` (`script` / `path-unit` / `not-enabled`) and `actions.install_commands` are in the answer; the 409 codes carry UI-SPEC's server sentences.
- 13-09: the demo fixture needs the three keys (helper-missing state for the pictures).

## Self-Check: PASSED
