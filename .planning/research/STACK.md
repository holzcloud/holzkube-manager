# Stack Research

**Domain:** Self-hosted bare-metal Talos Linux cluster/node control plane (open Sidero Omni alternative)
**Researched:** 2026-08-27
**Confidence:** HIGH for the Talos/Go core (verified against downloaded module source + live API), HIGH for frontend (verified against npm registry), MEDIUM for the local sandbox (verified against provisioner source, but not executed end-to-end).

---

## Verification Method — read this before trusting anything below

Version and API claims here were **not** taken from training data. They were produced by:

1. `curl https://proxy.golang.org/<module>/@latest` for every Go module version.
2. `go get github.com/siderolabs/talos/pkg/machinery@v1.13.9` into a scratch module, then reading the **actual downloaded source** at `/Users/holz/go/pkg/mod/github.com/siderolabs/talos/pkg/machinery@v1.13.9`. Every import path below was confirmed to exist on disk at that version.
3. Live `POST` against `https://factory.talos.dev/schematics` — the example payload below returned a real schematic ID.
4. `curl https://registry.npmjs.org/<pkg>/latest` for every npm version.
5. Reading provisioner source in `siderolabs/talos` at tag `v1.13.9` for the sandbox section.

Two findings below contradict what a model would likely assert from memory. They are flagged **VERIFIED CORRECTION**.

---

## Recommended Stack

### Core Technologies

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| Go | **1.26.7** (min 1.26.6) | Backend language | Non-negotiable: `machinery` is Go. See Go version trap below. |
| `github.com/siderolabs/talos/pkg/machinery` | **v1.13.9** | Talos machine API client, config generation, patching, COSI resources | The official, and only sane, way to speak the Talos API. Separate module from the Talos OS itself — you do **not** pull in the whole OS tree. |
| `github.com/cosi-project/runtime` | **v1.14.1** (pinned by machinery) | Resource `Get`/`List`/`Watch` on Talos state | `client.Client.COSI` is a `state.State` from this module. This is how you read live node state in 2026. |
| `github.com/siderolabs/image-factory/pkg/client` + `pkg/schematic` | **v1.5.1** | Image Factory API client | **VERIFIED CORRECTION:** an official Go client exists. Do not hand-roll HTTP. |
| stdlib `net/http` + `http.ServeMux` | Go 1.26 | HTTP routing | Go 1.22+ method+wildcard patterns cover this app fully. No router dependency needed. |
| React | **19.2.8** | UI | Constraint from PROJECT.md; also correct for dense dashboards + streams. |
| TypeScript | **7.0.2** | Type safety | TS 7 (native Go compiler, "tsgo") is `latest` on npm as of today. |
| Vite | **8.2.2** | Frontend build | Produces the static bundle that `embed.FS` swallows. |
| Tailwind CSS | **4.3.3** | Styling | v4 is current. Uses the Vite plugin, not PostCSS. |

### Talos Integration — exact import paths (all confirmed present at v1.13.9)

```
github.com/siderolabs/talos/pkg/machinery/client            // Client, New, OptionFunc
github.com/siderolabs/talos/pkg/machinery/client/config     // talosconfig file format
github.com/siderolabs/talos/pkg/machinery/api/machine       // MachineService + LifecycleService
github.com/siderolabs/talos/pkg/machinery/api/storage       // Disks
github.com/siderolabs/talos/pkg/machinery/api/cluster       // cluster health checks
github.com/siderolabs/talos/pkg/machinery/api/time
github.com/siderolabs/talos/pkg/machinery/api/inspect
github.com/siderolabs/talos/pkg/machinery/api/resource      // COSI transport
github.com/siderolabs/talos/pkg/machinery/api/common
github.com/siderolabs/talos/pkg/machinery/api/security
github.com/siderolabs/talos/pkg/machinery/config            // Provider, VersionContract
github.com/siderolabs/talos/pkg/machinery/config/generate   // NewInput, Input.Config
github.com/siderolabs/talos/pkg/machinery/config/generate/secrets  // Bundle
github.com/siderolabs/talos/pkg/machinery/config/configpatcher     // Apply, StrategicMerge, JSON6902
github.com/siderolabs/talos/pkg/machinery/config/configdiff        // DiffConfigs  <-- free diff view
github.com/siderolabs/talos/pkg/machinery/config/configloader
github.com/siderolabs/talos/pkg/machinery/config/bundle
github.com/siderolabs/talos/pkg/machinery/config/machine    // machine.TypeControlPlane / TypeWorker
github.com/siderolabs/talos/pkg/machinery/config/types/v1alpha1
github.com/siderolabs/talos/pkg/machinery/resources/hardware  // SystemInformation (UUID)
github.com/siderolabs/talos/pkg/machinery/resources/network   // NodeAddress, LinkStatus
github.com/siderolabs/talos/pkg/machinery/resources/runtime   // MachineStatus
github.com/siderolabs/talos/pkg/machinery/resources/k8s
github.com/siderolabs/talos/pkg/machinery/resources/etcd
github.com/siderolabs/talos/pkg/machinery/resources/block     // Disk, DiscoveredVolume
github.com/siderolabs/talos/pkg/machinery/resources/cluster
github.com/siderolabs/talos/pkg/machinery/constants
github.com/siderolabs/talos/pkg/machinery/nethelpers
```

**`api/network` does NOT exist.** The investigation brief asked for it. There is no typed network RPC — network state comes exclusively from COSI resources under `resources/network` (`NodeAddress`, `LinkStatus`, `RouteStatus`). Build the network view on COSI, not on a typed API that isn't there.

Full `api/` directory at v1.13.9 is exactly: `cluster`, `common`, `inspect`, `machine`, `resource`, `security`, `storage`, `time`.

#### Connecting with cluster PKI

