---
phase: 13-host-aktionen
plan: 09
subsystem: docs
tags: [host-actions, fixture, api-contract, guide, readme, screenshots, gate]
status: complete

requires:
  - phase: 13-host-aktionen
    plan: 02
    provides: figure-first Health.Public sentences for the wall
  - phase: 13-host-aktionen
    plan: 07
    provides: deploy/HOST-HELPER.md, units_test.go, InstallCommands held to the guide
  - phase: 13-host-aktionen
    plan: 08
    provides: the four buttons, the reason line, the helper notice on /host
provides:
  - demo.json /api/v1/host actions in the helper-missing state; the wall's host reason figure first
  - TestTheFixtureShowsTheRealInstallCommands (Go) and two fixture assertions (vitest)
  - api-contract Host actions completed (428/403 codes as sent, 409 details, withdrawn, 10-s and startup withdrawal, result file mode, helper-missing example, wall's public order)
  - guide "Host actions" subsection
  - README host line and Host paragraph; host.png and wall.png re-rendered
affects: [14]

actuals:
  tokens: 6700
  tasks: 2
  commits: 3
plan_head_before: 6176573e70f15ddb0f9bdb170d4044c224dee63a
plan_head_after: 8f36495890a801442313f6ec1b30a1cd138d3a33

tech-stack:
  added: []
  patterns:
    - "A fixture's copy of a server constant is compared with the constant by a Go test (Detect on an empty fstest.MapFS is the missing list)"

key-files:
  created: []
  modified:
    - web/fixtures/demo.json
    - web/src/fixtures.test.ts
    - internal/host/hostaction/units_test.go
    - docs/api-contract.md
    - docs/guide.md
    - README.md
    - docs/screenshots/host.png
    - docs/screenshots/wall.png
    - web/scripts/readme-images.mjs

key-decisions:
  - "The fixture's missing list is compared with Detect(fstest.MapFS{}), not with a hand-written list: the server's order and the path-unit item's first path come from the code"
  - "The contract's 403 codes corrected to what the server sends (forbidden.confirmation-invalid / -expired, forbidden.role); the previous text said confirmation.invalid"
  - "readme-images.mjs scrolls to the top before measuring a tall shot: /host is now taller than the window and the range click left its header out of the picture"

requirements-completed: [HACT-01, HACT-02, HACT-03, HACT-04, HACT-05, HACT-06, HACT-07, HACT-08]

duration: 45min
completed: 2026-09-29
---

# Phase 13 Plan 09: Fixture, contract, guide, README and pictures Summary

**The demo fixture now shows /host as a machine without the helper (not available, three pieces missing, the four install commands, held to the Go constants by a test), the wall's host reason is figure first, the contract, the guide and the README describe the four host actions and the root helper, host.png shows the buttons and the helper notice, and `./bin/task ci` exits 0 on the Pi with the production service untouched.**

## Where this ran

The operator's Raspberry Pi 5 (aarch64). Go 1.26.7 via GOTOOLCHAIN, Node v22 via nvm, Playwright Chromium from `~/.cache/ms-playwright`. No `-race`: ThreadSanitizer refuses the Pi 5 kernel's address space, so the race verdict belongs to CI. Nothing was installed, pushed, tagged or released.

**Production service:** `systemctl show holzkube-manager.service -p ActiveEnterTimestamp -p NRestarts` before (03:10Z): `ActiveEnterTimestamp=Mon 2026-09-28 18:06:00 CEST`, `NRestarts=0`; after (03:53Z): the same, `ActiveEnterTimestamp=Mon 2026-09-28 18:06:00 CEST`, `NRestarts=0`. Its unit, `/usr/local/bin`, and `/var/lib/holzkube-manager` were not touched. `test ! -e /usr/local/sbin/holzkube-manager-host && test ! -e /etc/systemd/system/holzkube-manager-host.path`: rc 0. After the gate only the production daemon was running (`pgrep -fa holzkube-managerd`: `/usr/local/bin/holzkube-managerd` alone).

## Performance

- **Duration:** ~45 min (most of it the gate)
- **Started:** 2026-09-29T03:10Z
- **Completed:** 2026-09-29T03:55Z
- **Tasks:** 2
- **Files:** 9 modified

## Accomplishments

- **Fixture:** `/api/v1/host.actions` = `order: null`, result not readable with `host-action.no-result` and the server's sentence, `available: false`, `missing` script / path-unit / not-enabled with their paths, `install_commands` = `InstallCommands`. The wall's host `reason` is `82.1 °C ≥ 80 °C · cpu_thermal`; the page's `health.warnings` stay name first.
- **Tests:** `TestTheFixtureShowsTheRealInstallCommands` (Go); in `fixtures.test.ts` the wall sentence is asserted from the fixture's own sensor values, and a new test asserts the helper-missing state (the schema's default would otherwise parse a fixture without `actions`).
- **Contract:** the status table now has the three 409 details verbatim, `428 sudo.required`, `403 forbidden.role`, `403 forbidden.confirmation-invalid/-expired`; the order's `withdrawn` state; "No order is left lying" (the 10-s withdrawal by rename, the startup withdrawal, the helper's own age window); the result file's `0644` in the helper's `0755` state directory; a helper-missing `actions` example; the wall's `reason` in its public, figure-first order and the `warnings` row saying the page keeps the name first.
- **Guide:** "Host actions" under "The Host page": the four actions (update = the timer's run, which installs), who may press them (operator, typed hostname, sudo window, audit names), why a helper and that `deploy/HOST-HELPER.md` installs it, what the status box shows (placed, picked up, started, waiting, back; rejected / failed with `journalctl -u holzkube-manager-host`; withdrawn after 10 s with `systemctl status holzkube-manager-host.path`), one order at a time, no host actions in a container, the drop-in for another data directory.
- **README:** the "What it does" host line adds restart service, restart or shut down the machine, check for an update and install it, through a root-owned helper installed from `deploy/`, the service never root; the Host paragraph adds the four actions and the helper notice. The alpha statement is unchanged.
- **Pictures:** `host.png` (1440 x 1791) shows the four buttons, the helper reason line, the helper notice with its three missing paths and the four commands, the cards and the 24-h curves; `wall.png` shows the host tile as "manager · 82.1 °C ≥ 80 °C · …" -- the figure whole, the sensor name cut, which is what figure first is for. Only fixture values appear (`manager-01.homelab.example`, `srv-node-0x.homelab.example`, the fixture's model/OS/kernel strings).

## Task Commits

1. **Task 1: fixture, contract, guide, README** - `ce3c2ae` (docs)
2. **Gate fix: biome format of the new assertion** - `26acc7a` (style)
3. **Task 2: host.png, wall.png, the renderer's scroll** - `8f36495` (docs)

## Fault injections (each put in from a saved copy of demo.json, run, red read from the test command's own exit code, restored and `cmp`-checked -- "restored" printed after every one)

| ID | Injection | rc | Failing line |
|---|---|---|---|
| I1 | fixture's first install command `-m 0755` -> `-m 0775` | 1 | `--- FAIL: TestTheFixtureShowsTheRealInstallCommands` `units_test.go:659: the fixture's install_commands differ from InstallCommands.` |
| I2 | `not-enabled` item removed from the fixture's `missing` | 1 | `units_test.go:668: the fixture's missing list is [{script ...} {path-unit ...}], want what Detect reports with nothing installed: [... {not-enabled /etc/systemd/system/paths.target.wants/holzkube-manager-host.path}]` |
| I3 | the whole `actions` object removed (JSON still valid) | 1 | `units_test.go:655: the fixture's /api/v1/host has no actions: the page would parse the default and show no helper notice` |
| I4 | `available: false` -> `true` | 1 | first run printed the pointer (`is 0x21d5183cc240`); message fixed to a switch, rerun: `units_test.go:682: the fixture's actions.available is true, want false: the helper is not installed` |
| W1 | wall reason back to name first (`cpu_thermal 82.1 °C ≥ 80 °C`) | 1 | vitest `× puts the host on the wall and on /host in warning, the same host both times` -- `expected 'cpu_thermal 82.1 °C ≥ 80 °C' to be '82.1 °C ≥ 80 °C · cpu_thermal'` |
| W2 | `actions` removed (as I3) | 1 | vitest `× shows /host with the helper not installed, the widest the page gets` -- `expected [] to deeply equal [ Array(3) ]` (note: `available` alone stayed green, because the schema's default is false; the missing list is what catches it) |

## Verification (exit codes read from the command itself)

- Task 1: `go test ./internal/host/hostaction ./internal/httpapi ./internal/config ./internal/publicrepo -count=1`: rc 0 (httpapi 287 s under load). `npx vitest run --project jsdom src/fixtures.test.ts src/routes/host.test.tsx src/routes/wall.test.tsx`: rc 0, 3 files, 157 tests.
- Acceptance greps: `install_commands` 1, `82.1 °C ≥ 80 °C · cpu_thermal` 1, `func TestTheFixtureShowsTheRealInstallCommands` 1, `/api/v1/host/actions/` 5, `host-action.no-result` 4, `withdrawn` 5 in the contract, `HOST-HELPER.md` 2 and `Check for updates and install` 1 in the guide, `helper` 2 and `alpha` 2 in the README.
- **`./bin/task ci`, first run: rc 201** -- `lint:web` refused the one-line `toBe` in `fixtures.test.ts` (biome format). Fixed in `26acc7a`.
- **`./bin/task ci`, second run: rc 0.** lint:web, test:web, build, lint:go, `go test ./...` (no FAIL), and test:layout: "Nothing out of reach at 390px and 1280px, and every control is at least 44px at 390px." -- with the new fixture, so `/host` at 390 px was measured with the helper notice and its commands.
- In the same run, the chain's non-failing `test:next` step (`go test ./...` again) had `--- FAIL: TestANodeSaysHowItBooted (46.16s)` `live_test.go:856: SecurityState on a SecureBoot node: talos: Get on 00000000-0000-4000-8000-000000000001 timed out` -- the known flake under load. Rerun alone: `go test ./internal/upgrade/ -count=1 -run TestANodeSaysHowItBooted -v`: rc 0, `--- PASS (6.52s)`; the whole package `go test ./internal/upgrade/ -count=1`: rc 0.
- `GOOS=linux GOARCH=amd64 go build ./...`: rc 0. `GOOS=darwin GOARCH=arm64 go build ./...`: rc 0. `go test ./internal/publicrepo/ -count=1`: rc 0 (after all content was in place). `file bin/holzkube-managerd`: `ELF 64-bit LSB executable, ARM aarch64`.
- `git diff 6176573..HEAD -- go.mod go.sum web/package.json web/package-lock.json`: empty.
- `git status --porcelain -- docs/screenshots` before the commit: only `host.png` and `wall.png`.

## Render

`./bin/task build:web`, then `go build -ldflags "-s -w -X main.version=v0.1.0" -o bin/holzkube-managerd ./cmd/holzkube-managerd` (`--version`: `v0.1.0`), `node web/scripts/readme-images.mjs` (rc 0; the script's own daemon on 127.0.0.1 with a temporary data directory, stopped by the script). Every other re-rendered image (banner, social, app-detail, apps, clusters, kubernetes, node-hardware, phone, the three web/public icons) restored with `git checkout --`. The first render of host.png lost the page's top (see Deviations); the second is the committed one. Afterwards `bin/holzkube-managerd` was rebuilt with `./bin/task build:go` (`v0.0.1-170-g8f36495`).

## Optional probe: the helper under the user manager

`systemd-run --user --unit=hkm-helper-probe --path-property=PathExists=<scratch>/data/host-order -- unshare --user --map-root-user env HOLZKUBE_MANAGER_HOST_ORDER=… HOLZKUBE_MANAGER_HOST_STATE_DIR=<scratch>/state HOLZKUBE_MANAGER_SYSTEMCTL=<scratch>/stub bash <scratch>/helper.sh` (a copy of `deploy/holzkube-manager-host.sh`; the stub systemctl appends its arguments to a file). One order `update 0123456789abcdef` placed by hand (0600, written to a temp name and moved in). Observed: the order file gone; the stub called once with `start --no-block holzkube-manager-update.service`; `last` = `0123456789abcdef update started 2026-09-29T03:23:49Z`; the journal has one `Started hkm-helper-probe.service` and one `Auftrag 0123456789abcdef: update`; the service `Result=success`, `ExecMainStatus=0`, `NRestarts=0`, inactive -- it ran once and did not loop. The path unit was stopped afterwards and `systemctl --user list-units --all 'hkm-*'` lists nothing. The system manager was never addressed.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] The README renderer cut off the top of /host**
- **Found during:** Task 2 (first render)
- **Issue:** `shoot()` clicks the "24 h" button, which scrolls it into view; with the helper notice /host is taller than the 900-px window, the page stayed scrolled, and `boundingBox()` (viewport-relative) then sized the window from the scrolled position. The picture started at the helper notice's last line: no header, no buttons.
- **Fix:** scroll the window and every scrolled element back to 0 before measuring the `through` element.
- **Files modified:** web/scripts/readme-images.mjs (not in the plan's list)
- **Commit:** 8f36495

**2. [Rule 1 - Truthfulness] The contract's 403 codes**
- **Found during:** Task 1
- **Issue:** the Host actions subsection said `403 confirmation.invalid` / `confirmation.expired`; the server sends `forbidden.confirmation-invalid` / `forbidden.confirmation-expired` (problem.go).
- **Fix:** corrected, and `forbidden.role` and `428 sudo.required` added to the status table.
- **Commit:** ce3c2ae

**3. [Rule 3 - Blocking] biome format of the new assertion**
- **Found during:** Task 2 (first gate run, rc 201)
- **Fix:** `biome format --write src/fixtures.test.ts`, line break only.
- **Commit:** 26acc7a

**4. README / guide wording after seeing the picture**
- The first draft said the helper notice is "at the bottom"; the rendered page shows it under the header, after the hardening note. Both texts corrected in 8f36495.

### Additions beyond the plan's lists

- `web/src/fixtures.test.ts`: the wall sentence and the helper-missing state asserted from the TypeScript side (W1, W2 above).

**Total deviations:** 3 auto-fixed (2 bug/truthfulness, 1 blocking) plus one wording correction. **Impact:** none on scope.

## Known Stubs

None.

## Threat Flags

None. No route, no code path of the daemon changed; the fixture and the pictures carry documentation values only, and `internal/publicrepo` passes.

## Next Phase Readiness

- Phase 13 is complete on the page, in the docs and in the pictures. What no test here could do: the real two-session reboot on the Pi, and installing the helper on the production host -- both the operator's call (CLAUDE.md), recorded as the UI-SPEC's manual backstops.
- Phase 14: the dialog's close X (28 px) on a phone, as 13-08 recorded.

## Self-Check: PASSED
