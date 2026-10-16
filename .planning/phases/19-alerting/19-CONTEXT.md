# Phase 19: Alerting - Context

**Gathered:** 2026-10-16
**Status:** Ready for planning

<domain>
## Phase Boundary

Der Betreiber wird benachrichtigt, wenn etwas seine Aufmerksamkeit braucht. Heute gibt es nur `/metrics`, die Wand und Banner. Quellen sind die vorhandenen Zustände: Knoten nicht erreichbar, Platte, Temperatur, Snapshot überfällig, Zertifikat (Admin-Zertifikat und Zertifikate im Cluster), Konfigurationsdrift. Geliefert wird: ein Auswerter, der Übergänge erkennt (nicht jeden Takt), ein Kanal, Einstellungen und eine Anzeige, was zuletzt gemeldet wurde.

</domain>

<decisions>
## Implementation Decisions

Alle als Auswahl vom Betreiber am 2026-10-16 getroffen, sofern nicht als Ermessen von Claude gekennzeichnet.

- **D-01:** Der Kanal ist ein **Webhook**: eine vom Betreiber konfigurierte URL, an die ein JSON-Dokument geht. Damit lassen sich ntfy, Gotify, Matrix, Slack und eigene Skripte bedienen, ohne dass holzkube-manager einen dieser Dienste kennt. Kein SMTP, keine eingebaute ntfy-Anbindung. — **Reversibility:** costly — die Form des Dokuments ist eine veröffentlichte Schnittstelle für Empfänger.
- **D-02:** Die URL kann ein Geheimnis tragen (Token); sie wird wie ein Geheimnis behandelt (im Datenverzeichnis, nie im Audit-Klartext, nie in der Wand oder in Logs).

### Claude's Discretion
Welche Zustände wie auslösen (nach dem Muster: ein Übergang nach „braucht Aufmerksamkeit", ein Übergang zurück „wieder in Ordnung", keine Wiederholung jeden Takt), Mindestabstand und Zusammenfassen, Form des JSON-Dokuments (Schlüssel, Version), Wiederholung bei Fehlschlag mit Grenze, Zustand über Neustarts (was schon gemeldet wurde, damit ein Neustart nicht alles neu meldet), die Einstellungsseite und ob ein Test-Knopf („Testnachricht senden") dazugehört. Wo eine echte Alternative besteht, geht sie als Auswahl an den Betreiber zurück.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

- `CLAUDE.md` — öffentliches Repository, README im selben Schritt, Changelog erst beim Release, jeder Wächter rot gesehen, Fragen als Auswahl, Linter lokal: `GOTOOLCHAIN=go1.26.9 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1 run ./...`.
- `.planning/AUDIT-2026-10-08.md` — Quelle des Kandidaten.
- `internal/health`, `internal/metrics`, `internal/certwatch`, `internal/configdrift`, `internal/upgrade` (Backup-Zustand), `internal/inventory/view.go` (`ClusterView`), `internal/host` (Temperatur, Platte), `internal/config` und `internal/httpapi/handlers/settings` (Einstellungen), `internal/audit` (Allowlist), `docs/api-contract.md`, `docs/guide.md`, `README.md`.
- Muster für Aufbau und Beweis: `internal/certwatch` und `internal/configdrift` (periodischer Leser, „not read yet", Wächter vom Kontext **ohne Frist** aus — der Talos-Client verweigert Aufrufe ohne Frist, siehe Quick-Task 261014-gl2), die Phasen 15 und 16 unter `.planning/phases/`.

</canonical_refs>

<deferred>
## Deferred Ideas

- ntfy direkt, SMTP-Mail (bewusst nicht gewählt).
- Stille Zeiten, Eskalationsstufen, mehrere Empfänger.
- Alarmregeln pro Knoten.

</deferred>

---

*Phase: 19-alerting*
*Context gathered: 2026-10-16*
