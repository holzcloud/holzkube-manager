package kube

import (
	"context"
	"errors"
	"strings"
)

// ErrNoNodeFor reports a machine Kubernetes has no node for: one that never
// joined, or one the API server has forgotten.
var ErrNoNodeFor = errors.New("kube: Kubernetes has no node for this machine")

// FindNodeIn finds the node of one machine in a list of nodes: by name first,
// since Talos registers the kubelet under the hostname (case does not matter,
// a hostname is not case-sensitive), then by address, for a cluster that
// overrode the node name.
//
// An empty hostname or address never matches anything. A machine whose
// hostname is not known must not be taken for the first node that has no name
// either.
func FindNodeIn(nodes []Node, hostname, addr string) (Node, bool) {
	if hostname != "" {
		for _, n := range nodes {
			if strings.EqualFold(n.Name, hostname) {
				return n, true
			}
		}
	}
	if addr != "" {
		for _, n := range nodes {
			if n.InternalAddress == addr {
				return n, true
			}
		}
	}
	return Node{}, false
}

// FindNodeFor is the one lookup from a machine to its Kubernetes node. Whoever
// is about to change a node -- a power action, a removal -- asks it here, so
// that two features cannot disagree about which node a machine is.
//
// The node's name comes from the cluster's answer, never from the caller: what
// goes into a later field selector is what the API server called the node.
// ErrNoNodeFor is returned when there is none.
func (c *Client) FindNodeFor(ctx context.Context, hostname, addr string) (Node, error) {
	nodes, err := c.Nodes(ctx)
	if err != nil {
		return Node{}, err
	}
	if n, ok := FindNodeIn(nodes, hostname, addr); ok {
		return n, nil
	}
	return Node{}, ErrNoNodeFor
}
