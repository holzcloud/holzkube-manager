package imagefactory_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/imagefactory"
)

// serverReturning stands up a server whose extension endpoint answers with a
// fixed body and status. These tests are about how the client reacts to an
// upstream that misbehaves, so the endpoint is a prop rather than a fake.
func serverReturning(t *testing.T, status int, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// TestClientRefusesAnOversizedResponse checks the cap is applied before the
// decoder sees anything. Unbounded decoding of a body this process does not
// control is a denial of service with no upside, and it is the same argument
// internal/httpapi/handlers already makes about inbound bodies.
func TestClientRefusesAnOversizedResponse(t *testing.T) {
	// A syntactically valid JSON array far past the cap. If the cap were absent
	// this would decode successfully, so the test cannot pass by accident.
	oversized := "[" + strings.Repeat(`{"name":"x","ref":"","digest":"","author":"","description":""},`, 40000)
	oversized = strings.TrimSuffix(oversized, ",") + "]"
	if len(oversized) <= 1<<20 {
		t.Fatalf("the oversized fixture is %d bytes, which is not past the cap", len(oversized))
	}

	client := newClient(t, serverReturning(t, http.StatusOK, oversized))
	_, err := client.Extensions(t.Context(), catalogVersion)

	if !errors.Is(err, imagefactory.ErrUpstreamUnavailable) {
		t.Fatalf("error = %v, want ErrUpstreamUnavailable", err)
	}
	if !strings.Contains(err.Error(), "cap") {
		t.Errorf("the error does not say the body was refused for its size: %v", err)
	}
}

// TestClientRefusesAnUnknownField makes an upstream schema change loud. A field
// silently dropped here is a field nobody notices until an operator wonders why
// a value they can see in the Factory's own API is not in holzkube-manager.
func TestClientRefusesAnUnknownField(t *testing.T) {
	body := `[{"name":"siderolabs/intel-ucode","ref":"r","digest":"d","author":"a","description":"x","surprise":1}]`
	client := newClient(t, serverReturning(t, http.StatusOK, body))

	_, err := client.Extensions(t.Context(), catalogVersion)
	if !errors.Is(err, imagefactory.ErrUpstreamUnavailable) {
		t.Fatalf("error = %v, want ErrUpstreamUnavailable", err)
	}
	if !strings.Contains(err.Error(), "surprise") {
		t.Errorf("the error does not name the unknown field: %v", err)
	}
}

// TestClientRefusesTrailingContent stops a response that is two documents from
// being read as its first one.
func TestClientRefusesTrailingContent(t *testing.T) {
	body := `[{"name":"siderolabs/intel-ucode","ref":"r","digest":"d","author":"a","description":"x"}]  {"and":"more"}`
	client := newClient(t, serverReturning(t, http.StatusOK, body))

	_, err := client.Extensions(t.Context(), catalogVersion)
	if !errors.Is(err, imagefactory.ErrUpstreamUnavailable) {
		t.Fatalf("error = %v, want ErrUpstreamUnavailable", err)
	}
	if !strings.Contains(err.Error(), "trailing content") {
		t.Errorf("the error does not say the response carried trailing content: %v", err)
	}
}

// TestClientReportsTheUpstreamStatus checks that a non-2xx is a failure and
// that the status survives into the message. The code is the difference between
// "retry later" and "this request is wrong", so losing it makes the error
// unactionable.
func TestClientReportsTheUpstreamStatus(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusBadGateway, http.StatusNotFound, http.StatusTooManyRequests} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			client := newClient(t, serverReturning(t, status, `{"error":"nope"}`))

			_, err := client.Extensions(t.Context(), catalogVersion)
			if !errors.Is(err, imagefactory.ErrUpstreamUnavailable) {
				t.Fatalf("error = %v, want ErrUpstreamUnavailable", err)
			}
			if !strings.Contains(err.Error(), fmt.Sprint(status)) {
				t.Errorf("the error does not carry the status code %d: %v", status, err)
			}
		})
	}
}

// TestClientTimesOutAndLeaksNoGoroutine covers the reason the client owns an
// explicit http.Client: http.DefaultClient has no timeout, so an upstream that
// accepts a connection and stops talking holds a goroutine for the life of the
// process.
func TestClientTimesOutAndLeaksNoGoroutine(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()

	client, err := imagefactory.New(srv.URL, imagefactory.WithTimeout(100*time.Millisecond))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	before := settledGoroutines()

	start := time.Now()
	_, err = client.Extensions(t.Context(), catalogVersion)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("a call that outlived the client timeout returned no error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want it to wrap context.DeadlineExceeded", err)
	}
	if !errors.Is(err, imagefactory.ErrUpstreamUnavailable) {
		t.Errorf("error = %v, want ErrUpstreamUnavailable", err)
	}
	if elapsed > 5*time.Second {
		t.Errorf("the call took %s; the timeout did not bound it", elapsed)
	}

	close(release)
	after := settledGoroutines()
	// A small band, not equality: the test server and the transport both keep
	// their own bookkeeping goroutines, and asserting an exact count would fail
	// for reasons that have nothing to do with a leak.
	if after > before+2 {
		t.Errorf("goroutines went from %d to %d across a timed-out call; the request did not release", before, after)
	}
}

