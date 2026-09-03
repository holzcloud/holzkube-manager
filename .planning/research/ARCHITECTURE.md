# Architecture Research

**Domain:** Out-of-cluster Talos Linux node & cluster control plane (single Go binary, embedded React UI, direct gRPC to `:50000`)
**Researched:** 2026-08-27
**Confidence:** MEDIUM-HIGH — API surfaces verified against `pkg.go.dev` and official Talos v1.13 docs; the structural design is opinionated synthesis. See [Confidence & Sourcing](#confidence--sourcing).

> Read [FEATURES.md](./FEATURES.md) first. This document does not re-derive the 15-step provisioning decomposition; it turns it into a persisted state machine, a package layout, and a build order.

---

## The Three Load-Bearing Constraints

Everything below follows from three facts, and every design choice should be checked against them:

1. **The process holds cluster PKI and can wipe machines.** The state directory is functionally equivalent to root on every node. Security is not a phase; it is a property of the store and the mutation path.
2. **holzkube must work when the cluster does not.** That is its entire structural advantage over `tuppr`, `talos-operator`, and Kubernetes Dashboard. Any read path that transitively depends on `:6443` throws that advantage away.
3. **The developer has no Talos hardware and no `talosctl`.** A capability to run the real client against a real gRPC listener must exist before feature work, or every phase after Phase 1 is written blind.

---

## Standard Architecture

### System Overview

```
┌──────────────────────────────────────────────────────────────────────────┐
│  BROWSER  (React + TS + Vite, served from embed.FS)                      │
│  ┌────────────┐ ┌────────────┐ ┌────────────┐ ┌────────────────────────┐ │
│  │ Dashboard  │ │ Node detail│ │ Provision  │ │ ONE SSE connection     │ │
│  │            │ │ + xterm.js │ │ wizard     │ │ /api/v1/stream         │ │
│  └────────────┘ └────────────┘ └────────────┘ └────────────────────────┘ │
└─────────────────────────────┬────────────────────────────────────────────┘
                              │ HTTPS: JSON REST (commands) + SSE (events)
┌─────────────────────────────▼────────────────────────────────────────────┐
│  HTTP LAYER  internal/httpapi                                            │
│  recover → requestID → session auth → CSRF → sudo-gate → audit → handler  │
├──────────────────────────────────────────────────────────────────────────┤
│  DOMAIN SERVICES  (all cluster-scoped; none import net/http)             │
│  ┌──────────┐┌──────────┐┌──────────┐┌──────────┐┌──────────┐┌─────────┐ │
│  │inventory ││ cluster  ││  config  ││ discovery││ factory  ││  jobs   │ │
│  │(UUID-key)││(import,  ││(gen/patch││(scan +   ││(Image    ││(engine: │ │
│  │          ││ secrets) ││ /diff)   ││ manual)  ││ Factory) ││provision│ │
│  └──────────┘└──────────┘└──────────┘└──────────┘└──────────┘│ upgrade)│ │
│  ┌──────────────────────┐┌──────────────────────┐            └─────────┘ │
│  │ health (HealthLevel  ││ streamhub            │                        │
│  │ supervisors per node)││ (fan-out, ring, gaps)│                        │
│  └──────────────────────┘└──────────────────────┘                        │
├──────────────────────────────────────────────────────────────────────────┤
│  TALOS ACCESS  internal/talos                                            │
│  ┌─────────────────────┐  ┌──────────────────────────────────────────┐   │
│  │ pool (conn cache,   │  │ NodeClient        MaintenanceClient      │   │
│  │ deadlines, retry-   │  │ (cluster mTLS)    (insecure, 6 RPCs only)│   │
│  │ reads-only, breaker)│  └──────────────────────────────────────────┘   │
│  └─────────────────────┘                                                 │
│  ══════════════ TRANSPORT SEAM: Dialer + DiscoverySource ══════════════   │
│  ┌────────────┐   ┌────────────┐   ┌──────────────┐  ┌────────────────┐  │
│  │ direct     │   │ fake       │   │ (v2) tunnel  │  │ (v2) tunnel    │  │
│  │ net.Dial   │   │ in-process │   │ SideroLink   │  │ registration   │  │
│  │ :50000     │   │ listener   │   │ resolve+dial │  │ (inbound)      │  │
│  └────────────┘   └────────────┘   └──────────────┘  └────────────────┘  │
├──────────────────────────────────────────────────────────────────────────┤
│  PERSISTENCE  internal/store  (0600 files, atomic rename, rev/CAS)       │
│  ┌────────┐┌────────┐┌────────┐┌────────┐┌────────┐┌────────┐┌────────┐  │
│  │machines││clusters││secrets ││patches ││schemat.││ jobs   ││ audit  │  │
│  └────────┘└────────┘└────────┘└────────┘└────────┘└────────┘└────────┘  │
└──────────────────────────────────────────────────────────────────────────┘
        │ gRPC :50000 mTLS                              │ HTTPS
        ▼                                               ▼
  Talos nodes (apid)                            factory.talos.dev
```

**The only two external dependencies of the running process are Talos nodes on `:50000` and the Image Factory over HTTPS.** Nothing else. That is worth protecting.

### Component Responsibilities

| Component | Owns | Must NOT know about |
|-----------|------|---------------------|
| `httpapi` | Routing, JSON codec, middleware chain, SSE endpoint, embedded asset serving | Talos, gRPC, file paths, secret material |
| `auth` | argon2id verification, session lifecycle, sudo-mode re-auth, login rate limiting | Anything Talos; users as a plural concept |
| `store` | Atomic 0600 writes, per-entity locking, `rev` CAS, schema version + migrations, process lock, backup/restore | Domain semantics — it stores typed records, it does not interpret them |
| `audit` | Append-only JSONL, daily rotation, hash chain, attempt + outcome records | HTTP; it is called by middleware, it does not read requests |
| `inventory` | Machine records keyed by **UUID**; `cluster_id` (nullable), role, schematic ID, last-known addr, last-seen, lock flag | gRPC, dialing, live state (it stores facts, not liveness) |
| `cluster` | Cluster records, secrets bundle (create **and import**), derived talosconfig, bootstrap intent record | HTTP, browsers, job scheduling |
| `talos/transport` | `Dialer`, `Target`, credentials; direct/fake/(later)tunnel impls | Domain types — it knows UUIDs and addresses, nothing about "nodes" or "upgrades" |
| `talos/pool` | Connection caching, deadlines, retry policy, circuit breaking, reachability | Which cluster is "current" — everything is passed a `ClusterID` |
| `talos/client` | Typed wrappers: `NodeClient` (full) and `MaintenanceClient` (restricted to the 6 permitted RPCs) | Persistence, HTTP, jobs |
| `health` | `HealthLevel` axis, per-node COSI watch supervisors, staleness tracking, degradation | Mutations of any kind — read-only by construction |
| `streamhub` | Topic registry, one upstream stream per topic, fan-out, ring buffers, gap markers, sequence numbers | SSE/WebSocket framing (that is `httpapi`'s job) |
| `config` | Config generation from secrets, versioned patches, structural merge + diff, validation, required-apply-mode computation | Applying anything — it produces bytes, `jobs` applies them |
| `factory` | Image Factory HTTP client: POST schematic → ID, resolve ISO/installer/PXE URLs, extension catalog per version | Talos gRPC, clusters, nodes |
| `discovery` | `DiscoverySource` interface; subnet scan on `:50000`, manual IP entry, insecure identity probe | Writing inventory records (it emits candidates; the operator promotes them) |
| `jobs` | Generic persisted step engine: intent→act→confirm, leases, restart recovery, cancellation, progress events | The specifics of any one job — those are `JobKind` implementations |
| `provision` | The blank-machine state machine, as a `JobKind` | HTTP, storage internals |
| `upgrade` | Rolling Talos/K8s orchestration, as a `JobKind` | HTTP, storage internals |
| `talossim` | In-process fake Talos node: real protobufs, real listener, real mTLS, scriptable failures | Production code (it is imported only by tests and the dev command) |

---

## Recommended Project Structure

```
holzkube/
├── cmd/
│   ├── holzkubed/                # the product: server + embedded UI
│   │   └── main.go
│   └── holzkube-dev/             # dev-only: runs talossim, seeds fixtures
│       └── main.go               # build tag `dev` — keeps deps out of the product
├── internal/
│   ├── httpapi/
│   │   ├── router.go             # route table; the ONLY file that knows URL shapes
│   │   ├── middleware/           # recover, reqid, session, csrf, sudo, audit
│   │   ├── sse.go                # single multiplexed SSE endpoint
│   │   ├── problem.go            # RFC 9457 problem+json errors
│   │   └── handlers/             # thin: decode → call service → encode. No logic.
│   ├── auth/                     # argon2id, sessions, sudo-mode, rate limit
│   ├── store/
│   │   ├── store.go              # entity-shaped interface — NEVER leaks paths
│   │   ├── fsstore/              # the 0600-file implementation
│   │   ├── atomic.go             # tmp → fsync → rename → fsync(dir)
│   │   ├── lock.go               # process flock + per-entity mutex map
│   │   └── migrate/              # forward-only schema migrations
│   ├── audit/                    # JSONL + hash chain
│   ├── model/                    # shared record types + ClusterID/MachineID types
│   ├── inventory/                # machine records, UUID-keyed
│   ├── cluster/                  # cluster records, secrets, import, bootstrap intent
│   ├── talos/
│   │   ├── transport/
│   │   │   ├── transport.go      # Dialer, Target, Creds  ◀── THE SEAM
│   │   │   ├── direct/           # net.Dial to addr:50000
│   │   │   └── fake/             # bufconn / loopback listener
│   │   ├── pool/                 # conn cache, deadlines, retry, breaker
│   │   ├── client/               # NodeClient, MaintenanceClient
│   │   └── cosiwatch/            # WatchKind helpers, reconnect, bookmarks
│   ├── health/                   # HealthLevel, supervisors, staleness
│   ├── streamhub/                # topics, fan-out, ring buffers, gaps
│   ├── config/
│   │   ├── generate/             # machinery/config/generate wrapper
│   │   ├── patch/                # versioned patch store, merge
│   │   ├── diff/                 # STRUCTURAL diff of rendered configs
│   │   ├── applymode/            # compute required apply mode from a diff
│   │   └── redact/               # secret redaction for the UI + a walk-test
│   ├── factory/                  # Image Factory client
│   ├── discovery/                # DiscoverySource: scan/, manual/
│   ├── jobs/
│   │   ├── engine.go             # persist, lease, recover, cancel
│   │   ├── step.go               # Step: Intent/Act/Observe
│   │   └── kinds/
│   │       ├── provision/        # THE state machine
│   │       ├── upgradetalos/
│   │       ├── upgradek8s/
│   │       └── removenode/
│   └── talossim/                 # fake Talos node — imported by tests + holzkube-dev
│       ├── node.go               # MachineService + LifecycleService impls
│       ├── cosi.go               # inmem COSI state → protobuf/server.NewState
│       └── script.go             # scriptable failures: go-silent, reject, flap
├── sandbox/                      # SEPARATE Go module — see "Dependency weight"
│   └── ...                       # wraps siderolabs/talos pkg/provision (docker/qemu)
├── web/                          # React + TS + Vite
│   ├── src/
│   └── dist/                     # embedded via embed.FS
└── test/
    └── contract/                 # ONE suite, run against fake AND real
```

### Structure Rationale

- **`internal/model/` exists so `ClusterID` is a real type.** `type ClusterID string` and `type MachineID string` (the UUID). Making them distinct types means the compiler finds every place multi-cluster scoping was forgotten. This costs nothing today and is the entire multi-cluster insurance policy.
- **`store` is entity-shaped, not path-shaped.** `store.Machines().Get(ctx, id)` — never `store.ReadFile("machines/x.json")`. This is the escape hatch: when files stop being enough, `fsstore` is replaced by `sqlitestore` and nothing above changes.
- **`handlers/` are thin by rule.** If a handler contains a loop or a conditional about domain state, that logic belongs in a service. This is what keeps a future `holzkubectl` cheap.
- **`talossim` lives in `internal/`, not `test/`.** It is a real gRPC server, useful in `holzkube-dev` for demos and manual UI work, not just in `_test.go` files.
- **`sandbox/` is a separate Go module.** See below — this is not stylistic.

### Dependency weight: the two Talos modules

`github.com/siderolabs/talos/pkg/machinery` is **its own Go module** (verified: `go.mod` declares that module path). It is the light one — protobufs, client, config generation. That is all the product needs.

`pkg/provision` — the Docker and QEMU cluster provisioners — lives in the **root** `github.com/siderolabs/talos` module, which drags in a large fraction of an operating system.

> **Rule:** `cmd/holzkubed` must depend on `pkg/machinery` only. The real-cluster sandbox goes in a separate module under `sandbox/`, or behind a `//go:build sandbox` tag with its own `go.mod`. Getting this wrong produces a 200 MB binary with a supply-chain surface you did not intend, and it will be painful to unwind later.

---

## Architectural Patterns

### Pattern 1: The transport seam — `Dialer` **and** `DiscoverySource`

**What:** PROJECT.md commits to abstracting the transport so SideroLink is retrofittable. The honest finding is that **a dial abstraction alone is insufficient**, because a tunnel inverts the direction of contact: today holzkube finds nodes by scanning outward; with SideroLink, nodes register inward. Two seams are required, not one.

```go
// internal/talos/transport

// Target names a machine by stable identity. Addr is a HINT, never identity.
type Target struct {
    Cluster model.ClusterID // "" for an unassigned / maintenance-mode machine
    Machine model.MachineID // UUID — the stable key
    Addr    string          // last known "host:50000"; may be stale (DHCP)
}

type CredKind int
const (
    CredMaintenance CredKind = iota // insecure/self-signed, no client cert
    CredCluster                     // full cluster mTLS from the secrets bundle
)

type Creds struct {
    Kind        model.CredKind
    TLS         *tls.Config
    Fingerprint string // optional pinned server cert fingerprint (maintenance)
}

// Dialer is THE seam. Everything above it speaks Target; nothing above it
// constructs an address or a grpc.DialOption.
type Dialer interface {
    Kind() string // "direct" | "tunnel" | "fake"

    // Resolve maps stable identity to something dialable.
    // direct: read Addr / re-probe.  tunnel: look up the overlay IP.
    Resolve(ctx context.Context, t Target) (string, error)

    // DialOptions returns transport-specific gRPC options, which are then
    // handed to machinery's client.WithGRPCDialOptions.
    DialOptions(ctx context.Context, t Target, c Creds) ([]grpc.DialOption, error)

    // Probe is a cheap liveness + identity check that must work with
    // CredMaintenance (no cluster PKI available yet).
    Probe(ctx context.Context, t Target) (Identity, error)
}

// DiscoverySource is the SECOND seam — the one people forget.
// Scanning pushes outward; a tunnel registers inward. Same interface, opposite flow.
type DiscoverySource interface {
    Kind() string // "subnet-scan" | "manual" | "tunnel-registration" | "fake"
    Run(ctx context.Context, out chan<- Candidate) error
}
```

**Why this is implementable without forking machinery:** `client.New` accepts `WithGRPCDialOptions(...)` and `WithTLSConfig(...)` (verified on pkg.go.dev). A custom `grpc.WithContextDialer` is enough to route every connection through an arbitrary transport, including an in-process pipe. The seam is real, not aspirational.

**What sits above:** `pool` → `client` → all domain services. They see `Target`, `NodeClient`, `MaintenanceClient`.
**What sits below:** `direct` (a `net.Dialer`), `fake` (dial an in-process listener), later `tunnel`.

**Trade-off:** `Resolve` returning a string leaks the assumption that endpoints are addressable strings. That holds for TCP, WireGuard overlays, and unix sockets alike, so it is a safe leak. The genuinely risky assumption is `Probe` — a tunnel transport cannot probe a node that has not yet dialed in, so `Probe` must be allowed to return `ErrNotReachableYet` and callers must treat that as a state, not an error. Bake that into the contract now.

### Pattern 2: The connection pool, and direct-vs-proxied

Talos supports two addressing axes (verified): `-e/--endpoints` selects which node's `apid` terminates the connection; `-n/--nodes` selects which nodes execute the request. `apid` on the endpoint proxies and aggregates. In the Go client this is context metadata, not extra connections: `client.WithNodes(ctx, ...)` fans out and aggregates; `client.WithNode(ctx, n)` proxies a single node unprocessed.

**Recommendation: direct-per-node by default, proxying as an explicit, recorded fallback.**

| | Direct to each node | Proxied through a CP endpoint |
|---|---|---|
| Per-node reachability truth | **Real** — you learn what is actually up | Masked — you learn what the proxy can reach |
| Survives control-plane outage | **Yes** | No — dies with the endpoint node |
| Upgrading the endpoint node itself | Unaffected | **Breaks mid-operation** |
| Log/dmesg streams | One hop | Two hops, extra buffering |
| Nodes not directly routable | Fails | Works |
| One call for many nodes | N calls | 1 call, aggregated |

Because holzkube exists to be useful during outages (constraint 2), the masked-reachability and dies-with-the-endpoint rows are decisive. Proxying is a fallback, and **never** through a node that is the target of the current mutating step.

```go
// Pool key: cluster + endpoint + credential kind.
// Keying on address alone would cross-wire PKI between clusters — a security bug.
type poolKey struct {
    cluster  model.ClusterID
    endpoint string
    creds    model.CredKind
}
```

Lifecycle rules:

- **Lazy create, idle-TTL evict** (~5 min). At 5–20 nodes an unbounded cache would also be fine; the TTL exists so dead nodes stop consuming file descriptors.
- **Deadlines: per-call, never per-connection.** Unary budget ~5–10 s. Streams get **no** deadline — they are bounded by keepalive and the subscriber's context instead.
- **Retry only idempotent reads.** A blanket gRPC retry interceptor is the single most dangerous thing you can add to this codebase: it would silently retry `Bootstrap`, `Reset`, `ApplyConfiguration`, and `Upgrade`. The interceptor must carry an **allowlist of retryable full-method names**, and the default must be "do not retry."
- **Keepalive conservatively** (~30 s time, 10 s timeout, `PermitWithoutStream: false`). Aggressive client keepalive against `apid` risks `ENHANCE_YOUR_CALM` disconnects.
- **Circuit breaker per machine.** After N consecutive dial failures, back off to a long interval (cap ~60 s, jittered). A dead node must not cost more than a dead node's worth of work.

**`NodeClient` vs `MaintenanceClient` are different Go types, deliberately.** Maintenance mode permits only `apply-config`, `version`, `get` (non-sensitive resources), `meta`, `reset`, and `wipe disk`. Encoding that as a narrower interface makes "call `ServiceList` on a maintenance node" a **compile error** instead of a confusing runtime failure inside the provisioning wizard. This is a nearly free win and it protects the Core Value path.

### Pattern 3: The file store — atomic, CAS-guarded, path-hiding

```go
func writeAtomic(dir, name string, data []byte) error {
    tmp, err := os.CreateTemp(dir, "."+name+".tmp*")   // SAME directory
    if err != nil { return err }
    defer os.Remove(tmp.Name())
    if err := tmp.Chmod(0o600); err != nil { return err }
    if _, err := tmp.Write(data); err != nil { return err }
    if err := tmp.Sync(); err != nil { return err }     // durability before rename
    if err := tmp.Close(); err != nil { return err }
    if err := os.Rename(tmp.Name(), filepath.Join(dir, name)); err != nil { return err }
    return fsyncDir(dir)                                // rename itself must be durable
}
```

Three layers of concurrency control, each for a different failure:

| Layer | Mechanism | Prevents |
|-------|-----------|----------|
| Process | `flock` on `$STATE/.lock`, held for the process lifetime | Two `holzkubed` instances shredding each other |
| Entity | `map[string]*sync.Mutex` keyed by record identity, inside the store | Two goroutines interleaving read-modify-write |
| API | `rev uint64` on every record; writes require the expected `rev` | Two browser tabs silently clobbering each other → `409 Conflict` |

**Schema versioning:** a top-level `VERSION` file plus a `schema` field on each record. Migrations are forward-only `func(dir string) error`, run at startup while holding the process lock, and are preceded by an automatic `backups/pre-migration-<from>-<ts>.tar.gz`. Refuse to start if `VERSION` is *newer* than the binary — a downgrade must fail loudly, not corrupt.

**Backup/restore:** the state directory *is* the backup artifact; `tar czf` is the whole procedure. Ship `holzkubed backup` and `holzkubed restore` subcommands (an admin escape hatch, not the out-of-scope `holzkubectl`). Restore must refuse while the process lock is held.

### Pattern 4: The job engine — intent → act → observe

Provisioning, upgrades, and node removal are all long-running, restart-surviving, and partially non-idempotent. Build **one** engine, not three orchestrators.

```go
type Step struct {
    Name string

    // Act performs the side effect. May be non-idempotent.
    Act func(ctx context.Context, j *Job) error

    // Observe answers "did Act already happen?" WITHOUT causing it.
    // Used on restart when the engine finds this step in-flight.
    // A nil Observe means the step is NOT auto-resumable — the engine
    // must park the job and require a human. This is a feature.
    Observe func(ctx context.Context, j *Job) (done bool, err error)
}
```

The engine persists **before** and **after** every `Act`:

```
persist{step: N, phase: "intent"}  →  fsync  →  Act()  →  persist{step: N, phase: "done"}
```

A crash in the gap leaves `phase: "intent"`. On restart the engine runs `Observe`. If `Observe` is nil — as it is for `Bootstrap`, which has no cheap "did I already do this?" that is not itself the question — the job is parked as `needs_attention` and the operator is shown exactly what is unknown and how to resolve it.

> **This is the general rule the whole system rests on: every side-effecting step needs a corresponding read-only "did it happen?" query. Steps that lack one must be protected by a lease plus a persisted intent record instead of being retried.**

Other engine rules:

- **One mutating lease per cluster.** Two clusters may upgrade concurrently; one cluster may not run two mutating jobs. Reads are never blocked.
- **Cancellation is checked at step boundaries only,** and `cancel_requested` is persisted. Mid-`upgrade_stream` cancellation is physically impossible — the node is already rebooting. The UI must say *"cancel = do not start the next node"*, because a cancel button that implies otherwise is worse than no button.
- **Progress is emitted to `streamhub`, not to a WebSocket held by the initiating request.** The browser is a viewer. Closing the tab does nothing. This is what makes the flow survive a closed tab, as FEATURES requires.

### Pattern 5: The stream hub — one upstream per topic, drop-never-block

```
Talos gRPC server-stream  ──┐
(logs / dmesg / events)     │  ONE reader goroutine per topic
                            ▼
                     ┌─────────────┐    ring buffer (last K frames, for replay)
                     │ topic       │
                     │ refcount=3  │
                     └──┬───┬───┬──┘
      bounded chan (1024)│   │   │
                         ▼   ▼   ▼
                       sub  sub  sub   ──→ multiplexed onto ONE SSE conn per tab
```

- **Topic key:** `(cluster, machine, kind, params)` — e.g. `logs:svc=kubelet:tail=100`. Three viewers of the same kubelet log share **one** upstream gRPC stream. Refcounted; last unsubscribe cancels the upstream after a ~5 s linger so page navigation does not thrash it.
- **Backpressure rule, non-negotiable:** the topic reader **never blocks on a subscriber send**.
  ```go
  select {
  case sub.ch <- frame:
  default:
      sub.dropped++          // emit a {"type":"gap","dropped":N} marker later
  }
  ```
  One slow browser must not stall the gRPC reader, and must not stall the other viewers. Lost lines are acceptable; a stalled stream during a boot debug is not. **Always surface the gap** — silently dropped log lines during troubleshooting are actively harmful.
- **The ring buffer doubles as replay.** A subscriber joining late gets the last K frames immediately. This matters enormously for "the node started booting, then I opened the tab."

**SSE, not WebSocket, for v1 — and exactly one connection per tab.**

| | SSE | WebSocket |
|---|---|---|
| Direction needed here | Server → browser only ✅ | Bidirectional (unused) |
| Browser auto-reconnect + `Last-Event-ID` | **Built in** | Hand-rolled |
| Cookie auth / reverse proxies | Plain HTTP, just works | Upgrade handshake, more proxy edge cases |
| Debuggable with `curl` | **Yes** | No |
| Binary frames | No (not needed) | Yes |

The one real SSE trap: **HTTP/1.1 caps ~6 connections per origin.** With 5–20 nodes and several open panels a naive one-stream-per-panel design deadlocks the tab. Two fixes; take both:

1. Serve **HTTP/2** (holzkube should be HTTPS by default anyway — see Security), which removes the limit.
2. **Multiplex all subscriptions over a single `/api/v1/stream` SSE connection per tab**, with subscribe/unsubscribe as ordinary `POST`s and a `topic` field on every event. One reconnect code path, one auth path, no connection-limit risk. This is the recommended shape regardless of HTTP version.

Reconnection: every event carries `id: <topic>:<seq>`. On reconnect the browser sends `Last-Event-ID`; the hub replays from the ring if it can and emits a gap marker if it cannot. **Never fake continuity.**

Reserve WebSocket for a genuinely interactive future path. Note that xterm.js here is a *renderer* for log output, which is unidirectional — it does not require WebSocket. Talos has no SSH and no shell by design.

### Pattern 6: `HealthLevel` as a type, not a convention

FEATURES defines a NONE / NODE / ETCD / K8S axis. Make it a compile-time-visible concept so "works when the cluster is down" is testable rather than aspirational.

```go
type HealthLevel uint8
const (
    LevelNone HealthLevel = iota // bare machine, no PKI
    LevelNode                    // one node reachable on mTLS
    LevelEtcd                    // etcd quorum
    LevelK8s                     // kube-apiserver
)

// Every read-model field carries its provenance. The API returns this shape
// rather than omitting unavailable fields — omission is indistinguishable
// from "zero" in a UI, and that is how you get a blank dashboard.
type Field[T any] struct {
    Value       T            `json:"value,omitzero"`
    Level       HealthLevel  `json:"level"`
    Available   bool         `json:"available"`
    StaleSince  *time.Time   `json:"stale_since,omitempty"`
    Unavailable string       `json:"unavailable_reason,omitempty"`
}
```

**The testable assertion:** boot `talossim` with etcd and the Kubernetes API scripted down, request the node detail view, and assert every `LevelNode` field still has `Available: true`. That single test enforces constraint 2 forever. Without it, the K8s dependency creeps back in within three phases.

### Pattern 7: Watch primary, poll as heartbeat

Talos exposes COSI over the machine API as `client.COSI` (a `state.State`). `WatchKind(ctx, kind, ch, state.WithBootstrapContents(true))` delivers the current set as `Created` events, then a `Bootstrapped` marker, then live deltas — list-and-watch in one call, which is exactly the dashboard primitive.

**Use both, for different jobs:**

| Mechanism | Job | Interval |
|-----------|-----|----------|
| COSI `WatchKind` | Node facts: `MachineStatus`, `ServiceStatus`, `NodeAddress`, `Disks`, versions | Push |
| Unary `Version()` heartbeat | Liveness + a bound on how stale "watching" can silently become | 30–60 s, jittered |
| etcd member/health query | Cluster-level ETCD facts, separate supervisor | 30 s |
| Kubernetes facts | K8S-level only, separate supervisor, allowed to fail | 60 s |

A watch alone cannot promptly tell you a node vanished — a dead TCP connection can hide behind keepalive for a while. The heartbeat bounds that. Conversely, polling alone at dashboard latency would be a needless request storm against `apid`.

**At 5–20 nodes, one connection per node with several `WatchKind` subscriptions is entirely comfortable** — tens of goroutines, no scheduler pressure. Do not build a scheduler for this.

Degradation, per node, as an explicit machine:

```
unknown ──dial──> connecting ──ok──> watching ──stream err──> degraded
                       │                 ▲                        │
                       │  backoff+jitter │                        │ N failures
                       └─────────────────┴──────── down <─────────┘
                                                    │
                            keeps last-known snapshot + staleSince
```

**Never delete inventory data because a node is unreachable.** Grey it, stamp it "as of 4m ago", and say why. The browser shows a dashboard of *last known truth plus age*, which is the only honest thing to show and also the most useful during an outage.

### Pattern 8: The fake-first testing seam (and its one real risk)

Because there is no hardware and no `talosctl`, the highest-leverage thing in the entire codebase is an in-process fake Talos node that the **real, unmodified production client** can talk to. This is verified as buildable:

- `machinery/api/machine` exports `RegisterMachineServiceServer` and `RegisterLifecycleServiceServer` — the generated **server** interfaces are public.
- `cosi-project/runtime/pkg/state/protobuf/server.NewState(coreState)` wraps any `CoreState` into the gRPC State service, so an in-memory COSI store becomes a real Talos-shaped resource API.

```go
// internal/talossim — sketch
func New(t *testing.T, opts ...Option) *Sim {
    st := state.WrapCore(namespaced.NewState(inmem.Build))  // real COSI, in memory
    gs := grpc.NewServer(grpc.Creds(simTLS()))              // REAL mTLS
    machine.RegisterMachineServiceServer(gs, &machineSvc{st: st, script: ...})
    machine.RegisterLifecycleServiceServer(gs, &lifecycleSvc{...})
    v1alpha1.RegisterStateServer(gs, cosiserver.NewState(st))
    // ... listen on 127.0.0.1:0; the production Dialer points at it
}
```

Scriptable behaviours that the design specifically needs to exercise:
`go_silent(90s)` (the install window), `reject_apply(reason)`, `second_bootstrap_returns_AlreadyExists`, `flap_connection`, `slow_log_consumer`, `ip_changes_on_reboot` (DHCP), `etcd_down`, `k8s_down`, `version_out_of_supported_range`.

**The real risk, named: fake drift.** A fake written from documentation agrees with your assumptions, not with Talos. Three mitigations, in order of value:

1. **The fake serves the real generated protobufs and the real COSI server wrapper.** Wire shape cannot drift; only semantics can.
2. **One contract test suite, two backends.** `test/contract/` runs identically against `talossim` and against a Docker-provisioned real cluster. Divergence is a test failure, not a surprise in production.
3. **Record/replay.** Capture real responses once from the Docker sandbox and use them as fake fixtures.

### The sandbox ladder (three tiers, three different phases)

FEATURES says "sandbox in Phase 1, no exceptions." The more precise answer, given the Docker provisioner's limits, is a ladder:

| Tier | What | Cost | Can test | Cannot test | Needed by |
|------|------|------|----------|-------------|-----------|
| **0 — `talossim`** | In-process fake, real protobufs, real mTLS | Low; pure Go; runs in CI | Every client path, streaming, backpressure, job recovery, degradation, error branches | Real Talos semantics | **Phase 1 — blocking** |
| **1 — Docker provisioner** | `pkg/provision/providers/docker`; real Talos, real etcd, real COSI | Medium; Docker is present | Read paths against real resources, service status, cluster bootstrap, contract tests | **No disks, no maintenance mode, no upgrades** | Phase 2 |
| **2 — QEMU provisioner** | `pkg/provision/providers/qemu`; full VMs, ISO boot | High; slowest | **Maintenance mode → apply-config → install → reboot → join** — the Core Value | Real hardware quirks, amd64-specific behaviour | Before Phase 6 |

Two caveats to record now:

- **The dev machine is `darwin/arm64`; the target hardware is amd64.** QEMU on this host will run **arm64** Talos images natively (fast, via HVF); running amd64 images means emulation and is impractically slow. The machine API is architecture-independent, so the flow is validatable — but **amd64-specific schematic and installer paths cannot be validated locally.** Budget one real-hardware verification pass before declaring the Core Value done.
- `pkg/provision` lives in the heavy root module. Keep it in `sandbox/` as its own module (see [Dependency weight](#dependency-weight-the-two-talos-modules)).

---

## Data Flow

### Read path (dashboard)

```
Talos node COSI WatchKind ──► health supervisor (per node, per cluster)
                                     │ merges with
                                     ▼
                              inventory record (store, UUID-keyed)
                                     │
                                     ▼
                          read model: Field[T]{value, level, stale_since}
                                     │
                    ┌────────────────┴────────────────┐
                    ▼                                 ▼
        GET /api/v1/clusters/{c}/nodes        SSE event topic=nodes:{c}
             (snapshot on load)                  (deltas thereafter)
                    └────────────────┬────────────────┘
                                     ▼
                              React store → UI
```

**The browser never polls the Talos API and never opens more than one stream.** holzkube maintains exactly one upstream watch per node regardless of how many tabs are open. Fan-out is the server's job.

### Mutating path

```
POST /api/v1/clusters/{c}/nodes/{uuid}/reset
   {"confirm":"talos-cp-01","graceful":true}
        │
        ├─ session auth        → 401
        ├─ CSRF check          → 403   (custom header + Origin/Sec-Fetch-Site)
        ├─ sudo-mode gate      → 428   (destructive + session older than N min)
        ├─ AUDIT: attempt      → fsync BEFORE anything happens
        ├─ confirm-string check vs the stored record → 400  (SERVER-side)
        ├─ node-lock check     → 409
        ├─ cluster job lease   → 409 if a mutating job is running
        ▼
   jobs.Enqueue(kind=reset) ──► engine ──► pool ──► Dialer ──► node
        │                          │
        │                          └─► progress events ──► streamhub ──► SSE
        ▼
   202 Accepted {"job_id": "..."}   ← never 200; this is asynchronous
        │
        └─ AUDIT: outcome (success | error) appended with the same job_id
```

Two things to notice: **audit is written on the attempt, before the action** (log only successes and you lose exactly the forensics you need), and **destructive endpoints return `202` with a job ID**, never a synchronous `200`. Synchronous destructive endpoints are how you end up with a UI that lies about what finished.

### Live stream path

```
browser POST /api/v1/stream/subscribe {"topic":"logs:cluster=c1:machine=<uuid>:svc=kubelet"}
        │
        ▼
   streamhub.Subscribe(topic, sub)
        │  refcount 0 → 1: start upstream
        ▼
   pool.Get(cluster, target) → NodeClient.Logs(ctx, ...) [gRPC server-stream]
        │
        ▼
   topic reader goroutine ──► ring buffer (replay)
                         └──► non-blocking send to each subscriber chan
                                   │
                                   ▼
                        GET /api/v1/stream  (the ONE SSE conn for this tab)
                        id: logs:...:1041
                        data: {"topic":"logs:...","line":"..."}
                                   │
                                   ▼
                              xterm.js render
```

---

## The Provisioning State Machine

> This is the Core Value. It is persisted per machine (keyed by **UUID**), it spans reboots, and it must survive both a closed browser tab and a killed process. It is implemented as a `JobKind` on the generic engine, so it inherits intent→act→observe, leases, and restart recovery for free.

### States

| State | Meaning | Health | Persisted facts |
|-------|---------|--------|-----------------|
| `discovered` | Answered on `:50000` in maintenance mode; insecure probe returned UUID, version, disks, MACs | NONE | uuid, addr, version, disks, macs, first_seen |
| `identified` | Operator confirmed this is the intended physical machine; optional cert fingerprint pinned | NONE | + pinned_fingerprint?, operator note |
| `assigned` | Cluster, role, install disk, schematic chosen | NONE | + cluster_id, role, install_disk, schematic_id |
| `config_rendered` | MachineConfig generated from cluster secrets + role + patches; `machine.install.image` derived from **the same schematic**; validated | NONE | + config_hash, rendered blob, validation result |
| `approved` | Diff + dry-run shown; operator approved. Separate state so approval is an auditable event | NONE | + approved_by, approved_at, config_hash (frozen) |
| `applying` | `ApplyConfiguration` in flight over the insecure maintenance connection | NONE | + apply_started_at |
| `installing` | Apply accepted; the machine is writing to disk and rebooting. **The silent window.** | NONE | + install_started_at, expected_duration |
| `awaiting_mtls` | Expected back on cluster PKI; probing with cluster credentials, matching on UUID not IP | NONE→NODE | + probe_attempts, last_probe |
| `bootstrap_required` | Control-plane node, and the cluster has no etcd members. Blocked on a guarded action | NODE | + bootstrap gate reference |
| `bootstrapping` | `Bootstrap` RPC in flight. Guarded by a cluster lease + a persisted intent record | NODE→ETCD | + bootstrap_intent_id |
| `bootstrap_indeterminate` | Process died between bootstrap intent and confirmation. **Never auto-retried.** | NODE | + intent record, resolution instructions |
| `joining` | Talos healthy on mTLS; awaiting etcd membership (CP) or Node registration (worker) | ETCD | + join_started_at |
| `ready` | Healthy node, promoted into the inventory as a cluster member. Terminal success | NODE | + joined_at |
| `failed` | Terminal failure with a reason code and a `retry_from` marker naming the earliest safe state | varies | + reason_code, detail, retry_from |
| `cancelled` | Operator cancelled at a step boundary. Machine may be mid-anything — say so | varies | + cancelled_by, last_state |
| `abandoned` | Operator gave up. Record retained for audit; machine returns to the unassigned pool | — | + abandoned_at |

### Transitions and failure edges

| From | Event / step | To | On failure | Retry safety |
|------|--------------|-----|-----------|--------------|
| — | scan or manual IP finds `:50000` in maintenance mode | `discovered` | no answer → not created | Safe; scanning is read-only. **Never auto-act on a discovery.** |
| `discovered` | operator confirms identity (± fingerprint) | `identified` | fingerprint mismatch → `failed(fingerprint_mismatch)` | Safe. Mismatch is a **possible MITM** — surface it, do not shrug |
| `identified` | choose cluster / role / disk / schematic | `assigned` | even CP count → **warn**, do not block; wrong disk → operator's call | Safe; pure metadata |
| `assigned` | generate MachineConfig | `config_rendered` | secrets missing → `failed(no_cluster_secrets)`; invalid patch → `failed(invalid_config)`; schematic mismatch → **hard block** | Safe & idempotent (pure function of inputs) |
| `config_rendered` | validate + dry-run + diff | `config_rendered` | validation error → stay, show error | Safe |
| `config_rendered` | operator approves | `approved` | — | Safe. `config_hash` freezes here; a later patch edit invalidates approval |
| `approved` | `ApplyConfiguration` (`--insecure`) | `applying` | — | — |
| `applying` | apply returns OK | `installing` | conn drop mid-apply → **`indeterminate`**: re-probe; still in maintenance ⇒ not applied, retry; on mTLS ⇒ it worked | **Observable.** Re-probe is the discriminator; do not blind-retry |
| `applying` | apply rejected | `failed(apply_rejected)` | — | Safe to retry after fixing config |
| `installing` | **three-way reappearance probe** (below) | `awaiting_mtls` / `failed` / stay | timeout budget exceeded → `failed(install_timeout)` + console-access advice | Probing is read-only; retry freely |
| `awaiting_mtls` | mTLS probe succeeds, UUID matches | `joining` or `bootstrap_required` | UUID mismatch → `failed(identity_mismatch)` — you are talking to a **different machine** | Safe |
| `awaiting_mtls` | machine answers but still in **maintenance mode** | `failed(install_did_not_happen)` | Almost always: a prior Talos install on disk shadowed the ISO | Safe; needs `wipe disk` or a boot-order change |
| `bootstrap_required` | pre-flight: `EtcdMemberList` returns members | `joining` | — | Refuse to bootstrap. **Observation beats assumption.** |
| `bootstrap_required` | acquire cluster bootstrap lease + write intent + fsync | `bootstrapping` | lease held → `409`, stay | Lease is the concurrency guard |
| `bootstrapping` | `Bootstrap` returns OK → write `done` | `joining` | `AlreadyExists` → treat as success, record it | Talos's own guard, used as a **second** line of defence |
| `bootstrapping` | **process dies before confirmation** | `bootstrap_indeterminate` | — | **Never auto-retried.** Resolve by *querying* etcd membership |
| `bootstrap_indeterminate` | operator triggers reconcile → `EtcdMemberList` | `joining` (members exist) or `bootstrap_required` (none) | node unreachable → stay | The query is read-only and settles the question |
| `joining` | etcd member healthy (CP) / Node registered (worker) | `ready` | CNI absent → Talos healthy but K8s `NotReady`. **This is normal.** Show it as expected, not red | Safe |
| `joining` | time skew / CA mismatch / wrong endpoint | `failed(join_failed)` + specific reason | — | Retry from `config_rendered` after fixing |
| `ready` | — | terminal | — | — |
| any non-terminal | operator cancels at a step boundary | `cancelled` | — | Must state what physical state the machine is likely in |
| `failed` | operator retries | `retry_from` state | — | `retry_from` is **computed**, never "start over" — restarting a machine that already installed is a wipe |

### The silent window: a three-way probe, not a spinner

`installing` is the scariest state in the product (FEATURES step 12). The machine stops answering; a spinner tells the operator nothing. Discriminate:

```
        ┌──────────────────────────────────────────────┐
        │ every 10s while in `installing`               │
        └───────────────┬──────────────────────────────┘
                        ▼
   A) insecure probe @ last known addr → answers, MAINTENANCE MODE
      ⇒ the install did NOT happen (config lost / rebooted into ISO)
      ⇒ failed(install_did_not_happen) — actionable, specific

   B) mTLS probe with CLUSTER creds @ last known addr,
      AND @ any newly-discovered addr whose UUID matches
      ⇒ install succeeded (possibly at a NEW IP — DHCP moved it)
      ⇒ awaiting_mtls → joining

   C) nothing answers anywhere
      ⇒ still installing. Show ELAPSED TIME against an expected budget,
        not a spinner: "installing — 2m14s elapsed, typically 3–6 min"
      ⇒ on budget exceeded: failed(install_timeout) + console-access advice
        (Talos has no SSH by design; the console is the only recourse)
```

Case B is precisely why inventory is UUID-keyed. An IP-keyed design cannot distinguish "my machine came back at a new address" from "a stranger appeared."

### Making a double bootstrap structurally impossible

FEATURES flags a repeated `Bootstrap` as catastrophic. "Guarded by a confirmation dialog" is not a guarantee. Four independent layers, each covering a different failure:

| Layer | Mechanism | Covers |
|-------|-----------|--------|
| 1. Observation | `EtcdMemberList` pre-flight; refuse if members exist | The ordinary case, and post-crash reconciliation |
| 2. Lease | `clusters/<id>/locks/bootstrap.lock` via `O_CREAT\|O_EXCL` | Two concurrent requests / two jobs |
| 3. Persisted intent | `bootstrap.json` written **and fsynced before** the RPC: `{state: in_progress, machine, at}` | Process death mid-RPC — the record survives |
| 4. Talos itself | `Bootstrap` returns `AlreadyExists` on a bootstrapped node | Everything the above missed |

The state `bootstrap_indeterminate` exists specifically because layer 3 detects the crash but cannot resolve it. **Resolution is layer 1: ask etcd.** That is the whole pattern — a non-idempotent action is made safe by pairing it with a read-only query that reveals whether it happened.

`bootstrap.json` is **cluster-scoped and single-valued**, so "bootstrap a second node" has nowhere to be recorded. The data model refuses the operation before the code does.

---

## State Model (on disk)

```
$HOLZKUBE_STATE/                       # ~/.holzkube or /var/lib/holzkube, mode 0700
├── .lock                              # flock, held for process lifetime
├── VERSION                            # schema version, plain integer
├── config.yaml                        # 0600 — listen addr, TLS, state dir, factory URL
├── auth/
│   └── users.json                     # 0600 — argon2id hash + params (t,m,p)
├── machines/                          # FLAT, UUID-addressed, ALL machines
│   └── <machine-uuid>.json            # 0600 — cluster_id is a nullable FIELD
├── clusters/
│   └── <cluster-id>/
│       ├── cluster.json               # 0600 — name, endpoint, versions, imported|created
│       ├── secrets.yaml               # 0600 — THE PKI BUNDLE. root-equivalent.
│       ├── talosconfig                # 0600 — derived; regenerable from secrets
│       ├── bootstrap.json             # 0600 — single-valued: none|in_progress|done|indeterminate
│       ├── patches/<patch-id>/
│       │   ├── meta.json              # name, scope (cluster|node), target, rev
│       │   └── v<N>.yaml              # append-only versions — never mutated
│       └── locks/                     # O_CREAT|O_EXCL leases: bootstrap.lock, job.lock
├── schematics/
│   └── <schematic-id>.json            # source YAML + resolved ISO/installer/PXE URLs
├── jobs/
│   ├── <job-id>.json                  # 0600 — kind, cluster, targets, cursor, phase
│   └── <job-id>.jsonl                 # 0600 — append-only step log
├── audit/
│   └── 2026-08-27.jsonl               # 0600, daily, 30-day retention, hash-chained
└── backups/
    └── pre-migration-3-20260827.tar.gz
```

### Why machines are flat and cluster-scoping is a field

The tempting layout puts assigned machines under `clusters/<id>/nodes/`. Resist it:

- **Assignment would become a file move**, which is a rename race in the middle of provisioning — exactly where you least want one.
- **UUID lookup would require scanning every cluster directory**, and UUID lookup is the single hottest operation (the reappearance probe does it constantly).
- **"Unassigned machine" would be a special case** rather than `cluster_id: null`, and special cases are where multi-cluster support quietly dies.

Flat `machines/<uuid>.json` with a nullable `cluster_id` makes discovery, assignment, and node removal all pure field updates. `clusters/<id>/` holds only genuinely cluster-owned artifacts: secrets, patches, bootstrap state, leases.

### Where this design strains, and the escape hatch

| Strain | When | Severity |
|--------|------|----------|
| `ReadDir` + unmarshal per list request | ~hundreds of machines | Low — fix with an in-memory index built at startup, maintained on write. A cache, not a rewrite. |
| No queries ("all nodes with schematic X and Talos < 1.9") | The moment you want fleet reporting | **Medium — this is the real wall.** Files have no index. |
| No cross-entity transactions | Assign machine + update cluster atomically | Medium — today solved by ordering writes so a crash leaves a recoverable state, which is fine at this scale |
| Audit / job log growth | Bounded by rotation and retention | Non-issue |

**The escape hatch is the `store` interface, and it only works if the interface never leaks paths.** Because callers say `store.Machines().Get(ctx, id)` and never `ReadFile`, replacing `fsstore` with `sqlitestore` or `bboltstore` is one new implementation and zero changes above. That discipline costs nothing on day one and is the entire insurance policy. **Any `os.ReadFile` outside `internal/store/fsstore/` is an architecture bug** — worth a lint rule.

The counter-pressure worth respecting: PROJECT.md chose files precisely because they are human-readable and backed up with ordinary tools. That is a genuinely good reason at 5–20 nodes. Do not pre-emptively adopt SQLite; just keep the door unlocked.

---

## Upgrade Orchestration

A rolling upgrade is a `JobKind` on the generic engine. Per node, in order:

| # | Step | `Observe` (restart-safe?) | Notes |
|---|------|---------------------------|-------|
| 1 | `precheck_node_health` | n/a (read-only) | Refuse if the node is already unhealthy |
| 2 | `precheck_quorum_survives_loss` | n/a | **Before** touching a CP node, confirm etcd survives losing it. This is the gate that prevents the #1 cluster-killer |
| 3 | `precheck_node_lock` | n/a | Locked nodes are skipped, visibly |
| 4 | `resolve_installer_image` | n/a | From the node record's **`schematic_id`**. Without it, extensions silently vanish |
| 5 | `check_kernel_args_drift` | n/a | The v1.13 bug: GRUB + `grubUseUKICmdline: false` ⇒ the new API drops `extraKernelArgs` and **reports success**. Warn; offer `--legacy` |
| 6 | `cordon` (K8s) | ✅ read node's spec | Skipped with a visible note when K8s is unreachable |
| 7 | `upgrade_stream` | ✅ read the node's version | v1.13 streaming `LifecycleService.Upgrade`; progress → `streamhub` |
| 8 | `wait_node_back` | ✅ probe | Same three-way probe logic as provisioning — reuse it |
| 9 | `wait_etcd_member_healthy` | ✅ query | CP only. **The health gate between nodes** |
| 10 | `uncordon` | ✅ read spec | |
| 11 | `verify_version` | ✅ read | Catches the "succeeded but didn't change" class |

Design consequences worth stating explicitly:

- **The health gate is a step, not an ambient condition.** It appears in the job log and the audit trail, so "why did it wait 4 minutes" is answerable after the fact.
- **Every step here has a real `Observe`.** That makes Talos upgrades safely **auto-resumable** after a crash — unlike provisioning, which parks at bootstrap. Different default policies for different job kinds, and the reason is structural (observability of the step), not a preference.
- **Cancellation semantics, stated honestly:** cancel is checked between nodes. A node already rebooting into a new Talos version will finish doing so. The UI must read *"Stop after the current node"*, never a bare "Cancel."
- **Process dies mid-upgrade:** on startup, scan `jobs/` for non-terminal jobs, enter `recovering`, run the in-flight step's `Observe`, then resume. If `Observe` returns an error (node unreachable), park as `needs_attention` with the specific unknown surfaced — never guess.
- **Kubernetes upgrade is a separate `JobKind`** (prepull → static pods → kube-proxy → kubelet), gated at `LevelK8s`, deferred per FEATURES. It shares the engine and nothing else. Adopt Omni's behaviour of **showing bootstrap-manifest diffs rather than auto-applying** them — that is the correct default.

---

## Multi-Cluster: Exactly Where Scoping Must Appear

One cluster exists today. These ten points are where `cluster_id` must be present from the first commit; every one is cheap now and expensive later.

| # | Scoping point | Cost if skipped |
|---|---------------|-----------------|
| 1 | `type ClusterID string` in `internal/model`, first parameter of every service method | The compiler finds nothing; you find it by hand across 200 call sites |
| 2 | Store layout: `clusters/<id>/...`; `cluster_id` field on machine records | Data migration + a rename race |
| 3 | Routes: `/api/v1/clusters/{clusterID}/...` (only `/machines` for unassigned, `/schematics`, `/auth` are global) | Every URL and every frontend fetch changes |
| 4 | **Pool key includes `ClusterID`** | **Security bug:** an address-keyed pool cross-wires PKI between clusters |
| 5 | Credential resolution is `secretsFor(clusterID)` — never a package-level `var talosconfig` | A global talosconfig is the single hardest thing to unpick later |
| 6 | Job records carry `cluster_id`; the mutating lease is **per cluster** | Cluster B blocks on cluster A's upgrade for no reason |
| 7 | Stream topic keys include `cluster_id` | Cross-cluster log bleed — subtle and alarming |
| 8 | One health supervisor set **per cluster** | A global supervisor cannot express per-cluster degradation |
| 9 | Audit records carry `cluster_id` | Your audit history becomes unattributable |
| 10 | UI routes `/clusters/:id/...` from day one; the switcher is a no-op with one cluster | A full frontend routing rewrite |

**Cluster import is the first path exercised, not bootstrap.** The author's homelab cluster already exists (PROJECT.md Context). The import flow — accept an existing secrets bundle or talosconfig, probe the endpoint, enumerate etcd members and nodes, populate the UUID-keyed inventory — must be a first-class service in `internal/cluster`, designed alongside creation rather than bolted on. Concretely: `cluster.Import(ctx, ImportRequest)` and `cluster.Create(ctx, CreateRequest)` both produce the same `Cluster` record with an `origin` field. A design that assumes "we generated these secrets" will not survive contact with a real cluster.

---

## Security Architecture

### Blast radius — state it plainly

`clusters/<id>/secrets.yaml` contains the cluster CA **private key**. Anyone who reads it can mint an admin `talosconfig` and an admin `kubeconfig`, and can therefore wipe every machine in the cluster. **The holzkube state directory is equivalent to root on every node.** Two consequences that must shape deployment docs, not just code:

1. The holzkube host is inside the cluster's trust boundary. It deserves control-plane-grade treatment, not "that Raspberry Pi in the corner."
2. Compromise of the host is compromise of the cluster. There is no partial-credential design that avoids this — config generation genuinely requires the CA key. Say so rather than implying defence in depth that does not exist.

### Secrets on disk

- `0600` files, `0700` directories. **Refuse to start if `stat(stateDir).Mode()&0o077 != 0`** — a three-line check that catches the most common real-world mistake.
- **No encryption at rest in v1.** PROJECT.md's "no crypto-eigenbau" is the right call: a passphrase-derived key that lives in the same process, on the same host, protects against approximately nothing while adding a key-management failure mode. The honest mitigation is **full-disk encryption** (FileVault / LUKS) plus host hygiene — document that as the requirement it is.
- Container/Compose deployment: the state dir must be a named volume or bind mount with correct ownership, and the container must not run as root. Ship that in the Compose file, not as prose.

### Secrets in memory

Load per operation from the store; do not keep a package-level global. Skip `mlock` and buffer-zeroing theatre — Go's GC copies and moves memory, so zeroing a `[]byte` does not reliably erase anything, and shipping it invites false confidence. Spend the effort where it actually pays:

**Secrets must never appear in:** logs, error strings, HTTP responses, audit records, job step params, or the rendered-config view.

Redaction deserves its own package and its own test. A regex over YAML will miss a field on the next Talos release. The workable design is a **JSON-path denylist** (`.machine.ca.key`, `.cluster.ca.key`, `.cluster.etcd.ca.key`, `.cluster.secret`, `.machine.token`, `.cluster.token`, `.cluster.aescbcEncryptionSecret`, `.cluster.secretboxEncryptionSecret`, service-account key, …) **plus a fail-closed rule for unknown keys under a known-secret parent** — plus the thing that actually guarantees it:

> A test that walks a freshly generated MachineConfig through the redactor and asserts that **no PEM block and no high-entropy base64 run above N bytes survives**. That test, not the denylist, is the real guarantee — and it keeps working when Talos adds a field.

### Sessions and transport

- **HTTPS by default.** A session cookie granting access to cluster PKI, sent in plaintext over a home LAN, is a real vulnerability, and `Secure` cookies require TLS anyway. Generate a self-signed cert on first run, accept a provided cert, and allow `--insecure-http` **only when bound to loopback**. HTTP/2 comes along for free and removes the SSE connection-limit problem.
- Cookie: `HttpOnly; Secure; SameSite=Lax; Path=/`. Opaque 256-bit session ID, **rotated on login**. Idle expiry plus an absolute cap.
- **In-memory session store, deliberately.** A restart logging everyone out is a feature for a single-operator tool, and it removes a class of on-disk secret.
- Login: constant-time comparison, argon2id at roughly `t=3, m=64 MiB, p=4`, **with the parameters stored beside the hash** so they can be raised without invalidating the password. Rate-limit per source IP and globally.

### CSRF

`SameSite=Lax` is necessary but not sufficient. For a JSON API the cheap, robust combination is:

1. Require `Content-Type: application/json` on all mutating requests, **and**
2. Require a custom header (`X-Holzkube-CSRF: 1`), **and**
3. Validate `Origin` / `Sec-Fetch-Site` on mutating routes.

A cross-origin HTML form cannot set (1) or (2) — those are outside the CORS "simple request" envelope, so the browser preflights and the request never arrives. This is near-zero cost and does not require token plumbing. A double-submit token can be layered on later if desired.

### Confirmation gates — server-enforced, or worthless

```
POST /api/v1/clusters/{c}/nodes/{uuid}/reset
{"confirm": "talos-cp-01", "graceful": true}
```

The **server** compares `confirm` against the stored hostname and rejects a mismatch. A client-side dialog protects against nothing — not a UI bug, not a mis-scripted request, not a confused operator with two tabs open.

Layer **sudo-mode** on top: destructive actions (`reset`, `wipe`, `etcd member remove`, `bootstrap`) require re-entering the password if the session is older than N minutes → `428 Precondition Required`. This is the only mechanism that meaningfully limits the damage from a stolen session cookie, and it is perhaps 40 lines.

Also architectural, not cosmetic: the **maintenance-mode MITM window** (FEATURES step 7). Maintenance mode uses a self-signed cert with no mutual verification. Expose optional `--cert-fingerprint` pinning, persist the pinned fingerprint on the machine record, verify it on every subsequent maintenance connection, and state in the UI that the fingerprint is obtainable only from a physical/serial/IPMI console. Papering over this in the wizard would be dishonest.

### Audit integrity

Format: JSONL, one file per day, `0600`, opened `O_APPEND`, `fsync` per record (volume is trivial — a handful per minute at most).

```json
{"seq":1041,"ts":"2026-08-27T09:14:02Z","actor":"holz","session":"a1b2…",
 "src_ip":"192.168.1.10","cluster_id":"c-homelab","machine_id":"<uuid>",
 "action":"node.reset","params":{"graceful":true},"job_id":"j-88",
 "outcome":"attempt","prev_hash":"…","hash":"…"}
```

- **Write the attempt before acting; append the outcome after.** Success-only logging discards exactly the forensics you need.
- **Hash chain:** `hash_n = sha256(hash_{n-1} || canonical(record_n))`. This makes silent mid-file tampering detectable, and `holzkubed audit verify` becomes a real command.
- **State the limit honestly:** an attacker with write access to the state directory can rewrite the entire chain. Hash chaining is tamper-*evidence* against corruption and casual edits, not tamper-*proofing*. Genuine tamper-proofing requires shipping records off-box (syslog / an append-only remote) — note it as a v2 option rather than overselling what ships.
- Params are redacted through the same redactor as the config viewer. Same code path, same test.

### Egress

`factory.talos.dev` is the only outbound dependency of the process. Keep it isolated in `internal/factory` with a configurable base URL (self-hosted Image Factory is a real deployment mode), a strict timeout, and no ability to influence anything except schematic records. Note that Image Factory sees your schematic contents — extension choices and kernel args — which is a mild information disclosure worth mentioning in docs.

---

## Scaling Considerations

Reframed for this domain — the scale axis is nodes and clusters, not users. There is one user.

| Scale | What holds | What to change |
|-------|-----------|----------------|
| 1 cluster, 5–20 nodes (today) | Everything. One goroutine set per node, files on disk, watch + heartbeat, in-process fan-out. | Nothing. Do not optimise. |
| 3 clusters, ~60 nodes | Still fine. ~60 gRPC conns, ~180 watch subscriptions. | Add the in-memory inventory index. Make the heartbeat interval configurable. |
| 10 clusters, ~300 nodes | Strains. `ReadDir` on every list is felt; per-node goroutine sets get noisy; a subnet scan across many /24s is slow. | Swap `fsstore` → SQLite behind the existing interface. Pool the watch supervisors. Make discovery scans concurrent and bounded. |
| Beyond | Out of scope by design. | This is Omni's territory, and Omni's architecture (tunnel + registration + a real datastore) is the correct one there. |

### Scaling priorities

1. **First bottleneck: per-request `ReadDir` + unmarshal of the machines directory.** Fix with an in-memory index rebuilt at startup, maintained on write. Cheap and non-structural.
2. **Second bottleneck: goroutine-per-node-per-watch.** Fix by consolidating each node's subscriptions onto one connection (which the design already does) and only then by pooling supervisors.
3. **Third: subnet scanning at scale.** Bounded concurrency and a fast dial timeout — but honestly, at that scale scanning is the wrong discovery model and the `DiscoverySource` seam exists precisely so it can be replaced.

---

## Anti-Patterns

### 1. A global `*client.Client` or a package-level talosconfig
**What people do:** `var talosClient *client.Client` initialised in `main`.
**Why it's wrong:** it hard-codes one cluster, cross-wires PKI the moment a second cluster appears, and makes the maintenance-mode client impossible to express. It is the single hardest thing to unpick later.
**Instead:** `pool.Get(ctx, clusterID, target, credKind)`, keyed on `(cluster, endpoint, credKind)`.

### 2. A blanket gRPC retry interceptor
**What people do:** add a `grpc.WithDefaultServiceConfig` retry policy "for reliability."
**Why it's wrong:** it will retry `Bootstrap`, `Reset`, `ApplyConfiguration`, and `Upgrade`. A retried `Bootstrap` is the catastrophic case FEATURES warns about, and a retried `Reset` wipes a machine twice.
**Instead:** an explicit allowlist of retryable full-method names; default is no retry. Non-idempotent calls get intent records and observation queries instead.

### 3. Wizard state in React
**What people do:** hold provisioning progress in component state, drive the flow from the browser.
**Why it's wrong:** the flow spans a reboot and 3–6 minutes of silence. Closing the tab, sleeping the laptop, or a Wi-Fi hiccup loses the job — and the machine keeps installing regardless.
**Instead:** a persisted job. The browser subscribes to it and can be closed at any point.

### 4. IP-keyed inventory
**What people do:** `map[string]*Node` keyed by address, because it is what you have during discovery.
**Why it's wrong:** DHCP moves addresses, and it moves them precisely across the reboot in the middle of provisioning. The record then either duplicates or attaches to a stranger.
**Instead:** UUID-keyed; the address is a mutable hint. This is a data-model decision, expensive to retrofit.

### 5. Letting the browser fan out to nodes
**What people do:** the frontend polls `/nodes/:id/status` for each node on a timer.
**Why it's wrong:** N tabs × M nodes × 1 Hz against `apid`, and every tab sees different data.
**Instead:** one upstream watch per node in the server; the browser gets a snapshot plus one SSE stream.

### 6. Text-diffing YAML
**What people do:** `diff` the old and new config strings.
**Why it's wrong:** key ordering, comments, and anchors produce noise that buries the one line that matters; and it cannot distinguish "changed" from "reformatted."
**Instead:** render both configs to structured values and diff **structurally**. Also distinguish *rejected* fields from *silently ignored* ones — Talos does both, and only one produces an error.

### 7. Treating "apply succeeded" as "config took effect"
**What people do:** report success when `ApplyConfiguration` returns OK.
**Why it's wrong:** Talos silently ignores some fields, and `--mode=staged` deliberately does not alter the running config.
**Instead:** re-read the resulting `MachineConfig` resource and diff against intent. Compute the *required* apply mode from the patch rather than letting the user pick one that will fail at apply time.

### 8. Any read path that transitively needs `:6443`
**What people do:** enrich the node list with Kubernetes node conditions, then render nothing when the call fails.
**Why it's wrong:** it converts the product's core structural advantage into its core failure mode.
**Instead:** `HealthLevel` on every field, separate supervisors per level, and the degradation test from Pattern 6.

### 9. Importing the root `siderolabs/talos` module into the product binary
**What people do:** `import "github.com/siderolabs/talos/pkg/provision"` in `internal/` for the sandbox.
**Why it's wrong:** it pulls a large fraction of an operating system into a management binary — build time, binary size, and supply-chain surface — and it is painful to unwind once anything else has grown a dependency on it.
**Instead:** `pkg/machinery` only in the product; the real-cluster sandbox lives in a separate module under `sandbox/`.

### 10. One SSE connection per panel
**What people do:** open `/api/stream/logs/:node` per open log view.
**Why it's wrong:** HTTP/1.1 caps ~6 connections per origin; with 5–20 nodes the tab deadlocks in a way that looks like a server hang.
**Instead:** one multiplexed `/api/v1/stream` per tab, with `POST`-based subscribe/unsubscribe.

---

## Integration Points

### External services

| Service | Integration pattern | Gotchas |
|---------|---------------------|---------|
| Talos machine API (`:50000`) | gRPC + mTLS via `pkg/machinery/client`; node targeting through context metadata | Version-coupled — declare a supported range and warn on out-of-range nodes. `-e` vs `-n` semantics: direct by default, proxy as fallback. |
| Talos maintenance mode (`:50000`, insecure) | Same client, `InsecureSkipVerify` or pinned fingerprint, **no client cert** | Only 6 RPCs available; unauthenticated in both directions. Model as a distinct Go type. |
| Talos COSI resource API | `client.COSI` (`state.State`); `WatchKind(..., WithBootstrapContents(true))` | Resource shapes change between Talos versions. Reconnect with backoff; `Bootstrapped` marks end-of-initial-set. |
| Image Factory (`factory.talos.dev`) | Plain HTTPS: `POST /schematics` → content-addressed ID; URL construction for ISO / installer / PXE | **Installer and initramfs honor extensions only — kernel args and META are silently dropped.** Warn at authoring time. Content-addressed ⇒ POST is idempotent ⇒ retry is always safe. |
| Kubernetes API (`:6443`) | Only for K8S-level facts, cordon/drain, and kubeconfig brokering | **Not holzkube's domain.** The seam is the port. Deep-link to k9s/Headlamp rather than growing workload views. |

### Internal boundaries

| Boundary | Communication | Notes |
|----------|---------------|-------|
| `httpapi` → services | Direct calls, `ClusterID` first param | Handlers stay thin; this is what keeps a future CLI cheap |
| services → `talos/pool` | `Target` + `CredKind`; never an address | Enforces the transport seam |
| `talos/*` → `transport.Dialer` | The seam | Only place that knows about addresses and dial options |
| `jobs` → services | Direct calls inside steps | Steps must be `Observe`-able or the kind must declare itself non-auto-resumable |
| everything → `store` | Entity-shaped interface | **No `os.ReadFile` outside `fsstore`** — worth a lint rule |
| `streamhub` → `httpapi` | Channel of framed events | Hub knows nothing about SSE framing |
| `audit` ← middleware | Called on the mutating path | Cross-cutting by construction, never per-feature |

---

## Build Order

Dependency-ordered. The overriding constraint: **no Talos hardware, no `talosctl`.** Phase 1 exists so that every later phase is testable at all.

```
Phase 0  ──► Phase 1 ──┬──► Phase 2 ──┬──► Phase 3 (streams)
(skeleton)   (seam +   │    (inventory│
             talossim) │     + import │
                       │     + health)│
                       │              └──► Phase 4 (jobs) ──┬──► Phase 5 (config)
                       │                                    │         │
                       └──► Phase 1b (factory, parallel)    │         ▼
                                     │                      └──► Phase 6 (PROVISIONING)
                                     └──────────────────────────────► │
                                                                      ▼
                                                            Phase 7 (upgrades)
```

| Phase | Delivers | Depends on | Parallelisable with | Testable how |
|-------|----------|------------|--------------------|--------------|
| **0. Skeleton** | Go module; `store` (atomic writes, per-entity locks, `rev` CAS, `VERSION` + migrations, process lock, permission guard); `audit` (JSONL + hash chain); `auth` (argon2id, sessions, sudo-mode, rate limit); `httpapi` middleware chain + problem+json; TLS-by-default; Vite scaffold + `embed.FS` | — | store/audit/auth ∥ frontend scaffold ∥ TLS+config | **Zero Talos.** Pure unit tests, including crash-injection on the store. |
| **1. Transport seam + `talossim`** ⚠️ | `transport.Dialer` + `DiscoverySource` interfaces; `direct` and `fake` impls; `pool` (deadlines, reads-only retry allowlist, breaker, reachability); `NodeClient` / `MaintenanceClient`; **`talossim`**: in-process fake serving real protobufs + real COSI over real mTLS, with scriptable failures | 0 | **1b (factory) has no Talos dependency — build in parallel** | The fake *is* the test rig. Contract-test skeleton lands here. |
| **1b. Image Factory client** | `POST /schematics` → ID; ISO/installer/PXE URL resolution; version-scoped extension catalog; **kernel-args-vs-installer warning** | 0 | ∥ Phase 1 | httptest against recorded Factory responses. |
| **2. Inventory + import + health** | `model.ClusterID`/`MachineID`; UUID-keyed inventory with `schematic_id`; **cluster import** (secrets/talosconfig → probe → enumerate → populate) and create; `HealthLevel`; per-node COSI watch supervisors + heartbeat; degradation; cluster-scoped read routes; dashboard + node detail UI | 1 | backend ∥ frontend once the API shape is agreed | `talossim` with `etcd_down` / `k8s_down` / `ip_changes`. **Tier-1 Docker sandbox lands here** to catch fake drift early. |
| **3. Streaming** | `streamhub` (topics, refcount, ring, non-blocking fan-out, gap markers, sequences); single multiplexed SSE endpoint; `Last-Event-ID` replay; log/dmesg UI with xterm.js | 2 | **∥ Phase 4** — different subsystems | `talossim` `slow_log_consumer` + `flap_connection`; assert gap markers and that one slow subscriber never stalls another. |
| **4. Jobs engine + node actions** | `jobs`: record, `Step{Act, Observe}`, intent→act→confirm, per-cluster lease, restart recovery, step-boundary cancel, progress → `streamhub`; reboot/shutdown/reset with server-side confirm + sudo-mode; audit on every mutation | 2 (3 for progress UI) | ∥ Phase 3 | Kill and restart the process mid-step; assert correct resume-or-park. This is the highest-value test in the project. |
| **5. Config domain** | Generate from imported secrets; versioned patch store; structural merge + render + **structural diff**; validate; dry-run; **required-apply-mode computation**; redaction package + the entropy walk-test | 2 | ∥ Phase 3 | Golden-file tests; no Talos needed for diff/merge/redact. |
| **6. PROVISIONING (Core Value)** | `discovery` (subnet scan + manual, behind `DiscoverySource`); the full state machine as a `JobKind`; three-way reappearance probe; four-layer bootstrap guard; server-driven wizard UI | 1b, 2, 4, 5 | — (this is the convergence point) | `talossim` scripting the whole flow incl. `go_silent`, `ip_changes_on_reboot`, `second_bootstrap`. **Tier-2 QEMU sandbox required before calling this done.** |
| **7. Upgrades** | Rolling Talos upgrade `JobKind`; health-gate steps; installer resolution from `schematic_id`; kernel-args-drift detection; live progress + stop-after-current-node | 4, 2, 1b | ∥ etcd management (members/snapshot) | `talossim` + Tier-1 Docker (Docker cannot do real upgrades — the gate logic is fake-tested, the RPC path is Docker-tested). |
| **8. Hardening + real hardware pass** | Backup/restore commands; audit verify; supported-version-range enforcement; Compose packaging; **one verification pass on real amd64 hardware** | all | — | The only phase that genuinely requires the homelab. |

### The three things this ordering gets right

1. **Phase 1 is `talossim`, and it blocks everything.** It is not a testing chore appended at the end; it is the reason Phases 2–7 can exist on a laptop with no cluster. FEATURES said "sandbox in Phase 1, no exceptions" — the refinement is that the *fake* is Phase 1 (cheap, CI-able, no `talosctl`), Docker is Phase 2 (catches fake drift against real COSI), and QEMU is pre-Phase-6 (the only tier that can exercise maintenance mode → install → join).
2. **Jobs (4) precede provisioning (6).** Provisioning is the hardest job on the engine — the one with a non-observable step and a mandatory manual-resolution state. Building it before the engine means building the engine badly, inside a wizard.
3. **Cluster import is in Phase 2, not Phase 6.** The author's cluster already exists; import is the first path exercised in anger. A design that discovers this in Phase 6 has already written `cluster.Create` as though secrets are always generated locally.

### What can genuinely run in parallel

- Phase 0: `store`/`audit`/`auth` ∥ frontend scaffold ∥ TLS + config loading.
- Phase 1 ∥ **Phase 1b** — the Image Factory client touches no Talos code at all and is a clean, self-contained unit of work.
- Phase 3 ∥ Phase 4 — `streamhub` and `jobs` share only the event channel type; agree that type first, then split.
- Within Phase 2: backend read model ∥ frontend, once the `Field[T]` response shape is fixed.
- Phase 7's etcd management (member list/remove/snapshot, including the no-quorum `cp` fallback) is independent of the upgrade orchestration and can be built alongside it.

---

## Confidence & Sourcing

| Area | Basis | Transport tier | Actual authority |
|------|-------|----------------|------------------|
| `machinery/client` surface (`New`, options, `Client` fields, `COSI state.State`, `WithNode`/`WithNodes`) | pkg.go.dev, read directly | LOW | **HIGH** — canonical generated API reference |
| `pkg/machinery` is a separate Go module | `go.mod` read verbatim from the repo | LOW | **HIGH** — primary source |
| COSI `state.CoreState`, `Event`, `EventType`, `WithBootstrapContents`; `protobuf/server.NewState` | pkg.go.dev | LOW | **HIGH** |
| `machine.RegisterMachineServiceServer` / `RegisterLifecycleServiceServer` exported | pkg.go.dev index | LOW | **HIGH** — this is what makes `talossim` viable |
| `-e` vs `-n` / apid proxy-and-aggregate semantics | Web search, multiple corroborating sources | MEDIUM (verified) | **MEDIUM-HIGH** |
| v1.13 streaming `LifecycleService.Upgrade`, deprecated flags, `extraKernelArgs` bug | Official Talos v1.13 upgrade docs | LOW | **HIGH** — primary vendor docs |
| Image Factory API + which assets honor kernel args | Official Talos v1.13 Image Factory docs | LOW | **HIGH** — primary vendor docs |
| `config/generate` (`NewInput`, options, `Input.Config`) | pkg.go.dev | LOW | **HIGH** |
| `pkg/provision` Provisioner interface, docker/qemu providers, library-usable | pkg.go.dev | LOW | **MEDIUM-HIGH** — API confirmed; "no talosctl needed" is inferred from the exported surface, **not exercised** |
| Docker-vs-QEMU provisioner limitations (no disks / no maintenance mode under Docker) | Web search + Talos QEMU docs | MEDIUM | **MEDIUM** — directionally solid, exact boundaries unverified |
| Package layout, state machine, job engine, hub design, security architecture, build order | Author's synthesis over the above + FEATURES.md | — | **Opinion**, grounded. Argue with it. |

The `classify-confidence` seam scores by transport and returns `LOW` for `webfetch`, since it cannot attest provenance. That understates primary vendor documentation and `pkg.go.dev`, so both dimensions are reported.

**Known gaps, honestly:**

- `cosi-project/runtime/pkg/state/impl/inmem` was **not** directly verified. The `protobuf/server.NewState(CoreState)` wrapper was, so the `talossim` design holds for *any* `CoreState` implementation — worst case, write a small in-memory one. Low risk, but it is an assumption.
- Whether `pkg/provision`'s Docker provider works cleanly on `darwin/arm64` was **not** tested. Budget a spike in Phase 2; if it fails, Tier 1 collapses into Tier 0 and the fake-drift risk rises accordingly.
- The exact `MachineService` streaming RPC signatures (`Logs`, `Dmesg`, `Events`) were not individually enumerated; the streaming design assumes standard server-streaming shapes, which is near-certain but unconfirmed.
- Talos v1.14 was not reviewed. The supported-version-range decision should account for it.
- gRPC keepalive tolerances on `apid` are a stated recommendation (conservative values), not a measured one. Verify against the Tier-1 sandbox before shipping aggressive settings.

## Sources

- `machinery/client` package reference — https://pkg.go.dev/github.com/siderolabs/talos/pkg/machinery/client
- `machinery/api/machine` package reference — https://pkg.go.dev/github.com/siderolabs/talos/pkg/machinery/api/machine
- `machinery/config/generate` package reference — https://pkg.go.dev/github.com/siderolabs/talos/pkg/machinery/config/generate
- `pkg/machinery/go.mod` (module split) — https://raw.githubusercontent.com/siderolabs/talos/main/pkg/machinery/go.mod
- `pkg/provision` package reference — https://pkg.go.dev/github.com/siderolabs/talos/pkg/provision
- COSI `state` package reference — https://pkg.go.dev/github.com/cosi-project/runtime/pkg/state
- COSI `state/protobuf/server` package reference — https://pkg.go.dev/github.com/cosi-project/runtime/pkg/state/protobuf/server
- Talos v1.13 upgrade guide (streaming API, `extraKernelArgs` warning) — https://docs.siderolabs.com/talos/v1.13/configure-your-talos-cluster/lifecycle-management/upgrading-talos.md
- Talos v1.13 Image Factory — https://docs.siderolabs.com/talos/v1.13/learn-more/image-factory.md
- Talos network connectivity / ports (`:50000` apid, `:50001` trustd) — https://docs.siderolabs.com/talos/v1.13/learn-more/talos-network-connectivity.md
- Endpoints vs nodes in talosctl — https://oneuptime.com/blog/post/2026-03-03-understand-endpoints-vs-nodes-in-talosctl/view
- Talos gRPC API architecture — https://oneuptime.com/blog/post/2026-03-03-understand-talos-linux-grpc-api-architecture/view
- Talos controllers and resources (COSI) — https://www.talos.dev/v1.9/learn-more/controllers-resources/
- Talos QEMU local platform — https://www.talos.dev/v1.6/talos-guides/install/local-platforms/qemu/
- Companion research — `.planning/research/FEATURES.md`

---
*Architecture research for: out-of-cluster Talos node & cluster control plane*
*Researched: 2026-08-27*
