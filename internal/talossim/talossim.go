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

	"github.com/holzcloud/holzkube/internal/talos"
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

	// Maintenance makes the node report itself as running the maintenance-mode
	// API surface.
	Maintenance bool

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

	pki *pki
	srv *grpc.Server

	tcp  net.Listener
	pipe *bufconn.Listener

	wg sync.WaitGroup

	mu sync.Mutex
	// verified records the common name of every client certificate the TLS
	// stack verified against ClientCAs. It is what lets a test assert that mTLS
	// actually happened rather than that a request merely succeeded.
	verified []string
	closed   bool
}

// New builds and starts a simulated node. It returns a running server or an
// error, never a half-built value: a simulator that is listening on one of its
// two listeners is worse than one that failed.
func New(opts Options) (*Server, error) {
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

	p, err := newPKI(opts.Hostname, opts.NodeIP)
	if err != nil {
		return nil, err
	}

	s := &Server{opts: opts, pki: p, node: newNodeState(opts)}

	tcp, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("talossim: listen on loopback: %w", err)
	}

	s.tcp = tcp
	s.pipe = bufconn.Listen(bufSize)

	// TLS is configured once, on the server, rather than by wrapping each
	// listener: grpc.Creds then performs exactly one handshake per connection on
	// either path, and both paths get the identical client-certificate
	// requirement.
	s.srv = grpc.NewServer(
		grpc.Creds(credentials.NewTLS(p.serverTLS())),
		grpc.UnaryInterceptor(s.recordPeer),
	)
	s.registerNodeServices(s.srv)

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
func (s *Server) Addr() string { return s.tcp.Addr().String() }

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

// UntrustedClientCreds are credentials whose client certificate comes from an
// unrelated authority. They trust this node's server certificate, so a
// handshake made with them fails for exactly one reason: the node cannot verify
// the client. That is the negative control for the mTLS claim.
func (s *Server) UntrustedClientCreds() (talos.Creds, error) {
	foreign, err := newPKI(s.opts.Hostname, s.opts.NodeIP)
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

	s.srv.Stop()
	s.wg.Wait()

	err := s.pipe.Close()
	if err != nil && !errors.Is(err, net.ErrClosed) {
		return fmt.Errorf("talossim: close the in-process listener: %w", err)
	}
	return nil
}
