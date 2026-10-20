package machineconfig_test

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/machineconfig"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// The proofs of Phase 17 against a real Talos client and a simulated node: an
// apply is refused when the node changed since the plan, the refusal sends
// nothing, and what it says names paths and nothing else.

const (
	ownPatch   = "machine:\n  certSANs:\n    - 10.0.0.9\n"
	otherPatch = "machine:\n  certSANs:\n    - 10.0.0.10\n"
	imagePatch = "apiVersion: v1alpha1\nkind: KubeletConfig\nimage: ghcr.io/siderolabs/kubelet:v1.34.2\n"
)

// planNode is one simulated control-plane node with a Connect that counts the
// clients it opens and can hang an observer on each of them.
type planNode struct {
	t        *testing.T
	cl       *talossim.Cluster
	sim      *talossim.Server
	connects atomic.Int32

	mu   sync.Mutex
	hook talos.ApplyObserver
}

func newPlanNode(t *testing.T) *planNode {
	t.Helper()

	cl, err := talossim.NewCluster("homelab", "https://192.168.1.10:6443")
	if err != nil {
		t.Fatalf("NewCluster: %v", err)
	}
	sim, err := talossim.New(talossim.Options{Hostname: "cp-1", Cluster: cl, ControlPlane: true})
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}
	t.Cleanup(func() { _ = sim.Close() })
	return &planNode{t: t, cl: cl, sim: sim}
}

func (n *planNode) client(ctx context.Context, id model.MachineID) (*talos.ClusterClient, error) {
	return talos.NewClusterClient(ctx, n.sim.Dialer(), talos.Target{
		Cluster: "c1", Machine: id, Addr: n.sim.Host(),
	}, n.sim.ClientCreds(), talos.Mode{})
}

// connect is the Connect of the service under test: it counts, and it hangs the
// current hook on the client as the daemon hangs the drift observer.
func (n *planNode) connect(ctx context.Context, id model.MachineID) (*talos.ClusterClient, error) {
	n.connects.Add(1)
	cc, err := n.client(ctx, id)
	if err != nil {
		return nil, err
	}
	n.mu.Lock()
	hook := n.hook
	n.mu.Unlock()
	if hook != nil {
		cc.SetApplyObserver(hook)
	}
	return cc, nil
}

func (n *planNode) setHook(h talos.ApplyObserver) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.hook = h
}

func (n *planNode) service() *machineconfig.Service {
	return machineconfig.New(machineconfig.Deps{Connect: n.connect})
}

// ctx is a context with a deadline, which the Talos client requires.
func (n *planNode) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

// raw reads the node's unredacted active configuration through a client that is
// not counted. The bytes stay in the test and are never logged.
func (n *planNode) raw() string {
	n.t.Helper()
	ctx, cancel := n.ctx()
	defer cancel()
	cc, err := n.client(ctx, "m1")
	if err != nil {
		n.t.Fatalf("client: %v", err)
	}
	defer func() { _ = cc.Close() }()
	b, err := cc.MachineConfigYAML(ctx)
	if err != nil {
		n.t.Fatalf("MachineConfigYAML: %v", err)
	}
	return string(b)
}

// thirdParty changes the node as talosctl or another operator would: a client
// without an observer, no reboot.
func (n *planNode) thirdParty(patch string) {
	n.t.Helper()
	ctx, cancel := n.ctx()
	defer cancel()
	cc, err := n.client(ctx, "m1")
	if err != nil {
		n.t.Fatalf("a third party cannot connect: %v", err)
	}
	defer func() { _ = cc.Close() }()
	active, err := cc.MachineConfigYAML(ctx)
	if err != nil {
		n.t.Fatalf("a third party cannot read: %v", err)
	}
	patched, err := machineconfig.ApplyPatches(active, []string{patch})
	if err != nil {
		n.t.Fatalf("ApplyPatches: %v", err)
	}
	if _, err := cc.ApplyConfigurationWithMode(ctx, patched, "no-reboot"); err != nil {
		n.t.Fatalf("a third party cannot apply: %v", err)
	}
}

