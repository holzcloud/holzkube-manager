# Roadmap: holzkube-manager

Seit 2026-09-28 in der GSD-Form nach einem Milestone-Abschluss. Die zehn Phasen
von v1.14 mit allen Details stehen unverändert in
`milestones/v1.14-ROADMAP.md`; was danach kam, in `MILESTONES.md`.

v1.18 setzt die Phasennummern von v1.14 fort und beginnt bei Phase 11. v1.15
bis v1.17 liefen ohne GSD-Nummern (v1.15 als `v1.15-01` bis `v1.15-03`), es
gibt also nichts, womit 11 zusammenstoßen könnte.

## Milestones

- ✅ **v1.14 Talos-Verwaltung** — Phasen 1–10 (gebaut 2026-09-11, OPS-05 offen)
- ✅ **v1.15 Betrieb** — 3 Phasen (2026-09-13)
- ✅ **v1.16 Omni-Parität** — Abschnitt B (2026-09-17)
- ✅ **v1.17 Kubernetes und Pods** — 6 Scheiben (2026-09-19)
- ✅ **ohne Milestone** — Wand, Hardware, Verlauf, Apps, Power → Release v0.1.0 (2026-09-27)
- 🚧 **v1.18 Host & Telefon** — Phasen 11–14 (angelegt 2026-09-28, in Arbeit)

## Phases

<details>
<summary>✅ v1.14 Talos-Verwaltung (Phasen 1–10)</summary>

- [x] Phase 1: Foundation Skeleton (6/6 plans) — 2026-08-28
- [x] Phase 2: Transport Seam, `talossim` & Image Factory (31/31 plans) — 2026-09-05
- [x] Phase 3: Inventar, Cluster-Import & Health — 2026-09-06
- [~] Phase 4: Walking Skeleton — Instrument gebaut, Messung offen (80, 81)
- [x] Phase 5: Streaming — 2026-09-08
- [x] Phase 6: Jobs-Engine & Node-Aktionen — 2026-09-09
- [x] Phase 7: Config-Domain — 2026-09-10
- [~] Phase 8: Provisioning — Abnahme offen (82, 84)
- [x] Phase 9: Upgrades & etcd-Verwaltung — nie auf Hardware (85)
- [~] Phase 10: Härtung & Hardware-Durchlauf — OPS-05 offen (87)

</details>

<details>
<summary>✅ v1.15 Betrieb</summary>

- [x] Support-Bundle-Export (V2-OPS-01)
- [x] COSI-Watches statt Heartbeat (INV-13)
- [x] Prometheus-`/metrics` (V2-API-02)

</details>

### 🚧 v1.18 Host & Telefon (in Arbeit)

**Milestone-Ziel:** Das Gerät, auf dem holzkube-manager selbst läuft, ist in der
Oberfläche sichtbar und bedienbar wie ein Knoten; und die Oberfläche ist auf
einem Telefon ohne zu kleine Tippziele benutzbar, gehalten von einem Wächter.

Die Reihenfolge folgt der Anweisung „erst manager-pi, dann das telefon-ui" und
einer Abhängigkeit: Phase 14 misst die Host-Seite erst, wenn sie ihre Knöpfe
und Dialoge aus Phase 13 hat. Jede Phase ist eine senkrechte Scheibe — Lesen,
API und Seite zusammen —, damit am Ende jeder Phase etwas im Browser steht, das
der Betreiber prüfen kann, statt einer API, die niemand ansieht.

- [x] **Phase 11: Host-Seite — Gerät, Dienst, Live-Werte** - Der Betreiber öffnet `/host` und sieht das Gerät, den Dienst und dessen Live-Werte; was die Härtung verbirgt, heißt „nicht lesbar", nie 0 (completed 2026-09-28)
- [x] **Phase 12: Host wie ein Knoten — Verlauf, Warnung, Wand** - Der Host hat Verlauf über 1 h / 6 h / 24 h, eine Warnung mit Grund und steht in Navigation und Wand mit einem Zustand (completed 2026-09-29)
- [ ] **Phase 13: Host-Aktionen über einen root-eigenen Helfer** - Neustart, Herunterfahren, Dienst-Neustart, Update-Suche — ohne dass der Daemon Root bekommt
- [ ] **Phase 14: Telefon — Tippziele, die ein Daumen trifft** - Jedes Bedienelement ≥ 44 px bei 390 px, auch hinter einem Tipp und auf `/host`, gehalten von der Layout-Prüfung

