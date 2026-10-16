# Milestones

Nachgezogen am 2026-09-28. Bis dahin lief die Arbeit seit v1.14 in eigenen
`MILESTONE-*.md`-Dateien statt in GSD-Phasen, und das GSD-Werkzeug hielt deshalb
Phase 3 von v1.14 für den nächsten Schritt. Diese Datei ist die Liste, die es
liest; der Inhalt jedes Milestones steht weiter in seiner eigenen Datei.

**Namen von Milestones sind keine Release-Versionen.** Die Releases hießen bis
2026-09-26 v1.16.x bis v1.32.0 und fangen seitdem bei v0.0.1 neu an
(`.claude/skills/release/SKILL.md`). Ein Milestone wird deshalb nicht getaggt.

## v1.14 Talos-Verwaltung (gebaut 2026-08-27 bis 2026-09-11)

Zehn Phasen, 37 Pläne in den Phasen 1–2, die Phasen 3–10 als Summary je Phase.
Alles gegen `internal/talossim` ausgeführt, keine Zeile auf echter Hardware.

- Archiv: `milestones/v1.14-ROADMAP.md`, `milestones/v1.14-REQUIREMENTS.md`,
  `milestones/v1.14-phases/`
- `REQUIREMENTS.md` bleibt zusätzlich an seinem Ort, weil
  `internal/planning/requirements_test.go` die Traceability-Tabelle dort liest.

### Known Gaps

- **OPS-05 🚫** — Durchlauf auf echter amd64-Hardware (Ledger 87). Hängt mit 82
  (Provisioning-Abnahme), 84 (MachineConfig gegen echtes Talos) und 85
  (Upgrade) zusammen: ein Durchlauf schließt alle vier.
- Phase 4 Walking Skeleton: Instrument gebaut, Messung nie gelaufen (80, 81).

## v1.15 Betrieb (2026-09-12 bis 2026-09-13)

Support-Bundle-Export, COSI-Watches statt Heartbeat, Prometheus-`/metrics`.
Datei: `MILESTONE-v1.15.md`, Abschluss `MILESTONE-v1.15-CLOSEOUT.md`, Phasen
unter `milestones/v1.15-phases/`.

## v1.16 Omni-Parität (2026-09-13 bis 2026-09-17)

Abschnitt B gebaut und bis v1.16.11 ausgeliefert. Abschnitt C (PXE, ARM64/SBC,
SideroLink, KMS) gebaut, mangels Hardware unbelegt. Datei: `MILESTONE-v1.16.md`.

### Known Gaps

- Offene Entscheidungen des Betreibers: Cloud-/Infrastruktur-Provider, SAML.
- CA-Rotation auf echter Hardware unbelegt (103, 143).

## v1.17 Kubernetes und Pods (2026-09-19)

client-go, Lesen, Node- und Pod-Aktionen, Manifeste, Workload-Proxy — alle
sechs Scheiben gebaut. Datei: `MILESTONE-v1.17.md`.

### Known Gaps

- Manifeste (148) und Workload-Proxy (151) nur gegen `internal/kubesim`
  bewiesen, nie gegen einen echten Cluster.

## Ohne Milestone-Datei (2026-09-20 bis 2026-09-27) → Release v0.1.0

Direkt auf Anweisung gebaut, ohne Planungsdokument: die Wand fürs IT-Büro,
Live-Hardware und Sensoren pro Knoten, Verlaufsdiagramme (1 h / 6 h / 24 h,
über Neustarts), Apps-Übersicht, ein Power-Modell für Cluster, Knoten und App
(inkl. Wake-on-LAN), das Kubernetes-Upgrade in talosctl-Reihenfolge (Ledger 86,
173), die neue Marke, und der Neustart der Versionsnummern bei v0.0.1.

### Known Gaps

- Live-Sensoren, Wake-on-LAN und erzwungene Talos-Power-Aktionen sind auf
  echter Hardware noch nicht geübt (Release-Notiz v0.0.1).

## v1.18 Host & Telefon (angelegt 2026-09-28, gebaut bis 2026-10-02) → Releases v0.2.0 bis v0.2.2

Phasen 11–14, 39 Pläne, alle ausgeführt. Die Host-Seite (Gerät, Dienst,
Live-Werte, Verlauf, Wand), fünf Host-Aktionen über einen root-eigenen Helfer
und die Tippziel-Prüfung bei 390 px. Die stündliche Update-Unit liegt jetzt in
`deploy/`. Phase 13 und 14 stehen auf `human_needed`
(`13-VERIFICATION.md`, `14-VERIFICATION.md`); der Milestone ist **nicht
archiviert**, weil die Handproben fehlen.

