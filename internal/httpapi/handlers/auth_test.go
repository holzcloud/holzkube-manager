package handlers_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube/internal/talos"
)

// TestMeReportsTheDryRunMode is FOUND-12's API half.
//
// The field lives on the authenticated identity endpoint rather than on
// GET /api/v1/system/status, which is the endpoint 02-PATTERNS.md recommends
// and the one Phase 1 explicitly declined to extend: that endpoint answers
// before authentication, and whether this instance can currently change
// anything is not something an anonymous caller needs to be told. The banner
// this field feeds is only ever rendered for a signed-in operator, so asking
// here costs nothing and discloses nothing.
func TestMeReportsTheDryRunMode(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		mode talos.Mode
		want bool
	}{
		{name: "live", mode: talos.Mode{}, want: false},
		{name: "dry run", mode: talos.Mode{DryRun: true}, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := newServerInMode(t, tc.mode)
			c := s.newClient(t)
			c.setup()

			resp, body := c.do(http.MethodGet, "/api/v1/auth/me", nil)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET /api/v1/auth/me = %d, body %s", resp.StatusCode, body)
			}

			var got struct {
				Username string `json:"username"`
				DryRun   bool   `json:"dry_run"`
			}
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("decode %s: %v", body, err)
			}
			if got.Username == "" {
				t.Errorf("the existing identity fields went missing: %s", body)
			}
			if got.DryRun != tc.want {
				t.Errorf("dry_run = %v, want %v (body %s)", got.DryRun, tc.want, body)
			}
		})
	}
}

// TestSetupRefusesTheReservedActorUsername is T-02-47.
//
// `system` is a reserved audit actor, `actor` is part of the hash chain, and
// the archive is never shortened (D-16). An operator account by that name would
// make every record it writes permanently ambiguous between the process and a
// person, so the collision is closed before the account can exist.
func TestSetupRefusesTheReservedActorUsername(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"system", "System", "SYSTEM"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			s := newServer(t, 5*time.Minute)
			c := s.newClient(t)

			resp, body := c.do(http.MethodPost, "/api/v1/setup", map[string]string{
				"username": name,
				"password": "correct-horse-battery-staple",
			})
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("setup as %q = %d, want 400 (body %s)", name, resp.StatusCode, body)
			}
			if !strings.Contains(string(body), "reserved") {
				t.Errorf("the refusal does not say the name is reserved: %s", body)
			}
		})
	}

	// And the reservation is narrow: a name that merely contains it is fine.
	s := newServer(t, 5*time.Minute)
	c := s.newClient(t)
	resp, body := c.do(http.MethodPost, "/api/v1/setup", map[string]string{
		"username": "systems-admin",
		"password": "correct-horse-battery-staple",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("setup as %q = %d, want 201 (body %s)", "systems-admin", resp.StatusCode, body)
	}
}
