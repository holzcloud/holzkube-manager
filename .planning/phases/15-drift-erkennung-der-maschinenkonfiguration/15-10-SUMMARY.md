---
phase: 15-drift-erkennung-der-maschinenkonfiguration
plan: 10
subsystem: docs, ledger, gate
tags: [drift, ledger, readme, guide, gate, state]
requires: [15-01, 15-02, 15-03, 15-04, 15-05, 15-06, 15-07, 15-08, 15-09]
provides:
  - "vier offene Ledger-Einträge 176 bis 179 in Tabelle, JSON-Block und Kopfzählungen (open 22 auf 26, total 175 auf 179)"
  - "README: der Punkt Configuration drift in Was-es-tut, kein neues Bild"
  - "docs/guide.md: der Abschnitt Configuration drift mit den Grenzen, der Verweis aus Schema 7, die Metriken-Aussage zu den Labels wieder wahr"
  - "das Gate der ganzen Phase, mit Exitcodes, und eine ehrliche Liste dessen, was nur CI, ein Browser oder ein echter Knoten prüfen kann"
affects: []
actuals:
  tokens: 7100
  tasks: 3
  commits: 2
plan_head_before: 239bcf40561bc0c51ed5059b5825a4e5f780eb1a
plan_head_after: 6ca513c82ab008c12765fb5b5b70b6e8ae91333c
tech-stack:
  added: []
  patterns: ["Ledger-Eintrag in allen drei Hälften aus einer Quelle erzeugt, vom Planungstest gegeneinander gehalten", "jede Grenze im Leitfaden hat einen offenen Ledger-Eintrag"]
key-files:
  created: []
  modified:
    - .planning/WINDOWS.md
    - README.md
    - docs/guide.md
    - .planning/STATE.md
key-decisions:
  - "Kein Bild im README (Betreiber, 2026-10-14): die Funktion ist ein Abschnitt, eine Zeile und ein Banner auf vorhandenen Bildschirmen"
  - "REQUIREMENTS.md bleibt für DRIFT-01 bis DRIFT-09 auf Pending: ein Teil dessen, was die Anforderungen verlangen (Browser, echter Knoten), ist nicht geprüft, das Abschließen gehört der Verifikation der Phase"
requirements-completed: []
status: complete
---

# Phase 15 Plan 10: Dokumentation, Ledger, Gate Summary

Vier offene Ledger-Einträge für das, was die Phase nicht belegen kann (Blindfleck von Compare, A1/A2 und staged/try, unsichtbare Geheimniswerte, nicht redigierte Felder in `config-baselines/`), die Funktion in README und Leitfaden mit jeder Grenze im Absatz der Behauptung, und das Gate der ganzen Phase: grün, soweit es hier läuft.

BASE: 239bcf40561bc0c51ed5059b5825a4e5f780eb1a

`plan_head_before` ist derselbe Wert. `git rev-list --count 239bcf4..HEAD` vor dem SUMMARY-Commit: 2 (561b5d1, 6ca513c). Die Prüfung der ganzen Phase läuft gegen den BASE aus Plan 01 (`eba8d7083533593a68287f93ec29baf7d32dfc3b`), siehe Task 3.

## Was gebaut wurde

- **Task 1, Tracer und Erweiterung (561b5d1).** `.planning/WINDOWS.md`: die Einträge 176 (`defect`, `internal/machineconfig/diff.go`, COMPARE SIEHT NUR DAS ERSTE DOKUMENT), 177 (`unrun-verify`, `internal/configdrift/decide.go`, ZWEI ANNAHMEN UEBER ECHTE KNOTEN), 178 (`gap`, `internal/machineconfig/snapshot.go`, EINE REINE AENDERUNG EINES GEHEIMNISWERTS), 179 (`gap`, `internal/machineconfig/snapshot.go`, FELDER, DIE REDACT NICHT KENNT). Alle `open`, Phase `15`, Grund beginnt mit `Offen:`, SCHLIESSBEDINGUNG im Text, `recorded_at` 2026-10-15T11:10:00.000Z, `last_updated` 2026-10-15. Der Tracer war 176 allein durch alle drei Hälften, danach 177 bis 179 je mit dem Planungstest dazwischen.
- **Task 2, README und Leitfaden (6ca513c).** README: der Punkt **Configuration drift** nach „Certificates and security“. `docs/guide.md`: `### Configuration drift` nach „Certificates inside the cluster“ (Lesetakt und Budget, Basis, was zählt und warum keine Ausnahmeliste, die sieben Zustände als Tabelle, wo es erscheint und das Banner nach 24 Stunden, D-13, Bestätigen, die Grenzen, Reparaturen, die vier Metriken), ein Verweis aus „Schema 7 and going back“, in „Metrics“ die Liste dessen, was exportiert wird, und der Satz zu den Labels.
- **Task 3, Gate (kein Code).** Siehe unten. STATE.md von Hand repariert (Abweichung 4).

