package fsstore

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/holzcloud/holzkube/internal/model"
	"github.com/holzcloud/holzkube/internal/store"
)

// openTestStore opens a store over a fresh 0700 directory and closes it when
// the test ends, so that the process lock does not leak into the next test.
func openTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod temp dir: %v", err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open(%s): %v", dir, err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, dir
}

func TestProcessLockRefusesSecondOpen(t *testing.T) {
	_, dir := openTestStore(t)

	second, err := Open(dir)
	if err == nil {
		_ = second.Close()
		t.Fatal("a second Open on the same data directory succeeded; two instances can interleave writes")
	}
	if !errors.Is(err, store.ErrAlreadyRunning) {
		t.Fatalf("second Open error = %v, want store.ErrAlreadyRunning", err)
	}
	if !strings.Contains(err.Error(), dir) {
		t.Fatalf("error %q does not name the data directory %q", err, dir)
	}
}

func TestProcessLockIsReleasedOnClose(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	first, err := Open(dir)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	second, err := Open(dir)
	if err != nil {
		t.Fatalf("Open after Close: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestConcurrentPutSameIDYieldsExactlyOneWinner(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()

	seed, err := s.Users().Put(ctx, model.User{ID: "alice", Username: "alice"})
	if err != nil {
		t.Fatalf("seed Put: %v", err)
	}

	const goroutines = 50
	results := make([]error, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func() {
			defer wg.Done()
			_, err := s.Users().Put(ctx, model.User{ID: "alice", Username: "alice", Rev: seed.Rev})
			results[i] = err
		}()
	}
	wg.Wait()

	var winners, conflicts int
	for i, err := range results {
		switch {
		case err == nil:
			winners++
		case errors.Is(err, store.ErrConflict):
			conflicts++
		default:
			t.Fatalf("goroutine %d: unexpected error %v", i, err)
		}
	}
	if winners != 1 {
		t.Fatalf("winners = %d, want exactly 1 (last-write-wins instead of compare-and-swap)", winners)
	}
	if conflicts != goroutines-1 {
		t.Fatalf("conflicts = %d, want %d", conflicts, goroutines-1)
	}

	after, err := s.Users().Get(ctx, "alice")
	if err != nil {
		t.Fatalf("Get after the race: %v", err)
	}
	if after.Rev != seed.Rev+1 {
		t.Fatalf("rev = %d after 50 concurrent puts, want %d (exactly one increment)", after.Rev, seed.Rev+1)
	}
}

func TestConcurrentPutDistinctIDsAllSucceed(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()

	const goroutines = 50
	results := make([]error, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func() {
			defer wg.Done()
			id := model.UserID("user-" + string(rune('a'+i%26)) + "-" + itoa(i))
			_, err := s.Users().Put(ctx, model.User{ID: id, Username: string(id)})
			results[i] = err
		}()
	}
	wg.Wait()

	for i, err := range results {
		if err != nil {
			t.Fatalf("goroutine %d writing its own id failed: %v", i, err)
		}
	}

	all, err := s.Users().List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != goroutines {
		t.Fatalf("List returned %d users, want %d", len(all), goroutines)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

func TestPutWithStaleRevIsRejectedNotOverwritten(t *testing.T) {
	s, dir := openTestStore(t)
	ctx := context.Background()

	if _, err := s.Users().Put(ctx, model.User{ID: "bob", Username: "bob"}); err != nil {
		t.Fatalf("seed Put: %v", err)
	}
	before, err := os.ReadFile(filepath.Join(dir, "users", "bob.json"))
	if err != nil {
		t.Fatalf("read record: %v", err)
	}

	_, err = s.Users().Put(ctx, model.User{ID: "bob", Username: "mallory", Rev: 0})
	if !errors.Is(err, store.ErrConflict) {
		t.Fatalf("Put with rev 0 on an existing record: err = %v, want store.ErrConflict", err)
	}

	after, err := os.ReadFile(filepath.Join(dir, "users", "bob.json"))
	if err != nil {
		t.Fatalf("read record after the rejected put: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("a rejected put still changed the file:\nbefore %s\nafter  %s", before, after)
	}
}

func TestRepeatedPutIsByteStable(t *testing.T) {
	s, dir := openTestStore(t)
	ctx := context.Background()
	path := filepath.Join(dir, "users", "carol.json")

	rec := model.User{ID: "carol", Username: "carol", PasswordHash: "$argon2id$fake"}

	first, err := s.Users().Put(ctx, rec)
	if err != nil {
		t.Fatalf("first Put: %v", err)
	}
	bytes1, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after first Put: %v", err)
	}

	rec.Rev = first.Rev
	second, err := s.Users().Put(ctx, rec)
	if err != nil {
		t.Fatalf("second Put: %v", err)
	}
	bytes2, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after second Put: %v", err)
	}

	if second.Rev != first.Rev+1 {
		t.Fatalf("rev went %d -> %d, want exactly one increment", first.Rev, second.Rev)
	}

	normalized := bytes.ReplaceAll(bytes2, revToken(second.Rev), revToken(first.Rev))
	if !bytes.Equal(bytes1, normalized) {
		t.Fatalf("an identical record serialized differently on the second put:\nfirst  %s\nsecond %s", bytes1, bytes2)
	}

	entries, err := os.ReadDir(filepath.Join(dir, "users"))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("users directory holds %v, want exactly carol.json", names)
	}
}

func revToken(rev uint64) []byte {
	return []byte(`"rev": ` + itoa(int(rev)))
}

func TestConcurrentGetDuringPutNeverSeesAPartialRecord(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()

	cur, err := s.Users().Put(ctx, model.User{ID: "dave", Username: "dave"})
	if err != nil {
		t.Fatalf("seed Put: %v", err)
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		deadline := time.Now().Add(500 * time.Millisecond)
		for time.Now().Before(deadline) {
			next, err := s.Users().Put(ctx, model.User{ID: "dave", Username: "dave", Rev: cur.Rev})
			if err != nil {
				t.Errorf("writer: %v", err)
				break
			}
			cur = next
		}
		close(stop)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			got, err := s.Users().Get(ctx, "dave")
			if err != nil {
				t.Errorf("reader saw %v; a concurrent write must leave either the whole old or the whole new record", err)
				return
			}
			if got.Username != "dave" {
				t.Errorf("reader saw a torn record: %+v", got)
				return
			}
		}
	}()

	wg.Wait()
}
