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
	"time"

	"github.com/holzcloud/holzkube-manager/internal/history"
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

func getHistory(t *testing.T, c *inventoryHarness, path string) (int, map[string]any, []byte) {
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
	status, v, raw := getHistory(t, c, base)
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
		status, v, raw := getHistory(t, c, base+tc.query)
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

	if status, _, raw := getHistory(t, c, app("default", "Deployment", "web")); status != http.StatusOK ||
		!strings.Contains(string(raw), `"series":{}`) {
		t.Errorf("before any listing: %d (%s), want 200 and an empty object", status, raw)
	}

	c.history.SetApps(model.ClusterID(cluster), []string{
		history.AppSubject(model.ClusterID(cluster), "default", "Deployment", "web"),
	})
	c.history.Record(history.AppSubject(model.ClusterID(cluster), "default", "Deployment", "web"),
		time.Now(), map[string]float64{"cpu": 120, "memory": 1 << 26})

	// The kind is matched as the detail route matches it, without case.
	status, v, raw := getHistory(t, c, app("default", "deployment", "web")+"?range=24h")
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
