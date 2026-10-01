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

// tookEffect wraps fsstore's functions so that they do what they do and then
// report a failure of the tidying after it, as fsstore does when removing a
// temporary or flushing the directory fails (WR-03).
func tookEffect(err error) error {
	return fmt.Errorf("fsync directory: input/output error: %w", errors.Join(err, fsstore.ErrTookEffect))
}

// TestPlaceWhoseTidyingFailed is WR-03: once the link has succeeded the order
// is live, and the helper may already be carrying it out. A failure after it
// must not answer "failed" -- the operator would be told nothing happened
// while the host reboots -- and above all the order must get its pickup timer,
// or one nobody picks up lies there until the next start.
//
// Fault injected and seen red: Place treating the error as a failed placement.
func TestPlaceWhoseTidyingFailed(t *testing.T) {
	dir := t.TempDir()
	timers := &manualTimers{}
	logs := &logRecorder{}
	box := NewBox(Config{
		DataDir: dir,
		Place: func(path string, data []byte) error {
			if err := fsstore.PlaceNew(path, data); err != nil {
				return err
			}
			return tookEffect(nil)
		},
		Claim:     fsstore.Claim,
		AfterFunc: timers.afterFunc,
		Logger:    slog.New(logs),
	})
	defer box.Close()

	o, err := box.Place(Reboot)
	if err != nil {
		t.Fatalf("Place = %v, want the order: it is in place", err)
	}
	if got := box.Order(); got == nil || got.ID != o.ID || got.State != StatePending {
		t.Errorf("Order() = %+v, want %s pending", got, o.ID)
	}
	if n := timers.count(); n != 1 {
		t.Fatalf("%d pickup timers armed, want 1: a live order must be withdrawn if nobody takes it", n)
	}
	timers.fire(t, 0)
	if got := box.Order(); got == nil || got.State != StateWithdrawn {
		t.Errorf("Order() after the timer = %+v, want withdrawn", got)
	}
	if errs := logs.at(slog.LevelError); len(errs) != 1 || !strings.Contains(errs[0], "input/output error") {
		t.Errorf("the tidying failure was logged as %q, want one error naming it", errs)
	}
}

// TestWithdrawWhoseTidyingFailed: once the claim's rename has succeeded the
// order is withdrawn, whatever the read or remove after it says. Reporting it
// "could not withdraw" left the state to Order()'s Lstat, which found the name
// gone and said "picked-up" for an order nobody picked up -- and the page then
// waited for a helper answer that could never come.
//
// Fault injected and seen red: the three claim sites treating the error as a
// failed claim.
func TestWithdrawWhoseTidyingFailed(t *testing.T) {
	claimThenFail := func(path, tag string) ([]byte, error) {
		data, err := fsstore.Claim(path, tag)
		if err != nil {
			return data, err
		}
		return data, tookEffect(nil)
	}
	newBox := func(t *testing.T, dir string) (*Box, *manualTimers, *logRecorder) {
		t.Helper()
		timers := &manualTimers{}
		logs := &logRecorder{}
		box := NewBox(Config{
			DataDir:   dir,
			Place:     fsstore.PlaceNew,
			Claim:     claimThenFail,
			AfterFunc: timers.afterFunc,
			Logger:    slog.New(logs),
		})
		return box, timers, logs
	}

	t.Run("the pickup timer", func(t *testing.T) {
		box, timers, _ := newBox(t, t.TempDir())
		defer box.Close()
		o, err := box.Place(Poweroff)
		if err != nil {
			t.Fatalf("Place: %v", err)
		}
		timers.fire(t, 0)
		if got := box.Order(); got == nil || got.ID != o.ID || got.State != StateWithdrawn {
			t.Errorf("Order() = %+v, want %s withdrawn", got, o.ID)
		}
	})

	t.Run("close", func(t *testing.T) {
		box, _, _ := newBox(t, t.TempDir())
		o, err := box.Place(Poweroff)
		if err != nil {
			t.Fatalf("Place: %v", err)
		}
		box.Close()
		if got := box.Order(); got == nil || got.ID != o.ID || got.State != StateWithdrawn {
			t.Errorf("Order() after Close = %+v, want %s withdrawn", got, o.ID)
		}
	})

	t.Run("the startup sweep", func(t *testing.T) {
		dir := t.TempDir()
		if err := fsstore.PlaceNew(filepath.Join(dir, OrderFileName), []byte("reboot 0123456789abcdef\n")); err != nil {
			t.Fatalf("PlaceNew: %v", err)
		}
		box, _, logs := newBox(t, dir)
		defer box.Close()
		if w := logs.at(slog.LevelWarn); len(w) != 1 || !strings.Contains(w[0], "was withdrawn") {
			t.Errorf("the sweep warned %q, want that the order was withdrawn", w)
		}
	})
}

