package handlers

import (
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/holzcloud/holzkube/internal/audit"
	"github.com/holzcloud/holzkube/internal/httpapi"
)

// maxActionLen bounds the action filter. The token is a short dotted name; a
// longer one is a mistake or a probe, and either deserves a 400 rather than a
// scan of the archive.
const maxActionLen = 64

// AuditRoutes serves the audit log.
//
// The response is audit.Page as-is: its json tags are the wire shape pinned in
// docs/api-contract.md, including next_cursor being present and number-or-null.
// Re-wrapping it here would be a second place for that shape to drift.
func AuditRoutes(d httpapi.Deps) []httpapi.Route {
	return []httpapi.Route{
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/audit",
			RequiresSession: true,
			Action:          "audit.list",
			Handler: handler(func(w http.ResponseWriter, r *http.Request) {
				filter, problem := parseAuditFilter(r.URL.Query())
				if problem != nil {
					httpapi.WriteProblem(w, r, problem)
					return
				}

				page, err := d.Audit.Query(r.Context(), filter)
				if err != nil {
					httpapi.WriteInternal(w, r, d.Logger, err)
					return
				}
				writeJSON(w, http.StatusOK, page)
			}),
		},
	}
}

// parseAuditFilter decodes the query parameters of the Audit Query Contract.
//
// Every parameter is optional and every malformed one is a 400 naming the
// field. Silently ignoring an unparsable from= would answer a different
// question than the one asked, which in a forensic view is worse than an error.
func parseAuditFilter(q url.Values) (audit.Filter, *httpapi.Problem) {
	var (
		f    audit.Filter
		errs []httpapi.FieldError
	)

	if v := q.Get("from"); v != "" {
		ts, err := time.Parse(time.RFC3339, v)
		if err != nil {
			errs = append(errs, httpapi.FieldError{Field: "from", Reason: "must be an RFC 3339 timestamp"})
		} else {
			f.From = ts.UTC()
		}
	}
	if v := q.Get("to"); v != "" {
		ts, err := time.Parse(time.RFC3339, v)
		if err != nil {
			errs = append(errs, httpapi.FieldError{Field: "to", Reason: "must be an RFC 3339 timestamp"})
		} else {
			f.To = ts.UTC()
		}
	}
	if !f.From.IsZero() && !f.To.IsZero() && f.To.Before(f.From) {
		errs = append(errs, httpapi.FieldError{Field: "to", Reason: "must not be earlier than from"})
	}

	if v := q.Get("action"); v != "" {
		if !isActionToken(v) {
			errs = append(errs, httpapi.FieldError{
				Field:  "action",
				Reason: "must be a dotted token of lowercase letters, digits, '.', '-' and '_'",
			})
		} else {
			f.Action = v
		}
	}

	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			errs = append(errs, httpapi.FieldError{Field: "limit", Reason: "must be a positive integer"})
		} else {
			// Above the ceiling the request is served at the ceiling rather
			// than refused: the caller asked for "as much as possible", and the
			// page tells them where to continue.
			f.Limit = n
		}
	}

	if v := q.Get("cursor"); v != "" {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil || n == 0 {
			// Zero is explicitly not a cursor. Accepting it would let a client
			// that mistook null for 0 get a silently wrong first page.
			errs = append(errs, httpapi.FieldError{Field: "cursor", Reason: "must be a sequence number from a previous next_cursor"})
		} else {
			f.Cursor = n
		}
	}

	if len(errs) > 0 {
		return audit.Filter{}, httpapi.Validation("The audit query parameters are not valid.", errs...)
	}
	return f, nil
}

func isActionToken(s string) bool {
	if len(s) > maxActionLen {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}
