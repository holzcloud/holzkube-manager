---
phase: 15-drift-erkennung-der-maschinenkonfiguration
plan: 01
subsystem: machineconfig
tags: [drift, snapshot, redact, multi-document-diff, talos]
requires: []
provides:
  - "machineconfig.Snapshot: normalisierter, redigierter Schnappschuss mit Digest"
  - "machineconfig.CompareDocuments: Vergleich ueber alle Dokumente einer Konfiguration"
  - "machineconfig.Service.ReadSnapshot: liest einen Knoten mit eigener Klassenfrist"
affects: [15-02, 15-03, 15-04, 15-05, 15-08, 15-10]
actuals:
  tokens: 6975
  tasks: 2
  commits: 2
plan_head_before: f7cc6bfa3ecfac36c71b3914d1d5cbcb35a40cb6
plan_head_after: dc6661c458648bd5873fb119a014f2b72de3771c
tech-stack:
  added: []
  patterns: ["Snapshot nur aus Redact, bei Fehler jedes Byte verwerfen", "Klassenfrist im Leser, nicht beim Aufrufer"]
key-files:
  created:
    - internal/machineconfig/snapshot.go
    - internal/machineconfig/snapshot_node_test.go
    - internal/machineconfig/snapshot_test.go
  modified: []
key-decisions:
  - "Snapshot-Text: Dokumente nach kind[/name] sortiert (v1alpha1 fuer das Dokument mit version), doppelte Schluessel mit #2, #3; Kommentare weg; yaml.v3 mit 2 Leerzeichen Einzug, Dokumente mit '---' verbunden"
  - "Ein Fehler von Redact verwirft jedes Byte; ErrSnapshotUnreadable hat einen festen Satz ohne Konfigurationsinhalt"
requirements-completed: [DRIFT-02, DRIFT-05]
status: complete
---

# Phase 15 Plan 01: Schnappschuss und Mehrdokument-Vergleich Summary

Ein Schnappschuss einer Knotenkonfiguration (nur aus `Redact`, normalisiert, sha256-Digest), ein `CompareDocuments`, das Aenderungen in jedem Dokument sieht (wo `Compare` nur das erste liest), und `Service.ReadSnapshot`, das auf einem Kontext ohne Frist einen echten Knoten liest.

BASE: eba8d7083533593a68287f93ec29baf7d32dfc3b

Hinweis zur Messung von `commits:`. Zwischen BASE und meinem ersten Commit liegt der Commit f7cc6bf (Planungs-Konfiguration `allow_default_branch_commits`, vom Koordinator auf Wahl des Betreibers; nicht Teil dieses Plans). `plan_head_before` ist deshalb f7cc6bf, der HEAD vor meinem ersten Commit; `git rev-list --count f7cc6bf..HEAD` ergab 2 (diese beiden Task-Commits, vor dem SUMMARY-Commit). Alle Guards gegen BASE laufen wie verlangt.

## Wo es lief

Mac (darwin/arm64), `go version go1.27.2 darwin/arm64`, gegen `internal/talossim`. Kein echter Cluster, kein Pi, kein `task`. `go test -race ./internal/machineconfig/` lief hier trotzdem gruen (ok); auf dem Pi geht das nicht, CI bleibt massgeblich. Nichts gepusht, nichts getaggt, kein Changelog.

## Was gebaut wurde

- `internal/machineconfig/snapshot.go` (Commit 61ffac7): `MaxSnapshotBytes` (1 MiB), `Snap{Text, Digest}`, `ErrSnapshotUnreadable`, `ErrSnapshotTooLarge`, `ErrSnapshotUnsafe`, `Snapshot`, `CompareDocuments`, `(*Service).ReadSnapshot`. `diff.go`, `redact.go`, `service.go`, `applymode.go` sind unberuehrt (`walk`, `render`, `unionKeys` werden nur aufgerufen).
- `internal/machineconfig/snapshot_node_test.go` (61ffac7): der Tracer `TestAChangeInAnotherDocumentIsSeenThroughTheRealClient`. Echter Client gegen talossim, gelesen von `context.Background()`, ein Zweitclient aendert `KubeletConfig.image` per `no-reboot`-Apply, `CompareDocuments` liefert genau eine Aenderung `KubeletConfig.image` (modified). Die Zeile mit `Compare` (0 Aenderungen) steht als Zeuge und ausdruecklich nicht als Waechter; wird sie rot, ist der Blindfleck von `Compare` repariert und der Ledger-Eintrag (Plan 10) zu schliessen.
- `internal/machineconfig/snapshot_test.go` (Commit dc6661c): zwoelf Randtests nach der Liste des Plans.

