package auth

import (
	"testing"
	"time"
)

// SetClockForTest replaces the clock the service reads the session limit and
// the sudo window by, for the tests of other packages that drive the routes
// over a real Service. Within this package the tests set s.now directly.
//
// It exists for a test harness on a machine whose wall clock moves under it --
// a laptop that sleeps in the middle of a suite, a time step -- where a window
// opened a few seconds ago by the test's reckoning reads as long closed.
//
// It refuses outside a test binary: testing.Testing is false in every binary
// go build makes, so the daemon cannot reach it, and the service keeps the
// clock New gave it.
func (s *Service) SetClockForTest(now func() time.Time) {
	if !testing.Testing() {
		panic("auth: SetClockForTest outside a test binary")
	}
	s.now = now
}
