package upgrade

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// The gate before a worker's reboot (milestone v1.17, phase 18, D-02).
//
// A control-plane node has a gate before it is touched: etcd must be able to
// spare it. A worker had none, and a worker's reboot takes the pods on it down
// as surely as a removal does. Talos drains a worker itself when it upgrades,
// but its drain loop gives up after five minutes, writes a warning, and goes on
// -- so a PodDisruptionBudget that holds a pod protects it for five minutes and
// then not at all. This gate stands before the first call to the node
// (ImagePull, Upgrade) and can only look: it cannot change how Talos behaves
// afterwards, so it says in advance what it can see.
//
// What it checks. Every other node in the cluster is Ready, because losing a
// worker when another one is already out is the second failure and not the first
// (a node whose machine is Disabled here is off on purpose and is not counted),
// and, for every pod a drain would move, the cluster is asked in a dry-run
// eviction whether that pod may go. The server decides that, with its own
// budget logic; nothing is calculated here, and the gate never changes a
// workload.
//
// What it does not do, said once so that nobody takes it for more:
//
//   - It is a snapshot. Talos drains afterwards on its own and the cluster can
//     change in between.
//   - It asks one pod at a time against the state as it is now. Two pods of one
//     budget that allows one disruption both pass; the budget that is dangerous
//     is the one that allows none, and that one is found.
//   - The plan's preview (gate_preview) does not show it. It runs only in the
//     job.
//
// # The bounds of the repeat loop
//
// A gate that is closed is asked again every pollInterval, because the second
// worker of a rolling upgrade meets it while the first one's pods are still
// coming back, and refusing at once would fail every upgrade with more than one
// worker at its second node. Three things bound that loop, and all three are
// this file's, because the job engine bounds nothing here: internal/jobs gives
// Do a context that ends only on shutdown or an operator's cancel, and it has
// no time limit per step.
//
//  1. The budget (WorkerGateBudget, five minutes) is the only time limit the
//     gate has. Without it the loop would run as long as the daemon does.
//  2. The budget also bounds one call. Every pass runs under a context that
//     ends when the budget does, so a Kubernetes that takes the connection and
//     never answers ends the gate with a sentence at the budget and not never.
//     When that context ends while the step's own is alive, it is the end of the
//     budget: the sentence says Kubernetes did not answer, and carries no
//     context error.
//  3. The step's context bounds the loop from outside. The wait between passes
//     is a timer selected against ctx.Done() -- not Deps.Wait, which a test
//     makes return at once and which would turn the loop into a busy one -- and
//     when ctx has ended Do returns the context's error unchanged. Only that
//     lets the engine tell an operator's cancel (the job is cancelled) and a
//     shutdown (the job stays for the restart) from a failure; a sentence in
//     its place would make both of them failed.

// WorkerGateBudget is how long a gate that is closed is asked again before the
// step ends. It is Talos' own patience: its drain gives up after five minutes,
// so a budget that has held for that long is one this upgrade would meet.
const WorkerGateBudget = 5 * time.Minute

// workerGateStepPrefix is the name of the step, before the machine's id.
const workerGateStepPrefix = "check Kubernetes can spare "

// WorkerInput is what the gate read, shown whether it passes or refuses.
type WorkerInput struct {
	// Node is the worker's name in Kubernetes.
	Node string

	// PodsChecked is how many pods a drain would move, which were asked about.
	PodsChecked int

	// Refusals are the pods the cluster would not let go.
	Refusals []kube.EvictionRefusal

	// Unowned are the pods nothing owns: Talos stops them at the reboot and no
	// budget protects them. Information, never a refusal.
	Unowned []string

	// NotReady are the other nodes whose Ready is not True, as "name
	// (Ready=state)". Ignored are the nodes left out of that on purpose: their
	// machine is Disabled here.
	NotReady, Ignored []string

	// SelfReady is the Ready of the worker being upgraded. It does not close the
	// gate: its reboot takes nothing away from a node that is not serving.
	SelfReady string
}

