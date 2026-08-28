package middleware

import (
	"bytes"
	"net/http"
)

// Sudo gates destructive routes behind a recent re-authentication.
//
// It reads the route's declarative Destructive marking rather than pattern
// matching on the URL (D-06). That marking is the single place where what is
// dangerous is readable, which is what makes a new destructive route that
// forgot the flag a visible oversight instead of a silent one. Phase 6's node
// actions and phase 9's etcd member removal gate through this same link.
//
// The window is the only mechanism that limits the damage a stolen session
// cookie can do (T-01-25): every other check in the chain is satisfied by the
// cookie itself.
//
// touch runs after a destructive action has actually been performed, so a
// series of them stays inside one window (D-05). It runs only for a response
// that succeeded: a refused or failed request must not buy the caller time it
// did not earn.
func Sudo(
	destructive bool,
	hasSudo func(*http.Request) bool,
	touch func(*http.Request),
	deny func(http.ResponseWriter, *http.Request),
) Middleware {
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

			if touch == nil {
				next.ServeHTTP(w, r)
				return
			}

			// The response is held back until the window has been refreshed.
			//
			// This is not stylistic. The session manager persists the session
			// the first time anything is written to the response, so a session
			// value set after the handler has answered is computed, discarded
			// and never noticed -- the window silently fails to refresh and
			// the operator is asked for their password again mid-series. The
			// refresh has to happen while the response is still ours to write.
			held := &heldResponse{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(held, r)

			if held.status < http.StatusBadRequest {
				touch(r)
			}
			held.flush()
		})
	}
}

// heldResponse buffers a response so a middleware can still act on the request
// context after the handler has finished.
//
// Buffering is safe here and only here: this link wraps destructive routes,
// whose answers are a status code and at most a few hundred bytes of
// problem+json. Nothing that streams is destructive.
type heldResponse struct {
	http.ResponseWriter

	status      int
	wroteHeader bool
	body        bytes.Buffer
}

func (h *heldResponse) WriteHeader(code int) {
	if h.wroteHeader {
		return
	}
	h.status = code
	h.wroteHeader = true
}

func (h *heldResponse) Write(b []byte) (int, error) {
	if !h.wroteHeader {
		h.WriteHeader(http.StatusOK)
	}
	return h.body.Write(b)
}

// Status reports the status the handler chose, so the gate can tell a performed
// action from a refused one.
func (h *heldResponse) Status() int { return h.status }

func (h *heldResponse) flush() {
	h.ResponseWriter.WriteHeader(h.status)
	if h.body.Len() > 0 {
		_, _ = h.ResponseWriter.Write(h.body.Bytes())
	}
}
