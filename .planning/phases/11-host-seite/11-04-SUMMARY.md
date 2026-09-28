---
phase: 11-host-seite
plan: 04
subsystem: api, ui, config
tags: [go, statfs, mountinfo, du, singleflight, update-status, react, zod]

requires:
  - phase: 11-02
    provides: "updatestatus.Read, ErrNotRecorded, DefaultPath; the script's STATUS_DIR default"
  - phase: 11-03
    provides: "parseMountinfo, topmostAt, the procsubset-pid fixture with the data-dir bind mount, Live, the /host Live section"
provides:
  - "internal/host/filesystems.go: Filesystem, FSUsage, filesystems() deduped by major:minor, fsUsage in df arithmetic, DirSize, dirSizer (60 s cache, singleflight, 5 s deadline)"
  - "statBlocks in sys_linux.go (syscall.Stat_t) and sys_other.go (ok false)"
  - "View.Service {version, started_at, uptime_seconds, data_dir{path,size}, update}; Live.Filesystems"
  - "Config.DataDir, Version, Started, UpdateStatusPath; composition-root wiring in main.go"
  - "--update-status-file / HOLZKUBE_MANAGER_UPDATE_STATUS_FILE, documented in docs/guide.md, tied to the script default by a test"
  - "/host Service card and Filesystems card; hostSchema.service and live.filesystems; demo fixture in the Pi's production state"
affects: [11-05, 11-06, 11-07, 12]

actuals:
  tokens: 18200
  tasks: 3
  commits: 3
plan_head_before: 01ffa64ba653702ad0a8ccce9b9fbbcbd048a928
plan_head_after: 8c96604689f2d4f2900ce453e9cc0f1f7f69c774

tech-stack:
  added: []
  patterns:
    - "Two paths share a filesystem only when mountinfo proves the same major:minor; without a mount table nothing is merged and no device is claimed"
    - "A cached measurement carries its measured_at, and a failed re-measure keeps serving the older good value with that age"
    - "A shared walk runs under context.WithoutCancel(request) plus its own deadline, so one closed tab cannot cancel what every waiter shares"

key-files:
  created:
    - internal/host/filesystems.go
    - internal/host/filesystems_test.go
    - internal/host/service_test.go
  modified:
    - internal/host/host.go
    - internal/host/collector.go
    - internal/host/collector_test.go
    - internal/host/sys_linux.go
    - internal/host/sys_other.go
    - internal/config/config.go
    - internal/config/config_test.go
    - docs/guide.md
    - cmd/holzkube-managerd/main.go
    - web/src/api.ts
    - web/src/routes/host.tsx
    - web/src/routes/host.test.tsx
    - web/fixtures/demo.json

key-decisions:
  - "The filesystem percent is df's Use% exactly: used / (used + available), rounded UP as gnulib df rounds. formatPercent's rounding gives 24% where df prints 25% on the measured numbers"
  - "A failed walk also counts as an attempt for the 60 s cache, so a slow or failing data directory is not re-walked on every 3 s poll"
  - "The data-dir row is stat'ed at the data directory itself, not at its mount point, which a later mount could cover"
  - "The update row's 'ago' is measured against observed_at, not the browser clock, so a stale reading does not age its own timestamps"
  - "An empty UpdateStatusPath in host.Config falls back to updatestatus.DefaultPath; config always fills it in production"

patterns-established:
  - "Host rows that are readings render MissingValue in place of the value, and the label line stays"

requirements-completed: [HOST-02, HMON-02, HOST-03]

