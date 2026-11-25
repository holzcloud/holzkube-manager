package upgrade

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// The rule of the gate before a worker's reboot, and the cases of its step that
// need no cluster at all. Where a test sets pollInterval it is sequential and
// puts the value back: Go runs a parallel test only after every sequential one
// has finished.

// TestDecideWorker is the rule as a pure function: every branch is about a
// cluster in a state that is dear to produce against a simulator.
func TestDecideWorker(t *testing.T) {
	budget := kube.EvictionRefusal{
		Pod: "default/db-0",
		Reason: "a PodDisruptionBudget refuses to let this pod go right now: " +
			"Cannot evict pod (The disruption budget db-pdb needs 1 healthy pods and has 0 currently)",
	}
	notAsked := kube.EvictionRefusal{
		Pod:    "default/api-0",
		Reason: "the cluster could not be asked whether this pod may go: 500 internal error",
	}

	tests := []struct {
		name string
		in   WorkerInput

		wantOK     bool
		wantReason []string // all in the refusal
		wantDetail []string // all in the summary of a gate that passed

		// first must come before then in the refusal.
		first, then string
	}{
		{
			name:       "everything is free",
			in:         WorkerInput{Node: "w-1", Others: 2, PodsChecked: 3},
			wantOK:     true,
			wantDetail: []string{"can spare w-1", "2 other nodes Ready", "3 pods"},
		},
		{
			name:       "another node that is not Ready closes the gate and is named with its state",
			in:         WorkerInput{Node: "w-1", Others: 2, NotReady: []string{"w-2 (Ready=False)"}},
			wantReason: []string{"w-2 (Ready=False)", "second failure"},
		},
		{
			name:       "a node whose Ready is Unknown is not Ready",
			in:         WorkerInput{Node: "w-1", Others: 2, NotReady: []string{"w-3 (Ready=Unknown)"}},
			wantReason: []string{"w-3 (Ready=Unknown)"},
		},
		{
			name: "a node that is off on purpose is not counted and is named",
			in: WorkerInput{
				Node: "w-1", Others: 1, PodsChecked: 1, Ignored: []string{"w-2"},
			},
			wantOK:     true,
			wantDetail: []string{"w-2", "off on purpose"},
		},
		{
			name: "the node being upgraded may itself be not Ready, and the detail says so",
			in: WorkerInput{
				Node: "w-1", Others: 2, SelfReady: "False",
			},
			wantOK:     true,
			wantDetail: []string{"w-1 is not Ready itself", "does not hold"},
		},
		{
			name: "readiness and budget together name both, readiness first",
			in: WorkerInput{
				Node: "w-1", Others: 2, NotReady: []string{"w-2 (Ready=False)"},
				PodsChecked: 1, Refusals: []kube.EvictionRefusal{budget},
			},
			wantReason: []string{"w-2 (Ready=False)", "db-pdb", "default/db-0"},
			first:      "w-2 (Ready=False)", then: "db-pdb",
		},
		{
			name: "a pod a budget holds names the budget and what to do",
			in: WorkerInput{
				Node: "w-1", PodsChecked: 1, Refusals: []kube.EvictionRefusal{budget},
			},
			wantReason: []string{"db-pdb", "default/db-0", "Raise the replica count", "loosen the budget", "move the load", "five minutes"},
		},
		{
			name: "a pod the cluster could not be asked about is a refusal that says so",
			in: WorkerInput{
				Node: "w-1", PodsChecked: 1, Refusals: []kube.EvictionRefusal{notAsked},
			},
			wantReason: []string{"could not be asked", "default/api-0"},
		},
		{
			name: "pods nothing owns never close the gate and are named in what it says",
			in: WorkerInput{
				Node: "w-1", Others: 1, PodsChecked: 1, Unowned: []string{"default/scratch"},
			},
			wantOK:     true,
			wantDetail: []string{"default/scratch", "Talos stops", "no budget"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := decideWorker(tc.in)
			if v.OK != tc.wantOK {
				t.Fatalf("OK = %v, want %v (reason %q)", v.OK, tc.wantOK, v.Reason)
			}
			if v.OK && v.Reason != "" {
				t.Errorf("a gate that passed carries the reason %q", v.Reason)
			}
			for _, want := range tc.wantReason {
				if !strings.Contains(v.Reason, want) {
					t.Errorf("the refusal %q does not say %q", v.Reason, want)
				}
			}
			if tc.first != "" && strings.Index(v.Reason, tc.first) > strings.Index(v.Reason, tc.then) {
				t.Errorf("the refusal %q names %q before %q: readiness comes first", v.Reason, tc.then, tc.first)
			}
			if tc.wantOK {
				detail := v.summary("w-1")
				for _, want := range tc.wantDetail {
					if !strings.Contains(detail, want) {
						t.Errorf("the detail %q does not say %q", detail, want)
					}
				}
			}
		})
	}
}

