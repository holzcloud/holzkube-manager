---
phase: 11-host-seite
fixed_at: 2026-09-28T20:30:00Z
review_path: .planning/phases/11-host-seite/11-REVIEW.md
iteration: 1
findings_in_scope: 9
fixed: 9
skipped: 0
status: all_fixed
---

# Phase 11: Code Review Fix Report

**Fixed at:** 2026-09-28T20:30:00Z
**Source review:** .planning/phases/11-host-seite/11-REVIEW.md
**Iteration:** 1

**Summary:**
- Findings in scope (critical_warning): 9
- Fixed: 9
- Skipped: 0
- Also asked for, if trivial: IN-01 fixed, IN-05 skipped (not trivial)

**Where verification ran.** The main checkout on the operator's Pi (aarch64), with `workflow.use_worktrees: false`, so no worktree was used. Go ran as `GOTOOLCHAIN=go1.26.7`, without `-race`. The root cases of the update script ran in an unprivileged user namespace (`unshare --user --map-root-user`). Nothing touched `holzkube-manager.service`, `/var/lib/holzkube-manager` or `/usr/local/bin`. Nothing was pushed or released.

**Guards.** Every fix below comes with a regression test. Each test was run against the reinstated fault and seen to fail, then seen to pass with the fix. The commit messages quote the red output.

**Final gates, run from the main checkout. Each exit code was read from the command itself:**
- `go test ./internal/host/... ./internal/httpapi/... ./internal/config/ ./internal/publicrepo/`: rc=0
- `npm --prefix web run test`: rc=0, 55 files, 537 tests
- `./bin/task lint:go`: rc=0, 0 issues

## Fixed Issues

### CR-01: Root-run `python3 -` / `python3 -c` imports modules from the caller's working directory

**Files modified:** `deploy/holzkube-manager-update.sh`, `internal/host/updatestatus/script_test.go`
**Commit:** 41c5caf
**Status:** fixed
**Applied fix:**
- Every `python3` call now runs with `-I`.
- The script does `cd /` once at the top. Before that it resolves `SELF=$(readlink -f "$0")`, because `$0` may be relative and the self-replace and `--help` use it.
- A new script_test case plants `json.py` and `datetime.py` in the caller's directory and on PYTHONPATH. It also has the curl stub record its working directory.

**Seen red:**
- Without both fixes: `cwd json`.
- Without `-I` alone: `PYTHONPATH json`.
- Without `cd /` alone: `curl ran in /tmp/...`.

### WR-01: Network rates are divided by a window stamped before the data-directory walk

**Files modified:** `internal/host/collector.go`, `internal/host/network_test.go`
**Commits:** 58500c0, plus the test fake adjusted in acf520d
**Status:** fixed: requires human verification (logic)
**Applied fix:**
- `readLive` now reads the clock immediately before the link and `/proc/stat` counters, instead of taking the request's start time.
- `TestNetworkRateWindowExcludesTheSizeWalk` makes the walk move a manual clock by 2 s.

**Seen red:** `rates_over_seconds = 5, want 3`, with eth0 at 60000 B/s instead of 100000. Re-checked after the WR-08 change.

### WR-02: Interfaces with operstate `unknown` are shown as "down"

**Files modified:** `internal/host/network.go`, `internal/host/network_test.go`, `internal/host/testdata/pi5/.../flags` (7 new fixture files), `web/src/api.ts`, `web/src/routes/host.tsx`, `web/src/routes/host.test.tsx`, `web/fixtures/demo.json`, `docs/api-contract.md`
**Commits:** eabc3c2, 922d2ba (gofmt)
**Status:** fixed: requires human verification (logic, and a wire-shape change)
**Applied fix:**
- operstate still decides when it knows. Only `up` counts as up; every other state is down.
- For `unknown`, empty or unreadable operstate, the `flags` file's IFF_UP decides.
- When neither file can be read, `up` is now `null`. Go has `*bool`, zod has `.nullable()`, and the row says "state not readable".
- The pi5 fixture gets the flags files the Pi reports (lo is 0x9). demo.json and the contract no longer show lo as down.
- The README's `host.png` was not re-rendered, because the virtual interfaces are collapsed there.

**Seen red:**
- Go, with operstate alone deciding: `TestNetworkLinkState` and `TestNetworkPi5` fail.
- UI, without the null branch: the new host.test case fails.
- Schema, back to `z.boolean()`: "expected boolean, received null".

### WR-03: The darwin seam gives readable empty lists and `read-failed` instead of `unsupported`

**Files modified:** `internal/host/sys.go`, `internal/host/sys_other.go`, `internal/host/collector.go`, `internal/host/collector_test.go`
**Commit:** 93a084e
**Status:** fixed: requires human verification (logic)
**Applied fix:**
- The non-Linux `osSys` now carries an `unsupportedPlatform` marker.
- On that platform, `Read` answers every reading as `unsupported` and states only the process's own version, start time and data directory path.
- A compile-time assertion keeps the marker on the darwin `osSys`.

