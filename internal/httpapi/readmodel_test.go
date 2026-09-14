package httpapi_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/clustertemplate"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/scale"
	"github.com/holzcloud/holzkube-manager/internal/upgrade"
)

// TestNoReadModelSendsNullForAList.
//
// The contract says it in the inventory handler -- "Never null: a null reads to
// a client as 'the server did not check', which is a weaker claim than 'there
// are none'" -- and four read models broke it in their emptiest case, which is
// the case an operator meets first.
//
// Go marshals a nil slice as `null`. The browser's schemas declare these fields
// as `z.array(...).default([])`, and a zod default applies to `undefined` and
// not to `null`, so the parse throws. What the operator sees is not an error:
// TanStack Query retries, the panel sits on its loading line, and nothing in
// the console or the server log says anything. A 200 went out and the screen
// stayed blank.
//
// That is how it was found -- clicking the button in a browser against a real
// server -- and not by any test here, because every test in this repository
// supplies its own fixtures with the lists populated.
//
// The fields are listed rather than derived from the struct, so that adding a
// list to a read model is a decision somebody makes here too.
func TestNoReadModelSendsNullForAList(t *testing.T) {
	t.Parallel()

	// Each entry is what the real function returns in its emptiest case: a
	// cluster with no machines, a template that resolves to nothing, a plan
	// over no nodes. Not a zero value -- a zero value would prove the struct
	// literal is fine and say nothing about the code that builds one.
	cases := []struct {
		what   string
		value  any
		fields []string
	}{
		{
			what:   "a scale plan for a cluster with no machines",
			value:  scale.Make(scale.Input{Cluster: model.Cluster{ID: "c1", Name: "homelab"}}),
			fields: []string{"removals", "additions", "advice"},
		},
		{
			what: "a template plan that resolves to nothing",
			value: clustertemplate.Make(
				clustertemplate.Template{Kind: clustertemplate.Kind, Name: "homelab"},
				clustertemplate.Fleet{},
			),
			fields: []string{"problems", "notes"},
		},
		{
			what:   "a template node set that resolves to no machines",
			value:  clustertemplate.Make(clustertemplate.Template{Kind: clustertemplate.Kind, Name: "homelab"}, clustertemplate.Fleet{}).ControlPlane,
			fields: []string{"machines"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.what, func(t *testing.T) {
			raw, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			var back map[string]any
			if err := json.Unmarshal(raw, &back); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			for _, f := range tc.fields {
				v, present := back[f]
				if !present {
					t.Errorf("%q is missing entirely; the browser's schema expects it", f)
					continue
				}
				if v == nil {
					t.Errorf("%q is null. Go marshals a nil slice that way, and the browser's "+
						"schema declares it z.array(...).default([]) -- a zod default applies to "+
						"undefined and not to null, so the parse throws, the query retries, and "+
						"the panel sits on its loading line saying nothing.\n  %s", f, raw)
				}
			}
		})
	}
}

// TestEveryListInAReadModelIsAccountedFor.
//
// The list above is written by hand, which means it can fall behind. This walks
// the same types and fails on a slice field that carries neither omitempty nor
// a line in the table above -- so a new list is a decision rather than an
// omission.
func TestEveryListInAReadModelIsAccountedFor(t *testing.T) {
	t.Parallel()

	known := map[string]bool{
		"scale.Plan.removals": true, "scale.Plan.additions": true, "scale.Plan.advice": true,
		"clustertemplate.Plan.problems": true, "clustertemplate.Plan.notes": true,
		"clustertemplate.NodeSetPlan.machines": true,
		"upgrade.Plan.nodes":                   true,
	}

	for _, v := range []any{
		scale.Plan{}, clustertemplate.Plan{}, clustertemplate.NodeSetPlan{}, upgrade.Plan{},
	} {
		typ := reflect.TypeOf(v)
		path := typ.PkgPath()
		pkg := path[strings.LastIndex(path, "/")+1:]

		for i := range typ.NumField() {
			f := typ.Field(i)
			if f.Type.Kind() != reflect.Slice {
				continue
			}
			tag, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if tag == "" || tag == "-" || strings.Contains(f.Tag.Get("json"), "omitempty") {
				continue
			}
			key := pkg + "." + typ.Name() + "." + tag
			if !known[key] {
				t.Errorf("%s is a list on the wire and is not in this test's table. A nil one "+
					"marshals to null, which the browser's schemas refuse.", key)
			}
		}
	}
}
