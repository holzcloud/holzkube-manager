---
phase: 02-transport-seam-talossim-image-factory
plan: 20
subsystem: api
tags: [go, react, typescript, unicode, yaml, image-factory, validation, bidi]

requires:
  - phase: 02-14
    provides: "the measured refusal set in `representable` and the `divergingClasses` / `refusedCodepoints` tables both layers read"
  - phase: 02-17
    provides: "`INSTALLER_REPOSITORY_NAMES_LITERAL`, the four installer names as source literals a Go test can read"
  - phase: 02-19
    provides: "the partial-assets answer this plan leaves untouched"
provides:
  - "`imagefactory.NotRepresentableReason`, the single exported statement of which scalars holzkube will carry, with the canonical writer and the HTTP validator as its two call sites"
  - "`schematicInput.refuseUnrepresentable`, routing name, cluster, kernel_args, extensions and meta through that one predicate and reporting every bad field at once"
  - "a decided, documented and tested contract for an unpaired surrogate and raw invalid UTF-8, checked on the request body's bytes before `encoding/json` can rewrite either to U+FFFD"
  - "`REFUSED_RANGES` in `web/src/routes/images.tsx`: the browser's refusal set declared as data and equal to the server's"
  - "`internal/imagefactory/guard_drift_test.go`, which fails in both directions when the two sets stop agreeing and pins 02-17's four installer names to `installerCandidates`"
  - "`StoredText`, the rendering contract that keeps a stored bidi control inside its own cell"
affects: [02-21, any phase adding a stored operator-authored field, any phase adding a form input for cluster]

actuals:
  tokens: 18600
  tasks: 3
  commits: 6

tech-stack:
  added: []
  patterns:
    - "One exported predicate, call sites that reference it rather than restate it (the shape `registryRefused` established), with a test driving both through one table"
    - "A raw-bytes check standing before the JSON decoder, for the one class of defect that is invisible after decoding"
    - "A Go test reading a TypeScript source file to pin two sides of one rule, comparing behaviour rather than two declarations, and failing in both directions"
    - "Stored operator text rendered inside a directional isolate rather than refused, when the differential proved the character round-trips"

key-files:
  created:
    - internal/imagefactory/guard_drift_test.go
  modified:
    - internal/imagefactory/schematicid.go
    - internal/imagefactory/schematicid_test.go
    - internal/httpapi/handlers/schematics.go
    - internal/httpapi/handlers/schematics_test.go
    - web/src/routes/images.tsx
    - web/src/routes/images.test.tsx
    - docs/api-contract.md

key-decisions:
  - "An unpaired surrogate escape and a raw invalid UTF-8 byte are REFUSED with a 400 naming the member of the body that carried them, never repaired. Checked on the raw bytes before the decoder runs, because after decoding the evidence is gone."
  - "`validate` checks every operator-supplied scalar the request carries, not only the two fields the gap named. That keeps `createProblem`'s NotRepresentableError branch as a backstop rather than the route's first answer, and it is what lets three bad fields be reported in one response."
  - "The browser's refusal set is declared as data (`REFUSED_RANGES`) so a Go test can read it. The set is exactly the server's — including the astral range, so an emoji is now refused at the row instead of at the round trip."
  - "U+202E is deliberately NOT refused on either side: 02-14 measured it round-tripping through the Factory. holzkube therefore carries it and renders it safely, inside a `<bdi>`, instead of blocking a value the API accepts."
  - "The bidi rendering contract is applied to every stored-string render site uniformly, and stated in a comment at the component rather than only here."

patterns-established:
  - "Refusal reasons name the character class and the field, never the value, on every layer that produces one (T-02-64)"
  - "A drift guard that cannot find what it guards fails rather than skips"
  - "A codepoint table has one definition and every layer that asserts against it reads that one"

requirements-completed: [FACT-01, FACT-02, FACT-06]

