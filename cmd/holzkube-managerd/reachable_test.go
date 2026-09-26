package main

// Every route has to be reachable from the interface, or be named here as one
// that deliberately is not.
//
// This guard exists because the same omission shipped three times. The
// password form had a route and a sudo flow from phase 1 and no way in until
// phase 10 (UAT gap G-01-1). The support bundle was argued for on the grounds
// that during an incident the operator is in a browser rather than on a
// console, and shipped with nothing to click. node.remove-from-cluster made
// the whole of phase 9's etcd work curl-only.
//
// None of those were caught by a test, because every test that existed asked
// whether the route *worked*. A route that works and cannot be reached passes
// all of them, and looks finished in a diff.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/httpapi/handlers"
)

// notInTheInterface is every route the browser deliberately never calls.
//
// Each entry needs a reason, and the reasons are the point: "the frontend does
// not call it" is what somebody would also have written about the support
// bundle. What belongs here is a route whose *caller is not the interface*.
// The OIDC routes are not in this list and not in the table either: they are
// registered only when a provider is configured, and routeTable is walked here
// with empty Deps. That is a gap in the guard's reach rather than a decision,
// and it is stated rather than papered over -- the two of them are a browser
// redirect target and its start, neither of which the SPA calls.
var notInTheInterface = map[string]string{
	"GET /metrics": "Prometheus scrapes it on a timer. It needs no session because a scraper has " +
		"none, and a link to it would offer the operator a page of text nobody wants to read. " +
		"What the interface owes it is discoverability rather than a call, and the settings " +
		"screen carries the scrape config for that reason.",
}

// interfaceInFlight is the routes whose screen is being written at the same time
// as the routes, against docs/api-contract.md, in a change of its own.
//
// It is NOT an exemption in notInTheInterface's sense -- these routes' caller
// is the interface -- and it is built so that it cannot go stale the way a
// written exemption can: each group holds only while web/src mentions NONE of
// its routes. The moment the screen lands and mentions one, that group is void
// for all of its routes, and every one it does not mention is reported like
// any other route. Deleting a void group then is housekeeping; leaving it is
// harmless, because it no longer exempts anything.
//
// Groups and not one set, because two pieces of work can be in flight at once
// and land at different times: with one set, the power screen landing voided
// the exemption of a history route whose screen was still being written, and
// the only way to keep the guard green would have been a permanent exemption
// -- which is exactly the kind of line this construction exists to avoid.
func interfaceInFlight() []map[string]bool {
	group := func(routes []httpapi.Route) map[string]bool {
		out := map[string]bool{}
		for _, route := range routes {
			out[route.Method+" "+route.Pattern] = true
		}
		return out
	}
	return []map[string]bool{
		// The power model (2026-09-26): the screen is being built against the
		// contract in parallel, by a separate change.
		group(handlers.PowerRoutes(httpapi.Deps{})),
		// The metrics history (2026-09-26): the charts that open on the last
		// day are being written against the contract in parallel.
		group(handlers.HistoryRoutes(httpapi.Deps{})),
	}
}

// TestEveryRouteIsReachableFromTheInterface walks the route table against the
// frontend sources.
//
// The match is deliberately crude -- the path's literal prefix and its trailing
// segment -- because the frontend builds these paths by interpolation and
// anything stricter would be a test of how the strings happen to be assembled.
// What it can say is that nothing under web/src mentions this route at all,
// which is exactly the shape all three historical omissions had.
//
// Two limits, stated rather than left to be discovered:
//
// A path named only in a comment counts as mentioned. That was measured --
// deleting the password route from the API client still passes, because
// settings.tsx explains in prose which route it posts to. It is a weakness and
// not a fatal one: prose about a route exists because somebody built the
// feature, and a route that was never wired has no prose either. Removing the
// support bundle and remove-from-cluster from the client is caught.
//
// Routes registered conditionally are invisible here, because routeTable is
// walked with empty Deps. Today that is the two OIDC routes and nothing else.
func TestEveryRouteIsReachableFromTheInterface(t *testing.T) {
	t.Parallel()

	sources := readFrontendSources(t)

	// Each group is void as soon as the interface mentions any route of it.
	inFlight := map[string]bool{}
	for _, group := range interfaceInFlight() {
		void := false
		for _, route := range routeTable(httpapi.Deps{}) {
			if group[route.Method+" "+route.Pattern] && mentions(sources, route.Pattern) {
				void = true
				break
			}
		}
		if void {
			continue
		}
		for name := range group {
			inFlight[name] = true
		}
	}

	var unreachable []string
	for _, route := range routeTable(httpapi.Deps{}) {
		name := route.Method + " " + route.Pattern
		if _, known := notInTheInterface[name]; known {
			continue
		}
		if inFlight[name] {
			continue
		}
		if !mentions(sources, route.Pattern) {
			unreachable = append(unreachable, name)
		}
	}

	if len(unreachable) > 0 {
		t.Errorf("no file under web/src mentions these routes:\n  %s\n\n"+
			"Either give the route a way in, or add it to notInTheInterface with a reason that "+
			"says who calls it instead. A route that works and cannot be reached passes every "+
			"other test in this repository and looks finished in a diff -- which is how the "+
			"password form, the support bundle and remove-from-cluster each shipped.",
			strings.Join(unreachable, "\n  "))
	}
}

