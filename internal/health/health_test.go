package health_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/health"
)

// TestLevelIsAStringOnTheWire pins D-13: the iota order is an implementation
// detail and must not reach a client.
func TestLevelIsAStringOnTheWire(t *testing.T) {
	t.Parallel()

	for _, lvl := range health.Levels() {
		b, err := json.Marshal(lvl)
		if err != nil {
			t.Fatalf("marshal %v: %v", lvl, err)
		}
		if !strings.HasPrefix(string(b), `"`) {
			t.Fatalf("level %v encoded as %s, want a JSON string", lvl, b)
		}

		var back health.Level
		if err := json.Unmarshal(b, &back); err != nil {
			t.Fatalf("unmarshal %s: %v", b, err)
		}
		if back != lvl {
			t.Fatalf("round trip of %v gave %v", lvl, back)
		}
	}
}

func TestUnknownLevelNameIsRefused(t *testing.T) {
	t.Parallel()

	var l health.Level
	if err := json.Unmarshal([]byte(`"quorum"`), &l); err == nil {
		t.Fatal("an unknown level name decoded without error; it must not silently become LevelNone")
	}
}

// TestThreeStatesAreDistinguishable is D-14's table, as a test.
func TestThreeStatesAreDistinguishable(t *testing.T) {
	t.Parallel()

	now := time.Now()

	confirmed := health.Known(health.LevelNode, "v1.13.9")
	if !confirmed.Available || confirmed.StaleSince != nil {
		t.Fatalf("confirmed field is %+v", confirmed)
	}

	stale := health.Stale(health.LevelNode, "v1.13.9", now, "node did not answer")
	if stale.Available || stale.StaleSince == nil {
		t.Fatalf("stale field is %+v", stale)
	}
	if stale.Value != "v1.13.9" {
		t.Fatal("a stale field dropped its value; the UI cannot tell that from null")
	}

	never := health.Never[string](health.LevelK8s, "no Kubernetes-derived resource on this node")
	if never.Available || never.StaleSince != nil || never.UnavailableReason == "" {
		t.Fatalf("never-read field is %+v", never)
	}
}

// TestStaleWithoutAMomentIsNeverRead guards the one shape the contract cannot
// express: "old, but we cannot say since when".
func TestStaleWithoutAMomentIsNeverRead(t *testing.T) {
	t.Parallel()

	f := health.Stale(health.LevelNode, 7, time.Time{}, "never asked")
	if f.StaleSince != nil {
		t.Fatalf("a zero stale_since survived: %+v", f)
	}
}

func TestOnlyLiveStagesClaimConfirmation(t *testing.T) {
	t.Parallel()

	want := map[health.Stage]bool{
		health.StageUnknown:    false,
		health.StageConnecting: false,
		health.StageWatching:   true,
		health.StageDegraded:   true,
		health.StageDown:       false,
	}
	for _, s := range health.Stages() {
		if s.Live() != want[s] {
			t.Fatalf("stage %v Live()=%v, want %v", s, s.Live(), want[s])
		}
	}
}
