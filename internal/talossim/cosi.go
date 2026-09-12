package talossim

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"

	cosiapi "github.com/cosi-project/runtime/api/v1alpha1"
	"github.com/cosi-project/runtime/pkg/safe"
	"github.com/cosi-project/runtime/pkg/state"
	"github.com/cosi-project/runtime/pkg/state/impl/inmem"
	"github.com/cosi-project/runtime/pkg/state/impl/namespaced"
	cosiserver "github.com/cosi-project/runtime/pkg/state/protobuf/server"
	"google.golang.org/grpc"

	"github.com/siderolabs/talos/pkg/machinery/config/configloader"
	machinetype "github.com/siderolabs/talos/pkg/machinery/config/machine"
	"github.com/siderolabs/talos/pkg/machinery/nethelpers"
	"github.com/siderolabs/talos/pkg/machinery/resources/block"
	"github.com/siderolabs/talos/pkg/machinery/resources/cluster"
	configres "github.com/siderolabs/talos/pkg/machinery/resources/config"
	"github.com/siderolabs/talos/pkg/machinery/resources/hardware"
	"github.com/siderolabs/talos/pkg/machinery/resources/k8s"
	"github.com/siderolabs/talos/pkg/machinery/resources/network"
	runtimeres "github.com/siderolabs/talos/pkg/machinery/resources/runtime"

	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// COSI returns the node's resource state.
//
// It is the read surface holzkube-manager actually uses: client.Client.COSI is a
// state.State, it supports Watch, and the resources that carry a node's
// identity, addresses, disks and services all live there rather than behind a
// typed RPC. A test seeds through this accessor and the production client
// reads what it seeded over the wire; a scenario that has to remove a resource
// -- k8s_down in plan 02-03 -- removes it through the same one.
func (s *Server) COSI() state.State { return s.cosi }

// registerCOSI builds the in-memory resource state and registers it on the
// gRPC server.
//
// srv is the same server MachineService is registered on, and that is
// load-bearing rather than tidy: the production client builds its COSI adapter
// from the one *grpc.ClientConn it dialled, so a state server living on a
// second listener would be reachable by a hand-written test and invisible to
// the client the product ships. That divergence -- the simulator passing a
// test the real client could not -- is the whole thing TRANS-06 exists to
// prevent.
func (s *Server) registerCOSI(srv *grpc.Server) {
	s.cosi = state.WrapCore(namespaced.NewState(inmem.Build))

	cosiapi.RegisterStateServer(srv, cosiserver.NewState(s.cosi))
}

// SetHostname changes what the node calls itself, the way a configuration
// apply does on a real machine.
//
// Both halves, and the second is the one that matters. A real node's hostname
// lives in network.HostnameStatus; the hostname in each RPC's response
// envelope is a copy Talos fills in from there, and every reader in this
// product that wants a hostname reads the resource (facts.go). A simulator
// that changed only the envelope would let a test assert a rename that no
// production reader would ever have seen -- the simulator passing a test the
// real node could not, which is what TRANS-06 exists against.
func (s *Server) SetHostname(ctx context.Context, hostname string) error {
	s.node.setHostname(hostname)

	_, err := safe.StateUpdateWithConflicts(ctx, s.COSI(),
		network.NewHostnameStatus(network.NamespaceName, network.HostnameID).Metadata(),
		func(r *network.HostnameStatus) error {
			r.TypedSpec().Hostname = hostname
			return nil
		})
	if err != nil {
		return fmt.Errorf("talossim: set hostname to %q: %w", hostname, err)
	}
	return nil
}