```go
c, err := client.New(ctx,
    client.WithConfigFromFile("/var/lib/holzkube/clusters/homelab/talosconfig"),
    client.WithNodes(ctx, "192.168.1.41"),
)
```

Other verified `OptionFunc`s: `WithConfig`, `WithConfigContext`, `WithContextName`, `WithTLSConfig`, `WithGRPCDialOptions`, `WithDefaultGRPCDialOptions`, `WithEndpoints`, `WithCluster`, `WithUnixSocket`, `WithNode`.

#### Connecting to a node in MAINTENANCE MODE (no cluster PKI)

```go
c, err := client.New(ctx,
    client.WithTLSConfig(&tls.Config{InsecureSkipVerify: true}), //nolint:gosec
    client.WithEndpoints("192.168.1.50"),
)
```

**Why this is the correct path, verified in source:** `client/connection.go:67-70` reads `c.options.tlsConfig` and, if non-nil, returns `credentials.NewTLS(tlsConfig)` immediately — short-circuiting *before* `buildTLSConfig(configContext)` is ever consulted. So `WithTLSConfig` is what lets you connect with no talosconfig and no CA at all. This is the same thing `talosctl --insecure` does.

**Do NOT** reach for `client/insecure_credentials.go`. It is gated behind `//go:build sidero.debug` and is not compiled in a normal build. A model working from memory may suggest it; it will not compile.

Maintenance mode only exposes a small RPC subset — realistically `ApplyConfiguration`, `Version`, `Disks`, and COSI reads. Design the discovery flow around that, not around the full API.

#### Generating machine config

Verified from the module's own `config/generate/example_test.go`:

```go
versionContract, _ := config.ParseContractFromVersion("v1.13")
secretsBundle, _ := secrets.NewBundle(secrets.NewFixedClock(time.Now()), versionContract)

input, _ := generate.NewInput("homelab", "https://192.168.1.41:6443", constants.DefaultKubernetesVersion,
    generate.WithVersionContract(versionContract),
    generate.WithSecretsBundle(secretsBundle),
    generate.WithEndpointList([]string{"192.168.1.41"}),
    generate.WithInstallImage("factory.talos.dev/metal-installer/<schematic>:v1.13.9"),
    generate.WithInstallExtraKernelArgs([]string{"console=ttyS0"}),
)

cfg, _ := input.Config(machine.TypeControlPlane) // or machine.TypeWorker
yaml, _ := cfg.Bytes()

clientCfg, _ := input.Talosconfig()  // the talosconfig to persist 0600
```

`secretsBundle` marshals to YAML/JSON — that is your per-cluster secret file. Generate it **once per cluster** and reuse for every node.

Other verified generate options worth knowing: `WithInstallDisk`, `WithAdditionalSubjectAltNames`, `WithRegistryMirror`, `WithDNSDomain`, `WithClusterCNIConfig`, `WithSysctls`, `WithAllowSchedulingOnControlPlanes`, `WithKubeSpanEnabled`, `WithClusterDiscovery`, `WithKubePrismPort`.

#### Config patching

Both mechanisms are supported and both are current:

```go
patches, _ := configpatcher.LoadPatches([]string{"@patch.yaml"})  // strategic merge or JSON6902
out, _ := configpatcher.Apply(configpatcher.WithBytes(originalYAML), patches)
```

- `configpatcher.StrategicMerge(cfg, patch)` — strategic merge (the modern, preferred form)
- `configpatcher.JSON6902(yamlBytes, jsonpatch.Patch)` — RFC 6902
- `configpatcher.LoadPatch` auto-detects which one a blob is

For the **diff view requirement**, `config/configdiff` gives it to you for free:

```go
diffText, _ := configdiff.DiffConfigs(oldCfg, newCfg)          // human-readable
patchList, _ := configdiff.Patch(original, modified)           // original -> patches
```

Do not pull in a third-party YAML differ. Using `configdiff` means your UI diff is semantically identical to what Talos itself computes.

#### Reading node state — COSI, not typed RPCs

`client.Client.COSI` is a `state.State` (wired at `client/client.go:180`). For the dashboard, COSI is the right surface in 2026: it supports `Watch`, so live dashboards are push-based rather than polled.

Resources that matter for holzkube's requirements:

| Need | Resource | Package |
|------|----------|---------|
| **Stable node identity** | `SystemInformation` (`.UUID`) | `resources/hardware` |
| Health/stage/version | `MachineStatus` | `resources/runtime` |
| Addresses | `NodeAddress`, `LinkStatus` | `resources/network` |
| Disks | `Disk`, `DiscoveredVolume` | `resources/block` |
| Services | `ServiceStatus` | `resources/v1alpha1` |
| etcd members | `EtcdMember` | `resources/etcd` |
| K8s version | `KubeletSpec`, `StaticPodStatus` | `resources/k8s` |

**On UUID-keyed inventory (raised by the FEATURES researcher — CONFIRMED):** `resources/hardware/system_information.go` defines type `SystemInformations.hardware.talos.dev` in namespace `hardware`, with field `UUID string`. This is the SMBIOS UUID and is the correct primary key for the node inventory. Keying on IP is wrong — DHCP moves IPs and you will silently duplicate or lose nodes.

Use typed RPCs only where COSI has no equivalent: `Dmesg`, `Logs`, `EtcdSnapshot`, `Reboot`, `Shutdown`, `Reset`, `Bootstrap`, `ApplyConfiguration`, `ServiceRestart`.

#### UPGRADES — the biggest trap in this project

**VERIFIED CORRECTION.** Read this before planning the upgrade phase.

In `api/machine/machine_grpc.pb.go`, `MachineService.Upgrade` carries:

```
// Deprecated: Do not use.
// Upgrade initiates the upgrade of the node to a new version of Talos.
//
// Use LifecycleService Upgrade RPC instead.
```

