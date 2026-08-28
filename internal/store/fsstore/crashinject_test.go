package fsstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/holzcloud/holzkube/internal/model"
)

// TestCrashInject drives the atomic write sequence into each of its three
// interruption points and asserts the property that matters after a kill -9:
// the next start reads exactly one whole record, never a mixture, and never a
// leftover temporary as if it were state.
//
//	create temp -> chmod -> [duringTempWrite] write -> fsync -> close
//	  -> [afterTempWrite] rename -> [afterRename] fsync(dir)
//
// Before the rename the old record is still the whole truth; after it, the new
// one is. There is no window in which neither is.
func TestCrashInject(t *testing.T) {
	cases := []struct {
		name  string
		point interruptPoint
		// wantUsername is which of the two records must survive.
		wantUsername string
	}{
		{"crash midway through writing the temporary file", duringTempWrite, "before"},
		{"crash after the temporary file is durable, before the rename", afterTempWrite, "before"},
		{"crash after the rename, before the directory fsync", afterRename, "after"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Chmod(dir, 0o700); err != nil {
				t.Fatalf("chmod: %v", err)
			}
			ctx := context.Background()

			first, err := Open(dir)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			seed, err := first.Users().Put(ctx, model.User{ID: "eve", Username: "before"})
			if err != nil {
				t.Fatalf("seed Put: %v", err)
			}

			setInterrupt(t, tc.point)
			_, err = first.Users().Put(ctx, model.User{ID: "eve", Username: "after", Rev: seed.Rev})
			if !errors.Is(err, errInterrupted) {
				t.Fatalf("interrupted Put err = %v, want errInterrupted", err)
			}
			clearInterrupt()

			// The process is gone; nothing ran a deferred cleanup.
			if err := first.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}

			second, err := Open(dir)
			if err != nil {
				t.Fatalf("Open after the crash: %v", err)
			}
			defer second.Close()

			got, err := second.Users().Get(ctx, "eve")
			if err != nil {
				t.Fatalf("Get after the crash: %v (a crash must not corrupt the record)", err)
			}
			if got.Username != tc.wantUsername {
				t.Fatalf("username = %q, want %q: the surviving record is neither the whole old nor the whole new one",
					got.Username, tc.wantUsername)
			}

			if leftovers := tempFilesIn(t, dir); len(leftovers) != 0 {
				t.Fatalf("Open did not sweep the orphaned temporary: %v", leftovers)
			}

			entries, err := os.ReadDir(filepath.Join(dir, "users"))
			if err != nil {
				t.Fatalf("ReadDir: %v", err)
			}
			if len(entries) != 1 || entries[0].Name() != "eve.json" {
				names := make([]string, 0, len(entries))
				for _, e := range entries {
					names = append(names, e.Name())
				}
				t.Fatalf("users directory holds %v, want exactly eve.json", names)
			}
		})
	}
}

func TestCrashInjectDuringTempWriteDoesNotTouchTheRecord(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	ctx := context.Background()

	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	seed, err := s.Users().Put(ctx, model.User{ID: "eve", Username: "before"})
	if err != nil {
		t.Fatalf("seed Put: %v", err)
	}
	path := filepath.Join(dir, "users", "eve.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	setInterrupt(t, duringTempWrite)
	if _, err := s.Users().Put(ctx, model.User{ID: "eve", Username: "after", Rev: seed.Rev}); !errors.Is(err, errInterrupted) {
		t.Fatalf("err = %v, want errInterrupted", err)
	}
	clearInterrupt()

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("a crash before the rename still changed the record:\nbefore %s\nafter  %s", before, after)
	}
	_ = s.Close()
}
