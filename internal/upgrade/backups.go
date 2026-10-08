package upgrade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/rand/v2"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// Scheduled etcd snapshots (2026-10-08).
//
// Nothing here is a second mechanism. A scheduled snapshot is written by the
// same SnapshotStore the pre-upgrade snapshot uses, into the same per-cluster
// directory, with the same atomic write; it differs by a name (see
// SnapshotKindScheduled) and by being taken as a job, so that it takes the
// cluster's one lease like everything else that touches a cluster and shows up
// in the jobs list.
//
// The honest limit, stated here and in the guide: these files live on the
// device the manager runs on. They survive a bad upgrade and a lost etcd; they
// do not survive the loss of that device. Copy the ones that matter off the box.

// JobKindEtcdBackup is a scheduled (or run-now) snapshot.
const JobKindEtcdBackup = model.JobKind("cluster.etcd-backup")

// Errors the backup path reports.
var (
	// ErrDiskLow reports a snapshot refused because the data directory does not
	// have twice the snapshot's size free. A snapshot that fills the disk takes
	// the daemon's own state down with it, which is a worse day than a missing
	// backup.
	ErrDiskLow = errors.New("upgrade: not enough free disk space for a snapshot")

	// ErrBackupSkipped marks a run that was not attempted for a stated reason:
	// the cluster could not be reached, or something else held it.
	ErrBackupSkipped = errors.New("upgrade: snapshot skipped")

	// ErrSnapshotNotFound reports a download of a snapshot that is not there.
	ErrSnapshotNotFound = errors.New("upgrade: no such snapshot")

	// ErrDownloadUnavailable reports an instance that cannot stream a stored
	// snapshot.
	ErrDownloadUnavailable = errors.New("upgrade: this instance cannot hand out stored snapshots")

	// ErrBackupsNotWired reports a service built without the means to run one.
	ErrBackupsNotWired = errors.New("upgrade: this instance was started without scheduled snapshots")
)

// DiskLowError carries the numbers behind ErrDiskLow.
type DiskLowError struct {
	Free, Need uint64
}

func (e *DiskLowError) Error() string {
	return fmt.Sprintf("%s: %s free, %s needed (twice the expected snapshot size)",
		ErrDiskLow.Error(), humanBytes(e.Free), humanBytes(e.Need))
}

// Is lets errors.Is(err, ErrDiskLow) match.
func (e *DiskLowError) Is(target error) bool { return target == ErrDiskLow }

func humanBytes(n uint64) string {
	const mib = 1 << 20
	if n >= 1<<30 {
		return fmt.Sprintf("%.1f GiB", float64(n)/(1<<30))
	}
	return fmt.Sprintf("%.0f MiB", float64(n)/mib)
}

// minSnapshotEstimate is what the disk guard assumes a snapshot takes when
// nothing better is known (no snapshot yet and the node did not say). An etcd
// is rarely smaller.
const minSnapshotEstimate = 16 << 20

// guardDisk refuses unless twice the expected snapshot is free. The estimate
// is the largest of what the caller knows (the node's database size), the
// biggest snapshot already stored for the cluster, and a floor.
func (st *SnapshotStore) guardDisk(cluster model.ClusterID, estimate int64) error {
	if st.fs.FreeBytes == nil {
		return nil
	}
	est := max(estimate, minSnapshotEstimate)
	if snaps, err := st.List(cluster); err == nil {
		for _, s := range snaps {
			est = max(est, s.Bytes)
		}
	}
	free, err := st.fs.FreeBytes(st.dir)
	if err != nil {
		// Not being able to look is not a reason to take no backup.
		return nil //nolint:nilerr // deliberate: an unreadable gauge must not stop a backup
	}
	need := uint64(est) * 2
	if free < need {
		return &DiskLowError{Free: free, Need: need}
	}
	return nil
}

