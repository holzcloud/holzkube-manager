# `internal/store/testdata`

Data-directory fixtures for the schema-migration tests in
`internal/store/migrate`. Each subdirectory is a data directory frozen in one
state that `migrate.Run` must handle. A test never runs against these
directories in place — it copies one into `t.TempDir()` first, because `Run`
writes `VERSION` and may create `backups/`.

| Fixture | State on disk | What it pins |
|---|---|---|
| `version-current/` | `VERSION` = `CurrentVersion` | `Run` is a no-op: no backup, no rewrite. Upgrading a binary that changed nothing must not churn the operator's data directory. |
| `version-previous/` | `VERSION` = `0` | Refusal to start on a version no release ever wrote. `readVersion` rejects it rather than passing it to the migration loop, which used to skip every step and then stamp the directory as fully migrated. |
| `version-future/` | `VERSION` = `CurrentVersion + 1` | Refusal to start. A newer holzkube-manager wrote this directory; an older binary that "helpfully" proceeded would silently downgrade records it does not understand. Nothing is written and no backup is created. |
| `version-garbage/` | `VERSION` = `not-a-number` | Refusal to guess. An unreadable version is not version 0 and not the current version; either assumption risks running the wrong migrations against real data. |
| `version-1/` | `VERSION` = `1`, one user record | The 1 -> 2 migration under the real `Run`: the `schematics/` directory is created, the existing record survives, and the pre-migration tarball does *not* contain `schematics/` — which is what proves the backup was taken before the migration ran rather than after. |
| `legacy-no-version/` | no `VERSION`, one user record | A data directory written by holzkube-manager before `VERSION` existed (phase 1, plan 01). It is by definition at version 1, not version 0. Since phase 2 that makes it one version behind, so it is migrated and backed up like any other version-1 directory. |

An empty data directory has no fixture: `t.TempDir()` already is one. It is the
case where `Run` writes `VERSION` and deliberately creates no backup, because
there is nothing to lose.

The upgrade path's *failure* branches have no fixture of their own. `migrate.run`
takes the target version as a parameter, so those tests start from
`version-current/` and advance it to `CurrentVersion + 1` with a table supplied
by the test. That is how the path was proven before any real migration existed,
and it is still how a migration that explodes, and a target no migration
reaches, are driven — without requiring a data directory to claim a version the
code says cannot exist. The *successful* path now has a real fixture too:
`version-1/`, migrated by the shipped table.

Version `0` therefore appears only as something to refuse. `readVersion`'s doc
comment has always said that no release wrote a version 0 layout; the parser
now agrees with it.
