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

## Round 2 — A2-page-docs

**Source:** the adversarial verification, items V-05, V-06, V-07, V-08, V-23,
V-27, and the page side of A1's commits (1388949, 29197b2, e3d56b2).
**Where it ran:** the operator's Raspberry Pi 5 (aarch64), main checkout, no
worktree; Go from `~/.local/go` (no `-race`), Node through nvm. Nothing
installed (`/usr/local/sbin/holzkube-manager-host` and
`/etc/systemd/system/holzkube-manager-update-check.service` absent before and
after); `holzkube-manager.service` `ActiveEnterTimestamp=Tue 2026-09-29
20:25:38 CEST, NRestarts=0` before and after. Nothing pushed, no release, no
changelog entry.

**The page side of A1, checked first.** A1 already moved the page onto the
helper's own word: the buttons follow `actions.busy` (`helperWaitsForCheck` is
gone), and `orderPhase` finishes a check only on `done` and fails it on
`failed`. What was left on the page is below (V-07: the answer bounded by the
`done` record, one failed sentence; the no-answer sentence derived from
`CHECK_WITHIN_MS`).

### Per item

- **V-05 [warning] — fixed** (by A1 in e3d56b2; verified here). The guide's
  status-box bullet now reads "`journalctl -u holzkube-manager-host` says why
  -- for a failed **Check for updates**, `journalctl -u
  holzkube-manager-update-check`, since the check itself is what failed",
  matching the page and the guide's own action list. No further change; docs
  carry no guard.
- **V-06 [info] — fixed.** 13-UI-SPEC.md gets "Additions for Check for updates":
  every check-specific string as HostActions.tsx says it (dialog, reasons,
  each status phase including the single failed sentence, the busy
  not-picked-up sentence, the outdated notice, the two server details), with
  the rules that go with them (finished only by `done`, one failed sentence,
  whom the reason line describes). Each string was grepped against the source
  (all present).
