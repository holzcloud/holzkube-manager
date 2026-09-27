package upgrade

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/siderolabs/talos/pkg/machinery/config/configloader"

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

	// KubeProxy moves the kube-proxy DaemonSet to an image and waits for the
	// rollout, returning what it saw; ErrNoKubeProxy when the cluster runs
	// none. It speaks the Kubernetes API, which is the composition root's to
	// reach. Nil means no Kubernetes access: the configuration is still
	// updated, and the step says the DaemonSet was not.
	KubeProxy func(ctx context.Context, cluster model.ClusterID, image string) (string, error)
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

// kubernetesSteps upgrades the Kubernetes control plane and the kubelets, in
// the order talosctl upgrade-k8s uses (ledger 86):
//
//  1. pull every image the run will need onto every node, so no component is
//     restarted onto an image its node still has to download;
//  2. the API server on each control-plane node, then the controller manager,
//     then the scheduler -- the server before its clients, one node at a time,
//     each waited for until the node reports the pod back, ready, on the new
//     image;
//  3. kube-proxy, whose image lives both in the control-plane configuration
//     and in a DaemonSet in the Kubernetes API;
//  4. the kubelet on each node, control plane first.
//
// Every change is written the way a configuration apply writes one: the node's
// current configuration is read, the patch is merged into it, and the whole
// result is applied. The version the node runs therefore has one source, its
// configuration, and the health gate runs before every control-plane node.
func kubernetesSteps(d Deps, req Request, cluster model.ClusterID) ([]jobs.Step, error) {
	to, err := ParseVersion(req.To)
	if err != nil {
		return nil, err
	}
	if len(req.Machines) == 0 {
		return nil, errors.New("upgrade: this run names no nodes")
	}

	steps := make([]jobs.Step, 0, len(req.Machines)+5)
	steps = append(steps, prepullStep(d, cluster, req.Machines, to))
	for _, component := range talos.ControlPlaneComponents {
		steps = append(steps, componentStep(d, cluster, req.Machines, component, to))
	}
	steps = append(steps, kubeProxyStep(d, cluster, req.Machines, to))
	for _, id := range req.Machines {
		steps = append(steps, kubernetesNodeStep(d, cluster, id, to))
	}
	return steps, nil
}

// componentConfigKey is where each control-plane component's image lives in
// the machine configuration.
var componentConfigKey = map[string]string{
	"kube-apiserver":          "apiServer",
	"kube-controller-manager": "controllerManager",
	"kube-scheduler":          "scheduler",
}

// kubernetesImages are the images a node needs for a run: its kubelet, and on
// a control-plane node the three static pods and kube-proxy.
func kubernetesImages(to Version, controlPlane bool) (system, cri []string) {
	v := to.String()
	system = []string{kubeletImage(to)}
	if controlPlane {
		for _, c := range talos.ControlPlaneComponents {
			cri = append(cri, "registry.k8s.io/"+c+":"+v)
		}
	}
	cri = append(cri, proxyImage(to))
	return system, cri
}

func kubeletImage(to Version) string { return "ghcr.io/siderolabs/kubelet:" + to.String() }
func proxyImage(to Version) string   { return "registry.k8s.io/kube-proxy:" + to.String() }

// imageChange is one image a Kubernetes upgrade moves, in both of the shapes a
// node's configuration can hold it.
//
// Talos v1.14 generates a configuration in which every Kubernetes component is
// a document of its own -- KubeletConfig, KubeAPIServerConfig and so on, each
// with an image -- and its validator refuses the old v1alpha1 field set beside
// such a document ("kube-apiserver config is already set in v1alpha1 config").
// A configuration generated before v1.14 has only the v1alpha1 field. So the
// patch is decided per node, from the configuration the node actually has: the
// document when it carries one, the old field when it does not.
type imageChange struct {
	kind   string // the v1.14 document kind
	legacy string // the v1alpha1 patch
	path   string // the v1alpha1 path, which is also what the apply mode is read from
	image  string
}