### Vorher und nachher des Ledgers (gemessen)

| Größe | vorher | nachher |
|---|---|---|
| `open_count` | 22 | 26 (+4) |
| `total_count` | 175 | 179 (+4) |
| `waived_count`, `fixed_count` | 19, 134 | 19, 134 |
| Zeilen mit `^| ` am Anfang (mit Kopfzeile der Tabelle) | 176 | 180 |

Eindeutige Zeichenketten, je mit `grep -c` über die Datei (Tabelle und JSON-Block):

| Zeichenkette | Treffer |
|---|---|
| `COMPARE SIEHT NUR DAS ERSTE DOKUMENT` | 2 |
| `ZWEI ANNAHMEN UEBER ECHTE KNOTEN` | 2 |
| `EINE REINE AENDERUNG EINES GEHEIMNISWERTS` | 2 |
| `FELDER, DIE REDACT NICHT KENNT` | 2 |

`go test ./internal/planning/ -count=1 -v` nach dem vierten Eintrag: Exit 0, neun `--- PASS`, keine `--- SKIP`.

## Rot gesehen

Exitcodes sind die des `go test`-Befehls selbst (in Dateien umgeleitet, keine Pipe). Sicherung der Datei außerhalb des Repos, Wiederherstellung und `cmp -s` jedes Mal mit Exit 0, danach grün (Exit 0).

| Injektion | Test | Exit | scheiternde Zeile |
|---|---|---|---|
| `open_count` im Kopf von 26 auf 27 | `TestTheFrontMatterCountsWhatIsThere` | 1 | `windows_test.go:199: front matter says open_count: 27, the ledger holds 26` |
| in der Tabellenzeile von 178 `UNSICHTBAR` zu `SICHTBAR`, JSON-Block unberührt | `TestTheLedgersTwoHalvesSayTheSameThing` | 1 | `windows_test.go:156: entry 178 differs between the table and the JSON block` |

Beide haben tatsächlich injiziert und nicht nur etwas anderes gebrochen: die Datei baut nicht, der Test liest sie. Zur Leitfaden-Seite gibt es keine Injektion: Texte haben keinen Wächter außer `internal/config` (Optionstabelle), `internal/host/hostaction` (Marker) und `internal/publicrepo`, und die liefen grün.

## Das Gate der ganzen Phase

Jeder Befehl mit dem Exitcode des Befehls selbst (Ausgaben in Dateien, nicht durch Pipe).

