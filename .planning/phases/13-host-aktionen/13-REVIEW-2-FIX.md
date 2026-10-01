---
phase: 13-host-aktionen
fixed_at: 2026-10-01T00:00:00Z
review_path: .planning/phases/13-host-aktionen/13-REVIEW-2.md
iteration: 1
findings_in_scope: 4
fixed: 4
skipped: 0
status: all_fixed
---

# Phase 13: Code Review Fix Report (second review, WR-01..WR-04)

**Source review:** .planning/phases/13-host-aktionen/13-REVIEW-2.md
**Iteration:** 1
**Scope:** the four warnings. Info findings were not applied. IN-01 is not fixed
as such: WR-01 narrows it on the server side, because the routes measure from
the helper's record time, which comes after the placement.

**Summary:**
- Findings in scope: 4
- Fixed: 4 (WR-01 and WR-02 change logic and need a human to confirm them)
- Skipped: 0

## Where it ran

All of it ran on the operator's Raspberry Pi 5 (aarch64), in the main checkout
(`workflow.use_worktrees` is false), with Go from `~/.local/go` and Node through
nvm. There was no `-race`: ThreadSanitizer refuses this kernel. Nothing was
installed. `holzkube-manager.service` showed `ActiveEnterTimestamp=Tue 2026-09-29
20:25:38 CEST, NRestarts=0` at the end, the same as at the start of phase 13.
`/usr/local/sbin/holzkube-manager-host` and
`/etc/systemd/system/holzkube-manager-update-check.service` are absent. Nothing
was pushed.

## Fixed Issues

### WR-01: While the helper waits for a check, new orders were accepted, then withdrawn

**Files modified:** `internal/host/hostaction/result.go`, `internal/host/hostaction/result_test.go`, `internal/host/hostaction/units_test.go`, `internal/host/collector.go`, `internal/httpapi/handlers/host.go`, `internal/httpapi/problem.go`, `internal/httpapi/hosthelperapi_test.go`, `docs/api-contract.md`, `web/src/components/HostActions.tsx`, `web/src/components/HostActions.test.tsx`
**Commit:** e9d3e5e
**Status:** fixed: requires human verification (logic)
**Applied fix:** both host routes now refuse every action with 409
`conflict.host-helper-busy` while `host.Collector.CheckRunning()` is true. That
happens before any token is issued or checked and before any order file is
written. Refusal order is container → missing → outdated → busy. On the confirm
route the busy check comes after the outdated check and before the hostname is
compared; on the action route it comes before the body and the token.

`hostaction.CheckRunning(result, answered, now)` is true when all of these hold:
- the helper's last record is `check-update started`;
- the record is less than `HelperServiceLimit` old, and not in the future;
- the update status's `checked_at` is earlier than the record. An answer in the
  same second counts as over.

`HelperServiceLimit` is 3 min, and a unit test holds it to the helper service's
`TimeoutStartSec`.

**Deviation from the brief's wording:** the brief's condition was "a started
check younger than the limit". I added "and the update status has recorded no
run since". The helper records nothing when a check succeeds, so `started`
stays its last word. Without that clause the routes would refuse every order
for 3 minutes after every successful check, while the page shows the check as
finished. The added clause is the same reading the page already uses
(`update-finished`).

**Page:** `helperWaitsForCheck(host)` applies the same rule to a check the page
does not follow, such as one placed by another tab or by the API. The refusal's
detail is "An update check is running; wait for it to finish. No order was
placed." It begins with the page's `REASON.checkRunning`, and a Go test reads
that sentence from HostActions.tsx.

**Not changed:** the withdraw log line and the "not picked up" sentence still
name the path unit. With the refusal in place, a check can only cause a
withdrawal in the milliseconds between its status write and the end of the
unit.

