package history

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
)

// fleet is a fake inventory: which machines exist, which answer, and what the
// cluster's apps are.
type fleet struct {
	mu       sync.Mutex
	machines []model.Machine
	clusters []model.Cluster
	down     map[model.MachineID]bool
	apps     []kube.App
}

func newFleet(ids ...model.MachineID) *fleet {
	f := &fleet{down: map[model.MachineID]bool{}, clusters: []model.Cluster{{ID: "c1"}}}
	for _, id := range ids {
		f.machines = append(f.machines, model.Machine{ID: id, Cluster: "c1"})
	}
	return f
}

func (f *fleet) inventory(context.Context) ([]model.Machine, []model.Cluster, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]model.Machine(nil), f.machines...), append([]model.Cluster(nil), f.clusters...), nil
}

func (f *fleet) hardware(_ context.Context, id model.MachineID) (inventory.HardwareView, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down[id] {
		return inventory.HardwareView{}, errors.New("connection refused")
	}
	return inventory.HardwareView{
		Machine: id,
		CPU:     inventory.HardwareCPU{UsagePercent: 18.2, PerCore: []float64{10, 30}},
		Memory:  inventory.HardwareMemory{TotalBytes: 1000, UsedBytes: 250},
		Network: []inventory.HardwareLink{{RxBytesPerSec: 100, TxBytesPerSec: 7}, {RxBytesPerSec: 50}},
		Disks:   []inventory.HardwareDisk{{ReadBytesPerSec: 4096, WriteBytesPerSec: 1}},
		Temperatures: []inventory.HardwareTemperature{
			{Chip: "coretemp", Label: "Package id 0", Celsius: 57},
		},
		Fans: []inventory.HardwareFan{{Chip: "nct6798", Label: "fan2", RPM: 1080}},
	}, nil
}

func (f *fleet) listApps(context.Context, model.ClusterID) (kube.Apps, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return kube.Apps{Apps: append([]kube.App(nil), f.apps...)}, nil
}

func (f *fleet) set(fn func(f *fleet)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

// clock is a settable time for the sampler.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newTestSampler(f *fleet, st *Store, c *clock, logger *slog.Logger) *Sampler {
	if logger == nil {
		logger = quiet()
	}
	return NewSampler(SamplerDeps{
		History:   st,
		Logger:    logger,
		Now:       c.Now,
		Inventory: f.inventory,
		Hardware:  f.hardware,
		Apps:      f.listApps,
	})
}

