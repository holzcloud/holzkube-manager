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

// The three outcomes the script records.
const (
	// OutcomeStarted: the order was valid, and its systemctl command was
	// about to run (the record is written before, because a reboot may end the
	// script).
	OutcomeStarted Outcome = "started"
	// OutcomeRejected: the order was refused -- not the one fixed shape, or
	// too old. Nothing was done.
	OutcomeRejected Outcome = "rejected"
	// OutcomeFailed: the order could not be carried out -- its systemctl
	// command failed, or the order could not be consumed.
	OutcomeFailed Outcome = "failed"
)

var resultOutcomes = []Outcome{OutcomeStarted, OutcomeRejected, OutcomeFailed}

// Result is the helper's record of the last order it handled.
type Result struct {
	// ID is the order's id; empty when the helper could not trust the order
	// enough to repeat it (a malformed order, R6).
	ID string `json:"id"`
	// Action is the order's action; empty exactly when ID is.
	Action Action `json:"action"`
	// Outcome is one of the three Outcome constants.
	Outcome Outcome `json:"outcome"`
	// At is when the helper recorded it, in UTC.
	At time.Time `json:"at"`
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
	// order it started -- a started order was valid, so it has both.
	if id == "-" || action == "-" {
		if id != action {
			return Result{}, errors.New("the host helper's result file leaves out only one of id and action")
		}
		if r.Outcome == OutcomeStarted {
			return Result{}, errors.New("the host helper's result file records a started order without an id")
		}
	} else {
		if !idPattern.MatchString(id) {
			return Result{}, errors.New("the id in the host helper's result file is not 16 lowercase hex characters")
		}
		if !Action(action).Known() {
			return Result{}, errors.New("the action in the host helper's result file is not one of the five host actions")
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
