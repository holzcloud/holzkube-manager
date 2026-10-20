---
phase: 17-versionspruefung-zwischen-plan-und-apply
plan: 01
subsystem: api
tags: [talos, machineconfig, digest, optimistic-concurrency, go]
status: complete

requires:
  - phase: 15-drift-erkennung-der-maschinenkonfiguration
    provides: Snapshot, CompareDocuments, the apply observer on the Talos client
provides:
  - base_digest in the plan answer and as a required field of the apply body
  - 409 conflict.config-changed-since-plan with a sentence naming the changed paths
  - one apply per node at a time (lock with the request's deadline)
  - bounded in-process memory of recent plans for the sentence
affects: [17-02 (UI, guide, README, declarative tables), phase 17 verification]

actuals:
  tokens: 16173
  tasks: 3
  commits: 3
plan_head_before: b3c55ef5b452e38366704b19994ca1380967c1f0
plan_head_after: 7efc8acedad45da02f5845e7fbcfe304926f822f
commits: 3

tech-stack:
  added: []
  patterns:
    - "check and patch on one reading through one client, check strictly before the single send"
    - "a lock per node as a one-slot channel so the wait ends with the request's context"

key-files:
  created:
    - internal/machineconfig/sinceplan.go
    - internal/machineconfig/sinceplan_test.go
    - internal/machineconfig/changedsinceplan_test.go
    - internal/configdrift/refused_apply_test.go
  modified:
    - internal/machineconfig/service.go
    - internal/httpapi/handlers/config.go
    - internal/httpapi/problem.go
    - internal/httpapi/configapi_test.go
    - internal/configdrift/helpers_test.go
    - internal/configdrift/apply_test.go
    - internal/configdrift/callers_test.go
    - internal/configdrift/expected_test.go
    - docs/api-contract.md

key-decisions:
  - "Snapshot-failure errors (unreadable, too large, unsafe) stay with writeInventoryError (500, no detail): open question 2 of the research, decided here; the apply sends nothing in that case"
  - "ErrApplyBusy is a typed error with its own operator sentence (no package prefix) and unwraps to the context's error; the handler maps it to 409 store.conflict"
  - "The sentence uses Path, Kind, LenBefore, LenAfter only; never Before/After"
  - "Memo limits: 4 per node, 8 MiB total, one hour (planner's choice, assumption A5); they bear on wording only"

requirements-addressed: [VERS-01, VERS-02, VERS-03, VERS-04, VERS-05]
requirements-completed: []  # not ticked here by the plan's own instruction; the phase verification after plan 02 does it

duration: one session
completed: 2026-10-20
---

# Phase 17 Plan 01: Version check between plan and apply, server side

An apply is now refused with 409 `conflict.config-changed-since-plan` when the node's active configuration is not the one its plan read; nothing is sent, the sentence names the changed paths over all documents without values, and two applies on one node cannot overwrite each other.

BASE: b3c55ef5b452e38366704b19994ca1380967c1f0

## Where it ran

Mac (darwin/arm64), go1.27.3 locally (the linter ran under `GOTOOLCHAIN=go1.26.9`), simulated Talos node (`talossim`) with the real Talos client. No real cluster, no `go test -race` (CI only), no Pi (linux/arm64). Nothing installed, pushed, tagged or published.

## Commits

| Task | Commit | What |
|------|--------|------|
| 1 (tracer) | 473da17 | `Preview.BaseDigest`, `Apply(..., baseDigest)`, one client / one reading / `checkBase` before the send, route field and 400/409 mapping, `CodeConfigChangedSincePlan` plus contract row, `planThenApply` for the four configdrift callers |
| 2 | 47d5e4e | `planMemo`, sentence from `CompareDocuments`, eight-path cap, branches for unknown state and unnameable difference, node proofs |
| 3 | 7efc8ac | `lockMachine` (one-slot channel per node), `ErrApplyBusy` to 409 `store.conflict`, T4 and T5 proofs |

## ROT runs (exit codes of `go test` itself)

- Task 1: the four route tests plus the `planThenApply` helper against the pre-change code: `go vet` exit 1 and `go test ./internal/httpapi/` exit 1, `go test ./internal/configdrift/` exit 1. All compile errors (`httpapi.CodeConfigChangedSincePlan` undefined, `Preview.BaseDigest` undefined, `Apply` takes four arguments). A compile error is the red, as the plan allows.
- Task 2: unit tests: build failed (`newPlanMemo` undefined), exit 1. Node tests run against the Task 1 state with the unit file set aside: exit 1, 7 of 8 already green as expected (the core of Task 1 carries them), `TestTheRefusalSentenceNamesPathsAndNothingElse` red (Known false, no paths).
- Task 3: `TestLockMachine*`: build failed, exit 1. With the unit file set aside, `TestTwoAppliesOnOneNodeDoNotOverwriteEachOther` and `TestAnApplyWaitingForTheNodeGivesUpWithItsContext` exit 1 (second apply opened a client while the first was held; waiting apply returned nil). `TestARefusedApplyLeavesNoExpectationInTheBaseline` was green against the Task 2 state, as the plan predicted (Task 1 put the check before the observer); it became a measurement through injections (c) and (d) below.

## Injections (each: file backed up outside the repo, one change, exit code of the `go test` command itself, restore verified byte-identical, green run after)

Task 1 (route level, `TestAnApply*`):
- F1 check removed: exit 1, `configapi_test.go:507: apply after a foreign change: 200`.
- F3 comparison ignored (`snap.Digest == snap.Digest`): exit 1, same line 507.
- F4 check after `ApplyConfigurationWithMode`: exit 1, lines 522 (`the node heard 1 ApplyConfiguration call(s) after the refusal`) and 530 (`the refused patch reached the node`).
- F2 check on the patched bytes: `TestAnApplyOnAnUnchangedNodeStillLandsOverTheRoute` exit 1, line 483 (`apply: 409`).

Task 2 (`internal/machineconfig`):
- F5 empty digest means do not check: exit 1, `changedsinceplan_test.go:229`.
- F8 sentence from `Compare` instead of `CompareDocuments`: exit 1, line 294 (`sentence lacks "KubeletConfig.image (modified)"`).
- F9 sentence appends `Change.After`: exit 1, `changedsinceplan_test.go:304` (address and image in the sentence) and `sinceplan_test.go:116`.
- F10 plan keeps the raw bytes and the apply patches them: exit 1, lines 361/364 (`reverted the rotated token to the one the plan saw`). This is the proof for the digest choice (option A of the research).
- F7 `abort()` removed: exit 1, line 394 (`ErrStagedAlreadyPending` on the second staged apply).
- F12 snapshot failure swallowed in `checkBase`: exit 1, `sinceplan_test.go:185`.
- F15 missing earlier state means no refusal: exit 1, line 322.
- F14 second reading after the check (`s.read`): exit 1, line 415 (`an apply opened 2 clients, want 1`).

Task 3:
- F6 slots never fill (no lock): exit 1; red in `TestLockMachineWaitsOnTheSameNodeAndNotOnAnother`, `TestTwoAppliesOnOneNodeDoNotOverwriteEachOther` (lines 472, 480) and `TestAnApplyWaitingForTheNodeGivesUpWithItsContext` (lines 520, 526).
- F13 lock ignores the context: exit 1; `TestLockMachineGivesUpWithTheContext` and `TestAnApplyWaitingForTheNodeGivesUpWithItsContext` fail on their own 5 s limit (lines 274, 523), not by hanging.
- F4 against the baseline (check after the send): `TestARefusedApplyLeavesNoExpectationInTheBaseline` exit 1, `refused_apply_test.go:56: a refused apply left the expectation {Digest:e462a0c7... Mode:no-reboot ...}`.
- F1 against the baseline (check removed): exit 1, `refused_apply_test.go:48: Apply after a foreign change returned <nil>`.

Every injection was seen red. No injection stayed green, so no gap had to be closed. The 200 ms in the lock test supports the red view only; `go test ./internal/machineconfig/ -run 'TestTwoAppliesOnOneNode|TestAnApplyWaitingForTheNode|TestLockMachine' -count=20` ran green, exit 0 (25 s).

## Gates

| Gate | Result |
|------|--------|
| `go build ./...`, `go vet ./internal/machineconfig/ ./internal/httpapi/... ./internal/configdrift/`, `gofmt -l internal` | exit 0, empty |
| `go test ./internal/machineconfig/ ./internal/configdrift/ ./internal/audit/ ./cmd/holzkube-managerd/ ./internal/httpapi/handlers/ ./internal/httpapi/middleware/` | ok |
| `go test ./internal/httpapi/ -run 'TestThePlanRoute...|TestAnApply...'` (the four new route tests, 7 digest subtests) | PASS, no SKIP |
| `TestEveryProblemCode*`, `TestOnlyKnownCallersSendAConfigurationToAManagedNode` | PASS (one sender stays in `service.go`) |
| golangci-lint v2.13.1 on machineconfig, httpapi, configdrift | 0 issues (one `context-as-argument` finding in my test helper was fixed first) |
| `go test ./internal/httpapi/` as a whole | FAIL, one test, pre-existing and unrelated (below) |
| `go test ./internal/publicrepo/` | FAIL, history test, pre-existing and unrelated (below) |
| Files the plan keeps untouched (changelog, config, compat, talos, configdrift apply.go/decide.go, diff.go, applymode.go, snapshot.go, redact.go, web/fixtures, docs/screenshots) against BASE | empty diff |

## Deviations from Plan

None in behaviour. Notes:
- The plan lists `internal/httpapi/configapi_test.go` for `unlockEverything`/`openSudo`; the sudo window has to be opened before `unlockEverything` (the lock route is destructive), so the new tests call `openSudo` first. A test-order detail, not a design change.
- The digest table test has seven rows (the plan names five); the extra two are non-hex 64 characters and whitespace around a valid digest. The table includes an upper-case row built from the plan's own digest, with a precondition that it differs from the lower-case one.

### Pre-existing failures, not caused by this plan (logged, not fixed; see `deferred-items.md`)
- `internal/httpapi` `TestNoAssetIsInlinedPastTheContentSecurityPolicy`: the embedded built CSS inlines a `data:` font. Untouched by this plan (no web file changed).
- `internal/publicrepo` `TestNoIdentifierOfTheRealInstallationIsInTheHistory`: names commits 35675c0, 41eec4c, d94e372, 0d8e7ed, b75b44d, d48ea05, c35ec03. None is in `b3c55ef..HEAD` or an ancestor of BASE; the two checked (35675c0, 41eec4c) sit on the local branches `backup/local-main-2026-09-05` and `pre-public-rewrite`; the others were not traced. The tree half of the guard passes.

## Auth gates

None.

## Known Stubs

None.

## Threat Flags

None beyond the plan's register. T-17-01 (sentence leaks values), T-17-02 (digest as a switch), T-17-03/04 (lost update, hung request), T-17-05 (unbounded memo), T-17-06 (refused apply sets an expectation) each have a test and an injection above. T-17-07 (writer in the read-to-send window) is accepted as planned.

## Limits of what is proved (verbatim from the plan)

Proved only against the simulator and a simulated second client. Not proved:
- the re-encoding of a real Talos node (two readings without a change giving the same digest, assumption A3);
- the length of the window between reading and sending on a real node (A4);
- the `ErrApplyBusy` branch of the handler over HTTP: the route budget is 30 s and cannot be shortened, so only the service has a proof; the handler branch (409 `store.conflict`) has none;
- a run on the Pi (linux/arm64).

`go test -race` did not run (CI only).

## Next

Plan 02: interface (`web/src/routes/config.tsx`, `api.ts`), guide and README, declarative tables (budget row in `cmd/holzkube-managerd/budget_test.go`, audit allowlist for `base_digest`), ledger. Plan 02 reads the `BASE:` line above.

## Self-Check: PASSED

Created files exist (`sinceplan.go`, `sinceplan_test.go`, `changedsinceplan_test.go`, `refused_apply_test.go`); commits 473da17, 47d5e4e, 7efc8ac are ancestors of HEAD.
