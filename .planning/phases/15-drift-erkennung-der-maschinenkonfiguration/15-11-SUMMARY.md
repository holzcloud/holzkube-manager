---
phase: 15-drift-erkennung-der-maschinenkonfiguration
plan: 11
subsystem: configdrift, docs, ledger, web
tags: [drift, adopt-window, gap-closure, wiring-test, ledger]
requires: [15-01, 15-02, 15-03, 15-04, 15-05, 15-06, 15-07, 15-08, 15-09, 15-10]
provides:
  - "AdoptWindow (= BannerAfter, 24 h): die Erwartung eines Applys mit reboot oder auto zählt so lange für die Übernahme; settling bleibt bei 15 Minuten, no-reboot behält 15 Minuten, staged ohne Ende"
  - "die Probe der Verifikation 2026-10-15 als Dauertest, die Ränder auf die Sekunde, D-13 für späte Landungen"
  - "Leitfaden und Ledger 177 nennen die Fristen je Modus, den Preis und die Grenze der Belegbarkeit"
  - "ein Rendertest über den echten routeTree hält ConfigDriftBanner in AppShell und NodeConfigDrift mit der Maschine der Seite in node-detail"
affects: []
actuals:
  tokens: 31000
  tasks: 3
  commits: 3
plan_head_before: 973f76a1e62b87f84676efb1376e0232446e352a
plan_head_after: f60a4680a303309c62beade0d02fa4dfc95ad41f
tech-stack:
  added: []
  patterns: ["Adoption und Settling sind zwei Regeln (adopts, settles), stale ist die Verneinung von adopts", "Rendertest über den echten Routenbaum mit gespiegelten API-Antworten und einem fetch, der jede nicht gespiegelte Adresse laut nennt"]
key-files:
  created:
    - web/src/routes/drift-wiring.test.tsx
  modified:
    - internal/configdrift/configdrift.go
    - internal/configdrift/apply.go
    - internal/configdrift/decide.go
    - internal/configdrift/expected_test.go
    - docs/guide.md
    - .planning/WINDOWS.md
key-decisions:
  - "Adoption und Settling getrennt (Betreiberentscheidung 2026-10-14): AdoptWindow 24 h für reboot und auto, settling 15 Minuten für jeden Modus außer staged"
  - "no-reboot behält 15 Minuten für die Übernahme (Ermessensentscheidung des Plans, benannt): es wirkt ohne Neustart und der Knoten wird gleich danach gelesen; ein fehlgeschlagener Anstoß bleibt ein Fehlalarm in der sicheren Richtung. Wollte der Betreiber auch no-reboot verlängern: eine Zeile in adopts, eine in den Tabellen der Tests, die Fristen in Leitfaden und Ledger"
requirements-completed: []
status: complete
---

# Phase 15 Plan 11: Gap-Schließung G-15-1 und W-15-1 Summary

Ein eigener Apply mit `reboot` oder `auto` wird auch dann als Landung übernommen, wenn die erste Lesung nach dem Neustart erst mit dem Takt von drei Stunden kommt (Zustand ok, Quelle apply): die Erwartung zählt dafür 24 Stunden (`AdoptWindow`), settling bleibt 15 Minuten; und die Verwendung von Banner und Knotenabschnitt hält ein Rendertest über den echten Routenbaum.

BASE: 973f76a1e62b87f84676efb1376e0232446e352a

`plan_head_before` ist derselbe Wert. `git rev-list --count 973f76a..HEAD` vor dem SUMMARY-Commit: 3 (442e0b6, 5801be9, f60a468). Nicht gepusht, nicht getaggt, `internal/changelog` unberührt.

## Wo es lief

Mac (darwin/arm64), Go 1.27.2, CI-Linter mit `GOTOOLCHAIN=go1.26.9` (golangci-lint v2.13.1), Node v26.11.1, vitest 4.1.11 (jsdom-Projekt). Kein echter Cluster, kein Pi, kein Browser (weder das Browser-Projekt noch der Layout-Wächter), kein Daemon-Lauf, kein `task`. `go test -race ./internal/configdrift/` lief auf dem Mac und war grün (Exitcode 0); auf dem Pi nicht.

