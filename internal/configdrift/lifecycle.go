package configdrift

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/siderolabs/talos/pkg/machinery/gendata"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
)

// What Confirm refuses, in words the operator can read: which situation it is
// and what to do about it. The HTTP layer maps each to its own status.
var (
	// ErrNotRead means there is no current good reading of the node to confirm:
	// it was not read since the daemon started, or the last attempt failed.
	ErrNotRead = errors.New("configdrift: this node has no current reading to confirm: it has not been read " +
		"since holzkube-manager started, or the last attempt failed; wait for a reading that works and look at the difference again")

	// ErrNoDrift means the node does not differ from its baseline, so there is
	// nothing to confirm.
	ErrNoDrift = errors.New("configdrift: this node does not differ from its baseline, so there is nothing to confirm")

	// ErrDigestChanged means what the operator looked at is not the state last
	// read: the configuration changed again, or the digest is not one this node
	// has. The digest is the compare-and-swap of confirming (D-11).
	ErrDigestChanged = errors.New("configdrift: the configuration is not the one you looked at; " +
		"it changed again or the digest is not this node's, so look at the difference again before confirming")
)

// findMachine is the inventory record of one machine, or ErrUnknownMachine.
func (s *Service) findMachine(ctx context.Context, id model.MachineID) (model.Machine, error) {
	machines, err := s.deps.AllMachines(ctx)
	if err != nil {
		return model.Machine{}, fmt.Errorf("configdrift: list the machines: %w", err)
	}
	for _, m := range machines {
		if m.ID == id {
			return m, nil
		}
	}
	return model.Machine{}, ErrUnknownMachine
}

// Confirm makes the state the operator saw the new baseline: "so ist es
// gewollt" (D-07). Nothing is done to the node -- not read, not written -- and
// nothing is reset; the daemon confirms the last reading it holds in memory.
//
// It confirms only what the operator saw. digest is the digest of the reading
// shown to them, and a different one is ErrDigestChanged: a third change
// between the screen and the click is not confirmed with it.
func (s *Service) Confirm(ctx context.Context, id model.MachineID, digest string) error {
	m, err := s.findMachine(ctx, id)
	if err != nil {
		return err
	}

	// The first look is outside the lock: most refusals need no write and no
	// wait for one.
	if _, err := s.confirmable(id, digest); err != nil {
		return err
	}

	s.storeMu.Lock()
	defer s.storeMu.Unlock()

	for range decideAttempts {
		// The second look is inside it. Two confirmations of the same state both
		// pass the first; the one that got the lock second finds the node ok.
		rd, err := s.confirmable(id, digest)
		if err != nil {
			return err
		}
		rec := model.ConfigBaseline{
			Machine:   id,
			Format:    model.BaselineFormat,
			Cluster:   m.Cluster,
			Snapshot:  rd.snap.Text,
			Digest:    rd.snap.Digest,
			TakenAt:   s.deps.Now(),
			Source:    model.BaselineConfirmed,
			Machinery: gendata.VersionTag,
		}
		base, err := s.deps.Baselines.Get(ctx, id)
		switch {
		case err == nil:
			// Confirming settles a real drift; it does not repair a baseline
			// that cannot be trusted. That is a file to remove, after which the
			// next reading lays a new one.
			if why := damageOf(base, id); why != "" {
				return fmt.Errorf("configdrift: the stored baseline (config-baselines/%s.json) is damaged: %s; "+
					"it is left as it is; remove the file and the next reading lays a new baseline", id, why)
			}
			rec.Rev = base.Rev
		case errors.Is(err, store.ErrNotFound):
			// The record went away since the reading. What the operator
			// confirmed is still what they saw, so it becomes the baseline.
			rec.Rev = 0
		default:
			return fmt.Errorf("configdrift: the baseline of %s cannot be loaded and is left as it is: %w", id, err)
		}

		saved, err := s.deps.Baselines.Put(ctx, rec)
		if errors.Is(err, store.ErrConflict) {
			continue
		}
		if err != nil {
			return fmt.Errorf("configdrift: store the confirmed baseline: %w", err)
		}
		s.markConfirmed(m, rd.cluster, sentenceOK(saved))
		return nil
	}
	return fmt.Errorf("configdrift: the baseline of %s was being changed at the same time: %w", id, store.ErrConflict)
}

// confirmable is the reading Confirm would confirm, or the reason it cannot:
// ErrNotRead, ErrNoDrift, ErrDigestChanged, in this order.
func (s *Service) confirmable(id model.MachineID, digest string) (reading, error) {
	s.mu.Lock()
	r, ok := s.readings[id]
	var rd reading
	if ok {
		rd = *r
	}
	s.mu.Unlock()

	switch {
	case !ok || rd.state == StateNotRead || rd.state == StateUnreadable:
		return reading{}, ErrNotRead
	case rd.state != StateDrift:
		return reading{}, ErrNoDrift
	case !isDigest(digest) || digest != rd.snap.Digest:
		// The format is checked as well as the equality: the HTTP layer
		// validates the format on its own and with another error, and this does
		// not lean on that. An empty digest never confirms anything, not even a
		// reading that has none.
		return reading{}, ErrDigestChanged
	}
	return rd, nil
}

