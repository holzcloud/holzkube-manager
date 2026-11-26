package jobs

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// The three node actions, and what makes each of them safe to resume.
//
// Each is two steps: confirm the node is there, then do the thing. The first
// step exists so that a job against a node that is already gone fails with
// "the node did not answer" rather than with whatever the second step's RPC
// happens to say, and because it is the cheap check that catches a wrong
// target before anything irreversible runs.
//
// The second step is where the verifiability question bites, and the three
// answers are different:
//
//   - **Reboot** is verifiable. A node that rebooted has a boot id -- an
//     uptime that reset -- and asking is a read. An interrupted reboot is
//     therefore resumed by asking whether the node already came back.
//   - **Shutdown** is verifiable in one direction only: a node that answers
//     has not been shut down, and the step runs. A node that does not answer
//     proves nothing on resume -- Resume runs right after the daemon starts,
//     and on a Raspberry Pi that can be before its network is up -- so
//     silence is "cannot tell", which parks.
//   - **Reset** is **not verifiable**, and that is the whole reason the parked
//     state exists. Nothing a node can be asked distinguishes "this node was
//     wiped five seconds ago and is booting" from "this node is booting for
//     another reason", and the cost of guessing wrong is a second wipe.

// Connector opens a client to a machine. It is the inventory's Connect, passed
// as a function so this package does not depend on the inventory.
type Connector func(ctx context.Context, id model.MachineID) (*talos.ClusterClient, error)

// probeTimeout bounds the "is the node there" checks. It is the probe class's
// own budget: the question is liveness, and a node that has not answered in
// five seconds has answered.
const probeTimeout = 8 * time.Second

// NodeOption adjusts RegisterNodeActions.
type NodeOption func(*nodeOptions)

type nodeOptions struct {
	afterReboot func(model.MachineID)
}

// AfterReboot registers a hook that runs once a reboot job's node has taken the
// reboot -- or, on resume, was found to have rebooted already. It is how things
// that are only true until the next boot (a staged configuration) get cleared.
func AfterReboot(fn func(model.MachineID)) NodeOption {
	return func(o *nodeOptions) { o.afterReboot = fn }
}

// afterRebooted wraps a reboot step so that hook runs when the node is known to
// have rebooted. It adds no step: a job record written by an older build has two
// steps, and a rebuilt list of three would park it.
func afterRebooted(s Step, id model.MachineID, hook func(model.MachineID)) Step {
	if hook == nil {
		return s
	}
	do, happened := s.Do, s.Happened
	s.Do = func(ctx context.Context, j *model.Job) error {
		if err := do(ctx, j); err != nil {
			return err
		}
		hook(id)
		return nil
	}
	if happened != nil {
		s.Happened = func(ctx context.Context, j *model.Job) (bool, error) {
			ok, err := happened(ctx, j)
			if ok && err == nil {
				hook(id)
			}
			return ok, err
		}
	}
	return s
}

// RegisterNodeActions teaches an engine the three node actions.
func RegisterNodeActions(e *Engine, connect Connector, opts ...NodeOption) {
	var o nodeOptions
	for _, opt := range opts {
		opt(&o)
	}

	e.Register(model.JobReboot, func(j model.Job) ([]Step, error) {
		return []Step{
			ReachableStep(connect, j.Machine),
			afterRebooted(RebootStep(connect, j.Machine, false), j.Machine, o.afterReboot),
		}, nil
	})

	e.Register(model.JobShutdown, func(j model.Job) ([]Step, error) {
		return []Step{
			ReachableStep(connect, j.Machine),
			ShutdownStep(connect, j.Machine, false),
		}, nil
	})

	e.Register(model.JobReset, func(j model.Job) ([]Step, error) {
		opts, err := ResetOptionsFromParams(j.Params)
		if err != nil {
			return nil, err
		}

		return []Step{
			ReachableStep(connect, j.Machine),
			{
				Name: "wipe the node",
				Do: func(ctx context.Context, job *model.Job) error {
					ctx, cancel, err := talos.WithClassDeadline(ctx, talos.MethodReset)
					if err != nil {
						return err
					}
					defer cancel()

					cc, err := connect(ctx, job.Machine)
					if err != nil {
						return err
					}
					defer cc.Close() //nolint:errcheck // as above

					if err := cc.Reset(ctx, opts); err != nil {
						return err
					}
					job.Steps[job.Current].Detail = describeReset(opts)
					return nil
				},
				// Deliberately nil. Nothing a node can be asked separates
				// "wiped five seconds ago and booting" from "booting for
				// another reason", and the cost of being wrong is a second
				// wipe. An interruption here parks the job and a person
				// decides -- which is the outcome this engine was built to
				// produce.
				Happened: nil,
			},
		}, nil
	})
}