var componentDocKind = map[string]string{
	"kube-apiserver":          "KubeAPIServerConfig",
	"kube-controller-manager": "KubeControllerManagerConfig",
	"kube-scheduler":          "KubeSchedulerConfig",
}

func componentChange(component string, to Version) imageChange {
	key := componentConfigKey[component]
	image := "registry.k8s.io/" + component + ":" + to.String()
	return imageChange{
		kind:   componentDocKind[component],
		legacy: "cluster:\n  " + key + ":\n    image: " + image + "\n",
		path:   ".cluster." + key + ".image",
		image:  image,
	}
}

func proxyChange(to Version) imageChange {
	return imageChange{
		kind:   "KubeProxyConfig",
		legacy: "cluster:\n  proxy:\n    image: " + proxyImage(to) + "\n",
		path:   ".cluster.proxy.image",
		image:  proxyImage(to),
	}
}

func kubeletChange(to Version) imageChange {
	return imageChange{
		kind:   "KubeletConfig",
		legacy: "machine:\n  kubelet:\n    image: " + kubeletImage(to) + "\n",
		path:   ".machine.kubelet.image",
		image:  kubeletImage(to),
	}
}

// patchFor is the patch this change needs against one node's configuration.
func (c imageChange) patchFor(base []byte) (string, error) {
	provider, err := configloader.NewFromBytes(base)
	if err != nil {
		return "", fmt.Errorf("upgrade: the node's configuration did not load: %w", err)
	}
	for _, d := range provider.Documents() {
		if d.Kind() == c.kind {
			return "apiVersion: v1alpha1\nkind: " + c.kind + "\nimage: " + c.image + "\n", nil
		}
	}
	return c.legacy, nil
}

// applyImage merges one image change into a node's current configuration and
// applies the whole result.
//
// The whole result, and that is the defect this replaced: the Kubernetes
// upgrade used to send its three image lines AS the configuration. A real node
// refuses a document that names no machine type, cluster or secrets, so every
// run failed at its first node; the simulator stored it, so every test passed.
func applyImage(ctx context.Context, d Deps, id model.MachineID, change imageChange) error {
	cc, err := d.Connect(ctx, id)
	if err != nil {
		return err
	}
	defer cc.Close() //nolint:errcheck // the verdict is the apply's

	readCtx, cancel, err := talos.WithClassDeadline(ctx, talos.MethodCOSIList)
	if err != nil {
		return err
	}
	base, err := cc.MachineConfigYAML(readCtx)
	cancel()
	if err != nil {
		return fmt.Errorf("upgrade: reading the configuration of %s: %w", id, err)
	}
	patch, err := change.patchFor(base)
	if err != nil {
		return err
	}
	full, err := machineconfig.ApplyPatches(base, []string{patch})
	if err != nil {
		return fmt.Errorf("upgrade: merging the image change into the configuration of %s: %w", id, err)
	}

	// The mode is computed from the paths, through the same whitelist a
	// configuration apply goes through. It is no-reboot for every path here,
	// and computing it rather than writing it down is what keeps this honest
	// the day a path that does need a restart is added.
	verdict := machineconfig.ModeFor([]string{change.path})

	applyCtx, cancel, err := talos.WithClassDeadline(ctx, talos.MethodApplyConfiguration)
	if err != nil {
		return err
	}
	defer cancel()
	_, err = cc.ApplyConfigurationWithMode(applyCtx, full, string(verdict.Mode))
	return err
}

// controlPlaneOf is the run's nodes that are control-plane nodes, in the run's
// order, read at step time because the inventory is the source of roles.
func controlPlaneOf(ctx context.Context, d Deps, cluster model.ClusterID, ids []model.MachineID) ([]model.Machine, error) {
	machines, err := d.Machines(ctx, cluster)
	if err != nil {
		return nil, err
	}
	var out []model.Machine
	for _, id := range ids {
		if m, ok := machineByID(machines, id); ok && m.Role == model.RoleControlPlane && !m.Locked {
			out = append(out, m)
		}
	}
	return out, nil
}