| Befehl | Exit | Ergebnis |
|---|---|---|
| `go build ./...` | 0 | sauber |
| `go vet ./...` | 0 | sauber |
| `test -z "$(gofmt -l cmd internal)"` | 0 | nichts zu formatieren |
| `go test ./... -count=1` | 1 | 42 Pakete `ok`; scheitern: die vier bekannten `internal/httpapi`-Tests (`TestCSPHashMatchesTheServedShell`, `TestForeignHostIsRefused`, `TestNoAssetIsInlinedPastTheContentSecurityPolicy`, `TestFontsAreServedAsFiles`; `web/dist` fehlt, nicht meine) und `internal/power` `TestADisabledNodeStaysOffUntilItIsEnabled` (der bekannte Zeittest unter Last) |
| `go test ./internal/power/ -count=1` (allein) | 0 | `ok` 5,6 s: der Zeittest war ein Lastfehler, kein Fehler |
| `go test ./internal/httpapi/ -count=1 -skip '<die vier>'` | 0 | `ok` 185,8 s: alles andere im Paket läuft, auch die Drift-Routen und die Erreichbarkeit |
| `go test ./cmd/holzkube-managerd/ -count=1 -v` | 0 | `ok`, keine `--- SKIP` (Erreichbarkeit der Routen, AST-Wächter der Verdrahtung) |
| `go test -race ./internal/configdrift/ ./internal/talos/ ./internal/inventory/ -count=1` | 0 | alle drei `ok`; lief, weil `uname -s` hier `Darwin` ist |
| `GOTOOLCHAIN=go1.26.9 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1 run ./...` | 0 | `0 issues.` |
| `cd web && NODE_OPTIONS=--no-experimental-webstorage npx vitest run --project jsdom` | 0 | 70 Dateien, 927 Tests bestanden, nichts als übersprungen gemeldet |
| `npm --prefix web run typecheck` | 0 | sauber |
| `npm --prefix web run lint` | 0 | 0 Fehler; 2 Warnungen und 1 Info, alle in Dateien, die die Phase nicht berührt hat (`src/components/DataTable.tsx:137` noUselessFragments; `src/routes/wall.test.tsx:83/85` noExplicitAny samt nicht benutzter Unterdrückung) |
| `go test ./internal/publicrepo/ ./internal/planning/ -count=1 -v` | 0 | beide `ok`, keine `--- SKIP` |
| `go test ./internal/config/ ./internal/host/... ./internal/publicrepo/ ./internal/planning/ -count=1` (Task 2) | 0 | alle `ok` |

Die Prüfung gegen BASE aus Plan 01 als ein Befehl (`BASE=eba8d7083533593a68287f93ec29baf7d32dfc3b`, `git cat-file -e` Exit 0), Ausgabe leer, `test -z` Exit 0:

```
git diff --name-only "$BASE" -- internal/changelog internal/certwatch internal/machineconfig/diff.go \
  internal/machineconfig/applymode.go internal/compat deploy .goreleaser.yaml .github docs/screenshots \
  web/fixtures web/src/routes/wall.tsx web/src/routes/wall.test.tsx \
  internal/httpapi/handlers/kubernetes.go internal/httpapi/handlers/wall_certificates.go \
  internal/httpapi/handlers/wall_host.go internal/httpapi/handlers/wall_trends.go internal/httpapi/handlers/walllink.go
```

Ergänzend `git diff --name-only "$BASE" -- web/src`: genau die Dateien aus Plan 09 (`api.ts`, `api.test.ts`, `api.nulls.test.ts`, `AppShell.tsx`, `ConfigDriftBanner`, `ConfigDriftRow`, `NodeConfigDrift` je mit Test, `clusters.tsx`, `clusters.test.tsx`, `node-detail.tsx`). Für Task 2: `git diff --name-only 239bcf4 -- docs/screenshots web/fixtures internal/changelog` leer. `docs/api-contract.md` enthält „Configuration drift“ (Zeile 4302), die Vorbedingung von Task 1.

## Anforderungen

`.planning/REQUIREMENTS.md` ist nicht verändert, DRIFT-01 bis DRIFT-09 bleiben `Pending` (Abweichung 1). Getragen werden sie von: DRIFT-01: 03, 04, 06; DRIFT-02: 01, 02, 03; DRIFT-03: 03, 04; DRIFT-04: 05, 07; DRIFT-05: 01, 03, 08, 09; DRIFT-06: 06, 09; DRIFT-07: 03, 04, 05, 06, 08, 09; DRIFT-08: 04, 08, 09; DRIFT-09: 02, 10.

## (a) Wo es lief

Mac, Darwin arm64, `go version go1.27.2 darwin/arm64` (der Linter lief mit `GOTOOLCHAIN=go1.26.9`, der CI-Version), `node v26.11.1`. Kein echter Cluster, kein Pi, kein `task`, kein Browser. `go test -race` lief nur für `configdrift`, `talos` und `inventory` und nur hier (Darwin); auf dem Pi läuft der Race-Detektor nicht (ThreadSanitizer lehnt den Adressraum des Pi-5-Kerns ab). Nichts gepusht, nichts getaggt, kein Changelog, nichts installiert.

## (b) Was nur CI prüfen kann

