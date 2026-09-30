---
phase: 14-telefon-tippziele
plan: 02
subsystem: web
tags: [host-actions, a11y, focus, radix-dialog, vitest, browser-test]
status: complete

requires: []
provides:
  - "HostActionDialog: focus back to the opener on cancel, none after an order; follows the group's reason (sentence above the footer, confirm off, aria-describedby); Problem reset on typing"
  - "phaseSentence 'back' for reboot/poweroff without a boot time: the clause left out"
  - "the host-level pair 16 px from the service pair above md (zero-width spacer)"
  - "/host waiting notice: aria-live=polite, no role"
affects: [14-04, 14-07, 13-12]

actuals:
  tokens: 4600
  tasks: 2
  commits: 2
plan_head_before: 56e5f7f82803fb9a0b4029718df3814f327a0ed4
plan_head_after: 8ee19f4fc6df12941ccbc2da17d4db83dde9f104

tech-stack:
  added: []
  patterns:
    - "Radix modal DialogContent composes its own onCloseAutoFocus that prevents the default and focuses context.triggerRef; without a DialogTrigger that is null, so the caller records the opener on click and focuses it itself"

key-files:
  created: []
  modified:
    - web/src/components/HostActions.tsx
    - web/src/components/HostActions.test.tsx
    - web/src/routes/host.tsx
    - web/src/routes/host.test.tsx
    - web/src/routes/host.browser.test.tsx

key-decisions:
  - "Focus on cancel is returned explicitly from a ref HostActions sets on click, not by letting Radix's default run: Radix's modal content always prevents the default and focuses its DialogTrigger ref, which these buttons are not"
  - "The pair spacing is a zero-width spacer (w-0) kept in the markup and max-md:hidden, so the gap stays keyed to the reboot button for a fifth action (13-12)"
  - "The waiting notice gets aria-live=polite and no role, so getByRole('status') keeps finding exactly the order box"

requirements-completed: []
requirements-partial: [MOB-03]

duration: 11min
completed: 2026-09-30
---

# Phase 14 Plan 02: The 13-UI-REVIEW items carried into this phase Summary

**If an operator cancels a host action dialog, focus now goes back to the button that opened it. An open dialog turns its confirm off and says why when a disabling reason appears. An old error clears once the operator types again. The "back" sentence never reads "up since .". Above md the host-level pair stands 16 px from the service pair, not 32. The waiting notice is a polite live region. Every changed behaviour was seen red first.**

## Where it ran

On the operator's Pi (aarch64). Node came from nvm. The browser tests ran in the browser project's Chromium on the Pi. No daemon was started. `holzkube-manager.service` and `/var/lib/holzkube-manager` were not touched. Nothing was installed, pushed or released.

## Performance

- **Duration:** about 11 min
- **Started:** 2026-09-30T09:32:09Z
- **Completed:** 2026-09-30T09:43:31Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments

- **Focus on cancel (13-UI-REVIEW fix 1).** Keep running, Escape and the close X each return focus to the button that opened the dialog. Once this dialog has placed an order, it does not hand focus back, so the status box can take it.
- **An open dialog follows the reason (fix 2).**
  - HostActions passes its computed `reason` into HostActionDialog.
  - When a reason is present, the dialog shows the reason line's own sentence above the footer, in `#host-action-dialog-reason`.
  - The confirm button is disabled and points to that sentence through `aria-describedby`.
  - `submit` checks the reason as well, so the Enter key cannot get around the disabled button.
  - The server's 409/403 stays the real lock.
- **Stale Problem.** A change in the field resets the mutation when it holds an error.
- **"back" without a boot time (fix 3).**
  - Reboot: "done. The host restarted and holzkube-manager is back."
  - Poweroff: "the host was switched on again and holzkube-manager is back."
  - With a boot time, both sentences are unchanged.
- **Pair spacing (item 4).**
  - The spacer before "Restart host" changed from `w-4` to `w-0`, so the gap is 8 + 0 + 8.
  - The gap at 1200 px was **32 px before and 16 px after**. The browser test measures it.
  - The spacer keeps `max-md:hidden`, so the 390 px phone grid is untouched and every existing phone test passes.
  - **This is the phase's one deliberate change above md.** It comes from 13-UI-REVIEW and corrects Phase 13's markup to Phase 13's own spec. It lands before 14-04 takes the D-08 1280 before-dump, so D-08 compares the tap-target work alone.
- **Waiting notice.** It now has `aria-live="polite"` and no role. The test checks that exactly one `status` remains on the page.
- **Copy held verbatim.** The "no answer" sentence was already asserted character for character in the HostOrderStatus table. The "recorded" test now asserts "recorded {time}" and that "placed" is absent. Both strings already matched 14-UI-SPEC, so no code string changed.

## The red runs (exit codes read from the command itself)

The test files were changed first and the component was left as it was.

- **Task 1, jsdom** (`cd web && npx vitest run --project jsdom src/components/HostActions.test.tsx`): **exit 1**, 5 failed, 91 passed.
  - `focus returns to the button that opened it on cancel with Keep running`, `… with Escape`, `… with the close X`. Focus was on `<body>`.
  - `the reason in an open dialog: shown above the footer, and the confirm is off until it is gone`. The sentence was not in the dialog.
  - `the Problem clears as soon as the typed text changes`. The `<p>` was still there.
