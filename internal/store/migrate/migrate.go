// Package migrate advances a data directory's schema version.
//
// Migrations run in one direction only. There is no reverse path and none is
// held open as an empty stub, because a reverse path is a promise: it says the
// operator may downgrade the binary after an upgrade has already rewritten
// their records. Honouring that promise for real data is far harder than it
// looks, and pretending to honour it is worse than refusing. The way back is
// the pre-migration tarball, which is a copy of what actually existed rather
// than a guess at how to undo what happened to it.
package migrate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/holzcloud/holzkube-manager/internal/store"
	"github.com/holzcloud/holzkube-manager/internal/store/migrate/backup"
)

// CurrentVersion is the schema version this binary understands.
const CurrentVersion = 2

const (
	// VersionFileName holds the schema version of a data directory.
	VersionFileName = "VERSION"

	// BackupsDirName is re-exported from the backup package so that callers
	// and tests have one name to use.
	BackupsDirName = backup.DirName

	filePerm = 0o600

	// dirPerm is the mode of a directory a migration creates. It matches the
	// mode fsstore uses for every entity directory: a schematic record carries
	// kernel arguments and META values, which may be secrets, and a directory
	// created one mode wider than the rest would be found by the permission
	// guard on the next start rather than here.
	dirPerm = 0o700
)

var (
	// ErrVersionTooNew is returned when the data directory was written by a
	// newer holzkube-manager. Starting anyway would let an older binary rewrite
	// records whose shape it does not know, which is silent data loss.
	ErrVersionTooNew = errors.New("migrate: data directory is newer than this binary")

	// ErrVersionUnreadable is returned when VERSION exists but is not a
	// version. Guessing is not an option: the wrong guess runs the wrong
	// migrations against real data.
	ErrVersionUnreadable = errors.New("migrate: VERSION is not a schema version")
)

// Migration advances a data directory by exactly one version.
type Migration struct {
	From  int
	To    int
	Apply func(dir string) error
}

// migrations is the ordered, forward-only list.
//
// Phase 1 defined exactly one schema version and left this list empty, on the
// understanding that a later phase would append the first entry to machinery
// that had already been proven against fixtures. Phase 2 is that phase: the
// 1 -> 2 step below is the first real migration, and it runs the same backup,
// ordered application and deferred VERSION write the fixture tests exercised.
// Every later phase appends; nothing here is ever edited in place, because a
// migration that has run on someone's data directory is history rather than
// code.
var migrations = []Migration{
	{
		// Schematics gain their own entity directory (D-09). Creating it here
		// rather than relying on Open's MkdirAll means a directory that fails
		// to migrate is left at version 1 with no half-made layout, and the
		// pre-migration tarball is a copy of a directory this step had not yet
		// touched.
		From: 1,
		To:   2,
		Apply: func(dir string) error {
			path := filepath.Join(dir, "schematics")
			if err := os.MkdirAll(path, dirPerm); err != nil {
				return fmt.Errorf("create %s: %w", path, err)
			}
			return nil
		},
	},
}

// Run brings dir up to CurrentVersion, or refuses to start.
func Run(dir string) error {
	return run(dir, migrations, CurrentVersion)
}

// run advances dir to target using migs. target is a parameter rather than
// CurrentVersion directly so that the whole upgrade path — backup, ordered
// application, deferred VERSION write, failure handling — can be exercised
// against a forward step the shipped table does not contain. That is how the
// path was proven before any real migration existed, and it is still how the
// failure branches are driven: a test supplies a table advancing the current
// version to the next one, rather than a synthetic 0 -> 1 step that readVersion
// rejects as impossible.
func run(dir string, migs []Migration, target int) error {
	from, err := readVersion(dir, target)
	if err != nil {
		return err
	}

	switch {
	case from > target:
		return fmt.Errorf("%w: %s says version %d, this binary understands version %d; upgrade holzkube-manager",
			ErrVersionTooNew, filepath.Join(dir, VersionFileName), from, target)
	case from == target:
		// Nothing to do, and deliberately nothing written: an upgrade that
		// changed no schema must not churn the operator's data directory.
		return nil
	}

	// Resolve the whole path before writing anything at all. A gap in the
	// table used to be discovered only after the backup had been taken and
	// every migration skipped, at which point VERSION was stamped current
	// regardless — the directory was marked migrated when nothing had run.
	// Refusing here means an unadvanceable directory is left exactly as it
	// was found, with no tarball implying a migration was attempted.
	planned, err := plan(migs, from, target)
	if err != nil {
		return err
	}

	// A backup before the first change, not after the last one. Everything
	// below this line can fail with the directory half-migrated.
	if _, err := backup.Create(dir, from, target); err != nil {
		return fmt.Errorf("migrate: refusing to migrate without a backup: %w", err)
	}

	at := from
	for _, m := range planned {
		if err := m.Apply(dir); err != nil {
			// VERSION still reads the old value, so a fixed binary re-runs
			// this migration rather than skipping it.
			return fmt.Errorf("migrate: %d -> %d: %w", m.From, m.To, err)
		}
		at = m.To
	}

	// plan guarantees this, so a mismatch means the table was mutated under
	// us. Stamping the target anyway is the one outcome this package exists
	// to prevent, so assert instead.
	if at != target {
		return fmt.Errorf("migrate: reached version %d, not %d; VERSION left at %d", at, target, from)
	}

	return writeVersion(dir, target)
}

