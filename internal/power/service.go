package power

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/codes"

	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/upgrade"
)

// ErrNotFound reports a machine or cluster this installation has no record of.
var ErrNotFound = errors.New("power: no such machine or cluster")

// UnavailableError is an action that may not be pressed right now, carrying
// the same sentence the GET showed for it.
type UnavailableError struct {
	Action Action
	Reason string
}

func (e *UnavailableError) Error() string {
	return "power: " + string(e.Action) + " is not available: " + e.Reason
}

// Deps is what the power model needs. Every field but the timings is required.
type Deps struct {
	Store  store.Store
	Logger *slog.Logger
	Jobs   *jobs.Engine

	// Connect opens a Talos client to one machine: the inventory's Connect.
	Connect jobs.Connector

	// Kube reaches one cluster's Kubernetes API: the inventory's KubeClient. A
	// function rather than a client, because a job outlives the request that
	// submitted it and a minted certificate lasts an hour.
	Kube func(ctx context.Context, id model.ClusterID) (*kube.Client, error)

	// Machines lists a cluster's machines as the inventory shows them --
	// departed ones left out -- so that this and every screen agree on what
	// "the cluster" is.
	Machines func(ctx context.Context, id model.ClusterID) ([]model.Machine, error)

	// Gate is the etcd health gate the rolling upgrade uses. A stop of a
	// control-plane node asks it the question it was built for: does the
	// cluster survive losing that node.
	Gate *upgrade.Gate

	// Waker sends Wake-on-LAN.
	Waker Waker

	// PollInterval is how often a wait looks again, BootBudget how long a node
	// gets to come up, KubeBudget how long the Kubernetes API gets to answer
	// after one has. Zero takes the defaults.
	PollInterval time.Duration
	BootBudget   time.Duration
	KubeBudget   time.Duration
}

// The defaults for the waits.
const (
	// DefaultPollInterval: often enough that a node that is back is noticed
	// within seconds, rarely enough that a node that is not is not hammered --
	// the Talos circuit breaker will throttle it harder than this anyway.
	DefaultPollInterval = 5 * time.Second

	// DefaultBootBudget: a mini-PC's firmware, a Talos boot and the kubelet
	// registering, with room for the slow one. A node that has not answered
	// by then is one somebody should look at, and the job says so.
	DefaultBootBudget = 15 * time.Minute

	// DefaultKubeBudget: how long a Kubernetes API that is coming back gets to
	// start answering before an uncordon gives up on it.
	DefaultKubeBudget = 10 * time.Minute

	// probeBudget bounds one "is it answering" check: the connection's own
	// probe and the Version call behind it.
	probeBudget = 8 * time.Second
)

// Service is the power model.
type Service struct {
	deps Deps

	mu sync.Mutex
	// seenUp remembers which disabled machines the keeper has already seen
	// answering, so it says so once when one comes up rather than every sweep.
	seenUp map[model.MachineID]bool
}

// New builds the service.
func New(d Deps) *Service {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	if d.PollInterval <= 0 {
		d.PollInterval = DefaultPollInterval
	}
	if d.BootBudget <= 0 {
		d.BootBudget = DefaultBootBudget
	}
	if d.KubeBudget <= 0 {
		d.KubeBudget = DefaultKubeBudget
	}
	return &Service{deps: d, seenUp: map[model.MachineID]bool{}}
}

// NodeJobKind is the job kind a node action submits. It reads as the action's
// own name on the Jobs screen: "node.stop", "node.force-restart".
func NodeJobKind(a Action) model.JobKind { return model.JobKind("node." + string(a)) }

// ClusterJobKind is the job kind a cluster action submits: "cluster.start".
func ClusterJobKind(a Action) model.JobKind { return model.JobKind("cluster." + string(a)) }

// MachineReport answers "what can I do to this node, and why not".
func (s *Service) MachineReport(ctx context.Context, id model.MachineID) (Report, error) {
	_, facts, err := s.machineFacts(ctx, id)
	if err != nil {
		return Report{}, err
	}
	return nodeReport(facts), nil
}

// ClusterReport answers it for a whole cluster.
func (s *Service) ClusterReport(ctx context.Context, id model.ClusterID) (Report, error) {
	_, _, facts, err := s.clusterFacts(ctx, id)
	if err != nil {
		return Report{}, err
	}
	return clusterReport(facts), nil
}

