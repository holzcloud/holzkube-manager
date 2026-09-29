// Package hostaction is the daemon's half of the host actions (HACT-01..08):
// restart the host, shut it down, restart holzkube-manager, and run the update
// the hourly timer runs.
//
// # The daemon places an order and runs nothing
//
// None of the four is something holzkube-manager may do itself. The daemon runs
// unprivileged under a hardened unit (NoNewPrivileges, no AF_UNIX to reach
// logind or PID 1), and that hardening stays. So this package does exactly one
// thing on the machine: it places a one-line order -- the action, one space, a
// random id, a newline -- in the daemon's data directory. Nothing else goes into
// it: not who asked (that is in the audit log), not when, no parameter. Every
// byte root parses is attack surface.
//
// The other half is root's: deploy/holzkube-manager-host.sh, started by a
// systemd path unit when the order appears. It consumes the order before it
// acts, accepts exactly four lines of one fixed shape, runs a fixed systemctl
// command for each, and records what came of it in its own state directory,
// which this package reads back (result.go).
//
// # One slot
//
// There is one order file and never a queue. Placement goes through the Place
// function the composition root hands in -- fsstore.PlaceNew, whose link(2)
// fails when the name is taken -- so a second order while the first still
// waits is refused (ErrPending), atomically in the kernel rather than by a
// check this process makes first.
//
// # No order is left lying
//
// An order nobody picks up is not harmless: the helper might be installed but
// stopped, and whoever starts it later would carry out a reboot nobody asked
// for any more. So the Box withdraws an order the helper has not taken within
// DefaultPickupTimeout (D-13), and withdraws any order it finds when it is
// built -- that one was placed by a previous process (R9). Withdrawing is a
// claim through fsstore.Claim: a rename, so whoever acts first, the helper or
// this process, owns the order, and the other sees it gone (R8).
//
// Nothing in this package opens, writes, renames or removes a file: the
// file-access guard (TestNoDirectFileAccessOutsideFsstore) scans it, and the
// daemon's writes to its data directory go through fsstore. It asks only
// whether the order still exists (Lstat), and it reads the helper's result
// through an fs.FS rooted at "/".
package hostaction

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// Action is one of the four host actions. Its string is what the order file
// carries and what the route path ends in.
type Action string

// The four host actions, and nothing else: the helper script knows exactly
// these and refuses every other word.
const (
	// Reboot restarts the whole machine (systemctl reboot).
	Reboot Action = "reboot"
	// Poweroff switches the machine off (systemctl poweroff).
	Poweroff Action = "poweroff"
	// RestartService restarts holzkube-manager.service and nothing else.
	RestartService Action = "restart-service"
	// Update starts holzkube-manager-update.service: the unit the hourly timer
	// runs, which looks for a newer release and installs it if there is one.
	Update Action = "update"
)

// Actions returns the four actions in one fixed order: the order the routes
// are registered in and the contract lists them in. (The page arranges its
// buttons its own way, harmless first.)
func Actions() []Action {
	return []Action{Reboot, Poweroff, RestartService, Update}
}

// Known reports whether a is one of the four actions.
func (a Action) Known() bool {
	for _, k := range Actions() {
		if a == k {
			return true
		}
	}
	return false
}

// OrderFileName is the one slot, a name directly in the data directory.
const OrderFileName = "host-order"

// ReferenceOrderPath is where the order lies in the reference installation,
// whose data directory is /var/lib/holzkube-manager. The shipped path unit
// watches exactly this path, and the helper script reads it by default; a
// daemon started with another data directory places its orders where no
// helper looks.
const ReferenceOrderPath = "/var/lib/holzkube-manager/" + OrderFileName

// ErrPending means an order already waits for the helper: the slot is taken,
// and the new order was not placed.
var ErrPending = errors.New("hostaction: an order is already waiting for the helper")

// OrderState is where the last placed order is, as far as this process can
// tell without asking the helper.
type OrderState string

const (
	// StatePending is an order whose file is still there: the helper has not
	// taken it.
	StatePending OrderState = "pending"
	// StatePickedUp is an order whose file is gone. The helper consumes an order
	// before it acts, so gone means taken -- what came of it is the result.
	StatePickedUp OrderState = "picked-up"
	// StateWithdrawn is an order this process took back because the helper had
	// not picked it up within the pickup timeout (D-13). Nothing was done.
	StateWithdrawn OrderState = "withdrawn"
)

