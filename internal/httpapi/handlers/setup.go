package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/holzcloud/holzkube/internal/auth"
	"github.com/holzcloud/holzkube/internal/httpapi"
	"github.com/holzcloud/holzkube/internal/model"
)

const (
	minUsernameLen = 3
	maxUsernameLen = 64
	minPasswordLen = 12
)

type setupRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type setupResponse struct {
	ID       model.UserID `json:"id"`
	Username string       `json:"username"`
}

// SetupRoutes serves the first-run wizard that creates the one operator account.
func SetupRoutes(d httpapi.Deps) []httpapi.Route {
	return []httpapi.Route{
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/setup",
			RequiresSession: false,
			Action:          "setup.create",
			Handler:         handler(func(w http.ResponseWriter, r *http.Request) { createFirstUser(d, w, r) }),
		},
	}
}

func createFirstUser(d httpapi.Deps, w http.ResponseWriter, r *http.Request) {
	var req setupRequest
	if err := decodeJSON(w, r, &req); err != nil {
		httpapi.WriteProblem(w, r, httpapi.Validation("The request body could not be read as a setup request."))
		return
	}

	username := strings.TrimSpace(req.Username)
	var fieldErrs []httpapi.FieldError
	if len(username) < minUsernameLen || len(username) > maxUsernameLen {
		fieldErrs = append(fieldErrs, httpapi.FieldError{
			Field:  "username",
			Reason: "must be between 3 and 64 characters",
		})
	}
	if len(req.Password) < minPasswordLen {
		fieldErrs = append(fieldErrs, httpapi.FieldError{
			Field:  "password",
			Reason: "must be at least 12 characters",
		})
	}
	if len(fieldErrs) > 0 {
		httpapi.WriteProblem(w, r, httpapi.Validation("The account details are not valid.", fieldErrs...))
		return
	}

	users, err := d.Store.Users().List(r.Context())
	if err != nil {
		httpapi.WriteInternal(w, r, d.Logger, err)
		return
	}
	if len(users) > 0 {
		// Actively refuse rather than merely hide the route (D-01). A route
		// that is only hidden is still a route; a 409 is a fact a client can
		// act on and a reviewer can test.
		httpapi.WriteProblem(w, r, httpapi.Conflict("setup.already-completed",
			"An operator account already exists. Setup can only run once."))
		return
	}

	hash, err := auth.Hash(req.Password)
	if err != nil {
		httpapi.WriteInternal(w, r, d.Logger, err)
		return
	}

	id, err := newID()
	if err != nil {
		httpapi.WriteInternal(w, r, d.Logger, err)
		return
	}

	created, err := d.Store.Users().Put(r.Context(), model.User{
		ID:           model.UserID(id),
		Username:     username,
		PasswordHash: hash,
		CreatedAt:    time.Now().UTC(),
	})
	if err != nil {
		httpapi.WriteInternal(w, r, d.Logger, err)
		return
	}

	if _, err := d.Store.Settings().Put(r.Context(), model.Settings{
		SetupCompleted: true,
		CreatedAt:      time.Now().UTC(),
	}); err != nil {
		httpapi.WriteInternal(w, r, d.Logger, err)
		return
	}

	if err := d.Auth.StartSession(r.Context(), created); err != nil {
		httpapi.WriteInternal(w, r, d.Logger, err)
		return
	}

	writeJSON(w, http.StatusCreated, setupResponse{ID: created.ID, Username: created.Username})
}

// newID returns an opaque 128-bit identifier. Identifiers are opaque so that no
// caller can derive one from a username and address another operator's record.
func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