coverage:
  - id: D1
    description: "`name` and `cluster` pass through the same refusal predicate as their already-guarded siblings; a POST carrying a refused codepoint in either is a 400 naming that field, stores nothing, and makes no upstream request"
    requirement: FACT-02
    verification:
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestCreateRefusesARefusedCodepointInTheName"
        status: pass
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestCreateRefusesARefusedCodepointInTheCluster"
        status: pass
      - kind: other
        ref: "reverting `in.refuseUnrepresentable(&errs)` turns 10 of those subtests red; restoring it turns them green"
        status: pass
    human_judgment: false
  - id: D2
    description: "One predicate states the rule; the canonical serialiser and the request validator both call it. `grep -c 'func representable'` reports 1 and the exported form is referenced from the handler."
    requirement: FACT-06
    verification:
      - kind: unit
        ref: "internal/imagefactory/schematicid_test.go#TestNotRepresentableReasonIsTheOneStatementOfTheRule"
        status: pass
      - kind: other
        ref: "grep -c 'func representable' internal/imagefactory/schematicid.go == 1; grep NotRepresentableReason internal/httpapi/handlers/schematics.go"
        status: pass
    human_judgment: false
  - id: D3
    description: "An unpaired surrogate submitted through the API has a decided, documented and tested contract: a 400 naming the field, never a 201 over a rewritten character. The same for a raw byte that is not valid UTF-8."
    requirement: FACT-02
    verification:
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestCreateRefusesAnUnpairedSurrogateInTheRawBody"
        status: pass
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestCreateRefusesRawInvalidUTF8InTheBody"
        status: pass
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestCreateReadsAWellFormedEscapeAsTheCharacterItEncodes"
        status: pass
    human_judgment: false
  - id: D4
    description: "G-02-11's own truth at the route: U+2028, a C1 codepoint and U+FEFF in `kernel_args` each answer a 400 naming `kernel_args` and its one-based entry index, and the body does not carry the unreachable-Factory sentence. Driven from the same declared table as the serialiser tests."
    requirement: FACT-02
    verification:
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestCreateRefusesAMeasuredDivergenceWithAFieldNamed400"
        status: pass
      - kind: unit
        ref: "internal/imagefactory/schematicid_test.go#TestSchematicIDRefusesEveryMeasuredDivergence"
        status: pass
    human_judgment: false
  - id: D5
    description: "The client's refusal set is the server's refusal set, and a drift guard fails when the two stop agreeing — in both directions"
    requirement: FACT-01
    verification:
      - kind: unit
        ref: "internal/imagefactory/guard_drift_test.go#TestBrowserRefusalSetEqualsTheServers"
        status: pass
      - kind: unit
        ref: "web/src/routes/images.test.tsx#refuses exactly the classes the server refuses"
        status: pass
      - kind: other
        ref: "adding a browser range the Go side lacks reports 1 over-refusal; removing one the Go side has reports 2 under-refusals; both restored"
        status: pass
    human_judgment: false
  - id: D6
    description: "The four installer repository names plan 02-17's browser sweep measures are pinned to `installerCandidates`, so a fifth Go candidate cannot leave that sweep measuring stale strings and still passing"
    requirement: FACT-01
    verification:
      - kind: unit
        ref: "internal/imagefactory/guard_drift_test.go#TestBrowserInstallerNamesEqualInstallerCandidates"
        status: pass
      - kind: other
        ref: "adding a fifth Go candidate fails the guard with 'declares 4 installer names, installerCandidates produces 6'; restored"
        status: pass
    human_judgment: false
  - id: D7
    description: "A stored value containing a bidi control cannot reorder the text around it in the saved table: every stored-string render site carries a directional isolate"
    requirement: FACT-02
    verification:
      - kind: unit
        ref: "web/src/routes/images.test.tsx#isolates every stored string a bidi override could otherwise reorder"
        status: pass
      - kind: other
        ref: "removing `<StoredText>` from the name cell fails the test with \"expected 'TD' to be 'BDI'\"; restored"
        status: pass
    human_judgment: false
  - id: D8
    description: "`docs/api-contract.md` names every field that can produce the refusal, including the two that could not before, and states the surrogate contract as a client-visible change"
    verification:
      - kind: other
        ref: "docs/api-contract.md lines 841-871 name `name` and `cluster` among the refusable fields; lines 875-900 state the surrogate contract"
        status: pass
    human_judgment: false
  - id: D9
    description: "The visual half of the bidi contract — that an isolated override actually reads as contained in a browser, and that the wider row is not rearranged"
    verification: []
    human_judgment: true
    rationale: "jsdom does not implement bidirectional layout, so the test proves the `<bdi>` element is present and cannot prove what it looks like. Whether the containment reads correctly, and whether a reversed name inside a tidy row is an acceptable thing to show an operator, needs a human looking at a real browser."

duration: 33 min
completed: 2026-08-30
status: complete
---

# Phase 02 Plan 20: Two Unguarded Fields, a Decided Surrogate, and a Guard That Keeps the Two Halves Equal Summary

