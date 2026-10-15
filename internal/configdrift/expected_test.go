package configdrift_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/configdrift"
	"github.com/holzcloud/holzkube-manager/internal/machineconfig"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// ownPatch is the change an own apply makes in these tests.
const ownPatch = "machine:\n  certSANs:\n    - 10.0.0.9\n"

// expecting is a node whose baseline was laid by a first reading, with a reader
// the test controls and a clock it moves. It has four versions of the node's
// configuration, with four different digests:
//
//	base  what the node was when the baseline was laid
//	own   base with the change an apply from this installation makes
//	third base with a change somebody else makes (KubeletConfig.image)
//	both  third with the own change on top, which is what an own apply produces
//	      on a node a third party changed before
type expecting struct {
	n    *node
	fake *fakeReader
	clk  *clock
	logs *logSink
	svc  *configdrift.Service

	base, own, third, both machineconfig.Snap
	rawOwn                 []byte // the bytes of "own", unredacted, as an apply would send them
}

func newExpecting(t *testing.T) *expecting {
	t.Helper()

	n := newNode(t)
	e := &expecting{n: n, fake: &fakeReader{}, clk: newClock(), logs: &logSink{}}

	raw, err := machineconfig.ApplyPatches(n.cl.ControlPlaneConfig, []string{ownPatch})
	if err != nil {
		t.Fatalf("ApplyPatches: %v", err)
	}
	both, err := machineconfig.ApplyPatches(n.cl.ControlPlaneConfig, []string{kubeletPatch, ownPatch})
	if err != nil {
		t.Fatalf("ApplyPatches: %v", err)
	}
	e.rawOwn = raw
	e.base, e.own, e.third, e.both = snapOf(t, n.cl.ControlPlaneConfig), snapOf(t, raw), snapPatched(t, n, kubeletPatch), snapOf(t, both)

	seen := map[string]string{}
	for name, s := range map[string]machineconfig.Snap{"base": e.base, "own": e.own, "third": e.third, "both": e.both} {
		if other, dup := seen[s.Digest]; dup {
			t.Fatalf("precondition: the versions %s and %s have the same digest, so the tests prove nothing", name, other)
		}
		seen[s.Digest] = name
	}

	e.svc = n.service(t, withRead(e.fake.Read), withClock(e.clk), withLog(e.logs))
	e.fake.set(e.base, nil)
	e.svc.Tick(context.Background())
	if got := e.rec(t); got.Digest != e.base.Digest || got.Rev != 1 {
		t.Fatalf("precondition: the baseline is rev %d digest %s, want rev 1 of the base", got.Rev, got.Digest)
	}
	return e
}

func (e *expecting) rec(t *testing.T) model.ConfigBaseline {
	t.Helper()
	b, err := e.n.st.ConfigBaselines().Get(context.Background(), "m1")
	if err != nil {
		t.Fatalf("Get the baseline: %v", err)
	}
	return b
}

func (e *expecting) file(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(e.n.baselinePath())
	if err != nil {
		t.Fatalf("read the baseline file: %v", err)
	}
	return raw
}

// expect leaves an expectation in the record, as the observer does before an
// apply, and checks that it is there.
func (e *expecting) expect(t *testing.T, snap machineconfig.Snap, mode string, at time.Time) {
	t.Helper()
	b := e.rec(t)
	b.Expected = &model.ExpectedConfig{Digest: snap.Digest, Mode: mode, At: at}
	if _, err := e.n.st.ConfigBaselines().Put(context.Background(), b); err != nil {
		t.Fatalf("lay an expectation: %v", err)
	}
	if got := e.rec(t).Expected; got == nil || got.Digest != snap.Digest || got.Mode != mode {
		t.Fatalf("precondition: the record's expectation is %+v, want %s/%s", got, mode, snap.Digest)
	}
}

// read makes the node say snap and runs one pass.
func (e *expecting) read(t *testing.T, snap machineconfig.Snap) configdrift.Detail {
	t.Helper()
	e.fake.set(snap, nil)
	e.svc.Tick(context.Background())
	return detail(t, e.svc)
}

