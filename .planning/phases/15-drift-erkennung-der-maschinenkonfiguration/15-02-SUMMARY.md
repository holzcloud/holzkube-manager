---
phase: 15-drift-erkennung-der-maschinenkonfiguration
plan: 02
subsystem: store
tags: [drift, baseline, fsstore, schema-migration, guide]
requires: []
provides:
  - "model.ConfigBaseline / ExpectedConfig und die Konstanten BaselineFormat, BaselineFirstSight, BaselineApply, BaselineConfirmed"
  - "store.ConfigBaselineStore und Store.ConfigBaselines() (Get, List, Put, Delete ueber model.MachineID)"
  - "fsstore: Entity config-baselines/, ein Datensatz je Knoten, Rev-CAS, 0600 in 0700"
  - "Schema 7: Migration 6 auf 7 legt config-baselines/ leer an; Fixture version-6"
affects: [15-03, 15-04, 15-05, 15-06, 15-07, 15-08, 15-10]
actuals:
  tokens: 14500
  tasks: 3
  commits: 3
plan_head_before: 6612ff55762c9df291fc74266337c1958c592bb5
plan_head_after: aac2678eb4e2b0f16d11b1da8c4b2d4901fbf812
tech-stack:
  added: []
  patterns: ["Entity ueber den generischen recordStore", "Schema-Migration legt ein Verzeichnis an und fuellt nichts nach"]
key-files:
  created:
    - internal/model/configdrift.go
    - internal/store/fsstore/configbaselines_test.go
    - internal/store/testdata/version-6/VERSION
    - internal/store/testdata/version-6/users/alice.json
  modified:
    - internal/store/store.go
    - internal/store/fsstore/fsstore.go
    - internal/store/fsstore/inventory.go
    - internal/store/migrate/migrate.go
    - internal/store/migrate/migrate_test.go
    - internal/store/testdata/version-current/VERSION
    - internal/store/testdata/version-future/VERSION
    - internal/store/testdata/README.md
    - docs/guide.md
key-decisions:
  - "D-08 umgesetzt: CurrentVersion 7, Migration 6 auf 7; die Folge fuer ein zurueckgerolltes Binary steht im Leitfaden"
  - "AppliedOnDriftAt (D-13) ist schon im Format, weil eine spaetere Ergaenzung eine Migration der gespeicherten Datensaetze waere"
requirements-completed: []
status: complete
---

# Phase 15 Plan 02: Platz der Basis im Datenverzeichnis Summary

Die Basis einer Knotenkonfiguration hat ihren Platz: ein Entity `config-baselines/` (ein Datensatz je Knoten, atomar, mit Rev-Pruefung, 0600 in 0700), ein Datenverzeichnis-Schema 7 mit Migration 6 auf 7 und im Leitfaden der Weg zurueck nach einem Rollback des Binarys. Geschrieben wird noch keine Basis (Plan 03).

BASE: 6612ff55762c9df291fc74266337c1958c592bb5

`plan_head_before` ist derselbe Wert, es lag nichts dazwischen. `git rev-list --count BASE..HEAD` vor diesem SUMMARY-Commit: 3.

## Wo es lief

Mac (darwin/arm64), `go1.27.2`. Kein echter Cluster, kein Pi, kein `task`. `go test -race ./internal/store/...` lief hier gruen; auf dem Pi geht das nicht, CI bleibt massgeblich. Nichts gepusht, nichts getaggt, kein Changelog, nichts installiert.

## Was gebaut wurde

- `internal/model/configdrift.go` (3c8c519): `ConfigBaseline`, `ExpectedConfig`, die vier Konstanten, mit einem Doc-Kommentar je Feld. Der Typkommentar sagt, dass `Expected` nie in place geaendert wird.
- `internal/store/store.go`, `fsstore/fsstore.go`, `fsstore/inventory.go` (3c8c519): `ConfigBaselineStore`, `Store.ConfigBaselines()`, `kindConfigBaselines`, `configBaselineStore` ueber `recordStore[model.MachineID, model.ConfigBaseline]`, Verzeichnis in der Schleife von `Open`. Am generischen `recordStore` ist nichts geaendert.
- `internal/store/fsstore/configbaselines_test.go` (3c8c519): der Tracer `TestAConfigBaselineRoundTripsAndSurvivesARestart` (alle Felder, Zeiten und der Zeiger `Expected` ueber Neustart, `List`), dazu stale Rev, Rev fuer einen fehlenden Datensatz, `TestADamagedBaselineIsNotAMissingOne`, 0600/0700, Schluessel `../x` und leer, Delete fehlend und `TestOpenCreatesTheConfigBaselinesDirectory` (siehe Abweichungen).
- `internal/store/migrate/migrate.go` (aeca2da): `CurrentVersion = 7`, Eintrag `From: 6, To: 7` mit Kommentar zu D-08 und `ErrVersionTooNew`. Der Rest der Tabelle ist unberuehrt.
- `internal/store/migrate/migrate_test.go`, Fixtures (aeca2da): `TestMigrateVersionSixGainsConfigBaselines` (VERSION als Zahl 7, Modus 0700, leer, `alice.json` ueberlebt, Sicherung `pre-migration-6-to-7-*` enthaelt `config-baselines` nicht); `version-6/` (VERSION 6 ohne Zeilenumbruch, `users/alice.json` als Kopie aus `version-5`), `version-current` 7, `version-future` 8.
- `docs/guide.md`, `internal/store/testdata/README.md` (aac2678): Zeile `config-baselines/` in der Verzeichnisliste, Unterabschnitt "Schema 7 and going back" (Schema 6 auf 7, Sicherung `backups/pre-migration-6-to-7-<Zeit>.tar.gz`, Fehlermeldung des alten Binarys, der Update-Timer rollt Binary und nicht Verzeichnis, zwei Wege heraus), ein Verweissatz im Absatz ueber das Zurueckrollen unter "Updating itself every hour", die Zeile `version-6/` in der Fixture-Tabelle. Der Verweis auf den Drift-Abschnitt (Plan 10) fehlt, wie der Plan es fuer die Zeit davor verlangt.

