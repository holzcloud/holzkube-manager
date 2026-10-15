---
phase: 15-drift-erkennung-der-maschinenkonfiguration
verified: 2026-10-15T11:45:00Z
re_verified: 2026-10-15T13:00:00Z
status: human_needed
score: 9/9 must-haves verified
covered_files:
  - .planning/WINDOWS.md
  - .planning/phases/15-drift-erkennung-der-maschinenkonfiguration/15-01-PLAN.md
  - .planning/phases/15-drift-erkennung-der-maschinenkonfiguration/15-01-SUMMARY.md
  - .planning/phases/15-drift-erkennung-der-maschinenkonfiguration/15-02-PLAN.md
  - .planning/phases/15-drift-erkennung-der-maschinenkonfiguration/15-02-SUMMARY.md
  - .planning/phases/15-drift-erkennung-der-maschinenkonfiguration/15-03-PLAN.md
  - .planning/phases/15-drift-erkennung-der-maschinenkonfiguration/15-03-SUMMARY.md
  - .planning/phases/15-drift-erkennung-der-maschinenkonfiguration/15-04-PLAN.md
  - .planning/phases/15-drift-erkennung-der-maschinenkonfiguration/15-04-SUMMARY.md
  - .planning/phases/15-drift-erkennung-der-maschinenkonfiguration/15-05-PLAN.md
  - .planning/phases/15-drift-erkennung-der-maschinenkonfiguration/15-05-SUMMARY.md
  - .planning/phases/15-drift-erkennung-der-maschinenkonfiguration/15-06-PLAN.md
  - .planning/phases/15-drift-erkennung-der-maschinenkonfiguration/15-06-SUMMARY.md
  - .planning/phases/15-drift-erkennung-der-maschinenkonfiguration/15-07-PLAN.md
  - .planning/phases/15-drift-erkennung-der-maschinenkonfiguration/15-07-SUMMARY.md
  - .planning/phases/15-drift-erkennung-der-maschinenkonfiguration/15-08-PLAN.md
  - .planning/phases/15-drift-erkennung-der-maschinenkonfiguration/15-08-SUMMARY.md
  - .planning/phases/15-drift-erkennung-der-maschinenkonfiguration/15-09-PLAN.md
  - .planning/phases/15-drift-erkennung-der-maschinenkonfiguration/15-09-SUMMARY.md
  - .planning/phases/15-drift-erkennung-der-maschinenkonfiguration/15-10-PLAN.md
  - .planning/phases/15-drift-erkennung-der-maschinenkonfiguration/15-10-SUMMARY.md
  - .planning/phases/15-drift-erkennung-der-maschinenkonfiguration/15-11-PLAN.md
  - .planning/phases/15-drift-erkennung-der-maschinenkonfiguration/15-11-SUMMARY.md
  - cmd/holzkube-managerd/main.go
  - docs/guide.md
  - internal/configdrift/apply.go
  - internal/configdrift/configdrift.go
  - internal/configdrift/decide.go
  - internal/configdrift/detail.go
  - internal/configdrift/expected_test.go
  - internal/configdrift/health.go
  - internal/configdrift/lifecycle.go
  - internal/machineconfig/snapshot.go
  - web/src/components/AppShell.tsx
  - web/src/routes/drift-wiring.test.tsx
  - web/src/routes/node-detail.tsx
covered_digest: "v3:sha256:f2f2ead5238af25364b2c7d9175b0ca8bdfc8bb9c66dd4584a70d0af923a4665"
behavior_unverified: 0
overrides_applied: 0
re_verification:
  previous_status: gaps_found
  previous_score: 8/9
  gaps_closed:
    - "Eigene Applies mit reboot/auto landen nach dem Takt von 3 h als Basis (AdoptWindow 24 h), D-13 gehalten (DRIFT-04, D-10)"
    - "W1: Verwendung von ConfigDriftBanner (AppShell) und NodeConfigDrift (node-detail) durch Rendertest gehalten"
  gaps_remaining: []
  regressions: []
gaps: []
deferred: []
advisory: []
human_verification:
  - test: "Echter Knoten, Modi reboot und auto: Konfiguration anwenden, Neustart abwarten, die naechste Lesung des Daemons ansehen (Takt 3 h oder Daemon-Neustart + 1 min)"
    expected: "Knoten steht ok mit Quelle apply, nicht drift; zweimal lesen vorher ohne Drift; identische Bytes ohne Drift (A1, A2, Timing; Ledger 177)"
    why_human: "talossim wendet jeden Modus sofort an und zeigt weder Neustart noch Wartezeit; die Regel ist nur mit gesetzten Datensaetzen und vorgestellter Uhr belegt"
  - test: "Browser auf dem Pi: Knotenseite (Zustand, Diff), Clusterkarte, Banner, Klickpfad Bestaetigen mit Sudo-Dialog"
    expected: "Aussehen, Layout, Tippziele (44 px) in Ordnung; Bestaetigen macht den Zustand zur Basis und schreibt nichts auf den Knoten"
    why_human: "Der Rendertest haelt nur Verwendung und Text im jsdom, nicht Layout, Raster oder Tippziele; Browserprojekt und Layout-Waechter liefen nicht"
  - test: "Daemon-Lauf auf dem Pi (linux/arm64), Banner nach echten 24 h, go test -race dort"
    expected: "Die Lesung laeuft im echten Daemon, das Banner erscheint nach 24 h unbestaetigter Drift auf jeder Seite"
    why_human: "Verdrahtung nur ueber AST-Waechter und Testharnesse belegt; Banner nur mit vorgestellter Uhr; kein Pi, kein Daemon-Lauf in dieser Umgebung"
