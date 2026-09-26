package talossim

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"hash/fnv"
	"runtime"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/siderolabs/talos/pkg/machinery/api/common"
	"github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/api/storage"
)

// nodeState is the mutable state of the simulated machine.
//
// It is the shape internal/auth's Limiter uses for its per-source state: one
// struct, one mutex, and every method taking the lock for the whole of its
// work. It lives on the Server rather than inside a handler closure because
// the scenario engine mutates it from outside a call -- a closure would make
// the state reachable only from the RPC that captured it.
//
// The rule this type exists to keep is that a mutation must be observable
// through a later read. A Bootstrap that only sets a flag nothing reports is
// indistinguishable from a Bootstrap that did nothing, and a simulator whose
// mutations are invisible is easier to satisfy than a real node.
type nodeState struct {
	mu sync.Mutex

	now func() time.Time

	hostname string
	version  string

	bootstrapped   bool
	bootstrapCalls int

	// uploadedSnapshot is what EtcdRecover has left on the node, and
	// recoveredFrom is what a recovery bootstrap then started etcd from.
	//
	// They are two fields and not one because uploading and recovering are two
	// operations on a real node, and the interesting failure lives between
	// them: a recovery bootstrap on a node that has no uploaded snapshot must
	// refuse, and a simulator with one field could not tell that state apart
	// from "recovered successfully".
	uploadedSnapshot []byte
	recoveredFrom    []byte
	recoverCalls     int

	reboots  int
	lastBoot time.Time

	// images is what ImagePull has put on this node. Upgrade refuses a
	// reference that is not here, which is what makes the two-call shape the
	// real LifecycleService requires testable rather than a claim in a
	// comment.
	images map[string]bool

	// removedMembers is what EtcdRemoveMemberByID has taken out of this node's
	// idea of the membership.
	removedMembers map[uint64]bool

	resets     int
	poweredOff bool

	// shutdowns and forcedShutdowns count Shutdown RPCs, the second only those
	// that asked Talos to skip its own cordon and drain; lastRebootMode is the
	// mode the last Reboot asked for. A force-stop and a stop reach the same
	// RPC, and the only difference between them is this one flag -- so a test
	// asserting which one happened has to be able to read the flag back.
	shutdowns       int
	forcedShutdowns int
	lastRebootMode  string

	// powerOns counts the times something outside the node switched it back
	// on -- Wake-on-LAN, in the product. stayDown makes the next Reboot leave
	// the node off instead of bringing it back, which is a node that does not
	// survive its reboot: the case a rolling restart has to stop at.
	powerOns int
	stayDown bool

	// stayUp makes a Reboot accepted and not performed: the node goes on
	// answering with the boot time it had. It is the few seconds on real
	// hardware between a node accepting a reboot and going down, stretched
	// out -- the window in which "it answers" is not "it is back".
	stayUp bool

	// bootTakes is Options.BootTakes.
	bootTakes time.Duration

	appliedConfigs int
}

