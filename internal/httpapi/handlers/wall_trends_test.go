package handlers

import (
	"reflect"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/history"
)

// TestTheWallsClusterCurveIgnoresANodeThatWasOff: the cluster's load is the
// mean of the nodes that answered, so a worker switched off for an hour does
// not drag the curve toward zero for that hour.
func TestTheWallsClusterCurveIgnoresANodeThatWasOff(t *testing.T) {
	t.Parallel()

	got := clusterAverage([]map[string][]history.Point{
		{"cpu": {{60_000, 20}, {120_000, 40}}},
		{"cpu": {{60_000, 60}}}, // off at 120 000
	}, "cpu", "memory")

	want := map[string][]history.Point{"cpu": {{60_000, 40}, {120_000, 40}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cluster average = %v, want %v", got, want)
	}
	if _, ok := got["memory"]; ok {
		t.Fatal("a series no node reported was sent as an empty curve")
	}
}
