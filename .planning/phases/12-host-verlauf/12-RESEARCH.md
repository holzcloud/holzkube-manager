# Phase 12: Host wie ein Knoten — Verlauf, Warnung, Wand - Research

**Researched:** 2026-09-28
**Domain:** Extending the existing Go history sampler, host collector and wall answer; React chart/sidebar/wall components. No new libraries.
**Confidence:** HIGH (every mechanism below was read in this session's checkout; the few assumptions are listed in the Assumptions Log)
**Where this ran:** On the operator's Raspberry Pi 5, natively on aarch64 (`uname -m` = aarch64). Go 1.26.7 via `GOTOOLCHAIN=go1.26.7` from `~/.local/go`, Node v22.23.3 via nvm, Playwright Chromium present. The production service and `/var/lib/holzkube-manager` were not touched; the only live-system reads were `/sys/class/hwmon` and `/sys/class/thermal` (read-only).

## Summary

Everything this phase needs already exists and only has to be connected: `internal/history` stores rings over absolute slots where "not written" is a gap by construction, persists them in one file that survives a restart (flushed at most every 30 min and on shutdown), and serves `history.View`; `internal/host.Collector` produces `Reading[T]` values that are either readable or carry a reason; the wall answer already embeds extra data (`trends`) beside `kube.Wall`; the browser already has `LiveChart` (gap-aware `segments()`), `RangePicker`, `useChartRange`, `merge`, `Sparkline` and the shared `Sensors` module. The phase adds one history subject, one sampler dependency, one route, one pure assessment function, two per-sensor fields, one field on the wall answer, and the UI to show them.

Three findings change how the plan must be cut. **(1) The collector has a single rate memo shared by every caller** (`c.prev`, advanced by any read ≥ 0.5 s after the previous one). If the sampler simply calls `Collector.Read` every 15 s, the page's 3-s CPU/network rates and the sampler's 15-s averages corrupt each other's windows. The sampler needs its own baseline (a second memo), and the sampler must not call `Read` (which also walks the data directory). **(2) `Sampler.pass` returns early when the inventory cannot be listed** — a host sampled "after the machines" would lose its history exactly when the store is in trouble; the host must be sampled before that early return. **(3) D-06 removes `temperatureLimits` from the browser**, but the node page uses it too — so `warn_c`/`danger_c` go onto the shared `inventory.HardwareTemperature` and are filled at *both* construction sites (node and host), from one Go function ported rule-for-rule from `Sensors.tsx`. On the Pi 5 the `cpu_thermal` hwmon chip has no `temp1_max`/`temp1_crit` (measured this session), so reaching D-07's "danger 110 °C" requires reading the twin thermal zone's `critical` trip point.

**Primary recommendation:** Build in four vertical slices — (A) Go: limits + assessment + sampler/host subject + history route; (B) Go: wall `host` field; (C) web: shared chart blocks + `/host` history + state line + warning notice + Sidebar mark + wall tile; (D) fixtures, README/guide/contract, screenshots, Pi verification — each guard seen red against its reinstated fault.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

#### Verlauf (HMON-05)

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

#### Schwellen und Warnung (HMON-07)

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

#### Zustand, Navigation, Wand (HOST-04)

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

### Ohne Rückfrage entschieden (from CONTEXT.md, verbatim)

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

### Deferred Ideas (OUT OF SCOPE)

- Schwellen für Temperatur und Belegung in den Einstellungen änderbar machen.
- Benachrichtigung nach außen, wenn der Host in „Warnung" geht.
- Der Host im Dashboard `/` neben den Clustern (die Roadmap verlangt
  Navigation und Wand, nicht das Dashboard).

Also out of scope (CONTEXT `<domain>`): keine neue Pipeline, keine neuen Bereiche, keine neuen Diagrammtypen; keine Benachrichtigung nach außen; keine konfigurierbaren Schwellen in der Oberfläche; keine Aktionen (Phase 13).

### Binding from the approved UI-SPEC ("Checker resolutions", 12-UI-SPEC.md lines 425-446)

1. Hardening state: the 1-minute load takes the Processor card's `text-xl` figure slot when usage is not readable; usage sits as a muted `MissingValue` row below.
2. Unreadable load is drawn by `MissingValue` (the ad-hoc "Load not readable: …" line goes).
3. Fan icon: `motion-safe:animate-spin`.
4. 12 px tracking (`--hc-track-snug`, `web/src/index.css`) fixed app-wide; layout audit and node-page tests stay green.
5. `gap-1.5` stays (inherited, D-05).
6. "1 core" singular; the filesystem caption states the df-style percentage it shows.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| HMON-05 | Zu CPU, Speicher, Temperatur und Netzwerk gibt es Verlaufsdiagramme über 1 h / 6 h / 24 h, die einen Neustart des Daemons überleben | `history.Store` rings + `persist.go` already survive a restart (`TestTheHistorySurvivesARestart`); add `HostSubject`, `SamplerDeps.Host`, a sampler-only rate baseline in the collector, `GET /api/v1/host/history`, and the shared chart blocks on `/host` (Patterns 1-3, 6) |
| HMON-07 | Überschreitet Temperatur oder Datenträgerbelegung eine Schwelle, zeigt der Host eine Warnung mit dem Grund | Port `temperatureLimits` to Go as one function feeding `warn_c`/`danger_c`; read twin-zone trip points on the host; `host.Assess(Live)` produces warnings "value ≥ threshold" worst-first (Patterns 4-5) |
| HOST-04 | Der Host erscheint in der Navigation und auf der Wand wie ein Knoten, mit einem Zustand (gesund / Warnung / nicht lesbar) | `health` block on `/api/v1/host` (Sidebar mark via the same `['host']` query); `host` field on the wall answer from the sampler's last snapshot with the 45-s staleness rule; host tile prepended in `Named` (Patterns 5, 7, 8) |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **Public repository:** no LAN address, host name, public name, mail, MAC or home directory from the real setup — not in tests, fixtures, ledger, commit messages. Use `192.168.1.10`, `homeserver`, `example.com`-style values (the fixtures already use `manager-01.homelab.example`). `internal/publicrepo` fails the gate on known values (stored as hashes). The Pi's real hostname must not appear in any fixture, test, screenshot or summary of this phase.
- **README rule:** every new feature updates README "What it does" in the same change; a screen of its own gets a rendered screenshot (`task build && node web/scripts/readme-images.mjs`), never one taken from the real cluster. Depth goes to `docs/guide.md`.
- **Changelog:** only at release time (`internal/changelog/changelog.json`); releases only on the operator's request (memory: release-only-on-request). Not part of this phase.
- **Alpha** badge/sentences stay untouched.
- **Verification honesty:** say where each check ran. This phase runs on the Pi (aarch64): no `go test -race` (ThreadSanitizer refuses the Pi 5 kernel), Go via `~/.local/go`, Node via nvm. Replacing/restarting the production service or copying `/var/lib/holzkube-manager` is the operator's call — never done here.
- **Method:** a guard counts only after it has gone red against the fault deliberately put back; an injection that did not inject is an unperformed measurement. Every commit message claiming coverage means the fault was reinstated and seen red.
- **Every question is a choice** via `AskUserQuestion` — but the orchestrator ordered this milestone to run without questions: decide and record ("decided without asking").
- **Exit codes:** read them from the command itself, never through a pipe (memory: empty-output-is-not-green).
- **No push CI** (no Actions minutes): `./bin/task ci` locally before any push.
- **Talos pin** rule is unaffected by this phase.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Reading host values every 15 s | API/Backend (`internal/history` sampler) | `internal/host` collector | D-02: one pipeline; the sampler already owns the timer |
| Storing and persisting host history | Database/Storage (`history.Store`, `metrics.bin`) | — | Existing ring + file; only a new subject key |
| Temperature limits (warn/danger per sensor) | API/Backend (`internal/inventory`, one Go function) | Browser only displays | D-06: one implementation; node and host share it |
| Host state `ok/warn/unknown`, warning sentences | API/Backend (`internal/host`, pure `Assess`) | — | D-06/D-10: wall and wall links have no browser logic to rely on |
| Wall `host` field | API/Backend (`handlers/kubernetes.go` wall route) | `host.Collector` snapshot | D-14: the one route a wall link opens must carry it |
| History charts on `/host` | Browser (shared chart blocks, `useChartRange`, `merge`) | — | D-05: same components as a node |
| Sidebar state mark | Browser (`Sidebar.tsx`, `['host']` query) | Server `health` block | D-13: no second endpoint |
| Wall host tile | Browser (`wall.tsx` `Named`) | Server `host` field | D-14 |

## Standard Stack

No package is added. Everything is in the repository or already a dependency.

### Core (existing, versions read this session)
| Library | Version | Purpose | Source |
|---------|---------|---------|--------|
| Go toolchain | go1.26.7 (gate), go1.27.1 local | backend | `go version` this session |
| `@tanstack/react-query` | 5.102.6 | polling, shared `['host']` query | `web/package.json` line 22; `node_modules/@tanstack/query-core/package.json` |
| `react` | 19.2.8 | UI | `web/package.json` line 28 |
| `zod` | 4.4.3 | response schemas (`hostSchema`, `wallSchema`, `temperatureSchema`) | `web/package.json` line 33 |
| `vitest` (+ `@vitest/browser-playwright`) | 4.1.11 | jsdom + browser projects | `web/package.json` lines 44-53; projects `jsdom` and `browser` in `web/vite.config.ts` |
| `playwright` | 1.62.1 | layout audit, README images | `web/package.json` line 48 |
| `lucide-react` | 1.34.0 | `AlertTriangle`, `Cpu` | UI-SPEC inventory |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Sampler calling `Collector.Read` | a dedicated `Collector.Sample` with its own rate baseline | `Read` shares the page's rate memo and walks the data directory (`readService` → `dirSizer`) every minute forever; `Sample` reads only what history and the assessment need. **Use `Sample`.** |
| Wall computing a fresh host reading per request | the sampler's last snapshot + 45-s staleness | A fresh read on every wall refresh makes D-11's "older than three sampling intervals" dead code; the snapshot costs nothing per request and turns grey if the sampler stalls. **Use the snapshot.** |
| A charting library | the existing hand-built `LiveChart` | Out of scope (no new chart types); `LiveChart` already draws gaps. |

**Installation:** none.

## Package Legitimacy Audit

This phase installs no external package (Go or npm). `go.mod`/`go.sum` and `web/package.json`/`package-lock.json` must be unchanged by it; the plan-checker can assert `git diff --stat` on those four files is empty.

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| (none) | — | — | — | — | — | — |

**Packages removed due to [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

## Architecture Patterns

### System Architecture Diagram

```
                       every 15 s (one ticker, history.Sampler.Run)
                                   │
                                   ▼
                    ┌──────── Sampler.pass(at) ────────────────────────────┐
                    │ 1. host: deps.Host(ctx) ──► Collector.Sample          │
                    │      readLive() with SAMPLER baseline (not c.prev)    │
                    │      ├─► HistoryValues(live) → map (readable only)    │
                    │      │     → Store.Record("host/local", at, map)      │
                    │      └─► Assess(live) → Health; stored as last{at,…}  │
                    │ 2. inventory (may fail → return; host already written)│
                    │ 3. Retain(known, live)  (host/local always retained)  │
                    │ 4. Prune(at); nodes + apps as today                   │
                    └──────────────────────────┬────────────────────────────┘
                                               ▼
                                   FlushIfDue (≥30 min) / Flush on shutdown
                                               ▼
                                   <data>/history/metrics.bin

 Browser /host (3 s) ─► GET /api/v1/host ─► Collector.Read (PAGE baseline c.prev)
                                            └─► View + health = Assess(view.Live)
 Sidebar (30 s, off on /host) ─► same ['host'] query ─► health.state → mark
 /host charts ─► GET /api/v1/host/history?range= ─► Store.Query("host/local")
                  merge(history, live points) ─► shared ProcessorChart/MemoryChart/NetworkChart/Sensors
 Wall (10 s, session or wall link) ─► GET /api/v1/clusters/{id}/wall
      ├─► kube ForTheWall (nodes, summary — host NOT added)
      ├─► trendsForTheWall (history)
      └─► hostForTheWall: Collector.Latest(now) ─► age > 3×FineStep ? unknown : last health
                          → {"host": {"name","state","reason"}} | null
```

### Recommended file placement
```
internal/inventory/limits.go        # TemperatureLimits(kind, high, crit) (warn, danger) — ported from Sensors.tsx
internal/inventory/hardware.go      # HardwareTemperature gains WarnC/DangerC; flattenSensors fills them
internal/host/sensors.go            # trip points: passive/hot → HighC, critical → CriticalC; fills WarnC/DangerC
internal/host/health.go             # Health, HealthState, Assess(Live) — pure
internal/host/sample.go             # Collector.Sample, HistoryValues, Latest
internal/host/collector.go          # rates() takes the baseline to use
internal/history/history.go         # HostSubject(), retained() explicit case
internal/history/sampler.go         # SamplerDeps.Host, sampleHost before the inventory
internal/httpapi/handlers/history.go# GET /api/v1/host/history in HistoryRoutes
internal/httpapi/handlers/wall_host.go # wallHost + hostForTheWall (beside wall_trends.go)
internal/httpapi/handlers/kubernetes.go # clusterWall embeds Host
web/src/components/charts/HardwareCharts.tsx # ProcessorChart, CoreList, MemoryChart, NetworkChart (moved from NodeHardware)
web/src/components/HostState.tsx    # HostStateMark
web/src/routes/host.tsx, web/src/components/Sidebar.tsx, web/src/routes/wall.tsx, web/src/api.ts
```

### Pattern 1: History subject and retention (D-01)

Verified source [VERIFIED: internal/history/history.go:127-128, 402-414]:
```go
func MachineSubject(id model.MachineID) string { return "machine/" + string(id) }
...
func retained(key string, machines map[model.MachineID]bool, clusters map[model.ClusterID]bool) bool {
	if id, ok := strings.CutPrefix(key, "machine/"); ok {
		return machines[model.MachineID(id)]
	}
	if rest, ok := strings.CutPrefix(key, "app/"); ok {
		cluster, _, _ := strings.Cut(rest, "/")
		return clusters[model.ClusterID(cluster)]
	}
	// A key this version does not know how to read -- one a later version
	// wrote, loaded after a downgrade -- is left to age out rather than
	// deleted on sight.
	return true
}
```
Add `func HostSubject() string { return "host/local" }` and, first in `retained`, `if key == HostSubject() { return true }`. `forgetFailing` (sampler.go:294-311) also calls `retained`, so the host's failure memory is kept too — correct. `Prune` stays as is (24 h silence).

**Guard design note:** removing the explicit case leaves the default `return true`, so a test cannot go red against *that* removal — by design. The fault to reinstate is "the host filed like a machine": `HostSubject()` returning `"machine/local"` → `Retain` with no machine `local` deletes it → red.

### Pattern 2: Sampler dependency, host first (D-02, D-03)

Verified [VERIFIED: internal/history/sampler.go:66-84, 139-150]:
```go
type SamplerDeps struct {
	History *Store
	Logger  *slog.Logger
	Now     func() time.Time
	Interval time.Duration
	Inventory func(ctx context.Context) ([]model.Machine, []model.Cluster, error)
	Hardware func(ctx context.Context, id model.MachineID) (inventory.HardwareView, error)
	Apps func(ctx context.Context, cluster model.ClusterID) (kube.Apps, error)
}
...
	at := s.deps.Now()

	machines, clusters, err := s.deps.Inventory(ctx)
	if err != nil {
		s.failed("inventory", "the inventory could not be listed; nothing is sampled", err)
		return
	}
```
Add `Host func(ctx context.Context) (map[string]float64, error)` (optional: nil = not sampled). In `pass`, call `s.sampleHost(ctx, at)` **between `at := s.deps.Now()` and the inventory call**, so an inventory failure does not skip the host. `sampleHost` mirrors `sampleMachine`: on error `s.failed(HostSubject(), "the host could not be read; its charts have a gap until it can", err)`, else `recovered` + `Record(HostSubject(), at, values)`. `Record` with an empty map is a no-op (history.go:239-241), so a darwin build (everything `unsupported`) records nothing and does not refresh `seen`.

Wiring [VERIFIED: cmd/holzkube-managerd/main.go:649-667]: add `Host: hostCollector.Sample` to the `history.NewSampler(history.SamplerDeps{…})` literal; `hostCollector` is built at main.go:562, before the sampler.

### Pattern 3: The collector's rate memo — give the sampler its own baseline

Verified [VERIFIED: internal/host/collector.go:57-60, 323-329]:
```go
const (
	minRateWindow = 500 * time.Millisecond
	maxRateWindow = 5 * time.Minute
)
...
	if prev == nil || window < 0 || window >= minRateWindow {
		c.prev = &cur
	}
```
Every read ≥ 0.5 s after the previous one replaces `c.prev`. Consequences if the sampler shared it: with `/host` open, the sampler's CPU% becomes a ≤ 3-s window instead of 15 s, and about one page poll in five gets a shortened window. **Recommendation (decided):** refactor `rates(live, cur, …)` to take the baseline slot (`*(*counters)`) it reads and advances; `Read` passes `&c.prev`, `Sample` passes `&c.samplePrev`. Both under `c.mu`.

`Sample(ctx) (map[string]float64, error)` does **not** call `readService`/`readDevice` except `Sys.Uname()` for the wall name: `readService` walks the data directory (cached 60 s) and reads the update status — nothing the history or the assessment needs.

`HistoryValues(l Live) map[string]float64` (in `internal/host`, so `internal/history` does not import `internal/host`):
- `cpu` only if `l.CPU.Usage.Readable`; `core:<i>` only if `l.CPU.PerCore.Readable`.
- `memory` = `100 * used / total` only if `l.Memory.Readable && total > 0` — the node formula [VERIFIED: sampler.go:241-243 `if v.Memory.TotalBytes > 0 { out["memory"] = 100 * float64(v.Memory.UsedBytes) / float64(v.Memory.TotalBytes) }`].
- `rx`/`tx`: over `l.Network.Value.Physical` only (D-03). **Decided:** write `rx`/`tx` only when *every* physical link has a non-null rate (a link without a baseline would otherwise under-report for one slot); a one-slot gap is invisible (`segments()` gap threshold is ≥ 45 s).
- `temp:<Chip>/<Label>` and `fan:<Chip>/<Label>` for every entry when `l.Sensors.Readable` — same spelling as sampler.go:259-264.
- No `read`/`write` (the host has no disk-throughput reading).

### Pattern 4: Temperature limits in Go, one function (D-06, D-07)

The browser rule to port verbatim [VERIFIED: web/src/components/charts/Sensors.tsx:24-37]:
```ts
export const TEMPERATURE_DEFAULTS = {
  cpu: { warn: 80, danger: 95 },
  disk: { warn: 60, danger: 70 },
  board: { warn: 70, danger: 85 },
  gpu: { warn: 80, danger: 95 },
  other: { warn: 75, danger: 90 },
} as const satisfies Record<string, { warn: number; danger: number }>

export function temperatureLimits(t: Temperature): { warn: number; danger: number } {
  const fallback: { warn: number; danger: number } =
    TEMPERATURE_DEFAULTS[t.kind as keyof typeof TEMPERATURE_DEFAULTS] ?? TEMPERATURE_DEFAULTS.other
  const danger = t.critical_c ?? fallback.danger
  const warn = t.high_c !== null && t.high_c < danger ? t.high_c : Math.min(fallback.warn, danger)
  return { warn, danger }
}
```
Go: `inventory.TemperatureLimits(kind string, high, crit *float64) (warn, danger float64)` with the same table and branches; `HardwareTemperature` [VERIFIED: internal/inventory/hardware.go:192-202 — fields `Chip`, `Kind`, `Label`, `Celsius`, `HighC *float64 json:"high_c"`, `CriticalC *float64 json:"critical_c"`] gains `WarnC float64 json:"warn_c"` and `DangerC float64 json:"danger_c"`, filled at all three construction sites: `internal/inventory/hardware.go:631` (nodes), `internal/host/sensors.go:108` (zone fallback) and `:191` (hwmon). The guard is the existing `NodeHardware.test.tsx` `temperatureLimits` cases ported into a Go table test, plus a node-page render test showing the ▲ at the server's `warn_c`.

Browser: `temperatureSchema` gains `warn_c: z.number()` and `danger_c: z.number()` — **no `.default()`** (a default would be a browser-side threshold). `Sensors.tsx` uses `t.warn_c`/`t.danger_c` for `severityOf` and the sparkline `max`; `temperatureLimits` is deleted (UI-SPEC: "removed, not kept beside it"). `TEMPERATURE_DEFAULTS` is still imported by `NodeHardware.tsx` for the per-disk `temperature_c` figure (Storage card, not a sensor) — see Open Question 1.

**Host trip points (D-07):** measured on this Pi this session:
```
/sys/class/hwmon/hwmon0 cpu_thermal   files: device name power subsystem temp1_input uevent   (no temp1_max, no temp1_crit)
/sys/class/hwmon/hwmon1 rp1_adc       temp1_input (no max/crit)
/sys/class/thermal/thermal_zone0 cpu-thermal
  trip_point_0 critical 110000
  trip_point_1 active 50000
  trip_point_2 active 60000
  trip_point_3 active 67500
  trip_point_4 active 75000
```
So on the Pi, `cpu_thermal` gets no limit from hwmon; D-07's source "a trip point of type passive or hot" and the specifics' "Gefahr bei 110 °C" both come from the **twin zone** (`talos.ThermalTwinName("cpu-thermal") == "cpu_thermal"` [VERIFIED: internal/talos/sensors.go:677-679]). Implement in `readSensors`: read every zone's `type` and `trip_point_<n>_type`/`_temp` (bounded, e.g. ≤ 16 trips), build `zoneLimits[ThermalTwinName(type)] = {high: lowest passive|hot, crit: lowest critical}`; an hwmon temperature whose chip has no `temp*_max` takes `high` from its twin, none without `temp*_crit` takes `crit`; the zone fallback path takes its own zone's. `active` trips are ignored. Result on the Pi: `high_c: null`, `critical_c: 110`, `warn_c: 80` (cpu default), `danger_c: 110`. The comment at sensors.go:86-89 ("Its trip points are never used as limits…") must be rewritten, and the pi5 fixture (`internal/host/testdata/pi5/sys/devices/virtual/thermal/thermal_zone0/`, which today has only `trip_point_0` = `critical`/`110000`) gains trips 1-4 as `active` 50000/60000/67500/75000 so the test proves "active is not a threshold" (fault: count `active` as `high` → warn 50 → red).

### Pattern 5: The assessment — pure, in `internal/host` (D-06, D-08..D-12)

```go
// internal/host/health.go  (shape decided here; names are the planner's to keep)
type HealthState string
const (
	HealthOK      HealthState = "ok"
	HealthWarn    HealthState = "warn"
	HealthUnknown HealthState = "unknown"
)
type Health struct {
	State      HealthState `json:"state"`
	Summary    string      `json:"summary"`
	Warnings   []string    `json:"warnings"`   // worst first; never null
	Unreadable []string    `json:"unreadable"` // reason sentences; never null
}
func Assess(l Live) Health
```
Rules:
- **Temperatures rated:** `l.Sensors` not readable → unreadable "Temperatures could not be read: {reason.Message}." Readable with zero temperatures → judged by filesystems alone, `ok` summary says so (D-11). Each `t` with `t.Celsius >= t.WarnC` warns; `>= t.DangerC` is "critical".
- **Filesystems rated:** every row of `l.Filesystems` (the root row, and the data-directory row when separate). `Usage` not readable → unreadable "Usage of {mount} could not be read: {reason.Message}." Zero rows → unreadable (defensive; `filesystems()` always returns the root row). Warn when `used*5 >= (used+available)*4` — integer form of `used/(used+available) ≥ 0.8`, df's denominator, the same boundary as the page's meter [VERIFIED: web/src/routes/host.tsx:368 `const ceiling = used + free`, :377 `warn={ceiling * 0.8}`; web/src/components/charts/Meter.tsx:20-24 `if (warn !== undefined && value >= warn) return 'warn'`]. Never compare the rounded-up display percent (79.01 % prints "80%").
- **CPU/memory hidden by hardening do not rate** (D-11).
- **State:** any warning → `warn` (with `Unreadable` still listed: "Also not readable: …"); else any unreadable → `unknown`; else `ok` (D-10).
- **Sentences** (UI-SPEC copy contract): temperature "{sensor} {82.1} °C ≥ {80} °C", past danger "{sensor} {112.0} °C ≥ {110} °C, critical"; filesystem "{mount} {91}% used ≥ 80%" with the percent as df prints it (ceil). Sensor name: chip, then label unless the label is a bare `temp<N>` and it is the chip's only sensor. Order: critical before warning, then by relative excess `(value−threshold)/threshold`, largest first.
- **Summary:** ok → "Temperatures and filesystems are below their thresholds." / no-sensor variant; warn → "1 threshold crossed." / "{n} thresholds crossed."; unknown → the unreadable sentences joined by a space.

Where it appears: `host.View` gains `Health Health json:"health"`, set in `Read` from its own fresh `Live` (the page's ▲ and header never disagree). `Sample` stores `last{at, name, health}` under `c.mu`.

### Pattern 6: `GET /api/v1/host/history` (D-04)

Put it in `HistoryRoutes` beside its siblings [VERIFIED: internal/httpapi/handlers/history.go — routes use `RequiresSession: true`, `MinRole: model.RoleReader`, errors via `historyConfigured` (`upstream.history-unavailable`) and `historyRange` (400 `Validation`, field `range`)]. No inventory check (the host is not in the inventory). Handler: `writeJSON(w, 200, d.History.Query(history.HostSubject(), rg, time.Now()))`. Not audited, not a wall-link route.

Gate lists that must learn the route:
- `cmd/holzkube-managerd/budget_test.go` `noUpstream` list [VERIFIED: lines 1802-1807 name `"GET /api/v1/machines/{id}/hardware/history"` and `"GET /api/v1/host"`] — add `"GET /api/v1/host/history"` with a reason comment.
- `cmd/holzkube-managerd/reachable_test.go` — passes once `web/src` mentions `/api/v1/host/history` (the api.ts function).
- `docs/api-contract.md` — new subsection after "### GET /api/v1/host: read on every call, kept nowhere" (line 1432) and the wall section; `TestEveryProblemCodeIsInTheContract` needs no new code (no new problem type).
- `internal/httpapi/historyapi_test.go` — add the host case (401 without session, 200 reader, 400 bad range, 502 without store).

Browser name: `api.host` is a function (`host: (): Promise<Host> => sendJSON('GET', '/api/v1/host', hostSchema)` [VERIFIED: web/src/api.ts:3092]), so UI-SPEC's `api.host.history(range)` would hang a property off a function — use a sibling `api.hostHistory(range)` (decided).

### Pattern 7: Wall `host` field (D-14)

Verified [VERIFIED: internal/httpapi/handlers/kubernetes.go:521-532]:
```go
		wall, err := client.ForTheWall(ctx, r.URL.Query().Get("namespace"), now)
		...
		writeJSON(w, http.StatusOK, struct {
			kube.Wall
			Trends wallTrends `json:"trends"`
		}{Wall: wall, Trends: trendsForTheWall(r.Context(), d, model.ClusterID(r.PathValue("id")), now)})
```
Add `Host *wallHost `json:"host"`` to that anonymous struct, `hostForTheWall(d.Host, now)` in a new `wall_host.go`:
```go
type wallHost struct {
	Name   string           `json:"name"`   // hostname; "holzkube-manager host" when not readable
	State  host.HealthState `json:"state"`  // ok | warn | unknown
	Reason string           `json:"reason"` // "healthy" | first warning [+ " and {n} more"] | "not readable"
}
```
- `d.Host == nil` or no sample yet → `nil` (JSON `null`) → no tile (UI-SPEC).
- `now − last.at > 3*history.FineStep` (45 s) → `unknown`, reason "not readable" (D-11's third clause; the `/host` page can show the sentence "The last reading is older than 45 s." — the wall shows only the short reason).
- The unknown reason on the wall is the fixed "not readable", never `Unreadable[...]` (those carry error text with paths).
- Wall states are the wall's own [VERIFIED: internal/kube/wall.go:48-66 `StateOK State = "ok"`, `StateWarn State = "warn"`, `StateDown State = "down"`, `StateStopped State = "stopped"`, `StateUnknown State = "unknown"`]; the host uses only `ok`/`warn`/`unknown`, which exist in `TILE_COLOURS` [VERIFIED: web/src/routes/wall.tsx:104-110 `ok: 'bg-green-700 …'`, `warn: 'bg-amber-700 …'`, `unknown: 'bg-zinc-600 …'`].
- `kube.Wall.Nodes` and `Summary` are not touched (D-14).
- If the kube client fails, the wall route errors as today and no host tile appears (the whole screen says "The cluster could not be asked.") — accepted.

Browser: `wallSchema` gains `host: z.object({name: z.string(), state: z.string(), reason: z.string()}).nullish().transform(v => v ?? null)`. `LeftHalf` prepends `{kind: 'Host', namespace: '', name, state, detail: \`manager · ${reason}\`}` to the tiles passed to `<Named title="Nodes">`, so `Named`'s size class counts it. In `Named`: `data-kind={tile.kind === 'Host' ? 'host' : undefined}` and skip `NodeTrend` when `tile.kind === 'Host'` (a node sharing the name must not lend its curve). The headline's node count keeps reading `wall.nodes`.

### Pattern 8: Sidebar mark without skewing the page (D-13)

Verified that react-query gives **each observer its own interval timer** [VERIFIED: web/node_modules/@tanstack/query-core/build/modern/queryObserver.js:158-165 `this.#refetchIntervalId = timeoutManager.setInterval(() => { … this.#executeFetch(); }, this.#currentRefetchInterval);`] and only dedupes a fetch that is already in flight [VERIFIED: query.js:157-162]. So a sidebar observer at 30 s beside `/host`'s 3-s observer adds an extra `GET /api/v1/host` at an arbitrary phase, shortening the next page window. **Decided:** the sidebar's `useQuery({ queryKey: ['host'], queryFn: api.host, retry: false, refetchInterval: onHostPage ? false : 30_000 })`, with `onHostPage` from `useRouterState({ select: s => s.location.pathname === '/host' })`. On `/host` the page's poll feeds the mark; elsewhere 30 s. With Pattern 3 the sampler cannot be affected at all.

States: no data and no error → no mark; `query.error` set → hollow ring + sr-only " — not readable, holzkube-manager did not answer" (never a green dot kept from before); else `data.health.state`. The Sidebar renders only inside `AppShell` under the authenticated route (`__root.tsx:79 component: AppShell`), never on the wall/wall-link screen, so a wall link can never trigger a 401 redirect from this query. `WhatsNew` already uses `useQuery` inside the Sidebar, so every render path has a `QueryClientProvider`.

Cost: each open tab reads the full host every 30 s, including the data-directory walk that `dirSizer` caches for 60 s — at most one walk a minute per daemon. Acceptable on the Pi; noted.

### Pattern 9: `/host` charts (D-05, UI-SPEC)

- Move, not copy: from `NodeHardware.tsx` extract `ProcessorChart` (`LiveChart` "Processor load", key `cpu`, `yMax 100`, `height 160`), `CoreList` (per-core figure + ▲ + `Sparkline` of `core:{i}`, `grid grid-cols-2 gap-x-4 gap-y-2 sm:grid-cols-3 xl:grid-cols-6`), `MemoryChart` ("Memory in use", key `memory`, `yMax 100`), `NetworkChart` ("Network throughput", `rx` slot 2 "Inbound", `tx` slot 1 "Outbound", `formatRate`). `NodeHardware` imports them back; its tests stay green unchanged (pixel-identical).
- `/host`: `useLiveSeries('host', host, host.observed_at, pick)` where `pick` writes only readable keys (mirror of `HistoryValues`); `useChartRange(['host'], api.hostHistory)`; `merge(chart.history.data, live, chart.windowMs, Date.parse(host.observed_at))`; `Sensors` gets `history`.
- "Not readable — no history": show the sentence when the current reading is not readable **and** the merged series for the key has 0 points in the window. Treat `rate.no-baseline` as *pending*, not unreadable (render `LiveChart` with its own "The curve starts now…" overlay), or the first poll after a restart flashes "Not readable — no history".
- Rename `h2` "Live" → "Readings", `id="host-readings"`; update `host.test.tsx:400` which asserts the heading name `'Live'`.

### Anti-Patterns to Avoid
- **Sampler calling `Collector.Read`:** corrupts the page's rate windows and walks the data directory every minute forever.
- **Sampling the host after the inventory call:** the host history stops whenever the store is unreadable.
- **Writing 0 for an unreadable value:** breaks HMON-06 and the ring semantics; the key must be absent.
- **A `.default()` on `warn_c`/`danger_c` or `health` in zod:** reintroduces browser-side thresholds/states.
- **Comparing the df display percent (ceil) with 80:** warns at 79.01 %; compare the exact ratio.
- **Host as a `kube.Tile` in `wall.nodes`:** changes `summary` and the "{n} nodes" headline (D-14 rejected).
- **Putting error text into the wall reason:** a wall link on a corridor TV would show `Could not read /sys/...`.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Storing/persisting a time series | a new file, ring or ticker | `history.Store.Record/Query`, `FlushIfDue`, `Flush` | D-02; gaps, tiers, atomic write, restart survival already proven |
| Gap drawing | a NaN/zero marker in the browser | `LiveChart` `segments()` (gap = > max(45 s, 3× median step)) [VERIFIED: LiveChart.tsx:296-312] | already correct for missing points |
| History + live splice | a second merge | `merge()` in `useChartRange.ts` | same as node page |
| Chip classification, twin names | new string rules | `talos.ClassifyChip`, `talos.ThermalTwinName`, `talos.Millidegrees` | one rule set with the node path |
| Range parsing | a new parser | `history.ParseRange`, `historyRange()` | same 400 problem shape |
| Temperature defaults | a second table in the browser | `inventory.TemperatureLimits` | D-06 one implementation |
| Wall tile colour/sizing | a new tile component | `Named` + `TILE_COLOURS` | size class computed across all tiles |
| Missing value rendering | new "n/a" text | `MissingValue` (host.tsx) | the only way a missing value is drawn |

**Key insight:** every hard property here (gap semantics, restart survival, df arithmetic, chip rules) is already implemented and guarded once; the risk in this phase is a *second* implementation that drifts, not a missing one.

## Runtime State Inventory

Not a rename phase. One runtime effect worth stating: after the next release the production daemon's `/var/lib/holzkube-manager/history/metrics.bin` gains the subject `host/local`. The file format (`fileVersion = 1`) is unchanged, so no migration. A downgrade to v0.1.0 keeps `host/local` (unknown keys are retained) until it ages out after 24 h — harmless. Stored data / live service config / OS-registered state / secrets / build artifacts: none affected — verified by reading `persist.go` (format) and `main.go` (no new flags, no new files).

## Common Pitfalls

### Pitfall 1: Shared rate memo
**What goes wrong:** sampler and page shorten each other's rate windows; history CPU% becomes a 1-3 s value while the page is open.
**Why:** `c.prev` is advanced by every read ≥ 0.5 s apart (collector.go:327-329).
**How to avoid:** separate baselines (Pattern 3). **Test:** a fake clock: page reads at 0, 3, 6 s; a `Sample` at 4.5 s; the 6-s page read reports `rates_over_seconds == 3.0`, and the next `Sample` at 19.5 s reports a 15-s window. Fault: pass `&c.prev` from `Sample` → red.

### Pitfall 2: Host skipped when the inventory fails
**What goes wrong:** `pass` returns before any sampling on an inventory error (sampler.go:145-149).
**How to avoid:** sample the host first. **Test:** inventory stub returns an error; the host series still gets a point. Fault: move `sampleHost` after the inventory → red.

### Pitfall 3: `useLiveSeries.append` drops a key that is absent from one reading
[VERIFIED: web/src/hooks/useLiveSeries.ts:44-53 — `const next: Series = {}` … `for (const [name, value] of Object.entries(values))` … `store.set(key, next)`]. A key missing from a reading (a sensor that failed one read, CPU turning unreadable) loses its whole live curve, not just one point. For `/host` this is mostly harmless (history still carries it), but the planner should decide explicitly; recommended: carry forward absent keys' kept points (cut to the window). Gaps then show because no point exists for that time. This changes node behaviour only in the case where a key disappears — node tests must stay green.

### Pitfall 4: Boundary disagreement meter vs warning
Meter: `value >= ceiling*0.8` on `used` with `ceiling = used + free`. Server must use the same denominator and `>=`. A test at exactly 80.0 % (e.g. used 80, avail 20) and at 79.99 % must agree between `Assess` and what the meter would draw (UI-SPEC "consistency" row).

### Pitfall 5: Active trip points as thresholds
The Pi has four `active` trips (50/60/67.5/75 °C). Counting them as `high` would warn at 50 °C permanently (D-07). Test with the extended pi5 fixture.

### Pitfall 6: Wall host name leaking the real hostname
Fixtures and tests must use `manager-01.homelab.example`/`example-host`; any live check on the Pi prints only field names and states, never the hostname (as Phase 11's verification did).

### Pitfall 7: "Not readable — no history" on the first poll
`rate.no-baseline` is `readable: false`; treat it as pending (Pattern 9).

### Pitfall 8: README screenshot script and audit do not serve host history yet
[VERIFIED: web/scripts/readme-images.mjs] `history(url)` branches only on `/apps/` vs node, and `live()` refreshes `observed_at` only for paths ending `/hardware`. Add a host branch (series `rx`, `tx`, `temp:cpu_thermal/temp1`, `temp:rp1_adc/temp1`, with a ≥ 5-min hole) and refresh `/api/v1/host`'s `observed_at`. The layout audit serves fixtures by pathname only (`FIXTURES[path]`, query ignored) and otherwise lets the request through to its real dev daemon — which, from this phase on, samples **the machine running the audit**. Add `/api/v1/host/history` to `demo.json` so the audit is deterministic and never draws a real host's values.

### Pitfall 9: 12 px tracking root cause is unmeasured
The body has `letter-spacing: var(--hc-track-snug)` = `-0.015em` [VERIFIED: web/src/index.css:117, 364]. The Phase 11 screenshot (docs/screenshots/host.png, 1440×900 at DPR 1) shows collapsed word spaces in 12-px lines ("412MiB·measured23sago"). −0.015 em is −0.18 px at 12 px, which alone should not swallow a space; Manrope's narrow space plus DPR-1 hinting likely contributes [ASSUMED]. Plan a measurement first (Chromium, 12-px `text-xs` span: width of "a b" minus "ab" with and without the rule), then the minimal fix (e.g. `letter-spacing: normal` for `text-xs`), then a browser test that holds the measured space advance — seen red with the old rule.

### Pitfall 10: Known unrelated flake in the full gate
`internal/upgrade TestANodeSaysHowItBooted` times out under full-suite load (Phase 11 verification, truth #20); it passes alone. Re-run the package alone before reading a red `task ci` as this phase's.

## Code Examples

### Sampler host step (shape)
```go
// internal/history/sampler.go — inside pass(), right after `at := s.deps.Now()`
if s.deps.Host != nil {
	s.sampleHost(ctx, at)
}

func (s *Sampler) sampleHost(ctx context.Context, at time.Time) {
	key := HostSubject()
	values, err := s.deps.Host(ctx)
	if err != nil {
		s.failed(key, "the host could not be read; its charts have a gap until it can", err)
		return
	}
	s.recovered(key, "the host can be read again")
	s.deps.History.Record(key, at, values)
}
```

### Restart + gap test (success criterion 2), modelled on `TestTheHistorySurvivesARestart`
```go
// internal/history/persist_test.go pattern [VERIFIED: persist_test.go:44-76 uses
// Open(path, fsstore.ReadFile, fsstore.WriteFileAtomic, t0, quiet()), Record, Flush(now), Open again, allRanges]
before := Open(path, fsstore.ReadFile, fsstore.WriteFileAtomic, t0, quiet())
for at := t0; !at.After(now); at = at.Add(FineStep) {
	v := map[string]float64{"temp:cpu_thermal/temp1": 50}
	if !(at.After(t0.Add(20*time.Minute)) && at.Before(t0.Add(40*time.Minute))) {
		v["cpu"] = 12 // readable outside the hole
	}
	before.Record(HostSubject(), at, v)
}
// Flush, reopen, then: cpu has no point inside (20m,40m) in 1h/6h/24h; temp has all;
// no cpu point equals 0. Fault: HistoryValues writing cpu=0 when unreadable → red
// (tested at the host.HistoryValues level) ; encode dropping the host subject → red.
```

### Sidebar mark data
```tsx
const onHostPage = useRouterState({ select: (s) => s.location.pathname === '/host' })
const host = useQuery({
  queryKey: ['host'],
  queryFn: api.host,
  retry: false,
  refetchInterval: onHostPage ? false : 30_000,
})
const state = host.error ? 'unanswered' : host.data?.health.state // undefined → no mark
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Temperature limits computed in the browser (`temperatureLimits`) | Server sends `warn_c`/`danger_c` per sensor | this phase (D-06) | node page and host page and wall share one rule |
| `/host` "Nothing on this page is stored" | host recorded every 15 s, 24 h, through restarts | this phase | copy changes (UI-SPEC "Readings" line, footer) |
| Zone trip points never used as limits (sensors.go:86-89) | `passive`/`hot` → high, `critical` → danger; `active` ignored | this phase (D-07) | Pi danger line moves from 95 (cpu default) to 110 |

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | The critical trip point (110 °C on the Pi) should become the danger limit; CONTEXT states it under `<specifics>`, D-07 names only the warn sources | Pattern 4 | Danger/"critical" wording at 110 instead of 95; sparkline max 110. Easy to flip: drop `critical` from the trip map |
| A2 | "alle Temperaturen und/oder die Belegung der beiden Dateisysteme" means: unknown if the sensors reading is not readable OR any filesystem row's usage is not readable | Pattern 5 | A hidden data-directory row would otherwise let a full disk read as healthy — the chosen reading is the safer one |
| A3 | `rx`/`tx` are written only when every physical link has a rate | Pattern 3 | One-slot gaps when a link appears; invisible (< 45 s) |
| A4 | The 12-px word-spacing collapse is partly font/hinting, not only `-0.015em` | Pitfall 9 | The fix might need `word-spacing` rather than letter-spacing — the measurement task decides |
| A5 | `/api/v1/host` states from its own fresh reading while the wall uses the ≤ 15-s-old sampler snapshot; they may differ for up to one interval | Pattern 5/7 | A crossing is on the page up to 15 s before the wall; acceptable, stated in the guide |

## Open Questions (RESOLVED)

1. RESOLVED (planner: drives get warn/danger from the same Go rule, 12-02/12-04) — **The node page's per-disk temperature figure** (`NodeHardware.tsx` Storage card, `TEMPERATURE_DEFAULTS.disk`)
   - What we know: it is a `HardwareDisk.temperature_c`, not a sensor row; D-06 speaks of sensors.
   - What's unclear: whether it must also come from the server.
   - Recommendation (decided): keep a single browser constant `DISK_TEMPERATURE = { warn: 60, danger: 70 }` for that figure only, with a comment pointing at the Go table and a Go test that asserts the Go `disk` defaults are 60/70 — or, cheaper and cleaner, add `warn_c`/`danger_c` to `HardwareDisk` from the same Go function. Planner picks the second if it fits the node plan; either keeps one source of numbers.
2. DEFERRED (carried, not fixed in this phase) — **Two chips with the same name** (IN-05 from Phase 11): `temp:<chip>/<label>` keys collide, so two NVMe drives share one history series on the host. Not the Pi; carried, not fixed here.
3. DEFERRED (human item after the operator's next release) — **Production sighting:** the Pi's real service still runs v0.1.0; host history, warning and tile on the real service are visible only after a release the operator cuts. Carry as a human item, as Phase 11 did.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | backend, tests | ✓ | go1.27.1 local; go1.26.7 via `GOTOOLCHAIN` | — |
| Node / npm | web tests, build, audit | ✓ | v22.23.3 / 10.9.9 (nvm) | — |
| Playwright Chromium | `test:browser`, layout audit, README images | ✓ | chromium-1243 in `~/.cache/ms-playwright` | — |
| `./bin/task`, `./bin/golangci-lint`, `./bin/holzkube-managerd` | gate | ✓ | present in `bin/` | — |
| `go test -race` | race detector | ✗ on the Pi | — | CI's alone (none runs now) — state it |
| Real hwmon/thermal | live check of limits | ✓ (read-only) | Pi 5, values above | fixtures |

Baseline measured this session: `GOTOOLCHAIN=go1.26.7 go test -count=1 ./internal/history/ ./internal/host/ ./internal/httpapi/handlers/` printed `ok` for all three packages (handlers 57 s).

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` (go1.26.7); vitest 4.1.11 projects `jsdom` + `browser` (Playwright Chromium); layout audit `web/scripts/layout-audit.mjs` |
| Config file | `web/vite.config.ts` (vitest projects); `Taskfile.yml` |
| Quick run command | `GOTOOLCHAIN=go1.26.7 go test -count=1 ./internal/history/ ./internal/host/... ./internal/inventory/ ./internal/httpapi/... ./cmd/holzkube-managerd/` and `npm --prefix web exec -- vitest run --project jsdom src/routes/host.test.tsx src/routes/wall.test.tsx src/components/NodeHardware.test.tsx src/components/charts/Sensors.test.tsx src/fixtures.test.ts` |
| Full suite command | `GOTOOLCHAIN=go1.26.7 go test -count=1 ./internal/...` ; `npm --prefix web run test` ; `./bin/task test:layout` ; before a push `./bin/task ci` |

Run Go with `export PATH=$HOME/.local/go/bin:$PATH`. Read the exit code of the command itself (`; echo rc=$?` directly after it, no pipe).

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| HMON-05 | `host/local` retained by `Retain` with empty inventories; pruned after 24 h | unit | `go test ./internal/history -run 'TestTheHostIsNeverForgotten'` | ❌ Wave 0 |
| HMON-05 | sampler records host keys at the pass timestamp; host sampled when inventory fails | unit | `go test ./internal/history -run 'TestTheSamplerRecordsTheHost\|TestTheHostIsSampledWhenTheInventoryFails'` | ❌ |
| HMON-05 | `HistoryValues`: unreadable cpu/memory/core absent (never 0); rx/tx physical only | unit | `go test ./internal/host -run TestHistoryValues` | ❌ |
| HMON-05 | sampler baseline separate from page baseline | unit | `go test ./internal/host -run TestSampleHasItsOwnBaseline` | ❌ |
| HMON-05 | write, flush, reopen: history kept, unreadable stretch is a gap | unit | `go test ./internal/history -run TestTheHostHistorySurvivesARestartWithItsGap` | ❌ |
| HMON-05 | `GET /api/v1/host/history`: 401/200/400/502 | integration | `go test ./internal/httpapi -run TestHostHistoryAPI` | ❌ (extend historyapi_test.go) |
| HMON-05 | route in budget and reachable lists | gate | `go test ./cmd/holzkube-managerd -run 'TestEveryRoute'` | ✅ (lists to extend) |
| HMON-05 | `/host` charts; "Not readable — no history"; gap drawn as two runs | component | `vitest run --project jsdom src/routes/host.test.tsx` | ✅ (extend) |
| HMON-05 | NodeHardware unchanged after extraction | component | `vitest run --project jsdom src/components/NodeHardware.test.tsx` | ✅ |
| HMON-07 | `TemperatureLimits` = ported browser rule (all old cases) | unit | `go test ./internal/inventory -run TestTemperatureLimits` | ❌ |
| HMON-07 | Pi trips: active ignored, critical → danger 110, warn 80 | unit | `go test ./internal/host -run TestTripPoints` | ❌ (extend pi5 fixture) |
| HMON-07 | warning names value and threshold; boundary 80 % and 80.0 °C; worst first | unit | `go test ./internal/host -run TestAssessWarns` | ❌ |
| HMON-07 | notice renders server sentences in order; ▲ from `warn_c` | component | `vitest run --project jsdom src/routes/host.test.tsx src/components/charts/Sensors.test.tsx` | ✅ (extend) |
| HOST-04 | unreadable rated values never `ok` (table over combinations) | unit | `go test ./internal/host -run TestUnreadableIsNeverHealthy` | ❌ |
| HOST-04 | wall `host`: null without sample; stale > 45 s → unknown; reason text | unit | `go test ./internal/httpapi/handlers -run TestTheWallsHost` | ❌ |
| HOST-04 | sidebar mark: none / unanswered / three shapes with sr-only text | component | `vitest run --project jsdom src/components/Sidebar.test.tsx` | ❌ |
| HOST-04 | wall host tile first in Nodes, `data-kind="host"`, no trend, headline count unchanged, absent → no tile | component | `vitest run --project jsdom src/routes/wall.test.tsx` | ✅ (extend) |
| all | fixtures parse with the product schemas | component | `vitest run --project jsdom src/fixtures.test.ts` | ✅ (add `/api/v1/host/history`) |
| all | `/host` and `/wall` fit 390 px and 1280 px, tap targets ≥ 44 px | e2e | `./bin/task test:layout` | ✅ |
| all | warn shape of `/host` at 390 px | browser | `npm --prefix web run test:browser -- src/routes/host.browser.test.tsx` | ✅ (extend) |

### Fault-injection table (each must be seen red, then restored; `git diff` empty afterwards)
| Guard | Fault to reinstate | Expected red |
|-------|--------------------|--------------|
| host retained | `HostSubject()` returns `"machine/local"` | `TestTheHostIsNeverForgotten` |
| host sampled despite inventory failure | call `sampleHost` after the inventory's early return | `TestTheHostIsSampledWhenTheInventoryFails` |
| no zero for unreadable | `HistoryValues` writes `cpu = 0` when `Usage` not readable | `TestHistoryValues`, restart/gap test |
| physical-only network | sum `Virtual` too in `rx` | `TestHistoryValues` |
| separate baselines | `Sample` passes `&c.prev` | `TestSampleHasItsOwnBaseline` |
| restart survival | encode skips `host/local` (e.g. filter the key in `encodeLocked`) | restart/gap test |
| threshold check (D-09) | replace `t.Celsius >= t.WarnC` with `false` (and separately the filesystem check) | `TestAssessWarns` |
| df denominator | use `used/size` instead of `used/(used+avail)` | `TestAssessWarns` boundary case |
| never ok when unreadable (D-12) | `Assess` ignores `Unreadable` (returns `ok`) | `TestUnreadableIsNeverHealthy` |
| active trips ignored (D-07) | treat `active` as `high` | `TestTripPoints` |
| ported limits | drop the `high_c < danger` guard | `TestTemperatureLimits` |
| wall staleness | skip the `> 3*FineStep` check | `TestTheWallsHost` |
| wall not in summary | append the host to `wall.Nodes` | `TestTheWallsHost` (asserts `nodes`/`summary` unchanged) + `wall.test.tsx` headline count |
| sidebar never green on failure | render last `data.health` when `error` is set | `Sidebar.test.tsx` |
| no browser thresholds | reinstate `temperatureLimits` in `Sensors` (ignore `warn_c`) with a sensor whose server `warn_c` differs from the default | `Sensors.test.tsx` |
| "Not readable — no history" | render `LiveChart` with empty points for an unreadable key | `host.test.tsx` |
| 390-px warning list | drop `break-words` on the warning `ul` with a long mount path | `host.browser.test.tsx` |

### Sampling Rate
- **Per task commit:** the quick commands above for the touched package/files.
- **Per wave merge:** `GOTOOLCHAIN=go1.26.7 go test -count=1 ./internal/...` and `npm --prefix web run test`.
- **Phase gate:** `./bin/task ci` green (re-run `internal/upgrade` alone if only `TestANodeSaysHowItBooted` times out), plus a dev daemon on the Pi (127.0.0.1, `mktemp -d` data dir): `/api/v1/host` `health` present, `/api/v1/host/history` gains points every 15 s, restart the **dev** daemon and see the history kept with a gap; wall answer carries `host` (only states/field names printed, never the hostname).

### Wave 0 Gaps
- [ ] `internal/host/health_test.go` — Assess tables (HMON-07, HOST-04)
- [ ] `internal/host/sample_test.go` — HistoryValues, separate baseline, Latest staleness
- [ ] `internal/inventory/limits_test.go` — ported limit cases
- [ ] `internal/history` tests — host subject, sampler host, restart/gap
- [ ] `internal/httpapi/handlers/wall_host_test.go`
- [ ] `web/src/components/Sidebar.test.tsx` (new file)
- [ ] pi5 fixture: trip points 1-4 (`active`)
- [ ] `web/fixtures/demo.json`: `health` on `/api/v1/host` (warn, `cpu_thermal 82.1 °C ≥ 80 °C`, reading 82.1), `warn_c`/`danger_c` on every temperature (host and node fixture), `/api/v1/host/history`, wall `host`

## Security Domain

`security_enforcement: true`, ASVS level 1.

### Applicable ASVS Categories
| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | yes (unchanged) | session gate on the new route (`RequiresSession: true`) |
| V3 Session Management | no change | — |
| V4 Access Control | yes | `MinRole: model.RoleReader`; the new route is **not** `WallLink`; `TestOnlyTheWallAcceptsAWallLink` must stay green |
| V5 Input Validation | yes | `range` via `history.ParseRange` (400 on anything but 1h/6h/24h); sysfs reads bounded (`readBounded`, trip-point count cap) |
| V6 Cryptography | no | — |
| V8 Data Protection | yes | wall/wall-link disclosure: name, state, short reason only; no address, no version, no error text |

### Known Threat Patterns
| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Wall link reading more than the wall | Information disclosure | host data rides inside the one wall route; no new wall-link route |
| Error messages with paths on a public screen | Information disclosure | wall reason for `unknown` is the fixed "not readable" |
| Real identifiers in fixtures/screenshots | Information disclosure (public repo) | documentation names only; `internal/publicrepo` gate |
| Unbounded sysfs read (trip points) | DoS | cap trips per zone, reuse `maxSmallFile` |
| Extra polling load from every tab | DoS (self) | sidebar 30 s, off on `/host`; data-dir walk cached 60 s |

## Sources

### Primary (HIGH confidence — read this session)
- `internal/history/{history,sampler,persist}.go` and tests — subject keys, `retained`, `Record`, sampler flow, file format, `FlushEvery = 30 * time.Minute`
- `internal/host/{host,collector,sensors,filesystems,network}.go`, `testdata/pi5` — Reading, rate memo, sensor walk, df arithmetic
- `internal/inventory/hardware.go` — `HardwareTemperature`, `flattenSensors`
- `internal/talos/sensors.go` — `ClassifyChip`, `ThermalTwinName`
- `internal/httpapi/handlers/{history,host,kubernetes,wall_trends}.go`, `internal/httpapi/router.go`, `internal/kube/wall.go`
- `cmd/holzkube-managerd/{main.go,budget_test.go,reachable_test.go}`
- `web/src/{api.ts,routes/host.tsx,routes/wall.tsx,components/NodeHardware.tsx,components/Sidebar.tsx,components/charts/*,hooks/*}`, `web/scripts/{readme-images,layout-audit}.mjs`, `web/fixtures/demo.json`, `web/src/index.css`
- `web/node_modules/@tanstack/query-core/build/modern/{queryObserver,query}.js` v5.102.6 — per-observer interval, in-flight dedupe
- Live Pi `/sys/class/hwmon`, `/sys/class/thermal` (read-only)
- `.planning/phases/12-host-verlauf/12-CONTEXT.md`, `12-UI-SPEC.md`; `.planning/phases/11-host-seite/11-VERIFICATION.md`, `11-07-SUMMARY.md`, `11-UI-REVIEW.md`; `.planning/REQUIREMENTS.md`, `STATE.md`, ROADMAP phase 12

### Secondary / Tertiary
- none (no web sources were needed: no new library, all behaviour in-repo)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — no additions; versions read from package files
- Architecture: HIGH — every integration point read; the two structural risks (shared memo, early return) found in code
- Pitfalls: HIGH for 1-8 and 10 (code-verified); MEDIUM for 9 (root cause of the tracking defect unmeasured)

**Research date:** 2026-09-28
**Valid until:** 2026-10-28 (in-repo facts; re-check if `internal/history` or `internal/host` change first)
