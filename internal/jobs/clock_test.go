package jobs_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// What a resumed node action may conclude, on a host without a clock that
// keeps time while it is off (the Raspberry Pi this runs on) and right after
// the daemon started, possibly before its network is up.

func simConnector(t *testing.T) (*talossim.Server, jobs.Connector) {
	t.Helper()
	cl, err := talossim.NewCluster("homelab", "https://192.168.1.10:6443")
	if err != nil {
		t.Fatal(err)
	}
	sim, err := talossim.New(talossim.Options{Hostname: "cp-1", Cluster: cl, ControlPlane: true, Bootstrapped: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sim.Close() })
	return sim, func(ctx context.Context, id model.MachineID) (*talos.ClusterClient, error) {
		return talos.NewClusterClient(ctx, sim.Dialer(), talos.Target{
			Cluster: "c1", Machine: id, Addr: sim.Host(),
		}, sim.ClientCreds(), talos.Mode{})
	}
}

// TestRebootedSinceDoesNotConcludeFromAClockItCannotBelieve: the node has not
// rebooted since it came up, a moment ago.
func TestRebootedSinceDoesNotConcludeFromAClockItCannotBelieve(t *testing.T) {
	t.Parallel()
	_, connect := simConnector(t)

	// The control: a start after the node's boot, by a clock that is right.
	if back, err := jobs.RebootedSince(t.Context(), connect, "m1", time.Now()); err != nil || back {
		t.Fatalf("a node that did not reboot = (%v, %v), want (false, nil)", back, err)
	}

	// The step's start was recorded by a clock that was days behind (set late
	// at boot, before NTP): the node's boot is "after" it although no reboot
	// ever happened. Taking that for a reboot skips one that never ran.
	back, err := jobs.RebootedSince(t.Context(), connect, "m1", time.Now().Add(-72*time.Hour))
	if err == nil {
		t.Errorf("a step start 72h ago by this clock answered (%v, nil); it is no evidence "+
			"either way and must be \"cannot tell\"", back)
	}

	// The clock went back since the start was recorded. Answering "not
	// rebooted" reboots the node a second time.
	back, err = jobs.RebootedSince(t.Context(), connect, "m1", time.Now().Add(time.Hour))
	if err == nil {
		t.Errorf("a step start an hour in this clock's future answered (%v, nil); "+
			"want \"cannot tell\"", back)
	}
}

// TestAShutdownIsNotDoneBecauseTheNodeDoesNotAnswer: a daemon that has just
// started cannot reach anything until its network is up, and a node it cannot
// reach is not therefore off.
func TestAShutdownIsNotDoneBecauseTheNodeDoesNotAnswer(t *testing.T) {
	t.Parallel()
	_, connect := simConnector(t)

	step := jobs.ShutdownStep(connect, "m1", false)
	if done, err := step.Happened(t.Context(), &model.Job{}); err != nil || done {
		t.Fatalf("a node that answers = (%v, %v), want (false, nil): the shutdown runs", done, err)
	}

	noNetwork := func(context.Context, model.MachineID) (*talos.ClusterClient, error) {
		return nil, errors.New("dial tcp 192.168.1.10:50000: connect: network is unreachable")
	}
	step = jobs.ShutdownStep(noNetwork, "m1", false)
	done, err := step.Happened(t.Context(), &model.Job{})
	if err == nil {
		t.Errorf("a node this host cannot reach answered (%v, nil); silence is no proof of "+
			"a shutdown, so it must be \"cannot tell\"", done)
	}
}
