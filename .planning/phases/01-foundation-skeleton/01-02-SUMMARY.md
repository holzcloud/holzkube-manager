---
phase: 01-foundation-skeleton
plan: 02
subsystem: database
tags: [go, flock, syscall, mutex, compare-and-swap, atomic-write, fsync, crash-injection, tar, gzip, schema-migration, file-permissions, go-ast]

# Dependency graph
requires:
  - phase: 01-01
    provides: "the entity-shaped store interface, fsstore with the tmp -> chmod 0600 -> fsync -> rename -> fsync(dir) sequence, model records carrying Rev, and the composition root that opens the store"
provides:
  - "store.AcquireProcessLock: an exclusive non-blocking flock on the data directory, released by the kernel on kill -9"
  - "store.EntityLocks: one mutex per kind/id with refcounted teardown, so writers to different records never contend"
  - "store.ErrAlreadyRunning and store.TempFilePrefix as shared contract"
  - "fsstore.Guard: a start-time permission guard that collects every violation into one actionable error"
  - "fsstore crash-injection hook with three interruption points on the atomic write sequence"
  - "migrate.CurrentVersion / migrate.Run: forward-only schema migrations with refusal on a too-new or unparsable VERSION"
  - "backup.Create: a 0600 gzip tarball of the whole data directory, written before the first migration touches anything"
  - "TestNoDirectFileAccessOutsideFsstore: FOUND-07 enforced by an executed AST test instead of review"
affects: [01-03, 01-04, 01-05, 01-06, 03-inventory, 06-jobs, 07-patches, 09-upgrades]

actuals:
  tokens: 23000
  tasks: 3
  commits: 6

tech-stack:
  added:
    - "syscall.Flock (stdlib) — no new module dependency; go.mod is unchanged"
    - "archive/tar + compress/gzip (stdlib) for the pre-migration tarball"
    - "go/parser + go/ast (stdlib, test-only) for the architecture guard"
  patterns:
    - "Three layers of concurrency control: process flock, per-entity mutex, rev compare-and-swap"
    - "Per-entity lock keys are kind + \"/\" + id, with refcounted teardown so the map cannot grow without bound"
    - "Crash injection as a production-code hook armed only from a test file, so atomic.go never imports testing"
    - "An injected crash deliberately skips cleanup, because kill -9 runs no deferred cleanup either"
    - "Temporary records carry store.TempFilePrefix and are swept by Open before anything is read"
    - "Permission violations are collected, never repaired: silently chmod-ing hides that the files were exposed"
    - "Forward-only migrations with no reverse stub; the way back is the tarball, not a guess at how to undo"
    - "Architecture rules are enforced by AST tests that assert they scanned a plausible file count"

key-files:
  created:
    - internal/store/lock.go
    - internal/store/lock_test.go
    - internal/store/fsstore/permissions.go
    - internal/store/fsstore/permissions_test.go
    - internal/store/fsstore/atomic_test.go
    - internal/store/fsstore/concurrency_test.go
    - internal/store/fsstore/crashinject_test.go
    - internal/store/migrate/migrate.go
    - internal/store/migrate/migrate_test.go
    - internal/store/migrate/backup/backup.go
    - internal/store/testdata/README.md
  modified:
    - internal/store/store.go
    - internal/store/fsstore/fsstore.go
    - internal/store/fsstore/atomic.go
    - internal/httpapi/endtoend_test.go

key-decisions:
  - "backup lives in its own package internal/store/migrate/backup, because Go permits one package per directory and the plan's own call site is written backup.Create(...)"
  - "The permission guard refuses to start and never repairs: an automatic chmod would hide the window during which the secrets were readable"
  - "The crash hook is armed only from a test file, so atomic.go does not import testing into the shipped binary"
  - "An injected crash skips the deferred cleanup, because that is what makes the orphaned temp file — and therefore Open's sweep — real"
  - "os.MkdirAll and os.Stat are not forbidden by the architecture guard: creating a directory and asking whether a path exists are not accesses to a state record"
  - "The architecture guard exempts the whole internal/store subtree but re-checks store.go explicitly, so the seam itself cannot quietly start touching files"
  - "A missing VERSION with existing records means version 1 (a plan-01 directory), not version 0; no release ever wrote a version 0 layout"