// SubmitMachine checks one node action against the same rules the report
// shows and submits it as a job.
func (s *Service) SubmitMachine(ctx context.Context, id model.MachineID, a Action, actor string) (model.Job, error) {
	rec, facts, err := s.machineFacts(ctx, id)
	if err != nil {
		return model.Job{}, err
	}
	if ok, why := nodeReport(facts).Available(a); !ok {
		return model.Job{}, &UnavailableError{Action: a, Reason: why}
	}

	params, err := plan{Members: []member{memberOf(rec, facts.Answer == answerYes)}}.params()
	if err != nil {
		return model.Job{}, err
	}
	return s.deps.Jobs.Submit(ctx, model.Job{
		Kind:    NodeJobKind(a),
		Cluster: rec.Cluster,
		Machine: rec.ID,
		Params:  params,
		Actor:   actor,
	})
}

// SubmitCluster checks one cluster action and submits it as a job.
//
// The order the nodes are walked in is decided here and stored with the job,
// the way a rolling upgrade stores its node list: a resumed job walks the nodes
// it was submitted for, and a cluster that gained a node in between does not
// have it silently included.
func (s *Service) SubmitCluster(ctx context.Context, id model.ClusterID, a Action, actor string) (model.Job, error) {
	cluster, recs, facts, err := s.clusterFacts(ctx, id)
	if err != nil {
		return model.Job{}, err
	}
	if ok, why := clusterReport(facts).Available(a); !ok {
		return model.Job{}, &UnavailableError{Action: a, Reason: why}
	}

	members := make([]member, 0, len(recs))
	for i, rec := range recs {
		answered := facts.Members[i].Answer == answerYes
		// A restart walks only what is meant to be running. A disabled node is
		// off on purpose, and rebooting one somebody powered on anyway would
		// be the opposite of what the mark asks for.
		if (a == Restart || a == ForceRestart) && (rec.Disabled || !answered) {
			continue
		}
		members = append(members, memberOf(rec, answered))
	}
	params, err := plan{Members: walkOrder(members, cluster.Endpoint)}.params()
	if err != nil {
		return model.Job{}, err
	}
	return s.deps.Jobs.Submit(ctx, model.Job{
		Kind:    ClusterJobKind(a),
		Cluster: id,
		Params:  params,
		Actor:   actor,
	})
}

// machineFacts reads one node.
func (s *Service) machineFacts(ctx context.Context, id model.MachineID) (model.Machine, nodeFacts, error) {
	rec, err := s.deps.Store.Machines().Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return model.Machine{}, nodeFacts{}, fmt.Errorf("%w: machine %s", ErrNotFound, id)
		}
		return model.Machine{}, nodeFacts{}, err
	}

	facts := nodeFacts{
		Disabled:      rec.Disabled,
		PowerCordoned: rec.PowerCordoned,
		HasMACs:       len(macsOf(rec)) > 0,
	}

	if rec.Cluster == "" {
		facts.Answer = answerUnknown
		facts.Unknown = "It belongs to no cluster, so there are no credentials to reach it with."
		return rec, facts, nil
	}

	cluster, err := s.deps.Store.Clusters().Get(ctx, rec.Cluster)
	switch {
	case err == nil:
		facts.ClusterDisabled = cluster.Disabled
		facts.ClusterLocked = cluster.Locked
	case errors.Is(err, store.ErrNotFound):
		facts.Answer = answerUnknown
		facts.Unknown = "Its cluster is no longer in the inventory, so there are no credentials to reach it with."
		return rec, facts, nil
	default:
		return model.Machine{}, nodeFacts{}, err
	}

	facts.Answer, facts.Unknown = s.probe(ctx, rec.ID)

	// The gate only for the case it decides: a control-plane node that is up
	// and could be taken down. Asking it for a worker, or for a node that is
	// already off, would dial every control-plane node for an answer nobody
	// reads.
	if rec.Role == model.RoleControlPlane && facts.Answer == answerYes && !facts.ClusterLocked {
		facts.GateRefusal = s.gateRefusal(ctx, rec.Cluster, rec.ID)
	}
	return rec, facts, nil
}

