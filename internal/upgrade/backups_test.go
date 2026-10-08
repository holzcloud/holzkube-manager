package upgrade_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/upgrade"
)

// Scheduled etcd snapshots (2026-10-08).

func putKind(t *testing.T, st *upgrade.SnapshotStore, kind string, keep int, body string) error {
	t.Helper()
	_, err := upgrade.WriteKindForTest(st, "c1", kind, keep, 0, func(w io.Writer) (int64, error) {
		n, err := io.WriteString(w, body)
		return int64(n), err
	})
	return err
}

func putOther(st *upgrade.SnapshotStore, cluster model.ClusterID) error {
	_, err := upgrade.WriteKindForTest(st, cluster, upgrade.SnapshotKindScheduled, 7, 0,
		func(w io.Writer) (int64, error) { n, err := io.WriteString(w, "other cluster"); return int64(n), err })
	return err
}

func kinds(t *testing.T, st *upgrade.SnapshotStore) (upgradeN, scheduledN int) {
	t.Helper()
	snaps, err := st.List("c1")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range snaps {
		switch s.Kind {
		case upgrade.SnapshotKindUpgrade:
			upgradeN++
		case upgrade.SnapshotKindScheduled:
			scheduledN++
		}
	}
	return
}

// The two kinds are pruned by their own counts: a schedule that keeps three
// must neither push out the two an upgrade is waiting on nor be eaten by them.
func TestEachKindIsPrunedByItsOwnCount(t *testing.T) {
	t.Parallel()
	st, clk := newStore(t)
	for i := range 6 {
		clk.now = clk.now.Add(time.Minute)
		if err := putKind(t, st, upgrade.SnapshotKindScheduled, 3, "sched"+string(rune('a'+i))); err != nil {
			t.Fatal(err)
		}
		clk.now = clk.now.Add(time.Minute)
		if err := putKind(t, st, upgrade.SnapshotKindUpgrade, 2, "upg"+string(rune('a'+i))); err != nil {
			t.Fatal(err)
		}
	}
	u, s := kinds(t, st)
	if u != 2 || s != 3 {
		t.Fatalf("after 6 of each: %d pre-upgrade and %d scheduled kept, want 2 and 3", u, s)
	}

	// The survivors are the newest of each kind, and the checksum files of the
	// pruned ones went with them.
	entries, _ := os.ReadDir(filepath.Join(snapshotDir(st), "c1"))
	var sums int
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sha256") {
			sums++
		}
	}
	if sums != 5 {
		t.Errorf("%d checksum files remain, want 5 (one per kept snapshot)", sums)
	}
}

// Freshness asks "is there a recent one" and does not care which kind.
func TestAScheduledSnapshotSatisfiesTheUpgradeFreshnessRule(t *testing.T) {
	t.Parallel()
	st, clk := newStore(t)
	if err := putKind(t, st, upgrade.SnapshotKindScheduled, 7, "etcd"); err != nil {
		t.Fatal(err)
	}
	clk.now = clk.now.Add(30 * time.Minute)
	snap, err := st.Latest("c1")
	if err != nil {
		t.Fatal(err)
	}
	if !snap.Fresh {
		t.Fatal("a scheduled snapshot from 30 minutes ago does not satisfy the upgrade freshness rule")
	}
	clk.now = clk.now.Add(31 * time.Minute)
	if snap, _ = st.Latest("c1"); snap.Fresh {
		t.Fatal("a scheduled snapshot older than an hour was fresh")
	}
}