## Rot gesehen

Alle Exitcodes sind die des `go test`-Befehls selbst (kein Pipe-Exit). Jede Injektion: genau eine Aenderung am Produktionscode, vorher `go build ./... && go vet ./internal/store/...` mit Exit 0, Wiederherstellung per `cp` und `cmp -s` (Exit 0), danach gruen.

ROT vor jedem Eingriff:

| Task | Lauf | Exit | scheiternde Zeile |
|---|---|---|---|
| 1 | Tests gegen den Stand ohne Produktionscode | 1 | `[build failed]`: `undefined: model.ConfigBaseline`, `Store has no field or method ConfigBaselines` (Compile-Fehler, vom Plan als Rot zugelassen) |
| 2 | `TestMigrateVersionSixGainsConfigBaselines` ohne Fixture | 1 | `copy fixture version-6: lstat ../testdata/version-6: no such file or directory` |
| 2 | derselbe Test mit Fixture, vor dem Code | 1 | `VERSION = "6", want 7` |

Task 1, Injektionen:

| Injektion | Exit | scheiternde Zeilen |
|---|---|---|
| (a) `setRev` des Entities schreibt nichts | 1 | `TestAConfigBaselineRoundTripsAndSurvivesARestart`: `first Put returned rev 0, want 1`; `TestAConfigBaselinePutRefusesAStaleRevision`: `second Put returned rev 0, want 2` |
| (b) `s.configBaselines.dir` fehlt in der Schleife von `Open` | 1 | `TestADamagedBaselineIsNotAMissingOne`: `plant the damaged file: ... no such file or directory`; `TestOpenCreatesTheConfigBaselinesDirectory`: `Open did not create config-baselines: stat ...: no` |
| (c, zusaetzlich) `recordStore.Put` behandelt jeden Lesefehler wie "fehlt" (`... \|\| err != nil`) | 1 | `TestADamagedBaselineIsNotAMissingOne`: `Put over a damaged baseline returned no error` |

Task 2, Injektionen (alle in `migrate.go`):

| Injektion | Exit | scheiternde Zeilen |
|---|---|---|
| (a) der Eintrag `From: 6` fehlt, `CurrentVersion` bleibt 7 | 1 | `TestMigrateVersionSixGainsConfigBaselines`: `no migration path from version 6 to 7`; `TestMigrationsAreOrderedAndForwardOnly`: `the last migration ends at 6 but CurrentVersion is 7`; ausserdem die vier Tests der aelteren Fixtures (Legacy, 1, 2, 5), weil auch deren Pfad bis 7 reicht |
| (b) Verzeichnis mit `0o755` angelegt | 1 | `config-baselines is 0755, want 0700` |
| (c) die Migration legt zusaetzlich `invented.json` an | 1 | `the migration created 1 baselines. A baseline exists only because a node was read, so anything written here is invented` |
| (e, zusaetzlich) `CurrentVersion` bleibt 6, der Eintrag 6 auf 7 steht | 1 | sieben Tests rot, darunter `VERSION = "6", want 7` und `the last migration ends at 7 but CurrentVersion is 6` |

Alle Injektionen haben tatsaechlich injiziert und nicht nur den Build gebrochen.

## Verifikation

- `go test ./internal/store/... ./internal/model/ ./internal/planning/ -count=1`: alle ok. `go test -race ./internal/store/...`: ok (nur Mac).
- `go test ./internal/config/ ./internal/host/... ./internal/publicrepo/ -count=1`: ok. `TestNoDirectFileAccessOutsideFsstore`: ok.
- `go build ./...`, `go vet ./...`, `gofmt -l internal/store internal/model`: sauber.
- CI-Linter `GOTOOLCHAIN=go1.26.9 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1 run ./...`: `0 issues.`
- Guard gegen BASE: `git diff --name-only 6612ff5... -- deploy internal/changelog` ist leer.
- Akzeptanz: `config-baselines/` kommt 3 Mal in `docs/guide.md` vor, "Schema 7 and going back" einmal, `version-6/` in der Fixture-README; VERSION-Dateien 7, 8, 6.
- `go test ./...` insgesamt: nur die vier bekannten `internal/httpapi`-Tests (`TestCSPHashMatchesTheServedShell`, `TestForeignHostIsRefused`, `TestNoAssetIsInlinedPastTheContentSecurityPolicy`, `TestFontsAreServedAsFiles`) scheitern, weil `web/dist` fehlt. Nicht dieser Plan.

