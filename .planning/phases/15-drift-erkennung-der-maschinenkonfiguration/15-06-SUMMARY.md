---
phase: 15-drift-erkennung-der-maschinenkonfiguration
plan: 06
subsystem: configdrift, inventory, metrics, model
tags: [drift, cluster-view, metrics, banner, d-05, d-06, never-nil-lists]
requires: [15-03, 15-04]
provides:
  - "internal/model: ConfigDriftHealth, NodeConfigDrift (Read, Drifting, Judged), die ConfigState*-Konstanten"
  - "internal/configdrift: (*Service).HealthFor, BannerAfter, bannerSentence; der Pass hält seine Mitglieder (members) statt Zahlen"
  - "internal/inventory: ClusterView.ConfigDrift, SetConfigDrift, configDriftOf in Clusters und Cluster"
  - "internal/metrics: holzkube_node_config_drift, holzkube_cluster_config_drift, holzkube_cluster_config_nodes_judged, holzkube_node_config_read_age_seconds"
  - "Testaufbau newAdoptedNode (echtes Inventar über talossim), newTwoNodes, driftOf, withDrift für die Pläne 07 und 08"
affects: [15-07, 15-08, 15-09, 15-10]
actuals:
  tokens: 14650
  tasks: 3
  commits: 3
plan_head_before: d764d2361fd9ca8d954728a6bcd918c5df833d5e
plan_head_after: f205cba4f0e428f260169046928701884978cde6
tech-stack:
  added: []
  patterns: ["abgeleitete Sicht, nie gespeichert, Setter am Inventar wie bei den Zertifikaten", "Zahlen der Sicht aus den Lesungen, nicht aus Zahlen, die der Pass hielt", "Banner vom Server aus dem Beginn im Datensatz, now als Parameter"]
key-files:
  created:
    - internal/model/configdrifthealth.go
    - internal/configdrift/health.go
    - internal/configdrift/health_test.go
    - internal/configdrift/view_test.go
    - internal/inventory/configdrift_test.go
    - internal/metrics/configdrift_test.go
  modified:
    - internal/configdrift/configdrift.go
    - internal/inventory/inventory.go
    - internal/inventory/view.go
    - internal/metrics/metrics.go
    - internal/metrics/metrics_test.go
key-decisions:
  - "Entscheidung zu 15-04 und 15-05: `passes` hielt Zahlen je Cluster, nicht je Knoten, und war nach einem Anstoß falsch. Es genügt nicht; der Pass hält jetzt seine Mitglieder (Maschine, Hostname), und HealthFor leitet Items und Zählungen aus den Lesungen ab. Ein Anstoß zeigt sich damit sofort an der Karte"
  - "Die Sätze je Knoten in der Clusterliste sind für unreadable und baseline-damaged feste Sätze ohne Fehlertext: die Liste ist Reader-lesbar, der Fehlertext einer gescheiterten Lesung kann eine Adresse oder einen Pfad des Datenverzeichnisses nennen"
  - "Banner = der älteste bekannte Beginn ist mindestens BannerAfter her; die Warnung nennt diesen Knoten und zählt die übrigen abweichenden"
requirements-completed: []
status: complete
---

# Phase 15 Plan 06: Karte, Metrik und Banner Summary

Die Clusterkarte trägt jetzt `config_drift` (Zahlen, Zustand, Beginn und letzte gute Lesung je Knoten, nie ein Diff-Inhalt), `/metrics` trägt vier Serien, die nur für einen verglichenen Knoten 0 sagen, und das Banner nach 24 Stunden unbestätigter Drift wird auf dem Server aus dem Beginn im Datensatz gerechnet; alles abgeleitet, nichts gespeichert, kein Knoten beim Scrape gefragt.

BASE: d764d2361fd9ca8d954728a6bcd918c5df833d5e

`plan_head_before` ist derselbe Wert. `git rev-list --count BASE..HEAD` vor diesem SUMMARY-Commit: 3 (9ab5c7a, baa6d7e, f205cba).

## Wo es lief

Mac (darwin/arm64), `go1.27.2`, kein echter Cluster, kein Pi, kein `task`. `go test -race ./internal/configdrift/ ./internal/inventory/ ./internal/metrics/ -count=1` lief hier grün (nur Mac; auf dem Pi nicht lauffähig, CI bleibt maßgeblich). Der CI-Linter `GOTOOLCHAIN=go1.26.9 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1 run ./...` meldet `0 issues.` Nichts gepusht, nichts getaggt, kein Changelog, nichts installiert. Die vier `internal/httpapi`-Tests, die `web/dist` brauchen, sind nicht meine und wurden nicht angefasst.