## Rot gesehen

Alle Exitcodes sind die des `go test`-Befehls selbst (nicht die einer Pipe). Jede Injektion: genau eine Aenderung an `snapshot.go`, vorher `go build ./... && go vet ./internal/machineconfig/` mit Exit 0, danach Wiederherstellung per `cp` und `cmp -s` (Exit 0), danach gruen (`ok`).

Task 1 (Tracer):

| Lauf | Exit | scheiternde Zeile |
|---|---|---|
| ROT vor dem Eingriff | 1 | `[build failed]`: `svc.ReadSnapshot undefined`, `undefined: machineconfig.CompareDocuments` (Compile-Fehler, vom Plan als Rot zugelassen) |
| 1(a) `CompareDocuments` nur ueber den ersten Dokumentschluessel (`sorted[:1]`) | 1 | `--- FAIL: TestAChangeInAnotherDocumentIsSeenThroughTheRealClient`: `CompareDocuments found 0 changes, want exactly 1: []` |
| 1(b) `Snapshot` zerlegt `raw` statt `redacted` | 1 | `the snapshot carries no redaction marker; it was not made from Redact` sowie `the snapshot leaks the Talos CA private key / etcd CA private key / cluster secret / bootstrap token / machine (trustd) token` |
| 1(c) `ReadSnapshot` ohne `WithClassDeadline` | 1 | `ReadSnapshot from a context without a deadline: talos: Get on m1: talos: refusing a call with no deadline` |

Task 2 (Raender):

| Injektion | Test | Exit | scheiternde Zeile |
|---|---|---|---|
| 2(a) Sortierung der Dokumente entfaellt | `TestSwappedDocumentsAreTheSameSnapshot` | 1 | `the same documents in another order are another digest` und `... another text` |
| 2(b) Dokumentschluessel ignoriert `name` (`name != "" && false`) | `TestDocumentsOfTheSameKindAreToldApartByName` | 1 | `Paths is [KubeAuthorizerConfig#2.name KubeAuthorizerConfig.name], want [KubeAuthorizerConfig/node.type KubeAuthorizerConfig/rbac.type]` |
| 2(c) Teil-Bytes von Redact genutzt, kein Fehler | `TestAnUnparsableConfigurationYieldsNoSnapshot` | 1 | `a configuration that does not load produced a snapshot` |
| 2(d) `CompareDocuments` ohne erneute Normalisierung | `TestCompareDocumentsNeverCarriesASecret` | 1 | `the comparison carries the planted secret at v1alpha1.cluster.token` / `v1alpha1.machine.token` (und umgekehrt) |
| 2(e) Groessendeckel fuer Rohbytes entfaellt | `TestASnapshotOverTheCapIsRefused` | 1 | `the error is machineconfig: this version of holzkube-manager could not load ..., want ErrSnapshotTooLarge` |
| 2(f) Schnappschuss aus `raw` gebaut | `TestNoSecretInTheSnapshot` | 1 | `the snapshot leaks the cluster secret`, `... bootstrap token`, `... machine (trustd) token`, `the snapshot still contains a PEM private key block`, `the snapshot carries no redaction marker` |
| 2(g) `Changes`/`Paths` bei leerem Vergleich nil | `TestAnEmptyComparisonSerialisesEmptyListsAndNotNull` | 1 | `an empty comparison is {"changes":null,"paths":null,"duplicates":false}, want empty lists` |

Alle sieben haben tatsaechlich injiziert und nicht nur den Build gebrochen; keine Injektion blieb gruen, es gab keine Lucke zu ersetzen.

Zwei Tests halten Blindflecke fest und zaehlen **nicht** als rot gesehen: `TestAChangedSecretValueIsInvisibleByDesign` (D-02: ein geaenderter Geheimniswert ist unsichtbar) und der zweite Teil von `TestTheSnapshotHoldsNoMoreAndNoLessThanTheConfigView` (D-14: `machine.env` und `machine.files` stehen im Text, wie die Config-Ansicht sie zeigt). Beide verweisen im Kommentar auf den offenen Ledger-Eintrag des Plans 10. Die Patchmaschine hat den Eintrag unter `machine.files` angenommen; beide eingepflanzten Werte stehen im Schnappschuss.

