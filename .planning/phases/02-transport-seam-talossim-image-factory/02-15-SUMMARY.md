---
phase: 02-transport-seam-talossim-image-factory
plan: 15
subsystem: ui
tags: [react, tailwind, tailwind-merge, vitest, testing-library, image-factory]

requires:
  - phase: 02-transport-seam-talossim-image-factory
    provides: "plan 02-12's three-state usability badge and its pinned verdict sentences; plan 02-13's architecture qualifier"
provides:
  - "A schematic detail dialog whose width class survives the tailwind-merge pass instead of being silently outranked by the component default"
  - "A verdict that consults isProbed before it reads usable, so a record cannot claim a probe confirmation it never received"
  - "Regression tests for both zero forms of probed_at with usable:true, asserted in the saved list and in the detail dialog"
  - "A jsdom-level assertion of the dialog's merged class attribute, which is the half of the width claim jsdom can actually check"
affects: [02-17, 02-21]

actuals:
  tokens: 2844
  tasks: 2
  commits: 3

tech-stack:
  added: []
  patterns:
    - "A caller that must beat a component default carries the same tailwind-merge group AND the same variant prefix as the default"
    - "A three-state verdict tests for the absence of a verdict before it reads either verdict value"

key-files:
  created: []
  modified:
    - web/src/routes/images.tsx
    - web/src/routes/images.test.tsx

key-decisions:
  - "The width fix is a variant-prefixed class at the call site (sm:max-w-3xl), not a change to DialogContent — every other dialog in the app depends on sm:max-w-sm as its default."
  - "The dialog test asserts the merged class attribute rather than a pixel width, and is named for the merge, because jsdom lays nothing out; the pixel assertion is deferred to plan 02-17's browser runner."
  - "The two zero forms of probed_at get one test each rather than a parameterised pair, because the empty string and the 0001-01-01 timestamp arrive from different producers."
  - "The GREEN commit is fix(...) rather than feat(...): this is a defect correction, not new behaviour."

patterns-established:
  - "Merge-group discipline: a width/size class set by a caller over a shadcn-style component default must match the default's variant prefix or it is dead code."
  - "Verdict ordering: existence of the subject is tested before any reading of it."

requirements-completed: [FACT-02, FACT-03]

coverage:
  - id: D1
    description: "The schematic detail dialog is allowed to grow past 384px on a wide screen — the caller's width class survives the merge and the component's narrow default no longer reaches the element."
    requirement: FACT-03
    verification:
      - kind: unit
        ref: "web/src/routes/images.test.tsx#keeps the detail dialog's width class through the class merge"
        status: pass
      - kind: other
        ref: "node -e twMerge(base + caller) against tailwind-merge 3.6.0 — reproduces the defect and the fix at the merge level"
        status: pass
    human_judgment: true
    rationale: "The merge outcome is proven; the resulting rendered width is not. jsdom computes no layout, so no assertion in this plan measures a pixel. The width stated below (768px at >=640px) is derived from Tailwind tokens, not measured. Plan 02-17 adds the browser runner that measures it."
  - id: D2
    description: "A record with usable:true and an empty probed_at renders the no-verdict sentence in the saved list and in the detail dialog."
    requirement: FACT-02
    verification:
      - kind: unit
        ref: "web/src/routes/images.test.tsx#refuses a usable claim from a record with an empty probe timestamp"
        status: pass
    human_judgment: false
  - id: D3
    description: "A record with usable:true and probed_at 0001-01-01T00:00:00Z renders the no-verdict sentence in the saved list and in the detail dialog."
    requirement: FACT-02
    verification:
      - kind: unit
        ref: "web/src/routes/images.test.tsx#refuses a usable claim from a record carrying the zero timestamp"
        status: pass
    human_judgment: false
  - id: D4
    description: "The three verdict sentences and the architecture qualifier are unchanged — only the order in which the branches are reached changed."
    requirement: FACT-02
    verification:
      - kind: unit
        ref: "web/src/routes/images.test.tsx#renders three distinguishable usability states and the reason for a refusal"
        status: pass
      - kind: other
        ref: "md5 of the removed and added 'Usable — the build probe confirmed it' diff lines is identical (626f9c99f9230fc48716ec61b6e764c2) — pure relocation"
        status: pass
    human_judgment: false

duration: 10 min
completed: 2026-08-30
status: complete
---

