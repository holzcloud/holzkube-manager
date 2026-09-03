# Project Research Summary

**Project:** holzkube
**Domain:** Out-of-cluster Talos Linux node & cluster lifecycle control plane (single Go binary, embedded React UI, direct gRPC to `:50000`) — an operator-owned alternative to Sidero Omni
**Researched:** 2026-08-27
**Confidence:** HIGH on Talos mechanics and stack versions, MEDIUM-HIGH on architecture (opinionated synthesis), MEDIUM on the sandbox (verified in source, not executed)

> Source documents: [STACK.md](./STACK.md), [FEATURES.md](./FEATURES.md), [ARCHITECTURE.md](./ARCHITECTURE.md), [PITFALLS.md](./PITFALLS.md).
> The four researchers ran partly in sequence and corrected each other. **This document is the single authoritative position.** Where a source document disagrees with a ruling in [Resolved Conflicts](#resolved-conflicts--rulings), this document wins.

---

## Executive Summary

holzkube is an out-of-cluster control plane for Talos Linux: a Go daemon that holds cluster PKI, speaks the Talos machine API (gRPC + mTLS on `:50000`) directly to node IPs, and serves an embedded React UI. Research confirms the market gap is real — `siderolabs/theila` has been archived since 2022, Omni is BUSL-licensed, and every other OSS option is either a terminal UI, a config generator, or an in-cluster controller that dies exactly when you need it. The unoccupied slot is precisely what PROJECT.md describes. The technical path is also clear and unusually well-supported: `siderolabs/talos/pkg/machinery` (its own light Go module) provides the client, config generation, strategic-merge patching, and a **structural diff** (`config/configdiff`) for free; COSI (`client.COSI` as a `state.State`) provides push-based live node state via `WatchKind`; `siderolabs/image-factory/pkg/client` is an official Go client for the Factory. Almost nothing needs hand-rolling.

The defensible ground is narrower than it looks. `talos-pilot` (Rust TUI, MIT, actively maintained) already delivers node/service monitoring, log streaming, etcd diagnostics, PKI expiry, drain/reboot ops, a bootstrap wizard, config generation and `apply-config`. holzkube's differentiated surface is exactly four things it lacks — **Image Factory provisioning, health-gated upgrades, persistent UUID-keyed multi-cluster inventory, and versioned patches with diff** — delivered in a browser. The read-only dashboard half is simultaneously the most rewarding thing to build and the thing that is already solved; the gradient of the work points continuously away from the Core Value. Phase ordering, not any feature, is the mitigation (PITFALLS P18). v1 acceptance is binary and verbatim: *a blank mini-PC becomes a healthy cluster node without the operator opening a terminal.*

The dominant risk is not build difficulty but **silent divergence and the absence of hardware**. The developer has no `talosctl`, no `~/.talos/config`, and the homelab cluster was unreachable at project start — so the first version of everything gets written against beliefs about Talos, and the first real target is the operator's only production cluster (P17). The single highest-leverage item in the entire project is therefore `talossim`: an in-process fake Talos node serving **real generated protobufs** over **real mTLS**, with a real in-memory COSI state wrapped by `protobuf/server.NewState`, that the unmodified production client talks to. It is a Phase 1 blocker, not a testing chore. Layered on top of it, a family of pitfalls all share one shape — *the API returns success and the running system no longer matches its declared configuration*: kernel args dropped on upgrade, extensions vanishing when the schematic is forgotten, `.machine.install` applied as a no-op, `staged` silently discarding the previous stage, list-append duplicating patch entries, a schematic POST returning `200` for a nonexistent extension. The general countermeasure is the same everywhere: **declared-vs-observed verification after every mutation, and structural diff of actually-merged configs before every apply.** "The API said OK" is not evidence in this domain.

---

## Resolved Conflicts — Rulings

These are the points where the four documents disagreed. Each is ruled; do not re-litigate from the source documents.

### 1. Persistence — files win, behind a path-hiding `store` interface

**Ruling (already made by the orchestrator): ARCHITECTURE's position. Everything is `0600` files on disk. No SQLite in v1.**

STACK.md recommended splitting: `0600` files for secrets, `modernc.org/sqlite` for the non-secret half (audit log, versioned patches, multi-cluster inventory, task journal). ARCHITECTURE argued the opposite and is correct for this scale: at 5–20 nodes, `ReadDir` + unmarshal is nothing, files are human-readable and `cp`-backupable (which is *why* PROJECT.md locked the decision), and the real insurance is not the storage engine but the **interface shape**.

The discipline that makes this safe:

- `store` is **entity-shaped, never path-shaped**: `store.Machines().Get(ctx, id)`, never `store.ReadFile("machines/x.json")`.
- **Any `os.ReadFile` outside `internal/store/fsstore/` is an architecture bug.** Worth a lint rule.
- Atomic writes (tmp → `chmod 0600` → `fsync` → `rename` → `fsync(dir)`), three layers of concurrency control (process `flock`, per-entity mutex, `rev` CAS → `409 Conflict`), forward-only migrations with a pre-migration tarball, refuse to start if `VERSION` is newer than the binary.

With that in place, `fsstore` → `sqlitestore` is one new implementation and **zero changes above**.

**The specific trigger that justifies revisiting:** fleet-wide queries — "all nodes with schematic X and Talos < 1.9", or any cross-cluster reporting. Files have no index; that is the real wall, and it arrives around 10 clusters / ~300 nodes. Growth of the machines directory alone is *not* a trigger (fix with an in-memory index built at startup, maintained on write — a cache, not a rewrite).

### 2. Sandbox tiers — not a conflict; an emphasis difference. Ship the three-tier ladder.

PITFALLS said "prefer QEMU over Docker — Docker cannot exercise the Core Value at all." ARCHITECTURE proposed Tier 0 (`talossim`) → Tier 1 (Docker) → Tier 2 (QEMU). **These agree on the facts and differ only on what Docker is *for*.** PITFALLS is right that Docker cannot test provisioning; ARCHITECTURE is right that Docker's job is not provisioning — it is catching **fake drift** against real Talos, real etcd and real COSI, which is the mitigation for the very risk (P17) that PITFALLS raises.

**Single recommended strategy:**

| Tier | What | Lands in | Purpose | Hard limits (verified from provisioner source at v1.13.9) |
|---|---|---|---|---|
| **0 — `talossim`** | In-process fake: real protobufs, real mTLS, real in-mem COSI, scriptable failures | **Phase 1 — blocking** | Every client path, streaming, backpressure, job crash-recovery, degradation, error branches | Cannot teach real Talos semantics |
| **1 — Docker provisioner** | `pkg/provision/providers/docker` | Phase 2 | Contract tests vs real COSI/etcd; recorded fixtures; keepalive/`ENHANCE_YOUR_CALM` verification | Config injected via `USERDATA` env at container create ⇒ **no maintenance mode**; no disks; no install; no upgrades; **only control-plane nodes get host port bindings**, so on darwin workers are unreachable from the host |
| **2 — QEMU provisioner** | `pkg/provision/providers/qemu`, `sudo` + `vmnet-shared` | **Before Phase 6 is declared done** | maintenance → apply-config → install → reboot → join; reset; real upgrades; ISO/installer assets | arm64 only on this host; amd64 installer paths not validatable locally |

Two mandatory workarounds recorded now: run holzkube itself **inside the Docker network** during dev (`docker run --network holzkube-dev …`) or Tier 1 shows exactly one node on macOS; and `preflight_darwin.go` rejects anything but darwin/arm64 for QEMU — the dev box qualifies, but **this has not been executed** and must be spiked in Phase 2, with a nested-Linux-VM fallback (Lima/Colima/UTM) if `vmnet-shared` proves painful.

Additionally, per PITFALLS: **build fakes by recording real traffic, not by hand** (Tier 1 produces the fixtures), and run **one contract suite against both backends** so divergence is a test failure rather than a production surprise.

### 3. FEATURES.md is superseded on three points — carry PITFALLS' corrections forward

These are authoritative; FEATURES.md is **wrong** on each and a naive implementation from it would misfire.

**(a) The no-reboot allowlist is not `features.*`.** The verified v1.13 allowlist is: `.debug`, `.cluster`, `.machine.time`, `.machine.ca`, `.machine.acceptedCAs`, `.machine.certCANs`, `.machine.install`, `.machine.network`, **`.machine.nodeAnnotations`**, `.machine.nodeLabels`, `.machine.nodeTaints`, `.machine.sysfs`, `.machine.sysctls`, `.machine.logging`, `.machine.controlplane`, `.machine.kubelet`, `.machine.pods`, `.machine.kernel`, `.machine.registries`, and **only these five** features subkeys: `kubernetesTalosAPIAccess`, `hostDNS`, `imageCache`, `kubePrism`, `nodeAddressSortAlgorithm`. Everything else under `.machine.features` (`rbac`, `stableHostname`, `apidCheckExtKeyUsage`, `diskQuotaSupport`, …) **requires a reboot**. A recommendation engine built from FEATURES.md's `features.*` would confidently recommend `no-reboot` for changes that fail at apply time. The allowlist ships as a **tested constant with a version tag**, not a comment.

**(b) `talosctl wipe disk` is NOT the remedy for "a prior install shadows the ISO."** Verified: `wipe disk` wipes a block device "which is not used as a volume" — it therefore **cannot wipe the system disk of a running Talos node**. The actual remedies are `talosctl reset` with appropriate wipe scope (normal path), the `talos.experimental.wipe=system` kernel parameter at the GRUB console (boot-loop path), or a boot-order/physical change. `wipe disk` remains correct for *secondary/data* disks and does work in maintenance mode via `--insecure` (note `--method` defaults to `FAST` = metadata only; `ZEROES` is what actually destroys data).

**(c) `.machine.nodeAnnotations` was missing from FEATURES.md's allowlist.** Included above.

### 4. Upgrade ordering — Talos-before-Kubernetes is allowed only behind a hard gate

PITFALLS is right. FEATURES.md's P2-Talos / P3-Kubernetes ordering is defensible on cost but is **the exact ordering that produces [siderolabs/talos#12398](https://github.com/siderolabs/talos/issues/12398)**: Talos advanced `1.6.8 → 1.11.5` while Kubernetes sat at 1.28.3, and `upgrade-k8s` was then blocked *by the very skew it would fix* — because it works by patching each control-plane node's machine config, and Talos validates the whole resulting config, which still names the too-old current versions. Kubernetes cannot go forward and Talos cannot easily go back. There is no clean recovery.

**Required ordering constraint:**

1. The **Talos↔Kubernetes compatibility matrix is data, not documentation** (`talosMinor → [supported k8s minors]`; 1.13 → 1.36…1.31), and it lands in the **cluster-overview phase** — it is a read-only property, available before any upgrade capability exists.
2. **The forward-compatibility gate is a release blocker for the Talos upgrade phase.** Before offering Talos vN+1, compute whether the cluster's *current* Kubernetes version remains supported by vN+1. If not, **block** (not warn — this is a one-way door): *"Upgrading to Talos 1.14 would drop support for Kubernetes 1.31 (this cluster). Upgrade Kubernetes to ≥1.32 first."*
3. **"Distance from the edge" is displayed permanently** on the cluster overview, so the slow drift is visible while it is still cheap to fix.
4. **No single-hop upgrades across a minor boundary, ever.** Talos config migration is only tested between adjacent minors. Compute and display the full chain (`1.11.5 → 1.11.latest → 1.12.latest → 1.13.x`) and execute it as a sequence with the health gate between every step. No "upgrade to latest" button.
5. `upgrade-k8s --dry-run` is cheap and reports deprecated API resources — ship it before owning K8s upgrade orchestration.

Kubernetes upgrade may still be deferred to v1.x. It may **not** be deferred without (1) and (2).

### 5. Supported Talos version range — v1.13 **and** v1.14, RCs explicitly opt-in

STACK.md pinned research to machinery v1.13.9 and suggested declaring v1.11–v1.13. PITFALLS notes 1.13 reaches end of community support **at the 1.14.0 release (~2026-08-30 — days from now)**, and that `/versions` already ends in `v1.14.0-rc.2`.

**Recommendation:**

- **Declare v1.12 – v1.14 as the supported range.** v1.12 is the floor because the machine config became **multi-document** there, which is a real behavioural boundary (RFC 6902 JSON patches stop working). Anything below 1.12 forces a second patch engine for no user.
- **v1.14 must be in the range from day one.** Shipping a tool whose newest supported Talos is EOL in its first week is not viable. Build the version-gating logic with two supported minors in it, not one — the code shape (`observed version → capability set`) is what matters.
- **Pin `machinery` to the newest version compatible with the range and keep the Talos API behind holzkube's own interface** (PROJECT.md already mandates a transport abstraction — extend it to the API surface). `LifecycleService` is new in this window; expect churn.
- **RC handling, concretely:** `GET /versions` returns 108 entries ending `…v1.14.0-alpha.1, -alpha.2, -beta.0, -beta.1, -rc.1, -rc.2`. **`versions[len-1]` is an RC.** Filter pre-releases by default; offering one requires an explicit "include pre-releases" toggle that is off by default and visibly labelled. `:latest` on the OCI installer ref resolves to latest *stable* and skips prereleases — but the version list does not.
- **Store the observed Talos version per node and gate features on it.** Flag out-of-range nodes visibly. This is a PROJECT.md constraint and must become a tested check, not a README sentence.

---

## Key Findings

### Recommended Stack

The stack is essentially dictated and was verified by downloading module source and executing live API calls, not from memory. Go + `pkg/machinery` is non-negotiable (the alternative is reimplementing protobuf/mTLS by hand). The backend needs almost no third-party libraries: stdlib `net/http` with Go 1.22+ method+wildcard patterns covers all routing, `log/slog` covers logging, and SSE is ~40 lines on `http.Flusher`. No job queue — an in-process `TaskManager` with `errgroup` is the right shape for one operator and one rolling upgrade at a time; Redis/Postgres would violate the no-runtime-dependencies constraint outright.

**Core technologies:**
- **Go 1.26.7** (min 1.26.6) — backend. **Trap:** the dev box has 1.26.4; `machinery` v1.13.9 requires ≥1.26.6. `GOTOOLCHAIN=auto` rescues it locally but CI on an older image fails confusingly. **Pin `toolchain go1.26.7` in `go.mod`.**
- **`siderolabs/talos/pkg/machinery`** — Talos client, config generation, strategic-merge patching, COSI. **Its own Go module** (light). `pkg/provision` lives in the **heavy root module** — see module weight below.
- **`cosi-project/runtime` v1.14.1** (pinned by machinery; latest is v1.16.2 — let MVS pick) — `client.COSI` is a `state.State`; `WatchKind(..., WithBootstrapContents(true))` is list-and-watch in one call and is *the* dashboard primitive.
- **`siderolabs/image-factory/pkg/client` + `pkg/schematic` v1.5.1** — official Go client. **Do not hand-roll HTTP.** `schematic.Schematic.ID()` computes the ID locally without a round-trip.
- **React 19.2.8 + TypeScript 7.0.2 + Vite 8.2.2 + Tailwind 4.3.3** (`@tailwindcss/vite`, not PostCSS), embedded via `//go:embed all:dist` — the `all:` prefix is required or Vite's `_`/`.` files are silently skipped.
- **TanStack Router 1.170.32 + TanStack Query 5.102.6** (there is **no** v6), **CodeMirror 6 + `@codemirror/merge`** (not Monaco — bundle size inside a single binary), **`@xterm/xterm@6.0.0`** (the old `xterm` package is frozen at 5.3.0 and dead), **zod 4.4.3** at the API boundary.
- **SSE over WebSocket** — every stream here (logs, dmesg, upgrade progress) is a gRPC server-stream → browser, i.e. strictly unidirectional; SSE gets cookie auth, proxies, `curl`-debuggability and `Last-Event-ID` reconnect for free. `coder/websocket` v1.8.15 only if an interactive shell ever lands (Talos has no shell by design, so probably never).
- **Auth/security:** `alexedwards/argon2id` v1.0.0, `alexedwards/scs/v2` v2.9.0 (server-side sessions, file store), `justinas/nosurf` v1.2.0, `golang.org/x/sync`, `siderolabs/go-retry` v0.3.3.
- **Tooling:** Taskfile (v3), goreleaser v2.18.0 (**requires a pure-Go dep tree** — this is why `mattn/go-sqlite3` would be wrong if SQLite ever lands), golangci-lint v2.13.1 (**v2 config schema**; enable `gosec` — this tool holds cluster PKI), Biome 2.5.10.

**Two corrections a model would get wrong from memory:**
- **`client.Upgrade()` / `client.UpgradeWithOptions()` and `MachineService.Upgrade` are deprecated** (verified in source at `client/client.go:580,594`). The correct path is `c.LifecycleClient.Upgrade(...)` — a **server-streaming** RPC, which is exactly what "live upgrade progress" needs.
- **`pkg/machinery/api/network` does not exist.** Network state comes exclusively from COSI `resources/network` (`NodeAddress`, `LinkStatus`, `RouteStatus`).

**Module weight rule:** `cmd/holzkubed` depends on `pkg/machinery` **only**. The sandbox (Docker/QEMU provisioners, root module) goes in a **separate Go module under `sandbox/`**. Getting this wrong yields a ~200 MB binary with an unintended supply-chain surface and is painful to unwind.

**Maintenance mode connection:** `client.WithTLSConfig(&tls.Config{InsecureSkipVerify: true})` + `WithEndpoints(...)`, verified via the short-circuit at `client/connection.go:67-70`. Do **not** reach for `client/insecure_credentials.go` — it is gated behind `//go:build sidero.debug` and will not compile.

### Expected Features

**Must have (table stakes — without these it is not a Talos management plane):**
- Persistent, **UUID-keyed**, multi-cluster node inventory (`resources/hardware.SystemInformation.UUID`) — the spine everything hangs off
- Per-node health / Talos & K8s version / CPU / RAM / disks / network links, all via COSI
- Talos service status (etcd, kubelet, apid, machined, trustd) — highest signal-per-line-of-code in the product
- Cluster overview: CP vs worker, etcd members, health — degrading to per-node NODE-level truth when etcd is down
- View MachineConfig (rendered + raw) — **with server-side redaction, shipped in the same phase**
- Node actions reboot / shutdown / reset with server-enforced confirmation
- Apply config with mode selection, where the **mode is computed from the diff**, not merely offered
- Cluster bootstrap (guarded, once-only), etcd member list / remove / snapshot
- Local login (argon2id + session) and an audit log (JSONL, daily, hash-chained)

**Should have (the four things that justify building this at all):**
- **Blank machine → cluster join, in the browser** — the Core Value; no OSS tool does this in a web UI
- **Image Factory schematic builder** as a saved, reusable object linked to the node record
- **Maintenance-mode discovery** (subnet scan on `:50000` + manual IP) — replaces Omni's entire SideroLink+PXE+agent stack with a scan; the architectural bet, and a good one at homelab scale
- **Versioned, reusable config patches with structural diff** + dry-run + `--mode=try`
- **Health-gated rolling upgrade** with live progress and stop-after-current-node
- **Works when the cluster is down** — free structurally, but only if *enforced*

**Defer (v1.x / v2+):**
- Live log + dmesg streaming — **the single most seductive item on the list** (xterm.js! live output!) and explicitly not on the Core Value path. Holding this line is the specific argument that will happen.
- Kubernetes upgrade orchestration (behind the §4 gate), node removal (drain→reset), node lock, etcd snapshot
- PXE/netboot (Image Factory has a PXE frontend — a URL swap *if* schematics are modeled well now), SideroLink tunnel, OIDC/multi-user, BMC/IPMI, `holzkubectl`, Prometheus `/metrics` (export, never ingest), support bundle export

**Never (independently corroborated — `talos-pilot` draws the identical line):** workload management (Pods/Deployments/Helm), an own metrics pipeline, a generic unguarded YAML editor, cluster templates / GitOps sync, machine-class DSLs, in-cluster deployment, an agent on nodes. **The seam is the port: `:50000` is holzkube; `:6443` is k9s/Lens/Headlamp.** Deep-link out, broker a kubeconfig.

### Architecture Approach

A single Go binary, `cmd/holzkubed`, with thin HTTP handlers over domain services over a **path-hiding store** and a **transport seam**. Reads are push-based (one COSI watch per node in the server, fanned out to browsers over exactly **one multiplexed SSE connection per tab**); mutations are asynchronous jobs returning `202 Accepted` with a job ID, never a synchronous `200`. Long-running work — provisioning, upgrades, node removal — runs on **one** generic job engine with `Step{Act, Observe}` semantics, persisting intent before the side effect and confirmation after, so a crash in the gap is resolved by a read-only "did it happen?" query rather than a retry.

**Major components:**
1. **`transport` — the seam, and it is TWO interfaces.** `Dialer` (Resolve / DialOptions / Probe, over a `Target{Cluster, Machine, Addr}` where **Addr is a hint, never identity**) *and* `DiscoverySource` (Run → channel of candidates). Two are required because **a tunnel inverts the direction of contact**: today holzkube scans outward; with SideroLink, nodes register inward. A dial abstraction alone does not retrofit a tunnel. Bake `ErrNotReachableYet` into `Probe`'s contract now — a tunnel cannot probe a node that has not dialled in, and that is a *state*, not an error.
2. **`pool` + `client`** — connection cache keyed on `(ClusterID, endpoint, CredKind)` (**address-only keying cross-wires PKI between clusters — a security bug**). Direct-per-node by default, apid proxying as an explicit recorded fallback and **never** through the node targeted by the current mutating step. Per-call deadlines always, no deadline on streams. **Retry allowlist by full method name; default is no retry** — a blanket retry interceptor would retry `Bootstrap`, `Reset`, `ApplyConfiguration` and `Upgrade`. `NodeClient` and `MaintenanceClient` are **different Go types**, so calling `ServiceList` on a maintenance node is a compile error.
3. **`store` (fsstore)** — atomic 0600 writes, `flock` + per-entity mutex + `rev` CAS, forward-only migrations, backup/restore subcommands. Machines are **flat and UUID-addressed** with `cluster_id` as a **nullable field**, not nested under clusters — otherwise assignment becomes a file move (a rename race in the middle of provisioning), UUID lookup requires scanning every cluster dir, and "unassigned" becomes a special case.
4. **`jobs` + `provision` + `upgrade` JobKinds** — the persisted state machine (`discovered → identified → assigned → config_rendered → approved → applying → installing → awaiting_mtls → bootstrap_required/joining → ready`, plus `failed` with a **computed `retry_from`**, `cancelled`, `bootstrap_indeterminate`). Progress goes to `streamhub`, never to a socket held by the initiating request, so closing the tab does nothing.
5. **`health` with `HealthLevel` as a Go type** — `Field[T]{value, level, available, stale_since, unavailable_reason}` on every read-model field. This makes "still works when the cluster is down" **one enforceable test**: boot `talossim` with etcd and the K8s API scripted down, request node detail, assert every `LevelNode` field still has `Available: true`. Without it the K8s dependency creeps back within three phases.
6. **`streamhub`** — one upstream gRPC stream per topic `(cluster, machine, kind, params)`, refcounted, ring-buffered for replay, **non-blocking fan-out with visible gap markers**. The reader never blocks on a subscriber; a slow browser must not stall the gRPC reader or the other viewers, and dropped lines must be surfaced.
7. **`config`** — generation from the secrets bundle, versioned patch store, **structural merge + structural diff of actually-rendered configs**, validation, required-apply-mode computation, and a `redact` package with an entropy walk-test.
8. **`factory`** — Image Factory client; the only outbound dependency of the process, with a configurable base URL (self-hosted Factory is a real deployment mode).
9. **`talossim`** — in-process fake Talos node in `internal/` (not `test/`, because `cmd/holzkube-dev` uses it for demos and manual UI work).

**Security posture, stated plainly:** `clusters/<id>/secrets.yaml` contains the cluster CA **private key**. The holzkube state directory is **equivalent to root on every node**; the HTTP port is the soft path around Talos' entire hardened surface. There is no partial-credential design that avoids this — config generation genuinely requires the CA key. Consequences: HTTPS by default (self-signed on first run; `--insecure-http` only on loopback), default bind `127.0.0.1`, refuse to start if `stat(stateDir).Mode()&0o077 != 0`, `SameSite` + custom-header + `Origin`/`Sec-Fetch-Site` CSRF triad, **server-side** confirmation tokens, sudo-mode re-auth for destructive actions (`428`), no encryption at rest (FileVault/LUKS is the honest mitigation — a passphrase in the same process protects against nothing), and audit written as **intent before the call, outcome after**, hash-chained, with the limit stated honestly (tamper-*evidence*, not tamper-proofing).

### Critical Pitfalls

Ranked by what changes the build.

1. **P18 — Building the easy half and never reaching the Core Value (project failure).** The read-only surface is what `talos-pilot` already ships, and it is the most rewarding thing to build. **Prevention is phase ordering, not a feature.** Force a crude walking skeleton end-to-end against QEMU early — hardcoded schematic, manual IP, generated config, apply, join — before polishing anything. Track "days since the Core Value path last ran end-to-end" as a visible metric.
2. **P17 — Mock-only development, first real target is production (cluster-destroying).** `talossim` is the mitigation, but a hand-written fake encodes the author's misconceptions as passing tests. Therefore: record fixtures from Tier 1 rather than hand-writing them; make **"verified against real Talos" an explicit per-feature attribute** held open until it flips; build **`--dry-run` for the whole binary** into the client wrapper from day one; and ship a **per-cluster read-only mutation lock** so the homelab is adopted read-only first.
3. **The silent-divergence family (P9, P10, P11 Trap A, P12) — node-bricking, and all report success.** Kernel args have **two sources of truth** (`.machine.install.extraKernelArgs` and the boot-asset schematic) and the upgrade RPC **structurally cannot carry them** — `LifecycleServiceUpgradeRequest` contains only `Containerd` and `InstallArtifactsSource{ImageName}`. `installer`/`initramfs` honour **system extensions only**; kernel args and META are silently dropped. Upgrading a Factory-imaged node with the stock installer **deletes every extension** and reports success. `.machine.install` is on the no-reboot allowlist but is a **no-op until the next install/upgrade**. `staged` **does not stack** — a second staged apply silently discards the first. Strategic-merge **appends lists**, so a "reusable" patch is not idempotent by default. **One countermeasure covers most of it: post-mutation declared-vs-observed verification** (kernel cmdline, extension list, Talos version, schematic ID), plus structural diff of the *actually merged* config with **list growth called out explicitly** ("`extraKernelArgs`: 3 → 5, 2 appended").
4. **P3 — The health gate that looks green but does not protect quorum (cluster-destroying).** The right question is not "is everything healthy" but "**if I take node X down, will the remaining members still hold quorum?**" The gate must filter **learners** (they do not vote), require **raft applied-index convergence**, check **etcd alarms** (a `NOSPACE` alarm freezes a cluster whose member list looks perfect), **refuse at ≤2 voting members**, re-evaluate **before every node including the first**, and **render its inputs, not just its verdict**. Talos' own quorum protection covers etcd only — it will happily let you destroy Ceph's quorum; say so in a one-time per-cluster acknowledgement.
5. **P2 — `talosctl reset` defaults are maximally destructive.** `--wipe-mode=all` (destroys **user data disks**, not just the OS) and `--reboot=false` (the machine **powers off** — a headless mini-PC in a cupboard is now stranded). An argument-less mapping inherits both. Never expose an argument-less Reset: default to `STATE+EPHEMERAL` + reboot, **enumerate the disks that will be wiped by model/serial/size before the confirm activates**, type-the-hostname for anything broader, and display the effective flags.
6. **P1 / P5 / P16 — the PKI cluster.** The talosconfig client cert expires in ~12 months and Talos does **not** rotate client certs; every node goes red **in the same second** and the operator concludes the cluster died. Without the secrets bundle there is **no software path back in**. Therefore: **persist the secrets bundle as a hard precondition of adopting any cluster**, parse `NotAfter` at startup and on every dashboard render, warn at 90/30 days, distinguish `x509: certificate has expired` from "node down" as a **cluster-level banner**, ship one-click renew — and **defer CA rotation entirely** (P5's blast radius is worse than its absence). Import must read from a **control-plane** node (`.machine.ca` on a worker has no private key) and must end with a **live connectivity proof using a cert minted from the imported bundle**, not "Imported ✓". `gen secrets` is unreachable outside "Create new cluster" — no lazy "generate if missing" fallback, ever. And **redaction ships with "View MachineConfig"**, because that feature *is* the leak: `.machine.ca.key`, `.cluster.secret`, `.cluster.token`, `.machine.token`, `cluster.serviceAccount.key` are all in the response.
7. **P8 — Applying a config to the wrong machine.** Maintenance mode is **unauthenticated in both directions**; two identical mini-PCs on the same subnet are indistinguishable; DHCP can move the lease between scan and apply. **Re-read the UUID immediately before apply and abort on mismatch** (five lines, closes the DHCP race). Never auto-act on a scan result. Prove it is Talos (TCP → TLS → `version` RPC → `get`) before sending anything. Offer `--cert-fingerprint` pinning and say in one plain sentence that the fingerprint is only obtainable from a physical/serial/IPMI console.
8. **P4 — Bootstrap is one-shot and non-idempotent.** Four independent layers: `EtcdMemberList` pre-flight, an `O_CREAT|O_EXCL` lease, a persisted-and-fsynced intent record, and Talos' own `AlreadyExists`. The `bootstrap_indeterminate` state exists because layer 3 detects a crash but cannot resolve it — **resolution is layer 1: ask etcd.** `bootstrap.json` is cluster-scoped and single-valued, so "bootstrap a second node" has nowhere to be recorded. Recovery (`--recover-from`) is a **separate flow with an inverted precondition**, never the same button with an optional file field.
9. **P15 / P14 — the UI must not lie, and gRPC will help it lie.** A dead node does not refuse connections, it goes silent; a call with no deadline hangs forever. **Every RPC has a deadline, enforced structurally** in a wrapper that will not issue a call without one. Per-node fan-out is independent with partial rendering. Every datum carries its observation time; staleness is visually loud; **unknown is never treated as OK** and actions with unverifiable preconditions are disabled *with the reason shown*. Error taxonomy is a **shared type defined once, early** ("node unreachable (TCP timeout)" ≠ "apid refused" ≠ "certificate expired" ≠ "K8s API down, Talos fine"). On streams: **start with keepalive OFF** — Talos' gRPC factory sets no enforcement policy, so grpc-go's 5-minute `MinTime` applies and a "responsive" 10–30 s client keepalive earns `GOAWAY / ENHANCE_YOUR_CALM`, tearing down **every stream on that connection** and looking exactly like a flapping node. One context per stream, cancelled in `defer`, with a 100-open/close goroutine-baseline test.
10. **P7 — Storing full machine configs silently downgrades Kubernetes.** `upgrade-k8s` rewrites component image references in the live config; a "restore this config" button is a **one-click control-plane downgrade labelled as a recovery feature**. The persisted source of truth is (secrets bundle + patches + cluster name + endpoint + k8s version + **pinned Talos version contract**). Rendered configs are ephemeral artifacts — generate → validate → diff → apply → discard. This is a **schema decision**, so decide before the first migration.
11. **HTTP/1.1's ~6-connections-per-origin cap** deadlocks a naive stream-per-panel design at 5–20 nodes in a way that looks like a server hang. One multiplexed `/api/v1/stream` per tab, `POST`-based subscribe/unsubscribe, plus HTTP/2 (which HTTPS-by-default gives you anyway).
12. **`--mode=try`'s 60-second auto-revert is Talos' best safety primitive** and reads like a debug flag. Make it the **recommended default for any diff touching `.machine.network`**, with a live countdown and a "Keep this change" button that re-applies in `auto`. If the change kills connectivity, the node heals itself. This is genuinely differentiating.
13. **Image Factory schematic creation is not validation.** Verified by direct experiment: POSTing a nonexistent extension returns **HTTP 200 + a real ID**; the failure surfaces only at model-build time (**HTTP 400** on the ISO URL). A UI showing "Schematic created ✓" after the POST is lying. Validate names against `/version/<v>/extensions/official` **before** POSTing, then probe the target model (HEAD the ISO / GET the installer manifest) before marking it usable. Also: the installer OCI repo name is **version-dependent** (`metal-installer` resolves at 1.13.7, not at 1.9.0) — build per version and verify the manifest resolves. And the schematic ID **is recoverable from any running node** via a virtual system extension (`talosctl get extensions`), which matters enormously for the import path where nodes predate holzkube; conversely the Factory deliberately provides **no way to list schematics**, so an ID neither stored nor on a node is genuinely gone.

---

## Implications for Roadmap

### Hard ordering constraints (violate these and the project breaks)

| # | Constraint | Why |
|---|---|---|
| 1 | **`talossim` before any feature work** | No hardware, no `talosctl`. Every phase after it is otherwise written blind. It is a correctness dependency, not a testing convenience. |
| 2 | **The jobs engine before provisioning** | Provisioning is the hardest job on the engine — the one with a non-observable step (`Bootstrap`) and a mandatory manual-resolution state. Building it first means building the engine badly, inside a wizard. |
| 3 | **Cluster import in the inventory phase, not the provisioning phase** | The operator's homelab cluster **already exists**; import is the first path exercised in anger. A design that discovers this later has already written `cluster.Create` as though secrets are always generated locally. `Import` and `Create` produce the same record with an `origin` field. |
| 4 | **Cluster overview / etcd health before rolling upgrades** | The health gate cannot exist before etcd member health does (P3). |
| 5 | **The Talos↔K8s compatibility matrix before the Talos upgrade phase; the forward gate is a release blocker for it** | Otherwise the ordering manufactures #12398 (P6, ruling §4). |
| 6 | **Redaction ships inside the "View MachineConfig" phase** | That feature *is* the leak (P16). Do not sequence redaction after the view. |
| 7 | **Secrets-bundle capture + cert-expiry visibility ship with the PKI store** | Retrofitting a bundle requirement after clusters are onboarded means re-onboarding them (P1, P5). |
| 8 | **`schematic_id` on the node record from the first schema** | It is the link between provisioning and upgrades. Without it, upgrades are unsafe to ship at all (P9c). The column is free. |
| 9 | **UUID-keyed inventory, multi-cluster scoping, deadlines, error taxonomy, `--dry-run`, per-cluster read-only lock: all foundation** | Every one of these is cheap now and expensive to retrofit across an existing codebase. |
| 10 | **QEMU (Tier 2) before the provisioning phase is declared done; one real amd64 hardware pass before v1** | Docker structurally cannot exercise maintenance mode, install, or upgrade; QEMU on darwin/arm64 cannot exercise amd64 installer paths. |

### Suggested phase structure

#### Phase 0: Foundation skeleton
**Rationale:** Zero Talos dependency, so it is fully testable on day one and parallelises three ways.
**Delivers:** Go module (`toolchain go1.26.7` pinned); `store` (entity-shaped interface, `fsstore`, atomic writes, `flock` + per-entity mutex + `rev` CAS, `VERSION` + forward-only migrations, permission guard); `audit` (JSONL, daily, hash chain, intent-before/outcome-after); `auth` (argon2id with stored params, sessions rotated on login, sudo-mode, rate limiting); `httpapi` middleware chain + RFC 9457 problem+json; HTTPS-by-default with self-signed generation; default bind `127.0.0.1`; Vite scaffold + `embed.FS` (`all:` prefix); Taskfile with `build:go` depending on `build:web`.
**Addresses:** Local login, audit log, single binary.
**Avoids:** P16 (security foundations); establishes the `store` escape hatch that makes ruling §1 safe.
**Parallel:** store/audit/auth ∥ frontend scaffold ∥ TLS + config loading.

#### Phase 1: Transport seam + `talossim` ⚠️ BLOCKING
**Rationale:** The highest-leverage item in the project. Nothing after this is testable without it.
**Delivers:** `Dialer` **and** `DiscoverySource` interfaces; `direct` + `fake` implementations; `pool` (deadlines enforced structurally, reads-only retry allowlist, circuit breaker, keepalive OFF initially); `NodeClient` / `MaintenanceClient` as distinct types; the shared **error taxonomy**; whole-binary `--dry-run`; **`talossim`** — real protobufs, real mTLS, real in-memory COSI via `protobuf/server.NewState`, with scripted failures (`go_silent(90s)`, `reject_apply`, `second_bootstrap_returns_AlreadyExists`, `flap_connection`, `slow_log_consumer`, `ip_changes_on_reboot`, `etcd_down`, `k8s_down`, `version_out_of_supported_range`); contract-test skeleton.
**Avoids:** P17, P14, P15.
**Parallel:** Phase 1b has no Talos dependency at all.

#### Phase 1b: Image Factory client (parallel with Phase 1)
**Rationale:** Self-contained, touches no Talos code, and it unblocks provisioning — one of the four differentiators.
**Delivers:** `SchematicCreate` / `SchematicGet` via the official client; local `Schematic.ID()` pre-check; version-scoped extension catalog; **pre-release filtering**; **pre-POST extension-name validation + post-POST model probe** before marking a schematic usable; version-dependent installer repo-name resolution; ISO/installer/PXE URL construction with **no hardcoded arch**; the **kernel-args-vs-installer warning at authoring time**; `BrokenVersions` to grey out bad versions.
**Avoids:** P9 (all five sub-bugs).
**Testable:** httptest against recorded Factory responses.

#### Phase 2: Inventory + cluster import + health
**Rationale:** The spine. Import first, because the cluster already exists.
**Delivers:** `model.ClusterID` / `MachineID` as distinct types; flat UUID-keyed machine records with nullable `cluster_id`, `schematic_id`, lock flag, last-seen; **cluster import** (bundle from a CP node → fingerprint → **live connectivity proof with a minted cert**) and create; `gen secrets` reachable only from create; **client-cert expiry parsing + banner**; per-cluster **read-only mutation lock**; `HealthLevel` + `Field[T]`; per-node COSI watch supervisors + jittered heartbeat; degradation state machine (never delete data because a node is unreachable); Talos↔K8s **compatibility matrix as data** + "distance from the edge"; cluster-scoped read routes; dashboard + node detail UI.
**Avoids:** P1, P5, P15, P6 (matrix half).
**Sandbox:** **Tier 1 Docker lands here** — spike it early; if `pkg/provision` fails on darwin/arm64, Tier 1 collapses into Tier 0 and the fake-drift risk rises materially.

#### Phase 3: Streaming (parallel with Phase 4)
**Rationale:** Genuinely valuable and genuinely off the Core Value path. **This is the phase to cut or shrink if v1 is at risk.**
**Delivers:** `streamhub` (topics, refcount, ring buffer, non-blocking fan-out, visible gap markers, sequence numbers); **one multiplexed SSE endpoint per tab** with `Last-Event-ID` replay; explicit stream state in the UI (live / reconnecting / node rebooting / disconnected); log + dmesg via xterm.js.
**Avoids:** P14, the HTTP/1.1 connection cap.

#### Phase 4: Jobs engine + node actions (parallel with Phase 3)
**Rationale:** Constraint #2. The engine must exist before the hardest job runs on it.
**Delivers:** `jobs` (`Step{Act, Observe}`, intent→act→confirm with fsync, per-cluster mutating lease, restart recovery, step-boundary cancellation, progress → `streamhub`); reboot / shutdown / **reset with the full P2 dialog** (disk enumeration, wipe-scope and reboot controls, effective flags displayed, type-the-hostname); server-side confirmation tokens; sudo-mode gating; audit on every mutation; `202 Accepted` + job ID for all destructive endpoints.
**Avoids:** P2, P16.
**Highest-value test in the project:** kill and restart the process mid-step; assert correct resume-or-park.

#### Phase 5: Config domain
**Rationale:** Provisioning needs generation; the patch library is a differentiator; the diff is the flagship safety feature.
**Delivers:** Generation from the imported bundle with a **pinned Talos version contract per cluster**; versioned append-only patch store (**strategic merge only — no RFC 6902**); **structural diff of actually-merged rendered configs** with list-growth and duplicate detection; per-patch idempotency test; validation + dry-run; **required-apply-mode computation from the verified allowlist as a tested constant**; `.machine.install` labelled "takes effect at next install/upgrade"; second staged apply refused; `--mode=try` recommended for `.machine.network` diffs with a 60 s countdown; **`redact` package + the entropy walk-test**, used by the config view, the raw tab, the diff, the API response and the audit log — one function, five call sites.
**Avoids:** P7, P11, P12, P16.

#### Phase 6: PROVISIONING — the Core Value
**Rationale:** The convergence point and the reason for phases 1b, 2, 4, 5. **Make it a phase, not a milestone.**
**Delivers:** `discovery` (bounded-concurrency subnet scan on `:50000` + manual IP, behind `DiscoverySource`; a configured node at the target IP reported as such, never "nothing found"); the full persisted state machine as a `JobKind`; **UUID re-verification immediately before apply**; optional `--cert-fingerprint` pinning with honest UI copy; **auto-populate `.machine.install.image` from the same schematic ID as the ISO**; the **three-way reappearance probe** with elapsed time against an expected budget (never a spinner); the **four-layer bootstrap guard** with a separate recovery flow; server-driven wizard UI that survives a closed tab.
**Avoids:** P4, P8, P13, P18.
**Entry requirement:** Tier 2 QEMU working, **and a machine that is allowed to be blank** (third mini-PC or a VM). Budget this as a phase entry requirement, not a stretch goal.
**Acceptance, binary:** a blank machine becomes a healthy cluster node without opening a terminal.

#### Phase 7: Upgrades (+ etcd management in parallel)
**Rationale:** Depends on the jobs engine, the health gate, and `schematic_id`.
**Delivers:** Rolling Talos upgrade `JobKind` via **`LifecycleClient.Upgrade` (streaming)**; the **full P3 health gate** (learners excluded, raft convergence, alarms, refuse at ≤2 voting members, inputs rendered, re-evaluated before every node); installer ref built from `schematic_id` with **refusal to upgrade a node whose schematic is unknown** (offer to read it from the node); **kernel-args/GRUB drift precondition** blocking the one-click path; **computed intermediate-minor upgrade chain**, no "latest" button, pre-releases filtered; **the K8s forward-compatibility gate (release blocker)**; **post-upgrade declared-vs-observed verification** as the phase's acceptance criterion; stop-after-current-node cancellation with honest copy; non-fatal API warnings captured and surfaced. In parallel: etcd member list / remove (**always show hostname/UUID, never raw hex IDs**) / snapshot with the no-quorum `cp /var/lib/etcd/member/snap/db` fallback.
**Avoids:** P3, P6, P9c, P10.

#### Phase 8: Hardening + real hardware pass
**Rationale:** The only phase that genuinely requires the homelab.
**Delivers:** Backup/restore subcommands; `audit verify`; **supported-version-range enforcement as a tested check** (v1.12–v1.14, RCs opt-in); Docker/Compose packaging with correct volume ownership and non-root container; **one verification pass on real amd64 hardware**.

### Phase Ordering Rationale

- **The dependency root is single:** the gRPC client + PKI store. Everything hangs off it, which is why the seam and the fake are Phase 1 and why getting the transport interface right on day one matters.
- **The ordering is defensive against P18.** Phases 0–5 exist to unblock Phase 6 and to make it safe; each should be justified against that, and anything that unblocks neither is a v1.x candidate. Streaming (Phase 3) is the deliberate exception, isolated so it can be cut.
- **A walking skeleton should precede the polished Phase 6.** Get the ugly end-to-end path — hardcoded schematic, manual IP, generated config, apply, join — running against QEMU as early as it is technically possible. It converts the scariest unknowns (the four-minute silence, double-bootstrap, wrong-machine) into known problems in week three rather than month six.
- **Parallelism is real, not aspirational:** 0's three tracks; 1 ∥ 1b; 3 ∥ 4; backend ∥ frontend within 2 once the `Field[T]` response shape is fixed; etcd management ∥ upgrade orchestration in 7.

### Release blockers (not nice-to-haves)

- `talossim` (Phase 1) — without it nothing is verifiable
- Server-side redaction, shipped with the config view
- Secrets-bundle capture + cert-expiry visibility, shipped with the PKI store
- `schematic_id` persisted per node, from the first schema
- UUID re-verification before apply
- Four-layer bootstrap guard
- Reset dialog with wipe-scope, reboot control and disk enumeration
- Full P3 health gate before any rolling upgrade ships
- K8s forward-compatibility gate before the Talos upgrade ships
- Post-upgrade declared-vs-observed verification
- Deadline enforcement + error taxonomy + partial rendering on every read surface
- One QEMU pass through the Core Value, and one real amd64 hardware pass

### Research Flags

**Phases likely needing `--research-phase` during planning:**
- **Phase 1** — `talossim` viability rests on two unverified assumptions: `cosi-project/runtime/pkg/state/impl/inmem` was not directly verified (the `protobuf/server.NewState(CoreState)` wrapper was, so worst case is writing a small `CoreState`), and the exact `MachineService` streaming RPC signatures (`Logs`, `Dmesg`, `Events`) were not enumerated. Also: the `machinery` Go API surface was not examined in depth by the pitfalls research — verify method names, streaming shapes and option structs against the pinned version here.
- **Phase 2** — the Tier 1 Docker provisioner on `darwin/arm64` was **not tested**. Spike it first; the fallback plan changes the risk profile of every later phase.
- **Phase 6** — QEMU on darwin/arm64 was confirmed in source (`preflight_darwin.go`) but **not executed**; `sudo` + `vmnet-shared` may be painful, with a nested Linux VM (Lima/Colima/UTM) as fallback. Maintenance-mode resource sensitivity (which resources are readable insecurely, defined by the `sensitivity` field on resource *definitions*) is discoverable only from a real node — the wizard must not be built on resources that turn out not to exist there.
- **Phase 7** — Kubernetes upgrade orchestration (prepull → static pods → kube-proxy → kubelet) was not researched in depth; the compatibility matrix needs a maintained data source; Talos v1.14's upgrade surface was not reviewed.

**Phases with standard patterns (skip research):**
- **Phase 0** — argon2id, sessions, CSRF, atomic file writes, JSONL audit. All well-trodden; the library choices are already verified.
- **Phase 3** — SSE + ring buffers + fan-out is a solved shape; the design is fully specified in ARCHITECTURE Pattern 5.
- **Phase 5** — `machinery` does the merge, the diff and the validation. The work is wiring and the redaction test, not discovery.

---

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | **HIGH** | Module source downloaded and read on disk at v1.13.9 (every import path confirmed); live `POST` to `factory.talos.dev` returned a real schematic ID; npm registry `latest` for every frontend package. Two deprecations and one nonexistent package were caught that memory would have got wrong. QEMU-on-darwin is MEDIUM-HIGH (source confirms, not executed). |
| Features | **MEDIUM-HIGH** | Omni and Talos v1.13 docs read in full from the vendor's own index; OSS landscape verified via the GitHub API (`archived`, `pushed_at`, license). `talos-pilot` detail is self-reported by its maintainer. Prioritization and the MVP cut are grounded opinion. Superseded on three technical points by PITFALLS (ruling §3). |
| Architecture | **MEDIUM-HIGH** | API surfaces verified on pkg.go.dev; the `pkg/machinery` module split read verbatim from `go.mod`; `RegisterMachineServiceServer` / `RegisterLifecycleServiceServer` confirmed exported (this is what makes `talossim` viable). The structural design — package layout, job engine, state machine, security architecture, build order — is opinionated synthesis. |
| Pitfalls | **HIGH** | Primary vendor docs quoted verbatim (allowlist, reset flags, merge semantics, the `extraKernelArgs` warning, "renewed at least once a year"); six real `siderolabs/talos` issues read via the GitHub API with verbatim error output; **first-hand Image Factory experiments** including the bogus-extension 200-then-400 finding. gRPC keepalive specifics are MEDIUM (inferred from grpc-go defaults given an absent enforcement policy in `pkg/grpc/factory`) — verify in the sandbox; it is a five-minute test. |

**Overall confidence: HIGH** on what to build and what will hurt; **MEDIUM** on execution details that only a real node can settle.

### Gaps to Address

- **No live Talos node was available to any researcher.** Everything about node *behaviour* — timing, error strings, stream death, maintenance-mode resource sensitivity — is documented, not observed. → This is P17 applied to the research itself. Handle by recording fixtures from Tier 1 in Phase 2 and holding "verified against real Talos" open per feature.
- **Docker provisioner on darwin/arm64 untested; QEMU on darwin/arm64 unexecuted.** → Spike both early (Phase 2 and pre-Phase-6 respectively). Fallbacks: Tier 1 collapses into Tier 0 (accept higher drift risk, record fixtures later from real hardware); nested Linux VM for QEMU.
- **gRPC keepalive tolerances on `apid` are recommended, not measured.** → Start with keepalive OFF, make the parameters configurable, and run a 1-hour idle soak plus an explicit `ENHANCE_YOUR_CALM` test in Tier 1.
- **`--wipe-mode` vs `--system-labels-to-wipe` precedence is undocumented.** → Test in the sandbox **before** wiring the reset dialog (Phase 4).
- **Whether Talos ever creates etcd learner members on its own is unconfirmed.** → Filter learners in the health gate regardless; it is free.
- **Talos v1.14 was not reviewed by any researcher**, and 1.13 goes EOL at its release (~2026-08-30). → Ruling §5 sets the range; validate v1.14's `LifecycleService` and config-generation surface during Phase 1 and again in Phase 7.
- **Kubernetes upgrade orchestration was not researched in depth**, and the compatibility matrix needs a maintained source. → Phase 7 research flag; ship `upgrade-k8s --dry-run` before owning the orchestration.
- **The `machinery` API surface was not examined in depth by the pitfalls research.** → Reconcile against the pinned version in Phase 1.

---

## Sources

### Primary (HIGH confidence)
- Downloaded module source at `/Users/holz/go/pkg/mod/github.com/siderolabs/talos/pkg/machinery@v1.13.9` — every import path, `client/connection.go`, `client/client.go`, `config/generate/example_test.go`, `config/contract.go`, `config/configpatcher/*`, `config/configdiff/*`, `resources/hardware/system_information.go`, `api/machine/{machine,lifecycle}_grpc.pb.go`
- `siderolabs/talos` v1.13.9 `pkg/provision/providers/docker/{docker,node}.go`, `pkg/provision/providers/qemu/{preflight,launch}_darwin.go`
- `siderolabs/image-factory` v1.5.1 `pkg/client/client.go`, `pkg/schematic/schematic.go`, `docs/api.md`
- Live Image Factory API (2026-08-27): `POST /schematics` (real ID returned), `GET /versions`, `GET /version/<v>/extensions/official`, OCI manifest probes, bogus-extension 200-then-400 experiment
- Talos v1.13 official docs read in full — apply modes + allowlist, configuration patches, reproducible machine configuration, resetting a machine, disaster recovery, etcd maintenance, upgrading Talos, upgrading Kubernetes, support matrix, CA rotation, cert management, the insecure flag, Image Factory, network connectivity, production notes, CLI reference (169 KB)
- Sidero Omni documentation index (`docs.siderolabs.com/llms.txt`) and feature pages
- `proxy.golang.org` and `registry.npmjs.org` for every pinned version; GitHub REST API for repo state and releases
- pkg.go.dev for `machinery/client`, `machinery/api/machine`, `machinery/config/generate`, `pkg/provision`, COSI `state` and `state/protobuf/server`

### Secondary (MEDIUM confidence)
- `siderolabs/talos` issues #12398 (Talos/K8s skew deadlock), #12210 (SDBoot kernel-arg warning, 1.12 patch issues), #12562 (JSON patches vs multi-document), #12462 (MachineConfig serialization), #13109, #13681 — user reports with verbatim error output
- `-e` vs `-n` / apid proxy-and-aggregate semantics — multiple corroborating sources
- `Handfish/talos-pilot` README — maintainer self-reported feature list
- grpc.io keepalive guide, grpc/grpc keepalive doc, grpc-go #5564 and #1341

### Tertiary (LOW confidence / needs validation)
- gRPC keepalive `MinTime` of 5 minutes against Talos `apid` — inferred from grpc-go defaults given the absent enforcement policy. **Verify in Tier 1.**
- Docker-vs-QEMU provisioner behaviour on `darwin/arm64` — directionally solid, exact boundaries unexercised
- MAAS / Tinkerbell — judged out of the design space early, not examined in depth. Revisit only if netboot is promoted to in-scope.

---
*Research completed: 2026-08-27*
*Ready for roadmap: yes*
