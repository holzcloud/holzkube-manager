---
phase: 15-drift-erkennung-der-maschinenkonfiguration
plan: 05
subsystem: configdrift, talos
tags: [drift, apply-observer, expected-digest, settling, staged, try, d-10, d-13]
requires: [15-03, 15-04]
provides:
  - "internal/talos: ApplyAttempt, ApplyObserver, (*ClusterClient).SetApplyObserver, observeApply an beiden Apply-Wegen des Clients"
  - "internal/configdrift: (*Service).ApplyObserver; Erwartungsregeln in decide.go (gelandet wird Basis, settling, staged ohne Ablauf, veraltet und leer werden gelöscht, D-13)"
affects: [15-06, 15-07, 15-08, 15-10]
actuals:
  tokens: 16400
  tasks: 2
  commits: 2
plan_head_before: fd97795730bca147740718969541eaa49e0af339
plan_head_after: 4162bc90c6098071e9760aa037f94ecf55f27620
tech-stack:
  added: []
  patterns: ["Beobachter am einen Engpass statt an den Aufrufern", "erwarteter Digest vor dem RPC, Ergebnisfunktion räumt bei Fehler auf und stößt bei Erfolg an", "Erwartung gilt nur mit Digest, Modus und Fenster; veraltet heißt: nie übernommen und gelöscht"]
key-files:
  created:
    - internal/talos/applyobserver.go
    - internal/talos/applyobserver_test.go
    - internal/configdrift/apply.go
    - internal/configdrift/apply_test.go
    - internal/configdrift/expected_test.go
  modified:
    - internal/talos/client.go
    - internal/configdrift/decide.go
    - internal/configdrift/detail.go
    - internal/configdrift/configdrift.go
key-decisions:
  - "Der Beobachter hängt am ClusterClient (zwei Methoden, eine Funktion observeApply), nicht an den drei Aufrufern; die Methodenmenge Apply* ist als Test gepinnt"
  - "Keine bedingungslose Übernahme der Lesung nach no-reboot (Rückfall der Recherche): sie würde eine gleichzeitige Änderung eines Dritten verschlucken; Preis: ist A2 falsch, erscheint ein eigener Apply als Drift, die der Betreiber bestätigt"
  - "D-13: landet ein eigener Apply auf einer offenen, unbestätigten Drift, bleiben Basis und DriftSince, die Erwartung ist verbraucht, AppliedOnDriftAt wird gesetzt"
  - "Eine verfallene Erwartung wird doppelt behandelt: gelöscht (bei Lesung gleich Basis und beim Schreiben einer Drift) und bei der Landungsregel nie übernommen"
requirements-completed: []
status: complete
---

# Phase 15 Plan 05: Eigene Applies am Engpass Summary

Jeder Apply, den diese Installation über `ClusterClient.ApplyConfiguration` oder `ApplyConfigurationWithMode` sendet, hinterlässt vor dem RPC einen erwarteten Digest (nie Inhalt) in der Basis des Knotens; landet genau diese Fassung, wird sie die Basis (Source `apply`) und es entsteht nie Drift, eine Änderung eines Dritten bleibt Drift, und eine offene, unbestätigte Drift bleibt auch nach einem eigenen Apply offen (D-13).

BASE: fd97795730bca147740718969541eaa49e0af339

`plan_head_before` ist derselbe Wert. `git rev-list --count BASE..HEAD` vor diesem SUMMARY-Commit: 2 (cedfb01, 4162bc9).

## Wo es lief

Mac (darwin/arm64), `go1.27.2`, kein echter Cluster, kein Pi, kein `task`. `go test -race ./internal/talos/ ./internal/configdrift/ -count=1` lief hier grün (nur Mac; auf dem Pi nicht lauffähig, CI bleibt maßgeblich). Der CI-Linter `GOTOOLCHAIN=go1.26.9 ... golangci-lint@v2.13.1 run ./...` meldet `0 issues.` Nichts gepusht, nichts getaggt, kein Changelog, nichts installiert. Die vier `internal/httpapi`-Tests, die `web/dist` brauchen, sind nicht meine.

Hinweis zum Ablauf: der Lauf wurde einmal durch einen Spend-Limit-Fehler der API unterbrochen (nicht durch den Plan); danach wurde der Stand per `git status`, `go build`, `go vet` und die Tests neu geprüft statt aus dem Gedächtnis übernommen. Task 2 war zu diesem Zeitpunkt ungebucht, aber vollständig und grün; seine Injektionen (a) bis (l) waren da schon gelaufen (Protokoll unten).