// RebootStep reboots one node, and knows afterwards whether it did.
//
// It is exported, with ShutdownStep and ReachableStep, because the power model
// (internal/power) walks the same three RPCs across a node and across a whole
// cluster, and a second copy of them there would be a second answer to "how do
// we know a reboot happened" -- the question this engine was built around. The
// machine is a parameter rather than read off the job so that one job can walk
// several nodes; the three node actions pass the job's own.
//
// force asks Talos's FORCE mode: no cordon, no drain, no graceful handover.
func RebootStep(connect Connector, id model.MachineID, force bool) Step {
	name, detail := "reboot the node", "the node accepted the reboot"
	if force {
		name, detail = "force-reboot the node", "the node accepted the reboot, without draining anything first"
	}
	return Step{
		Name: name,
		Do: func(ctx context.Context, job *model.Job) error {
			// The mutation class budget, applied here because a job's
			// context deliberately has none: a job outlives the request that
			// asked for it. Without this the call is refused outright by the
			// deadline gate -- which a test found, and which would otherwise
			// have been a reboot button that never worked.
			ctx, cancel, err := talos.WithClassDeadline(ctx, talos.MethodReboot)
			if err != nil {
				return err
			}
			defer cancel()

			cc, err := connect(ctx, id)
			if err != nil {
				return err
			}
			defer cc.Close() //nolint:errcheck // the verdict is the RPC's, not the close's

			if force {
				err = cc.RebootForced(ctx)
			} else {
				err = cc.Reboot(ctx)
			}
			if err != nil {
				return err
			}
			job.Steps[job.Current].Detail = detail
			return nil
		},
		// A node that has rebooted has been up for less time than the step has
		// been running. That is a read, and it is the difference between
		// resuming a reboot and rebooting twice.
		Happened: func(ctx context.Context, job *model.Job) (bool, error) {
			return RebootedSince(ctx, connect, id, job.Steps[job.Current].StartedAt)
		},
	}
}

// ShutdownStep powers one node off. force skips Talos's own cordon and drain;
// see talos.ClusterClient.ShutdownForced for the two callers that need that.
func ShutdownStep(connect Connector, id model.MachineID, force bool) Step {
	name := "shut the node down"
	if force {
		name = "force the node off"
	}
	return Step{
		Name: name,
		Do: func(ctx context.Context, job *model.Job) error {
			ctx, cancel, err := talos.WithClassDeadline(ctx, talos.MethodShutdown)
			if err != nil {
				return err
			}
			defer cancel()

			cc, err := connect(ctx, id)
			if err != nil {
				return err
			}
			defer cc.Close() //nolint:errcheck // as above

			if force {
				err = cc.ShutdownForced(ctx)
			} else {
				err = cc.Shutdown(ctx)
			}
			if err != nil {
				return err
			}
			job.Steps[job.Current].Detail = "the node accepted the shutdown; it will not come back on its own"
			return nil
		},
		// A node that answers has not been shut down: run the step. A node
		// that does not answer is what a shutdown looks like, and also what
		// every node looks like from a daemon that has just started and whose
		// network is not up yet. It used to be taken as "done", which wrote a
		// running node down as off. Silence is no evidence, so it parks.
		Happened: func(ctx context.Context, _ *model.Job) (bool, error) {
			if Reachable(ctx, connect, id) {
				return false, nil
			}
			return false, errors.New("the node does not answer. That is what a node that " +
				"was shut down looks like, and also what any node looks like while this " +
				"host's network is not up yet, so it is not proof the shutdown took effect")
		},
	}
}

// ReachableStep is the cheap check every action starts with.
func ReachableStep(connect Connector, id model.MachineID) Step {
	return Step{
		Name: "check the node answers",
		Do: func(ctx context.Context, job *model.Job) error {
			probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
			defer cancel()

			cc, err := connect(probeCtx, id)
			if err != nil {
				return fmt.Errorf("the node did not answer: %w", err)
			}
			defer cc.Close() //nolint:errcheck // the verdict is the probe's

			version, err := cc.Probe(probeCtx)
			if err != nil {
				return fmt.Errorf("the node did not answer: %w", err)
			}
			job.Steps[job.Current].Detail = "answering, Talos " + version
			return nil
		},
		// A read that changes nothing is idempotent by construction, so
		// repeating it after an interruption costs one RPC and risks nothing.
		// Saying "it has not happened" is therefore always safe here, and it
		// keeps the resume path from having to special-case a read-only step.
		Happened: func(context.Context, *model.Job) (bool, error) { return false, nil },
	}
}

// Reachable reports whether a node answers a liveness probe right now.
func Reachable(ctx context.Context, connect Connector, id model.MachineID) bool {
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	cc, err := connect(probeCtx, id)
	if err != nil {
		return false
	}
	defer cc.Close() //nolint:errcheck // a liveness probe's verdict is its own

	_, err = cc.Probe(probeCtx)
	return err == nil
}

