# Phase 12 — deferred items

Found during execution, outside the plan that found them. Not fixed there.

## From 12-01 (2026-09-28)

- **`./bin/task lint:web` is red on files 12-01 did not touch.** Biome reports
  `web/src/components/DataTable.tsx:137` `lint/complexity/noUselessFragments`,
  and `web/src/routes/wall.test.tsx:81/83` `suppressions/unused` +
  `lint/suspicious/noExplicitAny`. Both files are unchanged since before 12-01
  (base 8246ab9); the three files 12-01 changed pass `biome check`. Whoever
  next runs `./bin/task ci` before a push meets these first.
