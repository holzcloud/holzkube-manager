package upgrade_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/compat"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/upgrade"
)

// The Kubernetes plan's edges at service level: what happens when the source of
// the newest patch is missing, fails, or answers something that is not a
// release, and which plans must never ask it at all.
//
// The helpers are the ones plan 02 reuses (kubernetesMachine,
// newKubernetesPlanService), so their names are part of an agreement.

var kubernetesMachineSeq atomic.Int64

// kubernetesMachine is a control-plane node that runs the given Talos and
// Kubernetes versions, as the inventory last observed them.
func kubernetesMachine(talosVersion, kubernetesVersion string) model.Machine {
	n := kubernetesMachineSeq.Add(1)
	m := model.Machine{
		ID:       model.MachineID(fmt.Sprintf("kubernetes-plan-machine-%d", n)),
		Hostname: fmt.Sprintf("cp-%d", n),
		Role:     model.RoleControlPlane,
	}
	m.Snapshot.TalosVersion = talosVersion
	m.Snapshot.KubernetesVersion = kubernetesVersion
	return m
}

// newKubernetesPlanService builds a service that plans a Kubernetes upgrade
// over the given machines and reads nothing from a node. A nil src means
// WithKubernetesReleases is not called at all.
//
// Its gate sees no control-plane node, so a plan that gets past the chain is
// still blocked by the gate. Tests built on it therefore look at Chain,
// BlockReason and what the source was asked, and never at Blocked == false.
func newKubernetesPlanService(machines []model.Machine, src upgrade.KubernetesReleaseSource) *upgrade.Service {
	deps := upgrade.Deps{
		Machines: func(context.Context, model.ClusterID) ([]model.Machine, error) { return machines, nil },
		ResolveInstaller: func(context.Context, string, string, bool) (string, error) {
			return "", errors.New("a Kubernetes plan has no installer")
		},
	}
	deps.Gate = upgrade.NewGate(
		func(context.Context, model.MachineID) (*talos.ClusterClient, error) {
			return nil, errors.New("not connected")
		},
		func(context.Context, model.ClusterID) ([]model.Machine, error) { return nil, nil },
	)
	svc := upgrade.NewService(deps, nil)
	if src != nil {
		svc.WithKubernetesReleases(src)
	}
	return svc
}

// fakeKubernetesSource answers with a fixed reply or a fixed error and writes
// down every question it was asked.
type fakeKubernetesSource struct {
	mu    sync.Mutex
	asked [][]int
	reply map[int]string
	err   error
}

func (f *fakeKubernetesSource) source() upgrade.KubernetesReleaseSource {
	return func(_ context.Context, minors []int) (map[int]string, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.asked = append(f.asked, append([]int(nil), minors...))
		if f.err != nil {
			return nil, f.err
		}
		return f.reply, nil
	}
}

func (f *fakeKubernetesSource) questions() [][]int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]int(nil), f.asked...)
}

func planKubernetes(t *testing.T, svc *upgrade.Service, to string) upgrade.Plan {
	t.Helper()
	plan, err := svc.PlanKubernetes(context.Background(), "c1", to)
	if err != nil {
		t.Fatalf("PlanKubernetes(%s): %v", to, err)
	}
	return plan
}

func chainOf(plan upgrade.Plan) []string {
	out := make([]string, 0, len(plan.Chain))
	for _, s := range plan.Chain {
		out = append(out, s.To.String())
	}
	return out
}

