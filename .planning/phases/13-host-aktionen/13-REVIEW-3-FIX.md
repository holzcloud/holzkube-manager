---
phase: 13-host-aktionen
fixed_at: 2026-10-02T08:30:00Z
review_path: .planning/phases/13-host-aktionen/13-REVIEW-3.md
iteration: 1
findings_in_scope: 9
fixed: 7
skipped: 2
status: partial
---

# Phase 13: Code Review Fix Report (round 4, 13-REVIEW-3)

**Fixed at:** 2026-10-02
**Source review:** .planning/phases/13-host-aktionen/13-REVIEW-3.md
**Iteration:** 1

**Summary:**
- In scope: CR-01, WR-01..WR-03, and IN-01, IN-02, IN-04 (asked for where
  trivial or falling out of the warnings). IN-03 and IN-05 were asked for
  only if cheap.
- Fixed: CR-01, WR-01, WR-02, WR-03, IN-01, IN-02, IN-04.
- Not fixed: IN-03, IN-05 (reasons below).

## Where it ran

The operator's Raspberry Pi 5 (aarch64), main checkout
(`workflow.use_worktrees` is `false`), Go from `~/.local/go` without `-race`,
Node through nvm. Every gate below ran in the main checkout, so it can be
reproduced from this tree. Root cases of the update script ran as root in an
unprivileged user namespace with stand-ins for systemctl, curl, journalctl and
sleep, as the existing tests do; the read-only cases add a mount namespace with
read-only bind mounts, which is what `ProtectSystem=strict` does to the paths
outside `ReadWritePaths=`. Nothing on the host was stopped, started, enabled,
installed or replaced. Nothing was pushed or released, and there is no
changelog entry.

Host record (`systemctl show`), before and after: see "Host record" at the
end -- unchanged.

## Fixed Issues

### CR-01: the hourly update now refuses a host it would not really update

**Commits:** 3c44529 (and e37de8a, gofmt of its test)
**Mechanism chosen: (b) and (a), with (c) already in place.**

- **(b) The script asks before it touches anything.** Every run that may
  install (not `--check`, which installs and restarts nothing) runs
  `check_host` after the lock and before the network: `systemctl show --value
  -p LoadState` must be `loaded`; `systemctl show --value -p ExecStart` must
  be exactly one `path=` that is `$BIN` or resolves to it (`readlink -f`); and
  a running service must answer at `$HEALTH_URL` (`wait_healthy 3`). A stopped
  service is not asked first: the update may be what brings it back, and the
  check after the restart decides. On any refusal the script fails with the
  reason, and the EXIT trap records `failed` (installed = what is on disk,
  latest = null, since it never looked). ExecStart rather than
  `/proc/<MainPID>/exe`: what matters for "updated" to be true is what the
  restart will run, and that is ExecStart of the loaded unit; the exe of the
  current process answers a different question (it is about to be replaced)
  and could not be tested with the shell stand-ins.
  `update.conf` may now set `HOLZKUBE_MANAGER_SERVICE` (new) and
  `HOLZKUBE_MANAGER_HEALTH_URL` for a differently laid out host; the paths
  stay the unit's defaults (WR-02).
- **(a) The layout is documented and shipped.** `deploy/holzkube-manager.service`
  is the daemon's own unit -- the reference installation's hardening line for
  line, data in `/var/lib/holzkube-manager`, the default `--listen`, settings
  from `EnvironmentFile=-/etc/holzkube-manager/holzkube-managerd.env` so the
  unit stays replaceable -- carried in the release archive. docs/guide.md has
  a new "Running it as a service" with its install block; "Updating itself
  every hour", HOST-HELPER.md "The hourly update" and the README (Quick start
  and the "Updates itself" line) state the layout the update needs and what it
  does elsewhere. The README's "the systemd unit ... is in the guide" is now
  true.
- **(c) already held:** routes/host.tsx shows "The update status names X as
  installed; this process runs Y." when status.json's `installed` differs
  from the running version. Its test went red with that line removed (below).
