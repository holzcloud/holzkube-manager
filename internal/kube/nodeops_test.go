package kube_test

import (
	"errors"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/kubesim"
)

// TestFindNodeForMatchesTheHostnameAnyCase: a hostname is not case-sensitive,
// and Talos registers the kubelet under the hostname, so a machine recorded as
// w-1 is the node the API server lists as W-1.
func TestFindNodeForMatchesTheHostnameAnyCase(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{Nodes: []kubesim.Node{{Name: "cp-1"}, {Name: "W-1"}}})

	node, err := client.FindNodeFor(ctx, "w-1", "")
	if err != nil {
		t.Fatalf("FindNodeFor: %v", err)
	}
	if node.Name != "W-1" {
		t.Errorf("found %q; the name must be the one the cluster gave, not the one asked for", node.Name)
	}
}

// TestFindNodeForFallsBackToTheAddress: a cluster that overrode the node name
// is still found, by the address its kubelet registered.
func TestFindNodeForFallsBackToTheAddress(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{Nodes: []kubesim.Node{
		{Name: "cp-1", InternalAddress: "192.168.1.11"},
		{Name: "node-a", InternalAddress: "192.168.1.10"},
	}})

	node, err := client.FindNodeFor(ctx, "not-the-node-name", "192.168.1.10")
	if err != nil {
		t.Fatalf("FindNodeFor: %v", err)
	}
	if node.Name != "node-a" {
		t.Errorf("found %q, want node-a: the one with that address", node.Name)
	}
}

// TestFindNodeForSaysWhenThereIsNoNode: a machine that never joined has no
// node, and that is a named condition rather than a lookup failure.
func TestFindNodeForSaysWhenThereIsNoNode(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{Nodes: []kubesim.Node{{Name: "cp-1", InternalAddress: "192.168.1.11"}}})

	_, err := client.FindNodeFor(ctx, "w-9", "192.168.1.19")
	if !errors.Is(err, kube.ErrNoNodeFor) {
		t.Fatalf("FindNodeFor for a machine with no node returned %v, want ErrNoNodeFor", err)
	}
}

// TestFindNodeForNeverMatchesAnEmptyHostname: a machine whose hostname is not
// known matches no node. Without the rule it would match the first node that
// has no name or no address either, and a removal would drain the wrong one.
func TestFindNodeForNeverMatchesAnEmptyHostname(t *testing.T) {
	t.Parallel()

	nodes := []kube.Node{{Name: ""}, {Name: "cp-1", InternalAddress: ""}}
	if n, ok := kube.FindNodeIn(nodes, "", ""); ok {
		t.Fatalf("FindNodeIn with no hostname and no address matched %+v", n)
	}
}
