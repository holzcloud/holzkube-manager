package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// TestBusyIsAJobInsideAStepAndNothingElse pins what a restart is asked to wait
// for: a job that is executing. Not a finished one, and not a parked one --
// nothing runs in a parked job, and counting it would hold every update back
// until a person had decided about it.
func TestBusyIsAJobInsideAStepAndNothingElse(t *testing.T) {
	t.Parallel()

	st := newStore(t)
	e := newEngine(t, st)
	if e.Busy() {
		t.Fatal("an engine that has run nothing is busy")
	}

	entered := make(chan struct{})
	e.Register(model.JobReset, func(model.Job) ([]jobs.Step, error) {
		return []jobs.Step{{
			Name: "wipe the node",
			Do: func(ctx context.Context, _ *model.Job) error {
				close(entered)
				<-ctx.Done()
				return ctx.Err()
			},
			// No Happened: interrupted, this job is parked at the next start.
		}}, nil
	})
	// A job on a machine in no cluster takes no lease, and is still a job.
	if _, err := e.Submit(t.Context(), model.Job{Kind: model.JobReset, Machine: "m1"}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	waitEntered(t, entered)
	if !e.Busy() {
		t.Fatal("a job is inside a step and the engine is not busy")
	}

	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	second := newEngine(t, st)
	second.Register(model.JobReset, func(model.Job) ([]jobs.Step, error) {
		return []jobs.Step{{Name: "wipe the node", Do: func(context.Context, *model.Job) error { return nil }}}, nil
	})
	if err := second.Resume(t.Context()); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	list, err := second.List(t.Context())
	if err != nil || len(list) != 1 || list[0].State != model.JobParked {
		t.Fatalf("after the restart: %v, %+v; want the one job, parked", err, list)
	}
	deadline := time.Now().Add(5 * time.Second)
	for second.Busy() {
		if time.Now().After(deadline) {
			t.Fatal("a parked job keeps the engine busy; no update would ever be installed past it")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
