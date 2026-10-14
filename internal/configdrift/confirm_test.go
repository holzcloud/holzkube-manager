package configdrift_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/configdrift"
	"github.com/holzcloud/holzkube-manager/internal/machineconfig"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// countingReader is the real reader of machineconfig with a count of how often
// the service called it. Confirming must not add to it.
type countingReader struct {
	n    *node
	read atomic.Int64
}

func (c *countingReader) Read(ctx context.Context, id model.MachineID) (machineconfig.Snap, error) {
	c.read.Add(1)
	return c.n.mc.ReadSnapshot(ctx, id)
}

// TestTracerConfirmingTurnsTheReadingIntoTheBaselineAndTouchesNothing goes from
// a real node that somebody else changed, through the real client, the real
// store and Detail, to "so ist es gewollt" (D-07): the state that was read
// becomes the baseline, and the node is neither read nor written while that
// happens. The two counters are the measurement of "no intervention on the node".
func TestTracerConfirmingTurnsTheReadingIntoTheBaselineAndTouchesNothing(t *testing.T) {
	t.Parallel()

	n := newNode(t)
	cr := &countingReader{n: n}
	svc := n.service(t, withRead(cr.Read))
	bg := context.Background()
	if _, ok := bg.Deadline(); ok {
		t.Fatal("precondition: the context must carry no deadline")
	}

	svc.Tick(bg)
	n.thirdPartyChange(t, kubeletPatch)
	svc.Tick(bg)

	d, err := svc.Detail(bg, "m1")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if d.State != configdrift.StateDrift {
		t.Fatalf("precondition: state is %q (%s), want drift", d.State, d.Sentence)
	}
	if !sha256Hex.MatchString(d.Digest) {
		t.Fatalf("precondition: the digest is %q, want 64 hex characters", d.Digest)
	}
	if d.Diff == nil || d.DriftSince.IsZero() {
		t.Fatalf("precondition: drift without a diff or a start: %+v", d)
	}
	before, err := n.st.ConfigBaselines().Get(bg, "m1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if before.DriftSince.IsZero() {
		t.Fatal("precondition: the record carries no DriftSince")
	}

	reads, applies := cr.read.Load(), n.sim.Calls("ApplyConfiguration")
	if reads != 2 || applies != 1 {
		t.Fatalf("precondition: %d reads and %d applies before confirming, want 2 and 1 (two ticks, one third party)", reads, applies)
	}

	if err := svc.Confirm(bg, "m1", d.Digest); err != nil {
		t.Fatalf("Confirm: %v", err)
	}

	// The node was not touched: not read, not written.
	if got := cr.read.Load(); got != reads {
		t.Errorf("Confirm read the node: %d reads before, %d after", reads, got)
	}
	if got := n.sim.Calls("ApplyConfiguration"); got != applies {
		t.Errorf("Confirm wrote to the node: %d applies before, %d after", applies, got)
	}

	// The display no longer shows the old difference.
	after, err := svc.Detail(bg, "m1")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if after.State != configdrift.StateOK {
		t.Errorf("state after confirming is %q (%s), want ok", after.State, after.Sentence)
	}
	if after.Diff != nil {
		t.Errorf("the old difference is still shown: %+v", after.Diff)
	}
	if !after.DriftSince.IsZero() {
		t.Errorf("drift_since is %v after confirming, want empty", after.DriftSince)
	}
	if after.BaselineSource != model.BaselineConfirmed {
		t.Errorf("Detail names baseline source %q, want confirmed", after.BaselineSource)
	}

	// The record is the reading.
	rec, err := n.st.ConfigBaselines().Get(bg, "m1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	switch {
	case rec.Source != model.BaselineConfirmed:
		t.Errorf("source is %q, want confirmed", rec.Source)
	case rec.Digest != d.Digest:
		t.Errorf("the record's digest is %q, want the one the operator saw, %q", rec.Digest, d.Digest)
	case rec.Digest == before.Digest:
		t.Error("the baseline did not move")
	case !rec.DriftSince.IsZero():
		t.Errorf("DriftSince is %v in the record, want empty", rec.DriftSince)
	case !rec.AppliedOnDriftAt.IsZero():
		t.Errorf("AppliedOnDriftAt is %v in the record, want empty", rec.AppliedOnDriftAt)
	case rec.Expected != nil:
		t.Errorf("Expected is %+v in the record, want none", rec.Expected)
	case rec.Rev != before.Rev+1:
		t.Errorf("rev is %d, want %d", rec.Rev, before.Rev+1)
	case rec.Cluster != "c1" || rec.Machine != "m1" || rec.Format != model.BaselineFormat || rec.Snapshot == "":
		t.Errorf("the record is %+v, want cluster c1, machine m1, the format and a snapshot", rec)
	}

	// The node still holds the changed configuration, which is the baseline now.
	onDisk, err := os.ReadFile(n.baselinePath())
	if err != nil {
		t.Fatalf("read the baseline file: %v", err)
	}
	svc.Tick(bg)
	if got := cr.read.Load(); got != reads+1 {
		t.Fatalf("the next tick read the node %d times, want 1", got-reads)
	}
	again, err := svc.Detail(bg, "m1")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if again.State != configdrift.StateOK {
		t.Errorf("state after the next reading is %q (%s), want ok", again.State, again.Sentence)
	}
	afterTick, err := os.ReadFile(n.baselinePath())
	if err != nil {
		t.Fatalf("read the baseline file: %v", err)
	}
	if !bytes.Equal(onDisk, afterTick) {
		t.Errorf("a reading of the confirmed state rewrote the baseline:\n before %s\n after  %s", onDisk, afterTick)
	}
	if got := n.sim.Calls("ApplyConfiguration"); got != applies {
		t.Errorf("a node was written after confirming: %d applies, want %d", got, applies)
	}
}

