---
phase: 02-transport-seam-talossim-image-factory
plan: 19
subsystem: api
tags: [image-factory, rfc9457, secureboot, partial-response, zod, react, go]

requires:
  - phase: 02-16
    provides: "Option C and the WarningInstallerSecureBootRepoFallbackUnverified constant in warnings.go, with its contract surface deliberately deferred here"
  - phase: 02-17
    provides: "the rewritten Taskfile; gates in this plan were run directly rather than through it"
  - phase: 02-18
    provides: "the problem-type taxonomy re-rooted at urn:holzkube-manager:problem:, which is the base every code in this plan's fixtures carries"
provides:
  - "GET /schematics/{id}/assets returns the four registry-free references even when the installer cannot be resolved, with `installer: null` and an `installer_error` carrying the problem's code and detail"
  - "An asset panel that renders the four references it received and explains the installer's absence in the server's own words, separating a retryable non-answer from a verdict about this version"
  - "The assets route's real atomicity, and its four outcomes, written into docs/api-contract.md"
  - "Handler-level tests for the refused, unreachable and provisional installer branches, none of which had one"
  - "A warning-code drift guard that enumerates instead of naming, so a Go code with no TS mirror goes red"
affects: [02-20, 02-21, 02-DECISION-probe-budget, upgrade-rpc-consumers]

actuals:
  tokens: 78000
  tasks: 4
  commits: 3

tech-stack:
  added: []
  patterns:
    - "A partial success answers 200 with the withheld field explicitly null and a sibling member saying why, rather than a problem body that discards what was computed"
    - "A client-side remedy is derived from the problem `code`; the sentence itself is always the server's `detail`"

key-files:
  created: []
  modified:
    - internal/httpapi/handlers/schematics.go
    - internal/httpapi/handlers/schematics_test.go
    - internal/imagefactory/warnings_test.go
    - web/src/api.ts
    - web/src/lib/problem.ts
    - web/src/routes/images.tsx
    - web/src/routes/images.test.tsx
    - docs/api-contract.md

key-decisions:
  - "Option A on task 1's gate (pre-answered by the user): 200, `installer` null, and an `installer_error` member carrying the problem's code and detail. The null is load-bearing — it makes the unresolved case a decode-time fact rather than something a client must remember to check."
  - "`installer_error` is absent, never null, on a resolved answer, so proven and provisional responses stay byte-identical and the member's presence is itself the signal."
  - "The client derives only the *remedy* from the code; the sentence shown is the server's `detail` verbatim, so one condition is never described as two problems."
  - "The SecureBoot clause is local client copy by necessity — `secureboot` is a query parameter that never reaches the stored record, so the client is the only place that knows the request asked for it."
  - "TestWarningDetailsMatchTheUI now enumerates exported Warning* codes rather than naming them, closing the blind spot that let 02-16 ship a Go code with no TS mirror and stay green."

patterns-established:
  - "Partial-answer shape: withheld field null + `<field>_error` {code, detail}, member omitted when the field resolved"
  - "Contract outcome tables: every combination of the withheld field, its error member and `warnings` is enumerated, so no field's meaning is left implied"

requirements-completed: [FACT-03, FACT-02]

