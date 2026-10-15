---
phase: 15-drift-erkennung-der-maschinenkonfiguration
plan: 08
subsystem: httpapi, audit, daemon
tags: [drift, http, confirm, audit, wiring, ast-guard, api-contract]
requires: [15-04, 15-05, 15-06, 15-07]
provides:
  - "GET /api/v1/machines/{id}/config/drift (Operator) und POST .../config/drift/confirm (Admin, Destructive, Digest als Compare-and-Swap)"
  - "httpapi.Deps.Drift, handlers.ConfigDriftRoutes, CodeDriftNotRead, CodeNoDrift, CodeDriftChanged"
  - "audit.MachineParam und audit.MachineFromPath: der Knoten steht im Audit-Eintrag, serverseitig aus dem Pfad, nach der Redaktion"
  - "cmd/holzkube-managerd: der Drift-Dienst vollständig verdrahtet; TestTheDaemonWiresTheDriftWatch hält sechs Stücke einzeln"
  - "docs/api-contract.md: Abschnitt 'Configuration drift', drei Fehlercodes, Metriken-Ausnahme"
affects: [15-09, 15-10]
actuals:
  tokens: 17600
  tasks: 3
  commits: 3
plan_head_before: c4d0ae0ab07c398168bb1327b4a53bb6cba266f9
plan_head_after: 66da1a7ea7a89afaccea0e5a1fd970c8bdf553ed
tech-stack:
  added: []
  patterns: ["Route nach dem Muster backup-schedule (Admin, Destructive, kein ClusterScope)", "serverseitiger Audit-Parameter aus dem Pfad nach der Redaktion (wie AccountParam)", "AST-Wächter über main.go, ein Stück je Meldung", "Harness baut den Dienst so, wie der Daemon es tut"]
key-files:
  created:
    - internal/httpapi/handlers/configdrift.go
    - internal/httpapi/configdriftapi_test.go
    - cmd/holzkube-managerd/drift_wiring_test.go
  modified:
    - internal/httpapi/router.go
    - internal/httpapi/problem.go
    - internal/httpapi/endtoend_test.go
    - internal/httpapi/middleware/audit.go
    - internal/audit/redact.go
    - cmd/holzkube-managerd/main.go
    - cmd/holzkube-managerd/budget_test.go
    - cmd/holzkube-managerd/reachable_test.go
    - docs/api-contract.md
key-decisions:
  - "Der Audit-Eintrag nennt den Knoten als params.machine, vom Server aus dem Pfad geschrieben, nach der Redaktion; ein Body-Schlüssel machine bestimmt ihn nicht. Der Digest steht im Klartext, jeder andere Schlüssel redigiert"
  - "Das Bestätigen trägt kein ClusterScope: es schreibt nur lokalen Zustand und geht auch auf einem schreibgeschützt adoptierten Cluster"
  - "Der Drift-Dienst läuft auf context.WithCancel(context.Background()); der Wächter nimmt nur diese Form an (kein Deadline-Kontext, kein Request)"
requirements-completed: []
status: complete
---

# Phase 15 Plan 08: HTTP, Audit und Verdrahtung Summary

Die Drift ist über HTTP erreichbar (Lesen mit Diff für Operatoren, Bestätigen für Admins mit Sudo-Fenster und Digest als Compare-and-Swap), der Audit-Eintrag des Bestätigens nennt zum ersten Mal einen Knoten, und der Daemon verdrahtet den Dienst vollständig, was ein AST-Wächter über sechs Stücke von `main.go` einzeln hält.

BASE: c4d0ae0ab07c398168bb1327b4a53bb6cba266f9

`plan_head_before` ist derselbe Wert. `git rev-list --count BASE..HEAD` vor diesem SUMMARY-Commit: 3 (d4d68ed, 7b6c2c4, 66da1a7).

## Wo es lief