// StoredSnapshot is one file in a cluster's snapshot directory.
type StoredSnapshot struct {
	// ID is the file name, which is also what a download names. It is checked
	// against the strict name grammar before it becomes a path.
	ID      string    `json:"id"`
	Kind    string    `json:"kind"`
	TakenAt time.Time `json:"taken_at"`
	Bytes   int64     `json:"bytes"`

	// SHA256 is empty for a snapshot written before checksums were kept.
	SHA256 string `json:"sha256,omitempty"`
}

// List returns a cluster's stored snapshots, newest first.
func (st *SnapshotStore) List(cluster model.ClusterID) ([]StoredSnapshot, error) {
	dir, err := st.clusterDir(cluster)
	if err != nil {
		return nil, err
	}
	entries, err := st.fs.List(dir)
	if err != nil {
		return nil, err
	}
	var out []StoredSnapshot
	for _, e := range entries {
		at, kind, ok := parseSnapshotName(e.Name)
		if !ok || e.IsDir || e.Size == 0 {
			continue
		}
		out = append(out, StoredSnapshot{
			ID: e.Name, Kind: kind, TakenAt: at, Bytes: e.Size,
			SHA256: st.readChecksum(filepath.Join(dir, e.Name)),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TakenAt.After(out[j].TakenAt) })
	return out, nil
}

func (st *SnapshotStore) readChecksum(path string) string {
	if st.fs.ReadFile == nil {
		return ""
	}
	b, err := st.fs.ReadFile(path + checksumSuffix)
	if err != nil {
		return ""
	}
	f := strings.Fields(string(b))
	if len(f) == 0 || len(f[0]) != 64 {
		return ""
	}
	return strings.ToLower(f[0])
}

// Open opens one stored snapshot for download.
func (st *SnapshotStore) Open(cluster model.ClusterID, id string) (io.ReadCloser, int64, error) {
	dir, err := st.clusterDir(cluster)
	if err != nil {
		return nil, 0, err
	}
	// The id comes from a URL and becomes a path: only a name this package
	// would have written is accepted, so no separator and no ".." gets in.
	if _, _, ok := parseSnapshotName(id); !ok || id != filepath.Base(id) {
		return nil, 0, ErrSnapshotNotFound
	}
	if st.fs.Open == nil {
		return nil, 0, ErrDownloadUnavailable
	}
	r, n, err := st.fs.Open(filepath.Join(dir, id))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, 0, ErrSnapshotNotFound
	}
	return r, n, err
}

// BackupStatus is the record of the last attempt, kept next to the snapshots.
// It is what lets a restart see that a run was already made, and what the
// screen shows when the answer to "did last night's backup run" is no.
type BackupStatus struct {
	LastAttemptAt time.Time   `json:"last_attempt_at,omitzero"`
	LastTrigger   string      `json:"last_trigger,omitempty"` // schedule | manual
	LastResult    string      `json:"last_result,omitempty"`  // running | ok | skipped | failed
	LastReason    string      `json:"last_reason,omitempty"`
	LastJob       model.JobID `json:"last_job,omitempty"`
}

// Attempt results.
const (
	BackupRunning = "running"
	BackupOK      = "ok"
	BackupSkipped = "skipped"
	BackupFailed  = "failed"
)

const statusFile = "backup-status.json"

func (st *SnapshotStore) readStatus(cluster model.ClusterID) BackupStatus {
	var s BackupStatus
	dir, err := st.clusterDir(cluster)
	if err != nil || st.fs.ReadFile == nil {
		return s
	}
	b, err := st.fs.ReadFile(filepath.Join(dir, statusFile))
	if err != nil {
		return s
	}
	_ = json.Unmarshal(b, &s)
	return s
}

