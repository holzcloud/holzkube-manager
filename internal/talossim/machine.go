package talossim

import (
	"context"
	"runtime"

	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/siderolabs/talos/pkg/machinery/api/machine"
)

// machineService is the simulated node's MachineService.
//
// It embeds machine.UnimplementedMachineServiceServer, so every RPC this plan
// does not implement answers Unimplemented rather than failing to compile. That
// is the honest default for a 54-method interface: the methods holzkube
// actually calls are implemented one at a time, and an unimplemented one shows
// up as a clear gRPC status instead of as a nil dereference.
//
// This plan implements exactly two: Version, which is the liveness probe the
// client performs at construction, and Hostname, which is what Dialer.Probe
// needs to fill in a talos.Identity. The rest of the surface, the COSI state
// and the streams belong to plan 02-08.
type machineService struct {
	machine.UnimplementedMachineServiceServer

	server *Server
}

// Version answers with the version the simulator was configured for.
func (m *machineService) Version(_ context.Context, _ *emptypb.Empty) (*machine.VersionResponse, error) {
	return &machine.VersionResponse{
		Messages: []*machine.Version{{
			Version: &machine.VersionInfo{
				Tag:       m.server.opts.TalosVersion,
				GoVersion: runtime.Version(),
				Os:        "linux",
				Arch:      runtime.GOARCH,
			},
		}},
	}, nil
}
