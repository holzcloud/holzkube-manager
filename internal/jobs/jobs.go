// Package jobs runs long-running, dangerous operations as persisted records.
//
// The whole design follows from one sentence: **a kill -9 in the middle of a
// step must never produce a doubled side effect.** Everything else here is
// what that sentence costs.
//
// A step is not a function. It is a function plus a paired read-only question
// -- "did this already happen?" -- and a step that cannot answer that question
// is marked unverifiable. The distinction is not bookkeeping:
//
//   - A verifiable step interrupted mid-flight is resumed by asking. If the
//     side effect happened, the step is skipped; if not, it runs.
//   - An unverifiable step interrupted mid-flight is **parked**, and a human
//     decides. The machine cannot know, and both guesses are wrong in a way
//     that matters: retrying a reset that already ran is a second wipe;
//     declaring it failed when it succeeded reports a node as intact that is
//     not.
//
// "Parked" is therefore not a failure mode of this engine. It is the engine's
// most important output.
//
// One mutating job runs per cluster at a time (JOB-03). Two operations
// overlapping on one cluster is how a reboot lands in the middle of an upgrade,
// and the lease is what makes that impossible rather than unlikely.
package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
	"github.com/holzcloud/holzkube-manager/internal/streamhub"
)

var (
	// ErrNotFound reports a job that is not in the store.
	ErrNotFound = errors.New("jobs: no such job")

	// ErrClusterBusy reports that another mutating job holds the cluster's
	// lease (JOB-03).
	ErrClusterBusy = errors.New("jobs: another job is already running on this cluster")

	// ErrUnknownKind reports a job whose kind this binary has no steps for.
	// It is what an older record meets after an upgrade removed its kind, and
	// it parks rather than fails.
	ErrUnknownKind = errors.New("jobs: unknown job kind")

	// ErrNotCancellable reports a cancel request against a job that has
	// already finished.
	ErrNotCancellable = errors.New("jobs: job is no longer running")
)

// Step is one unit of work plus the question that makes it safe to resume.
type Step struct {
	Name string

	// Do performs the step.
	Do func(ctx context.Context, j *model.Job) error

	// Happened answers "did this already take effect?" without changing
	// anything.
	//
	// It is nil for a step that cannot be checked, and nil is a real and
	// common answer rather than an omission: nothing a node can be asked
	// distinguishes "this node was reset five seconds ago and is booting" from
	// "this node is booting for another reason". A nil Happened makes the step
	// unverifiable, which makes an interrupted run of it a parked job.
	Happened func(ctx context.Context, j *model.Job) (bool, error)
}

// Verifiable reports whether this step can be resumed without a human.
func (s Step) Verifiable() bool { return s.Happened != nil }

// Definition is one job kind: its steps, in order.
type Definition struct {
	Kind  JobKindDef
	Steps []Step
}

// JobKindDef is model.JobKind, re-stated so this package's registry type reads
// as a definition rather than as a string.
type JobKindDef = model.JobKind

// Builder produces the steps for one job. It is a function of the job record
// rather than a fixed list because a step closes over the job's parameters --
// which disk to wipe, whether to reboot -- and those are stored per job.
type Builder func(j model.Job) ([]Step, error)

// Deps is what the engine needs.
type Deps struct {
	Store  store.Store
	Logger *slog.Logger

	// Hub is where step progress is published, so a browser watching a job
	// sees it move. It is the same fan-out the log panels use, which is why
	// phase 5 came first (JOB-04).
	Hub *streamhub.Hub

	Now func() time.Time
}

// Engine runs jobs.
type Engine struct {
	deps Deps

	builders map[model.JobKind]Builder

	mu sync.Mutex
	// leases maps a cluster to the job currently holding it. It is in memory
	// because it is a statement about *this* process: a lease that survived a
	// crash would block every job on a cluster until somebody cleared it by
	// hand, and the record a crashed job left behind is already the thing that
	// says what happened.
	leases map[model.ClusterID]model.JobID

	// running maps a job to its cancel function, so a cancel request reaches
	// the step boundary promptly rather than at the next poll.
	running map[model.JobID]context.CancelFunc

	wg     sync.WaitGroup
	closed bool
}

