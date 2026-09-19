package kube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// JobKindDrain is draining a node as a job.
//
// A job and not a synchronous route, and the reason is the write timeout rather
// than taste: a drain waits out each pod's termination grace period, which is
// thirty seconds by default and whatever the workload's owner chose in general.
// A node with a dozen pods can therefore outlast any HTTP response this server
// is willing to hold open -- and an operation whose answer arrives after the
// socket closed is an operation nobody can find out about.
//
// The other half is the same argument the engine was built on: a drain that was
// interrupted has a state worth knowing. The node is cordoned and some pods
// moved, which is a cluster somebody has to finish rather than one they have to
// guess about.
const JobKindDrain model.JobKind = "node.drain"

// DrainDeps is what the drain job needs from the composition root.
type DrainDeps struct {
	Logger *slog.Logger

	// Client reaches one cluster's Kubernetes API. It is a function rather than
	// a client because a job outlives the request that submitted it, and the
	// certificate this product mints is good for an hour: a resumed job has to
	// be able to get a fresh one.
	Client func(ctx context.Context, cluster model.ClusterID) (*Client, error)
}

// DrainRequest is a drain as the operator asked for it.
type DrainRequest struct {
	Node string `json:"node"`

	// The two decisions the drain refuses to take on its own. They are stored
	// on the job because a resumed drain must run the drain that was asked for
	// -- a resume that quietly gained --force is the trap JOB-07 exists for.
	Force           bool `json:"force"`
	DeleteLocalData bool `json:"delete_local_data"`
}

// Params renders a request for storage.
func (r DrainRequest) Params() (map[string]string, error) {
	raw, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	return map[string]string{"request": string(raw)}, nil
}

// DrainRequestFromParams reads a stored job's parameters.
func DrainRequestFromParams(p map[string]string) (DrainRequest, error) {
	var r DrainRequest
	raw, ok := p["request"]
	if !ok {
		return DrainRequest{}, errors.New("kube: this job carries no request")
	}
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return DrainRequest{}, fmt.Errorf("kube: this job's request could not be read: %w", err)
	}
	if strings.TrimSpace(r.Node) == "" {
		return DrainRequest{}, errors.New("kube: this drain names no node")
	}
	return r, nil
}

// DrainBudget bounds one drain.
//
// Ten minutes: long enough for a dozen pods with the default thirty-second
// grace period plus the cluster taking its time, and short enough that a drain
// nobody can finish says so rather than running until the process restarts. A
// drain that hits it is not a failure with nothing to show -- the job carries
// what moved and what is left.
const DrainBudget = 10 * time.Minute

// RegisterDrain wires the drain into the engine.
func RegisterDrain(e *jobs.Engine, d DrainDeps) {
	e.Register(JobKindDrain, func(j model.Job) ([]jobs.Step, error) {
		req, err := DrainRequestFromParams(j.Params)
		if err != nil {
			return nil, err
		}
		return []jobs.Step{drainStep(d, j.Cluster, req)}, nil
	})
}

func drainStep(d DrainDeps, cluster model.ClusterID, req DrainRequest) jobs.Step {
	return jobs.Step{
		Name: "drain " + req.Node,

		Do: func(ctx context.Context, job *model.Job) error {
			client, err := d.Client(ctx, cluster)
			if err != nil {
				return err
			}

			result, err := client.Drain(ctx, req.Node, DrainOptions{
				Force:           req.Force,
				DeleteLocalData: req.DeleteLocalData,
				Timeout:         DrainBudget,
			})

			// The detail is written whether or not the drain finished, because
			// a drain that stopped is exactly the case where what it did
			// matters: the node is cordoned and some pods moved.
			job.Steps[job.Current].Detail = describeDrain(result)
			return err
		},

		// A drain is verifiable, which is unusual for a destructive step and
		// true here: "are there pods left on this node that this drain would
		// move" is a read, and it answers the same question the step was asked.
		// So an interrupted drain resumes rather than parking for a human.
		Happened: func(ctx context.Context, _ *model.Job) (bool, error) {
			client, err := d.Client(ctx, cluster)
			if err != nil {
				return false, err
			}

			pods, err := client.PodsOnNode(ctx, req.Node)
			if err != nil {
				return false, err
			}
			for _, pod := range pods {
				if pod.wouldMove(DrainOptions{Force: req.Force, DeleteLocalData: req.DeleteLocalData}) {
					return false, nil
				}
			}
			return true, nil
		},
	}
}

// describeDrain is the sentence the jobs screen shows.
//
// Counts and not lists, with the blocked pods named: a screen that printed
// forty evicted pod names would bury the two that did not move, and those two
// are the whole reason to look.
func describeDrain(r DrainResult) string {
	parts := []string{strconv.Itoa(len(r.Evicted)) + " evicted"}
	if len(r.SkippedDaemonSet) > 0 {
		parts = append(parts, strconv.Itoa(len(r.SkippedDaemonSet))+" DaemonSet pods left in place")
	}
	if len(r.SkippedMirror) > 0 {
		parts = append(parts, strconv.Itoa(len(r.SkippedMirror))+" static pods left in place")
	}
	for _, b := range r.Blocked {
		parts = append(parts, b.Pod+": "+b.Reason)
	}
	return strings.Join(parts, "; ")
}