- **V-07 [info] — fixed** (5edc2b9). Since 1388949 the poll-to-poll flip V-07
  describes cannot happen (a check is `started` until the helper's `done` or
  `failed`). What remained: a check that ended `done` while its own status
  says `failed` (it could not read the installed version: `record_status`
  turns rc 0 with an empty version into failed) said "finished. The check
  failed..." while a helper-failed check said "the check could not look for a
  newer release...". Both now say one `CHECK_FAILED`: "the check failed, and
  nothing was installed. `journalctl -u holzkube-manager-update-check` says
  why." -- red. And the check's answer is bounded by the helper's `done`: an
  update status newer than it is a later run's, so a check that found "up to
  date" no longer turns into "the check failed" when the next hourly run fails.
  New test "one failed check is said one way" (four arrivals, one sentence;
  the moment between status and record still says started; a later failed run
  is not the check's).
- **V-08 [info] — fixed** (5edc2b9). The orderPhase rows' comment no longer
  claims WR-02; they now hold what they say -- a failed record is `failed`, not
  finished as `done` is, and not finished by an update status recorded while
  the check ran (new rows: an hourly "up to date" during it; the poll failing).
  Both faults turn them red (below).
- **V-23 [info] — fixed** (tie by A1 in e3d56b2, verified red here; 23a937e).
  `TestHostActionsWaitForARunningCheck` reads `CHECK_WITHIN_MS` from the page
  against `HelperServiceLimit`; the buttons no longer depend on it (they
  follow `actions.busy`). The last copy of the limit, the literal "within 3
  min" in the no-answer sentence, is now said from `CHECK_WITHIN_MS`.
- **V-27 [info] — fixed** (aa9e279). The reason line describes the fieldset
  only when the group's reason turns all five off; when only the check is off
  (helper too old) it describes the check button, and the four that are on
  carry no description. Test by attribute and by accessible description; the
  group-reason helper also asserts no button repeats the group's description.

### Red runs (exit code read from the command itself; every fault restored, `cmp` 0, then green)

V-27, `npx vitest run --project jsdom src/components/HostActions.test.tsx`
(exit 1 each): the page as at e3d56b2 (`git show HEAD:` -- `cmp` 0 --, 2
failed, the two outdated rows); the check button without its
`aria-describedby` (2 failed); every button described whenever there is a line
(23 failed). Restored, 159 passed.

V-07/V-08, same command (exit 1 each): the page as at aa9e279 (8 failed); the
`done` record not bounding the answer (1: "a check done, then a later hourly
run failed"); a check's `failed` record finished like `done` (5, the four
orderPhase failed-check rows among them); the update status asked before the
helper's word, for a check too (4: "check failed, the update status failed
too", "...an hourly run during it said up to date", "check started, an update
status since", "the status written, the helper not yet done"); a failed answer
said through `checkSentence` again (4). Restored, 167 passed.

V-23, `go test ./internal/httpapi -run 'TestHostActionsWaitForARunningCheck$'`
(exit 1 each): `CHECK_WITHIN_MS = 4 * 60 * 1000` on the page; `HelperServiceLimit
= 4 * time.Minute` in result.go with the page at 3 (both name line 663, "the
page and hostaction.HelperServiceLimit disagree"). Restored (`cmp` 0), exit 0.

### Screenshots

No rendered screen changed: the demo fixture has no order and no outdated
helper, so host.png shows neither the status box nor the check's reason; the
V-27 change is an attribute. `readme-images.mjs` not run, no picture committed.

### Final suites (Pi, main checkout)

- `go test ./internal/... ./cmd/... -count=1`: first run exit 1 --
  `internal/upgrade` `TestANodeSaysHowItBooted` timed out against its fake
  Talos under the load of the full run (no file of that package touched);
  alone exit 0; the full run again exit 0, 41 `ok`, no other lines.
- `./bin/task lint:go`: exit 0, 0 issues.
- `npx vitest run --project jsdom` (web/): exit 0, 56 files, 791 tests.
- `npm --prefix web run test:browser`: exit 0, 5 files, 23 tests.
- `npm run typecheck`: exit 0; `npm run lint`: exit 0 (its 2 warnings and 1
  info are in DataTable.tsx and wall.test.tsx, untouched).
- No class changed, so `task test:layout` was not needed.
- `go test ./internal/publicrepo/` before each commit: exit 0.

## Round 2 — B-unit-guard

Owner of V-09..V-19 (and V-26 as a human item). Ran on the operator's Pi
(aarch64, systemd 257.13), main checkout, no worktree; Go 1.27.1 from
`~/.local/go`, no `-race`. Nothing installed: the helper units, the check unit
and `/usr/local/sbin` are untouched, and the only systemd units started were
transient units in the user manager (`systemd-run --user`) for the timing
measurement. `holzkube-manager.service` before and after:
ActiveEnterTimestamp `Tue 2026-09-29 20:25:38 CEST`, NRestarts 0.

Commits: `95d5be0` (timeout model, TimeoutStopSec=10s), and the allow-list
commit that carries this section.

### Design

- **`readUnitFile` refuses what systemd would read differently** (V-15):
  `refuseAmbiguousUnitBytes` fails on invalid UTF-8, a byte-order mark
  anywhere, any control character but tab and newline (so `\r` and NUL), and
  any line ending in a backslash, comments included. `TestUnitsVerify` runs
  the same guard over its three sources before its `^ExecStart=` rewrite.
- **Allow-lists, not deny-lists** (V-14, V-16..V-19, V-10..V-12):
  `wantExactly(u, sections, spec)` requires exactly the listed sections in
  order, and exactly the listed keys, each once, each with its value (exact,
  or a check function). Any other key fails naming `file:line`. The check
  unit's list: `[Unit]` Description (non-empty) and Documentation (the
  guide); `[Service]` Type, ExecStart (`UpdateScriptPath --check`),
  TimeoutStartSec and TimeoutStopSec (positive spans, then the model),
  StateDirectory, CapabilityBoundingSet (empty), RestrictAddressFamilies (the
  four, any order), and the nineteen hardening lines.
- **Same discipline for the helper's service and path unit** (task item;
  `TestUnitsAgree` was a deny-list for both): the service list carries
  `TimeoutStartSec` with a check that it equals `HelperServiceLimit` (so
  `TimeoutSec=` or another value cannot desynchronise the routes), and
  `ReadWritePaths=-/var/lib/holzkube-manager`, `RestrictAddressFamilies=AF_UNIX`
  plus the hardening; the path unit's list is `PathExists=`, `Unit=` and
  `WantedBy=paths.target`. `TestTheCheckUnitRunsOnlyTheCheck` still pins
  `HelperServiceLimit` to the service on its own, refusing `TimeoutSec=`.
