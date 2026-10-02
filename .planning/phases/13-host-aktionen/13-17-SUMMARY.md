---
phase: 13-host-aktionen
plan: 17
subsystem: host-actions
tags: [host-actions, update-unit, reason-line, fixtures, api-contract, guide, tdd, layout]
status: complete

requires:
  - phase: 13-host-aktionen
    plan: 16
    provides: 409 conflict.host-update-unit-missing, UpdateUnitMissing, Box.UpdateUnit, NeedsUpdateUnit, UpdateUnitInstallCommands
provides:
  - actions.update_unit and actions.update_unit_install_commands in GET /api/v1/host, in every branch, never null
  - UPDATE_UNIT_ACTIONS, REASON.updateUnit and actionReason in the routes' order (group, update script, update unit, older helper)
  - a reason line that carries every distinct button reason in button order (at most two sentences) and describes exactly those buttons
  - HostUpdateUnitNotice, after the update-script note, never in a container
  - TestThePageNeedsTheUpdateUnitWhereTheRoutesDo; the demo fixture's update-unit commands held byte for byte
  - docs/api-contract.md and docs/guide.md for the reading, the note and the order of reasons
affects: [phase-13-verification]

actuals:
  tokens: 14808   # chars/4 over git diff d01f1ac..735a768
  tasks: 3
  commits: 2      # git rev-list --count d01f1ac..HEAD at SUMMARY time; Task 3 changed no file
plan_head_before: d01f1acc3dc528d6a4dd9b93cd260527fe1bbd01
plan_head_after: 735a7680cf981bb719251d03aa937621c4bac26b

tech-stack:
  added: []
  patterns:
    - "One reason line for single-button reasons: the distinct reasons over HOST_ACTIONS in button order, joined by a space; a button is described by it exactly when it has a reason of its own"
    - "A page constant held to a server predicate by a Go test that reads the .tsx line and compares sorted lists"

key-files:
  created: []
  modified:
    - internal/host/host.go
    - internal/host/collector.go
    - internal/host/actions_test.go
    - internal/host/hostaction/units_test.go
    - web/src/api.ts
    - web/src/components/HostActions.tsx
    - web/src/components/HostActions.test.tsx
    - web/src/routes/host.tsx
    - web/src/routes/host.test.tsx
    - web/src/fixtures.test.ts
    - web/fixtures/demo.json
    - web/fixtures/host-helper-installed.json
    - docs/api-contract.md
    - docs/guide.md

key-decisions:
  - "The update unit's reason sits after the update script's and before the older helper's in actionReason, as the routes ask"
  - "Single-button reasons share one line, in button order, at most two sentences; a group reason still wins and is said once"
  - "helperInstalledFS in internal/host now carries the update unit, as the reference installation does"
  - "No picture rendered and README unchanged: no visible change in the demo, no new screen"

requirements-completed: [HACT-04, HACT-08]

coverage:
  - id: D1
    description: "GET /api/v1/host carries update_unit and update_unit_install_commands in every branch, cloned, never null"
    requirement: HACT-04
    verification:
      - kind: unit
        ref: "internal/host/actions_test.go#TestReadCarriesActions, TestInstallCommandsAreNotShared, TestUnsupportedPlatformUpdateUnit"
        status: pass
  - id: D2
    description: "With the unit missing only Check for updates and install is off; the line carries one or two reasons; group reasons win; the note shows the answer's commands"
    requirement: HACT-04
    verification:
      - kind: unit
        ref: "web/src/components/HostActions.test.tsx, web/src/routes/host.test.tsx (the update-unit notice (13-17))"
        status: pass
  - id: D3
    description: "UPDATE_UNIT_ACTIONS equals NeedsUpdateUnit; both fixtures carry both keys; the demo's commands are UpdateUnitInstallCommands"
    requirement: HACT-04
    verification:
      - kind: unit
        ref: "internal/host/hostaction/units_test.go#TestThePageNeedsTheUpdateUnitWhereTheRoutesDo, TestTheFixtureShowsTheRealInstallCommands; web/src/fixtures.test.ts"
        status: pass
  - id: D4
    description: "./bin/task ci green on the Pi; the layout dump differs from 13-15's only on /settings, explained by b850900"
    requirement: HACT-08
    verification:
      - kind: other
        ref: "~/.cache/holzkube-manager-layout/13-17-ci.log, 13-17-compare.txt"
        status: pass