---

# Phase 15: Drift-Erkennung der Maschinenkonfiguration - Verifikationsbericht

**Phase Goal:** Der Betreiber sieht, wenn die aktive Konfiguration eines Knotens von dem abweicht, was holzkube-manager zuletzt gesetzt oder gelesen hat; periodische Lesung nach `internal/certwatch`, Anzeige auf Knotenseite (mit Diff), Clusterkarte, `/metrics`, Banner nach 24 h unbestaetigt, Bestaetigen "so ist es gewollt" (neue Basis, Admin, Sudo, Audit, kein Schreiben auf den Knoten); nie ein stilles "keine Drift", wo die Lesung fehlschlug.
**Verified:** 2026-10-15
**Status:** human_needed (nach Re-Verifikation 2026-10-15; Erstverifikation war gaps_found, 8/9)
**Re-verification:** Ja, nach Gap-Closure-Plan 15-11 (siehe Abschnitt unten). Die Tabellen darunter sind der Stand der Erstverifikation, Wahrheit 5, 6, 7 und 9 sind durch die Re-Verifikation ueberholt. Erstverifikation: Wo die Verifikation lief: Mac, Darwin arm64, Go 1.27.2 (Linter mit go1.26.9). Kein Pi, kein echter Knoten, kein Browser, kein Daemon-Lauf.

## Re-Verifikation 2026-10-15 (nach Plan 15-11: 442e0b6, 5801be9, f60a468, 33846e6; nichts gepusht)

Wo: Mac, Darwin arm64, Go 1.27.2 (Linter mit go1.26.9), Node 26 mit `NODE_OPTIONS=--no-experimental-webstorage`. Kein Pi, kein echter Knoten, kein Browser, kein Daemon-Lauf. Das SUMMARY wurde nicht als Beleg genommen; alles Folgende ist neu gelesen oder gemessen.

### Gap 1 (DRIFT-04, D-10): geschlossen

Code gelesen (`decide.go`): `adopts(e, now)` = staged immer; reboot/auto `age < AdoptWindow` (`AdoptWindow = BannerAfter = 24 h`, `configdrift.go:67`); alles andere (no-reboot) `age < SettleWindow`; try wird in `apply.go` gar nicht erst als Erwartung notiert (`modeTry`) und deshalb nie uebernommen. `settles` ist unveraendert 15 min und schliesst staged aus; `stale = !adopts`. Die Landungsregel fragt `adopts`; der D-13-Zweig (offene Drift, `AppliedOnDriftAt`, `Expected = nil`, Basis und `DriftSince` bleiben, Zustand drift) steht unveraendert darin. Eine Lesung der alten Fassung (Digest gleich Basis) behaelt eine noch zaehlende Erwartung (`spent` nur bei `stale`).

Eigene Injektionen (Build und `go vet` vorher je Exit 0, Test-Exit vom Befehl `go test ./internal/configdrift/ -count=1`, Wiederherstellung mit `cmp`, `git status` sauber; Ausgangslage gruen, Exit 0):

| # | Eingriff in `decide.go` (1 Zeile, Diffstat 1/1) | Exit | rot |
|---|---|---|---|
| a | reboot/auto zaehlen nur bis `SettleWindow` (alte 15-Minuten-Regel) | 1 | `TestAnApplyWithARestartIsAdoptedWhenTheNextReadingOnlyComesWithThePass` (reboot, auto), `TestAnExpectationStillCountsUntilItRunsOut`, `TestAnOwnApplyLandingHoursLaterOnAnOpenDriftKeepsItOpen` (3 Zeilen) |
| b | D-13-Zweig tot (Landung auf offener Drift loescht `DriftSince`) | 1 | `TestAnOwnApplyLandingOnAnOpenDriftKeepsItOpen`, `TestConfirmingAfterALandingMakesTheLandedStateTheBaseline` (2 Zeilen), `TestAnOwnApplyLandingHoursLaterOnAnOpenDriftKeepsItOpen` (3 Zeilen) |

Ein erster Versuch von (b) hat wegen einer Einrueckung nicht injiziert (Assertion im Skript, `git diff` leer, Exit 0): das habe ich als Nicht-Messung verworfen und wiederholt; kein `build failed`/`undefined` in einer der Ausgaben.