- **The timeout model is systemd's** (V-09, V-13): `checkUnitWorstCase =
  start + 4 x stop` (stop-sigterm, stop-sigkill, final-sigterm,
  final-sigkill, each armed with TimeoutStopSec; true only because the list
  rules out SendSIGKILL=, FinalKillSignal=, TimeoutStartFailureMode=,
  TimeoutAbortSec=, TimeoutSec=, ExecStop*/ExecStopPost and ordering). The
  test wants it at least `checkUnitMargin` (15 s) below the helper's limit.
  Measured on the Pi, transient user units, TimeoutStartSec=3s
  TimeoutStopSec=2s: 3.25 s (killed by SIGTERM), 5.47 s (ignores SIGTERM),
  11.98 s (KillSignal= and FinalKillSignal=SIGCONT, surviving every signal) =
  3 + 4 x 2. The unit goes to `TimeoutStopSec=10s`: worst case 2min40s, 20 s
  under 3min. The comments in the unit and in `holzkube-manager-host.sh` now
  state that bound and why the kill keys are absent; the busy rows that named
  "2 min 15 s" (hostaction_test.go, hosthelperapi_test.go) now name 2 min 40 s.

### Per item

| ID | Outcome |
|----|---------|
| V-09 | fixed: model start + 4 x stop with a 15 s margin; TimeoutStopSec 15s -> 10s (95d5be0) |
| V-10 | fixed: TimeoutSec= is off both services' lists; the helper's TimeoutStartSec is checked against HelperServiceLimit in the allow-list, and the span reader refuses TimeoutSec= |
| V-11 | fixed: SendSIGKILL= is off the list |
| V-12 | fixed: FinalKillSignal=, TimeoutStartFailureMode=/TimeoutAbortSec=, After= are off the list |
| V-13 | fixed: unit and helper-script comments state 2min40s = 2min + 4 x 10s and name the keys that would change it (95d5be0) |
| V-14 | fixed: [Unit] allows Description and Documentation only |
| V-15 | fixed: refuseAmbiguousUnitBytes (CR, NUL, BOM, control chars, invalid UTF-8, trailing backslash), in readUnitFile and TestUnitsVerify |
| V-16 | fixed: BindPaths=, BindReadOnlyPaths=, RootDirectory= (and every other mount key) are off the list |
| V-17 | fixed: PassEnvironment= is off the list |
| V-18 | fixed: StandardOutput=/StandardError= are off the list |
| V-19 | fixed: as V-14, the guard is an allow-list |
| V-26 | human verification item, below |

### Red runs (exit code read from the command itself; each fault restored, `cmp` 0 against the pre-injection copy and against `git show HEAD:`, then green)

Commit 95d5be0, `go test ./internal/host/hostaction -run
TestTheCheckUnitRunsOnlyTheCheck -count=1`: the unit as at the commit before
(`TimeoutStopSec=15s`) exit 1, "TimeoutStartSec 2m0s + 4 x TimeoutStopSec 15s
= 3m0s ... want ... at least 15s below the helper service's 3m0s";
`TimeoutStopSec=12s` exit 1; `11s` exit 0 (the boundary, 2m44s + 15 s =
2m59s). Restored, exit 0.

Allow-list commit, a script writing each injection byte-exact into the
shipped file, running `go test ./internal/host/hostaction -run
'^(TestUnitsVerify|TestUnitsAgree|TestTheCheckUnitRunsOnlyTheCheck)$'
-count=1`, and restoring; 39 injections, every one `exit=1`, every restore
`cmp` 0, then the whole package exit 0. Into the check unit: OnSuccess=,
Wants=, OnFailure=, After=holzkube-manager-update.service; `# nur
nachsehen\rExecStartPost=... --force`; `\rExecStart=\rExecStart=... --force`;
`# x\0ExecStartPost=...`; `\xEF\xBB\xBFExecStartPost=...`; `# x\rRestrictAddressFamilies=`;
a comment ending in `\` before ExecStart=; `Type=oneshot \` (an assignment
continued into ExecStart=); TimeoutSec=5min; SendSIGKILL=no;
FinalKillSignal=SIGCONT; TimeoutStartFailureMode=abort + TimeoutAbortSec=5min;
BindPaths=/usr/local/bin; BindReadOnlyPaths= over update.conf;
RootDirectory=; PassEnvironment=BASH_ENV ...; StandardOutput=file: and
truncate: on the daemon binary; TimeoutStopSec=15s, 31s, absent, 90s;
--check removed; ExecStartPost=... --force; AF_PACKET; DynamicUser=true; a
second [Service]; an [Install]. Into the helper's service: TimeoutSec=1min
after TimeoutStartSec=3min; TimeoutStartSec=5min; Wants=; an ExecStartPost=
behind `\r`; CapabilityBoundingSet=; PassEnvironment=. Into the path unit:
PathChanged=; MakeDirectory=true. Each failure names the line, e.g.
`holzkube-manager-update-check.service:58: [Unit] OnSuccess=holzkube-manager-update.service is not a line this unit may carry`.

Control, the same injections against the previous guard (`git show
HEAD:internal/host/hostaction/units_test.go`, restored `cmp` 0 afterwards):
OnSuccess=, Wants=, the `\r`- and BOM-hidden ExecStartPost=, the comment
ending in `\`, TimeoutSec=5min, SendSIGKILL=no, FinalKillSignal=SIGCONT,
TimeoutStartFailureMode=abort, BindPaths=, PassEnvironment=,
StandardOutput=file:, helper TimeoutSec=1min and helper Wants= all **exit 0**;
so the injections inject and the new guard is what catches them.
PathChanged= and the assignment continuation were already red before
(TestUnitsAgree's trigger list; systemd-analyze's "no ExecStart=").

`systemd-analyze verify --man=no` on scratch copies (ExecStart= pointed at a
stub): the three shipped units exit 0 with **no output**. With a hidden
`ExecStartPost=/nonexistent/hidden-*` behind `\r`, behind NUL and behind a
BOM, each exit 1 with "Command /nonexistent/hidden-cr|nul|bom is not
executable": systemd 257 reads all three hidden lines. `Description=x \`
before ExecStart= gives "Unknown key 'Description' in section [Service]" and
"Service has no ExecStart=": systemd joins an assignment's continuation. A
continuation on a comment line it does not join (the control above stayed
green, TestUnitsVerify included); the guard refuses both anyway.

### V-26: human verification item (cannot be done here without installing)

Nobody has run the check unit under its real sandbox. That needs root and an
installed unit, so it is the operator's step. After installing the helper as
deploy/HOST-HELPER.md says, from a release that carries this unit (it has
`TimeoutStopSec=10s`; `systemctl cat holzkube-manager-update-check.service`
shows which one is installed):

```sh
sudo systemctl start holzkube-manager-update-check.service; echo "exit=$?"
systemctl show holzkube-manager-update-check.service -p Result -p ExecMainStatus
journalctl -u holzkube-manager-update-check.service -b --no-pager -n 50
sudo cat /var/lib/holzkube-manager-update/status.json
```

Expected: `exit=0`, `Result=success`, `ExecMainStatus=0`; the journal shows
the script comparing the installed version with the newest release and no
"Permission denied", "Operation not permitted", "Could not resolve host" or
"Address family not supported"; `status.json` has a `checked_at` from just
now and the daemon user can read it (`sudo -u holzkube-manager cat` on the
same path). A failure here points at a directive of the unit (most likely
RestrictAddressFamilies=, MemoryDenyWriteExecute= for python3, or
ProtectSystem=strict with the StateDirectory=).

### Final suites (Pi, main checkout)

- `go test ./internal/host/hostaction/ ./internal/httpapi/ -count=1 -v`:
  exit 0, no SKIP (TestUnitsVerify ran systemd-analyze; the root-namespace
  script tests ran under unshare).
- `go test ./internal/... ./cmd/... -count=1`: exit 0, 41 `ok`, no other lines.
- `./bin/task lint:go`: exit 0, 0 issues.
- `go test ./internal/publicrepo/` before each commit: exit 0.
- web/ not touched, so no web suite was run.

## Round 3 — verification findings W1, W2, I1–I5

Ran on the operator's Pi (aarch64), in the main checkout, no worktree, no
`-race`. Every exit code below was read from the command itself, not through
a pipe. Every injection was restored and checked with `cmp` against the
pre-injection copy or `git show HEAD:`, then the suite went green again.
`holzkube-manager.service` was the same before and after: ActiveEnterTimestamp
Tue 2026-09-29 20:25:38 CEST, NRestarts 0. Nothing was pushed or released.

### Per item

| Item | Outcome | Commit |
|------|---------|--------|
| W1 page lacks the boot clause (V-02) | fixed | 59b2523 |
| W2 systemdSpan overflow | fixed | 21c594c |
| I1 CheckHeld / checkHeld without an age bound | fixed | ebb226d |
| I2 page-constant ties by substring; ResultWithin unpinned | fixed | b308179 |
| I3 api.ts "four host actions" | fixed (comment) | cc39887 |
| I4 checkAnswer window and the answer taken back | fixed | dad669a |
| I5 Unicode white space in unit files | fixed | ddfc86a |

- **W1.** A started check is now `started` exactly while the host answer's
  `actions.busy` is true. Once it is false the check is over (`no-answer`).
  The server's busy already reads the record against HelperServiceLimit and
  the boot (CheckRunning), so the page keeps no second copy of that rule.
  This applies both to the order the page placed and to the one it rebuilds
  from the helper's record after a reload. The box gets a new red sentence
  when the machine booted after the record (the server's whole-second rule):
  "started, but the host restarted before the check ended. Nothing was
  installed; `journalctl -u holzkube-manager-update-check` says how far it
  got." It is recorded in 13-UI-SPEC.md (copy row and rule), and guide.md
  says the page offers the buttons again. The running-check test fixtures
  now carry `busy: true`, as the server sends it.
- **W2.** systemdSpan holds each part (`n > maxUnitSpan/unit`) and each
  partial sum (`total > maxUnitSpan - d`) under 24 h before it adds them, so
  nothing can wrap. strconv.ParseInt replaces Atoi.
- **I1.** CheckHeld returns false for a started record that is
  HelperServiceLimit or more older than the withdrawn order's placement. The
  page's checkHeld does the same with CHECK_WITHIN_MS, which I2 now ties
  exactly to HelperServiceLimit.
- **I2.** pageMillis requires exactly one `export const NAME = <product>` in
  HostActions.tsx. The product must be integer literals joined by ` * `, with
  nothing else on the line, and the name must be assigned nowhere else. It
  evaluates the product and compares durations.
  TestBusyFromTheCheckThisBoxPlacedEnds steps 59 s and 1 s as literals and
  pins ResultWithin to 1 min.
- **I4.** Both parts were real wrong displays and both are fixed:
  - checkAnswer reaches back to done − 3 min only for `from: 'result'`. An
    order this page placed, or one the daemon reports, has a true placement
    time.
  - HostOrderStatus keeps the answer it read, per followed order id, so the
    next hourly run no longer turns the emerald answer into "not in the
    update status".
  - The same-second case (`checked == end`) is left as it was: the script
    writes before the helper records, so a status in the done second is the
    check's.
- **I5.** refuseAmbiguousUnitBytes refuses any `unicode.IsSpace` rune other
  than the space and the tab.

### Red runs

**W2.** Injected into deploy/holzkube-manager-update-check.service, test
`-run '^(TestUnitsVerify|TestUnitsAgree|TestTheCheckUnitRunsOnlyTheCheck)$'`:

- `TimeoutStopSec=2562048h 9223371278s` against the old test: exit 0 (the
  gap). Against the new test: exit 1, "longer than 24h0m0s".
- `TimeoutStartSec=2562048h 9223371330s`: exit 1.
- `TimeoutStopSec=23h 2h` (the sum branch): exit 1.
- The helper service with `TimeoutStartSec=2562048h 9223371458s`: exit 1
  (TestUnitsAgree).
- Restored: exit 0.

**I5.** Byte-exact injections, with the bytes checked with `od`:

- U+00A0 after `TimeoutStopSec=10s`, U+2028 after `TimeoutStartSec=2min`, and
  U+0085 after `ExecStart=… --check`.
- New test, TestUnitsAgree plus TestTheCheckUnitRunsOnlyTheCheck alone: each
  exit 1, "white space U+… that systemd does not take for white space".
  TestUnitsVerify is also red now, at the same guard.
- HEAD's test, the same two tests: all three exit 0. The ExecStart one was
  also exit 0 with TestUnitsVerify included, the case nothing caught.

**I2.**

- `ResultWithin = 3 * time.Minute`: `go test ./internal/host/hostaction/`
  exit 1 ("ResultWithin = 3m0s, want 1m0s"). Against HEAD's test: exit 0.
- API test (`-run 'TestHostActionsWaitForARunningCheck$'`), each exit 1:
  - CHECK_WITHIN_MS `3 * 60 * 1000 * 2`: "is 6m0s";
  - `4 * 60 * 1000 // CHECK_WITHIN_MS = 3 * 60 * 1000`: "assigns it 2 times";
  - `4 * 60 * 1000 // was 3 * 60 * 1000`: "not a product of integer
    literals";
  - RESULT_WITHIN_MS `60 * 1000 * 3`: "is 3m0s".
