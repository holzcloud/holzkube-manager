package hostaction

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"regexp"
	"strings"
	"time"
)

// Why the reader is strict
//
// The helper runs as root and writes its result into its own state directory
// (never into the daemon's: root writing where an unprivileged user can plant
// a link is the symlink trap, D-05). This reader is the only thing between that
// file and the host page, and whatever it accepts is shown to the operator as
// a fact about their machine. So it accepts exactly what the script writes --
// one line of four fields -- and refuses everything else with an error that
// names the rule and never repeats the file's bytes.

// DefaultResultPath is where deploy/holzkube-manager-host.sh records the
// outcome of the last order: its StateDirectory, which root creates and owns.
const DefaultResultPath = "/var/lib/holzkube-manager-host/last"

// MaxResultSize is the largest result file ReadResult accepts, in bytes. The
// script writes at most 16+1+15+1+8+1+20+1 = 63; the cap is there so a
// corrupted or planted file cannot make the daemon read without bound.
const MaxResultSize = 128

// ErrNoResult means the result file does not exist: the helper has recorded no
// order on this machine yet, which is a state and not a fault.
var ErrNoResult = errors.New("the host helper has not recorded an order on this machine")

// Outcome is what the helper did with an order.
type Outcome string

// The four outcomes the script records.
const (
	// OutcomeStarted: the order was valid, and its systemctl command was
	// about to run (the record is written before, because a reboot may end the
	// script).
	OutcomeStarted Outcome = "started"
	// OutcomeDone: a check's unit ended successfully -- the check looked, and
	// what it found is in the update status. Only check-update records it: the
	// helper waits for the check unit, so for the check alone the end of its
	// systemctl is the end of the order. Until the helper records it (or
	// failed), the helper is busy with the check and picks up nothing else.
	OutcomeDone Outcome = "done"
	// OutcomeRejected: the order was refused -- not the one fixed shape, or
	// too old. Nothing was done.
	OutcomeRejected Outcome = "rejected"
	// OutcomeFailed: the order could not be carried out -- its systemctl
	// command failed, or the order could not be consumed.
	OutcomeFailed Outcome = "failed"
)

var resultOutcomes = []Outcome{OutcomeStarted, OutcomeDone, OutcomeRejected, OutcomeFailed}

// Result is the helper's record of the last order it handled.
type Result struct {
	// ID is the order's id; empty when the helper could not trust the order
	// enough to repeat it (a malformed order, R6).
	ID string `json:"id"`
	// Action is the order's action; empty exactly when ID is.
	Action Action `json:"action"`
	// Outcome is one of the four Outcome constants.
	Outcome Outcome `json:"outcome"`
	// At is when the helper recorded it, in UTC.
	At time.Time `json:"at"`
}

// HelperServiceLimit is holzkube-manager-host.service's TimeoutStartSec: the
// longest the helper can be busy with one order before systemd ends it.
// TestTheCheckUnitRunsOnlyTheCheck holds it to the shipped unit.
const HelperServiceLimit = 3 * time.Minute

// CheckRunning reports whether r, the helper's last record, is a check the
// helper is still waiting for (13-REVIEW-2 WR-01, and its verification V-01,
// V-02, V-21). The helper starts the check unit blocking, so while the check
// runs its service stays activating, the path unit cannot start it a second
// time, and an order placed then would lie in the slot until the Box withdrew
// it. So the routes refuse every host action while this is true.
//
// The helper records the end of a check itself -- done or failed for its
// order, once its systemctl returns -- so a check is running exactly while
// its record is still "started", and nobody else's word counts: not the
// update status, which the hourly run writes as well. Two things end it
// without a record, and both are read here:
//
//   - HelperServiceLimit: systemd ends the helper then, whatever the check
//     did, and the helper records nothing more;
//   - a boot after the record: whatever the helper was waiting for went down
//     with the machine. boot is when the machine last booted, by the same
//     clock as now; the zero time when it could not be read, and then only
//     the limit counts. A record is from before the boot when its whole
//     second (the script records whole seconds, rounded down) ended by then.
//
// A record from the future (the clock went back) is not trusted to hold the
// routes shut.
func CheckRunning(r Result, boot, now time.Time) bool {
	if r.Action != CheckUpdate || r.Outcome != OutcomeStarted {
		return false
	}
	if age := now.Sub(r.At); age < 0 || age >= HelperServiceLimit {
		return false
	}
	return boot.IsZero() || r.At.Add(time.Second).After(boot)
}

// CheckHeld reports whether r, the helper's last record, says the helper was
// waiting for an update check while the order id, placed at placed, lay in
// the slot: a check of another order that has no end yet -- started less than
// HelperServiceLimit before that placement, and not from before the last boot
// (boot as in CheckRunning) -- or one whose end was recorded after that
// placement. The slot holds one order, so a check that ended after this one
// was placed was already running when it was. A started record older than the
// limit is a helper systemd ended before the order was placed: it held
// nothing, and the order's withdrawal is the path unit's to explain
// (13-REVIEW-2 round 3, I1). It is the withdrawal's diagnosis, and the page's
// "not picked up" sentence asks the same: a helper busy with a check is not a
// path unit that stopped.
func CheckHeld(r Result, id string, placed, boot time.Time) bool {
	if r.Action != CheckUpdate || r.ID == id {
		return false
	}
	switch r.Outcome {
	case OutcomeStarted:
		if placed.Sub(r.At) >= HelperServiceLimit {
			return false
		}
		return boot.IsZero() || r.At.Add(time.Second).After(boot)
	case OutcomeDone, OutcomeFailed:
		return !r.At.Before(placed.Truncate(time.Second))
	default:
		return false
	}
}

