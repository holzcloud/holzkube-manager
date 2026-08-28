package migrate

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// copyFixture materializes one of internal/store/testdata's frozen data
// directories into a scratch directory. Run writes, so the fixture is never
// touched in place.
func copyFixture(t *testing.T, name string) string {
	t.Helper()
	src := filepath.Join("..", "testdata", name)
	dst := t.TempDir()
	if err := os.Chmod(dst, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	err := filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
	if err != nil {
		t.Fatalf("copy fixture %s: %v", name, err)
	}
	return dst
}

func readVersionFile(t *testing.T, dir string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, VersionFileName))
	if err != nil {
		t.Fatalf("read VERSION: %v", err)
	}
	return strings.TrimSpace(string(raw))
}

func listTree(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(p string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		if rel != "." {
			out = append(out, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	slices.Sort(out)
	return out
}

func backupFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, BackupsDirName))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatalf("read backups dir: %v", err)
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func TestMigrateFreshDirectoryWritesVersionAndNoBackup(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	if err := Run(dir); err != nil {
		t.Fatalf("Run on an empty directory: %v", err)
	}

	if got := readVersionFile(t, dir); got != strconv.Itoa(CurrentVersion) {
		t.Fatalf("VERSION = %q, want %q", got, strconv.Itoa(CurrentVersion))
	}
	if files := backupFiles(t, dir); len(files) != 0 {
		t.Fatalf("a fresh directory produced backups %v; there was nothing to back up", files)
	}
}

func TestMigrateCurrentVersionIsANoOp(t *testing.T) {
	dir := copyFixture(t, "version-current")
	before := listTree(t, dir)

	if err := Run(dir); err != nil {
		t.Fatalf("Run at the current version: %v", err)
	}

	if got := readVersionFile(t, dir); got != strconv.Itoa(CurrentVersion) {
		t.Fatalf("VERSION = %q, want unchanged %q", got, strconv.Itoa(CurrentVersion))
	}
	if after := listTree(t, dir); !slices.Equal(before, after) {
		t.Fatalf("Run at the current version changed the directory:\nbefore %v\nafter  %v", before, after)
	}
}

func TestMigrateLegacyDirectoryWithoutVersionIsTreatedAsCurrent(t *testing.T) {
	dir := copyFixture(t, "legacy-no-version")

	if err := Run(dir); err != nil {
		t.Fatalf("Run on a pre-VERSION directory: %v", err)
	}

	if got := readVersionFile(t, dir); got != strconv.Itoa(CurrentVersion) {
		t.Fatalf("VERSION = %q, want %q", got, strconv.Itoa(CurrentVersion))
	}
	if files := backupFiles(t, dir); len(files) != 0 {
		t.Fatalf("a directory already at the current schema produced backups %v", files)
	}
	if _, err := os.Stat(filepath.Join(dir, "users", "alice.json")); err != nil {
		t.Fatalf("the existing record did not survive: %v", err)
	}
}

func TestMigrateUpgradeBacksUpBeforeChangingAnything(t *testing.T) {
	dir := copyFixture(t, "version-previous")

	var sawVersionAt string
	migs := []Migration{{
		From: CurrentVersion - 1,
		To:   CurrentVersion,
		Apply: func(d string) error {
			// The backup must already exist and VERSION must still be the old
			// value at the moment the first migration runs.
			if files := backupFiles(t, d); len(files) != 1 {
				t.Errorf("migration ran with %d backups on disk, want exactly 1 written beforehand", len(files))
			}
			sawVersionAt = readVersionFile(t, d)
			return nil
		},
	}}

	if err := run(dir, migs); err != nil {
		t.Fatalf("run: %v", err)
	}

	if sawVersionAt != strconv.Itoa(CurrentVersion-1) {
		t.Fatalf("VERSION was %q while the migration ran, want the old value %d", sawVersionAt, CurrentVersion-1)
	}
	if got := readVersionFile(t, dir); got != strconv.Itoa(CurrentVersion) {
		t.Fatalf("VERSION = %q after a successful migration, want %d", got, CurrentVersion)
	}

	files := backupFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("backups = %v, want exactly one tarball", files)
	}
	name := files[0]
	wantPrefix := "pre-migration-" + strconv.Itoa(CurrentVersion-1) + "-to-" + strconv.Itoa(CurrentVersion) + "-"
	if !strings.HasPrefix(name, wantPrefix) || !strings.HasSuffix(name, ".tar.gz") {
		t.Fatalf("backup name %q does not match %s<timestamp>.tar.gz", name, wantPrefix)
	}

	info, err := os.Stat(filepath.Join(dir, BackupsDirName, name))
	if err != nil {
		t.Fatalf("stat backup: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("backup mode is %04o, want 0600: the tarball holds every secret in the directory", info.Mode().Perm())
	}
	if info.Size() == 0 {
		t.Fatal("backup tarball is empty")
	}
}

func TestMigrateFailureLeavesTheOldVersionInPlace(t *testing.T) {
	dir := copyFixture(t, "version-previous")

	boom := errors.New("migration exploded")
	migs := []Migration{{
		From:  CurrentVersion - 1,
		To:    CurrentVersion,
		Apply: func(string) error { return boom },
	}}

	err := run(dir, migs)
	if !errors.Is(err, boom) {
		t.Fatalf("run error = %v, want the migration's own error", err)
	}

	if got := readVersionFile(t, dir); got != strconv.Itoa(CurrentVersion-1) {
		t.Fatalf("VERSION = %q after a failed migration, want the old value %d", got, CurrentVersion-1)
	}
	if files := backupFiles(t, dir); len(files) != 1 {
		t.Fatalf("backups = %v, want the pre-migration tarball to survive the failure", files)
	}
}

func TestMigrateRefusesAVersionNewerThanTheBinary(t *testing.T) {
	dir := copyFixture(t, "version-future")
	before := listTree(t, dir)

	err := Run(dir)
	if err == nil {
		t.Fatal("Run accepted a directory written by a newer holzkube; it would downgrade records it cannot read")
	}
	if !errors.Is(err, ErrVersionTooNew) {
		t.Fatalf("err = %v, want ErrVersionTooNew", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, strconv.Itoa(CurrentVersion+1)) || !strings.Contains(msg, strconv.Itoa(CurrentVersion)) {
		t.Fatalf("error %q must name both the on-disk version %d and the binary version %d",
			msg, CurrentVersion+1, CurrentVersion)
	}

	if got := readVersionFile(t, dir); got != strconv.Itoa(CurrentVersion+1) {
		t.Fatalf("VERSION = %q, want it left untouched at %d", got, CurrentVersion+1)
	}
	if after := listTree(t, dir); !slices.Equal(before, after) {
		t.Fatalf("a refused start still wrote to the directory:\nbefore %v\nafter  %v", before, after)
	}
}

func TestMigrateRefusesAnUnreadableVersion(t *testing.T) {
	dir := copyFixture(t, "version-garbage")
	before := listTree(t, dir)

	err := Run(dir)
	if err == nil {
		t.Fatal("Run guessed a version instead of refusing an unparsable VERSION")
	}
	if !strings.Contains(err.Error(), VersionFileName) {
		t.Fatalf("error %q does not name the offending file", err)
	}
	if after := listTree(t, dir); !slices.Equal(before, after) {
		t.Fatalf("a refused start still wrote to the directory:\nbefore %v\nafter  %v", before, after)
	}
}

func TestMigrationsAreOrderedAndForwardOnly(t *testing.T) {
	for i, m := range migrations {
		if m.To != m.From+1 {
			t.Errorf("migration %d goes %d -> %d; each step must advance by exactly one", i, m.From, m.To)
		}
		if i > 0 && m.From != migrations[i-1].To {
			t.Errorf("migration %d starts at %d but the previous ends at %d: the chain has a hole",
				i, m.From, migrations[i-1].To)
		}
	}
	if n := len(migrations); n > 0 && migrations[n-1].To != CurrentVersion {
		t.Errorf("the last migration ends at %d but CurrentVersion is %d", migrations[n-1].To, CurrentVersion)
	}
}
