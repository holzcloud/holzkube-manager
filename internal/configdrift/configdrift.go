// Package configdrift reads the configuration of every node of every cluster,
// remembers what it first saw as the baseline, and says when a node no longer
// matches it.
//
// It is built after internal/certwatch -- a periodic read, held in memory, one
// node that does not answer never taking the others with it, and "not read yet"
// until a pass has been through -- and differs from it in three ways that come
// from the research behind this feature:
//
//   - It reads every node, control plane or not, and skips only the ones that
//     are switched off (a node with Disabled set, and every node of a cluster
//     with Disabled set). A cluster that is merely stopped is not skipped: it
//     costs each node up to the five seconds the client's probe takes, one node
//     after the other, and that is a stated limit of the feature.
//   - It keeps state on disk: the baseline and the start of a running
//     deviation (DriftSince) live in the data directory, so that neither starts
//     over with a restart of the daemon.
//   - It carries its own budget per node. The Talos client refuses every call
//     without a deadline and the daemon starts its background loops on
//     context.Background(); certwatch lacked exactly this (ledger 137,
//     Deps.ReadBudget). The reader here gives Read a context with a deadline,
//     and the tests start from one without.
//
// The one rule that shapes the code: a reading that failed is never "no
// drift". Every path that does not end in a compared snapshot ends in a state
// that says "unknown", and the last good reading and a stored DriftSince
// survive it.
package configdrift

import (
	"context"
	"sync"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/machineconfig"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
)

// Defaults for Deps.
const (
	// DefaultEvery is the period between passes.
	DefaultEvery = 3 * time.Hour

	// DefaultReadBudget bounds one node's connection and read together.
	DefaultReadBudget = 20 * time.Second

	// DefaultSettleWindow is how long a difference after an apply from this
	// daemon is expected before it counts as drift (StateSettling).
	DefaultSettleWindow = 15 * time.Minute
)

// pokeBuffer is how many pokes wait for a read. A poke beyond it is dropped: the
// next pass reads the node anyway.
const pokeBuffer = 64

// Deps is what the service needs from the rest of the program.
type Deps struct {
	// Clusters lists the clusters.
	Clusters func(ctx context.Context) ([]model.Cluster, error)

	// Machines lists the nodes that exist in one cluster (inventory.MachinesOf).
	Machines func(ctx context.Context, id model.ClusterID) ([]model.Machine, error)

	// AllMachines lists every inventory record, for Detail's "no such machine".
	AllMachines func(ctx context.Context) ([]model.Machine, error)

	// Read returns the snapshot of one node's active configuration. In the
	// daemon it is machineconfig.Service.ReadSnapshot; the raw bytes never
	// leave that package.
	Read func(ctx context.Context, id model.MachineID) (machineconfig.Snap, error)

	// Baselines holds one baseline per node.
	Baselines store.ConfigBaselineStore

	// Every is the period between passes. Zero means DefaultEvery.
	Every time.Duration

	// ReadBudget is how long one node may take, connection and read together.
	// Zero means DefaultReadBudget. It is put on the context handed to Read
	// because the context Run is started with has no deadline of its own.
	ReadBudget time.Duration

	// SettleWindow: see DefaultSettleWindow. Zero means the default.
	SettleWindow time.Duration

	// Now defaults to time.Now.
	Now func() time.Time

	// Log gets one line per change of a node's state, never per pass.
	Log func(msg string, args ...any)
}

// reading is what the last pass learned about one node. It is held in memory
// and starts empty with the process.
type reading struct {
	state State

	// at is the last attempt; lastGood is the last reading that worked.
	at       time.Time
	lastGood time.Time

	// snap is the snapshot of the last good reading and survives a failed one.
	snap machineconfig.Snap

	// since is the start of the running deviation, zero when there is none. It
	// mirrors the record's DriftSince and survives a failed reading.
	since time.Time

	sentence string
	cluster  model.ClusterID
	node     string
}

// pass is what the last pass over one cluster covered.
type pass struct {
	at time.Time

	// nodes counts the nodes read or attempted, switched-off ones left out;
	// read counts those that were compared with a baseline or founded one.
	nodes int
	read  int
}

