# Phase 12 — UI Review

**Audited:** 2026-09-29
**Baseline:** 12-UI-SPEC.md (approved), including "Checker resolutions (orchestrator, 2026-09-28)"
**Screenshots:** not captured (code-only audit, as the task asked; no server started). The committed renders `docs/screenshots/host.png` (1440 px) and `docs/screenshots/wall.png` (1600x900) were read as visual evidence.
**Interaction captures:** off (workflow.ui_interaction_capture not set). Every interaction finding below comes from the code.

---

## Pillar Scores

| Pillar | Score | Key Finding |
|--------|-------|-------------|
| 1. Copywriting | 3/4 | Contract strings are exact, but the committed `host.png` still shows the threshold rule from before IN-05 |
| 2. Visuals | 3/4 | The focal point holds, but Processor and Memory each say "not readable" twice |
| 3. Color | 2/4 | The sensor ▲ is always `--viz-danger`, so an amber Warning shows a red mark; on the wall the amber host tile looks the same as a cordoned node |
| 4. Typography | 3/4 | Sizes and weights as declared; the sensor figure rounds to "82 °C" beside the notice's "82.1 °C" |
| 5. Spacing | 4/4 | Every authored value is on the declared scale; the exceptions are inherited, as the spec lists them |
| 6. Experience Design | 3/4 | States are covered; the wall tile cuts the warning figure at 1600 px |

**Overall: 18/24**

---

## Top 3 Priority Fixes

1. **WARNING — The sensor ▲ is drawn in danger red for a warn-level reading** (`web/src/components/charts/Sensors.tsx:92-93`, `text-[color:var(--viz-danger)]` is not tied to severity). On `host.png`, cpu_thermal at 82 °C (warn 80, danger 110) shows a red ▲ under an amber notice and an amber sidebar triangle. The spec requires the per-value marks to "agree with the notice". **Fix:** colour the ▲ with `SEVERITY_COLOR[severity]` (amber for warn, red only past `danger_c`). This also changes the node page, which the spec allows (D-05: shared). Add a test for a warn-level sensor.
2. **WARNING — The wall host tile loses its only fact at desktop width** (`web/src/routes/wall.tsx:494`, `truncate`). `wall.png` shows "manager · cpu_thermal 8…". The number that makes it a warning is cut off, and the `manager ·` prefix takes up the line's first 10 characters. Its amber-700 is also the same colour as "cordoned" srv-node-03, so the tile tells you nothing a node tile would not. **Fix:** put the reason first and the figure early in the server's wall sentence (e.g. `82.1 °C cpu_thermal`), or show the "manager" role as a small label on the name line instead of a prefix on line 2. The spec's backstop only covers truncation at 390 px, but this happens at 1600 px.
3. **WARNING — The committed README picture is out of date** (`docs/screenshots/host.png`, committed 2026-09-28 23:51, before fix 41350bd). It shows the old threshold rule ("…or at a default for its kind … when the chip names none"), which is not the contract text in `host.tsx:252-254`. The CLAUDE.md rule "a feature is not done until the README says so" makes this a shipping defect. **Fix:** `task build && node web/scripts/readme-images.mjs`, then commit only `host.png` (and `wall.png` if fix 2 changes it).

Further findings (below the top 3): 4. the duplicated "Not readable" in the Processor and Memory cards; 5. the "82 °C" vs "82.1 °C" disagreement; 6. the state line and the notice heading both say "1 threshold crossed".

---

## Detailed Findings

### Pillar 1: Copywriting (3/4)
- PASS: the state words `Healthy / Warning / Not readable` (`HostState.tsx`, `STATE_WORD`); the sidebar sr-only forms, including "— not readable, holzkube-manager did not answer"; the notice heading with singular/plural (`host.tsx:240`); "Also not readable: …" (`:249`); the threshold rule with the IN-05 trip-point clause (`:252-254`); the Readings line (`:504-505`); the RangePicker failure note (`:513`); the footer with "History is sampled every 15 s." (`:213`); "Not readable — no history" (`:891`); the hardening clause "…and are not recorded." (`:141-142`); "1 core" in the singular (`:708`, checker resolution 6); the filesystem caption "usable" (`:455`).
- WARNING: `docs/screenshots/host.png` shows the old threshold sentence (fix 3 above). The public README therefore shows copy the app no longer has.
- WARNING (minor): the header summary "1 threshold crossed." and the notice heading "Warning — 1 threshold crossed" sit 30 px apart and say the same thing. The spec asks for this, but the notice heading could simply be "Warning" plus the list. This is recorded for the spec, not as an implementation fault.

