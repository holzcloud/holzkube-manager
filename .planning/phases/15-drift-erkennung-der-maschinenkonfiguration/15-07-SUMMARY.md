---
phase: 15-drift-erkennung-der-maschinenkonfiguration
plan: 07
subsystem: inventory, configdrift, provision
tags: [drift, apply-observer, d-10, callers, provisioning, source-guard]
requires: [15-05, 15-06]
provides:
  - "internal/inventory: (*Service).SetApplyObserver und der Einbau des Beobachters in Connect, der eine Weg zu einem Client"
  - "internal/provision: Deps.ForgetConfig, gerufen im Schritt 'apply the configuration' nach der zweiten Prüfung der Maschine und vor dem Schreiben"
  - "internal/configdrift: Quelltext-Wächter TestOnlyKnownCallersSendAConfigurationToAManagedNode (Aufrufer mit Anzahl)"
  - "Ende-zu-Ende-Tests: config.apply (Tracer), Kubernetes-Upgrade, CA-Rotation, je als eigener Apply"
affects: [15-08, 15-09, 15-10]
actuals:
  tokens: 8250
  tasks: 3
  commits: 3
plan_head_before: 4d7ac95034e4961f9df380b8864975614718d591
plan_head_after: a52c4db7b0d53092cc7ed7182fab3f7f3c6815b4
tech-stack:
  added: []
  patterns: ["ein Beobachter am einzigen Weg zum Client statt an den Aufrufern", "Quelltext-Wächter mit Anzahl je Datei", "Haken zwischen Prüfung und Schreiben, mit der Aufzeichnung des Zählers im Augenblick des Aufrufs"]
key-files:
  created:
    - internal/inventory/applyobserver_test.go
    - internal/configdrift/callers_test.go
    - internal/configdrift/callers_guard_test.go
    - internal/upgrade/drift_test.go
    - internal/rotateca/drift_test.go
    - internal/provision/forget_test.go
  modified:
    - internal/inventory/inventory.go
    - internal/inventory/mutate.go
    - internal/provision/job.go
key-decisions:
  - "Der Beobachter wird in Connect angehängt, nachdem NewClusterClient den Client geliefert hat, mit dem Beobachter, den der Dienst unter s.mu liest. Ein Client, der schon herausgegeben ist, behält den, den er bekam; SetApplyObserver(nil) wirkt nur auf neue Clients"
  - "Der Wächter zählt die Aufrufstellen je Datei (jeweils 1 in machineconfig/service.go, upgrade/job.go, rotateca/job.go, provision/job.go), nicht nur die Dateien: ein zweiter Aufruf in einer bekannten Datei ist eine neue Stelle"
  - "Annahme A9 der Recherche geprüft: der Haken sitzt nicht im Record-Schritt am Ende, sondern im Schritt 'apply the configuration', zwischen zweiter Prüfung und Schreiben"
requirements-completed: []
status: complete
---

# Phase 15 Plan 07: Die Aufrufer Summary

`inventory.Connect` hängt den Beobachter des Drift-Dienstes an jeden Client, den es herausgibt; die Route config.apply, ein ganzer Kubernetes-Upgrade-Job und eine ganze CA-Rotation sind Ende zu Ende als eigene Applies gemessen (Zustand `ok`, Quelle `apply`, nie Drift), ein vierter Sender einer Konfiguration fällt im Quelltext-Wächter auf, und der Provisionierungs-Job vergisst die alte Basis einer Maschine unmittelbar vor dem Schreiben.

BASE: 4d7ac95034e4961f9df380b8864975614718d591

`plan_head_before` ist derselbe Wert. `git rev-list --count BASE..HEAD` vor diesem SUMMARY-Commit: 3 (58a8701, 494a5a5, a52c4db).

## Wo es lief

Mac (darwin/arm64), `go1.27.2`, kein echter Cluster, kein Pi, kein `task`. `go test -race ./internal/inventory/ ./internal/configdrift/ ./internal/talos/ -count=1` lief hier grün, ebenso `-race` über `./internal/upgrade/ ./internal/rotateca/ ./internal/provision/` (nur Mac; auf dem Pi nicht lauffähig, dort bleibt CI maßgeblich). CI-Linter `GOTOOLCHAIN=go1.26.9 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1 run ./...`: `0 issues.` Nichts gepusht, nichts getaggt, kein Changelog, nichts installiert. Die vier `internal/httpapi`-Tests, die `web/dist` brauchen (`TestCSPHashMatchesTheServedShell`, `TestForeignHostIsRefused`, `TestNoAssetIsInlinedPastTheContentSecurityPolicy`, `TestFontsAreServed...`), scheitern in `go test ./internal/httpapi/` erwartungsgemäß und sind nicht Gegenstand; alles übrige in `httpapi` ist grün.

