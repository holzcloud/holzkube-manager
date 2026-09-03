---
phase: 02-transport-seam-talossim-image-factory
plan: 21
subsystem: api
tags: [image-factory, schematics, records, broken-windows-ledger, doc-contract, go, typescript]

requires:
  - phase: 02-transport-seam-talossim-image-factory
    provides: "plans 02-14 through 02-20 each wrote a `## Ledger entries to file` section instead of touching `.planning/WINDOWS.md`; this plan is the single writer that files them"
  - phase: 02-transport-seam-talossim-image-factory
    provides: "plan 02-13's `model.Schematic.Arch`, the field whose identity constraint G-02-20 is about"
provides:
  - "A recorded decision on whether a schematic record's identity carries the architecture (02-DECISION-schematic-identity.md): Option B is the direction, Option C is what this round wrote, implementation scoped out of phase 02"
  - "The identity constraint stated beside `model.Schematic.ID` and `.Arch` and beside the `409` in docs/api-contract.md: one stored customisation holds exactly one architecture's verdict"
  - "Three overbroad unrecoverability statements narrowed to the probe's outcome rather than the record's age, held up by a format test"
  - "WINDOWS entries 30-56: the entry plan 02-09 owed, and every deferral plans 02-14 through 02-20 handed over"
  - "02-VERIFICATION.md row 7 corrected in place with a dated note: a conjunctive criterion that had been marked verified on a disjunctive reading"
affects: [any phase that opens the schematic store, /gsd-ship, phase-02 verification]

actuals:
  tokens: 17214
  tasks: 4
  commits: 5

tech-stack:
  added: []
  patterns:
    - "A doc-comment claim that depends on a runtime string format is pinned by a test that names the comment it holds up"
    - "A ledger correction that the tool cannot express as an amend is filed as a superseding entry that names what it supersedes, leaving the original open"

key-files:
  created:
    - .planning/phases/02-transport-seam-talossim-image-factory/02-DECISION-schematic-identity.md
  modified:
    - internal/model/model.go
    - internal/httpapi/handlers/schematics_test.go
    - web/src/api.ts
    - docs/api-contract.md
    - .planning/WINDOWS.md
    - .planning/phases/02-transport-seam-talossim-image-factory/02-VERIFICATION.md

key-decisions:
  - "Schematic identity: Option B (one record, per-architecture verdicts) is the decided direction; Option C (state the constraint, change nothing) is what this records plan wrote. B's implementation is scoped to the phase that next opens the schematic store — most likely the one that adds a re-probe route, because 02-DECISION-probe-budget.md's 409-partial-update variant and B are the same edit seen from two sides."
  - "Option A (store key becomes `(schematic id, architecture)`) rejected: it buys architecture coexistence by paying with the record key, and the Factory will not enumerate schematics, so the record id is the one identifier nothing else in the system can reproduce."
  - "The unrecoverability claim is narrowed, not inverted. A refused record's architecture is recoverable by parsing prose written for an operator — weaker and more fragile than a field — and nothing does or should parse it."
  - "02-14's `task lint:go is unrun` handover was filed as the PATH trap, not as an absent tool. golangci-lint is at ~/go/bin/golangci-lint (2.13.1) and the repo lints clean at 0 issues; filing the entry as drafted would have re-filed a falsehood the round had already corrected."
  - "Ledger amendments (entries 13, 20, 21, 27) are filed as superseding entries rather than edits, because `gsd-tools windows` has status/append/waive/fixed and no amend verb, and hand-editing the file is prohibited."

patterns-established:
  - "Format-dependency test: when a doc comment in another language depends on a Go error string, the assertion is a regexp over the whole sentence and the comment names the test by name in both directions"
  - "In-place record correction: the original assessment is quoted verbatim inside the corrected row and the reasoning goes in a dated note beneath the table, so the history of the reading stays visible"

requirements-completed: [FACT-02, FACT-03, FACT-06]

