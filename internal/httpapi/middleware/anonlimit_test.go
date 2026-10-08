package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAnonLimitStopsOnePeerAndLeavesOthers(t *testing.T) {
	t.Parallel()

	h := AnonLimit(3, time.Hour, nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	call := func(peer string) int {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		r.RemoteAddr = peer
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec.Code
	}
	for i := range 3 {
		if got := call("203.0.113.9:1000"); got != http.StatusNoContent {
			t.Fatalf("request %d inside the burst got %d", i, got)
		}
	}
	if got := call("203.0.113.9:1001"); got != http.StatusTooManyRequests {
		t.Errorf("the fourth request got %d, want 429", got)
	}
	if got := call("198.51.100.4:1000"); got != http.StatusNoContent {
		t.Errorf("another peer got %d; one scanner must not lock out the operator", got)
	}
}