// idPattern is the id as the daemon generates it: 8 random bytes in lowercase
// hex.
var idPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)

// ReadResult reads the helper's result file at path through fsys.
//
// path is absolute; its leading "/" is stripped because fsys is rooted at "/".
// An absent file is ErrNoResult. Everything that is not exactly what the
// script writes is an error that names the rule, alongside the zero Result.
func ReadResult(fsys fs.FS, path string) (Result, error) {
	name, absolute := strings.CutPrefix(path, "/")
	if !absolute || !fs.ValidPath(name) {
		return Result{}, fmt.Errorf("the host helper's result path %s is not a clean absolute path", path)
	}

	info, err := fs.Stat(fsys, name)
	if errors.Is(err, fs.ErrNotExist) {
		return Result{}, fmt.Errorf("%w (%s does not exist)", ErrNoResult, path)
	}
	if err != nil {
		return Result{}, fmt.Errorf("could not read the host helper's result file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Result{}, fmt.Errorf("the host helper's result file %s is not a regular file", path)
	}
	if info.Size() > MaxResultSize {
		return Result{}, fmt.Errorf("the host helper's result file is larger than %d bytes", MaxResultSize)
	}

	raw, err := readResultLimited(fsys, name)
	if err != nil {
		return Result{}, err
	}
	return parseResult(raw)
}

// readResultLimited reads name, refusing a file that grew past MaxResultSize
// after the size check.
func readResultLimited(fsys fs.FS, name string) ([]byte, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, fmt.Errorf("could not read the host helper's result file: %w", err)
	}
	defer func() { _ = f.Close() }()

	raw, err := io.ReadAll(io.LimitReader(f, MaxResultSize+1))
	if err != nil {
		return nil, fmt.Errorf("could not read the host helper's result file: %w", err)
	}
	if len(raw) > MaxResultSize {
		return nil, fmt.Errorf("the host helper's result file is larger than %d bytes", MaxResultSize)
	}
	return raw, nil
}

// parseResult accepts `<id> <action> <outcome> <time>\n` and nothing else.
func parseResult(raw []byte) (Result, error) {
	text := string(raw)
	line, rest, found := strings.Cut(text, "\n")
	if !found || rest != "" {
		return Result{}, errors.New("the host helper's result file is not exactly one line ending in a newline")
	}
	fields := strings.Split(line, " ")
	if len(fields) != 4 {
		return Result{}, errors.New("the host helper's result line is not four fields separated by single spaces")
	}
	id, action, outcome, at := fields[0], fields[1], fields[2], fields[3]

	var r Result

	switch {
	case outcome == string(OutcomeStarted):
		r.Outcome = OutcomeStarted
	case outcome == string(OutcomeDone):
		r.Outcome = OutcomeDone
	case outcome == string(OutcomeRejected):
		r.Outcome = OutcomeRejected
	case outcome == string(OutcomeFailed):
		r.Outcome = OutcomeFailed
	default:
		names := make([]string, len(resultOutcomes))
		for i, o := range resultOutcomes {
			names[i] = string(o)
		}
		return Result{}, fmt.Errorf("the outcome in the host helper's result file is none of %s", strings.Join(names, ", "))
	}

	// "-" stands for an id and an action the helper could not trust: an order
	// that did not match the one fixed shape (rejected, R6), or one it could
	// not consume and therefore never read as valid (failed). Never for an
	// order it started or finished -- such an order was valid, so it has both.
	if id == "-" || action == "-" {
		if id != action {
			return Result{}, errors.New("the host helper's result file leaves out only one of id and action")
		}
		if r.Outcome == OutcomeStarted || r.Outcome == OutcomeDone {
			return Result{}, errors.New("the host helper's result file records a started or done order without an id")
		}
	} else {
		if !idPattern.MatchString(id) {
			return Result{}, errors.New("the id in the host helper's result file is not 16 lowercase hex characters")
		}
		if !Action(action).Known() {
			return Result{}, errors.New("the action in the host helper's result file is not one of the five host actions")
		}
		// Only the check waits for its command, so only the check is ever
		// done; a done reboot is not something the script writes.
		if r.Outcome == OutcomeDone && Action(action) != CheckUpdate {
			return Result{}, errors.New("the host helper's result file records done for an order other than check-update")
		}
		r.ID = id
		r.Action = Action(action)
	}

	t, err := time.Parse(time.RFC3339, at)
	if err != nil {
		// The parse error would quote the value; the rule is enough.
		return Result{}, errors.New("the time in the host helper's result file is not an RFC 3339 time")
	}
	r.At = t.UTC()
	return r, nil
}
