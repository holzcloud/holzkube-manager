package imagefactory_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTheLiveDriftTestsAreScheduled is ledger entries 5, 30, 35 and 64, which
// all say one thing in four places: the only guards this repository has against
// factory.talos.dev moving underneath its recordings are opt-in, and nothing
// scheduled them.
//
// A schedule in a workflow file is easy to lose -- a rename, a refactor of the
// CI, a merge that drops a file nobody imports -- and losing it is silent,
// because a watcher that does not run produces no output at all. That is the
// same failure the entries describe, so the schedule gets a test, and the test
// asserts the three things a run needs to mean anything: the file exists, it
// has a cron trigger, and it turns the live tests on by name.
func TestTheLiveDriftTestsAreScheduled(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", ".github", "workflows", "factory-drift.yml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the Image Factory drift schedule is gone (%v). The live tests are opt-in, so "+
			"without it nothing in this repository ever asks whether the recorded fixtures still "+
			"match factory.talos.dev -- which is ledger entries 5, 30, 35 and 64.", err)
	}
	workflow := string(raw)

	if !strings.Contains(workflow, "schedule:") || !strings.Contains(workflow, "cron:") {
		t.Error("the drift workflow has no schedule, so it only runs when somebody remembers it -- " +
			"which is the state the ledger entries describe")
	}

	// The variable name, not a description of it: this is the switch that
	// decides whether the live tests run at all, and a workflow that set a
	// misspelled one would skip every test and pass.
	if !strings.Contains(workflow, liveEnv) {
		t.Errorf("the drift workflow does not set %s, so the live tests skip themselves and the "+
			"run is green without having asked the Factory anything", liveEnv)
	}

	for _, name := range []string{"TestLiveFactory", "TestLiveCanonical"} {
		if !strings.Contains(workflow, name) {
			t.Errorf("the drift workflow does not run %s", name)
		}
	}
}