## Was gebaut wurde

- **Task 1, Tracer (442e0b6).** `configdrift.go`: `AdoptWindow = BannerAfter` mit Kommentar (warum ein Tag), der Kommentar von `DefaultSettleWindow` sagt, dass er nur das Settling betrifft. `apply.go`: `modeReboot`, `modeAuto`. `decide.go`: `adopts(e, now)` (staged immer; reboot und auto solange das Alter kleiner als `AdoptWindow` ist; jeder andere Modus solange es kleiner als `SettleWindow` ist; strikte Grenze), `stale` ist `!adopts`, die Landungsregel fragt `adopts` direkt, `settles` ist unverändert, der Kopfkommentar von `decide` ist auf die Regel je Modus mit dem Preis umgeschrieben; der zweite Landungszweig (D-13) bleibt. `expected_test.go`: die Probe `TestAnApplyWithARestartIsAdoptedWhenTheNextReadingOnlyComesWithThePass` (reboot, auto), `TestAnExpectationStillCountsUntilItRunsOut` (neu), `TestAnOwnApplyLandingHoursLaterOnAnOpenDriftKeepsItOpen` (neu, drei Zeilen), und drei vorhandene Tests als Tabellen über die Modi (`TestAnApplyThatNeverLandedStopsBeingExpected`, `TestAnExpectationThatRanOutIsRemovedWhenTheNodeDrifts`, `TestAnExpectationThatRanOutIsNotAdopted`, die letzte mit dem Preis im Kommentar). Geändert gegenüber BASE im Paket: genau diese vier Dateien.
- **Task 2, Leitfaden und Ledger 177 (5801be9).** `docs/guide.md`: „The baseline“ nennt die Fristen je Modus (`up to 24 hours after the apply` einmal, `look like your own apply` einmal, nur in „What it cannot see“), ein neuer Stichpunkt vor dem zu A2. `.planning/WINDOWS.md`: Eintrag 177 geändert, in Tabelle und JSON-Block zeichengleich: neuer Satz `ZEITPUNKT DER LANDUNG NACH REBOOT UND AUTO`, SCHLIESSBEDINGUNG um den Satz zu reboot und auto erweitert, `reason` mit „A1, A2 und das Timing der Landung nach reboot und auto“.
- **Task 3, Rendertest (f60a468).** `web/src/routes/drift-wiring.test.tsx`: `routeTree` bei `/nodes/m-cp-1`, gespiegelt `status`, `me`, `clusters.list` (erster Cluster der Fixture mit `config_drift`), `machines.get` (Fixture), `configDrift.get`; `fetch` wirft mit der Adresse, `EventSource` still, `queryClient.clear()` je Test. Test 1: der Satz des Banners steht in einem Element mit `role="status"`. Test 2: der Satz des Abschnitts, `toHaveBeenCalledWith('m-cp-1')`, der Knopf „Confirm as intended“.

## ROT zuerst (Task 1)

- **Die Probe gegen den unveränderten Stand** (`go test ./internal/configdrift/ -run TestAnApplyWithARestartIsAdopted`): Exitcode 1, `--- FAIL: TestAnApplyWithARestartIsAdoptedWhenTheNextReadingOnlyComesWithThePass/reboot` und `/auto`, `expected_test.go:838 (vor dem Anhängen der Tests davor; heute :989): the applied version, read one pass later, is "drift" (The configuration differs from the baseline taken ... the difference was first seen ...), want ok`. Das ist die Assertion, kein Compile-Fehler. Nach dem Eingriff grün.
- **Die Tests mit fallender Prämisse** (ganzes Paket, neue Regel, vor dem Anpassen): Exitcode 1, rot wurden zwei, nicht drei: `TestAnExpectationThatRanOutIsRemovedWhenTheNodeDrifts` (`expected_test.go:298: an expectation that ran out survived the write of a drift: &{... Mode:reboot ...}`) und `TestAnApplyThatNeverLandedStopsBeingExpected` (`:271: an expectation that ran out is still in the record: &{... Mode:reboot ...}`); beide, weil ihre Prämisse „reboot verfällt nach SettleWindow“ fällt. Der dritte, `TestAnExpectationThatRanOutIsNotAdopted`, blieb grün, weil er heute den Modus no-reboot benutzt (der Plan hatte ihn für reboot und auto genannt, die Zeilen gibt es erst in der Tabelle; siehe Abweichung 1). Kein anderes Rot.

