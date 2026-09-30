---
phase: 13-host-aktionen
plan: 13
subsystem: host-actions
tags: [host-actions, update-check, root-helper, systemd, contract, tdd]
status: complete

requires:
  - phase: 13-host-aktionen
    plan: 12
    provides: the fifth order check-update, deploy/holzkube-manager-update-check.service, the check route
provides:
  - the older helper frozen at 8b64a06 (internal/host/hostaction/testdata/holzkube-manager-host-before-check.sh) and TestAnOlderHelperRefusesTheCheck
  - the helper's marker line `# holzkube-manager-host orders: reboot poweroff restart-service update check-update`, held by TestTheHelperNamesItsOrders
  - hostaction.HelperOrdersMarker, UpdateCheckUnitPath, UpdateScriptPath
  - the check unit under the systemd-analyze verify gate, and TestTheCheckUnitRunsOnlyTheCheck
  - proof, as root, that the update script's --check installs nothing
  - the check unit in InstallCommands, HOST-HELPER.md, the release archive, demo.json and the API contract
affects: [13-14, 13-15]

actuals:
  tokens: 14700
  tasks: 2
  commits: 2
plan_head_before: 8f5c1d9f2e40e7b857be86c576c7d0f664bac1f0
plan_head_after: ed3db0149486413a64e7c0f934adc6c3b5ee36e6

tech-stack:
  added: []
  patterns:
    - "An older version of root code frozen byte for byte in testdata and run as root in a user namespace, so a new order word is proven harmless against what hosts may still have installed"
    - "A marker line in a script held to the script's own pattern, case arms and the Go list, so a daemon can read it as the truth about the code"
    - "Stubs that log their calls only when a variable names a log, so existing cases stay unchanged and one case can ask what was downloaded or called"

key-files:
  created:
    - internal/host/hostaction/testdata/holzkube-manager-host-before-check.sh
  modified:
    - deploy/holzkube-manager-host.sh
    - deploy/holzkube-manager-host.service
    - deploy/HOST-HELPER.md
    - internal/host/hostaction/helper.go
    - internal/host/hostaction/helper_test.go
    - internal/host/hostaction/script_test.go
    - internal/host/hostaction/units_test.go
    - internal/host/updatestatus/script_test.go
    - internal/httpapi/handlers/confirm_test.go
    - .goreleaser.yaml
    - web/fixtures/demo.json
    - web/src/routes/host.browser.test.tsx
    - web/src/routes/host.test.tsx
    - docs/api-contract.md

key-decisions:
  - "The marker line is `# holzkube-manager-host orders: ` followed by the words, single spaces; a helper without it knows the first four orders. It is additive, so a later order is a new word, not a migration"
  - "The check unit is held to all of the helper service's sandbox keys except RestrictAddressFamilies (it must reach GitHub) and ReadWritePaths (it may write only its StateDirectory), plus an empty CapabilityBoundingSet"
  - "The check unit's network is described in words in HOST-HELPER.md: a quoted RestrictAddressFamilies line naming AF_INET and AF_UNIX would read as a loosened daemon to TestGuideKeepsTheDaemonsHardening"

requirements-completed: []

metrics:
  duration: 23min
  completed: 2026-09-30
---

# Phase 13 Plan 13: The helper's order list as a contract, and the check unit in the install set Summary

**The helper's list of orders is now a tested contract with root code that lives on hosts. The helper as it was before check-update (frozen from 8b64a06) refuses the new word as root without acting. The shipped helper names its five orders on one marker line that a test holds to its code. The check unit passes systemd-analyze and is proven to run only `--check`, which installs nothing, as root. The install commands, guide, archive, fixture and contract all include the check unit.**

## Where it ran

The operator's Raspberry Pi 5 (aarch64), Go from `~/.local/go`, Node through nvm, the browser project's Chromium. No `-race`: ThreadSanitizer refuses this kernel, so the race verdict belongs to CI. Root cases ran only inside `unshare --user --map-root-user`, against stand-in systemctl and curl.

