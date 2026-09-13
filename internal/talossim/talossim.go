// Package talossim is an in-process Talos node.
//
// It is not an interface stub. It is a real gRPC server serving the real
// MachineService protobuf from pkg/machinery, behind a real TLS listener that
// requires and verifies a client certificate. A test that passes against it has
// exercised the wire, the protobuf and the handshake -- which is what makes it
// usable as the substitute for hardware that every later phase depends on.
//
// It serves on two listeners at once, deliberately:
//
//   - a loopback TCP socket, which the production direct dialer reaches exactly
//     as it would reach a real machine, and
//   - an in-process pipe, which never touches the network stack.
//
// The same client code above the transport seam drives both. That is the proof
// that the seam is a seam rather than a comment.
//
// This package must never appear in the product's dependency graph: a fake
// Talos node answering inside a real deployment is indistinguishable from a
// real one. TestSimulatorIsNotInTheProduct enforces that. For the same reason
// the package does not import testing -- it is an ordinary package, and a
// testing import would put the test framework into any binary that ever links
// it.
package talossim

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/test/bufconn"

	"github.com/cosi-project/runtime/pkg/state"

	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// DefaultTalosVersion is the version the simulated node reports when Options
// does not say otherwise. It matches the pinned pkg/machinery version, so the
// default case is the case the client library was built for.
const DefaultTalosVersion = "v1.13.9"

// bufSize is the in-process listener's buffer. It only has to hold one gRPC
// frame at a time; the connection is synchronous beyond that.
const bufSize = 1 << 20

// Options configures a simulated node.
type Options struct {
	// NodeIP is the address the node claims to have. It is a SAN on the node's
	// certificate and the value a discovery source reports; it does not have to
	// be an address anything actually listens on.
	NodeIP string

	// Hostname is what the Hostname RPC answers.
	Hostname string

	// TalosVersion is what the Version RPC answers. Defaults to
	// DefaultTalosVersion.
	TalosVersion string

	// KernelArgs are extra kernel arguments this node booted with, on top of
	// the ones Talos sets for itself. They are what UPG-04's drift check reads
	// on the node's side: a node whose bootloader carries an argument its
	// configuration does not is the case the check exists for, and setting one
	// here is how a test produces it.
	KernelArgs []string

	// Maintenance makes the node report itself as running the maintenance-mode
	// API surface.
	Maintenance bool

	// StreamMessages is how many messages Logs, Dmesg and Events each emit
	// before completing. Defaults to DefaultStreamMessages. It is bounded on
	// purpose -- see Streamer.
	StreamMessages int

	// StreamChunk pads each streamed message to this many bytes.
	//
	// A real node's log is volume: kilobytes per second, megabytes over a
	// boot. The default message here is a single short line, which is right
	// for a test that counts messages and wrong for one that has to cross a
	// size threshold -- and a test that reached 256 KiB by asking for nine
	// thousand short messages would be a test measuring an unbuffered channel
	// rather than the thing it means to measure. Zero leaves the message as
	// it is.
	StreamChunk int

	// Cluster, when set, makes this node a member of a simulated cluster: it
	// serves that cluster's machine configuration, and its certificate
	// authority is the cluster's own OS CA rather than a fresh one. A node
	// without it is a node holzkube-manager can talk to but cannot adopt, which is the
	// right shape for every test that is not about adoption.
	Cluster *Cluster

	// ControlPlane selects which of the cluster's two configurations this node
	// serves. It is the difference D-05 turns on: only a control-plane node's
	// configuration carries the OS CA private key, and an import aimed at a
	// worker must be refused by name rather than half-completed.
	ControlPlane bool

	// Bootstrapped starts the node with etcd already running.
	//
	// It exists because the default is the truthful simulation of a *fresh*
	// node -- one the provisioning path has just installed and not yet
	// bootstrapped -- and that is not the node the adoption path meets. An
	// imported cluster is by definition already running, so a test that
	// imports one and then asks it anything about Kubernetes or etcd is
	// testing against a node in a state no adopted node is ever in.
	//
	// It sets only the flag, not a bootstrap call: BootstrapCalls stays at
	// zero, so a test can still tell a node that came up this way from one
	// this product bootstrapped.
	Bootstrapped bool

	// Members is the cluster membership this node's discovery reports. Leaving
	// it empty is the truthful simulation of a cluster whose discovery service
	// is switched off -- which is a supported configuration and must not look
	// like a broken one.
	Members []MemberFixture

	// SchematicID is served as the virtual schematic system extension. Empty
	// means the node was not installed from a Factory image and reports no
	// schematic, which is a fact holzkube-manager has to be able to show as "none"
	// rather than guess at (D-12).
	SchematicID string

	// KubeletImage is what the KubeletSpec resource carries, and therefore
	// where the node's Kubernetes version comes from. Defaults to the pinned
	// DefaultKubernetesVersion.
	KubeletImage string

	// The hardware the node claims. The defaults describe a small mini-PC,
	// because that is the hardware this product was written for.
	CPUs      int
	MemoryMiB uint64
	Disks     []DiskFixture

	// Now is the clock the node stamps its state with. It is a field rather
	// than a call to time.Now for the same reason auth.Service carries one: a
	// test that has to assert "the service last changed at the last boot"
	// should be able to say what time that was.
	Now func() time.Time
}

