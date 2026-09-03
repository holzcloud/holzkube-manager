---
phase: 02-transport-seam-talossim-image-factory
plan: 24
subsystem: api
tags: [go, imagefactory, http, conflict, compare-and-swap, react, tanstack-query, windows-ledger]

requires:
  - phase: 02-transport-seam-talossim-image-factory
    provides: "plan 02-22's scaled-ratio latency injection, the three imagefactory budget constants and the two route deadlines"
  - phase: 02-transport-seam-talossim-image-factory
    provides: "plan 02-23's concurrent candidate resolution, AssetsRouteBudget = ManifestTimeout + 5s, and the browser request ceiling"
provides:
  - "the store.ErrConflict branch of POST /api/v1/schematics writes the probe verdict it just computed to Usable, ProbedAt and ProbeReason under a compare-and-swap, and to no other field"
  - "two stated, tested and commented conditions under which the refresh is declined, and two compare-and-swap failures that answer the plain conflict without retrying or recreating"
  - "three 409 detail sentences chosen by outcome, so an operator reading the create form learns what became of the verdict"
  - "a no-verdict badge that names the recovery and what it will look like, promising no verdict"
  - "the create mutation invalidating the saved-list query on settle rather than on success"
  - "WINDOWS entries 57-65: the supersession of 20 and 48, G-02-9 recorded open, and this round's residuals"
  - "an implementation record appended below 02-DECISION-probe-budget.md's ratified text"
affects: [phase-03-inventory, phase-05-streaming, image-factory, schematics-ui]

actuals:
  tokens: 24242
  tasks: 3
  commits: 6

tech-stack:
  added: []
  patterns:
    - "a read-modify-write on a route that answers a conflict: Get, replace a named field set, Put with the read Rev, and answer the plain conflict on any failure — never retry, never recreate"
    - "byte-identity assertions over the marshalled record with the mutated fields cleared, so a field added to the struct later is covered without anyone editing the test"
    - "a one-shot store hook in the test harness, to be deterministically between a handler's read and its write"

key-files:
  created: []
  modified:
    - internal/httpapi/handlers/schematics.go
    - internal/httpapi/handlers/schematics_test.go
    - internal/model/model.go
    - docs/api-contract.md
    - web/src/api.ts
    - web/src/routes/images.tsx
    - web/src/routes/images.test.tsx
    - .planning/WINDOWS.md
    - .planning/phases/02-transport-seam-talossim-image-factory/02-DECISION-probe-budget.md
    - .gitignore

key-decisions:
  - "The 409 path writes Usable, ProbedAt and ProbeReason and nothing else, under two conditions: the fresh probe answered, and the stored Arch is the architecture it asked about"
  - "A lost compare-and-swap is not retried and a record deleted mid-refresh is not recreated — both answer the plain conflict"
  - "G-02-9 stays open and is carried as WINDOWS entry 58; the recovery is a re-submission answered as an error, not a re-probe route"
  - "WINDOWS 20, 48 and 8 marked fixed on stated evidence only: the composition guard and the offline elapsed-time tests, never on the code reading correctly"

patterns-established:
  - "Comment narrowing rather than replacement: when a behaviour changes, the surviving argument is kept verbatim in substance and only the clause that stopped being true is rewritten"
  - "A ledger claim that has changed gets a new entry opening in capitals with what it supersedes, never an edited row"

requirements-completed: [FACT-02, FACT-06]

