package upgrade

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"

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
//     is scheduled onto it from here on. This is the first step that asks
//     anything of the cluster, so it is where everything that can refuse
//     refuses, before the first change: for a control-plane node the quorum
//     rule, for every node whether the identity the removal acts as may patch
//     and delete nodes and evict pods, and whether Kubernetes answers at all.
//     It also keeps the name the cluster gives the node (kubeNodeParam), for
//     the steps that come after the wipe.
//  3. **drain the node** (all roles), through the eviction API, so every
//     PodDisruptionBudget is honoured. The pods have to be gone before the
//     node leaves the membership or is wiped, and every refusal has to stand
//     before the first step that changes the node: a budget that holds, a pod
//     no controller owns, or a Kubernetes that does not answer ends the job
//     here with nothing wiped and the node still in etcd. Talos drains by
//     itself when it resets a node, but it gives up after five minutes, logs a
//     warning and goes on -- a pod a budget protects is stopped anyway. This
//     drain takes the veto seriously and never deletes a pod instead of
//     evicting it, and never evicts a pod nothing would recreate: it names it.
//     A node whose kubelet reports nothing is the exception to waiting: its
//     pods cannot finish shutting down, so they are evicted and not waited
//     for.
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
//  7. **delete the Kubernetes node object** (all roles), through the
//     Kubernetes API. After the wipe and not before: a kubelet that is still
//     running puts its object back, and once the machine is wiped none is
//     left to. Nothing else deletes the object, so without this step the
//     machine stays on the cluster's node list as NotReady for ever. The
//     object deleted is the one the cordon step named, not one found again
//     through the inventory: a wiped machine boots into maintenance mode and the
//     inventory may call it something else by then. If the API no longer
//     answers -- the endpoint in a control-plane node's configuration can be
//     that node's own address -- or the configuration has lost its Kubernetes
//     authority since, the job fails here, in words that name `kubectl delete
//     node <name>`, rather than pass over it. A job that kept no name looks the
//     machine up through the inventory, as it always did.
//  8. **forget the node**: take it out of the inventory. Without it the
//     dashboard shows a node that is never coming back, which is
//     indistinguishable from one that is down.
//
// A removal that stops before the first irreversible step has not wiped the
// node, and the node is still in etcd and still in the inventory. It is not
// untouched: the pods the drain had already evicted have moved, and their
// controllers have started them elsewhere, so they stay moved. When the drain
// fails -- a budget that holds past the deadline, a pod nothing owns, an API
// that went away -- the job takes back the cordon it made itself, and says so
// in the failure. That holds when Kubernetes stops answering between the cordon
// and the drain as well: the step then makes one attempt, through a fresh
// client and the name the cordon step kept (kubeNodeParam), and says what came
// of it; a cordon it could not take back is named as still standing, with
// `kubectl uncordon`, and a rollback that did not happen is never claimed.
// Only its own cordon is given back: the cordon step notes in the job's
// parameters (cordonedByJobParam) when it was the one that made the node
// unschedulable, and a cordon without that note was somebody else's and stays.
// When the job is interrupted instead, nothing is given back. A shutdown leaves
// the job unfinished, and a restart carries on with the cordon in place; a
// cancel by the operator ends the job cancelled with its cordon still set.
// Only a failure in the drain step gives the cordon back: a control-plane
// removal that fails later, leaving etcd, leaves the node cordoned and drained.
//
// A cluster that never had Kubernetes access is skipped, visibly, in the
// cordon, drain and delete steps. A removal that did have access -- the cordon
// step named the node -- and loses it on the way does not skip: the drain step
// fails before the wipe, and the delete step after it, each in words that name
// the command to run.
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
//   - delete the node object: verifiable. Kubernetes lists a node of the name
//     the cordon step kept or it does not (a job without that name asks about
//     the machine instead); one that is not listed has been deleted, and a
//     restart inside the DELETE skips the step instead of failing on a second
//     one. Cannot tell (Kubernetes does not answer) is not an error: the
//     delete is safe to repeat, and the repeat says what is left.
//   - forget: verifiable. The machine is in the inventory or it is not.

// removeRoleParam is the job parameter that decides the step list. It is
// stored with the job so a resumed run rebuilds the steps it was submitted
// with, whatever the inventory says about the node by then.
const removeRoleParam = "role"

