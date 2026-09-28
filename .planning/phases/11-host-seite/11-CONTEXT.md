# Phase 11: Host-Seite — Gerät, Dienst, Live-Werte - Context

**Gathered:** 2026-09-28
**Status:** Ready for planning

<domain>
## Phase Boundary

Eine neue Seite `/host` zeigt das Gerät, auf dem holzkube-manager läuft
(Hostname, Modell, Architektur, Betriebssystem, Kernel, Laufzeit seit dem Boot),
den Dienst selbst (laufende Version, Laufzeit des Prozesses, Größe und freier
Platz des Datenverzeichnisses, Update-Status soweit hinterlegt) und die
Live-Werte des Hosts (CPU, Load, Speicher, Swap, Datenträger, Temperaturen,
Lüfter, Netzwerk-Durchsatz). Was die Härtung der Unit verbirgt, steht als
„nicht lesbar" mit Ursache da, nie als 0.

Requirements: HOST-01, HOST-02, HOST-03, HMON-01, HMON-02, HMON-03, HMON-04,
HMON-06.

**Nicht diese Phase:** kein Verlauf und kein Eintrag in `internal/history`
(Phase 12), keine Warnschwellen und kein Host-Zustand gesund/Warnung/nicht
lesbar, keine Kachel auf der Wand, kein Eintrag in der Navigation mit Zustand
(Phase 12 — ein schlichter Navigationseintrag ohne Zustand ist hier erlaubt,
damit die Seite erreichbar ist), keine Aktionen und kein Helfer (Phase 13),
keine Tippziel-Arbeit über das hinaus, was die vorhandenen `ui/`-Bausteine
ohnehin mitbringen (Phase 14).

</domain>

<decisions>
## Implementation Decisions

### Datenquelle und Lesbarkeit (HMON-06)

- **D-01:** Neues Paket `internal/host`, das **lokal** liest — über ein
  `fs.FS`, das in Produktion auf `/` zeigt und in Tests auf einen Fixture-Baum
  (`testdata/pi5/`, `testdata/amd64/`, `testdata/procsubset-pid/`). Kein
  Subprozess, kein D-Bus, kein Root. Syscalls nur, wo sie kein `/proc` brauchen
  (`uname`, `statfs`, `clock_gettime`).
- **D-02:** Jeder Abschnitt der Antwort trägt seine Lesbarkeit selbst:
  `readable: bool` plus `reason` (Code + Satz) statt eines Wertes. Das Muster ist
  `health.Field[T]` (available / unavailable_reason); ob `Field[T]` direkt
  verwendet oder ein schmaler Host-Typ gebaut wird, entscheidet der Plan —
  bindend ist: ein nicht lesbarer Wert hat **keinen** Zahlenwert im JSON (kein
  `0`, kein `omitzero`, das zu `0` würde).
- **D-03:** „Durch die Härtung verborgen" wird **deterministisch** erkannt,
  nicht geraten: `/proc/self/mountinfo` (unter `ProcSubset=pid` weiter lesbar)
  nennt für das `/proc`-Mount die Option `subset=pid`. `ENOENT` auf
  `/proc/stat`, `/proc/meminfo`, `/proc/loadavg` **und** `subset=pid` im
  Mount → Ursache `hardening.proc-subset` mit dem Satz, dass die Unit
  `ProcSubset=pid` setzt, und der einen Zeile, die es ändert:
  `ProcSubset=all`. Jeder andere Lesefehler → Ursache `read-failed` mit dem
  Fehler. Nie eine stille 0.
- **D-04:** Was unter `ProcSubset=pid` trotzdem geht, wird **nicht** über `/proc`
  gelesen: Hostname und Kernel über `uname(2)`, Laufzeit seit dem Boot über
  `clock_gettime(CLOCK_BOOTTIME)` (`/proc/uptime` ist verborgen, HOST-01 steht
  aber nicht in der Liste der erlaubt-unlesbaren Werte), Modell über
  `/sys/firmware/devicetree/base/model` (Pi; das abschließende NUL wird
  abgeschnitten) bzw. `/sys/class/dmi/id/{sys_vendor,product_name}` (amd64),
  Betriebssystem aus `/etc/os-release` (`PRETTY_NAME`), Architektur aus
  `runtime.GOARCH` plus `uname -m`, Kernzahl aus
  `/sys/devices/system/cpu/online`.