// TestTheSamplerRecordsWhatTheGaugesShow is the mapping from one hardware view
// and one apps list to the contract's series keys.
func TestTheSamplerRecordsWhatTheGaugesShow(t *testing.T) {
	f := newFleet("m1")
	f.apps = []kube.App{
		{Namespace: "default", Kind: "Deployment", Name: "web", CPUMillis: 120, MemoryBytes: 64 << 20, UsageKnown: true},
		{Namespace: "default", Kind: "CronJob", Name: "idle"},
	}
	st := NewMemory()
	c := &clock{now: t0}
	newTestSampler(f, st, c, nil).pass(context.Background())

	got := st.Query(MachineSubject("m1"), Range1h, t0).Series
	want := map[string]float64{
		"cpu": 18.2, "memory": 25, "rx": 150, "tx": 7, "read": 4096, "write": 1,
		"core:0": 10, "core:1": 30, "temp:coretemp/Package id 0": 57, "fan:nct6798/fan2": 1080,
	}
	if len(got) != len(want) {
		t.Errorf("series %v, want exactly %v", keys(got), want)
	}
	for k, v := range want {
		if p := got[k]; len(p) != 1 || p[0] != (Point{ms(t0), v}) {
			t.Errorf("%s = %v, want [[%v %v]]", k, p, ms(t0), v)
		}
	}

	web := st.Query(AppSubject("c1", "default", "Deployment", "web"), Range1h, t0).Series
	if len(web["cpu"]) != 1 || web["cpu"][0][1] != 120 || web["memory"][0][1] != float64(64<<20) {
		t.Errorf("web = %v, want cpu 120 and memory %d", web, 64<<20)
	}

	// An app whose usage nobody knows records nothing -- and still exists.
	if st.Has(AppSubject("c1", "default", "CronJob", "idle")) {
		t.Error("an app with unknown usage was recorded")
	}
	if known, certain := st.AppKnown("c1", "default", "CronJob", "idle"); !known || !certain {
		t.Errorf("the listed app reads known=%v certain=%v, want both", known, certain)
	}
	if known, certain := st.AppKnown("c1", "default", "Deployment", "gone"); known || !certain {
		t.Errorf("an unlisted app reads known=%v certain=%v, want unknown and certain", known, certain)
	}
	if _, certain := st.AppKnown("c2", "default", "Deployment", "web"); certain {
		t.Error("an app of a cluster never listed reads as certainly known or unknown")
	}
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestASilentNodeLeavesAGapNotAZero: the second pass finds m2 down, and m2's
// chart has a hole at that moment rather than a point at zero.
//
// Fault injected and seen red: sampleMachine recording the empty view on an
// error (Record(key, at, HardwareValues(inventory.HardwareView{}))) -- a cpu
// point of 0 appeared where the gap belongs.
func TestASilentNodeLeavesAGapNotAZero(t *testing.T) {
	f := newFleet("m1", "m2")
	st := NewMemory()
	c := &clock{now: t0}
	s := newTestSampler(f, st, c, nil)

	s.pass(context.Background())
	f.set(func(f *fleet) { f.down["m2"] = true })
	c.advance(FineStep)
	s.pass(context.Background())

	if got := st.Query(MachineSubject("m1"), Range1h, c.Now()).Series["cpu"]; len(got) != 2 {
		t.Fatalf("the node that answered has %v, want two points", got)
	}
	got := st.Query(MachineSubject("m2"), Range1h, c.Now()).Series["cpu"]
	if len(got) != 1 || got[0][0] != ms(t0) {
		t.Errorf("the node that went silent has %v, want only its first point: a gap, never a zero", got)
	}
}

// TestAFailingNodeIsLoggedOnce: a powered-off node is the ordinary state of a
// homelab, and one warning a day about it is information where one every
// fifteen seconds is noise.
//
// Fault injected and seen red: failed() logging every time (first := true) --
// three warnings for three passes.
func TestAFailingNodeIsLoggedOnce(t *testing.T) {
	f := newFleet("m1")
	f.down["m1"] = true
	rec := &recorder{}
	st := NewMemory()
	c := &clock{now: t0}
	s := newTestSampler(f, st, c, slog.New(rec))

	for range 3 {
		s.pass(context.Background())
		c.advance(FineStep)
	}
	f.set(func(f *fleet) { f.down["m1"] = false })
	s.pass(context.Background())
	s.pass(context.Background())

	if w, i := rec.count(slog.LevelWarn), rec.count(slog.LevelInfo); w != 1 || i != 1 {
		t.Errorf("three failing passes and two good ones logged %d warnings and %d infos, want 1 and 1", w, i)
	}
}

type recorder struct {
	mu     sync.Mutex
	levels []slog.Level
}

func (r *recorder) Enabled(context.Context, slog.Level) bool { return true }
func (r *recorder) Handle(_ context.Context, rec slog.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.levels = append(r.levels, rec.Level)
	return nil
}
func (r *recorder) WithAttrs([]slog.Attr) slog.Handler { return r }
func (r *recorder) WithGroup(string) slog.Handler      { return r }
func (r *recorder) count(l slog.Level) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, x := range r.levels {
		if x == l {
			n++
		}
	}
	return n
}

