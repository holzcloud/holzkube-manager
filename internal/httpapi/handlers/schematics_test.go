package handlers_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/holzcloud/holzkube/internal/httpapi"
	"github.com/holzcloud/holzkube/internal/httpapi/handlers"
	"github.com/holzcloud/holzkube/internal/imagefactory"
)

// catalogVersion is the one Talos version the fake Factory serves a catalog
// for. Every other version answers 404, because that is what the real Factory
// does: the catalog is version-scoped, and a version it does not know is not an
// empty catalog.
const catalogVersion = "v1.13.9"

// baseCatalog is the catalog the fake serves for catalogVersion.
var baseCatalog = []string{
	"siderolabs/intel-ucode",
	"siderolabs/amd-ucode",
	"siderolabs/iscsi-tools",
}

// fakeFactory reproduces the one upstream behaviour that makes this whole
// feature necessary: a POST naming an extension that does not exist succeeds,
// and the refusal only surfaces when an image is requested. It is a small
// sibling of internal/imagefactory's own fake, which lives in that package's
// test binary and cannot be imported from here.
type fakeFactory struct {
	*httptest.Server

	mu        sync.Mutex
	counts    map[string]int
	documents map[string]string
	// catalog is what GET /version/<v>/extensions/official answers with.
	catalog []string
	// unbuildable names extensions the catalog lists and the image endpoint
	// nevertheless refuses. This is the upstream asymmetry itself: the catalog
	// check passes, the POST succeeds, and only the probe finds out.
	unbuildable map[string]bool
	// probeUnavailable names extensions the catalog lists and whose image
	// endpoint answers 5xx rather than refusing. That is the other not-usable
	// state: the Factory did not answer, which says nothing about the
	// schematic and must not be recorded as if it had.
	probeUnavailable map[string]bool
	// down forces every endpoint to answer 503, which is how a test says "the
	// Factory did not answer" rather than "the Factory refused".
	down bool
	// forgedID, when non-empty, is the id POST /schematics answers with
	// instead of the hash of the document. It is the FACT-06 state: a
	// well-formed 201 carrying an id the local serialiser did not predict.
	forgedID string
	// manifestStatus, when non-zero, is what every registry manifest request
	// answers with, whatever the repository name. It is how a test says "no
	// candidate carries a manifest" (404, a refusal about this schematic) or
	// "the registry did not answer the question" (503, an outage) without
	// touching the Factory endpoints the same fake serves.
	manifestStatus int
	// hijackRepo names a repository whose manifest request has its connection
	// closed with no response at all. That is the *transport* failure the
	// provisional branch needs, and it is the one shape a status code cannot
	// express -- factory.talos.dev is known to throttle exactly this way.
	hijackRepo string
}

func newFakeFactory(t *testing.T) *fakeFactory {
	t.Helper()
	f := &fakeFactory{
		counts:           map[string]int{},
		documents:        map[string]string{},
		catalog:          slices.Clone(baseCatalog),
		unbuildable:      map[string]bool{},
		probeUnavailable: map[string]bool{},
	}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Close)
	return f
}

// listButRefuse adds an extension to the catalog that the image endpoint will
// nevertheless refuse to build. It is the trap FACT-02 exists for, reproduced.
func (f *fakeFactory) listButRefuse(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.catalog = append(f.catalog, name)
	f.unbuildable[name] = true
}

// listButFailToProbe adds an extension the catalog lists and whose image
// endpoint answers 503. The schematic is created upstream and the probe learns
// nothing about it -- not a refusal, an outage.
func (f *fakeFactory) listButFailToProbe(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.catalog = append(f.catalog, name)
	f.probeUnavailable[name] = true
}

// newFactoryClient wires a client to this fake, with a short timeout so a
// misrouted request fails the test rather than stalling it.
func (f *fakeFactory) client(t *testing.T) *imagefactory.Client {
	t.Helper()
	c, err := imagefactory.New(f.URL, imagefactory.WithTimeout(5*time.Second))
	if err != nil {
		t.Fatalf("imagefactory.New: %v", err)
	}
	return c
}

func (f *fakeFactory) count(shape string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.counts[shape]
}

func (f *fakeFactory) setDown(down bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.down = down
}

func (f *fakeFactory) isDown() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.down
}

// forgeID makes every subsequent POST /schematics answer with id rather than
// with the hash of the document it was sent.
func (f *fakeFactory) forgeID(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.forgedID = id
}

// answerEveryManifestWith makes every registry manifest request answer with
// status, whatever repository name it asks about.
//
// Called after the schematic exists, never before: the create path does not
// touch the registry, and setting this first would only obscure which request
// the status belongs to.
func (f *fakeFactory) answerEveryManifestWith(status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.manifestStatus = status
}

// dropTheConnectionFor closes the connection, with no response, on a manifest
// request for repo. The resolver reads that as "this candidate was never ruled
// out" rather than as a refusal, which is the whole of the provisional branch.
func (f *fakeFactory) dropTheConnectionFor(repo string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hijackRepo = repo
}

func (f *fakeFactory) manifestBehaviour() (status int, hijack string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.manifestStatus, f.hijackRepo
}

// fakeInstallerRepos is the whitelist of registry repository names this fake
// answers a manifest for: the platform-prefixed and legacy installer names, and
// their SecureBoot counterparts.
//
// A whitelist rather than an unconditional 200 on purpose. The resolver's whole
// job is to ask the registry which name resolves, and a fake that says yes to
// every name proves nothing about it -- including that a SecureBoot request
// asks for a SecureBoot name at all, which is what G-02-4 found missing.
var fakeInstallerRepos = map[string]bool{
	string(imagefactory.PlatformMetal) + "-installer":            true,
	string(imagefactory.PlatformMetal) + "-installer-secureboot": true,
	"installer":            true,
	"installer-secureboot": true,
}

