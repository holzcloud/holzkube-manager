# Phase 11: Host-Seite — Pattern Map

**Mapped:** 2026-09-28
**Files analyzed:** 24 (new + modified)
**Analogs found:** 20 / 24

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|---|---|---|---|---|
| `internal/host/host.go` (View, `Reading[T]`, `Reason`) | model | transform | `internal/inventory/hardware.go` (HardwareView types, l.145-210); `internal/health/health.go` (`Field[T]`, l.104 — pattern only, NOT reused) | role-match |
| `internal/host/collector.go` (memo, rates, 60 s size cache) | service | request-response + memo | `internal/inventory/hardware.go` (`hardwareMemo` l.69-80, memo update l.290-316, `usableBaseline` l.320-328) | exact |
| `internal/host/sys.go`, `sys_linux.go`, `sys_other.go` | utility | syscall I/O | none in repo with build-tagged syscalls | no analog |
| `internal/host/mountinfo.go` | utility | file-I/O parse | none | no analog (RESEARCH Pattern 2/3) |
| `internal/host/proc.go` | utility | file-I/O parse | `internal/inventory/hardware.go` `memoryView` l.487-496 (formula) | partial |
| `internal/host/identity.go` | utility | file-I/O | none | no analog |
| `internal/host/sensors.go` | utility | file-I/O walk | `internal/talos/sensors.go` (`discoverSensors` l.340-512, `thermalZones` l.515-575) | exact (logic), different transport |
| `internal/host/network.go` | utility | file-I/O | `inventory.HardwareLink` l.179-190 + `perSecond` l.417-422 | role-match |
| `internal/host/filesystems.go` | utility | file-I/O + syscall | `inventory.HardwareFilesystem` l.150-162 | partial |
| `internal/host/updatestatus.go` | utility | file-I/O strict parse | none dedicated | no analog |
| `internal/host/testdata/{pi5,amd64,procsubset-pid}/` | test fixture | — | `internal/store/testdata/` (hand-written trees) | role-match |
| `internal/host/*_test.go`, `updatescript_test.go` | test | — | package tests beside analogs | role-match |
| `internal/talos/sensors.go` (MODIFY: export helpers) | utility | transform | itself | exact |
| `internal/inventory/hardware.go` (MODIFY: export `cpuPercent`) | service | transform | itself l.398-410 | exact |
| `internal/httpapi/handlers/host.go` | controller | request-response | `internal/httpapi/handlers/inventory.go` (`machineHardware` l.715-740, route table l.82-97) | exact |
| `internal/httpapi/router.go` (MODIFY: `Deps.Host`) | config | — | `Deps` struct l.148ff | exact |
| `cmd/holzkube-managerd/main.go` (MODIFY: build collector, `Host:` in literal, `HostRoutes(deps)`) | config | — | `inventory.New(...)` l.223, route list l.824-832, Deps literal comment l.554-560 | exact |
| `cmd/holzkube-managerd/budget_test.go` (MODIFY: `noUpstream`) | test | — | l.1795-1806 | exact |
| `internal/config` + `docs/guide.md` (MODIFY: `--update-status-file`) | config | — | existing option rows (`readme_test.go` l.26-31) | exact |
| `docs/api-contract.md` (MODIFY) | docs | — | existing `/api/v1/machines/{id}/hardware` entry | exact |
| `deploy/holzkube-manager-update.sh` (MODIFY: EXIT trap + status write) | script | file-I/O | itself (`fail()` l.70, `trap` l.211, `LOCAL_VERSION` l.194) | exact |
| `web/src/api.ts` (MODIFY: `hostSchema`, `api.host`) | model/client | request-response | `hardwareSchema` l.1227ff, `api.machines.hardware` l.3136, `orEmpty` l.56 | exact (minus `.default(0)`) |
| `web/src/routes/host.tsx` (+ test) | component/route | polling | `web/src/components/NodeHardware.tsx` (l.74-100) + `web/src/routes/nodes.tsx` (`createRoute` l.252) + `node-detail.tsx` (dl/notices) | exact |
| `web/src/components/charts/Sensors.tsx` (moved out of NodeHardware) | component | transform | `NodeHardware.tsx` `TEMPERATURE_DEFAULTS` l.41, `temperatureLimits` l.49, `KIND_LABEL` l.57, `sensorKey` l.433, `Sensors` l.441, `SEVERITY_COLOR` l.426 | exact (move) |
| `web/src/App.tsx`, `Sidebar.tsx`, `web/scripts/layout-audit.mjs`, `web/fixtures/demo.json` (MODIFY) | config | — | `nodesRoute` App.tsx l.24/50; NAV entry Sidebar.tsx l.66-72; `ROUTES` layout-audit.mjs l.68 | exact |

