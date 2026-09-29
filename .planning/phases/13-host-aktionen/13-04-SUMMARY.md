---
phase: 13-host-aktionen
plan: 04
subsystem: host
tags: [host-actions, root-helper, bash, user-namespace, fault-injection, reader]
status: complete

requires:
  - phase: 13-host-aktionen
    plan: 01
    provides: deploy/holzkube-manager-host.sh, hostaction.ReadResult, the runHelper pattern
provides:
  - TestHostScript / TestHostScriptAsRoot (the root helper's 28-case matrix, run as root in a user namespace)
  - TestReadResult / TestReadResultPath (the reader's table, planted markers)
  - F9-F13 and reader injections (a)/(b) recorded red
affects: [13-05, 13-07, 13-09]

actuals:
  tokens: 7800
  tasks: 2
  commits: 2
plan_head_before: 42173433697badd78b0f48c8d0a7c33410b510ac
plan_head_after: 1be64ec0c0adbb59546958f19a8514a00a1495de

tech-stack:
  added: []
  patterns:
    - "Script matrix: copy refused without its overrides, sealed env incl. TMPDIR, 5-s CommandContext that kills the whole process group"
    - "Stand-in systemctl logs argv plus whether the order still exists at call time (consume-before-act made observable)"
    - "Reader refusal rows each plant a marker the error must not contain"

key-files:
  created:
    - internal/host/hostaction/script_test.go
    - internal/host/hostaction/result_test.go
  modified:
    - deploy/holzkube-manager-host.sh
    - internal/host/hostaction/result.go

key-decisions:
  - "An oversize order is journalled as `zu lang (mehr als 64 Byte)`: the script reads at most 65 bytes and does not know the file's real length, so it no longer states a number it did not measure"
  - "The age window gives two reasons: `veraltet` for older than 60 s, `aus der Zukunft` for more than 5 s ahead"
  - "ReadResult requires an absolute path (leading slash); a relative one was silently read although the doc and the error say absolute"
  - "The script's work directory follows TMPDIR, which the test points into its own temp dir, so a run killed by the timeout leaves nothing in the host's /tmp"

requirements-completed: [HACT-01, HACT-02, HACT-03, HACT-04, HACT-06]

duration: 9min
completed: 2026-09-29
---

# Phase 13 Plan 04: Script matrix and reader table Summary

**The root helper, run as root in a user namespace against a stand-in systemctl, carries out exactly the four orders after consuming them and refuses 20 other shapes (plus stale/future, a directory and a symlinked state dir) without putting their bytes in the journal. Each of F9-F13 turned the matrix red, and the daemon's reader refuses everything the script would not write.**

## Where this ran

This ran on the operator's Raspberry Pi 5 (aarch64), with Go 1.26.7 via GOTOOLCHAIN and **no -race** (ThreadSanitizer refuses the Pi 5 kernel's address space). `unshare --user --map-root-user id -u` printed `0`. **TestHostScriptAsRoot ran and was not skipped**: the verification run had 0 `--- SKIP` lines and 32 `--- PASS` lines for `-run TestHostScript`. Nothing was installed. The helper only ran as temp copies with its three overrides and a stand-in systemctl.

The production service was not touched. `systemctl show holzkube-manager.service -p ActiveEnterTimestamp` read `Mon 2026-09-28 18:06:00 CEST` both before (01:36Z) and after (01:44Z).

## Performance

- **Duration:** about 9 min (01:36Z to 01:45Z)
- **Tasks:** 2
- **Files:** 4 (2 created, 2 modified)

## Accomplishments

- `script_test.go`:
  - `newHostScriptEnv` refuses a copy that lacks any of the three overrides.
  - The environment is sealed: PATH, HOME, TMPDIR, LC_ALL=C, the overrides and the stand-in's log.
  - Every run has a 5-s `exec.CommandContext` with `Setpgid` and a `Cancel` that SIGKILLs the whole group, plus `WaitDelay`.
  - The stand-in logs its argv and `order-still-there` when the order exists at call time.
  - Every case checks the exit code, the stand-in log, whether the order is still there, and `last` read through `ReadResult(os.DirFS(stateDir), "/last")`, including mode 0644 and no `.last.*` leftovers.
- `TestHostScriptAsRoot` has 28 subtests:
  - the 4 valid orders, a failing systemctl, and no order;
  - 16 malformed shapes, each checked for its exact reason and byte count and for its marker being absent from the output;
  - a symlink to a **valid** order, a FIFO, a stale order and a future order;
  - a directory named host-order, and a symlinked state directory.
- `TestHostScript` runs as non-root: `bash -n` passes, and a run exits 1, records nothing, calls nothing and leaves the order.
- `result_test.go` has 27 table rows: 7 accepted, 19 refused, and 1 absent file giving ErrNoResult. `TestReadResultPath` covers one accepted absolute path, five refused path forms and a directory.

## Task Commits

1. **Task 1: the script's matrix (HACT-06, F9-F13)**, `058b535` (test)
2. **Task 2: the daemon reads only what the script writes (D-05, R6)**, `1be64ec` (test)

## Fault injections

For each one: the fault was put in, the test ran, the exit code was read from `go test` itself, and the file was restored from a saved copy. After every restore, `cmp` against the saved copy exited 0.

The script injections ran `go test ./internal/host/hostaction -count=1 -run TestHostScriptAsRoot -v`. The line numbers in the first pass are from before the 3-line TMPDIR comment went into `env()`. In the committed file they are 3 higher, which is why the F11 re-run reads `:491`.

| Fault | What was injected | rc | Red subtests and failing lines |
|---|---|---|---|
| F9 | Pattern line replaced by `read -r a i < "$work/o"; "$SYSTEMCTL" "$a"; exit 0` | 1 | **`rejects_foreign_word`**: `script_test.go:436: exit = 0, want 2`, `:438: systemctl was called, want never:` / `halt`, `:440: the script recorded nothing`. 18 subtests red in total: 11 rejects and stale/future called the stand-in, and the valid ones had no `last`. The shapes refused before the pattern (no newline, NUL, twice, 100 bytes, empty) stayed green, as they should. |
| F10 | `iflag=nonblock` only, and `if [[ -L $ORDER ]]` changed to `if false` | 1 | **`rejects_a_symlink_to_a_valid_order`** was the only red subtest: `:463: exit = 0, want 2`, `:465: systemctl was called, want never`, `:467: last = {"0123456789abcdef" "reboot" "started"}, want {"" "" "rejected"}`, `:469: output lacks the symlink reason`, `:472: output quotes the link's target` |
| F11 | `iflag=nofollow` only, and `elif [[ ! -f $ORDER ]]` changed to `elif false` | 1 | **`rejects_a_FIFO (5.01s)`**: `script_test.go:488: the helper script did not finish within 5s -- something blocked it`. No `dd` process was left afterwards (the group kill worked). After the TMPDIR change it was re-run: red again (`:491`, 5.00s), with no leftover in /tmp. |
| F12 | The `rm` block moved after the systemctl call, plus `rm` before the failure exit | 1 | All 4 **`valid_*`** subtests, e.g. `:373: systemctl called with "reboot\norder-still-there\n", want exactly "reboot\n"` and `:373: systemctl was called while the order was still there`. Also **`systemctl_fails`** (`:387`, same). Every reject: `the order is still there, want it consumed` (`:439` for the table rows, `:466` symlink, `:493` FIFO, `:521` stale and future). Directory case: `:542: exit = 2, want 1`. 26 subtests red. |
| F13 | Both age-window lines removed | 1 | **`rejects_a_stale_order`** and **`rejects_a_future_order`**: `:518: exit = 0, want 2`, `:520: systemctl was called, want never`, `:522: last = {… "reboot" "started"}, want {… "reboot" "rejected"}`, `:524: output lacks "Auftrag verworfen: veraltet (24 Byte)"` / `"… aus der Zukunft (24 Byte)"` |

The reader injections ran `go test ./internal/host/hostaction -count=1 -run TestReadResult -v`.

| Fault | What was injected | rc | Red row and failing line |
|---|---|---|---|
| (a) | The `if r.Outcome == OutcomeStarted { return … }` block in the `-` branch removed | 1 | **`TestReadResult/-_with_started`**: `result_test.go:215: accepted {ID: Action: Outcome:started At:2026-09-28 09:37:01 +0000 UTC}, want a refusal naming "started order without an id"` |
| (b) | The Stat check `info.Size() > MaxResultSize` removed, `io.LimitReader` replaced by `io.ReadAll(f)`, and the `len(raw)` check removed | 1 | **`TestReadResult/above_MaxResultSize`**: `result_test.go:221: err = "the host helper's result line is not four fields separated by single spaces", want it to name "larger than"` |
| (b) single | Only the Stat check removed; separately, only the LimitReader and len check removed | 0 / 0 | Stays green, as the plan predicted: `fstest.MapFS`'s Stat reports the true size, so either check alone still refuses the file. **Neither is claimed as a measurement.** Both runs are recorded only to show that the injection injects nothing observable. |

Two fixes were seen red before they existed, because the tests were written first and run against the unchanged code:
- The script's `rejects_100_bytes` failed with `script_test.go:442: output lacks "Auftrag verworfen: zu lang (mehr als 64 Byte)"`, and `rejects_a_future_order` failed with `:524: output lacks "Auftrag verworfen: aus der Zukunft (24 Byte)"`.
- `TestReadResultPath` failed with `result_test.go:256: "srv/last": err = <nil>, want a refusal that is not ErrNoResult`.

## Verification (exit codes read from the command itself)

- `go test ./internal/host/hostaction -count=1 -run TestHostScript -v`: rc 0, 32 `--- PASS`, 0 SKIP.
- `bash -n deploy/holzkube-manager-host.sh`: rc 0. `test -x` also passes.
- `go test ./internal/host/hostaction -count=1 -run TestReadResult -v`: rc 0, 29 PASS.
- `go test ./internal/host/... ./internal/store/... -count=1`: rc 0. This includes `TestNoDirectFileAccessOutsideFsstore` in `internal/store/fsstore`.
- Plan verification, `go test ./internal/host/... ./internal/httpapi -run 'TestHostScript|TestReadResult|TestHostActionRoundTrip' -v`: rc 0, with no SKIP. The round trip also passes against the changed script.
- `golangci-lint run ./internal/host/hostaction/...`: 0 issues.
- `git diff --stat -- go.mod go.sum`: empty.
- `go test ./internal/publicrepo/`: ok.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] The journal stated a length it never measured**
- **Found during:** Task 1, case `rejects 100 bytes`
- **Issue:** A 100-byte order was journalled as `zu lang (65 Byte)`, which is what `dd` read and not how long the file was.
- **Fix:** It is now `reject "zu lang" "mehr als 64"`, which logs `zu lang (mehr als 64 Byte)`.
- **Files:** deploy/holzkube-manager-host.sh
- **Commit:** 058b535