// kubeletPatch2 is a second, different change to the same document as
// kubeletPatch: a third party who changes the node again.
const kubeletPatch2 = "apiVersion: v1alpha1\nkind: KubeletConfig\nimage: ghcr.io/siderolabs/kubelet:v1.34.3\n"

// snapPatched is the snapshot of the node's configuration with one patch.
func snapPatched(t *testing.T, n *node, patch string) machineconfig.Snap {
	t.Helper()
	patched, err := machineconfig.ApplyPatches(n.cl.ControlPlaneConfig, []string{patch})
	if err != nil {
		t.Fatalf("ApplyPatches: %v", err)
	}
	return snapOf(t, patched)
}

// drifting is a node whose baseline was laid and that a third party has changed
// since; the service has read both. The reader is a fake so that a test can
// change what the node says and have the digests in hand.
type drifting struct {
	n      *node
	fake   *fakeReader
	svc    *configdrift.Service
	before machineconfig.Snap
	after  machineconfig.Snap
}

func newDrifting(t *testing.T, opts ...serviceOpt) *drifting {
	t.Helper()
	n := newNode(t)
	fake := &fakeReader{}
	before, after := twoSnaps(t, n)
	d := &drifting{
		n: n, fake: fake, before: before, after: after,
		svc: n.service(t, append([]serviceOpt{withRead(fake.Read)}, opts...)...),
	}
	ctx := context.Background()
	fake.set(before, nil)
	d.svc.Tick(ctx)
	fake.set(after, nil)
	d.svc.Tick(ctx)
	if got := detail(t, d.svc); got.State != configdrift.StateDrift || got.Digest != after.Digest {
		t.Fatalf("precondition: state %q with digest %q (%s), want drift with the digest of the changed reading", got.State, got.Digest, got.Sentence)
	}
	return d
}

// record is the baseline file as bytes.
func (d *drifting) record(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(d.n.baselinePath())
	if err != nil {
		t.Fatalf("read the baseline file: %v", err)
	}
	return raw
}