duration: "2026-10-02T04:15Z to 04:56Z, about 41 min"
completed: 2026-10-02
---

# Phase 13 Plan 17: The page half of the update-unit refusal Summary

**GET /api/v1/host now carries `actions.update_unit` and `actions.update_unit_install_commands`. When the unit is missing, the Host page turns off Check for updates and install alone, gives its reason in the one reason line (two sentences when the check is off for an older helper too) and shows a note with the four commands from the answer. The page is held to `NeedsUpdateUnit` by a Go test, and fixtures, contract and guide are updated to match.**

## Where it ran

On the operator's Raspberry Pi 5 (aarch64), with Go 1.27.1 from `~/.local/go` and no `-race`, so the race verdict belongs to CI. `./bin/task ci` ran its `test` step on the pinned toolchain and `test:next` on go1.27.1. The amd64 binary was compiled, not executed. Nothing on the host was stopped, started, enabled, installed or replaced. Nothing was pushed, tagged or released, and there is no changelog entry.

On the code moving: quick task 261001-sa9 landed on main before this plan. None of this plan's code references moved. The line numbers in the plan's `read_first` were stale, for example `TestTheFixtureShowsTheRealInstallCommands` is at :1603 and not :1075, so I followed the code.

## Commits

| Task | Commit | What |
|------|--------|------|
| 1 | 13ce423 | feat(13-17): turn off Check for updates and install while the update unit is missing |
| 2 | 735a768 | feat(13-17): hold the page's update-unit button to the routes, fixtures and docs |
| 3 | none | the comparison and the gate found nothing in the two files the task names |

## Task 1: from the Box to the button

- **RED** against 13-16's tree, with exit codes read from the commands themselves:
  - `go test ./internal/host -run 'TestReadCarriesActions|TestInstallCommandsAreNotShared|TestUnsupportedPlatform'` exited 1 because the build failed: `actions_test.go:181:9: a.UpdateUnit undefined (type Actions has no field or method UpdateUnit)`.
  - `vitest run --project jsdom HostActions.test.tsx host.test.tsx` exited 1 with 8 failed and 299 passed. Failing: the four update-unit notice cases in host.test, plus "with the update unit missing: only Check for updates and install is off", "... and an older helper: one line, both reasons, in button order", "parses an answer without update_unit" and "the update dialog opened while the update unit goes missing carries the unit reason".
  - Three cases passed on RED: "script too", "a reader" and "the helper missing". They are regression guards that hold behaviour 13-16 already had. Injection (c) shows the "script too" case going red.
- **GREEN:** the Go set exited 0 (6 tests PASS, 11 subtests, 0 SKIP). Vitest exited 0 with 307 passing. `lint` exited 0 after `biome format` on the three edited files, and `typecheck` exited 0. `go test ./internal/host/... ./internal/httpapi/...` exited 0. publicrepo exited 0.
- **Tracer gate:** I re-ran the verify chain end to end after the commit work (above), and it exited 0.

Injections. Each was restored from a scratch copy and `cmp`-checked, with the exit code read from the command:

| | Fault | Exit | Failing tests |
|-|-------|------|---------------|
| (a) | readActions' UpdateUnit left `[]hostaction.Missing{}` | 1 | TestReadCarriesActions/{the_update_unit_masked,no_update_unit,nothing_installed}: `actions_test.go:185: actions.update_unit = [], want [{Item:update-unit ...}]` |
| (a2) | readUnsupported's UpdateUnitInstallCommands not cloned (extra) | 1 | TestInstallCommandsAreNotShared: `actions_test.go:275: an unsupported platform: the answer's update_unit_install_commands are hostaction.UpdateUnitInstallCommands itself, not a copy` |
| (b) | actionReason's unit clause removed | 1 | 3 failed: "with the update unit missing: only ...", "... and an older helper: one line, both reasons ...", "the update dialog opened while the update unit goes missing ..." |
| (c) | unit clause before the update script's | 1 | 1 failed: "with the update unit and the update script missing: the script reason alone". Received both sentences |
| (d) | describes() back to `actionReason(...) === line` | 1 | 1 failed: "with the update unit missing and an older helper: one line, both reasons, in button order" |
| (e) | `line = reason ?? actionReason('check-update', null, host)` | 1 | 2 failed: "with the update unit missing: only ..." (no line), "... and an older helper ..." |
| (f) | the note's render removed from host.tsx | 1 | 4 failed: the update-unit notice's "names the unit ..." and the three "comes after ..." cases |
| (f2) | `!host.container &&` dropped from the note (extra) | 1 | 1 failed: "shows nothing in a container: the container notice already says why" |

## Task 2: fixtures, the cross-check, contract and guide

- **RED:**
  - `go test ./internal/host/hostaction -run 'TestTheFixtureShowsTheRealInstallCommands|TestThePageNeedsTheUpdateUnitWhereTheRoutesDo|TestUpdateUnitInstallCommandsMatchTheGuide' -v` exited 1: `units_test.go:1663: the fixture's update_unit_install_commands differ from UpdateUnitInstallCommands.` and `units_test.go:1666: the fixture's update_unit is []hostaction.Missing(nil), want an empty list`.
  - `vitest fixtures.test.ts` exited 1: "carries `update_unit` in both /host fixtures as written, the unit there in both", `expected undefined to deeply equal []`.
  - TestThePageNeedsTheUpdateUnitWhereTheRoutesDo already passed on RED, because Task 1 had added UPDATE_UNIT_ACTIONS. Injection (i) shows it red.
- **GREEN:**
  - The Go set above exited 0 with 3 PASS and 0 SKIP.
  - Vitest exited 0 on fixtures, HostActions and host, with 349 passing.
  - `go test ./internal/httpapi -run 'TestEveryProblemCodeIsInTheContract|TestHostUpdateActionsNeedTheUpdateScript'` exited 0, and so did publicrepo.

| | Fault | Exit | First FAIL line |
|-|-------|------|-----------------|
| (g) | `"update_unit": []` removed from host-helper-installed.json | 1 | fixtures.test "carries `update_unit` in both /host fixtures as written ...": `AssertionError: expected undefined to deeply equal []` |
| (h) | `--now` changed to `--nov` in the demo's update_unit_install_commands | 1 | `units_test.go:1663: the fixture's update_unit_install_commands differ from UpdateUnitInstallCommands.` |
| (i) | UPDATE_UNIT_ACTIONS widened with 'check-update' | 1 (Go) / 1 (vitest) | `units_test.go:1750: the page's UPDATE_UNIT_ACTIONS is ["check-update" "update"], hostaction.NeedsUpdateUnit names ["update"]; the page and the routes disagree`. HostActions.test also failed 2: the unit-alone and the two-reasons cases |

**No picture rendered.** The demo's `update_unit` is `[]`, so no note shows and nothing visible on any screen changes. 13-REVIEW-2-FIX measured that a render rewrites every PNG with time noise alone, so a render would have committed noise.

**README unchanged.** There is no new screen, and 13-16's README bullet already covers the hourly update. The depth went into `docs/guide.md`: a new "Without the hourly update's unit" paragraph, "Which reason comes first" (container, helper missing, update script, update unit, older helper, busy, with the page's group reasons named), and one sentence in "Updating itself every hour".

## Task 3: the layout, the gate and the host

- **Layout:** `LAYOUT_DUMP=.../13-17-after.jsonl ./bin/task test:layout` exited 0 and wrote 967 controls. `layout-dump-compare.mjs 13-15-after.jsonl 13-17-after.jsonl` **exited 1** with `compared 973 controls, 25 differ`.
  - Every difference is on `/settings`: 12 MOVED, 7 ONLY AFTER, 6 ONLY BEFORE.
  - Outside `/settings` the two dumps have 924 entries each and are equal, every `/host` line included.
  - Cause: b850900 (quick-261001-sa9, "unlink single sign-on from Settings -> Accounts") added the "Unlink single sign-on" button (after `#23`, 145x28). That shifts every later control's index on the page by one, and the wider actions cell narrows the role column (`Role of holz` 405 to 385). This commit landed after 13-15's dump. It is not part of this plan and not an IN-02..IN-04 fix. Nothing was changed.
