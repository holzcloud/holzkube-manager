package httpapi_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// TestTheStatusRouteSaysWhenAJobIsExecuting is the bit the update script reads
// before it restarts the daemon.
//
// The strings are compared as bytes on purpose. deploy/holzkube-manager-update.sh
// has no JSON parser at that point and greps the body for `"busy":true`, so
// the spelling -- no space after the colon -- is part of the contract between
// the two, and a pretty-printed answer would make the script see an idle
// daemon for ever.
func TestTheStatusRouteSaysWhenAJobIsExecuting(t *testing.T) {
	h := newJobHarness(t)

	_, raw := h.do(t, http.MethodGet, "/api/v1/system/status", nil)
	if !strings.Contains(string(raw), `"busy":false`) {
		t.Fatalf("an idle instance does not answer \"busy\":false: %s", raw)
	}

	release := make(chan struct{})
	entered := make(chan struct{})
	h.jobs.Register("test.hold", func(model.Job) ([]jobs.Step, error) {
		return []jobs.Step{{
			Name: "hold",
			Do: func(ctx context.Context, _ *model.Job) error {
				close(entered)
				select {
				case <-release:
				case <-ctx.Done():
				}
				return nil
			},
			Happened: func(context.Context, *model.Job) (bool, error) { return false, nil },
		}}, nil
	})
	j, err := h.jobs.Submit(t.Context(), model.Job{Kind: "test.hold"})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	<-entered

	_, raw = h.do(t, http.MethodGet, "/api/v1/system/status", nil)
	if !strings.Contains(string(raw), `"busy":true`) {
		t.Errorf("a job is inside a step and the status route does not answer \"busy\":true: %s", raw)
	}

	close(release)
	deadline := time.Now().Add(10 * time.Second)
	for h.jobs.Busy() {
		if time.Now().After(deadline) {
			t.Fatalf("job %s finished its only step and the engine is still busy", j.ID)
		}
		time.Sleep(10 * time.Millisecond)
	}
	_, raw = h.do(t, http.MethodGet, "/api/v1/system/status", nil)
	if !strings.Contains(string(raw), `"busy":false`) {
		t.Errorf("the job is over and the status route does not answer \"busy\":false: %s", raw)
	}
}
