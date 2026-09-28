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
