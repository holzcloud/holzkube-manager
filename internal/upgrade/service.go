package upgrade

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// Service is what the HTTP layer holds.
//
// The plan it builds is the screen: every node that will be walked, in the
// order they will be walked, each with the check that applies to it and the
// reason it would be skipped. A plan that only said "upgrade the cluster"
// would be a plan whose refusals all arrive one at a time, minutes apart, in
// the middle of a rolling operation.
type Service struct {
	deps Deps

	// releases lists the Talos versions this installation can offer. It is a
	// function because the list comes from the Image Factory and this package
	// has no business knowing that.
	releases func(ctx context.Context) ([]string, error)
}

// NewService wires one up.
func NewService(d Deps, releases func(ctx context.Context) ([]string, error)) *Service {
	return &Service{deps: d, releases: releases}
}

// NodePlan is one node's place in a rolling upgrade.
type NodePlan struct {
	Machine  model.MachineID `json:"machine"`
	Hostname string          `json:"hostname,omitempty"`
	Role     string          `json:"role"`

	// From is what the node is running now.
	From string `json:"from"`

	// Schematic is the Image Factory schematic read off the node (UPG-03), and
	// SchematicSentence says what it means for this node's extensions.
	Schematic         string `json:"schematic,omitempty"`
	SchematicSentence string `json:"schematic_sentence,omitempty"`

	// Installer is the exact reference this node's upgrade will install. It is
	// shown per node rather than once per run because two nodes in one cluster
	// can legitimately have different schematics.
	Installer string `json:"installer"`

	// Skipped and SkipReason are UPG-14: a locked node is walked past, not
	// failed, and the screen says so before the run rather than during it.
	Skipped    bool   `json:"skipped"`
	SkipReason string `json:"skip_reason,omitempty"`

	// KernelArgs is the comparison when it found a difference (UPG-04). Both
	// lists travel, because a diff an operator cannot see is a diff they have
	// to take on trust.
	KernelArgs *KernelArgDrift `json:"kernel_args,omitempty"`

	// Blocked and BlockReason are the per-node refusals that can be known
	// before the run: an unreadable schematic, kernel-argument drift.
	Blocked     bool   `json:"blocked"`
	BlockReason string `json:"block_reason,omitempty"`
}

// Plan is what the upgrade screen shows before anything is submitted.
type Plan struct {
	Cluster model.ClusterID `json:"cluster"`

	// To is the version this run installs.
	To string `json:"to"`

	// Chain is every version that has to be passed through to reach a target
	// further away than one minor (UPG-05). A run installs one of them; the
	// rest are the runs that follow.
	Chain []Step `json:"chain,omitempty"`

	// Strand is UPG-06, evaluated against the version this run installs.
	Strand StrandCheck `json:"strand"`

	// Gate is the health gate as it stands right now. It is shown before the
	// run and re-evaluated before every node -- what is here is a preview, and
	// the field name says which.
	Gate Verdict `json:"gate_preview"`

	// Nodes is the walk order.
	Nodes []NodePlan `json:"nodes"`

	// Blocked is whether this run can be submitted at all.
	Blocked bool `json:"blocked"`

	// BlockReason is the first thing standing in the way, which is the one an
	// operator acts on.
	BlockReason string `json:"block_reason,omitempty"`
}

// Releases lists the Talos versions this installation can upgrade to.
func (s *Service) Releases(ctx context.Context) ([]Version, error) {
	if s.releases == nil {
		return nil, errors.New("upgrade: this instance has no source of Talos releases")
	}
	raw, err := s.releases(ctx)
	if err != nil {
		return nil, err
	}
	return Releases(raw), nil
}

