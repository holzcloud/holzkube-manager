package power

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// The steps the fourteen power jobs are built from.
//
// The Talos half -- is it there, reboot it, shut it down -- is the node-action
// code in internal/jobs, called rather than copied: whether a reboot happened
// is a question with one answer in this product, and it is the answer those
// steps already give. What is added here is the Kubernetes half around them
// (cordon, drain, uncordon), the part only a power action needs (wake, wait,
// the disabled mark), and the walk.
//
// Every step is either a read -- repeated after an interruption at the cost of
// a call -- or has a paired "did it happen" check, so an interrupted power job
// resumes instead of parking. The one place that cannot tell is "reboot every
// node at once" when some node does not answer, and that parks, because
// rebooting twice is not something to guess about.

// Register teaches the engine the fourteen jobs. It has to run before the
// engine's Resume, like every other registration: a stored job whose kind is
// not known at resume time is parked.
func (s *Service) Register(e *jobs.Engine) {
	for _, a := range Actions() {
		e.Register(NodeJobKind(a), func(j model.Job) ([]jobs.Step, error) {
			p, err := planFrom(j.Params)
			if err != nil {
				return nil, err
			}
			if len(p.Members) != 1 {
				return nil, fmt.Errorf("power: a node job walks one node, and this one names %d", len(p.Members))
			}
			return s.nodeSteps(a, j.Cluster, p.Members[0]), nil
		})
		e.Register(ClusterJobKind(a), func(j model.Job) ([]jobs.Step, error) {
			p, err := planFrom(j.Params)
			if err != nil {
				return nil, err
			}
			return s.clusterSteps(a, j.Cluster, p.Members), nil
		})
	}
}

// drainOptions is the drain a power action runs.
//
// Pods with an emptyDir are moved rather than refused. A node that is being
// switched off takes its emptyDir contents with it whatever this does -- the
// pods on it are evicted by the node controller within minutes of it going
// silent -- so refusing to stop until somebody deleted every scratch volume by
// hand would make stop useless in the cluster it exists for. Pods nothing owns
// are still refused, and named: those are not coming back anywhere, and the
// operator should decide about them rather than learn afterwards. Budgets are
// honoured, always; force-stop is the verb for not honouring them.
var drainOptions = kube.DrainOptions{DeleteLocalData: true, Timeout: kube.DrainBudget}

func (s *Service) nodeSteps(a Action, cluster model.ClusterID, m member) []jobs.Step {
	one := []member{m}
	switch a {
	case Stop:
		return []jobs.Step{
			s.reachable(m),
			s.gate(cluster, m),
			s.cordon(cluster, m, true),
			s.drain(cluster, m),
			s.shutdown(m, false, false),
		}
	case ForceStop:
		return []jobs.Step{s.reachable(m), s.shutdown(m, true, false)}
	case Start:
		return []jobs.Step{
			s.refuseDisabled(cluster, m),
			s.wake(one, "wake "+m.Hostname, true),
			s.waitAnswer(one, "wait for "+m.Hostname+" to answer"),
			s.uncordon(cluster, one, "uncordon "+m.Hostname, false),
		}
	case Disable:
		steps := []jobs.Step{}
		if m.Answered {
			// The gate before the mark: a disable etcd refuses must not leave
			// the node marked "stays off" while it is still running.
			steps = append(steps, s.reachable(m), s.gate(cluster, m))
		}
		steps = append(steps, s.markMachine(m, true))
		if m.Answered {
			steps = append(steps, s.cordon(cluster, m, true), s.drain(cluster, m), s.shutdown(m, false, false))
		}
		return steps
	case Enable:
		return []jobs.Step{
			s.markMachine(m, false),
			s.refuseDisabled(cluster, m),
			s.wake(one, "wake "+m.Hostname, true),
			s.waitAnswer(one, "wait for "+m.Hostname+" to answer"),
			s.uncordon(cluster, one, "uncordon "+m.Hostname, false),
		}
	case Restart:
		steps := []jobs.Step{s.reachable(m), s.cordon(cluster, m, true), s.drain(cluster, m)}
		reboot := len(steps)
		return append(steps,
			s.reboot(m, false),
			s.waitBack(cluster, m, reboot, true),
			s.uncordon(cluster, one, "uncordon "+m.Hostname, true),
		)
	case ForceRestart:
		steps := []jobs.Step{s.reachable(m), s.remember(cluster, one)}
		reboot := len(steps)
		return append(steps,
			s.reboot(m, true),
			s.waitBack(cluster, m, reboot, false),
			s.uncordon(cluster, one, "uncordon "+m.Hostname+" if it was schedulable", true),
		)
	}
	return nil
}

