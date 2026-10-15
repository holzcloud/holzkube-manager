package metrics_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

// The configuration-drift series (D-05). What the tests here hold, in one
// sentence: a number that says "fine" is only ever written for a node that was
// compared, and a cluster is said to be fine only when every node of it was.

// driftItem is one node of a view, in the states the reader produces.
func driftItem(node, state string, since, readAt time.Time) model.NodeConfigDrift {
	return model.NodeConfigDrift{
		Machine: model.MachineID("machine-of-" + node), Node: node, State: state,
		Since: since, ReadAt: readAt, Sentence: "a sentence",
	}
}

// withDrift sets the view of cluster i by hand. nodes is the number of nodes
// the pass covered; the items are the ones the test lists.
func withDrift(f fleet, i, nodes int, items ...model.NodeConfigDrift) fleet {
	if items == nil {
		items = []model.NodeConfigDrift{}
	}
	f.clusters[i].ConfigDrift = &model.ConfigDriftHealth{CheckedAt: now, Nodes: nodes, Items: items}
	return f
}

// mustHave fails when the input of a test does not contain the case the test
// wants to exclude: an exclusion proves nothing about a view that never had it.
func mustHave(t *testing.T, f fleet, i int, node, state string) {
	t.Helper()
	for _, it := range f.clusters[i].ConfigDrift.Items {
		if it.Node == node && it.State == state {
			return
		}
	}
	t.Fatalf("precondition: the view has no node %q in the state %q", node, state)
}

const (
	nodeDrift    = "holzkube_node_config_drift"
	clusterDrift = "holzkube_cluster_config_drift"
	judged       = "holzkube_cluster_config_nodes_judged"
	readAge      = "holzkube_node_config_read_age_seconds"
)