// New builds an engine.
func New(d Deps) *Engine {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Engine{
		deps:     d,
		builders: map[model.JobKind]Builder{},
		leases:   map[model.ClusterID]model.JobID{},
		running:  map[model.JobID]context.CancelFunc{},
	}
}

// Register teaches the engine a job kind.
func (e *Engine) Register(kind model.JobKind, build Builder) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.builders[kind] = build
}

// Topic is where a job's progress is published.
func Topic(id model.JobID) streamhub.Topic { return streamhub.Topic("job:" + string(id)) }

// Progress is one entry on a job's stream.
type Progress struct {
	Job   model.JobID    `json:"job"`
	State model.JobState `json:"state"`
	Step  string         `json:"step,omitempty"`
	Index int            `json:"index"`
	Total int            `json:"total"`

	// StepState is the step's own state, so a viewer can tell "starting" from
	// "finished" without diffing two messages.
	StepState model.StepState `json:"step_state,omitempty"`

	Detail string    `json:"detail,omitempty"`
	At     time.Time `json:"at"`
}

// Submit creates a job and starts it if the cluster's lease is free.
//
// It returns immediately: the caller is an HTTP handler answering 202 with the
// job's id (JOB-09), and a destructive operation that completes inside a
// request is a destructive operation whose progress nobody can watch and whose
// failure arrives as a timeout.
func (e *Engine) Submit(ctx context.Context, j model.Job) (model.Job, error) {
	if j.Kind == "" {
		return model.Job{}, errors.New("jobs: a job needs a kind")
	}

	e.mu.Lock()
	build, known := e.builders[j.Kind]
	e.mu.Unlock()
	if !known {
		return model.Job{}, fmt.Errorf("%w: %s", ErrUnknownKind, j.Kind)
	}

	steps, err := build(j)
	if err != nil {
		return model.Job{}, err
	}
	if len(steps) == 0 {
		return model.Job{}, fmt.Errorf("jobs: %s has no steps", j.Kind)
	}

	id, err := newJobID()
	if err != nil {
		return model.Job{}, err
	}

	j.ID = id
	j.State = model.JobPending
	j.Current = 0
	j.CreatedAt = e.deps.Now().UTC()
	j.Rev = 0
	j.Steps = make([]model.JobStep, 0, len(steps))
	for _, s := range steps {
		j.Steps = append(j.Steps, model.JobStep{
			Name:  s.Name,
			State: model.StepPending,
			// Recorded now, from the step list that will actually run. On
			// resume the stored value is used rather than a fresh lookup: a
			// later binary that added a check must not conclude that an older
			// interrupted step was safe to retry.
			Verifiable: s.Verifiable(),
		})
	}

	// The lease is taken before the record is written, so two concurrent
	// submissions cannot both see a free cluster.
	if err := e.acquire(j.Cluster, j.ID); err != nil {
		return model.Job{}, err
	}

	stored, err := e.deps.Store.Jobs().Put(ctx, j)
	if err != nil {
		e.release(j.Cluster, j.ID)
		return model.Job{}, err
	}

	e.start(stored, steps)
	return stored, nil
}

// start runs a job in the background.
//
// The goroutine gets its own copy, and that is not defensive tidiness: a
// model.Job passed by value still shares its Steps array and its Params map
// with the caller. Submit's caller is an HTTP handler that JSON-encodes the
// returned job into its 202 while run() below is already writing
// Steps[i].State into the same backing array. The race detector caught it on
// CI, on the path every reboot, shutdown, reset, provision and upgrade takes.
func (e *Engine) start(j model.Job, steps []Step) {
	j = j.Clone()

	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return
	}
	// Deliberately not derived from the request context: a job outlives the
	// HTTP call that asked for it, and a reboot that stopped because the
	// operator closed the tab would be the worst kind of half-done.
	runCtx, cancel := context.WithCancel(context.Background())
	e.running[j.ID] = cancel
	e.mu.Unlock()

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		defer func() {
			e.mu.Lock()
			delete(e.running, j.ID)
			e.mu.Unlock()
			e.release(j.Cluster, j.ID)
		}()

		e.run(runCtx, j, steps)
	}()
}