func (s *Service) clusterSteps(a Action, cluster model.ClusterID, members []member) []jobs.Step {
	switch a {
	case Stop:
		return s.stopWalk(cluster, members)
	case ForceStop:
		return []jobs.Step{s.forceOffAll(members)}
	case Start:
		return s.startWalk(cluster, members)
	case Disable:
		return append([]jobs.Step{s.markCluster(cluster, true)}, s.stopWalk(cluster, members)...)
	case Enable:
		return append([]jobs.Step{s.markCluster(cluster, false)}, s.startWalk(cluster, members)...)
	case Restart:
		var steps []jobs.Step
		for _, m := range members {
			one := []member{m}
			steps = append(steps, s.cordon(cluster, m, true), s.drain(cluster, m))
			reboot := len(steps)
			steps = append(steps,
				s.reboot(m, false),
				// The rule the operator asked for: stop at the first node that
				// does not come back healthy. A failed wait fails the job, and
				// nothing after it runs -- the next node is never touched.
				s.waitBack(cluster, m, reboot, true),
				s.uncordon(cluster, one, "uncordon "+m.Hostname, true),
			)
		}
		return steps
	case ForceRestart:
		steps := []jobs.Step{s.remember(cluster, members)}
		reboot := len(steps)
		return append(steps,
			s.rebootAll(members),
			s.waitAllBack(members, reboot),
			s.uncordon(cluster, members, "uncordon the nodes that were schedulable", true),
		)
	}
	return nil
}

// stopWalk takes a cluster down in walkOrder.
//
// Workers are cordoned, drained with every budget honoured, and shut down
// gracefully. Control-plane nodes are cordoned and shut down WITHOUT a drain
// and with Talos's own cordon-and-drain skipped. Draining them would move pods
// from one control-plane node to the next until the last one, where a budget
// refuses the final move and leaves the cluster half-stopped with its API still
// up; and once etcd has lost its quorum -- after the second of three -- the
// last one's graceful shutdown would wait on a cordon the dead API server can
// never perform. Their own services are still stopped in order.
//
// Every step tolerates a node that is already off, which a node disabled
// earlier is: the walk's job is "everything off", and a node that already is
// has had it happen.
func (s *Service) stopWalk(cluster model.ClusterID, members []member) []jobs.Step {
	var steps []jobs.Step
	for _, m := range members {
		if m.controlPlane() {
			steps = append(steps, s.cordon(cluster, m, false), s.shutdown(m, true, true))
			continue
		}
		steps = append(steps, s.cordon(cluster, m, true), s.drain(cluster, m), s.shutdown(m, false, true))
	}
	return steps
}

// startWalk brings a cluster up: control planes woken first, then the
// Kubernetes API waited for, then the workers, then every node this product
// cordoned for the stop uncordoned. A disabled node is skipped throughout, as
// the operator decided -- that is what the mark is for.
func (s *Service) startWalk(cluster model.ClusterID, members []member) []jobs.Step {
	var planes, workers []member
	for _, m := range members {
		if m.controlPlane() {
			planes = append(planes, m)
		} else {
			workers = append(workers, m)
		}
	}
	return []jobs.Step{
		s.refuseClusterDisabled(cluster),
		s.wake(planes, "wake the control plane", false),
		s.waitKube(cluster),
		s.wake(workers, "wake the workers", false),
		s.waitAnswer(workers, "wait for the workers to answer"),
		s.uncordon(cluster, members, "uncordon the nodes the stop cordoned", true),
	}
}

// reachable is the node actions' own first step, renamed after the node.
func (s *Service) reachable(m member) jobs.Step {
	step := jobs.ReachableStep(s.deps.Connect, m.Machine)
	step.Name = "check " + m.Hostname + " answers"
	return step
}

// shutdown is the node actions' shutdown. tolerant makes it a no-op on a node
// that is already off, which is what a cluster walk needs and a node stop --
// whose first step established that the node answers -- does not.
func (s *Service) shutdown(m member, force, tolerant bool) jobs.Step {
	inner := jobs.ShutdownStep(s.deps.Connect, m.Machine, force)
	step := inner
	step.Name = "shut down " + m.Hostname
	if force {
		step.Name = "force " + m.Hostname + " off"
	}
	if tolerant {
		step.Do = func(ctx context.Context, job *model.Job) error {
			if !jobs.Reachable(ctx, s.deps.Connect, m.Machine) {
				job.Steps[job.Current].Detail = m.Hostname + " is already off"
				return nil
			}
			return inner.Do(ctx, job)
		}
	}
	return step
}

