# Phase 2: Transport Seam, `talossim` & Image Factory - Context

**Gathered:** 2026-08-28
**Status:** Ready for planning

<domain>
## Phase Boundary

Every Talos interaction runs through a replaceable seam and is testable without
hardware; schematics and image URLs are derived correctly and provably usable.

Two tracks, genuinely independent:

- **(a) Transport seam + pool + `talossim`** — FOUND-12, TRANS-01…TRANS-07
- **(b) Image Factory client** — FACT-01…FACT-06

`talossim` (TRANS-06 🚫) is the phase's only release blocker, and ordering
constraint #1 makes it a correctness dependency for **every phase ≥ 3**, not a
testing chore. It must land in the first wave: TRANS-01…TRANS-05 are only
verifiable against it, and it is also the oracle for FOUND-12's "no mutation
reached a node".

**Not this phase:** no inventory, no cluster import, no health model (Phase 3);
no provisioning (Phase 8); no streaming UI (Phase 5). TRANS-08 (the contract
suite run against BOTH fake and real Talos) belongs to Phase 3 — but its
**fake half is built here**.

</domain>

<research_flag_resolved>
## The roadmap's research flag is closed

ROADMAP flagged Phase 2 with two unverified assumptions. Both were settled
during this discussion **by compilation, not by reading**:

1. **`cosi-project/runtime/.../inmem` exists.** At the machinery-pinned
   v1.14.1: `inmem.Build`, `inmem.NewState`, `inmem.NewStateWithOptions`. The
   canonical wiring `state.WrapCore(namespaced.NewState(inmem.Build))` plus
   `protobuf/server.NewState` + `cosiapi.RegisterStateServer` is exactly what
   the unmodified production client's `Client.COSI` adapter speaks to. The
   fallback of hand-writing a `CoreState` is not needed.

2. **The streaming signatures are enumerated.** `Logs(*LogsRequest,
   grpc.ServerStreamingServer[common.Data])`, `Dmesg(*DmesgRequest,
   grpc.ServerStreamingServer[common.Data])`, `Events(*EventsRequest,
   grpc.ServerStreamingServer[Event])`. Note these are type **aliases** (`=`)
   onto gRPC generics, not defined types — implementing them means implementing
   `grpc.ServerStreamingServer[T]` directly; there is no named interface.

`MachineServiceServer` has **54 methods**, 10 of them streaming (`Logs`,
`Dmesg`, `Events`, `Read`, `Copy`, `List`, `DiskUsage`, `ImageList`,
`EtcdSnapshot`, `PacketCapture`).

A full `talossim` compiles and cross-compiles CGO-free against machinery
v1.13.9 + cosi v1.14.1 **with the Talos root module absent from the graph**.

### Two errors in the research documents

Both would have been hit on day one. Planning must not follow `STACK.md`
verbatim on these points:

- **`STACK.md`'s connect snippet does not compile.** It passes
  `client.WithNodes(ctx, "…")` to `client.New` as an option. `WithNodes` and
  `WithNode` live in `client/context.go` and return `context.Context` — they
  are gRPC-metadata context decorators, **not** `OptionFunc`s. The endpoint /
  node distinction is load-bearing: endpoints are who you dial, nodes are who
  apid proxies to, set per-call on the context.
- **`STACK.md`'s pin pair is not simultaneously satisfiable.** It pins
  machinery v1.13.9 and image-factory v1.5.1 and says "an official Go client
  exists. Do not hand-roll HTTP." Resolving both fails outright: image-factory
  v1.5.1 requires machinery `v1.14.0-rc.2.0.20260825161121-322de8bf2974`. See
  D-01 — this discussion overrules that research ruling.

</research_flag_resolved>

<decisions>
## Implementation Decisions

### Image Factory client

