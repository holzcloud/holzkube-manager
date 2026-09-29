---
phase: 13-host-aktionen
plan: 07
subsystem: host-actions
tags: [host-actions, systemd, units, install-guide, release-archive, gate]
status: complete

requires:
  - phase: 13-host-aktionen
    plan: 04
    provides: deploy/holzkube-manager-host.sh with its ORDER / STATE_DIR defaults and exit codes
  - phase: 13-host-aktionen
    plan: 06
    provides: HelperScriptPath, PathUnitPath, ServiceUnitPath, WantsLinkPath, InstallCommands
provides:
  - deploy/holzkube-manager-host.path and deploy/holzkube-manager-host.service
  - deploy/HOST-HELPER.md (install, probe, daemon hardening unchanged, other data directory, results, recovery, update, uninstall)
  - the four helper files in the default release archive
  - internal/host/hostaction/units_test.go (six tests, the systemd-analyze output gate with its negative control)
affects: [13-08, 13-09]

actuals:
  tokens: 9400
  tasks: 2
  commits: 2
plan_head_before: c06f7a82856aed070c64017731f83bfc387004ae
plan_head_after: ce6a2277c210d249418990a057bf6c1c41a8b548

tech-stack:
  added: []
  patterns:
    - "systemd-analyze verify gated on empty combined output and rc 0, with a negative control in the same test"
    - "Unit files read by a minimal line reader (sections, Key=Value, comments skipped, anything else fatal) for consistency checks; systemd-analyze stays the parser"
    - "Every assignment of a hardening key must carry the wanted value (a later line would win in systemd)"

key-files:
  created:
    - deploy/holzkube-manager-host.path
    - deploy/holzkube-manager-host.service
    - deploy/HOST-HELPER.md
    - internal/host/hostaction/units_test.go
  modified:
    - .goreleaser.yaml

key-decisions:
  - "The drop-in for another data directory also adds ReadWritePaths=-<dir> on the service: without it the script cannot remove the order there and fails every order"
  - "TestGuideKeepsTheDaemonsHardening checks the bullets inside the section 'The service's own unit stays as it is', not anywhere in the file, and rejects weakening phrases in the whole guide"
  - "The update-script test rejects any mention of holzkube-manager-host or HOST-HELPER, comments included (D-19: it does not so much as name the helper)"
  - "The unit comments are German and ASCII, like the two deploy scripts; the guide is English, like docs/guide.md"

requirements-completed: [HACT-08]

duration: 14min
completed: 2026-09-29
---

# Phase 13 Plan 07: The helper's units, install guide and archive Summary

**`deploy/` now holds the path unit, the root oneshot service, the script and `HOST-HELPER.md`; all four travel in the release archive; `systemd-analyze verify` accepts both units by output, not just by exit code; and six tests hold units, script defaults, guide, archive list and the Go constants together.**

## Where this ran

The operator's Raspberry Pi 5 (aarch64), systemd 257 (257.13-1~deb13u1), Go 1.26.7 via GOTOOLCHAIN, no -race (ThreadSanitizer refuses the Pi 5 kernel's address space). **Nothing was installed**: no file under `/etc`, `/usr/local` or `/var/lib` was written and no `systemctl` command addressed the system manager; `ls /usr/local/sbin/holzkube-manager-host /etc/systemd/system/holzkube-manager-host.path` both fail afterwards. `systemd-analyze verify` ran on copies in temporary directories only. The production service was not touched: `ActiveEnterTimestamp=Mon 2026-09-28 18:06:00 CEST` before (02:28Z) and after (02:41Z).

## Performance

- **Duration:** ~14 min
- **Started:** 2026-09-29T02:28:25Z
- **Completed:** 2026-09-29T02:42Z
- **Tasks:** 2
- **Files:** 5 (4 created, 1 modified)

## Accomplishments

