# Phase 11 deferred items

- ~~**README screenshot of /host (from 11-06).**~~ **Closed in 11-07.**
  `web/scripts/readme-images.mjs` now shoots `/host` from the demo fixture into
  `docs/screenshots/host.png`, and README's "A look around" shows it. Only that
  image was committed: the script re-renders every picture, and the others were
  put back, so they still show the navigation without the Host entry and the
  version label of the build they were taken from. Re-rendering them all is a
  separate, cosmetic change.
