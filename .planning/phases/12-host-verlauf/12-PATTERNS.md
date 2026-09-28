# Phase 12: Host wie ein Knoten — Verlauf, Warnung, Wand - Pattern Map

**Mapped:** 2026-09-28
**Files analyzed:** 22
**Analogs found:** 21 / 22

All analogs are git-tracked source. RESEARCH.md Patterns 1-9 already carry verified line refs; this map adds the concrete analog excerpts the planner copies from.

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match |
|---|---|---|---|---|
| `internal/history/history.go` (HostSubject, retained case) | model | CRUD | `MachineSubject`, `retained` (history.go:127-128, 402-414) | exact |
| `internal/history/sampler.go` (SamplerDeps.Host, sampleHost) | service | batch | `sampleMachine` (sampler.go:190-199) | exact |
| `internal/history/*_test.go` (host restart+gap, inventory-fails) | test | file-I/O | `persist_test.go:44-76` `TestTheHistorySurvivesARestart` | exact |
| `internal/inventory/limits.go` (TemperatureLimits) + test | utility | transform | `web/src/components/charts/Sensors.tsx:24-37` (port) | exact (cross-language) |
| `internal/inventory/hardware.go` (WarnC/DangerC, flattenSensors ~631) | model | transform | same file, `HardwareTemperature` 192-202 | exact |
| `internal/host/sensors.go` (trip points, limits at :108, :191) | service | file-I/O | same file; `talos.ThermalTwinName` (internal/talos/sensors.go:677) | exact |
| `internal/host/testdata/pi5/.../thermal_zone0/trip_point_{1..4}_*` | test fixture | file-I/O | existing `trip_point_0_*` there | exact |
| `internal/host/health.go` (Assess) + `health_test.go` | service (pure) | transform | `internal/kube/wall.go:48-66` states; table tests in `internal/host/*_test.go` | role-match |
| `internal/host/sample.go` (Sample, HistoryValues, Latest) | service | request-response | `Collector.Read` in `collector.go`; `HardwareValues` sampler.go:~230-265 | exact |
| `internal/host/collector.go` (rates takes baseline slot) | service | transform | same file 323-329 | exact |
| `internal/httpapi/handlers/history.go` (GET /api/v1/host/history) | route/controller | request-response | `machineHistory` (history.go:62-90) | exact |
| `internal/httpapi/historyapi_test.go` (host cases) | test | request-response | existing machine cases in same file | exact |
| `internal/httpapi/handlers/wall_host.go` | controller helper | request-response | `wall_trends.go` (1-60) | exact |
| `internal/httpapi/handlers/kubernetes.go` (wall struct gains Host) | controller | request-response | same file 521-532 | exact |
| `cmd/holzkube-managerd/main.go` (Host: hostCollector.Sample; Deps.Host) | config/wiring | — | main.go:649-667 sampler literal | exact |
| `cmd/holzkube-managerd/budget_test.go` noUpstream | test | — | lines 1802-1807 | exact |
| `web/src/components/charts/HardwareCharts.tsx` | component | request-response | extract from `NodeHardware.tsx:150-195, 204-210, 255-263` | exact (move) |
| `web/src/components/HostState.tsx` (HostStateMark) | component | — | P{n} chip slot in `Sidebar.tsx:236-247`; icon use in `charts/Meter.tsx` | role-match |
| `web/src/components/charts/Sensors.tsx` (use warn_c/danger_c, delete temperatureLimits) | component | transform | same file | exact |
| `web/src/routes/host.tsx` (+ tests) | route/component | request-response (poll) | `NodeHardware.tsx:46-140` (useLiveSeries/useChartRange/merge wiring) | exact |
| `web/src/components/Sidebar.tsx` | component | request-response (poll) | `WhatsNew` useQuery inside Sidebar | role-match |
| `web/src/routes/wall.tsx` (host tile in Named) | component | request-response | `Named` 399-482 | exact |
| `web/src/api.ts` (hostHistory, schemas) | client | request-response | `hardwareHistory` api.ts:3314; `host` api.ts:3092 | exact |
| `web/src/index.css` 12 px tracking fix | config (style) | — | `--hc-track-snug` index.css:117, 364 | no analog for the measurement test |
| `web/fixtures/demo.json`, `web/scripts/readme-images.mjs` | fixture/script | batch | existing `/api/v1/host`, node `history(url)` branch | exact |
| `docs/api-contract.md`, README, `docs/guide.md` | docs | — | "### GET /api/v1/host" section (line ~1432) | exact |