## Was gebaut wurde

- **Task 1, Tracer (9ab5c7a).** `model/configdrifthealth.go`: die Wire-Form wie unter interfaces im Plan, dazu die sieben `ConfigState*`-Strings (ein Test hält sie an die `State`-Konstanten von `configdrift`) und die drei Prädikate `Read`, `Drifting`, `Judged`, die HealthFor und Metrik teilen, damit „hat ein Urteil“ an genau einer Stelle steht. `configdrift/health.go`: `HealthFor` (nil bis zum ersten Pass, Items nie nil, nach Hostname sortiert, `CheckedAt` ist der Pass oder die jüngste Lesung). `inventory`: Feld, `ClusterView.ConfigDrift`, `SetConfigDrift`, `configDriftOf`, Aufruf in `Clusters` und `Cluster`, genau nach dem Muster der Zertifikate.
- **Task 2, Metrik (baa6d7e).** `metrics.writeConfigDrift`: vier Familien mit HELP und TYPE auch ohne Serie. Knoten-Drift 1 bei gesetztem Beginn, 0 bei Zustand `ok`, `baseline-created`, `settling`, sonst keine Serie; Cluster 1 bei irgendeiner Drift, 0 nur bei `Nodes > 0` und Urteil auf jedem Knoten, sonst keine Serie; `nodes_judged` für jeden gelesenen Cluster auch mit 0; Lesealter `max(0, now - ReadAt)`, nicht für einen nie gelesenen Knoten. Label `node` ist der Hostname. Der Paketkommentar nennt die Ausnahme zur Regel „kein Knoten ist je Label-Wert“; der Kommentar von `TestCardinalityDoesNotGrowWithTheFleet` ist angepasst (nur der Kommentar).
- **Task 3, Banner (f205cba).** `BannerAfter = 24h`; `Banner` ist wahr, wenn der älteste bekannte Beginn mindestens so alt ist; `Warning` nennt Hostname, Clustername und den Beginn (UTC) des ältesten und zählt die übrigen abweichenden Knoten, ohne Pfad, Wert oder Digest.
- Gegen BASE unberührt: `git diff --name-only d764d23... -- internal/certwatch internal/httpapi internal/changelog web cmd` ist leer (gemessen, Ausgabe `[]`). Keine Wand-Datei.

## Rot gesehen

Exitcodes sind die des `go test`-Befehls selbst. Jede Injektion: genau eine Änderung am Produktionscode (das Skript bricht ab, wenn der Treffer nicht genau einer ist), danach `go build ./...` und `go vet` mit Exit 0 **vor** dem Testlauf (sonst „NON-MEASUREMENT“), Wiederherstellung per `cp` und `cmp` (identisch, in jedem Lauf ausgegeben), danach grün.

### Task 1

RED gegen den Stand vor dem Eingriff: Exit 1, Compile-Fehler (`SetConfigDrift`, `HealthFor`, `ConfigDrift` undefiniert). GRÜN danach.

| Injektion | Test | Exit | scheiternde Zeile |
|---|---|---|---|
| (a) `HealthFor` liefert vor dem ersten Pass ein leeres, nicht-nil Ergebnis | `TestTracerAReadingReachesTheClusterView` | 1 | `before the first pass ConfigDrift is &{... Nodes:0 ... Items:[]}, want nil ("not read yet")` |
| (b) `Drifted` bleibt 0 (`h.Drifted += 0`) | derselbe | 1 | `after a third party's change ConfigDrift is &{... Drifted:0 ... State:drift ...}, want one drifted node` |
| (c) `Cluster` setzt die Sicht nicht | derselbe | 1 | `Cluster: after the first pass ConfigDrift is nil` |
| (d) leerer Pass setzt `Items` auf nil (`var items`) | `TestAnEmptyPassSerialisesItsItemsAsAList` | 1 | `an empty pass serialises its items as null` |

Zusätzliche Tests (siehe Abweichung 1) mit ihren Injektionen:

| Injektion | Test | Exit | scheiternde Zeile |
|---|---|---|---|
| (e) Sicht bleibt auf dem Stand des Passes (Lesung jünger als der Pass wird ignoriert) | `TestTheCountsFollowAPokedReadingBeforeThePassComes` | 1 | `after a poked reading the view is &{... Nodes:1 ReadNodes:0 Drifted:0 ... State:not-read ...}, want 1 node, 1 read, 1 drifted` |
| (f) Satz je Knoten ist der rohe Satz (Fehlertext) | `TestAnUnreadableNodeCarriesNoErrorTextInTheList` | 1 | `the sentence in the cluster list carries "192.168.1.10"` (und `/var/lib`, `connection refused`) |
| (g) Konstante in `model` weicht ab (`"settles"`) | `TestTheStatesOfTheViewAreTheStatesOfThePackage` | 1 | `state "settling": configdrift says "settling", model says "settles"` |
| (h1) Satz je Knoten trägt einen Pfad (`" KubeletConfig.image changed"` angehängt) | `TestTracerAReadingReachesTheClusterView` | 1 | `the cluster JSON carries "KubeletConfig"; the list is readable by a Reader and the difference is configuration content` |

### Task 2

RED gegen den Stand nach Task 1: Exit 1, zehn Metrik-Tests und `TestTracerAReadingReachesTheMetrics` scheitern (nach einer Korrektur des Tests selbst: ein erster Lauf stürzte wegen `build(2)` statt `build(1, 1)` mit einem Index-Panic ab und zählte nicht als Messung).

| Injektion | Test | Exit | scheiternde Zeile |
|---|---|---|---|
| (a) Serie für jedes Item, auch `not-read` | `TestANodeNeverReadHasNoDriftSeries` | 1 | `node never has a drift series; a node nobody could judge is not "no drift"` (auch `silent`, `damaged`) |
| (b) `unreadable` mit Beginn schreibt keine Serie | `TestAnUnreadableNodeWithAKnownDriftIsStillOne` | 1 | `an unreadable node with a known start = 0 (present false), want 1` |
| (c) Cluster 0 schon bei „mindestens ein Knoten“ (`judged > 0`) | `TestAClusterIsZeroOnlyWhenEveryNodeHasAVerdict`, `TestAClusterWithPartialCoverageIsOneOrAbsent` | 1 | `partial coverage without drift writes a series` und `one node judged, one never read: the cluster says it has no drift` (vier Fälle) |
| (c2) `nodes_judged` zählt `not-read` mit | `TestNodesJudgedCountsOnlyVerdicts` | 1 | `nodes judged = 6 (present true), want 5` und `... of a cluster with no verdict = 1 ... want 0` |
| (d) Alter auch ohne `ReadAt` | `TestReadAgeIsSecondsAndAbsentForANeverReadNode` | 1 | `a node never read has a read age; there is nothing to be old` |
| (e) Label `node` ist die Maschinen-ID | `TestConfigDriftLabelsAreNamesAndNeverMachineIDs` | 1 | `precondition: the series with the host name is not there` (siehe unten) |
| (e2) zusätzlich ein Label `machine` am Lesealter | derselbe | 1 | `machine id 4c4c4544-... appears in the export` |
| (f) `nodes_judged` 0 auch für einen Cluster ohne Sicht | `TestAClusterNobodyHasReadWritesNoConfigSeries` | 1 | `holzkube_cluster_config_nodes_judged has a series for a cluster nobody has read` |
| (g) Lesealter-Serie fehlt ganz | `TestPerNodeConfigSeriesGrow...`, `TestReadAgeIs...` | 1 | `three more nodes in a read cluster add 3 series, want 6` und `read age = 0 (present false), want 90` |
| (h) Alter ohne `max(0, ...)` | `TestReadAgeIsSecondsAndAbsentForANeverReadNode` | 1 | `read age in the future = -60 (present true), want 0, never negative` |

(e) schlug über die Vorbedingung (Serie mit Hostname fehlt) fehl, nicht über die ID-Zeile selbst; deshalb (e2), das die ID-Zeile allein zum Scheitern bringt. Die Bestandswächter `TestEveryMetricCarriesHelpAndType`, `TestNoMachineEverBecomesALabelValue`, `TestCardinalityDoesNotGrowWithTheFleet` blieben durchgehend grün.

### Task 3

RED gegen den Stand nach Task 2: Exit 1 (Compile-Fehler `configdrift.BannerAfter` undefiniert). Dann, um ein Verhalten statt nur den Compile-Fehler zu messen, die Konstante allein gesetzt (kein Banner): Exit 1, sieben `--- FAIL` (Banner-Warten, Zählung ab Beginn, Neustart, unlesbar, Bestätigen, ältester Knoten, Satz). Die Inventar-Tests waren zu diesem Zeitpunkt grün, weil der Setter aus Task 1 stammt; ihre Messung ist (e).

