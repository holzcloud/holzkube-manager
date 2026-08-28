package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

type sudoProbe struct {
	handlerCalls int
	denyCalls    int
	touchCalls   int
	touchedAfter int // handlerCalls at the moment touch ran
	status       int
}

func (p *sudoProbe) handler(status int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		p.handlerCalls++
		w.WriteHeader(status)
	})
}

func (p *sudoProbe) deny() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		p.denyCalls++
		w.WriteHeader(http.StatusPreconditionRequired)
	}
}

func (p *sudoProbe) touch() func(*http.Request) {
	return func(*http.Request) {
		p.touchCalls++
		p.touchedAfter = p.handlerCalls
	}
}

func (p *sudoProbe) run(t *testing.T, destructive, open bool, handlerStatus int) {
	t.Helper()
	mw := Sudo(destructive,
		func(*http.Request) bool { return open },
		p.touch(),
		p.deny())

	rec := httptest.NewRecorder()
	mw(p.handler(handlerStatus)).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/account/password", nil))
	p.status = rec.Code
}

// TestSudoGateRefusesADestructiveRouteWithNoWindow is the whole point of the
// link: the handler must not run at all, not merely have its effect undone.
func TestSudoGateRefusesADestructiveRouteWithNoWindow(t *testing.T) {
	var p sudoProbe
	p.run(t, true, false, http.StatusNoContent)

	if p.handlerCalls != 0 {
		t.Errorf("handler ran %d times behind a closed window", p.handlerCalls)
	}
	if p.denyCalls != 1 {
		t.Errorf("deny called %d times, want 1", p.denyCalls)
	}
	if p.touchCalls != 0 {
		t.Errorf("a refused request refreshed the window %d times", p.touchCalls)
	}
	if p.status != http.StatusPreconditionRequired {
		t.Errorf("status = %d, want 428", p.status)
	}
}

// TestSudoGateRestartsTheWindowAfterTheHandler pins the ordering D-05 asks for:
// the refresh follows the action, so a series of them stays inside one window.
func TestSudoGateRestartsTheWindowAfterTheHandler(t *testing.T) {
	var p sudoProbe
	p.run(t, true, true, http.StatusNoContent)

	if p.handlerCalls != 1 {
		t.Errorf("handler ran %d times behind an open window, want 1", p.handlerCalls)
	}
	if p.denyCalls != 0 {
		t.Errorf("deny ran %d times behind an open window", p.denyCalls)
	}
	if p.touchCalls != 1 {
		t.Fatalf("touch ran %d times, want 1", p.touchCalls)
	}
	if p.touchedAfter != 1 {
		t.Error("the window was refreshed before the handler ran, not after it")
	}
}

// TestSudoGateDoesNotRefreshOnAFailedAction keeps a rejected request from
// buying time: only an action that actually happened restarts the window.
func TestSudoGateDoesNotRefreshOnAFailedAction(t *testing.T) {
	var p sudoProbe
	p.run(t, true, true, http.StatusUnauthorized)

	if p.handlerCalls != 1 {
		t.Errorf("handler ran %d times, want 1", p.handlerCalls)
	}
	if p.touchCalls != 0 {
		t.Errorf("a failed action refreshed the window %d times", p.touchCalls)
	}
}

// TestSudoGateIgnoresRoutesThatAreNotDestructive is the other half of D-06: the
// flag decides, so an unmarked mutating route is untouched by this link.
func TestSudoGateIgnoresRoutesThatAreNotDestructive(t *testing.T) {
	var p sudoProbe
	p.run(t, false, false, http.StatusNoContent)

	if p.handlerCalls != 1 {
		t.Errorf("handler ran %d times on a non-destructive route, want 1", p.handlerCalls)
	}
	if p.denyCalls != 0 {
		t.Errorf("a non-destructive route was gated %d times", p.denyCalls)
	}
	if p.touchCalls != 0 {
		t.Errorf("a non-destructive route refreshed the window %d times", p.touchCalls)
	}
}
