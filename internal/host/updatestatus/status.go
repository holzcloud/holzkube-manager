// Package updatestatus reads what the update script last recorded.
package updatestatus

import (
	"errors"
	"io/fs"
	"time"
)

// DefaultPath is where deploy/holzkube-manager-update.sh records its outcome.
const DefaultPath = "/var/lib/holzkube-manager-update/status.json"

// MaxSize is the largest status file Read accepts.
const MaxSize = 4096

// ErrNotRecorded means the status file does not exist.
var ErrNotRecorded = errors.New("the update script has not recorded a run on this machine")

// The five outcomes the script records.
const (
	OutcomeCurrent    = "current"
	OutcomeAvailable  = "available"
	OutcomeUpdated    = "updated"
	OutcomeRolledBack = "rolled-back"
	OutcomeFailed     = "failed"
)

// Status is one recorded run of the update script.
type Status struct {
	CheckedAt time.Time `json:"checked_at"`
	Installed *string   `json:"installed"`
	Latest    *string   `json:"latest"`
	Outcome   string    `json:"outcome"`
}

// Read is not written yet.
func Read(fsys fs.FS, path string) (Status, error) {
	return Status{}, nil
}
