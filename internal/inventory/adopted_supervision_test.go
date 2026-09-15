package inventory_test

import (
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/health"
	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// TestAnAdoptedNodeIsObservedWithoutARestart is the regression guard for the
// first thing an operator saw on real hardware after adoption succeeded: a
// cluster card reading "2 nodes, 0 healthy, 0 degraded, 2 not answering",
// moments after the import had PROVEN it could reach the control plane.
//
// The order here is the operator's and it is the whole point. The daemon is
// already running -- Start has already walked the store and supervised what was
// in it -- and the cluster arrives afterwards. Every machine the adoption
// records is therefore a machine Start never saw, and nothing else was asking
// anyone to look at them: Supervise had exactly one production caller, the
// manual "add a node" handler, so an adopted node sat at StageUnknown until the
// next restart happened to pick it up from the store.
//
// A node the product has just authenticated to, reported as not answering, is
// worse than a wrong colour. It is the screen teaching an operator that its
// health column is noise -- and this is the product whose entire claim is an
// inventory that stays honest.
//
// The assertion is deliberately about the READ MODEL and not about an internal
// map. What was broken is what the operator sees, and a test that reached into
// the service's supervised set would pass again the moment somebody satisfied
// the set without the observation arriving.
func TestAnAdoptedNodeIsObservedWithoutARestart(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixtureWith(t, talossim.Options{Hostname: "cp-1", ControlPlane: true, Bootstrapped: true}, time.Second)

	// The running daemon: Start walks a store that does not yet hold this
	// cluster, which is exactly what makes the adoption path responsible for
	// what it records afterwards.
	if err := f.svc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	f.importCluster(ctx, t)

	// No Supervise call here on purpose. watch_test.go makes that call itself,
	// which is why the gap survived: the guard for the supervisor's LIFETIME
	// proved the handler's call outlives the request, and nothing asked whether
	// the adoption path makes a call at all.
	f.await(ctx, t, 30*time.Second, func(v inventory.MachineView) bool {
		return v.Stage == health.StageWatching
	}, "the adopted node never left StageUnknown, so nothing was observing it. "+
		"This is the cluster card reading 'not answering' about a node the import "+
		"had just authenticated to -- the adoption path recorded the machine and "+
		"asked nobody to look at it")
}