## Was gebaut wurde

- `talos/applyobserver.go` (cedfb01): `ApplyAttempt{Cluster, Machine, Config, Mode}`, `ApplyObserver`, `SetApplyObserver`, `observeApply` (liefert immer eine aufrufbare Funktion). `client.go`: beide Apply-Methoden rufen `observeApply` (bei `ApplyConfigurationWithMode` nach der Moduswahl, vor dem RPC) und melden das Ergebnis über ein `defer` mit benanntem `err`, also auf jedem Rückgabepfad. `ApplyConfiguration` meldet `reboot` (Wert null der Leitung). `MaintenanceClient` unberührt.
- `configdrift/apply.go`: `ApplyObserver()`. `try`: nichts. Sonst Schnappschuss der Bytes (scheitert er: Logzeile, `nil`, der Apply läuft), dann unter `storeMu` Erwartung (Digest, Modus, Zeit) in die Basis, nur wenn eine unbeschädigte existiert. Ergebnisfunktion: bei Fehler die Erwartung dieses Versuchs löschen (verglichen wird Digest **und** Zeit), bei Erfolg `Poke`. Schreibzugriffe im Ergebnis über `context.WithoutCancel`.
- `configdrift/decide.go`: die Entscheidungstabelle um die Erwartungsregeln erweitert (Kopfkommentar neu): Landungsregel, D-13-Zweig, `settling` (nicht staged, im Fenster, keine offene Drift), Löschen von Erwartungen, die der Basis gleichen oder verfallen sind, `settles` und `stale` als Hilfen. `detail.go`: `sentenceSettling`.
- Tests: `apply_test.go` (Tracer über den echten Client gegen talossim), `expected_test.go` (Erwartungsregeln auf Entscheidungsebene mit gesetzten Datensätzen und vorgestellter Uhr, Ränder des Beobachters, D-13), `talos/applyobserver_test.go` (alle fünf Modi und der Weg ohne Modus, gescheiterte Applies inklusive Ablehnung durch den Knoten, unbekannter Modus, ohne Beobachter, nil-Ergebnis, gepinnte Methodenmenge).
- Gegen BASE unberührt: `git diff --name-only $BASE -- internal/machineconfig internal/upgrade internal/rotateca internal/inventory internal/provision internal/certwatch internal/changelog` ist leer.

## Rot gesehen

Exitcodes sind die des `go test`-Befehls selbst. Jede Injektion: genau eine Änderung am Produktionscode (das Skript verlangt genau einen Treffer), vorher `go build ./... && go vet` mit Exit 0, Wiederherstellung per `cp`, danach `cmp -s` mit Exit 0 (alle Zeilen unten). Gegen den Stand vor dem Eingriff (RED) zuerst, dann die Injektionen gegen die grüne Fassung.

### ROT vor jedem Eingriff

| Task | Lauf | Exit | scheiternde Zeile |
|---|---|---|---|
| 1 | Tracer ohne Beobachter | 1 | `cc.SetApplyObserver undefined`, `svc.ApplyObserver undefined`, `[build failed]` (Compile-Fehler, vom Plan zugelassen) |
| 2 | `expected_test.go` gegen den Stand nach Task 1 | 1 | `TestAnApplyInFlightIsSettlingNotDrift/{no-reboot,reboot,auto}`: `... reads as "drift", want settling`; `TestANoOpApplyClearsItsExpectation`: `the expectation of an apply that changed nothing is still there`; `TestAnApplyThatNeverLandedStopsBeingExpected`: `an expectation that ran out is still in the record`; `TestAnExpectationThatRanOutIsNotAdopted`: `reads as "ok" ..., want drift`; `TestAnOwnApplyLandingOnAnOpenDriftKeepsItOpen`: `reads as "ok" ..., want drift`; `TestConfirmingAfterALanding...`: Vorbedingung `state "ok", AppliedOnDriftAt 0001-...` |

Schon grün gegen den Stand nach Task 1 und deshalb **nicht** als Rot gezählt: die Aufrufe des Beobachters in den talos-Tests, `TestAStagedConfigurationPendingIsNotDrift`, `TestAStagedConfigurationNeverSettles` (ohne settling war „sofort drift" ohnehin so), `TestTryIsNeverAdopted`, `TestEveryModeButTryLeavesAnExpectation`, `TestAFailedApplyLeavesNoExpectation`, `TestAnApplyForANodeWithoutABaselineSetsNothing`, `TestAnApplyThatCannotBeSnapshottedDoesNotFailTheApply`. Sie sind nur über ihre Injektionen belegt (die Garbage-Variante: siehe unter „Nicht belegt").

