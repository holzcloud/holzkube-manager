package changelog_test

import (
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/changelog"
)

// The changelog is the one file in this repository whose only reader is an
// operator, so nothing about it fails loudly on its own: a duplicated version,
// an entry with no text, or a list in the wrong order all render, and all
// render as something slightly untrue. These are the claims the panel makes
// about it, held here rather than hoped for.

func TestTheChangelogIsWellFormed(t *testing.T) {
	t.Parallel()

	releases := changelog.Releases()
	if len(releases) == 0 {
		t.Fatal("the changelog is empty; the panel would read as 'nothing ever changed'")
	}

	seen := map[string]bool{}
	for _, r := range releases {
		if !strings.HasPrefix(r.Version, "v") {
			t.Errorf("version %q does not start with v; it has to be spelled exactly as the "+
				"git tag is, because the release pipeline checks this file against the tag", r.Version)
		}
		if seen[r.Version] {
			t.Errorf("version %q appears twice; the panel would render it twice and the second "+
				"one would silently win any lookup", r.Version)
		}
		seen[r.Version] = true

		if r.Date == "" {
			t.Errorf("%s has no date", r.Version)
		}
		if len(r.Changes) == 0 {
			t.Errorf("%s lists no changes; a release worth a version is worth a sentence", r.Version)
		}
		for i, c := range r.Changes {
			if strings.TrimSpace(c.Text) == "" {
				t.Errorf("%s change %d has no text; the icon is decoration and says nothing on "+
					"its own", r.Version, i)
			}
		}
	}
}

// TestReleasesAreNewestFirst.
//
// The panel does not sort. It renders in the order it is given and opens on the
// first series, so an entry appended to the bottom of the file would be shown
// as the oldest release in the newest series -- which looks like a rendering
// quirk rather than like a file in the wrong order.
//
// Comparing version strings properly is a whole package, and this does not
// attempt it: dates are already required above, they are ISO, and they are what
// a reader is actually comparing.
func TestReleasesAreNewestFirst(t *testing.T) {
	t.Parallel()

	releases := changelog.Releases()
	for i := 1; i < len(releases); i++ {
		if releases[i].Date > releases[i-1].Date {
			t.Errorf("%s (%s) is listed after %s (%s); the file is newest first",
				releases[i].Version, releases[i].Date,
				releases[i-1].Version, releases[i-1].Date)
		}
	}
}

func TestSeriesGroupsAVersionByItsFirstTwoComponents(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ version, want string }{
		{"v1.16.0", "v1.16"},
		{"v1.16.0-beta.2", "v1.16"},
		{"v0.1.0", "v0.1"},
		{"v2.0.0-rc.1", "v2.0"},
		// Not a shape this project produces, and it still has to return
		// something rather than panic: the panel groups by whatever this says.
		{"v3", "v3"},
	} {
		got := changelog.Release{Version: tc.version}.Series()
		if got != tc.want {
			t.Errorf("Series(%q) = %q, want %q", tc.version, got, tc.want)
		}
	}
}

// TestASeriesIsContiguous keeps the panel's grouping honest.
//
// It builds its tabs by walking the list in order and opening a new group the
// first time it sees a series name. A file that interleaved two series -- v1.16,
// v0.1, v1.16 -- would therefore produce two tabs with the same name, and the
// second would be unreachable because both buttons carry the same label.
func TestASeriesIsContiguous(t *testing.T) {
	t.Parallel()

	var order []string
	closed := map[string]bool{}
	for _, r := range changelog.Releases() {
		series := r.Series()
		if len(order) > 0 && order[len(order)-1] == series {
			continue
		}
		if closed[series] {
			t.Errorf("series %s appears again after %s; the panel would render two tabs with "+
				"one name", series, order[len(order)-1])
		}
		if len(order) > 0 {
			closed[order[len(order)-1]] = true
		}
		order = append(order, series)
	}
}

// TestNewestIsTheFirstEntry pins what the release pipeline relies on.
func TestNewestIsTheFirstEntry(t *testing.T) {
	t.Parallel()

	releases := changelog.Releases()
	if changelog.Newest().Version != releases[0].Version {
		t.Errorf("Newest() = %s, first entry = %s", changelog.Newest().Version, releases[0].Version)
	}
}

// TestReleasesHandsOutACopy. The slice is package state, and a caller that
// sorted or truncated it in place would change what every later caller sees --
// including the HTTP handler, which builds one response per request from it.
func TestReleasesHandsOutACopy(t *testing.T) {
	t.Parallel()

	first := changelog.Releases()
	original := first[0].Version
	first[0].Version = "v0.0.0-tampered"

	if changelog.Releases()[0].Version != original {
		t.Error("mutating the returned slice changed what the next caller sees")
	}
}