// TestAForgottenMachineLosesItsHistory: forgetting is the operator saying it is
// gone, and the next pass deletes its history -- from memory, and from the file
// at the next write. A forgotten cluster takes its apps' history with it.
//
// Fault injected and seen red: the Retain call removed from pass -- m2's
// history stayed in memory and in the rewritten file.
func TestAForgottenMachineLosesItsHistory(t *testing.T) {
	path := historyPath(t)
	f := newFleet("m1", "m2")
	f.apps = []kube.App{{Namespace: "default", Kind: "Deployment", Name: "web", CPUMillis: 1, UsageKnown: true}}
	c := &clock{now: t0}
	st := Open(path, fsstore.ReadFile, fsstore.WriteFileAtomic, t0, quiet())
	s := newTestSampler(f, st, c, nil)

	s.pass(context.Background())
	if !st.Has(MachineSubject("m2")) || !st.Has(AppSubject("c1", "default", "Deployment", "web")) {
		t.Fatal("the first pass recorded nothing to forget")
	}

	f.set(func(f *fleet) { f.machines = f.machines[:1] })
	c.advance(FineStep)
	s.pass(context.Background())
	if st.Has(MachineSubject("m2")) {
		t.Error("a forgotten machine's history is still held")
	}
	if !st.Has(MachineSubject("m1")) {
		t.Error("the machine that was not forgotten lost its history")
	}
	if err := st.Flush(c.Now()); err != nil {
		t.Fatal(err)
	}
	if Open(path, fsstore.ReadFile, fsstore.WriteFileAtomic, c.Now(), quiet()).Has(MachineSubject("m2")) {
		t.Error("a forgotten machine's history is still in the file")
	}

	// And a forgotten cluster: its apps go, and its machines -- still machines,
	// now in no cluster -- keep theirs until it ages out.
	f.set(func(f *fleet) {
		f.clusters = nil
		f.machines[0].Cluster = ""
	})
	c.advance(FineStep)
	s.pass(context.Background())
	if st.Has(AppSubject("c1", "default", "Deployment", "web")) {
		t.Error("a forgotten cluster's app history is still held")
	}
	if !st.Has(MachineSubject("m1")) {
		t.Error("a machine left in no cluster lost its history with the cluster")
	}
}

// TestPassesNeverOverlap: a pass slower than the interval delays the next one;
// it never runs beside it.
//
// Fault injected and seen red: Run starting each pass on its own goroutine
// (go s.pass(ctx)) -- the one node was being read by several passes at once.
func TestPassesNeverOverlap(t *testing.T) {
	var inFlight, most, calls atomic.Int32
	st := NewMemory()
	s := NewSampler(SamplerDeps{
		History:  st,
		Logger:   quiet(),
		Interval: 2 * time.Millisecond,
		Inventory: func(context.Context) ([]model.Machine, []model.Cluster, error) {
			return []model.Machine{{ID: "m1", Cluster: "c1"}}, nil, nil
		},
		Hardware: func(ctx context.Context, _ model.MachineID) (inventory.HardwareView, error) {
			n := inFlight.Add(1)
			defer inFlight.Add(-1)
			for {
				m := most.Load()
				if n <= m || most.CompareAndSwap(m, n) {
					break
				}
			}
			calls.Add(1)
			select {
			case <-time.After(20 * time.Millisecond):
			case <-ctx.Done():
			}
			return inventory.HardwareView{}, nil
		},
		Apps: func(context.Context, model.ClusterID) (kube.Apps, error) { return kube.Apps{}, nil },
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Run(ctx)
	}()
	time.Sleep(150 * time.Millisecond)
	cancel()
	<-done

	if calls.Load() < 3 {
		t.Fatalf("only %d passes ran; the measurement needs several", calls.Load())
	}
	if most.Load() != 1 {
		t.Errorf("%d passes read the node at the same time; passes must never overlap", most.Load())
	}
}

// TestShutdownWritesTheLastMinute is the restart half of the round trip: the
// hourly update stops the daemon well inside a minute of its last write, and
// what the sampler had recorded since is written on the way out.
//
// Fault injected and seen red: the Flush in Run's shutdown branch removed --
// the reopened history was empty.
func TestShutdownWritesTheLastMinute(t *testing.T) {
	path := historyPath(t)
	f := newFleet("m1")
	c := &clock{now: t0}
	st := Open(path, fsstore.ReadFile, fsstore.WriteFileAtomic, t0, quiet())
	s := newTestSampler(f, st, c, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Run(ctx)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for !st.Has(MachineSubject("m1")) {
		if time.Now().After(deadline) {
			t.Fatal("the first pass never recorded")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done

	if st.Writes() != 1 {
		t.Errorf("the file was written %d times, want once, on the way out", st.Writes())
	}
	reopened := Open(path, fsstore.ReadFile, fsstore.WriteFileAtomic, t0, quiet())
	if got := reopened.Query(MachineSubject("m1"), Range1h, t0).Series["cpu"]; len(got) != 1 {
		t.Errorf("after a restart the node's hour is %v, want the one pass before it", got)
	}
}