- `go test -race` für alles außer den drei genannten Paketen (die Läufe der Pläne 01 bis 08 nennen je ihre Pakete; ein Gesamtlauf `-race ./...` hat hier nicht stattgefunden) und auf dem Pi ganz.
- `task ci` als Ganzes (kein `task` hier).
- Das Browser-Projekt von vitest und der Layout-Wächter (`web/scripts/layout-audit.mjs`): kein Chromium. Damit ist für den neuen Abschnitt der Knotenseite, den Knopf **Confirm this configuration as the new baseline**, die Zeile auf der Clusterkarte und den Link im Banner **nicht** gemessen, dass die Tippziele 44 px erreichen (die Klassen `max-md:min-h-11` stehen, der jsdom-Test prüft nur das), und auch nicht, dass die Seite so aussieht, wie sie gemeint ist.
- Die vier `internal/httpapi`-Tests, die `web/dist` brauchen: ein Build des Frontends ist nicht Teil dieses Laufs.

## (c) Was nur ein echter Knoten prüfen kann

A1 (Talos schreibt die aktive Konfiguration nicht selbst um) und A2 (ein echter Knoten liefert nach einem Apply semantisch das Gesendete zurück) sind nur gegen die Dokumentation und gegen `talossim` belegt (Ledger 177). `staged` und `try` bildet der Simulator nicht nach; die Erwartungsregeln dazu sind auf der Ebene der Entscheidung mit gesetzten Datensätzen belegt. Ungeprüft: das Verhalten nach `reboot` und `auto`, der Fall eines Knotens, der die Verbindung annimmt und dann schweigt, so dass das Budget von 20 Sekunden wirklich abläuft, die Grenze „ein nur gestoppter Cluster kostet je Knoten bis zu fünf Sekunden“ (aus dem Code der Sonde gelesen, nicht gemessen), das Banner nach einer echten Wartezeit von 24 Stunden (die Uhr war überall vorgestellt), und kein Lauf des Daemons selbst (Plan 08: der AST-Wächter hält die Verdrahtung, kein Prozess wurde gestartet). Der Weg zurück nach Schema 7 (Rollback des Update-Skripts, `restore` der Sicherung) ist aus dem Code gelesen, nicht ausgeführt.

## (d) Was nur der Betreiber entscheiden kann

Jeweils als Auswahl, mit der Empfehlung zuerst:

1. **Ledger 179: nicht redigierte Felder.** Empfohlen: **A. Eine echte Basis-Datei auf Geheimnisse in `machine.files`, `machine.env`, `machine.install.extraKernelArgs`, `cluster.inlineManifests` prüfen** (Kosten: ein Blick in eine Datei unter `config-baselines/`, die Geheimnisse des Clusters enthalten kann und nur auf dem Rechner des Betreibers liegt; Nutzen: schließt A7, und trägt nichts ein Geheimnis, bleibt D-14 so). B. **D-14 sofort neu fassen** (Schwärzung nach Feld mit einem Hash je Wert samt Migration der gespeicherten Basen; Kosten: Schema 8 und eine Migration, Nutzen: nie ein Rohwert in der Basis; sinnvoll erst, wenn A etwas findet). C. **Den Eintrag als Hinweis offen lassen** (Kosten: nichts, Nutzen: nichts; die Basen liegen weiter verbatim in Sicherungen).
2. **Ledger 178: geänderte Geheimniswerte.** Empfohlen: **A. Verzicht, endgültig** (die Rotation eines Tokens ist eine Handlung, die der Betreiber selbst kennt; Kosten: eine rotierte Token fällt nie auf). B. **Ein Digest je Geheimnisfeld in der Basis** (Nutzen: eine Rotation wird sichtbar, ohne Wert; Kosten: ein neues Format, eine Migration, ein Hash eines Geheimnisses auf der Platte, der bei schwachen Werten ratbar wäre, also eine neue Bedrohung zu bewerten).
3. **Wie oft der Update-Timer in der Praxis zurückrollt** (Schema 7, D-08). Empfohlen: **A. Beobachten, nichts tun**: ein Rollback nach dem ersten Start der Version mit Schema 7 lässt den Dienst nicht starten, der Weg heraus steht im Leitfaden. B. **Dem Update-Skript beibringen, bei einem Rollback die Sicherung vor der Migration einzuspielen** (Nutzen: kein Handgriff; Kosten: Eingriff in das Skript, das hier nicht angefasst wurde, und ein Rollback, das Änderungen seit der Migration verwirft).

## (e) Die Ermessensentscheidungen dieser Phase

