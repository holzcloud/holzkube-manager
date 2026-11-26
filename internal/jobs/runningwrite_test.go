package jobs_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
)

// refusesRunning is a store whose job table refuses every write that records
// a step as running. Everything else is written.
type refusesRunning struct {
	store.Store
	jobs *refusingJobs
}

func (r *refusesRunning) Jobs() store.JobStore { return r.jobs }

type refusingJobs struct {
	store.JobStore
}

var errDiskFull = errors.New("no space left on device")

func (r *refusingJobs) Put(ctx context.Context, rec model.Job) (model.Job, error) {
	for _, s := range rec.Steps {
		if s.State == model.StepRunning {
			return model.Job{}, errDiskFull
		}
	}
	return r.JobStore.Put(ctx, rec)
}

// TestAStepWhoseRunningStateCannotBeRecordedDoesNotRun is the crash reasoning
// with its premise taken away.
//
// Resume decides about an interrupted step from the record that says the step
// was running. When that write failed, the engine used to log it and run the
// step anyway; a crash inside the step then left a record saying the step had
// never started, and Resume ran it as new. For a step with no Happened -- a
// wipe -- that is the second wipe. The test refuses exactly that write, crashes
// inside the step if it runs, and restarts.
func TestAStepWhoseRunningStateCannotBeRecordedDoesNotRun(t *testing.T) {
	t.Parallel()

	inner := newStore(t)
	st := &refusesRunning{Store: inner, jobs: &refusingJobs{JobStore: inner.Jobs()}}

	var (
		mu      sync.Mutex
		wipes   int
		entered = make(chan struct{}, 4)
	)
	build := func(model.Job) ([]jobs.Step, error) {
		return []jobs.Step{{
			Name: "wipe the node",
			Do: func(ctx context.Context, _ *model.Job) error {
				mu.Lock()
				wipes++
				mu.Unlock()
				entered <- struct{}{}
				<-ctx.Done()
				return ctx.Err()
			},
			Happened: nil,
		}}, nil
	}

	first := newEngine(t, st)
	first.Register(model.JobReset, build)
	j, err := first.Submit(t.Context(), model.Job{Kind: model.JobReset, Cluster: testCluster, Machine: "m1"})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Give the step every chance to start, then "crash" inside it.
	select {
	case <-entered:
	case <-time.After(300 * time.Millisecond):
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	second := newEngine(t, inner)
	second.Register(model.JobReset, build)
	if err := second.Resume(t.Context()); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	select {
	case <-entered:
	case <-time.After(300 * time.Millisecond):
	}
	_ = second.Close()

	mu.Lock()
	n := wipes
	mu.Unlock()
	if n != 0 {
		t.Fatalf("the wipe ran %d time(s) although its running state was never recorded; "+
			"a step not on record must not start", n)
	}

	got, err := inner.Jobs().Get(t.Context(), j.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State != model.JobFailed {
		t.Errorf("job is %q, want failed", got.State)
	}
	if got.Steps[0].State != model.StepFailed || !strings.Contains(got.Steps[0].Detail, "not started") {
		t.Errorf("step is %q (%q), want failed and saying it was not started",
			got.Steps[0].State, got.Steps[0].Detail)
	}
}