patterns-established:
  - "Startup order in fsstore.Open is load-bearing and documented at the function: create dir -> flock -> permission guard -> temp sweep -> migration -> entity directories"
  - "Every new entity store takes the shared *store.EntityLocks and locks around the read-compare-write of its rev CAS"
  - "A new schema version is one appended Migration{From, To, Apply}; the surrounding path is already proven by fixtures"
  - "An architecture guard test must assert it actually scanned the tree, or it passes for the wrong reason"

requirements-completed: [FOUND-07, FOUND-08, FOUND-09]

coverage:
  - id: D1
    description: "A second process on the same data directory refuses to start, naming the directory and the pid holding the lock; the lock is released by the kernel on kill -9"
    requirement: "FOUND-08"
    verification:
      - kind: unit
        ref: "internal/store/lock_test.go#TestProcessLockExcludesSecondAcquire"
        status: pass
      - kind: unit
        ref: "internal/store/lock_test.go#TestProcessLockNamesTheHoldingPID"
        status: pass
      - kind: integration
        ref: "internal/store/fsstore/concurrency_test.go#TestProcessLockRefusesSecondOpen"
        status: pass
      - kind: manual_procedural
        ref: "two real holzkubed processes on one --data-dir: the second exits 1 with 'is locked by pid 65924'; after kill -9 of the first, a fresh instance starts"
        status: pass
    human_judgment: false
  - id: D2
    description: "Concurrent writes serialize per record: 50 puts on one id yield exactly one winner, 49 ErrConflict and exactly one rev increment, while 50 puts on distinct ids all succeed"
    requirement: "FOUND-08"
    verification:
      - kind: unit
        ref: "internal/store/fsstore/concurrency_test.go#TestConcurrentPutSameIDYieldsExactlyOneWinner"
        status: pass
      - kind: unit
        ref: "internal/store/fsstore/concurrency_test.go#TestConcurrentPutDistinctIDsAllSucceed"
        status: pass
      - kind: unit
        ref: "internal/store/lock_test.go#TestEntityLocksDoNotSerializeDifferentKeys"
        status: pass
      - kind: unit
        ref: "go test ./internal/store/... -count=1 -race"
        status: pass
    human_judgment: false
  - id: D3
    description: "A stale rev is rejected without touching the file, and a repeated identical put is byte-stable with no extra artifact in the directory"
    requirement: "FOUND-08"
    verification:
      - kind: unit
        ref: "internal/store/fsstore/concurrency_test.go#TestPutWithStaleRevIsRejectedNotOverwritten"
        status: pass
      - kind: unit
        ref: "internal/store/fsstore/concurrency_test.go#TestRepeatedPutIsByteStable"
        status: pass
    human_judgment: false
  - id: D4
    description: "An interruption at any of the three points of the atomic write sequence leaves exactly one whole record readable and no orphaned temporary that the next start could read as state"
    requirement: "FOUND-08"
    verification:
      - kind: unit
        ref: "internal/store/fsstore/crashinject_test.go#TestCrashInject (three subtests: duringTempWrite, afterTempWrite, afterRename)"
        status: pass
      - kind: unit
        ref: "internal/store/fsstore/crashinject_test.go#TestCrashInjectDuringTempWriteDoesNotTouchTheRecord"
        status: pass
      - kind: unit
        ref: "internal/store/fsstore/atomic_test.go#TestOpenSweepsOrphanedTemporaries"
        status: pass
    human_judgment: true
    rationale: "The three interruption points are injected in-process, not by an actual SIGKILL, and no test cuts power. That is the right trade for a unit suite — a real kill -9 cannot be aimed between fsync and rename deterministically — but it means the sequence is proven against the failure model, not against the hardware. Torn writes caused by the filesystem or the disk rather than by the process are out of reach here and would need fault injection at the block layer."
  - id: D5
    description: "The state carries a schema version; an upgrade writes a 0600 tarball before the first change and advances VERSION only after the last migration succeeds; a VERSION newer than the binary or unparsable refuses the start without writing anything"
    requirement: "FOUND-09"
    verification:
      - kind: unit
        ref: "internal/store/migrate/migrate_test.go#TestMigrateUpgradeBacksUpBeforeChangingAnything"
        status: pass
      - kind: unit
        ref: "internal/store/migrate/migrate_test.go#TestMigrateFailureLeavesTheOldVersionInPlace"
        status: pass
      - kind: unit
        ref: "internal/store/migrate/migrate_test.go#TestMigrateRefusesAVersionNewerThanTheBinary"
        status: pass
      - kind: unit
        ref: "internal/store/migrate/migrate_test.go#TestMigrateRefusesAnUnreadableVersion"
        status: pass
      - kind: integration
        ref: "internal/store/fsstore/concurrency_test.go#TestOpenRefusesASchemaVersionNewerThanTheBinary"
        status: pass
    human_judgment: true
    rationale: "Every branch of the migration path is executed, but against fixtures, because phase 1 defines exactly one schema version and therefore has no real migration. The machinery is proven; the first genuine data conversion is not, and cannot be until phase 3 appends one."
  - id: D6
    description: "A start against a data directory with permissions wider than 0700, or any state file wider than 0600, is refused with every violation named at once and a repair command"
    requirement: "FOUND-10"
    verification:
      - kind: unit
        ref: "internal/store/fsstore/permissions_test.go#TestPermissionGuardRejectsAWideOpenDirectory"
        status: pass
      - kind: unit
        ref: "internal/store/fsstore/permissions_test.go#TestPermissionGuardReportsEveryViolationAtOnce"
        status: pass
      - kind: unit
        ref: "internal/store/fsstore/permissions_test.go#TestPermissionGuardCoversBackupsAndTheLockFile"
        status: pass
      - kind: integration
        ref: "internal/store/fsstore/permissions_test.go#TestOpenRefusesAWideOpenDataDirectory"
        status: pass
      - kind: manual_procedural
        ref: "chmod 0755 on a real data dir: holzkubed exits with 'is mode 0755, want 0700' and the repair command"
        status: pass
    human_judgment: false
  - id: D7
    description: "No package above internal/store/fsstore reaches around the seam to the filesystem, enforced by an executed test rather than by review"
    requirement: "FOUND-07"
    verification:
      - kind: unit
        ref: "internal/store/fsstore/permissions_test.go#TestNoDirectFileAccessOutsideFsstore"
        status: pass
      - kind: other
        ref: "negative control: an os.ReadFile temporarily planted in internal/httpapi/web.go made the test fail at web.go:76, then was removed"
        status: pass
    human_judgment: false