## Abweichungen vom Plan

- **Task 2, Schritt 1 ("Rot vor Schritt 2"):** Alle zwoelf Randtests waren beim ersten Lauf bereits gruen, weil Task 1 den Deckel, den festen Fehlersatz, die Schluessel mit Name und die Normalisierung in `CompareDocuments` schon enthielt. Es gab deshalb keinen separaten ROT-Lauf vor einem Code-Schritt von Task 2; Rot kommt hier ausschliesslich aus den Injektionen 2(a) bis 2(g), wie der Plan es fuer diesen Fall vorsieht. Schritt 2 (GRUEN) brauchte keine Codeaenderung.
- **Commit-Guard:** Mein Guard verbietet Commits auf `main`. Ich habe an dieser Stelle angehalten und gemeldet; der Koordinator hat auf Wahl des Betreibers `git.allow_default_branch_commits: true` gesetzt und mit f7cc6bf committet. Ich habe das mit `gsd_run query git.base-branch --is-protected main` (jetzt `false`) selbst geprueft, bevor ich committet habe. Das ist die Ursache der Differenz zwischen BASE und `plan_head_before` oben. Keine Codeabweichung.

- **REQUIREMENTS.md:** `requirements mark-complete DRIFT-02 DRIFT-05` lief, ich habe das Ergebnis wieder verworfen. DRIFT-02 verlangt auch die Basis im Datenverzeichnis (Plan 02) und DRIFT-05 die Anzeige auf der Knotenseite (spaetere Plaene); dieser Plan liefert nur den Baustein (Schnappschuss, Vergleich). Ein "Complete" waere hier eine falsche Aussage. `requirements-completed` im Frontmatter nennt sie trotzdem, wie der Plan es angibt; das Abhaken gehoert dem Plan, der die jeweilige Anforderung abschliesst.

## Verifikation

- `go test ./internal/machineconfig/ ./internal/talossim/ ./internal/planning/ -count=1`: alle ok. `go test -race ./internal/machineconfig/`: ok (nur Mac).
- `go build ./...`, `go vet ./internal/machineconfig/`, `gofmt -l internal/machineconfig`: sauber.
- CI-Linter `GOTOOLCHAIN=go1.26.9 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1 run ./...`: `0 issues.`
- Guard gegen BASE: `git diff --name-only eba8d70... -- internal/machineconfig/diff.go redact.go service.go applymode.go internal/changelog internal/certwatch` ergibt leer.
- Nicht untersucht, nicht meine: die vier `internal/httpapi`-Tests, die `web/dist` brauchen.

## Was NICHT belegt ist

- **A1** (Talos schreibt die aktive Konfiguration nicht selbst um) und **A2** (ein echter Knoten liefert semantisch das Gesendete): nur gegen talossim gesehen, nie gegen einen echten Knoten.
- Kein Pi, kein echter Cluster, `-race` nur auf dem Mac gelaufen, CI bleibt massgeblich.
- Der Endtext-Deckel (`len(text) > MaxSnapshotBytes` nach der Normalisierung) und `ErrSnapshotUnsafe` haben keinen eigenen Test und keine Injektion: der Endtext ist nach der Rohbytes-Pruefung kaum erreichbar, der PEM-Guertel nicht, solange `Redact` haelt. Ungeprueft.
- Die Nummerierung doppelter Dokumentschluessel (`#2`) ist nur indirekt gesehen (Injektion 2(b) zeigt, dass zwei gleichartige Dokumente nicht verschmelzen); ein eigener Test, der zwei gleiche Schluessel erzeugt, fehlt, weil eine gueltige Konfiguration das nicht hergibt.
- Rotierte Geheimniswerte und von `Redact` unbekannte Felder (D-02, D-14) sind Absicht und im Ledger (Plan 10) zu fuehren, hier nur als Test gehalten.
- Die Blindstelle von `Compare` (liest nur das erste Dokument) ist hier nicht behoben (D-09), nur als Zeuge im Tracer gehalten.

## Threat Flags

Keine neue Angriffsflaeche ausser der im Plan geplanten.

## Self-Check: PASSED

Dateien vorhanden (`snapshot.go`, `snapshot_node_test.go`, `snapshot_test.go`), Commits 61ffac7 und dc6661c sind Vorfahren von HEAD.