// Service holds the last reading of each node.
type Service struct {
	deps Deps

	// mu protects readings, passes and logged.
	mu       sync.Mutex
	readings map[model.MachineID]*reading
	passes   map[model.ClusterID]pass
	logged   map[model.MachineID]State

	// pokes carries the nodes that were asked for out of turn.
	pokes chan model.MachineID

	// storeMu serialises read-modify-write of one baseline. The store refuses a
	// stale write by itself; this keeps two passes of the service from reaching
	// it at the same time at all.
	storeMu sync.Mutex
}

// New builds the service.
func New(d Deps) *Service {
	if d.Every == 0 {
		d.Every = DefaultEvery
	}
	if d.ReadBudget == 0 {
		d.ReadBudget = DefaultReadBudget
	}
	if d.SettleWindow == 0 {
		d.SettleWindow = DefaultSettleWindow
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Log == nil {
		d.Log = func(string, ...any) {}
	}
	return &Service{
		deps:     d,
		readings: map[model.MachineID]*reading{},
		passes:   map[model.ClusterID]pass{},
		logged:   map[model.MachineID]State{},
		pokes:    make(chan model.MachineID, pokeBuffer),
	}
}

// Run reads once shortly after start and then every period until ctx ends. A
// node that was poked is read without waiting for the next period.
//
// ctx is the daemon's, with no deadline of its own; every read gets its budget
// from readNode.
func (s *Service) Run(ctx context.Context, firstAfter time.Duration) {
	timer := time.NewTimer(firstAfter)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			s.Tick(ctx)
			s.Flush(ctx)
			timer.Reset(s.deps.Every)
		case id := <-s.pokes:
			s.flush(ctx, []model.MachineID{id})
		}
	}
}

// Tick clears up after machines that are gone, then makes one pass over every
// cluster that is not switched off.
//
// A cluster with Disabled set is skipped: its nodes are off on purpose, and a
// read would cost each of them up to the probe's deadline and say nothing. Its
// last reading stays with its age, and a cluster never read says "not read
// yet".
func (s *Service) Tick(ctx context.Context) {
	// First, so that a node that changed its cluster is not compared with the
	// baseline it had in the cluster it left.
	s.reconcile(ctx)

	clusters, err := s.deps.Clusters(ctx)
	if err != nil {
		s.deps.Log("configuration drift: list the clusters", "err", err)
		return
	}
	for _, c := range clusters {
		if ctx.Err() != nil {
			return
		}
		if c.Disabled {
			continue
		}
		s.Refresh(ctx, c.ID)
	}
}

// Refresh reads one cluster's nodes now, one after the other.
func (s *Service) Refresh(ctx context.Context, cluster model.ClusterID) {
	machines, err := s.deps.Machines(ctx, cluster)
	if err != nil {
		s.deps.Log("configuration drift: list the nodes", "cluster", string(cluster), "err", err)
		return
	}

	present := make(map[model.MachineID]bool, len(machines))
	p := pass{}
	for _, m := range machines {
		present[m.ID] = true
		if m.Disabled {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		p.nodes++
		if s.readNode(ctx, cluster, m) {
			p.read++
		}
	}

	s.mu.Lock()
	p.at = s.deps.Now()
	s.passes[cluster] = p
	// A node that is gone from the list loses its reading. Its record stays;
	// clearing up after vanished machines is not this package's job.
	for id, r := range s.readings {
		if r.cluster == cluster && !present[id] {
			delete(s.readings, id)
			delete(s.logged, id)
		}
	}
	s.mu.Unlock()
}

// readNode reads one node under its own budget and records the outcome. It
// reports whether the node was compared.
func (s *Service) readNode(ctx context.Context, cluster model.ClusterID, m model.Machine) bool {
	readCtx, cancel := context.WithTimeout(ctx, s.deps.ReadBudget)
	snap, err := s.deps.Read(readCtx, m.ID)
	cancel()
	if err != nil {
		s.unreadable(ctx, cluster, m, reasonUnreadable(err))
		return false
	}
	return s.decide(ctx, cluster, m, snap)
}
