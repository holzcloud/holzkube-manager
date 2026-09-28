---
phase: 11-host-seite
reviewed: 2026-09-28T17:50:39Z
depth: standard
files_reviewed: 55
files_reviewed_list:
  - README.md
  - cmd/holzkube-managerd/budget_test.go
  - cmd/holzkube-managerd/main.go
  - deploy/holzkube-manager-update.sh
  - docs/api-contract.md
  - docs/guide.md
  - go.mod
  - internal/config/config.go
  - internal/config/config_test.go
  - internal/host/collector.go
  - internal/host/collector_test.go
  - internal/host/filesystems.go
  - internal/host/filesystems_test.go
  - internal/host/helpers_test.go
  - internal/host/host.go
  - internal/host/identity.go
  - internal/host/identity_test.go
  - internal/host/live_test.go
  - internal/host/mountinfo.go
  - internal/host/mountinfo_test.go
  - internal/host/network.go
  - internal/host/network_test.go
  - internal/host/proc.go
  - internal/host/proc_test.go
  - internal/host/realkernel_test.go
  - internal/host/sensors.go
  - internal/host/sensors_test.go
  - internal/host/service_test.go
  - internal/host/sys.go
  - internal/host/sys_linux.go
  - internal/host/sys_other.go
  - internal/host/updatestatus/script_test.go
  - internal/host/updatestatus/status.go
  - internal/host/updatestatus/status_test.go
  - internal/httpapi/endtoend_test.go
  - internal/httpapi/handlers/host.go
  - internal/httpapi/hostapi_test.go
  - internal/httpapi/router.go
  - internal/inventory/hardware.go
  - internal/talos/sensors.go
  - internal/talos/sensors_test.go
  - web/fixtures/demo.json
  - web/scripts/layout-audit.mjs
  - web/scripts/readme-images.mjs
  - web/src/App.tsx
  - web/src/api.ts
  - web/src/components/NodeHardware.test.tsx
  - web/src/components/NodeHardware.tsx
  - web/src/components/Sidebar.tsx
  - web/src/components/charts/Sensors.test.tsx
  - web/src/components/charts/Sensors.tsx
  - web/src/fixtures.test.ts
  - web/src/routes/host.browser.test.tsx
  - web/src/routes/host.test.tsx
  - web/src/routes/host.tsx
findings:
  critical: 1
  warning: 8
  info: 5
  total: 14
status: issues_found
---

# Phase 11: Code Review Report

**Reviewed:** 2026-09-28T17:50:39Z
**Depth:** standard
**Files Reviewed:** 55
**Status:** issues_found

Session ran on the operator's Pi (aarch64). `go test ./internal/host/...` is green here, and `GOOS=darwin go vet ./internal/host/` is clean. Where a finding says "verified", it was run in this session; everything else comes from reading the code.

## Summary

I reviewed the new `internal/host` package (the collector, the readers, the syscall seam, the update-status reader), the `GET /api/v1/host` route, the changes to the root-run update script, and the `/host` page with its zod schema.

The Reading[T] → JSON → zod → UI chain holds for the values that pass through it. `readable:false` carries no `value` key, the zod discriminated union makes the renderer branch before it formats, and no `.default(0)` sits inside a reading. The trap in the update script keeps the exit code (verified: `exit 3` with a failing command inside the trap still exits 3). Bounded reads are applied consistently. No real identifiers from the operator's installation appear in the diff, fixtures, testdata or demo.json. I grepped for the live unit's hosts, domain and LAN range, MAC patterns, home directories and mail addresses.

The defects are elsewhere:
- **Root-run Python in the update script can be hijacked.** It imports modules from the caller's current directory.
- **Wrong or invented values reach the page in places the Reading type does not cover:**
  - Network rates are computed over a window stamped before a walk that can take up to 5 s.
  - `lo` and every interface whose operstate is `unknown` are shown as "down".
  - On darwin, sensors and network come out as readable empty lists instead of `unsupported`.
  - A data directory that is a symlink is attributed to the wrong filesystem.
- **The update script's status record can be stale or disagree with the reader.** A run killed by a signal records nothing. `installed` can be the version from before the run after the binary was already replaced. The writer can emit `installed: null` for outcomes the reader rejects.

## Structural Findings (fallow)

No structural pre-pass was provided.

## Narrative Findings (AI reviewer)

## Critical Issues

### CR-01: Root-run `python3 -` / `python3 -c` imports modules from the caller's working directory