- HEAD's API test against the `* 2`, the `// CHECK_WITHIN_MS` and the `* 3`
  injections: each exit 0.

**I1.**

- Go, bound removed: exit 1 (three TestCheckHeld rows, plus the withdrawal
  warning row "a check started 6 h before").
- Go, `>` for `>=`: exit 1 (the exactly-3-min row).
- Page, bound removed: 2 failed.
- Page, `>` for `>=`: 1 failed.
- Restored: green.

**W1.** vitest, `src/components/HostActions.test.tsx`:

- The new tests against the page at HEAD: exit 1, 4 failed. These are the
  order placed here, the one from the record after a reload, the box, and the
  orderPhase row.
- Busy clause removed: 5 failed.
- Boot rule `<` for `<=`: 1 failed (a boot in the record's next second).
- Cut-off sentence never chosen: 2 failed.
- Restored: 177 passed.

**I4.**

- The new tests against the page before the change: 3 failed.
- Widening for every order: 2 failed.
- Widening for the wrong orders: 4 failed.
- Nothing kept: 1 failed.
- Kept for any order id: first green (a gap in the new tests), so a test
  was added for it; then 1 failed.
- Restored: 181 passed.

**I3.** A comment only; there is no guard and no red run.

### Not in this round's list

- V-22 (Place's lock does not close the helper's rename between busyLocked's
  Lstat and the link, info) was not among the items, and was not changed.
- V-26 stays the operator's root step, as above.

### Final suites (Pi, main checkout)

- `go test ./internal/... ./cmd/... -count=1`: exit 0, 41 `ok`, and only "no
  test files" lines otherwise.
- `./bin/task lint:go`: exit 0, 0 issues.
- `go test ./internal/publicrepo/` before each commit: exit 0.
- web: `npx vitest run --project jsdom` exit 0, 56 files and 805 tests.
  `npm run test:browser` exit 0, 5 files and 23 tests. `npm run lint` exit 0
  (2 old warnings and 1 info in DataTable.tsx and wall.test.tsx, untouched).
  `npm run typecheck` exit 0.

## Remaining notes — IN-02, IN-03

Ran on the operator's Pi (aarch64), in the main checkout, no worktree, no
`-race`. Every exit code below was read from the command itself, not through a
pipe. Every injection was restored and checked with `cmp` against the
pre-injection copy, then the suite went green again. `holzkube-manager.service`
was the same before and after: ActiveEnterTimestamp Tue 2026-09-29 20:25:38
CEST, NRestarts 0. Its unit, the hourly update unit and timer, and
`/var/lib/holzkube-manager` were not touched; nothing was installed, pushed or
released, and there is no changelog entry.

| Item | Outcome | Commits |
|------|---------|---------|
| IN-02 check and hourly update race on the status file | fixed | 30d1eb5, 3c32ee4, c47f2ac (lint) |
| IN-03 check unit's sandbox could be tighter | fixed; the run under the system manager stays V-26 | 5bd85d0 |

### IN-02

**How the script is installed and replaced.** The script is
`/usr/local/sbin/holzkube-manager-update`. The hourly
`holzkube-manager-update.service` and the check unit both exec it, and after a
healthy update it replaces itself from the release archive. The hourly unit is
the operator's own (`/etc/systemd/system`), not shipped: oneshot, no timeout,
`PrivateTmp=true`. So both modes run the same file, except for the moment a
running old script replaces itself.

**Choice: flock(1) in the script, on `$STATUS_DIR/.lock`.** systemd ordering
was not used, for these reasons:

- `After=` on the check unit only orders jobs queued together. It does not
  make an hourly run wait for a check already running, and that half would
  need the operator's unshipped hourly unit changed.
- `Conflicts=` would stop an update in the middle of an install.
- A unit-level wait does not count against `TimeoutStartSec`. It would break
  the check unit's worst-case sum against the helper's 3 min.
- Neither covers a manual `sudo holzkube-manager-update`.

The lock covers both directions and manual runs.

- **Where the lock is taken.** The run takes it before it reads the installed
  version, and before the EXIT trap. A run that gives up therefore records
  nothing; a record written beside the holder would be the race again.
- **Wait bounds.** `--check` waits 60 s, so wait plus curl's 30 s stays under
  the check unit's `TimeoutStartSec=2min`. Every other run waits 600 s. The
  check unit can never run longer than 2min40s.
- **Override.** `HOLZKUBE_MANAGER_UPDATE_LOCK_WAIT` replaces either wait. Only
  the tests use it.
- **Who cannot hold the lock.** The file is 0600 root, created under umask 077.
  The daemon's user reads `status.json` but cannot open the lock, so it cannot
  hold off an update. A non-root `--check` may not record, so it does not lock.

**Keeping the hourly update going.** The hourly update must not stop on the
lock itself. If flock is missing, the status directory is unusable, the file
cannot be opened, or flock fails with any exit code other than its conflict
code (`-E 75`), the run goes ahead unlocked, as before. It also logs a WARNUNG.
Only a lock held for real makes a run wait. After 600 s the hourly run exits 1
with a message naming the lock, and the timer fires again the next hour.

**Stale locks.** The lock is the kernel's and ends with the last process that
holds the descriptor. A dead run's `.lock` file blocks nothing. Children inherit
the descriptor; the longest-lived one is curl, bounded by `--max-time 300`.
None of them daemonizes.

**Compatibility.**

- An old installed script with the new units behaves exactly as before.
- A new script with old units works. Both write the status directory: the
  hourly unit has no sandbox, and every check unit has had
  `StateDirectory=`.
- Only the self-replacement moment is unlocked: an old running script beside a
  new one.

**Reversibility.** High. A later release can drop the lock, and the leftover
empty `.lock` is harmless; nothing reads it.

**Not changed.**

- `--rollback` stays unlocked. It is the path someone takes under time
  pressure.
- The page comment in HostActions.tsx ("would run the update script twice")
  still holds for older scripts, and the page's main reason (the helper is
  busy) is unchanged.