## Pattern Assignments

### `internal/history/sampler.go` — copy `sampleMachine` (lines 190-199)
```go
func (s *Sampler) sampleMachine(ctx context.Context, id model.MachineID, at time.Time) {
	key := MachineSubject(id)
	view, err := s.deps.Hardware(ctx, id)
	if err != nil {
		s.failed(key, "a node did not answer the hardware read; its charts have a gap until it does", err)
		return
	}
	s.recovered(key, "a node answers the hardware read again")
	s.deps.History.Record(key, at, HardwareValues(view))
}
```
`sampleHost` is this with `HostSubject()`, `s.deps.Host(ctx)`, and values passed straight to `Record`. Call it in `pass` right after `at := s.deps.Now()`, BEFORE the `Inventory` early return (sampler.go:139-150). Guarded by `if s.deps.Host != nil`.

### `internal/history/history.go` — RESEARCH Pattern 1 excerpt
Add `func HostSubject() string { return "host/local" }` beside `MachineSubject` (127-128); first line in `retained` (402): `if key == HostSubject() { return true }`. Red-fault: HostSubject returning `"machine/local"`.

### `internal/httpapi/handlers/history.go` — copy `machineHistory` (62-90) minus inventory
Route entry, copy lines 34-40:
```go
{
	Method:          http.MethodGet,
	Pattern:         "/api/v1/machines/{id}/hardware/history",
	RequiresSession: true,
	MinRole:         model.RoleReader,
	Handler:         handler(machineHistory(d)),
},
```
Handler body to keep: `historyConfigured(d)` → `WriteProblem`; `rg, ok := historyRange(w, r)`; then `writeJSON(w, http.StatusOK, d.History.Query(history.HostSubject(), rg, time.Now()))`. Drop `inventoryConfigured` and `MachineRecord`.

### `internal/httpapi/handlers/wall_host.go` — copy shape of `wall_trends.go`
Same package/import layout (`context`, `time`, `internal/history`, `internal/httpapi`, `internal/model`; add `internal/host`), a doc-commented response struct, and a nil-safe builder:
```go
func trendsForTheWall(ctx context.Context, d httpapi.Deps, cluster model.ClusterID, now time.Time) wallTrends {
	out := wallTrends{...}
	if d.History == nil || d.Inventory == nil {
		return out
	}
```
`hostForTheWall(d, now) *wallHost` returns nil when `d.Host == nil` or no sample; `now-last.at > 3*history.FineStep` → `unknown`/"not readable". Embed in `kubernetes.go:521-532` anonymous struct as `Host *wallHost \`json:"host"\``.

### `internal/inventory/limits.go` — port `Sensors.tsx:24-37` rule for rule (see RESEARCH Pattern 4). Test table = the `temperatureLimits` cases in `NodeHardware.test.tsx`, moved to Go.

### `internal/host/health.go` — RESEARCH Pattern 5 (types, rules, sentences). States reuse the string values of `kube.StateOK/StateWarn/StateUnknown` (wall.go:48-66) so `TILE_COLOURS` matches. Disk rule `used*5 >= (used+available)*4`.

### `internal/host/sample.go` + `collector.go` — RESEARCH Pattern 3. `rates(...)` gets a `**counters` baseline param; `Read` passes `&c.prev`, `Sample` `&c.samplePrev`, both under `c.mu`. `HistoryValues` mirrors `HardwareValues` keys (`cpu`, `memory` = 100*used/total, `rx`/`tx` physical only, `temp:<chip>/<label>`, `fan:<chip>/<label>`, `core:<n>`); unreadable ⇒ key absent.