// run executes a job's steps, persisting before and after each one.
//
// Two writes per step is the price of being able to say anything after a
// crash. The alternative -- writing once at the end -- makes every
// interruption indistinguishable from every other.
func (e *Engine) run(ctx context.Context, j model.Job, steps []Step) {
	j.State = model.JobRunning
	j.StartedAt = e.deps.Now().UTC()
	j = e.save(ctx, j)
	e.publish(j, "", model.StepPending, "")

	for i := j.Current; i < len(steps); i++ {
		// The cancel check is at the step boundary and nowhere else (JOB-04).
		// Cancelling inside a step would leave exactly the ambiguity this
		// engine exists to avoid.
		if cancelled, reason := e.cancelRequested(ctx, j); cancelled {
			j.State = model.JobCancelled
			j.FinishedAt = e.deps.Now().UTC()
			j = e.save(ctx, j)
			e.publish(j, steps[i].Name, model.StepPending, reason)
			return
		}

		j.Current = i
		j.Steps[i].State = model.StepRunning
		j.Steps[i].StartedAt = e.deps.Now().UTC()
		j = e.save(ctx, j)
		e.publish(j, steps[i].Name, model.StepRunning, "")

		err := steps[i].Do(ctx, &j)

		now := e.deps.Now().UTC()
		j.Steps[i].FinishedAt = now
		switch {
		case err == nil:
			j.Steps[i].State = model.StepDone
		case errors.Is(err, context.Canceled) && ctx.Err() != nil:
			// The process is shutting down mid-step. Leave the step marked
			// running and the job marked running: what happens next is
			// Resume's decision at the next start, and it is the decision this
			// engine exists to make carefully.
			e.deps.Logger.Info("job interrupted by shutdown",
				slog.String("job", string(j.ID)), slog.String("step", steps[i].Name))
			return
		default:
			j.Steps[i].State = model.StepFailed
			j.Steps[i].Detail = err.Error()
			j.State = model.JobFailed
			j.FinishedAt = now
			j = e.save(ctx, j)
			e.publish(j, steps[i].Name, model.StepFailed, err.Error())
			return
		}

		j = e.save(ctx, j)
		e.publish(j, steps[i].Name, model.StepDone, j.Steps[i].Detail)
	}

	j.State = model.JobSucceeded
	j.Current = len(steps) - 1
	j.FinishedAt = e.deps.Now().UTC()
	j = e.save(ctx, j)
	e.publish(j, "", model.StepDone, "")
}

// cancelRequested re-reads the record, because a cancel arrives through the
// store from an HTTP handler rather than through a channel.
func (e *Engine) cancelRequested(ctx context.Context, j model.Job) (bool, string) {
	if ctx.Err() != nil {
		return true, "the job was cancelled"
	}
	current, err := e.deps.Store.Jobs().Get(ctx, j.ID)
	if err != nil {
		return false, ""
	}
	if current.CancelRequested {
		return true, "cancelled at a step boundary, before the next step ran"
	}
	return false, ""
}

