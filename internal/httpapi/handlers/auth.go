package handlers

import (
	"errors"
	"net/http"

	"github.com/holzcloud/holzkube/internal/auth"
	"github.com/holzcloud/holzkube/internal/httpapi"
	"github.com/holzcloud/holzkube/internal/model"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type sudoRequest struct {
	Password string `json:"password"`
}

type meResponse struct {
	ID       model.UserID `json:"id"`
	Username string       `json:"username"`
}

// AuthRoutes serves login, logout and the current-identity probe.
func AuthRoutes(d httpapi.Deps) []httpapi.Route {
	return []httpapi.Route{
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/auth/login",
			RequiresSession: false,
			Action:          "auth.login",
			Handler:         handler(func(w http.ResponseWriter, r *http.Request) { login(d, w, r) }),
		},
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/auth/logout",
			RequiresSession: true,
			Action:          "auth.logout",
			Handler:         handler(func(w http.ResponseWriter, r *http.Request) { logout(d, w, r) }),
		},
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/auth/me",
			RequiresSession: true,
			Action:          "auth.me",
			Handler:         handler(func(w http.ResponseWriter, r *http.Request) { me(d, w, r) }),
		},
		{
			Method:  http.MethodPost,
			Pattern: "/api/v1/auth/sudo",

			// Not destructive, deliberately. Gating the re-authentication
			// behind an open window would be a lock whose key is inside it.
			Destructive: false,

			RequiresSession: true,
			Action:          "auth.sudo",
			Handler:         handler(func(w http.ResponseWriter, r *http.Request) { openSudo(d, w, r) }),
		},
	}
}

func login(d httpapi.Deps, w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(w, r, &req); err != nil {
		// Even a malformed login body answers with the authentication problem
		// rather than a validation one: a client that can tell "bad shape" from
		// "bad credentials" can probe the account space by shape alone.
		httpapi.WriteProblem(w, r, httpapi.Unauthenticated())
		return
	}

	_, err := d.Auth.Login(r.Context(), req.Username, req.Password)
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		httpapi.WriteProblem(w, r, httpapi.Unauthenticated())
		return
	case err != nil:
		httpapi.WriteInternal(w, r, d.Logger, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func logout(d httpapi.Deps, w http.ResponseWriter, r *http.Request) {
	if err := d.Auth.Logout(r.Context()); err != nil {
		httpapi.WriteInternal(w, r, d.Logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// openSudo re-authenticates an already authenticated session, opening the
// window that destructive routes require (D-05).
//
// It answers 401 rather than 428 for a wrong password: the caller is logged in,
// so the failure is about the credential, not about the window.
func openSudo(d httpapi.Deps, w http.ResponseWriter, r *http.Request) {
	var req sudoRequest
	if err := decodeJSON(w, r, &req); err != nil {
		httpapi.WriteProblem(w, r, httpapi.Unauthenticated())
		return
	}

	u, ok := d.Auth.CurrentUser(r.Context())
	if !ok {
		httpapi.WriteProblem(w, r, httpapi.Unauthenticated())
		return
	}

	valid, err := auth.Verify(req.Password, u.PasswordHash)
	if err != nil {
		httpapi.WriteInternal(w, r, d.Logger, err)
		return
	}
	if !valid {
		httpapi.WriteProblem(w, r, httpapi.Unauthenticated())
		return
	}

	d.Auth.OpenSudoWindow(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

func me(d httpapi.Deps, w http.ResponseWriter, r *http.Request) {
	u, ok := d.Auth.CurrentUser(r.Context())
	if !ok {
		httpapi.WriteProblem(w, r, httpapi.Unauthenticated())
		return
	}
	writeJSON(w, http.StatusOK, meResponse{ID: u.ID, Username: u.Username})
}
