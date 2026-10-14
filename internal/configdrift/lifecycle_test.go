package configdrift_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/configdrift"
	"github.com/holzcloud/holzkube-manager/internal/machineconfig"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// inventory is the lists the service is given, which a test changes under it.
type inventory struct {
	mu       sync.Mutex
	machines []model.Machine
	clusters []model.Cluster
	allErr   error
}

func (i *inventory) all(context.Context) ([]model.Machine, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.allErr != nil {
		return nil, i.allErr
	}
	return slices.Clone(i.machines), nil
}

func (i *inventory) ofCluster(_ context.Context, id model.ClusterID) ([]model.Machine, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	var out []model.Machine
	for _, m := range i.machines {
		if m.Cluster == id {
			out = append(out, m)
		}
	}
	return out, nil
}

func (i *inventory) list(context.Context) ([]model.Cluster, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	return slices.Clone(i.clusters), nil
}

func (i *inventory) set(machines ...model.Machine) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.machines = machines
}

func (i *inventory) failAll(err error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.allErr = err
}

// withInventory gives the service the three lists of one inventory.
func withInventory(i *inventory) serviceOpt {
	return func(o *serviceOpts) {
		o.allMachines, o.machines, o.clusters = i.all, i.ofCluster, i.list
	}
}

// oneMachineInventory is the node of newNode, alone, in an enabled cluster c1.
func oneMachineInventory(n *node) *inventory {
	return &inventory{
		machines: []model.Machine{n.machine},
		clusters: []model.Cluster{{ID: "c1", Name: "homelab"}},
	}
}

// putBaseline lays a baseline record for a machine straight into the store, as
// an earlier run of the daemon would have left it.
func putBaseline(t *testing.T, n *node, id model.MachineID, cluster model.ClusterID, snap machineconfig.Snap) {
	t.Helper()
	_, err := n.st.ConfigBaselines().Put(context.Background(), model.ConfigBaseline{
		Machine: id, Format: model.BaselineFormat, Cluster: cluster,
		Snapshot: snap.Text, Digest: snap.Digest,
		TakenAt: time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC),
		Source:  model.BaselineFirstSight, Machinery: "v1.14.0",
	})
	if err != nil {
		t.Fatalf("lay the baseline of %s: %v", id, err)
	}
}

// baselineFile is the path of any machine's baseline file.
func baselineFile(n *node, id model.MachineID) string {
	return filepath.Join(n.dir, "config-baselines", string(id)+".json")
}

func mustExist(t *testing.T, path, why string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("%s: %v", why, err)
	}
}

func mustBeGone(t *testing.T, path, why string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("%s (stat: %v)", why, err)
	}
}

// TestForgetDropsTheBaselineAndTheReading: the baseline file and the reading in
// memory are both gone, forgetting again is no error, and the next reading lays
// a new baseline out of whatever the node says then.
func TestForgetDropsTheBaselineAndTheReading(t *testing.T) {
	t.Parallel()

	d := newDrifting(t)
	ctx := context.Background()
	mustExist(t, d.n.baselinePath(), "precondition: the baseline file")
	if got := detail(t, d.svc); got.State != configdrift.StateDrift {
		t.Fatalf("precondition: state is %q, want drift", got.State)
	}

	d.svc.Forget(ctx, "m1")

	mustBeGone(t, d.n.baselinePath(), "Forget left the baseline file")
	got := detail(t, d.svc)
	if got.State != configdrift.StateNotRead {
		t.Errorf("after Forget Detail says %q (%s), want not-read: the reading was left in memory", got.State, got.Sentence)
	}
	if got.Diff != nil || !got.DriftSince.IsZero() || got.Digest != "" {
		t.Errorf("after Forget Detail still carries the reading: %+v", got)
	}

	d.svc.Forget(ctx, "m1") // nothing left to forget: no error, no panic

	d.svc.Tick(ctx)
	got = detail(t, d.svc)
	if got.State != configdrift.StateBaselineCreated {
		t.Fatalf("the reading after Forget gave %q (%s), want baseline-created", got.State, got.Sentence)
	}
	rec, err := d.n.st.ConfigBaselines().Get(ctx, "m1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rec.Rev != 1 || rec.Digest != d.after.Digest || rec.Source != model.BaselineFirstSight {
		t.Errorf("the new baseline is rev %d, %q, %q, want a new record from what the node says now", rec.Rev, rec.Digest, rec.Source)
	}
}

