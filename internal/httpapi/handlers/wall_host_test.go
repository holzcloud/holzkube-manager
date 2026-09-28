package handlers

import (
	"encoding/json"
	"reflect"
	"testing"
	"testing/fstest"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/history"
	"github.com/holzcloud/holzkube-manager/internal/host"
	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/kube"
)

// TestTheWallsHost (D-14, D-11): the host's tile is its name, its state and a
// short reason; an unreadable host and a stalled sampler both say the fixed
// "not readable"; and the host sits beside the cluster's nodes, never among
// them.
//
// Faults injected and seen red: the staleness check skipped (a snapshot a
// minute old still said "healthy"); newWallAnswer appending the host to
// Wall.Nodes (three nodes, and a summary that counted it).
func TestTheWallsHost(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	fresh := now.Add(-5 * time.Second)
	snap := func(at time.Time, name string, h host.Health) host.Snapshot {
		return host.Snapshot{At: at, Name: name, Health: h}
	}
	unreadable := []string{"Temperatures could not be read: Could not read /sys/class/hwmon: permission denied."}

	for _, tc := range []struct {
		name string
		in   host.Snapshot
		want wallHost
	}{
		{"fresh and ok", snap(fresh, "example-host", host.Health{State: host.HealthOK}),
			wallHost{"example-host", host.HealthOK, "healthy"}},
		{"one warning is the reason", snap(fresh, "example-host", host.Health{State: host.HealthWarn, Warnings: []string{"CPU 84.0 °C ≥ 80 °C"}}),
			wallHost{"example-host", host.HealthWarn, "CPU 84.0 °C ≥ 80 °C"}},
		{"three warnings: the first and a count", snap(fresh, "example-host", host.Health{
			State:    host.HealthWarn,
			Warnings: []string{"CPU 112.0 °C ≥ 110 °C, critical", "/ 91% used ≥ 80%", "rp1_adc 76.0 °C ≥ 75 °C"},
		}), wallHost{"example-host", host.HealthWarn, "CPU 112.0 °C ≥ 110 °C, critical and 2 more"}},
		{"unknown never shows its sentences", snap(fresh, "example-host", host.Health{State: host.HealthUnknown, Summary: unreadable[0], Unreadable: unreadable}),
			wallHost{"example-host", host.HealthUnknown, "not readable"}},
		{"exactly three passes old is still current", snap(now.Add(-3*history.FineStep), "example-host", host.Health{State: host.HealthOK}),
			wallHost{"example-host", host.HealthOK, "healthy"}},
		{"older than three passes is not known, whatever it said", snap(now.Add(-3*history.FineStep-time.Second), "example-host", host.Health{State: host.HealthOK}),
			wallHost{"example-host", host.HealthUnknown, "not readable"}},
		{"a stale warning is not known either", snap(now.Add(-time.Minute), "example-host", host.Health{State: host.HealthWarn, Warnings: []string{"/ 91% used ≥ 80%"}}),
			wallHost{"example-host", host.HealthUnknown, "not readable"}},
		{"no hostname", snap(fresh, "", host.Health{State: host.HealthOK}),
			wallHost{"holzkube-manager host", host.HealthOK, "healthy"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := wallHostFrom(tc.in, now); got == nil || *got != tc.want {
				t.Errorf("wallHostFrom = %+v, want %+v", got, tc.want)
			}
		})
	}

	t.Run("no tile without a host reader or before its first sample", func(t *testing.T) {
		t.Parallel()

		if got := hostForTheWall(httpapi.Deps{}, now); got != nil {
			t.Errorf("without a host reader: %+v, want nil", got)
		}
		unsampled := host.New(host.Config{FS: fstest.MapFS{}})
		if got := hostForTheWall(httpapi.Deps{Host: unsampled}, now); got != nil {
			t.Errorf("before the first sample: %+v, want nil", got)
		}
	})

	t.Run("the host is beside the nodes, not among them", func(t *testing.T) {
		t.Parallel()

		wall := kube.Wall{
			GeneratedAt: now,
			Nodes: []kube.Tile{
				{Kind: "Node", Name: "cp-1", State: kube.StateOK},
				{Kind: "Node", Name: "worker-1", State: kube.StateWarn},
			},
			Summary: map[kube.State]int{kube.StateOK: 1, kube.StateWarn: 1},
		}
		answer := func(h *wallHost) map[string]json.RawMessage {
			t.Helper()
			raw, err := json.Marshal(newWallAnswer(wall, wallTrends{}, h))
			if err != nil {
				t.Fatal(err)
			}
			var out map[string]json.RawMessage
			if err := json.Unmarshal(raw, &out); err != nil {
				t.Fatal(err)
			}
			return out
		}

		got := answer(&wallHost{Name: "example-host", State: host.HealthWarn, Reason: "/ 91% used ≥ 80%"})
		var nodes []kube.Tile
		if err := json.Unmarshal(got["nodes"], &nodes); err != nil || len(nodes) != 2 {
			t.Errorf("nodes = %s, want the cluster's two", got["nodes"])
		}
		// A literal, not wall.Summary: the map is shared with the answer, and
		// a fault that counted the host there would change both sides.
		var summary map[kube.State]int
		if want := map[kube.State]int{kube.StateOK: 1, kube.StateWarn: 1}; json.Unmarshal(got["summary"], &summary) != nil || !reflect.DeepEqual(summary, want) {
			t.Errorf("summary = %s, want the cluster's %v", got["summary"], want)
		}
		var tile wallHost
		if err := json.Unmarshal(got["host"], &tile); err != nil || tile.Name != "example-host" {
			t.Errorf("host = %s, want the host's tile", got["host"])
		}

		if h, present := answer(nil)["host"]; !present || string(h) != "null" {
			t.Errorf("without a host: host = %s (present %v), want null", h, present)
		}
	})
}
