package talos

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

// ErrorKind is what went wrong at the transport, in the three shapes that call
// for three different reactions.
//
// The distinction between unreachable and rejected is load-bearing and it is
// the reason this type exists at all. "The node is gone" and "the node said no"
// arrive as the same gRPC status code often enough that reading the code alone
// is wrong: a node whose etcd is down answers codes.Unavailable for every Etcd*
// RPC while Version keeps working, and a client that read that as a dead node
// would open its circuit breaker and turn one broken subsystem into a node-wide
// outage. It is the same argument internal/httpapi/problem.go makes for keeping
// "who are you" and "you may not" apart: two failures that call for two
// different reactions must not arrive under one name.
type ErrorKind int

const (
	// KindUnreachable means nothing answered: the connection was refused,
	// dropped or never made, so the request did not reach the node. It is the
	// only kind an allowlisted read is retried on.
	KindUnreachable ErrorKind = iota + 1

	// KindTimeout means the node accepted the connection and then did not
	// answer inside the call's deadline. It is a transport failure too -- the
	// node did not answer -- but it points at load or a wedged service rather
	// than at the address, which is why it carries its own contract code.
	KindTimeout

	// KindRejected means the node answered and the answer was a refusal. It is
	// never a transport failure: the request arrived, was understood and was
	// declined, so retrying it produces the same refusal and the breaker has
	// nothing to protect against. The upstream status travels on Error.Status.
	KindRejected
)

// errorKinds is every kind, in declaration order.
//
// Go cannot enumerate the constants of a type, so this is the enumeration, and
// TestErrorKindMapping walks it. A kind added to the block above without a row
// here is a kind with no decided contract code, which is exactly the "anonymous
// internal error" this file exists to prevent -- so the test fails on the
// omission rather than on its consequences three phases later.
var errorKinds = []ErrorKind{KindUnreachable, KindTimeout, KindRejected}

// ErrorKinds returns every classified transport failure kind.
func ErrorKinds() []ErrorKind { return append([]ErrorKind(nil), errorKinds...) }

func (k ErrorKind) String() string {
	switch k {
	case KindUnreachable:
		return "unreachable"
	case KindTimeout:
		return "timeout"
	case KindRejected:
		return "rejected"
	default:
		return fmt.Sprintf("ErrorKind(%d)", int(k))
	}
}

// The reserved RFC 9457 code tokens of the upstream family's node half.
//
// They are spelled as literals here and not imported, because internal/talos
// sits below HTTP and importing internal/httpapi would invert the layering --
// the shape that later becomes an import cycle. The duplication is held honest
// from the other side: errors_test.go imports internal/httpapi, asserts these
// equal httpapi.CodeUpstreamNodeUnreachable and httpapi.CodeUpstreamNodeTimeout
// by identifier, and reads problem.go for any third node token that has no kind
// producing it.
const (
	codeNodeUnreachable = "upstream.node-unreachable"
	codeNodeTimeout     = "upstream.node-timeout"
)

// ProblemCode returns the reserved contract code token for this kind, and false
// when the kind deliberately has none.
//
// KindRejected has none. The upstream family means "an upstream dependency did
// not answer" and carries HTTP 502; a node that heard the request and declined
// it did answer, and reporting that as a 502 would send an operator to inspect
// a network that is fine. What a refusal becomes at the HTTP edge is Phase 6's
// decision, made with the route that first drives a node call in front of it.
func (k ErrorKind) ProblemCode() (string, bool) {
	switch k {
	case KindUnreachable:
		return codeNodeUnreachable, true
	case KindTimeout:
		return codeNodeTimeout, true
	case KindRejected:
		return "", false
	default:
		return "", false
	}
}

