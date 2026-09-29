---
phase: 13-host-aktionen
reviewed: 2026-09-29T04:05:06Z
depth: deep
files_reviewed: 53
files_reviewed_list:
  - .goreleaser.yaml
  - README.md
  - cmd/holzkube-managerd/budget_test.go
  - cmd/holzkube-managerd/main.go
  - deploy/HOST-HELPER.md
  - deploy/holzkube-manager-host.path
  - deploy/holzkube-manager-host.service
  - deploy/holzkube-manager-host.sh
  - docs/api-contract.md
  - docs/guide.md
  - internal/audit/redact.go
  - internal/host/actions_test.go
  - internal/host/collector.go
  - internal/host/collector_test.go
  - internal/host/health.go
  - internal/host/health_test.go
  - internal/host/host.go
  - internal/host/hostaction/helper.go
  - internal/host/hostaction/helper_owner_other.go
  - internal/host/hostaction/helper_owner_unix.go
  - internal/host/hostaction/helper_test.go
  - internal/host/hostaction/hostaction.go
  - internal/host/hostaction/hostaction_test.go
  - internal/host/hostaction/result.go
  - internal/host/hostaction/result_test.go
  - internal/host/hostaction/script_test.go
  - internal/host/hostaction/units_test.go
  - internal/host/sensors.go
  - internal/host/sensors_test.go
  - internal/httpapi/endtoend_test.go
  - internal/httpapi/handlers/confirm_test.go
  - internal/httpapi/handlers/host.go
  - internal/httpapi/handlers/wall_host_test.go
  - internal/httpapi/hostactionsapi_test.go
  - internal/httpapi/hostgatesapi_test.go
  - internal/httpapi/hosthelperapi_test.go
  - internal/httpapi/problem.go
  - internal/httpapi/router.go
  - internal/processguard_test.go
  - internal/store/fsstore/atomic.go
  - internal/store/fsstore/place_test.go
  - web/fixtures/demo.json
  - web/scripts/readme-images.mjs
  - web/src/api.ts
  - web/src/components/HostActions.test.tsx
  - web/src/components/HostActions.tsx
  - web/src/components/charts/Sensors.test.tsx
  - web/src/components/charts/Sensors.tsx
  - web/src/fixtures.test.ts
  - web/src/routes/host.browser.test.tsx
  - web/src/routes/host.test.tsx
  - web/src/routes/host.tsx
  - web/src/routes/wall.test.tsx
findings:
  critical: 0
  warning: 6
  info: 4
  total: 10
status: issues_found
---

# Phase 13: Code Review Report

**Reviewed:** 2026-09-29T04:05:06Z
**Depth:** deep
**Files Reviewed:** 53
**Status:** issues_found

## Summary

I traced the whole chain: route → `handlers/host.go` → `hostaction.Box` → `fsstore.PlaceNew`/`Claim` → path unit → `deploy/holzkube-manager-host.sh` → `last` → `ReadResult` → `/host` page.

**No path lets anyone beyond the four locks make root do something.** Every host route needs a session. The confirm route needs the operator role. The action routes also need sudo and are audited. The token is bound to `{host.<action>, "@host"}` and is rebuilt from the route. No wall link reaches any of these routes.

The root script holds up against the attacks it was built for:
- It reads with `dd` using `nofollow,nonblock` and a 65-byte limit.
- It matches one anchored ERE under `LC_ALL=C`, with an exact-size check that catches NUL bytes and multiple lines.
- It never logs the content.
- It uses fixed argv per action.
- It writes `last` only into a root-owned, non-group/other-writable state directory. That directory is checked to be a real directory, not a symlink.

A symlink, a hardlink, a FIFO, a directory or a malformed line can at most make the helper refuse or fail. None of them makes it act.

The defects are in the **exactly-once / exactly-owner guarantees**:
- The helper's `rm -f` defeats the daemon's rename-claim (R8), so "whoever renames first owns the order" is not true for the helper side.
- `Box.Close()` disarms the pickup timer without withdrawing, so an order can outlive the process. Only the 60 s age window then guards it, and that window depends on a wall clock that a Pi restores from a saved timestamp. The guide promises "never carried out late".
- Error paths after an irreversible filesystem step (link, rename) report the wrong state.
- The page can lock its own buttons indefinitely.
- The root-script test matrix can skip silently.

## Narrative Findings (AI reviewer)

## Warnings

