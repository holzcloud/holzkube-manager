package httpapi_test

// The role gate at the HTTP boundary (V2-AUTH-02).
//
// The unit tests in internal/auth cover which refusals the domain makes. What
// these cover is the thing a unit test cannot: that the gate is actually wired
// into the chain, on every route, and that a route which forgot to name a role
// cannot be registered at all.

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// TestARouteWithNoRoleCannotBeRegistered is the composition-time half.
//
// It is the property that makes every other route safe without a test of its
// own: a session route with no MinRole does not serve a 403, it stops the
// process before the listener opens.
func TestARouteWithNoRoleCannotBeRegistered(t *testing.T) {
	t.Parallel()

	for name, rt := range map[string]httpapi.Route{
		"a session route with no role": {
			Method:          http.MethodGet,
			Pattern:         "/api/v1/forgot-the-role",
			RequiresSession: true,
			Handler:         http.NotFoundHandler(),
		},
		"a public route that names one": {
			Method:          http.MethodGet,
			Pattern:         "/api/v1/needs-no-session",
			RequiresSession: false,
			MinRole:         model.RoleAdmin,
			Handler:         http.NotFoundHandler(),
		},
		"a role that is not a role": {
			Method:          http.MethodGet,
			Pattern:         "/api/v1/invented-role",
			RequiresSession: true,
			MinRole:         "superuser",
			Handler:         http.NotFoundHandler(),
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			defer func() {
				if recover() == nil {
					t.Error("the route was registered. A permission nobody chose is not a " +
						"permission anybody reviewed, and this is the moment it is cheap to say so")
				}
			}()

			_ = httpapi.New(httpapi.Deps{Routes: []httpapi.Route{rt}})
		})
	}
}

// TestARoleIsEnforcedOnTheWayIn is the runtime half.
//
// The gate is one middleware link and it is easy to wire in the wrong place --
// or to wire and then have something short-circuit before it. These drive a
// real session at each of the three roles against three real routes.
func TestARoleIsEnforcedOnTheWayIn(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.setupAndLogin(t)

	// Creating an account is Destructive -- it changes who can reach cluster
	// PKI -- so the window has to be open first. That is the gate doing its
	// job and not scaffolding: an admin adding an account is asked for their
	// password, which is the whole of D-06.
	if resp, raw := h.do(t, http.MethodPost, "/api/v1/auth/sudo",
		map[string]string{"password": testPass}); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("sudo: %d (%s)", resp.StatusCode, raw)
	}

	// The account setup created is an admin, so it can make the other two.
	for _, role := range []model.UserRole{model.RoleOperator, model.RoleReader} {
		resp, raw := h.do(t, http.MethodPost, "/api/v1/users", map[string]string{
			"username": string(role) + "-account",
			"password": newAccountPass,
			"role":     string(role),
		})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("creating the %s account: %d (%s)", role, resp.StatusCode, raw)
		}
	}

	// The gate's verdict is 403 or not-403, and that is deliberately all these
	// assert. This harness has no inventory service, so an allowed request to
	// /machines answers 502 -- which is the route saying "I got past the gate
	// and had nothing to ask". Asserting 200 would be asserting that the
	// harness is fully wired, and a change there would fail this test for a
	// reason that has nothing to do with roles.
	const forbidden = true

	for _, tc := range []struct {
		role     model.UserRole
		username string
		password string

		// A read every role has, a fleet write only operators and admins have,
		// and an account route only admins have.
		machines, lock, users bool
	}{
		{model.RoleAdmin, testUser, testPass, !forbidden, !forbidden, !forbidden},
		{model.RoleOperator, "operator-account", newAccountPass, !forbidden, !forbidden, forbidden},
		{model.RoleReader, "reader-account", newAccountPass, !forbidden, forbidden, forbidden},
	} {
		t.Run(string(tc.role), func(t *testing.T) {
			c := h.asUser(t, tc.username, tc.password)

			for _, call := range []struct {
				what   string
				method string
				path   string
				body   any
				refuse bool
			}{
				{"read the machines", http.MethodGet, "/api/v1/machines", nil, tc.machines},
				{"lock a node", http.MethodPost, "/api/v1/machines/x/lock",
					map[string]any{"locked": true, "reason": "because"}, tc.lock},
				{"read the accounts", http.MethodGet, "/api/v1/users", nil, tc.users},
			} {
				got, raw := c.status(t, call.method, call.path, call.body)
				is403 := got == http.StatusForbidden

				switch {
				case call.refuse && !is403:
					t.Errorf("a %s could %s: %d (%s)", tc.role, call.what, got, raw)
				case !call.refuse && is403:
					t.Errorf("a %s could not %s: %s", tc.role, call.what, raw)
				}
			}
		})
	}
}

// newAccountPass is what the two accounts this test creates sign in with.
const newAccountPass = "a-long-enough-passphrase"

// setupAndLogin runs the wizard and signs in as the account it creates, which
// is an admin.
func (h *harness) setupAndLogin(t *testing.T) {
	t.Helper()

	resp, raw := h.do(t, http.MethodPost, "/api/v1/setup", map[string]string{
		"username": testUser, "password": testPass,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("setup: %d (%s)", resp.StatusCode, raw)
	}
	resp, raw = h.do(t, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"username": testUser, "password": testPass,
	})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("login: %d (%s)", resp.StatusCode, raw)
	}
}

// asClient is a second signed-in browser against the same server.
//
// It has its own cookie jar, because the harness's client is already signed in
// as the admin and a shared jar would make every request below that admin's --
// which is a test that passes for the wrong reason.
type asClient struct {
	srv    string
	client *http.Client
}

func (h *harness) asUser(t *testing.T, username, password string) *asClient {
	t.Helper()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	c := &asClient{
		srv: h.srv.URL,
		client: &http.Client{
			Jar: jar,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // test server uses an ephemeral cert
			},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}

	if got, raw := c.status(t, http.MethodPost, "/api/v1/auth/login",
		map[string]string{"username": username, "password": password}); got != http.StatusNoContent {
		t.Fatalf("login as %s: %d (%s)", username, got, raw)
	}
	return c
}

func (c *asClient) status(t *testing.T, method, path string, body any) (int, []byte) {
	t.Helper()

	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rdr = bytes.NewReader(raw)
	}

	req, err := http.NewRequest(method, c.srv+path, rdr)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-Holzkube-Manager-CSRF", "1")

	resp, err := c.client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, raw
}