// seedCOSI puts the resources a freshly booted node would already have into
// the state.
//
// It goes through Server.COSI rather than through the state value directly, so
// that seeding and a scenario's mutation are the same operation against the
// same object. A read against an unseeded node returns NotFound, which is a
// truthful answer but not a useful starting point: the point of the simulator
// is that a caller can ask it something before the test has done anything.
func (s *Server) seedCOSI(ctx context.Context) error {
	info := hardware.NewSystemInformation(hardware.SystemInformationID)
	info.TypedSpec().Manufacturer = "talossim"
	info.TypedSpec().ProductName = "simulated node"
	info.TypedSpec().Version = s.opts.TalosVersion
	info.TypedSpec().SerialNumber = "SIM-" + s.opts.Hostname
	info.TypedSpec().UUID = nodeUUID(s.opts.Hostname)

	if err := s.COSI().Create(ctx, info); err != nil {
		return fmt.Errorf("talossim: seed %s: %w", hardware.SystemInformationType, err)
	}

	addr, err := netip.ParseAddr(s.opts.NodeIP)
	if err != nil {
		return fmt.Errorf("talossim: node address %q: %w", s.opts.NodeIP, err)
	}

	bits := 32
	if addr.Is6() {
		bits = 128
	}

	nodeAddr := network.NewNodeAddress(network.NamespaceName, network.NodeAddressDefaultID)
	nodeAddr.TypedSpec().Addresses = []netip.Prefix{netip.PrefixFrom(addr, bits)}
	nodeAddr.TypedSpec().SortAlgorithm = nethelpers.AddressSortAlgorithmV2

	if err := s.COSI().Create(ctx, nodeAddr); err != nil {
		return fmt.Errorf("talossim: seed %s: %w", network.NodeAddressType, err)
	}

	hostname := network.NewHostnameStatus(network.NamespaceName, network.HostnameID)
	hostname.TypedSpec().Hostname = s.opts.Hostname

	if err := s.COSI().Create(ctx, hostname); err != nil {
		return fmt.Errorf("talossim: seed %s: %w", network.HostnameStatusType, err)
	}

	if err := s.seedHardware(ctx); err != nil {
		return err
	}
	if err := s.seedKernelCmdline(ctx); err != nil {
		return err
	}
	if err := s.seedExtensions(ctx); err != nil {
		return err
	}
	if err := s.seedCluster(ctx); err != nil {
		return err
	}

	return s.seedKubernetes(ctx)
}

// seedHardware puts the inventory facts INV-06 asks for into the state.
//
// They are COSI resources rather than answers to Memory() and CPUInfo() calls
// because that is where a real node keeps them, and because a dashboard that
// re-asked those RPCs on a timer would be the blind polling INV-13 rules out
// (D-19). The two halves -- what the simulator serves and what
// talos.ClusterClient reads -- grow together or the field is not testable.
func (s *Server) seedHardware(ctx context.Context) error {
	for i := range s.opts.CPUs {
		cpu := hardware.NewProcessorInfo(strconv.Itoa(i))
		cpu.TypedSpec().Manufacturer = "SimuCorp"
		cpu.TypedSpec().ProductName = "Simulated CPU"
		cpu.TypedSpec().CoreCount = 4
		cpu.TypedSpec().ThreadCount = 8
		cpu.TypedSpec().MaxSpeed = 3600

		if err := s.COSI().Create(ctx, cpu); err != nil {
			return fmt.Errorf("talossim: seed %s: %w", hardware.ProcessorType, err)
		}
	}

	// One module carrying the whole capacity. A real board reports several and
	// holzkube-manager sums them; one module exercises the same sum with one term.
	mem := hardware.NewMemoryModuleInfo("DIMM0")
	mem.TypedSpec().Size = uint32(s.opts.MemoryMiB) //nolint:gosec // fixture sizes are far below the uint32 ceiling
	mem.TypedSpec().Manufacturer = "SimuCorp"
	mem.TypedSpec().DeviceLocator = "DIMM0"

	if err := s.COSI().Create(ctx, mem); err != nil {
		return fmt.Errorf("talossim: seed %s: %w", hardware.MemoryModuleType, err)
	}

	for _, d := range s.opts.Disks {
		disk := block.NewDisk(block.NamespaceName, d.Device)
		disk.TypedSpec().DevPath = "/dev/" + d.Device
		disk.TypedSpec().Size = d.Size
		disk.TypedSpec().PrettySize = prettySize(d.Size)
		disk.TypedSpec().Model = d.Model
		disk.TypedSpec().Serial = d.Serial
		disk.TypedSpec().Transport = d.Transport

		if err := s.COSI().Create(ctx, disk); err != nil {
			return fmt.Errorf("talossim: seed %s: %w", block.DiskType, err)
		}
	}

	link := network.NewLinkStatus(network.NamespaceName, "eth0")
	link.TypedSpec().Type = nethelpers.LinkEther
	link.TypedSpec().LinkState = true
	link.TypedSpec().MTU = 1500
	link.TypedSpec().SpeedMegabits = 1000
	link.TypedSpec().Driver = "simnet"
	link.TypedSpec().HardwareAddr = nethelpers.HardwareAddr(hardwareAddr(s.opts.Hostname))

	if err := s.COSI().Create(ctx, link); err != nil {
		return fmt.Errorf("talossim: seed %s: %w", network.LinkStatusType, err)
	}

	return nil
}