// TestConfirmRefusesAChangedReading: between the screen and the click a third
// party changes the node again and a reading sees it. The operator confirmed a
// state that is no longer the state; nothing is written, and the new one can be
// confirmed on its own digest (D-11: the digest is the compare-and-swap).
func TestConfirmRefusesAChangedReading(t *testing.T) {
	t.Parallel()

	d := newDrifting(t)
	ctx := context.Background()
	seen := detail(t, d.svc).Digest

	third := snapPatched(t, d.n, kubeletPatch2)
	if third.Digest == seen {
		t.Fatal("precondition: the second change must differ from the first")
	}
	d.fake.set(third, nil)
	d.svc.Tick(ctx)
	if got := detail(t, d.svc); got.State != configdrift.StateDrift || got.Digest != third.Digest {
		t.Fatalf("precondition: state %q digest %q, want drift with the digest of the third state", got.State, got.Digest)
	}
	rawBefore := d.record(t)

	err := d.svc.Confirm(ctx, "m1", seen)
	if !errors.Is(err, configdrift.ErrDigestChanged) {
		t.Fatalf("Confirm with the digest of the old reading returned %v, want ErrDigestChanged", err)
	}
	if !bytes.Equal(rawBefore, d.record(t)) {
		t.Error("a refused Confirm wrote the baseline")
	}
	if got := detail(t, d.svc); got.State != configdrift.StateDrift || got.Digest != third.Digest || got.Diff == nil {
		t.Errorf("after the refusal Detail says %q digest %q, want drift with the new digest and its diff", got.State, got.Digest)
	}

	if err := d.svc.Confirm(ctx, "m1", third.Digest); err != nil {
		t.Fatalf("Confirm with the digest of the current reading: %v", err)
	}
	rec, err := d.n.st.ConfigBaselines().Get(ctx, "m1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rec.Digest != third.Digest || rec.Source != model.BaselineConfirmed {
		t.Errorf("the baseline is %q from %q, want the third state, confirmed", rec.Digest, rec.Source)
	}
}

// TestConfirmRefusesWhatWasNeverRead: a daemon that has not read the node since
// it started has nothing to confirm, even when the record in the store carries
// a deviation from an earlier run.
func TestConfirmRefusesWhatWasNeverRead(t *testing.T) {
	t.Parallel()

	someDigest := strings.Repeat("a", 64)

	t.Run("no baseline", func(t *testing.T) {
		t.Parallel()
		n := newNode(t)
		svc := n.service(t)
		err := svc.Confirm(context.Background(), "m1", someDigest)
		if !errors.Is(err, configdrift.ErrNotRead) {
			t.Errorf("Confirm returned %v, want ErrNotRead", err)
		}
		if _, statErr := os.Stat(n.baselinePath()); !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("a refused Confirm left a baseline file behind (%v)", statErr)
		}
	})

	t.Run("a drift recorded by an earlier run", func(t *testing.T) {
		t.Parallel()
		d := newDrifting(t)
		rawBefore := d.record(t)
		digest := detail(t, d.svc).Digest

		// The daemon restarts: the store stays, the memory is empty.
		fresh := d.n.service(t, withRead(d.fake.Read))
		if got := detail(t, fresh); got.State != configdrift.StateNotRead {
			t.Fatalf("precondition: a fresh service says %q, want not-read", got.State)
		}
		err := fresh.Confirm(context.Background(), "m1", digest)
		if !errors.Is(err, configdrift.ErrNotRead) {
			t.Errorf("Confirm returned %v, want ErrNotRead", err)
		}
		if !bytes.Equal(rawBefore, d.record(t)) {
			t.Error("a refused Confirm wrote the baseline")
		}
	})
}

