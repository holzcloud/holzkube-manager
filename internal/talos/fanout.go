package talos

// Per-node fan-out (TRANS-01, TRANS-05, T-02-29, T-02-31).
//
// The property this file exists for is one sentence: an unreachable node costs
// one unreachable node's worth of work. Not a shared lock every other node
// queues behind, not a serial loop where node three waits out node one's
// deadline, and not one audit record per node.

import (
	"context"

	"golang.org/x/sync/errgroup"
)

// FanOutConcurrency bounds how many nodes are contacted at once.
//
// A hundred-node inventory opening a hundred TLS connections in one instant is
// a burst the product inflicts on itself, and on whatever switch is between it
// and the rack. Eight is enough that a fan-out over a homelab finishes in one
// round trip and small enough that a large one degrades into batches rather
// than into a connection storm.
const FanOutConcurrency = 8

// Result is one target's outcome. Exactly one of Value and Err is meaningful,
// and Target says which node either belongs to -- by identity, never by
// address, because a fan-out over an inventory is exactly where an
// address-keyed result would be attributed to the wrong node after a DHCP
// lease change.
type Result[T any] struct {
	Target Target
	Value  T
	Err    error
}

// FanOut calls every target concurrently and returns every outcome.
//
// It returns results rather than an error, and it never stops early. A fan-out
// that abandoned the remaining nodes on the first failure would make the health
// of the whole inventory depend on which node happened to answer first, which
// is the serialisation TRANS-05 forbids in a different costume.
//
// Each target gets its own goroutine and therefore its own deadline: the
// per-node budget belongs inside call, where WithClassDeadline applies it, so
// one node that has gone silent holds up nothing but its own goroutine.
//
// **It writes no audit record and takes no audit dependency, deliberately.**
// internal/audit is fail-closed and fsyncs every record under one global mutex,
// and it was sized on the assumption of a handful of records per minute. A
// fan-out that audited per node would make that mutex the bottleneck of every
// inventory operation, and because the writer is fail-closed the contention
// would surface as *mutation failure* rather than as latency (T-02-31).
// Recording an operation is the caller's single decision, not N of them.
//
// breaker may be nil. When it is not, each target is checked before its call is
// made and the outcome is folded back in, so a node whose circuit is open costs
// a map lookup instead of a connection.
func FanOut[T any](
	ctx context.Context,
	breaker *Breaker,
	targets []Target,
	call func(context.Context, Target) (T, error),
) []Result[T] {
	results := make([]Result[T], len(targets))

	// A plain errgroup rather than errgroup.WithContext: the context variant
	// cancels the group on the first error, which is precisely the behaviour
	// this function must not have. The callbacks below never return an error,
	// so Wait is used only for its bounded-concurrency and join semantics.
	var g errgroup.Group
	g.SetLimit(FanOutConcurrency)

	for i, target := range targets {
		g.Go(func() error {
			results[i] = Result[T]{Target: target}

			if breaker != nil {
				// A refused call is this target's outcome, not the group's: an
				// open circuit on one node must not end the fan-out over the
				// others. The nil below is deliberate and is why the group is a
				// plain errgroup rather than the context variant.
				if err := breaker.Allow(target.Machine); err != nil {
					results[i].Err = err
					return nil //nolint:nilerr // the error is this target's result, never the group's
				}
			}

			value, err := call(ctx, target)
			results[i].Value = value
			results[i].Err = err

			if breaker != nil {
				breaker.Observe(target.Machine, err)
			}
			return nil
		})
	}

	// The error is always nil by construction; it is discarded rather than
	// returned so that no caller writes an if that can never be true.
	_ = g.Wait()

	return results
}