- `holzkube-manager.service` at the start of Task 1: `ActiveEnterTimestamp=Tue 2026-09-29 20:25:38 CEST`, `NRestarts=0`
- after Task 2: `ActiveEnterTimestamp=Tue 2026-09-29 20:25:38 CEST`, `NRestarts=0`. Identical.
- Nothing installed: `/usr/local/sbin/holzkube-manager-host`, `/etc/systemd/system/holzkube-manager-host.path` and `/etc/systemd/system/holzkube-manager-update-check.service` are all absent (exit 0). Nothing was pushed, tagged or released.

## The frozen older helper

`git log -1 --format=%H 8ba3647 -- deploy/holzkube-manager-host.sh` = `8b64a062b51497f8259f9461b4313216e05ff24b`, the commit the plan named. `git show 8b64a06:deploy/holzkube-manager-host.sh` was written to `internal/host/hostaction/testdata/holzkube-manager-host-before-check.sh` (mode 0644). After every step, `cmp` against that `git show` exited 0.

## Red runs (exit codes read from the command itself)

Each injection was saved, edited once, run, restored from the saved copy, checked with `cmp` (exit 0), and followed by one green run.

| # | Injection | Test | Exit | Failing line |
|---|---|---|---|---|
| RED | the tests before the marker line existed | TestTheHelperNamesItsOrders | 1 | `units_test.go:459: the helper script has no line beginning "# holzkube-manager-host orders: "` (all other new tests were already green on 13-12's code) |
| I1 | `--no-block` added to the check arm | TestHostScriptAsRoot | 1 | `valid_check-update: systemctl called with "start --no-block holzkube-manager-update-check.service\n", want exactly "start holzkube-manager-update-check.service\n"` |
| I2 | `check-update` removed from the marker line | TestTheHelperNamesItsOrders | 1 | `units_test.go:519: the marker line names [poweroff reboot restart-service update], Actions() is [check-update poweroff reboot restart-service update]` |
| I2b | `halt` added to the pattern only | TestTheHelperNamesItsOrders | 1 | `units_test.go:519: the pattern names [check-update halt poweroff reboot restart-service update], Actions() is [...]` |
| I3 | ` --check` removed from the check unit's ExecStart | TestTheCheckUnitRunsOnlyTheCheck | 1 | `units_test.go:601: holzkube-manager-update-check.service: [Service] ExecStart=/usr/local/sbin/holzkube-manager-update, want ExecStart=/usr/local/sbin/holzkube-manager-update --check` |
| I4 | `ProtectHome` misspelt `ProtectHom` in the check unit | TestUnitsVerify | 1 | `units_test.go:259: systemd-analyze verify on the shipped units: exit 0, output: .../holzkube-manager-update-check.service:57: Unknown key 'ProtectHom' in section [Service], ignoring.` |
| I5 | the frozen copy given check-update in its pattern and a case arm | TestAnOlderHelperRefusesTheCheck | 1 | `script_test.go:784: exit = 0, want 2`; `:786: systemctl was called, want never`; `:788: last = {"c0ffee00c0ffee11" "check-update" "started"}, want {"" "" "rejected"}`; `:793: output quotes the order's id`. Restored; `cmp` against `git show 8b64a06:...` exit 0 |
| I6 | the `exit 0` that ends the CHECK_ONLY block removed from deploy/holzkube-manager-update.sh | TestUpdateScriptAsRoot/check_installs_nothing,_as_root | 1 | `outcome = "updated", want "available"`; `installed binary answers "holzkube-managerd 0.2.0" ... the check replaced it`; `holzkube-managerd.previous exists`; `the check asked for .../releases/assets/11`, `.../assets/13`, the health URL; `the check called systemctl`. Restored; `git diff --quiet -- deploy/holzkube-manager-update.sh` exit 0 |
| (a) | the guide's install block given the old second line | TestInstallCommandsMatchTheGuide | 1 | `units_test.go:734: the guide's install block differs from InstallCommands (the page's block).` |
| (b) | the check unit removed from .goreleaser.yaml | TestTheArchiveCarriesTheHelper | 1 | `units_test.go:878: the default release archive does not carry deploy/holzkube-manager-update-check.service` |
| (c) | demo.json given the old second line | TestTheFixtureShowsTheRealInstallCommands | 1 | `units_test.go:951: the fixture's install_commands differ from InstallCommands.` |

The verify gate's negative control on the helper service stays in place and still reports: `negative control (ProtectHom=true), exit 0: .../holzkube-manager-host.service:50: Unknown key 'ProtectHom' in section [Service], ignoring.`

## Green

- Task 1 `<verify>`: `go test ./internal/host/hostaction -run 'TestHostScript|TestAnOlderHelperRefusesTheCheck|TestTheHelperNamesItsOrders|TestUnitsVerify|TestUnitsAgree|TestTheCheckUnitRunsOnlyTheCheck' -v`: exit 0, 52 PASS lines, 0 SKIP. `go test ./internal/host/updatestatus -run TestUpdateScript -v`: exit 0, 0 SKIP (`check_installs_nothing,_as_root` and `check_available` PASS). `TestEveryHostActionRequiresTyping`: exit 0. The freeze check (`test ... = 8b64a06 && cmp ...`): exit 0. `git diff --quiet 8ba3647 -- deploy/holzkube-manager-update.sh`: exit 0.
- Task 2 `<verify>`: the seven guide/archive/fixture/unit tests: exit 0, 0 SKIP. `go test ./internal/... ./cmd/... -count=1`: exit 0, 41 `ok` packages, no FAIL (publicrepo included). `./bin/task lint:go`: 0 issues.
- `cd web && npx vitest run --project jsdom`: exit 0, 56 files, 742 tests. The first run exited 1 with three 5-s timeouts in `images.test.tsx`, a file this plan did not touch. That run overlapped the whole Go suite on the Pi. Run again unloaded, it exited 0.
- `npm --prefix web run test:browser -- src/routes/host.browser.test.tsx`: exit 0, 9 passed, now measuring against the real (wider) second line. Web lint exit 0 (the 2 pre-existing warnings), typecheck exit 0.
- `git diff --quiet 8ba3647 -- go.mod go.sum web/package.json web/package-lock.json`: exit 0. No dependency changed.

## Task Commits

1. **Task 1: the helper's order list as a contract**: `fab98ad` (feat)
2. **Task 2: the check unit in what the operator installs**: `ed3db01` (feat)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Stale text] deploy/holzkube-manager-host.service still said "vier feste Befehle"**
- **Found during:** Task 1 (listed by 13-12 under "Left for later plans" for 13-13)
- **Fix:** it now says "fuenf". This is a comment line only; TestUnitsVerify and TestUnitsAgree are green.
- **Commit:** fab98ad

**2. [Rule 3 - Blocking] internal/host/hostaction/helper_test.go holds InstallCommands' exact lines**
- **Found during:** Task 2
- **Issue:** TestInstallCommandsAreTheGuidesBlock, which is in the plan's verify list, compares against its own copy of the four lines. That file is not in the plan's file list.
- **Fix:** its copy takes the new second line, and its "installs under /etc/systemd/system/" loop now includes UpdateCheckUnitPath.
- **Commit:** ed3db01

**3. confirm_test.go**
- 13-12 had already made the failure message name HACT-05. What this plan changed is the doc comment above TestEveryHostActionRequiresTyping, which still said every host action takes the machine away. It now says HACT-05 asks for the typing on every host action, and that the check is typed too, even though it takes nothing away.

### Left for later plans

- The daemon does not read the marker line yet, and no route refuses a check against an older helper. Both are 13-14's.
- HostHelperNotice's "four orders" wording, the README, the older-helper paragraph in HOST-HELPER.md, and re-rendering the README picture of /host (its fixture's second install line is now longer) are 13-15's. HACT-04 is **not** marked complete.

## Known Stubs

None.

## Threat Flags

None beyond the plan's register (T-13-67 to T-13-70). No endpoint and no daemon file access were added. The new surface is the guide and archive telling the operator to install one more root-run unit, and that unit is the one the register's T-13-67 and T-13-69 cover.

## Self-Check: PASSED

- FOUND: internal/host/hostaction/testdata/holzkube-manager-host-before-check.sh
- FOUND: fab98ad, ed3db01
