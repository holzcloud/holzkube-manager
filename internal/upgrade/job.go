package upgrade

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/machineconfig"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// The rolling upgrade, as a job.
//
// One step per node, and that is the design rather than an implementation
// detail. A single step that walked every node would be a step whose
// interruption nobody can reason about: the engine would know the step was
// interrupted and not which node it was on, and the answer to "did it happen"
// would be different for every node in the list. One step per node makes each
// one individually verifiable -- the node is on the new version or it is not,
// and asking is a read.
//
// The health gate runs **inside** each node's step, immediately before that
// node is touched (UPG-02). Evaluating it once at the start would be
// evaluating it against a cluster that the previous node has since changed,
// which is exactly the cluster the requirement is about.

// JobKindTalosUpgrade and JobKindKubernetesUpgrade are the two rolling
// upgrades. They are separate kinds rather than one with a parameter because
// the engine's single-flight lease is per cluster and per kind, and because a
// resumed job must not have to work out which of two quite different
// operations it was.
const (
	JobKindTalosUpgrade      = model.JobKind("cluster.upgrade-talos")
	JobKindKubernetesUpgrade = model.JobKind("cluster.upgrade-kubernetes")
)

// ErrStopRequested reports that the operator asked to stop after the current
// node (UPG-08).
var ErrStopRequested = errors.New("upgrade: stopped after the current node, as asked")

// Deps is what the rolling upgrade needs.
type Deps struct {
	// Connect opens a client to one machine.
	Connect func(ctx context.Context, id model.MachineID) (*talos.ClusterClient, error)

	// Machines lists a cluster's machines, control plane first. The order is
	// the caller's to decide and it matters: control-plane nodes go first,
	// because a worker upgraded against an older control plane is the version
	// skew Kubernetes forbids in the other direction.
	Machines func(ctx context.Context, id model.ClusterID) ([]model.Machine, error)

	// RoleOf reports one machine's role.
	//
	// It is here rather than derived from Machines because a restore names a
	// node and not a cluster: the operator is choosing which member's data
	// becomes the cluster's, and asking for the cluster first to find the role
	// would be asking a question whose answer this code would then have to
	// pick from.
	RoleOf func(ctx context.Context, id model.MachineID) (model.MachineRole, error)

	// Gate is the etcd health gate, re-evaluated before every node.
	Gate *Gate

	// Record writes back what a node ended up running, so the inventory shows
	// the new version without waiting for the next observation pass.
	Record func(ctx context.Context, id model.MachineID) error

	// ResolveInstaller resolves the installer reference for one node, with
	// that node's SecureBoot state in it. It belongs to the composition root
	// because resolving means asking the Image Factory.
	ResolveInstaller ResolveInstaller
}

// Register teaches a job engine both rolling upgrades.
func Register(e *jobs.Engine, d Deps) {
	e.Register(JobKindTalosUpgrade, func(j model.Job) ([]jobs.Step, error) {
		req, err := RequestFromParams(j.Params)
		if err != nil {
			return nil, err
		}
		return talosSteps(d, req, j.Cluster)
	})

	e.Register(JobKindKubernetesUpgrade, func(j model.Job) ([]jobs.Step, error) {
		req, err := RequestFromParams(j.Params)
		if err != nil {
			return nil, err
		}
		return kubernetesSteps(d, req, j.Cluster)
	})
}

// Request is a rolling upgrade as the operator assembled it.
type Request struct {
	// To is the version this run installs. It is one version and not a chain:
	// a chain is several runs, and collapsing them into one would hide the
	// health gate between the steps that most need it.
	To string `json:"to"`

	// Machines are the nodes to walk, in order, by UUID. They are named
	// explicitly rather than derived at run time so that a resumed job walks
	// the nodes it was submitted for -- a cluster that gained a node while the
	// job was interrupted must not have it silently included.
	Machines []model.MachineID `json:"machines"`

	// Schematics maps each machine to the Image Factory schematic it was
	// installed from, read from the node at plan time (UPG-03).
	Schematics map[model.MachineID]string `json:"schematics,omitempty"`

	// Installers is the exact installer reference each node will be upgraded
	// with, resolved when the plan was made.
	//
	// Resolved and carried, not rebuilt here, and the reason is the defect
	// this replaced: the reference used to be assembled from parts at step
	// time as "factory.talos.dev/installer/<id>:<version>". It never carried
	// SecureBoot, so upgrading a SecureBoot node installed the ordinary
	// installer -- which does not produce a SecureBoot node -- and took
	// SecureBoot away from a machine that had it, on a path an operator runs
	// against a cluster they depend on, with nothing in the result saying so.
	Installers map[model.MachineID]string `json:"installers,omitempty"`
}