**Red runs** (`go test ./internal/host/updatestatus/ -run TestUpdateScriptRunsOneAtATime`):

- **HEAD's script, before the fix.** Exit 1, all five subtests red:
  - "an update waits for a running check": the update ended while the check
    looked; outcome "available", want "updated". This is the review's race,
    reproduced.
  - "a check waits for a running update": the check ended beside the install;
    outcome "updated", want "current".
  - Both give-up subtests: exit 0, want 1, and status.json overwritten.
  - The killed-run subtest: no lock file.
- **`umask 077` disabled.** Exit 1: lock file mode -rw-rw-r--.
- **Give-up that goes on instead of failing.** Exit 1. Done on both the first
  form and the `-E 75` form; both give-up subtests went red with status.json
  rewritten.
- **A `setsid /bin/sleep 301 &` child holding the descriptor.** Exit 1: the
  next run waited 5 s and failed ("a dead run's lock held this one"). The
  orphans were killed afterwards (`pgrep` count 0).
- **"An update whose flock fails goes ahead unlocked"** (stub flock exit 71),
  against 30d1eb5's script: exit 1, want 0. The real-world form of that fault
  was seen under a seccomp filter denying flock: the script waited 60 s and
  blamed "another run".
- Restored: the package is green. 67 PASS in the first green run, plus the
  flock-error subtest after.

