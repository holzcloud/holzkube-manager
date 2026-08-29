package talos

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/cosi-project/runtime/pkg/state"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/client"
)

// probeDeadline bounds the liveness probe both constructors perform.
//
// It is the probe class deadline: deliberately short, because the probe exists
// to turn "constructed" into "reachable", and a caller that waits a minute to
// be told a node is down has been given the worst of both answers.
const probeDeadline = 5 * time.Second

// ClusterClient is a connection to a configured Talos node, using full cluster
// mTLS.
//
// It is a separate type from the maintenance-mode client rather than one type
// with a mode flag (D-06). The two speak different API surfaces and trust
// different things, and a single type with a boolean is an invitation to write
// code that forgets to check it -- the same argument store.SettingsStore makes
// for not having a List method.
type ClusterClient struct {
	conn *conn
}

// MaintenanceClient is a connection to a node that has not been configured yet.
//
// Its method set is the whole design. Maintenance mode realistically serves
// ApplyConfiguration, Version, Disks and COSI reads, so this type serves
// exactly those and nothing else -- which means a cluster-only call against a
// node in maintenance mode is a compile error rather than a runtime rejection.
// That is what success criterion 3's "nicht verwechselbar" is asking for, and
// it is the difference between a failure the person writing the line pays for
// and a failure whoever reaches it in production pays for.
//
// machinery provides no such split: both connect paths produce the same
// *client.Client and nothing on the value records its mode. The separation is
// built entirely here, which is why growing this type is easy and why
// TestMaintenanceClientMethodSetIsClosed pins the set whole rather than
// asserting that one particular method is absent.
//
// internal/auth's package doc names what that package must not know; this type
// says the mirror image -- what it must not be able to do.
type MaintenanceClient struct {
	conn *conn
}

// conn is the shared connection both client types are built on.
//
// It exists so the two constructors cannot drift in how they dial, and -- more
// importantly -- so there is exactly one call path. The deadline gate and the
// retry loop are installed on it as gRPC interceptors rather than inside each
// wrapper method, because a policy that has to be remembered once per method is
// a policy that will be forgotten in one of them, and the method somebody
// forgets is the one that hangs forever in production.
type conn struct {
	c      *client.Client
	target Target
	dialer Dialer

	// now is the clock. It is a field rather than a call to time.Now so that
	// the deadline and breaker behaviour can be tested at the durations it
	// actually runs at instead of at durations chosen to keep a test fast --
	// the same reason auth.Service carries one.
	now func() time.Time
}

// dial is the one place either client type reaches a node.
func dial(ctx context.Context, d Dialer, t Target, c Creds) (*conn, error) {
	if d == nil {
		return nil, errors.New("talos: nil dialer")
	}
	if t.Machine == "" {
		return nil, errors.New("talos: target has no machine ID; the address is a hint and never identity")
	}
	if c.TLS == nil {
		return nil, fmt.Errorf("talos: %s credentials carry no TLS configuration", c.Kind)
	}

	addr, err := d.Resolve(ctx, t)
	if err != nil {
		return nil, fmt.Errorf("talos: resolve %s: %w", t.Machine, err)
	}

	opts, err := d.DialOptions(ctx, t, c)
	if err != nil {
		return nil, fmt.Errorf("talos: dial options for %s: %w", t.Machine, err)
	}

	n := &conn{target: t, dialer: d, now: time.Now}

	// The policy interceptors are appended after the transport's own options,
	// so a Dialer cannot displace them: a transport chooses how bytes travel,
	// not whether a call carries a deadline.
	opts = append(opts,
		grpc.WithChainUnaryInterceptor(n.unaryPolicy),
		grpc.WithChainStreamInterceptor(n.streamPolicy),
	)

	// client.WithNodes / client.WithNode are context decorators returning a
	// context.Context, not OptionFuncs -- they set the gRPC metadata apid uses
	// to proxy onward. They belong on the per-call context, which is why none
	// of them appears here.
	//
	// WithTLSConfig is what lets the maintenance path connect with no
	// talosconfig and no CA at all: a non-nil tlsConfig short-circuits in
	// client/connection.go before buildTLSConfig is consulted, which is exactly
	// what talosctl --insecure does. client/insecure_credentials.go is behind a
	// sidero.debug build tag and does not compile in a normal build.
	cl, err := client.New(ctx,
		client.WithTLSConfig(c.TLS),
		client.WithEndpoints(addr),
		client.WithGRPCDialOptions(opts...),
	)
	if err != nil {
		return nil, fmt.Errorf("talos: connect to %s: %w", t.Machine, err)
	}

	n.c = cl
	return n, nil
}

