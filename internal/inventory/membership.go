package inventory

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// The cluster decides who is in it, not this daemon.
//
// Until this, the membership was read exactly once, at adoption. A node that
// joined afterwards -- by the operator's own talosctl, by a machine booting a
// config it was handed elsewhere -- existed in Kubernetes and never in the
// inventory, until somebody typed its address into "Add a node by address".
// The operator's ruling is that the manager is a view of the cluster: what the
// cluster says it contains is what the manager shows, without being told.
//
// So every heartbeat, per cluster, one reachable node is asked for its
// cluster.Member list, and the inventory follows it: a member it has no
// record of is identified by UUID and recorded (the same path adoption takes,
// so D-10 and INV-03 hold unchanged), and a member whose role the cluster
// reports differently from the record gets the cluster's answer.
//
// What it does not do is delete. A member that leaves the list keeps its
// record, marked by its own supervisor as not answering: INV-08 forbids a node
// that stops answering from disappearing, and a discovery list that briefly
// comes back short is indistinguishable, from here, from a node that left.

// membershipLoop keeps every cluster's inventory in step with the cluster's own
// membership until ctx ends.
func (s *Service) membershipLoop(ctx context.Context) {
	for {
		s.SyncMembership(ctx)

		select {
		case <-ctx.Done():
			return
		case <-time.After(jitter(s.deps.Heartbeat)):
		}
	}
}

// SyncMembership runs one membership pass over every stored cluster.
func (s *Service) SyncMembership(ctx context.Context) {
	clusters, err := s.deps.Store.Clusters().List(ctx)
	if err != nil {
		s.deps.Logger.Error("membership sync could not list the clusters", slog.Any("error", err))
		return
	}
	for _, c := range clusters {
		if ctx.Err() != nil {
			return
		}
		s.syncCluster(ctx, c)
	}
}

func (s *Service) syncCluster(ctx context.Context, c model.Cluster) {
	// Same reason Refresh carries its own ceiling: the loop's context has no
	// deadline, and a Talos call on one is refused before it leaves (D-04).
	ctx, cancel := context.WithTimeout(ctx, RefreshBudget)
	defer cancel()

	machines, err := s.MachinesOf(ctx, c.ID)
	if err != nil {
		s.deps.Logger.Error("membership sync could not list a cluster's machines",
			slog.String("cluster", string(c.ID)), slog.Any("error", err))
		return
	}

	creds, err := s.clusterCreds(ctx, c.ID)
	if err != nil {
		// A cluster without stored credentials cannot be asked anything; its
		// machines' own supervisors already say so, once each.
		return
	}

	members, ok := s.readMembers(ctx, c, creds, machines)
	if !ok || len(members) == 0 {
		// Nobody answered, or discovery is off: the membership is not known,
		// and an unknown membership changes nothing.
		return
	}

	byHost := make(map[string]model.Machine, len(machines))
	for _, m := range machines {
		if m.Hostname != "" {
			byHost[m.Hostname] = m
		}
	}

	for _, m := range members {
		role := model.RoleWorker
		if m.ControlPlane {
			role = model.RoleControlPlane
		}

		if rec, known := byHost[m.Hostname]; known && m.Hostname != "" {
			if rec.Role != role {
				s.syncRole(ctx, rec, role)
			}
			continue
		}

		s.adoptJoined(ctx, c.ID, creds, m, role)
	}
}

