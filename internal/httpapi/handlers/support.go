package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// The support bundle's route.
//
// It exists next to the subcommand rather than instead of it because the two
// are reached from different places: the subcommand is what somebody runs on
// the host when the interface is part of what is broken, and this is what they
// use when it is not -- during an incident an operator is in a browser looking
// at a red dashboard, and telling them to find an SSH session first is telling
// them to do the collection by hand after all.
//
// It streams, and it is marked Streaming for the same reason the etcd snapshot
// is: a bundle over a fleet is tens of megabytes, and buffering one in memory
// to satisfy a write timeout would be buffering it in the middle of the
// incident it documents.

// SupportRoutes serves the bundle.
func SupportRoutes(d httpapi.Deps) []httpapi.Route {
	return []httpapi.Route{
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/clusters/{id}/support-bundle",
			RequiresSession: true,
			Streaming:       true,
			Action:          "support.bundle",
			Handler:         handler(supportBundle(d)),
		},
	}
}

func supportBundle(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Support == nil {
			httpapi.WriteProblem(w, r, httpapi.Upstream("upstream.support-unavailable",
				"This instance was started without the support-bundle collector."))
			return
		}

		cluster := model.ClusterID(r.PathValue("id"))
		name := fmt.Sprintf("holzkube-manager-support-%s-%s.tar.gz",
			cluster, time.Now().UTC().Format("20060102T150405Z"))

		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))

		// The collector never fails on a node; it records gaps and carries on,
		// so the only error that can reach here is the inventory read. That is
		// before a byte has been written, which is why a problem document is
		// still the right answer at this point and would not be a few lines
		// further down -- appending JSON to a gzip stream would produce a file
		// that unpacks until it does not.
		man, err := d.Support.Write(r.Context(), cluster, w)
		if err != nil {
			httpapi.WriteInternal(w, r, d.Logger, err)
			return
		}

		// The gaps are logged rather than sent: the response body is the
		// archive, and the manifest inside it already carries them for whoever
		// opens it. This is for the operator watching the server's own log.
		if len(man.Incomplete) > 0 {
			d.Logger.Warn("support bundle is incomplete",
				"cluster", string(cluster), "gaps", len(man.Incomplete))
		}
	}
}
