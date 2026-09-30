---
phase: 14-telefon-tippziele
fixed_at: 2026-09-30T12:28:16Z
review_path: .planning/phases/14-telefon-tippziele/14-REVIEW.md
iteration: 1
findings_in_scope: 5
fixed: 5
skipped: 0
status: all_fixed
---

# Phase 14: Code Review Fix Report

**Fixed at:** 2026-09-30T12:28:16Z
**Source review:** .planning/phases/14-telefon-tippziele/14-REVIEW.md
**Iteration:** 1

**Summary:**
- Findings in scope: 5 (CR-01, WR-01..WR-04; the six Info findings were out of scope)
- Fixed: 5
- Skipped: 0

## Where it ran

This ran on the operator's Pi (aarch64), in the main checkout (`workflow.use_worktrees` is false, so no worktree was used). Node came from nvm, Go from `~/.local/go`, and Chromium from the browser project. There was no `-race`. Nothing was installed, pushed, tagged or released. `holzkube-manager.service` was not touched: `NRestarts=0` and `ActiveEnterTimestamp=Tue 2026-09-29 20:25:38 CEST`, the same values as the 13-11 gate. Every exit code below was read from the command itself, not through a pipe.

## Fixed Issues

### CR-01: "Keep running", Escape and the X still close the host dialog while the order is in flight

**Files modified:** `web/src/components/HostActions.tsx`, `web/src/components/HostActions.test.tsx`
**Commit:** 46fd740
**Applied fix:** `onOpenChange` now ignores a close while `run.isPending`. Escape, a tap outside and the close X all reach the dialog through that handler. "Keep running" is `disabled={run.isPending}`. The `onCloseAutoFocus` comment no longer claims that every close placed nothing.
**Red run:** The new test holds a confirm open, tries each of the three ways to close, and expects three things: the same dialog node is still there, Keep running is disabled, and `place` has not been called. After that the order goes through with the dialog still showing.
- Against the old code: exit 1, 3 failed. The dialog was `null` for all three.
- With only the guard removed: exit 1, Escape and the X failed.
- With only `disabled` removed: exit 1, all three failed.
- After the fix: exit 0, 102/102 in the file.
**Status:** fixed: requires human verification. This is a state-handling change. Watching a real 428 sudo prompt on top of the dialog (Keep running under it stays off until the prompt answers) is worth one look on a phone.

### WR-01: Focus is lost to `<body>` on a cancel after a reason arrived while the dialog was open

**Files modified:** `web/src/components/HostActions.tsx`, `web/src/components/HostActions.test.tsx`
**Commit:** e43103e
**Applied fix:** When the opener is disabled, a cancel now focuses the group's reason line. The dialog reaches that line through a ref rather than a `getElementById`. The line has `tabIndex={-1}`, so it is not a tab stop, and the audit's control selector excludes `[tabindex="-1"]`. No class changed.
**Red run:** The new test opens the dialog, rerenders with `pollFailed=true`, cancels with Keep running, Escape and the X, and expects the reason line to have focus.
- Against the old code: exit 1, 3 failed. The element with focus was `<body>`.
- After the fix: exit 0.

### WR-02: The request monitor's allowlist passes a sudo grant

**Files modified:** `web/scripts/layout-audit.mjs`
**Commit:** f276922
**Applied fix:** The allowlist is now only `path.endsWith('/confirm')`. `/api/v1/auth/` and `/api/v1/setup` are no longer let through. Setup and login run outside any opener, and a clean run shows only two lines: `POST /api/v1/users -> 428` at each width. Both comments now say that a 2xx sudo grant counts as EXECUTED.
**Red run:** The same deliberate fault was used both times. The Sudo dialog opener names the audit's own account, so the replayed user create is a 409, and it "closes" by submitting the sudo password. That leaves the grant as the only 2xx.
- Old allowlist plus fault: exit 0, with `POST /api/v1/auth/sudo -> 204` at both widths. This is the gap, seen green.
- New allowlist plus fault: exit 1, with `EXECUTED  POST /api/v1/auth/sudo -> 204` at both widths.
- The fault was removed from a saved copy and checked with `cmp`. After the commit, `git show HEAD:web/scripts/layout-audit.mjs | cmp - web/scripts/layout-audit.mjs` was byte-identical.
- The clean run exits 0.

Logs: `~/.cache/holzkube-manager-layout/14-fix-wr02-{gap,red,green}.log`.

### WR-03: The waiting notice's `aria-live` is mounted together with its text

**Files modified:** `web/src/routes/host.tsx`, `web/src/routes/host.test.tsx`
**Commit:** 18f32f3
**Applied fix:** An empty `<div aria-live="polite">` with no role is always mounted, and only the waiting sentence comes and goes inside it. The stale notice is a sibling, so the order box is still the only `role="status"`. The existing test now checks `aria-live` on the enclosing region instead of on the `<p>`.
**Red run:** The new test renders a started order while the host is answering. It expects exactly one empty polite region, then fails the poll, and expects the sentence inside that same node. After the poll recovers, the sentence is gone and the region is still there.
- Against the old code: exit 1, 2 failed with "expected [] to have a length of 1".
- After the fix: exit 0.

**Layout:** When empty, the div has no height, and its margin collapses into its neighbours' inside `space-y-5`. LAYOUT_DUMP at 1280px against `14-before.jsonl` compared 964 controls, and 0 differed.

### WR-04: The README claims every menu and dialog is held to 44px; the audit opens nine

**Files modified:** `README.md`
**Commit:** 8d7edbd
**Applied fix:** The line now reads: "Every screen, one-handed, and the menus and dialogs behind its main actions: the build fails when a control on any of them is under 44px." This is what the nine openers measure, and the guide already says it that way. The change is text only, and `go test ./internal/publicrepo/...` is ok.

## Verification after all five

All of these ran in the main checkout on the Pi:

- `LAYOUT_DUMP=… ./bin/task test:layout`: exit 0. It covered 27 routes and 9 opened states, with 0 EXECUTED lines.
- `layout-dump-compare.mjs 14-before.jsonl 14-fix-final.jsonl`: exit 0, 964 controls compared, 0 differed. The desk is identical.
- `npx vitest run --project jsdom`: exit 0, 56 files, 719 tests.
- `npm --prefix web run test:browser`: exit 0, 5 files, 22 tests.
- `npm --prefix web run lint`: exit 0.
- `npm --prefix web run typecheck`: exit 0.
- `git status --porcelain` after the runs was empty.

---

_Fixed: 2026-09-30T12:28:16Z_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