**Red runs (exit codes read from the command itself):**
- The new `TestHostActionsWaitForARunningCheck` on the tree before the fix
  exited 1. All three "refused" rows and "outdated and busy" failed (e.g.
  `POST /api/v1/host/actions/reboot with a valid host token: 202, want 409
  conflict.host-helper-busy`, `a refused request placed an order`). All seven
  controls passed.
- The test was then restructured to take its times inside each subtest. The
  first version took them once at the start, and the full-package run (327 s
  on the Pi) aged "5 s ago" past the limit. The restructured test was seen red
  again with both refusals removed: exit 1, the same four failing.
- Each route's refusal was removed on its own (I1 action, I2 confirm). Both
  exited 1, naming the route that placed an order or issued a token.
- `CheckRunning` was made to ignore the update status (I3). Exit 1: "the
  check's answer arrived" and "the answer in the second the check started"
  were refused, and the TestCheckRunning row "answered in its own second"
  failed.
- `CheckRunning` was made to ignore the limit (I4). Exit 1: "a check started
  4 min ago" was refused, and the TestCheckRunning row "at the limit" failed.
- The busy refusal was moved before the outdated refusal on both routes (I5).
  Exit 1 with `code "conflict.host-helper-busy", want
  "conflict.host-helper-outdated"`.
- `HelperServiceLimit` was set to 4 min (I6). Exit 1 with `TimeoutStartSec is
  3m0s, HelperServiceLimit is 4m0s`.
- The page's `checkRunning` sentence was changed (I8). Exit 1 with
  `HostActions.tsx has no checkRunning: 'An update check is running; wait for
  it to finish.'`.
- Web, new rows before the page change: 2 failed (`while the helper waits for
  it`, `with only an older update status`) and the controls passed. Removing
  the page clause again (W1) gave the same 2. Ignoring the update status (W2)
  failed `once its answer arrived`. Ignoring the limit (W3) failed `once it
  started 3 min ago`.

Every injection was restored from a saved copy, `cmp` exited 0, and a green
run followed.

### WR-02: A failed check showed the generic sentence and the helper's journal

**Files modified:** `web/src/components/HostActions.tsx`, `web/src/components/HostActions.test.tsx`, `docs/api-contract.md`
**Commit:** c96fb0e
**Status:** fixed: requires human verification (logic)
**Applied fix:** the `failed` phase of a `check-update` order now reads "the
check could not look for a newer release, and nothing was installed.
`journalctl -u holzkube-manager-update-check` says why." The other actions keep
the helper's sentence. The contract now says where the reason is. The guide
already promised this.

New rows:
- two `orderPhase` rows with `resultFor(ID, 'check-update', 'failed')`, without
  and with a failed update status;
- a status row;
- a row that takes the phase from `orderPhase` over those readings and checks
  that the check unit's journal is shown and the helper's is not.

**Red run:** on the old sentence, exit 1 with 2 failed. Received was "Check for
updates — the helper could not carry it out. journalctl -u
holzkube-manager-host says why."

### WR-03: The check unit's worst case left out TimeoutStopSec

**Files modified:** `deploy/holzkube-manager-update-check.service`, `deploy/holzkube-manager-host.sh` (comment only), `internal/host/hostaction/units_test.go`
**Commit:** 7dc7b58
**Applied fix:** added `TimeoutStopSec=15s`, which gives 2 min 15 s against the
helper's 3 min. Both unit comments were updated. `TestTheCheckUnitRunsOnlyTheCheck`
now requires `TimeoutStopSec=` (it fails when the key is absent) and checks
that start + stop is below the helper service's `TimeoutStartSec`.

**Red runs:**
- The new test on the unit without the key: exit 1, "has no TimeoutStopSec=".
- `TimeoutStopSec=90s`: exit 1, `2m0s + 1m30s = 3m30s ... below ... 3m0s`.
- `TimeoutStopSec=1min`: exit 1, `= 3m0s` (the edge).

**systemd-analyze verify** on a scratch copy with a stub `ExecStart ... --check`
exited 0 with 0 bytes of output. `TestUnitsVerify` also passed, and its
negative control still reported `Unknown key 'ProtectHom'`. `bash -n` on the
helper exited 0.