Mac (darwin/arm64), `go1.27.2`, kein echter Cluster, kein Pi, kein `task`, kein Browser, kein Daemon-Prozess. `go test -race ./cmd/... ./internal/audit/... ./internal/httpapi/... -count=1` lief hier: grün bis auf die vier Tests, die `web/dist` brauchen (`TestCSPHashMatchesTheServedShell`, `TestForeignHostIsRefused`, `TestNoAssetIsInlinedPastTheContentSecurityPolicy`, `TestFontsAreServedAsFiles`; sie scheitern auch ohne `-race` und sind nicht Gegenstand). Auf dem Pi läuft `-race` nicht; dort bleibt CI maßgeblich. CI-Linter `GOTOOLCHAIN=go1.26.9 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1 run ./...`: `0 issues.` Nichts gepusht, nichts getaggt, kein Changelog, nichts installiert.

## Was gebaut wurde

- **Task 1, Tracer (d4d68ed).** `handlers/configdrift.go` mit `ConfigDriftRoutes` und der GET-Route (`MinRole: Operator`, kein `budgetedContext`, Kommentar nennt `noUpstream`), `Deps.Drift`. Der Prüfstand bekommt `withDrift()` (impliziert den Konfigurationsdienst; das Inventar ist wie bei `withConfig` Sache des Aufrufers) und baut den Dienst so, wie der Daemon es tun soll: `Read: deps.Config.ReadSnapshot`, `inv.SetConfigDrift`, `inv.SetApplyObserver`, `deps.Drift` im Aufbau von `deps` vor der Routenliste; der Dienst steht im Feld `harness.drift`. Der Tracer läuft vom echten Router über Inventar, Talos-Client und Store bis zum Simulator: erst `baseline-created` mit 64-stelligem Digest, ohne Diff; nach einer Änderung eines Dritten (Client ohne Beobachter) `drift` mit `drift_since` und dem Pfad `KubeletConfig.image`, ohne Geheimniswert des Clusters, ohne PEM-Schlüssel, ohne `null` für `changes` und `paths`, und `config_drift.items` in der Clusterliste ist eine Liste, ohne Diff-Inhalt.
- **Task 2 (7b6c2c4).** POST `.../config/drift/confirm`: `Destructive`, `Admin`, `Action config.drift-confirm`, kein `ClusterScope`; Digest gegen `^[0-9a-f]{64}$`, sonst 400 mit Feldfehler `digest`; `Confirm`, danach `Detail` mit 200. Die drei Codes `conflict.drift-not-read`, `conflict.no-drift`, `conflict.drift-changed` (Konstanten mit Doc-Kommentar, Zeilen in der Code-Tabelle des Vertrags). `audit.MachineParam`, `machinePath`, `MachineFromPath`, der Allowlist-Eintrag `{"digest"}`, und `captureParams` setzt den Knoten nach der Redaktion. Zwölf neue Tests (`TestConfirming...`, `TestTheConfirmation...`, `TestTheBodyCannotName...`, `TestTheWallCarriesNoConfigDrift`).
- **Task 3 (66da1a7).** `main.go`: `configdrift.New` direkt nach `configSvc` (vor `provision.Register`), danach `inv.SetApplyObserver(drift.ApplyObserver())` und `inv.SetConfigDrift(drift.HealthFor)`; `ForgetConfig: drift.Forget` in `provision.Deps`; `Drift: drift` im Literal von `httpapi.Deps`; `go drift.Run(driftCtx, time.Minute)` auf `driftCtx, stopDrift := context.WithCancel(context.Background())` neben dem Zertifikats-Wächter; `handlers.ConfigDriftRoutes(deps)` in `routeTable`. `drift_wiring_test.go` (AST) hält die sechs Stücke einzeln. Zwei Einträge in `noUpstream`, eine Gruppe in `interfaceInFlight()`. Abschnitt „Configuration drift“ im API-Vertrag (Routen, sieben Zustände, `config_drift`, Banner, Audit, Fehlercodes, D-13, nie `null`, vier Metriken) und ein Satz im Absatz zu `/metrics`.
- Gegen BASE unberührt: `git diff --name-only c4d0ae0 -- internal/httpapi/handlers/kubernetes.go internal/httpapi/handlers/wall_certificates.go internal/httpapi/handlers/wall_host.go internal/httpapi/handlers/wall_trends.go internal/httpapi/handlers/walllink.go internal/changelog web` ist leer (gemessen nach jedem der drei Tasks, 0 Zeilen).

