# Pitfalls Research

**Domain:** Out-of-cluster Talos Linux node & cluster lifecycle control plane (Go + gRPC, single operator, bare metal)
**Researched:** 2026-08-27
**Talos version studied:** v1.13 (v1.13.7 current; v1.14.0-rc.2 in the Factory version list)
**Confidence:** HIGH on Talos mechanics (primary vendor docs read verbatim + live Image Factory API experiments), MEDIUM on gRPC/architecture inference. See [Confidence & Sourcing](#confidence--sourcing).

> **Relationship to FEATURES.md.** FEATURES.md flagged six pitfalls. All six are **verified here** (§P9, §P10, §P11, §P12, §P13, and the version-range note), three of them are **corrected or sharpened**, and this document adds twenty more. Where a FEATURES.md claim needed adjustment it is called out explicitly in a `> **Correction to FEATURES.md**` block.

---

## Severity Legend

| Severity | Meaning |
|----------|---------|
| **CLUSTER-DESTROYING** | Cluster is lost or unmanageable; recovery needs a snapshot, a console, or a rebuild |
| **NODE-BRICKING** | Node needs physical access (USB, console, power button) to recover |
| **DATA-LOSS** | Persistent data destroyed; the cluster survives |
| **LOCKOUT** | Everything still runs, but holzkube (or the operator) can no longer manage it |
| **ANNOYING** | Confusing, slow, or embarrassing; recoverable from the UI |

---

## Critical Pitfalls

### P1: The talosconfig expiry lockout — holzkube locks itself out of the cluster after one year

**Severity: LOCKOUT (escalating to CLUSTER-DESTROYING if the secrets bundle was never captured)**

**What goes wrong:**
Talos automatically rotates all *server-side* certificates (etcd, Kubernetes, Talos API). It does **not** rotate *client* certificates. The Talos docs are explicit: "Client certificates (`talosconfig` and `kubeconfig`) are the user's responsibility. The `talosconfig` file should be renewed at least once a year."

holzkube is a daemon that stores one `talosconfig` on disk at `0600` and runs for years. Roughly 12 months after the cluster is created, **every node simultaneously becomes unreachable** with a TLS handshake error. Nothing is actually wrong with the cluster. The operator, staring at a dashboard where all nodes went red at once, will reasonably conclude the cluster died and start doing genuinely dangerous things.

**Why it happens:**
Certificate expiry is invisible until it fires, it fires all at once, and it fires long after anyone remembers building the tool. `talosctl` users hit this too, but they hit it one command at a time and the error is in their terminal. In holzkube it presents as a total, correlated outage.

**Why it can escalate to unrecoverable:**
The renewal paths are: (a) `talosctl config new` using a *still-valid* `os:admin` talosconfig, (b) regenerate from `secrets.yaml`, or (c) extract `.machine.ca.crt` + `.machine.ca.key` from a control-plane node's machine config — which itself requires a working talosconfig. If holzkube holds only an expired talosconfig **and never captured the secrets bundle**, there is no software path back in. Recovery requires the operator to find their original `secrets.yaml` elsewhere, or physical console access.

**How to avoid:**
1. **Persist the secrets bundle, always.** Treat `secrets.yaml` (Talos CA cert *and key*, K8s CA, bootstrap tokens) as a hard precondition for adopting a cluster into the inventory. Refuse to fully onboard a cluster without it, or mark it prominently as `RECOVERY-IMPOSSIBLE`. This turns P1 from unrecoverable into a two-minute fix.
2. **Parse `NotAfter` from the stored client cert at startup and on every dashboard render.** Surface days-remaining as a first-class cluster property next to Talos/K8s version.
3. **Warn at 90 days, warn loudly at 30, block nothing.** A red banner: "holzkube's client certificate expires in 27 days. Renew now." with a one-click renew.
4. **Implement one-click renew** — issue a new client cert from the stored secrets bundle (`gen config --output-types talosconfig` equivalent via `machinery`), write it atomically, keep the old one as `.bak` until the new one is proven with a live `version` call against a node.
5. **Distinguish "cert expired" from "node down" in the error path.** An `x509: certificate has expired` must never render as a red node. It must render as a cluster-level banner. This is a specific, testable error-classification requirement.
6. Consider issuing holzkube itself a **short-TTL cert that it auto-renews** from the bundle (e.g. 30 days, renewed at 15), rather than a 1-year cert it forgets about. This makes the renewal path exercised continuously instead of once a year.

**Warning signs:**
All nodes transition unreachable within the same second. Error strings containing `x509`, `certificate has expired`, `bad certificate`, `tls: failed to verify certificate`.

**Phase to address:** PKI/secrets store phase (P1 in FEATURES.md's MVP). The expiry check and the secrets-bundle requirement must land *with* the store, not later — retrofitting a secrets-bundle requirement after clusters are already onboarded means re-onboarding them.

---

### P2: `talosctl reset` defaults are maximally destructive — and the UI will inherit them

**Severity: DATA-LOSS + NODE-BRICKING**

**What goes wrong:**
Verified against the v1.13 CLI reference, `reset` defaults are:

| Flag | Default | Consequence of the default |
|------|---------|----------------------------|
| `--wipe-mode` | **`all`** | Wipes **the whole disk set, including user data disks** — not just the OS |
| `--reboot` | **`false`** | The machine **powers off**, it does not come back |
| `--graceful` | `true` | Tries to cordon/drain and leave etcd — *fails* on a single-member etcd |
| `--user-disks-to-wipe` | (empty) | — |
| `--system-labels-to-wipe` | (empty) | Ignored unless set; otherwise `--wipe-mode` governs |

Two independent disasters hide in these defaults:

- **`--wipe-mode=all` destroys Ceph/Longhorn/local-path data on the node's *extra* disks.** An operator clicking "Reset node" to reinstall the OS on a mini-PC with a second NVMe full of persistent volumes loses that data. The Talos docs themselves carry a warning about this for cloud VMs ("might result in the VM being unable to boot as this wipes the entire disk").
- **`--reboot=false` means the machine shuts down.** For a headless mini-PC in a cupboard, "reset" therefore means "walk over and press the power button." An operator resetting a remote node has just stranded it.

**Why it happens:**
A UI author maps a "Reset" button onto the API call with zero arguments and inherits the CLI's defaults. The CLI's defaults are chosen for "I am standing at the machine and want it factory-clean." holzkube's user is not standing at the machine.

**How to avoid:**
1. **Never expose an argument-less Reset.** The reset dialog must render every consequential flag as an explicit, defaulted-to-safe control:
   - **Scope:** `Wipe OS only (STATE + EPHEMERAL)` ← *holzkube's default* · `Wipe system disk` · `Wipe everything including data disks` ← requires a second confirmation
   - **After reset:** `Reboot (return to maintenance mode)` ← *holzkube's default* · `Shut down` ← warn: "the machine will not come back on its own"
   - **Graceful:** on, with automatic detection that it *cannot* be graceful (see P3).
2. **Enumerate and display the disks that will be wiped**, by model/serial/size, before the confirm button activates. The Talos API can list disks; use it. "This will wipe: `nvme0n1` (Samsung 990, 1TB, system) and `nvme1n1` (WD SN770, 2TB, **contains data**)."
3. **Type-the-hostname confirmation** for anything beyond `STATE + EPHEMERAL`.
4. **Map holzkube's vocabulary to the underlying flags visibly.** Show the effective `--wipe-mode` / `--system-labels-to-wipe` in the dialog. An operator who knows Talos must be able to verify holzkube is doing what they think.

**Warning signs:**
Any code path constructing a `ResetRequest` without explicitly setting wipe scope and reboot behaviour. A reset dialog with fewer than three controls.

**Phase to address:** Node actions phase (P1). This is the single cheapest catastrophic-loss prevention in the product.

---

### P3: The health gate that looks green but does not protect quorum

**Severity: CLUSTER-DESTROYING**

**What goes wrong:**
The naive health gate is: *"is the previous node `Ready` again? then proceed."* This does not protect etcd, for four separate reasons:

1. **It asks the wrong question.** The correct question is not "is everything healthy now" but **"if I take node X down, will the *remaining* members still hold quorum?"** With `n` voting members, quorum is `floor(n/2)+1`, and taking one down is safe only if `n-1 >= floor(n/2)+1`.

   | Voting members | Quorum | Can lose | Safe to take one down? |
   |---|---|---|---|
   | 1 | 1 | 0 | **No** (cluster stops) |
   | 2 | 2 | 0 | **No** — strictly worse than 1 |
   | 3 | 2 | 1 | Yes |
   | 4 | 3 | 1 | Yes, but no better than 3 |
   | 5 | 3 | 2 | Yes |

   **The even-number trap:** 2 members tolerate *zero* failures while doubling the failure surface. 4 tolerate the same 1 as 3. Even counts are never correct. holzkube must warn at config-generation time when adding a control-plane node would make the count even (FEATURES.md flags this at step 8 of the provisioning flow — it belongs in the upgrade gate too).

2. **It counts members, not healthy members.** `talosctl etcd members` lists members; presence is not health. A member that is present but has fallen behind on raft apply is a vote you do not actually have. `talosctl etcd status` exposes `RAFT INDEX`, `RAFT TERM`, `RAFT APPLIED INDEX` per member — **the gate must compare applied index across members and require convergence**, not merely a non-empty member list.

3. **It counts learners.** `etcd status` has a `LEARNER` column. Learner members **do not vote**. A 3-member list where one is a learner is a 2-voter cluster. Counting it as 3 produces exactly the wrong answer.

4. **It ignores the etcd alarm state.** A `NOSPACE` alarm (default 2 GiB quota) halts etcd writes. Every member is "present," the member count is 3, and the cluster is frozen. `talosctl etcd alarm list` is a required input to the gate.

**The comforting-but-insufficient fact:** Talos already refuses to upgrade a control-plane node if it would break etcd quorum, and serialises concurrent control-plane upgrades. **Do not rely on this.** It protects etcd only. The Talos docs say so directly: "other components of the cluster may not be able to recover from more than one node rebooting at a time (e.g. any software that maintains a quorum or state across nodes, such as Rook/Ceph)." In a homelab running Ceph on three nodes, Talos will happily let you destroy Ceph's quorum while dutifully protecting etcd's.

**How to avoid:**
Define the gate as a **pure function with an explicit, logged input set**, evaluated *before* each node and again *after* the previous node returns:

```
CanTakeDown(node) =
    votingMembers      = etcd.members.filter(!learner).count
    votingMembers - 1  >= floor(votingMembers/2) + 1
  AND all voting members' raftAppliedIndex within N of the leader's
  AND etcd.alarms is empty
  AND every other CP node's etcd service state == Running/Healthy
  AND node is not `locked` in inventory
  AND (if K8s reachable) no PDB would be violated  -- advisory only
```

- **Render the gate's inputs in the UI, not just its verdict.** "Proceeding: 3 voting members (0 learners), applied indices converged (Δ≤12), no alarms." An opaque green check is untrustworthy and undebuggable.
- **Re-evaluate before every node**, including the first. Cluster state changes between nodes.
- **Refuse, don't warn, at `votingMembers <= 2`** for rolling operations. Offer a single-node override that requires typing the cluster name.
- **Surface non-etcd quorum risk explicitly.** A one-time acknowledgement per cluster: "holzkube gates on etcd quorum only. If you run Ceph/Longhorn/MinIO, verify their health between nodes." Honest > silently wrong.

**Warning signs:**
Health-gate code that calls a K8s `/readyz` and nothing else. Any `len(members)` without a learner filter. A gate that runs once at the start of a rolling operation.

**Phase to address:** Cluster overview / etcd phase must land **before** the rolling-upgrade phase. FEATURES.md's dependency graph already says this — enforce it as a hard phase ordering, not a preference.

---

### P4: Bootstrap is a one-shot, non-idempotent, cluster-destroying primitive

**Severity: CLUSTER-DESTROYING**

**What goes wrong:**
`talosctl bootstrap` tells a control-plane node to abort its etcd join loop and **form a brand-new single-member etcd cluster**. The Talos docs carry an explicit `Warning: Run this command ONCE on a SINGLE control plane node.`

Run it a second time, or on a second node, and you get two disjoint etcd clusters that believe they are the same Kubernetes cluster. This is not a partition that heals; it is split-brain with divergent state. Recovery is: pick a winner, wipe the loser's EPHEMERAL partition, let it rejoin.

The failure is *easy to trigger from a UI*: the provisioning flow spans reboots and minutes of silence (FEATURES.md step 12: "the node goes silent here — this is the scariest gap"). An operator watching a spinner that has said nothing for four minutes will click the button again. A retry loop in holzkube's job runner will do it faster and more reliably than any human.

**How to avoid:**
1. **Bootstrap is not a retryable job step.** Whatever the job framework does elsewhere, this operation must be marked non-retryable and guarded by a persisted `bootstrapped_at` timestamp on the cluster record, written **before** the RPC is issued and never cleared by a retry path.
2. **Precondition check, always, immediately before the call:** query etcd membership across all known control-plane nodes. If *any* node reports *any* etcd member, refuse and explain. This catches both the double-click and the wrong-node case.
3. **Idempotency key on the button.** The confirm action carries a nonce; a replay is a no-op with a clear "already bootstrapped" message, not an error and not a second call.
4. **Distinct affordance for the recovery variant.** `bootstrap --recover-from=<snapshot>` (plus `--recover-skip-hash-check` when the snapshot came from `cp /var/lib/etcd/member/snap/db`) is a *different operation* on a *deliberately wiped* cluster. Do not reuse the "Bootstrap cluster" button with an optional file field — the precondition check that protects normal bootstrap must be *inverted* here (recovery expects `etcd` in `Preparing` on all CP nodes). Two buttons, two flows, two guards.
5. **The UI must make waiting feel like progress.** Elapsed time, current sub-step, expected duration, and — critically — an explicitly disabled Bootstrap button with the reason ("bootstrap already issued 3m12s ago; waiting for etcd to report healthy"). A disabled button with a reason prevents the click that a spinner invites.

**Warning signs:**
`bootstrap` reachable from a generic retry/backoff wrapper. No `bootstrapped_at` column in the schema. One button handling both bootstrap and recovery.

**Phase to address:** Provisioning wizard phase. The persisted job state machine that FEATURES.md calls "a first-class architectural requirement" exists substantially *because of this pitfall* — make that connection explicit in the plan.

---

### P5: The secrets bundle is the cluster's identity — regenerate it and the cluster is orphaned

**Severity: CLUSTER-DESTROYING**

**What goes wrong:**
`secrets.yaml` holds the Talos CA (cert **and** private key), the Kubernetes CA, the etcd CA, service-account keys, and bootstrap tokens. Every certificate in the cluster chains to it. It is not a config file; it is the cluster's identity.

Three ways a management tool destroys a cluster with it:

- **Silent regeneration.** A "Generate config for new node" code path that calls `gen secrets` when it cannot find an existing bundle — a reasonable-looking fallback — produces a config signed by a **different CA**. The new node boots, presents a cert nothing trusts, and never joins. Worse, if that config is applied to an *existing* node, that node leaves the cluster and cannot be talked to with the old talosconfig either. Diagnosing this from the symptoms ("certificate signed by unknown authority") is genuinely hard.
- **Partial import.** Adopting the operator's existing homelab cluster means capturing the bundle from a control-plane node's machine config (`.machine.ca.crt` / `.machine.ca.key`, `.cluster.ca`, `.cluster.aggregatorCA`, `.cluster.serviceAccount`, `.cluster.etcd.ca`, `.cluster.secret`, `.cluster.token`, `.machine.token`). Miss one field and config generation succeeds, the node applies it, and the node then fails in some *specific* way (kubelet can't authenticate, or the aggregation layer breaks) that looks nothing like "you missed a CA."
  **Note:** `.machine.ca` on a *worker* node does not contain the private key. Import must read from a **control-plane** node.
- **Rotation without write-back.** `talosctl rotate-ca` completes and the docs then say: "stash the new Talos CA, update `secrets.yaml` (if using that for machine configuration generation) with new CA key and certificate." If holzkube rotates and does not write the new CA into its stored bundle, every subsequently generated config is signed by the *old, now-removed* CA. Nodes provisioned after the rotation cannot join, and the failure is delayed until the next provisioning — long after the causal action.

> **Correction to FEATURES.md:** FEATURES.md treats "import the existing cluster's secrets" as a distinct, easily-forgotten *requirement*. It is more than that — it is the operation with the widest silent blast radius in the product, and it needs a verification step, not just an implementation.

**How to avoid:**
1. **Make `gen secrets` unreachable except from an explicit "Create new cluster" action.** No fallback, no lazy initialisation, no "generate if missing." A missing bundle is a hard error with an actionable message, never a silent new identity.
2. **Fingerprint the bundle.** Store `sha256(TalosCA.crt)` on the cluster record. Before generating *any* node config, assert the loaded bundle's fingerprint matches. Before applying a generated config, assert the target node's current CA (readable via the API) matches — for maintenance-mode nodes there is nothing to compare, which is precisely why P8 matters.
3. **Verify the import, don't trust it.** After importing a bundle, immediately (a) mint a throwaway short-TTL `os:reader` client cert from it and (b) prove it connects to a control-plane node. If that round-trip fails, the import is wrong *now*, not in six months. This single test converts the entire class of partial-import bugs into an immediate, obvious failure.
4. **CA rotation writes back atomically or not at all.** Rotation is: rotate → capture new CA → write new `secrets.yaml` + new `talosconfig` → verify connectivity → commit. Any failure rolls back to the previous files. Given the blast radius and that `rotate-ca --dry-run` defaults to `true`, **do not ship rotation in v1** — ship *expiry visibility* (P1) and defer the rotate action.
5. **Back up the bundle before every mutation** to a timestamped `0600` file. Cheap insurance against exactly the failure mode that has no other recovery.

**Warning signs:**
Any `GenerateSecrets`/`NewBundle` call outside the create-cluster path. A cluster record with no CA fingerprint. An import flow that ends in "Imported ✓" without a connectivity proof.

**Phase to address:** Secrets store + cluster import phase (P1). The verification round-trip is part of the import story, not a follow-up.

---

### P6: Talos upgrades are easy, Kubernetes upgrades are hard — and that asymmetry can strand the cluster permanently

**Severity: CLUSTER-DESTROYING (as a management dead-end)**

**What goes wrong:**
Talos 1.13 supports Kubernetes 1.36 down to 1.31 — six minors. Talos and Kubernetes upgrades are fully decoupled (since Talos v1.0), so nothing forces you to move them together. Talos upgrades are a single API call. Kubernetes upgrades are a multi-phase orchestration.

A UI that makes Talos upgrades one click and defers Kubernetes upgrades to "later" therefore **manufactures a version-skew deadlock**. This is not hypothetical — it is [siderolabs/talos#12398](https://github.com/siderolabs/talos/issues/12398), where an operator upgraded Talos `1.6.8 → 1.7.7 → 1.8.4 → 1.9.6 → 1.11.5` while Kubernetes sat at 1.28.3, and then found `upgrade-k8s` unable to proceed:

```
failed updating service "kube-apiserver": ... rpc error: code = InvalidArgument
  * kubelet image is not valid: version of Kubernetes 1.28.3 is too old to be used with Talos 1.11.5
  * kube-controller-manager image is not valid: ...
  * kube-scheduler image is not valid: ...
```

The trap is subtle and worth stating precisely: `upgrade-k8s` works by *patching each control-plane node's machine config*. Talos validates the **whole resulting config**, which still names the too-old *current* versions in other fields. So the very mechanism that would fix the skew is blocked *by* the skew. The cluster is stuck: Kubernetes cannot go forward (Talos rejects it) and Talos cannot easily go back.

> **Roadmap consequence — this contradicts the naive priority.** FEATURES.md places Talos rolling upgrade at **P2** and Kubernetes upgrade at **P3** ("genuinely HIGH complexity; decoupled since Talos v1.0, so it can wait"). That ordering is defensible on cost, but it is the exact ordering that produces #12398 in the field. If Talos upgrades ship without Kubernetes upgrades, **the compatibility guard below is not optional — it is the thing that makes shipping them in that order safe.**

**How to avoid:**
1. **Model the compatibility matrix as data, not documentation.** A table of `talosMinor → [supported k8s minors]` (1.13 → 1.36…1.31; 1.12 → 1.35…1.30), refreshable, with the running versions checked against it continuously.
2. **Gate the Talos upgrade on forward K8s compatibility.** Before offering Talos vN+1, compute whether the cluster's *current* Kubernetes version remains supported by vN+1. If it does not, **block the upgrade** with: "Upgrading to Talos 1.14 would drop support for Kubernetes 1.31 (this cluster). Upgrade Kubernetes to ≥1.32 first." A warning is not enough; this is a one-way door.
3. **Show "distance from the edge" permanently.** On the cluster overview: "Kubernetes 1.31 — supported by Talos 1.13, **oldest supported**. Two more Talos minors and you are stranded." This makes the slow drift visible while it is still cheap to fix.
4. **Enforce the intermediate-minor path.** Talos config migration is only *tested between adjacent minor releases*; the docs' recommended path is the latest patch of every intermediate minor. Never offer "upgrade to latest" as a single hop across minors. Compute and display the full chain (`1.11.5 → 1.11.latest → 1.12.latest → 1.13.7`) and execute it as a sequence with the health gate between every step.
5. **Ship `upgrade-k8s --dry-run` before shipping the upgrade itself.** It is cheap, it reports deprecated API resources, and it turns the K8s upgrade from a black box into something the operator can evaluate. It also gives holzkube a K8s-upgrade surface in v1.x without owning the orchestration.

**Warning signs:**
An upgrade UI that lists Talos versions without reference to the running Kubernetes version. A "latest" button. Any single-hop upgrade across a minor boundary.

**Phase to address:** The compatibility matrix belongs in the **cluster overview** phase (it is a read-only property). The gate belongs in the Talos upgrade phase and is a **release blocker for it**.

---

### P7: Storing full machine configs silently downgrades Kubernetes

**Severity: CLUSTER-DESTROYING**

**What goes wrong:**
`talosctl upgrade-k8s` **mutates the live machine configuration on every control-plane node**, rewriting the image references for `kube-apiserver`, `kube-controller-manager`, `kube-scheduler`, `kube-proxy` and `kubelet`. The Talos docs are unusually blunt about the consequence:

> "If you are storing full machine configuration files (`controlplane.yaml`, `worker.yaml`) in Git, these versions will drift out of sync. **Re-applying those stale files later could unintentionally downgrade components.** That is why we do not recommend storing full machine configurations."

A management tool that snapshots each node's rendered config "for safety" and offers a "restore this config" button has built a **one-click control-plane downgrade** and labelled it as a recovery feature. Downgrading `kube-apiserver` across a minor is not a supported operation and can corrupt etcd state written by the newer version.

The same trap has a second mouth: **the Talos version contract.** Reproducible generation requires a *fixed* `--talos-version`. The docs warn: "If you leave the Talos version contract unset, or change it to a newer version, `talosctl gen config` may generate a different machine configuration that introduces new fields or defaults that did not exist in your original config. This can silently change cluster behavior." So a node provisioned in June and one provisioned in November, from the same bundle and the same patches, get **structurally different configs** if the contract floats.

**How to avoid:**
1. **The persisted source of truth is (secrets bundle + patches + cluster name + endpoint + k8s version + pinned Talos version contract).** Never a rendered config. This is precisely the vendor-recommended model and it happens to be exactly what FEATURES.md already proposes ("versioned, reusable config patches") — so the architecture is right; the discipline is to *not also* keep full configs "just in case."
2. **Store `talosVersionContract` on the cluster record and pin it.** Changing it is an explicit, warned, audited action — never a side effect of the operator picking a newer Talos version to install.
3. **Rendered configs are ephemeral artifacts.** Generate → validate → diff → apply → discard. If they must be shown or downloaded, watermark them: "snapshot for inspection — do not re-apply; regenerate from patches instead."
4. **If a config-restore feature is ever built, strip the version-bearing fields** (all component images, kubelet image) and re-derive them from the live cluster before applying. Or simply do not build it.
5. **Detect and surface drift** between "what the patches would produce" and "what is running." Drift in component images after a K8s upgrade is *expected and correct*; the UI should say so rather than flagging it red and tempting a "fix" that downgrades.

**Warning signs:**
A `machine_configs` table with a `yaml` column and a `restore` endpoint. Any place a stored config is written back to a node. `gen config` called without an explicit version contract.

**Phase to address:** Config/patch model phase — this is a **schema decision**, and schema decisions are the expensive kind to reverse. Decide before the first migration is written.

---

### P8: Applying a config to the wrong machine — the discovery + maintenance-mode combination

**Severity: NODE-BRICKING / DATA-LOSS**

**What goes wrong:**
Maintenance mode has, by design, **no authentication in either direction**. Verified from the docs: the node serves a *self-signed* cert, the client presents *no* cert, and neither side verifies the other. The only defence is `--cert-fingerprint`, and the fingerprint is printed **to the machine's console at boot** — obtainable only via physical, VM, serial, or IPMI console.

Now combine that with subnet scanning as the discovery mechanism. holzkube scans `192.168.1.0/24:50000`, finds three machines in maintenance mode, and shows them as a list. Two of them are **identical mini-PCs bought at the same time**. The operator picks one and applies a control-plane config.

Concretely, what goes wrong:
- **Wrong machine, right config.** The config installs Talos to disk on a machine that was meant to stay a worker — or was somebody else's machine entirely. Talos installs, wipes the target disk, and joins the cluster. There is no undo.
- **Right machine, but not the machine you looked at.** DHCP moved the lease between the scan and the apply. The row said `192.168.1.57`; by apply time that address belongs to a different host.
- **Not a Talos node at all.** Something else is listening on `:50000`. The apply fails, but holzkube has just sent a config *containing cluster secrets* to an unknown TLS endpoint it did not authenticate.

**How to avoid:**
1. **Identity is UUID, never IP.** Read the machine UUID (available insecurely) at discovery *and again immediately before apply*. If it changed, abort with "the machine at 192.168.1.57 is no longer the machine you selected." This is a five-line check that closes the DHCP race entirely.
2. **Never auto-act on a scan result.** Discovery populates a list. Nothing in that list is targeted without an explicit human selection *and* a confirmation that re-displays the identity (UUID, MACs, disk models/serials, Talos version).
3. **Support and encourage `--cert-fingerprint`, and be honest about it.** Offer an optional fingerprint field; when populated, pin it. Explain in the UI, in one sentence, that the fingerprint comes from the console and that without it this step is unauthenticated. FEATURES.md is right that this must not be papered over — the mitigation is *plain language*, not a hidden setting.
4. **Prove it is Talos before sending anything.** The probe sequence is: TCP connect → TLS → `version` RPC → `get` on a non-sensitive resource. Only a machine that answers the Talos `version` RPC is a candidate. A bare open port is not a discovery.
5. **Never send secrets to an unverified endpoint speculatively.** The config (which contains the cluster CA private material for a control-plane node) goes out exactly once, after confirmation, to a re-verified UUID.
6. **Physical-identification affordance.** Show disk serials and MAC addresses prominently — they are what an operator can actually check against the sticker on the box. Consider offering the operator a place to record a human label per UUID at first sight, so the second identical mini-PC is never ambiguous again.

**Warning signs:**
Apply paths keyed on IP. A discovery result acted on without re-reading the UUID. A "provision all discovered machines" button (do not build this).

**Phase to address:** Discovery phase and provisioning wizard phase, jointly. The re-verify-before-apply check is the load-bearing one.

---

### P9: The Image Factory schematic family — four distinct silent-divergence bugs

**Severity: NODE-BRICKING / ANNOYING (depending on which extension vanishes)**

FEATURES.md flags two of these. Here is the full family, all verified.

**(a) `installer` and `initramfs` honour system extensions only.**
Confirmed verbatim in the v1.13 docs' Restrictions section: "`installer` and `initramfs` images only support system extensions (kernel args and META are ignored)"; "`kernel` assets don't depend on the schematic." So a schematic with `extraKernelArgs` produces an **ISO that has them** and an **installed system that does not**. The machine boots correctly from USB and then installs a subtly different system. → *Warn at schematic-authoring time, the moment a kernel arg or META entry is added: "these apply to the ISO/PXE image only and will be dropped by the installer. Set kernel args via `.machine.install.extraKernelArgs` in the machine config instead."*

**(b) Schematic drift between boot media and installed system.**
The docs state the rule directly: "the `installer` image in the machine configuration should be using the same schematic as the ISO/PXE boot image." Nothing enforces it. → *holzkube must auto-populate `.machine.install.image` from the same schematic ID used for the ISO. FEATURES.md calls this "the highest-value automation in the entire flow" and that is correct.*

**(c) Forgetting the schematic at upgrade time — extensions silently vanish.**
`talosctl upgrade`'s default is `--image ghcr.io/siderolabs/installer:v1.13.0` — **the stock installer**. Upgrade a Factory-imaged node with the default and every system extension (gvisor, `amd-ucode`, `intel-ucode`, iSCSI tools, NVIDIA drivers…) is gone after reboot. The upgrade reports success. → *holzkube must construct the installer reference from the schematic ID stored on the node record and must **refuse to upgrade a node whose schematic ID is unknown**, offering instead to read it from the node first (see (e)).*

**(d) Version/architecture/repo-name mismatches.**
Verified live against `factory.talos.dev`:
- `GET /versions` returns **108 entries including pre-releases**, ending `v1.14.0-alpha.1, -alpha.2, -beta.0, -beta.1, -rc.1, -rc.2`. **A naive "latest = last element" offers an RC.** Filter pre-releases; make opting into them explicit.
- `GET /version/<v>/extensions/official` is **version-scoped** (51 extensions at v1.9.0, more at v1.13.7) and returns Talos-version-pinned refs (`ghcr.io/siderolabs/amazon-ena:2.16.1-v1.13.7`). An extension valid at one version may not exist at another — so **a saved schematic can become un-buildable at upgrade time.**
- The installer OCI repo name is **version-dependent**: `factory.talos.dev/v2/installer/<id>/manifests/v1.13.7` → `200` and `.../metal-installer/<id>/manifests/v1.13.7` → `200`, but `metal-installer` at `v1.9.0` → connection failure. Do not hardcode either name across the supported range.

**(e) The schematic ID is recoverable — use it.**
Image Factory appends a *virtual* system extension whose version is the schematic ID, readable via `talosctl get extensions`. This means holzkube can **recover the schematic ID from any running node** rather than depending on the operator remembering it. This matters enormously for the import-existing-cluster path, where nodes were provisioned before holzkube existed. Conversely: Factory deliberately provides **no way to list schematics** (they may contain secrets in kernel args), so an ID that is neither stored nor on a running node is genuinely gone.

**BONUS, verified by direct experiment:** POSTing a schematic that references a **non-existent extension succeeds**:
```
POST /schematics  {customization.systemExtensions.officialExtensions: [siderolabs/totally-not-real-ext]}
  → HTTP 200  {"id":"068236bd62b862fce9936f0300ef192d201331174606f921e8fc381f3e4a0000"}
GET /image/068236.../v1.13.7/metal-amd64.iso
  → HTTP 400
```
**Schematic creation is not validation.** A UI that says "Schematic created ✓" after the POST is lying. → *Validate extension names against `/version/<v>/extensions/official` **before** POSTing, and after POSTing immediately probe the target model (a `HEAD` on the ISO URL / a registry manifest `GET` for the installer) to prove buildability for the intended Talos version. Only then mark the schematic usable.*

**Phase to address:** Image Factory phase. The `schematic_id` column on the **node** record (not the cluster) is the link between provisioning and upgrades — FEATURES.md correctly identifies this as a schema decision that must land early.

---

### P10: The `extraKernelArgs` silent divergence on upgrade (Talos v1.13)

**Severity: NODE-BRICKING (if the args were load-bearing) / ANNOYING otherwise**

**Verified verbatim in the v1.13 upgrade docs** — FEATURES.md's flag is accurate:

> The new upgrade API does not pass `.machine.install.extraKernelArgs` to the installer, so a GRUB node with `.machine.install.grubUseUKICmdline` set to `false` gets a `grub.cfg` written without those arguments. **The upgrade succeeds and reports no error**, but the node reboots with a kernel command line that no longer matches its machine configuration.
> `grubUseUKICmdline` defaults to `false` for machine configurations generated before Talos v1.12, so **existing GRUB clusters are affected without opting in.**

This is the archetype of the whole class: *the API returns success, the node comes back healthy, and the running system no longer matches its declared configuration.* If the dropped args were `intel_iommu=on` for PCI passthrough, or a console redirect, or a storage driver parameter, the node comes back "healthy" and broken in a way nothing reports.

**A related, separate trap** surfaced in [#12210](https://github.com/siderolabs/talos/issues/12210): on systemd-boot nodes, applying a config with kernel args yields `WARNING: extra kernel arguments are not supported when booting using SDBoot` — a *warning on stderr* that a programmatic client will discard entirely unless it deliberately captures and surfaces non-fatal messages.

**How to avoid:**
1. **Detect the vulnerable combination and refuse to upgrade silently.** Before any Talos upgrade, evaluate: `bootloader == GRUB` AND `grubUseUKICmdline != true` AND `.machine.install.extraKernelArgs` is non-empty. If all three hold, block the one-click path and present the two documented options: `--legacy` (supported only until Talos 1.18 — say so, with the deadline) or migrate to UKI cmdline. **Talos will not warn. holzkube must.**
2. **Capture and display non-fatal API messages.** Design the client wrapper so warnings are a first-class part of every mutating response, surfaced in the UI and written to the audit log — never discarded because the RPC returned OK.
3. **Post-upgrade verification, generally.** After every upgrade, compare *declared* vs *observed*: kernel cmdline, installed extension list, Talos version, schematic ID. Report divergence explicitly. This one mechanism catches P9(a), P9(c), and P10 — it is the highest-leverage guardrail in the upgrade phase, and it exists because **"the API said OK" is not evidence in this domain.**
4. **Track the 1.18 deadline in code.** If holzkube emits `--legacy`, it must warn when the cluster's Talos version approaches 1.18.

**Phase to address:** Talos upgrade phase, as a precondition check. The declared-vs-observed verification is the phase's acceptance criterion.

---

### P11: Apply-mode traps — including two the docs bury

**Severity: ANNOYING → NODE-BRICKING**

FEATURES.md flags the allowlist. Here is the verified v1.13 list, which differs from FEATURES.md in one material way, plus three traps FEATURES.md does not cover.

**The exact allowlist** (fields appliable without reboot in v1.13):
`.debug`, `.cluster`, `.machine.time`, `.machine.ca`, `.machine.acceptedCAs`, `.machine.certCANs`, `.machine.install`, `.machine.network`, `.machine.nodeAnnotations`, `.machine.nodeLabels`, `.machine.nodeTaints`, `.machine.sysfs`, `.machine.sysctls`, `.machine.logging`, `.machine.controlplane`, `.machine.kubelet`, `.machine.pods`, `.machine.kernel`, `.machine.registries`, and **only these five** features subkeys: `features.kubernetesTalosAPIAccess`, `features.hostDNS`, `features.imageCache`, `features.kubePrism`, `features.nodeAddressSortAlgorithm`.

> **Correction to FEATURES.md:** FEATURES.md writes `features.*`, implying the whole subtree. It is **five named subkeys only**. Everything else under `.machine.features` (`rbac`, `stableHostname`, `apidCheckExtKeyUsage`, `diskQuotaSupport`, …) requires a reboot. FEATURES.md also omits `.machine.nodeAnnotations`. A `no-reboot` recommendation engine built from FEATURES.md's list would confidently recommend `no-reboot` for changes that fail at apply time.

**Trap A — `.machine.install` is on the allowlist but is a no-op until the next install/upgrade.** The docs annotate it: "(configuration is only applied during install/upgrade)". So changing `.machine.install.image` or `.machine.install.extraKernelArgs` applies "immediately," returns success, changes the stored config, and **changes nothing about the running system**. The operator sees ✓ and believes the node now has the new kernel args. It does not, and will not until it is upgraded (and see P10 for what happens then). → *Special-case this field: on a diff touching `.machine.install`, render "stored now, takes effect at next install/upgrade" instead of a generic success.*

**Trap B — `staged` does not stack.** Verbatim from the docs: "applying change on next reboot (`--mode=staged`) doesn't modify current node configuration, so next call to `talosctl edit machineconfig --mode=staged` will not see changes." Two staged edits in a row: **the second silently discards the first**, because the second is computed against the *running* config. In a UI where staging changes feels like queueing them, this loses work invisibly. → *Either forbid a second staged apply while one is pending (show "a staged change is pending; reboot to apply, or discard it"), or compute subsequent staged patches against the pending staged config and re-send the union. Forbidding is simpler and honest.*

**Trap C — `try` is a safety feature that reads like a debug flag.** `--mode=try` applies immediately and **auto-reverts after 1 minute** unless another config update lands. This is the correct default for risky network changes: if the change kills connectivity, the node heals itself. → *Make `try` the recommended mode for any diff touching `.machine.network`, with a live 60-second countdown and a "Keep this change" button that re-applies in `auto`. This turns Talos' best safety primitive into the product's most reassuring interaction, and it is genuinely differentiating.*

**What happens when the config is invalid** — the precise answer, since it drives the whole guardrail design:
- **Malformed or schema-invalid** → rejected at the API with `InvalidArgument` before anything is written. Real examples: `recovered: expected a mapping node` ([#12210](https://github.com/siderolabs/talos/issues/12210)), `kubelet image is not valid: version of Kubernetes 1.28.3 is too old…` ([#12398](https://github.com/siderolabs/talos/issues/12398)). **The node is unaffected.** Talos does not brick on invalid config.
- **Valid but wrong** (bad control-plane endpoint, wrong subnet, wrong install disk) → accepted, applied, and the node fails to boot or fails to join. The docs have a whole "Recover from node boot failures" section for this. **This is the dangerous case**, and no amount of schema validation catches it.
→ *Therefore: validation is table stakes but not the safety mechanism. The safety mechanisms are the **structural diff** (does this change what I think?), **`--mode=try`** for network changes, and **dry-run**. Do not let "we validate the config" stand in for them.*

**Recommend the mode; do not just offer it.** Compute the required mode from the diff: intersect changed paths with the allowlist → if fully contained, offer `no-reboot`; otherwise state *which specific field* forces the reboot. "Reboot required: `.machine.features.stableHostname` is not in the immediately-appliable set" is a genuinely better experience than `talosctl`'s.

**Phase to address:** Config patch / apply phase. The allowlist belongs in code as a tested constant with a version tag, not in a comment.

---

### P12: Patch merge semantics — lists append, so re-applying a patch corrupts it

**Severity: ANNOYING → NODE-BRICKING**

**What goes wrong:**
Talos strategic merge patches have a rule that surprises everyone: **"If the field value is a list, the patch value is appended to the list."** Only five paths override this (`cluster.network.podSubnets` and `serviceSubnets` overwrite; `network.interfaces` merges on `interface`/`deviceSelector`; `vlans` merges on `vlanId`; `apiServer.auditPolicy` replaces; `ExtensionServiceConfig.configFiles` merges on `mountPath`).

For a product whose differentiator is a **versioned, reusable patch library**, this is the central hazard. Apply the same "add nameserver 192.168.1.1" patch twice and the node has two identical nameservers. Apply an `extraKernelArgs` patch twice and the kernel command line has duplicates. Apply a `nodeTaints` patch across a re-provision and taints accumulate. Reusable patches are **not idempotent by default** — which is precisely the property the feature's name implies they have.

Two more patch-engine facts that will bite:
- **From Talos v1.12+ the machine config is multi-document, and RFC 6902 JSON patches no longer work.** Confirmed by [#12210](https://github.com/siderolabs/talos/issues/12210) and [#12562](https://github.com/siderolabs/talos/issues/12562), and by the K8s upgrade doc's note: "From Talos v1.13 the machine configuration is a multi-document configuration, for which strategic-merge patches should be used rather than RFC 6902 (`op`/`path`/`value`) patches." → **Build the patch engine on strategic merge only.** A JSON-patch implementation is wasted work that also silently limits the supported version range.
- **Removal requires `$patch: delete`**, and **the main `v1alpha1` document cannot be deleted**. A "remove this setting" UI affordance must emit `$patch: delete`, not an empty value. Also: a single patch file cannot modify the same document twice.

**How to avoid:**
1. **The diff must be computed by actually merging.** Render current config → apply patch through the real `machinery` merge → render result → structurally diff the two rendered configs. Never diff patch text, and never *predict* the merge result. This is the only thing that makes append-semantics visible before apply, and it converts the entire class into a visible diff. (FEATURES.md says "diff the rendered merged config, structurally" — this pitfall is *why*, and it is worth stating in the plan so the requirement survives scoping pressure.)
2. **Detect and flag list growth in the diff UI.** If a merge grows a list, call it out: "`extraKernelArgs`: 3 entries → 5 entries (2 appended)." An operator seeing that immediately knows whether it is intended.
3. **Detect duplicates after merge** and warn — duplicate list entries are almost always a re-applied patch.
4. **Test re-application explicitly.** For every patch in the library, a test that applies it twice and asserts the second application is a no-op, or is clearly reported as not-a-no-op. Make idempotency a *property of each stored patch*, computed and displayed, rather than an assumption.

**Phase to address:** Config patch phase. Rule 1 is architectural — a text-diff implementation would have to be thrown away.

---

### P13: Prior install shadows the ISO — and `wipe disk` will not fix it

**Severity: ANNOYING (but it eats an afternoon)**

**What goes wrong:**
A machine with an existing Talos install on disk boots **that install**, not the inserted ISO, depending on boot order. Re-provisioning appears to do nothing: the operator boots the ISO, scans, and the machine either never appears in maintenance mode or appears as a fully configured node belonging to the old cluster.

> **Correction to FEATURES.md:** FEATURES.md lists the remedies as "`wipe disk` / `talos.experimental.wipe=system` / boot-order change." The first is wrong in the common case. Verified from the CLI reference: `talosctl wipe disk` wipes "a block device (disk or partition) **which is not used as a volume**." It therefore **cannot wipe the system disk of a running Talos node.** The actual remedies are:
> - `talosctl reset` (from the running node, with appropriate wipe scope) — the normal path;
> - the `talos.experimental.wipe=system` kernel parameter, entered at the GRUB console — the path when the node is stuck in a boot loop;
> - changing boot order / removing the disk — the physical path.
>
> `wipe disk` is the right tool for *secondary/data* disks (e.g. clearing a Ceph OSD disk before reuse), and it does work in maintenance mode via `--insecure`. Note `--method` defaults to `FAST` (metadata only); `ZEROES` is the one that actually destroys data — relevant when decommissioning hardware.

**How to avoid:**
- **Detect the situation and name it.** If a scan finds a node at the expected address that is *configured* (responds to mTLS, or refuses `--insecure`) rather than in maintenance mode, say exactly that: "192.168.1.57 is a configured Talos node belonging to cluster `homelab`, not a blank machine. To re-provision, reset it first." Do not report "no machines found."
- **Put it in the provisioning checklist** at the "boot the machine" step, since holzkube cannot automate USB boot anyway: "Machine has a previous Talos install? Reset it first, or it will boot that install instead of the ISO."
- **Offer "Reset and re-provision"** as a single guarded flow for machines already in inventory — this is the common real case (repurposing a node) and chaining it removes the trap entirely.

**Phase to address:** Discovery phase (detection + messaging), provisioning wizard (checklist).

---

### P14: Long-lived gRPC streams — one bad keepalive kills every stream on the connection

**Severity: ANNOYING (but presents as "holzkube is unreliable", which is fatal for trust)**

**What goes wrong:**
holzkube will hold many long-lived server-streaming RPCs per node: logs, `dmesg`, events, COSI resource watches, upgrade progress. Several distinct failure modes stack:

1. **Keepalive misconfiguration is self-inflicted and catastrophic in blast radius.** Talos' gRPC server factory sets no keepalive `EnforcementPolicy`, so grpc-go server defaults apply: `MinTime` 5 minutes, `PermitWithoutStream` false. A client configured with a "responsive" 10–30 s keepalive violates this; the server responds with `GOAWAY / ENHANCE_YOUR_CALM`. Because HTTP/2 multiplexes, that tears down **the entire `ClientConn` and every stream on it** — all logs, all watches, for that node, at once. Then the client reconnects, pings aggressively again, and the cycle repeats. This looks exactly like "the node is flapping." *(MEDIUM confidence on the specific defaults — inferred from absence of an enforcement policy in `pkg/grpc/factory` plus documented grpc-go defaults. Verify empirically in the sandbox; it is a five-minute test.)*
2. **The opposite failure: no keepalive at all.** A stream that is silent for a long time (a node that logs nothing) is dropped by an intermediate NAT/firewall/switch idle timeout with no notification. The stream simply stops producing, forever, with no error. The UI shows an empty log pane that looks calm and is dead.
3. **Reboot mid-stream is the *normal* case, not the exception.** Every upgrade, every `reboot`, every `apply --mode=reboot` kills every stream to that node. The stream may error, or may just hang until a timeout. Either way, holzkube must treat "node rebooting" as an expected state transition with automatic, backed-off reconnect — and must not present the gap as an error.
4. **Backpressure.** A node dumping `dmesg` after a boot storm produces data faster than a browser WebSocket consumer accepts it. Without a bounded buffer with a drop policy, memory grows until holzkube is OOM-killed — taking down the tool that was supposed to help.
5. **Goroutine and connection leaks.** A browser tab closes; the WebSocket dies; the gRPC stream and its reader goroutine live on because nothing cancelled the context. At homelab scale this leaks slowly enough to look like "it needs a restart every few weeks" rather than a bug.

**How to avoid:**
1. **Start with keepalive OFF** (rely on TCP), add it only if idle-drop is observed, and if added set `Time` **well above 5 minutes** with `PermitWithoutStream: false`. Make the parameters configurable so a wrong default is fixable without a rebuild. Explicitly test for `ENHANCE_YOUR_CALM` / `GOAWAY` in the sandbox.
2. **One `context.Context` per stream, derived from the consumer's lifetime, always cancelled in `defer`.** Non-negotiable. Add a goroutine-count metric on an internal debug endpoint and a sandbox test that opens and closes 100 streams and asserts the count returns to baseline. This is the cheapest possible detection for the whole leak class.
3. **Bounded ring buffer per stream with an explicit drop policy** — drop oldest, and **tell the UI**: "… 1,432 lines dropped …" inline. Silent dropping destroys trust in a log viewer; visible dropping preserves it.
4. **Model connection state explicitly** in the UI: `live` · `reconnecting (attempt 3)` · `node rebooting` · `disconnected`. Never leave a pane that silently stopped updating looking identical to one that is live. This is the same honesty requirement as P15 and should share a mechanism.
5. **Separate the `ClientConn` used for streams from the one used for control operations** where practical, so a stream-induced `GOAWAY` cannot interrupt an in-flight upgrade or apply.
6. **Reconnect with jittered exponential backoff and a cap.** During a rolling upgrade, N nodes reboot in sequence; unjittered reconnects synchronise into a thundering herd against the control-plane endpoint.

**Phase to address:** The transport/client foundation phase sets the connection and context discipline (items 1, 2, 5, 6). The streaming phase adds buffering and UI state (3, 4). Items 2 and 6 must be in the foundation — retrofitting context discipline across an existing streaming codebase is miserable.

---

### P15: When the cluster is down, the UI must not lie — and gRPC will help it lie

**Severity: ANNOYING → CLUSTER-DESTROYING (via a wrong decision made on stale data)**

**What goes wrong:**
holzkube exists to be useful when the cluster is broken (PROJECT.md's core architectural bet). That value is destroyed if, during an outage, the UI shows the last-known-good state as though it were live. An operator who believes etcd has 3 healthy members, when in fact those numbers are 40 minutes old and two nodes are down, will confidently take an action that finishes the cluster off — a reset, a "just reboot it," a member removal.

The mechanism that produces the lie is mundane: **a dead node does not refuse connections, it goes silent.** TCP SYNs to a powered-off machine are dropped, so `connect()` hangs for the OS timeout. A gRPC call with no deadline hangs indefinitely. Concretely:
- **Calls that hang rather than fail:** anything to a powered-off or network-partitioned node; anything to the Kubernetes API server when `kube-apiserver` is down but the node is up (TLS handshake completes, request hangs); `etcd snapshot` without quorum; `upgrade` with `--drain` when the K8s API is unavailable (`--drain` defaults to **true** with a 5-minute `--drain-timeout`).
- **Calls that fail cleanly:** connection-refused when the node is up but `apid` is not; `InvalidArgument` on bad configs; TLS errors on cert problems.

The failure pattern: a page fans out to N nodes, one hangs, the whole page hangs or times out at some outer limit, and the operator sees a spinner — or worse, cached data with no indication of its age.

**How to avoid:**
1. **Every RPC has a deadline. No exceptions, enforced structurally** — a client wrapper that will not issue a call without one, so it cannot be forgotten. Short (2–5 s) for health polls, long and explicit for upgrades.
2. **Per-node fan-out is independent and partial results render immediately.** One unreachable node must never delay or blank the other nine. This is a data-fetching architecture decision, not a UI polish item.
3. **Every displayed datum carries its observation time, and staleness is visually loud.** "Last seen 41 minutes ago" in amber; hard grey-out past a threshold. The operator must never have to wonder whether a number is live.
4. **Encode FEATURES.md's health tiers (NONE / NODE / ETCD / K8S) in the data model and degrade per tier.** When etcd has no quorum, the cluster overview shows per-node NODE-level truth plus an explicit "etcd quorum lost — cluster-level data unavailable" banner. It does **not** blank, and it does **not** show yesterday's member list as current.
5. **Classify errors and say which layer failed.** "Node unreachable (TCP timeout)" ≠ "apid refused" ≠ "certificate expired" (P1!) ≠ "Kubernetes API unavailable, Talos API fine". Each implies a completely different next action; collapsing them into a red dot throws away the tool's most valuable output during an outage.
6. **Disable actions whose preconditions cannot currently be verified**, with the reason shown. If etcd health is unknown, the rolling-upgrade button is disabled and says "cannot verify etcd quorum." Unknown must never be treated as OK — this is the same principle as P3, applied at the UI layer.
7. **Set `--drain=false` deliberately, or handle the timeout**, when upgrading a node whose K8s API is unreachable. Otherwise every upgrade attempt costs 5 minutes of hanging before it even starts.

**Phase to address:** Foundation (deadlines, error taxonomy, per-node fan-out) and every read surface thereafter. The error taxonomy is a **shared type** — define it once, early.

---

### P16: Security — the HTTP port is root on every machine

**Severity: CLUSTER-DESTROYING**

**What goes wrong:**
An attacker who reaches holzkube's HTTP port and gets past (or around) authentication can: read the cluster CA **private key**, mint themselves an `os:admin` talosconfig valid for a year, wipe every node's disks, remove etcd members, and apply arbitrary machine configs. holzkube is a **higher-value target than any single node**, and it runs outside the cluster with none of the cluster's protections. Talos itself has no SSH and a hardened API surface; holzkube is the soft path around all of it.

Domain-specific mistakes, beyond generic web security:

| Mistake | Why it is specific here | Prevention |
|---|---|---|
| **Secrets in API responses** | "View MachineConfig" returns `.machine.ca.key`, `.cluster.secret`, `.cluster.token`, `.machine.token`, `cluster.serviceAccount.key`. The obvious implementation returns the node's config verbatim. | **Redact server-side, on a strict allowlist**, before serialisation. Never redact in the frontend — the data has already left the building. Reuse the exact same redactor for the raw-YAML view, the diff view, the API response, and the audit log. One function, four call sites. |
| **Secrets in logs and errors** | gRPC error messages and debug logging can echo the request body — which for `ApplyConfiguration` is the whole config including CA keys. Then holzkube's own log file becomes the leak. | A logging wrapper that never logs request bodies for `ApplyConfiguration`/`GenerateConfiguration`. Log a content hash and a field-path list instead. Grep the log for known secret prefixes in CI as a regression test. |
| **Secrets in the diff view** | A diff of two configs is a *side-by-side rendering of both*, so a naive diff leaks secrets even if the single-config view redacts. And a redacted diff can show `<redacted> → <redacted>` as "changed", which is useless. | Diff on redacted-but-hashed values so a genuine change is visible (`ca.key: <secret, unchanged>` vs `ca.key: <secret, CHANGED>`) without exposing content. |
| **Session cookie without `SameSite`/CSRF protection** | A cross-site POST to `/api/nodes/x/reset` wipes hardware. This is CSRF with a physical-world outcome. | `SameSite=Strict`, `HttpOnly`, `Secure`; plus an explicit CSRF token on every mutating request. Do not rely on `SameSite` alone. |
| **Confirmation implemented client-side** | A confirm dialog in React is not a control; the endpoint is still one `curl` away. | The **server** requires a confirmation token in the destructive request body — e.g. the node hostname, validated server-side against the target's actual hostname. Guardrails that live only in the UI are decoration. |
| **Audit log written after the fact** | The interesting entries are exactly the ones that crashed, hung, or were interrupted. Log-on-success loses them. | Write **intent before the call** and **outcome after**, as two linked records. An intent with no outcome is itself a finding, and it is what you will want during the post-mortem. |
| **Audit log missing the target's identity** | "Reset node 192.168.1.57" is worthless six months later after DHCP churn. | Record UUID + hostname + IP + cluster + actor + full parameters (wipe mode, reboot flag, schematic ID) + result. Parameters matter: "reset" without wipe-mode does not tell you whether data was destroyed. |
| **Secrets bundle world-readable via a backup path** | The `0600` file is correct, and then a "download config" endpoint or a Docker volume mounted `755` undoes it. | Enforce `0600` **on read** (fail loudly if the mode is wrong), never serve raw secrets over HTTP, and document the Compose volume permissions. |
| **Binding to `0.0.0.0` by default** | A homelab tool bound to all interfaces on a flat network is reachable by every IoT device on the LAN. | Default bind `127.0.0.1`. Exposing it is an explicit, documented choice. |
| **Over-privileged standing credentials** | holzkube holds a 1-year `os:admin` cert continuously, including for the read-only dashboard. | Talos RBAC supports scoped roles and TTLs (`talosctl config new --roles os:reader --crt-ttl`). Consider a long-lived `os:reader` cert for read paths and a short-TTL `os:admin` cert minted from the bundle only for mutations. Reduces the blast radius of a stolen on-disk cert substantially. |

**Warning signs:**
Any handler returning a machine config without passing through the redactor. `log.Printf("%+v", req)` anywhere near the Talos client. A destructive endpoint whose handler does not validate a confirmation token.

**Phase to address:** Auth/audit phase for sessions and audit; **but redaction must land with the "View MachineConfig" feature itself** (P1 in FEATURES.md's MVP), because that feature *is* the leak. Do not sequence redaction after the view.

---

### P17: Developing against mocks and meeting real hardware only at the end

**Severity: CLUSTER-DESTROYING (the first real target is the operator's actual homelab cluster)**

**What goes wrong:**
PROJECT.md is explicit: `talosctl` is not installed, `~/.talos/config` does not exist, and the cluster was unreachable from the dev machine at project start. So the first version of everything gets written against a hand-rolled fake — and a hand-rolled fake encodes **the author's beliefs about Talos**, which is exactly what has not been validated yet. Every misunderstanding in this document becomes a fake that confirms it.

The specific danger is that **the first real target is production.** There is one cluster, it is the operator's homelab, and it is the thing the tool is for. A tool that was only ever run against a fake, pointed at a real cluster, with reset and apply-config wired up, is how a homelab dies.

**What a fake cannot teach you — the list that must be exercised against real Talos:**

| Behaviour | Why a fake gets it wrong |
|---|---|
| **Maintenance-mode API envelope** | Which resources are readable insecurely is defined by the `sensitivity` field on resource *definitions* — discoverable only from a real node (`talosctl get rd --insecure`, `select(.spec.sensitivity == null)`). A fake will happily serve everything, and the wizard will be built on resources that do not exist in maintenance mode. |
| **The apply-modes allowlist** | Enforced by the node. A fake accepts anything, so `no-reboot` recommendation logic is untested until it fails in production. |
| **Patch merge semantics (P12)** | List-append vs replace, `$patch: delete`, multi-document matching. Nobody reimplements these correctly from prose. *(Mitigation: use the real `machinery` merge code in-process — this one is testable without a node.)* |
| **Reboot/upgrade timing and stream death** | How long a node is silent, what a stream does when the node goes away, whether the error is `Unavailable` or a hang. Pure timing behaviour, invisible to a fake. |
| **gRPC keepalive/`ENHANCE_YOUR_CALM` (P14)** | A fake server has no enforcement policy. The bug appears only against real `apid`. |
| **Cert/TLS behaviour** | Maintenance-mode self-signed certs, fingerprint pinning, expiry handling, endpoint-vs-node cert errors. |
| **Real etcd state shapes** | Learner members, raft index divergence, alarms, defrag behaviour. A fake returns three tidy healthy members forever — which is precisely the input under which a broken health gate (P3) passes. |
| **Bootstrap timing and the silent window** | The scariest part of the provisioning flow is a fake's easiest case. |
| **Multi-document config serialisation** | v1.12+ changed the shape and there have been real serialisation regressions ([#12462](https://github.com/siderolabs/talos/issues/12462)). |

**How to avoid:**
1. **`talosctl cluster create --provisioner docker` in Phase 1, before any feature.** PROJECT.md and FEATURES.md both already demand this; the point here is that it is a **correctness dependency, not a testing convenience**, and the schedule must treat slipping it as a blocking risk rather than a deferral.
2. **Prefer QEMU over Docker for the flows that matter.** The Docker provisioner does not have real disks, real boot, real maintenance mode, or a real install step — so it cannot exercise the Core Value at all. Docker is fine for API-shape work; **the provisioning flow needs QEMU**, and that needs to be known early, not discovered in month three.
3. **Build fakes by recording real traffic, never by hand.** Capture real responses from the sandbox into golden fixtures. A hand-written fake tests your beliefs; a recorded one tests reality. Re-record on every supported Talos version — this doubles as the version-range compatibility test PROJECT.md's constraints require.
4. **Make "verified against real Talos" an explicit per-feature attribute** in the plan, and hold a feature "not done" until it flips. Prevents a wall of green tests standing in for evidence.
5. **`--dry-run` mode for the whole binary**, logging every mutating RPC without sending it. This is what makes the first pointing-at-the-homelab moment survivable, and it costs almost nothing if built into the client wrapper from day one (rather than bolted on later).
6. **Read-only mode / per-cluster mutation lock.** Adopt the real cluster in read-only first. Enabling mutations is an explicit, per-cluster, audited toggle. FEATURES.md already proposes a per-node "lock" — a per-*cluster* mutation lock is the same cheap idea one level up, and it is what protects the homelab during development.
7. **Get a third mini-PC, or a Proxmox/QEMU VM, before the provisioning phase.** The Core Value is "blank machine → cluster node." It cannot be verified without a machine that is allowed to be blank. Budget this as a **phase entry requirement**, not a stretch goal.

**Phase to address:** Phase 1 (sandbox). Items 5 and 6 belong in the foundation because they are what makes every later phase safe to test against reality.

---

### P18: Building the easy half and never reaching the Core Value

**Severity: The project fails**

**What goes wrong:**
FEATURES.md establishes that `talos-pilot` (MIT, actively maintained, 243 stars) already delivers node/service monitoring, log streaming, etcd quorum and alarm diagnostics, PKI expiry, drain/reboot ops, a bootstrap wizard, config generation and `apply-config` — everything except upgrades, Image Factory, persistent multi-cluster inventory, versioned patches with diff, and a web UI.

The read-only surface is also, by a wide margin, the **most rewarding thing to build**. `talosctl get` maps almost directly to a table; a dashboard exists after a weekend and looks impressive. Provisioning, by contrast, spans reboots, requires a persisted state machine, needs real hardware, and its scariest moment (P4, step 12) is a four-minute silence.

So the gradient of the work points away from the Core Value, continuously. The predictable outcome: six months in, a beautiful dashboard, no provisioning, and a tool that is strictly worse than a terminal for the things it does — plus a maintenance burden.

**A sharper way to see it:** every feature in the read-only half is one that talos-pilot already ships. Every hour spent there is an hour spent re-implementing a solved problem, in a harder medium, for an audience of one who already has the terminal.

**How to avoid:**
1. **Make the Core Value a *phase*, not a milestone.** "Blank machine → cluster node" must be a thing that gets done, in full, in a numbered phase — not the eventual sum of many enabling phases. Enabling phases justify themselves by unblocking it.
2. **Cut the read-only surface in v1 to what provisioning needs plus one trust-building screen.** Node detail with health/version/hardware/services is cheap and builds confidence in the gRPC layer. Log streaming is explicitly **v1.x in FEATURES.md** — it is the single most seductive item on the list (xterm.js! live output!) and it is not on the Core Value path. Hold that line; it is the specific line that will be argued about.
3. **Order phases so provisioning is unblocked early.** Image Factory + discovery + config generation + persisted job state are all P1 in FEATURES.md's MVP for exactly this reason. Anything that does not unblock them or make them safe is a candidate for v1.x.
4. **Define v1 acceptance as the FEATURES.md sentence, verbatim, and treat it as binary:** *a blank mini-PC becomes a healthy cluster node without the operator opening a terminal.* Not "provisioning works," not "the wizard is done." One machine, one browser, no terminal.
5. **Force the moment of truth early with a walking skeleton.** Before polishing anything, get a crude end-to-end path working against a QEMU VM: hardcoded schematic, manual IP, generated config, apply, join. Ugly is fine. This surfaces P4, P8, P9 and the step-12 silence while there is still time to design for them — and it converts the scariest unknown into a known problem in week three rather than month six.
6. **Track "days since the Core Value path last ran end-to-end"** as a visible project metric. It is the single number that predicts this failure.

**Phase to address:** Roadmap structure itself. This pitfall is prevented by phase *ordering*, not by any feature.

---

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Key inventory on IP instead of UUID | No discovery/identity plumbing | Corrupts on the first DHCP lease change; breaks the wrong-machine guard (P8); painful migration | **Never** — UUID from day one |
| Store rendered machine configs as the source of truth | Trivial "view config"/"restore" | Silent K8s downgrade (P7); schema rewrite later | **Never** — vendor explicitly advises against it |
| Single-cluster schema | Simpler joins | PROJECT.md already commits to multi-cluster; retrofit touches every table | **Never** — already decided |
| Hand-written Talos API fakes | Unblocks dev without hardware | Encodes your misconceptions as passing tests (P17) | Only as a stopgap **behind** a recorded-fixture plan with a date |
| No `schematic_id` on the node record | One less column | Upgrades silently strip extensions (P9c); unsafe to ship upgrades at all | **Never** — column is free, and it is recoverable from nodes via `get extensions` |
| Client-side-only confirmation dialogs | Fast to build | Destructive endpoints are one `curl` away (P16) | **Never** for destructive actions |
| RPCs without deadlines | Less boilerplate | Hangs during exactly the outage the tool exists for (P15) | **Never** — enforce structurally in the client wrapper |
| Audit log written only on success | Simpler | Loses the crashed/hung operations you will actually need (P16) | **Never** — intent+outcome is barely more code |
| Skip `--dry-run`/read-only mode | Ship features faster | First real target is the operator's only cluster (P17) | **Never** — cheap in the wrapper, priceless once |
| Talos version range "documented in the README" | No code | Silent misbehaviour on out-of-range nodes; PROJECT.md requires it as a constraint | Acceptable in v0; must become an enforced, tested check before v1 |
| Text-diff instead of structural merge-diff | Much simpler | Cannot reveal list-append (P12); noisy; makes the flagship safety feature untrustworthy | **Never** for the apply path; fine for a read-only "raw YAML" tab |
| Defer CA rotation | Less code | None meaningful | **Recommended** — ship expiry *visibility* (P1), defer the rotate action (P5) |
| Reuse one `ClientConn` for streams and control ops | Fewer connections | A stream `GOAWAY` interrupts an in-flight upgrade (P14) | Acceptable early; separate before the upgrade phase |

---

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| **Image Factory — schematic POST** | Treating HTTP 200 as validation | Verified: a bogus extension yields 200 + an ID; the 400 comes at model build. Validate names against `/version/<v>/extensions/official` first, then probe the target model before marking it usable (P9) |
| **Image Factory — version list** | `versions[len-1]` as "latest" | The list ends in `v1.14.0-rc.2`. Filter pre-releases; make opting in explicit |
| **Image Factory — installer ref** | Hardcoding `factory.talos.dev/installer/…` | Repo name is version-dependent (`metal-installer` exists at 1.13.7, not at 1.9.0). Build per version and verify the manifest resolves before issuing an upgrade |
| **Image Factory — extension catalog** | One global extension list | Version-scoped, with version-pinned refs. A schematic valid at one Talos version may not build at another |
| **Image Factory — offline** | Assuming reachability | Air-gapped/offline homelabs exist; Factory can be self-hosted. Make the base URL configurable and fail with a clear, non-fatal message |
| **Talos apid `:50000`** | Assuming direct dial to every node is the only pattern | Canonical pattern is `-e <CP endpoint> -n <target>`, where apid on a CP node **proxies**. Direct dial works on flat L2 but the endpoint-vs-node distinction produces confusing certificate errors. Model both; default to CP-endpoint-with-node-header for cluster members, direct dial for maintenance mode |
| **Talos trustd `:50001`** | Scanning/using it | It is for worker→CP trust, not clients. Scan `:50000` only |
| **Kubernetes API `:6443`** | Letting a K8s call block a node view | Strict seam: `:50000` is holzkube, `:6443` is k9s/Headlamp. K8s calls are optional enrichment with their own short deadlines, never on the critical path (P15) |
| **etcd member removal** | Presenting raw hex member IDs | `remove-member` takes a member ID and there is no confirmation. Always map ID → node hostname/UUID; docs say prefer `etcd leave` and use `remove-member` only for a broken/unreachable member |
| **`machinery` version coupling** | Pinning one version and assuming it spans the range | Config schema changed materially at v1.12 (multi-document); client/server skew produces warnings that a programmatic client discards. Declare a supported range, test each version in the sandbox, surface out-of-range nodes |
| **`talosctl` stderr warnings** | Discarding non-fatal messages | Real warnings carry real meaning (`extra kernel arguments are not supported when booting using SDBoot`; server/client version skew). Capture and surface them (P10) |

---

## Performance Traps

Scale here is ~5–20 nodes and one operator. Most classic scaling concerns are irrelevant; these are not.

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| **Unbounded log/dmesg buffering** | RSS climbs during a boot storm; OOM kill | Bounded ring buffer + visible drop counter (P14) | One node, one noisy boot — **not a scale threshold at all** |
| **Goroutine/stream leak per closed tab** | Slow RSS growth; "needs a restart weekly" | Context per stream, cancelled on consumer close; goroutine-count assertion test | ~dozens of tab opens |
| **Serial subnet scan with a long timeout** | `/24` scan takes minutes; wizard feels broken | Bounded worker pool (~64), short connect timeout (~500 ms–1 s), stream results as found, show progress | Any `/24` — i.e. immediately |
| **Aggressive scanning trips network gear** | Consumer APs/switches rate-limit or drop; IDS alerts; flaky Wi-Fi for the household | Cap concurrency, cap scan rate, cap range size, require explicit CIDR entry | `/16` on consumer hardware |
| **Sequential per-node polling** | Dashboard latency = sum of all nodes; one dead node stalls the page | Parallel fan-out, per-node deadlines, partial render (P15) | 5+ nodes with one unreachable |
| **Polling every node every second** | Constant load on `apid`; noisy logs | COSI watches where available; otherwise 5–10 s polls with jitter | Continuous |
| **Unjittered reconnect storms during rolling upgrades** | Herd hits the CP endpoint each time a node returns | Jittered exponential backoff with a cap (P14) | 3+ nodes |
| **Rendering full configs in the browser for diffing** | Sluggish UI; secrets shipped to the client | Diff server-side, send the structural result only (also P16) | Any realistic config |

---

## Security Mistakes

Consolidated in **P16**. Ranked by blast radius:

| Mistake | Risk | Prevention |
|---------|------|------------|
| Serving unredacted machine config over HTTP | Full cluster takeover — CA private key is in there | Server-side allowlist redaction, one shared redactor, applied before serialisation, used by *every* config surface |
| No server-side confirmation on destructive endpoints | One CSRF/`curl` wipes hardware | Server validates a confirmation token (target hostname) in the request body |
| Session cookie without CSRF defence | Cross-site POST resets a node | `SameSite=Strict` + `HttpOnly` + `Secure` **and** an explicit CSRF token |
| Logging gRPC request bodies | Secrets land in the log file, then in a support bundle | Never log `ApplyConfiguration`/`GenerateConfiguration` bodies; log hash + field paths; CI grep for secret prefixes |
| Bind `0.0.0.0` by default | Reachable by every device on a flat LAN | Default `127.0.0.1`; exposure is explicit and documented |
| Permanent `os:admin` credential for all operations | Stolen on-disk cert = a year of full control | Long-lived `os:reader` for reads; short-TTL `os:admin` minted from the bundle for mutations |
| Secrets bundle reachable via download/backup endpoints | Complete cluster compromise | Never serve raw secrets over HTTP; enforce `0600` on read; document Compose volume perms |
| Audit log without target identity and parameters | Cannot reconstruct an incident | UUID + hostname + IP + cluster + actor + full parameters + result; intent written before the call |
| Applying config to an unauthenticated maintenance-mode node | Cluster secrets sent to an attacker-controlled host | Re-verify UUID immediately before apply; offer `--cert-fingerprint` pinning; be honest in the UI (P8) |
| Weak/absent rate limiting on login | Offline-grade brute force against argon2id | Rate limit + lockout; argon2id parameters tuned for ≥250 ms |

---

## UX Pitfalls

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| A spinner during the post-apply install window | Operator assumes failure and re-runs bootstrap (P4) → split brain | Named state, elapsed time, expected duration, current sub-step, **disabled** action buttons with reasons |
| Generic "Are you sure?" on reset | Habituation; the one dangerous click looks like all the others | Type-the-hostname, plus an explicit list of the disks that will be wiped (P2) |
| Showing stale data as live during an outage | Wrong decisions at the worst moment (P15) | Observation timestamps everywhere; loud staleness; explicit "unknown" ≠ "healthy" |
| Green check after a schematic POST | False confidence; failure surfaces later at ISO download or upgrade | Validate + probe the model, then mark usable (P9) |
| Offering all six apply modes with no guidance | Operator picks `no-reboot`, gets an apply-time error they cannot interpret | Compute the mode from the diff; name the specific field forcing a reboot (P11) |
| Red "NotReady" on a freshly joined node | Operator thinks provisioning failed and starts over | "Talos healthy; Kubernetes NotReady — normal until a CNI is installed" (FEATURES.md step 14) |
| Raw etcd member IDs in the removal UI | Wrong member removed → quorum loss | Always show hostname/UUID; ID secondary |
| Warnings discarded because the RPC returned OK | Silent divergence goes unnoticed (P10) | Surface non-fatal warnings in the UI and audit log |
| "Upgrade to latest" as one button | Skips intermediate minors; may offer an RC; may strand K8s (P6, P9d) | Show the computed upgrade chain; filter pre-releases; gate on K8s compatibility |
| Log pane that silently stops updating | Operator debugs a node using a dead stream | Explicit stream state: live / reconnecting / node rebooting / disconnected (P14) |
| Hiding that maintenance mode is unauthenticated | Operator assumes a security property that does not exist | State it in one plain sentence at the point of use; offer fingerprint pinning (P8) |

---

## "Looks Done But Isn't" Checklist

- [ ] **View MachineConfig:** often missing **server-side** redaction — verify `.machine.ca.key`, `.cluster.secret`, `.cluster.token`, `.machine.token`, `.cluster.serviceAccount.key` are absent from the **HTTP response body**, not just the rendered page. Check the raw tab, the diff view, and the audit log too.
- [ ] **Reset:** often missing wipe-scope and reboot controls — verify the default is `STATE+EPHEMERAL` + reboot, that data disks are listed before confirm, and that the effective flags are displayed.
- [ ] **Bootstrap:** often missing the pre-flight membership check — verify a second invocation is refused, and that the recovery variant is a separate flow.
- [ ] **Health gate:** often missing learner filtering, raft-index convergence, and alarm checks — verify it **refuses** at 2 voting members and that its inputs are visible in the UI, not just its verdict.
- [ ] **Talos upgrade:** often missing the schematic-derived installer ref — verify a Factory-imaged node still has its extensions **after** an upgrade (`get extensions`), and that a node with an unknown schematic ID is refused.
- [ ] **Talos upgrade:** often missing the `extraKernelArgs`/GRUB precondition — verify the vulnerable combination is detected and blocks the one-click path (P10).
- [ ] **Config apply:** often missing the reboot-requirement computation — verify a `.machine.features.stableHostname` change is correctly reported as reboot-requiring, and that a `.machine.install` change is labelled "takes effect at next upgrade."
- [ ] **Patch library:** often missing idempotency — verify applying the same patch twice does not duplicate list entries, and that the diff shows list growth explicitly.
- [ ] **Secrets import:** often missing verification — verify the import ends with a live connectivity proof using a cert minted from the imported bundle, not just "file parsed."
- [ ] **PKI:** often missing client-cert expiry — verify a talosconfig expiring in <90 days produces a cluster-level banner, and that an expired cert renders as "credential expired," not "all nodes down."
- [ ] **Discovery:** often missing re-verification — verify that changing a machine's identity between scan and apply aborts the apply.
- [ ] **Streaming:** often missing cleanup — verify goroutine count returns to baseline after 100 open/close cycles, and that a node reboot mid-stream shows "node rebooting," not an error.
- [ ] **Every read surface:** often missing deadlines and partial rendering — verify that one powered-off node does not blank or hang the dashboard.
- [ ] **Audit log:** often missing intent records and parameters — verify an interrupted reset leaves an intent entry with the wipe mode recorded.
- [ ] **Version range:** often missing enforcement — verify an out-of-range node is visibly flagged, not silently mishandled.
- [ ] **Core Value:** often missing the whole point — verify a blank machine becomes a healthy node **without opening a terminal**.

---

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| P1 talosconfig expired, bundle held | **LOW** | Regenerate talosconfig from `secrets.yaml`; replace on disk; reconnect |
| P1 talosconfig expired, **no** bundle | **HIGH** | No software path. Recover `secrets.yaml` from the operator's own backups, or extract `.machine.ca` from a CP node via physical/serial console |
| P2 reset wiped data disks | **HIGH / terminal** | Restore from backup. If none, the data is gone (`--method=FAST` leaves blocks but no supported recovery) |
| P2 reset powered the node off | **LOW but physical** | Press the power button. Unavoidable — which is why the default must be reboot |
| P3 quorum lost, snapshot exists | **MEDIUM** | Verify quorum truly unrecoverable → wipe EPHEMERAL on CP nodes (`reset --graceful=false --reboot --system-labels-to-wipe=EPHEMERAL`) → confirm etcd is `Preparing` on all → `bootstrap --recover-from=snapshot` on one node → others rejoin |
| P3 quorum lost, no snapshot, one member intact | **HIGH** | Snapshot the survivor first (`cp /var/lib/etcd/member/snap/db`), then force it to a single-member cluster with `--recover-skip-hash-check` |
| P3 quorum lost, no snapshot, nothing intact | **TERMINAL** | Rebuild the cluster. Workload state is gone |
| P4 bootstrapped twice → split brain | **HIGH** | Pick the authoritative member; wipe EPHEMERAL on all others; let them rejoin. Any writes to the losing side are lost |
| P5 wrong secrets bundle used for a node | **MEDIUM** | The node never joins; it is isolated but harmless. Re-apply a correctly generated config (insecure if it fell back to maintenance mode), or reset and re-provision |
| P5 secrets bundle lost, cluster running | **MEDIUM** | Re-extract from a CP node's machine config while a valid talosconfig still exists — **do this immediately**; the window closes when the cert expires (P1) |
| P6 Talos ahead, K8s stranded | **HIGH** | No clean path. Options: downgrade Talos (unsupported), or rebuild the control plane from an etcd snapshot at a compatible version. **Prevention is the only real answer** |
| P7 stale config re-applied → K8s downgraded | **HIGH** | Re-run `upgrade-k8s` to the correct version immediately; verify etcd health. Cross-minor apiserver downgrade may have corrupted state — snapshot before touching anything |
| P9 extensions vanished after upgrade | **LOW** | Re-upgrade to the same Talos version using the **correct Factory installer ref**. Non-destructive |
| P10 kernel args dropped | **LOW** | Re-upgrade with `--legacy`, or migrate to UKI cmdline. Non-destructive but requires another reboot |
| P8 config applied to the wrong machine | **HIGH** | The machine is wiped and installed. Reset it, restore from backup if it held data, and **rotate the cluster CA** if the config went to an untrusted host — the private key left the building |
| P12 duplicated list entries | **LOW** | Author a corrective patch using `$patch: delete`, or regenerate the config from patches and re-apply |
| P13 prior install shadows ISO | **LOW but physical** | Reset the node, or `talos.experimental.wipe=system` at the GRUB console, or change boot order |
| P14 stream leak | **LOW** | Restart holzkube. Fix the context discipline |
| P16 credentials leaked via logs/API | **HIGH** | `rotate-ca` for Talos and Kubernetes; reissue every talosconfig/kubeconfig; audit for unexpected API activity |

---

## Pitfall-to-Phase Mapping

Phase names follow FEATURES.md's dependency graph; adjust to the actual ROADMAP.

| Pitfall | Severity | Prevention Phase | Verification |
|---------|----------|------------------|--------------|
| P17 Mock-only development | CLUSTER-DESTROYING | **Sandbox (first)** | A recorded-fixture test suite replays against a live QEMU sandbox; `--dry-run` and read-only mode both exist |
| P14 gRPC streams/keepalive | ANNOYING | **Transport foundation** | 100 open/close cycles return goroutines to baseline; no `ENHANCE_YOUR_CALM` over a 1-hour idle soak |
| P15 Dishonest UI when cluster down | CLUSTER-DESTROYING | **Transport foundation** + every read surface | Kill one node: dashboard renders in <5 s, that node reads "unreachable", others live; actions with unverifiable preconditions are disabled |
| P1 talosconfig expiry lockout | LOCKOUT | **PKI/secrets store** | Inject a cert expiring in 10 days → banner appears; inject an expired cert → renders as "credential expired", not "nodes down" |
| P5 Secrets bundle identity | CLUSTER-DESTROYING | **Secrets store + cluster import** | `gen secrets` unreachable outside create-cluster; import ends with a live connectivity proof; CA fingerprint stored and asserted |
| P7 Full-config storage → K8s downgrade | CLUSTER-DESTROYING | **Config/patch schema** | No `yaml` blob is ever written back to a node; Talos version contract pinned per cluster |
| P16 Secret leakage / CSRF / audit | CLUSTER-DESTROYING | **View-config feature (redaction)**, Auth/audit (rest) | Automated test asserts no secret material in any HTTP response or log line; destructive endpoints reject requests lacking a server-validated token |
| P2 Reset defaults | DATA-LOSS | **Node actions** | Reset dialog lists affected disks and defaults to `STATE+EPHEMERAL` + reboot; sandbox test asserts the constructed request |
| P9 Image Factory schematic family | NODE-BRICKING | **Image Factory** | Bogus extension is rejected pre-POST; `schematic_id` persisted per node; pre-releases filtered; installer ref resolves before use |
| P8 Wrong-machine apply | NODE-BRICKING | **Discovery + provisioning wizard** | UUID re-verified immediately before apply; changing identity aborts; fingerprint field present |
| P4 Bootstrap double-run | CLUSTER-DESTROYING | **Provisioning wizard** | Second bootstrap is refused; recovery is a separate flow; `bootstrapped_at` persisted before the RPC |
| P13 Prior install shadows ISO | ANNOYING | **Discovery** | A configured node at the target IP is reported as such, not as "nothing found" |
| P11 Apply-mode traps | ANNOYING→NODE-BRICKING | **Config patch/apply** | Allowlist is a tested constant; `.machine.install` labelled deferred; second staged apply refused; `try` offered for network diffs |
| P12 Patch list-append | ANNOYING→NODE-BRICKING | **Config patch/apply** | Every stored patch has an idempotency test; diff reports list growth |
| P3 Health gate | CLUSTER-DESTROYING | **Cluster overview/etcd** (must precede upgrades) | Gate refuses at 2 voting members; learners excluded; alarms block; inputs rendered |
| P6 Talos/K8s skew stranding | CLUSTER-DESTROYING | **Cluster overview** (matrix) + **Talos upgrade** (gate) | Talos upgrade that would strand the running K8s version is blocked; upgrade chain shows intermediate minors |
| P10 extraKernelArgs divergence | NODE-BRICKING | **Talos upgrade** | Vulnerable GRUB combination detected and blocked; post-upgrade declared-vs-observed check reports divergence |
| P18 Scope collapse | Project failure | **Roadmap ordering** | v1 acceptance is binary: blank machine → healthy node, no terminal. Walking skeleton runs end-to-end before any polish |

---

## Confidence & Sourcing

The `classify-confidence` seam scores by **transport** and returns `LOW` for `webfetch`/`websearch`/`curl` because it cannot attest provenance. That understates these findings, so both dimensions are reported, following FEATURES.md's precedent.

| Finding area | Method | Transport tier (seam) | Actual authority |
|---|---|---|---|
| Reset flags & semantics (P2, P13) | v1.13 reset guide + `reference/cli.md` (169 KB) parsed directly | LOW | **HIGH** — canonical CLI reference, flags and defaults quoted |
| Apply modes & allowlist (P11) | v1.13 "Edit Machine Configuration", full page | LOW | **HIGH** — allowlist transcribed verbatim; corrects FEATURES.md |
| Patch merge semantics (P12) | v1.13 "Configuration Patches", full page | LOW | **HIGH** — merge rules and exceptions quoted |
| etcd quorum, recovery, maintenance (P3, P4) | v1.13 disaster-recovery + etcd-maintenance + `bootstrap` CLI | LOW | **HIGH** — procedures and warnings quoted |
| Upgrade semantics, streaming API, `extraKernelArgs` (P6, P10) | v1.13 upgrade guide, incl. the `Warning` block verbatim | LOW | **HIGH** — vendor's own documented known limitation |
| K8s upgrade, config drift, SSA pruning (P7) | Kubernetes-guides upgrading-kubernetes + reproducible-machine-configuration | LOW | **HIGH** — vendor explicitly advises against storing full configs |
| Support matrix / version skew (P6) | v1.13 support-matrix page | LOW | **HIGH** — primary |
| PKI lifetimes, client-cert renewal, `rotate-ca` (P1, P5) | v1.13 cert-management + ca-rotation + `rotate-ca` CLI | LOW | **HIGH** — "renewed at least once a year" quoted |
| Maintenance mode & insecure API set (P8) | v1.13 "The insecure flag", full page | LOW | **HIGH** — supported-command list quoted |
| Image Factory restrictions & schematic recovery (P9a/b/e) | v1.13 image-factory page, Restrictions section | LOW | **HIGH** — quoted |
| **Image Factory live behaviour (P9d + the bogus-extension finding)** | **Direct API calls to `factory.talos.dev`** — schematic POST idempotency, `/versions`, `/version/<v>/extensions/official`, OCI manifest probes, bogus-extension 200-then-400 | LOW | **HIGH — first-hand experiment**, reproducible; the strongest evidence in this document |
| Real-world failure modes (P6, P10, P17) | `siderolabs/talos` issues #12398, #12210, #12562, #12462, #13109, #13681 read via the GitHub API | LOW | **MEDIUM-HIGH** — user reports, but with verbatim error output |
| gRPC keepalive specifics (P14) | grpc-go/grpc docs + issues; Talos `pkg/grpc/factory` inspected for an enforcement policy (**none found**) | LOW | **MEDIUM** — the 5-minute `MinTime` is inferred from grpc-go defaults given the absent policy. **Verify in the sandbox.** |
| Severity ratings, phase mapping, prevention designs | Author's synthesis over the above | — | **Opinion**, grounded. Argue with it. |

**Known gaps:**
- **No live Talos node was available.** `talosctl` is not installed and the cluster was unreachable (PROJECT.md). Everything about node *behaviour* — timing, error strings, stream death, maintenance-mode resource sensitivity — is documented, not observed. This is P17 applying to this very document, and it is why P17 recommends recorded fixtures over hand-written beliefs.
- **The `machinery` Go API surface was not examined in depth.** Method names, streaming shapes and option structs need checking against the pinned version during Phase 1.
- **Talos v1.14 is at rc.2 and 1.13 reaches end of community support at the 1.14.0 release (~2026-08-30, i.e. days from now).** The supported-version-range decision should be made against 1.13 **and** 1.14, not 1.13 alone.
- **`--wipe-mode` interaction with `--system-labels-to-wipe` is not documented precisely.** Both flags exist; their precedence when both are set is unstated. Test in the sandbox before wiring the reset dialog.
- **etcd `LEARNER` members in Talos:** the column exists in `etcd status`, but whether Talos ever creates learners on its own was not confirmed. The health gate should filter them regardless — it is free.

## Sources

- Talos v1.13 — Resetting a Machine — https://docs.siderolabs.com/talos/v1.13/configure-your-talos-cluster/lifecycle-management/resetting-a-machine
- Talos v1.13 — Edit Machine Configuration (apply modes, allowlist) — https://docs.siderolabs.com/talos/v1.13/configure-your-talos-cluster/system-configuration/editing-machine-configuration
- Talos v1.13 — Configuration Patches (merge semantics) — https://docs.siderolabs.com/talos/v1.13/configure-your-talos-cluster/system-configuration/patching
- Talos v1.13 — Reproducible Machine Configuration — https://docs.siderolabs.com/talos/v1.13/configure-your-talos-cluster/system-configuration/reproducible-machine-configuration
- Talos v1.13 — Disaster Recovery — https://docs.siderolabs.com/talos/v1.13/build-and-extend-talos/cluster-operations-and-maintenance/disaster-recovery
- Talos v1.13 — etcd Maintenance — https://docs.siderolabs.com/talos/v1.13/build-and-extend-talos/cluster-operations-and-maintenance/etcd-maintenance
- Talos v1.13 — Upgrading Talos Linux (incl. `extraKernelArgs` warning) — https://docs.siderolabs.com/talos/v1.13/configure-your-talos-cluster/lifecycle-management/upgrading-talos
- Talos — Upgrading Kubernetes (config drift, SSA, pruning) — https://docs.siderolabs.com/kubernetes-guides/advanced-guides/upgrading-kubernetes
- Talos v1.13 — Support Matrix — https://docs.siderolabs.com/talos/v1.13/getting-started/support-matrix
- Talos v1.13 — CA Rotation — https://docs.siderolabs.com/talos/v1.13/security/ca-rotation
- Talos v1.13 — Managing PKI and certificate lifetimes — https://docs.siderolabs.com/talos/v1.13/security/cert-management
- Talos v1.13 — The insecure flag (maintenance mode) — https://docs.siderolabs.com/talos/v1.13/configure-your-talos-cluster/system-configuration/insecure
- Talos v1.13 — Image Factory — https://docs.siderolabs.com/talos/v1.13/learn-more/image-factory
- Talos v1.13 — Network Connectivity (ports 50000/50001) — https://docs.siderolabs.com/talos/v1.13/learn-more/talos-network-connectivity
- Talos v1.13 — Production Notes (secrets bundle, bootstrap-once warning) — https://docs.siderolabs.com/talos/v1.13/getting-started/prodnotes
- Talos v1.13 — CLI Reference (`reset`, `bootstrap`, `upgrade`, `rotate-ca`, `wipe disk`, `etcd remove-member`) — https://docs.siderolabs.com/talos/v1.13/reference/cli
- Image Factory live API — `POST /schematics`, `GET /versions`, `GET /version/<v>/extensions/official`, `GET /v2/{installer,metal-installer}/<id>/manifests/<v>`, `GET /image/<id>/<v>/metal-amd64.iso` (queried 2026-08-27)
- siderolabs/talos#12398 — Talos/K8s skew blocks `upgrade-k8s` — https://github.com/siderolabs/talos/issues/12398
- siderolabs/talos#12210 — 1.12 machineconfig patch issues; SDBoot kernel-arg warning — https://github.com/siderolabs/talos/issues/12210
- siderolabs/talos#12562 — json patches vs multi-document config — https://github.com/siderolabs/talos/issues/12562
- siderolabs/talos#12462 — MachineConfig serialization mismatch — https://github.com/siderolabs/talos/issues/12462
- siderolabs/talos#13109 — kubelet timeout during `upgrade-k8s` — https://github.com/siderolabs/talos/issues/13109
- siderolabs/talos#13681 — upgrade fails with Factory installer ref — https://github.com/siderolabs/talos/issues/13681
- siderolabs/talos#6627 — Recovering control plane from loss of quorum — https://github.com/siderolabs/talos/discussions/6627
- gRPC Keepalive guide — https://grpc.io/docs/guides/keepalive/
- grpc/grpc keepalive doc (`ENHANCE_YOUR_CALM`, `MinTime`) — https://github.com/grpc/grpc/blob/master/doc/keepalive.md
- grpc-go#5564 — shared connection keepalive → too many pings — https://github.com/grpc/grpc-go/issues/5564
- grpc-go#1341 — goroutine/connection leak — https://github.com/grpc/grpc-go/issues/1341
- Omni — Gate Talos Upgrades with Healthchecks (prior art for the gate) — https://docs.siderolabs.com/omni/cluster-management/gate-talos-upgrades-with-healthchecks
- Omni — Wipe a Machine — https://docs.siderolabs.com/omni/cluster-management/wipe-a-machine

---
*Pitfalls research for: out-of-cluster Talos Linux node & cluster management control plane (holzkube)*
*Researched: 2026-08-27*
