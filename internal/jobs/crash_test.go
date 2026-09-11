package jobs_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
)

// The most valuable test in the project, in the roadmap's own words: kill the
// process in the middle of a step and assert correct resume-or-park.
//
// "Kill the process" is here a new Engine over the same data directory. The
// thing that survives a real kill -9 is the store, and the store here is the
// real fsstore with its real atomic writes — so what is being tested is
// exactly what would survive. What is *not* tested this way is the operating
// system's behaviour on an interrupted write, and that is covered separately
// by the crash-injection tests in internal/store/fsstore.

const testCluster = model.ClusterID("c1")

func newStore(t *testing.T) store.Store {
	t.Helper()

	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	st, err := fsstore.Open(dir)
	if err != nil {
		t.Fatalf("fsstore.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func newEngine(t *testing.T, st store.Store) *jobs.Engine {
	t.Helper()

	e := jobs.New(jobs.Deps{
		Store:  st,
		Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})
	t.Cleanup(func() { _ = e.Close() })
	return e
}

// interruptible is a step that blocks until the test lets it finish, so a
// "crash" can be staged precisely inside it.
type interruptible struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once

	mu       sync.Mutex
	ran      int
	happened bool
}

func (s *interruptible) enter() {
	s.once.Do(func() { close(s.entered) })
}

func (s *interruptible) runs() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ran
}

func (s *interruptible) setHappened(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.happened = v
}

