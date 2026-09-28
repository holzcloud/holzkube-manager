// Package updatestatus reads what the update script last recorded (HOST-03).
//
// # Who writes the file
//
// deploy/holzkube-manager-update.sh, run as root by holzkube-manager-update.timer,
// writes DefaultPath from a single EXIT trap after every run that reached a
// decision: when it checked, which version was installed, which release was the
// newest, and what came of it. The write is a side matter for the script -- it
// never changes the script's exit code and never makes an update fail (D-15) --
// so a file that is missing only means the installed script predates this
// package, or never ran.
//
// # Why it lives where it lives
//
// The file sits in /var/lib/holzkube-manager-update, a directory root creates
// and owns, and deliberately not in the daemon's data directory (D-14). The
// daemon's user owns its data directory, and root writing a file into a
// directory an unprivileged user controls is the textbook symlink trap: that
// user swaps the target for a link and root writes wherever it points. So root
// writes into its own directory, and the daemon only reads.
//
// # Why the reader is strict
//
// This reader is the only thing between that file and the host page. Whatever
// it accepts is shown to the operator as a fact about their machine, so it
// accepts only what the script writes: a bounded size, one JSON object, an RFC
// 3339 time, one of five outcomes, and version strings of a fixed shape. A
// file that is not that is refused with an error naming the field and the rule.
// Read never invents a value: there is no fallback time, no "unknown" version,
// and no partially filled Status next to an error (D-16).
package updatestatus

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"regexp"
	"strings"
	"time"
)

// DefaultPath is where deploy/holzkube-manager-update.sh records its outcome.
const DefaultPath = "/var/lib/holzkube-manager-update/status.json"

// MaxSize is the largest status file Read accepts, in bytes. The script writes
// well under 200; the cap exists so a corrupted or planted file cannot make
// the daemon read without bound.
const MaxSize = 4096

// ErrNotRecorded means the status file does not exist: nothing has been
// recorded, which is a state and not a fault.
var ErrNotRecorded = errors.New("the update script has not recorded a run on this machine")

// The five outcomes the script records.
const (
	// OutcomeCurrent: the installed version is the newest release.
	OutcomeCurrent = "current"
	// OutcomeAvailable: --check found a newer release and changed nothing.
	OutcomeAvailable = "available"
	// OutcomeUpdated: the newest release was installed and the service came
	// back healthy.
	OutcomeUpdated = "updated"
	// OutcomeRolledBack: the new release did not come up healthy and the
	// previous binary was put back.
	OutcomeRolledBack = "rolled-back"
	// OutcomeFailed: the run ended with an error before any of the above.
	OutcomeFailed = "failed"
)

var outcomes = []string{OutcomeCurrent, OutcomeAvailable, OutcomeUpdated, OutcomeRolledBack, OutcomeFailed}

// versionPattern is the shape of a release version as the script records it:
// a tag name with or without its leading "v". The tag comes from GitHub, so
// nothing wider than this reaches the page.
var versionPattern = regexp.MustCompile(`^v?[0-9A-Za-z.+-]{1,64}$`)

// Status is one recorded run of the update script.
type Status struct {
	// CheckedAt is when the run ended, in UTC.
	CheckedAt time.Time `json:"checked_at"`
	// Installed is the version installed when the run ended; nil only for a
	// failed run that could not tell.
	Installed *string `json:"installed"`
	// Latest is the newest release the run saw; nil only for a failed run that
	// never got as far as asking.
	Latest *string `json:"latest"`
	// Outcome is one of the five Outcome constants.
	Outcome string `json:"outcome"`
}

// rawStatus keeps every field as raw JSON, so that a key that is absent (nil)
// and a key that is present with null ("null") can be told apart.
type rawStatus struct {
	CheckedAt json.RawMessage `json:"checked_at"`
	Installed json.RawMessage `json:"installed"`
	Latest    json.RawMessage `json:"latest"`
	Outcome   json.RawMessage `json:"outcome"`
}

