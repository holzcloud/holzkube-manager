---
phase: 15-drift-erkennung-der-maschinenkonfiguration
plan: 03
subsystem: configdrift
tags: [drift, reader, baseline, deadline, ledger-137]
requires: [15-01, 15-02]
provides:
  - "internal/configdrift: Deps, Service, New, Run, Tick, Refresh, Detail, die sieben Zustände, Detail mit Diff und Notice"
  - "die Entscheidungstabelle (erste Sicht, ok, Drift, beschädigt, unlesbar, Re-Normalisierung) unter storeMu"
  - "Testaufbau (newNode, service, thirdPartyChange, secrets, fakeReader, clock, logSink), auf dem die Pläne 04 bis 07 bauen"
affects: [15-04, 15-05, 15-06, 15-07, 15-08, 15-10]
actuals:
  tokens: 15500
  tasks: 2
  commits: 2
plan_head_before: 160a6a4e272a68319c0fdd6f625d7c1ea8ccdde5
plan_head_after: 7ae661d22665538b0d0931e5452d05910d3304cf
tech-stack:
  added: []
  patterns: ["eigenes Budget je Knoten um Verbindung und Lesen", "nur Übergänge loggen", "Beginn einer Drift im Datensatz, nicht im Speicher"]
key-files:
  created:
    - internal/configdrift/configdrift.go
    - internal/configdrift/decide.go
    - internal/configdrift/detail.go
    - internal/configdrift/helpers_test.go
    - internal/configdrift/tracer_test.go
    - internal/configdrift/reader_test.go
    - internal/configdrift/deadline_test.go
  modified: []
key-decisions:
  - "Eine Lesung schreibt nie etwas Unsicheres: PEM-Gürtel und Größendeckel stehen vor jedem Zugriff auf den Store, also auch vor dem Verändern einer vorhandenen Basis"
  - "unreadable übernimmt den Beginn einer Drift aus dem Datensatz (Quelle der Wahrheit), nicht aus der vorigen Lesung; ein frischer Prozess weiß ihn dadurch auch ohne eine gute Lesung"
  - "Logzeilen nennen Cluster, Hostname, Maschine und Zustand, nie einen Fehlertext; ok ist der Ruhezustand, die angelegte Basis zählt als ok"
requirements-completed: []
status: complete
---

# Phase 15 Plan 03: Der Leser Summary

Das Paket `internal/configdrift` liest jeden eingeschalteten Knoten jedes eingeschalteten Clusters unter einem eigenen Budget von 20 Sekunden, legt beim ersten Mal die Basis an und sagt es, erkennt danach jede Abweichung in jedem Dokument, setzt den Beginn einer Drift genau einmal in den Datensatz und meldet in jedem Fehlerfall "unbekannt" statt "in Ordnung". Der Tracer läuft von `context.Background()` durch den echten Talos-Client gegen talossim in einen echten `fsstore` und endet bei `Detail` mit dem Diff.

BASE: 160a6a4e272a68319c0fdd6f625d7c1ea8ccdde5

`plan_head_before` ist derselbe Wert. `git rev-list --count BASE..HEAD` vor diesem SUMMARY-Commit: 2.

## Wo es lief

Mac (darwin/arm64), `go1.27.2`. Kein echter Cluster, kein Pi, kein `task`. `go test -race ./internal/configdrift/` lief hier grün; auf dem Pi geht das nicht, CI bleibt maßgeblich. Der CI-Linter (`GOTOOLCHAIN=go1.26.9 ... golangci-lint@v2.13.1 run ./...`) meldet `0 issues.` (der erste Lauf fand 9: sieben Doc-Kommentar-Formen, ein ungeprüftes Type-Assert im Test, eine unbenutzte Option im Testaufbau; alle behoben). Nichts gepusht, nichts getaggt, kein Changelog, nichts installiert.

## Was gebaut wurde

