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

// errRPCCancelled is the shape a cancelled gRPC call hands back: a status
// error, which is neither context.Canceled nor a wrapper around it. Spelled
// out here instead of built with google.golang.org/grpc/status so that the
// engine's tests keep saying the engine knows nothing about a transport.
type errRPCCancelled struct{}

func (errRPCCancelled) Error() string { return "rpc error: code = Canceled desc = context canceled" }

// TestShutdownInsideARemoteCallLeavesTheJobForResume is the daemon restarting
// while a step is inside a Talos call -- which is what the hourly update does
// to whatever is running.
//
// The step's error is the transport's, not the context's, and the engine used
// to tell an interruption by the error. It recorded the job as failed, so the
// next start had nothing to resume and the remaining nodes of an upgrade were
// never walked.
func TestShutdownInsideARemoteCallLeavesTheJobForResume(t *testing.T) {
	t.Parallel()

	st := newStore(t)
	e := newEngine(t, st)
	entered := make(chan struct{})
	e.Register(model.JobReboot, func(model.Job) ([]jobs.Step, error) {
		return []jobs.Step{{
			Name: "upgrade the node",
			Do: func(ctx context.Context, _ *model.Job) error {
				close(entered)
				<-ctx.Done()
				return errRPCCancelled{}
			},
			Happened: func(context.Context, *model.Job) (bool, error) { return false, nil },
		}}, nil
	})

	j, err := e.Submit(t.Context(), model.Job{Kind: model.JobReboot, Cluster: testCluster, Machine: "m1"})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	waitEntered(t, entered)
	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	stored, err := st.Jobs().Get(t.Context(), j.ID)
	if err != nil {
		t.Fatalf("get job after the shutdown: %v", err)
	}
	if stored.State != model.JobRunning || stored.Steps[0].State != model.StepRunning {
		t.Fatalf("after a shutdown inside a remote call the job is %q and its step %q, want running "+
			"and running: a job written down as failed is one Resume never looks at again",
			stored.State, stored.Steps[0].State)
	}

	// And the restart does pick it up.
	ran := make(chan struct{})
	second := newEngine(t, st)
	second.Register(model.JobReboot, func(model.Job) ([]jobs.Step, error) {
		return []jobs.Step{{
			Name:     "upgrade the node",
			Do:       func(context.Context, *model.Job) error { close(ran); return nil },
			Happened: func(context.Context, *model.Job) (bool, error) { return false, nil },
		}}, nil
	})
	if err := second.Resume(t.Context()); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	select {
	case <-ran:
	case <-time.After(10 * time.Second):
		t.Fatal("the interrupted step was not run again after the restart")
	}
}

// TestCancelInsideARemoteCallEndsCancelled is the operator's cancel arriving
// while the step is inside a Talos call: cancelled, not failed.
func TestCancelInsideARemoteCallEndsCancelled(t *testing.T) {
	t.Parallel()

	st := newStore(t)
	e := newEngine(t, st)
	entered := make(chan struct{})
	e.Register(model.JobReboot, func(model.Job) ([]jobs.Step, error) {
		return []jobs.Step{{
			Name: "upgrade the node",
			Do: func(ctx context.Context, _ *model.Job) error {
				close(entered)
				<-ctx.Done()
				return errRPCCancelled{}
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
	if done := waitForTerminal(t, e, j.ID, 10*time.Second); done.State != model.JobCancelled {
		t.Fatalf("job is %q after an operator cancel inside a remote call, want cancelled", done.State)
	}
}
