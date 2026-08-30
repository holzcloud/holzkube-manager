package talos

// Per-node circuit breaker (TRANS-05, T-02-29, T-02-30).
//
// This is internal/auth.Limiter's data structure keyed by machine id instead of
// by source address, and its justifications transfer verbatim: per-key state
// under one mutex, a sweep driven by the only method that adds an entry, and a
// Len that exists so a test can assert the map does not grow without bound.
// There is no background goroutine, for the reason Limiter gives -- a lifetime
// goroutine needs a shutdown path and somewhere to be stopped in every test
// that builds one.
//
// What the breaker is for: an unreachable node must cost one unreachable
// node's worth of work. Without it, every screen that lists an inventory pays
// the full class deadline for every node that is down, every time it refreshes.

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

const (
	// BreakerFailureThreshold is how many consecutive transport failures open a
	// node's circuit. Three, because one is noise and two is a coincidence: a
	// single dropped packet must not take a node out of the inventory.
	BreakerFailureThreshold = 3

	// BreakerCooldown is how long an open circuit refuses before it lets one
	// trial call through. It is longer than any class deadline, so a node that
	// is merely slow is not probed while the previous probe is still running.
	BreakerCooldown = 30 * time.Second

	// breakerRetention is how long a machine nobody has heard from is
	// remembered. Anything older is indistinguishable from a machine that was
	// never seen, and keeping it is a slow leak in a process meant to run for
	// months (T-02-30).
	breakerRetention = time.Hour

	// breakerSweepInterval bounds how often the map is walked.
	breakerSweepInterval = time.Minute
)

// ErrCircuitOpen reports that a node's circuit is open and the call was
// short-circuited without dialing.
//
// It is deliberately distinguishable from a transport failure: "we did not try"
// and "we tried and nothing answered" are different facts, and an operator
// looking at a node that is being skipped needs to be told it is being skipped.
var ErrCircuitOpen = errors.New("talos: circuit open, the call was not attempted")

// Breaker tracks consecutive transport failures per machine.
//
// The zero value is not usable; call NewBreaker.
type Breaker struct {
	mu    sync.Mutex
	nodes map[model.MachineID]*breakerState

	lastSweep time.Time

	// now is the clock, so the cooldown and the retention window can be walked
	// out in a test without waiting the minutes they describe.
	now func() time.Time
}

// breakerState is one machine's recent history.
type breakerState struct {
	failures int
	openedAt time.Time
	touched  time.Time
}

// NewBreaker returns an empty breaker.
func NewBreaker() *Breaker {
	return &Breaker{
		nodes: make(map[model.MachineID]*breakerState),
		now:   time.Now,
	}
}

// Allow reports whether a call to this machine may be attempted.
//
// An open circuit lets exactly one trial call through once the cooldown has
// elapsed, and starts the cooldown again as it does -- so a second caller
// arriving in the same instant is still refused, and one trial is one trial
// rather than one per goroutine.
func (b *Breaker) Allow(machine model.MachineID) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	s, ok := b.nodes[machine]
	if !ok || s.failures < BreakerFailureThreshold {
		return nil
	}

	now := b.now()
	if now.Sub(s.openedAt) < BreakerCooldown {
		return fmt.Errorf("talos: %s: %w (%d consecutive transport failures, next trial in %s)",
			machine, ErrCircuitOpen, s.failures, (BreakerCooldown - now.Sub(s.openedAt)).Round(time.Second))
	}

	// Half-open: this caller is the trial. Restarting the cooldown here is what
	// makes it a single trial rather than a flood the moment the window opens.
	s.openedAt = now
	s.touched = now
	return nil
}

// Success records that the machine answered, and forgets its failures.
//
// Proving the node is reachable ends the episode, exactly as auth.Limiter's
// Succeed does: the node that dropped two packets is not still being skipped an
// hour later.
func (b *Breaker) Success(machine model.MachineID) {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.nodes, machine)
}

// Fail records one consecutive transport failure.
//
// The sweep runs from here because this is the only method that adds an entry,
// so the map cannot grow between sweeps.
func (b *Breaker) Fail(machine model.MachineID) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := b.now()
	b.sweep(now)

	s, ok := b.nodes[machine]
	if !ok {
		s = &breakerState{}
		b.nodes[machine] = s
	}
	s.failures++
	s.touched = now
	if s.failures == BreakerFailureThreshold {
		s.openedAt = now
	}
}

// Observe folds one call's outcome into the breaker, and is the only place the
// rule about which failures count is written down.
//
// A transport failure -- nothing answered, or nothing answered in time -- is
// what the breaker exists for. A refusal is not: etcd_down is a node answering
// codes.Unavailable for every Etcd* RPC while Version keeps working, and
// opening that node's circuit would turn one broken subsystem into a node-wide
// outage. A refusal therefore leaves the count exactly as it found it -- it
// neither advances it, because the transport is fine, nor clears it, because an
// answer from one subsystem is not evidence about the calls that were timing
// out.
//
// Anything that is not a classified transport failure at all -- a cancellation,
// a refusal this package made before the wire -- is likewise ignored. A call
// the caller abandoned says nothing about the node, and counting it would let
// one cancelled fan-out open every circuit in the inventory.
func (b *Breaker) Observe(machine model.MachineID, err error) {
	if err == nil {
		b.Success(machine)
		return
	}

	kind, ok := ErrorKindOf(err)
	if !ok {
		return
	}

	switch kind {
	case KindUnreachable, KindTimeout:
		b.Fail(machine)
	case KindRejected:
		// Deliberately nothing.
	}
}

// Len reports how many machines are being remembered. It exists so a test can
// assert the map does not grow without bound (T-02-30).
func (b *Breaker) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	return len(b.nodes)
}

// Failures reports the consecutive transport failure count for one machine.
//
// It exists so a test can assert that a refusal left the count where it was,
// which is a claim about a number and cannot be made by observing whether the
// circuit happens to be open.
func (b *Breaker) Failures(machine model.MachineID) int {
	b.mu.Lock()
	defer b.mu.Unlock()

	if s, ok := b.nodes[machine]; ok {
		return s.failures
	}
	return 0
}

// sweep drops machines nobody has heard from in a while.
func (b *Breaker) sweep(now time.Time) {
	if now.Sub(b.lastSweep) < breakerSweepInterval {
		return
	}
	b.lastSweep = now

	for machine, s := range b.nodes {
		if now.Sub(s.touched) >= breakerRetention {
			delete(b.nodes, machine)
		}
	}
}
