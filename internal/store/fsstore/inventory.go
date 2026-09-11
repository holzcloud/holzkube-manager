package fsstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
)

// The inventory entities are implemented once, generically, rather than as
// three more hand-written copies of the schematic store.
//
// That is a deliberate departure from the note above schematicStore, which
// chose two readable copies over a shared abstraction. The trade changes at
// three: the CAS branch below is the part that is subtle -- a non-zero rev for
// a record that is not there is a conflict and not a create, because silently
// recreating it would undo a deletion -- and three hand-copied versions of a
// subtle branch is three places for it to drift. The entity-specific parts
// that remain are exactly the parts that differ: the key rule, the lock kind
// and the noun in the error message.
type recordStore[K ~string, T any] struct {
	locks *store.EntityLocks
	dir   string

	// kind is the first half of the lock key, so that clusters/a and
	// machines/a never contend.
	kind string

	// noun is what a conflict message calls this record.
	noun string

	// key, rev and setRev are the three things the generic code cannot know.
	// They are functions rather than an interface on the model types because
	// persistence concerns have no business appearing as methods on a record
	// that other packages pass around.
	key    func(T) K
	rev    func(T) uint64
	setRev func(*T, uint64)
}

func (rs *recordStore[K, T]) path(id K) (string, error) {
	if err := safeKey(string(id)); err != nil {
		return "", err
	}
	return filepath.Join(rs.dir, string(id)+".json"), nil
}

func (rs *recordStore[K, T]) Get(_ context.Context, id K) (T, error) {
	var rec T
	p, err := rs.path(id)
	if err != nil {
		return rec, err
	}
	if err := readJSON(p, &rec); err != nil {
		var zero T
		return zero, err
	}
	return rec, nil
}

func (rs *recordStore[K, T]) List(_ context.Context) ([]T, error) {
	out := make([]T, 0, 1)
	err := listJSON(rs.dir, func(raw []byte) error {
		var rec T
		if err := json.Unmarshal(raw, &rec); err != nil {
			return err
		}
		out = append(out, rec)
		return nil
	})
	return out, err
}

func (rs *recordStore[K, T]) Put(_ context.Context, rec T) (T, error) {
	var zero T

	id := rs.key(rec)
	p, err := rs.path(id)
	if err != nil {
		return zero, err
	}

	defer rs.locks.LockEntity(rs.kind, string(id))()

	var current T
	switch err := readJSON(p, &current); {
	case err == nil:
		if rs.rev(rec) != rs.rev(current) {
			return zero, fmt.Errorf("%w: %s %s is at rev %d, put carried %d",
				store.ErrConflict, rs.noun, id, rs.rev(current), rs.rev(rec))
		}
	case errors.Is(err, store.ErrNotFound):
		if rs.rev(rec) != 0 {
			return zero, fmt.Errorf("%w: %s %s does not exist but put carried rev %d",
				store.ErrConflict, rs.noun, id, rs.rev(rec))
		}
	default:
		return zero, err
	}

	rs.setRev(&rec, rs.rev(rec)+1)
	raw, err := marshalRecord(rec)
	if err != nil {
		return zero, err
	}
	if err := writeAtomic(p, raw); err != nil {
		return zero, err
	}
	return rec, nil
}

func (rs *recordStore[K, T]) Delete(_ context.Context, id K) error {
	p, err := rs.path(id)
	if err != nil {
		return err
	}
	defer rs.locks.LockEntity(rs.kind, string(id))()
	if err := removeAndSync(p); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return store.ErrNotFound
		}
		return err
	}
	return nil
}

// clusterStore, clusterSecretStore and machineStore are the three concrete
// entities. Each is a thin wrapper rather than a type alias so that the
// interface it satisfies is the narrow one from the store package -- the
// secrets entity in particular must not grow a List, and wrapping is what
// makes that a compile-time property rather than a convention.
type clusterStore struct {
	recordStore[model.ClusterID, model.Cluster]
}

type machineStore struct {
	recordStore[model.MachineID, model.Machine]
}

type clusterSecretStore struct {
	inner recordStore[model.ClusterID, model.ClusterSecrets]
}

func (cs *clusterSecretStore) Get(ctx context.Context, id model.ClusterID) (model.ClusterSecrets, error) {
	return cs.inner.Get(ctx, id)
}

func (cs *clusterSecretStore) Put(ctx context.Context, rec model.ClusterSecrets) (model.ClusterSecrets, error) {
	return cs.inner.Put(ctx, rec)
}

func (cs *clusterSecretStore) Delete(ctx context.Context, id model.ClusterID) error {
	return cs.inner.Delete(ctx, id)
}

func newClusterStore(dir string, locks *store.EntityLocks) *clusterStore {
	return &clusterStore{recordStore[model.ClusterID, model.Cluster]{
		locks:  locks,
		dir:    dir,
		kind:   kindClusters,
		noun:   "cluster",
		key:    func(c model.Cluster) model.ClusterID { return c.ID },
		rev:    func(c model.Cluster) uint64 { return c.Rev },
		setRev: func(c *model.Cluster, v uint64) { c.Rev = v },
	}}
}

func newClusterSecretStore(dir string, locks *store.EntityLocks) *clusterSecretStore {
	return &clusterSecretStore{inner: recordStore[model.ClusterID, model.ClusterSecrets]{
		locks:  locks,
		dir:    dir,
		kind:   kindClusterSecrets,
		noun:   "cluster secrets",
		key:    func(s model.ClusterSecrets) model.ClusterID { return s.Cluster },
		rev:    func(s model.ClusterSecrets) uint64 { return s.Rev },
		setRev: func(s *model.ClusterSecrets, v uint64) { s.Rev = v },
	}}
}

func newMachineStore(dir string, locks *store.EntityLocks) *machineStore {
	return &machineStore{recordStore[model.MachineID, model.Machine]{
		locks:  locks,
		dir:    dir,
		kind:   kindMachines,
		noun:   "machine",
		key:    func(m model.Machine) model.MachineID { return m.ID },
		rev:    func(m model.Machine) uint64 { return m.Rev },
		setRev: func(m *model.Machine, v uint64) { m.Rev = v },
	}}
}