**`name` and `cluster` now pass through the same exported refusal predicate as their already-guarded siblings, an unpaired surrogate is refused on the request body's raw bytes before `encoding/json` can rewrite it to U+FFFD, and the browser's refusal set is declared as data and pinned to the server's by a Go drift guard that fails in both directions.**

## Performance

- **Duration:** 33 min
- **Started:** 2026-08-30T09:07:00Z
- **Completed:** 2026-08-30T09:40:00Z
- **Tasks:** 3
- **Files modified:** 8 (1 created, 7 modified)

## Gap closure

**Plan 02-14 did not halt.** Its measurement succeeded — 116 DIVERGES rows above U+007F, 0 NOT OBSERVED, four refusal clauses each citing its rows — so the conditional branch in this plan's own acceptance criteria does not apply and both gaps close on their unconditional terms.

- **G-02-17 — closed on both `missing` bullets.** `name` and `cluster` go through the same guard as `kernel_args` and `meta`. The rendering contract for bidi controls is decided, applied to every stored-string render site uniformly, and tested.
- **G-02-11 — closed on both `missing` bullets.** The browser guard now covers exactly the classes 02-14 measured, and the unpaired-surrogate contract is decided, documented in `docs/api-contract.md` and tested from a raw request body rather than a marshalled struct.

## Accomplishments

- **One rule, two call sites, four more fields behind it.** `NotRepresentableReason` is the exported statement of which scalars holzkube will carry; the unexported `representable` is now a call rather than a copy, and `schematicInput.refuseUnrepresentable` is its second call site. `name`, `cluster`, `kernel_args`, `extensions` and `meta` all ask the same question, and each appends rather than returning, so a body with three bad fields answers with three field errors instead of the first one.
- **The lone surrogate is decided rather than emergent.** The request body is read whole and checked before decoding: an escaped unpaired surrogate, or a raw byte that is not valid UTF-8, answers `400` naming the member of the body that carried it. `json.RawMessage` gives that member's bytes without interpreting its escapes, which is what makes the field name available without decoding. A well-formed pair is judged as the astral character it encodes and never for its encoding.
- **The browser set is data, and equal to the server's.** `REFUSED_RANGES` replaces a four-comparison `if` nothing could inspect. The widening added the C1 range, both line separators, the byte order mark and everything above U+FFFD; the codepoints 02-14 measured as round-tripping — U+00A0, U+200B, U+202E, U+FDD0, U+FFFD — stay accepted, and there is a test per accepted codepoint as well as per refused class.
- **A drift guard that compares behaviour and fails both ways.** `TestBrowserRefusalSetEqualsTheServers` sweeps all 1.1M codepoints through `NotRepresentableReason` and through the ranges it parses out of `images.tsx`. Verified by hand in both directions before shipping: adding a browser-only range reports one over-refusal, removing a shared one reports two under-refusals.
- **02-17's residual closed in the same file.** `TestBrowserInstallerNamesEqualInstallerCandidates` binds `INSTALLER_REPOSITORY_NAMES_LITERAL` to `installerCandidates` across both SecureBoot states. A fifth Go candidate now fails that test instead of leaving the browser sweep measuring four stale strings.
- **A stored override cannot rearrange its row.** `StoredText` renders every stored operator string inside a `<bdi>`. The cell may look strange; the row cannot lie about which record it belongs to.

## Task Commits

1. **Task 1 (tracer, TDD): One predicate, four fields, and a decided answer for the lone surrogate**
   - `6832eb1` (test) — the RED half: refused `name`, refused `cluster`, all-fields-at-once, raw-body surrogate and invalid UTF-8, plus both over-refusal cases and the predicate's own binding to the writer
   - `1306579` (feat) — the GREEN half: `NotRepresentableReason`, `refuseUnrepresentable`, `readBody`/`rawBodyRefusal`/`rawTextReason`, `entryReason`, and the contract change in `docs/api-contract.md`
2. **Task 2 (TDD): The browser refuses exactly what the server refuses, and a guard says so**
   - `18e690d` (test) — `guard_drift_test.go`, failing because the browser declared no set a test could read
   - `691cc0a` (feat) — `REFUSED_RANGES`, the widened predicate, `name` in `hasUnusableValue`, and the web tests
3. **Task 3: A stored bidi control cannot reorder the table around it** — `ebf1d6c` (feat)

**Plan metadata:** see the `docs(02-20)` commit that follows this file.

## Files Created/Modified