// reboot is the node actions' reboot, renamed after the node.
func (s *Service) reboot(m member, force bool) jobs.Step {
	step := jobs.RebootStep(s.deps.Connect, m.Machine, force)
	step.Name = "reboot " + m.Hostname
	if force {
		step.Name = "force-reboot " + m.Hostname
	}
	return step
}

// gate asks etcd whether the cluster survives losing this node, immediately
// before it is taken down -- the same gate, asked at the same moment, as a
// rolling upgrade asks it (UPG-02). The report asked it too, when the button
// was drawn; this is the answer for the cluster as it is now.
func (s *Service) gate(cluster model.ClusterID, m member) jobs.Step {
	return jobs.Step{
		Name: "check etcd can spare " + m.Hostname,
		Do: func(ctx context.Context, job *model.Job) error {
			if !m.controlPlane() {
				job.Steps[job.Current].Detail = m.Hostname + " is a worker and not an etcd member; nothing to check"
				return nil
			}
			if s.deps.Gate == nil {
				return errors.New("power: this instance has no etcd health gate, so a control-plane node " +
					"cannot be stopped carefully; force-stop does not check")
			}
			v, err := s.deps.Gate.Evaluate(ctx, cluster, m.Machine)
			if err != nil {
				return err
			}
			if !v.OK {
				return fmt.Errorf("power: etcd cannot spare %s right now: %s Force-stop does not check",
					m.Hostname, v.Reason)
			}
			job.Steps[job.Current].Detail = fmt.Sprintf(
				"etcd's health gate passed with %d voting member(s) and no alarms", v.Input.Voting)
			return nil
		},
		Happened: never,
	}
}

// never is the Happened of a step that only reads, or is idempotent: repeating
// it after an interruption costs a call and risks nothing, so "it has not
// happened" is always the safe answer.
func never(context.Context, *model.Job) (bool, error) { return false, nil }

// errNoKubeNode reports a machine Kubernetes has no node for -- a machine that
// never joined, or one the API server has forgotten.
var errNoKubeNode = errors.New("power: Kubernetes has no node for this machine")

// findNode finds a machine's Kubernetes node: by name first, since Talos
// registers the kubelet under the hostname, then by address, for a cluster
// that overrode the node name.
func findNode(ctx context.Context, client *kube.Client, m member) (kube.Node, error) {
	nodes, err := client.Nodes(ctx)
	if err != nil {
		return kube.Node{}, err
	}
	for _, n := range nodes {
		if strings.EqualFold(n.Name, m.Hostname) {
			return n, nil
		}
	}
	for _, n := range nodes {
		if m.Addr != "" && n.InternalAddress == m.Addr {
			return n, nil
		}
	}
	return kube.Node{}, errNoKubeNode
}

// cordon stops the scheduler placing anything new on a node, and records that
// it was this product that did so -- BEFORE the cordon, so that an interruption
// between the two leaves a mark that is owed an uncordon rather than a cordon
// nobody will ever undo.
//
// strict makes a Kubernetes API that does not answer a failure. It is the
// careful half's rule: a stop that cannot drain has not been careful. A
// control-plane node in a cluster stop is the exception, because by the time
// the walk reaches it the API server may be the thing that is going away.
func (s *Service) cordon(cluster model.ClusterID, m member, strict bool) jobs.Step {
	return jobs.Step{
		Name: "cordon " + m.Hostname,
		Do: func(ctx context.Context, job *model.Job) error {
			detail := &job.Steps[job.Current].Detail
			client, err := s.deps.Kube(ctx, cluster)
			if err != nil {
				if strict {
					return fmt.Errorf("power: Kubernetes did not answer, so %s cannot be drained first; "+
						"the forced variant of this action does not need it: %w", m.Hostname, err)
				}
				*detail = "Kubernetes did not answer, so " + m.Hostname + " was left as it is"
				return nil
			}
			node, err := findNode(ctx, client, m)
			switch {
			case errors.Is(err, errNoKubeNode):
				*detail = "Kubernetes has no node for " + m.Hostname + "; there is nothing to cordon"
				return nil
			case err != nil:
				if strict {
					return err
				}
				*detail = "Kubernetes did not answer, so " + m.Hostname + " was left as it is"
				return nil
			case node.Unschedulable:
				*detail = m.Hostname + " was already cordoned"
				return nil
			}

			if err := s.updateMachine(ctx, m.Machine, func(rec *model.Machine) { rec.PowerCordoned = true }); err != nil {
				return err
			}
			if err := client.Cordon(ctx, node.Name, true); err != nil {
				return err
			}
			*detail = "cordoned; holzkube-manager uncordons it when it is started again"
			return nil
		},
		Happened: func(ctx context.Context, _ *model.Job) (bool, error) {
			client, err := s.deps.Kube(ctx, cluster)
			if err != nil {
				return false, nil //nolint:nilerr // cannot tell, and repeating the cordon is harmless
			}
			node, err := findNode(ctx, client, m)
			if err != nil {
				return false, nil //nolint:nilerr // as above
			}
			return node.Unschedulable, nil
		},
	}
}

