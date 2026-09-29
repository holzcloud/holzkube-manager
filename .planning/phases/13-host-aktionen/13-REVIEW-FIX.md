---
phase: 13-host-aktionen
fixed_at: 2026-09-29T05:25:00Z
review_path: .planning/phases/13-host-aktionen/13-REVIEW.md
iteration: 1
findings_in_scope: 10
fixed: 10
skipped: 0
status: all_fixed
---

# Phase 13: Code Review Fix Report

**Fixed at:** 2026-09-29T05:25:00Z
**Source review:** .planning/phases/13-host-aktionen/13-REVIEW.md
**Iteration:** 1

**Summary:**
- Findings in scope: 10 (WR-01..WR-06 as fix_scope critical_warning, plus IN-01..IN-04 on request)
- Fixed: 10
- Skipped: 0

Every fix except IN-02 has a regression test. Each test was seen RED against the reinstated fault and then GREEN. IN-02 changes only comments and docs. The logic fixes (WR-01..WR-05, IN-04) are marked "fixed: requires human verification": the tiers below check behaviour through tests, not the semantics of every interleaving.

## Where verification ran

Everything ran in the main checkout: `workflow.use_worktrees` is `false`, so no worktree was created. The machine was the operator's Pi 5 (aarch64). Go was `~/.local/go/bin` with `GOTOOLCHAIN=go1.26.7`, without `-race`. The helper script ran only as a temporary copy under `unshare --user --map-root-user`, with overrides and a stand-in systemctl. Nothing was installed, and the service, `/var/lib/holzkube-manager`, `/usr/local/bin` and `/etc/systemd` were not touched.

Final gates, with exit codes taken from each command itself:
- `go test ./internal/... ./cmd/...`: rc 0
- `npm --prefix web run test`: rc 0 (59 files, 712 tests)
- `./bin/task lint:go`: rc 0, 0 issues
- `npm --prefix web run lint`: rc 0. It reports 2 warnings and 1 info, all older than this change and in files it did not touch (`DataTable.tsx`, `wall.test.tsx`).

The first full Go run caught a real consequence of WR-05: a helper-API control test had forged a host token with no session. That is fixed in e76cf50, and the second full run was green.

## Fixed Issues

### WR-01: The helper's `rm -f` makes the daemon's rename-claim one-sided

**Files modified:** `deploy/holzkube-manager-host.sh`, `internal/host/hostaction/script_test.go`, `docs/api-contract.md`
**Commit:** 698ff3e
**Status:** fixed: requires human verification
**Applied fix:** The script now takes ownership of the order before reading it. It renames it (`mv -f -T`) to `.holzkube-manager-tmp-helper-claim-$$` in the same directory. A failed rename with the name gone means the order was withdrawn: the script exits 0 without acting. It then reads the claimed name with `dd iflag=nofollow,nonblock` and removes it before acting. A directory under the order's name is still refused (failed, not taken). New `TestHostScriptClaimRace` uses a `BASH_ENV` hook, since a function wins over the fixed PATH. It plays the daemon's rename just before each of stat, mv, dd and rm, with and without a newer order B placed at the same moment. It asserts that exactly one side owns A and that B is never lost. It was RED against the old script (the daemon won before rm, the script rebooted anyway and deleted B).

### WR-02: A pending order survives a daemon stop

**Files modified:** `internal/host/hostaction/hostaction.go`, `internal/host/hostaction/hostaction_test.go`, `web/src/components/HostActions.tsx`, `web/src/components/HostActions.test.tsx`, `docs/guide.md`, `docs/api-contract.md`
**Commit:** ef27c8b
**Status:** fixed: requires human verification
**Applied fix:**
- `Box.Close()` now claims the last order if it still waits (tag `shutdown-<id>`) and logs that nothing was done.
- The page keeps all four buttons off while a reboot, poweroff or restart-service order is `started`, until it is back. The new reason is "The last host action is still under way; wait for it to finish."
- The guide no longer promises "never carried out late". It names the helper's age window as a net that depends on the clock, and the contract gains a "shutdown withdrawal" bullet.

