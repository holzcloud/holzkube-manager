package nodestream_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/nodestream"
	"github.com/holzcloud/holzkube-manager/internal/streamhub"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

const machineID = model.MachineID("00000000-0000-4000-8000-000000000001")

func newManager(t *testing.T) (*nodestream.Manager, *streamhub.Hub, *talossim.Server) {
	t.Helper()

	sim, err := talossim.New(talossim.Options{Hostname: "stream-node", StreamMessages: 4})
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}
	t.Cleanup(func() { _ = sim.Close() })

	hub := streamhub.New()
	t.Cleanup(func() { _ = hub.Close() })

	m := nodestream.New(nodestream.Deps{
		Hub:    hub,
		Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		Open: func(ctx context.Context, id model.MachineID) (*talos.ClusterClient, error) {
			return talos.NewClusterClient(ctx, sim.Dialer(), talos.Target{
				Machine: id,
				Addr:    sim.Host(),
			}, sim.ClientCreds(), talos.Mode{})
		},
		Linger:       50 * time.Millisecond,
		RetryBackoff: 100 * time.Millisecond,
	})
	t.Cleanup(func() { _ = m.Close() })

	return m, hub, sim
}

// collect reads items until one satisfies want, or the deadline passes.
func collect(t *testing.T, ch <-chan streamhub.Item, within time.Duration, want func(nodestream.Message) bool) []nodestream.Message {
	t.Helper()

	deadline := time.After(within)
	var seen []nodestream.Message
	for {
		select {
		case item, ok := <-ch:
			if !ok {
				return seen
			}
			if item.Event == nil {
				continue
			}
			var msg nodestream.Message
			if err := json.Unmarshal(item.Event.Data, &msg); err != nil {
				t.Fatalf("decode message: %v", err)
			}
			seen = append(seen, msg)
			if want(msg) {
				return seen
			}
		case <-deadline:
			t.Fatalf("nothing matching arrived in %s; saw %+v", within, seen)
			return seen
		}
	}
}

// TestTheStreamSaysWhatItIsDoing is STREAM-04: the connection state is an
// event *in* the stream, not something inferred from its absence.
//
// Four causes produce "no lines are arriving" -- the service is quiet, we are
// reconnecting, the node is rebooting, the node is gone -- and they call for
// four different reactions from the person reading. A stream that carries only
// log lines cannot tell them apart.
func TestTheStreamSaysWhatItIsDoing(t *testing.T) {
	t.Parallel()

	m, hub, _ := newManager(t)
	src := nodestream.Source{Machine: machineID, Kind: nodestream.KindDmesg}

	ch, cancel, err := hub.Subscribe(src.Topic(), 0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	release, err := m.Acquire(t.Context(), src)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer release()

	seen := collect(t, ch, 15*time.Second, func(msg nodestream.Message) bool {
		return msg.State == nodestream.StateLive
	})

	if len(seen) == 0 || seen[0].State != nodestream.StateLive {
		t.Fatalf("the first thing on the stream is %+v, want the live state", seen)
	}
}

// TestLinesReachTheHub is criterion 1: the operator sees the output.
func TestLinesReachTheHub(t *testing.T) {
	t.Parallel()

	m, hub, _ := newManager(t)
	src := nodestream.Source{Machine: machineID, Kind: nodestream.KindLogs, Service: "kubelet"}

	ch, cancel, err := hub.Subscribe(src.Topic(), 0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	release, err := m.Acquire(t.Context(), src)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer release()

	seen := collect(t, ch, 15*time.Second, func(msg nodestream.Message) bool {
		return msg.Line != ""
	})

	var lines int
	for _, msg := range seen {
		if msg.Line != "" {
			lines++
		}
	}
	if lines == 0 {
		t.Fatalf("no log lines reached the hub; saw %+v", seen)
	}
}

// TestSeveralPanelsShareOneUpstreamReader is what the reference counting is
// for: four panels on one node's log must be one follow stream against that
// node, not four.
func TestSeveralPanelsShareOneUpstreamReader(t *testing.T) {
	t.Parallel()

	m, _, sim := newManager(t)
	src := nodestream.Source{Machine: machineID, Kind: nodestream.KindLogs, Service: "kubelet"}

	releases := make([]func(), 0, 4)
	for range 4 {
		release, err := m.Acquire(t.Context(), src)
		if err != nil {
			t.Fatalf("Acquire: %v", err)
		}
		releases = append(releases, release)
	}

	if got := m.Active(); got != 1 {
		t.Fatalf("four panels produced %d readers, want 1", got)
	}

	// Give the reader a moment to have opened its stream, then count the
	// arrivals at the node itself. The simulator's counter is the only
	// evidence that is about the node rather than about our bookkeeping.
	deadline := time.Now().Add(10 * time.Second)
	for sim.Calls("Logs") == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if got := sim.Calls("Logs"); got != 1 {
		t.Errorf("the node saw %d Logs calls for four panels, want 1", got)
	}

	for _, release := range releases {
		release()
	}

	// The linger is 50ms in this fixture; the reader must be gone shortly
	// after the last panel closed.
	deadline = time.Now().Add(5 * time.Second)
	for m.Active() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := m.Active(); got != 0 {
		t.Errorf("%d readers still running after every panel closed", got)
	}
}

// TestOneClosedPanelDoesNotStopAnother is the reference count in the other
// direction, and the bug it prevents is one an operator would report as "the
// log randomly stops".
func TestOneClosedPanelDoesNotStopAnother(t *testing.T) {
	t.Parallel()

	m, hub, _ := newManager(t)
	src := nodestream.Source{Machine: machineID, Kind: nodestream.KindDmesg}

	ch, cancel, err := hub.Subscribe(src.Topic(), 0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	first, err := m.Acquire(t.Context(), src)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	second, err := m.Acquire(t.Context(), src)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer second()

	first()
	time.Sleep(300 * time.Millisecond) // well past the fixture's 50ms linger

	if got := m.Active(); got != 1 {
		t.Fatalf("closing one of two panels left %d readers, want the stream still running", got)
	}

	collect(t, ch, 15*time.Second, func(msg nodestream.Message) bool {
		return msg.State == nodestream.StateLive || msg.Line != ""
	})
}
