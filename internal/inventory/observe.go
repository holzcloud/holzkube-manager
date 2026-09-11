package inventory

import (
	"context"
	"crypto/x509"
	"errors"
	"log/slog"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/health"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// recordMachine files a machine under its UUID, creating the record if it is
// new and updating the address if it moved.
//
// The UUID always wins against the address (D-10). If a known address answers
// with an unfamiliar UUID, the stranger gets a record of its own and the
// machine that used to live there is marked as no longer being at that
// address -- it is never overwritten. The other direction, address wins, would
// swap two machines' entire histories on a DHCP rotation, and would do it
// silently.
func (s *Service) recordMachine(
	ctx context.Context,
	cluster model.ClusterID,
	facts talos.NodeFacts,
	addr string,
	role model.MachineRole,
) (model.Machine, error) {
	if facts.UUID == "" {
		return model.Machine{}, talos.ErrNoMachineIdentity
	}

	now := s.deps.Now().UTC()

	if addr != "" {
		if err := s.evictAddressHolders(ctx, facts.UUID, addr, now); err != nil {
			return model.Machine{}, err
		}
	}

	rec, err := s.deps.Store.Machines().Get(ctx, facts.UUID)
	switch {
	case err == nil:
	case errors.Is(err, store.ErrNotFound):
		rec = model.Machine{ID: facts.UUID, AdoptedAt: now, Role: model.RoleUnknown}
	default:
		return model.Machine{}, err
	}

	if cluster != "" {
		rec.Cluster = cluster
	}
	if role != "" && role != model.RoleUnknown {
		rec.Role = role
	}
	if rec.Role == "" {
		rec.Role = model.RoleUnknown
	}
	if facts.Hostname != "" {
		rec.Hostname = facts.Hostname
	}
	if addr != "" {
		rec.Addr = addr
		rec.LostAddrAt = time.Time{}
	}

	return s.deps.Store.Machines().Put(ctx, rec)
}

// evictAddressHolders marks every other machine that claims addr as no longer
// being there.
//
// It is the half of D-10 that is easy to leave out and expensive to omit: two
// records both claiming one address is what makes the next lookup ambiguous,
// and the ambiguity is resolved by whichever record happened to be read first.
func (s *Service) evictAddressHolders(ctx context.Context, keep model.MachineID, addr string, now time.Time) error {
	all, err := s.deps.Store.Machines().List(ctx)
	if err != nil {
		return err
	}
	for _, other := range all {
		if other.ID == keep || other.Addr != addr || !other.LostAddrAt.IsZero() {
			continue
		}
		other.LostAddrAt = now
		if _, err := s.deps.Store.Machines().Put(ctx, other); err != nil {
			return err
		}
		s.deps.Logger.Info("a different machine answered at a known address",
			slog.String("addr", addr),
			slog.String("was", string(other.ID)),
			slog.String("now", string(keep)))
	}
	return nil
}

// Start launches one supervisor per machine and keeps them running until the
// context is cancelled or Close is called.
//
// Eager, not lazy (D-17). A supervisor that only runs while somebody is
// looking makes the dashboard slow exactly when it is needed and makes
// stale_since meaningless on the first load. Pattern 7 is explicit that five
// to twenty nodes need no scheduler, so this is a goroutine each.
func (s *Service) Start(ctx context.Context) error {
	machines, err := s.deps.Store.Machines().List(ctx)
	if err != nil {
		return err
	}

	runCtx, cancel := context.WithCancel(ctx)

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		cancel()
		return errors.New("inventory: service is closed")
	}
	s.stop = cancel
	s.mu.Unlock()

	for _, m := range machines {
		s.supervise(runCtx, m.ID)
	}
	return nil
}

// Supervise starts an observer for one machine if it does not already have
// one. It is what the adoption path calls for each machine it just recorded.
func (s *Service) Supervise(ctx context.Context, id model.MachineID) {
	s.supervise(ctx, id)
}

func (s *Service) supervise(ctx context.Context, id model.MachineID) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	if _, running := s.observed[id]; running {
		s.mu.Unlock()
		return
	}
	s.observed[id] = newObservation()
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.observeLoop(ctx, id)
	}()
}

func newObservation() *observation {
	return &observation{stage: health.StageUnknown, levels: map[health.Level]levelState{}}
}

// observeLoop is one machine's supervisor.
//
// The jitter is not decoration: without it every node in the fleet is asked in
// the same millisecond after a restart, which is a burst against a cluster
// that may already be struggling and a saw-tooth in every graph.
func (s *Service) observeLoop(ctx context.Context, id model.MachineID) {
	for {
		s.Refresh(ctx, id)

		select {
		case <-ctx.Done():
			return
		case <-time.After(jitter(s.deps.Heartbeat)):
		}
	}
}

func jitter(d time.Duration) time.Duration {
	if d <= 0 {
		return time.Second
	}
	// +/- 20%, which is enough to spread a fleet without making the interval
	// something an operator cannot reason about.
	spread := float64(d) * 0.4
	return d - time.Duration(spread/2) + time.Duration(rand.Float64()*spread)
}