# Phase 02 Plan 15: Two Corrections in the Images Route Summary

**The detail dialog's width class now outranks the component default instead of being silently discarded by tailwind-merge, and `UsabilityVerdict` asks whether a record holds a verdict before it reads what the verdict says.**

## Performance

- **Duration:** 10 min
- **Started:** 2026-08-30T08:55:00Z (approx., agent start)
- **Completed:** 2026-08-30T07:05:00Z UTC (09:05 local)
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments

- **G-02-19 closed.** `web/src/routes/images.tsx` sets `sm:max-w-3xl` on the detail `DialogContent`. The unprefixed `max-w-3xl` is gone rather than left beside its replacement.
- **G-02-14 closed.** `UsabilityVerdict` tests `!isProbed(probedAt)` first. A record carrying `usable: true` with a zero `probed_at` now renders "Not verified — the build probe has no verdict" in both render sites.
- **Two regression tests per gap-shaped failure mode**, each verified to go red when its fix is reverted, restored, and re-run green.
- **`web/src/components/ui/dialog.tsx` untouched**, as were `internal/`, `docs/api-contract.md` and `.planning/WINDOWS.md`.

## The dialog width, and what the evidence is worth

**Downstream note for plan 02-17 — read this before measuring anything.**

The dialog's width rule is now `sm:max-w-3xl`. Tailwind v4's stock tokens (unmodified in this
app — `web/src/index.css` overrides neither) give:

| Token | Value | Pixels at a 16px root |
|---|---|---|
| `--breakpoint-sm` | `40rem` | 640px |
| `--container-3xl` (`max-w-3xl`) | `48rem` | **768px** |
| `--container-sm` (`max-w-sm`) | `24rem` | 384px |

So the expected width is **768px at any viewport ≥ 640px**, up from the 384px UAT measured at
1200px. Below 640px the component's own `max-w-[calc(100%-2rem)]` governs — and note that it
*also* did not govern before, because the unprefixed `max-w-3xl` was removing it from the merged
output entirely.

**How that number was established, precisely:** it was **not measured**. Nothing in this plan
lays anything out. Two things were established, and neither is a pixel:

1. **The merge outcome**, reproduced directly against the installed `tailwind-merge@3.6.0` with
   the real base string from `DialogContent`:
   - before — `... w-full sm:max-w-sm max-h-[85vh] max-w-3xl overflow-y-auto` (both survive,
     `sm:` wins the cascade, and `max-w-[calc(100%-2rem)]` was dropped)
   - after — `... w-full max-w-[calc(100%-2rem)] max-h-[85vh] sm:max-w-3xl overflow-y-auto`
2. **The class attribute on the rendered element**, asserted in jsdom: `sm:max-w-3xl` present,
   `sm:max-w-sm` absent, unprefixed `max-w-3xl` absent, `max-w-[calc(100%-2rem)]` present.

The 768px figure is therefore a **derivation from tokens, not an observation**. The one piece of
corroboration worth having: the same derivation predicts 384px for `sm:max-w-sm`, which is exactly
what the UAT measured in a real browser at a 1200px viewport — so the token→pixel mapping does hold
in this app. That makes 768px a well-founded expectation, not a verified fact.

02-17 should assert the measured width is **> 384px** (the regression guard) and expect **768px**
(the derivation), and should treat a disagreement between the two as a finding rather than as a
test to loosen.

## Task Commits

1. **Task 1: the detail dialog stops being clamped at 384px** — `be48336` (fix)
2. **Task 2 (RED): a usable claim with no probe timestamp reproduces G-02-14** — `b4f58d8` (test)
3. **Task 2 (GREEN): ask whether there is a verdict before asserting what it is** — `a8fa98d` (fix)

No REFACTOR commit: the GREEN change is a three-line relocation with nothing left to clean up.

## Files Created/Modified

- `web/src/routes/images.tsx` — width class prefixed to `sm:` at the detail dialog call site with a comment naming the merge-group trap; `UsabilityVerdict`'s `!isProbed` branch moved above the `usable` branch with the ordering argued rather than left incidental; a note on the exported `isProbed` recording that the badge now consults it on every branch.
- `web/src/routes/images.test.tsx` — two fixtures (`CLAIMED_UNPROBED_EMPTY`, `CLAIMED_UNPROBED_ZERO_TIME`) and three tests: the merge assertion and one per zero form, each covering the saved list and the detail dialog.

