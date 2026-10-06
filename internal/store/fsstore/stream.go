package fsstore

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// WriteStreamAtomic is WriteFileAtomic for a file too large to hold in memory:
// an etcd snapshot is as large as a cluster's database, and the caller streams
// it from a node rather than building it.
//
// The sequence is writeAtomic's -- a temporary in the same directory carrying
// the prefix the startup sweep removes, 0600 before the first byte, fsync, rename,
// fsync of the directory -- so that a stream that fails half way, or a process
// that dies in the middle of one, leaves no file under path. fn returns how many
// bytes it wrote; a failure from it removes the temporary and is returned
// wrapped.
func WriteStreamAtomic(path string, fn func(io.Writer) (int64, error)) (n int64, err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return 0, fmt.Errorf("create directory %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, tempPrefix+"*")
	if err != nil {
		return 0, fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()

	if err = tmp.Chmod(filePerm); err != nil {
		return 0, fmt.Errorf("chmod temp file: %w", err)
	}
	if n, err = fn(tmp); err != nil {
		return 0, err
	}
	if err = tmp.Sync(); err != nil {
		return 0, fmt.Errorf("fsync temp file: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return 0, fmt.Errorf("close temp file: %w", err)
	}
	if err = os.Rename(tmpName, path); err != nil {
		return 0, fmt.Errorf("rename into place: %w", err)
	}
	if err = fsyncDir(dir); err != nil {
		return 0, fmt.Errorf("fsync directory: %w", err)
	}
	return n, nil
}

// DirFile is one entry of ListDir.
type DirFile struct {
	Name  string
	Size  int64
	IsDir bool
}

// ListDir lists a directory under the data directory. A directory that does
// not exist is an empty one: the caller creates it by writing into it.
func ListDir(dir string) ([]DirFile, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]DirFile, 0, len(entries))
	for _, e := range entries {
		f := DirFile{Name: e.Name(), IsDir: e.IsDir()}
		if info, ierr := e.Info(); ierr == nil {
			f.Size = info.Size()
		}
		out = append(out, f)
	}
	return out, nil
}

// RemoveFile deletes a file and flushes the directory entry. A file that is
// already gone is not an error.
func RemoveFile(path string) error {
	err := removeAndSync(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// TempPrefix is the prefix of the temporaries WriteStreamAtomic and
// WriteFileAtomic leave while writing; one found without a writer is debris.
const TempPrefix = tempPrefix