// Refresh performs one observation pass over a machine and persists what it
// learned.
//
// It is exported because the adoption path and the tests both want a pass to
// have happened before they look, and because a screen that has just been
// opened may legitimately ask for one. It never returns an error: an
// unreachable node is a state the inventory records, not a failure a caller
// has to handle.
func (s *Service) Refresh(ctx context.Context, id model.MachineID) {
	rec, err := s.deps.Store.Machines().Get(ctx, id)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			s.deps.Logger.Error("inventory refresh could not read the machine record",
				slog.String("machine", string(id)), slog.Any("error", err))
		}
		return
	}

	obs := s.observationFor(id)
	obs.enter(health.StageConnecting)

	creds, err := s.clusterCreds(ctx, rec.Cluster)
	if err != nil {
		obs.fail(health.LevelNode, "this machine belongs to no cluster with stored credentials", false)
		return
	}

	cc, err := talos.NewClusterClient(ctx, s.deps.Dialer, talos.Target{
		Cluster: rec.Cluster,
		Machine: rec.ID,
		Addr:    rec.Addr,
	}, creds, s.deps.Mode)
	if err != nil {
		obs.fail(health.LevelNode, unreachableReason(err), certificateExpired(err))
		return
	}
	defer cc.Close() //nolint:errcheck // an observation's verdict is not a close error's to change

	now := s.deps.Now().UTC()
	snap := model.MachineSnapshot{ObservedAt: now}
	previous := rec.Snapshot

	facts, err := cc.NodeFacts(ctx)
	if err != nil {
		obs.fail(health.LevelNode, unreachableReason(err), certificateExpired(err))
		return
	}

	snap.TalosVersion = facts.TalosVersion
	snap.SchematicID = facts.SchematicID
	snap.Manufacturer = facts.Manufacturer
	snap.ProductName = facts.ProductName
	snap.SerialNumber = facts.SerialNumber
	snap.MemoryMiB = facts.MemoryMiB
	snap.KubernetesVersion = facts.KubernetesVersion()

	for _, c := range facts.CPUs {
		snap.CPUs = append(snap.CPUs, model.CPU{
			Manufacturer: c.Manufacturer,
			ProductName:  c.ProductName,
			Cores:        c.Cores,
			Threads:      c.Threads,
			MaxSpeedMHz:  c.MaxSpeedMHz,
		})
	}
	for _, d := range facts.BlockDevices {
		snap.Disks = append(snap.Disks, model.Disk{
			Device:     d.Device,
			Size:       d.Size,
			PrettySize: d.PrettySize,
			Model:      d.Model,
			Serial:     d.Serial,
			Transport:  d.Transport,
			Rotational: d.Rotational,
			Readonly:   d.Readonly,
			CDROM:      d.CDROM,
		})
	}
	for _, l := range facts.Links {
		iface := model.Interface{
			Name:         l.Name,
			HardwareAddr: l.HardwareAddr,
			MTU:          l.MTU,
			Up:           l.Up,
			SpeedMbit:    l.SpeedMbit,
			Driver:       l.Driver,
			Kind:         l.Kind,
		}
		// The node's addresses are reported per node rather than per link, so
		// they are attached to the link that is up. Attaching them to every
		// link would claim each address on every interface.
		if l.Up && len(facts.Addresses) > 0 && len(iface.Addresses) == 0 {
			iface.Addresses = facts.Addresses
		}
		snap.Interfaces = append(snap.Interfaces, iface)
	}

	// The Talos version the Version RPC reports wins over the SMBIOS one: it
	// is what the running system says about itself, and the other is what the
	// firmware was told.
	if version, err := cc.Version(ctx); err == nil {
		snap.TalosVersion = version
	}

	if services, err := cc.ServiceList(ctx); err == nil {
		for _, svc := range services {
			snap.Services = append(snap.Services, model.Service{
				ID:            svc.ID,
				State:         svc.State,
				Running:       strings.EqualFold(svc.State, "Running"),
				Healthy:       svc.Healthy,
				HealthUnknown: svc.HealthUnknown,
			})
		}
	}

	obs.confirm(health.LevelNode, now)

	// Kubernetes-derived facts. Their absence is a statement about Kubernetes
	// and never about the node: the node-level fields above stay confirmed
	// whatever happens here, which is the property INV-07 and INV-08 turn on.
	if snap.KubernetesVersion != "" {
		obs.confirm(health.LevelK8s, now)
	} else {
		snap.KubernetesVersion = previous.KubernetesVersion
		obs.fail(health.LevelK8s, "the node reports no kubelet, so its Kubernetes version is unknown", false)
	}

	// etcd, asked only of control-plane nodes: a worker has no etcd and
	// reporting "etcd unreachable" for it would be a permanent false alarm.
	if rec.Role == model.RoleControlPlane {
		members, err := cc.EtcdMemberList(ctx)
		if err != nil {
			snap.EtcdMember = previous.EtcdMember
			obs.fail(health.LevelEtcd, "etcd did not answer on this node", false)
		} else {
			for _, m := range members {
				// Matched on hostname, and a node that reports none simply
				// does not match: matching the empty string would claim
				// membership for every member whose hostname etcd did not
				// report either.
				if facts.Hostname != "" && m.Hostname == facts.Hostname {
					snap.EtcdMember = true
					break
				}
			}
			obs.confirm(health.LevelEtcd, now)
		}
	}

	rec.Snapshot = snap
	rec.SeenAt = now
	if facts.Hostname != "" {
		rec.Hostname = facts.Hostname
	}
	if _, err := s.deps.Store.Machines().Put(ctx, rec); err != nil {
		s.deps.Logger.Error("inventory refresh could not persist the snapshot",
			slog.String("machine", string(id)), slog.Any("error", err))
	}
}

