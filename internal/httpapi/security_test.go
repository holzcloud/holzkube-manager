package httpapi_test

import (
	"crypto/sha256"
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube/internal/auth"
	"github.com/holzcloud/holzkube/internal/httpapi"
	"github.com/holzcloud/holzkube/internal/store/fsstore"
)

// minimalDeps is enough of a wiring to build the handler; these tests only
// exercise headers, which the outermost link sets before anything else runs.
func minimalDeps(t *testing.T) httpapi.Deps {
	t.Helper()

	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	st, err := fsstore.Open(dir)
	if err != nil {
		t.Fatalf("fsstore.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	au, err := auth.New(st, 24*time.Hour)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	return httpapi.Deps{
		Store:      st,
		Auth:       au,
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		SudoWindow: 5 * time.Minute,
		AuditChain: httpapi.ChainStatus{OK: true},
	}
}

var inlineRE = regexp.MustCompile(`(?s)<script\b([^>]*)>(.*?)</script>`)

// TestCSPHashMatchesTheServedShell is the check that keeps the policy honest.
// A script-src hash that does not match the bytes actually served makes the
// browser refuse the pre-paint theme script, and nothing in any server log says
// so -- the console simply renders wrong.
func TestCSPHashMatchesTheServedShell(t *testing.T) {
	srv := httptest.NewServer(httpapi.New(minimalDeps(t)))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	csp := resp.Header.Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("no Content-Security-Policy on the SPA shell")
	}

	found := 0
	for _, m := range inlineRE.FindAllStringSubmatch(string(body), -1) {
		if strings.Contains(m[1], "src=") {
			continue
		}
		found++
		sum := sha256.Sum256([]byte(m[2]))
		want := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
		if !strings.Contains(csp, want) {
			t.Errorf("inline script #%d hashes to %s, which script-src does not carry.\nCSP: %s",
				found, want, csp)
		}
	}
	if found == 0 {
		t.Error("no inline script found in the served shell; the extraction no longer matches the markup")
	}
	if strings.Contains(csp, "script-src") && strings.Contains(csp, "'unsafe-inline'") {
		// style-src legitimately carries it; script-src must not.
		scriptSrc := ""
		for _, d := range strings.Split(csp, "; ") {
			if strings.HasPrefix(d, "script-src ") {
				scriptSrc = d
			}
		}
		if strings.Contains(scriptSrc, "'unsafe-inline'") {
			t.Errorf("script-src carries 'unsafe-inline': %s", scriptSrc)
		}
	}
}

// TestSecurityHeadersCoverStaticAssetsToo pins the position of the link: it is
// outermost, so the file server's responses carry the headers as well.
func TestSecurityHeadersCoverStaticAssets(t *testing.T) {
	srv := httptest.NewServer(httpapi.New(minimalDeps(t)))
	t.Cleanup(srv.Close)

	for _, path := range []string{"/", "/api/v1/nope"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("get %s: %v", path, err)
		}
		resp.Body.Close()
		for header, want := range map[string]string{
			"X-Content-Type-Options": "nosniff",
			"X-Frame-Options":        "DENY",
			"Referrer-Policy":        "no-referrer",
		} {
			if got := resp.Header.Get(header); got != want {
				t.Errorf("%s: %s = %q, want %q", path, header, got, want)
			}
		}
		if resp.Header.Get("Content-Security-Policy") == "" {
			t.Errorf("%s: no Content-Security-Policy", path)
		}
		// HSTS must not be sent over plain HTTP.
		if got := resp.Header.Get("Strict-Transport-Security"); got != "" {
			t.Errorf("%s: HSTS sent over plain HTTP: %q", path, got)
		}
	}
}