Leitfaden und Ledger 177 gelesen: der Leitfaden nennt die Fristen je Modus (24 h reboot/auto, 15 min no-reboot, staged bis zum Boot, try nie), den Preis (derselbe Byte-Apply eines Dritten im Fenster ist nicht unterscheidbar), dass eine offene Drift nicht durch eine Landung geschlossen wird, und dass das Timing nur mit gesetzten Datensaetzen und vorgestellter Uhr belegt ist. Ledger 177 traegt das Timing in Beschreibung, SCHLIESSBEDINGUNG (Neustart abwarten, dann die naechste Lesung) und Grund; Status offen; Tabelle und JSON zeichengleich (`TestTheLedgersTwoHalvesSayTheSameThing` gruen). Kein Ueberanspruch. Zwei Kleinigkeiten, kein Gap: der Leitfaden sagt "`settling` stays at 15 minutes for every mode", obwohl staged nie settelt (`settles` schliesst es aus); und die 24 Stunden im Leitfaden hat kein Test an `AdoptWindow` gebunden (SUMMARY nennt das selbst).

### W1: Verdrahtung gehalten

`web/src/routes/drift-wiring.test.tsx` rendert den echten `routeTree` bei `/nodes/m-cp-1` mit gespiegelten API-Antworten (Fixture, Zod-geparst) und einem `fetch`, das jede nicht gespiegelte Adresse laut nennt. Ausgangslage 2/2 gruen (Exit 0). Eigene Injektionen, `cd web && NODE_OPTIONS=--no-experimental-webstorage npx vitest run --project jsdom src/routes/drift-wiring.test.tsx > Datei`, `cmp` danach identisch:

| # | Eingriff | Exit | rot |
|---|---|---|---|
| c | Zeile `<ConfigDriftBanner className="px-4 pt-2" />` aus `AppShell.tsx` | 1 | "the shell shows the configuration banner on a node page": `Unable to find an element with the text: cp-1 has differed ...` (Assertion, 1 failed / 1 passed) |
| d | Zeile `<NodeConfigDrift machine={m.id} ...>` aus `node-detail.tsx` | 1 | "the node page asks for the configuration of its own machine and shows it": `Unable to find an element with the text: This node differs ...` (1 failed / 1 passed) |

Der Test haelt Verwendung und Text im jsdom, nicht Aussehen, Raster, Tippziele.

### Regression (Mac)

| Befehl | Exit | Ergebnis |
|---|---|---|
| `go build ./...`, `go vet ./...`, `gofmt -l .` | 0, 0, 0 | leer |
| `go test` configdrift machineconfig talos inventory planning publicrepo cmd/holzkube-managerd `-count=1` | 0 | alle ok |
| `go test -race ./internal/configdrift/ -count=1` | 0 | ok (nur Mac, nur dieses Paket; auf dem Pi nicht moeglich/nicht gelaufen) |
| golangci-lint v2.13.1 mit go1.26.9 `run ./...` | 0 | `0 issues.` |
| vitest `--project jsdom` komplett | 0 | 71 Dateien, 929 Tests (vorher 70/927) |
| `tsc --noEmit` / `biome check src` | 0 / 0 | biome: die alten 2 Warnungen + 1 Info, keine in der neuen Datei |
| `git diff --name-only eba8d70 -- internal/changelog internal/certwatch internal/machineconfig/diff.go` | leer | D-09, Changelog und certwatch unberuehrt |

Nicht gelaufen: `go test ./internal/httpapi` (die vier bekannten `web/dist`-Tests), `task ci`, vitest-Browserprojekt, Layout-Waechter.

### Neue Luecken: keine

Keine Debt-Marker, keine Regression, kein neuer Blocker gefunden. Der Status ist `human_needed`, weil die verbleibenden Punkte nur ein Mensch am echten System belegen kann (Frontmatter `human_verification`).

### Empfehlung zu DRIFT-01..09

- **Durch die Evidenz als erledigt markierbar (Code, Tests und rot gesehene Waechter):** DRIFT-01, DRIFT-02, DRIFT-03, DRIFT-06 (Zahlen und Karte; Layout offen), DRIFT-08, DRIFT-09 (Dokumentation vorhanden, Ledger ehrlich; keine Betreiberwerte).
- **Im Code geschlossen, aber bis zur Pruefung auf dem Pi Pending lassen:** DRIFT-04 (reboot/auto-Timing und A1/A2 nur mit gesetzten Datensaetzen, Ledger 177), DRIFT-05 (Knotenseite: Verwendung gehalten, Aussehen/Klickpfad im Browser ungeprueft), DRIFT-07 (Banner: Regel durch Injektion H gehalten, echte 24 h und Anzeige im Browser ungeprueft). Wenn der Betreiber "Code und Tests" als Massstab nimmt, ist auch das markierbar; ehrlicher ist, diese drei bis zum Lauf auf dem Pi offen zu halten.