// TestAnOwnApplyThatLandedBecomesTheBaseline: whichever mode the apply was
// made in, the configuration it sent, found on the node, is the baseline.
func TestAnOwnApplyThatLandedBecomesTheBaseline(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{"no-reboot", "reboot", "auto", "staged"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()

			e := newExpecting(t)
			e.expect(t, e.own, mode, e.clk.Now())
			e.clk.Advance(2 * time.Minute)

			d := e.read(t, e.own)
			if d.State != configdrift.StateOK {
				t.Fatalf("a landed own apply (%s) reads as %q (%s), want ok", mode, d.State, d.Sentence)
			}
			b := e.rec(t)
			if b.Source != model.BaselineApply || b.Digest != e.own.Digest || b.Snapshot != e.own.Text {
				t.Errorf("the baseline is %q/%s, want the landed configuration from source apply", b.Source, b.Digest)
			}
			if b.Expected != nil || !b.DriftSince.IsZero() || !b.AppliedOnDriftAt.IsZero() {
				t.Errorf("the baseline still carries %+v", b)
			}
			if !b.TakenAt.Equal(e.clk.Now()) {
				t.Errorf("TakenAt is %v, want the time of the reading %v", b.TakenAt, e.clk.Now())
			}
		})
	}
}

// TestAStagedConfigurationPendingIsNotDrift: a staged configuration changes
// nothing until the next boot, which may be days away. The node stays ok and
// the expectation stays, with no end of its own; when it lands, it is the
// baseline.
func TestAStagedConfigurationPendingIsNotDrift(t *testing.T) {
	t.Parallel()

	e := newExpecting(t)
	e.expect(t, e.own, "staged", e.clk.Now())
	e.clk.Advance(72 * time.Hour)

	if d := e.read(t, e.base); d.State != configdrift.StateOK {
		t.Fatalf("a node whose staged configuration is waiting reads as %q (%s), want ok", d.State, d.Sentence)
	}
	if b := e.rec(t); b.Expected == nil || b.Expected.Digest != e.own.Digest || b.Expected.Mode != "staged" {
		t.Fatalf("after three days the expectation is %+v, want the staged one still there", b.Expected)
	}

	if d := e.read(t, e.own); d.State != configdrift.StateOK {
		t.Fatalf("the staged configuration landed and reads as %q (%s), want ok", d.State, d.Sentence)
	}
	if b := e.rec(t); b.Source != model.BaselineApply || b.Digest != e.own.Digest || b.Expected != nil {
		t.Errorf("the landed staged configuration is not the baseline: %+v", b)
	}
}

// TestAStagedConfigurationNeverSettles: staged changes nothing now, so a third
// version is not the apply on its way; it is somebody else's change, at once.
func TestAStagedConfigurationNeverSettles(t *testing.T) {
	t.Parallel()

	e := newExpecting(t)
	e.expect(t, e.own, "staged", e.clk.Now())
	e.clk.Advance(time.Minute)

	d := e.read(t, e.third)
	if d.State != configdrift.StateDrift {
		t.Fatalf("a third version while a staged one waits reads as %q (%s), want drift: staged has no window", d.State, d.Sentence)
	}
	if b := e.rec(t); b.DriftSince.IsZero() {
		t.Error("the start of the drift was not written")
	}
}

// TestAnApplyInFlightIsSettlingNotDrift: an apply that has not landed (it
// reboots the node, or is on its way) is settling for at most the window, and
// drift after it. Nothing is written while it settles.
func TestAnApplyInFlightIsSettlingNotDrift(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{"no-reboot", "reboot", "auto"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()

			e := newExpecting(t)
			e.expect(t, e.own, mode, e.clk.Now())
			before := e.file(t)
			e.clk.Advance(time.Minute)

			d := e.read(t, e.third)
			if d.State != configdrift.StateSettling {
				t.Fatalf("a young apply (%s) and a version that is neither reads as %q (%s), want settling", mode, d.State, d.Sentence)
			}
			if d.Diff != nil || !d.DriftSince.IsZero() {
				t.Errorf("a settling node has a diff or a drift start: %+v", d)
			}
			if !strings.Contains(d.Sentence, "not taken effect") {
				t.Errorf("the sentence %q does not say the apply has not taken effect", d.Sentence)
			}
			if !bytes.Equal(before, e.file(t)) {
				t.Error("a reading that settles rewrote the baseline")
			}

			// One second inside the window is still settling; at the window it is drift.
			e.clk.Advance(configdrift.DefaultSettleWindow - time.Minute - time.Second)
			if d := e.read(t, e.third); d.State != configdrift.StateSettling {
				t.Fatalf("one second before the window ends the state is %q, want settling", d.State)
			}
			e.clk.Advance(time.Second)
			d = e.read(t, e.third)
			if d.State != configdrift.StateDrift {
				t.Fatalf("at the end of the window the state is %q (%s), want drift", d.State, d.Sentence)
			}
			if d.DriftSince.IsZero() {
				t.Error("drift after the window has no start")
			}
		})
	}
}