func (f *fakeFactory) serve(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")

	f.mu.Lock()
	f.counts[r.Method+" /"+parts[0]]++
	f.mu.Unlock()

	if f.isDown() {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/versions":
		writeFakeJSON(w, http.StatusOK, []string{
			"v1.12.0", "v1.13.8", "v1.13.9", "v1.14.0-alpha.1", "v1.14.0-rc.2",
		})

	case r.Method == http.MethodGet && len(parts) == 4 &&
		parts[0] == "version" && parts[2] == "extensions" && parts[3] == "official":
		if parts[1] != catalogVersion {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		f.mu.Lock()
		names := slices.Clone(f.catalog)
		f.mu.Unlock()
		catalog := make([]map[string]string, 0, len(names))
		for _, name := range names {
			catalog = append(catalog, map[string]string{
				"name": name, "ref": "ghcr.io/" + name + ":1", "digest": "sha256:0",
				"author": "Sidero Labs", "description": name,
			})
		}
		writeFakeJSON(w, http.StatusOK, catalog)

	case r.Method == http.MethodPost && r.URL.Path == "/schematics":
		f.createSchematic(w, r)

	case (r.Method == http.MethodGet || r.Method == http.MethodHead) && len(parts) == 4 && parts[0] == "image":
		f.serveImage(w, parts[1])

	case (r.Method == http.MethodGet || r.Method == http.MethodHead) && len(parts) == 5 &&
		parts[0] == "v2" && parts[3] == "manifests":
		status, hijack := f.manifestBehaviour()
		if hijack != "" && parts[1] == hijack {
			// No status, no body, no clean close: the client sees a transport
			// error, which is a different verdict from any HTTP answer.
			if hj, ok := w.(http.Hijacker); ok {
				conn, _, err := hj.Hijack()
				if err == nil {
					_ = conn.Close()
				}
			}
			return
		}
		if status != 0 {
			http.Error(w, "manifest: forced", status)
			return
		}
		if !fakeInstallerRepos[parts[1]] {
			http.Error(w, "manifest unknown", http.StatusNotFound)
			return
		}
		writeFakeJSON(w, http.StatusOK, map[string]any{"schemaVersion": 2})

	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (f *fakeFactory) createSchematic(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil || len(body) == 0 {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	sum := sha256.Sum256(body)
	id := hex.EncodeToString(sum[:])

	f.mu.Lock()
	if f.forgedID != "" {
		id = f.forgedID
	}
	f.documents[id] = string(body)
	f.mu.Unlock()

	writeFakeJSON(w, http.StatusCreated, map[string]string{"id": id, "schematic": string(body)})
}

// serveImage answers the way the Factory does: 400 for a schematic naming an
// extension the catalog does not contain, 200 otherwise.
func (f *fakeFactory) serveImage(w http.ResponseWriter, id string) {
	f.mu.Lock()
	doc, seen := f.documents[id]
	catalog := slices.Clone(f.catalog)
	unbuildable := make(map[string]bool, len(f.unbuildable))
	for k, v := range f.unbuildable {
		unbuildable[k] = v
	}
	unavailable := make(map[string]bool, len(f.probeUnavailable))
	for k, v := range f.probeUnavailable {
		unavailable[k] = v
	}
	f.mu.Unlock()

	if !seen {
		http.Error(w, "unknown schematic", http.StatusNotFound)
		return
	}
	// Only the officialExtensions block names extensions. Kernel arguments are
	// also emitted as a YAML sequence, and reading those as extension names
	// would make every schematic carrying one un-buildable in the fake and
	// nowhere else.
	inExtensions := false
	for _, line := range strings.Split(doc, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "officialExtensions:":
			inExtensions = true
			continue
		case trimmed == "":
			continue
		case !strings.HasPrefix(trimmed, "- "):
			inExtensions = false
			continue
		case !inExtensions:
			continue
		}
		name := strings.Trim(strings.TrimPrefix(trimmed, "- "), `"'`)
		if unavailable[name] {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		if !slices.Contains(catalog, name) || unbuildable[name] {
			http.Error(w, "extension not found", http.StatusBadRequest)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}

func writeFakeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// schematicServer is the full object graph with a Factory pointed at the fake.
func schematicServer(t *testing.T) (*server, *fakeFactory) {
	t.Helper()
	f := newFakeFactory(t)
	return newServerWithFactory(t, 5*time.Minute, f.client(t)), f
}

// operator is a logged-in client on a freshly set-up instance.
func operator(t *testing.T, s *server) *client {
	t.Helper()
	c := s.newClient(t)
	c.setup()
	return c
}

type problemWithErrors struct {
	Type   string `json:"type"`
	Status int    `json:"status"`
	Code   string `json:"code"`
	Errors []struct {
		Field  string `json:"field"`
		Reason string `json:"reason"`
	} `json:"errors"`
}

func decodeProblemWithErrors(t *testing.T, raw []byte) problemWithErrors {
	t.Helper()
	var p problemWithErrors
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("decode problem: %v (body: %s)", err, raw)
	}
	return p
}

func decodeInto(t *testing.T, raw []byte, dst any) {
	t.Helper()
	if err := json.Unmarshal(raw, dst); err != nil {
		t.Fatalf("decode body: %v (body: %s)", err, raw)
	}
}

// createBody is the POST /api/v1/schematics request body.
func createBody(name string, extensions, kernelArgs []string) map[string]any {
	body := map[string]any{
		"name":          name,
		"talos_version": catalogVersion,
		"arch":          string(imagefactory.ArchAMD64),
		"extensions":    extensions,
	}
	if kernelArgs != nil {
		body["kernel_args"] = kernelArgs
	}
	return body
}

// TestSchematicRoutesAreTheSevenContracted reads the route table itself. The
// flags are the contract: all seven behind a session, DELETE and only DELETE
// destructive, and an Action on both mutating routes -- a mutating route with
// an empty Action executes with no audit record at all.
func TestSchematicRoutesAreTheSevenContracted(t *testing.T) {
	s, _ := schematicServer(t)
	routes := handlers.SchematicRoutes(s.deps)

	want := []struct {
		method      string
		pattern     string
		destructive bool
		action      string
	}{
		{http.MethodGet, "/api/v1/factory/versions", false, ""},
		{http.MethodGet, "/api/v1/factory/extensions", false, ""},
		{http.MethodPost, "/api/v1/schematics", false, "schematic.create"},
		{http.MethodGet, "/api/v1/schematics", false, ""},
		{http.MethodGet, "/api/v1/schematics/{id}", false, ""},
		{http.MethodGet, "/api/v1/schematics/{id}/assets", false, ""},
		{http.MethodDelete, "/api/v1/schematics/{id}", true, "schematic.delete"},
	}

	if len(routes) != len(want) {
		t.Fatalf("SchematicRoutes returned %d routes, want %d", len(routes), len(want))
	}
	for _, w := range want {
		idx := slices.IndexFunc(routes, func(r httpapi.Route) bool {
			return r.Method == w.method && r.Pattern == w.pattern
		})
		if idx < 0 {
			t.Errorf("%s %s is not registered", w.method, w.pattern)
			continue
		}
		rt := routes[idx]
		if !rt.RequiresSession {
			t.Errorf("%s %s does not require a session", w.method, w.pattern)
		}
		if rt.Destructive != w.destructive {
			t.Errorf("%s %s Destructive = %v, want %v", w.method, w.pattern, rt.Destructive, w.destructive)
		}
		if rt.Action != w.action {
			t.Errorf("%s %s Action = %q, want %q", w.method, w.pattern, rt.Action, w.action)
		}
	}
}

// TestFactoryVersionsSplitsAndNamesTheNewestStable is FACT-05's server half:
// the answer separates prereleases structurally and newest_stable is a
// comparison, never the last element of the upstream list.
func TestFactoryVersionsSplitsAndNamesTheNewestStable(t *testing.T) {
	s, _ := schematicServer(t)
	c := operator(t, s)

	resp, raw := c.do(http.MethodGet, "/api/v1/factory/versions", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200 (body: %s)", resp.StatusCode, raw)
	}

	var got struct {
		Stable       []string          `json:"stable"`
		Prerelease   []string          `json:"prerelease"`
		NewestStable string            `json:"newest_stable"`
		Broken       map[string]string `json:"broken"`
	}
	decodeInto(t, raw, &got)

	if want := []string{"v1.12.0", "v1.13.8", "v1.13.9"}; !slices.Equal(got.Stable, want) {
		t.Errorf("stable = %v, want %v", got.Stable, want)
	}
	if want := []string{"v1.14.0-alpha.1", "v1.14.0-rc.2"}; !slices.Equal(got.Prerelease, want) {
		t.Errorf("prerelease = %v, want %v", got.Prerelease, want)
	}
	if got.NewestStable != "v1.13.9" {
		t.Errorf("newest_stable = %q, want v1.13.9 -- the last upstream element is a release candidate", got.NewestStable)
	}
	// [] and {} rather than null: a null reads to a client as "the server did
	// not check", which is a different and weaker statement.
	if !strings.Contains(string(raw), `"broken":{`) {
		t.Errorf("broken is not an object: %s", raw)
	}
	if got.Broken == nil {
		t.Error("broken decoded as nil; it must be an empty object")
	}
}

func TestFactoryVersionsRequiresASession(t *testing.T) {
	s, _ := schematicServer(t)
	owner := s.newClient(t)
	owner.setup()

	anon := s.newClient(t)
	resp, raw := anon.do(http.MethodGet, "/api/v1/factory/versions", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401 (body: %s)", resp.StatusCode, raw)
	}
}

// TestFactoryExtensionsIsVersionScoped: the catalog is served for the exact
// version asked for and there is no fallback to another one.
func TestFactoryExtensionsIsVersionScoped(t *testing.T) {
	s, _ := schematicServer(t)
	c := operator(t, s)

	resp, raw := c.do(http.MethodGet, "/api/v1/factory/extensions?version="+catalogVersion, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200 (body: %s)", resp.StatusCode, raw)
	}
	var got struct {
		Version    string `json:"version"`
		Extensions []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"extensions"`
	}
	decodeInto(t, raw, &got)
	if got.Version != catalogVersion {
		t.Errorf("version = %q, want %q", got.Version, catalogVersion)
	}
	if len(got.Extensions) != len(baseCatalog) {
		t.Fatalf("catalog has %d entries, want %d", len(got.Extensions), len(baseCatalog))
	}
	if got.Extensions[0].Name != baseCatalog[0] {
		t.Errorf("first extension = %q, want %q", got.Extensions[0].Name, baseCatalog[0])
	}
}

func TestFactoryExtensionsWithoutAVersionIsAValidationProblem(t *testing.T) {
	s, _ := schematicServer(t)
	c := operator(t, s)

	for _, query := range []string{"", "?version=", "?version=not-a-version", "?version=v1.13.9/../evil"} {
		resp, raw := c.do(http.MethodGet, "/api/v1/factory/extensions"+query, nil)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("%q: got %d, want 400 (body: %s)", query, resp.StatusCode, raw)
		}
		p := decodeProblemWithErrors(t, raw)
		if p.Code != "validation.failed" {
			t.Errorf("%q: code = %q, want validation.failed", query, p.Code)
		}
		if len(p.Errors) == 0 || p.Errors[0].Field != "version" {
			t.Errorf("%q: the problem does not name the version field: %s", query, raw)
		}
	}
}

// TestFactoryExtensionsForAnUnknownVersionIsUpstream: a catalog fetch that
// fails is a failure to validate, and it is never answered with an empty list.
func TestFactoryExtensionsForAnUnknownVersionIsUpstream(t *testing.T) {
	s, _ := schematicServer(t)
	c := operator(t, s)

	resp, raw := c.do(http.MethodGet, "/api/v1/factory/extensions?version=v1.11.0", nil)
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("got %d, want 502 (body: %s)", resp.StatusCode, raw)
	}
	p := decodeProblemWithErrors(t, raw)
	if p.Code != httpapi.CodeUpstreamFactoryUnavailable {
		t.Errorf("code = %q, want %q", p.Code, httpapi.CodeUpstreamFactoryUnavailable)
	}
	if p.Type != httpapi.ProblemBaseURI+"upstream" {
		t.Errorf("type = %q, want the upstream problem type", p.Type)
	}
}

// TestCreateRejectsUnknownExtensionsBeforeAnyPOST is T-02-35: an unvalidated
// extension name must never reach the Factory. Both unknown names are reported
// at once, and the fake records zero schematic POSTs.
func TestCreateRejectsUnknownExtensionsBeforeAnyPOST(t *testing.T) {
	s, f := schematicServer(t)
	c := operator(t, s)

	resp, raw := c.do(http.MethodPost, "/api/v1/schematics",
		createBody("bad", []string{"siderolabs/does-not-exist", "siderolabs/also-not-real"}, nil))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got %d, want 400 (body: %s)", resp.StatusCode, raw)
	}
	p := decodeProblemWithErrors(t, raw)
	if p.Code != "validation.failed" {
		t.Errorf("code = %q, want validation.failed", p.Code)
	}
	reasons := string(raw)
	for _, name := range []string{"siderolabs/does-not-exist", "siderolabs/also-not-real"} {
		if !strings.Contains(reasons, name) {
			t.Errorf("the problem does not name %q; every unknown name is reported at once: %s", name, raw)
		}
	}
	if len(p.Errors) != 2 {
		t.Errorf("errors has %d entries, want one per unknown name: %s", len(p.Errors), raw)
	}
	if n := f.count("POST /schematics"); n != 0 {
		t.Errorf("the Factory recorded %d schematic POSTs; validation must happen before any of them", n)
	}
}

// TestCreateRefusesALocallyUnrenderableValueAsAnInputProblem closes G-02-6.
//
// A control character in a kernel argument is refused by holzkube's own
// canonical serialiser, in Schematic.ID(), before the catalog is fetched and
// before any POST. The UAT saw that answered as HTTP 502 "The Image Factory did
// not answer usably" after 18.658ms -- a sentence that blames a third party,
// invites a retry that can never succeed, and never names the field.
//
// The counters are the load-bearing assertion: zero upstream requests is what
// makes "this is your input, not their outage" a fact rather than a claim.
func TestCreateRefusesALocallyUnrenderableValueAsAnInputProblem(t *testing.T) {
	// Written as an escape rather than as a literal byte, so the test source
	// stays readable and greppable. U+0007 is a bell.
	const bad = "console=ttyS0\x07"

	cases := map[string]struct {
		body      map[string]any
		wantField string
	}{
		"kernel argument": {
			body: map[string]any{
				"name": "bell", "talos_version": catalogVersion,
				"arch": string(imagefactory.ArchAMD64), "extensions": []string{},
				"kernel_args": []string{"quiet", bad},
			},
			wantField: "kernel_args",
		},
		"meta value": {
			body: map[string]any{
				"name": "bell", "talos_version": catalogVersion,
				"arch": string(imagefactory.ArchAMD64), "extensions": []string{},
				"meta": []map[string]any{{"key": 10, "value": bad}},
			},
			wantField: "meta",
		},
		"extension name": {
			body: map[string]any{
				"name": "bell", "talos_version": catalogVersion,
				"arch": string(imagefactory.ArchAMD64), "extensions": []string{bad},
			},
			wantField: "extensions",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			s, f := schematicServer(t)
			c := operator(t, s)

			resp, raw := c.do(http.MethodPost, "/api/v1/schematics", tc.body)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("got %d, want 400 (body: %s)", resp.StatusCode, raw)
			}
			p := decodeProblemWithErrors(t, raw)
			if p.Code != "validation.failed" {
				t.Errorf("code = %q, want validation.failed (body: %s)", p.Code, raw)
			}
			if p.Type != httpapi.ProblemBaseURI+"validation" {
				t.Errorf("type = %q, want the validation problem type", p.Type)
			}
			if len(p.Errors) != 1 {
				t.Fatalf("errors has %d entries, want exactly one: %s", len(p.Errors), raw)
			}
			if p.Errors[0].Field != tc.wantField {
				t.Errorf("field = %q, want %q", p.Errors[0].Field, tc.wantField)
			}
			if p.Errors[0].Reason == "" {
				t.Error("the field error carries no reason")
			}

			// The whole claim of this route's 400: nothing crossed the network,
			// so nothing about the Image Factory is being asserted (T-02-65).
			if n := f.count("POST /schematics"); n != 0 {
				t.Errorf("the Factory recorded %d schematic POSTs; this refusal is local", n)
			}
			if n := f.count("GET /version"); n != 0 {
				t.Errorf("the Factory recorded %d catalog fetches; this refusal is local", n)
			}
		})
	}
}

// TestCreateRefusalDoesNotEchoTheOffendingValue is T-02-64, asserted on the
// bytes that actually leave the process. Kernel arguments can carry secrets --
// that is why the Factory offers no way to enumerate schematics -- and a problem
// body is rendered in a browser, may be logged by a proxy, and outlives the form
// that produced it.
func TestCreateRefusalDoesNotEchoTheOffendingValue(t *testing.T) {
	const secret = "talos.config=https://example.invalid/?token=s3cr3t\x07"

	s, _ := schematicServer(t)
	c := operator(t, s)

	resp, raw := c.do(http.MethodPost, "/api/v1/schematics", map[string]any{
		"name": "secret", "talos_version": catalogVersion,
		"arch": string(imagefactory.ArchAMD64), "extensions": []string{},
		"kernel_args": []string{secret},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got %d, want 400 (body: %s)", resp.StatusCode, raw)
	}
	if strings.Contains(string(raw), "s3cr3t") {
		t.Errorf("the response body echoed the offending value: %s", raw)
	}
}

// TestCreateSplitsALocalRefusalFromAFactoryOutage pins the split from both
// sides. The 400 above is only correct if the 502 is still reachable: an
// unreachable Factory is not the operator's input problem, and reporting it as
// one would send them to edit a schematic that is fine.
func TestCreateSplitsALocalRefusalFromAFactoryOutage(t *testing.T) {
	s, f := schematicServer(t)
	c := operator(t, s)
	f.setDown(true)

	resp, raw := c.do(http.MethodPost, "/api/v1/schematics",
		createBody("fine", []string{"siderolabs/intel-ucode"}, []string{"console=ttyS0"}))
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("got %d, want 502 (body: %s)", resp.StatusCode, raw)
	}
	p := decodeProblemWithErrors(t, raw)
	if p.Code != httpapi.CodeUpstreamFactoryUnavailable {
		t.Errorf("code = %q, want %q", p.Code, httpapi.CodeUpstreamFactoryUnavailable)
	}
}

// TestCreateReturns201WithWarningsAndAProbedVerdict is the happy path, and the
// warning is FACT-04: kernel arguments reach the ISO and not the installed
// system, and the operator has to be told while they are authoring.
func TestCreateReturns201WithWarningsAndAProbedVerdict(t *testing.T) {
	s, _ := schematicServer(t)
	c := operator(t, s)

	resp, raw := c.do(http.MethodPost, "/api/v1/schematics",
		createBody("workers", []string{"siderolabs/intel-ucode"}, []string{"console=ttyS0"}))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("got %d, want 201 (body: %s)", resp.StatusCode, raw)
	}

	var got struct {
		ID           string    `json:"id"`
		Name         string    `json:"name"`
		TalosVersion string    `json:"talos_version"`
		Canonical    string    `json:"canonical"`
		Extensions   []string  `json:"extensions"`
		KernelArgs   []string  `json:"kernel_args"`
		Usable       bool      `json:"usable"`
		ProbedAt     time.Time `json:"probed_at"`
		Warnings     []struct {
			Code   string `json:"code"`
			Detail string `json:"detail"`
		} `json:"warnings"`
	}
	decodeInto(t, raw, &got)

	if len(got.ID) != 64 {
		t.Errorf("id = %q, want the 64-character Factory id", got.ID)
	}
	if !got.Usable {
		t.Errorf("usable = false for a schematic whose probe succeeded")
	}
	if got.ProbedAt.IsZero() {
		t.Error("probed_at is zero although the probe answered; zero means never probed")
	}
	if got.Canonical == "" {
		t.Error("canonical is empty; the Factory's own document is what is stored")
	}
	if len(got.Warnings) != 1 || got.Warnings[0].Code != imagefactory.WarningInstallerIgnoresKernelArgs {
		t.Fatalf("warnings = %+v, want the kernel-args warning", got.Warnings)
	}
	if !strings.Contains(got.Warnings[0].Detail, "installer") ||
		!strings.Contains(got.Warnings[0].Detail, "initramfs") {
		t.Errorf("the warning detail names neither installer nor initramfs: %q", got.Warnings[0].Detail)
	}

	// The stored record is readable back, and the list has it.
	resp, raw = c.do(http.MethodGet, "/api/v1/schematics/"+got.ID, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET by id: got %d, want 200 (body: %s)", resp.StatusCode, raw)
	}
	resp, raw = c.do(http.MethodGet, "/api/v1/schematics", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET list: got %d, want 200 (body: %s)", resp.StatusCode, raw)
	}
	var list []struct {
		ID string `json:"id"`
	}
	decodeInto(t, raw, &list)
	if len(list) != 1 || list[0].ID != got.ID {
		t.Errorf("list = %+v, want exactly the created schematic", list)
	}
}

// TestSchematicCollectionsAreArraysNeverNull: a nil slice encodes as null, and
// a null reads to a client as "the server did not say" rather than "there are
// none". The zod schemas on the other side of this contract expect arrays.
func TestSchematicCollectionsAreArraysNeverNull(t *testing.T) {
	s, _ := schematicServer(t)
	c := operator(t, s)

	// No kernel arguments and no META, so both would be nil in the record.
	resp, raw := c.do(http.MethodPost, "/api/v1/schematics",
		createBody("no-collections", []string{"siderolabs/intel-ucode"}, nil))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("got %d, want 201 (body: %s)", resp.StatusCode, raw)
	}
	for _, want := range []string{`"kernel_args":[]`, `"meta":[]`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("create response does not contain %s: %s", want, raw)
		}
	}

	var created struct {
		ID string `json:"id"`
	}
	decodeInto(t, raw, &created)

	resp, raw = c.do(http.MethodGet, "/api/v1/schematics/"+created.ID, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET: got %d, want 200 (body: %s)", resp.StatusCode, raw)
	}
	for _, want := range []string{`"kernel_args":[]`, `"meta":[]`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("stored record does not contain %s: %s", want, raw)
		}
	}
}

// TestCreateWithNothingToWarnAboutReturnsAnEmptyArray: [] and null are
// different statements and only one of them is true here.
func TestCreateWithNothingToWarnAboutReturnsAnEmptyArray(t *testing.T) {
	s, _ := schematicServer(t)
	c := operator(t, s)

	resp, raw := c.do(http.MethodPost, "/api/v1/schematics",
		createBody("plain", []string{"siderolabs/intel-ucode"}, nil))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("got %d, want 201 (body: %s)", resp.StatusCode, raw)
	}
	if !strings.Contains(string(raw), `"warnings":[]`) {
		t.Errorf("warnings is not an empty array: %s", raw)
	}
}

// TestCreateWithAFailingProbeIsStoredNotUsable is FACT-02 and PITFALLS P9: the
// Factory accepts a schematic naming an extension that does not exist, and only
// the probe finds out. The record is kept -- the schematic does exist upstream
// -- but nothing about it reads as success.
func TestCreateWithAFailingProbeIsStoredNotUsable(t *testing.T) {
	s, f := schematicServer(t)
	c := operator(t, s)

	// The catalog check has to be got past to reach the probe, so the fake
	// lists an extension whose image it will nevertheless refuse to build.
	f.listButRefuse("siderolabs/accepted-but-unbuildable")

	resp, raw := c.do(http.MethodPost, "/api/v1/schematics",
		createBody("probe-fails", []string{"siderolabs/accepted-but-unbuildable"}, nil))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("got %d, want 201 (body: %s)", resp.StatusCode, raw)
	}

	var got struct {
		ID          string    `json:"id"`
		Usable      bool      `json:"usable"`
		ProbedAt    time.Time `json:"probed_at"`
		ProbeReason string    `json:"probe_reason"`
	}
	decodeInto(t, raw, &got)
	if got.Usable {
		t.Error("usable = true although the probe refused; creation is not validation")
	}
	// A red badge with no stated cause is a verdict the operator cannot act on.
	if got.ProbeReason == "" {
		t.Error("the probe refused and said nothing; probe_reason is empty")
	}
	// The package prefix is Go's, not the operator's.
	if strings.HasPrefix(got.ProbeReason, "imagefactory:") {
		t.Errorf("probe_reason carries the package prefix: %q", got.ProbeReason)
	}

	// And it is still false when read back, not only in the create response.
	resp, raw = c.do(http.MethodGet, "/api/v1/schematics/"+got.ID, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET by id: got %d, want 200 (body: %s)", resp.StatusCode, raw)
	}
	decodeInto(t, raw, &got)
	if got.Usable {
		t.Error("the stored record says usable although the probe refused")
	}
	if got.ProbeReason == "" {
		t.Error("the stored record kept the verdict and dropped the reason for it")
	}
}

// TestUnreachableProbeRecordsNoReason keeps the two not-usable states apart at
// the field level. A probe that could not reach the Factory says nothing about
// the schematic, so a reason recorded for it would read as one -- and would send
// an operator to repair something that was never shown to be broken.
func TestUnreachableProbeRecordsNoReason(t *testing.T) {
	s, f := schematicServer(t)
	c := operator(t, s)

	f.listButFailToProbe("siderolabs/probe-unreachable")

	resp, raw := c.do(http.MethodPost, "/api/v1/schematics",
		createBody("probe-unreachable", []string{"siderolabs/probe-unreachable"}, nil))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("got %d, want 201 (body: %s)", resp.StatusCode, raw)
	}

	var got struct {
		Usable      bool      `json:"usable"`
		ProbedAt    time.Time `json:"probed_at"`
		ProbeReason string    `json:"probe_reason"`
	}
	decodeInto(t, raw, &got)
	if got.Usable {
		t.Error("usable = true although the probe never answered")
	}
	if !got.ProbedAt.IsZero() {
		t.Errorf("probed_at = %s; a probe that did not answer has not probed", got.ProbedAt)
	}
	if got.ProbeReason != "" {
		t.Errorf("probe_reason = %q; the Factory said nothing about this schematic", got.ProbeReason)
	}
}

// TestCreateStoresTheArchitectureTheProbeUsed is the second half of G-02-8.
//
// The probe verdict is scoped to one architecture -- probe_reason already names
// it inside its own sentence -- so a record that stores usable without storing
// the architecture stores a claim whose subject is missing. The UAT names the
// consequence: an operator cannot detect after the fact that a schematic was
// probed at an architecture they never chose.
//
// arm64 and not amd64 on purpose: createBody posts amd64, so an assertion
// against amd64 would pass against a hardcoded value or a defaulted one.
func TestCreateStoresTheArchitectureTheProbeUsed(t *testing.T) {
	s, _ := schematicServer(t)
	c := operator(t, s)

	body := createBody("arm-workers", []string{"siderolabs/intel-ucode"}, nil)
	body["arch"] = string(imagefactory.ArchARM64)

	resp, raw := c.do(http.MethodPost, "/api/v1/schematics", body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("got %d, want 201 (body: %s)", resp.StatusCode, raw)
	}

	var created struct {
		ID   string `json:"id"`
		Arch string `json:"arch"`
	}
	decodeInto(t, raw, &created)
	if created.Arch != string(imagefactory.ArchARM64) {
		t.Errorf("201 body arch = %q, want %q", created.Arch, imagefactory.ArchARM64)
	}

	resp, raw = c.do(http.MethodGet, "/api/v1/schematics/"+created.ID, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET by id: got %d, want 200 (body: %s)", resp.StatusCode, raw)
	}
	var readBack struct {
		Arch string `json:"arch"`
	}
	decodeInto(t, raw, &readBack)
	if readBack.Arch != string(imagefactory.ArchARM64) {
		t.Errorf("stored arch = %q, want %q; the record did not keep what the probe used",
			readBack.Arch, imagefactory.ArchARM64)
	}
}

// TestUnansweredProbeStillStoresTheArchitecture separates the two statements
// that the ProbedAt conditional three lines above the stamp invites merging.
//
// ProbedAt records whether an answer arrived; Arch records what the question
// was about. A probe that could not reach the Factory still asked about an
// architecture, and withholding it would recreate in miniature the ambiguity
// G-02-8 is about.
func TestUnansweredProbeStillStoresTheArchitecture(t *testing.T) {
	s, f := schematicServer(t)
	c := operator(t, s)

	f.listButFailToProbe("siderolabs/probe-unreachable")

	body := createBody("probe-unreachable-arm", []string{"siderolabs/probe-unreachable"}, nil)
	body["arch"] = string(imagefactory.ArchARM64)

	resp, raw := c.do(http.MethodPost, "/api/v1/schematics", body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("got %d, want 201 (body: %s)", resp.StatusCode, raw)
	}

	var got struct {
		Arch     string    `json:"arch"`
		ProbedAt time.Time `json:"probed_at"`
	}
	decodeInto(t, raw, &got)
	if !got.ProbedAt.IsZero() {
		t.Errorf("probed_at = %s; a probe that did not answer has not probed", got.ProbedAt)
	}
	if got.Arch != string(imagefactory.ArchARM64) {
		t.Errorf("arch = %q, want %q; what the probe was asked about is a fact whether or not it answered",
			got.Arch, imagefactory.ArchARM64)
	}
}

// TestTheStoredArchitectureDoesNotDefaultTheAssetsQuery pins FACT-03 against
// the obvious next thought.
//
// Reading rec.Arch as a default for a missing ?arch= is wrong for the reason
// assetRequest's own comment gives: holzkube is developed on arm64 and targets
// amd64, so a defaulted architecture is a bug that only ever appears on someone
// else's machine. The record describes what was probed; the parameter asks what
// to build. This test is the difference between a comment and a rule.
func TestTheStoredArchitectureDoesNotDefaultTheAssetsQuery(t *testing.T) {
	s, _ := schematicServer(t)
	c := operator(t, s)

	body := createBody("arm-assets", []string{"siderolabs/intel-ucode"}, nil)
	body["arch"] = string(imagefactory.ArchARM64)
	resp, raw := c.do(http.MethodPost, "/api/v1/schematics", body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: got %d, want 201 (body: %s)", resp.StatusCode, raw)
	}
	var created struct {
		ID   string `json:"id"`
		Arch string `json:"arch"`
	}
	decodeInto(t, raw, &created)
	if created.Arch != string(imagefactory.ArchARM64) {
		t.Fatalf("arch = %q, want %q -- the premise of this test", created.Arch, imagefactory.ArchARM64)
	}

	resp, raw = c.do(http.MethodGet, "/api/v1/schematics/"+created.ID+"/assets", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("assets with no arch: got %d, want 400 (body: %s)", resp.StatusCode, raw)
	}
	p := decodeProblemWithErrors(t, raw)
	if len(p.Errors) == 0 || p.Errors[0].Field != "arch" {
		t.Errorf("the problem does not name the arch field: %s", raw)
	}
}

// TestCreateAgainstAnOutageIsUpstreamAndNotInternal: an Image Factory failure
// is reported as what it is. internal.unexpected would tell the operator their
// own installation is broken.
func TestCreateAgainstAnOutageIsUpstreamAndNotInternal(t *testing.T) {
	s, f := schematicServer(t)
	c := operator(t, s)
	f.setDown(true)

	resp, raw := c.do(http.MethodPost, "/api/v1/schematics",
		createBody("outage", []string{"siderolabs/intel-ucode"}, nil))
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("got %d, want 502 (body: %s)", resp.StatusCode, raw)
	}
	p := decodeProblemWithErrors(t, raw)
	if p.Code != httpapi.CodeUpstreamFactoryUnavailable {
		t.Errorf("code = %q, want %q", p.Code, httpapi.CodeUpstreamFactoryUnavailable)
	}
	if p.Code == "internal.unexpected" {
		t.Error("an upstream outage was reported as an internal error")
	}
}

// TestCreateStoresASchematicWhoseIDWasNotPredicted is CR-02.
//
// A Factory answer carrying an id the local canonical serialiser did not
// predict means the schematic exists upstream under an id holzkube did not
// choose. The Factory offers no way to list schematics back, so a record
// dropped here is a reference an operator cannot recover — which is the whole
// reason the handler keeps the record whatever the probe said.
//
// The reporting has to be non-retryable too. The Factory answered, and
// retrying reproduces the identical mismatch, so upstream.factory-unavailable
// — documented as "the Factory did not answer" and Retryable — sends the
// operator to orphan a second schematic.
func TestCreateStoresASchematicWhoseIDWasNotPredicted(t *testing.T) {
	s, f := schematicServer(t)
	c := operator(t, s)

	const forged = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	f.forgeID(forged)

	resp, raw := c.do(http.MethodPost, "/api/v1/schematics",
		createBody("mismatched", []string{"siderolabs/intel-ucode"}, nil))
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("got %d, want 502 (body: %s)", resp.StatusCode, raw)
	}
	p := decodeProblemWithErrors(t, raw)
	if p.Code == httpapi.CodeUpstreamFactoryUnavailable {
		t.Error("an id mismatch is reported as upstream.factory-unavailable, which the contract " +
			"documents as retryable: the Factory did answer, and a retry reproduces the mismatch " +
			"while orphaning a second schematic")
	}
	if p.Code != httpapi.CodeUpstreamFactoryRejected {
		t.Errorf("code = %q, want %q", p.Code, httpapi.CodeUpstreamFactoryRejected)
	}

	// The record exists. This is the half the operator cannot reconstruct: the
	// Factory will not enumerate schematics, so an id that never reached the
	// store is gone.
	resp, raw = c.do(http.MethodGet, "/api/v1/schematics/"+forged, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET the schematic the Factory created: got %d, want 200 (body: %s)", resp.StatusCode, raw)
	}
	var stored struct {
		ID        string    `json:"id"`
		Canonical string    `json:"canonical"`
		Usable    bool      `json:"usable"`
		ProbedAt  time.Time `json:"probed_at"`
	}
	decodeInto(t, raw, &stored)
	if stored.ID != forged {
		t.Errorf("id = %q, want the Factory's own %q", stored.ID, forged)
	}
	if stored.Canonical == "" {
		t.Error("canonical is empty; a record filed under the Factory's id without the Factory's " +
			"document hashes to nothing")
	}
	if stored.Usable {
		t.Error("usable = true although no probe ran")
	}
	if !stored.ProbedAt.IsZero() {
		t.Errorf("probed_at = %s; the probe never ran", stored.ProbedAt)
	}
}

// TestCreateTheSameContentTwiceIs409NotInternal is CR-01.
//
// The schematic id is the SHA-256 of the Factory's canonical document, so two
// authoring attempts carrying the same customisation collide however they are
// named -- an operator re-submitting after a browser reload lands here, and so
// does one authoring the same extension set for a second cluster. The record
// is built with Rev left at zero, which the store reads as a CAS conflict
// against an existing record and reports as store.ErrConflict. That error is
// precise; the handler has to be the thing that says so, rather than mapping
// the one outcome the store described exactly onto the one problem type that
// carries nothing but an instance id.
func TestCreateTheSameContentTwiceIs409NotInternal(t *testing.T) {
	s, _ := schematicServer(t)
	c := operator(t, s)

	resp, raw := c.do(http.MethodPost, "/api/v1/schematics",
		createBody("first", []string{"siderolabs/intel-ucode"}, nil))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("first create: got %d, want 201 (body: %s)", resp.StatusCode, raw)
	}
	var first struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Rev  uint64 `json:"rev"`
	}
	decodeInto(t, raw, &first)

	// Identical customisation under a second label. The label is the
	// operator's own and is not part of the id.
	resp, raw = c.do(http.MethodPost, "/api/v1/schematics",
		createBody("second", []string{"siderolabs/intel-ucode"}, nil))
	if resp.StatusCode == http.StatusInternalServerError {
		t.Fatalf("the second create answered 500: a conflict the store distinguishes precisely is "+
			"reported as internal.unexpected, which tells the operator their own installation is "+
			"broken (body: %s)", raw)
	}
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("second create: got %d, want 409 (body: %s)", resp.StatusCode, raw)
	}
	p := decodeProblemWithErrors(t, raw)
	if p.Type != httpapi.TypeConflict {
		t.Errorf("type = %q, want %q", p.Type, httpapi.TypeConflict)
	}
	if p.Code != "store.conflict" {
		t.Errorf("code = %q, want store.conflict", p.Code)
	}

	// Nothing was overwritten. A POST that replaced the record would be doing
	// on a route the contract marks Destructive: false what DELETE is behind
	// the sudo window to do.
	resp, raw = c.do(http.MethodGet, "/api/v1/schematics/"+first.ID, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET by id: got %d, want 200 (body: %s)", resp.StatusCode, raw)
	}
	var stored struct {
		Name string `json:"name"`
		Rev  uint64 `json:"rev"`
	}
	decodeInto(t, raw, &stored)
	if stored.Name != "first" {
		t.Errorf("name = %q after a second create under another label, want %q: the refused create "+
			"renamed the record it refused", stored.Name, "first")
	}
	if stored.Rev != first.Rev {
		t.Errorf("rev = %d, want %d: the refused create wrote", stored.Rev, first.Rev)
	}

	// And there is still exactly one record, not two and not none.
	resp, raw = c.do(http.MethodGet, "/api/v1/schematics", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET list: got %d, want 200 (body: %s)", resp.StatusCode, raw)
	}
	var list []struct {
		ID string `json:"id"`
	}
	decodeInto(t, raw, &list)
	if len(list) != 1 || list[0].ID != first.ID {
		t.Errorf("list = %+v, want exactly the one schematic that content hashes to", list)
	}
}

// TestAssetsReturnsEveryReferenceForTheRequestedArchitecture is FACT-03: the
// architecture is a parameter, and the installer reference is resolved rather
// than assembled.
func TestAssetsReturnsEveryReferenceForTheRequestedArchitecture(t *testing.T) {
	s, _ := schematicServer(t)
	c := operator(t, s)

	id := mustCreate(t, c, "assets", []string{"siderolabs/intel-ucode"})

	for _, arch := range []string{"amd64", "arm64"} {
		resp, raw := c.do(http.MethodGet, "/api/v1/schematics/"+id+"/assets?arch="+arch, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: got %d, want 200 (body: %s)", arch, resp.StatusCode, raw)
		}
		var got struct {
			ISO       string `json:"iso"`
			PXE       string `json:"pxe"`
			DiskImage string `json:"disk_image"`
			Cmdline   string `json:"cmdline"`
			Installer string `json:"installer"`
		}
		decodeInto(t, raw, &got)
		for name, u := range map[string]string{
			"iso": got.ISO, "pxe": got.PXE, "disk_image": got.DiskImage, "cmdline": got.Cmdline,
		} {
			if u == "" {
				t.Errorf("%s: %s is empty", arch, name)
			}
			if !strings.Contains(u, "metal-"+arch) {
				t.Errorf("%s: %s = %q does not carry the requested architecture", arch, name, u)
			}
		}
		if !strings.Contains(got.Installer, "metal-installer") || !strings.Contains(got.Installer, id) {
			t.Errorf("%s: installer = %q, want the resolved reference for this schematic", arch, got.Installer)
		}
	}
}

// TestAssetsCarriesTheWarningsFieldOnTheHappyPath pins the shape rather than a
// value. The field is deliberately named `warnings`, the same name the 201 body
// uses, so a client has one shape to learn rather than two -- and it is present
// and empty rather than absent when there is nothing to say, for the reason this
// file already states about every other collection: a null reads as "the server
// did not check".
//
// The non-empty case is asserted by TestAssetsProvenAndProvisionalAnswersAreUnchanged,
// not here. This comment used to say it could not be: the branch requires a
// candidate repository that fails at the *transport* level, "which this
// package's handler fake cannot produce without hijacking its connection too".
// The premise was true and the conclusion did not follow -- hijacking is six
// lines (dropTheConnectionFor), and WINDOWS entry 23 recorded the resulting hole
// for two plans. The package-level test in internal/imagefactory still owns the
// resolver's own behaviour (TestInstallerImageWarnsWhenThePreferredNameWasNeverRuledOut);
// what the handler test owns is the wire shape that behaviour produces, which is
// a different claim and is the one a client depends on.
func TestAssetsCarriesTheWarningsFieldOnTheHappyPath(t *testing.T) {
	s, _ := schematicServer(t)
	c := operator(t, s)
	id := mustCreate(t, c, "assets-warnings", []string{"siderolabs/intel-ucode"})

	resp, raw := c.do(http.MethodGet, "/api/v1/schematics/"+id+"/assets?arch=amd64", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200 (body: %s)", resp.StatusCode, raw)
	}

	var got struct {
		Installer string `json:"installer"`
		Warnings  []struct {
			Code   string `json:"code"`
			Detail string `json:"detail"`
		} `json:"warnings"`
	}
	decodeInto(t, raw, &got)

	if got.Installer == "" {
		t.Fatal("the happy path produced no installer reference")
	}
	if len(got.Warnings) != 0 {
		t.Errorf("a proven installer name produced warnings: %+v", got.Warnings)
	}
	if !strings.Contains(string(raw), `"warnings"`) {
		t.Errorf("the assets response carries no warnings field at all: %s", raw)
	}
	if strings.Contains(string(raw), `"warnings":null`) {
		t.Errorf("warnings encoded as null, which reads as \"the server did not check\": %s", raw)
	}
}

// assetsBody is the assets response as a client that knows about the
// unresolved-installer outcome decodes it.
//
// Installer is a pointer because the field is null and never absent when the
// reference could not be resolved. That is the whole of the shape decision: a
// client written before this outcome existed fails to decode a null into a
// string rather than reading an empty string as a proven reference.
type assetsBody struct {
	ISO            string  `json:"iso"`
	PXE            string  `json:"pxe"`
	DiskImage      string  `json:"disk_image"`
	Cmdline        string  `json:"cmdline"`
	Installer      *string `json:"installer"`
	InstallerError *struct {
		Code   string `json:"code"`
		Detail string `json:"detail"`
	} `json:"installer_error"`
	Warnings []struct {
		Code   string `json:"code"`
		Detail string `json:"detail"`
	} `json:"warnings"`
}

// wantAssetURLs is the four registry-free references for a request, built the
// way the handler builds them.
//
// Derived rather than transcribed, and derived through the same package the
// handler calls. Nothing in ISOURL, PXEURL, DiskImageURL or CmdlineURL reaches
// the registry -- their only error is a validation error about the request --
// which is the fact the whole partial-answer outcome rests on. An expectation of
// "not empty" would pass on four strings assembled out of nothing, so the exact
// values are the assertion.
func wantAssetURLs(t *testing.T, base, id string, secureBoot bool) (iso, pxe, disk, cmdline string) {
	t.Helper()
	r := imagefactory.AssetRequest{
		SchematicID: id,
		Version:     catalogVersion,
		Arch:        imagefactory.ArchAMD64,
		Platform:    imagefactory.PlatformMetal,
		SecureBoot:  secureBoot,
	}
	build := func(name string, fn func(string, imagefactory.AssetRequest) (string, error)) string {
		u, err := fn(base, r)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return u
	}
	return build("iso", imagefactory.ISOURL),
		build("pxe", imagefactory.PXEURL),
		build("disk_image", imagefactory.DiskImageURL),
		build("cmdline", imagefactory.CmdlineURL)
}

// assertFourReferences checks the registry-free four against their exact values.
func assertFourReferences(t *testing.T, got assetsBody, base, id string, secureBoot bool) {
	t.Helper()
	iso, pxe, disk, cmdline := wantAssetURLs(t, base, id, secureBoot)
	for _, pair := range []struct{ name, got, want string }{
		{"iso", got.ISO, iso},
		{"pxe", got.PXE, pxe},
		{"disk_image", got.DiskImage, disk},
		{"cmdline", got.Cmdline, cmdline},
	} {
		if pair.got != pair.want {
			t.Errorf("%s = %q, want %q", pair.name, pair.got, pair.want)
		}
	}
}

// TestAssetsWithARefusedInstallerStillReturnsTheOtherFourReferences is G-02-15's
// first half, in the shape an operator actually meets it: a SecureBoot request
// at a version where no SecureBoot repository carries a manifest.
//
// The ratified half of T-02-53 is asserted here too and is not what changed: no
// ordinary installer is substituted, and `installer` is null rather than a
// plausible string. What changed is that the four references built at
// schematics.go's URL block -- pure string assembly over the request, never a
// registry call -- are no longer discarded because a fifth value could not be
// resolved.
func TestAssetsWithARefusedInstallerStillReturnsTheOtherFourReferences(t *testing.T) {
	s, f := schematicServer(t)
	c := operator(t, s)
	id := mustCreate(t, c, "installer-refused", []string{"siderolabs/intel-ucode"})

	// Every candidate answers 404: the registry answered, about this schematic,
	// and none of the names carries a manifest. That is a refusal and not an
	// outage, and the code has to say which.
	f.answerEveryManifestWith(http.StatusNotFound)

	resp, raw := c.do(http.MethodGet, "/api/v1/schematics/"+id+"/assets?arch=amd64&secureboot=true", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200 -- four references that never touched the registry were discarded "+
			"because a fifth could not be resolved (body: %s)", resp.StatusCode, raw)
	}

	var got assetsBody
	decodeInto(t, raw, &got)
	assertFourReferences(t, got, f.URL, id, true)

	if got.Installer != nil {
		t.Fatalf("installer = %q, want null: no reference resolved and none is ever assembled", *got.Installer)
	}
	if !strings.Contains(string(raw), `"installer":null`) {
		t.Errorf("installer is absent rather than null; absence is the signal a client forgets to check: %s", raw)
	}
	if got.InstallerError == nil {
		t.Fatalf("no installer_error member, so a client cannot tell an unresolved reference from a proven one: %s", raw)
	}
	if got.InstallerError.Code != httpapi.CodeUpstreamFactoryRejected {
		t.Errorf("installer_error.code = %q, want %q -- the registry answered and refused, which no retry changes",
			got.InstallerError.Code, httpapi.CodeUpstreamFactoryRejected)
	}
	if !strings.Contains(got.InstallerError.Detail, catalogVersion) {
		t.Errorf("installer_error.detail = %q, want the version named in it", got.InstallerError.Detail)
	}
	if len(got.Warnings) != 0 {
		t.Errorf("an unresolved installer produced warnings: %+v", got.Warnings)
	}
	// The one thing that must not have happened.
	if strings.Contains(string(raw), `"installer":"`) {
		t.Errorf("a reference was substituted for a SecureBoot request: %s", raw)
	}
}

// TestAssetsWithAnUnreachableRegistryStillReturnsTheOtherFourReferences is the
// same outcome from the other cause, and the reason the two have to be told
// apart: this one is retryable and the refusal is not. It is also the common
// one -- the branch is reached far more often by a registry that did not answer
// in time than by a version that genuinely has no installer.
func TestAssetsWithAnUnreachableRegistryStillReturnsTheOtherFourReferences(t *testing.T) {
	s, f := schematicServer(t)
	c := operator(t, s)
	id := mustCreate(t, c, "installer-unreachable", []string{"siderolabs/intel-ucode"})

	f.answerEveryManifestWith(http.StatusServiceUnavailable)

	resp, raw := c.do(http.MethodGet, "/api/v1/schematics/"+id+"/assets?arch=amd64", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200 (body: %s)", resp.StatusCode, raw)
	}

	var got assetsBody
	decodeInto(t, raw, &got)
	assertFourReferences(t, got, f.URL, id, false)

	if got.Installer != nil {
		t.Fatalf("installer = %q, want null", *got.Installer)
	}
	if got.InstallerError == nil {
		t.Fatalf("no installer_error member: %s", raw)
	}
	if got.InstallerError.Code != httpapi.CodeUpstreamFactoryUnavailable {
		t.Errorf("installer_error.code = %q, want %q -- the registry did not answer, which says nothing "+
			"about the schematic and is retryable",
			got.InstallerError.Code, httpapi.CodeUpstreamFactoryUnavailable)
	}
	if got.InstallerError.Detail == "" {
		t.Error("installer_error carries no detail, so the client has nothing to show but a code")
	}
}

// TestAssetsProvenAndProvisionalAnswersAreUnchanged pins the two outcomes that
// existed before this change, field by field, including the members that must
// stay *absent*.
//
// The provisional subtest also closes WINDOWS entry 23, which recorded that the
// provisional branch had no handler-level test at all. The note that used to sit
// on TestAssetsCarriesTheWarningsFieldOnTheHappyPath said this package's fake
// could not produce a transport-level failure without hijacking its connection.
// It can: dropTheConnectionFor does exactly that, in six lines, and it is the
// only way to reach a branch whose entire definition is "no HTTP answer at all".
func TestAssetsProvenAndProvisionalAnswersAreUnchanged(t *testing.T) {
	t.Run("proven", func(t *testing.T) {
		s, f := schematicServer(t)
		c := operator(t, s)
		id := mustCreate(t, c, "installer-proven", []string{"siderolabs/intel-ucode"})

		resp, raw := c.do(http.MethodGet, "/api/v1/schematics/"+id+"/assets?arch=amd64", nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("got %d, want 200 (body: %s)", resp.StatusCode, raw)
		}

		var got assetsBody
		decodeInto(t, raw, &got)
		assertFourReferences(t, got, f.URL, id, false)

		if got.Installer == nil {
			t.Fatal("installer is null on a proven answer")
		}
		if !strings.Contains(*got.Installer, "metal-installer/"+id+":"+catalogVersion) {
			t.Errorf("installer = %q, want the resolved reference for this schematic", *got.Installer)
		}
		if len(got.Warnings) != 0 {
			t.Errorf("a proven name produced warnings: %+v", got.Warnings)
		}
		// Absent, not null and not empty. A proven answer that carried the
		// member at all would make it something a client has to inspect rather
		// than something whose presence is the signal.
		if strings.Contains(string(raw), "installer_error") {
			t.Errorf("a proven answer carries an installer_error member: %s", raw)
		}
	})

	t.Run("provisional", func(t *testing.T) {
		s, f := schematicServer(t)
		c := operator(t, s)
		id := mustCreate(t, c, "installer-provisional", []string{"siderolabs/intel-ucode"})

		// The preferred name is never ruled out: the connection is dropped with
		// no answer, and the legacy name answers. Usable, but provisional.
		f.dropTheConnectionFor(string(imagefactory.PlatformMetal) + "-installer")

		resp, raw := c.do(http.MethodGet, "/api/v1/schematics/"+id+"/assets?arch=amd64", nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("got %d, want 200 (body: %s)", resp.StatusCode, raw)
		}

		var got assetsBody
		decodeInto(t, raw, &got)
		assertFourReferences(t, got, f.URL, id, false)

		if got.Installer == nil {
			t.Fatal("installer is null on a provisional answer; the reference is usable and is returned")
		}
		if !strings.Contains(*got.Installer, "/installer/"+id+":"+catalogVersion) {
			t.Errorf("installer = %q, want the legacy name that answered", *got.Installer)
		}
		if len(got.Warnings) != 1 {
			t.Fatalf("warnings = %+v, want exactly the fallback warning", got.Warnings)
		}
		if got.Warnings[0].Code != imagefactory.WarningInstallerRepoFallbackUnverified {
			t.Errorf("warnings[0].code = %q, want %q",
				got.Warnings[0].Code, imagefactory.WarningInstallerRepoFallbackUnverified)
		}
		if got.Warnings[0].Detail == "" {
			t.Error("the fallback warning carries no detail, so it says nothing to the operator reading it")
		}
		if strings.Contains(string(raw), "installer_error") {
			t.Errorf("a provisional answer carries an installer_error member: %s", raw)
		}
	})
}

func TestAssetsWithAnUnknownArchitectureIsAValidationProblem(t *testing.T) {
	s, _ := schematicServer(t)
	c := operator(t, s)
	id := mustCreate(t, c, "bad-arch", []string{"siderolabs/intel-ucode"})

	for _, query := range []string{"", "?arch=", "?arch=riscv64"} {
		resp, raw := c.do(http.MethodGet, "/api/v1/schematics/"+id+"/assets"+query, nil)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("%q: got %d, want 400 (body: %s)", query, resp.StatusCode, raw)
		}
		p := decodeProblemWithErrors(t, raw)
		if p.Code != "validation.failed" {
			t.Errorf("%q: code = %q, want validation.failed", query, p.Code)
		}
		if len(p.Errors) == 0 || p.Errors[0].Field != "arch" {
			t.Errorf("%q: the problem does not name the arch field: %s", query, raw)
		}
		// A request that fails before the URL builders returns no references at
		// all, and that is not the same thing as an unresolved installer. There
		// is nothing to return: the four references are functions of a request
		// this one never became.
		if strings.Contains(string(raw), `"iso"`) {
			t.Errorf("%q: a refused request answered with asset references: %s", query, raw)
		}
	}
}

// TestAssetsSecureBootSuffixesTheURLs is where G-02-4 was found and where it
// stays closed. The gap was not that the ISO lacked the suffix -- it had it --
// but that the installer reference in the same answer did not change at all, so
// the route handed an operator a SecureBoot ISO paired with the ordinary
// installer. Asserting the ISO alone is what let that ship, so both are
// asserted here, in one response.
func TestAssetsSecureBootSuffixesTheURLs(t *testing.T) {
	s, _ := schematicServer(t)
	c := operator(t, s)
	id := mustCreate(t, c, "secureboot", []string{"siderolabs/intel-ucode"})

	resp, raw := c.do(http.MethodGet, "/api/v1/schematics/"+id+"/assets?arch=amd64&secureboot=true", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200 (body: %s)", resp.StatusCode, raw)
	}
	var got struct {
		ISO       string `json:"iso"`
		Installer string `json:"installer"`
	}
	decodeInto(t, raw, &got)
	if !strings.Contains(got.ISO, "metal-amd64-secureboot") {
		t.Errorf("iso = %q, want the secureboot suffix", got.ISO)
	}
	if !strings.Contains(got.Installer, "/metal-installer-secureboot/") {
		t.Errorf("installer = %q, want the SecureBoot installer repository -- a SecureBoot ISO "+
			"paired with the ordinary installer is the drift this assertion exists to stop", got.Installer)
	}

	// And the ordinary request is unchanged, so the two answers are a pair
	// rather than one value that happens to carry a suffix.
	resp, raw = c.do(http.MethodGet, "/api/v1/schematics/"+id+"/assets?arch=amd64", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("without secureboot: got %d, want 200 (body: %s)", resp.StatusCode, raw)
	}
	var plain struct {
		ISO       string `json:"iso"`
		Installer string `json:"installer"`
	}
	decodeInto(t, raw, &plain)
	if strings.Contains(plain.ISO, "secureboot") || strings.Contains(plain.Installer, "secureboot") {
		t.Errorf("an ordinary request answered with SecureBoot assets: iso=%q installer=%q", plain.ISO, plain.Installer)
	}
}

func TestGetAnUnknownSchematicIs404(t *testing.T) {
	s, _ := schematicServer(t)
	c := operator(t, s)

	unknown := strings.Repeat("a", 64)
	resp, raw := c.do(http.MethodGet, "/api/v1/schematics/"+unknown, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("got %d, want 404 (body: %s)", resp.StatusCode, raw)
	}
}

// TestDeleteWithoutSudoIs428 is T-02-37: deletion is Destructive, so a live
// session is not enough. The Factory offers no way to list schematics, so the
// stored record is the only copy of a reference an upgrade needs.
func TestDeleteWithoutSudoIs428(t *testing.T) {
	s, _ := schematicServer(t)
	c := operator(t, s)
	id := mustCreate(t, c, "doomed", []string{"siderolabs/intel-ucode"})

	resp, raw := c.do(http.MethodDelete, "/api/v1/schematics/"+id, nil)
	if resp.StatusCode != http.StatusPreconditionRequired {
		t.Fatalf("got %d, want 428 (body: %s)", resp.StatusCode, raw)
	}
	p := decodeProblemWithErrors(t, raw)
	if p.Code != "sudo.required" {
		t.Errorf("code = %q, want sudo.required", p.Code)
	}
}

func TestDeleteWithAnOpenSudoWindowRemovesTheRecord(t *testing.T) {
	s, _ := schematicServer(t)
	c := operator(t, s)
	id := mustCreate(t, c, "doomed", []string{"siderolabs/intel-ucode"})

	if resp, raw := c.sudo(testPass); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("sudo: got %d, want 204 (body: %s)", resp.StatusCode, raw)
	}
	resp, raw := c.do(http.MethodDelete, "/api/v1/schematics/"+id, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: got %d, want 204 (body: %s)", resp.StatusCode, raw)
	}
	resp, raw = c.do(http.MethodGet, "/api/v1/schematics/"+id, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("after delete: got %d, want 404 (body: %s)", resp.StatusCode, raw)
	}
}

func mustCreate(t *testing.T, c *client, name string, extensions []string) string {
	t.Helper()
	resp, raw := c.do(http.MethodPost, "/api/v1/schematics", createBody(name, extensions, nil))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create %s: got %d, want 201 (body: %s)", name, resp.StatusCode, raw)
	}
	var got struct {
		ID string `json:"id"`
	}
	decodeInto(t, raw, &got)
	if got.ID == "" {
		t.Fatalf("create %s: no id in %s", name, raw)
	}
	return got.ID
}

// refusedCodepoints is the class table the live differential produced, in the
// vocabulary this layer speaks.
//
// It is a second copy of internal/imagefactory's divergingClasses, because the
// two live in different test packages and Go has no way to share one. The copy
// cannot drift, though, and the reason is the first half of the test below:
// every entry here is asserted against Schematic.ID() itself before it is ever
// POSTed. If the serialiser stops refusing one of these, this test fails at
// that assertion rather than quietly agreeing with a stale list. Plan 02-20
// extends the same table to name and cluster.
var refusedCodepoints = []struct {
	class string
	cp    rune
	// reason is the class name the problem body must carry. It is never the
	// value (T-02-64).
	reason string
}{
	{"C1 control", 0x0085, "control character"},
	{"YAML line separator", 0x2028, "line separator"},
	{"YAML paragraph separator", 0x2029, "line separator"},
	{"byte order mark", 0xFEFF, "byte order mark"},
	{"above the printable ceiling", 0x1F600, "above U+FFFD"},
}

// TestCreateRefusesAMeasuredDivergenceWithAFieldNamed400 is G-02-11's truth
// asserted at the route, which is the only place it can be asserted.
//
// A serialiser-level suite cannot stand in for this. The claim is not "ID()
// returns an error" but "the operator is told which of their inputs is wrong",
// and between the two sits createProblem, whose job is to keep a local refusal
// from being reported as somebody else's outage. U+2028 is the codepoint that
// produced the failure live: the document holzkube emitted was invalid YAML,
// the Factory answered 400, the client folded every non-2xx into
// ErrUpstreamUnavailable, and the operator was shown "The Image Factory did not
// answer usably" -- a sentence blaming a third party, telling them to retry
// something no retry can fix, for a mistake in their own kernel argument.
//
// So the absence of that sentence is asserted explicitly and not inferred from
// the status. A 400 carrying it would satisfy a status-only check, and the
// adjacent proposition passing for the named one is the failure this whole
// round is about.
func TestCreateRefusesAMeasuredDivergenceWithAFieldNamed400(t *testing.T) {
	// The sentence factoryProblem produces when the Factory did not answer.
	const unreachableFactorySentence = "The Image Factory did not answer usably"

	for _, tc := range refusedCodepoints {
		t.Run(fmt.Sprintf("%s %U", tc.class, tc.cp), func(t *testing.T) {
			bad := "console=" + string(tc.cp) + "ttyS0"

			// The binding between this table and the serialiser's. A codepoint
			// this layer claims is refused but the serialiser accepts would
			// otherwise reach the Factory and be reported as an outage.
			probe := imagefactory.Schematic{Customization: imagefactory.Customization{
				ExtraKernelArgs: []string{bad},
			}}
			if _, err := probe.ID(); !errors.Is(err, imagefactory.ErrSchematicNotRepresentable) {
				t.Fatalf("the serialiser no longer refuses %U (%v); this table and "+
					"internal/imagefactory's divergingClasses have drifted", tc.cp, err)
			}

			s, f := schematicServer(t)
			c := operator(t, s)

			// Second of two entries, so the reported position is a real
			// position and not the only one there was.
			resp, raw := c.do(http.MethodPost, "/api/v1/schematics", map[string]any{
				"name": "measured divergence", "talos_version": catalogVersion,
				"arch": string(imagefactory.ArchAMD64), "extensions": []string{},
				"kernel_args": []string{"quiet", bad},
			})

			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("got %d, want 400 (body: %s)", resp.StatusCode, raw)
			}
			if strings.Contains(string(raw), unreachableFactorySentence) {
				t.Errorf("the response blamed the Image Factory for a refusal that never "+
					"left this process: %s", raw)
			}

			p := decodeProblemWithErrors(t, raw)
			if p.Code != "validation.failed" {
				t.Errorf("code = %q, want validation.failed (body: %s)", p.Code, raw)
			}
			if len(p.Errors) != 1 {
				t.Fatalf("errors has %d entries, want exactly one: %s", len(p.Errors), raw)
			}
			if p.Errors[0].Field != "kernel_args" {
				t.Errorf("field = %q, want kernel_args", p.Errors[0].Field)
			}
			if !strings.Contains(p.Errors[0].Reason, "entry 2") {
				t.Errorf("reason = %q, want the one-based entry position", p.Errors[0].Reason)
			}
			if !strings.Contains(p.Errors[0].Reason, tc.reason) {
				t.Errorf("reason = %q, want it to name the class %q", p.Errors[0].Reason, tc.reason)
			}
			if !strings.Contains(p.Errors[0].Reason, fmt.Sprintf("%U", tc.cp)) {
				t.Errorf("reason = %q, want the %%U rendering of the codepoint", p.Errors[0].Reason)
			}
			if strings.Contains(p.Errors[0].Reason, "ttyS0") {
				t.Errorf("reason echoed the offending value: %q", p.Errors[0].Reason)
			}

			// Nothing crossed the network, so nothing about the Image Factory
			// is being asserted (T-02-65).
			if n := f.count("POST /schematics"); n != 0 {
				t.Errorf("the Factory recorded %d schematic POSTs; this refusal is local", n)
			}
		})
	}
}
