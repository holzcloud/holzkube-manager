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
		Outdated        []hostaction.Missing `json:"outdated"`
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
	if v.Actions.Outdated == nil {
		t.Error("actions.outdated is null or absent; the page reads a list")
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
		if !*v.Actions.Available || len(v.Actions.Missing) != 0 || len(v.Actions.Outdated) != 0 {
			t.Errorf("installed: actions = available %v, missing %+v, outdated %+v; want available, nothing missing or outdated",
				*v.Actions.Available, v.Actions.Missing, v.Actions.Outdated)
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

// TestHostActionsWaitForARunningCheck (13-REVIEW-2 WR-01): the helper carries
// out a check blocking -- it waits for the check unit, up to its own service's
// limit -- so while one runs it picks up nothing else. An order placed then
// would lie in the slot until the Box withdrew it after 10 s, with a sentence
// blaming the path unit. So while the helper's last record is a check it has
// started, younger than that limit, and the update status is not yet newer
// than it, both routes refuse every host action with 409
// conflict.host-helper-busy: no token, nothing placed. The page turns the
// buttons off by the same two readings.
//
// The refusals keep their order: container, missing, outdated, busy.
func TestHostActionsWaitForARunningCheck(t *testing.T) {
	t.Parallel()

	// Every time is taken inside the subtest, just before its harness is
	// built: the subtests run in parallel with the whole package, and a
	// "5 s ago" taken when the test began can be minutes old by then.
	const id = "c0ffee00c0ffee11"
	stamp := func(at time.Time) string { return at.UTC().Truncate(time.Second).Format(time.RFC3339) }
	checkStarted := func(ago time.Duration) func(now time.Time) func(m fstest.MapFS) {
		return func(now time.Time) func(m fstest.MapFS) {
			return helperRecorded(id + " check-update started " + stamp(now.Add(-ago)))
		}
	}
	recorded := func(action, outcome string, ago time.Duration) func(now time.Time) func(m fstest.MapFS) {
		return func(now time.Time) func(m fstest.MapFS) {
			return helperRecorded(id + " " + action + " " + outcome + " " + stamp(now.Add(-ago)))
		}
	}
	noStatus := func(time.Time) fstest.MapFS { return fstest.MapFS{} }
	statusAgo := func(ago time.Duration) func(now time.Time) fstest.MapFS {
		return func(now time.Time) fstest.MapFS { return updateRecorded(now.Add(-ago)) }
	}
	type fixture struct {
		name   string
		edit   func(now time.Time) func(m fstest.MapFS)
		status func(now time.Time) fstest.MapFS
	}
	build := func(t *testing.T, f fixture) *harness {
		t.Helper()
		now := time.Now()
		var edit func(m fstest.MapFS)
		if f.edit != nil {
			edit = f.edit(now)
		}
		return hostHelperHarness(t, helperState(t, edit), f.status(now))
	}

	busy := []fixture{
		{"a check started 5 s ago, no update status yet", checkStarted(5 * time.Second), noStatus},
		{"a check started 5 s ago, the update status from an hour before", checkStarted(5 * time.Second), statusAgo(time.Hour)},
		// The check unit may take 2 min and its stop 15 s: still the helper's.
		{"a check started 2 min 15 s ago", checkStarted(135 * time.Second), noStatus},
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

			// Available stays true: the helper is installed; the page reads
			// the running check from the result and the update status.
			if v := readHelperView(t, h); !*v.Actions.Available {
				t.Error("actions.available = false while a check runs; the helper is installed")
			}
		})
	}

	// The controls: the same fixture with the check over, or never a check,
	// goes through -- so the refusals above are the running check's.
	through := []fixture{
		{"the check's answer arrived", checkStarted(5 * time.Second), statusAgo(0)},
		{"the answer in the second the check started", checkStarted(5 * time.Second), statusAgo(5 * time.Second)},
		{"a check started 4 min ago, past the helper's 3-min limit", checkStarted(4 * time.Minute), noStatus},
		{"a check that failed", recorded("check-update", "failed", 5*time.Second), noStatus},
		{"an update started", recorded("update", "started", 5*time.Second), noStatus},
		{"a check recorded in the future (the clock went back)", checkStarted(-time.Hour), noStatus},
		{"nothing recorded", nil, noStatus},
	}
	for _, tc := range through {
		t.Run("goes through: "+tc.name, func(t *testing.T) {
			t.Parallel()
			requireGoesThrough(t, build(t, tc), hostaction.Reboot)
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
}
