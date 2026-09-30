---
phase: 14-telefon-tippziele
plan: 06
subsystem: ui
tags: [tap-targets, tailwind, max-md, navigation-drawer, reset-dialog, layout-audit]
status: complete

requires:
  - "14-05: the primitives at 44 px and the three remaining single-place findings"
  - "14-04: the 1280 before-dump (14-before.jsonl) and the comparer"
provides:
  - "Sidebar.tsx: 44-px navigation links below md, and a drawer that scrolls (max-md:overflow-y-auto)"
  - "WhatsNew.tsx: 44-px version button and release chips below md"
  - "NodeActions.tsx: Reset dialog disk rows as one 44-px label each; flag labels 44 px"
  - "NodeSchedulingActions.tsx: checkbox phone margin max-md:mt-3"
  - "./bin/task test:layout green: 27 routes, 9 opened states"
affects: [14-07]

actuals:
  tokens: 2600
  tasks: 2
  commits: 2
plan_head_before: 4a4d6e333d27c9f4423cb1c4d5106806721ca195
plan_head_after: 46a363febd2bac12d8fdac0582bc26dea5b64c0b

tech-stack:
  added: []
  patterns:
    - "A disk row as a label that takes over the li's flex classes: the whole row is the phone target, the desk box is unchanged"
    - "A drawer that holds 44-px rows scrolls on its own below md; the audit's CUT OFF line catches one that does not"

key-files:
  created: []
  modified:
    - web/src/components/Sidebar.tsx
    - web/src/components/WhatsNew.tsx
    - web/src/components/NodeActions.tsx
    - web/src/components/NodeSchedulingActions.tsx

key-decisions:
  - "The wipe-mode radios stay unchanged: the audit did not name them. Their labels are already 44 px or more at 390"
  - "The flag labels get max-md:min-h-11 and 24-px checkboxes anyway, as the UI-SPEC names them. The audit passed them before, so that change has no red run of its own"
  - "When the Reset dialog does not ask for disks, a row is a plain div with the old flex classes, not a label with no control in it"

requirements-completed: [MOB-01]
requirements-partial: [MOB-03]

duration: 18min
completed: 2026-09-30
---

# Phase 14 Plan 06: The single places and the whole audit green Summary

**Below md, the navigation links, the version button, the release chips and the Reset dialog's disk rows are now 44 px, and the drawer scrolls. `./bin/task test:layout` exits 0 with "Nothing out of reach at 390px and 1280px, every control is at least 44px at 390px, on 27 routes and in 9 opened states." The 1280 dump compares `964 controls, 0 differ` against the 14-04 before-dump.**

## Where it ran

On the operator's Pi (aarch64). Node came from nvm and Go from `~/.local/go`. No `-race` run. Nothing was installed or pushed. `holzkube-manager.service` and `/var/lib/holzkube-manager` were not touched. Logs are in `$HOME/.cache/holzkube-manager-layout/14-06*`.

## Task 1: navigation and What's new (72cdb8c)

- **Sidebar:** every Link gets `max-md:min-h-11`, and the nav gets `max-md:overflow-y-auto`. No key handler was added.
- **Sidebar header comment:** it now says a new area needs one entry here, one route in `web/src/routeTree.ts` and one entry in `web/scripts/layout-routes.json`.
- **WhatsNew:** the version button gets `max-md:flex max-md:min-h-11 max-md:items-center`, and each release chip gets `max-md:min-h-11 max-md:px-3`.

**Checks:**
- jsdom Sidebar and WhatsNew: 15/15 (exit 0).
- WhatsNew browser test: 1/1 (exit 0).
- The audit exited 201, with one finding left, in the Reset dialog. `ok 390px / · Navigation (15 controls)` and `ok 390px / · What's new (3 controls)`. No CUT OFF.

**Seen red:** I took `max-md:overflow-y-auto` out again and reran the audit. It exited 201 with `CUT OFF 390px / · Navigation -- the drawer is 885px tall in 844px and does not scroll`. After restoring the file from a saved copy (`cmp=0`), the opener was ok again.

## Task 2: Reset dialog and the whole gate (46a363f)

**NodeActions (Reset dialog):**
- Each disk row is now one `<label className="flex items-center gap-2 max-md:min-h-11">` around its checkbox (`max-md:size-6`, aria-label `Wipe {device}` kept) and its text.
- When the dialog does not ask for disks, a row is a `div` with the same flex classes.
- The two flag labels get `max-md:min-h-11`, and their checkboxes `mt-1 max-md:mt-3 max-md:size-6`.
- The wipe-mode radios are unchanged, because the audit did not name them.

**NodeSchedulingActions:** both checkboxes change from `max-md:mt-2.5` to `max-md:mt-3`.

**Seen red:** I took `max-md:min-h-11` off the disk label and reran the audit. It exited 201 with `SMALL 390px /nodes/m-cp-1 · Reset dialog (2)`, listing `<input> (via its label) 326x32 "Wipe /dev/nvme0n1"` and `326x24 "Wipe /dev/mmcblk0"`. The file was restored with `cmp=0`. Before this plan, the 14-06-t1 log showed the same two inputs as bare 13×13 boxes.

**The final gate:** `LAYOUT_DUMP=… ./bin/task test:layout` gave **exit 0** (read from the command itself) and printed:
```
Nothing out of reach at 390px and 1280px, every control is at least 44px at 390px, on 27 routes and in 9 opened states.
```
- Counts: 27 routes and 9 opened states, as the plan expected.
- /host passes on the route (`ok 390px /host (14 controls, 27 items)`) and in both openers: `Host actions (helper installed)` and `Host action dialog`, each with 4 controls, at 390 and 1280. That is MOB-03's measured half.

**Desk unchanged (D-08):** `layout-dump-compare.mjs 14-before.jsonl 14-06-after.jsonl` gave **exit 0** and `compared 964 controls, 0 differ`.

**Other checks:**
- jsdom NodeActions and NodeSchedulingActions: 13/13.
- Full jsdom: 711/711.
- Browser: 22/22.
- lint and typecheck: exit 0 each.

## Word-diff check against fda4579

I read the word diff of all four files against fda4579. Every added class token that sets a size, padding, margin or position starts with `max-md:`. The only unprefixed class tokens in the diff are copies of existing classes:
- `flex items-center gap-2`, moved from the disk `li` to its label, or to the `div`.
- `mt-1`, re-emitted on the flag checkboxes.

## Files fixed beyond the plan's list, and exemptions

None of either. All the findings were in the four files the plan lists, and the audit has no new exemption.

## Deviations from Plan

1. **[Rule 3] vitest was run from `web/`.** This is the same quick-command issue as in the earlier plans.
2. **The D-08 dump comparison was run here.** It gave 0 differ. 14-07 still takes its own final one.

## Known Stubs

None.

## Threat Flags

None. For T-14-12: each disk row is its own label with exactly one checkbox and no nesting. The typed phrase and the server token in ResetDialog are unchanged. The NodeActions tests still find every checkbox by "Wipe {device}".

## Self-Check: PASSED

- FOUND: all four files in key-files
- FOUND: commits 72cdb8c, 46a363f
- FOUND: $HOME/.cache/holzkube-manager-layout/14-06-final.log, 14-06-compare.txt