- `internal/configdrift/configdrift.go` (5a42ca4, ergänzt 7ae661d): Paketkommentar mit Zweck, Vorbild certwatch und den drei Unterschieden; `Deps`, `New` (Standardwerte 3 h, 20 s, 15 min), `Run`, `Tick` (überspringt Cluster mit `Disabled`), `Refresh` (überspringt Knoten mit `Disabled`, liest seriell, `context.WithTimeout(ctx, ReadBudget)` um `Deps.Read`, legt die Pass-Angabe ab, wirft Lesungen verschwundener Knoten weg).
- `internal/configdrift/decide.go`: die Entscheidungstabelle unter `storeMu`: (0) unsicherer Schnappschuss, (1) Basis fehlt, nur `ErrNotFound` zählt (erste Sicht, Source `first-sight`, Machinery `gendata.VersionTag`), (2) beschädigte Basis, (3) gleicher Digest oder nach Neu-Normalisierung gleich (ok, Text einmal neu geschrieben, `DriftSince`/`AppliedOnDriftAt` gelöscht), (4) Drift (Beginn einmal geschrieben). Bei `ErrConflict` bis zu dreimal neu laden und neu entscheiden. `set` loggt Zustandswechsel.
- `internal/configdrift/detail.go`: `State` mit den sieben Konstanten, `ErrUnknownMachine`, `Detail` (snake_case wie im Plan), `Notice`, die festen Sätze, `Detail()` (Basis tolerant gegen Fehler, Diff nur bei Drift über `CompareDocuments`, ein nie gelesener Knoten ist `not-read` auch bei vorhandener Basis).
- Tests: `helpers_test.go` (`newNode`, `reopen`, `service` mit Optionen, `thirdPartyChange`, `secrets`, `fakeReader`, `clock`, `logSink`), `tracer_test.go` (Tracer und `TestEveryChangeCountsNoPathIsExcepted` mit drei Fällen), `reader_test.go` (die zwölf Randtests des Plans außer den beiden Frist-Tests, dazu zwei zusätzliche, siehe Abweichungen), `deadline_test.go` (die zwei Wächter gegen Ledger 137).
- `internal/machineconfig`, `internal/store`, `internal/certwatch`, `internal/changelog` sind gegen BASE unberührt: `git diff --name-only $BASE -- internal/machineconfig internal/store internal/certwatch internal/changelog` ist leer.

## Rot gesehen

Alle Exitcodes sind die des `go test`-Befehls selbst (in Dateien umgeleitet, kein Pipe-Exit). Jede Injektion: genau eine Änderung am Produktionscode (`inject.sh` verlangt, dass die Ersetzung genau einmal trifft), vorher `go build ./... && go vet ...` mit Exit 0, Wiederherstellung per `cp` und `cmp -s` mit Exit 0, danach grün. Zwei Injektionen haben beim ersten Versuch nur den Build gebrochen (2(b) `declared and not used: since`, 2(i) `declared and not used: prev`); das ist keine Messung, beide wurden mit einer Fassung wiederholt, die baut, und nur die Wiederholung zählt.

ROT vor jedem Eingriff:

| Task | Lauf | Exit | scheiternde Zeile |
|---|---|---|---|
| 1 | Tests ohne Produktionsdateien | 1 | `no non-test Go files in .../internal/configdrift`, `[build failed]` (Compile-Fehler, vom Plan als Rot zugelassen) |
| 2 | alle neuen Tests gegen den Stand nach Task 1 | 1 | `TestAnEncodingChangeIsNotDrift`, `TestADriftingNodeStaysDriftingWhenItStopsAnswering` (Beginn nach Neustart und Lesefehler leer), `TestAPrivateKeyInASnapshotIsNeverWritten` (beide Fälle), `TestADamagedBaselineIsReportedAndNeverOverwritten` (alle vier Varianten), `TestAnOutageIsLoggedOnTheTransitionOnly` |

Schon grün gegen den Stand nach Task 1 und deshalb **nicht** als Rot gezählt: `TestAFailedReadIsNeverNoDrift`, `TestANodeNeverReadAndNotAnsweringHasNoBaseline`, `TestAnUnreadableConfigurationStoresNothing`, `TestNoSecretReachesTheBaselineFile`, `TestTheStartOfADriftSurvivesARestart`, `TestEveryReadCarriesADeadline`, `TestARunningDaemonsContextReadsARealNode`. Sie sind nur über ihre Injektionen unten belegt.

Task 1, Injektionen (gegen die Fassung 5a42ca4):

| Injektion | Exit | scheiternde Zeilen |
|---|---|---|
| (a) die Basis wird bei jeder gleichen Lesung neu geschrieben | 1 | Tracer: `a reading that found nothing new rewrote the baseline`, `rev is 2 after an equal reading, want 1`, `the record is rev 3 ... want rev 2` |
| (b) der Drift-Zweig liefert ok | 1 | Tracer: `state after a third party's change is "ok" ..., want drift`; `TestEveryChangeCountsNoPathIsExcepted`: alle drei Fälle `state is "ok" ..., want drift` |
| (c) `DriftSince` wird bei jeder abweichenden Lesung neu gesetzt (`if true`) | 1 | Tracer: `DriftSince moved or was rewritten: rev 3, ... then ...` |

