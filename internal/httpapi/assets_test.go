package httpapi_test

import (
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
)

// TestNoAssetIsInlinedPastTheContentSecurityPolicy.
//
// The bundle and the policy are written in two different languages by two
// different tools, and nothing made them agree. The server serves
// `font-src 'self'`; the bundler inlines any asset under its size limit as a
// `data:` URI; and the two smallest font subsets -- the Cyrillic-ext faces of
// both families -- fell under it. So the browser refused to load exactly those
// two and fell back for any text needing them.
//
// Nothing failed. The page rendered, the requests were never made, the server
// logged nothing, and every test in this repository passed. It is visible in
// one place: the console of a browser that has loaded the real bundle, which
// is where it was found.
//
// The fix is on the build side -- assetsInlineLimit: 0 -- because the policy is
// the tight one and it is the build that should bend. This holds that: if an
// inlined asset comes back, the CSP that would have to permit it is a weaker
// CSP, and this fails instead.
func TestNoAssetIsInlinedPastTheContentSecurityPolicy(t *testing.T) {
	t.Parallel()

	// The policy, read from the function that produces it rather than
	// restated. A guard that wrote out its own copy of the directives would
	// keep passing while the real ones moved.
	csp := httpapi.ContentSecurityPolicy()
	if csp == "" {
		t.Fatal("there is no Content-Security-Policy, so this guard is checking nothing")
	}

	// Which schemes the policy permits, per directive. Only the ones a bundler
	// can produce are interesting.
	permits := func(directive, scheme string) bool {
		for _, part := range strings.Split(csp, ";") {
			part = strings.TrimSpace(part)
			if name, rest, ok := strings.Cut(part, " "); ok && name == directive {
				return strings.Contains(rest, scheme)
			}
		}
		return false
	}

	assets := httpapi.EmbeddedAssets()
	if len(assets) == 0 {
		t.Fatal("the embedded bundle has no files; this guard is not reading it")
	}

	checked := 0
	for name, body := range assets {
		if !strings.HasSuffix(name, ".css") && !strings.HasSuffix(name, ".js") {
			continue
		}
		checked++

		for _, kind := range []struct{ prefix, directive string }{
			{"data:font/", "font-src"},
			{"data:image/", "img-src"},
			{"data:application/font", "font-src"},
		} {
			if !strings.Contains(body, kind.prefix) {
				continue
			}
			if permits(kind.directive, "data:") {
				continue
			}
			t.Errorf("%s inlines %s as a data: URI and the policy's %s does not permit data:. "+
				"The browser refuses it silently -- nothing errors, the asset is simply not "+
				"there. Either stop inlining it (build.assetsInlineLimit) or widen the policy "+
				"deliberately.\n  policy: %s", name, strings.TrimSuffix(kind.prefix, "/"),
				kind.directive, csp)
		}
	}

	if checked == 0 {
		t.Fatal("no CSS or JS was examined; the bundle is missing or this guard stopped finding it")
	}
}

// TestFontsAreServedAsFiles is the positive half.
//
// A guard that only refuses data: URIs would pass against a bundle with no
// fonts at all, which is the other way for the typeface to go missing.
func TestFontsAreServedAsFiles(t *testing.T) {
	t.Parallel()

	fonts := 0
	for name := range httpapi.EmbeddedAssets() {
		if strings.HasSuffix(name, ".woff2") {
			fonts++
		}
	}
	if fonts == 0 {
		t.Error("the bundle carries no font files, so either the fonts went away or they went " +
			"back to being inlined")
	}
}
