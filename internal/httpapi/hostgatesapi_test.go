package httpapi_test

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/holzcloud/holzkube-manager/internal/host"
	"github.com/holzcloud/holzkube-manager/internal/host/hostaction"
	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
)

// The locks in front of the four host actions (Phase 13, D-07, D-08, D-10).
//
// Every one of them is a declarative flag on a route or a single call in a
// handler, and a flag that silently flips passes every test that only walks
// the happy path. So each lock is asserted here on all four routes, from the
// outside, the way a browser meets it -- and each was removed once, on
// purpose, and this file seen red (13-03-SUMMARY lists the injections).
//
// The fifth lock D-10 names is not here: a session route without a role
// cannot be registered at all (TestARouteWithNoRoleCannotBeRegistered).

// exampleHostname is what the stand-in host answers uname with, and so what
// the operator has to type.
const exampleHostname = "example-host"

// A machine id of the shape the inventory uses: a UUID, which a node token
// names and a host token never does.
const exampleMachineID = "3f0c6a2e-8d1b-4c57-9a6e-2b7f1d0e4c93"

// hostTarget is the Machine of every host token: handlers' hostIntentTarget,
// which is unexported and repeated here on purpose -- a change to it has to
// show up as a change to this file.
const hostTarget = "@host"

// The accounts the gates are tried with, beside the admin setupAndLogin makes.
const (
	gateOperator = "operator-account"
	gateReader   = "reader-account"
)

// hostGates is a harness with host actions over an installed helper, and the
// signed-in accounts the locks are tried with.
type hostGates struct {
	*harness

	// operator is an operator-role account with its sudo window open: the
	// least that may place a host order.
	operator *asClient
	// shut is the same operator account in a second session, whose window
	// has not been opened.
	shut *asClient
	// reader is a reader-role account; its window is open too, so that the
	// role is the only thing that stands in its way.
	reader *asClient
}

// newHostGates builds the harness. The Box reads through installedHelperFS --
// the fixture plan 01 introduced -- so that a check for an installed helper
// finds one, and these tests keep testing the locks, not the helper.
func newHostGates(t *testing.T, sys host.Sys) *hostGates {
	t.Helper()

	fsys, _ := installedHelperFS(t)
	h := newHarness(t,
		withHost(host.New(host.Config{FS: fstest.MapFS{}, Sys: sys})),
		withHostActions(func(dataDir string) *hostaction.Box {
			return hostaction.NewBox(hostaction.Config{
				FS:      fsys,
				DataDir: dataDir,
				Place:   fsstore.PlaceNew,
			})
		}),
	)
	h.setupAndLogin(t)
	if resp, raw := h.do(t, http.MethodPost, "/api/v1/auth/sudo",
		map[string]string{"password": testPass}); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("sudo: %d (%s)", resp.StatusCode, raw)
	}

	for _, acct := range []struct {
		name string
		role model.UserRole
	}{{gateOperator, model.RoleOperator}, {gateReader, model.RoleReader}} {
		if resp, raw := h.do(t, http.MethodPost, "/api/v1/users", map[string]string{
			"username": acct.name, "password": newAccountPass, "role": string(acct.role),
		}); resp.StatusCode != http.StatusCreated {
			t.Fatalf("creating %s: %d (%s)", acct.name, resp.StatusCode, raw)
		}
	}

	g := &hostGates{
		harness:  h,
		operator: h.asUser(t, gateOperator, newAccountPass),
		shut:     h.asUser(t, gateOperator, newAccountPass),
		reader:   h.asUser(t, gateReader, newAccountPass),
	}
	for _, c := range []*asClient{g.operator, g.reader} {
		if got, raw := c.status(t, http.MethodPost, "/api/v1/auth/sudo",
			map[string]string{"password": newAccountPass}); got != http.StatusNoContent {
			t.Fatalf("sudo: %d (%s)", got, raw)
		}
	}
	return g
}

// orderPath is where a placed order lies.
func (g *hostGates) orderPath() string {
	return filepath.Join(g.dataDir, hostaction.OrderFileName)
}