func TestAPlanWithIntermediatesAsksTheSourceOnceForExactlyTheMinorsBetween(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		from, to  string
		reply     map[int]string
		wantAsked []int
		wantChain []string
	}{
		{"one minor on the way", "v1.32.4", "v1.34.1",
			map[int]string{33: "v1.33.7"}, []int{33}, []string{"v1.33.7", "v1.34.1"}},
		{"two minors on the way, one question", "v1.31.0", "v1.34.1",
			map[int]string{32: "v1.32.9", 33: "v1.33.7"}, []int{32, 33},
			[]string{"v1.32.9", "v1.33.7", "v1.34.1"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := &fakeKubernetesSource{reply: tc.reply}
			svc := newKubernetesPlanService([]model.Machine{kubernetesMachine("v1.13.9", tc.from)}, src.source())

			plan := planKubernetes(t, svc, tc.to)

			asked := src.questions()
			if len(asked) != 1 {
				t.Fatalf("the source was asked %d times (%v), want once", len(asked), asked)
			}
			if fmt.Sprint(asked[0]) != fmt.Sprint(tc.wantAsked) {
				t.Errorf("the source was asked about %v, want %v", asked[0], tc.wantAsked)
			}
			if got := chainOf(plan); fmt.Sprint(got) != fmt.Sprint(tc.wantChain) {
				t.Errorf("the chain is %v, want %v (blocked: %q)", got, tc.wantChain, plan.BlockReason)
			}
			if strings.Contains(plan.BlockReason, "newest patch") {
				t.Errorf("the plan says the patch could not be found, though it was: %q", plan.BlockReason)
			}
		})
	}
}

func TestAPlanWhoseLookupFailedIsBlockedWithASentence(t *testing.T) {
	t.Parallel()

	src := &fakeKubernetesSource{err: errors.New("dl.example.com: connection refused")}
	svc := newKubernetesPlanService([]model.Machine{kubernetesMachine("v1.13.9", "v1.32.4")}, src.source())

	plan := planKubernetes(t, svc, "v1.34.1")

	if !plan.Blocked {
		t.Fatal("a chain whose newest patch could not be found was not blocked")
	}
	if len(plan.Chain) != 0 {
		t.Errorf("a blocked plan still carries a chain: %v", chainOf(plan))
	}
	for _, want := range []string{"newest patch", "1.33", "connection refused"} {
		if !strings.Contains(plan.BlockReason, want) {
			t.Errorf("the block does not mention %q: %q", want, plan.BlockReason)
		}
	}
	if strings.Contains(plan.BlockReason, "v1.33.0") {
		t.Errorf("the block offers the first release of 1.33 in place of the patch: %q", plan.BlockReason)
	}
}

func TestASourceAnswerThatIsNotAStableReleaseOfThatMinorIsAFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		reply map[int]string
	}{
		{"another minor", map[int]string{33: "v1.34.2"}},
		{"a pre-release", map[int]string{33: "v1.33.7-rc.1"}},
		{"not a version", map[int]string{33: "stable"}},
		{"no entry for the minor", map[int]string{}},
		{"no map at all", nil},
		{"another major", map[int]string{33: "v2.33.7"}},
		{"no patch number, which would parse as patch zero", map[int]string{33: "v1.33"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := &fakeKubernetesSource{reply: tc.reply}
			svc := newKubernetesPlanService([]model.Machine{kubernetesMachine("v1.13.9", "v1.32.4")}, src.source())

			plan := planKubernetes(t, svc, "v1.34.1")

			if !plan.Blocked {
				t.Fatalf("the answer %v was taken as a patch; the chain is %v", tc.reply, chainOf(plan))
			}
			if len(plan.Chain) != 0 {
				t.Errorf("a blocked plan still carries a chain: %v", chainOf(plan))
			}
			if !strings.Contains(plan.BlockReason, "newest patch") {
				t.Errorf("the block is not the sentence about the newest patch: %q", plan.BlockReason)
			}
		})
	}
}

func TestNoSourceBlocksOnlyMultiMinorChains(t *testing.T) {
	t.Parallel()

	svc := newKubernetesPlanService([]model.Machine{kubernetesMachine("v1.13.9", "v1.32.4")}, nil)

	plan := planKubernetes(t, svc, "v1.34.1")
	if !plan.Blocked || len(plan.Chain) != 0 {
		t.Fatalf("a chain with a step on the way and no source was planned: blocked=%v chain=%v",
			plan.Blocked, chainOf(plan))
	}
	if !strings.Contains(plan.BlockReason, "no source of Kubernetes releases") {
		t.Errorf("the block does not say there is no source: %q", plan.BlockReason)
	}
	if !strings.Contains(plan.BlockReason, "v1.33") {
		t.Errorf("the block does not say which target needs no lookup: %q", plan.BlockReason)
	}

	// The next minor and a patch jump have nothing on the way. The gate of this
	// service blocks them on its own account, so what is looked at is the chain
	// and that the sentence about the source is not the reason.
	next := planKubernetes(t, svc, "v1.33.2")
	if got := chainOf(next); fmt.Sprint(got) != "[v1.33.2]" {
		t.Errorf("the next minor gave the chain %v, want [v1.33.2]", got)
	}
	patch := planKubernetes(t, svc, "v1.32.9")
	if len(patch.Chain) != 0 {
		t.Errorf("a patch jump gave the chain %v, want none", chainOf(patch))
	}
	for name, p := range map[string]upgrade.Plan{"next minor": next, "patch jump": patch} {
		if strings.Contains(p.BlockReason, "source of Kubernetes releases") {
			t.Errorf("the %s was blocked for want of a source: %q", name, p.BlockReason)
		}
	}
}

