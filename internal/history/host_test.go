package history

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
)

// The host's history (2026-09-28): the machine this daemon runs on, sampled in
// the same pass as the nodes and filed under host/local. These hold the four
// ways it could quietly go wrong -- deleted because it is not in the inventory,
// stamped with a different instant, skipped whenever the inventory fails, and
// a failure recorded as a zero or logged every fifteen seconds.

// hostReading is what a Pi's host reader answers: its CPU sensor and a fan.
func hostReading() map[string]float64 {
	return map[string]float64{"temp:cpu_thermal/temp1": 51.5, "fan:pwmfan/fan1": 2100}
}

// hostSampler is newTestSampler with a host reader.
func hostSampler(f *fleet, st *Store, c *clock, logger *slog.Logger,
	host func(context.Context) (map[string]float64, error),
) *Sampler {
	s := newTestSampler(f, st, c, logger)
	s.deps.Host = host
	return s
}

// TestTheHostIsNeverForgotten: the host is in no inventory, so Retain -- which
// deletes whatever the inventory no longer holds -- must keep it; Prune ages it
// out after a day of silence like any subject.
//
// The key itself is pinned: it is what the history file on disk carries, and
// a renamed key would be a host whose day of history is orphaned by an update.
//
// Faults injected and seen red: HostSubject returning "machine/local" (the pin
// failed; Retain alone stayed green, since retained compares against
// HostSubject itself); the same with retained's host case removed -- the host
// filed like a machine, and Retain with no machine "local" deleted it.
func TestTheHostIsNeverForgotten(t *testing.T) {
	if HostSubject() != "host/local" {
		t.Errorf("HostSubject() = %q; the history file carries host/local", HostSubject())
	}

	st := NewMemory()
	st.Record(HostSubject(), t0, map[string]float64{"temp:cpu_thermal/temp1": 50})

	st.Retain(map[model.MachineID]bool{}, map[model.ClusterID]bool{})
	if !st.Has(HostSubject()) {
		t.Fatal("Retain with an empty inventory deleted the host's history")
	}

	st.Prune(t0.Add(Retention))
	if !st.Has(HostSubject()) {
		t.Fatal("Prune at exactly a day of silence deleted the host; it is kept for the whole day")
	}
	st.Prune(t0.Add(Retention + FineStep))
	if st.Has(HostSubject()) {
		t.Error("the host is still there after a day and one step of silence; Prune ages it like any subject")
	}
}

// TestTheSamplerRecordsTheHost: one pass files the host at the pass's instant,
// in the same slot as the machines of that pass.
func TestTheSamplerRecordsTheHost(t *testing.T) {
	f := newFleet("m1")
	st := NewMemory()
	c := &clock{now: t0}
	s := hostSampler(f, st, c, nil, func(context.Context) (map[string]float64, error) {
		return hostReading(), nil
	})

	s.pass(context.Background())

	got := st.Query(HostSubject(), Range1h, c.Now()).Series
	want := hostReading()
	if len(got) != len(want) {
		t.Errorf("host series %v, want exactly %v", keys(got), want)
	}
	for k, v := range want {
		if p := got[k]; len(p) != 1 || p[0] != (Point{ms(t0), v}) {
			t.Errorf("%s = %v, want [[%v %v]]", k, p, ms(t0), v)
		}
	}
	node := st.Query(MachineSubject("m1"), Range1h, c.Now()).Series["cpu"]
	if len(node) != 1 || node[0][0] != ms(t0) {
		t.Errorf("the node of the same pass is at %v, want the host's instant %v", node, ms(t0))
	}
}

// TestTheHostIsSampledWhenTheInventoryFails: a store that cannot list the
// machines says nothing about the machine the daemon runs on (RESEARCH
// Pitfall 2).
//
// Fault injected and seen red: the sampleHost call moved below the
// inventory's early return -- the host had no point.
func TestTheHostIsSampledWhenTheInventoryFails(t *testing.T) {
	st := NewMemory()
	c := &clock{now: t0}
	s := NewSampler(SamplerDeps{
		History: st,
		Logger:  quiet(),
		Now:     c.Now,
		Inventory: func(context.Context) ([]model.Machine, []model.Cluster, error) {
			return nil, nil, errors.New("the store is unreadable")
		},
		Host: func(context.Context) (map[string]float64, error) {
			return hostReading(), nil
		},
	})

	s.pass(context.Background())

	got := st.Query(HostSubject(), Range1h, c.Now()).Series["temp:cpu_thermal/temp1"]
	if len(got) != 1 || got[0] != (Point{ms(t0), 51.5}) {
		t.Errorf("with the inventory failing the host has %v, want its point at %v", got, ms(t0))
	}
}