func (s *Service) observationFor(id model.MachineID) *observation {
	s.mu.Lock()
	defer s.mu.Unlock()

	obs, ok := s.observed[id]
	if !ok {
		obs = newObservation()
		s.observed[id] = obs
	}
	return obs
}

// status returns a machine's per-level confirmation state, loading the
// persisted snapshot's timestamp for a machine no supervisor has confirmed
// yet.
//
// That load is D-16's second half. A restart of holzkubed most likely happens
// during the outage the operator is trying to understand, and the record's own
// ObservedAt is the honest answer to "when was this last true".
func (s *Service) status(id model.MachineID, snapshotAt time.Time) (health.Stage, map[health.Level]levelState) {
	obs := s.observationFor(id)
	obs.mu.Lock()
	defer obs.mu.Unlock()

	levels := make(map[health.Level]levelState, len(health.Levels()))
	for _, lvl := range health.Levels() {
		st, ok := obs.levels[lvl]
		if !ok {
			st = levelState{
				confirmedAt: snapshotAt,
				reason:      "not confirmed since holzkube-manager started",
			}
		}
		levels[lvl] = st
	}
	return obs.stage, levels
}

// expiredCertificate reports whether this machine's last failure was an
// expired client certificate.
func (s *Service) expiredCertificate(id model.MachineID) bool {
	obs := s.observationFor(id)
	obs.mu.Lock()
	defer obs.mu.Unlock()
	return obs.expired
}

func (o *observation) enter(stage health.Stage) {
	o.mu.Lock()
	defer o.mu.Unlock()

	// connecting never overwrites a live stage: a heartbeat that is about to
	// re-confirm a watching node must not make it look like it dropped.
	if stage == health.StageConnecting && o.stage.Live() {
		return
	}
	o.stage = stage
}

func (o *observation) confirm(level health.Level, at time.Time) {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.levels[level] = levelState{confirmedAt: at, ok: true}
	if level == health.LevelNode {
		o.failures = 0
		o.expired = false
		o.stage = health.StageWatching
	}
	// A node that answers while one of its upper levels does not is degraded,
	// not down. The distinction is what keeps an etcd outage from being read
	// as five dead machines.
	for _, lvl := range health.Levels() {
		if st, ok := o.levels[lvl]; ok && !st.ok && o.stage == health.StageWatching {
			o.stage = health.StageDegraded
		}
	}
}

func (o *observation) fail(level health.Level, reason string, expired bool) {
	o.mu.Lock()
	defer o.mu.Unlock()

	prev := o.levels[level]
	o.levels[level] = levelState{confirmedAt: prev.confirmedAt, ok: false, reason: reason}

	if level != health.LevelNode {
		if o.stage == health.StageWatching {
			o.stage = health.StageDegraded
		}
		return
	}

	o.expired = expired
	o.failures++
	if o.failures >= downgradeAfter {
		o.stage = health.StageDown
		return
	}
	o.stage = health.StageDegraded
}

// unreachableReason turns a transport failure into a sentence for the screen.
//
// It deliberately does not carry the Go error text: talos.Error already
// classifies the failure into a kind, and the kinds are what an operator can
// act on. A wrapped gRPC status string in a dashboard cell is noise that
// pushes the actionable part off the edge.
func unreachableReason(err error) string {
	if certificateExpired(err) {
		return "the client certificate for this cluster has expired"
	}
	if kind, ok := talos.ErrorKindOf(err); ok {
		switch kind {
		case talos.KindTimeout:
			return "the node accepted the connection and did not answer in time"
		case talos.KindUnreachable:
			return "the node could not be reached"
		case talos.KindRejected:
			return "the node refused the request"
		}
	}
	return "the node could not be reached"
}

// certificateExpired reports the one failure that is not about the node.
//
// An expired client certificate takes every node in a cluster down in the same
// second, and an operator shown five red nodes starts the wrong repair. D-23
// makes it a named state for exactly this reason.
func certificateExpired(err error) bool {
	if err == nil {
		return false
	}

	var invalid x509.CertificateInvalidError
	if errors.As(err, &invalid) && invalid.Reason == x509.Expired {
		return true
	}

	// The typed error above is what a local verification produces. A handshake
	// failure reported by the peer arrives as a gRPC status carrying the same
	// sentence and no structure, so the string is the second line of defence
	// rather than the only one.
	return strings.Contains(err.Error(), "certificate has expired")
}
