package provision_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/provision"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// Bootstrapping etcd twice destroys a cluster. These tests are about the four
// independent mechanisms that make it impossible, one at a time, because each
// covers a case the others do not.

const (
	testCluster = model.ClusterID("c1")
	testMachine = model.MachineID("00000000-0000-4000-8000-000000000001")
)

func newBootstrapper(t *testing.T) (*provision.Bootstrapper, string) {
	t.Helper()

	dir := t.TempDir()
	b, err := provision.NewBootstrapper(filepath.Join(dir, "bootstrap"))
	if err != nil {
		t.Fatalf("NewBootstrapper: %v", err)
	}
	return b, filepath.Join(dir, "bootstrap")
}

func newNode(t *testing.T) (*talossim.Server, *talos.ClusterClient) {
	t.Helper()

	sim, err := talossim.New(talossim.Options{Hostname: "cp-1"})
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}
	t.Cleanup(func() { _ = sim.Close() })

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)

	cc, err := talos.NewClusterClient(ctx, sim.Dialer(), talos.Target{
		Cluster: testCluster, Machine: testMachine, Addr: sim.Host(),
	}, sim.ClientCreds(), talos.Mode{})
	if err != nil {
		t.Fatalf("NewClusterClient: %v", err)
	}
	t.Cleanup(func() { _ = cc.Close() })

	return sim, cc
}

// TestASecondBootstrapIsRefused is the whole point, through the ordinary path.
func TestASecondBootstrapIsRefused(t *testing.T) {
	t.Parallel()

	b, _ := newBootstrapper(t)
	sim, cc := newNode(t)

	if err := b.Bootstrap(t.Context(), cc, testCluster, testMachine, sim.Host()); err != nil {
		t.Fatalf("the first bootstrap: %v", err)
	}

	err := b.Bootstrap(t.Context(), cc, testCluster, testMachine, sim.Host())
	if !errors.Is(err, provision.ErrAlreadyBootstrapped) {
		t.Fatalf("the second bootstrap returned %v, want ErrAlreadyBootstrapped", err)
	}

	// And the node was asked exactly once. The record refusing is what
	// matters, not the node's own refusal catching it afterwards.
	if got := sim.Calls("Bootstrap"); got != 1 {
		t.Fatalf("the node saw %d Bootstrap calls, want 1", got)
	}
}

// TestTwoConcurrentBootstrapsCannotBothProceed is mechanism 2, and it is the
// only one of the four that covers two operators clicking at the same moment.
//
// O_CREAT|O_EXCL is atomic in the kernel, which is what makes this decidable
// rather than a race.
func TestTwoConcurrentBootstrapsCannotBothProceed(t *testing.T) {
	t.Parallel()

	b, _ := newBootstrapper(t)
	sim, cc := newNode(t)

	const attempts = 8
	var wg sync.WaitGroup
	results := make([]error, attempts)

	for i := range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = b.Bootstrap(t.Context(), cc, testCluster, testMachine, sim.Host())
		}()
	}
	wg.Wait()

	var succeeded int
	for _, err := range results {
		if err == nil {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Fatalf("%d of %d concurrent bootstraps succeeded, want exactly 1", succeeded, attempts)
	}
	if got := sim.Calls("Bootstrap"); got != 1 {
		t.Fatalf("the node saw %d Bootstrap calls under %d concurrent attempts, want 1",
			got, attempts)
	}
}