// TestABaselineOfAVanishedMachineIsRemoved: a machine that is no longer in the
// inventory leaves nothing behind that a later machine could be compared with.
func TestABaselineOfAVanishedMachineIsRemoved(t *testing.T) {
	t.Parallel()

	n := newNode(t)
	fake := &fakeReader{}
	before, _ := twoSnaps(t, n)
	fake.set(before, nil)
	inv := oneMachineInventory(n)
	svc := n.service(t, withRead(fake.Read), withInventory(inv))
	putBaseline(t, n, "gone", "c1", before)
	mustExist(t, baselineFile(n, "gone"), "precondition: the baseline of the machine that is not in the inventory")

	svc.Tick(context.Background())

	mustBeGone(t, baselineFile(n, "gone"), "the baseline of a machine that is not in the inventory is still there")
	mustExist(t, n.baselinePath(), "the baseline of the machine that is in the inventory was removed with it")
}

// TestABaselineOfAMachineInAnotherClusterIsRemoved: a machine that moved to
// another cluster is another machine as far as a baseline goes, and the node is
// not compared with what it was in the cluster it left.
func TestABaselineOfAMachineInAnotherClusterIsRemoved(t *testing.T) {
	t.Parallel()

	t.Run("nothing reads it again", func(t *testing.T) {
		t.Parallel()
		n := newNode(t)
		fake := &fakeReader{}
		before, _ := twoSnaps(t, n)
		fake.set(before, nil)
		inv := oneMachineInventory(n)
		svc := n.service(t, withRead(fake.Read), withInventory(inv))
		svc.Tick(context.Background())
		mustExist(t, n.baselinePath(), "precondition: the baseline laid by the first reading")

		moved := n.machine
		moved.Cluster = "c2" // a cluster that is not listed, so nothing reads the node again
		inv.set(moved)
		svc.Tick(context.Background())

		mustBeGone(t, n.baselinePath(), "the baseline of a machine that moved to another cluster is still there")
	})

	t.Run("it is read in the new cluster as a first sight", func(t *testing.T) {
		t.Parallel()
		n := newNode(t)
		fake := &fakeReader{}
		before, after := twoSnaps(t, n)
		fake.set(before, nil)
		inv := oneMachineInventory(n)
		svc := n.service(t, withRead(fake.Read), withInventory(inv))
		svc.Tick(context.Background())
		mustExist(t, n.baselinePath(), "precondition: the baseline laid by the first reading")

		// It moves, and what it says differs from what it said in c1.
		moved := n.machine
		moved.Cluster = "c2"
		inv.set(moved)
		inv.mu.Lock()
		inv.clusters = []model.Cluster{{ID: "c2", Name: "other"}}
		inv.mu.Unlock()
		fake.set(after, nil)
		svc.Tick(context.Background())

		got := detail(t, svc)
		if got.State != configdrift.StateBaselineCreated {
			t.Fatalf("a machine that moved was compared with its old baseline: state %q (%s), want baseline-created", got.State, got.Sentence)
		}
		rec, err := n.st.ConfigBaselines().Get(context.Background(), "m1")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if rec.Cluster != "c2" || rec.Digest != after.Digest || rec.Rev != 1 {
			t.Errorf("the baseline is cluster %q digest %q rev %d, want a new one for c2 from the new reading", rec.Cluster, rec.Digest, rec.Rev)
		}
	})
}

