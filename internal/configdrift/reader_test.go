package configdrift_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/configdrift"
	"github.com/holzcloud/holzkube-manager/internal/machineconfig"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
)

const kubeletPatch = "apiVersion: v1alpha1\nkind: KubeletConfig\nimage: ghcr.io/siderolabs/kubelet:v1.34.2\n"

// twoSnaps returns the snapshot of the node's configuration and the snapshot of
// the same configuration after a third party changed one document.
func twoSnaps(t *testing.T, n *node) (before, after machineconfig.Snap) {
	t.Helper()
	patched, err := machineconfig.ApplyPatches(n.cl.ControlPlaneConfig, []string{kubeletPatch})
	if err != nil {
		t.Fatalf("ApplyPatches: %v", err)
	}
	before, after = snapOf(t, n.cl.ControlPlaneConfig), snapOf(t, patched)
	if before.Digest == after.Digest {
		t.Fatal("precondition: the patch must change the snapshot")
	}
	return before, after
}

func detail(t *testing.T, svc *configdrift.Service) configdrift.Detail {
	t.Helper()
	d, err := svc.Detail(context.Background(), "m1")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	return d
}

// splitDocuments cuts a snapshot text into its documents at the lines that hold
// only "---". A plain split at "---" would cut through the line of a PEM
// armour, which ends in dashes.
func splitDocuments(text string) []string {
	var docs []string
	cur := ""
	for _, line := range strings.SplitAfter(text, "\n") {
		if line == "---\n" {
			docs = append(docs, cur)
			cur = ""
			continue
		}
		cur += line
	}
	return append(docs, cur)
}