And in `client/client.go`, **both** convenience helpers are deprecated:
- line 580 — `func (c *Client) Upgrade(...)` → `// Deprecated: use LifecycleClient instead.`
- line 594 — `func (c *Client) UpgradeWithOptions(...)` → `// Deprecated: use LifecycleClient instead.`

So `client.UpgradeWithOptions(...)` — which is what a model will suggest from memory, and which is what most blog posts show — is the **deprecated** path.

The correct path is the `LifecycleClient` field on the client (`client/client.go:49`, wired at `:177`):

```go
stream, err := c.LifecycleClient.Upgrade(ctx, &machine.LifecycleServiceUpgradeRequest{
    Source: &machine.InstallArtifactsSource{
        ImageName: "factory.talos.dev/metal-installer/<schematic>:v1.13.9",
    },
})
// server-streaming: consume LifecycleServiceUpgradeResponse for live progress
```

This is a **server-streaming** RPC (`grpc.ServerStreamingClient[LifecycleServiceUpgradeResponse]`), which is exactly what the "Upgrade-Fortschritt live in der UI" requirement needs — you get native progress events instead of having to poll.

**The kernel-args divergence — CONFIRMED, and worse than reported.** The full field set of `LifecycleServiceUpgradeRequest` is:

```go
type LifecycleServiceUpgradeRequest struct {
    Containerd *common.ContainerdInstance
    Source     *InstallArtifactsSource   // { ImageName string }  <- that's all
}
```

There is **no** kernel-args field, **no** destination, and `InstallArtifactsSource` contains only `ImageName`. Even `LifecycleServiceInstallRequest`, which does have a `Destination`, has only `InstallDestination{ Disk string }` — still no kernel args.

Consequence for holzkube's design: **kernel command line is not something the upgrade RPC can carry, structurally.** It has exactly two sources of truth:
1. `.machine.install.extraKernelArgs` in the node's machine config, and
2. the schematic baked into the boot asset.

So the upgrade orchestrator must, per node: (a) look up the node's stored schematic ID, (b) construct `factory.talos.dev/metal-installer/<schematic>:<version>`, (c) verify the node's machine config still carries the intended `extraKernelArgs`, and only then upgrade. If you skip (a), system extensions silently vanish on upgrade. If you skip (c), the node reboots with a cmdline that no longer matches intent and the upgrade still reports success.

**Persist the schematic ID per node in the inventory.** This is a hard requirement, not a nice-to-have.

#### Supported Talos version range

`config/contract.go` defines contracts `TalosVersion1_0` through `TalosVersion1_13`. Config *generation* can therefore target back to v1.0.

That is not the same as API compatibility. Recommendation:

- **Declare support for Talos v1.11 – v1.13.** Below 1.11 you lose `talosctl` distribution via the Factory and hit multiple `contract.Greater(TalosVersion1_11)` behavior forks in the config generator.
- **Current stable is v1.13.9.** Latest release overall is **v1.14.0-rc.2** (prerelease — confirmed via GitHub releases API), so v1.14 will land during this project's lifetime. Because `LifecycleService` is brand new in this window, expect churn; keep the Talos client behind holzkube's own interface (PROJECT.md already mandates a transport abstraction — extend that to the API surface).
- Store the observed Talos version per node and **gate features on it**. A v1.11 node will not have `LifecycleService`; you need the deprecated `MachineService.Upgrade` as a documented fallback for older nodes.

### Image Factory

**VERIFIED CORRECTION: there is an official Go client. Do not hand-roll HTTP.**

```
github.com/siderolabs/image-factory/pkg/client     // v1.5.1
github.com/siderolabs/image-factory/pkg/schematic  // v1.5.1
```

Verified exported surface of `pkg/client`:

| Method | Use |
|--------|-----|
| `New(baseURL, ...Option) (*Client, error)` | construct |
| `SchematicCreate(ctx, schematic.Schematic) (string, *schematic.Schematic, error)` | create; returns ID + **normalized** schematic |
| `SchematicGet(ctx, id) (*schematic.Schematic, error)` | read back |
| `Versions(ctx) ([]string, error)` | enumerate Talos versions |
| `BrokenVersions(ctx) ([]string, error)` | versions the Factory flags as broken — **use this to grey out bad versions in the UI** |
| `ExtensionsVersions(ctx, talosVersion) ([]ExtensionInfo, error)` | enumerate system extensions |
| `OverlaysVersions(ctx, talosVersion) ([]OverlayInfo, error)` | enumerate overlays |
| `TalosctlList(ctx, talosVersion) ([]string, error)` | talosctl download URLs |

`schematic.Schematic.ID()` computes the ID **locally** without a round-trip — use it to detect "this schematic already exists" before POSTing.

`pkg/schematic` types: `Schematic{Owner, Overlay, Customization}`, `Customization{EmbeddedMachineConfiguration, ExtraKernelArgs, Meta, SystemExtensions, Bootloader, SecureBoot, DiskImage}`, `MetaValue{Key uint8, Value string}`, `SystemExtensions{OfficialExtensions []string}`.

#### Live-verified schematic payload

Executed against `https://factory.talos.dev/schematics` on 2026-08-27:

```yaml
customization:
  extraKernelArgs:
    - console=ttyS0
  systemExtensions:
    officialExtensions:
      - siderolabs/intel-ucode
      - siderolabs/iscsi-tools
```

Real response:

```json
{
  "id": "20e64852c1be21e6c5e22cafc52c2dcc5add07e66ce62e30fad173d709d5b652",
  "schematic": "customization:\n    extraKernelArgs:\n        - console=ttyS0\n    systemExtensions:\n        officialExtensions:\n            - siderolabs/intel-ucode\n            - siderolabs/iscsi-tools\n"
}
```

