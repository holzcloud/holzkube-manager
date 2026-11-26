package handlers

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
)

func newEngine(t *testing.T) *jobs.Engine {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod data dir: %v", err)
	}
	st, err := fsstore.Open(dir)
	if err != nil {
		t.Fatalf("fsstore.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	e := jobs.New(jobs.Deps{Store: st, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	t.Cleanup(func() { _ = e.Close() })
	registerExclusive(e)
	e.Register("test.noop", func(model.Job) ([]jobs.Step, error) {
		return []jobs.Step{{Name: "noop", Do: func(context.Context, *model.Job) error { return nil }}}, nil
	})
	return e
}

// TestAHoldRefusesAJob: while a request holds its cluster, a job submitted on
// that cluster is refused with the engine's busy error -- the half of the lease
// an HTTP test cannot time, because an apply against the simulator lasts one
// RPC. Once released, the cluster takes a job again.
func TestAHoldRefusesAJob(t *testing.T) {
	e := newEngine(t)
	d := httpapi.Deps{Jobs: e}

	release, err := holdCluster(t.Context(), d, "c-1", "config.apply", "m-1")
	if err != nil {
		t.Fatalf("hold: %v", err)
	}
	// Before the engine's own cleanup (cleanups run last-in first-out): a test
	// that stops early must not leave Close waiting on a hold nobody releases.
	t.Cleanup(release)

	if _, err := e.Submit(t.Context(), model.Job{Kind: "test.noop", Cluster: "c-1"}); !errors.Is(err, jobs.ErrClusterBusy) {
		t.Fatalf("a job on a held cluster: %v, want ErrClusterBusy", err)
	}
	if _, err := holdCluster(t.Context(), d, "c-1", "etcd.restore", "m-2"); !errors.Is(err, jobs.ErrClusterBusy) {
		t.Fatalf("a second hold on a held cluster: %v, want ErrClusterBusy", err)
	}
	if _, err := e.Submit(t.Context(), model.Job{Kind: "test.noop", Cluster: "c-2"}); err != nil {
		t.Fatalf("another cluster is not held: %v", err)
	}

	// A cancel of the hold job must not hand the lease back while the request
	// it protects is still running.
	list, _ := e.List(t.Context())
	for _, j := range list {
		if j.Kind != JobKindExclusive {
			continue
		}
		// The engine writes the record as the step starts, and a cancel landing
		// on the same revision is a store conflict; it is asked again.
		var err error
		for range 50 {
			if _, err = e.Cancel(t.Context(), j.ID); err == nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if err != nil {
			t.Fatalf("cancel the hold: %v", err)
		}
	}
	time.Sleep(50 * time.Millisecond)
	if _, err := e.Submit(t.Context(), model.Job{Kind: "test.noop", Cluster: "c-1"}); !errors.Is(err, jobs.ErrClusterBusy) {
		t.Fatalf("after cancelling the hold job the cluster took a job (%v) while the request still ran", err)
	}

	release()
	release() // twice is harmless

	deadline := time.Now().Add(5 * time.Second)
	for {
		_, err := e.Submit(t.Context(), model.Job{Kind: "test.noop", Cluster: "c-1"})
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the cluster is still held after the release: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestAHoldLeftByACrashIsNotParked: a process that died inside a hold has
// nothing left to wait for, so Resume closes the job instead of parking it for
// a person.
func TestAHoldLeftByACrashIsNotParked(t *testing.T) {
	steps, err := buildExclusive(model.Job{Params: map[string]string{exclusiveParamHold: "gone"}})
	if err != nil || len(steps) != 1 {
		t.Fatalf("build: %v, %d steps", err, len(steps))
	}
	if !steps[0].Verifiable() {
		t.Fatal("the hold step is unverifiable, so an interrupted one would be parked")
	}
	done := make(chan error, 1)
	go func() { done <- steps[0].Do(t.Context(), &model.Job{}) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Do: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a hold whose request is gone waits forever")
	}
}
