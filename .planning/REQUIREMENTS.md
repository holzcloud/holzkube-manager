# Requirements: holzkube — Milestone v1.18 Host & Telefon

**Defined:** 2026-09-28
**Core Value:** Eine neue Maschine wird komplett in der UI zum Cluster-Node — ohne `talosctl`, ohne Omni.

Die Anforderungen von v1.14 mit ihren sechzehn Release-Blockern stehen in
`milestones/v1.14-REQUIREMENTS.md`; OPS-05 bleibt dort offen.

## Entscheidungen

Der Betreiber hat diesen Milestone mit „ohne mich zu nerven bis zum ende"
beauftragt. Was sonst eine Rückfrage wäre, ist hier entschieden und begründet,
damit er es am Ende auf einen Blick prüfen und umwerfen kann.

- **Der Daemon bekommt kein Root.** Die Produktions-Unit läuft als eigener
  Benutzer ohne Capabilities, mit `NoNewPrivileges`, ohne `AF_UNIX` (also ohne
  D-Bus) und mit `ProcSubset=pid`. Diese Härtung bleibt. Aktionen am Host
  laufen über einen **Auftrag im Datenverzeichnis**, den eine root-eigene
  systemd-Path-Unit abholt und mit einem festen Skript ausführt — dasselbe
  Muster wie `holzkube-manager-update`. Der Daemon kann damit genau die
  Aktionen auslösen, die das Skript kennt, und sonst nichts.
- **Monitoring liest `/proc` und `/sys` direkt.** Unter `ProcSubset=pid` sind
  `/proc/stat`, `/proc/meminfo` und `/proc/loadavg` unsichtbar. Die Oberfläche
  sagt dann „nicht lesbar" und nennt die eine Zeile in der Unit, die es ändert
  (`ProcSubset=all`), statt Nullen zu zeigen. `ProtectProc=invisible` bleibt:
  fremde Prozesse bleiben unsichtbar.
- **Der Verlauf nutzt `internal/history`**, dieselben Bereiche (1 h / 6 h /
  24 h) und dieselben Diagramme wie ein Knoten. Keine neue Pipeline.
- **Host-Aktionen sind zerstörend im Sinne von D-06:** Sudo-Fenster,
  getippte Bestätigung (Hostname), Audit-Eintrag, Rolle mindestens Operator.
- **Deployment-Dateien werden geliefert, nicht installiert.** Die neuen Units
  und das Skript liegen in `deploy/`; sie auf dem Pi zu installieren und den
  Dienst neu zu starten ist nach `CLAUDE.md` Sache des Betreibers.
- **Telefon: Tabellen bleiben wischbar** (Entscheidung des Betreibers vom
  2026-09-17, V2-UI-01); dieser Milestone ändert nur Tippzielgrößen.

## v1.18 Requirements

### Host-Übersicht (HOST)

- [x] **HOST-01**: Der Betreiber sieht auf einer eigenen Seite das Gerät, auf dem holzkube-manager läuft: Hostname, Modell, Architektur, Betriebssystem, Kernel, Laufzeit seit dem Boot
- [ ] **HOST-02**: Der Betreiber sieht den Zustand des Dienstes selbst: laufende Version, Laufzeit des Prozesses, Größe und freier Platz des Datenverzeichnisses
- [ ] **HOST-03**: Der Betreiber sieht, ob und wann zuletzt nach Updates gesucht wurde und welche Version verfügbar ist, soweit der Update-Mechanismus es hinterlegt
- [ ] **HOST-04**: Der Host erscheint in der Navigation und auf der Wand wie ein Knoten, mit einem Zustand (gesund / Warnung / nicht lesbar)

### Host-Monitoring (HMON)

