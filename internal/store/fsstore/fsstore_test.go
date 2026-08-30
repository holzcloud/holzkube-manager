package fsstore

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
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

// openStore prepares a data directory and opens a Store over it. The chmod is
// not incidental: Guard refuses a directory that is not 0700, so a test that
// skips it fails at Open for a reason unrelated to what it is testing.
func openStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// schematicFixture is a record with every field populated, so a round-trip test
// proves the whole shape survives rather than the two fields a minimal fixture
// would carry.
func schematicFixture() model.Schematic {
	return model.Schematic{
		ID:           "376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba",
		Cluster:      "homelab",
		Name:         "workers with intel microcode",
		TalosVersion: "v1.13.9",
		Canonical:    "customization: {}\n",
		Extensions:   []string{"siderolabs/intel-ucode"},
		KernelArgs:   []string{"console=ttyS0"},
		Meta:         []model.MetaValue{{Key: 10, Value: "value"}},
		Usable:       true,
		ProbedAt:     time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC),
		CreatedAt:    time.Date(2026, 8, 29, 11, 0, 0, 0, time.UTC),
	}
}

// TestSchematicPutAssignsTheFirstRevision covers the create half of the CAS
// contract: a record arriving at rev 0 is stored and comes back at rev 1.
func TestSchematicPutAssignsTheFirstRevision(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	stored, err := s.Schematics().Put(ctx, schematicFixture())
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if stored.Rev != 1 {
		t.Errorf("Rev = %d, want 1", stored.Rev)
	}

	got, err := s.Schematics().Get(ctx, stored.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !reflect.DeepEqual(got, stored) {
		t.Errorf("the record came back different:\n got  %+v\n want %+v", got, stored)
	}
}

// TestSchematicPutRefusesAStaleRevision is the lost-update guard. Two writers
// that both read rev 1 must not both succeed, and the error must name both
// revisions so a caller can say what happened rather than only that it failed.
func TestSchematicPutRefusesAStaleRevision(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	stored, err := s.Schematics().Put(ctx, schematicFixture())
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	stale := stored
	stale.Rev = 0
	if _, err := s.Schematics().Put(ctx, stale); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("Put with a stale rev: err = %v, want store.ErrConflict", err)
	} else if !strings.Contains(err.Error(), "1") || !strings.Contains(err.Error(), "0") {
		t.Errorf("the conflict does not name both revisions: %v", err)
	}

	fresh := stored
	fresh.Name = "renamed"
	if _, err := s.Schematics().Put(ctx, fresh); err != nil {
		t.Fatalf("Put with the current rev: %v", err)
	}
}

// TestSchematicPutRefusesARevisionForARecordThatDoesNotExist stops a create
// disguised as an update. Accepting it would silently resurrect a record
// another operator had just deleted.
func TestSchematicPutRefusesARevisionForARecordThatDoesNotExist(t *testing.T) {
	s := openStore(t)

	rec := schematicFixture()
	rec.Rev = 7
	if _, err := s.Schematics().Put(context.Background(), rec); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("err = %v, want store.ErrConflict", err)
	}
}

// TestSchematicGetDistinguishesMissingFromUnaddressable keeps "no such record"
// and "that is not a key" apart. A 404 and a 400 are different answers.
func TestSchematicGetDistinguishesMissingFromUnaddressable(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	if _, err := s.Schematics().Get(ctx, "0000000000000000000000000000000000000000000000000000000000000000"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("unknown id: err = %v, want store.ErrNotFound", err)
	}
	for _, bad := range []model.SchematicID{"", "../../etc/passwd", "has spaces"} {
		if _, err := s.Schematics().Get(ctx, bad); !errors.Is(err, store.ErrInvalidKey) {
			t.Errorf("Get(%q): err = %v, want store.ErrInvalidKey", bad, err)
		}
	}
	if err := s.Schematics().Delete(ctx, "../escape"); !errors.Is(err, store.ErrInvalidKey) {
		t.Errorf("Delete with an unaddressable key: err = %v, want store.ErrInvalidKey", err)
	}
}

// TestSchematicRoundTripsByteIdentically means "the file changed" continues to
// mean "the record changed", which is what makes a pre-migration backup diff
// readable and a crash-injection test meaningful.
func TestSchematicRoundTripsByteIdentically(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	stored, err := s.Schematics().Put(ctx, schematicFixture())
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	path := filepath.Join(s.Dir(), "schematics", string(stored.ID)+".json")

	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read record: %v", err)
	}

	// The same record put again at its current revision: the only change is
	// the revision, so re-reading and re-writing must be reproducible.
	again, err := s.Schematics().Put(ctx, stored)
	if err != nil {
		t.Fatalf("second Put: %v", err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("re-read record: %v", err)
	}
	if again.Rev != 2 {
		t.Errorf("Rev = %d after a second Put, want 2", again.Rev)
	}

	stored.Rev = again.Rev
	third, err := marshalRecord(stored)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Equal(second, third) {
		t.Errorf("serialising the same record twice produced different bytes:\n%s\n%s", second, third)
	}
	if bytes.Equal(first, second) {
		t.Error("a second Put did not change the file at all, so the revision was not written")
	}
}

// TestSchematicSurvivesARestart is the FACT-06 persistence half: the record,
// its revision, its Factory-canonical document and its usable verdict are all
// still there after the process that wrote them is gone.
func TestSchematicSurvivesARestart(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	first, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	stored, err := first.Schematics().Put(context.Background(), schematicFixture())
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	second, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })

	got, err := second.Schematics().Get(context.Background(), stored.ID)
	if err != nil {
		t.Fatalf("Get after restart: %v", err)
	}
	if !reflect.DeepEqual(got, stored) {
		t.Fatalf("the record changed across a restart:\n got  %+v\n want %+v", got, stored)
	}
	if !got.Usable || got.Canonical == "" || got.Rev != stored.Rev {
		t.Errorf("usable=%v canonical=%q rev=%d: the verdict, the document or the revision did not survive",
			got.Usable, got.Canonical, got.Rev)
	}

	list, err := second.Schematics().List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List returned %d records, want 1", len(list))
	}
}

// TestSchematicListIsEmptyAndNotNil keeps an empty store encodable as [].
func TestSchematicListIsEmptyAndNotNil(t *testing.T) {
	list, err := openStore(t).Schematics().List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list == nil {
		t.Fatal("List returned nil for an empty store")
	}
	if len(list) != 0 {
		t.Fatalf("an empty store listed %d records", len(list))
	}
}

// TestSchematicDeleteReportsAMissingRecord keeps delete honest: removing
// something that was not there is not a success.
func TestSchematicDeleteReportsAMissingRecord(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	stored, err := s.Schematics().Put(ctx, schematicFixture())
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := s.Schematics().Delete(ctx, stored.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := s.Schematics().Delete(ctx, stored.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("second Delete: err = %v, want store.ErrNotFound", err)
	}
}
