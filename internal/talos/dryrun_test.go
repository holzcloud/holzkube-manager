package talos_test

// The proof for D-03 / FOUND-12, in three parts.
//
// The requirement's wording is "keine Mutation erreicht einen Node". That is a
// claim about the node, so every assertion here that matters is made at the
// node: talossim's per-method call counter, which is incremented by a server
// interceptor before any handler runs and therefore counts a call that arrived
// and was refused just as readily as one that succeeded. An assertion made
// only on the returned error would pass for a gate that logged "would mutate"
// and issued the call anyway, which is precisely the advisory shape D-03
// forbids.

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/siderolabs/talos/pkg/machinery/api/machine"

	"github.com/holzcloud/holzkube/internal/talos"
	"github.com/holzcloud/holzkube/internal/talossim"
)

// mutationRPCs is the mutation class, read out of the class table rather than
// written down again here.
//
// This is the whole point of the enumeration. A second hand-maintained list of
// mutating methods would be a list that stops matching the table, and the
// entry it stops matching is a mutating RPC nobody proved was gated. Reading
// the table means a method a later phase classifies ClassMutation is covered
// on the day it is classified, with no edit here.
func mutationRPCs(t *testing.T) []string {
	t.Helper()

	var out []string
	for method, class := range talos.DeadlineClasses() {
		if class == talos.ClassMutation {
			out = append(out, method)
		}
	}

	// A table that produced nothing would make every assertion below vacuous.
	if len(out) < 15 {
		t.Fatalf("the class table names only %d mutating RPCs; the enumeration is not reading it", len(out))
	}

	// Map iteration order is random, and a failure that reports a different
	// order on every run is a failure that is harder to reproduce than to
	// cause.
	sort.Strings(out)
	return out
}

// streamingRPCs is the set of methods that are streams in the protocol,
// derived from the generated service descriptor.
//
// It is needed because the mutation class is not the same shape as the
// protocol's: EtcdRecover is a client stream and a mutation, so an enumeration
// that assumed "mutation implies unary" would skip the one method where the
// two halves of the gate could disagree.
func streamingRPCs() map[string]bool {
	out := map[string]bool{}
	prefix := "/" + machine.MachineService_ServiceDesc.ServiceName + "/"
	for _, sd := range machine.MachineService_ServiceDesc.Streams {
		out[prefix+sd.StreamName] = true
	}
	return out
}

func shortName(method string) string {
	if i := strings.LastIndexByte(method, '/'); i >= 0 {
		return method[i+1:]
	}
	return method
}

// callRPC issues one RPC by full method name through the shared call path and
// returns the error it produced.
//
// Streams are driven to a reply rather than merely opened. Opening a stream
// returns to the caller before the server has necessarily registered the call,
// so an assertion on the sim's counter made right after the open would be a
// race; waiting for the response makes the round trip a fact rather than a
// hope.
func callRPC(ctx context.Context, cc *talos.ClusterClient, method string, streaming map[string]bool) error {
	if !streaming[method] {
		return cc.RawInvoke(ctx, method, &emptypb.Empty{}, &emptypb.Empty{})
	}

	desc := &grpc.StreamDesc{
		StreamName:    shortName(method),
		ClientStreams: true,
		ServerStreams: true,
	}
	cs, err := cc.RawStream(ctx, desc, method)
	if err != nil {
		return err
	}
	if err := cs.CloseSend(); err != nil {
		return err
	}
	return cs.RecvMsg(&emptypb.Empty{})
}

func dryRunClient(t *testing.T, sim *talossim.Server, mode talos.Mode) *talos.ClusterClient {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	cc, err := talos.NewClusterClient(ctx, sim.Dialer(),
		simTarget(sim, "00000000-0000-0000-0000-0000000000d0"), sim.ClientCreds(), mode)
	if err != nil {
		t.Fatalf("NewClusterClient(dry-run=%v): %v", mode.DryRun, err)
	}
	t.Cleanup(func() {
		if err := cc.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return cc
}

// TestDryRunRefusesEveryMutationAtTheNode is part one: the enumeration.
//
// For every RPC the class table marks ClassMutation, the call is refused with
// ErrDryRun and the simulator's counter for that method stays at zero.
func TestDryRunRefusesEveryMutationAtTheNode(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "dry-run-node"})
	cc := dryRunClient(t, sim, talos.Mode{DryRun: true})
	streaming := streamingRPCs()

	methods := mutationRPCs(t)
	for _, method := range methods {
		ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
		err := callRPC(ctx, cc, method, streaming)
		cancel()

		if !errors.Is(err, talos.ErrDryRun) {
			t.Errorf("%s returned %v, which does not satisfy errors.Is(err, talos.ErrDryRun)", method, err)
		}
		if n := sim.Calls(shortName(method)); n != 0 {
			t.Errorf("%s reached the node %d time(s) while dry-run was on; "+
				"a refusal that still put the call on the wire is a refusal in name only", method, n)
		}
	}

	t.Logf("refused %d mutating RPCs, none of which reached the node", len(methods))
}

