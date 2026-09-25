package inventory

import (
	"context"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

// LiveReadWait is how long a view waits for its live read before it answers
// with what it has.
//
// The operator's ruling is that a screen shows the cluster as it is when the
// screen is opened, not as it was at the last heartbeat. So opening one asks
// the nodes -- and the membership -- again. It does not wait for ever: a node
// that does not answer within this bound is shown with its last known facts
// and their age, which is what INV-08 asks of a node that is down, and the
// read it was waiting on carries on in the background and lands on the next
// load.
const LiveReadWait = 3 * time.Second

// ReadLive re-reads from the cluster what a view is about to show.
//
// An empty cluster means every cluster; an empty machine means every machine
// of the cluster. Naming a machine skips the membership pass: a view of one
// node is a question about that node.
//
// Reads already in flight for the same target are joined rather than
// duplicated, so a page polled by several tabs asks each node once.
func (s *Service) ReadLive(ctx context.Context, cluster model.ClusterID, machine model.MachineID) {
	var waits []<-chan struct{}

	if machine != "" {
		if ch := s.inFlight("machine/"+string(machine), func(ctx context.Context) {
			s.Refresh(ctx, machine)
		}); ch != nil {
			waits = append(waits, ch)
		}
	} else {
		clusters, err := s.deps.Store.Clusters().List(ctx)
		if err != nil {
			return
		}
		for _, c := range clusters {
			if cluster != "" && c.ID != cluster {
				continue
			}
			if ch := s.inFlight("members/"+string(c.ID), func(ctx context.Context) {
				s.syncCluster(ctx, c)
			}); ch != nil {
				waits = append(waits, ch)
			}
		}

		machines, err := s.presentMachines(ctx)
		if err != nil {
			return
		}
		for _, m := range machines {
			if m.Cluster == "" || (cluster != "" && m.Cluster != cluster) {
				continue
			}
			if ch := s.inFlight("machine/"+string(m.ID), func(ctx context.Context) {
				s.Refresh(ctx, m.ID)
			}); ch != nil {
				waits = append(waits, ch)
			}
		}
	}

	timeout := time.NewTimer(LiveReadWait)
	defer timeout.Stop()
	for _, ch := range waits {
		select {
		case <-ch:
		case <-timeout.C:
			return
		case <-ctx.Done():
			return
		}
	}
}

// inFlight starts read under key unless one is already running, and returns
// the channel that closes when the running one finishes.
//
// The read runs on the service's lifetime, not the request's: a page closed
// mid-read must not cancel it into a failed pass that marks a healthy node
// down. Before Start there is no lifetime, and nothing is started.
func (s *Service) inFlight(key string, read func(context.Context)) <-chan struct{} {
	s.mu.Lock()
	if ch, ok := s.reads[key]; ok {
		s.mu.Unlock()
		return ch
	}
	if s.closed || s.runCtx == nil {
		s.mu.Unlock()
		return nil
	}
	ch := make(chan struct{})
	s.reads[key] = ch
	ctx := s.runCtx
	s.wg.Add(1)
	s.mu.Unlock()

	go func() {
		defer s.wg.Done()
		defer func() {
			s.mu.Lock()
			delete(s.reads, key)
			s.mu.Unlock()
			close(ch)
		}()
		read(ctx)
	}()
	return ch
}