// NodeState is a snapshot of the simulated machine's mutable state.
//
// It is a value rather than a live view on purpose: a caller holding it cannot
// race the server, and a test comparing two snapshots is comparing two facts
// rather than one pointer with itself.
type NodeState struct {
	// Hostname and Version are what the node currently reports. They are not
	// the Options it was built with: a scenario may change either.
	Hostname string
	Version  string

	// Bootstrapped is whether etcd has been started on this node.
	Bootstrapped bool

	// BootstrapCalls counts every Bootstrap RPC, including the ones refused
	// because the node was already bootstrapped. A refusal is still a call,
	// and a test asserting that the second one was refused needs to see it.
	BootstrapCalls int

	// Reboots counts completed Reboot RPCs; LastBoot is when the node last
	// came up, which is what the service list reports as each service's last
	// state change.
	Reboots  int
	LastBoot time.Time

	// Resets counts completed Reset RPCs. PoweredOff is true once the node has
	// been shut down, or reset without a reboot; a powered-off node answers
	// Unavailable rather than answering normally.
	Resets     int
	PoweredOff bool

	// AppliedConfigs counts configurations actually applied. A dry-run apply
	// is not counted, because a dry run that changed the node would not be one.
	AppliedConfigs int

	// Shutdowns counts Shutdown RPCs and ForcedShutdowns the ones that asked
	// Talos to skip its own cordon and drain. LastRebootMode is the mode the
	// last Reboot asked for: "DEFAULT", "POWERCYCLE" or "FORCE", empty before
	// the first one. PowerOns counts the times the node was switched back on
	// from outside, which is what Wake-on-LAN does.
	Shutdowns       int
	ForcedShutdowns int
	LastRebootMode  string
	PowerOns        int

	// RecoverCalls counts EtcdRecover uploads. UploadedSnapshot is what the
	// last one left on the node; RecoveredFrom is what a recovery bootstrap
	// then started etcd from.
	//
	// The last two are separate for the reason acceptSnapshot gives: an upload
	// that succeeded and a recovery that was never asked for is a cluster
	// running its old etcd while somebody believes it was restored, and a test
	// can only tell those apart if the simulator can.
	RecoverCalls     int
	UploadedSnapshot []byte
	RecoveredFrom    []byte
}

func newNodeState(opts Options) *nodeState {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &nodeState{
		now:          now,
		hostname:     opts.Hostname,
		version:      opts.TalosVersion,
		lastBoot:     now(),
		bootstrapped: opts.Bootstrapped,
		bootTakes:    opts.BootTakes,
	}
}

// snapshot returns the current state as a value.
// pullImage records that an image is on the node.
func (n *nodeState) pullImage(ref string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.images == nil {
		n.images = map[string]bool{}
	}
	n.images[ref] = true
}

// hasImage reports whether an image has been pulled.
func (n *nodeState) hasImage(ref string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.images[ref]
}

// upgradeTo is what makes an upgrade observable.
//
// It changes what Version reports, and the reason it is a state change rather
// than a streamed line is the whole of UPG-07: a node that streamed an
// upgrade's output and then reported the same version it reported before has
// not upgraded, and nothing about the stream says so. A simulator that only
// streamed could not tell the two apart, and neither could a test written
// against it.
//
// The reboot counter advances too, because a node that has installed a new
// system reboots into it -- and the uptime that follows from it is what the
// reboot job's own verification reads.
func (n *nodeState) upgradeTo(version string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.version = version
	n.reboots++
	n.lastBoot = n.now().Add(n.bootTakes)
}

// removeMember drops a member from this node's idea of the etcd membership.
func (n *nodeState) removeMember(id uint64) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.removedMembers == nil {
		n.removedMembers = map[uint64]bool{}
	}
	n.removedMembers[id] = true
}

// leaveEtcd stops etcd on this node.
//
// The node stops reporting itself as bootstrapped, which makes every Etcd*
// read refuse exactly as it does on a node that was never bootstrapped. Those
// two states are genuinely indistinguishable from outside on a real node, and
// a simulator that distinguished them would let a test rely on a difference
// that is not there.
func (n *nodeState) leaveEtcd() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.bootstrapped = false
}

func (n *nodeState) snapshot() NodeState {
	n.mu.Lock()
	defer n.mu.Unlock()

	return NodeState{
		Hostname:        n.hostname,
		Version:         n.version,
		Bootstrapped:    n.bootstrapped,
		BootstrapCalls:  n.bootstrapCalls,
		Reboots:         n.reboots,
		LastBoot:        n.lastBoot,
		Resets:          n.resets,
		PoweredOff:      n.poweredOff,
		AppliedConfigs:  n.appliedConfigs,
		RecoverCalls:    n.recoverCalls,
		Shutdowns:       n.shutdowns,
		ForcedShutdowns: n.forcedShutdowns,
		LastRebootMode:  n.lastRebootMode,
		PowerOns:        n.powerOns,
		// Cloned, because NodeState promises a caller holding it cannot race
		// the server, and a shared backing array is exactly that race.
		UploadedSnapshot: bytes.Clone(n.uploadedSnapshot),
		RecoveredFrom:    bytes.Clone(n.recoveredFrom),
	}
}

