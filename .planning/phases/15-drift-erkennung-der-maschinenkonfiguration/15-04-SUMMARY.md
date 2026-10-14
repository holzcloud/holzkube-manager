---
phase: 15-drift-erkennung-der-maschinenkonfiguration
plan: 04
subsystem: configdrift
tags: [drift, confirm, forget, reconcile, poke, deadline, ledger-137]
requires: [15-01, 15-02, 15-03]
provides:
  - "internal/configdrift: Confirm mit ErrNotRead, ErrNoDrift, ErrDigestChanged (und ErrUnknownMachine), Forget, reconcile, Poke, Flush, Run mit Takt und Anstößen"
  - "Testaufbau: inventory (austauschbare Listen), withInventory, withAllMachines, fakeReader.count/limit/read, countingReader, drifting, barrier"
affects: [15-05, 15-06, 15-07, 15-08, 15-10]
actuals:
  tokens: 13500
  tasks: 3
  commits: 3
plan_head_before: 93154867f4e520b520cdddde973d4a32e4251da1
plan_head_after: 1b0f0d46c08038bca1b0d1dbf0d729c08d381ffe
tech-stack:
  added: []
  patterns: ["Compare-and-Swap über den Digest der gesehenen Lesung, zweimal geprüft (vor und in der Sperre)", "Aufräumen im Lauf statt Haken, löscht nur bei zwei gelesenen Listen", "Anstoß: nicht blockierender Puffer, Flush mit Budget je Knoten"]
key-files:
  created:
    - internal/configdrift/lifecycle.go
    - internal/configdrift/confirm_test.go
    - internal/configdrift/lifecycle_test.go
  modified:
    - internal/configdrift/configdrift.go
    - internal/configdrift/decide.go
    - internal/configdrift/detail.go
    - internal/configdrift/helpers_test.go
key-decisions:
  - "Aufräumen im Lauf statt Haken in ForgetMachine/ForgetCluster: ein Abgleich am Anfang jedes Tick deckt Vergessen, Umhängen und eine wiederhergestellte Sicherung mit einer Regel ab"
  - "Kein Wiederherstellen einer beschädigten Basis über Confirm: Confirm bestätigt nur eine echte Drift und überschreibt nie eine Basis, der damageOf misstraut; der Weg heraus ist Datei löschen (oder Forget), die nächste Lesung legt eine neue an"
  - "Confirm prüft die Lesung zweimal: vor storeMu (ohne Warten) und in storeMu (gegen zwei gleichzeitige Bestätigungen)"
  - "Flush liest über dieselbe Funktion wie Refresh (readNode), das Budget je Knoten steht an genau einer Stelle"
requirements-completed: []
status: complete
---

# Phase 15 Plan 04: Der Lebenslauf der Basis Summary

Das Paket `internal/configdrift` kann jetzt „so ist es gewollt“ bestätigen (der gelesene Stand wird die Basis, gemessen ohne Lesen und ohne Schreiben am Knoten, nur gegen den Digest, den der Betreiber gesehen hat), eine Basis vergessen, am Anfang jedes Laufs Basen verschwundener oder umgezogener Maschinen aufräumen (nie bei einer ungelesenen Liste) und einen Knoten außer der Reihe lesen, mit eigenem Budget auch auf dem Weg Poke, Flush, Lesen.

BASE: 93154867f4e520b520cdddde973d4a32e4251da1

`plan_head_before` ist derselbe Wert. `git rev-list --count BASE..HEAD` vor diesem SUMMARY-Commit: 3 (276e723, ced36aa, 1b0f0d4).

## Wo es lief

Mac (darwin/arm64), `go1.27.2`. Kein echter Cluster, kein Pi, kein `task`. `go test -race ./internal/configdrift/ -count=1` lief hier grün, dazu `-race -count=10` für `TestTwoConfirmations`, `TestPoke*` und `TestAPoked*`; auf dem Pi geht das nicht, CI bleibt maßgeblich. Der CI-Linter (`GOTOOLCHAIN=go1.26.9 ... golangci-lint@v2.13.1 run ./...`) meldet `0 issues.` (der erste Lauf fand drei: ein ungenutzter Parameter, zwei ungenutzte Testoptionen; behoben). Nichts gepusht, nichts getaggt, kein Changelog, nichts installiert. Die vier `internal/httpapi`-Tests, die `web/dist` brauchen, sind nicht untersucht und nicht meine.

