package machineconfig

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

var (
	// ErrChangedSincePlan reports that the configuration a node holds is no
	// longer the one the plan was made from: somebody else changed it between
	// the plan and the apply. Nothing was sent (Phase 17, D-01). There is no
	// switch that overrides it; the way out is a new plan.
	ErrChangedSincePlan = errors.New("machineconfig: the configuration of the node changed since the plan")

	// ErrBaseDigestRequired reports an apply that did not say which
	// configuration its plan was made from. The digest is required, not
	// optional: an optional one would be exactly the switch D-01 rules out.
	ErrBaseDigestRequired = errors.New("machineconfig: an apply needs the base digest of its plan")

	// ErrApplyBusy reports an apply that gave up waiting for another apply on
	// the same node, because the context of its request ended first. Nothing
	// was sent.
	ErrApplyBusy = errors.New("machineconfig: another apply on this node is still running")
)

// applyBusyError is ErrApplyBusy with the sentence the operator reads and the
// error of the context that ended the wait.
type applyBusyError struct{ cause error }

func (e *applyBusyError) Error() string {
	return "Nothing was applied: another apply on this node was still running when the time of this " +
		"request ran out. Look at the node, then plan again."
}

func (e *applyBusyError) Is(target error) bool { return target == ErrApplyBusy }
func (e *applyBusyError) Unwrap() error        { return e.cause }

// maxNamedChanges is how many changed paths the sentence names before it says
// how many more there are.
const maxNamedChanges = 8

// ChangedSincePlanError is the refusal of an apply whose node changed after the
// plan. Its text is the sentence the operator reads: it travels as the detail of
// the problem answer and is shown on the screen as it is.
//
// The sentence uses of every Change only Path, Kind and the two lengths. Before
// and After are rendered values, and redaction knows secrets only: a host name,
// an address or a SAN stays in them, and the sentence adds nothing the plan did
// not already show the same operator. Paths are keys of the configuration.
type ChangedSincePlanError struct {
	// Known reports whether the plan's earlier state was still held, so that
	// Changes says what differs. The refusal never depends on it: it rests on
	// the digest alone.
	Known bool

	// Changes are the differences from the plan's state to the node's, over
	// every document of the configuration, in the order CompareDocuments gives.
	Changes []Change
}

// Error is the sentence of the refusal.
func (e *ChangedSincePlanError) Error() string {
	const head = "Nothing was applied: the configuration of this node changed after the plan was made"
	const tail = "Plan again to see what your patch does on top of the current configuration."

	switch {
	case !e.Known:
		return head + ", and the earlier state is no longer held, so the difference cannot be named. " + tail
	case len(e.Changes) == 0:
		return head + ", but in its form: no setting differs that could be named, " +
			"for instance a value written another way. " + tail
	}

	named := e.Changes
	more := 0
	if len(named) > maxNamedChanges {
		more = len(named) - maxNamedChanges
		named = named[:maxNamedChanges]
	}
	parts := make([]string, 0, len(named)+1)
	for _, c := range named {
		parts = append(parts, c.Path+" ("+kindWords(c)+")")
	}
	if more > 0 {
		parts = append(parts, fmt.Sprintf("and %d more", more))
	}

	return head + ". Changed since the plan: " + strings.Join(parts, ", ") + ". " +
		"That can be another operator, talosctl, or an apply of your own " +
		"(the roll-back of a try is such a change). " + tail
}

// Is makes errors.Is(err, ErrChangedSincePlan) true.
func (e *ChangedSincePlanError) Is(target error) bool { return target == ErrChangedSincePlan }

// kindWords is what happened at a path, without the value.
func kindWords(c Change) string {
	if c.Kind == ChangeListGrew {
		return fmt.Sprintf("list changed, %s became %d", entries(c.LenBefore), c.LenAfter)
	}
	return string(c.Kind)
}