Notes that matter: `201 Created`; body is YAML (JSON is accepted as a YAML subset); **unknown fields are rejected**; the returned `schematic` field is canonical and authoritative — persist *that*, not your input, because the Factory may normalize or add fields. Well-known default (no customization) schematic ID: `376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba`.

#### Asset URL patterns

`GET /image/:schematic/:version/:path`, where `<arch>` ∈ `amd64|arm64`, `<platform>` e.g. `metal`:

| Path | Asset |
|------|-------|
| `metal-amd64.iso` | ISO (what you hand the operator for USB boot) |
| `metal-amd64.raw.xz` | raw disk image |
| `kernel-amd64` | kernel |
| `initramfs-amd64.xz` | initramfs — *"including system extensions if configured"* |
| `cmdline-metal-amd64` | **the resolved kernel command line for the schematic** |
| `installer-amd64.tar` | installer tarball — *"including system extensions if configured"* |
| `metal-installer-amd64.tar` | per-platform installer |

OCI (this is what the upgrade RPC consumes):

- **Modern:** `factory.talos.dev/metal-installer/<schematic>:<version>`
- **Legacy:** `factory.talos.dev/installer/<schematic>:<version>` — documented as "Legacy `installer` Image". Use the platform-prefixed form.
- `:latest` resolves to latest **stable**, skipping prereleases.

#### The installer / extraKernelArgs divergence — CONFIRMED

The FEATURES researcher's claim holds, and the official API reference states it plainly. The docs annotate **only** `initramfs-<arch>.xz`, `installer-<arch>.tar` and `<platform>-installer-<arch>.tar` with *"(including system extensions if configured)"*. The ISO and disk-image entries carry **no such qualifier** — they receive the full customization, kernel args and META included.

That asymmetry is the bug factory: an operator boots an ISO built from schematic X (gets `console=ttyS0`), the node installs, later gets upgraded via the installer image for schematic X — and the kernel args are **not** re-derived from the schematic. They must already be in `.machine.install.extraKernelArgs`.

**Design consequence for the Image Factory client:** when holzkube creates a schematic with `extraKernelArgs`, it must **also** write those same args into the generated machine config via `generate.WithInstallExtraKernelArgs(...)`. Treat the schematic and the machine config as two halves of one intent that must be kept in sync, and surface drift in the UI. Use the `cmdline-metal-amd64` endpoint to show the operator the actual resolved cmdline for a schematic.

`meta` has the same shape of problem — it is initial-META-only, applied at image build, and is not reapplied on upgrade.

### Go backend supporting libraries

| Library | Version | Purpose | Why |
|---------|---------|---------|-----|
| stdlib `net/http` | Go 1.26 | routing | `mux.HandleFunc("GET /api/nodes/{id}", ...)` covers everything here. Zero deps, zero upgrade risk. |
| stdlib `log/slog` | Go 1.26 | structured logging | In stdlib since 1.21. Nothing else is justified. |
| stdlib `embed` | Go 1.26 | SPA embedding | See SPA fallback note below. |
| **SSE** (hand-rolled, ~40 lines) | — | live logs, dmesg, upgrade progress | See rationale below. |
| `github.com/coder/websocket` | **v1.8.15** | WebSocket, *only* if you add an interactive terminal | Actively maintained successor to `nhooyr/websocket`. Context-aware, `net.Conn` adapter. |
| `golang.org/x/crypto` | **v0.55.0** | `argon2.IDKey` | PROJECT.md mandates argon2id. |
| `github.com/alexedwards/argon2id` | **v1.0.0** | argon2id hash/verify + encoded-string format | Thin, correct wrapper: sane defaults, constant-time compare, PHC-format strings. Removes the easiest crypto mistake. |
| `github.com/alexedwards/scs/v2` | **v2.9.0** | server-side sessions | Cookie holds only an opaque ID; session state server-side means real logout/revocation. Has a file-backed store, matching the "files on disk" model. |
| `github.com/justinas/nosurf` | **v1.2.0** | CSRF | Plain `http.Handler` middleware, no framework assumptions. |
| `golang.org/x/sync` | **v0.22.0** | `errgroup`, `semaphore` | Rolling upgrade orchestration. |
| `github.com/siderolabs/go-retry` | **v0.3.3** | retry/backoff | Already in the Talos dependency graph; matches Talos semantics for "node is rebooting, keep trying". |

#### SSE vs WebSocket — use SSE

Recommend **SSE** for logs, dmesg, and upgrade progress. Rationale:

- Every one of those is a **gRPC server-stream → browser**, i.e. strictly unidirectional. SSE is the exact shape; WebSocket is a bidirectional tool used unidirectionally.
- SSE is plain HTTP: your session cookie, CSRF, and reverse proxy all just work. WebSocket upgrade needs separate auth handling.
- Automatic browser reconnect with `Last-Event-ID` — meaningful when a node reboots mid-upgrade, which is the normal case here.
- Go implementation is `http.Flusher` plus a loop. No dependency.

Add `coder/websocket` **only** if an interactive shell lands (bidirectional). The Talos API does not expose an interactive shell, so this may never be needed — but `xterm.js` is still the right renderer for *read-only* log tailing, fed by SSE.

#### SPA embedding and fallback

```go
//go:embed all:dist
var distFS embed.FS
```

Use `all:` — without it, `go:embed` silently skips files starting with `_` or `.`, which Vite emits. Serve `/api/` from the mux first, then fall back: try the file, and on `os.IsNotExist` serve `index.html` with `200`. Never let the SPA fallback shadow `/api/` — return real 404s there so fetch errors are diagnosable.

#### Long-running job orchestration

Do **not** add a job queue (asynq/machinery/river). Wrong shape: at homelab scale there is one operator, a handful of nodes, and at most one rolling upgrade at a time. A durable broker adds Redis/Postgres — directly contradicting the single-binary, no-runtime-deps constraint.