## Was gebaut wurde

- `lifecycle.go` (276e723, ced36aa, 1b0f0d4):
  - `ErrNotRead`, `ErrNoDrift`, `ErrDigestChanged` mit Sätzen, die Lage und nächsten Schritt nennen.
  - `Confirm(ctx, id, digest)`: Maschine über `AllMachines` (`ErrUnknownMachine`), erster Blick auf die Lesung außerhalb der Sperre, zweiter in `storeMu`; Reihenfolge `ErrNotRead` (keine Lesung oder `unreadable`), `ErrNoDrift` (alles außer `drift`), `ErrDigestChanged` (nicht 64-stelliger Kleinbuchstaben-Hex oder ungleich dem Digest der letzten guten Lesung). Geschrieben wird Source `confirmed`, Digest und Text der Lesung, `TakenAt` jetzt, Machinery der laufenden Maschinerie, Cluster der Maschine, ohne `DriftSince`, `AppliedOnDriftAt`, `Expected`; Rev des geladenen Datensatzes, bei fehlendem Datensatz Rev 0, bei `ErrConflict` bis zu dreimal neu. Danach geht die Lesung im Speicher in einem Zug auf `ok` (`markConfirmed` unter einem `mu`-Halt: eine inzwischen gescheiterte Lesung behält ihren Zustand, nur der Beginn der Drift fällt weg).
  - `Forget`, `reconcile`, `Poke` (Puffer 64, voller Puffer verwirft), `Flush` und `flush` (Anstöße ohne Doppelte, aufgelöst über `AllMachines` und `Clusters`; unbekannt, ohne Cluster, `Disabled` am Knoten oder am Cluster wird verworfen; gelesen über `readNode`, also mit dem Budget je Knoten).
- `configdrift.go`: `pokes`-Kanal, `Run` mit `select` über Kontext, Takt (`Tick`, dann `Flush`, Timer mit `Every` neu) und Anstöße; `Tick` ruft `reconcile(ctx)` vor dem Lesen.
- `decide.go`: `set` zerlegt in `put` (unter gehaltenem `mu`, meldet den Zustandswechsel) und `set`, damit `markConfirmed` atomar arbeiten kann; kein Verhalten geändert.
- Tests: `confirm_test.go` (Tracer, die vier Verweigerungen, der gleichzeitige Fall, beschädigte Basis, verschwundener Datensatz), `lifecycle_test.go` (Forget, Aufräumen in vier Formen, Disabled, Poke, Run, Frist), `helpers_test.go` erweitert.
- Gegen BASE unberührt: `git diff --name-only $BASE -- internal/machineconfig internal/store internal/certwatch internal/changelog internal/talos internal/inventory` ist leer (auch nach den Injektionen in `snapshot.go`, siehe unten).

## Rot gesehen

Alle Exitcodes sind die des `go test`-Befehls selbst (in Dateien umgeleitet). Jede Injektion: genau eine Änderung am Produktionscode (`inject.sh` verlangt, dass die Ersetzung genau einmal trifft), vorher `go build ./... && go vet ...` mit Exit 0, Wiederherstellung per `cp` und `cmp -s` mit Exit 0, danach grün. Zwei Injektionen haben beim ersten Versuch nur den Build gebrochen (3(h2) `declared and not used: disabled`, 3(i2) `"internal/talos" imported and not used`); das ist keine Messung, beide wurden mit einer Fassung wiederholt, die baut, nur die Wiederholung zählt.

### ROT vor jedem Eingriff

| Task | Lauf | Exit | scheiternde Zeile |
|---|---|---|---|
| 1 | Tracer ohne `Confirm` | 1 | `svc.Confirm undefined (type *configdrift.Service has no field or method Confirm)`, `[build failed]` (Compile-Fehler, vom Plan zugelassen) |
| 2 | die neuen Tests gegen den Stand nach Task 1 | 1 | `TestConfirmDoesNotRestoreADamagedBaseline/a_changed_snapshot_under_the_old_digest`: `Confirm over a damaged baseline succeeded`; `TestConfirmRefusesADigestThatIsNotTheReadings/a_reading_that_has_no_digest...`: `Confirm("") returned <nil>, want ErrDigestChanged`; `TestTwoConfirmationsOfTheSameStateAreOneConfirmation`: `2 confirmations succeeded and 0 were refused with ErrNoDrift` und `the record is at rev 4, want 3: it was written more than once`, in 30 von 30 Läufen bei `-count=30` |
| 3 | alle neuen Tests gegen den Stand nach Task 2 | 1 | `d.svc.Forget undefined`, `svc.Poke undefined`, `svc.Flush undefined`, `[build failed]` |