// requireNoOrder fails when anything lies in the order slot: a refused request
// must have placed nothing, whatever it answered. What it finds it removes, so
// that one leaked order is reported where it was placed and not again by every
// later request that meets a taken slot.
func (g *hostGates) requireNoOrder(t *testing.T, what string) {
	t.Helper()
	if _, err := os.Lstat(g.orderPath()); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("%s: the order slot is not empty (lstat: %v); a refused request placed an order", what, err)
		g.clearOrder(t)
	}
}

// clearOrder empties the slot, as the helper would after reading it.
func (g *hostGates) clearOrder(t *testing.T) {
	t.Helper()
	if err := os.Remove(g.orderPath()); err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
}

// hostToken asks the host confirm route, as c, for a token for a, typing the
// hostname.
func hostToken(t *testing.T, c *asClient, a hostaction.Action) string {
	t.Helper()
	got, raw := c.status(t, http.MethodPost, "/api/v1/host/confirm", map[string]string{
		"action": "host." + string(a), "typed": exampleHostname,
	})
	if got != http.StatusOK {
		t.Fatalf("confirm host.%s: %d, want 200 (%s)", a, got, raw)
	}
	var tok struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(raw, &tok); err != nil || tok.Token == "" {
		t.Fatalf("confirm answer %s: %v", raw, err)
	}
	return tok.Token
}

// problemCode is the code of a problem answer, or "" when there is none.
func problemCode(raw []byte) string {
	var p struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(raw, &p)
	return p.Code
}

func actionPath(a hostaction.Action) string { return "/api/v1/host/actions/" + string(a) }

