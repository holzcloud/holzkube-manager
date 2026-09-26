# `talossim` — the in-process Talos node

`internal/talossim` is not an interface stub. It is a real gRPC server serving
the real `MachineService`, `StorageService` and COSI `State` protobufs from
`pkg/machinery`, behind a real TLS listener that requires and verifies a client
certificate. The unmodified production client — machinery's own `client.New`,
reached through holzkube-manager's transport seam — drives it. A test that passes
against `talossim` has exercised the wire, the protobuf and the handshake.

It exists so that every phase from three onward can be built and tested without
hardware, and so that the failure modes hardware produces once a quarter can be
produced on demand, in a second, in a test.

## The two rules this fake lives by

**1. Missing coverage is a test failure, never a zero value.** The simulator
embeds `machine.UnimplementedMachineServiceServer`, so each of the 54 RPCs
holzkube-manager does not reach answers `codes.Unimplemented` rather than a plausible
empty success. Which methods are implemented is not a judgement call that can
drift: `TestMethodCoverage` walks `internal/talos` for machinery-client call
sites and fails when one of them lands on an inherited method.

**2. The simulator is never easier to satisfy than the hardware.** A second
`Bootstrap` is refused. An un-bootstrapped node has no etcd member list — and
answers `FailedPrecondition` rather than an empty list, because an empty list is
a cluster with no members, which is a different fact. A powered-off node answers
`Unavailable`. Every scenario below reproduces the documented failure shape —
the gRPC status code, the timing, the connection behaviour — rather than a
convenient approximation that would later let real hardware surprise the
product.

## The `EventsWatch` hazard, and why no assertion here checks only `err`

`client.EventsWatch` silently drops events it cannot decode, and it returns
**`nil` — not an error —** on `io.EOF`, `codes.Canceled` and
`codes.DeadlineExceeded`.

A node that has gone silent, and a listener that is flapping, therefore look
like *clean success* to a caller that only checks the error return. That is not
a quirk to work around; it is the single most important fact about writing
assertions against this simulator. Every scenario below is consequently provable
by an observation other than a bare error: a timing bound, a counter the
simulator keeps (`Server.Calls`, `Server.ListenerTransitions`,
`Streamer.BlockedSends`), an address the simulator recorded
(`Server.AddressHistory`), or the connection state itself. A test that asserts
`err != nil` and stops has asserted nothing.

## Injecting a scenario

```go
sim, err := talossim.New(talossim.Options{Hostname: "node-1"})
restore, err := sim.Inject(talossim.Scenario{
    Name:     talossim.ScenarioGoSilent,
    Duration: 90 * time.Second,
})
defer restore()
```

Injection mutates a node that is **already running**: no restart, no re-listen,
no client reconnect. A test that has to rebuild the simulator and redial to see
a fault has proven nothing about a fault appearing mid-session, which is the
only way faults appear on hardware. The returned function restores the state
the node was in before, so scenarios compose across a test suite instead of
leaking into the next test. `Server.Active()` reports what is currently injected.

`Inject` **refuses a name that is not in the registry**. An unregistered
scenario is a missing specification, not a fault that quietly does nothing.

## The scenario catalogue (TRANS-07)

The right-hand column is normative. ROADMAP success criterion 2 is not "the sim
can fail" but *"der Client verhält sich in jedem Fall **definiert** statt
zufällig"* — and a defined behaviour written nowhere cannot be asserted. These
are the definitions; plan 02-05 asserts the production client against them. The
deadline classes and the retry allowlist they refer to are specified in
`02-CONTEXT.md` `<deadline_policy>`.

This table and `talossim.Registry` are asserted equal by
`TestDocumentedScenariosMatchRegistry`, so the document cannot rot into a lie.

<!-- scenario-table:begin -->

| Scenario | What the sim does | Defined client behaviour |
|---|---|---|
| `go_silent` | accepts the TCP connection and completes the TLS handshake, then writes no response byte for the configured duration (default 90 s) | every non-stream call fails at **its own class deadline** (probe 5 s, fast read 10 s, mutation 30 s) and never at 90 s; a concurrent call to a second target is unaffected; after the configured consecutive-failure count the per-node breaker opens |
| `reject_apply` | `ApplyConfiguration` returns `codes.InvalidArgument` with a message | the call is **not retried** — it is in the mutation class — and surfaces as a distinguishable typed failure carrying the upstream message; the breaker records no failure, because an answered call is not a transport failure |
| `second_bootstrap_returns_AlreadyExists` | the first `Bootstrap` succeeds; every later one returns `codes.AlreadyExists`. Injection performs that first `Bootstrap`, so the node is already bootstrapped when the client arrives and the client's own first call is a second bootstrap | the client surfaces `AlreadyExists` as its own outcome, never retries it, and never reports success; this is the exact case the retry allowlist exists to prevent |
| `flap_connection` | closes and reopens the listener on a configurable cycle | a fast-read call retries at most twice (200 ms → 800 ms, full jitter), inside the **original** deadline, and succeeds if a window opens; a mutation fails on the first `Unavailable`; a stream is never restarted |
| `slow_log_consumer` | `Logs` produces faster than the consumer reads until the send blocks | the client does not deadlock; the caller's context cancellation tears the stream down within 1 s; the 60 s idle timeout applies to *no data flowing*, not to a blocked send |
| `ip_changes_on_reboot` | after a simulated reboot the sim listens on a new address and the old one refuses connections | `Dialer.Resolve` is consulted again on the next call, because `Target.Addr` is a hint and `MachineID` is identity; a stale address yields `Unavailable`, never an answer attributed to the wrong node |
| `etcd_down` | every `Etcd*` RPC returns `codes.Unavailable`; `Version` and `Hostname` still answer | node-level calls keep working; etcd calls fail distinguishably; the breaker does **not** open, because the node is reachable and only one subsystem is not |
| `k8s_down` | the k8s-related COSI resources are absent and the k8s service reports not-running in `ServiceList` | a COSI read for a k8s resource returns a not-found-shaped result rather than an error; the node stays reachable and the failure is attributable to Kubernetes, not to the transport |
| `version_out_of_supported_range` | `Version` reports a Talos version outside the supported range v1.12–v1.14 | `NewClusterClient` refuses with a named error identifying the observed version and the supported range, rather than proceeding against an untested API surface |

