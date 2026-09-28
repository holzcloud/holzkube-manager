# Phase 11 — UI Review

**Audited:** 2026-09-28
**Baseline:** 11-UI-SPEC.md (approved)
**Screenshots:** no live capture (no dev server on 3000/5173/8080; none started). Rendered fixture screenshot `docs/screenshots/host.png` (1440x900, demo fixture, hardening state) used as visual evidence.
**Interaction captures:** off (workflow.ui_interaction_capture not set). Interaction findings below are code-derived.

---

## Pillar Scores

| Pillar | Score | Key Finding |
|--------|-------|-------------|
| 1. Copywriting | 3/4 | Contract copy verbatim; three off-contract strings ("Load not readable: …", "No filesystem was reported.", invented free-space reason), "1 cores" plural bug |
| 2. Visuals | 2/4 | In the production (hardening) state the declared focal point is absent; Processor and Memory cards are near-empty shells |
| 3. Color | 4/4 | Accent only on nav pill and focus ring; notices use the verbatim amber/slate classes; severity never colour-only |
| 4. Typography | 3/4 | Four sizes held; 12px Manrope detail lines render with collapsed word spacing in the screenshot |
| 5. Spacing | 3/4 | Declared scale followed; Processor header-only card and uneven Memory/Filesystems row heights |
| 6. Experience Design | 3/4 | All states covered; MissingValue pattern bypassed for load; spinning fan ignores reduced motion; filesystem meter/text denominators differ from spec |

**Overall: 18/24**

---

## Top 3 Priority Fixes