### Task 1, Injektionen (gegen cedfb01, Test `TestTracerAnOwnApply...`)

| Injektion | Exit | scheiternde Zeile |
|---|---|---|
| (a) `ApplyConfigurationWithMode` ruft `observeApply` nicht | 1 | `apply_test.go:74: an own apply left no expected digest in the baseline` |
| (b) der Beobachter schreibt die Erwartung nicht (`_ = exp`) | 1 | dieselbe Zeile |
| (c) die Landungsregel entfällt (`if false`) | 1 | `apply_test.go:104: state after an own apply landed is "drift" (...), want ok` |
| (d, zusätzlich) die Ergebnisfunktion pokt nicht | 1 | `state after an own apply landed is "baseline-created" ..., want ok` |

### Task 2, Injektionen (a) bis (l), gegen 4162bc9

| Injektion | Test | Exit | scheiternde Zeile |
|---|---|---|---|
| (a) `ApplyConfiguration` ohne `observeApply` | `TestAnObserverHearsEveryConfigurationAClusterClientSends/ApplyConfiguration_sends_no_mode...` | 1 | `applyobserver_test.go:121: the observer heard 0 attempts and 0 outcomes, want 1 and 1` |
| (b) Ergebnis nur bei Erfolg gemeldet | `TestTheObserverHearsAFailedApply` (beide Zeilen), `TestAFailedApplyLeavesNoExpectation` (beide Zeilen) | 1 | `the observer heard 1 attempts and 0 outcomes, want 1 and 1`; `the outcome is [], want the node's rejection`; `a failed apply left an expectation: &{Digest:... Mode:no-reboot ...}` |
| (c) `try` wie die anderen | `TestTryIsNeverAdopted` (Beobachter und Client) | 1 | `the observer wants to hear the result of a try`; `a try changed the record`; `a try made the node be read (2 reads, was 1)` |
| (d) staged im Fenster (`settles` ohne Ausnahme) | `TestAStagedConfigurationNeverSettles` | 1 | `a third version while a staged one waits reads as "settling" ..., want drift: staged has no window` |
| (d2, zusätzlich) staged verfällt (`stale` ohne Ausnahme) | `TestAStagedConfigurationPendingIsNotDrift` | 1 | `after three days the expectation is <nil>, want the staged one still there` |
| (e) `settling` entfällt | `TestAnApplyInFlightIsSettlingNotDrift` (3 Modi) | 1 | `a young apply (reboot) and a version that is neither reads as "drift" ..., want settling` |
| (f) verfallene Erwartung wird bei Lesung gleich Basis nicht gelöscht | `TestAnApplyThatNeverLandedStopsBeingExpected` | 1 | `an expectation that ran out is still in the record` |
| (f2, zusätzlich) dasselbe beim Schreiben einer Drift | `TestAnExpectationThatRanOutIsRemovedWhenTheNodeDrifts` | 1 | `an expectation that ran out survived the write of a drift` |
| (f3, zusätzlich) Erwartung gleich Basis wird nicht gelöscht | `TestANoOpApplyClearsItsExpectation` | 1 | `the expectation of an apply that changed nothing is still there` |
| (g) `observeApply` auch vor der Moduswahl | `TestAnUnknownModeIsRefusedBeforeTheObserverIsCalled` | 1 | `the observer heard 1 attempts and 0 outcomes of a mode that was refused before sending, want none` |
| (h) staged schreibt keine Erwartung | `TestEveryModeButTryLeavesAnExpectation/staged` | 1 | `mode staged left no expectation` |
| (i) Landung verschiebt die Basis auch bei offener Drift | `TestAnOwnApplyLandingOnAnOpenDriftKeepsItOpen` | 1 | `an apply that landed on an open drift reads as "ok" ..., want drift` |
| (j) `settling` auch bei gesetztem `DriftSince` | `TestAnApplyInFlightOnAnOpenDriftIsStillDrift` | 1 | `... on an open drift, reads as "settling" ..., want drift` |
| (k) `Confirm` lässt `AppliedOnDriftAt` stehen (`lifecycle.go`, danach per `cmp` zurück) | `TestConfirmingAfterALandingMakesTheLandedStateTheBaseline/the_operator_confirms` | 1 | `the confirmed baseline still carries {Machine:m1 Rev:5 ...}` |
| (l) dritte Methode `ApplyConfigurationUnobserved` am Client (Build grün) | `TestEveryClusterClientMethodThatSendsAConfigurationIsObserved` | 1 | `ClusterClient's methods that apply a configuration are [ApplyConfiguration ApplyConfigurationUnobserved ApplyConfigurationWithMode], want [ApplyConfiguration ApplyConfigurationWithMode]` |

