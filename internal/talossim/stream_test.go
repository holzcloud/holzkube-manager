package talossim_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/cosi-project/runtime/pkg/safe"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/siderolabs/talos/pkg/machinery/api/common"
	"github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/resources/hardware"
	"github.com/siderolabs/talos/pkg/machinery/resources/network"

	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// TestProductionClientReadsSeededCOSIState is the COSI half of TRANS-06.
//
// It is deliberately routed through client.Client.COSI -- the unmodified
// production client's own state adapter -- rather than through the simulator's
// state value. Reading the seeded resource in-process would prove only that a
// map holds what was put in it. Reading it through the adapter proves the
// state server is registered on the connection the real client dials, encodes
// the resource the way that client decodes it, and is reachable with the
// credentials the product uses.
func TestProductionClientReadsSeededCOSIState(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "cosi-node", NodeIP: "10.4.0.7"})
	cl := newMachineryClient(t, sim)
	ctx := testContext(t)

	info, err := safe.StateGetByID[*hardware.SystemInformation](ctx, cl.COSI, hardware.SystemInformationID)
	if err != nil {
		t.Fatalf("read %s through the production client's COSI accessor: %v", hardware.SystemInformationType, err)
	}
	if got := info.TypedSpec().UUID; got == "" {
		t.Error("the seeded SystemInformation carries no UUID; STACK.md makes it the node's primary key")
	}
	if got := info.TypedSpec().SerialNumber; got != "SIM-cosi-node" {
		t.Errorf("SerialNumber = %q, want %q", got, "SIM-cosi-node")
	}

	addr, err := safe.StateGetByID[*network.NodeAddress](ctx, cl.COSI, network.NodeAddressDefaultID)
	if err != nil {
		t.Fatalf("read %s through the production client's COSI accessor: %v", network.NodeAddressType, err)
	}
	if len(addr.TypedSpec().Addresses) != 1 {
		t.Fatalf("the seeded NodeAddress carries %d addresses, want 1", len(addr.TypedSpec().Addresses))
	}
	if got := addr.TypedSpec().Addresses[0].Addr().String(); got != "10.4.0.7" {
		t.Errorf("node address = %q, want the address the node was configured with, %q", got, "10.4.0.7")
	}
}

// TestScenarioMutationOfCOSIStateIsVisibleToTheClient is the write half: the
// accessor a scenario seeds and removes through is the same one the production
// client reads.
//
// The k8s_down scenario in plan 02-03 works by removing resources from this
// state, so a removal that the client could not observe would make that
// scenario a no-op that still passed.
func TestScenarioMutationOfCOSIStateIsVisibleToTheClient(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "mutable-cosi"})
	cl := newMachineryClient(t, sim)
	ctx := testContext(t)

	seeded, err := safe.StateGetByID[*hardware.SystemInformation](ctx, sim.COSI(), hardware.SystemInformationID)
	if err != nil {
		t.Fatalf("read the seeded resource through Server.COSI: %v", err)
	}

	if err := sim.COSI().Destroy(ctx, seeded.Metadata()); err != nil {
		t.Fatalf("destroy through Server.COSI: %v", err)
	}

	if _, err := safe.StateGetByID[*hardware.SystemInformation](ctx, cl.COSI, hardware.SystemInformationID); err == nil {
		t.Fatal("the production client still reads a resource the scenario removed; " +
			"the accessor and the served state are not the same object")
	}
}

// TestLogsStreamsToCompletion pins the bounded half of T-02-08-01: a stream
// ends, and it ends after the number of messages the node was configured for.
func TestLogsStreamsToCompletion(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "log-node", StreamMessages: 5})
	cl := newMachineryClient(t, sim)
	ctx := testContext(t)

	stream, err := cl.Logs(ctx, "system", common.ContainerDriver_CONTAINERD, "apid", false, -1)
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}

	got := drainData(t, stream)
	if got != 5 {
		t.Errorf("Logs delivered %d messages, want the configured 5", got)
	}
}

// TestDmesgAndEventsStream covers the other two streams. Events is separated
// out because its payload is a typed Any rather than opaque bytes, so a
// failure to build that payload would not show up in the Logs test.
func TestDmesgAndEventsStream(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "stream-node", StreamMessages: 3})
	cl := newMachineryClient(t, sim)
	ctx := testContext(t)

	dmesg, err := cl.Dmesg(ctx, false, true)
	if err != nil {
		t.Fatalf("Dmesg: %v", err)
	}
	if got := drainData(t, dmesg); got != 3 {
		t.Errorf("Dmesg delivered %d messages, want 3", got)
	}

	events, err := cl.MachineClient.Events(ctx, &machine.EventsRequest{})
	if err != nil {
		t.Fatalf("Events: %v", err)
	}

	count := 0
	for {
		ev, err := events.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Events.Recv: %v", err)
		}
		count++

		if ev.GetMetadata().GetHostname() != "stream-node" {
			t.Errorf("event answered by %q, want %q", ev.GetMetadata().GetHostname(), "stream-node")
		}

		payload, err := ev.GetData().UnmarshalNew()
		if err != nil {
			t.Fatalf("unmarshal the event payload: %v", err)
		}
		if _, ok := payload.(*machine.MachineStatusEvent); !ok {
			t.Errorf("event payload is %T, want *machine.MachineStatusEvent", payload)
		}
	}
	if count != 3 {
		t.Errorf("Events delivered %d messages, want 3", count)
	}
}

