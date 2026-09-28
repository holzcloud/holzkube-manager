# Phase 12: Host wie ein Knoten — Verlauf, Warnung, Wand - Context

**Gathered:** 2026-09-28
**Status:** Ready for planning

<domain>
## Phase Boundary

Der Host aus Phase 11 bekommt, was ein Knoten hat: Verlaufsdiagramme über
1 h / 6 h / 24 h für CPU, Speicher, Temperatur und Netzwerk aus
`internal/history` (überleben einen Neustart, Lücken statt Nulllinien), eine
Warnung mit Wert und Schwelle bei zu hoher Temperatur oder zu voller Platte,
und einen Zustand (gesund / Warnung / nicht lesbar), der in der Navigation und
als Kachel auf der Wand erscheint.

Requirements: HMON-05, HMON-07, HOST-04.

**Nicht diese Phase:** keine neue Pipeline, keine neuen Bereiche, keine neuen
Diagrammtypen; keine Benachrichtigung nach außen (Mail, Push); keine
konfigurierbaren Schwellen in der Oberfläche; keine Aktionen (Phase 13).

</domain>

<decisions>
## Implementation Decisions

### Verlauf (HMON-05)

- **D-01:** Ein neues Subjekt im vorhandenen `history.Store`:
  `history.HostSubject()` = `"host/local"`. `retained()` bekommt dafür einen
  ausdrücklichen Fall (heute würde der unbekannte Präfix zwar zufällig behalten,
  aber „zufällig" ist kein Vertrag); `Retain()` löscht den Host nie, `Prune()`
  wie jedes Subjekt nach 24 h Stille.
- **D-02:** Der Sampler bekommt eine optionale Abhängigkeit
  `SamplerDeps.Host func(ctx) (map[string]float64, error)` und schreibt den Host
  im selben Durchlauf, mit demselben Zeitstempel und demselben `FlushIfDue`
  wie die Knoten. Kein eigener Ticker, keine zweite Datei.
- **D-03:** Die Serienschlüssel sind die der Knoten (`HardwareValues`):
  `cpu`, `memory`, `rx`, `tx`, `temp:<chip>/<label>`, `fan:<chip>/<label>`,
  `core:<n>`. Ein Wert, der nicht lesbar ist (D-05 aus Phase 11), wird
  **nicht** geschrieben — der Schlüssel fehlt in der Map, der Ring bleibt NaN,
  das Diagramm zeigt eine Lücke. `rx`/`tx` summieren nur die physischen
  Schnittstellen (Phase 11 D-09), sonst zählte der Docker-Verkehr doppelt.
- **D-04:** Route `GET /api/v1/host/history?range=1h|6h|24h`, `RoleReader`,
  gleiche Antwort (`history.View`) und gleiche Fehler wie
  `/api/v1/machines/{id}/hardware/history`.
- **D-05:** Die Seite nutzt `useChartRange`, `RangePicker`, `LiveChart` und
  `merge` genau wie `NodeHardware`; die Diagrammblöcke werden dafür aus
  `NodeHardware.tsx` in geteilte Komponenten gezogen, nicht kopiert. Unter der
  Produktions-Unit sind CPU und Speicher nicht lesbar → ihre Diagramme stehen
  mit dem Satz „Not readable — no history" da statt als leere Achse.

### Schwellen und Warnung (HMON-07)

- **D-06:** Der Zustand wird **im Server** berechnet (die Wand und ein
  Wall-Link brauchen ihn ohne Browser-Logik). Die Temperaturgrenzen ziehen
  dafür von `temperatureLimits` in `NodeHardware.tsx` nach Go um und werden je
  Sensor als `warn_c`/`danger_c` mitgeschickt; der Browser zeigt die
  Server-Grenzen, statt sie selbst zu rechnen — eine Implementierung.
- **D-07:** Temperatur-Warnung, wenn ein Sensor `≥ warn`. `warn` = eigene
  `high_c` des Chips (hwmon `temp*_max` bzw. ein Trip-Point vom Typ `passive`
  oder `hot`), sonst Vorgabe je Art (CPU 80 °C, wie heute im Browser). Trip-Points
  vom Typ `active` sind Lüfterstufen und **keine** Schwelle (auf dem Pi 5 läge
  die Warnung sonst bei 50 °C).
- **D-08:** Datenträger-Warnung, wenn `/` oder das Dateisystem des
  Datenverzeichnisses `≥ 80 %` belegt ist — dieselbe Grenze, bei der der Meter
  schon heute gelb wird. Seite, Navigation und Kachel sind sich damit einig.
- **D-09:** Die Warnung nennt Wert und Schwelle, z. B. „cpu-thermal 82.1 °C ≥
  80 °C" oder „/ 91 % used ≥ 80 %"; bei mehreren die schlimmste zuerst, alle
  auf der Seite. Der Test ist gegen eine entfernte Schwellenprüfung rot zu
  sehen.

### Zustand, Navigation, Wand (HOST-04)

- **D-10:** Genau ein Zustand aus `ok` / `warn` / `unknown` (UI: „Healthy",
  „Warning", „Not readable"). Reihenfolge: eine Warnung schlägt „nicht lesbar"
  (ein bekanntes Problem ist wichtiger als ein fehlendes), „nicht lesbar"
  schlägt „gesund".
- **D-11:** „Nicht lesbar" heißt: die Werte, **auf denen der Zustand beruht**,
  sind nicht lesbar — alle Temperaturen **und**/oder die Belegung der beiden
  Dateisysteme, oder der letzte Schnappschuss ist älter als drei
  Abtastintervalle. Durch die Härtung verborgene CPU/Speicher-Werte machen den
  Host **nicht** grau: keine Schwelle hängt an ihnen, und ein Pi, der unter der
  ausgelieferten Härtung dauerhaft grau wäre, würde lehren, Grau zu übersehen.
  Ein Host ohne jeden Temperatursensor (VM) wird allein nach den Platten
  bewertet und sagt das.
- **D-12:** Der Guard: ein Host, dessen bewertete Werte nicht lesbar sind,
  ist nie `ok` — Tabellentest über die Kombinationen, rot gesehen gegen eine
  Implementierung, die `ok` zurückgibt.
- **D-13:** Navigation: der Sidebar-Eintrag „Host" (aus Phase 11) bekommt einen
  Zustandspunkt in den drei Farben mit Text für Screenreader; die Daten kommen
  aus der Host-Antwort, die ohnehin gepollt wird, kein zweiter Endpunkt.
- **D-14:** Wand: die Antwort von `GET /api/v1/clusters/{id}/wall` bekommt ein
  Feld `host` (Name, Zustand, Grund-Satz) — **nicht** als Eintrag in
  `kube.Wall.Nodes`, weil der Host kein Kubernetes-Knoten ist und die
  Cluster-Summen nicht verfälschen darf. Die Wand zeichnet ihn als erste Kachel
  im Abschnitt „Nodes", mit dem Zusatz „manager". Wall-Links sehen ihn damit
  ebenfalls, und die Offenlegungsregel der Wand hält: Name, Zustand, Grund —
  keine Adresse, keine Version.

### Claude's Discretion

- Wo die Zustandsberechnung wohnt (`internal/host` erwartet) und wie die Wand
  an sie kommt (Deps-Feld).
- Ob `unknown` auf der Wand die vorhandene Farbe `unknown` (grau) nutzt —
  erwartet ja.
- Genaue Sätze der Warnungen.

### Ohne Rückfrage entschieden

Gewählt wurde jeweils die empfohlene Antwort; verworfen:

- *Eigener Sampler/Ticker für den Host* — verworfen, eine zweite Schleife ist
  die „neue Pipeline", die REQUIREMENTS ausschließt.
- *Unlesbare Werte als 0 in den Verlauf* — verworfen, verstößt gegen HMON-06
  und die Ring-Semantik (Lücke = nicht gehört).
- *Jeder unlesbare Wert macht den Host „nicht lesbar"* — verworfen, der Pi
  wäre unter `ProcSubset=pid` dauerhaft grau.
- *Schwellen im Browser berechnen (wie heute bei Knoten)* — verworfen, die Wand
  und die Navigation brauchen dieselbe Entscheidung ohne sie zu duplizieren.
- *Datenträger-Warnung erst bei 90 %* — verworfen, der Meter wird bei 80 %
  gelb; Kachel und Meter müssen dasselbe sagen.
- *Host als Eintrag in `wall.nodes`* — verworfen, er würde in
  `summary`/Kapazität eines Clusters mitgezählt, zu dem er nicht gehört.
- *Schwellen konfigurierbar machen* — verworfen für diesen Milestone
  (Umfang); vorgemerkt.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/history`: `Store.Record/Query`, `Range` (1h/6h/24h), `View`,
  `Retain`/`retained`/`Prune`, `persist.go` (Datei überlebt Neustart, Flush beim
  Beenden), `sampler.go` (`SamplerDeps`, `pass`, `HardwareValues`).
- `internal/httpapi/handlers/history.go`: Muster der History-Route.
- `internal/httpapi/handlers/wall_trends.go` und `kubernetes.go` (`ForTheWall`,
  Antwort mit eingebetteten `trends`) — dort kommt `host` hinzu.
- `web/src/components/NodeHardware.tsx`: `useLiveSeries`, `useChartRange`,
  `merge`, `temperatureLimits`.
- `web/src/routes/wall.tsx`: `Named`, `TILE_COLOURS` (`ok`/`warn`/`unknown`).

### Established Patterns
- Ringe über absolute Slots: nicht geschrieben = Lücke.
- Wand-Antwort trägt alles, was ein Wall-Link sehen darf, weil der Link nur
  diese eine Route öffnen kann (`Route.WallLink`).
- Guards rot gegen den wieder eingesetzten Fehler.

### Integration Points
- `cmd/holzkube-managerd/main.go`: Sampler-Deps bekommen den Host-Collector.
- `Sidebar.tsx`: Zustandspunkt am Eintrag „Host".
- `web/fixtures/demo.json`: Host-Verlauf und `host` in der Wand-Antwort, damit
  die Layout-Prüfung beides sieht.
- `docs/api-contract.md`: neue Route, neues Feld der Wand.

</code_context>

<specifics>
## Specific Ideas

- Pi 5: `thermal_zone0` meldet nur `active`-Trip-Points (50/60/67,5/75 °C) und
  `critical` 110 °C, keinen `passive` — die CPU-Warnung liegt dort also bei der
  Vorgabe 80 °C, Gefahr bei 110 °C. Die Firmware drosselt ab etwa 80–85 °C,
  die Vorgabe passt.
- Erfolgskriterium 2 wird mit einem Test gehalten, der Store schreibt, flusht,
  neu öffnet und eine Phase ohne lesbare CPU als Lücke wiederfindet.

</specifics>

<deferred>
## Deferred Ideas

- Schwellen für Temperatur und Belegung in den Einstellungen änderbar machen.
- Benachrichtigung nach außen, wenn der Host in „Warnung" geht.
- Der Host im Dashboard `/` neben den Clustern (die Roadmap verlangt
  Navigation und Wand, nicht das Dashboard).

</deferred>