- **D-05:** CPU-Auslastung (`/proc/stat`), Speicher und Swap (`/proc/meminfo`)
  werden aus `/proc` gelesen und stehen unter der Produktions-Unit als „nicht
  lesbar" da. Load kommt aus `/proc/loadavg` und, wenn das verborgen ist, aus
  `sysinfo(2)` — dieselben drei Werte, exakt (vom Orchestrator nachträglich
  entschieden, Erfolgskriterium 3 entsprechend angepasst). Der Guard-Test läuft gegen den Fixture-Baum `procsubset-pid` und ist
  gegen eine wieder eingesetzte 0 rot zu sehen.

### Was die Seite zeigt

- **D-06:** Drei Abschnitte, von oben: **Gerät** (Hostname, Modell,
  Architektur, OS, Kernel, Laufzeit seit Boot), **Dienst** (laufende Version,
  Laufzeit des Prozesses, Datenverzeichnis: Größe, Dateisystem, frei,
  Update-Status), **Live** (CPU/Load, Speicher/Swap, Datenträger, Temperaturen
  und Lüfter, Netzwerk). Ein Hinweisblock oben, nur wenn etwas wegen der Härtung
  fehlt: welche Werte, warum, welche Zeile.
- **D-07:** Datenträger: genau `/` und das Dateisystem, auf dem das
  Datenverzeichnis liegt, per `statfs`; welches Mount das ist, aus
  `/proc/self/mountinfo`. Liegen beide auf demselben Dateisystem, eine Zeile mit
  beiden Namen, nicht zwei gleiche Balken. Größe des Datenverzeichnisses per
  Verzeichnislauf, **gecacht für 60 s** (auf einer SD-Karte liest man nicht bei
  jedem Poll den Baum).
- **D-08:** Temperaturen: `/sys/class/thermal/thermal_zone*` und
  `/sys/class/hwmon/hwmon*`, der hwmon-Zwilling einer thermal zone wird wie bei
  den Knoten zusammengelegt (`talos.ClassifyChip`-Logik wiederverwenden, nicht
  kopieren). Nur `temp*_input` und `fan*_input`; Spannungen (`in*_input`,
  z. B. `rp1_adc`) werden nicht gezeigt. Kein Lüfter gemeldet → „kein Lüfter
  gemeldet", nicht eine leere Liste ohne Satz.
- **D-09:** Netzwerk: je Schnittstelle Durchsatz aus
  `/sys/class/net/<if>/statistics/{rx,tx}_bytes`. Standardmäßig nur physische
  Schnittstellen (`/sys/class/net/<if>/device` existiert); `lo`, `veth*`,
  Docker-Brücken sind virtuell und werden eingeklappt als „N virtuelle
  Schnittstellen" gezeigt, nicht verschwiegen.
- **D-10:** Die Oberfläche ist Englisch wie der Rest des Produkts: „Not
  readable", „Not recorded", Seitentitel „Host". Die deutschen Wörter in
  Requirements und Roadmap sind die Anforderung, nicht der UI-Text.

### API und Live-Aktualisierung

- **D-11:** Eine Leseroute `GET /api/v1/host`, `RequiresSession`,
  `MinRole: RoleReader`, kein `Action` (Lesen wird nicht auditiert). Sie liefert
  Identität, Dienst, Update-Status und Live-Werte in **einer** Antwort mit
  `observed_at` — eine Antwort, eine Uhr, wie `HardwareView`.
