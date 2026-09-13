package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/nodestream"
	"github.com/holzcloud/holzkube-manager/internal/streamhub"
)

// One SSE connection per browser tab, carrying every panel that tab has open.
//
// A connection per panel would be simpler to write and wrong in a way that
// only shows up in use: browsers cap concurrent connections per origin at six,
// and a node detail page with kubelet, etcd, apid and dmesg open is already
// four of them. The multiplexing is what keeps the seventh panel from silently
// never connecting.
//
// Resumption is the part worth reading carefully. SSE gives the client exactly
// one string to hand back -- `Last-Event-ID` -- and this connection carries
// several topics at different positions. So the id of every event is the whole
// cursor set, `topic=id,topic=id`, and the browser's own automatic reconnect
// therefore replays each topic from where that topic actually stood. A
// per-topic id would resume one stream correctly and silently truncate the
// rest.

// streamKeepAlive is how often a comment frame is written to an idle stream.
//
// It is not for the browser, which is content to wait: it is for everything
// between, which is not. A proxy with a sixty-second idle timeout will drop a
// quiet log stream, and the operator sees a panel that stopped for no reason.
const streamKeepAlive = 20 * time.Second

// maxStreamTopics caps how many topics one connection may carry.
//
// Each is an upstream follow stream against a node, so this is a limit on what
// one browser tab can ask a cluster to do. Twelve is more panels than a screen
// holds and far fewer than a node notices.
const maxStreamTopics = 12

// StreamRoutes serves the one multiplexed event stream.
//
// It is Streaming and therefore, by the rule in httpapi.New, cannot be
// Destructive: the sudo gate holds a response back until the window has been
// refreshed, and a held response is not a stream.
func StreamRoutes(d httpapi.Deps) []httpapi.Route {
	return []httpapi.Route{
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/stream",
			RequiresSession: true,
			MinRole:         model.RoleReader,
			Streaming:       true,
			Handler:         handler(streamEvents(d)),
		},
	}
}

func streamEvents(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Hub == nil || d.NodeStreams == nil {
			httpapi.WriteProblem(w, r, httpapi.Upstream("upstream.streaming-unavailable",
				"This instance was started without streaming."))
			return
		}

		topics, problem := requestedTopics(r)
		if problem != nil {
			httpapi.WriteProblem(w, r, problem)
			return
		}

		// The two halves of the entry blocker this phase exists to lift.
		//
		// SetWriteDeadline clears the process-wide WriteTimeout for this
		// connection only. That timeout is sized for argon2id and the login
		// rate limiter and has nothing to say about a stream; left in place it
		// kills every stream at the same age, which looks like a bug in the
		// log viewer.
		//
		// Flush is what makes the bytes leave. Both reach the real connection
		// through the Unwrap chain the middleware wrappers now implement --
		// before that, this handler would have buffered silently.
		rc := http.NewResponseController(w)
		if err := rc.SetWriteDeadline(time.Time{}); err != nil {
			httpapi.WriteInternal(w, r, d.Logger, fmt.Errorf("streaming is not supported on this connection: %w", err))
			return
		}

		cursors := parseCursor(r.Header.Get("Last-Event-ID"))

		// Subscribe to everything before writing a byte, so that a bad topic
		// is a problem response rather than an error inside a 200 the client
		// has already started parsing.
		type feed struct {
			topic streamhub.Topic
			items <-chan streamhub.Item
		}
		feeds := make([]feed, 0, len(topics))

		for _, src := range topics {
			topic := src.Topic()

			release, err := d.NodeStreams.Acquire(r.Context(), src)
			if err != nil {
				httpapi.WriteInternal(w, r, d.Logger, err)
				return
			}
			defer release()

			items, cancel, err := d.Hub.Subscribe(topic, cursors[topic])
			if err != nil {
				httpapi.WriteInternal(w, r, d.Logger, err)
				return
			}
			defer cancel()

			feeds = append(feeds, feed{topic: topic, items: items})
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Connection", "keep-alive")
		// Without this, a reverse proxy that buffers will hold the whole
		// stream and deliver it when it ends -- which for a follow stream is
		// never.
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		if err := rc.Flush(); err != nil {
			return
		}

		// One goroutine-free merge: a select over a slice needs reflect, so
		// each feed gets a forwarding goroutine into one channel. They end
		// when their subscription channel closes, which cancel() above
		// guarantees on the way out.
		merged := make(chan taggedItem, len(feeds)*4+8)
		for _, f := range feeds {
			go func(f feed) {
				for item := range f.items {
					merged <- taggedItem{topic: f.topic, item: item}
				}
			}(f)
		}

		keepAlive := time.NewTicker(streamKeepAlive)
		defer keepAlive.Stop()

		for {
			select {
			case <-r.Context().Done():
				return

			case <-keepAlive.C:
				if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
					return
				}
				if err := rc.Flush(); err != nil {
					return
				}

			case ti := <-merged:
				switch {
				case ti.item.Event != nil:
					cursors[ti.topic] = ti.item.Event.ID
					if err := writeEvent(w, "message", ti.topic, cursors, ti.item.Event.Data); err != nil {
						return
					}
				case ti.item.Gap != nil:
					// A visible break, not a silent omission. An operator
					// reading a log to diagnose an outage and not being told
					// that lines are missing draws confident wrong
					// conclusions.
					payload := fmt.Sprintf(`{"missed":%d,"through":%d}`,
						ti.item.Gap.Missed, ti.item.Gap.Through)
					if ti.item.Gap.Through > cursors[ti.topic] {
						cursors[ti.topic] = ti.item.Gap.Through
					}
					if err := writeEvent(w, "gap", ti.topic, cursors, []byte(payload)); err != nil {
						return
					}
				}
				if err := rc.Flush(); err != nil {
					return
				}
			}
		}
	}
}