**File:** `deploy/holzkube-manager-update.sh:138-148` (new: `import datetime, json, sys`). The same pattern already existed at lines 264 and 288 (`import json`).
**Issue:** `python3 -` and `python3 -c` put the current working directory at the front of `sys.path`. The script never changes directory and runs as root. So an operator who runs `sudo holzkube-manager-update` (or `--check`) from a directory another local user can write to, such as `/tmp` or a shared checkout, executes that user's `datetime.py` or `json.py` as root. I verified this in this session: a `datetime.py` in the current directory is imported by `echo 'import datetime' | python3 - x`, and is not imported with `python3 -I`. `os` and `sys` are frozen or builtin and cannot be shadowed, so the uid check on line 128 is safe. The status write adds a new root import path (`datetime`), and it runs on every exit. The hourly timer runs from `/` and is not exposed; manual sudo runs are.
**Fix:** Run every Python call in isolated mode, and leave the caller's directory once at the top:
```bash
set -euo pipefail
cd /            # nothing below depends on the caller's directory
...
if ! python3 -I - "$outcome" "${INSTALLED:-}" "${LATEST:-}" > "$tmp" <<'PY'
...
LATEST_JSON=$(printf '%s' "$RELEASES_JSON" | python3 -I -c "...")
read -r TAG ... < <(printf '%s' "$LATEST_JSON" | python3 -I -c "...")
python3 -I -c 'import os, sys; ...' "$dir"
```
Add a script_test case that puts a `datetime.py` in `cmd.Dir` which writes a marker file, and assert the marker is absent.

## Warnings

### WR-01: Network rates are divided by a window stamped before the data-directory walk

**File:** `internal/host/collector.go:112-121, 191, 202`; `internal/host/filesystems.go:193-213`
**Issue:** `Read` takes `now` first, then runs `readService`. That call walks the data directory synchronously, for up to `dirSizeDeadline` (5 s) once every 60 s. Only after that does `readLive` read `/proc/stat` and `/sys/class/net/*/statistics`. The counters are stored as `counters{at: now}`, stamped before the walk, and `network()` divides the byte delta by `cur.at - prev.at`. So:
- In the poll that includes a walk, the counters span `window + walkDuration` but are divided by `window`. Throughput is over-reported.
- In the poll after it, the counters span `window - walkDuration`. Throughput is under-reported.

With the 3 s poll and a 2 s walk on an SD card, that is about +66 % and then -66 %, once a minute. `rates_over_seconds` is wrong by the same amount. CPU % is not affected, because it is a ratio of kernel counters. No test covers this, because the fake FS walk takes no time.
**Fix:** Take the rate clock when the counters are read, not at the start of the request:
```go
func (c *Collector) readLive(now time.Time) Live {
    ...
    links, linkErr := readLinks(c.cfg.FS)
    sampledAt := c.cfg.Now()           // the counters' own instant
    cur := counters{at: sampledAt, links: linkCounters(links)}
    ... read /proc/stat immediately after, before anything slow ...
```
Alternatively, move `readService` (and its walk) after `readLive`. Pin the fix with a test whose fake FS sleeps inside the walk.

### WR-02: Interfaces with operstate `unknown` (loopback, WireGuard, many USB NICs) are shown as "down"

**File:** `internal/host/network.go:354-356`; `web/src/routes/host.tsx:516-521`
**Issue:** `Up` is true only when `operstate == "up"`. The kernel reports `unknown` for loopback and for drivers without carrier reporting (tun/wg, some USB Ethernet). On this Pi, `/sys/class/net/lo/operstate` is `unknown` while `flags` is `0x9` (IFF_UP). The page therefore shows `lo · down` while the same row shows traffic. The demo fixture and the api-contract example both encode `"lo", "up": false` with non-null rates, and `network_test.go:113` asserts `link("lo", false, nil)`. The test pins the bug. An unreadable operstate also becomes `false`, which is a fabricated reading under D-02.
**Fix:** Derive administrative state from `flags & IFF_UP` (0x1) and keep operstate as the carrier signal. Alternatively, send the operstate string and let the UI say "up (state unknown)". Treat an unreadable state as absent, not as down:
```go
if raw, ok := readTrimmed(fsys, dir+"/flags"); ok {
    if f, err := strconv.ParseUint(strings.TrimPrefix(raw, "0x"), 16, 32); err == nil {
        s.link.Up = f&0x1 != 0
    }
}
```
Then fix the fixture, the contract example and `network_test.go:113`.

### WR-03: The darwin seam does not give `unsupported`; it gives readable empty lists and `read-failed`

