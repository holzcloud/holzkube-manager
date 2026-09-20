package kube_test

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/kubesim"
)

// The wall (2026-09-20).
//
// Read from four metres away by somebody who is not operating anything. Three
// things it must never claim, and every test here is one of them:
//
//   - A node nobody is hearing from is not a healthy node. That is the single
//     case the screen exists for.
//   - A CronJob between runs is not an outage. Get that wrong and the wall is red
//     every night at three, and by the second week nobody looks at it.
//   - Something deliberately stopped is not broken. It is a decision, and seeing
//     it must not be the same as being alarmed by it.

func tilesByName(tiles []kube.Tile) map[string]kube.Tile {
	out := map[string]kube.Tile{}
	for _, tile := range tiles {
		out[tile.Name] = tile
	}
	return out
}

// TestANodeNobodyIsHearingFromIsNeverGreen.
func TestANodeNobodyIsHearingFromIsNeverGreen(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{Nodes: []kubesim.Node{
		{Name: "cp-1", Ready: corev1.ConditionTrue},
		{Name: "cp-2", Ready: corev1.ConditionUnknown},
		{Name: "cp-3", Ready: corev1.ConditionFalse},
		{Name: "cp-4", Ready: corev1.ConditionTrue, Unschedulable: true},
	}})

	wall, err := client.ForTheWall(ctx, "", time.Now())
	if err != nil {
		t.Fatalf("ForTheWall: %v", err)
	}
	found := tilesByName(wall.Nodes)

	if got := found["cp-2"].State; got != kube.StateUnknown {
		t.Errorf("a node whose kubelet stopped reporting is %q, want unknown", got)
	}
	if got := found["cp-3"].State; got != kube.StateDown {
		t.Errorf("a NotReady node is %q, want down -- it runs nothing", got)
	}
	// A cordon is a decision somebody made and the node still runs what it has.
	if got := found["cp-4"].State; got != kube.StateWarn {
		t.Errorf("a cordoned node is %q, want warn", got)
	}
	if got := found["cp-1"].State; got != kube.StateOK {
		t.Errorf("a ready node is %q, want ok", got)
	}
}

// TestACronJobBetweenRunsIsNotAnOutage.
//
// It has no pods, and arithmetic over desired and ready calls nought of nought
// an outage. On a wall that is red every night at three, for ever.
func TestACronJobBetweenRunsIsNotAnOutage(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{CronJobs: []kubesim.CronJob{
		{Namespace: "backup", Name: "nightly", Schedule: "0 3 * * *"},
	}})

	wall, err := client.ForTheWall(ctx, "", time.Now())
	if err != nil {
		t.Fatalf("ForTheWall: %v", err)
	}
	if got := tilesByName(wall.Workloads)["nightly"].State; got != kube.StateOK {
		t.Errorf("a CronJob between runs is %q, want ok", got)
	}
}

// TestSomethingDeliberatelyStoppedIsNotBroken.
func TestSomethingDeliberatelyStoppedIsNotBroken(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{
		Deployments: []kubesim.Deployment{
			{Namespace: "default", Name: "paused", Desired: 0},
			{Namespace: "default", Name: "broken", Desired: 3, Ready: 0},
			{Namespace: "default", Name: "limping", Desired: 3, Ready: 2},
			{Namespace: "default", Name: "fine", Desired: 2, Ready: 2},
		},
		CronJobs: []kubesim.CronJob{
			{Namespace: "backup", Name: "suspended-one", Schedule: "0 3 * * *", Suspended: true},
		},
	})

	wall, err := client.ForTheWall(ctx, "", time.Now())
	if err != nil {
		t.Fatalf("ForTheWall: %v", err)
	}
	found := tilesByName(wall.Workloads)

	for name, want := range map[string]kube.State{
		"paused":        kube.StateStopped,
		"suspended-one": kube.StateStopped,
		"broken":        kube.StateDown,
		"limping":       kube.StateWarn,
		"fine":          kube.StateOK,
	} {
		if got := found[name].State; got != want {
			t.Errorf("%s is %q, want %q (%s)", name, got, want, found[name].Detail)
		}
	}
}