type taggedItem struct {
	topic streamhub.Topic
	item  streamhub.Item
}

// writeEvent writes one SSE frame.
//
// The id is the whole cursor set rather than this topic's number, because a
// browser hands back exactly one Last-Event-ID for the connection. See the
// note at the top of this file.
func writeEvent(w http.ResponseWriter, event string, topic streamhub.Topic, cursors map[streamhub.Topic]uint64, data []byte) error {
	var b strings.Builder
	fmt.Fprintf(&b, "event: %s\n", event)
	fmt.Fprintf(&b, "id: %s\n", formatCursor(cursors))
	// The topic rides in the payload rather than in the event name, so that a
	// client adds a panel without having to add an addEventListener.
	fmt.Fprintf(&b, "data: {\"topic\":%q,\"payload\":%s}\n\n", topic, data)

	_, err := w.Write([]byte(b.String()))
	return err
}

// formatCursor renders the per-topic positions as one Last-Event-ID string.
func formatCursor(cursors map[streamhub.Topic]uint64) string {
	parts := make([]string, 0, len(cursors))
	for topic, id := range cursors {
		parts = append(parts, fmt.Sprintf("%s=%d", topic, id))
	}
	// Sorted, so that the same set of positions always produces the same
	// string. An id that varies with map iteration order is an id that looks
	// like it changed when it did not.
	sortStrings(parts)
	return strings.Join(parts, ",")
}

// parseCursor reads a Last-Event-ID back into per-topic positions.
//
// It is parsing a header the client controls, so an entry that does not make
// sense is skipped rather than refused: a malformed cursor should cost the
// client a replay from the start of the buffer, not the whole connection.
func parseCursor(header string) map[streamhub.Topic]uint64 {
	out := map[streamhub.Topic]uint64{}
	if header == "" {
		return out
	}
	for _, part := range strings.Split(header, ",") {
		name, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		id, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			continue
		}
		out[streamhub.Topic(name)] = id
	}
	return out
}

// requestedTopics reads and validates the topics a client asked for.
func requestedTopics(r *http.Request) ([]nodestream.Source, *httpapi.Problem) {
	raw := r.URL.Query()["topic"]
	if len(raw) == 0 {
		return nil, httpapi.Validation("Name at least one topic to stream.",
			httpapi.FieldError{Field: "topic", Reason: "required"})
	}
	if len(raw) > maxStreamTopics {
		return nil, httpapi.Validation(
			fmt.Sprintf("One connection may carry at most %d topics; this asked for %d. "+
				"Each topic is a follow stream against a node.", maxStreamTopics, len(raw)),
			httpapi.FieldError{Field: "topic", Reason: "too many"})
	}

	seen := map[streamhub.Topic]bool{}
	out := make([]nodestream.Source, 0, len(raw))
	for _, t := range raw {
		src, err := nodestream.ParseTopic(streamhub.Topic(t))
		if err != nil {
			return nil, httpapi.Validation(err.Error(),
				httpapi.FieldError{Field: "topic", Reason: "unrecognised"})
		}
		// A duplicate would take two references on one reader and two
		// subscriptions on one topic, and would deliver every line twice.
		if seen[src.Topic()] {
			continue
		}
		seen[src.Topic()] = true
		out = append(out, src)
	}
	return out, nil
}

// sortStrings is an insertion sort over a handful of entries. It is here
// rather than as a slices.Sort call to keep the import list of this file to
// what it is about; the slice is at most maxStreamTopics long.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