// drain is the existing drain -- the eviction API, every budget honoured --
// against one node, skipped for a node that does not answer: nothing is running
// on a machine that is off, and evicting its pods through the API would only
// leave them terminating against a kubelet that is not there.
func (s *Service) drain(cluster model.ClusterID, m member) jobs.Step {
	return jobs.Step{
		Name: "drain " + m.Hostname,
		Do: func(ctx context.Context, job *model.Job) error {
			detail := &job.Steps[job.Current].Detail
			if !jobs.Reachable(ctx, s.deps.Connect, m.Machine) {
				*detail = m.Hostname + " does not answer, so nothing is running on it to move"
				return nil
			}
			client, err := s.deps.Kube(ctx, cluster)
			if err != nil {
				return err
			}
			node, err := findNode(ctx, client, m)
			if errors.Is(err, errNoKubeNode) {
				*detail = "Kubernetes has no node for " + m.Hostname + "; there is nothing to drain"
				return nil
			}
			if err != nil {
				return err
			}
			result, err := client.Drain(ctx, node.Name, drainOptions)
			*detail = describeDrain(result)
			return err
		},
		// "Is anything left here this drain would move" is a read and it is
		// the question the drain was asked, so an interrupted drain resumes.
		Happened: func(ctx context.Context, _ *model.Job) (bool, error) {
			if !jobs.Reachable(ctx, s.deps.Connect, m.Machine) {
				return true, nil
			}
			client, err := s.deps.Kube(ctx, cluster)
			if err != nil {
				return false, nil //nolint:nilerr // cannot tell; the drain is safe to repeat
			}
			node, err := findNode(ctx, client, m)
			if err != nil {
				return errors.Is(err, errNoKubeNode), nil
			}
			left, err := client.Undrained(ctx, node.Name, drainOptions)
			if err != nil {
				return false, nil //nolint:nilerr // as above
			}
			return len(left) == 0, nil
		},
	}
}

func describeDrain(r kube.DrainResult) string {
	parts := []string{fmt.Sprintf("%d evicted", len(r.Evicted))}
	if n := len(r.SkippedDaemonSet); n > 0 {
		parts = append(parts, fmt.Sprintf("%d DaemonSet pods left in place", n))
	}
	for _, b := range r.Blocked {
		parts = append(parts, b.Pod+": "+b.Reason)
	}
	return strings.Join(parts, "; ")
}

// remember marks, before a forced reboot, every node that is schedulable now:
// "uncordon if it was cordoned by this action" needs to know what it was before
// the action. Best effort, because a force-restart is exactly what somebody
// reaches for when the cluster is not answering -- a Kubernetes API that is
// down leaves nothing marked, and the nodes come back as they come back.
func (s *Service) remember(cluster model.ClusterID, members []member) jobs.Step {
	return jobs.Step{
		Name: "note which nodes are schedulable",
		Do: func(ctx context.Context, job *model.Job) error {
			detail := &job.Steps[job.Current].Detail
			client, err := s.deps.Kube(ctx, cluster)
			if err != nil {
				// Best effort, as above: the reboot this precedes does not
				// need Kubernetes, and refusing it for want of an API server
				// would refuse it in exactly the case it is for.
				*detail = "Kubernetes did not answer, so no node will be uncordoned afterwards"
				return nil //nolint:nilerr // deliberately not a failure; see above
			}
			var noted []string
			for _, m := range members {
				node, err := findNode(ctx, client, m)
				if err != nil || node.Unschedulable {
					continue
				}
				if err := s.updateMachine(ctx, m.Machine, func(rec *model.Machine) { rec.PowerCordoned = true }); err != nil {
					return err
				}
				noted = append(noted, m.Hostname)
			}
			*detail = "schedulable before the reboot: " + orNone(noted)
			return nil
		},
		Happened: never,
	}
}

