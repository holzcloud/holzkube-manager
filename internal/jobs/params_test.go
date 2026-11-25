package jobs_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// TestAStepsChangeToTheJobsParamsIsStoredAndSeenByTheNextStep: a step that
// records something in the job's parameters -- the removal's cordon step
// records that the cordon was its own -- must find it again in the next step
// and in the stored job. The engine saves a handful of fields after each step;
// parameters were not among them, so a step's note to a later step went
// nowhere, and a restart between the two would not have had it either.
func TestAStepsChangeToTheJobsParamsIsStoredAndSeenByTheNextStep(t *testing.T) {
	t.Parallel()

	st := newStore(t)
	e := newEngine(t, st)

	var mu sync.Mutex
	var seen string
	e.Register(model.JobReset, func(model.Job) ([]jobs.Step, error) {
		return []jobs.Step{
			{Name: "note it", Do: func(_ context.Context, j *model.Job) error {
				j.Params["note"] = "from the first step"
				return nil
			}},
			{Name: "read it and fail", Do: func(_ context.Context, j *model.Job) error {
				mu.Lock()
				seen = j.Params["note"]
				mu.Unlock()
				return errors.New("failing on purpose, so the failed path stores the parameters too")
			}},
		}, nil
	})

	j, err := e.Submit(t.Context(), model.Job{
		Kind: model.JobReset, Cluster: testCluster, Machine: "m1",
		Params: map[string]string{"role": "worker"},
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	after := waitForTerminal(t, e, j.ID, 10*time.Second)

	mu.Lock()
	defer mu.Unlock()
	if seen != "from the first step" {
		t.Errorf("the second step saw the note %q, want the first step's", seen)
	}
	if got := after.Params["note"]; got != "from the first step" {
		t.Errorf("the stored job's parameters are %v; the first step's note was not stored", after.Params)
	}
	if got := after.Params["role"]; got != "worker" {
		t.Errorf("the submitted parameter was lost: %v", after.Params)
	}
}