func TestASingleHopNeverAsksTheSource(t *testing.T) {
	t.Parallel()

	// A source that would fail every question: if it is asked, the plan says so.
	src := &fakeKubernetesSource{err: errors.New("the source is down")}
	svc := newKubernetesPlanService([]model.Machine{kubernetesMachine("v1.13.9", "v1.32.4")}, src.source())

	for _, to := range []string{"v1.32.9", "v1.33.2"} {
		plan := planKubernetes(t, svc, to)
		if strings.Contains(plan.BlockReason, "the source is down") {
			t.Errorf("a plan to %s depended on the source: %q", to, plan.BlockReason)
		}
	}
	if got := src.questions(); len(got) != 0 {
		t.Errorf("the source was asked %v for targets with nothing on the way", got)
	}
}

func TestAStrandedTargetNeverAsksTheSource(t *testing.T) {
	t.Parallel()

	window := compat.Check("v1.12.3", "v1.31.0")
	if !window.Known {
		t.Fatal("Talos 1.12 has no row in the compatibility matrix")
	}
	// Five minors past the top of what Talos 1.12 carries, however high the
	// matrix puts it, and still within the cap on intermediate minors.
	to := fmt.Sprintf("v1.%d.0", window.Window.MaxMinor+5)

	src := &fakeKubernetesSource{err: errors.New("the source is down")}
	svc := newKubernetesPlanService([]model.Machine{kubernetesMachine("v1.12.3", "v1.31.0")}, src.source())

	plan := planKubernetes(t, svc, to)

	if !plan.Blocked {
		t.Fatalf("a target the running Talos does not carry was not blocked: %q", plan.BlockReason)
	}
	if strings.Contains(plan.BlockReason, "newest patch") || strings.Contains(plan.BlockReason, "the source is down") {
		t.Errorf("the block is the source's sentence and not the window's: %q", plan.BlockReason)
	}
	if !strings.Contains(plan.BlockReason, "supports") {
		t.Errorf("the block does not say what the running Talos supports: %q", plan.BlockReason)
	}
	if got := src.questions(); len(got) != 0 {
		t.Errorf("the source was asked %v about a target that cannot be taken", got)
	}
}

func TestAChainLongerThanTheCapIsBlockedBeforeAnyRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, from, to string
	}{
		{"more minors than the cap", "v1.10.0", "v1.30.0"},
		{"a target typed to exhaust the list", "v1.10.0", "v1.2000000000.0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := &fakeKubernetesSource{reply: map[int]string{}}
			// Talos 1.99 has no row in the matrix, so the window does not answer
			// first and the cap is what is being tested.
			svc := newKubernetesPlanService([]model.Machine{kubernetesMachine("v1.99.0", tc.from)}, src.source())

			plan := planKubernetes(t, svc, tc.to)

			if !plan.Blocked || len(plan.Chain) != 0 {
				t.Fatalf("a chain past the cap was planned: blocked=%v chain=%d", plan.Blocked, len(plan.Chain))
			}
			if !strings.Contains(plan.BlockReason, fmt.Sprint(upgrade.MaxKubernetesIntermediates)) {
				t.Errorf("the block does not name the cap %d: %q", upgrade.MaxKubernetesIntermediates, plan.BlockReason)
			}
			if got := src.questions(); len(got) != 0 {
				t.Errorf("the source was asked %v before the cap was applied", got)
			}
		})
	}
}
