package host

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// fixedNow is the instant every fixture test reads at. Not UTC on purpose: the
// View has to convert it, and a zone that is already UTC would hide a missing
// conversion.
var fixedNow = time.Date(2026, 9, 28, 12, 0, 3, 0, time.FixedZone("CEST", 2*60*60))

func tracerSys() fakeSys {
	return fakeSys{uname: Uname{Nodename: "example-host", Release: "6.18.50+rpt-rpi-2712", Machine: "aarch64"}}
}

func newTestCollector(fsys fstest.MapFS, sys Sys) *Collector {
	return New(Config{FS: fsys, Sys: sys, Now: func() time.Time { return fixedNow }})
}

func TestTracerReadsUname(t *testing.T) {
	t.Parallel()

	v := newTestCollector(fstest.MapFS{}, tracerSys()).Read(context.Background())

	if !v.ObservedAt.Equal(fixedNow) || v.ObservedAt.Location() != time.UTC {
		t.Errorf("observed_at = %v, want %v in UTC", v.ObservedAt, fixedNow.UTC())
	}
	if !v.Device.Hostname.Readable || v.Device.Hostname.Value == nil || *v.Device.Hostname.Value != "example-host" {
		t.Errorf("hostname = %+v, want readable example-host", v.Device.Hostname)
	}
	if !v.Device.Kernel.Readable || v.Device.Kernel.Value == nil || *v.Device.Kernel.Value != "6.18.50+rpt-rpi-2712" {
		t.Errorf("kernel = %+v, want readable 6.18.50+rpt-rpi-2712", v.Device.Kernel)
	}

	device, _ := marshalView(t, v)["device"].(map[string]any)
	hostname, _ := device["hostname"].(map[string]any)
	if hostname["value"] != "example-host" || hostname["readable"] != true {
		t.Errorf("device.hostname on the wire = %v", hostname)
	}
}

func TestUnreadableUnameHasNoValue(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		err  error
		code string
	}{
		{"a failing syscall", errors.New("operation not permitted"), CodeReadFailed},
		{"a platform that has none", errUnsupported, CodeUnsupported},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := newTestCollector(fstest.MapFS{}, fakeSys{unameErr: tc.err}).Read(context.Background())
			device, _ := marshalView(t, v)["device"].(map[string]any)

			for _, key := range []string{"hostname", "kernel"} {
				r, _ := device[key].(map[string]any)
				if r["readable"] != false {
					t.Errorf("%s: readable = %v, want false", key, r["readable"])
				}
				if _, has := r["value"]; has {
					t.Errorf("%s carries a value key although it was not read: %v", key, r)
				}
				reason, _ := r["reason"].(map[string]any)
				if reason["code"] != tc.code {
					t.Errorf("%s: reason code = %v, want %s", key, reason["code"], tc.code)
				}
				if msg, _ := reason["message"].(string); msg == "" {
					t.Errorf("%s: the reason has no sentence", key)
				}
			}
		})
	}
}

// readingViews are the shapes the well-formedness walk runs over: everything
// readable, and everything failing.
func readingViews() map[string]View {
	return map[string]View{
		"readable":   newTestCollector(fstest.MapFS{}, tracerSys()).Read(context.Background()),
		"unreadable": newTestCollector(fstest.MapFS{}, fakeSys{unameErr: errors.New("nope"), bootErr: errors.New("nope"), loadsErr: errors.New("nope")}).Read(context.Background()),
	}
}

// TestEveryReadingIsWellFormed is D-02 on the wire: every object that says
// whether it was read either carries a value and no reason, or a reason and no
// value. A zero standing in for an unread value is the one shape this rules out.
func TestEveryReadingIsWellFormed(t *testing.T) {
	t.Parallel()

	for name, v := range readingViews() {
		seen := 0
		walkJSON(marshalView(t, v), "", func(path string, obj map[string]any) {
			readable, has := obj["readable"]
			if !has {
				return
			}
			seen++
			_, hasValue := obj["value"]
			reason, hasReason := obj["reason"]
			switch readable {
			case true:
				if !hasValue || hasReason {
					t.Errorf("%s %s: readable:true needs a value and no reason: %v", name, path, obj)
				}
			case false:
				if hasValue {
					t.Errorf("%s %s: readable:false carries a value key: %v", name, path, obj)
				}
				if _, ok := reason.(map[string]any); !ok {
					t.Errorf("%s %s: readable:false has no reason object: %v", name, path, obj)
				}
			default:
				t.Errorf("%s %s: readable is %v, not a boolean", name, path, readable)
			}
		})
		if seen == 0 {
			t.Fatalf("%s: no reading was found in the view, so this test proves nothing", name)
		}
	}
}

// nullableKeys names every key that may be JSON null, with the reason. Every
// other null is a bug: a nil slice that marshals to null is exactly the default
// this guard exists to catch.
var nullableKeys = map[string]string{
	"rates_over_seconds": "null while there is no usable previous reading (D-12)",
}

func TestNoNulls(t *testing.T) {
	t.Parallel()

	for name, v := range readingViews() {
		var nulls []string
		walkNulls(marshalView(t, v), "", func(path, key string) {
			if _, ok := nullableKeys[key]; !ok {
				nulls = append(nulls, path)
			}
		})
		if len(nulls) > 0 {
			sort.Strings(nulls)
			t.Errorf("%s: null where a value or an empty list belongs:\n  %s", name, strings.Join(nulls, "\n  "))
		}
	}
}

// walkJSON calls visit for every object in a decoded JSON tree.
func walkJSON(node any, path string, visit func(path string, obj map[string]any)) {
	switch n := node.(type) {
	case map[string]any:
		visit(path, n)
		for k, child := range n {
			walkJSON(child, path+"."+k, visit)
		}
	case []any:
		for _, child := range n {
			walkJSON(child, path+"[]", visit)
		}
	}
}

// walkNulls calls found for every null value, with the key it sits under.
func walkNulls(node any, path string, found func(path, key string)) {
	switch n := node.(type) {
	case map[string]any:
		for k, child := range n {
			if child == nil {
				found(path+"."+k, k)
				continue
			}
			walkNulls(child, path+"."+k, found)
		}
	case []any:
		for _, child := range n {
			if child == nil {
				found(path+"[]", "")
				continue
			}
			walkNulls(child, path+"[]", found)
		}
	}
}