coverage:
  - id: D1
    description: "The identity question — whether a schematic record's identity carries the architecture — is a recorded decision with its reasoning, its rejected alternative and its scoping, rather than an accident of the id being the Factory's hash"
    requirement: FACT-06
    verification:
      - kind: other
        ref: "test -f .planning/phases/02-transport-seam-talossim-image-factory/02-DECISION-schematic-identity.md"
        status: pass
    human_judgment: true
    rationale: "A decision document is judged on whether its reasoning is sound and its scoping is one a later phase can actually pick up. Existence is checkable; adequacy is not."
  - id: D2
    description: "The identity constraint is stated beside the two fields that create it (`model.Schematic.ID` and `.Arch`) and beside the `409` an operator actually meets in docs/api-contract.md, with no change to any struct field, JSON tag, store schema or migration"
    requirement: FACT-06
    verification:
      - kind: other
        ref: "git diff -U0 f96cf03..HEAD -- internal/model/model.go | grep non-comment lines → none"
        status: pass
      - kind: unit
        ref: "go test ./... -count=1 -race"
        status: pass
    human_judgment: false
  - id: D3
    description: "A refused probe's stored reason names the architecture it asked about, in a format a test pins — so narrowing the unrecoverability claim does not depend on a format nobody holds"
    requirement: FACT-02
    verification:
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestRefusalReasonNamesTheArchitectureItAskedAbout"
        status: pass
      - kind: other
        ref: "mutation check: dropping `/%s` for arch from probe.go's ErrSchematicNotBuildable message turns this test red and leaves every other test in the package green"
        status: pass
    human_judgment: false
  - id: D4
    description: "The three statements claiming a pre-02-13 record's architecture is unrecoverable are narrowed to the subset for which that is true — the records whose probe succeeded or never answered — in model.go, web/src/api.ts and WINDOWS entry 21"
    requirement: FACT-03
    verification:
      - kind: unit
        ref: "npm --prefix web run typecheck && npm --prefix web test (8 files, 126 tests)"
        status: pass
      - kind: other
        ref: "git diff -U0 f96cf03..HEAD -- web/src/api.ts | grep non-comment lines → none"
        status: pass
    human_judgment: true
    rationale: "Whether a narrowed prose claim reads as narrowed rather than as inverted is a judgment. The prohibition was explicit — do not restate this as 'the architecture is always recoverable' — and only a reader can confirm the second half lands."
  - id: D5
    description: "The `unrun-verify` entry plan 02-09's task-3 acceptance criterion required is in the ledger, naming the plan-09 throttle, the unprobed `talos.MaxSupportedVersion` and the unprobed v1.13.x below the pin"
    verification:
      - kind: other
        ref: "gsd-tools windows status — entry 30, kind unrun-verify, file internal/imagefactory/live_test.go, status open"
        status: pass
    human_judgment: false
  - id: D6
    description: "02-VERIFICATION.md's row for that criterion says what actually happened — conjunctive criterion, two of three conjuncts met — rather than marking it verified on a softened reading, with the original assessment corrected in place rather than deleted"
    verification: []
    human_judgment: true
    rationale: "The point of the correction is that the record shows the history of its own reading. Whether it does is a reading, not a check."
  - id: D7
    description: "Every `## Ledger entries to file` section written by plans 02-14 through 02-20 is filed (entries 31-56), and every entry those plans claimed closed is verified against the tree before being marked fixed (23, 29, 33, 38, 40, 43)"
    verification:
      - kind: other
        ref: "gsd-tools windows status — 47 open / 0 waived / 9 fixed / 56 total, agreeing across frontmatter, markdown table and JSON block"
        status: pass
    human_judgment: true
    rationale: "Completeness against six independently-written handover sections is a reading of six documents against one ledger. The counts reconcile mechanically; the mapping does not."

duration: 23 min
completed: 2026-08-30
status: complete
---

# Phase 02 Plan 21: Schematic Identity, Narrowed Claims and the Round's Ledger Summary

**One customisation holds exactly one architecture's verdict — recorded as a decision, stated beside the two fields that create it; the unrecoverability claim narrowed to the probe's outcome behind a format test; and 27 ledger entries filed for a round that had been writing them into SUMMARYs.**

## Performance

- **Duration:** 23 min
- **Started:** 2026-08-30T09:43:01Z
- **Completed:** 2026-08-30T10:06:07Z
- **Tasks:** 4 (1 decision checkpoint, 3 executed)
- **Files modified:** 6 (1 created, 5 modified)

## Accomplishments

