# Phase 12 — deferred items

Found during execution, outside the plan that found them. Not fixed there.

## From 12-01 (2026-09-28)

- **Corrected in 12-08: `./bin/task lint:web` is not red.** This entry first
  said it failed. Measured by the orchestrator and again in 12-08's gate, it
  exits 0: Biome reports `web/src/components/DataTable.tsx:137`
  `lint/complexity/noUselessFragments` and `web/src/routes/wall.test.tsx:81/83`
  `suppressions/unused` + `lint/suspicious/noExplicitAny` as **warnings**, not
  errors (12-04 saw the same: 2 warnings and 1 info, exit 0). They are still
  worth cleaning up, but nothing blocks a push on them.

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

## From 12-08 (2026-09-29)

- **`TestEveryProblemCodeIsInTheContract` only sees the `Code*` constants in
  `internal/httpapi/problem.go`.** A code passed as a string literal --
  `httpapi.Upstream("upstream.history-unavailable", …)` and the other
  `…-unavailable` codes in `internal/httpapi/handlers` -- is invisible to it.
  Seen in 12-08: renaming `upstream.history-unavailable` everywhere in the
  contract left the test green (exit 0); renaming a constant's code
  (`validation.fingerprint-mismatch`) turned it red. A literal scan over
  handlers finds 17 codes not named in `docs/api-contract.md`, among them
  `upstream.inventory-unavailable`, `upstream.jobs-unavailable`,
  `upstream.config-unavailable`, `notfound.user`, `notfound.job`,
  `conflict.username-taken` and `oidc.provider-unreachable`.
  `upstream.history-unavailable` and `upstream.host-unavailable` are named.
  A fix is to have `problemCodes` also collect the first argument of every
  `httpapi.<Constructor>("…")` call outside tests, and to document the codes
  it then finds -- a contract change of its own, not this phase's.