// metadata is the response envelope every RPC carries.
//
// Talos puts the answering node's hostname here so that a client fanning one
// call across several nodes can tell the replies apart. The simulator does the
// same, which is what lets a test assert that the right node answered rather
// than only that an answer arrived (T-02-08-03).
func (n *nodeState) metadata() *common.Metadata {
	n.mu.Lock()
	defer n.mu.Unlock()

	return &common.Metadata{Hostname: n.hostname}
}

// up reports whether the node is answering.
//
// A node that has been shut down does not reply with a successful zero value;
// it does not reply at all. Unavailable is the closest honest equivalent, and
// it is deliberately not Unimplemented: the method coverage guard reads that
// code as "the simulator has drifted", and a powered-off node is a state, not
// a drift.
func (n *nodeState) up() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.poweredOff {
		return status.Errorf(codes.Unavailable, "talossim: %s is powered off", n.hostname)
	}
	return nil
}

// bootstrap starts etcd on the node.
//
// A second bootstrap of a node that already has an etcd data directory fails
// on a real machine, so it fails here. Returning success twice would make the
// simulator easier to satisfy than the hardware, which moves the surprise from
// the test suite to the cluster.
func (n *nodeState) bootstrap() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.bootstrapCalls++

	if n.bootstrapped {
		return status.Errorf(codes.AlreadyExists,
			"talossim: %s is already bootstrapped: etcd data directory is not empty", n.hostname)
	}

	n.bootstrapped = true
	return nil
}

// acceptSnapshot stores what an EtcdRecover upload delivered.
//
// It does not start etcd and does not mark the node bootstrapped, because
// EtcdRecover on a real node does neither: it writes the file and stops. A
// simulator that recovered here would hide the failure mode this pair exists
// to make testable -- an upload that succeeded and a recovery that was never
// asked for, which leaves a cluster running its old etcd while somebody
// believes it was restored.
func (n *nodeState) acceptSnapshot(b []byte) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.recoverCalls++
	n.uploadedSnapshot = b
}

// recoverBootstrap starts etcd from an uploaded snapshot.
//
// Unlike an ordinary bootstrap it is allowed on a node that already has an
// etcd data directory -- replacing that directory is the entire operation. It
// refuses a node with no uploaded snapshot, which is what a real node does:
// there is nothing to recover from.
func (n *nodeState) recoverBootstrap(skipHashCheck bool) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.bootstrapCalls++

	if n.uploadedSnapshot == nil {
		return status.Errorf(codes.FailedPrecondition,
			"talossim: %s has no uploaded snapshot to recover from; send one with EtcdRecover first",
			n.hostname)
	}

	// The integrity check a real node makes, and the reason skipHashCheck
	// exists. A snapshot taken through the etcd API carries a hash; one copied
	// off a data directory does not, and Talos asks the caller to say which it
	// is rather than guessing. The simulator's stand-in for "has a hash" is the
	// prefix EtcdSnapshot writes, which is the only thing about these bytes
	// that is ever true.
	if !skipHashCheck && !bytes.HasPrefix(n.uploadedSnapshot, []byte(snapshotPrefix)) {
		return status.Errorf(codes.InvalidArgument,
			"talossim: the uploaded snapshot fails its integrity check on %s", n.hostname)
	}

	n.recoveredFrom = n.uploadedSnapshot
	n.bootstrapped = true
	return nil
}

// reboot restarts the node. etcd data survives a reboot, so bootstrapped does.
//
// A node told to stay down goes off instead, and stays off until something
// switches it on: it is the node whose reboot did not bring it back.
func (n *nodeState) reboot(mode string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.reboots++
	n.lastRebootMode = mode
	switch {
	case n.stayDown:
		n.poweredOff = true
	case n.stayUp:
		// Accepted, and nothing happened.
	default:
		n.lastBoot = n.now().Add(n.bootTakes)
	}
}