## Rot gesehen

Exitcodes sind die des `go test`-Befehls selbst (vom Skript `inj.py` aufgerufen, das den Exit des Unterprozesses ausgibt). Jede Injektion: die Ersetzung muss genau einmal treffen (sonst bricht das Skript ab), danach `go build ./... && go vet` mit Exit 0 **vor** dem Testlauf (sonst „NON-MEASUREMENT“), Wiederherstellung per `cp` und `cmp` (Exit 0, in jedem Lauf ausgegeben), danach grün. Zwei erste Läufe von Task 3 (a bis f) brachen wegen eines Fehlers im Aufruf meines Skripts (Quoting des Testbefehls) vor dem Test ab: kein Test lief, `main.go` wurde per `cmp` wiederhergestellt; die Läufe wurden wiederholt, nur die Wiederholung zählt.

### Task 1

RED gegen den Stand vor dem Eingriff: Exit 1, Compile-Fehler (`deps.Drift undefined`, `undefined: handlers.ConfigDriftRoutes`). GRÜN danach.

| Injektion | Test | Exit | scheiternde Zeile |
|---|---|---|---|
| (a) `MinRole` der GET-Route ist `RoleReader` | `TestTheDriftRouteIsForOperatorsAndUp` | 1 | `a reader reads the drift: 200, want 403: the diff is configuration content` |
| (b) der Handler schreibt die Antwort ohne `Diff` | `TestTracerTheDriftRouteShowsTheStateAndTheDiffOfARealNode` | 1 | `a drifting node's answer carries no diff` |
| (c) `ErrUnknownMachine` wird nicht abgebildet (`&& false`) | `TestTheDriftRouteIsForOperatorsAndUp` | 1 | `an unknown machine: 500 (...), want 404 notfound.record` |

### Task 2

RED gegen den Stand nach Task 1: Exit 1, elf Tests rot, alle mit 404 `notfound.route` (die POST-Route fehlt); `TestTheWallCarriesNoConfigDrift` war grün, wie es einer Beobachtung zukommt. Zwei Fehler der Tests selbst wurden im Lauf behoben und zählen nicht als Messung: der Lock-Body trug ein Feld `reason`, das die Route nicht kennt (400), und der Fall „kein Body“ schickte keinen Content-Type (CSRF-403 statt 400).

| Injektion | Test | Exit | scheiternde Zeile |
|---|---|---|---|
| (a) `Destructive` entfällt | `TestConfirmingNeedsTheSudoWindow` | 1 | `confirming without an open sudo window: 200, want 428` |
| (b) `MinRole` ist `RoleOperator` | `TestConfirmingIsForAdminsOnly` | 1 | `a operator confirmed the drift: 200, want 403` (und: `a refused confirmation changed the baseline`) |
| (c) der Pfad-Knoten wird nur gesetzt, wenn der Body keinen Schlüssel `machine` lieferte (nach der Redaktion) | `TestTheBodyCannotNameTheNodeInTheAuditRecord` | 1 | `the record names the node <redacted>, want the one in the path "..."` |
| (c2, zusätzlich) wie (c) und `machine` in der Allowlist | derselbe | 1 | `the record names the node somebody-elses-node, want the one in the path` und `the record carries "somebody-elses-node", which nothing ever removes from the archive` |
| (d) `machinePath` ohne die Action | `TestTheConfirmationNamesTheNodeAndTheDigestInTheAuditRecord` (und der Body-Test) | 1 | `the record names the node <nil>, want "...": an admin who confirmed a drift without a node is no record` |
| (e) Digest nicht in der Allowlist (`{}`) | `TestTheConfirmationNamesTheNodeAndTheDigestInTheAuditRecord` | 1 | `the record names the digest <redacted>, want "2ff43f...": it proves which state was confirmed` |
| (f) die Route trägt ein `ClusterScope` (Cluster der Maschine aus dem Store) | `TestConfirmingWorksOnAReadOnlyAdoptedCluster` (und die meisten übrigen, denn ein importierter Cluster ist schon gesperrt) | 1 | `confirming on a cluster that is locked: 403 (... The cluster is locked read-only ...), want 200: it writes local state only` |
| (g) der Handler liest vor dem Bestätigen mit `ReadSnapshot` | `TestConfirmingTouchesNothingAtTheNode` | 1 | `confirming made the node hear Get 1 times, want 0` und `... Version 1 times ...` |
| (h) die Digest-Prüfung entfällt (`false &&`) | `TestConfirmingRefusesABadDigest` | 1 | sechs Fälle `...: 409, want 400` (nicht hex, zu kurz, zu lang, Großbuchstaben, leer, kein Digest) |

