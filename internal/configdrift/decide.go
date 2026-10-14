package configdrift

import (
	"context"
	"errors"
	"time"

	"github.com/siderolabs/talos/pkg/machinery/gendata"

	"github.com/holzcloud/holzkube-manager/internal/machineconfig"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
)

// decideAttempts is how often one reading is decided again after the store
// refused a write because somebody else wrote the baseline in between.
const decideAttempts = 3

// decide is the table of what one successful reading means, in this order:
//
//  1. no baseline yet: this reading becomes it (first sight);
//  2. same digest as the baseline: ok;
//  3. otherwise: drift, with the start of it written once.
//
// It runs under storeMu. It reports whether the node ended up compared.
func (s *Service) decide(ctx context.Context, cluster model.ClusterID, m model.Machine, snap machineconfig.Snap) bool {
	s.storeMu.Lock()
	defer s.storeMu.Unlock()

	now := s.deps.Now()
	for range decideAttempts {
		base, err := s.deps.Baselines.Get(ctx, m.ID)
		if errors.Is(err, store.ErrNotFound) {
			created, err := s.deps.Baselines.Put(ctx, model.ConfigBaseline{
				Machine:   m.ID,
				Format:    model.BaselineFormat,
				Cluster:   m.Cluster,
				Snapshot:  snap.Text,
				Digest:    snap.Digest,
				TakenAt:   now,
				Source:    model.BaselineFirstSight,
				Machinery: gendata.VersionTag,
			})
			if errors.Is(err, store.ErrConflict) {
				continue
			}
			if err != nil {
				s.unreadable(ctx, cluster, m, err)
				return false
			}
			s.record(cluster, m, StateBaselineCreated, now, snap, created.DriftSince, sentenceBaselineCreated(created))
			return true
		}
		if err != nil {
			s.unreadable(ctx, cluster, m, err)
			return false
		}

		if base.Digest == snap.Digest {
			if !base.DriftSince.IsZero() || !base.AppliedOnDriftAt.IsZero() {
				// The node is back at the baseline: there is no difference left
				// to date or to qualify.
				base.DriftSince, base.AppliedOnDriftAt = time.Time{}, time.Time{}
				if _, err := s.deps.Baselines.Put(ctx, base); errors.Is(err, store.ErrConflict) {
					continue
				} else if err != nil {
					s.unreadable(ctx, cluster, m, err)
					return false
				}
			}
			s.record(cluster, m, StateOK, now, snap, time.Time{}, sentenceOK(base))
			return true
		}

		if base.DriftSince.IsZero() {
			base.DriftSince = now
			if _, err := s.deps.Baselines.Put(ctx, base); errors.Is(err, store.ErrConflict) {
				continue
			} else if err != nil {
				s.unreadable(ctx, cluster, m, err)
				return false
			}
		}
		s.record(cluster, m, StateDrift, now, snap, base.DriftSince, sentenceDrift(base, base.DriftSince))
		return true
	}
	s.unreadable(ctx, cluster, m, store.ErrConflict)
	return false
}

// record puts a compared reading into memory.
func (s *Service) record(cluster model.ClusterID, m model.Machine, st State, now time.Time, snap machineconfig.Snap, since time.Time, sentence string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.readings[m.ID] = &reading{
		state: st, at: now, lastGood: now, snap: snap, since: since,
		sentence: sentence, cluster: cluster, node: m.Hostname,
	}
}

// unreadable records a reading that failed. The last good reading stays.
func (s *Service) unreadable(_ context.Context, cluster model.ClusterID, m model.Machine, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := reading{cluster: cluster, node: m.Hostname}
	if prev, ok := s.readings[m.ID]; ok {
		next = *prev
	}
	next.state = StateUnreadable
	next.at = s.deps.Now()
	next.sentence = sentenceUnreadable(err, next.lastGood)
	s.readings[m.ID] = &next
}
