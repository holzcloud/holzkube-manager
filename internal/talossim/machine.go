package talossim

import (
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

	reboots  int
	lastBoot time.Time

	resets     int
	poweredOff bool

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
}

func newNodeState(opts Options) *nodeState {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &nodeState{
		now:      now,
		hostname: opts.Hostname,
		version:  opts.TalosVersion,
		lastBoot: now(),
	}
}

// snapshot returns the current state as a value.
func (n *nodeState) snapshot() NodeState {
	n.mu.Lock()
	defer n.mu.Unlock()

	return NodeState{
		Hostname:       n.hostname,
		Version:        n.version,
		Bootstrapped:   n.bootstrapped,
		BootstrapCalls: n.bootstrapCalls,
		Reboots:        n.reboots,
		LastBoot:       n.lastBoot,
		Resets:         n.resets,
		PoweredOff:     n.poweredOff,
		AppliedConfigs: n.appliedConfigs,
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

// reboot restarts the node. etcd data survives a reboot, so bootstrapped does.
func (n *nodeState) reboot() {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.reboots++
	n.lastBoot = n.now()
}

// shutdown powers the node off.
func (n *nodeState) shutdown() {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.poweredOff = true
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

// SetHostname changes what the node reports as its hostname, as a
// configuration apply would on a real machine.
func (s *Server) SetHostname(hostname string) { s.node.setHostname(hostname) }

// SetVersion changes what the node reports as its Talos version, which is what
// an upgrade does.
func (s *Server) SetVersion(version string) { s.node.setVersion(version) }

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
func (m *machineService) Bootstrap(_ context.Context, _ *machine.BootstrapRequest) (*machine.BootstrapResponse, error) {
	if err := m.server.node.up(); err != nil {
		return nil, err
	}
	if err := m.server.node.bootstrap(); err != nil {
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
func (m *machineService) ApplyConfiguration(_ context.Context, req *machine.ApplyConfigurationRequest) (*machine.ApplyConfigurationResponse, error) {
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
func (m *machineService) Reboot(_ context.Context, _ *machine.RebootRequest) (*machine.RebootResponse, error) {
	if err := m.server.node.up(); err != nil {
		return nil, err
	}

	m.server.node.reboot()

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
func (m *machineService) Shutdown(_ context.Context, _ *machine.ShutdownRequest) (*machine.ShutdownResponse, error) {
	if err := m.server.node.up(); err != nil {
		return nil, err
	}

	resp := &machine.ShutdownResponse{
		Messages: []*machine.Shutdown{{
			Metadata: m.server.node.metadata(),
			ActorId:  actorID(),
		}},
	}

	m.server.node.shutdown()

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
