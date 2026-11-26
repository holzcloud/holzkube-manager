package handlers

import (
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/auth"
)

// UseLimiterClock makes every AuthRoutes built from now until the end of t use
// a password limiter reading now. A test calling it must not run in parallel.
func UseLimiterClock(t *testing.T, now func() time.Time) {
	t.Helper()
	prev := newLimiter
	newLimiter = func() *auth.Limiter { return auth.NewLimiterWithClock(now) }
	t.Cleanup(func() { newLimiter = prev })
}
