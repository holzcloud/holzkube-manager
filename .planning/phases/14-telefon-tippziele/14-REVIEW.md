---
phase: 14-telefon-tippziele
reviewed: 2026-09-30T11:50:41Z
depth: standard
files_reviewed: 27
files_reviewed_list:
  - web/package.json
  - web/scripts/layout-audit.mjs
  - web/scripts/layout-dump-compare.mjs
  - web/scripts/layout-routes.json
  - web/src/App.tsx
  - web/src/routeTree.ts
  - web/src/layoutRoutes.test.ts
  - web/src/fixtures.test.ts
  - web/fixtures/demo.json
  - web/fixtures/host-helper-installed.json
  - web/src/components/HostActions.tsx
  - web/src/components/HostActions.test.tsx
  - web/src/components/NodeActions.tsx
  - web/src/components/NodeSchedulingActions.tsx
  - web/src/components/Sidebar.tsx
  - web/src/components/WhatsNew.tsx
  - web/src/components/ui/button.tsx
  - web/src/components/ui/dialog.tsx
  - web/src/components/ui/dropdown-menu.tsx
  - web/src/components/ui/select.tsx
  - web/src/components/ui/sonner.tsx
  - web/src/components/ui/sonner.browser.test.tsx
  - web/src/routes/host.tsx
  - web/src/routes/host.test.tsx
  - web/src/routes/host.browser.test.tsx
  - README.md
  - docs/guide.md
findings:
  critical: 1
  warning: 4
  info: 6
  total: 11
status: issues_found
---

# Phase 14: Code Review Report

**Reviewed:** 2026-09-30T11:50:41Z
**Depth:** standard
**Files Reviewed:** 27
**Status:** issues_found

## Summary

I reviewed the diff `567cd4c..HEAD` for the listed files, reading the changed regions and the code around them. I ran `layoutRoutes.test.ts`, `fixtures.test.ts` and `HostActions.test.tsx` (jsdom) on the Pi (aarch64): 140/140 pass. I did not run the browser suites or the full layout audit.

Things I checked specifically and found clean:
- **Tailwind classes against `${`:** none in the added lines. Every new class is a whitespace-bounded literal.
- **Layout above md:** every new size class carries a `max-md:` prefix (button, dialog close, DialogHeader `pr-8`, menu and select items, sonner chip, sidebar links, drawer `overflow-y-auto`, WhatsNew). The HostActions spacer went from `w-4` to `w-0`, which is the intended desk change (32 px down to 16 px) and has its own test. One behaviour change reaches the desk, in IN-06.
- **Audit and confirms:** no opener types into a confirmation field or addresses a confirm button. The Power items all open a confirmation first (PowerMenu `onSelect -> setPending`). A host order placed by the audit's own daemon would land in its temporary data dir, where no helper watches (`hostaction.ReferenceOrderPath`). The request monitor does have a gap for sudo (WR-02).
- **Public-repo identifiers:** the new fixture entries (`srv-node-01.homelab.example`, the disk serials and models) all already existed in `demo.json` at diff_base and are the documentation replacements from 2a05b7b. No LAN address, mail address, MAC or home path appears in the added lines.

What I did find: the HostActions dialog's new focus logic assumes that every close placed nothing, and that is false while the order is in flight. There is also a focus-to-disabled-button hole in the very scenario the phase added, a live region that most likely stays silent, an allowlist in the request monitor that would pass a sudo grant, and a README claim wider than the audit's coverage.

## Structural Findings (fallow)

No structural pre-pass was provided.

## Narrative Findings (AI reviewer)

## Critical Issues

### CR-01: "Keep running", Escape and the X still close the host dialog while the order is in flight, and the order goes through anyway