- The health probe is one function (`healthy`/`wait_healthy N`) used before
  and after; TestTheUpdateTimingContract counts both waits: worst case 38m48s,
  6m12s under TimeoutStartSec=45min.
- `--help` printed a fixed line range (2..29) and would have cut the new
  header; it now prints the comment header up to the first code line.

**The operator's installation behaves as before:** all existing script cases
pass unchanged (healthy update, already current, rollback, lock, signal
cases), and `check_host`'s systemd questions and health probe, extracted and
run read-only as the session user against the production
`holzkube-manager.service`, passed (exit 0: LoadState loaded, ExecStart
`/usr/local/bin/holzkube-managerd`, health answered). The write checks were
not asked there (not root).

**Red runs** (exit codes from `go test` / `vitest` themselves; every fault
restored from a copy, `cmp` 0, then green):

| | Fault | Exit | What failed |
|-|-------|------|-------------|
| new tests vs. the unchanged script | -- | 1 | 6 refusal cases: `exit = 0, want 1`, `outcome = "updated", want "failed"`; "no daemon unit, the binary already the newest" recorded `current` |
| e | LoadState check removed | 1 | no daemon unit / masked / no unit + newest: `exit = 0, want 1` |
| f | ExecStart comparison removed | 1 | another binary: `exit = 0, want 1` |
| g | health preflight removed | 1 | not answering: `outcome = "rolled-back", want "failed"` |
| h | `check_host` never called | 1 | all six refusal cases |
| i | single-ExecStart check removed | 1 | without / with two ExecStart (first try stayed green: the `says` string "ExecStart" matched the temp path; tightened to "nicht genau ein ExecStart=" and a two-ExecStart case added, then red) |
| j | link resolution removed | 1 | link to the binary: `exit = 1, want 0` |
| a | daemon unit ExecStart -> /opt/... | 1 | `units_test.go:1519: ... want exactly /usr/local/bin/holzkube-managerd` |
| b | `Environment=HOLZKUBE_MANAGER_LISTEN=192.168.1.10:8443` added | 1 | `units_test.go:1530: ... sets HOLZKUBE_MANAGER_LISTEN` |
| c | `ProtectProc=invisible` removed | 1 | `units_test.go:1547: ProtectProc=[], want exactly ProtectProc=invisible` |
| d | guide installs the binary to /usr/bin | 1 | `units_test.go:1572: the guide's daemon install block has no ...` |
| (c) | page mismatch line disabled (`const mismatch = false`) | 1 | `names a status that disagrees with the running version` |
| verify | the daemon unit is a new negative control (ProtectHom=true) | 0 + output | `holzkube-manager.service:58: Unknown key 'ProtectHom'` |

### WR-01: a failed restart after the install rolls back

**Commit:** ba937ef

The install's `systemctl restart` is now an `if`; on failure the run skips the
health check (an old process that still answers is not the new release) and
goes to the roll back. The roll back's own restart warns on failure and still
records `rolled-back`, since the previous binary is back on disk. The timing
test counts restart lines with or without an `if` around them.

Red runs (as root in a user namespace; the systemctl stand-in fails restart
with `HKM_STUB_RESTART_FAIL=all|second`):

| | Fault | Exit | What failed |
|-|-------|------|-------------|
| new tests vs. the unchanged script | -- | 1 | `outcome = "failed", want "rolled-back"`, `installed = "0.2.0", want "0.1.0"`, `restart called 1 times, want 2`; roll back's restart failing: `outcome = "failed"` |
| a | bare restart put back | 1 | as above |
| b | restart made `|| true` | 1 | `exit = 0, want 1`, `outcome = "updated", want "rolled-back"` -- the old process answered |
| c | roll back's restart bare again | 1 | `outcome = "failed", want "rolled-back"` |

### WR-02: what the sandbox cannot write is said, and a real run can be checked

**Commits:** 91f341f (and 594f530, the test helper it left unused)