Schon grün gegen den Stand nach Task 1 und deshalb **nicht** als Rot gezählt: `TestConfirmRefusesAChangedReading`, `...WhatWasNeverRead`, `...AReadingThatIsNoLongerCurrent`, `...WhenThereIsNothingToConfirm`, `...AMachineThatDoesNotExist`, der Hauptteil von `...ADigestThatIsNotTheReadings`, `TestConfirmWhenTheRecordWentAway`. Sie sind nur über ihre Injektionen belegt. Ein erster Lauf von `TestTwoConfirmations` scheiterte an meinem Testfehler (die Maschinenliste des Tests war während des Aufbaus noch leer: `Detail: configdrift: no such machine`); behoben, die Zahlen oben sind die nach der Behebung.

### Task 1, Injektionen (gegen die Fassung 276e723)

| Injektion | Exit | scheiternde Zeilen |
|---|---|---|
| (a) `Confirm` liest den Knoten vor dem Schreiben (`Deps.Read`) | 1 | `Confirm read the node: 2 reads before, 3 after`; `the next tick read the node 2 times, want 1` |
| (b) `DriftSince` bleibt im Datensatz stehen | 1 | `DriftSince is 2026-10-14 14:07:35 ... in the record, want empty`; `a reading of the confirmed state rewrote the baseline` |
| (c) die Lesung im Speicher wird nicht aktualisiert | 1 | `state after confirming is "drift" (...), want ok`; `drift_since is ... after confirming, want empty` |

### Task 2, Injektionen (gegen die Fassung ced36aa)

| Injektion | Exit | scheiternde Zeilen |
|---|---|---|
| (a) Digest-Prüfung entfällt (`case false`) | 1 | `TestConfirmRefusesAChangedReading`: `Confirm with the digest of the old reading returned <nil>, want ErrDigestChanged`; `TestConfirmRefusesADigestThatIsNotTheReadings`: alle acht Digests (`empty`, `short`, `another valid digest`, `upper case`, `a prefix`, ...) ohne `ErrDigestChanged`, `a refused Confirm wrote the baseline` |
| (a2, zusätzlich) nur die Formatprüfung entfällt, die Gleichheit bleibt | 1 | nur `.../a_reading_that_has_no_digest_is_not_confirmed_by_an_empty_one`: `Confirm("") returned <nil>, want ErrDigestChanged`. Die Formatprüfung ist sonst durch die Gleichheit gedeckt; messbar ist sie nur an einem Leser, der einen Schnappschuss ohne Digest liefert |
| (b) Prüfung „keine Drift“ entfällt | 1 | `TestConfirmRefusesWhenThereIsNothingToConfirm/ok`: `Confirm in state "ok" returned <nil>, want ErrNoDrift` und `a refused Confirm wrote the baseline`; ebenso `baseline-created`; `baseline-damaged` bekommt statt `ErrNoDrift` den Ladefehler; dazu `TestTwoConfirmations...` |
| (c) Prüfung „aktuell gelesen“ entfällt (`case false`) | 1 | `TestConfirmRefusesAReadingThatIsNoLongerCurrent`: `returned configdrift: this node does not differ from its baseline, ... want ErrNotRead`; `TestConfirmRefusesWhatWasNeverRead` beide Fälle ebenso |
| (d) erneute Prüfung in der Sperre entfällt | 1 | `2 confirmations succeeded and 0 were refused with ErrNoDrift, want 1 and 1: [<nil> <nil>]`; `the record is at rev 4, want 3: it was written more than once`. Bei `-count=50` rot in 50 von 50 Läufen |

Zusätzliche Injektionen (Rule 2, nicht vom Plan verlangt), je Exit wie angegeben, Wiederherstellung per `cmp` Exit 0:

| Injektion | Test | Exit | Ergebnis |
|---|---|---|---|
| (e) `damageOf` in `Confirm` entfällt | `TestConfirmDoesNotRestoreADamagedBaseline/a_changed_snapshot_under_the_old_digest` | 1 | `Confirm over a damaged baseline succeeded` |
| (f) ein Ladefehler zählt wie „fehlt“ | dieselbe, Variante `garbage` | **0, grün** | der Store lehnt das Schreiben über eine nicht lesbare Datei selbst ab (Plan 02, Injektion 1(c)); die Variante ist auf dieser Ebene **nicht** rot zu sehen, die Schicht darunter fängt sie |
| (g) ein fehlender Datensatz ist ein Fehler | `TestConfirmWhenTheRecordWentAway` | 1 | `Confirm: configdrift: the baseline of m1 cannot be loaded and is left as it is: store: record not found` |

### Task 3, Injektionen (gegen die Fassung 1b0f0d4)

| Injektion | Test | Exit | scheiternde Zeilen |
|---|---|---|---|
| (a) ein Fehler von `AllMachines` gilt als leere Liste | `TestReconcileNeverDeletesWhenTheMachineListFails` | 1 | `a baseline was deleted while the machine list could not be read: open .../config-baselines/gone.json: no such file or directory` |
| (b) `reconcile` entfällt in `Tick` | `TestABaselineOfAVanishedMachineIsRemoved`, `TestABaselineOfAMachineInAnotherClusterIsRemoved` (beide Unterfälle), dazu `TestAKnownMachinesBaselineIsKept` | 1 | `the baseline of a machine that is not in the inventory is still there`; `a machine that moved was compared with its old baseline: state "drift"`; die beiden `...NeverDeletes...`-Tests scheitern an der fehlenden Logzeile |
| (c) die Prüfung `Disabled` am Knoten entfällt (in `Refresh`) | `TestADisabledNodeIsNeverRead` | 1 | `the reader was called for [m1 m2], want only m1 (m2 is switched off)`; `a disabled node says "baseline-created"` |
| (c2, zusätzlich) dasselbe in `flush` | dieselbe | 1 | dieselben Zeilen |
| (d) `Poke` blockiert (`s.pokes <- id`) | `TestPokeNeverBlocks` | 1 | `Poke blocked: 2000 pokes with nothing reading them did not return within two seconds` |
| (e) `Run` ignoriert die Anstöße | `TestAPokedNodeIsReadWithoutWaitingForTheTick`, `TestAPokedNodeReachesARealNode...` | 1 | `a poked node was not read within five seconds, with the first tick an hour away`; `... within ten seconds` |
| (f) `Forget` lässt die Lesung im Speicher | `TestForgetDropsTheBaselineAndTheReading` | 1 | `after Forget Detail says "drift" (...), want not-read: the reading was left in memory` |
| (g) `reconcile` benutzt die Teilliste bei einem Fehler von `Baselines.List` | `TestReconcileNeverDeletesWhenTheBaselinesCannotBeListed` | 1 | `a baseline of a vanished machine was deleted on a partial list: stat .../gone.json: no such file or directory` |
| (h) die Prüfung `Disabled` am Cluster entfällt (in `Tick`) | `TestADisabledClusterIsNeverRead` | 1 | `the reader was called for [m1 m2], want only m1 (cluster c2 is switched off)` |
| (h2, zusätzlich) dasselbe in `flush` | dieselbe | 1 | dieselben Zeilen |
| (i) `flush` ruft den Leser ohne das Budget (Kontext des Aufrufers) | `TestAPokedNodeIsReadWithoutWaitingForTheTick` | 1 | `the reader was handed a context without a deadline on the way Poke, Flush, read; the Talos client refuses such a call`. Der Test mit dem echten Client blieb dabei **grün**: `ReadSnapshot` setzt die Klassenfrist selbst, eine Schicht fängt die andere (wie bei Plan 03 (h1)) |
| (i2, zusätzlich) wie (i) und die Klassenfrist in `machineconfig/snapshot.go` entfällt | `TestAPokedNodeReachesARealNodeFromAContextWithoutADeadline` und der Test mit dem Fake | 1 | `state "unreadable" (The node could not be read: talos: Get on m1: talos: refusing a call with no deadline ...), want baseline-created`; dazu die Zeile des Fakes |
| (j, zusätzlich) die Auslese doppelter Anstöße entfällt | `TestAPokedNodeIsReadWithoutWaitingForTheTick` | 1 | `the reader was called 2 times for two pokes before a reading, want 1` |