// uncordon lets the scheduler use nodes again, and clears the mark.
//
// onlyMarked limits it to nodes this product cordoned -- the rule for
// everything but a plain start, which is the operator saying "bring this node
// back into service" and uncordons whatever cordoned it. A disabled node is
// never uncordoned: keeping it out of the scheduler is half of what disabled
// means.
func (s *Service) uncordon(cluster model.ClusterID, members []member, name string, onlyMarked bool) jobs.Step {
	return jobs.Step{
		Name: name,
		Do: func(ctx context.Context, job *model.Job) error {
			detail := &job.Steps[job.Current].Detail
			var targets []member
			for _, m := range members {
				rec, err := s.deps.Store.Machines().Get(ctx, m.Machine)
				if err != nil || rec.Disabled || (onlyMarked && !rec.PowerCordoned) {
					continue
				}
				targets = append(targets, m)
			}
			if len(targets) == 0 {
				*detail = "nothing to uncordon"
				return nil
			}

			// A node that has just booted -- or a control plane that has just
			// come back -- takes a while to have an API server in front of it,
			// so the client is waited for rather than asked once.
			var client *kube.Client
			err := s.poll(ctx, s.deps.KubeBudget, func() bool {
				c, err := s.deps.Kube(ctx, cluster)
				if err != nil {
					return false
				}
				if _, err := c.ServerVersion(ctx); err != nil {
					return false
				}
				client = c
				return true
			})
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return fmt.Errorf("power: Kubernetes did not answer within %s, so %s could not be "+
					"uncordoned; start it again once the API is back", s.deps.KubeBudget, names(targets))
			}

			var done, absent []string
			for _, m := range targets {
				node, err := findNode(ctx, client, m)
				if errors.Is(err, errNoKubeNode) {
					absent = append(absent, m.Hostname)
					continue
				}
				if err != nil {
					return err
				}
				if err := client.Cordon(ctx, node.Name, false); err != nil {
					return err
				}
				if err := s.updateMachine(ctx, m.Machine, func(rec *model.Machine) { rec.PowerCordoned = false }); err != nil {
					return err
				}
				done = append(done, m.Hostname)
			}
			*detail = "uncordoned " + orNone(done)
			if len(absent) > 0 {
				*detail += "; Kubernetes has no node for " + strings.Join(absent, ", ")
			}
			return nil
		},
		Happened: never,
	}
}

// markMachine sets or clears a node's disabled mark.
func (s *Service) markMachine(m member, disabled bool) jobs.Step {
	name := "mark " + m.Hostname + " disabled"
	if !disabled {
		name = "clear " + m.Hostname + "'s disabled mark"
	}
	return jobs.Step{
		Name: name,
		Do: func(ctx context.Context, job *model.Job) error {
			if err := s.updateMachine(ctx, m.Machine, func(rec *model.Machine) { rec.Disabled = disabled }); err != nil {
				return err
			}
			if disabled {
				job.Steps[job.Current].Detail = m.Hostname + " stays off until it is enabled"
			}
			return nil
		},
		Happened: func(ctx context.Context, _ *model.Job) (bool, error) {
			rec, err := s.deps.Store.Machines().Get(ctx, m.Machine)
			if err != nil {
				return false, err
			}
			return rec.Disabled == disabled, nil
		},
	}
}

// markCluster sets or clears a cluster's disabled mark. It is the first step
// of a cluster disable rather than the last, so that a start that arrives while
// the stop is still walking is refused instead of racing it.
func (s *Service) markCluster(cluster model.ClusterID, disabled bool) jobs.Step {
	name := "mark the cluster disabled"
	if !disabled {
		name = "clear the cluster's disabled mark"
	}
	return jobs.Step{
		Name: name,
		Do: func(ctx context.Context, _ *model.Job) error {
			return s.updateCluster(ctx, cluster, func(c *model.Cluster) { c.Disabled = disabled })
		},
		Happened: func(ctx context.Context, _ *model.Job) (bool, error) {
			c, err := s.deps.Store.Clusters().Get(ctx, cluster)
			if err != nil {
				return false, err
			}
			return c.Disabled == disabled, nil
		},
	}
}