// PlanTalos builds the screen for a rolling Talos upgrade.
func (s *Service) PlanTalos(ctx context.Context, cluster model.ClusterID, to string) (Plan, error) {
	target, err := ParseVersion(to)
	if err != nil {
		return Plan{}, err
	}

	machines, err := s.deps.Machines(ctx, cluster)
	if err != nil {
		return Plan{}, err
	}
	if len(machines) == 0 {
		return Plan{}, fmt.Errorf("upgrade: cluster %s has no machines in the inventory", cluster)
	}

	plan := Plan{Cluster: cluster, To: target.String()}

	// The chain first, because a target two minors away is not one run and the
	// operator has to see that before anything else on this screen means what
	// it appears to mean.
	if lowest, ok := lowestVersion(machines); ok {
		available, rerr := s.Releases(ctx)
		if rerr == nil {
			if chain, cerr := Chain(lowest, target, available); cerr == nil {
				plan.Chain = chain
			} else {
				plan.Blocked = true
				plan.BlockReason = cerr.Error()
			}
		}
	}

	// UPG-06, against the version this run installs.
	plan.Strand = CheckStrand(target, clusterKubernetesVersion(machines))
	if plan.Strand.Blocked && !plan.Blocked {
		plan.Blocked = true
		plan.BlockReason = plan.Strand.Sentence + " " + plan.Strand.Remedy
	}

	// Control-plane nodes first. A worker upgraded against an older control
	// plane is the version skew Kubernetes forbids in the other direction.
	ordered := walkOrder(machines)

	for _, m := range ordered {
		node := NodePlan{
			Machine:  m.ID,
			Hostname: m.Hostname,
			Role:     string(m.Role),
			From:     m.Snapshot.TalosVersion,
		}

		if m.Locked {
			node.Skipped = true
			node.SkipReason = SkipReason(m)
			plan.Nodes = append(plan.Nodes, node)
			continue
		}

		// The schematic is read from the node, now, and never taken from the
		// stored snapshot: a snapshot is what the node said last time, and the
		// failure this check prevents is permanent and silent.
		cc, cerr := s.deps.Connect(ctx, m.ID)
		if cerr != nil {
			node.Blocked = true
			node.BlockReason = fmt.Sprintf("%s did not answer, so its Image Factory schematic "+
				"could not be read. Upgrading it without knowing the schematic would install a "+
				"stock system and silently remove every system extension it has.", nameOf(m))
			plan.Nodes = append(plan.Nodes, node)
			continue
		}

		verdict, serr := CheckSchematic(ctx, cc)
		if serr != nil {
			_ = cc.Close()
			node.Blocked = true
			node.BlockReason = serr.Error()
			plan.Nodes = append(plan.Nodes, node)
			continue
		}
		cc2 := cc

		node.Schematic = verdict.ID
		node.SchematicSentence = verdict.Sentence
		node.Installer = InstallerFor(verdict.ID, target)

		// UPG-04, on the same connection. The upgrade RPC carries an installer
		// image and nothing else -- kernel arguments are written at install
		// time from the machine configuration -- so a node whose bootloader
		// has arguments its configuration does not is a node where this path
		// silently discards them.
		drift, derr := ReadKernelArgDrift(ctx, cc2)
		_ = cc2.Close()
		if derr == nil && drift.Drifted {
			node.Blocked = true
			node.BlockReason = drift.Sentence
			node.KernelArgs = &drift
		}

		plan.Nodes = append(plan.Nodes, node)
	}

	for _, n := range plan.Nodes {
		if n.Blocked && !plan.Blocked {
			plan.Blocked = true
			plan.BlockReason = n.BlockReason
		}
	}

	// The gate as a preview. It is not the gate that decides -- that one runs
	// inside each node's step, against the cluster as it is then -- and the
	// field name says so, because a screen showing a stale pass as though it
	// were the decision is worse than showing nothing.
	plan.Gate = s.gatePreview(ctx, cluster)
	if !plan.Gate.OK && !plan.Blocked {
		plan.Blocked = true
		plan.BlockReason = plan.Gate.Reason
	}

	return plan, nil
}

// gatePreview evaluates the health gate and never returns an empty verdict.
//
// A gate that could not be evaluated is a refusal, not a blank. The first
// version of this dropped the error and left the zero Verdict, which renders
// as a gate that neither passed nor refused -- and a screen showing that would
// be a screen where the most important check on it silently said nothing.
func (s *Service) gatePreview(ctx context.Context, cluster model.ClusterID) Verdict {
	gate, err := s.deps.Gate.Evaluate(ctx, cluster, "")
	if err == nil {
		return gate
	}
	return Verdict{
		Reason: "The health gate could not be evaluated: " + err.Error() +
			". Nothing is upgraded while this is the answer -- a gate that cannot read a " +
			"cluster's etcd cannot say whether the cluster survives losing a node.",
	}
}