// shutdown powers the node off.
func (n *nodeState) shutdown(force bool) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.shutdowns++
	if force {
		n.forcedShutdowns++
	}
	n.poweredOff = true
}

// powerOn is the node being switched on from outside. It boots, so the boot
// time moves; a node that was already on is left exactly as it was, which is
// what a Wake-on-LAN packet does to a machine that is running.
func (n *nodeState) powerOn() {
	n.mu.Lock()
	defer n.mu.Unlock()

	if !n.poweredOff {
		return
	}
	n.powerOns++
	n.poweredOff = false
	n.lastBoot = n.now().Add(n.bootTakes)
}

func (n *nodeState) setStayDown(v bool) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.stayDown = v
}

func (n *nodeState) setStayUp(v bool) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.stayUp = v
}

// reset wipes the node. A reset erases the etcd data directory, so the node
// comes back needing a bootstrap; whether it comes back at all depends on the
// reboot flag the caller sent.
func (n *nodeState) reset(reboot bool) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.resets++
	n.bootstrapped = false
	n.appliedConfigs = 0

	if reboot {
		n.reboots++
		n.lastBoot = n.now()
	} else {
		n.poweredOff = true
	}
}

// applyConfig records an applied machine configuration. A dry run is not an
// application and is not recorded by the caller.
func (n *nodeState) applyConfig() {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.appliedConfigs++
}

// setBootstrapped sets the flag directly, without counting a Bootstrap call.
//
// Injection is not an RPC. Routing the second_bootstrap scenario's precondition
// through nodeState.bootstrap would inflate BootstrapCalls, and a test asserting
// that the client called Bootstrap once would then be reading the scenario's own
// setup back as evidence about the client.
func (n *nodeState) setBootstrapped(v bool) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.bootstrapped = v
}

func (n *nodeState) setHostname(hostname string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.hostname = hostname
}

func (n *nodeState) setVersion(version string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.version = version
}

// Node returns a snapshot of the simulated machine's mutable state.
//
// It is the seam the scenario engine reads through: a scenario asserting that
// a mutation happened asks the node, not the response it just received, so the
// assertion survives a handler that returns a plausible reply while changing
// nothing.
func (s *Server) Node() NodeState { return s.node.snapshot() }

// SetVersion changes what the node reports as its Talos version, which is what
// an upgrade does.
func (s *Server) SetVersion(version string) { s.node.setVersion(version) }

// PowerOn switches a node that is off back on, which is what a Wake-on-LAN
// packet does to a real one. It is a method on the server rather than an RPC
// because a machine that is off has no API to call: whatever wakes it arrives
// from outside, and a test's stand-in for the magic packet calls this.
func (s *Server) PowerOn() { s.node.powerOn() }

// StayDownOnReboot makes every later Reboot leave the node off instead of
// bringing it back, until it is switched on again. It is the node a rolling
// restart has to stop at: one whose reboot was accepted and which never came
// back.
func (s *Server) StayDownOnReboot(v bool) { s.node.setStayDown(v) }

// StayUpOnReboot makes every later Reboot accepted and not performed: the node
// keeps answering, with the boot time it had. On hardware that is the seconds
// between a node accepting a reboot and going down; here it lasts, so that a
// test can tell a wait that asks "has it rebooted" from one that only asks
// "does it answer".
func (s *Server) StayUpOnReboot(v bool) { s.node.setStayUp(v) }

// actorID is the identifier Talos returns for an asynchronous action so that a
// caller can correlate the reply with the events the action goes on to emit.
func actorID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand does not fail on any platform this runs on, and a
		// correlation identifier is not a security boundary. Degrading to a
		// fixed value keeps the simulator from having an error path that a
		// real node does not have.
		return "00000000000000000000000000000000"
	}
	return hex.EncodeToString(b[:])
}