### `web/src/components/charts/HardwareCharts.tsx` — move from `NodeHardware.tsx`
Processor (150-157):
```tsx
<LiveChart
  title="Processor load"
  series={[{ key: 'cpu', label: 'Load', slot: 1, points: history.cpu ?? [] }]}
  format={(v) => `${Math.round(v)}%`}
  yMax={100}
  height={160}
  windowMs={chart.windowMs}
/>
```
CoreList (158-185: `grid grid-cols-2 gap-x-4 gap-y-2 sm:grid-cols-3 xl:grid-cols-6`, `severityOf(usage, 75, 90)`, ▲, `Sparkline` of `core:${i}`), MemoryChart (204-210, key `memory`, `yMax 100`), NetworkChart (255-263, `rx` Inbound slot 2, `tx` Outbound slot 1, `formatRate`). Props: `history`, `windowMs`, (cores: `perCore`). `NodeHardware` imports them back; its tests stay unchanged. Add the "Not readable — no history" slot at the call site in host.tsx (or an optional `unreadable` prop), not in node rendering.

### `web/src/components/HostState.tsx` — slot + style from `Sidebar.tsx:236-247`
```tsx
<span className="flex-1">{area.label}</span>
{area.phase !== null && ( <span className="rounded border ... text-[10px]" title=...>P{area.phase}</span> )}
```
Mark goes in that slot for the Host entry; shapes per UI-SPEC "The state mark" table (fixed `inline-flex size-4` box, sr-only text, `title` = summary).

### `web/src/components/Sidebar.tsx` — RESEARCH "Sidebar mark data" snippet (useRouterState `onHostPage` → `refetchInterval: false` on /host, else 30_000; `retry: false`; queryKey `['host']`).

### `web/src/routes/wall.tsx` — `Named` tile (455-481)
```tsx
<div key={`${tile.kind}/${tile.namespace}/${tile.name}`} data-row="" data-state={tile.state}
  className={`... ${TILE_COLOURS[tile.state] ?? TILE_COLOURS.unknown}`}>
  <p className="break-words font-semibold leading-tight">{tile.name}</p>
  <p className="truncate opacity-80">{tile.namespace === '' ? tile.detail : `${tile.namespace} · ${tile.detail}`}</p>
  {trends?.[tile.name] !== undefined && (<NodeTrend .../>)}
</div>
```
Prepend `{kind:'Host', namespace:'', name, state, detail:`manager · ${reason}`}` in `LeftHalf` at line 247 (`<Named title="Nodes" tiles={wall.nodes} …/>`); add `data-kind={tile.kind==='Host'?'host':undefined}`; skip `NodeTrend` for `kind==='Host'`. Headline keeps counting `wall.nodes`.

### `web/src/api.ts`
Copy `hardwareHistory: (id, range: HistoryRange): Promise<History> =>` (3314) as sibling `hostHistory: (range: HistoryRange)` hitting `/api/v1/host/history?range=`. `temperatureSchema` gains `warn_c`/`danger_c: z.number()` (no `.default()`); `hostSchema` gains `health`; `wallSchema` gains `host ... .nullish().transform(v => v ?? null)`.

## Shared Patterns

- **Problem responses:** `httpapi.WriteProblem` + `historyConfigured` / `historyRange` (handlers/history.go:51-61) — every history route.
- **Failure memory in the sampler:** `s.failed(key, sentence, err)` / `s.recovered(key, sentence)` — one-sentence operator-facing log lines.
- **Gaps, never zeros:** absent key in the `Record` map ⇒ NaN ring slot ⇒ `LiveChart` `segments()` gap. Applies to Go `HistoryValues` and the browser `useLiveSeries` pick.
- **Wall disclosure:** only name/state/short reason in the wall answer; no error text, address or version.
- **Guards red first:** every new test is seen failing against its named reinstated fault (RESEARCH Pitfalls 1, 2, 4, 5; D-09, D-12).
- **Public repo:** fixtures use `manager-01.homelab.example` / `example-host` only.

## No Analog Found

| File | Role | Reason |
|---|---|---|
| browser test for 12 px word-space advance (`index.css` fix) | test | No existing measurement of glyph spacing; follow RESEARCH Pitfall 9 (measure "a b" vs "ab" in Chromium) |

## Metadata

**Search scope:** internal/history, internal/host, internal/inventory, internal/httpapi/handlers, cmd/holzkube-managerd, web/src/{components,routes,api.ts}
**Pattern extraction date:** 2026-09-28