// cordonedByJobParam is the job parameter that says the cordon step made this
// node unschedulable itself. It decides, when the drain stops, whether the job
// gives the cordon back: a cordon the operator set before asking for the
// removal is theirs and stays. It is written with the end of the cordon step,
// when the engine saves the job. A crash between the patch and that save
// leaves the node cordoned and the parameter unset, which is the safe way
// round: a cordon somebody else set is never released, one too many left in
// place is named in the sentence.
const cordonedByJobParam = "cordoned_by_job"

// kubeNodeParam is the job parameter that holds the name the cluster itself
// gives this machine's Node object, as its own list of nodes shows it (never
// text from a request). The cordon step writes it as soon as it finds the node,
// whether the node was cordoned already or not, and removes it when it skips:
// a value that was there before that step ran is not that step's. The deletion
// of the Node object reads it, and so does the drain step when it can no longer
// resolve the node.
//
// It exists because the deletion comes after the wipe, and a wiped machine
// boots into maintenance mode: the inventory may call it something else by
// then, with another address, and a lookup by hostname and address would find no
// node and call that success while the object is still listed. A job without
// the parameter -- the cordon step found no node, or was confirmed after a
// restart without running -- looks the machine up through the inventory as it
// always did.
const kubeNodeParam = "kubernetes_node"

// removalPermissions is what the removal's identity must be allowed to do, and
// the cordon step asks before it changes anything: a removal that learned
// after the wipe that it may not delete the Node object would have wiped a
// machine and left it listed.
//
//   - patch nodes: the cordon (and its rollback).
//   - delete nodes: the deletion of the Node object, after the wipe.
//   - create pods/eviction: the eviction of every pod the drain moves.
var removalPermissions = []kube.Permission{
	{Verb: "patch", Resource: "nodes"},
	{Verb: "delete", Resource: "nodes"},
	{Verb: "create", Resource: "pods/eviction"},
}

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
	return append(steps, d.wipeStep(j), d.deleteNodeStep(j), d.forgetStep(j)), nil
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

