package inventory_test

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// heardApply is one attempt as an observer was told about it.
type heardApply struct {
	cluster model.ClusterID
	machine model.MachineID
	mode    string
	config  []byte
}

// recorder is an observer that keeps what it heard. It copies the bytes: the
// real observer must not keep them, a test that wants to look at them has to.
type recorder struct {
	mu    sync.Mutex
	heard []heardApply
}

func (r *recorder) observe(_ context.Context, a talos.ApplyAttempt) func(error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.heard = append(r.heard, heardApply{
		cluster: a.Cluster, machine: a.Machine, mode: a.Mode, config: bytes.Clone(a.Config),
	})
	return nil
}

func (r *recorder) all() []heardApply {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]heardApply(nil), r.heard...)
}

// TestConnectInstallsTheApplyObserverOnEveryClient: Connect is the one way a
// caller outside the inventory reaches a node (D-10), so it is the one place
// the observer of the configuration drift watch can be hung. A client that
// comes out of it without the observer would send a configuration the watch
// never hears about, and the next reading would call it drift.
func TestConnectInstallsTheApplyObserverOnEveryClient(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixture(t, talossim.Options{ControlPlane: true})
	c := f.importCluster(ctx, t)
	machines, err := f.svc.Machines(ctx)
	if err != nil || len(machines) != 1 {
		t.Fatalf("Machines = %v, %v; want the one node", machines, err)
	}
	id := machines[0].ID

	rec := &recorder{}
	f.svc.SetApplyObserver(rec.observe)

	send := func(label string) {
		t.Helper()
		cc, err := f.svc.Connect(ctx, id)
		if err != nil {
			t.Fatalf("%s: Connect: %v", label, err)
		}
		defer func() { _ = cc.Close() }()

		applyCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if _, err := cc.ApplyConfigurationWithMode(applyCtx, f.cluster.ControlPlaneConfig, "no-reboot"); err != nil {
			t.Fatalf("%s: ApplyConfigurationWithMode: %v", label, err)
		}
	}

	send("first client")
	heard := rec.all()
	if len(heard) != 1 {
		t.Fatalf("the observer heard %d attempts after the first apply, want 1", len(heard))
	}
	got := heard[0]
	if got.machine != id || got.cluster != c.ID || got.mode != "no-reboot" {
		t.Errorf("the observer heard machine %q of cluster %q in mode %q, want %q of %q in no-reboot",
			got.machine, got.cluster, got.mode, id, c.ID)
	}
	if !bytes.Equal(got.config, f.cluster.ControlPlaneConfig) {
		t.Error("the observer did not hear the bytes that were sent")
	}

	send("second client")
	if n := len(rec.all()); n != 2 {
		t.Fatalf("the observer heard %d attempts after a second client applied, want 2: "+
			"only the first client Connect gave out carried it", n)
	}

	f.svc.SetApplyObserver(nil)
	send("client after the observer was taken away")
	if n := len(rec.all()); n != 2 {
		t.Errorf("the observer heard %d attempts after it was taken away, want it to stay at 2", n)
	}
}
