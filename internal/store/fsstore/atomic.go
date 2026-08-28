package fsstore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/holzcloud/holzkube/internal/store"
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
