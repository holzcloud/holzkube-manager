// Package backup writes the pre-migration tarball.
//
// It is deliberately its own package rather than a file inside migrate: the
// concern is "capture this directory as it stands", which has nothing to do
// with schema sequencing and everything to do with not losing an operator's
// cluster PKI to a migration bug.
package backup

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/holzcloud/holzkube/internal/store"
)

const (
	// DirName is the subdirectory of the data directory that holds tarballs.
	DirName = "backups"

	dirPerm  = 0o700
	filePerm = 0o600
)

// Create writes a gzip-compressed tarball of the whole data directory to
// <dir>/backups/pre-migration-<from>-to-<to>-<RFC3339>.tar.gz and returns its
// path.
//
// Excluded: the backups directory itself (a backup of the backups grows
// quadratically), the process lock file (it is runtime state, and restoring a
// stale pid is worse than useless), and any half-written temporary record.
//
// The tarball is 0600 and lives inside the 0700 data directory, because it
// contains a verbatim copy of every secret in that directory — a world-readable
// backup would hand away exactly what the permission guard exists to protect.
// It is fsynced before this function returns: a backup that is still in the
// page cache when the migration corrupts the original is not a backup.
func Create(dir string, from, to int) (path string, err error) {
	backupDir := filepath.Join(dir, DirName)
	if err := os.MkdirAll(backupDir, dirPerm); err != nil {
		return "", fmt.Errorf("backup: create %s: %w", backupDir, err)
	}

	name := "pre-migration-" + strconv.Itoa(from) + "-to-" + strconv.Itoa(to) +
		"-" + time.Now().UTC().Format(time.RFC3339) + ".tar.gz"
	path = filepath.Join(backupDir, name)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, filePerm)
	if err != nil {
		return "", fmt.Errorf("backup: create %s: %w", path, err)
	}
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = os.Remove(path)
		}
	}()

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	if err = writeTree(tw, dir); err != nil {
		return "", err
	}
	if err = tw.Close(); err != nil {
		return "", fmt.Errorf("backup: close tar: %w", err)
	}
	if err = gz.Close(); err != nil {
		return "", fmt.Errorf("backup: close gzip: %w", err)
	}
	if err = f.Sync(); err != nil {
		return "", fmt.Errorf("backup: fsync %s: %w", path, err)
	}
	if err = f.Close(); err != nil {
		return "", fmt.Errorf("backup: close %s: %w", path, err)
	}
	return path, nil
}

func writeTree(tw *tar.Writer, root string) error {
	return filepath.WalkDir(root, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if skip(rel, d) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		// Symlinks and devices are not part of a holzkube data directory; a
		// tarball that followed one would capture something outside the tree.
		if !d.IsDir() && !info.Mode().IsRegular() {
			return nil
		}

		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		src, err := os.Open(p) //nolint:gosec // the backup writer is part of the store layer and owns these paths
		if err != nil {
			return err
		}
		defer src.Close()
		_, err = io.Copy(tw, src)
		return err
	})
}

func skip(rel string, d os.DirEntry) bool {
	if rel == DirName && d.IsDir() {
		return true
	}
	if rel == store.LockFileName {
		return true
	}
	return strings.HasPrefix(filepath.Base(rel), store.TempFilePrefix)
}
