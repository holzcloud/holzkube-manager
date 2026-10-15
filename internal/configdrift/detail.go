package configdrift

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/machineconfig"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
)

// State is what is known about one node's configuration right now.
//
// The words are shown to the operator and counted in the metrics, so a rename
// later pulls callers, the API and the screen with it.
type State string

const (
	// StateNotRead means nothing has been read from the node since this daemon
	// started. It is never "ok" and never "no drift": the daemon does not know.
	StateNotRead State = "not-read"

	// StateBaselineCreated means the reading just now was the first one of this
	// node, and it is the baseline from here on.
	StateBaselineCreated State = "baseline-created"

	// StateOK means the node's configuration is the baseline.
	StateOK State = "ok"

	// StateSettling means an apply from this daemon is on its way and has not landed
	// yet, so a difference is expected for a while, at most the settle window.
	StateSettling State = "settling"

	// StateDrift means the node's configuration differs from the baseline and
	// nothing this installation did explains it.
	StateDrift State = "drift"

	// StateUnreadable means the node could not be read, or its configuration could
	// not be turned into a snapshot. It says nothing about drift in either
	// direction.
	StateUnreadable State = "unreadable"

	// StateBaselineDamaged means the stored baseline cannot be trusted, so nothing
	// is compared with it and nothing overwrites it.
	StateBaselineDamaged State = "baseline-damaged"
)

// ErrUnknownMachine reports a machine id the inventory does not hold.
var ErrUnknownMachine = errors.New("configdrift: no such machine")

// Notice is the standing statement about what a snapshot cannot show. It is
// part of every Detail, so a screen cannot present "no drift" without it.
const Notice = "A change of nothing but a secret value, such as a rotated token, is not visible here: " +
	"snapshots hold the redacted configuration (D-02). Fields such as machine.files, machine.env and " +
	"inlineManifests are shown as the node reports them."

// Detail is everything the screen and the API say about one node.
type Detail struct {
	Machine model.MachineID `json:"machine"`
	State   State           `json:"state"`

	// Sentence says what the state means, in words.
	Sentence string `json:"sentence"`

	// CheckedAt is the last attempt, successful or not; ReadAt is the last
	// reading that worked.
	CheckedAt time.Time `json:"checked_at,omitzero"`
	ReadAt    time.Time `json:"read_at,omitzero"`

	BaselineTakenAt time.Time `json:"baseline_taken_at,omitzero"`
	BaselineSource  string    `json:"baseline_source,omitempty"`

	// DriftSince is the start of the running deviation, kept across failed
	// readings and restarts.
	DriftSince time.Time `json:"drift_since,omitzero"`

	// Digest is the digest of the last good reading, 64 hex characters.
	Digest string `json:"digest,omitempty"`

	// Diff is the baseline against the last good reading, only while the state
	// is drift.
	Diff *machineconfig.Diff `json:"diff,omitempty"`

	Notice string `json:"notice"`
}

// Detail says where one node stands.
//
// A node that was not read since the daemon started is not-read even when a
// baseline lies in the store: the baseline is what was true once, the reading
// is what is true now.
func (s *Service) Detail(ctx context.Context, id model.MachineID) (Detail, error) {
	machines, err := s.deps.AllMachines(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("configdrift: list the machines: %w", err)
	}
	known := false
	for _, m := range machines {
		if m.ID == id {
			known = true
			break
		}
	}
	if !known {
		return Detail{}, ErrUnknownMachine
	}

	s.mu.Lock()
	r, read := s.readings[id]
	var rd reading
	if read {
		rd = *r
	}
	s.mu.Unlock()

	d := Detail{Machine: id, State: StateNotRead, Sentence: sentenceNotRead, Notice: Notice}

	// A failure to read the baseline changes nothing about the state: it is a
	// field that is missing.
	base, baseErr := s.deps.Baselines.Get(ctx, id)
	if baseErr == nil {
		d.BaselineTakenAt = base.TakenAt
		d.BaselineSource = base.Source
	}

	if !read {
		return d, nil
	}
	d.State = rd.state
	d.Sentence = rd.sentence
	d.CheckedAt = rd.at
	d.ReadAt = rd.lastGood
	d.DriftSince = rd.since
	d.Digest = rd.snap.Digest

	if rd.state == StateDrift && baseErr == nil {
		diff, err := machineconfig.CompareDocuments(base.Snapshot, rd.snap.Text)
		if err != nil {
			return Detail{}, fmt.Errorf("configdrift: compare the baseline with the reading: %w", err)
		}
		d.Diff = &diff
	}
	return d, nil
}

