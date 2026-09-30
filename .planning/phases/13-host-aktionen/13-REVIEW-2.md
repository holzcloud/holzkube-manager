---
phase: 13-host-aktionen
reviewed: 2026-09-30T16:11:27Z
depth: standard
files_reviewed: 39
files_reviewed_list:
  - deploy/holzkube-manager-host.sh
  - deploy/holzkube-manager-host.service
  - deploy/holzkube-manager-update-check.service
  - deploy/HOST-HELPER.md
  - .goreleaser.yaml
  - internal/audit/redact.go
  - internal/host/collector.go
  - internal/host/host.go
  - internal/host/actions_test.go
  - internal/host/hostaction/helper.go
  - internal/host/hostaction/helper_test.go
  - internal/host/hostaction/hostaction.go
  - internal/host/hostaction/result.go
  - internal/host/hostaction/result_test.go
  - internal/host/hostaction/script_test.go
  - internal/host/hostaction/units_test.go
  - internal/host/hostaction/testdata/holzkube-manager-host-before-check.sh
  - internal/host/updatestatus/script_test.go
  - internal/httpapi/handlers/host.go
  - internal/httpapi/handlers/confirm_test.go
  - internal/httpapi/problem.go
  - internal/httpapi/router.go
  - internal/httpapi/hostactionsapi_test.go
  - internal/httpapi/hostgatesapi_test.go
  - internal/httpapi/hosthelperapi_test.go
  - cmd/holzkube-managerd/budget_test.go
  - web/src/api.ts
  - web/src/api.nulls.test.ts
  - web/src/components/HostActions.tsx
  - web/src/components/HostActions.test.tsx
  - web/src/routes/host.tsx
  - web/src/routes/host.test.tsx
  - web/src/routes/host.browser.test.tsx
  - web/src/fixtures.test.ts
  - web/fixtures/demo.json
  - web/fixtures/host-helper-installed.json
  - docs/api-contract.md
  - docs/guide.md
  - README.md
findings:
  critical: 0
  warning: 4
  info: 4
  total: 8
status: issues_found
---

# Phase 13: Code Review Report (second review: plans 13-12 to 13-15)

**Reviewed:** 2026-09-30T16:11:27Z
**Depth:** standard
**Files Reviewed:** 39
**Status:** issues_found

## Summary

This review covers the diff `7e1f672..HEAD`: the fifth host action `check-update`, the root helper's blocking `systemctl start` for it, the new check unit, the marker-line reader (`KnownOrders`/`Outdated`), the 409 `conflict.host-helper-outdated` refusal, the web button, dialog and notices, and the docs. It was read on the operator's Pi (aarch64). Nothing was run against the installed system, and no source file was changed.

**What holds up:**
- **Helper script.** The new word only widens the anchored alternation and adds one case arm with a fixed argv. Nothing from the order reaches the command line, and the claim/consume/symlink handling is unchanged. A daemon that controls the data directory still chooses only which branch runs.
- **Marker reader.** It gates on `scriptInstalled` (uid 0, not group- or world-writable), re-stats, caps the read with `LimitReader(Max+1)`, and never quotes the file. Its only TOCTOU window is against root.
- **Refusal order and locks.** Container, then missing, then outdated, all before the body or token on the action route and before the hostname compare and token issue on the confirm route. The four locks (session, operator, sudo window, typed hostname) and the audit action come from the route loop over `Actions()`. The daemon still starts no process.
- **Dialog lock.** The CR-01 lock (`!open && !run.isPending`, Keep running disabled while pending) is unchanged and applies to the check dialog.
- **Public repo.** No identifying values in the diff; `internal/publicrepo` passes (exit 0).

**What is wrong:**
- The blocking check creates a new window, up to 2 minutes or more, in which the helper is busy but the daemon accepts and then withdraws orders, and blames the path unit when it does (WR-01).
- The usual way a check fails is shown with the wrong journal and with a sentence that is never tested (WR-02).
- The "2 min under 3 min" timeout argument leaves out the stop timeout (WR-03).
- The guard that the check unit "runs only the check" does not rule out extra `Exec*=` lines or extra address families (WR-04).

## Warnings

### WR-01: While the helper waits for a check, new orders are accepted, then withdrawn after 10 s with a wrong diagnosis

**File:** `deploy/holzkube-manager-host.sh:217`, `internal/host/hostaction/hostaction.go:292-335` (Place), `:343-365` (withdraw), `internal/httpapi/handlers/host.go:303-345`
**Issue:** Before this change, the helper service never stayed busy while the daemon was up and taking orders. `update` used `--no-block`, and `reboot`/`poweroff` return at once. During `restart-service` the daemon itself is stopping.

`check-update` now keeps `holzkube-manager-host.service` in `activating` for up to `TimeoutStartSec=2min` (WR-03: longer in the worst case). The problem:
- `Box.Place` refuses a second order only while the order *file* exists (`ErrPending`). The helper renamed the check order away before it started waiting, so the daemon accepts the next order: a reboot, an update or a second check.
- `holzkube-manager-host.path` (`PathExists=`) cannot start a second instance of a unit that is still activating. Its start job merges into the running one.
- After `DefaultPickupTimeout` (10 s) the daemon withdraws the order. It logs "check `systemctl status holzkube-manager-host.path`", and the page shows "the helper did not pick up the order within 10 s ... Check that the helper is running". Both are wrong: the path unit is fine, and the helper is busy.

