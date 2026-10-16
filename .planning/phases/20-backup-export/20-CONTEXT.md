# Phase 20: Backup-Export - Context

**Gathered:** 2026-10-16
**Status:** Ready for planning

<domain>
## Phase Boundary

Die etcd-Backups überleben den Verlust des Pi. Heute liegen die geplanten Snapshots (v0.5.0) nur auf dem Gerät und gehen mit seiner SD-Karte verloren. Geliefert wird: Export jedes erfolgreichen Snapshots an ein Ziel außerhalb des Geräts, mit Prüfsumme, sichtbarem Ergebnis und einem Überfällig-Zustand für den Export. In **dieser** Phase nur ein Ziel (S3-kompatibel); SFTP folgt später als eigene Phase über dieselbe Austauschnaht.

</domain>

<decisions>
## Implementation Decisions

Alle als Auswahl vom Betreiber am 2026-10-16 getroffen, sofern nicht als Ermessen von Claude gekennzeichnet.

- **D-01:** Das erste Ziel ist **S3-kompatibel** (Bucket mit Zugangsschlüssel, z. B. MinIO im LAN oder ein Cloud-Anbieter). **SFTP folgt später** als eigene Phase; diese Phase baut eine **Austauschnaht** (Schnittstelle „Ziel"), damit das zweite Ziel wenig kostet. — **Reversibility:** costly — Schnittstelle und Einstellungsformat sind veröffentlicht.
- **D-02:** Der Zugangsschlüssel ist ein **Geheimnis** im Datenverzeichnis (nie im Audit-Klartext, nie in Logs, nie in einer Antwort).

### Claude's Discretion
Eigener schmaler S3-Client oder eine Bibliothek (die Abhängigkeit ist zu begründen und gegen die Größe des Releases auf dem Pi abzuwägen), Signaturverfahren, Pfadstil und Region, was bei Fehlschlag passiert (Wiederholung mit Grenze, Zustand „Export fehlgeschlagen" sichtbar, nie still), Aufbewahrung am Ziel (nicht löschen, oder die Aufbewahrungszahl des lokalen Zeitplans spiegeln — eine echte Alternative: dann als Auswahl zurück an den Betreiber), Verschlüsselung der Übertragung (TLS, Zertifikatsprüfung), die Einstellungsseite, ein Test-Knopf („Testobjekt schreiben"). Die Beweisgrenze: nur gegen einen eigenen Fake-Server (`httptest`), kein echter Bucket.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

- `CLAUDE.md` — öffentliches Repository, README im selben Schritt, Changelog erst beim Release, jeder Wächter rot gesehen, Fragen als Auswahl, Linter lokal: `GOTOOLCHAIN=go1.26.9 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1 run ./...`.
- `.planning/AUDIT-2026-10-08.md` — Quelle des Kandidaten.
- `internal/upgrade/backups.go` (`BackupScheduler`, Snapshots, Prüfsumme), `internal/store/fsstore` (Dateien, atomare Schreibvorgänge), `internal/jobs` (Job-Muster), `internal/inventory` (`SetBackupHealth`), `internal/metrics` (Backup-Serien), `internal/audit` (Allowlist), `docs/api-contract.md`, `docs/guide.md` (Abschnitt Backups), `README.md`.
- Muster für Aufbau und Beweis: `internal/certwatch` und `internal/configdrift` (periodischer Leser, „not read yet", Wächter vom Kontext **ohne Frist** aus — der Talos-Client verweigert Aufrufe ohne Frist, siehe Quick-Task 261014-gl2), die Phasen 15 und 16 unter `.planning/phases/`.

</canonical_refs>

<deferred>
## Deferred Ideas

- SFTP-Ziel (eigene spätere Phase, bewusst als zweites).
- Wiederherstellung aus dem Export (Download vom Ziel in die Oberfläche).
- Verschlüsselung der Snapshots vor dem Export.

</deferred>

---

*Phase: 20-backup-export*
*Context gathered: 2026-10-16*