coverage:
  - id: D1
    description: "/ and the data directory's filesystem, one row when they share major:minor (the production bind mount), two otherwise; prefix only at a / boundary; no mount table means no merge"
    requirement: HMON-02
    verification:
      - kind: unit
        ref: "internal/host/filesystems_test.go#TestFilesystemsDedupeByDevice, TestFilesystemsOnTwoDevices, TestFilesystemsPrefixBoundary, TestFilesystemsWithoutMountinfo"
        status: pass
    human_judgment: false
  - id: D2
    description: "Size, used and available byte-identical to df -B1; a failing statfs hides that row only"
    requirement: HMON-02
    verification:
      - kind: unit
        ref: "internal/host/filesystems_test.go#TestStatfsMatchesDf, TestFilesystemStatfsFails"
        status: pass
      - kind: manual_check
        ref: "throwaway real-kernel read on the Pi against df -B1 / (see 'Where this ran')"
        status: pass
    human_judgment: false
  - id: D3
    description: "Data-dir size equals du -s -B1 (hard link once, directories counted, symlink not followed), cached 60 s"
    requirement: HOST-02
    verification:
      - kind: unit
        ref: "internal/host/filesystems_test.go#TestDataDirSize (ran, not skipped, on the Pi), TestDataDirSizeIsCached, TestDataDirSizeFails"
        status: pass
    human_judgment: false
  - id: D4
    description: "Service section: version, started_at in UTC, process uptime, data dir; update recorded / not recorded (host or container sentence) / not readable, never invented"
    requirement: HOST-03
    verification:
      - kind: unit
        ref: "internal/host/service_test.go#TestServiceSection, TestUpdateStatusReadings, TestUpdateStatusPathIsConfigurable; collector_test.go#TestNoNulls, TestEveryReadingIsWellFormed"
        status: pass
    human_judgment: false
  - id: D5
    description: "--update-status-file: absolute only, settable by flag and env, in the guide, default tied to the script's STATUS_DIR and updatestatus.DefaultPath"
    requirement: HOST-03
    verification:
      - kind: unit
        ref: "internal/config/config_test.go#TestEveryOptionIsSettableByFlagAndByEnvironment, TestUpdateStatusFileMustBeAbsolute, TestUpdateStatusDefaultMatchesTheScript; readme_test.go#TestTheReadmeDocumentsEveryOption"
        status: pass
    human_judgment: false
  - id: D6
    description: "/host Service card and Filesystems card: outcome sentences, amber on failed/rolled-back only, mismatch line, Not recorded / Not readable, merged meter at 25%, two meters when split, one unreadable row beside a rendered one"
    requirement: HOST-02
    verification:
      - kind: automated_ui
        ref: "web/src/routes/host.test.tsx#the Service card, the Filesystems card"
        status: pass
      - kind: automated_ui
        ref: "./bin/task test:layout (/host at 390 px and 1280 px)"
        status: pass
    human_judgment: false

duration: 24min
completed: 2026-09-28
status: complete
---

# Phase 11 Plan 04: Service and Filesystems Summary

**/host now shows the service: the version `--version` prints, how long the process has run, the data directory with a du-exact size walked at most once a minute, the free space on its filesystem, and the update status. The update status shows what the update script recorded or, when it recorded nothing, "Not recorded", with no time or version filled in. A Filesystems card shows `/` and the data directory's filesystem as one meter when they share a device number, and its figures are the ones `df -B1` prints.**

## Where this ran

Everything ran on the operator's Raspberry Pi 5 (aarch64, Linux 6.18), with Go 1.26.7 and no `-race`, because ThreadSanitizer refuses this kernel's address space. `TestDataDirSize` **ran and passed** here; it was not skipped. It compares against GNU `du` 9.7.

I also ran one throwaway real-kernel check. A temporary test file ran the Collector with `os.DirFS("/")` and `OS()`, and I deleted the file before committing. Its result:
- **Filesystems:** it returned exactly one row, `/` on `/dev/mmcblk0p2` (ext4) with roles `root · data directory`, and its three numbers were byte-identical to `df -B1 /` taken at the same moment: `125260451840 31274614784 88820592640 27%`.
- **Data-dir size:** the size of a real directory matched `du -s -B1` (`7512064`).
- **Update status:** the row said `update.not-recorded`, because this Pi has no status file yet.

I did not touch the production service, `/var/lib/holzkube-manager` or `/usr/local/bin`.

## Performance

- **Duration:** about 24 min
- **Started:** 2026-09-28T15:26Z
- **Completed:** 2026-09-28T15:50Z
- **Tasks:** 3
- **Files:** 3 created, 13 modified

## Accomplishments

- **`filesystems()`** compares the root entry and the data directory's mount by `MajMin`. The production unit's bind mount (`179:2 /var/lib/holzkube-manager` beside `179:2 /`) therefore becomes one row labelled with the root entry.
  - `/var/lib/holzkube` does not capture `/var/lib/holzkube-manager`.
  - Without a mount table, no merge and no device is claimed.
  - `roles` and the slice are never null.
- **`fsUsage`** computes `size = blocks·frsize`, `used = (blocks−bfree)·frsize` (saturating) and `available = bavail·frsize`. The fstype comes from mountinfo, never from the statfs magic.
- **`dirSizer`** walks at most once per 60 s, with one walk at a time through `singleflight` and a 5 s deadline under `context.WithoutCancel`.
  - It sums `st_blocks·512` over every entry, directories included, counts each `(dev, ino)` once, and does not follow symlinks.
  - A failed walk serves the last good value with its `measured_at`. Without one, it gives a reason that names the directory, and a timeout says "the size walk took longer than 5 s".