// TestANoOpApplyClearsItsExpectation: an apply that changed nothing leaves an
// expectation that is the baseline itself; the next reading removes it.
func TestANoOpApplyClearsItsExpectation(t *testing.T) {
	t.Parallel()

	e := newExpecting(t)
	e.expect(t, e.base, "no-reboot", e.clk.Now())

	if d := e.read(t, e.base); d.State != configdrift.StateOK {
		t.Fatalf("state is %q (%s), want ok", d.State, d.Sentence)
	}
	b := e.rec(t)
	if b.Expected != nil {
		t.Errorf("the expectation of an apply that changed nothing is still there: %+v", b.Expected)
	}
	if b.Source != model.BaselineFirstSight {
		t.Errorf("the baseline's source is %q, want first-sight: nothing was applied that moved it", b.Source)
	}
}

// TestAnApplyThatNeverLandedStopsBeingExpected: an apply that never landed stops
// counting after the window, and somebody else applying the very same bytes
// later is a change to report.
func TestAnApplyThatNeverLandedStopsBeingExpected(t *testing.T) {
	t.Parallel()

	e := newExpecting(t)
	e.expect(t, e.own, "reboot", e.clk.Now())
	e.clk.Advance(configdrift.DefaultSettleWindow)

	if d := e.read(t, e.base); d.State != configdrift.StateOK {
		t.Fatalf("state is %q (%s), want ok", d.State, d.Sentence)
	}
	if b := e.rec(t); b.Expected != nil {
		t.Fatalf("an expectation that ran out is still in the record: %+v", b.Expected)
	}

	d := e.read(t, e.own)
	if d.State != configdrift.StateDrift {
		t.Fatalf("a third party applying the bytes of an apply that never landed reads as %q (%s), want drift", d.State, d.Sentence)
	}
	if b := e.rec(t); b.Source == model.BaselineApply {
		t.Error("the stale expectation became the baseline")
	}
}

// TestAnExpectationThatRanOutIsRemovedWhenTheNodeDrifts: the clean-up is not
// only for a node that is back at its baseline; a node that differs for another
// reason loses it with the write that dates the drift.
func TestAnExpectationThatRanOutIsRemovedWhenTheNodeDrifts(t *testing.T) {
	t.Parallel()

	e := newExpecting(t)
	e.expect(t, e.own, "reboot", e.clk.Now())
	e.clk.Advance(configdrift.DefaultSettleWindow + time.Minute)

	if d := e.read(t, e.third); d.State != configdrift.StateDrift {
		t.Fatalf("state is %q (%s), want drift", d.State, d.Sentence)
	}
	b := e.rec(t)
	if b.Expected != nil {
		t.Errorf("an expectation that ran out survived the write of a drift: %+v", b.Expected)
	}
	if b.DriftSince.IsZero() || b.Rev != 3 {
		t.Errorf("the record is rev %d with DriftSince %v, want rev 3 (expectation, then one write) and a start", b.Rev, b.DriftSince)
	}
}

// TestAnExpectationThatRanOutIsNotAdopted is the other half: a record that still
// holds an expectation past its window (nothing has read the node since) does
// not make the node's matching configuration the baseline.
func TestAnExpectationThatRanOutIsNotAdopted(t *testing.T) {
	t.Parallel()

	e := newExpecting(t)
	e.expect(t, e.own, "no-reboot", e.clk.Now())
	e.clk.Advance(2 * configdrift.DefaultSettleWindow)

	d := e.read(t, e.own)
	if d.State != configdrift.StateDrift {
		t.Fatalf("a configuration that matches an expectation two windows old reads as %q (%s), want drift", d.State, d.Sentence)
	}
	if b := e.rec(t); b.Source == model.BaselineApply || b.Digest != e.base.Digest {
		t.Errorf("the baseline moved to %q/%s on an expectation that had run out", b.Source, b.Digest)
	}
}

