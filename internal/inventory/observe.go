package inventory

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
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

	// Read, decide, write -- and retry the whole thing rather than the write,
	// because every decision below is made against the record that was read.
	// A refresh landing its snapshot between the two is the ordinary case now
	// that persistSnapshot no longer gives up on the first collision, and
	// there is nothing here that a second look does not simply redo.
	apply := func(rec model.Machine) model.Machine {
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
		return rec
	}

	for range writeAttempts {
		rec, err := s.deps.Store.Machines().Get(ctx, facts.UUID)
		switch {
		case err == nil:
		case errors.Is(err, store.ErrNotFound):
			// Rev stays zero, which is what a create carries. If the machine
			// was filed by somebody else in the meantime the Put conflicts and
			// the next pass reads their record instead of replacing it.
			rec = model.Machine{ID: facts.UUID, AdoptedAt: now, Role: model.RoleUnknown}
		default:
			return model.Machine{}, err
		}

		saved, err := s.deps.Store.Machines().Put(ctx, apply(rec))
		switch {
		case err == nil:
			// A machine in the inventory has an observer. The invariant lives
			// HERE, at the one place machines enter the inventory, and not at
			// each caller -- because as a per-caller obligation it was already
			// forgotten once, by the path that matters most.
			//
			// Adoption recorded every member of the cluster it had just
			// authenticated to and asked nobody to look at any of them, so a
			// freshly imported cluster read "not answering" for every node
			// until the daemon happened to restart and Start picked them up
			// out of the store. TestAnAdoptedNodeIsObservedWithoutARestart is
			// that case.
			//
			// supervise is idempotent and refuses before Start, so recording a
			// machine while the service is not running is still the no-op it
			// was; the warning it logs is the honest one.
			s.supervise(saved.ID)
			return saved, nil
		case errors.Is(err, store.ErrConflict):
			continue
		default:
			return model.Machine{}, err
		}
	}

	return model.Machine{}, fmt.Errorf("%w: the record for machine %s changed under %d successive attempts",
		store.ErrConflict, facts.UUID, writeAttempts)
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
	s.runCtx = runCtx
	s.mu.Unlock()

	for _, m := range machines {
		s.supervise(m.ID)
	}
	return nil
}

// Supervise starts an observer for one machine if it does not already have one.
//
// It is no longer what any handler calls. recordMachine supervises what it
// records, so the invariant "a machine in the inventory has an observer" is
// held at the single place machines enter the inventory rather than by each
// caller remembering to ask -- which is how the adoption path came to record a
// whole cluster and ask for none of them. This stays exported for the one thing
// a caller can legitimately want that recording does not express: starting an
// observer for a machine that is already on disk.
//
// It takes no context on purpose. A supervisor's lifetime is the service's,
// fixed by Start; an HTTP handler that supplied the context would be supplying
// its own request's -- which cancels when the response is written, leaving the
// node that was just adopted with a supervisor that ran for a few milliseconds
// and stopped. That is what this signature used to allow and what a handler
// actually did.
func (s *Service) Supervise(id model.MachineID) {
	s.supervise(id)
}