- `internal/imagefactory/guard_drift_test.go` — **created.** `package imagefactory` (not `imagefactory_test`), because it reads the unexported `installerCandidates`; `canonical_live_test.go` in the same directory is the external test package on purpose. Holds both mirrors: the refusal-set sweep and the installer-name pin.
- `internal/imagefactory/schematicid.go` — `NotRepresentableReason` exported with the evidence-citing doc comment moved onto it and a paragraph saying why a request field and a document scalar share a rule; `representable` reduced to a one-line call.
- `internal/imagefactory/schematicid_test.go` — `TestNotRepresentableReasonIsTheOneStatementOfTheRule`, which asserts the predicate's reason and the writer's `NotRepresentableError.Reason` are the same string, plus the accepted-codepoint half.
- `internal/httpapi/handlers/schematics.go` — `refuseUnrepresentable` in `validate`; `readBody`, `rawBodyRefusal`, `rawTextReason`, `unpairedSurrogateReason` and `hexQuad` for the pre-decode check; `entryReason` extracted so a validate refusal and a serialiser refusal read identically; a paragraph on `createProblem` recording that its refusal branch is now a backstop.
- `internal/httpapi/handlers/schematics_test.go` — seven new tests and a `client.doRaw` that posts bytes without marshalling.
- `web/src/routes/images.tsx` — `REFUSED_RANGES`, the table-walking `hasControlCharacter`, the widened message, `nameError` folded into `hasUnusableValue`, and `StoredText` applied at four render sites.
- `web/src/routes/images.test.tsx` — the refusal set as two labelled tables, the `name` field guarded end to end, an accepted-name case, and the `OVERRIDE_STORED` fixture with the isolation test.
- `docs/api-contract.md` — `name` and `cluster` added to the refusable fields with a paragraph saying what changed for a client written before them; the surrogate contract as its own section; the never-repaired rule stated explicitly.

## Decisions Made

1. **Refuse the unpaired surrogate rather than accept the repair.** Two answers were defensible. Refusing is the one that matches every other decision on this route — report and refuse, never repair — and it is what T-02-67 asks for. Accepting would mean storing a schematic under a name its author would not recognise, with an id computed over a character they never sent.
2. **The check reads raw bytes and stands before the decoder.** There is no other place it can stand. After `json.Decode` the value is valid UTF-8 with nothing wrong in it, which is exactly why the hole survived a browser-side refusal and every test written after it.
3. **`validate` checks every operator-supplied scalar, not only `name` and `cluster`.** The gap named two fields; checking only those would have made "report every bad field at once" false for a body mixing a bad name with a bad kernel argument, because the second is raised inside `Author`. Checking all of them costs nothing (one predicate, one loop) and buys the property the plan asked for. `createProblem`'s branch stays as the backstop for document paths the request vocabulary does not enumerate and for in-process callers.
4. **The drift guard compares behaviour, not two declarations.** The Go side is swept codepoint by codepoint through the real predicate; only the browser side is parsed. A guard comparing two declarations would pass while the Go declaration and the Go behaviour disagreed.
5. **The surrogate range is asserted present in the browser table but excluded from the sweep.** Go cannot hold an unpaired surrogate in a string — `string(rune(0xD800))` is U+FFFD — so asking the predicate about one asks it about a different character. Its server-side twin is `rawBodyRefusal`, and the guard says so in the failure message.
6. **`<bdi>` rather than a CSS class.** It is `unicode-bidi: isolate` by definition, so the test can assert on the element rather than on a class name that a refactor could rename without changing behaviour.
7. **The bidi contract is a rendering decision, not a refusal.** U+202E round-trips through the Factory, so refusing it would block a value the API accepts — the exact over-refusal this plan's drift guard exists to prevent. Carrying it and isolating it is the answer.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing critical] `validate` extended past `name` and `cluster` to every operator-supplied scalar**

- **Found during:** Task 1
- **Issue:** The plan's behaviour row requires "a POST with bad values in `name`, `cluster` and `kernel_args` reports all three field errors in one response". `kernel_args` was only ever refused inside `Schematic.ID()`, downstream of `validate`, so a body with a bad `name` returned before the kernel argument was ever examined. Checking only the two named fields would have shipped that row false.
- **Fix:** `refuseUnrepresentable` walks `name`, `cluster`, `kernel_args`, `extensions` and `meta` through the one predicate. No rule is restated — it is the same function the serialiser calls.
- **Files modified:** `internal/httpapi/handlers/schematics.go`
- **Verification:** `TestCreateReportsEveryRefusedFieldAtOnce`; `TestCreateRefusesALocallyUnrenderableValueAsAnInputProblem` and `TestCreateRefusesAMeasuredDivergenceWithAFieldNamed400` still pass unchanged, which is the evidence the reason strings did not move.
- **Committed in:** `1306579`

