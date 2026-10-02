package httpapi_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"testing/fstest"
	"time"

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
	return hostHelperHarnessUp(t, boxFS, collectorFS, nil)
}

// hostHelperHarnessUp is hostHelperHarness with the Box told how long the
// machine has been up (nil: not told).
func hostHelperHarnessUp(t *testing.T, boxFS fs.FS, collectorFS fs.FS, up func() (time.Duration, error)) *harness {
	t.Helper()
	sys := hostSys{uname: host.Uname{Nodename: "example-host", Release: "6.18.50+rpt-rpi-2712", Machine: "aarch64"}}
	h := newHarness(t,
		withHostActions(func(dataDir string) *hostaction.Box {
			return hostaction.NewBox(hostaction.Config{FS: boxFS, DataDir: dataDir, Place: fsstore.PlaceNew, SinceBoot: up})
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
		Available                   *bool                `json:"available"`
		Missing                     []hostaction.Missing `json:"missing"`
		Outdated                    []hostaction.Missing `json:"outdated"`
		UpdateScript                []hostaction.Missing `json:"update_script"`
		Busy                        *bool                `json:"busy"`
		InstallCommands             []string             `json:"install_commands"`
		UpdateScriptInstallCommands []string             `json:"update_script_install_commands"`
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
	if v.Actions.Outdated == nil {
		t.Error("actions.outdated is null or absent; the page reads a list")
	}
	if v.Actions.UpdateScript == nil {
		t.Error("actions.update_script is null or absent; the page reads a list")
	}
	if !reflect.DeepEqual(v.Actions.InstallCommands, hostaction.InstallCommands) {
		t.Errorf("actions.install_commands = %q, want hostaction.InstallCommands %q",
			v.Actions.InstallCommands, hostaction.InstallCommands)
	}
	if !reflect.DeepEqual(v.Actions.UpdateScriptInstallCommands, hostaction.UpdateScriptInstallCommands) {
		t.Errorf("actions.update_script_install_commands = %q, want hostaction.UpdateScriptInstallCommands %q",
			v.Actions.UpdateScriptInstallCommands, hostaction.UpdateScriptInstallCommands)
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
		if !*v.Actions.Available || len(v.Actions.Missing) != 0 || len(v.Actions.Outdated) != 0 || len(v.Actions.UpdateScript) != 0 {
			t.Errorf("installed: actions = available %v, missing %+v, outdated %+v, update_script %+v; want available, nothing missing or outdated",
				*v.Actions.Available, v.Actions.Missing, v.Actions.Outdated, v.Actions.UpdateScript)
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

// olderHelperScript is the helper as it was before the check, frozen at
// 8b64a06 by 13-13: installed completely, and it refuses check-update.
func olderHelperScript(t *testing.T) *fstest.MapFile {
	t.Helper()
	b, err := os.ReadFile("../host/hostaction/testdata/holzkube-manager-host-before-check.sh")
	if err != nil {
		t.Fatal(err)
	}
	return &fstest.MapFile{Data: b, Mode: 0o755, Sys: &syscall.Stat_t{Uid: 0, Gid: 0}}
}

// requireCheckRefused asks the confirm route for host.check-update and the
// check's action route with a valid token, and wants 409 code from both, no
// token handed out and nothing placed.
func requireCheckRefused(t *testing.T, h *harness, code string) {
	t.Helper()

	resp, raw := h.do(t, http.MethodPost, "/api/v1/host/confirm", map[string]string{
		"action": "host.check-update", "typed": "example-host",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("POST /api/v1/host/confirm for host.check-update: %d, want 409 %s (%s)", resp.StatusCode, code, raw)
	} else if p := decodeProblem(t, resp, raw); p.Code != code {
		t.Errorf("POST /api/v1/host/confirm for host.check-update: code %q, want %q", p.Code, code)
	}
	if strings.Contains(string(raw), `"token"`) {
		t.Errorf("POST /api/v1/host/confirm for host.check-update handed out a token: %s", raw)
	}

	tok := sessionHostToken(t, h.confirmer, h.srv.URL, h.client, "host.check-update")
	const path = "/api/v1/host/actions/check-update"
	resp, raw = h.do(t, http.MethodPost, path, map[string]string{"confirmation": tok})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("POST %s with a valid host token: %d, want 409 %s (%s)", path, resp.StatusCode, code, raw)
	} else if p := decodeProblem(t, resp, raw); p.Code != code {
		t.Errorf("POST %s: code %q, want %q", path, p.Code, code)
	}
	requireNothingPlaced(t, h, path)
}

// TestHostCheckNeedsANewerHelper: a helper installed completely but too old
// for the check -- its script names no check-update on the marker line, or
// the check unit is not there -- keeps the four older orders and is never
// handed a check (D-12, D-14, HACT-04). The routes refuse the check with 409
// before a token is issued or checked and before Place; reboot still goes
// through. And the refusals keep their order: container, missing, outdated.
func TestHostCheckNeedsANewerHelper(t *testing.T) {
	t.Parallel()

	scriptOutdated := hostaction.Missing{Item: hostaction.OutdatedScript, Path: hostaction.HelperScriptPath}
	checkUnit := hostaction.Missing{Item: hostaction.OutdatedCheckUnit, Path: hostaction.UpdateCheckUnitPath}

	cases := []struct {
		name string
		edit func(t *testing.T, m fstest.MapFS)
		want []hostaction.Missing
	}{
		{"the helper before the check", func(t *testing.T, m fstest.MapFS) {
			m[helperScriptName] = olderHelperScript(t)
		}, []hostaction.Missing{scriptOutdated}},
		{"no check unit", func(_ *testing.T, m fstest.MapFS) { delete(m, helperCheckUnit) }, []hostaction.Missing{checkUnit}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := hostHelperHarness(t, helperState(t, func(m fstest.MapFS) { tc.edit(t, m) }), fstest.MapFS{})

			v := readHelperView(t, h)
			if !*v.Actions.Available || len(v.Actions.Missing) != 0 {
				t.Errorf("actions = available %v, missing %+v; want available and nothing missing: the four older orders work",
					*v.Actions.Available, v.Actions.Missing)
			}
			if !reflect.DeepEqual(v.Actions.Outdated, tc.want) {
				t.Errorf("actions.outdated = %+v, want %+v", v.Actions.Outdated, tc.want)
			}

			requireCheckRefused(t, h, "conflict.host-helper-outdated")

			// The control: reboot confirms and places over the same helper.
			resp, raw := h.do(t, http.MethodPost, "/api/v1/host/confirm", map[string]string{
				"action": "host.reboot", "typed": "example-host",
			})
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("POST /api/v1/host/confirm for host.reboot: %d, want 200 (%s)", resp.StatusCode, raw)
			}
			var confirmed struct {
				Token string `json:"token"`
			}
			if err := json.Unmarshal(raw, &confirmed); err != nil || confirmed.Token == "" {
				t.Fatalf("confirm answer %s: %v", raw, err)
			}
			resp, raw = h.do(t, http.MethodPost, "/api/v1/host/actions/reboot", map[string]string{"confirmation": confirmed.Token})
			if resp.StatusCode != http.StatusAccepted {
				t.Errorf("POST /api/v1/host/actions/reboot: %d, want 202 (%s)", resp.StatusCode, raw)
			}
			if _, err := os.Lstat(h.dataDir + "/" + hostaction.OrderFileName); err != nil {
				t.Errorf("no reboot order was placed: %v", err)
			}
		})
	}

	// In a container the container's refusal comes first, whatever the
	// helper's age.
	t.Run("in a container", func(t *testing.T) {
		t.Parallel()
		older := helperState(t, func(m fstest.MapFS) { m[helperScriptName] = olderHelperScript(t) })
		h := hostHelperHarness(t, older, fstest.MapFS{".dockerenv": {}})
		requireCheckRefused(t, h, "conflict.host-in-container")
	})

	// With a piece missing the missing refusal comes first: the install
	// commands install the newer script too.
	t.Run("missing and older", func(t *testing.T) {
		t.Parallel()
		older := helperState(t, func(m fstest.MapFS) {
			m[helperScriptName] = olderHelperScript(t)
			delete(m, helperWantsLink)
		})
		h := hostHelperHarness(t, older, fstest.MapFS{})
		requireCheckRefused(t, h, "conflict.host-helper-missing")
		if v := readHelperView(t, h); len(v.Actions.Outdated) != 0 {
			t.Errorf("actions.outdated = %+v while missing is %+v; want nothing until the helper is installed",
				v.Actions.Outdated, v.Actions.Missing)
		}
	})
}

// The paths, as an fs.FS rooted at "/" names them, of the helper's result
// and of the update script's status file.
const (
	helperResultName = helperStateDir + "/last"
	updateStatusName = "var/lib/holzkube-manager-update/status.json"
)

// checkBusyDetail is the refusal's detail: the page's own reason line for a
// running check, and that nothing was placed.
const checkBusyDetail = "An update check is running; wait for it to finish. No order was placed."

// helperRecorded puts the helper's result line into the fixture.
func helperRecorded(line string) func(m fstest.MapFS) {
	return func(m fstest.MapFS) {
		m[helperResultName] = &fstest.MapFile{Data: []byte(line + "\n"), Mode: 0o644}
	}
}

// updateRecorded is a filesystem holding a status file the update script
// wrote at checkedAt.
func updateRecorded(checkedAt time.Time) fstest.MapFS {
	return fstest.MapFS{updateStatusName: {
		Data: []byte(`{"checked_at":"` + checkedAt.UTC().Format(time.RFC3339) +
			`","installed":"v0.2.0","latest":"v0.2.0","outcome":"current"}` + "\n"),
		Mode: 0o644,
	}}
}

// requireGoesThrough confirms and places action and wants 200 and 202: the
// control that a harness which refuses does so for the reason under test.
func requireGoesThrough(t *testing.T, h *harness, action hostaction.Action) {
	t.Helper()
	resp, raw := h.confirmAndPlace(t, action, "example-host")
	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("confirm and place %s: %d, want 202 (%s)", action, resp.StatusCode, raw)
	}
	if _, err := os.Lstat(h.dataDir + "/" + hostaction.OrderFileName); err != nil {
		t.Errorf("no %s order was placed: %v", action, err)
	}
}

// TestHostActionsWaitForARunningCheck (13-REVIEW-2 WR-01, and its
// verification V-01, V-02, V-21): the helper carries out a check blocking --
// it waits for the check unit, up to its own service's limit -- so while one
// runs it picks up nothing else. An order placed then would lie in the slot
// until the Box withdrew it after 10 s. So while the helper's own last record
// is a check it has started and not yet recorded done or failed, younger than
// that limit and not from before the last boot, both routes refuse every host
// action with 409 conflict.host-helper-busy: no token, nothing placed, and
// actions.busy says so to the page. The update status has no say: the hourly
// run writes it too, also while a check runs.
//
// The refusals keep their order: container, missing, outdated, busy.
func TestHostActionsWaitForARunningCheck(t *testing.T) {
	t.Parallel()

	// Every time is taken inside the subtest, just before its harness is
	// built: the subtests run in parallel with the whole package, and a
	// "5 s ago" taken when the test began can be minutes old by then.
	const id = "c0ffee00c0ffee11"
	stamp := func(at time.Time) string { return at.UTC().Truncate(time.Second).Format(time.RFC3339) }
	recorded := func(action, outcome string, ago time.Duration) func(now time.Time) func(m fstest.MapFS) {
		return func(now time.Time) func(m fstest.MapFS) {
			return helperRecorded(id + " " + action + " " + outcome + " " + stamp(now.Add(-ago)))
		}
	}
	checkStarted := func(ago time.Duration) func(now time.Time) func(m fstest.MapFS) {
		return recorded("check-update", "started", ago)
	}
	noStatus := func(time.Time) fstest.MapFS { return fstest.MapFS{} }
	statusAgo := func(ago time.Duration) func(now time.Time) fstest.MapFS {
		return func(now time.Time) fstest.MapFS { return updateRecorded(now.Add(-ago)) }
	}
	type fixture struct {
		name   string
		edit   func(now time.Time) func(m fstest.MapFS)
		status func(now time.Time) fstest.MapFS
		// up is how long the machine has been up; zero: an hour.
		up time.Duration
	}
	build := func(t *testing.T, f fixture) *harness {
		t.Helper()
		now := time.Now()
		var edit func(m fstest.MapFS)
		if f.edit != nil {
			edit = f.edit(now)
		}
		up := f.up
		if up == 0 {
			up = time.Hour
		}
		// The update status on both filesystems, as on the host, where the
		// Box and the host reader read the same root: whether the Box looks
		// at it is the question, not whether it can.
		status := f.status(now)
		both := func(m fstest.MapFS) {
			if edit != nil {
				edit(m)
			}
			for name, file := range status {
				m[name] = file
			}
		}
		return hostHelperHarnessUp(t, helperState(t, both), status, func() (time.Duration, error) { return up, nil })
	}

	busy := []fixture{
		{name: "a check started 5 s ago, no update status yet", edit: checkStarted(5 * time.Second), status: noStatus},
		{name: "a check started 5 s ago, the update status from an hour before", edit: checkStarted(5 * time.Second), status: statusAgo(time.Hour)},
		// V-01: the hourly run ended during the check and wrote the status;
		// the helper still waits for the check.
		{name: "a check started 5 s ago, an hourly run recorded since", edit: checkStarted(5 * time.Second), status: statusAgo(0)},
		{name: "a check started 5 s ago, an update status in its own second", edit: checkStarted(5 * time.Second), status: statusAgo(5 * time.Second)},
		{name: "a check started 5 s ago, an update status from the future", edit: checkStarted(5 * time.Second), status: statusAgo(-time.Hour)},
		// The check unit may take 2 min, and its stop up to 4 x 10 s when a
		// process survives every signal: still the helper's.
		{name: "a check started 2 min 40 s ago", edit: checkStarted(160 * time.Second), status: noStatus},
	}
	for _, tc := range busy {
		t.Run("refused: "+tc.name, func(t *testing.T) {
			t.Parallel()
			h := build(t, tc)

			requireRefused(t, h, "conflict.host-helper-busy")
			requireCheckRefused(t, h, "conflict.host-helper-busy")

			tok := sessionHostToken(t, h.confirmer, h.srv.URL, h.client, "host.reboot")
			resp, raw := h.do(t, http.MethodPost, "/api/v1/host/actions/reboot", map[string]string{"confirmation": tok})
			if resp.StatusCode == http.StatusConflict {
				if p := decodeProblem(t, resp, raw); p.Detail != checkBusyDetail {
					t.Errorf("detail = %q, want %q", p.Detail, checkBusyDetail)
				}
			}
			requireNothingPlaced(t, h, "/api/v1/host/actions/reboot")

			// Available stays true: the helper is installed. busy is what
			// turns the page's buttons off.
			v := readHelperView(t, h)
			if !*v.Actions.Available {
				t.Error("actions.available = false while a check runs; the helper is installed")
			}
			if v.Actions.Busy == nil || !*v.Actions.Busy {
				t.Errorf("actions.busy = %v while the routes refuse as busy, want true", v.Actions.Busy)
			}
		})
	}

	// The controls: the same fixture with the check over, or never a check,
	// goes through -- so the refusals above are the running check's.
	through := []fixture{
		{name: "a check the helper recorded done", edit: recorded("check-update", "done", 5*time.Second), status: noStatus},
		{name: "a check that failed", edit: recorded("check-update", "failed", 5*time.Second), status: noStatus},
		{name: "a check started 4 min ago, past the helper's 3-min limit", edit: checkStarted(4 * time.Minute), status: noStatus},
		// V-02: the machine went down mid-check; nothing waits for it now.
		{name: "a check started 40 s ago, the machine up for 20 s", edit: checkStarted(40 * time.Second), status: noStatus, up: 20 * time.Second},
		{name: "an update started", edit: recorded("update", "started", 5*time.Second), status: noStatus},
		{name: "a check recorded in the future (the clock went back)", edit: checkStarted(-time.Hour), status: noStatus},
		{name: "nothing recorded", status: noStatus},
	}
	for _, tc := range through {
		t.Run("goes through: "+tc.name, func(t *testing.T) {
			t.Parallel()
			h := build(t, tc)
			if v := readHelperView(t, h); v.Actions.Busy == nil || *v.Actions.Busy {
				t.Errorf("actions.busy = %v, want false", v.Actions.Busy)
			}
			requireGoesThrough(t, h, hostaction.Reboot)
		})
	}

	// The order: a container, a missing piece and an older helper are each
	// asked before the running check.
	t.Run("in a container and busy", func(t *testing.T) {
		t.Parallel()
		h := build(t, fixture{edit: checkStarted(5 * time.Second), status: func(time.Time) fstest.MapFS {
			return fstest.MapFS{".dockerenv": {}}
		}})
		requireRefused(t, h, "conflict.host-in-container")
	})
	t.Run("missing and busy", func(t *testing.T) {
		t.Parallel()
		h := build(t, fixture{edit: func(now time.Time) func(m fstest.MapFS) {
			return func(m fstest.MapFS) {
				checkStarted(5 * time.Second)(now)(m)
				delete(m, helperWantsLink)
			}
		}, status: noStatus})
		requireRefused(t, h, "conflict.host-helper-missing")
	})
	t.Run("outdated and busy", func(t *testing.T) {
		t.Parallel()
		h := build(t, fixture{edit: func(now time.Time) func(m fstest.MapFS) {
			return func(m fstest.MapFS) {
				checkStarted(5 * time.Second)(now)(m)
				delete(m, helperCheckUnit)
			}
		}, status: noStatus})
		requireCheckRefused(t, h, "conflict.host-helper-outdated")
		// The four older actions are refused as busy.
		for _, action := range []string{"reboot", "update"} {
			tok := sessionHostToken(t, h.confirmer, h.srv.URL, h.client, "host."+action)
			resp, raw := h.do(t, http.MethodPost, "/api/v1/host/actions/"+action, map[string]string{"confirmation": tok})
			if resp.StatusCode != http.StatusConflict {
				t.Errorf("POST %s: %d, want 409 conflict.host-helper-busy (%s)", action, resp.StatusCode, raw)
			} else if p := decodeProblem(t, resp, raw); p.Code != "conflict.host-helper-busy" {
				t.Errorf("POST %s: code %q, want conflict.host-helper-busy", action, p.Code)
			}
			requireNothingPlaced(t, h, action)
		}
	})

	// The page's reason line for a running check is the refusal's first
	// sentence, so the greyed-out button and a refused client read the same.
	src, err := os.ReadFile("../../web/src/components/HostActions.tsx")
	if err != nil {
		t.Fatal(err)
	}
	if want := "checkRunning: '" + strings.TrimSuffix(checkBusyDetail, " No order was placed.") + "'"; !strings.Contains(string(src), want) {
		t.Errorf("web/src/components/HostActions.tsx has no %s; the page's reason and the refusal disagree", want)
	}
	// The Box holds a taken check back for as long as the page waits for its
	// record before it says "no answer".
	if got := pageMillis(t, string(src), "RESULT_WITHIN_MS"); got != hostaction.ResultWithin {
		t.Errorf("web/src/components/HostActions.tsx: RESULT_WITHIN_MS is %v, hostaction.ResultWithin %v; the page and the Box disagree",
			got, hostaction.ResultWithin)
	}
	// And a check for as long as the routes hold its record to be running
	// (13-REVIEW-2 V-04 c): past it the page says "no answer".
	if got := pageMillis(t, string(src), "CHECK_WITHIN_MS"); got != hostaction.HelperServiceLimit {
		t.Errorf("web/src/components/HostActions.tsx: CHECK_WITHIN_MS is %v, hostaction.HelperServiceLimit %v; the page and the routes disagree",
			got, hostaction.HelperServiceLimit)
	}
}

// pageMillis is the value of the page constant name, declared in src as
// `export const NAME = <product>` and nowhere else, where the product is
// integer literals joined by " * " and nothing more on the line -- no other
// operator, no comment. Anything else fails: a substring match would pass
// "3 * 60 * 1000 * 2", or a comment beside a changed value (13-REVIEW-2
// round 3, I2).
func pageMillis(t *testing.T, src, name string) time.Duration {
	t.Helper()
	decl := regexp.MustCompile(`(?m)^export const ` + regexp.QuoteMeta(name) + ` = (.*)$`)
	m := decl.FindAllStringSubmatch(src, -1)
	if n := len(regexp.MustCompile(`\b`+regexp.QuoteMeta(name)+`\s*=[^=]`).FindAllString(src, -1)); len(m) != 1 || n != 1 {
		t.Fatalf("web/src/components/HostActions.tsx declares %s %d times and assigns it %d times, want exactly one `export const %s = ...`", name, len(m), n, name)
	}
	expr := m[0][1]
	if !regexp.MustCompile(`^[0-9]+( \* [0-9]+)*$`).MatchString(expr) {
		t.Fatalf("web/src/components/HostActions.tsx: %s = %q is not a product of integer literals and nothing else on the line", name, expr)
	}
	ms := int64(1)
	for _, f := range strings.Split(expr, " * ") {
		n, err := strconv.ParseInt(f, 10, 64)
		if err != nil || n > 1<<20 {
			t.Fatalf("web/src/components/HostActions.tsx: %s: factor %q is not a small integer", name, f)
		}
		ms *= n
		if ms > int64(24*time.Hour/time.Millisecond) {
			t.Fatalf("web/src/components/HostActions.tsx: %s = %s is more than a day", name, expr)
		}
	}
	return time.Duration(ms) * time.Millisecond
}

// TestABusyRefusalKeepsTheToken (13-REVIEW-2 V-04): the action route refuses
// a busy helper before it looks at the token, so a client turned away keeps
// its confirmation and can place the order once the check is over, without
// typing the hostname again. The token is replayed after the check recorded
// done, and must still be good.
func TestABusyRefusalKeepsTheToken(t *testing.T) {
	t.Parallel()

	fsys, stateDir := installedHelperFS(t)
	last := filepath.Join(stateDir, "last")
	write := func(outcome string) {
		t.Helper()
		line := "c0ffee00c0ffee11 check-update " + outcome + " " +
			time.Now().UTC().Add(-5*time.Second).Truncate(time.Second).Format(time.RFC3339) + "\n"
		if err := os.WriteFile(last, []byte(line), 0o644); err != nil { //nolint:gosec // the reader wants 0644, as the helper writes it
			t.Fatal(err)
		}
	}
	write("started")
	h := hostHelperHarness(t, fsys, fstest.MapFS{})

	tok := sessionHostToken(t, h.confirmer, h.srv.URL, h.client, "host.reboot")
	const path = "/api/v1/host/actions/reboot"
	resp, raw := h.do(t, http.MethodPost, path, map[string]string{"confirmation": tok})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("POST %s while a check runs: %d, want 409 conflict.host-helper-busy (%s)", path, resp.StatusCode, raw)
	}
	if p := decodeProblem(t, resp, raw); p.Code != "conflict.host-helper-busy" {
		t.Fatalf("POST %s: code %q, want conflict.host-helper-busy", path, p.Code)
	}
	requireNothingPlaced(t, h, path)

	write("done")
	resp, raw = h.do(t, http.MethodPost, path, map[string]string{"confirmation": tok})
	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("POST %s with the same token after the check: %d, want 202 -- the busy refusal spent it (%s)", path, resp.StatusCode, raw)
	}
}

// flipResultFS hides the helper's result file from the first Stat and shows
// it from the second on: the helper recording "started" for a check in the
// moment between the action route's busy question and its placement.
type flipResultFS struct {
	fs.FS
	stats *atomic.Int32
}

func (f flipResultFS) Stat(name string) (fs.FileInfo, error) {
	if name == helperResultName && f.stats.Add(1) == 1 {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
	}
	return fs.Stat(f.FS, name)
}

func (f flipResultFS) Lstat(name string) (fs.FileInfo, error) { return fs.Lstat(f.FS, name) }

func (f flipResultFS) ReadLink(name string) (string, error) { return fs.ReadLink(f.FS, name) }

// TestAHelperBusyAtPlacementIsRefusedAsBusy (13-REVIEW-2 V-22): Place asks
// the busy question once more under the slot's lock, and the route answers
// what it says as the same 409, not as an internal error.
func TestAHelperBusyAtPlacementIsRefusedAsBusy(t *testing.T) {
	t.Parallel()

	fsys, stateDir := installedHelperFS(t)
	line := "c0ffee00c0ffee11 check-update started " + time.Now().UTC().Truncate(time.Second).Format(time.RFC3339) + "\n"
	if err := os.WriteFile(filepath.Join(stateDir, "last"), []byte(line), 0o644); err != nil { //nolint:gosec // as the helper writes it
		t.Fatal(err)
	}
	stats := &atomic.Int32{}
	h := hostHelperHarness(t, flipResultFS{FS: fsys, stats: stats}, fstest.MapFS{})

	tok := sessionHostToken(t, h.confirmer, h.srv.URL, h.client, "host.reboot")
	const path = "/api/v1/host/actions/reboot"
	resp, raw := h.do(t, http.MethodPost, path, map[string]string{"confirmation": tok})
	if n := stats.Load(); n < 2 {
		t.Fatalf("the result file was asked for %d times, want the route's question and Place's: the flip injected nothing", n)
	}
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("POST %s, the check taken between the question and the placement: %d, want 409 (%s)", path, resp.StatusCode, raw)
	} else if p := decodeProblem(t, resp, raw); p.Code != "conflict.host-helper-busy" || p.Detail != checkBusyDetail {
		t.Errorf("POST %s: %q %q, want conflict.host-helper-busy %q", path, p.Code, p.Detail, checkBusyDetail)
	}
	requireNothingPlaced(t, h, path)
}