Zu (c): die Injektion des Plans („der Parameter wird vor der Redaktion gesetzt“) ist als eine Änderung am Code nicht so einzuspielen, dass das Ergebnis etwas anderes wäre als `<redacted>`, weil die Redaktion jeden Schlüssel außerhalb der Allowlist löscht und das Setzen nach der Redaktion bei (c) wegfällt. Gemessen ist deshalb zweierlei: (c) der Pfadwert überschreibt nicht, was aus dem Body kam, und (c2) der Fall, der die Bedrohung T-15-30 wirklich ausmacht: ein erlaubter Body-Schlüssel `machine` würde den Eintrag bestimmen. Beide sind rot.

Der Wand-Test ist eine **Beobachtung** und kein Wächter: `TestTheWallCarriesNoConfigDrift` ist grün und wird von keiner Injektion in diesem Plan rot. Er läuft gegen den Prüfstand mit Kubernetes-Simulator (die Wand braucht ihn; ein erster Aufbau mit dem Konfigurations-Prüfstand lief in einen Timeout gegen `192.168.1.41:6443`, 502, und wurde ersetzt) und gegen einen Cluster, dessen Karte `config_drift` trägt; die Wand-Antwort (mit Wand-Link und mit Sitzung) enthält das Wort `drift` nicht. Der Zustand der Karte ist dort `baseline-created`, nicht `drift`, weil dieser Prüfstand den Simulator nicht herausgibt, mit dem ein Dritter eine Änderung machen könnte.

### Task 3

RED gegen den Stand vor dem Eingriff: Exit 1 (`main.go does not import .../internal/configdrift`; der Wächter bricht an der ersten Stelle ab, das ist die Messung des Plans). Danach, mit nur `ConfigDriftRoutes` in `routeTable` (die Reihenfolge der Arbeit aus dem Plan): `TestEveryRouteThatReachesUpstreamHasABudgetRow` Exit 1 (`these routes have no row in the budget table`) und `TestEveryRouteIsReachableFromTheInterface` Exit 1 (`no file under web/src mentions these routes`), beide nennen die zwei Routen.

