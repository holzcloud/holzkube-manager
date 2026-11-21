package upgrade

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// Removing a node from its cluster, as a job (UPG-13).
//
// This used to run inside the HTTP request: leave etcd, reset, forget. A
// client that went away, a budget that expired or a daemon that restarted in
// the middle left a node that was out of etcd and not wiped, still listed in
// the inventory -- and nothing recorded how far it had got. It also took no
// cluster lease, so it could run beside an upgrade (JOB-03).
//
// The order is the whole of it, and each step is there because skipping it
// leaves something behind:
//
//  1. **check the node answers** (all roles). A node that is down fails the job
//     here, before anything has changed.
//  2. **cordon the node** (all roles), through the Kubernetes API: nothing new
//     is scheduled onto it from here on.
//  3. **drain the node** (all roles), through the eviction API, so every
//     PodDisruptionBudget is honoured. The pods have to be gone before the
//     node leaves the membership or is wiped, and every refusal has to stand
//     before the first step that changes the node: a budget that holds, or a
//     Kubernetes that does not answer, ends the job here with nothing wiped and
//     the node still in etcd. Talos drains by itself when it resets a node, but
//     it gives up after five minutes, logs a warning and goes on -- a pod a
//     budget protects is stopped anyway. This drain takes the veto seriously
//     and never deletes a pod instead of evicting it.
//  4. **leave etcd** (control plane only), on the node itself, so it forfeits
//     leadership if it has it and removes itself from the membership. Doing
//     this from another node while this one still runs leaves a member that
//     believes it is still in a cluster that has forgotten it. The quorum rule
//     is applied again here, immediately before the leave, because a job
//     starts later than the request that checked it.
//  5. **let etcd settle** (control plane only), EvictionWait: the gap in which
//     a raft that has just lost a member settles. Wiping into it is how the
//     removal of one member looks to the cluster like the loss of two.
//  6. **wipe the node**: reset of the system disk, so the machine does not
//     rejoin on its next boot with the cluster's PKI.
//  7. **forget the node**: take it out of the inventory. Without it the
//     dashboard shows a node that is never coming back, which is
//     indistinguishable from one that is down.
//
// What each step says after an interruption (Resume):
//
//   - cordon: verifiable. The node is unschedulable or it is not; repeating
//     the cordon is harmless.
//   - drain: verifiable. "Is anything left here that this drain would move" is
//     a read, the same question the drain was asked, so a restart inside the
//     drain carries on instead of parking.
//   - leave etcd: verifiable. The membership is read through a peer; a node
//     that is no longer listed has left.
//   - settle: verifiable. It is a wait, so it has happened once EvictionWait
//     has passed since it started -- including a daemon that was down for
//     longer than that.
//   - wipe: **not verifiable**, as for node.reset: nothing a node can be asked
//     tells "wiped a moment ago" from "booting for another reason", and the
//     cost of guessing wrong is a second wipe. The RPC only initiates, so the
//     window is milliseconds, and the job parks for a person.
//   - forget: verifiable. The machine is in the inventory or it is not.

// removeRoleParam is the job parameter that decides the step list. It is
// stored with the job so a resumed run rebuilds the steps it was submitted
// with, whatever the inventory says about the node by then.
const removeRoleParam = "role"

// RemovalParams is what a removal job stores.
func RemovalParams(m model.Machine) map[string]string {
	return map[string]string{
		"cluster":       string(m.Cluster),
		removeRoleParam: string(m.Role),
	}
}

func removeNodeSteps(d Deps, j model.Job) ([]jobs.Step, error) {
	if j.Machine == "" {
		return nil, errors.New("upgrade: a node removal names no node")
	}

	role := model.MachineRole(j.Params[removeRoleParam])
	if role != model.RoleControlPlane && role != model.RoleWorker {
		return nil, fmt.Errorf("upgrade: a node removal must state the node's role "+
			"(%s or %s), got %q: a control-plane node's steps include leaving etcd and a "+
			"worker's do not", model.RoleControlPlane, model.RoleWorker, role)
	}

	steps := []jobs.Step{jobs.ReachableStep(d.Connect, j.Machine), d.cordonStep(j), d.drainStep(j)}
	if role == model.RoleControlPlane {
		steps = append(steps, d.leaveEtcdStep(j), d.settleStep())
	}
	return append(steps, d.wipeStep(j), d.forgetStep(j)), nil
}

