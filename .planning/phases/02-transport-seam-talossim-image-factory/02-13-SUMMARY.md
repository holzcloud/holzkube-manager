---
phase: 02-transport-seam-talossim-image-factory
plan: 13
subsystem: api
tags: [go, react, zod, image-factory, schematics, provenance]

requires:
  - phase: 02-transport-seam-talossim-image-factory
    provides: "02-10 gave the asset panel its own architecture, so the panel stops rewriting what the next schematic is created and probed against — the first of G-02-8's two missing bullets"
  - phase: 02-transport-seam-talossim-image-factory
    provides: "02-12 rewrote the no-verdict badge sentence and pinned all three sentences with tests; this plan renders beside them rather than inside them"
provides:
  - "model.Schematic.Arch — the architecture a schematic was authored and probed against, on the record"
  - "createSchematic stamps in.Arch unconditionally, beside ProbedAt but not conditional on it"
  - "schematicSchema requires arch, so a server that stops sending it is a decode failure rather than a silently missing qualifier"
  - "UsabilityBadge renders the architecture beside the verdict at all three call sites; an empty architecture renders 02-12's sentences byte for byte"
  - "a test pinning FACT-03: the stored architecture defaults nothing, ?arch= is still required"
  - "docs/api-contract.md documents arch on the schematic resource and its explicit non-use as an assets default"
  - ".planning/WINDOWS.md entry 21 — the records that can never be qualified"
affects: [phase-06-jobs, phase-08-bare-metal, image-factory, schematics-ui]

actuals:
  tokens: 63500
  tasks: 2
  commits: 4

tech-stack:
  added: []
  patterns:
    - "A qualifier composes beside a verdict rather than being spliced into it, so a later rewrite of the verdict copy carries nothing extra"
    - "An additive, unversioned record field whose zero value is a true statement about older records — the ProbeReason precedent, reused"

key-files:
  created: []
  modified:
    - internal/model/model.go
    - internal/httpapi/handlers/schematics.go
    - internal/httpapi/handlers/schematics_test.go
    - web/src/api.ts
    - web/src/routes/images.tsx
    - web/src/routes/images.test.tsx
    - docs/api-contract.md
    - .planning/WINDOWS.md

key-decisions:
  - "Arch is stamped unconditionally, unlike ProbedAt: ProbedAt records whether an answer arrived, Arch records what the question was about"
  - "The architecture is rendered beside the verdict, never inside it — 02-12's three sentences are untouched and survive a later Option-1 rewrite"
  - "assetRequest deliberately does not read rec.Arch; ?arch= stays required and undefaulted (FACT-03), now pinned by a test rather than a comment"
  - "arch is required in the zod schema rather than optional, because the Go struct carries no omitempty and a missing field is a server regression"
  - "No store schema-version bump and no migration: an empty architecture is exactly what an older record means, and nothing on the record could seed a backfill"

patterns-established:
  - "Qualifier-beside-verdict: UsabilityBadge wraps an unchanged UsabilityVerdict and appends a sibling, so verdict copy and qualifier have separate owners"
  - "Distinguishable fixtures: the three usability fixtures no longer share an architecture, so a row assertion cannot pass against a hardcoded value"

requirements-completed: [FACT-02, FACT-03]

