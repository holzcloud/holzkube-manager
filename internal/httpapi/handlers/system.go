package handlers

import (
	"net/http"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
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
}

// SystemRoutes serves the instance status the UI polls before rendering
// anything else.
func SystemRoutes(d httpapi.Deps) []httpapi.Route {
	return []httpapi.Route{
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
					// A name, never a path. The audit directory sits under the
					// XDG-resolved absolute data directory, so the full path
					// discloses the OS username and the home directory layout
					// of a host whose data directory the threat model calls
					// equivalent to root on every managed node -- and this
					// answers before authentication. It is the same class of
					// string Internal(err) exists to strip. The operator does
					// not need the directory they configured; they deal with a
					// break by hand on a host they are already logged in to.
					AuditChain: chain.Public(),
				})
			}),
		},
	}
}