// updateStatus is a read-modify-write under a lock: the scheduler and the job
// both write it.
func (st *SnapshotStore) updateStatus(cluster model.ClusterID, mut func(*BackupStatus)) {
	dir, err := st.clusterDir(cluster)
	if err != nil {
		return
	}
	st.statusMu.Lock()
	defer st.statusMu.Unlock()
	s := st.readStatus(cluster)
	mut(&s)
	b, err := json.Marshal(s)
	if err != nil {
		return
	}
	// Best effort: a status that cannot be written costs the screen a line,
	// not the snapshot.
	_, _ = st.fs.WriteStream(filepath.Join(dir, statusFile), func(w io.Writer) (int64, error) {
		n, werr := w.Write(b)
		return int64(n), werr
	})
}

// healthOf derives a cluster's backup health from what is on disk.
func healthOf(c model.Cluster, snaps []StoredSnapshot, st BackupStatus, now time.Time) model.BackupHealth {
	h := model.BackupHealth{}
	if c.BackupSchedule != nil {
		h.Interval = c.BackupSchedule.Interval
	}
	h.Enabled = c.BackupSchedule.Enabled()

	var lastScheduled time.Time
	for i, s := range snaps { // newest first
		if i == 0 {
			t := s.TakenAt
			h.NewestAt = &t
			age := int64(now.Sub(t).Seconds())
			age = max(age, 0)
			h.AgeSeconds = &age
		}
		if s.Kind == SnapshotKindScheduled && lastScheduled.IsZero() {
			lastScheduled = s.TakenAt
			t := s.TakenAt
			h.LastSuccessAt = &t
		}
	}

	if h.Enabled {
		iv, _ := model.BackupInterval(c.BackupSchedule.Interval)
		base := lastScheduled
		if base.IsZero() {
			base = c.BackupSchedule.Since
		}
		h.Overdue = now.After(base.Add(2 * iv))
	}

	h.LastResult, h.LastReason = st.LastResult, st.LastReason
	// A scheduled snapshot at least as new as the last attempt is the result,
	// whatever the record says: the job may have been killed between writing
	// the file and writing the record.
	if !lastScheduled.IsZero() && !lastScheduled.Before(st.LastAttemptAt) {
		h.LastResult, h.LastReason = BackupOK, ""
	}
	return h
}

// BackupHealthFor is what the overview and the metrics read. It is nil when
// this instance keeps no snapshots.
func (s *Service) BackupHealthFor(c model.Cluster, now time.Time) *model.BackupHealth {
	if s.snapshots == nil {
		return nil
	}
	snaps, _ := s.snapshots.List(c.ID)
	h := healthOf(c, snaps, s.snapshots.readStatus(c.ID), now)
	return &h
}

// BackupState is what the Backups section is drawn from.
type BackupState struct {
	Snapshots []StoredSnapshot   `json:"snapshots"`
	Status    BackupStatus       `json:"status"`
	Health    model.BackupHealth `json:"health"`

	// NextDueAt is when the schedule will next try, absent when it is off.
	NextDueAt *time.Time `json:"next_due_at,omitempty"`

	// FreeBytes is the free space where the snapshots are kept; absent when
	// this instance cannot tell.
	FreeBytes *uint64 `json:"free_bytes,omitempty"`
}

// BackupsOf reports a cluster's stored snapshots and the standing of its
// schedule.
func (s *Service) BackupsOf(c model.Cluster) (BackupState, error) {
	if s.snapshots == nil {
		return BackupState{}, ErrNoSnapshotStore
	}
	snaps, err := s.snapshots.List(c.ID)
	if err != nil {
		return BackupState{}, err
	}
	now := s.snapshots.now()
	st := s.snapshots.readStatus(c.ID)
	out := BackupState{Snapshots: snaps, Status: st, Health: healthOf(c, snaps, st, now)}
	if out.Snapshots == nil {
		out.Snapshots = []StoredSnapshot{}
	}
	if c.BackupSchedule.Enabled() {
		t := nextDue(c.BackupSchedule, newestScheduled(snaps), st, 0)
		out.NextDueAt = &t
	}
	if s.snapshots.fs.FreeBytes != nil {
		if free, err := s.snapshots.fs.FreeBytes(s.snapshots.dir); err == nil {
			out.FreeBytes = &free
		}
	}
	return out, nil
}