`TestCloseWithdrawsAWaitingOrder` was RED against the old Close. The page rows were RED against the old update-only check.

### WR-03: Errors after the irreversible step report the wrong state

**Files modified:** `internal/store/fsstore/atomic.go`, `internal/store/fsstore/place_test.go`, `internal/host/hostaction/hostaction.go`, `internal/host/hostaction/hostaction_test.go`
**Commit:** c33f8d1
**Status:** fixed: requires human verification
**Applied fix:**
- New `fsstore.ErrTookEffect`. `PlaceNew` wraps any error after a successful `link`, and `Claim` any error after a successful `rename` (read or remove). Test seams are `placeNew(…, settle)` and `claim(…, remove)`.
- `Box.Place` treats a wrapped error as placed: it sets the state, arms the timer and logs. `withdraw`, `sweep` and `Close` treat it as withdrawn.

The fsstore and Box tests were RED against unmarked errors and against a Box that ignored the mark. One of them uses a real case, a symlink under the order's name.

### WR-04: The page can lock all four host buttons with no way out

**Files modified:** `web/src/components/HostActions.tsx`, `web/src/components/HostActions.test.tsx`, `docs/guide.md`
**Commit:** 3d3ad8c
**Status:** fixed: requires human verification
**Applied fix:** There is a new final phase, `no-answer`, in red with Dismiss. It applies in two cases:
- 60 s (`RESULT_WITHIN_MS`) after placing with no result for the id, unless the daemon reports the file still there, which stays "placed".
- 15 min (`STARTED_WITHIN_MS`) after placing, when a started action has not become update-finished or back.

A failed poll still reads "waiting". Each case has its own sentence and names the journal to read (`holzkube-manager-update` for an update). The buttons come back. The new orderPhase rows were RED against the unbounded phases.

### WR-05: One typed confirmation allows unlimited host orders for 10 minutes, from any operator session

**Files modified:** `internal/jobs/confirm.go`, `internal/jobs/confirm_test.go`, `internal/httpapi/handlers/host.go`, `internal/httpapi/handlers/confirm_test.go`, `internal/httpapi/hostgatesapi_test.go`, `internal/httpapi/hosthelperapi_test.go`, `docs/api-contract.md`, `docs/guide.md`
**Commits:** 718070b, e76cf50
**Status:** fixed: requires human verification
**Applied fix:**
- **New token pair in jobs.** `Confirmer.IssueOnce` issues a token `stamp.nonce.sig`. The random nonce is covered by the HMAC, and the payload is marked "once:" so this kind cannot cross with the other. `Confirmer.CheckOnce` verifies the token and spends it under a mutex. `ErrConfirmationUsed` wraps `ErrConfirmationInvalid`, which becomes 403 `forbidden.confirmation-invalid`.
- **Why the nonce.** The first attempt reused `Issue`. The round-trip test showed that two confirmations in the same second produce identical tokens, so spending one spent the other.
- **Session binding.** The host intent now carries `Params{"session": sha256(session id)}` on both issue and check.
- **Sudo replay still works.** A 428 from the sudo gate comes before the check, so the replay after the password finds the token unspent; this is tested.
- **Node routes unchanged.** `Issue` and `Check` stayed as they were, and a test holds `Check` stateless.

RED checks:
- The original `Issue`/`Check` without a session: both the replay and the other session placed an order.
- An intent without the session: the other session placed an order.
- A `CheckOnce` that does not spend: 16 of 16 racing requests got through.

The follow-up e76cf50 moved three API tests to session-bound tokens: the control places with the confirm route's own token, and `sessionHostToken` is proven to be accepted.

### WR-06: The root-script security matrix skips silently when user namespaces are unavailable