coverage:
  - id: D1
    description: "model.Schematic carries Arch (JSON arch) — the architecture the schematic was authored and probed against"
    requirement: FACT-02
    verification:
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestCreateStoresTheArchitectureTheProbeUsed"
        status: pass
    human_judgment: false
  - id: D2
    description: "POST /api/v1/schematics with arch=arm64 stores arm64; the 201 body and GET /api/v1/schematics/{id} both carry it"
    requirement: FACT-02
    verification:
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestCreateStoresTheArchitectureTheProbeUsed"
        status: pass
    human_judgment: false
  - id: D3
    description: "A create whose probe could not reach the Factory still stores the architecture, with probed_at zero — what the question was about is a fact whether or not it was answered"
    requirement: FACT-02
    verification:
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestUnansweredProbeStillStoresTheArchitecture"
        status: pass
    human_judgment: false
  - id: D4
    description: "The stored architecture defaults nothing: GET /api/v1/schematics/{id}/assets without ?arch= is still 400 with a field error naming arch"
    requirement: FACT-03
    verification:
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestTheStoredArchitectureDoesNotDefaultTheAssetsQuery"
        status: pass
      - kind: integration
        ref: "internal/httpapi/handlers/schematics_test.go#TestAssetsWithAnUnknownArchitectureIsAValidationProblem"
        status: pass
    human_judgment: false
  - id: D5
    description: "The detail dialog shows the architecture the verdict is about, beside the verdict"
    requirement: FACT-02
    verification:
      - kind: integration
        ref: "web/src/routes/images.test.tsx#names the architecture the verdict is about, beside the verdict"
        status: pass
    human_judgment: false
  - id: D6
    description: "A record written before the field existed renders exactly the badge plan 02-12 leaves, with no qualifier and no placeholder — in the detail dialog and in the saved list"
    requirement: FACT-02
    verification:
      - kind: integration
        ref: "web/src/routes/images.test.tsx#leaves a record written before the architecture existed unqualified"
        status: pass
      - kind: integration
        ref: "web/src/routes/images.test.tsx#renders three distinguishable usability states and the reason for a refusal"
        status: pass
    human_judgment: false
  - id: D7
    description: "The saved-schematics table shows each record's own architecture beside its verdict, with at least one arm64 among the fixtures so a hardcoded amd64 cannot pass"
    requirement: FACT-02
    verification:
      - kind: integration
        ref: "web/src/routes/images.test.tsx#renders three distinguishable usability states and the reason for a refusal"
        status: pass
    human_judgment: false
  - id: D8
    description: "The creation-result panel shows the architecture the new schematic was probed at"
    requirement: FACT-02
    verification:
      - kind: integration
        ref: "web/src/routes/images.test.tsx#names the architecture the new schematic was created at on the result panel"
        status: pass
    human_judgment: false
  - id: D9
    description: "docs/api-contract.md documents arch on the schematic resource, says what an empty value means, and states from both sides that it does not default the assets query"
    requirement: FACT-03
    verification:
      - kind: other
        ref: "grep -c '\"arch\"' docs/api-contract.md == 1; grep -n 'never defaulted|FACT-03' docs/api-contract.md"
        status: pass
    human_judgment: false
  - id: D10
    description: "The records that can never be qualified are an open .planning/WINDOWS.md entry rather than a silence"
    verification:
      - kind: other
        ref: "gsd-tools windows status — entry 21, kind deviation, phase 02, status open"
        status: pass
    human_judgment: false
  - id: D11
    description: "The saved list read by an operator with one amd64 and one arm64 schematic reads as intended, and a pre-field record reads as unqualified rather than as amd64"
    verification: []
    human_judgment: true
    rationale: "The plan's own <human-check>. jsdom proves the strings are present and attached to the right rows; it does not prove the qualifier is legible beside the badge at real widths, or that an operator reads an absent qualifier as 'unknown' rather than as an oversight. The /images screen has still never been opened in a browser (the standing D10 concern from plan 02-04)."

duration: 9 min
completed: 2026-08-29
status: complete
---

# Phase 02 Plan 13: The Probed Architecture on the Record Summary

**`model.Schematic.Arch` stamped from the value `Author` probed with, required by the zod schema, and rendered beside the Usability verdict at all three call sites — with an empty architecture left deliberately unqualified and filed on the broken-windows ledger.**

## Performance

- **Duration:** 9 min
- **Started:** 2026-08-29T17:44:20Z
- **Completed:** 2026-08-29T17:53:21Z
- **Tasks:** 2
- **Files modified:** 8

## Accomplishments

- `model.Schematic` gained `Arch string` (`json:"arch"`) beside `TalosVersion`, with a doc comment
  stating what it is, why it is on the record rather than a query parameter, and why it is additive
  and unversioned — including the clause that makes the last point load-bearing: the architecture a
  past probe used is not derivable from anything the record holds, so there is nothing for a
  migration to write.
- `createSchematic` stamps `Arch: in.Arch` **unconditionally**, three lines below the `ProbedAt`
  conditional that invites the symmetry. The comment beside the stamp names the distinction:
  `ProbedAt` records whether an answer arrived, `Arch` records what the question was about.
- `schematicOut` was checked and left alone, as the plan asked: it normalises nil collections, and a
  string has no nil form, so the record's own JSON shape carries the field to the 201 body, the list
  and the get with no further change.
- `assetRequest` is untouched. Its FACT-03 comment now says out loud that the record carries an
  architecture and that it is deliberately not read here — "using the description as the default is
  the FACT-03 bug wearing a record's clothes" — and a new test asserts a missing `?arch=` is still a
  400 naming the `arch` field.