### WR-04: The "runs only the check" guard did not rule out other commands or families

**Files modified:** `internal/host/hostaction/units_test.go`
**Commit:** 51f3cf7
**Applied fix:** the test now:
- requires exactly one `ExecStart=`;
- fails on every other `Exec*` key in any section;
- pins `RestrictAddressFamilies=` to exactly `AF_INET AF_INET6 AF_NETLINK
  AF_UNIX` (sorted);
- also forbids `User=`, `Group=`, `DynamicUser=` and `SupplementaryGroups=`.

**Red runs:** each injection was green against the old test (exit 0) and red
against the new one (exit 1):
- `ExecStartPost=/usr/local/sbin/holzkube-manager-update --force`: "...:52:
  [Service] ExecStartPost=... the check unit runs its one ExecStart= and
  nothing else";
- `AF_PACKET` added: "want exactly AF_INET AF_INET6 AF_NETLINK AF_UNIX";
- `DynamicUser=true`: "...:51: DynamicUser=true must not be in the check
  unit".

## Final verification (on the Pi, main checkout, exit codes read directly)

- `go test ./internal/... ./cmd/... -count=1`: exit 0, 41 `ok` packages, no
  FAIL lines. `publicrepo` is included.
- `./bin/task lint:go`: exit 0, 0 issues.
- `npx vitest run --project jsdom` (from web/): exit 0, 56 files, 773 tests.
- `npm --prefix web run test:browser`: exit 0, 5 files, 23 tests.
- `npm run lint`: exit 0. It reports the two warnings and one info that were
  already there, in `wall.test.tsx` and `DataTable.tsx`, not in files changed
  here. `npm run typecheck`: exit 0.

A commit not made by this fixer (466aa67, docs) landed between 7dc7b58 and
51f3cf7.

---

_Fixed: 2026-10-01_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_

## Round 2 — A1-helper-busy

**Source:** the adversarial verification of the fixes above, items V-01..V-04,
V-20..V-22, V-24, V-25. **Where it ran:** the operator's Raspberry Pi 5
(aarch64), main checkout, no worktree, Go from `~/.local/go` (no `-race`: this
kernel refuses ThreadSanitizer), Node through nvm. Root-namespace helper tests
ran (`unshare --user --map-root-user`, verified with `-v`: the
`TestHostScriptAsRoot` subtests report PASS, not SKIP). Nothing installed:
`/usr/local/sbin/holzkube-manager-host` and
`/etc/systemd/system/holzkube-manager-update-check.service` are absent before
and after. `holzkube-manager.service`: `ActiveEnterTimestamp=Tue 2026-09-29
20:25:38 CEST, NRestarts=0` before and after. Nothing pushed, no release, no
changelog entry.

**Correction of the report above.** Its "Not changed" paragraph under WR-01
said a check could cause a withdrawal only in the milliseconds between its
status write and the end of its unit. That was wrong (V-20): the update script
writes `checked_at` when it exits, so an hourly or manual run that ended
during a check unlocked the routes for the rest of the check. The design below
removes the update status from the question.

### Design: the helper's own word

The recommended design, without deviation in substance:

- The helper records the end of a check: after its blocking
  `systemctl start holzkube-manager-update-check.service` returns 0 it writes
  `<id> check-update done`; non-zero still writes `failed`. The four other
  orders keep `started` as their last word (their `systemctl` queues a job or
  restarts a service). `ReadResult` accepts `done` for `check-update` only and
  still refuses `done` with `- -` or for any other action; the root script
  tests parse every record with it, so writer and reader stay one format.
- `hostaction.CheckRunning(r, boot, now)`: the last record is a `check-update`
  `started`, `0 <= age < HelperServiceLimit`, and not from before the last
  boot (`r.At + 1 s > boot`, the record being rounded down to its second;
  boot unknown means only the limit counts). The update status has no say.
