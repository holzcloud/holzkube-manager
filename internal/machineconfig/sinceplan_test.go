package machineconfig

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

// moving is a clock the test moves.
type moving struct{ t time.Time }

func (m *moving) now() time.Time { return m.t }

func newMoving() *moving { return &moving{t: time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)} }

func TestPlanMemoDropsTheOldestBeyondItsLimits(t *testing.T) {
	t.Parallel()

	clock := newMoving()
	m := newPlanMemo(2, 1<<20, time.Hour, clock.now)
	for _, d := range []string{"d1", "d2", "d3"} {
		clock.t = clock.t.Add(time.Second)
		m.remember("m1", Snap{Digest: d, Text: "text of " + d})
	}

	if _, ok := m.recall("m1", "d1"); ok {
		t.Error("the oldest of three entries survived a limit of two per node")
	}
	for _, d := range []string{"d2", "d3"} {
		if got, ok := m.recall("m1", d); !ok || got != "text of "+d {
			t.Errorf("recall(%s) = %q, %v, want its text", d, got, ok)
		}
	}

	// The limit is per node: another node's entry is not displaced.
	m.remember("m2", Snap{Digest: "e1", Text: "other node"})
	if _, ok := m.recall("m2", "e1"); !ok {
		t.Error("an entry of another node was displaced by m1's")
	}
	// And an entry belongs to its node: m1's digest is not found under m2.
	if _, ok := m.recall("m2", "d3"); ok {
		t.Error("a digest of m1 was found under m2")
	}
}

func TestPlanMemoForgetsAfterItsLifetime(t *testing.T) {
	t.Parallel()

	clock := newMoving()
	m := newPlanMemo(4, 1<<20, time.Hour, clock.now)
	m.remember("m1", Snap{Digest: "d1", Text: "text"})

	clock.t = clock.t.Add(time.Hour - time.Second)
	if _, ok := m.recall("m1", "d1"); !ok {
		t.Fatal("the entry is gone before its lifetime ended")
	}
	clock.t = clock.t.Add(2 * time.Second)
	if _, ok := m.recall("m1", "d1"); ok {
		t.Error("an entry older than its lifetime was delivered")
	}
}

func TestPlanMemoHoldsAtMostItsTotalBytes(t *testing.T) {
	t.Parallel()

	clock := newMoving()
	m := newPlanMemo(4, 100, time.Hour, clock.now)
	body := strings.Repeat("x", 40)
	for i, id := range []string{"m1", "m2", "m3"} {
		clock.t = clock.t.Add(time.Second)
		m.remember(model.MachineID(id), Snap{Digest: fmt.Sprintf("d%d", i), Text: body})
	}

	if got := m.bytes(); got > 100 {
		t.Errorf("the memo holds %d bytes, want at most 100", got)
	}
	if _, ok := m.recall("m1", "d0"); ok {
		t.Error("the oldest entry survived the total limit")
	}
	if _, ok := m.recall("m3", "d2"); !ok {
		t.Error("the newest entry was dropped for the total limit")
	}
}

func TestTheSentenceBuilderNamesPathsKindsAndLengths(t *testing.T) {
	t.Parallel()

	err := &ChangedSincePlanError{Known: true, Changes: []Change{
		{Path: "KubeletConfig.image", Kind: ChangeModified, Before: "old-value", After: "new-value"},
		{Path: "v1alpha1.machine.certSANs", Kind: ChangeListGrew, LenBefore: 3, LenAfter: 4, After: "[192.168.1.10]"},
		{Path: "v1alpha1.machine.kubelet", Kind: ChangeAdded, After: "added-value"},
		{Path: "v1alpha1.machine.logging", Kind: ChangeRemoved, Before: "removed-value"},
	}}
	got := err.Error()

	if !strings.HasPrefix(got, "Nothing was applied:") {
		t.Errorf("the sentence begins %q, want 'Nothing was applied:'", got)
	}
	for _, want := range []string{
		"KubeletConfig.image (modified)",
		"v1alpha1.machine.certSANs (list changed, 3 entries became 4)",
		"v1alpha1.machine.kubelet (added)",
		"v1alpha1.machine.logging (removed)",
		"Plan again",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the sentence lacks %q:\n%s", want, got)
		}
	}
	for _, leak := range []string{"old-value", "new-value", "192.168.1.10", "added-value", "removed-value", "more"} {
		if strings.Contains(got, leak) {
			t.Errorf("the sentence holds %q, which is a value or a count it must not carry:\n%s", leak, got)
		}
	}
	if !errors.Is(err, ErrChangedSincePlan) {
		t.Error("errors.Is(err, ErrChangedSincePlan) is false")
	}
}