// gateDeps is a Deps with the two readings a gate step makes of the inventory.
func gateDeps(machines []model.Machine, kubeFn func(context.Context, model.ClusterID) (*kube.Client, error)) Deps {
	return Deps{
		Machines: func(context.Context, model.ClusterID) ([]model.Machine, error) { return machines, nil },
		Kube:     kubeFn,
	}
}

func runGateStep(d Deps, id model.MachineID) (string, error) {
	step := workerGateStep(d, "homelab", id)
	job := &model.Job{Steps: []model.JobStep{{Name: step.Name}}}
	err := step.Do(context.Background(), job)
	return job.Steps[0].Detail, err
}

// TestTheGateStepIsADeclaredNoOpForAControlPlaneNodeAndALockedNode: neither is
// asked anything. The control-plane node has etcd's gate in its upgrade step; a
// locked node is skipped there. Kubernetes is not looked at for either.
func TestTheGateStepIsADeclaredNoOpForAControlPlaneNodeAndALockedNode(t *testing.T) {
	var asked atomic.Int32
	d := gateDeps([]model.Machine{
		{ID: "cp", Role: model.RoleControlPlane, Hostname: "cp-1"},
		{ID: "wl", Role: model.RoleWorker, Hostname: "w-9", Locked: true},
	}, func(context.Context, model.ClusterID) (*kube.Client, error) {
		asked.Add(1)
		return nil, errors.New("must not be asked")
	})
	// Short, so that a gate that does ask ends in a measurement and not in five
	// minutes of waiting.
	d.WorkerGateBudget = 100 * time.Millisecond

	for id, want := range map[model.MachineID]string{"cp": "etcd's own gate", "wl": "locked"} {
		detail, err := runGateStep(d, id)
		if err != nil {
			t.Errorf("%s: the gate step returned %v, want a no-op", id, err)
		}
		if !strings.Contains(detail, want) {
			t.Errorf("%s: the detail %q does not say why it did nothing (%q)", id, detail, want)
		}
	}
	if n := asked.Load(); n != 0 {
		t.Errorf("Kubernetes was asked %d times for a node the gate has nothing to check for", n)
	}
}

// TestTheGateSaysItDidNotRunForAClusterWithoutKubernetesAuthority: the one
// error that is a fact about the configuration and not about the cluster.
func TestTheGateSaysItDidNotRunForAClusterWithoutKubernetesAuthority(t *testing.T) {
	d := gateDeps([]model.Machine{{ID: "w", Role: model.RoleWorker, Hostname: "w-1"}},
		func(context.Context, model.ClusterID) (*kube.Client, error) {
			return nil, fmt.Errorf("no way in: %w", kube.ErrNoKubernetesAuthority)
		})
	d.WorkerGateBudget = 100 * time.Millisecond

	detail, err := runGateStep(d, "w")
	if err != nil {
		t.Fatalf("the gate step returned %v, want a visible skip", err)
	}
	if !strings.Contains(detail, "did not run") {
		t.Errorf("the detail %q does not say that the gate did not run", detail)
	}
}

// TestTheGateRefusesImmediatelyWhenNoKubernetesClientIsWired: a gate that never
// runs would be a lie. It must not wait out its budget either.
func TestTheGateRefusesImmediatelyWhenNoKubernetesClientIsWired(t *testing.T) {
	d := gateDeps([]model.Machine{{ID: "w", Role: model.RoleWorker, Hostname: "w-1"}}, nil)
	d.WorkerGateBudget = time.Minute

	started := time.Now()
	_, err := runGateStep(d, "w")
	if err == nil {
		t.Fatal("the gate step passed with no Kubernetes client wired")
	}
	if !strings.Contains(err.Error(), "wired") {
		t.Errorf("the failure %q does not name the wiring", err)
	}
	if took := time.Since(started); took > 2*time.Second {
		t.Errorf("the gate waited %s before refusing a build that cannot ask at all", took)
	}
}

// TestTheGateTreatsASilentKubernetesAsAClosedGate: any error but the missing
// authority is a cluster that did not answer, and a cluster whose API does not
// answer is not one that is fine.
func TestTheGateTreatsASilentKubernetesAsAClosedGate(t *testing.T) {
	was := pollInterval
	pollInterval = 5 * time.Millisecond
	t.Cleanup(func() { pollInterval = was })

	var asked atomic.Int32
	d := gateDeps([]model.Machine{{ID: "w", Role: model.RoleWorker, Hostname: "w-1"}},
		func(context.Context, model.ClusterID) (*kube.Client, error) {
			asked.Add(1)
			return nil, errors.New("dial tcp 192.168.1.10:6443: connect: connection refused")
		})
	d.WorkerGateBudget = 40 * time.Millisecond

	_, err := runGateStep(d, "w")
	if err == nil {
		t.Fatal("the gate passed on a Kubernetes that did not answer")
	}
	if !strings.Contains(err.Error(), "did not answer") {
		t.Errorf("the failure %q does not say that Kubernetes did not answer", err)
	}
	if n := asked.Load(); n < 2 {
		t.Errorf("Kubernetes was asked %d times, want it asked again until the budget", n)
	}
}
