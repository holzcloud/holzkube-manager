package configdrift_test

import (
	"context"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/configdrift"
)

// TestEveryReadCarriesADeadline: the Talos client refuses a call without one,
// and the context the daemon starts its background loops with has none. certwatch
// lacked exactly this in v0.6.0 and v0.7.0 (ledger 137), and no test saw it
// because every test context carried a deadline of its own.
func TestEveryReadCarriesADeadline(t *testing.T) {
	t.Parallel()

	n := newNode(t)
	fake := &fakeReader{}
	good, _ := twoSnaps(t, n)
	fake.set(good, nil)
	svc := n.service(t, withRead(fake.Read))

	// context.Background() is what main.go passes down: no deadline of its own.
	bg := context.Background()
	if _, ok := bg.Deadline(); ok {
		t.Fatal("precondition: the context must carry no deadline")
	}
	svc.Tick(bg)

	if fake.calls != 1 {
		t.Fatalf("the reader was called %d times, want 1", fake.calls)
	}
	if !fake.hadLimit {
		t.Fatal("Read was handed a context without a deadline; the Talos client refuses such a call")
	}
	if fake.remaining <= 0 || fake.remaining > configdrift.DefaultReadBudget {
		t.Errorf("the deadline is %v away, want within (0, %v]", fake.remaining, configdrift.DefaultReadBudget)
	}
}

// TestARunningDaemonsContextReadsARealNode is the same claim through the real
// client against the simulator, started from a context with no deadline.
func TestARunningDaemonsContextReadsARealNode(t *testing.T) {
	t.Parallel()

	n := newNode(t)
	svc := n.service(t)

	bg := context.Background()
	if _, ok := bg.Deadline(); ok {
		t.Fatal("precondition: the context must carry no deadline")
	}
	svc.Tick(bg)

	d := detail(t, svc)
	if d.State != configdrift.StateBaselineCreated {
		t.Fatalf("nothing was read from the node through a context without a deadline: state %q (%s)", d.State, d.Sentence)
	}
}
