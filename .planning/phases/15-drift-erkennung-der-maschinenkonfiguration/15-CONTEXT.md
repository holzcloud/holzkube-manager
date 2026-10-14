# Phase 15: Drift-Erkennung der Maschinenkonfiguration - Context

**Gathered:** 2026-10-14
**Status:** Ready for planning

<domain>
## Phase Boundary

Der Betreiber sieht, wenn die aktive Konfiguration eines Knotens von dem
abweicht, was holzkube-manager zuletzt von ihm kannte oder gesetzt hat. Heute
fällt eine Änderung am Knoten selbst nicht auf, weil das Diff nur beim Patchen
berechnet wird (`AUDIT-2026-10-08.md`, Kandidat „Drift-Erkennung").

Geliefert wird: Lesen, Vergleichsbasis speichern, Anzeige (Knotenseite mit Diff,
Zeile auf der Clusterkarte, Banner nach 24 h, `/metrics`) und das Bestätigen
„so ist es gewollt". **Nicht** geliefert: Zurückschreiben auf den Knoten (eigene
Phase), ein Vergleich gegen Schwesterknoten oder gegen eine erzeugte
Konfiguration, vom Betreiber pflegbare Ausnahmen.

</domain>

<decisions>
## Implementation Decisions

Alle als Auswahl vom Betreiber am 2026-10-14 getroffen.

### Vergleichsbasis
- **D-01:** Basis ist der **letzte bekannte Stand**: die redigierte aktive Konfiguration, die holzkube-manager nach dem ersten Sehen eines Knotens und nach jedem eigenen Apply festhält. Drift heißt: der Knoten hat sich seitdem ohne uns verändert (talosctl, anderer Operator). Ein Knoten ohne Basis hat keine Abweichung; seine erste Lesung legt die Basis an und sagt das. — **Reversibility:** costly — die Basis ist persistierter Zustand im Datenverzeichnis, ein Format und ein Aktualisierungsweg, die später nur mit Migration zu ändern sind.
- **D-02:** Gespeichert wird ein **redigierter Schnappschuss** (Geheimnisse unkenntlich über `machineconfig.Redact`), kein Hash und keine vollständige Konfiguration. Damit zeigt die Seite, **was** sich geändert hat (Diff wie `machineconfig.Compare`). Eine Änderung nur eines Geheimniswerts bleibt unsichtbar; das ist zu benennen. Es kommt nie ein unredigierter Schlüssel in das Verzeichnis. — **Reversibility:** costly — Schnappschussformat im Datenverzeichnis.

### Rauschen
- **D-03:** Felder, die sich ohne Zutun ändern könnten, stehen in einer **kleinen festen Ausnahmeliste im Code, per Recherche belegt** (die Recherche misst, welche Pfade das sind). Alles andere zählt. Kein Bedienelement, kein gespeicherter Zustand beim Betreiber. Eine vergessene Ausnahme zeigt sich als Fehlalarm, nicht als übersehene Drift.

### Lesen
- **D-04:** Das Lesen ist **periodisch nach dem Muster `internal/certwatch`**: alle paar Stunden jeder erreichbare Knoten, dazu nach jedem eigenen Apply. Kein COSI-Watch. Ein Knoten, der nicht antwortet, behält seine letzte Lesung und wird als nicht gelesen gezählt; ein nie gelesener Knoten sagt „not read yet", nie „keine Drift".