// prepullStep pulls every image the run will install, on every node, before
// anything is changed. Pulling is where an unreachable registry or a mistyped
// version fails, and failing there costs nothing: no component has been
// restarted yet.
func prepullStep(d Deps, cluster model.ClusterID, ids []model.MachineID, to Version) jobs.Step {
	return jobs.Step{
		Name: "pull the Kubernetes " + to.String() + " images onto every node",
		Do: func(ctx context.Context, job *model.Job) error {
			machines, err := d.Machines(ctx, cluster)
			if err != nil {
				return err
			}
			pulled := 0
			for _, id := range ids {
				me, ok := machineByID(machines, id)
				if !ok || me.Locked {
					continue
				}
				system, cri := kubernetesImages(to, me.Role == model.RoleControlPlane)
				if err := pullOnto(ctx, d, id, system, cri); err != nil {
					return fmt.Errorf("upgrade: pulling the images onto %s: %w", nameOf(me), err)
				}
				pulled++
			}
			job.Steps[job.Current].Detail = fmt.Sprintf("images pulled onto %d node(s)", pulled)
			return nil
		},
		// Pulling again is harmless and cheap once the images are there, so a
		// resumed run simply does it again rather than guessing.
	}
}

func pullOnto(ctx context.Context, d Deps, id model.MachineID, system, cri []string) error {
	cc, err := d.Connect(ctx, id)
	if err != nil {
		return err
	}
	defer cc.Close() //nolint:errcheck // the verdict is the pulls'

	pull := func(ref string, into func(context.Context, string) error) error {
		pullCtx, cancel, err := talos.WithClassDeadline(ctx, talos.MethodImagePull)
		if err != nil {
			return err
		}
		defer cancel()
		return into(pullCtx, ref)
	}
	for _, ref := range system {
		if err := pull(ref, cc.ImagePull); err != nil {
			return fmt.Errorf("%s: %w", ref, err)
		}
	}
	for _, ref := range cri {
		if err := pull(ref, cc.ImagePullCRI); err != nil {
			return fmt.Errorf("%s: %w", ref, err)
		}
	}
	return nil
}

// componentStep moves one control-plane component to the new version, one
// control-plane node at a time, each behind the health gate and each waited
// for until the node reports the pod back ready on the new image.
func componentStep(d Deps, cluster model.ClusterID, ids []model.MachineID, component string, to Version) jobs.Step {
	want := "registry.k8s.io/" + component + ":" + to.String()
	return jobs.Step{
		Name: "move " + component + " to " + to.String() + " on every control-plane node",
		Do: func(ctx context.Context, job *model.Job) error {
			cps, err := controlPlaneOf(ctx, d, cluster, ids)
			if err != nil {
				return err
			}
			for _, me := range cps {
				if at, err := staticPodImage(ctx, d, me.ID, component); err == nil && at.Image == want && at.Ready {
					continue // already there -- a resumed run, or a node done by hand
				}
				verdict, err := d.Gate.Evaluate(ctx, cluster, me.ID)
				if err != nil {
					return err
				}
				if !verdict.OK {
					return fmt.Errorf("upgrade: the health gate refused before %s: %s", nameOf(me), verdict.Reason)
				}
				if err := applyImage(ctx, d, me.ID, componentChange(component, to)); err != nil {
					return err
				}
				if err := waitForStaticPod(ctx, d, me, component, want); err != nil {
					return err
				}
			}
			job.Steps[job.Current].Detail = fmt.Sprintf("%s runs %s on %d control-plane node(s)",
				component, to, len(cps))
			return nil
		},
		Happened: func(ctx context.Context, _ *model.Job) (bool, error) {
			cps, err := controlPlaneOf(ctx, d, cluster, ids)
			if err != nil {
				return false, nil //nolint:nilerr // an inventory that cannot be read is not "done"
			}
			for _, me := range cps {
				at, err := staticPodImage(ctx, d, me.ID, component)
				if err != nil || at.Image != want || !at.Ready {
					return false, nil //nolint:nilerr // unreachable is not "done"
				}
			}
			return true, nil
		},
	}
}

