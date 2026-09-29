# Phase 13 — UI Review

**Audited:** 2026-09-29
**Baseline:** .planning/phases/13-host-aktionen/13-UI-SPEC.md (incl. "Checker resolutions" and "Additions from the code-review fixes")
**Screenshots:** not captured (code-only audit, no dev server started by order); the committed render docs/screenshots/host.png (fixture: helper missing, admin session) was inspected
**Interaction captures:** off (not requested; interaction findings below are code-derived)

---

## Pillar Scores

| Pillar | Score | Key Finding |
|--------|-------|-------------|
| 1. Copywriting | 3/4 | Every contract string present verbatim; "back" sentence degrades to "up since ." when uptime is unreadable |
| 2. Visuals | 3/4 | Order, grouping and red machine-wide pair as specified; waiting notice is not a live region |
| 3. Color | 3/4 | Slate/emerald/red sets and accent reservation exact; disabled red outline text at 50 % opacity is faint (host.png) |
| 4. Typography | 4/4 | Only text-xs / text-sm / text-base / text-xl; authored weights 400/600 only |
| 5. Spacing | 3/4 | On the scale, but the pair separator measures 32 px (gap-2 + w-4 + gap-2), not the declared 16 px |
| 6. Experience Design | 3/4 | Typed-hostname gate, reason line, sudo and status focus solid; cancelling the dialog drops focus to body, and an open dialog ignores a reason that appears after it opened |

**Overall: 19/24**

---

## Top 3 Priority Fixes

1. **WARNING — Focus lost on Cancel/Escape/X** — `onCloseAutoFocus={(e) => e.preventDefault()}` (web/src/components/HostActions.tsx:323) runs on every close, not only after a 202. On "Keep running", Esc or the X the trigger is still enabled, yet focus goes to `<body>`; a keyboard or screen-reader user is thrown to the top of the document next to four destructive buttons. Fix: prevent only when an order was just placed (e.g. a `placedRef` set in `onSuccess`), otherwise let Radix return focus to the trigger.
2. **WARNING — Open dialog stays confirmable after the buttons turn off** — `HostActionDialog` only knows `matches`/`isPending`; if a poll fails, another operator's order appears, or the session changes while the dialog is open, `disabledReason` turns non-null in the header but the dialog's confirm stays live. The server 409/403 is the real lock, so nothing unsafe happens, but the operator gets a Problem after typing the name instead of the reason up front. Fix: pass `reason` into the dialog; when non-null, disable the confirm and show the reason line above the footer (or close the dialog with the reason).
3. **WARNING — "up since ." with an empty time** — web/src/components/HostActions.tsx:678-684: when `uptime_seconds` is unreadable, `bootTime` is null and the emerald box reads "...holzkube-manager is back; up since ." (note: `back` for reboot/poweroff cannot be reached without a boot time, but the render path still builds the broken sentence and a later change to the phase logic would expose it). Fix: drop the "; up since …" clause when `boot === null`.

---

## Detailed Findings

### Pillar 1: Copywriting (3/4)
- All four labels, dialog titles, descriptions, connection boxes, red shutdown box, helper and password lines, reason lines (incl. checker additions `notAnswering`, `updateRunning` and the review-fix `underWay`), status sentences, helper notice and "Keep running" / "Dismiss status" match the contract (HostActions.tsx:51-158, 593-688, 797-818; host.tsx:177-179).
- WARNING: "up since ." fallback (above, fix 3).
- Minor: the review-fix phase "no answer" sentence ("no answer: the helper recorded nothing for this order within 1 min", :663) and the NOT_DONE set (:607-621) are not in the UI-SPEC Copywriting table; the Additions section names the phase only. Record the exact copy in the spec.
- Minor: the meta line switches to "recorded {time}" for `from: 'result'` (:754) — sound, but undeclared.
- Minor: a typed text that does not match gives no inline hint; the only cue is the disabled confirm next to the label "Type {hostname} to confirm". Acceptable per spec, but a trailing-character typo stays silent.