// isDigest says whether text is a sha256 as lower-case hex.
func isDigest(text string) bool {
	if len(text) != 64 {
		return false
	}
	for _, c := range text {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// markConfirmed puts the reading in memory on ok, so that the display does not
// keep showing the difference that was just made the baseline.
//
// It changes what is there under one hold of mu: a reading that failed since
// the check keeps its state (the node is not reachable, whatever was confirmed),
// only the start of the deviation goes.
func (s *Service) markConfirmed(m model.Machine, cluster model.ClusterID, sentence string) {
	s.mu.Lock()
	next := reading{cluster: cluster, node: m.Hostname}
	if cur, ok := s.readings[m.ID]; ok {
		next = *cur
	}
	if next.state == StateDrift {
		next.state = StateOK
		next.sentence = sentence
	}
	next.since = time.Time{}
	changed := s.put(m, &next)
	s.mu.Unlock()

	if changed {
		s.deps.Log("configuration drift: state of a node changed",
			"cluster", string(cluster), "node", m.Hostname, "machine", string(m.ID), "state", string(next.state))
	}
}

// Forget drops what is held about one machine: the baseline file and the
// reading in memory. Whatever of the two is not there is not an error. The next
// reading lays a new baseline out of what the node says then.
//
// It is also the way out of a baseline that is damaged, which nothing else
// repairs.
func (s *Service) Forget(ctx context.Context, id model.MachineID) {
	s.storeMu.Lock()
	err := s.deps.Baselines.Delete(ctx, id)
	s.storeMu.Unlock()
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.deps.Log("configuration drift: forget a baseline", "machine", string(id), "err", err)
	}

	s.mu.Lock()
	delete(s.readings, id)
	delete(s.logged, id)
	s.mu.Unlock()
}

// reconcile clears up after machines that are gone and machines that moved, at
// the start of every pass: a baseline whose machine is not in the inventory any
// more, or whose cluster is not the machine's cluster now, is removed.
//
// It runs in the pass and not as a hook in ForgetMachine and ForgetCluster
// because those two do not always delete a machine (a forgotten cluster only
// releases its machines), and a pass also covers a machine that was moved by
// hand and a data directory restored from a backup, with one rule.
//
// It deletes only when it knows both lists. "The machines could not be listed"
// is not "there are none", and "the baselines could not be listed" -- which is
// what one damaged file does to the store's List, with the part read until then
// still in hand -- is not a list to delete by. In either case nothing goes and
// a line says why; the damaged file stops the clearing up until it is removed.
func (s *Service) reconcile(ctx context.Context) {
	machines, err := s.deps.AllMachines(ctx)
	if err != nil {
		s.deps.Log("configuration drift: the machines could not be listed, nothing is cleared up", "err", err)
		return
	}

	s.storeMu.Lock()
	defer s.storeMu.Unlock()

	bases, err := s.deps.Baselines.List(ctx)
	if err != nil {
		s.deps.Log("configuration drift: the baselines could not be listed, nothing is cleared up", "err", err)
		return
	}

	cluster := make(map[model.MachineID]model.ClusterID, len(machines))
	for _, m := range machines {
		cluster[m.ID] = m.Cluster
	}
	for _, b := range bases {
		if c, known := cluster[b.Machine]; known && c == b.Cluster {
			continue
		}
		if err := s.deps.Baselines.Delete(ctx, b.Machine); err != nil && !errors.Is(err, store.ErrNotFound) {
			s.deps.Log("configuration drift: remove the baseline of a machine that is gone or moved",
				"machine", string(b.Machine), "err", err)
			continue
		}
		s.deps.Log("configuration drift: removed the baseline of a machine that is gone or moved",
			"machine", string(b.Machine))
		s.mu.Lock()
		delete(s.readings, b.Machine)
		delete(s.logged, b.Machine)
		s.mu.Unlock()
	}
}

// Poke asks for one node to be read before the next pass, for example because a
// configuration was just applied to it. It never blocks: when the buffer is
// full the poke is dropped, because the next pass reads the node anyway.
func (s *Service) Poke(id model.MachineID) {
	select {
	case s.pokes <- id:
	default:
	}
}

// Flush reads the nodes that were poked, now, each once however often it was
// poked, under the budget every node gets. Run calls it; a test may.
func (s *Service) Flush(ctx context.Context) {
	s.flush(ctx, nil)
}

// flush reads first and everything else that is waiting in the buffer.
func (s *Service) flush(ctx context.Context, first []model.MachineID) {
	ids := first
	for {
		select {
		case id := <-s.pokes:
			ids = append(ids, id)
			continue
		default:
		}
		break
	}
	if len(ids) == 0 {
		return
	}

	machines, err := s.deps.AllMachines(ctx)
	if err != nil {
		s.deps.Log("configuration drift: the machines could not be listed, a poke is dropped", "err", err)
		return
	}
	clusters, err := s.deps.Clusters(ctx)
	if err != nil {
		s.deps.Log("configuration drift: the clusters could not be listed, a poke is dropped", "err", err)
		return
	}
	byID := make(map[model.MachineID]model.Machine, len(machines))
	for _, m := range machines {
		byID[m.ID] = m
	}
	off := make(map[model.ClusterID]bool, len(clusters))
	for _, c := range clusters {
		off[c.ID] = c.Disabled
	}

	done := make(map[model.MachineID]bool, len(ids))
	for _, id := range ids {
		if done[id] {
			continue
		}
		done[id] = true
		m, known := byID[id]
		disabled, inCluster := off[m.Cluster]
		if !known || m.Cluster == "" || m.Disabled || !inCluster || disabled {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		s.readNode(ctx, m.Cluster, m)
	}
}