func digestOf(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// TestAFailedReadIsNeverNoDrift: a node that cannot be read is unknown, never
// fine. The last good reading stays, with its time.
func TestAFailedReadIsNeverNoDrift(t *testing.T) {
	t.Parallel()

	n := newNode(t)
	clk := newClock()
	fake := &fakeReader{}
	svc := n.service(t, withRead(fake.Read), withClock(clk))

	base, _ := twoSnaps(t, n)
	fake.set(base, nil)
	svc.Tick(context.Background())
	if d := detail(t, svc); d.State != configdrift.StateBaselineCreated {
		t.Fatalf("precondition: first reading gave %q, want baseline-created", d.State)
	}
	goodAt := clk.Now()

	clk.Advance(3 * time.Hour)
	fake.set(machineconfig.Snap{}, errors.New("dial tcp 192.168.1.10:50000: i/o timeout"))
	for i := range 2 {
		svc.Tick(context.Background())
		d := detail(t, svc)
		if d.State == configdrift.StateOK || d.State == configdrift.StateBaselineCreated {
			t.Fatalf("pass %d: a node that could not be read is %q", i, d.State)
		}
		if d.State != configdrift.StateUnreadable {
			t.Fatalf("pass %d: state is %q, want unreadable", i, d.State)
		}
		if !strings.Contains(d.Sentence, "could not be read") || !strings.Contains(d.Sentence, "i/o timeout") {
			t.Errorf("the sentence %q does not give the reason", d.Sentence)
		}
		if !strings.Contains(d.Sentence, "Last good reading: 2026-10-14 09:00 UTC") {
			t.Errorf("the sentence %q does not give the time of the last good reading", d.Sentence)
		}
		if !d.ReadAt.Equal(goodAt) {
			t.Errorf("ReadAt is %v, want the last good reading %v", d.ReadAt, goodAt)
		}
		if d.Digest != base.Digest {
			t.Errorf("the last good digest was lost: %q", d.Digest)
		}
		if !d.CheckedAt.After(goodAt) {
			t.Errorf("CheckedAt %v is not after the last good reading", d.CheckedAt)
		}
	}
}

// TestANodeNeverReadAndNotAnsweringHasNoBaseline: nothing to remember means
// nothing is written.
func TestANodeNeverReadAndNotAnsweringHasNoBaseline(t *testing.T) {
	t.Parallel()

	n := newNode(t)
	fake := &fakeReader{}
	fake.set(machineconfig.Snap{}, errors.New("connection refused"))
	svc := n.service(t, withRead(fake.Read))

	svc.Tick(context.Background())

	d := detail(t, svc)
	if d.State != configdrift.StateUnreadable {
		t.Fatalf("state is %q, want unreadable", d.State)
	}
	if !d.ReadAt.IsZero() {
		t.Errorf("ReadAt is %v for a node that was never read", d.ReadAt)
	}
	if _, err := n.st.ConfigBaselines().Get(context.Background(), "m1"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("a baseline exists for a node nobody read: %v", err)
	}
	if _, err := os.Stat(n.baselinePath()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the baseline file exists: %v", err)
	}
	if fake.calls == 0 {
		t.Fatal("precondition: the reader was never called")
	}
}

// TestADriftingNodeStaysDriftingWhenItStopsAnswering: the start of a deviation
// is kept through an outage, in the same process and across a restart, and the
// record is not touched by a failed reading.
func TestADriftingNodeStaysDriftingWhenItStopsAnswering(t *testing.T) {
	t.Parallel()

	n := newNode(t)
	clk := newClock()
	fake := &fakeReader{}
	svc := n.service(t, withRead(fake.Read), withClock(clk))
	before, after := twoSnaps(t, n)
	ctx := context.Background()

	fake.set(before, nil)
	svc.Tick(ctx)
	clk.Advance(time.Hour)
	fake.set(after, nil)
	svc.Tick(ctx)
	driftStart := clk.Now()
	if d := detail(t, svc); d.State != configdrift.StateDrift || !d.DriftSince.Equal(driftStart) {
		t.Fatalf("precondition: state %q since %v, want drift since %v", d.State, d.DriftSince, driftStart)
	}
	recBefore, err := os.ReadFile(n.baselinePath())
	if err != nil {
		t.Fatalf("read the baseline file: %v", err)
	}
	lastGood := clk.Now()

	// The node stops answering.
	clk.Advance(5 * time.Hour)
	fake.set(machineconfig.Snap{}, errors.New("i/o timeout"))
	svc.Tick(ctx)
	d := detail(t, svc)
	if d.State != configdrift.StateUnreadable {
		t.Fatalf("state is %q, want unreadable", d.State)
	}
	if !d.DriftSince.Equal(driftStart) {
		t.Errorf("DriftSince is %v after a failed reading, want %v", d.DriftSince, driftStart)
	}
	if !d.ReadAt.Equal(lastGood) || d.Digest != after.Digest {
		t.Errorf("the last good reading was lost: %v %q", d.ReadAt, d.Digest)
	}
	if rec, _ := os.ReadFile(n.baselinePath()); !bytes.Equal(rec, recBefore) {
		t.Error("a failed reading changed the baseline file")
	}

	// It answers again, with the same deviating configuration: the start is
	// the original one.
	fake.set(after, nil)
	svc.Tick(ctx)
	if d := detail(t, svc); d.State != configdrift.StateDrift || !d.DriftSince.Equal(driftStart) {
		t.Errorf("after the return: state %q since %v, want drift since %v", d.State, d.DriftSince, driftStart)
	}

	// A restart on top of an outage: the new process has no reading at all and
	// its first attempt fails; the start still comes from the record.
	n.reopen(t)
	restarted := n.service(t, withRead(fake.Read), withClock(clk))
	fake.set(machineconfig.Snap{}, errors.New("i/o timeout"))
	restarted.Tick(ctx)
	d = detail(t, restarted)
	if d.State != configdrift.StateUnreadable || !d.DriftSince.Equal(driftStart) {
		t.Errorf("after a restart and a failed reading: state %q since %v, want unreadable since %v",
			d.State, d.DriftSince, driftStart)
	}
}

// TestADamagedBaselineIsReportedAndNeverOverwritten: a file nobody can trust is
// neither a first sight nor something to write over.
func TestADamagedBaselineIsReportedAndNeverOverwritten(t *testing.T) {
	t.Parallel()

	damage := map[string]func(t *testing.T, raw []byte) []byte{
		"garbage": func(*testing.T, []byte) []byte { return []byte("this is not a baseline {") },
		"a changed snapshot under the old digest": func(t *testing.T, raw []byte) []byte {
			t.Helper()
			var m map[string]any
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			text, ok := m["snapshot"].(string)
			if !ok {
				t.Fatal("precondition: the stored snapshot is not a string")
			}
			m["snapshot"] = text + "extra: injected\n"
			out, err := json.Marshal(m)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			return out
		},
		"an unknown format": func(t *testing.T, raw []byte) []byte {
			t.Helper()
			var m map[string]any
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			m["format"] = 99
			out, err := json.Marshal(m)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			return out
		},
		"an empty snapshot": func(t *testing.T, raw []byte) []byte {
			t.Helper()
			var m map[string]any
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			m["snapshot"] = ""
			m["digest"] = digestOf("")
			out, err := json.Marshal(m)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			return out
		},
	}

	for name, damageIt := range damage {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			n := newNode(t)
			fake := &fakeReader{}
			svc := n.service(t, withRead(fake.Read))
			before, after := twoSnaps(t, n)
			ctx := context.Background()

			fake.set(before, nil)
			svc.Tick(ctx)
			if d := detail(t, svc); d.State != configdrift.StateBaselineCreated {
				t.Fatalf("precondition: first reading gave %q", d.State)
			}
			raw, err := os.ReadFile(n.baselinePath())
			if err != nil {
				t.Fatalf("read the baseline file: %v", err)
			}
			damaged := damageIt(t, raw)
			if bytes.Equal(damaged, raw) {
				t.Fatal("precondition: the file was not damaged")
			}
			if err := os.WriteFile(n.baselinePath(), damaged, 0o600); err != nil {
				t.Fatalf("damage the baseline: %v", err)
			}

			// Once with the configuration the baseline was taken from, once with
			// a changed one: neither may overwrite the file or pass for a first
			// sight or for ok.
			for _, snap := range []machineconfig.Snap{before, after} {
				fake.set(snap, nil)
				svc.Tick(ctx)
				d := detail(t, svc)
				if d.State != configdrift.StateBaselineDamaged {
					t.Fatalf("state is %q (%s), want baseline-damaged", d.State, d.Sentence)
				}
				if !strings.Contains(d.Sentence, "config-baselines/m1.json") {
					t.Errorf("the sentence %q does not name the file", d.Sentence)
				}
				now, err := os.ReadFile(n.baselinePath())
				if err != nil {
					t.Fatalf("read the baseline file: %v", err)
				}
				if !bytes.Equal(now, damaged) {
					t.Fatalf("the damaged baseline was rewritten:\n before %s\n after  %s", damaged, now)
				}
			}
		})
	}
}

