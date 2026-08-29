package talos

// Deadline policy (D-04, TRANS-04).
//
// The policy is expressed as a table keyed by gRPC full method name rather than
// as constants scattered across call sites, for the reason
// internal/config/config.go's optionTable gives: a policy is only reviewable if
// it can be read in one place, and a per-entry rationale is only worth writing
// where the entry is.
//
// The classes and their memberships were confirmed by the operator on
// 2026-08-29 (02-CONTEXT.md <deadline_policy>, plan 02-05 task 1, option-a).
// They were derived from the real 54-method MachineServiceServer surface at
// machinery v1.13.9 rather than invented, and plan 02-03's ExpectedClient
// column was derived from them in turn -- so changing a value here changes
// internal/talossim/scenario.go and docs/talossim.md in the same commit.

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"time"
)

// DeadlineClass is how long a call of a given shape is allowed to take.
type DeadlineClass int

const (
	// ClassProbe is the D-05 liveness check: a Version call made to turn
	// "constructed" into "reachable". It is the same RPC as a fast read under a
	// shorter budget, so it is not a table row -- it is selected explicitly by
	// the probe, which is the only caller that means it.
	ClassProbe DeadlineClass = iota + 1

	// ClassFastRead is a call the node answers out of state it already has.
	ClassFastRead

	// ClassMutation is a call that starts work. The budget bounds the call and
	// not the operation: the long work is asynchronous on the node, so an
	// upgrade that takes four minutes is still a thirty-second call.
	ClassMutation

	// ClassStream carries no total deadline at all, because a stream that is
	// still delivering data has not failed. It is bounded instead by
	// StreamFirstByteDeadline and StreamIdleTimeout, which is the difference
	// between "this is taking a long time" and "nothing is coming".
	ClassStream
)

// The confirmed deadlines.
const (
	// ProbeDeadline bounds the liveness check. Deliberately short: the probe
	// exists to turn "constructed" into "reachable", and a caller that waits a
	// minute to be told a node is down has been given the worst of both
	// answers.
	ProbeDeadline = 5 * time.Second

	// FastReadDeadline bounds a read the node answers from state it holds.
	FastReadDeadline = 10 * time.Second

	// MutationDeadline bounds the call that *initiates* a mutation. It is not
	// how long the mutation may take: Talos performs the work asynchronously
	// and answers as soon as it has accepted the instruction, so a deadline
	// scaled to the work would be a deadline scaled to the wrong thing.
	MutationDeadline = 30 * time.Second

	// StreamFirstByteDeadline is how long a stream may take to produce anything
	// at all. Opening a stream against a node that has gone silent is
	// indistinguishable from opening one against a node with nothing to say
	// until the first byte arrives, so this is the bound that tells them apart.
	StreamFirstByteDeadline = 10 * time.Second

	// StreamIdleTimeout is how long a stream may go without data once it has
	// started. It measures the node sending nothing while the caller is waiting
	// for something -- never a caller that has simply stopped reading, whose
	// backpressure is not a fault.
	StreamIdleTimeout = 60 * time.Second
)

// Deadline is the class's total budget, and zero for the stream class.
func (c DeadlineClass) Deadline() time.Duration {
	switch c {
	case ClassProbe:
		return ProbeDeadline
	case ClassFastRead:
		return FastReadDeadline
	case ClassMutation:
		return MutationDeadline
	case ClassStream:
		return 0
	default:
		return 0
	}
}

func (c DeadlineClass) String() string {
	switch c {
	case ClassProbe:
		return "probe"
	case ClassFastRead:
		return "fast read"
	case ClassMutation:
		return "mutation"
	case ClassStream:
		return "stream"
	default:
		return fmt.Sprintf("DeadlineClass(%d)", int(c))
	}
}

var (
	// ErrNoDeadline is returned when a call would go to the wire on a context
	// with no deadline.
	//
	// It is a refusal and it is never retryable. There is no value that
	// disables it and there is no default-less path through this package: a
	// Talos RPC that can block forever holds a connection and an operator's
	// attention for as long as the process lives, and the fix is a deadline at
	// the call site rather than a retry here. WithClassDeadline is how a caller
	// gets one without having to know the number.
	ErrNoDeadline = errors.New("talos: refusing a call with no deadline")

	// ErrUnclassifiedMethod is returned for an RPC that has no entry in the
	// class table.
	//
	// An unclassified RPC is a gap in the specification, not a candidate for a
	// default. A default would mean the first caller of a new mutating RPC
	// silently inherits whatever number happened to be convenient, and would
	// mean nobody ever has to decide whether it may be retried -- which is the
	// decision the allowlist in retry.go exists to force.
	ErrUnclassifiedMethod = errors.New("talos: RPC has no deadline class")
)