// unaryPolicy is the single path every unary call on either client type takes.
//
// It asks for the response trailers, because their presence is what separates
// "the node answered and refused" from "nothing answered". A server always
// sends trailers with its status; a call that never reached one has none. The
// status code cannot make that distinction on its own -- codes.Unavailable
// arrives both from a refused connection and from a node whose etcd is down --
// and getting it wrong is what would let one broken subsystem open a node's
// circuit breaker.
func (n *conn) unaryPolicy(
	ctx context.Context,
	method string,
	req, reply any,
	cc *grpc.ClientConn,
	invoker grpc.UnaryInvoker,
	opts ...grpc.CallOption,
) error {
	var trailer metadata.MD

	err := invoker(ctx, method, req, reply, cc, append(opts, grpc.Trailer(&trailer))...)
	if err == nil {
		return nil
	}

	return classify(ctx, shortMethod(method), n.target.Machine, len(trailer) > 0, err)
}

// streamPolicy classifies a failure to open a server stream.
//
// A stream that opened and then failed is classified where it fails, on Recv,
// because at the moment a stream is created the node has not answered anything
// yet and there are no trailers to read.
func (n *conn) streamPolicy(
	ctx context.Context,
	desc *grpc.StreamDesc,
	cc *grpc.ClientConn,
	method string,
	streamer grpc.Streamer,
	opts ...grpc.CallOption,
) (grpc.ClientStream, error) {
	cs, err := streamer(ctx, desc, cc, method, opts...)
	if err != nil {
		return nil, classify(ctx, shortMethod(method), n.target.Machine, false, err)
	}
	return &policyStream{ClientStream: cs, conn: n, op: shortMethod(method)}, nil
}

// policyStream classifies the errors a running stream produces.
type policyStream struct {
	grpc.ClientStream

	conn *conn
	op   string
}

func (s *policyStream) RecvMsg(m any) error {
	err := s.ClientStream.RecvMsg(m)
	if err == nil {
		return nil
	}
	// io.EOF is the ordinary end of a stream and is not a failure; it is
	// returned untouched so a caller's errors.Is(err, io.EOF) keeps working.
	if errors.Is(err, io.EOF) {
		return err
	}
	return classify(s.Context(), s.op, s.conn.target.Machine, len(s.Trailer()) > 0, err)
}

// shortMethod is the trailing element of a gRPC full method name.
//
// The short name is what an operator reading an error recognises:
// "EtcdMemberList" is the call they made, "/machine.MachineService/..." is the
// protobuf's business.
func shortMethod(fullMethod string) string {
	if i := strings.LastIndexByte(fullMethod, '/'); i >= 0 {
		return fullMethod[i+1:]
	}
	return fullMethod
}

