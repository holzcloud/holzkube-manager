package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

const (
	// LockFileName is the flock target inside the data directory. It is
	// exported so that the permission guard and the backup writer can name it
	// without re-deriving the string.
	LockFileName = "holzkube.lock"

	lockFilePerm = 0o600
)

// AcquireProcessLock takes an exclusive, non-blocking flock on
// <dir>/holzkube.lock and records the calling pid in it.
//
// This is the outermost of the three layers of concurrency control. The
// per-entity mutex and the rev compare-and-swap protect a data directory from
// the goroutines of one process; only the flock protects it from a second
// process. Two holzkubed instances on one directory would interleave writes
// that each believe they hold the current revision, which no amount of
// in-process locking can detect.
//
// The lock is advisory and released by the returned closure, by closing the
// file, or by the kernel when the process dies — including on kill -9, which
// is what keeps a crashed instance from wedging the directory permanently.
func AcquireProcessLock(dir string) (release func() error, err error) {
	path := filepath.Join(dir, LockFileName)

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, lockFilePerm)
	if err != nil {
		return nil, fmt.Errorf("store: open lock file %s: %w", path, err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		holder := lockHolder(path)
		_ = f.Close()
		return nil, fmt.Errorf("%w: %s is locked by %s", ErrAlreadyRunning, dir, holder)
	}

	// Record who holds it, so the loser of the race can say something useful
	// instead of "resource temporarily unavailable".
	if err := f.Truncate(0); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("store: truncate lock file %s: %w", path, err)
	}
	if _, err := f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("store: write lock file %s: %w", path, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("store: fsync lock file %s: %w", path, err)
	}

	var once sync.Once
	return func() error {
		var rerr error
		once.Do(func() {
			if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil {
				rerr = fmt.Errorf("store: release lock %s: %w", path, err)
			}
			if err := f.Close(); err != nil && rerr == nil {
				rerr = fmt.Errorf("store: close lock file %s: %w", path, err)
			}
		})
		return rerr
	}, nil
}

// lockHolder reports the pid recorded in the lock file, in a form fit for an
// error message. flock is advisory, so reading the file while another process
// holds the lock is legal.
func lockHolder(path string) string {
	raw, err := os.ReadFile(path) //nolint:gosec // the lock file is store-owned metadata, not a state record
	if err != nil {
		return "another process"
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid <= 0 {
		return "another process"
	}
	return "pid " + strconv.Itoa(pid)
}

// EntityLocks is the middle layer: one mutex per (kind, id), so that two
// writers to different records never wait on each other while two writers to
// the same record always do.
//
// A single mutex over the whole store would also be correct and would also be
// a bottleneck; the read-modify-write of a rev CAS must be atomic per record,
// not per store.
type EntityLocks struct {
	mu    sync.Mutex
	locks map[string]*entityLock
}

type entityLock struct {
	mu sync.Mutex
	// refs counts holders and waiters. The entry is deleted when it drops to
	// zero, so that a long-lived process writing millions of short-lived
	// session records does not accumulate a mutex per record forever.
	refs int
}

// NewEntityLocks returns an empty lock map.
func NewEntityLocks() *EntityLocks {
	return &EntityLocks{locks: make(map[string]*entityLock)}
}

// LockEntity blocks until the caller holds the lock for one record and returns
// the release function. The key is kind + "/" + id, so users/a and sessions/a
// are distinct records and do not contend.
func (l *EntityLocks) LockEntity(kind, id string) func() {
	key := kind + "/" + id

	l.mu.Lock()
	el, ok := l.locks[key]
	if !ok {
		el = &entityLock{}
		l.locks[key] = el
	}
	el.refs++
	l.mu.Unlock()

	el.mu.Lock()

	var once sync.Once
	return func() {
		once.Do(func() {
			el.mu.Unlock()
			l.mu.Lock()
			el.refs--
			if el.refs == 0 {
				delete(l.locks, key)
			}
			l.mu.Unlock()
		})
	}
}

// Len reports how many records currently have a lock entry. It exists so a
// test can assert that the map does not grow without bound.
func (l *EntityLocks) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.locks)
}