### Pillar 2: Visuals (3/4)
- PASS: in Warning, the amber notice with `AlertTriangle` is the first box under the header (`host.tsx:126`), which is the declared focal point. Checker resolution 1 is implemented: with usage hidden, "load 0.52" takes the `text-xl` slot (`:728-737`). The state mark is three shapes in a fixed `size-4` box (`HostState.tsx`). The host tile has no trend (`wall.tsx:499`).
- WARNING: the Processor card says it twice: "Not readable / Hidden by the unit's ProcSubset=pid…" in the header (`:736`) and "Not readable — no history" in the body (`:891`). The Memory card also does it twice, with the sentences stacked (`:775-803`). Both come from the contract, but the reader parses two not-readable statements per card, and on `host.png` the Processor card is almost all empty space around one 12 px line. **Fix:** when `HistorySlot` returns the sentence and the card body already has a `MissingValue` for the same reason, show only one (e.g. make the slot sentence "No history" when the reason is shown elsewhere in the card).
- Icon-only controls: none are added. The sidebar mark carries sr-only text, and charts have `role="img"` plus `aria-label` (`LiveChart.tsx:101-102`).

### Pillar 3: Color (2/4)
- BLOCKER-adjacent WARNING: the ▲ in the sensor row always uses `--viz-danger` (`Sensors.tsx:93`), so a warn-level sensor gets a red mark. On `/host` the three signals for one fact disagree: amber sidebar triangle, amber notice, red ▲. The spec puts `--viz-warn` on "sensor ▲" (Color table, Status row). This is a contract violation, not a matter of taste.
- WARNING: on the wall, `TILE_COLOURS.warn` (amber-700) is what the spec asks for, but a node that is only "cordoned" uses the same colour. With the name wrapped and line 2 truncated, the host tile cannot be told apart from a node tile by colour or shape (`wall.png`). The only clue is the `manager ·` text.
- PASS: accent is limited to the active nav pill and the focus rings. The selected range button is `secondary`, not accent (`RangePicker.tsx`). Chart lines use `--viz-series-1/2`. The unknown ring is muted grey, not amber. There are no hard-coded hex values in the phase's TSX (the mark uses `var(--viz-*)`).

### Pillar 4: Typography (3/4)
- PASS: authored sizes are `text-xs`, `text-sm`, `text-base` and `text-xl`, and authored weights are 400 and 600 (`font-semibold` on the state word, notice heading and h2), matching the declared set. `tabular-nums` is on the warning list and the figures. Checker resolution 4 is done app-wide: `--text-xs--letter-spacing: normal` (`index.css:263`).
- WARNING: the sensor figure uses `toFixed(0)`, giving "82 °C" (`Sensors.tsx:91`), while the notice says "82.1 °C". The spec deliberately made the fixture 82.1 "so the ▲ and the notice agree". The rounding shows two numbers for one reading. A value of 79.6 would read "80 °C" with no ▲, which looks like a threshold miss. **Fix:** one decimal on host sensor figures, or on every sensor figure.
- Note: `host.png` predates the tracking fix only in the notice copy. Line spacing there already looks normal.

### Pillar 5: Spacing (4/4)
- State line `mt-2 gap-x-2 gap-y-1` (`host.tsx:108`); notice `px-3 py-2`, `gap-2`, `mt-1 space-y-1` (`:237-251`); chart cards `space-y-3`; grid `gap-4 lg:grid-cols-[minmax(0,1fr)_20rem]` (`:517`); RangePicker `gap-2`/`gap-1` plus `max-md:min-w-12`. All match the declared scale and its listed exceptions. `break-words` is on the warning `ul` and the also-line, as the phone section requires.
- Minor: the Readings section uses `space-y-4` (`:498`) inside the page's `space-y-5`. The spec does not list this value, but it is inherited from P11. It is not scored down.

### Pillar 6: Experience Design (3/4)
- PASS: sidebar loading shows no mark and a failed poll shows the ring (`Sidebar.tsx:295-310`). The sidebar poll is suspended on `/host` (`:194`), so the page's rate baseline is not shortened. The history error note is present. The slot shows the sentence only when a value is unreadable and has no points; `rate.no-baseline` is treated as pending (`host.tsx:889`). `aria-live` is on the state word only. When `host === null` there is no wall tile (`wall.tsx:245`). Reduced motion is handled: the fan uses `motion-safe:animate-spin` (`Sensors.tsx:132`). The page adds no destructive actions.
- WARNING: the wall tile truncates at 1600 px (fix 2).
- WARNING: the README screenshot is out of date (fix 3).
- Info: the Warning notice has no link to the offending row (sensor or filesystem). The spec forbids links in the notice, so this is not scored. It is worth reconsidering in Phase 13.

---

## Registry Safety

Registry audit: 0 third-party blocks checked, no flags (`components.json` has `registries: {}`, and this phase adds no shadcn component).

---

## Files Audited
- `.planning/phases/12-host-verlauf/12-UI-SPEC.md`
- `web/src/routes/host.tsx`
- `web/src/components/HostState.tsx`
- `web/src/components/Sidebar.tsx`
- `web/src/routes/wall.tsx`
- `web/src/components/charts/RangePicker.tsx`, `Sensors.tsx`, `LiveChart.tsx`
- `web/src/index.css`
- `web/fixtures/demo.json`
- `docs/screenshots/host.png`, `docs/screenshots/wall.png`