# Metrics
duration: 18 min
completed: 2026-08-28
status: complete
---

# Phase 1 Plan 02: Store Hardening Summary

**The store went from "writes atomically" to "survives a second process, a `kill -9` between `fsync` and `rename`, a schema upgrade and a `0755` data directory" — three layers of concurrency control, forward-only migrations with a pre-migration tarball, a start-time permission guard, and FOUND-07 turned from a review rule into an AST test that fails the build.**

## Performance

- **Duration:** 18 min
- **Started:** 2026-08-28T01:25:00Z
- **Completed:** 2026-08-28T01:43:00Z
- **Tasks:** 3 (all TDD: RED then GREEN)
- **Files changed:** 20 (16 created, 4 modified)

## Accomplishments

- **The process `flock` closes the gap plan 01 recorded as open.** Two real `holzkubed` processes were run against one data directory: the second exited 1 with `is locked by pid 65924`. After `kill -9` of the first, a fresh instance started — the kernel releases the lock, so a crashed instance cannot wedge the directory.
- **Per-entity mutexes replaced the per-entity-*store* mutex.** The old `userStore.mu` serialized every user write against every other; the lock map keys on `kind + "/" + id`, so 50 writers to 50 records now run concurrently while 50 writers to one record still produce exactly one winner. The map is refcounted, so a long-lived process writing millions of session records does not accumulate a mutex per record.
- **Crash injection covers all three interruption points**, and — this is the part that makes it real — an injected crash deliberately skips the deferred cleanup, because `kill -9` runs no deferred cleanup either. The orphaned temp file it leaves is exactly what `Open`'s sweep must handle, so the test proves the sweep instead of assuming it.
- **The architecture guard is enforced, not asserted.** `TestNoDirectFileAccessOutsideFsstore` parses every non-test Go file under `internal/` and `cmd/` and fails on `os.ReadFile`-class calls outside the store layer. It was verified by negative control: an `os.ReadFile` planted in `internal/httpapi/web.go` made it fail at the right line. It also asserts it scanned at least 15 files, so a broken relative path cannot make it pass vacuously.
- **`go.mod` is unchanged.** flock via `syscall`, tarball via `archive/tar` + `compress/gzip`, the guard via `go/parser` — all stdlib. No new dependency, and therefore no new package-legitimacy surface.

