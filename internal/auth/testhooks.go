package auth

import (
	"testing"
	"time"

	"github.com/alexedwards/argon2id"
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

// UseParamsForTest sets the parameters new hashes are written with, in place
// of the calibrated ones, for the tests of other packages that sign in through
// the routes hundreds of times. Within this package the tests store
// activeParams directly (useTestParams).
//
// It is the one writer of activeParams besides the calibration, and it is
// what "production code has no way to lower the cost" has to hold against: it
// refuses outside a test binary -- testing.Testing is false in every binary go
// build makes -- so the daemon's hashes are written with the calibrated cost
// and nothing else. It must run before the first hash: auth.New calibrates if
// nothing is set yet, and a hash written before the swap keeps its own cost.
func UseParamsForTest(p *argon2id.Params) {
	if !testing.Testing() {
		panic("auth: UseParamsForTest outside a test binary")
	}
	cp := *p
	activeParams.Store(&cp)
}