// readMembers asks the cluster's own nodes for the membership, control planes
// first, and falls back to the cluster's endpoint when no stored node answers.
func (s *Service) readMembers(
	ctx context.Context,
	c model.Cluster,
	creds talos.Creds,
	machines []model.Machine,
) ([]talos.Member, bool) {
	targets := make([]talos.Target, 0, len(machines)+1)
	for _, want := range []bool{true, false} {
		for _, m := range machines {
			if m.Addr == "" || !m.LostAddrAt.IsZero() || (m.Role == model.RoleControlPlane) != want {
				continue
			}
			targets = append(targets, talos.Target{Cluster: c.ID, Machine: m.ID, Addr: m.Addr})
		}
	}
	if c.Endpoint != "" {
		targets = append(targets, talos.Target{
			Cluster: c.ID, Machine: model.MachineID("endpoint:" + c.Endpoint), Addr: c.Endpoint,
		})
	}

	for _, t := range targets {
		cc, err := talos.NewClusterClient(ctx, s.deps.Dialer, t, creds, s.deps.Mode)
		if err != nil {
			continue
		}
		members, err := cc.Members(ctx)
		_ = cc.Close()
		if err != nil {
			continue
		}
		return members, true
	}
	return nil, false
}

// syncRole gives a record the role the cluster reports for it.
func (s *Service) syncRole(ctx context.Context, rec model.Machine, role model.MachineRole) {
	err := s.merge(ctx, rec, func(m model.Machine) model.Machine {
		m.Role = role
		return m
	})
	if err != nil {
		s.deps.Logger.Error("membership sync could not record a role the cluster reports",
			slog.String("machine", string(rec.ID)), slog.Any("error", err))
		return
	}
	s.deps.Logger.Info("a machine's role follows the cluster's membership",
		slog.String("machine", string(rec.ID)),
		slog.String("was", string(rec.Role)),
		slog.String("now", string(role)))
}

// adoptJoined records a member the inventory has no record of.
func (s *Service) adoptJoined(
	ctx context.Context,
	cluster model.ClusterID,
	creds talos.Creds,
	m talos.Member,
	role model.MachineRole,
) {
	key := string(cluster) + "/" + m.Hostname

	facts, addr, err := s.identifyMember(ctx, cluster, creds, m)
	if err != nil {
		if s.firstMiss(key) {
			s.deps.Logger.Warn("a member of the cluster could not be asked who it is, so it is not in the inventory yet",
				slog.String("cluster", string(cluster)),
				slog.String("hostname", m.Hostname),
				slog.String("addresses", strings.Join(m.Addresses, ", ")),
				slog.String("remedy", "it is retried every heartbeat; add it by address if this host reaches it at none of these"),
				slog.Any("error", err))
		}
		return
	}

	// A machine filed under another stored cluster is not taken from it: that
	// is the silent move ErrAlreadyAdopted exists against (ledger 139).
	existing, err := s.deps.Store.Machines().Get(ctx, facts.UUID)
	switch {
	case err == nil:
		if existing.Cluster != "" && existing.Cluster != cluster {
			if s.firstMiss(key) {
				s.deps.Logger.Warn("a member of the cluster is filed under another cluster, so it is left there",
					slog.String("cluster", string(cluster)),
					slog.String("machine", string(facts.UUID)),
					slog.String("filed_under", string(existing.Cluster)))
			}
			return
		}
	case errors.Is(err, store.ErrNotFound):
	default:
		s.deps.Logger.Error("membership sync could not read a machine record",
			slog.String("machine", string(facts.UUID)), slog.Any("error", err))
		return
	}

	rec, err := s.recordMachine(ctx, cluster, facts, addr, role)
	if err != nil {
		s.deps.Logger.Error("membership sync could not record a member of the cluster",
			slog.String("cluster", string(cluster)),
			slog.String("hostname", m.Hostname), slog.Any("error", err))
		return
	}
	s.clearMiss(key)
	s.deps.Logger.Info("a member of the cluster is now in the inventory",
		slog.String("cluster", string(cluster)),
		slog.String("machine", string(rec.ID)),
		slog.String("hostname", rec.Hostname),
		slog.String("addr", addr),
		slog.String("role", string(rec.Role)))
}

// firstMiss reports whether key has not failed before, and remembers that it
// has now: a member that cannot be reached is logged once, not every heartbeat.
func (s *Service) firstMiss(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, seen := s.missed[key]; seen {
		return false
	}
	s.missed[key] = struct{}{}
	return true
}

func (s *Service) clearMiss(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.missed, key)
}