### WR-01: The helper's `rm -f` makes the daemon's rename-claim one-sided. A withdrawn order can still be carried out, and a newer order can be silently deleted

**File:** `deploy/holzkube-manager-host.sh:130-146` (with `internal/host/hostaction/hostaction.go:314-335`, `internal/store/fsstore/atomic.go:274-296`, `docs/api-contract.md:1795-1800`)

**Issue:** R8's premise is "rename is atomic and whoever renames first owns the order". But the helper does not claim. It reads (`dd`) and then unlinks with `rm -f`, and `rm -f` exits 0 when the name is already gone (checked: `rm -f -- missing; echo $?` prints `0`). Suppose the daemon's 10 s `Claim` (a rename) lands between the helper's `dd` and its `rm`:
- The daemon sets `StateWithdrawn` and logs "host order withdrawn … nothing was done".
- The helper's `rm -f` succeeds on nothing, and the helper runs `systemctl reboot` anyway.

Variant: if a new order B was linked into `host-order` in that window, the helper's `rm -f` deletes B. It then carries out the withdrawn A. B never gets a result, so the page follows B at `picked-up` forever (see WR-04).

The window is narrow: the helper has to reach `dd` right at the 10 s mark, for example on a loaded Pi or a busy PID 1. Still, it is exactly the race R8 and Pitfall 6 claim to close, and `api-contract.md` states the guarantee as fact. Pitfall 6 only handled the removal *before* `dd`.

**Fix:** Make the helper's consumption a claim as well. The minimal fix is to not ignore a vanished name:
```bash
if ! rm -- "$ORDER" 2>/dev/null; then
  if [[ ! -e $ORDER && ! -L $ORDER ]]; then
    log "Auftrag vor der Abholung zurueckgezogen"
    exit 0            # the daemon claimed it first: do not act
  fi
  log "FEHLER: Auftrag nicht entfernbar"; record - - failed; exit 1
fi
```
This closes the withdrawn-but-executed case. To also rule out deleting a newer order B, have root claim by rename before reading: `rename(2)` does not follow a symlink in the last component. For example, `mv -T -- "$ORDER" "$DATA/.holzkube-manager-tmp-helper-claim"`, then `dd iflag=nofollow` from the claim name, then `rm` it. The daemon's startup sweep already removes that prefix. Add a script test that renames the order away between `dd` and `rm` (for example via a `dd` wrapper on `PATH`) and asserts that `systemctl` is not called.

### WR-02: A pending order survives a daemon stop, and only a 60 s wall-clock window stops it running later. The guide says "never carried out late"

**File:** `internal/host/hostaction/hostaction.go:372-382`, `deploy/holzkube-manager-host.sh:68-69,161-163`, `web/src/components/HostActions.tsx:190-197`, `docs/guide.md:301-304`

**Issue:** `Box.Close()` only stops the pickup timer. `TestCloseStopsThePickupTimer` pins that it withdraws nothing ("the next start's sweep will"). The next start's sweep, however, runs *after* the helper at boot: PathExists fires at `paths.target`, long before the daemon. So an order placed and not consumed when the daemon stops is guarded only by the script's `MAX_AGE=60`, measured against the wall clock. That can happen when the path unit is stopped (for example during a system shutdown, when path unit and daemon stop in parallel) or failed (start-limit-hit).

