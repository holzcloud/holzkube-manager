package configdrift

import (
	"context"
	"errors"

	"github.com/holzcloud/holzkube-manager/internal/machineconfig"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// modeTry is the apply mode whose configuration returns by itself. It is a
// literal of this package because talos carries the mode as the caller's string.
const modeTry = "try"

// ApplyObserver is what hears every configuration this installation sends to a
// node, hung on the Talos client (talos.ClusterClient.SetApplyObserver) at the
// one place all of them pass (D-10).
//
// Before the RPC it leaves an expected digest in the node's baseline: the
// digest of the snapshot of the bytes about to go, the mode and the time. A
// reading that finds exactly that configuration turns it into the baseline
// (decide), so an own change is never drift. It is a digest and never content:
// the bytes hold the cluster's secrets, and the snapshot made of them is
// dropped as soon as its digest is taken.
//
// What it does not do is decide anything about the node, and it never makes an
// apply fail: whatever goes wrong is a line in the log and, at worst, a false
// alarm the operator confirms. A node without a baseline gets no expectation,
// because the first reading lays the baseline out of what the node says then.
//
// "try" leaves nothing: the configuration returns by itself, and a reading in
// its minute would otherwise be taken for the new baseline of a node that is
// about to go back.
//
// When the RPC is over, an error removes the expectation (a refused or failed
// apply changed nothing, and a third party applying the same bytes later must
// not be explained away by it), and success pokes the node so that it is read
// out of turn (D-04).
func (s *Service) ApplyObserver() talos.ApplyObserver {
	return func(ctx context.Context, a talos.ApplyAttempt) func(error) {
		if a.Mode == modeTry {
			return nil
		}

		snap, err := machineconfig.Snapshot(a.Config)
		if err != nil {
			// The reason is one of machineconfig's fixed sentences and holds no
			// content. The apply goes on; there is no expectation, and the
			// worst that follows is a difference nobody expected.
			s.deps.Log("configuration drift: a configuration about to be applied cannot be snapshotted, "+
				"so no change is expected from it", "machine", string(a.Machine), "err", err)
			return nil
		}

		// The context of the attempt may be cancelled by the time the result is
		// in; tidying up after a failed apply is not allowed to fail for that.
		bg := context.WithoutCancel(ctx)
		exp := model.ExpectedConfig{Digest: snap.Digest, Mode: a.Mode, At: s.deps.Now()}
		expected := s.expect(bg, a.Machine, exp)

		return func(applyErr error) {
			if applyErr != nil {
				if expected {
					s.unexpect(bg, a.Machine, exp)
				}
				return
			}
			s.Poke(a.Machine)
		}
	}
}

// expect writes the expectation of an apply into the node's baseline and
// reports whether it did. A node without a baseline, a baseline that cannot be
// trusted and a store that fails are all "no": the baseline is not invented and
// not repaired here.
func (s *Service) expect(ctx context.Context, id model.MachineID, exp model.ExpectedConfig) bool {
	s.storeMu.Lock()
	defer s.storeMu.Unlock()

	for range decideAttempts {
		base, err := s.deps.Baselines.Get(ctx, id)
		switch {
		case errors.Is(err, store.ErrNotFound):
			return false
		case err != nil:
			s.deps.Log("configuration drift: the baseline cannot be loaded, so no change is expected from an apply",
				"machine", string(id), "err", err)
			return false
		case damageOf(base, id) != "":
			return false
		}

		base.Expected = &exp
		if _, err := s.deps.Baselines.Put(ctx, base); errors.Is(err, store.ErrConflict) {
			continue
		} else if err != nil {
			s.deps.Log("configuration drift: the expectation of an apply cannot be stored",
				"machine", string(id), "err", err)
			return false
		}
		return true
	}
	return false
}

// unexpect removes the expectation of one apply, and only that one: a later
// apply has replaced it by now, and its expectation is not this one's to take.
func (s *Service) unexpect(ctx context.Context, id model.MachineID, exp model.ExpectedConfig) {
	s.storeMu.Lock()
	defer s.storeMu.Unlock()

	for range decideAttempts {
		base, err := s.deps.Baselines.Get(ctx, id)
		if err != nil {
			// Gone, or not readable: there is nothing here this call may touch.
			return
		}
		if base.Expected == nil || base.Expected.Digest != exp.Digest || !base.Expected.At.Equal(exp.At) {
			return
		}
		base.Expected = nil
		if _, err := s.deps.Baselines.Put(ctx, base); errors.Is(err, store.ErrConflict) {
			continue
		} else if err != nil {
			s.deps.Log("configuration drift: the expectation of a failed apply cannot be removed",
				"machine", string(id), "err", err)
		}
		return
	}
}