// TestAnUnreadableConfigurationStoresNothing: a configuration this build cannot
// load is "unreadable", and no file comes of it.
func TestAnUnreadableConfigurationStoresNothing(t *testing.T) {
	t.Parallel()

	n := newNode(t)
	fake := &fakeReader{}
	fake.set(machineconfig.Snap{}, machineconfig.ErrSnapshotUnreadable)
	svc := n.service(t, withRead(fake.Read))

	svc.Tick(context.Background())

	d := detail(t, svc)
	if d.State != configdrift.StateUnreadable {
		t.Fatalf("state is %q, want unreadable", d.State)
	}
	if !strings.Contains(d.Sentence, "cannot load the node's configuration") {
		t.Errorf("the sentence %q does not say that this version cannot load the configuration", d.Sentence)
	}
	if _, err := os.Stat(n.baselinePath()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a file was written for a configuration nobody could load: %v", err)
	}
}

// TestAPrivateKeyInASnapshotIsNeverWritten is the belt to Redact's braces.
func TestAPrivateKeyInASnapshotIsNeverWritten(t *testing.T) {
	t.Parallel()

	text := "version: v1alpha1\nkey: |\n  -----BEGIN PRIVATE KEY-----\n  MC4CAQAwBQYDK2VwBCIEIInventedInventedInventedInventedInvented\n  -----END PRIVATE KEY-----\n"
	if !machineconfig.ContainsPrivateKey([]byte(text)) {
		t.Fatal("precondition: the planted text must hold a PEM private key")
	}
	bad := machineconfig.Snap{Text: text, Digest: digestOf(text)}

	t.Run("without a baseline", func(t *testing.T) {
		t.Parallel()
		n := newNode(t)
		fake := &fakeReader{}
		fake.set(bad, nil)
		svc := n.service(t, withRead(fake.Read))

		svc.Tick(context.Background())

		if d := detail(t, svc); d.State != configdrift.StateUnreadable {
			t.Errorf("state is %q, want unreadable", d.State)
		}
		if _, err := os.Stat(n.baselinePath()); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("a snapshot with a private key was written: %v", err)
		}
	})

	t.Run("beside a baseline", func(t *testing.T) {
		t.Parallel()
		n := newNode(t)
		fake := &fakeReader{}
		svc := n.service(t, withRead(fake.Read))
		good, _ := twoSnaps(t, n)
		fake.set(good, nil)
		svc.Tick(context.Background())
		before, err := os.ReadFile(n.baselinePath())
		if err != nil {
			t.Fatalf("precondition: no baseline: %v", err)
		}

		fake.set(bad, nil)
		svc.Tick(context.Background())

		if d := detail(t, svc); d.State != configdrift.StateUnreadable {
			t.Errorf("state is %q, want unreadable", d.State)
		}
		after, err := os.ReadFile(n.baselinePath())
		if err != nil {
			t.Fatalf("read the baseline file: %v", err)
		}
		if !bytes.Equal(before, after) {
			t.Error("the baseline changed when a snapshot with a private key came in")
		}
		if strings.Contains(string(after), "BEGIN PRIVATE KEY") {
			t.Error("the baseline file holds a private key")
		}
	})
}

