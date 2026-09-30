---
phase: 14-telefon-tippziele
plan: 04
subsystem: testing
tags: [layout-audit, playwright, d-08, desktop-unchanged, measurement]
status: complete

requires:
  - "14-02: the 16 px pair gap in HostActions.tsx, landed before and part of this baseline"
  - "14-03: the nine openers and the first extended run (14-03-first-red.log)"
provides:
  - "layout-audit.mjs LAYOUT_DUMP: one JSON line per control at 1280 px on every route and in every opener, refused inside the repository"
  - "web/scripts/layout-dump-compare.mjs: the D-08 comparer (key route · opener · index · tag, boxes held equal)"
  - "the before-dump, outside the repository: $HOME/.cache/holzkube-manager-layout/14-before.jsonl (964 controls)"
affects: [14-05, 14-06, 14-07]

actuals:
  tokens: 3700
  tasks: 2
  commits: 1
plan_head_before: 445e5ab484ba4b9441e1e67f7aace2dc4934007d
plan_head_after: 27ace4b2979329ebabf608a649e575d3ad470382

tech-stack:
  added: []
  patterns:
    - "An environment switch that writes outside the repository resolves its path (symlinks too) and refuses the checkout before the daemon starts"
    - "One selector constant handed as an argument into the in-page function, so the size pass and the dump cannot drift apart"

key-files:
  created:
    - web/scripts/layout-dump-compare.mjs
  modified:
    - web/scripts/layout-audit.mjs

key-decisions:
  - "The dump comes out of findSmallTargets' own loop (a `dump` flag), with the same selector and skips, and records the element's own box, not its label's"
  - "The comparer also fails on a repeated key and on a line that is not a control, besides a moved box, a one-sided control and an empty dump"
  - "No determinism fix and no exclusion: the noise floor measured 0 of 964 without one"

requirements-completed: []
requirements-partial: [MOB-01]

duration: 11min
completed: 2026-09-30
---

# Phase 14 Plan 04: The D-08 instrument and the before-dump Summary

**The audit can now write the size and position of every control at 1280 px (`LAYOUT_DUMP`). `layout-dump-compare.mjs` judges two such dumps. Two dumps of the unchanged tree compared as `compared 964 controls, 0 differ`, so the instrument's noise floor is 0. The before-dump was taken while no tap-target file differed from fda4579. It lies outside the repository and is not committed.**

## Where it ran

On the operator's Pi (aarch64). Node came from nvm and Go from `~/.local/go`. No `-race` run, nothing installed, nothing pushed. `holzkube-manager.service` and `/var/lib/holzkube-manager` were not touched. Every audit run started its own daemon on 127.0.0.1 with a temporary data directory.

## Performance

- **Duration:** about 11 min (10:07 to 10:18 UTC)
- **Tasks:** 2. Task 1 has a commit. Task 2 changed no file in the repository, because the noise floor was already 0.
- **Files:** 1 created, 1 modified

## Task 1: the dump and the comparer (commit 27ace4b)

**Red first.** The six comparer cases were written as JSONL files in the session scratchpad before the script existed. Run against the missing script, the identical pair gave `exit=1` (want 0), with `Cannot find module`.

**Green.** Each exit code below was read from the command itself:

| Case | Exit | Output |
|------|------|--------|
| two identical dumps | 0 | `compared 3 controls, 0 differ` |
| one box moved by 1 px (x 900 to 901) | 1 | `MOVED /nodes/m-cp-1 · Power menu · #0 · <div>`, printed with both boxes and both names |
| a control present only in before | 1 | `ONLY BEFORE …` |
| a control present only in after | 1 | `ONLY AFTER …` |
| an empty dump (before, after, or both) | 1 | `… has no controls: a dump that measured nothing compares equal to anything` |

**Each guard seen red.** Each fault was put back in a copy of the comparer and run against the case it guards:

- `x` removed from the compared fields: the moved case gave **exit=0**.
- The empty check removed: empty against empty gave **exit=0**, `compared 0 controls, 0 differ`.
- The only-after loop removed: a new control gave **exit=0**.
- The repository refusal removed in the audit itself, run with `HOLZKUBE_BINARY=/nonexistent-binary` so it stops before measuring: `LAYOUT_DUMP=../fault-dump.jsonl` **created the file inside the checkout**. The file was then removed and the script restored; `cmp` against the saved copy gave 0.

**Refusal.** Each case exited 1 before any build or daemon started:

- a relative path inside the repository
- an absolute path inside the repository
- a symlink from outside that points into the checkout
- a directory that does not exist

