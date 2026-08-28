package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestProcessLockExcludesSecondAcquire(t *testing.T) {
	dir := t.TempDir()

	release, err := AcquireProcessLock(dir)
	if err != nil {
		t.Fatalf("first AcquireProcessLock: %v", err)
	}
	t.Cleanup(func() { _ = release() })

	_, err = AcquireProcessLock(dir)
	if err == nil {
		t.Fatal("second AcquireProcessLock succeeded; the whole point of the lock is that it does not")
	}
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second AcquireProcessLock error = %v, want ErrAlreadyRunning", err)
	}
	if !strings.Contains(err.Error(), dir) {
		t.Fatalf("error %q does not name the data directory %q", err, dir)
	}
}

func TestProcessLockNamesTheHoldingPID(t *testing.T) {
	dir := t.TempDir()

	release, err := AcquireProcessLock(dir)
	if err != nil {
		t.Fatalf("AcquireProcessLock: %v", err)
	}
	t.Cleanup(func() { _ = release() })

	raw, err := os.ReadFile(filepath.Join(dir, LockFileName))
	if err != nil {
		t.Fatalf("read lock file: %v", err)
	}
	got, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("lock file %q does not hold an integer pid: %v", raw, err)
	}
	if got != os.Getpid() {
		t.Fatalf("lock file records pid %d, want %d", got, os.Getpid())
	}

	_, err = AcquireProcessLock(dir)
	if err == nil {
		t.Fatal("second AcquireProcessLock succeeded")
	}
	if !strings.Contains(err.Error(), strconv.Itoa(os.Getpid())) {
		t.Fatalf("error %q does not name the holding pid %d", err, os.Getpid())
	}
}

func TestProcessLockReleaseAllowsReacquire(t *testing.T) {
	dir := t.TempDir()

	release, err := AcquireProcessLock(dir)
	if err != nil {
		t.Fatalf("first AcquireProcessLock: %v", err)
	}
	if err := release(); err != nil {
		t.Fatalf("release: %v", err)
	}

	release2, err := AcquireProcessLock(dir)
	if err != nil {
		t.Fatalf("AcquireProcessLock after release: %v", err)
	}
	if err := release2(); err != nil {
		t.Fatalf("second release: %v", err)
	}
}

func TestProcessLockFileIsNotWorldReadable(t *testing.T) {
	dir := t.TempDir()

	release, err := AcquireProcessLock(dir)
	if err != nil {
		t.Fatalf("AcquireProcessLock: %v", err)
	}
	t.Cleanup(func() { _ = release() })

	info, err := os.Stat(filepath.Join(dir, LockFileName))
	if err != nil {
		t.Fatalf("stat lock file: %v", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("lock file mode is %04o, want no group or other bits", info.Mode().Perm())
	}
}

func TestEntityLocksSerializeTheSameKey(t *testing.T) {
	locks := NewEntityLocks()

	const goroutines = 50
	counter := 0
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			unlock := locks.LockEntity("users", "same")
			defer unlock()
			counter++ // -race turns an unserialized increment into a failure
		}()
	}
	wg.Wait()

	if counter != goroutines {
		t.Fatalf("counter = %d, want %d", counter, goroutines)
	}
}

func TestEntityLocksDoNotSerializeDifferentKeys(t *testing.T) {
	locks := NewEntityLocks()

	unlockA := locks.LockEntity("users", "a")
	defer unlockA()

	done := make(chan struct{})
	go func() {
		unlockB := locks.LockEntity("users", "b")
		unlockB()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("locking a different id blocked while another id was held: the lock map serializes globally")
	}

	unlockKind := make(chan struct{})
	go func() {
		u := locks.LockEntity("sessions", "a")
		u()
		close(unlockKind)
	}()
	select {
	case <-unlockKind:
	case <-time.After(2 * time.Second):
		t.Fatal("locking the same id under a different kind blocked: the key does not include the kind")
	}
}

func TestEntityLocksReleaseEmptiesTheMap(t *testing.T) {
	locks := NewEntityLocks()

	for i := range 100 {
		unlock := locks.LockEntity("sessions", fmt.Sprintf("s%d", i))
		unlock()
	}
	if n := locks.Len(); n != 0 {
		t.Fatalf("lock map holds %d entries after every holder released; it grows without bound", n)
	}
}
