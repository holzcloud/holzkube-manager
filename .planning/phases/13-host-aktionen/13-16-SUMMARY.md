---
phase: 13-host-aktionen
plan: 16
subsystem: host-actions
tags: [host-actions, systemd, update, sandbox, timer, release-archive, problem-codes, tdd]
status: complete

requires:
  - phase: 13-host-aktionen
    plan: 15
    provides: the update-script refusal (409 conflict.host-update-script-missing) and its place in the order container, missing, update script, outdated, busy (13-REVIEW-2-FIX IN-04)
provides:
  - deploy/holzkube-manager-update.service and deploy/holzkube-manager-update.timer: the hourly update, the operator's ExecStart and schedule, sandboxed line by line
  - UpdateUnitPath, UpdateTimerPath, UpdateUnitInstallCommands, held byte for byte to deploy/HOST-HELPER.md and docs/guide.md
  - both units (and every other file in deploy/) in the default release archive
  - TestTheUpdateUnitRunsTheUpdate, TestTheUpdateTimerKeepsTheHour, TestTheUpdateTimingContract; TestUnitsVerify over five units
  - 409 conflict.host-update-unit-missing on POST /api/v1/host/confirm (host.update) and POST /api/v1/host/actions/update, before a token or an order
  - MissingUpdateUnit, NeedsUpdateUnit, UpdateUnitMissing, Box.UpdateUnit
  - README, docs/guide.md "Updating itself every hour", deploy/HOST-HELPER.md "The hourly update", docs/api-contract.md
affects: [13-17, phase-13-verification]

actuals:
  tokens: 25878   # chars/4 over `git show` of the plan's four commits (c317861, 4e48d57, fd811b0, 442a602)
  tasks: 3
  commits: 4      # measured: git log --no-merges --oneline 0d6233f..442a602 | grep -c '(13-16)'
plan_head_before: 0d6233f6df65b1f11c9cbfe21d4cb705f88438b5
plan_head_after: 442a6020f2fb38aca09fbf72059f97613ad5a123

tech-stack:
  added: []
  patterns:
    - "A timing contract computed from the script's own numbers, each read by a pattern that must match exactly once, so a changed number fails until the model is redone"
    - "A unit is held by a closed allow-list; every line the sandbox leaves out has its reason in the unit and in the test's doc comment"
    - "A unit's presence is read through fs.Stat, so systemctl link (symlink to a real unit) counts and a mask (symlink to /dev/null) does not"

key-files:
  created:
    - deploy/holzkube-manager-update.service
    - deploy/holzkube-manager-update.timer
  modified:
    - .goreleaser.yaml
    - deploy/HOST-HELPER.md
    - docs/guide.md
    - docs/api-contract.md
    - README.md
    - internal/host/hostaction/helper.go
    - internal/host/hostaction/helper_test.go
    - internal/host/hostaction/units_test.go
    - internal/httpapi/problem.go
    - internal/httpapi/handlers/host.go
    - internal/httpapi/hostactionsapi_test.go
    - internal/httpapi/hosthelperapi_test.go

key-decisions:
  - "The update unit's refusal sits after the update script's and before outdated and busy: container, missing, update script, update unit, outdated, busy"
  - "Only update needs the update unit; check-update starts the check unit, which Outdated already covers"
  - "CapabilityBoundingSet=, SystemCallFilter=, ProtectProc= and ProcSubset= stay out of the update unit until a real install and restart through it has been measured as root"
  - "The update unit is looked for in /etc/systemd/system only; a unit kept elsewhere in systemd's search path is reported missing"
  - "No changelog entry and no release: none was asked for"

requirements-completed: [HACT-08, HACT-04]