- `UsabilityBadge` was split: an unchanged `UsabilityVerdict` holding 02-12's three sentences
  verbatim, and a wrapper that returns that verdict **as-is** when `arch` is absent or empty, or puts
  it in a `flex flex-wrap items-center gap-2` row beside `architecture: {arch}` when it is not. All
  three states get the qualifier, including the no-verdict one.
- Wired at all three call sites: the detail dialog (task 1, the tracer), the saved-schematics table
  row and the creation-result panel (task 2).
- `docs/api-contract.md` gained `"arch": "amd64"` in the resource example and a fourth bullet under
  `usable`/`probed_at`/`probe_reason`, plus one sentence on the assets `arch` paragraph saying the
  same thing from the other side.
- `.planning/WINDOWS.md` entry **21** (`kind: deviation`, phase 02, `internal/model/model.go`,
  status `open`) records the permanent residue: every schematic stored before this field existed
  carries an empty architecture forever, and those are precisely the records the G-02-8 leak
  produced.

## Task Commits

1. **Task 1 (tracer, tdd) — RED:** `f4d172b` (test) — three Go cases and two web cases, all failing
   for the right reason (`201 body arch = "", want "arm64"`).
2. **Task 1 — GREEN:** `193b05e` (feat) — model field, unconditional stamp, FACT-03 comment, zod
   field, badge split, detail dialog wired.
3. **Task 2 (tdd) — RED:** `3cac71c` (test) — the three usability fixtures given distinguishable
   architectures, a fourth `PRE_ARCH` fixture, and the creation-result case.
4. **Task 2 — GREEN:** `0872da9` (feat) — the two remaining call sites, the contract, the ledger.

No REFACTOR commit: the badge split was the minimal shape that satisfies "byte for byte unchanged
when `arch` is empty", so there was nothing left to clean up.

**Plan metadata:** see the `docs(02-13)` commit.

## Files Created/Modified