// NewClusterClient dials a configured node and proves it is answering.
//
// Construction is not a connectivity assertion on its own: machinery's
// client.New ignores its context, and the grpc.NewClient underneath it is lazy,
// so a client to an unplugged machine is built without complaint and fails at
// the first call instead. Per D-05 this constructor therefore ends with an
// explicit Version probe under the probe class deadline, and returns
// (nil, error) rather than a half-built value that looks connected.
//
// The cost of that probe is reported the way auth reports its argon2id
// calibration -- at debug level, because unlike calibration it is paid per
// connection and would otherwise be the noisiest line in the log.
func NewClusterClient(ctx context.Context, d Dialer, t Target, c Creds) (*ClusterClient, error) {
	if c.Kind != CredCluster {
		return nil, fmt.Errorf(
			"talos: refusing to build a cluster client with %q credentials: it requires %q. "+
				"maintenance mode has no cluster PKI and is served by a different client type (D-06)",
			c.Kind, CredCluster)
	}
	if c.TLS != nil && c.TLS.InsecureSkipVerify {
		// T-02-01. The cluster path is the one that carries cluster PKI, so it
		// is the one where a skipped verification is worth the most to an
		// attacker. There is no flag that turns this off.
		return nil, errors.New(
			"talos: refusing cluster credentials with InsecureSkipVerify: " +
				"server verification is never disabled on the cluster path; " +
				"an unverifiable node belongs on the maintenance path with a pinned fingerprint")
	}

	n, err := dial(ctx, d, t, c)
	if err != nil {
		return nil, err
	}

	cc := &ClusterClient{conn: n}
	if err := n.proveAnswering(ctx, cc.Version); err != nil {
		return nil, err
	}
	return cc, nil
}

// NewMaintenanceClient dials a node that has not been configured yet.
//
// The connection is made with client.WithTLSConfig, which is the verified path
// for connecting with no talosconfig and no CA: a non-nil tlsConfig
// short-circuits in machinery's client/connection.go before buildTLSConfig is
// ever consulted. That is what talosctl --insecure does, and it is why
// InsecureSkipVerify is permitted here and refused on the cluster path.
//
// The TLS configuration is still supplied by the caller, because this package
// never invents trust. Creds.Fingerprint is the seam on which a later phase
// pins the maintenance certificate without a signature change (T-02-27); no
// pinning is performed here yet.
func NewMaintenanceClient(ctx context.Context, d Dialer, t Target, c Creds) (*MaintenanceClient, error) {
	if c.Kind != CredMaintenance {
		return nil, fmt.Errorf(
			"talos: refusing to build a maintenance client with %q credentials: it requires %q. "+
				"a configured node is reached with cluster PKI through a different client type (D-06)",
			c.Kind, CredMaintenance)
	}

	n, err := dial(ctx, d, t, c)
	if err != nil {
		return nil, err
	}

	mc := &MaintenanceClient{conn: n}
	if err := n.proveAnswering(ctx, mc.Version); err != nil {
		return nil, err
	}
	return mc, nil
}

// proveAnswering is D-05, shared by both constructors: turn "constructed" into
// "reachable", and refuse a node running a Talos version outside the supported
// window while the probe's answer is still in hand.
//
// The range check rides on the liveness probe rather than on a second RPC: the
// probe already had to ask for the version, and a client that proved a node is
// answering and then proceeded against an API surface nobody has tested has
// spent the probe and learned only half of what it cost. It applies to the
// maintenance path too -- that path exists in order to apply a configuration,
// and applying one to an untested version is the more expensive mistake.
func (n *conn) proveAnswering(ctx context.Context, version func(context.Context) (string, error)) error {
	probeCtx, cancel := context.WithTimeout(ctx, probeDeadline)
	defer cancel()

	started := n.now()

	v, err := version(probeCtx)
	if err != nil {
		_ = n.c.Close()
		return fmt.Errorf("talos: %s did not answer the liveness probe: %w", n.target.Machine, err)
	}

	if err := CheckSupportedVersion(v); err != nil {
		_ = n.c.Close()
		return fmt.Errorf("talos: refusing to build a client for %s: %w", n.target.Machine, err)
	}

	slog.Debug("talos node reachable",
		slog.String("machine", string(n.target.Machine)),
		slog.String("transport", n.dialer.Kind()),
		slog.String("version", v),
		slog.Duration("probe", n.now().Sub(started)))

	return nil
}

