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
	"strconv"
	"strings"

	"github.com/holzcloud/holzkube/internal/store"
	"github.com/holzcloud/holzkube/internal/store/migrate/backup"
)

// CurrentVersion is the schema version this binary understands.
const CurrentVersion = 1

const (
	// VersionFileName holds the schema version of a data directory.
	VersionFileName = "VERSION"

	// BackupsDirName is re-exported from the backup package so that callers
	// and tests have one name to use.
	BackupsDirName = backup.DirName

	filePerm = 0o600
)

var (
	// ErrVersionTooNew is returned when the data directory was written by a
	// newer holzkube. Starting anyway would let an older binary rewrite
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

// migrations is the ordered, forward-only list. Phase 1 defines exactly one
// schema version, so the list is empty: every version-1 directory is already
// current and there is nothing to apply. The surrounding machinery is
// exercised by tests against fixtures anyway, so that phase 3 only appends an
// entry to a path that has already been proven.
var migrations = []Migration{}

// Run brings dir up to CurrentVersion, or refuses to start.
func Run(dir string) error {
	return run(dir, migrations)
}

func run(dir string, migs []Migration) error {
	from, err := readVersion(dir)
	if err != nil {
		return err
	}

	switch {
	case from > CurrentVersion:
		return fmt.Errorf("%w: %s says version %d, this binary understands version %d; upgrade holzkube",
			ErrVersionTooNew, filepath.Join(dir, VersionFileName), from, CurrentVersion)
	case from == CurrentVersion:
		// Nothing to do, and deliberately nothing written: an upgrade that
		// changed no schema must not churn the operator's data directory.
		return nil
	}

	// A backup before the first change, not after the last one. Everything
	// below this line can fail with the directory half-migrated.
	if _, err := backup.Create(dir, from, CurrentVersion); err != nil {
		return fmt.Errorf("migrate: refusing to migrate without a backup: %w", err)
	}

	at := from
	for _, m := range migs {
		if m.From != at {
			continue
		}
		if err := m.Apply(dir); err != nil {
			// VERSION still reads the old value, so a fixed binary re-runs
			// this migration rather than skipping it.
			return fmt.Errorf("migrate: %d -> %d: %w", m.From, m.To, err)
		}
		at = m.To
	}

	return writeVersion(dir, CurrentVersion)
}

// readVersion reports the schema version of dir.
//
// A missing VERSION is ambiguous and is resolved by looking at the directory:
// empty means a fresh install, which is current by definition; existing
// records mean a directory written by holzkube before VERSION existed, which
// is version 1 — not version 0, because no release ever wrote a version 0
// layout.
func readVersion(dir string) (int, error) {
	path := filepath.Join(dir, VersionFileName)
	raw, err := os.ReadFile(path) //nolint:gosec // migrate is part of the store layer and owns this path
	switch {
	case errors.Is(err, os.ErrNotExist):
		empty, err := isEmpty(dir)
		if err != nil {
			return 0, err
		}
		if empty {
			if err := writeVersion(dir, CurrentVersion); err != nil {
				return 0, err
			}
			return CurrentVersion, nil
		}
		if err := writeVersion(dir, 1); err != nil {
			return 0, err
		}
		return 1, nil
	case err != nil:
		return 0, fmt.Errorf("migrate: read %s: %w", path, err)
	}

	v, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || v < 0 {
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