**Acceptance.**

- `grep -c LAYOUT_DUMP web/scripts/layout-audit.mjs` gave 8.
- `git ls-files '*.jsonl'` printed nothing.
- `biome check` on both scripts exited 0.

**What the audit changed.**

- The control selector is now one constant, `CONTROL_SELECTOR`, passed into `findSmallTargets` as an argument.
- The dump comes out of that function's own loop, behind a `dump` flag, at the point where a control counts as measured. So it uses the same selector and the same skips as the size pass.
- Each line is `{route, opener, index, tag, name (aria-label or text, 40 chars), x, y, width, height}`. The box is the element's own `getBoundingClientRect`, rounded.
- Lines are written at 1280 px only:
  - for the route pass, scoped to `<body>` with an empty opener
  - for each opener, scoped to its `expect` element, after its animations finished and after the enabled check

## Task 2: noise floor and before-dump

1. **Before the first class change.** `git diff --stat fda4579 HEAD -- web/src/components/ui web/src/components/Sidebar.tsx web/src/components/WhatsNew.tsx web/src/components/NodeActions.tsx web/src/components/NodeSchedulingActions.tsx` printed nothing: 0 bytes, exit 0. 14-02's `HostActions.tsx` pair gap is already in the measured tree, on purpose.
2. **Two dumps of the same tree.**
   - `LAYOUT_DUMP=$HOME/.cache/holzkube-manager-layout/14-before.jsonl ./bin/task test:layout` gave exit=201 (the findings are still there).
   - The same with `14-noise.jsonl` also gave exit=201.
   - Both logs say `LAYOUT_DUMP: 964 controls at 1280px written to …` and `7 finding(s)`.
3. **Noise floor.** `node web/scripts/layout-dump-compare.mjs 14-before.jsonl 14-noise.jsonl` gave **exit=0, `compared 964 controls, 0 differ`**. A third dump, the Task 1 smoke run in the scratchpad, also compares 964/0 against the before-dump.
   - No determinism fix was needed and no control is excluded. The wall's refreshed `generated_at` and the relative times did not move a box. `/wall` has 0 controls, and names are never compared.
4. **The dump changed nothing that is reported.** The finding header lines of `14-04-before.log` (`SMALL`, `CLIPPED`, `SIDEWAYS`, `OFFSCREEN`, `CUT OFF`) equal those of `14-03-first-red.log`: 7 lines, `diff` exit 0. The only other indented lines that differ are vitest's start time and duration. The logs contain no OPENER, EMPTY or EXECUTED line.
5. **N = 964 controls** across 26 route scopes and 8 opener scopes at 1280 px. `/wall` is measured but holds no control, so it writes no line. The opener scopes hold:
   - the /host action group and the dialog: 4 each
   - the Select list: 15
   - the Reset dialog: 11
   - the Power menu: 7
   - What's new, the Power confirmation and the sudo dialog: 3 each

   The Navigation opener is measured at 390 only, so it has no desk lines.

**Where the dumps lie.** `$HOME/.cache/holzkube-manager-layout/`:

- `14-before.jsonl`, the D-08 before-dump
- `14-noise.jsonl`
- `14-04-before.log`
- `14-04-noise.log`
- `14-04-noise-compare.txt`

None of them is committed. `git status --porcelain` was empty after the runs.

**For 14-07.** Take the after-dump the same way, after the last class change (and again after 13-12 lands, Pitfall 13). Then run `node web/scripts/layout-dump-compare.mjs $HOME/.cache/holzkube-manager-layout/14-before.jsonl <after>`. A difference is read, not regenerated.

## Deviations from Plan

**1. [Rule 2 - Correctness] The comparer also refuses a repeated key and a malformed line.** A dump whose index is not unique within its scope would silently overwrite one control with another in the Map, and that would hide a difference. The plan did not name these two cases. Both fail with exit 1.

**2. Task 2 has no commit of its own.** Its only file, `layout-audit.mjs`, was to be touched only if the noise floor was not zero. It was zero. Its result is recorded here and in the logs outside the repository, and the SUMMARY commit carries it.

The known quick-command deviation from earlier plans does not apply: this plan ran no vitest outside `task test:layout`.

## Self-Check: PASSED

- web/scripts/layout-dump-compare.mjs: FOUND
- web/scripts/layout-audit.mjs, LAYOUT_DUMP: FOUND (8)
- commit 27ace4b: FOUND
- $HOME/.cache/holzkube-manager-layout/14-before.jsonl: FOUND, non-empty (964 lines)