### Known Gaps

- Auf dem Pi nie gelaufen: Helfer installieren, Neustart mit zwei Sitzungen,
  Dienst-Neustart und Herunterfahren je einmal, die ausgelieferten Update-Units
  unter ihrer Sandbox.
- Telefon-Handprobe (D-11) nicht durchgeführt; D-08 für `/host` nach 13-12
  nicht wiederholt. MOB-03 bleibt „partial".

## Ohne Milestone-Datei (2026-10-03 bis 2026-10-09) → Releases v0.3.0 bis v0.5.0

Direkt auf Anweisung gebaut: das Redesign 2026 (v0.3.0/v0.3.1, Mockup
`docs/mockups/webui-2026.html`), Talos-machinery v1.14.2, Talos-Update-Suche und
frischer etcd-Snapshot vor jedem Talos-Upgrade (v0.4.0), bessere
Raumnutzung (Desktop, Tablet, Telefon), die Tiefenanalyse
(`AUDIT-2026-10-08.md`) mit ihren Sicherheits- und Update-Skript-Fixes, das
Entfernen eines Knotens als dauerhafter Job und geplante etcd-Backups mit
Aufbewahrung, Download und Überfällig-Markierung (v0.5.0, 2026-10-09).

### Known Gaps

- Backups liegen nur auf dem Pi; ein Export-Ziel (SFTP/S3) ist eine Entscheidung
  des Betreibers.
- Release-Signatur (Empfehlung minisign) und Alerting-Kanal: offen, siehe Audit.
- Nicht gelaufen am 2026-10-08: `./bin/task ci`, Browser- und Layout-Tests.

## Ohne Milestone-Datei (2026-10-12) → Releases v0.6.0 und v0.7.0

Direkt auf Anweisung gebaut: Zertifikatsablauf im Cluster (v0.6.0, Quick-Task
261012-hs1, nachträglich erfasst) und die Kubernetes-Upgradekette mit dem neuesten
Patch je Minor (Phase 16, Anforderung K8S-01, v0.7.0), dazu die korrigierte
compat-Matrix für Talos 1.12 bis 1.14 (Quick-Task 261012-l9s) und die fünf
gebündelten Dependabot-Bumps (261012-hxm).

### Known Gaps

- **Phase 16 `human_needed`:** das Format von `dl.k8s.io/release/stable-1.N.txt`
  ist einmal gemessen und hat keine Live-Prüfung (Ledger 175); ein echter Cluster
  hat noch keinen Zwischenpatch daraus angenommen. Die Kette bleibt ein Hinweis,
  nicht erzwungen (Ledger 174).
- Zertifikatsablauf nie gegen einen echten Talos-Cluster gelaufen.
- Matrixzeilen für Talos 1.7 bis 1.11 nicht nachgeprüft.

## Ohne Milestone-Datei (2026-10-14 bis 2026-10-15) → Releases v0.7.1 und v0.8.0

Direkt auf Anweisung gebaut: die Frist-Korrektur am Zertifikatsablauf (v0.7.1,
Quick-Task 261014-gl2; v0.6.0 und v0.7.0 lasen in Produktion nichts, weil der Leser
ohne Frist startete) und die Drift-Erkennung der Maschinenkonfiguration (Phase 15,
v0.8.0): Basis je Knoten im Datenverzeichnis (Schema 7), periodischer Leser,
Knotenseite mit Diff, Clusterzeile, Metriken, Banner nach 24 h, Bestätigen. Elf Pläne
(15-11 Lückenschluss), unabhängig verifiziert, `human_needed`.

### Known Gaps

- **Phase 15 `human_needed`:** ein echter Knoten mit Apply und Neustart (A1, A2 und
  das Timing, Ledger 177), Aussehen und Klickpfad im Browser auf dem Pi, das Banner
  nach echten 24 Stunden. DRIFT-04, 05 und 07 stehen bis dahin auf Pending.
- Eine geänderte Geheimnisfarbe allein ist unsichtbar (Ledger 178); die nicht
  redigierten Rohfelder liegen in `config-baselines/` (Ledger 179).
- Der Zertifikatsablauf ist erst mit v0.7.1 überhaupt lauffähig und nie an einem
  echten Cluster gesehen.