coverage:
  - id: D1
    description: "A re-POST of an identical customisation whose fresh probe answers refreshes Usable, ProbedAt and ProbeReason on the stored record and still answers 409"
    requirement: "FACT-02"
    verification:
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestConflictRefreshesTheVerdictItJustComputed"
        status: pass
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestConflictRefreshStoresTheFactorysRefusal"
        status: pass
    human_judgment: false
  - id: D2
    description: "The refresh touches nothing but the three probe fields and Rev, asserted over the whole marshalled record"
    verification:
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestConflictRefreshTouchesNothingButTheThreeProbeFields"
        status: pass
    human_judgment: false
  - id: D3
    description: "The refresh is declined when the fresh probe did not answer and when the stored record holds another architecture's verdict; in both cases the record is byte-identical"
    verification:
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestConflictWithNoAnswerFromTheProbeChangesNothing"
        status: pass
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestConflictAtAnotherArchitectureDeclinesTheRefresh"
        status: pass
    human_judgment: false
  - id: D4
    description: "A lost compare-and-swap and a record deleted between the read and the write both answer the plain conflict, never retrying and never recreating"
    verification:
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestConflictRefreshLosingTheCompareAndSwapAnswersThePlainConflict"
        status: pass
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestConflictRefreshAgainstADeletedRecordDoesNotRecreateIt"
        status: pass
    human_judgment: false
  - id: D5
    description: "The 409 detail differs by outcome, so an operator reading the create form's error learns which one they got"
    verification:
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestConflictDetailNamesWhichRefreshOutcomeHappened"
        status: pass
    human_judgment: true
    rationale: "The test pins the three sentences apart and pins the absence of a promise, but whether the wording actually tells an operator what happened to their verdict is a reading judgment no assertion makes."
  - id: D6
    description: "model.go, docs/api-contract.md and web/src/api.ts each say what still holds and what changed about a second POST"
    verification:
      - kind: manual_procedural
        ref: "git show efb31da -- internal/model/model.go docs/api-contract.md web/src/api.ts"
        status: pass
    human_judgment: true
    rationale: "Prose narrowing. Nothing asserts that a narrowed sentence is now accurate rather than merely different; a reader has to check it against the branch."
  - id: D7
    description: "The no-verdict badge names the recovery and what it costs, and promises no verdict"
    verification:
      - kind: automated_ui
        ref: "web/src/routes/images.test.tsx#names the recovery on the no-verdict badge, and what it will look like"
        status: pass
      - kind: automated_ui
        ref: "web/src/routes/images.test.tsx#leaves the usable and refused branches exactly as they were"
        status: pass
    human_judgment: false
  - id: D8
    description: "The saved list is refetched after a create that failed, and the failed create's sentence survives that refetch until a form field changes"
    verification:
      - kind: automated_ui
        ref: "web/src/routes/images.test.tsx#refetches the saved list after a create that failed"
        status: pass
      - kind: automated_ui
        ref: "web/src/routes/images.test.tsx#keeps the failed create sentence through the refetch and drops it on an edit"
        status: pass
    human_judgment: false
  - id: D9
    description: "The ledger carries the supersession of entries 20 and 48, G-02-9 recorded open, and this round's residuals, with all three representations in agreement"
    verification:
      - kind: other
        ref: "gsd-tools windows status (open 52, waived 0, fixed 13, total 65; frontmatter, markdown rows and JSON entries all agree, ids 1..65 contiguous and unique)"
        status: pass
    human_judgment: true
    rationale: "The tool proves the three representations agree and the ids are well-formed. Whether each entry's prose is an accurate account of what this round moved is exactly the judgment entry 48 exists to force, and only a reader can make it."
  - id: D10
    description: "The implementation record is appended below the ratified decision text, which is unchanged byte for byte"
    verification:
      - kind: other
        ref: "head -c 7767 02-DECISION-probe-budget.md | shasum -a 256 == 24bc3e11f8c65a9bf30f933cd7885de323715be60a840b32dfaf7cb1f156ec31"
        status: pass
    human_judgment: false

duration: 23 min
completed: 2026-09-03
status: complete
---

# Phase 02 Plan 24: The conflict keeps the verdict it computed Summary

**The `409` on `POST /api/v1/schematics` now writes the probe verdict it already computed to `Usable`, `ProbedAt` and `ProbeReason` under a compare-and-swap and to nothing else — and G-02-9 is recorded as the open window the ratified decision says it is.**

## Performance

- **Duration:** 23 min
- **Started:** 2026-09-03T19:38:00Z
- **Completed:** 2026-09-03T20:01:00Z
- **Tasks:** 3
- **Files modified:** 10

## Accomplishments