func (n *planNode) plan(svc *machineconfig.Service, patch string) machineconfig.Preview {
	n.t.Helper()
	ctx, cancel := n.ctx()
	defer cancel()
	p, err := svc.Plan(ctx, "m1", []string{patch})
	if err != nil {
		n.t.Fatalf("Plan: %v", err)
	}
	return p
}

func (n *planNode) apply(svc *machineconfig.Service, patch string, mode machineconfig.Mode, digest string) error {
	n.t.Helper()
	ctx, cancel := n.ctx()
	defer cancel()
	_, err := svc.Apply(ctx, "m1", []string{patch}, mode, digest)
	return err
}

func (n *planNode) calls() int { return n.sim.Calls("ApplyConfiguration") }

func TestThePlanCarriesTheDigestOfTheConfigurationItRead(t *testing.T) {
	t.Parallel()

	n := newPlanNode(t)
	svc := n.service()

	snap, err := svc.ReadSnapshot(context.Background(), "m1")
	if err != nil {
		t.Fatalf("ReadSnapshot: %v", err)
	}
	first := n.plan(svc, ownPatch)
	second := n.plan(svc, ownPatch)

	if first.BaseDigest != snap.Digest {
		t.Errorf("the plan carries %q, want the snapshot digest %q", first.BaseDigest, snap.Digest)
	}
	if first.BaseDigest != second.BaseDigest {
		t.Errorf("two plans of an unchanged node carry %q and %q, want the same", first.BaseDigest, second.BaseDigest)
	}

	n.thirdParty(imagePatch)
	if third := n.plan(svc, ownPatch); third.BaseDigest == first.BaseDigest {
		t.Error("a plan after a change in the KubeletConfig document carries the old digest")
	}
}

func TestAnApplyIsRefusedWhenTheNodeChangedSinceThePlan(t *testing.T) {
	t.Parallel()

	n := newPlanNode(t)
	svc := n.service()
	p := n.plan(svc, ownPatch)

	// A document that is not the first one: the case Compare cannot see.
	n.thirdParty(imagePatch)
	sent := n.calls()

	err := n.apply(svc, ownPatch, machineconfig.ModeNoReboot, p.BaseDigest)
	if !errors.Is(err, machineconfig.ErrChangedSincePlan) {
		t.Fatalf("Apply after a foreign change returned %v, want ErrChangedSincePlan", err)
	}
	if got := n.calls(); got != sent {
		t.Errorf("the node heard %d ApplyConfiguration call(s) after the refusal, want none", got-sent)
	}
	raw := n.raw()
	if strings.Contains(raw, "10.0.0.9") {
		t.Error("the refused patch is on the node")
	}
	if !strings.Contains(raw, "kubelet:v1.34.2") {
		t.Error("the third party's change is gone from the node")
	}

	// An apply on a node that did not change still lands (the refusal is for a
	// changed node, not for every apply).
	fresh := n.plan(svc, ownPatch)
	if err := n.apply(svc, ownPatch, machineconfig.ModeNoReboot, fresh.BaseDigest); err != nil {
		t.Fatalf("Apply on an unchanged node: %v", err)
	}
	if !strings.Contains(n.raw(), "10.0.0.9") {
		t.Error("the apply on an unchanged node did not land")
	}
}

