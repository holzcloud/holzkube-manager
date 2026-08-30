package talossim_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/client"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// newMachineryClient builds the unmodified production client against a
// simulated node.
//
// It is machinery's own client.New, dialled through the simulator's Dialer --
// the same construction talos.NewClusterClient performs. The wrapper is not
// used here because the wrapper deliberately exposes only the methods the
// product has a use for yet, and these tests are about the node's API surface
// rather than about the wrapper's.
func newMachineryClient(t *testing.T, sim *talossim.Server) *client.Client {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	creds := sim.ClientCreds()
	target := talos.Target{
		Cluster: model.ClusterID("sim"),
		Machine: model.MachineID("00000000-0000-0000-0000-0000000000aa"),
		Addr:    sim.Host(),
	}

	opts, err := sim.Dialer().DialOptions(ctx, target, creds)
	if err != nil {
		t.Fatalf("dial options: %v", err)
	}

	cl, err := client.New(ctx,
		client.WithTLSConfig(creds.TLS),
		client.WithEndpoints(sim.Addr()),
		client.WithGRPCDialOptions(opts...),
	)
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	t.Cleanup(func() {
		if err := cl.Close(); err != nil {
			t.Errorf("client.Close: %v", err)
		}
	})

	return cl
}

func testContext(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// TestNodeAnswersTheImplementedSurface calls every RPC this plan implements and
// asserts each returns a populated message that says which node answered.
//
// A simulator whose replies are empty is not usable as an oracle: a caller
// asserting on a field cannot tell "the node reported nothing" from "the field
// is not implemented", and the second is a drift the test suite should be
// catching rather than absorbing.
func TestNodeAnswersTheImplementedSurface(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "surface-node", TalosVersion: "v1.13.9"})
	cl := newMachineryClient(t, sim)
	ctx := testContext(t)

	// EtcdMemberList and EtcdStatus report on a running etcd, so the node has
	// to be bootstrapped before they can answer anything truthful.
	if err := cl.Bootstrap(ctx, &machine.BootstrapRequest{}); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	version, err := cl.Version(ctx)
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if got := version.GetMessages()[0].GetVersion().GetTag(); got != "v1.13.9" {
		t.Errorf("Version tag = %q, want %q", got, "v1.13.9")
	}
	assertAnsweredBy(t, "Version", version.GetMessages()[0].GetMetadata().GetHostname(), "surface-node")

	hostname, err := cl.MachineClient.Hostname(ctx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("Hostname: %v", err)
	}
	if got := hostname.GetMessages()[0].GetHostname(); got != "surface-node" {
		t.Errorf("Hostname = %q, want %q", got, "surface-node")
	}
	assertAnsweredBy(t, "Hostname", hostname.GetMessages()[0].GetMetadata().GetHostname(), "surface-node")

	disks, err := cl.Disks(ctx)
	if err != nil {
		t.Fatalf("Disks: %v", err)
	}
	if len(disks.GetMessages()[0].GetDisks()) == 0 {
		t.Error("Disks returned no block devices")
	}
	assertAnsweredBy(t, "Disks", disks.GetMessages()[0].GetMetadata().GetHostname(), "surface-node")

	services, err := cl.ServiceList(ctx)
	if err != nil {
		t.Fatalf("ServiceList: %v", err)
	}
	if len(services.GetMessages()[0].GetServices()) == 0 {
		t.Error("ServiceList returned no services")
	}
	assertAnsweredBy(t, "ServiceList", services.GetMessages()[0].GetMetadata().GetHostname(), "surface-node")

	applied, err := cl.ApplyConfiguration(ctx, &machine.ApplyConfigurationRequest{
		Data: []byte("version: v1alpha1\n"),
		Mode: machine.ApplyConfigurationRequest_AUTO,
	})
	if err != nil {
		t.Fatalf("ApplyConfiguration: %v", err)
	}
	if applied.GetMessages()[0].GetModeDetails() == "" {
		t.Error("ApplyConfiguration returned no mode details")
	}
	assertAnsweredBy(t, "ApplyConfiguration", applied.GetMessages()[0].GetMetadata().GetHostname(), "surface-node")

	members, err := cl.EtcdMemberList(ctx, &machine.EtcdMemberListRequest{})
	if err != nil {
		t.Fatalf("EtcdMemberList: %v", err)
	}
	if len(members.GetMessages()[0].GetMembers()) == 0 {
		t.Error("EtcdMemberList returned no members on a bootstrapped node")
	}
	assertAnsweredBy(t, "EtcdMemberList", members.GetMessages()[0].GetMetadata().GetHostname(), "surface-node")

	etcd, err := cl.EtcdStatus(ctx)
	if err != nil {
		t.Fatalf("EtcdStatus: %v", err)
	}
	if etcd.GetMessages()[0].GetMemberStatus().GetMemberId() == 0 {
		t.Error("EtcdStatus returned no member id")
	}
	assertAnsweredBy(t, "EtcdStatus", etcd.GetMessages()[0].GetMetadata().GetHostname(), "surface-node")

	reboot, err := cl.RebootWithResponse(ctx)
	if err != nil {
		t.Fatalf("Reboot: %v", err)
	}
	if reboot.GetMessages()[0].GetActorId() == "" {
		t.Error("Reboot returned no actor id, so nothing correlates the reply with the events it causes")
	}
	assertAnsweredBy(t, "Reboot", reboot.GetMessages()[0].GetMetadata().GetHostname(), "surface-node")

	// Shutdown and Reset both stop the node, so they get their own nodes: an
	// assertion made after a shutdown would be an assertion about a machine
	// that is off.
	shutdownSim := newSim(t, talossim.Options{Hostname: "shutdown-node"})
	shutdown, err := newMachineryClient(t, shutdownSim).ShutdownWithResponse(testContext(t))
	if err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if shutdown.GetMessages()[0].GetActorId() == "" {
		t.Error("Shutdown returned no actor id")
	}
	assertAnsweredBy(t, "Shutdown", shutdown.GetMessages()[0].GetMetadata().GetHostname(), "shutdown-node")

	resetSim := newSim(t, talossim.Options{Hostname: "reset-node"})
	resetResp, err := newMachineryClient(t, resetSim).ResetGenericWithResponse(testContext(t),
		&machine.ResetRequest{Graceful: true, Reboot: true})
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if resetResp.GetMessages()[0].GetActorId() == "" {
		t.Error("Reset returned no actor id")
	}
	assertAnsweredBy(t, "Reset", resetResp.GetMessages()[0].GetMetadata().GetHostname(), "reset-node")
}