- `holzkube-manager-host.path`: `PathExists=/var/lib/holzkube-manager/host-order`, `Unit=holzkube-manager-host.service`, `WantedBy=paths.target`, no `MakeDirectory=`, no `Environment=`; comment says what it is, that only the operator installs it, and why no MakeDirectory.
- `holzkube-manager-host.service`: the 13-RESEARCH text verbatim (`Type=oneshot`, `ExecStart=/usr/local/sbin/holzkube-manager-host`, `TimeoutStartSec=3min`, `StateDirectory=holzkube-manager-host` 0755, `ReadWritePaths=-/var/lib/holzkube-manager`, `UMask=0022`, the full D-18 list with `RestrictAddressFamilies=AF_UNIX`), no `[Install]`, no RemainAfterExit, no capability or syscall restriction; the comment block gives the reason for each.
- `deploy/HOST-HELPER.md`: what the helper is and why not D-Bus or sudoers; needs (systemd 250+, GNU coreutils); the install block between the markers, identical to `InstallCommands`; the three-step probe ending with "Check for updates and install"; the daemon's eight hardening lines named as unchanged; the drop-ins for another data directory; `last`, the journal and the exit codes; recovery after `unit-start-limit-hit`; manual updating; uninstall.
- `.goreleaser.yaml`: the four files in `archives[default].files` after the update script, with the comment that nothing installs or replaces them.
- `units_test.go`: TestUnitsVerify, TestUnitsAgree, TestInstallCommandsMatchTheGuide, TestGuideKeepsTheDaemonsHardening, TestTheArchiveCarriesTheHelper, TestTheUpdateScriptDoesNotShipTheHelper.

## Task Commits

1. **Task 1: The units, the guide and the archive** - `ef90427` (feat)
2. **Task 2: The gate that reads verify's output, and the consistency tests** - `ce6a227` (test)

## Fault injections (each put in, run, red read from `go test`'s own exit code, restored from a saved copy; `cmp` against the copy printed nothing and `restored` after every one)