// patchedConfig is the node's active configuration with a patch applied, the
// bytes an apply sends.
func (n *node) patchedConfig(t *testing.T, patch string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cc, err := n.connect(ctx, n.machine.ID)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = cc.Close() }()
	active, err := cc.MachineConfigYAML(ctx)
	if err != nil {
		t.Fatalf("MachineConfigYAML: %v", err)
	}
	patched, err := machineconfig.ApplyPatches(active, []string{patch})
	if err != nil {
		t.Fatalf("ApplyPatches: %v", err)
	}
	return patched
}

// TestAFailedApplyLeavesNoExpectation: a refused (dry-run) or rejected apply
// changed nothing, so it expects nothing; somebody else applying exactly that
// configuration afterwards is drift.
func TestAFailedApplyLeavesNoExpectation(t *testing.T) {
	t.Parallel()

	rows := []struct {
		name   string
		dryRun bool
		reject bool
	}{
		{"dry-run", true, false},
		{"rejected by the node", false, true},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()

			n := newNode(t)
			svc := n.service(t)
			bg := context.Background()
			svc.Tick(bg)
			cfg := n.patchedConfig(t, ownPatch)

			lift := func() {}
			if row.reject {
				l, err := n.sim.Inject(talossim.Scenario{Name: talossim.ScenarioRejectApply})
				if err != nil {
					t.Fatalf("Inject: %v", err)
				}
				lift = l
				defer lift()
			}
			ctx, cancel := context.WithTimeout(bg, 30*time.Second)
			defer cancel()
			cc, err := talos.NewClusterClient(ctx, n.sim.Dialer(),
				talos.Target{Cluster: "c1", Machine: "m1", Addr: n.sim.Host()}, n.sim.ClientCreds(), talos.Mode{DryRun: row.dryRun})
			if err != nil {
				t.Fatalf("NewClusterClient: %v", err)
			}
			defer func() { _ = cc.Close() }()
			cc.SetApplyObserver(svc.ApplyObserver())

			if _, err := cc.ApplyConfigurationWithMode(ctx, cfg, "no-reboot"); err == nil {
				t.Fatal("precondition: the apply was not refused")
			}
			b, err := n.st.ConfigBaselines().Get(bg, "m1")
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if b.Expected != nil {
				t.Fatalf("a failed apply left an expectation: %+v", b.Expected)
			}

			// Somebody else applies the very same configuration.
			lift()
			n.thirdPartyChange(t, ownPatch)
			svc.Tick(bg)
			d, err := svc.Detail(bg, "m1")
			if err != nil {
				t.Fatalf("Detail: %v", err)
			}
			if d.State != configdrift.StateDrift {
				t.Fatalf("a third party applying what a failed apply had tried reads as %q (%s), want drift", d.State, d.Sentence)
			}
		})
	}
}

// observe calls the service's observer as a client would, with the bytes the
// test names, and returns what it hands back.
func observe(e *expecting, mode string, cfg []byte) func(error) {
	return e.svc.ApplyObserver()(context.Background(), talos.ApplyAttempt{
		Cluster: "c1", Machine: "m1", Config: cfg, Mode: mode,
	})
}

// TestTryIsNeverAdopted: try rolls back by itself, so it leaves no expectation
// and no poke, through the client and straight at the observer.
func TestTryIsNeverAdopted(t *testing.T) {
	t.Parallel()

	t.Run("the observer", func(t *testing.T) {
		t.Parallel()

		e := newExpecting(t)
		before := e.file(t)
		reads := e.fake.count()

		if out := observe(e, "try", e.rawOwn); out != nil {
			t.Error("the observer wants to hear the result of a try; there is nothing to do with it")
		}
		if !bytes.Equal(before, e.file(t)) {
			t.Errorf("a try changed the record:\n before %s\n after  %s", before, e.file(t))
		}
		e.svc.Flush(context.Background())
		if got := e.fake.count(); got != reads {
			t.Errorf("a try made the node be read (%d reads, was %d)", got, reads)
		}
	})

	t.Run("through the client", func(t *testing.T) {
		t.Parallel()

		n := newNode(t)
		cr := &countingReader{n: n}
		svc := n.service(t, withRead(cr.Read))
		own := n.ownApplies(svc)
		bg := context.Background()
		svc.Tick(bg)
		file, err := os.ReadFile(n.baselinePath())
		if err != nil {
			t.Fatalf("read the baseline file: %v", err)
		}
		reads := cr.read.Load()

		ctx, cancel := context.WithTimeout(bg, 30*time.Second)
		defer cancel()
		if _, err := own.Apply(ctx, "m1", []string{ownPatch}, machineconfig.ModeTry); err != nil {
			t.Fatalf("Apply: %v", err)
		}
		after, err := os.ReadFile(n.baselinePath())
		if err != nil {
			t.Fatalf("read the baseline file: %v", err)
		}
		if !bytes.Equal(file, after) {
			t.Errorf("a try changed the record:\n before %s\n after  %s", file, after)
		}
		svc.Flush(bg)
		if got := cr.read.Load(); got != reads {
			t.Errorf("a try made the node be read (%d reads, was %d)", got, reads)
		}
	})
}

