package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/holzcloud/holzkube-manager/internal/auth"
	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// The links a screen in a corridor is left open on (2026-09-20).
//
// Administrator only, in both directions. Creating one hands out a credential
// that will sit on a television for months; revoking one turns a screen off from
// across the building. Neither is an operator's routine act, and the role that
// already means "may hand out the credentials that make this instance
// unnecessary" is the one that fits.

// wallLinkView is a link as a screen shows it. Never its hash.
type wallLinkView struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	CreatedAt  string `json:"created_at"`
	CreatedBy  string `json:"created_by"`
	LastUsedAt string `json:"last_used_at"`
}

// WallLinkNotice is what the screen says beside a freshly minted link.
//
// A constant, because it is a statement about what this product can and cannot
// do afterwards rather than UI copy: there is no route that returns the link a
// second time, because only its hash was kept.
const WallLinkNotice = "This is the only time this link is shown. Only its hash is stored, so it " +
	"cannot be recovered — a lost link is revoked and replaced. Anybody holding it can see the " +
	"wall, and nothing else: it opens one route, it reads, and it can never change anything."

// WallLinkRoutes are the three an administrator uses.
func WallLinkRoutes(d httpapi.Deps) []httpapi.Route {
	return []httpapi.Route{
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/wall-links",
			RequiresSession: true,
			MinRole:         model.RoleAdmin,
			Action:          "wall-link.list",
			Handler:         handler(listWallLinks(d)),
		},
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/wall-links",
			RequiresSession: true,
			MinRole:         model.RoleAdmin,

			// Destructive, and the word is right even though nothing is
			// destroyed: this hands out a credential that will live on a
			// screen for months, and D-06 exists for acts whose consequences
			// outlast the click.
			Destructive: true,
			Action:      "wall-link.create",
			Handler:     handler(createWallLink(d)),
		},
		{
			Method:          http.MethodDelete,
			Pattern:         "/api/v1/wall-links/{id}",
			RequiresSession: true,
			MinRole:         model.RoleAdmin,

			// Destructive because it turns a screen off from across the
			// building, and because a revocation done by accident is only
			// noticed by whoever walks past the wall.
			Destructive: true,
			Action:      "wall-link.revoke",
			Handler:     handler(revokeWallLink(d)),
		},
	}
}

func listWallLinks(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		links, err := d.Auth.WallLinks(r.Context())
		if err != nil {
			httpapi.WriteInternal(w, r, d.Logger, err)
			return
		}

		out := make([]wallLinkView, 0, len(links))
		for _, link := range links {
			out = append(out, wallLinkView{
				ID:         link.ID,
				Label:      link.Label,
				CreatedAt:  stamp(link.CreatedAt),
				CreatedBy:  link.CreatedBy,
				LastUsedAt: stamp(link.LastUsedAt),
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"links": out})
	}
}

func createWallLink(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Label string `json:"label"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, decodeProblem(err))
			return
		}

		label := strings.TrimSpace(body.Label)
		if label == "" || len(label) > 64 {
			// The label is what somebody reads when deciding which link to
			// revoke. A list of unnamed credentials is a list nobody can act on.
			httpapi.WriteProblem(w, r, httpapi.Validation(
				"The wall link details are not valid.",
				httpapi.FieldError{
					Field:  "label",
					Reason: "say which screen this is for, in up to 64 characters",
				}))
			return
		}

		me, _ := d.Auth.CurrentUser(r.Context())
		link, token, err := d.Auth.CreateWallLink(r.Context(), label, me.Username)
		if err != nil {
			writeWallLinkError(w, r, d, err)
			return
		}

		writeJSON(w, http.StatusCreated, map[string]any{
			"link": wallLinkView{
				ID:        link.ID,
				Label:     link.Label,
				CreatedAt: stamp(link.CreatedAt),
				CreatedBy: link.CreatedBy,
			},
			"token":  token,
			"notice": WallLinkNotice,
		})
	}
}

func revokeWallLink(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := d.Auth.RevokeWallLink(r.Context(), r.PathValue("id")); err != nil {
			writeWallLinkError(w, r, d, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func writeWallLinkError(w http.ResponseWriter, r *http.Request, d httpapi.Deps, err error) {
	switch {
	case errors.Is(err, auth.ErrNoSuchWallLink):
		httpapi.WriteProblem(w, r, httpapi.NotFound("notfound.record", "No such wall link."))
	case errors.Is(err, auth.ErrTooManyWallLinks):
		httpapi.WriteProblem(w, r, &httpapi.Problem{
			Type:   httpapi.TypeConflict,
			Title:  "There are already as many wall links as this allows",
			Status: http.StatusConflict,
			Detail: "Every request carrying a link is compared against all of them, and a list " +
				"nobody prunes is a list of credentials nobody has looked at. Revoke one that is " +
				"no longer on a screen.",
			Code: httpapi.CodeRefusedKind,
		})
	case errors.Is(err, auth.ErrInvalidWallLink):
		httpapi.WriteProblem(w, r, httpapi.Validation(err.Error()))
	default:
		httpapi.WriteInternal(w, r, d.Logger, err)
	}
}