// wait is how long the settle step waits.
func (d Deps) wait() time.Duration {
	if d.EvictionWait > 0 {
		return d.EvictionWait
	}
	return EvictionWait
}

func (d Deps) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

func (d Deps) sleep(ctx context.Context, dur time.Duration) error {
	if d.Wait != nil {
		return d.Wait(ctx, dur)
	}
	t := time.NewTimer(dur)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// thisMachine finds the node being removed in the cluster's current machines.
func (d Deps) thisMachine(ctx context.Context, j *model.Job) (model.Machine, []model.Machine, error) {
	machines, err := d.Machines(ctx, j.Cluster)
	if err != nil {
		return model.Machine{}, nil, err
	}
	m, ok := machineByID(machines, j.Machine)
	if !ok {
		return model.Machine{}, nil, fmt.Errorf("upgrade: %s is no longer in the inventory", j.Machine)
	}
	return m, machines, nil
}

// readMembership reads etcd's membership for a removal.
//
// Through a peer first and through the node itself last. Asking the node that
// is being removed is what this used to do, and it cannot answer the question
// a resumed job has: once the node has left, its own etcd is stopped and
// refuses to list anything. A peer answers either way.
func (d Deps) readMembership(
	ctx context.Context,
	self model.Machine,
	machines []model.Machine,
) (MemberList, error) {
	order := make([]model.Machine, 0, len(machines))
	for _, m := range machines {
		if m.Role == model.RoleControlPlane && m.ID != self.ID {
			order = append(order, m)
		}
	}
	order = append(order, self)

	var lastErr error
	for _, m := range order {
		list, err := d.membersVia(ctx, m, machines)
		if err == nil {
			return list, nil
		}
		lastErr = err
	}
	return MemberList{}, lastErr
}

func (d Deps) membersVia(ctx context.Context, via model.Machine, machines []model.Machine) (MemberList, error) {
	cc, err := d.Connect(ctx, via.ID)
	if err != nil {
		return MemberList{}, err
	}
	defer cc.Close() //nolint:errcheck // the membership read's verdict is its own

	return Members(ctx, cc, machines)
}

// memberOf reports whether the list has this machine. known is false when it
// cannot be told: a machine with no recorded hostname matches no member.
func memberOf(list MemberList, m model.Machine) (member, known bool) {
	if m.Hostname == "" {
		return false, false
	}
	for _, e := range list.Members {
		if e.Machine == m.ID || strings.EqualFold(e.Hostname, m.Hostname) {
			return true, true
		}
	}
	return false, true
}

// probeStatus asks one machine's etcd for its own status; an answer, however
// unhappy its contents, means the node is there to vote.
func (d Deps) probeStatus(ctx context.Context, id model.MachineID) error {
	cc, err := d.Connect(ctx, id)
	if err != nil {
		return err
	}
	defer cc.Close() //nolint:errcheck // the probe's verdict is its own

	statusCtx, cancel, err := talos.WithClassDeadline(ctx, talos.MethodEtcdStatus)
	if err != nil {
		return err
	}
	defer cancel()

	_, err = cc.EtcdStatus(statusCtx)
	return err
}

// checkRemovable is the quorum rule for taking a control-plane node out. It
// reports whether the node is still an etcd member, and refuses when the
// cluster cannot spare it.
//
// A node that etcd no longer lists is not refused: there is nothing left to
// lose, and refusing would make a half-finished removal impossible to finish.
func (d Deps) checkRemovable(ctx context.Context, m model.Machine, machines []model.Machine) (bool, error) {
	list, err := d.readMembership(ctx, m, machines)
	if err != nil {
		return false, fmt.Errorf("upgrade: %s is a control-plane node and its cluster's etcd "+
			"membership could not be read, so there is no way to tell whether the cluster "+
			"survives losing it. Nothing has been changed: %w", nameOf(m), err)
	}
	if member, known := memberOf(list, m); known && !member {
		return false, nil
	}
	list = ProbeVoters(ctx, list, d.probeStatus)
	if err := RefuseIfCannotSpareAVoter(list, nameOf(m)); err != nil {
		return false, err
	}
	return true, nil
}

func (d Deps) leaveEtcdStep(_ model.Job) jobs.Step {
	return jobs.Step{
		Name: "leave etcd",
		Do: func(ctx context.Context, job *model.Job) error {
			m, machines, err := d.thisMachine(ctx, job)
			if err != nil {
				return err
			}

			member, err := d.checkRemovable(ctx, m, machines)
			if err != nil {
				return err
			}
			if !member {
				job.Steps[job.Current].Detail = "etcd no longer lists this node, so there was nothing to leave"
				return nil
			}

			cc, err := d.Connect(ctx, m.ID)
			if err != nil {
				return err
			}
			defer cc.Close() //nolint:errcheck // the leave's verdict is its own

			leaveCtx, cancel, err := talos.WithClassDeadline(ctx, talos.MethodEtcdLeaveCluster)
			if err != nil {
				return err
			}
			defer cancel()

			if err := cc.EtcdLeaveCluster(leaveCtx); err != nil {
				return fmt.Errorf("upgrade: %s could not leave etcd: %w. Nothing has been wiped",
					nameOf(m), err)
			}
			job.Steps[job.Current].Detail = "the node left etcd"
			return nil
		},
		Happened: func(ctx context.Context, job *model.Job) (bool, error) {
			m, machines, err := d.thisMachine(ctx, job)
			if err != nil {
				return false, err
			}
			list, err := d.readMembership(ctx, m, machines)
			if err != nil {
				return false, fmt.Errorf("etcd's membership could not be read: %w", err)
			}
			member, known := memberOf(list, m)
			if !known {
				return false, errors.New("the node has no recorded hostname, so its etcd " +
					"membership cannot be matched")
			}
			return !member, nil
		},
	}
}

func (d Deps) settleStep() jobs.Step {
	return jobs.Step{
		Name: "let etcd settle",
		Do: func(ctx context.Context, job *model.Job) error {
			if err := d.sleep(ctx, d.wait()); err != nil {
				return err
			}
			job.Steps[job.Current].Detail = fmt.Sprintf(
				"waited %s for etcd to settle before wiping the node", d.wait())
			return nil
		},
		// A wait has happened once it has been waited. Measured from the
		// step's recorded start, so a daemon that was down for longer than the
		// wait does not wait again.
		Happened: func(_ context.Context, job *model.Job) (bool, error) {
			started := job.Steps[job.Current].StartedAt
			return !started.IsZero() && d.now().Sub(started) >= d.wait(), nil
		},
	}
}

func (d Deps) wipeStep(j model.Job) jobs.Step {
	return jobs.Step{
		Name: "wipe the node",
		Do: func(ctx context.Context, job *model.Job) error {
			cc, err := d.Connect(ctx, j.Machine)
			if err != nil {
				return err
			}
			defer cc.Close() //nolint:errcheck // the reset's verdict is its own

			resetCtx, cancel, err := talos.WithClassDeadline(ctx, talos.MethodReset)
			if err != nil {
				return err
			}
			defer cancel()

			if err := cc.Reset(resetCtx, talos.ResetOptions{
				// The system disk, so the machine comes back in maintenance
				// mode and can be provisioned again. Not every disk: the
				// operator removing a node from a cluster has not necessarily
				// asked for the data on it to go, and a removal that wiped
				// more than it was asked to is not undoable.
				Mode:     "system-disk",
				Graceful: true,
				Reboot:   true,
			}); err != nil {
				return err
			}
			job.Steps[job.Current].Detail = "wiped the system disk, rebooting"
			return nil
		},
		// Deliberately nil, as for node.reset: see the comment on this file.
		Happened: nil,
	}
}

func (d Deps) forgetStep(j model.Job) jobs.Step {
	return jobs.Step{
		Name: "forget the node",
		Do: func(ctx context.Context, job *model.Job) error {
			if d.Forget == nil {
				return errors.New("upgrade: this build was wired without a way to forget a machine")
			}
			if err := d.Forget(ctx, j.Machine); err != nil {
				return err
			}
			job.Steps[job.Current].Detail = "removed from the inventory"
			return nil
		},
		Happened: func(ctx context.Context, job *model.Job) (bool, error) {
			machines, err := d.Machines(ctx, job.Cluster)
			if err != nil {
				return false, err
			}
			_, present := machineByID(machines, j.Machine)
			return !present, nil
		},
	}
}

// cordonLeftNote is what a failed drain says about the cordon it leaves
// behind. The job does not take it back yet; the sentence says so rather than
// leave the operator to find a node nothing is scheduled onto.
const cordonLeftNote = "The node stays cordoned, so nothing new is scheduled onto it, until it is " +
	"made schedulable again."

// drainOptions are the drain a removal runs.
//
// DeleteLocalData for the reason power.drainOptions gives: the node is about
// to be wiped, and its emptyDir goes with it whatever this does. No Force: a
// pod no controller owns is not coming back anywhere, and it is named in the
// failure instead of being lost. Budgets are honoured, always.
func (d Deps) drainOptions() kube.DrainOptions {
	return kube.DrainOptions{
		DeleteLocalData: true,
		Timeout:         d.drainBudget(),
	}
}

// drainBudget is how long the drain may take.
func (d Deps) drainBudget() time.Duration {
	if d.DrainBudget > 0 {
		return d.DrainBudget
	}
	return kube.DrainBudget
}

// drainRetry is the pause between the drain's attempts at a pod a budget
// refuses.
func (d Deps) drainRetry() time.Duration {
	if d.DrainPace > 0 {
		return d.DrainPace
	}
	return kube.DrainRetry
}

// drainGonePoll is how often the drain looks again at pods that were evicted
// and are still shutting down.
func (d Deps) drainGonePoll() time.Duration {
	if d.DrainPace > 0 {
		return d.DrainPace
	}
	return kube.DrainGonePoll
}

// kubeSkip says why a Kubernetes step has nothing to do.
type kubeSkip int

const (
	// kubeWork: there is a node to act on.
	kubeWork kubeSkip = iota
	// kubeNoAccess: the cluster's configuration holds no Kubernetes authority,
	// so no Kubernetes client can be had for it. That is a fact about the
	// configuration, and the only thing that is a skip.
	kubeNoAccess
	// kubeNoNode: Kubernetes answers and has no node for this machine.
	kubeNoNode
)

// kubeTarget is what a Kubernetes step acts on: the machine, a client for its
// cluster, and the node that machine is there.
type kubeTarget struct {
	machine model.Machine
	client  *kube.Client
	node    kube.Node
}

// kubeNode resolves a removal's machine to its Kubernetes node.
//
// It returns work, a visible skip, or an error. A Kubernetes that does not
// answer is an error and never a skip (D-03 Q7): an API server that is down
// would otherwise remove the safety net at the very moment it is needed. The
// only skips are a configuration without Kubernetes authority and a node
// Kubernetes does not know.
//
// first is the cordon step's: nothing has been changed yet, and the sentence
// says so, because that is what the operator most needs to hear.
func (d Deps) kubeNode(ctx context.Context, job *model.Job, first bool) (kubeTarget, kubeSkip, error) {
	m, _, err := d.thisMachine(ctx, job)
	if err != nil {
		return kubeTarget{}, kubeWork, err
	}
	changed := ""
	if first {
		changed = ". Nothing has been changed"
	}

	if d.Kube == nil {
		return kubeTarget{}, kubeWork, fmt.Errorf("upgrade: this build was wired without a Kubernetes "+
			"client, so %s cannot be drained before it is removed%s", nameOf(m), changed)
	}
	client, err := d.Kube(ctx, job.Cluster)
	if errors.Is(err, kube.ErrNoKubernetesAuthority) {
		return kubeTarget{machine: m}, kubeNoAccess, nil
	}
	if err != nil {
		return kubeTarget{}, kubeWork, fmt.Errorf("upgrade: Kubernetes did not answer, so %s cannot "+
			"be drained before it is removed: %w%s", nameOf(m), err, changed)
	}

	node, err := client.FindNodeFor(ctx, m.Hostname, m.Addr)
	if errors.Is(err, kube.ErrNoNodeFor) {
		return kubeTarget{machine: m}, kubeNoNode, nil
	}
	if err != nil {
		return kubeTarget{}, kubeWork, fmt.Errorf("upgrade: Kubernetes did not answer, so %s cannot "+
			"be drained before it is removed: %w%s", nameOf(m), err, changed)
	}
	return kubeTarget{machine: m, client: client, node: node}, kubeWork, nil
}

func (d Deps) cordonStep(_ model.Job) jobs.Step {
	return jobs.Step{
		Name: "cordon the node",
		Do: func(ctx context.Context, job *model.Job) error {
			detail := &job.Steps[job.Current].Detail
			t, skip, err := d.kubeNode(ctx, job, true)
			if err != nil {
				return err
			}
			switch skip {
			case kubeNoAccess:
				*detail = "this cluster has no Kubernetes access configured, so the node was " +
					"not cordoned and not drained"
				return nil
			case kubeNoNode:
				*detail = "Kubernetes has no node for " + nameOf(t.machine) +
					", so there was nothing to cordon"
				return nil
			case kubeWork:
			}

			if t.node.Unschedulable {
				*detail = t.node.Name + " was already cordoned"
				return nil
			}
			if err := t.client.Cordon(ctx, t.node.Name, true); err != nil {
				return fmt.Errorf("upgrade: %s could not be cordoned: %w. Nothing has been changed",
					nameOf(t.machine), err)
			}
			*detail = "cordoned " + t.node.Name + ": nothing new is scheduled onto it"
			return nil
		},
		// Cannot tell is not an error: repeating a cordon is harmless, and an
		// error here would park a job that is safe to carry on.
		Happened: func(ctx context.Context, job *model.Job) (bool, error) {
			t, skip, err := d.kubeNode(ctx, job, false)
			if err != nil || skip != kubeWork {
				return false, nil //nolint:nilerr // cannot tell; the cordon is safe to repeat
			}
			return t.node.Unschedulable, nil
		},
	}
}

func (d Deps) drainStep(_ model.Job) jobs.Step {
	return jobs.Step{
		Name: "drain the node",
		Do: func(ctx context.Context, job *model.Job) error {
			detail := &job.Steps[job.Current].Detail
			t, skip, err := d.kubeNode(ctx, job, false)
			if err != nil {
				return err
			}
			switch skip {
			case kubeNoAccess:
				*detail = "this cluster has no Kubernetes access configured, so the node was " +
					"not drained; its pods stop with the wipe"
				return nil
			case kubeNoNode:
				*detail = "Kubernetes has no node for " + nameOf(t.machine) +
					", so there was nothing to drain"
				return nil
			case kubeWork:
			}

			result, err := t.client.Drain(ctx, t.node.Name, d.drainOptions())
			*detail = result.Summary()
			if err != nil {
				if ctx.Err() != nil {
					// The daemon is shutting down or the operator cancelled:
					// the engine reads this as an interruption, not a verdict.
					return err
				}
				return drainFailure(nameOf(t.machine), result, err, cordonLeftNote)
			}
			return nil
		},
		// "Is anything left here that this drain would move" is a read, and
		// it is the question the drain was asked, so an interrupted drain
		// resumes instead of parking. Cannot tell is not an error: the drain
		// is safe to repeat.
		Happened: func(ctx context.Context, job *model.Job) (bool, error) {
			t, skip, err := d.kubeNode(ctx, job, false)
			if err != nil || skip != kubeWork {
				return false, nil //nolint:nilerr // cannot tell; the drain is safe to repeat
			}
			left, err := t.client.Undrained(ctx, t.node.Name, d.drainOptions())
			if err != nil {
				return false, nil //nolint:nilerr // as above
			}
			return len(left) == 0, nil
		},
	}
}

// drainFailure is the sentence a drain that did not finish ends the job with.
// It carries what the server said about each pod that stayed -- the budget's
// own message and the pod's name -- and what is still true: nothing has been
// wiped and the node has not left etcd. cordonNote says what became of the
// cordon, and is a parameter because that is the one part which changes when
// the job learns to take its own cordon back.
func drainFailure(node string, result kube.DrainResult, err error, cordonNote string) error {
	const untouched = "Nothing has been wiped and the node has not left etcd."
	if errors.Is(err, kube.ErrDrainBlocked) {
		return fmt.Errorf("upgrade: %s was not removed: its pods could not all be moved off it (%s). %s %s",
			node, result.Summary(), untouched, cordonNote)
	}
	return fmt.Errorf("upgrade: %s was not removed: draining it failed (%s): %w. %s %s",
		node, result.Summary(), err, untouched, cordonNote)
}
