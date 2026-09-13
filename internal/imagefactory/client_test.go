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

// TestClientReportsAnUnknownFieldAndStillAnswers is the two halves of what
// replaced a refusal.
//
// The Factory adds fields without telling anyone, and this catalog is read on
// every visit to the Images screen. Refusing the response meant one additive
// upstream field took that screen down for everybody until a new
// holzkube-manager shipped. Decoding past it silently would be the other
// mistake: nobody would find out until an operator asked why a value visible
// in the Factory's own API is missing here. So the answer arrives and the
// addition is reported.
func TestClientReportsAnUnknownFieldAndStillAnswers(t *testing.T) {
	body := `[{"name":"siderolabs/intel-ucode","ref":"r","digest":"d","author":"a","description":"x","surprise":1}]`

	var gotPath, gotField string
	client := newClient(t, serverReturning(t, http.StatusOK, body),
		imagefactory.WithDriftObserver(func(path, field string) {
			gotPath, gotField = path, field
		}))

	catalog, err := client.Extensions(t.Context(), catalogVersion)
	if err != nil {
		t.Fatalf("an additive upstream field took the extension catalog down: %v", err)
	}
	if len(catalog) != 1 || catalog[0].Name != "siderolabs/intel-ucode" {
		t.Fatalf("catalog = %+v, want the one extension the response carried", catalog)
	}
	if catalog[0].Ref != "r" || catalog[0].Description != "x" {
		t.Errorf("the fields this package does know were not decoded: %+v", catalog[0])
	}

	if gotField != "surprise" {
		t.Errorf("the drift observer was told %q, want the unknown field's name; an addition "+
			"nobody is told about is one nobody finds out about", gotField)
	}
	if !strings.Contains(gotPath, "extensions") {
		t.Errorf("the drift observer was told path %q, which does not say which response drifted", gotPath)
	}
}