coverage:
  - id: D1
    description: "systemd-analyze verify accepts all five shipped units with empty output; misspelt copies of the update service and the timer each print a line"
    requirement: HACT-08
    verification:
      - kind: unit
        ref: "internal/host/hostaction/units_test.go#TestUnitsVerify"
        status: pass
  - id: D2
    description: "The update unit and the timer are held line for line and in time against the script, the check unit, the helper and the hour"
    requirement: HACT-08
    verification:
      - kind: unit
        ref: "internal/host/hostaction/units_test.go#TestTheUpdateUnitRunsTheUpdate, TestTheUpdateTimerKeepsTheHour, TestTheUpdateTimingContract"
        status: pass
  - id: D3
    description: "The shipped update unit rates below the installed hand-made unit's 9.2 (systemd-analyze security --offline=yes --threshold=91)"
    requirement: HACT-08
    verification:
      - kind: other
        ref: "Task 2 security check (verify block), exit 0; shipped 5.2 MEDIUM, installed 9.2 UNSAFE"
        status: pass
  - id: D4
    description: "UpdateUnitInstallCommands held byte for byte to deploy/HOST-HELPER.md and docs/guide.md; the archive carries both units and every deploy/ file"
    requirement: HACT-08
    verification:
      - kind: unit
        ref: "internal/host/hostaction/units_test.go#TestUpdateUnitInstallCommandsMatchTheGuide, TestTheArchiveCarriesTheHelper"
        status: pass
  - id: D5
    description: "Both routes refuse update with 409 conflict.host-update-unit-missing while the unit is absent, a directory or masked; the other four actions go through"
    requirement: HACT-04
    verification:
      - kind: integration
        ref: "internal/httpapi/hosthelperapi_test.go#TestHostUpdateNeedsTheUpdateUnit"
        status: pass
      - kind: unit
        ref: "internal/host/hostaction/helper_test.go#TestUpdateUnitMissing, TestNeedsUpdateUnit"
        status: pass

duration: "started 2026-10-01T16:10Z (ledger base), paused after a WIP commit, finished 2026-10-02T04:15Z; working time not measured"
completed: 2026-10-02
---

# Phase 13 Plan 16: The hourly update's units, shipped and held, and update refused without them Summary

**holzkube-manager-update.service and .timer now ship in deploy/ and the release archive, behaviour-compatible with the operator's hand-made pair, sandboxed to a 5.2 exposure (the hand-made unit rates 9.2), held line by line and in time by tests; Check for updates and install is refused with 409 conflict.host-update-unit-missing before a token or an order while the unit is not installed.**

## Where it ran

The operator's Raspberry Pi 5, aarch64, Go 1.27.1 from `~/.local/go`, no `-race` (the race verdict is CI's). Nothing on the host was stopped, started, enabled, installed or replaced; the system manager was never asked to load or run anything. Nothing pushed, tagged or released; no changelog entry.

Task 1 and Task 2 ran on 2026-10-01 (c317861 at 18:17, 4e48d57 at 18:22 CEST). Task 3 was paused mid-way and committed as WIP fd811b0, merged into main as 795c14a together with quick task 261001-sa9; it was finished on 2026-10-02 (442a602). The Task 1/2 injections (a)-(q) were **redone on 2026-10-02** against the finished tree, because the first session left no record of their exit codes; their RED runs, verify output, write-set and /proc traces are the 2026-10-01 logs, quoted from the scratchpad.

## Commits and how they were counted