func TestAnApplyWithoutADigestIsRefused(t *testing.T) {
	t.Parallel()

	n := newPlanNode(t)
	svc := n.service()
	n.plan(svc, ownPatch)
	sent, opened := n.calls(), n.connects.Load()

	if err := n.apply(svc, ownPatch, machineconfig.ModeNoReboot, ""); !errors.Is(err, machineconfig.ErrBaseDigestRequired) {
		t.Fatalf("Apply with an empty digest returned %v, want ErrBaseDigestRequired", err)
	}
	if got := n.connects.Load(); got != opened {
		t.Error("an apply without a digest opened a client")
	}

	// A well-formed digest that is not the node's.
	if err := n.apply(svc, ownPatch, machineconfig.ModeNoReboot, strings.Repeat("0", 64)); !errors.Is(err, machineconfig.ErrChangedSincePlan) {
		t.Fatalf("Apply with a wrong digest returned %v, want ErrChangedSincePlan", err)
	}
	if got := n.calls(); got != sent {
		t.Errorf("the node heard %d ApplyConfiguration call(s), want none", got-sent)
	}
	if strings.Contains(n.raw(), "10.0.0.9") {
		t.Error("a refused apply changed the node")
	}
}

func TestTheRefusalSentenceNamesPathsAndNothingElse(t *testing.T) {
	t.Parallel()

	n := newPlanNode(t)
	svc := n.service()
	p := n.plan(svc, ownPatch)

	before, err := svc.ReadSnapshot(context.Background(), "m1")
	if err != nil {
		t.Fatalf("ReadSnapshot: %v", err)
	}
	// Two foreign changes: a value in a later document and an address in a list.
	n.thirdParty(imagePatch)
	n.thirdParty("machine:\n  certSANs:\n    - 192.168.1.10\n")
	after, err := svc.ReadSnapshot(context.Background(), "m1")
	if err != nil {
		t.Fatalf("ReadSnapshot: %v", err)
	}

	// The precondition that gives the absences below their meaning: the values
	// are in the difference CompareDocuments hands out, so a sentence without
	// them left them out on purpose.
	diff, err := machineconfig.CompareDocuments(before.Text, after.Text)
	if err != nil {
		t.Fatalf("CompareDocuments: %v", err)
	}
	var sawImage, sawAddress bool
	for _, c := range diff.Changes {
		sawImage = sawImage || strings.Contains(c.After, "kubelet:v1.34.2")
		sawAddress = sawAddress || strings.Contains(c.After, "192.168.1.10")
	}
	if !sawImage || !sawAddress {
		t.Fatalf("precondition: the difference does not carry the new values (image %v, address %v): %+v",
			sawImage, sawAddress, diff.Changes)
	}

	err = n.apply(svc, ownPatch, machineconfig.ModeNoReboot, p.BaseDigest)
	var refusal *machineconfig.ChangedSincePlanError
	if !errors.As(err, &refusal) {
		t.Fatalf("Apply returned %v, want a *ChangedSincePlanError", err)
	}
	if !refusal.Known {
		t.Error("the plan's earlier state is held by the service that made the plan, but Known is false")
	}
	got := err.Error()
	for _, want := range []string{"KubeletConfig.image (modified)", "v1alpha1.machine.certSANs (list changed, 0 entries became 1)"} {
		if !strings.Contains(got, want) {
			t.Errorf("the sentence lacks %q:\n%s", want, got)
		}
	}
	absent := []string{"192.168.1.10", "kubelet:v1.34.2", machineconfig.RedactedMarker}
	for name, secret := range secretsOf(t, n.cl) {
		absent = append(absent, secret, base64.StdEncoding.EncodeToString([]byte(secret)))
		_ = name
	}
	for _, leak := range absent {
		if strings.Contains(got, leak) {
			t.Errorf("the sentence holds a value it must not (%.20q...):\n%s", leak, got)
		}
	}
}

func TestARefusalStillHappensWhenThePlanIsNoLongerHeld(t *testing.T) {
	t.Parallel()

	n := newPlanNode(t)
	planner := n.service()
	p := n.plan(planner, ownPatch)
	n.thirdParty(imagePatch)

	// A service that never saw the plan: the daemon after its hourly update.
	fresh := n.service()
	err := n.apply(fresh, ownPatch, machineconfig.ModeNoReboot, p.BaseDigest)
	var refusal *machineconfig.ChangedSincePlanError
	if !errors.As(err, &refusal) {
		t.Fatalf("Apply on a service without the plan returned %v, want a refusal", err)
	}
	if refusal.Known {
		t.Error("Known is true although the service never held the plan's state")
	}
	if !strings.Contains(err.Error(), "no longer held") {
		t.Errorf("the sentence does not say the earlier state is gone:\n%s", err)
	}
	if strings.Contains(n.raw(), "10.0.0.9") {
		t.Error("the refused patch is on the node")
	}
}