## Injektionen Task 1 (gegen die grüne Fassung, je ein Treffer, Build und vet vorher 0)

Je: `buildvet=0`, Exitcode des Testbefehls (`go test ./internal/configdrift/ -count=1`) 1, danach `cmp -s` gegen die gesicherte Datei Exitcode 0 (`decide.go`), danach grün.

| | Eingriff in `decide.go` | rot wurden |
|---|---|---|
| a | reboot und auto zählen nur bis `SettleWindow` (die Regel vor diesem Plan) | die Probe (beide Zeilen, `:989: the applied version, read one pass later, is "drift"`), `TestAnExpectationStillCountsUntilItRunsOut` reboot und auto (`:392: a reboot apply 23h59m59s old that landed reads as "drift"`), `TestAnOwnApplyLandingHoursLater...` alle drei Zeilen (`:857` Satz, `:851`, Vorbedingung `:837`) |
| b | reboot und auto ohne Ende (`return true`) | `TestAnExpectationThatRanOutIsNotAdopted` reboot 24h, auto 24h, reboot 48h; `TestAnApplyThatNeverLandedStopsBeingExpected` reboot, auto (`:289`); `...RemovedWhenTheNodeDrifts` reboot, auto (`:322`) |
| c | Grenze von `AdoptWindow` nicht strikt (`<=`) | genau die Zeilen „reboot 24h“ und „auto 24h“ von `...NotAdopted` (`:357: ... reads as "ok"`) und die Zeilen reboot und auto von `...NeverLandedStopsBeingExpected` (`:289`); die Zeilen „minus eine Sekunde“ blieben grün |
| d | auch no-reboot nimmt `AdoptWindow` | die no-reboot-Zeilen der drei Tabellen (`:289`, `:322`, `:357` für 15m und 30m) |
| e | `settles` nimmt `adopts` (Settling wächst mit) | `TestAnApplyInFlightIsSettlingNotDrift` reboot und auto (`:228: at the end of the window the state is "settling"`), dazu die Zeile „drift dated after the settle window“ (`:835` Vorbedingung) |
| f | der D-13-Zweig nur, solange `settles` wahr ist | alle drei Zeilen von `...LandingHoursLater...` (`:851: ... reads as "ok"`); die bestehenden D-13-Tests blieben grün |
| g | Lesung gleich Basis löscht jede Erwartung (`spent := base.Expected != nil`) | die Probe an `:979: the expectation did not survive the reading of the old version: <nil>`, und `TestAStagedConfigurationPendingIsNotDrift` (`:162: after three days the expectation is <nil>, want the staged one still there`) |
| h | das Schreiben der Drift löscht eine verfallene Erwartung nicht (`spent := false`) | `...RemovedWhenTheNodeDrifts` alle drei Zeilen (`:322`), `...NotAdopted` (`:364: the expectation that ran out is still in the record`) |

Keine Injektion blieb grün; keine war nur ein Buildbruch. Alle acht Injektionen waren Einzeiler mit genau einem Treffer (das Skript bricht bei einer anderen Trefferzahl ab).

## Task 2: Messung des Ledgers (vorher und nachher)