- **D-12:** Live heißt Polling wie bei `NodeHardware` (`useQuery` mit
  `refetchInterval: HARDWARE_POLL_INTERVAL_MS`), kein SSE. Raten (CPU %,
  Netzwerk-Bytes/s) rechnet der Server aus der Differenz zum vorigen Zähler,
  den ein Collector im Prozess hält — gleiches Muster wie `hardwareMemo` /
  `usableBaseline` in `internal/inventory/hardware.go`; der erste Aufruf nach
  dem Start hat für Raten noch keine Basis und sagt das („noch keine zweite
  Messung"), statt 0 zu zeigen.
- **D-13:** Die Elementtypen für Temperatur, Lüfter, Schnittstelle und
  Dateisystem übernehmen die JSON-Form von `inventory.HardwareTemperature`,
  `HardwareFan`, `HardwareLink`, `HardwareFilesystem`, damit die Anzeige-Bausteine
  von `NodeHardware.tsx` (Meter, Sensorliste, `temperatureLimits`) geteilt statt
  kopiert werden. Zod-Schema in `web/src/api.ts` mit
  `.nullish().transform(orEmpty)` für Listen (Ledger 162), Go-Seite mit
  `make()` — kein `null` im Dokument.

### Update-Status (HOST-03)

- **D-14:** `deploy/holzkube-manager-update.sh` schreibt nach jedem Lauf als
  root eine kleine JSON-Datei `/var/lib/holzkube-manager-update/status.json`
  (Verzeichnis root, 0755; Datei 0644; atomar über `mktemp` im selben
  Verzeichnis und `mv`): `checked_at` (RFC 3339, UTC), `installed`, `latest`,
  `outcome` (`current` | `available` | `updated` | `rolled-back` | `failed`).
  **Nicht** ins Datenverzeichnis des Daemons: root schreibt nicht in ein
  Verzeichnis, das dem unprivilegierten Benutzer gehört (Symlink-Falle).
- **D-15:** Das Schreiben ist Nebensache: es darf ein Update nie scheitern
  lassen (`|| true`, eigene Funktion), und `--check` ohne root schreibt nichts,
  wenn das Verzeichnis nicht schreibbar ist. Ein Test fährt das Skript gegen ein
  Test-Verzeichnis mit Stellvertretern für `curl`/`systemctl`.
- **D-16:** Der Daemon liest die Datei (Pfad per Flag/Env überschreibbar,
  Größenlimit, strenges Parsen). Fehlt sie → „Not recorded" mit dem Satz, dass
  das installierte Update-Skript nichts hinterlegt; kaputt → „Not readable" mit
  Ursache. Nie eine erfundene Zeit oder Version.

### Container-Betrieb (Docker / compose.yaml)

- **D-17:** Der Daemon erkennt, dass er in einem Container läuft
  (`/.dockerenv`, `/run/.containerenv`, oder `container=` in der Umgebung von
  PID 1 soweit lesbar) und sagt es oben auf der Seite: „holzkube-manager runs in
  a container. Kernel, CPU, memory and temperatures are the host's; hostname,
  network and filesystems are the container's." Die Werte werden gezeigt, aber
  Hostname und Netzwerk tragen den Zusatz „container". Phase 13 baut darauf
  (keine Host-Aktionen im Container).

### Claude's Discretion

- Paketaufteilung innerhalb von `internal/host` (Collector, Leser, Typen),
  Namen der Go-Typen, genaue Formulierung der englischen Sätze.
- Ob `/api/v1/host` in `handlers/host.go` neu entsteht (erwartet) und wie die
  Deps-Struktur `httpapi.Deps` den Collector bekommt.
- Aufbau der React-Komponente (`routes/host.tsx` plus geteilte Bausteine aus
  `NodeHardware.tsx` herausgezogen), solange nichts dupliziert wird, was schon
  existiert.

### Ohne Rückfrage entschieden

Gewählt wurde jeweils die empfohlene Antwort; verworfen:

- *Speicher und Swap über `sysinfo(2)` lesen, das auch unter `ProcSubset=pid`
  geht* — verworfen, weil `sysinfo` kein `MemAvailable` und keinen Page-Cache
  kennt: „belegt" wäre um den Cache zu hoch, eine plausible falsche Zahl.
  Load dagegen ist über `sysinfo` exakt und wird so gelesen (D-05).
- *Unlesbarkeit am `ENOENT` allein erkennen* — verworfen, ein fehlendes `/proc`
  in einem exotischen Container wäre dann fälschlich „Härtung"; `subset=pid` im
  Mount ist der Beweis.
- *Update-Status ins Datenverzeichnis des Daemons schreiben* — verworfen,
  root-Schreibzugriff in ein Verzeichnis des unprivilegierten Benutzers ist eine
  Symlink-Falle.
- *Update-Status aus dem Journal der Update-Unit lesen* — verworfen, der Daemon
  hat kein `AF_UNIX`/D-Bus und darf das Journal nicht lesen.
- *SSE statt Polling* — verworfen, `NodeHardware` pollt; ein zweites
  Live-Muster für eine Seite ist Gewicht ohne Nutzen.
- *Alle Schnittstellen gleichrangig zeigen* — verworfen, auf dem Pi sind es
  mit Docker-Brücken und veth sieben, davon zwei echte.
- *Im Container die Seite ganz sperren* — verworfen, Kernel, CPU und
  Temperaturen sind dort die des Hosts und nützlich; ehrlich beschriften reicht.
- *UI-Texte deutsch* — verworfen, das Produkt ist durchgehend englisch.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/inventory/hardware.go`: `HardwareView` und Elementtypen
  (`HardwareTemperature` mit `high_c`/`critical_c`, `HardwareFan`,
  `HardwareLink`, `HardwareFilesystem`), Raten aus Zählerdifferenzen
  (`computeRates`, `cpuPercent`, `perSecond`, `usableBaseline`).
- `internal/talos/sensors.go`: `ClassifyChip`, `inputsNamed`, `numbered`,
  `millidegrees` — Sensorlogik, heute über die Talos-RPCs; der reine Teil ist
  wiederverwendbar.
- `internal/health/health.go`: `Field[T]` mit `Known` / `Stale` / `Never`.
- `web/src/components/NodeHardware.tsx`: `temperatureLimits`,
  `TEMPERATURE_DEFAULTS`, `cpuTemperature`, `KIND_LABEL`, Aufbau der Meter.
- `web/src/components/charts/Meter.tsx` (`Meter`, `severityOf`),
  `LiveChart.tsx`, `Sparkline.tsx`.
- `cmd/holzkube-managerd/main.go`: `var version` (ldflags) — die laufende
  Version.

### Established Patterns
- Routen als `httpapi.Route` mit `RequiresSession`, `MinRole`; Lesen ohne
  `Action`. Neue Route gehört in `docs/api-contract.md`.
- Keine `null`-Listen im JSON (Ledger 162, `internal/kube/emptylists_test.go`);
  im Browser `.nullish().transform(orEmpty)`.
- Guards werden gegen den wieder eingesetzten Fehler rot gesehen (CLAUDE.md).
- `web/fixtures/demo.json` speist die Layout-Prüfung; `/host` braucht dort eine
  Antwort, die `web/src/fixtures.test.ts` gegen das zod-Schema prüft.

### Integration Points
- `internal/httpapi/router.go` / `handlers/` — neue `HostRoutes`.
- `web/src/App.tsx` Routenbaum, `web/src/components/Sidebar.tsx` (schlichter
  Eintrag „Host").
- `web/scripts/layout-audit.mjs` `ROUTES` — `/host` aufnehmen.
- `deploy/holzkube-manager-update.sh` (Status-Datei) — wird über
  `.goreleaser` mit ausgeliefert.

</code_context>

<specifics>
## Specific Ideas

- Gemessen auf dem Pi 5 (Debian 13, arm64): Modell steht in
  `/sys/firmware/devicetree/base/model` mit abschließendem NUL; hwmon hat
  `cpu_thermal` (Zwilling von `thermal_zone0`, Typ `cpu-thermal`), `rp1_adc`
  (ein `temp1_input` plus Spannungen) und `rpi_volt` (keine Eingänge); **kein
  Lüfter-hwmon** auf dieser Maschine; `thermal_zone0` hat Trip-Points
  `active` 50/60/67,5/75 °C (Lüfterstufen) und `critical` 110 °C;
  `/sys/class/dmi` existiert nicht; Schnittstellen: `eth0`, `wlan0`, `lo`,
  `docker0`, eine Docker-Brücke, zwei `veth*`.
- Die Fixture-Bäume bilden genau das ab (Pi 5) plus einen amd64-Desktop mit
  `k10temp`/`nct6798`, Lüftern und DMI.
- **Wichtig für die Auslieferung:** `holzkube-manager-update.sh` ersetzt sich
  nach einem gesunden Update selbst aus dem Release-Archiv. Die Änderung aus
  D-14 erreicht den Pi also mit dem nächsten Release über den Mechanismus, den
  der Betreiber schon installiert hat — ohne Handgriff von uns. Ab dann zeigt
  `/host` den Update-Status; bis dahin „Not recorded".
- Die echte Unit des Betreibers liegt nicht im Repository; Beispielwerte in
  Doku und Tests nur mit Dokumentationsadressen (`192.0.2.0/24`,
  `example.com`).

</specifics>

<deferred>
## Deferred Ideas

- Eine Referenz-Unit `deploy/holzkube-manager.service` im Repository — nicht
  verlangt; die Anleitung in Phase 13 beschreibt nur die Helfer-Units.
- Spannungen (`in*_input`) und Drosselungs-Flags des Pi (`vcgencmd
  get_throttled` bräuchte einen Subprozess) — nicht in diesem Milestone.
- Wunsch „Hardware-Temperaturen" (2026-09-21): die Knoten sind seit v0.0.1
  über Talos abgedeckt, der Manager-Pi selbst mit dieser Phase; die dritte
  genannte Quelle (Prometheus im Cluster) bleibt ungebaut.

</deferred>