**File:** `web/src/components/HostActions.tsx:331-341, 356-369, 431-433`
**Issue:** While `run.isPending` is true (confirm, then place, shown as "Working…"), the Keep running button, Escape and the close X all stay active. Each of them calls `onClose()`, which unmounts the dialog. The mutation's `onSuccess` is passed as a `useMutation` option, and TanStack Query runs those even after the observer unmounts. So `api.hostActions.place` still completes, `onPlaced(order)` still fires, and the host reboots or powers off after the operator explicitly said "Keep running". The new `onCloseAutoFocus` comment makes it worse by writing down the false premise: "A cancel -- Keep running, Escape, the close X -- placed nothing". The focus logic hands focus back to the trigger on exactly this path, as if nothing had happened. The behaviour itself predates this phase, but the phase built its focus contract on top of it, and no test covers a close while pending.
**Fix:** Do not let the dialog close mid-flight, or make a close mid-flight an honest statement:
```tsx
<Dialog open onOpenChange={(open) => { if (!open && !run.isPending) onClose() }}>
  ...
  <Button type="button" variant="outline" onClick={onClose} disabled={run.isPending}>
    Keep running
  </Button>
```
Also add a test: start a confirm that never resolves, press Escape, Keep running and the X, and assert that the dialog stays open and `place` is never reached after a close.

## Warnings

### WR-01: Focus is lost to `<body>` on a cancel after a reason arrived while the dialog was open

**File:** `web/src/components/HostActions.tsx:365-369` (with `:269-272`, `:268` `disabled={reason !== null}`)
**Issue:** The phase added "the reason in an open dialog" (a failed poll, another order, a role change). In that state the trigger buttons are `disabled` too, because the same `reason` feeds them. If the operator then cancels, `opener.current?.focus()` calls `focus()` on a disabled `<button>`, which does nothing. `e.preventDefault()` has already stopped Radix's fallback, so focus lands on `<body>`. That is exactly the defect 13-UI-REVIEW fix 1 set out to remove. The comment "the trigger is still on and gets focus back" is only true when no reason arrived. The test at `HostActions.test.tsx:121-138` covers the reason-free path only.
**Fix:** Fall back to a focusable element when the opener is disabled, for example the group's reason line:
```tsx
onCloseAutoFocus={(e) => {
  e.preventDefault()
  if (placed.current) return
  const el = opener.current
  if (el && !el.disabled) el.focus()
  else document.getElementById('host-actions-reason')?.focus()  // give the <p> tabIndex={-1}
}}
```
Add a test case: open the dialog, rerender with `pollFailed=true`, press Escape, and assert where focus went.

### WR-02: The request monitor's allowlist passes a sudo grant, the one thing the sudo opener promises never to do

**File:** `web/scripts/layout-audit.mjs:1106-1107`
**Issue:** `allowed = path.startsWith('/api/v1/auth/') || ...` whitelists `POST /api/v1/auth/sudo` and `/api/v1/auth/oidc/sudo`. A 2xx there opens the sudo window for the rest of the context, and every later destructive opener in that context would then reach its handler. The header comment (lines 254-262) says the audit never submits the sudo dialog and that "the request monitor is the proof". For sudo it is not: a regression that submitted the password would print a `REQUEST` line and stay green. The comment at 1085-1089 also says setup and login happen outside any opener, so nothing inside an opener needs `/api/v1/auth/` at all.
**Fix:** Narrow the allowlist to what an opener can legitimately send, and never let sudo through:
```js
const allowed = path.endsWith('/confirm')
// or at most: path === '/api/v1/auth/me' || path.endsWith('/confirm')
```
Then prove it the repository's way: point the sudo opener's steps at the password field and submit, and watch the run go red with EXECUTED.

### WR-03: The waiting notice's `aria-live` is mounted together with its text, so it is most likely never announced

**File:** `web/src/routes/host.tsx:176-186`
**Issue:** The `<p aria-live="polite">` exists only while `waitingFor !== null`, and it is inserted into the DOM already carrying its sentence. Screen readers (NVDA, JAWS, VoiceOver) announce changes inside a live region that was already in the accessibility tree. They generally do not announce a region that appears with its content in the same mutation. So the "polite waiting notice" from 13-UI-REVIEW is likely silent the one time it matters, when the host goes away. Only a later switch between the two sentences would be announced. `host.test.tsx:1631-1633` asserts the attribute, not that anything would be announced.
**Fix:** Keep an always-mounted, role-less polite region and swap the text inside it:
```tsx
<div aria-live="polite">
  {waitingFor !== null && <p className="rounded-md border ...">{...}</p>}
</div>
```
Keep the stale notice as a sibling, so there is still only one `role="status"` on the page.

