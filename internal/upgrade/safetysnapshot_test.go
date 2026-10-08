package upgrade_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
	"github.com/holzcloud/holzkube-manager/internal/upgrade"
)

// clock is a settable time source, so freshness is decided by the test.
type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

func newStore(t *testing.T) (*upgrade.SnapshotStore, *clock) {
	t.Helper()
	c := &clock{now: time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)}
	return upgrade.NewSnapshotStore(filepath.Join(t.TempDir(), "snaps"), realOps(), c.Now), c
}

// realOps is the data directory's own primitives, the ones main hands in.
func realOps() upgrade.FileOps {
	return upgrade.FileOps{
		WriteStream: fsstore.WriteStreamAtomic,
		List: func(dir string) ([]upgrade.DirFile, error) {
			entries, err := fsstore.ListDir(dir)
			out := make([]upgrade.DirFile, 0, len(entries))
			for _, e := range entries {
				out = append(out, upgrade.DirFile{Name: e.Name, Size: e.Size, IsDir: e.IsDir})
			}
			return out, err
		},
		Remove:     fsstore.RemoveFile,
		TempPrefix: fsstore.TempPrefix,
		ReadFile:   fsstore.ReadFile,
		Open:       fsstore.OpenFile,
	}
}

func put(t *testing.T, st *upgrade.SnapshotStore, cluster string, body string) (upgrade.SafetySnapshot, error) {
	t.Helper()
	return upgrade.WriteForTest(st, model.ClusterID(cluster), func(w io.Writer) (int64, error) {
		n, err := io.WriteString(w, body)
		return int64(n), err
	})
}

func TestNoSnapshotIsNotFresh(t *testing.T) {
	t.Parallel()
	st, _ := newStore(t)
	snap, err := st.Latest("c1")
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if snap.Present || snap.Fresh {
		t.Fatalf("a cluster with no snapshot is present=%v fresh=%v", snap.Present, snap.Fresh)
	}
}

func TestASnapshotIsFreshForAnHourAndNotAfter(t *testing.T) {
	t.Parallel()
	st, clk := newStore(t)
	if _, err := put(t, st, "c1", "etcd bytes"); err != nil {
		t.Fatalf("write: %v", err)
	}

	for _, tc := range []struct {
		after time.Duration
		fresh bool
	}{
		{0, true},
		{59 * time.Minute, true},
		{time.Hour, false},
		{26 * time.Hour, false},
	} {
		clk.now = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC).Add(tc.after)
		snap, err := st.Latest("c1")
		if err != nil {
			t.Fatalf("Latest: %v", err)
		}
		if snap.Fresh != tc.fresh {
			t.Errorf("%s after taking it: fresh = %v, want %v", tc.after, snap.Fresh, tc.fresh)
		}
	}
}

// A clock that moved backwards must not extend a snapshot's life: a file dated
// in the future is not evidence of anything.
func TestASnapshotFromTheFutureIsNotFresh(t *testing.T) {
	t.Parallel()
	st, clk := newStore(t)
	if _, err := put(t, st, "c1", "etcd bytes"); err != nil {
		t.Fatalf("write: %v", err)
	}
	clk.now = clk.now.Add(-2 * time.Hour)
	snap, err := st.Latest("c1")
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if snap.Fresh {
		t.Fatal("a snapshot dated after the clock was reported fresh")
	}
}