- **`Service`** carries the version (the same variable `--version` prints), `started_at` in UTC, whole-second process uptime, the data directory, and `updatestatus.Read` mapped as follows:
  - `ErrNotRecorded` → `update.not-recorded`, with the host sentence or, when `detectContainer` says so, the container sentence;
  - any other error → `read-failed`, "The update status file exists but could not be read: …";
  - success → the recorded status.
- **`--update-status-file`** accepts absolute paths only and stores them cleaned. It has a row in the guide and two sentences under the table. `TestUpdateStatusDefaultMatchesTheScript` holds its directory equal to the script's `STATUS_DIR` default and its value equal to `updatestatus.DefaultPath`.
- **`main.go`:** `started := time.Now()` comes right after the subcommand dispatch. The data directory is made absolute with a wrapped error, and `Version`, `Started`, `DataDir` and `UpdateStatusPath: cfg.UpdateStatusFile` all go into `host.Config`.
- **Service card**, beside Device, shows:
  - Version and Running for;
  - Data directory, with "{size} · measured {ago}";
  - Free space, taken from the row with the "data directory" role;
  - Update check: the outcome sentences, amber on `failed` and `rolled-back` only, "Checked {ago} · {time}", the installed/latest line with a null half left out, and the mismatch line (a leading `v` is ignored).
- **Filesystems card**, beside Memory, shows one dense meter per row:
  - the label is the mount in mono, plus the roles joined with " · ";
  - the meter's maximum is `used + available`, with warn/danger at 80/90 % of it;
  - the detail line is "{used} of {size} · {free} free · {device}";
  - an unreadable row shows its label and "Not readable".
- **Demo fixture:** it now carries the Pi's production state: one merged row with the measured numbers, and "update not recorded".

## Task Commits

1. **Task 1: filesystems deduped by device number, df arithmetic, du-sized data dir cached 60 s:** `030b9c3` (feat)
2. **Task 2: service section, `--update-status-file`, composition-root wiring:** `7ba0b9a` (feat)
3. **Task 3: Service and Filesystems cards on /host:** `8c96604` (feat)

## Fault injections (each seen red on the Pi, then restored and checked with `cmp` against a backup)

| # | Injection | Guard | Observed red |
|---|-----------|-------|--------------|
| T1a | `data.MountPoint == root.MountPoint` in place of `MajMin` | TestFilesystemsDedupeByDevice | `got 2 rows, want exactly 1 for one filesystem: [{Mount:/ … Roles:[root] …} {Mount:/var/lib/holzkube-manager Device:/dev/mmcblk0p2 FSType:ext4 Roles:[data directory] …` |
| T1b | freshness check replaced by `fresh := false` | TestDataDirSizeIsCached | `at 30 s: {Bytes:1064960 MeasuredAt:…10:00:33…}, want the first walk's {Bytes:16384 MeasuredAt:…10:00:03…} -- the walk ran again inside 60 s` |
| T1c | the `(dev, ino)` duplicate check removed | TestDataDirSize | `size = 720896, du -s -B1 says 409600` (the 300000-byte hard-linked file counted twice) |
| T2a | `ErrNotRecorded` mapped to `Read(Status{CheckedAt: time.Now(), Installed/Latest: version, Outcome: current})` | TestUpdateStatusReadings | `absent_on_a_host` and `absent_in_a_container`: `update = {Readable:true …} (value &{CheckedAt:2026-09-28 15:34:10… Outcome:current}), want not readable with a reason and no value` |
| T2b | config default moved to `/var/lib/holzkube-manager/update-status.json` | TestUpdateStatusDefaultMatchesTheScript | `the daemon reads the update status in /var/lib/holzkube-manager, the script writes it in /var/lib/holzkube-manager-update` and `the default is … updatestatus.DefaultPath is …` |
| T2c | `installed`/`latest` removed from `nullableKeys` | TestNoNulls | `recorded failure, two filesystems: null where a value or an empty list belongs: .service.update.value.latest` (this shows the new view really carries the null) |
| T3a | display `formatPercent((used / size) * 100)` (the UI-SPEC formula) | host.test.tsx, Filesystems card | `Unable to find an element with the text: 25%`; the meter drew `23%` (2 tests) |
| T3b | display `formatPercent(share(used, used + available))` (right denominator, ordinary rounding) | host.test.tsx, Filesystems card | the same `Unable to find … 25%`; the meter drew `24%` (2 tests) |

