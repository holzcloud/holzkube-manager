package model_test

import (
	"reflect"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

// TestJobCloneCoversEveryReferenceField is the guard on the fix for window 97.
//
// Clone exists because a model.Job passed by value is not a copy: Steps is a
// slice header and Params is a map, and handing the job to the goroutine that
// runs it while an HTTP handler still holds the same value is two goroutines
// writing and reading one backing array. That was live from phase 6 until the
// race detector caught it on CI.
//
// The obvious way for it to come back is a third reference-typed field added
// to Job by somebody who does not know this method exists. Naming the two
// current fields in a test would not catch that, so this asks the type itself:
// every slice and every map on Job must come back from Clone pointing
// somewhere else.
func TestJobCloneCoversEveryReferenceField(t *testing.T) {
	t.Parallel()

	original := populated(t, reflect.TypeFor[model.Job]())
	clone := original.Interface().(model.Job).Clone() //nolint:errcheck // the type is fixed
	cloned := reflect.ValueOf(clone)

	checked := 0
	typ := reflect.TypeFor[model.Job]()
	for i := range typ.NumField() {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}

		switch field.Type.Kind() {
		case reflect.Slice, reflect.Map:
		default:
			continue
		}

		checked++

		before := original.Field(i)
		after := cloned.Field(i)

		if before.IsNil() || after.IsNil() {
			t.Errorf("%s: one side is nil, so this comparison says nothing. The populator above "+
				"needs to know how to fill a %s", field.Name, field.Type)
			continue
		}
		if before.Pointer() == after.Pointer() {
			t.Errorf("Job.%s is shared with the clone: both point at %#x. A goroutine writing "+
				"through one of them is writing through the other, which is exactly the race "+
				"Clone was added to close", field.Name, before.Pointer())
		}
	}

	if checked == 0 {
		t.Fatal("Job has no slice or map fields, so either the type changed completely or this " +
			"test is looking at the wrong thing")
	}
	t.Logf("checked %d reference-typed fields on model.Job", checked)
}

// populated builds a value of typ with every slice and map field non-empty.
//
// Non-empty matters: an empty slice's backing pointer is not a useful thing to
// compare, and slices.Clone of a nil slice is nil, so a comparison over zero
// values would pass for a field Clone does not touch at all.
func populated(t *testing.T, typ reflect.Type) reflect.Value {
	t.Helper()

	v := reflect.New(typ).Elem()
	for i := range typ.NumField() {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}

		switch field.Type.Kind() {
		case reflect.Slice:
			v.Field(i).Set(reflect.MakeSlice(field.Type, 1, 1))
		case reflect.Map:
			m := reflect.MakeMap(field.Type)
			m.SetMapIndex(reflect.New(field.Type.Key()).Elem(), reflect.New(field.Type.Elem()).Elem())
			v.Field(i).Set(m)
		default:
		}
	}
	return v
}