coverage:
  - id: D1
    description: "An assets request whose installer cannot be resolved still returns the ISO, PXE, disk-image and cmdline references, with their exact values"
    requirement: FACT-03
    verification:
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestAssetsWithARefusedInstallerStillReturnsTheOtherFourReferences"
        status: pass
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestAssetsWithAnUnreachableRegistryStillReturnsTheOtherFourReferences"
        status: pass
    human_judgment: false
  - id: D2
    description: "The installer alone is marked unresolved, in a shape a client cannot mistake for a proven reference or for an absent one"
    requirement: FACT-02
    verification:
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestAssetsWithARefusedInstallerStillReturnsTheOtherFourReferences (asserts \"installer\":null literally present, installer_error present)"
        status: pass
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestAssetsProvenAndProvisionalAnswersAreUnchanged (asserts installer_error absent on both resolved outcomes)"
        status: pass
    human_judgment: false
  - id: D3
    description: "The operator-facing message distinguishes 'wait and retry' from 'this version has no installer under that name', and comes from the server's own detail"
    requirement: FACT-02
    verification:
      - kind: unit
        ref: "web/src/routes/images.test.tsx#renders the four references it did receive when the installer could not be resolved"
        status: pass
      - kind: unit
        ref: "web/src/routes/images.test.tsx#separates a refused installer from a registry that did not answer"
        status: pass
      - kind: other
        ref: "provenance probe: the stub detail was replaced with an arbitrary sentence and the suite stayed green, proving the rendered text is the server's and not component copy"
        status: pass
    human_judgment: false
  - id: D4
    description: "When the request asked for SecureBoot the message says so and says what unticking the control would change"
    requirement: FACT-02
    verification:
      - kind: unit
        ref: "web/src/routes/images.test.tsx#names SecureBoot, and what unticking it would ask instead, only when it was ticked"
        status: pass
    human_judgment: false
  - id: D5
    description: "The provisional-installer branch has a handler-level test (WINDOWS entry 23)"
    verification:
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestAssetsProvenAndProvisionalAnswersAreUnchanged/provisional"
        status: pass
    human_judgment: false
  - id: D6
    description: "docs/api-contract.md describes the assets route's atomicity as the code implements it, with four outcomes and what `warnings` means in each"
    verification: []
    human_judgment: true
    rationale: "Whether the description a human is asked to ratify now matches the behaviour is the judgement the UAT filed as required before ship. No test can assert that a prose section is candid; it has to be read against the handler."
  - id: D7
    description: "02-16's Option C contract surface: the TS mirror constant, the warning-table row, and the assets-route text naming both installer codes"
    verification:
      - kind: unit
        ref: "internal/imagefactory/warnings_test.go#TestWarningDetailsMatchTheUI (enumerating; verified red by drifting the TS constant)"
        status: pass
    human_judgment: false

duration: 29 min
completed: 2026-08-30
status: complete
---

# Phase 02 Plan 19: Return what was computed when the installer cannot be resolved Summary

**The assets route stops discarding four registry-free references because a fifth could not be resolved: `200` with `installer: null` and an `installer_error` carrying the problem's code and detail, a panel that tells "wait and retry" from "this version has no SecureBoot installer" in the server's own words, and a contract that finally describes the atomicity the code implements.**

## Performance

- **Duration:** 29 min
- **Started:** 2026-08-30T08:34:00Z
- **Completed:** 2026-08-30T09:02:52Z
- **Tasks:** 4 (one a pre-answered decision gate)
- **Files modified:** 8

## Accomplishments

- **G-02-15 closed on both `missing` bullets.** `schematics.go`'s `if err != nil { WriteProblem; return }` threw away ISO, PXE, disk-image and cmdline — four values built by pure string assembly that never touches the registry — because the installer could not be resolved. They are now returned, and the installer alone is marked unresolved.
- **The unresolved case cannot be read as a proven one.** `installer` is `null`, not an empty string: `warnings: []` on this route is contractually an affirmative claim that the repository name was *proven*, so an empty string would have carried that claim about a reference that does not exist. `installer_error` is *absent* rather than null on a resolved answer, so proven and provisional responses are byte-identical to what they were.
- **The panel says which of the two things happened.** The old branch was one hardcoded `role="alert"` sentence that never read `assets.error`, never named SecureBoot and never suggested unticking the control that would have worked — so a `400` naming an architecture and a registry that had not answered in 50 seconds produced identical words for opposite remedies. The sentence is now the server's `detail`; the client adds only the remedy, keyed on the code.
- **The branch has tests on both sides of the wire, where it had none.** Two handler tests (refused, unreachable) plus four web tests. The handler tests assert the four references' *exact* values, derived through `imagefactory`'s own URL builders rather than transcribed.
- **WINDOWS entry 23 is fixed as a side effect.** The provisional branch now has a handler-level test. The comment that said this package's fake could not produce a transport failure "without hijacking its connection too" was right about the premise and wrong about the conclusion — hijacking is six lines.
- **The contract matches the code a human was asked to ratify.** `docs/api-contract.md:655` said the route answers "with no installer reference" while it answered with no references at all. It now states the route's real (non-)atomicity, enumerates four outcomes, and gives a table of what `warnings` means in each.
- **02-16's Option C handoff is landed**, and the drift guard that let it be deferred silently is closed.

## Task Commits

1. **Task 1: the wire shape of a partial assets answer** — no commit; a decision gate, pre-answered by the user as Option A (see Decisions Made).
2. **Task 2: return what was computed, and say what was not** — `622048e` (feat, TDD)
3. **Task 3: an asset panel that says which of the two things happened** — `ed296eb` (feat, TDD)
4. **Task 4: land 02-16's follow-up and close the record** — `77c920f` (feat)

TDD cycles were run as RED-then-GREEN within each task and committed once at GREEN rather than as separate `test(...)` commits — see Deviations.

## Files Created/Modified

- `internal/httpapi/handlers/schematics.go` — `assetReferences.Installer` is now `*string`; new `installerUnresolved` member; `schematicAssets` splits the installer failure from the request failure and serves what it computed. The comment now carries both halves of the rule: never substitute, and never let that rule take four unrelated values down with it.
- `internal/httpapi/handlers/schematics_test.go` — `answerEveryManifestWith` and `dropTheConnectionFor` on the fake; `assetsBody`, `wantAssetURLs`, `assertFourReferences`; three new tests (one with two subtests); the unknown-arch test now also asserts that a pre-URL failure returns no references at all.
- `internal/imagefactory/warnings_test.go` — `TestWarningDetailsMatchTheUI` enumerates the exported codes instead of naming one.
- `web/src/api.ts` — `installer` is nullable, `installer_error` is optional, both documented with what their absence means; `WARNING_INSTALLER_SECUREBOOT_REPO_FALLBACK_UNVERIFIED` mirrored from Go.
- `web/src/lib/problem.ts` — `CODE_UPSTREAM_FACTORY_UNAVAILABLE` / `CODE_UPSTREAM_FACTORY_REJECTED`, named where the taxonomy lives.
- `web/src/routes/images.tsx` — the whole-request alert now renders `messageFor(assets.error)`; new `UnresolvedInstallerRow`; a sentence saying the four references that did arrive are correct and usable.
- `web/src/routes/images.test.tsx` — `installerError` stub option, `assetsFor` produces the unresolved shape, four new tests.
- `docs/api-contract.md` — the assets section's atomicity, four outcomes, the `warnings`-meaning table, `installer_error`, the fourth warning-code row and the reconciled "closed taxonomy" text.

## Decisions Made

- **Task 1 gate: Option A**, pre-answered by the user on 2026-08-30 and carried in the plan's `<pre-decision>`, so the gate did not block. Execution did not find cause to revisit it; the `<rejected-alternative>` (keep the `502`, carry the references as RFC 9457 extension members) stayed rejected and no escalation was needed. The null proved its worth immediately: making `installer` nullable in `api.ts` produced a TypeScript error at the exact render site that would otherwise have shipped an empty row.
- **`installer_error` carries no `status` member.** It travels inside a `200`; a status here would be a number describing a response that never happened.
- **The remedy sentences are client copy, keyed on `code`; the descriptive sentence is always the server's `detail`.** The server does not state a remedy and the two remedies are opposite, so this is the one thing the client legitimately adds. Verified non-hardcoded by replacing the stubbed detail with an arbitrary string and watching the suite stay green.
- **The SecureBoot clause says unticking asks a *different question*, not that the ordinary installer would do.** Wording checked by a test asserting "does not produce a SecureBoot node" is present.
- **T-02-53's ratified half was not reopened.** No substitution path exists in any outcome added here, and the handler comment now records *why* — SecureBoot is a per-request query parameter absent from the stored record, so a substitution would be undetectable forever by construction. A test asserts no reference string appears in the refused SecureBoot body at all.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] `web/src/routes/images.tsx` had to be touched in task 2, which does not own it**

- **Found during:** Task 2, at the `npm --prefix web run typecheck` acceptance criterion
- **Issue:** Task 2's `<files>` are the handler, its test, `api.ts` and the contract; `images.tsx` belongs to task 3. But making `installer` nullable in `api.ts` immediately broke `AssetRow`'s `value: string` at `images.tsx:1229`, and task 2's own acceptance criterion requires `typecheck` to exit 0. The plan did not anticipate that the schema change surfaces the render site.
- **Fix:** The smallest type-honest change — the installer row was omitted while null, with a comment naming task 3 as its owner. It is not a stub: it displays nothing false, and it renders no empty value beside four correct references. Task 3 replaced it with `UnresolvedInstallerRow` in the next commit, so the seam existed for one commit.
- **Files modified:** `web/src/routes/images.tsx`
- **Verification:** `npm --prefix web run typecheck` exits 0 at `622048e`; the branch is fully rendered and tested at `ed296eb`.
- **Committed in:** `622048e`

**2. [Rule 2 - Missing Critical] The warning-code drift guard was extended beyond the plan's ask**

- **Found during:** Task 4
- **Issue:** The plan says "run the guard that checks the two sides agree". There was no such guard for the new code. `TestWarningDetailsMatchTheUI` checked installer-family codes *by name*, one at a time — 02-16-SUMMARY.md names this as the reason it could ship the Go half with no TS mirror and stay green. Adding the mirror while leaving the guard blind would have left the next code in exactly the same position.
- **Fix:** The test now loops over `exportedWarningCodes(t)`, the AST walk the file already had, so any exported `Warning*` constant is covered the moment it is declared. The failure message names the two places to update.
- **Files modified:** `internal/imagefactory/warnings_test.go`
- **Verification:** Drifted the TS constant to `installer.secureboot-repo-fallback-DRIFTED`; the test failed naming the constant and both files; restored and green.
- **Committed in:** `77c920f`

**3. [Process] TDD gate commits were not split into `test(...)` then `feat(...)`**

- **Found during:** Tasks 2 and 3
- **Issue:** Tasks 2 and 3 carry `tdd="true"`, and the executor protocol's RED gate calls for a `test(...)` commit before implementation. RED was run and observed — the two new handler tests failed with `got 502, want 200` before the handler changed, and all four web tests failed before the panel changed — but each task was committed once, at GREEN.
- **Why:** Each task's acceptance criteria require the Go, TS and contract halves to land *in the same commit* ("Mirror the shape in `web/src/api.ts` in the same commit"; "update `docs/api-contract.md` … in the same task"). A separate RED commit would have left a commit where `api.ts` and the handler disagree. The RED evidence is recorded here and in the Verification section instead of in the log.
- **Verification:** The plan's "reverting the handler change makes the two new tests fail" criterion was satisfied directly by the observed RED run, which is that revert.

---

**Total deviations:** 3 (1 blocking auto-fix, 1 missing-critical auto-fix, 1 process note)
**Impact on plan:** None on scope. Deviation 1 is one commit's worth of intra-plan sequencing; deviation 2 makes the mechanism the plan asked to "run" actually exist; deviation 3 is a commit-granularity note with the RED evidence preserved.

## Prohibitions Honoured

- **T-02-53's ratified half untouched.** No outcome added here substitutes an ordinary installer for a SecureBoot request. `TestAssetsWithARefusedInstallerStillReturnsTheOtherFourReferences` asserts no reference string appears in the body at all.
- **No installer reference is assembled, guessed or defaulted.** The four references this plan stops discarding are the ones already built without the registry; `installer` stays registry-proven or null.
- **No timeout, deadline or budget changed.** `git diff 65b9b20..HEAD` touches no timeout constant, no `writeTimeout`, and no `budget_test.go`.
- **`internal/imagefactory/installer.go`, `probe.go` and `schematicid.go` untouched.** The only `imagefactory` file changed is `warnings_test.go`; `warnings.go` itself needed no edit, because 02-16 had already declared the constant.
- **`.planning/WINDOWS.md` untouched.** Residuals are handed to plan 02-21 below.

## Interaction with the open probe-budget decision

`02-DECISION-probe-budget.md` stays open and this plan answers none of it. **The timeout is not addressed and must not be read as addressed.** WINDOWS entry 20 records the cold two-candidate serial walk at `2 × 30s` against a `60s` writeTimeout, and the 49.8s and 60.002s observations in G-02-15 are that composition. This plan changes *what a timeout returns*, not how often one happens — an operator who now sees four references on a slow request is meeting the same 60-second wall, with a better answer at the end of it. If anything, a milder symptom makes the underlying budget question easier to defer, which is the opposite of what the measurements warrant.

## Ledger entries to file

`.planning/WINDOWS.md` was **not** touched — plan 02-21 owns the ledger for this round. These belong in it:

| Kind | Where | Description |
|---|---|---|
| `todo` → **mark entry 23 `fixed`** | `internal/httpapi/handlers/schematics_test.go` | [R2-WR-06] The provisional-warning branch of `GET /assets` now has a handler-level test: `TestAssetsProvenAndProvisionalAnswersAreUnchanged/provisional`, which drops the connection on `metal-installer`, asserts the legacy name resolved, and asserts exactly one `installer.repo-fallback-unverified` warning survives the handler with a non-empty detail. The stale comment claiming the fake could not produce a transport failure has been corrected in place. Close it — this is a first-hand reading, not an inference. |
| `deviation` | `internal/imagefactory/installer.go` | **Entry 20 is unchanged and must stay open.** This plan changed what an unresolved installer returns, not the cold `2 × 30s` walk that produces it. The 49.8s and 60.002s observations recorded in G-02-15 are that composition. Do not let "the panel now shows four references on a timeout" read as the timeout having been addressed; it is still owned by `02-DECISION-probe-budget.md`. |
| `deviation` | `docs/api-contract.md` | The assets route's response shape changed for two outcomes: `502` with no body became `200` with `"installer": null` plus `installer_error`. `installer` is now nullable on every answer. No external clients exist, which is why this is a note and not a migration — but the note is the difference between having decided that and not having noticed it. Matches T-02-91/T-02-92. |
| `todo` (still open, **not** fixed here) | `docs/api-contract.md` | Entries 25 and 26 (R2-IN-02, R2-IN-03) sit inside the assets section this plan rewrote and were deliberately left alone: both are marked "scoped out by the user", and a scoping decision is not the executor's to overturn. Entry 25's SecureBoot example still omits `warnings`, and it now sits beside a table that enumerates what `warnings` means in every outcome, so the inconsistency is more visible than before. Worth re-offering to the user rather than closing silently. |
| `todo` (**partially** addressed) | `web/src/api.ts` | Entry 27 records `WARNING_INSTALLER_REPO_FALLBACK_UNVERIFIED` as exported, pinned by the Go guard and unused by application code. Still true, and now true of a second constant: `WARNING_INSTALLER_SECUREBOOT_REPO_FALLBACK_UNVERIFIED` is also unused, because `SchematicWarnings` renders `{code}`/`{detail}` generically with no per-code branch. That is the intended design (an unknown code still reaches the operator), so the entry is about the constants' purpose being drift-pinning rather than rendering — the guard now enumerates, which is what gives them that purpose. Re-word rather than close. |

## Known Stubs

None. The one transient omission (the installer row while `installer` was null, in `622048e`) existed for a single commit inside this plan and is fully rendered and tested at `ed296eb`.

## Threat Flags

None. No new endpoint, auth path, file access pattern or schema at a trust boundary. The change removes a self-inflicted denial of service (T-02-91) and adds one response member whose entire design constraint — that a pre-existing client cannot read it as proof (T-02-92) — is what the null and the `omitempty` implement. T-02-93 (a substituted or assembled installer reference) is restated as a prohibition and asserted by test. T-02-94 (a contract milder than the behaviour) is closed by the contract landing in the same commit as the handler.

## Verification

| Check | Result |
|---|---|
| `go test ./... -count=1 -race` | green, all 15 packages |
| `npm --prefix web test` | green, 8 files / 123 tests (was 119) |
| `PATH=…:$(go env GOPATH)/bin golangci-lint run` | `0 issues.` |
| `gofmt -l ./cmd ./internal` | no output |
| `npm --prefix web run lint` | `Checked 49 files. No fixes applied.` |
| `npm --prefix web run typecheck` | exits 0 |
| RED evidence, task 2 | both new handler tests failed `got 502, want 200` before the handler change |
| RED evidence, task 3 | all four new web tests failed before the panel change |
| Message provenance | stub detail replaced with `PROVENANCE PROBE: a sentence no component could contain.` → suite still green, proving the rendered sentence is the server's |
| Drift-guard bite | TS constant drifted → `TestWarningDetailsMatchTheUI` failed naming the constant, `api.ts` and the contract table |

`task lint` and `task test` were **not** used as the entry points: plan 02-17 rewrote `Taskfile.yml`, and the dispatch directed gates to be run directly. The underlying commands above are the same ones those targets wrap.

## Issues Encountered

None. `golangci-lint` was present at `~/go/bin/golangci-lint` (2.13.1) as recorded by 02-16, the repository linted clean at 0 issues before and after, and no pre-existing failure was encountered.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- G-02-15 is closed on both bullets, and the UAT's "required before ship" item — the assets-route atomicity written into `docs/api-contract.md` — is discharged.
- 02-16's handoff is fully landed; nothing dangles between the two plans.
- Plan 02-21 has five ledger entries above, one of which (entry 23) is a close.
- **Open and untouched:** the cold-path timing behind `02-DECISION-probe-budget.md`. It is the cause of the branch this plan improved the *consequence* of, and it is not less urgent for that.

---
*Phase: 02-transport-seam-talossim-image-factory*
*Completed: 2026-08-30*

## Self-Check: PASSED

- All 8 modified files present on disk.
- All three task commits (`622048e`, `ed296eb`, `77c920f`) present in `git log --all`.
- `UnresolvedInstallerRow` present in `images.tsx`; `installer_error` present in `api.ts` and in the contract.
- Every task's `<acceptance_criteria>` re-run at close-out; the plan-level `<verification>` block is transcribed in the Verification table above.