// Read reads the status file at path through fsys.
//
// path is absolute, as an operator would name it; its leading "/" is stripped
// because fsys is rooted at "/". An absent file is ErrNotRecorded. Everything
// else that is not exactly what the script writes is an error that names the
// field and the rule, alongside the zero Status.
func Read(fsys fs.FS, path string) (Status, error) {
	name := strings.TrimPrefix(path, "/")
	if !fs.ValidPath(name) {
		return Status{}, fmt.Errorf("the update status path %s is not a clean absolute path", path)
	}

	info, err := fs.Stat(fsys, name)
	if errors.Is(err, fs.ErrNotExist) {
		return Status{}, fmt.Errorf("%w (%s does not exist)", ErrNotRecorded, path)
	}
	if err != nil {
		return Status{}, fmt.Errorf("could not read the update status file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Status{}, fmt.Errorf("the update status file %s is not a regular file", path)
	}
	if info.Size() > MaxSize {
		return Status{}, errors.New("the update status file is larger than 4 KiB")
	}

	raw, err := readLimited(fsys, name)
	if err != nil {
		return Status{}, err
	}
	return parse(raw)
}

// readLimited reads name, refusing a file that grew past MaxSize after the
// size check.
func readLimited(fsys fs.FS, name string) ([]byte, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, fmt.Errorf("could not read the update status file: %w", err)
	}
	defer func() { _ = f.Close() }()

	raw, err := io.ReadAll(io.LimitReader(f, MaxSize+1))
	if err != nil {
		return nil, fmt.Errorf("could not read the update status file: %w", err)
	}
	if len(raw) > MaxSize {
		return nil, errors.New("the update status file is larger than 4 KiB")
	}
	return raw, nil
}

func parse(raw []byte) (Status, error) {
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	// DisallowUnknownFields stays off on purpose: a later script may record
	// more, and an older daemon should still read what it knows.

	var r rawStatus
	if err := dec.Decode(&r); err != nil {
		if errors.Is(err, io.EOF) {
			return Status{}, errors.New("the update status file is not valid JSON: it is empty")
		}
		return Status{}, fmt.Errorf("the update status file is not valid JSON of the expected shape: %w", err)
	}
	// One object and nothing after it. A second object would mean two writers,
	// or a file assembled from pieces; either way not what the script wrote.
	var extra json.RawMessage
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return Status{}, errors.New("the update status file has data after the JSON object")
	}

	var s Status

	checked, err := parseTime(r.CheckedAt)
	if err != nil {
		return Status{}, err
	}
	s.CheckedAt = checked

	outcome, err := parseOutcome(r.Outcome)
	if err != nil {
		return Status{}, err
	}
	s.Outcome = outcome

	if s.Installed, err = parseVersion("installed", r.Installed, outcome); err != nil {
		return Status{}, err
	}
	if s.Latest, err = parseVersion("latest", r.Latest, outcome); err != nil {
		return Status{}, err
	}
	return s, nil
}

func parseTime(raw json.RawMessage) (time.Time, error) {
	if raw == nil {
		return time.Time{}, errors.New("the update status file has no checked_at")
	}
	var text string
	if isNull(raw) || json.Unmarshal(raw, &text) != nil {
		return time.Time{}, errors.New("checked_at in the update status file is not a string")
	}
	t, err := time.Parse(time.RFC3339, text)
	if err != nil {
		// The parse error would quote the value; the rule is enough.
		return time.Time{}, errors.New("checked_at in the update status file is not an RFC 3339 time")
	}
	return t.UTC(), nil
}

func parseOutcome(raw json.RawMessage) (string, error) {
	if raw == nil {
		return "", errors.New("the update status file has no outcome")
	}
	var text string
	if isNull(raw) || json.Unmarshal(raw, &text) != nil {
		return "", errors.New("outcome in the update status file is not a string")
	}
	for _, o := range outcomes {
		if text == o {
			return o, nil
		}
	}
	return "", fmt.Errorf("outcome in the update status file is none of %s", strings.Join(outcomes, ", "))
}

// parseVersion reads installed or latest. Both keys are required; null is
// accepted only for a failed run, which may have ended before it knew.
func parseVersion(field string, raw json.RawMessage, outcome string) (*string, error) {
	if raw == nil {
		return nil, fmt.Errorf("the update status file has no %s", field)
	}
	if isNull(raw) {
		if outcome == OutcomeFailed {
			return nil, nil
		}
		return nil, fmt.Errorf("%s in the update status file is null, which only a failed run may record", field)
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return nil, fmt.Errorf("%s in the update status file is not a string", field)
	}
	if !versionPattern.MatchString(text) {
		return nil, fmt.Errorf("%s in the update status file is not a version (%s)", field, versionPattern)
	}
	return &text, nil
}

func isNull(raw json.RawMessage) bool {
	return string(raw) == "null"
}
