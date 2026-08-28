package imagefactory_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// catalogVersion is the Talos version the recorded extension catalog belongs
// to. The fake serves a catalog for this version and 404s for every other one,
// because that is what the real Factory does -- the catalog is version-scoped,
// and a version it does not know is not an empty catalog.
const catalogVersion = "v1.13.9"

// fakeFactory is an Image Factory that reproduces the documented upstream
// behaviours, including the one that makes this whole package necessary: a POST
// naming an extension that does not exist succeeds, and the failure only
// surfaces when an image is requested.
//
// It is deliberately not a friendly stub. Every response shape here was
// observed against factory.talos.dev, and the trap is reproduced faithfully so
// that a client which mistakes creation for validation fails against the fake
// exactly as it would fail against a real machine.
type fakeFactory struct {
	*httptest.Server

	mu sync.Mutex
	// counts requests per "METHOD /path-shape", so a test can assert that a
	// request was never made rather than only that it failed.
	counts map[string]int
	// documents maps a schematic id to the document that produced it.
	documents map[string]string
	// known extension names for catalogVersion.
	known map[string]struct{}

	// versions and catalog are the recorded upstream answers.
	versionsJSON []byte
	catalogJSON  []byte

	// isoStatus, when non-zero, overrides the ISO answer.
	isoStatus int
}

func newFakeFactory(t *testing.T) *fakeFactory {
	t.Helper()

	f := &fakeFactory{
		counts:       map[string]int{},
		documents:    map[string]string{},
		known:        map[string]struct{}{},
		versionsJSON: readTestdata(t, "versions.json"),
		catalogJSON:  readTestdata(t, "extensions-"+catalogVersion+".json"),
	}

	var catalog []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(f.catalogJSON, &catalog); err != nil {
		t.Fatalf("decode recorded catalog: %v", err)
	}
	if len(catalog) == 0 {
		t.Fatal("the recorded catalog is empty; the fake would validate nothing")
	}
	for _, e := range catalog {
		f.known[e.Name] = struct{}{}
	}

	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Close)
	return f
}

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read testdata/%s: %v", name, err)
	}
	return raw
}

// count returns how many times a request shape was seen.
func (f *fakeFactory) count(shape string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.counts[shape]
}

func (f *fakeFactory) record(shape string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.counts[shape]++
}

// setISOStatus forces every subsequent image request to answer with status.
func (f *fakeFactory) setISOStatus(status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.isoStatus = status
}

func (f *fakeFactory) serve(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/versions":
		f.record("GET /versions")
		writeJSON(w, http.StatusOK, f.versionsJSON)

	case r.Method == http.MethodGet && len(parts) == 4 &&
		parts[0] == "version" && parts[2] == "extensions" && parts[3] == "official":
		f.record("GET /version/*/extensions/official")
		if parts[1] != catalogVersion {
			// A version the Factory does not know is a 404, not an empty list.
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, f.catalogJSON)

	case r.Method == http.MethodPost && r.URL.Path == "/schematics":
		f.record("POST /schematics")
		f.createSchematic(w, r)

	case (r.Method == http.MethodGet || r.Method == http.MethodHead) && len(parts) == 4 && parts[0] == "image":
		f.record("* /image/*")
		f.serveImage(w, parts[1])

	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

// createSchematic reproduces the trap: the document is accepted whatever it
// names, the id is the hash of the canonical bytes, and the document is echoed
// back as the authoritative record.
func (f *fakeFactory) createSchematic(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	if len(body) == 0 {
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}

	sum := sha256.Sum256(body)
	id := hex.EncodeToString(sum[:])

	f.mu.Lock()
	f.documents[id] = string(body)
	f.mu.Unlock()

	// The real Factory has been observed answering both 200 and 201 for this
	// endpoint; the client must accept either, so the fake uses the one the
	// live capture recorded.
	writeJSON(w, http.StatusCreated, mustJSON(map[string]string{
		"id":        id,
		"schematic": string(body),
	}))
}

// serveImage answers the way the Factory does: 400 for a schematic that names
// an extension the catalog does not contain, 200 otherwise. This is the
// asymmetry the whole package exists to handle -- the POST said nothing.
func (f *fakeFactory) serveImage(w http.ResponseWriter, id string) {
	f.mu.Lock()
	forced := f.isoStatus
	doc, seen := f.documents[id]
	f.mu.Unlock()

	if forced != 0 {
		w.WriteHeader(forced)
		return
	}
	if !seen {
		http.Error(w, "unknown schematic", http.StatusNotFound)
		return
	}
	for _, name := range extensionNamesIn(doc) {
		f.mu.Lock()
		_, ok := f.known[name]
		f.mu.Unlock()
		if !ok {
			http.Error(w, "extension not found", http.StatusBadRequest)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}

// extensionNamesIn pulls the official extension names out of a canonical
// schematic document. A line scan rather than a YAML parse: the fake only has
// to be faithful about behaviour, and a parser here would be a second
// implementation of the thing under test.
func extensionNamesIn(doc string) []string {
	var (
		names  []string
		inList bool
	)
	for _, line := range strings.Split(doc, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "officialExtensions:":
			inList = true
		case inList && strings.HasPrefix(trimmed, "- "):
			names = append(names, strings.Trim(strings.TrimPrefix(trimmed, "- "), `"'`))
		case trimmed == "":
			// keep going
		case !strings.HasPrefix(trimmed, "- "):
			inList = false
		}
	}
	return names
}

func writeJSON(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func mustJSON(v any) []byte {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return raw
}