// WorkerVerdict is the gate's answer.
type WorkerVerdict struct {
	OK bool

	// Skipped is why the gate did not run, when it did not: a cluster this
	// product has no Kubernetes access to, or a node Kubernetes does not know.
	// It is a visible skip and not a pass.
	Skipped string

	// Reason is the sentence for a gate that is closed.
	Reason string

	Input WorkerInput
}

// workerGateBudget is the budget this run uses.
func (d Deps) workerGateBudget() time.Duration {
	if d.WorkerGateBudget > 0 {
		return d.WorkerGateBudget
	}
	return WorkerGateBudget
}

// decideWorker is the rule, as a pure function of what was read: a gate is open
// when no pod would be held. It is separate from the reading for the reason
// decide is: every branch is about a cluster in a state that is dear to produce.
func decideWorker(in WorkerInput) WorkerVerdict {
	v := WorkerVerdict{Input: in}
	if len(in.Refusals) == 0 {
		v.OK = true
		return v
	}

	held := make([]string, 0, len(in.Refusals))
	for _, r := range in.Refusals {
		held = append(held, r.Pod+": "+r.Reason)
	}
	v.Reason = fmt.Sprintf("%s would be held on %s: %s. Talos drains a worker itself when it upgrades and "+
		"gives up after five minutes and goes on, so a budget would not protect a pod through this upgrade. "+
		"Raise the replica count of the workload, loosen the budget or move the load, then start the "+
		"upgrade again",
		podCount(len(in.Refusals)), in.Node, strings.Join(held, "; "))
	return v
}

// podCount is "1 pod" or "3 pods".
func podCount(n int) string {
	if n == 1 {
		return "1 pod"
	}
	return strconv.Itoa(n) + " pods"
}

// summary is the detail of a gate that passed.
func (v WorkerVerdict) summary(name string) string {
	return fmt.Sprintf("Kubernetes can spare %s: %s asked about, none would be held (dry run, nothing was changed)",
		name, podCount(v.Input.PodsChecked))
}

// workerGateStep is the step before a node's upgrade step. One for every node,
// so the wait shows its name while it waits and so that a restart repeats the
// gate and verifies the upgrade, instead of doing both inside one step whose
// interruption nobody could reason about.
func workerGateStep(d Deps, cluster model.ClusterID, id model.MachineID) jobs.Step {
	return jobs.Step{
		Name: workerGateStepPrefix + string(id),

		Do: func(ctx context.Context, job *model.Job) error {
			machines, err := d.Machines(ctx, cluster)
			if err != nil {
				return err
			}
			me, ok := machineByID(machines, id)
			if !ok {
				return fmt.Errorf("upgrade: %s is no longer in the inventory", id)
			}
			detail := &job.Steps[job.Current].Detail

			// Neither is asked anything: Kubernetes is not looked at for them.
			if me.Locked {
				*detail = SkipReason(me)
				return nil
			}
			if me.Role == model.RoleControlPlane {
				*detail = nameOf(me) + " is a control-plane node: etcd's own gate stands in its upgrade " +
					"step, so this one has nothing to check"
				return nil
			}
			return d.holdWorkerGate(ctx, cluster, me, detail)
		},

		// The gate only reads, and asking it again costs a call and risks
		// nothing: after an interruption it is asked again, which is the safe
		// answer, and the same reasoning as power's gate.
		Happened: func(context.Context, *model.Job) (bool, error) { return false, nil },
	}
}

