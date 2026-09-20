package kube_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/kubesim"
)

// An empty list is an empty list, and never null (2026-09-20).
//
// # What this exists because of
//
// The operator opened /kubernetes on a cluster whose namespaces have no
// ResourceQuota and got a screenful of zod errors instead of a page. The answer
// carried `"quotas": null`, because a nil slice in Go marshals to null and every
// namespace without a quota had one.
//
// The client's schemas say `.default([])`, which applies to a MISSING field and
// not to an explicit null -- so the parse failed, and a screen that had been
// tested, fixtured, audited and released showed nothing at all.
//
// # Why every test missed it
//
// Every one of them agreed with the others rather than with a cluster. The Go
// tests asserted on the structs and never marshalled them; the component tests
// used fixtures somebody wrote by hand, which naturally had arrays in them; the
// fixture guard validated those same fixtures against the same schemas; and the
// layout audit served those same fixtures again. Four independent-looking checks,
// one shared assumption, and none of them was the product answering a real
// cluster.
//
// So this marshals what the real code paths produce, against a cluster with
// nothing in it, and reads the JSON -- which is the only place the defect was
// ever visible.
//
// # Why an empty cluster is the case to check
//
// A nil slice appears exactly when something is absent, so the answer nobody
// exercises is the one that breaks. "It works on my cluster" is how this shipped.

// TestNoAnswerContainsANullList walks every Kubernetes answer and fails on a null
// anywhere in it.
//
// Deliberately every null rather than only list fields: a null string would be the
// same defect one type along, and there is no field in any of these answers that
// is meant to be null. The check is therefore the whole property rather than a
// list of exceptions that would need maintaining.
func TestNoAnswerContainsANullList(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{})

	// Usage needs a metrics API to exist at all: without one the client answers
	// ErrNoMetrics, which the route turns into "nobody is collecting this" and is
	// a different answer entirely. The case this guard is about is a metrics API
	// that ANSWERS and has nothing in it, so it gets a cluster with one.
	_, measured := newCluster(t, kubesim.Options{Usage: map[string]string{}})

	// Every route's answer, from the empty cluster. Named, because a failure has
	// to say WHICH screen would be blank.
	answers := map[string]func() (any, error){
		"nodes":      func() (any, error) { return client.Nodes(ctx) },
		"pods":       func() (any, error) { return client.Pods(ctx, "") },
		"workloads":  func() (any, error) { return client.Workloads(ctx, "") },
		"resources":  func() (any, error) { return client.Resources(ctx, "") },
		"services":   func() (any, error) { return client.Services(ctx, "") },
		"node usage": func() (any, error) { return measured.NodeUsage(ctx) },
		"pod usage":  func() (any, error) { return measured.PodUsage(ctx, "") },
		"capacity":   func() (any, error) { return client.Capacity(ctx) },
		"storage":    func() (any, error) { return client.Storage(ctx, "") },
		"network":    func() (any, error) { return client.Network(ctx, "") },
		"access":     func() (any, error) { return client.AccessControl(ctx, "") },
		"inventory":  func() (any, error) { return client.Inventory(ctx) },
		"sweep": func() (any, error) {
			return client.PlanSweep(ctx, "", time.Now())
		},
		"events": func() (any, error) { return client.Events(ctx, "") },
	}

	for name, answer := range answers {
		t.Run(name, func(t *testing.T) {
			value, err := answer()
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			encoded, err := json.Marshal(value)
			if err != nil {
				t.Fatalf("marshalling %s: %v", name, err)
			}
			if path := findNull(encoded); path != "" {
				t.Errorf("%s answers null at %s on an empty cluster.\n\n"+
					"A nil slice marshals to null, and the client's schemas default a MISSING "+
					"field rather than an explicit null -- so the screen shows a parse error "+
					"instead of a page. Build the slice with make() or set it before returning.\n\n"+
					"%s", name, path, encoded)
			}
		})
	}
}

// findNull returns a path to the first null in a JSON document, or "".
func findNull(encoded []byte) string {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	// Numbers stay as numbers rather than becoming float64 strings; it changes
	// nothing here and keeps a failure's printed path readable.
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return ""
	}
	return walkForNull(value, "")
}

func walkForNull(value any, path string) string {
	switch typed := value.(type) {
	case nil:
		if path == "" {
			return "the whole answer"
		}
		return path
	case map[string]any:
		for key, item := range typed {
			if found := walkForNull(item, path+"."+key); found != "" {
				return found
			}
		}
	case []any:
		for i, item := range typed {
			if found := walkForNull(item, fmt.Sprintf("%s[%d]", path, i)); found != "" {
				return found
			}
		}
	}
	return ""
}