**File:** `internal/host/sys_other.go:10-12`; `internal/host/sensors.go:127-131`; `internal/host/network.go:331-335`; `internal/host/collector.go:193-195, 298-315`
**Issue:** The seam says that on non-Linux "every section of the page is 'not readable', with the reason saying why". The api-contract says `unsupported` covers "a darwin build". Only the four syscalls return `errUnsupported`. The file reads still go through `os.DirFS("/")`, so on darwin:
- `/sys/class/hwmon` and `/sys/class/net` return ENOENT. `classEntries` and `readLinks` map ENOENT to `nil, nil`, so sensors and network are **readable empty lists**. The page says "This machine reports no temperature sensors" and "No physical network interface found". Those are readable claims about a machine that was never looked at.
- `/proc/stat`, `/proc/meminfo` and `/proc/loadavg` are ENOENT without `subset=pid`, so they come out as `read-failed` with "open proc/stat: no such file or directory", not `unsupported`.
- Model and cores come out as `read-failed` against `/sys/...`.
**Fix:** Put a platform gate in the collector. On non-Linux, set every file-backed reading to `Hidden(Reason{Code: CodeUnsupported, ...})`. For example, add a `supported bool` to `Sys` (`osSys{}.Linux()`), or have `readLive`/`readDevice` return early when `runtime.GOOS != "linux"` and the FS is the production one. Add a test with a `Sys` that returns `errUnsupported` everywhere, and assert that no reading on the page comes out readable.

### WR-04: A run killed by a signal records nothing and leaves the previous status on the page

**File:** `deploy/holzkube-manager-update.sh:163-170, 193`
**Issue:** Bash runs the EXIT trap on SIGTERM, but `$?` inside the trap is then the status of the last completed command, usually 0, not 143. I verified this in this session with `trap t EXIT; sleep 5` killed by SIGTERM: the trap saw `rc=0` and the process exited 143. When OUTCOME is still empty, `record_status` returns without writing. This happens on `systemctl stop holzkube-manager-update`, on shutdown during the hourly run, or on Ctrl-C of a manual run. It includes a kill during the health loop after `install` and `systemctl restart`. The status file keeps the previous run's `current`/`available` with the old version, while a different binary may now be installed and nothing was rolled back.
**Fix:** Trap the signals explicitly, so the trap knows the run was interrupted:
```bash
on_signal() { OUTCOME=${OUTCOME:-failed}; exit 130; }
trap on_signal INT TERM HUP
trap on_exit EXIT
```
Add a script_test case that sends SIGTERM to the script while the `sleep` stub blocks, and asserts `outcome == failed`.

### WR-05: The writer can record `installed` values the reader rejects, or ones that are stale

**File:** `deploy/holzkube-manager-update.sh:197-198, 378-379, 399-406, 417-422`; `internal/host/updatestatus/status.go:230-239`
**Issue:** The reader, the contract and the zod schema all say `installed`/`latest` are `null` **only** for `failed`. The writer does not keep that promise:
- Line 406: `INSTALLED=$(... ) || INSTALLED=""` sets `installed` to null with `outcome=updated` when `--version` fails. The reader then refuses the whole file, and the page shows "Not readable" instead of "Updated".
- If `BIN` was not executable before the run (`LOCAL_VERSION=keine`) but an older `PREVIOUS` exists, the rollback path records `rolled-back` with `installed: null`. That file is also rejected.
- `INSTALLED` is set once, before any change. Suppose any command fails under `set -e` after line 378 has replaced the binary: `systemctl restart` returning non-zero, or the self-replacing `install` on line 402 failing after a **healthy** update. The run is then recorded as `failed` with the **old** version as installed. The page says "The last update run failed; X is still installed", and that is false.
**Fix:** In `on_exit`, set `INSTALLED` from the binary that is on disk at exit, not from the snapshot taken before the run:
```bash
on_exit() {
  local rc=$?
  set +e
  [[ -n ${TMP:-} ]] && rm -rf "$TMP"
  local now; now=$("$BIN" --version 2>/dev/null | awk '{print $NF}')
  [[ -n $now ]] && INSTALLED=$now
  record_status "$rc" >/dev/null 2>&1 || true
}
```
In `record_status`, downgrade to `outcome=failed` whenever `INSTALLED` is empty, so the writer can never produce a file its own reader refuses. Add a failing-`--version` case to `TestUpdateScriptAsRoot`.

### WR-06: The ownership check on the status directory ignores its mode; root still writes through a path others can swap