## Task Commits

Each task ran RED then GREEN; no refactor commit was needed.

1. **Task 1 RED — three layers of concurrency control** — `4f400f0` (test)
2. **Task 1 GREEN — flock, entity lock map, rev CAS** — `67cf151` (feat)
3. **Task 2 RED — schema version, migration, backup** — `051993a` (test)
4. **Task 2 GREEN — `migrate.Run` and `backup.Create`** — `82a7920` (feat)
5. **Task 3 RED — permission guard, crash injection, architecture guard** — `b058b45` (test)
6. **Task 3 GREEN — `Guard`, `interruptAfter`, temp sweep, AST guard** — `03be623` (feat)

## Files Created/Modified

**`internal/store` — the seam**
- `lock.go` — `AcquireProcessLock` (flock `LOCK_EX|LOCK_NB`, pid recorded and fsynced) and `EntityLocks` with refcounted teardown
- `store.go` — added `ErrAlreadyRunning` and `TempFilePrefix`, the shared crash contract between `fsstore` (which creates temporaries) and `migrate` (which must not back them up)
- `lock_test.go` — exclusion, pid naming, release/re-acquire, `0600` lock file, same-key serialization, different-key and different-kind independence, map teardown

**`internal/store/fsstore` — the implementation**
- `fsstore.go` — `Open` now runs the documented startup order; every `Put` takes the per-entity lock around its rev CAS; records serialize through `marshalRecord` with fixed indentation
- `atomic.go` — `interruptPoint` with `duringTempWrite` / `afterTempWrite` / `afterRename`, `errInterrupted`, and a `crashed` flag that suppresses cleanup on an injected crash
- `permissions.go` — `Guard` (mask `0o077`, all violations collected, repair command in the message) and `sweepTempFiles`
- `concurrency_test.go`, `atomic_test.go`, `crashinject_test.go`, `permissions_test.go`

**`internal/store/migrate` — schema**
- `migrate.go` — `CurrentVersion`, `Migration`, `Run`, version reading with the empty/legacy distinction, atomic `VERSION` write
- `backup/backup.go` — `Create` writes a `0600` gzip tarball of the whole directory, excluding `backups/`, the lock file and temporaries, and fsyncs before returning
- `migrate_test.go`, `testdata/` (five fixtures + a README explaining what each one pins)

**`internal/httpapi`**
- `endtoend_test.go` — one added `os.Chmod(dir, 0o700)`; see deviation 3

## Decisions Made

- **`backup` is a package, not a file.** The plan lists `internal/store/migrate/backup.go` *and* writes the call as `backup.Create(dir, from, CurrentVersion)`. Go permits one package per directory, so exactly one of those can hold. The call site is the stronger statement of intent and the concern genuinely separates — "capture this directory as it stands" has nothing to do with schema sequencing — so it became `internal/store/migrate/backup/backup.go`.
- **The guard refuses; it never repairs.** An automatic `chmod` would fix the symptom and erase the evidence. The operator needs to know their key material was group-readable, not to have it quietly tightened.
- **All violations are collected into one message with a repair command.** The plan asked for this and it is worth restating why: seven starts to discover seven files is a guard that trains operators to stop reading its output.
- **`os.MkdirAll` and `os.Stat` are not forbidden by the architecture guard.** Creating the data directory and asking whether a path exists are not accesses to a state record. This keeps `cmd/holzkubed/main.go` legal without an exemption, and the guard still catches every read, write, enumerate and rename.
- **The guard exempts `internal/store/` as a subtree but re-checks `store.go` explicitly.** `fsstore`, `lock.go` and `migrate/` *are* the seam; exempting them is correct. Exempting the interface file along with them would let the seam quietly start taking paths, so it is scanned separately.
- **A missing `VERSION` with existing records is version 1, not version 0.** No release ever wrote a version 0 layout; a directory without `VERSION` is one plan 01 wrote. Treating it as 0 would run a migration over data that needs none.
- **Records are now indented JSON with a trailing newline.** Determinism is the point — a repeated identical `Put` must be byte-identical so that "the file changed" means the record changed — and readable diffs against a backup tarball are the reason the file format was chosen in the first place.

## Deviations from Plan

### 1. [Rule 3 - Blocking] `backup.Create` required a package, so `backup.go` became `backup/backup.go`