`internal/machineconfig/snapshot.go` wurde für (i2) angefasst und per `cmp -s` auf byte-gleich zurückgestellt (Exit 0); `git status --short` und `git diff --stat -- internal/machineconfig` sind leer.

## Verifikation

- `go test ./internal/configdrift/ ./internal/planning/ -count=1`: ok.
- Die elf benannten Tests von Task 3 (`-run 'TestForget|TestABaselineOf|TestAKnownMachinesBaselineIsKept|TestReconcileNever|TestADisabled|TestPoke|TestAPoked|TestAPokeFor'`): zwölf `--- PASS` (mit dem zusätzlichen Test für den echten Client), kein `--- SKIP`.
- `go test ./internal/configdrift/ -run 'TestConfirm|TestTwoConfirmations|TestTracerConfirming'`: alle PASS; `TestTwoConfirmations -count=30` und `-count=50` grün.
- `go test -race ./internal/configdrift/ -count=1`: ok (nur Mac).
- `go build ./...`, `go vet ./internal/configdrift/`, `gofmt -l internal/configdrift`: sauber.
- `go test ./internal/store/fsstore/ -run TestNoDirectFileAccessOutsideFsstore` und `go test ./internal/publicrepo/`: ok.
- CI-Linter über `./...`: `0 issues.`
- `grep -n 'reconcile(' internal/configdrift/configdrift.go` trifft den Aufruf in `Tick` (Zeile 202).

## Abweichungen vom Plan

**1. [Rule 2] Zusätzliche Tests und Messungen.** `TestConfirmDoesNotRestoreADamagedBaseline` (die Ermessensentscheidung 2 aus dem Objective hatte keinen Test), `TestConfirmWhenTheRecordWentAway` (der Rev-0-Zweig aus dem Plantext), eine Unterprobe „ein Leser ohne Digest wird nicht durch einen leeren bestätigt“ (nur so ist die Formatprüfung des Digests überhaupt messbar) und `TestAPokedNodeReachesARealNodeFromAContextWithoutADeadline` (die Lehre aus certwatch: der Frist-Wächter liest auch durch den echten Client gegen talossim). Alle mit Injektion rot gesehen, bis auf (f), siehe oben.

**2. [Plan-Annahme] Der Pass hat keinen Eintrag je Maschine und keine Knotenliste.** Der Plan sagt, `Forget` lasse „den Eintrag im Pass der Maschine“ fallen und `Flush` hänge den Knoten in „die Knotenliste“ des Passes. `pass` aus Plan 03 trägt nur `at`, `nodes` und `read` je Cluster. `Forget` und `Flush` lassen `passes` deshalb unberührt; die Zahlen „n von m gelesen“ stimmen nach einem Anstoß bis zum nächsten Lauf nicht auf den Knoten genau. Plan 06 entscheidet, ob das genügt.

**3. [Rule 2] `Flush` braucht auch die Liste der Cluster.** Der Plan nennt nur `Disabled` am Knoten; die Wahrheit „jeder Knoten eines ausgeschalteten Clusters wird nie gelesen“ gilt auch für den Anstoß. `flush` ruft deshalb `Deps.Clusters` und verwirft Knoten ausgeschalteter oder unbekannter Cluster (Injektion (h2) misst es).

**4. Injektion (c) von Task 2 ist `case false`, nicht „ein gescheiterter Versuch zählt als Lesung“.** Mit `case !ok:` allein bliebe `TestConfirmRefusesWhatWasNeverRead` grün, der Plan verlangt beide rot; `case false` macht beide rot. Die wörtliche Fassung (ein `unreadable` als `drift` behandeln) ist nicht einzeln gemessen.

**5. `helpers_test.go`, `decide.go` und `detail.go` stehen nicht in `files_modified`.** `helpers_test.go`: die austauschbaren Listen, die der Plantext verlangt (`inventory`, `withInventory`, `withAllMachines`), und `fakeReader.count/limit/read`. `decide.go`: `set` in `put` zerlegt, ohne Verhaltensänderung (nötig, damit `markConfirmed` unter einem Halt von `mu` arbeitet). `detail.go` und `configdrift.go`: zwei Kommentare nennen jetzt Plan 05 statt 04 als den, der Settling setzt.