| Injektion | Test | Exit | scheiternde Zeile |
|---|---|---|---|
| (a) Schwelle 0 | `TestTheBannerWaitsTwentyFourHours` | 1 | `a drift 0s old: Banner is true, want false` |
| (b) Schwelle zählt ab `ReadAt` statt ab `Since` | `TestTheBannerCountsFromTheStartOfTheDriftNotTheLastReading` | 1 | `24 hours after the start of the drift there is no banner, though the last reading is younger` |
| (c) `unreadable` zählt nicht zum Banner | `TestAnUnreadableNodeWithADriftOlderThanADayStillBringsTheBanner` | 1 | `a node that stopped answering lost the banner it had earned` und die Variante nach Neustart |
| (d) `markConfirmed` lässt den Beginn stehen (`lifecycle.go`) | `TestConfirmingTakesTheBannerAway` | 1 | `after confirming the banner is true with "..."` und `... State:ok Since:2026-10-14 09:00:00 ... want ok, no start, none` |
| (e) `configDriftOf` übergibt den Cluster nicht (`model.Cluster{}`) | `TestTheClusterViewCarriesWhatTheDriftFunctionGives` | 1 | `Clusters: the cluster the function answered for has <nil>` |
| (f) `ReadNodes` zählt `unreadable` als gelesen (`model.Read`) | `TestAnUnreadableNodeIsNotCountedAsRead` | 1 | `a node that did not answer is 1 of 1 read, want 0 of 1` |
| (g) ältester ist der jüngste (`After` statt `Before`) | `TestTheBannerNamesTheOldestNodeAndCountsTheRest` | 1 | `the oldest drift is 24 hours old and there is no banner` |
| (g2) Satz aus `items[0]` statt dem ältesten | derselbe | 1 | `the warning "The configuration of cp-1 ..." does not name "cp-2"` |
| (g3) die Zahl der übrigen zählt den genannten mit | derselbe | 1 | `... "2 other nodes differ as well." does not name "1 other node"` |
| (i) Grenze `>` statt `>=` | `TestTheBannerWaitsTwentyFourHours` | 1 | `a drift 24h0m0s old: Banner is false, want true` |
| (h2) Banner-Satz trägt einen Pfad (`(KubeletConfig.image)` eingefügt) | `TestTheBannerSentenceNamesNoConfigurationContent` | 1 | `the banner sentence "... last knew (KubeletConfig.image). ..." carries "KubeletConfig"` (und `"image"`) |

Der Plantext zu (d) sagt „`Banner` aus allen Items mit Zustand `drift` unabhängig von der Zeit, nachdem `Confirm` gelaufen ist (der Zustand bleibt im Speicher stehen)“. Das lässt sich als eine Änderung am Produktionscode nicht so einspielen, dass der Test es misst: nach dem Bestätigen steht der Zustand auf `ok`. Gemessen ist stattdessen der echte Fehler, der das Banner nach dem Bestätigen am Leben hielte: der Beginn bleibt im Speicher stehen.

Grün am Ende: `go test ./internal/configdrift/ ./internal/inventory/ ./internal/metrics/ ./internal/model/ ./internal/planning/ -count=1` (alle ok), `go test -race` über die ersten drei (Mac), `go test ./internal/publicrepo/` ok, `go build ./...`, `go vet` und `gofmt -l` über die vier Pakete sauber, Linter `0 issues.`

## Abweichungen vom Plan

**1. [Rule 2 - fehlende Korrektheit] Pass mit Mitgliedern, Zählungen aus den Lesungen.** 15-04 und 15-05 haben die Entscheidung dem Plan 06 überlassen. `pass` trug nur `at`, `nodes`, `read`; nach einem Anstoß stimmten „n von m“ nicht. Jetzt trägt `pass` `members` (Maschine zu Hostname), und `HealthFor` leitet `Nodes`, `ReadNodes`, `Drifted` und Items aus den Lesungen der Mitglieder ab (ein Mitglied ohne Lesung ist `not-read`, eine Lesung eines Knotens, der nicht Mitglied ist, zählt nicht). `configdrift.go` steht nicht in `files_modified` des Plans (Änderung: Feld `members` statt `nodes`/`read`, Aufbau in `Refresh`). Test und Injektion: (e) in Task 1. Folge: `Refresh` ignoriert den Rückgabewert von `readNode`; der Linter hat das nicht beanstandet.

