package talos

// Retry policy (D-04, TRANS-04, T-02-28).
//
// The direction of this table is the whole decision, and it is the direction
// internal/audit/redact.go argues for its redaction allowlist. A denylist of
// non-idempotent RPCs forgets the next mutating RPC a later phase adds -- and
// what it forgets is retried. A retried Bootstrap whose first attempt already
// committed is precisely the second_bootstrap_returns_AlreadyExists scenario
// TRANS-07 names, and a retried Reset is worse. A forgotten allowlist entry, by
// contrast, costs a retry nobody gets.
//
// So: retried is the exception, written down once, derived from the fast read
// class, and asserted against that class by TestRetryAllowlistIsExactlyTheFastReadClass.

import (
	"context"
	"crypto/rand"
	"math/big"
	"time"
)

// The confirmed retry policy.
const (
	// RetryAttempts is the total number of attempts, so two retries.
	RetryAttempts = 3

	// RetryBackoffBase is the bound on the first retry's delay. The delay is
	// drawn uniformly from [0, base] -- full jitter, not partial -- so that a
	// fleet of callers that failed against the same node at the same instant
	// does not come back at the same instant and fail it again.
	RetryBackoffBase = 200 * time.Millisecond

	// RetryBackoffCeiling is where the doubling stops. With RetryAttempts at 3
	// the two delays drawn are bounded by 200 ms and 400 ms, so nothing reaches
	// this today; it is the policy's stated upper bound and it is what starts
	// mattering the day somebody raises the attempt count.
	RetryBackoffCeiling = 800 * time.Millisecond
)

// retryable is the allowlist: the fast read class, minus three deliberate
// exclusions.
//
// It is keyed by full method name so that it cannot accidentally match a
// same-named RPC on another service.
var retryable = map[string]bool{
	MethodVersion:                         true,
	MethodHostname:                        true,
	MethodServiceList:                     true,
	machineService + "Memory":             true,
	machineService + "LoadAvg":            true,
	machineService + "SystemStat":         true,
	machineService + "CPUInfo":            true,
	machineService + "CPUFreqStats":       true,
	machineService + "DiskStats":          true,
	machineService + "NetworkDeviceStats": true,
	machineService + "Mounts":             true,
	machineService + "Netstat":            true,
	machineService + "Processes":          true,
	machineService + "Stats":              true,
	machineService + "Containers":         true,
	MethodEtcdMemberList:                  true,
	MethodEtcdStatus:                      true,
	machineService + "EtcdAlarmList":      true,
	MethodCOSIGet:                         true,
	MethodCOSIList:                        true,
	MethodDisks:                           true,

	// The three deliberate exclusions. Each is a read, so the class is right;
	// each is listed here as false rather than left out, so the table shows the
	// decision instead of leaving it to be inferred from an absence.
	//
	// EtcdSnapshot copies the whole etcd database off the node. A retry storm
	// against a struggling control plane is its own outage, and the one thing
	// an operator taking a snapshot does not need is three of them.
	MethodEtcdSnapshot: false,

	// PacketCapture puts an interface into a capturing mode and streams every
	// frame. Two overlapping captures on a node under load is a second
	// incident.
	MethodPacketCapture: false,

	// DiskUsage walks a filesystem tree. It is a read, and it is the most
	// expensive read on the surface.
	MethodDiskUsage: false,
}

// Retryable reports whether an RPC may be retried at all.
//
// It answers false for anything it has not been told about, which is the point:
// an RPC nobody classified is an RPC nobody decided about, and the safe reading
// of an undecided mutation is "do not repeat it".
func Retryable(method string) bool { return retryable[method] }

// retryBackoff is the bound-respecting, fully jittered delay before attempt n.
//
// The doubling is a loop rather than a shift for the reason
// internal/auth/ratelimit.go gives: a large attempt count has to be arithmetic
// that cannot overflow into a negative duration, which would read as "no delay"
// and quietly undo the whole mechanism at exactly the point it matters most.
func retryBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}

	bound := RetryBackoffBase
	for range attempt - 1 {
		if bound >= RetryBackoffCeiling {
			bound = RetryBackoffCeiling
			break
		}
		bound *= 2
	}
	if bound > RetryBackoffCeiling {
		bound = RetryBackoffCeiling
	}

	n, err := rand.Int(rand.Reader, big.NewInt(int64(bound)+1))
	if err != nil {
		// The entropy source failing is not a reason to hammer a node without
		// waiting. Falling back to the full bound is the conservative direction:
		// it costs latency, never a retry storm.
		return bound
	}
	return time.Duration(n.Int64())
}

// do runs a call under the confirmed retry policy.
//
// Three properties, in the order they matter:
//
//  1. Only an allowlisted method is retried, and only on KindUnreachable. A
//     node that answered is not a transport failure, so a refusal is returned
//     as it came -- retrying it would produce the same refusal and would count
//     an answered call against the node's circuit breaker.
//
//  2. Every attempt runs inside the *original* deadline. The budget covers all
//     attempts; retries never extend it. Before sleeping, the loop asks what is
//     left and abandons rather than sleeping past it, so a caller who allowed
//     150 ms gets an answer in 150 ms and not in 150 ms plus a backoff.
//
//  3. The caller's cancellation ends the loop immediately and is returned
//     untouched, because a call the caller abandoned says nothing about the
//     node.
func (n *conn) do(ctx context.Context, method string, call func(context.Context) error) error {
	allowed := Retryable(method)

	var err error
	for attempt := 1; attempt <= RetryAttempts; attempt++ {
		err = call(ctx)
		if err == nil {
			return nil
		}

		if !allowed {
			return err
		}
		if attempt == RetryAttempts {
			return err
		}

		kind, ok := ErrorKindOf(err)
		if !ok || kind != KindUnreachable {
			return err
		}

		delay := retryBackoff(attempt)
		if !n.budgetAllows(ctx, delay) {
			return err
		}

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return err
		case <-timer.C:
		}
	}

	return err
}

// budgetAllows reports whether there is room inside the call's original
// deadline for this delay and for the attempt that would follow it.
//
// Sleeping right up to the deadline and then issuing an attempt that is
// guaranteed to expire immediately would be worse than not retrying: it turns a
// clean failure at the deadline into a confusing one, and it spends the whole
// remaining budget on a call nobody can answer.
func (n *conn) budgetAllows(ctx context.Context, delay time.Duration) bool {
	deadline, ok := ctx.Deadline()
	if !ok {
		// Unreachable in practice: requireDeadline has already refused a
		// deadline-less call by the time this runs. Returning false is the
		// conservative direction if that ever stops being true.
		return false
	}
	return delay < deadline.Sub(n.now())
}