1. **Production state has no focal point** (WARNING) — on the Pi, CPU usage and memory are hidden, so the Processor figure (the contract's focal point) is a muted "Not readable" and the Processor card is header-only (`host.tsx:598-638`); the page reads as a wall of muted text under a notice. Fix: when usage is unreadable, let the load figure take the figure slot at `font-semibold text-xl tabular-nums` ("load 0.52") with the 5/15 values and the short reason below, so the card keeps one strong number; update the UI-SPEC focal-point line accordingly.
2. **Load's unreadable branch bypasses the readability pattern** (WARNING) — `host.tsx:611-614` renders one line "Load not readable: {reason}", contradicting the spec rule that MissingValue is the only way a missing value is drawn and adding off-contract copy. Fix: render `<MissingValue reason={load.reason} align="right" />` (or a labelled variant) instead.
3. **Fan icon spins without a reduced-motion guard** (WARNING) — `Sensors.tsx:152` `animate-spin` runs indefinitely; no `motion-reduce`/`prefers-reduced-motion` exists anywhere in `web/src`. Fix: `motion-safe:animate-spin` (the RPM figure already carries the information).

---

## Detailed Findings

### Pillar 1: Copywriting (3/4)
Matches contract: h1, header line, Live line, loading "Reading the host…", error card copy, stale/container/hardening notices (hardening sentence built from response, `joinList`), update outcome sentences, "No swap configured.", sensor and fan empty sentences, virtual-interface singular/plural, footer with rates clause omitted without baseline, short hardening reason.
- WARNING `host.tsx:613` — "Load not readable: {detail}" is not in the Copywriting Contract.
- WARNING `host.tsx:346` — "No filesystem was reported." is off-contract (no empty state declared for filesystems; acceptable addition but undocumented).
- WARNING `host.tsx:229-233` — the client invents a `read-failed` reason "The answer names no filesystem for the data directory." — a client-authored reason posing as a server reason.
- WARNING `host.tsx:595` — "{n} cores" yields "1 cores" on a single-core machine; no singular branch (the virtual-interface line has one).
- Minor: fallback outcome sentences (`host.tsx:314-324`, e.g. "Updated.", "The update was rolled back.") are undeclared; reasonable degradations, should be added to the contract.

### Pillar 2: Visuals (2/4)
- WARNING (screenshot) — the demo fixture is, by spec decision, the production state. In it the Processor card is a single header row with a right-aligned muted two-line reason and a tiny load line; the Memory card holds two muted lines. The contract's focal point ("the Live block's Processor figure") does not exist in the state the operator actually sees. The hardening notice becomes the first thing the eye lands on, which the spec allows only "when a notice is shown" — true here, but then the Live block has no anchor at all.
- WARNING (screenshot) — Memory (two text lines) and Filesystems (one dense meter) side by side at `md:grid-cols-2` give two short cards of unequal content weight; with the Sensors column running taller on the right the left column looks fragmented.
- Hierarchy otherwise clear: h1 > card titles > dt/dd; Device/Service `dl` alignment clean; mono used for identifiers only.
- Icons: nav `Cpu` icon paired with a text label; the ▶ marker is `aria-hidden` with text in the summary. No icon-only buttons.

### Pillar 3: Color (4/4)
- Accent: no `text-primary`/`bg-primary`/`border-primary` in `host.tsx` or `Sensors.tsx`; accent only via Sidebar active pill and `--ring`, as reserved.
- Notice boxes are the verbatim amber (stale, `host.tsx:85`) and slate (container/hardening, `host.tsx:91,97`) strings; hardening correctly slate, not amber.
- Update outcome amber only for `failed`/`rolled-back` (`host.tsx:275-279`).
- Severity: meters via `severityOf`, sensor ▲ with sr-only text (`Sensors.tsx:112-119`). `text-[color:var(--viz-danger)]` on the ▲ for both warn and danger severities — the marker is red even at "high"; inherited from NodeHardware, not introduced here (informational).
- No hardcoded hex/rgb in audited files.

### Pillar 4: Typography (3/4)
- Sizes: `text-xs`, `text-sm`, `text-base`, `text-xl` only — matches the four declared. CPU figure correctly `text-xl`, not `text-2xl`.
- Weights: 600 (`font-semibold` h1, h2, CPU figure, `strong` in hardening notice) plus inherited 500 (CardTitle, Meter, sensor figure) — the documented known deviation.
- WARNING (screenshot) — 12px muted lines render with near-collapsed word spacing: "412MiB·measured23sago", "Theupdatescriptinstalledonthismachine…", "Hiddenbytheunit'sProcSubset=pid". Likely `--hc-track-snug` letter-spacing (`index.css:364`) applied at 12px; it affects the whole app (sidebar alpha text shows the same) but on `/host` it hits the reason lines that carry the page's meaning. Fix: reset tracking to `normal` for `text-xs` or scope the snug tracking to headings.
- `tabular-nums` present on all polled figures checked (CPU, load, rates, sizes, sensor °C).

### Pillar 5: Spacing (3/4)
- Page `space-y-5`, cards `gap-4`, `dl` `gap-x-4 gap-y-2`, notices `px-3 py-2`, card headers `gap-3`, summary `min-h-11` — all as declared.
- Arbitrary values: `md:grid-cols-[10rem_minmax(0,1fr)]`, `lg:grid-cols-[minmax(0,1fr)_20rem]` (declared), `[animation-duration:2s]` and `text-[color:…]` in shared Sensors (inherited). None new.
- WARNING — Processor card with no `CardContent` (per-core omitted under hardening) leaves header-only padding; the right-aligned reason block pushes the card header to three stacked lines on the right against one on the left (screenshot).
- WARNING — `FanList` uses `space-y-1` and sensor rows `py-1.5` (6px), outside the declared scale; inherited, but the spec only exempts `gap-1.5` in the fan row, not `py-1.5` in the sensor list.

### Pillar 6: Experience Design (3/4)
State coverage (code-derived):
- Loading: "Reading the host…" text, no skeleton (as contracted).
- Error without data: Host card + `Problem` + retry sentence; `retry: false` with `refetchInterval` keeps polling.
- Stale: amber notice + `opacity-60` on body; notices themselves stay undimmed — correct.
- Partial: every Device row via `Row`/`MissingValue`; memory one reason for both; per-core omitted not zeroed; rates "—" plus one "Waiting for a second reading" line.
- Empty: sensors, fans, physical interfaces, filesystems.
- Zero/one/many virtual interfaces handled.
- WARNING `host.tsx:611-614` — load's unreadable branch not via MissingValue (see Fix 2).
- WARNING `Sensors.tsx:152` — perpetual spin, no reduced-motion handling (see Fix 3).
- WARNING `host.tsx:368-381` — filesystem bar max is `used + free` and display is df-style ceil, while the detail says "{used} of {size}"; the spec declares `formatPercent(used/size)`. Deliberate (df parity, commented), but the bar and its caption now use different denominators and the contract was not updated.
- Minor `host.tsx:683-685` — `share()` returns NaN → `formatPercent` prints a bare "—" if memory total is 0, which the hard rules forbid ("a bare '—' without words"); unlikely path.
- No `aria-live` on polled figures — correct choice for a 3 s poll (would be noisy).
- No destructive actions; nothing to confirm.

Registry audit: `components.json` present, no third-party registries declared (`registries: {}`) — skipped, 0 blocks checked.

---

## Files Audited
- web/src/routes/host.tsx
- web/src/components/charts/Sensors.tsx
- web/src/components/Sidebar.tsx (NAV_AREAS entry)
- web/src/lib/format.ts (formatPercent)
- web/src/index.css (tracking token)
- docs/screenshots/host.png
- .planning/phases/11-host-seite/11-UI-SPEC.md
