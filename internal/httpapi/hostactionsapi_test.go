package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"testing/fstest"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/host"
	"github.com/holzcloud/holzkube-manager/internal/host/hostaction"
	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
)

// The host actions end to end (Phase 13): the real confirm and action routes,
// the real order slot over fsstore.PlaceNew, the real helper script run as
// root in a user namespace against a stand-in systemctl, and the real reader
// of what the script recorded -- one test for the whole path, so that a seam
// between two layers (a token the action route cannot check, an order the
// script cannot consume, a result the daemon cannot read) turns it red.

// The paths the operator installs the helper to, as an fs.FS rooted at "/"
// names them.
const (
	helperScriptName  = "usr/local/sbin/holzkube-manager-host"
	helperPathUnit    = "etc/systemd/system/holzkube-manager-host.path"
	helperServiceUnit = "etc/systemd/system/holzkube-manager-host.service"
	helperWantsLink   = "etc/systemd/system/paths.target.wants/holzkube-manager-host.path"
	helperStateDir    = "var/lib/holzkube-manager-host"
)

// overlayFS serves some names from a MapFS and everything else from a real
// directory. It implements fs.ReadLinkFS, so fs.Lstat sees the MapFS's
// symlink as a symlink, as it would on the host.
type overlayFS struct {
	fixed fstest.MapFS
	disk  fs.FS
}

func (o overlayFS) pick(name string) fs.FS {
	if _, ok := o.fixed[name]; ok {
		return o.fixed
	}
	return o.disk
}

func (o overlayFS) Open(name string) (fs.File, error) { return o.pick(name).Open(name) }

func (o overlayFS) Stat(name string) (fs.FileInfo, error) { return fs.Stat(o.pick(name), name) }

func (o overlayFS) Lstat(name string) (fs.FileInfo, error) { return fs.Lstat(o.pick(name), name) }

func (o overlayFS) ReadLink(name string) (string, error) { return fs.ReadLink(o.pick(name), name) }

// installedHelperFS is a root filesystem on which the helper is installed the
// way deploy/HOST-HELPER.md installs it: the script regular, 0755 and owned by
// uid 0, both unit files, and the path unit enabled (its paths.target.wants
// symlink). Everything else -- the helper's state directory in particular --
// is a real temporary directory, returned as stateDir, which the script run
// writes into and the Box reads back through fsys.
func installedHelperFS(t *testing.T) (fsys fs.FS, stateDir string) {
	t.Helper()

	root := t.TempDir()
	stateDir = filepath.Join(root, filepath.FromSlash(helperStateDir))
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Explicitly: MkdirAll honours the umask, and the script refuses a state
	// directory that group or other can write.
	if err := os.Chmod(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}

	fixed := fstest.MapFS{
		helperScriptName: {
			Data:    []byte("#!/usr/bin/env bash\n"),
			Mode:    0o755,
			ModTime: time.Now(),
			Sys:     &syscall.Stat_t{Uid: 0, Gid: 0},
		},
		helperPathUnit:    {Data: []byte("[Path]\nPathExists=" + hostaction.ReferenceOrderPath + "\n"), Mode: 0o644},
		helperServiceUnit: {Data: []byte("[Service]\nType=oneshot\n"), Mode: 0o644},
		helperWantsLink:   {Data: []byte("/" + helperPathUnit), Mode: fs.ModeSymlink | 0o777},
	}
	return overlayFS{fixed: fixed, disk: os.DirFS(root)}, stateDir
}

// helperOverrides are the environment names through which the script copy is
// pointed away from the host's own paths and its real systemctl.
var helperOverrides = []string{
	"HOLZKUBE_MANAGER_HOST_ORDER",
	"HOLZKUBE_MANAGER_HOST_STATE_DIR",
	"HOLZKUBE_MANAGER_SYSTEMCTL",
}

// requireRootNamespace skips, visibly, when root cannot be had in an
// unprivileged user namespace here. On the Pi it was measured working, so there
// the root run happens.
func requireRootNamespace(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skipf("the host helper targets Linux hosts; this is %s", runtime.GOOS)
	}
	for _, tool := range []string{"bash", "unshare", "dd", "stat"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not installed here; the round trip needs it", tool)
		}
	}
	out, err := exec.Command("unshare", "--user", "--map-root-user", "id", "-u").CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "0" {
		t.Skipf("no unprivileged user namespace here: %v: %s", err, out)
	}
}

