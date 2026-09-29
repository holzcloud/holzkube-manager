package fsstore

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/holzcloud/holzkube-manager/internal/store"
)

const (
	dirPerm  = 0o700
	filePerm = 0o600

	// tempPrefix marks a record that is mid-write. Open sweeps every file
	// carrying it before reading anything, and safeKey rejects it as an
	// identifier, so a temporary can never collide with a real record.
	tempPrefix = store.TempFilePrefix
)

// interruptPoint names a place in the write sequence where a test can make the
// process behave as if it had been killed.
type interruptPoint int32

const (
	interruptNone interruptPoint = iota

	// duringTempWrite: the payload is half written and nothing is renamed.
	duringTempWrite

	// afterTempWrite: the temporary file is complete and durable, but the
	// rename has not happened.
	afterTempWrite

	// afterRename: the rename is done but the directory entry is not yet
	// flushed.
	afterRename
)

// errInterrupted is what an injected crash returns. It is never produced by
// real I/O.
var errInterrupted = errors.New("fsstore: write interrupted at an injected crash point")

// interruptAfter is the crash-injection hook. It is written only by the test
// helpers in atomic_test.go and defaults to interruptNone, so production
// builds run straight through every check below. It is atomic rather than a
// plain variable so that the race detector stays quiet in a package whose
// other tests are deliberately concurrent.
var interruptAfter atomic.Int32

func interruptedAt(p interruptPoint) bool {
	return interruptPoint(interruptAfter.Load()) == p
}

// writeAtomic writes data to path so that a reader (or a crash) never observes
// a partial record.
//
// The sequence is the whole point and each step earns its place:
//
//	tmp file in the same directory -> chmod 0600 -> write -> fsync(file)
//	-> rename -> fsync(dir)
//
// Same directory, because rename is only atomic within a filesystem. chmod
// before the write, because a 0644 window is a window. fsync on the file
// before the rename, because rename only orders the directory entry, not the
// data behind it. fsync on the directory afterwards, because otherwise the
// rename itself can be lost on power failure.
//
// An injected crash returns errInterrupted *without* removing the temporary
// file. That is not an oversight: a real kill -9 runs no deferred cleanup
// either, and the orphan it leaves is precisely what Open must sweep.
func writeAtomic(path string, data []byte) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, tempPrefix+"*")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()

	// crashed suppresses cleanup, so an injected interruption leaves the same
	// debris on disk that a killed process would.
	crashed := false
	defer func() {
		if err != nil && !crashed {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()

	if err = tmp.Chmod(filePerm); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}

	if interruptedAt(duringTempWrite) {
		_, _ = tmp.Write(data[:len(data)/2])
		_ = tmp.Sync()
		_ = tmp.Close()
		crashed = true
		err = errInterrupted
		return err
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

	if interruptedAt(afterTempWrite) {
		crashed = true
		err = errInterrupted
		return err
	}

	if err = os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename into place: %w", err)
	}

	if interruptedAt(afterRename) {
		crashed = true
		err = errInterrupted
		return err
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

// WriteFileAtomic is writeAtomic for a file that is not a record: something
// kept in the data directory beside the store rather than inside it, which
// still has to survive a crash as either the old version or the new one.
//
// It is this function and not a copy of its sequence because the sequence is
// the store's crash contract -- the temp prefix the startup sweep removes, the
// modes Guard insists on, the two fsyncs -- and a second copy is a second place
// for one of them to be forgotten. The metrics history (internal/history) is
// its first caller: a file rewritten once a minute is the file most likely to
// be mid-write when the power goes.
func WriteFileAtomic(path string, data []byte) error {
	return writeAtomic(path, data)
}

// PlaceNew puts a new file at path only if nothing is there yet: either the
// whole of data appears under path, or -- when path is taken -- nothing changes
// and the error wraps fs.ErrExist.
//
// It is the one-slot primitive of the host actions (internal/host/hostaction):
// the daemon leaves a one-line order for the root helper, and a second order
// while the first still waits is refused, never queued and never swapped in.
//
// It lives here for the same reason WriteFileAtomic does: the data directory is
// reached through this package and nowhere else
// (TestNoDirectFileAccessOutsideFsstore), and the sequence is the store's crash
// contract -- a temporary carrying the prefix the startup sweep removes, the
// 0600 mode Guard insists on, the two fsyncs.
//
// It ends in link(2) and not in rename(2), and that is the whole difference to
// writeAtomic. A rename replaces whatever lies at path, so "refuse if taken"
// would have to be a check followed by the rename -- and two requests, or two
// processes, can both pass the check. A link fails with EEXIST when the name
// exists, atomically, in the kernel. The temporary name is removed afterwards in
// every case; on success the file lives on under path alone.
func PlaceNew(path string, data []byte) (err error) {
	dir := filepath.Dir(path)

	tmp, err := os.CreateTemp(dir, tempPrefix+"*")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = tmp.Close()
		}
		// The temporary is never the result: on success path holds a second
		// link to the same inode, on failure there is nothing to keep.
		_ = os.Remove(tmpName)
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
	closed = true
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err = os.Link(tmpName, path); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("%s is already there: %w", path, fs.ErrExist)
		}
		return fmt.Errorf("link into place: %w", err)
	}
	if err = os.Remove(tmpName); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove temp file: %w", err)
	}
	if err = fsyncDir(dir); err != nil {
		return fmt.Errorf("fsync directory: %w", err)
	}
	return nil
}

// ReadFile is WriteFileAtomic's other half: the one read of a file kept beside
// the store. It is here rather than an os.ReadFile in the caller because the
// data directory is reached through this package and nowhere else
// (TestNoDirectFileAccessOutsideFsstore), and a file the store writes is a file
// the store reads.
func ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path) //nolint:gosec // a path inside the data directory, named by its owner
}