// DefaultPickupTimeout is how long an order may wait for the helper before the
// Box withdraws it (D-13). The path unit starts the helper within a second of
// the order appearing; ten seconds without a pickup means no helper is
// watching.
const DefaultPickupTimeout = 10 * time.Second

// Order is the last order this process placed.
type Order struct {
	// ID is 16 lowercase hex characters, random, the helper's result names it.
	ID string `json:"id"`
	// Action is what was ordered.
	Action Action `json:"action"`
	// PlacedAt is when the order was placed, in UTC.
	PlacedAt time.Time `json:"placed_at"`
	// State is computed when the order is asked for, never stored.
	State OrderState `json:"state"`
}

// Config is what a Box works with.
type Config struct {
	// FS is rooted at "/"; the helper's result file is read through it.
	// Production: os.DirFS("/").
	FS fs.FS
	// DataDir is the daemon's data directory, absolute. The order is placed
	// directly in it.
	DataDir string
	// ResultPath is the absolute path of the helper's result file. Empty means
	// DefaultResultPath.
	ResultPath string
	// Place puts data at path only if nothing is there, and fails with an
	// error wrapping fs.ErrExist otherwise. Production: fsstore.PlaceNew.
	Place func(path string, data []byte) error
	// Claim takes the file at path out of its name, returns what it held and
	// removes it, and fails with an error wrapping fs.ErrNotExist when nothing
	// is there. Production: fsstore.Claim.
	//
	// It is what withdraws orders: the one the helper did not pick up within
	// PickupTimeout, and any order present when the Box is built. A Box without
	// Claim never withdraws and never sweeps -- no timer is armed, so none can
	// call a nil function. The API tests build such Boxes; the daemon never
	// does.
	Claim func(path, tag string) ([]byte, error)
	// PickupTimeout is how long an order may wait before it is withdrawn. Zero
	// means DefaultPickupTimeout.
	PickupTimeout time.Duration
	// AfterFunc arms a timer that calls f once after d and returns the function
	// that stops it. Nil means time.AfterFunc; tests fire timers by hand.
	AfterFunc func(d time.Duration, f func()) (stop func() bool)
	// Now is the clock the placement time comes from. Nil means time.Now.
	Now func() time.Time
	// Rand is where order ids come from. Nil means crypto/rand.Reader.
	Rand io.Reader
	// Logger receives what the Box has to say. Nil discards it.
	Logger *slog.Logger
}

// Box is the one slot for host orders.
type Box struct {
	cfg Config

	// mu is held across a placement and across a withdrawal. That is what keeps
	// an old order's timer from claiming a newer order: the timer looks at
	// last under the same lock the newer Place set it under, and gives up
	// unless last is still its own order.
	mu   sync.Mutex
	last *Order
	// stop stops the pickup timer of the last order, if one is armed.
	stop   func() bool
	closed bool
}

// NewBox builds a Box.
//
// With Claim configured it withdraws, before it returns, any order already in
// the slot: this process has not placed one yet, so it was placed by a
// previous process, and it must not wait for a helper started later (R9). The
// helper's own age window is the first net -- it may run at boot before this
// daemon does -- and this is the second. A Box without Claim touches nothing
// until Place is called.
func NewBox(cfg Config) *Box {
	if cfg.ResultPath == "" {
		cfg.ResultPath = DefaultResultPath
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Rand == nil {
		cfg.Rand = rand.Reader
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.DiscardHandler)
	}
	if cfg.PickupTimeout <= 0 {
		cfg.PickupTimeout = DefaultPickupTimeout
	}
	if cfg.AfterFunc == nil {
		cfg.AfterFunc = func(d time.Duration, f func()) func() bool {
			return time.AfterFunc(d, f).Stop
		}
	}
	b := &Box{cfg: cfg}

	if p := b.orderPath(); p != ReferenceOrderPath {
		// A log line rather than a page sentence: the shipped path unit watches
		// ReferenceOrderPath only, so every order from this daemon would be
		// withdrawn after the pickup timeout unless the helper was told where
		// to look.
		cfg.Logger.Info("host orders are placed outside the path the shipped helper watches; "+
			"install the drop-in described in deploy/HOST-HELPER.md or every order is withdrawn",
			"order_path", p, "helper_watches", ReferenceOrderPath)
	}

	if cfg.Claim != nil {
		b.sweep()
	}
	return b
}