// runHelper runs a copy of deploy/holzkube-manager-host.sh as root inside a
// user namespace, against order and stateDir, with a stand-in systemctl that
// appends its argv to the returned log path. It returns the exit code, read
// from the process itself.
//
// The copy is refused when it lacks any of the overrides: a script that no
// longer reads them would run against /var/lib and the real systemctl.
func runHelper(t *testing.T, order, stateDir string) (rc int, stubLog string) {
	t.Helper()

	src, err := os.ReadFile(filepath.Join("..", "..", "deploy", "holzkube-manager-host.sh"))
	if err != nil {
		t.Fatalf("read the helper script: %v", err)
	}
	for _, name := range helperOverrides {
		if !strings.Contains(string(src), name) {
			t.Fatalf("the helper script does not read %s; refusing to run it as root, it would address this host", name)
		}
	}

	dir := t.TempDir()
	script := filepath.Join(dir, "holzkube-manager-host.sh")
	if err := os.WriteFile(script, src, 0o755); err != nil { //nolint:gosec // a test copy of a script, run by bash
		t.Fatal(err)
	}
	stubLog = filepath.Join(dir, "systemctl.log")
	stub := filepath.Join(dir, "systemctl")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> '"+stubLog+"'\n"), 0o755); err != nil { //nolint:gosec // the stand-in must be executable
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "unshare", "--user", "--map-root-user", "bash", script)
	// The whole environment: nothing of the test process leaks in.
	cmd.Env = []string{
		"PATH=/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=" + dir,
		"LC_ALL=C",
		"HOLZKUBE_MANAGER_HOST_ORDER=" + order,
		"HOLZKUBE_MANAGER_HOST_STATE_DIR=" + stateDir,
		"HOLZKUBE_MANAGER_SYSTEMCTL=" + stub,
	}
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	t.Logf("helper script (root in a user namespace):\n%s", out)
	if err == nil {
		return 0, stubLog
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("run the helper script: %v", err)
	}
	return exitErr.ExitCode(), stubLog
}