## Ergebnis in einem Absatz (Erstverifikation)

Die Kette Lesung -> Basis -> `GET .../config/drift` -> `config_drift` auf der Clusterkarte -> `/metrics` -> Banner -> `POST .../confirm` ist in jedem Hop im Code aufgerufen, nicht nur deklariert, und die meisten Waechter wurden von mir mit eingesetztem Fehler rot gesehen. Die Randregeln "nie stilles keine Drift", Geheimnisse, Audit, Sudo und die Entscheidungen D-01..D-14 halten. Gefunden habe ich eine echte Luecke in D-10/DRIFT-04: ein Apply mit Neustart (reboot, auto) wird als Drift gemeldet, sobald die erste Lesung nach dem Neustart erst mit dem 3-Stunden-Takt kommt. Dazu eine Warnung: die Verdrahtung des Banners in `AppShell.tsx` und des Abschnitts in `node-detail.tsx` wird von keinem Test gehalten (gemessen).

## Goal Achievement

### Observable Truths

| # | Wahrheit | Status | Evidenz |
|---|----------|--------|---------|
| 1 | Kette Ende zu Ende: Aenderung durch Dritten am Knoten -> Leser -> API (Zustand + Diff) -> Clusterkarte `config_drift` -> `/metrics` -> Banner (24 h, serverseitig) -> Confirm (Admin, Destructive/Sudo, Audit `config.drift-confirm`, Digest-CAS, ohne Knoten) | VERIFIED | `main.go`: `configdrift.New` (Z. ~300), `inv.SetApplyObserver(drift.ApplyObserver())`, `inv.SetConfigDrift(drift.HealthFor)`, `provision.Deps{ForgetConfig: drift.Forget}`, `Drift: drift` in `httpapi.Deps`, `handlers.ConfigDriftRoutes(deps)` in `routeTable`, `go drift.Run(driftCtx, time.Minute)` mit `context.WithCancel(context.Background())` (keine Frist; jede Lesung bekommt in `readNode` ihr Budget 20 s). `inventory.Connect` haengt den Beobachter an jeden Client (`mutate.go`). `ClusterView.ConfigDrift` -> `Clusters()`/`Cluster()`; Exporter liest `inv.Clusters` (`main.go:632`). `TestTracerTheDriftRouteShowsTheStateAndTheDiffOfARealNode` (httpapi) laesst einen Dritten `KubeletConfig.image` am talossim aendern, `Tick`, GET zeigt `drift` samt Diff-Pfad `KubeletConfig.image`, Clusterliste traegt `config_drift` ohne Konfigurationsinhalt. Confirm: `Destructive: true`, `MinRole: Admin`, `Action: config.drift-confirm`, Digest als Regex und als CAS in `confirmable` (zweimal geprueft, einmal unter `storeMu`). `Confirm` hat keine Abhaengigkeit, die den Knoten beschreiben koennte (`Deps` hat nur `Read`, das nicht in Confirm gerufen wird; Injektion B). Alle Pakete gruen (siehe Gate). |
| 2 | D-01..D-14 eingehalten | VERIFIED (mit Luecke zu D-10, siehe Wahrheit 5) | D-01/02: `Snapshot` = `Redact` + Normalisierung, `Baselines.Put` nur mit `snap.Text`; D-03/12: keine Ausnahmeliste im Code (`TestEveryChangeCountsNoPathIsExcepted`); D-04: `Run`/`Tick`/`Flush`/`Poke`, kein Watch; D-05: vier Flaechen vorhanden; D-06: `BannerAfter = 24h`, `HealthFor` rechnet ab `Since` (Record), nicht ab letzter Lesung; D-07: nur Confirm, kein Reset-Pfad; D-08: `CurrentVersion = 7`, Migration 6->7 legt `config-baselines/` an, Fixtures `version-6`; D-09: `git diff eba8d70 -- internal/machineconfig/diff.go` leer, `CompareDocuments` in `snapshot.go`; D-11: `Destructive`; D-13: Zweig in `decide.go` (`DriftSince` bleibt, `AppliedOnDriftAt`, `Expected = nil`, Zustand drift), Injektion C rot; D-14: Rohfelder wie die Config-Ansicht, Ledger 179 und Leitfaden nennen es. |
| 3 | Nie ein stilles "keine Drift": Lesefehler, ungelesen, beschaedigte Basis, Knoten aus, Cluster aus, Teilabdeckung | VERIFIED | `unreadable` setzt `StateUnreadable` und behaelt `since`/`lastGood`; `damaged` schreibt nie zurueck; `HealthFor` liefert `nil` bis zu einem Durchlauf ("not read yet"); `Judged()` zaehlt `not-read`/`unreadable`/`baseline-damaged` ohne Drift nie als Urteil; Cluster-Metrik 0 nur bei `judged == h.Nodes`. Injektionen A (Lesefehler als ok: 10+ Tests rot, u. a. `TestAFailedReadIsNeverNoDrift`) und G (Cluster 0 bei Teilabdeckung: `TestAClusterIsZeroOnlyWhenEveryNodeHasAVerdict` rot). Ausgeschaltete Cluster/Knoten werden nicht gelesen; ihre letzte Lesung bleibt mit Alter (Leitfaden nennt es, `read_age_seconds` waechst); siehe Info 2. |
| 4 | Geheimnisse: nichts Unredigiertes in `config-baselines/`, API, `/metrics`, Audit, Logs; Allowlist, Reader-Satz ohne Adresse/Pfad; Wand unberuehrt | VERIFIED | `Snapshot` nur ueber `Redact`, PEM-Guertel `ContainsPrivateKey` zweimal (Snapshot, `decide`), Deckel 1 MiB. `TestNoSecretReachesTheBaselineFile` liest die echte Datei; Injektion L (Redact uebersprungen) laesst 6 Tests in `machineconfig` und mehrere in `configdrift` rot werden. Audit: Allowlist `config.drift-confirm: {digest}`, Knoten aus dem Pfad nach der Redaktion (`MachineFromPath`), Injektion I rot (`TestTheBodyCannotNameTheNodeInTheAuditRecord`). Clusterliste: `listSentence` ersetzt Fehlertexte (Adresse/Pfad) durch feste Saetze; die Diff-Route ist Operator+. Metrik-Labels: Cluster-ID und Hostname, nie Machine-ID. Wand: `git diff eba8d70..HEAD` auf `internal/httpapi/handlers/wall*`, `kubernetes.go`, `web/src/routes/wall.tsx`, `internal/certwatch`, `internal/changelog` leer; `TestTheWallCarriesNoConfigDrift` vorhanden. Logzeilen tragen Cluster, Knoten, Machine-ID, Zustand; Fehlertexte nur aus Transport/Store (kein Konfigurationsinhalt, `ErrSnapshot*` haben feste Saetze). Restrisiko: D-14 (Rohfelder liegen wortgleich auf der Platte), vom Betreiber entschieden, Ledger 179. |
| 5 | Eigene Applies verschieben die Basis richtig fuer no-reboot, reboot, auto, staged, try (DRIFT-04, D-10) | FAILED (partial) | no-reboot, staged (ohne Fenster), try (nie uebernommen), D-13 und die Aufrufer (Service.Apply, upgrade.applyImage, rotateca.rewrite; Quelltext-Waechter `knownSenders`) sind belegt. reboot/auto mit Neustart: Landung wird nach 15 min nicht mehr uebernommen, naechste Lesung kommt mit dem 3-h-Takt -> falsche Drift (Wegwerf-Probe, siehe Gap). Fail-safe-Richtung (Fehlalarm, nie verschluckt), aber der Betreiber muss jeden solchen Apply bestaetigen und bekommt nach 24 h ein Banner. |
| 6 | Verdrahtung: nichts nur deklariert | VERIFIED (mit Warnung W1) | Alle Hops oben aufgerufen. Waechter rot gesehen: `SetConfigDrift` entfernt, `SetApplyObserver` entfernt, `go drift.Run` auf Kontext mit Frist, `ForgetConfig` entfernt, `Drift:` im Deps entfernt (jeweils `TestTheDaemonWiresTheDriftWatch` rot, Meldung nennt das Stueck), `ConfigDriftRoutes` aus `routeTable` (zwei Tests rot). Web: `ConfigDriftRow` in `clusters.tsx` ist von `clusters.test.tsx` gehalten (Entfernung: 1 Test rot). `AppShell.tsx` (`<ConfigDriftBanner>`) und `node-detail.tsx` (`<NodeConfigDrift>`) sind NICHT gehalten: Entfernung ergibt vitest 927/927 gruen (beide gemessen). Die Verwendungszeilen existieren. |
| 7 | Dokumentation: README, Leitfaden, API-Vertrag; ehrliche Grenzen; Changelog unberuehrt; keine Betreiberwerte; Ledger 176..179 offen | VERIFIED (eine Ungenauigkeit) | README: Absatz in "What it does" (kein Bild, vom Betreiber so entschieden). Leitfaden: Abschnitte "Schema 7 and going back" und "Configuration drift" mit sieben Zustaenden, A1/A2, Geheimniswerte, Rohfelder, Schema-7-Rollback, staged/try nur auf Entscheidungsebene, Plan liest nur ein Dokument. API-Vertrag: beide Routen, drei 409-Codes, `config_drift`, Audit, Metriken. `internal/changelog` unberuehrt. `internal/publicrepo` gruen. Ledger: 176 defect, 177 unrun-verify, 178 gap, 179 gap, alle `open`; Kopf `open_count: 26`, `total_count: 179`, gezaehlt 26/19/134 (+4). Ungenauigkeit: der Leitfaden sagt, ein Apply werde Basis, "when that version shows up on the node"; fuer reboot/auto gilt das nur im Fenster (Gap). |
| 8 | Ehrlichkeit der SUMMARYs: was NICHT belegt ist, steht da | VERIFIED | 15-05 und 15-10 benennen A1/A2, staged/try nur auf Entscheidungsebene, kein Browser/Layout/Tippziele, kein Pi, `-race` nur drei Pakete auf dem Mac, Banner nur mit vorgestellter Uhr, kein Daemon-Lauf, Rollback nach Schema 7 aus dem Code gelesen. 15-09 nennt das Fehlen der Render-Tests fuer AppShell/node-detail (bestaetigt). Nicht genannt: das Reboot-Timing (Gap), das die SUMMARYs als "richtig auch fuer reboot/auto" (D-10) durchgehen lassen; das ist kein Vorsatz, sondern eine Annahme, die kein Simulator zeigen konnte. |
| 9 | Gate laeuft wie behauptet | VERIFIED | siehe unten |