**File:** `deploy/holzkube-manager-update.sh:125-138`
**Issue:** As root, the only check is `st_uid == 0`. A directory that already exists, is owned by root, and is group- or world-writable without the sticky bit (created by something else, or chmod'ed by hand) passes. `mktemp` creates the temporary file with O_EXCL. The later `> "$tmp"` is an ordinary O_TRUNC open that follows symlinks, though. Another user who can write to the directory can replace `.status.XXXXXX` with a symlink between the `mktemp` and the redirect, and root then truncates and writes the target of that symlink. The `install -d -m 0755` only applies when the script creates the directory itself. This is the same symlink trap D-14 moved the file to avoid.
**Fix:** Refuse any directory that anyone but root can write to:
```bash
python3 -I -c 'import os,stat,sys; s=os.lstat(sys.argv[1]); sys.exit(0 if stat.S_ISDIR(s.st_mode) and s.st_uid==0 and not s.st_mode & 0o022 else 1)' "$dir" || return 0
```
Also write the JSON to the file descriptor `mktemp` opened, or do the whole write in Python with `os.open(..., O_WRONLY|O_NOFOLLOW)`. Add a case with a root-owned 0777 directory.

### WR-07: One file that disappears during the size walk fails the whole measurement

**File:** `internal/host/filesystems.go:251-261`
**Issue:** `walkSize` returns the first error from `WalkDir`, and the error from `d.Info()`. The data directory is written with temp-file-plus-rename (sessions, audit, store), so a file listed by `ReadDir` and gone before `Info()` returns ENOENT and aborts the walk. The first walk after start then shows "Not readable: Could not read /var/lib/...: lstat ...: no such file or directory" for 60 s. Later walks keep a stale `measured_at` without saying why. `du` ignores vanished entries.
**Fix:**
```go
if err != nil {
    if errors.Is(err, fs.ErrNotExist) { return nil } // vanished between list and stat, as du treats it
    return err
}
...
info, err := d.Info()
if errors.Is(err, fs.ErrNotExist) { return nil }
```

### WR-08: A data directory reached through a symlink is attributed to the wrong filesystem

**File:** `cmd/holzkube-managerd/main.go:558-561`; `internal/host/filesystems.go:78-82, 106-123`
**Issue:** `filepath.Abs` does not resolve symlinks, and `mountOf` matches the lexical path against mount points. Consider `/var/lib/holzkube-manager -> /mnt/ssd/hkm`, a common way to move state off an SD card. `mountOf` finds `/` (same MajMin as root), merges the rows, and runs `statfs` on `/`. "Free space" for the data directory then shows the SD card's free space instead of the SSD's. It is a plausible wrong number, which D-02 treats as worse than a missing one.
**Fix:** Resolve before matching: `dataDirAbs, err = filepath.EvalSymlinks(dataDirAbs)` in main.go (keep the unresolved path for display in `DataDir.Path`). Alternatively, compare `st_dev` from `stat(2)` of the data directory against the root's, as the api-contract already claims ("decided by device number (`stat(2)`'s `st_dev`)"). Today the code compares mountinfo MajMin instead.

## Info

### IN-01: The comment on `maxLinks` has the truncation order backwards

**File:** `internal/host/network.go:272-277, 340-342`
**Issue:** The comment says veths "sort last" and are the ones dropped. `wlan0` sorts after `veth*` ('w' > 'v'), so on a host over 512 interfaces a physical Wi-Fi link is dropped before the veths. The truncation happens before physical and virtual are told apart.
**Fix:** Classify first and truncate only the virtual list, or fix the comment.

### IN-02: The api-contract examples use `v`-prefixed versions the script never writes

**File:** `docs/api-contract.md` (status file example `"installed": "v0.1.0", "latest": "v0.1.1"`; host example `"version": "v0.1.0"`)
**Issue:** The script records `${TAG#v}` and the last field of `--version`, both without `v`. demo.json correctly uses `0.1.0`. The contract shows a shape production never emits.
**Fix:** Use `0.1.0` / `0.1.1` in the examples.

### IN-03: The config default repeats `updatestatus.DefaultPath` as a literal

**File:** `internal/config/config.go:281`
**Issue:** The same path exists in two Go places (plus the script). The test ties config to the script, not to the constant.
**Fix:** Set `def: updatestatus.DefaultPath`, or assert that the two are equal in config_test.

### IN-04: The model reason names the device tree on amd64 even when DMI failed

**File:** `internal/host/identity.go:244-263`
**Issue:** When both DMI files are unreadable (not merely empty), the reason is `Could not read /sys/firmware/devicetree/base/model: ... no such file`. On a PC, that sends the operator to a file that never existed there.
**Fix:** When the device tree is ENOENT, report the DMI error instead.

### IN-05: Sensor and fan React keys collide for chips with the same name

**File:** `web/src/components/charts/Sensors.tsx:55-57, 92, 144`; `internal/host/sensors.go:154-158`
**Issue:** The key is `${chip}/${label}`. Two NVMe drives (`nvme/Composite`) or two `drivetemp` chips produce duplicate keys: React warns and may reuse the wrong row between polls. Unlike the node path, the host path does not carry device model or serial to tell them apart.
**Fix:** Include the hwmon index in the key (for example, add `Chip: name` plus a stable `id: "hwmon3/temp1"`), or key by position within the group.

---

_Reviewed: 2026-09-28T17:50:39Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
