---
phase: 02-transport-seam-talossim-image-factory
plan: 10
subsystem: ui
tags: [react, tanstack-query, radix, localstorage, rfc9457, image-factory]

# Dependency graph
requires:
  - phase: 02-transport-seam-talossim-image-factory
    provides: "The /images screen (plan 02-06): the creation form, the saved-schematics table, the detail dialog and the asset panel"
  - phase: 01-foundation-skeleton
    provides: "web/src/lib/problem.ts — ProblemError with a stable `code`, and the rule that the UI branches on the code and never on the status or the message text"
provides:
  - "AssetPanel owns its own architecture state, seeded from the remembered value and never writing back to it"
  - "useRememberedArch is the creation form's alone and the only writer of holzkube.images.arch"
  - "SchematicDetail has an isError branch that tells a deleted record apart from a failed fetch"
  - "A notfound.schematic detail failure invalidates ['schematics'] exactly once per failed id, so the stale row leaves the saved list"
  - "A regression test that switches the panel to arm64, closes the dialog and asserts both the form control and localStorage are unchanged"
affects: [02-12 (badge copy), 02-13 (Arch on model.Schematic, arch-qualified verdict)]

actuals:
  tokens: 16550
  tasks: 2
  commits: 4

tech-stack:
  added: []
  patterns:
    - "A persisted preference is bound to exactly one control; anywhere else it is passed down as a seed value with no setter"
    - "A dialog error branch keys its copy on the problem `code`, never on the message text or the status"
    - "Query invalidation on a failed fetch is `exact`, so the failing query is not in the effect's own blast radius, and ref-guarded to once per id"

key-files:
  created: []
  modified:
    - web/src/routes/images.tsx
    - web/src/routes/images.test.tsx

key-decisions:
  - "The asset panel's architecture is seeded from the remembered value rather than defaulted: reopening a dialog starts from the operator's build preference again, not from the last thing the panel happened to be asked"
  - "No reset effect for the panel's state — Radix unmounts dialog content on close, which was asserted by a reopen assertion in the regression test rather than assumed"
  - "The list invalidation uses `exact: true`, which the rest of this file does not: a prefix invalidation would also refetch this dialog's own ['schematics', id] query, putting the effect's own cause inside its dependencies"
  - "Only notfound.schematic invalidates the list. A 500 leaves the row, because the stored record is the only recoverable copy of an id the Image Factory will not enumerate"
  - "02-06-SUMMARY.md:60's description of the remembered architecture as 'the asset panel's architecture' is corrected here rather than edited there: it is now, accurately, the creation form's"

patterns-established:
  - "Seed-not-binding: a remembered preference reaches a nested read-only surface as a starting value with no setter, so an inspection cannot rewrite a preference"
  - "Two-reason error copy: a dialog that cannot show a record says which of the two reasons applies, and the branch is decided by the RFC 9457 code"

requirements-completed: [FACT-02, FACT-03]

coverage:
  - id: D1
    description: "Switching the asset panel to arm64 changes only the asset URLs; the creation form's Architecture select and holzkube.images.arch are both unchanged afterwards"
    requirement: "FACT-03"
    verification:
      - kind: unit
        ref: "web/src/routes/images.test.tsx#does not let the asset panel rewrite the remembered architecture"
        status: pass
    human_judgment: false
  - id: D2
    description: "Closing and reopening a schematic's dialog starts the panel from the remembered value again, not from the last panel choice"
    requirement: "FACT-03"
    verification:
      - kind: unit
        ref: "web/src/routes/images.test.tsx#does not let the asset panel rewrite the remembered architecture"
        status: pass
    human_judgment: false
  - id: D3
    description: "A schematic created after inspecting another schematic's arm64 URLs is posted with the architecture the creation form shows"
    requirement: "FACT-03"
    verification:
      - kind: unit
        ref: "web/src/routes/images.test.tsx#creates the next schematic for the architecture the form shows, not the one just inspected"
        status: pass
    human_judgment: false
  - id: D4
    description: "A detail fetch answering 404 notfound.schematic opens a dialog saying the schematic is no longer stored, with no delete control, no canonical document and no asset panel"
    requirement: "FACT-02"
    verification:
      - kind: unit
        ref: "web/src/routes/images.test.tsx#says a schematic is no longer stored instead of opening an empty dialog"
        status: pass
    human_judgment: false
  - id: D5
    description: "A notfound.schematic detail failure refetches the saved list, so the stale row leaves"
    requirement: "FACT-02"
    verification:
      - kind: unit
        ref: "web/src/routes/images.test.tsx#says a schematic is no longer stored instead of opening an empty dialog"
        status: pass
    human_judgment: false
  - id: D6
    description: "A non-404 failure renders a different sentence that does not claim the schematic was deleted, and leaves the row in the saved list"
    requirement: "FACT-02"
    verification:
      - kind: unit
        ref: "web/src/routes/images.test.tsx#distinguishes a failed fetch from a deleted schematic and keeps the row"
        status: pass
    human_judgment: false
  - id: D7
    description: "The two error dialogs read correctly to an operator in a real browser — that the deletion sentence is the one an operator who deleted a schematic in another tab would want, and that the failure sentence does not read as an accusation"
    verification: []
    human_judgment: true
    rationale: "The /images screen has still never been opened in a browser; all 95 web tests run in jsdom. This extends the standing D10 concern from plan 02-06 to the two new dialog branches. Copy adequacy is a judgment no assertion makes."