**Score:** 8/9 Wahrheiten verifiziert, 0 present-but-behavior-unverified.

### Gate (selbst ausgefuehrt; Exitcodes aus dem Testbefehl, in Dateien umgeleitet)

| Befehl | Exit | Ergebnis |
|---|---|---|
| `go build ./...`, `go vet ./...`, `gofmt -l .` | 0, 0, 0 | sauber, `gofmt` ohne Ausgabe |
| `go test` configdrift machineconfig talos inventory store/... model metrics provision upgrade rotateca planning publicrepo cmd/holzkube-managerd `-count=1` | 0 | alle `ok` |
| `go test ./internal/httpapi -count=1` | 1 | genau die 4 erwarteten `web/dist`-Fehler (TestCSPHashMatchesTheServedShell, TestForeignHostIsRefused, TestNoAssetIsInlinedPastTheContentSecurityPolicy, TestFontsAreServedAsFiles); `handlers`, `middleware` ok |
| `go test -race` configdrift talos inventory | 0 | alle `ok` (nur diese drei, nur Mac) |
| vitest `--project jsdom` | 0 | 70 Dateien, 927 Tests |
| `npx tsc --noEmit` / `npx biome check src` | 0 / 0 | biome: 2 Warnungen + 1 Info in `DataTable.tsx`, `wall.test.tsx`, nicht von der Phase |
| golangci-lint v2.13.1 mit go1.26.9 | 0 | `0 issues.` |