// Server is a running simulated node.
type Server struct {
	opts Options

	// node is the mutable state of the simulated machine: what it has been
	// bootstrapped into, what version it runs, when it last booted. It lives
	// on the server rather than inside a handler closure because a scenario
	// mutates it from outside a call.
	node *nodeState

	// cosi is the node's resource state. It is the read surface the product
	// uses, served over the same gRPC server as MachineService because that is
	// the one connection the production client's COSI adapter dials.
	cosi state.State

	// streams is the per-server emitter behind Logs, Dmesg and Events.
	streams *Streamer

	pki *pki

	// The phase-9 node states: an etcd alarm, a lost quorum and an installer
	// that refuses. They are guarded by the Server's own mutex and are set
	// directly rather than through Registry -- see the note in upgrade.go on
	// why they are not scenarios.
	etcdAlarmType  string
	etcdNoQuorum   bool
	upgradeRefuses bool
	srv            *grpc.Server

	pipe *bufconn.Listener

	// done is closed by Close, before the gRPC server is stopped. It is what
	// releases a handler parked by the go_silent scenario: grpc.Server.Stop
	// blocks until every pending RPC returns, so a blocking scenario without a
	// shutdown signal would deadlock Close and hang the whole test binary
	// (T-02-15).
	done chan struct{}

	// wg tracks the Serve goroutines; sg tracks the goroutines a scenario owns,
	// which have to be gone before the server is stopped so that none of them
	// can re-listen behind Close's back (T-02-16).
	wg sync.WaitGroup
	sg sync.WaitGroup

	// lmu guards the TCP listener and its history. It is separate from mu
	// because flap_connection closes and reopens the listener from its own
	// goroutine while calls guarded by mu are in flight, and the two locks are
	// never held at the same time.
	lmu         sync.Mutex
	tcp         net.Listener
	tcpAddr     string
	transitions int
	addrHistory []string

	mu sync.Mutex
	// verified records the common name of every client certificate the TLS
	// stack verified against ClientCAs. It is what lets a test assert that mTLS
	// actually happened rather than that a request merely succeeded.
	verified []string

	// scenarios are the faults currently injected, keyed by name so that
	// activeScenario is one map lookup and two scenarios can be active at once.
	scenarios map[ScenarioName]Scenario

	// calls counts RPCs by short method name. It is the observation that
	// proves a client did not retry.
	calls map[string]int

	// silence is the live go_silent injection, if any.
	silence *silence

	closed bool
}

// New builds and starts a simulated node. It returns a running server or an
// error, never a half-built value: a simulator that is listening on one of its
// two listeners is worse than one that failed.
// newServer builds the server value and opens nothing.
//
// It is separate from New so that a test can build the same value New builds
// and then hand serveOn listeners it can inspect. Rebuilding this struct in a
// test would be the usual kind of duplication: the test would keep passing
// while the thing it claims to be about had changed.
func newServer(opts Options) (*Server, error) {
	if opts.TalosVersion == "" {
		opts.TalosVersion = DefaultTalosVersion
	}
	if opts.Hostname == "" {
		opts.Hostname = "talossim"
	}
	if opts.NodeIP == "" {
		opts.NodeIP = "127.0.0.1"
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}

	if opts.KubeletImage == "" {
		opts.KubeletImage = "ghcr.io/siderolabs/kubelet:v" + DefaultKubernetesVersion
	}
	if opts.CPUs == 0 {
		opts.CPUs = 1
	}
	if opts.MemoryMiB == 0 {
		opts.MemoryMiB = 16384
	}
	if opts.Disks == nil {
		opts.Disks = []DiskFixture{{
			Device: "nvme0n1", Size: 512 << 30, Model: "SIMULATED NVMe", Transport: "nvme",
		}}
	}

	p, err := newPKI(opts.Hostname, opts.NodeIP, clusterOSCA(opts.Cluster), opts.Maintenance)
	if err != nil {
		return nil, err
	}

	return &Server{
		opts:      opts,
		pki:       p,
		node:      newNodeState(opts),
		streams:   newStreamer(opts.StreamMessages, opts.StreamChunk),
		done:      make(chan struct{}),
		scenarios: make(map[ScenarioName]Scenario),
		calls:     make(map[string]int),
	}, nil
}