// TestASecretRotatedSinceThePlanIsCarriedForwardNotReverted is the proof of the
// choice the digest rests on: it is made over the redacted snapshot, so a secret
// rotated by somebody else is invisible to it, and it is harmless only because
// the apply patches the bytes it has just read and not anything it kept.
func TestASecretRotatedSinceThePlanIsCarriedForwardNotReverted(t *testing.T) {
	t.Parallel()

	n := newPlanNode(t)
	svc := n.service()
	p := n.plan(svc, ownPatch)

	oldToken := n.cl.Secrets.TrustdInfo.Token
	const newToken = "abcdef.0123456789abcdef"
	if oldToken == newToken || !strings.Contains(n.raw(), oldToken) {
		t.Fatal("precondition: the node does not hold the old token in the clear")
	}
	n.thirdParty("machine:\n  token: " + newToken + "\n")
	if raw := n.raw(); !strings.Contains(raw, newToken) || strings.Contains(raw, oldToken) {
		t.Fatal("precondition: the third party's token did not replace the old one")
	}

	if err := n.apply(svc, ownPatch, machineconfig.ModeNoReboot, p.BaseDigest); err != nil {
		t.Fatalf("a rotated secret alone was refused: %v", err)
	}
	raw := n.raw()
	if !strings.Contains(raw, newToken) {
		t.Error("the apply did not carry the rotated token forward")
	}
	if strings.Contains(raw, oldToken) {
		t.Error("the apply reverted the rotated token to the one the plan saw")
	}
	if !strings.Contains(raw, "10.0.0.9") {
		t.Error("the patch is not on the node")
	}
}

func TestARefusedStagedApplyWithdrawsItsClaim(t *testing.T) {
	t.Parallel()

	n := newPlanNode(t)
	svc := n.service()
	p := n.plan(svc, ownPatch)
	n.thirdParty(imagePatch)

	if err := n.apply(svc, ownPatch, machineconfig.ModeStaged, p.BaseDigest); !errors.Is(err, machineconfig.ErrChangedSincePlan) {
		t.Fatalf("the staged apply returned %v, want ErrChangedSincePlan", err)
	}
	ctx, cancel := n.ctx()
	defer cancel()
	view, err := svc.Get(ctx, "m1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if view.StagedPending {
		t.Error("a refused staged apply is reported as waiting for the next boot")
	}

	fresh := n.plan(svc, ownPatch)
	if err := n.apply(svc, ownPatch, machineconfig.ModeStaged, fresh.BaseDigest); err != nil {
		t.Fatalf("a staged apply after the refused one returned %v, want it to land", err)
	}
}

func TestAnApplyOpensOneClientForReadingAndSending(t *testing.T) {
	t.Parallel()

	n := newPlanNode(t)
	svc := n.service()

	before := n.connects.Load()
	p := n.plan(svc, ownPatch)
	if got := n.connects.Load() - before; got != 1 {
		t.Errorf("a plan opened %d clients, want 1", got)
	}

	before = n.connects.Load()
	if err := n.apply(svc, ownPatch, machineconfig.ModeNoReboot, p.BaseDigest); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := n.connects.Load() - before; got != 1 {
		t.Errorf("an apply opened %d clients, want 1: the check and the send must share one", got)
	}
}

