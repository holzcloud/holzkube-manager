package upgrade

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

// The safety snapshot: no Talos upgrade starts without a fresh etcd snapshot
// of the cluster, taken by this instance and kept in its data directory.
//
// Talos' own upgrade guidance asks for an etcd snapshot before an upgrade
// because there is no command that brings a cluster back once it has lost its
// quorum, except a restore from one. A rolling upgrade is exactly the
// operation that can cost a quorum -- the health gate exists to prevent it --
// so the snapshot is the thing that makes the worst case recoverable rather
// than final. It is not left to the operator's memory: the start is refused
// without it, on the server, so no screen and no API client can skip it.
//
// "Fresh" is an hour. An upgrade is a decision made against the cluster as it
// is now, and the state it can be restored to should be the state that
// decision was made on; a snapshot from yesterday would restore a cluster
// missing a day of writes.

// SafetySnapshotMaxAge is how old a snapshot may be and still allow a start.
const SafetySnapshotMaxAge = time.Hour

// keepSafetySnapshots is how many snapshots per cluster are kept. A snapshot is
// the size of the cluster's etcd; the newest two are the one a run needs and
// the one before it, and anything older is disk spent on a cluster that has
// moved on. The data directory holds cluster secrets already, and so does
// every snapshot: the files are 0600 in a 0700 directory.
const keepSafetySnapshots = 2

// ErrSnapshotRequired reports a Talos upgrade refused for want of a fresh
// snapshot.
var ErrSnapshotRequired = errors.New("upgrade: a fresh etcd snapshot is required before a Talos upgrade")

// ErrNoSnapshotStore reports that this instance cannot keep snapshots. A start
// is refused in that state too: no store means no snapshot, and "no snapshot"
// is the one answer this check must not turn into a pass.
var ErrNoSnapshotStore = errors.New("upgrade: this instance has nowhere to keep a safety snapshot")

// SafetySnapshot is what the screen shows about a cluster's newest snapshot.
type SafetySnapshot struct {
	Cluster model.ClusterID `json:"cluster"`

	// Present is whether any snapshot exists.
	Present bool `json:"present"`

	// TakenAt and Bytes describe the newest one.
	TakenAt *time.Time `json:"taken_at,omitempty"`
	Bytes   int64      `json:"bytes,omitempty"`

	// Fresh is whether it is young enough to allow a start right now, and
	// ValidUntil is when it stops being.
	Fresh      bool       `json:"fresh"`
	ValidUntil *time.Time `json:"valid_until,omitempty"`

	// MaxAgeMinutes says how fresh "fresh" is, so the screen does not carry a
	// second copy of the number.
	MaxAgeMinutes int `json:"max_age_minutes"`
}

// FileOps is how the store reaches the disk. The composition root hands it the
// data directory's own primitives (internal/store/fsstore): nothing outside
// that package opens a path in the data directory itself, and a snapshot is a
// file in it like any other, only too large to be held in memory.
type FileOps struct {
	// WriteStream writes whatever fn streams to path atomically: a failure
	// leaves no file at path.
	WriteStream func(path string, fn func(io.Writer) (int64, error)) (int64, error)

	// List lists a directory; one that does not exist is empty.
	List func(dir string) ([]DirFile, error)

	// Remove deletes a file; one already gone is not an error.
	Remove func(path string) error

	// TempPrefix marks the temporaries WriteStream leaves while writing.
	TempPrefix string
}

// DirFile is one entry of FileOps.List.
type DirFile struct {
	Name  string
	Size  int64
	IsDir bool
}

// SnapshotStore keeps the snapshots in a directory of its own.
type SnapshotStore struct {
	dir string
	fs  FileOps
	now func() time.Time

	// mu keeps two takes for one cluster from interleaving their pruning.
	mu sync.Mutex
}

// NewSnapshotStore keeps snapshots under dir, one subdirectory per cluster.
func NewSnapshotStore(dir string, ops FileOps, now func() time.Time) *SnapshotStore {
	if now == nil {
		now = time.Now
	}
	return &SnapshotStore{dir: dir, fs: ops, now: now}
}

const snapshotStamp = "20060102T150405.000Z"