// New builds and starts a simulated node. It returns a running server or an
// error, never a half-built value: a simulator that is listening on one of its
// two listeners is worse than one that failed.
func New(opts Options) (*Server, error) {
	s, err := newServer(opts)
	if err != nil {
		return nil, err
	}

	raw, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("talossim: listen on loopback: %w", err)
	}

	return s.serveOn(raw, bufconn.Listen(bufSize))
}

// serveOn finishes construction on the two listeners it is handed, and closes
// both if it cannot finish.
//
// It is split out of New for one reason: the promise in New's doc -- "a running
// server or an error, never a half-built value" -- was not kept on the seeding
// path, and a promise about listeners is only checkable by a test that can see
// the listeners. New opens them itself, so nothing outside could. This takes
// them as arguments, so an internal test can hand it a pair that records
// whether they were closed and then hand it a fixture that cannot seed.
func (s *Server) serveOn(raw net.Listener, pipe *bufconn.Listener) (*Server, error) {
	// Wrapped from the start rather than only while a scenario is active: the
	// connections flap_connection has to sever are the ones accepted before it
	// was injected, and a listener swapped in later would not know about them.
	tcp := newTrackingListener(raw)

	s.tcp = tcp
	s.tcpAddr = tcp.Addr().String()
	s.addrHistory = []string{s.tcpAddr}
	s.pipe = pipe

	// TLS is configured once, on the server, rather than by wrapping each
	// listener: grpc.Creds then performs exactly one handshake per connection on
	// either path, and both paths get the identical client-certificate
	// requirement.
	// The scenario gate is chained after recordPeer rather than folded into it:
	// recordPeer answers "did mTLS happen", the gate answers "does this call
	// happen at all", and a scenario that refuses a call must still have had
	// its certificate verified -- otherwise go_silent would be
	// indistinguishable from a handshake failure. The stream interceptor exists
	// for the same reason the unary one does: a node that has gone silent has
	// gone silent for Logs too.
	s.srv = grpc.NewServer(
		grpc.Creds(credentials.NewTLS(s.pki.serverTLS())),
		grpc.ChainUnaryInterceptor(s.recordPeer, s.scenarioUnary),
		grpc.ChainStreamInterceptor(s.scenarioStream),
	)
	s.registerNodeServices(s.srv)
	s.registerCOSI(s.srv)

	// Seeding happens before the listeners are served, so no caller can
	// observe the node in the half-populated state between construction and
	// the first resource being written.
	//
	// Both listeners are already open at this point, and a failure here used
	// to return without closing either of them. New's doc promises "a running
	// server or an error, never a half-built value" -- a leaked listener
	// holding a loopback port for the life of the process is exactly the
	// half-built value it promises not to leave behind, and it costs more in a
	// test binary than in production because a test binary builds hundreds of
	// these.
	if err := s.seedCOSI(context.Background()); err != nil {
		s.srv.Stop()
		_ = tcp.Close()
		_ = pipe.Close()
		return nil, err
	}

	s.serve(s.tcp)
	s.serve(s.pipe)

	return s, nil
}

func (s *Server) serve(l net.Listener) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		// Serve returns ErrServerStopped on a clean shutdown, and a closed
		// listener error when Close raced it. Neither is reportable: Close is
		// the only thing that stops this server.
		_ = s.srv.Serve(l)
	}()
}

// recordPeer notes the verified client certificate behind each call.
func (s *Server) recordPeer(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	if pr, ok := peer.FromContext(ctx); ok {
		if info, ok := pr.AuthInfo.(credentials.TLSInfo); ok {
			for _, chain := range info.State.VerifiedChains {
				if len(chain) > 0 {
					s.mu.Lock()
					s.verified = append(s.verified, chain[0].Subject.CommonName)
					s.mu.Unlock()
				}
			}
		}
	}
	return handler(ctx, req)
}

// Addr is the loopback TCP address the node listens on. It is what a direct
// dialer resolves a Target to.
//
// It reads the address the node is bound to rather than the listener, because
// flap_connection leaves the node with no listener for part of its cycle and
// the address is still the address it will come back on. ip_changes_on_reboot
// is the one scenario that changes what this answers.
func (s *Server) Addr() string {
	s.lmu.Lock()
	defer s.lmu.Unlock()

	return s.tcpAddr
}