**2. [Rule 3 - Blocker] `hexQuad` assembled digit by digit instead of through `strconv.ParseUint`**

- **Found during:** Task 1
- **Issue:** `golangci-lint` flagged `rune(v)` from a `uint64` as G115 (integer overflow conversion), and the surrogate-pair test as QF1001 (De Morgan). Both were introduced by this plan; the repo lints at 0 issues and had to stay there.
- **Fix:** hex digits accumulated into a `rune` directly, so the result is a rune by construction; the pairing test rewritten as a named `paired` boolean, which also fixed a latent slice-bounds hazard in `b[i+8:]`.
- **Files modified:** `internal/httpapi/handlers/schematics.go`
- **Verification:** `golangci-lint run` reports 0 issues.
- **Committed in:** `1306579`

**3. [Rule 1 - Bug] Literal invisible codepoints in Go test source**

- **Found during:** Task 1
- **Issue:** A literal U+FEFF in a Go string is a compile error ("illegal byte order mark"), and literal U+0085/U+2028/U+200B/U+202E trip `staticcheck` ST1018.
- **Fix:** every such codepoint written as a `\uXXXX` escape, which is also what the surrounding test files already do and what makes the source greppable.
- **Files modified:** `internal/httpapi/handlers/schematics_test.go`
- **Verification:** `gofmt -l` silent, `golangci-lint run` 0 issues.
- **Committed in:** `6832eb1` / `1306579`

**4. [Rule 3 - Blocker] The installer-name parser read `readonly string[]` as the array**

- **Found during:** Task 2
- **Issue:** The first `[`/`]` pair after the identifier is the type annotation's, not the array literal's, so the guard reported "carries no string literals" against a file that declares four.
- **Fix:** the extraction anchors on the assignment (`name … = [ … ]`) rather than on the first bracket.
- **Files modified:** `internal/imagefactory/guard_drift_test.go`
- **Verification:** the guard passes against the current file and fails when a fifth Go candidate is added.
- **Committed in:** `18e690d`

---

**Total deviations:** 4 auto-fixed (1 × Rule 1, 1 × Rule 2, 2 × Rule 3).
**Impact on plan:** None on scope. Deviation 1 widened one function past the two fields the gap named, in the direction the plan's own behaviour row required; the rest were mechanical.

## Issues Encountered

**The browser form has no `cluster` input.** The plan's Task 2 behaviour row asks for "`name` and `cluster` inputs guarded in the form". There is no cluster field on this form and the request it builds carries no cluster, so there is nothing to guard. `name` is guarded and feeds the one `hasUnusableValue` computation; a comment at that computation says where a cluster input belongs when one is added. The server guards `cluster` regardless, which is where the value can actually arrive today (an API client).

**`docs/api-contract.md` line references in D8 are approximate.** They were correct at the time of writing and will move with the next edit to that file; the greppable anchors are the phrases "The fields that can produce it are" and "An unpaired surrogate in the request body is a `400`".

## Ledger entries to file

For plan **02-21**, which owns `.planning/WINDOWS.md` this round. This plan did not touch it.

### Entries this plan addresses

- **Entry 29 — `fixed`.** *"A lone surrogate submitted through the API rather than the browser is still silently rewritten to U+FFFD by encoding/json before the schematic id is computed."* Closed exactly as the entry proposed: a raw-bytes check (`rawBodyRefusal`) standing before the decoder in `createSchematic`, answering `400` and naming the field, with `TestCreateRefusesAnUnpairedSurrogateInTheRawBody` posting a raw byte slice rather than a marshalled struct. Raw invalid UTF-8 — the same rewrite by a different route — is closed with it. The canonical serialiser was not widened, so the precomputed id FACT-06 rests on has not moved.
- **Entry 13 — `partially fixed`, recommend keeping open with an amended description.** *"`schematicInput.Cluster` is accepted, unvalidated and never sent."* The *unvalidated* half is closed: `cluster` now goes through `NotRepresentableReason` in `validate` and a refused codepoint in it is a `400` naming `cluster`. The *never sent* half is unchanged and still true — `cluster` is stored on the record and reaches no other layer, and the authoring form offers no input for it. Suggested amended text: "`schematicInput.Cluster` is validated and stored but never sent anywhere and has no form input."