// registerNodeServices registers the node's API surface on the gRPC server.
//
// MachineService and StorageService go on the same server, because the
// production client creates both stubs on one connection: c.MachineClient and
// c.StorageClient are built from the same *grpc.ClientConn. Splitting them
// across two servers would let a hand-written test pass against a topology the
// real client cannot reach.
func (s *Server) registerNodeServices(srv *grpc.Server) {
	machine.RegisterMachineServiceServer(srv, &machineService{server: s})

	// The third service, added in phase 9. Talos v1.13 moved install and
	// upgrade off MachineService onto LifecycleService, and the streaming
	// upgrade is the one internal/talos calls -- so a simulator that served
	// only the first two would answer Unimplemented to the one RPC the
	// upgrade path depends on.
	machine.RegisterLifecycleServiceServer(srv, &lifecycleService{server: s})
	storage.RegisterStorageServiceServer(srv, &storageService{server: s})
}

// machineService is the simulated node's MachineService.
//
// It embeds machine.UnimplementedMachineServiceServer, so every one of the 54
// RPCs this milestone does not reach answers Unimplemented rather than failing
// to compile. That is the honest default for a surface this wide: the methods
// holzkube-manager actually calls are implemented one at a time, and an unimplemented
// one shows up as a clear gRPC status instead of as a zero-valued success that
// a test would read as working.
//
// Which methods are implemented is not a judgement call that can drift:
// TestMethodCoverage walks internal/talos for machinery-client call sites and
// fails when one of them lands on an inherited method.
type machineService struct {
	machine.UnimplementedMachineServiceServer

	server *Server
}

// storageService is the simulated node's StorageService.
//
// Disks lives here rather than on MachineService -- that is where the resolved
// machinery module puts it, and client.Client.Disks calls it through
// c.StorageClient. The plan named it among the MachineService methods; the
// module is what the production client speaks, so the module wins.
type storageService struct {
	storage.UnimplementedStorageServiceServer

	server *Server
}

// Version answers with the version the node currently reports.
//
// It reads live node state rather than the Options the server was built with,
// because an upgrade changes the answer and a simulator that kept reporting
// its construction argument could not be used to test one.
func (m *machineService) Version(_ context.Context, _ *emptypb.Empty) (*machine.VersionResponse, error) {
	if err := m.server.node.up(); err != nil {
		return nil, err
	}

	n := m.server.node.snapshot()

	return &machine.VersionResponse{
		Messages: []*machine.Version{{
			Metadata: m.server.node.metadata(),
			Version: &machine.VersionInfo{
				Tag:       n.Version,
				GoVersion: runtime.Version(),
				Os:        "linux",
				Arch:      runtime.GOARCH,
			},
			Platform: &machine.PlatformInfo{Name: "metal", Mode: "metal"},
		}},
	}, nil
}

// Hostname answers with the hostname the node currently reports.
func (m *machineService) Hostname(_ context.Context, _ *emptypb.Empty) (*machine.HostnameResponse, error) {
	if err := m.server.node.up(); err != nil {
		return nil, err
	}

	n := m.server.node.snapshot()

	return &machine.HostnameResponse{
		Messages: []*machine.Hostname{{
			Metadata: m.server.node.metadata(),
			Hostname: n.Hostname,
		}},
	}, nil
}

