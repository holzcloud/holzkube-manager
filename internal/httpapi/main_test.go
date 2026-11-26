package httpapi_test

import (
	"os"
	"testing"

	"github.com/alexedwards/argon2id"

	"github.com/holzcloud/holzkube-manager/internal/auth"
)

// TestMain writes this package's password hashes at a cost of milliseconds
// instead of the calibrated quarter second.
//
// Every harness signs up, signs in and opens the sudo window: three argon2id
// computations of 64 MiB, at most two at a time process-wide (auth's
// hashSlots), over a few hundred parallel tests. Under -race on a loaded
// machine one computation measured 3.5 s, the package took 300 to 500 s and
// once ran into go test's 10-minute limit, and every harness waited its turn
// for the hash while the times it had staged aged by the wall clock. Nothing
// here is a test of the hash's cost; internal/auth holds the calibration and
// the cost to their numbers.
func TestMain(m *testing.M) {
	auth.UseParamsForTest(&argon2id.Params{
		Memory:      8 * 1024,
		Iterations:  1,
		Parallelism: 1,
		SaltLength:  16,
		KeyLength:   32,
	})
	os.Exit(m.Run())
}