## Was gebaut wurde

- **Task 1, Tracer (58a8701).** `inventory.go`: Feld `applyObserver` unter `s.mu`. `mutate.go`: `SetApplyObserver` nach dem Muster der anderen Setter; `Connect` hängt den Beobachter an den Client, nachdem `talos.NewClusterClient` ihn geliefert hat und bevor er zurückgegeben wird. Kein Byte an `machineconfig`, `upgrade`, `rotateca` geändert. `TestConnectInstallsTheApplyObserverOnEveryClient` (Maschine, Cluster, Modus und Bytes, zweiter Client, nach `SetApplyObserver(nil)` nichts mehr) und `TestTracerAnApplyThroughTheInventoryIsAnOwnApply` (echtes Inventar, `machineconfig.Service{Connect: inv.Connect}.Apply`, `inv.SetApplyObserver(svc.ApplyObserver())` als einzige Stelle, an der der Test den Beobachter bekannt macht; Gegenprobe: der Digest der Lesung nach dem Apply ist ein anderer als der der Basis; danach `ok`, Quelle `apply`, kein `Expected`, kein Diff; ein Dritter, nachdem der Beobachter abgenommen ist, ist `drift` mit Beginn).
- **Task 2 (494a5a5).** `TestAKubernetesUpgradeIsAnOwnApplyForTheDriftWatch` (aufgebaut wie der bestehende Job-Test, Job nach `v1.35.2`) und `TestACARotationIsAnOwnApplyForTheDriftWatch` (`newRotationFixture`, vier Durchgänge, Lesung mit den Zugangsdaten, die das Inventar nach der Rotation hält): jeweils erste Sicht, Job, Vorbedingung (Konfiguration des Knotens weicht von der Basis ab; bei der Rotation zusätzlich: der Knoten stellt nicht mehr von der alten Autorität aus), `Flush`, dann Zustand `ok`, Quelle `apply`, Digest der Basis gleich dem der Lesung, und ein weiterer `Tick` ändert nichts. `TestOnlyKnownCallersSendAConfigurationToAManagedNode`: parst die Produktionsquellen unter `internal` und `cmd` (ohne `internal/talos`, ohne `internal/talossim`), findet jeden Selektor `ApplyConfigurationWithMode` oder `ApplyConfiguration`, ruft oder als Wert genommen, und vergleicht mit vier bekannten Stellen samt Anzahl und Begründung; er zählt die gescannten Dateien (Mindestzahl 15, gemessen 231) und scheitert darunter.
- **Task 3 (a52c4db).** `provision.Deps.ForgetConfig` (optional, nil heißt nichts zu vergessen) und sein Aufruf im Schritt „apply the configuration“ nach dem zweiten `VerifyMachine` und vor `talos.NewMaintenanceClient`; Kommentar an der Stelle. Schrittliste und Namen unverändert. Vier Tests in einem internen Test (`steps` ist unexportiert): `TestTheApplyStepForgetsTheOldBaselineBeforeItWrites` (UUID gleich, Zähler des Simulators im Augenblick des Aufrufs 0, danach genau ein Apply), `TestAWrongMachineForgetsNothing`, `TestWithoutAForgetHookTheApplyStepIsUnchanged`, dazu `TestAFailedWriteStillForgetsTheOldBaseline` (Szenario `reject_apply`).
- **A9 der Recherche (Signatur nicht geprüft) ist hier geprüft:** der Haken sitzt nicht im `Record`-Schritt am Ende des Jobs. Dort ließe er zwischen dem Neustart des Knotens und dem Ende des Jobs ein Fenster, in dem ein Lesen die neue Konfiguration gegen die alte Basis vergliche; im Schritt „apply the configuration“ steht fest, dass es diese Maschine ist, und eine gescheiterte Prüfung hat nichts vergessen.
- Gegen BASE unberührt: `git diff --name-only 4d7ac95 -- internal/machineconfig internal/certwatch internal/changelog internal/jobs internal/talos internal/httpapi web cmd internal/upgrade/job.go internal/rotateca/job.go` ist leer (gemessen, 0 Zeilen).