// TestConfirmRefusesAReadingThatIsNoLongerCurrent: a drift was read, and then
// the node stopped answering. The reading in memory is old; confirming it would
// confirm what the operator can no longer be told is still true.
func TestConfirmRefusesAReadingThatIsNoLongerCurrent(t *testing.T) {
	t.Parallel()

	d := newDrifting(t)
	ctx := context.Background()
	digest := detail(t, d.svc).Digest
	rawBefore := d.record(t)

	d.fake.set(machineconfig.Snap{}, errors.New("i/o timeout"))
	d.svc.Tick(ctx)
	if got := detail(t, d.svc); got.State != configdrift.StateUnreadable {
		t.Fatalf("precondition: state is %q, want unreadable", got.State)
	}

	err := d.svc.Confirm(ctx, "m1", digest)
	if !errors.Is(err, configdrift.ErrNotRead) {
		t.Errorf("Confirm of a reading that is not current returned %v, want ErrNotRead", err)
	}
	if !bytes.Equal(rawBefore, d.record(t)) {
		t.Error("a refused Confirm wrote the baseline")
	}
	if got := detail(t, d.svc); got.State != configdrift.StateUnreadable {
		t.Errorf("after the refusal Detail says %q, want unreadable", got.State)
	}
}

// TestConfirmRefusesWhenThereIsNothingToConfirm: a node that matches its
// baseline, one whose baseline was just laid, and one whose baseline cannot be
// trusted have no drift. The digest given is the reading's own, so that the
// digest check cannot be what refuses.
func TestConfirmRefusesWhenThereIsNothingToConfirm(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		state configdrift.State
		setup func(t *testing.T, d *drifting)
	}{
		{"ok", configdrift.StateOK, func(t *testing.T, d *drifting) {
			d.fake.set(d.before, nil)
			d.svc.Tick(context.Background())
		}},
		{"baseline-created", configdrift.StateBaselineCreated, nil},
		{"baseline-damaged", configdrift.StateBaselineDamaged, func(t *testing.T, d *drifting) {
			if err := os.WriteFile(d.n.baselinePath(), []byte("not json"), 0o600); err != nil {
				t.Fatalf("damage the baseline: %v", err)
			}
			d.svc.Tick(context.Background())
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var d *drifting
			if tc.setup == nil {
				n := newNode(t)
				fake := &fakeReader{}
				before, after := twoSnaps(t, n)
				fake.set(before, nil)
				d = &drifting{n: n, fake: fake, before: before, after: after, svc: n.service(t, withRead(fake.Read))}
				d.svc.Tick(context.Background())
			} else {
				d = newDrifting(t)
				tc.setup(t, d)
			}
			got := detail(t, d.svc)
			if got.State != tc.state {
				t.Fatalf("precondition: state is %q (%s), want %q", got.State, got.Sentence, tc.state)
			}
			if !sha256Hex.MatchString(got.Digest) {
				t.Fatalf("precondition: the reading has digest %q, want 64 hex characters", got.Digest)
			}
			rawBefore := d.record(t)

			err := d.svc.Confirm(context.Background(), "m1", got.Digest)
			if !errors.Is(err, configdrift.ErrNoDrift) {
				t.Errorf("Confirm in state %q returned %v, want ErrNoDrift", tc.state, err)
			}
			if !bytes.Equal(rawBefore, d.record(t)) {
				t.Errorf("a refused Confirm wrote the baseline")
			}
			if after := detail(t, d.svc); after.State != tc.state {
				t.Errorf("after the refusal Detail says %q, want %q", after.State, tc.state)
			}
		})
	}
}