// TestAnInterruptedUnverifiableStepParksTheJob is the sentence this engine was
// built around.
//
// A reset has no paired "did it happen?" query -- nothing a node can be asked
// distinguishes "wiped five seconds ago and booting" from "booting for another
// reason" -- so an interruption inside it must produce a parked job and a
// person's decision. Both alternatives are wrong in a way that matters:
// retrying wipes a second time, failing reports an intact node that is not.
func TestAnInterruptedUnverifiableStepParksTheJob(t *testing.T) {
	t.Parallel()

	st := newStore(t)
	step := &interruptible{entered: make(chan struct{}), release: make(chan struct{})}

	first := newEngine(t, st)
	first.Register(model.JobReset, func(model.Job) ([]jobs.Step, error) {
		return []jobs.Step{{
			Name: "wipe the node",
			Do: func(ctx context.Context, _ *model.Job) error {
				step.mu.Lock()
				step.ran++
				step.mu.Unlock()
				step.enter()
				<-ctx.Done()
				return ctx.Err()
			},
			// Deliberately nil: this is the case.
			Happened: nil,
		}}, nil
	})

	j, err := first.Submit(t.Context(), model.Job{Kind: model.JobReset, Cluster: testCluster, Machine: "m1"})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	select {
	case <-step.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the step never started")
	}

	// The crash: the process goes away while the step is in flight.
	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	stored, err := st.Jobs().Get(t.Context(), j.ID)
	if err != nil {
		t.Fatalf("get job after the crash: %v", err)
	}
	if stored.Steps[0].State != model.StepRunning {
		t.Fatalf("the interrupted step is recorded as %q, want running — "+
			"a record that does not say the step was in flight cannot be reasoned about",
			stored.Steps[0].State)
	}

	// The restart.
	second := newEngine(t, st)
	second.Register(model.JobReset, func(model.Job) ([]jobs.Step, error) {
		return []jobs.Step{{
			Name: "wipe the node",
			Do: func(context.Context, *model.Job) error {
				step.mu.Lock()
				step.ran++
				step.mu.Unlock()
				return nil
			},
			Happened: nil,
		}}, nil
	})

	if err := second.Resume(t.Context()); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	after, err := second.Get(t.Context(), j.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if after.State != model.JobParked {
		t.Fatalf("the job is %q after a restart inside an unverifiable step, want parked", after.State)
	}
	if after.ParkedReason == "" {
		t.Error("the parked job says nothing about what a person has to decide")
	}
	if step.runs() != 1 {
		t.Fatalf("the unverifiable step ran %d times across the restart; "+
			"a doubled side effect is exactly what this engine exists to prevent", step.runs())
	}
}

// TestAnInterruptedVerifiableStepIsSkippedWhenItAlreadyHappened is the other
// branch: the step can be checked, the check says it took effect, so it is not
// run again.
func TestAnInterruptedVerifiableStepIsSkippedWhenItAlreadyHappened(t *testing.T) {
	t.Parallel()

	st := newStore(t)
	step := &interruptible{entered: make(chan struct{}), release: make(chan struct{})}

	build := func(model.Job) ([]jobs.Step, error) {
		return []jobs.Step{{
			Name: "reboot the node",
			Do: func(ctx context.Context, _ *model.Job) error {
				step.mu.Lock()
				step.ran++
				step.mu.Unlock()
				step.enter()
				<-ctx.Done()
				return ctx.Err()
			},
			Happened: func(context.Context, *model.Job) (bool, error) {
				step.mu.Lock()
				defer step.mu.Unlock()
				return step.happened, nil
			},
		}}, nil
	}

	first := newEngine(t, st)
	first.Register(model.JobReboot, build)

	j, err := first.Submit(t.Context(), model.Job{Kind: model.JobReboot, Cluster: testCluster, Machine: "m1"})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	select {
	case <-step.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the step never started")
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// The node did in fact reboot before the process died.
	step.setHappened(true)

	second := newEngine(t, st)
	second.Register(model.JobReboot, func(model.Job) ([]jobs.Step, error) {
		return []jobs.Step{{
			Name: "reboot the node",
			Do: func(context.Context, *model.Job) error {
				step.mu.Lock()
				step.ran++
				step.mu.Unlock()
				return nil
			},
			Happened: func(context.Context, *model.Job) (bool, error) { return true, nil },
		}}, nil
	})

	if err := second.Resume(t.Context()); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	after := waitForTerminal(t, second, j.ID, 10*time.Second)
	if after.State != model.JobSucceeded {
		t.Fatalf("the job is %q, want succeeded", after.State)
	}
	if after.Steps[0].State != model.StepSkipped {
		t.Errorf("the step is %q, want skipped: its paired check said it had already taken effect",
			after.Steps[0].State)
	}
	if step.runs() != 1 {
		t.Fatalf("the step ran %d times; the check said it had already happened", step.runs())
	}
}

// TestAnInterruptedVerifiableStepRunsAgainWhenItDidNot is the third branch,
// and the one that would be a lost operation if it were missing.
func TestAnInterruptedVerifiableStepRunsAgainWhenItDidNot(t *testing.T) {
	t.Parallel()

	st := newStore(t)
	step := &interruptible{entered: make(chan struct{}), release: make(chan struct{})}

	first := newEngine(t, st)
	first.Register(model.JobReboot, func(model.Job) ([]jobs.Step, error) {
		return []jobs.Step{{
			Name: "reboot the node",
			Do: func(ctx context.Context, _ *model.Job) error {
				step.mu.Lock()
				step.ran++
				step.mu.Unlock()
				step.enter()
				<-ctx.Done()
				return ctx.Err()
			},
			Happened: func(context.Context, *model.Job) (bool, error) { return false, nil },
		}}, nil
	})

	j, err := first.Submit(t.Context(), model.Job{Kind: model.JobReboot, Cluster: testCluster, Machine: "m1"})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	select {
	case <-step.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the step never started")
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	second := newEngine(t, st)
	second.Register(model.JobReboot, func(model.Job) ([]jobs.Step, error) {
		return []jobs.Step{{
			Name: "reboot the node",
			Do: func(context.Context, *model.Job) error {
				step.mu.Lock()
				step.ran++
				step.mu.Unlock()
				return nil
			},
			Happened: func(context.Context, *model.Job) (bool, error) { return false, nil },
		}}, nil
	})

	if err := second.Resume(t.Context()); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	after := waitForTerminal(t, second, j.ID, 10*time.Second)
	if after.State != model.JobSucceeded {
		t.Fatalf("the job is %q, want succeeded", after.State)
	}
	if step.runs() != 2 {
		t.Fatalf("the step ran %d times; the check said it had not taken effect, so it had to run again",
			step.runs())
	}
}

// TestAnUnknownKindIsParkedRatherThanGuessed covers the upgrade case: a stored
// job whose kind this binary no longer builds must not be run with somebody
// else's steps.
func TestAnUnknownKindIsParkedRatherThanGuessed(t *testing.T) {
	t.Parallel()

	st := newStore(t)

	// Written straight to the store: this is a record an older binary left.
	if _, err := st.Jobs().Put(t.Context(), model.Job{
		ID:      "leftover",
		Kind:    model.JobKind("node.something-removed"),
		Cluster: testCluster,
		State:   model.JobRunning,
		Steps:   []model.JobStep{{Name: "do it", State: model.StepRunning}},
	}); err != nil {
		t.Fatalf("put: %v", err)
	}

	e := newEngine(t, st)
	if err := e.Resume(t.Context()); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	after, err := e.Get(t.Context(), "leftover")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if after.State != model.JobParked {
		t.Fatalf("a job of an unknown kind is %q, want parked", after.State)
	}
}

// TestOneMutatingJobPerCluster is JOB-03.
func TestOneMutatingJobPerCluster(t *testing.T) {
	t.Parallel()

	st := newStore(t)
	step := &interruptible{entered: make(chan struct{}), release: make(chan struct{})}

	e := newEngine(t, st)
	e.Register(model.JobReboot, func(model.Job) ([]jobs.Step, error) {
		return []jobs.Step{{
			Name: "reboot the node",
			Do: func(ctx context.Context, _ *model.Job) error {
				step.enter()
				select {
				case <-step.release:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			},
			Happened: func(context.Context, *model.Job) (bool, error) { return false, nil },
		}}, nil
	})

	if _, err := e.Submit(t.Context(), model.Job{Kind: model.JobReboot, Cluster: testCluster, Machine: "m1"}); err != nil {
		t.Fatalf("first Submit: %v", err)
	}
	select {
	case <-step.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the first job never started")
	}

	_, err := e.Submit(t.Context(), model.Job{Kind: model.JobReboot, Cluster: testCluster, Machine: "m2"})
	if !errors.Is(err, jobs.ErrClusterBusy) {
		t.Fatalf("the second job on the same cluster returned %v, want ErrClusterBusy", err)
	}

	// A different cluster is unaffected: the lease is per cluster and not
	// global, or one slow node would stop the whole fleet.
	other, err := e.Submit(t.Context(), model.Job{Kind: model.JobReboot, Cluster: "c2", Machine: "m3"})
	if err != nil {
		t.Fatalf("a job on another cluster was refused: %v", err)
	}
	if other.ID == "" {
		t.Error("the second cluster's job has no id")
	}

	close(step.release)
}

// TestCancelStopsAtAStepBoundary is JOB-04: a cancel takes effect between
// steps, never inside one. Cancelling inside a step would leave exactly the
// ambiguity the whole engine exists to avoid.
func TestCancelStopsAtAStepBoundary(t *testing.T) {
	t.Parallel()

	st := newStore(t)
	firstEntered := make(chan struct{})
	var secondRan int
	var mu sync.Mutex

	e := newEngine(t, st)
	e.Register(model.JobReboot, func(model.Job) ([]jobs.Step, error) {
		return []jobs.Step{
			{
				Name: "first",
				Do: func(ctx context.Context, _ *model.Job) error {
					close(firstEntered)
					// Long enough for the cancel to arrive while this step is
					// running, so the assertion is about the boundary and not
					// about a race.
					select {
					case <-time.After(500 * time.Millisecond):
					case <-ctx.Done():
					}
					return nil
				},
				Happened: func(context.Context, *model.Job) (bool, error) { return false, nil },
			},
			{
				Name: "second",
				Do: func(context.Context, *model.Job) error {
					mu.Lock()
					secondRan++
					mu.Unlock()
					return nil
				},
				Happened: func(context.Context, *model.Job) (bool, error) { return false, nil },
			},
		}, nil
	})

	j, err := e.Submit(t.Context(), model.Job{Kind: model.JobReboot, Cluster: testCluster, Machine: "m1"})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	<-firstEntered

	if _, err := e.Cancel(t.Context(), j.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	after := waitForTerminal(t, e, j.ID, 10*time.Second)
	if after.State != model.JobCancelled {
		t.Fatalf("the job is %q, want cancelled", after.State)
	}

	mu.Lock()
	defer mu.Unlock()
	if secondRan != 0 {
		t.Fatalf("the second step ran %d times after a cancel", secondRan)
	}
	if after.Steps[0].State != model.StepDone {
		t.Errorf("the first step is %q; a cancel at a boundary does not undo what already ran",
			after.Steps[0].State)
	}
}

// waitForTerminal polls until a job stops moving.
func waitForTerminal(t *testing.T, e *jobs.Engine, id model.JobID, within time.Duration) model.Job {
	t.Helper()

	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		j, err := e.Get(t.Context(), id)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if j.State.Terminal() || j.State == model.JobParked {
			return j
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job %s did not finish in %s", id, within)
	return model.Job{}
}