- **Task 1, the placed guard:** the test `focus does not return to the button after an order was placed` was added after the green run.
  - With `if (!placed.current)` removed, so focus always returns: **exit 1**, 1 failed (that test). With the guard restored: exit 0, 97 passed.
  - The existing route test in host.test.tsx ("after an order is placed … focus lands on the status box") stayed **green** with the fault in. The followed order disables the trigger, so a stray `focus()` does nothing. That is why the unit test exists. It keeps the trigger enabled by never following the order.
- **Task 2, jsdom** (`… src/components/HostActions.test.tsx src/routes/host.test.tsx`): **exit 1**, 6 failed, 188 passed.
  - `reboot back with the boot time unknown: no "up since" at all` received "…is back; up since .".
  - `poweroff back with the boot time unknown: …` failed the same way.
  - The 4 `a started {action} and a failed poll: the waiting notice …` cases failed because `aria-live` was missing.
- **Task 2, browser** (`npm --prefix web run test:browser -- src/routes/host.browser.test.tsx`): **exit 1**, 1 failed, 8 passed. `stands the host-level pair 16 px from the service pair` reported "expected 32 to be close to 16".

## The green runs

- HostActions.test.tsx and host.test.tsx: exit 0, 194 passed.
- host.browser.test.tsx: exit 0, 9 passed.
- The whole jsdom project: exit 0, 56 files and 708 tests.
- The whole browser project: exit 0, 4 files and 20 tests.
- `npm run lint`: exit 0. Its remaining 2 warnings and 1 info are in DataTable.tsx and wall.test.tsx, which this plan did not touch.
- `npm run typecheck`: exit 0.

## Task Commits

1. **Task 1: focus back on cancel, the dialog follows the reason, Problem reset** - `8756ea6` (feat)
2. **Task 2: "back" without "up since", polite waiting notice, 16-px pair spacing, copy asserted** - `8ee19f4` (feat)

## Files Created/Modified

- `web/src/components/HostActions.tsx`: opener ref; `reason` and `opener` props on the dialog; `placed` ref; `onCloseAutoFocus` handling both cases; reason paragraph and `aria-describedby`; Problem reset; "back" sentences; `w-0` spacer.
- `web/src/components/HostActions.test.tsx`: 3 focus-on-cancel cases, no focus after an order, the reason in an open dialog, the Problem clearing, the two "back" sentences without a boot time, "recorded {time}".
- `web/src/routes/host.tsx`: the waiting notice as a polite live region.
- `web/src/routes/host.test.tsx`: `aria-live`, no role, one `status`.
- `web/src/routes/host.browser.test.tsx`: the 16-px gap at 1200 px, measured between the two named buttons.

## Decisions Made

See `key-decisions` in the frontmatter. The main one is how focus comes back. The plan said: "preventDefault only when placed, otherwise Radix returns focus to the trigger". That does not work here. Radix's modal `DialogContent` composes its own `onCloseAutoFocus`, which always prevents the default and focuses `context.triggerRef`. That ref is null because the header buttons are not `DialogTrigger`s. So HostActions records the clicked button, and the dialog focuses it itself when no order was placed.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Radix does not return focus without a DialogTrigger**
- **Found during:** Task 1 GREEN.
- **Issue:** The plan's mechanism, conditional `preventDefault`, left focus on `<body>`. `@radix-ui/react-dialog` (`DialogContentModal`) composes a handler that always prevents the default and focuses `triggerRef`, which is null here.
- **Fix:** HostActions keeps an `opener` ref, set in each button's onClick, and passes it to the dialog. `onCloseAutoFocus` always prevents the default and focuses the opener unless `placed.current` is set.
- **Files modified:** web/src/components/HostActions.tsx
- **Commit:** 8756ea6

**2. [Rule 2 - Missing test] Nothing held the "no focus back after an order" half**
- **Found during:** Task 1, after injecting the fault.
- **Issue:** The existing route test stays green with the placed guard removed.
- **Fix:** Added the unit test `focus does not return to the button after an order was placed`. It went red against the fault (exit 1) and green with the guard in place.
- **Commit:** 8756ea6

**3. [Rule 3 - Blocking] The plan's jsdom command fails from the repo root**
- This is the same deviation as in 14-01. `npm --prefix web exec -- vitest run --project jsdom …` exits 1 with "No projects matched the filter "jsdom"". Vitest was run from `web/` instead (`cd web && npx vitest run --project jsdom …`). `npm --prefix web run test:browser` and `lint`/`typecheck` work as written.

## Known Stubs

None.

## Threat Flags

None. T-14-04 and T-14-05 are mitigated as planned: the dialog follows the reason, and focus goes back to the opener on cancel. Both are held by tests seen red. The routes were not changed.

## Issues Encountered

None beyond the deviations above.

## Next Phase Readiness

14-04 can take the D-08 1280 before-dump. The only change above md in this phase, the 16-px pair gap, has landed. 13-12's fifth action needs nothing from here. The reason prop, the focus rule and the spacer do not count actions, and the gap test measures between named buttons.

## Self-Check: PASSED