// TestCancelledStreamTearsDownPromptly is the cancellation half of
// T-02-08-01.
//
// A stream that keeps producing after its caller has gone is a leak, and in a
// test suite that opens one per scenario it is a leak that accumulates for the
// life of the process. The bound is one second: long enough not to be a timing
// test, short enough that a stream ignoring cancellation cannot pass.
func TestCancelledStreamTearsDownPromptly(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "cancel-node", StreamMessages: 1_000_000})
	cl := newMachineryClient(t, sim)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	stream, err := cl.Logs(ctx, "system", common.ContainerDriver_CONTAINERD, "apid", true, -1)
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}

	if _, err := stream.Recv(); err != nil {
		t.Fatalf("first Recv: %v", err)
	}

	cancel()

	done := make(chan error, 1)
	go func() {
		for {
			if _, err := stream.Recv(); err != nil {
				done <- err
				return
			}
		}
	}()

	select {
	case err := <-done:
		if status.Code(err) != codes.Canceled && !errors.Is(err, context.Canceled) {
			t.Errorf("the stream ended with %v, want a cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("the stream was still running a second after the caller cancelled it")
	}
}

// TestCancellingAStreamStopsTheEmitter is the half of cancellation the wire
// test cannot see.
//
// A handler that returns when its caller goes away still leaves the producer
// behind it blocked on a send nobody will ever take, and from the client side
// that leak is indistinguishable from a clean teardown -- the RPC ends either
// way. So the emitter is checked directly: cancelling the context must close
// the channel, which is the only observable that separates "the goroutine
// stopped" from "the goroutine is parked forever".
func TestCancellingAStreamStopsTheEmitter(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{StreamMessages: 1_000_000})

	ctx, cancel := context.WithCancel(t.Context())
	ch := sim.Streams().Open(ctx, "cancelled")

	if _, ok := <-ch; !ok {
		t.Fatal("the emitter closed before delivering anything")
	}

	cancel()

	// After cancellation the emitter may still hand over the one message it
	// had already offered, and then it must close. Anything beyond that is a
	// producer that is not reading its context.
	deadline := time.After(time.Second)
	delivered := 0

	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
			delivered++
			if delivered > 1 {
				t.Fatalf("the emitter delivered %d further messages after cancellation; "+
					"it is not reading the stream context and its goroutine outlives the stream", delivered)
			}
		case <-deadline:
			t.Fatal("the emitter had not closed a second after its context was cancelled; " +
				"the producer goroutine outlives the stream it belongs to")
		}
	}
}

// TestStalledConsumerBlocksTheEmitter is the memory half of T-02-08-01.
//
// The assertion is made against the emitter directly rather than through gRPC
// on purpose: gRPC has its own flow-control window, so a test driven through
// the wire would be asserting on that window's size rather than on whether the
// simulator buffers. What has to be true is narrower and checkable -- the
// emitter makes no progress at all while nobody is reading, whatever the
// stream was configured to carry.
func TestStalledConsumerBlocksTheEmitter(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{StreamMessages: 100_000})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	ch := sim.Streams().Open(ctx, "stalled")

	// Nothing reads ch. An emitter that buffered ahead would run to 100,000;
	// one that blocks stops after the single message it is offering.
	time.Sleep(100 * time.Millisecond)

	if got := sim.Streams().Produced(); got > 1 {
		t.Fatalf("the emitter produced %d messages against a consumer that read none; "+
			"it is buffering rather than blocking, and a stalled scenario would exhaust memory", got)
	}

	// Draining shows the block was a block and not a stall: the same emitter
	// runs to completion once somebody reads.
	consumed := 0
	for range ch {
		consumed++
		if consumed == 10 {
			break
		}
	}
	if consumed != 10 {
		t.Fatalf("consumed %d messages after unblocking, want 10", consumed)
	}
	if got := sim.Streams().Produced(); got < 10 {
		t.Errorf("Produced() = %d after 10 messages were consumed, want at least 10", got)
	}
}

// TestStreamsRefuseOnAPoweredOffNode keeps the streams honest about node state:
// a machine that is off does not stream.
func TestStreamsRefuseOnAPoweredOffNode(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{StreamMessages: 2})
	cl := newMachineryClient(t, sim)
	ctx := testContext(t)

	if _, err := cl.MachineClient.Shutdown(ctx, &machine.ShutdownRequest{}); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	stream, err := cl.Dmesg(ctx, false, true)
	if err != nil {
		if status.Code(err) != codes.Unavailable {
			t.Fatalf("Dmesg on a powered-off node returned %v, want Unavailable", err)
		}
		return
	}
	if _, err := stream.Recv(); status.Code(err) != codes.Unavailable {
		t.Errorf("Dmesg on a powered-off node streamed %v, want Unavailable", err)
	}
}

// TestCOSIIsRegisteredOnTheMachineServiceConnection is the co-location claim
// stated as an assertion rather than as a comment.
//
// One client, one connection: if a MachineService call and a COSI read both
// succeed through it, the state server cannot be living on a second listener.
func TestCOSIIsRegisteredOnTheMachineServiceConnection(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "one-connection"})
	cl := newMachineryClient(t, sim)
	ctx := testContext(t)

	if _, err := cl.MachineClient.Hostname(ctx, &emptypb.Empty{}); err != nil {
		t.Fatalf("Hostname over the connection: %v", err)
	}
	if _, err := safe.StateGetByID[*hardware.SystemInformation](ctx, cl.COSI, hardware.SystemInformationID); err != nil {
		t.Fatalf("COSI read over the same connection: %v; "+
			"the state server is not on the connection the production client dials", err)
	}
}

func drainData(t *testing.T, stream interface{ Recv() (*common.Data, error) }) int {
	t.Helper()

	count := 0
	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return count
		}
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}
		if len(msg.GetBytes()) == 0 {
			t.Error("a stream message carried no bytes")
		}
		count++
	}
}