func TestAFailedStreamKeepsNothing(t *testing.T) {
	t.Parallel()
	st, _ := newStore(t)
	boom := errors.New("connection reset")
	_, err := upgrade.WriteForTest(st, "c1", func(w io.Writer) (int64, error) {
		_, _ = io.WriteString(w, "half of an etcd")
		return 14, boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want it to wrap the stream's failure", err)
	}
	snap, _ := st.Latest("c1")
	if snap.Present {
		t.Fatal("a stream that failed half way left something that counts as a snapshot")
	}
	entries, _ := os.ReadDir(filepath.Join(snapshotDir(st), "c1"))
	if len(entries) != 0 {
		t.Fatalf("the half-written temporary was left behind: %v", entries)
	}
}

func TestAnEmptyStreamKeepsNothing(t *testing.T) {
	t.Parallel()
	st, _ := newStore(t)
	if _, err := put(t, st, "c1", ""); err == nil {
		t.Fatal("an empty snapshot was accepted")
	}
	if snap, _ := st.Latest("c1"); snap.Present {
		t.Fatal("an empty snapshot counts as one")
	}
}

func TestOnlyTheNewestTwoAreKept(t *testing.T) {
	t.Parallel()
	st, clk := newStore(t)
	for i := range 4 {
		clk.now = clk.now.Add(time.Minute)
		if _, err := put(t, st, "c1", strings.Repeat("x", i+1)); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(snapshotDir(st), "c1"))
	if err != nil {
		t.Fatal(err)
	}
	var kept int
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".snapshot") { // not the checksum files beside them
			kept++
		}
	}
	if kept != 2 {
		t.Fatalf("%d snapshots kept, want 2", kept)
	}
	snap, _ := st.Latest("c1")
	if snap.Bytes != 4 {
		t.Fatalf("the newest is %d bytes, want the last one written (4)", snap.Bytes)
	}
}

func TestSnapshotsAreFiledPerClusterAndPrivate(t *testing.T) {
	t.Parallel()
	st, _ := newStore(t)
	if _, err := put(t, st, "c1", "one"); err != nil {
		t.Fatal(err)
	}
	if snap, _ := st.Latest("c2"); snap.Present {
		t.Fatal("another cluster's snapshot counts for this one")
	}
	dir := filepath.Join(snapshotDir(st), "c1")
	di, _ := os.Stat(dir)
	if di.Mode().Perm() != 0o700 {
		t.Errorf("directory mode %v, want 0700", di.Mode().Perm())
	}
	entries, _ := os.ReadDir(dir)
	fi, _ := entries[0].Info()
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("file mode %v, want 0600", fi.Mode().Perm())
	}
}

func TestAClusterIdCannotEscapeTheDirectory(t *testing.T) {
	t.Parallel()
	st, _ := newStore(t)
	for _, id := range []string{"", ".", "..", "../x", `a\b`, "a/b"} {
		if _, err := put(t, st, id, "x"); err == nil {
			t.Errorf("cluster id %q was filed", id)
		}
		if _, err := st.Latest(model.ClusterID(id)); err == nil {
			t.Errorf("cluster id %q was looked up", id)
		}
	}
}

// RequireSafetySnapshot is the gate, and it fails closed in each way it can.
func TestRequireSafetySnapshotFailsClosed(t *testing.T) {
	t.Parallel()
	svc := newCheckService(nil, nil)

	if err := svc.RequireSafetySnapshot("c1"); !errors.Is(err, upgrade.ErrNoSnapshotStore) {
		t.Fatalf("no store: err = %v, want ErrNoSnapshotStore", err)
	}

	st, clk := newStore(t)
	svc.WithSnapshots(st)
	if err := svc.RequireSafetySnapshot("c1"); !errors.Is(err, upgrade.ErrSnapshotRequired) {
		t.Fatalf("no snapshot: err = %v, want ErrSnapshotRequired", err)
	}

	if _, err := put(t, st, "c1", "etcd"); err != nil {
		t.Fatal(err)
	}
	if err := svc.RequireSafetySnapshot("c1"); err != nil {
		t.Fatalf("fresh snapshot: %v", err)
	}
	if err := svc.RequireSafetySnapshot("c2"); !errors.Is(err, upgrade.ErrSnapshotRequired) {
		t.Fatalf("another cluster's snapshot passed this one: %v", err)
	}

	clk.now = clk.now.Add(2 * time.Hour)
	err := svc.RequireSafetySnapshot("c1")
	if !errors.Is(err, upgrade.ErrSnapshotRequired) {
		t.Fatalf("stale snapshot: err = %v, want ErrSnapshotRequired", err)
	}
	if !strings.Contains(err.Error(), "older than 60 minutes") {
		t.Errorf("the refusal does not say how fresh is fresh: %v", err)
	}
}

