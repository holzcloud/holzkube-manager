package inventory

// The watch half of "watch primary, poll as heartbeat" (INV-13, D-19).
//
// What this adds to observe.go is latency and nothing else. A watch says a
// topic changed; the observation pass that was already there does the reading.
// The two facts that follow from that are worth stating before the code:
//
// The heartbeat is not replaced. It re-confirms on its own timer whether or not
// a watch is running, because a subscription's characteristic failure is to
// stop delivering without saying so -- and a poller that switched itself off
// once a watch was up would make exactly that failure invisible. The cost of
// keeping it is one read per node per forty-five seconds, which is what the
// product already paid.
//
// A watch is never a confirmation. Only a pass that read something moves a
// level's confirmed_at, so a node whose watch is live and whose reads have all
// failed is a node that is down, and it says so. The watch's own state is
// reported separately, for the operator rather than for the state machine: a
// flapping watch on a node the heartbeat reaches is a note, not an alarm.

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/health"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

const (
	// WatchSettle is how long a change is allowed to gather company before the
	// refresh it triggers.
	//
	// One operator action moves several resources at once -- a config apply
	// changes the configuration, the hostname and the addresses in the same
	// second -- and without this each of them would start its own full read of
	// the node. It is short enough that nobody watching the screen can tell it
	// from immediate, which is the whole budget this phase is spending.
	WatchSettle = 750 * time.Millisecond

	// WatchRebuildFloor and WatchRebuildCeiling bound the rebuild of a watch
	// that died.
	//
	// The minimum is not zero and the reason is the failure it is most likely
	// to meet: a node that is down refuses the subscription immediately, so a
	// rebuild with no floor is a tight loop against an unreachable machine.
	// The maximum is below the heartbeat on purpose -- a watch that is out for
	// longer than a poll interval has stopped being the primary path, and the
	// poller is already covering it.
	WatchRebuildFloor = 2 * time.Second

	// WatchRebuildCeiling is the other end of that bound. See above: it is
	// below the heartbeat on purpose.
	WatchRebuildCeiling = 40 * time.Second
)

// watchState is one machine's subscription as the screen reports it.
type watchState struct {
	// live is set only once the watch has delivered its initial snapshot.
	// A subscription that has been constructed and has not yet said what the
	// node currently holds is not watching anything -- it is connecting -- and
	// reporting it as live would claim a freshness nobody has established.
	live bool

	since    time.Time
	reason   string
	restarts int
}

// WatchStatus is the watch as the API serves it.
//
// It is deliberately not folded into health.Stage. A watch that cannot be
// established while the heartbeat is reading the node fine means the node is
// answering and the operator's changes will take up to a heartbeat to appear --
// which is the product's old behaviour, not a fault in the node. Painting that
// node degraded would put an alarm colour on a working machine and teach the
// operator to ignore the colour.
type WatchStatus struct {
	Live   bool      `json:"live"`
	Since  time.Time `json:"since,omitzero"`
	Reason string    `json:"reason,omitempty"`

	// Restarts counts how often this machine's watch has had to be rebuilt
	// since holzkube-manager started. A watch that is live and has restarted
	// two hundred times is the failure this field exists to make visible: it
	// looks healthy at every instant and is delivering nothing between them.
	Restarts int `json:"restarts"`
}

// watchLoop keeps one machine's subscription up for as long as the supervisor
// runs, rebuilding it with backoff when it ends.
func (s *Service) watchLoop(ctx context.Context, id model.MachineID) {
	attempt := 0

	for {
		delivered := s.watchOnce(ctx, id)
		if ctx.Err() != nil {
			return
		}

		// A watch that delivered its snapshot before dying was a working watch,
		// so the next rebuild starts from the floor rather than from wherever
		// the last failure had climbed to. Without this a node that is
		// unreachable for ten minutes and then comes back waits out the full
		// backoff it accumulated while it was gone.
		if delivered {
			attempt = 0
		} else {
			attempt++
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(watchBackoff(attempt)):
		}
	}
}

// watchBackoff doubles from the floor to the ceiling, with the same jitter the
// heartbeat uses and for the same reason: a fleet that lost its cluster
// certificate retries in one synchronised burst otherwise.
func watchBackoff(attempt int) time.Duration {
	d := WatchRebuildFloor
	for range attempt {
		d *= 2
		if d >= WatchRebuildCeiling {
			d = WatchRebuildCeiling
			break
		}
	}
	return jitter(d)
}