- **D-01:** The Image Factory client is **hand-rolled HTTP** against
  `factory.talos.dev`; the official `siderolabs/image-factory` client is **not**
  used. This deliberately overrules `STACK.md`'s "do not hand-roll HTTP" ruling,
  which was written against a pin combination that does not resolve: every
  image-factory release requires machinery at a `v1.14.0-rc.2` pseudo-version
  and lists the Talos **root** module in its direct requires, so adopting it
  would move holzkube off its stable machinery pin and pull the root module into
  the graph — in a blind spot the current depguard does not cover. The needed
  surface is small: `POST /schematics`, `GET /versions`,
  `GET /version/<v>/extensions/official`, plus URL construction. Copy the
  `pkg/schematic` struct rather than importing it. This is also the only option
  under which the roadmap's own claim that track (b) has "null
  Talos-Abhängigkeit" is literally true. — **Reversibility:** reversible —
  swapping to the official client later is a package-local change, and becomes
  attractive once a stable machinery v1.14.0 ships and image-factory pins a
  release rather than an RC.

- **D-02:** URL construction is **derived per version, never hardcoded**, and
  the architecture is a parameter. The installer repository name is
  version-dependent — Talos v1.13.9 builds
  `factory.talos.dev/metal-installer/<schematic>:<version>`, older versions use
  `installer` — so the resolved name must be verified against the OCI manifest
  rather than assumed. ISO is
  `/image/<schematic>/<version>/metal-<arch>.iso`, PXE is
  `/pxe/<schematic>/<version>/metal-<arch>`, disk image is
  `/image/<schematic>/<version>/metal-<arch>.raw.zst`; secureboot adds a
  `-secureboot` suffix. — **Reversibility:** reversible.

### Dry-run (FOUND-12)

- **D-03:** `--dry-run` is enforced **in the transport**, at the last layer
  before the wire: every mutating RPC returns a dry-run refusal, with a test
  that proves no other call site can reach the network. "No mutation reaches a
  node" is a statement about the node, so it has to be provable there — an HTTP
  middleware check only sees HTTP requests and would miss anything Phase 6's
  jobs engine drives. Additionally a **UI banner** makes the mode visible, so
  dry-run is not merely true but obvious to the operator during an incident.
  The `Destructive` route flag (D-06) is explicitly **not** the mechanism:
  destructive ≠ mutating. — **Reversibility:** costly — moving the enforcement
  point later touches every Talos call site and invalidates the test that is the
  requirement's only proof.

### Deadlines and retries (TRANS-04)

- **D-04:** Every node call carries a deadline **structurally** — the wrapper
  refuses to issue a call without one; there is no default-less path. Deadline
  classes and the retry allowlist are specified in `<deadline_policy>` below.
  Retries never extend a call's deadline: the budget covers all attempts. —
  **Reversibility:** reversible — the values are constants; the *structure*
  (no call without a deadline) is the part that must not be relaxed.

- **D-05:** Constructing a client is **not** a connectivity assertion.
  `client.New` ignores its context and `grpc.NewClient` is lazy, so no TCP
  connect, TLS handshake or certificate validation happens until the first RPC.
  Liveness is proven by an explicit **`Version` probe** under the standard
  deadline — it is the one RPC available in both cluster and maintenance mode,
  and it exercises the full TLS handshake, so it validates the certificate path
  too. — **Reversibility:** reversible.

### Client type separation (TRANS-03)

- **D-06:** Cluster and maintenance clients are **two distinct wrapper types
  with non-overlapping method sets**, not one type carrying a mode flag.
  machinery provides no such split — both paths produce the same
  `*client.Client` and nothing on the value records its mode — so the separation
  is built entirely in holzkube. Maintenance mode realistically exposes only
  `ApplyConfiguration`, `Version`, `Disks` and COSI reads, so that type stays
  small and a **compile error**, not a runtime rejection, is what stops a
  cluster-only call against a maintenance node. That is precisely what success
  criterion 3 ("nicht verwechselbar") is asking for. — **Reversibility:**
  costly — collapsing the types later touches every call site.

### Claude's Discretion

The operator expressed no preference on the four areas below, so they are
decided here with their rationale. Each is recorded so it can be objected to
rather than discovered later.