// talosSteps builds one step per node.
func talosSteps(d Deps, req Request, cluster model.ClusterID) ([]jobs.Step, error) {
	to, err := ParseVersion(req.To)
	if err != nil {
		return nil, err
	}
	if len(req.Machines) == 0 {
		return nil, errors.New("upgrade: this run names no nodes")
	}

	steps := make([]jobs.Step, 0, len(req.Machines))
	for _, id := range req.Machines {
		installer := req.Installers[id]
		if installer == "" {
			return nil, fmt.Errorf("upgrade: this run carries no installer reference for %s. "+
				"It is resolved against the Image Factory when the plan is made -- with that "+
				"node's SecureBoot state in it -- and a step that assembled one here would "+
				"install something the operator never saw", id)
		}
		steps = append(steps, talosNodeStep(d, cluster, id, to, req.Schematics[id], installer))
	}
	return steps, nil
}

func talosNodeStep(
	d Deps,
	cluster model.ClusterID,
	id model.MachineID,
	to Version,
	schematic string,
	installer string,
) jobs.Step {

	return jobs.Step{
		Name: "upgrade " + string(id) + " to " + to.String(),

		Do: func(ctx context.Context, job *model.Job) error {
			machines, err := d.Machines(ctx, cluster)
			if err != nil {
				return err
			}
			me, ok := machineByID(machines, id)
			if !ok {
				return fmt.Errorf("upgrade: %s is no longer in the inventory", id)
			}

			// UPG-14. A locked node is skipped, not failed: the lock is
			// somebody saying "not this one", and stopping the whole run
			// because of it would make the lock a blunt instrument.
			if me.Locked {
				job.Steps[job.Current].Detail = SkipReason(me)
				return nil
			}

			// UPG-02, here and not at the start of the run: this node is being
			// taken down in a cluster the previous node just changed.
			if me.Role == model.RoleControlPlane {
				verdict, err := d.Gate.Evaluate(ctx, cluster, id)
				if err != nil {
					return err
				}
				if !verdict.OK {
					return fmt.Errorf("upgrade: the health gate refused before %s: %s",
						nameOf(me), verdict.Reason)
				}
				job.Steps[job.Current].Detail = "health gate passed: " +
					strconv.Itoa(verdict.Input.Voting) + " voting etcd members, no alarms"
			}

			cc, err := d.Connect(ctx, id)
			if err != nil {
				return err
			}

			// The image is pulled before anything is replaced. A wrong
			// installer reference, an unreachable registry or a schematic that
			// does not exist fails here, while the node is still running the
			// system it is running -- which costs nothing.
			pullCtx, cancel, err := talos.WithClassDeadline(ctx, talos.MethodImagePull)
			if err != nil {
				_ = cc.Close()
				return err
			}
			err = cc.ImagePull(pullCtx, installer)
			cancel()
			if err != nil {
				_ = cc.Close()
				return fmt.Errorf("upgrade: pulling %s onto %s: %w", installer, nameOf(me), err)
			}

			// A control-plane node that holds etcd leadership hands it over
			// first. Skipping this is not a failure so much as an avoidable
			// stall: etcd elects a new leader on its own after a timeout, and
			// everything that writes to the cluster waits for it.
			if me.Role == model.RoleControlPlane {
				forfeitCtx, cancel, err := talos.WithClassDeadline(ctx, talos.MethodEtcdForfeitLeadership)
				if err == nil {
					if to, ferr := cc.EtcdForfeitLeadership(forfeitCtx); ferr == nil && to != "" {
						job.Steps[job.Current].Detail = "etcd leadership handed to " + to
					}
					cancel()
				}
			}

			stream, err := cc.Upgrade(ctx, installer)
			if err != nil {
				_ = cc.Close()
				return err
			}

			var lines []string
			drainErr := stream.Drain(func(p talos.UpgradeProgress) {
				if p.Message == "" {
					return
				}
				lines = append(lines, p.Message)
				job.Steps[job.Current].Detail = p.Message
			})
			_ = stream.Close()
			_ = cc.Close()

			if drainErr != nil && !isNodeLeaving(drainErr) {
				return drainErr
			}
			if len(lines) > 0 {
				job.Steps[job.Current].Detail = lines[len(lines)-1] + "; waiting for the node to come back"
			}

			// UPG-07. "The API said OK" is not proof: the stream ending is the
			// node leaving, and a node that fell over looks the same from
			// here. What settles it is the node, afterwards, reporting the
			// version that was installed -- and its own services running.
			obs, err := waitForUpgrade(ctx, d, id, to, schematic)
			if err != nil {
				return err
			}
			job.Steps[job.Current].Detail = fmt.Sprintf(
				"%s is running Talos %s and its services are up", nameOf(me), obs.TalosVersion)

			if d.Record != nil {
				return d.Record(ctx, id)
			}
			return nil
		},

		// Verifiable, and this is the one place in this product where a step's
		// verification and its acceptance criterion are literally the same
		// read. A node that is already on the target version has had this step
		// happen to it, whatever interrupted the job.
		Happened: func(ctx context.Context, _ *model.Job) (bool, error) {
			cc, err := d.Connect(ctx, id)
			if err != nil {
				// A node that cannot be reached has not been shown to be
				// upgraded. Not an error: the step simply runs, and its first
				// act is the same connection failing more usefully.
				return false, nil //nolint:nilerr // see above
			}
			defer cc.Close() //nolint:errcheck // the verdict is the read's

			if _, err := VerifyUpgraded(ctx, cc, to, schematic); err != nil {
				return false, nil //nolint:nilerr // not upgraded, which is the answer, not a failure
			}
			return true, nil
		},
	}
}

