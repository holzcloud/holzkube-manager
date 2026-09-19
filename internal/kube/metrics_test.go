package kube_test

import (
	"errors"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/kubesim"
)

// What is actually being used (2026-09-19).
//
// Two claims, and the first is the one that matters on most clusters: Talos does
// not ship metrics-server, so "nobody is collecting this" is the ordinary answer
// and has to be distinguishable from "usage is zero".

// TestAClusterWithoutMetricsSaysSoRatherThanShowingZero.
func TestAClusterWithoutMetricsSaysSoRatherThanShowingZero(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	// No Usage map at all, which is what most clusters are.
	_, client := newCluster(t, kubesim.Options{})

	_, err := client.NodeUsage(ctx)
	if !errors.Is(err, kube.ErrNoMetrics) {
		t.Fatalf("err = %v, want ErrNoMetrics", err)
	}
	if _, err := client.PodUsage(ctx, "default"); !errors.Is(err, kube.ErrNoMetrics) {
		t.Errorf("pod usage = %v, want ErrNoMetrics", err)
	}
}

// TestAPodsUsageIsTheSumOfItsContainers.
//
// The fake splits every pod's usage across two containers precisely so a reader
// that took only the first would understate it. `kubectl top pod` sums them, and
// a screen that did not would report a sidecar-heavy pod as idle.
func TestAPodsUsageIsTheSumOfItsContainers(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{Usage: map[string]string{
		"pod/default/website-abc": "400m,512Mi",
	}})

	usage, err := client.PodUsage(ctx, "default")
	if err != nil {
		t.Fatalf("PodUsage: %v", err)
	}
	if len(usage) != 1 {
		t.Fatalf("usage = %+v", usage)
	}
	if usage[0].CPUMillis != 400 {
		t.Errorf("cpu = %dm, want the sum across containers rather than the first one",
			usage[0].CPUMillis)
	}
	if usage[0].MemoryBytes != 512*1024*1024 {
		t.Errorf("memory = %d bytes, want the sum across containers", usage[0].MemoryBytes)
	}
}

// TestNodeUsageIsReadPerNode, and the namespace filter is the server's.
func TestNodeUsageIsReadPerNode(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	sim, client := newCluster(t, kubesim.Options{Usage: map[string]string{
		"node/cp-1":               "1200m,3Gi",
		"node/worker-2":           "300m,1Gi",
		"pod/default/website-abc": "400m,512Mi",
		"pod/db/postgres-0":       "800m,2Gi",
	}})

	nodes, err := client.NodeUsage(ctx)
	if err != nil {
		t.Fatalf("NodeUsage: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("nodes = %+v", nodes)
	}
	if nodes[0].Name != "cp-1" || nodes[0].CPUMillis != 1200 {
		t.Errorf("first node = %+v", nodes[0])
	}
	// The string form is what a screen shows, beside the number it compares.
	if nodes[0].CPU != "1200m" {
		t.Errorf("cpu string = %q", nodes[0].CPU)
	}

	one, err := client.PodUsage(ctx, "db")
	if err != nil {
		t.Fatalf("PodUsage: %v", err)
	}
	if len(one) != 1 || one[0].Name != "postgres-0" {
		t.Errorf("filtered pods = %+v, want only the namespace asked for", one)
	}
	// The filter is the SERVER's: a cluster with ten thousand pods must not send
	// all of them so that one namespace can be shown.
	if got := sim.Calls("GET /apis/metrics.k8s.io/v1beta1/namespaces/db/pods"); got != 1 {
		t.Errorf("the namespaced metrics path was requested %d times", got)
	}
}
