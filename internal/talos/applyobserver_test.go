package talos_test

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// observerTarget is the identity of the node of these tests; the cluster is
// set, which simTarget leaves empty, because the observer is told which cluster
// a configuration goes to.
func observerTarget(sim *talossim.Server) talos.Target {
	t := simTarget(sim, "00000000-0000-0000-0000-0000000000ab")
	t.Cluster = "cluster-obs"
	return t
}

func observedClient(t *testing.T, sim *talossim.Server, mode talos.Mode) *talos.ClusterClient {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	cc, err := talos.NewClusterClient(ctx, sim.Dialer(), observerTarget(sim), sim.ClientCreds(), mode)
	if err != nil {
		t.Fatalf("NewClusterClient: %v", err)
	}
	t.Cleanup(func() { _ = cc.Close() })
	return cc
}

// recorder is an observer that writes down what it was told, in order, and how
// many applies had reached the node at the moment it was told.
type recorder struct {
	mu       sync.Mutex
	sim      *talossim.Server
	attempts []talos.ApplyAttempt
	reached  []int // applies that had reached the node when the attempt was reported
	outcomes []error
	events   []string
}

func (r *recorder) observer() talos.ApplyObserver {
	return func(_ context.Context, a talos.ApplyAttempt) func(error) {
		r.mu.Lock()
		defer r.mu.Unlock()
		// The configuration is copied: whoever sends it owns the slice.
		a.Config = bytes.Clone(a.Config)
		r.attempts = append(r.attempts, a)
		r.reached = append(r.reached, r.sim.Calls("ApplyConfiguration"))
		r.events = append(r.events, "attempt")
		return func(err error) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.outcomes = append(r.outcomes, err)
			r.events = append(r.events, "outcome")
		}
	}
}

func (r *recorder) snapshot() (attempts []talos.ApplyAttempt, reached []int, outcomes []error, events []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.attempts), slices.Clone(r.reached), slices.Clone(r.outcomes), slices.Clone(r.events)
}

// TestAnObserverHearsEveryConfigurationAClusterClientSends: the five modes
// through ApplyConfigurationWithMode and the way that sends no mode, each with
// the cluster, the machine, the mode and exactly the bytes, before the RPC
// reached the node, and the result with nil on success.
func TestAnObserverHearsEveryConfigurationAClusterClientSends(t *testing.T) {
	t.Parallel()

	rows := []struct {
		name     string
		mode     string // "" is ApplyConfiguration, which sends none
		wantMode string
	}{
		{"no-reboot", "no-reboot", "no-reboot"},
		{"reboot", "reboot", "reboot"},
		{"staged", "staged", "staged"},
		{"try", "try", "try"},
		{"auto", "auto", "auto"},
		{"ApplyConfiguration sends no mode, which is a reboot on the wire", "", "reboot"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()

			sim := newSim(t, talossim.Options{Hostname: "observed"})
			cc := observedClient(t, sim, talos.Mode{})
			rec := &recorder{sim: sim}
			cc.SetApplyObserver(rec.observer())

			cfg := []byte("version: v1alpha1\nmachine:\n  type: worker\n# " + row.name + "\n")
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()

			var err error
			if row.mode == "" {
				_, err = cc.ApplyConfiguration(ctx, cfg)
			} else {
				_, err = cc.ApplyConfigurationWithMode(ctx, cfg, row.mode)
			}
			if err != nil {
				t.Fatalf("apply: %v", err)
			}

			attempts, reached, outcomes, events := rec.snapshot()
			if len(attempts) != 1 || len(outcomes) != 1 {
				t.Fatalf("the observer heard %d attempts and %d outcomes, want 1 and 1", len(attempts), len(outcomes))
			}
			a := attempts[0]
			if a.Cluster != "cluster-obs" || a.Machine != model.MachineID("00000000-0000-0000-0000-0000000000ab") {
				t.Errorf("the attempt is for %q/%q, want the client's cluster and machine", a.Cluster, a.Machine)
			}
			if a.Mode != row.wantMode {
				t.Errorf("the mode is %q, want %q", a.Mode, row.wantMode)
			}
			if !bytes.Equal(a.Config, cfg) {
				t.Errorf("the observer was handed %q, want exactly the bytes sent", a.Config)
			}
			if reached[0] != 0 {
				t.Errorf("%d applies had reached the node when the observer was told; it must be told before the RPC", reached[0])
			}
			if outcomes[0] != nil {
				t.Errorf("the outcome of a successful apply is %v, want nil", outcomes[0])
			}
			if !slices.Equal(events, []string{"attempt", "outcome"}) {
				t.Errorf("events are %v, want the attempt and then its outcome", events)
			}
			if n := sim.Calls("ApplyConfiguration"); n != 1 {
				t.Errorf("the node got %d applies, want 1", n)
			}
		})
	}
}

