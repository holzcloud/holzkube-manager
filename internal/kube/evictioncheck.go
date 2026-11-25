package kube

import (
	"context"
	"fmt"

	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Asking whether a node's pods may go, without moving any of them
// (milestone v1.17, phase 18).
//
// The Kubernetes server decides whether an eviction would violate a
// PodDisruptionBudget: which pods a budget selects, how many it lets go
// (maxUnavailable or minAvailable, as a number or a percentage), what an
// unhealthy pod counts for (unhealthyPodEvictionPolicy), and what two budgets on
// one pod mean. Every one of those is a rule that changes between Kubernetes
// versions, and re-deriving any of them here would be a second implementation
// that is right until the day it is not. So nothing is calculated in this file.
// The question is put to the server in the one form it answers without acting
// on: an eviction with DryRun=All, which the server takes through all of its
// checks and then returns from before it writes a thing.

// EvictionRefusal is one pod the cluster would not let go, and the sentence for
// it. The sentence is the one a drain gives for the same pod, so an operator who
// read it at a gate reads the same words at a removal.
type EvictionRefusal struct {
	// Pod is "namespace/name".
	Pod string `json:"pod"`

	// Reason is why: the budget's own message when the server names one, and
	// otherwise that the cluster could not be asked.
	Reason string `json:"reason"`
}

// EvictionCheck is what asking a node's pods one by one came to.
type EvictionCheck struct {
	// Checked is how many pods were asked about: the ones a drain would try to
	// evict. DaemonSet, static and finished pods are not a decision of this call.
	Checked int `json:"checked"`

	// Refusals are the pods the cluster would not let go, or could not say about.
	Refusals []EvictionRefusal `json:"refusals,omitempty"`

	// Unowned are the pods no controller owns, by name. A drain refuses them and
	// nothing recreates them, so a restart of the node stops them for good; no
	// budget can protect a pod that no controller owns, and they are not asked
	// about. This is information and never a refusal.
	Unowned []string `json:"unowned,omitempty"`
}

// CheckEvictions asks the cluster, once for each pod a drain with these options
// would evict from this node, whether that pod may go. It changes nothing: every
// eviction is a dry run, and a pod that was asked about is exactly where it was.
//
// It is the only call in this package that lets the server apply its budget
// logic without acting on the answer. A pod the server cannot answer for (a 500
// for two budgets, a 403, a network error) is a refusal with that said: that the
// cluster could not be asked is not the same as it being fine to go on, and a
// caller that has to decide must not read it as consent.
//
// An error is returned only when the pods themselves cannot be read, or when
// ctx ended: both mean there is no answer, which is for the caller to word.
func (c *Client) CheckEvictions(ctx context.Context, node string, opts DrainOptions) (EvictionCheck, error) {
	pods, err := c.PodsOnNode(ctx, node)
	if err != nil {
		return EvictionCheck{}, err
	}

	var check EvictionCheck
	for _, p := range pods {
		switch classifyPod(p.pod, opts) {
		case podBare:
			check.Unowned = append(check.Unowned, p.Name())
			continue
		case podEvictable:
		default:
			continue
		}
		check.Checked++

		eviction := &policyv1.Eviction{
			ObjectMeta:    metav1.ObjectMeta{Name: p.pod.Name, Namespace: p.pod.Namespace},
			DeleteOptions: &metav1.DeleteOptions{DryRun: []string{metav1.DryRunAll}},
		}
		err := c.cs.PolicyV1().Evictions(p.pod.Namespace).Evict(ctx, eviction)
		switch {
		case err == nil || apierrors.IsNotFound(err):
			// May go, or is gone already.
		case ctx.Err() != nil:
			return check, ctx.Err()
		default:
			reason, veto := vetoReason(err)
			if !veto {
				reason = fmt.Sprintf("the cluster could not be asked whether this pod may go: %v", err)
			}
			check.Refusals = append(check.Refusals, EvictionRefusal{Pod: p.Name(), Reason: reason})
		}
	}
	return check, nil
}
