package kube

import (
	"context"
	"errors"
	"fmt"
	"strings"

	authv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Acting as the operator (2026-09-19).
//
// # The problem this fixes
//
// Every Kubernetes call this product makes arrives at the API server as
// `holzkube-manager`, in the group `system:masters`. That has two consequences
// and both are enterprise blockers.
//
// The cluster's own audit log records the certificate's common name. So a
// cluster that logs everything faithfully records that "holzkube-manager"
// scaled a deployment, for every operator, for ever -- and the one question an
// audit log exists to answer, WHO, is the one it cannot. This product's own
// archive knows; the cluster's does not, and those are read by different people
// for different reasons.
//
// And `system:masters` bypasses RBAC entirely. A cluster cannot express "this
// person may restart pods in `web` and nothing else", because by the time the
// request arrives the person is gone.
//
// # What impersonation does, and the one thing it must never do
//
// The API server accepts `Impersonate-User` and `Impersonate-Group` from a
// client allowed to impersonate. The request is then authorised as that user,
// and the audit log records BOTH: the impersonator and the impersonated. That
// is exactly the pair worth having.
//
// The rule: **a refusal while impersonating is never retried as the admin.** A
// fallback would make the whole thing decorative -- every request would succeed
// either way, the cluster's RBAC would decide nothing, and the audit log would
// show an admin action whenever somebody lacked a role. So a 403 is carried back
// as a 403 and says whose it was.
//
// # Why it is off until it is proven
//
// Turning this on for a cluster whose RBAC has never heard of the operator
// breaks every screen at once. So it is a per-cluster setting, and there is a
// route that ASKS the cluster what the identity may do before anybody switches
// it on -- a preview in the same sense the manifest plan is one.

// ErrImpersonationRefused reports a cluster that refused the identity this
// product acted as.
var ErrImpersonationRefused = errors.New("kube: the cluster refused the identity this acted as")

// Identity is who a request should arrive as.
//
// The zero value means "as this product's own certificate", which is what every
// call did before this existed and what a cluster that has not been set up for
// impersonation still needs.
type Identity struct {
	// User is the name the API server will authorise and log. For a cluster
	// with OIDC that is usually the operator's email or subject, prefixed the
	// way the API server's --oidc-username-prefix says.
	User string

	// Groups are optional. Empty means the API server uses whatever the user
	// maps to, which for an OIDC user is what its groups claim says.
	Groups []string
}

// Empty reports whether this identity asks for anything.
func (i Identity) Empty() bool { return strings.TrimSpace(i.User) == "" }

// String is what appears on a screen and in a refusal.
func (i Identity) String() string {
	if i.Empty() {
		return "holzkube-manager (this product's own certificate)"
	}
	if len(i.Groups) == 0 {
		return i.User
	}
	return i.User + " in " + strings.Join(i.Groups, ", ")
}

// As returns a client whose every call arrives as this identity.
//
// A new clientset rather than a mutated one: a rest.Config is shared by the
// clients built from it, so setting the impersonation on the existing one would
// change the identity of calls already in flight elsewhere -- and the one thing
// worse than acting as the wrong identity is doing it intermittently.
func (c *Client) As(identity Identity) (*Client, error) {
	if identity.Empty() {
		return c, nil
	}

	cfg := rest.CopyConfig(c.cfg)
	cfg.Impersonate = rest.ImpersonationConfig{
		UserName: identity.User,
		Groups:   identity.Groups,
	}

	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("kube: building a client for %s: %w", identity, err)
	}

	out := *c
	out.cs = cs
	out.cfg = cfg
	out.identity = identity

	// The dynamic client and the mapper are rebuilt too, or the manifest path
	// would keep acting as the admin while everything else did not -- which is
	// the worst of both: an audit log that is right about the reads and wrong
	// about the writes.
	dyn, mapper, err := dynamicFor(cfg)
	if err != nil {
		return nil, err
	}
	out.dyn = dyn
	out.mapper = mapper
	return &out, nil
}

// Identity is who this client acts as.
func (c *Client) Identity() Identity { return c.identity }

// Permission is one thing an identity may or may not do.
type Permission struct {
	// Verb and Resource as RBAC names them: "list" on "pods".
	Verb     string `json:"verb"`
	Resource string `json:"resource"`

	// Namespace is empty for a cluster-wide check.
	Namespace string `json:"namespace"`

	Allowed bool `json:"allowed"`

	// Reason is the authoriser's own sentence, when it gives one. An RBAC
	// denial usually does not, which is itself worth showing rather than
	// replacing with a guess.
	Reason string `json:"reason"`
}

// WhatMayI asks the cluster what this client's identity is allowed to do.
//
// Through SelfSubjectAccessReview, which answers for whoever the request
// arrives as -- so on an impersonating client it answers for the impersonated
// user, which is the whole point. Asking is the only honest way: RBAC is a
// cluster's own arrangement of roles and bindings, and any answer computed here
// would be this product's guess about somebody else's configuration.
func (c *Client) WhatMayI(ctx context.Context, checks []Permission) ([]Permission, error) {
	out := make([]Permission, 0, len(checks))
	for _, check := range checks {
		review := &authv1.SelfSubjectAccessReview{
			Spec: authv1.SelfSubjectAccessReviewSpec{
				ResourceAttributes: &authv1.ResourceAttributes{
					Namespace: check.Namespace,
					Verb:      check.Verb,
					Resource:  check.Resource,
				},
			},
		}
		answer, err := c.cs.AuthorizationV1().SelfSubjectAccessReviews().
			Create(ctx, review, metav1.CreateOptions{})
		if err != nil {
			return nil, fmt.Errorf("kube: asking whether %s may %s %s: %w",
				c.identity, check.Verb, check.Resource, err)
		}
		check.Allowed = answer.Status.Allowed
		check.Reason = answer.Status.Reason
		out = append(out, check)
	}
	return out, nil
}

// EveryPermissionThisProductUses is the set a screen previews before anybody
// turns impersonation on.
//
// It is the product's own list rather than a guess at the operator's needs: these
// are the verbs the routes actually issue, so an identity that passes this can
// use the product and one that does not will meet a refusal somewhere. Written
// here once, beside the client that issues them.
func EveryPermissionThisProductUses(namespace string) []Permission {
	return []Permission{
		{Verb: "list", Resource: "pods", Namespace: namespace},
		{Verb: "get", Resource: "pods", Namespace: namespace},
		{Verb: "delete", Resource: "pods", Namespace: namespace},
		{Verb: "get", Resource: "pods/log", Namespace: namespace},
		{Verb: "create", Resource: "pods/eviction", Namespace: namespace},
		{Verb: "list", Resource: "events", Namespace: namespace},
		{Verb: "list", Resource: "deployments", Namespace: namespace},
		{Verb: "update", Resource: "deployments/scale", Namespace: namespace},
		{Verb: "patch", Resource: "deployments", Namespace: namespace},
		{Verb: "list", Resource: "services", Namespace: namespace},
		{Verb: "list", Resource: "nodes"},
		{Verb: "patch", Resource: "nodes"},
	}
}