| Injektion | Test | Exit | scheiternde Zeile |
|---|---|---|---|
| (a) `inv.SetApplyObserver(...)` entfällt | `TestTheDaemonWiresTheDriftWatch` | 1 | `main.go calls SetApplyObserver 0 times, want exactly once: without it the watch does not see an apply of this daemon and reports every one as drift` |
| (b) `inv.SetConfigDrift(...)` entfällt | derselbe | 1 | `main.go calls SetConfigDrift 0 times, ...: without it the cluster card never says anything about configuration drift` |
| (c) `go drift.Run(...)` entfällt (`_ = driftCtx`) | derselbe | 1 | `main.go starts drift.Run in a goroutine 0 times, want exactly once: without it nothing is ever read` |
| (c2, zusätzlich) der Kontext ist `context.WithTimeout(context.Background(), time.Hour)` | derselbe | 1 | `the context driftCtx of drift.Run at main.go:838:2 is not made by context.WithCancel(context.Background()): the watch must live as long as the daemon and not as long as a deadline or a request` |
| (d) `Drift: drift` entfällt im Literal | derselbe | 1 | `the httpapi.Deps literal sets Drift 0 times, want once: a field set after the literal is nil in every handler, and the routes answer 502` |
| (e) `ForgetConfig: drift.Forget` entfällt | derselbe | 1 | `the provision.Deps literal sets ForgetConfig 0 times, want once: without it a machine that is reset and set up again is compared with the baseline of its old life` |
| (f) `handlers.ConfigDriftRoutes(deps)` entfällt in `routeTable` | derselbe | 1 | `routeTable does not call handlers.ConfigDriftRoutes: the routes of the drift watch would be answered with 404` |
| (g) der Eintrag `GET .../config/drift` in `noUpstream` entfällt | `TestEveryRouteThatReachesUpstreamHasABudgetRow` | 1 | `these routes have no row in the budget table and are not named as reaching nothing` (Route genannt) |
| (g2) der Eintrag `POST .../config/drift/confirm` entfällt | derselbe | 1 | dieselbe Zeile (Route genannt) |
| (h) die Gruppe in `interfaceInFlight()` entfällt (`web/src` nennt die Routen nicht) | `TestEveryRouteIsReachableFromTheInterface` | 1 | `no file under web/src mentions these routes:` (beide Routen genannt) |

Der Kontext des Daemons: `driftCtx` wird aus `context.Background()` abgeleitet, ohne Deadline und ohne Request; der Wächter akzeptiert nur diese Form (Variable, die von `context.WithCancel(context.Background())` zugewiesen wird). Das ist die Lehre von certwatch (quick task 261014-gl2).

Grün am Ende: `go test ./cmd/... ./internal/publicrepo/ ./internal/configdrift/... ./internal/planning/ ./internal/audit/... -count=1` alle ok; `go test ./internal/httpapi/...` grün bis auf die vier `web/dist`-Tests; `go build ./...`, `go vet ./cmd/... ./internal/httpapi/... ./internal/audit/...` und `gofmt -l cmd internal` sauber; Linter `0 issues.`

## Abweichungen vom Plan

**1. [Rule 3 - Wiederverwendung] `problemCode` gibt es schon** (`hostgatesapi_test.go`, Signatur `problemCode(raw)`); mein erster Entwurf einer eigenen Hilfe kollidierte und ist gestrichen.

**2. Der Test des 502 ohne Watch ist ein eigener Test** (`TestADriftRouteWithoutTheWatchSaysSo`), nicht Teil von `TestTheDriftRouteIsForOperatorsAndUp` wie im Plan; er prüft zusätzlich den Code `upstream.drift-unavailable`.

**3. Der Audit-Eintrag eines abgelehnten Requests.** Der Handler liest den Body mit `decodeJSON`, das unbekannte Felder verweigert (400). `TestTheBodyCannotNameTheNodeInTheAuditRecord` schickt deshalb einen Body mit `machine` und einem dritten Schlüssel und liest den **Versuchs**-Eintrag, der vor dem Handler geschrieben wird. Der Plan sagt „im Versuch und im Ergebnis die Maschine“: der Ergebnis-Eintrag trägt keine Parameter (nur Ausgang), geprüft ist dort nur, dass ein Erfolgs-Eintrag existiert (`TestTheConfirmationNamesTheNodeAndTheDigestInTheAuditRecord`: ein Versuch, ein Erfolg).

**4. Reihenfolge von Task 3.** Die Injektion (c) von Task 2 ist als „Parameter vor der Redaktion setzen“ nicht sinnvoll einzuspielen (siehe oben); ersetzt durch (c) und (c2).

**5. Zusätzliche Injektion (c2) in Task 3** (Kontext mit Deadline), weil der Auftrag die Lehre von certwatch ausdrücklich verlangt; und (g2) einzeln für den zweiten `noUpstream`-Eintrag.

**6. Wand-Beobachtung auf einem anderen Prüfstand** (siehe oben), weil der Konfigurations-Prüfstand die Wand nicht beantworten kann.