// Resume is the most important function in this package.
//
// It runs at startup over every job the store holds that is not terminal, and
// decides, per job, between three outcomes:
//
//   - the interrupted step is verifiable and its paired question says the side
//     effect happened: skip it and carry on;
//   - the interrupted step is verifiable and says it did not: run it;
//   - the interrupted step is not verifiable: **park the job** and say what a
//     person has to check.
//
// It never guesses. A job whose kind this binary does not know is parked too,
// for the same reason: the steps it would run are not the steps it was
// submitted with.
func (e *Engine) Resume(ctx context.Context) error {
	all, err := e.deps.Store.Jobs().List(ctx)
	if err != nil {
		return err
	}

	for _, j := range all {
		if j.State.Terminal() || j.State == model.JobParked {
			continue
		}

		e.mu.Lock()
		build, known := e.builders[j.Kind]
		e.mu.Unlock()

		if !known {
			e.park(ctx, j, fmt.Sprintf(
				"this build has no steps for a %s job, so it cannot be continued. "+
					"Check the node and close the job by hand.", j.Kind))
			continue
		}

		steps, err := build(j)
		if err != nil || len(steps) != len(j.Steps) {
			e.park(ctx, j, "the job's steps could not be rebuilt as they were submitted, "+
				"so continuing would run something other than what was asked for.")
			continue
		}

		idx := j.Current
		if idx < 0 || idx >= len(steps) {
			e.park(ctx, j, "the job's recorded position is outside its own step list.")
			continue
		}

		// A job that was interrupted between steps -- its current step is
		// pending or done -- is simply continued. The dangerous case is the
		// one below it.
		if j.Steps[idx].State != model.StepRunning {
			if j.Steps[idx].State == model.StepDone || j.Steps[idx].State == model.StepSkipped {
				idx++
			}
			if idx >= len(steps) {
				j.State = model.JobSucceeded
				j.FinishedAt = e.deps.Now().UTC()
				_ = e.save(ctx, j)
				continue
			}
			j.Current = idx
			e.resumeJob(ctx, j, steps)
			continue
		}

		// Interrupted *inside* a step. The stored Verifiable flag decides, not
		// a fresh look at the step: the question is what was true when the
		// process died.
		if !j.Steps[idx].Verifiable {
			e.park(ctx, j, fmt.Sprintf(
				"holzkube-manager was interrupted while running %q, and that step cannot be checked "+
					"after the fact. It may or may not have taken effect. Look at the node, "+
					"then close or re-submit this job.", j.Steps[idx].Name))
			continue
		}

		happened, err := steps[idx].Happened(ctx, &j)
		if err != nil {
			e.park(ctx, j, fmt.Sprintf(
				"holzkube-manager was interrupted while running %q and could not find out whether it "+
					"took effect (%v). Look at the node, then close or re-submit this job.",
				j.Steps[idx].Name, err))
			continue
		}

		if happened {
			j.Steps[idx].State = model.StepSkipped
			j.Steps[idx].Detail = "already in effect when holzkube-manager restarted"
			j.Steps[idx].FinishedAt = e.deps.Now().UTC()
			idx++
			if idx >= len(steps) {
				j.State = model.JobSucceeded
				j.FinishedAt = e.deps.Now().UTC()
				_ = e.save(ctx, j)
				continue
			}
		} else {
			j.Steps[idx].State = model.StepPending
		}
		j.Current = idx
		e.resumeJob(ctx, j, steps)
	}
	return nil
}

// resumeJob re-takes the lease and continues a job.
func (e *Engine) resumeJob(ctx context.Context, j model.Job, steps []Step) {
	if err := e.acquire(j.Cluster, j.ID); err != nil {
		e.park(ctx, j, "another job holds this cluster; resume it once that one is done.")
		return
	}
	j = e.save(ctx, j)
	e.start(j, steps)
}

// park stops a job and says what a person has to decide.
func (e *Engine) park(ctx context.Context, j model.Job, reason string) {
	j.State = model.JobParked
	j.ParkedReason = reason
	j = e.save(ctx, j)
	e.publish(j, "", "", reason)

	e.deps.Logger.Warn("job parked for a human",
		slog.String("job", string(j.ID)),
		slog.String("kind", string(j.Kind)),
		slog.String("reason", reason))
}

// Cancel asks a job to stop at its next step boundary.
func (e *Engine) Cancel(ctx context.Context, id model.JobID) (model.Job, error) {
	j, err := e.deps.Store.Jobs().Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return model.Job{}, ErrNotFound
		}
		return model.Job{}, err
	}
	if j.State.Terminal() {
		return model.Job{}, ErrNotCancellable
	}

	j.CancelRequested = true
	stored, err := e.deps.Store.Jobs().Put(ctx, j)
	if err != nil {
		return model.Job{}, err
	}

	// The stored flag is what the loop reads at the boundary; the context
	// cancel is what stops a step that is waiting on a node so the boundary
	// arrives promptly. Both, because either alone is slow or unreliable.
	e.mu.Lock()
	cancel := e.running[id]
	e.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return stored, nil
}

// Get returns one job.
func (e *Engine) Get(ctx context.Context, id model.JobID) (model.Job, error) {
	j, err := e.deps.Store.Jobs().Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return model.Job{}, ErrNotFound
		}
		return model.Job{}, err
	}
	return j, nil
}

// List returns every job, newest first.
func (e *Engine) List(ctx context.Context) ([]model.Job, error) {
	all, err := e.deps.Store.Jobs().List(ctx)
	if err != nil {
		return nil, err
	}
	for i, jj := range all {
		for k := i + 1; k < len(all); k++ {
			if all[k].CreatedAt.After(jj.CreatedAt) {
				all[i], all[k] = all[k], all[i]
				jj = all[i]
			}
		}
	}
	return all, nil
}

