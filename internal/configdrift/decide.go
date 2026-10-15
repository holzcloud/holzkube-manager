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
//  3. same digest: ok. A deviation that was running is over, and an expectation
//     of an apply that is the baseline itself or has run out is dropped;
//  4. the digest an own apply left behind (Expected), while that expectation
//     still counts: that configuration is the baseline (source apply) and the
//     node is ok -- unless a deviation is open and was never confirmed, in
//     which case the baseline stays where it is, the expectation is spent and
//     the node stays drift (D-13);
//  5. different digest, but the stored text means the same once the running
//     machinery has normalised it: ok, and the text is brought to that form once;
//  6. an own apply is on its way (Expected, not staged, younger than the settle
//     window, whatever its mode) and no deviation is open: settling, nothing is
//     written;
//  7. otherwise drift, with the start of it written once.
//
// An expectation counts for the adoption (rule 4) while it is a staged one (a
// staged configuration changes nothing until the next boot and has no end of its
// own), an apply with a restart (reboot, auto) younger than AdoptWindow, or any
// other apply younger than the settle window. Settling (rule 6) is shorter and
// the same for every mode but staged: the settle window. It is a digest, so only
// that exact configuration is ever taken for an own change. The price of the
// long window for a restart is that somebody else applying exactly the same
// bytes inside it cannot be told from the landing (named since 15-05).
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
			sameAsBaseline := renormalised.Digest == snap.Digest

			// An apply from this installation left this digest behind, and this
			// is it.
			if exp := base.Expected; exp != nil && exp.Digest == snap.Digest && !sameAsBaseline && s.adopts(exp, now) {
				if !base.DriftSince.IsZero() {
					// D-13: a deviation is open and nobody confirmed it, and
					// this landed on top of it. Moving the baseline would make
					// the earlier difference disappear unseen, so it stays: the
					// start, the baseline and the state are what they were, the
					// expectation is spent, and the record says an apply landed
					// on it.
					if base.AppliedOnDriftAt.IsZero() {
						base.AppliedOnDriftAt = now
					}
					base.Expected = nil
					saved, err := s.deps.Baselines.Put(ctx, base)
					if errors.Is(err, store.ErrConflict) {
						continue
					}
					if err != nil {
						s.unreadable(ctx, cluster, m, reasonStoreFailed(err))
						return false
					}
					s.record(cluster, m, StateDrift, now, snap, saved.DriftSince, sentenceDrift(saved, saved.DriftSince))
					return true
				}

				// What the operator meant is now what the node runs: it is the
				// baseline, and there is no difference to report.
				landed := base
				landed.Format, landed.Cluster = model.BaselineFormat, m.Cluster
				landed.Snapshot, landed.Digest = snap.Text, snap.Digest
				landed.TakenAt, landed.Source, landed.Machinery = now, model.BaselineApply, gendata.VersionTag
				landed.Expected, landed.DriftSince, landed.AppliedOnDriftAt = nil, time.Time{}, time.Time{}
				saved, err := s.deps.Baselines.Put(ctx, landed)
				if errors.Is(err, store.ErrConflict) {
					continue
				}
				if err != nil {
					s.unreadable(ctx, cluster, m, reasonStoreFailed(err))
					return false
				}
				s.record(cluster, m, StateOK, now, snap, time.Time{}, sentenceOK(saved))
				return true
			}

			if sameAsBaseline {
				base.Snapshot, base.Digest, base.Machinery = renormalised.Text, renormalised.Digest, gendata.VersionTag
				base.DriftSince, base.AppliedOnDriftAt = time.Time{}, time.Time{}
				if exp := base.Expected; exp != nil && (exp.Digest == snap.Digest || s.stale(exp, now)) {
					base.Expected = nil
				}
				if _, err := s.deps.Baselines.Put(ctx, base); errors.Is(err, store.ErrConflict) {
					continue
				} else if err != nil {
					s.unreadable(ctx, cluster, m, reasonStoreFailed(err))
					return false
				}
				s.record(cluster, m, StateOK, now, snap, time.Time{}, sentenceOK(base))
				return true
			}

			// An apply is on its way and the node is not what it was: for a
			// while that is the apply, not somebody else. It is never said of a
			// deviation that is already open, which is somebody else's by now.
			if exp := base.Expected; exp != nil && base.DriftSince.IsZero() && s.settles(exp, now) {
				s.record(cluster, m, StateSettling, now, snap, time.Time{}, sentenceSettling(exp, s.deps.SettleWindow))
				return true
			}

			// Drift. An expectation that has run out goes in the same write.
			spent := base.Expected != nil && s.stale(base.Expected, now)
			if base.DriftSince.IsZero() || spent {
				if base.DriftSince.IsZero() {
					base.DriftSince = now
				}
				if spent {
					base.Expected = nil
				}
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

		// The node is the baseline. A deviation that was running is over, and
		// an expectation that is this very configuration (an apply that changed
		// nothing) or has run out is of no use any more. One that is still
		// waiting -- a reboot not done yet, a staged configuration -- stays.
		spent := base.Expected != nil && (base.Expected.Digest == base.Digest || s.stale(base.Expected, now))
		if !base.DriftSince.IsZero() || !base.AppliedOnDriftAt.IsZero() || spent {
			base.DriftSince, base.AppliedOnDriftAt = time.Time{}, time.Time{}
			if spent {
				base.Expected = nil
			}
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

// settles reports whether an expectation still explains a difference by the
// apply being on its way: a mode that changes the running configuration, not
// yet past the settle window. A staged configuration does not: it changes
// nothing until the next boot, so a difference while it waits is not it.
//
// Settling is the one thing that stays at the settle window for every mode;
// whether the configuration that was sent is taken for the landing is adopts.
func (s *Service) settles(e *model.ExpectedConfig, now time.Time) bool {
	return e.Mode != modeStaged && now.Sub(e.At) < s.deps.SettleWindow
}

// adopts reports whether an expectation still counts for the adoption: whether a
// reading of exactly the configuration it names is taken for the landing of the
// apply. A staged one counts without end (it lands at the next boot, whenever
// that is). An apply with a restart (reboot, auto) counts for AdoptWindow,
// because the first reading after the restart can be a whole pass away. Any
// other mode (no-reboot) counts for the settle window: it takes effect without
// a restart and the node is read right after. The edge is strict: at the end of
// the window it no longer counts.
func (s *Service) adopts(e *model.ExpectedConfig, now time.Time) bool {
	age := now.Sub(e.At)
	switch e.Mode {
	case modeStaged:
		return true
	case modeReboot, modeAuto:
		return age < AdoptWindow
	default:
		return age < s.deps.SettleWindow
	}
}

// stale reports whether an expectation has run out: it no longer counts for the
// adoption. Nothing is explained by it any more, and what it names is not taken
// for an own change when it shows up.
func (s *Service) stale(e *model.ExpectedConfig, now time.Time) bool {
	return !s.adopts(e, now)
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
	changed := s.put(m, r)
	s.mu.Unlock()

	if changed {
		s.deps.Log("configuration drift: state of a node changed",
			"cluster", string(cluster), "node", m.Hostname, "machine", string(m.ID), "state", string(r.state))
	}
}

// put stores a reading and reports whether the state is another one than the
// last that was logged. The caller holds mu.
func (s *Service) put(m model.Machine, r *reading) bool {
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
	return changed
}