**6. Kleinigkeiten, die der Plan offen ließ.** `TakenAt` einer bestätigten Basis ist der Zeitpunkt des Bestätigens (so steht es im Plan), das Feld im Modell sagt „wann gelesen“; der Satz im Detail („taken ... from an operator who confirmed“) passt zum Plan. Eine Basis, der `damageOf` misstraut oder die sich nicht laden lässt, macht `Confirm` zu einem einfachen Fehler mit Dateinamen und Weg heraus, ohne fünften exportierten Fehler. Ein Fehler beim Löschen in `Forget` und `reconcile` wird geloggt (mit Maschinen-Id und Fehler, wie `Tick` es für seine Listen tut), nicht gemeldet, weil `Forget` laut Vertrag keinen Fehler zurückgibt.

## Auth gates

Keine.

## Known Stubs

Keine. `StateSettling` und `SettleWindow` sind weiter nur vorhanden und werden erst von Plan 05 gesetzt.

## Was NICHT belegt ist

- Das Verhalten echter Knoten (A1, A2), `-race` auf dem Pi (dort nicht lauffähig, CI), kein Pi. Alles hier lief gegen talossim und einen Fake-Leser; `-race` nur auf dem Mac.
- Die Injektion 2(f) (ein Ladefehler der Basis wird wie „fehlt“ behandelt) bleibt grün: die Garbage-Variante von `TestConfirmDoesNotRestoreADamagedBaseline` ist durch den Store (Plan 02) gedeckt und auf dieser Ebene nicht rot zu sehen.
- Der gleichzeitige Fall steht auf einer Sperre am Eingang von `AllMachines`, die beide Bestätigungen gemeinsam loslässt; er ist damit nahezu, aber nicht streng deterministisch. Rot gesehen in 30 von 30 (gegen Task 1) und 50 von 50 (Injektion (d)) Läufen.
- „Ein ausgeschalteter Knoten zählt nicht als nicht gelesen“: die Pass-Zahlen (`nodes`, `read`) haben noch keinen Leser außer dem Paket; belegt ist nur, dass der Leser nie gerufen wird. Plan 06 gibt ihnen eine Anzeige.
- Der Zweig `ErrConflict` in `Confirm` (neu laden, neu prüfen) hat keinen Test: es gibt in diesem Paket keinen zweiten Schreiber, der ihn auslöste.
- Dass `Run` nach `Tick` auch `Flush` ruft, hat keinen Test (das Entfernen der Zeile wurde nicht gemessen). Auch die Logzeile „removed the baseline of a machine that is gone or moved“ und die Fehlerzeile von `Forget` haben keine Zusicherung.
- Ein Lesen, das schon läuft, während `Forget` oder `reconcile` die Basis löscht, kann danach eine neue Basis anlegen (bei einer verschwundenen Maschine räumt der nächste Lauf sie wieder weg); das ist nicht nachgestellt.
- Die Begrenzung „eine beschädigte Basis-Datei hält das Aufräumen an, bis sie gelöscht ist“ ist belegt (Injektion (g)), der Leitfaden-Satz dazu gehört Plan 10.
- Die Route (Plan 08), der Beobachter am Engpass für den Anstoß nach einem eigenen Apply (Plan 05) und die Verdrahtung in `main.go` fehlen noch; das Paket hat weiter keinen Aufrufer außer den Tests.

## Threat Flags

Keine neue Angriffsfläche über die im Plan geplante hinaus. T-15-13 (Digest als Compare-and-Swap) rot gesehen mit 2(a), T-15-14 mit 2(b) und 2(c), T-15-15 mit 3(a) und 3(g), T-15-16 mit 3(d); T-15-SC: nichts installiert.

## Self-Check: PASSED

Dateien vorhanden (`lifecycle.go`, `confirm_test.go`, `lifecycle_test.go`, geändert `configdrift.go`, `decide.go`, `detail.go`, `helpers_test.go`), Commits 276e723, ced36aa und 1b0f0d4 sind Vorfahren von HEAD.