// acquire takes a cluster's lease (JOB-03).
func (e *Engine) acquire(cluster model.ClusterID, id model.JobID) error {
	if cluster == "" {
		// A job on a machine that belongs to no cluster has nothing to
		// serialise against. Refusing it would make a maintenance-mode machine
		// permanently untouchable.
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	if holder, taken := e.leases[cluster]; taken && holder != id {
		return fmt.Errorf("%w: %s is running", ErrClusterBusy, holder)
	}
	e.leases[cluster] = id
	return nil
}

func (e *Engine) release(cluster model.ClusterID, id model.JobID) {
	if cluster == "" {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.leases[cluster] == id {
		delete(e.leases, cluster)
	}
}

// save persists a job's execution state and returns the stored record.
//
// It is a read-modify-write rather than a bare Put, and the reason is a
// concrete bug rather than caution: a cancel arrives from an HTTP handler and
// writes CancelRequested to the same record, bumping its revision. The running
// job's copy is then stale, its next Put fails the compare-and-swap, and
// without this the job never records another state -- a cancelled job that
// runs forever.
//
// So the two writers own different fields and neither clobbers the other's.
// The engine owns where the job *is*; everything else -- the cancel flag
// today, whatever a later phase adds -- is taken from the stored record.
//
// A failed save is logged and not fatal, and that is a deliberate and
// uncomfortable choice: the job is already running, and refusing to continue
// because the progress record could not be written would abandon the operation
// half-done for a bookkeeping failure. What it costs is that the crash
// reasoning above is only as good as the last successful write, which is why
// the failure is logged loudly.
func (e *Engine) save(ctx context.Context, j model.Job) model.Job {
	// Two attempts. One retry covers the ordinary race -- a cancel landing
	// between the read and the write -- and a loop would be a spin against
	// whatever is writing faster than this.
	for attempt := range 2 {
		stored, err := e.deps.Store.Jobs().Get(ctx, j.ID)
		if err != nil {
			if !errors.Is(err, store.ErrNotFound) {
				e.deps.Logger.Error("could not read a job before saving it",
					slog.String("job", string(j.ID)), slog.Any("error", err))
				return j
			}
			// Not there yet: this is the first write.
			stored = j
			stored.Rev = 0
		}

		// The engine's fields onto the record everybody else has been writing.
		stored.State = j.State
		stored.Steps = j.Steps
		stored.Current = j.Current
		stored.StartedAt = j.StartedAt
		stored.FinishedAt = j.FinishedAt
		stored.ParkedReason = j.ParkedReason

		saved, err := e.deps.Store.Jobs().Put(ctx, stored)
		if err == nil {
			return saved
		}
		if errors.Is(err, store.ErrConflict) && attempt == 0 {
			continue
		}
		e.deps.Logger.Error("could not persist job progress",
			slog.String("job", string(j.ID)), slog.Any("error", err))
		return j
	}
	return j
}

// publish puts a progress entry on the job's topic.
func (e *Engine) publish(j model.Job, step string, stepState model.StepState, detail string) {
	if e.deps.Hub == nil {
		return
	}
	raw, err := json.Marshal(Progress{
		Job:       j.ID,
		State:     j.State,
		Step:      step,
		Index:     j.Current,
		Total:     len(j.Steps),
		StepState: stepState,
		Detail:    detail,
		At:        e.deps.Now().UTC(),
	})
	if err != nil {
		return
	}
	_, _ = e.deps.Hub.Publish(Topic(j.ID), raw)
}

// Close stops every running job and waits for them.
//
// The jobs are cancelled rather than abandoned, so a step that is waiting on a
// node returns instead of being killed mid-call -- and the record it leaves is
// "running", which is exactly what Resume is written to interpret.
func (e *Engine) Close() error {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return nil
	}
	e.closed = true
	cancels := make([]context.CancelFunc, 0, len(e.running))
	for _, cancel := range e.running {
		cancels = append(cancels, cancel)
	}
	e.mu.Unlock()

	for _, cancel := range cancels {
		cancel()
	}
	e.wg.Wait()
	return nil
}

func newJobID() (model.JobID, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("jobs: generate job id: %w", err)
	}
	return model.JobID(hex.EncodeToString(b[:])), nil
}