**Wo geprüft wird.** Sitzungen laufen auf dem Pi des Betreibers (aarch64); arm64 wird dort nativ ausgeführt, und ein Bericht nennt das.
Der Produktions-Daemon daneben wird für keine dieser Phasen ersetzt oder neu
gestartet, und der Helfer aus Phase 13 wird nicht installiert — beides ist nach
`CLAUDE.md` Sache des Betreibers. Geprüft wird gegen einen eigens gestarteten
Daemon mit eigenem Datenverzeichnis. `go test -race` gibt es auf dem Pi nicht;
das Race-Urteil kommt aus CI.

**Methode.** Jeder Wächter, den eine Phase als Beleg nennt, ist gegen den
wieder eingesetzten Fehler rot gesehen worden, mit dem Exit-Code vom Befehl
selbst gelesen. Ein Wächter, der nach einer Injektion grün bleibt, die nichts
injiziert hat, ist kein Ergebnis.

## Phase Details

### Phase 11: Host-Seite — Gerät, Dienst, Live-Werte

**Goal**: Der Betreiber öffnet `/host` und sieht das Gerät, auf dem holzkube-manager läuft, den Dienst selbst und dessen Live-Werte — CPU, Speicher, Datenträger, Temperatur, Netzwerk. Wo die Härtung der Unit einen Wert verbirgt, steht „nicht lesbar" mit der Ursache, nie eine 0.
**Depends on**: Nothing (erste Phase von v1.18; baut auf dem ausgelieferten v0.1.0 auf)
**Requirements**: HOST-01, HOST-02, HOST-03, HMON-01, HMON-02, HMON-03, HMON-04, HMON-06
**Success Criteria** (what must be TRUE):

  1. Auf dem Pi zeigt `/host` Hostname, Modell, Architektur, Betriebssystem, Kernel und Laufzeit seit dem Boot, und dieselben Werte nennt die Shell auf derselben Maschine; daneben die laufende Version des Dienstes, die Laufzeit des Prozesses sowie Größe und freien Platz des Datenverzeichnisses.
  2. Die Seite zeigt live, ohne Neuladen: CPU-Auslastung, Load, Arbeitsspeicher und Swap; die Belegung von `/` und des Dateisystems, auf dem das Datenverzeichnis liegt; die Temperaturen aus thermal zones und hwmon und, wo vorhanden, die Lüfterdrehzahl; den Durchsatz je Netzwerkschnittstelle. Auf dem Pi stimmen die Werte mit dem überein, was `/proc` und `/sys` in der Shell zeigen.
  3. Unter der Härtung der Produktions-Unit (`ProcSubset=pid`) stehen CPU und Speicher als „nicht lesbar" da (Load kommt dann aus `sysinfo(2)` und bleibt exakt), mit der Ursache und der einen Zeile, die es ändert (`ProcSubset=all`) — nie als 0. Der Test, der das hält, ist gegen eine wieder eingesetzte 0 rot gesehen worden.
  4. Hinterlegt der Update-Mechanismus, wann zuletzt gesucht wurde und welche Version verfügbar ist, zeigt die Seite beides; hinterlegt er nichts — der Stand der heute auf dem Pi installierten `holzkube-manager-update` —, sagt sie „nicht hinterlegt", statt eine Zeit oder eine Version zu erfinden. Was das Skript dafür künftig schreibt, liegt in `deploy/` und wird nicht installiert.