// seedExtensions puts the node's system extensions into the state, including
// the virtual "schematic" one that carries the Image Factory schematic id.
//
// A node with no SchematicID configured gets no such resource at all, and that
// absence is the point: it is what a node not installed from a Factory image
// looks like, and holzkube-manager has to report it as "none" rather than derive one
// from the Talos version (D-12).
// seedKernelCmdline puts the node's own boot command line into the state.
//
// It is what UPG-04's comparison reads on one side, and the simulator has to
// serve it for that check to be a check rather than an error nobody sees: the
// drift comparison treats a read it cannot perform as "no drift", which is the
// conservative direction and is also silent.
//
// The default carries the arguments Talos sets for itself, so a node with no
// KernelArgs option configured shows *no* drift -- if it showed drift, every
// node in every cluster would be blocked and the check would be switched off.
func (s *Server) seedKernelCmdline(ctx context.Context) error {
	args := append([]string{
		"init_on_alloc=1", "slab_nomerge", "pti=on", "consoleblank=0",
		"talos.platform=metal", "printk.devkmsg=on",
	}, s.opts.KernelArgs...)

	cmdline := runtimeres.NewKernelCmdline()
	cmdline.TypedSpec().Cmdline = strings.Join(args, " ")

	if err := s.COSI().Create(ctx, cmdline); err != nil {
		return fmt.Errorf("talossim: seed %s: %w", runtimeres.KernelCmdlineType, err)
	}
	return nil
}

func (s *Server) seedExtensions(ctx context.Context) error {
	if s.opts.SchematicID == "" {
		return nil
	}

	ext := runtimeres.NewExtensionStatus(runtimeres.NamespaceName, "0")
	ext.TypedSpec().Metadata.Name = talos.SchematicExtensionName
	ext.TypedSpec().Metadata.Version = s.opts.SchematicID

	if err := s.COSI().Create(ctx, ext); err != nil {
		return fmt.Errorf("talossim: seed %s: %w", runtimeres.ExtensionStatusType, err)
	}
	return nil
}

