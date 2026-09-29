---
phase: 13-host-aktionen
plan: 05
subsystem: host-actions
tags: [host-actions, fsstore, claim, pickup-timer, process-guard, ast]
status: complete

requires:
  - phase: 13-host-aktionen
    plan: 01
    provides: fsstore.PlaceNew, internal/host/hostaction (Box, Place, Order), the main.go Box literal
provides:
  - fsstore.Claim (rename-claim, read <= 4 KiB of a regular file, remove; ENOENT wraps fs.ErrNotExist)
  - hostaction.Config.Claim / PickupTimeout / AfterFunc, DefaultPickupTimeout (10 s), StateWithdrawn
  - the Box's pickup timer, claim-withdrawal and startup sweep; one info line when the order path is not ReferenceOrderPath
  - internal/processguard_test.go (TestTheDaemonStartsNoProcess, TestProcessGuardRecognisesAProcessStart)
affects: [13-06, 13-08, 13-09]

actuals:
  tokens: 10600
  tasks: 3
  commits: 3
plan_head_before: 90bcb23779d3f55d637e88d342988b9aec9d50a8
plan_head_after: 49fb1e0198c32050ec6a41b6bed823cfffb19c7d

tech-stack:
  added: []
  patterns:
    - "Withdraw by claim: rename to a temp-prefixed name, then remove -- whoever renames first owns the file"
    - "Timers injected as AfterFunc and fired by hand in tests; the callback re-checks id and closed under the owner's mutex"
    - "Process guard in two halves: own packages' imports via go list, every call form via an import-alias-aware AST scan"

key-files:
  created:
    - internal/store/fsstore/place_test.go
    - internal/host/hostaction/hostaction_test.go
    - internal/processguard_test.go
  modified:
    - internal/store/fsstore/atomic.go
    - internal/host/hostaction/hostaction.go
    - cmd/holzkube-managerd/main.go

key-decisions:
  - "The process guard's AST half resolves import aliases (sc \"syscall\" is still syscall) and refuses a dot import of os, syscall or x/sys/unix, which would hide the calls from a selector check; the plan named selectors only"
  - "Claim reads only a regular file (Lstat first) and removes whatever it claimed either way: a link planted under the order name is withdrawn, never followed"
  - "Place stops the previous order's timer when it places a new one (the previous file must have been gone); the callback's id check is what actually protects the newer order, and the test fires the old timer anyway"
  - "The path-mismatch info line is logged by every NewBox, with or without Claim; only the sweep and the timer depend on Claim"

requirements-completed: [HACT-06, HACT-07]

duration: 18min
completed: 2026-09-29
---

# Phase 13 Plan 05: Claim, pickup timer, process guard Summary

**The daemon's only effect on the host is one order file, and no order outlives 10 s without a helper: `fsstore.Claim` takes it back by rename, the Box withdraws it on a timer and at start, and `TestTheDaemonStartsNoProcess` goes red against a smuggled `exec.Command` and a smuggled `syscall.ForkExec`.**

## Where this ran

The operator's Raspberry Pi 5 (aarch64), Go 1.26.7 via GOTOOLCHAIN, **no -race** (ThreadSanitizer refuses the Pi 5 kernel's address space). Nothing was installed and the production service was not touched: `systemctl show holzkube-manager.service -p ActiveEnterTimestamp` read `Mon 2026-09-28 18:06:00 CEST` before (01:47Z) and after (02:04Z) the plan.

## Performance

- **Duration:** ~18 min
- **Started:** 2026-09-29T01:47:13Z
- **Completed:** 2026-09-29T02:05Z
- **Tasks:** 3
- **Files:** 6 (3 created, 3 modified)

## Accomplishments