**Research**: `/proc` unter `ProcSubset=pid` und `ProtectProc=invisible` — welche Dateien genau verschwinden und woran der Daemon das von einem echten Lesefehler unterscheidet; wo der Pi 5 Temperatur und Lüfter in `/sys` ablegt und wo ein amd64-Host.
**Plans**: 7/7 plans complete

Plans:
**Wave 1**
- [x] 11-01-PLAN.md — Tracer: `internal/host` + `GET /api/v1/host` + `/host` mit Gerät-Karte, Container-Hinweis, Navigation, Layout-Fixture (HOST-01)

**Wave 2** *(blocked on Wave 1 completion)*
- [x] 11-02-PLAN.md — Update-Skript hinterlegt sein Ergebnis über einen EXIT-Trap; strenger Leser `internal/host/updatestatus` (HOST-03)
- [x] 11-03-PLAN.md — CPU, Load, Speicher, Swap live; Härtung `subset=pid` deterministisch erkannt, „Not readable" statt 0; Echt-Kernel-Test (HMON-01, HMON-06)

**Wave 3** *(blocked on Wave 2 completion)*
- [x] 11-04-PLAN.md — Dienst-Karte (Version, Laufzeit, Datenverzeichnis, Update-Status) und Dateisysteme, nach Gerätenummer zusammengelegt (HOST-02, HMON-02, HOST-03)

**Wave 4** *(blocked on Wave 3 completion)*
- [x] 11-05-PLAN.md — Sensoren: hwmon/thermal über die Talos-Regeln, gemeinsames Sensor-Modul mit NodeHardware (HMON-03)

**Wave 5** *(blocked on Wave 4 completion)*
- [x] 11-06-PLAN.md — Netzwerk: physische Schnittstellen mit Durchsatz, virtuelle eingeklappt (HMON-04)

**Wave 6** *(blocked on Wave 5 completion)*
- [x] 11-07-PLAN.md — Vertrag und Anleitung, 390-px-Browsermessung, Abgleich auf dem Pi gegen die Shell und `subset=pid`, `./bin/task ci`

**Cross-cutting constraints:**
- Long host identifiers wrap and never widen the page at 390 px

Waves: 1 → {01} · 2 → {02, 03} · 3 → {04} · 4 → {05} · 5 → {06} · 6 → {07}
**UI hint**: yes

### Phase 12: Host wie ein Knoten — Verlauf, Warnung, Wand

**Goal**: Der Host steht neben den Knoten wie einer von ihnen: mit Verlaufsdiagrammen über 1 h / 6 h / 24 h aus `internal/history`, mit einer Warnung, die ihren Grund nennt, und mit einem Eintrag in der Navigation und einer Kachel auf der Wand.
**Depends on**: Phase 11
**Requirements**: HMON-05, HMON-07, HOST-04
**Success Criteria** (what must be TRUE):

  1. Zu CPU, Speicher, Temperatur und Netzwerk zeigt `/host` Verlaufsdiagramme über 1 h / 6 h / 24 h — dieselben Bereiche und dieselben Diagramme wie ein Knoten, geschrieben in `internal/history`, ohne neue Pipeline.
  2. Nach einem Neustart des Daemons ist der Verlauf bis zum Neustart noch da; eine Zeit, in der ein Wert nicht lesbar war, erscheint als Lücke, nicht als Nulllinie.
  3. Überschreitet die Temperatur oder die Belegung eines Datenträgers ihre Schwelle, zeigt der Host eine Warnung, die Wert und Schwelle nennt. Der Test dafür ist gegen eine entfernte Schwellenprüfung rot gesehen worden.
  4. Der Host steht in der Navigation und auf der Wand wie ein Knoten, mit genau einem von drei Zuständen — gesund, Warnung, nicht lesbar. Ein Host, dessen Werte nicht lesbar sind, erscheint dort nie als gesund, und der Test, der das hält, ist gegen einen Host rot gesehen worden, der es doch tat.

**Plans:** 8/8 plans complete

