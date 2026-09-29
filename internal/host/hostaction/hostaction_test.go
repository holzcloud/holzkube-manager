package hostaction

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
)

// manualTimers stands in for time.AfterFunc: it records every timer and fires
// one only when the test says so, so no test here sleeps.
type manualTimers struct {
	mu     sync.Mutex
	timers []*manualTimer
}

type manualTimer struct {
	d       time.Duration
	f       func()
	stopped bool
}

func (m *manualTimers) afterFunc(d time.Duration, f func()) func() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := &manualTimer{d: d, f: f}
	m.timers = append(m.timers, t)
	return func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		was := !t.stopped
		t.stopped = true
		return was
	}
}

func (m *manualTimers) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.timers)
}

// fire runs timer i's function, whether or not it was stopped: a time.Timer's
// Stop loses against a function that has already started, and the Box must be
// right either way.
func (m *manualTimers) fire(t *testing.T, i int) {
	t.Helper()
	m.mu.Lock()
	if i >= len(m.timers) {
		m.mu.Unlock()
		t.Fatalf("timer %d was never armed (%d armed)", i, len(m.timers))
	}
	f := m.timers[i].f
	m.mu.Unlock()
	f()
}

// logRecorder keeps every record with its attributes rendered as k=v.
type logRecorder struct {
	mu      sync.Mutex
	records []logged
}

type logged struct {
	level slog.Level
	text  string
}

func (r *logRecorder) Enabled(context.Context, slog.Level) bool { return true }
func (r *logRecorder) Handle(_ context.Context, rec slog.Record) error {
	var b strings.Builder
	b.WriteString(rec.Message)
	rec.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(&b, " %s=%v", a.Key, a.Value)
		return true
	})
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, logged{level: rec.Level, text: b.String()})
	return nil
}
func (r *logRecorder) WithAttrs([]slog.Attr) slog.Handler { return r }
func (r *logRecorder) WithGroup(string) slog.Handler      { return r }

func (r *logRecorder) at(l slog.Level) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, x := range r.records {
		if x.level == l {
			out = append(out, x.text)
		}
	}
	return out
}

// countingClaim wraps fsstore.Claim and counts the claims that took a file.
type countingClaim struct {
	mu    sync.Mutex
	taken int
}

func (c *countingClaim) claim(path, tag string) ([]byte, error) {
	data, err := fsstore.Claim(path, tag)
	if err == nil {
		c.mu.Lock()
		c.taken++
		c.mu.Unlock()
	}
	return data, err
}

func (c *countingClaim) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.taken
}

type rig struct {
	dir    string
	box    *Box
	timers *manualTimers
	logs   *logRecorder
	claims *countingClaim
}

func (r *rig) orderPath() string { return filepath.Join(r.dir, OrderFileName) }

// newRig builds a Box the way main.go does -- fsstore.PlaceNew and
// fsstore.Claim over a real 0700 data directory -- with hand-fired timers and
// a recording logger. before runs on the directory before the Box is built.
func newRig(t *testing.T, before func(dir string)) *rig {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if before != nil {
		before(dir)
	}
	r := &rig{dir: dir, timers: &manualTimers{}, logs: &logRecorder{}, claims: &countingClaim{}}
	r.box = NewBox(Config{
		DataDir:   dir,
		Place:     fsstore.PlaceNew,
		Claim:     r.claims.claim,
		AfterFunc: r.timers.afterFunc,
		Logger:    slog.New(r.logs),
	})
	t.Cleanup(r.box.Close)
	return r
}

func (r *rig) place(t *testing.T, a Action) Order {
	t.Helper()
	o, err := r.box.Place(a)
	if err != nil {
		t.Fatalf("Place(%s): %v", a, err)
	}
	return o
}