- **G-02-20 closed.** `02-DECISION-schematic-identity.md` records the identity question with its three options, the answer, the reasoning and the scoping. The constraint an operator actually meets is now stated beside `model.Schematic.ID`, beside `.Arch`, and beside the `409` in `docs/api-contract.md`: the record id is the SHA-256 of a document with no architecture in it, so the two architectures of one customisation are one record, and probing the other means deleting this one and authoring it again.
- **G-02-22 closed.** `model.go`'s `Arch` comment and `web/src/api.ts`'s `arch` comment now condition on the probe's *outcome* rather than on the record's *age*, and WINDOWS entry 21's overbroad clause is superseded by entry 31. `TestRefusalReasonNamesTheArchitectureItAskedAbout` pins the sentence all three depend on — a doc comment in another language that rested on an unpinned Go format now rests on a red test.
- **G-02-21 closed.** WINDOWS entry 30 is the `unrun-verify` 02-09's task-3 criterion required and never got, and `02-VERIFICATION.md` row 7 says what actually happened instead of marking a conjunctive criterion verified on a disjunctive reading.
- **The round's ledger is filed.** Entries 31–56 carry every `## Ledger entries to file` item handed over by plans 02-14 through 02-20, keeping each plan's own wording. Six entries were marked fixed on first-hand evidence read from the tree; seven that plans asked to be kept open were kept open.

## Task Commits

1. **Task 1: the identity decision** — no commit of its own; resolved as a checkpoint and recorded in Task 2's artifacts (see *Decisions Made*).
2. **Task 2: record the identity decision where the next reader will meet it** — `30d2085` (docs)
3. **Task 3: narrow the three overbroad statements** (TDD) — `7476a80` (test, RED via mutation) → `89ea047` (docs, GREEN)
4. **Task 4: file the ledger and correct the row** — `162fe0f` (docs)

**Plan metadata:** see the final `docs(02-21)` commit.

## Files Created/Modified

- `.planning/phases/.../02-DECISION-schematic-identity.md` — **created.** The observation (verified live), the three options, the answer, the reasoning, what is scoped where and what would have to be true for B to be picked up.
- `internal/model/model.go` — **comment lines only.** A new doc comment on `ID` stating the identity constraint and its operational consequence; `Arch`'s comment gains the constraint cross-reference and the narrowed recoverability claim.
- `docs/api-contract.md` — a paragraph beside the existing `409` `store.conflict` section stating that a second architecture collides too, and why that is a recorded constraint rather than a defect.
- `internal/httpapi/handlers/schematics_test.go` — `TestRefusalReasonNamesTheArchitectureItAskedAbout` plus `probeRefusalShape`, the regexp that is the format assertion.
- `web/src/api.ts` — **comment lines only.** The `arch` field's doc comment split into the refused case and the succeeded/never-answered case.
- `.planning/WINDOWS.md` — **tool-written only.** 27 appends, 6 `fixed`.
- `.planning/phases/.../02-VERIFICATION.md` — row 7 corrected in place with a dated note beneath the table.

## Decisions Made

### Task 1 — the identity checkpoint, and how it was resolved

The plan's task 1 is `type="checkpoint:decision" gate="blocking"`. GSD auto mode is **not** active
in this project (`workflow.auto_advance: false`, `workflow._auto_chain_active: false`), so the
default executor behaviour would be to stop and return the checkpoint. It was resolved without
stopping, and the reasoning should be visible rather than assumed:

- The **dispatching orchestrator pre-answered it** in the executor's instructions: *"G-02-20 is a
  DECISION to record, not a schema change to implement… Record the identity decision; the plan
  explicitly says a schema change is out of scope for a records plan."*
- The **plan's own `<recommendation>`** says the same thing independently: *"Option B as the target,
  Option C as this round's action."*
- The project runs `mode: yolo`.

The answer taken is therefore the recommendation as written, not "the first option" — auto-selecting
option-a would have contradicted both the plan and the dispatcher. **This is recorded as a
non-standard checkpoint resolution rather than as an approval**, because the two are not the same
thing and the difference is exactly the kind of reread G-02-21 exists to punish.

### The decision itself