- `fsstore.Claim(path, tag)`: tag checked against `^[a-z0-9-]{1,64}$` before anything moves; `rename(path, <dir>/.holzkube-manager-tmp-claim-<tag>)`; ENOENT wraps `fs.ErrNotExist`; Lstat, then a read through `io.LimitReader` at 4 KiB of a regular file only; `removeAndSync` in every case after a successful rename. A crash-left claim carries the temp prefix, so `fsstore.Open`'s sweep removes it (tested).
- `place_test.go`: PlaceNew 0600 and whole, a second placement `fs.ErrExist` with the first byte for byte, no temporary either way, `fsstore.Open` (Guard) accepts a data directory holding the order.
- The Box: `Config.Claim`, `PickupTimeout` (default `DefaultPickupTimeout = 10 * time.Second`), `AfterFunc` (default `time.AfterFunc(...).Stop`), `StateWithdrawn`. Place arms one timer per placed order when Claim is configured; the timer runs under the Box's mutex and claims only while the last order is still its own and not settled; success -> withdrawn plus one warning with id, action, "within 10 s" and `systemctl status holzkube-manager-host.path`; ENOENT -> the helper took it, silence; other error -> logged, state left to Lstat. NewBox with Claim withdraws a leftover `host-order` (tag `startup`, byte count only in the warning). NewBox logs one info line naming both paths and `deploy/HOST-HELPER.md` when the order path is not `ReferenceOrderPath`. Close stops the timer and a late firing claims nothing. Still no os.Remove/Rename/Link in the package (TestNoDirectFileAccessOutsideFsstore green).
- main.go: `Claim: fsstore.Claim`, the Box still built after `fsstore.Open`.
- `internal/processguard_test.go`: two subtests (go-list half: own packages in `go list -deps ./cmd/holzkube-managerd` list no `os/exec`, with a floor of 15 own packages; AST half: 226 non-test files under cmd/ and internal/, simulators excluded, floor 15), a pure classifier `processStarts`, and a negative control over 13 in-memory sources. `os/exec` itself is in the daemon's dependency list today (`go list -deps ... | grep -c '^os/exec$'` = 1), through third-party packages only.

## Task Commits

1. **Task 1: one slot, and a claim that races nobody** - `c8a9b1e` (feat)
2. **Task 2: withdrawn after 10 s, withdrawn at start, never someone else's** - `6169888` (feat)
3. **Task 3: the daemon starts no process** - `49fb1e0` (test)

## Fault injections (each put in, run, seen red from `go test`'s own exit code, restored from a saved copy or deleted)

| ID | Injection | rc | Failing line |
|---|---|---|---|
| F15 | PlaceNew: `os.Link(tmpName, path)` -> `os.Rename(tmpName, path)` | 1 | `place_test.go:54: second PlaceNew = <nil>, want an error wrapping fs.ErrExist: a second order must never replace one that waits` (and `:73: Open changed the placed order to "poweroff fedcba9876543210\n"`) |
| F16 | Place does not arm the timer (`b.stop = b.cfg.AfterFunc(...)` replaced) | 1 | `hostaction_test.go:195: Place armed 0 pickup timers, want 1: an order nobody picks up would lie there until a helper starts` -> `--- FAIL: TestWithdrawAnOrderNotPickedUp` (plus the three other timer tests) |
| - | the timer's `b.last.ID != id` check removed | 1 | `hostaction_test.go:255: B's order is gone after A's timer fired` -> `--- FAIL: TestWithdrawNeverTakesANewerOrder` |
| - | the timer's `b.closed` check removed | 1 | `hostaction_test.go:415: a timer firing after Close withdrew the order`, `:418 ... State:withdrawn`, `:421: 1 claims after Close` |
| - | NewBox's `b.sweep()` not called | 1 | `hostaction_test.go:279: after NewBox the data directory holds [host-order], want nothing` |
| F7 | new `internal/host/smuggle.go`: `exec.Command("systemctl", "reboot").Run()` | 1 | **both halves red:** `processguard_test.go:72: github.com/holzcloud/holzkube-manager/internal/host imports os/exec and is part of cmd/holzkube-managerd.` and `:126: 1 way(s) to start a process ... internal/host/smuggle.go:3: imports "os/exec"` |
| F8 | new `internal/host/smuggle.go`: `syscall.ForkExec(...)`, no os/exec | 1 | **AST half red, go-list half green:** `--- PASS: .../own_packages_in_the_daemon_import_no_os/exec`, `--- FAIL: .../no_source_starts_a_process` with `:126 ... internal/host/smuggle.go:6: uses syscall.ForkExec` |
| - | classifier: the SYS_EXECVE ident case disabled | 1 | `processguard_test.go:258: not recognised: the guard would let unix.SYS_EXECVE through` (and SYS_EXECVEAT) |
| - | classifier: import aliases not resolved (`processStartSelectors[pkg.Name]`) | 1 | `processguard_test.go:258: not recognised: ... syscall.ForkExec under another name`, `... unix.Exec` |