The web page closes most of this window by turning all five buttons off while the check is `started` (`HostActions.tsx:238-247`). But the design says the server is the lock ("The server is the lock (the routes refuse ... a second order)"). The window stays open to API clients, and to the page as soon as the check reads as final before the helper has finished (see IN-01).

**Fix:** Refuse on the server while the helper is known to be busy with a check. For example, in the action handler before `Place` (or inside `Box.Place`):
```go
// The helper blocks on a running check; nothing else can be picked up.
if r, err := d.HostActions.Result(); err == nil &&
    r.Action == hostaction.CheckUpdate && r.Outcome == hostaction.OutcomeStarted &&
    now.Sub(r.At) < helperCheckLimit /* the helper service's TimeoutStartSec */ {
    httpapi.WriteProblem(w, r, httpapi.Conflict(httpapi.CodeHostOrderPending,
        "An update check is still running. Wait for it to finish, then try again."))
    return
}
```
Once the busy check exists, the "not picked up" sentence and the withdraw log line should also stop naming the path unit as the cause. The other option is to have the check not block the helper: `--no-block`, plus an `OnFailure=`/`ExecStopPost=` on the check unit that records the failure in its own state directory.

### WR-02: The usual way a check fails shows the generic "failed" sentence and points at the wrong journal

**File:** `web/src/components/HostActions.tsx:689-691`, `:847-848`; `docs/guide.md:248`; `docs/api-contract.md` ("What `check-update` runs")
**Issue:** When the check cannot look (GitHub unreachable, the update script missing, the unit timed out), this happens in order:
1. The update script's `on_exit` writes `status.json` with `outcome: failed`.
2. The unit fails and `systemctl start` exits non-zero.
3. The helper records `<id> check-update failed`. The contract documents this: "`started` ... becomes `failed` when the check could not look".

`orderPhase` returns `result.value.outcome` whenever it is not `started` (line 689), before it looks at the update status. So in the steady state the box says "the helper could not carry it out. `journalctl -u holzkube-manager-host` says why." The helper's journal only says "Auftrag ... (check-update) gescheitert"; the reason is in `journalctl -u holzkube-manager-update-check`.

The check-specific sentence ("The check failed, and nothing was installed. `journalctl -u holzkube-manager-update-check` says why.", `checkSentence`) only shows if a poll lands in the milliseconds between steps 1 and 3. The guide promises that sentence ("A check that fails says so; `journalctl -u holzkube-manager-update-check` says why").

The test row "check finished, the check failed" (`HostActions.test.tsx:1728`) drives `update-finished` directly. No row anywhere uses `resultFor(ID, 'check-update', 'failed')`, so the real path is untested.

**Fix:** Give a failed check its own sentence in `phaseSentence`, and add an `orderPhase` row plus a status row for `resultFor(ID, 'check-update', 'failed')`:
```tsx
case 'failed':
  return order.action === 'check-update'
    ? <>the check could not look for a newer release, and nothing was installed. {CHECK_JOURNAL} says why.</>
    : <>the helper could not carry it out. {JOURNAL} says why.</>
```

### WR-03: "The check ends below the helper's limit" leaves out TimeoutStopSec, so the worst case is 3.5 min, not 2

**File:** `deploy/holzkube-manager-update-check.service:30-33,50`, `deploy/holzkube-manager-host.sh:211-216`, `internal/host/hostaction/units_test.go` (`TestTheCheckUnitRunsOnlyTheCheck`, the `lastSpan` comparison)
**Issue:** When a oneshot hits `TimeoutStartSec=2min`, systemd sends SIGTERM and waits up to `TimeoutStopSec` (default `DefaultTimeoutStopSec=90s`) before SIGKILL. The start job, and with it the helper's blocking `systemctl start`, only returns once the unit has really stopped. The bound is therefore 2 min + 90 s = 3.5 min, which is more than the helper service's `TimeoutStartSec=3min`.

When the process does not end on SIGTERM, systemd kills the helper before it can record `failed`, which is exactly what the unit comment and the helper comment say the 2-minute limit prevents. The test compares only the two `TimeoutStartSec` values, so it cannot see this. In normal runs the script's `trap 'exit 143' TERM` and curl's `--max-time 30` end things quickly; the problem is that the stated guarantee is not enforced.

**Fix:** Bound the stop phase in the check unit, and have the test check the sum:
```ini
TimeoutStartSec=2min
TimeoutStopSec=15s
```
In the test, `c + stop < h`, where `stop` is `TimeoutStopSec` (fail if it is absent).

### WR-04: TestTheCheckUnitRunsOnlyTheCheck would pass a unit that also runs the full update as root, or opens more address families