// ServiceList reports the node's services.
//
// It is one of the two reads a bootstrap is observable through: etcd is
// Preparing and unhealthy on a node that has not been bootstrapped and Running
// and healthy on one that has, and every service's last state change is the
// node's last boot -- so a reboot moves the timestamps as it does on hardware.
func (m *machineService) ServiceList(_ context.Context, _ *emptypb.Empty) (*machine.ServiceListResponse, error) {
	if err := m.server.node.up(); err != nil {
		return nil, err
	}

	n := m.server.node.snapshot()
	boot := timestamppb.New(n.LastBoot)

	service := func(id string, running bool, message string) *machine.ServiceInfo {
		state := "Preparing"
		if running {
			state = "Running"
		}
		return &machine.ServiceInfo{
			Id:    id,
			State: state,
			Events: &machine.ServiceEvents{Events: []*machine.ServiceEvent{{
				Msg:   message,
				State: state,
				Ts:    boot,
			}}},
			Health: &machine.ServiceHealth{
				Healthy:     running,
				LastMessage: message,
				LastChange:  boot,
			},
		}
	}

	etcdMessage := "etcd is waiting for the cluster to be bootstrapped"
	if n.Bootstrapped {
		etcdMessage = "etcd is a member of the cluster"
	}

	// k8s_down is a Kubernetes failure on a node that is otherwise healthy, so
	// only the kubelet stops running: machined, apid and etcd carry on. A
	// scenario that took the whole service list down would be indistinguishable
	// from a node that had gone away.
	kubeletRunning := n.Bootstrapped
	kubeletMessage := "kubelet is running"
	if m.server.kubernetesIsDown() {
		kubeletRunning = false
		kubeletMessage = "kubelet is not running: failed to start, see logs"
	}

	return &machine.ServiceListResponse{
		Messages: []*machine.ServiceList{{
			Metadata: m.server.node.metadata(),
			Services: []*machine.ServiceInfo{
				service("machined", true, "service started"),
				service("apid", true, "listening on :50000"),
				service("kubelet", kubeletRunning, kubeletMessage),
				service("etcd", n.Bootstrapped, etcdMessage),
			},
		}},
	}, nil
}

// Bootstrap starts etcd on the node.
//
// A node that already has an etcd data directory refuses, exactly as a real
// one does. Answering a second bootstrap with success would make this
// simulator easier to satisfy than the hardware it stands in for.
// SystemStat answers with the node's boot time.
//
// It exists here because the product asks for it, and it asks for exactly one
// field: a reboot job's paired "did it happen?" check is "has this node been
// up for less time than the request has existed". The simulator therefore has
// to move lastBoot when it reboots, which it already does -- which is what
// makes the check testable at all rather than only assertable against
// hardware.
func (m *machineService) SystemStat(_ context.Context, _ *emptypb.Empty) (*machine.SystemStatResponse, error) {
	state := m.server.node.snapshot()

	return &machine.SystemStatResponse{
		Messages: []*machine.SystemStat{{
			Metadata: m.server.node.metadata(),
			//nolint:gosec // a boot time in seconds since the epoch cannot be negative
			BootTime: uint64(state.LastBoot.Unix()),
		}},
	}, nil
}

func (m *machineService) Bootstrap(_ context.Context, req *machine.BootstrapRequest) (*machine.BootstrapResponse, error) {
	if err := m.server.node.up(); err != nil {
		return nil, err
	}

	// Two operations behind one RPC, and the simulator keeps them apart for
	// the reason the product's two client methods do: an ordinary bootstrap
	// refuses a node that already has etcd, and a recovery bootstrap is
	// defined by replacing it.
	if req.GetRecoverEtcd() {
		if err := m.server.node.recoverBootstrap(req.GetRecoverSkipHashCheck()); err != nil {
			return nil, err
		}
	} else if err := m.server.node.bootstrap(); err != nil {
		return nil, err
	}

	return &machine.BootstrapResponse{
		Messages: []*machine.Bootstrap{{Metadata: m.server.node.metadata()}},
	}, nil
}

// EtcdMemberList reports the etcd members this node knows about.
//
// On a node that has not been bootstrapped etcd is not running, and a real
// node answers that with an error rather than with an empty list -- an empty
// list is a cluster with no members, which is a different fact.
func (m *machineService) EtcdMemberList(_ context.Context, _ *machine.EtcdMemberListRequest) (*machine.EtcdMemberListResponse, error) {
	if err := m.server.node.up(); err != nil {
		return nil, err
	}

	n := m.server.node.snapshot()
	if !n.Bootstrapped {
		return nil, status.Errorf(codes.FailedPrecondition,
			"talossim: etcd is not running on %s: the node has not been bootstrapped", n.Hostname)
	}

	return &machine.EtcdMemberListResponse{
		Messages: []*machine.EtcdMembers{{
			Metadata:      m.server.node.metadata(),
			LegacyMembers: []string{n.Hostname},
			Members: []*machine.EtcdMember{{
				Id:         memberID(n.Hostname),
				Hostname:   n.Hostname,
				PeerUrls:   []string{"https://" + m.server.opts.NodeIP + ":2380"},
				ClientUrls: []string{"https://" + m.server.opts.NodeIP + ":2379"},
			}},
		}},
	}, nil
}

