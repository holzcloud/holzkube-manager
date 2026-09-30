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
