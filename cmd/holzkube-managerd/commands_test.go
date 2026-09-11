package main

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/store/migrate/backup"
)

// The operational subcommands (OPS-01, OPS-02).
//
// What these are about is the two ways a restore destroys something: writing
// outside the directory it was aimed at, and replacing a working installation
// with nothing to go back to.

func dataDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	// The store's permission guard refuses a directory group or other can
	// read, so the fixture has to be as tight as a real data directory is.
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	return dir
}

func write(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestABackupRestoresToWhatItCaptured is the round trip, and the control for
// everything below it.
func TestABackupRestoresToWhatItCaptured(t *testing.T) {
	t.Parallel()

	dir := dataDir(t)
	write(t, filepath.Join(dir, "VERSION"), "5")
	write(t, filepath.Join(dir, "clusters", "c1.json"), `{"id":"c1"}`)
	write(t, filepath.Join(dir, "cluster-secrets", "c1.json"), `{"os_ca_key":"secret"}`)

	archive, err := backup.CreateNamed(dir, "test")
	if err != nil {
		t.Fatalf("CreateNamed: %v", err)
	}

	// Everything gone, then restored.
	for _, name := range []string{"VERSION", "clusters", "cluster-secrets"} {
		if err := os.RemoveAll(filepath.Join(dir, name)); err != nil {
			t.Fatalf("remove %s: %v", name, err)
		}
	}

	if _, err := backup.Restore(archive, dir, ""); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	for path, want := range map[string]string{
		filepath.Join(dir, "VERSION"):                    "5",
		filepath.Join(dir, "clusters", "c1.json"):        `{"id":"c1"}`,
		filepath.Join(dir, "cluster-secrets", "c1.json"): `{"os_ca_key":"secret"}`,
	} {
		got, err := os.ReadFile(path) //nolint:gosec // the test owns these paths
		if err != nil {
			t.Errorf("read %s: %v", path, err)
			continue
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", path, got, want)
		}
	}
}

// TestARestoredFileIsNotReadableByAnybodyElse.
//
// A restore that produced a 0644 would produce a data directory fsstore then
// refuses to open -- which is a restore that appeared to work.
func TestARestoredFileIsNotReadableByAnybodyElse(t *testing.T) {
	t.Parallel()

	dir := dataDir(t)
	write(t, filepath.Join(dir, "cluster-secrets", "c1.json"), "secret")

	archive, err := backup.CreateNamed(dir, "test")
	if err != nil {
		t.Fatalf("CreateNamed: %v", err)
	}

	target := dataDir(t)
	if _, err := backup.Restore(archive, target, ""); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	info, err := os.Stat(filepath.Join(target, "cluster-secrets", "c1.json"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf("a restored secret is mode %04o; group and other must have nothing", perm)
	}
}

// TestARestoreRefusesToWriteOutsideTheDataDirectory is the one that matters.
//
// A path in a tarball is whatever the person who made it wrote, and
// `../../../etc/shadow` is a valid tar entry. The check is on the *resolved*
// path rather than on the text, because `a/../../b` contains no leading `..`
// and still escapes.
func TestARestoreRefusesToWriteOutsideTheDataDirectory(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"../escaped.txt",
		"a/../../escaped.txt",
		"/etc/shadow",
	} {
		archive := filepath.Join(t.TempDir(), "evil.tar.gz")
		writeEvilArchive(t, archive, name)

		dir := dataDir(t)
		_, err := backup.Restore(archive, dir, "")
		if !errors.Is(err, backup.ErrOutsideDestination) {
			t.Errorf("an archive containing %q was accepted: %v", name, err)
		}
	}
}

// TestARestoreBacksUpWhatWasThere.
//
// A restore that went wrong without one would have replaced a working
// installation with a broken one and left nothing to go back to.
func TestARestoreBacksUpWhatWasThere(t *testing.T) {
	t.Parallel()

	source := dataDir(t)
	write(t, filepath.Join(source, "VERSION"), "5")
	archive, err := backup.CreateNamed(source, "test")
	if err != nil {
		t.Fatalf("CreateNamed: %v", err)
	}

	// A different directory, with something valuable in it.
	target := dataDir(t)
	write(t, filepath.Join(target, "VERSION"), "4")
	write(t, filepath.Join(target, "irreplaceable.json"), "the only copy")

	safety, err := backup.Restore(archive, target, "pre-restore")
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if safety == "" {
		t.Fatal("the restore wrote nothing to go back to")
	}
	if _, err := os.Stat(safety); err != nil {
		t.Fatalf("the safety backup is not on disk: %v", err)
	}

	// The restore happened...
	got, err := os.ReadFile(filepath.Join(target, "VERSION")) //nolint:gosec // the test owns this path
	if err != nil || string(got) != "5" {
		t.Fatalf("VERSION = %q (%v), want 5", got, err)
	}

	// ...and the thing that was only in the target is recoverable from the
	// safety backup.
	recovered := dataDir(t)
	if _, err := backup.Restore(safety, recovered, ""); err != nil {
		t.Fatalf("restoring the safety backup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(recovered, "irreplaceable.json")); err != nil {
		t.Errorf("the safety backup does not contain what was only in the target: %v", err)
	}
}

// TestARestoreRefusesASymlinkRatherThanSkippingIt.
//
// Silently dropping one produces a restored directory that is missing
// something nobody is told about.
func TestARestoreRefusesASymlinkRatherThanSkippingIt(t *testing.T) {
	t.Parallel()

	archive := filepath.Join(t.TempDir(), "link.tar.gz")
	writeSymlinkArchive(t, archive)

	_, err := backup.Restore(archive, dataDir(t), "")
	if err == nil {
		t.Fatal("an archive containing a symlink was restored")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("the refusal %q does not say what was in the archive", err)
	}
}

// TestABackupNeverOverwritesABackup.
//
// The names carry a timestamp precisely so that never has to happen, and
// O_EXCL is what makes it true rather than likely.
func TestABackupNeverOverwritesABackup(t *testing.T) {
	t.Parallel()

	dir := dataDir(t)
	write(t, filepath.Join(dir, "VERSION"), "5")

	first, err := backup.CreateNamed(dir, "test")
	if err != nil {
		t.Fatalf("CreateNamed: %v", err)
	}

	// The same second, so the names collide if they carry only a timestamp.
	// One of the two has to fail rather than one silently replacing the other.
	second, err := backup.CreateNamed(dir, "test")
	if err != nil {
		if !strings.Contains(err.Error(), "exists") && !os.IsExist(err) {
			t.Fatalf("the second backup failed for a reason other than the name being taken: %v", err)
		}
		return
	}
	if second == first {
		t.Fatal("two backups were written to the same path")
	}
}

// TestBackupsAreListedNewestFirst, which is the order somebody restoring reads
// them in.
func TestBackupsAreListedNewestFirst(t *testing.T) {
	t.Parallel()

	dir := dataDir(t)
	write(t, filepath.Join(dir, "VERSION"), "5")

	if _, err := backup.CreateNamed(dir, "older"); err != nil {
		t.Fatalf("CreateNamed: %v", err)
	}
	// The modification times are what the ordering reads, so they are set
	// explicitly rather than hoped for: two backups written in the same
	// millisecond would otherwise order arbitrarily.
	entries, err := backup.List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("List returned %d entries", len(entries))
	}

	older := entries[0].Path
	if err := os.Chtimes(older, entries[0].ModTime.Add(-time.Hour), entries[0].ModTime.Add(-time.Hour)); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	if _, err := backup.CreateNamed(dir, "newer"); err != nil {
		t.Fatalf("CreateNamed: %v", err)
	}

	entries, err = backup.List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("List returned %d entries, want 2", len(entries))
	}
	if !strings.HasPrefix(entries[0].Name, "newer") {
		t.Errorf("the newest backup is listed second: %v", []string{entries[0].Name, entries[1].Name})
	}
}

// writeEvilArchive produces a tarball with one entry at the given path.
func writeEvilArchive(t *testing.T, path, name string) {
	t.Helper()

	f, err := os.Create(path) //nolint:gosec // the test owns this path
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close() //nolint:errcheck // the test's verdict is the restore's

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	body := []byte("owned")
	if err := tw.WriteHeader(&tar.Header{
		Name: name, Mode: 0o600, Size: int64(len(body)), Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatalf("write body: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
}

// writeSymlinkArchive produces a tarball whose only entry is a symlink.
func writeSymlinkArchive(t *testing.T, path string) {
	t.Helper()

	f, err := os.Create(path) //nolint:gosec // the test owns this path
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close() //nolint:errcheck // as above

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	if err := tw.WriteHeader(&tar.Header{
		Name: "link", Linkname: "/etc/shadow", Typeflag: tar.TypeSymlink, Mode: 0o777,
	}); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
}
