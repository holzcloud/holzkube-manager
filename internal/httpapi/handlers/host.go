package handlers

import (
	"net/http"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// The machine holzkube-manager runs on (HOST-01, Phase 11).
//
// One route, and each of its settings is a decision rather than a default.
//
// **It requires a session and a reader.** The answer names the hostname, the
// board and the kernel of the machine that holds every cluster's secrets. A
// reader already sees the version and the whole cluster topology, so a reader
// may see this too; somebody who has not signed in may not.
//
// **It carries no audit Action.** It is a read that changes nothing, and an open
// page asks for it every three seconds; recording it would fill an archive D-16
// keeps for ever with the fact that somebody had a tab open. The same reasoning
// as /metrics.
//
// **It is not a wall-link route.** The wall's host tile is Phase 12, and a
// credential for a screen opens only what that screen shows.
//
// It reaches no node and no upstream service: everything it answers is read
// from this process's own namespace, through a handful of files and four
// syscalls (internal/host). That is why it is in budget_test.go's list of
// routes that reach nothing.

// HostRoutes serves the host page's one read.
func HostRoutes(d httpapi.Deps) []httpapi.Route {
	return []httpapi.Route{
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/host",
			RequiresSession: true,
			MinRole:         model.RoleReader,
			Handler:         handler(readHost(d)),
		},
	}
}

func readHost(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Host == nil {
			httpapi.WriteProblem(w, r, httpapi.Upstream("upstream.host-unavailable",
				"This instance was started without a host reader."))
			return
		}
		writeJSON(w, http.StatusOK, d.Host.Read(r.Context()))
	}
}