## Rot gesehen

Exitcodes sind die des `go test`-Befehls selbst. Jede Injektion: genau eine Änderung am Produktionscode (das Skript bricht ab, wenn die Ersetzung nicht genau einmal trifft), danach `go build ./...` und `go vet` mit Exit 0 **vor** dem Testlauf, Wiederherstellung per `cp` und `cmp -s` (Exit 0, in jedem Lauf ausgegeben), danach grün.

### Task 1

RED gegen den Stand vor dem Eingriff: Exit 1, Compile-Fehler (`SetApplyObserver` undefiniert), beide Pakete. Ein Compile-Fehler ist hier die Messung, die der Plan verlangt.

| Injektion | Test | Exit | scheiternde Zeile |
|---|---|---|---|
| (a) `Connect` hängt den Beobachter nicht an (`_ = observer`) | `TestConnectInstallsTheApplyObserverOnEveryClient` | 1 | `the observer heard 0 attempts after the first apply, want 1` |
| (a) | `TestTracerAnApplyThroughTheInventoryIsAnOwnApply` | 1 | `state after an own apply through the inventory is "baseline-created" (...), want ok` |
| (b) nur der erste Client trägt ihn (`s.applyObserver = nil` nach dem Lesen) | `TestConnectInstallsTheApplyObserverOnEveryClient` | 1 | `the observer heard 1 attempts after a second client applied, want 2: only the first client Connect gave out carried it` |
| (b) | der Tracer ebenfalls | 1 | `state after an own apply ... "baseline-created" ... want ok` (die Lesungen des ersten `Tick` verbrauchen den ersten Client) |
| (c, zusätzlich) `SetApplyObserver(nil)` ist ein Nichts | `TestConnectInstallsTheApplyObserverOnEveryClient` | 1 | `the observer heard 3 attempts after it was taken away, want it to stay at 2` |
| (c) | der Tracer | 1 | `state after a third party's change is "ok" (...), want drift: an own apply swallowed it` |

(b) wurde nach einem ersten Versuch wiederholt, der ein nicht definiertes Flag benutzte und den Build brach: das war keine Messung und ist nicht mitgezählt.

Zu (a): der Tracer wird hier über den fehlenden Anstoß rot (ohne Beobachter kein `Poke`, der Zustand bleibt `baseline-created`), nicht über eine gemeldete Drift. Der Fall „Anstoß da, Erwartung fehlt“ ist Sache der Tests von Plan 05; hier wird gemessen, dass der Beobachter überhaupt ankommt.

### Task 2

RED-Lage: die zwei Job-Tests waren gegen den Stand nach Task 1 sofort grün (so im Plan vorgesehen: Rot kommt aus den Injektionen). Der Wächter lief beim ersten Mal rot, aber aus einem Befund, nicht aus einer eingefügten Stelle (siehe Abweichung 1).

| Injektion | Test | Exit | scheiternde Zeile |
|---|---|---|---|
| (a) `ApplyConfigurationWithMode` ruft `observeApply` nicht (`internal/talos/client.go`) | `TestTracerAnApplyThroughTheInventoryIsAnOwnApply` | 1 | `state after an own apply through the inventory is "baseline-created" ..., want ok` |
| (a) | `TestAKubernetesUpgradeIsAnOwnApplyForTheDriftWatch` | 1 | `after the flush: the state after a Kubernetes upgrade is "baseline-created" ..., want ok` |
| (a) | `TestACARotationIsAnOwnApplyForTheDriftWatch` | 1 | `after the flush: the state after a CA rotation is "baseline-created" ..., want ok` |
| (a) | dazu `TestConnectInstallsTheApplyObserverOnEveryClient` | 1 | `the observer heard 0 attempts after the first apply, want 1` |
| (b) `Connect` hängt nicht an (`internal/inventory/mutate.go`) | dieselben drei, einzeln | 1 | dieselben drei Zeilen wie in (a) |
| (c) neue Datei `internal/jobs/zz_unobserved.go` mit einem `ApplyConfigurationWithMode`-Aufruf (Build und vet Exit 0) | `TestOnlyKnownCallersSendAConfigurationToAManagedNode` | 1 | `internal/jobs/zz_unobserved.go: sends a configuration to a node (1 place(s)) and is not a known caller` |
| (c2, zusätzlich) ein zweiter Aufruf in der bekannten `internal/rotateca/job.go` | derselbe | 1 | `the callers that send a configuration to a node are not the known ones:` (Datei mit 2 Stellen gegen bekannte 1) |

