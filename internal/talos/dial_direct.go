package talos

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// ApidPort is the port Talos serves its API on. It lives here rather than at
// any call site because a port is a property of the transport, and the whole
// point of the seam is that nothing above it knows one.
const ApidPort = 50000

// probeDialTimeout bounds the reachability half of Probe. It is short because
// the answer this probe is asked for is "is anything there", and a scan across
// a subnet pays this timeout once per address that is not.
const probeDialTimeout = 2 * time.Second

// NewDirectDialer returns the Dialer that reaches a node over plain TCP, the
// way holzkube reaches machines today.
//
// port defaults to ApidPort. It is a parameter rather than a constant because
// tests point this dialer at a simulated node on an ephemeral port, and because
// that is the whole demonstration: the transport is configurable below the seam
// and invisible above it.
func NewDirectDialer(port int) Dialer {
	if port <= 0 {
		port = ApidPort
	}
	return &directDialer{port: port}
}

type directDialer struct{ port int }

func (d *directDialer) Kind() string { return "direct" }

// Resolve turns identity into an address by reading the Target's address hint.
//
// A tunnel dialer would instead look up the overlay address it assigned when
// the node registered, which is why this is a method and not a helper function.
func (d *directDialer) Resolve(_ context.Context, t Target) (string, error) {
	if t.Addr == "" {
		return "", fmt.Errorf(
			"talos: cannot reach %s over the direct transport: the target carries no address hint; "+
				"a direct dial needs one, and it is set by discovery -- a node that has never been "+
				"seen must be discovered first, or reached over a transport that does not need an "+
				"address, such as a SideroLink tunnel", t.Machine)
	}
	return net.JoinHostPort(t.Addr, strconv.Itoa(d.port)), nil
}

// DialOptions returns the transport credentials built from the supplied TLS
// configuration. There is nothing transport-specific beyond that: a direct dial
// is what gRPC does by default, so this dialer's job is to add nothing.
func (d *directDialer) DialOptions(_ context.Context, _ Target, c Creds) ([]grpc.DialOption, error) {
	if c.TLS == nil {
		return nil, errors.New("talos: credentials carry no TLS configuration")
	}
	return []grpc.DialOption{
		grpc.WithTransportCredentials(credentials.NewTLS(c.TLS)),
	}, nil
}

// Probe is the cheap liveness and identity check.
//
// It must work with CredMaintenance, which means with no cluster PKI at all, so
// it does not make an RPC and does not verify the server: a node in maintenance
// mode serves a self-signed certificate and there is nothing yet to verify it
// against. Trust for that path comes from comparing Creds.Fingerprint at the
// call site, which is why this returns what it observed rather than a verdict.
//
// The distinction it does draw is the one callers need: a target that cannot be
// reached at all reports ErrNotReachableYet, which is a state. A target that
// answered -- meaning it spoke TLS, and the TLS was wrong -- is reachable and
// broken, which is an error.
//
// The line is drawn at "spoke TLS" and not at "accepted the connection",
// because the TCP accept is the kernel's and not the node's: a socket comes out
// of the listen backlog whether or not apid is serving behind it yet. A node
// that is still booting therefore completes the TCP handshake and then dies
// before a byte of TLS -- ECONNRESET, EPIPE or a clean EOF -- which is exactly
// the "not yet" this probe exists to report. Classifying that as a fault would
// paint a booting fleet broken, which is the failure ErrNotReachableYet was
// introduced to prevent. It is the same distinction errors.go draws between
// KindUnreachable and KindRejected, one layer down: what separates them is
// whether anything answered, not whether a socket opened.
func (d *directDialer) Probe(ctx context.Context, t Target) (Identity, error) {
	addr, err := d.Resolve(ctx, t)
	if err != nil {
		return Identity{}, err
	}

	dialCtx, cancel := context.WithTimeout(ctx, probeDialTimeout)
	defer cancel()

	var dialer net.Dialer
	conn, err := dialer.DialContext(dialCtx, "tcp", addr)
	if err != nil {
		// Nothing accepted the connection. On a subnet scan this is the common
		// case and not a fault, and with a tunnel transport it is what every
		// node looks like until it dials in.
		return Identity{}, fmt.Errorf("talos: %s at %s: %w: %w", t.Machine, addr, ErrNotReachableYet, err)
	}
	defer conn.Close() //nolint:errcheck // the probe owns this connection and discards it

	// InsecureSkipVerify is correct here and only here: this is the maintenance
	// path, which by definition has no CA to verify against. It is confined to
	// the probe, and NewClusterClient refuses credentials carrying it (T-02-01).
	tlsConn := tls.Client(conn, &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec // maintenance mode has no cluster PKI; see the comment above
		MinVersion:         tls.VersionTLS12,
	})
	if err := tlsConn.HandshakeContext(dialCtx); err != nil {
		if handshakeLostTheConnection(err) {
			return Identity{}, fmt.Errorf("talos: %s at %s: %w: the connection died before the TLS "+
				"handshake produced anything: %w", t.Machine, addr, ErrNotReachableYet, err)
		}
		return Identity{}, fmt.Errorf("talos: %s at %s accepted a connection but not a TLS handshake: %w", t.Machine, addr, err)
	}

	certs := tlsConn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return Identity{}, fmt.Errorf("talos: %s at %s presented no certificate", t.Machine, addr)
	}
	leaf := certs[0]

	id := Identity{Machine: t.Machine, Hostname: leaf.Subject.CommonName}
	if len(leaf.DNSNames) > 0 {
		id.Hostname = leaf.DNSNames[0]
	}

	// A node that has not been configured yet has no cluster CA to be signed
	// by, so it serves a certificate signed by itself. That is the one thing
	// about a node's state that is legible before authenticating to it.
	id.Maintenance = leaf.Issuer.String() == leaf.Subject.String()

	return id, nil
}

// handshakeLostTheConnection reports that the TLS handshake failed because the
// connection went away rather than because the peer disagreed about TLS.
//
// The two are told apart by the shape of the error and not by a list of errno
// values, which would be a list that is wrong on the next platform. Everything
// crypto/tls reports from a socket operation arrives as a *net.OpError -- a
// reset, a broken pipe, a connection the kernel aborted, the handshake deadline
// expiring -- and a peer that closed cleanly with nothing said arrives as io.EOF
// or io.ErrUnexpectedEOF. Everything else crypto/tls returns is a statement
// about TLS: a record that is not a record, an alert, a certificate that does
// not verify. Those mean a node answered, and a node that answered is reachable.
func handshakeLostTheConnection(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	var opErr *net.OpError
	return errors.As(err, &opErr)
}
