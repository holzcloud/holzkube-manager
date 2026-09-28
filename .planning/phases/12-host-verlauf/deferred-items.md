# Phase 12 — deferred items

Found during execution, outside the plan that found them. Not fixed there.

## From 12-01 (2026-09-28)

- **`./bin/task lint:web` is red on files 12-01 did not touch.** Biome reports
  `web/src/components/DataTable.tsx:137` `lint/complexity/noUselessFragments`,
  and `web/src/routes/wall.test.tsx:81/83` `suppressions/unused` +
  `lint/suspicious/noExplicitAny`. Both files are unchanged since before 12-01
  (base 8246ab9); the three files 12-01 changed pass `biome check`. Whoever
  next runs `./bin/task ci` before a push meets these first.

## From 12-07 (2026-09-28)

- **Text below 12 px that is not `text-xs` still carries the body's -0.24px
  tracking.** 12-07 fixed the word space for the `text-xs` utility
  (`--text-xs--letter-spacing: normal`); arbitrary sizes do not read that
  theme variable. Seen in a 1280×844 render of the wall: the namespace aside
  ("each tile takes the colour of the worst workload in it", `text-[0.7rem]
  md:text-[1.4vmin]`) and the legend ("switched off on purpose") read with
  collapsed spaces. The same sizes: `wall.tsx` 407/578/583/594
  (`text-[0.7rem]`), `Sidebar.tsx` 263/277, `Alpha.tsx` 22, `LiveChart.tsx`
  130/139/145 (`text-[10px]`/`text-[11px]`), `upgrades.tsx` 385/557,
  `provision.tsx` 216, `images.tsx` 1714. A fix is `tracking-normal` on those
  elements (or a smaller token that sets its own letter-spacing), held the way
  `typography.browser.test.tsx` holds `text-xs`; the wall's fit is sensitive,
  so it wants the layout audit after.