Task 2, Injektionen (gegen die Endfassung):

| Injektion | Test | Exit | scheiternde Zeilen |
|---|---|---|---|
| (a) Fehlerzweig setzt `StateOK` | `TestAFailedReadIsNeverNoDrift` (und zwei weitere) | 1 | `pass 0: a node that could not be read is "ok"`; `state is "ok", want unreadable` |
| (b) Fehlerzweig verliert den Beginn (`next.since = time.Time{}`) | `TestADriftingNodeStaysDriftingWhenItStopsAnswering` | 1 | `DriftSince is 0001-01-01 ... after a failed reading, want 2026-10-14 10:00:00`; `after a restart and a failed reading: ... since 0001-01-01` |
| (c) beschädigte Datei wie fehlende behandelt (`err != nil` statt `ErrNotFound`) | `TestADamagedBaselineIsReportedAndNeverOverwritten/garbage` | 1 | `state is "unreadable" (The baseline could not be stored or loaded: invalid character 'h' ...), want baseline-damaged` |
| (c2, zusätzlich) `damageOf` liefert immer "" | dieselbe, Varianten `an_unknown_format`, `a_changed_snapshot_under_the_old_digest` | 1 | `state is "ok" (...), want baseline-damaged` |
| (d) PEM-Prüfung vor dem Schreiben entfällt | `TestAPrivateKeyInASnapshotIsNeverWritten` | 1 | `state is "baseline-created", want unreadable`; `a snapshot with a private key was written: <nil>` |
| (e) Neu-Normalisierung entfällt | `TestAnEncodingChangeIsNotDrift` | 1 | `state is "drift" (...), want ok: another encoding is not a difference` |
| (f) `snapshot.go` baut den Text aus `raw` statt `redacted` | `TestNoSecretReachesTheBaselineFile` | 1 | `state is "unreadable" (The configuration still held a private key after redaction...), want baseline-created` (siehe Abweichung 2) |
| (f2, zusätzlich) wie (f) und der PEM-Gürtel in `decide.go` entfernt | dieselbe | 1 | `the file holds the cluster secret`, `the snapshot holds the bootstrap token`, `... the machine (trustd) token`, `... the Talos CA private key`, `... the etcd CA private key`, `the baseline holds a PEM private key` |
| (g) Budget je Knoten entfällt (`WithCancel`) | `TestEveryReadCarriesADeadline` | 1 | `Read was handed a context without a deadline; the Talos client refuses such a call` |
| (h1) nur das Budget entfällt | `TestARunningDaemonsContextReadsARealNode` | **0, grün** | `ReadSnapshot` setzt die Klassenfrist selbst: eine Schicht fängt die andere. So vom Plan vorhergesagt. |
| (h2) nur die Klassenfrist in `ReadSnapshot` entfällt | dieselbe | **0, grün** | das Budget trägt. Wie (h1). |
| (h) beides entfällt (zusammengesetzt) | dieselbe | 1 | `state "unreadable" (The node could not be read: talos: Get on m1: talos: refusing a call with no deadline ...)` |
| (i) Übergangsfilter beim Loggen entfällt (`|| true`) | `TestAnOutageIsLoggedOnTheTransitionOnly` | 1 | `two good passes wrote 2 lines, want at most the baseline's: [... state=baseline-created ... state=ok]` |
| (j) `DriftSince` wird nicht in den Datensatz geschrieben | `TestTheStartOfADriftSurvivesARestart` | 1 | `DriftSince is 2026-10-15 16:00:00 +0000 UTC after a restart, want the original 2026-10-14 10:00:00`; `the record holds DriftSince 0001-01-01 ...` |

Zusätzliche Injektionen (Rule 2, nicht vom Plan verlangt), je Exit 1, Wiederherstellung per `cmp` Exit 0: die Lesung verschwundener Knoten wird nicht verworfen (`TestAVanishedNodeLosesItsReading`: `a node that left the list is still "baseline-created"`); `Disabled` am Knoten wird nicht mehr übersprungen (`a disabled node was read 1 times`); `Disabled` am Cluster wird nicht mehr übersprungen (`a node of a disabled cluster was read 1 times`).

Die Datei `internal/machineconfig/snapshot.go` wurde für (f), (f2), (h2) und (h) angefasst und jedes Mal per `cmp -s` auf byte-gleich zurückgestellt (Exit 0); `git diff --stat -- internal/machineconfig` ist leer.