Plans:
**Wave 1**
- [x] 12-01-PLAN.md — Tracer: ein Host-Sensor über `Collector.Sample`, den einen Sampler (`host/local`), den Store und `GET /api/v1/host/history` bis zur Kurve auf `/host`; Wächter für Aufbewahrung, Inventar-Ausfall und Fehlerantworten (HMON-05)
- [x] 12-02-PLAN.md — Temperaturgrenzen als eine Go-Regel (`warn_c`/`danger_c` für Knoten und Host), Trip-Points des Pi (passive/hot/critical, nie active), `host.Assess` mit Sätzen und „nie gesund, wenn nicht lesbar" (HMON-07, HOST-04)

**Wave 2** *(blocked on Wave 1 completion)*
- [x] 12-03-PLAN.md — Alle Werte im Verlauf mit eigenem Raten-Fenster des Samplers, Neustart-mit-Lücke-Test, `health` auf `/api/v1/host`, `host` in der Antwort der Wand (veraltet → nicht lesbar) (HMON-05, HOST-04, HMON-07)
- [x] 12-04-PLAN.md — Der Browser zeichnet die Grenzen des Servers; seine eigene Regel entfällt; Laufwerks-Temperatur und reduzierte Bewegung (HMON-07)

**Wave 3** *(blocked on Wave 2 completion)*
- [x] 12-05-PLAN.md — Geteilte Diagrammblöcke aus `NodeHardware`, Verlauf auf `/host` über 1 h / 6 h / 24 h, „Not readable — no history", Texte; Checker-Auflagen 1, 2, 6 (HMON-05)

**Wave 4** *(blocked on Wave 3 completion)*
- [x] 12-06-PLAN.md — Zustandszeile und Warnhinweis auf `/host`, Zustandsmarke in der Navigation, Host-Kachel als erste im Abschnitt „Nodes" der Wand (HOST-04, HMON-07)

**Wave 5** *(blocked on Wave 4 completion)*
- [x] 12-07-PLAN.md — 12-px-Wortabstand gemessen und behoben, Verlaufs-Fixture für Layout-Prüfung und Bilder, README mit neuem `host.png` und `wall.png` (HMON-05, HMON-07, HOST-04)

**Wave 6** *(blocked on Wave 5 completion)*
- [x] 12-08-PLAN.md — Vertrag und Anleitung; auf dem Pi: Verlauf über einen Neustart mit Lücke, nichts aufgezeichnet unter `subset=pid`, `./bin/task ci` (HMON-05, HMON-07, HOST-04)

**Cross-cutting constraints:**
- The warning list at 390 px wraps a long mount path instead of widening the page
- The wall's host tile with a 26-character name fits at 390 px
- A long warning sentence truncates on the wall tile and is complete on /host

Waves: 1 → {01, 02} · 2 → {03, 04} · 3 → {05} · 4 → {06} · 5 → {07} · 6 → {08}
**UI hint**: yes

### Phase 13: Host-Aktionen über einen root-eigenen Helfer

