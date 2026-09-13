package handlers

// Account management (V2-AUTH-02).
//
// Every route here is admin-only, and that is the one privilege boundary in
// this product that is about the instance rather than about the fleet: an
// operator may reboot every node and may not create an account, because an
// account is how somebody else gets to reboot every node tomorrow.

import (
	"errors"
	"net/http"
	"strings"

	"github.com/holzcloud/holzkube-manager/internal/auth"
	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
)

// UserRoutes serves the account list and the four things an admin does to one.
//
// Creating, deleting and resetting a password are Destructive: each of them
// changes who can reach cluster PKI, which is the same argument that makes the
// operator's own password change destructive. Changing a role is Destructive
// for the same reason in one direction -- promotion to admin -- and marking it
// only in that direction would be a flag that depends on the body, which is
// exactly what D-06 says a marking must not be.
func UserRoutes(d httpapi.Deps) []httpapi.Route {
	return []httpapi.Route{
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/users",
			RequiresSession: true,
			MinRole:         model.RoleAdmin,
			Handler:         handler(listUsers(d)),
		},
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/users",
			RequiresSession: true,
			MinRole:         model.RoleAdmin,
			Destructive:     true,
			Action:          "user.create",
			Handler:         handler(createUser(d)),
		},
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/users/{id}/role",
			RequiresSession: true,
			MinRole:         model.RoleAdmin,
			Destructive:     true,
			Action:          "user.role",
			Handler:         handler(setUserRole(d)),
		},
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/users/{id}/password",
			RequiresSession: true,
			MinRole:         model.RoleAdmin,
			Destructive:     true,
			Action:          "user.password-reset",
			Handler:         handler(resetUserPassword(d)),
		},
		{
			Method:          http.MethodDelete,
			Pattern:         "/api/v1/users/{id}",
			RequiresSession: true,
			MinRole:         model.RoleAdmin,
			Destructive:     true,
			Action:          "user.delete",
			Handler:         handler(deleteUser(d)),
		},
	}
}

// userView is an account as the API reports it.
//
// There is no password hash and no way to ask for one. auth.Users strips it
// before this code sees it, so this type is a second statement of the same
// rule rather than the only one keeping it.
type userView struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`

	// LinkedIdentity says whether this account signs in through the identity
	// provider. The issuer and subject themselves are not reported: they are
	// somebody's identity at a third party, and "is it linked" is the whole of
	// what an admin looking at a list needs.
	LinkedIdentity bool `json:"linked_identity"`

	// Self marks the account making the request, so an interface can refuse
	// the two things it should not offer somebody about themselves without
	// re-deriving who they are.
	Self bool `json:"self"`
}

func viewOfUser(u model.User, self model.UserID) userView {
	return userView{
		ID:             string(u.ID),
		Username:       u.Username,
		Role:           string(u.Role.OrAdmin()),
		CreatedAt:      u.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		LinkedIdentity: u.HasIdentityBinding(),
		Self:           u.ID == self,
	}
}

func listUsers(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		users, err := d.Auth.Users(r.Context())
		if err != nil {
			httpapi.WriteInternal(w, r, d.Logger, err)
			return
		}

		me, _ := d.Auth.CurrentUser(r.Context())
		views := make([]userView, 0, len(users))
		for _, u := range users {
			views = append(views, viewOfUser(u, me.ID))
		}
		writeJSON(w, http.StatusOK, map[string]any{"users": views})
	}
}

func createUser(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Role     string `json:"role"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, decodeProblem(err))
			return
		}

		username := strings.TrimSpace(body.Username)
		var fieldErrs []httpapi.FieldError
		if len(username) < minUsernameLen || len(username) > maxUsernameLen {
			fieldErrs = append(fieldErrs, httpapi.FieldError{
				Field: "username", Reason: "must be between 3 and 64 characters",
			})
		}
		if isReservedActor(username) {
			// The same refusal setup makes, for the same reason: `system` is
			// how the audit archive says "the process did this", and an
			// account by that name makes every such record ambiguous forever.
			fieldErrs = append(fieldErrs, httpapi.FieldError{
				Field:  "username",
				Reason: "is reserved: the audit log uses it to mean a mutation the process itself initiated",
			})
		}
		if len(body.Password) < minPasswordLen {
			fieldErrs = append(fieldErrs, httpapi.FieldError{
				Field: "password", Reason: "must be at least 12 characters",
			})
		}
		if !model.UserRole(body.Role).Valid() {
			fieldErrs = append(fieldErrs, httpapi.FieldError{
				Field: "role", Reason: "must be one of admin, operator or reader",
			})
		}
		if len(fieldErrs) > 0 {
			httpapi.WriteProblem(w, r, httpapi.Validation("The account details are not valid.", fieldErrs...))
			return
		}

		id, err := newID()
		if err != nil {
			httpapi.WriteInternal(w, r, d.Logger, err)
			return
		}

		created, err := d.Auth.CreateUser(r.Context(), id, username, body.Password, model.UserRole(body.Role))
		if err != nil {
			writeUserError(w, r, d, err)
			return
		}

		me, _ := d.Auth.CurrentUser(r.Context())
		writeJSON(w, http.StatusCreated, viewOfUser(created, me.ID))
	}
}