- **D-07:** `talossim` lives in **`internal/talossim`** (the research's intent),
  and the same plan that creates it **extends `internal/depguard_test.go`** with
  two new assertions: a module-graph guard (`go list -m all` must never contain
  `github.com/siderolabs/talos`) and a version-pin assertion on the resolved
  machinery and cosi versions. The existing guard only walks
  `go list -deps ./cmd/holzkubed`, which is package-level: a test-only package
  under `internal/` can add the heavy module to `go.mod` and to every
  `go test ./...` compile while the guard stays green. The guard's own doc
  comment makes this argument — "a guard added after the mistake has to be paid
  for with a refactor rather than with a red test" — so it is closed *before*
  the first Talos import lands, not after. — **Reversibility:** costly — moving
  `talossim` to a separate module later also moves the production client's test
  imports across a module boundary.

- **D-08:** FACT-05's "bekannt kaputte Versionen" come from a **curated list
  embedded in the binary** (reviewable in git, shipped with the release), not
  from an upstream feed and not from operator-editable UI state. No
  machine-readable upstream source exists; a UI-editable list would strand
  operational knowledge inside one installation. Pre-release filtering needs no
  data at all — it is structural, from the semver prerelease component. —
  **Reversibility:** reversible.

- **D-09:** Schematics are persisted through the **store's entity seam** — a
  `Schematics()` accessor on `store.Store` following the existing pattern — not
  through ad-hoc file writes. `internal/store/store.go` states the rule
  directly: any `os.ReadFile` outside `internal/store/fsstore` is an
  architecture bug. This is the first of several entity types; machines and
  clusters follow in Phase 3, and `model.ClusterID` / `model.MachineID` already
  exist with no users waiting for exactly this. — **Reversibility:** costly —
  the store schema version is bumped, so a later reshape needs a migration.

- **D-10:** The **audit actor vocabulary for non-HTTP mutations is fixed now**
  and documented in `docs/api-contract.md` alongside the existing actor
  semantics: `system` for a process-initiated mutation, `job:<id>` reserved for
  Phase 6's jobs engine. The actor enrichment stays in the HTTP adapter for now
  — promoting it to a shared exported type is Phase 6's job. Deciding the
  vocabulary while there is still exactly one writer is far cheaper than
  reconciling two conventions later, and D-16 (unlimited retention, no deletion
  path) means whatever Phase 2 writes is in the archive permanently. —
  **Reversibility:** one-way — the field format is hashed into the audit chain;
  changing it later forces either a chain break at the seam or a rewrite
  migration across the whole archive.

</decisions>

<deadline_policy>
## Deadline classes and retry allowlist — CONFIRMED (2026-08-29)

**Status: confirmed by the operator at plan 02-05's task-1 checkpoint
(`option-a`).** The proposal below stands exactly as written — every deadline
class, every class membership, the retry allowlist and the backoff policy. No
value and no membership changed, which is why plan 02-03's `ExpectedClient`
strings in `internal/talossim/scenario.go` and the matching rows in
`docs/talossim.md` are byte-identical to what they were before the confirmation
and must stay that way: `TestDocumentedScenariosMatchRegistry` compares row
sets, not prose, so a gratuitous rewording of an unchanged decision would pass
silently while desynchronising the two surfaces.

This section is closed. A later plan that wants to change a deadline value or a
class membership is changing a confirmed decision, and must amend
`internal/talossim/scenario.go`, `docs/talossim.md` and
`internal/talos/deadline.go` in the same commit.

The operator asked to be shown a proposal rather than decide these from scratch.
Derived from the real 54-method `MachineServiceServer` surface at machinery
v1.13.9, not invented.

### Deadline classes

| Class | Deadline | RPCs |
|---|---|---|
| **Probe** | 5 s | `Version` used as the liveness check (D-05) |
| **Fast read** | 10 s | `Version`, `Hostname`, `ServiceList`, `Memory`, `LoadAvg`, `SystemStat`, `CPUInfo`, `CPUFreqStats`, `DiskStats`, `NetworkDeviceStats`, `Mounts`, `Netstat`, `Processes`, `Stats`, `Containers`, `EtcdMemberList`, `EtcdStatus`, `EtcdAlarmList`, COSI Get/List |
| **Mutation** | 30 s to *initiate* | `ApplyConfiguration`, `Bootstrap`, `Reset`, `Reboot`, `Shutdown`, `Upgrade`, `Rollback`, `MetaWrite`, `MetaDelete`, `ServiceStart/Stop/Restart`, `ImagePull`, `EtcdLeaveCluster`, `EtcdRemoveMemberByID`, `EtcdForfeitLeadership`, `EtcdDefragment`, `EtcdRecover`, `EtcdDowngrade*` — the long work is asynchronous on the node; this bounds the call, not the operation |
| **Stream** | no total deadline | `Logs`, `Dmesg`, `Events`, `Read`, `Copy`, `List`, `DiskUsage`, `ImageList`, `EtcdSnapshot`, `PacketCapture` — bounded instead by a **first-byte deadline of 10 s** and an **idle timeout of 60 s** without data |

### Retry allowlist

Retried, and only on transport-level `Unavailable` / connection failure:
every RPC in the **Fast read** class above.

**Never retried**, and the exclusions are deliberate:
- every **Mutation** — not idempotent; `Bootstrap` retried is exactly the
  `second_bootstrap_returns_AlreadyExists` scenario TRANS-07 names
- every **Stream** — a retry restarts a partially consumed stream, silently
  duplicating or dropping data
- `EtcdSnapshot`, `PacketCapture`, `DiskUsage` — reads, but expensive enough
  that a retry storm is its own outage

Policy: at most **2 retries** (3 attempts), exponential backoff 200 ms → 800 ms
with full jitter, all attempts inside the original call deadline.

### One conflict this creates — now a Phase 5 ENTRY BLOCKER

`cmd/holzkubed/main.go:42` sets `WriteTimeout = 60 s` process-wide, and its
comment scopes the reasoning to argon2id plus the login rate limiter — nothing
about upstream calls. Mutations at 30 s fit. **Streams do not fit at all**, and
none of the three `ResponseWriter` wrappers in the chain implements `Flush`,
`Unwrap` or `Hijack`, so a streaming endpoint on this chain silently buffers.
Phase 5 needs this resolved; Phase 2 should not paper over it.

The `option-a` confirmation makes this explicit rather than merely deferred:
the stream deadline policy (no total deadline, a 10 s first-byte deadline, a
60 s idle timeout) is implemented at the transport layer in plan 02-05 and is
testable there against `slow_log_consumer`, and **Phase 2 deliberately exposes
no streaming HTTP route**, so the collision cannot bite inside this phase. It is
recorded as a **Phase 5 entry blocker** in `.planning/ROADMAP.md` § *Phase 5*
and in `STATE.md`'s blockers, because a Phase 5 plan that adds an SSE route
before reading this section would ship a stream that silently buffers and dies
at 60 s. The two things Phase 5 must do before its first streaming route:

1. Implement `Flush`, `Unwrap` and `Hijack` (or the `http.ResponseController`
   route via `Unwrap`) on all three `ResponseWriter` wrappers in the middleware
   chain.
2. Scope `WriteTimeout` so it does not apply to streaming routes — per-route,
   or via `http.ResponseController.SetWriteDeadline` on the streaming handler.

</deadline_policy>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase scope and requirements
- `.planning/ROADMAP.md` § *Phase 2* — goal, 5 success criteria, 2 parallel tracks, ordering constraint #1
- `.planning/REQUIREMENTS.md` — FOUND-12, TRANS-01…TRANS-07, FACT-01…FACT-06 verbatim
- `.planning/phases/01-foundation-skeleton/01-CONTEXT.md` — D-01…D-16, inherited and not to be re-litigated

### Research — read WITH the corrections in `<research_flag_resolved>`
- `.planning/research/STACK.md` — machinery pin, maintenance-mode connect. **Its connect snippet does not compile and its image-factory ruling is overruled by D-01.**
- `.planning/research/SUMMARY.md` §§ transport seam, talossim, image factory
- `.planning/research/PITFALLS.md` — the factory `200`-for-bogus-extension trap behind FACT-02
- `.planning/research/ARCHITECTURE.md` — transport surface

### Existing contracts this phase must not break
- `docs/api-contract.md` — RFC 9457 taxonomy (declared **closed**), CSRF rules, the "add a Routes function, never touch router.go" rule. A transport failure code must be minted deliberately here, in the same commit.
- `internal/store/store.go:5-7` — any `os.ReadFile` outside `internal/store/fsstore` is an architecture bug (drives D-09)
- `internal/depguard_test.go` — module boundary; extended by D-07
- `sandbox/README.md` — what belongs in the separate module

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable assets
- **Composition root is additive.** `cmd/holzkubed/main.go` builds one
  `httpapi.Deps`, then `slices.Concat`s per-package `…Routes(deps)` functions.
  A transport wires in as a constructor call in `run()` **before** the
  `slices.Concat` line, plus one `Deps` field and one `Routes` entry.
- **`model.ClusterID` / `model.MachineID`** exist with zero users, and
  `audit.Record` already reserves the matching columns.
- **`internal/tlsx`** already generates and manages certificates — the shape a
  `talossim` mTLS setup needs.

### Established patterns that constrain this phase
- **D-06 (Phase 1): destructive routes are marked declaratively** at the route,
  read by middleware. Binding for all later phases. Note D-03 above: this is
  *not* the dry-run mechanism.
- **RFC 9457 taxonomy is closed.** No transport failure code exists yet. If one
  is not minted deliberately, every unreachable node becomes
  `internal.unexpected`, which by design carries no detail — and that code lands
  in the permanent audit archive.
- **Audit is fail-closed and fsyncs per record under one global mutex**, sized
  on the assumption of "a handful per minute". A per-node fan-out of mutations
  invalidates that assumption, and contention becomes *mutation failure*, not
  just latency.
- **`internal/audit/redact.go`'s allowlist is one shared table** every plan
  adding an action token must edit — a guaranteed merge point across a phase
  planned as two parallel tracks. A forgotten entry is silent: the record is
  written with every param reading `<redacted>`, permanently.

### Integration points
- `httpapi.Deps` — new transport field; note `Deps` is copied **by value** at
  `main.go:156`, so a field assigned after that line is the zero value inside
  every handler closure, with no compile error.
- `docs/api-contract.md` — new routes and the new problem code.
- `internal/store` — schema bump for D-09.

</code_context>

<specifics>
## Specific Ideas

- The nine TRANS-07 scenarios are enumerated and closed; what is **not**
  specified anywhere is the *client's defined behaviour* per scenario, which
  success criterion 2 demands. Planning owes an expected-behaviour table
  alongside the injection mechanism.
- `Client.EventsWatch` silently drops undecodable events and returns **nil**
  (not an error) on `io.EOF`, `codes.Canceled` and `codes.DeadlineExceeded`.
  `go_silent(90s)` and `flap_connection` will therefore look like clean success
  to a caller that only checks the error — build the fault-injection assertions
  on something else.
- `client.WithNodes` (plural) makes apid **aggregate** several nodes into one
  reply, with per-node identity only in each message's metadata. `WithNode`
  (singular) is the safer default for per-node inventory.
- `talossim` should embed `UnimplementedMachineServiceServer` (54 methods) plus
  a test enumerating the methods holzkube's client actually calls, failing if
  any is unimplemented — small sim, no silent drift.

</specifics>

<deferred>
## Deferred Ideas

- **Promoting the audit actor adapter to a shared exported type** — Phase 6,
  when the jobs engine becomes the second writer. Only the vocabulary is fixed
  now (D-10).
- **`WriteTimeout` / streaming redesign** — Phase 5, and since the `option-a`
  confirmation it is a Phase 5 **entry blocker** rather than a loose deferral.
  Flagged here because Phase 2 sets the deadline policy that collides with it.
  See `<deadline_policy>` § *One conflict this creates* for the two concrete
  things that must land before Phase 5's first streaming route.
- **Official image-factory client** — revisit when a stable machinery v1.14.0
  ships and image-factory pins a release rather than an RC (D-01).
- **TRANS-08, the contract suite run against real Talos** — Phase 3 by the
  roadmap, but its fake half is built here.
- **Tier-1 Docker provisioner** — Phase 3, and it belongs in `sandbox/` because
  it needs the Talos root module (D-07).

</deferred>

---

*Phase: 2-transport-seam-talossim-image-factory*
*Context gathered: 2026-08-28*