// Error is a classified failure of one node call.
//
// It names the operation and the machine, and it deliberately does not name the
// node's address. This value travels outward: Phase 6 turns it into an RFC 9457
// problem document that reaches a browser, and gRPC's own diagnostic for an
// unreachable node embeds the address it tried to dial. That is the same class
// of string audit.ChainStatus.Public() strips before it leaves the process, and
// an operator identifies a node by its machine id anyway -- the address is a
// hint, and a hint is not worth disclosing.
//
// The cause is still reachable through Unwrap for whoever is holding a
// debugger. It is the rendered message that is address-free.
type Error struct {
	// Op is the short gRPC method name the failure happened in, for example
	// "EtcdMemberList". A transport failure whose only record says nothing
	// happened is the one that is hardest to diagnose.
	Op string

	// Machine is the node the call was addressed to, by identity.
	Machine model.MachineID

	// Kind is the classification.
	Kind ErrorKind

	// Status is the upstream gRPC status code. It is meaningful for
	// KindRejected, where it is the node's own verdict; for the other kinds it
	// is whatever gRPC produced and is kept for diagnosis rather than for
	// branching.
	Status codes.Code

	// Err is the cause.
	Err error
}

func (e *Error) Error() string {
	switch e.Kind {
	case KindRejected:
		// The node's own words. They are what makes a refusal actionable --
		// reject_apply is only useful if the operator can read why the
		// configuration was refused -- and they cannot contain the address
		// holzkube-manager dialled, because they were written on the other side of it.
		return fmt.Sprintf("talos: %s on %s was rejected by the node (%s): %s",
			e.Op, e.Machine, e.Status, upstreamMessage(e.Err))
	case KindTimeout:
		return fmt.Sprintf("talos: %s on %s timed out", e.Op, e.Machine)
	case KindUnreachable:
		return fmt.Sprintf("talos: %s on %s: the node is unreachable", e.Op, e.Machine)
	default:
		return fmt.Sprintf("talos: %s on %s failed (%s)", e.Op, e.Machine, e.Kind)
	}
}

func (e *Error) Unwrap() error { return e.Err }

// upstreamMessage is the node's message, without gRPC's "rpc error: code = ..."
// framing, which repeats what Status already carries.
func upstreamMessage(err error) string {
	if st, ok := status.FromError(err); ok {
		return st.Message()
	}
	if err == nil {
		return "no detail"
	}
	return err.Error()
}

// classify is the single place a failure becomes a typed one, so that every
// call path classifies identically.
//
// answered is what separates KindUnreachable from KindRejected, and it is not
// derivable from the status code: a node whose etcd is down returns
// codes.Unavailable from a connection that is open and working. It is supplied
// by the caller, which observes whether the RPC produced response trailers --
// a server that answered always sends them, and a connection that never
// reached one never does.
//
// A call the caller cancelled is deliberately not classified at all: it is
// returned as it came. Cancellation says nothing about the node -- it says the
// caller changed their mind -- and turning it into a transport failure would
// let one cancelled fan-out open the circuit breaker of every healthy node in
// the inventory.
func classify(ctx context.Context, op string, machine model.MachineID, answered bool, err error) error {
	if err == nil {
		return nil
	}

	var already *Error
	if errors.As(err, &already) {
		return err
	}

	// A refusal this package made before the call reached the wire is not a
	// fact about the node, so it is returned as it came. Classified, ErrDryRun
	// would arrive as KindUnreachable -- there are no response trailers,
	// because nothing was sent -- and a process started with --dry-run would
	// then open the circuit breaker of every node it declined to mutate, on
	// the strength of calls that never happened. ErrorKindOf's doc already
	// promises that a refusal from this package before the wire is not a
	// classified transport failure; this is where that is true.
	if errors.Is(err, ErrDryRun) {
		return err
	}

	if ctx != nil && errors.Is(ctx.Err(), context.Canceled) {
		return err
	}
	if errors.Is(err, context.Canceled) {
		return err
	}

	e := &Error{Op: op, Machine: machine, Err: err, Status: codes.Unknown}
	if st, ok := status.FromError(err); ok {
		e.Status = st.Code()
	}

	switch {
	case errors.Is(err, context.DeadlineExceeded), e.Status == codes.DeadlineExceeded:
		e.Kind = KindTimeout
	case !answered:
		e.Kind = KindUnreachable
	default:
		e.Kind = KindRejected
	}

	return e
}

// ErrorKindOf reports how a failure was classified, and false when the error is
// not a classified transport failure at all -- a cancellation, a refusal from
// this package before the wire, or anything else.
//
// It exists so the breaker and the retry loop ask one question in one way
// instead of each repeating an errors.As.
func ErrorKindOf(err error) (ErrorKind, bool) {
	var te *Error
	if errors.As(err, &te) {
		return te.Kind, true
	}
	return 0, false
}
