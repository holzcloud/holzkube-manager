# Deferred items, Phase 17

Found while executing plan 17-01; not caused by it, not fixed (scope boundary).

- `go test ./internal/httpapi/` `TestNoAssetIsInlinedPastTheContentSecurityPolicy` fails at BASE b3c55ef as well: the embedded built stylesheet inlines a `data:` font while the CSP `font-src` is `'self'`. Needs a web rebuild or `build.assetsInlineLimit`; no Go change of this phase touches it.
- `go test ./internal/publicrepo/` `TestNoIdentifierOfTheRealInstallationIsInTheHistory` names commits 35675c0, 41eec4c, d94e372, 0d8e7ed, b75b44d, d48ea05, c35ec03. None is in `b3c55ef..HEAD`. Two were traced (35675c0 and 41eec4c): they sit on the local branches `backup/local-main-2026-09-05` and `pre-public-rewrite`; the others were not traced. The operator decides whether those local branches are deleted.