// refuseDisabled stops a node start at a disabled node, or a node of a
// disabled cluster, even when the report said it could go -- the mark may have
// been set between the button and the job.
func (s *Service) refuseDisabled(cluster model.ClusterID, m member) jobs.Step {
	return jobs.Step{
		Name: "check " + m.Hostname + " is not disabled",
		Do: func(ctx context.Context, _ *model.Job) error {
			rec, err := s.deps.Store.Machines().Get(ctx, m.Machine)
			if err != nil {
				return err
			}
			if rec.Disabled {
				return fmt.Errorf("power: %s is disabled; enable it to start it", m.Hostname)
			}
			if c, err := s.deps.Store.Clusters().Get(ctx, cluster); err == nil && c.Disabled {
				return errors.New("power: its cluster is disabled; enable the cluster first")
			}
			return nil
		},
		Happened: never,
	}
}

func (s *Service) refuseClusterDisabled(cluster model.ClusterID) jobs.Step {
	return jobs.Step{
		Name: "check the cluster is not disabled",
		Do: func(ctx context.Context, _ *model.Job) error {
			c, err := s.deps.Store.Clusters().Get(ctx, cluster)
			if err != nil {
				return err
			}
			if c.Disabled {
				return errors.New("power: the cluster is disabled; enable it to start it")
			}
			return nil
		},
		Happened: never,
	}
}

// active is the members that are not disabled, read now rather than when the
// job was submitted: a node disabled while a cluster start is walking is
// skipped by the rest of the walk.
func (s *Service) active(ctx context.Context, members []member) ([]member, error) {
	var out []member
	for _, m := range members {
		rec, err := s.deps.Store.Machines().Get(ctx, m.Machine)
		if err != nil {
			return nil, err
		}
		if !rec.Disabled {
			out = append(out, m)
		}
	}
	return out, nil
}

// wake sends Wake-on-LAN to every member that is not disabled and does not
// answer. strictMAC makes a member with no known address a failure, which is
// right for one node and wrong for a cluster, where the others should still be
// woken and the wait names the one that did not come.
func (s *Service) wake(members []member, name string, strictMAC bool) jobs.Step {
	return jobs.Step{
		Name: name,
		Do: func(ctx context.Context, job *model.Job) error {
			detail := &job.Steps[job.Current].Detail
			targets, err := s.active(ctx, members)
			if err != nil {
				return err
			}

			var macs []net.HardwareAddr
			var woken, noMAC, already []string
			for _, m := range targets {
				if jobs.Reachable(ctx, s.deps.Connect, m.Machine) {
					already = append(already, m.Hostname)
					continue
				}
				rec, err := s.deps.Store.Machines().Get(ctx, m.Machine)
				if err != nil {
					return err
				}
				addrs := macsOf(rec)
				if len(addrs) == 0 {
					noMAC = append(noMAC, m.Hostname)
					continue
				}
				for _, a := range addrs {
					mac, _ := net.ParseMAC(a)
					macs = append(macs, mac)
				}
				woken = append(woken, m.Hostname+" ("+strings.Join(addrs, ", ")+")")
			}

			if len(noMAC) > 0 && (strictMAC || len(woken) == 0) {
				return fmt.Errorf("power: no network card address is known for %s, so it cannot be "+
					"woken over the network; power it on by hand", strings.Join(noMAC, ", "))
			}
			if len(macs) == 0 {
				*detail = "already answering: " + orNone(already) + "; nothing to wake"
				return nil
			}

			report, err := s.deps.Waker.Wake(ctx, macs)
			if err != nil {
				return err
			}
			*detail = "sent Wake-on-LAN for " + strings.Join(woken, ", ") + " to " +
				strings.Join(report.Destinations, ", ")
			if len(noMAC) > 0 {
				*detail += "; no address is known for " + strings.Join(noMAC, ", ") + ", which has to be powered on by hand"
			}
			return nil
		},
		// Every node it would wake already answers: sending the packet again
		// would change nothing, and not sending it is not a risk either -- the
		// wait that follows is what finds out.
		Happened: func(ctx context.Context, _ *model.Job) (bool, error) {
			targets, err := s.active(ctx, members)
			if err != nil {
				return false, nil //nolint:nilerr // a wake is harmless to repeat
			}
			for _, m := range targets {
				if !jobs.Reachable(ctx, s.deps.Connect, m.Machine) {
					return false, nil
				}
			}
			return true, nil
		},
	}
}

