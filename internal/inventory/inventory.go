// Package inventory is holzkube-manager's node and cluster inventory: what exists, what
// it is, and how sure we are right now.
//
// Two properties shape everything here.
//
// The inventory is honest. A node that does not answer keeps its record and
// its last known facts, marked with the moment confirmation stopped; it never
// disappears and it never silently becomes an empty row (INV-08). Every fact
// the API serves carries the layer it depends on, so an operator can tell
// "etcd is down" from "the dashboard is broken" (INV-07).
//
// The inventory is keyed on identity. A machine is its UUID; an address is a
// hint that changes. A DHCP lease that moves between two nodes must not swap
// their histories, so a known address answering with an unfamiliar UUID
// produces a new record and leaves the old one alone (D-10).
package inventory

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/health"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

var (
	// ErrNotControlPlane reports an adoption aimed at a node that carries no
	// control-plane material.
	//
	// It is its own error because the remedy is specific and nothing else
	// suggests it: name a control-plane node. A worker's machine
	// configuration carries the OS CA certificate without its private key, so
	// the derivation cannot produce a bundle, and a cluster adopted without
	// one could be watched and never entered again (D-02, D-05).
	ErrNotControlPlane = errors.New("inventory: node carries no control-plane material")

	// ErrClusterLocked reports a mutating operation refused by the
	// per-cluster read-only lock (INV-12, P17).
	ErrClusterLocked = errors.New("inventory: cluster is locked read-only")

	// ErrNotFound reports a cluster or machine that is not in the inventory.
	ErrNotFound = errors.New("inventory: no such record")
)

// Deps is what the service needs.
type Deps struct {
	Store  store.Store
	Dialer talos.Dialer
	Mode   talos.Mode
	Logger *slog.Logger

	// Now is the clock. It is a field for the same reason auth.Service carries
	// one: staleness is the product's central claim, and a test about it has
	// to be able to say what time it is.
	Now func() time.Time

	// Heartbeat is how often a supervisor re-confirms a node. Pattern 7 is
	// explicit that five to twenty nodes need no scheduler, so this is a
	// goroutine per node with jitter and nothing more (D-17).
	Heartbeat time.Duration
}

// DefaultHeartbeat is the re-confirmation interval when Deps does not say.
//
// INV-13 rules out blind polling, and this is not it: the facts come from the
// resource state the node already maintains, and the heartbeat exists to move
// stale_since forward rather than to discover anything. Forty-five seconds
// sits in the middle of the 30-60s band the requirement names.
const DefaultHeartbeat = 45 * time.Second

// Service is the inventory.
type Service struct {
	deps Deps

	mu sync.Mutex

	// observed is the in-memory half of the read model: per machine, what each
	// level last confirmed and when.
	//
	// It is deliberately not persisted. What is persisted is the snapshot
	// itself (D-16), and on restart every level loads as unconfirmed with the
	// snapshot's own timestamp -- which is the truth: holzkube-manager has not
	// confirmed anything since it started.
	observed map[model.MachineID]*observation

	// supervised is which machines have loops running.
	//
	// It is separate from observed, and the separation is a bug this phase
	// found rather than a distinction somebody designed. observed is filled by
	// observationFor, which every read of the read model calls -- so with one
	// map, listing the machines before Start meant Start found an entry for
	// each of them, concluded they were already supervised, and started
	// nothing. The fleet then sat at whatever the last read had recorded, for
	// ever, with no error anywhere. It has not bitten in production because
	// holzkube-managerd calls Start before it serves; it would have bitten the
	// first time an adoption ran on an instance that had served a list.
	supervised map[model.MachineID]struct{}

	// runCtx is the lifetime of the supervisors, set by Start.
	//
	// Supervise uses it rather than its caller's context, which is the second
	// half of the same bug: the HTTP adoption path passed the request's
	// context, so a node adopted through the API got a supervisor that was
	// cancelled the moment the response was written.
	runCtx context.Context //nolint:containedctx // it is the supervisors' lifetime, not a call's

	// stop cancels the running supervisors.
	stop   context.CancelFunc
	wg     sync.WaitGroup
	closed bool
}

// New builds the service. It starts nothing; Start does that.
func New(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	if d.Heartbeat <= 0 {
		d.Heartbeat = DefaultHeartbeat
	}
	return &Service{
		deps:       d,
		observed:   map[model.MachineID]*observation{},
		supervised: map[model.MachineID]struct{}{},
	}
}

// observation is one machine's per-level confirmation state.
//
// The levels are tracked separately because they fail separately (D-17): etcd
// going down must not stop the node-level read, and a node whose kubelet has
// never started is not a node that is down.
type observation struct {
	mu sync.Mutex

	stage  health.Stage
	levels map[health.Level]levelState

	// failures counts consecutive failed passes. It is what moves the stage
	// from degraded to down, so that one missed read does not repaint a
	// working cluster.
	failures int

	// watch is the state of this machine's resource subscription. It is kept
	// beside the levels rather than among them because it is not a level: a
	// level is something a node said, and this is something about how we are
	// listening (INV-13).
	watch watchState

	// expired records that the last failure was an expired client
	// certificate. It is kept apart from every other failure because it is not
	// a node problem at all: five red nodes send an operator to the wrong
	// repair, and the right one is one banner about one certificate (D-23).
	expired bool
}

type levelState struct {
	confirmedAt time.Time
	ok          bool
	reason      string
}

// downgradeAfter is how many consecutive failed passes turn degraded into
// down.
//
// Two, not one: a single failed pass is indistinguishable from a node that was
// busy, and painting the fleet red on it is how an operator learns to ignore
// the colour. Pattern 7 fixes the states and leaves the constants here on
// purpose.
const downgradeAfter = 2

// Close stops the supervisors and waits for them.
func (s *Service) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	stop := s.stop
	s.mu.Unlock()

	if stop != nil {
		stop()
	}
	s.wg.Wait()
	return nil
}
