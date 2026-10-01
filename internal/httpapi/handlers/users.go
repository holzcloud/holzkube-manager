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
	"net/url"
	"strings"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/auth"
	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
)

// UserRoutes serves the account list and the things an admin does to one.
//
// Creating, deleting, resetting a password and unlinking single sign-on are
// Destructive: each of them changes who can reach cluster PKI, which is the
// same argument that makes the operator's own password change destructive.
// Unlinking changes it through the provider -- which identity there the next
// single sign-on resolves to -- and that is a door to the same PKI. Changing a role is Destructive
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
			// A service account and its token, in one request and one answer.
			// The token is in that answer and nowhere else, ever: only its
			// hash is stored, so a lost token is rotated rather than recovered.
			Method:          http.MethodPost,
			Pattern:         "/api/v1/service-accounts",
			RequiresSession: true,
			MinRole:         model.RoleAdmin,
			Destructive:     true,
			Action:          "service-account.create",
			Handler:         handler(createServiceAccount(d)),
		},
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/service-accounts/{id}/token",
			RequiresSession: true,
			MinRole:         model.RoleAdmin,
			Destructive:     true,
			Action:          "service-account.rotate",
			Handler:         handler(rotateServiceAccountToken(d)),
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
		{
			// Registered whether or not a provider is configured: a binding
			// left over from a provider that has since been switched off is
			// exactly the one that needs removing, and a route that existed
			// only while a provider is configured could not remove it.
			Method:          http.MethodDelete,
			Pattern:         "/api/v1/users/{id}/identity",
			RequiresSession: true,
			MinRole:         model.RoleAdmin,
			Destructive:     true,
			Action:          "user.identity-unlink",
			Handler:         handler(unlinkUserIdentity(d)),
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

	// Kind is "person" or "service". A list that did not distinguish them
	// would offer a password reset for something that has no password.
	Kind string `json:"kind"`

	// TokenIssuedAt and LastUsedAt are reported for a service account and are
	// absent for a person. Together they answer the question an operator
	// actually has about a machine credential: how old is it, and is anything
	// still using it.
	TokenIssuedAt string `json:"token_issued_at,omitempty"`
	LastUsedAt    string `json:"last_used_at,omitempty"`

	// LinkedIdentity says whether this account signs in through the identity
	// provider. The subject is never reported: it is somebody's identifier at
	// a third party. The issuer is reported only as its host, in
	// LinkedProvider, because "which provider" is what an admin deciding
	// whether to unlink needs, and the issuer's path adds nothing to that.
	LinkedIdentity bool `json:"linked_identity"`

	// LinkedProvider is the issuer's host (and port, when it has one) for a
	// linked account, and absent otherwise.
	LinkedProvider string `json:"linked_provider,omitempty"`

	// Self marks the account making the request, so an interface can refuse
	// the two things it should not offer somebody about themselves without
	// re-deriving who they are.
	Self bool `json:"self"`
}

func viewOfUser(u model.User, self model.UserID) userView {
	v := userView{
		ID:             string(u.ID),
		Username:       u.Username,
		Role:           string(u.Role.OrAdmin()),
		Kind:           string(u.Kind.OrPerson()),
		CreatedAt:      stamp(u.CreatedAt),
		LinkedIdentity: u.HasIdentityBinding(),
		Self:           u.ID == self,
	}
	if u.HasIdentityBinding() {
		v.LinkedProvider = providerHost(u.Issuer)
	}
	if u.IsService() {
		v.TokenIssuedAt = stamp(u.TokenIssuedAt)
		// Empty rather than the zero time, and the difference is what the
		// screen renders: "never used" is a fact about this credential, and
		// "1 January year 1" is a fact about Go.
		v.LastUsedAt = stamp(u.LastUsedAt)
	}
	return v
}

// providerHost is an issuer reduced to its host: never the path, and empty
// when the issuer does not parse as a URL with one.
func providerHost(issuer string) string {
	parsed, err := url.Parse(issuer)
	if err != nil {
		return ""
	}
	return parsed.Host
}

// stamp formats a time, and the empty string for one that was never set.
func stamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// createServiceAccount mints a machine identity and hands back its token once.
func createServiceAccount(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Username string `json:"username"`
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
			fieldErrs = append(fieldErrs, httpapi.FieldError{
				Field:  "username",
				Reason: "is reserved: the audit log uses it to mean a mutation the process itself initiated",
			})
		}
		if !model.UserRole(body.Role).Valid() {
			fieldErrs = append(fieldErrs, httpapi.FieldError{
				Field: "role", Reason: "must be one of admin, operator or reader",
			})
		}
		if len(fieldErrs) > 0 {
			httpapi.WriteProblem(w, r, httpapi.Validation("The service account details are not valid.", fieldErrs...))
			return
		}

		id, err := newID()
		if err != nil {
			httpapi.WriteInternal(w, r, d.Logger, err)
			return
		}

		created, token, err := d.Auth.CreateServiceAccount(r.Context(), id, username, model.UserRole(body.Role))
		if err != nil {
			writeUserError(w, r, d, err)
			return
		}

		me, _ := d.Auth.CurrentUser(r.Context())
		writeJSON(w, http.StatusCreated, map[string]any{
			"account": viewOfUser(created, me.ID),
			"token":   token,
			"notice":  TokenNotice,
		})
	}
}

// TokenNotice is what the screen says beside a freshly minted token.
//
// It is a constant because it is a statement about what this product can and
// cannot do afterwards, not UI copy: there is no route that returns a token a
// second time, because only its hash was kept.
const TokenNotice = "This is the only time this token is shown. Only its hash is stored, so it " +
	"cannot be recovered — a lost token is replaced by rotating it, which is the same act as " +
	"revoking the old one. Send it as an Authorization header: Bearer " + auth.TokenPrefix + "…"

func rotateServiceAccountToken(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, err := d.Auth.RotateToken(r.Context(), model.UserID(r.PathValue("id")))
		if err != nil {
			writeUserError(w, r, d, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"token": token, "notice": TokenNotice})
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

// unlinkUserIdentity removes an account's single sign-on binding and answers
// with the account as it now is, the same shape a role change answers with.
func unlinkUserIdentity(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		saved, err := d.Auth.UnlinkIdentity(r.Context(), model.UserID(r.PathValue("id")))
		switch {
		case errors.Is(err, auth.ErrNotLinked):
			httpapi.WriteProblem(w, r, httpapi.Conflict(httpapi.CodeNotLinked,
				"This account is not linked to single sign-on, so there is nothing to unlink."))
			return
		case err != nil:
			writeUserError(w, r, d, err)
			return
		}

		// No session is ended, this one included: a session belongs to the
		// account, and the account -- its ID, role and password -- is what it
		// was. The binding only decides where the next provider sign-in lands.
		me, _ := d.Auth.CurrentUser(r.Context())
		writeJSON(w, http.StatusOK, viewOfUser(saved, me.ID))
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
	case errors.Is(err, auth.ErrNotAServiceAccount):
		httpapi.WriteProblem(w, r, httpapi.Conflict("conflict.not-a-service-account",
			"That account is a person and has a password rather than a token. An admin can reset "+
				"their password; nobody can mint a token for them."))
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
