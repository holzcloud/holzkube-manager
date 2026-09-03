---
phase: 02-transport-seam-talossim-image-factory
plan: 01
subsystem: infra
tags: [talos, grpc, mtls, machinery, cosi, transport-seam, test-fake, depguard]

# Dependency graph
requires:
  - phase: 01-foundation
    provides: "internal/store's seam shape, internal/tlsx's certificate generation, internal/auth's constructor and clock conventions, internal/depguard_test.go's goList and offendingPackages"
provides:
  - "internal/talos: the transport seam (Dialer, DiscoverySource, Target, Creds, Identity, Candidate, three sentinels)"
  - "internal/talos.ClusterClient: the unmodified machinery client behind the seam, with the D-05 liveness probe at construction"
  - "internal/talos.NewDirectDialer: the direct TCP transport to <addr>:50000, with a credential-free Probe"
  - "internal/talos.NewManualSource: the operator-named discovery source"
  - "internal/talossim: an in-process Talos node -- real gRPC, real mTLS, two listeners, Version and Hostname"
  - "Module pins for pkg/machinery v1.13.9 and cosi-project/runtime v1.14.1, asserted by test"
  - "Module-graph, version-pin, simulator-containment and no-address-above-the-seam guards"
affects: [02-02, 02-03, 02-05, 02-08, provisioning, discovery, sidero-link-retrofit]

actuals:
  tokens: 21892
  tasks: 2
  commits: 3

tech-stack:
  added:
    - github.com/siderolabs/talos/pkg/machinery v1.13.9
    - github.com/cosi-project/runtime v1.14.1 (indirect, arrives with machinery)
    - google.golang.org/grpc v1.81.0
    - google.golang.org/protobuf
  patterns:
    - "Transport seam: identity above, address below. Dialer.Resolve is the single place a Target becomes an address."
    - "Two seams, not one: Dialer for outward contact, DiscoverySource for inward. A tunnel inverts direction, so a dial abstraction alone is insufficient."
    - "The fake implements the seam; the seam never imports the fake. internal/talossim depends on internal/talos and not the reverse."
    - "Executed AST guards over review rules, with a negative control so a guard cannot pass vacuously."
    - "Module boundaries asserted at module-graph level, not only at package level."

key-files:
  created:
    - internal/talos/talos.go
    - internal/talos/client.go
    - internal/talos/dial_direct.go
    - internal/talos/discovery_manual.go
    - internal/talos/talos_test.go
    - internal/talos/seam_test.go
    - internal/talossim/talossim.go
    - internal/talossim/tls.go
    - internal/talossim/machine.go
    - internal/talossim/dialer.go
    - internal/talossim/discovery.go
    - internal/talossim/errors.go
    - internal/talossim/tracer_test.go
  modified:
    - go.mod
    - go.sum
    - internal/depguard_test.go

key-decisions:
  - "talossim serves the same gRPC server on two listeners at once -- a loopback TCP socket and an in-process bufconn pipe -- so that the two Dialers under test are genuinely different transports rather than the same socket reached twice."
  - "The node certificate's ServerName is pinned to the constant \"talossim\" rather than derived from the listen address, so one client TLS config verifies over both transports and the transports stay indistinguishable above the seam."
  - "TLS is configured once via grpc.Creds instead of by wrapping each listener, so exactly one handshake happens per connection on either path and both paths carry the identical RequireAndVerifyClientCert requirement."
  - "directDialer.Probe reads the presented leaf certificate instead of making an RPC, because Dialer.Probe carries no Creds by design: a maintenance-mode node has no cluster PKI, so the probe reports what it observed and leaves the trust decision (fingerprint comparison) to the call site."
  - "Maintenance mode is inferred from a self-signed leaf (issuer == subject), which is the one thing about a node's state that is legible before authenticating to it."
  - "NewClusterClient refuses CredMaintenance and refuses InsecureSkipVerify outright (D-06, T-02-01): the cluster type will not be talked into being the maintenance type by a flag."
  - "cosi-project/runtime stays an indirect requirement -- nothing in this plan imports it -- and the pin is asserted on the resolved version from `go list -m all`, which is true for indirect modules too."

patterns-established:
  - "Seam guard: an executed AST walk that reports its scanned-file count and fails when the count is implausibly low, plus a table-driven negative control that pins the classification."
  - "Simulator containment: a fake that lives inside the product module is admissible only with a test asserting the product binary cannot reach it."
  - "Sentinel errors carry a written retryability rationale, so a later retry allowlist has a source of truth rather than a guess."

