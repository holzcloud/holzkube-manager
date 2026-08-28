package fsstore

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/holzcloud/holzkube/internal/model"
)

// TestOpenReapsExpiredSessions covers the only growth path the store had.
//
// scs runs no background cleanup for a user-supplied Store, and the only
// deletion paths are Find and Delete. A session nobody ever looks up again --
// the normal case, because the token left with the browser -- stayed on disk
// forever, and every login added one.
func TestOpenReapsExpiredSessions(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	ctx := context.Background()
	live := model.Session{ID: "live0000", Data: []byte(`{}`), ExpiresAt: time.Now().Add(time.Hour)}
	stale := model.Session{ID: "stale000", Data: []byte(`{}`), ExpiresAt: time.Now().Add(-time.Hour)}
	for _, rec := range []model.Session{live, stale} {
		if _, err := s.Sessions().Put(ctx, rec); err != nil {
			t.Fatalf("put %s: %v", rec.ID, err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// The next start is where the sweep happens: single writer, nothing
	// serving yet.
	reopened, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })

	recs, err := reopened.Sessions().List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("sessions = %d, want only the live one", len(recs))
	}
	if recs[0].ID != live.ID {
		t.Errorf("surviving session = %q, want the unexpired %q", recs[0].ID, live.ID)
	}
}
