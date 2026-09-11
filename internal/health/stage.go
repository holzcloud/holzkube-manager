package health

import "fmt"

// Stage is where a node's observer currently stands.
//
// The five states are ARCHITECTURE Pattern 7's automaton, spelled out because
// the distinctions are the ones an operator reads off the screen:
//
//	unknown -> connecting -> watching
//	                  ^         |
//	                  |         v
//	                  +----- degraded -> down
//
// degraded and down differ in what they claim, not in how bad they are. A
// degraded observer has a connection that answered recently and has since
// failed some reads; a down observer has nothing, and every fact it holds is
// stale. Collapsing the two would mean either alarming on the first missed
// read or staying calm through a total outage.
type Stage int

const (
	// StageUnknown is an observer that has not started. It is the zero value
	// so that a freshly loaded record reads as "nobody has looked yet" rather
	// than as a claim.
	StageUnknown Stage = iota

	// StageConnecting is dialling, or redialling after a failure.
	StageConnecting

	// StageWatching is connected with reads succeeding.
	StageWatching

	// StageDegraded is connected, with some reads failing. A node whose etcd
	// is down but whose apid answers lives here.
	StageDegraded

	// StageDown is not reachable. Facts keep their last value and gain a
	// stale_since; nothing is dropped (D-16).
	StageDown
)

var stageNames = [...]string{"unknown", "connecting", "watching", "degraded", "down"}

// String returns the wire name.
func (s Stage) String() string {
	if s < 0 || int(s) >= len(stageNames) {
		return fmt.Sprintf("Stage(%d)", int(s))
	}
	return stageNames[s]
}

// MarshalJSON writes the wire name, for the same reason Level does.
func (s Stage) MarshalJSON() ([]byte, error) {
	if s < 0 || int(s) >= len(stageNames) {
		return nil, fmt.Errorf("health: %d is not a stage", int(s))
	}
	return []byte(`"` + stageNames[s] + `"`), nil
}

// Stages returns every stage, so a test can walk the vocabulary.
func Stages() []Stage {
	return []Stage{StageUnknown, StageConnecting, StageWatching, StageDegraded, StageDown}
}

// Live reports whether facts from an observer in this stage are confirmed
// right now. It is the single place the Field[T].Available decision is made,
// so a new stage cannot be added without deciding what it claims.
func (s Stage) Live() bool { return s == StageWatching || s == StageDegraded }
