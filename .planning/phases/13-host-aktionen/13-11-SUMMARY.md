---
phase: 13-host-aktionen
plan: 11
subsystem: host-actions
tags: [host-actions, phone-layout, tailwind, browser-test, gap-closure]
status: complete
gap_closure: true
gap_ids: [G-13-3]

requires:
  - phase: 13-host-aktionen
    plan: 10
    provides: host.browser.test.tsx helper-notice test in its two-shape form
provides:
  - host action button classes written through cn() as whitespace-bounded literals, so max-md:whitespace-normal reaches the built stylesheet
  - spillsPastItsButton (per-button fit check) and lineBoxes in host.browser.test.tsx
  - a 1200-px desktop-shape test for the four host action buttons
affects: [14]

actuals:
  tokens: 2400
  tasks: 2
  commits: 1
plan_head_before: e82e209de541c0e3d047eacc6355c18c812669be
plan_head_after: c265f421ccbb0a8ab597690d2bd340d43c7f710f

tech-stack:
  added: []
  patterns:
    - "Measure a control against its own border box (scroll width, descendant boxes, text-line Range rects), not only against the page edge"
    - "Class lists with conditionals go through cn(); never write a class flush against a template interpolation"

key-files:
  created: []
  modified:
    - web/src/components/HostActions.tsx
    - web/src/routes/host.browser.test.tsx

key-decisions:
  - "The class list is fixed with cn() rather than by shortening HOST_ACTION_LABEL.update: the dialog title and status box share the label, and 13-UI-SPEC decided the two-line wrap"
  - "The fit check measures left and right against the button's border box (0.5 px tolerance), as the plan specified; vertical containment is not asserted"

metrics:
  duration: 20min
  completed: 2026-09-30
---

# Phase 13 Plan 11: Host action labels fit their buttons at 390 px (G-13-3) Summary

**The update button's `max-md:whitespace-normal` now reaches the stylesheet (class list through `cn()`), so "Check for updates and install" wraps to two lines inside its button at 390 px. A per-button fit check in Chromium and a 1200-px desktop-shape check hold it, and both went red against the fault.**

## Where it ran

On the operator's Pi (aarch64): Node via nvm, the browser project's Chromium, Go from `~/.local/go`. No `-race` (the race verdict is CI's). Nothing installed, pushed, tagged or released. The production service was not addressed.

## Root cause (as the plan found it, reconfirmed)

`className={\`... max-md:whitespace-normal${spec.destructive ? ' text-destructive' : ''}\`}`: the last class was written flush against the interpolation, so Tailwind's source scanner saw a token it could not read and generated no rule. The button's base `whitespace-nowrap` won. jsdom cannot see this, because the class is in `classList` at runtime. The page-edge check `overflowing` cannot see it either: a 191-px label in a 390-px page never reaches the page edge.

## Tasks

| Task | Name | Commit | Files |
| ---- | ---- | ------ | ----- |
| 1 | The update label wraps inside its button at 390 px, measured in Chromium | c265f42 | web/src/components/HostActions.tsx, web/src/routes/host.browser.test.tsx |
| 2 | The whole gate on the Pi, production untouched | none (the gate found nothing, no file changed) | none |

## Red runs (exit codes read from the command itself)

1. **Fit check against the current markup (step 1):** `npm --prefix web run test:browser -- src/routes/host.browser.test.tsx` exited **1**, with 3 failed and 4 passed. The failing tests were "lays the four host actions out" and both helper-notice shapes (the disabled buttons). Each named the button:
   - `"Check for updates and install" scrolls: scrollWidth 191 > clientWidth 189`
   - `<svg class="lucide lucide-download size-4"> ... spans -0.5..15.5 outside 0.0..191.0`
   - `text line of ... "Check for updates and install" spans 19.5..191.5 outside 0.0..191.0`
2. **Desktop check against an unprefixed `min-h-11` (step 3):** exited **1**, with 1 failed and 7 passed: `Check for updates and install height: expected 44 to be 28`. Restored afterwards, verified with `cmp`.
3. **Fit check against the reinstated original template (step 4):** the class template from `git show HEAD:` was put back, and the run exited **1**, with 3 failed and 5 passed. It showed the same three findings for "Check for updates and install" (sw 191 / cw 189, icon -0.5 px, text to 191.5 px). Restored afterwards, verified with `cmp`.

The green runs after the fix: browser file exit 0 (8 passed), jsdom `HostActions.test.tsx` + `host.test.tsx` exit 0 (186 passed; the `text-destructive` per-button test is among them), lint exit 0 (after one formatter wrap in the new helper), typecheck exit 0.

## Built stylesheet

After `./bin/task build:web` (exit 0), `grep -c 'max-md\:whitespace-normal' internal/httpapi/dist/assets/index-*.css` returns **1**. The plan measured 0 at planning.

## Interpolation scan (step 5)

`grep -rnE '[a-z0-9]\$\{' web/src --include=*.tsx` after the fix returns:
- `routes/audit.test.tsx:25` and `routes/images.test.tsx:190`, test data strings;
- `components/ApplyManifest.tsx:148` and `components/MachineClasses.tsx:77`, prose pluralisation;
- `components/HostActions.tsx:248`, the new comment that quotes the fault.

None is a class list, so nothing else needed fixing.

## Gate (Task 2)

- Before: `NRestarts=0`, `ActiveEnterTimestamp=Tue 2026-09-29 20:25:38 CEST`
- `./bin/task ci`: **exit 0**. It covered web lint, both vitest projects (59 files, 716 tests), the build, golangci-lint 2.13.1 (0 issues), `go test ./... -count=1` (publicrepo ok), `test:layout` and `test:next`. The known `internal/upgrade` flake did not occur (ok, 73.5 s and 41.5 s).
- `GOOS=linux GOARCH=amd64 GOTOOLCHAIN=go1.26.7 go build ./...`: exit 0
- `file bin/holzkube-managerd`: `ELF 64-bit LSB executable, ARM aarch64, ...`
- After: `NRestarts=0`, `ActiveEnterTimestamp=Tue 2026-09-29 20:25:38 CEST`, identical
- `/usr/local/sbin/holzkube-manager-host` and `/etc/systemd/system/holzkube-manager-host.path`: both absent (the test exited 0)
- `git diff --stat a45015c..HEAD -- go.mod go.sum web/package.json web/package-lock.json`: empty
- `git status --porcelain` after the gate: empty

## Deviations from Plan

None. The plan was executed as written. One formatter-only wrap in the new helper was needed for biome before the commit, which is not a behaviour change.

## Known Stubs

None.

## Self-Check: PASSED

- FOUND: web/src/components/HostActions.tsx (contains `max-md:whitespace-normal`, `Check for updates and install`)
- FOUND: web/src/routes/host.browser.test.tsx (contains `scrollWidth`, `index.css`)
- FOUND: commit c265f42