func TestTakeWithoutAStoreIsRefused(t *testing.T) {
	t.Parallel()
	svc := newCheckService(nil, nil)
	if _, err := svc.TakeSafetySnapshot(context.Background(), "c1"); !errors.Is(err, upgrade.ErrNoSnapshotStore) {
		t.Fatalf("err = %v, want ErrNoSnapshotStore", err)
	}
}

func TestOnlyPlainClusterIdsAreFiled(t *testing.T) {
	t.Parallel()
	st, _ := newStore(t)
	for _, id := range []string{".hidden", "a b", "a:b", strings.Repeat("x", 129), "ä", "a\nb"} {
		if _, err := put(t, st, id, "x"); err == nil {
			t.Errorf("cluster id %q was filed", id)
		}
	}
	for _, id := range []string{"c-homelab", "a_b.c-d", "0123", strings.Repeat("x", 128)} {
		if _, err := put(t, st, id, "x"); err != nil {
			t.Errorf("cluster id %q was refused: %v", id, err)
		}
	}
}

// One snapshot per cluster at a time, and the second is refused, not queued:
// a queue of requests that each still take one is disk spent by clicking.
func TestASecondSnapshotOfTheSameClusterIsRefusedWhileOneRuns(t *testing.T) {
	t.Parallel()
	st, _ := newStore(t)

	started, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := upgrade.WriteForTest(st, "c1", func(w io.Writer) (int64, error) {
			close(started)
			<-release
			n, err := io.WriteString(w, "etcd")
			return int64(n), err
		})
		done <- err
	}()
	<-started

	if _, err := put(t, st, "c1", "second"); !errors.Is(err, upgrade.ErrSnapshotInProgress) {
		t.Fatalf("second take of the same cluster: err = %v, want ErrSnapshotInProgress", err)
	}
	// Another cluster is not held up by it.
	if _, err := put(t, st, "c2", "other"); err != nil {
		t.Fatalf("a different cluster was held up: %v", err)
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("the first take: %v", err)
	}
	// And once it is done, the cluster can be snapshotted again.
	if _, err := put(t, st, "c1", "third"); err != nil {
		t.Fatalf("after it finished: %v", err)
	}
}

// Asking for a cluster that has no machines must not leave a directory for it.
func TestTakingForAnUnknownClusterCreatesNothing(t *testing.T) {
	t.Parallel()
	svc := newCheckService(nil, nil)
	st, _ := newStore(t)
	svc.WithSnapshots(st)

	if _, err := svc.TakeSafetySnapshot(context.Background(), "ghost"); err == nil {
		t.Fatal("a cluster with no machines was snapshotted")
	}
	if _, err := os.Stat(filepath.Join(snapshotDir(st), "ghost")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a directory was made for a cluster nobody has: %v", err)
	}
}

// The refusal reaches a browser, so it does not carry the path the disk
// complained about.
func TestTheRefusalDoesNotNameAPath(t *testing.T) {
	t.Parallel()
	ops := realOps()
	ops.List = func(string) ([]upgrade.DirFile, error) {
		return nil, errors.New("open /var/lib/holzkube-manager/upgrade-snapshots/c1: permission denied")
	}
	svc := newCheckService(nil, nil).WithSnapshots(upgrade.NewSnapshotStore(t.TempDir(), ops, nil))

	err := svc.RequireSafetySnapshot("c1")
	if !errors.Is(err, upgrade.ErrSnapshotRequired) {
		t.Fatalf("err = %v, want ErrSnapshotRequired", err)
	}
	if strings.Contains(err.Error(), "/var/lib") || strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("the refusal names the disk's complaint: %v", err)
	}
}
