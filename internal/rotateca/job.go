package rotateca

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/machineconfig"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// JobKindRotateCA is the rotation as a job.
//
// A job and not a request, for the reason the engine exists: this is four
// passes over every node, and a process that dies in the middle has to be able
// to say which pass it was on. A rotation that forgot that would leave a
// cluster trusting two authorities with nobody able to say whether that was
// step two or step four.
const JobKindRotateCA model.JobKind = "cluster.rotate-ca"

// Deps is what the rotation needs from the composition root.
type Deps struct {
	Store  store.Store
	Logger *slog.Logger

	// Connect reaches one node with the credentials currently in the store.
	// It is the inventory's connector, so the rotation dials what the rest of
	// the product dials -- including, after pass 3, the new certificate.
	Connect func(ctx context.Context, id model.MachineID) (*talos.ClusterClient, error)

	// Machines is the cluster's inventory.
	Machines func(ctx context.Context, cluster model.ClusterID) ([]model.Machine, error)

	// AdoptAuthority replaces the cluster's stored authority with the new one
	// and mints this installation a certificate from it, proving it against a
	// node before keeping it. It is the inventory's, because that is where the
	// minting and the proof already live (V2-OPS-02's first half).
	AdoptAuthority func(ctx context.Context, cluster model.ClusterID, a Authority) error

	Now func() time.Time
}

// Request is a rotation as the operator asked for it.
type Request struct {
	// Machines is every node the rotation must reach, by UUID, as the
	// inventory listed them when the plan was made.
	//
	// Stored on the job rather than re-listed at each step, and that is the
	// difference between a rotation and a surprise: a node that joins
	// mid-rotation is a node this run never planned for, and a node that
	// disappears from the inventory mid-rotation must still stop the run,
	// because it is the one that will refuse the new certificate.
	Machines []model.MachineID `json:"machines"`
}

// RequestFromParams reads a stored job's parameters.
func RequestFromParams(p map[string]string) (Request, error) {
	var r Request
	raw, ok := p["request"]
	if !ok {
		return Request{}, errors.New("rotateca: this job carries no request")
	}
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return Request{}, fmt.Errorf("rotateca: this job's request could not be read: %w", err)
	}
	if len(r.Machines) == 0 {
		return Request{}, errors.New("rotateca: this rotation names no nodes")
	}
	return r, nil
}

// Params renders a request for storage.
func (r Request) Params() (map[string]string, error) {
	raw, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	return map[string]string{"request": string(raw)}, nil
}

// Register wires the rotation into the engine.
func Register(e *jobs.Engine, d Deps) {
	e.Register(JobKindRotateCA, func(j model.Job) ([]jobs.Step, error) {
		req, err := RequestFromParams(j.Params)
		if err != nil {
			return nil, err
		}
		return steps(d, req, j.Cluster), nil
	})
}

// steps is the rotation, in the only order that keeps a cluster reachable.
//
// Every node completes a pass before any node starts the next one. Node by
// node through all four passes would put a cluster into a state where one node
// issues from the new authority and its neighbours do not accept it -- which
// is what etcd peers and trustd talk over.
func steps(d Deps, req Request, cluster model.ClusterID) []jobs.Step {
	out := []jobs.Step{prepareStep(d, cluster)}

	for _, id := range req.Machines {
		out = append(out, acceptStep(d, cluster, id))
	}
	out = append(out, preflightIssue(d, cluster, req))
	for _, id := range req.Machines {
		out = append(out, issueStep(d, cluster, id))
	}
	out = append(out, adoptStep(d, cluster))
	for _, id := range req.Machines {
		out = append(out, pruneStep(d, cluster, id))
	}
	return append(out, finishStep(d, cluster))
}

