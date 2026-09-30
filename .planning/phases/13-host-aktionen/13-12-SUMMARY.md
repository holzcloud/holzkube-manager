---
phase: 13-host-aktionen
plan: 12
subsystem: host-actions
tags: [host-actions, update-check, root-helper, systemd, tdd, tracer]
status: complete

requires:
  - phase: 13-host-aktionen
    plan: 09
    provides: the helper round trip as root in a user namespace (runHelper, installedHelperFS)
  - phase: 13-host-aktionen
    plan: 11
    provides: the host action buttons' cn() class list and the phone/desktop browser layout tests
provides:
  - hostaction.CheckUpdate ("check-update"), fifth in Actions(), with its route, typed hostname, operator role, sudo window and audit host.check-update
  - the root helper's fifth order, `check-update <id>` -> `systemctl start holzkube-manager-update-check.service` (blocking)
  - deploy/holzkube-manager-update-check.service, a hardened oneshot running `holzkube-manager-update --check`
  - the "Check for updates" button, dialog, status sentences, 3-min window and the busy reason on /host
  - orderPhase comparing checked_at with the placement truncated to the second (for update and check-update)
  - TestHostCheckRoundTrip and runHelperWithStub (a stand-in systemctl that can play a unit)
affects: [13-13, 13-14, 13-15]

actuals:
  tokens: 16500
  tasks: 2
  commits: 2
plan_head_before: 7e1f672dd718f85647d9dcf0426db990649c0393
plan_head_after: f37fff3033d40464f8d9cc2566edbcd71f72ebe9

tech-stack:
  added: []
  patterns:
    - "A stand-in systemctl that plays the unit it is asked to start: it logs its argv, and for the one expected argv it writes what the unit would write (tmp name, rename), so the round trip reaches the page through the real reader"
    - "A per-action it.each built with flatMap over (how, dialog), so a lock held per dialog is proven for every dialog it has to hold"

key-files:
  created:
    - deploy/holzkube-manager-update-check.service
  modified:
    - deploy/holzkube-manager-host.sh
    - internal/host/hostaction/hostaction.go
    - internal/host/hostaction/result.go
    - internal/host/hostaction/result_test.go
    - internal/httpapi/handlers/host.go
    - internal/httpapi/handlers/confirm_test.go
    - internal/httpapi/router.go
    - internal/audit/redact.go
    - cmd/holzkube-managerd/budget_test.go
    - internal/httpapi/hostactionsapi_test.go
    - internal/httpapi/hostgatesapi_test.go
    - web/src/api.ts
    - web/src/components/HostActions.tsx
    - web/src/components/HostActions.test.tsx
    - web/src/routes/host.browser.test.tsx

key-decisions:
  - "A finished check is emerald for current, available and (an hourly update in between) updated; only failed is red. A newer release found is what the check was asked for, not a fault"
  - "A check with a failed poll and no result yet falls through to the ordinary picked-up / no-answer logic instead of 'waiting'; the stale reading's observed_at does not advance, so the page does not jump to no-answer during an outage"
  - "The helper's 'back' phase sentence has a check-update case (the last-known started sentence) only for exhaustiveness; orderPhase never returns back for a check"

requirements-completed: []

metrics:
  duration: 33min
  completed: 2026-09-30
---

# Phase 13 Plan 12: Check for updates -- the fifth host action, end to end Summary

**"Check for updates" on /host: behind the same four locks as the other four actions, it places `check-update <id>`. The root helper runs only `holzkube-manager-update-check.service` for it, and that unit runs the update script's `--check`. The answer (installed version and newest release) comes back in the status box and under Update check. Nothing is installed.**

## Where it ran

The operator's Raspberry Pi 5 (aarch64), Go from `~/.local/go`, Node through nvm, the browser project's Chromium. No `-race`: ThreadSanitizer refuses this kernel, so the race verdict belongs to CI. The helper ran as root only inside `unshare --user --map-root-user` against a stand-in systemctl.

- `holzkube-manager.service` at the start of Task 1: `ActiveEnterTimestamp=Tue 2026-09-29 20:25:38 CEST`, `NRestarts=0`
- after Task 2: `ActiveEnterTimestamp=Tue 2026-09-29 20:25:38 CEST`, `NRestarts=0`. Identical.
- Nothing installed: `/usr/local/sbin/holzkube-manager-host`, `/etc/systemd/system/holzkube-manager-host.path` and `/etc/systemd/system/holzkube-manager-update-check.service` are all absent (exit 0). Nothing was pushed, tagged or released.

## D-08 before-dump (for 13-15)

`LAYOUT_DUMP=$HOME/.cache/holzkube-manager-layout/13-12-before.jsonl ./bin/task test:layout` exited **0**, before any file of this plan changed. The dump has **964 lines**, **36** of them with route `/host`. It sits outside the repository and was not committed.

## Red runs (exit codes read from the command itself)