**Goal**: Der Betreiber kann den Host neu starten, herunterfahren, den Dienst holzkube-manager neu starten und „jetzt nach Updates suchen" auslösen — ohne dass der Daemon Root, D-Bus oder eine Capability bekommt. Er legt einen Auftrag im Datenverzeichnis ab; eine root-eigene Path-Unit holt ihn ab und ein festes Skript führt ihn aus einer festen Liste aus, nach dem Muster von `holzkube-manager-update`.
**Depends on**: Phase 11 (die Seite, auf der die Aktionen stehen, und der Hostname für die getippte Bestätigung)
**Requirements**: HACT-01, HACT-02, HACT-03, HACT-04, HACT-05, HACT-06, HACT-07, HACT-08
**Success Criteria** (what must be TRUE):

  1. Auf `/host` stehen vier Aktionen: Host neu starten, Host herunterfahren, Dienst neu starten, jetzt nach Updates suchen. Jede verlangt ein Sudo-Fenster und den getippten Hostnamen, wird unterhalb der Rolle Operator abgewiesen und steht im Audit-Log. Jede dieser vier Sperren ist einzeln entfernt und ihr Test dabei rot gesehen worden.
  2. Der Daemon führt eine Host-Aktion nie selbst aus: kein `systemctl`, kein `reboot`, kein Subprozess — er schreibt einen Auftrag ins Datenverzeichnis. Ein Wächter hält das und ist gegen einen eingeschmuggelten Prozessaufruf rot gesehen worden.
  3. Das Helfer-Skript, gegen ein Test-Datenverzeichnis und einen Stellvertreter für `systemctl` gefahren, führt genau die vier bekannten Aufträge aus. Ein unbekannter oder verformter Auftrag — ein fremdes Wort, ein Pfad, Shell-Metazeichen — wird verworfen und protokolliert, und der Test ist gegen ein Skript rot gesehen worden, das ihn doch ausführt.
  4. Ist der Helfer nicht installiert — auf dem Pi der Stand, bis der Betreiber ihn installiert —, sind die Aktionen gesperrt, die Seite nennt, was aus `deploy/` wohin gehört, und im Datenverzeichnis entsteht kein Auftrag, den niemand abholt.
  5. `deploy/` enthält Path-Unit, Service-Unit, Skript und eine Anleitung zur Installation; `systemd-analyze verify` nimmt die Units an, und die Anleitung verlangt keine Zeile weniger Härtung an der Unit des Daemons (`NoNewPrivileges`, kein `AF_UNIX`, `ProcSubset=pid` bleiben).

**Research**: Path-Units — `PathChanged` gegen `PathModified` gegen `DirectoryNotEmpty`, und wie ein Auftrag, den der Daemon per Rename atomar ablegt, genau einmal abgeholt wird; wie der Daemon ohne D-Bus erkennt, ob der Helfer installiert ist.
**Plans:** 6/9 plans executed
**UI hint**: yes

Plans:
**Wave 1**
- [x] 13-01-PLAN.md — Tracer: „Check for updates and install" vom Knopf über Bestätigung, Sudo, Audit, Auftragsdatei (`fsstore.PlaceNew`), Root-Skript unter `unshare` und strengen Ergebnisleser zurück auf die Seite; Tabelle der Tipp-Pflicht (HACT-04, HACT-05, HACT-06)
- [x] 13-02-PLAN.md — Aus Phase 12 übernommen: Thermal-Zone ohne lesbaren Typ nie „gesund", Satz der Wand mit der Zahl zuerst, ▲ in der Farbe seiner Schwere (HOST-04, HMON-07)

**Wave 2** *(blocked on Wave 1 completion)*
- [x] 13-03-PLAN.md — Die vier Sperren je Aktion einzeln rot gesehen: Sudo-Fenster, getippter Hostname, Rolle Operator, Audit-Eintrag; Tokens an `@host` gebunden (HACT-01..05)
- [x] 13-04-PLAN.md — Das Helfer-Skript als Matrix: vier Aufträge, alles andere verworfen ohne Inhalt im Journal, Symlink/FIFO/veraltet, Verbrauch vor dem Handeln; Leser und Schreiber ein Format (HACT-01..04, HACT-06)
- [x] 13-05-PLAN.md — Der Daemon schreibt eine Datei und sonst nichts: Wächter gegen jeden Prozessstart, Rückzug nach 10 s und beim Start, `fsstore.Claim` (HACT-06, HACT-07)
- [x] 13-06-PLAN.md — Helfer installiert? Drei Dateien ohne D-Bus, 409 ohne Auftrag bei fehlendem Helfer oder im Container, `actions` mit Fehlendem und Installationsbefehlen (HACT-07)

**Wave 3** *(blocked on Wave 2 completion)*
- [ ] 13-07-PLAN.md — `deploy/`: Path- und Service-Unit, `HOST-HELPER.md`, Release-Archiv; `systemd-analyze verify` nach Ausgabe geprüft, Anleitung = Seite, Härtung des Daemons unverändert, Update-Skript liefert den Helfer nie (HACT-08)
- [ ] 13-08-PLAN.md — Die Seite: vier Aktionen mit Dialog, Gründe für gesperrte Knöpfe, Auftragsstatus bis „zurück", Warte-Hinweis statt Fehlerseite, Helfer-Hinweis, 390 px gemessen (HACT-01..05, HACT-07)