Nicht behauptet und nicht gelaufen: `-race` ueber das ganze Repo, `task ci`, vitest-Browserprojekt, Layout-Waechter, Pi.

### Waechter mit eingesetztem Fehler rot gesehen (eigene Injektionen; Original gesichert, danach `cmp` identisch, `git status` sauber)

Jede Injektion hat tatsaechlich injiziert (Diffstat 1 Datei) und das Testergebnis kam vom Befehl selbst; keine war ein reiner Buildfehler (ausser dass ich dies geprueft habe: keine `build failed`-Zeile).

| # | Fehler eingesetzt | Befehl | Exit | Rot |
|---|---|---|---|---|
| A | Lesefehler wird `StateOK` (`decide.go`) | `go test ./internal/configdrift` | 1 | u. a. `TestAFailedReadIsNeverNoDrift`, `TestANodeNeverReadAndNotAnsweringHasNoBaseline`, `TestADriftingNodeStaysDriftingWhenItStopsAnswering` |
| B | Confirm liest den Knoten (`lifecycle.go`) | dito | 1 | `TestTracerConfirmingTurnsTheReadingIntoTheBaselineAndTouchesNothing` (ein echter Schreibzugriff ist ohne neue Abhaengigkeit nicht einsetzbar; `Deps` hat keinen Apply-Weg) |
| C | Landung loescht offene Drift (D-13-Zweig aus) | dito | 1 | `TestAnOwnApplyLandingOnAnOpenDriftKeepsItOpen`, `TestConfirmingAfterALandingMakesTheLandedStateTheBaseline` |
| E | Lesebudget entfernt (`WithTimeout` -> `WithCancel`) | dito | 1 | `TestEveryReadCarriesADeadline`, `TestAPokedNodeIsReadWithoutWaitingForTheTick` |
| D1-D5 | `SetConfigDrift` / `SetApplyObserver` / `go drift.Run` mit Frist / `ForgetConfig` / `Drift:` im Deps weg (`main.go`) | `go test ./cmd/holzkube-managerd -run TestTheDaemonWiresTheDriftWatch` | je 1 | `TestTheDaemonWiresTheDriftWatch` mit der Meldung des Stuecks |
| D6 | `ConfigDriftRoutes` aus `routeTable` | `go test ./cmd/holzkube-managerd` | 1 | `TestEveryAllowlistedActionIsAReachableRoute`, `TestTheDaemonWiresTheDriftWatch` |
| G | Cluster-Metrik 0 bei Teilabdeckung | `go test ./internal/metrics` | 1 | `TestAClusterWithPartialCoverageIsOneOrAbsent`, `TestAClusterIsZeroOnlyWhenEveryNodeHasAVerdict` |
| H | Banner nach 1 h statt 24 h | `go test ./internal/configdrift` | 1 | `TestTheBannerWaitsTwentyFourHours`, `TestTheBannerCountsFromTheStartOfTheDriftNotTheLastReading` |
| I | Knoten im Audit nicht aus dem Pfad | `go test ./internal/httpapi/...` | 1 | `TestTheConfirmationNamesTheNodeAndTheDigestInTheAuditRecord`, `TestTheBodyCannotNameTheNodeInTheAuditRecord` |
| J | Confirm-Route nicht `Destructive` | dito | 1 | `TestConfirmingNeedsTheSudoWindow` |
| L | `Snapshot` ohne `Redact` | `go test ./internal/machineconfig ./internal/configdrift` | 1 | 6 Tests in machineconfig (u. a. `TestNoSecretInTheSnapshot`, `TestTheSnapshotHoldsNoMoreAndNoLessThanTheConfigView`) und mehrere in configdrift |
| K1 | `<ConfigDriftBanner>` aus `AppShell.tsx` | vitest jsdom | 0 | KEIN Test rot (927/927): ungehalten |
| K2 | `<NodeConfigDrift>` aus `node-detail.tsx` | dito | 0 | KEIN Test rot: ungehalten |
| K3 | `<ConfigDriftRow>` aus `clusters.tsx` | dito | 1 | `clusters.test.tsx` "has a configuration row ..." |