// seedCluster puts the cluster membership and the node's active machine
// configuration into the state.
//
// The machine configuration is the whole reason a simulated cluster exists:
// the adoption path reads it off a control-plane node and derives the secrets
// bundle from it, and a node serving a hand-written stub would let that
// derivation pass a test it could not pass against Talos.
func (s *Server) seedCluster(ctx context.Context) error {
	for _, m := range s.opts.Members {
		member := cluster.NewMember(cluster.NamespaceName, m.ID)
		member.TypedSpec().Hostname = m.Hostname
		member.TypedSpec().NodeID = m.ID
		member.TypedSpec().OperatingSystem = "Talos (" + s.opts.TalosVersion + ")"
		member.TypedSpec().MachineType = machinetype.TypeWorker

		if m.ControlPlane {
			member.TypedSpec().MachineType = machinetype.TypeControlPlane
			member.TypedSpec().ControlPlane = &cluster.ControlPlane{APIServerPort: 6443}
		}

		for _, a := range m.Addresses {
			addr, err := netip.ParseAddr(a)
			if err != nil {
				return fmt.Errorf("talossim: member %s address %q: %w", m.ID, a, err)
			}
			member.TypedSpec().Addresses = append(member.TypedSpec().Addresses, addr)
		}

		if err := s.COSI().Create(ctx, member); err != nil {
			return fmt.Errorf("talossim: seed %s: %w", cluster.MemberType, err)
		}
	}

	if s.opts.Cluster == nil {
		return nil
	}

	provider, err := configloader.NewFromBytes(s.opts.Cluster.Config(s.opts.ControlPlane))
	if err != nil {
		return fmt.Errorf("talossim: load simulated machine config: %w", err)
	}

	if err := s.COSI().Create(ctx, configres.NewMachineConfig(provider)); err != nil {
		return fmt.Errorf("talossim: seed %s: %w", configres.MachineConfigType, err)
	}
	return nil
}

// prettySize renders a byte count the way Talos does, to one decimal place in
// the largest unit that keeps the number under a thousand.
func prettySize(n uint64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := uint64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "kMGTPE"[exp])
}

// hardwareAddr derives a stable locally-administered MAC from the hostname, so
// that two runs of the same test see the same address.
func hardwareAddr(hostname string) net.HardwareAddr {
	sum := memberID(hostname)
	return net.HardwareAddr{
		0x02,
		byte(sum >> 32), byte(sum >> 24), byte(sum >> 16), byte(sum >> 8), byte(sum),
	}
}

// seedKubernetes puts the node's Kubernetes-side resources into the state.
//
// It is separate from the rest of the seeding because it is also the restore
// path for the k8s_down scenario, which removes exactly these. A scenario that
// removed a resource nothing could put back would be a one-way door: the first
// test to inject it would leave every later test on that node looking at a
// cluster with no Kubernetes.
func (s *Server) seedKubernetes(ctx context.Context) error {
	nodename := k8s.NewNodename(k8s.NamespaceName, k8s.NodenameID)
	nodename.TypedSpec().Nodename = s.opts.Hostname

	if err := s.COSI().Create(ctx, nodename); err != nil {
		return fmt.Errorf("talossim: seed %s: %w", k8s.NodenameType, err)
	}

	// The kubelet's image tag is where the node's Kubernetes version comes
	// from (D-20). It is a k8s-namespace resource, so k8s_down removes it
	// along with the nodename -- which is exactly right: with Kubernetes down
	// the version becomes unknown, while every LevelNode fact stays confirmed.
	kubelet := k8s.NewKubeletSpec(k8s.NamespaceName, k8s.KubeletID)
	kubelet.TypedSpec().Image = s.opts.KubeletImage
	kubelet.TypedSpec().ExpectedNodename = s.opts.Hostname

	if err := s.COSI().Create(ctx, kubelet); err != nil {
		return fmt.Errorf("talossim: seed %s: %w", k8s.KubeletSpecType, err)
	}

	return nil
}

// nodeUUID derives a stable SMBIOS-shaped identifier from the hostname.
//
// STACK.md's inventory ruling is that the SMBIOS UUID is the node's primary
// key and an IP address is not, so the simulator has to have one, and it has
// to be the same across two calls for a test about identity to mean anything.
func nodeUUID(hostname string) string {
	sum := memberID(hostname)

	// The masks are what keep each group inside its width; the format verbs
	// then print the fixed number of hex digits a UUID has. Doing the
	// narrowing with masks rather than with conversions keeps the arithmetic
	// visibly total -- there is no value of sum for which a group overflows.
	return fmt.Sprintf("%08x-%04x-4%03x-8%03x-%012x",
		(sum>>32)&0xffffffff,
		(sum>>16)&0xffff,
		(sum>>4)&0x0fff,
		sum&0x0fff,
		sum&0xffffffffffff,
	)
}