// TestAnUnknownFieldIsStillRefusedWhenTheRestIsNotJSON keeps the tolerance from
// swallowing a genuinely broken response.
func TestAnUnknownFieldIsStillRefusedWhenTheRestIsNotJSON(t *testing.T) {
	client := newClient(t, serverReturning(t, http.StatusOK, `[{"name":"x","surprise":1,}]`))

	if _, err := client.Extensions(t.Context(), catalogVersion); !errors.Is(err, imagefactory.ErrUpstreamUnavailable) {
		t.Fatalf("error = %v, want ErrUpstreamUnavailable for a malformed body", err)
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

// The three budgets, and the promise the client-wide timeout used to keep by
// accident.
//
// http.Client.Timeout is one value for every request a client makes. That is
// why the ISO probe -- which makes the Factory build a ~335MB image
// synchronously, measured cold at 30.50 to 32.69 seconds -- was bounded by a
// number sized against a list of extensions, and why it produced no verdict on
// every one of those five runs. Removing the field is what makes three budgets
// expressible; requireDeadline is what stops the removal turning a forgotten
// wrapper into an unbounded request.

// TestClientCarriesNoClientWideTimeout asserts the removal behaviourally rather
// than by grepping for an absent assignment. A grep proves a string is not in a
// file; this proves the value the client runs with.
func TestClientCarriesNoClientWideTimeout(t *testing.T) {
	plain, err := imagefactory.New(imagefactory.DefaultBaseURL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := plain.HTTPClientTimeoutForTest(); got != 0 {
		t.Errorf("http.Client.Timeout = %s on a default client, want 0; one client-wide value "+
			"cannot express three budgets, and the one it used to express bounded the ISO "+
			"probe with a number sized against a JSON list", got)
	}

	// The supplied client carries a timeout of its own, which is exactly the
	// shape WINDOWS entry 8 (WR-01) recorded being silently discarded. It is
	// now neither honoured nor discarded: it is not the mechanism.
	supplied, err := imagefactory.New(imagefactory.DefaultBaseURL,
		imagefactory.WithHTTPClient(&http.Client{Timeout: 7 * time.Second}))
	if err != nil {
		t.Fatalf("New(WithHTTPClient): %v", err)
	}
	if got := supplied.HTTPClientTimeoutForTest(); got != 0 {
		t.Errorf("http.Client.Timeout = %s through WithHTTPClient, want 0; the budgets are not "+
			"carried on the copied client", got)
	}
}

// TestRequestWithNoDeadlineIsRefusedBeforeTheWire is the other half. With no
// client-wide timeout, a request built on a deadline-less context would run
// until the process ends, so the refusal has to happen before anything is sent
// -- which is what the request count asserts.
func TestRequestWithNoDeadlineIsRefusedBeforeTheWire(t *testing.T) {
	fake := newFakeFactory(t)
	client := newClient(t, fake.URL)

	// context.Background and not t.Context(): both are deadline-less today,
	// and this test is about the deadline-less case specifically rather than
	// about whatever the harness happens to hand out.
	ctx := context.Background()

	t.Run("probeStatus", func(t *testing.T) {
		if _, err := client.ProbeStatusUnbudgetedForTest(ctx, fake.URL+"/image/x/y/z"); !errors.Is(err, imagefactory.ErrNoDeadline) {
			t.Fatalf("err = %v, want ErrNoDeadline", err)
		}
		if got := fake.count("* /image/*"); got != 0 {
			t.Errorf("the image endpoint saw %d requests; a refused call must reach no wire at all", got)
		}
	})

	t.Run("do", func(t *testing.T) {
		if err := client.DoUnbudgetedForTest(ctx, fake.URL+"/versions"); !errors.Is(err, imagefactory.ErrNoDeadline) {
			t.Fatalf("err = %v, want ErrNoDeadline", err)
		}
		if got := fake.count("GET /versions"); got != 0 {
			t.Errorf("/versions saw %d requests; a refused call must reach no wire at all", got)
		}
	})
}

// TestBudgetOptionsRejectNonPositiveValues keeps the three options in one
// register. Zero would mean "no budget", which after the removal of the
// client-wide timeout means "no bound at all" -- the state requireDeadline
// exists to refuse, arrived at through configuration instead of through a
// forgotten wrapper.
func TestBudgetOptionsRejectNonPositiveValues(t *testing.T) {
	options := map[string]func(time.Duration) imagefactory.Option{
		"WithTimeout":         imagefactory.WithTimeout,
		"WithProbeTimeout":    imagefactory.WithProbeTimeout,
		"WithManifestTimeout": imagefactory.WithManifestTimeout,
	}
	for name, opt := range options {
		t.Run(name, func(t *testing.T) {
			for _, bad := range []time.Duration{0, -time.Second} {
				if _, err := imagefactory.New(imagefactory.DefaultBaseURL, opt(bad)); err == nil {
					t.Errorf("%s(%s) was accepted", name, bad)
				}
			}
			if _, err := imagefactory.New(imagefactory.DefaultBaseURL, opt(time.Second)); err != nil {
				t.Errorf("%s(1s): %v", name, err)
			}
		})
	}
}

// TestEachBudgetBoundsItsOwnWorkloadAndNoOther is the package-level statement of
// what the three constants are for: a workload is bounded by its own budget, and
// moving another one does not move it.
//
// Each case gives the workload under test a budget the fake's latency fits
// inside and gives the other two a budget it does not, so a call that took the
// wrong constant fails. The latencies are milliseconds because the ratio is
// what is being asserted, not the size.
func TestEachBudgetBoundsItsOwnWorkloadAndNoOther(t *testing.T) {
	const (
		roomy  = 2 * time.Second
		narrow = 20 * time.Millisecond
		delay  = 200 * time.Millisecond
	)

	t.Run("the ISO probe takes the probe budget", func(t *testing.T) {
		fake := newFakeFactory(t)
		client := newClientWithBudgets(t, fake.URL, narrow, roomy, narrow)
		// The schematic was never POSTed to this fake, so the image endpoint
		// would 404 on it. The status is forced because the question here is
		// which budget bounded the request, not what the Factory thought of the
		// schematic.
		fake.setISOStatus(http.StatusOK)
		fake.answerAfter("HEAD /image", delay)

		if err := client.ProbeBuildable(t.Context(), schematicA, catalogVersion, imagefactory.ArchAMD64); err != nil {
			t.Fatalf("ProbeBuildable: %v -- the probe was bounded by something other than the probe budget", err)
		}
	})

	t.Run("a manifest GET takes the manifest budget", func(t *testing.T) {
		fake := newFakeFactory(t)
		client := newClientWithBudgets(t, fake.URL, narrow, narrow, roomy)
		fake.answerAfter("GET /v2", delay)

		ref, _, err := client.InstallerImage(t.Context(), imagefactory.AssetRequest{
			SchematicID: schematicA,
			Version:     installerModernVersion,
			Arch:        imagefactory.ArchAMD64,
			Platform:    imagefactory.PlatformMetal,
		})
		if err != nil {
			t.Fatalf("InstallerImage: %v -- the manifest GET was bounded by something other than the manifest budget", err)
		}
		if ref == "" {
			t.Error("InstallerImage resolved no reference")
		}
	})

	t.Run("a JSON endpoint takes the JSON budget", func(t *testing.T) {
		fake := newFakeFactory(t)
		client := newClientWithBudgets(t, fake.URL, roomy, narrow, narrow)
		fake.answerAfter("GET /versions", delay)

		if _, err := client.Versions(t.Context()); err != nil {
			t.Fatalf("Versions: %v -- the JSON endpoint was bounded by something other than the JSON budget", err)
		}
	})

	t.Run("a JSON budget no longer bounds the probe", func(t *testing.T) {
		// The defect itself: the probe outruns the JSON budget and stays well
		// inside its own. Before the split this produced ErrUpstreamUnavailable
		// and therefore no verdict.
		fake := newFakeFactory(t)
		client := newClientWithBudgets(t, fake.URL, narrow, roomy, narrow)
		fake.setISOStatus(http.StatusOK)
		fake.answerAfter("HEAD /image", delay)

		if err := client.ProbeBuildable(t.Context(), schematicA, catalogVersion, imagefactory.ArchAMD64); errors.Is(err, imagefactory.ErrUpstreamUnavailable) {
			t.Fatalf("a probe %s long produced %v against a %s probe budget; this is G-02-1", delay, err, roomy)
		}
	})
}