// settledGoroutines waits for the count to stop moving so the comparison is not
// made against a transport that is still tearing down.
func settledGoroutines() int {
	last := runtime.NumGoroutine()
	for range 50 {
		time.Sleep(20 * time.Millisecond)
		now := runtime.NumGoroutine()
		if now == last {
			return now
		}
		last = now
	}
	return last
}

// TestClientDoesNotFollowACrossHostRedirect is the anti-spoofing control. A
// Factory that can bounce this client anywhere is a Factory that can have
// schematic contents -- kernel arguments and META values among them -- delivered
// to a host the operator never configured.
func TestClientDoesNotFollowACrossHostRedirect(t *testing.T) {
	var elsewhereHit bool
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		elsewhereHit = true
		_, _ = w.Write([]byte(`[]`))
	}))
	defer elsewhere.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+r.URL.Path, http.StatusFound)
	}))
	defer redirector.Close()

	client := newClient(t, redirector.URL)
	_, err := client.Extensions(t.Context(), catalogVersion)

	if err == nil {
		t.Fatal("a cross-host redirect was followed without complaint")
	}
	if elsewhereHit {
		t.Error("the client followed a redirect to a different host")
	}
	if !strings.Contains(err.Error(), "refusing a redirect") {
		t.Errorf("the error does not say a redirect was refused: %v", err)
	}
}

// TestClientFollowsASameHostRedirect is the negative control for the test
// above: the guard must stop a host change, not every redirect. Without this,
// a guard that refused everything would look identically correct.
func TestClientFollowsASameHostRedirect(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/moved", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`["v1.13.9"]`))
	})
	mux.HandleFunc("/versions", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/moved", http.StatusFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	versions, err := newClient(t, srv.URL).Versions(t.Context())
	if err != nil {
		t.Fatalf("a same-host redirect was refused: %v", err)
	}
	if len(versions) != 1 || versions[0] != "v1.13.9" {
		t.Errorf("versions = %v, want the redirected answer", versions)
	}
}

// TestExtensionsRejectsAMissingCatalog keeps "this version has no extensions"
// and "this version is not one I know" apart. Conflating them offers the
// operator an empty menu and calls it a complete list.
func TestExtensionsRejectsAMissingCatalog(t *testing.T) {
	fake := newFakeFactory(t)
	client := newClient(t, fake.URL)

	// A version the fake has no catalog for answers 404, as the real Factory
	// does for an unknown version.
	_, err := client.Extensions(t.Context(), "v1.12.0")
	if !errors.Is(err, imagefactory.ErrUpstreamUnavailable) {
		t.Fatalf("error for a version with no catalog = %v, want ErrUpstreamUnavailable", err)
	}

	// And an upstream that answers 200 with an empty array is also refused: an
	// empty catalog is not an answer this client will validate against.
	empty := newClient(t, serverReturning(t, http.StatusOK, `[]`))
	_, err = empty.Extensions(t.Context(), catalogVersion)
	if !errors.Is(err, imagefactory.ErrUpstreamUnavailable) {
		t.Fatalf("error for an empty catalog = %v, want ErrUpstreamUnavailable", err)
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("the error does not say the catalog was empty: %v", err)
	}
}

// TestNoRouteFromACatalogFailureToAPost is the structural half of the
// no-fallback rule. It is not enough that a catalog failure returns an error:
// there must be no path from that failure to a POST, because a schematic
// created against an unvalidated extension list is exactly the artefact P9
// describes -- accepted by the Factory and un-buildable afterwards.
func TestNoRouteFromACatalogFailureToAPost(t *testing.T) {
	fake := newFakeFactory(t)
	client := newClient(t, fake.URL)

	// v1.12.0 has no catalog in the fake, so the fetch fails.
	_, err := imagefactory.Author(t.Context(), client, imagefactory.AuthorRequest{
		TalosVersion: "v1.12.0",
		Arch:         imagefactory.ArchAMD64,
		Schematic:    goodSchematic(),
	})
	if err == nil {
		t.Fatal("authoring against a version with no catalog succeeded")
	}
	if !errors.Is(err, imagefactory.ErrUpstreamUnavailable) {
		t.Errorf("error = %v, want ErrUpstreamUnavailable", err)
	}

	if got := fake.count("POST /schematics"); got != 0 {
		t.Errorf("POST /schematics was called %d times after the catalog fetch failed; "+
			"a failed catalog fetch must fail validation, never fall through to creation", got)
	}
	if got := fake.count("* /image/*"); got != 0 {
		t.Errorf("the image endpoint was probed %d times for a schematic that was never created", got)
	}
}