### WR-04: The README claims every menu and dialog is held to 44px; the audit opens nine

**File:** `README.md:110-111`
**Issue:** "every menu and dialog a tap opens: the build fails when a control on any of them is under 44px." The audit's `OPENERS` (layout-audit.mjs:246-431) are nine fixed states. Other dialogs and menus are never opened, for example PodDiagnosis, NodeWhy, the audit record dialog, the schematic detail dialog, ResumeAfterProvider, the PowerMenu result dialog, the drain panel's checkboxes and the Power menus on clusters and apps. They are only covered indirectly, through the shared primitives. The guide (1668-1675) describes the scope correctly; the README, which is the public claim, does not. CLAUDE.md makes the README the operator-facing statement of what the product does.
**Fix:** Make it match what is measured, for example: "Every screen, one-handed, and the menus and dialogs behind its main actions: the build fails when a control on any of them is under 44px." Alternatively, add openers for the remaining dialogs.

## Info

### IN-01: The guide says all nine opened states are measured "at both widths"; the navigation drawer is phone-only

**File:** `docs/guide.md:1669-1670`
**Issue:** The `Navigation` opener has `widths: [390]` (layout-audit.mjs:305), which is correct, since there is no drawer on a desk. The sentence still says "measures each on its own, at both widths".
**Fix:** Change it to "at both widths where the state exists (the drawer only on the phone)".

### IN-02: The LAYOUT_DUMP "inside the repository" test accepts a folder whose name starts with `..`

**File:** `web/scripts/layout-audit.mjs:164-165`
**Issue:** `!inside.startsWith('..')` treats `<repo>/..cache/dump.jsonl` (relative path `..cache/dump.jsonl`) as outside the repository. Also, only the directory is resolved through symlinks. A dump path that is itself a symlink into the checkout is written through (`writeFileSync` follows it).
**Fix:** Use `inside === '..' || inside.startsWith(`..${sep}`)` to mean "outside". Also `lstat` the file and refuse it if it is a symlink.

### IN-03: On the phone, the Reset flag checkboxes sit 14px below their label line

**File:** `web/src/components/NodeActions.tsx:402-406, 418-422`
**Issue:** The labels are `items-start` with no vertical padding, and the text's first line (`text-sm`, 20px) runs from 0 to 20. With `max-md:mt-3 max-md:size-6` the checkbox runs from 12 to 36, so its centre is at 24 against the heading line's 10. It lines up with the description line, not with "Leave etcd first" or "Reboot afterwards". The `mt-3` was copied from NodeSchedulingActions, where the label has `py-1` and `text-xs`, so the geometry is different.
**Fix:** Align it with the first line, for example `max-md:mt-0` with `max-md:-my-0.5`, or give the label `max-md:items-center` for single-line rows. Check it in the 390px screenshot.

### IN-04: Hard-coded DOM ids for the reason lines

**File:** `web/src/components/HostActions.tsx:425, 438` (and `host-actions-reason` at `:277`)
**Issue:** `id="host-action-dialog-reason"` and `host-actions-reason` are literals. They are fine while exactly one HostActions renders, but they collide the moment a second one does (for example a test harness, or a future per-node host panel).
**Fix:** Use `useId()`.

### IN-05: A redundant assertion in the 16px spacing test

**File:** `web/src/routes/host.browser.test.tsx:698-699`
**Issue:** `toBeCloseTo(16, 0)` already means |diff| < 0.5. The next line checks ≤ 0.5 again, which is close to the same thing.
**Fix:** Keep one of the two.

### IN-06: Disk rows in the Reset dialog now toggle on a text click at desk width too

**File:** `web/src/components/NodeActions.tsx:501-516`
**Issue:** The `<label>` wrapper is not breakpoint-scoped. Above md, a click on the device text now toggles "Wipe /dev/…", which it did not do before. The layout is unchanged, and the D-08 dump measures the input's own box, so the dump would not show it. The change is harmless, since the typed phrase and the token still gate the reset, but it is an above-md behaviour change in a phase declared "below md only".
**Fix:** Either accept it and note it in the summary, or leave it as is. No code change is needed if accepted.

---

_Reviewed: 2026-09-30T11:50:41Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