// The gRPC service prefixes. COSI is served by cosi-project's own descriptor
// over the same connection as the two Talos services, because that is the one
// connection machinery's COSI adapter dials.
const (
	machineService = "/machine.MachineService/"
	storageService = "/storage.StorageService/"
	cosiService    = "/cosi.resource.State/"
)

// The full method names this package itself calls, plus the three deliberate
// retry exclusions, which are named rather than spelled out at their use sites.
const (
	MethodVersion            = machineService + "Version"
	MethodHostname           = machineService + "Hostname"
	MethodServiceList        = machineService + "ServiceList"
	MethodEtcdMemberList     = machineService + "EtcdMemberList"
	MethodEtcdStatus         = machineService + "EtcdStatus"
	MethodApplyConfiguration = machineService + "ApplyConfiguration"
	MethodBootstrap          = machineService + "Bootstrap"
	MethodReboot             = machineService + "Reboot"
	MethodLogs               = machineService + "Logs"
	MethodEtcdSnapshot       = machineService + "EtcdSnapshot"
	MethodPacketCapture      = machineService + "PacketCapture"
	MethodDiskUsage          = machineService + "DiskUsage"
	MethodDisks              = storageService + "Disks"
	MethodCOSIGet            = cosiService + "Get"
	MethodCOSIList           = cosiService + "List"
	MethodCOSIWatch          = cosiService + "Watch"
)

// deadlineClasses is the confirmed policy, in one place, reviewable.
//
// A method absent from this map cannot be called: WithClassDeadline and the
// call path both refuse it by name. That is deliberate and it is the property
// T-02-33 asks for -- an RPC that silently took a default deadline would be a
// repudiation risk, because the record would not say what bound the call.
var deadlineClasses = map[string]DeadlineClass{
	// Fast read: the node answers out of state it already holds.
	MethodVersion:                         ClassFastRead,
	MethodHostname:                        ClassFastRead,
	MethodServiceList:                     ClassFastRead,
	machineService + "Memory":             ClassFastRead,
	machineService + "LoadAvg":            ClassFastRead,
	machineService + "SystemStat":         ClassFastRead,
	machineService + "CPUInfo":            ClassFastRead,
	machineService + "CPUFreqStats":       ClassFastRead,
	machineService + "DiskStats":          ClassFastRead,
	machineService + "NetworkDeviceStats": ClassFastRead,
	machineService + "Mounts":             ClassFastRead,
	machineService + "Netstat":            ClassFastRead,
	machineService + "Processes":          ClassFastRead,
	machineService + "Stats":              ClassFastRead,
	machineService + "Containers":         ClassFastRead,
	MethodEtcdMemberList:                  ClassFastRead,
	MethodEtcdStatus:                      ClassFastRead,
	machineService + "EtcdAlarmList":      ClassFastRead,
	MethodCOSIGet:                         ClassFastRead,

	// COSI List is a server stream in the protocol and a bounded read in
	// meaning, so it takes the fast read budget as a *total* deadline. A
	// resource listing that has not finished in ten seconds is not a stream
	// that is still working.
	MethodCOSIList: ClassFastRead,

	// StorageService.Disks is the one entry the confirmed policy does not name.
	// The policy was derived from the MachineService surface, and Disks lives
	// on StorageService -- but D-06 puts it in MaintenanceClient's method set,
	// so leaving it unclassified would refuse the one call maintenance mode
	// exists to support. It is a cheap idempotent read, which is what the fast
	// read class is.
	MethodDisks: ClassFastRead,

	// Mutation: thirty seconds to *initiate*. Never retried.
	MethodApplyConfiguration:                 ClassMutation,
	MethodBootstrap:                          ClassMutation,
	machineService + "Reset":                 ClassMutation,
	MethodReboot:                             ClassMutation,
	machineService + "Shutdown":              ClassMutation,
	machineService + "Upgrade":               ClassMutation,
	machineService + "Rollback":              ClassMutation,
	machineService + "MetaWrite":             ClassMutation,
	machineService + "MetaDelete":            ClassMutation,
	machineService + "ServiceStart":          ClassMutation,
	machineService + "ServiceStop":           ClassMutation,
	machineService + "ServiceRestart":        ClassMutation,
	machineService + "ImagePull":             ClassMutation,
	machineService + "EtcdLeaveCluster":      ClassMutation,
	machineService + "EtcdRemoveMemberByID":  ClassMutation,
	machineService + "EtcdForfeitLeadership": ClassMutation,
	machineService + "EtcdDefragment":        ClassMutation,
	machineService + "EtcdRecover":           ClassMutation,
	machineService + "EtcdDowngradeEnable":   ClassMutation,
	machineService + "EtcdDowngradeValidate": ClassMutation,
	machineService + "EtcdDowngradeCancel":   ClassMutation,

	// Stream: no total deadline, a first-byte deadline and an idle timeout
	// instead. Never retried -- a retry restarts a partially consumed stream,
	// silently duplicating or dropping data.
	MethodLogs:                   ClassStream,
	machineService + "Dmesg":     ClassStream,
	machineService + "Events":    ClassStream,
	machineService + "Read":      ClassStream,
	machineService + "Copy":      ClassStream,
	machineService + "List":      ClassStream,
	MethodDiskUsage:              ClassStream,
	machineService + "ImageList": ClassStream,
	MethodEtcdSnapshot:           ClassStream,
	MethodPacketCapture:          ClassStream,
	MethodCOSIWatch:              ClassStream,
}

