package httpapi

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"io/fs"
	"net/http"
	"path"
	"regexp"
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
func SPAHandler(setupRequired func(*http.Request) bool) http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			WriteProblem(w, r, Internal(err))
		})
	}
	files := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := path.Clean(r.URL.Path)

		if setupRequired != nil && !isAssetPath(clean) && clean != SetupPath && setupRequired(r) {
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

// inlineScriptSources returns CSP source tokens for every inline <script> in
// the embedded index.html.
//
// The hash is computed from the bytes this binary actually serves rather than
// being written down somewhere. A hardcoded digest would drift the first time
// anyone edited index.html, and the failure mode is the worst kind: the browser
// silently refuses the pre-paint theme script and the console renders wrong,
// with nothing in any server log to say so.
//
// index.html carries one inline script -- the dark-first theme applied before
// the first paint (D-11) -- and one external module. Only scripts without a
// src attribute need a hash; the module is covered by 'self'.
func inlineScriptSources(sub fs.FS) []string {
	raw, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		return nil
	}
	var out []string
	for _, m := range inlineScriptRE.FindAllSubmatch(raw, -1) {
		if bytes.Contains(m[1], []byte("src=")) {
			continue
		}
		sum := sha256.Sum256(m[2])
		out = append(out, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'")
	}
	return out
}

var inlineScriptRE = regexp.MustCompile(`(?s)<script\b([^>]*)>(.*?)</script>`)

// ContentSecurityPolicy builds the policy for the embedded UI.
//
// script-src carries the hash of the one inline script rather than
// 'unsafe-inline', so an injected <script> is refused even though the shell has
// one of its own. style-src keeps 'unsafe-inline' because the component library
// sets inline styles; that is the weaker half and is the reason the script half
// is worth getting right.
func ContentSecurityPolicy() string {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return ""
	}
	script := append([]string{"'self'"}, inlineScriptSources(sub)...)
	return strings.Join([]string{
		"default-src 'self'",
		"script-src " + strings.Join(script, " "),
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data:",
		"font-src 'self'",
		"connect-src 'self'",
		"frame-ancestors 'none'",
		"base-uri 'none'",
		"form-action 'none'",
		"object-src 'none'",
	}, "; ")
}