// TestOrderStateWhenTheSlotCannotBeRead is IN-04: only "not there" means the
// helper took the order. An Lstat that fails for another reason (EACCES here)
// is no answer, and calling it picked up would tell the page the helper had
// the order while it may still wait in the slot.
//
// Fault injected and seen red: every Lstat error read as picked-up.
func TestOrderStateWhenTheSlotCannotBeRead(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a 0000 directory; the refusal cannot be made here")
	}
	r := newRig(t, nil)
	o := r.place(t, Update)
	if err := os.Chmod(r.dir, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(r.dir, 0o700) })

	for range 3 {
		if got := r.box.Order(); got == nil || got.ID != o.ID || got.State != StatePending {
			t.Fatalf("Order() with the slot unreadable = %+v, want %s pending", got, o.ID)
		}
	}
	if errs := r.logs.at(slog.LevelError); len(errs) != 1 || !strings.Contains(errs[0], "permission denied") {
		t.Errorf("three polls logged %q, want the error once", errs)
	}

	if err := os.Chmod(r.dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if err := os.Remove(r.orderPath()); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got := r.box.Order(); got == nil || got.State != StatePickedUp {
		t.Errorf("Order() once the slot reads empty = %+v, want picked-up", got)
	}
}

// busyRig is a Box over a real data directory and a real result file, with a
// clock and a boot the test sets.
type busyRig struct {
	*rig
	result string
	now    time.Time
	up     time.Duration
	upErr  error
}

func newBusyRig(t *testing.T) *busyRig {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	state := t.TempDir()
	b := &busyRig{
		rig:    &rig{dir: dir, timers: &manualTimers{}, logs: &logRecorder{}, claims: &countingClaim{}},
		result: filepath.Join(state, "last"),
		now:    time.Date(2026, 9, 30, 16, 0, 0, 0, time.UTC),
		up:     time.Hour,
	}
	b.box = NewBox(Config{
		FS:         os.DirFS("/"),
		DataDir:    dir,
		ResultPath: b.result,
		Place:      fsstore.PlaceNew,
		Claim:      b.claims.claim,
		AfterFunc:  b.timers.afterFunc,
		Now:        func() time.Time { return b.now },
		SinceBoot:  func() (time.Duration, error) { return b.up, b.upErr },
		Logger:     slog.New(b.logs),
	})
	t.Cleanup(b.box.Close)
	return b
}