// TestAnEncodingChangeIsNotDrift: the same configuration in another encoding
// (documents in another order) is the baseline, and the stored text is brought
// to the running machinery's form once.
func TestAnEncodingChangeIsNotDrift(t *testing.T) {
	t.Parallel()

	n := newNode(t)
	fake := &fakeReader{}
	svc := n.service(t, withRead(fake.Read))
	snap, _ := twoSnaps(t, n)
	ctx := context.Background()

	parts := splitDocuments(snap.Text)
	if len(parts) < 2 {
		t.Fatal("precondition: the configuration must have more than one document")
	}
	slices.Reverse(parts)
	reversed := strings.Join(parts, "---\n")
	if reversed == snap.Text {
		t.Fatal("precondition: the encoding must differ from the normal form")
	}
	if again := snapOf(t, []byte(reversed)); again.Digest != snap.Digest || again.Text != snap.Text {
		t.Fatal("precondition: normalising the other encoding must give the normal form")
	}
	if _, err := n.st.ConfigBaselines().Put(ctx, model.ConfigBaseline{
		Machine: "m1", Format: model.BaselineFormat, Cluster: "c1",
		Snapshot: reversed, Digest: digestOf(reversed),
		TakenAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), Source: model.BaselineFirstSight, Machinery: "v1.0.0",
	}); err != nil {
		t.Fatalf("plant the baseline: %v", err)
	}

	fake.set(snap, nil)
	svc.Tick(ctx)

	if d := detail(t, svc); d.State != configdrift.StateOK {
		t.Fatalf("state is %q (%s), want ok: another encoding is not a difference", d.State, d.Sentence)
	}
	rec, err := n.st.ConfigBaselines().Get(ctx, "m1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rec.Snapshot != snap.Text || rec.Digest != snap.Digest || rec.Rev != 2 || rec.Machinery == "v1.0.0" {
		t.Errorf("the baseline is rev %d machinery %q after the pass; want the normal form, rev 2 and the running machinery",
			rec.Rev, rec.Machinery)
	}

	svc.Tick(ctx)
	if again, _ := n.st.ConfigBaselines().Get(ctx, "m1"); again.Rev != 2 {
		t.Errorf("the second pass wrote the baseline again: rev %d", again.Rev)
	}
}

