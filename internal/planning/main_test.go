package planning_test

import (
	"fmt"
	"os"
	"testing"
)

// TestMain skips the whole package when the planning directory is not there.
//
// Every test in this package reads .planning/, the ledger and the requirements,
// and since 2026-10-30 (#21) that directory is no longer under version control:
// a fresh clone, and therefore CI, does not have it. Before this, the package
// failed on every such checkout with "no such file or directory", which made
// the release gate red for a reason that says nothing about the code.
//
// On the operator's machine and in any session working on the planning files
// the directory exists and the tests run as before.
func TestMain(m *testing.M) {
	if _, err := os.Stat("../../.planning"); err != nil {
		fmt.Println("internal/planning: .planning/ is not in this checkout (it is not versioned); nothing to check")
		os.Exit(0)
	}
	os.Exit(m.Run())
}