// DeadlineClasses returns a copy of the class table, so a reviewer -- and
// TestClassTable -- can read the whole policy rather than infer it.
func DeadlineClasses() map[string]DeadlineClass {
	return maps.Clone(deadlineClasses)
}

// ClassOf reports the deadline class of a gRPC full method name.
func ClassOf(method string) (DeadlineClass, bool) {
	c, ok := deadlineClasses[method]
	return c, ok
}

// WithClassDeadline applies the confirmed class deadline for a method.
//
// It is the way a caller gets a correct deadline without having to know the
// number, and it is a ceiling rather than a floor: a caller who already has a
// shorter budget keeps it, because context.WithTimeout takes the earlier of the
// two. A method with no entry in the class table is refused rather than given a
// default.
//
// A stream method gets a cancellable context and no total deadline: the bound
// on a stream is StreamFirstByteDeadline and StreamIdleTimeout, applied by the
// call path itself, and a total deadline would kill a stream that was working.
func WithClassDeadline(ctx context.Context, method string) (context.Context, context.CancelFunc, error) {
	class, ok := ClassOf(method)
	if !ok {
		return nil, nil, fmt.Errorf("talos: %s: %w", method, ErrUnclassifiedMethod)
	}

	if class == ClassStream {
		streamCtx, cancel := context.WithCancel(ctx)
		return streamCtx, cancel, nil
	}

	callCtx, cancel := context.WithTimeout(ctx, class.Deadline())
	return callCtx, cancel, nil
}

// requireDeadline is the structural gate.
//
// It is the half of D-04 that must not be relaxed. The numbers above are
// constants and an operator may reasonably want them different; that a call
// cannot be issued without one of them is not negotiable, and there is no
// value, flag or context key that turns this off.
func requireDeadline(ctx context.Context) error {
	if _, ok := ctx.Deadline(); !ok {
		return ErrNoDeadline
	}
	return nil
}

// probeClassKey marks a context as carrying the probe budget.
//
// The probe is the one place a method's class is not derivable from its name:
// Version is a fast read at ten seconds and a liveness check at five, and it is
// the same RPC either way. Rather than duplicate the method under a second name
// -- which would put a lie in the class table -- the caller that means the
// liveness check says so on the context, and only the two constructors do.
type probeClassKey struct{}

func withProbeClass(ctx context.Context) context.Context {
	return context.WithValue(ctx, probeClassKey{}, true)
}

func isProbeClass(ctx context.Context) bool {
	v, _ := ctx.Value(probeClassKey{}).(bool)
	return v
}