## Verifikation

- `go test ./internal/configdrift/ ./internal/machineconfig/ ./internal/store/... ./internal/certwatch/ ./internal/planning/ -count=1`: alle ok.
- `go test -race ./internal/configdrift/ -count=1`: ok (nur Mac).
- `go test ./internal/store/fsstore/ -run TestNoDirectFileAccessOutsideFsstore -count=1`: ok. `go test ./internal/publicrepo/ -count=1`: ok.
- `go build ./...`, `go vet ./internal/configdrift/`, `gofmt -l internal/configdrift`: sauber.
- Die fünf Tests der zweiten Verify-Zeile von Task 2 ergeben fünf `--- PASS`.
- CI-Linter über `./...`: `0 issues.`
- `grep -n 'context.WithTimeout' internal/configdrift/configdrift.go` trifft den Aufruf um `Deps.Read` (Zeile 241).
- Nicht untersucht, nicht meine: die vier `internal/httpapi`-Tests, die `web/dist` brauchen.

## Abweichungen vom Plan

**1. [Test-Fehler, im Lauf behoben] Der Test der Neu-Normalisierung zerschnitt die PEM-Zeilen.** Der erste Entwurf zerlegte den Schnappschuss mit `strings.Split(text, "---\n")`. Die Rüstzeilen der Zertifikate (`-----BEGIN CERTIFICATE-----`) enden auf `---\n`, der Schnitt ging durch sie, der Text wurde unlesbar und `snapOf` meldete `ErrSnapshotUnreadable`. Geholfen hat eine Zerlegung an den Zeilen, die nur `---` enthalten (`splitDocuments` im Test); eine Probe gegen sechs Umordnungen (umgekehrt, v1alpha1 zuerst, Tausch zweier Dokumente) hat bestätigt, dass die Normalisierung idempotent ist und jede Reihenfolge auf denselben Digest führt. Das ist die Annahme, auf der die Neu-Normalisierung steht, und sie hält.

**2. [Plan-Annahme unvollständig] Injektion (f) ist rot, aber nicht an der Zeile, die der Plan meint.** Der Plan erwartet, dass `TestNoSecretReachesTheBaselineFile` an einem Geheimnis in der Datei scheitert. Mit dem Rohtext im Schnappschuss fängt der PEM-Gürtel in `decide.go` zuerst: der Zustand ist `unreadable` und es entsteht keine Datei, der Test scheitert am Zustand. Das ist die Tiefenverteidigung, die der Plan will, aber die Datei-Prüfung selbst war damit nicht gemessen. Deshalb (f2): beide Schichten entfernt, dann scheitern die Zeilen für Cluster-Secret, Bootstrap-Token, Trustd-Token, Talos- und etcd-CA-Schlüssel an der Datei und am Schnappschuss. Zwei Mängel des Tests selbst kamen dabei ans Licht und sind behoben: die Basis-Datei ist JSON und maskiert `<` und Zeilenumbrüche, ein Marker- oder PEM-Vergleich gegen die Rohdatei wäre leer ausgegangen, deshalb wird zusätzlich der aus der Datei dekodierte Schnappschuss geprüft, und die Marker-Prüfung ist ein `t.Error`, kein `t.Fatal`, damit die Geheimnis-Schleife auch dann läuft.

**3. [Plan-Annahme] Das Überschreiben einer beschädigten Datei verhindert in der Garbage-Variante schon der Store.** Injektion (c) (Fehler wie "fehlt" behandeln) führt dort zu einem `Put` mit Rev 0, den `fsstore` über eine unlesbare Datei ablehnt (Plan 02, Injektion 1(c)); der Test wird rot, weil der Zustand `unreadable` statt `baseline-damaged` ist, nicht weil etwas überschrieben worden wäre. Für die beiden gültigen JSON-Varianten (unbekanntes Format, veränderter Text) trägt die Prüfung `damageOf` allein; (c2) misst sie. Die Variante "leerer Snapshot" bleibt bei (c2) grün, weil die Neu-Normalisierung des leeren Textes ebenfalls zu `baseline-damaged` führt; sie hat also zwei Schichten und ist nur mit beiden gemessen.

**4. [Rule 2] Zwei zusätzliche Tests.** `TestSwitchedOffNodesAndClustersAreNeverRead` (ein ausgeschalteter Knoten und alle Knoten eines ausgeschalteten Clusters werden nie gelesen, ein eingeschalteter wird es: die Vorbedingung, dass der Leser überhaupt aufgerufen wird) und `TestAVanishedNodeLosesItsReading`. Beide stehen als Wahrheiten im Plan, hatten aber keinen eigenen Test; je mit Injektion rot gesehen (siehe oben).