const sentenceNotRead = "Not read yet since this daemon started."

// when formats a moment for a sentence.
func when(t time.Time) string { return t.UTC().Format("2006-01-02 15:04 UTC") }

func describeSource(source string) string {
	switch source {
	case model.BaselineFirstSight:
		return "the first reading of this node"
	case model.BaselineApply:
		return "a configuration apply from holzkube-manager"
	case model.BaselineConfirmed:
		return "an operator who confirmed a difference"
	}
	return "an unknown source"
}

func sentenceBaselineCreated(b model.ConfigBaseline) string {
	return "This reading is now the baseline of this node (" + when(b.TakenAt) + ", " +
		describeSource(b.Source) + "). Later changes that this installation did not make will show up here as a difference."
}

func sentenceDrift(b model.ConfigBaseline, since time.Time) string {
	out := "The configuration differs from the baseline taken " + when(b.TakenAt) + " from " +
		describeSource(b.Source) + "; the difference was first seen " + when(since) + "."
	if !b.AppliedOnDriftAt.IsZero() {
		out += " The state includes an earlier change that was never confirmed, and a change made through " +
			"holzkube-manager landed on top of it on " + when(b.AppliedOnDriftAt) + "."
	}
	return out
}

// sentenceSettling says that an apply from this installation has not taken
// effect yet, and when a difference stops being explained by it.
func sentenceSettling(e *model.ExpectedConfig, window time.Duration) string {
	return "A configuration sent through holzkube-manager on " + when(e.At) + " (mode " + e.Mode + ") has not taken " +
		"effect on the node yet, so it differs from its baseline for now. That is expected until " +
		when(e.At.Add(window)) + "; if it still differs then, it is reported as a difference."
}

func sentenceOK(b model.ConfigBaseline) string {
	return "The configuration is the baseline taken " + when(b.TakenAt) + " from " + describeSource(b.Source) + "."
}

func sentenceDamaged(id model.MachineID, why string) string {
	return "The stored baseline (config-baselines/" + string(id) + ".json) is damaged: " + why +
		". It was left as it is and this node is not compared until it is dealt with."
}

// maxErrorText bounds how much of an error reaches a sentence.
const maxErrorText = 200

// shorten cuts an error text to maxErrorText characters.
func shorten(text string) string {
	runes := []rune(text)
	if len(runes) <= maxErrorText {
		return text
	}
	return string(runes[:maxErrorText]) + "..."
}

// reasonUnreadable says why a reading failed. Errors from machineconfig carry
// fixed sentences and no configuration content; any other error is a transport,
// protocol or store failure whose text may name the node's address, which the
// reader's role is shown anyway, and which is cut to maxErrorText characters.
func reasonUnreadable(err error) string {
	switch {
	case errors.Is(err, machineconfig.ErrSnapshotUnreadable):
		return "This version of holzkube-manager cannot load the node's configuration; a node running a newer " +
			"Talos than this build knows is the usual reason."
	case errors.Is(err, machineconfig.ErrSnapshotTooLarge):
		return "The node's configuration is larger than a snapshot may be."
	case errors.Is(err, machineconfig.ErrSnapshotUnsafe):
		return "The configuration still held a private key after redaction, so nothing was stored."
	case errors.Is(err, store.ErrConflict):
		return "The baseline was being changed at the same time; the next pass will try again."
	}
	return "The node could not be read: " + shorten(err.Error())
}

// reasonStoreFailed is the reason when the reading worked and the baseline could
// not be stored or loaded.
func reasonStoreFailed(err error) string {
	return "The baseline could not be stored or loaded: " + shorten(err.Error()) + "."
}

// sentenceUnreadable is a reason plus how stale the last good reading is.
func sentenceUnreadable(reason string, lastGood time.Time) string {
	if !strings.HasSuffix(reason, ".") {
		reason += "."
	}
	if lastGood.IsZero() {
		return reason + " It has not been read successfully since this daemon started."
	}
	return reason + " Last good reading: " + when(lastGood) + "."
}
