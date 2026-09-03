# Feature Research

**Domain:** Bare-metal Talos Linux node & cluster lifecycle control plane (out-of-cluster, single-operator)
**Researched:** 2026-08-27
**Confidence:** MEDIUM-HIGH (see [Confidence & Sourcing Note](#confidence--sourcing-note))

---

## Executive Finding: Does an OSS Talos Web UI Already Exist?

**No. The gap is real.** Verified against the GitHub API on 2026-08-27:

| Project | Kind | Status | Stars | Verdict |
|---------|------|--------|-------|---------|
| `siderolabs/theila` | **Web UI** (TypeScript) | **ARCHIVED 2022-05-27** | 47 | The only OSS Talos web UI ever shipped. Killed and superseded by Omni. |
| `siderolabs/omni` | Web UI (Go) | Active | 1358 | **BUSL** — not open source. Production use requires a support contract. |
| `Handfish/talos-pilot` | **TUI** (Rust, MIT) | Active, pushed 2026-07 | 243 | Closest functional analogue. Terminal-only. See seam analysis below. |
| `aenix-io/talm` | CLI/GitOps (Apache-2.0) | Active | 484 | Config templating. No UI, no runtime ops. |
| `budimanjojo/talhelper` | CLI (BSD-3) | **ARCHIVED** | 694 | Config generation only. |
| `home-operations/tuppr` | **In-cluster** controller (AGPL) | Active | 298 | Upgrades Talos/K8s — but dies with the cluster. |
| `alperencelik/talos-operator` | **In-cluster** operator (Apache-2.0) | Active | 72 | Same structural flaw: in-cluster. |

**Conclusion:** There is **no actively maintained open-source web UI for Talos node/cluster management.** Every OSS alternative is either (a) a terminal UI, (b) a declarative CLI config generator, or (c) an in-cluster controller that is unavailable precisely when you need it. holzkube's positioning — *out-of-cluster web UI* — is unoccupied territory.

### The talos-pilot seam (important — it overlaps more than expected)

talos-pilot is **not** just a monitor. It already does: node/service monitoring, log streaming, etcd quorum & alarms, process tree, network + pcap, disk enumeration, PKI expiry, drain/reboot/rolling ops, an interactive **bootstrap wizard**, **config generation** (talosconfig / controlplane.yaml / worker.yaml), **apply-config**, and **cluster bootstrap**. It logs operations to `~/.talos-pilot/audit.log`.

It explicitly does **not** do: Talos or Kubernetes **upgrades**, **Image Factory** integration, **persistent multi-cluster inventory**, a **versioned reusable patch library with diff**, or any **web** interface.

> **Roadmap consequence:** "read-only Talos dashboard" is *already solved* by talos-pilot for anyone willing to use a terminal. holzkube's defensible ground is the four things talos-pilot lacks — **Image Factory→ISO provisioning, health-gated rolling upgrades, persistent inventory, and versioned patches with diff** — delivered in a browser. Do not let v1 collapse into "another read-only dashboard."

---

## The Cluster-Health Axis

holzkube runs **outside** the cluster specifically to be useful when the cluster is broken (PROJECT.md constraint). Every feature below is tagged with what it actually needs:

| Tag | Meaning | Why it matters |
|-----|---------|----------------|
| **NONE** | Works against a bare/maintenance-mode machine. No cluster, no PKI. | The disaster-recovery floor. |
| **NODE** | Needs one node reachable with Talos API + valid talosconfig mTLS. | Works with a dead control plane. |
| **ETCD** | Needs etcd quorum. | Fails during the exact outage you're debugging. |
| **K8S** | Needs the Kubernetes API server up. | Most fragile. |

**Design rule:** the more features that sit at NONE/NODE, the more holzkube justifies its out-of-cluster architecture. Anything at K8S must degrade gracefully rather than blanking the whole UI.

---

## Feature Landscape

### Table Stakes (Users Expect These)

Missing these = the tool is not a Talos management plane.

| Feature | Why Expected | Complexity | Health | Notes |
|---------|--------------|------------|--------|-------|
| **Node inventory (persistent, multi-cluster)** | Omni, Rancher, MAAS all have it. Without persistence you are a CLI with extra steps. | MEDIUM | NONE | The spine everything else hangs off. Must survive node-down. Store IP, UUID, MAC, role, cluster, schematic ID, last-seen. |
| **Per-node health/version/hardware panel** | Baseline of every competitor. `talosctl get` provides it directly. | LOW | NODE | Talos/K8s version, CPU/RAM, disks, network links. All via COSI resources over gRPC. |
| **Talos service status** (etcd, kubelet, apid, machined, trustd) | `talosctl service` is the #1 triage command. | LOW | NODE | Trivial once the gRPC client exists. Highest signal-per-line-of-code in the product. |
| **Live log + dmesg streaming** | talos-pilot, Omni, k9s all stream. Non-streaming logs are useless for boot debugging. | MEDIUM | NODE | Server-streaming gRPC → WebSocket/SSE → xterm.js. Backpressure and reconnect are the real work. |
| **Cluster overview** (CP vs worker, etcd members, health) | The "is my cluster OK" glance. | MEDIUM | ETCD (partial NODE) | Degrade: show per-node NODE-level truth when etcd is down. Do not blank the page. |
| **View MachineConfig (rendered + raw YAML)** | You cannot trust a config tool that hides the config. | LOW | NODE | `talosctl get mc v1alpha1 -o jsonpath={.spec}`. **Must redact secrets in the UI.** |
| **Node actions: reboot / shutdown / reset** | Table stakes for any node manager. | LOW | NODE | Reset needs `--graceful` toggle exposed — graceful is impossible on single-node etcd. Confirmation gate mandatory. |
| **Apply config with mode selection** | Talos has 6 apply modes; hiding them makes the tool a toy. | MEDIUM | NODE | auto / no-reboot / staged / try / reboot. See [Apply-mode trap](#pitfall-the-apply-mode-trap). |
| **Cluster bootstrap** (first CP node) | Without it you cannot create a cluster, only join one. | LOW | NONE→ETCD | One-shot, non-idempotent, catastrophic if run twice. Must be guarded. |
| **etcd member list / remove / snapshot** | The core recovery workflow. Omni and talos-pilot both expose it. | MEDIUM | ETCD (snapshot degrades) | `talosctl etcd snapshot` fails without quorum — fall back to `cp /var/lib/etcd/member/snap/db`. |
| **Talos rolling upgrade with health gate** | Omni's headline feature. Ungated upgrades break quorum. | HIGH | NODE + ETCD gate | v1.13 has a **streaming** `LifecycleService.Upgrade` API with real-time progress. Use it. |
| **Kubernetes upgrade** | Talos decoupled OS and K8s upgrades since v1.0; both are needed. | HIGH | K8S | Prepull → static pods → kube-proxy ds → kubelet. Reimplements `talosctl upgrade-k8s` logic. |
| **Destructive-action confirmation** | Tool holds cluster PKI and can wipe machines. | LOW | — | Type-the-node-name pattern. Cheap, prevents the worst class of incident. |
| **Local auth (user+password, session)** | Anything holding cluster PKI on a network needs a login. | LOW | — | argon2id + session cookie, per PROJECT.md. Behind an interface for later OIDC. |
| **Audit log** | Omni ships it; PROJECT.md requires it at v1. | LOW | — | Copy Omni's shape exactly: JSONL, one file per day, 30-day retention. ~200 LOC. |

### Differentiators (Why Build This Instead of Using talosctl)

| Feature | Value Proposition | Complexity | Health | Notes |
|---------|-------------------|------------|--------|-------|
| **Blank-machine → cluster-join in the browser** | **The Core Value.** No OSS tool does this in a web UI. Omni needs SideroLink; talos-pilot needs a terminal. | HIGH | NONE→ETCD | See [full decomposition](#the-provisioning-flow-decomposed). This is the product. |
| **Image Factory schematic builder** | Turns "pick extensions from a list" into a first-class, *saved, reusable* object. Nobody OSS does this. | MEDIUM | NONE | POST YAML → content-addressable ID. Idempotent, so retries are free. Store schematic ID **on the node record** — it is the missing link between "what ISO did I boot" and "what installer must I upgrade with." |
| **Maintenance-mode discovery (scan / manual IP)** | Replaces Omni's entire SideroLink+PXE+agent stack with a subnet scan. Radically simpler and needs no infrastructure. | MEDIUM | NONE | Scan `:50000`, `--insecure` probe for version+UUID. **This is holzkube's architectural bet — and it is a good one at homelab scale.** |
| **Versioned, reusable config patches with diff** | talosctl gives you a YAML file and hope. A diff before apply is the single biggest safety win available. | MEDIUM-HIGH | NODE | Render current → render proposed → structural diff. Pairs with dry-run + `--mode=try` auto-revert. |
| **Health-gated rolling upgrade with live progress & cancel** | Omni-grade orchestration, self-hosted. Prevents the #1 cluster-killer (upgrading CP nodes past quorum). | HIGH | NODE + ETCD | Depends on inventory + health. Node-at-a-time, verify etcd will survive the node leaving *before* touching it. |
| **Works when the cluster is down** | Structural advantage over tuppr / talos-operator / Kubernetes Dashboard, all of which are in-cluster. | — (architectural) | — | Free, but only if you *enforce* it: never make NODE-level features depend on a K8S-level call. |
| **Local Talos sandbox for dev/test** | PROJECT.md constraint: no reliable hardware access. Also becomes the demo/onboarding path. | MEDIUM | NONE | `talosctl cluster create docker` provisioner + Talos API fakes. **Must be an early phase, not a late one.** |
| **Node "lock"** (exclude from upgrades/patches) | Omni has it; cheap to add; makes staged rollouts safe on a small cluster. | LOW | — | A boolean on the inventory record. High value/cost ratio. |
| **Single binary, embedded UI** | Homelab operators reward zero-dependency deploys. Omni on-prem needs etcd + registries + a signing key. | LOW | — | `embed.FS`. Already a Constraint in PROJECT.md. |

### Anti-Features (Deliberately NOT Building)

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| **Workload management (Pods, Deployments, Helm)** | "It's a cluster UI, why can't I see my pods?" | Unbounded scope. k9s/Lens/Headlamp/Rancher are mature, free, and better. You would ship a worse Headlamp and never finish provisioning. Also inverts the value prop: workload views need K8S health, which is exactly when holzkube should still work. | **Deep-link out.** Broker a kubeconfig and hand off. The seam is the port: **holzkube owns :50000 (Talos machine API), k9s/Headlamp own :6443 (Kubernetes API).** talos-pilot draws the same line explicitly. |
| **Own metrics/monitoring pipeline** | "Show me CPU graphs over time." | You would build a bad Prometheus. Requires a TSDB, retention policy, alerting — a second product. Omni doesn't do this either; it *exports* to Prometheus. | Show only **instantaneous** values the Talos API returns. If someone wants history, they run Prometheus. Optionally expose a `/metrics` endpoint later (Omni's exact strategy). |
| **SideroLink / WireGuard tunnel** | "Omni has it." | It is Omni's *entire* registration mechanism and a massive build: WG keypair lifecycle, overlay IPAM, NAT traversal (which Omni warns is not universally compatible), join-token rotation. Buys nothing when nodes are on the same L2. | Direct gRPC to node IPs. Keep the transport behind an interface (already a PROJECT.md decision) so a tunnel is retrofittable. |
| **PXE / netboot infrastructure** | "Real provisioning is netboot." | Omni's bare-metal provider needs a DHCP proxy (:67, :4011), TFTP (:69), iPXE binary patching, and a Talos "Agent Mode" extension. That is a networking product, not a UI feature, and it perturbs the operator's home network. | ISO/USB for v1. Image Factory already exposes a PXE frontend (`pxe.factory.talos.dev`) — so v2 netboot is a *URL*, not a rewrite, provided schematics are modeled properly now. |
| **Multi-user RBAC / OIDC / SAML** | Every enterprise tool has it. | Omni ships Admin/Operator/Auditor roles, ACLs, SAML integrations for **six** IdPs. For a single-operator homelab this is pure cost and a large attack surface with no user. | Single local account. Auth behind an interface. Audit log records "who" as a constant — the *format* is what matters, so it stays useful if multi-user ever lands. |
| **Generic unguarded YAML editor** | "Just let me edit the config." | An editor with no schema, no diff, and no dry-run is a footgun that reboots production hardware. Talos silently ignores some fields and rejects others only at apply time — Omni documents this exact failure mode ("patch applied, value in merged config, machine ignores it"). | **Structured patches + validate + dry-run + diff + `--mode=try`.** Raw YAML is *viewable* (table stakes) but edits flow through the guarded path. |
| **Cluster templates / full GitOps declarative sync** | Omni has them; talm and talhelper exist. | A second, competing source of truth. Omni needs six config sources and a whole troubleshooting page to explain when they disagree. For one operator and one cluster, imperative patches + audit log + diff give the same safety at a fraction of the cost. | Imperative patches with versioning. Export-to-YAML later if GitOps is genuinely wanted. |
| **Autoscaling (Cluster Autoscaler / Karpenter)** | Present in Omni's docs. | Meaningless on fixed bare-metal mini-PCs. Nothing to scale into. | Nothing. |
| **Machine classes as label-matcher DSL** | Omni's model for auto-allocating machines. | A query language for selecting among ~5 machines. Classic enterprise-pattern cargo-culting. | Plain tags/labels on inventory records + manual selection. Revisit past ~20 nodes. |
| **In-cluster deployment option** | "Just run it as a Deployment." | Directly contradicts the Core Value. It would be unavailable during the outage it exists to fix. | Out-of-cluster only. Ship Docker Compose. |
| **Agent installed on nodes** | Makes discovery/telemetry easier. | Talos is immutable by design — an agent means a system extension, which means a custom schematic, which means every node needs re-imaging to onboard. Adds a failure mode (agent down ≠ node down). | Agentless. The Talos machine API *is* the agent. Preserve this. |
| **BMC / IPMI / Redfish power control** | Omni's bare-metal provider has it; genuinely useful for a dead node. | Per-vendor auth quirks, credential storage for a *second* security domain, and consumer mini-PCs frequently have no BMC at all. | Defer to v2. If the hardware gains BMCs, add it as an optional per-node field. Not v1. |

---

## The Provisioning Flow, Decomposed

> This is holzkube's Core Value. Each sub-step is a place the flow can stall; the UI's job is to make the *current* step and the *reason for stalling* unambiguous. A provisioning wizard that says only "failed" is worthless.

| # | Sub-step | Health | What Can Go Wrong | What the UI Must Surface |
|---|----------|--------|-------------------|--------------------------|
| 1 | **Author schematic** (extensions, kernel args, META) | NONE | Extension name invalid, or unavailable for the chosen Talos version (Factory rejects the model). | Version-scoped extension picker fetched from Factory, not a free-text field. Show which extensions exist for the selected version. |
| 2 | **POST schematic → Factory** | NONE | Factory unreachable; air-gapped network. | Return the schematic ID and **persist it**. Idempotent (content-addressed) so retry is always safe — say so. |
| 3 | **Resolve ISO/installer URLs** | NONE | Wrong arch; wrong model type. | Show the exact URLs for ISO **and** installer. ⚠️ **`installer` and `initramfs` honor system extensions ONLY — kernel args and META are silently ignored.** Warn when a schematic sets kernel args, because the ISO and the installed system will then differ. |
| 4 | **Write ISO to USB, boot machine** | — | Entirely outside the software. | Be honest: give the download link and a checklist. Do not pretend to automate it. |
| 5 | **Machine reaches maintenance mode** | NONE | **(a) A prior Talos install exists on disk → the machine boots *that*, not the ISO.** (b) Static-IP network → no DHCP → node never appears on the network at all. (c) Secure Boot mismatch. | Document/warn (a) prominently — it is the #1 re-provisioning failure. For (b), state plainly that DHCP is the only zero-touch path; static IP requires embedded config or `talos.config.early` kernel args. |
| 6 | **Discover the machine** | NONE | Wrong subnet; host firewall; scan finds nothing; scan finds *unexpected* machines. | Subnet scan on `:50000` **plus** a manual-IP escape hatch (PROJECT.md already requires both). Show scan progress and range. Never auto-act on a discovered machine. |
| 7 | **Identify & verify the machine** | NONE | Cannot distinguish two identical mini-PCs. Maintenance mode uses a **self-signed** cert; neither side authenticates. | Show UUID, MACs, disks, Talos version — all readable via `--insecure`. Offer optional **`--cert-fingerprint` verification**; be explicit that the fingerprint is only obtainable from the physical/serial/IPMI console. This is a real MITM window; say so rather than hiding it. |
| 8 | **Choose role, cluster, install disk** | NONE | Wrong disk selected → wipes data. Wrong role → broken quorum. | Disk picker with size/model/serial/transport, system-disk flagged. **Warn on even-numbered control-plane counts** — quorum requires 1/3/5. |
| 9 | **Generate MachineConfig** | NONE | Cluster secrets missing/wrong → node joins nothing. Patch produces invalid YAML. **Installer image schematic ≠ ISO schematic → extensions vanish on install.** | Auto-fill `machine.install.image` from the *same* schematic ID used for the ISO. This is the highest-value automation in the entire flow — it is the mistake everyone makes by hand. |
| 10 | **Validate + dry-run + diff** | NONE | Field rejected at apply time; field silently ignored by Talos. | `talosctl validate` equivalent + full rendered diff. Distinguish "rejected" from "accepted but ignored." |
| 11 | **Apply config (`--insecure`)** | NONE | Wrong node targeted; connection drops mid-apply. | ⚠️ Only these APIs work in maintenance mode: **`apply-config`, `version`, `get` (non-sensitive resources only), `meta`, `reset`, `wipe disk`.** Design the wizard within that envelope — no service status, no logs, no dmesg pre-apply. |
| 12 | **Machine installs to disk, reboots** | NONE | Installer image pull fails (bad ref / no egress); disk too small; install hangs. | Node goes silent here — **this is the scariest gap in the whole flow.** Poll for the node coming back on mTLS; show an explicit "installing, expect ~N min" state with elapsed time, not a spinner. Surface console-access advice on timeout (Talos has no SSH by design). |
| 13 | **Bootstrap etcd** (first CP node **only**) | →ETCD | **Running it twice, or on a second node, is catastrophic.** | Make it a distinct, once-only, heavily-guarded action. Detect and refuse if the cluster already has etcd members. |
| 14 | **Node joins cluster** | ETCD | Endpoint wrong; time skew; CA mismatch; CNI absent so the node stays NotReady. | Watch etcd membership + node health. "Talos healthy but Kubernetes NotReady" is normal pre-CNI — say that instead of showing red. |
| 15 | **Node appears in dashboard** | NODE | Inventory record and live node fail to reconcile (IP changed via DHCP). | Key inventory on **UUID**, not IP. DHCP *will* move addresses. This is a data-model decision that is expensive to retrofit. |

**Cross-cutting requirement:** this flow is long-running and spans reboots. It needs a **persisted state machine per machine** (`discovered → configured → installing → joining → ready → failed`), not ephemeral wizard state in the browser. Closing the tab must not lose the provisioning job. *Treat this as a first-class architectural requirement, not a UI detail.*

---

## Feature Dependencies

```
[Talos gRPC client + talosconfig/PKI store]
    └──requires──> nothing (foundation)
            │
            ├──> [Local Talos sandbox]  ◀── MUST come early; everything else is blind without it
            │
            ├──> [Node inventory (UUID-keyed, multi-cluster)]
            │        ├──> [Node detail: health/version/hardware/services]
            │        │        └──> [Log + dmesg streaming]
            │        ├──> [Cluster overview / etcd members]
            │        │        └──> [etcd snapshot / member removal]
            │        └──> [Audit log]   (wraps every mutating action)
            │
            ├──> [View MachineConfig]
            │        └──> [Config patches (versioned, reusable)]
            │                 └──> [Diff view] ──> [Dry-run] ──> [Apply w/ mode]
            │                                                        │
            │                                                        └──> [Node lock]
            │
            └──> [Node actions: reboot/shutdown/reset]

[Image Factory schematic builder]
    └──> [ISO/installer URL resolution]
             └──requires──> [schematic ID stored on node record]
                      └──> [Talos rolling upgrade]   ◀── upgrade needs the SAME schematic
                                └──requires──> [health gate] ──requires──> [Cluster overview]

[Maintenance-mode discovery]
    └──> [PROVISIONING WIZARD] ──requires──> [inventory] + [schematic] + [config generation]
                                              + [cluster secrets store] + [persisted job state]
             └──> [Cluster bootstrap]  (first CP node only)

[Kubernetes upgrade] ──requires──> [kubeconfig brokering] + [cluster overview]

[Workload management] ──CONFLICTS──> [out-of-cluster value prop]   ✗ not built
[In-cluster deployment] ──CONFLICTS──> [Core Value]                ✗ not built
```

### Dependency Notes

- **Everything requires the gRPC client + PKI store.** Single hard root. Get the transport interface right on day one (PROJECT.md already commits to abstracting it).
- **Sandbox before features.** PROJECT.md flags no reliable hardware access. Every feature built without a sandbox is built blind. This is a *dependency*, not a nice-to-have.
- **Upgrades require the schematic ID.** A node imaged from a custom schematic must be upgraded with the *matching* `installer` image or its extensions silently disappear. If the inventory does not carry the schematic ID, upgrades are unsafe. **This links two features that look unrelated** — model it in the schema from the start.
- **Provisioning requires a cluster secrets store.** You cannot generate a joining MachineConfig without the cluster's PKI bundle (`talosctl gen secrets`). For a *new* cluster it must be created; for an *existing* one it must be imported. **Importing the author's existing homelab cluster is a distinct, easily-forgotten requirement** — PROJECT.md's Context says the cluster already exists, so import is the *first* path exercised, not a later one.
- **Diff requires rendering, not text-diffing.** Diffing raw YAML strings produces noise. Diff the *rendered merged config*, structurally.
- **Health gate requires cluster overview.** Rolling upgrades cannot exist before etcd member health does.
- **Inventory must be UUID-keyed.** DHCP moves IPs; IP-keyed inventory corrupts itself on the first lease change. Cheap now, expensive later.
- **Audit log is a cross-cutting wrapper.** Build it as middleware on the mutating-action path, not as a feature bolted on at the end.

---

## MVP Definition

### Launch With (v1)

Ruthlessly cut to: *prove the Core Value, plus the minimum to trust it.*

- [ ] **Local Talos sandbox + API fakes** — without it, nothing else is verifiable. Phase 1, no exceptions.
- [ ] **gRPC client + talosconfig/PKI store (0600)** — the root dependency.
- [ ] **Import existing cluster + node inventory (UUID-keyed, multi-cluster schema)** — the author's cluster already exists; this is the first real use.
- [ ] **Node detail: health, versions, hardware, disks, network, service status** — the trust-building surface; also cheap.
- [ ] **View MachineConfig (rendered + raw, secrets redacted)**
- [ ] **Node actions: reboot / shutdown / reset, with confirmation**
- [ ] **Image Factory schematic builder + ISO/installer URL resolution**
- [ ] **Maintenance-mode discovery (scan + manual IP)**
- [ ] **Config generation + validate + apply (`--insecure`) + persisted provisioning job state**
- [ ] **Cluster bootstrap (guarded, once-only)**
- [ ] **Local login (argon2id + session) + audit log (JSONL/daily)** — the tool holds cluster PKI; non-negotiable.

**v1 acceptance test:** a blank mini-PC becomes a healthy cluster node without the operator opening a terminal.

### Add After Validation (v1.x)

- [ ] **Live log + dmesg streaming** — high value, but streaming/backpressure/xterm.js is a real chunk of work and the Core Value does not require it. *Trigger: first time debugging a node feels blind.*
- [ ] **Config patches: versioned, reusable, with diff + dry-run + mode selection** — the safety layer. *Trigger: second config change made by hand.*
- [ ] **Talos rolling upgrade with health gate + live progress + cancel** — use the v1.13 streaming `LifecycleService.Upgrade` API. *Trigger: a Talos release the operator wants.*
- [ ] **etcd member list / remove / snapshot** (with the no-quorum `cp` fallback) — *Trigger: before the first upgrade, ideally.*
- [ ] **Node removal (cordon/drain → reset → de-inventory)**
- [ ] **Node lock** — trivial once upgrades exist.
- [ ] **Kubernetes upgrade** — genuinely HIGH complexity; decoupled from Talos upgrades since Talos v1.0, so it can wait.

### Future Consideration (v2+)

- [ ] **PXE/netboot** — defer; Image Factory's PXE frontend makes it a URL swap *if* schematics are modeled well now.
- [ ] **SideroLink-style tunnel transport** — only if nodes ever leave the LAN. Interface is already reserved.
- [ ] **OIDC / multi-user** — only on a real second user. Interface reserved.
- [ ] **BMC/IPMI/Redfish power control** — only if hardware acquires BMCs.
- [ ] **`holzkubectl` CLI** — REST API stays clean to permit it; do not build two interfaces at once.
- [ ] **Support bundle export** — `talosctl support` equivalent; nice for issue reports.
- [ ] **Prometheus `/metrics` endpoint** — export, never ingest. Omni's exact strategy.
- [ ] **ARM64 / SBC** — schematics are arch-parameterized; untested.

---

## Feature Prioritization Matrix

| Feature | User Value | Impl. Cost | Health | Priority |
|---------|-----------|-----------|--------|----------|
| Local Talos sandbox + fakes | HIGH (enabling) | MEDIUM | NONE | **P1** |
| gRPC client + PKI store | HIGH (enabling) | MEDIUM | — | **P1** |
| Node inventory (UUID-keyed, multi-cluster) | HIGH | MEDIUM | NONE | **P1** |
| Node detail / health / services | HIGH | LOW | NODE | **P1** |
| View MachineConfig | MEDIUM | LOW | NODE | **P1** |
| Node reboot/shutdown/reset + confirmation | HIGH | LOW | NODE | **P1** |
| Image Factory schematic builder | HIGH | MEDIUM | NONE | **P1** |
| Maintenance-mode discovery | HIGH | MEDIUM | NONE | **P1** |
| Config generation + apply + job state | HIGH | HIGH | NONE | **P1** |
| Cluster bootstrap (guarded) | HIGH | LOW | NONE | **P1** |
| Local login + audit log | HIGH | LOW | — | **P1** |
| Config patches + diff + dry-run | HIGH | MEDIUM-HIGH | NODE | **P2** |
| Log/dmesg streaming | HIGH | MEDIUM | NODE | **P2** |
| Talos rolling upgrade + health gate | HIGH | HIGH | NODE+ETCD | **P2** |
| etcd members / snapshot / remove | MEDIUM | MEDIUM | ETCD | **P2** |
| Cluster overview + etcd health | MEDIUM | MEDIUM | ETCD | **P2** |
| Node removal (drain→reset) | MEDIUM | MEDIUM | K8S | **P2** |
| Node lock | MEDIUM | LOW | — | **P2** |
| Kubernetes upgrade | MEDIUM | HIGH | K8S | **P3** |
| Support bundle export | LOW | LOW | NODE | **P3** |
| PXE / netboot | LOW (today) | HIGH | NONE | **P3** |
| BMC power control | LOW | MEDIUM | — | **P3** |
| Workload management | — | — | — | **✗ never** |
| Own metrics pipeline | — | — | — | **✗ never** |
| Multi-user RBAC | — | — | — | **✗ never (v1)** |

---

## Competitor Feature Analysis

| Feature | Sidero Omni (BUSL) | talos-pilot (TUI, MIT) | holzkube's Approach |
|---------|--------------------|------------------------|---------------------|
| **Machine discovery** | SideroLink/WireGuard tunnel, mandatory; egress :443 + WG port; NAT-incompatible in some configs | Manual node/endpoint config | **Subnet scan on :50000 + manual IP.** No tunnel, no agent, no infra. Simpler and sufficient on one L2. |
| **Bare-metal provisioning** | PXE + DHCP proxy + TFTP + iPXE + Agent Mode extension + BMC; wipes all disks on accept | Bootstrap wizard, maintenance mode, config gen + apply | **ISO/USB + maintenance mode + Factory schematic.** No network infrastructure required. |
| **Image customization** | ExtensionsConfigurations + KernelArgs as separate first-class resources | None | **Image Factory schematic as a saved, reusable object,** linked to the node record and reused for upgrades. |
| **Config model** | Six sources (patches, extensions, kernel args, manifests, platform, defaults); direct `talosctl` **blocked** at API layer | Generates controlplane/worker YAML from templates | **Cluster-level + node-level patches, merged, diffed, dry-run.** Two scopes, not six. `talosctl` never blocked. |
| **Talos upgrade** | Rolling, CP-first, etcd-health-gated, drain/cordon, `upgradeStrategy` concurrency, node locking | Not supported | **Rolling, one node at a time, etcd health gate, live progress, cancellable.** Uses v1.13 streaming upgrade API. |
| **K8s upgrade** | Prepull → static pods → kube-proxy → kubelet; bootstrap manifests shown as diff, not auto-applied | Not supported | Same sequence, deferred to v1.x. Adopt the "show manifest diff, don't auto-apply" behavior — it is correct. |
| **etcd backup** | Scheduled + manual to S3, with restore flow | Quorum/alarm/leader diagnostics | Manual snapshot to local disk v1.x. **Include the no-quorum `cp /var/lib/etcd/member/snap/db` fallback** — the normal path fails exactly when you need it. |
| **Audit log** | JSONL, daily files, 30-day retention, Auditor/Admin role | `~/.talos-pilot/audit.log` | **Copy Omni's format verbatim.** Single user, so no roles. Low cost, high credibility. |
| **Auth** | SAML/OIDC, 6 IdP integrations, ACLs, 3 roles, service accounts | None (inherits talosconfig) | Single local account (argon2id), behind an interface. |
| **Runs when cluster is down** | Yes (out-of-cluster) — but needs SideroLink up | Yes (direct talosctl) | **Yes — direct gRPC, no tunnel, no agent, no in-cluster dependency.** Strongest position of the three. |
| **Workload management** | Service proxy + manifest sync | Explicitly refuses; "complementary to k9s" | **Explicitly refuses.** Same seam: :50000 vs :6443. Boundary confirmed correct. |
| **Interface** | Web | Terminal | **Web** — the unoccupied slot. |
| **License** | BUSL (support contract for production) | MIT | Operator-owned. |

**Verdict on the declared scope boundary:** PROJECT.md's "workload management out of scope" is **correct and independently corroborated** — talos-pilot, the closest OSS analogue, draws the identical line and states it explicitly ("complementary to k9s, not a replacement"). The seam is the port number: **:50000 = Talos machine API = holzkube; :6443 = Kubernetes API = k9s/Lens/Headlamp/Rancher.** Keep it.

---

## Pitfalls Worth Flagging to the Roadmap

These emerged from feature research and should be cross-checked against PITFALLS.md:

### Pitfall: The apply-mode trap
Only a **fixed allowlist** of config fields can be applied without a reboot (`.machine.{time,ca,acceptedCAs,certCANs,install,network,nodeLabels,nodeTaints,sysfs,sysctls,logging,controlplane,kubelet,pods,kernel,registries,features.*}`, `.cluster`, `.debug`). Everything else forces a reboot. A UI that offers `--mode=no-reboot` without checking the patch against this list will hand the user a confusing apply-time failure. **Compute the required mode from the diff and recommend it.** Also note `--mode=staged` does not alter the running config, so a subsequent staged edit will not see prior staged changes — a genuinely surprising behavior worth surfacing in the UI.

### Pitfall: Schematic drift between ISO and installer
`installer` and `initramfs` models honor **system extensions only** — `extraKernelArgs` and `meta` are silently dropped. A node booted from an ISO with kernel args, then installed via an installer image built from the same schematic, ends up **without** those kernel args. Warn at schematic-authoring time.

### Pitfall: Silent config divergence on upgrade (Talos v1.13)
The new streaming upgrade API does **not** pass `.machine.install.extraKernelArgs` to the installer. On GRUB nodes with `grubUseUKICmdline: false` (the default for configs generated before v1.12), the upgrade **succeeds and reports no error**, but the node reboots with a kernel command line that no longer matches its machine config. Mitigations: `--legacy` (supported only until Talos 1.18) or migrate to UKI cmdline. **holzkube should detect this condition and warn**, because Talos itself will not.

### Pitfall: Existing disk install shadows the ISO
A machine with a prior Talos install boots that install even when the ISO is inserted. Re-provisioning silently does nothing. Requires `wipe disk` / `talos.experimental.wipe=system` / boot-order change.

### Pitfall: Maintenance mode is unauthenticated
Self-signed cert, no client cert, no mutual verification. `--cert-fingerprint` is the only defense and it is only readable from a physical/serial/IPMI console. Do not paper over this in the wizard.

### Pitfall: Talos version range must be explicit
The machine API changes between Talos releases, and `machinery` is version-coupled. Omni maintains a formal support matrix and a version support policy. holzkube needs a declared supported range and a visible warning when a node falls outside it (already a PROJECT.md constraint — make sure it becomes a real, tested feature, not a README sentence).

---

## Confidence & Sourcing Note

The `classify-confidence` seam scores by **transport** and returns `LOW` for `webfetch`/`websearch`/`curl` because it cannot attest provenance. That understates these findings, so both dimensions are reported honestly:

| Finding area | Source | Transport tier (seam) | Actual source authority |
|---|---|---|---|
| Omni feature surface | `docs.siderolabs.com/llms.txt` — the vendor's own complete doc index, read verbatim | LOW | **HIGH** — primary vendor docs, full nav enumerated, not summarized |
| talosctl command surface | `docs.siderolabs.com/talos/v1.13/reference/cli.md` (169 KB, parsed) | LOW | **HIGH** — canonical CLI reference |
| Image Factory / maintenance mode / apply modes / upgrades | Official Talos v1.13 docs, read in full | LOW | **HIGH** — primary vendor docs |
| OSS Talos UI landscape | GitHub REST API (`archived`, `pushed_at`, stars, license) cross-checked with two web searches | LOW | **HIGH** — machine-readable repo state, independently corroborated |
| talos-pilot feature detail | Project README | LOW | **MEDIUM** — self-reported by maintainer |
| Bare-metal comparisons (MAAS/Tinkerbell) | Web search; Omni bare-metal provider docs read directly | LOW | **MEDIUM** — Omni portion is primary; MAAS/Tinkerbell portion is not deeply verified |
| Prioritization, MVP cut, anti-feature reasoning | Author's synthesis | — | **Opinion**, grounded in the above. Argue with it. |

**Known gaps:** MAAS and Tinkerbell were not examined in depth — they were judged out of the relevant design space early (both are multi-service PXE+BMC+workflow platforms, and PXE is explicitly out of scope). If netboot is ever promoted to in-scope, revisit them properly. Talos v1.14 is in RC and was not reviewed; the supported-version-range decision should account for it.

## Sources

- Sidero Omni documentation index and feature pages — https://docs.siderolabs.com/llms.txt, https://docs.siderolabs.com/omni/
- Omni configuration model — https://docs.siderolabs.com/omni/omni-cluster-setup/how-configuration-works-in-omni.md
- Omni machine registration (SideroLink) — https://docs.siderolabs.com/omni/infrastructure-and-extensions/machine-registration.md
- Omni bare-metal infrastructure provider — https://docs.siderolabs.com/omni/omni-cluster-setup/setting-up-the-bare-metal-infrastructure-provider.md
- Omni cluster upgrades — https://docs.siderolabs.com/omni/cluster-management/upgrading-clusters.md
- Omni audit log — https://docs.siderolabs.com/omni/cluster-management/using-audit-log.md
- talosctl CLI reference (v1.13) — https://docs.siderolabs.com/talos/v1.13/reference/cli.md
- Talos Image Factory — https://docs.siderolabs.com/talos/v1.13/learn-more/image-factory.md
- Talos config acquisition — https://docs.siderolabs.com/talos/v1.13/configure-your-talos-cluster/system-configuration/acquire.md
- Talos `--insecure` / maintenance mode — https://docs.siderolabs.com/talos/v1.13/configure-your-talos-cluster/system-configuration/insecure.md
- Talos editing machine configuration (apply modes) — https://docs.siderolabs.com/talos/v1.13/configure-your-talos-cluster/system-configuration/editing-machine-configuration.md
- Talos upgrade guide (incl. v1.13 streaming upgrade API + extraKernelArgs warning) — https://docs.siderolabs.com/talos/v1.13/configure-your-talos-cluster/lifecycle-management/upgrading-talos.md
- Talos disaster recovery — https://docs.siderolabs.com/talos/v1.13/build-and-extend-talos/cluster-operations-and-maintenance/disaster-recovery.md
- Talos ISO / bare metal — https://docs.siderolabs.com/talos/v1.13/platform-specific-installations/bare-metal-platforms/iso.md
- awesome-talos community projects — https://github.com/siderolabs/awesome-talos
- talos-pilot — https://github.com/Handfish/talos-pilot
- Theila (archived) — https://github.com/siderolabs/theila
- GitHub REST API repo metadata for theila, talos-pilot, omni, image-factory, talm, talhelper, tuppr, talos-operator (queried 2026-08-27)

---
*Feature research for: bare-metal Talos cluster & node management control plane*
*Researched: 2026-08-27*
