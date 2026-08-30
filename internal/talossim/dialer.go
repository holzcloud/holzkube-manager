package talossim

import (
	"context"
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// Dialer returns a talos.Dialer that reaches this node through its in-process
// listener, without touching the network stack.
//
// The direction of the dependency is the point: the fake implements the seam,
// and internal/talos never imports internal/talossim. That is what keeps the
// simulator out of the product's dependency graph while still letting the
// unmodified production client be driven against it.
func (s *Server) Dialer() talos.Dialer { return &pipeDialer{server: s} }

type pipeDialer struct{ server *Server }

// Kind reports "fake". A log line or an error from a test run names the
// transport that produced it, so an accidental in-process connection in a test
// that meant to use TCP is visible rather than silent.
func (d *pipeDialer) Kind() string { return "fake" }

// Resolve returns a syntactically valid address that is never actually dialled:
// the context dialer below ignores it and connects to the in-process listener.
// gRPC still needs a parseable target, so this returns the node's real address
// rather than a placeholder that a future resolver change might reject.
func (d *pipeDialer) Resolve(_ context.Context, t talos.Target) (string, error) {
	if t.Machine == "" {
		return "", fmt.Errorf("talossim: %w", errNoMachine)
	}
	return d.server.Addr(), nil
}

// DialOptions routes every connection into the in-process listener. This is the
// whole mechanism by which the unmodified machinery client can be pointed at an
// arbitrary transport: grpc.WithContextDialer replaces the socket, and nothing
// above the seam knows.
func (d *pipeDialer) DialOptions(_ context.Context, _ talos.Target, c talos.Creds) ([]grpc.DialOption, error) {
	if c.TLS == nil {
		return nil, errNoTLS
	}
	return []grpc.DialOption{
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return d.server.pipe.DialContext(ctx)
		}),
		// The credentials are returned here as well as configured on the
		// machinery client, because a caller that supplies its own dial options
		// must end up with the same TLS on both transports or the comparison
		// between them proves nothing.
		grpc.WithTransportCredentials(credentials.NewTLS(c.TLS)),
	}, nil
}

// Probe answers from what the simulator was configured with. A real transport
// would make an RPC here; the simulator knows the answer without one, and
// pretending otherwise would only test the simulator.
func (d *pipeDialer) Probe(_ context.Context, t talos.Target) (talos.Identity, error) {
	id := d.server.Identity()
	id.Machine = t.Machine
	return id, nil
}
