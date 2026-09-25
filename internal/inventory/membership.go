package inventory

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"
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
// A member that leaves the list is not deleted. Its record is stamped LeftAt,
// and once DepartedAfter has passed without it coming back it is hidden from
// every view and every plan (presentMachines) while its labels and lock stay
// on disk for the day it rejoins. The grace period is INV-08's concern kept:
// a node that merely stops answering is still listed by discovery and stays
// visible as not answering, and a list that comes back short for one pass
// hides nothing.
//
// A member whose recorded address is an IP the cluster no longer reports for
// it is asked again at the addresses it does report, and the one that answers
// with the same UUID becomes the record's address.

// DepartedAfter is how long a machine has to be missing from its cluster's
// membership before the views stop showing it.
const DepartedAfter = 10 * time.Minute

// presentMachines lists the machines that are not departed: every record
// except one its cluster has not listed for DepartedAfter.
func (s *Service) presentMachines(ctx context.Context) ([]model.Machine, error) {
	all, err := s.deps.Store.Machines().List(ctx)
	if err != nil {
		return nil, err
	}
	now := s.deps.Now()
	out := all[:0]
	for _, m := range all {
		if !departed(m, now) {
			out = append(out, m)
		}
	}
	return out, nil
}

func departed(m model.Machine, now time.Time) bool {
	return !m.LeftAt.IsZero() && now.Sub(m.LeftAt) >= DepartedAfter
}

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

	// Every record, departed ones included: a departed machine that is listed
	// again is recognised here and comes back with its labels and lock.
	all, err := s.deps.Store.Machines().List(ctx)
	if err != nil {
		s.deps.Logger.Error("membership sync could not list a cluster's machines",
			slog.String("cluster", string(c.ID)), slog.Any("error", err))
		return
	}

	machines := make([]model.Machine, 0, len(all))
	for _, m := range all {
		if m.Cluster == c.ID {
			machines = append(machines, m)
		}
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

	listed := make(map[model.MachineID]bool, len(members))
	for _, m := range members {
		role := model.RoleWorker
		if m.ControlPlane {
			role = model.RoleControlPlane
		}

		if rec, known := byHost[m.Hostname]; known && m.Hostname != "" {
			listed[rec.ID] = true
			if rec.Role != role || !rec.LeftAt.IsZero() {
				s.syncListed(ctx, rec, role)
			}
			if addressMoved(rec.Addr, m.Addresses) {
				s.followAddress(ctx, c.ID, creds, rec, m)
			}
			continue
		}

		if id, ok := s.adoptJoined(ctx, c.ID, creds, m, role); ok {
			listed[id] = true
		}
	}

	now := s.deps.Now().UTC()
	for _, rec := range machines {
		if listed[rec.ID] || !rec.LeftAt.IsZero() {
			continue
		}
		err := s.merge(ctx, rec, func(m model.Machine) model.Machine {
			if m.LeftAt.IsZero() {
				m.LeftAt = now
			}
			return m
		})
		if err != nil {
			s.deps.Logger.Error("membership sync could not mark a machine the cluster no longer lists",
				slog.String("machine", string(rec.ID)), slog.Any("error", err))
			continue
		}
		s.deps.Logger.Info("the cluster no longer lists a machine; it is hidden if it does not come back",
			slog.String("cluster", string(c.ID)),
			slog.String("machine", string(rec.ID)),
			slog.String("hostname", rec.Hostname),
			slog.Duration("after", DepartedAfter))
	}
}

// addressMoved reports whether a recorded IP is one the member no longer
// reports. A name or an empty address is left alone: a DNS name or an
// endpoint the operator typed is theirs, not the membership's.
func addressMoved(addr string, reported []string) bool {
	ip, err := netip.ParseAddr(addr)
	if err != nil || len(reported) == 0 {
		return false
	}
	for _, r := range reported {
		if other, err := netip.ParseAddr(r); err == nil && other == ip {
			return false
		}
	}
	return true
}

// followAddress re-asks a member at the addresses it reports and moves the
// record to the one that answers as the same machine.
func (s *Service) followAddress(
	ctx context.Context,
	cluster model.ClusterID,
	creds talos.Creds,
	rec model.Machine,
	m talos.Member,
) {
	facts, addr, err := s.identifyMember(ctx, cluster, creds, m)
	if err != nil || facts.UUID != rec.ID {
		return
	}
	if _, err := s.recordMachine(ctx, cluster, facts, addr, ""); err != nil {
		s.deps.Logger.Error("membership sync could not move a machine to its new address",
			slog.String("machine", string(rec.ID)), slog.Any("error", err))
		return
	}
	s.deps.Logger.Info("a machine's address follows the cluster's membership",
		slog.String("machine", string(rec.ID)),
		slog.String("was", rec.Addr),
		slog.String("now", addr))
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

// syncListed gives a record the role the cluster reports for it and clears
// LeftAt: the cluster lists it again.
func (s *Service) syncListed(ctx context.Context, rec model.Machine, role model.MachineRole) {
	err := s.merge(ctx, rec, func(m model.Machine) model.Machine {
		m.Role = role
		m.LeftAt = time.Time{}
		return m
	})
	if err != nil {
		s.deps.Logger.Error("membership sync could not record a role the cluster reports",
			slog.String("machine", string(rec.ID)), slog.Any("error", err))
		return
	}
	if rec.Role == role {
		s.deps.Logger.Info("a machine the cluster had stopped listing is listed again",
			slog.String("machine", string(rec.ID)))
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
) (model.MachineID, bool) {
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
		return "", false
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
			return "", false
		}
	case errors.Is(err, store.ErrNotFound):
	default:
		s.deps.Logger.Error("membership sync could not read a machine record",
			slog.String("machine", string(facts.UUID)), slog.Any("error", err))
		return "", false
	}

	rec, err := s.recordMachine(ctx, cluster, facts, addr, role)
	if err != nil {
		s.deps.Logger.Error("membership sync could not record a member of the cluster",
			slog.String("cluster", string(cluster)),
			slog.String("hostname", m.Hostname), slog.Any("error", err))
		return "", false
	}
	s.clearMiss(key)
	s.deps.Logger.Info("a member of the cluster is now in the inventory",
		slog.String("cluster", string(cluster)),
		slog.String("machine", string(rec.ID)),
		slog.String("hostname", rec.Hostname),
		slog.String("addr", addr),
		slog.String("role", string(rec.Role)))
	return rec.ID, true
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