`client.go` nach (l) ist per `cmp -s` byte-gleich wiederhergestellt (Exit 0); ebenso alle anderen Dateien nach jeder Injektion.

Zusätzliche Injektionen (nicht vom Plan verlangt), alle Exit 1, Wiederherstellung `cmp` Exit 0:

| Injektion | Test | scheiternde Zeile |
|---|---|---|
| (m) die Landungsregel prüft die Verfallszeit nicht | `TestAnExpectationThatRanOutIsNotAdopted` | `a configuration that matches an expectation two windows old reads as "ok" ..., want drift` |
| (n) die Ergebnisfunktion vergleicht Digest und Zeit nicht | `TestTheOutcomeOfAnEarlierApplyDoesNotRemoveALaterExpectation` | `the failure of the first apply removed the expectation of the second: <nil>` |
| (q) der Beobachter schreibt die Bytes in den Modus (Geheimnisse in die Datei) | `TestAnOwnApplyLeavesNoSecretInTheBaselineFile` | `before the apply landed in a reading: the baseline file holds the machine (trustd) token` (und vier weitere Geheimnisse) |
| (s) Fenstergrenze `<` zu `<=` | `TestAnApplyInFlightIsSettlingNotDrift` | `at the end of the window the state is "settling" ...` |
| (t) ein Knoten ohne Basis bekommt einen Datensatz | `TestAnApplyForANodeWithoutABaselineSetsNothing` | `an apply for a node nobody had read left a baseline file (stat: <nil>)` |

Die Lehre aus certwatch (Fristen): der Tracer und `TestAFailedApply...` laufen von `context.Background()` (Tick, Flush) durch den echten Client gegen talossim; Apply-Aufrufe tragen eine Frist, weil der Client keine ohne akzeptiert.

## Verifikation

- `go test ./internal/talos/ ./internal/configdrift/ ./internal/machineconfig/ ./internal/planning/ -count=1`: ok (ein `--- SKIP` stammt vom vorhandenen `TestScenarioContractAgainstRealTalos`, Tier-1-Lauf gegen einen echten Talos, nicht von diesem Plan).
- `go test -race ./internal/talos/ ./internal/configdrift/ -count=1`: ok (Mac).
- `go test ./internal/talos/ -run 'TestMaintenanceClientMethodSetIsClosed|TestNoAddressAboveTheSeam'`: PASS (die Siegelwächter bleiben).
- `go build ./...`, `go vet ./internal/talos/ ./internal/configdrift/`, `gofmt -l`: sauber. `go test ./internal/publicrepo/ ./internal/store/...`: ok.
- CI-Linter über `./...`: `0 issues.`
- Alle Greps der Akzeptanzkriterien: `type ApplyObserver` (1), `observeApply(` in `client.go` (2), `func (s *Service) ApplyObserver` (1), `StateSettling` in `decide.go` (bereits durch `detail.go`-Konstante, genutzt in `decide.go`).

## Abweichungen vom Plan

**1. [Rule 2] Zusätzliche Tests und Injektionen:** `TestAnExpectationThatRanOutIsNotAdopted`, `TestAnExpectationThatRanOutIsRemovedWhenTheNodeDrifts`, `TestTheOutcomeOfAnEarlierApplyDoesNotRemoveALaterExpectation`, `TestAnOwnApplyLeavesNoSecretInTheBaselineFile` mit Injektionen (d2), (f2), (f3), (m), (n), (q), (s), (t), (d) aus Task 1. Grund: die Doppelsicherung für verfallene Erwartungen (löschen **und** nie übernehmen) macht (f) allein über die Dritt-Anwender-Szene nicht messbar; deshalb prüft der Test den Datensatz und ein eigener Test die Landungsregel.