## Decisions Made

- **The width fix lives at the call site, not in `DialogContent`.** `sm:max-w-sm` is the default every
  other dialog in the app relies on. Widening the component to serve one caller would have moved the
  defect rather than fixed it.
- **The dialog test is named for the merge, not for the width.** A test called "the dialog is wide"
  that only reads a class attribute promises something jsdom cannot deliver, and the next reader
  would trust it further than it deserves. The name states what it checks; the comment states where
  the other half lives.
- **Two tests for the two zero forms, not one parameterised test.** `''` comes from a hand-written or
  partially-migrated JSON record; `0001-01-01T00:00:00Z` comes from a decoded zero `time.Time`. A
  single case covering one while claiming both is how `isProbed` would quietly lose half its job.
- **The GREEN commit is `fix`, not `feat`.** See TDD Gate Compliance below.

## Deviations from Plan

None — plan executed exactly as written.

**Total deviations:** 0
**Impact on plan:** None. Both prohibitions on scope (no copy changes, no fourth state, no
`dialog.tsx` edit, no `internal/`, no pixel measurement) held; `git diff --stat` over the plan's
commits names only `web/src/routes/images.tsx` and `web/src/routes/images.test.tsx`.

## TDD Gate Compliance

Task 2 ran RED → GREEN with no REFACTOR. The gate sequence is present but the GREEN commit is
`fix(02-15): ...` rather than `feat(02-15): ...`, so a scanner looking literally for `feat(` will
not find it. This is deliberate: the change corrects a defect in behaviour that already shipped
(G-02-14), and `feat` would describe it falsely. The RED commit `b4f58d8` precedes the GREEN commit
`a8fa98d` in `git log`, and the reversion check below is the substantive evidence the gate exists to
produce.

## Verification

| Check | Result |
|---|---|
| `npm --prefix web run test` | 111 passed / 7 files, green |
| `npm --prefix web run typecheck` | exit 0 |
| `npm --prefix web run lint` | `Checked 47 files`, exit 0 |
| `grep -cE '[" ]max-w-3xl' web/src/routes/images.tsx` | `0` |
| `grep -c 'sm:max-w-3xl' web/src/routes/images.tsx` | `1` |
| `git diff --stat web/src/components/` | empty (dialog.tsx untouched) |
| Verdict sentences unchanged | md5 of the removed and added `Usable — ...` lines identical: `626f9c99f9230fc48716ec61b6e764c2` |
| No file deletions across the three commits | `git diff --diff-filter=D` empty |

**Reversion checks** (both required by the plan's acceptance criteria, both performed and restored):

- Reverting `sm:max-w-3xl` → `max-w-3xl` turned the merge test red with
  `expected [...] to include 'sm:max-w-3xl'`. Restored; suite green.
- Restoring the `if (usable)` test above the `isProbed` test turned **both** new verdict tests red
  on the affirmative sentence — the live reproduction of G-02-14. Restored; suite green.

## Issues Encountered

None.

## User Setup Required

None — no external service configuration required.

## Next Phase Readiness

- **Plan 02-17 is unblocked.** Its width-based reasoning about the hyphen-break defect G-02-10 will
  now be tuned against the intended layout rather than against a 384px clamp. Read
  "The dialog width, and what the evidence is worth" above before writing the measurement — the
  expected value is 768px at ≥640px, and it is derived rather than observed.
- **Nothing here touches the open probe-state question.** G-02-1, G-02-2 and G-02-9 remain with the
  open `02-DECISION-probe-budget.md`; no fourth state, field or re-probe path was added.
- **The server was not changed.** The G-02-14 fix is a client-side ordering guarantee, which is the
  point: the producers that can create such a record (migration, import, hand edit) are exactly the
  ones the POST handler does not constrain.

---
*Phase: 02-transport-seam-talossim-image-factory*
*Completed: 2026-08-30*

## Self-Check: PASSED

Both modified files and this SUMMARY exist on disk; all three task commits (`be48336`, `b4f58d8`,
`a8fa98d`) are present in `git log`. All plan-level `<verification>` commands were re-run at
close-out: test suite green (111/111), typecheck 0, lint 0, and `git diff --stat` over the plan's
commits names only the two files the plan declares.