| Größe | vorher | nachher |
|---|---|---|
| Zeilen mit `^| ` in `.planning/WINDOWS.md` (mit Kopfzeile) | 180 | 180 |
| `open_count` | 26 | 26 |
| `total_count` | 179 | 179 |
| Zeilen mit `ZEITPUNKT DER LANDUNG NACH REBOOT UND AUTO` | 0 | 2 (Tabelle Zeile 2355, JSON-Block Zeile 2303) |

`go test ./internal/planning/ -count=1 -v`: Exitcode 0, kein SKIP. `go test ./internal/publicrepo/`: grün.

**Injektion in die Handarbeit:** in der Tabellenzeile von 177 „Der Preis: wendet“ zu „wenden“, JSON-Block unberührt: `go test ./internal/planning/ -run TestTheLedgersTwoHalvesSayTheSameThing`, Exitcode 1, `windows_test.go:156: entry 177 differs between the table and the JSON block:`; `cp` zurück, `cmp -s` Exitcode 0, danach grün. **Zum Leitfaden gibt es keinen Wächter** (Text; nur `internal/config`, `internal/host/hostaction` und `internal/publicrepo` lesen Dokumente und laufen grün), also keine Injektion; die Zahl 24 Stunden im Leitfaden wird von keinem Test gegen `AdoptWindow` gehalten.

## Injektionen Task 3 (vitest nur auf der neuen Datei, Exitcode des Befehls selbst)

Je: ein Treffer, `cmp -s` Exitcode 0 auf der Datei, danach grün. Gelesen wurde jeweils die Assertion des Tests, kein Importfehler.

| | Eingriff | vitest | gelesene Zeile |
|---|---|---|---|
| a | die Zeile `<ConfigDriftBanner className="px-4 pt-2" />` aus `AppShell.tsx` gelöscht | Exitcode 1, Test 1 rot, Test 2 grün | `TestingLibraryElementError: Unable to find an element with the text: cp-1 has differed from its baseline for more than a day. ...` |
| b | die Zeile `<NodeConfigDrift machine={m.id} ...>` aus `node-detail.tsx` gelöscht | Exitcode 1, Test 2 rot, Test 1 grün | `Unable to find an element with the text: This node differs from its baseline and nothing this installation did explains it.` |
| c | `machine={m.hostname.value ?? ""}` statt `m.id` (`npm run typecheck` dabei Exitcode 0) | Exitcode 1, Test 2 rot, Test 1 grün | `AssertionError: expected "get" to be called with arguments: [ 'm-cp-1' ]` |

(a) und (b) lassen einen verwaisten Import zurück: `tsc` bricht dort (Exitcode 1, `noUnusedLocals`), vitest nicht, und die gelesene Zeile ist die Assertion. Bei (c) brach der erste Versuch mit `m.hostname` den Typ (das Feld ist ein Objekt, kein String; `tsc` Exitcode 1): die Assertionszeile war dieselbe, aber die Bedingung des Plans (Typprüfung vorher Exitcode 0) war nicht erfüllt, deshalb wurde (c) mit `m.hostname.value ?? ""` wiederholt, `typecheck` Exitcode 0. `AppShell.tsx` und `node-detail.tsx` sind gegen BASE unberührt (`git diff --name-only` unter `web/src` nennt nur die neue Testdatei).

Suite: vorher laut Plan 70 Dateien und 927 Tests; nachher `Test Files 71 passed (71)`, `Tests 929 passed (929)` (+1 Datei, +2 Tests), Exitcode 0. `npx tsc --noEmit` Exitcode 0. `npx biome check src`: Exitcode 0, 2 Warnungen und 1 Info (die alten), keine in `drift-wiring.test.tsx`.

## Gate