// OpenBackup opens a stored snapshot for download.
func (s *Service) OpenBackup(cluster model.ClusterID, id string) (io.ReadCloser, int64, error) {
	if s.snapshots == nil {
		return nil, 0, ErrNoSnapshotStore
	}
	return s.snapshots.Open(cluster, id)
}

func newestScheduled(snaps []StoredSnapshot) time.Time {
	for _, s := range snaps {
		if s.Kind == SnapshotKindScheduled {
			return s.TakenAt
		}
	}
	return time.Time{}
}

// retryGap is how long a failed or skipped attempt waits before the scheduler
// tries again: a sixth of the interval, between ten minutes and an hour. A
// cluster that is down must not get a job record every minute, and one that
// was only busy should not wait six hours.
func retryGap(interval time.Duration) time.Duration {
	return min(max(interval/6, 10*time.Minute), time.Hour)
}

// nextDue is when a schedule is next due. jitter is added to the interval
// part and not to the retry spacing.
func nextDue(sched *model.BackupSchedule, lastSuccess time.Time, st BackupStatus, jitter time.Duration) time.Time {
	iv, _ := model.BackupInterval(sched.Interval)
	due := sched.Since
	if !lastSuccess.IsZero() {
		due = lastSuccess.Add(iv)
	}
	due = due.Add(jitter)
	if st.LastAttemptAt.After(lastSuccess) {
		if r := st.LastAttemptAt.Add(retryGap(iv)); r.After(due) {
			due = r
		}
	}
	return due
}

// ---------------------------------------------------------------------------
// The job

// SubmitFunc is how the service starts a job; jobs.Engine.Submit.
type SubmitFunc func(ctx context.Context, j model.Job) (model.Job, error)

// RegisterBackups teaches the engine the snapshot job and keeps the engine's
// Submit for the scheduler and the run-now route.
func (s *Service) RegisterBackups(e *jobs.Engine) {
	s.submit = e.Submit
	e.Register(JobKindEtcdBackup, func(model.Job) ([]jobs.Step, error) {
		return []jobs.Step{s.backupStep()}, nil
	})
}

const (
	backupParamKeep    = "keep"
	backupParamTrigger = "trigger"
)

// Triggers.
const (
	TriggerSchedule = "schedule"
	TriggerManual   = "manual"
)

func (s *Service) backupStep() jobs.Step {
	return jobs.Step{
		Name: "take an etcd snapshot",
		Do: func(ctx context.Context, j *model.Job) error {
			keep, _ := strconv.Atoi(j.Params[backupParamKeep])
			if keep < 1 {
				keep = model.DefaultBackupKeep
			}
			err := s.takeScheduled(ctx, j.Cluster, keep)
			if err != nil && errors.Is(err, context.Canceled) {
				// Interrupted, not finished: the engine resumes or cancels, and
				// the record says "running" until it does.
				return err
			}
			s.finishAttempt(j.Cluster, j.ID, err)
			return err
		},
		// Resumable: the snapshot exists, or it does not. Taking another is
		// harmless, but not needed when one newer than the job is on disk.
		Happened: func(_ context.Context, j *model.Job) (bool, error) {
			snaps, err := s.snapshots.List(j.Cluster)
			if err != nil {
				return false, err
			}
			for _, sn := range snaps {
				if sn.Kind == SnapshotKindScheduled && !sn.TakenAt.Before(j.CreatedAt) {
					return true, nil
				}
			}
			return false, nil
		},
	}
}