func entries(n int) string {
	if n == 1 {
		return "1 entry"
	}
	return fmt.Sprintf("%d entries", n)
}

// The limits of planMemo. They are the planner's choice (assumption A5 of the
// research of Phase 17) and they bear on the wording of a refusal only, never
// on whether there is one: that rests on the digest.
const (
	// planMemoPerNode is how many plans of one node are kept. The operator
	// plans, looks, plans again; four covers that without keeping a history.
	planMemoPerNode = 4

	// planMemoMaxBytes bounds all of it. An entry is a redacted snapshot of at
	// most MaxSnapshotBytes, usually a few KiB.
	planMemoMaxBytes = 8 << 20

	// planMemoLifetime is how long a plan is worth naming a difference against.
	// A plan older than that is one the operator has forgotten.
	planMemoLifetime = time.Hour
)

// planMemo holds the redacted snapshot text of recent plans so that a refusal
// can say what changed. Process memory only, nothing on disk, and never raw
// bytes: what it holds is what Snapshot returns.
type planMemo struct {
	perNode  int
	maxBytes int
	lifetime time.Duration
	now      func() time.Time

	mu      sync.Mutex
	entries map[model.MachineID][]memoEntry // oldest first
	total   int
}

type memoEntry struct {
	digest string
	text   string
	at     time.Time
}

func newPlanMemo(perNode, maxBytes int, lifetime time.Duration, now func() time.Time) *planMemo {
	return &planMemo{
		perNode:  perNode,
		maxBytes: maxBytes,
		lifetime: lifetime,
		now:      now,
		entries:  map[model.MachineID][]memoEntry{},
	}
}

// remember keeps a plan's snapshot. A plan of a state already held refreshes
// it. Beyond the limits the oldest entry goes first.
func (m *planMemo) remember(id model.MachineID, snap Snap) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.expire()
	list := m.entries[id]
	for i, e := range list {
		if e.digest == snap.Digest {
			m.total -= len(e.text)
			list = append(list[:i:i], list[i+1:]...)
			break
		}
	}
	list = append(list, memoEntry{digest: snap.Digest, text: snap.Text, at: m.now()})
	m.total += len(snap.Text)
	for len(list) > m.perNode {
		m.total -= len(list[0].text)
		list = list[1:]
	}
	m.entries[id] = list

	for m.total > m.maxBytes {
		if !m.dropOldest() {
			break
		}
	}
}

// recall returns the text held for a node's digest, if it is still held and not
// older than the lifetime.
func (m *planMemo) recall(id model.MachineID, digest string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.expire()
	for _, e := range m.entries[id] {
		if e.digest == digest {
			return e.text, true
		}
	}
	return "", false
}

// bytes is how much text the memo holds.
func (m *planMemo) bytes() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.total
}

// expire drops what is older than the lifetime. The caller holds the lock.
func (m *planMemo) expire() {
	cutoff := m.now().Add(-m.lifetime)
	for id, list := range m.entries {
		kept := list[:0:0]
		for _, e := range list {
			if e.at.Before(cutoff) {
				m.total -= len(e.text)
				continue
			}
			kept = append(kept, e)
		}
		if len(kept) == 0 {
			delete(m.entries, id)
			continue
		}
		m.entries[id] = kept
	}
}

// dropOldest removes the oldest entry of all nodes. The caller holds the lock.
func (m *planMemo) dropOldest() bool {
	var (
		oldestID model.MachineID
		oldestAt time.Time
		found    bool
	)
	for id, list := range m.entries {
		if len(list) == 0 {
			continue
		}
		if !found || list[0].at.Before(oldestAt) {
			oldestID, oldestAt, found = id, list[0].at, true
		}
	}
	if !found {
		return false
	}
	list := m.entries[oldestID]
	m.total -= len(list[0].text)
	if len(list) == 1 {
		delete(m.entries, oldestID)
	} else {
		m.entries[oldestID] = list[1:]
	}
	return true
}
