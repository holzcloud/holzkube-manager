package fsstore

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dataDir is a data directory the way the daemon keeps it: 0700, so that
// Guard would accept it.
func dataDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	return dir
}

// TestPlaceNew holds the one-slot contract of the host actions (D-02, R2): the
// order appears whole and 0600, a second one is refused with fs.ErrExist and
// leaves the first alone byte for byte, and no temporary stays behind either
// way. The second-placement row is the one that goes red when PlaceNew's
// link(2) becomes a rename(2) -- a rename replaces whatever lies at the path.
func TestPlaceNew(t *testing.T) {
	dir := dataDir(t)
	path := filepath.Join(dir, "host-order")
	first := []byte("reboot 0123456789abcdef\n")

	if err := PlaceNew(path, first); err != nil {
		t.Fatalf("first PlaceNew: %v", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("the placed file is not there: %v", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Errorf("placed file mode = %v, want a regular file with 0600", info.Mode())
	}
	if got, _ := os.ReadFile(path); !bytes.Equal(got, first) {
		t.Errorf("placed content = %q, want %q", got, first)
	}
	if left := tempFilesIn(t, dir); len(left) != 0 {
		t.Errorf("the first placement left temporaries behind: %v", left)
	}

	t.Run("a second placement is refused and changes nothing", func(t *testing.T) {
		err := PlaceNew(path, []byte("poweroff fedcba9876543210\n"))
		if !errors.Is(err, fs.ErrExist) {
			t.Fatalf("second PlaceNew = %v, want an error wrapping fs.ErrExist: a second order must never replace one that waits", err)
		}
		if got, _ := os.ReadFile(path); !bytes.Equal(got, first) {
			t.Errorf("after the refused placement the file holds %q, want the first order %q unchanged", got, first)
		}
		if left := tempFilesIn(t, dir); len(left) != 0 {
			t.Errorf("the refused placement left temporaries behind: %v", left)
		}
	})

	t.Run("the store opens over a placed order", func(t *testing.T) {
		s, err := Open(dir)
		if err != nil {
			t.Fatalf("Open over a data directory holding a placed order: %v", err)
		}
		if err := s.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if got, _ := os.ReadFile(path); !bytes.Equal(got, first) {
			t.Errorf("Open changed the placed order to %q", got)
		}
	})
}

// TestClaim holds the withdrawal primitive (D-13, R8): whoever renames first
// owns the order, so the daemon's withdrawal can never race the root helper
// between a check and a read.
func TestClaim(t *testing.T) {
	t.Run("takes the file out of its name, returns it and leaves nothing", func(t *testing.T) {
		dir := dataDir(t)
		path := filepath.Join(dir, "host-order")
		want := []byte("update 0123456789abcdef\n")
		if err := PlaceNew(path, want); err != nil {
			t.Fatalf("PlaceNew: %v", err)
		}

		got, err := Claim(path, "withdrawn-0123456789abcdef")
		if err != nil {
			t.Fatalf("Claim: %v", err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("Claim returned %q, want %q", got, want)
		}
		if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("the order is still under its name after the claim (Lstat: %v)", err)
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("ReadDir: %v", err)
		}
		if len(entries) != 0 {
			var names []string
			for _, e := range entries {
				names = append(names, e.Name())
			}
			t.Errorf("the claim left %v behind, want an empty directory", names)
		}
	})

	t.Run("an absent file is fs.ErrNotExist", func(t *testing.T) {
		dir := dataDir(t)
		_, err := Claim(filepath.Join(dir, "host-order"), "startup")
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("Claim of an absent file = %v, want an error wrapping fs.ErrNotExist (it means the helper took the order)", err)
		}
	})

	t.Run("a tag outside [a-z0-9-] is refused before anything moves", func(t *testing.T) {
		for _, tag := range []string{"", "../x", "a/b", "UPPER", "sp ace", "dot.dot", strings.Repeat("a", 65)} {
			dir := dataDir(t)
			path := filepath.Join(dir, "host-order")
			if err := PlaceNew(path, []byte("reboot 0123456789abcdef\n")); err != nil {
				t.Fatalf("PlaceNew: %v", err)
			}
			if _, err := Claim(path, tag); err == nil {
				t.Errorf("Claim with tag %q succeeded, want a refusal", tag)
			}
			if _, err := os.Lstat(path); err != nil {
				t.Errorf("Claim with the refused tag %q moved the order: %v", tag, err)
			}
			if left := tempFilesIn(t, dir); len(left) != 0 {
				t.Errorf("Claim with the refused tag %q left %v", tag, left)
			}
		}
	})

	t.Run("a claim left by a crash is swept at the next start", func(t *testing.T) {
		dir := dataDir(t)
		// What a process killed between the rename and the remove leaves.
		leftover := claimPath(filepath.Join(dir, "host-order"), "withdrawn-0123456789abcdef")
		if err := os.WriteFile(leftover, []byte("reboot 0123456789abcdef\n"), 0o600); err != nil {
			t.Fatalf("plant: %v", err)
		}
		s, err := Open(dir)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer s.Close()
		if _, err := os.Lstat(leftover); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Open left the claimed order %s behind (Lstat: %v)", filepath.Base(leftover), err)
		}
	})
}

