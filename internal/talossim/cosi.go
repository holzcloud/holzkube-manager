package talossim

import (
	"context"
	"fmt"
	"net/netip"

	cosiapi "github.com/cosi-project/runtime/api/v1alpha1"
	"github.com/cosi-project/runtime/pkg/state"
	"github.com/cosi-project/runtime/pkg/state/impl/inmem"
	"github.com/cosi-project/runtime/pkg/state/impl/namespaced"
	cosiserver "github.com/cosi-project/runtime/pkg/state/protobuf/server"
	"google.golang.org/grpc"

	"github.com/siderolabs/talos/pkg/machinery/nethelpers"
	"github.com/siderolabs/talos/pkg/machinery/resources/hardware"
	"github.com/siderolabs/talos/pkg/machinery/resources/k8s"
	"github.com/siderolabs/talos/pkg/machinery/resources/network"
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

	return s.seedKubernetes(ctx)
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