// takeScheduled snapshots the cluster from the first control-plane node that
// answers, into the schedule's kind.
func (s *Service) takeScheduled(ctx context.Context, cluster model.ClusterID, keep int) error {
	machines, err := s.deps.Machines(ctx, cluster)
	if err != nil {
		return err
	}
	cc, _, err := s.anyControlPlane(ctx, machines)
	if err != nil {
		return fmt.Errorf("%w: the cluster could not be reached: %w", ErrBackupSkipped, err)
	}
	defer cc.Close() //nolint:errcheck // the byte count is the verdict

	// Best effort: the node's own idea of its database size sharpens the disk
	// guard's estimate; without it the largest stored snapshot is used.
	var estimate int64
	if status, serr := cc.EtcdStatus(ctx); serr == nil {
		estimate = status.DBSize
	}

	ctx, cancel := context.WithTimeout(ctx, SafetySnapshotTimeout)
	defer cancel()
	_, err = s.snapshots.writeKind(cluster, SnapshotKindScheduled, keep, estimate, func(w io.Writer) (int64, error) {
		return Snapshot(ctx, cc, w)
	})
	switch {
	case errors.Is(err, ErrDiskLow), errors.Is(err, ErrSnapshotInProgress):
		return fmt.Errorf("%w: %w", ErrBackupSkipped, err)
	}
	return err
}

func (s *Service) finishAttempt(cluster model.ClusterID, job model.JobID, err error) {
	result, reason := BackupOK, ""
	if err != nil {
		result = BackupFailed
		if errors.Is(err, ErrBackupSkipped) {
			result = BackupSkipped
		}
		reason = strings.ReplaceAll(err.Error(), s.snapshots.dir, "<snapshots>")
		if len(reason) > 300 {
			reason = reason[:300] + "..."
		}
	}
	s.snapshots.updateStatus(cluster, func(st *BackupStatus) {
		st.LastResult, st.LastReason, st.LastJob = result, reason, job
	})
}

// SubmitBackup records the attempt and starts the snapshot job.
//
// The attempt is written BEFORE the job is submitted: a daemon killed between
// the two leaves a record that says "tried", and the next start waits out the
// retry gap instead of trying twice.
func (s *Service) SubmitBackup(ctx context.Context, cluster model.ClusterID, keep int, trigger, actor string) (model.Job, error) {
	if s.snapshots == nil {
		return model.Job{}, ErrNoSnapshotStore
	}
	if s.submit == nil {
		return model.Job{}, ErrBackupsNotWired
	}
	if keep < 1 {
		keep = model.DefaultBackupKeep
	}
	s.snapshots.updateStatus(cluster, func(st *BackupStatus) {
		st.LastAttemptAt, st.LastTrigger = s.snapshots.now().UTC(), trigger
		st.LastResult, st.LastReason, st.LastJob = BackupRunning, "", ""
	})
	j, err := s.submit(ctx, model.Job{
		Kind:    JobKindEtcdBackup,
		Cluster: cluster,
		Actor:   actor,
		Params:  map[string]string{backupParamKeep: strconv.Itoa(keep), backupParamTrigger: trigger},
	})
	if err != nil {
		result, reason := BackupFailed, err.Error()
		if errors.Is(err, jobs.ErrClusterBusy) {
			result, reason = BackupSkipped, "another job holds this cluster; the schedule will try again"
		}
		s.snapshots.updateStatus(cluster, func(st *BackupStatus) {
			st.LastResult, st.LastReason = result, reason
		})
		return model.Job{}, err
	}
	s.snapshots.updateStatus(cluster, func(st *BackupStatus) { st.LastJob = j.ID })
	return j, nil
}

// ---------------------------------------------------------------------------
// The scheduler

// BackupSchedulerDeps is what the scheduler needs.
type BackupSchedulerDeps struct {
	// Clusters lists the clusters with their schedules.
	Clusters func(ctx context.Context) ([]model.Cluster, error)

	// Now is the clock; nil is time.Now.
	Now func() time.Time

	// Jitter picks a delay in [0, max). Nil is a random one. It is drawn once
	// per cluster per process, so a restart does not move a run that was
	// already due and a fleet of clusters does not all start on the minute.
	Jitter func(limit time.Duration) time.Duration

	// Every is how often the scheduler looks; zero is a minute.
	Every time.Duration

	// Log receives one line per decision that is not "nothing due".
	Log func(msg string, args ...any)
}

