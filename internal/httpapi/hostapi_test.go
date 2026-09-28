package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"testing/fstest"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/host"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// The host page's route at the HTTP boundary: who may read it, that reading it
// leaves nothing in the archive, and what an instance without a host reader
// answers. What the readings are is internal/host's business; this is about the
// door.

// hostSys is a host.Sys that answers from its fields. The package's own fake is
// unexported, and this one only has to say one thing.
type hostSys struct{ uname host.Uname }

func (s hostSys) Uname() (host.Uname, error)        { return s.uname, nil }
func (hostSys) BootTime() (time.Duration, error)    { return 26090 * time.Second, nil }
func (hostSys) Loads() (host.Loads, error)          { return host.Loads{}, nil }
func (hostSys) Statfs(string) (host.FSStats, error) { return host.FSStats{}, nil }

func TestHostAPI(t *testing.T) {
	t.Parallel()

	collector := host.New(host.Config{
		FS:  fstest.MapFS{},
		Sys: hostSys{uname: host.Uname{Nodename: "example-host", Release: "6.18.50+rpt-rpi-2712", Machine: "aarch64"}},
	})

	t.Run("no session", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t, withHost(collector))
		resp, raw := h.do(t, http.MethodGet, "/api/v1/host", nil)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("GET /api/v1/host without a session: %d, want 401 (%s)", resp.StatusCode, raw)
		}
	})

	t.Run("admin and reader read it, and nothing is audited", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t, withHost(collector))
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

		resp, raw := h.do(t, http.MethodGet, "/api/v1/host", nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("admin: %d, want 200 (%s)", resp.StatusCode, raw)
		}
		var body struct {
			ObservedAt string `json:"observed_at"`
			Device     struct {
				Hostname struct {
					Readable bool   `json:"readable"`
					Value    string `json:"value"`
				} `json:"hostname"`
			} `json:"device"`
			Health struct {
				State      string          `json:"state"`
				Summary    string          `json:"summary"`
				Warnings   json.RawMessage `json:"warnings"`
				Unreadable json.RawMessage `json:"unreadable"`
			} `json:"health"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("decode: %v (%s)", err, raw)
		}
		if !body.Device.Hostname.Readable || body.Device.Hostname.Value != "example-host" {
			t.Errorf("device.hostname = %+v, want readable example-host (%s)", body.Device.Hostname, raw)
		}
		if _, err := time.Parse(time.RFC3339Nano, body.ObservedAt); err != nil {
			t.Errorf("observed_at %q is not a timestamp: %v", body.ObservedAt, err)
		}
		// The server's one decision about the host (D-06), with lists that
		// are lists even when empty.
		switch host.HealthState(body.Health.State) {
		case host.HealthOK, host.HealthWarn, host.HealthUnknown:
		default:
			t.Errorf("health.state = %q, want ok, warn or unknown (%s)", body.Health.State, raw)
		}
		if body.Health.Summary == "" {
			t.Errorf("health.summary is empty (%s)", raw)
		}
		for name, list := range map[string]json.RawMessage{"warnings": body.Health.Warnings, "unreadable": body.Health.Unreadable} {
			var items []string
			if len(list) == 0 || list[0] != '[' || json.Unmarshal(list, &items) != nil {
				t.Errorf("health.%s = %s, want an array", name, list)
			}
		}

		if got, raw := reader.status(t, http.MethodGet, "/api/v1/host", nil); got != http.StatusOK {
			t.Errorf("reader: %d, want 200 (%s)", got, raw)
		}

		if after := len(h.auditPage(t, "").Items); after != before {
			t.Errorf("reading the host wrote to the archive: %d records before, %d after", before, after)
		}
	})

	t.Run("an instance without a host reader", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)
		h.setupAndLogin(t)
		resp, raw := h.do(t, http.MethodGet, "/api/v1/host", nil)
		if resp.StatusCode != http.StatusBadGateway {
			t.Fatalf("no collector: %d, want 502 (%s)", resp.StatusCode, raw)
		}
		if p := decodeProblem(t, resp, raw); p.Code != "upstream.host-unavailable" {
			t.Errorf("code = %q, want upstream.host-unavailable", p.Code)
		}
	})
}