// ComponentBudget is how long one control-plane node gets to bring a
// component back on its new image: the kubelet notices the changed manifest,
// stops the old pod, starts the new one, and the new one has to pass its
// readiness probe. An expectation rather than a hard deadline, like
// ReappearBudget.
const ComponentBudget = 5 * time.Minute

// pollInterval is how often a waiting step asks the node again. A variable so
// the package's tests can wait in milliseconds instead of seconds.
var pollInterval = 10 * time.Second

func staticPodImage(ctx context.Context, d Deps, id model.MachineID, component string) (talos.StaticPod, error) {
	cc, err := d.Connect(ctx, id)
	if err != nil {
		return talos.StaticPod{}, err
	}
	defer cc.Close() //nolint:errcheck // the verdict is the read's

	readCtx, cancel, err := talos.WithClassDeadline(ctx, talos.MethodCOSIList)
	if err != nil {
		return talos.StaticPod{}, err
	}
	defer cancel()
	pods, err := cc.StaticPods(readCtx)
	if err != nil {
		return talos.StaticPod{}, err
	}
	for _, p := range pods {
		if p.Component == component {
			return p, nil
		}
	}
	return talos.StaticPod{}, fmt.Errorf("upgrade: %s reports no %s static pod", id, component)
}

func waitForStaticPod(ctx context.Context, d Deps, me model.Machine, component, want string) error {
	started := time.Now()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
		at, err := staticPodImage(ctx, d, me.ID, component)
		if err == nil && at.Image == want && at.Ready {
			return nil
		}
		if time.Since(started) > ComponentBudget {
			return fmt.Errorf("upgrade: %s has not reported %s ready on %s after %s. The configuration "+
				"was applied; what has not happened is the kubelet bringing the pod back on the new image",
				nameOf(me), component, want, round(time.Since(started)))
		}
	}
}

// kubeProxyStep moves kube-proxy: its image in every control-plane node's
// configuration, which Talos renders the manifest from, and its image in the
// DaemonSet, which is what actually runs. A cluster without kube-proxy --
// a CNI that replaces it, or the proxy disabled in the configuration -- has
// nothing to move, and the step says so rather than failing.
func kubeProxyStep(d Deps, cluster model.ClusterID, ids []model.MachineID, to Version) jobs.Step {
	return jobs.Step{
		Name: "move kube-proxy to " + to.String(),
		Do: func(ctx context.Context, job *model.Job) error {
			cps, err := controlPlaneOf(ctx, d, cluster, ids)
			if err != nil {
				return err
			}
			for _, me := range cps {
				if err := applyImage(ctx, d, me.ID, proxyChange(to)); err != nil {
					return err
				}
			}
			if d.KubeProxy == nil {
				job.Steps[job.Current].Detail = "kube-proxy's image set in the configuration; " +
					"no Kubernetes access here to move the running DaemonSet"
				return nil
			}
			detail, err := d.KubeProxy(ctx, cluster, proxyImage(to))
			if errors.Is(err, ErrNoKubeProxy) {
				job.Steps[job.Current].Detail = "this cluster runs no kube-proxy; nothing to move"
				return nil
			}
			if err != nil {
				return err
			}
			job.Steps[job.Current].Detail = detail
			return nil
		},
	}
}

// ErrNoKubeProxy is a cluster without a kube-proxy DaemonSet, which is a
// cluster whose CNI replaces it rather than a fault.
var ErrNoKubeProxy = errors.New("upgrade: this cluster runs no kube-proxy")

// kubernetesNodeStep moves one node's kubelet, the last part of the run.
func kubernetesNodeStep(d Deps, cluster model.ClusterID, id model.MachineID, to Version) jobs.Step {
	return jobs.Step{
		Name: "move " + string(id) + "'s kubelet to Kubernetes " + to.String(),

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

			if err := applyImage(ctx, d, id, kubeletChange(to)); err != nil {
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
		case <-time.After(pollInterval):
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