// assertAnsweredBy pins T-02-08-03: a reply that does not say which node
// produced it cannot be told apart from a reply produced by the wrong one, and
// the ip_changes_on_reboot scenario is exactly the case where that matters.
func assertAnsweredBy(t *testing.T, rpc, got, want string) {
	t.Helper()

	if got != want {
		t.Errorf("%s was answered by %q, want %q; a response that does not identify its node "+
			"cannot distinguish the right node answering from any node answering", rpc, got, want)
	}
}

// TestBootstrapChangesWhatALaterReadReports is the mutation-observability
// claim.
//
// A Bootstrap that only flipped a private flag would be indistinguishable from
// a Bootstrap that did nothing at all, and the scenario engine in plan 02-03
// has nothing to script against a node whose state is invisible. So the test
// reads the node before and after through two different RPCs.
func TestBootstrapChangesWhatALaterReadReports(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "bootstrap-node"})
	cl := newMachineryClient(t, sim)
	ctx := testContext(t)

	if _, err := cl.EtcdMemberList(ctx, &machine.EtcdMemberListRequest{}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("EtcdMemberList before Bootstrap returned %v, want FailedPrecondition: "+
			"a node without etcd running has no member list, and an empty list is a different fact", err)
	}
	if etcdState(ctx, t, cl) != "Preparing" {
		t.Error("etcd is Running on a node that has not been bootstrapped")
	}

	if err := cl.Bootstrap(ctx, &machine.BootstrapRequest{}); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	members, err := cl.EtcdMemberList(ctx, &machine.EtcdMemberListRequest{})
	if err != nil {
		t.Fatalf("EtcdMemberList after Bootstrap: %v", err)
	}
	if got := members.GetMessages()[0].GetMembers()[0].GetHostname(); got != "bootstrap-node" {
		t.Errorf("etcd member hostname = %q, want %q", got, "bootstrap-node")
	}
	if etcdState(ctx, t, cl) != "Running" {
		t.Error("etcd is not Running after a successful Bootstrap")
	}

	// The second bootstrap is a call against changed state, and a real node
	// refuses it because the etcd data directory is no longer empty.
	err = cl.Bootstrap(ctx, &machine.BootstrapRequest{})
	if status.Code(err) != codes.AlreadyExists {
		t.Fatalf("the second Bootstrap returned %v, want AlreadyExists; "+
			"a simulator that accepts it twice is easier to satisfy than the hardware", err)
	}

	if got := sim.Node().BootstrapCalls; got != 2 {
		t.Errorf("BootstrapCalls = %d, want 2; a refused call is still a call", got)
	}
	if !sim.Node().Bootstrapped {
		t.Error("the refused second Bootstrap un-bootstrapped the node")
	}
}