- `hostaction.Box.Busy()` = that, or a check this Box placed that the helper
  has taken (file gone, not withdrawn) and recorded nothing for yet, for at
  most `ResultWithin` (1 min, the page's `RESULT_WITHIN_MS`). `Box.Place` asks
  `busyLocked()` under the slot's mutex and returns `ErrBusy`; the action
  route answers that as the same 409. Boot comes from `SinceBoot`
  (CLOCK_BOOTTIME, `host.OS().BootTime`, wired in `main.go`).
- **One deviation, with its reason:** the page no longer recomputes the rule.
  `GET /api/v1/host` carries `actions.busy` (= `Box.Busy()`), and the page
  turns its buttons off by it. A second copy of the rule in TypeScript was the
  drift V-04(c)/V-23 describe, and the Box's memory of its own check cannot be
  mirrored from the page anyway. `Collector.CheckRunning` is gone.
- On the page a check is finished only by the helper's `done` (or `failed`),
  never by an update status that appears meanwhile; its answer is the update
  status only when that was recorded within the check (no older than the
  placement, or than 3 min before the `done` record for an order the page
  knows only from that record), otherwise the box says, in slate, that what it
  found is not in the update status this page reads.

**The frozen older helper (testdata, 8b64a06)** never records a check: it
rejects `check-update <id>` as an unknown form and records `- - rejected`, and
the daemon's `Outdated` keeps it from being placed (409
`conflict.host-helper-outdated`). Its records therefore never make `busy` true;
`TestAnOlderHelperRefusesTheCheck` and the outdated API tests stay green
unchanged. A helper from main between aa4d4e9 and 1388949 (knows the check,
does not record `done`) would hold the routes for 3 min after each successful
check; it was never released (`git tag --contains aa4d4e9` is empty) and is
not installed on this Pi. Nothing tells it apart from the current helper, by
decision: the marker line names orders, not records.

### Per item