// TestHostUpdateNeedsTheUpdateUnit (13-16): the update order starts
// holzkube-manager-update.service, which neither the helper's install commands
// nor the update script's install. While /etc/systemd/system holds no regular
// file under its name -- absent, a directory, or masked (a symlink to
// /dev/null) -- the confirm route and the action route refuse update with 409
// conflict.host-update-unit-missing, before a token is issued or checked and
// before anything is placed. check-update, reboot, poweroff and
// restart-service start no such unit and go through over the same helper.
//
// The order of the refusals: container, missing, update script, update unit,
// outdated, busy. The unit after the script, because the script is the reason
// both update buttons share; before busy, because waiting will not cure it.
//
// Faults injected and seen red (13-16): the action route's question removed;
// the confirm route's removed; the unit's question moved before the update
// script's; the unit dropped from installedHelperFS (the round trips that
// place update).
func TestHostUpdateNeedsTheUpdateUnit(t *testing.T) {
	t.Parallel()

	const code = "conflict.host-update-unit-missing"
	// The page shows the note since 13-17, so the detail points at it, as the
	// update script's sibling refusal does (13-REVIEW-3 IN-01).
	const detail = "The unit holzkube-manager-update.service is not installed, so no order was placed. The Host page says how to install it."

	// refusedUpdate wants code from the confirm route and the action route of
	// update, no token and nothing placed.
	refusedUpdate := func(t *testing.T, h *harness, code string) {
		t.Helper()
		resp, raw := h.do(t, http.MethodPost, "/api/v1/host/confirm", map[string]string{
			"action": "host.update", "typed": "example-host",
		})
		if resp.StatusCode != http.StatusConflict {
			t.Errorf("POST /api/v1/host/confirm for host.update: %d, want 409 %s (%s)", resp.StatusCode, code, raw)
		} else if p := decodeProblem(t, resp, raw); p.Code != code {
			t.Errorf("POST /api/v1/host/confirm for host.update: code %q, want %q", p.Code, code)
		}
		if strings.Contains(string(raw), `"token"`) {
			t.Errorf("POST /api/v1/host/confirm for host.update handed out a token: %s", raw)
		}

		tok := sessionHostToken(t, h.confirmer, h.srv.URL, h.client, "host.update")
		const path = "/api/v1/host/actions/update"
		resp, raw = h.do(t, http.MethodPost, path, map[string]string{"confirmation": tok})
		if resp.StatusCode != http.StatusConflict {
			t.Errorf("POST %s with a valid host token: %d, want 409 %s (%s)", path, resp.StatusCode, code, raw)
		} else if p := decodeProblem(t, resp, raw); p.Code != code {
			t.Errorf("POST %s: code %q, want %q", path, p.Code, code)
		}
		requireNothingPlaced(t, h, path)
	}

	// masked is the helper installed with the update unit masked the way
	// systemctl mask does it: a real symlink to /dev/null, which fs.Stat
	// follows on disk as it does on the host.
	masked := func(t *testing.T) fs.FS {
		t.Helper()
		fsys, stateDir := installedHelperFS(t)
		o, ok := fsys.(overlayFS)
		if !ok {
			t.Fatalf("installedHelperFS returned %T, not an overlayFS", fsys)
		}
		delete(o.fixed, helperUpdateUnit)
		root := strings.TrimSuffix(stateDir, filepath.FromSlash("/"+helperStateDir))
		unitFile := filepath.Join(root, filepath.FromSlash(helperUpdateUnit))
		if err := os.MkdirAll(filepath.Dir(unitFile), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("/dev/null", unitFile); err != nil {
			t.Fatal(err)
		}
		return o
	}

	cases := []struct {
		name string
		fsys func(t *testing.T) fs.FS
	}{
		{"absent", func(t *testing.T) fs.FS {
			return helperState(t, func(m fstest.MapFS) { delete(m, helperUpdateUnit) })
		}},
		{"a directory", func(t *testing.T) fs.FS {
			return helperState(t, func(m fstest.MapFS) {
				m[helperUpdateUnit] = &fstest.MapFile{Mode: fs.ModeDir | 0o755}
			})
		}},
		{"masked", masked},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := hostHelperHarness(t, tc.fsys(t), fstest.MapFS{})

			v := readHelperView(t, h)
			if !*v.Actions.Available || len(v.Actions.Missing) != 0 || len(v.Actions.Outdated) != 0 || len(v.Actions.UpdateScript) != 0 {
				t.Errorf("actions = available %v, missing %+v, outdated %+v, update_script %+v; want the helper and the script installed",
					*v.Actions.Available, v.Actions.Missing, v.Actions.Outdated, v.Actions.UpdateScript)
			}

			refusedUpdate(t, h, code)
			resp, raw := h.do(t, http.MethodPost, "/api/v1/host/confirm", map[string]string{
				"action": "host.update", "typed": "example-host",
			})
			if p := decodeProblem(t, resp, raw); p.Detail != detail {
				t.Errorf("detail = %q, want %q", p.Detail, detail)
			}

			// The control: the four that start no update unit go through,
			// the check included -- last, because a check the helper has
			// taken and not yet answered makes it busy for every order after.
			for _, a := range []hostaction.Action{hostaction.Reboot, hostaction.Poweroff, hostaction.RestartService, hostaction.CheckUpdate} {
				requireGoesThrough(t, h, a)
				if err := os.Remove(h.dataDir + "/" + hostaction.OrderFileName); err != nil {
					t.Fatal(err)
				}
			}
		})
	}

	// The order of the refusals, each with the unit missing too.
	noUnit := func(edit func(m fstest.MapFS)) func(m fstest.MapFS) {
		return func(m fstest.MapFS) {
			delete(m, helperUpdateUnit)
			if edit != nil {
				edit(m)
			}
		}
	}
	t.Run("in a container", func(t *testing.T) {
		t.Parallel()
		h := hostHelperHarness(t, helperState(t, noUnit(nil)), fstest.MapFS{".dockerenv": {}})
		refusedUpdate(t, h, "conflict.host-in-container")
	})
	t.Run("helper missing", func(t *testing.T) {
		t.Parallel()
		h := hostHelperHarness(t, helperState(t, noUnit(func(m fstest.MapFS) { delete(m, helperWantsLink) })), fstest.MapFS{})
		refusedUpdate(t, h, "conflict.host-helper-missing")
	})
	t.Run("update script missing too", func(t *testing.T) {
		t.Parallel()
		h := hostHelperHarness(t, helperState(t, noUnit(func(m fstest.MapFS) { delete(m, helperUpdateScript) })), fstest.MapFS{})
		refusedUpdate(t, h, "conflict.host-update-script-missing")
	})
	t.Run("busy too", func(t *testing.T) {
		t.Parallel()
		started := "c0ffee00c0ffee11 check-update started " + time.Now().UTC().Add(-5*time.Second).Truncate(time.Second).Format(time.RFC3339)
		h := hostHelperHarnessUp(t, helperState(t, noUnit(helperRecorded(started))), fstest.MapFS{},
			func() (time.Duration, error) { return time.Hour, nil })
		// The unit before busy: waiting will not cure it.
		refusedUpdate(t, h, code)
	})
}