**Files modified:** `internal/host/hostaction/script_test.go`, `internal/httpapi/hostactionsapi_test.go`, `.github/workflows/ci.yml`
**Commit:** 55cc6a6
**Applied fix:** On Linux, `requireHostNamespace` and `requireRootNamespace` now fail when unshare is missing or cannot map root. With `HOLZKUBE_MANAGER_NO_USERNS=1` they skip instead, reporting "SKIPPED, not verified", as `TestUnitsVerify` does. The CI Linux job sets `kernel.apparmor_restrict_unprivileged_userns=0` before `go test`, so the matrix runs there. That CI step has not been run: there are no CI minutes and nothing was pushed. It was seen RED with a failing `unshare` first on PATH: the new guards fail, the opt-out skips visibly, and the old code printed `ok` for both packages.

### IN-01: The process guard does not catch raw exec syscalls by number or `go:linkname`

**Files modified:** `internal/processguard_test.go`
**Commit:** 215af88
**Applied fix:** A cheap and honest extension existed. The guard now reports:
- every `//go:linkname` directive (files are parsed with comments)
- `import "C"`
- `.s` files under `cmd/` and `internal/`
- any use of `Syscall`, `Syscall6`, `RawSyscall` and their variants from syscall or x/sys/unix whose first argument is not a named `SYS_` constant, including the function used as a value

None of these occurs in the module today. It was seen RED with `unix.Syscall(221, …)` and with a `//go:linkname` injected as non-test files in `internal/host`; the old guard stayed green on the former. The negative control gains six cases and a clean `SYS_GETPID` call. The guard still cannot see what `reflect` or unsafe function-pointer tricks could reach. The doc comment now lists what it refuses rather than claiming completeness.

### IN-02: The comments say "no Environment=" while the guide tells operators to add one

**Files modified:** `deploy/holzkube-manager-host.sh`, `deploy/holzkube-manager-host.service`, `deploy/HOST-HELPER.md`
**Commit:** 8b64a06
**Applied fix:** The script and the unit now name `HOLZKUBE_MANAGER_HOST_ORDER` as the one override a drop-in sets for a non-default data directory. `…_STATE_DIR` and `…_SYSTEMCTL` stay test-only. The HOST-HELPER.md drop-in note says the same, and that the script takes the order by a rename. This is comments and docs only, with no regression test: there is no behaviour to put back. `bash -n` and `TestUnitsVerify` (systemd-analyze) pass.

### IN-03: The client and server compare the typed hostname differently

**Files modified:** `web/src/components/HostActions.tsx`, `web/src/components/HostActions.test.tsx`
**Commit:** 484cf34
**Applied fix:** The dialog now enables confirm on `typed.trim() === hostname`, as the server does, and sends the text as typed. The new test (" example-host " enables it and reaches the confirm route) was RED against the exact comparison.

### IN-04: `Box.Order()` reports any `Lstat` error as `picked-up`

**Files modified:** `internal/host/hostaction/hostaction.go`, `internal/host/hostaction/hostaction_test.go`
**Commit:** b750f22
**Status:** fixed: requires human verification
**Applied fix:** Only `fs.ErrNotExist` now means picked up. Any other error keeps the order pending and is logged once, not on every poll. `TestOrderStateWhenTheSlotCannotBeRead` makes the directory 0000 (EACCES) and was RED against the old reading.

## Notes for the operator

- **Flaky page tests.** One run of the web component and route tests showed 2 failures while another heavy vitest ran at the same time. The rerun was green (575/575), and so was the full suite (712/712). This looks like timing under load on the Pi, not these changes.
- **Only the page was modified for the no-answer phase.** `13-UI-SPEC.md` does not list the new `no-answer` phase or the "under way" reason, and was not changed.
- **README.** It needs no change; no feature was added.

---

_Fixed: 2026-09-29T05:25:00Z_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
