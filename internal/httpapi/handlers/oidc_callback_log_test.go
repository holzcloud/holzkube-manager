package handlers_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// lockedBuffer is a journal a handler goroutine writes and the test reads.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func withJournal(j *lockedBuffer) depOption {
	return func(d *httpapi.Deps) {
		d.Logger = slog.New(slog.NewTextHandler(j, &slog.HandlerOptions{Level: slog.LevelWarn}))
	}
}

// TestACallbackRefusedBeforeTheExchangeSaysWhichCheckItWas is the guard for
// ledger 138. On the operator's Pi a sudo round trip came back, answered in 32
// microseconds with a redirect to /login, and wrote nothing: the journal could
// rule out the token exchange and the three named sudo refusals and could not
// say which of no-flow, state-mismatch and no-code had fired.
//
// It asserts the code, whether the caller still had a session -- which is what
// separates a lost flow from a lost cookie -- and that neither single-use value
// from the query string reached the journal.
func TestACallbackRefusedBeforeTheExchangeSaysWhichCheckItWas(t *testing.T) {
	t.Parallel()

	const state, code = "state-value-that-must-not-be-logged", "code-value-that-must-not-be-logged"

	for _, tc := range []struct {
		name     string
		signedIn bool
	}{
		{"with a live session, the flow alone was lost", true},
		{"with no session at all", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			journal := &lockedBuffer{}
			s := newServerWith(t, 5*time.Minute, nil, talos.Mode{}, withProvider(t), withJournal(journal))

			c := s.newClient(t)
			if tc.signedIn {
				c.setup()
			} else {
				s.newClient(t).setup()
			}

			resp, _ := c.do(http.MethodGet, "/api/v1/auth/oidc/callback?state="+state+"&code="+code, nil)
			if resp.StatusCode != http.StatusFound {
				t.Fatalf("callback = %d, want 302", resp.StatusCode)
			}
			if got := resp.Header.Get("Location"); got != "/login?sso_error=no-flow" {
				t.Fatalf("Location = %q, want the no-flow refusal", got)
			}

			log := journal.String()
			if !strings.Contains(log, "refused before the token exchange") || !strings.Contains(log, "code=no-flow") {
				t.Fatalf("the refusal left no line naming its check -- the Pi's journal, where a "+
					"32-microsecond redirect to /login explained nothing. journal:\n%s", log)
			}
			want := "signed_in=false"
			if tc.signedIn {
				want = "signed_in=true"
			}
			if !strings.Contains(log, want) {
				t.Errorf("journal does not say %s:\n%s", want, log)
			}
			if !strings.Contains(log, "state_present=true") || !strings.Contains(log, "code_present=true") {
				t.Errorf("journal does not say which parameters arrived:\n%s", log)
			}
			if strings.Contains(log, state) || strings.Contains(log, code) {
				t.Errorf("a single-use value from the query string reached the journal:\n%s", log)
			}
		})
	}
}