**Seen red:**
- With the gate disabled, sensors and network came out readable, and the rest came out `read-failed` or `update.not-recorded`.
- With the marker method removed, `GOOS=darwin go vet` fails.

### WR-04: A run killed by a signal records nothing

**Files modified:** `deploy/holzkube-manager-update.sh`, `internal/host/updatestatus/script_test.go`
**Commits:** 2adfd4e, 19526ca (test WaitDelay)
**Status:** fixed
**Applied fix:**
- `trap 'exit 130' INT`, `trap 'exit 143' TERM` and `trap 'exit 129' HUP`, placed next to the EXIT trap.
- The new cases signal the script's whole process group while a stub blocks.

**Seen red:** without the traps, "the script recorded no status" for all three signals.

### WR-05: The writer can record `installed` values the reader rejects, or ones that are stale

**Files modified:** `deploy/holzkube-manager-update.sh`, `internal/host/updatestatus/script_test.go`
**Commit:** f56ab0d
**Status:** fixed: requires human verification (logic)
**Applied fix:**
- `on_exit` now reads `installed` from the binary on disk at exit.
- `record_status` downgrades any outcome other than `failed` to `failed` when `installed` or `latest` is empty.
- Three new root cases: killed in the health loop, a new binary whose `--version` fails once installed, and a rollback when there was no binary before the run.

**Seen red:**
- Against the old script: `installed = "0.1.0", want "0.2.0"`, and the reader refused the other two files.
- With the downgrade alone removed, the failing `--version` case is refused again.

### WR-06: The ownership check on the status directory ignores its mode

**Files modified:** `deploy/holzkube-manager-update.sh`, `internal/host/updatestatus/script_test.go`
**Commit:** 9f4104b
**Status:** fixed
**Applied fix:**
- The status directory is now checked with `lstat`. It must be a directory, owned by uid 0, with no group or other write bit.
- New root cases for 0777, 0775, 0757 and 1777.
- The optional "write through mktemp's fd" part was not done. With only root able to write the directory, the swap it defends against is no longer possible.

**Seen red:** all four modes wrote `status.json` under the old check.

### WR-07: One file that disappears during the size walk fails the whole measurement

**Files modified:** `internal/host/filesystems.go`, `internal/host/filesystems_test.go`
**Commit:** e0cb20b
**Status:** fixed: requires human verification (logic)
**Applied fix:**
- ENOENT below the data directory is now skipped, both from the WalkDir callback and from `Info()`.
- The data directory itself missing is still an error.
- The test removes a file and a directory right after the listing and compares the result with `du` of what remains.

**Seen red:** with the old walk, and with either of the two skips removed on its own.

### WR-08: A data directory reached through a symlink is attributed to the wrong filesystem

**Files modified:** `internal/host/collector.go`, `internal/host/filesystems.go`, `internal/host/filesystems_test.go`, `internal/host/network_test.go`, `docs/api-contract.md`
**Commits:** acf520d, 3be412d (revive rename)
**Status:** fixed: requires human verification (logic)
**Applied fix:**
- The resolution happens inside the collector through its `fs.FS`, not in main.go, so the guard covers the whole path.
- `resolvePath` follows each symlink component with `fs.Lstat`/`fs.ReadLink`, up to 40 hops. `readLive` looks up the mount and runs statfs on the resolved path.
- `data_dir.path` still shows the configured path. A path that cannot be resolved falls back to the lexical one.
- The contract text now says how the data directory's filesystem is found.
- The WR-01 test fake now passes `Lstat`/`ReadLink` through without moving its clock.

**Seen red:** with the resolution disabled, the absolute, relative and chained cases each got one merged row on `/`.

### IN-01 (Info, requested if trivial): The comment on `maxLinks` has the truncation order backwards

**Files modified:** `internal/host/network.go`
**Commit:** 1c0aac8
**Applied fix:** The comment was corrected to say the truth: the cut is by name, before physical and virtual are told apart, so wlan0 would be dropped before the veths. The behaviour is unchanged, so there is no guard.

## Skipped Issues

### IN-05 (Info, requested if trivial): Sensor and fan React keys collide for chips with the same name

**File:** `web/src/components/charts/Sensors.tsx:55-57, 92, 144`
**Reason:** Not trivial. `sensorKey` (`chip/label`) is the React key, and it is also the sparkline history identity (`temp:${sensorKey(t)}` in NodeHardware). Two NVMe drives therefore share one history as well as one key. An index-based React key would hide the warning and leave the merged history in place. A real fix needs a stable per-sensor id from the server, such as the hwmon index.
**Original issue:** The key is `${chip}/${label}`. Two NVMe drives or two drivetemp chips produce duplicate keys.

---

_Fixed: 2026-09-28T20:30:00Z_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
