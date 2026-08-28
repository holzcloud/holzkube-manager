package talos

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/siderolabs/talos/pkg/machinery/client"
)

// probeTimeout bounds the liveness probe that NewClusterClient performs.
//
// It is deliberately short. The probe exists to turn "constructed" into
// "reachable", and a caller that waits a minute to be told a node is down has
// been given the worst of both answers.
const probeTimeout = 5 * time.Second

// ClusterClient is a connection to a configured Talos node, using full cluster
// mTLS.
//
// It is a separate type from the maintenance-mode client rather than one type
// with a mode flag (D-06). The two speak different API surfaces and trust
// different things, and a single type with a boolean is an invitation to write
// code that forgets to check it -- the same argument store.SettingsStore makes
// for not having a List method.
type ClusterClient struct {
	c      *client.Client
	target Target
	dialer Dialer

	// now is the clock. It is a field rather than a call to time.Now so that
	// the deadline and breaker behaviour can be tested at the durations it
	// actually runs at instead of at durations chosen to keep a test fast --
	// the same reason auth.Service carries one.
	now func() time.Time
}

// NewClusterClient dials a configured node and proves it is answering.
//
// Construction is not a connectivity assertion on its own: machinery's
// client.New ignores its context, and the grpc.NewClient underneath it is lazy,
// so a client to an unplugged machine is built without complaint and fails at
// the first call instead. Per D-05 this constructor therefore ends with an
// explicit Version probe under a short deadline, and returns (nil, error)
// rather than a half-built value that looks connected.
//
// The cost of that probe is reported the way auth reports its argon2id
// calibration -- at debug level, because unlike calibration it is paid per
// connection and would otherwise be the noisiest line in the log.
func NewClusterClient(ctx context.Context, d Dialer, t Target, c Creds) (*ClusterClient, error) {
	if d == nil {
		return nil, errors.New("talos: nil dialer")
	}
	if t.Machine == "" {
		return nil, errors.New("talos: target has no machine ID; the address is a hint and never identity")
	}
	if c.Kind != CredCluster {
		return nil, fmt.Errorf(
			"talos: refusing to build a cluster client with %q credentials: "+
				"maintenance mode has no cluster PKI and is served by a different client type (D-06)",
			c.Kind)
	}
	if c.TLS == nil {
		return nil, errors.New("talos: cluster credentials carry no TLS configuration")
	}
	if c.TLS.InsecureSkipVerify {
		// T-02-01. The cluster path is the one that carries cluster PKI, so it
		// is the one where a skipped verification is worth the most to an
		// attacker. There is no flag that turns this off.
		return nil, errors.New(
			"talos: refusing cluster credentials with InsecureSkipVerify: " +
				"server verification is never disabled on the cluster path; " +
				"an unverifiable node belongs on the maintenance path with a pinned fingerprint")
	}

	addr, err := d.Resolve(ctx, t)
	if err != nil {
		return nil, fmt.Errorf("talos: resolve %s: %w", t.Machine, err)
	}

	opts, err := d.DialOptions(ctx, t, c)
	if err != nil {
		return nil, fmt.Errorf("talos: dial options for %s: %w", t.Machine, err)
	}

	// client.WithNodes / client.WithNode are context decorators returning a
	// context.Context, not OptionFuncs -- they set the gRPC metadata apid uses
	// to proxy onward. They belong on the per-call context, which is why none
	// of them appears here.
	cl, err := client.New(ctx,
		client.WithTLSConfig(c.TLS),
		client.WithEndpoints(addr),
		client.WithGRPCDialOptions(opts...),
	)
	if err != nil {
		return nil, fmt.Errorf("talos: connect to %s: %w", t.Machine, err)
	}

	cc := &ClusterClient{c: cl, target: t, dialer: d, now: time.Now}

	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	started := cc.now()
	version, err := cc.Version(probeCtx)
	if err != nil {
		_ = cl.Close()
		return nil, fmt.Errorf("talos: %s did not answer the liveness probe: %w", t.Machine, err)
	}

	slog.Debug("talos node reachable",
		slog.String("machine", string(t.Machine)),
		slog.String("transport", d.Kind()),
		slog.String("version", version),
		slog.Duration("probe", cc.now().Sub(started)))

	return cc, nil
}

// Target returns the identity this client was built for. It is the only way
// above the seam to ask which node a client belongs to, and it deliberately
// answers with identity rather than with an address.
func (c *ClusterClient) Target() Target { return c.target }

// Transport returns the Kind of the dialer behind this client, for logging and
// for tests that assert which transport was used.
func (c *ClusterClient) Transport() string { return c.dialer.Kind() }

// Version returns the node's Talos version tag.
func (c *ClusterClient) Version(ctx context.Context) (string, error) {
	resp, err := c.c.Version(ctx)
	if err != nil {
		return "", fmt.Errorf("talos: version of %s: %w", c.target.Machine, err)
	}
	if len(resp.GetMessages()) == 0 {
		return "", fmt.Errorf("talos: %s returned an empty version response", c.target.Machine)
	}
	return resp.GetMessages()[0].GetVersion().GetTag(), nil
}

// Close releases the connection.
func (c *ClusterClient) Close() error {
	if err := c.c.Close(); err != nil {
		return fmt.Errorf("talos: close %s: %w", c.target.Machine, err)
	}
	return nil
}