func etcdState(ctx context.Context, t *testing.T, cl *client.Client) string {
	t.Helper()

	resp, err := cl.ServiceList(ctx)
	if err != nil {
		t.Fatalf("ServiceList: %v", err)
	}
	for _, svc := range resp.GetMessages()[0].GetServices() {
		if svc.GetId() == "etcd" {
			return svc.GetState()
		}
	}
	t.Fatal("ServiceList reported no etcd service")
	return ""
}

// TestUnimplementedMethodIsUnimplemented proves the inherited path is the
// inherited path.
//
// The simulator gets all 54 MachineService methods from the embedded
// UnimplementedMachineServiceServer, and the whole value of that arrangement is
// that a method nobody wrote answers with a status a caller can see rather than
// with a zero value a caller would believe. Containers is chosen because
// holzkube-manager does not call it -- if that ever changes, TestMethodCoverage fails
// and this test has to pick another.
func TestUnimplementedMethodIsUnimplemented(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{})
	cl := newMachineryClient(t, sim)

	_, err := cl.MachineClient.Containers(testContext(t), &machine.ContainersRequest{})
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("an unimplemented RPC returned %v, want Unimplemented; "+
			"a zero-valued success here would let a caller believe a method exists that does not", err)
	}
}

// TestRebootAndShutdownAreObservable pins the rest of the mutable state.
func TestRebootAndShutdownAreObservable(t *testing.T) {
	t.Parallel()

	clock := &fakeClock{now: time.Date(2026, 8, 29, 4, 0, 0, 0, time.UTC)}
	sim := newSim(t, talossim.Options{Hostname: "reboot-node", Now: clock.Now})
	cl := newMachineryClient(t, sim)
	ctx := testContext(t)

	before := sim.Node().LastBoot

	clock.advance(90 * time.Second)
	if _, err := cl.RebootWithResponse(ctx); err != nil {
		t.Fatalf("Reboot: %v", err)
	}

	after := sim.Node().LastBoot
	if !after.After(before) {
		t.Errorf("LastBoot did not move across a Reboot: before %s, after %s", before, after)
	}
	if got := sim.Node().Reboots; got != 1 {
		t.Errorf("Reboots = %d, want 1", got)
	}

	// The reboot is visible through an RPC as well as through the snapshot:
	// every service reports its last change as the node's last boot.
	services, err := cl.ServiceList(ctx)
	if err != nil {
		t.Fatalf("ServiceList: %v", err)
	}
	if got := services.GetMessages()[0].GetServices()[0].GetHealth().GetLastChange().AsTime(); !got.Equal(after) {
		t.Errorf("service last change = %s, want the last boot %s", got, after)
	}

	if _, err := cl.ShutdownWithResponse(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if !sim.Node().PoweredOff {
		t.Fatal("the node is not powered off after Shutdown")
	}

	_, err = cl.Version(ctx)
	if status.Code(err) != codes.Unavailable {
		t.Errorf("a powered-off node answered Version with %v, want Unavailable; "+
			"a machine that is off does not reply successfully", err)
	}
}

// TestResetWipesTheNode pins the one mutation that undoes another.
func TestResetWipesTheNode(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "wiped-node"})
	cl := newMachineryClient(t, sim)
	ctx := testContext(t)

	if err := cl.Bootstrap(ctx, &machine.BootstrapRequest{}); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if _, err := cl.ResetGenericWithResponse(ctx, &machine.ResetRequest{Graceful: true, Reboot: true}); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if sim.Node().Bootstrapped {
		t.Error("the node is still bootstrapped after a Reset wiped its etcd data directory")
	}
	if _, err := cl.EtcdMemberList(ctx, &machine.EtcdMemberListRequest{}); status.Code(err) != codes.FailedPrecondition {
		t.Errorf("EtcdMemberList after Reset returned %v, want FailedPrecondition", err)
	}

	// A bootstrap after a reset succeeds, because the reset really did clear
	// the state the first bootstrap set.
	if err := cl.Bootstrap(ctx, &machine.BootstrapRequest{}); err != nil {
		t.Errorf("Bootstrap after Reset: %v", err)
	}
}