// TestAFailingHostIsLoggedOnce: a host whose read fails is logged once when it
// starts and once when it recovers, and the passes it failed are a gap in its
// history, not a zero.
func TestAFailingHostIsLoggedOnce(t *testing.T) {
	f := newFleet()
	rec := &subjectRecorder{}
	st := NewMemory()
	c := &clock{now: t0}
	failing := true
	var mu sync.Mutex
	s := hostSampler(f, st, c, slog.New(rec), func(context.Context) (map[string]float64, error) {
		mu.Lock()
		defer mu.Unlock()
		if failing {
			return nil, errors.New("open /sys/class/hwmon: permission denied")
		}
		return hostReading(), nil
	})

	for range 2 {
		s.pass(context.Background())
		c.advance(FineStep)
	}
	mu.Lock()
	failing = false
	mu.Unlock()
	recoveredAt := c.Now()
	s.pass(context.Background())
	c.advance(FineStep)
	s.pass(context.Background())

	warns := rec.lines(slog.LevelWarn)
	if len(warns) != 1 || warns[0] != HostSubject() {
		t.Errorf("two failing passes logged warnings for %v, want one for %s", warns, HostSubject())
	}
	infos := rec.lines(slog.LevelInfo)
	if len(infos) != 1 || infos[0] != HostSubject() {
		t.Errorf("the recovery logged %v, want one line for %s", infos, HostSubject())
	}

	got := st.Query(HostSubject(), Range1h, c.Now()).Series["temp:cpu_thermal/temp1"]
	if len(got) != 2 {
		t.Fatalf("the host has %v, want two points: the failed passes are a gap", got)
	}
	for _, p := range got {
		if p[0] < ms(recoveredAt) {
			t.Errorf("a point at %v, before the host could be read again at %v", p[0], ms(recoveredAt))
		}
		if p[1] == 0 {
			t.Errorf("a zero at %v: a failed read is a gap, never a 0", p[0])
		}
	}
}

// subjectRecorder keeps the level and the subject of every line logged.
type subjectRecorder struct {
	mu     sync.Mutex
	logged []struct {
		level   slog.Level
		subject string
	}
}

func (r *subjectRecorder) Enabled(context.Context, slog.Level) bool { return true }
func (r *subjectRecorder) Handle(_ context.Context, rec slog.Record) error {
	subject := ""
	rec.Attrs(func(a slog.Attr) bool {
		if a.Key == "subject" {
			subject = a.Value.String()
		}
		return true
	})
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logged = append(r.logged, struct {
		level   slog.Level
		subject string
	}{rec.Level, subject})
	return nil
}
func (r *subjectRecorder) WithAttrs([]slog.Attr) slog.Handler { return r }
func (r *subjectRecorder) WithGroup(string) slog.Handler      { return r }

// lines is the subjects of every line logged at l, in order.
func (r *subjectRecorder) lines(l slog.Level) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, x := range r.logged {
		if x.level == l {
			out = append(out, x.subject)
		}
	}
	return out
}