- Status directory: as root, one that cannot be created or written (EROFS)
  now ends the run before the lock, naming it; it used to drop record and lock
  silently. A directory refused for safety (symlink, foreign owner, writable
  by others) still holds no update up, and now logs a warning.
- `check_host` checks before the download that the binary's directory and
  the previous binary's (or its parent, when missing) are writable; an
  update.conf moving BIN or PREVIOUS used to fail half-way after the
  download. ReadWritePaths cannot follow update.conf, so the script refuses
  and the docs say which settings work under the unit.
- A script directory it cannot write no longer turns a healthy update into
  `failed`; the self-replacement warns and the old script stays.
- HOST-HELPER.md and the guide: back up the hand-made units into
  `/root/holzkube-manager-units-before` and `systemctl disable` the service
  before the block (IN-04's stale-enablement half); after it, start one run by
  hand and read `Result`, the journal and `status.json`, with what success
  looks like and the commands that put the hand-made units back; which
  update.conf settings work. The unit, HOST-HELPER.md and the guide say the
  sandbox has not yet been through a real install and restart as root.

Red runs (read-only bind mounts as root in a user + mount namespace):

| | Fault | Exit | What failed |
|-|-------|------|-------------|
| new tests vs. the unchanged script | -- | 1 | read-only status dir: `exit = 0, want 1`, curl and systemctl called; binary/previous dir: GitHub asked before failing; self-replacement: `exit = 1, want 0`, `outcome = "failed", want "updated"` |
| a | unwritable status dir reported as 1 (unsafe) | 1 | status dir root cannot write |
| b | failed `install -d` reported as 1 | 1 | status dir root cannot create |
| c | the fail for 2 made a log line | 1 | both status-dir cases |
| d | `writable_dir` a no-op | 1 | the three directory cases |
| e | bare self-replacing install put back | 1 | script that cannot replace itself |

The positive control "a healthy update replaces the script from the archive"
was added beside it (the self-replacement had no test before).

### WR-03: the Host page says when no timer runs the hourly update

**Commit:** 5000e17

Detected from files, like the helper's path unit: `UpdateTimerMissing`
reports `update-timer` (timer not a regular file once symlinks are followed:
absent, a directory, masked) or else `update-timer-not-enabled` (no
`timers.target.wants` link, Lstat). `GET /api/v1/host` carries
`actions.update_timer`, never null. Nothing is refused: the button starts the
service, and the manual action still works. The page shows "Nothing runs the
update every hour" with the update-unit commands (the last line alone where
only the enablement is missing), only while the unit itself is present --
otherwise the update-unit note covers it with the same commands. Not seen,
and documented: a timer stopped with `systemctl stop` until the next boot,
and a timer of another name. Fixtures carry `update_timer: []`, so no picture
changes and none was re-rendered.

| | Fault | Exit | What failed |
|-|-------|------|-------------|
| tests before the code | -- | 1 / 1 | Go build failure (`undefined: MissingUpdateTimer`); vitest 3 failures |
| a | wants link not asked | 1 | `helper_test.go:631: UpdateTimerMissing = [], want [{update-timer-not-enabled ...}]` |
| b | fs.Lstat for the timer file | 1 | the systemctl-link case reported missing |
| c | collector does not ask | 1 | `actions_test.go:212: actions.update_timer = [], want ...` |
| d | page never shows the note | 1 | two notice tests |
| e | note shown beside the update-unit note | 1 | "leaves it to the update-unit notice" |
| f | demo fixture without update_timer | 1 | fixtures.test.ts |

### IN-01: the update-unit refusal points at the Host page

**Commit:** c924352 -- detail now "The unit holzkube-manager-update.service is
not installed, so no order was placed. The Host page says how to install it.";
pinned test and api-contract follow. Red: the new pin against the old detail,
exit 1 (`hosthelperapi_test.go:902`).

### IN-02: the health window's upper bound