**Wave 4** *(blocked on Wave 3 completion)*
- [ ] 13-09-PLAN.md — Fixture im Zustand des Pi, Vertrag, Anleitung, README mit neuem `host.png` und `wall.png`; `./bin/task ci` auf dem Pi, Produktionsdienst unberührt, nichts installiert (HACT-01..08)

**Cross-cutting constraints:**
- A 40-character hostname wraps inside the host action dialog at 390 px
- A /host page that did not place the reboot order shows the waiting notice, not the stale notice
- Nothing is installed on the operator's host and the production service is never restarted; the helper ships in `deploy/` only

Waves: 1 → {01, 02} · 2 → {03, 04, 05, 06} · 3 → {07, 08} · 4 → {09}

### Phase 14: Telefon — Tippziele, die ein Daumen trifft

**Goal**: Bei 390 px Breite trifft ein Daumen jedes Bedienelement — auf jeder Route, auch in dem, was erst nach einem Tipp erscheint, und auf der neuen Host-Seite —, und die Layout-Prüfung in `task ci` wird rot, sobald eines darunter fällt. Oberhalb `md` bleibt alles, wie es ist.
**Depends on**: Phase 13 (die Host-Seite hat ihre Aktionen und den Bestätigungsdialog erst dann)
**Requirements**: MOB-01, MOB-02, MOB-03
**Success Criteria** (what must be TRUE):

  1. Bei 390 px hat jedes Bedienelement auf jeder Route eine Tippfläche von mindestens 44 × 44 px; bei 1280 px ist die Oberfläche unverändert.
  2. Die Layout-Prüfung misst auch, was erst nach einem Tipp erscheint — die mobile Navigation, Menüs wie das Power-Menü, Bestätigungs- und Sudo-Dialog —, und eine Route, die im Router steht und in der Prüfung fehlt, macht die Prüfung rot.
  3. Die Prüfung ist rot gesehen worden, dreimal einzeln: gegen ein Element, das auf unter 44 px zurückgesetzt ist, gegen einen Dialogknopf unter 44 px und gegen eine Route, die aus ihrer Liste genommen ist — jedes Mal endet `task test:layout` mit einem Exit-Code ungleich 0, gelesen vom Befehl selbst.
  4. `/host` mit seinen Aktionen und dem Bestätigungsdialog besteht die Prüfung bei 390 px, und auf einem Telefon lässt sich der Host ansehen und eine Aktion bis zur getippten Bestätigung führen, ohne zu zoomen.

**Ausgangslage**: `web/scripts/layout-audit.mjs` misst Tippziele unter 44 px bei 390 px seit e2bd690 (2026-09-18) und läuft in `task ci` mit; laut 400e1d4 bestanden am 2026-09-26 alle Routen. Die Zeile „kein Wächter für Tippzielgrößen" in `STATE.md` stammte aus der Übergabe vom 2026-09-17. Was der Wächter nicht misst: Er klickt nach dem Login nichts an, sieht also keinen Dialog, kein Menü und die mobile Navigation nicht, und seine Routen sind eine Hand-Liste. Das ist die Arbeit dieser Phase, neben `/host`.
**Plans**: TBD
**UI hint**: yes

## Progress

**Execution Order:** 11 → 12 → 13 → 14

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 11. Host-Seite — Gerät, Dienst, Live-Werte | v1.18 | 7/7 | Complete    | 2026-09-28 |
| 12. Host wie ein Knoten — Verlauf, Warnung, Wand | v1.18 | 8/8 | Complete    | 2026-09-29 |
| 13. Host-Aktionen über einen root-eigenen Helfer | v1.18 | 6/9 | In Progress|  |
| 14. Telefon — Tippziele, die ein Daumen trifft | v1.18 | 0/TBD | Not started | - |