- **The verdict stops being thrown away.** A re-POST of an identical customisation already ran the whole `Author` sequence including a fresh probe — G-02-9 measured `HTTP 409 in 4.186898875s`, a probe that ran and succeeded, against a record that stayed `usable=false` with `probed_at` zero. That verdict is now written back.
- **The write is bounded and the bound is asserted over the whole record.** `Usable`, `ProbedAt`, `ProbeReason`, and nothing else. The byte-identity test marshals the record before and after, clears those three and `Rev`, and compares — so a field added to `model.Schematic` next year is covered without anyone remembering the test exists.
- **Both declining conditions are stated, tested and argued where the decision is made.** The probe must have answered, and the stored `Arch` must be the architecture it asked about. A record written before `Arch` existed carries an empty one and is never refreshed, deliberately.
- **Neither compare-and-swap failure invents a write path.** A lost CAS lets the concurrent writer win; a record deleted mid-refresh is not recreated. Both answer the plain conflict, and the no-retry is written down as the decision rather than left as an omission.
- **Three statements in the tree that said a second POST is refused rather than merged now say what is still true** — `model.Schematic.ID`, `model.Schematic.Arch`, `docs/api-contract.md` — and `web/src/api.ts` warns a client that `probed_at` may have been written by a later conflicting POST.
- **The badge names the recovery and its price**, and the create mutation refetches the saved list on settle, because a failed create can now change stored state.
- **The ledger says exactly what this round moved.** Entries 20, 48 and 8 are fixed on stated evidence; entry 58 keeps G-02-9 open; six further entries carry the round's residuals.

## Task Commits

1. **Task 1 (RED): eight conflict-path tests** — `6596e40` (test)
2. **Task 1 (GREEN): the refresh, the two conditions, the three sentences, the narrowed statements** — `efb31da` (feat)
3. **Task 2 (RED): the recovery copy, the refetch, the stale-error survival** — `b84e001` (test)
4. **Task 2 (GREEN): the badge copy, the ratified-decision comment, `onSettled`** — `0328030` (feat)
5. **Task 3: the ledger and the implementation record** — `970855d` (docs)

**Plan metadata:** see the `docs(02-24)` commit.

_Task 1 and Task 2 are `tdd="true"`; each produced a `test` commit before its `feat` commit. Neither needed a `refactor` commit._

## Files Created/Modified

- `internal/httpapi/handlers/schematics.go` — the narrowed conflict-branch comment, `conflictDetail`, `refreshTheStoredVerdict`, `archMismatchReason`.
- `internal/httpapi/handlers/schematics_test.go` — eight conflict-path cases, the `storeHook` / `hookedStore` / `hookedSchematics` harness and `withStoreHook`, `conflictRefreshServer`, `coldCreate`, `storedRecord`, `exceptTheProbeFields`, `mustString`, `mustNumber`; and the narrowed revision assertion in `TestCreateTheSameContentTwiceIs409NotInternal`.
- `internal/model/model.go` — `Schematic.ID` and `Schematic.Arch` narrowed.
- `docs/api-contract.md` — `POST /api/v1/schematics` gains the refresh, its two conditions and its three outcomes; the second-architecture paragraph gains the clause noting it is also the declined case.
- `web/src/api.ts` — `usable` and `probed_at` warn that a later conflicting POST may have written them.
- `web/src/routes/images.tsx` — the no-verdict recovery sentence, the ratified-decision comment correction, `onSuccess` split from `onSettled`.
- `web/src/routes/images.test.tsx` — `fail.method`, `RECOVERY_COPY`, `collapsed`, and the four new cases.
- `.planning/WINDOWS.md` — entries 57–65; 8, 20, 48 and 63 marked fixed.
- `.planning/phases/.../02-DECISION-probe-budget.md` — the implementation record, appended.
- `.gitignore` — `.planning/state.json` (see Deviations).

## The numbers this plan was asked to record

**Route count, task 1.** `grep -c 'httpapi.Route{' internal/httpapi/handlers/schematics.go` reported **1** before the task and **1** after. No route was added, which is one of the four things separating the ratified Option 2 from Option 1.

**Ledger counts, all three representations.** Frontmatter, markdown table rows by status, and JSON block entries by status:

| | open | waived | fixed | total |
|---|---|---|---|---|
| frontmatter | 52 | 0 | 13 | 65 |
| markdown table | 52 | 0 | 13 | 65 |
| JSON block | 52 | 0 | 13 | 65 |

Ids are contiguous `1..65` and unique in both the table and the JSON block, and the two id lists are identical.

**The evidence entries 20 and 48 were marked fixed on**, and nothing else:

- `cmd/holzkube-managerd/budget_test.go` `TestRouteBudgetsComposeAgainstWriteTimeout` passes and declares **both** Factory routes `withinBudget` with an empty `deferredTo` — assets: `30s sum, largest call 30s, route deadline 35s, slack 5s, writeTimeout 2m10s`; create: `2m30s sum, largest call 1m30s, route deadline 2m0s`.
- The offline elapsed-time tests pass: `TestAssetsRouteAnswersInsideItsCeiling` answered in **603.727542ms** against a 700ms scaled ceiling, `TestCreateRouteAnswersInsideItsCeiling` in **623.888083ms** against 2.4s.

They were **not** marked fixed on the code reading correctly, and the superseding entry 57 says so in those words.

**Decision-document prefix digest.** `head -c 7767 02-DECISION-probe-budget.md | shasum -a 256` is `24bc3e11f8c65a9bf30f933cd7885de323715be60a840b32dfaf7cb1f156ec31` after the append, unchanged. The file grew from 7767 to 11899 bytes.

## What this round moved on the assets route, and what it did not

Stated here because it is the confusion WINDOWS entry 48 exists to prevent.

**Moved.** `resolveInstallerRepo` no longer walks its candidates serially: plan 02-23 issues every candidate at once on a context derived from the caller's and decides in declared candidate order, so the route's worst case is one candidate's budget rather than the sum. `AssetsRouteBudget` went 65s → 35s, derived as `ManifestTimeout + 5s`. `writeTimeout` went 60s → 130s and now names the budgets it covers.

**Did not move.** Nothing re-measured the installer resolution against `factory.talos.dev` after the change. Plan 02-22's two live runs on 2026-09-03 measured the **ISO probe** (cold 28.445563875s and 31.491771083s, warm 3.752628125s and 2.493417375s) and not the installer resolution. **The improvement is measured offline against fakes and unmeasured against `factory.talos.dev`** — 305.96ms against 606.93ms serialised at the client, 603.57ms against 1.0599s serialised at the HTTP route. G-02-2's 43.42s and 60.002907792s remain the correct record of what the serial walk cost, and now live in a comment beside `installerRepoRetryInterval`.

A route that declares a ceiling is not the same as a route that is fast. Plan 02-22's executor said that about its own change; it is still the right sentence about this one.

## G-02-9 is not closed

This plan **mitigates** G-02-9 and leaves it **open**. That is the frontmatter's claim (`gap_ids: [G-02-1]`, with G-02-9 in `gaps_mitigated_not_closed`), the decision document's claim, and WINDOWS entry 58's claim.

What exists now: a re-submission of the identical customisation answers `409` and refreshes the verdict as a side effect. What does not exist: a re-probe route, a re-probe button, a re-probe job. The recovery is not discoverable from the saved list, it reaches the operator as an error on the create form rather than as an action, it is declined when the record holds another architecture's verdict or when the fresh probe does not answer either, and a probe that times out again leaves the record exactly as it was. The structural answer is `02-DECISION-probe-budget.md`'s Option 1, which the user did not take.

## Ledger entries filed

Nine appended (57–65), four marked fixed (8, 20, 48, 63). Every residual plans 02-22 and 02-23 recorded under `## Ledger entries to file` is filed.

| id | kind | file | what it records | status |
|---|---|---|---|---|
| 57 | deviation | `internal/imagefactory/installer.go` | supersedes 20 and 48; what moved and what did not, with the measured figures | open |
| 58 | deviation | `internal/httpapi/handlers/schematics.go` | G-02-9 remains open after round 4 | open |
| 59 | deviation | `cmd/holzkube-managerd/main.go` | `writeTimeout` raised process-wide, cross-referenced to the Phase 5 `Flush`/`Unwrap`/`Hijack` blocker | open |
| 60 | deviation | `internal/httpapi/handlers/schematics.go` | `CreateRouteBudget` 120s is one JSON budget below its route's 150s call sum; outcome is the existing fail-safe | open |
| 61 | deviation | `internal/httpapi/middleware/audit.go` | the archive does not show the refresh (T-02-112, accepted) | open |
| 62 | todo | `web/src/routes/images.tsx` | the PITFALLS:164 progress indicator is still not built; a stated ceiling is a different thing | open |
| 63 | todo | `internal/imagefactory/client.go` | supersedes 8, carrying the reason the `fixed` verb cannot | fixed |
| 64 | unrun-verify | `internal/imagefactory/live_test.go` | supersedes 5, which stays open; records the elapsed-time bound and the `NOT OBSERVED` skip | open |
| 65 | todo | `cmd/holzkube-managerd/budget_test.go` | known limitation of the guard: an over-provisioned half-change passes R1 and R2 | open |