// leftovers is every name in the data directory, the order included.
func (r *rig) leftovers(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// TestWithdrawAnOrderNotPickedUp is D-13: an order the helper has not taken
// within 10 s is withdrawn, so a helper started later cannot carry it out by
// surprise.
//
// Fault injected and seen red: Place not arming the timer (F16).
func TestWithdrawAnOrderNotPickedUp(t *testing.T) {
	r := newRig(t, nil)
	o := r.place(t, Reboot)

	if n := r.timers.count(); n != 1 {
		t.Fatalf("Place armed %d pickup timers, want 1: an order nobody picks up would lie there until a helper starts", n)
	}
	if d := r.timers.timers[0].d; d != 10*time.Second {
		t.Errorf("pickup timeout = %v, want 10s (D-13)", d)
	}
	r.timers.fire(t, 0)

	if left := r.leftovers(t); len(left) != 0 {
		t.Errorf("after the withdrawal the data directory holds %v, want nothing", left)
	}
	got := r.box.Order()
	if got == nil || got.ID != o.ID || got.State != StateWithdrawn {
		t.Errorf("Order() after the withdrawal = %+v, want %s withdrawn", got, o.ID)
	}
	warns := r.logs.at(slog.LevelWarn)
	if len(warns) != 1 {
		t.Fatalf("the withdrawal logged %d warnings, want exactly 1: %q", len(warns), warns)
	}
	for _, want := range []string{o.ID, "reboot", "10 s", "systemctl status holzkube-manager-host.path"} {
		if !strings.Contains(warns[0], want) {
			t.Errorf("the withdrawal warning %q does not name %q", warns[0], want)
		}
	}
}

// TestWithdrawLeavesAPickedUpOrderAlone: the helper removes the order before it
// acts; a timer that finds it gone has nothing to do and says nothing.
func TestWithdrawLeavesAPickedUpOrderAlone(t *testing.T) {
	r := newRig(t, nil)
	o := r.place(t, Update)
	if err := os.Remove(r.orderPath()); err != nil { // what the helper does first
		t.Fatalf("remove: %v", err)
	}
	r.timers.fire(t, 0)

	if got := r.box.Order(); got == nil || got.ID != o.ID || got.State != StatePickedUp {
		t.Errorf("Order() = %+v, want %s picked-up", got, o.ID)
	}
	if n := r.claims.count(); n != 0 {
		t.Errorf("the timer claimed %d files after the helper took the order, want 0", n)
	}
	if w := r.logs.at(slog.LevelWarn); len(w) != 0 {
		t.Errorf("a picked-up order produced warnings: %q", w)
	}
}

// TestWithdrawNeverTakesANewerOrder is the reason the timer runs under the
// Box's mutex and checks the id: A's timer firing late must not withdraw B.
func TestWithdrawNeverTakesANewerOrder(t *testing.T) {
	r := newRig(t, nil)
	r.place(t, Update)
	if err := os.Remove(r.orderPath()); err != nil {
		t.Fatalf("remove: %v", err)
	}
	b := r.place(t, RestartService)

	r.timers.fire(t, 0) // A's timer, late

	data, err := os.ReadFile(r.orderPath())
	if err != nil {
		t.Fatalf("B's order is gone after A's timer fired: %v", err)
	}
	if want := "restart-service " + b.ID + "\n"; string(data) != want {
		t.Errorf("the slot holds %q, want B's order %q", data, want)
	}
	if got := r.box.Order(); got == nil || got.ID != b.ID || got.State != StatePending {
		t.Errorf("Order() = %+v, want %s pending", got, b.ID)
	}
	if n := r.claims.count(); n != 0 {
		t.Errorf("A's timer claimed %d files, want 0", n)
	}
}

// TestSweepAtStart is R9: an order present when the Box is built was placed by
// a previous process, and is withdrawn before the first request.
func TestSweepAtStart(t *testing.T) {
	t.Run("an order from before the start is withdrawn", func(t *testing.T) {
		line := "poweroff 0123456789abcdef\n"
		r := newRig(t, func(dir string) {
			if err := fsstore.PlaceNew(filepath.Join(dir, OrderFileName), []byte(line)); err != nil {
				t.Fatalf("PlaceNew: %v", err)
			}
		})
		if left := r.leftovers(t); len(left) != 0 {
			t.Errorf("after NewBox the data directory holds %v, want nothing", left)
		}
		warns := r.logs.at(slog.LevelWarn)
		if len(warns) != 1 {
			t.Fatalf("the startup withdrawal logged %d warnings, want 1: %q", len(warns), warns)
		}
		if !strings.Contains(warns[0], fmt.Sprintf("bytes=%d", len(line))) {
			t.Errorf("the startup warning %q does not give the byte count", warns[0])
		}
		if strings.Contains(warns[0], "0123456789abcdef") || strings.Contains(warns[0], "poweroff") {
			t.Errorf("the startup warning %q repeats the file's content; only its size belongs there", warns[0])
		}
		if r.box.Order() != nil {
			t.Errorf("Order() after the sweep is not nil: this process placed nothing")
		}
	})

	t.Run("no order, no warning", func(t *testing.T) {
		r := newRig(t, nil)
		if w := r.logs.at(slog.LevelWarn); len(w) != 0 {
			t.Errorf("a start without an order logged warnings: %q", w)
		}
	})

	t.Run("a Box without Claim leaves the slot alone", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, OrderFileName)
		if err := fsstore.PlaceNew(path, []byte("reboot 0123456789abcdef\n")); err != nil {
			t.Fatalf("PlaceNew: %v", err)
		}
		timers := &manualTimers{}
		box := NewBox(Config{DataDir: dir, Place: fsstore.PlaceNew, AfterFunc: timers.afterFunc})
		defer box.Close()
		if _, err := os.Lstat(path); err != nil {
			t.Errorf("a Box without Claim removed the order: %v", err)
		}
		if err := os.Remove(path); err != nil {
			t.Fatalf("remove: %v", err)
		}
		if _, err := box.Place(Update); err != nil {
			t.Fatalf("Place: %v", err)
		}
		if n := timers.count(); n != 0 {
			t.Errorf("a Box without Claim armed %d timers, want 0", n)
		}
	})
}