// TestConfirmRefusesAMachineThatDoesNotExist: an id the inventory does not hold
// is a refusal of its own, with nothing written for it.
func TestConfirmRefusesAMachineThatDoesNotExist(t *testing.T) {
	t.Parallel()

	d := newDrifting(t)
	rawBefore := d.record(t)

	err := d.svc.Confirm(context.Background(), "no-such-machine", detail(t, d.svc).Digest)
	if !errors.Is(err, configdrift.ErrUnknownMachine) {
		t.Errorf("Confirm of an unknown machine returned %v, want ErrUnknownMachine", err)
	}
	if !bytes.Equal(rawBefore, d.record(t)) {
		t.Error("a refused Confirm wrote the baseline")
	}
	if _, statErr := os.Stat(strings.Replace(d.n.baselinePath(), "m1.json", "no-such-machine.json", 1)); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("a baseline was written for a machine that does not exist (%v)", statErr)
	}
}

// TestConfirmRefusesADigestThatIsNotTheReadings: whatever else it is -- empty,
// short, upper case, another valid digest, the right one with a character
// added -- a digest that is not the reading's is the same refusal.
func TestConfirmRefusesADigestThatIsNotTheReadings(t *testing.T) {
	t.Parallel()

	d := newDrifting(t)
	ctx := context.Background()
	right := detail(t, d.svc).Digest
	rawBefore := d.record(t)

	wrong := map[string]string{
		"empty":                   "",
		"short":                   "abc",
		"another valid digest":    d.before.Digest,
		"all a":                   strings.Repeat("a", 64),
		"upper case":              strings.ToUpper(right),
		"a prefix":                right[:63],
		"the right one and a 0":   right + "0",
		"the right one in spaces": " " + right + " ",
	}
	if strings.ToUpper(right) == right {
		t.Fatal("precondition: the digest has no letters to upper-case")
	}
	for name, digest := range wrong {
		err := d.svc.Confirm(ctx, "m1", digest)
		if !errors.Is(err, configdrift.ErrDigestChanged) {
			t.Errorf("%s: Confirm(%q) returned %v, want ErrDigestChanged", name, digest, err)
		}
	}
	if !bytes.Equal(rawBefore, d.record(t)) {
		t.Error("a refused Confirm wrote the baseline")
	}
	if got := detail(t, d.svc); got.State != configdrift.StateDrift {
		t.Errorf("after the refusals Detail says %q, want drift", got.State)
	}

	t.Run("a reading that has no digest is not confirmed by an empty one", func(t *testing.T) {
		t.Parallel()

		// A reader that hands back a snapshot without a digest is a defect;
		// the empty string a client sends must not equal it.
		d := newDrifting(t)
		broken := d.after
		broken.Digest = ""
		d.fake.set(broken, nil)
		d.svc.Tick(ctx)
		if got := detail(t, d.svc); got.State != configdrift.StateDrift || got.Digest != "" {
			t.Fatalf("precondition: state %q digest %q, want drift with no digest", got.State, got.Digest)
		}
		rawBefore := d.record(t)

		err := d.svc.Confirm(ctx, "m1", "")
		if !errors.Is(err, configdrift.ErrDigestChanged) {
			t.Errorf("Confirm(\"\") returned %v, want ErrDigestChanged", err)
		}
		if !bytes.Equal(rawBefore, d.record(t)) {
			t.Error("a refused Confirm wrote the baseline")
		}
	})
}

// barrier holds the first two callers until both are in, so that two confirms
// are past their first look at the node's state before either has finished.
type barrier struct {
	mu      sync.Mutex
	armed   bool
	waiting int
	release chan struct{}
}

func (b *barrier) arm() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.armed, b.waiting, b.release = true, 0, make(chan struct{})
}

func (b *barrier) pass() {
	b.mu.Lock()
	if !b.armed {
		b.mu.Unlock()
		return
	}
	b.waiting++
	if b.waiting == 2 {
		close(b.release)
		b.armed = false
		b.mu.Unlock()
		return
	}
	release := b.release
	b.mu.Unlock()
	<-release
}