Entry 63 is marked fixed on creation because its content is a resolved statement — entry 8's cause is gone — and the `fixed` verb carries no reason field, so the reason had to become an entry. Recorded here rather than left to be noticed.

## Decisions Made

- **The verdict is refreshed, the operator's text is not.** The conflict branch's existing argument — a `Destructive: false` route must not do a destructive route's damage to the label, the cluster and the version — is kept verbatim in substance. The verdict is exempt for a stated reason: it is not the operator's text, the second POST carries no competing version of it, and the fresh probe is a strictly better-informed answer to the same question.
- **No retry on a lost compare-and-swap.** A retry loop on a route that is already answering a conflict would be inventing a write path nobody asked for.
- **The refresh is silent on failure.** A failed `Get` or `Put` answers the plain `409`. The refresh is a bonus and must never turn a conflict into a `500`.
- **Three outcome sentences, and the third names its own reason.** Refreshed-and-builds, refreshed-and-refused, and not-refreshed — where not-refreshed says which of its two conditions applied, naming both architectures in the mismatch case.
- **The unanswered sentence offers a retry and never a guarantee.** "Submitting the same customisation again runs it once more, which is a retry rather than a guarantee." A promise the product cannot keep is the failure the badge copy already had once.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 — Bug] `TestCreateTheSameContentTwiceIs409NotInternal` asserted the old behaviour**

- **Found during:** Task 1 (GREEN)
- **Issue:** The pre-existing conflict test asserted `stored.Rev == first.Rev` — "the refused create wrote". That is precisely the claim this plan narrows: the refresh is exactly one compare-and-swap write, so the revision advances by one while nothing the operator wrote is touched.
- **Fix:** Removed the revision assertion and replaced it with a comment naming `TestConflictRefreshTouchesNothingButTheThreeProbeFields` as the test that now owns that bound over the whole marshalled record. The label assertion — the part of the old claim that is still true — is untouched.
- **Files modified:** `internal/httpapi/handlers/schematics_test.go`
- **Verification:** `go test ./internal/httpapi/... ./internal/store/... -count=1 -race` green.
- **Committed in:** `efb31da`

**2. [Rule 3 — Blocking] `errcheck` refused the unchecked map type assertions**

- **Found during:** Task 1 (GREEN), at `golangci-lint run`
- **Issue:** The new tests read the wire form as `map[string]any` and asserted `.(string)` / `.(float64)` unchecked — 11 `errcheck` findings.
- **Fix:** Added `mustString` and `mustNumber` helpers that `t.Fatalf` with the actual type and the whole record.
- **Files modified:** `internal/httpapi/handlers/schematics_test.go`
- **Verification:** `golangci-lint run` → `0 issues.`
- **Committed in:** `efb31da`

**3. [Rule 3 — Blocking] `biome` reflowed the new badge sentence**

- **Found during:** Task 2 (GREEN), at `npm --prefix web run lint`
- **Issue:** The added JSX text did not match biome's fill.
- **Fix:** Reflowed to biome's output.
- **Files modified:** `web/src/routes/images.tsx`
- **Verification:** `npm --prefix web run lint` clean.
- **Committed in:** `0328030`

**4. [Rule 3 — Blocking] `stubFactory.fail` could not express a POST-only failure**

- **Found during:** Task 2 (RED)
- **Issue:** `fail` matched on path prefix alone, and `/api/v1/schematics` is both the create route and the saved-list route. A 409 keyed on the path would also have broken the list, making "a failed create still refetches the list" unobservable.
- **Fix:** Added an optional `method` to `fail`, with the reason written beside it.
- **Files modified:** `web/src/routes/images.test.tsx`
- **Verification:** the refetch case observes exactly two list fetches.
- **Committed in:** `b84e001`