// prepareStep generates the authority the cluster is moving to and stores it
// beside the one it is moving from.
//
// Stored, and that is what makes every later step resumable: the passes are
// written in terms of "the new authority", and a process that died between two
// of them would otherwise generate a second new authority and leave the
// cluster trusting three.
func prepareStep(d Deps, cluster model.ClusterID) jobs.Step {
	return jobs.Step{
		Name: "prepare a new certificate authority",
		Do: func(ctx context.Context, _ *model.Job) error {
			sec, err := d.Store.ClusterSecrets().Get(ctx, cluster)
			if err != nil {
				return err
			}
			if len(sec.NextOSCACrt) > 0 && len(sec.NextOSCAKey) > 0 {
				return nil
			}
			a, err := NewAuthority(d.Now())
			if err != nil {
				return err
			}
			sec.NextOSCACrt, sec.NextOSCAKey = a.Crt, a.Key
			_, err = d.Store.ClusterSecrets().Put(ctx, sec)
			return err
		},
		Happened: func(ctx context.Context, _ *model.Job) (bool, error) {
			sec, err := d.Store.ClusterSecrets().Get(ctx, cluster)
			if err != nil {
				return false, err
			}
			return len(sec.NextOSCACrt) > 0 && len(sec.NextOSCAKey) > 0, nil
		},
	}
}

// acceptStep is pass 1 for one node.
func acceptStep(d Deps, cluster model.ClusterID, id model.MachineID) jobs.Step {
	return jobs.Step{
		Name: "let " + string(id) + " accept the new authority",
		Do: func(ctx context.Context, job *model.Job) error {
			next, err := d.pending(ctx, cluster)
			if err != nil {
				return err
			}
			return d.rewrite(ctx, id, job, func(state State, cfg []byte, _ model.MachineRole) ([]byte, bool, error) {
				patch, changes := AcceptPatch(state, next.Crt)
				if !changes {
					return nil, false, nil
				}
				out, err := machineconfig.ApplyPatches(cfg, []string{patch})
				return out, true, err
			})
		},
		Happened: func(ctx context.Context, _ *model.Job) (bool, error) {
			next, err := d.pending(ctx, cluster)
			if err != nil {
				return false, err
			}
			state, err := d.read(ctx, id)
			if err != nil {
				return false, err
			}
			return state.Accepts(next.Crt), nil
		},
	}
}

// preflightIssue is the refusal, placed where it decides something.
//
// After pass 1 and before pass 2, because that is the last moment at which
// stopping costs nothing: a cluster that accepts two authorities and issues
// from the old one is a working cluster. One step later, a node that cannot be
// reached is a node that will refuse the certificate every other node accepts.
//
// It is a step rather than a check at submission time for the same reason: the
// question is not "was every node up when the operator clicked" but "is every
// node up now, with pass 2 about to start".
func preflightIssue(d Deps, cluster model.ClusterID, req Request) jobs.Step {
	return jobs.Step{
		Name: "check that every node answered before switching authority",
		Do: func(ctx context.Context, job *model.Job) error {
			next, err := d.pending(ctx, cluster)
			if err != nil {
				return err
			}

			reached := make(map[model.MachineID]error, len(req.Machines))
			for _, id := range req.Machines {
				state, err := d.read(ctx, id)
				switch {
				case err != nil:
					reached[id] = err
				case !state.Accepts(next.Crt):
					// Reached and not ready is its own finding: the node
					// answered, so this is not a network fault, and pass 2
					// would cut it off just the same.
					reached[id] = errors.New("it does not accept the new authority yet")
				default:
					reached[id] = nil
				}
			}
			if err := RefuseUnlessEveryNodeAnswered(reached); err != nil {
				return err
			}
			job.Steps[job.Current].Detail = fmt.Sprintf("%d nodes accept both authorities", len(req.Machines))
			return nil
		},
		// Deliberately unverifiable: this step asserts a fact about a moment,
		// and a resumed run has to establish it again rather than inherit it.
		// A nil Happened parks the job, which is the honest outcome -- the
		// operator decides whether the fleet is in the state they left it in.
		Happened: nil,
	}
}

// issueStep is pass 2 for one node.
func issueStep(d Deps, cluster model.ClusterID, id model.MachineID) jobs.Step {
	return jobs.Step{
		Name: "let " + string(id) + " issue from the new authority",
		Do: func(ctx context.Context, job *model.Job) error {
			next, err := d.pending(ctx, cluster)
			if err != nil {
				return err
			}
			return d.rewrite(ctx, id, job, func(state State, cfg []byte, role model.MachineRole) ([]byte, bool, error) {
				if state.IssuesFrom(next.Crt) {
					return nil, false, nil
				}
				if !state.Accepts(next.Crt) {
					return nil, false, fmt.Errorf("rotateca: %s does not accept the new authority yet, "+
						"so switching it would present a certificate its peers refuse: pass 1 has not "+
						"happened here. %w", id, ErrOutOfOrder)
				}
				out, err := machineconfig.ApplyPatches(cfg, []string{IssuePatch(next, role)})
				return out, true, err
			})
		},
		Happened: func(ctx context.Context, _ *model.Job) (bool, error) {
			next, err := d.pending(ctx, cluster)
			if err != nil {
				return false, err
			}
			state, err := d.read(ctx, id)
			if err != nil {
				return false, err
			}
			return state.IssuesFrom(next.Crt), nil
		},
	}
}

