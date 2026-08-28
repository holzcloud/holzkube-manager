package handlers

import (
	"net/http"

	"github.com/holzcloud/holzkube/internal/httpapi"
)

type systemStatus struct {
	SetupRequired bool                `json:"setup_required"`
	AuditChain    httpapi.ChainStatus `json:"audit_chain"`
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
				chain := d.AuditChain
				if chain.OK && d.Audit != nil {
					ok, file, line, err := d.Audit.Verify(r.Context())
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
					AuditChain:    chain,
				})
			}),
		},
	}
}