### Pillar 2: Visuals (3/4)
- Header right half, harmless pair first, machine-wide pair apart with `text-destructive`, AlertTriangle + red title on the destructive dialogs, destructive vs default confirm — all as specified (:235-256, :327-334, :377). host.png confirms placement and the reason line under the buttons, right-aligned.
- Deviation (accepted): `<fieldset aria-label>` instead of `div role="group"`; reset classes keep the phone grid intact.
- WARNING: the waiting notice (host.tsx:176) is a plain `<p>`; the status box (role="status") carries the transition, so it is announced, but the replacement of the amber stale notice by the slate one is silent. Low impact.
- Icons are always paired with a visible label; no icon-only controls added.

### Pillar 3: Color (3/4)
- SLATE/EMERALD/RED constants (:58-60) are the NodeActions/P11 strings verbatim; `phaseColour` maps phases exactly as the table (emerald only for back / current / updated; red for rejected, failed, not-picked-up, no-answer, other update outcomes).
- Accent only on the default confirm of the two service dialogs (`variant="default"`) and focus rings; header buttons outline, notices never accent. Waiting is slate, stale stays amber, never both (host.tsx:175-189).
- Carried item 4: the sensor ▲ in host.png is amber for a warn value, not danger red — resolved.
- WARNING: disabled "Restart host" / "Shut down host" are `text-destructive` at `disabled:opacity-50` over the dark ground; in host.png they read noticeably fainter than the neutral pair. Legible, but the state that most needs explaining is the least legible. Consider leaving the destructive tint off while disabled.

### Pillar 4: Typography (4/4)
- Sizes: text-xs (reason, meta, how-it-is-carried-out, pre, closing line), text-sm (body, boxes), DialogTitle default, h1 unchanged. Weights authored: font-semibold only; 500 inherited from Button/Label (accepted, resolution 3). `tabular-nums` on the meta line. Hostname in `font-mono break-all` in title and label (long-hostname backstop covered in code).
- Note: `<code className="font-mono text-xs">` inside a text-sm sentence (:601, :651) is a deliberate step down; consistent across the file.

### Pillar 5: Spacing (3/4)
- gap-1 / gap-2 / space-y-1 / space-y-4 / mt-1 / mt-2 / px-3 py-2 all as declared; no arbitrary values authored (only the inherited `text-[0.8rem]` in ui/button).
- WARNING: the pair spacer is `w-4` inside a `gap-2` flex row, so the two pairs sit 8 + 16 + 8 = 32 px apart; the spec's table declares 16 px. Visible in host.png. Either accept 32 px in the spec or use `w-0`/negative margin to reach 16.
- Phone: `max-md:min-h-11 max-md:h-auto max-md:whitespace-normal` on each button, Input `max-md:h-11`, Dismiss `size="sm"` → 44 px below md. The dialog X stays 28 px (declared Phase 14 boundary).

### Pillar 6: Experience Design (3/4)
Destructive-action safety, as asked:
- No accidental trigger: every action, including update and restart service, needs the exact hostname (trimmed, `hostname !== ''` guarded, :308), the confirm is disabled until then and while pending, Enter submits only when `matches && !isPending` (:312), then a sudo step. A double submit is blocked by `isPending`. Good.
- Disabled states always explained: one reason line whenever `reason !== null`, wired via `aria-describedby`, covering container, helper, reader, hostname, not answering, update running, under way, pending (:170-203). Undefined role offers nothing. Good. Gap: the dialog does not re-check (fix 2).
- Focus: Radix initial focus lands on the Input (first focusable; the X is rendered after children). After a 202 focus moves to the status box once per order id (`takeFocus` = `from === 'held'`, effect keyed on id). Defect: the unconditional `onCloseAutoFocus` preventDefault on cancel (fix 1).
- Errors: `Problem` keeps the dialog open with the typed text. Minor: a stale `run.error` stays visible after the user edits the text; reset it on change.
- Final phases offer "Dismiss status", dismissed per id including when the order comes back from `order` or `result`; the 60 s / 15 min "no answer" phases prevent a stuck disabled state.
- Registry audit: shadcn initialized, no third-party registries (`registries: {}`), no block added — nothing to check.

---

## Files Audited
- .planning/phases/13-host-aktionen/13-UI-SPEC.md
- web/src/components/HostActions.tsx
- web/src/routes/host.tsx
- web/src/components/ui/button.tsx
- web/src/components/ui/dialog.tsx
- web/src/components/ui/input.tsx
- docs/screenshots/host.png