// TestEveryModeButTryLeavesAnExpectation: before the RPC, each mode leaves an
// expectation with its own name and the digest of what is sent, and only that.
func TestEveryModeButTryLeavesAnExpectation(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{"no-reboot", "reboot", "staged", "auto", "try"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()

			e := newExpecting(t)
			before := e.file(t)

			observe(e, mode, e.rawOwn)

			if mode == "try" {
				if !bytes.Equal(before, e.file(t)) {
					t.Error("a try left something in the record")
				}
				return
			}
			b := e.rec(t)
			if b.Expected == nil {
				t.Fatalf("mode %s left no expectation", mode)
			}
			if b.Expected.Mode != mode || b.Expected.Digest != e.own.Digest {
				t.Errorf("the expectation is %+v, want mode %s and the digest of what is sent %s", *b.Expected, mode, e.own.Digest)
			}
			if !b.Expected.At.Equal(e.clk.Now()) {
				t.Errorf("the expectation's time is %v, want now %v", b.Expected.At, e.clk.Now())
			}
			if b.Digest != e.base.Digest || b.Snapshot != e.base.Text {
				t.Error("the observer moved the baseline")
			}
		})
	}
}

// TestAnApplyForANodeWithoutABaselineSetsNothing: the first reading lays a
// baseline out of what the node says; an apply does not invent one.
func TestAnApplyForANodeWithoutABaselineSetsNothing(t *testing.T) {
	t.Parallel()

	n := newNode(t)
	svc := n.service(t)
	e := &expecting{svc: svc}

	out := observe(e, "no-reboot", n.cl.ControlPlaneConfig)
	if out != nil {
		out(nil)
	}
	if _, err := os.Stat(n.baselinePath()); !os.IsNotExist(err) {
		t.Errorf("an apply for a node nobody had read left a baseline file (stat: %v)", err)
	}
}

// TestAnApplyThatCannotBeSnapshottedDoesNotFailTheApply: bytes that are not a
// configuration give no expectation and no error, only a line in the log.
func TestAnApplyThatCannotBeSnapshottedDoesNotFailTheApply(t *testing.T) {
	t.Parallel()

	e := newExpecting(t)
	garbage := []byte("machine: [this is not\n  a configuration")
	if _, err := machineconfig.Snapshot(garbage); err == nil {
		t.Fatal("precondition: the bytes load as a configuration")
	}
	before := e.file(t)

	if out := observe(e, "no-reboot", garbage); out != nil {
		t.Error("the observer returned an outcome for a configuration it could not make a snapshot of")
	}
	if !bytes.Equal(before, e.file(t)) {
		t.Error("the record changed")
	}
	logged := false
	for _, line := range e.logs.Lines() {
		if strings.Contains(line, "cannot be snapshotted") {
			logged = true
		}
		if strings.Contains(line, "this is not") {
			t.Errorf("the log holds configuration content: %q", line)
		}
	}
	if !logged {
		t.Errorf("nothing was logged; lines: %v", e.logs.Lines())
	}
}

// TestTheOutcomeOfAnEarlierApplyDoesNotRemoveALaterExpectation: two applies, the
// first one's failure comes in after the second has left its own expectation.
func TestTheOutcomeOfAnEarlierApplyDoesNotRemoveALaterExpectation(t *testing.T) {
	t.Parallel()

	e := newExpecting(t)
	rawThird, err := machineconfig.ApplyPatches(e.n.cl.ControlPlaneConfig, []string{kubeletPatch})
	if err != nil {
		t.Fatalf("ApplyPatches: %v", err)
	}

	first := observe(e, "no-reboot", e.rawOwn)
	e.clk.Advance(time.Second)
	second := observe(e, "no-reboot", rawThird)
	if first == nil || second == nil {
		t.Fatal("precondition: the observer returned no outcome")
	}
	if got := e.rec(t).Expected; got == nil || got.Digest != e.third.Digest {
		t.Fatalf("precondition: the expectation is %+v, want the second apply's", got)
	}

	first(context.DeadlineExceeded)
	if got := e.rec(t).Expected; got == nil || got.Digest != e.third.Digest {
		t.Fatalf("the failure of the first apply removed the expectation of the second: %+v", got)
	}
	second(context.DeadlineExceeded)
	if got := e.rec(t).Expected; got != nil {
		t.Errorf("the failure of the second apply left its own expectation: %+v", got)
	}
}

