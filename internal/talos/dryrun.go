package talos

// Dry-run enforcement (D-03, FOUND-12).
//
// FOUND-12's wording is "keine Mutation erreicht einen Node". That is a claim
// about the node, so it has to be provable at the node -- which decides where
// the enforcement lives. An HTTP middleware check would only ever see HTTP
// requests, and Phase 6's jobs engine drives mutations that no request
// initiated; a check in each wrapper method would be a rule that has to be
// remembered once per method, and the one somebody forgets is the one that
// reboots a production node. So the refusal sits on the gRPC client
// interceptor chain of the single shared connect path both client types are
// built on, as the innermost frame before the invoker: the last layer this
// process controls before the bytes leave it.
//
// It is deliberately *not* internal/httpapi's Destructive route flag. Those are
// two different properties -- ServiceRestart mutates and is not destructive,
// and D-06's Destructive marks the routes that need a fresh sudo window -- and
// conflating them would make the safety mode depend on a UI-facing
// classification.

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc"
)

// ErrDryRun reports that a mutating RPC was refused because the process is
// running in dry-run mode.
//
// It is a refusal and never a retryable condition. There is no value that
// disables it for one call, and there is no code path that logs "would mutate"
// and then issues the call anyway: an advisory dry-run is worse than none,
// because it is the mode an operator trusts while it is not protecting them.
// The only thing that changes it is restarting the process without --dry-run.
var ErrDryRun = errors.New("talos: refusing a mutating call in dry-run mode")

// Mode carries the process-level operating decisions the transport enforces on
// every call made through a client it built.
//
// It is a required constructor parameter rather than a variadic option or a
// package-level variable, for the reason deadline.go gives about default
// deadlines: a policy that can be omitted is a policy that will be omitted, and
// the call site that omits this one is the one that mutates a node while the
// operator believes nothing can. The zero value is the live mode, so dry-run is
// something a caller has to ask for and never something it inherits.
type Mode struct {
	// DryRun refuses every RPC the deadline class table marks ClassMutation,
	// before it reaches the wire. Reads and streams are unaffected: the mode
	// disables mutations, not the product.
	DryRun bool
}

// dryRunInterceptor is the pair of gRPC client interceptors that enforce
// Mode.DryRun.
//
// Membership of the mutation class is read from deadline.go's class table
// rather than from a second list of mutating methods kept here. Two lists is
// exactly how the next mutating RPC gets missed, and the class table is already
// the reviewed definition of which calls change a node. It also means an RPC
// nobody classified cannot slip past: WithClassDeadline and the call path both
// refuse an unclassified method by name (ErrUnclassifiedMethod), so a new RPC
// fails loudly instead of being waved through this gate as "not a mutation".
//
// Both interceptors are returned, and both are installed, whether or not
// dry-run is on. A gate that is only present in one of the two modes is a gate
// whose presence is itself a code path.
func dryRunInterceptor(enabled bool) (grpc.UnaryClientInterceptor, grpc.StreamClientInterceptor) {
	unary := func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		if err := refuseIfMutating(enabled, method); err != nil {
			return err
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}

	stream := func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		// A stream is a read and is permitted -- except when the class table
		// says otherwise. EtcdRecover is a client stream in the protocol and a
		// mutation in meaning, and a gate that reasoned "streams are reads"
		// from the protocol shape rather than from the class table would let it
		// through.
		if err := refuseIfMutating(enabled, method); err != nil {
			return nil, err
		}
		return streamer(ctx, desc, cc, method, opts...)
	}

	return unary, stream
}

// refuseIfMutating is the whole decision, in one place, so the unary and stream
// halves cannot drift.
func refuseIfMutating(enabled bool, method string) error {
	if !enabled {
		return nil
	}
	if class, ok := ClassOf(method); !ok || class != ClassMutation {
		return nil
	}

	// The refusal follows tlsx.LoopbackGuard's shape: what was refused, why,
	// and what would have to change. An operator reading this line should not
	// have to go and find out which flag they are in.
	return fmt.Errorf(
		"talos: refusing %s: it is a mutating RPC and this process was started with --dry-run, "+
			"so it is not issued and no node is changed; restart holzkube-managerd without --dry-run "+
			"(or with HOLZKUBE_MANAGER_DRY_RUN=false) to issue it: %w",
		shortMethod(method), ErrDryRun)
}