| ID | Injection | rc | Failing line |
|---|---|---|---|
| F17 | `ProtectHome=true` -> `ProtectHom=true` in `deploy/holzkube-manager-host.service` | 1 | `--- FAIL: TestUnitsVerify` `units_test.go:231: systemd-analyze verify on the shipped units: exit 0, output: /tmp/TestUnitsVerify697019574/001/holzkube-manager-host.service:48: Unknown key 'ProtectHom' in section [Service], ignoring.` (note: verify's own exit was 0 -- the gate went red on the output alone) |
| F18 | `-m 0755` -> `-m 0775` in the guide's first install line | 1 | `--- FAIL: TestInstallCommandsMatchTheGuide` `units_test.go:437: the guide's install block differs from InstallCommands (the page's block).` with both blocks quoted |
| F19 | `tar -xzf "$ARCHIVE" -C "$WORK" deploy/holzkube-manager-host.sh` added after `set -euo pipefail` in `deploy/holzkube-manager-update.sh` | 1 | `--- FAIL: TestTheUpdateScriptDoesNotShipTheHelper` `units_test.go:599: ../../../deploy/holzkube-manager-update.sh:31 names the host helper: tar -xzf ...` |
| RAE | `RemainAfterExit=yes` added after `Type=oneshot` in the service | 1 | `--- FAIL: TestUnitsAgree` `units_test.go:356: holzkube-manager-host.service:40: RemainAfterExit=yes must not be in the helper's service` |
| NC | the test's negative control given a valid line (`ProtectHome=yes`) instead of the misspelling -- a verify that "stopped reporting" | 1 | `--- FAIL: TestUnitsVerify` `units_test.go:245: negative control: systemd-analyze verify printed nothing (exit 0) for a service with ProtectHom=true. ...` |
| G1 | a sentence asking for `ProcSubset=all` appended to the guide | 1 | `--- FAIL: TestGuideKeepsTheDaemonsHardening` `units_test.go:513: the guide contains "ProcSubset=all", which loosens the daemon's hardening` |
| G2 | the `` - `UMask=0077` `` bullet removed from the guide | 1 | `--- FAIL: TestGuideKeepsTheDaemonsHardening` `units_test.go:507: the section "## The service's own unit stays as it is" does not name `UMask=0077` as unchanged` |
| A1 | `deploy/HOST-HELPER.md` removed from the default archive's files | 1 | `--- FAIL: TestTheArchiveCarriesTheHelper` `units_test.go:581: the default release archive does not carry deploy/HOST-HELPER.md (files: [...])` |
| S1 | the script's STATE_DIR default changed to `/var/lib/holzkube-manager-helper` | 1 | `--- FAIL: TestUnitsAgree` `units_test.go:381: the script's STATE_DIR defaults to /var/lib/holzkube-manager-helper, want /var/lib/holzkube-manager-host` |
| T0 | test binary run with `PATH` pointing at an empty directory (no systemd-analyze) | 1 | `--- FAIL: TestUnitsVerify` `units_test.go:177: systemd-analyze is not installed, so the helper's units cannot be verified ...` |
| T0' | same, with `HOLZKUBE_MANAGER_NO_SYSTEMD_ANALYZE=1` | 0 | `--- SKIP: TestUnitsVerify` `units_test.go:175: SKIPPED, not verified: systemd-analyze is not available here ...` |

A first attempt at NC (replacing the misspelling with the identical `ProtectHome=true`) went red on the test's other guard instead ("the shipped service has no line ProtectHome=true to misspell"), because the replacement then changed nothing; the NC row above is the injection that reaches the output check.

## Verification (exit codes read from the command itself)

- Task 1 verify: temp copies, ExecStart pointed at a stub, `systemd-analyze verify --man=no`: rc 0, output empty.
- Acceptance greps: every count >= 1 (PathExists, WantedBy, ExecStart, ReadWritePaths, StateDirectory, ProcSubset=pid, NoNewPrivileges=true, both markers, the four files in `.goreleaser.yaml`); `.goreleaser.yaml` parses as YAML.
- `go test ./internal/host/hostaction -count=1 -run '<the six>' -v`: rc 0, 6 `--- PASS`, 0 `--- SKIP`; the negative control logs `Unknown key 'ProtectHom' in section [Service], ignoring.`
- `go test ./internal/... ./cmd/... -count=1`: rc 0, no FAIL line.
- `./bin/task lint:go`: rc 0, 0 issues.
- `go test ./internal/publicrepo/ -count=1`: ok (after both commits' content was in place).
- `git diff c06f7a8 -- go.mod go.sum`: empty (`gopkg.in/yaml.v3` was already a direct dependency).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing critical functionality] The other-data-directory drop-in also needs ReadWritePaths**
- **Found during:** Task 1 (writing the guide)
- **Issue:** the plan's drop-in (reset `PathExists=`, set `Environment=HOLZKUBE_MANAGER_HOST_ORDER=`) leaves the new directory read-only under `ProtectSystem=strict`; the script's consuming `rm` would fail and every order end `failed` (Pitfall 4).
- **Fix:** the service drop-in in `HOST-HELPER.md` adds `ReadWritePaths=-/srv/holzkube-manager` and says why.
- **Files modified:** deploy/HOST-HELPER.md
- **Commit:** ef90427

Additions beyond the plan's list, all in `units_test.go`: TestUnitsAgree also requires the rest of D-18 (PrivateDevices, kernel/namespace protections, `RestrictAddressFamilies=AF_UNIX`, MemoryDenyWriteExecute, SystemCallArchitectures) and forbids `EnvironmentFile=` and `AmbientCapabilities=`; it checks that the script writes `$STATE_DIR/last`; TestInstallCommandsMatchTheGuide and TestTheArchiveCarriesTheHelper also check that every `deploy/` file the install commands name exists and is in the archive.

## Known Stubs

None.

## Threat Flags

None beyond the plan's threat model (T-13-38..T-13-45). No new route or endpoint; the only new surface is root code the operator installs by hand, which is the plan's subject.

## Next Phase Readiness

- 13-08: the page's install block and `HOST-HELPER.md` are now held to the same `InstallCommands`; the recovery command the D-13 message names is the guide's.
- 13-09: docs/guide.md and the README can link `deploy/HOST-HELPER.md`; it is in the archive.

## Self-Check: PASSED
