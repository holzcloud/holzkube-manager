package configdrift

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
//  0. a snapshot that must not be stored (a private key, over the cap): the
//     node is unreadable and nothing is written;
//  1. no baseline yet (and only "not found" means that): this reading becomes
//     it, first sight;
//  2. a baseline that cannot be trusted: baseline-damaged, nothing is written;
//  3. same digest: ok -- and if the digests differ but the stored text means
//     the same once the running machinery has normalised it, that is also ok,
//     and the text is brought to that form once;
//  4. otherwise drift, with the start of it written once.
//
// It runs under storeMu and reports whether the node ended up compared.
func (s *Service) decide(ctx context.Context, cluster model.ClusterID, m model.Machine, snap machineconfig.Snap) bool {
	s.storeMu.Lock()
	defer s.storeMu.Unlock()

	now := s.deps.Now()

	// The belt to Redact's braces, and the cap: nothing that is not safe goes
	// near the store, so a baseline that exists is not touched either.
	if len(snap.Text) > machineconfig.MaxSnapshotBytes {
		s.unreadable(ctx, cluster, m, reasonUnreadable(machineconfig.ErrSnapshotTooLarge))
		return false
	}
	if machineconfig.ContainsPrivateKey([]byte(snap.Text)) {
		s.unreadable(ctx, cluster, m, reasonUnreadable(machineconfig.ErrSnapshotUnsafe))
		return false
	}

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
				s.unreadable(ctx, cluster, m, reasonStoreFailed(err))
				return false
			}
			s.record(cluster, m, StateBaselineCreated, now, snap, time.Time{}, sentenceBaselineCreated(created))
			return true
		}
		if err != nil {
			// Anything but "not found" is not a first sight: the file is there
			// and cannot be read, and writing over it would destroy it.
			s.damaged(cluster, m, now, snap, "it could not be read or decoded")
			return false
		}
		if why := damageOf(base, m.ID); why != "" {
			s.damaged(cluster, m, now, snap, why)
			return false
		}

		if base.Digest != snap.Digest {
			// The stored text may be the same configuration in another
			// encoding, written by an older build or older machinery. Judging
			// "drift" on bytes alone would report a difference nobody made.
			renormalised, err := machineconfig.Snapshot([]byte(base.Snapshot))
			if err != nil {
				s.damaged(cluster, m, now, snap, "its snapshot does not load with this version of holzkube-manager")
				return false
			}
			if renormalised.Digest == snap.Digest {
				base.Snapshot, base.Digest, base.Machinery = renormalised.Text, renormalised.Digest, gendata.VersionTag
				base.DriftSince, base.AppliedOnDriftAt = time.Time{}, time.Time{}
				if _, err := s.deps.Baselines.Put(ctx, base); errors.Is(err, store.ErrConflict) {
					continue
				} else if err != nil {
					s.unreadable(ctx, cluster, m, reasonStoreFailed(err))
					return false
				}
				s.record(cluster, m, StateOK, now, snap, time.Time{}, sentenceOK(base))
				return true
			}

			if base.DriftSince.IsZero() {
				base.DriftSince = now
				if _, err := s.deps.Baselines.Put(ctx, base); errors.Is(err, store.ErrConflict) {
					continue
				} else if err != nil {
					s.unreadable(ctx, cluster, m, reasonStoreFailed(err))
					return false
				}
			}
			s.record(cluster, m, StateDrift, now, snap, base.DriftSince, sentenceDrift(base, base.DriftSince))
			return true
		}

		if !base.DriftSince.IsZero() || !base.AppliedOnDriftAt.IsZero() {
			// The node is back at the baseline: there is no difference left to
			// date or to qualify.
			base.DriftSince, base.AppliedOnDriftAt = time.Time{}, time.Time{}
			if _, err := s.deps.Baselines.Put(ctx, base); errors.Is(err, store.ErrConflict) {
				continue
			} else if err != nil {
				s.unreadable(ctx, cluster, m, reasonStoreFailed(err))
				return false
			}
		}
		s.record(cluster, m, StateOK, now, snap, time.Time{}, sentenceOK(base))
		return true
	}
	s.unreadable(ctx, cluster, m, reasonUnreadable(store.ErrConflict))
	return false
}

// damageOf says what is wrong with a stored baseline, or "" when nothing is.
func damageOf(b model.ConfigBaseline, id model.MachineID) string {
	switch {
	case b.Format != model.BaselineFormat:
		return "its format is not one this version of holzkube-manager knows"
	case b.Snapshot == "":
		return "it holds no snapshot"
	case b.Digest != digestOf(b.Snapshot):
		return "its digest does not match its snapshot"
	case b.Machine != id:
		return "it belongs to another machine"
	}
	return ""
}

func digestOf(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// record puts a compared reading into memory.
func (s *Service) record(cluster model.ClusterID, m model.Machine, st State, now time.Time, snap machineconfig.Snap, since time.Time, sentence string) {
	s.set(cluster, m, &reading{
		state: st, at: now, lastGood: now, snap: snap, since: since,
		sentence: sentence, cluster: cluster, node: m.Hostname,
	})
}

// damaged records a reading that worked but could not be compared because the
// stored baseline cannot be trusted. The file is not touched.
func (s *Service) damaged(cluster model.ClusterID, m model.Machine, now time.Time, snap machineconfig.Snap, why string) {
	s.record(cluster, m, StateBaselineDamaged, now, snap, time.Time{}, sentenceDamaged(m.ID, why))
}

// unreadable records a reading that failed. The last good reading stays, and so
// does the start of a deviation: it is read from the record, which is where it
// lives, so a process that has never read the node still knows it. A failed
// reading is "unknown", never "fine".
func (s *Service) unreadable(ctx context.Context, cluster model.ClusterID, m model.Machine, reason string) {
	var since time.Time
	haveRecord := false
	if base, err := s.deps.Baselines.Get(ctx, m.ID); err == nil {
		since, haveRecord = base.DriftSince, true
	}

	s.mu.Lock()
	next := reading{cluster: cluster, node: m.Hostname}
	if prev, ok := s.readings[m.ID]; ok {
		next = *prev
	}
	if haveRecord {
		next.since = since
	}
	next.state = StateUnreadable
	next.at = s.deps.Now()
	next.sentence = sentenceUnreadable(reason, next.lastGood)
	s.mu.Unlock()

	s.set(cluster, m, &next)
}

// set stores a reading and logs the change of state, once per change.
//
// Only a transition is a line: a node that is off for three hours must not
// write one every pass. "ok" is the resting state, and the baseline being laid
// counts as it, so a quiet node writes nothing at all. The line names the
// machine and the state and never an error text or configuration content.
func (s *Service) set(cluster model.ClusterID, m model.Machine, r *reading) {
	s.mu.Lock()
	s.readings[m.ID] = r
	prev, seen := s.logged[m.ID]
	if !seen {
		prev = StateOK
	}
	changed := r.state != prev
	if r.state == StateBaselineCreated {
		s.logged[m.ID] = StateOK
	} else {
		s.logged[m.ID] = r.state
	}
	s.mu.Unlock()

	if changed {
		s.deps.Log("configuration drift: state of a node changed",
			"cluster", string(cluster), "node", m.Hostname, "machine", string(m.ID), "state", string(r.state))
	}
}
