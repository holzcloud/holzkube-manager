package httpapi_test

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// The hardware route's contract, as the frontend reads it: the exact key set
// at every level, arrays that are empty rather than null, and nulls exactly
// where a number is unknown. It is checked on the decoded JSON rather than on
// the Go types, because the Go types are what could drift from the contract
// without anything in this package noticing -- a renamed tag compiles.

var (
	hardwareKeys = []string{
		"machine", "hostname", "observed_at", "uptime_seconds", "rates_over_seconds",
		"cpu", "memory", "filesystems", "disks", "network", "temperatures", "fans", "sensors_notice",
	}
	hardwareCPUKeys = []string{
		"model", "cores", "threads", "usage_percent", "iowait_percent", "per_core", "load1", "load5", "load15",
	}
	hardwareMemoryKeys = []string{
		"total_bytes", "used_bytes", "cache_bytes", "available_bytes", "swap_total_bytes", "swap_used_bytes",
	}
	hardwareFilesystemKeys  = []string{"mount", "device", "size_bytes", "used_bytes"}
	hardwareDiskKeys        = []string{"name", "model", "size_bytes", "read_bytes_per_sec", "write_bytes_per_sec", "temperature_c"}
	hardwareLinkKeys        = []string{"name", "up", "speed_mbit", "rx_bytes_per_sec", "tx_bytes_per_sec"}
	hardwareTemperatureKeys = []string{"chip", "kind", "label", "celsius", "high_c", "critical_c"}
	hardwareFanKeys         = []string{"chip", "label", "rpm"}
)

