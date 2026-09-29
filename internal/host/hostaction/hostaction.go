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
)

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

	mu   sync.Mutex
	last *Order
}

// NewBox builds a Box. It touches nothing until Place is called.
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
	return &Box{cfg: cfg}
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
	return o, nil
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

// Close releases what the Box holds. It holds nothing yet.
func (b *Box) Close() {}