## Abweichungen vom Plan

**1. [Plan-Annahme falsch] Injektion 1(b) macht den Roundtrip-Test nicht rot.** Der Plan sagt voraus, dass ohne Eintrag in der Verzeichnisschleife der Roundtrip-Test rot wird ("das Schreiben scheitert am fehlenden Verzeichnis"). Das stimmt nicht: `writeAtomic` legt das Verzeichnis selbst an (`os.MkdirAll`, `atomic.go:79`). Im ersten Lauf der Injektion wurde nur `TestADamagedBaselineIsNotAMissingOne` rot, und nur, weil es die Datei per `os.WriteFile` selbst hinlegt. Der Beitrag von `Open` war damit nur zufaellig bewacht. Fix: `TestOpenCreatesTheConfigBaselinesDirectory` (Verzeichnis nach `Open` da, 0700, `List` leer ohne Fehler) und die Injektion wiederholt; jetzt rot an einer Zeile, die dafuer gedacht ist. Das ist die Zeile "(b)" in der Tabelle oben. Commit 3c8c519.

**2. [Rule 2] Zusaetzliche Injektionen (1c, 2e).** 1(c) belegt, dass `TestADamagedBaselineIsNotAMissingOne` tatsaechlich die Put-Seite des Satzes haelt, auf dem Plan 03 beruht. 2(e) belegt, dass eine vergessene Erhoehung von `CurrentVersion` nicht still gruen bleibt.

**3. Kein Eingriff in Rollback-Wording ueber den Plan hinaus.** Im Leitfaden steht, dass `restore` den Stand vor der Migration wiederherstellt und dass Aenderungen seither fuer die im Backup enthaltenen Dateien weg sind. Das ist aus `backup.Restore` gelesen ("unpacks over"), nicht ausgefuehrt.

## Auth gates

Keine.

## Known Stubs

Keine. `ConfigBaselines()` hat noch keinen Aufrufer ausser den Tests; das ist der Zuschnitt des Plans (Persistenz zuerst, Plan 03 schreibt die Basis), kein Platzhalter im Code.

## Was NICHT belegt ist

- Das tatsaechliche Rollback-Verhalten des Update-Skripts auf dem Pi: aus `deploy/holzkube-manager-update.sh` (Rollback ab Zeile 440 tauscht `$BIN`, nicht das Verzeichnis) gelesen, nicht ausgefuehrt. Der Leitfaden-Satz "ein zurueckgerolltes Binary startet nicht" ist eine Folgerung aus `ErrVersionTooNew` und dem Skript, nicht beobachtet.
- Dass `holzkube-managerd restore` mit dem alten Binary die Pre-Migration-Sicherung sauber einspielt und der Dienst danach startet: nicht ausgefuehrt. Der Unterabschnitt beschreibt es aus dem Code von `backup.Restore`.
- Die Injektion "Sicherung entsteht nach statt vor der Migration" ist fuer 6 auf 7 nicht eigens gemacht; die Aussage steht in `TestMigrateVersionSixGainsConfigBaselines` (Sicherung enthaelt `config-baselines` nicht), die zugrundeliegende Reihenfolge in `run` wurde in diesem Plan nicht angefasst und die aelteren Fixture-Tests halten sie.
- Die Injektion, dass `readJSON` einen Decode-Fehler als `ErrNotFound` meldet, wurde nicht gemacht (`atomic.go` gehoert nicht zu diesem Plan). Die Get-Seite von `TestADamagedBaselineIsNotAMissingOne` ist deshalb ohne eigene Rot-Messung; die Put-Seite ist durch 1(c) gemessen.
- Die Digest-Pruefung gegen eine inhaltlich veraenderte, aber gueltige Datei (T-15-04, zweiter Teil) macht Plan 03.
- Kein Pi, kein echter Cluster; `-race` nur auf dem Mac gelaufen, CI bleibt massgeblich.
- `docs/guide.md`: die Verzeichnisliste fuehrt `machine-classes/` weiterhin nicht auf (vorher schon so, nicht dieser Plan, nicht angefasst).

## Threat Flags

Keine neue Angriffsflaeche ausser der im Plan geplanten (T-15-04 bis T-15-07 mitigiert bzw. akzeptiert wie dort).

## Self-Check: PASSED

Dateien vorhanden (`configdrift.go`, `configbaselines_test.go`, `version-6/VERSION`, `version-6/users/alice.json`); Commits 3c8c519, aeca2da, aac2678 sind Vorfahren von HEAD.