// watchOnce runs one subscription from construction to death and reports
// whether it ever delivered its snapshot.
func (s *Service) watchOnce(ctx context.Context, id model.MachineID) bool {
	rec, err := s.deps.Store.Machines().Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// The machine was removed while its supervisor was running. There
			// is nothing to watch and nothing to report.
			return false
		}
		s.setWatch(id, false, "the machine record could not be read")
		return false
	}

	creds, err := s.clusterCreds(ctx, rec.Cluster)
	if err != nil {
		s.setWatch(id, false, "this machine belongs to no cluster with stored credentials")
		return false
	}

	cc, err := talos.NewClusterClient(ctx, s.deps.Dialer, talos.Target{
		Cluster: rec.Cluster,
		Machine: rec.ID,
		Addr:    rec.Addr,
	}, creds, s.deps.Mode)
	if err != nil {
		s.setWatch(id, false, unreachableReason(err))
		return false
	}
	defer cc.Close() //nolint:errcheck // the watch's verdict is not a close error's to change

	watch := cc.Watch(ctx)
	defer watch.Close() //nolint:errcheck // Close only cancels and waits

	// The watchdog. A watch does not notice a node that has gone away -- see
	// the note at the top of internal/talos/watch.go, which measured it -- so
	// the thing that notices is the poller, and this is where its verdict is
	// applied. Without it a node can be unplugged and its subscription will go
	// on reporting itself live for a quarter of an hour.
	//
	// It is the concrete reason the heartbeat could not be removed once the
	// watch existed. The poll is not a fallback kept out of caution; it is the
	// only liveness signal this pair has.
	stopWatchdog := make(chan struct{})
	defer close(stopWatchdog)

	go s.watchdog(ctx, id, watch, stopWatchdog)

	delivered := false

	for ev := range watch.Events() {
		if ev.Snapshot {
			delivered = true
			s.setWatch(id, true, "")
			continue
		}
		if !delivered {
			// The seam does not emit changes before the snapshot, so this is
			// unreachable rather than tolerated. It is here because the
			// alternative -- refreshing on it -- would silently accept a
			// contract change that made every startup a burst of reads.
			continue
		}

		s.settle(ctx, watch.Events())
		s.Refresh(ctx, id)
	}

	if ctx.Err() != nil {
		return delivered
	}

	reason := "the node ended the resource watch"
	if err := watch.Err(); err != nil {
		reason = watchReason(err)
	} else if s.stageOf(id) == health.StageDown {
		// The watchdog closed it. Saying so is the difference between an
		// operator reading "the watch stopped for no stated reason" and
		// reading the one sentence that explains both halves of what is on
		// the screen.
		reason = "the node stopped answering, so its resource watch was closed rather than left claiming to be live"
	}
	s.setWatch(id, false, reason)
	s.deps.Logger.Info("a node's resource watch ended",
		slog.String("machine", string(id)), slog.String("reason", reason))

	return delivered
}

// watchdog closes a subscription the heartbeat has stopped believing in.
//
// The trigger is StageDown and not the first failed read. A node that missed
// one pass was very likely busy, and tearing down and rebuilding its watch for
// that would make a working fleet churn its subscriptions; StageDown already
// means two consecutive passes failed, which is the point at which the
// inventory itself stops claiming the node is there. A watch that outlived that
// claim is reporting a freshness the rest of the read model has withdrawn.
func (s *Service) watchdog(ctx context.Context, id model.MachineID, watch *talos.Watch, stop <-chan struct{}) {
	// Checked on the heartbeat's own interval, because the heartbeat is what
	// produces the verdict being read: asking more often than it answers only
	// re-reads the same value.
	ticker := time.NewTicker(s.deps.Heartbeat)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		case <-ticker.C:
		}

		obs := s.observationFor(id)
		obs.mu.Lock()
		down := obs.stage == health.StageDown
		obs.mu.Unlock()

		if down {
			s.deps.Logger.Info("closing a resource watch the heartbeat no longer believes in",
				slog.String("machine", string(id)))
			_ = watch.Close()
			return
		}
	}
}

// settle drains the events that arrive during WatchSettle, so that one
// operator action produces one read rather than six.
func (s *Service) settle(ctx context.Context, events <-chan talos.WatchEvent) {
	timer := time.NewTimer(WatchSettle)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-events:
			if !ok {
				// The watch ended mid-settle. The refresh still runs: the
				// change that started this is real whether or not the
				// subscription survived long enough to say more about it.
				return
			}
		case <-timer.C:
			return
		}
	}
}

// watchReason turns a failed subscription into a sentence.
//
// It borrows unreachableReason for the transport failures, because a watch that
// could not be established against a dead node and a read that could not be
// made against the same node are the same fact and should read the same way on
// the screen.
func watchReason(err error) string {
	if _, ok := talos.ErrorKindOf(err); ok {
		return unreachableReason(err)
	}
	return "the resource watch failed"
}

// setWatch records the subscription's state, counting a restart every time a
// watch that had been live stops being live.
func (s *Service) setWatch(id model.MachineID, live bool, reason string) {
	obs := s.observationFor(id)

	obs.mu.Lock()
	defer obs.mu.Unlock()

	if obs.watch.live && !live {
		obs.watch.restarts++
	}
	if obs.watch.live == live && obs.watch.reason == reason {
		// Nothing changed, so since keeps its meaning: how long this state has
		// held, rather than when it was last written.
		return
	}

	obs.watch = watchState{
		live:     live,
		since:    s.deps.Now().UTC(),
		reason:   reason,
		restarts: obs.watch.restarts,
	}
}

// watchStatus is the read model's view of one machine's subscription.
func (s *Service) watchStatus(id model.MachineID) WatchStatus {
	obs := s.observationFor(id)

	obs.mu.Lock()
	defer obs.mu.Unlock()

	return WatchStatus{
		Live:     obs.watch.live,
		Since:    obs.watch.since,
		Reason:   obs.watch.reason,
		Restarts: obs.watch.restarts,
	}
}

// stageOf reads one machine's stage.
func (s *Service) stageOf(id model.MachineID) health.Stage {
	obs := s.observationFor(id)

	obs.mu.Lock()
	defer obs.mu.Unlock()

	return obs.stage
}