// TestNoSecretReachesTheBaselineFile reads from a real node through the real
// reader and then the file itself: the baseline is one more exit of Redact.
func TestNoSecretReachesTheBaselineFile(t *testing.T) {
	t.Parallel()

	n := newNode(t)
	secrets := n.secrets(t)
	svc := n.service(t)

	svc.Tick(context.Background())

	if d := detail(t, svc); d.State != configdrift.StateBaselineCreated {
		t.Fatalf("state is %q (%s), want baseline-created", d.State, d.Sentence)
	}
	file, err := os.ReadFile(n.baselinePath())
	if err != nil {
		t.Fatalf("read the baseline file: %v", err)
	}
	// The file is JSON, which escapes angle brackets and newlines: a secret is
	// looked for in the file as it lies on the disk and in the snapshot text
	// decoded from it, where a PEM block is plain again.
	var rec model.ConfigBaseline
	if err := json.Unmarshal(file, &rec); err != nil {
		t.Fatalf("the baseline file is not a baseline: %v", err)
	}
	if !strings.Contains(rec.Snapshot, machineconfig.RedactedMarker) {
		t.Error("the stored snapshot carries no redaction marker, so it was not made from Redact")
	}
	for name, secret := range secrets {
		for _, form := range forms(secret) {
			escaped, err := json.Marshal(form)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			for where, haystack := range map[string]string{
				"the file":     string(file),
				"the snapshot": rec.Snapshot,
			} {
				if strings.Contains(haystack, form) || strings.Contains(haystack, strings.Trim(string(escaped), `"`)) {
					t.Errorf("%s holds %s", where, name)
				}
			}
		}
	}
	if machineconfig.ContainsPrivateKey(file) || machineconfig.ContainsPrivateKey([]byte(rec.Snapshot)) {
		t.Error("the baseline holds a PEM private key")
	}
}

// TestTheStartOfADriftSurvivesARestart: a banner that counts from the last
// restart never fires.
func TestTheStartOfADriftSurvivesARestart(t *testing.T) {
	t.Parallel()

	n := newNode(t)
	clk := newClock()
	fake := &fakeReader{}
	before, after := twoSnaps(t, n)
	ctx := context.Background()

	svc := n.service(t, withRead(fake.Read), withClock(clk))
	fake.set(before, nil)
	svc.Tick(ctx)
	clk.Advance(time.Hour)
	fake.set(after, nil)
	svc.Tick(ctx)
	start := clk.Now()
	if d := detail(t, svc); !d.DriftSince.Equal(start) {
		t.Fatalf("precondition: DriftSince is %v, want %v", d.DriftSince, start)
	}

	clk.Advance(30 * time.Hour)
	n.reopen(t)
	restarted := n.service(t, withRead(fake.Read), withClock(clk))
	if d := detail(t, restarted); d.State != configdrift.StateNotRead {
		t.Fatalf("precondition: a new process says %q before its first reading, want not-read", d.State)
	}
	restarted.Tick(ctx)

	d := detail(t, restarted)
	if d.State != configdrift.StateDrift {
		t.Fatalf("state after the restart is %q, want drift", d.State)
	}
	if !d.DriftSince.Equal(start) {
		t.Errorf("DriftSince is %v after a restart, want the original %v", d.DriftSince, start)
	}
	rec, err := n.st.ConfigBaselines().Get(ctx, "m1")
	if err != nil || !rec.DriftSince.Equal(start) {
		t.Errorf("the record holds DriftSince %v (%v), want %v", rec.DriftSince, err, start)
	}
}