func TestHostActionGates(t *testing.T) {
	t.Parallel()

	g := newHostGates(t, hostSys{uname: host.Uname{Nodename: exampleHostname, Machine: "aarch64"}})

	// The subtests share one data directory and one order slot, so they run
	// one after the other.

	t.Run("sudo window", func(t *testing.T) {
		for _, a := range hostaction.Actions() {
			// The token comes from the real confirm route, which is not
			// destructive: a shut window is asked for at the action, not
			// before.
			tok := hostToken(t, g.shut, a)
			got, raw := g.shut.status(t, http.MethodPost, actionPath(a), map[string]string{"confirmation": tok})
			if got != http.StatusPreconditionRequired || problemCode(raw) != "sudo.required" {
				t.Errorf("%s with the sudo window shut: %d %s, want 428 sudo.required (%s)",
					a, got, problemCode(raw), raw)
			}
			g.requireNoOrder(t, string(a)+" with the sudo window shut")
		}
	})

	t.Run("reader", func(t *testing.T) {
		got, raw := g.reader.status(t, http.MethodPost, "/api/v1/host/confirm", map[string]string{
			"action": "host.reboot", "typed": exampleHostname,
		})
		if got != http.StatusForbidden || problemCode(raw) != httpapi.CodeForbiddenRole {
			t.Errorf("reader on the host confirm route: %d %s, want 403 %s (%s)",
				got, problemCode(raw), httpapi.CodeForbiddenRole, raw)
		}
		for _, a := range hostaction.Actions() {
			// A valid token and an open window: nothing but the role is
			// missing.
			tok, _ := g.confirmer.Issue(jobs.Intent{Action: "host." + string(a), Machine: hostTarget})
			got, raw := g.reader.status(t, http.MethodPost, actionPath(a), map[string]string{"confirmation": tok})
			if got != http.StatusForbidden || problemCode(raw) != httpapi.CodeForbiddenRole {
				t.Errorf("reader on %s: %d %s, want 403 %s (%s)",
					a, got, problemCode(raw), httpapi.CodeForbiddenRole, raw)
			}
			g.requireNoOrder(t, "reader on "+string(a))
		}
	})

	t.Run("no confirmation", func(t *testing.T) {
		for _, a := range hostaction.Actions() {
			for name, body := range map[string]any{
				"no body field": map[string]string{},
				"an empty one":  map[string]string{"confirmation": ""},
				"a made-up one": map[string]string{"confirmation": "1700000000.bm90LWEtdG9rZW4"},
			} {
				got, raw := g.operator.status(t, http.MethodPost, actionPath(a), body)
				if got != http.StatusForbidden || problemCode(raw) != httpapi.CodeConfirmationInvalid {
					t.Errorf("%s with %s: %d %s, want 403 %s (%s)",
						a, name, got, problemCode(raw), httpapi.CodeConfirmationInvalid, raw)
				}
				g.requireNoOrder(t, string(a)+" with "+name)
			}
		}
	})

	t.Run("another action's token", func(t *testing.T) {
		acts := hostaction.Actions()
		for i, a := range acts {
			other := acts[(i+1)%len(acts)]
			tok := hostToken(t, g.operator, other)
			got, raw := g.operator.status(t, http.MethodPost, actionPath(a), map[string]string{"confirmation": tok})
			if got != http.StatusForbidden || problemCode(raw) != httpapi.CodeConfirmationInvalid {
				t.Errorf("%s with a host.%s token: %d %s, want 403 %s (%s)",
					a, other, got, problemCode(raw), httpapi.CodeConfirmationInvalid, raw)
			}
			g.requireNoOrder(t, string(a)+" with host."+string(other)+"'s token")
		}
	})

	t.Run("node token", func(t *testing.T) {
		// A node reboot token names a machine id. The node and host actions
		// share one confirmer, so only the intent keeps them apart.
		tok, _ := g.confirmer.Issue(jobs.Intent{Action: "node.reboot", Machine: exampleMachineID})
		for _, a := range hostaction.Actions() {
			got, raw := g.operator.status(t, http.MethodPost, actionPath(a), map[string]string{"confirmation": tok})
			if got != http.StatusForbidden || problemCode(raw) != httpapi.CodeConfirmationInvalid {
				t.Errorf("%s with a node.reboot token: %d %s, want 403 %s (%s)",
					a, got, problemCode(raw), httpapi.CodeConfirmationInvalid, raw)
			}
			g.requireNoOrder(t, string(a)+" with a node.reboot token")
		}
	})

	t.Run("host token bound to a machine", func(t *testing.T) {
		// The right action, bound to a machine id rather than to the host:
		// the action route rebuilds the target itself, so this token cannot
		// match -- neither alone, nor with the machine it names offered in
		// the body, where a handler that took its target from the request
		// would find it.
		for _, a := range hostaction.Actions() {
			tok, _ := g.confirmer.Issue(jobs.Intent{Action: "host." + string(a), Machine: exampleMachineID})

			got, raw := g.operator.status(t, http.MethodPost, actionPath(a), map[string]string{"confirmation": tok})
			if got != http.StatusForbidden || problemCode(raw) != httpapi.CodeConfirmationInvalid {
				t.Errorf("%s with a host token bound to a machine: %d %s, want 403 %s (%s)",
					a, got, problemCode(raw), httpapi.CodeConfirmationInvalid, raw)
			}
			g.requireNoOrder(t, string(a)+" with a host token bound to a machine")

			got, raw = g.operator.status(t, http.MethodPost, actionPath(a), map[string]string{
				"confirmation": tok, "machine": exampleMachineID,
			})
			if got < 400 {
				t.Errorf("%s with a host token bound to a machine and that machine in the body: %d, want a refusal (%s)",
					a, got, raw)
			}
			g.requireNoOrder(t, string(a)+" with a host token bound to a machine and that machine in the body")
		}
	})

	t.Run("operator places each action", func(t *testing.T) {
		// HACT-01..04 at the route: the least role, an open window and the
		// matching token open each of the four.
		for _, a := range hostaction.Actions() {
			tok := hostToken(t, g.operator, a)
			got, raw := g.operator.status(t, http.MethodPost, actionPath(a), map[string]string{"confirmation": tok})
			if got != http.StatusAccepted {
				t.Fatalf("%s by an operator with sudo and a token: %d, want 202 (%s)", a, got, raw)
			}
			order, err := os.ReadFile(g.orderPath())
			if err != nil {
				t.Fatalf("%s: the order after a 202: %v", a, err)
			}
			if !strings.HasPrefix(string(order), string(a)+" ") {
				t.Errorf("%s: order file %q does not name the action", a, order)
			}
			g.clearOrder(t)
		}
	})
}