## Pattern Assignments

### `internal/httpapi/handlers/host.go` (controller, request-response)

**Analog:** `internal/httpapi/handlers/inventory.go`

Route table shape (l.82-89) — read route, no `Action`:
```go
{
    Method:          http.MethodGet,
    Pattern:         "/api/v1/clusters",
    RequiresSession: true,
    MinRole:         model.RoleReader,
    Handler:         handler(listClusters(d)),
},
```
Handler shape (l.722-739):
```go
func machineHardware(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		ctx, cancel := budgetedContext(r, HardwareRouteBudget)
		defer cancel()
		view, err := d.Inventory.Hardware(ctx, model.MachineID(r.PathValue("id")))
		if err != nil {
			writeInventoryError(w, r, d, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}
```
For host: `d.Host.Read(r.Context())` then `writeJSON`; unexpected error → `httpapi.WriteInternal(w, r, d.Logger, err)` (as `metrics.go` l.62-65). Nil-collector handling: prefer constructing the collector unconditionally in `main` (no new problem code); if a nil check stays, its code must go into `docs/api-contract.md` (`contract_codes_test.go`). `HostRoutes(httpapi.Deps{})` must not dereference at construction. File header doc-comment explaining why no audit Action: copy the prose style of `metrics.go` l.10-31.

### `internal/host/collector.go` (service, memo)

**Analog:** `internal/inventory/hardware.go`

Window constants (l.52-59) — reuse values `500*time.Millisecond` / `5*time.Minute`:
```go
minRateWindow = 500 * time.Millisecond
maxRateWindow = 5 * time.Minute
```
Memo update under mutex, later reading wins (l.310-313):
```go
if sample.Counters.At.After(memo.counters.At) {
	memo.counters = sample.Counters
}
```
Baseline check (l.320-328) — copy the window check but DROP the `Boot.Equal` clause (RESEARCH Pattern 5: jitter on a derived boot time):
```go
window := cur.At.Sub(prev.At)
return window >= minRateWindow && window <= maxRateWindow
```
CPU arithmetic: export `cpuPercent` (l.398) as `CPUPercent` and call it; do not copy. Do NOT use `perSecond` (l.417) for links: it returns 0 on a backwards counter; host needs "no rate" (nil).

### `internal/host/sensors.go` (utility, fs walk)

**Analog:** `internal/talos/sensors.go` — export additively, then call:
- `ClassifyChip` l.79 (already exported)
- `inputsNamed` l.636 → `InputsNamed`; `numbered` l.658 → `Numbered`; `millidegrees` l.689 → `Millidegrees`
- twin rule l.558 `if have[strings.ReplaceAll(typ, "-", "_")] {` → `ThermalTwinName(typ)`, used by `thermalZones` too
- "zones only when no CPU chip has temps" l.490-497:
```go
if sc.Kind == SensorCPU && len(sc.Temps) > 0 {
	cpuTemps = true
}
...
if !cpuTemps {
	zones, err := c.thermalZones(ctx, layout.Chips)
```
Emit `inventory.HardwareTemperature` / `inventory.HardwareFan` (hardware.go l.192-210) unchanged. hwmon entries are symlinks: do not filter on `IsDir()`.

### `internal/host/host.go` (model)

Element JSON shapes to mirror (hardware.go l.150-190): `HardwareFilesystem{mount, device, size_bytes, used_bytes}` extended with `available_bytes, fstype, roles`; `HardwareLink{name, up, speed_mbit, rx_bytes_per_sec, tx_bytes_per_sec}` but speed/rates nullable. Readability: RESEARCH Pattern 1 `Reading[T]` with `*T` + `omitempty` — NOT `health.Field[T]` (its `value,omitzero` at health.go l.104 erases a readable 0). All slices built with `make()` (Ledger 162; see `internal/kube/emptylists_test.go`).

### `web/src/api.ts` (client schema)

**Analog:** `hardwareSchema` l.1227ff, list rule l.56:
```ts
const orEmpty = <T>(value: T[] | null | undefined): T[] => value ?? []
per_core: z.array(z.number()).nullish().transform(orEmpty),
```
Fetch (l.3136): `sendJSON('GET', '/api/v1/host', hostSchema)`.
Extract the temperature and fan element schemas of `hardwareSchema` into named consts shared by both. Host link/filesystem/cpu/memory values use the `reading(...)` discriminated union (RESEARCH Pattern 1) with NO `.default(0)` inside (hardwareSchema uses `.default(0)` everywhere — do not copy that).