<!-- scenario-table:end -->

## Scenario parameters

`talossim.Scenario` is one struct for all nine, so an unused field is not a
trap: a field left at its zero value is filled from
`Registry[Name].Defaults`, and a field a given scenario ignores is documented as
ignored.

| Field | Read by | Default |
|---|---|---|
| `Duration` | `go_silent` only | 90 s |
| `Cycle` | `flap_connection` only | 200 ms |
| `Version` | `version_out_of_supported_range` only | `v1.15.0` |

## The completeness gate

`TestEveryScenarioIsImplemented` iterates the registry, injects each scenario
against a fresh node, exercises the RPC or the connection its *What the sim
does* column names, and fails when the node's behaviour is **indistinguishable
from the un-injected baseline**.

A scenario that is registered, documented and inert is the worst outcome
available here: it would make plan 02-05's contract suite pass against nothing.
Asserting "no panic" would not catch it. Comparing against a baseline does.

## Simulated clusters (phase 3)

`talossim.NewCluster(name, endpoint)` generates a real secrets bundle with
machinery's own generator and derives from it the two machine configurations a
node of each role serves, plus an admin talosconfig.

A node built with `Options.Cluster` set issues its certificates from **that
cluster's Talos OS certificate authority** rather than from a fresh one. That is
what makes the adoption path testable end to end: holzkube-manager derives the bundle
from the configuration it read, mints itself a client certificate from that CA,
and reconnects. Against a node with an unrelated authority, the second
connection would fail for a reason that has nothing to do with whether the
derivation was correct — and the connectivity proof would be untestable.

`Options.ControlPlane` selects which of the two configurations the node serves.
The difference is the one D-05 turns on: only a control-plane node's
configuration carries the OS CA *private key*, and an import aimed at a worker
must be refused by name rather than half-completed.

### What `k8s_down` removes

Every resource `seedKubernetes` creates, so that the scenario and its restore
stay each other's exact inverse: `k8s.Nodename` and `k8s.KubeletSpec`. The
kubelet spec joined the list in phase 3 because that is where a node's
Kubernetes version is read from — leaving it behind would mean a node with
Kubernetes down still confidently reporting a Kubernetes version.

## Simulated hardware

The hardware view reads counters and sensors live, so the node has them
(`hardware.go`):

- **Counters that advance.** `SystemStat`'s CPU times, `DiskStats` and
  `NetworkDeviceStats` are each a constant rate times the time since the node's
  last boot. They move between two reads, restart from zero on a reboot, and
  what a view computes from them is known in advance: `SimulatedThreadBusy`,
  `SimulatedIowait`, `SimulatedDiskReadBytesPerSec` and the rest are exported
  so a test asserts the rate the node was built to produce. The partitions, the
  loop device, loopback and a pod's veth carry traffic too, because a view that
  summed everything the kernel counts can only be caught against a node that
  has them.
- **`Memory`, `LoadAvg`, `Mounts`** in the shape Talos reports them —
  EPHEMERAL mounted at `/var` and again at every bind mount made from it.
- **A read-only `/sys`** served by `List` and `Read`: `/sys/class` entries are
  symlinks into `/sys/devices`, chip attributes sit beside a `device` link, an
  NVMe model is space-padded as the controller reports it, and a missing file
  is refused with the error Talos returns (`codes.Unknown`).
  `Options.Sensors` picks the tree: `SensorsDesktop` (coretemp, an nvme chip
  per NVMe disk — each three degrees warmer than the last — and an nct6798
  with one fan at 0 rpm), `SensorsNone`, or `SensorsThermalZoneOnly` (a CPU
  temperature that exists only as a thermal zone).
  `Options.ListHidesSymlinks` makes `List` blind to links, which is how the
  product's fallback of probing file names is exercised.
