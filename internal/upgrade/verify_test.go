package upgrade_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/upgrade"
)

// TestEveryNotUpgradedVerdictNamesTheRemedy is the other half of UPG-07.
//
// The requirement is that "the API said OK" is not accepted as proof, and the
// verdicts it produces are precise about what went wrong: the version is right
// and the image is not, these services are not running, it reports something
// that is not a version at all. Every one of them hands the operator a broken
// node and, until this test existed, nothing to do about it.
//
// Talos has an answer -- the previous installation is on the other boot
// partition -- and this product does not implement it. That combination is the
// worst one to leave unsaid, because somebody reading "the upgrade did not
// take" reasonably concludes there is nothing left to try.
func TestEveryNotUpgradedVerdictNamesTheRemedy(t *testing.T) {
	t.Parallel()

	want, err := upgrade.ParseVersion("v1.13.9")
	if err != nil {
		t.Fatalf("ParseVersion: %v", err)
	}

	rows := []struct {
		name     string
		observed upgrade.Observed
	}{
		{
			name:     "a version that did not change",
			observed: upgrade.Observed{TalosVersion: "v1.13.8", Healthy: true},
		},
		{
			name:     "something that is not a version at all",
			observed: upgrade.Observed{TalosVersion: "not-a-version", Healthy: true},
		},
		{
			name: "the right version with the wrong image",
			observed: upgrade.Observed{
				TalosVersion: "v1.13.9", SchematicID: "0123456789abcdef", Healthy: true,
			},
		},
		{
			name: "the right version with services down",
			observed: upgrade.Observed{
				TalosVersion: "v1.13.9", Healthy: false, Unhealthy: []string{"kubelet"},
			},
		},
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()

			err := upgrade.VerdictFor(row.observed, want, "fedcba9876543210")
			if err == nil {
				t.Fatal("this observation was accepted, so the test says nothing about its verdict")
			}
			if !strings.Contains(err.Error(), "talosctl rollback") {
				t.Errorf("the verdict does not name the one recovery Talos offers:\n  %v", err)
			}
			if !strings.Contains(err.Error(), "does not undo an upgrade") {
				t.Errorf("the verdict does not say this product will not do it, which is the half "+
					"that stops somebody waiting for a button:\n  %v", err)
			}
		})
	}
}

// TestTheRollbackNoticeDoesNotInventACommandLine keeps the notice honest about
// what it knows.
//
// machinery's API says the operation exists -- MachineService.Rollback is in
// the descriptor and this product does not call it. The exact talosctl
// invocation is Talos' documentation's to state, and a flag spelled wrongly
// here would be worse than no flag on the one screen where somebody is going
// to paste it.
func TestTheRollbackNoticeDoesNotInventACommandLine(t *testing.T) {
	t.Parallel()

	// A flag is two dashes against a letter. The house em-dash is two dashes
	// against a space, and the first version of this check did not tell them
	// apart -- it failed on the notice's own prose.
	if flagLike.MatchString(upgrade.RollbackNotice) {
		t.Errorf("the notice carries a command-line flag, which nothing in this repository can "+
			"verify against a real Talos:\n  %s", upgrade.RollbackNotice)
	}
	if !strings.Contains(upgrade.RollbackNotice, "documentation") {
		t.Error("the notice does not point anywhere for the exact invocation")
	}
}

// flagLike matches a command-line flag and not the house em-dash.
var flagLike = regexp.MustCompile(`--[A-Za-z]`)
