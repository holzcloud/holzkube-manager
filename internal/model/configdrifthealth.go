package model

import "time"

// The states of a node's configuration as the cluster card and the metrics
// say them. They are the same strings as the State constants of
// internal/configdrift (a test there holds the two together); they are
// repeated here because this package imports no package of the domain.
const (
	ConfigStateNotRead         = "not-read"
	ConfigStateBaselineCreated = "baseline-created"
	ConfigStateOK              = "ok"
	ConfigStateSettling        = "settling"
	ConfigStateDrift           = "drift"
	ConfigStateUnreadable      = "unreadable"
	ConfigStateBaselineDamaged = "baseline-damaged"
)

// NodeConfigDrift is what a cluster's card says about one node's configuration.
// It carries a state, a start, a moment and a sentence, and never the
// difference itself: the cluster list is readable by a Reader, and the
// difference is configuration content that only the operator's route serves.
type NodeConfigDrift struct {
	Machine MachineID `json:"machine"`

	// Node is the host name. It is also the label value of the node's metrics,
	// and never the machine id.
	Node string `json:"node"`

	// State is one of the ConfigState values.
	State string `json:"state"`

	// Since is the start of the known deviation. It is set in the state
	// "unreadable" too, when the record knows one: a node that stops answering
	// while it drifts is still a node that drifts.
	Since time.Time `json:"since,omitzero"`

	// ReadAt is the last reading that worked.
	ReadAt time.Time `json:"read_at,omitzero"`

	// Sentence is said by the server and names no path and no value.
	Sentence string `json:"sentence"`
}

// Read reports whether the node answered at its last attempt: what it said was
// compared, or founded a baseline, or could not be judged against a damaged one.
// A node that did not answer has not been read, whatever it said before.
func (n NodeConfigDrift) Read() bool {
	switch n.State {
	case ConfigStateOK, ConfigStateBaselineCreated, ConfigStateDrift, ConfigStateSettling, ConfigStateBaselineDamaged:
		return true
	}
	return false
}

// Drifting reports whether a deviation is known: a start is set. It holds for
// an unreadable node whose record knows one, and it is the one thing that
// makes a node count as drifted.
func (n NodeConfigDrift) Drifting() bool { return !n.Since.IsZero() }

// Judged reports whether there is a verdict on the node: it is known to drift,
// or it was compared with a baseline (or founded one, or is on its way after an
// apply from here). A node that was never read, cannot be read, or has a baseline
// that cannot be trusted has none, and nothing that says "fine" may be derived
// from it.
func (n NodeConfigDrift) Judged() bool {
	if n.Drifting() {
		return true
	}
	switch n.State {
	case ConfigStateOK, ConfigStateBaselineCreated, ConfigStateSettling:
		return true
	}
	return false
}

// ConfigDriftHealth is what a cluster's card says about the configuration of its
// nodes. It is derived from the last readings and never stored.
type ConfigDriftHealth struct {
	// CheckedAt is when the nodes were last read.
	CheckedAt time.Time `json:"checked_at"`

	// Nodes counts the nodes the last pass covered, ReadNodes those that
	// answered at their last attempt, Drifted those with a known deviation. A
	// cluster whose nodes did not answer is not reported as fine.
	Nodes     int `json:"nodes"`
	ReadNodes int `json:"read_nodes"`
	Drifted   int `json:"drifted"`

	// Banner is true once a deviation has run unconfirmed for 24 hours,
	// counted from its start. The server computes it: the interface only shows
	// it. Warning is the sentence for it and names no configuration content.
	Banner  bool   `json:"banner"`
	Warning string `json:"warning,omitempty"`

	// Items is one entry per node of the pass, by host name. It is a list and
	// never null, also when the pass had no node.
	Items []NodeConfigDrift `json:"items"`
}