### `web/src/routes/host.tsx` (route component, polling)

**Analog:** `web/src/components/NodeHardware.tsx` l.74-82:
```tsx
const hardware = useQuery({
  queryKey: ['hardware', machine.id],
  queryFn: () => api.machines.hardware(machine.id),
  refetchInterval: HARDWARE_POLL_INTERVAL_MS,
  retry: false,
})
```
Use `queryKey: ['host']`, `api.host`. Do NOT copy the `useLiveSeries` block (l.87-100) — no curves this phase (UI-SPEC). Route registration: copy `export const nodesRoute = createRoute({...})` from `routes/nodes.tsx` l.252 (parent `authenticatedRoute` from `@/routes/__root`); loading line pattern from nodes.tsx (`<p className="text-sm text-muted-foreground">…</p>`). Card/dl/notice class strings verbatim from `node-detail.tsx` as listed in UI-SPEC; `<details>` disclosure from `DataTable.tsx` `PhoneRow`.

### `web/src/components/charts/Sensors.tsx` (moved)

Move from `NodeHardware.tsx`: `TEMPERATURE_DEFAULTS` (l.41), `temperatureLimits` (l.49), `KIND_LABEL` (l.57), `SEVERITY_COLOR` (l.426), `sensorKey` (l.433), `Sensors` (l.441), fan row markup, optionally `HARDWARE_POLL_INTERVAL_MS` (l.29). Add a prop to omit the sparkline column. NodeHardware then imports them; no copies left.

### Wiring files

- `Sidebar.tsx` l.66-72 entry shape (`path, label, icon, phase: null, description`); insert `/host`, `Cpu` after Nodes.
- `App.tsx` l.24 import + l.50 route tree entry, same as `nodesRoute`.
- `layout-audit.mjs` `ROUTES` l.68: add `'/host'`.
- `budget_test.go` l.1795-1806: add `"GET /api/v1/host"` to the `noUpstream` list with a comment (reaches no node, no upstream).
- `main.go`: put `Host: hostCollector` INSIDE the Deps literal (comment l.554-560 explains why); append `handlers.HostRoutes(deps)` to the list at l.824ff.

### `deploy/holzkube-manager-update.sh`

Existing anchors: `fail() { printf 'FEHLER: %s\n' "$*" >&2; exit 1; }` (l.70); `LOCAL_VERSION=$("$BIN" --version ...)` (l.194, move before first network call); `trap 'rm -rf "$TMP"' EXIT` (l.211, replace by the single `on_exit` trap that also calls `record_status || true` and never `exit`s). JSON via `python3` (already required, l.92). Test copies the script into `t.TempDir()` before running it (self-replacement, l.281-286).

## Shared Patterns

- **File access:** only via `fs.FS` (`os.DirFS("/")` prod, `os.OpenRoot(dir).FS()` tests); `os.ReadFile/Open/ReadDir/Readlink` forbidden by `internal/store/fsstore/permissions_test.go` l.246-251. No leading `/` in fs paths.
- **Build tags:** every `x/sys/unix` call in `*_linux.go`; `!linux` stub returns `errUnsupported` (darwin build + macOS CI).
- **No null lists / no fake zeros:** Go `make()`, zod `.nullish().transform(orEmpty)`; unreadable → no `value` key.
- **gosec G115:** line-scoped `//nolint:gosec // reason` as `internal/history/persist.go` l.260.
- **Doc-comment style:** long "why" comments on types and routes, as in `metrics.go` and `hardware.go`.
- **Guards red:** every new test named with the fault it was seen red against (CLAUDE.md).
- **Fixtures:** hand-written, documentation values only (`example-host`, `192.0.2.x`), never copied from `/sys`.

## No Analog Found

| File | Role | Data Flow | Reason |
|---|---|---|---|
| `internal/host/sys_linux.go` / `sys_other.go` | utility | syscall | no build-tagged syscall code exists yet; use RESEARCH Pattern 7 + x/sys/unix |
| `internal/host/mountinfo.go` | utility | parse | no mountinfo parser; RESEARCH Pattern 2/3 |
| `internal/host/identity.go` | utility | file-I/O | no local-host identity reader; RESEARCH "Identity sources" |
| `internal/host/updatestatus.go` | utility | strict parse | no status-file reader; RESEARCH D-16 notes |

## Metadata

**Analog search scope:** `internal/inventory`, `internal/talos`, `internal/health`, `internal/httpapi`, `cmd/holzkube-managerd`, `web/src`, `web/scripts`, `deploy/`
**Files scanned:** ~15
**Pattern extraction date:** 2026-09-28
