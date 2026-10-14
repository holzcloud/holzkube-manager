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

	rd, err := s.confirmable(id, digest)
	if err != nil {
		return err
	}

	s.storeMu.Lock()
	defer s.storeMu.Unlock()

	for range decideAttempts {
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
	case digest != rd.snap.Digest:
		return reading{}, ErrDigestChanged
	}
	return rd, nil
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