func TestTheSentenceBuilderCapsAtEightAndSaysHowManyMore(t *testing.T) {
	t.Parallel()

	var changes []Change
	for i := 1; i <= 11; i++ {
		changes = append(changes, Change{Path: fmt.Sprintf("Doc.path%02d", i), Kind: ChangeModified})
	}
	got := (&ChangedSincePlanError{Known: true, Changes: changes}).Error()

	for i := 1; i <= 8; i++ {
		if p := fmt.Sprintf("Doc.path%02d", i); !strings.Contains(got, p) {
			t.Errorf("the sentence lacks %s:\n%s", p, got)
		}
	}
	if strings.Contains(got, "Doc.path09") {
		t.Errorf("the ninth path is in the sentence:\n%s", got)
	}
	if !strings.Contains(got, "and 3 more") {
		t.Errorf("the sentence does not say 'and 3 more':\n%s", got)
	}
}

func TestTheSentenceBuilderHasABranchForADifferenceItCannotName(t *testing.T) {
	t.Parallel()

	got := (&ChangedSincePlanError{Known: true}).Error()
	if !strings.HasPrefix(got, "Nothing was applied:") || !strings.Contains(got, "Plan again") {
		t.Errorf("the sentence for an empty difference is %q", got)
	}
	if !strings.Contains(got, "no setting") {
		t.Errorf("the sentence does not say that no setting differs:\n%s", got)
	}
	if strings.Contains(got, "Changed since the plan:") {
		t.Errorf("the sentence announces a list of changes and names none:\n%s", got)
	}
}

func TestTheSentenceBuilderHasABranchForAnUnknownEarlierState(t *testing.T) {
	t.Parallel()

	got := (&ChangedSincePlanError{Known: false}).Error()
	if !strings.HasPrefix(got, "Nothing was applied:") || !strings.Contains(got, "Plan again") {
		t.Errorf("the sentence for an unknown state is %q", got)
	}
	if !strings.Contains(got, "no longer held") {
		t.Errorf("the sentence does not say the earlier state is no longer held:\n%s", got)
	}
}

// TestCheckBaseNeverReturnsNilForASnapshotFailure: a check that cannot be done
// is not a check that passed.
func TestCheckBaseNeverReturnsNilForASnapshotFailure(t *testing.T) {
	t.Parallel()

	s := New(Deps{})
	huge := []byte(strings.Repeat("# padding\n", MaxSnapshotBytes/10+10))
	if len(huge) <= MaxSnapshotBytes {
		t.Fatalf("precondition: the configuration is %d bytes, not over the limit", len(huge))
	}
	err := s.checkBase("m1", huge, strings.Repeat("0", 64))
	if err == nil {
		t.Fatal("checkBase returned nil for a configuration it could not snapshot")
	}
	if !errors.Is(err, ErrSnapshotTooLarge) {
		t.Errorf("checkBase returned %v, want ErrSnapshotTooLarge", err)
	}
}

// acquire takes the lock of a node on a goroutine and reports when it got it.
func acquire(ctx context.Context, s *Service, id model.MachineID) <-chan func() {
	got := make(chan func(), 1)
	go func() {
		release, err := s.lockMachine(ctx, id)
		if err != nil {
			got <- nil
			return
		}
		got <- release
	}()
	return got
}

func TestLockMachineWaitsOnTheSameNodeAndNotOnAnother(t *testing.T) {
	t.Parallel()

	s := New(Deps{})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	release, err := s.lockMachine(ctx, "m1")
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}

	// Another node does not wait.
	other, err := s.lockMachine(ctx, "m2")
	if err != nil {
		t.Fatalf("lock of another node: %v", err)
	}
	other()

	// The same node does, until the first is released.
	second := acquire(ctx, s, "m1")
	select {
	case <-second:
		t.Fatal("a second lock of the same node was granted while the first was held")
	case <-time.After(150 * time.Millisecond):
	}
	release()
	select {
	case got := <-second:
		if got == nil {
			t.Fatal("the second lock failed after the first was released")
		}
		got()
	case <-time.After(5 * time.Second):
		t.Fatal("the second lock was not granted after the first was released")
	}
}

func TestLockMachineGivesUpWithTheContext(t *testing.T) {
	t.Parallel()

	s := New(Deps{})
	long, cancelLong := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelLong()
	release, err := s.lockMachine(long, "m1")
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}
	defer release()

	short, cancelShort := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancelShort()
	done := make(chan error, 1)
	go func() {
		_, err := s.lockMachine(short, "m1")
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, ErrApplyBusy) {
			t.Errorf("the lock returned %v, want ErrApplyBusy", err)
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("the lock returned %v, want it to wrap the context's error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the lock did not give up when its context ended")
	}
}

func TestLockMachineReleasesOnEveryPath(t *testing.T) {
	t.Parallel()

	s := New(Deps{})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for i := 0; i < 3; i++ {
		release, err := s.lockMachine(ctx, "m1")
		if err != nil {
			t.Fatalf("lock %d: %v", i, err)
		}
		release()
		// A second release must not free somebody else's hold.
		release()
	}

	// The slot is free after a release, and a double release did not make room
	// for two holders.
	first, err := s.lockMachine(ctx, "m1")
	if err != nil {
		t.Fatalf("lock after releases: %v", err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		short, c := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer c()
		if _, err := s.lockMachine(short, "m1"); !errors.Is(err, ErrApplyBusy) {
			t.Errorf("a second holder got the lock: %v", err)
		}
	}()
	wg.Wait()
	first()
}
