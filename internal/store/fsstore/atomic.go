package fsstore

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	dirPerm  = 0o700
	filePerm = 0o600
)

// writeAtomic writes data to path so that a reader (or a crash) never observes
// a partial record.
//
// The sequence is the whole point and each step earns its place:
//
//	tmp file in the same directory -> chmod 0600 -> write -> fsync(file)
//	-> rename -> fsync(dir)
//
// Same directory, because rename is only atomic within a filesystem. chmod
// before the rename, because a 0644 window is a window. fsync on the file
// before the rename, because rename only orders the directory entry, not the
// data behind it. fsync on the directory afterwards, because otherwise the
// rename itself can be lost on power failure.
func writeAtomic(path string, data []byte) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()

	if err = tmp.Chmod(filePerm); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if _, err = tmp.Write(data); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("fsync temp file: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err = os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename into place: %w", err)
	}
	if err = fsyncDir(dir); err != nil {
		return fmt.Errorf("fsync directory: %w", err)
	}
	return nil
}

// fsyncDir flushes a directory entry so that a completed rename survives a
// power failure.
func fsyncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// removeAndSync deletes a file and flushes the directory entry.
func removeAndSync(path string) error {
	if err := os.Remove(path); err != nil {
		return err
	}
	return fsyncDir(filepath.Dir(path))
}
