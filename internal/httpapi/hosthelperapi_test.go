package httpapi_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"testing/fstest"

	"github.com/holzcloud/holzkube-manager/internal/auth"
	"github.com/holzcloud/holzkube-manager/internal/host"
	"github.com/holzcloud/holzkube-manager/internal/host/hostaction"
	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/store"
	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
)

// A host action with nothing to carry it out is refused before anything is
// written (Phase 13 D-12, D-14, HACT-07): with the helper missing, or with the
// daemon in a container, both the confirm route and every action route answer
// 409, and the data directory holds no order and no half-written one
// afterwards. The action route is handed a valid host token from the
// harness's own confirmer, so that the refusal is the helper's and not the
// token's.

// helperState is the installed fixture changed by edit (nil: installed).
func helperState(t *testing.T, edit func(m fstest.MapFS)) fs.FS {
	t.Helper()
	fsys, _ := installedHelperFS(t)
	o, ok := fsys.(overlayFS)
	if !ok {
		t.Fatalf("installedHelperFS returned %T, not an overlayFS", fsys)
	}
	if edit != nil {
		edit(o.fixed)
	}
	return o
}

// hostHelperHarness serves the host routes over a Box reading boxFS and a host
// reader reading collectorFS, both over the same Box, as the composition root
// wires them; signed in, with the sudo window open.
func hostHelperHarness(t *testing.T, boxFS fs.FS, collectorFS fs.FS) *harness {
	t.Helper()
	sys := hostSys{uname: host.Uname{Nodename: "example-host", Release: "6.18.50+rpt-rpi-2712", Machine: "aarch64"}}
	h := newHarness(t,
		withHostActions(func(dataDir string) *hostaction.Box {
			return hostaction.NewBox(hostaction.Config{FS: boxFS, DataDir: dataDir, Place: fsstore.PlaceNew})
		}),
		withHostOver(func(box *hostaction.Box) *host.Collector {
			return host.New(host.Config{FS: collectorFS, Sys: sys, Actions: box})
		}),
	)
	h.setupAndLogin(t)
	if resp, raw := h.do(t, http.MethodPost, "/api/v1/auth/sudo",
		map[string]string{"password": testPass}); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("sudo: %d (%s)", resp.StatusCode, raw)
	}
	if h.confirmer == nil {
		t.Fatal("the harness brought no confirmer; the action route could not be handed a valid token")
	}
	return h
}

// hostHelperView is the part of actions this file reads.
type hostHelperView struct {
	Container bool `json:"container"`
	Actions   struct {
		Available       *bool                `json:"available"`
		Missing         []hostaction.Missing `json:"missing"`
		InstallCommands []string             `json:"install_commands"`
	} `json:"actions"`
}

