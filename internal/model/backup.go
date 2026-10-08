package model

import "time"

// The scheduled etcd snapshots (2026-10-08).
//
// A schedule is a property of a cluster and it is off until somebody turns it
// on: a snapshot is the size of the cluster's etcd and it holds every
// Kubernetes secret in it, so keeping them is a decision, not a default.

// Backup interval presets. The schedule is a preset rather than a cron
// expression because the question an operator has is "how much may I lose",
// and three answers cover it; a free-form expression is a way to schedule a
// snapshot every minute on an SD card.
const (
	BackupOff    = "off"
	BackupEvery6 = "6h"
	BackupDaily  = "daily"
	BackupWeekly = "weekly"
)

// DefaultBackupKeep is how many scheduled snapshots are kept when the operator
// does not say, and MaxBackupKeep the most they may ask for. The ceiling is
// there because each one is a copy of the whole etcd on the Pi's own disk.
const (
	DefaultBackupKeep = 7
	MaxBackupKeep     = 60
)

// BackupInterval is how often a preset runs. ok is false for "off" and for a
// word that is not a preset.
func BackupInterval(preset string) (d time.Duration, ok bool) {
	switch preset {
	case BackupEvery6:
		return 6 * time.Hour, true
	case BackupDaily:
		return 24 * time.Hour, true
	case BackupWeekly:
		return 7 * 24 * time.Hour, true
	}
	return 0, false
}

// BackupSchedule is a cluster's snapshot schedule. Additive on Cluster and
// unversioned, for the reason Disabled is: a record written before it existed
// decodes as nil, which is "off", which is what it was.
type BackupSchedule struct {
	// Interval is one of the presets above; "off" or empty means no schedule.
	Interval string `json:"interval"`

	// Keep is how many scheduled snapshots are kept. Pre-upgrade snapshots are
	// not counted: they are a different kind and have their own two.
	Keep int `json:"keep"`

	// Since is when the schedule was last changed. A schedule that has never
	// produced a snapshot measures "overdue" from here, so turning it on is
	// not an immediate alarm.
	Since time.Time `json:"since"`
}

// Enabled reports whether the schedule runs.
func (s *BackupSchedule) Enabled() bool {
	if s == nil {
		return false
	}
	_, ok := BackupInterval(s.Interval)
	return ok
}

// BackupHealth is what the overview, the wall and the metrics say about a
// cluster's snapshots. It is derived from the files on disk and the attempt
// record, never stored.
type BackupHealth struct {
	Enabled  bool   `json:"enabled"`
	Interval string `json:"interval"`

	// LastSuccessAt is the newest scheduled snapshot; NewestAt the newest of
	// any kind (a pre-upgrade snapshot is a backup too).
	LastSuccessAt *time.Time `json:"last_success_at,omitempty"`
	NewestAt      *time.Time `json:"newest_at,omitempty"`

	// AgeSeconds is the age of the newest snapshot of any kind; absent when
	// there is none.
	AgeSeconds *int64 `json:"age_seconds,omitempty"`

	// Overdue is true when the schedule is on and the last scheduled snapshot
	// is older than twice its interval (or none was ever taken in that long).
	Overdue bool `json:"overdue"`

	// LastResult is ok, skipped or failed; LastReason says why for the last
	// two.
	LastResult string `json:"last_result,omitempty"`
	LastReason string `json:"last_reason,omitempty"`
}