- **Found during:** Task 2
- **Issue:** `files_modified` lists `internal/store/migrate/backup.go` while the same task's action text calls `backup.Create(dir, from, CurrentVersion)`. A file named `backup.go` inside `internal/store/migrate/` is part of package `migrate`; Go allows exactly one package per directory, so `backup.Create` cannot resolve. The two requirements are jointly unsatisfiable, the same class of conflict as plan 01's deviation 2.
- **Fix:** `internal/store/migrate/backup/backup.go`, package `backup`, exported `Create`. The call site now reads exactly as the plan wrote it and the plan's own `key_links` pattern `backup\.` matches.
- **Impact:** One extra directory. `BackupsDirName` is re-exported from `migrate` so callers and tests still have one name to use.
- **Committed in:** `82a7920`

### 2. [Rule 2 - Missing Critical] Refcounted teardown on the per-entity lock map

- **Found during:** Task 1
- **Issue:** The plan specifies `map[string]*sync.Mutex` behind a `sync.Mutex`. Taken literally that map only grows. Sessions are rewritten on nearly every request and are created and deleted constantly, so a long-running `holzkubed` would accumulate one mutex per session id ever seen and never release one — an unbounded leak in the one process that is meant to run for months.
- **Fix:** Each entry carries a `refs` count of holders and waiters; the entry is deleted when it reaches zero. `EntityLocks.Len()` exists so a test can assert it.
- **Verification:** `TestEntityLocksReleaseEmptiesTheMap` locks and releases 100 distinct keys and asserts the map is empty afterwards. `-race` clean.
- **Committed in:** `67cf151`

### 3. [Rule 3 - Blocking] `endtoend_test.go` needed `chmod 0700` on its fixture directory

- **Found during:** Task 3
- **Issue:** The new permission guard made `go test ./...` fail in `internal/httpapi`, which is outside this plan's declared files. The cause is not a bug in the guard: Go's `t.TempDir()` creates its numbered subdirectory with `0777 &^ umask`, i.e. `0755` on a normal host, and the guard correctly refuses it. The plan's own tests set `0700` explicitly; plan 01's end-to-end harness predates the guard and did not.
- **Fix:** One `os.Chmod(dir, 0o700)` in `newHarness`, with a comment explaining why the fixture has to be as tight as a real data directory. The guard was not weakened.
- **Consideration rejected:** having `Open` chmod the directory itself. FOUND-10 and the plan's own `planner_assumptions` are explicit that "bemängeln" means refusing with a naming error, because a silent fix is indistinguishable from no check at all.
- **Verification:** `go test ./... -count=1 -race` passes; the guard still rejects a real `0755` directory, confirmed against the built binary.
- **Committed in:** `03be623`

### 4. [Rule 2 - Missing Critical] The crash hook was moved out of production code

- **Found during:** Task 3
- **Issue:** The first implementation put `setInterrupt(t *testing.T, ...)` in `atomic.go`, which made the production package import `testing`. That pulls the testing framework and its flag registration into the shipped binary — in a project whose whole premise is one small self-contained binary.
- **Fix:** `atomic.go` keeps only the `atomic.Int32` hook, the interruption points and `errInterrupted`; `setInterrupt` and `clearInterrupt` live in `atomic_test.go`. Production builds read a hook that nothing can arm.
- **Committed in:** `03be623`

### 5. [Rule 2 - Missing Critical] The architecture guard asserts it actually scanned the tree

- **Found during:** Task 3
- **Issue:** The guard walks a relative path (`../../..`) to the repository root. If that path ever breaks — a package move, a different test working directory — the walk finds nothing, reports no violations and passes. A guard that passes for the wrong reason is worse than no guard, because it is believed.
- **Fix:** The test counts scanned files and fails below 15. It was additionally verified by negative control: an `os.ReadFile` temporarily planted in `internal/httpapi/web.go` produced `../../../internal/httpapi/web.go:76: os.ReadFile` and a failure; the canary was then removed and the file confirmed unchanged against git.
- **Committed in:** `03be623`

---

**Total deviations:** 5 (2 Rule 3 blocking, 3 Rule 2 missing-critical)
**Impact on plan:** No scope creep. Deviation 1 is forced by Go's package rules and preserves the plan's own call syntax. Deviation 3 is the only file touched outside `files_modified` — one line in a test fixture, required because the guard works. Deviations 2, 4 and 5 each close a hole that would have shipped as a passing test suite.