// hostActionsView is the part of GET /api/v1/host this file reads.
type hostActionsView struct {
	Actions struct {
		Order *struct {
			ID     string `json:"id"`
			Action string `json:"action"`
			State  string `json:"state"`
		} `json:"order"`
		Result struct {
			Readable bool `json:"readable"`
			Value    *struct {
				ID      string `json:"id"`
				Action  string `json:"action"`
				Outcome string `json:"outcome"`
				At      string `json:"at"`
			} `json:"value"`
			Reason *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"reason"`
		} `json:"result"`
	} `json:"actions"`
}

// confirmAndPlace asks the confirm route for a token for action, typing typed,
// and presents it to the action route. It returns the action route's answer.
func (h *harness) confirmAndPlace(t *testing.T, action hostaction.Action, typed string) (*http.Response, []byte) {
	t.Helper()
	resp, raw := h.do(t, http.MethodPost, "/api/v1/host/confirm", map[string]string{
		"action": "host." + string(action), "typed": typed,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("confirm host.%s: %d, want 200 (%s)", action, resp.StatusCode, raw)
	}
	var tok struct {
		Token  string `json:"token"`
		Action string `json:"action"`
	}
	if err := json.Unmarshal(raw, &tok); err != nil || tok.Token == "" {
		t.Fatalf("confirm answer %s: %v", raw, err)
	}
	if tok.Action != "host."+string(action) {
		t.Errorf("confirm echoed action %q, want host.%s", tok.Action, action)
	}
	return h.do(t, http.MethodPost, "/api/v1/host/actions/"+string(action),
		map[string]string{"confirmation": tok.Token})
}

func TestHostActionRoundTrip(t *testing.T) {
	t.Parallel()
	requireRootNamespace(t)

	fsys, stateDir := installedHelperFS(t)
	sys := hostSys{uname: host.Uname{Nodename: "example-host", Release: "6.18.50+rpt-rpi-2712", Machine: "aarch64"}}

	h := newHarness(t,
		withHostActions(func(dataDir string) *hostaction.Box {
			// ResultPath left to its default: the fixture's state directory
			// is where the helper's StateDirectory= puts it on the host.
			return hostaction.NewBox(hostaction.Config{
				FS:      fsys,
				DataDir: dataDir,
				Place:   fsstore.PlaceNew,
			})
		}),
		withHostOver(func(box *hostaction.Box) *host.Collector {
			return host.New(host.Config{FS: fstest.MapFS{}, Sys: sys, Actions: box})
		}),
	)
	h.setupAndLogin(t)
	if resp, raw := h.do(t, http.MethodPost, "/api/v1/auth/sudo",
		map[string]string{"password": testPass}); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("sudo: %d (%s)", resp.StatusCode, raw)
	}

	// Before any order: no order, and no result -- a state, not a fault.
	var before hostActionsView
	h.getJSON(t, "/api/v1/host", &before)
	if before.Actions.Order != nil {
		t.Errorf("actions.order before any order = %+v, want null", before.Actions.Order)
	}
	if before.Actions.Result.Readable || before.Actions.Result.Reason == nil ||
		before.Actions.Result.Reason.Code != host.CodeNoResult {
		t.Errorf("actions.result before any order = %+v, want not readable with %s", before.Actions.Result, host.CodeNoResult)
	}

	// Placed through the gates.
	resp, raw := h.confirmAndPlace(t, hostaction.Update, "example-host")
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("POST /api/v1/host/actions/update: %d, want 202 (%s)", resp.StatusCode, raw)
	}
	var placed struct {
		Order struct {
			ID       string `json:"id"`
			Action   string `json:"action"`
			PlacedAt string `json:"placed_at"`
			State    string `json:"state"`
		} `json:"order"`
	}
	if err := json.Unmarshal(raw, &placed); err != nil {
		t.Fatalf("decode 202: %v (%s)", err, raw)
	}
	id := placed.Order.ID
	if len(id) != 16 || strings.Trim(id, "0123456789abcdef") != "" {
		t.Fatalf("order id %q is not 16 lowercase hex characters", id)
	}
	if placed.Order.Action != "update" || placed.Order.State != "pending" {
		t.Errorf("202 order = %+v, want action update, state pending", placed.Order)
	}

	// Exactly one line, and nothing else, mode 0600.
	orderPath := filepath.Join(h.dataDir, hostaction.OrderFileName)
	info, err := os.Lstat(orderPath)
	if err != nil {
		t.Fatalf("the order file: %v", err)
	}
	if info.Mode() != 0o600 {
		t.Errorf("order file mode = %v, want -rw-------", info.Mode())
	}
	first, err := os.ReadFile(orderPath) //nolint:gosec // a path in the test's own data directory
	if err != nil {
		t.Fatal(err)
	}
	if want := "update " + id + "\n"; string(first) != want {
		t.Fatalf("order file = %q, want %q", first, want)
	}

	// One slot: a second order is refused and the first is left as it was.
	resp, raw = h.confirmAndPlace(t, hostaction.Update, "example-host")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("second order: %d, want 409 (%s)", resp.StatusCode, raw)
	}
	if p := decodeProblem(t, resp, raw); p.Code != "conflict.host-order-pending" {
		t.Errorf("second order code = %q, want conflict.host-order-pending", p.Code)
	}
	again, err := os.ReadFile(orderPath) //nolint:gosec // a path in the test's own data directory
	if err != nil {
		t.Fatalf("the first order after the refused second: %v", err)
	}
	if string(again) != string(first) {
		t.Errorf("the refused second order changed the first: %q, was %q", again, first)
	}

	var pending hostActionsView
	h.getJSON(t, "/api/v1/host", &pending)
	if o := pending.Actions.Order; o == nil || o.ID != id || o.State != "pending" {
		t.Errorf("actions.order while the file lies there = %+v, want %s pending", o, id)
	}

	// The root half: the helper consumes the order, runs the timer's unit
	// through the stand-in, and records that it started it.
	rc, stubLog := runHelper(t, orderPath, stateDir)
	if rc != 0 {
		t.Fatalf("helper exit = %d, want 0", rc)
	}
	calls, err := os.ReadFile(stubLog) //nolint:gosec // the test's own stand-in log
	if err != nil {
		t.Fatalf("the stand-in systemctl was never called: %v", err)
	}
	if got, want := string(calls), "start --no-block holzkube-manager-update.service\n"; got != want {
		t.Errorf("systemctl called with %q, want exactly %q", got, want)
	}
	if _, err := os.Lstat(orderPath); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the order is still there after the helper ran: %v", err)
	}

	// And back to the page.
	var after hostActionsView
	h.getJSON(t, "/api/v1/host", &after)
	res := after.Actions.Result
	if !res.Readable || res.Value == nil {
		t.Fatalf("actions.result after the helper ran = %+v, want readable", res)
	}
	if res.Value.ID != id || res.Value.Action != "update" || res.Value.Outcome != "started" {
		t.Errorf("actions.result = %+v, want %s update started", *res.Value, id)
	}
	if _, err := time.Parse(time.RFC3339, res.Value.At); err != nil {
		t.Errorf("actions.result.at %q is not RFC 3339: %v", res.Value.At, err)
	}
	if o := after.Actions.Order; o == nil || o.ID != id || o.State != "picked-up" {
		t.Errorf("actions.order after the helper ran = %+v, want %s picked-up", o, id)
	}
}

// getJSON GETs path with the harness's session and decodes a 200 into v.
func (h *harness) getJSON(t *testing.T, path string, v any) {
	t.Helper()
	resp, raw := h.do(t, http.MethodGet, path, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: %d (%s)", path, resp.StatusCode, raw)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatalf("decode GET %s: %v (%s)", path, err, raw)
	}
}
