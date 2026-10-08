package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// The operator's cancel and the process's shutdown both cancel the step's
// context, and used to be indistinguishable: a cancel during a running step was
// read as a shutdown, left the job "running" with CancelRequested set, and the
// next restart resumed the very job the operator had cancelled.

func waitEntered(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		t.Fatal("the step never started")
	}
}

func TestCancelDuringARunningStepEndsCancelled(t *testing.T) {
	t.Parallel()

	st := newStore(t)
	e := newEngine(t, st)
	entered := make(chan struct{})
	e.Register(model.JobReboot, func(model.Job) ([]jobs.Step, error) {
		return []jobs.Step{{
			Name: "wait for the node",
			Do: func(ctx context.Context, _ *model.Job) error {
				close(entered)
				<-ctx.Done()
				return ctx.Err()
			},
			Happened: func(context.Context, *model.Job) (bool, error) { return false, nil },
		}}, nil
	})

	j, err := e.Submit(t.Context(), model.Job{Kind: model.JobReboot, Cluster: testCluster, Machine: "m1"})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	waitEntered(t, entered)

	if _, err := e.Cancel(t.Context(), j.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	done := waitForTerminal(t, e, j.ID, 10*time.Second)
	if done.State != model.JobCancelled {
		t.Fatalf("job is %q after an operator cancel, want cancelled", done.State)
	}

	// A restart must not bring it back.
	_ = e.Close()
	second := newEngine(t, st)
	second.Register(model.JobReboot, func(model.Job) ([]jobs.Step, error) {
		return []jobs.Step{{Name: "wait for the node", Do: func(context.Context, *model.Job) error {
			t.Error("a cancelled job was resumed")
			return nil
		}, Happened: func(context.Context, *model.Job) (bool, error) { return false, nil }}}, nil
	})
	if err := second.Resume(t.Context()); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
}

func TestShutdownAtAStepBoundaryDoesNotCancelTheJob(t *testing.T) {
	t.Parallel()

	st := newStore(t)
	first := newEngine(t, st)
	entered := make(chan struct{})
	build := func(ran *chan struct{}) jobs.Builder {
		return func(model.Job) ([]jobs.Step, error) {
			return []jobs.Step{
				{
					Name: "one",
					// Swallows the shutdown and returns cleanly, so the engine
					// reaches the boundary with a cancelled context.
					Do: func(ctx context.Context, _ *model.Job) error {
						if ran == nil {
							close(entered)
							<-ctx.Done()
						}
						return nil
					},
					Happened: func(context.Context, *model.Job) (bool, error) { return false, nil },
				},
				{
					Name: "two",
					Do: func(context.Context, *model.Job) error {
						if ran != nil {
							close(*ran)
						}
						return nil
					},
					Happened: func(context.Context, *model.Job) (bool, error) { return false, nil },
				},
			}, nil
		}
	}
	first.Register(model.JobReboot, build(nil))

	j, err := first.Submit(t.Context(), model.Job{Kind: model.JobReboot, Cluster: testCluster, Machine: "m1"})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	waitEntered(t, entered)
	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	stored, err := st.Jobs().Get(t.Context(), j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State == model.JobCancelled {
		t.Fatalf("a process shutdown at a step boundary marked the job cancelled")
	}

	ran := make(chan struct{})
	second := newEngine(t, st)
	second.Register(model.JobReboot, build(&ran))
	if err := second.Resume(t.Context()); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	select {
	case <-ran:
	case <-time.After(10 * time.Second):
		t.Fatal("the job was not resumed after the restart")
	}
}

func TestCancellingAParkedJobCancelsIt(t *testing.T) {
	t.Parallel()

	st := newStore(t)
	e := newEngine(t, st)
	e.Register(model.JobReset, func(model.Job) ([]jobs.Step, error) {
		return []jobs.Step{{Name: "wipe", Do: func(context.Context, *model.Job) error { return nil }}}, nil
	})
	parked, err := st.Jobs().Put(t.Context(), model.Job{
		ID: "parked1", Kind: model.JobReset, Cluster: testCluster, State: model.JobParked,
		ParkedReason: "look at the node",
		Steps:        []model.JobStep{{Name: "wipe", State: model.StepRunning}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := e.Cancel(t.Context(), parked.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	got, err := e.Get(t.Context(), parked.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != model.JobCancelled {
		t.Fatalf("a cancelled parked job is %q, want cancelled", got.State)
	}
	if got.FinishedAt.IsZero() {
		t.Error("a cancelled parked job has no finish time")
	}
}

func TestSubmitAfterCloseLeavesNothingBehind(t *testing.T) {
	t.Parallel()

	st := newStore(t)
	e := newEngine(t, st)
	e.Register(model.JobReboot, func(model.Job) ([]jobs.Step, error) {
		return []jobs.Step{{Name: "x", Do: func(context.Context, *model.Job) error { return nil }}}, nil
	})
	_ = e.Close()

	if _, err := e.Submit(t.Context(), model.Job{Kind: model.JobReboot, Cluster: testCluster, Machine: "m1"}); err == nil {
		t.Fatal("Submit after Close succeeded")
	}
	all, err := st.Jobs().List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Fatalf("Submit after Close left %d job record(s) behind, first is %q", len(all), all[0].State)
	}
}
