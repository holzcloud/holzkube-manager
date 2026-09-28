# Phase 11 deferred items

- **README screenshot of /host (from 11-06).** CLAUDE.md asks for a screenshot in
  "A look around" when a feature is a screen of its own. 11-06 added the README
  bullet; the screenshot needs `/host` added to `web/scripts/readme-images.mjs`
  and `task build && node web/scripts/readme-images.mjs`, which re-renders every
  image in docs/screenshots and docs/brand. Better done once at phase end, after
  11-07, than mid-phase. The demo fixture already carries the full /host shape.