// TestNewRejectsAnUnusableBaseURL keeps a misconfiguration from becoming a
// runtime surprise at the first request.
func TestNewRejectsAnUnusableBaseURL(t *testing.T) {
	for _, bad := range []string{"", "factory.talos.dev", "ftp://factory.talos.dev", "https://", "://x"} {
		if _, err := imagefactory.New(bad); err == nil {
			t.Errorf("New(%q) accepted an unusable base URL", bad)
		}
	}
	if _, err := imagefactory.New(imagefactory.DefaultBaseURL); err != nil {
		t.Errorf("New(DefaultBaseURL): %v", err)
	}
}

// TestPathSegmentsAreValidatedNotEscaped checks that an operator-supplied
// version or id cannot address a different endpoint than the one the code reads
// as being addressed.
func TestPathSegmentsAreValidatedNotEscaped(t *testing.T) {
	fake := newFakeFactory(t)
	client := newClient(t, fake.URL)
	ctx := t.Context()

	for _, bad := range []string{"../../schematics", "v1.13.9/../..", "v1.13.9?x=1", "latest", ""} {
		if _, err := client.Extensions(ctx, bad); err == nil {
			t.Errorf("Extensions accepted %q as a Talos version", bad)
		}
	}
	for _, bad := range []string{"../x", strings.Repeat("z", 64), "ABCDEF", ""} {
		if err := client.ProbeBuildable(ctx, bad, catalogVersion, imagefactory.ArchAMD64); err == nil {
			t.Errorf("ProbeBuildable accepted %q as a schematic id", bad)
		}
	}
	if err := client.ProbeBuildable(ctx, emptySchematicID, catalogVersion, "riscv64"); err == nil {
		t.Error("ProbeBuildable accepted an architecture the Factory does not build for")
	}
}

// TestCreateSchematicRefusesAnIDItDidNotPredict is the FACT-06 guard. A Factory
// answer that disagrees with the local computation means the two serialisations
// have drifted, so every id computed here without a round trip is suspect.
func TestCreateSchematicRefusesAnIDItDidNotPredict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprintf(w, `{"id":%q,"schematic":"customization: {}\n"}`, strings.Repeat("a", 64))
	}))
	defer srv.Close()

	created, err := newClient(t, srv.URL).CreateSchematic(t.Context(), goodSchematic())
	if !errors.Is(err, imagefactory.ErrSchematicIDMismatch) {
		t.Fatalf("error = %v, want ErrSchematicIDMismatch", err)
	}
	if !strings.Contains(err.Error(), consoleSchematicID) {
		t.Errorf("the error does not carry the locally computed id: %v", err)
	}
	// The schematic does exist upstream, so the Factory's authoritative answer
	// is still handed back for a caller that knows what to do with it.
	if created.ID == "" || created.Canonical == "" {
		t.Error("the Factory's own answer was discarded along with the mismatch")
	}
}

// TestValidateExtensionsNamesEveryUnknownAtOnce: reporting only the first turns
// fixing three typos into three round trips to an upstream that is not fast.
func TestValidateExtensionsNamesEveryUnknownAtOnce(t *testing.T) {
	catalog := []imagefactory.Extension{{Name: "siderolabs/intel-ucode"}, {Name: "siderolabs/iscsi-tools"}}

	err := imagefactory.ValidateExtensions(catalog, []string{
		"siderolabs/intel-ucode", "siderolabs/nope-one", "siderolabs/nope-two", "siderolabs/nope-one",
	})
	if !errors.Is(err, imagefactory.ErrExtensionUnknown) {
		t.Fatalf("error = %v, want ErrExtensionUnknown", err)
	}
	for _, want := range []string{"siderolabs/nope-one", "siderolabs/nope-two"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not name %s: %v", want, err)
		}
	}
	if got := strings.Count(err.Error(), "siderolabs/nope-one"); got != 1 {
		t.Errorf("a repeated unknown name is listed %d times, want once", got)
	}

	if err := imagefactory.ValidateExtensions(catalog, []string{"siderolabs/intel-ucode"}); err != nil {
		t.Errorf("a known extension was rejected: %v", err)
	}
	if err := imagefactory.ValidateExtensions(nil, nil); err != nil {
		t.Errorf("wanting nothing from an empty catalog is not an error: %v", err)
	}
	if err := imagefactory.ValidateExtensions(nil, []string{"siderolabs/intel-ucode"}); !errors.Is(err, imagefactory.ErrExtensionUnknown) {
		t.Errorf("an empty catalog validated a name; error = %v", err)
	}
}