// TestTheObserverHearsAFailedApply: a dry-run refusal and a rejection by the
// node both come back as the outcome, with the error the caller got.
func TestTheObserverHearsAFailedApply(t *testing.T) {
	t.Parallel()

	t.Run("dry-run", func(t *testing.T) {
		t.Parallel()

		sim := newSim(t, talossim.Options{Hostname: "observed-dry"})
		cc := observedClient(t, sim, talos.Mode{DryRun: true})
		rec := &recorder{sim: sim}
		cc.SetApplyObserver(rec.observer())

		ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
		defer cancel()
		_, err := cc.ApplyConfigurationWithMode(ctx, []byte("version: v1alpha1\n"), "no-reboot")
		if !errors.Is(err, talos.ErrDryRun) {
			t.Fatalf("precondition: the apply returned %v, want ErrDryRun", err)
		}

		attempts, _, outcomes, _ := rec.snapshot()
		if len(attempts) != 1 || len(outcomes) != 1 {
			t.Fatalf("the observer heard %d attempts and %d outcomes, want 1 and 1", len(attempts), len(outcomes))
		}
		if !errors.Is(outcomes[0], talos.ErrDryRun) {
			t.Errorf("the outcome is %v, want the error the caller got (ErrDryRun)", outcomes[0])
		}
	})

	t.Run("rejected by the node", func(t *testing.T) {
		t.Parallel()

		sim := newSim(t, talossim.Options{Hostname: "observed-rejected"})
		lift, err := sim.Inject(talossim.Scenario{Name: talossim.ScenarioRejectApply})
		if err != nil {
			t.Fatalf("Inject: %v", err)
		}
		t.Cleanup(lift)
		cc := observedClient(t, sim, talos.Mode{})
		rec := &recorder{sim: sim}
		cc.SetApplyObserver(rec.observer())

		ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
		defer cancel()
		_, callErr := cc.ApplyConfigurationWithMode(ctx, []byte("version: v1alpha1\n"), "reboot")
		if callErr == nil {
			t.Fatal("precondition: the node accepted a configuration it was set to reject")
		}

		_, _, outcomes, _ := rec.snapshot()
		if len(outcomes) != 1 || outcomes[0] == nil || !strings.Contains(outcomes[0].Error(), "validation failed") {
			t.Errorf("the outcome is %v, want the node's rejection", outcomes)
		}
	})
}

// TestAnUnknownModeIsRefusedBeforeTheObserverIsCalled: a mode that is not one is
// no configuration that was sent.
func TestAnUnknownModeIsRefusedBeforeTheObserverIsCalled(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "observed-unknown"})
	cc := observedClient(t, sim, talos.Mode{})
	rec := &recorder{sim: sim}
	cc.SetApplyObserver(rec.observer())

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	if _, err := cc.ApplyConfigurationWithMode(ctx, []byte("version: v1alpha1\n"), "noreboot"); err == nil {
		t.Fatal("an unknown mode was not refused")
	}

	attempts, _, outcomes, _ := rec.snapshot()
	if len(attempts) != 0 || len(outcomes) != 0 {
		t.Errorf("the observer heard %d attempts and %d outcomes of a mode that was refused before sending, want none",
			len(attempts), len(outcomes))
	}
	if n := sim.Calls("ApplyConfiguration"); n != 0 {
		t.Errorf("the node got %d applies, want none", n)
	}
}

// TestAClientWithoutAnObserverBehavesAsBefore: nothing is required of a caller
// that does not set one, on either way of applying.
func TestAClientWithoutAnObserverBehavesAsBefore(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "unobserved"})
	cc := observedClient(t, sim, talos.Mode{})

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	withMode, err := cc.ApplyConfigurationWithMode(ctx, []byte("version: v1alpha1\n"), "no-reboot")
	if err != nil {
		t.Fatalf("ApplyConfigurationWithMode: %v", err)
	}
	if withMode.Details == "" {
		t.Error("the node's account of what it did is empty")
	}
	if _, err := cc.ApplyConfiguration(ctx, []byte("version: v1alpha1\n")); err != nil {
		t.Fatalf("ApplyConfiguration: %v", err)
	}
	if n := sim.Calls("ApplyConfiguration"); n != 2 {
		t.Errorf("the node got %d applies, want 2", n)
	}
}

// TestAnObserverThatReturnsNothingIsFine: the outcome is optional.
func TestAnObserverThatReturnsNothingIsFine(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "observed-nil"})
	cc := observedClient(t, sim, talos.Mode{})
	heard := 0
	cc.SetApplyObserver(func(context.Context, talos.ApplyAttempt) func(error) {
		heard++
		return nil
	})

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	if _, err := cc.ApplyConfigurationWithMode(ctx, []byte("version: v1alpha1\n"), "auto"); err != nil {
		t.Fatalf("ApplyConfigurationWithMode: %v", err)
	}
	if _, err := cc.ApplyConfiguration(ctx, []byte("version: v1alpha1\n")); err != nil {
		t.Fatalf("ApplyConfiguration: %v", err)
	}
	if heard != 2 {
		t.Errorf("the observer was told %d times, want 2", heard)
	}
}

// TestEveryClusterClientMethodThatSendsAConfigurationIsObserved pins the set of
// ClusterClient methods whose name starts with Apply: exactly the two that go
// through observeApply.
//
// A third way to send a configuration would be a way around the observer, and
// with it a way for an own change to show up as drift (or, worse, to be
// explained away). Adding one is a deliberate edit of this list, and the edit
// is where the new method is made to call observeApply.
func TestEveryClusterClientMethodThatSendsAConfigurationIsObserved(t *testing.T) {
	t.Parallel()

	want := []string{"ApplyConfiguration", "ApplyConfigurationWithMode"}

	typ := reflect.TypeOf(&talos.ClusterClient{})
	var got []string
	for i := range typ.NumMethod() {
		if name := typ.Method(i).Name; strings.HasPrefix(name, "Apply") {
			got = append(got, name)
		}
	}
	slices.Sort(got)

	if !slices.Equal(got, want) {
		t.Errorf("ClusterClient's methods that apply a configuration are %v, want %v; "+
			"a new one must be made to call observeApply and be added here", got, want)
	}
}