**7. Allowlist-Eintrag nach den etcd-Einträgen eingefügt,** damit gofmt die Ausrichtung der Nachbarn nicht ändert (kein Verhalten).

## Nicht gemessen und nicht bewiesen

- **Ein Lauf des Daemons selbst gegen einen echten Cluster.** Kein Prozess wurde gestartet; der AST-Wächter hält die Verdrahtung, aber was `main.go` zur Laufzeit tut (Reihenfolge von `inv.Start`, `--dry-run`, das erste Lesen nach einer Minute) ist nicht gesehen. Der Prüfstand baut den Dienst von Hand nach dem Vorbild des Daemons; dass die beiden übereinstimmen, hält nur der Wächter über die sechs Stücke.
- **Zweige der Wächter-Meldungen, die ich nicht rot gesehen habe:** das Stück 1 ohne `Read: ...ReadSnapshot` oder ohne `Baselines`; Stück 2 mit einem anderen Argument als `drift.ApplyObserver()`; Stück 3 mit einem anderen Argument als `drift.HealthFor`; Stück 5 mit `Drift` oder `ForgetConfig` auf einer anderen Variablen; Stück 4 mit `context.Background()` unmittelbar als Argument. Die Zweige sind geschrieben, aber nicht eingespielt; gemessen ist, was die Tabelle oben nennt (jedes Stück fehlt, und der Kontext mit Deadline).
- **Der Wächter erkennt die Variable des Kontexts nach Namen** (jede Zuweisung `driftCtx := context.WithCancel(context.Background())` irgendwo in der Datei genügt). Eine zweite Zuweisung gleichen Namens an anderer Stelle wäre ihm unsichtbar.
- **Die Wand ist eine Beobachtung** (siehe oben) und kein Wächter; ihre Antwort wurde nur für einen Cluster ohne Drift-Zustand gesehen.
- **`null`-Listen:** der Tracer prüft `changes` und `paths` und `items` auf `null` im Text; eine Injektion, die eine Liste `null` macht, wurde in diesem Plan nicht eingespielt (die Wächter dafür stehen in Plan 06 für `items`).
- **Der Leitfaden** (`docs/guide.md`) ist nicht geschrieben; der Vertrag verweist auf ihn für die Auskunft zu nicht redigierten Feldern und zu den Kosten einer Lesung (Plan 10). Kein README-, Changelog- oder Ledger-Eintrag in diesem Plan.
- **Die Oberfläche** (Plan 09) ist nicht berührt; die Gruppe in `interfaceInFlight()` ist so gebaut, dass sie ungültig wird, sobald `web/src` eine der Routen nennt, und von Plan 09 gelöscht wird.
- **`-race` auf dem Pi:** lief nur auf dem Mac; es gab keinen Pi.
- **A1 und A2 der Recherche** (was ein echter Knoten nach einem Apply zurückgibt): wie in den früheren Plänen nur gegen talossim gesehen.

## Auth gates

Keine.

## Known Stubs

Keine.

## Threat Flags

Keine über den Plan hinaus. T-15-27 (Repudiation): rot mit 2(c), 2(c2), 2(d), 2(e); T-15-28 (Elevation of Privilege): rot mit 2(a), 2(b); T-15-29 (Information Disclosure): rot mit 1(a), 1(b); T-15-30 (Tampering, Body bestimmt die Maschine): rot mit 2(c), 2(c2); T-15-31 (Verdrahtung): rot mit 3(a) bis 3(h) einschließlich (c2) und (g2); T-15-SC: nichts installiert.

## Self-Check: PASSED

Dateien vorhanden (`internal/httpapi/handlers/configdrift.go`, `internal/httpapi/configdriftapi_test.go`, `cmd/holzkube-managerd/drift_wiring_test.go`; geändert `router.go`, `problem.go`, `endtoend_test.go`, `middleware/audit.go`, `internal/audit/redact.go`, `main.go`, `budget_test.go`, `reachable_test.go`, `docs/api-contract.md`), Commits d4d68ed, 7b6c2c4 und 66da1a7 sind Vorfahren von HEAD.
