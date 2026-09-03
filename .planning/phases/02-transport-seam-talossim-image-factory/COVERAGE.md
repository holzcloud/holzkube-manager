# API Coverage — Talos machine API (gRPC) + Image Factory (HTTP)

> Full coverage by default. Opt-outs are explicit, reasoned decisions.

**Detector note (honest record).** `api-coverage.cjs` returned `detected: false`
when run over this phase's scope. That is a **vocabulary miss, not a finding**:
the ROADMAP section and `02-CONTEXT.md` are written in German, and the detector's
trigger vocabulary is English (`integrate`, `wire`, `api`, `grpc`, `endpoint`…).
The same detector fires on the English PLAN.md bodies this phase produces. The
phase demonstrably integrates two external surfaces — the Talos machine API over
gRPC/mTLS (TRANS-01) and the Image Factory HTTP API (FACT-01…FACT-06) — so the
matrix is authored deliberately rather than skipped on a false negative.

---

## Surface 1 — Image Factory (`factory.talos.dev`, hand-rolled HTTP per D-01)

| capability | decision | reason |
|---|---|---|
| `POST /schematics` (schematic create) | INTEGRATE | |
| `GET /versions` (Talos version list) | INTEGRATE | |
| `GET /version/<v>/extensions/official` (version-scoped extension catalog) | INTEGRATE | |
| local schematic-ID precomputation (`Schematic.ID()`) | INTEGRATE | |
| ISO URL derivation `/image/<id>/<v>/metal-<arch>.iso` | INTEGRATE | |
| PXE URL derivation `/pxe/<id>/<v>/metal-<arch>` | INTEGRATE | |
| disk-image URL derivation `/image/<id>/<v>/metal-<arch>.raw.zst` | INTEGRATE | |
| resolved kernel cmdline `/image/<id>/<v>/cmdline-metal-<arch>` | INTEGRATE | |
| secure-boot asset variants (`-secureboot` suffix) | INTEGRATE | |
| OCI installer repo resolution `/v2/<repo>/<id>/manifests/<v>` | INTEGRATE | D-02 requires the version-dependent repo name be resolved, not assumed |
| model-build probe (buildability confirmation after POST) | INTEGRATE | FACT-02 — a POST returning 200 is not validation |
| `GET /schematics/<id>` (schematic read-back) | OPT-OUT | not needed — the Factory's canonical schematic document is returned by the POST and persisted verbatim (D-09); a read-back adds a network round trip for data already held |
| `BrokenVersions` upstream lookup | OPT-OUT | explicitly overruled by D-08 — the broken list is curated and embedded in the binary, reviewable in git |
| `GET /version/<v>/overlays/official` (overlay catalog) | OPT-OUT | not needed — overlays are single-board-computer specific and no SBC target is in the supported scope (metal amd64/arm64 only) |
| `GET /talosctl/<v>` (talosctl download URLs) | OPT-OUT | explicitly out of scope — holzkube's product claim is that the operator never needs `talosctl`; shipping its download links contradicts the Core Value |
| direct `kernel-<arch>` / `initramfs-<arch>.xz` asset URLs | OPT-OUT | not needed — the PXE endpoint already serves the boot script that references both; a second, hand-assembled path would be a drift source |
| `installer-<arch>.tar` / `metal-installer-<arch>.tar` tarballs | OPT-OUT | not needed — the upgrade path consumes the OCI reference, not the tarball; the tarball is a manual-recovery artifact |

## Surface 2 — Talos machine API via `pkg/machinery` (gRPC/mTLS)

Capabilities are grouped by RPC class rather than by all 54 `MachineServiceServer`
methods individually: the deadline/retry policy (`02-CONTEXT.md` `<deadline_policy>`)
is authored per class, and a per-method matrix would restate that table without
adding a decision. `talossim` implements the **full** 54-method surface either way
(TRANS-06) — the decisions below are about the *production client wrapper*.

| capability | decision | reason |
|---|---|---|
| `Version` (liveness probe, D-05) | INTEGRATE | |
| `ApplyConfiguration` (maintenance + cluster) | INTEGRATE | |
| `Bootstrap` | INTEGRATE | |
| `Disks` (maintenance-mode surface) | INTEGRATE | |
| `Hostname` | INTEGRATE | |
| COSI `Get` / `List` (read surface via `Client.COSI`) | INTEGRATE | |
| COSI `Watch` | OPT-OUT | not needed yet — the push-based health model is Phase 3; the seam and the in-memory COSI state built here already carry it |
| fast-read RPC family (13 node-telemetry reads, listed below) | OPT-OUT | not needed yet — deadline- and retry-classified here and implemented in `talossim`; typed client wrappers belong to Phase 3 (Inventar/Health), which owns the consumers |
| etcd read family (`EtcdMemberList`, `EtcdStatus`, `EtcdAlarmList`) | OPT-OUT | not needed yet — deadline-classified and simulated (`etcd_down` scenario); typed wrappers belong to Phase 9 (etcd-Verwaltung) |
| mutation family (16 state-changing RPCs, listed below) | OPT-OUT | not needed yet — each is deadline-classified, excluded from the retry allowlist and dry-run gated here; typed wrappers belong to Phase 6 and Phase 9, which own the jobs engine |
| streaming family (10 server-streaming RPCs, listed below) | OPT-OUT | not needed yet — implemented in `talossim` and deadline-classified with first-byte and idle bounds; consumption is Phase 5 (Streaming) |
| `LifecycleService` | OPT-OUT | explicitly out of scope — brand new in this Talos window and expected to churn (`.planning/research/STACK.md`); adopting it now would pin holzkube to an unstable surface |
| `ClusterService` health checks | OPT-OUT | not needed yet — the health model is Phase 3 |
| `TimeService` | OPT-OUT | not needed — no holzkube feature reads node time |
| `InspectService` | OPT-OUT | not needed — controller-dependency introspection has no consumer in the product |
| `SecurityService` (certificate issuance) | OPT-OUT | not needed yet — PKI issuance belongs to the cluster-import and provisioning phases (3 and 8) |
| `StorageService` beyond `Disks` | OPT-OUT | not needed yet — disk selection for provisioning is Phase 8 |

### Members of the three grouped rows

**fast-read RPC family (13):** `ServiceList`, `Memory`, `LoadAvg`, `SystemStat`, `CPUInfo`,
`CPUFreqStats`, `DiskStats`, `NetworkDeviceStats`, `Mounts`, `Netstat`, `Processes`, `Stats`,
`Containers`.

**mutation family (16):** `Reboot`, `Shutdown`, `Reset`, `Upgrade`, `Rollback`, `MetaWrite`,
`MetaDelete`, `ServiceStart`, `ServiceStop`, `ServiceRestart`, `ImagePull`,
`EtcdLeaveCluster`, `EtcdRemoveMemberByID`, `EtcdForfeitLeadership`, `EtcdDefragment`,
`EtcdRecover` (plus the `EtcdDowngrade*` pair, which shares the same disposition).

**streaming family (10):** `Logs`, `Dmesg`, `Events`, `Read`, `Copy`, `List`, `DiskUsage`,
`ImageList`, `EtcdSnapshot`, `PacketCapture`.

Grouping these three families into one matrix row each is deliberate: the deadline and retry
policy in `02-CONTEXT.md` `<deadline_policy>` is authored per class, and a per-method matrix
would restate that table without adding a decision. `talossim` implements the full 54-method
`MachineServiceServer` surface regardless (TRANS-06) — the opt-out is about the *production
client wrapper*, not about the fake.