// TestTheWorstThingIsFirst, because a screen that cannot draw everything has to
// draw the part that matters.
func TestTheWorstThingIsFirst(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{Deployments: []kubesim.Deployment{
		// Named so that alphabetical order is the OPPOSITE of the order wanted:
		// otherwise this test would pass against no sorting at all.
		{Namespace: "default", Name: "aaa-fine", Desired: 2, Ready: 2},
		{Namespace: "default", Name: "mmm-limping", Desired: 3, Ready: 2},
		{Namespace: "default", Name: "zzz-broken", Desired: 3, Ready: 0},
	}})

	wall, err := client.ForTheWall(ctx, "", time.Now())
	if err != nil {
		t.Fatalf("ForTheWall: %v", err)
	}
	if len(wall.Workloads) != 3 {
		t.Fatalf("workloads = %+v, want three", wall.Workloads)
	}
	want := []string{"zzz-broken", "mmm-limping", "aaa-fine"}
	for i, name := range want {
		if wall.Workloads[i].Name != name {
			t.Errorf("position %d is %q, want %q -- worst first",
				i, wall.Workloads[i].Name, name)
		}
	}
}

// TestOnlyWarningsReachTheWall, and the newest first.
//
// Events answers newest LAST, because that is how a log reads. A wall has room
// for six lines and they have to be the six most recent, so the order is turned
// round for it -- and a wall covered in "Scheduled" and "Pulled" is a wall
// nobody reads.
func TestOnlyWarningsReachTheWall(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	// LastSeen apart, so "newest" is a fact about the fixture rather than about
	// the order it happens to be written in.
	now := time.Now()
	_, client := newCluster(t, kubesim.Options{Events: []kubesim.Event{
		{Type: "Warning", Reason: "FailedScheduling", Kind: "Pod", Name: "old",
			Message: "old one", LastSeen: now.Add(-time.Hour)},
		{Type: "Normal", Reason: "Pulled", Kind: "Pod", Name: "any",
			Message: "pulled an image", LastSeen: now.Add(-time.Minute)},
		{Type: "Warning", Reason: "BackOff", Kind: "Pod", Name: "new",
			Message: "new one", Count: 340, LastSeen: now},
	}})

	wall, err := client.ForTheWall(ctx, "", time.Now())
	if err != nil {
		t.Fatalf("ForTheWall: %v", err)
	}
	if len(wall.Warnings) != 2 {
		t.Fatalf("warnings = %+v, want the two warnings and not the Normal one", wall.Warnings)
	}
	if wall.Warnings[0].Reason != "BackOff" {
		t.Errorf("first warning is %q, want the newest", wall.Warnings[0].Reason)
	}
	// A FailedScheduling seen 340 times is a different situation from one seen
	// once, and from four metres that count is the whole message.
	if wall.Warnings[0].Count != 340 {
		t.Errorf("count = %d, want 340", wall.Warnings[0].Count)
	}
}

// TestTheAnswerSaysWhenItWasTrue.
//
// A wall that cannot go stale lies during exactly the incident it exists for: a
// daemon that died at two leaves a green screen up all night.
func TestTheAnswerSaysWhenItWasTrue(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{})

	at := time.Date(2026, 9, 20, 11, 30, 0, 0, time.UTC)
	wall, err := client.ForTheWall(ctx, "", at)
	if err != nil {
		t.Fatalf("ForTheWall: %v", err)
	}
	if !wall.GeneratedAt.Equal(at) {
		t.Errorf("generated at %v, want %v", wall.GeneratedAt, at)
	}
}

// TestANamespaceNeverHidesANode.
//
// The worst possible failure of this screen: somebody picks a namespace and a
// dead node stops being on the wall.
func TestANamespaceNeverHidesANode(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{
		Nodes: []kubesim.Node{{Name: "cp-1", Ready: corev1.ConditionFalse}},
		Deployments: []kubesim.Deployment{
			{Namespace: "web", Name: "site", Desired: 1, Ready: 1},
			{Namespace: "other", Name: "elsewhere", Desired: 1, Ready: 1},
		},
	})

	wall, err := client.ForTheWall(ctx, "web", time.Now())
	if err != nil {
		t.Fatalf("ForTheWall: %v", err)
	}
	if len(wall.Nodes) != 1 || wall.Nodes[0].State != kube.StateDown {
		t.Errorf("nodes = %+v, want the dead node even in a namespace view", wall.Nodes)
	}
	// The workloads ARE narrowed, which is what the filter is for.
	for _, tile := range wall.Workloads {
		if tile.Namespace != "web" {
			t.Errorf("%s/%s is in the web-only wall", tile.Namespace, tile.Name)
		}
	}
}