requirements-completed: [TRANS-01, TRANS-02, TRANS-06]

coverage:
  - id: D1
    description: "The unmodified machinery client (client.New, no fork, no patch) completes a Version RPC against talossim over a real TLS listener with client-certificate verification -- no hardware, no talosctl, no network egress."
    requirement: TRANS-06
    verification:
      - kind: integration
        ref: "internal/talossim/tracer_test.go#TestTracerRealClientReachesFakeNode"
        status: pass
    human_judgment: false
  - id: D2
    description: "talossim is a real gRPC server behind a real mTLS listener, not an interface stub: a client certificate the server cannot verify is refused at the handshake."
    requirement: TRANS-06
    verification:
      - kind: integration
        ref: "internal/talossim/tracer_test.go#TestTracerRefusesUnverifiableClientCertificate"
        status: pass
    human_judgment: false
  - id: D3
    description: "Server verification is never disabled on the cluster path, and maintenance credentials cannot build a cluster client (T-02-01, D-06)."
    verification:
      - kind: unit
        ref: "internal/talossim/tracer_test.go#TestTracerRefusesMaintenanceCredentialsOnTheClusterPath"
        status: pass
    human_judgment: false
  - id: D4
    description: "Swapping the Dialer implementation (direct TCP vs. talossim in-process) requires no change above the seam: the same ClusterClient construction call works against both."
    requirement: TRANS-02
    verification:
      - kind: integration
        ref: "internal/talos/talos_test.go#TestDialerSwap"
        status: pass
    human_judgment: false
  - id: D5
    description: "Two DiscoverySource implementations feed one fan-in call site, and a source that cannot probe a node until that node dials in reports ErrNotReachableYet as a state rather than as a failure."
    requirement: TRANS-02
    verification:
      - kind: integration
        ref: "internal/talos/talos_test.go#TestDiscoverySourcesShareOneCallSite"
        status: pass
      - kind: unit
        ref: "internal/talos/talos_test.go#TestProbeReportsNotReachableYet"
        status: pass
      - kind: integration
        ref: "internal/talos/talos_test.go#TestProbeReadsIdentityWithoutClusterPKI"
        status: pass
    human_judgment: false
  - id: D6
    description: "go list -deps ./cmd/holzkubed reports no package of the Talos root module and no package of internal/talossim, and go list -m all reports no Talos root module requirement at all."
    verification:
      - kind: integration
        ref: "internal/depguard_test.go#TestBinaryDependencyWeight"
        status: pass
      - kind: integration
        ref: "internal/depguard_test.go#TestModuleGraphExcludesTalosRoot"
        status: pass
      - kind: integration
        ref: "internal/depguard_test.go#TestSimulatorIsNotInTheProduct"
        status: pass
    human_judgment: false
  - id: D7
    description: "The resolved machinery and cosi-project/runtime module versions equal the pinned versions, asserted by a test rather than by reading go.mod."
    verification:
      - kind: integration
        ref: "internal/depguard_test.go#TestPinnedUpstreamVersions"
        status: pass
    human_judgment: false
  - id: D8
    description: "No production package outside internal/talos imports pkg/machinery/client or constructs a node endpoint, asserted by an executed AST guard that reports its scanned-file count."
    verification:
      - kind: unit
        ref: "internal/talos/seam_test.go#TestNoAddressAboveTheSeam"
        status: pass
      - kind: unit
        ref: "internal/talos/seam_test.go#TestSeamGuardRecognisesAViolation"
        status: pass
    human_judgment: false
  - id: D9
    description: "The product still cross-compiles CGO-free for linux/amd64 and linux/arm64 with the new module requirements in the graph."
    verification:
      - kind: other
        ref: "CGO_ENABLED=0 GOOS=linux GOARCH={arm64,amd64} go build ./cmd/holzkubed"
        status: pass
    human_judgment: false
  - id: D10
    description: "Two concurrent ClusterClient calls against two different Targets do not serialise on a shared lock: cancelling one leaves the other running (TRANS-01)."
    requirement: TRANS-01
    verification: []
    human_judgment: true
    rationale: "Authored as `verification: backstop` in the plan. This plan builds no fan-out, so there is nothing here that could serialise; the explicit proof is owned by plan 02-05 (breaker and fan-out). The predicate must abstain rather than pass until then."