### IN-03

**Added to the check unit:** `SystemCallFilter=@system-service`,
`SystemCallErrorNumber=EPERM`, `ProtectProc=invisible`, `ProcSubset=pid`,
`PrivateIPC=true`. TestTheCheckUnitRunsOnlyTheCheck lists exactly these five,
with their values.

**Already there:** ProtectKernel*, ProtectClock, ProtectHostname,
LockPersonality, RestrictNamespaces, RestrictRealtime, RestrictSUIDSGID,
SystemCallArchitectures, UMask.

**Not added:**

- `RemoveIPC=`. systemd.exec(5) on this host: "It has no effect on IPC objects
  owned by the root user."
- `IPAddressDeny=` for RFC 1918 or link-local ranges. The reference host's
  `/etc/resolv.conf` names an IPv4 resolver in an RFC 1918 network, and IPv6
  resolvers are often link-local, so DNS would break.

**What `--check` needs,** measured against the real GitHub API with a daemon
binary built from this tree as `HOLZKUBE_MANAGER_BIN` and scratch paths for
everything else:

- **System calls.** `strace -f` of a root run (user namespace, status
  directory created by the run) plus the lock-wait path: 78 distinct system
  calls, `comm` against the expanded `@system-service` set: none outside it.
  Executed: bash, readlink, uname, install, python3 (×5), flock, curl, awk,
  mktemp, chmod, mv, and holzkube-managerd `--version` (×2).
- **/proc.** It reads `/proc/self/{auxv,cgroup,mountinfo,maps}` (Go),
  `/proc/sys/vm/overcommit_memory` (glibc) and `/proc/filesystems`
  (coreutils/libselinux).
- **The filter under systemd.** A user-manager transient service running the
  script with `SystemCallFilter=@system-service` and the *kill* action (plus
  NoNewPrivileges, MemoryDenyWriteExecute, RestrictAddressFamilies,
  PrivateIPC): exit 0, and "available" was recorded. Controls:
  - `~flock` added: "Bad system call".
  - `~connect` added: "die Release-Liste ... ist nicht lesbar", exit 1.

  So the filter was live.