// gateRefusal asks the etcd gate whether the cluster can spare one node, and
// turns its verdict into the one sentence a screen shows.
//
// The gate's own reason names the number that decided it and is kept for the
// job, where there is room for it. Here there is one sentence, and it is
// whichever of the two conditions the operator can do something different
// about: quorum -- this node is one of too few -- or health -- etcd is not
// settled enough to lose anybody right now.
func (s *Service) gateRefusal(ctx context.Context, cluster model.ClusterID, id model.MachineID) string {
	if s.deps.Gate == nil {
		return ReasonEtcdUnhealthy
	}
	v, err := s.deps.Gate.Evaluate(ctx, cluster, id)
	if err != nil {
		// A gate that could not be asked has not said yes.
		return ReasonEtcdUnhealthy
	}
	if v.OK {
		return ""
	}
	if len(v.Input.Unreachable) > 0 || v.Input.Voting < upgrade.MinVotingMembers {
		return ReasonQuorum
	}
	return ReasonEtcdUnhealthy
}

// clusterFacts reads a cluster and every one of its machines, probing them in
// parallel: a cluster of six nodes with two off must not take two probe budgets
// in series to say so.
func (s *Service) clusterFacts(ctx context.Context, id model.ClusterID) (model.Cluster, []model.Machine, clusterFacts, error) {
	cluster, err := s.deps.Store.Clusters().Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return model.Cluster{}, nil, clusterFacts{}, fmt.Errorf("%w: cluster %s", ErrNotFound, id)
		}
		return model.Cluster{}, nil, clusterFacts{}, err
	}
	recs, err := s.deps.Machines(ctx, id)
	if err != nil {
		return model.Cluster{}, nil, clusterFacts{}, err
	}
	sort.Slice(recs, func(i, j int) bool { return nameOf(recs[i]) < nameOf(recs[j]) })

	facts := clusterFacts{
		Disabled: cluster.Disabled,
		Locked:   cluster.Locked,
		Members:  make([]memberFacts, len(recs)),
	}
	var wg sync.WaitGroup
	for i, rec := range recs {
		facts.Members[i] = memberFacts{Disabled: rec.Disabled, PowerCordoned: rec.PowerCordoned}
		wg.Add(1)
		go func() {
			defer wg.Done()
			facts.Members[i].Answer, _ = s.probe(ctx, rec.ID)
		}()
	}
	wg.Wait()
	return cluster, recs, facts, nil
}

// probe asks whether a node answers right now.
//
// Only the answers a node can give count as "it is off". A node that does not
// answer, times out, or has had its circuit opened by three of those in a row
// is off as far as anything here can tell. A node that answered and refused
// this installation's certificate, or runs a Talos this build refuses, is on --
// and saying "off" would offer to wake a machine that is running.
func (s *Service) probe(ctx context.Context, id model.MachineID) (answer, string) {
	ctx, cancel := context.WithTimeout(ctx, probeBudget)
	defer cancel()

	cc, err := s.deps.Connect(ctx, id)
	if err == nil {
		_, err = cc.Probe(ctx)
		_ = cc.Close()
	}
	if err == nil {
		return answerYes, ""
	}

	var te *talos.Error
	switch {
	case errors.Is(err, talos.ErrUnsupportedVersion), errors.Is(err, talos.ErrPreRelease):
		return answerUnknown, "It answers, but runs a Talos version this build does not manage."
	case errors.Is(err, talos.ErrFingerprintPin):
		return answerUnknown, "It answers with a certificate the operator did not confirm, so it is not asked anything."
	case errors.As(err, &te) && te.Kind == talos.KindRejected &&
		(te.Status == codes.Unauthenticated || te.Status == codes.PermissionDenied):
		return answerUnknown, "It answers, but refuses this installation's certificate."
	}
	return answerNo, ""
}

// member is one node as a job walks it. Hostname and address are carried so
// that the job can name the node on the Jobs screen and find its Kubernetes
// node without asking the store for either.
type member struct {
	Machine  model.MachineID   `json:"machine"`
	Hostname string            `json:"hostname"`
	Addr     string            `json:"addr,omitempty"`
	Role     model.MachineRole `json:"role"`

	// Answered is whether it answered when the job was submitted. It decides
	// the shape of a disable -- a node that is already off is disabled by the
	// mark alone -- and is stored so that a resumed job has the same shape.
	Answered bool `json:"answered"`
}