**2. [Rule 1 - Bug] A future order was journalled as "veraltet"**
- **Found during:** Task 1, case `rejects a future order`
- **Fix:** The one combined window check is split into two: `veraltet` (older than 60 s) and `aus der Zukunft` (more than 5 s ahead). Both still record `<id> <action> rejected`. F13 removes both lines.
- **Files:** deploy/holzkube-manager-host.sh
- **Commit:** 058b535

**3. [Rule 1 - Bug] ReadResult read relative paths**
- **Found during:** Task 2, `TestReadResultPath`
- **Issue:** `strings.TrimPrefix` accepted `srv/last` as well, although the doc says "path is absolute" and the error says "not a clean absolute path". No caller passes a relative path; all use `DefaultResultPath` or an absolute override.
- **Fix:** Changed to `strings.CutPrefix(path, "/")`, which refuses a path without the leading slash.
- **Files:** internal/host/hostaction/result.go
- **Commit:** 1be64ec

**4. [Rule 2 - Hygiene] The script's work directory in tests**
- **Found during:** F11. The killed run left an empty `/tmp/tmp.*`, which was removed by hand.
- **Fix:** The test environment sets `TMPDIR` to the test's own temp directory. The plan's environment list did not name it. The script itself is unchanged; in production the unit's own tmp applies.
- **Files:** internal/host/hostaction/script_test.go
- **Commit:** 058b535

**Total deviations:** 4 auto-fixed (3 bugs, 1 hygiene). **Impact:** the journal and the reader now say only what is true. The script's structure (consume, validate, record, act) and its German comments are unchanged.

Notes:
- As the dispatch note said, the reader table refuses `-` only with `started`. `- - failed` is an accepted row, following 13-01 deviation 1.
- The matrix has 20 refused-shape subtests: 16 in the table, plus the symlink, the FIFO, stale and future. The plan's "18 malformed shapes" figure is covered.

## Known Stubs

None. This plan adds only tests and two small corrections.

## Threat Flags

None. No new surface: T-13-19..T-13-23 and T-13-26 are now each held by a test that was seen red.

## Next Phase Readiness

- Plan 05 (claim-rename, timer, process guard) can rely on `TestHostScriptAsRoot` to show that the script's side of R8/R9 stays as it is: the "withdrawn before pickup" exit is unchanged.
- Plan 07 (units) still owns PrivateDevices/TimeoutStartSec and `ReadWritePaths` (Pitfall 4). This matrix shows the script's `failed` path for an order it cannot remove.

## Self-Check: PASSED