- **ProtectProc and ProcSubset.** The user manager does not apply them
  (mountinfo showed plain proc, /proc/meminfo readable). They were measured
  instead with `unshare -Urpfm` and `mount -t proc -o
  subset=pid,hidepid=invisible`: /proc/meminfo and /proc/sys were absent, and
  `--check` as root exited 0 with the status recorded.
- **Not measured together.** All five keys at once, under the system manager
  as root with the token file and `update.conf`, are the human item. V-26
  instructions are extended in 13-VERIFICATION.md and 13-UAT.md #17: no
  "Bad system call", "Operation not permitted" or "ohne Sperre" in the
  journal, `.lock` -rw------- root, and the security exposure near 1.9.

**systemd-analyze** (systemd 257) on a scratch copy of the committed unit
(ExecStart pointed at a stub):

- `verify`: exit 0 with 0 bytes of output.
- `security --offline=true`: exposure 3.4 before the change, 1.9 after.

**Red runs** (`-run '^(TestUnitsVerify|TestUnitsAgree|TestTheCheckUnitRunsOnlyTheCheck)$'`):

- New keys against the old allow-list: exit 1, five "is not a line this unit
  may carry".
- `ProcSubset=` line removed: exit 1, "has no ProcSubset=".
- `ProtectProc=default`: exit 1.
- `ProcSubset=pids`: exit 1. The allow-list failed, and so did TestUnitsVerify;
  `systemd-analyze verify` on a scratch copy said "Failed to parse
  ProcSubset=pids, ignoring" with exit 0.
- `SystemCallFilter=@system-servce`: exit 1, both tests.
- Restored (`cmp`): green.

### Final suites (Pi, main checkout)

