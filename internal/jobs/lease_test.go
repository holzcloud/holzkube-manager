package jobs_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
)

// holdsTheEnd is a store whose job table stops at the write of a terminal
// state: it says "the end is being written" on arrived and does not write until
// proceed is closed. That is the window the tests below stand in.
type holdsTheEnd struct {
	store.Store
	jobs *holdingJobs
}

func (h *holdsTheEnd) Jobs() store.JobStore { return h.jobs }

type holdingJobs struct {
	store.JobStore

	once    sync.Once
	arrived chan struct{}
	proceed chan struct{}
}

func (h *holdingJobs) Put(ctx context.Context, rec model.Job) (model.Job, error) {
	if rec.State.Terminal() {
		h.once.Do(func() { close(h.arrived) })
		<-h.proceed
	}
	return h.JobStore.Put(ctx, rec)
}

func newHoldingStore(t *testing.T) (*holdsTheEnd, *holdingJobs) {
	t.Helper()
	inner := newStore(t)
	hj := &holdingJobs{
		JobStore: inner.Jobs(),
		arrived:  make(chan struct{}),
		proceed:  make(chan struct{}),
	}
	return &holdsTheEnd{Store: inner, jobs: hj}, hj
}

// letGo releases the held write. It is registered after the engine is made, so
// it runs before the engine's Close, which waits for every job and would
// otherwise wait for a write nobody is going to let through.
func (h *holdingJobs) letGo(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		select {
		case <-h.proceed:
		default:
			close(h.proceed)
		}
	})
}

// TestTheClusterIsFreeBeforeTheEndOfAJobIsWritten is the property behind a 409
// nobody could explain.
//
// A job was written as finished first and gave up its hold on the cluster a
// moment later, when its goroutine returned. A client that waited for
// "finished" and then sent the next job landed in that moment and was refused
// with "another job is already running on this cluster", with nothing running.
// The test stands in the moment: the write of the end is held, and a second job
// on the same cluster must be accepted while it is.
//
// The three ends are three code paths -- the last step done, a step that
// failed, a job cancelled at a boundary -- and each wrote the end before the
// hold was given up.
func TestTheClusterIsFreeBeforeTheEndOfAJobIsWritten(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		do   func(ctx context.Context, j *model.Job) error
	}{
		{"succeeded", func(context.Context, *model.Job) error { return nil }},
		{"failed", func(context.Context, *model.Job) error { return errors.New("the step failed") }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			st, hj := newHoldingStore(t)
			e := newEngine(t, st)
			hj.letGo(t)
			e.Register(model.JobReboot, func(model.Job) ([]jobs.Step, error) {
				return []jobs.Step{{Name: "the one step", Do: tc.do}}, nil
			})

			if _, err := e.Submit(t.Context(), model.Job{Kind: model.JobReboot, Cluster: testCluster, Machine: "m1"}); err != nil {
				t.Fatalf("first Submit: %v", err)
			}
			select {
			case <-hj.arrived:
			case <-time.After(10 * time.Second):
				t.Fatal("the end of the first job was never written")
			}

			// The end of the first job is not on disk yet. The cluster must
			// not be held for a job that has nothing left to do.
			_, err := e.Submit(t.Context(), model.Job{Kind: model.JobReboot, Cluster: testCluster, Machine: "m2"})
			if errors.Is(err, jobs.ErrClusterBusy) {
				t.Fatalf("a job that has run all its steps still holds the cluster: %v", err)
			}
		})
	}
}