// TestTwoConfirmationsOfTheSameStateAreOneConfirmation: two operators click on
// the same screen at once. One confirms; the other finds the node already ok.
// The record is written once and is readable.
func TestTwoConfirmationsOfTheSameStateAreOneConfirmation(t *testing.T) {
	t.Parallel()

	var gate barrier
	m := model.Machine{ID: "m1", Cluster: "c1", Hostname: "cp-1", Role: model.RoleControlPlane}
	d := newDrifting(t, withAllMachines(func(context.Context) ([]model.Machine, error) {
		gate.pass()
		return []model.Machine{m}, nil
	}))
	ctx := context.Background()
	digest := detail(t, d.svc).Digest
	before, err := d.n.st.ConfigBaselines().Get(ctx, "m1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	gate.arm()
	results := make([]error, 2)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = d.svc.Confirm(ctx, "m1", digest)
		}()
	}
	wg.Wait()

	var won, refused int
	for _, err := range results {
		switch {
		case err == nil:
			won++
		case errors.Is(err, configdrift.ErrNoDrift):
			refused++
		default:
			t.Errorf("a confirmation returned %v, want nil or ErrNoDrift", err)
		}
	}
	if won != 1 || refused != 1 {
		t.Errorf("%d confirmations succeeded and %d were refused with ErrNoDrift, want 1 and 1: %v", won, refused, results)
	}

	rec, err := d.n.st.ConfigBaselines().Get(ctx, "m1")
	if err != nil {
		t.Fatalf("the record is not readable after two confirmations: %v", err)
	}
	if rec.Source != model.BaselineConfirmed || rec.Digest != digest {
		t.Errorf("the record is %q from %q, want the confirmed state", rec.Digest, rec.Source)
	}
	if rec.Rev != before.Rev+1 {
		t.Errorf("the record is at rev %d, want %d: it was written more than once", rec.Rev, before.Rev+1)
	}
}

// TestConfirmDoesNotRestoreADamagedBaseline: the file went bad after the
// reading. Confirm confirms a real drift; it does not make a damaged baseline
// good by writing over it. The way out is the sentence baseline-damaged names.
func TestConfirmDoesNotRestoreADamagedBaseline(t *testing.T) {
	t.Parallel()

	cases := map[string]func(t *testing.T, path string){
		"garbage": func(t *testing.T, path string) {
			if err := os.WriteFile(path, []byte("this is not json"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"a changed snapshot under the old digest": func(t *testing.T, path string) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var rec map[string]any
			if err := json.Unmarshal(raw, &rec); err != nil {
				t.Fatal(err)
			}
			rec["snapshot"] = "edited: by hand\n"
			out, err := json.Marshal(rec)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, out, 0o600); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, damage := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			d := newDrifting(t)
			digest := detail(t, d.svc).Digest
			damage(t, d.n.baselinePath())
			rawBefore := d.record(t)

			err := d.svc.Confirm(context.Background(), "m1", digest)
			if err == nil {
				t.Fatal("Confirm over a damaged baseline succeeded")
			}
			if !bytes.Equal(rawBefore, d.record(t)) {
				t.Error("Confirm wrote over a damaged baseline")
			}
		})
	}
}

// TestConfirmWhenTheRecordWentAway: the record is gone, the reading is in
// memory. What the operator saw becomes the baseline, as a new record.
func TestConfirmWhenTheRecordWentAway(t *testing.T) {
	t.Parallel()

	d := newDrifting(t)
	ctx := context.Background()
	digest := detail(t, d.svc).Digest
	if err := os.Remove(d.n.baselinePath()); err != nil {
		t.Fatalf("remove the baseline: %v", err)
	}

	if err := d.svc.Confirm(ctx, "m1", digest); err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	rec, err := d.n.st.ConfigBaselines().Get(ctx, "m1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rec.Rev != 1 || rec.Source != model.BaselineConfirmed || rec.Digest != digest {
		t.Errorf("the record is rev %d, %q, %q, want a new record, confirmed, with the digest seen", rec.Rev, rec.Source, rec.Digest)
	}
}