// Target returns the identity this client was built for. It is the only way
// above the seam to ask which node a client belongs to, and it deliberately
// answers with identity rather than with an address.
func (c *ClusterClient) Target() Target { return c.conn.target }

// Transport returns the Kind of the dialer behind this client, for logging and
// for tests that assert which transport was used.
func (c *ClusterClient) Transport() string { return c.conn.dialer.Kind() }

// COSI is the node's resource state, read-only in practice for everything
// holzkube does with it.
func (c *ClusterClient) COSI() state.State { return c.conn.c.COSI }

// Version returns the node's Talos version tag.
func (c *ClusterClient) Version(ctx context.Context) (string, error) {
	resp, err := c.conn.c.Version(ctx)
	if err != nil {
		return "", err
	}
	if len(resp.GetMessages()) == 0 {
		return "", fmt.Errorf("talos: %s returned an empty version response", c.conn.target.Machine)
	}
	return resp.GetMessages()[0].GetVersion().GetTag(), nil
}

// Hostname returns the node's hostname.
//
// It goes through MachineClient rather than through a wrapper on
// client.Client, because machinery provides no wrapper for this RPC.
func (c *ClusterClient) Hostname(ctx context.Context) (string, error) {
	resp, err := c.conn.c.MachineClient.Hostname(ctx, &emptypb.Empty{})
	if err != nil {
		return "", err
	}
	if len(resp.GetMessages()) == 0 {
		return "", fmt.Errorf("talos: %s returned an empty hostname response", c.conn.target.Machine)
	}
	return resp.GetMessages()[0].GetHostname(), nil
}

// Service is one entry of the node's service list.
type Service struct {
	ID      string
	State   string
	Healthy bool

	// HealthUnknown distinguishes "reported unhealthy" from "has no health
	// check", which are different facts about a service and would otherwise
	// both arrive as Healthy == false.
	HealthUnknown bool
}

// ServiceList reports the node's services and their health.
func (c *ClusterClient) ServiceList(ctx context.Context) ([]Service, error) {
	resp, err := c.conn.c.ServiceList(ctx)
	if err != nil {
		return nil, err
	}

	var out []Service
	for _, msg := range resp.GetMessages() {
		for _, svc := range msg.GetServices() {
			out = append(out, Service{
				ID:            svc.GetId(),
				State:         svc.GetState(),
				Healthy:       svc.GetHealth().GetHealthy(),
				HealthUnknown: svc.GetHealth().GetUnknown(),
			})
		}
	}
	return out, nil
}

// EtcdMember is one member of the node's etcd cluster.
type EtcdMember struct {
	ID       uint64
	Hostname string
}

// EtcdMemberList reports the etcd membership as this node sees it.
func (c *ClusterClient) EtcdMemberList(ctx context.Context) ([]EtcdMember, error) {
	resp, err := c.conn.c.EtcdMemberList(ctx, &machine.EtcdMemberListRequest{})
	if err != nil {
		return nil, err
	}

	var out []EtcdMember
	for _, msg := range resp.GetMessages() {
		for _, m := range msg.GetMembers() {
			out = append(out, EtcdMember{ID: m.GetId(), Hostname: m.GetHostname()})
		}
	}
	return out, nil
}

// ApplyResult is the node's own account of what an applied configuration did.
type ApplyResult struct {
	Mode    string
	Details string
}

// ApplyConfiguration hands the node a machine configuration.
func (c *ClusterClient) ApplyConfiguration(ctx context.Context, cfg []byte) (ApplyResult, error) {
	return applyConfiguration(ctx, c.conn, cfg)
}

// Bootstrap initialises etcd on this node. It is a cluster-only operation:
// a node in maintenance mode has no etcd to bootstrap, which is why
// MaintenanceClient does not have this method and why calling it there does not
// compile.
func (c *ClusterClient) Bootstrap(ctx context.Context) error {
	return c.conn.c.Bootstrap(ctx, &machine.BootstrapRequest{})
}