- **Option B is the decided direction; Option C is what this round wrote; B's implementation is
  scoped out of phase 02.** B leaves the record id alone, which is the property worth protecting —
  the Factory will not enumerate schematics, so the stored record is the only place the id can be
  read back from. It also models the truth: `usable`, `probed_at` and `probe_reason` were always
  per-architecture facts stored as properties of the schematic, which is what G-02-8 found and what
  `Arch` was added to patch over.
- **Option A rejected.** It makes two architectures coexist by making the store key
  `(schematic id, architecture)` — paying with the one identifier nothing else in the system can
  reproduce.
- **B's natural home is the phase that next opens the schematic store**, most likely the one adding
  a re-probe route: `02-DECISION-probe-budget.md`'s Option-2 variant already proposes writing
  `Usable`/`ProbedAt`/`ProbeReason` on a conflicting POST, and the moment a `409` is allowed to
  update a verdict, "under which architecture" has to be answered. That answer is B.

### The narrowed claim

- **Narrowed, not inverted.** All three statements now say the architecture is readable out of a
  *refused* record's `probe_reason` and nowhere else — and say in the same breath that recovering it
  means parsing prose written for an operator, that this is weaker and more fragile than a field,
  and that nothing does or should. The correction is to the claim, not to the behaviour.

### The superseded 02-14 entry

- 02-14 handed over an `unrun-verify` for `task lint:go` on the grounds that *"golangci-lint is not
  installed on this host"*. **That is false.** It is installed at `~/go/bin/golangci-lint` (2.13.1,
  the version CI pins) and merely off the default `PATH`; with that path exported the repo lints
  clean at **0 issues**. Entry 34 therefore records **the PATH trap** — a tool read as absent
  because a bare command name failed, which has now produced two entries (6 and 19, both fixed) and
  one SUMMARY claiming a permanent tooling gap — and not an absent tool. Filing the handover as
  drafted would have re-filed a falsehood the round had already corrected.

## Ledger: what was filed, and to whom it belongs

`.planning/WINDOWS.md` was mutated **only** through `gsd-tools windows append` / `fixed`. No
hand-edit: not the table, not the JSON block, not the frontmatter. Final counts reconcile across all
three representations — **47 open / 0 waived / 9 fixed / 56 total**.

### The entry this plan owed on its own account

| id | kind | file | Handed over by |
|---|---|---|---|
| 30 | `unrun-verify` | `internal/imagefactory/live_test.go` | **02-09's task-3 acceptance criterion** — the entry G-02-21 is about. Names the plan-09 throttle, `talos.MaxSupportedVersion` (v1.14, a range bound and not a concrete tag, so unprobeable as one), no v1.13.x below the pin ever probed, and the fake's v1.9.0 SecureBoot cell still an assumption. |
| 31 | `deviation` | `internal/model/model.go` | **This plan (G-02-22).** Supersedes entry 21's recoverability clause; entry 21 stays open for the window itself. |

### Filed from the round's handovers