// kubernetesSteps upgrades the Kubernetes control plane and the kubelets.
//
// It walks the same nodes in the same order and through the same gate, and the
// difference is what it writes: a Kubernetes upgrade is a configuration change
// to the static pod and kubelet images rather than a new system on disk, so
// there is no installer and no reboot. What it shares with the Talos path is
// the thing that matters -- the gate runs before every node, and the result is
// verified against the node rather than against the call's return value.
func kubernetesSteps(d Deps, req Request, cluster model.ClusterID) ([]jobs.Step, error) {
	to, err := ParseVersion(req.To)
	if err != nil {
		return nil, err
	}
	if len(req.Machines) == 0 {
		return nil, errors.New("upgrade: this run names no nodes")
	}

	steps := make([]jobs.Step, 0, len(req.Machines))
	for _, id := range req.Machines {
		steps = append(steps, kubernetesNodeStep(d, cluster, id, to))
	}
	return steps, nil
}

func kubernetesNodeStep(d Deps, cluster model.ClusterID, id model.MachineID, to Version) jobs.Step {
	return jobs.Step{
		Name: "move " + string(id) + " to Kubernetes " + to.String(),

		Do: func(ctx context.Context, job *model.Job) error {
			machines, err := d.Machines(ctx, cluster)
			if err != nil {
				return err
			}
			me, ok := machineByID(machines, id)
			if !ok {
				return fmt.Errorf("upgrade: %s is no longer in the inventory", id)
			}
			if me.Locked {
				job.Steps[job.Current].Detail = SkipReason(me)
				return nil
			}

			if me.Role == model.RoleControlPlane {
				verdict, err := d.Gate.Evaluate(ctx, cluster, id)
				if err != nil {
					return err
				}
				if !verdict.OK {
					return fmt.Errorf("upgrade: the health gate refused before %s: %s",
						nameOf(me), verdict.Reason)
				}
			}

			// The version the node is asked to run is written into its machine
			// configuration, which is the same mechanism a configuration apply
			// uses -- deliberately, because a Kubernetes version that arrived
			// by some other path would be a second source of truth about what
			// this node runs.
			patch, paths := kubernetesPatch(to, me.Role == model.RoleControlPlane)

			// The mode is computed from the paths this patch touches, through
			// the same whitelist a configuration apply goes through. It is
			// `no-reboot` for every path here -- .machine.kubelet and .cluster
			// are both on the list -- and computing it rather than writing it
			// down is what keeps this honest the day somebody adds a path to
			// the patch that does need a restart.
			verdict := machineconfig.ModeFor(paths)

			cc, err := d.Connect(ctx, id)
			if err != nil {
				return err
			}
			applyCtx, cancel, err := talos.WithClassDeadline(ctx, talos.MethodApplyConfiguration)
			if err != nil {
				_ = cc.Close()
				return err
			}
			_, err = cc.ApplyConfigurationWithMode(applyCtx, patch, string(verdict.Mode))
			cancel()
			_ = cc.Close()
			if err != nil {
				return err
			}

			obs, err := waitForKubernetes(ctx, d, id, to)
			if err != nil {
				return err
			}
			job.Steps[job.Current].Detail = fmt.Sprintf("%s reports Kubernetes %s",
				nameOf(me), obs.KubernetesVersion)

			if d.Record != nil {
				return d.Record(ctx, id)
			}
			return nil
		},

		Happened: func(ctx context.Context, _ *model.Job) (bool, error) {
			cc, err := d.Connect(ctx, id)
			if err != nil {
				return false, nil //nolint:nilerr // unreachable is not "done"
			}
			defer cc.Close() //nolint:errcheck // the verdict is the read's

			factsCtx, cancel, err := talos.WithClassDeadline(ctx, talos.MethodCOSIList)
			if err != nil {
				return false, err
			}
			defer cancel()

			facts, err := cc.NodeFacts(factsCtx)
			if err != nil {
				return false, nil //nolint:nilerr // as above
			}
			got, err := ParseVersion(facts.KubernetesVersion())
			if err != nil {
				return false, nil //nolint:nilerr // a version that cannot be read is not the target
			}
			return got.Major == to.Major && got.Minor == to.Minor && got.Patch == to.Patch, nil
		},
	}
}

