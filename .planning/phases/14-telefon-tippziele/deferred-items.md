# Phase 14 — deferred items (found during execution, out of scope)

## 14-01

- **Duplicate `id="new-password"` on /settings.** The change-password card
  (`web/src/routes/settings.tsx:152`) and the New account form of AccountsCard
  (`web/src/routes/accounts.tsx:357`) both render `id="new-password"` on the same
  page. Each `<Label htmlFor="new-password">` therefore points at whichever the
  browser finds first, so one of the two labels names the wrong field for a
  screen reader. Found when the Sudo dialog opener's `#new-password` locator
  resolved to two elements; the opener now scopes to
  `form:has(#new-username) #new-password`. Not a tap-target change, so not
  fixed in Phase 14.

## 14-07

- **`src/routes/login.test.tsx` "links to the address the local account works
  on" times out under the full gate's load.** In the first `./bin/task ci` of
  14-07 on the Pi it failed after 1457 ms with `Unable to find role="link"`:
  `findByRole` keeps testing-library's default 1000 ms timeout, and the router
  had not rendered the link yet with the whole jsdom project running beside it.
  The same file alone passed 3 of 3 runs. Neither the test nor `login.tsx`
  changed in Phase 14. Not fixed here; a longer `timeout` on that one
  `findByRole` would be the candidate, after seeing it red under load again.
