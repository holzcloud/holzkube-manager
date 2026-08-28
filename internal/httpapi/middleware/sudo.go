package middleware

import "net/http"

// Sudo gates destructive routes behind a recent re-authentication.
//
// It reads the route's declarative Destructive marking rather than pattern
// matching on the URL (D-06). That marking is the single place where what is
// dangerous is readable, which is what makes a new destructive route that
// forgot the flag a visible oversight instead of a silent one.
//
// Nothing in this plan can open the window, so the gate is closed for every
// destructive route. That is deliberate: plan 04 adds the re-authentication
// endpoint that makes it passable.
func Sudo(destructive bool, hasSudo func(*http.Request) bool, deny func(http.ResponseWriter, *http.Request)) Middleware {
	return func(next http.Handler) http.Handler {
		if !destructive {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if hasSudo == nil || !hasSudo(r) {
				if deny != nil {
					deny(w, r)
				}
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