func (s *Service) supervise(id model.MachineID) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	if _, running := s.supervised[id]; running {
		s.mu.Unlock()
		return
	}
	ctx := s.runCtx
	if ctx == nil {
		// Nothing has started the service, so there is no lifetime to attach
		// to. Refusing is the honest answer: a supervisor on a context that
		// outlives nothing would be a goroutine nobody can stop.
		s.mu.Unlock()
		s.deps.Logger.Warn("a machine was supervised before the inventory was started",
			slog.String("machine", string(id)))
		return
	}
	s.supervised[id] = struct{}{}
	if _, ok := s.observed[id]; !ok {
		s.observed[id] = newObservation()
	}
	s.mu.Unlock()

	// Two loops, not one, and the second does not replace the first (INV-13,
	// D-19). The heartbeat re-confirms on its own timer whatever the watch is
	// doing, because a subscription that stops delivering without saying so is
	// the failure the poll exists against; the watch supplies the latency the
	// poll cannot.
	s.wg.Add(2)
	go func() {
		defer s.wg.Done()
		s.observeLoop(ctx, id)
	}()
	go func() {
		defer s.wg.Done()
		s.watchLoop(ctx, id)
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
	// The pass carries its own ceiling, because two of its four callers hand
	// it the supervisors' lifetime, and in production that lifetime has no
	// deadline. Every Talos call on such a context is refused before it leaves
	// the process (D-04, ErrNoDeadline), so each heartbeat pass failed, the
	// node went to StageDown, and the watchdog tore down a watch that was
	// working -- on a node that was up, every forty-five seconds, forever.
	// A caller with a shorter budget keeps it: WithTimeout takes the earlier.
	ctx, cancel := context.WithTimeout(ctx, RefreshBudget)
	defer cancel()

	rec, err := s.deps.Store.Machines().Get(ctx, id)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			s.deps.Logger.Error("inventory refresh could not read the machine record",
				slog.String("machine", string(id)), slog.Any("error", err))
		}
		return
	}

	obs := s.observationFor(id)

	// A machine whose cluster was forgotten survives as a record on purpose
	// (ForgetCluster), and nothing holds credentials to ask it with. That is
	// not a failure to answer, so it is not counted as one: before this it went
	// to StageDown after two passes and /metrics called every such machine
	// down, with "store: invalid key: empty" as the reason.
	if rec.Cluster == "" {
		if obs.unassigned() {
			s.deps.Logger.Info("a machine belongs to no cluster, so nothing can ask it until it is adopted again",
				slog.String("machine", string(id)))
		}
		return
	}

	obs.enter(health.StageConnecting)

	creds, err := s.clusterCreds(ctx, rec.Cluster)
	if err != nil {
		if obs.fail(health.LevelNode, "this machine belongs to no cluster with stored credentials", false) {
			s.deps.Logger.Warn("a machine cannot be observed because its cluster has no stored credentials",
				slog.String("machine", string(id)),
				slog.String("cluster", string(rec.Cluster)),
				slog.Any("error", err))
		}
		return
	}

	cc, err := talos.NewClusterClient(ctx, s.deps.Dialer, talos.Target{
		Cluster: rec.Cluster,
		Machine: rec.ID,
		Addr:    rec.Addr,
	}, creds, s.deps.Mode)
	if err != nil {
		if obs.fail(health.LevelNode, unreachableReason(err), certificateExpired(err)) {
			// The ADDRESS is here on purpose. A record can be filed against an
			// address the machine does not answer at -- adoption takes the
			// first one a member reports -- and "the node stopped answering"
			// without saying where it was asked is indistinguishable from a
			// node that is actually off.
			s.deps.Logger.Warn("a node did not answer",
				slog.String("machine", string(id)),
				slog.String("addr", rec.Addr),
				slog.String("reason", unreachableReason(err)),
				slog.Any("error", err))
		}
		return
	}
	defer cc.Close() //nolint:errcheck // an observation's verdict is not a close error's to change

	now := s.deps.Now().UTC()
	snap := model.MachineSnapshot{ObservedAt: now}
	previous := rec.Snapshot

	facts, err := cc.NodeFacts(ctx)
	if err != nil {
		if obs.fail(health.LevelNode, unreachableReason(err), certificateExpired(err)) {
			// Connected and then could not be read, which is a DIFFERENT repair
			// from not connecting: the machine is present and something above
			// the transport refused. SeenAt moves and the snapshot does not,
			// and this line is what makes that pair legible in a journal.
			s.deps.Logger.Warn("a node answered the connection and not the question",
				slog.String("machine", string(id)),
				slog.String("addr", rec.Addr),
				slog.String("reason", unreachableReason(err)),
				slog.Any("error", err))
		}

		// The connection was made, so the node answered something: this is a
		// machine that is present and cannot be read, which is a different
		// finding from one that is absent. Recording it is best-effort -- the
		// observation's verdict above is the answer, and a failed bookkeeping
		// write must not turn a reachable node into an error.
		if seenErr := s.persistSeen(ctx, rec, now); seenErr != nil {
			s.deps.Logger.Debug("could not record that the machine answered",
				"machine", rec.ID, "error", seenErr)
		}
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
			Socket:       c.Socket,
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

	if err := s.persistSnapshot(ctx, rec, snap, now, facts.Hostname); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// The machine was forgotten while its refresh was in flight. Its
			// absence is the answer, and a Put here would recreate it.
			return
		}
		s.deps.Logger.Error("inventory refresh could not persist the snapshot",
			slog.String("machine", string(id)), slog.Any("error", err))
	}
}

// writeAttempts bounds the compare-and-swap loops in this file.
//
// Three rather than one because the first attempt is the one that raced, and
// rather than unbounded because a machine whose record is being rewritten
// faster than a read and a write can complete has a different problem than a
// lost update, and spinning on it would hide that problem behind a busy loop.
// A writer that loses three times in a row says so instead.
const writeAttempts = 3

