package main

import (
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/audit"
	"github.com/holzcloud/holzkube-manager/internal/httpapi"
)

// This file closes the half of internal/audit/redact.go's claim that package
// could not assert about itself.
//
// That table says, in three separate comments, that it shows the *full set* of
// mutations -- including the ones listed with nothing permitted, so that a
// reader can see the whole surface rather than inferring it from absences. The
// fail-closed default makes the claim invisible when it is false: an action
// nobody listed and an action deliberately listed as permitting nothing both
// write `<redacted>` for everything, and no test inside internal/audit can tell
// them apart, because only the route table knows which actions exist.
//
// It shipped false. Phases 6 and 7 added eight audited actions -- the node
// actions, the confirmation, the job cancel, the config plan and apply, the
// patch create -- and not one of them had an entry, so every parameter of
// every reboot, reset and configuration apply was written as a marker while
// docs/api-contract.md described what those records would contain. Nothing
// failed, because nothing was looking.
//
// It lives in package main for the reason budget_test.go beside it does: the
// route table is assembled here, from every handler package's own Routes
// function, and this is the only place that knows the whole of it.
func TestEveryAuditedActionIsInTheAllowlist(t *testing.T) {
	var missing []string

	for _, route := range allRoutes() {
		if route.Action == "" {
			continue
		}
		if !audit.Listed(route.Action) {
			missing = append(missing, route.Method+" "+route.Pattern+" logs as "+route.Action)
		}
	}

	if len(missing) > 0 {
		t.Fatalf("these routes are audited under an action with no entry in the allowlist:\n  %s\n\n"+
			"Every parameter of each is written as %q, which is safe and is also a record that "+
			"says something happened and cannot say what. Add each action to internal/audit/redact.go, "+
			"with an empty list where nothing may appear in clear -- an empty entry is the table "+
			"stating that deliberately, which is the thing this guard exists to distinguish from "+
			"nobody having looked.",
			join(missing), audit.RedactedMarker)
	}
}

// TestEveryAllowlistedActionIsAReachableRoute is the other direction.
//
// An entry for an action no route emits is a decision about a field that no
// longer exists, and it reads as a live permission. Reviewing this table means
// reading entries that still apply.
func TestEveryAllowlistedActionIsAReachableRoute(t *testing.T) {
	emitted := map[string]bool{}
	for _, route := range allRoutes() {
		if route.Action != "" {
			emitted[route.Action] = true
		}
	}

	// Actions the audit log carries that no route declares, each with the
	// reason it is not one. They are written here rather than exempted in bulk
	// so that adding to the list is an argument.
	for _, action := range []string{
		// The setup and the login are audited by the handlers themselves
		// rather than by a route Action: both happen before there is a session
		// to attribute them to, which is the reason their records exist at
		// all.
		"setup.create",
		"auth.login",
		"auth.logout",
		"auth.sudo",
	} {
		emitted[action] = true
	}

	var stale []string
	for _, action := range audit.ListedActions() {
		if !emitted[action] {
			stale = append(stale, action)
		}
	}

	if len(stale) > 0 {
		t.Fatalf("these actions have allowlist entries and no route emits them:\n  %s\n\n"+
			"An entry for an action that cannot happen reads as a live permission for a field "+
			"that no longer exists. Remove it, or add the action to the exemption list above "+
			"with the reason it is audited without a route.", join(stale))
	}
}

// allRoutes is main's own route table, with a zero Deps.
//
// It calls routeTable rather than re-listing the handler packages, and that is
// the correction this file needed: when it kept its own list, phase 9's
// upgrade routes were added to main and not to the test, so the guard walked
// every route except the new ones and passed. A guard with its own copy of the
// thing it is guarding is a guard that goes quiet exactly when something is
// added.
//
// The dependencies are nil, which is exactly right for this: nothing here
// calls a handler. What is read is the declaration each route carries -- its
// method, its pattern and its action -- and those are literals in the Routes
// functions, independent of what the handlers close over.
func allRoutes() []httpapi.Route {
	return routeTable(httpapi.Deps{})
}

func join(lines []string) string {
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += "\n  "
		}
		out += l
	}
	return out
}