# Metrics
duration: 7 min
completed: 2026-08-29
status: complete
---

# Phase 02 Plan 10: Asset-Panel Architecture Split and the Detail-Dialog Error Branch Summary

**The asset panel now owns its architecture instead of rewriting the operator's build preference, and a schematic that cannot be shown produces a dialog that says which of two reasons applies instead of a modal whose entire text was the word "Close".**

## Performance

- **Duration:** 7 min
- **Started:** 2026-08-29T16:48:58Z
- **Completed:** 2026-08-29T16:55:29Z
- **Tasks:** 2 (both TDD, 2 commits each)
- **Files modified:** 2

## Accomplishments

- **The architecture leak is closed (G-02-8, first bullet).** `AssetPanel` holds `useState<Architecture>(archSeed)` beside the SecureBoot state it already owned. `SchematicDetail` and `SchematicDetailBody` no longer take an `onArchChange`; their `arch` prop is now `archSeed`, which says what it is. `useRememberedArch()` stays in `ImagesView` bound to the creation form alone, and is the only writer of `holzkube.images.arch`.
- **The regression that would have caught it exists.** Open a schematic, switch the panel to arm64, assert the ISO reference changed, close the dialog, then assert the creation form's Architecture select still reads amd64 and `localStorage` is still `amd64`. Closing first is load-bearing and commented: an open Radix modal marks the rest of the app `aria-hidden`, so the assertion would otherwise pass for the wrong reason.
- **Radix's unmount-on-close was asserted, not assumed.** The plan required either an assertion or a `key`-based reseed. The regression test reopens the dialog and asserts the panel is back at `metal-amd64`, which proves the state resets without a reset effect.
- **The detail dialog has an error branch (G-02-7).** `SchematicDetailUnavailable` renders inside `DialogHeader`/`DialogTitle`/`DialogDescription` so the modal keeps an accessible name, with the explanatory line marked `role="alert"` in this file's existing register. The branch is decided by `ProblemError.code`: `notfound.schematic` gets the sentence about the record being gone plus the note that the id is only recoverable from a stored record because the Factory will not list schematics back; anything else gets the sentence about the fetch having failed, which explicitly does not claim a deletion.
- **The stale row leaves, and only on the right failure.** A `notfound.schematic` detail failure invalidates `['schematics']` once per failed id, ref-guarded. A 500 does not, and a test asserts the fetch count is unchanged and the row is still there after closing the dialog.
- **The measured failure is gone.** G-02-7's evidence was `visibleTextLength: 5`, `fullText: 'Close'`, `hasAlert: false`. The replacement tests assert on the sentences, not on a length.

## Task Commits

Each task was executed TDD and committed atomically:

1. **Task 1 RED: the failing architecture-leak regression** — `c7457a3` (test)
2. **Task 1 GREEN: the asset panel owns its architecture** — `eb5baa3` (feat)
3. **Task 2 RED: the failing detail-dialog error-branch cases** — `c12f489` (test)
4. **Task 2 GREEN: the detail dialog says why it cannot show a schematic** — `30c09e4` (feat)

No REFACTOR commits: neither implementation had cleanup worth a separate commit.

## Files Created/Modified

- `web/src/routes/images.tsx` — `AssetPanel` gains its own `arch` state seeded from `archSeed`; `SchematicDetail`/`SchematicDetailBody` drop `onArchChange`; new `SchematicDetailUnavailable` component and the `record.isError` branch; the `notfound.schematic` list invalidation effect; `useRememberedArch`'s doc comment corrected to say it belongs to the creation form.
- `web/src/routes/images.test.tsx` — four new cases (two architecture, two error-branch) and a `listFetchCount` helper.

## Decisions Made

- **`exact: true` on the invalidation, which nothing else in this file uses.** TanStack Query matches query keys by prefix, so a bare `invalidateQueries({ queryKey: ['schematics'] })` would also refetch this dialog's own `['schematics', id]` query — putting the effect's own cause inside its dependency surface, which is precisely the loop T-02-50 is about. `exact: true` targets `SavedSchematics` and nothing else. The `useRef` guard bounds it to once per failed id regardless.
- **The seed is passed as `archSeed`, not `arch`.** Renaming the prop was the cheapest way to make the direction structural: a reviewer reading `archSeed` with no setter beside it cannot mistake it for a binding.
- **02-06-SUMMARY.md line 60 is corrected here, not there.** That summary describes the remembered architecture as "the asset panel's architecture", which is what made this read as an accidental leak rather than a decision. It is now, accurately, the creation form's — and the panel's is a per-inspection choice that is never persisted. The old summary is left as the record of what was actually shipped then.
- **The `fail` fixture's prefix match also catches the assets request.** Scoping `fail` to `/api/v1/schematics/` matches both the detail fetch and `/assets`. That is harmless because the error branch renders no asset panel, and a comment in the test says so, so the next reader does not file it as a fixture bug.

