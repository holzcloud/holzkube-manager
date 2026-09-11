package httpapi_test

import (
	"bufio"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/auth"
	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/nodestream"
	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
	"github.com/holzcloud/holzkube-manager/internal/streamhub"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// The streaming half of phase 5, driven through the real chain.
//
// That last part is the point of these tests rather than an implementation
// detail: phase 2 recorded an entry blocker saying this chain cannot stream
// and that it fails *silently* -- the wrappers buffer, the handler looks fine,
// and the operator sees a panel that never fills. A test that drove the
// handler directly would pass against exactly that.

// sseFrame is one parsed SSE frame.
type sseFrame struct {
	Event string
	ID    string
	Data  string
}

// readFrames reads SSE frames until n have arrived or the deadline passes.
func readFrames(t *testing.T, body *bufio.Reader, n int, within time.Duration) []sseFrame {
	t.Helper()

	done := make(chan []sseFrame, 1)
	go func() {
		var out []sseFrame
		var cur sseFrame
		for len(out) < n {
			line, err := body.ReadString('\n')
			if err != nil {
				break
			}
			line = strings.TrimRight(line, "\n")
			switch {
			case line == "":
				if cur.Event != "" || cur.Data != "" {
					out = append(out, cur)
					cur = sseFrame{}
				}
			case strings.HasPrefix(line, "event: "):
				cur.Event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "id: "):
				cur.ID = strings.TrimPrefix(line, "id: ")
			case strings.HasPrefix(line, "data: "):
				cur.Data = strings.TrimPrefix(line, "data: ")
			}
		}
		done <- out
	}()

	select {
	case out := <-done:
		return out
	case <-time.After(within):
		t.Fatalf("only saw fewer than %d SSE frames in %s", n, within)
		return nil
	}
}

func newStreamHarness(t *testing.T) *inventoryHarness {
	t.Helper()

	cl, err := talossim.NewCluster("homelab", "https://192.168.1.41:6443")
	if err != nil {
		t.Fatalf("NewCluster: %v", err)
	}
	sim, err := talossim.New(talossim.Options{
		Hostname: "cp-1", Cluster: cl, ControlPlane: true,
	})
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}
	t.Cleanup(func() { _ = sim.Close() })

	h := newHarness(t,
		withInventory(func(st *fsstore.Store) *inventory.Service {
			return inventory.New(inventory.Deps{
				Store:  st,
				Dialer: talos.NewDirectDialer(sim.Port()),
				Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
			})
		}),
		withStreaming(),
	)

	resp, raw := h.do(t, http.MethodPost, "/api/v1/setup", map[string]string{
		"username": testUser, "password": testPass,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("setup: %d (%s)", resp.StatusCode, raw)
	}
	resp, raw = h.do(t, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"username": testUser, "password": testPass,
	})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("login: %d (%s)", resp.StatusCode, raw)
	}

	return &inventoryHarness{harness: h, sim: sim, cluster: cl}
}

// openStream starts an SSE request and returns the reader. The caller closes
// the response.
func (h *inventoryHarness) openStream(t *testing.T, query string, lastEventID string) (*http.Response, *bufio.Reader) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.srv.URL+"/api/v1/stream"+query, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("X-Holzkube-Manager-CSRF", "1")
	if lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp, bufio.NewReader(resp.Body)
}

// TestStreamActuallyStreamsThroughTheWholeChain is the entry blocker, closed.
//
// The assertion that matters is not that the frames are correct but that they
// arrive *before the handler returns*. A chain that buffers produces exactly
// the same bytes, in the same order, at the end — which is why the phase-2
// note called the failure silent.
func TestStreamActuallyStreamsThroughTheWholeChain(t *testing.T) {
	h := newStreamHarness(t)

	// The hub is driven directly here rather than through a node: what is
	// under test is the HTTP chain, and a node that has to produce log output
	// on cue would make this a test about the simulator.
	const topic = streamhub.Topic("dmesg:00000000-0000-4000-8000-000000000001")

	resp, body := h.openStream(t, "?topic="+string(topic), "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream: %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type = %q, want text/event-stream", ct)
	}

	// The header arrived while the handler is still running. If the chain
	// buffered, the client would still be blocked on the first read.
	for i := range 3 {
		if _, err := h.hub.Publish(topic, []byte(`{"line":"hello `+string(rune('a'+i))+`"}`)); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}

	frames := readFrames(t, body, 3, 10*time.Second)
	if len(frames) < 3 {
		t.Fatalf("received %d frames, want 3", len(frames))
	}
	for i, f := range frames {
		if f.Event != "message" {
			t.Errorf("frame %d has event %q, want message", i, f.Event)
		}
		if !strings.Contains(f.Data, string(topic)) {
			t.Errorf("frame %d does not name its topic: %s", i, f.Data)
		}
	}
}