- **`go test ./internal/... ./cmd/... -count=1`.** Exit 0, 41 `ok`, nothing
  else, on the third run. The first two runs were exit 1, each with one
  timeout in `internal/upgrade` TestANodeSaysHowItBooted ("talos: Get ... timed
  out", once at 42 s and once at 30 s). The package does not depend on
  anything changed here (`go list -deps -test` shows no
  updatestatus/hostaction, and it reads nothing in deploy/). Alone it passed
  in 2.8 s, and the whole package in 25.8 s. It is a timing-sensitive live
  test under full load (load average around 5 from other processes), not a
  regression. It is worth watching.
- `./bin/task lint:go`: exit 0, 0 issues (after c47f2ac).
- `go test ./internal/publicrepo/` before each commit: exit 0.
- web/ was not touched, so no web suite was run.

## Remaining notes — IN-04

**Where it ran:** the operator's Raspberry Pi 5 (aarch64), main checkout, no
worktree. Go from `~/.local/go` (no `-race`), Node through nvm. Nothing
installed. `/usr/local/sbin/holzkube-manager-host` and
`/etc/systemd/system/holzkube-manager-update-check.service` are absent before
and after. `holzkube-manager.service` read `ActiveEnterTimestamp=Tue
2026-09-29 20:25:38 CEST, NRestarts=0` before and after. The real
`/usr/local/sbin/holzkube-manager-update` here is root 0755, so it counts as
installed. Nothing pushed, no release, no changelog entry.

### IN-04: the update script is now detected — fixed (4ebc14f)

**What.** Both update orders end in `/usr/local/sbin/holzkube-manager-update`:
`update` through `holzkube-manager-update.service`, `check-update` through the
check unit's `--check`. The helper's install commands do not install it. The
daemon now asks for it from the file alone, as it asks for the helper's
script. It must be a regular executable owned by uid 0 and writable by
nobody else (`hostaction.UpdateScriptMissing`, the shared `rootExecutable`).
That is a stat under `ProtectSystem=strict`: no D-Bus, no process, and the
file is never read. It is asked whatever `Missing` says, because the helper's
commands never bring it.

**Refused:** only `update` and `check-update` (`hostaction.NeedsUpdateScript`),
with `409 conflict.host-update-script-missing` on both routes. The confirm
route refuses once the body names one of them, before the hostname is
compared. The action route refuses before the body and the token. No token,
nothing placed. `reboot`, `poweroff` and `restart-service` go through.

**Order:** container → missing → update script → outdated → busy.

- The update script comes after container and missing because those decide
  whether anything can be placed at all. In a container, installing is the
  wrong advice.
- It comes before the older helper because it is the reason both update
  actions share. With it first, the page has one reason line for the pair, and
  the routes refuse both with the code that line names. With outdated first,
  the check and the update would carry different reasons. The older-helper
  note stays on the page meanwhile, so nothing is hidden.
- It comes before busy for the same reason outdated does: it is a standing
  condition that waiting will not cure.

**Wire.** `actions.update_script` (one item
`{"item":"update-script","path":"/usr/local/sbin/holzkube-manager-update"}`,
or `[]`, never null) and `actions.update_script_install_commands` (the one
line from `hostaction.UpdateScriptInstallCommands`). `available` is
unchanged.

**Page.**

- `actionReason` turns off only the two update buttons, with `REASON.updateScript`.
- The one reason line describes those two buttons through `aria-describedby`.
  The group and the other three describe nothing.
- `HostUpdateScriptNotice`, in the shared slate frame with no new classes,
  names the path, says in one sentence which buttons need it, and shows the
  server's command. It appears beside the helper notice or the older-helper
  note, never in a container.
- Install command: `sudo install -o root -g root -m 0755
  deploy/holzkube-manager-update.sh /usr/local/sbin/holzkube-manager-update`.
  The archive carries the file (`.goreleaser.yaml`, now held by
  `TestTheArchiveCarriesTheHelper`), and the script reinstalls itself with
  exactly that owner and mode.

**Docs:**

- `deploy/HOST-HELPER.md` has a new section, "The update script", whose
  command block is held byte for byte by
  `TestUpdateScriptInstallCommandsMatchTheGuide`.
- `docs/api-contract.md` gets the new row, `update_script`,
  `update_script_install_commands`, an example and the order paragraph.
- `docs/guide.md` gets "Without the update script."
- README unchanged: it describes the helper notice and the host picture, and
  no rendered screen changes (below).

**Fixtures.** `demo.json` and `host-helper-installed.json` carry both keys as
written, held by vitest (`fixtures.test.ts`) and Go
(`TestTheFixtureShowsTheRealInstallCommands`). The demo stays helper-missing,
with `update_script: []`: the reference installation has the script.

**Red runs.** Each was injected, seen fail through the command's own exit code
(not through a pipe), restored from a scratch copy with `cmp`, then green.
After the commit, `git show HEAD:<file> | cmp - <file>` holds for all five
injected files.

- Action route refusal removed (`if false && …` in `hostAction`):
  `TestHostUpdateActionsNeedTheUpdateScript` exit 1, 7 FAIL lines,
  "POST /api/v1/host/actions/update with a valid host token: 202, want 409".
- Confirm route refusal removed: exit 1, 7 FAIL lines. The confirm route
  answered "200 … handed out a token", and with busy, "code
  conflict.host-helper-busy, want conflict.host-update-script-missing".
- Update-script and outdated refusals swapped in the action route: exit 1,
  subtest `outdated` FAIL, "code conflict.host-helper-outdated, want
  conflict.host-update-script-missing".
- Ownership check dropped from the detection (mode only): all exit 1.
  - `TestUpdateScriptMissing`: owned_by_uid_1000 and no_owner_information FAIL.
  - `TestReadCarriesActions`: an_update_script_uid_1000_owns FAIL.
  - `TestHostUpdateActionsNeedTheUpdateScript`: owned_by_uid_1000 FAIL.
- Collector asks only once the helper is installed: `TestReadCarriesActions`
  exit 1, nothing_installed FAIL.
- Page reason clause removed (`actionReason`): `HostActions.test.tsx` exit 1,
  2 failed.
- Page reasons swapped (outdated before the script): exit 1, 1 failed (the
  "older helper too" case).
- Page notice removed (`host.tsx`): `host.test.tsx` exit 1, 3 failed.
- The page's `UPDATE_SCRIPT_ACTIONS` cut to `['check-update']`: exit 1, "the
  page and the routes disagree". On the first try that cross-check also failed
  on the correct list, because it compared the two lists in their declaration
  order. It now compares them sorted and was seen red again against the cut
  list.

**Not covered.** `update` also needs `holzkube-manager-update.service`, the
unit the hourly timer starts. This repository does not ship that unit, so
there is no install command to show, and the page does not look for it.
HOST-HELPER.md says so. If the operator's reference installation ever lacks
that unit, the update order still starts a unit that does not exist, and the
helper records `failed`.

### Final suites (Pi, main checkout)

- `go test ./internal/... ./cmd/... -count=1`: exit 0, 41 `ok`, nothing else.
  After that, a doc-comment-only change for revive, then
  `go test ./internal/host/... -count=1`: exit 0.
- Root namespace (`unshare --user --map-root-user`, `-v`):
  `TestHostActionRoundTrip`, `TestHostCheckRoundTrip` and `TestHostScriptAsRoot`
  PASS, 0 SKIP. `installedHelperFS` changed, so the end-to-end run was redone.
- `./bin/task lint:go`: exit 0, 0 issues. The first run found one revive
  comment form, which was fixed.
- web:
  - `npx vitest run --project jsdom`: exit 0, 56 files, 815 tests.
  - `npm run test:browser`: exit 0, 5 files, 23 tests.
  - `npm run lint`: exit 0. The 2 warnings and 1 info are in `DataTable.tsx`
    and `wall.test.tsx`, which this change does not touch.
  - `npm run typecheck`: exit 0.
- `go test ./internal/publicrepo/` before the commit: exit 0.
- `systemd-analyze verify`: no unit file changed, so nothing to verify.
- `./bin/task test:layout`: not run, because no class changed. The notice reuses
  `HelperInstallNotice`'s frame.
- Pictures: `task build && node web/scripts/readme-images.mjs` rewrote every
  PNG. A pixel diff of `host.png` (bbox 39,824–1239,1724) is only the
  render-time "since" dates and the dev-build version label
  (`v0.0.1-284-gd3de078-dirty` vs `v0.1.0`). The same noise is in screens this
  change does not touch. No picture shows the change, so all were restored and
  none committed.
