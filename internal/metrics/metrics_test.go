package metrics_test

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/health"
	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/metrics"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// now is the clock every test here runs on, so a certificate's remaining
// seconds are arithmetic rather than a race with the wall clock.
var now = time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)

type fleet struct {
	clusters []inventory.ClusterView
	machines []inventory.MachineView
	jobs     []model.Job
	chainOK  bool
}

func (f fleet) render(t *testing.T) string {
	t.Helper()

	e := metrics.New(metrics.Deps{
		Machines:         func(context.Context) ([]inventory.MachineView, error) { return f.machines, nil },
		Clusters:         func(context.Context) ([]inventory.ClusterView, error) { return f.clusters, nil },
		Jobs:             func(context.Context) ([]model.Job, error) { return f.jobs, nil },
		AuditChainIntact: func() bool { return f.chainOK },
		Now:              func() time.Time { return now },
	})

	var b strings.Builder
	if err := e.Write(t.Context(), &b); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return b.String()
}

// build makes a fleet of the given shape: one cluster per entry, each with that
// many machines.
func build(nodesPerCluster ...int) fleet {
	f := fleet{chainOK: true}

	for i, n := range nodesPerCluster {
		id := model.ClusterID(fmt.Sprintf("c%d", i))
		f.clusters = append(f.clusters, inventory.ClusterView{
			ID:                 id,
			Name:               fmt.Sprintf("cluster %d", i),
			Origin:             model.OriginImported,
			ClientCertNotAfter: now.Add(48 * time.Hour),
		})
		for j := range n {
			f.machines = append(f.machines, inventory.MachineView{
				ID:      model.MachineID(fmt.Sprintf("c%d-m%d", i, j)),
				Cluster: id,
				Stage:   health.StageWatching,
				Watch:   inventory.WatchStatus{Live: true},
				EtcdMember: health.Field[bool]{
					Value: j < 3, Level: health.LevelEtcd, Available: true,
				},
			})
		}
	}
	return f
}

// TestEveryMetricCarriesHelpAndType is criterion 2.
//
// A metric without a HELP is a number whose meaning lives in somebody's head,
// and a metric without a TYPE is one a query engine will average where it
// should have rated. The check is derived from the output rather than from a
// list: a metric added later without its two lines fails here without anybody
// having to remember to add it to a list.
func TestEveryMetricCarriesHelpAndType(t *testing.T) {
	t.Parallel()

	out := build(3, 2).render(t)

	helped := map[string]bool{}
	typed := map[string]bool{}
	samples := map[string]bool{}

	for _, line := range strings.Split(out, "\n") {
		switch {
		case line == "":
		case strings.HasPrefix(line, "# HELP "):
			name, text, _ := strings.Cut(strings.TrimPrefix(line, "# HELP "), " ")
			helped[name] = true
			if strings.TrimSpace(text) == "" {
				t.Errorf("%s has an empty HELP, which is the same as having none", name)
			}
		case strings.HasPrefix(line, "# TYPE "):
			name, typ, _ := strings.Cut(strings.TrimPrefix(line, "# TYPE "), " ")
			typed[name] = true
			switch typ {
			case "gauge", "counter", "histogram", "summary", "untyped":
			default:
				t.Errorf("%s is declared TYPE %q, which is not a Prometheus type", name, typ)
			}
		default:
			samples[sampleName(line)] = true
		}
	}

	if len(samples) == 0 {
		t.Fatal("the export produced no samples, so this test proves nothing")
	}

	// The structure a hand-rolled writer gets wrong: a family declared twice,
	// or a sample that appears before the TYPE line that governs it. Both are
	// rejected by a real parser, and both are invisible to a test that only
	// checks that the lines exist somewhere.
	assertFamiliesAreWellFormed(t, out)
	for name := range samples {
		if !helped[name] {
			t.Errorf("%s is exported with no HELP line", name)
		}
		if !typed[name] {
			t.Errorf("%s is exported with no TYPE line", name)
		}
	}

	// The requirement's own example of what not to build. A gauge holding the
	// last value of something that varies over time answers "what was it when
	// somebody last looked", which is the question Prometheus exists to stop
	// people asking.
	for _, forbidden := range []string{"last_", "_last", "_duration_seconds", "_latest"} {
		for name := range samples {
			if strings.Contains(name, forbidden) {
				t.Errorf("%s looks like a gauge standing in for a time series (%q); the thing that "+
					"keeps history is the scraper, not this process", name, forbidden)
			}
		}
	}
}

