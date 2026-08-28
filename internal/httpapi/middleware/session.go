package middleware

import (
	"net/http"

	"github.com/alexedwards/scs/v2"
)

// Session loads the session before the handler runs and writes it back after,
// so every link inside the chain can read the identity from the context.
func Session(sm *scs.SessionManager) Middleware {
	return func(next http.Handler) http.Handler {
		if sm == nil {
			return next
		}
		return sm.LoadAndSave(next)
	}
}