// clusterDir refuses a cluster id that is not a plain name. The id comes from
// the URL, and it becomes a path.
func (st *SnapshotStore) clusterDir(cluster model.ClusterID) (string, error) {
	id := string(cluster)
	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, `/\`+"\x00") {
		return "", fmt.Errorf("upgrade: %q is not a cluster id a snapshot can be filed under", id)
	}
	return filepath.Join(st.dir, id), nil
}

// Latest describes the newest snapshot of a cluster, or says there is none.
func (st *SnapshotStore) Latest(cluster model.ClusterID) (SafetySnapshot, error) {
	out := SafetySnapshot{Cluster: cluster, MaxAgeMinutes: int(SafetySnapshotMaxAge / time.Minute)}

	dir, err := st.clusterDir(cluster)
	if err != nil {
		return out, err
	}
	entries, err := st.fs.List(dir)
	if err != nil {
		return out, err
	}

	var (
		newestAt   time.Time
		newestSize int64
	)
	for _, e := range entries {
		at, ok := parseSnapshotName(e.Name)
		if !ok || e.IsDir || e.Size == 0 {
			continue
		}
		if at.After(newestAt) {
			newestAt, newestSize = at, e.Size
		}
	}
	if newestAt.IsZero() {
		return out, nil
	}

	until := newestAt.Add(SafetySnapshotMaxAge)
	out.Present = true
	out.TakenAt = &newestAt
	out.Bytes = newestSize
	out.ValidUntil = &until
	// A snapshot dated in the future is not fresh: the clock moved, and a
	// check that trusted the file's own claim would pass for an hour longer.
	out.Fresh = !newestAt.After(st.now()) && st.now().Before(until)
	return out, nil
}

func parseSnapshotName(name string) (time.Time, bool) {
	stem, ok := strings.CutSuffix(name, ".snapshot")
	if !ok {
		return time.Time{}, false
	}
	at, err := time.Parse(snapshotStamp, stem)
	if err != nil {
		return time.Time{}, false
	}
	return at.UTC(), true
}

// write stores whatever fn streams as the cluster's newest snapshot.
//
// It is written atomically, so a stream that fails half way leaves nothing
// that looks like a snapshot: the freshness check reads the directory, and a
// truncated file with a new name would be a pass for a snapshot that cannot be
// restored.
func (st *SnapshotStore) write(cluster model.ClusterID, fn func(io.Writer) (int64, error)) (SafetySnapshot, error) {
	st.mu.Lock()
	defer st.mu.Unlock()

	dir, err := st.clusterDir(cluster)
	if err != nil {
		return SafetySnapshot{}, err
	}
	final := filepath.Join(dir, st.now().UTC().Format(snapshotStamp)+".snapshot")
	n, err := st.fs.WriteStream(final, func(w io.Writer) (int64, error) {
		n, ferr := fn(w)
		if ferr != nil {
			return 0, fmt.Errorf("the snapshot did not complete, and none was kept: %w", ferr)
		}
		if n <= 0 {
			return 0, errors.New("the node sent an empty snapshot, and none was kept")
		}
		return n, nil
	})
	if err != nil {
		return SafetySnapshot{}, err
	}
	_ = n

	st.prune(dir)
	return st.Latest(cluster)
}

// prune removes all but the newest keepSafetySnapshots, and any temporary a
// crash left behind.
func (st *SnapshotStore) prune(dir string) {
	entries, err := st.fs.List(dir)
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		switch {
		case st.fs.TempPrefix != "" && strings.HasPrefix(e.Name, st.fs.TempPrefix):
			// Only a process that died mid-write leaves one: this run's own
			// temporary has been renamed by now.
			_ = st.fs.Remove(filepath.Join(dir, e.Name))
		default:
			if _, ok := parseSnapshotName(e.Name); ok {
				names = append(names, e.Name)
			}
		}
	}
	sort.Strings(names) // the stamp sorts as time does
	for len(names) > keepSafetySnapshots {
		_ = st.fs.Remove(filepath.Join(dir, names[0]))
		names = names[1:]
	}
}

// WithSnapshots gives the service somewhere to keep safety snapshots.
func (s *Service) WithSnapshots(st *SnapshotStore) *Service {
	s.snapshots = st
	return s
}

// SafetySnapshotOf reports a cluster's newest snapshot.
func (s *Service) SafetySnapshotOf(cluster model.ClusterID) (SafetySnapshot, error) {
	if s.snapshots == nil {
		return SafetySnapshot{Cluster: cluster, MaxAgeMinutes: int(SafetySnapshotMaxAge / time.Minute)},
			ErrNoSnapshotStore
	}
	return s.snapshots.Latest(cluster)
}

// TakeSafetySnapshot snapshots the cluster's etcd into the store.
func (s *Service) TakeSafetySnapshot(ctx context.Context, cluster model.ClusterID) (SafetySnapshot, error) {
	if s.snapshots == nil {
		return SafetySnapshot{}, ErrNoSnapshotStore
	}
	return s.snapshots.write(cluster, func(w io.Writer) (int64, error) {
		return s.Snapshot(ctx, cluster, w)
	})
}

// RequireSafetySnapshot is the check a Talos upgrade passes before it is
// confirmed and again before it is submitted. It fails closed: a store that is
// missing, a directory that cannot be read and a snapshot that is stale are the
// same answer.
func (s *Service) RequireSafetySnapshot(cluster model.ClusterID) error {
	snap, err := s.SafetySnapshotOf(cluster)
	if err != nil {
		if errors.Is(err, ErrNoSnapshotStore) {
			return err
		}
		return fmt.Errorf("%w: the snapshots could not be read (%v)", ErrSnapshotRequired, err)
	}
	if !snap.Fresh {
		if !snap.Present {
			return fmt.Errorf("%w: there is none for this cluster yet. Take one on the Upgrades screen; "+
				"it is the one thing that brings the cluster back if a node takes its quorum with it",
				ErrSnapshotRequired)
		}
		return fmt.Errorf("%w: the newest was taken %s ago and a snapshot older than %d minutes does not "+
			"describe the cluster this upgrade is being decided against",
			ErrSnapshotRequired, s.snapshots.now().Sub(*snap.TakenAt).Round(time.Minute), snap.MaxAgeMinutes)
	}
	return nil
}
