package handlers

import (
	"net/http"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/metrics"
)

// The Prometheus export (V2-API-02).
//
// Three things about the route, each of which is a decision rather than a
// default.
//
// **It requires no session**, because a scraper has none. Prometheus sends a
// bare GET on a timer and has no way to log in, so a metrics endpoint behind
// the session cookie is a metrics endpoint nobody can scrape -- which is how
// products end up shipping a second, unauthenticated port instead. What guards
// it is what guards everything else on this listener: the host allowlist in the
// outer chain, and a bind address that is loopback unless the operator said
// otherwise.
//
// **It is not under /api/v1.** `/metrics` is where every scraper looks by
// default, and a path this product invented would have to be configured
// everywhere by hand. It also means the export is not versioned with the API,
// which is right: a metric name is its own contract and renaming one breaks
// dashboards whatever the URL says.
//
// **It carries no audit Action.** It is a read that changes nothing and is
// requested every fifteen seconds forever; recording it would fill an archive
// D-16 keeps for ever with the fact that a scraper was scraping.

// MetricsRoutes serves the Prometheus exposition.
func MetricsRoutes(d httpapi.Deps) []httpapi.Route {
	return []httpapi.Route{
		{
			Method:          http.MethodGet,
			Pattern:         "/metrics",
			RequiresSession: false,
			Handler:         handler(serveMetrics(d)),
		},
	}
}

func serveMetrics(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Metrics == nil {
			// Plain text and 503, not a problem document. A scraper does not
			// read RFC 9457; what it does with a non-200 is mark the target
			// down, which is the correct reading of an instance that cannot
			// say what its fleet looks like.
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("# this instance was started without an inventory, so it has no fleet to report on\n"))
			return
		}

		// Rendered into memory and written once (see metrics.Exporter.Write):
		// a half-written exposition is one Prometheus discards entirely, so
		// failing after the first byte would lose the metrics that had been
		// gathered along with the ones that had not.
		w.Header().Set("Content-Type", metrics.ContentType)
		if err := d.Metrics.Write(r.Context(), w); err != nil {
			httpapi.WriteInternal(w, r, d.Logger, err)
			return
		}
	}
}
