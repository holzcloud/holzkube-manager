package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/clustertemplate"
	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// This file is the guard for the one way a client of a JSON API is wrong
// silently.
//
// encoding/json leaves a field whose name it cannot find at its zero value and
// reports nothing. A column decoded from a key the server does not send is
// therefore not blank and not an error: it is a 0, or a "", sitting in a table
// under a heading, looking exactly like an answer. Two of this tool's columns
// were wrong that way when it was first written -- a cluster's node count read
// from "machine_count", and a job's finished steps counted against a state
// string of "succeeded" -- and both printed a confident zero.
//
// The tests below hold every name this tool decodes against the server type
// that produces it. They import the server's packages, which the binary does
// not: a test binary may know where the truth is kept without the shipped tool
// carrying it.

// decoding is one of this tool's read shapes and the type the API builds it
// from.
type decoding struct {
	what   string
	client any
	server any

	// addedByTheHandler are names the response carries that the stored record
	// does not -- a computed membership count, a sentence. They are listed
	// rather than waved through so that adding one is a decision.
	addedByTheHandler []string
}

func TestEveryNameThisToolDecodesIsOneTheServerSends(t *testing.T) {
	for _, d := range []decoding{
		{what: "a machine", client: machineRow{}, server: inventory.MachineView{}},
		{what: "a cluster", client: clusterRow{}, server: inventory.ClusterView{}},
		{what: "a job", client: jobRow{}, server: model.Job{}},
		{what: "a job step", client: jobStepRow{}, server: model.JobStep{}},
		{
			what:   "a machine class",
			client: classRow{},
			server: model.MachineClass{},
			// The class view answers the only question anybody asks about a
			// class -- which machines, right now -- and neither is stored.
			addedByTheHandler: []string{"sentence", "count"},
		},
		{what: "a template plan", client: planRow{}, server: clustertemplate.Plan{}},
		{what: "one side of a plan", client: nodeSetRow{}, server: clustertemplate.NodeSetPlan{}},
	} {
		t.Run(d.what, func(t *testing.T) {
			sent := jsonNames(reflect.TypeOf(d.server))
			for _, name := range d.addedByTheHandler {
				sent[name] = struct{}{}
			}

			for name := range jsonNames(reflect.TypeOf(d.client)) {
				if _, ok := sent[name]; !ok {
					t.Errorf("%s: this tool decodes %q, which the server never sends. "+
						"It will read as a zero value and print as an answer.", d.what, name)
				}
			}
		})
	}
}

// TestTheStepStateThisToolMatchesIsOneTheServerUses. The count of finished
// steps is a string comparison, and a string comparison against a value that
// does not exist never matches and never complains.
func TestTheStepStateThisToolMatchesIsOneTheServerUses(t *testing.T) {
	if stepDone != string(model.StepDone) {
		t.Fatalf("this tool counts steps in state %q; the server calls that state %q",
			stepDone, model.StepDone)
	}
}

// jsonNames is the set of wire names a type serialises to, one level deep.
//
// One level is the right depth: every shape this tool decodes is flat apart
// from the nested rows, and those are listed as their own entries above so
// that a mismatch names the type it is in.
func jsonNames(t reflect.Type) map[string]struct{} {
	names := map[string]struct{}{}
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		tag, ok := f.Tag.Lookup("json")
		if !ok {
			// No tag: encoding/json uses the Go field name verbatim.
			names[f.Name] = struct{}{}
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		switch name {
		case "-":
		case "":
			names[f.Name] = struct{}{}
		default:
			names[name] = struct{}{}
		}
	}
	return names
}