## Verification

- Plan Task 1 verify, all green: `go test ./internal/host -run 'TestFilesystems|TestStatfsMatchesDf|TestFilesystemStatfsFails|TestFilesystemKeysCoverInventory|TestDataDirSize|TestNoNulls|TestEveryReadingIsWellFormed' -v` exited 0, every test showed `--- PASS`, and there was no `--- SKIP`.
- Plan Task 2 verify: the named tests exited 0, and `go test ./internal/config ./cmd/holzkube-managerd ./internal/host/updatestatus` exited 0.
- `go test ./internal/host/... ./internal/config ./internal/httpapi ./cmd/holzkube-managerd ./internal/inventory ./internal/publicrepo/ -count=1` exited 0. httpapi took 295 s and inventory 141 s on the loaded Pi.
- `GOOS=darwin GOARCH=arm64 go build -o /dev/null ./cmd/holzkube-managerd` exited 0.
- `./bin/task lint:go` (the pinned v2.13.1, whole repository) exited 0 with 0 issues.
- `npm --prefix web run test`: 53 files and 515 tests passed. `typecheck` exited 0. `lint` exited 0; its 2 warnings and 1 info are pre-existing and sit in `DataTable.tsx` and `wall.test.tsx`.
- `./bin/task test:layout` exited 0, including `ok 390px /host (3 controls, 5 items)` and `ok 1280px /host`.

## Deviations from Plan

**1. [Recorded UI-SPEC deviation, decided in RESEARCH Open Question 1] The filesystem percent's denominator is `used + available`, not `size`**
- The UI-SPEC wrote `formatPercent(used/size)`. Success criterion 2 asks for the shell's value, and on the Pi df prints 25 % while `used/size` gives 23 %. The reason is that ext4's root reserve is in neither used nor available.

**2. [Rule 1 - Plan arithmetic] The percent is rounded up, not rounded to nearest**
- The plan and RESEARCH stated `used/(used+available)` = 24.5 % → 25 %. The real quotient is 24.05 %, which `formatPercent` rounds to "24%", so the plan's own "25%" assertion could not pass that way.
- gnulib df rounds Use% up whenever the division leaves a remainder. The real-kernel check confirmed this: 26.04 % printed as 27 %.
- The meter's figure is therefore `dfPercent` = `ceil(used·100/(used+available))`. Fault T3b is the proof that this rounding matters.

**3. [Rule 1 - Test scope] The Device card's row-order test is scoped to the Device card**
- `getAllByRole('term')` over the whole screen would now also collect the Service card's rows. The assertion is unchanged; it now runs inside `cardOf('Device')`.

**4. [Rule 2] The page never draws a Free space line without a row to back it**
- The server always sends the data directory's row when a data directory is configured. If an answer ever lacks it, Free space shows "Not readable" with "The answer names no filesystem for the data directory." rather than an empty cell. This sentence is written in the browser, not sent by the server; it is the only one like that on the card.

**5. [Additions] Tests beyond the named list**
- `TestFilesystemsOnTwoDevices` goes through the whole Collector with a MapFS mount table.
- `TestDataDirSizeFails` covers a missing directory: the reason names it, and no 0 is shown.
- `TestUpdateStatusPathIsConfigurable` checks that a non-default path is read.
- A third `readingViews` shape (a recorded failed run on two filesystems) makes `TestNoNulls` and `TestEveryReadingIsWellFormed` see `latest: null` and a second filesystem row.

## Known Stubs

None. Every value on both cards comes from the answer.

## Next Phase Readiness

- Plan 05 (sensors) adds its card to the right column of the Live grid. The left column now holds Processor, then Memory beside Filesystems.
- Plan 07 should add `service` and `live.filesystems` to the wire description in `docs/api-contract.md` ("The machine holzkube-manager runs on"). That section still lists only the route and the reason codes.
- **On the operator's Pi, the Update check will say "Not recorded" until a release carrying the recording update script has been installed by the timer.** This is expected and is what the guide now says.

## Self-Check: PASSED

- `internal/host/filesystems.go`, `filesystems_test.go` and `service_test.go` exist.
- `git log` has commits `030b9c3`, `7ba0b9a` and `8c96604`.
- `git rev-list --count 01ffa64..8c96604` = 3.
