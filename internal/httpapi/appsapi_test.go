package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/kubesim"
)

// The apps routes (2026-09-26).
//
// What only a route test can see: that the filters in the query reach the
// client, that an app that is not there is a 404 rather than a 502 about an
// unreachable cluster, and that the wire shape is the one the screen was
// written against -- arrays as arrays, the keys named as the contract names
// them.

func appsAPICluster(t *testing.T) (*harness, string, *kubesim.Server) {
	t.Helper()

	web := func(name, node string) kubesim.Pod {
		return kubesim.Pod{
			Namespace: "default", Name: name, Node: node,
			OwnerKind: "ReplicaSet", OwnerName: "web-7d9c",
			ContainerNames: []string{"web"}, Ready: 1, Image: "nginx:1.27",
			Labels: map[string]string{"app": "web"},
		}
	}
	return adoptedClusterWithAPI(t, kubesim.Options{
		Nodes:       []kubesim.Node{{Name: "cp-1"}, {Name: "w-1"}},
		Deployments: []kubesim.Deployment{{Namespace: "default", Name: "web", Desired: 2, Ready: 2}},
		ReplicaSets: []kubesim.ReplicaSet{{Namespace: "default", Name: "web-7d9c", Owner: "web", Replicas: 2}},
		Pods:        []kubesim.Pod{web("web-7d9c-a", "cp-1"), web("web-7d9c-b", "w-1")},
		Summary: map[string]map[string]string{
			"default/web-7d9c-a": {"web": "300m,100Mi"},
			"default/web-7d9c-b": {"web": "20m,50Mi"},
		},
		Services: []kubesim.Service{{Namespace: "default", Name: "web",
			Selector: map[string]string{"app": "web"}, Ports: []kubesim.ServicePort{{Port: 80}}}},
	})
}

// TestTheAppsRouteAnswersTheContract.
func TestTheAppsRouteAnswersTheContract(t *testing.T) {
	h, id, api := appsAPICluster(t)
	base := "/api/v1/clusters/" + id + "/kubernetes/apps"

	resp, raw := h.do(t, http.MethodGet, base, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("apps: %d (%s)", resp.StatusCode, raw)
	}
	var list struct {
		CollectedAt    string `json:"collected_at"`
		UsageAvailable *bool  `json:"usage_available"`
		Notice         *string
		Apps           []struct {
			Namespace   string   `json:"namespace"`
			Kind        string   `json:"kind"`
			Name        string   `json:"name"`
			Pods        int      `json:"pods"`
			Nodes       []string `json:"nodes"`
			CPUMillis   int64    `json:"cpu_millis"`
			MemoryBytes int64    `json:"memory_bytes"`
			UsageKnown  bool     `json:"usage_known"`
			Images      []string `json:"images"`
		} `json:"apps"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("decode: %v (%s)", err, raw)
	}
	if list.CollectedAt == "" || list.UsageAvailable == nil || !*list.UsageAvailable ||
		list.Notice == nil {
		t.Errorf("the envelope = %s", raw)
	}
	if len(list.Apps) != 1 {
		t.Fatalf("apps = %s", raw)
	}
	web := list.Apps[0]
	if web.Kind != "Deployment" || web.Name != "web" || web.Pods != 2 || web.CPUMillis != 320 ||
		web.MemoryBytes != 150*1024*1024 || !web.UsageKnown || len(web.Nodes) != 2 {
		t.Errorf("web = %+v", web)
	}

	// The node filter is the query's, and it reaches the client: one pod, one
	// kubelet asked.
	before := api.Calls("GET /api/v1/nodes/cp-1/proxy/stats/summary")
	resp, raw = h.do(t, http.MethodGet, base+"?node=w-1", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("apps on w-1: %d (%s)", resp.StatusCode, raw)
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list.Apps) != 1 || list.Apps[0].Pods != 1 || list.Apps[0].CPUMillis != 20 {
		t.Errorf("on w-1 = %s, want web's one pod there at 20m", raw)
	}
	if got := api.Calls("GET /api/v1/nodes/cp-1/proxy/stats/summary"); got != before {
		t.Errorf("cp-1's kubelet was asked for a page about w-1")
	}

	// A namespace with nothing in it is an empty list, not null.
	resp, raw = h.do(t, http.MethodGet, base+"?namespace=empty", nil)
	if resp.StatusCode != http.StatusOK || !bytes.Contains(raw, []byte(`"apps":[]`)) {
		t.Errorf("an empty namespace: %d (%s), want \"apps\":[]", resp.StatusCode, raw)
	}
}

// TestTheAppDetailRouteFindsTheAppOrSaysItIsNotThere.
func TestTheAppDetailRouteFindsTheAppOrSaysItIsNotThere(t *testing.T) {
	h, id, _ := appsAPICluster(t)
	base := "/api/v1/clusters/" + id + "/kubernetes/apps/"

	resp, raw := h.do(t, http.MethodGet, base+"default/Deployment/web", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("detail: %d (%s)", resp.StatusCode, raw)
	}
	var detail struct {
		App struct {
			Name string `json:"name"`
			Pods int    `json:"pods"`
		} `json:"app"`
		CreatedAt string `json:"created_at"`
		Pods      []struct {
			Name       string            `json:"name"`
			Ready      string            `json:"ready"`
			Containers []json.RawMessage `json:"containers"`
		} `json:"pods"`
		Services []struct {
			Name  string   `json:"name"`
			Ports []string `json:"ports"`
		} `json:"services"`
		Events []json.RawMessage `json:"events"`
	}
	if err := json.Unmarshal(raw, &detail); err != nil {
		t.Fatalf("decode: %v (%s)", err, raw)
	}
	if detail.App.Name != "web" || detail.App.Pods != 2 || len(detail.Pods) != 2 ||
		detail.Pods[0].Ready != "1/1" || len(detail.Pods[0].Containers) != 1 {
		t.Errorf("detail = %s", raw)
	}
	if len(detail.Services) != 1 || detail.Services[0].Name != "web" {
		t.Errorf("services = %+v", detail.Services)
	}
	if detail.Events == nil || !bytes.Contains(raw, []byte(`"events":[]`)) {
		t.Errorf("no events is an empty list, not null or missing: %s", raw)
	}

	// Not there: a 404 with the code every named-object route uses, and not a
	// 502 -- the cluster answered perfectly well.
	for _, path := range []string{"default/Deployment/ghost", "default/Frobnicator/web"} {
		resp, raw = h.do(t, http.MethodGet, base+path, nil)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s: %d (%s), want 404", path, resp.StatusCode, raw)
			continue
		}
		var problem struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(raw, &problem); err != nil || problem.Code != "notfound.kubernetes-workload" {
			t.Errorf("%s: problem = %s", path, raw)
		}
	}
}