- **Reboot case:** a Pi 5 keeps its RTC running across a reboot and reaches `paths.target` well inside 60 s, so an order placed in the last seconds before shutdown runs after the reboot.
- **Power-loss case:** a board without a battery-backed RTC restores the clock from a saved timestamp (timesyncd's clock file or fake-hwclock). "now" is then close to the save time, and a leftover whose mtime is within about 5 s + boot time of that save passes both the age and the future check.
- **Manual case:** the recovery recipe in `HOST-HELPER.md` (`systemctl start holzkube-manager-host.path`) runs a leftover too if it is under 60 s old.

The page makes the shutdown case reachable. `disabledReason` blocks only `placed`, `picked-up` and a started `update`. After a `reboot` or `poweroff` order reaches `started`, all four buttons are live again while the host is going down.

`docs/guide.md:301-304` then promises: "An order left over when holzkube-manager starts … is withdrawn too, never carried out late". The helper at boot contradicts that.

**Fix:**
- `Close()` should withdraw the order it still holds:
  ```go
  func (b *Box) Close() {
      b.mu.Lock(); defer b.mu.Unlock()
      if b.stop != nil { b.stop(); b.stop = nil }
      if !b.closed && b.cfg.Claim != nil && b.last != nil && b.last.State != StateWithdrawn {
          if _, err := b.cfg.Claim(b.orderPath(), "shutdown-"+b.last.ID); err == nil {
              b.last.State = StateWithdrawn
          }
      }
      b.closed = true
  }
  ```
- In `disabledReason`, also block while a `reboot`, `poweroff` or `restart-service` order is `started` (until `back`).
- Correct the guide sentence: state the age window as the net at boot, not a guarantee.

### WR-03: Errors after the irreversible step in `PlaceNew`/`Claim` make the Box report the wrong state

**File:** `internal/store/fsstore/atomic.go:229-243` and `:284-296`; `internal/host/hostaction/hostaction.go:288-306,321-334`

**Issue:**
- **`PlaceNew`:** once `os.Link` succeeded, the order is live and the helper may already be running it. A later failure of `os.Remove(tmpName)` or `fsyncDir` still returns an error. `Box.Place` then returns an error, the route answers 500, `b.last` is not set and **no pickup timer is armed**. The operator is told the action failed while the host reboots. If the helper is not running, the order lies there with no withdrawal until the next daemon start. That is the D-13 case "an order left lying".
- **`Claim`:** after the rename has succeeded (the daemon owns the order), a `removeAndSync` or `readClaimed` failure is returned as a generic error. `withdraw` logs "could not withdraw" and leaves the state alone. `Order()` then sees the name gone and reports `picked-up` for an order nobody picked up, so the page waits at `picked-up` with the buttons locked (WR-04).

**Fix:** Treat everything after the link or rename as best effort, reported but not turned into "it did not happen":
```go
// PlaceNew, after a successful os.Link:
if err := os.Remove(tmpName); err != nil && !errors.Is(err, fs.ErrNotExist) { /* sweep removes it; log */ }
if err := fsyncDir(dir); err != nil { return &PlacedButNotSynced{err} } // or log and return nil
```
For `Claim`, return a result that distinguishes "claimed (owned), cleanup failed" from "not claimed", and let `withdraw` set `StateWithdrawn` whenever the rename succeeded.

### WR-04: The page can lock all four host buttons with no way out: `picked-up` and a started `update` have no timeout and no dismiss

**File:** `web/src/components/HostActions.tsx:190-197,399-405,439-478`; `web/src/routes/host.tsx` (the `held` state)

**Issue:** `disabledReason` disables every button while the followed order is `placed`, `picked-up`, or a `started` `update`. None of these phases is in `FINAL`, so no "Dismiss status" button is offered. `held` lives until dismissed, so on the page that placed the order the lock lasts as long as the tab is open. After a reload, `followedOrder` picks the daemon's order again for 15 minutes.

Reachable ways into a permanent `picked-up`:
- The helper consumes the order but records nothing: killed between `rm` and `record`, or `mktemp`/`mv` fails in the state directory under `set -e`.
- `last` is unreadable (`result.readable === false` falls back to the order state).
- The claim race in WR-01 or the claim-error case in WR-03.

A permanent "An update is running" happens when `update` started but the update status file is not readable. The guide itself describes that state for an update script that does not record yet (`docs/guide.md` "feature does not write it; it replaces itself with the recording one after the next healthy update"). The server would accept a new order the whole time. Only the page refuses.

**Fix:** Give the non-final phases an upper bound. For example: treat `picked-up` without a matching result for more than 30 s, and a started `update` without a newer `checked_at` for more than about 15 min, as a final "no answer from the helper, see `journalctl -u holzkube-manager-host`" phase with Dismiss. Alternatively, offer Dismiss on every phase except `placed`.

### WR-05: One typed confirmation allows unlimited host orders for 10 minutes, from any operator session

**File:** `internal/httpapi/handlers/host.go:225,263-269`; `internal/jobs/confirm.go:82-126`

**Issue:** The host token is an HMAC over `expires|host.<action>|@host`. It is neither single-use nor bound to the session or user that typed the hostname. After one confirmation, the same token places another `update`, or another `reboot` after a rejected, failed or withdrawn one, for `ConfirmationTTL` (10 min), without the hostname being typed again. The page never re-uses a token, but anything that can replay the request (a stale tab, a script, the 428 interceptor's replay, another operator holding the token) can.

For nodes this was an accepted design. For the host, D-09 argues explicitly that typing is required *every time* because there is only one host. The token lifetime quietly weakens that. Sudo, role and CSRF still apply, so this is not a bypass of the four locks, but it does bypass the typed confirmation per order.

**Fix:** Make host tokens single-use. Keep a small in-memory set of spent tokens (expiring with the TTL) in the `hostAction` handler, and reject a second `Check` of the same token with `forbidden.confirmation-invalid`. Bind the intent to the session as well, for example `Params: {"session": <session id hash>}` on both issue and check. A shorter TTL for host intents (for example 2 min) is a cheap addition.

### WR-06: The root-script security matrix skips silently when user namespaces are unavailable

**File:** `internal/host/hostaction/script_test.go:299-312` (and `internal/httpapi/hostactionsapi_test.go:114-122`)

**Issue:** Every test that runs the helper as root (the valid four, all injection payloads, symlink, FIFO, stale, directory, state-dir checks) goes through `requireHostNamespace`. That function calls `t.Skipf` when `unshare --user --map-root-user` fails. That is the default on Ubuntu 23.10+ and 24.04 runners (`kernel.apparmor_restrict_unprivileged_userns=1`) and in many containers. `go test` without `-v` prints `ok` for a package whose root matrix did not run. `TestUnitsVerify` deliberately fails loudly in the same situation unless an opt-out variable is set. The script matrix, the most security-relevant test in the phase, has no such guard. Per CLAUDE.md, that is an unperformed measurement reported as green.

**Fix:** Mirror `TestUnitsVerify`: on Linux, `t.Fatalf` when no user namespace can be made, unless something like `HOLZKUBE_MANAGER_NO_USERNS=1` is set, in which case skip with a "SKIPPED, not verified" message. Alternatively, run the matrix as real root in CI (`sudo -E go test -run TestHostScriptAsRoot`).

## Info

### IN-01: The process guard does not catch raw exec syscalls by number or `go:linkname`

**File:** `internal/processguard_test.go:33-43,163-179`

**Issue:** The guard catches the names `SYS_EXECVE`/`SYS_EXECVEAT`. It does not catch `syscall.RawSyscall(59, …)` / `unix.Syscall(221, …)` (the literal numbers on amd64 and arm64), or a `//go:linkname` to `syscall.forkExec`/`os.startProcess`. The rule text calls this test "the only thing that keeps the daemon from starting a process". The daemon unit's `SystemCallFilter=@system-service` allows `execve`.

**Fix:** Also report any `//go:linkname` directive and any `Syscall`/`RawSyscall`/`Syscall6` call whose first argument is not a named constant. Alternatively, soften the rule comment so it matches what the guard can actually see.

### IN-02: The comments say "no Environment=" while the guide tells operators to add one

**File:** `deploy/holzkube-manager-host.service:32-33`, `deploy/holzkube-manager-host.sh:51-53`, `deploy/HOST-HELPER.md:138`

**Issue:** The unit says "Kein Environment=: die Variablen … gibt es nur fuer die Tests". The script says "Im Betrieb setzt sie niemand". But the non-default data-directory section of `HOST-HELPER.md` has the operator set `Environment=HOLZKUBE_MANAGER_HOST_ORDER=…` in a drop-in. The overrides are therefore a production interface, which also covers `HOLZKUBE_MANAGER_SYSTEMCTL`, and the comments now understate it.

**Fix:** Update both comments to name the drop-in use of `HOLZKUBE_MANAGER_HOST_ORDER`. State that `HOLZKUBE_MANAGER_SYSTEMCTL` and `…_STATE_DIR` stay test-only.

### IN-03: The client and server compare the typed hostname differently

**File:** `internal/httpapi/handlers/host.go:217`, `web/src/components/HostActions.tsx:303,369`

**Issue:** The server accepts `TrimSpace(typed) == hostname`. The dialog enables the button only when `typed === hostname`. With a trailing space, pasted for example, the button stays disabled and the page gives no reason. The server rule and the API contract ("trimmed of surrounding blanks") are the looser ones.

**Fix:** Compare `typed.trim() === hostname` in the dialog as well, or drop the trim on the server and in the contract.

### IN-04: `Box.Order()` reports any `Lstat` error as `picked-up`

**File:** `internal/host/hostaction/hostaction.go:359-363`

**Issue:** `EACCES`, `EIO` and other errors are reported as "the helper took it". Only `fs.ErrNotExist` means that.

**Fix:** `if errors.Is(err, fs.ErrNotExist) { picked-up } else if err != nil { keep pending and log }`.

---

_Reviewed: 2026-09-29T04:05:06Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: deep_
