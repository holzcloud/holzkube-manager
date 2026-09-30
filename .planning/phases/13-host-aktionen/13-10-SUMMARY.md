---
phase: 13-host-aktionen
plan: 10
subsystem: host-actions
tags: [host-actions, helper-detection, fixture, api-contract, screenshots, gap-closure]
status: complete
gap_closure: true
gap_ids: [G-13-2]

requires:
  - phase: 13-host-aktionen
    plan: 06
    provides: hostaction.Detect, the Missing items
  - phase: 13-host-aktionen
    plan: 09
    provides: demo.json helper-missing state, TestTheFixtureShowsTheRealInstallCommands, host.png render recipe
provides:
  - Detect lists not-enabled only when both unit files are regular files and the wants entry is absent
  - TestHelperEveryCombination (all 16 presence combinations)
  - fixture, contract, page tests and host.png showing [script, path-unit] for an empty machine
affects: [14]

actuals:
  tokens: 4900
  tasks: 3
  commits: 3
plan_head_before: 84aa87888425ecf40fb78cdd445687aa4cbcd155
plan_head_after: dd670b3aadcc611307e5ad19ab509b42048c088d

tech-stack:
  added: []
  patterns:
    - "An invariant over every presence combination of the inputs (bitmask walk over fstest.MapFS deletions) beside the example rows"

key-files:
  created: []
  modified:
    - internal/host/hostaction/helper.go
    - internal/host/hostaction/helper_test.go
    - internal/host/actions_test.go
    - internal/httpapi/hosthelperapi_test.go
    - internal/host/hostaction/units_test.go
    - web/fixtures/demo.json
    - web/src/fixtures.test.ts
    - docs/api-contract.md
    - docs/screenshots/host.png
    - web/src/routes/host.test.tsx
    - web/src/routes/host.browser.test.tsx
    - web/src/api.ts

decisions:
  - "G-13-2 fixed in Detect, not in the sentence: not-enabled is only asked about once both unit files are there, so the page's 'installed but not enabled' is true whenever it is shown (D-12)"
  - "A wants entry with both unit files absent reads as [path-unit]: a dangling link does not make a unit installed"

metrics:
  duration: 10min
  completed: 2026-09-30
---

# Phase 13 Plan 10: Helper notice says not-enabled only when it is true (G-13-2) Summary

`hostaction.Detect` now asks about the `paths.target.wants` entry only once both unit files are regular files, so an empty machine reads `[script, path-unit]`, and the notice no longer says "holzkube-manager-host.path is installed but not enabled" one line below saying the path unit is missing. The fixture, the contract, the page tests and the README picture of /host all show that two-item list.

**Where it ran:** the operator's Pi (aarch64), Go 1.26.7 from ~/.local/go, Node via nvm. No `-race`. Nothing installed, pushed or released. holzkube-manager.service and /var/lib/holzkube-manager were not touched.

## Tasks

1. **Task 1 (tracer, TDD): Detect names not-enabled only with both units installed.** Commit `51631f8` (fix)
2. **Task 2: fixture, contract and host.png show the two-item list.** Commit `a1f1679` (docs)
3. **Task 3: page tests render only lists the server can send.** Commit `dd670b3` (test)

## Red runs (exit codes read from `go test` itself)