- [ ] **HMON-01**: Der Betreiber sieht live CPU-Auslastung, Load, Arbeitsspeicher und Swap des Hosts
- [ ] **HMON-02**: Der Betreiber sieht Belegung der Datenträger (mindestens das Dateisystem des Datenverzeichnisses und `/`)
- [ ] **HMON-03**: Der Betreiber sieht die Temperaturen des Hosts (thermal zones / hwmon) und, wo vorhanden, Lüfter
- [ ] **HMON-04**: Der Betreiber sieht den Durchsatz der Netzwerkschnittstellen
- [ ] **HMON-05**: Zu CPU, Speicher, Temperatur und Netzwerk gibt es Verlaufsdiagramme über 1 h / 6 h / 24 h, die einen Neustart des Daemons überleben
- [ ] **HMON-06**: Ein Wert, der wegen der Härtung der Unit nicht lesbar ist, wird als „nicht lesbar" mit der Ursache gezeigt, nie als 0
- [ ] **HMON-07**: Überschreitet Temperatur oder Datenträgerbelegung eine Schwelle, zeigt der Host eine Warnung mit dem Grund

### Host-Aktionen (HACT)

- [ ] **HACT-01**: Der Betreiber kann den Host neu starten
- [ ] **HACT-02**: Der Betreiber kann den Host herunterfahren
- [ ] **HACT-03**: Der Betreiber kann den Dienst holzkube-manager neu starten
- [ ] **HACT-04**: Der Betreiber kann „jetzt nach Updates suchen" auslösen
- [ ] **HACT-05**: Jede Host-Aktion verlangt Sudo-Fenster und getippten Hostnamen, läuft durch den Audit-Pfad und ist nur ab Rolle Operator erlaubt
- [ ] **HACT-06**: Der Daemon führt eine Host-Aktion nie selbst aus: er legt einen Auftrag ab, den ein root-eigener Helfer aus einer festen Liste ausführt; ein unbekannter Auftrag wird verworfen und protokolliert
- [ ] **HACT-07**: Ist der Helfer nicht installiert, sagt die Oberfläche das und nennt, was zu installieren ist, statt einen Auftrag abzulegen, den niemand abholt
- [ ] **HACT-08**: `deploy/` enthält die Units, das Skript und eine Anleitung zur Installation des Helfers

### Telefon (MOB)

- [ ] **MOB-01**: Bei 390 px Breite hat jedes Bedienelement eine Tippfläche von mindestens 44 × 44 px (unterhalb `md`; Desktop unverändert)
- [ ] **MOB-02**: Ein Wächter in der Layout-Prüfung misst die Tippzielgrößen auf allen Routen und wird rot, sobald ein Element darunter fällt
- [ ] **MOB-03**: Die neue Host-Seite erfüllt MOB-01 und ist auf dem Telefon bedienbar

## Future Requirements

- Host-Aktion „Rollback auf die vorige Version" (`holzkube-manager-update --rollback`)
- Mehr als ein Host (ein zweiter Manager, ein Standby)

## Out of Scope

- **Beliebige Befehle auf dem Host** — der Helfer kennt eine feste Liste; eine Shell im Browser wäre ein Root-Zugang über HTTP
- **Paket-Updates des Betriebssystems (apt)** — Sache des Betriebssystems, nicht dieses Produkts
- **Tabellen als Karten auf dem Telefon** — vom Betreiber am 2026-09-17 verworfen

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| HOST-01 | Phase 11 | Complete |
| HOST-02 | Phase 11 | Pending |
| HOST-03 | Phase 11 | Pending |
| HOST-04 | Phase 12 | Pending |
| HMON-01 | Phase 11 | Pending |
| HMON-02 | Phase 11 | Pending |
| HMON-03 | Phase 11 | Pending |
| HMON-04 | Phase 11 | Pending |
| HMON-05 | Phase 12 | Pending |
| HMON-06 | Phase 11 | Pending |
| HMON-07 | Phase 12 | Pending |
| HACT-01 | Phase 13 | Pending |
| HACT-02 | Phase 13 | Pending |
| HACT-03 | Phase 13 | Pending |
| HACT-04 | Phase 13 | Pending |
| HACT-05 | Phase 13 | Pending |
| HACT-06 | Phase 13 | Pending |
| HACT-07 | Phase 13 | Pending |
| HACT-08 | Phase 13 | Pending |
| MOB-01 | Phase 14 | Pending |
| MOB-02 | Phase 14 | Pending |
| MOB-03 | Phase 14 | Pending |

**Coverage:** 22 von 22 v1.18-Anforderungen einer Phase zugeordnet, keine doppelt.

---
*Traceability gefüllt am 2026-09-28 mit der Roadmap für v1.18 (Phasen 11–14).*