**2. [Rule 1] Der Satz für eine Landung auf offener Drift.** Plan 03 hatte in `sentenceDrift` „an earlier difference that was never confirmed" geschrieben, der Plan 05 verlangt die Wortfolge `earlier change that was never confirmed`. Der Satz in `detail.go` wurde auf „change" umgestellt (kein Test hatte den alten Wortlaut). `detail.go` und `configdrift.go` (ein Kommentar) stehen nicht in `files_modified`.

**3. Aufbau von `decide`.** Die Neu-Normalisierung wird vor der Landungsregel gerechnet (nicht danach), damit ein nicht ladbarer Schnappschuss weiter zuerst als beschädigt gilt, und die Landungsregel greift nicht, wenn die Lesung nach Neu-Normalisierung die Basis ist (dann ist der Apply ein No-op und seine Erwartung wird dort gelöscht). Semantisch die Reihenfolge des Plans, technisch ein Zug früher.

**4. Beobachter meldet über `defer` mit benanntem Rückgabewert** statt je Rückgabepfad einzeln; das deckt auch den Pfad „leere Antwort" ab.

**5. Die Ergebnisfunktion vergleicht Digest und Zeit** (der Plan nennt nur den Digest), damit zwei gleiche Fassungen nacheinander sich nicht gegenseitig die Erwartung löschen.

## Auth gates

Keine.

## Known Stubs

Keine.

## Was NICHT belegt ist

- A2 (ein echter Knoten liefert nach einem Apply semantisch das Gesendete; nur der Simulator liefert bytegleich) und A1 (Talos schreibt die aktive Konfiguration nicht selbst um) sind ungeprüft; ist A2 falsch, erscheint ein eigener Apply auf einem echten Knoten als Drift, die der Betreiber bestätigt. Gehört in Plan 10 als offener Ledger-Eintrag.
- staged und try kann der Simulator nicht nachbilden (er wendet jeden Modus sofort an): die Erwartungsregeln dazu sind nur auf der Ebene der Entscheidung mit gesetzten Datensätzen belegt, nicht gegen einen Knoten, der wirklich bis zum nächsten Boot wartet oder zurückrollt.
- `-race` lief nur auf dem Mac, nicht auf dem Pi (dort nicht lauffähig, CI).
- Die Garbage-Variante von `TestAnApplyThatCannotBeSnapshottedDoesNotFailTheApply` (Bytes, die kein Dokument sind) hat keine eigene Injektion: ob der Beobachter dort `nil` statt eines Ergebnisses liefert und eine Logzeile ohne Inhalt schreibt, ist grün gesehen, aber nicht rot.
- Die Ablehnung eines Applys vor dem RPC durch `machineconfig.Service` selbst (dry-run in `Service.Apply`) erreicht den Client und damit den Beobachter nie: dort entsteht keine Erwartung, weil es keinen Aufruf gibt; belegt ist nur der Client-Weg (`Mode{DryRun: true}`).
- Ein früher erwarteter Apply, dessen Erwartung ein späterer überschrieben hat und der danach scheitert, lässt nur den späteren verfallen: die frühere Erwartung ist weg (eine Erwartung je Knoten, so ist das Modell). Folge ist ein möglicher Fehlalarm, nie eine verschluckte Änderung.
- Während ein eigener Apply im Flug ist, wird auch eine gleichzeitige Änderung eines Dritten bis zu `SettleWindow` als `settling` gezeigt, danach als Drift (bewusster Preis, T-15-17); wendet ein Dritter byteweise dieselbe Fassung an wie wir, ist das nicht zu unterscheiden.
- Wer den Beobachter an einen Client hängt, ist Plan 07 (`inventory.Connect`); in der Produktion hängt er noch an keinem. `passes` ist weiter nicht je Knoten (siehe 15-04), die Zahlen „n von m" nach einem Anstoß gehören Plan 06.

## Threat Flags

Keine neue Angriffsfläche über den Plan hinaus. T-15-17 rot gesehen mit (d), (e), (f), (b), (h), (i), (j), (m); T-15-18 mit (q); T-15-19 (Beobachter lässt den Apply nie scheitern) nur grün gesehen (siehe oben); T-15-20 mit (l); T-15-SC: nichts installiert.

## Self-Check: PASSED

Dateien vorhanden (`applyobserver.go`, `applyobserver_test.go`, `apply.go`, `apply_test.go`, `expected_test.go`; geändert `client.go`, `decide.go`, `detail.go`, `configdrift.go`), Commits cedfb01 und 4162bc9 sind Vorfahren von HEAD.
