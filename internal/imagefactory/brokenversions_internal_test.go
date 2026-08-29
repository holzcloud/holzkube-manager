package imagefactory

import (
	"strings"
	"testing"
)

// TestBrokenReasonFindsAListedVersion exercises the lookup against a synthetic
// table.
//
// It is an internal test because the shipped table is empty -- see the doc
// comment on brokenVersions -- and a mechanism that is only ever called with an
// empty table is a mechanism nothing proves. The synthetic table is the same
// shape as the real one, so the day an entry is added the lookup is already
// known to work.
func TestBrokenReasonFindsAListedVersion(t *testing.T) {
	table := map[string]string{
		"v1.0.0": "example: the reason this version is listed",
	}

	reason, broken := brokenReason(table, "v1.0.0")
	if !broken {
		t.Fatal("a listed version was not reported as broken")
	}
	if reason != table["v1.0.0"] {
		t.Errorf("reason = %q, want %q", reason, table["v1.0.0"])
	}

	if reason, broken := brokenReason(table, "v1.0.1"); broken || reason != "" {
		t.Errorf("an unlisted version returned (%q, %v)", reason, broken)
	}
}

// TestBrokenReasonEveryEntryCarriesAReason is the guard that gives the table
// its whole value. D-08 keeps this list in the binary rather than in a UI
// because operational knowledge that lives in one installation is lost; what
// makes it reviewable in git is the reason beside each entry, and an entry
// without one is a version greyed out for no stated cause.
//
// It also pins the key shape, so a typo cannot silently list nothing.
func TestBrokenReasonEveryEntryCarriesAReason(t *testing.T) {
	for version, reason := range brokenVersions {
		if !talosVersionPattern.MatchString(version) {
			t.Errorf("%q is not a Talos version and can never match a lookup", version)
		}
		if strings.TrimSpace(reason) == "" {
			t.Errorf("%q is listed as broken with no reason", version)
		}
		if len(strings.TrimSpace(reason)) < 20 {
			t.Errorf("%q carries the reason %q, which is too short to review", version, reason)
		}
	}
}
