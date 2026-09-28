package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"testing/fstest"

	corev1 "k8s.io/api/core/v1"

	"github.com/holzcloud/holzkube-manager/internal/host"
	"github.com/holzcloud/holzkube-manager/internal/kubesim"
)

// The host on the wall, end to end (HOST-04, D-14): the wall's one answer
// carries the host's tile from the sampler's last snapshot, the same to a
// signed-in reader and to a wall link, and leaves the cluster's nodes as the
// cluster answered them.

// wallHostSys is a host whose root filesystem is a quarter full.
type wallHostSys struct{ hostSys }

func (wallHostSys) Statfs(string) (host.FSStats, error) {
	return host.FSStats{FrameSize: 4096, Blocks: 1_000_000, Free: 760_000, Avail: 750_000}, nil
}

// wallAnswerHost is the part of the wall's answer this file is about.
type wallAnswerHost struct {
	Nodes []struct {
		Name string `json:"name"`
	} `json:"nodes"`
	Summary map[string]int  `json:"summary"`
	Host    json.RawMessage `json:"host"`
}

func TestTheWallCarriesTheHost(t *testing.T) {
	t.Parallel()

	nodes := []kubesim.Node{
		{Name: "cp-1", Ready: corev1.ConditionTrue, Roles: []string{"control-plane"}},
		{Name: "worker-1", Ready: corev1.ConditionTrue},
	}

	t.Run("with a host reader", func(t *testing.T) {
		t.Parallel()

		collector := host.New(host.Config{
			FS: fstest.MapFS{
				"sys/class/hwmon/hwmon0/name":        {Data: []byte("cpu_thermal\n")},
				"sys/class/hwmon/hwmon0/temp1_input": {Data: []byte("64400\n")},
			},
			Sys: wallHostSys{hostSys{uname: host.Uname{Nodename: "example-host", Release: "6.18.50+rpt-rpi-2712", Machine: "aarch64"}}},
		})
		if _, err := collector.Sample(context.Background()); err != nil {
			t.Fatalf("Sample: %v", err)
		}

		h, id, _ := adoptedClusterWithAPI(t, kubesim.Options{Nodes: nodes}, withHost(collector))
		path := "/api/v1/clusters/" + id + "/wall"

		resp, raw := h.do(t, http.MethodGet, path, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: %d (%s)", path, resp.StatusCode, raw)
		}
		var wall wallAnswerHost
		if err := json.Unmarshal(raw, &wall); err != nil {
			t.Fatalf("decode: %v (%s)", err, raw)
		}
		var tile struct {
			Name   string `json:"name"`
			State  string `json:"state"`
			Reason string `json:"reason"`
		}
		if err := json.Unmarshal(wall.Host, &tile); err != nil {
			t.Fatalf("host = %s: %v", wall.Host, err)
		}
		if tile.Name != "example-host" {
			t.Errorf("host.name = %q, want example-host", tile.Name)
		}
		switch host.HealthState(tile.State) {
		case host.HealthOK, host.HealthWarn, host.HealthUnknown:
		default:
			t.Errorf("host.state = %q, want ok, warn or unknown", tile.State)
		}
		// 64.4 °C under the Pi's 80, / at 25 %: the sampler saw a healthy host.
		if tile.State != string(host.HealthOK) || tile.Reason != "healthy" {
			t.Errorf("host = %s, want ok and healthy", wall.Host)
		}
		if len(wall.Nodes) != len(nodes) {
			t.Errorf("%d nodes on the wall, want the cluster's %d: the host is not one of them (%s)", len(wall.Nodes), len(nodes), raw)
		}
		for _, n := range wall.Nodes {
			if n.Name == "example-host" {
				t.Errorf("the host is among the nodes (%s)", raw)
			}
		}

		// A wall link sees the same host.
		token := mintWallLink(t, h)
		status, screenRaw := asTheScreen(t, h, http.MethodGet, path, token, nil)
		if status != http.StatusOK {
			t.Fatalf("the wall link: %d (%s)", status, screenRaw)
		}
		var screen wallAnswerHost
		if err := json.Unmarshal([]byte(screenRaw), &screen); err != nil {
			t.Fatalf("decode: %v (%s)", err, screenRaw)
		}
		if string(screen.Host) != string(wall.Host) {
			t.Errorf("the wall link's host = %s, the session's %s; want the same", screen.Host, wall.Host)
		}
	})

	t.Run("without a host reader", func(t *testing.T) {
		t.Parallel()

		h, id, _ := adoptedClusterWithAPI(t, kubesim.Options{Nodes: nodes})
		resp, raw := h.do(t, http.MethodGet, "/api/v1/clusters/"+id+"/wall", nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("wall: %d (%s)", resp.StatusCode, raw)
		}
		var wall wallAnswerHost
		if err := json.Unmarshal(raw, &wall); err != nil {
			t.Fatalf("decode: %v (%s)", err, raw)
		}
		if string(wall.Host) != "null" {
			t.Errorf("host = %s, want null: no reader, no tile", wall.Host)
		}
	})
}

// A snapshot is read, not taken: the wall is answered from what the sampler
// saw, so a collector that never sampled has no tile even though it could.
func TestTheWallDoesNotReadTheHost(t *testing.T) {
	t.Parallel()

	collector := host.New(host.Config{
		FS:  fstest.MapFS{},
		Sys: wallHostSys{hostSys{uname: host.Uname{Nodename: "example-host"}}},
	})
	h, id, _ := adoptedClusterWithAPI(t, kubesim.Options{}, withHost(collector))
	resp, raw := h.do(t, http.MethodGet, "/api/v1/clusters/"+id+"/wall", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("wall: %d (%s)", resp.StatusCode, raw)
	}
	var wall wallAnswerHost
	if err := json.Unmarshal(raw, &wall); err != nil {
		t.Fatalf("decode: %v (%s)", err, raw)
	}
	if string(wall.Host) != "null" {
		t.Errorf("host = %s before any sample, want null", wall.Host)
	}
	if _, sampled := collector.Latest(); sampled {
		t.Error("answering the wall took a sample of the host")
	}
}