// TestSweepNamesTheHelperDropIn: the shipped path unit watches one path, and a
// daemon placing orders elsewhere says so once at start (Open Question 5).
func TestSweepNamesTheHelperDropIn(t *testing.T) {
	notThere := func(string, string) ([]byte, error) { return nil, fs.ErrNotExist }
	for _, tc := range []struct {
		name    string
		dataDir string
		want    int
	}{
		{"another data directory", "/srv/holzkube", 1},
		{"the reference data directory", filepath.Dir(ReferenceOrderPath), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logs := &logRecorder{}
			box := NewBox(Config{DataDir: tc.dataDir, Place: fsstore.PlaceNew, Claim: notThere, Logger: slog.New(logs)})
			defer box.Close()
			var hits []string
			for _, l := range logs.at(slog.LevelInfo) {
				if strings.Contains(l, "deploy/HOST-HELPER.md") {
					hits = append(hits, l)
				}
			}
			if len(hits) != tc.want {
				t.Fatalf("%d info lines name deploy/HOST-HELPER.md, want %d: %q", len(hits), tc.want, hits)
			}
			if tc.want == 1 {
				for _, p := range []string{filepath.Join(tc.dataDir, OrderFileName), ReferenceOrderPath} {
					if !strings.Contains(hits[0], p) {
						t.Errorf("the info line %q does not name %s", hits[0], p)
					}
				}
			}
		})
	}
}

// TestConcurrentPlace: the slot is one slot under any interleaving.
func TestConcurrentPlace(t *testing.T) {
	r := newRig(t, nil)
	const n = 8
	errs := make(chan error, n)
	var start sync.WaitGroup
	start.Add(1)
	var done sync.WaitGroup
	for range n {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait()
			_, err := r.box.Place(Reboot)
			errs <- err
		}()
	}
	start.Done()
	done.Wait()
	close(errs)

	ok, pending := 0, 0
	for err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrPending):
			pending++
		default:
			t.Errorf("Place: %v", err)
		}
	}
	if ok != 1 || pending != n-1 {
		t.Errorf("%d concurrent placements: %d placed and %d ErrPending, want 1 and %d", n, ok, pending, n-1)
	}
	if c := r.timers.count(); c != 1 {
		t.Errorf("%d timers armed, want 1 (one per placed order)", c)
	}
}

// TestCloseStopsThePickupTimer: after Close the process is going away; a timer
// that fires anyway withdraws nothing more, and Close itself has already
// withdrawn the order that still waited (WR-02).
func TestCloseStopsThePickupTimer(t *testing.T) {
	r := newRig(t, nil)
	r.place(t, Reboot)
	r.box.Close()

	if n := r.timers.count(); n != 1 {
		t.Fatalf("Place armed %d pickup timers, want 1", n)
	}
	if !r.timers.timers[0].stopped {
		t.Errorf("Close did not stop the pending pickup timer")
	}
	claims := r.claims.count()
	r.timers.fire(t, 0)
	if n := r.claims.count(); n != claims {
		t.Errorf("a timer firing after Close claimed %d more files, want 0", n-claims)
	}
}

// TestCloseWithdrawsAWaitingOrder is WR-02: an order still in the slot when the
// process ends must not outlive it. The next start's sweep comes too late --
// the path unit fires at boot, before the daemon starts -- and the helper's age
// window is only as good as a clock a Pi restores from a saved timestamp.
//
// Fault injected and seen red: Close only stopping the timer (the order stayed
// in the slot, pending).
func TestCloseWithdrawsAWaitingOrder(t *testing.T) {
	t.Run("an order still waiting is withdrawn", func(t *testing.T) {
		r := newRig(t, nil)
		o := r.place(t, Reboot)
		r.box.Close()

		if left := r.leftovers(t); len(left) != 0 {
			t.Errorf("after Close the data directory holds %v, want nothing", left)
		}
		if got := r.box.Order(); got == nil || got.ID != o.ID || got.State != StateWithdrawn {
			t.Errorf("Order() after Close = %+v, want %s withdrawn", got, o.ID)
		}
		warns := r.logs.at(slog.LevelWarn)
		if len(warns) != 1 || !strings.Contains(warns[0], o.ID) || !strings.Contains(warns[0], "nothing was done") {
			t.Errorf("Close logged %q, want one warning naming %s and that nothing was done", warns, o.ID)
		}
	})

	t.Run("an order the helper took is left to it", func(t *testing.T) {
		r := newRig(t, nil)
		o := r.place(t, RestartService)
		if err := os.Remove(r.orderPath()); err != nil { // what the helper does first
			t.Fatalf("remove: %v", err)
		}
		r.box.Close()
		if n := r.claims.count(); n != 0 {
			t.Errorf("Close claimed %d files after the helper took the order, want 0", n)
		}
		if got := r.box.Order(); got == nil || got.ID != o.ID || got.State != StatePickedUp {
			t.Errorf("Order() = %+v, want %s picked-up", got, o.ID)
		}
		if w := r.logs.at(slog.LevelWarn); len(w) != 0 {
			t.Errorf("Close after a pickup logged warnings: %q", w)
		}
	})

	t.Run("closing twice claims once", func(t *testing.T) {
		r := newRig(t, nil)
		r.place(t, Update)
		r.box.Close()
		r.box.Close()
		if n := r.claims.count(); n != 1 {
			t.Errorf("two Closes claimed %d files, want 1", n)
		}
	})
}