// TestNoStaleExemptions is the other direction.
//
// An exemption for a route that no longer exists is a line nobody will read
// again, and an exemption for a route the interface *has* since learned to call
// is a hole held open by a stale comment.
func TestNoStaleExemptions(t *testing.T) {
	t.Parallel()

	live := map[string]bool{}
	for _, route := range routeTable(httpapi.Deps{}) {
		live[route.Method+" "+route.Pattern] = true
	}

	sources := readFrontendSources(t)

	for name, reason := range notInTheInterface {
		if !live[name] {
			t.Errorf("notInTheInterface exempts %q, which is not in the route table", name)
			continue
		}
		if strings.TrimSpace(reason) == "" {
			t.Errorf("%q is exempted with no reason; the reason is the whole value of the list", name)
		}

	}

	// Deliberately no check that an exempted route goes unmentioned in web/src.
	// It was written and it was wrong: /metrics is exempted because a scraper
	// calls it and the browser does not, and the settings screen names the path
	// so that an operator can find it at all. Mentioning a route is not calling
	// it, and a guard that confused the two would have argued against the
	// discoverability the exemption itself asks for.
	_ = sources
}

// readFrontendSources concatenates the TypeScript under web/src, without its
// tests.
//
// Excluding them is the difference between a guard that works and one that
// does not, and it was established by trying both. With test files included,
// removing the support bundle from the API client still passed -- because
// clusters.test.tsx asserts the href, and the assertion kept the route looking
// reachable after the only thing that reached it was gone. Two of the three
// historical omissions went undetected that way.
//
// A test mentioning a route is not an operator reaching one. If the only place
// a path appears is a test, that is the finding rather than the exemption.
func readFrontendSources(t *testing.T) string {
	t.Helper()

	root := filepath.Join("..", "..", "web", "src")
	if _, err := os.Stat(root); err != nil {
		t.Skipf("web/src is not present, so there is no interface to check against: %v", err)
	}

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
		if base := filepath.Base(path); strings.Contains(base, ".test.") ||
			strings.Contains(base, ".browser.") {
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
		t.Fatal("web/src contains no TypeScript, so this test would pass by finding nothing")
	}
	return b.String()
}

// mentions reports whether the frontend refers to a route pattern.
//
// A pattern like /api/v1/machines/{id}/reboot is built in the browser as
// `/api/v1/machines/${id}/reboot`, so neither half alone is enough: the prefix
// matches too much -- every machine route shares it -- and the suffix matches
// too little, because "/reboot" appears in prose. Both halves have to be
// present, which is as close as a text match gets to the real question.
func mentions(sources, pattern string) bool {
	prefix, rest, found := strings.Cut(pattern, "{")
	if !found {
		return strings.Contains(sources, pattern)
	}
	if !strings.Contains(sources, prefix) {
		return false
	}

	// Everything after the last parameter. A pattern that ends in one -- say
	// /api/v1/schematics/{id} -- has nothing left, and the prefix is then the
	// whole of what can be checked.
	_, tail, _ := strings.Cut(rest, "}")
	for strings.Contains(tail, "{") {
		_, after, _ := strings.Cut(tail, "{")
		_, tail, _ = strings.Cut(after, "}")
	}
	if strings.TrimSpace(tail) == "" {
		return true
	}
	if strings.Contains(sources, tail) {
		return true
	}

	// The last segment as a bare string. Some routes are reached through one
	// call with the verb as an argument -- api.machines.action(id, 'reboot')
	// builds /api/v1/machines/${id}/${kind} -- so the tail never appears as
	// path text anywhere. What does appear is the segment itself, and asking
	// for that as well is the difference between a guard that reports the
	// three real omissions and one that reports every route built by
	// interpolation.
	segment := strings.Trim(tail, "/")
	if segment == "" {
		return false
	}

	// The same shape one level down: /power/${action} for seven routes that
	// differ only in their last segment. The fixed segment has to appear as
	// path text and the last one as a bare string, which is the rule above
	// applied to the part of the path that is interpolated. Two segments and
	// no more, so a long literal path is still held to appearing literally.
	if dir, last, nested := strings.Cut(segment, "/"); nested {
		if strings.Contains(last, "/") {
			return false
		}
		return strings.Contains(sources, "/"+dir+"/") && quotedIn(sources, last)
	}
	return quotedIn(sources, segment)
}

func quotedIn(sources, segment string) bool {
	return strings.Contains(sources, `'`+segment+`'`) || strings.Contains(sources, `"`+segment+`"`)
}