// TestTheCornerSentenceLeadsWithTheWorst.
func TestTheCornerSentenceLeadsWithTheWorst(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)

	for _, c := range []struct {
		name string
		opts kubesim.Options
		want string
	}{
		{
			name: "nothing wrong",
			opts: kubesim.Options{Nodes: []kubesim.Node{{Name: "cp-1", Ready: corev1.ConditionTrue}}},
			want: "everything is running",
		},
		{
			name: "something limping",
			opts: kubesim.Options{Deployments: []kubesim.Deployment{
				{Namespace: "default", Name: "site", Desired: 3, Ready: 2},
			}},
			want: "1 need attention",
		},
		{
			// A node nobody is hearing from counts here too: it is not warn, and
			// a sentence that said "everything is running" beside it would be the
			// exact lie this screen exists to prevent.
			name: "a node not reporting",
			opts: kubesim.Options{Nodes: []kubesim.Node{
				{Name: "cp-1", Ready: corev1.ConditionUnknown},
			}},
			want: "1 not running",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, client := newCluster(t, c.opts)
			wall, err := client.ForTheWall(ctx, "", time.Now())
			if err != nil {
				t.Fatalf("ForTheWall: %v", err)
			}
			if got := wall.Describe(); got != c.want {
				t.Errorf("corner says %q, want %q", got, c.want)
			}
		})
	}
}

// TestANamespaceTakesTheWorstThingInIt.
//
// The honeycomb on the right of the wall is one tile per namespace, and its
// colour is the worst thing inside. Named so alphabetical order is the opposite
// of what is wanted, so this cannot pass against no sorting.
func TestANamespaceTakesTheWorstThingInIt(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{Deployments: []kubesim.Deployment{
		{Namespace: "aaa-fine", Name: "one", Desired: 2, Ready: 2},
		{Namespace: "aaa-fine", Name: "two", Desired: 1, Ready: 1},
		{Namespace: "mmm-limping", Name: "postgres", Desired: 3, Ready: 2},
		{Namespace: "zzz-broken", Name: "api", Desired: 2, Ready: 0},
		{Namespace: "zzz-broken", Name: "web", Desired: 1, Ready: 1},
	}})

	wall, err := client.ForTheWall(ctx, "", time.Now())
	if err != nil {
		t.Fatalf("ForTheWall: %v", err)
	}
	if len(wall.Namespaces) != 3 {
		t.Fatalf("namespaces = %+v, want three", wall.Namespaces)
	}

	want := []kube.NamespaceTile{
		{Name: "zzz-broken", Total: 2, State: kube.StateDown, Worst: "api · 0 of 2 ready"},
		{Name: "mmm-limping", Total: 1, State: kube.StateWarn, Worst: "postgres · 2 of 3 ready"},
		{Name: "aaa-fine", Total: 2, State: kube.StateOK},
	}
	for i, expect := range want {
		got := wall.Namespaces[i]
		if got.Name != expect.Name {
			t.Errorf("position %d is %q, want %q -- worst first", i, got.Name, expect.Name)
			continue
		}
		if got.State != expect.State {
			t.Errorf("%s is %q, want %q", got.Name, got.State, expect.State)
		}
		if got.Total != expect.Total {
			t.Errorf("%s counts %d, want %d", got.Name, got.Total, expect.Total)
		}
		// The tile has to name the offender: a colour with no name sends
		// somebody looking, which is the whole thing the wall is meant to save.
		if got.Worst != expect.Worst {
			t.Errorf("%s blames %q, want %q", got.Name, got.Worst, expect.Worst)
		}
	}
}

// TestSomethingStoppedDoesNotColourItsNamespace.
//
// Deliberately stopped is a decision, not a fault. A namespace that went amber
// because an operator paused a job is a namespace that teaches them to ignore
// amber -- but a namespace where EVERYTHING is off is not green either, because
// green claims it is running.
func TestSomethingStoppedDoesNotColourItsNamespace(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{Deployments: []kubesim.Deployment{
		{Namespace: "mixed", Name: "running", Desired: 2, Ready: 2},
		{Namespace: "mixed", Name: "paused", Desired: 0},
		{Namespace: "all-off", Name: "paused", Desired: 0},
	}})

	wall, err := client.ForTheWall(ctx, "", time.Now())
	if err != nil {
		t.Fatalf("ForTheWall: %v", err)
	}
	byName := map[string]kube.NamespaceTile{}
	for _, tile := range wall.Namespaces {
		byName[tile.Name] = tile
	}

	mixed, ok := byName["mixed"]
	if !ok {
		t.Fatalf("namespaces = %+v, want one called mixed", wall.Namespaces)
	}
	if mixed.State != kube.StateOK {
		t.Errorf("mixed is %q, want %q -- a paused workload is not a fault",
			mixed.State, kube.StateOK)
	}
	if mixed.Stopped != 1 {
		t.Errorf("mixed counts %d stopped, want 1", mixed.Stopped)
	}

	off, ok := byName["all-off"]
	if !ok {
		t.Fatalf("namespaces = %+v, want one called all-off", wall.Namespaces)
	}
	if off.State != kube.StateStopped {
		t.Errorf("all-off is %q, want %q -- nothing in it is running",
			off.State, kube.StateStopped)
	}
}