### New residuals

- **`deviation` — records stored before this plan may carry refused codepoints in `name` or `cluster`.** Nothing migrates them; there is no backfill to write, because the values are the operator's own text and repairing them would be the silent rewrite this plan exists to prevent. Such a record is **readable but not re-creatable**, and there is no edit route — `POST`, `GET`, `GET /{id}`, `GET /{id}/assets` and `DELETE /{id}` are the whole surface — so an operator holding one **must delete and re-author it**. The saved table renders it safely in the meantime (`StoredText`), which is why this is a residual and not a defect. File against `internal/httpapi/handlers/schematics.go`.
- **`todo` — the client cannot guard `cluster` because the form has no cluster input.** The server does. When a cluster input is added it belongs in the single `hasUnusableValue` computation in `web/src/routes/images.tsx`, which already carries a comment saying so. File against `web/src/routes/images.tsx`.
- **`todo` — `createProblem`'s `NotRepresentableError` branch is now unreachable from the HTTP route in practice.** `refuseUnrepresentable` covers every document path the request vocabulary names, so the branch fires only for a future field that forgets a check, or for an in-process caller. It is kept deliberately (deleting it would turn the first of those into a `502` blaming the Factory, which is G-02-6) but it is a branch no route-level test can reach, which is the same shape as the `utf8.ValidString` note that produced entry 29. File against `internal/httpapi/handlers/schematics.go`.
- **`unrun-verify` — the codepoint sweep in `guard_drift_test.go` is exhaustive over Unicode but the *server* set behind it is not exhaustively measured.** It inherits 02-14's extrapolation: `U+FDD1`–`U+FDEF`, twenty-six of the C1 codepoints and the interior of the range above `U+FFFF` are refused on the strength of measured ends. The guard proves the two layers agree; it cannot prove the set is right. Extends 02-14's existing entry rather than starting a new claim.

### Closed from an earlier plan

- **02-14's coverage item D7 is closed by this plan.** `web/src/routes/images.tsx` no longer under-refuses relative to the server, and the doc comment above the guard no longer claims to transcribe a set it does not — the claim is now enforced by `TestBrowserRefusalSetEqualsTheServers` rather than asserted in prose. 02-14's ledger entry 2 (`stub — images.tsx now under-refuses`) can be marked fixed on that evidence.
- **02-17's residual 2 is closed by this plan.** `INSTALLER_REPOSITORY_NAMES_LITERAL` is bound to `installerCandidates` for both SecureBoot states, verified by adding a fifth Go candidate and watching the guard fail.

## User Setup Required

None — no external service configuration required. `HOLZKUBE_FACTORY_LIVE=1 go test ./internal/imagefactory/ -run TestLiveCanonical` remains opt-in and was not run by this plan; nothing in it depends on a live Factory.

## Next Phase Readiness

- **Plan 02-21 has everything it needs** to close entries 13 and 29 and to file the four new residuals, on evidence rather than on a guess. The `## Ledger entries to file` section above names the file, the status and the reason for each.
- **The seam future phases inherit:** a stored operator-authored string is refused at the input by one predicate and rendered inside `StoredText`. Any phase adding such a field does both, and `guard_drift_test.go` is where a new client-side rule gets pinned.
- **No blockers.** `go test ./... -count=1 -race` is green across every package, `npm --prefix web test` and `npm --prefix web run test:browser` are green across both projects, `golangci-lint run` reports 0 issues, `gofmt -l ./cmd ./internal` is silent, and `npm --prefix web run lint` / `typecheck` exit 0.

## Self-Check: PASSED

- `internal/imagefactory/guard_drift_test.go` exists on disk.
- All five task commits found in `git log`: `6832eb1`, `1306579`, `18e690d`, `691cc0a`, `ebf1d6c`.
- Every task's `<acceptance_criteria>` re-run after the last commit: Go suite green under `-race`, both web projects green, both lint gates at 0, `grep -c 'func representable'` == 1, `docs/api-contract.md` carries both additions.
- The plan-level `<verification>` commands all run and all pass; the falsifiability checks each criterion asked for (revert `validate`, add/remove a browser range, add a fifth Go installer candidate, remove one `StoredText`) were each performed and each restored, with `git diff` confirming no residue.

---
*Phase: 02-transport-seam-talossim-image-factory*
*Completed: 2026-08-30*