Both injected `smuggle.go` files compiled (`go build ./internal/host` ok) before the run, and were deleted afterwards; `git status --porcelain` then listed only `internal/processguard_test.go` (not yet committed), nothing under `internal/host`.

## Verification (exit codes read from the command itself)

- `go test ./internal/store/... -run 'TestPlaceNew|TestClaim|TestNoDirectFileAccessOutsideFsstore' -v`: rc 0, 3 top-level PASS.
- `go test ./internal/host/hostaction -run 'TestPlace|TestWithdraw|TestSweep|TestConcurrentPlace|TestClose' -v`: rc 0, 12 PASS lines (7 tests).
- `go test ./internal/httpapi -run TestHostActionRoundTrip -v`: rc 0, `--- PASS: TestHostActionRoundTrip`, no SKIP.
- `go test ./internal -run 'TestTheDaemonStartsNoProcess|TestProcessGuardRecognisesAProcessStart|TestBinaryDependencyWeight' -v`: rc 0, neither subtest skipped.
- `./bin/task lint:go`: rc 0, 0 issues (after each task).
- `go test ./internal/... ./cmd/... -count=1`: rc 1 on the first run -- only `TestANodeSaysHowItBooted` (`live_test.go:856: ... Get ... timed out`, the known flake under load); `go test ./internal/upgrade -count=1` alone: rc 0. Every other package ok.
- `GOOS=darwin GOARCH=arm64 go build ./...`: rc 0. `git diff 90bcb23 -- go.mod go.sum`: empty. `go test ./internal/publicrepo/`: ok.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing critical functionality] The AST half resolves import aliases and refuses dot imports**
- **Found during:** Task 3
- **Issue:** A selector check by identifier name alone (`syscall.ForkExec`) lets `import sc "syscall"; sc.ForkExec(...)` and `import . "syscall"; ForkExec(...)` through -- the guard would be green against a smuggled process start one rename away.
- **Fix:** The classifier maps each file's local import names to import paths and matches (path, name); a dot import of os, syscall or golang.org/x/sys/unix is reported. Both covered by negative-control rows, and the alias resolution was seen red when removed.
- **Files:** internal/processguard_test.go
- **Commit:** 49fb1e0

**2. [Rule 1 - Bug] TestCloseStopsThePickupTimer panicked instead of failing under F16**
- **Found during:** Task 2, F16 injection
- **Issue:** It indexed `timers[0]` without checking a timer was armed.
- **Fix:** A `Fatalf` on the timer count first. Committed with Task 2.

Also: the Task 2 tests were written after the Box code, not before; the red evidence for them is the four injections above (F16 and three more), each seen failing from `go test`'s exit code.

## Known Stubs

None. The API contract, guide and page copy for `withdrawn` are plans 13-08 and 13-09 by design (the web schema already takes `state` as a string).

## Threat Flags

None beyond the plan's threat model (T-13-27..T-13-32): Claim touches only the order path and a temp-prefixed sibling in the data directory.

## Next Phase Readiness

- 13-08: the order's `state` can now be `withdrawn`; the UI-SPEC sentence "not picked up" maps to it.
- 13-09: contract text for the 10-s withdrawal and the startup sweep; the warnings' wording is in hostaction.go (`withdraw`, `sweep`).

## Self-Check: PASSED