// TestAKnownMachinesBaselineIsKept: the same machine in the same cluster keeps
// its record, byte for byte, in the same run that removes another's.
func TestAKnownMachinesBaselineIsKept(t *testing.T) {
	t.Parallel()

	n := newNode(t)
	fake := &fakeReader{}
	before, _ := twoSnaps(t, n)
	fake.set(before, nil)
	svc := n.service(t, withRead(fake.Read), withInventory(oneMachineInventory(n)))
	putBaseline(t, n, "m1", "c1", before)
	putBaseline(t, n, "gone", "c1", before)
	kept, err := os.ReadFile(n.baselinePath())
	if err != nil {
		t.Fatalf("precondition: the baseline of the known machine: %v", err)
	}

	svc.Tick(context.Background())

	mustBeGone(t, baselineFile(n, "gone"), "precondition of the control: the run removed no baseline at all")
	after, err := os.ReadFile(n.baselinePath())
	if err != nil {
		t.Fatalf("the baseline of a known machine in its own cluster was removed: %v", err)
	}
	if !bytes.Equal(kept, after) {
		t.Errorf("the baseline of a known machine changed:\n before %s\n after  %s", kept, after)
	}
}

// TestReconcileNeverDeletesWhenTheMachineListFails: "the list could not be
// read" is not "there are no machines". Nothing is deleted, and a line says why.
func TestReconcileNeverDeletesWhenTheMachineListFails(t *testing.T) {
	t.Parallel()

	n := newNode(t)
	fake := &fakeReader{}
	before, _ := twoSnaps(t, n)
	fake.set(before, nil)
	inv := oneMachineInventory(n)
	sink := &logSink{}
	svc := n.service(t, withRead(fake.Read), withInventory(inv), withLog(sink))
	putBaseline(t, n, "m1", "c1", before)
	putBaseline(t, n, "gone", "c1", before)
	gone, err := os.ReadFile(baselineFile(n, "gone"))
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}
	inv.failAll(errors.New("inventory unavailable"))

	svc.Tick(context.Background())

	after, err := os.ReadFile(baselineFile(n, "gone"))
	if err != nil {
		t.Fatalf("a baseline was deleted while the machine list could not be read: %v", err)
	}
	if !bytes.Equal(gone, after) {
		t.Error("a baseline changed while the machine list could not be read")
	}
	mustExist(t, n.baselinePath(), "a baseline was deleted while the machine list could not be read")
	if !slices.ContainsFunc(sink.Lines(), func(l string) bool { return strings.Contains(l, "machines") && strings.Contains(l, "clear") }) {
		t.Errorf("no line says why nothing was cleared up: %q", sink.Lines())
	}
}

// TestReconcileNeverDeletesWhenTheBaselinesCannotBeListed: one damaged file
// makes the store's List fail, and what it returned before failing is a part of
// the list and not the list. Nothing is deleted, the damaged file is untouched,
// and a line says so. The limit this sets is stated in the guide: a damaged
// file stops the clearing up until it is removed.
func TestReconcileNeverDeletesWhenTheBaselinesCannotBeListed(t *testing.T) {
	t.Parallel()

	n := newNode(t)
	fake := &fakeReader{}
	before, _ := twoSnaps(t, n)
	fake.set(before, nil)
	sink := &logSink{}
	svc := n.service(t, withRead(fake.Read), withInventory(oneMachineInventory(n)), withLog(sink))

	// "gone" sorts before "zz", so it is in the part of the list that was read
	// before the damaged file stopped it.
	putBaseline(t, n, "gone", "c1", before)
	damaged := []byte("{this is not a baseline")
	if err := os.WriteFile(baselineFile(n, "zz"), damaged, 0o600); err != nil {
		t.Fatalf("damage a baseline: %v", err)
	}
	if _, err := n.st.ConfigBaselines().List(context.Background()); err == nil {
		t.Fatal("precondition: listing the baselines must fail with a damaged file in the directory")
	}

	svc.Tick(context.Background())

	mustExist(t, baselineFile(n, "gone"), "a baseline of a vanished machine was deleted on a partial list")
	got, err := os.ReadFile(baselineFile(n, "zz"))
	if err != nil || !bytes.Equal(got, damaged) {
		t.Errorf("the damaged file was touched: %q, %v", got, err)
	}
	if !slices.ContainsFunc(sink.Lines(), func(l string) bool { return strings.Contains(l, "baselines") && strings.Contains(l, "clear") }) {
		t.Errorf("no line says why nothing was cleared up: %q", sink.Lines())
	}
}

