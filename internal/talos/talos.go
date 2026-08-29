// Package talos defines holzkube's transport seam to Talos machines.
//
// The interface is deliberately identity-shaped and never address-shaped:
// everything above this package speaks Target{Cluster, Machine}, never
// "192.168.1.10:50000". Target.Addr exists as a hint that a Dialer may use, and
// it is never identity -- a node whose address changed on reboot is the same
// node, and a lookup keyed on the address would answer for the wrong one.
//
// Two rules make the seam real rather than aspirational, and both are asserted
// by tests rather than by review:
//
//   - No package outside internal/talos may import
//     github.com/siderolabs/talos/pkg/machinery/client. The machinery client is
//     the implementation of this seam, not a type that travels above it.
//   - Nothing above the seam constructs an address or a grpc.DialOption.
//     Dialer.Resolve maps identity to something dialable and
//     Dialer.DialOptions produces the transport options; a SideroLink tunnel
//     later resolves the same Target to an overlay address with no change to
//     any caller.
//
// TestNoAddressAboveTheSeam in seam_test.go enforces both.
//
// There are two seams here, not one, because a tunnel inverts the direction of
// contact. Today holzkube finds nodes by scanning outward; with SideroLink the
// nodes register inward. Dialer covers "how do I talk to a node I know about";
// DiscoverySource covers "how do I learn a node exists". A design with only the
// first would have to be reopened to retrofit the second.
package talos

import (
	"context"
	"crypto/tls"
	"errors"

	"google.golang.org/grpc"

	"github.com/holzcloud/holzkube/internal/model"
)

var (
	// ErrNotReachableYet reports that a target cannot be contacted yet and that
	// this is a state rather than a failure.
	//
	// It is retryable, and it is the one error above the seam that callers are
	// required to branch on instead of surfacing: a tunnel transport cannot
	// probe a node that has not dialled in yet, and a scan cannot probe a node
	// that is still booting. Reporting either as an error would turn "not yet"
	// into "broken" in the UI, and would make the tunnel retrofit look like a
	// regression.
	ErrNotReachableYet = errors.New("talos: target not reachable yet")

	// ErrNoDeadline and ErrUnclassifiedMethod live in deadline.go, next to the
	// class table they refuse against.

	// ErrUnsupportedInMaintenance is returned for an operation a node in
	// maintenance mode cannot serve. It is not retryable: maintenance mode is a
	// property of the node's current boot, so repeating the call changes
	// nothing until the node is configured.
	ErrUnsupportedInMaintenance = errors.New("talos: operation is not available in maintenance mode")
)

// Target names a machine by stable identity.
//
// Addr is a hint and never identity. It carries the last address a node was
// seen at so a direct Dialer has somewhere to start; it may be stale after a
// DHCP lease change, and a tunnel Dialer ignores it entirely.
type Target struct {
	// Cluster is empty for a machine that is not assigned to a cluster yet,
	// which is every machine in maintenance mode.
	Cluster model.ClusterID

	// Machine is the machine's UUID: the stable key.
	Machine model.MachineID

	// Addr is the last known host, without a port. The port belongs to the
	// Dialer, because it is a property of the transport and not of the node.
	Addr string
}

// CredKind distinguishes the two credential worlds a Talos node lives in. They
// are not interchangeable and no code should branch on a boolean: maintenance
// mode has no cluster PKI at all, and treating its connection as a degraded
// cluster connection is how server verification gets disabled by accident.
type CredKind string

const (
	// CredCluster is full cluster mTLS from the secrets bundle. Server
	// certificate verification is never disabled on this path.
	CredCluster CredKind = "cluster"

	// CredMaintenance is the pre-configuration world: the node serves a
	// self-signed certificate and there is no cluster CA to verify it against,
	// so trust comes from a pinned fingerprint instead.
	CredMaintenance CredKind = "maintenance"
)

// Creds carries the TLS material for one connection.
type Creds struct {
	Kind CredKind

	// TLS is the client configuration handed to the machinery client. It is
	// supplied by the caller so that this package never invents trust.
	TLS *tls.Config

	// Fingerprint optionally pins the server certificate. It is how a
	// maintenance-mode connection is trusted without a CA, and it is formatted
	// like tlsx.Fingerprint: colon-separated upper-case hex.
	Fingerprint string
}

// Identity is what a cheap probe learns about a machine without configuring it.
//
// Machine is holzkube's own identifier and is never the peer's to choose.
// Everything else on this type is read off the certificate the peer presented
// on a connection that verified nothing -- maintenance mode has no cluster CA,
// and Creds.Fingerprint is not yet pinned (T-02-27) -- so it is what something
// answering on the apid port said about itself, and not a fact holzkube has
// checked. Hostname is bounded and character-checked where it is harvested;
// Maintenance is a single bit, which is the whole of what bounds it. Neither
// is a basis for a trust decision until pinning lands.
type Identity struct {
	Machine  model.MachineID
	Hostname string
	Version  string

	// Maintenance reports that the node is running the maintenance-mode API
	// surface rather than the full one. It is the peer's own claim: a
	// self-signed certificate is what an unconfigured node presents, and also
	// what anything at all can present.
	Maintenance bool
}

// Candidate is a machine a DiscoverySource found.
type Candidate struct {
	Target   Target
	Identity Identity

	// Source is the Kind() of the DiscoverySource that produced it, so that a
	// candidate can be traced back to how it was learned about.
	Source string
}

// Dialer is the seam. Everything above it speaks Target; nothing above it
// constructs an address or a grpc.DialOption.
type Dialer interface {
	// Kind names the transport: "direct", "tunnel" or "fake".
	Kind() string

	// Resolve maps a stable identity to something dialable. A direct dialer
	// reads Target.Addr and adds the apid port; a tunnel dialer looks up the
	// overlay address it assigned when the node registered.
	Resolve(ctx context.Context, t Target) (string, error)

	// DialOptions returns the transport-specific gRPC options, which the caller
	// hands to machinery's client.WithGRPCDialOptions. Returning options rather
	// than a connection is what lets the unmodified machinery client be driven
	// over an arbitrary transport, including an in-process one.
	DialOptions(ctx context.Context, t Target, c Creds) ([]grpc.DialOption, error)

	// Probe is a cheap liveness and identity check that must work with
	// CredMaintenance, i.e. with no cluster PKI. It returns an error satisfying
	// errors.Is(err, ErrNotReachableYet) when the target has not been
	// contactable yet, which is a state and not a failure.
	Probe(ctx context.Context, t Target) (Identity, error)
}

// DiscoverySource is the second seam -- the one a dial abstraction alone would
// miss. Scanning pushes outward, a tunnel registration pushes inward, and both
// satisfy this interface.
//
// Run owns the channel's producer side and must return when ctx is cancelled or
// when the source is exhausted. It never closes out: a call site fans several
// sources into one channel, so closing belongs to the fan-in and not to any one
// source.
type DiscoverySource interface {
	// Kind names the source: "subnet-scan", "manual", "tunnel-registration" or
	// "fake".
	Kind() string

	Run(ctx context.Context, out chan<- Candidate) error
}