D-12, D-13 und D-14 hat der Betreiber gewählt, ebenso „kein Bild“. Die übrigen hat die Ausführung getroffen und sie stehen in den SUMMARY der Pläne: das Aufräumen verwaister Basen im Lauf statt in Haken (04); keine bedingungslose Übernahme der Lesung nach `no-reboot` (05; Preis: ist A2 falsch, erscheint ein eigener Apply als Drift); kein Wiederherstellen einer beschädigten Basis über Bestätigen (04); der Takt von drei Stunden und das Fenster von 15 Minuten (03); der Haken der Provisionierung im Schritt „apply the configuration“ statt am Ende des Jobs (07); keine Fehlertexte in der Clusterliste (06); der Abschnitt der Knotenseite zwischen Hardware und Live-Ausgabe (09, nicht im Browser gesehen).

## (f) Eine Zeile je Plan

Die Zahl ist die der Injektionszeilen in den Tabellen des jeweiligen SUMMARY; „grün gesehen“ sind Injektionen, die bewusst nicht rot wurden, weil eine zweite Schicht sie fängt.

| Plan | Inhalt | Injektionen, rot | Lücken, die geschlossen wurden |
|---|---|---|---|
| 01 | Schnappschuss, `CompareDocuments`, `ReadSnapshot` | 10 | Klassenfrist im Leser |
| 02 | `config-baselines/`, Schema 7 | 7 | `TestOpenCreatesTheConfigBaselinesDirectory` (der Test des Plans war an dieser Stelle nicht rot zu kriegen) |
| 03 | der Leser, sieben Zustände | 17 (2 grün gesehen) | PEM-Gürtel und Datei-Prüfung beide gemessen; zwei Wächter gegen Ledger 137 |
| 04 | Bestätigen, Aufräumen, Anstoß | 24 (2 grün gesehen) | Digest als Compare-and-Swap, zweimal geprüft |
| 05 | Beobachter am Engpass, Erwartung, settling | 24 | Methodenmenge der Apply-Wege gepinnt |
| 06 | Sicht, Metriken, Banner-Regel | 29 | keine Fehlertexte in der Liste für Reader |
| 07 | Aufrufer, Quelltext-Wächter, Provisionierung | 20 | der Wächter fand die Simulator-Seite; Haken im richtigen Schritt |
| 08 | HTTP, Audit, Verdrahtung | 22 | Knoten im Audit aus dem Pfad; AST-Wächter über sechs Stücke |
| 09 | Oberfläche | 18 | kein Knopf außer bei `drift`, nie „fine“ ohne Urteil |
| 10 | dieser Plan | 2 | der Leitfaden-Satz zu den Labels war falsch und stimmt wieder |

## Abweichungen vom Plan

**1. REQUIREMENTS.md nicht abgehakt.** Der Auftrag des Orchestrators erlaubt, DRIFT-01 bis DRIFT-09 als erledigt zu markieren, „nur wenn jede Wahrheit zutrifft“; der Plantext (Task 3, Schritt 3) lässt sie auf `Pending`, bis die Verifikation der Phase sie abschließt. Die Wahrheiten dieses Plans treffen zu, aber die Anforderungen selbst (Anzeige im Browser, Verhalten gegen echte Knoten) sind zum Teil ungeprüft, siehe (b) und (c). Ich bin dem Plantext gefolgt. ROADMAP: Plan 15-10 abgehakt und der Fortschritt der Phase von den Handlern nachgezogen.

**2. [Rule 1] Der Leitfaden-Satz des Plans zu den Labels war nicht ganz wahr.** Der Plan nennt „die drei Drift-Reihen“ als Reihen mit Hostnamen als Label. Es sind zwei: `holzkube_node_config_drift` und `holzkube_node_config_read_age_seconds`; die zwei Cluster-Reihen tragen nur `cluster`. Der Leitfaden sagt es so.

**3. [Rule 1] Die Grenze (h) des Plans war nicht ganz wahr.** Der Plan sagt, der Fehlertext einer nicht lesbaren Maschine könne ihre Adresse nennen, „die der Reader ohnehin sieht“. Nach Plan 06 trägt die Clusterliste, die ein Reader aufrufen darf, einen festen Satz ohne Adresse und ohne Pfad; der Text mit Adresse oder Pfad steht nur auf der Knotenseite (ab Operator). Der Leitfaden sagt das.