// ReappearBudget is how long a node gets to come back from an upgrade.
//
// It is the same shape as provisioning's: an expectation rather than a
// timeout, and the waiting below reports elapsed time against it. The number
// is larger than a provisioning install because an upgrade writes a system
// *and* restarts everything that was running on the node.
const ReappearBudget = 12 * time.Minute

// waitForUpgrade waits for a node to come back and verifies it.
func waitForUpgrade(
	ctx context.Context,
	d Deps,
	id model.MachineID,
	to Version,
	schematic string,
) (Observed, error) {
	started := time.Now()

	for {
		select {
		case <-ctx.Done():
			return Observed{}, ctx.Err()
		case <-time.After(10 * time.Second):
		}

		cc, err := d.Connect(ctx, id)
		if err == nil {
			obs, verr := VerifyUpgraded(ctx, cc, to, schematic)
			_ = cc.Close()
			switch {
			case verr == nil:
				return obs, nil
			case errors.Is(verr, ErrUnknownSchematic):
				// The node came back, on the right version, with the wrong
				// image. Waiting will not change that, and it is the failure
				// this whole check exists for.
				return obs, verr
			}
		}

		if time.Since(started) > ReappearBudget {
			return Observed{}, fmt.Errorf(
				"upgrade: %s has not come back as %s after %s (about %s is expected). It has not "+
					"necessarily failed -- an upgrade on a slow disk takes longer -- but this is "+
					"the point at which the node's own console is worth looking at",
				id, to, round(time.Since(started)), round(ReappearBudget))
		}
	}
}

// waitForKubernetes waits for a node's kubelet to report the new version.
func waitForKubernetes(ctx context.Context, d Deps, id model.MachineID, to Version) (Observed, error) {
	started := time.Now()

	for {
		select {
		case <-ctx.Done():
			return Observed{}, ctx.Err()
		case <-time.After(10 * time.Second):
		}

		cc, err := d.Connect(ctx, id)
		if err == nil {
			factsCtx, cancel, cerr := talos.WithClassDeadline(ctx, talos.MethodCOSIList)
			if cerr == nil {
				facts, ferr := cc.NodeFacts(factsCtx)
				cancel()
				if ferr == nil {
					got, perr := ParseVersion(facts.KubernetesVersion())
					if perr == nil && got.Major == to.Major && got.Minor == to.Minor && got.Patch == to.Patch {
						_ = cc.Close()
						return Observed{KubernetesVersion: facts.KubernetesVersion()}, nil
					}
				}
			}
			_ = cc.Close()
		}

		if time.Since(started) > ReappearBudget {
			return Observed{}, fmt.Errorf(
				"upgrade: %s has not reported Kubernetes %s after %s. The configuration was "+
					"applied; what has not happened is the kubelet coming up on the new version",
				id, to, round(time.Since(started)))
		}
	}
}