// waitAnswer waits for every member that is not disabled to answer its API.
func (s *Service) waitAnswer(members []member, name string) jobs.Step {
	return jobs.Step{
		Name: name,
		Do: func(ctx context.Context, job *model.Job) error {
			targets, err := s.active(ctx, members)
			if err != nil {
				return err
			}
			started := time.Now()
			var missing []member
			err = s.poll(ctx, s.deps.BootBudget, func() bool {
				missing = missing[:0]
				for _, m := range targets {
					if !jobs.Reachable(ctx, s.deps.Connect, m.Machine) {
						missing = append(missing, m)
					}
				}
				return len(missing) == 0
			})
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return fmt.Errorf("power: %s did not answer within %s. Wake-on-LAN may be switched "+
					"off in its firmware, or it is on a network a broadcast from here does not reach",
					names(missing), s.deps.BootBudget)
			}
			job.Steps[job.Current].Detail = fmt.Sprintf("%s answering after %s",
				orNone(hostnames(targets)), time.Since(started).Round(time.Second))
			return nil
		},
		Happened: never,
	}
}

// waitKube waits for the cluster's Kubernetes API to answer, which is what a
// control plane coming back up means for everything after it.
func (s *Service) waitKube(cluster model.ClusterID) jobs.Step {
	return jobs.Step{
		Name: "wait for the Kubernetes API",
		Do: func(ctx context.Context, job *model.Job) error {
			started := time.Now()
			var version string
			err := s.poll(ctx, s.deps.BootBudget, func() bool {
				c, err := s.deps.Kube(ctx, cluster)
				if err != nil {
					return false
				}
				version, err = c.ServerVersion(ctx)
				return err == nil
			})
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return fmt.Errorf("power: the Kubernetes API did not answer within %s of the control "+
					"plane being woken", s.deps.BootBudget)
			}
			job.Steps[job.Current].Detail = fmt.Sprintf("Kubernetes %s answering after %s",
				version, time.Since(started).Round(time.Second))
			return nil
		},
		Happened: never,
	}
}

// waitBack waits for a rebooted node to be back: answering, booted since the
// reboot was asked for, and -- when needReady -- its kubelet reporting Ready.
//
// "Booted since" is the reboot step's own evidence, asked of the step's own
// start: a node that is still answering in the seconds between accepting a
// reboot and going down has not rebooted, and taking its answer for "back"
// would uncordon it before it left.
func (s *Service) waitBack(cluster model.ClusterID, m member, rebootStep int, needReady bool) jobs.Step {
	return jobs.Step{
		Name: "wait for " + m.Hostname + " to come back",
		Do: func(ctx context.Context, job *model.Job) error {
			since := job.Steps[rebootStep].StartedAt
			started := time.Now()
			err := s.poll(ctx, s.deps.BootBudget, func() bool {
				back, err := jobs.RebootedSince(ctx, s.deps.Connect, m.Machine, since)
				if err != nil || !back {
					return false
				}
				return !needReady || s.kubeReady(ctx, cluster, m)
			})
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				what := "answering again"
				if needReady {
					what = "answering and Ready"
				}
				return fmt.Errorf("power: %s is not back (%s) within %s of its reboot. Nothing after "+
					"it has been touched", m.Hostname, what, s.deps.BootBudget)
			}
			job.Steps[job.Current].Detail = fmt.Sprintf("%s is back after %s",
				m.Hostname, time.Since(started).Round(time.Second))
			return nil
		},
		Happened: never,
	}
}

func (s *Service) kubeReady(ctx context.Context, cluster model.ClusterID, m member) bool {
	client, err := s.deps.Kube(ctx, cluster)
	if err != nil {
		return false
	}
	node, err := findNode(ctx, client, m)
	return err == nil && node.Ready == "True"
}

