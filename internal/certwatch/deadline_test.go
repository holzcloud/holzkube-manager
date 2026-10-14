package certwatch_test

import (
	"context"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/certwatch"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// TestEveryReadCarriesADeadline: the Talos client refuses a call without one,
// and the context the daemon starts the watch with has none. In v0.6.0 and
// v0.7.0 that meant no node was ever read on a running daemon, and no test saw it
// because every test context carried a deadline of its own.
func TestEveryReadCarriesADeadline(t *testing.T) {
	t.Parallel()

	var saw bool
	var left time.Duration
	s := certwatch.New(certwatch.Deps{
		Clusters: func(context.Context) ([]model.Cluster, error) {
			return []model.Cluster{{ID: "c1", Name: "home"}}, nil
		},
		ControlPlanes: func(context.Context, model.ClusterID) ([]model.Machine, error) {
			return []model.Machine{{ID: "m1", Hostname: "cp-1", Role: model.RoleControlPlane}}, nil
		},
		Read: func(ctx context.Context, _ model.MachineID) ([]talos.Certificate, error) {
			d, ok := ctx.Deadline()
			saw = ok
			left = time.Until(d)
			return nil, nil
		},
	})

	// context.Background() is what main.go passes down: no deadline of its own.
	s.Tick(context.Background())

	if !saw {
		t.Fatal("Read was handed a context without a deadline; the Talos client refuses such a call")
	}
	if left <= 0 || left > certwatch.DefaultReadBudget {
		t.Errorf("the deadline is %v away, want within (0, %v]", left, certwatch.DefaultReadBudget)
	}
}

// TestARunningDaemonsContextReadsARealNode is the same claim through the real
// client against the simulator, started from a context with no deadline.
func TestARunningDaemonsContextReadsARealNode(t *testing.T) {
	t.Parallel()

	sim, err := talossim.New(talossim.Options{ControlPlane: true})
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}
	t.Cleanup(func() { _ = sim.Close() })

	s := certwatch.New(certwatch.Deps{
		Clusters: func(context.Context) ([]model.Cluster, error) {
			return []model.Cluster{{ID: "c1", Name: "home"}}, nil
		},
		ControlPlanes: func(context.Context, model.ClusterID) ([]model.Machine, error) {
			return []model.Machine{{ID: "m1", Hostname: "cp-1", Role: model.RoleControlPlane}}, nil
		},
		Read: func(ctx context.Context, id model.MachineID) ([]talos.Certificate, error) {
			cc, err := talos.NewClusterClient(ctx, sim.Dialer(), talos.Target{
				Cluster: "c1", Machine: id, Addr: sim.Host(),
			}, sim.ClientCreds(), talos.Mode{})
			if err != nil {
				return nil, err
			}
			defer func() { _ = cc.Close() }()
			return cc.Certificates(ctx)
		},
	})

	s.Tick(context.Background())

	h := s.HealthFor(model.Cluster{ID: "c1", Name: "home"}, time.Now())
	if h == nil || h.ReadNodes != 1 || len(h.Items) == 0 {
		t.Fatalf("nothing was read from the node through a context without a deadline: %+v", h)
	}
}
