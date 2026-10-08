package machineconfig

import (
	"errors"
	"testing"
)

func TestASecondStagedClaimIsRefusedWhileTheFirstIsStillApplying(t *testing.T) {
	t.Parallel()

	s := New(Deps{})
	commit, _, err := s.reserveStaged("m1")
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}

	// The first apply's RPC has not returned yet.
	if _, _, err := s.reserveStaged("m1"); !errors.Is(err, ErrStagedAlreadyPending) {
		t.Fatalf("second claim during the first = %v, want ErrStagedAlreadyPending", err)
	}
	// Another node is unaffected.
	if _, _, err := s.reserveStaged("m2"); err != nil {
		t.Fatalf("claim on another node: %v", err)
	}

	commit("staged")
	if _, _, err := s.reserveStaged("m1"); !errors.Is(err, ErrStagedAlreadyPending) {
		t.Fatalf("claim after commit = %v, want ErrStagedAlreadyPending", err)
	}
}

func TestAFailedStagedApplyLeavesNothingPending(t *testing.T) {
	t.Parallel()

	s := New(Deps{})
	_, abort, err := s.reserveStaged("m1")
	if err != nil {
		t.Fatal(err)
	}
	abort()
	if _, _, err := s.reserveStaged("m1"); err != nil {
		t.Fatalf("claim after an aborted one: %v", err)
	}
}

func TestAnAbortDoesNotWithdrawSomeoneElsesClaim(t *testing.T) {
	t.Parallel()

	s := New(Deps{})
	_, abort, _ := s.reserveStaged("m1")
	s.ClearStaged("m1") // a reboot job finished meanwhile
	commit2, _, err := s.reserveStaged("m1")
	if err != nil {
		t.Fatal(err)
	}
	commit2("x")
	abort() // the first call's late failure
	if _, _, err := s.reserveStaged("m1"); !errors.Is(err, ErrStagedAlreadyPending) {
		t.Fatalf("a stale abort removed a newer claim: %v", err)
	}
}

func TestAClaimStillApplyingIsNotReportedAsWaiting(t *testing.T) {
	t.Parallel()

	s := New(Deps{})
	commit, _, _ := s.reserveStaged("m1")
	s.mu.Lock()
	rec := s.staged["m1"]
	s.mu.Unlock()
	if !rec.applying {
		t.Fatal("a claim in flight is not marked applying")
	}
	commit("done")
	s.mu.Lock()
	rec = s.staged["m1"]
	s.mu.Unlock()
	if rec.applying || rec.Digest != "done" {
		t.Fatalf("commit left %+v", rec)
	}
}