// persistSnapshot writes an observation onto the machine record, merging
// rather than overwriting when another writer got there first.
//
// Refresh reads the record at the top and writes it back at the bottom, and
// between those two points it talks to a node over the network -- the widest
// window in this service. Two writers legitimately live in that window: the
// supervisor's heartbeat and a manual POST /machines/{id}/refresh can overlap,
// and recordMachine can file a moved address for the same machine while a
// refresh is in flight. The loser used to receive store.ErrConflict, log it,
// and drop the round's observation on the floor.
//
// Retrying with the record read at the top would be the opposite bug: it would
// hand back the address, role, cluster and lock as they were before the winner
// changed them, which is a silent undo of somebody's write dressed up as a
// heartbeat. So the retry re-reads and applies only the three fields an
// observation owns -- the snapshot, the time it was taken, and the hostname
// the node itself just reported -- and leaves every other field at whatever
// the winner wrote.
func (s *Service) persistSnapshot(
	ctx context.Context,
	rec model.Machine,
	snap model.MachineSnapshot,
	now time.Time,
	hostname string,
) error {
	apply := func(m model.Machine) model.Machine {
		m.Snapshot = snap
		m.SeenAt = now
		if hostname != "" {
			m.Hostname = hostname
		}
		return m
	}

	return s.merge(ctx, rec, apply)
}

// merge applies a change to a machine record, re-reading and re-applying if
// somebody else wrote it first.
//
// The retry is what makes an observation a merge rather than a last-writer
// -wins overwrite: the observer and an operator's action write the same record
// from different fields, and a lost revision race used to throw the
// observation away silently.
func (s *Service) merge(ctx context.Context, rec model.Machine, apply func(model.Machine) model.Machine) error {
	for attempt := range writeAttempts {
		if attempt > 0 {
			fresh, err := s.deps.Store.Machines().Get(ctx, rec.ID)
			if err != nil {
				return err
			}
			rec = fresh
		}

		_, err := s.deps.Store.Machines().Put(ctx, apply(rec))
		switch {
		case err == nil:
			return nil
		case errors.Is(err, store.ErrConflict):
			continue
		default:
			return err
		}
	}

	return fmt.Errorf("%w: the machine record changed under %d successive attempts",
		store.ErrConflict, writeAttempts)
}

// persistSeen records that the machine answered, without claiming to have read
// anything from it.
//
// It exists because "nothing is there" and "it is there and cannot be read"
// are different findings that lead to different repairs -- the first sends an
// operator to the cable or the power, the second to the node itself -- and
// until this, they were the same record. A node whose connection succeeded and
// whose facts read failed persisted nothing at all, so its SeenAt stayed at
// the last time a *complete* observation worked, which is not what SeenAt says
// it is: "when the machine last answered anything at all".
//
// The snapshot is deliberately not touched. Nothing was read, so there is
// nothing to write, and stamping the old snapshot with a new time would turn a
// stale reading into one that looks current -- which is the failure the whole
// Field[T] read model exists against.
func (s *Service) persistSeen(ctx context.Context, rec model.Machine, now time.Time) error {
	return s.merge(ctx, rec, func(m model.Machine) model.Machine {
		m.SeenAt = now
		return m
	})
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

// unassigned puts the observer back to "nobody has asked", and reports whether
// that changed anything, so the journal says it once rather than every pass.
func (o *observation) unassigned() bool {
	o.mu.Lock()
	defer o.mu.Unlock()

	changed := o.stage != health.StageUnknown || o.failures != 0
	o.stage = health.StageUnknown
	o.failures = 0
	o.expired = false
	return changed
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

// fail records a level's failure and reports whether this one is worth saying
// out loud.
//
// The bool is the whole reason this returns anything. A node that is down stays
// down, and the heartbeat asks again every interval, so logging each failure
// turns the journal into one sentence repeated until the disk fills. Logging
// none of them is what shipped: the daemon wrote "the node stopped answering"
// every interval and never once said what it got back, and diagnosing a real
// cluster then needed a second round trip to the operator.
//
// Worth saying is a CHANGE: the first failure after the node was fine, and the
// moment the count crosses into StageDown. Everything between is the same fact
// again.
func (o *observation) fail(level health.Level, reason string, expired bool) bool {
	o.mu.Lock()
	defer o.mu.Unlock()

	prev := o.levels[level]
	o.levels[level] = levelState{confirmedAt: prev.confirmedAt, ok: false, reason: reason}

	if level != health.LevelNode {
		if o.stage == health.StageWatching {
			o.stage = health.StageDegraded
		}
		return prev.ok || prev.reason != reason
	}

	o.expired = expired
	o.failures++

	// The COUNT decides, not the stage. Refresh calls enter(StageConnecting)
	// before every attempt, so by the time this runs the stage says "trying"
	// and never "already down" -- deriving the answer from it logged the same
	// failure on every heartbeat, which is the defect this returns a bool for
	// in the first place. Found by the test, not by reading.
	crossed := o.failures == downgradeAfter
	if o.failures >= downgradeAfter {
		o.stage = health.StageDown
	} else {
		o.stage = health.StageDegraded
	}
	return o.failures == 1 || crossed || prev.reason != reason
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