## Deviations from Plan

**None — plan executed exactly as written.**

One in-cycle test correction, which is not a deviation from the plan but is worth recording because it is the same trap the plan warned about for Task 1: the first draft of the 500 case asserted the saved row was still present *while the dialog was open*, and failed because Radix `aria-hidden`s the rest of the app. Fixed inside the same GREEN cycle by closing the dialog before the assertion, with a comment pointing at the architecture regression above it. Both assertions (fetch count unchanged, row present) survive.

## Boundaries Held

Both prohibitions in the plan's `must_haves` were honoured, and both were verified rather than asserted:

- **No architecture on the record.** `git diff --stat internal/model/model.go` is empty. `model.Schematic` gains no `Arch` field, no placeholder for one, and the Usability badge copy is byte-identical. That is plan 02-13's work, sequenced after 02-12 rewrites the verdict sentence an architecture would qualify.
- **No probe state, no badge claim.** Nothing here touches the probe, its budget or its state; `02-DECISION-probe-budget.md` is untouched and unaffected.
- **G-02-8 is deliberately not claimed.** This plan carries `gap_ids: [G-02-7]` only. G-02-8's second `missing` bullet — persisting the probed architecture and showing it beside the verdict — is 02-13's, which claims the gap id. The gap is closed by the pair, and `reconcile_gaps` resolves it once 02-13 has a SUMMARY. This is sequencing, not a deferral, and T-02-54 is transferred to 02-13 (as T-02-69) rather than accepted here.

## Issues Encountered

None.

## Verification

| Check | Result |
|---|---|
| `npm --prefix web run test` | 7 files, **95 tests passed** (was 91 before this plan) |
| `npm --prefix web run lint` | **0 findings** (one biome formatter finding on a long `DialogTitle` line, fixed with `npm run format` before the commit) |
| `npm --prefix web run typecheck` | clean |
| `go test ./... -count=1 -race` | all packages **ok** — this plan touches no Go file |
| `internal/model/model.go` | unchanged (`git diff --stat` empty) |
| `.planning/WINDOWS.md` | unchanged — this plan appends nothing, so gap-closure wave 1 shares no file |

Task-level acceptance criteria, all re-run at close:

- `grep -n '<SchematicDetail'` → `images.tsx:361`, passing `id`, `archSeed` and `onClose` only — **pass**
- `grep -c 'onArchChange'` → `0` — **pass**
- `grep -n 'useState'` → `images.tsx:847` `const [arch, setArch] = useState<Architecture>(archSeed)` beside `:848` SecureBoot — **pass**
- `grep -c 'record.isError'` → `1` — **pass**
- The three pre-existing architecture and SecureBoot tests pass unchanged. As the plan predicted, "remembers the architecture rather than defaulting to the developer machine" is now a statement about the panel's *starting* value rather than about shared state — its assertion and its intent both still hold.

## Broken-Windows Ledger

Nothing appended. This plan produced no stub, no skipped test, no unrun `<verify>` and no deviation. (The standing observation from plan 02-09 — that `.planning/WINDOWS.md` entries 8–18 carry a `kind` outside gsd-tools' allowed set — was resolved on `main` in `e1a5c6c` before this plan started; it did not need to be exercised here either way.)

## User Setup Required

None — no external service configuration required.

## Next Phase Readiness

- **Ready for the rest of gap-closure.** Wave 1 (02-09, 02-10) is complete and the two plans shared no file, as designed.
- **02-12 is unblocked** and owns the Usability badge copy on this screen; nothing here touched it.
- **02-13 is unblocked by this plan and blocked on 02-12.** It adds `Arch` to `model.Schematic`, renders the verdict as arch-qualified, claims `G-02-8` and carries the ledger entry that an earlier revision of this plan held. The surface it needs is untouched.
- **Standing concern, unchanged and now slightly wider:** the `/images` screen has never been opened in a browser. Two new dialog branches were added under jsdom-only coverage. Recorded as D7 with `human_judgment: true`.

---
*Phase: 02-transport-seam-talossim-image-factory*
*Completed: 2026-08-29*

## Self-Check: PASSED

- Modified files present on disk: `web/src/routes/images.tsx`, `web/src/routes/images.test.tsx`
- SUMMARY present: `.planning/phases/02-transport-seam-talossim-image-factory/02-10-SUMMARY.md`
- All five commits reachable: `c7457a3`, `eb5baa3`, `c12f489`, `30c09e4`, `99a1639`
- Plan-level `<verification>` re-run at close: web tests 95 passed, web lint 0 findings, `go test ./... -count=1 -race` all ok, `internal/model/model.go` and `.planning/WINDOWS.md` both unchanged