**Commit:** 0f9da66 -- the guide says "usually within about 20 seconds ... at
most about two minutes". Documentation only.

### IN-04: replacing hand-made units

The `systemctl disable` and backup steps came with WR-02 (91f341f). The page
half, **ae3357f**: the update-unit and timer notes end "They replace a unit
(units) of the same name; systemctl cat ... shows what is there." Red: the new
text pinned against the old component, exit 1, 2 failures.

## Skipped Issues

### IN-03: read-only bind on the helper's script

**File:** `deploy/holzkube-manager-update.service`
**Reason:** not cheap in the sense that matters here. The line itself is one
line and an allow-list entry, but it is one more sandbox line that has never
run as root under systemd, which is exactly what WR-02 is about; the review
itself says it should be measured with the same real run. The test that keeps
the script from naming the helper still holds. It belongs with the first real
sandboxed run (human item below).

### IN-05: a manual check during a real install ends in a bare "failed"

**File:** `deploy/holzkube-manager-update.service`, `docs/api-contract.md`
**Reason:** not cheap: treating "update unit active" as busy needs a new
reading of the unit's state (the daemon has no D-Bus and reads files only),
or a new page state. The behaviour is documented in HOST-HELPER.md, the window
is a real install's, and the review marks it optional.

## Human items (the operator's call)

1. **The first real run through the shipped sandbox, as root.** Replace the
   hand-made units following HOST-HELPER.md "The hourly update" (copy first,
   disable the service, the block), then "Check one real run". The run that
   matters most is the first one that installs: until one has, the sandbox's
   `install` and `systemctl restart` have not been seen as root. IN-03's line
   and the omitted CapabilityBoundingSet=/SystemCallFilter=/ProtectProc= wait
   for that measurement.
2. **The new script on the production host arrives by itself** with the next
   healthy update (it replaces itself). From then on every hourly run asks
   `check_host` first; on the reference layout that passed read-only here.
   If the journal of `holzkube-manager-update` ever shows `FEHLER:
   holzkube-manager.service ...`, that is the new refusal, with its reason.
3. `deploy/holzkube-manager.service` is for new installations; the operator's
   own unit stays as it is.

## Final verification (Pi, main checkout, exit codes read from the command itself)

- `./bin/task ci`: first run exit 201 -- `lint:go` (golangci-lint 2.13.1,
  `unused`): `script_test.go:442:6: func releaseArchive is unused`, left by
  WR-02's archive helper. Fixed in **594f530**. Second run: **exit 0** --
  biome lint, vitest 61 files / 868 tests, build, `0 issues.`, `go test ./...`
  (go1.27.1 linux/arm64) and `test:next`, 82 `ok` lines and no FAIL, the
  layout audit. `internal/upgrade TestANodeSaysHowItBooted` did not time out
  this time.
- `go test ./internal/publicrepo/` exit 0 before each fix commit (and after
  e37de8a, the gofmt-only commit, run right after it).
- TestUnitsVerify ran (systemd-analyze, systemd 257), with the new daemon unit
  among the six and its own negative control.
- No rendered screen changed: both /host fixtures carry `update_timer: []` and
  `update_unit: []`, so the new note never shows in a picture; nothing was
  re-rendered or committed under docs/screenshots.

## Host record

| | before (07:11 CEST) | after (08:10 CEST) |
|-|--------|-------|
| holzkube-manager.service ActiveEnterTimestamp | Tue 2026-09-29 20:25:38 CEST | same |
| NRestarts / MainPID | 0 / 1149 | 0 / 1149 |
| holzkube-manager-update.timer | enabled, active; last 07:04:10 | enabled, active; last 08:04:40 (its own hourly run: "Bereits aktuell.") |
| /etc/systemd/system/holzkube-manager-update.{service,timer} | -- | `cmp`-identical to the 13-16 copies |
| /usr/local/bin/holzkube-managerd, /usr/local/sbin/holzkube-manager-update | Sep 27 10:50 | unchanged |

---

_Fixed: 2026-10-02_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