// PlanKubernetes builds the screen for a rolling Kubernetes upgrade.
//
// It is a shorter screen than the Talos one, because two of that screen's
// checks do not apply: there is no installer image, so there is no schematic
// to lose, and no new system is written, so there is no reboot. What stays is
// the chain, the compatibility gate against the *running Talos*, and the
// health gate.
func (s *Service) PlanKubernetes(ctx context.Context, cluster model.ClusterID, to string) (Plan, error) {
	target, err := ParseVersion(to)
	if err != nil {
		return Plan{}, err
	}

	machines, err := s.deps.Machines(ctx, cluster)
	if err != nil {
		return Plan{}, err
	}
	if len(machines) == 0 {
		return Plan{}, fmt.Errorf("upgrade: cluster %s has no machines in the inventory", cluster)
	}

	plan := Plan{Cluster: cluster, To: target.String()}

	from, ok := lowestKubernetes(machines)
	if !ok {
		plan.Blocked = true
		plan.BlockReason = "No node in this cluster has reported a Kubernetes version, so there " +
			"is nothing to upgrade from. Refresh the cluster first."
		return plan, nil
	}

	talosVersion := lowestTalos(machines)
	chain, cerr := KubernetesChain(from, target, talosVersion)
	if cerr != nil {
		plan.Blocked = true
		plan.BlockReason = cerr.Error()
		return plan, nil
	}
	plan.Chain = chain

	// The same gate as the Talos path, read the same way round: this asks
	// whether the *running Talos* supports the Kubernetes version being
	// installed.
	plan.Strand = CheckStrand(mustParse(talosVersion), target.String())
	if plan.Strand.Blocked {
		plan.Blocked = true
		plan.BlockReason = plan.Strand.Sentence + " " + plan.Strand.Remedy
	}

	for _, m := range walkOrder(machines) {
		node := NodePlan{
			Machine:  m.ID,
			Hostname: m.Hostname,
			Role:     string(m.Role),
			From:     m.Snapshot.KubernetesVersion,
		}
		if m.Locked {
			node.Skipped = true
			node.SkipReason = SkipReason(m)
		}
		plan.Nodes = append(plan.Nodes, node)
	}

	plan.Gate = s.gatePreview(ctx, cluster)
	if !plan.Gate.OK && !plan.Blocked {
		plan.Blocked = true
		plan.BlockReason = plan.Gate.Reason
	}

	return plan, nil
}

// RequestFor turns a plan into the job request that runs it.
//
// The node list comes from the plan rather than being derived again at submit
// time, so that what is submitted is what was shown: a cluster that gained a
// node between the screen and the button must not have it silently included.
func RequestFor(plan Plan) Request {
	req := Request{To: plan.To, Schematics: map[model.MachineID]string{}}
	for _, n := range plan.Nodes {
		if n.Skipped {
			// A locked node is left out of the request entirely rather than
			// carried and skipped at run time. Both work; this one means the
			// job's own step list is the list of things it will do, which is
			// what somebody watching it reads.
			continue
		}
		req.Machines = append(req.Machines, n.Machine)
		if n.Schematic != "" {
			req.Schematics[n.Machine] = n.Schematic
		}
	}
	return req
}

// EtcdMembers reads a cluster's membership through one of its control-plane
// nodes.
func (s *Service) EtcdMembers(ctx context.Context, cluster model.ClusterID) (MemberList, error) {
	machines, err := s.deps.Machines(ctx, cluster)
	if err != nil {
		return MemberList{}, err
	}

	cc, m, err := s.anyControlPlane(ctx, machines)
	if err != nil {
		return MemberList{}, err
	}
	defer cc.Close() //nolint:errcheck // the list is the verdict
	_ = m

	return Members(ctx, cc, machines)
}

// RemoveEtcdMember removes one member (UPG-11).
//
// The call is made through a *different* node than the member being removed,
// because a member cannot remove itself. Choosing that node here rather than
// asking the client to is not a convenience: a client that picked the wrong
// one would get the node's own refusal, which is about the API rather than
// about what to do instead.
func (s *Service) RemoveEtcdMember(ctx context.Context, cluster model.ClusterID, id string) error {
	machines, err := s.deps.Machines(ctx, cluster)
	if err != nil {
		return err
	}

	list, err := s.EtcdMembers(ctx, cluster)
	if err != nil {
		return err
	}
	member, ok := findMember(list, id)
	if !ok {
		return fmt.Errorf("upgrade: etcd has no member %s", id)
	}

	through := make([]model.Machine, 0, len(machines))
	for _, m := range machines {
		if m.Role == model.RoleControlPlane && m.ID != member.Machine {
			through = append(through, m)
		}
	}
	if len(through) == 0 {
		return fmt.Errorf("upgrade: %s is the only control-plane node this installation can "+
			"reach, and a member cannot remove itself. Remove it from another control-plane node, "+
			"or have the node leave the cluster itself", member.Name)
	}

	cc, _, err := s.anyControlPlane(ctx, through)
	if err != nil {
		return err
	}
	defer cc.Close() //nolint:errcheck // the removal's verdict is its own

	return RemoveMember(ctx, cc, list, id)
}