- **Task 1 RED**, new tests against the unchanged `helper.go`, `go test ./internal/host/... ./internal/httpapi/ -run 'TestHelper|TestReadCarriesActions|TestHostActionsNeedTheHelper'`: **EXIT=1**. Failing: `TestHelperEveryCombination` (6 "lists path-unit and not-enabled together" lines), `TestHelper/nothing_installed`, `TestHelper/script_installed,_no_unit_files,_not_enabled`, `TestHelper/.service_absent,_not_enabled`, `TestHelperBoxMissing`, `TestReadCarriesActions/nothing_installed`, `TestHostActionsNeedTheHelper/nothing_installed`, `TestHostActionsNeedTheHelper/script_installed,_no_unit_files,_not_enabled`.
- **Task 1 fault reinstated** (wants check made unconditional again, `go test ./internal/host/hostaction/ -run TestHelper`): the first try (`if true {`) did not compile ("declared and not used: unitsInstalled"), so it was not a measurement and was redone. The second try (`if unitsInstalled || !unitsInstalled {`) compiled: **EXIT=1**, failing `TestHelperBoxMissing`, `TestHelperEveryCombination`, `TestHelper/.service_absent,_not_enabled`, `TestHelper/script_installed,_no_unit_files,_not_enabled`, `TestHelper/nothing_installed`. Restored from the saved copy; EXIT=0 again; the tree held only the task's four files.
- **Task 2 RED** of the fixture guard before touching the fixture, `-run TestTheFixtureShowsTheRealInstallCommands`: **EXIT=1**, `units_test.go:665: Detect on an empty machine reports 2 pieces, want 3`.
- **Task 2 fault reinstated** (the old third fixture entry put back): **EXIT=1**, both the `slices.Equal` line and the new `names path-unit and not-enabled together` line fired. Restored, `cmp` clean, EXIT=0.

## Verification (green)

- `go test ./internal/host/... ./internal/httpapi/ -run 'TestHelper|TestReadCarriesActions|TestHostActionsNeedTheHelper'`: EXIT=0
- `go test ./internal/ -run TestTheDaemonStartsNoProcess`: EXIT=0
- `go test ./internal/host/... ./internal/httpapi/ ./internal/publicrepo/`: EXIT=0
- jsdom `host.test.tsx HostActions.test.tsx fixtures.test.ts`: EXIT=0, 221 tests. Browser `host.browser.test.tsx`: EXIT=0, both "fits the helper notice" rows ran by name
- `npm run lint`: EXIT=0 (2 warnings and 1 info from before, in wall.test.tsx and DataTable.tsx). `npm run typecheck`: EXIT=0
- `git diff --stat a45015c..HEAD -- go.mod go.sum web/package.json web/package-lock.json`: empty
- Acceptance greps: `func TestHelperEveryCombination` 1; `"item": "path-unit"` in demo.json 1; `"install_commands"` 1; `SCRIPT, PATH_UNIT, NOT_ENABLED` in host.test.tsx 0; `installed but not enabled` in HostActions.tsx 1

## Render (host.png)

- holzkube-manager.service **before**: `ActiveEnterTimestamp=Tue 2026-09-29 20:25:38 CEST`, `NRestarts=0`
- `./bin/task build:web` (rc 0), `go build -ldflags "-s -w -X main.version=v0.1.0"` (`--version`: v0.1.0), `node web/scripts/readme-images.mjs` (rc 0). All other re-rendered images (banner, social, app-detail, apps, clusters, kubernetes, node-hardware, phone, wall, three web/public icons) restored with `git checkout --`; `git status --porcelain -- docs/screenshots docs/brand web/public` showed only host.png. Then `./bin/task build:go` (rc 0).
- holzkube-manager.service **after**: `ActiveEnterTimestamp=Tue 2026-09-29 20:25:38 CEST`, `NRestarts=0`. No change.
- Checked host.png by eye: the notice lists `/usr/local/sbin/holzkube-manager-host` and `/etc/systemd/system/holzkube-manager-host.path` and no paths.target.wants line. The device values in it (hostname, model, kernel, disk) were grepped and all come from demo.json.

## Deviations from Plan

**1. [Rule 3 - Blocking] biome formatting of host.test.tsx**
- **Found during:** Task 3, `npm run lint` (EXIT=1, one format error in the edited file)
- **Fix:** `biome format --write` on the three Task 3 files before committing; lint EXIT=0 after
- **Commit:** dd670b3

**2. The browser test's `li` count is now exact and scoped to the notice** (the old test used `>= 3` over the whole page). The plan asked for the count to match each shape; scoping it to the notice is how that can be exact.

Otherwise the plan ran as written. README text unchanged: its Host paragraph ("names what is missing and the commands that install it") is still true.

## Known Stubs

None.

## Self-Check: PASSED

- FOUND: internal/host/hostaction/helper.go, helper_test.go, docs/screenshots/host.png, web/fixtures/demo.json
- FOUND commits: 51631f8, a1f1679, dd670b3