Pattern: an in-process `TaskManager` holding `map[TaskID]*Task`, each task a goroutine with a `context.CancelFunc` (gives you "abbrechbar"), appending to a bounded ring buffer of progress events fanned out over SSE. Persist a task journal as JSON on disk so a restart mid-upgrade is visible rather than silent. `errgroup` for health-gated sequencing; `siderolabs/go-retry` for the health gate itself.

The health gate should use the `cluster` API's health checks plus `MachineStatus` from COSI — do not just check "is the port open."

### Frontend

| Library | Version | Purpose | Why |
|---------|---------|---------|-----|
| `react` / `react-dom` | **19.2.8** | UI | current stable |
| `typescript` | **7.0.2** | types | current stable (native compiler) |
| `vite` | **8.2.2** | build | current stable |
| `@vitejs/plugin-react` | **6.1.0** | React plugin | matches Vite 8 |
| `@tanstack/react-query` | **5.102.6** | server state | v5 is still `latest` — **there is no v6**. A model may claim otherwise. |
| `@tanstack/react-router` | **1.170.32** | routing | recommended over React Router |
| `tailwindcss` + `@tailwindcss/vite` | **4.3.3** | styling | v4; use the Vite plugin |
| `shadcn` (CLI) | **4.19.0** | UI components | copy-in components, not a dependency |
| `lucide-react` | **1.34.0** | icons | shadcn default |
| `class-variance-authority` | **0.7.1** | variants | shadcn dependency |
| `@xterm/xterm` | **6.0.0** | log/console rendering | **VERIFIED: the package is `@xterm/xterm`.** The old `xterm` package is frozen at 5.3.0 and is dead. |
| `@xterm/addon-fit` | **0.11.0** | terminal resize | |
| `codemirror` + `@codemirror/merge` | **6.0.2** / **6.12.2** | YAML editing + diff | recommended over Monaco |
| `zod` | **4.4.3** | API response validation | Talos data is deeply nested; validate at the boundary |

**Router — TanStack Router over React Router.** Typed route params end-to-end, and this app is param-heavy (`/clusters/$clusterId/nodes/$nodeUuid/logs/$service`). It also integrates natively with TanStack Query for per-route loading. React Router 7 is fine but gives up the type safety, and its framework mode pushes an SSR model irrelevant to an embedded SPA.

**Editor — CodeMirror 6 over Monaco.** Decisive factor is bundle size inside a single Go binary: Monaco is several MB and needs web workers plus a non-trivial Vite setup. CodeMirror 6 is modular and tree-shakes to a fraction of that. `@codemirror/merge` provides a real side-by-side/unified diff view with `unifiedMergeView` — which covers the "Diff-Ansicht vor dem Anwenden" requirement directly.

Important pairing: render the diff from the **server-computed** `configdiff.DiffConfigs` output rather than diffing YAML strings client-side. CodeMirror displays it; Talos' own logic computes it.

**Query + SSE integration pattern:** do not try to force SSE through `useQuery`. Use `useQuery` for snapshots and a small `useEventSource` hook that pushes into `queryClient.setQueryData` for live updates. For append-only streams (logs, progress) keep a local reducer and never round-trip through the query cache.

### Persistence — CHALLENGE to a locked decision

PROJECT.md locks: *"Secrets als Dateien auf Disk (`0600`)"*. Flagging this rather than silently overriding, per instruction.

**Endorsed, unchanged, for secrets.** talosconfig, cluster PKI, and secrets bundles should stay `0600` files. They are naturally file-shaped, `machinery` reads and writes them as files (`clientCfg.Save(path)`), they are inspectable with `cat`, backed up with `cp`, and they avoid inventing a crypto layer. Keep this.

**Challenged for the non-secret inventory.** The requirements include multi-cluster inventory, versioned reusable config patches, and an audit log answering "who did what to which node when." An append-only audit log plus versioned patches plus a UUID-keyed node inventory across clusters is relational data with query needs, and JSON-files-in-a-directory will degrade: no atomic multi-entity updates, no indexes, hand-rolled concurrency control, and a torn write during a crash corrupts inventory.

**Recommendation:** keep the split.

- `0600` files: talosconfig, PKI, secrets bundles, password hash. *(unchanged — this is the locked decision, and it is correct)*
- Embedded DB: inventory, patches, audit log, task journal.

For the DB, **`modernc.org/sqlite`** (pure-Go SQLite, no cgo) preserves the single-binary/cross-compile constraint — cgo would break `goreleaser` cross-compilation, which is precisely why `mattn/go-sqlite3` is wrong here. bbolt is the alternative if you want zero SQL, but you lose ad-hoc queries the audit log will want.

This does not violate the *spirit* of the decision (no crypto eigenbau, no external service, still a single binary, still a single data directory). It swaps a hand-rolled file format for a well-tested embedded one **for the non-secret half only**.

**If the user rejects this:** the fallback is a single `inventory.json` written atomically (write temp + `fsync` + `rename`) under one mutex, with the audit log as a separate append-only JSONL file. That is genuinely workable at homelab scale — just be explicit that it is a scale ceiling, and never split inventory across many small files.

---

## Local Talos Sandbox for Development — the section that decides testability

The developer has **no reliable hardware access** and `talosctl` is **not installed**. PROJECT.md correctly makes this an early phase. Findings below come from reading provisioner source at tag `v1.13.9`.

### Step 1 — install `talosctl` (no package manager needed)

The Image Factory serves official binaries, verified live today (HTTP 200, `content-disposition: attachment`):

```bash
curl -Lo /usr/local/bin/talosctl \
  https://factory.talos.dev/talosctl/v1.13.9/talosctl-darwin-arm64
chmod +x /usr/local/bin/talosctl
talosctl version --client
```