// requireRefused asks the confirm route and every action route and wants 409
// code from each, with no token handed out and nothing in the data directory.
func requireRefused(t *testing.T, h *harness, code string) {
	t.Helper()

	resp, raw := h.do(t, http.MethodPost, "/api/v1/host/confirm", map[string]string{
		"action": "host.update", "typed": "example-host",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("POST /api/v1/host/confirm: %d, want 409 %s (%s)", resp.StatusCode, code, raw)
	} else if p := decodeProblem(t, resp, raw); p.Code != code {
		t.Errorf("POST /api/v1/host/confirm: code %q, want %q", p.Code, code)
	}
	if strings.Contains(string(raw), `"token"`) {
		t.Errorf("POST /api/v1/host/confirm handed out a token: %s", raw)
	}

	for _, a := range hostaction.Actions() {
		tok := sessionHostToken(t, h.confirmer, h.srv.URL, h.client, "host."+string(a))
		path := "/api/v1/host/actions/" + string(a)
		resp, raw := h.do(t, http.MethodPost, path, map[string]string{"confirmation": tok})
		if resp.StatusCode != http.StatusConflict {
			t.Errorf("POST %s with a valid host token: %d, want 409 %s (%s)", path, resp.StatusCode, code, raw)
		} else if p := decodeProblem(t, resp, raw); p.Code != code {
			t.Errorf("POST %s: code %q, want %q", path, p.Code, code)
		}
		requireNothingPlaced(t, h, path)
	}
}

// requireNothingPlaced fails when the data directory holds the order, or a
// name with the store's temporary prefix -- a placement begun and not
// finished. What it finds it names.
func requireNothingPlaced(t *testing.T, h *harness, after string) {
	t.Helper()
	entries, err := os.ReadDir(h.dataDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() == hostaction.OrderFileName || strings.HasPrefix(e.Name(), store.TempFilePrefix) {
			t.Errorf("after %s the data directory holds %s: a refused request placed an order", after, e.Name())
			if err := os.Remove(h.dataDir + "/" + e.Name()); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// readHelperView GETs /api/v1/host.
func readHelperView(t *testing.T, h *harness) hostHelperView {
	t.Helper()
	var v hostHelperView
	h.getJSON(t, "/api/v1/host", &v)
	if v.Actions.Available == nil {
		t.Fatal("actions.available is not in the answer")
	}
	if v.Actions.Missing == nil {
		t.Error("actions.missing is null or absent; the page reads a list")
	}
	if !reflect.DeepEqual(v.Actions.InstallCommands, hostaction.InstallCommands) {
		t.Errorf("actions.install_commands = %q, want hostaction.InstallCommands %q",
			v.Actions.InstallCommands, hostaction.InstallCommands)
	}
	return v
}

func TestHostActionsNeedTheHelper(t *testing.T) {
	t.Parallel()

	script := hostaction.Missing{Item: hostaction.MissingScript, Path: hostaction.HelperScriptPath}
	pathUnit := hostaction.Missing{Item: hostaction.MissingPathUnit, Path: hostaction.PathUnitPath}
	serviceUnit := hostaction.Missing{Item: hostaction.MissingPathUnit, Path: hostaction.ServiceUnitPath}
	notEnabled := hostaction.Missing{Item: hostaction.MissingNotEnabled, Path: hostaction.WantsLinkPath}

	cases := []struct {
		name string
		edit func(m fstest.MapFS)
		want []hostaction.Missing
	}{
		{"nothing installed", func(m fstest.MapFS) {
			for _, n := range []string{helperScriptName, helperPathUnit, helperServiceUnit, helperWantsLink} {
				delete(m, n)
			}
		}, []hostaction.Missing{script, pathUnit}},
		// Not enabled, but the unit files are not there either: path-unit is
		// the whole answer, and the route still refuses (G-13-2).
		{"script installed, no unit files, not enabled", func(m fstest.MapFS) {
			for _, n := range []string{helperPathUnit, helperServiceUnit, helperWantsLink} {
				delete(m, n)
			}
		}, []hostaction.Missing{pathUnit}},
		{"script owned by uid 1000", func(m fstest.MapFS) {
			m[helperScriptName] = &fstest.MapFile{Data: []byte("#!/bin/sh\n"), Mode: 0o755, Sys: &syscall.Stat_t{Uid: 1000}}
		}, []hostaction.Missing{script}},
		{"service unit absent", func(m fstest.MapFS) { delete(m, helperServiceUnit) }, []hostaction.Missing{serviceUnit}},
		{"path unit not enabled", func(m fstest.MapFS) { delete(m, helperWantsLink) }, []hostaction.Missing{notEnabled}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := hostHelperHarness(t, helperState(t, tc.edit), fstest.MapFS{})

			requireRefused(t, h, "conflict.host-helper-missing")

			v := readHelperView(t, h)
			if *v.Actions.Available {
				t.Error("actions.available = true with the helper missing")
			}
			if !reflect.DeepEqual(v.Actions.Missing, tc.want) {
				t.Errorf("actions.missing = %+v, want %+v", v.Actions.Missing, tc.want)
			}
		})
	}

	// The control: over the installed fixture the same requests go through,
	// so the refusals above are the helper's, not the harness's.
	t.Run("installed", func(t *testing.T) {
		t.Parallel()
		h := hostHelperHarness(t, helperState(t, nil), fstest.MapFS{})

		v := readHelperView(t, h)
		if !*v.Actions.Available || len(v.Actions.Missing) != 0 {
			t.Errorf("installed: actions = available %v, missing %+v; want available and nothing missing",
				*v.Actions.Available, v.Actions.Missing)
		}
		resp, raw := h.do(t, http.MethodPost, "/api/v1/host/confirm", map[string]string{
			"action": "host.update", "typed": "example-host",
		})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("installed: POST /api/v1/host/confirm: %d, want 200 (%s)", resp.StatusCode, raw)
		}
		var confirmed struct {
			Token string `json:"token"`
		}
		if err := json.Unmarshal(raw, &confirmed); err != nil || confirmed.Token == "" {
			t.Fatalf("installed: confirm answer %s: %v", raw, err)
		}
		resp, raw = h.do(t, http.MethodPost, "/api/v1/host/actions/update", map[string]string{"confirmation": confirmed.Token})
		if resp.StatusCode != http.StatusAccepted {
			t.Errorf("installed: POST /api/v1/host/actions/update: %d, want 202 (%s)", resp.StatusCode, raw)
		}
		if _, err := os.Lstat(h.dataDir + "/" + hostaction.OrderFileName); err != nil {
			t.Errorf("installed: no order was placed: %v", err)
		}

		// sessionHostToken's tokens are what the refusals above are tried
		// with; this is the proof that the route would take one.
		if err := os.Remove(h.dataDir + "/" + hostaction.OrderFileName); err != nil {
			t.Fatal(err)
		}
		tok := sessionHostToken(t, h.confirmer, h.srv.URL, h.client, "host.update")
		resp, raw = h.do(t, http.MethodPost, "/api/v1/host/actions/update", map[string]string{"confirmation": tok})
		if resp.StatusCode != http.StatusAccepted {
			t.Errorf("installed: a sessionHostToken token: %d, want 202 (%s)", resp.StatusCode, raw)
		}
	})
}

func TestHostActionsInAContainer(t *testing.T) {
	t.Parallel()

	// The helper's files are all there -- the refusal is the container's.
	container := fstest.MapFS{".dockerenv": {}}
	h := hostHelperHarness(t, helperState(t, nil), container)

	requireRefused(t, h, "conflict.host-in-container")

	v := readHelperView(t, h)
	if !v.Container {
		t.Error("container = false over a filesystem with /.dockerenv")
	}
	if *v.Actions.Available {
		t.Error("actions.available = true in a container")
	}
	if len(v.Actions.Missing) != 0 {
		t.Errorf("actions.missing = %+v, want nothing: the helper's files are all there", v.Actions.Missing)
	}
}

// sessionHostToken issues a host token exactly as the confirm route would for
// the session in client's cookie jar: single use, bound to the action, the
// host and a digest of the session id (WR-05). It is for the tests whose
// confirm route refuses before it would hand one out, so that the action
// route is tried with a token that is valid in every respect but the one the
// test is about.
func sessionHostToken(t *testing.T, confirmer *jobs.Confirmer, base string, client *http.Client, action string) string {
	t.Helper()
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	if client.Jar == nil {
		t.Fatal("the client keeps no cookies, so it has no session to bind a host token to")
	}
	for _, c := range client.Jar.Cookies(u) {
		if c.Name == auth.CookieName {
			sum := sha256.Sum256([]byte(c.Value))
			tok, _ := confirmer.IssueOnce(jobs.Intent{
				Action:  action,
				Machine: hostTarget,
				Params:  map[string]string{"session": hex.EncodeToString(sum[:])},
			})
			return tok
		}
	}
	t.Fatalf("no %s cookie for %s", auth.CookieName, base)
	return ""
}