# Metrics
duration: 14 min
completed: 2026-08-28
status: complete
---

# Phase 2 Plan 01: Transport Seam and talossim Summary

**The unmodified machinery client completes a Version RPC against an in-process fake Talos node over verified mTLS, driven through a Dialer seam that swaps a real TCP socket for an in-process pipe without one line changing above it.**

## Performance

- **Duration:** 14 min
- **Started:** 2026-08-28T19:40:56Z
- **Completed:** 2026-08-28T19:55:22Z
- **Tasks:** 2
- **Files modified:** 16 (13 created, 3 modified)

## Accomplishments

- `internal/talos` is the transport seam: `Dialer`, `DiscoverySource`, `Target` (identity, with `Addr` demoted to a documented hint), `Creds`, `Identity`, `Candidate`, and three sentinels each carrying a written retryability rationale.
- `talos.ClusterClient` wraps machinery's own `client.New` -- not a fork, not a patch -- and ends construction with an explicit `Version` probe under a 5 s deadline, because `client.New` ignores its context and `grpc.NewClient` is lazy (D-05).
- `internal/talossim` is a real gRPC server behind a real mTLS listener with `RequireAndVerifyClientCert` and an explicit `ClientCAs` pool. It serves the genuine `MachineService` protobuf and answers `Version` and `Hostname`.
- Both seam interfaces have two implementations. `TestDialerSwap` drives one parameterised call site against a loopback TCP socket and an in-process pipe and asserts an identical `Version`; `TestDiscoverySourcesShareOneCallSite` fans an outward-pushing and an inward-pushing source into one channel.
- The module boundary is now asserted at module-graph level as well as package level, the upstream pins are asserted on resolved versions, and the simulator is proven unreachable from the product binary.
- `TestNoAddressAboveTheSeam` walks 50 production source files and fails on any `machinery/client` import or endpoint construction outside `internal/talos`, with a table-driven negative control so it cannot pass vacuously.

## Task Commits

1. **Task 1 (tracer): the real client talks to the fake node** - `cd44350` (feat)
2. **Task 2: prove the seam is a seam** - `4a6f424` (feat)

**Plan metadata:** see the `docs(02-01)` commit that carries this file.

## Files Created/Modified

- `internal/talos/talos.go` - The seam: interfaces, value types, sentinels, and the package doc stating the rule the seam enforces.
- `internal/talos/client.go` - `ClusterClient` over machinery's client, with the D-05 probe and the D-06 / T-02-01 credential refusals.
- `internal/talos/dial_direct.go` - The direct TCP dialer, `ApidPort`, and a `Probe` that works with no cluster PKI.
- `internal/talos/discovery_manual.go` - The operator-named discovery source.
- `internal/talos/talos_test.go` - `TestDialerSwap` and the Probe / DiscoverySource contract tests.
- `internal/talos/seam_test.go` - The AST architecture guard and its negative control.
- `internal/talossim/talossim.go` - `Options`, `Server`, two listeners, the verified-peer record, lifecycle.
- `internal/talossim/tls.go` - A self-contained CA plus node and client leaves; the server and client TLS configurations.
- `internal/talossim/machine.go` - The `MachineService` implementation: `Version` and `Hostname` only.
- `internal/talossim/dialer.go` - `Server.Dialer()`, the in-process `grpc.WithContextDialer`.
- `internal/talossim/discovery.go` - `Server.DiscoverySource()`, the inward-pushing source.
- `internal/talossim/errors.go` - The package's two internal sentinels.
- `internal/talossim/tracer_test.go` - The end-to-end tracer plus the mTLS and credential negative controls.
- `internal/depguard_test.go` - `TestModuleGraphExcludesTalosRoot`, `TestPinnedUpstreamVersions`, `TestSimulatorIsNotInTheProduct`, and the extended negative-control table.
- `go.mod`, `go.sum` - The machinery and COSI pins.

## Decisions Made