func memberOf(rec model.Machine, answered bool) member {
	return member{
		Machine: rec.ID, Hostname: nameOf(rec), Addr: rec.Addr, Role: rec.Role, Answered: answered,
	}
}

func (m member) controlPlane() bool { return m.Role == model.RoleControlPlane }

// plan is a job's stored walk.
type plan struct {
	Members []member `json:"members"`
}

func (p plan) params() (map[string]string, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("power: recording the plan: %w", err)
	}
	return map[string]string{"plan": string(raw)}, nil
}

func planFrom(params map[string]string) (plan, error) {
	raw, ok := params["plan"]
	if !ok {
		return plan{}, errors.New("power: this job carries no plan")
	}
	var p plan
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return plan{}, fmt.Errorf("power: this job's plan cannot be read: %w", err)
	}
	return p, nil
}

// walkOrder is the order a cluster is taken down in, and a rolling restart
// walks it: workers first, then the control-plane nodes, and among those the
// one this daemon reaches the cluster through last.
//
// Workers first, because they hold the workloads and the control plane is what
// moves them: draining a worker needs an API server, and a cluster whose
// control plane went first has nothing left to drain anything with. The
// endpoint node last, because it is the one the daemon talks to: taking it down
// before the others would leave the rest of the walk without a way in. Within
// each group by name, so two runs over the same cluster walk the same way.
func walkOrder(members []member, endpoint string) []member {
	var workers, planes []member
	for _, m := range members {
		if m.controlPlane() {
			planes = append(planes, m)
		} else {
			workers = append(workers, m)
		}
	}
	byName := func(ms []member) {
		sort.SliceStable(ms, func(i, j int) bool { return ms[i].Hostname < ms[j].Hostname })
	}
	byName(workers)
	byName(planes)

	isEndpoint := func(m member) bool {
		return endpoint != "" && (m.Addr == endpoint || strings.EqualFold(m.Hostname, endpoint))
	}
	sort.SliceStable(planes, func(i, j int) bool { return !isEndpoint(planes[i]) && isEndpoint(planes[j]) })

	return append(workers, planes...)
}

func nameOf(m model.Machine) string {
	if m.Hostname != "" {
		return m.Hostname
	}
	return string(m.ID)
}

func macsOf(rec model.Machine) []string {
	ifaces := make([]InterfaceAddr, 0, len(rec.Snapshot.Interfaces))
	for _, i := range rec.Snapshot.Interfaces {
		ifaces = append(ifaces, InterfaceAddr{HardwareAddr: i.HardwareAddr, Kind: i.Kind})
	}
	macs := MACsOf(ifaces)
	out := make([]string, 0, len(macs))
	for _, m := range macs {
		out = append(out, m.String())
	}
	return out
}

// updateMachine applies a change to a machine record, re-reading on a lost
// revision race. The inventory's observer writes the same record every
// heartbeat, so a conflict here is the ordinary case rather than a surprise;
// the retry re-reads so that the observation it lost to is kept.
func (s *Service) updateMachine(ctx context.Context, id model.MachineID, change func(*model.Machine)) error {
	for range 5 {
		rec, err := s.deps.Store.Machines().Get(ctx, id)
		if err != nil {
			return err
		}
		change(&rec)
		_, err = s.deps.Store.Machines().Put(ctx, rec)
		if errors.Is(err, store.ErrConflict) {
			continue
		}
		return err
	}
	return fmt.Errorf("%w: the record for machine %s kept changing underneath", store.ErrConflict, id)
}

func (s *Service) updateCluster(ctx context.Context, id model.ClusterID, change func(*model.Cluster)) error {
	for range 5 {
		rec, err := s.deps.Store.Clusters().Get(ctx, id)
		if err != nil {
			return err
		}
		change(&rec)
		_, err = s.deps.Store.Clusters().Put(ctx, rec)
		if errors.Is(err, store.ErrConflict) {
			continue
		}
		return err
	}
	return fmt.Errorf("%w: the record for cluster %s kept changing underneath", store.ErrConflict, id)
}