// TestPlaceNewAfterTheLink is WR-03: once the link has put the order in place,
// it is live -- the helper may already be carrying it out. A failure after it
// (removing the temporary, flushing the directory) must not read as "not
// placed", or the caller reports a failed action, arms no pickup timer, and
// the order lies there unwatched.
//
// Fault injected and seen red: returning the after-link error unmarked.
func TestPlaceNewAfterTheLink(t *testing.T) {
	dir := dataDir(t)
	path := filepath.Join(dir, "host-order")
	want := []byte("reboot 0123456789abcdef\n")
	boom := errors.New("fsync directory: input/output error")

	err := placeNew(path, want, func(string, string) error { return boom })
	if !errors.Is(err, ErrTookEffect) {
		t.Fatalf("placeNew with a failing after-link step = %v, want an error wrapping ErrTookEffect", err)
	}
	if !errors.Is(err, boom) {
		t.Errorf("the error %v does not carry what failed", err)
	}
	if errors.Is(err, fs.ErrExist) {
		t.Errorf("the error %v reads as a taken slot", err)
	}
	if got, readErr := os.ReadFile(path); readErr != nil || !bytes.Equal(got, want) {
		t.Errorf("the order is %q (%v), want it in place", got, readErr)
	}
}

// TestClaimAfterTheRename is WR-03's other half: once the rename has taken the
// file out of its name, the claim owns it. A read or a remove that fails
// afterwards must not read as "nothing was claimed" -- the caller would leave
// its order "pending", and then report as picked up an order nobody picked up.
//
// Fault injected and seen red: returning the after-rename error unmarked.
func TestClaimAfterTheRename(t *testing.T) {
	t.Run("the remove fails", func(t *testing.T) {
		dir := dataDir(t)
		path := filepath.Join(dir, "host-order")
		want := []byte("update 0123456789abcdef\n")
		if err := PlaceNew(path, want); err != nil {
			t.Fatalf("PlaceNew: %v", err)
		}
		boom := errors.New("remove: input/output error")
		got, err := claim(path, "withdrawn-0123456789abcdef", func(string) error { return boom })
		if !errors.Is(err, ErrTookEffect) || !errors.Is(err, boom) {
			t.Fatalf("claim with a failing remove = %v, want an error wrapping ErrTookEffect and the cause", err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("claim returned %q, want what it read, %q", got, want)
		}
		if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("the order is still under its name (Lstat: %v)", err)
		}
	})

	// Only the daemon's own user can plant a link there; the claim takes it
	// out of the name and removes it, and never follows it.
	t.Run("the claimed name is a link", func(t *testing.T) {
		dir := dataDir(t)
		path := filepath.Join(dir, "host-order")
		target := filepath.Join(t.TempDir(), "elsewhere")
		if err := os.WriteFile(target, []byte("keep\n"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := os.Symlink(target, path); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		_, err := Claim(path, "startup")
		if !errors.Is(err, ErrTookEffect) {
			t.Fatalf("Claim of a link = %v, want an error wrapping ErrTookEffect", err)
		}
		if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("the link is still under the order's name (Lstat: %v)", err)
		}
		if left := tempFilesIn(t, dir); len(left) != 0 {
			t.Errorf("the claim left %v", left)
		}
		if got, err := os.ReadFile(target); err != nil || string(got) != "keep\n" {
			t.Errorf("the link's target was touched: %q, %v", got, err)
		}
	})
}