// holdWorkerGate asks until the gate is open, the budget ends, or ctx does.
func (d Deps) holdWorkerGate(ctx context.Context, cluster model.ClusterID, me model.Machine, detail *string) error {
	budget := d.workerGateBudget()
	deadline := d.now().Add(budget)
	var last string

	for {
		remaining := deadline.Sub(d.now())
		if remaining <= 0 {
			return closedGate(me, budget, last)
		}

		// One pass, under a context that ends with the budget: the call that
		// hangs is bounded by it, not only the loop around it.
		passCtx, cancel := context.WithTimeout(ctx, remaining)
		verdict, err := d.evaluateWorker(passCtx, cluster, me)
		cut := passCtx.Err() != nil // the budget ended while this pass was asking
		cancel()

		if ctx.Err() != nil {
			// Cancelled or shutting down. The engine reads exactly this, and a
			// sentence here would make it a failure.
			return ctx.Err()
		}
		switch {
		case err != nil:
			return err
		case verdict.Skipped != "":
			*detail = verdict.Skipped
			return nil
		case verdict.OK:
			*detail = verdict.summary(nameOf(me))
			return nil
		}
		// A pass the budget cut short has not learned anything: what it reports
		// is the budget's end, and it must not replace what the last complete
		// pass found, which is the reason the operator can act on.
		if !cut || last == "" {
			last = verdict.Reason
		}

		if wait := min(pollInterval, deadline.Sub(d.now())); wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			}
		}
	}
}

// closedGate is the failure of a gate that stayed closed for its whole budget.
func closedGate(me model.Machine, budget time.Duration, reason string) error {
	return fmt.Errorf("upgrade: Kubernetes cannot spare %s, and stayed that way for %s: %s. "+
		"Nothing was changed on the node: its image was not pulled and it was not restarted",
		nameOf(me), budget, reason)
}

// evaluateWorker reads the cluster once and decides. An error is only for a
// build that cannot ask at all; everything Kubernetes does wrong is a verdict
// that is not OK, and the loop asks again.
func (d Deps) evaluateWorker(ctx context.Context, cluster model.ClusterID, me model.Machine) (WorkerVerdict, error) {
	if d.Kube == nil {
		return WorkerVerdict{}, fmt.Errorf("upgrade: this build was wired without a Kubernetes client, so "+
			"the gate before %s cannot run. A gate that never runs would be a lie, so the upgrade stops", nameOf(me))
	}
	client, err := d.Kube(ctx, cluster)
	if errors.Is(err, kube.ErrNoKubernetesAuthority) {
		return WorkerVerdict{Skipped: "this product has no Kubernetes access to " + string(cluster) +
			" (its stored configuration holds no Kubernetes authority), so the gate did not run for " +
			nameOf(me)}, nil
	}
	if err != nil {
		return unanswered(ctx, err), nil
	}

	node, err := client.FindNodeFor(ctx, me.Hostname, me.Addr)
	if errors.Is(err, kube.ErrNoNodeFor) {
		return WorkerVerdict{Skipped: "Kubernetes has no node for " + nameOf(me) +
			" (it never joined, or the cluster has forgotten it), so there is nothing to check"}, nil
	}
	if err != nil {
		return unanswered(ctx, err), nil
	}

	check, err := client.CheckEvictions(ctx, node.Name, kube.DrainOptions{DeleteLocalData: true})
	if err != nil {
		return unanswered(ctx, err), nil
	}
	return decideWorker(WorkerInput{
		Node:        node.Name,
		PodsChecked: check.Checked,
		Refusals:    check.Refusals,
		Unowned:     check.Unowned,
	}), nil
}

// unanswered is the verdict for a Kubernetes that could not be asked: not OK,
// because a cluster whose API does not answer is not one that is fine. When the
// pass's context ran out the error is only that, and the sentence does not
// carry it.
func unanswered(ctx context.Context, err error) WorkerVerdict {
	reason := "Kubernetes did not answer before the budget for this gate ended"
	if ctx.Err() == nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		reason = "Kubernetes did not answer (" + err.Error() + ")"
	}
	return WorkerVerdict{Reason: reason + ". A cluster whose API does not answer is not taken as being " +
		"fine to go on with"}
}
