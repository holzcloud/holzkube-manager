package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/history"
	"github.com/holzcloud/holzkube-manager/internal/host"
	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// The history routes' contract, as the charts read it: the exact key set,
// series that are an object and never null, points that are pairs of numbers,
// and the range choosing the step. The machine's series are filled by the real
// sampler over the real inventory against a simulated node, so the keys checked
// here are the ones a node's hardware read actually produces.

var historyKeys = []string{"range", "step_seconds", "from", "to", "series"}

func getHistory(t *testing.T, c *harness, path string) (int, map[string]any, []byte) {
	t.Helper()
	resp, raw := c.do(t, http.MethodGet, path, nil)
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, nil, raw
	}
	var body any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode %s: %v (%s)", path, err, raw)
	}
	return resp.StatusCode, requireKeys(t, path, body, historyKeys), raw
}

// sampleOnce runs the real sampler over the harness's inventory until the
// machine has a first sample, then stops it.
func sampleOnce(t *testing.T, c *inventoryHarness, machine string) {
	t.Helper()

	s := history.NewSampler(history.SamplerDeps{
		History: c.history,
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Inventory: func(ctx context.Context) ([]model.Machine, []model.Cluster, error) {
			machines, err := c.store.Machines().List(ctx)
			if err != nil {
				return nil, nil, err
			}
			clusters, err := c.store.Clusters().List(ctx)
			return machines, clusters, err
		},
		Hardware: c.inv.Hardware,
		// The simulated node has no Kubernetes API; the apps half is held by
		// internal/history's own tests.
		Apps: func(context.Context, model.ClusterID) (kube.Apps, error) {
			return kube.Apps{}, errors.New("no Kubernetes API in this harness")
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Run(ctx)
	}()
	defer func() {
		cancel()
		<-done
	}()

	deadline := time.Now().Add(15 * time.Second)
	for !c.history.Has(history.MachineSubject(model.MachineID(machine))) {
		if time.Now().After(deadline) {
			t.Fatal("the sampler recorded nothing for the adopted machine")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestMachineHistoryAnswersTheContract walks the machine route through the
// three ranges, a refused one, and an unknown machine.
//
// Fault injected and seen red: machineHistory ignoring the parameter and always
// answering Range1h -- 6h and 24h came back with step 15 and range "1h".
func TestMachineHistoryAnswersTheContract(t *testing.T) {
	c := newInventoryHarnessWith(t, talossim.Options{Sensors: talossim.SensorsDesktop}, withHistory())
	c.adopt(t)
	id := firstMachineID(t, c)
	base := "/api/v1/machines/" + id + "/hardware/history"

	// Before anything was sampled: 200, and an object, never null.
	status, v, raw := getHistory(t, c.harness, base)
	if status != http.StatusOK {
		t.Fatalf("history before a sample: %d (%s)", status, raw)
	}
	if !strings.Contains(string(raw), `"series":{}`) {
		t.Errorf("a machine with no history answers %s; want \"series\":{}", raw)
	}
	if v["range"] != "1h" || v["step_seconds"] != float64(15) {
		t.Errorf("the default is range %v step %v, want 1h and 15", v["range"], v["step_seconds"])
	}

	sampleOnce(t, c, id)

	for _, tc := range []struct {
		query, rng string
		step       float64
	}{
		{"", "1h", 15}, {"?range=1h", "1h", 15}, {"?range=6h", "6h", 60}, {"?range=24h", "24h", 60},
	} {
		status, v, raw := getHistory(t, c.harness, base+tc.query)
		if status != http.StatusOK {
			t.Fatalf("%s: %d (%s)", tc.query, status, raw)
		}
		if v["range"] != tc.rng || v["step_seconds"] != tc.step {
			t.Errorf("%q: range %v step %v, want %s and %v", tc.query, v["range"], v["step_seconds"], tc.rng, tc.step)
		}
		for _, k := range []string{"from", "to"} {
			if s, _ := v[k].(string); s == "" {
				t.Errorf("%q: %s = %v, want RFC 3339", tc.query, k, v[k])
			} else if _, err := time.Parse(time.RFC3339, s); err != nil {
				t.Errorf("%q: %s %q is not RFC 3339: %v", tc.query, k, s, err)
			}
		}

		series, ok := v["series"].(map[string]any)
		if !ok {
			t.Fatalf("%q: series is %T, want an object", tc.query, v["series"])
		}
		for _, want := range []string{"cpu", "memory", "rx", "tx", "read", "write", "core:0"} {
			if _, ok := series[want]; !ok {
				t.Errorf("%q: no %q series among %d", tc.query, want, len(series))
			}
		}
		temps := 0
		for name, pts := range series {
			if strings.HasPrefix(name, "temp:") {
				temps++
			}
			for _, p := range requireArray(t, name, pts) {
				pair, ok := p.([]any)
				if !ok || len(pair) != 2 {
					t.Fatalf("%q: %s has point %v, want [ms, value]", tc.query, name, p)
				}
				if _, ok := pair[0].(float64); !ok {
					t.Errorf("%q: %s timestamp %v is not a number", tc.query, name, pair[0])
				}
				if _, ok := pair[1].(float64); !ok {
					t.Errorf("%q: %s value %v is not a number", tc.query, name, pair[1])
				}
			}
		}
		if temps == 0 {
			t.Errorf("%q: a desktop board's temperatures are not in the history", tc.query)
		}
	}

	resp, raw := c.do(t, http.MethodGet, base+"?range=2h", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("range=2h: %d (%s), want 400", resp.StatusCode, raw)
	}
	resp, raw = c.do(t, http.MethodGet, "/api/v1/machines/00000000-0000-0000-0000-000000000000/hardware/history", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("an unknown machine: %d (%s), want 404", resp.StatusCode, raw)
	}
}

// TestAppHistoryKnowsWhichAppsExist: 404 for a cluster the inventory does not
// hold and for an app the last listing did not name; 200 with an empty object
// for an app it named and nobody measured, and for any app before the cluster
// was ever listed -- nobody has asked yet, and "not found" would be a claim.
//
// Fault injected and seen red: appHistory ignoring the store's answer (no 404
// for an unlisted app).
func TestAppHistoryKnowsWhichAppsExist(t *testing.T) {
	c := newInventoryHarnessWith(t, talossim.Options{}, withHistory())
	cluster, _ := c.adopt(t)["id"].(string)
	if cluster == "" {
		t.Fatal("the adoption returned no cluster id")
	}
	app := func(ns, kind, name string) string {
		return "/api/v1/clusters/" + cluster + "/kubernetes/apps/" + ns + "/" + kind + "/" + name + "/history"
	}

	if status, _, raw := getHistory(t, c.harness, app("default", "Deployment", "web")); status != http.StatusOK ||
		!strings.Contains(string(raw), `"series":{}`) {
		t.Errorf("before any listing: %d (%s), want 200 and an empty object", status, raw)
	}

	c.history.SetApps(model.ClusterID(cluster), []string{
		history.AppSubject(model.ClusterID(cluster), "default", "Deployment", "web"),
	})
	c.history.Record(history.AppSubject(model.ClusterID(cluster), "default", "Deployment", "web"),
		time.Now(), map[string]float64{"cpu": 120, "memory": 1 << 26})

	// The kind is matched as the detail route matches it, without case.
	status, v, raw := getHistory(t, c.harness, app("default", "deployment", "web")+"?range=24h")
	if status != http.StatusOK {
		t.Fatalf("a listed app: %d (%s)", status, raw)
	}
	series, _ := v["series"].(map[string]any)
	if len(requireArray(t, "cpu", series["cpu"])) != 1 || len(requireArray(t, "memory", series["memory"])) != 1 {
		t.Errorf("a measured app's day is %s, want one cpu and one memory point", raw)
	}

	for name, path := range map[string]string{
		"an app the listing did not name":       app("default", "Deployment", "gone"),
		"a kind that is not a kind of app":      app("default", "ConfigMap", "web"),
		"a cluster the inventory does not hold": "/api/v1/clusters/nope/kubernetes/apps/default/Deployment/web/history",
	} {
		resp, raw := c.do(t, http.MethodGet, path, nil)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s: %d (%s), want 404", name, resp.StatusCode, raw)
		}
	}
}

// hostSensorsFS is one hwmon chip of the host, the way the Pi's CPU sensor
// sits in /sys: a driver name and one temperature, in millidegrees.
func hostSensorsFS() fstest.MapFS {
	return fstest.MapFS{
		"sys/class/hwmon/hwmon0/name":        {Data: []byte("cpu_thermal\n")},
		"sys/class/hwmon/hwmon0/temp1_input": {Data: []byte("64400\n")},
	}
}

// sampleHostOnce runs the real sampler, with no machines on record, until the
// host has a first sample, then stops it.
func sampleHostOnce(t *testing.T, store *history.Store, collector *host.Collector) {
	t.Helper()

	s := history.NewSampler(history.SamplerDeps{
		History: store,
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Inventory: func(context.Context) ([]model.Machine, []model.Cluster, error) {
			return nil, nil, nil
		},
		Hardware: func(context.Context, model.MachineID) (inventory.HardwareView, error) {
			return inventory.HardwareView{}, errors.New("no machines in this harness")
		},
		Apps: func(context.Context, model.ClusterID) (kube.Apps, error) {
			return kube.Apps{}, errors.New("no Kubernetes API in this harness")
		},
		Host: collector.Sample,
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Run(ctx)
	}()
	defer func() {
		cancel()
		<-done
	}()

	deadline := time.Now().Add(15 * time.Second)
	for !store.Has(history.HostSubject()) {
		if time.Now().After(deadline) {
			t.Fatal("the sampler recorded nothing for the host")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestHostHistoryAPI is the host's history end to end: the real collector
// reading a sensor out of /sys, the real sampler filing it under host/local,
// the real store, and the route answering it with the node's contract -- and
// the node route's refusals: 401 without a session, 400 for a range that is
// not one of the three, 502 without a store. A reader may read it, and reading
// it writes nothing to the archive.
//
// Fault injected and seen red: hostHistory without historyRange, always
// answering Range1h -- ?range=7d came back 200.
func TestHostHistoryAPI(t *testing.T) {
	t.Parallel()

	collector := host.New(host.Config{
		FS:  hostSensorsFS(),
		Sys: hostSys{uname: host.Uname{Nodename: "example-host", Release: "6.18.50+rpt-rpi-2712", Machine: "aarch64"}},
	})

	t.Run("a sensor read on the host is served as its series", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t, withHistory(), withHost(collector))
		h.setupAndLogin(t)
		sampleHostOnce(t, h.history, collector)

		status, v, raw := getHistory(t, h, "/api/v1/host/history?range=1h")
		if status != http.StatusOK {
			t.Fatalf("GET /api/v1/host/history: %d (%s)", status, raw)
		}
		if v["range"] != "1h" || v["step_seconds"] != float64(15) {
			t.Errorf("range %v step %v, want 1h and 15", v["range"], v["step_seconds"])
		}
		series, ok := v["series"].(map[string]any)
		if !ok {
			t.Fatalf("series is %T, want an object (%s)", v["series"], raw)
		}
		points := requireArray(t, "temp:cpu_thermal/temp1", series["temp:cpu_thermal/temp1"])
		if len(points) == 0 {
			t.Fatalf("no temp:cpu_thermal/temp1 point among %d series (%s)", len(series), raw)
		}
		found := false
		for _, p := range points {
			if pair, ok := p.([]any); ok && len(pair) == 2 && pair[1] == 64.4 {
				found = true
			}
		}
		if !found {
			t.Errorf("temp:cpu_thermal/temp1 = %v, want a point at 64.4", points)
		}
	})

	t.Run("no session", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t, withHistory(), withHost(collector))
		resp, raw := h.do(t, http.MethodGet, "/api/v1/host/history?range=1h", nil)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("GET /api/v1/host/history without a session: %d, want 401 (%s)", resp.StatusCode, raw)
		}
	})

	t.Run("a reader reads it before any sample, and nothing is audited", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t, withHistory(), withHost(collector))
		h.setupAndLogin(t)
		if resp, raw := h.do(t, http.MethodPost, "/api/v1/auth/sudo",
			map[string]string{"password": testPass}); resp.StatusCode != http.StatusNoContent {
			t.Fatalf("sudo: %d (%s)", resp.StatusCode, raw)
		}
		if resp, raw := h.do(t, http.MethodPost, "/api/v1/users", map[string]string{
			"username": "reader-account", "password": newAccountPass, "role": string(model.RoleReader),
		}); resp.StatusCode != http.StatusCreated {
			t.Fatalf("creating the reader: %d (%s)", resp.StatusCode, raw)
		}
		reader := h.asUser(t, "reader-account", newAccountPass)

		before := len(h.auditPage(t, "").Items)

		status, raw := reader.status(t, http.MethodGet, "/api/v1/host/history?range=6h", nil)
		if status != http.StatusOK {
			t.Fatalf("reader: %d, want 200 (%s)", status, raw)
		}
		var body any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("decode: %v (%s)", err, raw)
		}
		if v := requireKeys(t, "/api/v1/host/history", body, historyKeys); v["range"] != "6h" {
			t.Errorf("range = %v, want 6h", v["range"])
		}
		if !strings.Contains(string(raw), `"series":{}`) {
			t.Errorf("a host never sampled answers %s; want \"series\":{}", raw)
		}

		if after := len(h.auditPage(t, "").Items); after != before {
			t.Errorf("reading the host's history wrote to the archive: %d records before, %d after", before, after)
		}
	})

	t.Run("a range that is not one of the three", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t, withHistory(), withHost(collector))
		h.setupAndLogin(t)
		resp, raw := h.do(t, http.MethodGet, "/api/v1/host/history?range=7d", nil)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("range=7d: %d, want 400 (%s)", resp.StatusCode, raw)
		}
		p := decodeProblem(t, resp, raw)
		if p.Type != httpapi.TypeValidation {
			t.Errorf("type = %q, want %s", p.Type, httpapi.TypeValidation)
		}
		if len(p.Errors) != 1 || p.Errors[0].Field != "range" {
			t.Errorf("errors = %+v, want one naming the field range", p.Errors)
		}
	})

	t.Run("an instance without a history", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t, withHost(collector))
		h.setupAndLogin(t)
		resp, raw := h.do(t, http.MethodGet, "/api/v1/host/history?range=1h", nil)
		if resp.StatusCode != http.StatusBadGateway {
			t.Fatalf("no store: %d, want 502 (%s)", resp.StatusCode, raw)
		}
		if p := decodeProblem(t, resp, raw); p.Code != "upstream.history-unavailable" {
			t.Errorf("code = %q, want upstream.history-unavailable", p.Code)
		}
	})
}

// TestAMachineHistoryWithoutAStore is the node route's answer when the instance
// keeps no history: the same 502 the host route gives, so the page says the
// same thing for both.
func TestAMachineHistoryWithoutAStore(t *testing.T) {
	c := newInventoryHarnessWith(t, talossim.Options{})
	resp, raw := c.do(t, http.MethodGet,
		"/api/v1/machines/00000000-0000-0000-0000-000000000000/hardware/history?range=1h", nil)
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("no store: %d, want 502 (%s)", resp.StatusCode, raw)
	}
	if p := decodeProblem(t, resp, raw); p.Code != "upstream.history-unavailable" {
		t.Errorf("code = %q, want upstream.history-unavailable", p.Code)
	}
}