// EtcdStatus reports this node's own etcd member status.
func (m *machineService) EtcdStatus(_ context.Context, _ *emptypb.Empty) (*machine.EtcdStatusResponse, error) {
	if err := m.server.node.up(); err != nil {
		return nil, err
	}

	n := m.server.node.snapshot()
	if !n.Bootstrapped {
		return nil, status.Errorf(codes.FailedPrecondition,
			"talossim: etcd is not running on %s: the node has not been bootstrapped", n.Hostname)
	}

	id := memberID(n.Hostname)

	// The raft index advances with each boot. It is a monotonic counter on a
	// real node, and the conversion cannot overflow because Reboots only grows
	// by one per Reboot RPC.
	raftIndex := uint64(max(n.Reboots, 0)) + 1

	return &machine.EtcdStatusResponse{
		Messages: []*machine.EtcdStatus{{
			Metadata: m.server.node.metadata(),
			MemberStatus: &machine.EtcdMemberStatus{
				MemberId:         id,
				Leader:           id,
				ProtocolVersion:  "3.5.0",
				StorageVersion:   "3.6.0",
				DbSize:           20 << 20,
				DbSizeInUse:      4 << 20,
				RaftIndex:        raftIndex,
				RaftTerm:         raftIndex,
				RaftAppliedIndex: raftIndex,
			},
		}},
	}, nil
}

// ApplyConfiguration applies a machine configuration.
//
// A dry run is answered but not recorded: an apply that changed the node in
// dry-run mode would not be a dry run, and D-03 makes "no mutation reached the
// node" a property the transport has to be able to prove here.
func (m *machineService) ApplyConfiguration(ctx context.Context, req *machine.ApplyConfigurationRequest) (*machine.ApplyConfigurationResponse, error) {
	if err := m.server.node.up(); err != nil {
		return nil, err
	}
	if len(req.GetData()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "talossim: empty machine configuration")
	}

	details := "configuration applied"
	if req.GetDryRun() {
		details = "dry run: configuration validated, nothing written"
	} else {
		m.server.node.applyConfig()
		// The configuration becomes the node's ACTIVE configuration, and not
		// just a counter. Ledger 3 records the previous behaviour -- the apply
		// was counted and the bytes were dropped -- and what that cost is
		// specific: a test of any operation that writes configuration and then
		// reads it back proved only that an RPC had been issued. The CA
		// rotation is exactly that shape, four times over, so the simulator
		// has to be able to say what a node now trusts.
		// Only a node that HAS a configuration adopts one. A simulator built
		// without a Cluster models a machine with no cluster PKI: it serves no
		// MachineConfig resource, so there is nothing to replace and nothing to
		// serve back, and it keeps the old behaviour of counting the apply.
		// That is also the surface the provisioning path uses, which applies
		// through maintenance mode rather than here.
		if m.server.opts.Cluster != nil {
			if err := m.server.setMachineConfig(ctx, req.GetData()); err != nil {
				return nil, status.Error(codes.InvalidArgument, err.Error())
			}
		}
	}

	return &machine.ApplyConfigurationResponse{
		Messages: []*machine.ApplyConfiguration{{
			Metadata:    m.server.node.metadata(),
			Mode:        req.GetMode(),
			ModeDetails: details,
		}},
	}, nil
}

