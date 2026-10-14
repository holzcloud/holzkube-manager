package fsstore

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
)

// configBaselineFixture is a record with every field populated, so a round trip
// proves the whole shape survives, including the two times that drift tracking
// needs across a restart and the pointer to the expected configuration.
func configBaselineFixture() model.ConfigBaseline {
	return model.ConfigBaseline{
		Machine:          "6f1c0a52-3b7e-4d1a-9a55-0e5c1f2d7a10",
		Format:           model.BaselineFormat,
		Cluster:          "homelab",
		Snapshot:         "---\nv1alpha1:\n  machine:\n    type: worker\n",
		Digest:           "3f2a0c8e5d41b7a96c0d2e8f1a4b6c7d9e0f1a2b3c4d5e6f708192a3b4c5d6e7",
		TakenAt:          time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC),
		Source:           model.BaselineFirstSight,
		Machinery:        "v1.14.0",
		DriftSince:       time.Date(2026, 10, 14, 10, 30, 0, 0, time.UTC),
		AppliedOnDriftAt: time.Date(2026, 10, 14, 11, 0, 0, 0, time.UTC),
		Expected: &model.ExpectedConfig{
			Digest: "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f9",
			Mode:   "staged",
			At:     time.Date(2026, 10, 14, 11, 0, 0, 0, time.UTC),
		},
	}
}

// TestAConfigBaselineRoundTripsAndSurvivesARestart is the tracer: a baseline
// goes through the real store onto the real disk and is still whole when a new
// process opens the same directory. DriftSince in particular is only worth
// having if it survives, because a banner that counts 24 hours from the last
// restart never fires.
func TestAConfigBaselineRoundTripsAndSurvivesARestart(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	ctx := context.Background()

	first, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	stored, err := first.ConfigBaselines().Put(ctx, configBaselineFixture())
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if stored.Rev != 1 {
		t.Fatalf("first Put returned rev %d, want 1", stored.Rev)
	}
	got, err := first.ConfigBaselines().Get(ctx, stored.Machine)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !reflect.DeepEqual(got, stored) {
		t.Fatalf("Get differs from what Put returned:\n got  %+v\n want %+v", got, stored)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	second, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })

	after, err := second.ConfigBaselines().Get(ctx, stored.Machine)
	if err != nil {
		t.Fatalf("Get after restart: %v", err)
	}
	if !reflect.DeepEqual(after, stored) {
		t.Fatalf("the baseline changed across a restart:\n got  %+v\n want %+v", after, stored)
	}
	if after.Expected == nil || after.Expected.Digest != stored.Expected.Digest {
		t.Errorf("Expected = %+v, want the pointer to survive with its digest", after.Expected)
	}
	if after.DriftSince.IsZero() || after.AppliedOnDriftAt.IsZero() {
		t.Errorf("DriftSince=%v AppliedOnDriftAt=%v: a time did not survive the restart",
			after.DriftSince, after.AppliedOnDriftAt)
	}

	list, err := second.ConfigBaselines().List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || !reflect.DeepEqual(list[0], stored) {
		t.Fatalf("List = %+v, want exactly the stored baseline", list)
	}
}

// TestAConfigBaselinePutRefusesAStaleRevision is the lost-update guard: the
// drift check and an apply may both write the same node's baseline.
func TestAConfigBaselinePutRefusesAStaleRevision(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	first, err := s.ConfigBaselines().Put(ctx, configBaselineFixture())
	if err != nil {
		t.Fatalf("first Put: %v", err)
	}
	second, err := s.ConfigBaselines().Put(ctx, first)
	if err != nil {
		t.Fatalf("second Put: %v", err)
	}
	if second.Rev != 2 {
		t.Fatalf("second Put returned rev %d, want 2", second.Rev)
	}

	stale := first // still carries rev 1
	if _, err := s.ConfigBaselines().Put(ctx, stale); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("Put with a stale revision: err = %v, want store.ErrConflict", err)
	}
}

// TestAConfigBaselinePutRefusesARevisionForARecordThatDoesNotExist stops a
// create disguised as an update, which would resurrect a deleted baseline.
func TestAConfigBaselinePutRefusesARevisionForARecordThatDoesNotExist(t *testing.T) {
	s := openStore(t)

	rec := configBaselineFixture()
	rec.Rev = 7
	if _, err := s.ConfigBaselines().Put(context.Background(), rec); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("err = %v, want store.ErrConflict", err)
	}
}