// TestNoMachineEverBecomesALabelValue is the other half of criterion 1.
//
// A per-node label multiplies every series by the fleet, and the fleet is the
// axis that grows. It is also the wrong question: what an operator graphs is
// "how many nodes are down", and a dashboard is what answers "which one".
func TestNoMachineEverBecomesALabelValue(t *testing.T) {
	t.Parallel()

	f := build(4)
	out := f.render(t)

	for _, m := range f.machines {
		if strings.Contains(out, string(m.ID)) {
			t.Errorf("machine %s appears in the export; a node in a label value is the fleet "+
				"multiplied into the cardinality", m.ID)
		}
	}
}

// TestCardinalityDoesNotGrowWithTheFleet is criterion 4.
//
// The claim is stronger than the requirement asks for and it is the one the
// design actually makes: adding nodes adds *no* series at all, and adding a
// cluster adds the same fixed number every time. A multiplicative export would
// fail the first half; one that merely avoided a nodes x clusters label pair
// would pass the first and fail the second.
func TestCardinalityDoesNotGrowWithTheFleet(t *testing.T) {
	t.Parallel()

	small := seriesCount(t, build(3))
	large := seriesCount(t, build(300))

	if small != large {
		t.Errorf("a cluster of 3 nodes exports %d series and one of 300 exports %d. Cardinality "+
			"that follows the fleet is cardinality that follows the axis which grows", small, large)
	}

	one := seriesCount(t, build(5))
	two := seriesCount(t, build(5, 5))
	three := seriesCount(t, build(5, 5, 5))

	if two-one != three-two {
		t.Errorf("clusters cost %d series for the second and %d for the third; the growth is not "+
			"linear, which means something in here multiplies two labels together",
			two-one, three-two)
	}
	if two-one <= 0 {
		t.Fatalf("adding a cluster added %d series, so this test is measuring nothing", two-one)
	}
}

// TestAStageWithNoMachinesIsStillReported is the zero-series rule.
//
// Prometheus cannot tell a series that stopped being reported from a target
// that went away, so a "nodes down" graph that simply ends when the number
// reaches zero shows its last non-zero value for as long as anybody looks at
// it -- on the dashboard that exists for the outage, after the outage.
func TestAStageWithNoMachinesIsStillReported(t *testing.T) {
	t.Parallel()

	out := build(2).render(t)

	for _, st := range health.Stages() {
		want := fmt.Sprintf(`holzkube_machines{cluster="c0",stage=%q}`, st.String())
		if !strings.Contains(out, want) {
			t.Errorf("stage %q is not reported for a cluster that has no machines in it; a zero "+
				"that is absent reads as a value that is still whatever it last was", st)
		}
	}

	// And every job state, for a kind that has any jobs at all.
	f := build(1)
	f.jobs = []model.Job{{ID: "j1", Kind: model.JobReboot, State: model.JobSucceeded}}
	out = f.render(t)

	for _, st := range model.JobStates() {
		want := fmt.Sprintf(`holzkube_jobs{kind="node.reboot",state=%q}`, string(st))
		if !strings.Contains(out, want) {
			t.Errorf("job state %q is not reported for a kind that has jobs", st)
		}
	}
}

// TestAnExpiredCertificateIsANegativeNumberAndNotAMissingMetric is criterion 3.
//
// An expired client certificate takes every node in a cluster down in the same
// second (D-23). A series that vanished at expiry would go blank at exactly the
// moment somebody needed it, and "no data" is the one reading an alert rule
// cannot act on.
func TestAnExpiredCertificateIsANegativeNumberAndNotAMissingMetric(t *testing.T) {
	t.Parallel()

	f := build(1)
	f.clusters[0].ClientCertNotAfter = now.Add(-90 * time.Minute)

	out := f.render(t)

	v, ok := sampleValue(out, `holzkube_cluster_client_certificate_seconds{cluster="c0"}`)
	if !ok {
		t.Fatal("an expired certificate produced no metric at all, which is the reading an alert " +
			"rule cannot act on")
	}
	if v != -5400 {
		t.Errorf("remaining seconds = %v, want -5400", v)
	}
}

// TestOnlyAConfirmedEtcdMembershipCounts is the one number where a hopeful
// answer is worse than none.
//
// A node whose etcd level is stale is a node whose *last known* answer was
// "member". Counting it reports a quorum this instance cannot currently see,
// and a quorum figure that is optimistic is how somebody removes the member
// that was holding the cluster up.
func TestOnlyAConfirmedEtcdMembershipCounts(t *testing.T) {
	t.Parallel()

	f := build(3)
	// Three members, but one of them has not been confirmed since the outage
	// started.
	f.machines[2].EtcdMember = health.Field[bool]{Value: true, Level: health.LevelEtcd, Available: false}

	out := f.render(t)

	v, ok := sampleValue(out, `holzkube_cluster_etcd_members_confirmed{cluster="c0"}`)
	if !ok {
		t.Fatal("no etcd membership metric was exported")
	}
	if v != 2 {
		t.Errorf("confirmed etcd members = %v, want 2: the third node's membership is a remembered "+
			"answer and not a seen one", v)
	}
}