// record writes the helper's result line, recorded ago before the rig's now.
func (b *busyRig) record(t *testing.T, id string, a Action, o Outcome, ago time.Duration) {
	t.Helper()
	line := fmt.Sprintf("%s %s %s %s\n", id, a, o, b.now.Add(-ago).UTC().Truncate(time.Second).Format(time.RFC3339))
	if err := os.WriteFile(b.result, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
}

// pickUp does what the helper does first: the order's name goes.
func (b *busyRig) pickUp(t *testing.T) {
	t.Helper()
	if err := os.Remove(b.orderPath()); err != nil {
		t.Fatalf("pick up: %v", err)
	}
}

// wantRefused asks Busy and Place and wants both to say the helper is busy,
// with nothing placed.
func (b *busyRig) wantRefused(t *testing.T, why string) {
	t.Helper()
	if !b.box.Busy() {
		t.Errorf("%s: Busy = false, want true", why)
	}
	if _, err := b.box.Place(Reboot); !errors.Is(err, ErrBusy) {
		t.Errorf("%s: Place(reboot) = %v, want ErrBusy", why, err)
	}
	if _, err := os.Lstat(b.orderPath()); err == nil {
		t.Errorf("%s: a refused Place left an order in the slot", why)
		_ = os.Remove(b.orderPath())
	}
}

// wantPlaced asks Busy and Place and wants the order placed; the slot is
// emptied again afterwards, as a helper would.
func (b *busyRig) wantPlaced(t *testing.T, why string) {
	t.Helper()
	if b.box.Busy() {
		t.Errorf("%s: Busy = true, want false", why)
	}
	if _, err := b.box.Place(Reboot); err != nil {
		t.Errorf("%s: Place(reboot) = %v, want it placed", why, err)
		return
	}
	b.pickUp(t)
}

// TestBusyIsTheHelpersOwnWord (13-REVIEW-2 V-01, V-02, V-21): the helper is
// busy with a check while its own last record is that check started --
// whatever any update status says, which this Box does not even read -- and
// no longer once it recorded the end, its service's limit passed, or the
// machine booted since. Place refuses under its lock exactly when Busy says
// so.
func TestBusyIsTheHelpersOwnWord(t *testing.T) {
	const id = "c0ffee00c0ffee11"
	for _, tc := range []struct {
		name    string
		outcome Outcome
		action  Action
		ago     time.Duration
		up      time.Duration
		upErr   error
		busy    bool
	}{
		{"a check started 5 s ago", OutcomeStarted, CheckUpdate, 5 * time.Second, time.Hour, nil, true},
		{"a check started 2 min 40 s ago, the check unit's worst case", OutcomeStarted, CheckUpdate, 160 * time.Second, time.Hour, nil, true},
		{"a check started 5 s ago, the boot unknown", OutcomeStarted, CheckUpdate, 5 * time.Second, 0, errors.New("no clock"), true},
		{"a check done", OutcomeDone, CheckUpdate, 5 * time.Second, time.Hour, nil, false},
		{"a check failed", OutcomeFailed, CheckUpdate, 5 * time.Second, time.Hour, nil, false},
		{"a check started 3 min ago, the helper's limit", OutcomeStarted, CheckUpdate, HelperServiceLimit, time.Hour, nil, false},
		{"a check started 40 s ago, the machine up for 20 s", OutcomeStarted, CheckUpdate, 40 * time.Second, 20 * time.Second, nil, false},
		{"an update started 5 s ago", OutcomeStarted, Update, 5 * time.Second, time.Hour, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newBusyRig(t)
			b.up, b.upErr = tc.up, tc.upErr
			b.record(t, id, tc.action, tc.outcome, tc.ago)
			if tc.busy {
				b.wantRefused(t, tc.name)
			} else {
				b.wantPlaced(t, tc.name)
			}
		})
	}
}

// TestBusyFromTheCheckThisBoxPlaced (13-REVIEW-2 V-03, V-22): between the
// helper's rename of a check order and its "started" record, the record
// still names the order before, and only the Box knows the check was taken.
// It holds every order back until the helper records the check, or until
// ResultWithin has passed without a record.
func TestBusyFromTheCheckThisBoxPlaced(t *testing.T) {
	b := newBusyRig(t)
	// The record before: an older reboot, long answered.
	b.record(t, "0123456789abcdef", Reboot, OutcomeRejected, time.Hour)

	check, err := b.box.Place(CheckUpdate)
	if err != nil {
		t.Fatalf("Place(check-update): %v", err)
	}
	// Still in the slot: not busy -- the slot itself refuses the next one.
	if b.box.Busy() {
		t.Error("Busy = true while the check still waits in the slot, want false")
	}
	if _, err := b.box.Place(Reboot); !errors.Is(err, ErrPending) {
		t.Errorf("Place(reboot) with the check in the slot = %v, want ErrPending", err)
	}

	b.pickUp(t)
	b.now = b.now.Add(50 * time.Millisecond)
	b.wantRefused(t, "the check taken, nothing recorded for it")

	b.now = b.now.Add(time.Second)
	b.record(t, check.ID, CheckUpdate, OutcomeStarted, 0)
	b.wantRefused(t, "the check recorded started")

	b.now = b.now.Add(30 * time.Second)
	b.record(t, check.ID, CheckUpdate, OutcomeDone, 0)
	b.wantPlaced(t, "the check recorded done")
}