`GET https://factory.talos.dev/talosctl/v1.13.9` lists all platform URLs. (Homebrew `brew install siderolabs/tap/talosctl` also works; the curl route is version-pinned and matches the machinery version, which is preferable.)

### Step 2 — Docker provisioner (start here, know the ceiling)

```bash
talosctl cluster create --provisioner docker \
  --name holzkube-dev \
  --controlplanes 1 --workers 2 \
  --talos-version v1.13.9
```

**Verified limitations, read from `pkg/provision/providers/docker/`:**

| Limitation | Evidence | Impact on holzkube |
|-----------|----------|--------------------|
| **No maintenance mode** | `node.go:102` injects the machine config at container create via `USERDATA=<base64>` env var. Containers boot already-configured. | **The core value flow (discovery → apply-config → join) cannot be tested here.** This is the single biggest gap. |
| **No disks / no install** | `docker.go`: `// UserDiskName not implemented for docker` returns `""` | Disk views, install disk selection, wipe/reset untestable |
| **No upgrades** | no disk, no bootloader; container image is fixed at create | `LifecycleService.Upgrade` untestable |
| **No Image Factory assets** | containers use a Talos container image, not ISO/installer | schematic → boot path untestable |
| **Only controlplane nodes get host ports** | `node.go:171-186` — `PortBindings` set only when `nodeReq.Type == TypeInit \|\| TypeControlPlane` | **On macOS, container IPs are not routable from the host.** Workers are unreachable from a host-run holzkube. |

That last row is the sharp edge on darwin specifically. Docker Desktop for Mac gives no route to the container network, and the provisioner maps only the control plane's Talos API to `127.0.0.1:<random>`. A multi-node dashboard run from the host will see exactly one node.

**Mitigation — run holzkube itself inside the Docker network during development:**

```bash
docker run --rm -it --network holzkube-dev -p 8080:8080 \
  -v "$PWD:/src" -w /src golang:1.26 go run ./cmd/holzkube
```

Now container IPs resolve and every node is reachable, while the UI stays on `localhost:8080`. Bake this into a `Taskfile` target — it is the difference between a usable and a useless dev loop. Also useful: `--docker-ports` on `cluster create` to expose additional ports.

**Verdict:** Docker provisioner is excellent for the ~70% of holzkube that is inventory, dashboards, COSI watches, log/dmesg streaming, service status, etcd member listing, and config *viewing*. It cannot test provisioning or upgrades.

### Step 3 — QEMU provisioner on darwin/arm64 (for the flows Docker cannot reach)

**This is supported on macOS and is the notable finding here.** `pkg/provision/providers/qemu/` contains `arch_darwin.go`, `launch_darwin.go`, `node_darwin.go`, `preflight_darwin.go`.

`preflight_darwin.go` verifies exactly one thing:

```go
if runtime.GOARCH != "arm64" {
    return errors.New("currently qemu on darwin is supported only on arm machines")
}
```