// TestAnOwnApplyLeavesNoSecretInTheBaselineFile: the bytes of an apply hold the
// cluster's secrets. What the observer leaves is a digest, a mode and a time,
// before the apply lands and after.
func TestAnOwnApplyLeavesNoSecretInTheBaselineFile(t *testing.T) {
	t.Parallel()

	n := newNode(t)
	svc := n.service(t)
	own := n.ownApplies(svc)
	bg := context.Background()
	svc.Tick(bg)

	ctx, cancel := context.WithTimeout(bg, 30*time.Second)
	defer cancel()
	if _, err := own.Apply(ctx, "m1", []string{ownPatch}, machineconfig.ModeNoReboot); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	check := func(when string) {
		t.Helper()
		file, err := os.ReadFile(n.baselinePath())
		if err != nil {
			t.Fatalf("read the baseline file: %v", err)
		}
		for name, secret := range n.secrets(t) {
			for _, form := range forms(secret) {
				if strings.Contains(string(file), form) {
					t.Errorf("%s: the baseline file holds %s", when, name)
				}
			}
		}
		var rec map[string]any
		if err := json.Unmarshal(file, &rec); err != nil {
			t.Fatalf("the file is not JSON: %v", err)
		}
		if exp, ok := rec["expected"].(map[string]any); ok {
			keys := map[string]bool{}
			for k := range exp {
				keys[k] = true
			}
			if len(keys) != 3 || !keys["digest"] || !keys["mode"] || !keys["at"] {
				t.Errorf("%s: the expectation has the keys %v, want digest, mode and at", when, keys)
			}
		}
	}
	check("before the apply landed in a reading")
	if rec, _ := n.st.ConfigBaselines().Get(bg, "m1"); rec.Expected == nil {
		t.Fatal("precondition: there is no expectation to look at")
	}
	svc.Flush(bg)
	check("after")
}

// openDrift makes a third party's change a drift that is open, and returns the
// start of it.
func (e *expecting) openDrift(t *testing.T) time.Time {
	t.Helper()
	if d := e.read(t, e.third); d.State != configdrift.StateDrift {
		t.Fatalf("precondition: state %q, want drift", d.State)
	}
	since := e.rec(t).DriftSince
	if since.IsZero() {
		t.Fatal("precondition: the drift has no start")
	}
	return since
}

// TestAnOwnApplyLandingOnAnOpenDriftKeepsItOpen (D-13): somebody else changed
// the node, nobody confirmed it, and then an apply from here landed on top. The
// baseline is not moved to the sum, the start of the drift stays, the screen
// says the state includes the earlier change, and the diff shows both.
func TestAnOwnApplyLandingOnAnOpenDriftKeepsItOpen(t *testing.T) {
	t.Parallel()

	e := newExpecting(t)
	since := e.openDrift(t)
	e.clk.Advance(time.Hour)
	e.expect(t, e.both, "no-reboot", e.clk.Now())
	e.clk.Advance(time.Minute)

	d := e.read(t, e.both)
	if d.State != configdrift.StateDrift {
		t.Fatalf("an apply that landed on an open drift reads as %q (%s), want drift", d.State, d.Sentence)
	}
	if !d.DriftSince.Equal(since) {
		t.Errorf("DriftSince is %v, want the earlier start %v", d.DriftSince, since)
	}
	if !strings.Contains(d.Sentence, "earlier change that was never confirmed") {
		t.Errorf("the sentence %q does not name the earlier change that was never confirmed", d.Sentence)
	}
	if d.Diff == nil {
		t.Fatal("no diff")
	}
	paths := map[string]bool{}
	for _, c := range d.Diff.Changes {
		paths[c.Path] = true
	}
	if !paths["KubeletConfig.image"] || len(d.Diff.Changes) < 2 {
		t.Errorf("the diff has %v, want the earlier change (KubeletConfig.image) and the own one", paths)
	}

	b := e.rec(t)
	if b.Digest != e.base.Digest || b.Snapshot != e.base.Text || b.Source != model.BaselineFirstSight {
		t.Errorf("the baseline moved to %q/%s; it must stay what was never confirmed against", b.Source, b.Digest)
	}
	if !b.DriftSince.Equal(since) {
		t.Errorf("the record's DriftSince is %v, want %v", b.DriftSince, since)
	}
	if b.Expected != nil {
		t.Errorf("the expectation is still there: %+v", b.Expected)
	}
	if !b.AppliedOnDriftAt.Equal(e.clk.Now()) {
		t.Errorf("AppliedOnDriftAt is %v, want the time of the reading %v", b.AppliedOnDriftAt, e.clk.Now())
	}

	// Further readings change nothing.
	rev := b.Rev
	e.clk.Advance(time.Hour)
	if d := e.read(t, e.both); d.State != configdrift.StateDrift || !d.DriftSince.Equal(since) {
		t.Errorf("a later reading says %q since %v, want drift since %v", d.State, d.DriftSince, since)
	}
	if got := e.rec(t); got.Rev != rev || !got.AppliedOnDriftAt.Equal(b.AppliedOnDriftAt) {
		t.Errorf("a later reading rewrote the record: rev %d, was %d", got.Rev, rev)
	}
}