// Snapshot streams an etcd snapshot (UPG-12).
func (s *Service) Snapshot(ctx context.Context, cluster model.ClusterID, w io.Writer) (int64, error) {
	machines, err := s.deps.Machines(ctx, cluster)
	if err != nil {
		return 0, err
	}

	cc, _, err := s.anyControlPlane(ctx, machines)
	if err != nil {
		return 0, err
	}
	defer cc.Close() //nolint:errcheck // the byte count is the verdict

	return Snapshot(ctx, cc, w)
}

// RemoveNodeFromCluster is UPG-13.
func (s *Service) RemoveNodeFromCluster(ctx context.Context, id model.MachineID, controlPlane bool) error {
	cc, err := s.deps.Connect(ctx, id)
	if err != nil {
		return err
	}
	defer cc.Close() //nolint:errcheck // the removal's verdict is its own

	return RemoveNode(ctx, cc, controlPlane)
}

// anyControlPlane opens a client to the first control-plane node that answers.
//
// First that *answers*, not first in the list: the caller is asking a question
// about the cluster, and a cluster whose first control-plane node happens to
// be down can still answer it.
func (s *Service) anyControlPlane(ctx context.Context, machines []model.Machine) (
	*talos.ClusterClient, model.Machine, error,
) {
	var lastErr error
	for _, m := range machines {
		if m.Role != model.RoleControlPlane {
			continue
		}
		cc, err := s.deps.Connect(ctx, m.ID)
		if err != nil {
			lastErr = err
			continue
		}
		return cc, m, nil
	}
	if lastErr != nil {
		return nil, model.Machine{}, fmt.Errorf("upgrade: no control-plane node answered: %w", lastErr)
	}
	return nil, model.Machine{}, errors.New("upgrade: this cluster has no control-plane node in the inventory")
}

// walkOrder puts control-plane nodes first, then workers, each group by
// hostname so that two runs of the same plan walk the same order.
func walkOrder(machines []model.Machine) []model.Machine {
	out := append([]model.Machine(nil), machines...)
	sort.SliceStable(out, func(i, j int) bool {
		ci := out[i].Role == model.RoleControlPlane
		cj := out[j].Role == model.RoleControlPlane
		if ci != cj {
			return ci
		}
		return nameOf(out[i]) < nameOf(out[j])
	})
	return out
}

// lowestVersion is the oldest Talos any node in the cluster is running, which
// is where a chain has to start: the chain is the same for every node, and
// computing it from the newest would skip a minor for the oldest.
func lowestVersion(machines []model.Machine) (Version, bool) {
	var lowest Version
	found := false
	for _, m := range machines {
		v, err := ParseVersion(m.Snapshot.TalosVersion)
		if err != nil {
			continue
		}
		if !found || v.Less(lowest) {
			lowest, found = v, true
		}
	}
	return lowest, found
}

func lowestKubernetes(machines []model.Machine) (Version, bool) {
	var lowest Version
	found := false
	for _, m := range machines {
		v, err := ParseVersion(m.Snapshot.KubernetesVersion)
		if err != nil {
			continue
		}
		if !found || v.Less(lowest) {
			lowest, found = v, true
		}
	}
	return lowest, found
}

func lowestTalos(machines []model.Machine) string {
	v, ok := lowestVersion(machines)
	if !ok {
		return ""
	}
	return v.String()
}

// clusterKubernetesVersion is the version the compatibility gate is evaluated
// against: the lowest any node reports, because the gate's question is whether
// *the cluster* stays supported and the oldest node is the one that decides.
func clusterKubernetesVersion(machines []model.Machine) string {
	v, ok := lowestKubernetes(machines)
	if !ok {
		return ""
	}
	return v.String()
}

func mustParse(s string) Version {
	v, err := ParseVersion(s)
	if err != nil {
		// A version that does not parse produces the zero Version, which has
		// no compatibility row and is therefore blocked by CheckStrand. That
		// is the honest outcome and it is why this does not panic.
		return Version{}
	}
	return v
}
