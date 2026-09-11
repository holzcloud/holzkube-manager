package model

import "time"

// JobID identifies one long-running operation.
type JobID string

// JobState is where a job stands.
//
// Parked is the state that makes this engine worth having. A job that was
// interrupted in a step which cannot be checked -- no paired read-only "did it
// happen?" query -- is neither resumed nor failed: it is handed to a human,
// because the machine cannot know whether the side effect happened and both
// guesses are wrong in a way that matters. Retrying a reset that already ran is
// a second wipe; declaring it failed when it succeeded is a node reported as
// intact that is not.
type JobState string

const (
	// JobPending is created and waiting for the cluster's lease.
	JobPending JobState = "pending"

	// JobRunning is executing a step.
	JobRunning JobState = "running"

	// JobParked is interrupted in an unverifiable step and waiting for a
	// person to say what happened.
	JobParked JobState = "parked"

	// JobSucceeded ran every step.
	JobSucceeded JobState = "succeeded"

	// JobFailed stopped on an error.
	JobFailed JobState = "failed"

	// JobCancelled was stopped at a step boundary on request. It is distinct
	// from failed: nothing went wrong, somebody changed their mind, and the
	// steps that had already run still ran.
	JobCancelled JobState = "cancelled"
)

// Terminal reports whether a job will never change state again on its own.
func (s JobState) Terminal() bool {
	return s == JobSucceeded || s == JobFailed || s == JobCancelled
}

// JobKind names what a job does. It is the key the engine looks a job's step
// list up under, so a stored job from an older binary whose kind no longer
// exists is parked rather than guessed at.
type JobKind string

const (
	// JobReboot reboots a node.
	JobReboot JobKind = "node.reboot"

	// JobShutdown powers a node off.
	JobShutdown JobKind = "node.shutdown"

	// JobReset wipes a node. It is the most destructive thing this product
	// does.
	JobReset JobKind = "node.reset"
)

// StepState is where one step of a job stands.
type StepState string

const (
	// StepPending has not started.
	StepPending StepState = "pending"

	// StepRunning has started and has not reported an outcome.
	//
	// A step found in this state at startup is the whole problem: the process
	// died somewhere inside it, and whether the side effect happened is
	// exactly what the record cannot say.
	StepRunning StepState = "running"

	// StepDone completed.
	StepDone StepState = "done"

	// StepFailed reported an error.
	StepFailed StepState = "failed"

	// StepSkipped was not needed -- its paired check said the side effect had
	// already happened.
	StepSkipped StepState = "skipped"
)

// JobStep is one step's record.
type JobStep struct {
	Name  string    `json:"name"`
	State StepState `json:"state"`

	// Verifiable records whether this step had a paired read-only check at the
	// time it ran. It is stored rather than looked up on resume because the
	// answer must be the one that was true when the process died: a later
	// binary that added a check must not conclude that an older interrupted
	// step was safe to retry.
	Verifiable bool `json:"verifiable"`

	StartedAt  time.Time `json:"started_at,omitzero"`
	FinishedAt time.Time `json:"finished_at,omitzero"`

	// Detail is what happened, for the operator. It never carries a Go error
	// string verbatim for a failure the operator did not cause.
	Detail string `json:"detail,omitempty"`
}

// Job is one long-running operation as it is stored.
//
// It is persisted before each step runs and after each step ends, which is
// what makes a restart able to say anything at all about where it was. The
// cost is two small writes per step; the alternative is an engine whose
// crash behaviour is a guess.
type Job struct {
	ID JobID `json:"id"`

	Kind    JobKind   `json:"kind"`
	Cluster ClusterID `json:"cluster,omitempty"`
	Machine MachineID `json:"machine,omitempty"`

	State JobState `json:"state"`

	Steps []JobStep `json:"steps"`

	// Current is the index of the step being run, or the one that was running
	// when the process died.
	Current int `json:"current"`

	// Params are the job's inputs, already validated. They are stored so that
	// a resumed job runs the operation that was asked for rather than a
	// default -- a reset resumed with default flags would be maximally
	// destructive, which is precisely the trap JOB-07 exists for.
	Params map[string]string `json:"params,omitempty"`

	// CancelRequested is set by the operator and read at the next step
	// boundary. It is a stored flag rather than a channel because a cancel
	// must survive the restart that a cancel is often a reaction to.
	CancelRequested bool `json:"cancel_requested,omitempty"`

	// Actor is who asked for this. It is on the record as well as in the audit
	// archive, because a parked job's screen has to say whose decision it is
	// to finish.
	Actor string `json:"actor,omitempty"`

	// ParkedReason says what a person has to decide. It is the whole value of
	// the parked state: "interrupted" is not actionable, "the reset may or may
	// not have run; check the node" is.
	ParkedReason string `json:"parked_reason,omitempty"`

	CreatedAt  time.Time `json:"created_at"`
	StartedAt  time.Time `json:"started_at,omitzero"`
	FinishedAt time.Time `json:"finished_at,omitzero"`

	Rev uint64 `json:"rev"`
}