// sweep withdraws an order left by a previous process. Only the byte count is
// logged: the content is whatever the file held, and it is not this process's
// to repeat.
func (b *Box) sweep() {
	data, err := b.cfg.Claim(b.orderPath(), "startup")
	switch {
	case err == nil:
		b.cfg.Logger.Warn("a host order from before this start was withdrawn; nothing was done",
			"path", b.orderPath(), "bytes", len(data))
	case errors.Is(err, fs.ErrNotExist):
		// The ordinary start: no order waits.
	default:
		b.cfg.Logger.Error("could not withdraw a host order from before this start", "path", b.orderPath(), "err", err)
	}
}

// orderPath is the absolute path of the slot.
func (b *Box) orderPath() string {
	return filepath.Join(b.cfg.DataDir, OrderFileName)
}

// Place places one order for a. It returns ErrPending when an order already
// waits, and the placed order otherwise.
//
// The line is the action, one space, the id, one newline -- the whole of what
// the helper accepts (D-01).
func (b *Box) Place(a Action) (Order, error) {
	if !a.Known() {
		return Order{}, fmt.Errorf("hostaction: %q is not a host action", string(a))
	}
	if b.cfg.Place == nil {
		return Order{}, errors.New("hostaction: no placement function was configured")
	}

	raw := make([]byte, 8)
	if _, err := io.ReadFull(b.cfg.Rand, raw); err != nil {
		return Order{}, fmt.Errorf("hostaction: generate an order id: %w", err)
	}
	id := hex.EncodeToString(raw)
	data := []byte(string(a) + " " + id + "\n")

	b.mu.Lock()
	defer b.mu.Unlock()

	if err := b.cfg.Place(b.orderPath(), data); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return Order{}, ErrPending
		}
		return Order{}, fmt.Errorf("hostaction: place the order: %w", err)
	}

	o := Order{ID: id, Action: a, PlacedAt: b.cfg.Now().UTC(), State: StatePending}
	b.last = &o
	b.cfg.Logger.Info("host order placed", "id", id, "action", string(a))

	if b.cfg.Claim != nil && !b.closed {
		// The previous order's file was gone, or this placement would have
		// failed; its timer has nothing left to do.
		if b.stop != nil {
			b.stop()
		}
		b.stop = b.cfg.AfterFunc(b.cfg.PickupTimeout, func() { b.withdraw(id) })
	}
	return o, nil
}

// withdraw is the pickup timer of the order id. It claims the order only while
// that order is still the last one and not settled: it runs under the mutex
// Place sets last under, so an old timer that fires late finds a newer order
// in last and leaves it alone.
func (b *Box) withdraw(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed || b.last == nil || b.last.ID != id || b.last.State == StateWithdrawn {
		return
	}
	_, err := b.cfg.Claim(b.orderPath(), "withdrawn-"+id)
	switch {
	case err == nil:
		b.last.State = StateWithdrawn
		b.cfg.Logger.Warn("host order withdrawn: the helper did not pick up the order within "+
			seconds(b.cfg.PickupTimeout)+"; check `systemctl status holzkube-manager-host.path`",
			"id", id, "action", string(b.last.Action))
	case errors.Is(err, fs.ErrNotExist):
		// The helper took it: the ordinary case, nothing to do.
	default:
		// The state stays with Order()'s Lstat: an order still there reads
		// pending, which is the truth.
		b.cfg.Logger.Error("could not withdraw a host order", "id", id, "action", string(b.last.Action), "err", err)
	}
}

// seconds renders d as "10 s".
func seconds(d time.Duration) string {
	return strconv.FormatFloat(d.Seconds(), 'f', -1, 64) + " s"
}

// Order returns the last order this process placed, with its state as of now,
// or nil when it placed none.
func (b *Box) Order() *Order {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.last == nil {
		return nil
	}
	o := *b.last
	if o.State == StateWithdrawn {
		// Settled: the file is gone because this process claimed it, and an
		// Lstat would call that picked up.
		return &o
	}
	// Lstat, not Stat: the question is whether the name is there, not what a
	// link planted there would point at.
	if _, err := os.Lstat(b.orderPath()); err == nil {
		o.State = StatePending
	} else {
		o.State = StatePickedUp
	}
	return &o
}

// Result is what the helper last recorded.
func (b *Box) Result() (Result, error) {
	return ReadResult(b.cfg.FS, b.cfg.ResultPath)
}

// Close stops the pending pickup timer. A timer that fires anyway afterwards
// withdraws nothing.
func (b *Box) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	if b.stop != nil {
		b.stop()
		b.stop = nil
	}
}