**5. Kleinigkeit im Code, die der Plan offen ließ.** `pass` hat neben `at` und `nodes` ein Feld `read` (Anzahl der verglichenen Knoten), damit Plan 06 "n von m gelesen" ohne Umbau bekommt. Der Satz bei einem Lesefehler wird um einen Punkt ergänzt, wenn der Grund keinen hat (der erste Lauf hatte "no deadline It has not been read"). Ein Fehler beim Speichern oder Laden der Basis (Platte voll, Rechte) wird `unreadable` mit eigenem Grund und nicht `baseline-damaged`, weil dort nichts über die Datei bekannt ist.

## Auth gates

Keine.

## Known Stubs

Keine. `StateSettling`, `DefaultSettleWindow`, `Deps.SettleWindow` und der Satz zu `AppliedOnDriftAt` sind vorhanden, werden aber erst von den Plänen 04 und 05 gesetzt beziehungsweise gelesen. Das ist der Zuschnitt des Plans (der Satz wird aus dem Feld gebaut, das erst Plan 05 schreibt), kein Platzhalter im Code. Das Paket hat noch keinen Aufrufer außer den Tests; `main.go` verdrahtet es in einem späteren Plan.

## Was NICHT belegt ist

- **A1** (Talos schreibt die aktive Konfiguration nicht selbst um) und **A2** (ein echter Knoten liefert semantisch das Gesendete): nur gegen talossim gesehen, nie gegen einen echten Knoten. Ob ein echter Knoten nach einem Neustart oder Upgrade etwas an der aktiven Konfiguration verändert, das hier als Drift erscheint, ist ungeprüft.
- Die Wirkung auf dem Pi und gegen einen echten Cluster: nicht gelaufen. Insbesondere die Grenze "ein nur gestoppter Cluster kostet je Knoten bis zu fünf Sekunden, seriell" ist aus dem Code der Sonde gelesen, nicht gemessen; der Leitfaden-Satz dazu gehört Plan 10.
- `-race` lief nur auf dem Mac, nicht auf dem Pi; CI bleibt maßgeblich.
- Das Budget je Knoten ist nur gegen talossim und einen Fake-Leser belegt. Ein Knoten, der die Verbindung annimmt und dann schweigt, so dass das Budget von 20 Sekunden tatsächlich abläuft, wurde nicht nachgestellt (ein solcher Test würde 20 s dauern oder ein verstellbares Budget brauchen; `Deps.ReadBudget` ist verstellbar, ein Test mit kurzem Budget gegen einen hängenden Leser steht aus).
- Die Injektionen (a) bis (c) von Task 1 liefen gegen die Fassung von Task 1 (5a42ca4), nicht gegen die Endfassung; die betroffenen Zweige stehen unverändert in der Endfassung, die Tests laufen grün, aber die drei Messungen sind nicht wiederholt.
- Ein gleichzeitiger Schreiber derselben Basis (Plan 04) ist nur über den Wiederholungszweig bei `ErrConflict` vorbereitet; dieser Zweig hat keinen eigenen Test, weil es in diesem Plan keinen zweiten Schreiber gibt.
- Der Satz bei `AppliedOnDriftAt` (D-13) ist gebaut, hat aber keinen Test, weil erst Plan 05 das Feld schreibt.
- Rotierte Geheimniswerte und von `Redact` nicht erfasste Felder bleiben unsichtbar (D-02, D-14); `Detail.Notice` sagt es, die Ledger-Einträge führt Plan 10.
- Die Meldung in `StateBaselineDamaged` nennt die Datei `config-baselines/<ID>.json`; dass der Betreiber sie danach auf irgendeinem Weg wieder in Ordnung bringt (Löschen und neu anlegen), liefert Plan 04 (`Forget`).

## Threat Flags

Keine neue Angriffsfläche über die im Plan geplante hinaus (T-15-08 bis T-15-12 sind mitigiert und rot gesehen, T-15-SC: nichts installiert).

## Self-Check: PASSED

Dateien vorhanden (`configdrift.go`, `decide.go`, `detail.go`, `helpers_test.go`, `tracer_test.go`, `reader_test.go`, `deadline_test.go`), Commits 5a42ca4 und 7ae661d sind Vorfahren von HEAD.