- **Two listeners, not one.** `talossim` serves the same gRPC server on a loopback TCP socket *and* on an in-process `bufconn` pipe. Without this the "two dialers" test would be two paths to the same socket, and the swap would prove nothing. The in-process path never touches the network stack.
- **`ServerName` is a pinned constant.** Deriving it from the listen address would make the two transports distinguishable above the seam for a reason that has nothing to do with transport, so the node certificate is issued for `talossim` and both paths verify against that name.
- **One handshake per connection.** TLS is configured on the gRPC server via `grpc.Creds` rather than by wrapping each listener, so both paths carry the identical client-certificate requirement and neither double-handshakes.
- **`Probe` reads a certificate rather than making an RPC.** `Dialer.Probe` carries no `Creds` by design -- it has to work in maintenance mode, where there is no cluster PKI. It therefore does a TCP reach plus a TLS handshake, returns what the node presented, and leaves the trust decision (comparing `Creds.Fingerprint`) to the call site. `InsecureSkipVerify` appears exactly once in the repository, inside that probe, and `NewClusterClient` refuses any credentials carrying it.
- **Maintenance is inferred from a self-signed leaf.** `issuer == subject` is the one fact about a node's state that is legible before authenticating to it.
- **`cosi-project/runtime` stays indirect.** Nothing in this plan imports it; it arrives with machinery. The pin is asserted against `go list -m all`, which reports resolved versions for indirect modules too, so the assertion is not weakened by the requirement being indirect.
- **`STACK.md` was deliberately contradicted**, as the plan instructed: `client.WithNode`/`WithNodes` are context decorators, not `OptionFunc`s, so they appear nowhere in `client.New`'s option list. Nodes are set on the per-call context and this plan needs none.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] golangci-lint was not installed on this host**
- **Found during:** Task 1 (acceptance criterion `golangci-lint run` exits 0)
- **Issue:** The acceptance criteria and the plan-level verification both require `golangci-lint`, and the binary is not on `PATH` on this machine. The criterion could not be evaluated at all.
- **Fix:** Installed the exact version the project's own CI pins (`.github/workflows/ci.yml`: `golangci-lint-action@v9` with `version: v2.13.1`) via `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1`. This is a developer-tool install into `GOPATH/bin`; no project dependency and no repository file changed.
- **Files modified:** none
- **Verification:** `golangci-lint run` reports `0 issues.` against the pinned `.golangci.yml` v2 config.
- **Committed in:** n/a (tooling only)

**2. [Rule 2 - Missing Critical] The AST seam guard had no negative control**
- **Found during:** Task 2 (`TestNoAddressAboveTheSeam`)
- **Issue:** No package outside `internal/talos` violates the rule today, so the guard passes whether its classification is correct or vacuous -- the exact failure mode the repository already recognised for the dependency guard (`TestGuardRecognisesTheRootModule` exists for that reason). A guard that has never been shown to fire is a guard that may not.
- **Fix:** Added `TestSeamGuardRecognisesAViolation`, a table-driven test over synthetic source files that pins each classification: a `machinery/client` import, a `net.JoinHostPort` call and an endpoint `fmt.Sprintf` are flagged; a non-endpoint `fmt.Sprintf` and a clean file are not.
- **Files modified:** `internal/talos/seam_test.go`
- **Verification:** All five table rows pass; deleting any detection branch turns the corresponding row red.
- **Committed in:** `4a6f424`