## Required Artifacts / Key Links (Stichprobe der Hops)

| Von | Nach | Via | Status |
|---|---|---|---|
| `main.go` | `inventory.Service` | `SetApplyObserver`, `SetConfigDrift` (je genau einmal, AST-Waechter) | WIRED |
| `inventory.Connect` | `talos.ClusterClient` | `cc.SetApplyObserver(observer)` fuer jeden Client; beide Sendemethoden rufen `observeApply` | WIRED |
| Konfigurationsdienst, `upgrade.applyImage`, `rotateca.rewrite` | `inv.Connect` | `Connect:` in `main.go` (Z. 277, 404, 504, 605); Quelltext-Waechter `knownSenders` | WIRED |
| `provision` (Schritt apply) | `drift.Forget` | `Deps.ForgetConfig` vor dem Schreiben | WIRED |
| `Service.HealthFor` | Clusterliste und `/metrics` | `ClusterView.ConfigDrift` -> `Clusters()`; `metrics.Deps.Clusters: inv.Clusters` | WIRED |
| `handlers.ConfigDriftRoutes` | `routeTable` | `main.go` | WIRED |
| `AppShell.tsx` -> `ConfigDriftBanner`; `node-detail.tsx` -> `NodeConfigDrift`; `clusters.tsx` -> `ConfigDriftRow` | Server `config_drift` / `GET .../config/drift` | JSX-Verwendung, `api.ts` | WIRED (Verwendung ohne Test fuer die ersten beiden) |

Data-Flow (Level 4): Lesung -> `reading.snap` (Speicher) -> `Detail`/`HealthFor` -> JSON -> Komponenten; Baseline aus dem Store. Keine statische Rueckgabe, kein Mock im Produktionspfad: FLOWING.

## Requirements Coverage

| Requirement | Status | Evidenz / Bemerkung |
|---|---|---|
| DRIFT-01 erste Lesung legt Basis an und sagt es; "not read yet" nie "keine Drift" | SATISFIED | `decide` (baseline-created), `HealthFor` nil, Injektion A |
| DRIFT-02 redigierter, normalisierter Schnappschuss, Schema 6->7, beschaedigte Datei gemeldet | SATISFIED | Snapshot/fsstore/migrate, Injektion L, `TestADamagedBaselineIsReportedAndNeverOverwritten` |
| DRIFT-03 periodisch + nach Apply, eigene Frist auch auf Kontext ohne Frist, ungelesen zaehlt als nicht gelesen | SATISFIED | `Run`/`readNode`, Injektion E, D3 |
| DRIFT-04 eigene Applies, alle Modi; neu provisionierte Maschine erbt keine Basis | PARTIAL | Gap (reboot/auto mit Neustart); `Forget` bei Provisionierung belegt |
| DRIFT-05 Knotenseite mit Zustand und Diff ueber alle Dokumente | SATISFIED (Verwendung ungehalten, W1) | `CompareDocuments`, HTTP-Tracer mit `KubeletConfig.image` |
| DRIFT-06 Zeile "Configuration"; `/metrics` 0/1, nie 0 fuer nicht gelesen | SATISFIED | Injektion G, K3 |
| DRIFT-07 Banner auf jeder Seite erst nach 24 h; Beginn ueberlebt Neustart | SATISFIED (Verwendung ungehalten, W1) | `TestTheStartOfADriftSurvivesARestart`, Injektion H |
| DRIFT-08 Bestaetigen: Admin, Sudo, Audit mit Knoten und Digest, Knoten unberuehrt, kein Reset | SATISFIED | Injektionen B, I, J |
| DRIFT-09 README, Leitfaden, API-Vertrag; Grenzen im Ledger | SATISFIED bis auf die Reboot-Ungenauigkeit | Ledger 176..179, README-Absatz; Leitfadensatz zur Landung nach Gap-Fix angleichen |