// adoptStep is pass 3: this installation takes a certificate from the new
// authority, proves it against a node, and only then keeps it.
//
// It sits between pass 2 and pass 4 and not at either end. Before it, the
// nodes already issue from the new authority but still accept the old one, so
// the old certificate still works; after it, the old one can be dropped. Doing
// it earlier would mean holding a certificate no node accepts yet; later means
// dropping the only authority that lets this installation in.
func adoptStep(d Deps, cluster model.ClusterID) jobs.Step {
	return jobs.Step{
		Name: "take a certificate from the new authority and prove it",
		Do: func(ctx context.Context, _ *model.Job) error {
			next, err := d.pending(ctx, cluster)
			if err != nil {
				return err
			}
			// The proof inside dials every node in turn, so this budget is the
			// per-node one times the fleet rather than a single call's.
			ctx, cancel := context.WithTimeout(ctx, AdoptBudget)
			defer cancel()

			return d.AdoptAuthority(ctx, cluster, next)
		},
		Happened: func(ctx context.Context, _ *model.Job) (bool, error) {
			sec, err := d.Store.ClusterSecrets().Get(ctx, cluster)
			if err != nil {
				return false, err
			}
			return sameCertificate(sec.OSCACrt, sec.NextOSCACrt), nil
		},
	}
}

// pruneStep is pass 4 for one node.
func pruneStep(d Deps, cluster model.ClusterID, id model.MachineID) jobs.Step {
	return jobs.Step{
		Name: "let " + string(id) + " stop accepting the old authority",
		Do: func(ctx context.Context, job *model.Job) error {
			next, err := d.pending(ctx, cluster)
			if err != nil {
				return err
			}
			return d.rewrite(ctx, id, job, func(_ State, cfg []byte, _ model.MachineRole) ([]byte, bool, error) {
				return PruneConfig(cfg, next.Crt)
			})
		},
		Happened: func(ctx context.Context, _ *model.Job) (bool, error) {
			next, err := d.pending(ctx, cluster)
			if err != nil {
				return false, err
			}
			state, err := d.read(ctx, id)
			if err != nil {
				return false, err
			}
			if len(state.AcceptedCrts) == 0 {
				return state.IssuesFrom(next.Crt), nil
			}
			return len(state.AcceptedCrts) == 1 && sameCertificate(state.AcceptedCrts[0], next.Crt), nil
		},
	}
}

// finishStep clears the authority the cluster has finished moving to.
//
// Last, because while it is set every step above can tell which authority this
// run is about. Clearing it is what makes a second rotation a second rotation
// rather than a resume of this one.
func finishStep(d Deps, cluster model.ClusterID) jobs.Step {
	return jobs.Step{
		Name: "record the rotation as complete",
		Do: func(ctx context.Context, _ *model.Job) error {
			sec, err := d.Store.ClusterSecrets().Get(ctx, cluster)
			if err != nil {
				return err
			}
			if len(sec.NextOSCACrt) == 0 {
				return nil
			}
			if !sameCertificate(sec.OSCACrt, sec.NextOSCACrt) {
				return fmt.Errorf("rotateca: the stored authority is not the one this rotation "+
					"moved to, so the rotation is not complete and this step will not say it is. %w",
					ErrOutOfOrder)
			}
			sec.NextOSCACrt, sec.NextOSCAKey = nil, nil
			_, err = d.Store.ClusterSecrets().Put(ctx, sec)
			return err
		},
		Happened: func(ctx context.Context, _ *model.Job) (bool, error) {
			sec, err := d.Store.ClusterSecrets().Get(ctx, cluster)
			if err != nil {
				return false, err
			}
			return len(sec.NextOSCACrt) == 0, nil
		},
	}
}