// firstMachineID is the one machine the harness's adoption recorded.
func firstMachineID(t *testing.T, c *inventoryHarness) string {
	t.Helper()

	resp, raw := c.do(t, http.MethodGet, "/api/v1/machines", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("machines: %d (%s)", resp.StatusCode, raw)
	}
	var list struct {
		Machines []struct {
			ID string `json:"id"`
		} `json:"machines"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("decode machines: %v (%s)", err, raw)
	}
	if len(list.Machines) != 1 {
		t.Fatalf("machines = %d, want 1", len(list.Machines))
	}
	return list.Machines[0].ID
}

func requireKeys(t *testing.T, where string, obj any, want []string) map[string]any {
	t.Helper()

	m, ok := obj.(map[string]any)
	if !ok {
		t.Fatalf("%s is %T, want an object", where, obj)
	}
	got := make([]string, 0, len(m))
	for k := range m {
		got = append(got, k)
	}
	sort.Strings(got)
	exp := append([]string(nil), want...)
	sort.Strings(exp)
	if strings.Join(got, ",") != strings.Join(exp, ",") {
		t.Errorf("%s has keys %v, want exactly %v", where, got, exp)
	}
	return m
}

// requireArray fails on null: a missing list reads to a client as "the server
// did not check", which is a weaker claim than "there are none".
func requireArray(t *testing.T, where string, v any) []any {
	t.Helper()

	arr, ok := v.([]any)
	if !ok {
		t.Fatalf("%s is %v (%T), want an array -- empty rather than null", where, v, v)
	}
	return arr
}

// TestHardwareRouteAnswersTheContractShape walks the contract on a desktop
// board and on a node whose kernel exposes no sensors, which is the case where
// a null would otherwise appear.
func TestHardwareRouteAnswersTheContractShape(t *testing.T) {
	for _, tc := range []struct {
		name    string
		sensors talossim.SensorProfile
	}{
		{"a desktop board", talossim.SensorsDesktop},
		{"a node with no sensors", talossim.SensorsNone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newInventoryHarnessWith(t, talossim.Options{Sensors: tc.sensors})
			c.adopt(t)
			id := firstMachineID(t, c)

			resp, raw := c.do(t, http.MethodGet, "/api/v1/machines/"+id+"/hardware", nil)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("hardware: %d (%s)", resp.StatusCode, raw)
			}
			if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("content-type = %q, want application/json", ct)
			}
			if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
				t.Errorf("cache-control = %q; a cached gauge is a stale one", cc)
			}

			var body any
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Fatalf("decode: %v (%s)", err, raw)
			}
			v := requireKeys(t, "the response", body, hardwareKeys)

			if v["machine"] != id || v["hostname"] != "cp-1" {
				t.Errorf("machine %v hostname %v, want %s and cp-1", v["machine"], v["hostname"], id)
			}
			if s, _ := v["observed_at"].(string); s == "" {
				t.Errorf("observed_at = %v, want an RFC 3339 timestamp", v["observed_at"])
			} else if _, err := time.Parse(time.RFC3339, s); err != nil {
				t.Errorf("observed_at %q is not RFC 3339: %v", s, err)
			}
			if w, _ := v["rates_over_seconds"].(float64); w <= 0 {
				t.Errorf("rates_over_seconds = %v, want the window the rates were computed over", v["rates_over_seconds"])
			}

			cpu := requireKeys(t, "cpu", v["cpu"], hardwareCPUKeys)
			if perCore := requireArray(t, "cpu.per_core", cpu["per_core"]); len(perCore) != talossim.SimulatedThreadsPerCPU {
				t.Errorf("cpu.per_core has %d entries, want %d", len(perCore), talossim.SimulatedThreadsPerCPU)
			}
			requireKeys(t, "memory", v["memory"], hardwareMemoryKeys)

			for _, fs := range requireArray(t, "filesystems", v["filesystems"]) {
				requireKeys(t, "a filesystem", fs, hardwareFilesystemKeys)
			}
			disks := requireArray(t, "disks", v["disks"])
			if len(disks) == 0 {
				t.Fatal("disks is empty; the simulated node has one")
			}
			for _, raw := range disks {
				d := requireKeys(t, "a disk", raw, hardwareDiskKeys)
				_, isNumber := d["temperature_c"].(float64)
				if want := tc.sensors == talossim.SensorsDesktop; isNumber != want {
					t.Errorf("disk %v temperature_c = %v; want a number on a board with an NVMe sensor "+
						"and null on one without", d["name"], d["temperature_c"])
				}
			}
			for _, l := range requireArray(t, "network", v["network"]) {
				requireKeys(t, "a link", l, hardwareLinkKeys)
			}

			temps := requireArray(t, "temperatures", v["temperatures"])
			fans := requireArray(t, "fans", v["fans"])
			notice, _ := v["sensors_notice"].(string)

			switch tc.sensors {
			case talossim.SensorsNone:
				if len(temps) != 0 || len(fans) != 0 {
					t.Errorf("a node with no sensors lists %d temperatures and %d fans", len(temps), len(fans))
				}
				if notice == "" {
					t.Error("sensors_notice is empty on a node with no sensors")
				}
			default:
				if len(temps) == 0 || len(fans) == 0 {
					t.Fatalf("temperatures %d, fans %d; the desktop board has both", len(temps), len(fans))
				}
				kinds := map[string]bool{"cpu": true, "board": true, "disk": true, "gpu": true, "other": true}
				sawNullLimit := false
				for _, raw := range temps {
					tt := requireKeys(t, "a temperature", raw, hardwareTemperatureKeys)
					if kind, _ := tt["kind"].(string); !kinds[kind] {
						t.Errorf("temperature %v has kind %v, outside cpu|board|disk|gpu|other", tt["label"], tt["kind"])
					}
					if tt["high_c"] == nil {
						sawNullLimit = true
					}
				}
				if !sawNullLimit {
					t.Error("no temperature carries a null limit, but the board's SYSTIN sets none; " +
						"an unknown limit must be null, not zero")
				}
				for _, raw := range fans {
					requireKeys(t, "a fan", raw, hardwareFanKeys)
				}
				if notice != "" {
					t.Errorf("sensors_notice = %q on a board with temperatures and fans", notice)
				}
			}
		})
	}
}

// TestHardwareRouteReportsAMissingMachineAndADeadNode pins the two failures a
// panel has to tell apart: a machine that does not exist is a 404, and a node
// that does not answer is the upstream problem every other live node read
// reports -- not a 500, and not the last numbers the panel saw.
func TestHardwareRouteReportsAMissingMachineAndADeadNode(t *testing.T) {
	c := newInventoryHarness(t)
	c.adopt(t)
	id := firstMachineID(t, c)

	resp, raw := c.do(t, http.MethodGet, "/api/v1/machines/00000000-0000-0000-0000-00000000dead/hardware", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("an unknown machine: %d (%s), want 404", resp.StatusCode, raw)
	}
	if p := decodeProblem(t, resp, raw); p.Code != "notfound.record" {
		t.Errorf("an unknown machine: code %q, want notfound.record", p.Code)
	}

	if err := c.sim.Close(); err != nil {
		t.Fatalf("close the node: %v", err)
	}

	resp, raw = c.do(t, http.MethodGet, "/api/v1/machines/"+id+"/hardware", nil)
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("a node that does not answer: %d (%s), want 502", resp.StatusCode, raw)
	}
	if p := decodeProblem(t, resp, raw); p.Code != httpapi.CodeUpstreamNodeUnreachable {
		t.Errorf("a node that does not answer: code %q, want %q", p.Code, httpapi.CodeUpstreamNodeUnreachable)
	}
}