- `internal/model/model.go` — `Schematic.Arch`, the architecture the schematic was authored and probed against
- `internal/httpapi/handlers/schematics.go` — the unconditional stamp in the `rec` literal; `assetRequest`'s FACT-03 comment gains the explicit refusal
- `internal/httpapi/handlers/schematics_test.go` — three new tests: the round trip at arm64, the unanswered probe, and the assets query that is still 400
- `web/src/api.ts` — `arch: z.string()`, required, with the comment explaining what an empty value means
- `web/src/routes/images.tsx` — `UsabilityBadge` (qualifier) / `UsabilityVerdict` (02-12's sentences, unchanged); `arch` passed at all three call sites
- `web/src/routes/images.test.tsx` — fixture gains `arch`; the three usability fixtures no longer share one; `PRE_ARCH`; three new cases
- `docs/api-contract.md` — `arch` on the schematic resource, and its non-use as an assets default
- `.planning/WINDOWS.md` — entry 21

## Decisions Made

- **`Arch` is stamped unconditionally, unlike `ProbedAt`.** The symmetry with the conditional three
  lines above is tempting and wrong: a record whose probe never answered still has an architecture it
  *was* asked about, and withholding it would recreate G-02-8's ambiguity in miniature. Pinned by
  `TestUnansweredProbeStillStoresTheArchitecture`.
- **The architecture is rendered beside the verdict, never inside it.** Three sentences would
  otherwise become six; `ProbeReason` already names the architecture inside the refusal text, so the
  refused branch would say it twice; and `02-DECISION-probe-budget.md` Option 1 would rewrite those
  sentences again — a qualifier that composes with whatever they say survives that rewrite untouched.
  This is also why the plan ran after 02-12 rather than editing the same copy twice.
- **`assetRequest` deliberately does not read `rec.Arch`.** The record describes what was *probed*;
  the parameter asks what to *build*. FACT-03 is now held by a test
  (`TestTheStoredArchitectureDoesNotDefaultTheAssetsQuery`) rather than by a comment.
- **`arch` is required in the zod schema, not optional.** The Go struct carries no `omitempty`, so
  the field is always on the wire including as `""`; requiring it makes a server that stops sending
  it a decode failure instead of a silently missing qualifier. (T-02-72, accepted.)
- **The qualifier reads `architecture: {arch}`**, not "probed for {arch}". The plan offered the
  latter "or equivalent", but it asserts a probe in the branch whose whole point is that there is no
  verdict. `architecture: arm64` is true in all three states and contradicts none of 02-12's copy.

## Deviations from Plan

None — plan executed exactly as written.

Two things the plan asked to be *checked rather than assumed*, both confirmed and both resulting in
no change: `schematicOut` needed nothing (it normalises nil collections; a string has no nil form),
and `internal/store/fsstore` needed nothing (it marshals the record whole, so an added field needs no
store change and an older file decodes with the zero value).

One naming note that is a choice inside the plan's latitude rather than a deviation: the qualifier
wording, recorded under Decisions above.

## Issues Encountered

None. The prior blocker recorded in STATE.md — that `.planning/WINDOWS.md` was not appendable
because entries carried a `kind` outside gsd-tools' allowed set — is **resolved**: the ledger was
repaired in `e1a5c6c` and `windows append` / `windows status` both work. Entry 21 was appended
through the tool, never by hand.

## Verification

| Check | Result |
|---|---|
| `go test ./... -count=1 -race` | exit 0 — all 14 packages green |
| `npm --prefix web run test` | exit 0 — 106 tests, 7 files |
| `npm --prefix web run lint` (biome) | exit 0 — 47 files, no fixes applied |
| `npm --prefix web run typecheck` (`tsc --noEmit`) | exit 0 |
| `gofmt -l ./cmd ./internal` | prints nothing |
| `golangci-lint run` | **0 issues** — run with `PATH="$HOME/go/bin:$PATH"`; this closes the unrun-verify that two earlier plans recorded |
| `grep -c 'Arch string' internal/model/model.go` | 1 |
| `grep -c 'rec\.Arch' internal/httpapi/handlers/schematics.go` | 0 — `assetRequest` reads nothing from the record |
| `grep -c 'arch: z.string()' web/src/api.ts` | 1 |
| `grep -c 'arch={record\.arch}' web/src/routes/images.tsx` | 2 (detail dialog, saved list) |
| `grep -c 'arch={created\.arch}' web/src/routes/images.tsx` | 1 (creation result) |
| `grep -c '"arch"' docs/api-contract.md` | 1 |
| `gsd-tools windows status` | entry 21 present, `kind: deviation`, `status: open` (19 open, 21 total) |

The plan's `<human-check>` — opening the saved list with one amd64 and one arm64 schematic in a real
browser — is **not** discharged. It is recorded as coverage item D11 with `human_judgment: true`.

## Prohibitions Honoured

- **The stored architecture defaults nothing.** `assetRequest` is byte-unchanged apart from its
  comment; `grep -c 'rec\.Arch'` in `schematics.go` is 0; a test asserts the 400.
- **No probe state, no re-probe path.** `Usable`, `ProbedAt` and `ProbeReason` keep their exact
  meanings and their exact code. The 409 path is untouched. G-02-1, G-02-2 and G-02-9 stay with the
  open `02-DECISION-probe-budget.md`.
- **02-12's three badge sentences are not rewritten.** They were moved verbatim into
  `UsabilityVerdict`; 02-12's badge tests and the Go drift guard
  (`TestWarningDetailsMatchTheUI`) both still pass, and a record with an empty architecture renders
  them with nothing added.
- **No store schema version bump and no migration.** The field is additive and unversioned for the
  reason `ProbeReason` already states in `model.go`.

## User Setup Required

None — no external service configuration required.

## Next Phase Readiness

- **G-02-8 is closed on both bullets.** 02-10 gave the asset panel its own architecture; this plan
  puts the probed architecture on the record and shows it beside the verdict everywhere the verdict
  appears.
- This is the last plan of phase 02 (13 plans, 13 summaries). The phase is ready for
  `/gsd-verify-work 02`.
- **Carried forward:** `.planning/WINDOWS.md` entry 21 is open by design — every schematic stored
  before this field existed is unqualifiable forever, and those are the records the leak produced.
  The only remedy is deleting and re-authoring them. A future phase that adds a re-probe path
  (G-02-9's territory, still behind the open decision) could qualify them as a side effect; nothing
  short of that can.
- **Still open from earlier plans:** the `/images` screen has never been opened in a browser
  (D11 here, D10 in 02-04), and `02-DECISION-probe-budget.md` still owns G-02-1, G-02-2 and G-02-9.

---
*Phase: 02-transport-seam-talossim-image-factory*
*Completed: 2026-08-29*

## Self-Check: PASSED

All modified files present on disk; all five commits (`f4d172b`, `193b05e`, `3cac71c`, `0872da9`,
`b4126b7`) present in `git log --all`; working tree clean.