// TestAnInterruptedBootstrapIsUnclearAndNeverRetried is mechanism 3.
//
// A record with no outcome is what a crash mid-call leaves. It is the one fact
// nothing else can reconstruct, and the response to it is a person -- never a
// retry, because a retry there is the exact operation all of this prevents.
func TestAnInterruptedBootstrapIsUnclearAndNeverRetried(t *testing.T) {
	t.Parallel()

	b, dir := newBootstrapper(t)
	sim, cc := newNode(t)

	// The shape a crash mid-call leaves behind: the lease taken, no outcome.
	intent := `{"cluster":"c1","machine":"` + string(testMachine) +
		`","addr":"10.0.0.1","started_at":"2026-09-11T12:00:00Z"}`
	if err := os.WriteFile(filepath.Join(dir, "c1.json"), []byte(intent), 0o600); err != nil {
		t.Fatalf("write the interrupted record: %v", err)
	}

	err := b.Bootstrap(t.Context(), cc, testCluster, testMachine, sim.Host())
	if !errors.Is(err, provision.ErrBootstrapUnclear) {
		t.Fatalf("a bootstrap over an interrupted attempt returned %v, want ErrBootstrapUnclear", err)
	}
	if got := sim.Calls("Bootstrap"); got != 0 {
		t.Fatalf("the node was bootstrapped %d times over an unclear record; that is the "+
			"operation all four mechanisms exist to prevent", got)
	}
	if !strings.Contains(err.Error(), "etcd members") {
		t.Errorf("the refusal reads %q, which does not tell the operator what to check", err)
	}

	// The recovery flow: the operator looked and says what they found.
	pending, ok := b.PendingIntent(testCluster)
	if !ok {
		t.Fatal("the unclear attempt is not offered for recovery")
	}
	if pending.Addr != "10.0.0.1" {
		t.Errorf("the pending attempt names %q", pending.Addr)
	}

	if err := b.Resolve(testCluster, true, "etcd has three members"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if _, ok := b.PendingIntent(testCluster); ok {
		t.Error("the attempt is still pending after it was resolved")
	}

	// Resolved as bootstrapped, so a later attempt is refused as such rather
	// than as unclear.
	err = b.Bootstrap(t.Context(), cc, testCluster, testMachine, sim.Host())
	if !errors.Is(err, provision.ErrAlreadyBootstrapped) {
		t.Fatalf("after resolving as bootstrapped, a bootstrap returned %v", err)
	}
}

// TestAnUnparsableRecordIsAlsoUnclear closes the door the JSON decoder would
// otherwise leave open: something was written and cannot be read, and guessing
// would be guessing about a bootstrap.
func TestAnUnparsableRecordIsAlsoUnclear(t *testing.T) {
	t.Parallel()

	b, dir := newBootstrapper(t)
	sim, cc := newNode(t)

	if err := os.WriteFile(filepath.Join(dir, "c1.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	err := b.Bootstrap(t.Context(), cc, testCluster, testMachine, sim.Host())
	if !errors.Is(err, provision.ErrBootstrapUnclear) {
		t.Fatalf("a bootstrap over an unreadable record returned %v, want ErrBootstrapUnclear", err)
	}
	if got := sim.Calls("Bootstrap"); got != 0 {
		t.Fatal("the node was bootstrapped over a record that could not be read")
	}
}

// TestAFailedBootstrapMayBeRetried is the one case a second attempt is allowed,
// and it is worth pinning because the rest of this file is about refusing.
func TestAFailedBootstrapMayBeRetried(t *testing.T) {
	t.Parallel()

	b, dir := newBootstrapper(t)
	sim, cc := newNode(t)

	recorded := `{"cluster":"c1","machine":"` + string(testMachine) +
		`","addr":"10.0.0.1","started_at":"2026-09-11T12:00:00Z","outcome":"failed",` +
		`"detail":"the node refused"}`
	if err := os.WriteFile(filepath.Join(dir, "c1.json"), []byte(recorded), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := b.Bootstrap(t.Context(), cc, testCluster, testMachine, sim.Host()); err != nil {
		t.Fatalf("a bootstrap after a recorded failure: %v", err)
	}
	if got := sim.Calls("Bootstrap"); got != 1 {
		t.Fatalf("the node saw %d Bootstrap calls, want 1", got)
	}
}

// TestTheNodesOwnRefusalIsTheLastLineAndIsNotAFailure is mechanism 4, in the
// one situation where it is the only one of the four that can catch anything.
//
// The node is already bootstrapped *and* its etcd is not answering. Mechanism
// 1 therefore cannot tell -- that is precisely the case its own comment says
// it misses -- there is no prior record, and no other process is competing. So
// the call goes out and the node refuses it.
//
// And the refusal is not a failure: the cluster is in the state that was
// wanted, and reporting it as a failure would send somebody to fix something
// that works.
func TestTheNodesOwnRefusalIsTheLastLineAndIsNotAFailure(t *testing.T) {
	t.Parallel()

	b, _ := newBootstrapper(t)
	sim, cc := newNode(t)

	restoreBootstrap, err := sim.Inject(talossim.Scenario{
		Name: talossim.ScenarioSecondBootstrapAlreadyExists,
	})
	if err != nil {
		t.Fatalf("inject second_bootstrap_returns_AlreadyExists: %v", err)
	}
	defer restoreBootstrap()

	// etcd is down, so the pre-flight cannot answer. This is the gap
	// mechanism 1 documents about itself.
	restoreEtcd, err := sim.Inject(talossim.Scenario{Name: talossim.ScenarioEtcdDown})
	if err != nil {
		t.Fatalf("inject etcd_down: %v", err)
	}
	defer restoreEtcd()

	if err := b.Bootstrap(t.Context(), cc, testCluster, testMachine, sim.Host()); err != nil {
		t.Fatalf("a bootstrap the node itself refused as AlreadyExists was reported as an error: %v", err)
	}

	if got := sim.Calls("Bootstrap"); got != 1 {
		t.Fatalf("the node saw %d Bootstrap calls, want the one that got refused", got)
	}
	if _, ok := b.PendingIntent(testCluster); ok {
		t.Error("the attempt is recorded as pending after the node answered")
	}

	// And a second attempt is now refused by the record, without touching the
	// node again.
	err = b.Bootstrap(t.Context(), cc, testCluster, testMachine, sim.Host())
	if !errors.Is(err, provision.ErrAlreadyBootstrapped) {
		t.Fatalf("the next attempt returned %v, want ErrAlreadyBootstrapped", err)
	}
	if got := sim.Calls("Bootstrap"); got != 1 {
		t.Fatalf("the node saw %d Bootstrap calls in total, want 1", got)
	}
}
