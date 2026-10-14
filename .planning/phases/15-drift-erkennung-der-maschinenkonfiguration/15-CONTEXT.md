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