// drainOptions are the drain a removal runs.
//
// DeleteLocalData for the reason power.drainOptions gives: the node is about
// to be wiped, and its emptyDir goes with it whatever this does. No Force: a
// pod no controller owns is not coming back anywhere, and it is named in the
// failure instead of being lost. Budgets are honoured, always.
//
// RetryBlocked and WaitGone are set here and nowhere else: this is the drain
// whose next step cannot be taken back, so it waits -- up to the deadline --
// for a budget to open and for the evicted pods to be gone, where a power
// action reports a refusal at once. Neither turns a veto into a delete.
func (d Deps) drainOptions() kube.DrainOptions {
	return kube.DrainOptions{
		DeleteLocalData: true,
		Timeout:         d.drainBudget(),
		RetryBlocked:    d.drainRetry(),
		WaitGone:        d.drainGonePoll(),
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

// kubeDown is a Kubernetes that did not answer: the client could not be had,
// or the list of nodes could not be read. It is a type of its own because the
// steps that find it say different things -- before the drain, nothing has
// been changed; after the wipe, the node is gone and the object is not.
type kubeDown struct{ err error }

func (e *kubeDown) Error() string { return e.err.Error() }
func (e *kubeDown) Unwrap() error { return e.err }

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
	t, skip, err := d.resolveNode(ctx, job)
	if err == nil {
		return t, skip, nil
	}
	changed := ""
	if first {
		changed = ". Nothing has been changed"
	}
	// The target comes back with the machine resolveNode found, so that a step
	// that fails here can still name it; nothing else in it is set.
	var down *kubeDown
	if errors.As(err, &down) {
		return t, kubeWork, fmt.Errorf("upgrade: Kubernetes did not answer, so %s cannot "+
			"be drained before it is removed: %w%s", nameOf(t.machine), down.err, changed)
	}
	if t.machine.ID != "" && errors.Is(err, errNoKubeClient) {
		return t, kubeWork, fmt.Errorf("upgrade: this build was wired without a Kubernetes "+
			"client, so %s cannot be drained before it is removed%s", nameOf(t.machine), changed)
	}
	return t, kubeWork, err
}

// errNoKubeClient is a build wired without Deps.Kube.
var errNoKubeClient = errors.New("upgrade: no Kubernetes client was wired")

// resolveNode is kubeNode without the words: what Kubernetes says about the
// machine, with the machine returned alongside any failure so that the caller
// can name it.
func (d Deps) resolveNode(ctx context.Context, job *model.Job) (kubeTarget, kubeSkip, error) {
	m, _, err := d.thisMachine(ctx, job)
	if err != nil {
		return kubeTarget{}, kubeWork, err
	}
	if d.Kube == nil {
		return kubeTarget{machine: m}, kubeWork, errNoKubeClient
	}
	client, err := d.Kube(ctx, job.Cluster)
	if errors.Is(err, kube.ErrNoKubernetesAuthority) {
		return kubeTarget{machine: m}, kubeNoAccess, nil
	}
	if err != nil {
		return kubeTarget{machine: m}, kubeWork, &kubeDown{err: err}
	}

	node, err := client.FindNodeFor(ctx, m.Hostname, m.Addr)
	if errors.Is(err, kube.ErrNoNodeFor) {
		return kubeTarget{machine: m}, kubeNoNode, nil
	}
	if err != nil {
		return kubeTarget{machine: m}, kubeWork, &kubeDown{err: err}
	}
	return kubeTarget{machine: m, client: client, node: node}, kubeWork, nil
}

func (d Deps) cordonStep(j model.Job) jobs.Step {
	controlPlane := model.MachineRole(j.Params[removeRoleParam]) == model.RoleControlPlane
	return jobs.Step{
		Name: "cordon the node",
		Do: func(ctx context.Context, job *model.Job) error {
			detail := &job.Steps[job.Current].Detail

			// The quorum rule first, before anything in Kubernetes: a rule that
			// refuses only after the pods have moved has already cost something.
			// It is asked again at the leave, which is where it was always
			// asked, because a job starts later than the request that checked
			// it and runs for longer than the drain.
			if controlPlane {
				m, machines, err := d.thisMachine(ctx, job)
				if err != nil {
					return err
				}
				if _, err := d.checkRemovable(ctx, m, machines); err != nil {
					return err
				}
			}

			t, skip, err := d.kubeNode(ctx, job, true)
			if err != nil {
				return err
			}
			switch skip {
			case kubeNoAccess:
				delete(job.Params, kubeNodeParam)
				*detail = "this cluster has no Kubernetes access configured, so the node was " +
					"not cordoned and not drained"
				return nil
			case kubeNoNode:
				delete(job.Params, kubeNodeParam)
				*detail = "Kubernetes has no node for " + nameOf(t.machine) +
					", so there was nothing to cordon"
				return nil
			case kubeWork:
			}

			// Kept for the steps after the wipe, which cannot look the machine
			// up again: see kubeNodeParam.
			if job.Params == nil {
				job.Params = map[string]string{}
			}
			job.Params[kubeNodeParam] = t.node.Name

			if err := mayRemoveNodes(ctx, t); err != nil {
				return err
			}

			if t.node.Unschedulable {
				*detail = t.node.Name + " was already cordoned"
				return nil
			}
			job.Params[cordonedByJobParam] = "true"
			if err := t.client.Cordon(ctx, t.node.Name, true); err != nil {
				delete(job.Params, cordonedByJobParam)
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

// mayRemoveNodes asks the cluster whether the identity the removal acts as may
// do everything the removal will do, and refuses with a sentence if it may not
// -- or if the cluster cannot be asked, which is no answer to proceed on.
//
// The answer is the cluster's own (SelfSubjectAccessReview), so on a client
// that acts as the operator it is the operator's rights that are checked and
// not this product's certificate's.
func mayRemoveNodes(ctx context.Context, t kubeTarget) error {
	who := t.client.Identity().String()
	answers, err := t.client.WhatMayI(ctx, removalPermissions)
	if err != nil {
		return fmt.Errorf("upgrade: Kubernetes could not be asked whether %s may cordon, evict "+
			"and delete nodes, so %s was not removed: %w. Nothing has been changed",
			who, nameOf(t.machine), err)
	}

	var missing []string
	for _, a := range answers {
		if a.Allowed {
			continue
		}
		what := a.Verb + " " + a.Resource
		if a.Reason != "" {
			what += " (" + a.Reason + ")"
		}
		missing = append(missing, what)
	}
	if len(missing) > 0 {
		return fmt.Errorf("upgrade: removing %s needs permission to cordon, evict and delete nodes, "+
			"and %s may not: %s. Nothing has been changed",
			nameOf(t.machine), who, strings.Join(missing, "; "))
	}
	return nil
}

func (d Deps) drainStep(_ model.Job) jobs.Step {
	return jobs.Step{
		Name: "drain the node",
		Do: func(ctx context.Context, job *model.Job) error {
			detail := &job.Steps[job.Current].Detail
			t, skip, err := d.kubeNode(ctx, job, false)
			if err != nil {
				if ctx.Err() != nil {
					// Shutting down or cancelled: an interruption, not a verdict.
					// Nothing is given back, for the reason given below.
					return err
				}
				// Kubernetes stopped answering after the cordon, or the machine
				// could not be resolved. Nothing has been wiped, and the cordon
				// this job made is still there unless it can be taken back.
				return withCordonNote(fmt.Errorf("%w. %s", err, nothingWiped), d.giveBackCordonUnresolved(ctx, job, t.machine))
			}
			switch skip {
			case kubeNoAccess:
				if name := job.Params[kubeNodeParam]; name != "" {
					// The cordon step had access and named the node, so this
					// cluster is not one that never had any: its configuration
					// lost the authority since. Skipping would let the wipe
					// run without a drain (D-03 Q7, as the operator made it
					// precise on 2026-11-21).
					return withCordonNote(fmt.Errorf("upgrade: the configuration of this cluster holds no "+
						"Kubernetes authority any more, although this removal cordoned %s with it, so %s "+
						"cannot be drained before it is removed. %s",
						name, nameOf(t.machine), nothingWiped), d.giveBackCordonUnresolved(ctx, job, t.machine))
				}
				*detail = "this cluster has no Kubernetes access configured, so the node was " +
					"not drained; its pods stop with the wipe"
				return nil
			case kubeNoNode:
				*detail = "Kubernetes has no node for " + nameOf(t.machine) +
					", so there was nothing to drain"
				return nil
			case kubeWork:
			}

			opts := d.drainOptions()
			silent := t.node.Ready != string(corev1.ConditionTrue)
			if silent {
				// A kubelet that reports nothing cannot finish shutting a pod
				// down, so waiting for the pods to leave would be waiting for
				// the deadline. They are evicted, and the node can still be
				// removed: it is being wiped anyway.
				opts.WaitGone = 0
			}

			result, err := t.client.Drain(ctx, t.node.Name, opts)
			*detail = result.Summary()
			if err != nil {
				if ctx.Err() != nil {
					// The daemon is shutting down or the operator cancelled:
					// the engine reads this as an interruption, not a verdict.
					// Nothing is given back: the job is not over, and a restart
					// carries on with the cordon in place.
					return err
				}
				return drainFailure(nameOf(t.machine), result, err, giveBackCordon(ctx, job, t))
			}
			if silent {
				*detail += ". The kubelet on this node reports nothing (Ready is " + t.node.Ready +
					"), so the evicted pods were not waited for: they stay terminating until the " +
					"cluster's pod garbage collection removes them"
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

// deleteNodeStep deletes the machine's Node object, after the wipe.
//
// After and not before: a kubelet that is still running puts its object back
// within seconds of a delete, so a delete before the wipe can be undone by the
// node it was meant to remove. Once the machine is wiped no kubelet is left to
// do that. The object has to go at all because nothing else deletes it: Talos
// does not, and a removed machine would stay on every node list as NotReady for
// ever, indistinguishable from one that is down.
//
// The API may not answer any more. The Kubernetes endpoint in the
// configuration of a control-plane node is sometimes that node's own address,
// which no longer answers once it is wiped; this cannot be told from the
// repository. Then the step fails, in words that name the command which
// finishes it, and it does not skip: a removal that reports success with the
// object still listed is worse than one that says what is left.
func (d Deps) deleteNodeStep(_ model.Job) jobs.Step {
	return jobs.Step{
		Name: "delete the Kubernetes node object",
		Do: func(ctx context.Context, job *model.Job) error {
			// A name the cordon step kept is deleted as it is: no new lookup,
			// and no skip. See kubeNodeParam.
			if name := job.Params[kubeNodeParam]; name != "" {
				return d.deleteRecordedNode(ctx, job, name)
			}

			detail := &job.Steps[job.Current].Detail
			t, skip, err := d.resolveNode(ctx, job)
			if err != nil {
				if ctx.Err() != nil {
					// Shutting down or cancelled: an interruption, not a verdict.
					return err
				}
				return d.nodeObjectLeft(job, t.machine, "", err)
			}
			switch skip {
			case kubeNoAccess:
				*detail = "this cluster has no Kubernetes access configured, so the node's " +
					"Kubernetes object was not deleted"
				return nil
			case kubeNoNode:
				*detail = "Kubernetes has no node for " + nameOf(t.machine) +
					", so there was nothing to delete"
				return nil
			case kubeWork:
			}

			if err := t.client.DeleteNode(ctx, t.node.Name); err != nil {
				if ctx.Err() != nil {
					return err
				}
				return d.nodeObjectLeft(job, t.machine, t.node.Name, err)
			}
			*detail = "deleted the Kubernetes node object " + t.node.Name
			return nil
		},
		// An object that is not there has been deleted. Cannot tell is not an
		// error: deleting twice is harmless, and an error would park a job that
		// is safe to carry on.
		Happened: func(ctx context.Context, job *model.Job) (bool, error) {
			if name := job.Params[kubeNodeParam]; name != "" {
				// Asked by the name the cluster gave, not by the machine: only an
				// answer that lists no node of that name says it is gone.
				if d.Kube == nil {
					return false, nil
				}
				client, err := d.Kube(ctx, job.Cluster)
				if err != nil {
					return false, nil //nolint:nilerr // cannot tell; the delete is safe to repeat
				}
				_, err = client.FindNodeFor(ctx, name, "")
				return errors.Is(err, kube.ErrNoNodeFor), nil
			}

			_, skip, err := d.resolveNode(ctx, job)
			if err != nil {
				return false, nil //nolint:nilerr // cannot tell; the delete is safe to repeat
			}
			return skip == kubeNoNode, nil
		},
	}
}

// deleteRecordedNode deletes the Node object the cordon step named. Every way
// of not being able to is a failure that names `kubectl delete node <name>`:
// with a name kept there is an object to delete, so a Kubernetes that does not
// answer, a configuration that has lost its Kubernetes authority since the
// cordon, and a DELETE the API refuses are none of them "nothing to delete".
// The machine is looked up only to say who it is in the sentence.
func (d Deps) deleteRecordedNode(ctx context.Context, job *model.Job, name string) error {
	m, _, err := d.thisMachine(ctx, job)
	if err != nil {
		if ctx.Err() != nil {
			return err
		}
		return d.nodeObjectLeft(job, model.Machine{}, name, err)
	}
	if d.Kube == nil {
		return d.nodeObjectLeft(job, m, name, errNoKubeClient)
	}
	client, err := d.Kube(ctx, job.Cluster)
	if err != nil {
		if ctx.Err() != nil {
			return err
		}
		return d.nodeObjectLeft(job, m, name, err)
	}
	if err := client.DeleteNode(ctx, name); err != nil {
		if ctx.Err() != nil {
			return err
		}
		return d.nodeObjectLeft(job, m, name, err)
	}
	job.Steps[job.Current].Detail = "deleted the Kubernetes node object " + name
	return nil
}

// nodeObjectLeft is the sentence for a Node object that could not be deleted.
// The machine is wiped by now, and for a control-plane node it has left etcd,
// so the sentence says what is true and what to run.
func (d Deps) nodeObjectLeft(job *model.Job, m model.Machine, node string, cause error) error {
	if node == "" {
		node = nameOf(m)
	}
	who := nameOf(m)
	if who == "" {
		who = node
	}
	done := "has been wiped"
	if model.MachineRole(job.Params[removeRoleParam]) == model.RoleControlPlane {
		done = "has been wiped and has left etcd"
	}
	return fmt.Errorf("upgrade: %s %s, but its Kubernetes node object could not be deleted: %w. "+
		"Delete it yourself with `kubectl delete node %s`. The machine stays listed here until "+
		"it is forgotten by hand", who, done, cause, node)
}

// rollbackBudget is how long giving the cordon back may take. Its own deadline,
// because the context of a job that has just failed may be one that is nearly
// out of time, and a cordon left in place for want of a second is the kind of
// thing this exists to prevent.
const rollbackBudget = 15 * time.Second

// cordonGivenBack is the sentence for a cordon the job made and took back.
const cordonGivenBack = "It was cordoned for this removal and has been uncordoned again."

// nothingWiped is what is true of every removal that stops in the drain step,
// whatever stopped it: nothing irreversible has happened. The pods the drain
// already evicted have moved and stay moved; that is not a claim made here.
const nothingWiped = "Nothing has been wiped and the node has not left etcd."

// cordonNotGivenBack is the sentence for a cordon the job made and could not
// take back: it names the command, and it does not say the node was released.
func cordonNotGivenBack(node string, cause error) string {
	return fmt.Sprintf("It is still cordoned by this removal: it could not be uncordoned again (%v); "+
		"run `kubectl uncordon %s`.", cause, node)
}

// withCordonNote appends the sentence about the cordon to an error, when there
// is one: a job that did not make a cordon says nothing about taking it back.
func withCordonNote(err error, note string) error {
	if note == "" {
		return err
	}
	return fmt.Errorf("%w %s", err, note)
}

// giveBackCordon takes back the cordon the cordon step made, when it made it,
// and says in a sentence what became of it. The node was schedulable when the
// job started; a failed drain leaves it unwiped and in etcd, and schedulable
// again, except for the pods that have already moved and stay moved.
//
// Only the job's own cordon: one that was there before is the operator's, and
// releasing it would undo a decision somebody else made.
func giveBackCordon(ctx context.Context, job *model.Job, t kubeTarget) string {
	if job.Params[cordonedByJobParam] != "true" {
		return "It was already cordoned before this removal, as far as the job recorded, and is " +
			"left that way."
	}

	// WithoutCancel: a failed drain is the case this is for, and the job's own
	// context may be about to end.
	rollback, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackBudget)
	defer cancel()
	if err := t.client.Cordon(rollback, t.node.Name, false); err != nil {
		return cordonNotGivenBack(t.node.Name, err)
	}
	return cordonGivenBack
}

// giveBackCordonUnresolved is giveBackCordon for a step that could not resolve
// the node, so it has no client and no node: one attempt, through a fresh
// client and the name the cordon step kept. It returns the sentence about the
// cordon, or nothing when the job made none. It is only called for a failure,
// never for an interruption (the caller has checked ctx.Err()), and it claims
// the cordon is given back only when Cordon returned no error.
func (d Deps) giveBackCordonUnresolved(ctx context.Context, job *model.Job, m model.Machine) string {
	if job.Params[cordonedByJobParam] != "true" {
		return ""
	}
	name := job.Params[kubeNodeParam]
	if name == "" {
		// No name to give a cordon back by, and none is guessed.
		return cordonNotGivenBack(nameOf(m), errors.New("this job did not keep the node's name in Kubernetes"))
	}
	if d.Kube == nil {
		return cordonNotGivenBack(name, errNoKubeClient)
	}

	rollback, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackBudget)
	defer cancel()
	client, err := d.Kube(rollback, job.Cluster)
	if err != nil {
		return cordonNotGivenBack(name, err)
	}
	if err := client.Cordon(rollback, name, false); err != nil {
		return cordonNotGivenBack(name, err)
	}
	return cordonGivenBack
}

// drainFailure is the sentence a drain that did not finish ends the job with.
// It carries what the server said about each pod that stayed -- the budget's
// own message and the pod's name -- and what is still true: nothing has been
// wiped and the node has not left etcd. cordonNote says what became of the
// cordon.
//
// Two kinds of pod are told apart from the rest, because what to do about them
// differs: a pod a PodDisruptionBudget holds, which will move when the budget
// allows it, and a pod no controller owns, which no waiting will move and
// which this never evicts.
func drainFailure(node string, result kube.DrainResult, err error, cordonNote string) error {
	if errors.Is(err, kube.ErrDrainBlocked) {
		var advice []string
		if bare := podsBlockedBy(result, func(b kube.BlockedPod) bool { return b.Reason == kube.BarePodReason }); len(bare) > 0 {
			advice = append(advice, "Pods that no controller owns are never moved by a removal ("+
				strings.Join(bare, ", ")+"): they have to be deleted or recreated by hand, and the "+
				"removal can then be repeated.")
		}
		if held := podsBlockedBy(result, func(b kube.BlockedPod) bool { return b.Budget }); len(held) > 0 {
			advice = append(advice, "A PodDisruptionBudget holds "+strings.Join(held, ", ")+
				": change the budget or wait until it allows the eviction, then repeat the removal.")
		}
		advice = append(advice, nothingWiped, cordonNote)
		return fmt.Errorf("upgrade: %s was not removed: its pods could not all be moved off it (%s). %s",
			node, result.Summary(), strings.Join(advice, " "))
	}
	return fmt.Errorf("upgrade: %s was not removed: draining it failed (%s): %w. %s %s",
		node, result.Summary(), err, nothingWiped, cordonNote)
}

// podsBlockedBy names the pods of a drain's result that match.
func podsBlockedBy(result kube.DrainResult, match func(kube.BlockedPod) bool) []string {
	var out []string
	for _, b := range result.Blocked {
		if match(b) {
			out = append(out, b.Pod)
		}
	}
	return out
}