// Reboot restarts the node. etcd data survives, so a bootstrapped node comes
// back bootstrapped; what moves is the last-boot timestamp the service list
// reports.
func (m *machineService) Reboot(_ context.Context, req *machine.RebootRequest) (*machine.RebootResponse, error) {
	if err := m.server.node.up(); err != nil {
		return nil, err
	}

	m.server.node.reboot(req.GetMode().String())

	// ip_changes_on_reboot models a DHCP lease that did not survive the reboot.
	// The rebind happens before the reply is written and severs this connection
	// along with the listener, because a machine that is rebooting has gone
	// away -- a client whose connection survived the reboot would never observe
	// the address change, and the scenario would be inert against exactly the
	// caller it exists to test. The caller therefore sees either the reply or a
	// transport failure, and the address the node left behind refuses
	// connections from here on. See rebind in scenario_conn.go.
	if _, ok := m.server.activeScenario(ScenarioIPChangesOnReboot); ok {
		if err := m.server.rebind(); err != nil {
			return nil, status.Errorf(codes.Internal, "talossim: rebind after reboot: %v", err)
		}
	}

	return &machine.RebootResponse{
		Messages: []*machine.Reboot{{
			Metadata: m.server.node.metadata(),
			ActorId:  actorID(),
		}},
	}, nil
}

// Shutdown powers the node off. Afterwards it answers Unavailable, because a
// machine that is off does not answer at all.
func (m *machineService) Shutdown(_ context.Context, req *machine.ShutdownRequest) (*machine.ShutdownResponse, error) {
	if err := m.server.node.up(); err != nil {
		return nil, err
	}

	resp := &machine.ShutdownResponse{
		Messages: []*machine.Shutdown{{
			Metadata: m.server.node.metadata(),
			ActorId:  actorID(),
		}},
	}

	m.server.node.shutdown(req.GetForce())

	return resp, nil
}

// Reset wipes the node. The etcd data directory goes with it, so the node
// needs bootstrapping again; whether it comes back at all is the request's
// reboot flag.
func (m *machineService) Reset(_ context.Context, req *machine.ResetRequest) (*machine.ResetResponse, error) {
	if err := m.server.node.up(); err != nil {
		return nil, err
	}

	resp := &machine.ResetResponse{
		Messages: []*machine.Reset{{
			Metadata: m.server.node.metadata(),
			ActorId:  actorID(),
		}},
	}

	m.server.node.reset(req.GetReboot())

	return resp, nil
}

// Disks reports the node's block devices.
//
// It is on StorageService, which is why this method hangs off storageService
// and not off machineService: c.Disks() on the production client goes to
// c.StorageClient.
func (t *storageService) Disks(_ context.Context, _ *emptypb.Empty) (*storage.DisksResponse, error) {
	if err := t.server.node.up(); err != nil {
		return nil, err
	}

	return &storage.DisksResponse{
		Messages: []*storage.Disks{{
			Metadata: t.server.node.metadata(),
			Disks: []*storage.Disk{
				{
					DeviceName: "/dev/nvme0n1",
					Model:      "TALOSSIM NVMe",
					Serial:     "SIM0000000001",
					Size:       512 << 30,
					Type:       storage.Disk_NVME,
					BusPath:    "/pci0000:00/0000:00:1c.4/0000:03:00.0/nvme/nvme0/nvme0n1",
					Subsystem:  "/sys/class/block",
					SystemDisk: true,
				},
				{
					DeviceName: "/dev/sda",
					Model:      "TALOSSIM HDD",
					Serial:     "SIM0000000002",
					Size:       4 << 40,
					Type:       storage.Disk_HDD,
					BusPath:    "/pci0000:00/0000:00:17.0/ata1/host0/target0:0:0/0:0:0:0",
					Subsystem:  "/sys/class/block",
				},
			},
		}},
	}, nil
}

// memberID derives a stable etcd member identifier from the node's hostname.
//
// Real member IDs are assigned by etcd and are meaningless numbers; what a
// caller relies on is that the same member keeps the same one, which a hash of
// the hostname gives without the simulator having to keep a registry.
func memberID(hostname string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(hostname))
	return h.Sum64()
}