// TestADisabledNodeIsNeverRead: a node that is off on purpose is not read, not
// by a pass and not by a poke, and a node beside it in the same cluster is.
func TestADisabledNodeIsNeverRead(t *testing.T) {
	t.Parallel()

	n := newNode(t)
	fake := &fakeReader{}
	before, _ := twoSnaps(t, n)
	fake.set(before, nil)
	off := model.Machine{ID: "m2", Cluster: "c1", Hostname: "cp-2", Role: model.RoleControlPlane, Disabled: true}
	inv := &inventory{
		machines: []model.Machine{n.machine, off},
		clusters: []model.Cluster{{ID: "c1", Name: "homelab"}},
	}
	svc := n.service(t, withRead(fake.Read), withInventory(inv))
	ctx := context.Background()

	svc.Tick(ctx)
	svc.Poke("m2")
	svc.Flush(ctx)

	if got := fake.read(); !slices.Equal(got, []model.MachineID{"m1"}) {
		t.Errorf("the reader was called for %v, want only m1 (m2 is switched off)", got)
	}
	if got, err := svc.Detail(ctx, "m2"); err != nil || got.State != configdrift.StateNotRead {
		t.Errorf("a disabled node says %q (%v), want not-read", got.State, err)
	}
}

// TestADisabledClusterIsNeverRead: every node of a switched-off cluster is
// passed over, and the reader is not called once for it.
func TestADisabledClusterIsNeverRead(t *testing.T) {
	t.Parallel()

	n := newNode(t)
	fake := &fakeReader{}
	before, _ := twoSnaps(t, n)
	fake.set(before, nil)
	other := model.Machine{ID: "m2", Cluster: "c2", Hostname: "cp-9", Role: model.RoleControlPlane}
	inv := &inventory{
		machines: []model.Machine{n.machine, other},
		clusters: []model.Cluster{{ID: "c1", Name: "homelab"}, {ID: "c2", Name: "off", Disabled: true}},
	}
	svc := n.service(t, withRead(fake.Read), withInventory(inv))
	ctx := context.Background()

	svc.Tick(ctx)
	svc.Poke("m2")
	svc.Flush(ctx)

	if got := fake.read(); !slices.Equal(got, []model.MachineID{"m1"}) {
		t.Errorf("the reader was called for %v, want only m1 (cluster c2 is switched off)", got)
	}
	if got, err := svc.Detail(ctx, "m2"); err != nil || got.State != configdrift.StateNotRead {
		t.Errorf("a node of a disabled cluster says %q (%v), want not-read", got.State, err)
	}
}

// TestPokeNeverBlocks: nothing runs the service here, the buffer is far smaller
// than the number of pokes, and the caller still comes back.
func TestPokeNeverBlocks(t *testing.T) {
	t.Parallel()

	n := newNode(t)
	svc := n.service(t)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 1000 {
			svc.Poke("m1")
			svc.Poke("no-such-machine")
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Poke blocked: 2000 pokes with nothing reading them did not return within two seconds")
	}
}

// runInBackground starts Run on a context that has no deadline and returns
// the function that stops it and waits for it.
func runInBackground(svc *configdrift.Service, firstAfter time.Duration) (stop func(t *testing.T)) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.Run(ctx, firstAfter)
	}()
	return func(t *testing.T) {
		t.Helper()
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("Run did not return after its context ended")
		}
	}
}

// waitFor polls until cond holds or the time is up.
func waitFor(limit time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}