## Threat Flags

None. Every threat the plan registered is mitigated and tested:

| Threat | Mitigation | Proven by |
|---|---|---|
| T-01-10 concurrent `Put` | per-entity mutex + rev CAS | `TestConcurrentPutSameIDYieldsExactlyOneWinner` under `-race` |
| T-01-11 two processes | `flock LOCK_EX\|LOCK_NB` | `TestProcessLockRefusesSecondOpen` + two real binaries |
| T-01-12 wide permissions | `Guard` at start, `chmod 0600` before `rename` | five `TestPermissionGuard*` + `TestWriteAtomicKeepsTheTemporaryInTheTargetDirectory` |
| T-01-13 orphaned temporaries | prefix sweep in `Open` before any read | `TestOpenSweepsOrphanedTemporaries`, `TestCrashInject` |
| T-01-14 too-new schema | refusal naming both versions | `TestMigrateRefusesAVersionNewerThanTheBinary` |
| T-01-15 world-readable tarball | `0600` inside the `0700` directory, guarded | `TestMigrateUpgradeBacksUpBeforeChangingAnything`, `TestPermissionGuardCoversBackupsAndTheLockFile` |

No file created here introduces network, auth or trust-boundary surface beyond the register.

## Known Stubs

| Stub | File | Why it is intentional / who resolves it |
|---|---|---|
| `migrations` is an empty list | `internal/store/migrate/migrate.go:60` | Phase 1 defines exactly one schema version, so there is nothing to apply. The whole surrounding path is executed against fixtures so phase 3 only appends an entry. |
| `sessionStore.Put` still does not enforce CAS | `internal/store/fsstore/fsstore.go` | Deliberate, inherited from plan 01 and unchanged here: the session manager is the single writer per token and a spurious 409 would log the operator out. It now takes the per-entity lock, so the read-modify-write is atomic. |
| `flock` is Unix-only (`syscall.Flock`) | `internal/store/lock.go` | holzkube targets Linux and darwin (D-02 names both). There is no Windows build and no build tag pretending otherwise. |
| No `golangci-lint` rule for direct file access | — | FOUND-07 is now held by an executed test rather than by review, which was plan 01's carried-forward concern. A lint rule remains optional polish for plan 06. |

None of these prevents this plan's goal.

## Issues Encountered

- **`t.TempDir()` is `0755`, not `0700`.** Go creates the numbered subdirectory with `0777 &^ umask`. This surfaced only when the guard landed and broke plan 01's end-to-end test — see deviation 3. It is worth knowing for every later phase that writes a data-directory fixture.
- **`grep -cE 'func .*Down|Rollback'` matches the bare word anywhere in the file,** including in a comment, because the alternation binds loosely. The forward-only rationale in `migrate.go` is therefore written without that word. The doc comment says what it means without tripping its own acceptance check.
- **The `interruptAfter` hook needed `atomic.Int32` rather than a plain variable.** The package's other tests are deliberately concurrent, and `-race` would flag a plain package-level variable read from a goroutine while another test wrote it.

## User Setup Required

None — no external service configuration. Existing operators with a data directory created outside holzkube may need `chmod 0700` once; the refusal message prints the exact command.

## Next Phase Readiness

**Ready.** The persistence layer is now the thing later phases can lean on:

- **Plan 03 (audit)** — unaffected by design. `internal/audit` stays exempt from the architecture guard because the append-only JSONL is not a store entity; the exemption is documented in the test comment rather than folklore.
- **Plan 04 (auth)** — session writes now take a per-session-id lock, so the sudo window and rate-limit state can be persisted without a global bottleneck.
- **Plan 06 (build/release)** — `go.mod` is unchanged, so nothing new to pin or audit. A `golangci-lint` rule for direct file access is now optional rather than the only enforcement.
- **Phase 3 (inventory)** — the first real migration is one appended `Migration{From: 1, To: 2, Apply: ...}` plus a fixture; backup, ordering, deferred `VERSION` write and failure handling are already proven.

**Concerns to carry forward:**