**4. STATE.md von Hand.** Kopf (Frontmatter: `current_phase` 15, `current_phase_name`, `status: verifying`, `stopped_at`, `last_activity`) und „Current Position“ (zwei beschriftete Zeilen `Current Plan: 10` und `Total Plans in Phase: 10`, damit `state.advance-plan` die Position lesen kann) und der Satz im Abschnitt „Offen“ („Phase 15 … ist geplant und gebaut (10 Pläne) und wartet auf die Verifikation“; vom Plan vorgesehen). Dazu die Zeile zum Ledger („26 offen, 19 verzichtet“; stand auf 20) und der Absatz oben („Stand 2026-10-15, letztes Release v0.7.1, Phasen 13/14 und 16 `human_needed`“). Die Zählungen im Frontmatter (`progress`) rechnet `state.update-progress` neu.

**5. Die Ledger-Einträge sind aus einer Quelle erzeugt.** `gsd-tools windows append --kind gap` wird abgelehnt, also schrieb ein Hilfsskript im Scratchpad (nicht im Repo) die Tabellenzeile, das JSON-Objekt und die Kopfzählungen im Format von `row()` des Planungstests; der Test hält sie danach gegeneinander. Der Inhalt der Texte ist der des Plans, mit einer SCHLIESSBEDINGUNG je Eintrag. `recorded_at` ist 2026-10-15T11:10:00.000Z (der Tag des Anhängens); die Messung vom 2026-10-14 steht im Text von 176.

**6. Zusätze im Leitfaden über den Plan hinaus.** Die Grenze „der Plan der Config-Ansicht liest weiter nur das erste Dokument“ mit Verweis auf 176 (der Eintrag existiert, also gehört die Grenze in den Abschnitt); die Liste dessen, was `/metrics` exportiert, nennt jetzt auch `holzkube_node_certificate_seconds`, das seit v0.6.0 fehlte.

## Known Stubs

Keine.

## Threat Flags

Keine neue Angriffsfläche. T-15-35 (Werte der echten Installation): `internal/publicrepo` grün (Baum und Historie), nur Dokumentationswerte in Ledger, README und Leitfaden. T-15-36 (Behauptung ohne Beleg): jede Grenze im Leitfaden hat einen offenen Ledger-Eintrag, zwei Injektionen gegen den Planungstest rot. T-15-SC: nichts installiert.

## Was NICHT belegt ist

- Alles unter (b) und (c). Insbesondere: kein Browser hat die Oberfläche der Phase gesehen, kein echter Knoten die Annahmen A1 und A2, und `-race` lief nicht über das ganze Repository.
- Der Leitfaden ist gegen die SUMMARY der Pläne und den API-Vertrag geschrieben, nicht gegen eine laufende Installation. Aussagen wie „das Banner lässt sich nicht wegklicken“ und „nach einem Neustart fehlt das Banner bis zur ersten Lesung“ stammen aus Tests der Pläne 06 und 09 und aus dem Code, nicht aus einem Lauf des Daemons.
- Kein Test liest den neuen Abschnitt des Leitfadens gegen die Zustandsnamen im Code; ändert jemand einen Zustand oder eine Metrik, bleibt der Leitfaden still falsch. Die Zustände stehen im API-Vertrag und in `configdrift`, die Metriken in `internal/metrics`, der Leitfaden hat sie ein drittes Mal.
- Die Ledger-Einträge sind Text; dass 177 bis 179 sich schließen, wenn der Betreiber die genannten Läufe macht, ist deren Inhalt, nicht etwas, das ein Test hält.
- `internal/power` `TestADisabledNodeStaysOffUntilItIsEnabled` scheiterte im Lauf über alle Pakete unter Last und lief allein grün; die Ursache (Zeitabhängigkeit) ist nicht untersucht und nicht dieser Plan.

## Self-Check: PASSED

Dateien vorhanden (`.planning/WINDOWS.md`, `README.md`, `docs/guide.md`, `.planning/STATE.md`); Commits 561b5d1 und 6ca513c sind Vorfahren von HEAD; `grep -c` der vier eindeutigen Zeichenketten je 2; `open_count` 26 und `total_count` 179 (je plus vier gegenüber 22 und 175).
