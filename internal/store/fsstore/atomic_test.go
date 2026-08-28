package fsstore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube/internal/store"
)

// tempFilesIn reports every leftover temporary record under dir.
func tempFilesIn(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasPrefix(d.Name(), store.TempFilePrefix) {
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	return out
}

func TestWriteAtomicLeavesNoTemporaryOnSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "record.json")

	if err := writeAtomic(path, []byte(`{"a":1}`)); err != nil {
		t.Fatalf("writeAtomic: %v", err)
	}
	if leftovers := tempFilesIn(t, dir); len(leftovers) != 0 {
		t.Fatalf("temporary files survived a successful write: %v", leftovers)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("record mode is %04o, want 0600", info.Mode().Perm())
	}
}

func TestWriteAtomicKeepsTheTemporaryInTheTargetDirectory(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "users")
	path := filepath.Join(sub, "a.json")

	setInterrupt(t, afterTempWrite)
	err := writeAtomic(path, []byte(`{"a":1}`))
	if !errors.Is(err, errInterrupted) {
		t.Fatalf("writeAtomic err = %v, want errInterrupted", err)
	}

	leftovers := tempFilesIn(t, dir)
	if len(leftovers) != 1 {
		t.Fatalf("leftovers = %v, want exactly the one interrupted temporary", leftovers)
	}
	if got := filepath.Dir(leftovers[0]); got != sub {
		t.Fatalf("temporary is in %q, want %q: rename is only atomic within one directory", got, sub)
	}

	info, err := os.Stat(leftovers[0])
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("temporary mode is %04o, want 0600: chmod must precede the write, not follow the rename",
			info.Mode().Perm())
	}
}

func TestOpenSweepsOrphanedTemporaries(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "users"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	orphans := []string{
		filepath.Join(dir, store.TempFilePrefix+"root"),
		filepath.Join(dir, "users", store.TempFilePrefix+"nested"),
	}
	for _, p := range orphans {
		if err := os.WriteFile(p, []byte("half a record"), 0o600); err != nil {
			t.Fatalf("plant %s: %v", p, err)
		}
	}
	keep := filepath.Join(dir, "users", "real.json")
	if err := os.WriteFile(keep, []byte(`{"id":"real"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	if leftovers := tempFilesIn(t, dir); len(leftovers) != 0 {
		t.Fatalf("Open left orphaned temporaries behind: %v", leftovers)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("Open removed a real record: %v", err)
	}
}
