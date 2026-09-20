package handlers

import (
	"net/http"

	"github.com/holzcloud/holzkube-manager/internal/changelog"
	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

type systemStatus struct {
	SetupRequired bool                `json:"setup_required"`
	AuditChain    httpapi.ChainStatus `json:"audit_chain"`

	// The two ways in, as they apply to the address this request arrived on.
	// The sign-in page renders from these rather than from a build-time
	// assumption: one instance answers on a LAN address that offers both and a
	// public name that offers only the provider, and a page that guesses would
	// be wrong on one of them.
	//
	// Neither field discloses anything an unauthenticated caller could not
	// establish by trying: the routes themselves already answer differently.
	OIDCEnabled   bool `json:"oidc_enabled"`
	PasswordLogin bool `json:"password_login"`

	// LocalSignInURL is where the local account still works, offered only on an
	// address that refuses it. Empty when there is no honest answer -- every
	// address this instance answers to is SSO-only, or the only ones left are
	// loopback names that would point a browser at the machine it is running on.
	//
	// It names nothing an unauthenticated caller could not establish by trying,
	// for the same reason the two flags above do not: it is an address this
	// process already answers on, and reaching it still requires a password.
	LocalSignInURL string `json:"local_sign_in_url,omitempty"`

	// TalosRange is the version window this build was tested against, and
	// AllowPreRelease whether this instance accepts a pre-release inside it
	// (OPS-03).
	//
	// Both are served rather than written into the browser bundle, for the
	// reason every other constant on a screen is: a copy in the bundle drifts
	// from the constants that enforce it, and the whole value of this pair is
	// that the screen and the refusal agree.
	//
	// Neither discloses anything: the range is in the release notes and the
	// setting is visible to anybody who connects a node running a release
	// candidate.
	TalosRange      string `json:"talos_range"`
	AllowPreRelease bool   `json:"allow_prerelease"`
}

// versionResponse is what the sidebar's version number and the panel behind it
// render from.
//
// One route for both, and one fetch, because they are one claim: this build,
// and what changed in it. Two routes would be two answers that could disagree
// -- a version from one and a changelog from another is exactly how a panel
// comes to describe a release nobody is running.
type versionResponse struct {
	// Version is what the binary was built from. "dev" for a build straight
	// out of a working tree, and the tag for a release, because goreleaser
	// stamps it through -ldflags.
	Version string `json:"version"`

	// Releases is the curated changelog, newest first, with the series each
	// belongs to resolved here rather than in the browser. Splitting a version
	// string is the kind of thing that ends up done twice and differently;
	// there is one implementation of it and it is in Go.
	Releases []releaseView `json:"releases"`
}

type releaseView struct {
	Version string             `json:"version"`
	Series  string             `json:"series"`
	Date    string             `json:"date"`
	Changes []changelog.Change `json:"changes"`
}

// SystemRoutes serves the instance status the UI polls before rendering
// anything else.
func SystemRoutes(d httpapi.Deps) []httpapi.Route {
	return []httpapi.Route{
		{
			Method:  http.MethodGet,
			Pattern: "/api/v1/system/version",

			// Behind the session gate, unlike the status route above it, and
			// the difference is deliberate. status has to answer before
			// authentication because setup_required is what decides whether
			// the setup wizard is shown at all. Nothing needs this one before
			// a session exists -- the sidebar it feeds is already behind the
			// gate -- and a build number served to anonymous callers is a
			// list of which advisories apply, handed out for free. The
			// operator loses nothing: they are signed in when they read it.
			RequiresSession: true,
			// The lowest role there is. A reader looking at a screen has the
			// same need as an administrator to know which build drew it --
			// more, if anything, since they are the ones who report that
			// something looks wrong and cannot check the code themselves.
			MinRole: model.RoleReader,
			Action:  "system.version",
			// The request is not read: this route takes nothing and answers
			// the same for every caller who is allowed to ask it. Whether the
			// caller is allowed is decided before the handler, by
			// RequiresSession above.
			Handler: handler(func(w http.ResponseWriter, _ *http.Request) {
				all := changelog.Releases()
				views := make([]releaseView, 0, len(all))
				for _, rel := range all {
					views = append(views, releaseView{
						Version: rel.Version,
						Series:  rel.Series(),
						Date:    rel.Date,
						// Never null: an empty list is "this release changed
						// nothing anyone would notice", and null is "the
						// server did not check" -- and the browser's schema
						// defaults apply to undefined rather than to null, so
						// the second one does not render at all.
						Changes: append(make([]changelog.Change, 0, len(rel.Changes)), rel.Changes...),
					})
				}
				writeJSON(w, http.StatusOK, versionResponse{
					Version:  d.Version,
					Releases: views,
				})
			}),
		},
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/system/status",
			RequiresSession: false,
			Action:          "system.status",
			Handler: handler(func(w http.ResponseWriter, r *http.Request) {
				users, err := d.Store.Users().List(r.Context())
				if err != nil {
					httpapi.WriteInternal(w, r, d.Logger, err)
					return
				}

				// The startup verdict is authoritative for a break: a chain
				// found broken at startup must stay reported (D-15). When
				// startup was clean, re-verify live so that damage occurring
				// while the process runs is not hidden until the next restart.
				//
				// The live re-verify is behind the session gate even though the
				// route is not. This endpoint has to answer before
				// authentication, because setup_required is what tells the UI
				// whether to show the setup wizard -- but re-verification
				// re-reads and re-hashes the whole window under the audit
				// writer's mutex, and the audit middleware is fail-closed, so an
				// anonymous caller repeating this request could stall or fail
				// every authenticated mutation in the process. An anonymous
				// caller gets the startup snapshot, which is the part of the
				// answer they need; D-15 is unaffected either way, because a
				// break found at startup is reported from the snapshot and is
				// never re-checked here at all.
				chain := d.AuditChain
				if chain.OK && d.Audit != nil && d.Auth.IsAuthenticated(r.Context()) {
					ok, file, line, err := d.Audit.CachedVerify(r.Context())
					if err != nil {
						httpapi.WriteInternal(w, r, d.Logger, err)
						return
					}
					// A clean chain names the file that was checked; a broken
					// one names the file the break is in, which is the file the
					// operator has to deal with by hand.
					if ok {
						file = d.Audit.CurrentFile()
					}
					chain = httpapi.ChainStatus{OK: ok, BrokenAtLine: line, File: file}
				}

				writeJSON(w, http.StatusOK, systemStatus{
					SetupRequired: len(users) == 0,
					OIDCEnabled:   d.OIDC != nil,
					PasswordLogin: !d.SSOOnly(r),
					// Only on an address that refuses the local account: on one
					// that accepts it, a link to somewhere else is an invitation
					// to leave a page that already works.
					LocalSignInURL: localSignIn(d, r),
					// A name, never a path. The audit directory sits under the
					// XDG-resolved absolute data directory, so the full path
					// discloses the OS username and the home directory layout
					// of a host whose data directory the threat model calls
					// equivalent to root on every managed node -- and this
					// answers before authentication. It is the same class of
					// string Internal(err) exists to strip. The operator does
					// not need the directory they configured; they deal with a
					// break by hand on a host they are already logged in to.
					AuditChain:      chain.Public(),
					TalosRange:      talos.MinSupportedVersion + " to " + talos.MaxSupportedVersion,
					AllowPreRelease: d.TalosMode.AllowPreRelease,
				})
			}),
		},
	}
}

// localSignIn is the address to offer beside the identity provider.
//
// Only on an address that refuses the local password, because on one that
// accepts it the link would invite an operator away from a page that already
// works.
func localSignIn(d httpapi.Deps, r *http.Request) string {
	if !d.SSOOnly(r) {
		return ""
	}
	return d.LocalSignIn()
}
