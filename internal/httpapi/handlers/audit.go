package handlers

import (
	"net/http"

	"github.com/holzcloud/holzkube/internal/audit"
	"github.com/holzcloud/holzkube/internal/httpapi"
)

// auditPage is the response shape pinned in docs/api-contract.md.
//
// NextCursor is a pointer so it marshals as an explicit null rather than being
// omitted. The contract is that the field is always present and is either a
// number or null; null means "no further page". Zero is never a valid cursor,
// so a client that tests truthiness instead of `!== null` would only be right
// by accident.
type auditPage struct {
	Items      []audit.Record `json:"items"`
	NextCursor *uint64        `json:"next_cursor"`
}

// AuditRoutes serves the audit log. In phase 1 the query parameters from the
// contract are accepted and not yet applied; plan 03 implements the filters and
// the cursor, against this same response shape.
func AuditRoutes(d httpapi.Deps) []httpapi.Route {
	return []httpapi.Route{
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/audit",
			RequiresSession: true,
			Action:          "audit.list",
			Handler: handler(func(w http.ResponseWriter, r *http.Request) {
				records, err := d.Audit.List(r.Context())
				if err != nil {
					httpapi.WriteInternal(w, r, d.Logger, err)
					return
				}
				if records == nil {
					records = []audit.Record{}
				}
				writeJSON(w, http.StatusOK, auditPage{Items: records, NextCursor: nil})
			}),
		},
	}
}