// RebootedSince reports whether the node has been up for less time than the
// moment given, which is what a reboot after that moment looks like.
func RebootedSince(ctx context.Context, connect Connector, id model.MachineID, since time.Time) (bool, error) {
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	cc, err := connect(probeCtx, id)
	if err != nil {
		// A node that is not answering is a node that is very probably
		// rebooting right now. That is not an answer to "did it reboot",
		// though, and returning one would be the guess this whole mechanism
		// exists to avoid -- so it is reported as "cannot tell", which parks.
		return false, fmt.Errorf("the node is not answering, so it cannot say whether it rebooted: %w", err)
	}
	defer cc.Close() //nolint:errcheck // as above

	up, err := cc.Uptime(probeCtx)
	if err != nil {
		return false, err
	}
	if err := PlausibleElapsed(time.Since(since)); err != nil {
		return false, err
	}
	if up < 0 {
		return false, errors.New("the node's boot time is later than this host's clock, so " +
			"the two clocks disagree and whether it rebooted cannot be told")
	}
	// A node whose uptime is shorter than the age of the request has booted
	// since the request was made.
	return time.Since(since) > up, nil
}

// MaxTrustedElapsed is the longest time since a recorded moment that a
// resumed step believes. Longer is more likely a clock that jumped than a
// daemon that was away that long.
const MaxTrustedElapsed = 12 * time.Hour

// PlausibleElapsed says whether a time measured against a persisted wall-clock
// moment can be believed.
//
// The daemon runs on a Raspberry Pi, which has no clock that keeps time while
// it is off: at boot the clock is wherever it was saved, and it jumps when NTP
// answers. A step start recorded before such a jump, or compared after one, is
// a number that says nothing about how long ago it was. Negative is the clock
// having gone back; more than MaxTrustedElapsed is either a long absence or a
// jump, and the two cannot be told apart. Either is "not proven".
func PlausibleElapsed(elapsed time.Duration) error {
	switch {
	case elapsed < 0:
		return fmt.Errorf("this host's clock is %s earlier than when the step started, so it "+
			"went back (a clock set late at boot?) and the time since then cannot be measured",
			(-elapsed).Round(time.Second))
	case elapsed > MaxTrustedElapsed:
		return fmt.Errorf("the step started %s ago by this host's clock, which is more than the "+
			"%s a resumed step trusts: a clock that jumped looks the same as a long absence",
			elapsed.Round(time.Minute), MaxTrustedElapsed)
	}
	return nil
}

// ResetOptionsFromParams reads a reset's stored parameters.
//
// The parameters are read back rather than defaulted, and the difference
// matters more here than anywhere else in the product: `talosctl reset`
// defaults to `--wipe-mode=all --reboot=false`, the most destructive
// combination there is. A resumed reset that fell back to defaults would wipe
// more than was asked for, and leave the machine off.
func ResetOptionsFromParams(params map[string]string) (talos.ResetOptions, error) {
	mode := params["wipe_mode"]
	if mode == "" {
		return talos.ResetOptions{}, errors.New(
			"jobs: a reset must name its wipe mode; there is no default, because Talos's own " +
				"default wipes everything")
	}
	valid := false
	for _, m := range talos.ResetWipeModes() {
		if m == mode {
			valid = true
		}
	}
	if !valid {
		return talos.ResetOptions{}, fmt.Errorf("jobs: %q is not a wipe mode; one of %v",
			mode, talos.ResetWipeModes())
	}

	graceful, err := parseBoolParam(params, "graceful")
	if err != nil {
		return talos.ResetOptions{}, err
	}
	reboot, err := parseBoolParam(params, "reboot")
	if err != nil {
		return talos.ResetOptions{}, err
	}

	var disks []string
	if raw := params["user_disks"]; raw != "" {
		disks = strings.Split(raw, ",")
	}
	if mode == "user-disks" && len(disks) == 0 {
		return talos.ResetOptions{}, errors.New(
			"jobs: a user-disks reset must name the disks; naming none would wipe none, " +
				"which is not what the operator asked for")
	}

	return talos.ResetOptions{
		Mode:            mode,
		Graceful:        graceful,
		Reboot:          reboot,
		UserDisksToWipe: disks,
	}, nil
}

// parseBoolParam requires the flag to be present and explicit.
//
// A missing boolean would be false, and for a reset both of the booleans have
// a false value that is the dangerous one: not graceful risks etcd quorum, not
// rebooting leaves the machine off. There is no safe default, so there is no
// default.
func parseBoolParam(params map[string]string, name string) (bool, error) {
	raw, ok := params[name]
	if !ok || raw == "" {
		return false, fmt.Errorf("jobs: a reset must state %q explicitly; "+
			"its false value is the dangerous one and must be chosen rather than inherited", name)
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("jobs: %q is not a boolean for %q", raw, name)
	}
	return v, nil
}

// describeReset renders what a reset did, in the words an operator would use.
func describeReset(o talos.ResetOptions) string {
	var b strings.Builder
	switch o.Mode {
	case "all":
		b.WriteString("wiped every disk")
	case "system-disk":
		b.WriteString("wiped the system disk")
	case "user-disks":
		b.WriteString("wiped " + strings.Join(o.UserDisksToWipe, ", "))
	}
	if o.Graceful {
		b.WriteString(", after leaving etcd")
	} else {
		b.WriteString(", without leaving etcd first")
	}
	if o.Reboot {
		b.WriteString(", rebooting")
	} else {
		b.WriteString(", halting (the machine will not come back on its own)")
	}
	return b.String()
}