// TestOneConnectionCarriesSeveralTopics is the multiplexing claim from
// success criterion 2.
func TestOneConnectionCarriesSeveralTopics(t *testing.T) {
	h := newStreamHarness(t)

	const (
		a = streamhub.Topic("dmesg:00000000-0000-4000-8000-00000000000a")
		b = streamhub.Topic("logs:00000000-0000-4000-8000-00000000000b:kubelet")
	)

	resp, body := h.openStream(t, "?topic="+string(a)+"&topic="+string(b), "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream: %d", resp.StatusCode)
	}

	if _, err := h.hub.Publish(a, []byte(`{"line":"from a"}`)); err != nil {
		t.Fatalf("Publish a: %v", err)
	}
	if _, err := h.hub.Publish(b, []byte(`{"line":"from b"}`)); err != nil {
		t.Fatalf("Publish b: %v", err)
	}

	// Read until both topics have appeared rather than a fixed count. Both
	// topics also carry connection-state events from their readers -- neither
	// machine is in the inventory, so both report that they cannot connect --
	// and a fixed count can be filled entirely by one topic's state chatter
	// before the other's line arrives.
	var frames []sseFrame
	seen := map[string]bool{}
	for range 40 {
		frames = append(frames, readFrames(t, body, 1, 10*time.Second)...)
		seen = topicsIn(t, frames)
		if seen[string(a)] && seen[string(b)] {
			break
		}
	}
	if !seen[string(a)] || !seen[string(b)] {
		t.Fatalf("one connection did not carry both topics: %v", seen)
	}

	// The id carries both positions, because a browser hands back exactly one
	// Last-Event-ID for the connection. A per-topic id would resume one stream
	// and silently truncate the other.
	last := frames[len(frames)-1].ID
	if !strings.Contains(last, string(a)+"=") || !strings.Contains(last, string(b)+"=") {
		t.Fatalf("Last-Event-ID %q does not carry a position for both topics", last)
	}
}

// topicsIn is the set of topics a batch of frames named.
func topicsIn(t *testing.T, frames []sseFrame) map[string]bool {
	t.Helper()

	seen := map[string]bool{}
	for _, f := range frames {
		var env struct {
			Topic string `json:"topic"`
		}
		if err := json.Unmarshal([]byte(f.Data), &env); err != nil {
			t.Fatalf("decode frame: %v (%s)", err, f.Data)
		}
		seen[env.Topic] = true
	}
	return seen
}

// TestReconnectReplaysFromLastEventID is the other half of criterion 2: a
// reconnect must replay rather than leave a hole.
func TestReconnectReplaysFromLastEventID(t *testing.T) {
	h := newStreamHarness(t)

	const topic = streamhub.Topic("dmesg:00000000-0000-4000-8000-00000000000c")

	for i := range 5 {
		if _, err := h.hub.Publish(topic, []byte(`{"line":"`+strings.Repeat("x", i+1)+`"}`)); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}

	// Reconnect claiming to have seen the first two.
	resp, body := h.openStream(t, "?topic="+string(topic), string(topic)+"=2")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream: %d", resp.StatusCode)
	}

	frames := readFrames(t, body, 3, 10*time.Second)
	if len(frames) < 3 {
		t.Fatalf("replay delivered %d frames, want 3 (ids 3,4,5)", len(frames))
	}
	for i, f := range frames {
		var env struct {
			Payload struct {
				Line string `json:"line"`
			} `json:"payload"`
		}
		if err := json.Unmarshal([]byte(f.Data), &env); err != nil {
			t.Fatalf("decode frame %d: %v (%s)", i, err, f.Data)
		}
		if want := strings.Repeat("x", i+3); env.Payload.Line != want {
			t.Errorf("frame %d carries %q, want %q — the replay is off by one", i, env.Payload.Line, want)
		}
	}
}

// TestAStreamRouteIsRefusedIfItIsAlsoDestructive pins the composition-time
// rule: the sudo gate buffers a response, and a held response is not a stream.
func TestAStreamRouteIsRefusedIfItIsAlsoDestructive(t *testing.T) {
	t.Parallel()

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("a route that is both Streaming and Destructive was accepted; " +
				"the sudo gate would buffer it and the stream would never arrive")
		}
		if msg, ok := r.(string); !ok || !strings.Contains(msg, "Streaming and Destructive") {
			t.Errorf("the panic does not say why: %v", r)
		}
	}()

	_ = httpapi.New(httpapi.Deps{
		Auth: mustAuth(t),
		Routes: []httpapi.Route{{
			Method:      http.MethodGet,
			Pattern:     "/api/v1/impossible",
			Streaming:   true,
			Destructive: true,
			Handler:     http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		}},
	})
}

// mustAuth is the minimum httpapi.New dereferences while building the chain.
func mustAuth(t *testing.T) *auth.Service {
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

	au, err := auth.New(st, time.Hour)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	return au
}

// TestUnknownTopicIsRefusedBeforeAnyBytesAreWritten matters because the
// alternative is an error inside a 200 the client has already begun parsing.
func TestUnknownTopicIsRefusedBeforeAnyBytesAreWritten(t *testing.T) {
	h := newStreamHarness(t)

	resp, raw := h.do(t, http.MethodGet, "/api/v1/stream?topic=nonsense", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown topic: %d (%s), want 400", resp.StatusCode, raw)
	}
	p := decodeProblem(t, resp, raw)
	if !strings.HasPrefix(p.Code, "validation.") {
		t.Errorf("code = %q, want a validation code", p.Code)
	}
}

// TestTopicRoundTrip pins the topic vocabulary, which is a contract: a client
// sends these strings back on every reconnect.
func TestTopicRoundTrip(t *testing.T) {
	t.Parallel()

	rows := []nodestream.Source{
		{Machine: "abc", Kind: nodestream.KindDmesg},
		{Machine: "abc", Kind: nodestream.KindLogs, Service: "kubelet"},
	}
	for _, want := range rows {
		got, err := nodestream.ParseTopic(want.Topic())
		if err != nil {
			t.Fatalf("ParseTopic(%q): %v", want.Topic(), err)
		}
		if got != want {
			t.Errorf("round trip of %+v gave %+v", want, got)
		}
	}

	for _, bad := range []string{"", "logs:", "logs:abc", "dmesg:", "what:abc", "logs:abc:kubelet:extra"} {
		if _, err := nodestream.ParseTopic(streamhub.Topic(bad)); err == nil {
			t.Errorf("ParseTopic(%q) accepted a topic that is not one", bad)
		}
	}
}