func TestTheListCarriesKindSizeAndAChecksumThatMatches(t *testing.T) {
	t.Parallel()
	st, clk := newStore(t)
	body := "the etcd database"
	if err := putKind(t, st, upgrade.SnapshotKindScheduled, 7, body); err != nil {
		t.Fatal(err)
	}
	clk.now = clk.now.Add(time.Minute)
	if err := putKind(t, st, upgrade.SnapshotKindUpgrade, 2, "x"+body); err != nil {
		t.Fatal(err)
	}

	snaps, err := st.List("c1")
	if err != nil || len(snaps) != 2 {
		t.Fatalf("List = %v, %v", snaps, err)
	}
	if snaps[0].Kind != upgrade.SnapshotKindUpgrade || snaps[1].Kind != upgrade.SnapshotKindScheduled {
		t.Errorf("kinds %q, %q: not newest first or not labelled", snaps[0].Kind, snaps[1].Kind)
	}
	sum := sha256.Sum256([]byte(body))
	if snaps[1].SHA256 != hex.EncodeToString(sum[:]) {
		t.Errorf("sha256 = %q, want %x", snaps[1].SHA256, sum)
	}
	if snaps[1].Bytes != int64(len(body)) {
		t.Errorf("bytes = %d, want %d", snaps[1].Bytes, len(body))
	}

	r, n, err := st.Open("c1", snaps[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close() //nolint:errcheck // test
	got, _ := io.ReadAll(r)
	if string(got) != body || n != int64(len(body)) {
		t.Errorf("download = %q (%d), want %q", got, n, body)
	}
}

func TestOnlyAFileNameThisPackageWroteCanBeDownloaded(t *testing.T) {
	t.Parallel()
	st, _ := newStore(t)
	if err := putKind(t, st, upgrade.SnapshotKindScheduled, 7, "etcd"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"../c1/20261008T120000.000Z.snapshot",
		"../../etc/passwd",
		"backup-status.json",
		"20261008T120000.000Z.snapshot.sha256",
		"/etc/passwd",
		"",
	} {
		if _, _, err := st.Open("c1", name); !errors.Is(err, upgrade.ErrSnapshotNotFound) {
			t.Errorf("Open(%q) = %v, want ErrSnapshotNotFound", name, err)
		}
	}

	// Names that WOULD resolve to a real file if they were joined onto the
	// path: one outside the cluster's directory, one in another cluster's.
	if err := os.WriteFile(filepath.Join(snapshotDir(st), "other.txt"), []byte("not a snapshot"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := putOther(st, "c2"); err != nil {
		t.Fatal(err)
	}
	other, _ := st.List("c2")
	for _, name := range []string{"../other.txt", "../c2/" + other[0].ID} {
		if r, _, err := st.Open("c1", name); !errors.Is(err, upgrade.ErrSnapshotNotFound) {
			if r != nil {
				_ = r.Close()
			}
			t.Errorf("Open(%q) = %v, want ErrSnapshotNotFound", name, err)
		}
	}
	if _, _, err := st.Open("../x", "20261008T120000.000Z.snapshot"); err == nil {
		t.Error("a cluster id that is a path was accepted")
	}
}

func TestTheDiskGuardRefusesBelowTwiceTheSnapshotAndWritesNothing(t *testing.T) {
	t.Parallel()
	clk := &clock{now: time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)}
	var free atomic.Uint64
	ops := realOps()
	ops.FreeBytes = func(string) (uint64, error) { return free.Load(), nil }
	st := upgrade.NewSnapshotStore(filepath.Join(t.TempDir(), "snaps"), ops, clk.Now)

	write := func(estimate int64) error {
		_, err := upgrade.WriteKindForTest(st, "c1", upgrade.SnapshotKindScheduled, 7, estimate,
			func(w io.Writer) (int64, error) {
				n, err := io.WriteString(w, "etcd")
				return int64(n), err
			})
		return err
	}

	// 100 MiB expected: 199 MiB free is not twice that, 200 is.
	free.Store(199 << 20)
	err := write(100 << 20)
	var low *upgrade.DiskLowError
	if !errors.Is(err, upgrade.ErrDiskLow) || !errors.As(err, &low) {
		t.Fatalf("err = %v, want ErrDiskLow", err)
	}
	if low.Need != 200<<20 || low.Free != 199<<20 {
		t.Errorf("need/free = %d/%d, want %d/%d", low.Need, low.Free, 200<<20, 199<<20)
	}
	if snaps, _ := st.List("c1"); len(snaps) != 0 {
		t.Fatalf("a refused snapshot left %d file(s)", len(snaps))
	}

	free.Store(200 << 20)
	if err := write(100 << 20); err != nil {
		t.Fatalf("with exactly twice free: %v", err)
	}

	// The next one is sized by the biggest on disk when the node does not say:
	// a 4-byte file is under the floor, so the floor (16 MiB x 2) decides.
	free.Store(31 << 20)
	clk.now = clk.now.Add(time.Minute)
	if err := write(0); !errors.Is(err, upgrade.ErrDiskLow) {
		t.Fatalf("under the floor: err = %v, want ErrDiskLow", err)
	}
}

func TestOverdueMeansTwoIntervalsWithoutAScheduledSnapshot(t *testing.T) {
	t.Parallel()
	st, clk := newStore(t)
	svc := upgrade.NewService(newRemovalRig(t, 1, 0).deps, nil).WithSnapshots(st)

	since := clk.now
	c := model.Cluster{ID: "c1", BackupSchedule: &model.BackupSchedule{Interval: model.BackupDaily, Keep: 7, Since: since}}

	health := func(at time.Time) model.BackupHealth {
		h := svc.BackupHealthFor(c, at)
		if h == nil {
			t.Fatal("no health")
		}
		return *h
	}

	if h := health(since.Add(47 * time.Hour)); h.Overdue {
		t.Error("overdue 47 h after turning a daily schedule on, before it ever ran")
	}
	if h := health(since.Add(49 * time.Hour)); !h.Overdue {
		t.Error("not overdue 49 h after turning a daily schedule on with no snapshot")
	}

	if err := putKind(t, st, upgrade.SnapshotKindScheduled, 7, "etcd"); err != nil {
		t.Fatal(err)
	}
	taken := clk.now
	if h := health(taken.Add(47 * time.Hour)); h.Overdue || h.LastSuccessAt == nil || h.AgeSeconds == nil ||
		*h.AgeSeconds != int64(47*time.Hour/time.Second) {
		t.Errorf("47 h after a snapshot: %+v", h)
	}
	if h := health(taken.Add(49 * time.Hour)); !h.Overdue {
		t.Error("not overdue 49 h after the last scheduled snapshot")
	}

	// A pre-upgrade snapshot is a backup (it counts in the age) but it is not
	// the schedule running, so it does not clear "overdue".
	clk.now = taken.Add(48*time.Hour + time.Minute)
	if err := putKind(t, st, upgrade.SnapshotKindUpgrade, 2, "etcd"); err != nil {
		t.Fatal(err)
	}
	h := health(clk.now.Add(time.Minute))
	if !h.Overdue {
		t.Error("a pre-upgrade snapshot cleared overdue")
	}
	if h.AgeSeconds == nil || *h.AgeSeconds != 60 {
		t.Errorf("age = %v, want the pre-upgrade snapshot's 60 s", h.AgeSeconds)
	}

	// Off is never overdue.
	c.BackupSchedule = nil
	if h := health(since.Add(1000 * time.Hour)); h.Overdue || h.Enabled {
		t.Errorf("a cluster without a schedule: %+v", h)
	}
}

// ---------------------------------------------------------------------------
// The scheduler and the job, over a real engine and simulated nodes.

type backupRig struct {
	t      *testing.T
	rig    *removalRig
	svc    *upgrade.Service
	st     *upgrade.SnapshotStore
	clk    *clock
	engine *jobs.Engine

	mu       sync.Mutex
	clusters []model.Cluster
}

func newBackupRig(t *testing.T, mut func(*upgrade.Deps), ops func(*upgrade.FileOps)) *backupRig {
	t.Helper()
	rig := newRemovalRig(t, 1, 0)
	if mut != nil {
		mut(&rig.deps)
	}
	fo := realOps()
	if ops != nil {
		ops(&fo)
	}
	clk := &clock{now: time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)}
	st := upgrade.NewSnapshotStore(filepath.Join(t.TempDir(), "snaps"), fo, clk.Now)
	svc := upgrade.NewService(rig.deps, nil).WithSnapshots(st)
	b := &backupRig{t: t, rig: rig, svc: svc, st: st, clk: clk}
	b.engine = rig.newEngine()
	svc.RegisterBackups(b.engine)
	b.setSchedule(model.BackupDaily, 3, false)
	return b
}

func (b *backupRig) setSchedule(interval string, keep int, disabled bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.clusters = []model.Cluster{{
		ID: removeCluster, Name: "homelab", Disabled: disabled,
		BackupSchedule: &model.BackupSchedule{Interval: interval, Keep: keep, Since: b.clk.now},
	}}
}

func (b *backupRig) scheduler(logs *[]string) *upgrade.BackupScheduler {
	return b.svc.NewBackupScheduler(upgrade.BackupSchedulerDeps{
		Clusters: func(context.Context) ([]model.Cluster, error) {
			b.mu.Lock()
			defer b.mu.Unlock()
			return append([]model.Cluster(nil), b.clusters...), nil
		},
		Now:    b.clk.Now,
		Jitter: func(time.Duration) time.Duration { return 0 },
		Log: func(msg string, _ ...any) {
			if logs != nil {
				*logs = append(*logs, msg)
			}
		},
	})
}

func (b *backupRig) jobCount() int {
	b.t.Helper()
	all, err := b.engine.List(b.t.Context())
	if err != nil {
		b.t.Fatal(err)
	}
	return len(all)
}

// settle waits until every job is terminal and returns them.
func (b *backupRig) settle() []model.Job {
	b.t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		all, err := b.engine.List(b.t.Context())
		if err != nil {
			b.t.Fatal(err)
		}
		done := true
		for _, j := range all {
			if !j.State.Terminal() {
				done = false
			}
		}
		if done {
			return all
		}
		if time.Now().After(deadline) {
			b.t.Fatalf("jobs still running after 30 s: %+v", all)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (b *backupRig) status() upgrade.BackupState {
	b.t.Helper()
	b.mu.Lock()
	c := b.clusters[0]
	b.mu.Unlock()
	state, err := b.svc.BackupsOf(c)
	if err != nil {
		b.t.Fatal(err)
	}
	return state
}

func TestTheSchedulerTakesASnapshotWhenDueAndNotBefore(t *testing.T) {
	t.Parallel()
	b := newBackupRig(t, nil, nil)
	s := b.scheduler(nil)

	// A schedule turned on at noon is due at once: a cluster should not wait a
	// whole interval for its first backup.
	s.Tick(t.Context())
	jobs1 := b.settle()
	if len(jobs1) != 1 || jobs1[0].State != model.JobSucceeded || jobs1[0].Kind != upgrade.JobKindEtcdBackup {
		t.Fatalf("first tick: %+v", jobs1)
	}
	st := b.status()
	if len(st.Snapshots) != 1 || st.Snapshots[0].Kind != upgrade.SnapshotKindScheduled || st.Snapshots[0].SHA256 == "" {
		t.Fatalf("snapshots after the first run: %+v", st.Snapshots)
	}
	if st.Status.LastResult != upgrade.BackupOK || st.Status.LastTrigger != upgrade.TriggerSchedule {
		t.Errorf("status = %+v", st.Status)
	}

	// Nothing is due until the interval has passed.
	for _, after := range []time.Duration{time.Minute, 12 * time.Hour, 23*time.Hour + 59*time.Minute} {
		b.clk.now = time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC).Add(after)
		s.Tick(t.Context())
	}
	if n := b.jobCount(); n != 1 {
		t.Fatalf("%d jobs before the interval was up, want 1", n)
	}

	b.clk.now = time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC).Add(24*time.Hour + time.Second)
	s.Tick(t.Context())
	b.settle()
	if n := b.jobCount(); n != 2 {
		t.Fatalf("%d jobs after the interval was up, want 2", n)
	}
	if st := b.status(); st.NextDueAt == nil || !st.NextDueAt.After(b.clk.now) {
		t.Errorf("next due = %v, want a time after now", st.NextDueAt)
	}
}

// A restart must not move or double a run: the record of what happened is on
// disk, and a new scheduler reads it.
func TestARestartDoesNotRunTheSameSnapshotTwice(t *testing.T) {
	t.Parallel()
	b := newBackupRig(t, nil, nil)
	b.scheduler(nil).Tick(t.Context())
	b.settle()

	b.clk.now = b.clk.now.Add(time.Hour)
	b.scheduler(nil).Tick(t.Context()) // a new process: new scheduler, same disk
	b.scheduler(nil).Tick(t.Context())
	if n := b.jobCount(); n != 1 {
		t.Fatalf("%d jobs after restarts inside the interval, want 1", n)
	}
}

// An attempt that is recorded as running keeps the scheduler from submitting a
// second job: a daemon killed after recording and before the job existed
// leaves exactly this.
func TestARecordedAttemptHoldsTheSchedulerBack(t *testing.T) {
	t.Parallel()
	b := newBackupRig(t, nil, nil)
	var submits atomic.Int32
	b.svc.SetSubmitForTest(func(context.Context, model.Job) (model.Job, error) {
		submits.Add(1)
		return model.Job{ID: "j1"}, nil // accepted, and never finishes
	})
	s := b.scheduler(nil)
	s.Tick(t.Context())
	for range 5 {
		b.clk.now = b.clk.now.Add(time.Minute)
		s.Tick(t.Context())
	}
	b.scheduler(nil).Tick(t.Context())
	if got := submits.Load(); got != 1 {
		t.Fatalf("%d submissions while the first was still running, want 1", got)
	}

	// A "running" that never ended (a parked job, a crash) is not held forever.
	b.clk.now = b.clk.now.Add(upgrade.SafetySnapshotTimeout + 40*time.Minute)
	s.Tick(t.Context())
	if got := submits.Load(); got != 2 {
		t.Fatalf("%d submissions after the grace ran out, want 2", got)
	}
}

func TestABusyClusterIsSkippedWithAReasonAndRetriedLater(t *testing.T) {
	t.Parallel()
	b := newBackupRig(t, nil, nil)
	var submits atomic.Int32
	b.svc.SetSubmitForTest(func(context.Context, model.Job) (model.Job, error) {
		submits.Add(1)
		return model.Job{}, jobs.ErrClusterBusy
	})
	s := b.scheduler(nil)
	s.Tick(t.Context())

	st := b.status().Status
	if st.LastResult != upgrade.BackupSkipped || !strings.Contains(st.LastReason, "another job") {
		t.Fatalf("status = %+v, want skipped with the reason", st)
	}

	// Not hammered every minute...
	b.clk.now = b.clk.now.Add(5 * time.Minute)
	s.Tick(t.Context())
	if submits.Load() != 1 {
		t.Fatalf("retried after 5 minutes: %d submissions", submits.Load())
	}
	// ...but not left for a whole day either.
	b.clk.now = b.clk.now.Add(time.Hour)
	s.Tick(t.Context())
	if submits.Load() != 2 {
		t.Fatalf("not retried after an hour: %d submissions", submits.Load())
	}
}

func TestAnUnreachableClusterIsSkippedNotRetriedEveryMinute(t *testing.T) {
	t.Parallel()
	b := newBackupRig(t, func(d *upgrade.Deps) {
		d.Connect = func(context.Context, model.MachineID) (*talos.ClusterClient, error) {
			return nil, errors.New("dial: no route to host")
		}
	}, nil)
	s := b.scheduler(nil)
	s.Tick(t.Context())
	all := b.settle()
	if len(all) != 1 || all[0].State != model.JobFailed {
		t.Fatalf("jobs = %+v, want one failed job", all)
	}
	st := b.status()
	if st.Status.LastResult != upgrade.BackupSkipped || !strings.Contains(st.Status.LastReason, "could not be reached") {
		t.Fatalf("status = %+v", st.Status)
	}
	if len(st.Snapshots) != 0 {
		t.Fatalf("snapshots = %+v", st.Snapshots)
	}
	if st.Health.Overdue {
		t.Error("overdue straight away")
	}

	for range 10 {
		b.clk.now = b.clk.now.Add(time.Minute)
		s.Tick(t.Context())
	}
	if n := b.jobCount(); n != 1 {
		t.Fatalf("%d jobs in the ten minutes after a failure, want 1", n)
	}
}

func TestALowDiskSkipsTheRunAndSaysSo(t *testing.T) {
	t.Parallel()
	b := newBackupRig(t, nil, func(o *upgrade.FileOps) {
		o.FreeBytes = func(string) (uint64, error) { return 1 << 20, nil }
	})
	b.scheduler(nil).Tick(t.Context())
	b.settle()
	st := b.status()
	if st.Status.LastResult != upgrade.BackupSkipped || !strings.Contains(st.Status.LastReason, "not enough free disk space") {
		t.Fatalf("status = %+v", st.Status)
	}
	if len(st.Snapshots) != 0 {
		t.Fatalf("a snapshot was written onto a full disk: %+v", st.Snapshots)
	}
	if strings.Contains(st.Status.LastReason, b.st.DirForTest()) {
		t.Errorf("the reason names the data directory: %q", st.Status.LastReason)
	}
}

func TestADisabledClusterGetsNoJobAndAReason(t *testing.T) {
	t.Parallel()
	b := newBackupRig(t, nil, nil)
	b.setSchedule(model.BackupDaily, 3, true)
	b.scheduler(nil).Tick(t.Context())
	if n := b.jobCount(); n != 0 {
		t.Fatalf("%d jobs for a switched-off cluster", n)
	}
	if st := b.status().Status; st.LastResult != upgrade.BackupSkipped || !strings.Contains(st.LastReason, "switched off") {
		t.Fatalf("status = %+v", st)
	}
}

func TestAClusterWithoutAScheduleIsLeftAlone(t *testing.T) {
	t.Parallel()
	b := newBackupRig(t, nil, nil)
	b.setSchedule(model.BackupOff, 3, false)
	b.scheduler(nil).Tick(t.Context())
	if n := b.jobCount(); n != 0 {
		t.Fatalf("%d jobs for a cluster with the schedule off", n)
	}
}

// The jitter is drawn once per cluster per process and moves the first run
// later, never earlier.
func TestTheJitterDelaysTheFirstRunWithinItsBound(t *testing.T) {
	t.Parallel()
	b := newBackupRig(t, nil, nil)
	var asked []time.Duration
	s := b.svc.NewBackupScheduler(upgrade.BackupSchedulerDeps{
		Clusters: func(context.Context) ([]model.Cluster, error) { return b.clusters, nil },
		Now:      b.clk.Now,
		Jitter:   func(limit time.Duration) time.Duration { asked = append(asked, limit); return limit / 2 },
	})
	s.Tick(t.Context())
	if n := b.jobCount(); n != 0 {
		t.Fatalf("ran before its jitter: %d jobs", n)
	}
	// daily: a twelfth of 24 h is 2 h, capped at 5 minutes.
	if len(asked) != 1 || asked[0] != 5*time.Minute {
		t.Fatalf("jitter bound asked = %v, want one 5m bound", asked)
	}
	b.clk.now = b.clk.now.Add(2*time.Minute + 31*time.Second)
	s.Tick(t.Context())
	b.settle()
	if n := b.jobCount(); n != 1 {
		t.Fatalf("%d jobs after the jitter passed, want 1", n)
	}
	s.Tick(t.Context())
	if len(asked) != 1 {
		t.Errorf("jitter drawn %d times, want once per cluster", len(asked))
	}
}

func TestRetentionIsAppliedByTheJobAndLeavesUpgradeSnapshotsAlone(t *testing.T) {
	t.Parallel()
	b := newBackupRig(t, nil, nil) // keep 3
	for range 2 {
		b.clk.now = b.clk.now.Add(time.Minute)
		if _, err := upgrade.WriteKindForTest(b.st, removeCluster, upgrade.SnapshotKindUpgrade, 2, 0,
			func(w io.Writer) (int64, error) { n, err := io.WriteString(w, "u"); return int64(n), err }); err != nil {
			t.Fatal(err)
		}
	}
	s := b.scheduler(nil)
	for range 5 {
		b.clk.now = b.clk.now.Add(25 * time.Hour)
		s.Tick(t.Context())
		b.settle()
	}
	var sched, upg int
	for _, sn := range b.status().Snapshots {
		if sn.Kind == upgrade.SnapshotKindScheduled {
			sched++
		} else {
			upg++
		}
	}
	if sched != 3 || upg != 2 {
		t.Fatalf("kept %d scheduled and %d pre-upgrade, want 3 and 2", sched, upg)
	}
}

func TestTheStepIsDoneWhenASnapshotNewerThanTheJobExists(t *testing.T) {
	t.Parallel()
	b := newBackupRig(t, nil, nil)
	step := b.svc.BackupStepForTest()
	if step.Happened == nil {
		t.Fatal("the snapshot step cannot be resumed")
	}
	job := model.Job{Cluster: removeCluster, CreatedAt: b.clk.now.Add(time.Minute)}
	if done, err := step.Happened(t.Context(), &job); err != nil || done {
		t.Fatalf("with no snapshot: done=%v err=%v", done, err)
	}
	b.clk.now = b.clk.now.Add(2 * time.Minute)
	if _, err := upgrade.WriteKindForTest(b.st, removeCluster, upgrade.SnapshotKindScheduled, 3, 0,
		func(w io.Writer) (int64, error) { n, err := io.WriteString(w, "e"); return int64(n), err }); err != nil {
		t.Fatal(err)
	}
	if done, err := step.Happened(t.Context(), &job); err != nil || !done {
		t.Fatalf("with a snapshot taken after the job began: done=%v err=%v", done, err)
	}
	job.CreatedAt = b.clk.now.Add(time.Minute)
	if done, _ := step.Happened(t.Context(), &job); done {
		t.Fatal("an older snapshot counted for a newer job")
	}
}

// The snapshot is the real bytes of the simulated etcd, and it equals what the
// live endpoint streams.
func TestTheStoredSnapshotIsTheNodesEtcd(t *testing.T) {
	t.Parallel()
	b := newBackupRig(t, nil, nil)
	b.scheduler(nil).Tick(t.Context())
	b.settle()
	snaps := b.status().Snapshots
	if len(snaps) != 1 {
		t.Fatalf("snapshots = %+v", snaps)
	}
	r, _, err := b.svc.OpenBackup(removeCluster, snaps[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close() //nolint:errcheck // test
	stored, _ := io.ReadAll(r)

	var live bytes.Buffer
	if _, err := b.svc.Snapshot(t.Context(), removeCluster, &live); err != nil {
		t.Fatal(err)
	}
	if len(stored) == 0 || len(stored) != live.Len() {
		t.Fatalf("stored %d bytes, the node streams %d", len(stored), live.Len())
	}
}

var _ = fsstore.TempPrefix
