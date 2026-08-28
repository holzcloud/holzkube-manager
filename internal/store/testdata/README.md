# `internal/store/testdata`

Data-directory fixtures for the schema-migration tests in
`internal/store/migrate`. Each subdirectory is a data directory frozen in one
state that `migrate.Run` must handle. A test never runs against these
directories in place — it copies one into `t.TempDir()` first, because `Run`
writes `VERSION` and may create `backups/`.

| Fixture | State on disk | What it pins |
|---|---|---|
| `version-current/` | `VERSION` = `CurrentVersion` | `Run` is a no-op: no backup, no rewrite. Upgrading a binary that changed nothing must not churn the operator's data directory. |
| `version-previous/` | `VERSION` = `CurrentVersion - 1` | The upgrade path: a pre-migration tarball is written *before* the first change, and `VERSION` is advanced only after the last migration succeeds. |
| `version-future/` | `VERSION` = `CurrentVersion + 1` | Refusal to start. A newer holzkube wrote this directory; an older binary that "helpfully" proceeded would silently downgrade records it does not understand. Nothing is written and no backup is created. |
| `version-garbage/` | `VERSION` = `not-a-number` | Refusal to guess. An unreadable version is not version 0 and not the current version; either assumption risks running the wrong migrations against real data. |
| `legacy-no-version/` | no `VERSION`, one user record | A data directory written by holzkube before `VERSION` existed (phase 1, plan 01). It is by definition at version 1, not version 0, so no migration and no backup run. |

An empty data directory has no fixture: `t.TempDir()` already is one. It is the
case where `Run` writes `VERSION` and deliberately creates no backup, because
there is nothing to lose.

Version `0` exists only in `version-previous/`. No holzkube release ever wrote
it. It is here so that the entire upgrade path — backup, ordered application,
deferred `VERSION` write, failure handling — is executed by tests in phase 1,
while there is still exactly one real schema version and therefore no real
migration to hide behind. Phase 3 appends the first genuine migration to a
path that has already been proven.
