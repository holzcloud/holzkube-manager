package main

// The second half of "reachable from the interface", and the half that was
// missing.
//
// TestEveryRouteIsReachableFromTheInterface reads everything under web/src as
// ONE blob and asks whether a route's path appears in it. web/src contains the
// API client, so a path the client issues satisfies that question whether or
// not any screen ever calls the client function that issues it. That is the
// declaration-not-use blind spot the browser guard carries as a written entry
// (internal/imagefactory/guard_drift_test.go, guardBlindSpots), sitting in the
// guard where it costs the most: a client function nothing calls is a
// well-formed, compiling, type-checked way to reach a route that no operator
// can reach.
//
// It had live victims when this was written, found by the operator rather than
// by a test -- they imported a cluster and had no way to remove it again:
//
//	clusters.forget  DELETE /api/v1/clusters/{id}
//	machines.add     POST   /api/v1/machines
//
// The second is the one the code itself calls "the always-available second way
// into the inventory (D-08)", for a cluster whose membership does not fill
// itself. It was neither available nor a way.
//
// This asks the other question: for each member of the API client that issues a
// route, does anything OUTSIDE the client call it?

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// clientNotCalledByAScreen exempts a client member no screen calls, with the
// reason. Same discipline as notInTheInterface: the reason is the value, and
// "nothing calls it" is what somebody would also have written about forget.
var clientNotCalledByAScreen = map[string]string{
	"clusters.get": "the interface reads the cluster LIST and never one cluster alone; this " +
		"member exists for callers of the client that are not screens. It is dead weight in " +
		"the browser bundle rather than a missing button, which is a different repair.",
	"jobs.get": "the jobs screen polls the list and a job's detail arrives over the stream, so " +
		"a single-job read has no caller in the interface.",
	"patches.get": "the config screen reads a patch out of the list it already holds rather " +
		"than fetching one by id.",
}

var (
	clientGroup  = regexp.MustCompile(`^  (\w+):\s*\{`)
	clientMember = regexp.MustCompile(`^    (\w+):\s*(?:async\s*)?[(<]`)
	clientSend   = regexp.MustCompile(`send(?:JSON|Blob|Text|YAML)?\(\s*'(GET|POST|PUT|PATCH|DELETE)'\s*,\s*[` + "`" + `']([^` + "`" + `']+)`)
)

// TestEveryRouteTheClientIssuesIsCalledByAScreen is the use half.
//
// Its limits, stated rather than left to be found:
//
// It attributes a send() to the member declaration above it, by indentation.
// That is the shape api.ts has and not a property of TypeScript; a client
// written differently would attribute wrongly, and the count check below is
// what makes that visible rather than silent.
//
// A call is `group.member(`, so a screen that destructures the client
// (`const { forget } = api.clusters`) reads as not calling it. No screen does
// that today. If one starts, this guard reports a route that is reachable,
// which is the safe direction to be wrong in -- the other one is what shipped.
func TestEveryRouteTheClientIssuesIsCalledByAScreen(t *testing.T) {
	t.Parallel()

	issuers := routeIssuingClientMembers(t)
	if len(issuers) < 30 {
		t.Fatalf("found only %d route-issuing members in the API client; this guard reads "+
			"api.ts by indentation and a client written differently would be read wrongly. "+
			"A low count is that failure, not a small client.", len(issuers))
	}

	screens := frontendOutsideTheClient(t)

	var unreachable []string
	for member, routes := range issuers {
		if _, known := clientNotCalledByAScreen[member]; known {
			continue
		}
		group, name, _ := strings.Cut(member, ".")
		called := regexp.MustCompile(`\b` + regexp.QuoteMeta(group) + `\s*\.\s*` + regexp.QuoteMeta(name) + `\s*\(`)
		if !called.MatchString(screens) {
			unreachable = append(unreachable, member+" -- "+strings.Join(routes, ", "))
		}
	}
	sort.Strings(unreachable)

	if len(unreachable) > 0 {
		t.Errorf("the API client issues these routes and no screen calls the member that "+
			"issues them:\n  %s\n\n"+
			"The path is in web/src, so the reachability guard beside this one is satisfied. "+
			"Nobody can get there. Either give it a way in, or exempt it in "+
			"clientNotCalledByAScreen with a reason that says who calls it instead.",
			strings.Join(unreachable, "\n  "))
	}
}