// AdoptBudget bounds pass 3, which dials node after node with the new
// certificate until one answers: "the certificate is wrong" and "the node I
// happened to pick is off" are different findings, and only the first is a
// reason to throw the certificate away.
//
// A plain budget rather than a class deadline, because what it bounds is a
// series of TLS handshakes and not one RPC. Every RPC below takes its class
// deadline instead -- talos.WithClassDeadline -- which is what internal/jobs
// and internal/upgrade do, because the engine hands a step a context that can
// be cancelled and has no deadline, while D-04 refuses a Talos call without
// one.
const AdoptBudget = 5 * time.Minute

// pending is the authority this rotation is moving to.
func (d Deps) pending(ctx context.Context, cluster model.ClusterID) (Authority, error) {
	sec, err := d.Store.ClusterSecrets().Get(ctx, cluster)
	if err != nil {
		return Authority{}, err
	}
	if len(sec.NextOSCACrt) == 0 || len(sec.NextOSCAKey) == 0 {
		return Authority{}, errors.New("rotateca: this cluster has no rotation in progress, " +
			"so there is no new authority to write. The first step of the job generates it")
	}
	return Authority{Crt: sec.NextOSCACrt, Key: sec.NextOSCAKey}, nil
}

// read is one node's authorities, as the node itself reports them.
func (d Deps) read(ctx context.Context, id model.MachineID) (State, error) {
	cc, err := d.Connect(ctx, id)
	if err != nil {
		return State{}, err
	}
	defer cc.Close() //nolint:errcheck // a close error is not the rotation's verdict

	cfg, err := d.configOf(ctx, cc)
	if err != nil {
		return State{}, err
	}
	return Inspect(cfg)
}

// configOf reads a node's active configuration under the COSI read's own
// class deadline.
func (d Deps) configOf(ctx context.Context, cc *talos.ClusterClient) ([]byte, error) {
	readCtx, cancel, err := talos.WithClassDeadline(ctx, talos.MethodCOSIGet)
	if err != nil {
		return nil, err
	}
	defer cancel()

	return cc.MachineConfigYAML(readCtx)
}

// rewrite reads a node's configuration, lets edit decide what the new one is,
// and hands it back to the node.
//
// The whole configuration goes back rather than a patch, because that is the
// only thing Talos's ApplyConfiguration takes -- the patch is applied here,
// against the bytes that came off this node, so a node whose configuration
// differs from its neighbours' keeps its own.
//
// no-reboot and not auto. These two fields are applied by a service restart on
// the node, which is how talosctl rotate-ca works, and `auto` would leave the
// decision with a heuristic on a fleet-wide operation. If a node ever answers
// that it cannot apply without a reboot, that is a finding to read rather than
// a reboot to take during a CA rotation.
func (d Deps) rewrite(
	ctx context.Context,
	id model.MachineID,
	job *model.Job,
	edit func(state State, cfg []byte, role model.MachineRole) ([]byte, bool, error),
) error {
	cc, err := d.Connect(ctx, id)
	if err != nil {
		return err
	}
	defer cc.Close() //nolint:errcheck // as above

	cfg, err := d.configOf(ctx, cc)
	if err != nil {
		return err
	}
	state, err := Inspect(cfg)
	if err != nil {
		return err
	}

	role, err := d.roleOf(ctx, id)
	if err != nil {
		return err
	}

	next, changes, err := edit(state, cfg, role)
	if err != nil {
		return err
	}
	if !changes {
		job.Steps[job.Current].Detail = "already done"
		return nil
	}

	applyCtx, cancel, err := talos.WithClassDeadline(ctx, talos.MethodApplyConfiguration)
	if err != nil {
		return err
	}
	defer cancel()

	res, err := cc.ApplyConfigurationWithMode(applyCtx, next, string(machineconfig.ModeNoReboot))
	if err != nil {
		return err
	}
	job.Steps[job.Current].Detail = strings.TrimSpace(res.Mode + " " + res.Details)
	return nil
}

// roleOf is the node's role, which decides whether it is handed the
// authority's key.
func (d Deps) roleOf(ctx context.Context, id model.MachineID) (model.MachineRole, error) {
	rec, err := d.Store.Machines().Get(ctx, id)
	if err != nil {
		return "", err
	}
	return rec.Role, nil
}