// holdFirstApply makes the first configuration sent through any client of the
// node stop just before the RPC, where the observer of the drift baseline sits.
// reached says it stands there; the returned function lets it go (once, safe to
// call again).
func (n *planNode) holdFirstApply() (reached <-chan struct{}, release func()) {
	hit := make(chan struct{}, 1)
	gate := make(chan struct{})
	var first atomic.Bool
	n.setHook(func(context.Context, talos.ApplyAttempt) func(error) {
		if first.CompareAndSwap(false, true) {
			hit <- struct{}{}
			<-gate
		}
		return nil
	})
	var once sync.Once
	n.t.Cleanup(func() { once.Do(func() { close(gate) }) })
	return hit, func() { once.Do(func() { close(gate) }) }
}

// TestTwoAppliesOnOneNodeDoNotOverwriteEachOther is VERS-05. Both applies were
// planned against the same state; the first is held just before it sends; the
// second must not even read the node while the first runs. Released, the first
// lands and the second reads the new state and is refused. Without the lock the
// second reads the old state, passes the check and overwrites the first. The
// 200 milliseconds support the red view only: with the lock the outcome does not
// depend on timing.
func TestTwoAppliesOnOneNodeDoNotOverwriteEachOther(t *testing.T) {
	t.Parallel()

	n := newPlanNode(t)
	svc := n.service()
	p1 := n.plan(svc, ownPatch)
	p2 := n.plan(svc, otherPatch)
	if p1.BaseDigest != p2.BaseDigest {
		t.Fatal("precondition: the two plans were not made from the same state")
	}

	reached, release := n.holdFirstApply()

	firstDone := make(chan error, 1)
	go func() { firstDone <- n.apply(svc, ownPatch, machineconfig.ModeNoReboot, p1.BaseDigest) }()
	select {
	case <-reached:
	case <-time.After(10 * time.Second):
		t.Fatal("the first apply never reached the send")
	}

	opened := n.connects.Load()
	secondDone := make(chan error, 1)
	go func() { secondDone <- n.apply(svc, otherPatch, machineconfig.ModeNoReboot, p2.BaseDigest) }()
	time.Sleep(200 * time.Millisecond)
	if got := n.connects.Load(); got != opened {
		t.Errorf("the second apply opened %d client(s) while the first was running, want it to wait for the lock", got-opened)
	}

	release()
	if err := <-firstDone; err != nil {
		t.Fatalf("the first apply: %v", err)
	}
	if err := <-secondDone; !errors.Is(err, machineconfig.ErrChangedSincePlan) {
		t.Errorf("the second apply returned %v, want ErrChangedSincePlan", err)
	}

	raw := n.raw()
	if !strings.Contains(raw, "10.0.0.9") {
		t.Error("the first apply is not on the node")
	}
	if strings.Contains(raw, "10.0.0.10") {
		t.Error("the second apply overwrote the first")
	}
}

func TestAnApplyWaitingForTheNodeGivesUpWithItsContext(t *testing.T) {
	t.Parallel()

	n := newPlanNode(t)
	svc := n.service()
	p1 := n.plan(svc, ownPatch)
	p2 := n.plan(svc, otherPatch)

	reached, release := n.holdFirstApply()
	firstDone := make(chan error, 1)
	go func() { firstDone <- n.apply(svc, ownPatch, machineconfig.ModeNoReboot, p1.BaseDigest) }()
	select {
	case <-reached:
	case <-time.After(10 * time.Second):
		t.Fatal("the first apply never reached the send")
	}
	sent := n.calls()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	secondDone := make(chan error, 1)
	go func() {
		_, err := svc.Apply(ctx, "m1", []string{otherPatch}, machineconfig.ModeNoReboot, p2.BaseDigest)
		secondDone <- err
	}()
	select {
	case err := <-secondDone:
		if !errors.Is(err, machineconfig.ErrApplyBusy) {
			t.Errorf("the waiting apply returned %v, want ErrApplyBusy", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the waiting apply did not give up when its context ended")
	}
	if got := n.calls(); got != sent {
		t.Errorf("the node heard %d ApplyConfiguration call(s) from the waiting apply, want none", got-sent)
	}

	release()
	if err := <-firstDone; err != nil {
		t.Fatalf("the first apply: %v", err)
	}
}