### Anzeige
- **D-05:** Drift erscheint auf der **Knotenseite mit Diff**, als **Zeile auf der Clusterkarte** (wie „Cluster certificates"), als **Metrik in `/metrics`** (abgewichen 0/1 je Cluster und Knoten) und als **Banner auf jeder Seite**.
- **D-06:** Das Banner erscheint **erst, wenn Drift 24 Stunden unbestätigt bleibt**. Karte, Knotenseite und Metrik zeigen sie sofort. Der Betreiber hat das Banner trotz des Hinweises auf Dauerrauschen gewählt; die 24-Stunden-Schwelle ist die vereinbarte Abschwächung.

### Auflösung
- **D-07:** Der Betreiber kann das Diff ansehen und **„so ist es gewollt" bestätigen**: der aktuelle Stand wird die neue Basis. Aktion mit Admin-Rolle, Sudo-Fenster falls die Projektregeln es für Aktionen dieser Art verlangen, und Audit-Eintrag; kein Eingriff am Knoten. **Kein Zurücksetzen** in dieser Phase.

### Entscheidungen nach der Recherche (2026-10-14, als Auswahl)
- **D-08:** Die gespeicherten Basen bekommen eine **Schema-Migration 6 auf 7** (Konvention des Projekts). Bekannte Folge: ein vom Update-Timer zurückgerolltes Binary lehnt das Verzeichnis ab (`ErrVersionTooNew`); das Update-Skript hat dafür sein Rollback. — **Reversibility:** one-way — eine veröffentlichte Verzeichnisversion lässt sich nicht ohne Migration zurücknehmen.
- **D-09:** Der Mehrdokument-Vergleich ist eine **neue Funktion `CompareDocuments`** (Dokumente nach `kind[/name]` normalisiert). `machineconfig.Compare` bleibt unverändert; sein Blindfleck (liest nur das erste von bis zu 28 Dokumenten, gilt auch für `Plan`/`ModeFor`) wird als **offener Ledger-Eintrag** festgehalten, nicht repariert.
- **D-10:** Eigene Applies frischen die Basis über einen **erwarteten Digest am Engpass** `ClusterClient.ApplyConfigurationWithMode` auf (Aufrufer `Service.Apply`, `upgrade.applyImage`, `rotateca.rewrite`). Richtig auch für `staged`, `try`, `reboot`, `auto`; die Race „Apply im Flug" entfällt. Annahme A2 der Recherche (ein echter Knoten liefert nach Apply semantisch das Gesendete) ist ungeprüft und im Bericht zu nennen.
- **D-11:** „So ist es gewollt" ist eine **`Destructive`-Route** (Sudo-Fenster, Admin, Audit `config.drift-confirm`, Digest als Compare-and-Swap), nach dem Muster `backup-schedule`.
- **D-12:** Rauschen (D-03): die Recherche konnte **keinen** selbstverändernden Pfad belegen, die Ausnahmeliste ist daher **leer**. Es wird **kein Ausnahme-Mechanismus gebaut**; die Annahme „Talos schreibt die aktive Konfiguration nicht selbst um" (A1, nur Absenz in der Doku) steht im Bericht und im Ledger, und ein später gefundenes Rauschen wird dann gezielt ergänzt. Vom Betreiber am 2026-10-14 als Auswahl bestätigt (Auswahl A: nur dokumentieren, kein Mechanismus).
- **D-13:** Eine offene, unbestätigte Drift **bleibt sichtbar**, wenn ein eigener Apply auf dem Knoten landet. `DriftSince` bleibt gesetzt, der Satz sagt, dass der Stand „einen früheren, nie bestätigten Unterschied" einschließt; die Landungsregel (erwarteter Digest wird Basis) darf weder `DriftSince` noch die Drift löschen, und eine Erwartung, die auf eine offene Drift trifft, ist nie `settling`. Beendet wird eine offene Drift nur durch Bestätigen (D-07) oder dadurch, dass der Knoten zur Basis zurückkehrt (dann gibt es keinen Unterschied mehr). — **Reversibility:** reversible — Zustandsregel im Dienst, dazu ein zusätzliches Feld im Datensatz (`applied_on_drift_at`).
- **D-14:** Die rohen Felder, die `Redact` nicht kennt (`machine.files[].content`, `machine.env`, `machine.install.extraKernelArgs`, `cluster.inlineManifests[].contents`, `EnvironmentConfig`, `EtcFileConfig`, `ExtensionServiceConfig`), stehen im Schnappschuss **genau so, wie die Konfigurationsansicht sie schon zeigt** (Dateien 0600 in Verzeichnissen 0700, in Sicherungen wörtlich enthalten). Ein offener Ledger-Eintrag (179) und ein Absatz im Leitfaden benennen es; keine zusätzliche Schwärzung, kein Hash je Feld. — **Reversibility:** costly — eine spätere Schwärzung bedeutet, die gespeicherten Basen zu migrieren.
- **README:** kein Bild (Betreiber, 2026-10-14): die Funktion ist ein Abschnitt, eine Zeile und ein Banner auf vorhandenen Bildschirmen.

### Claude's Discretion
- Name und Ort des Pakets (nach dem Vorbild `certwatch`), Lesetakt in Stunden, Form der Persistenz (Datei je Knoten über `fsstore`), Wortlaut der Sätze. Alle Gestaltungsfragen, die ein Betreiber nicht entscheiden muss; wo doch eine echte Alternative besteht, geht sie als Auswahl zurück an den Betreiber.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

- `.planning/AUDIT-2026-10-08.md` — Kandidat „Drift-Erkennung der Maschinenkonfiguration" (Quelle).
- `CLAUDE.md` — öffentliches Repository, README im selben Schritt, Changelog erst beim Release, jeder Wächter rot gesehen, Fragen als Auswahl.
- `internal/machineconfig/service.go` — `read`, `renderFull`, `View`, `stagedRecord` (im Speicher, ausdrücklich nicht persistiert), `Compare` in `diff.go`.
- `internal/machineconfig/redact.go` — `Redact`, `ContainsPrivateKey`: der Typ trennt, es gibt keinen Weg zu Geheimnissen.
- `internal/machineconfig/diff.go` — `Compare`/`Diff`/`Change` für die Anzeige des Unterschieds.
- `internal/certwatch/certwatch.go` — Muster für periodisches Lesen, „not read yet", letzte Lesung bleibt, `HealthFor`.
- `internal/inventory/view.go` — `SetCertificateHealth`-Muster für die Clusterkarte; `ClusterView`.
- `internal/metrics/metrics.go` — Muster für Serien aus `ClusterView`.
- `web/src/components/NodeCertificates.tsx`, `CertificateBanner.tsx`, `web/src/routes/node-detail.tsx` — Anzeige-Muster.
- `.planning/quick/261012-hs1-*` und `internal/certwatch/` — wie der Zertifikatsablauf aufgebaut und rot gesehen wurde.
- `docs/api-contract.md`, `docs/guide.md`, `README.md` — Beschreibung der Funktion im selben Schritt.

</canonical_refs>

<code_context>
## Existing Code Insights

(Ergänzt 2026-10-14 nach der Recherche, siehe `15-RESEARCH.md`: `certwatch`-Fehler gefunden und separat behoben, Quick-Task 261014-gl2. Der neue Leser braucht ein eigenes Budget je Knoten (~20 s), und sein Wächter muss auf einem Kontext **ohne** Frist starten. Der Audit-Eintrag für HTTP-Routen trägt nie eine `machine_id`: `config.drift-confirm` braucht einen serverseitig aus dem Pfad gesetzten Parameter.)

### Reusable Assets
- `machineconfig.Redact`, `Compare`, `Service.read` — Lesen, Redigieren und Vergleichen sind vorhanden; neu ist nur die gespeicherte Basis.
- `certwatch.Service` (Deps-Funktionen, `Run`, `Tick`, `Refresh`, `HealthFor`, `Forget`-Verzicht) — das Gerüst für Takt, Teilausfall und „not read yet".
- Der Inventory-Setter (`SetCertificateHealth`) und die Metrik-Serien aus `ClusterView`.
- `fsstore` für atomare Dateien im Datenverzeichnis.

### Established Patterns
- Reads laufen über `inv.Connect`; der Aufrufer schließt den Client.
- Geheimnisse verlassen `internal/talos` und `internal/machineconfig` nur redigiert.
- Ein Wächter gilt erst, wenn er gegen den wieder eingesetzten Fehler rot gesehen ist; eine Injektion, die nichts injiziert, ist kein Ergebnis.

### Integration Points
- `cmd/holzkube-managerd/main.go` (Verdrahtung, wie bei `certWatch`), `ClusterView`, Knotenseite, `CertificateBanner`, Wand nicht (nicht gewählt).

</code_context>

<specifics>
## Specific Ideas

- Der Zertifikatsablauf (v0.6.0) ist das Vorbild für Aufbau, Ehrlichkeit („not read yet") und Tests; das Banner folgt derselben Stufenlogik, hier nur mit der Schwelle 24 h.

</specifics>

<deferred>
## Deferred Ideas

- Zurücksetzen auf die Basis (Schreibaktion auf den Cluster, mit Sudo, Bestätigung und Reboot-Frage) — eigene Phase.
- Vergleich gegen Schwesterknoten oder gegen eine aus Geheimnissen und Patches erzeugte Soll-Konfiguration.
- Vom Betreiber pflegbare Ausnahmen für Pfade.
- Wand-Kachel für Drift.

</deferred>

---

*Phase: 15-drift-erkennung-der-maschinenkonfiguration*
*Context gathered: 2026-10-14*