**3. [Rule 2 - Missing Critical] The mTLS and credential claims had no negative controls**
- **Found during:** Task 1
- **Issue:** The plan's `must_haves` assert that an unverifiable client certificate is refused at the handshake (T-02-06) and that `InsecureSkipVerify` appears on no cluster path (T-02-01), but the task text named only the positive tracer test. A successful RPC is not evidence of mutual TLS: a server that ignored client certificates would answer identically.
- **Fix:** Added `Server.UntrustedClientCreds()` (a client certificate from an unrelated authority, still trusting the node's server certificate, so the handshake fails for exactly one reason) plus `TestTracerRefusesUnverifiableClientCertificate` and `TestTracerRefusesMaintenanceCredentialsOnTheClusterPath`. The simulator also records the CN of every client certificate the TLS stack verified, so the positive test asserts verification happened rather than only that the call succeeded.
- **Files modified:** `internal/talossim/talossim.go`, `internal/talossim/tracer_test.go`, `internal/talos/client.go`
- **Verification:** Both refusal tests fail if `ClientAuth` is relaxed or if the constructor's credential checks are removed.
- **Committed in:** `cd44350`

**4. [Rule 3 - Blocking] The `go` directive moved from 1.26 to 1.26.6**
- **Found during:** Task 1 (`go get github.com/siderolabs/talos/pkg/machinery@v1.13.9`)
- **Issue:** `pkg/machinery` v1.13.9 requires a newer language version than the module declared, and the toolchain raised the directive as part of resolving it.
- **Fix:** Accepted the raise. The pinned `toolchain go1.26.7` already satisfies it and the CI matrix builds with it; nothing else changed.
- **Files modified:** `go.mod`
- **Verification:** `go build ./...`, both cross-compilations and the full race-enabled suite are green.
- **Committed in:** `cd44350`

---

**Total deviations:** 4 auto-fixed (2 blocking, 2 missing critical)
**Impact on plan:** No scope creep. Two were environment blockers with no repository effect beyond a `go.mod` line the toolchain wrote; two added negative controls for claims the plan's own `must_haves` already made but had not asked for a test of.

## Issues Encountered

- **`task` (go-task) is not installed on this host**, so the plan-level verification line `task build` could not be run by name. It was executed as its literal command sequence instead: this plan touches no file under `web/`, so `build:web` had unchanged inputs, and `go build -ldflags "-s -w -X main.version=$(git describe ...)" -o bin/holzkubed ./cmd/holzkubed` succeeded. `./bin/holzkubed --help` starts and prints the option table, so the binary still runs.
- **The direct dialer's `Probe` cannot fill `Identity.Version`.** See Known Stubs.

## Known Stubs

| File | What | Why, and what closes it |
|---|---|---|
| `internal/talos/dial_direct.go` (`directDialer.Probe`) | `Identity.Version` is left empty; only `Machine`, `Hostname` and `Maintenance` are populated. | The version is not carried in the TLS certificate and reading it needs an authenticated `Version` RPC, which `Dialer.Probe` cannot make because the interface carries no `Creds` -- by design, since the probe must work in maintenance mode. Closing this needs either a credential-carrying probe variant or a decision that a probe reports no version. It is a decision, not an omission, and belongs with the discovery work in a later plan. No caller reads the field yet. |
| `internal/talossim/machine.go` | 2 of the 54 `MachineService` RPCs are implemented; the rest inherit `Unimplemented` from the embedded server. | Deliberate and scoped: `<scope_decision>` in the plan moved the full method surface, the COSI state, the streams and `TestMethodCoverage` to plan 02-08 (wave 2). Unimplemented methods answer with a clear gRPC status rather than failing to compile or dereferencing nil. |

## Threat Flags

None. The surface this plan adds -- one outbound gRPC/mTLS channel and one loopback test listener -- is exactly what the plan's `<threat_model>` enumerated. `T-02-01`, `T-02-02`, `T-02-03`, `T-02-06` and `T-02-SC` all have executed mitigations; `T-02-04` was accepted in the plan and remains accepted (the simulator's keys are generated per `New` call, live in process memory only, and are never written to disk).

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- **Plan 02-08 can start from this output alone.** `internal/talossim` compiles, listens on two transports, and serves `Version` and `Hostname`. `internal/talos` now contains `client.go` and `dial_direct.go`, so 02-08's method-coverage guard has real machinery-client call sites to walk -- more than it would have seen had the two plans not been split.
- **Plans 02-02, 02-03 and 02-05 have their attachment points.** `ClusterClient` carries the injected `now` clock the deadline and breaker work needs, and the sentinels carry the retryability rationale the retry allowlist depends on.
- **Open concern (TRANS-01):** the concurrency truth is unproven and abstains rather than passes. There is no fan-out in this plan, so nothing here could serialise; plan 02-05 owns the explicit proof and must not inherit a green mark from this summary.
- **Open concern:** the `Probe`/`Identity.Version` gap above is a contract decision that discovery work will have to make.
- **Note for the release blocker:** TRANS-06 is authored across three plans. This one delivers the real listener, the real mTLS and the seam. The full method surface, the COSI state and the drift guard are 02-08; the scenario engine is 02-03. No part of the row is dropped, but none of the three may be read as completing it alone.

---
*Phase: 02-transport-seam-talossim-image-factory*
*Completed: 2026-08-28*

## Self-Check: PASSED

All 13 created source files exist on disk, the SUMMARY exists, and both task commits (`cd44350`, `4a6f424`) are reachable in `git log`. Every acceptance criterion of both tasks was executed, not asserted: `go build ./...`, both CGO-free cross-compilations, `go test ./... -count=1 -race` (17 packages green), and `golangci-lint run` at 0 issues.