// TestDryRunApplyChangesNothing pins the half of D-03 the node can prove: a
// dry-run apply is answered, and the node it was sent to is unchanged.
func TestDryRunApplyChangesNothing(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{})
	cl := newMachineryClient(t, sim)
	ctx := testContext(t)

	if _, err := cl.ApplyConfiguration(ctx, &machine.ApplyConfigurationRequest{
		Data:   []byte("version: v1alpha1\n"),
		DryRun: true,
	}); err != nil {
		t.Fatalf("ApplyConfiguration (dry run): %v", err)
	}
	if got := sim.Node().AppliedConfigs; got != 0 {
		t.Errorf("AppliedConfigs = %d after a dry run, want 0", got)
	}

	if _, err := cl.ApplyConfiguration(ctx, &machine.ApplyConfigurationRequest{
		Data: []byte("version: v1alpha1\n"),
	}); err != nil {
		t.Fatalf("ApplyConfiguration: %v", err)
	}
	if got := sim.Node().AppliedConfigs; got != 1 {
		t.Errorf("AppliedConfigs = %d after a real apply, want 1", got)
	}
}

// TestTwoNodesAreDistinguishable is the fan-out half of T-02-08-03: two
// simulated nodes answering the same call are told apart by the response, not
// by which client the caller happened to use.
func TestTwoNodesAreDistinguishable(t *testing.T) {
	t.Parallel()

	first := newSim(t, talossim.Options{Hostname: "node-a", NodeIP: "10.0.0.1"})
	second := newSim(t, talossim.Options{Hostname: "node-b", NodeIP: "10.0.0.2"})

	a, err := newMachineryClient(t, first).Version(testContext(t))
	if err != nil {
		t.Fatalf("Version of node-a: %v", err)
	}
	b, err := newMachineryClient(t, second).Version(testContext(t))
	if err != nil {
		t.Fatalf("Version of node-b: %v", err)
	}

	if a.GetMessages()[0].GetMetadata().GetHostname() == b.GetMessages()[0].GetMetadata().GetHostname() {
		t.Fatal("two different nodes returned the same identity; a caller fanning out cannot tell them apart")
	}
}

// fakeClock is the injected clock. The node stamps its state with it, so a test
// can say what "the last boot" was instead of asserting that one wall-clock
// reading is after another by an amount it cannot control.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.now
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.now = c.now.Add(d)
}
