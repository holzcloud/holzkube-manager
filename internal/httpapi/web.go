package httpapi

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// The all: prefix is mandatory, not stylistic. Without it go:embed silently
// skips files whose names begin with _ or . -- which is exactly how Vite names
// its asset directory. The build would succeed and the UI would 404 at runtime.
//
//go:embed all:dist
var distFS embed.FS

// SetupPath is where an unconfigured instance sends every UI route.
const SetupPath = "/setup"

// SPAHandler serves the embedded single-page app.
//
// While no operator account exists, every UI route redirects to /setup (D-01).
// The redirect is server-side so the rule holds even if the bundle is stale or
// the client-side check is bypassed.
func SPAHandler(setupRequired func() bool) http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			WriteProblem(w, r, Internal(err))
		})
	}
	files := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := path.Clean(r.URL.Path)

		if setupRequired != nil && !isAssetPath(clean) && clean != SetupPath && setupRequired() {
			http.Redirect(w, r, SetupPath, http.StatusFound)
			return
		}

		if clean != "/" {
			if f, err := sub.Open(strings.TrimPrefix(clean, "/")); err == nil {
				_ = f.Close()
				files.ServeHTTP(w, r)
				return
			}
		}

		// Unknown non-API paths are SPA routes: hand back the shell and let the
		// client router decide.
		serveIndex(w, r, sub)
	})
}

func serveIndex(w http.ResponseWriter, r *http.Request, sub fs.FS) {
	raw, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		WriteProblem(w, r, NotFound("notfound.ui",
			"The web UI is not built into this binary. Run `task build:web` and rebuild."))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(raw)
}

// isAssetPath keeps the setup redirect from swallowing the bundle the setup
// page itself needs to load.
func isAssetPath(p string) bool {
	return strings.HasPrefix(p, "/assets/") || path.Ext(p) != ""
}