// AddressHistory is every address this node has listened on, oldest first.
//
// ip_changes_on_reboot is only assertable if a test can name the address the
// node left behind; guessing it from a captured Addr() would work but would
// not survive a second rebind.
func (s *Server) AddressHistory() []string {
	s.lmu.Lock()
	defer s.lmu.Unlock()

	return append([]string(nil), s.addrHistory...)
}

// ListenerTransitions counts every close and every reopen of the TCP listener.
//
// It is the observation flap_connection is proven by. A test that only checked
// for a failed call would be asserting on a race between the client and the
// cycle, and would pass just as happily against a scenario that did nothing at
// all but was slow.
func (s *Server) ListenerTransitions() int {
	s.lmu.Lock()
	defer s.lmu.Unlock()

	return s.transitions
}

// Host is the address without the port, in the shape a talos.Target carries it.
func (s *Server) Host() string {
	host, _, err := net.SplitHostPort(s.Addr())
	if err != nil {
		return s.Addr()
	}
	return host
}

// Port is the loopback port. A direct dialer defaults to the apid port, so a
// test pointing one at this server has to be told which port to use.
func (s *Server) Port() int {
	_, port, err := net.SplitHostPort(s.Addr())
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(port)
	if err != nil {
		return 0
	}
	return n
}

// CA is the authority that signed this node's certificate.
func (s *Server) CA() *x509.Certificate { return s.pki.caCert }

// ClientCreds are cluster credentials that this node accepts: a client
// certificate its authority issued, and trust in that same authority.
func (s *Server) ClientCreds() talos.Creds {
	return talos.Creds{Kind: talos.CredCluster, TLS: s.pki.clientTLS()}
}

// MaintenanceCreds are what a client brings to a node in maintenance mode:
// nothing. There is no cluster authority to trust and no client certificate to
// present, so the configuration verifies nothing -- which is what
// `talosctl --insecure` does and the reason MaintenanceWarning exists.
//
// It is a method on the simulator rather than a value a test writes out because
// a test that wrote its own tls.Config could quietly keep trusting this node's
// authority, and would then be proving the maintenance path against a node that
// still has cluster PKI.
func (s *Server) MaintenanceCreds() talos.Creds {
	return talos.Creds{
		Kind: talos.CredMaintenance,
		TLS: &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // maintenance mode has no PKI; that is the point
			MinVersion:         tls.VersionTLS12,
		},
	}
}

// UntrustedClientCreds are credentials whose client certificate comes from an
// unrelated authority. They trust this node's server certificate, so a
// handshake made with them fails for exactly one reason: the node cannot verify
// the client. That is the negative control for the mTLS claim.
func (s *Server) UntrustedClientCreds() (talos.Creds, error) {
	// Deliberately nil rather than the cluster's authority: an "unrelated
	// authority" that happened to be the cluster's would verify, and the
	// negative control would prove nothing.
	foreign, err := newPKI(s.opts.Hostname, s.opts.NodeIP, nil, false)
	if err != nil {
		return talos.Creds{}, err
	}
	cfg := foreign.clientTLS()
	cfg.RootCAs = s.pki.pool()
	return talos.Creds{Kind: talos.CredCluster, TLS: cfg}, nil
}

// VerifiedClients returns the common names of the client certificates this node
// verified, in the order the calls arrived.
func (s *Server) VerifiedClients() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.verified...)
}

// Identity is what this node reports about itself. It is the same value the
// node's own discovery source emits.
func (s *Server) Identity() talos.Identity {
	n := s.node.snapshot()
	return talos.Identity{
		Hostname:    n.Hostname,
		Version:     n.Version,
		Maintenance: s.opts.Maintenance,
	}
}

// Close stops the node and waits for both listeners to finish. It is safe to
// call more than once, because a test that closes in a defer and again on a
// failure path is the normal case and not a bug.
func (s *Server) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	// Order matters and is the T-02-15/T-02-16 mitigation. done is closed
	// first so a handler parked by go_silent returns; the scenario goroutines
	// are drained next so flap_connection cannot re-listen after the server is
	// stopped; only then is the gRPC server stopped, because Stop blocks until
	// every pending RPC has returned.
	close(s.done)
	s.sg.Wait()

	s.srv.Stop()
	s.wg.Wait()

	// A listener a scenario opened after the gRPC server stopped serving it is
	// nobody else's to close, and a leaked one holds a port for the rest of the
	// test binary (T-02-16).
	s.closeListener()

	err := s.pipe.Close()
	if err != nil && !errors.Is(err, net.ErrClosed) {
		return fmt.Errorf("talossim: close the in-process listener: %w", err)
	}
	return nil
}