- `go test ./internal/configdrift/ ./internal/machineconfig/ ./internal/talos/ ./internal/inventory/ ./internal/planning/ ./internal/publicrepo/ ./cmd/holzkube-managerd/ -count=1`: alle ok.
- `go build ./...`, `go vet ./internal/configdrift/`, `gofmt -l internal cmd` leer: Exitcode 0.
- `go test -race ./internal/configdrift/ -count=1`: Exitcode 0 (Mac).
- `GOTOOLCHAIN=go1.26.9 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1 run ./...`: Exitcode 0, `0 issues.`
- Unberührt gegen BASE (ein Befehl über `internal/changelog`, certwatch, machineconfig, talos, inventory, model, store, upgrade, rotateca, provision, httpapi, cmd, README.md, docs/api-contract.md, docs/screenshots, web/fixtures, package.json, package-lock.json, wall.tsx, wall.test.tsx): leer. Geänderte Dateien außerhalb von `.planning/`: genau die sechs plus `WINDOWS.md` aus `files_modified`.
- Die vier Tests in `internal/httpapi`, die `web/dist` brauchen, gehören nicht zu diesem Plan und wurden nicht gelaufen.

## Deviations from Plan

**1. [Beobachtung, keine Änderung] `TestAnExpectationThatRanOutIsNotAdopted` wurde mit der neuen Regel nicht rot.** Der Plan nannte drei Tests mit fallender Prämisse; der dritte benutzt heute no-reboot und ist von der Änderung nicht betroffen. Die Zeilen für reboot und auto kamen mit der Tabelle, und die Injektionen (b), (c) und (h) haben sie rot gesehen. Es sind zwei Tests, die durch die Regel selbst rot wurden, nicht drei.

**2. [Wiederholung] Injektion 3(c), erster Versuch.** `m.hostname` ist kein String; der Typ brach (`tsc` Exitcode 1). Wiederholt mit `m.hostname.value ?? ""` bei grünem Typ.

**3. [Rule 3 - Blocking, Testdaten] Der Banner-Test nimmt nur den ersten Cluster der Fixture.** Die Fixture `/api/v1/clusters` hat mehrere Cluster, und mit `config_drift` an jedem stand der Satz mehrfach (`findByText` fand mehrere). Jetzt `.slice(0, 1)`. Keine Fixture geändert.

**4. Die Vorbedingung der Suite (70 Dateien, 927 Tests) wurde nicht selbst vor dem Eingriff gemessen**, sondern aus dem Plan (Verifikation) übernommen; gemessen ist nur nachher 71 und 929, was genau +1 und +2 ergibt.

## NOT proven (ungeprüft und so gesagt)

- Ein echter Knoten: A1, A2 und dieses Timing. talossim wendet jeden Modus sofort an und zeigt weder den Neustart noch die Wartezeit bis zur ersten Lesung; die Regel ist nur mit gesetzten Datensätzen und vorgestellter Uhr belegt. Ledger 177 bleibt offen (jetzt mit dem Timing in Beschreibung, SCHLIESSBEDINGUNG und Grund).
- Der Pi und `go test -race` dort.
- Ein Browser: Aussehen, Raster und Tippziele von Banner und Abschnitt, das Browser-Projekt und der Layout-Wächter.
- Das Banner nach echten 24 Stunden.
- Ein Lauf des Daemons.
- Der Preis bleibt: wendet ein Dritter innerhalb des Fensters (reboot, auto: bis 24 Stunden) genau dieselben Bytes an, ist das von der Landung nicht zu unterscheiden. Er steht im Kommentar des Tests, im Leitfaden und in Ledger 177.
- Dass die Zahl 24 Stunden im Leitfaden zu `AdoptWindow` passt, hält kein Test.

## Known Stubs

Keine.

## Threat Flags

Keine neue Angriffsfläche (T-15-37 bis T-15-39 des Plans sind mit den Injektionen oben gemessen; T-15-SC: nichts installiert, `web/package.json` und `package-lock.json` unberührt).

## Self-Check: PASSED

442e0b6, 5801be9 und f60a468 sind Vorfahren von HEAD; `web/src/routes/drift-wiring.test.tsx`, `internal/configdrift/decide.go` (`func (s *Service) adopts`), `docs/guide.md` (`up to 24 hours after the apply`) und `.planning/WINDOWS.md` (`ZEITPUNKT DER LANDUNG NACH REBOOT UND AUTO`, zwei Zeilen) sind vorhanden.
