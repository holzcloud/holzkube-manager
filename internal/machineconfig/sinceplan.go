package machineconfig

import "errors"

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

// ChangedSincePlanError is the refusal of an apply whose node changed after the
// plan. Its text is the sentence the operator reads: it travels as the detail of
// the problem answer and is shown on the screen as it is.
type ChangedSincePlanError struct {
	// Known reports whether the plan's earlier state was still held, so that
	// Changes says what differs. The refusal never depends on it: it rests on
	// the digest alone.
	Known bool

	// Changes are the differences from the plan's state to the node's, over
	// every document of the configuration.
	Changes []Change
}

// Error is the sentence of the refusal.
func (e *ChangedSincePlanError) Error() string {
	return "Nothing was applied: the configuration of this node changed after the plan was made, " +
		"and the earlier state is no longer held, so the difference cannot be named. " +
		"Plan again to see what your patch does on top of the current configuration."
}

// Is makes errors.Is(err, ErrChangedSincePlan) true.
func (e *ChangedSincePlanError) Is(target error) bool { return target == ErrChangedSincePlan }