Die drei einzeln roten Testnamen unter (a) und unter (b) sind die geforderten. `client.go`, `mutate.go` und `rotateca/job.go` sind per `cmp` wiederhergestellt (Exit 0); `internal/jobs/zz_unobserved.go` ist gelöscht (`test ! -e` bestätigt, `git status --short` ohne `jobs`).

**Nicht gemessen:** eine Injektion (c3), die eine bekannte Stelle verschwinden lässt, damit der Zweig „als Aufrufer gelistet, sendet nichts mehr“ des Wächters rot wird. Der Versuch (den Aufruf in `upgrade/job.go` durch einen Ausdruck ersetzen) brach den Build (`declared and not used`) und ist damit eine Nicht-Messung; ich habe sie nicht wiederholt. Der Zweig ist geschrieben, aber nicht rot gesehen.

### Task 3

RED gegen den Stand vor dem Eingriff: Exit 1, Compile-Fehler (`f.deps.ForgetConfig undefined`). Danach das Feld allein ohne Aufruf (Exit 1, Verhalten statt nur Compile): `TestTheApplyStepForgetsTheOldBaselineBeforeItWrites` (`ForgetConfig was called 0 times, want exactly once`) und `TestAFailedWriteStillForgetsTheOldBaseline` rot; die zwei anderen grün, wie sie sollen.

| Injektion | Test | Exit | scheiternde Zeile |
|---|---|---|---|
| (a) Aufruf hinter `mc.ApplyConfiguration` | `TestTheApplyStepForgetsTheOldBaselineBeforeItWrites` | 1 | `the machine had heard 1 configurations when ForgetConfig was called, want 0: the old baseline must go before the write and not after` |
| (a) | `TestAFailedWriteStillForgetsTheOldBaseline` | 1 | `ForgetConfig was called for [], want once for ...` |
| (b) Aufruf vor `buildConfig` und dem ersten Check der Maschine des Schritts | `TestAWrongMachineForgetsNothing` | 1 | `ForgetConfig was called for [00000000-0000-4000-8000-000000000000] although the machine was not the one asked for` |
| (c) Aufruf entfällt | `TestTheApplyStepForgetsTheOldBaselineBeforeItWrites` | 1 | `ForgetConfig was called 0 times, want exactly once` |
| (c) | `TestAFailedWriteStillForgetsTheOldBaseline` | 1 | `ForgetConfig was called for [], want once for ...` |
| (d, zusätzlich) falsche Maschine im Aufruf (`""`) | `TestTheApplyStepForgetsTheOldBaselineBeforeItWrites` | 1 | `ForgetConfig was called for "", want the machine "..."` |
| (e, zusätzlich) Aufruf nur im Erfolgsweg (nach der Detail-Zeile) | `TestAFailedWriteStillForgetsTheOldBaseline` | 1 | `ForgetConfig was called for [], want once for ...` |

Zu (b): der Plan nennt „vor dem ersten `VerifyMachine`“. Der Schritt „apply the configuration“ hat zwei Stellen, an denen die Maschine bestätigt wird; der erste Schritt „confirm this is the right machine“ ist ein eigener. Der Haken steht in der Injektion vor `buildConfig` und damit vor der Prüfung dieses Schritts. (Der Test, der das auch dort rot sieht, ist der der falschen Maschine.)

Grün am Ende: `go test ./internal/inventory/ ./internal/configdrift/ ./internal/upgrade/ ./internal/rotateca/ ./internal/provision/ ./internal/talos/ ./internal/planning/ -count=1` (alle ok), `-race` über die ersten drei (Mac), `go test ./internal/publicrepo/` ok, `go build ./...`, `go vet` und `gofmt -l` über die fünf Pakete sauber, Linter `0 issues.`

## Abweichungen vom Plan

