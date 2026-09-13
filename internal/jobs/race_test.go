package jobs_test

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// TestSubmitDoesNotHandBackAJobItsOwnGoroutineIsWriting is a production data
// race, found by CI's race detector and not by any local run.
//
// A model.Job is passed by value and is not therefore a copy: Steps is a slice
// header and Params is a map, and both point at storage the original keeps.
// Engine.start handed the job straight to the goroutine that runs it, while
// Submit's caller -- an HTTP handler -- still held the same value and was
// JSON-encoding it into its 202 response. run() writes Steps[i].State before
// each step and again after it. Two goroutines, one backing array, one of them
// writing.
//
// Every reboot, shutdown, reset, provision and upgrade submission takes that
// path, so this was not a test artefact: it was the response body of every
// destructive action in the product racing the action itself.
//
// There is deliberately no synchronisation between the marshalling and the
// job. The first version of this test waited on the step's "I have started"
// channel and then marshalled -- and passed even with the fix removed, because
// a channel receive is a happens-before edge: waiting on it is exactly what
// makes the two accesses ordered, which is the thing the test was supposed to
// prove does not happen. So the loop starts the moment Submit returns and the
// job runs through many quick steps underneath it.
func TestSubmitDoesNotHandBackAJobItsOwnGoroutineIsWriting(t *testing.T) {
	t.Parallel()

	st := newStore(t)
	e := newEngine(t, st)

	// Many small steps rather than one slow one: each is two writes into
	// Steps[i], so the window the marshalling has to land in is the whole run
	// rather than one instant.
	const steps = 40

	e.Register(model.JobReboot, func(model.Job) ([]jobs.Step, error) {
		list := make([]jobs.Step, 0, steps)
		for i := range steps {
			list = append(list, jobs.Step{
				Name: "step " + strconv.Itoa(i),
				Do:   func(context.Context, *model.Job) error { return nil },
			})
		}
		return list, nil
	})

	j, err := e.Submit(t.Context(), model.Job{
		Kind:    model.JobReboot,
		Cluster: testCluster,
		Machine: "m1",
		Params:  map[string]string{"graceful": "true"},
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// What the handler does with the value Submit returned, starting now.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := json.Marshal(j); err != nil {
			t.Fatalf("marshal the submitted job: %v", err)
		}
		if done, err := st.Jobs().Get(t.Context(), j.ID); err == nil && done.State.Terminal() {
			break
		}
	}
}

// TestCloneSharesNothingMutable is the property the fix rests on, asserted
// directly rather than through the race detector.
//
// The race test above only fails under -race. This one fails anywhere, which
// matters because a field added to model.Job that nobody adds to Clone is the
// same bug again and would otherwise be caught only on a CI run that happened
// to interleave badly.
func TestCloneSharesNothingMutable(t *testing.T) {
	t.Parallel()

	original := model.Job{
		ID:     "j1",
		Steps:  []model.JobStep{{Name: "one", State: model.StepPending}},
		Params: map[string]string{"mode": "graceful"},
	}

	clone := original.Clone()
	clone.Steps[0].State = model.StepRunning
	clone.Params["mode"] = "all"

	if original.Steps[0].State != model.StepPending {
		t.Error("writing a step on the clone changed the original; the Steps slice is shared, " +
			"which is the whole of the race this method exists to close")
	}
	if original.Params["mode"] != "graceful" {
		t.Error("writing a param on the clone changed the original; the Params map is shared")
	}
}