| id | kind | file | Handed over by |
|---|---|---|---|
| 32 | `unmet-truth` | `docs/api-contract.md` | 02-14 item 1 — the refusal set is a floor; six regions refused by extrapolation |
| 33 | `stub` | `web/src/routes/images.tsx` | 02-14 item 2 / coverage item D7 — images.tsx under-refuses. **Marked fixed** |
| 34 | `unmet-truth` | `Taskfile.yml` | 02-14 item 3 **(superseded as written)** + 02-16 item 2 — the PATH trap |
| 35 | `unrun-verify` | `internal/imagefactory/canonical_live_test.go` | 02-14 item 4 — `TestLiveCanonical` opt-in, 16 POSTs, nothing in CI runs it |
| 36 | `skipped-test` | `internal/talossim/scenario_test.go:311` | 02-14 item 5 — `TestScenarioGoSilent` flaked once under `-race` |
| 37 | `deviation` | `internal/imagefactory/canonical.go` | 02-14 item 6 — pre-02-14 records not re-validated against newly refused codepoints |
| 38 | `lint-warning` | `internal/imagefactory/canonical_live_test.go:627` | 02-16 — QF1012. **Marked fixed** |
| 39 | `deviation` | `internal/imagefactory/warnings.go` | 02-16 — edited outside its declared `files_modified` |
| 40 | `todo` | `web/src/api.ts` | 02-16 — new warning code had no TS mirror / contract row. **Marked fixed** |
| 41 | `unmet-truth` | `internal/imagefactory/live_test.go` | 02-16 — the version range itself, and the self-updating third row |
| 42 | `unmet-truth` | `web/src/routes/images.browser.test.tsx` | 02-17 residual 1 — one engine measured (Chromium, playwright 1.62.1, headless, 1200×900) |
| 43 | `todo` | `internal/imagefactory/guard_drift_test.go:138` | 02-17 residual 2 — `INSTALLER_REPOSITORY_NAMES` unpinned. **Marked fixed** |
| 44 | `unrun-verify` | `.github/workflows/ci.yml:71` | 02-17 residual 3 — the CI browser-install step has never run |
| 45 | `deviation` | `internal/httpapi/problem.go` | 02-18 item 1 — problem `type` re-rooted to `urn:` (T-02-90) |
| 46 | `deviation` | `internal/httpapi/middleware/audit_test.go:126` | 02-18 item 2 — one fixture spells the base literally |
| 47 | `deviation` | `docs/api-contract.md` | 02-18 item 3 — the URN namespace is unregistered with IANA |
| 48 | `deviation` | `internal/imagefactory/installer.go` | 02-19 — **read with entry 20, which stays open**: 02-19 changed what a timeout returns, not the cold 2×30s walk |
| 49 | `deviation` | `docs/api-contract.md` | 02-19 — assets route shape change (502-no-body → 200 with `installer: null`); T-02-91/92 |
| 50 | `todo` | `docs/api-contract.md:646` | 02-19 — entries 25/26 deliberately left alone; worth re-offering to the user |
| 51 | `todo` | `web/src/api.ts:344` | 02-19 — **amends entry 27, which stays open**: a second constant is now also unused |
| 52 | `todo` | `internal/httpapi/handlers/schematics.go:66` | 02-20 — **amends entry 13, which stays open**: the *unvalidated* half is closed, the *never sent* half is not |
| 53 | `deviation` | `internal/httpapi/handlers/schematics.go` | 02-20 — pre-02-20 records may carry refused codepoints in `name`/`cluster`; readable but not re-creatable |
| 54 | `todo` | `web/src/routes/images.tsx:329` | 02-20 — the client cannot guard `cluster` because the form has no cluster input |
| 55 | `todo` | `internal/httpapi/handlers/schematics.go` | 02-20 — `createProblem`'s `NotRepresentableError` branch is unreachable from the route |
| 56 | `unrun-verify` | `internal/imagefactory/guard_drift_test.go:65` | 02-20 — the sweep is exhaustive over Unicode; the server set behind it is not |

### Marked fixed — one line of evidence each, read from the tree

| id | Claim | Evidence read first-hand |
|---|---|---|
| 23 | [R2-WR-06] the provisional-warning branch of `GET /assets` has no handler-level test | `TestAssetsProvenAndProvisionalAnswersAreUnchanged` exists at `internal/httpapi/handlers/schematics_test.go:1552`, and the whole package passes under `-race`. |
| 29 | [R2-RESIDUAL] a lone surrogate through the API is silently rewritten to U+FFFD before the id is computed | `rawBodyRefusal(raw)` stands before the decoder at `internal/httpapi/handlers/schematics.go:271`, with `TestCreateRefusesAnUnpairedSurrogateInTheRawBody` at `schematics_test.go:2000`. |
| 33 | 02-14 D7 — `images.tsx` under-refuses relative to the server | `TestBrowserRefusalSetEqualsTheServers` at `internal/imagefactory/guard_drift_test.go:65` enforces set equality across the two layers; the prose claim is now a test. |
| 38 | 02-16 — QF1012 at `canonical_live_test.go:627` keeps `golangci-lint run` off zero | `PATH=~/go/bin:$PATH golangci-lint run` → **0 issues**, golangci-lint 2.13.1. |
| 40 | 02-16 — the SecureBoot fallback warning has no TS mirror and no contract row | `WARNING_INSTALLER_SECUREBOOT_REPO_FALLBACK_UNVERIFIED` at `web/src/api.ts:344`; the contract's warning table carries the row at `docs/api-contract.md:639`. |
| 43 | 02-17 residual 2 — `INSTALLER_REPOSITORY_NAMES` is not pinned to `installerCandidates` | `TestBrowserInstallerNamesEqualInstallerCandidates` at `guard_drift_test.go:138` reads `INSTALLER_REPOSITORY_NAMES_LITERAL` out of `images.browser.test.tsx:253` and compares it against the Go candidates. |

