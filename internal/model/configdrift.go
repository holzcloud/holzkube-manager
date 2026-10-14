package model

import "time"

// BaselineFormat is the version of the ConfigBaseline record as stored.
//
// It exists because the record is written into the operator's data directory
// and a later release may have to read what this one wrote. A reader that sees
// a format it does not know treats the baseline as unreadable rather than
// guessing.
const BaselineFormat = 1

// How a baseline came to be. The value is shown to the operator, so it says
// what happened and not what the code did.
const (
	// BaselineFirstSight: nobody told the daemon what the node should look
	// like; the first time it was read is the best statement available, and the
	// baseline says that it is only that.
	BaselineFirstSight = "first-sight"

	// BaselineApply: the baseline is what a configuration apply from this
	// daemon left behind, so it is a statement about the operator's intent.
	BaselineApply = "apply"

	// BaselineConfirmed: the operator looked at a difference and accepted it as
	// the new normal.
	BaselineConfirmed = "confirmed"
)

// ExpectedConfig is the configuration an apply from this daemon left behind on
// a node and that has not landed yet, for example because the node has not
// rebooted. It carries a digest and never content: what it is compared with is
// the digest of a later reading.
type ExpectedConfig struct {
	// Digest is the sha256 of the snapshot text the apply should produce.
	Digest string `json:"digest"`

	// Mode is the apply mode that was used (staged, no-reboot, ...), because
	// "not landed yet" is normal for one mode and a failure for another.
	Mode string `json:"mode"`

	// At is when the apply was made.
	At time.Time `json:"at"`
}

// ConfigBaseline is the last configuration this daemon knew a node to have,
// the thing a later reading is compared with to find drift. There is one per
// node, keyed by the machine id, in the config-baselines entity.
//
// A baseline is only ever created by a reading. Nothing invents one for a node
// nobody has read, because it would be a claim about a configuration no one has
// seen.
//
// A record is copied by value all over the place, which shares the Expected
// pointer between copies. Expected is therefore never changed in place: whoever
// changes it sets a new one.
type ConfigBaseline struct {
	// Machine is the node the baseline belongs to and the key of the record.
	Machine MachineID `json:"machine"`

	// Rev is the revision for compare-and-swap, as on every other record.
	Rev uint64 `json:"rev"`

	// Format is BaselineFormat at the time the record was written.
	Format int `json:"format"`

	// Cluster is the cluster the node belonged to when the baseline was taken.
	Cluster ClusterID `json:"cluster"`

	// Snapshot is the normalised, redacted text of the whole configuration, as
	// machineconfig.Snapshot produces it. It is never the unredacted text: a
	// secret that does not enter the snapshot cannot leak from the file, a
	// backup or a screen. That is also why a rotated secret is not drift.
	Snapshot string `json:"snapshot"`

	// Digest is the sha256 of Snapshot, so that "did it change" is one compare.
	Digest string `json:"digest"`

	// TakenAt is when the configuration in Snapshot was read from the node.
	TakenAt time.Time `json:"taken_at"`

	// Source says how the baseline came to be: BaselineFirstSight,
	// BaselineApply or BaselineConfirmed.
	Source string `json:"source"`

	// Machinery is the version of the Talos machinery that produced Snapshot
	// (gendata.VersionTag). Two snapshots from different machinery versions are
	// not compared as if they were the same text: a new version may render a
	// default the old one left out.
	Machinery string `json:"machinery"`

	// DriftSince is the start of the running deviation from the baseline, zero
	// when there is none. It is stored rather than kept in memory because the
	// banner that appears after 24 hours would otherwise start counting again
	// with every restart.
	DriftSince time.Time `json:"drift_since,omitzero"`

	// AppliedOnDriftAt is when an apply from this daemon landed on a deviation
	// that was open and not confirmed. The baseline is not moved then and
	// DriftSince stays, and the screen says the state includes an earlier
	// difference nobody confirmed. Confirming, and the node returning to the
	// baseline, clear it.
	AppliedOnDriftAt time.Time `json:"applied_on_drift_at,omitzero"`

	// Expected is what an apply left behind that has not landed yet, nil when
	// nothing is pending. See ExpectedConfig, and the note on the type about
	// never changing it in place.
	Expected *ExpectedConfig `json:"expected,omitempty"`
}