1. **The crash model is process-level, not hardware-level.** All three interruption points are injected in-process. Nothing here proves behaviour against a torn write from the filesystem or a lying disk cache; that would need block-layer fault injection. Recorded as `D4` with `human_judgment: true`.
2. **`FOUND-10` was not marked complete.** A sibling plan in this phase also declares it and has not finished; `requirements.ready-ids` returned `FOUND-10` as blocked, which is correct. `FOUND-07`, `-08` and `-09` were marked.
3. **The migration path has no real migration.** Every branch is executed, but against fixtures. The first genuine data conversion in phase 3 is where the design meets real records.
4. **On-disk record format changed to indented JSON.** No consumer outside the store reads those bytes, and no deployed installation exists, so nothing breaks. Noting it because a plan 01 data directory written before this change is still read correctly — `encoding/json` ignores whitespace — but will be rewritten on the next `Put`.

---
*Phase: 01-foundation-skeleton*
*Completed: 2026-08-28*

## Self-Check: PASSED

**Files claimed as created — all present on disk:** `internal/store/lock.go`, `internal/store/lock_test.go`, `internal/store/fsstore/permissions.go`, `internal/store/fsstore/permissions_test.go`, `internal/store/fsstore/atomic_test.go`, `internal/store/fsstore/concurrency_test.go`, `internal/store/fsstore/crashinject_test.go`, `internal/store/migrate/migrate.go`, `internal/store/migrate/migrate_test.go`, `internal/store/migrate/backup/backup.go`, `internal/store/testdata/README.md` — FOUND.

**Commits — all six present in `git log`:** `4f400f0`, `67cf151`, `051993a`, `82a7920`, `b058b45`, `03be623` — FOUND.

**Plan-level verification re-run at close-out:**

| Check | Result |
|---|---|
| `go test ./internal/store/... -count=1 -race` | PASS (exit 0) |
| `go build ./... && go vet ./...` | PASS (exit 0) |
| `go test ./... -count=1 -race` | PASS (no regression against 01-01) |
| `go test ./internal/store/fsstore -run TestNoDirectFileAccessOutsideFsstore -v -count=1` | PASS, not SKIP |

**Task acceptance criteria — all re-run:**

| Criterion | Result |
|---|---|
| `go test ./internal/store/... -run 'TestProcessLock|TestConcurrent' -race` exits 0 | PASS |
| `internal/store/lock.go` contains `func AcquireProcessLock` | PASS (1) |
| `store.go` contains `ErrConflict`, `ErrNotFound`, `ErrAlreadyRunning` | PASS |
| 50 concurrent puts on one id → 1 winner, 49 `ErrConflict`, rev +1 | PASS |
| 50 concurrent puts on 50 ids → 50 successes | PASS |
| a second `fsstore.Open` returns `ErrAlreadyRunning` | PASS |
| `go test ./internal/store/migrate/... -count=1` exits 0 | PASS |
| `migrate.go` contains `CurrentVersion` and `func Run` | PASS |
| `grep -cE 'func .*Down|Rollback' migrate.go` returns 0 | PASS (0) |
| `VERSION = CurrentVersion+1` → error naming both numbers, nothing written | PASS |
| `VERSION = CurrentVersion-1` → `0600` file under `backups/` before the migration | PASS |
| a failing migration leaves the old `VERSION` and keeps the tarball | PASS |
| `permissions.go` contains the mask `0o077` | PASS (1) |
| `TestCrashInject` covers all three interruption points | PASS (3 subtests) |
| `Open` on a `0755` dir errors naming the path and `0700` | PASS (unit + real binary) |
| `Open` on a `0700` dir with a `0644` file names that file | PASS |
| after a crash before `rename`, the next `Get` returns the whole old record and no temporary remains | PASS |

**Real-binary checks (not just unit tests):** two `holzkubed` processes on one `--data-dir` → the second exits 1 with `is locked by pid 65924`; after `kill -9` of the first, a fresh instance starts; `VERSION` is `1`; the directory is `drwx------` with `-rw-------` contents; `chmod 0755` on the directory makes the next start refuse with the repair command.

**Negative control on the architecture guard:** an `os.ReadFile` temporarily planted in `internal/httpapi/web.go` made `TestNoDirectFileAccessOutsideFsstore` fail at `web.go:76`; the canary was removed and the file confirmed unchanged against git.

**Hygiene:** no file deletions in the commit range `5bf65c6..HEAD`; `go.mod` and `go.sum` unchanged (no new dependency); nothing untracked; `.gsd/` and `.planning/milestone.lock` never staged.
