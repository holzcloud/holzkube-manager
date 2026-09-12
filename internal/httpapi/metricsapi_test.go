package httpapi_test

import (
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// TestMetricsIsScrapableWithoutASession is criterion 1 of v1.15 phase 3.
//
// Prometheus has no way to log in. It sends a bare GET on a timer, so a metrics
// endpoint behind the session cookie is one nobody can scrape -- and the usual
// consequence is a second, entirely unauthenticated listener on another port,
// which is strictly worse than this. What guards this route is what guards
// every route: the host allowlist in the outer chain, and a bind address that
// is loopback unless the operator said otherwise.
func TestMetricsIsScrapableWithoutASession(t *testing.T) {
	sim, err := talossim.New(talossim.Options{Hostname: "metrics-1", ControlPlane: true})
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}
	t.Cleanup(func() { _ = sim.Close() })

	h := newHarness(t, withInventory(func(st *fsstore.Store) *inventory.Service {
		return inventory.New(inventory.Deps{
			Store:  st,
			Dialer: talos.NewDirectDialer(sim.Port()),
			Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		})
	}))

	// No setup, no login: nobody has ever authenticated against this instance.
	resp, raw := h.do(t, http.MethodGet, "/metrics", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /metrics without a session = %d (%s); a scraper cannot produce a cookie",
			resp.StatusCode, raw)
	}

	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want the Prometheus text exposition type", ct)
	}
	if !strings.Contains(ct(resp), "version=0.0.4") {
		t.Errorf("Content-Type = %q carries no format version; a scraper then has to guess how to "+
			"parse the body", ct(resp))
	}

	body := string(raw)
	for _, want := range []string{
		"# HELP holzkube_machines ",
		"# TYPE holzkube_machines gauge",
		"holzkube_audit_chain_intact ",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the exposition does not contain %q:\n%s", want, body)
		}
	}

	// And it is genuinely outside the versioned API, because /metrics is where
	// every scraper looks by default and a path this product invented would
	// have to be configured by hand everywhere.
	if strings.Contains(body, "problem") {
		t.Errorf("the metrics body looks like a problem document:\n%s", body)
	}
}

// TestMetricsIsRefusedOnAHostThisInstanceDoesNotAnswerTo is the other half of
// criterion 1: no session does not mean no guard.
//
// The host allowlist is what closes DNS rebinding, and it sits in the outer
// chain so that it covers a route whose own middleware asks for nothing. A
// metrics endpoint that skipped it would be the one route on this listener a
// rebound browser could read -- and it publishes the shape of the fleet.
func TestMetricsIsRefusedOnAHostThisInstanceDoesNotAnswerTo(t *testing.T) {
	sim, err := talossim.New(talossim.Options{Hostname: "metrics-2", ControlPlane: true})
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}
	t.Cleanup(func() { _ = sim.Close() })

	h := newHarness(t,
		withInventory(func(st *fsstore.Store) *inventory.Service {
			return inventory.New(inventory.Deps{
				Store:  st,
				Dialer: talos.NewDirectDialer(sim.Port()),
				Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
			})
		}),
		withAllowedHosts("holzkube.example"),
	)

	resp, raw := h.do(t, http.MethodGet, "/metrics", nil, func(r *http.Request) {
		r.Host = "attacker.example"
	})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("GET /metrics with a foreign Host = %d (%s), want 403; the allowlist is what "+
			"keeps a rebound browser from reading the shape of the fleet", resp.StatusCode, raw)
	}

	// And the route that is on the allowlist still answers, so the test above
	// is not passing because everything is refused.
	resp, raw = h.do(t, http.MethodGet, "/metrics", nil, func(r *http.Request) {
		r.Host = "holzkube.example"
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /metrics on an allowed Host = %d (%s)", resp.StatusCode, raw)
	}
}

func ct(resp *http.Response) string { return resp.Header.Get("Content-Type") }