// TestADamagedBaselineIsNotAMissingOne is the sentence the drift check rests
// on: a baseline that cannot be read is reported and never treated as a first
// sight, because a first sight would overwrite it and the drift it recorded
// would be lost without a word.
func TestADamagedBaselineIsNotAMissingOne(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	ctx := context.Background()

	const id = model.MachineID("6f1c0a52-3b7e-4d1a-9a55-0e5c1f2d7a10")

	if _, err := s.ConfigBaselines().Get(ctx, id); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("a baseline that was never written: err = %v, want store.ErrNotFound", err)
	}

	path := filepath.Join(dir, "config-baselines", string(id)+".json")
	damaged := []byte("{\"machine\": \"6f1c0a52\", \"snapshot\": \"half a wri")
	if err := os.WriteFile(path, damaged, 0o600); err != nil {
		t.Fatalf("plant the damaged file: %v", err)
	}

	_, err = s.ConfigBaselines().Get(ctx, id)
	if err == nil {
		t.Fatal("Get of a damaged baseline returned no error")
	}
	if errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Get of a damaged baseline said it was missing: %v", err)
	}

	rec := configBaselineFixture()
	rec.Machine = id
	if _, err := s.ConfigBaselines().Put(ctx, rec); err == nil {
		t.Fatal("Put over a damaged baseline returned no error")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the file back: %v", err)
	}
	if !bytes.Equal(after, damaged) {
		t.Fatalf("Put overwrote the damaged file:\n got  %q\n want %q", after, damaged)
	}
}

// TestAConfigBaselineFileIsPrivate: the snapshot is redacted, but the raw
// fields Redact does not know are in it (D-14), so the file is as private as
// every other record of the data directory.
func TestAConfigBaselineFileIsPrivate(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	stored, err := s.ConfigBaselines().Put(context.Background(), configBaselineFixture())
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	sub := filepath.Join(dir, "config-baselines")
	dirInfo, err := os.Stat(sub)
	if err != nil {
		t.Fatalf("stat the directory: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Errorf("config-baselines is %v, want 0700", got)
	}
	fileInfo, err := os.Stat(filepath.Join(sub, string(stored.Machine)+".json"))
	if err != nil {
		t.Fatalf("stat the file: %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Errorf("the baseline file is %v, want 0600", got)
	}
}

// TestAConfigBaselineKeyCannotEscapeItsDirectory: the key is a machine id from
// the inventory, but the store does not trust that.
func TestAConfigBaselineKeyCannotEscapeItsDirectory(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	for _, bad := range []model.MachineID{"../x", ""} {
		if _, err := s.ConfigBaselines().Get(ctx, bad); !errors.Is(err, store.ErrInvalidKey) {
			t.Errorf("Get(%q): err = %v, want store.ErrInvalidKey", bad, err)
		}
		rec := configBaselineFixture()
		rec.Machine = bad
		if _, err := s.ConfigBaselines().Put(ctx, rec); !errors.Is(err, store.ErrInvalidKey) {
			t.Errorf("Put(%q): err = %v, want store.ErrInvalidKey", bad, err)
		}
	}
}

// TestDeletingAMissingConfigBaselineSaysSo: removing something that was not
// there is not a success.
func TestDeletingAMissingConfigBaselineSaysSo(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	stored, err := s.ConfigBaselines().Put(ctx, configBaselineFixture())
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := s.ConfigBaselines().Delete(ctx, stored.Machine); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := s.ConfigBaselines().Delete(ctx, stored.Machine); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("second Delete: err = %v, want store.ErrNotFound", err)
	}
}

// TestOpenCreatesTheConfigBaselinesDirectory: a fresh data directory has
// config-baselines/ as soon as it is opened, before anything is written. Put
// would create it on demand, so only looking at the disk after Open can tell
// whether Open did its part, and List on an empty store must be an empty list
// and not an error.
func TestOpenCreatesTheConfigBaselinesDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	info, err := os.Stat(filepath.Join(dir, "config-baselines"))
	if err != nil {
		t.Fatalf("Open did not create config-baselines: %v", err)
	}
	if !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Errorf("config-baselines is %v, want a directory with mode 0700", info.Mode())
	}
	list, err := s.ConfigBaselines().List(context.Background())
	if err != nil || len(list) != 0 {
		t.Errorf("List on an empty store = %v, %v, want an empty list", list, err)
	}
}