// plan returns the migrations that advance from to target, in the order they
// must run, or an error naming the version the chain gets stuck at.
func plan(migs []Migration, from, target int) ([]Migration, error) {
	var out []Migration
	at := from
	for at != target {
		i := slices.IndexFunc(migs, func(m Migration) bool { return m.From == at })
		if i < 0 {
			return nil, fmt.Errorf(
				"migrate: no migration path from version %d to %d; this binary has no step "+
					"that starts at version %d, so the data directory cannot be advanced and "+
					"has been left untouched", from, target, at)
		}
		if migs[i].To <= at {
			return nil, fmt.Errorf(
				"migrate: migration %d -> %d does not advance; the table is not forward-only",
				migs[i].From, migs[i].To)
		}
		out = append(out, migs[i])
		at = migs[i].To
	}
	return out, nil
}

// readVersion reports the schema version of dir.
//
// A missing VERSION is ambiguous and is resolved by looking at the directory:
// empty means a fresh install, which is current by definition; existing
// records mean a directory written by holzkube-manager before VERSION existed, which
// is version 1 — not version 0, because no release ever wrote a version 0
// layout.
func readVersion(dir string, target int) (int, error) {
	path := filepath.Join(dir, VersionFileName)
	raw, err := os.ReadFile(path) //nolint:gosec // migrate is part of the store layer and owns this path
	switch {
	case errors.Is(err, os.ErrNotExist):
		empty, err := isEmpty(dir)
		if err != nil {
			return 0, err
		}
		if empty {
			if err := writeVersion(dir, target); err != nil {
				return 0, err
			}
			return target, nil
		}
		if err := writeVersion(dir, 1); err != nil {
			return 0, err
		}
		return 1, nil
	case err != nil:
		return 0, fmt.Errorf("migrate: read %s: %w", path, err)
	}

	// v < 1, not v < 0: the doc comment above asserts that no release ever
	// wrote a version 0 layout, and a parser that accepts one contradicts it.
	// A VERSION of 0 reached the migration loop, matched no step, and was
	// stamped current — the silent data loss this package exists to prevent.
	v, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || v < 1 {
		return 0, fmt.Errorf("%w: %s contains %q", ErrVersionUnreadable, path, strings.TrimSpace(string(raw)))
	}
	return v, nil
}

// isEmpty reports whether dir holds no records yet. Runtime artefacts — the
// lock file, the backups directory, half-written temporaries — do not count as
// state, because none of them is something a migration would have to convert.
func isEmpty(dir string) (bool, error) {
	empty := true
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		base := filepath.Base(rel)
		if d.IsDir() {
			if rel == BackupsDirName {
				return filepath.SkipDir
			}
			return nil
		}
		if base == store.LockFileName || base == VersionFileName ||
			strings.HasPrefix(base, store.TempFilePrefix) {
			return nil
		}
		empty = false
		return filepath.SkipAll
	})
	if err != nil {
		return false, fmt.Errorf("migrate: inspect %s: %w", dir, err)
	}
	return empty, nil
}

// writeVersion replaces VERSION atomically. A half-written version file would
// make the directory unstartable on the very next boot, which is the one
// failure mode this whole package exists to avoid.
func writeVersion(dir string, v int) (err error) {
	path := filepath.Join(dir, VersionFileName)

	tmp, err := os.CreateTemp(dir, store.TempFilePrefix+"version-*")
	if err != nil {
		return fmt.Errorf("migrate: create temp version file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()

	if err = tmp.Chmod(filePerm); err != nil {
		return fmt.Errorf("migrate: chmod temp version file: %w", err)
	}
	if _, err = tmp.WriteString(strconv.Itoa(v) + "\n"); err != nil {
		return fmt.Errorf("migrate: write temp version file: %w", err)
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("migrate: fsync temp version file: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("migrate: close temp version file: %w", err)
	}
	if err = os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("migrate: rename version file into place: %w", err)
	}

	d, err := os.Open(dir) //nolint:gosec // migrate is part of the store layer and owns this path
	if err != nil {
		return fmt.Errorf("migrate: open %s: %w", dir, err)
	}
	defer d.Close()
	if err = d.Sync(); err != nil {
		return fmt.Errorf("migrate: fsync %s: %w", dir, err)
	}
	return nil
}