- **Gate:** `./bin/task ci` **exited 0**, read from the command itself (`~/.cache/holzkube-manager-layout/13-17-ci.log`, `13-17-ci.exit`).
  - Steps: npm ci; lint:web; test:web (61 files, 862 tests); build; lint:go (`0 issues.`); test (no FAIL); test:layout ("Nothing out of reach at 390px and 1280px ... 27 routes and in 9 opened states"); test:next on go1.27.1 (no FAIL).
  - No flake hit: no rerun of TestANodeSaysHowItBooted or login.test.tsx was needed.
- **Round trips:** `go test ./internal/httpapi -run 'TestHostActionRoundTrip|TestHostCheckRoundTrip|TestHostUpdateNeedsTheUpdateUnit' -v` exited 0 with 3 PASS and 0 SKIP. Both round trips ran the helper as root in a user namespace (2 log lines).
- **Builds** into the scratchpad:
  - arm64: `ELF 64-bit LSB executable, ARM aarch64, version 1 (SYSV), dynamically linked, interpreter /lib/ld-linux-aarch64.so.1, ...`
  - amd64 (GOTOOLCHAIN=go1.26.7): `ELF 64-bit LSB executable, x86-64, version 1 (SYSV), statically linked, ...`
  - arm64 is what runs here. amd64 was compiled, not executed.
- **Host, before (13-16's record) and after:**
  - `holzkube-manager.service`: `NRestarts=0`, `ActiveEnterTimestamp=Tue 2026-09-29 20:25:38 CEST`. Identical, and also identical at this plan's start and right before the gate.
  - `holzkube-manager-update.timer`: `ActiveState=active`, `UnitFileState=enabled`. Identical.
  - `/etc/systemd/system/holzkube-manager-update.service` and `.timer` are `cmp`-identical to `~/.cache/holzkube-manager-13-16/`.
  - `/usr/local/sbin/holzkube-manager-host` and `/etc/systemd/system/holzkube-manager-update-check.service` are absent.
  - The host check exited 0. `git status --porcelain` was empty.

## HACT-04 and HACT-08

Both hold with the hourly update shipped (operator decision 2026-10-01). The routes refuse Check for updates and install without its unit (13-16), and the page no longer offers it and says what to install (this plan).

## Human needed (the operator's call, not this executor's)

- Installing the shipped units on the Pi in place of the hand-made ones, then watching a first real install and restart go through them as root. That measurement is what would let CapabilityBoundingSet=, SystemCallFilter=, ProtectProc= and ProcSubset= into the unit.
- Installing the helper and probing it.
- The D-11 phone hand test.

## Not covered

- A missing or disabled **timer** is not detected. The button works without it, but the hourly runs do not happen. Only the Update check row's age shows it.
- A unit kept anywhere in systemd's search path other than `/etc/systemd/system` is reported missing.

## Deviations from Plan

None in behaviour. Three measurement notes:

1. **[Measurement] Layout comparison exited 1.** The cause is b850900 on /settings, explained above. The plan allowed for differences "from the IN-02..IN-04 fixes"; the one that occurred came from quick-261001-sa9 instead.
2. **[Measurement] Some RED cases passed before the code.** These were "script too", the group-reason cases and the page cross-check, because they hold behaviour or constants already there. Injections (c), (i) and the group cases' own assertions show them able to fail.
3. **[Addition] Two injections beyond the plan:** (a2), a branch not cloning the commands, and (f2), the note's container guard dropped. Both went red.

## Known Stubs

None.

## Self-Check: PASSED

- internal/host/host.go contains `update_unit_install_commands`. HostActions.tsx has 2 mentions of `UPDATE_UNIT_ACTIONS` (the plan asked for at least 2). host.tsx has `HostUpdateUnitNotice`.
- Commits 13ce423 and 735a768: FOUND.
- `~/.cache/holzkube-manager-layout/13-17-after.jsonl` and `13-17-compare.txt`: FOUND.