// TestAPokedNodeIsReadWithoutWaitingForTheTick: the first tick is an hour away
// and the node is read within seconds of the poke, once for two pokes. The
// reader records the deadline of the context it was handed, and Run's context
// has none: the way from Poke through Flush to the read has to put the budget
// on itself (the same fault as ledger 137, on the second way to the reader).
func TestAPokedNodeIsReadWithoutWaitingForTheTick(t *testing.T) {
	t.Parallel()

	n := newNode(t)
	fake := &fakeReader{}
	before, _ := twoSnaps(t, n)
	fake.set(before, nil)
	svc := n.service(t, withRead(fake.Read))

	// Both pokes are in before Run is: two pokes for one node are one reading.
	svc.Poke("m1")
	svc.Poke("m1")
	stop := runInBackground(svc, time.Hour)
	defer stop(t)

	ok := waitFor(5*time.Second, func() bool {
		d, err := svc.Detail(context.Background(), "m1")
		return err == nil && d.State != configdrift.StateNotRead
	})
	if !ok {
		t.Fatal("a poked node was not read within five seconds, with the first tick an hour away")
	}
	time.Sleep(200 * time.Millisecond) // a second reading of the same poke would show now
	if got := fake.count(); got != 1 {
		t.Errorf("the reader was called %d times for two pokes before a reading, want 1", got)
	}
	had, remaining := fake.limit()
	if !had {
		t.Fatal("the reader was handed a context without a deadline on the way Poke, Flush, read; the Talos client refuses such a call")
	}
	if remaining <= 0 || remaining > configdrift.DefaultReadBudget {
		t.Errorf("the deadline is %v away, want within (0, %v]", remaining, configdrift.DefaultReadBudget)
	}
	if d := detail(t, svc); d.State != configdrift.StateBaselineCreated {
		t.Errorf("the poked node is %q (%s), want baseline-created", d.State, d.Sentence)
	}
}

// TestAPokedNodeReachesARealNodeFromAContextWithoutADeadline is the same way
// through the real client against the simulator, where a missing deadline is a
// refusal and not a recorded fact.
func TestAPokedNodeReachesARealNodeFromAContextWithoutADeadline(t *testing.T) {
	t.Parallel()

	n := newNode(t)
	svc := n.service(t)
	stop := runInBackground(svc, time.Hour)
	defer stop(t)

	svc.Poke("m1")

	var last configdrift.Detail
	ok := waitFor(10*time.Second, func() bool {
		last = detail(t, svc)
		return last.State != configdrift.StateNotRead
	})
	if !ok {
		t.Fatal("a poked node was not read within ten seconds")
	}
	if last.State != configdrift.StateBaselineCreated {
		t.Errorf("a poked node read from Run's context is %q (%s), want baseline-created", last.State, last.Sentence)
	}
}

// TestAPokeForAMachineThatDoesNotExistReadsNothing: unknown, without a cluster
// and switched off are dropped without the reader being called, and a machine
// that is none of those is read, so the three silences prove something.
func TestAPokeForAMachineThatDoesNotExistReadsNothing(t *testing.T) {
	t.Parallel()

	n := newNode(t)
	fake := &fakeReader{}
	before, _ := twoSnaps(t, n)
	fake.set(before, nil)
	unassigned := model.Machine{ID: "m3", Hostname: "spare", Role: model.RoleWorker}
	inv := &inventory{
		machines: []model.Machine{n.machine, unassigned},
		clusters: []model.Cluster{{ID: "c1", Name: "homelab"}},
	}
	svc := n.service(t, withRead(fake.Read), withInventory(inv))
	ctx := context.Background()

	svc.Poke("no-such-machine")
	svc.Poke("m3")
	svc.Flush(ctx)
	if got := fake.read(); len(got) != 0 {
		t.Errorf("the reader was called for %v, want nothing for an unknown machine and one without a cluster", got)
	}

	svc.Poke("m1")
	svc.Flush(ctx)
	if got := fake.read(); !slices.Equal(got, []model.MachineID{"m1"}) {
		t.Errorf("a poke for a machine that exists reached the reader as %v, want [m1]", got)
	}
}