The dev machine is darwin/**arm64** — it qualifies. Networking uses `vmnet-shared` (`launch_darwin.go`), which needs elevated privileges.

```bash
brew install qemu
sudo talosctl cluster create --provisioner qemu \
  --name holzkube-qemu \
  --controlplanes 1 --workers 1 \
  --talos-version v1.13.9 \
  --disk 10240
```

QEMU gives real VMs with real disks: **maintenance mode, install, upgrades, Image Factory assets, and reset all become testable.**

Two caveats to plan around:
1. **Architecture mismatch.** QEMU on darwin/arm64 runs **arm64** Talos, while production hardware is **amd64**. Since PROJECT.md already scopes ARM as out-of-scope-but-parameterized, treat the sandbox as arch-parameterized validation, and never hardcode `amd64` in schematic or asset URL construction — the sandbox will catch that immediately, which is a benefit.
2. `sudo` + `vmnet-shared` is heavier than Docker. Keep it as the deliberate "provisioning phase" harness, not the everyday loop.

### Step 4 — fake the Talos API for unit tests

**There is no official fake/mock.** `machinery` ships generated gRPC stubs and nothing resembling a test double.

Correct approach — and it is well-supported by what the module already gives you:

- `machinery/api/machine` exports `MachineServiceServer` and `RegisterMachineServiceServer`, plus `LifecycleServiceServer`. Implement the handful of methods you use, register on a `grpc.NewServer()` over `bufconn`, and point the client at it with `client.WithGRPCDialOptions(...)`.
- `client.WithUnixSocket(path)` is a verified option and is an even simpler seam than bufconn for integration-style tests.
- For COSI, `cosi-project/runtime` ships an **in-memory state implementation** you can back `client.COSI` with — this is the biggest win, since most of the dashboard reads COSI. You get real `Watch` semantics in tests without a cluster.
- Because PROJECT.md already mandates a transport interface, define holzkube's own narrow port (e.g. `NodeClient`) and fake *that* for most tests; reserve gRPC-level fakes for the adapter's own tests.

### Recommended layering

| Layer | Harness | Covers |
|-------|---------|--------|
| Unit | in-memory COSI + hand-rolled gRPC server on bufconn | business logic, orchestration, health gates |
| Integration (daily) | Docker provisioner + holzkube in the same Docker network | inventory, dashboards, streams, config view/diff |
| E2E (provisioning phase) | QEMU provisioner, `sudo`, arm64 | maintenance mode, apply-config, join, upgrade, reset |
| Manual | real amd64 homelab hardware | final validation only |

---

## Installation

```bash
# --- Backend ---
go mod init github.com/holz/holzkube

go get github.com/siderolabs/talos/pkg/machinery@v1.13.9
go get github.com/siderolabs/image-factory@v1.5.1
go get github.com/alexedwards/argon2id@v1.0.0
go get github.com/alexedwards/scs/v2@v2.9.0
go get github.com/justinas/nosurf@v1.2.0
go get golang.org/x/sync@v0.22.0
go get github.com/siderolabs/go-retry@v0.3.3
go get modernc.org/sqlite            # if the persistence recommendation is accepted
go get github.com/coder/websocket@v1.8.15   # only if an interactive terminal lands

# --- Frontend ---
npm create vite@latest web -- --template react-ts
cd web
npm install react@19.2.8 react-dom@19.2.8
npm install @tanstack/react-query@5.102.6 @tanstack/react-router@1.170.32
npm install @xterm/xterm@6.0.0 @xterm/addon-fit@0.11.0
npm install codemirror@6.0.2 @codemirror/merge@6.12.2 @codemirror/lang-yaml
npm install zod@4.4.3 lucide-react@1.34.0 class-variance-authority@0.7.1
npm install -D typescript@7.0.2 vite@8.2.2 @vitejs/plugin-react@6.1.0
npm install -D tailwindcss@4.3.3 @tailwindcss/vite@4.3.3
npx shadcn@4.19.0 init

# --- Tooling ---
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1
go install github.com/goreleaser/goreleaser/v2@v2.18.0
npm install -D @biomejs/biome@2.5.10
brew install go-task/tap/go-task

# --- Sandbox ---
curl -Lo /usr/local/bin/talosctl https://factory.talos.dev/talosctl/v1.13.9/talosctl-darwin-arm64
chmod +x /usr/local/bin/talosctl
```

---

## Build / Release Tooling

| Tool | Version | Why |
|------|---------|-----|
| **Taskfile** (`go-task`) | v3 | YAML, cross-platform, real file-based `sources:`/`generates:` staleness checks. Make's tab rules and `.PHONY` noise buy nothing here. |
| **goreleaser** | **v2.18.0** | cross-compiled archives + Docker images + checksums from one config. Requires a **pure-Go** dependency tree — hence `modernc.org/sqlite`, not `mattn/go-sqlite3`. |
| **golangci-lint** | **v2.13.1** | v2 config schema (`version: "2"`) differs from v1 — do not copy a v1 `.golangci.yml`. Enable at minimum: `errcheck`, `govet`, `staticcheck`, `revive`, `gosec`, `bodyclose`, `contextcheck`, `errorlint`, `nilerr`. `gosec` matters: this tool holds cluster PKI and can wipe machines. |
| **Biome** | **2.5.10** | one Rust binary replacing ESLint + Prettier; ~10-100x faster and no plugin/config sprawl. ESLint 10 is fine but not worth the config surface for a solo project. |

Build order is a hard dependency: **frontend must build before Go compiles**, because `embed.FS` needs `web/dist` to exist. Encode that in Taskfile (`build:go` depends on `build:web`) rather than relying on `go generate`, which does not run automatically during `go build` and will produce confusing stale-asset bugs.

---

## Alternatives Considered

| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|-------------------------|
| stdlib `net/http` | chi v5.3.2 | If you want grouped middleware sugar. Genuinely fine; just not necessary. |
| SSE | WebSocket (`coder/websocket` v1.8.15) | Only when bidirectional — i.e. an interactive shell |
| TanStack Router 1.170.32 | React Router 7.18.2 | If the team already knows it and typed params don't matter |
| CodeMirror 6 + `@codemirror/merge` | Monaco 0.56.0 | If you want full VS Code IntelliSense and can absorb the MBs |
| `modernc.org/sqlite` | bbolt | If you want pure KV and no SQL; loses ad-hoc audit queries |
| Docker provisioner | QEMU provisioner | Provisioning, maintenance mode, upgrades — Docker cannot do these |
| Biome 2.5.10 | ESLint 10.9.1 + Prettier | If you need a specific ESLint plugin Biome lacks |
| Taskfile | Make / `go generate` | Make if the team insists; never `go generate` for the embed step |

---

## What NOT to Use

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| `client.Upgrade()` / `client.UpgradeWithOptions()` | **Verified deprecated** at `client/client.go:580,594` — *"use LifecycleClient instead"* | `c.LifecycleClient.Upgrade(...)` (server-streaming) |
| `MachineService.Upgrade` RPC | **Verified deprecated** in `machine_grpc.pb.go` | `LifecycleService.Upgrade` |
| `client/insecure_credentials.go` internals | Gated behind `//go:build sidero.debug`; not in a normal build | `client.WithTLSConfig(&tls.Config{InsecureSkipVerify: true})` |
| `pkg/machinery/api/network` | **Does not exist** | COSI `resources/network` |
| `factory.talos.dev/installer/<id>:<ver>` | Documented as the **legacy** path | `factory.talos.dev/metal-installer/<id>:<ver>` |
| Hand-rolled Image Factory HTTP client | An official Go client exists at `image-factory/pkg/client` | `client.New(...)` + `pkg/schematic` |
| `xterm` (npm) | Frozen at 5.3.0, superseded, unmaintained | `@xterm/xterm@6.0.0` |
| `gorilla/websocket` | v1.5.3; `coder/websocket` is the actively developed, context-aware option | `coder/websocket@v1.8.15` |
| `mattn/go-sqlite3` | cgo — breaks goreleaser cross-compilation and the single-binary constraint | `modernc.org/sqlite` |
| asynq / river / machinery (job queues) | Require Redis/Postgres — violates no-runtime-dependencies | in-process TaskManager + `errgroup` |
| `siderolabs/theila` as a reference | **Archived since 2022** — dead code against a long-obsolete Talos API | Nothing; there is no OSS Talos web UI to borrow from |
| IP-keyed node inventory | DHCP reassigns IPs → duplicate/lost nodes | `hardware.SystemInformation.UUID` |
| Assuming `@tanstack/react-query` v6 | v5.102.6 is `latest`; **no v6 exists** | v5 |
| A v1 `.golangci.yml` | golangci-lint v2 changed the config schema | `version: "2"` schema |

---

## Version Compatibility

| Package A | Compatible With | Notes |
|-----------|-----------------|-------|
| `machinery` v1.13.9 | **Go >= 1.26.6** | **TRAP:** PROJECT.md records Go **1.26.4** on the dev box. `go get` emitted *"requires go >= 1.26.6; switching to go1.26.7"*. Toolchain auto-download rescues it, but pin `toolchain go1.26.7` in `go.mod` and note it in the README, or CI on an older image will fail confusingly. |
| `machinery` v1.13.9 | `cosi-project/runtime` **v1.14.1** | machinery pins v1.14.1 in its `go.mod`. Latest is v1.16.2 — **let MVS pick v1.14.1**; do not force an upgrade without cause. |
| `machinery` v1.13.9 | `grpc` v1.81.0, `protobuf` v1.36.12 | transitively pinned; don't fight them |
| `machinery` v1.13.9 | Talos nodes v1.11 – v1.13 | v1.14.0-rc.2 exists; gate `LifecycleService` on observed node version |
| `image-factory` v1.5.1 | Talos v1.2.0+ | `/versions` currently lists v1.2.0 through v1.13.x |
| Factory `talosctl` downloads | Talos **v1.11.0+** only | older versions are not served |
| Factory SPDX / VEX / scans | v1.11.0+ / v1.13.0+ | Enterprise-only; community returns 404/402 |
| Tailwind 4.3.3 | Vite 8.2.2 | use `@tailwindcss/vite`, **not** PostCSS |
| React 19.2.8 | TanStack Query 5.102.6, Router 1.170.32 | both support React 19 |
| goreleaser v2.18.0 | pure-Go deps only | any cgo dependency breaks cross-compilation |
| QEMU provisioner | darwin **arm64 only** | `preflight_darwin.go` rejects amd64 explicitly |

---

## Confidence Summary

| Recommendation | Confidence | Basis |
|----------------|------------|-------|
| machinery v1.13.9 + all import paths | **HIGH** | module downloaded; paths read off disk |
| Maintenance mode via `WithTLSConfig` | **HIGH** | traced short-circuit at `connection.go:67-70` |
| `LifecycleClient` for upgrades; old API deprecated | **HIGH** | deprecation comments read in generated + client source |
| Upgrade RPC cannot carry kernel args | **HIGH** | full struct field list inspected |
| `hardware.SystemInformation.UUID` as inventory key | **HIGH** | resource definition read directly |
| Image Factory API + example payload | **HIGH** | executed live; real ID returned |
| Official Go client for Factory | **HIGH** | source of `pkg/client` + `pkg/schematic` read at v1.5.1 |
| installer/initramfs = extensions only | **HIGH** | official API reference wording |
| Frontend versions | **HIGH** | npm registry `latest` + dist-tags |
| Docker provisioner limitations | **HIGH** | provisioner source read at v1.13.9 |
| QEMU works on darwin/arm64 | **MEDIUM-HIGH** | source confirms support; **not executed** — validate early |
| SSE over WebSocket | **MEDIUM** | reasoned design judgment, not an external fact |
| SQLite for non-secret state | **MEDIUM** | design judgment; challenges a locked decision — needs user ruling |

---

## Open Questions for the Roadmap

1. **Persistence split needs a user decision** before the data-model phase. Flagged as a challenge above, not applied.
2. **Validate the QEMU sandbox in the very first sandbox phase.** If `vmnet-shared` proves painful under `sudo`, the fallback is a Linux VM (Lima/Colima or UTM) running the QEMU provisioner nested — determine this early, because the entire provisioning milestone depends on it.
3. **v1.14 will ship during this project.** Decide whether v1 targets v1.13 only, or v1.13+v1.14, before building the version-gating logic.
4. Whether an interactive terminal is ever in scope — decides if `coder/websocket` enters the dependency tree at all.

---

## Sources

- `https://proxy.golang.org/<module>/@latest` — authoritative Go module versions (machinery v1.13.9, cosi-runtime v1.16.2, image-factory v1.5.1, and all backend libs). **HIGH**
- Downloaded module source at `/Users/holz/go/pkg/mod/github.com/siderolabs/talos/pkg/machinery@v1.13.9` — every import path, `client/connection.go`, `client/client.go`, `client/options.go`, `config/generate/example_test.go`, `config/contract.go`, `config/configpatcher/*`, `config/configdiff/*`, `resources/hardware/system_information.go`, `api/machine/machine_grpc.pb.go`, `api/machine/lifecycle_grpc.pb.go`, `api/machine/lifecycle.pb.go`. **HIGH**
- `https://factory.talos.dev/schematics` — live POST, 2026-08-27, returned ID `20e64852c1be21e6c5e22cafc52c2dcc5add07e66ce62e30fad173d709d5b652`. **HIGH**
- `https://factory.talos.dev/versions`, `/version/v1.13.9/extensions/official`, `/talosctl/v1.13.9` — live. **HIGH**
- `https://raw.githubusercontent.com/siderolabs/image-factory/v1.5.1/docs/api.md` — official API reference (schematic shape, asset paths, OCI installer paths, the extensions-only wording). **HIGH**
- `siderolabs/image-factory` v1.5.1 `pkg/client/client.go`, `pkg/schematic/schematic.go`. **HIGH**
- `siderolabs/talos` v1.13.9 `pkg/provision/providers/docker/{docker,node}.go` and `pkg/provision/providers/qemu/{preflight,launch}_darwin.go`. **HIGH**
- `https://api.github.com/repos/siderolabs/talos/releases` — v1.14.0-rc.2 prerelease, v1.13.9 stable. **HIGH**
- `https://registry.npmjs.org/<pkg>` — `latest` + dist-tags for every frontend package. **HIGH**

---
*Stack research for: self-hosted Talos cluster/node control plane*
*Researched: 2026-08-27*