// TestAnOutageIsLoggedOnTheTransitionOnly: a node that is off for three hours
// must not write a line every pass.
func TestAnOutageIsLoggedOnTheTransitionOnly(t *testing.T) {
	t.Parallel()

	n := newNode(t)
	sink := &logSink{}
	fake := &fakeReader{}
	svc := n.service(t, withRead(fake.Read), withLog(sink))
	good, _ := twoSnaps(t, n)
	ctx := context.Background()

	fake.set(good, nil)
	svc.Tick(ctx)
	svc.Tick(ctx)
	if got := len(sink.Lines()); got > 1 {
		t.Fatalf("two good passes wrote %d lines, want at most the baseline's: %q", got, sink.Lines())
	}

	fake.set(machineconfig.Snap{}, errors.New("i/o timeout"))
	for range 3 {
		svc.Tick(ctx)
	}
	count := func(word string) int {
		c := 0
		for _, l := range sink.Lines() {
			if strings.Contains(l, "state="+word) {
				c++
			}
		}
		return c
	}
	if got := count("unreadable"); got != 1 {
		t.Fatalf("three failed passes wrote %d unreadable lines, want 1: %q", got, sink.Lines())
	}

	fake.set(good, nil)
	svc.Tick(ctx)
	svc.Tick(ctx)
	if got := count("ok"); got != 1 {
		t.Errorf("the return wrote %d ok lines, want 1: %q", got, sink.Lines())
	}
	for _, l := range sink.Lines() {
		if strings.Contains(l, "i/o timeout") {
			t.Errorf("a line carries an error text, which may name an address or a piece of a document: %q", l)
		}
	}
}

// TestSwitchedOffNodesAndClustersAreNeverRead: nothing waits out a deadline
// for a node that is off on purpose.
func TestSwitchedOffNodesAndClustersAreNeverRead(t *testing.T) {
	t.Parallel()

	t.Run("a disabled node", func(t *testing.T) {
		t.Parallel()
		n := newNode(t)
		fake := &fakeReader{}
		off := n.machine
		off.Disabled = true
		svc := n.service(t, withRead(fake.Read), withMachine(off))
		svc.Tick(context.Background())
		if fake.calls != 0 {
			t.Errorf("a disabled node was read %d times", fake.calls)
		}
		if d := detail(t, svc); d.State != configdrift.StateNotRead {
			t.Errorf("a disabled node never read is %q, want not-read", d.State)
		}
	})
	t.Run("a disabled cluster", func(t *testing.T) {
		t.Parallel()
		n := newNode(t)
		fake := &fakeReader{}
		svc := n.service(t, withRead(fake.Read), withCluster(model.Cluster{ID: "c1", Name: "homelab", Disabled: true}))
		svc.Tick(context.Background())
		if fake.calls != 0 {
			t.Errorf("a node of a disabled cluster was read %d times", fake.calls)
		}
	})
	t.Run("an enabled node is read", func(t *testing.T) {
		t.Parallel()
		n := newNode(t)
		fake := &fakeReader{}
		good, _ := twoSnaps(t, n)
		fake.set(good, nil)
		svc := n.service(t, withRead(fake.Read))
		svc.Tick(context.Background())
		if fake.calls != 1 {
			t.Errorf("an enabled node was read %d times, want 1 (the two checks above prove nothing otherwise)", fake.calls)
		}
	})
}

// TestAVanishedNodeLosesItsReading: a node that is gone from the cluster's list
// is no longer reported with a reading from the time it was there. Its record
// stays; clearing up after vanished machines is not this package's job.
func TestAVanishedNodeLosesItsReading(t *testing.T) {
	t.Parallel()

	n := newNode(t)
	fake := &fakeReader{}
	good, _ := twoSnaps(t, n)
	fake.set(good, nil)
	listed := []model.Machine{n.machine}
	svc := configdrift.New(configdrift.Deps{
		Clusters: func(context.Context) ([]model.Cluster, error) {
			return []model.Cluster{{ID: "c1", Name: "homelab"}}, nil
		},
		Machines:    func(context.Context, model.ClusterID) ([]model.Machine, error) { return listed, nil },
		AllMachines: func(context.Context) ([]model.Machine, error) { return []model.Machine{n.machine}, nil },
		Read:        fake.Read,
		Baselines:   n.st.ConfigBaselines(),
	})
	ctx := context.Background()

	svc.Tick(ctx)
	if d := detail(t, svc); d.State != configdrift.StateBaselineCreated {
		t.Fatalf("precondition: state is %q, want baseline-created", d.State)
	}

	listed = nil
	svc.Tick(ctx)

	if d := detail(t, svc); d.State != configdrift.StateNotRead {
		t.Errorf("a node that left the list is still %q", d.State)
	}
	if _, err := n.st.ConfigBaselines().Get(ctx, "m1"); err != nil {
		t.Errorf("the record of a vanished node was removed here: %v", err)
	}
}
