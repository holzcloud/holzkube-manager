package jobs_test

import (
	"context"
	"log/slog"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/streamhub"
)

// Every job published to its own topic and nothing ever dropped it, so the hub
// grew by one ring buffer per job for the life of the process.
func TestAFinishedJobsTopicIsDroppedAfterTheLinger(t *testing.T) {
	t.Parallel()

	st := newStore(t)
	hub := streamhub.New()
	t.Cleanup(func() { _ = hub.Close() })
	e := jobs.New(jobs.Deps{
		Store:       st,
		Hub:         hub,
		TopicLinger: 500 * time.Millisecond,
		Logger:      slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})
	t.Cleanup(func() { _ = e.Close() })
	e.Register(model.JobReboot, func(model.Job) ([]jobs.Step, error) {
		return []jobs.Step{{Name: "x", Do: func(context.Context, *model.Job) error { return nil }}}, nil
	})

	j, err := e.Submit(t.Context(), model.Job{Kind: model.JobReboot, Cluster: testCluster, Machine: "m1"})
	if err != nil {
		t.Fatal(err)
	}
	waitForTerminal(t, e, j.ID, 10*time.Second)

	// Within the linger the topic is still there for a viewer that just arrived.
	if !slices.Contains(hub.Topics(), jobs.Topic(j.ID)) {
		t.Fatal("the topic was dropped the instant the job ended, with no linger")
	}

	deadline := time.Now().Add(5 * time.Second)
	for slices.Contains(hub.Topics(), jobs.Topic(j.ID)) {
		if time.Now().After(deadline) {
			t.Fatal("the finished job's topic was never dropped")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
