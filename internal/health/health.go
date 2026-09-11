// Package health holds the read-model vocabulary the inventory answers in:
// what a fact is worth, where it came from, and how old it is.
//
// Everything holzkube-manager shows about a node is a claim with a provenance and an
// age, and the two are what keep the dashboard honest when a cluster is half
// dead. A plain value cannot say "this is what the node said four minutes ago
// and nobody has confirmed it since", and a missing value cannot say why it is
// missing. Field[T] says both, and Level says which layer had to be alive for
// the claim to be made at all -- which is the difference between "etcd is
// down" and "the dashboard is empty".
package health

import (
	"encoding/json"
	"fmt"
	"time"
)

// Level is the layer a fact depends on.
//
// The ordering is a dependency ordering and not a severity: a LevelK8s fact
// needs Kubernetes, which needs etcd, which needs the node. That is what makes
// the rule checkable -- when etcd is down, every LevelNode fact must still be
// available, because nothing about it went through etcd.
//
// On the wire it is a string. The iota order is an implementation detail, and
// serialising the number would publish it as a contract nobody meant to make.
type Level int

const (
	// LevelNone is a fact holzkube-manager holds itself: the record it stored, the
	// address it last dialled. Nothing outside this process has to be alive.
	LevelNone Level = iota

	// LevelNode is a fact the node's own apid answers out of its own state.
	// It survives etcd and Kubernetes being down, and that survival is the
	// property INV-07 and INV-08 are about.
	LevelNode

	// LevelEtcd is a fact that needs a quorate etcd: member lists, cluster
	// health.
	LevelEtcd

	// LevelK8s is a fact that needs the Kubernetes control plane. holzkube-manager
	// never dials :6443 (D-20), so in practice these are facts read node-side
	// from Kubernetes-derived resources, and they go unavailable first.
	LevelK8s
)

// levelNames is the wire vocabulary, in iota order.
var levelNames = [...]string{"none", "node", "etcd", "k8s"}

// String returns the wire name.
func (l Level) String() string {
	if l < 0 || int(l) >= len(levelNames) {
		return fmt.Sprintf("Level(%d)", int(l))
	}
	return levelNames[l]
}

// Levels returns every level in dependency order. It exists so a test can walk
// the whole vocabulary rather than restate it and drift from it.
func Levels() []Level { return []Level{LevelNone, LevelNode, LevelEtcd, LevelK8s} }

// MarshalJSON writes the wire name.
func (l Level) MarshalJSON() ([]byte, error) {
	if l < 0 || int(l) >= len(levelNames) {
		return nil, fmt.Errorf("health: %d is not a level", int(l))
	}
	return json.Marshal(levelNames[l])
}

// UnmarshalJSON reads the wire name. An unknown name is an error rather than
// LevelNone: silently downgrading an unrecognised level would make a client
// built against a newer server read "needs Kubernetes" as "needs nothing".
func (l *Level) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	for i, name := range levelNames {
		if name == s {
			*l = Level(i)
			return nil
		}
	}
	return fmt.Errorf("health: %q is not a level", s)
}

// Field is one fact as the API serves it.
//
// The three states are distinguishable on purpose, and all three are meant to
// be shown:
//
//	available=true,  stale_since=nil  -- confirmed now
//	available=false, stale_since=set  -- known, but old; Value is still there
//	available=false, stale_since=nil  -- never read; UnavailableReason says why
//
// available means "this value is confirmed right now", not "this value
// exists". A stale field keeps its Value: dropping it would be
// indistinguishable from null in the UI, and that is exactly how the empty
// dashboard INV-08 forbids comes about.
type Field[T any] struct {
	Value             T          `json:"value,omitzero"`
	Level             Level      `json:"level"`
	Available         bool       `json:"available"`
	StaleSince        *time.Time `json:"stale_since,omitempty"`
	UnavailableReason string     `json:"unavailable_reason,omitempty"`
}

// Known returns a fact confirmed right now.
func Known[T any](level Level, value T) Field[T] {
	return Field[T]{Value: value, Level: level, Available: true}
}

// Stale returns a fact that was confirmed once and has not been since.
//
// since is when confirmation stopped, not how old the value is: the client
// does that arithmetic, so there is exactly one clock in the answer. A caller
// that passes the zero time gets Never instead, because a stale marker with no
// moment attached is the one shape the three states above cannot represent.
func Stale[T any](level Level, value T, since time.Time, reason string) Field[T] {
	if since.IsZero() {
		return Never[T](level, reason)
	}
	since = since.UTC()
	return Field[T]{
		Value:             value,
		Level:             level,
		StaleSince:        &since,
		UnavailableReason: reason,
	}
}

// Never returns a fact that has never been read, with the reason it has not.
//
// The reason is not optional in practice even though the field is: "—" with no
// explanation is what sends an operator looking for a broken node when the
// answer is that nothing ever asked.
func Never[T any](level Level, reason string) Field[T] {
	return Field[T]{Level: level, UnavailableReason: reason}
}