**1. [Rule 1 - Befund] Der Wächter fand die Simulator-Seite.** Beim ersten Lauf meldete er `internal/talossim/machine.go` als unbekannten Aufrufer: dort steht `machine.ApplyConfiguration{...}` (der Nachrichtentyp der Antwort) und die Server-Methode gleichen Namens. Der Simulator ist der Empfänger eines Applys und erreicht keinen verwalteten Knoten. `internal/talossim` ist deshalb wie `internal/talos` aus dem Scan genommen, mit einem Satz Begründung im Test. Der Plan sagt „außerhalb von `internal/talos`“; die Ausnahme ist eine Ergänzung.

**2. Der Wächter zählt Stellen je Datei,** nicht nur Dateien (Plan: „kennt genau die Aufrufer“). Das ist strenger, und (c2) misst es.

**3. Die Test-Regex des Plans für die Provisionierung in `internal/httpapi` (`-run 'Provision'`) trifft nichts** (`[no tests to run]`): die Tests heißen `TestTheWizard...`, `TestAPlanNaming...`, `TestTheEvenControlPlane...`, `TestTheApplyIsGated...`, `TestTheBootstrapRecovery...` in `provisionapi_test.go`. Diese fünf liefen und sind grün; das gesamte Paket läuft grün bis auf die vier `web/dist`-Tests.

**4. Zusätzliche Tests.** `TestAFailedWriteStillForgetsTheOldBaseline` (das Verhalten des Plans „ein Fehler beim Schreiben ändert nichts“ stand unter `<behavior>`, aber nicht unter den Tests des Plans) und die Injektionen (c) in Task 1, (c2) in Task 2, (d) und (e) in Task 3.

**5. Der Test der Provisionierung ist ein interner Test (`package provision`) neben den externen** (`provision_test`) der übrigen Dateien im Verzeichnis, wie der Plan es verlangt. `storedSecrets` ist aus `machineconfig/diff_test.go` abgeschrieben, nicht importiert. Die Maintenance-Simulator-Aufbauten brauchten keine Sonderbehandlung: der Simulator ohne `Cluster` zählt den Apply, und `buildConfig` erzeugt die Konfiguration aus den Geheimnissen eines `talossim.NewCluster`. Der Checkpoint für „Aufbau nicht machbar“ trat nicht ein.

## NICHT bewiesen

- Ein echtes Zurücksetzen und Neuaufsetzen einer Maschine unter derselben UUID: nur der Maintenance-Simulator, der den Apply zählt. Dass `Service.Forget` im Daemon die Basis wirklich entfernt und `ForgetConfig` dort besetzt ist, ist Verdrahtung von Plan 08; hier ist nur der Haken im Job gemessen.
- Die Wirkung von A2 (was ein echter Knoten nach einem `no-reboot`-Apply zurückgibt, wann die Lesung die neue Konfiguration zeigt) auf echten Knoten: der Simulator übernimmt die gesendete Konfiguration sofort als aktive.
- `-race` auf dem Pi: lief nur auf dem Mac; es gab keinen Pi.
- Dass kein Aufrufer außerhalb von `internal` und `cmd` eine Konfiguration sendet: der Wächter scannt diese zwei Wurzeln und erkennt Selektoren nach Namen; ein Sender über Reflexion oder einen anders benannten Wrapper in `internal/talos` selbst (ausgenommen) wäre ihm unsichtbar. Letzteres deckt der Wächter der Methodenmenge aus Plan 05 ab.
- Der Zweig des Wächters „gelistet, sendet nichts mehr“ (siehe oben, c3).
- Daemon-Verdrahtung (`SetApplyObserver` mit `svc.ApplyObserver()`, `ForgetConfig` mit `Service.Forget`) ist Plan 08; bis dahin hängt im Daemon kein Beobachter.

## Auth gates

Keine.

## Known Stubs

Keine.

## Threat Flags

Keine. Der Beobachter bekommt Rohbytes nur im Speicher (T-15-26, Plan 05); in diesem Plan wird er nur angehängt.

## Self-Check: PASSED

Alle neun Dateien des Plans vorhanden (`internal/inventory/{applyobserver_test.go,inventory.go,mutate.go}`, `internal/configdrift/{callers_test.go,callers_guard_test.go}`, `internal/upgrade/drift_test.go`, `internal/rotateca/drift_test.go`, `internal/provision/{job.go,forget_test.go}`), Commits 58a8701, 494a5a5 und a52c4db sind Vorfahren von HEAD, `internal/jobs/zz_unobserved.go` existiert nicht.