// TestBusyFromTheCheckThisBoxPlacedEnds: the Box's memory of a check it placed
// ends after a minute without a record, with a withdrawal, and with another
// order taken. The minute is written out, not as ResultWithin: a test in terms
// of the constant passes at any value of it, and the page's RESULT_WITHIN_MS
// is held to the constant, not to this test (13-REVIEW-2 round 3, I2).
// ResultWithin = 3 min goes red here.
func TestBusyFromTheCheckThisBoxPlacedEnds(t *testing.T) {
	if ResultWithin != time.Minute {
		t.Errorf("ResultWithin = %v, want 1m0s: the helper records started within milliseconds, and the page says "+
			"\"no answer\" after a minute", ResultWithin)
	}
	t.Run("a minute without a record", func(t *testing.T) {
		b := newBusyRig(t)
		if _, err := b.box.Place(CheckUpdate); err != nil {
			t.Fatal(err)
		}
		b.pickUp(t)
		b.now = b.now.Add(59 * time.Second)
		b.wantRefused(t, "59 s without a record")
		b.now = b.now.Add(time.Second)
		b.wantPlaced(t, "a minute without a record")
	})
	t.Run("withdrawn", func(t *testing.T) {
		b := newBusyRig(t)
		if _, err := b.box.Place(CheckUpdate); err != nil {
			t.Fatal(err)
		}
		b.timers.fire(t, 0)
		if got := b.box.Order(); got == nil || got.State != StateWithdrawn {
			t.Fatalf("Order() = %+v, want withdrawn", got)
		}
		b.wantPlaced(t, "a check withdrawn before anybody took it")
	})
	t.Run("another order taken", func(t *testing.T) {
		b := newBusyRig(t)
		if _, err := b.box.Place(Update); err != nil {
			t.Fatal(err)
		}
		b.pickUp(t)
		b.wantPlaced(t, "an update taken, nothing recorded for it")
	})
}

// TestTheWithdrawalNamesABusyHelper (13-REVIEW-2 V-01): an order the helper did
// not pick up because it was waiting for an update check is not a sign of a
// stopped path unit, and the warning must not send the operator there. The
// routes refuse while the helper is busy, so such an order is placed only when
// the clock misleads the busy rule -- here a check recorded in the future (the
// clock went back) -- or when the check ended after the order was placed.
func TestTheWithdrawalNamesABusyHelper(t *testing.T) {
	const check = "c0ffee00c0ffee11"
	for _, tc := range []struct {
		name string
		// before is recorded before the placement, after (if set) between the
		// placement and the withdrawal.
		before, after func(t *testing.T, b *busyRig)
		busy          bool
	}{
		{
			name:   "a check recorded in the future, still without an end",
			before: func(t *testing.T, b *busyRig) { b.record(t, check, CheckUpdate, OutcomeStarted, -time.Hour) },
			busy:   true,
		},
		{
			name:   "a check that ended after the order was placed",
			before: func(t *testing.T, b *busyRig) { b.record(t, check, CheckUpdate, OutcomeStarted, -time.Hour) },
			after: func(t *testing.T, b *busyRig) {
				b.now = b.now.Add(5 * time.Second)
				b.record(t, check, CheckUpdate, OutcomeDone, 0)
			},
			busy: true,
		},
		{
			name:   "a check that ended before the order was placed",
			before: func(t *testing.T, b *busyRig) { b.record(t, check, CheckUpdate, OutcomeDone, 5*time.Second) },
		},
		{
			name: "a check started before the last boot",
			before: func(t *testing.T, b *busyRig) {
				b.up = time.Minute
				b.record(t, check, CheckUpdate, OutcomeStarted, 2*time.Minute)
			},
		},
		{
			name:   "an older reboot",
			before: func(t *testing.T, b *busyRig) { b.record(t, "0123456789abcdef", Reboot, OutcomeRejected, time.Hour) },
		},
		{name: "nothing recorded", before: func(*testing.T, *busyRig) {}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newBusyRig(t)
			tc.before(t, b)
			o, err := b.box.Place(Reboot)
			if err != nil {
				t.Fatalf("Place(reboot): %v -- the case needs an order the routes let through", err)
			}
			if tc.after != nil {
				tc.after(t, b)
			}
			b.timers.fire(t, 0)

			warns := b.logs.at(slog.LevelWarn)
			if len(warns) != 1 {
				t.Fatalf("the withdrawal logged %d warnings, want exactly 1: %q", len(warns), warns)
			}
			names := func(s string) bool { return strings.Contains(warns[0], s) }
			if !names(o.ID) || !names("10 s") {
				t.Errorf("the warning %q does not name the order and the timeout", warns[0])
			}
			if tc.busy {
				if names("holzkube-manager-host.path") || !names("update check") || !names("holzkube-manager-update-check") || !names(check) {
					t.Errorf("the helper was waiting for check %s, and the warning says %q", check, warns[0])
				}
			} else if !names("systemctl status holzkube-manager-host.path") || names("update check") {
				t.Errorf("nothing held the helper, and the warning says %q, want the path unit", warns[0])
			}
		})
	}
}
