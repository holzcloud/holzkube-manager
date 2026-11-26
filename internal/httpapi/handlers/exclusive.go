package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"sync"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/machineconfig"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// JobKindExclusive is the job that holds a cluster's lease while a route that
// acts inside its own request -- a configuration apply, an etcd restore, an
// etcd member removal -- runs.
//
// The jobs engine allows one mutating job per cluster (JOB-03), and those
// routes used to run beside it unseen: a Talos upgrade with one control-plane
// node rebooting, and a config apply in reboot mode on a second, was two of
// three down. The engine's lease is taken only by Submit, so these routes
// submit a job whose single step waits for the request to finish. That makes
// them refused while a job holds the cluster (409 store.cluster-busy, the same
// answer a second job gets), and makes a job submitted meanwhile refused in
// turn. The job is also visible in the job list, which says honestly what was
// holding the cluster.
const JobKindExclusive model.JobKind = "cluster.exclusive"

const (
	exclusiveParamHold    = "hold"
	exclusiveParamAction  = "action"
	exclusiveParamMachine = "machine"
)

// holds maps a hold token to the channel its request closes when done. In
// memory on purpose: the section cannot outlive the process that runs it.
var holds sync.Map

// registerExclusive teaches the engine the kind. It is called while the route
// table is built, which is before Resume runs at startup, so a hold job left
// behind by a crash meets a builder rather than "unknown kind".
func registerExclusive(e *jobs.Engine) {
	if e == nil {
		return
	}
	e.Register(JobKindExclusive, buildExclusive)
}

func buildExclusive(j model.Job) ([]jobs.Step, error) {
	token := j.Params[exclusiveParamHold]
	what := j.Params[exclusiveParamAction]
	if what == "" {
		what = "an action"
	}
	return []jobs.Step{{
		Name: "hold the cluster while " + what + " runs",
		// The context is ignored, and that is the point: a cancel of this job
		// must not hand the lease back while the request it protects is still
		// talking to a node. The request ends on its own budget.
		Do: func(context.Context, *model.Job) error {
			if v, ok := holds.Load(token); ok {
				if done, ok := v.(chan struct{}); ok {
					<-done
				}
			}
			return nil
		},
		// Verifiable, and always yes: the section runs inside a request of this
		// process, so a process that restarted has nothing left of it to wait
		// for. Without this a crash would park a job a person has to close.
		Happened: func(context.Context, *model.Job) (bool, error) { return true, nil },
	}}, nil
}

// guardApply is what a configuration apply does before it sends anything: it
// takes the cluster's lease for the rest of the request, and for a mode that
// reboots a control-plane node it asks the etcd health gate first -- the gate an
// upgrade asks before every node, called and not re-implemented.
//
// It answers a release to defer, or a problem to write, or an error from
// the engine (jobs.ErrClusterBusy) for writeJobError. In dry-run nothing is
// held and nothing asked: the apply refuses on its own, and a hold job for an
// apply that cannot happen would be a record of something that did not.
func guardApply(ctx context.Context, d httpapi.Deps, id model.MachineID, mode machineconfig.Mode) (func(), *httpapi.Problem, error) {
	none := func() {}
	if d.TalosMode.DryRun || d.Inventory == nil {
		return none, nil, nil
	}
	m, err := d.Inventory.MachineRecord(ctx, id)
	if errors.Is(err, inventory.ErrNotFound) {
		// The apply's own answer for a machine nobody has is the right one.
		return none, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}

	release, err := holdCluster(ctx, d, m.Cluster, "config.apply", id)
	if err != nil {
		return nil, nil, err
	}

	// try applies with a rollback timer and no reboot, staged changes nothing
	// now, no-reboot refuses a change that would need one. reboot reboots,
	// and auto usually does -- Talos decides, so it is counted as one.
	reboots := mode == machineconfig.ModeReboot || mode == "auto"
	if !reboots || m.Role != model.RoleControlPlane || m.Cluster == "" {
		return release, nil, nil
	}

	refuse := func(detail string) (func(), *httpapi.Problem, error) {
		release()
		return nil, &httpapi.Problem{
			Type:   httpapi.TypeConflict,
			Title:  "etcd cannot spare this node right now",
			Status: http.StatusConflict,
			Detail: detail,
			Code:   httpapi.CodeEtcdGateRefused,
		}, nil
	}
	if d.Upgrade == nil {
		return refuse("This apply would reboot a control-plane node, and this instance was started " +
			"without the upgrade service, so the etcd health gate cannot be asked. A gate that " +
			"cannot be asked has not said yes; apply with no-reboot or staged instead.")
	}
	v, err := d.Upgrade.EvaluateGate(ctx, m.Cluster, id)
	if err != nil {
		return refuse("This apply would reboot a control-plane node, and the etcd health gate " +
			"could not be evaluated: " + err.Error())
	}
	if !v.OK {
		return refuse("This apply would reboot a control-plane node, and etcd would not survive " +
			"losing it now: " + v.Reason)
	}
	return release, nil, nil
}

// holdCluster takes cluster's lease for the rest of the request. It returns
// the release, which the caller defers, or the error Submit refused with --
// jobs.ErrClusterBusy while another job runs -- for writeJobError to answer.
//
// A machine in no cluster, or an instance built without the jobs engine, has
// nothing to serialise against, and the release is a no-op.
func holdCluster(ctx context.Context, d httpapi.Deps, cluster model.ClusterID, action string, machine model.MachineID) (func(), error) {
	if d.Jobs == nil || cluster == "" {
		return func() {}, nil
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(raw[:])
	done := make(chan struct{})
	holds.Store(token, done)

	actor := ""
	if d.Auth != nil {
		if u, ok := d.Auth.CurrentUser(ctx); ok {
			actor = u.Username
		}
	}
	params := map[string]string{exclusiveParamHold: token, exclusiveParamAction: action}
	if machine != "" {
		params[exclusiveParamMachine] = string(machine)
	}
	if _, err := d.Jobs.Submit(context.WithoutCancel(ctx), model.Job{
		Kind:    JobKindExclusive,
		Cluster: cluster,
		Machine: machine,
		Actor:   actor,
		Params:  params,
	}); err != nil {
		holds.Delete(token)
		return nil, err
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
			holds.Delete(token)
		})
	}, nil
}