Verwaiste Requirements: keine (alle neun von Plaenen beansprucht).

## Anti-Patterns

Debt-Marker (TBD/FIXME/XXX) in den geaenderten Dateien: keine gefunden. Keine Stubs, keine leeren Handler; die `return nil` des Beobachters (`try`, nicht snapshotbar) sind dokumentierte Entscheidungen.

## Warnungen und Hinweise

**W1 (WARNING) Verdrahtung in `AppShell.tsx` und `node-detail.tsx` ungehalten.** Gemessen: `<ConfigDriftBanner>` bzw. `<NodeConfigDrift>` entfernt, vitest 927/927 gruen. Genau die "declaration-not-use"-Klasse; die Komponenten haben eigene Tests, ihre Verwendung nicht. Ein Guard sollte ergaenzt werden (der Orchestrator plant dafuer einen Quick-Task: ja, sinnvoll). Empfehlung: (a) ein Renderttest fuer `AppShell` mit gemocktem `api.clusters.list` (ein Cluster mit `config_drift.banner: true`) -> Banner-Satz erscheint, und fuer `NodeDetailPage` mit gemocktem `GET .../config/drift` -> Abschnitt erscheint; falls das Rendern der ganzen Seite zu schwer ist, (b) ein Quelltext-Test, der `AppShell.tsx` und `node-detail.tsx` liest, Kommentare entfernt und `<ConfigDriftBanner` bzw. `<NodeConfigDrift machine=` verlangt. Danach nach Muster der Methode selbst rot sehen (Zeile entfernen -> Test rot, wiederherstellen, `cmp`). Der Browser (Layout, Tippziele 44 px, Aussehen) bleibt fuer die Phase unverifiziert, das nimmt auch dieser Guard nicht ab.

**Info 2 Ausgeschalteter Cluster, zuletzt gelesen.** Sein letztes `ok` bleibt als Urteil (Metrik 0 fuer Knoten und, bei voller Abdeckung, Cluster), bis der Daemon neu startet; `read_age_seconds` waechst, die Karte nennt "Last read". Das ist im Leitfaden so beschrieben ("its last reading stays, with its age") und kein stilles Ergebnis, aber ein Alarm auf `cluster_config_drift == 0` allein bleibt dort stumm. Kein Fix noetig; Alarmregeln sollten das Alter einbeziehen (Leitfaden koennte es ausdruecklich sagen).

**Info 3** Linter-Warnungen von biome sind nicht von der Phase.

## Nicht belegt (Mensch/echte Umgebung noetig; aendert den Status nicht, da Gap Vorrang hat)

- A1/A2 gegen echte Knoten (Ledger 177), inklusive des Reboot-Timings aus dem Gap.
- Browser: Aussehen, Layout, Tippziele, Klickpfad Knotenseite -> Bestaetigen -> Sudo-Dialog.
- Das Banner nach echten 24 h (getestet mit vorgestellter Uhr; die Regel selbst ist durch H rot gesehen).
- Ein Lauf des Daemons selbst (Verdrahtung nur ueber den AST-Waechter und die Testharnesse); Pi/arm64; `-race` ueber das ganze Repo; `task ci`.
- Der Weg zurueck aus Schema 7 (aus Code und Leitfaden gelesen, nicht ausgefuehrt).

## Gaps Summary

Eine Luecke: DRIFT-04/D-10 fuer Applies mit Neustart. Der Fix ist klein (Adoption einer reboot/auto-Erwartung unabhaengig vom 15-min-Settling-Fenster, mit rotem Test zuerst, Leitfaden und Ledger 177 anpassen) und aendert nichts an Schema, API oder Oberflaeche. Alternativen, falls der Betreiber die Adoption aelterer Erwartungen nicht will: Folgeanstoesse nach 2, 5, 15 min nach einem erfolgreichen reboot/auto-Apply (deckt langsame Neustarts nicht ab) oder `jobs.AfterReboot` zusaetzlich an `drift.Poke` haengen (deckt nur Neustarts, die ueber die Jobs laufen). Empfehlung: die Adoption (erste Variante), weil sie die einzige ist, die kein Timing raten muss; der Preis (ein Dritter wendet spaeter genau dieselben Bytes an und wird nicht unterschieden) ist ohnehin schon in 15-05 benannt und unvermeidbar.

---

_Verified: 2026-10-15_
_Verifier: Claude (gsd-verifier)_