// TestDryRunOffLetsTheSameMutationsThrough is part two: the negative control.
//
// Without it, a gate that refused everything -- or a simulator that recorded
// nothing -- would pass part one and prove nothing. The calls are not expected
// to succeed: most of these RPCs answer Unimplemented on the simulator, and
// that is fine, because what is being asserted is that they arrived.
func TestDryRunOffLetsTheSameMutationsThrough(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "live-node"})
	cc := dryRunClient(t, sim, talos.Mode{})
	streaming := streamingRPCs()

	methods := mutationRPCs(t)
	for _, method := range methods {
		ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
		err := callRPC(ctx, cc, method, streaming)
		cancel()

		if errors.Is(err, talos.ErrDryRun) {
			t.Errorf("%s was refused as a dry run while dry-run was off", method)
		}
		if n := sim.Calls(shortName(method)); n != 1 {
			t.Errorf("%s reached the node %d time(s) with dry-run off, want 1 (error was %v)",
				method, n, err)
		}
	}

	t.Logf("delivered %d mutating RPCs to the node with dry-run off", len(methods))
}

// TestDryRunLeavesReadsAndStreamsAlone is the other half of the mode's meaning:
// it disables mutations, not the product.
func TestDryRunLeavesReadsAndStreamsAlone(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "dry-run-reads"})
	cc := dryRunClient(t, sim, talos.Mode{DryRun: true})

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	t.Run("a fast read answers with the node's real answer", func(t *testing.T) {
		host, err := cc.Hostname(ctx)
		if err != nil {
			t.Fatalf("Hostname under dry-run: %v", err)
		}
		if host != "dry-run-reads" {
			t.Errorf("Hostname returned %q, want the simulated node's own answer", host)
		}
		if n := sim.Calls("Hostname"); n != 1 {
			t.Errorf("Hostname reached the node %d time(s), want 1", n)
		}
	})

	t.Run("a stream opens and delivers", func(t *testing.T) {
		ls, err := cc.Logs(ctx, "kubelet")
		if err != nil {
			t.Fatalf("Logs under dry-run: %v", err)
		}
		if _, err := ls.Recv(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
			// A stream that produced data or ended cleanly is what is being
			// asserted; what must not happen is a dry-run refusal.
			if errors.Is(err, talos.ErrDryRun) {
				t.Fatalf("a log stream was refused as a mutation: %v", err)
			}
		}
		if n := sim.Calls("Logs"); n != 1 {
			t.Errorf("Logs reached the node %d time(s), want 1", n)
		}
	})
}

// TestDryRunRefusesApplyConfigurationInMaintenanceMode pins the exemption that
// is most tempting to make and most expensive to have made.
//
// Maintenance mode exists in order to apply a configuration to a node that has
// none. That is the single most consequential mutation the product performs,
// so a dry run that spared it would be a dry run that did not cover the call it
// matters most for.
func TestDryRunRefusesApplyConfigurationInMaintenanceMode(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "maintenance-node"})

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	mc, err := talos.NewMaintenanceClient(ctx, sim.Dialer(),
		simTarget(sim, "00000000-0000-0000-0000-0000000000d1"),
		talos.Creds{Kind: talos.CredMaintenance, TLS: sim.ClientCreds().TLS},
		talos.Mode{DryRun: true})
	if err != nil {
		t.Fatalf("NewMaintenanceClient: %v", err)
	}
	defer mc.Close() //nolint:errcheck // the test owns this client

	callCtx, callCancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer callCancel()

	if _, err := mc.ApplyConfiguration(callCtx, []byte("version: v1alpha1\n")); !errors.Is(err, talos.ErrDryRun) {
		t.Errorf("ApplyConfiguration returned %v, want a dry-run refusal", err)
	}
	if n := sim.Calls("ApplyConfiguration"); n != 0 {
		t.Errorf("ApplyConfiguration reached the node %d time(s) under dry-run", n)
	}

	// A maintenance read still works, for the same reason a cluster read does.
	if _, err := mc.Disks(callCtx); err != nil {
		t.Errorf("Disks under dry-run: %v", err)
	}
}

// TestDryRunRefusalNamesTheRPCAndTheWayOut is the message contract, in the
// shape tlsx.LoopbackGuard established: what was refused, why, and what would
// have to change.
func TestDryRunRefusalNamesTheRPCAndTheWayOut(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "dry-run-message"})
	cc := dryRunClient(t, sim, talos.Mode{DryRun: true})

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	err := cc.Bootstrap(ctx)
	if err == nil {
		t.Fatal("Bootstrap succeeded under dry-run")
	}

	msg := err.Error()
	for _, want := range []string{"Bootstrap", "dry-run", "no node is changed"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the refusal %q does not mention %q", msg, want)
		}
	}
}

// TestDryRunRefusalIsNotATransportFailure is why classify passes ErrDryRun
// through untouched.
//
// A refusal this process made before the call left it is not a fact about the
// node. Classified as one it would arrive as KindUnreachable -- there are no
// response trailers, because nothing was sent -- and a process started with
// --dry-run would then open the circuit breaker of every node it declined to
// mutate, on the strength of calls that never happened.
func TestDryRunRefusalIsNotATransportFailure(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "dry-run-classification"})
	cc := dryRunClient(t, sim, talos.Mode{DryRun: true})

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	err := cc.Reboot(ctx)
	if !errors.Is(err, talos.ErrDryRun) {
		t.Fatalf("Reboot returned %v, want a dry-run refusal", err)
	}
	if kind, ok := talos.ErrorKindOf(err); ok {
		t.Errorf("the dry-run refusal was classified as a %s transport failure; "+
			"the breaker would count a call that never happened against the node", kind)
	}
}