**File:** `internal/host/hostaction/units_test.go:601`, `:636-646`, `:665`
**Issue:** The test's claim is that the unit runs `UpdateScriptPath --check` "and nothing else". But:
- `wantOnly(..., "ExecStart", ...)` only looks at the key `ExecStart`. The forbidden-key list (`RemainAfterExit, Environment, EnvironmentFile, AmbientCapabilities, ReadWritePaths`) does not include `ExecStartPre`, `ExecStartPost`, `ExecCondition`, `ExecStop`, `ExecStopPost` or `ExecReload`. A line `ExecStartPost=/usr/local/sbin/holzkube-manager-update --force` (a full root install on every check) passes this test. `TestUnitsVerify`'s `^ExecStart=` rewrite does not catch it either.
- The `RestrictAddressFamilies` check only asserts that `AF_INET` and `AF_INET6` are present. `AF_PACKET` or `AF_BLUETOOTH` could be added without the test failing, although the brief asks for network access "and nothing more".
- `User=`/`Group=`/`DynamicUser=` are not pinned. That is harmless today, but the sandbox reasoning (root with an empty bounding set) depends on them.

This is a guard over root code that does not cover what it claims to cover.

**Fix:** Forbid every `Exec*` key except `ExecStart` (e.g. iterate `check.assignments` and fail on `strings.HasPrefix(a.key, "Exec") && a.key != "ExecStart"`). Pin the families exactly:
```go
if !slices.Equal(sorted(got), []string{"AF_INET", "AF_INET6", "AF_NETLINK", "AF_UNIX"}) { t.Errorf(...) }
```
Then inject `ExecStartPost=` and an extra family, and see the test go red.

## Info

### IN-01: `checked_at >= placedSecond` lets an hourly run finish the check early, and with it turn the buttons back on

**File:** `web/src/components/HostActions.tsx:692-700`
**Issue:** Truncating the placement to the second means a `status.json` written by the hourly timer earlier in the same second, or by an hourly run that was already going when the check was placed, marks the check `update-finished`. That phase is final, so all five buttons come back on while the helper is still blocked on the real check, which leads straight into WR-01. The chance is small (about a one-second window per hour, or overlap with a running hourly update), and the sentence then shows the hourly outcome, which the code accepts on purpose. Keep it in mind together with WR-01.
**Fix:** For `check-update`, also require that the helper has recorded something other than `started`, or keep `disabledReason` returning `checkRunning` until the result is final or `CHECK_WITHIN_MS` has passed.

### IN-02: A check and the hourly update can run the update script at the same time

**File:** `deploy/holzkube-manager-update-check.service`, `deploy/holzkube-manager-update.sh:124-180` (no lock), `web/src/components/HostActions.tsx:237-239`
**Issue:** The page comment says an update beside a running check "would run the update script twice at once", and the page blocks the update *button*. The hourly `holzkube-manager-update.service` is a different unit, and nothing orders it against the check unit. The script has no lock and `status.json` is last-writer-wins.

A check that overlaps an hourly install works out `OUTCOME=available` from the pre-install `LOCAL_VERSION`, then re-reads `INSTALLED` at exit. It can therefore overwrite the hourly `updated` record with `available` and installed == latest (the updated record is lost, and the reader may refuse the file).
**Fix:** Take a lock in the status directory for both modes (e.g. `exec 9>"$STATUS_DIR/.lock"; flock -w 60 9`). That touches the update script, which D-19 kept unchanged, so it is a decision for the operator. Or add `Conflicts=`/`After=holzkube-manager-update.service` to the check unit.

### IN-03: The check unit's sandbox could be tighter

**File:** `deploy/holzkube-manager-update-check.service:54-72`
**Issue:** The helper service leaves out `SystemCallFilter=` because of logind/reboot. That reason does not apply to the check unit, which also has an empty `CapabilityBoundingSet=`. `SystemCallFilter=@system-service` (with `SystemCallErrorNumber=EPERM`), `ProtectProc=invisible`, `ProcSubset=pid`, `PrivateIPC=true` and `RemoveIPC=true` would cost nothing for curl, tar-less `--check`, python3 and bash. `IPAddressDeny=` for link-local and RFC1918 ranges would stop a root process with the release token from reaching the LAN, although `REPO=` in `update.conf` could legitimately point elsewhere.
**Fix:** Add the keys, verify with `systemd-analyze security holzkube-manager-update-check.service`, and extend the test's key list.

### IN-04: `UpdateScriptPath` is exported but `Outdated` never checks it

**File:** `internal/host/hostaction/helper.go:77-80`, `:283-296`
**Issue:** `UpdateScriptPath` is used only by tests. The check unit's `ExecStart` needs `/usr/local/sbin/holzkube-manager-update`, which the helper install commands do not install; it is the reference installation's update script. On a systemd host with the helper but without that script, the check button is on and every check fails. Because of WR-02, the page then points at the helper's journal.
**Fix:** Either report a third `outdated`/`missing` item when `UpdateScriptPath` is not a root-owned executable, or say in the older-helper notice and the guide that the check needs the reference update script.

---

_Reviewed: 2026-09-30T16:11:27Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