// TestAnApplyInFlightOnAnOpenDriftIsStillDrift (D-13): an open drift is never
// settling, however young the apply that is expected.
func TestAnApplyInFlightOnAnOpenDriftIsStillDrift(t *testing.T) {
	t.Parallel()

	e := newExpecting(t)
	since := e.openDrift(t)
	e.expect(t, e.both, "reboot", e.clk.Now())
	e.clk.Advance(time.Minute)

	d := e.read(t, e.own)
	if d.State != configdrift.StateDrift {
		t.Fatalf("a version that is neither the baseline nor the expected one, on an open drift, reads as %q (%s), want drift", d.State, d.Sentence)
	}
	if !d.DriftSince.Equal(since) {
		t.Errorf("DriftSince is %v, want %v", d.DriftSince, since)
	}
}

// TestConfirmingAfterALandingMakesTheLandedStateTheBaseline (D-13): the only
// two ways out of a drift that an apply landed on.
func TestConfirmingAfterALandingMakesTheLandedStateTheBaseline(t *testing.T) {
	t.Parallel()

	landed := func(t *testing.T) *expecting {
		t.Helper()
		e := newExpecting(t)
		e.openDrift(t)
		e.clk.Advance(time.Hour)
		e.expect(t, e.both, "no-reboot", e.clk.Now())
		if d := e.read(t, e.both); d.State != configdrift.StateDrift || e.rec(t).AppliedOnDriftAt.IsZero() {
			t.Fatalf("precondition: state %q, AppliedOnDriftAt %v", d.State, e.rec(t).AppliedOnDriftAt)
		}
		return e
	}

	t.Run("the operator confirms", func(t *testing.T) {
		t.Parallel()

		e := landed(t)
		d := detail(t, e.svc)
		if err := e.svc.Confirm(context.Background(), "m1", d.Digest); err != nil {
			t.Fatalf("Confirm: %v", err)
		}
		b := e.rec(t)
		if b.Digest != e.both.Digest || b.Source != model.BaselineConfirmed {
			t.Errorf("the baseline is %q/%s, want the landed state, confirmed", b.Source, b.Digest)
		}
		if !b.DriftSince.IsZero() || !b.AppliedOnDriftAt.IsZero() || b.Expected != nil {
			t.Errorf("the confirmed baseline still carries %+v", b)
		}
		if got := detail(t, e.svc); got.State != configdrift.StateOK {
			t.Errorf("state after confirming is %q, want ok", got.State)
		}
	})

	t.Run("the node returns to the baseline", func(t *testing.T) {
		t.Parallel()

		e := landed(t)
		if d := e.read(t, e.base); d.State != configdrift.StateOK {
			t.Fatalf("state is %q (%s), want ok", d.State, d.Sentence)
		}
		b := e.rec(t)
		if b.Digest != e.base.Digest || !b.DriftSince.IsZero() || !b.AppliedOnDriftAt.IsZero() {
			t.Errorf("the record is %+v, want the base with both fields empty", b)
		}
	})
}