| Step | What | Exit | Failing line |
|---|---|---|---|
| T1.1 | `TestHostCheckRoundTrip` on the tree before this plan (the action name written as a literal, since `hostaction.CheckUpdate` did not compile) | 1 | `hostactionsapi_test.go:463: confirm host.check-update: 400, want 200 (... "four host actions and that is not one of them")` |
| T1.2 | web rows before any page change | 1 | the suite failed at collection: the zod schema refused `check-update` (ZodError) |
| T1.2 | web rows with only the API schema widened | 1 | 25 failed, among them `update, the status in the same second as the placement: finished` against the old later-than `orderPhase` |
| T1.6 | `max-md:col-span-2` removed, `host.browser.test.tsx` | 1 | `lays the check alone on the first row...`: `expected 191 to be 390`; restored, cmp equal |
| T2.1a | Destructive false for check-update only | 1 | `hostgatesapi_test.go:189: check-update with the sudo window shut: 202, want 428 sudo.required` |
| T2.1b | hostTypedPhrase[host.check-update] false | 1 (and handlers 1) | `hostgatesapi_test.go:409: host.check-update typing "example-hostx": 200, want 400`; `confirm_test.go:87: hostTypedPhrase[host.check-update] = false` |
| T2.1c | MinRole reader for check-update only | 1 | `hostgatesapi_test.go:210: reader on check-update: 202, want 403 forbidden.role` |
| T2.1d | Action empty for check-update only | 1 | `hostgatesapi_test.go:336: the archive holds no host.check-update attempt and success`. The router did **not** panic: the gate test's audit subtest caught it |
| T2.2 | Task 2 web rows against Task 1's tree | 1 | 3 failed: `while a check runs: all five off`, `check picked up, then no answer: still picked up, never waiting`, `check started, then no answer: still started, never waiting` |
| T2.4 | waiting exclusion removed | 1 | the two failed-poll check rows |
| T2.4 | truncation put back to `> placed` | 1 | `update, ... same second ...: finished` and `check, ... same second ...: finished` |
| T2.4 (extra) | CR-01 lock removed (`!open` without `!run.isPending`) | 1 | Escape and close X for both the Restart host and the Check for updates dialog |

After each lock injection `handlers/host.go` was restored from a saved copy and checked with `cmp`. `git diff --quiet -- internal/httpapi/handlers/host.go` exited 0 before Task 2's commit. All four locks went red without extending `hostgatesapi_test.go`, because its subtests already loop over `hostaction.Actions()`.

The CR-01 close lock was already per dialog. The extended test passed without a fix, and removing the lock turned it red for the check's dialog as well.

## Green

- `go test ./internal/httpapi -run 'TestHostCheckRoundTrip|TestHostActionRoundTrip|TestHostActionGates|TestHostConfirmGates|TestHostTokenOpensOneOrderInItsOwnSession|TestHostActionsNeedTheHelper|TestHostActionsInAContainer|TestEveryProblemCode' -v`: exit 0, `--- PASS: TestHostCheckRoundTrip`, 0 SKIP lines
- `TestTheDaemonStartsNoProcess` (D-11): PASS. No process start was added.
- `go test ./internal/... ./cmd/... -count=1`: exit 0. `./bin/task lint:go`: 0 issues.
- `npx vitest run --project jsdom`: 56 files, 742 tests, exit 0. `test:browser` on `host.browser.test.tsx`: 9 passed, exit 0.
- web lint (2 pre-existing warnings in `wall.test.tsx`) and typecheck: exit 0. `bash -n deploy/holzkube-manager-host.sh`: exit 0.
- `git diff --quiet 8ba3647 -- deploy/holzkube-manager-update.sh`: exit 0, so the update script is unchanged (D-19).
- Tracer gate: every Task 1 `<verify>` command passed on exactly the tree that was then committed as `aa4d4e9`, before Task 2 began.

## Task Commits

1. **Task 1: Check for updates end to end**: `aa4d4e9` (feat)
2. **Task 2: the four locks red for the check; the check never waits**: `f37fff3` (feat)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] "four host actions" wording in places outside the plan's file list**
- **Found during:** Task 1
- **Issue:** `result.go`'s refusal ("not one of the four host actions"), the host confirm route's validation detail, `router.go`'s HostActions comment and `confirm_test.go`'s message ("Every host action takes away the machine") became untrue with a fifth action.
- **Fix:** each now says five, or gives the HACT-05 reason; `result_test.go`'s expected error follows.
- **Files modified:** internal/host/hostaction/result.go, internal/host/hostaction/result_test.go, internal/httpapi/router.go, internal/httpapi/handlers/confirm_test.go
- **Commits:** aa4d4e9, f37fff3

**2. [Rule 3 - Blocking] The web RED needed the API schema first**
- **Found during:** Task 1 step 2
- **Issue:** the rows are built with `hostSchema.parse` when the suite is collected, so without `check-update` in the schema the whole file failed before any row ran. That would not have shown the same-second row red.
- **Fix:** `web/src/api.ts` was widened first, then the RED was measured row by row. Both runs are recorded above.

**3. [Rule 3 - Blocking] `runHelper` could not play a unit**
- **Fix:** `runHelperWithStub` adds sh code to the stand-in systemctl, and `runHelper` wraps it with none, so the existing round trip is unchanged.

### Left for later plans

- `deploy/holzkube-manager-host.service`'s comment still says the script "kennt nur vier feste Befehle". That unit is part of what the operator installs, and 13-13 owns the unit files and their gate.
- `HostHelperNotice`'s "knows exactly four orders ... the four buttons" belongs to 13-15, as the plan says.
- `.goreleaser.yaml` does not ship the new unit yet, and the systemd-analyze gate does not cover it yet. Both belong to 13-13.

## Known Stubs

None.

## Threat Flags

None beyond the plan's register (T-13-61 to T-13-66): no new endpoint besides the planned route, and no new file access by the daemon.

## Next Phase Readiness

The tree is green for 13-13. HACT-04 is **not** marked complete: 13-15 finishes it with the README.

## Self-Check: PASSED
