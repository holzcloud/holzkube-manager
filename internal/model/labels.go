package model

// Machine labels and the selectors over them (Omni parity phase 5).
//
// A label is the operator's own word about a machine, and a selector is a set
// of machines named by those words rather than one by one. That is the whole
// of it, and the restraint is the design: Omni's selectors and Kubernetes' are
// a small query language, and a query language is a thing this product would
// then have to keep in step with every screen that uses it. Equality and
// presence answer "which of my machines are these", and anything they cannot
// express is a label somebody has not written yet.

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
)

// MaxLabelKey and MaxLabelValue bound one entry, and MaxLabels bounds a
// machine's set.
//
// The numbers are not derived from anything upstream; they exist because a
// record with no bound on it is a record somebody can grow until the store's
// own writes get slow, and because a label nobody can read on a screen is not
// serving the purpose labels have.
const (
	MaxLabelKey   = 63
	MaxLabelValue = 63
	MaxLabels     = 32
)

// ValidateLabels reports every reason a label set cannot be stored, so a
// screen can show all of them at once rather than one per round trip.
func ValidateLabels(labels map[string]string) []error {
	var errs []error

	if len(labels) > MaxLabels {
		errs = append(errs, fmt.Errorf("a machine may carry %d labels and this is %d",
			MaxLabels, len(labels)))
	}

	for _, key := range sortedKeys(labels) {
		if err := validateLabelKey(key); err != nil {
			errs = append(errs, err)
		}
		if err := validateLabelValue(key, labels[key]); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

func validateLabelKey(key string) error {
	switch {
	case key == "":
		return fmt.Errorf("a label key cannot be empty")
	case len(key) > MaxLabelKey:
		return fmt.Errorf("the label key %q is %d characters and the limit is %d",
			key, len(key), MaxLabelKey)
	case strings.TrimSpace(key) != key:
		return fmt.Errorf("the label key %q begins or ends with a space, which is invisible on a "+
			"screen and makes two labels that look identical different", key)
	}
	if bad, ok := firstUnprintable(key); ok {
		return fmt.Errorf("the label key %q contains U+%04X, which is not printable", key, bad)
	}
	return nil
}

func validateLabelValue(key, value string) error {
	if len(value) > MaxLabelValue {
		return fmt.Errorf("the value of %q is %d characters and the limit is %d",
			key, len(value), MaxLabelValue)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("the value of %q begins or ends with a space", key)
	}
	if bad, ok := firstUnprintable(value); ok {
		return fmt.Errorf("the value of %q contains U+%04X, which is not printable", key, bad)
	}
	return nil
}

// firstUnprintable finds the first rune a screen cannot render as itself.
//
// It is the same concern internal/imagefactory's codepoint guard has and a much
// smaller version of it: a label is holzkube-manager's own record and never
// reaches an upstream canonicaliser, so what matters here is only that two
// labels that look alike are alike.
func firstUnprintable(s string) (rune, bool) {
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return r, true
		}
	}
	return 0, false
}

// LabelSelector names a set of machines by their labels.
//
// Match is an all-of: every entry has to hold. An empty selector matches
// nothing rather than everything, and that is the single most important
// decision in this file -- a machine class whose selector was cleared by
// accident would otherwise silently become "every machine in the fleet", and
// the operation on the other end of it is a cluster.
type LabelSelector struct {
	// Equals requires a label to be present with exactly this value.
	Equals map[string]string `json:"equals,omitempty"`

	// Present requires a label to be set, whatever its value.
	Present []string `json:"present,omitempty"`

	// Absent requires a label not to be set.
	//
	// It is here and "not equal to" is not, deliberately. "Absent" is a
	// statement about what somebody has not written; "not equal to" reads like
	// a statement about the machine and quietly includes every machine the
	// label was never written on, which is how a selector meant to exclude two
	// nodes selects a hundred.
	Absent []string `json:"absent,omitempty"`
}

// Empty reports a selector that names no condition at all.
func (s LabelSelector) Empty() bool {
	return len(s.Equals) == 0 && len(s.Present) == 0 && len(s.Absent) == 0
}

// Matches reports whether a machine's labels satisfy every condition.
//
// An empty selector matches nothing. See the type's own doc for why.
func (s LabelSelector) Matches(labels map[string]string) bool {
	if s.Empty() {
		return false
	}

	for key, want := range s.Equals {
		if got, ok := labels[key]; !ok || got != want {
			return false
		}
	}
	for _, key := range s.Present {
		if _, ok := labels[key]; !ok {
			return false
		}
	}
	for _, key := range s.Absent {
		if _, ok := labels[key]; ok {
			return false
		}
	}
	return true
}

// Sentence describes a selector in words, for a screen and for a refusal.
//
// A selector rendered as JSON is a selector an operator checks by squinting.
// This is the same idea as the etcd member list's own sentence: the data is
// there too, and the sentence is what somebody actually reads.
func (s LabelSelector) Sentence() string {
	if s.Empty() {
		return "nothing: a selector with no conditions matches no machines, deliberately"
	}

	var parts []string
	for _, key := range sortedKeys(s.Equals) {
		parts = append(parts, fmt.Sprintf("%s is %q", key, s.Equals[key]))
	}
	for _, key := range sortedCopy(s.Present) {
		parts = append(parts, fmt.Sprintf("%s is set", key))
	}
	for _, key := range sortedCopy(s.Absent) {
		parts = append(parts, fmt.Sprintf("%s is not set", key))
	}

	if len(parts) == 1 {
		return "machines where " + parts[0]
	}
	return "machines where " + strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

// MachineClassID identifies a named set of machines.
type MachineClassID string

// MachineClass is a selector with a name (Omni parity phase 5).
//
// It is the thing a cluster definition points at instead of listing machines
// one by one: "three control-plane nodes from class rack-b" is a sentence that
// survives a machine being replaced, and a list of three UUIDs is not.
//
// A class is deliberately not a group somebody adds machines to. It is a
// question asked of the labels, re-answered whenever it is used, so a machine
// joins by being labelled and leaves by being unlabelled -- one place to look
// when the membership is not what somebody expected.
type MachineClass struct {
	ID   MachineClassID `json:"id"`
	Name string         `json:"name"`

	// Description is the operator's own sentence about what this class is for.
	// A class named "storage" with no explanation is a class the next person
	// guesses at.
	Description string `json:"description,omitempty"`

	Selector LabelSelector `json:"selector"`

	CreatedAt time.Time `json:"created_at"`

	// Rev is the compare-and-swap revision. Every stored record carries one.
	Rev uint64 `json:"rev"`
}