// BackupScheduler submits snapshot jobs when a cluster's schedule says one is
// due. It decides; the job does.
type BackupScheduler struct {
	svc  *Service
	deps BackupSchedulerDeps

	mu     sync.Mutex
	jitter map[model.ClusterID]time.Duration
}

// NewBackupScheduler builds the scheduler.
func (s *Service) NewBackupScheduler(d BackupSchedulerDeps) *BackupScheduler {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Every <= 0 {
		d.Every = time.Minute
	}
	if d.Jitter == nil {
		d.Jitter = randomJitter
	}
	if d.Log == nil {
		d.Log = func(string, ...any) {}
	}
	return &BackupScheduler{svc: s, deps: d, jitter: map[model.ClusterID]time.Duration{}}
}

// Run looks every Every until ctx ends. The first look is after one period,
// not at start: a daemon that restarts in a loop must not take a snapshot per
// restart, and the persisted record makes the first look correct either way.
func (b *BackupScheduler) Run(ctx context.Context) {
	t := time.NewTicker(b.deps.Every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			b.Tick(ctx)
		}
	}
}

// maxJitter caps the start jitter: a twelfth of the interval, at most five
// minutes.
func maxJitter(interval time.Duration) time.Duration {
	return min(interval/12, 5*time.Minute)
}

func (b *BackupScheduler) jitterFor(c model.ClusterID, interval time.Duration) time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()
	j, ok := b.jitter[c]
	if !ok {
		j = b.deps.Jitter(maxJitter(interval))
		b.jitter[c] = j
	}
	return j
}

// Tick makes one pass over the clusters.
func (b *BackupScheduler) Tick(ctx context.Context) {
	if b.svc.snapshots == nil {
		return
	}
	clusters, err := b.deps.Clusters(ctx)
	if err != nil {
		b.deps.Log("backup scheduler: list the clusters", "err", err)
		return
	}
	now := b.deps.Now()
	for _, c := range clusters {
		if !c.BackupSchedule.Enabled() {
			continue
		}
		b.consider(ctx, c, now)
	}
}

func (b *BackupScheduler) consider(ctx context.Context, c model.Cluster, now time.Time) {
	// A run that is still going is held back by the attempt record alone: the
	// retry gap (an hour for every preset) outlasts the longest a snapshot may
	// take (SafetySnapshotTimeout), so there is no separate "running" rule.
	st := b.svc.snapshots.readStatus(c.ID)
	snaps, err := b.svc.snapshots.List(c.ID)
	if err != nil {
		return
	}
	iv, _ := model.BackupInterval(c.BackupSchedule.Interval)
	if now.Before(nextDue(c.BackupSchedule, newestScheduled(snaps), st, b.jitterFor(c.ID, iv))) {
		return
	}

	if c.Disabled {
		// Off on purpose: no job, no hourly failure to read, one stated reason.
		b.svc.snapshots.updateStatus(c.ID, func(s *BackupStatus) {
			s.LastAttemptAt, s.LastTrigger = now.UTC(), TriggerSchedule
			s.LastResult, s.LastReason = BackupSkipped, "the cluster is switched off"
		})
		return
	}

	if _, err := b.svc.SubmitBackup(ctx, c.ID, c.BackupSchedule.Keep, TriggerSchedule, "scheduler"); err != nil {
		b.deps.Log("backup scheduler: not started", "cluster", string(c.ID), "err", err)
	}
}

func randomJitter(limit time.Duration) time.Duration {
	if limit <= 0 {
		return 0
	}
	return time.Duration(rand.Int64N(int64(limit))) //nolint:gosec // a start delay, not a secret
}