**5. [Rule 3 — Blocking] `.planning/state.json` left untracked by the tooling**

- **Found during:** Task 3
- **Issue:** `gsd-tools` generated `.planning/state.json`, which has never been tracked in this repository and is not gitignored. The executor protocol forbids leaving a generated file untracked.
- **Fix:** Added it to `.gitignore` beside `.planning/milestone.lock`, which is the same class of machine artifact and the existing precedent here.
- **Files modified:** `.gitignore`
- **Verification:** `git status --short` clean.
- **Committed in:** the `docs(02-24)` metadata commit.

### Acceptance criteria that did not hold as written

**Task 1: "Reverting the refresh makes exactly the three refreshing tests fail and leaves the other five passing."**

Driven locally and restored, as the criterion asks. The real number is **seven of the eight fail**, and the one that passes is `TestConflictWithNoAnswerFromTheProbeChangesNothing` — the case that must not change. The criterion under-counted because five of the eight cases assert the *sentence* the refresh path produces, or the bound on it, and every one of those is absent when the refresh is gone: the arch-decline case checks the detail names both architectures, the lost-CAS case checks the detail does not claim a refresh, and the three-sentence case checks the three are distinct. Recorded rather than smoothed over: the guard is stronger than the criterion assumed, not weaker, but the criterion's number is wrong and a later reader running the experiment should not think something regressed.

**Total deviations:** 5 auto-fixed (1 bug, 4 blocking) and 1 acceptance criterion corrected in the record. **Impact:** no scope creep. Nothing was added that the plan did not ask for; no route, no probe state, no stored field, no response field, and the `409` status code is unchanged.

## Issues Encountered

None. Both TDD cycles went red then green on the first implementation attempt.

## Known Stubs

None. No hardcoded empty value, placeholder string or unwired component was introduced. `refreshTheStoredVerdict`'s two silent-failure returns are a specified behaviour with tests, not a stub.

## Threat Flags

None. The plan's `<threat_model>` covers the whole of the new surface: T-02-110 (a `Destructive: false` route writing to an existing record) and T-02-111 (an arm64 verdict on an amd64 record) are mitigated and each has its own test; T-02-112 (the archive not showing the refresh) and T-02-113 (repeated re-POSTs driving repeated builds) are accepted, and T-02-112 is filed as WINDOWS entry 61. `T-02-SC` holds: no Go module and no npm package added. Ids T-02-110..T-02-113 were verified free against the phase directory before use; **the next free id is T-02-114.**

## Verification

| Check | Result |
|---|---|
| `go build ./... && go vet ./...` | clean |
| `go test ./... -count=1 -race` | green, every package |
| `go test ./internal/httpapi/... ./internal/store/... -count=1 -race` | green |
| `go test ./internal/audit/... -count=1` | green |
| `npm --prefix web test` | 9 files, 137 tests, both vitest projects |
| `npm --prefix web run lint` / `run typecheck` | exit 0 |
| `golangci-lint run` (`~/go/bin/golangci-lint`) | `0 issues.` |
| `gofmt -l ./internal` | prints nothing |
| `gsd-tools windows status` | agrees with the frontmatter, the markdown table and the JSON block |
| decision-document prefix digest | `24bc3e11…f156ec31`, unchanged |

## User Setup Required

None — no external service configuration required.

## Next Phase Readiness

- Round 4 of gap closure is complete. G-02-1's badge and recovery half is closed; G-02-2's composition half is closed and its live half is unmeasured and recorded as such; G-02-9 is mitigated and open.
- Phase 2 has 24 of 24 plans summarised. The outstanding windows for this phase are the 52 open ledger entries, of which entries 57–65 are this round's and are each written to be checkable against the tree.
- **For Phase 5:** WINDOWS entry 59 cross-references the process-wide `writeTimeout` to the `Flush`/`Unwrap`/`Hijack` blocker in `02-CONTEXT.md` `<deadline_policy>`. Phase 5 should find it there rather than re-derive it.
- **Concern, stated plainly:** the assets route's improvement has never been measured against the live Factory. `TestLiveFactory` remains opt-in and scheduled by nothing (WINDOWS entries 5 and 64).

---
*Phase: 02-transport-seam-talossim-image-factory*
*Completed: 2026-09-03*