// TestHostUpdateActionsNeedTheUpdateScript (13-REVIEW-2 IN-04): both update
// orders end in /usr/local/sbin/holzkube-manager-update, which the helper's
// install commands do not install. While it is not a root-owned executable
// nobody else may change, the confirm route and the action route refuse
// update and check-update with 409 conflict.host-update-script-missing --
// before a token is issued or checked, and before anything is placed -- and
// actions.update_script names it. reboot, poweroff and restart-service go
// through over the same helper.
func TestHostUpdateActionsNeedTheUpdateScript(t *testing.T) {
	t.Parallel()

	const code = "conflict.host-update-script-missing"
	const detail = "The update script /usr/local/sbin/holzkube-manager-update is not installed, so no order was placed. The Host page says how to install it."
	want := []hostaction.Missing{{Item: hostaction.MissingUpdateScript, Path: hostaction.UpdateScriptPath}}
	updateActions := []hostaction.Action{hostaction.Update, hostaction.CheckUpdate}

	// refusedBoth wants code from the confirm route and the action route of
	// both update actions, no token and nothing placed.
	refusedBoth := func(t *testing.T, h *harness, code string) {
		t.Helper()
		for _, a := range updateActions {
			resp, raw := h.do(t, http.MethodPost, "/api/v1/host/confirm", map[string]string{
				"action": "host." + string(a), "typed": "example-host",
			})
			if resp.StatusCode != http.StatusConflict {
				t.Errorf("POST /api/v1/host/confirm for host.%s: %d, want 409 %s (%s)", a, resp.StatusCode, code, raw)
			} else if p := decodeProblem(t, resp, raw); p.Code != code {
				t.Errorf("POST /api/v1/host/confirm for host.%s: code %q, want %q", a, p.Code, code)
			}
			if strings.Contains(string(raw), `"token"`) {
				t.Errorf("POST /api/v1/host/confirm for host.%s handed out a token: %s", a, raw)
			}

			tok := sessionHostToken(t, h.confirmer, h.srv.URL, h.client, "host."+string(a))
			path := "/api/v1/host/actions/" + string(a)
			resp, raw = h.do(t, http.MethodPost, path, map[string]string{"confirmation": tok})
			if resp.StatusCode != http.StatusConflict {
				t.Errorf("POST %s with a valid host token: %d, want 409 %s (%s)", path, resp.StatusCode, code, raw)
			} else if p := decodeProblem(t, resp, raw); p.Code != code {
				t.Errorf("POST %s: code %q, want %q", path, p.Code, code)
			}
			requireNothingPlaced(t, h, path)
		}
	}

	cases := []struct {
		name string
		edit func(m fstest.MapFS)
	}{
		{"absent", func(m fstest.MapFS) { delete(m, helperUpdateScript) }},
		{"owned by uid 1000", func(m fstest.MapFS) {
			m[helperUpdateScript] = &fstest.MapFile{Data: []byte("#!/bin/sh\n"), Mode: 0o755, Sys: &syscall.Stat_t{Uid: 1000}}
		}},
		{"writable by its group", func(m fstest.MapFS) {
			m[helperUpdateScript] = &fstest.MapFile{Data: []byte("#!/bin/sh\n"), Mode: 0o775, Sys: &syscall.Stat_t{Uid: 0}}
		}},
		{"a directory", func(m fstest.MapFS) {
			m[helperUpdateScript] = &fstest.MapFile{Mode: fs.ModeDir | 0o755, Sys: &syscall.Stat_t{Uid: 0}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := hostHelperHarness(t, helperState(t, tc.edit), fstest.MapFS{})

			v := readHelperView(t, h)
			if !*v.Actions.Available || len(v.Actions.Missing) != 0 || len(v.Actions.Outdated) != 0 {
				t.Errorf("actions = available %v, missing %+v, outdated %+v; want available: the helper is installed",
					*v.Actions.Available, v.Actions.Missing, v.Actions.Outdated)
			}
			if !reflect.DeepEqual(v.Actions.UpdateScript, want) {
				t.Errorf("actions.update_script = %+v, want %+v", v.Actions.UpdateScript, want)
			}

			refusedBoth(t, h, code)
			resp, raw := h.do(t, http.MethodPost, "/api/v1/host/confirm", map[string]string{
				"action": "host.update", "typed": "example-host",
			})
			if p := decodeProblem(t, resp, raw); p.Detail != detail {
				t.Errorf("detail = %q, want %q", p.Detail, detail)
			}

			// The control: the three that do not need it go through.
			for _, a := range []hostaction.Action{hostaction.Reboot, hostaction.Poweroff, hostaction.RestartService} {
				requireGoesThrough(t, h, a)
				if err := os.Remove(h.dataDir + "/" + hostaction.OrderFileName); err != nil {
					t.Fatal(err)
				}
			}
		})
	}

	// The order of the refusals: container, missing, update script, outdated,
	// busy.
	t.Run("in a container", func(t *testing.T) {
		t.Parallel()
		h := hostHelperHarness(t, helperState(t, func(m fstest.MapFS) { delete(m, helperUpdateScript) }),
			fstest.MapFS{".dockerenv": {}})
		refusedBoth(t, h, "conflict.host-in-container")
	})
	t.Run("missing", func(t *testing.T) {
		t.Parallel()
		h := hostHelperHarness(t, helperState(t, func(m fstest.MapFS) {
			delete(m, helperUpdateScript)
			delete(m, helperWantsLink)
		}), fstest.MapFS{})
		refusedBoth(t, h, "conflict.host-helper-missing")
		// Named all the same: the helper's install commands do not bring it.
		if v := readHelperView(t, h); !reflect.DeepEqual(v.Actions.UpdateScript, want) {
			t.Errorf("actions.update_script = %+v while the helper is missing, want %+v", v.Actions.UpdateScript, want)
		}
	})
	t.Run("outdated", func(t *testing.T) {
		t.Parallel()
		h := hostHelperHarness(t, helperState(t, func(m fstest.MapFS) {
			delete(m, helperUpdateScript)
			delete(m, helperCheckUnit)
		}), fstest.MapFS{})
		// The reason both buttons share comes before the check's own.
		refusedBoth(t, h, code)
	})
	t.Run("busy", func(t *testing.T) {
		t.Parallel()
		started := "c0ffee00c0ffee11 check-update started " + time.Now().UTC().Add(-5*time.Second).Truncate(time.Second).Format(time.RFC3339)
		h := hostHelperHarnessUp(t, helperState(t, func(m fstest.MapFS) {
			delete(m, helperUpdateScript)
			helperRecorded(started)(m)
		}), fstest.MapFS{}, func() (time.Duration, error) { return time.Hour, nil })
		refusedBoth(t, h, code)
		// The three others are refused as busy.
		tok := sessionHostToken(t, h.confirmer, h.srv.URL, h.client, "host.reboot")
		resp, raw := h.do(t, http.MethodPost, "/api/v1/host/actions/reboot", map[string]string{"confirmation": tok})
		if resp.StatusCode != http.StatusConflict {
			t.Errorf("POST reboot: %d, want 409 conflict.host-helper-busy (%s)", resp.StatusCode, raw)
		} else if p := decodeProblem(t, resp, raw); p.Code != "conflict.host-helper-busy" {
			t.Errorf("POST reboot: code %q, want conflict.host-helper-busy", p.Code)
		}
		requireNothingPlaced(t, h, "reboot")
	})

	// The page turns off exactly the buttons the routes refuse: its list of
	// the actions that need the script is NeedsUpdateScript's.
	src, err := os.ReadFile("../../web/src/components/HostActions.tsx")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^export const UPDATE_SCRIPT_ACTIONS: readonly HostAction\[\] = \[([^\]]*)\]$`).FindAllStringSubmatch(string(src), -1)
	if len(m) != 1 {
		t.Fatalf("web/src/components/HostActions.tsx declares UPDATE_SCRIPT_ACTIONS %d times on one line, want once", len(m))
	}
	var page []string
	for _, w := range strings.Split(m[0][1], ",") {
		page = append(page, strings.Trim(strings.TrimSpace(w), "'"))
	}
	var server []string
	for _, a := range hostaction.Actions() {
		if hostaction.NeedsUpdateScript(a) {
			server = append(server, string(a))
		}
	}
	slices.Sort(page)
	slices.Sort(server)
	if !reflect.DeepEqual(page, server) {
		t.Errorf("the page's UPDATE_SCRIPT_ACTIONS is %q, hostaction.NeedsUpdateScript names %q; the page and the routes disagree", page, server)
	}
}