**2. [Rule 2 - Sicherheit] Keine Fehlertexte in der Clusterliste.** Der Satz einer gescheiterten Lesung kann eine Adresse, der einer gescheiterten Ablage einen Pfad des Datenverzeichnisses enthalten (`reasonUnreadable`, `reasonStoreFailed`). Die Liste ist Reader-lesbar, `Detail` nicht. `listSentence` ersetzt diese beiden Sätze in der Liste durch feste Sätze (der letzte gute Zeitpunkt bleibt). Test und Injektion (f) in Task 1. Nicht im Plan; die Bedrohung T-15-21 (nie Pfade oder Werte) deckt es.

**3. Zusätzliche Tests.** `TestTheStatesOfTheViewAreTheStatesOfThePackage` (die Wiederholung der Zustandsstrings in `model` darf nicht von `configdrift` abweichen), `TestAnUnreadableNodeIsNotCountedAsRead` (vom Plan als (f) von Task 3 verlangt, hier mit Namen wie im Plan), `TestWithoutAFunctionThereIsNoConfigDrift` und `TestTheClusterViewCarriesWhatTheDriftFunctionGives` wie verlangt; dazu die Metrik-Tests wie benannt. Ein Test in `view_test.go` heißt `TestTracerAReadingReachesTheMetrics` wie verlangt.

**4. Die Sätze und Zustandstexte sind Englisch**, wie die der Zertifikate (`certwatch.Sentence`) und der übrigen Sätze von `configdrift`; der Plantext beschreibt die Form auf Deutsch.

**5. Der Import des Inventars in `view_test.go` heißt `invsvc`**, weil `lifecycle_test.go` im selben Paket schon einen Typ `inventory` hat.

**6. Der Banner-Satz zählt „übrige“ als die übrigen Knoten mit bekannter Drift**, gleich wie alt sie sind.

## Auth gates

Keine.

## Known Stubs

Keine.

## Was NICHT belegt ist

- Die Anzeige gegen einen echten Cluster: alles lief gegen den Simulator (`talossim`) und gegen Sichten von Hand.
- `-race` lief nur auf dem Mac, nicht auf dem Pi (dort nicht lauffähig, CI bleibt maßgeblich); es gibt keinen Pi in dieser Sitzung.
- Die Wächter gegen Konfigurationsinhalt in der Clusterliste und im Banner-Satz sind rot gesehen, aber nur gegen **hineingeschriebene** Pfade (Injektionen (h1) und (h2)): sie beweisen, dass die Behauptung greift, nicht dass der Code nirgends sonst etwas durchlässt. Der Code baut die Sätze ausschließlich aus Hostname, Clustername, Zeit und den festen Sätzen der Lesung; ein anderer Weg zum Leck ist nicht geprüft.
- Der Hostname als Label: ein Knoten ohne Hostnamen im Inventar hätte `node=""`; das ist nicht behandelt (bei den Zertifikaten genauso).
- Die Metrik ist bis zu einen Takt (drei Stunden) alt; das Lesealter sagt es, aber der Takt ist hier nicht verändert.
- Das Zusammenspiel mit der Oberfläche (Karte „not read yet“, Banner) ist nicht gebaut und nicht geprüft: Oberfläche, HTTP und Wand sind nicht Teil dieses Plans (Pläne 07 bis 09).

## Threat Flags

Keine neue Angriffsfläche über den Plan hinaus. T-15-21 (Information Disclosure, Clusterliste): Tracer (rot mit (h1)), Banner-Satz (rot mit (h2)) und `TestAnUnreadableNodeCarriesNoErrorTextInTheList` mit Injektion (f). T-15-22 (Label): `TestConfigDriftLabelsAreNamesAndNeverMachineIDs`, rot mit (e2) (und (e) über die Vorbedingung). T-15-23 (falsche Zusicherung): rot mit 1(a), 2(a), 2(c), 2(c2), 2(f) und 3(f). T-15-SC: nichts installiert.

## Self-Check: PASSED

Dateien vorhanden (`model/configdrifthealth.go`, `configdrift/health.go`, `health_test.go`, `view_test.go`, `inventory/configdrift_test.go`, `metrics/configdrift_test.go`; geändert `configdrift/configdrift.go`, `inventory/inventory.go`, `inventory/view.go`, `metrics/metrics.go`, `metrics/metrics_test.go`), Commits 9ab5c7a, baa6d7e und f205cba sind Vorfahren von HEAD.
