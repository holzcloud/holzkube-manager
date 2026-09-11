package inventory

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// SetLock opens or closes a cluster's read-only mutation lock (INV-12).
//
// Unlocking is itself a destructive route -- it is what makes every other
// destructive route reachable -- so it sits behind the sudo window and is
// audited like one. The lock is enforced server-side at the route (D-22); a
// lock the UI merely honours is not a lock, which is the same argument that
// put dry-run into the transport rather than into a handler.
func (s *Service) SetLock(ctx context.Context, id model.ClusterID, locked bool) (model.Cluster, error) {
	c, err := s.deps.Store.Clusters().Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return model.Cluster{}, ErrNotFound
		}
		return model.Cluster{}, err
	}
	if c.Locked == locked {
		return c, nil
	}
	c.Locked = locked
	return s.deps.Store.Clusters().Put(ctx, c)
}

// CheckLock is what the route middleware asks before a mutating cluster-scoped
// operation runs.
//
// A machine that belongs to no cluster is not locked: there is no cluster to
// have adopted it read-only, and refusing would make an unassigned machine
// permanently untouchable.
func (s *Service) CheckLock(ctx context.Context, id model.ClusterID) error {
	if id == "" {
		return nil
	}
	c, err := s.deps.Store.Clusters().Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	if c.Locked {
		return fmt.Errorf("%w: %s was adopted read-only; unlock it before changing anything on it",
			ErrClusterLocked, c.Name)
	}
	return nil
}

// ClusterOfMachine reports which cluster a machine belongs to, so that a route
// scoped to a node can be checked against its cluster's lock.
func (s *Service) ClusterOfMachine(ctx context.Context, id model.MachineID) (model.ClusterID, error) {
	rec, err := s.deps.Store.Machines().Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return "", ErrNotFound
		}
		return "", err
	}
	return rec.Cluster, nil
}

// ForgetMachine removes a machine from the inventory.
//
// It removes the record and touches the machine not at all: there is no
// cordon, no drain and no reset here (that is phase 6). It happens only on an
// explicit operator action and never on a timer -- no TTL, no "unreachable for
// N days, therefore gone" (D-11, INV-09). A record that disappears on its own
// is exactly the record an operator needed during the outage that made it
// disappear.
func (s *Service) ForgetMachine(ctx context.Context, id model.MachineID) error {
	if err := s.deps.Store.Machines().Delete(ctx, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	s.mu.Lock()
	delete(s.observed, id)
	s.mu.Unlock()
	return nil
}

// ForgetCluster removes a cluster, its secrets and its machines' membership.
//
// The machines survive as unassigned records rather than being deleted with
// the cluster: they are physical machines that still exist, and a screen that
// forgets them because a cluster was removed has lost the inventory's point.
func (s *Service) ForgetCluster(ctx context.Context, id model.ClusterID) error {
	if _, err := s.deps.Store.Clusters().Get(ctx, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	machines, err := s.deps.Store.Machines().List(ctx)
	if err != nil {
		return err
	}
	for _, m := range machines {
		if m.Cluster != id {
			continue
		}
		m.Cluster = ""
		if _, err := s.deps.Store.Machines().Put(ctx, m); err != nil {
			return err
		}
	}

	// Secrets first, then the cluster: the invariant worth preserving under a
	// partial failure is "no bundle without its cluster", because an orphaned
	// bundle is cluster PKI on disk that nothing refers to and nothing will
	// ever clean up.
	if err := s.deps.Store.ClusterSecrets().Delete(ctx, id); err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	return s.deps.Store.Clusters().Delete(ctx, id)
}

// AddManual records a machine the operator named by address.
//
// It is the always-available second way into the inventory (D-08): after an
// import the membership fills itself, but a cluster with discovery switched
// off, or a cluster that is entirely down, leaves the operator with an address
// and nothing else. It is deliberately not a subnet scan -- that is a
// different capability and belongs where maintenance-mode machines have to be
// found (D-09).
func (s *Service) AddManual(ctx context.Context, cluster model.ClusterID, addr string) (model.Machine, error) {
	creds, err := s.clusterCreds(ctx, cluster)
	if err != nil {
		return model.Machine{}, err
	}

	provisional := talos.Target{
		Cluster: cluster,
		Machine: model.MachineID("manual:" + addr),
		Addr:    addr,
	}

	cc, err := talos.NewClusterClient(ctx, s.deps.Dialer, provisional, creds, s.deps.Mode)
	if err != nil {
		return model.Machine{}, err
	}
	defer cc.Close() //nolint:errcheck // the verdict is not a close error's to change

	facts, err := cc.NodeFacts(ctx)
	if err != nil {
		return model.Machine{}, err
	}

	role := model.RoleWorker
	if members, err := cc.Members(ctx); err == nil {
		for _, m := range members {
			if model.MachineID(m.ID) == facts.UUID && m.ControlPlane {
				role = model.RoleControlPlane
			}
		}
	} else {
		// Without a membership list the role is not known, and guessing
		// "worker" would silently exclude a control-plane node from the etcd
		// reads that only control-plane nodes get.
		role = model.RoleUnknown
	}

	rec, err := s.recordMachine(ctx, cluster, facts, addr, role)
	if err != nil {
		return model.Machine{}, err
	}
	s.deps.Logger.Info("machine added by address",
		slog.String("machine", string(rec.ID)), slog.String("addr", addr))
	return rec, nil
}

// Connect opens a cluster client to one machine in the inventory.
//
// It is the one way anything outside this package reaches a node, and that is
// deliberate: the credentials belong to the cluster, the address is a hint on
// the machine record, and a caller that assembled either itself would be a
// second answer to "how do we reach a node" that nothing keeps in step with
// the first. The stream readers are the first such caller; phase 6's jobs are
// the next.
//
// The caller owns the returned client and must Close it.
func (s *Service) Connect(ctx context.Context, id model.MachineID) (*talos.ClusterClient, error) {
	rec, err := s.deps.Store.Machines().Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	creds, err := s.clusterCreds(ctx, rec.Cluster)
	if err != nil {
		return nil, err
	}

	return talos.NewClusterClient(ctx, s.deps.Dialer, talos.Target{
		Cluster: rec.Cluster,
		Machine: rec.ID,
		Addr:    rec.Addr,
	}, creds, s.deps.Mode)
}
