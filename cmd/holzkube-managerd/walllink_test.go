package main

import (
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
)

// A wall link opens one route (2026-09-20).
//
// The credential lives on a television: bookmarked, photographed, mailed around
// an office. What stops that from being a problem is not a role somebody has to
// reason about -- it is that exactly one route in the table accepts it, and
// anybody can check which by reading the table.
//
// This is that check. A second route carrying the flag would be a widening
// nobody proposed, and it would be one added line in a diff.

// TestOnlyTheWallAcceptsAWallLink.
func TestOnlyTheWallAcceptsAWallLink(t *testing.T) {
	t.Parallel()

	var opened []string
	for _, route := range routeTable(httpapi.Deps{}) {
		if route.WallLink {
			opened = append(opened, route.Method+" "+route.Pattern)
		}
	}

	want := "GET /api/v1/clusters/{id}/wall"
	if len(opened) != 1 || opened[0] != want {
		t.Errorf("wall links open %v, want exactly [%s].\n\n"+
			"This credential is handed out as a URL on a screen in a corridor. Its whole safety "+
			"property is that ONE named route accepts it, checkable by reading the route table. "+
			"A second route here widens what a bookmarked, photographed, forwarded link can "+
			"reach, and does it in one added line.", opened, want)
	}
}

// TestAWallRouteStillRequiresSomething, so the flag cannot be read as "open".
func TestAWallRouteStillRequiresSomething(t *testing.T) {
	t.Parallel()

	for _, route := range routeTable(httpapi.Deps{}) {
		if !route.WallLink {
			continue
		}
		if !route.RequiresSession {
			t.Errorf("%s %s accepts a wall link and requires no session. The flag WIDENS a gate, "+
				"it does not replace one, and a route with neither is open to anybody",
				route.Method, route.Pattern)
		}
		if route.Destructive {
			t.Errorf("%s %s accepts a wall link and is destructive. A credential on a television "+
				"must not reach anything that changes a cluster", route.Method, route.Pattern)
		}
		if route.Method != "GET" {
			t.Errorf("%s %s accepts a wall link and is not a GET. A screen reads; anything else "+
				"is a widening", route.Method, route.Pattern)
		}
	}
}