// TestTheHostHistorySurvivesARestartWithItsGap (success criterion 2): the
// host's history written, flushed and read back is the same history -- and a
// stretch in which its CPU could not be read comes back as a gap, with no
// point and no 0, in every range, while the temperature it could read runs
// straight through it.
//
// Fault injected and seen red: encodeLocked leaving the host/local subject out
// of the file -- the reopened store had no host history at all.
func TestTheHostHistorySurvivesARestartWithItsGap(t *testing.T) {
	path := historyPath(t)
	now := t0.Add(3 * time.Hour)
	gapFrom, gapTo := t0.Add(20*time.Minute), t0.Add(40*time.Minute)
	inGap := func(at time.Time) bool { return at.After(gapFrom) && at.Before(gapTo) }

	before := Open(path, fsstore.ReadFile, fsstore.WriteFileAtomic, t0, quiet())
	for at := t0; !at.After(now); at = at.Add(FineStep) {
		sec := float64(at.Sub(t0) / time.Second)
		values := map[string]float64{"temp:cpu_thermal/temp1": 50 + sec/1000}
		if !inGap(at) {
			values["cpu"] = 5 + sec/1000
		}
		before.Record(HostSubject(), at, values)
	}
	if err := before.Flush(now); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	after := Open(path, fsstore.ReadFile, fsstore.WriteFileAtomic, now, quiet())
	want, got := allRanges(before, HostSubject(), now), allRanges(after, HostSubject(), now)
	for r := range want {
		if len(want[r].Series) == 0 {
			t.Fatalf("%s: nothing to compare; the test recorded nothing", r)
		}
		if !reflect.DeepEqual(got[r], want[r]) {
			t.Errorf("%s after a restart differs from before it:\n got %v\nwant %v", r, summary(got[r]), summary(want[r]))
		}

		if len(got[r].Series["cpu"]) == 0 {
			t.Errorf("%s: no cpu series at all after the restart", r)
		}
		for _, p := range got[r].Series["cpu"] {
			at := time.UnixMilli(int64(p[0]))
			if inGap(at) {
				t.Errorf("%s: a cpu point at %s, inside the stretch it was not read", r, at.Sub(t0))
			}
			if p[1] == 0 {
				t.Errorf("%s: a cpu point of 0 at %s; an unread value is a gap, never a 0", r, at.Sub(t0))
			}
		}
		// The 1-h range ends two hours after the stretch; the other two span it.
		if r == Range1h {
			continue
		}
		temps := 0
		for _, p := range got[r].Series["temp:cpu_thermal/temp1"] {
			if inGap(time.UnixMilli(int64(p[0]))) {
				temps++
			}
		}
		if want := int(gapTo.Sub(gapFrom)/(time.Duration(got[r].StepSeconds)*time.Second)) - 1; temps < want {
			t.Errorf("%s: %d temperature points inside the stretch, want at least %d -- it was read all along", r, temps, want)
		}
	}
}

// TestAHungHostReadDoesNotStallThePass (WR-03): a host read that never returns
// -- a statfs on a share that stopped answering -- costs the host its point in
// this pass and nothing else. The nodes of the same pass are still recorded,
// and shutdown still reaches its last write.
//
// Fault injected and seen red: sampleHost calling s.deps.Host inline again --
// the pass never returned, and Run never came back from its cancel.
func TestAHungHostReadDoesNotStallThePass(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	hung := func(context.Context) (map[string]float64, error) {
		// Deaf to its context, as a statfs in the kernel is.
		<-release
		return hostReading(), nil
	}

	t.Run("the pass", func(t *testing.T) {
		f := newFleet("m1")
		st := NewMemory()
		c := &clock{now: t0}
		s := hostSampler(f, st, c, nil, hung)
		s.deps.HostBudget = 20 * time.Millisecond

		done := make(chan struct{})
		go func() {
			defer close(done)
			s.pass(context.Background())
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("a hung host read held up the pass")
		}
		if st.Has(HostSubject()) {
			t.Error("the host has a point from a read that never finished")
		}
		if !st.Has(MachineSubject("m1")) {
			t.Error("the node of the same pass was not recorded")
		}
	})

	t.Run("shutdown", func(t *testing.T) {
		f := newFleet("m1")
		st := NewMemory()
		c := &clock{now: t0}
		s := hostSampler(f, st, c, nil, hung)
		s.deps.HostBudget = 20 * time.Millisecond

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			defer close(done)
			s.Run(ctx)
		}()
		deadline := time.Now().Add(5 * time.Second)
		for !st.Has(MachineSubject("m1")) {
			if time.Now().After(deadline) {
				t.Fatal("the first pass never recorded the node")
			}
			time.Sleep(time.Millisecond)
		}
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Run did not return after its cancel while the host read hung")
		}
	})
}