- **V-01 [warning] — fixed** (1388949, 29197b2, and the last commit). Busy is
  the helper's own record; the hourly run, a manual run or a future-dated
  status no longer ends it (API rows "an hourly run recorded since", "an update
  status in its own second", "an update status from the future" are refused).
  The withdrawal diagnosis: under the new rule a busy helper can make an order
  lie only when the wall clock misleads the age test (a record dated in the
  future after a step back, a forward step past the limit). For that remainder
  the Box reads the helper's record when it withdraws (`CheckHeld`: another
  order's check with no end since boot, or one that ended after this placement)
  and then names the check unit's journal instead of the path unit; the page's
  "not picked up" sentence says the helper was busy with a check. `CheckHeld`
  is in Go and its page copy is `checkHeld`.
- **V-02 [info] — fixed.** A reboot or power loss mid-check no longer holds
  the routes (boot clause); a successful check whose answer the daemon cannot
  read no longer holds them (the update status is not asked). What remains: a
  helper killed mid-check (systemd's own limit, a manual kill) holds them up
  to 3 min from its record, which is when systemd has ended it anyway.
- **V-03 [info] — fixed.** The Box's memory covers the window between the
  helper's rename and its `started` record.
- **V-04 [info] — fixed.** (a) `TestABusyRefusalKeepsTheToken`: refused as
  busy, the same token replayed after the check is `done` gets 202. (b) the
  page's `age >= 0` guard no longer exists (the page reads `actions.busy`);
  the Go guard is held by `TestCheckRunning` "recorded in the future" and the
  API row "a check recorded in the future". (c) `TestHostActionsWaitForARunningCheck`
  now reads `CHECK_WITHIN_MS = 3 * 60 * 1000` and `RESULT_WITHIN_MS = 60 * 1000`
  from HostActions.tsx against `HelperServiceLimit` and `ResultWithin` (this
  overlaps V-23, which another fixer owns; the buttons themselves no longer
  depend on `CHECK_WITHIN_MS`).
- **V-20 [info] — fixed** (the correction above, and the diagnosis now reads
  the helper's record).
- **V-21 [info] — fixed.** No cause it lists can hold the routes any more:
  none of them is a `started` record without an end, except the power loss,
  which the boot clause ends.
- **V-22 [info] — fixed.** `Place` refuses under its lock (`ErrBusy`);
  `TestAHelperBusyAtPlacementIsRefusedAsBusy` shows the route answers it as
  409 `conflict.host-helper-busy` with the token spent and nothing placed.
- **V-24 [info] — fixed.** HOST-HELPER.md (the order table's note, the `last`
  line with `done`, the exit codes), docs/api-contract.md (record fields, the
  check's records, the busy row, `actions.busy`, the withdrawal warning), and
  docs/guide.md (the `failed` bullet names the check unit's journal for a
  check -- this overlaps V-05 --, the "not picked up" bullet, and "One order at
  a time" now says a running check turns all five off, for at most 3 min and
  not past a reboot).
- **V-25 [info] — fixed.** As root: `check-update` against a stand-in
  `systemctl` that exits 1 records `<id> check-update failed`, exit 1; one that
  exits 0 records `done`, exit 0; in both the stand-in saw `started` for that
  id while it ran.

### Red runs (exit codes read from the command itself; every fault restored, `cmp` 0, then green)

Helper and reader (`go test ./internal/host/hostaction -run 'TestHostScriptAsRoot|TestReadResult'`, each exit 1):
no `done` record (`valid_check-update`); `|| true` around systemctl (both
"systemctl fails" rows: `last = ... "done"/"started", want "failed"`);
`--no-block` for the check; `done` for every order (four `valid_*` rows);
`started` recorded after systemctl (all seven); reader accepting `done` for
any action; reader without `done`; reader accepting `- - done`.

Busy (exit 1 each): `CheckRunning` ignoring the boot (unit row, Box row, API
row "the machine up for 20 s"); the future-record guard removed
(`TestCheckRunning`, `TestTheWithdrawalNamesABusyHelper`); the Box's memory
removed; `Place` unguarded (Box tests and the placement-race API test);
`ResultWithin` replaced by 3 min ("a minute without a record"); **the busy
refusal moved after `CheckOnce`** (`TestABusyRefusalKeepsTheToken`: replay
403 `forbidden.confirmation-invalid`, the only failing test); the `ErrBusy`
mapping removed (placement-race test, 500); the confirm route's refusal
removed (all six busy rows); `actions.busy` not sent (six rows);
`RESULT_WITHIN_MS` and `CHECK_WITHIN_MS` changed in HostActions.tsx.

**An injection that first did not inject:** reinstating "an update status at
or after the record ends the check" inside `Box.busyLocked` left the API test
green (exit 0). The harness gave the Box a filesystem without the status file
that the host reader saw, so the reinstated clause had nothing to read. The
fixture now puts the status on both, as on the host, and the same injection
exits 1 with the three V-01 rows failing.

Diagnosis (exit 1 each): the warning ignoring `CheckHeld`; `CheckHeld`
ignoring the boot; `CheckHeld` taking any end as after the placement.

Page (vitest jsdom, exit 1 each): no `done` branch in `orderPhase`; no
tolerance for an order known only from its `done` record; no freshness for a
done check's answer; the schema without `done`; `actions.busy` ignored; a check
finished by the update status; `checkHeld` ignored; `checkHeld` ignoring the
boot.

### Final suites (Pi, main checkout)

- `go test ./internal/... ./cmd/... -count=1`: exit 0, 41 `ok`, no other lines.
- `./bin/task lint:go`: exit 0, 0 issues.
- `npx vitest run --project jsdom` (web/): exit 0, 56 files, 783 tests.
- `npm --prefix web run test:browser`: exit 0, 5 files, 23 tests.
- `npm run typecheck`: exit 0; `npm run lint`: exit 0.
- `go test ./internal/publicrepo/` before each commit: exit 0.