### Deliberately left open

**5** (the factory.talos.dev throttle — still open, and demonstrated a second time by the plan-09
run entry 30 records). **13** (amended by 52; the *never sent* half is still true). **20** (the cold
2×30s installer walk against `writeTimeout=60s` — 02-19 changed what a timeout *returns*, not how
often one happens; the 49.8s and 60.002s measurements stand, and a milder symptom makes deferring
`02-DECISION-probe-budget.md` easier rather than more defensible, which is entry 48's whole point).
**21** (superseded on the recoverability clause by 31; the window itself is unchanged).
**25, 26** (marked "scoped out by the user" — a scoping decision is not an executor's to overturn).
**27** (amended by 51; re-word rather than close). **34** (the PATH trap, open until the lint task
resolves the binary rather than depending on the caller's `PATH`).

## Deviations from Plan

### 1. [Rule 2 — Missing critical] The identity constraint was also written into `docs/api-contract.md`

- **Found during:** Task 2.
- **Issue:** Task 2's `<files>` and the plan's `files_modified` name only the decision document and
  `model.go`. But the option this plan adopted is Option C, and the plan's own `<recommendation>`
  defines C as *"the constraint stated in the model, **the contract** and the ledger"*. Leaving the
  contract out would have shipped two thirds of the chosen option — and the contract is where an
  operator meeting a `409` actually looks, since `docs/api-contract.md:600` already explains the
  `409` for a second name, cluster and version and simply stops short of the architecture.
- **Fix:** One paragraph beside the existing `409` section stating that a second architecture
  collides too, why (the canonical document has no architecture in it), and that this is a recorded
  constraint pointing at `02-DECISION-schematic-identity.md`.
- **Files modified:** `docs/api-contract.md`
- **Verification:** `npm --prefix web test` and `go test ./... -count=1 -race` green; the contract is
  prose and has no compile gate, so the check is that the paragraph agrees with `model.go`'s.
- **Committed in:** `30d2085`

### 2. [Rule 3 — Blocking] `gsd-tools windows` has no amend verb, so four amendments are superseding entries

- **Found during:** Task 4.
- **Issue:** Task 4's acceptance criteria require *"Entry 21's description is the narrowed wording
  drafted in task 3"*, and plans 02-19 and 02-20 likewise asked for entries 27 and 13 to be
  **re-worded rather than closed**. `gsd-tools windows` exposes exactly four subcommands —
  `status`, `append`, `waive`, `fixed` — and `markFixed(ledger, id)` takes no reason argument. There
  is no way to change an existing entry's `description`. The plan prohibits hand-editing
  `.planning/WINDOWS.md` in any of its three representations, and that prohibition is right: the
  markdown table, the JSON block and the frontmatter counts only stay in step because the tool keeps
  them there.
- **Fix:** Each amendment is a **new entry that names what it supersedes and why**, with the
  superseded entry left open — 31 supersedes entry 21's recoverability clause, 48 stands beside
  entry 20, 51 amends entry 27, 52 amends entry 13. Every one of them opens by saying so in capitals
  so a ship gate reading the register linearly cannot read the old text without meeting the new one.
- **Files modified:** `.planning/WINDOWS.md` (tool-written only)
- **Verification:** `gsd-tools windows status` → `ok: true`, counts agreeing across frontmatter,
  table and JSON block.
- **Committed in:** `162fe0f`
- **Note:** the acceptance criterion *"Entry 21's description is the narrowed wording"* is therefore
  **not literally met** and cannot be without violating a prohibition. It is met in substance by
  entry 31. This is logged rather than silently reinterpreted — reinterpreting an acceptance
  criterion to fit what happened is precisely the failure G-02-21 is about, and doing it inside the
  plan that closes G-02-21 would be a joke at the register's expense.

---

**Total deviations:** 2 (1 missing critical, 1 blocking).
**Impact on plan:** No scope creep. `git diff` for `internal/model/model.go` and `web/src/api.ts`
across the whole plan is comment lines only; no struct field, JSON tag, store schema or migration
changed, and the prohibition on implementing the identity decision here held.

## Issues Encountered

**1. The TDD RED phase for a format assertion has to be produced by mutation.** Task 3's test asserts
a format that already exists, so it passes on first run — there is no honest way to write it red
first. The plan anticipated this and specified the alternative, which was carried out: `/%s` for
arch was dropped from `probe.go`'s `ErrSchematicNotBuildable` message, the new test went red with
the diagnostic it was written to print (`probe_reason = "…7ab5… at v1.13.9 answered HTTP 400" does
not match ^[0-9a-f]{64} at (v…)/(amd64|arm64) answered HTTP [0-9]{3}$`), **every other test in the
package stayed green** — which is the finding, since it is what let the format move unnoticed — and
`probe.go` was restored byte-for-byte before the test was committed.

**2. ~~`npm --prefix web test` runs one vitest project, not two.~~ — WITHDRAWN, this was false.**

> **CORRECTION (verifier + orchestrator, 2026-08-30).** The claim below is wrong and is retained
> only so the correction has something to point at. `web/vite.config.ts:113` declares TWO named
> projects, `jsdom` (line 119) and `browser` (line 137), and `web/package.json` declares
> `test:browser`. Measured: `npm --prefix web test` runs 8 files / 126 tests including
> `images.browser.test.tsx`; `--project browser` alone runs 7. The plan's criterion "green across
> both projects" is therefore literally met, not unmeetable.
>
> This mattered enough to correct rather than leave: a reader acting on the original sentence would
> conclude the browser project is redundant configuration and delete it — and it is the only layout
> evidence G-02-10 and G-02-19 have. It is also, precisely, this round's own failure shape: a
> confident sentence about what the tooling does, written without running it.

~~The plan's `<verification>` says green across both projects. `web/` has one vitest project; the
browser-mode file (`images.browser.test.tsx`) is exercised through the same command. No second
project configuration exists in `web/` today.~~

## Known Stubs

None introduced. Everything this plan touched outside the new test is a comment or a record.

## Threat Flags

None. `T-02-99`, `T-02-100` and `T-02-101` are all `mitigate` and all mitigated as the register
specified: every round deferral is filed through the tool with closures verified against the tree
(T-02-99), the verification row is corrected in place with the original reading visible and cites
entry 30 (T-02-100), and the unrecoverability claim is scoped in all three places behind a test that
pins the format (T-02-101). `T-02-SC` holds: no Go module and no npm package was added.

## User Setup Required

None — no external service configuration required.

## Next Phase Readiness

- **The gap-closure round is complete.** G-02-20, G-02-21 and G-02-22 are closed, and the ledger a
  ship gate reads now matches what the round left behind.
- **`/gsd-ship` will block:** 47 entries are open. That is the register doing its job, not a
  regression — before this plan it was 26 open with the round's deferrals living in six SUMMARYs
  where no gate could see them. Triage before ship is a deliberate next step, and entries 25, 26 and
  50 are explicitly flagged as worth re-offering to the user.
- **Two decisions are now open and waiting on the phase that touches their surface:**
  `02-DECISION-probe-budget.md` (unchanged, and entry 48 argues it got more pressing rather than
  less) and `02-DECISION-schematic-identity.md` (B, scoped to the phase that next opens the
  schematic store). They meet at the same edit — a `409` allowed to update a probe verdict has to
  say which architecture it is about.

## Self-Check: PASSED

- `.planning/phases/02-transport-seam-talossim-image-factory/02-DECISION-schematic-identity.md` — FOUND
- `30d2085`, `7476a80`, `89ea047`, `162fe0f` — all present in `git log`
- `go test ./... -count=1 -race` — 15 packages, all `ok`
- `npm --prefix web run typecheck` — exit 0; `npm --prefix web test` — 8 files / 126 tests passed
- `PATH=~/go/bin:$PATH golangci-lint run` — 0 issues
- `gsd-tools windows status` — `ok: true`; 47 open / 0 waived / 9 fixed / 56 total, agreeing across
  frontmatter, markdown table and JSON block
- `git diff f96cf03..HEAD -- internal/model/model.go web/src/api.ts` — comment lines only

---
*Phase: 02-transport-seam-talossim-image-factory*
*Completed: 2026-08-30*