// forceOffAll shuts every node down at once, without a drain and without
// Talos's own cordon: the cluster force-stop.
func (s *Service) forceOffAll(members []member) jobs.Step {
	return jobs.Step{
		Name: "force every node off at once",
		Do: func(ctx context.Context, job *model.Job) error {
			errs := s.fanOut(members, func(m member) error {
				if !jobs.Reachable(ctx, s.deps.Connect, m.Machine) {
					return nil
				}
				callCtx, cancel, err := talos.WithClassDeadline(ctx, talos.MethodShutdown)
				if err != nil {
					return err
				}
				defer cancel()
				cc, err := s.deps.Connect(callCtx, m.Machine)
				if err != nil {
					return err
				}
				defer cc.Close() //nolint:errcheck // the verdict is the RPC's
				return cc.ShutdownForced(callCtx)
			})
			if len(errs) > 0 {
				return fmt.Errorf("power: %s", strings.Join(errs, "; "))
			}
			job.Steps[job.Current].Detail = "every node accepted the shutdown: " + orNone(hostnames(members))
			return nil
		},
		// None of them answers, which is what every node off looks like.
		Happened: func(ctx context.Context, _ *model.Job) (bool, error) {
			for _, m := range members {
				if jobs.Reachable(ctx, s.deps.Connect, m.Machine) {
					return false, nil
				}
			}
			return true, nil
		},
	}
}

// rebootAll reboots every node at once in Talos's FORCE mode.
func (s *Service) rebootAll(members []member) jobs.Step {
	return jobs.Step{
		Name: "force-reboot every node at once",
		Do: func(ctx context.Context, job *model.Job) error {
			errs := s.fanOut(members, func(m member) error {
				callCtx, cancel, err := talos.WithClassDeadline(ctx, talos.MethodReboot)
				if err != nil {
					return err
				}
				defer cancel()
				cc, err := s.deps.Connect(callCtx, m.Machine)
				if err != nil {
					return err
				}
				defer cc.Close() //nolint:errcheck // the verdict is the RPC's
				return cc.RebootForced(callCtx)
			})
			if len(errs) > 0 {
				return fmt.Errorf("power: %s", strings.Join(errs, "; "))
			}
			job.Steps[job.Current].Detail = "every node accepted the reboot: " + orNone(hostnames(members))
			return nil
		},
		// Every node has rebooted since this step started. One that does not
		// answer cannot say, and "reboot it again, to be sure" is the doubled
		// side effect this engine exists to refuse -- so that parks.
		Happened: func(ctx context.Context, job *model.Job) (bool, error) {
			since := job.Steps[job.Current].StartedAt
			for _, m := range members {
				back, err := jobs.RebootedSince(ctx, s.deps.Connect, m.Machine, since)
				if err != nil {
					return false, err
				}
				if !back {
					return false, nil
				}
			}
			return true, nil
		},
	}
}

// waitAllBack waits for every node of a forced reboot to have booted again.
func (s *Service) waitAllBack(members []member, rebootStep int) jobs.Step {
	return jobs.Step{
		Name: "wait for every node to come back",
		Do: func(ctx context.Context, job *model.Job) error {
			since := job.Steps[rebootStep].StartedAt
			started := time.Now()
			var missing []member
			err := s.poll(ctx, s.deps.BootBudget, func() bool {
				missing = missing[:0]
				for _, m := range members {
					back, err := jobs.RebootedSince(ctx, s.deps.Connect, m.Machine, since)
					if err != nil || !back {
						missing = append(missing, m)
					}
				}
				return len(missing) == 0
			})
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return fmt.Errorf("power: %s did not come back within %s of the reboot",
					names(missing), s.deps.BootBudget)
			}
			job.Steps[job.Current].Detail = fmt.Sprintf("every node back after %s",
				time.Since(started).Round(time.Second))
			return nil
		},
		Happened: never,
	}
}

// fanOut runs one call per member concurrently and collects the failures,
// each named after its node. "At once" is the whole of what the forced cluster
// verbs promise, and one slow node must not hold the others back.
func (s *Service) fanOut(members []member, call func(member) error) []string {
	var (
		mu   sync.Mutex
		errs []string
		wg   sync.WaitGroup
	)
	for _, m := range members {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := call(m); err != nil {
				mu.Lock()
				errs = append(errs, m.Hostname+": "+err.Error())
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return errs
}

// poll calls check until it says yes or the budget is spent.
func (s *Service) poll(ctx context.Context, budget time.Duration, check func() bool) error {
	deadline := time.Now().Add(budget)
	for {
		if check() {
			return nil
		}
		if time.Now().After(deadline) {
			return context.DeadlineExceeded
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(s.deps.PollInterval):
		}
	}
}

func hostnames(members []member) []string {
	out := make([]string, 0, len(members))
	for _, m := range members {
		out = append(out, m.Hostname)
	}
	return out
}

func names(members []member) string { return orNone(hostnames(members)) }

func orNone(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, ", ")
}
