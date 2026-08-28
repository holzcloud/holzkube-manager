package talossim

import (
	"context"

	"github.com/holzcloud/holzkube/internal/model"

	"github.com/holzcloud/holzkube/internal/talos"
)

// DiscoverySource returns a talos.DiscoverySource that announces this node.
//
// It is the second implementation of that interface, and the reason it exists
// is the retrofit claim: scanning pushes outward, a registration source pushes
// inward, and both satisfy the same interface with the same call site above
// them. A simulator that only implemented Dialer would leave the harder half of
// the SideroLink question -- who tells holzkube a node exists -- untested.
func (s *Server) DiscoverySource() talos.DiscoverySource { return &fakeSource{server: s} }

type fakeSource struct{ server *Server }

func (f *fakeSource) Kind() string { return "fake" }

// Run emits the node's own candidate once and returns. It does not close out:
// a fan-in owns the channel, because several sources feed one.
func (f *fakeSource) Run(ctx context.Context, out chan<- talos.Candidate) error {
	candidate := talos.Candidate{
		Target: talos.Target{
			Machine: model.MachineID(f.server.opts.Hostname),
			Addr:    f.server.Host(),
		},
		Identity: f.server.Identity(),
		Source:   f.Kind(),
	}
	candidate.Identity.Machine = candidate.Target.Machine

	select {
	case out <- candidate:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
