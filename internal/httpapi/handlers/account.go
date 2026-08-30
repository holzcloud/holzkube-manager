package handlers

import (
	"errors"
	"net/http"

	"github.com/holzcloud/holzkube-manager/internal/auth"
	"github.com/holzcloud/holzkube-manager/internal/httpapi"
)

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// AccountRoutes serves the operator's own account.
//
// POST /api/v1/account/password is the first Destructive route in holzkube-manager and
// the proof of the pattern every later phase depends on (D-06): the flag is set
// here, on the route, and the middleware reads it. Phase 6's reboot, shutdown
// and reset and phase 9's etcd member removal hang off the same marking, so the
// shape of this entry matters more than the endpoint does.
//
// It is destructive because it changes the credential guarding cluster PKI. An
// attacker holding a stolen session cookie and no password can do many things;
// locking the operator out of their own installation must not be one of them.
func AccountRoutes(d httpapi.Deps) []httpapi.Route {
	return []httpapi.Route{
		{
			Method:  http.MethodPost,
			Pattern: "/api/v1/account/password",

			// The D-06 marking, deliberately alone in its own group: gofmt
			// column-aligns neighbouring fields, and a flag that reads as
			// "Destructive     true" is a flag a reviewer skims past.
			Destructive: true,

			RequiresSession: true,
			Action:          "account.password",

			Handler: handler(func(w http.ResponseWriter, r *http.Request) { changePassword(d, w, r) }),
		},
	}
}

func changePassword(d httpapi.Deps, w http.ResponseWriter, r *http.Request) {
	var req changePasswordRequest
	if err := decodeJSON(w, r, &req); err != nil {
		httpapi.WriteProblem(w, r, httpapi.Validation("The request body could not be read as a password change."))
		return
	}

	if len(req.NewPassword) < minPasswordLen {
		httpapi.WriteProblem(w, r, httpapi.Validation("The new password is not valid.", httpapi.FieldError{
			Field:  "new_password",
			Reason: "must be at least 12 characters",
		}))
		return
	}

	err := d.Auth.ChangePassword(r.Context(), req.CurrentPassword, req.NewPassword)
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		// The same answer as a failed login. A caller who can tell "wrong
		// current password" from "not allowed" learns something about a
		// credential they do not have.
		httpapi.WriteProblem(w, r, httpapi.Unauthenticated())
		return
	case err != nil:
		httpapi.WriteInternal(w, r, d.Logger, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
