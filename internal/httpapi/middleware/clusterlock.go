package middleware

import (
	"net/http"
)

// ClusterLock refuses a mutating request against a cluster that was adopted
// read-only (INV-12, P17).
//
// It works the way Destructive does and for the same reason (D-22): the route
// declares how to find the cluster it acts on, this link asks, and nothing
// anywhere pattern matches on a URL. A lock that only the user interface
// honours is not a lock -- the same argument that put dry-run into the
// transport rather than into a handler.
//
// It sits inside the audit link and outside the sudo gate. Inside audit,
// because an attempt to change a locked cluster is exactly the kind of thing
// the archive exists to hold. Outside sudo, because asking an operator for
// their password and then refusing them anyway teaches them that the password
// prompt means nothing.
//
// A route with no scope function is not cluster-scoped and passes straight
// through. That is every read route and every route about the instance itself,
// and it is why this link can be applied to the whole table.
func ClusterLock(
	scope func(*http.Request) (string, error),
	locked func(*http.Request, string) error,
	deny func(http.ResponseWriter, *http.Request, error),
) Middleware {
	return func(next http.Handler) http.Handler {
		if scope == nil || locked == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, err := scope(r)
			if err != nil {
				deny(w, r, err)
				return
			}
			if err := locked(r, id); err != nil {
				deny(w, r, err)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