func setUserRole(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Role string `json:"role"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, decodeProblem(err))
			return
		}

		me, _ := d.Auth.CurrentUser(r.Context())
		saved, err := d.Auth.SetRole(r.Context(), me.ID,
			model.UserID(r.PathValue("id")), model.UserRole(body.Role))
		if err != nil {
			writeUserError(w, r, d, err)
			return
		}
		writeJSON(w, http.StatusOK, viewOfUser(saved, me.ID))
	}
}

func resetUserPassword(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Password string `json:"password"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, decodeProblem(err))
			return
		}
		if len(body.Password) < minPasswordLen {
			httpapi.WriteProblem(w, r, httpapi.Validation("The password is not valid.",
				httpapi.FieldError{Field: "password", Reason: "must be at least 12 characters"}))
			return
		}

		if err := d.Auth.SetPassword(r.Context(), model.UserID(r.PathValue("id")), body.Password); err != nil {
			writeUserError(w, r, d, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func deleteUser(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := d.Auth.DeleteUser(r.Context(), model.UserID(r.PathValue("id"))); err != nil {
			writeUserError(w, r, d, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// writeUserError maps this domain's refusals, each to the sentence that names
// the remedy rather than the rule.
func writeUserError(w http.ResponseWriter, r *http.Request, d httpapi.Deps, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		httpapi.WriteProblem(w, r, httpapi.NotFound("notfound.user", "No such account."))
	case errors.Is(err, auth.ErrUsernameTaken):
		httpapi.WriteProblem(w, r, httpapi.Conflict("conflict.username-taken",
			"Another account already has that username."))
	case errors.Is(err, auth.ErrInvalidRole):
		httpapi.WriteProblem(w, r, httpapi.Validation("That is not a role.",
			httpapi.FieldError{Field: "role", Reason: "must be one of admin, operator or reader"}))
	case errors.Is(err, auth.ErrSelfDemotion):
		httpapi.WriteProblem(w, r, httpapi.Conflict("conflict.self-demotion",
			"An account cannot take away its own admin role. Another admin can do it — which is "+
				"also what stops this being one click away from nobody being able to undo it."))
	case errors.Is(err, auth.ErrLastAdmin):
		httpapi.WriteProblem(w, r, httpapi.Conflict("conflict.last-admin",
			"This is the only account that can manage this instance. Promote another account to "+
				"admin first; otherwise the repair for this is a shell on the host."))
	default:
		httpapi.WriteInternal(w, r, d.Logger, err)
	}
}
