package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

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