// TestAMachineWithNoClusterIsStillCounted is the fleet total staying honest.
//
// A machine belonging to no cluster is every machine in maintenance mode, and
// every machine whose cluster was removed. Dropping them from the export would
// make this page disagree with the screen about how many nodes exist, and the
// ones it dropped are the ones somebody is in the middle of adopting.
func TestAMachineWithNoClusterIsStillCounted(t *testing.T) {
	t.Parallel()

	f := build(2)
	f.machines = append(f.machines, inventory.MachineView{
		ID:    "orphan-1",
		Stage: health.StageDown,
	})

	out := f.render(t)

	v, ok := sampleValue(out, `holzkube_machines{cluster="",stage="down"}`)
	if !ok {
		t.Fatal("a machine belonging to no cluster is not counted anywhere in the export")
	}
	if v != 1 {
		t.Errorf("unassigned machines in the down stage = %v, want 1", v)
	}
}

// TestALabelValueCannotEndTheLineEarly is the escaping.
//
// A cluster's name is an operator's free text. An unescaped quote in one would
// not merely corrupt that series: it truncates the sample line and everything
// the scraper reads after it becomes nonsense, so one badly named cluster takes
// out the whole export.
func TestALabelValueCannotEndTheLineEarly(t *testing.T) {
	t.Parallel()

	f := build(1)
	f.clusters[0].Name = `he said "hello"` + "\n" + `and C:\went\home`

	out := f.render(t)

	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "holzkube_cluster_info") {
			continue
		}
		if strings.Count(line, `"`)%2 != 0 {
			t.Errorf("the info line has an odd number of quotes, so a value ended it early:\n  %s", line)
		}
		if !strings.Contains(line, `\"hello\"`) {
			t.Errorf("the quotes in the cluster name were not escaped:\n  %s", line)
		}
		if !strings.Contains(line, `\\went`) {
			t.Errorf("the backslashes in the cluster name were not escaped:\n  %s", line)
		}
		return
	}
	t.Fatal("no holzkube_cluster_info line was exported")
}

// sampleName is the metric name of a sample line, labels stripped.
func sampleName(line string) string {
	name, _, found := strings.Cut(line, "{")
	if found {
		return name
	}
	name, _, _ = strings.Cut(line, " ")
	return name
}

// sampleValue finds one sample by its exact name-and-labels prefix.
func sampleValue(out, series string) (float64, bool) {
	for _, line := range strings.Split(out, "\n") {
		rest, found := strings.CutPrefix(line, series+" ")
		if !found {
			continue
		}
		v, err := strconv.ParseFloat(rest, 64)
		if err != nil {
			return 0, false
		}
		return v, true
	}
	return 0, false
}

func seriesCount(t *testing.T, f fleet) int {
	t.Helper()

	n := 0
	for _, line := range strings.Split(f.render(t), "\n") {
		if line != "" && !strings.HasPrefix(line, "#") {
			n++
		}
	}
	return n
}

// assertFamiliesAreWellFormed checks the exposition's grammar rather than its
// content: each family's HELP and TYPE appear once, and before its samples.
//
// It was written after a real Prometheus parser was pointed at a live scrape of
// this endpoint and accepted it. This is the part of that verification that can
// run without one.
func assertFamiliesAreWellFormed(t *testing.T, out string) {
	t.Helper()

	helps := map[string]int{}
	types := map[string]int{}
	sampled := map[string]bool{}

	for _, line := range strings.Split(out, "\n") {
		switch {
		case line == "":
		case strings.HasPrefix(line, "# HELP "):
			name, _, _ := strings.Cut(strings.TrimPrefix(line, "# HELP "), " ")
			helps[name]++
			if sampled[name] {
				t.Errorf("%s has a HELP line after its samples; a parser reads the family as "+
					"declared twice", name)
			}
		case strings.HasPrefix(line, "# TYPE "):
			name, _, _ := strings.Cut(strings.TrimPrefix(line, "# TYPE "), " ")
			types[name]++
			if sampled[name] {
				t.Errorf("%s has a TYPE line after its samples", name)
			}
		default:
			sampled[sampleName(line)] = true
		}
	}

	for name, n := range helps {
		if n != 1 {
			t.Errorf("%s has %d HELP lines; a family declared twice is a family a parser rejects", name, n)
		}
	}
	for name, n := range types {
		if n != 1 {
			t.Errorf("%s has %d TYPE lines", name, n)
		}
	}
}
