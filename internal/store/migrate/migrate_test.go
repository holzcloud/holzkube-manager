package migrate

import (
	"archive/tar"
	"compress/gzip"
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

// TestMigrateLegacyDirectoryWithoutVersionIsTreatedAsVersionOne pins the
// reading of a directory written before VERSION existed. It is version 1, not
// version 0 and not "whatever the binary is": phase 1 wrote that layout, and
// phase 2 is the first release for which that makes it one version behind. So
// it is migrated, and because it is migrated it is backed up first.
func TestMigrateLegacyDirectoryWithoutVersionIsTreatedAsVersionOne(t *testing.T) {
	dir := copyFixture(t, "legacy-no-version")

	if err := Run(dir); err != nil {
		t.Fatalf("Run on a pre-VERSION directory: %v", err)
	}

	if got := readVersionFile(t, dir); got != strconv.Itoa(CurrentVersion) {
		t.Fatalf("VERSION = %q, want %q", got, strconv.Itoa(CurrentVersion))
	}
	if files := backupFiles(t, dir); len(files) != 1 {
		t.Fatalf("backups = %v, want the pre-migration tarball for the 1 -> %d step", files, CurrentVersion)
	}
	if _, err := os.Stat(filepath.Join(dir, "users", "alice.json")); err != nil {
		t.Fatalf("the existing record did not survive: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "schematics")); err != nil {
		t.Fatalf("the migration did not run on a pre-VERSION directory: %v", err)
	}
}

func TestMigrateUpgradeBacksUpBeforeChangingAnything(t *testing.T) {
	// A real forward step: the directory sits at the version this binary
	// writes today and the table advances it to the next one. That is the
	// shape phase 3 adds. Driving it with a synthetic version 0 instead would
	// exercise a version readVersion refuses as impossible.
	dir := copyFixture(t, "version-current")
	target := CurrentVersion + 1

	var sawVersionAt string
	migs := []Migration{{
		From: CurrentVersion,
		To:   target,
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

	if err := run(dir, migs, target); err != nil {
		t.Fatalf("run: %v", err)
	}

	if sawVersionAt != strconv.Itoa(CurrentVersion) {
		t.Fatalf("VERSION was %q while the migration ran, want the old value %d", sawVersionAt, CurrentVersion)
	}
	if got := readVersionFile(t, dir); got != strconv.Itoa(target) {
		t.Fatalf("VERSION = %q after a successful migration, want %d", got, target)
	}

	files := backupFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("backups = %v, want exactly one tarball", files)
	}
	name := files[0]
	wantPrefix := "pre-migration-" + strconv.Itoa(CurrentVersion) + "-to-" + strconv.Itoa(target) + "-"
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
	dir := copyFixture(t, "version-current")
	target := CurrentVersion + 1

	boom := errors.New("migration exploded")
	migs := []Migration{{
		From:  CurrentVersion,
		To:    target,
		Apply: func(string) error { return boom },
	}}

	err := run(dir, migs, target)
	if !errors.Is(err, boom) {
		t.Fatalf("run error = %v, want the migration's own error", err)
	}

	if got := readVersionFile(t, dir); got != strconv.Itoa(CurrentVersion) {
		t.Fatalf("VERSION = %q after a failed migration, want the old value %d", got, CurrentVersion)
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
		t.Fatal("Run accepted a directory written by a newer holzkube-manager; it would downgrade records it cannot read")
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

// A version the table cannot advance used to be backed up, skipped by every
// migration, and then stamped as fully migrated anyway. The directory was
// marked current while its records were still in the old shape, which is
// exactly the silent data loss this package exists to prevent.
func TestMigrateRefusesAVersionNoMigrationReaches(t *testing.T) {
	dir := copyFixture(t, "version-current")
	target := CurrentVersion + 2
	before := listTree(t, dir)

	// A hole: the table jumps straight to the last step, so nothing starts
	// at CurrentVersion.
	migs := []Migration{{
		From:  CurrentVersion + 1,
		To:    target,
		Apply: func(string) error { t.Error("a migration ran for a path that has a hole in it"); return nil },
	}}

	err := run(dir, migs, target)
	if err == nil {
		t.Fatal("run stamped a version the migrations never reached")
	}
	if !strings.Contains(err.Error(), strconv.Itoa(CurrentVersion)) {
		t.Fatalf("error %q does not name the version the chain is stuck at", err)
	}

	if got := readVersionFile(t, dir); got != strconv.Itoa(CurrentVersion) {
		t.Fatalf("VERSION = %q, want it left at %d rather than stamped %d", got, CurrentVersion, target)
	}
	if after := listTree(t, dir); !slices.Equal(before, after) {
		t.Fatalf("a directory that cannot be advanced was still written to:\nbefore %v\nafter  %v", before, after)
	}
	if files := backupFiles(t, dir); len(files) != 0 {
		t.Fatalf("backups = %v; a refusal must not leave a tarball implying a migration was attempted", files)
	}
}

// readVersion's own doc comment says no release ever wrote a version 0
// layout. A parser that accepted one let that impossible version through to
// the migration loop.
func TestMigrateRefusesVersionZero(t *testing.T) {
	dir := copyFixture(t, "version-previous")
	before := listTree(t, dir)

	err := Run(dir)
	if err == nil {
		t.Fatal("Run accepted VERSION 0, a version no release ever wrote")
	}
	if !errors.Is(err, ErrVersionUnreadable) {
		t.Fatalf("err = %v, want ErrVersionUnreadable", err)
	}
	if after := listTree(t, dir); !slices.Equal(before, after) {
		t.Fatalf("a refused start still wrote to the directory:\nbefore %v\nafter  %v", before, after)
	}
}

// TestMigrateVersionOneGainsTheSchematicsDirectory drives the first real
// migration in the table, with the real Run rather than a synthetic one.
//
// The ordering assertion is the point: the tarball must be a copy of the
// directory as it was found, so it must NOT contain the schematics directory
// the migration goes on to create. Checking the tarball's contents proves the
// backup happened first in a way that checking for the tarball's existence
// afterwards cannot.
func TestMigrateVersionOneGainsTheSchematicsDirectory(t *testing.T) {
	dir := copyFixture(t, "version-1")

	if err := Run(dir); err != nil {
		t.Fatalf("Run on a version-1 directory: %v", err)
	}

	if got := readVersionFile(t, dir); got != "2" {
		t.Fatalf("VERSION = %q, want 2", got)
	}

	info, err := os.Stat(filepath.Join(dir, "schematics"))
	if err != nil {
		t.Fatalf("the schematics directory was not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("schematics is not a directory")
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("schematics is %04o, want 0700: it holds kernel arguments and META values", perm)
	}

	if _, err := os.Stat(filepath.Join(dir, "users", "alice.json")); err != nil {
		t.Fatalf("the existing record did not survive the migration: %v", err)
	}

	files := backupFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("backups = %v, want exactly one pre-migration tarball", files)
	}
	names := tarballEntries(t, filepath.Join(dir, BackupsDirName, files[0]))
	if !slices.ContainsFunc(names, func(n string) bool { return strings.Contains(n, "users/alice.json") }) {
		t.Errorf("the backup does not contain the record it was taken to protect: %v", names)
	}
	if slices.ContainsFunc(names, func(n string) bool { return strings.Contains(n, "schematics") }) {
		t.Errorf("the backup contains the schematics directory, so it was taken after the migration ran: %v", names)
	}
}

// tarballEntries lists the paths inside a pre-migration backup.
func tarballEntries(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path) //nolint:gosec // a test reading a tarball it just made
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gunzip backup: %v", err)
	}
	defer gz.Close()

	var names []string
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("read backup: %v", err)
		}
		names = append(names, hdr.Name)
	}
	return names
}
