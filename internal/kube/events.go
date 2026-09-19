package kube

import (
	"context"
	"fmt"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
)

// Events (2026-09-19).
//
// The question a pod list cannot answer is "why is this Pending". Nothing about
// the pod says it: the answer is that no node had 4 CPUs free, or that the PVC
// it wants has no volume, or that a taint kept it off the only node that did.
// All of that is in the events, and only in the events.
//
// # They expire, and that is load-bearing
//
// A cluster keeps events for an hour by default. An empty list therefore means
// "nothing happened recently" OR "it happened ninety minutes ago", and those are
// different answers. The screen has to say which it cannot distinguish, and the
// route carries the window rather than leaving it implied -- the same
// distinction INV-08 makes about an unanswered node.

// Event is one thing the cluster reported.
type Event struct {
	// Type is Normal or Warning. Warning is the one somebody is looking for.
	Type string `json:"type"`

	// Reason is the cluster's own word: FailedScheduling, BackOff, Unhealthy,
	// Killing, Pulled.
	Reason string `json:"reason"`

	Message string `json:"message"`

	// Object is what it happened to, as "Kind/name".
	Object string `json:"object"`

	// Count is how many times this event repeated. A FailedScheduling seen 340
	// times is a different situation from one seen once.
	Count int32 `json:"count"`

	FirstSeen string `json:"first_seen"`
	LastSeen  string `json:"last_seen"`
}

// Events lists recent events, newest last.
//
// Newest LAST, deliberately, because that is how a log reads and how these are
// understood: the sequence is the story. A screen that reversed them would put
// the consequence above the cause.
func (c *Client) Events(ctx context.Context, namespace string) ([]Event, error) {
	return c.events(ctx, namespace, metav1.ListOptions{Limit: 500})
}

// EventsAbout lists the events for one object, which is what the detail of a
// broken pod shows.
func (c *Client) EventsAbout(ctx context.Context, namespace, kind, name string) ([]Event, error) {
	selector := fields.AndSelectors(
		fields.OneTermEqualSelector("involvedObject.name", name),
		fields.OneTermEqualSelector("involvedObject.kind", kind),
	)
	// The selector is the SERVER's, for the reason the pod list's namespace
	// filter is: a cluster with thousands of events must not send all of them
	// so that four can be shown.
	return c.events(ctx, namespace, metav1.ListOptions{
		FieldSelector: selector.String(),
		Limit:         200,
	})
}

func (c *Client) events(ctx context.Context, namespace string, opts metav1.ListOptions) ([]Event, error) {
	list, err := c.cs.CoreV1().Events(namespace).List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("kube: listing events: %w", err)
	}

	out := make([]Event, 0, len(list.Items))
	for _, e := range list.Items {
		row := Event{
			Type:    e.Type,
			Reason:  e.Reason,
			Message: e.Message,
			Count:   e.Count,
		}
		if e.InvolvedObject.Kind != "" {
			row.Object = e.InvolvedObject.Kind + "/" + e.InvolvedObject.Name
		}
		if !e.FirstTimestamp.IsZero() {
			row.FirstSeen = e.FirstTimestamp.UTC().Format(time.RFC3339)
		}
		row.LastSeen = lastSeen(e.LastTimestamp.Time, e.EventTime.Time, e.FirstTimestamp.Time)
		out = append(out, row)
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].LastSeen < out[j].LastSeen })
	return out, nil
}

// lastSeen picks whichever timestamp the cluster filled in.
//
// Three fields for one fact, because the events API was rewritten: the old one
// sets LastTimestamp, the new one sets EventTime, and an event from either can
// arrive in the same list. Reading only the first would silently date half of
// them to the zero time and sort them to the top.
func lastSeen(last, eventTime, first time.Time) string {
	for _, t := range []time.Time{last, eventTime, first} {
		if !t.IsZero() {
			return t.UTC().Format(time.RFC3339)
		}
	}
	return ""
}
