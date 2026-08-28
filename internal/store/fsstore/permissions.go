package fsstore

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// permMask is the set of bits that must be clear on everything in the data
// directory: no group access, no other access. The data directory holds
// password hashes, session material and, from phase 2 on, the cluster PKI —
// the highest-value surface in the project. A single 0644 file there is a key
// readable by every account on the host.
const permMask = 0o077

// ErrPermissions is returned when the data directory or anything in it grants
// access beyond the owner.
var ErrPermissions = errors.New("fsstore: data directory permissions are too permissive")

// Guard refuses to start on a data directory that anyone but the owner can
// read.
//
// Every violation is collected and reported in one error rather than the first
// one aborting: the operator should repair once and start once, not start
// seven times to discover seven files. Nothing is repaired automatically —
// silently chmod-ing an operator's files would hide the fact that they were
// exposed, and the window during which they were exposed is the thing worth
// knowing about.
func Guard(dir string) error {
	var violations []string

	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		mode := info.Mode()

		switch {
		case d.IsDir():
			if mode.Perm()&permMask != 0 {
				violations = append(violations, fmt.Sprintf(
					"%s is mode %04o, want 0700", p, mode.Perm()))
			}
		case mode.IsRegular():
			if mode.Perm()&permMask != 0 {
				violations = append(violations, fmt.Sprintf(
					"%s is mode %04o, want 0600", p, mode.Perm()))
			}
		default:
			// A symlink or device node in a holzkube data directory is not a
			// permission problem, it is a shape problem: it can point anywhere.
			violations = append(violations, fmt.Sprintf(
				"%s is not a regular file or directory (%s)", p, mode.Type()))
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("fsstore: inspect %s: %w", dir, err)
	}

	if len(violations) == 0 {
		return nil
	}
	return fmt.Errorf("%w:\n  %s\n\nRepair with: chmod 0700 %s && chmod 0600 %s",
		ErrPermissions, strings.Join(violations, "\n  "), dir, filepath.Join(dir, "*"))
}

// sweepTempFiles removes every partially written record left by a crash.
//
// It runs before anything is read, which is the whole point: a temporary file
// is by definition a record that was never committed, and reading one as state
// would resurrect a write that the crash discarded.
func sweepTempFiles(dir string) error {
	var stale []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() && strings.HasPrefix(d.Name(), tempPrefix) {
			stale = append(stale, p)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("fsstore: scan %s for temporary records: %w", dir, err)
	}

	for _, p := range stale {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("fsstore: remove orphaned temporary %s: %w", p, err)
		}
	}
	if len(stale) > 0 {
		return fsyncDir(dir)
	}
	return nil
}