// Reboot restarts the node. Cluster-only, for the same reason as Bootstrap.
func (c *ClusterClient) Reboot(ctx context.Context) error {
	// Reboot rather than RebootWithResponse, deliberately: talossim's
	// TestMethodCoverage resolves a call site by the method's name, and
	// RebootWithResponse resolves to no RPC in either service descriptor -- so
	// the guard would log it as "nothing for the simulator to implement" and
	// stop covering the one Reboot call the product makes. The response carries
	// nothing this caller reads.
	return c.conn.c.Reboot(ctx)
}

// Close releases the connection.
func (c *ClusterClient) Close() error { return c.conn.close() }

// Version returns the node's Talos version tag.
func (m *MaintenanceClient) Version(ctx context.Context) (string, error) {
	resp, err := m.conn.c.Version(ctx)
	if err != nil {
		return "", err
	}
	if len(resp.GetMessages()) == 0 {
		return "", fmt.Errorf("talos: %s returned an empty version response", m.conn.target.Machine)
	}
	return resp.GetMessages()[0].GetVersion().GetTag(), nil
}

// Disk is one block device the node reported.
//
// It is holzkube's own shape rather than machinery's: the seam's rule is that
// the machinery client is the implementation of this package and not a type
// that travels above it, and an installer picking a disk needs five fields
// rather than fourteen.
type Disk struct {
	Device string
	Model  string
	Serial string
	Size   uint64
	Type   string

	// System reports the disk Talos is or would be installed on.
	System bool
}

// Disks reports the node's block devices, which is what an installer picks
// from.
func (m *MaintenanceClient) Disks(ctx context.Context) ([]Disk, error) {
	resp, err := m.conn.c.Disks(ctx)
	if err != nil {
		return nil, err
	}

	var out []Disk
	for _, msg := range resp.GetMessages() {
		for _, d := range msg.GetDisks() {
			out = append(out, Disk{
				Device: d.GetDeviceName(),
				Model:  d.GetModel(),
				Serial: d.GetSerial(),
				Size:   d.GetSize(),
				Type:   d.GetType().String(),
				System: d.GetSystemDisk(),
			})
		}
	}
	return out, nil
}

// ApplyConfiguration hands an unconfigured node its machine configuration. It
// is the one thing maintenance mode exists for.
func (m *MaintenanceClient) ApplyConfiguration(ctx context.Context, cfg []byte) (ApplyResult, error) {
	return applyConfiguration(ctx, m.conn, cfg)
}

// COSI is the node's resource state. Maintenance mode serves reads of it, which
// is how a node can be inspected before it has been configured.
func (m *MaintenanceClient) COSI() state.State { return m.conn.c.COSI }

// Close releases the connection.
func (m *MaintenanceClient) Close() error { return m.conn.close() }

// applyConfiguration is shared by both client types, because the call is
// identical and the difference between the two types is which methods exist
// rather than how the shared ones behave.
func applyConfiguration(ctx context.Context, n *conn, cfg []byte) (ApplyResult, error) {
	resp, err := n.c.ApplyConfiguration(ctx, &machine.ApplyConfigurationRequest{Data: cfg})
	if err != nil {
		return ApplyResult{}, err
	}
	if len(resp.GetMessages()) == 0 {
		return ApplyResult{}, fmt.Errorf("talos: %s returned an empty apply response", n.target.Machine)
	}

	msg := resp.GetMessages()[0]
	return ApplyResult{Mode: msg.GetMode().String(), Details: msg.GetModeDetails()}, nil
}

func (n *conn) close() error {
	if err := n.c.Close(); err != nil {
		return fmt.Errorf("talos: close %s: %w", n.target.Machine, err)
	}
	return nil
}