The ledger `.git/gsd-plan-head-before-13-16` holds 0d6233f (the plan's last revision before Task 1). `git rev-list --count 0d6233f..442a602` is 22, but that range includes the merge 795c14a, the merge of origin/main and the 16 commits of quick task 261001-sa9 that landed on main while this plan was paused. The plan's own commits, counted with `git log --no-merges --oneline 0d6233f..442a602 | grep -c '(13-16)'`, are 4:

| Task | Commit | What |
|------|--------|------|
| 1 | c317861 | feat(13-16): ship the hourly update's service and timer |
| 2 | 4e48d57 | feat(13-16): hold the hourly update's units to their lines and their times |
| 3 (part) | fd811b0 | wip(13-16): paused mid-task (merged as 795c14a) |
| 3 | 442a602 | feat(13-16): refuse Check for updates and install while its unit is not installed |

## Task 1: the two units, their install commands and the archive

- **First RED** (2026-10-01, the tests against 13-15's tree): build failed, exit 1 -- `units_test.go:991:26: undefined: UpdateUnitInstallCommands` (then `UpdateTimerPath`, `UpdateUnitPath`).
- **GREEN**: all Task 1 tests pass; re-run 2026-10-02 inside Task 2's verify set, exit 0, 0 SKIP.
- **Verify, negative controls** (re-run 2026-10-02, verify block 2 exit 0, three controls):
  - `negative control (ProtectHom=true in holzkube-manager-host.service), exit 0: .../holzkube-manager-host.service:50: Unknown key 'ProtectHom' in section [Service], ignoring.`
  - `negative control (ProtectHom=true in holzkube-manager-update.service), exit 0: .../holzkube-manager-update.service:119: Unknown key 'ProtectHom' in section [Service], ignoring.`
  - `negative control (OnUnitActivSec=1h in holzkube-manager-update.timer), exit 0: .../holzkube-manager-update.timer:29: Unknown key 'OnUnitActivSec' in section [Timer], ignoring.`
- **(a) verify reads the copy, not the installed unit**: only the timer passed to `systemd-analyze verify`, a misspelt update service beside it in a scratch directory → exit 0 and printed `<scratch>/verify-a/holzkube-manager-update.service:119: Unknown key 'ProtectHom' in section [Service], ignoring.`; with the spelling restored, exit 0 and 0 lines.

Injections, redone 2026-10-02, each restored from a scratch copy and checked with `cmp`, exit codes read from `go test` itself:

| | Fault | Test | Exit | First FAIL line |
|-|-------|------|------|-----------------|
| (b) | `--now` → `--nov` in HOST-HELPER.md's update-unit block | TestUpdateUnitInstallCommandsMatchTheGuide | 1 | `units_test.go:1348: ../../../deploy/HOST-HELPER.md's update-unit block differs from UpdateUnitInstallCommands.` |
| (c) | timer's line removed from .goreleaser.yaml | TestTheArchiveCarriesTheHelper | 1 | `units_test.go:1554: the default release archive does not carry deploy/holzkube-manager-update.timer` |
| (d) | throwaway regular file in deploy/, not in the archive (then deleted) | TestTheArchiveCarriesTheHelper | 1 | `units_test.go:1554: the default release archive does not carry deploy/zz-13-16-throwaway.txt` |
| (e) | `systemctl daemon-reload` appended to the update script | TestTheUpdateScriptDoesNotShipTheHelper | 1 | `units_test.go:1584: ../../../deploy/holzkube-manager-update.sh:545 names daemon-reload: systemctl daemon-reload` |
| (f1) | install -d path → /usr/local/lib in the constant only | TestUpdateUnitInstallCommandsMatchTheGuide | 1 | `units_test.go:1348: ... HOST-HELPER.md's update-unit block differs from UpdateUnitInstallCommands.` |
| (f2) | the same in the constant and both guides (so only the PREVIOUS check can catch it) | TestUpdateUnitInstallCommandsMatchTheGuide | 1 | `units_test.go:1359: UpdateUnitInstallCommands = ["sudo install -d -o root -g root -m 0755 /usr/local/lib" ...` |

### Behaviour compatibility with the operator's pair

Key=value lines, comments stripped, the operator's copies from `$HOME/.cache/holzkube-manager-13-16/` against `deploy/`:

- **Same in both services**: `[Unit]`, `After=network-online.target`, `Wants=network-online.target`, `[Service]`, `Type=oneshot`, `ExecStart=/usr/local/sbin/holzkube-manager-update`, `PrivateTmp=true`; no `[Install]` in either.
- **Service, different**: `Description=` (installed: `holzkube-manager - Update aus dem neuesten GitHub-Release`; shipped: `... Update aus dem neuesten Release, stuendlich und auf Auftrag`); `Documentation=` (installed: the repository URL; shipped: deploy/HOST-HELPER.md's URL). Only in the shipped unit: `TimeoutStartSec=45min`, `TimeoutStopSec=30s` (the backstop above the computed 38m30s worst case; the installed unit has systemd's oneshot default, no start limit), `StateDirectory=holzkube-manager-update`, `StateDirectoryMode=0755`, `UMask=0022`, `ReadWritePaths=/usr/local/bin /usr/local/sbin -/usr/local/lib/holzkube-manager`, and the sandbox `NoNewPrivileges ProtectSystem=strict ProtectHome PrivateDevices ProtectKernelTunables ProtectKernelModules ProtectKernelLogs ProtectControlGroups ProtectClock ProtectHostname RestrictAddressFamilies(4) RestrictNamespaces RestrictRealtime RestrictSUIDSGID LockPersonality MemoryDenyWriteExecute SystemCallArchitectures=native PrivateIPC`.
- **Same in both timers**: `[Unit]`, `[Timer]`, `OnBootSec=5min`, `OnUnitActiveSec=1h`, `RandomizedDelaySec=5min`, `[Install]`, `WantedBy=timers.target`.
- **Timer, different**: `Description=` (installed: `holzkube-manager - stuendlich auf neue Releases pruefen`; shipped: `... stuendlich nach einem neueren Release sehen und es installieren`); `Documentation=` only in the shipped one; `Persistent=true` only in the installed one -- left out because it acts only with `OnCalendar=`, so it does nothing there.

## Task 2: lines, times, the sandbox measured

- **RED** (2026-10-01): exit 1 -- `units_test.go:1300: the guide has no install block between <!-- update-script-command:begin --> and <!-- update-script-command:end -->` and `units_test.go:1346: ... <!-- update-unit-commands:begin --> ...`; the three new unit tests passed on Task 1's units (no unit line needed changing).
- **GREEN**, re-run 2026-10-02: Task 2's verify set (11 tests) exit 0, 11 PASS, 0 SKIP. Verify block 2 (held blocks, README line, publicrepo) exit 0.
- **Computed worst cases** (TestTheUpdateTimingContract's t.Logf):
  - `update worst case 38m30s (lock 10m0s + list 30s + 2 x download 5m0s + 2 x restart 7m30s + 20 x (5s + 1s) + local 1m0s); TimeoutStartSec 45m0s leaves 6m30s above it, margin wanted 5m0s`
  - `update unit worst case 47m0s (TimeoutStartSec 45m0s + 4 x TimeoutStopSec 30s), the timer waits 1h0m0s`
  - `check unit worst case 2m40s; the update waits 10m0s for the lock; the check waits 1m0s + list 30s + slack 15s within 2m0s`
- **Security** (Task 2 verify block 3, run 2026-10-02 on scratch copies, ExecStart= pointed at /bin/true): exit 0; shipped copy rc 0, installed copy rc 1 at `--threshold=91`:
  - `→ Overall exposure level for holzkube-manager-update.service: 5.2 MEDIUM :-|` (shipped)
  - `→ Overall exposure level for holzkube-manager-update.service: 9.2 UNSAFE :-{` (installed, the control)
  - the check unit's copy (2026-10-01): `1.9 OK :-)`
- **Write set** (2026-10-01, `strace -f` of `TestUpdateScriptAsRoot/healthy_update` as root in a user namespace; strace followed into the namespace). Entries the script's processes created, removed or renamed: the status directory (`.lock`, `.status.*`, rename to `status.json`) = STATUS_DIR; `usr-local-lib/` created and `holzkube-managerd.previous` written = PREVIOUS's directory; `usr-local-bin/holzkube-managerd` unlinked and recreated = BIN's directory; `/tmp/holzkube-manager-update.XXXXXX` created and removed = the mktemp directory (PrivateTmp). Nothing else, apart from `O_CREAT` opens of `/dev/null` by shell redirections. The script's own directory was not written in this run (no self-replacement in the healthy-update fixture), so its `ReadWritePaths=` entry rests on the static check (the directory of UpdateScriptPath) alone.
- **/proc reading** of the two read-only traces (2026-10-01): `systemctl is-active` opens `/proc/`, `/proc/filesystems`, `/proc/sys/kernel/cap_last_cap`, `/proc/sys/kernel/ngroups_max` and `/proc/1/root` (O_PATH, EACCES as an unprivileged user) -- it does look at PID 1's root, which is the chroot check the unit's comment names; `journalctl -n 1` opens only `/proc/filesystems` and `/proc/sys/kernel/cap_last_cap`, nothing under `/proc/1`. This supports, and does not change, keeping ProtectProc= and CapabilityBoundingSet= out.

Injections (g)-(q), redone 2026-10-02, each restored and `cmp`-checked; afterwards `git diff --quiet HEAD -- deploy/holzkube-manager-update.sh deploy/holzkube-manager-host.sh deploy/holzkube-manager-update-check.service` exit 0:

| | Fault | Test | Exit | First FAIL line |
|-|-------|------|------|-----------------|
| (g) | ExecStart= with --force | TestTheUpdateUnitRunsTheUpdate | 1 | `holzkube-manager-update.service:110: [Service] ExecStart=/usr/local/sbin/holzkube-manager-update --force, want ExecStart=/usr/local/sbin/holzkube-manager-update` |
| (h) | ExecStartPost=/bin/true added | TestTheUpdateUnitRunsTheUpdate | 1 | `holzkube-manager-update.service:111: [Service] ExecStartPost=/bin/true is not a line this unit may carry` |
| (i) | CapabilityBoundingSet= added | TestTheUpdateUnitRunsTheUpdate | 1 | `holzkube-manager-update.service:136: [Service] CapabilityBoundingSet= is not a line this unit may carry` |
| (j1) | /usr/local/sbin removed from ReadWritePaths= | TestTheUpdateUnitRunsTheUpdate | 1 | `...:116: [Service] ReadWritePaths=/usr/local/bin -/usr/local/lib/holzkube-manager: want exactly -/usr/local/lib/holzkube-manager /usr/local/bin /usr/local/sbin` |
| (j2) | /etc added to ReadWritePaths= | TestTheUpdateUnitRunsTheUpdate | 1 | `...:116: [Service] ReadWritePaths=/usr/local/bin /usr/local/sbin -/usr/local/lib/holzkube-manager /etc: want exactly ...` |
| (k) | AF_PACKET added | TestTheUpdateUnitRunsTheUpdate | 1 | `...:128: [Service] RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6 AF_NETLINK AF_PACKET: want exactly AF_INET AF_INET6 AF_NETLINK AF_UNIX` |
| (l1) | Persistent=true in the timer | TestTheUpdateTimerKeepsTheHour | 1 | `holzkube-manager-update.timer:31: [Timer] Persistent=true is not a line this unit may carry` |
| (l2) | Unit=holzkube-manager-update-check.service in the timer | TestTheUpdateTimerKeepsTheHour | 1 | `holzkube-manager-update.timer:31: [Timer] Unit=holzkube-manager-update-check.service is not a line this unit may carry` |
| (m1) | TimeoutStartSec=20min | TestTheUpdateTimingContract | 1 | `update worst case 38m30s ...; TimeoutStartSec 20m0s leaves -18m30s above it, margin wanted 5m0s` |
| (m2) | TimeoutStopSec=5min | TestTheUpdateTimingContract | 1 | `units_test.go:1189: ... TimeoutStartSec 45m0s + 4 x TimeoutStopSec 5m0s = 1h5m0s, not below the timer's OnUnitActiveSec 1h0m0s` |
| (n) | the helper's update arm without --no-block | TestTheUpdateTimingContract | 1 | `units_test.go:1208: the helper's update arm runs systemctl start holzkube-manager-update.service, want exactly systemctl start --no-block holzkube-manager-update.service` |
| (o) | download()'s --max-time 300 → 1800 | TestTheUpdateTimingContract | 1 | `update worst case 1h28m30s (... 2 x download 30m0s ...); TimeoutStartSec 45m0s leaves -43m30s above it` |
| (p) | the check's lock wait 60 → 120 | TestTheUpdateTimingContract | 1 | `units_test.go:1203: the check waits 2m0s for the lock and 30s for the release list (+ 15s), more than the check unit's TimeoutStartSec 2m0s` |
| (q) | `--now` → `--nov` in the guide's update-unit block | TestUpdateUnitInstallCommandsMatchTheGuide | 1 | `units_test.go:1348: ../../../docs/guide.md's update-unit block differs from UpdateUnitInstallCommands.` |

## Task 3: the 409 while the unit is not installed

- **RED**, redone 2026-10-02 against Task 2's tree (`git archive 4e48d57` into the scratchpad with the three Task 3 test files copied in):
  - `go test ./internal/host/hostaction -run 'TestUpdateUnitMissing|TestNeedsUpdateUnit'` exit 1, build failed: `helper_test.go:421:30: undefined: MissingUpdateUnit`, `undefined: UpdateUnitMissing`, `NewBox(...).UpdateUnit undefined`, `undefined: NeedsUpdateUnit`.
  - `go test ./internal/httpapi -run TestHostUpdateNeedsTheUpdateUnit` exit 1: absent, a_directory, masked and busy_too red -- `hosthelperapi_test.go:896: POST /api/v1/host/confirm for host.update: 200, want 409 conflict.host-update-unit-missing`, `hosthelperapi_test.go:946: ... code "conflict.host-helper-busy", want "conflict.host-update-unit-missing"`. (The 2026-10-01 session's RED logs show the same.)
- **As found** (the WIP commit): `NeedsUpdateUnit` returned `a == Update || a == CheckUpdate` -- injection (u) had been left in. `TestNeedsUpdateUnit` exit 1 (`helper_test.go:536: NeedsUpdateUnit is true for [update check-update], want exactly [update]`) and `TestHostUpdateNeedsTheUpdateUnit` exit 1 (`hosthelperapi_test.go:908: confirm host.check-update: 409, want 200`). Fixed to `a == Update` in 442a602.
- **GREEN**: `TestUpdateUnitMissing|TestNeedsUpdateUnit|TestUpdateScriptMissing -v` exit 0, 0 SKIP; the six httpapi tests with -v exit 0, 0 SKIP; TestHostActionRoundTrip and TestHostCheckRoundTrip ran the helper script as root in a user namespace (2 log lines "root in a user namespace").

Injections (r)-(w), 2026-10-02, each restored and `cmp`-checked:

| | Fault | Test | Exit | First FAIL line |
|-|-------|------|------|-----------------|
| (r) | the action route's question removed | TestHostUpdateNeedsTheUpdateUnit | 1 | `busy_too: hosthelperapi_test.go:946: POST /api/v1/host/actions/update: code "conflict.host-helper-busy", want "conflict.host-update-unit-missing"` (absent, a_directory, masked red too: 202 and an order placed) |
| (s) | the confirm route's question removed | TestHostUpdateNeedsTheUpdateUnit | 1 | `absent: hosthelperapi_test.go:896: POST /api/v1/host/confirm for host.update: 200, want 409 conflict.host-update-unit-missing` |
| (t) | the unit's question moved before the update script's, both routes | TestHostUpdateNeedsTheUpdateUnit | 1 | `update_script_missing_too: hosthelperapi_test.go:938: POST /api/v1/host/confirm for host.update: code "conflict.host-update-unit-missing", want "conflict.host-update-script-missing"` |
| (u) | NeedsUpdateUnit also true for CheckUpdate | TestNeedsUpdateUnit / TestHostUpdateNeedsTheUpdateUnit | 1 / 1 | `helper_test.go:536: NeedsUpdateUnit is true for [update check-update], want exactly [update]` / `masked: hosthelperapi_test.go:908: confirm host.check-update: 409, want 200` |
| (v) | fs.Lstat instead of fs.Stat | TestUpdateUnitMissing | 1 | `a_symlink_to_a_regular_unit_elsewhere: helper_test.go:514: UpdateUnitMissing = [{Item:update-unit Path:/etc/systemd/system/holzkube-manager-update.service}], want []` |
| (w) | the unit dropped from installedHelperFS | whole ./internal/httpapi | 1 | `TestHostActionGates/sudo_window: hostgatesapi_test.go:187: confirm host.update: 409, want 200`; also red: TestHostConfirmGates, TestHostActionRoundTrip, TestHostActionsNeedTheHelper/installed, TestHostActionsWaitForARunningCheck, TestHostTokenOpensOneOrderInItsOwnSession |

On (v): the plan expected "the mask case red". Under `fs.Lstat` a mask (a symlink to /dev/null) is still not a regular file, so it is still reported missing and `TestHostUpdateNeedsTheUpdateUnit` stays green (exit 0, measured). The case that catches Lstat is the `systemctl link` one, which Lstat would wrongly call missing; the test's doc comment already names that case.

## Suites and gates (2026-10-02)

- `go test ./internal/... ./cmd/... -count=1`: exit 0, 41 packages ok, no FAIL.
- `./bin/task lint:go`: exit 0, `0 issues.`
- `go test ./internal/publicrepo/ -count=1`: exit 0 (before the commit as well).

## Host record, before and after

Verify block 4 (the host check) on the real records: **exit 0**. `holzkube-manager.service` NRestarts and ActiveEnterTimestamp identical to the 2026-10-01 before-record (NRestarts=0; the service has not restarted since 2026-09-29); `holzkube-manager-update.timer` ActiveState=active, UnitFileState=enabled, identical; `/etc/systemd/system/holzkube-manager-update.service` and `.timer` `cmp`-identical to the copies taken before Task 1; `/usr/local/sbin/holzkube-manager-host` and `/etc/systemd/system/holzkube-manager-update-check.service` still absent. The records were not re-taken.

The host check seen red without touching the host, on scratch copies of `$HOME/.cache/holzkube-manager-13-16/`, re-copied between runs: unaltered copy exit 0; (x1) NRestarts changed → exit 1; (x2) UnitFileState=disabled → exit 1; (x3) one byte appended to the copied .timer → exit 1 (`cmp: EOF on /etc/systemd/system/holzkube-manager-update.timer after byte 192`); an empty directory → exit 2.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] NeedsUpdateUnit was true for check-update as well**
- **Found during:** Task 3, the first run on the merged tree
- **Issue:** the WIP commit fd811b0 carried `return a == Update || a == CheckUpdate` -- injection (u), never restored before the pause. On that tree Check for updates was refused with 409 conflict.host-update-unit-missing whenever the update unit was missing.
- **Fix:** `return a == Update`.
- **Files modified:** internal/host/hostaction/helper.go
- **Commit:** 442a602

**2. [Process] Injections (a)-(q) redone**
- The Task 1 and Task 2 commit messages say each was seen red, but no exit code or FAIL line was kept. All were redone on 2026-10-02 and are recorded above; (f) was run twice, once with the constant alone and once with the constant and both guides changed.

**3. [Measurement] Injection (v) goes red on the systemctl-link case, not the mask case** -- see the note under Task 3's table.

## Known Stubs

None.

## Left for 13-17

The page half of the refusal: `update_unit` in `GET /api/v1/host`'s answer, the button off with its reason and a note naming the install commands, the fixtures and screenshots.

## Human needed (the operator's call)

- Installing the shipped units on the Pi in place of the hand-made ones (the guide's block replaces same-named units).
- The first real install and restart through the shipped unit, as root. That is the measurement that would let CapabilityBoundingSet=, SystemCallFilter=, ProtectProc= and ProcSubset= into the unit.

## Self-Check: PASSED

- deploy/holzkube-manager-update.service, deploy/holzkube-manager-update.timer: FOUND
- commits c317861, 4e48d57, fd811b0, 442a602: FOUND