// TestNoStaleClientExemptions is the other direction, and it is not decoration:
// an exemption for a member a screen has since learned to call is a hole held
// open by a stale comment, which is how entry 69 went wrong.
func TestNoStaleClientExemptions(t *testing.T) {
	t.Parallel()

	issuers := routeIssuingClientMembers(t)
	screens := frontendOutsideTheClient(t)

	for member, reason := range clientNotCalledByAScreen {
		if _, ok := issuers[member]; !ok {
			t.Errorf("clientNotCalledByAScreen exempts %q, which issues no route in the API "+
				"client -- the member was renamed or removed and the exemption outlived it", member)
			continue
		}
		if strings.TrimSpace(reason) == "" {
			t.Errorf("%q is exempted with no reason; the reason is the whole value of the list", member)
		}
		group, name, _ := strings.Cut(member, ".")
		called := regexp.MustCompile(`\b` + regexp.QuoteMeta(group) + `\s*\.\s*` + regexp.QuoteMeta(name) + `\s*\(`)
		if called.MatchString(screens) {
			t.Errorf("clientNotCalledByAScreen exempts %q on the grounds that no screen calls "+
				"it, and a screen now does. Delete the exemption.", member)
		}
	}
}

// routeIssuingClientMembers maps group.member to the routes it issues.
func routeIssuingClientMembers(t *testing.T) map[string][]string {
	t.Helper()

	source := readClientSource(t)
	issuers := map[string][]string{}

	var group, member string
	for _, line := range strings.Split(source, "\n") {
		if m := clientGroup.FindStringSubmatch(line); m != nil {
			group, member = m[1], ""
			continue
		}
		if m := clientMember.FindStringSubmatch(line); m != nil {
			member = m[1]
		}
		m := clientSend.FindStringSubmatch(line)
		if m == nil || group == "" || member == "" {
			continue
		}
		path := regexp.MustCompile(`\$\{[^}]*\}`).ReplaceAllString(m[2], "{}")
		key := group + "." + member
		route := m[1] + " " + path
		if !slicesContains(issuers[key], route) {
			issuers[key] = append(issuers[key], route)
		}
	}
	return issuers
}

func slicesContains(haystack []string, needle string) bool {
	for _, each := range haystack {
		if each == needle {
			return true
		}
	}
	return false
}

// readClientSource returns web/src/api.ts, which is the only place this product
// issues an HTTP request from the browser.
func readClientSource(t *testing.T) string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("..", "..", "web", "src", "api.ts"))
	if err != nil {
		t.Skipf("web/src/api.ts is not present, so there is no client to check: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("web/src/api.ts is empty, so this guard would pass by finding nothing")
	}
	return string(raw)
}

// frontendOutsideTheClient is every non-test TypeScript file under web/src
// EXCEPT the API client.
//
// Excluding api.ts is the whole point and is the difference between this guard
// and the one beside it: with the client included, a member that only calls
// itself -- which every member does -- counts as called.
//
// Tests are excluded for the reason readFrontendSources already establishes by
// measurement: clusters.test.tsx asserting an href kept a route looking
// reachable after the only thing that reached it was gone. A test calling a
// client member is not an operator reaching a route.
func frontendOutsideTheClient(t *testing.T) string {
	t.Helper()

	root := filepath.Join("..", "..", "web", "src")
	var b strings.Builder
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if ext := filepath.Ext(path); ext != ".ts" && ext != ".tsx" {
			return nil
		}
		base := filepath.Base(path)
		if base == "api.ts" || strings.Contains(base, ".test.") || strings.Contains(base, ".browser.") {
			return nil
		}
		raw, err := os.ReadFile(path) //nolint:gosec // a path this test walked itself
		if err != nil {
			return err
		}
		b.Write(raw)
		b.WriteByte('\n')
		return nil
	})
	if err != nil {
		t.Fatalf("read web/src: %v", err)
	}
	if b.Len() == 0 {
		t.Fatal("web/src holds no screens outside the API client, so this guard would pass by " +
			"finding nothing")
	}
	return b.String()
}