func hasSeries(out, prefix string) bool {
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

func TestADriftingNodeIsOneAndAReadNodeIsZero(t *testing.T) {
	t.Parallel()

	since := now.Add(-2 * time.Hour)
	f := withDrift(build(1), 0, 3,
		driftItem("cp-1", "drift", since, now),
		driftItem("cp-2", "ok", time.Time{}, now),
		driftItem("cp-3", "not-read", time.Time{}, time.Time{}),
	)
	mustHave(t, f, 0, "cp-3", "not-read")
	out := f.render(t)

	if v, ok := sampleValue(out, nodeDrift+`{cluster="c0",node="cp-1"}`); !ok || v != 1 {
		t.Errorf("a drifting node = %v (present %v), want 1", v, ok)
	}
	if v, ok := sampleValue(out, nodeDrift+`{cluster="c0",node="cp-2"}`); !ok || v != 0 {
		t.Errorf("a node that was read and matches = %v (present %v), want 0", v, ok)
	}
	// A drifting node is one for the cluster whatever the others have, and the
	// node nobody has read does not take that back.
	if v, ok := sampleValue(out, clusterDrift+`{cluster="c0"}`); !ok || v != 1 {
		t.Errorf("a cluster with a drifting node = %v (present %v), want 1", v, ok)
	}
	for _, state := range []string{"baseline-created", "settling"} {
		g := withDrift(build(1), 0, 1, driftItem("cp-1", state, time.Time{}, now))
		if v, ok := sampleValue(g.render(t), nodeDrift+`{cluster="c0",node="cp-1"}`); !ok || v != 0 {
			t.Errorf("a node in the state %s = %v (present %v), want 0: it has a verdict", state, v, ok)
		}
	}
}

func TestANodeNeverReadHasNoDriftSeries(t *testing.T) {
	t.Parallel()

	f := withDrift(build(1), 0, 4,
		driftItem("cp-1", "ok", time.Time{}, now),
		driftItem("never", "not-read", time.Time{}, time.Time{}),
		driftItem("silent", "unreadable", time.Time{}, time.Time{}),
		driftItem("damaged", "baseline-damaged", time.Time{}, now),
	)
	mustHave(t, f, 0, "never", "not-read")
	mustHave(t, f, 0, "silent", "unreadable")
	mustHave(t, f, 0, "damaged", "baseline-damaged")
	out := f.render(t)

	if _, ok := sampleValue(out, nodeDrift+`{cluster="c0",node="cp-1"}`); !ok {
		t.Fatal("precondition: the node with a verdict has no series, so the absence below proves nothing")
	}
	for _, node := range []string{"never", "silent", "damaged"} {
		if hasSeries(out, nodeDrift+`{cluster="c0",node="`+node+`"}`) {
			t.Errorf("node %s has a drift series; a node nobody could judge is not \"no drift\"", node)
		}
	}
	if hasSeries(out, readAge+`{cluster="c0",node="never"}`) {
		t.Error("a node never read has a read age")
	}
}

func TestAnUnreadableNodeWithAKnownDriftIsStillOne(t *testing.T) {
	t.Parallel()

	since := now.Add(-30 * time.Hour)
	f := withDrift(build(1), 0, 1, driftItem("cp-1", "unreadable", since, now.Add(-31*time.Hour)))
	mustHave(t, f, 0, "cp-1", "unreadable")
	out := f.render(t)

	if v, ok := sampleValue(out, nodeDrift+`{cluster="c0",node="cp-1"}`); !ok || v != 1 {
		t.Errorf("an unreadable node with a known start = %v (present %v), want 1: it was drifting when it stopped answering", v, ok)
	}
	if v, ok := sampleValue(out, clusterDrift+`{cluster="c0"}`); !ok || v != 1 {
		t.Errorf("the cluster of such a node = %v (present %v), want 1", v, ok)
	}
}

func TestAClusterNobodyHasReadWritesNoConfigSeries(t *testing.T) {
	t.Parallel()

	f := build(2)
	if f.clusters[0].ConfigDrift != nil {
		t.Fatal("precondition: the cluster has a view")
	}
	out := f.render(t)

	for _, name := range []string{nodeDrift, clusterDrift, judged, readAge} {
		if hasSeries(out, name+"{") || hasSeries(out, name+" ") {
			t.Errorf("%s has a series for a cluster nobody has read; \"not read yet\" is not \"0\"", name)
		}
		// The family is still declared, so that the scraper knows the name.
		if !strings.Contains(out, "# HELP "+name+" ") || !strings.Contains(out, "# TYPE "+name+" gauge") {
			t.Errorf("%s is not declared with HELP and TYPE when it has no series", name)
		}
	}
}

func TestAClusterIsZeroOnlyWhenEveryNodeHasAVerdict(t *testing.T) {
	t.Parallel()

	zero := time.Time{}

	// Every node has a verdict and none drifts: 0, the one place a cluster says it.
	full := withDrift(build(1), 0, 2, driftItem("a", "ok", zero, now), driftItem("b", "baseline-created", zero, now))
	if v, found := sampleValue(full.render(t), clusterDrift+`{cluster="c0"}`); !found || v != 0 {
		t.Errorf("full coverage without drift = %v (present %v), want 0", v, found)
	}

	cases := map[string]fleet{
		"one node judged, one never read": withDrift(build(1), 0, 2,
			driftItem("a", "ok", zero, now), driftItem("b", "not-read", zero, time.Time{})),
		"one node judged, one unreadable": withDrift(build(1), 0, 2,
			driftItem("a", "ok", zero, now), driftItem("b", "unreadable", zero, time.Time{})),
		"one node judged, one with a damaged baseline": withDrift(build(1), 0, 2,
			driftItem("a", "ok", zero, now), driftItem("b", "baseline-damaged", zero, now)),
		"a node of the pass missing from the items": withDrift(build(1), 0, 3,
			driftItem("a", "ok", zero, now), driftItem("b", "ok", zero, now)),
		"nothing judged": withDrift(build(1), 0, 2,
			driftItem("a", "not-read", zero, time.Time{}), driftItem("b", "unreadable", zero, time.Time{})),
		"a pass with no node": withDrift(build(1), 0, 0),
	}
	for name, f := range cases {
		if hasSeries(f.render(t), clusterDrift+`{cluster="c0"}`) {
			t.Errorf("%s: the cluster says it has no drift; only a verdict on every node may", name)
		}
	}
}

func TestAClusterWithPartialCoverageIsOneOrAbsent(t *testing.T) {
	t.Parallel()

	zero := time.Time{}

	// A drifting node counts whatever the others did.
	drifting := withDrift(build(1), 0, 3,
		driftItem("a", "drift", now.Add(-time.Hour), now),
		driftItem("b", "not-read", zero, time.Time{}),
		driftItem("c", "unreadable", zero, time.Time{}))
	if v, found := sampleValue(drifting.render(t), clusterDrift+`{cluster="c0"}`); !found || v != 1 {
		t.Errorf("partial coverage with drift = %v (present %v), want 1", v, found)
	}

	// No drift among the nodes that were judged: the series is not 0, it is not there.
	quiet := withDrift(build(1), 0, 3,
		driftItem("a", "ok", zero, now),
		driftItem("b", "ok", zero, now),
		driftItem("c", "not-read", zero, time.Time{}))
	mustHave(t, quiet, 0, "c", "not-read")
	out := quiet.render(t)
	if hasSeries(out, clusterDrift+`{cluster="c0"}`) {
		t.Error("partial coverage without drift writes a series; a cluster is not fine while a node was not looked at")
	}
	if v, found := sampleValue(out, judged+`{cluster="c0"}`); !found || v != 2 {
		t.Errorf("nodes judged = %v (present %v), want 2: an alert rule can see the gap", v, found)
	}
}

func TestNodesJudgedCountsOnlyVerdicts(t *testing.T) {
	t.Parallel()

	zero := time.Time{}
	f := withDrift(build(1, 1), 0, 8,
		driftItem("a", "ok", zero, now),
		driftItem("b", "baseline-created", zero, now),
		driftItem("c", "settling", zero, now),
		driftItem("d", "drift", now.Add(-time.Hour), now),
		driftItem("e", "unreadable", now.Add(-time.Hour), time.Time{}), // a known start is a verdict
		driftItem("f", "not-read", zero, time.Time{}),
		driftItem("g", "unreadable", zero, time.Time{}),
		driftItem("h", "baseline-damaged", zero, now),
	)
	f = withDrift(f, 1, 2,
		driftItem("x", "not-read", zero, time.Time{}), driftItem("y", "unreadable", zero, time.Time{}))
	out := f.render(t)

	if v, found := sampleValue(out, judged+`{cluster="c0"}`); !found || v != 5 {
		t.Errorf("nodes judged = %v (present %v), want 5: ok, baseline-created, settling, drift and the unreadable one with a start", v, found)
	}
	if v, found := sampleValue(out, judged+`{cluster="c1"}`); !found || v != 0 {
		t.Errorf("nodes judged of a cluster with no verdict = %v (present %v), want 0: it is a count, and it is there", v, found)
	}
}

func TestPerNodeConfigSeriesGrowWithTheNodesOfAReadCluster(t *testing.T) {
	t.Parallel()

	// One series per node and family, as for the certificates. The guard of the
	// cardinality of the fleet builds no such view and is not what this is.
	read := func(n int) int {
		items := make([]model.NodeConfigDrift, 0, n)
		for i := range n {
			items = append(items, driftItem(fmt.Sprintf("cp-%d", i), "ok", time.Time{}, now))
		}
		return seriesCount(t, withDrift(build(1), 0, n, items...))
	}
	two, five := read(2), read(5)
	if five-two != 3*2 {
		t.Errorf("three more nodes in a read cluster add %d series, want 6 (the drift and the read age of each)", five-two)
	}
}

func TestReadAgeIsSecondsAndAbsentForANeverReadNode(t *testing.T) {
	t.Parallel()

	zero := time.Time{}
	f := withDrift(build(1), 0, 4,
		driftItem("a", "ok", zero, now.Add(-90*time.Second)),
		driftItem("b", "not-read", zero, time.Time{}),
		driftItem("c", "unreadable", zero, now.Add(-2*time.Hour)), // the last good reading outlives the failure
		driftItem("d", "ok", zero, now.Add(time.Minute)),          // a clock that went back is not a negative age
	)
	mustHave(t, f, 0, "b", "not-read")
	out := f.render(t)

	if v, found := sampleValue(out, readAge+`{cluster="c0",node="a"}`); !found || v != 90 {
		t.Errorf("read age = %v (present %v), want 90", v, found)
	}
	if hasSeries(out, readAge+`{cluster="c0",node="b"}`) {
		t.Error("a node never read has a read age; there is nothing to be old")
	}
	if v, found := sampleValue(out, readAge+`{cluster="c0",node="c"}`); !found || v != 7200 {
		t.Errorf("read age of an unreadable node = %v (present %v), want 7200", v, found)
	}
	if v, found := sampleValue(out, readAge+`{cluster="c0",node="d"}`); !found || v != 0 {
		t.Errorf("read age in the future = %v (present %v), want 0, never negative", v, found)
	}
}

func TestConfigDriftLabelsAreNamesAndNeverMachineIDs(t *testing.T) {
	t.Parallel()

	f := withDrift(build(1), 0, 2,
		model.NodeConfigDrift{Machine: "4c4c4544-0043-4a10-8052-b8c04f4b4d31", Node: "cp-1", State: "ok", ReadAt: now},
		model.NodeConfigDrift{Machine: "4c4c4544-0044-5b21-9163-c9d15f5c5e42", Node: "cp-2", State: "drift", Since: now.Add(-time.Hour), ReadAt: now},
	)
	out := f.render(t)

	if _, found := sampleValue(out, nodeDrift+`{cluster="c0",node="cp-1"}`); !found {
		t.Fatal("precondition: the series with the host name is not there")
	}
	for _, it := range f.clusters[0].ConfigDrift.Items {
		if strings.Contains(out, string(it.Machine)) {
			t.Errorf("machine id %s appears in the export", it.Machine)
		}
	}
}
