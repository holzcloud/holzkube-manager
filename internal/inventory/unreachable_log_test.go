package inventory_test

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// TestANodeThatDoesNotAnswerSaysWhyOnceAndNotEveryInterval is the regression
// guard for what made a real diagnosis need a second round trip to the operator.
//
// Their journal carried, every interval and for hours:
//
//	"a node's resource watch ended"  reason="the node stopped answering, so its
//	 resource watch was closed rather than left claiming to be live"
//
// and nothing else. The daemon knew the reason -- unreachableReason classifies
// it and the read model carries it per field -- and never wrote it down. What
// the screen shows and what a journal needs are different things: a dashboard
// cell gets the classified sentence because a gRPC status string pushes the
// actionable part off the edge, and a journal gets the raw error because that is
// where somebody is looking when the sentence was not enough.
//
// The ADDRESS is the other half. A record can be filed against an address the
// machine does not answer at -- adoption files each member under the FIRST
// address it reports -- and a line that does not say where the node was asked
// cannot be told apart from a node that is genuinely off.
//
// Both halves of the assertion matter and the second is the one that keeps this
// honest: saying it every interval would have been the other way to make a
// journal useless, and a node that is down stays down.
func TestANodeThatDoesNotAnswerSaysWhyOnceAndNotEveryInterval(t *testing.T) {
	// The message this guard is about, spelled once. It is the whole log line's
	// msg field, so it cannot be satisfied by the same words appearing inside
	// the reason or the raw error beside it.
	const logLine = `msg="a node did not answer"`

	logs := &syncBuffer{}
	withFixtureLog(t, logs)

	ctx := testContext(t)
	// An hour's heartbeat so the background supervisor -- which the adoption
	// path now starts, and rightly -- does not refresh inside the window this
	// test is measuring. Its own Refresh calls are the measurement.
	f := newFixtureWith(t, talossim.Options{
		Hostname: "cp-1", ControlPlane: true, Bootstrapped: true,
	}, time.Hour)
	f.importCluster(ctx, t)

	id := f.onlyMachine(ctx, t)

	// Move the record to an address nothing listens on. Port 1 is reserved and
	// unbound, so the dial fails at the transport rather than anywhere that
	// could be mistaken for a Talos answer -- and this is the operator's own
	// case in miniature: a record filed against an address its machine does not
	// answer at, with everything else about the cluster correct.
	rec, err := f.store.Machines().Get(ctx, id)
	if err != nil {
		t.Fatalf("reading the machine: %v", err)
	}
	rec.Addr = "127.0.0.1:1"
	if _, err := f.store.Machines().Put(ctx, rec); err != nil {
		t.Fatalf("moving the machine to a dead address: %v", err)
	}

	logs.Reset()
	f.svc.Refresh(ctx, id)
	first := logs.String()

	if !strings.Contains(first, logLine) {
		t.Fatalf("the daemon recorded no line saying the node did not answer.\n"+
			"This is the defect: it logged that the watch ended and never what it got "+
			"back, so a real diagnosis needed another round trip.\nGot:\n%s", first)
	}
	if !strings.Contains(first, "127.0.0.1:1") {
		t.Errorf("the line does not name the address the node was asked at.\n"+
			"Without it, a record filed against an address the machine does not answer "+
			"at reads exactly like a machine that is switched off.\nGot:\n%s", first)
	}
	if !strings.Contains(first, string(id)) {
		t.Errorf("the line does not name the machine.\nGot:\n%s", first)
	}

	// Keep asking until the observer has made up its mind and calls the node
	// down. Two lines is the design and not an accident: the first failure,
	// which may still be a blip, and the moment the count crosses into
	// StageDown, which is the one an operator acts on.
	for range 6 {
		f.svc.Refresh(ctx, id)
	}
	// Counting the MESSAGE and not the phrase: "did not answer" occurs three
	// times in every one of these lines -- in msg, in the classified reason and
	// in the raw error -- so counting the phrase reported six for two lines and
	// sent this test after a defect that was not there.
	if lines := strings.Count(logs.String(), logLine); lines > 2 {
		t.Errorf("the failure was logged %d times while nothing about it changed.\n"+
			"Two is the design -- the first failure and the crossing into down. More than "+
			"that is how the operator's journal came to hold one sentence over and over.\n"+
			"Got:\n%s", lines, logs.String())
	}

	// Settled. From here the heartbeat asks every interval for as long as the
	// node stays down, and every one of those must be silent.
	logs.Reset()
	for range 4 {
		f.svc.Refresh(ctx, id)
	}
	if repeat := logs.String(); strings.Contains(repeat, logLine) {
		t.Errorf("a node that was already down logged its failure again.\n"+
			"This is the shape of the operator's journal: the same fact, every interval, "+
			"until it drowns out whatever changed.\nGot:\n%s", repeat)
	}
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *syncBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
}