// kubernetesPatch is the configuration change a Kubernetes upgrade writes.
//
// A control-plane node carries the three static pods as well as the kubelet; a
// worker carries only the kubelet. Writing the control-plane images onto a
// worker would be writing configuration for components it does not run, which
// Talos accepts and nobody can read afterwards.
func kubernetesPatch(to Version, controlPlane bool) (patch []byte, paths []string) {
	v := to.String()

	var b strings.Builder
	b.WriteString("machine:\n  kubelet:\n    image: ghcr.io/siderolabs/kubelet:" + v + "\n")
	paths = append(paths, ".machine.kubelet.image")

	if controlPlane {
		b.WriteString("cluster:\n")
		b.WriteString("  apiServer:\n    image: registry.k8s.io/kube-apiserver:" + v + "\n")
		b.WriteString("  controllerManager:\n    image: registry.k8s.io/kube-controller-manager:" + v + "\n")
		b.WriteString("  scheduler:\n    image: registry.k8s.io/kube-scheduler:" + v + "\n")
		paths = append(paths,
			".cluster.apiServer.image",
			".cluster.controllerManager.image",
			".cluster.scheduler.image")
	}
	return []byte(b.String()), paths
}

// isNodeLeaving reports an error that is the node rebooting into what it just
// installed rather than an upgrade failing.
//
// The distinction cannot be made perfectly and it is made conservatively: a
// connection that goes away after the installer has been writing is what a
// successful upgrade looks like, and treating it as a failure would fail every
// successful upgrade. What settles it either way is the verification
// afterwards, which is why being wrong here costs a sentence and not a verdict.
func isNodeLeaving(err error) bool {
	kind, ok := talos.ErrorKindOf(err)
	if !ok {
		return false
	}
	return kind == talos.KindUnreachable || kind == talos.KindTimeout
}

func machineByID(machines []model.Machine, id model.MachineID) (model.Machine, bool) {
	for _, m := range machines {
		if m.ID == id {
			return m, true
		}
	}
	return model.Machine{}, false
}

// Params turns a request into the job's stored parameters.
func (r Request) Params() map[string]string {
	out := map[string]string{"to": r.To}

	ids := make([]string, 0, len(r.Machines))
	for _, id := range r.Machines {
		ids = append(ids, string(id))
	}
	out["machines"] = strings.Join(ids, ",")

	if len(r.Schematics) > 0 {
		pairs := make([]string, 0, len(r.Schematics))
		for id, schematic := range r.Schematics {
			pairs = append(pairs, string(id)+"="+schematic)
		}
		sortStrings(pairs)
		out["schematics"] = strings.Join(pairs, ",")
	}
	if len(r.Installers) > 0 {
		pairs := make([]string, 0, len(r.Installers))
		for id, installer := range r.Installers {
			pairs = append(pairs, string(id)+"="+installer)
		}
		sortStrings(pairs)
		out["installers"] = strings.Join(pairs, ",")
	}
	return out
}

// RequestFromParams rebuilds a request from a stored job.
//
// Every omission is refused rather than defaulted, for the reason the reset
// job's parameters are: a resumed job that filled in a default would be
// upgrading a different set of nodes than the one somebody confirmed.
func RequestFromParams(params map[string]string) (Request, error) {
	r := Request{To: params["to"]}
	if r.To == "" {
		return Request{}, errors.New("upgrade: this job names no version to upgrade to")
	}

	raw := params["machines"]
	if raw == "" {
		return Request{}, errors.New("upgrade: this job names no nodes to walk")
	}
	for _, id := range strings.Split(raw, ",") {
		if id = strings.TrimSpace(id); id != "" {
			r.Machines = append(r.Machines, model.MachineID(id))
		}
	}

	if s := params["installers"]; s != "" {
		r.Installers = map[model.MachineID]string{}
		for _, pair := range strings.Split(s, ",") {
			id, installer, ok := strings.Cut(pair, "=")
			if !ok {
				return Request{}, fmt.Errorf("upgrade: %q is not a machine=installer pair", pair)
			}
			r.Installers[model.MachineID(id)] = installer
		}
	}

	if s := params["schematics"]; s != "" {
		r.Schematics = map[model.MachineID]string{}
		for _, pair := range strings.Split(s, ",") {
			id, schematic, ok := strings.Cut(pair, "=")
			if !ok {
				return Request{}, fmt.Errorf("upgrade: %q is not a machine=schematic pair", pair)
			}
			r.Schematics[model.MachineID(id)] = schematic
		}
	}
	return r, nil
}

func round(d time.Duration) string {
	if d < time.Minute {
		return strconv.Itoa(int(d.Seconds())) + "s"
	}
	return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
