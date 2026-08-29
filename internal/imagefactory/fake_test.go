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

// The Talos versions the fake answers registry manifest requests for. They
// exist because the installer repository name is version-dependent
// (PITFALLS P9(d)) and every resolution branch has to be exercisable offline.
const (
	// installerModernVersion resolves under the platform-prefixed repository.
	installerModernVersion = "v1.13.9"

	// installerLegacyVersion resolves only under the legacy "installer"
	// repository. This is the branch a resolver that hardcodes the modern name
	// gets wrong, and the failure is an upgrade that reports success while
	// dropping every system extension.
	installerLegacyVersion = "v1.9.0"

	// installerNoSecureBootVersion resolves under both ordinary names and under
	// neither SecureBoot name. It is the branch that proves a SecureBoot
	// request refuses rather than falling back to the ordinary installer, which
	// is the one substitution installerCandidates must never make. Its version
	// sits outside the supported v1.12-v1.14 range on purpose, like
	// installerBrokenVersion: this cell is constructed to reach a branch, and
	// nothing has checked whether any shipped version behaves this way.
	installerNoSecureBootVersion = "v1.8.0"

	// installerBrokenVersion resolves under no name at all.
	installerBrokenVersion = "v1.7.0"
)

// installerRepos maps a Talos version to the repository names that answer for
// it. A version absent from this map answers for none of them.
//
// This is a branch-coverage fixture, not a transcript of the registry. Each
// version here exists to make one resolution branch reachable offline, and the
// names under it were chosen for that -- not read off factory.talos.dev. The
// installerLegacyVersion row is the demonstration: it pins v1.9.0 to
// "installer" alone so TestInstallerImageFallsBackToTheLegacyName has a version
// where the fallback is the only path, while 02-04-SUMMARY.md:387 records
// metal-installer@v1.9.0 answering 200 confirmed live. Both statements are
// correct and they are about different things, so do not "fix" this map against
// the registry: doing so deletes the premise a passing test rests on. The
// registry's actual matrix is recorded by TestLiveFactory's installer-name
// subtest in live_test.go and in the plan summary, which is where an
// observation belongs.
//
// The modern row is the one cell that is also an observation: at v1.13.9 all
// four names answer, measured against the live registry, with the two SecureBoot
// names carrying a different image digest than the two ordinary ones (02-UAT.md
// G-02-4). The SecureBoot cell under installerLegacyVersion is an assumption --
// nothing has checked whether installer-secureboot answers at v1.9.0 -- and
// TestLiveFactory's matrix subtest is what settles it.
var installerRepos = map[string]map[string]bool{
	installerModernVersion: {
		"metal-installer": true, "installer": true,
		"metal-installer-secureboot": true, "installer-secureboot": true,
	},
	installerLegacyVersion:       {"installer": true, "installer-secureboot": true},
	installerNoSecureBootVersion: {"metal-installer": true, "installer": true},
	installerBrokenVersion:       {},
}

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

	// manifestStatus, when non-zero, overrides every registry manifest answer.
	// It is how a test distinguishes "the registry refused" from "the registry
	// did not answer", which are different statements about a schematic.
	manifestStatus int

	// unreachable is the set of repository names whose manifest requests are
	// answered with silence rather than with a status.
	//
	// A status code would not do, and that is the whole point of the knob. The
	// distinction under test is between the registry answering and the registry
	// not answering: a 503 is an answer, classified by registryRefused as the
	// upstream declining, and a fake that can only produce answers cannot
	// express "this candidate was never ruled out" at all. That is why the
	// matrix cell this knob reaches -- candidate 1 silent, candidate 2 2xx --
	// had never been testable (02-UAT.md G-02-3, third `missing` bullet).
	//
	// Be exact about which silence this is. Two shapes are on record and they
	// are not the same event. 02-04-SUMMARY.md:387 has a throttling Factory
	// accepting the connection and producing no HTTP response at all (curl exit
	// code 000); G-02-3's own evidence is the other one, a client timeout
	// ("resolved in 43.42s -- 30s timeout plus 13.4s"). This knob reproduces the
	// first. Both surface identically as a non-nil error from probeStatus
	// (probe.go:80-84) and take the same branch of resolveInstallerRepo, so the
	// code path under test is the right one and the classification is shared --
	// but the measured incident is not reproduced here. A timeout-shaped variant
	// would need a fake that sleeps past the client timeout, which is a 30s test.
	unreachable map[string]bool

	// forgedID, when non-empty, is the id POST /schematics answers with
	// instead of the hash of the document. It reproduces the one upstream
	// state FACT-06 exists to detect: a well-formed 201 carrying an id the
	// local canonical serialiser did not predict.
	forgedID string
}

func newFakeFactory(t *testing.T) *fakeFactory {
	t.Helper()

	f := &fakeFactory{
		counts:       map[string]int{},
		documents:    map[string]string{},
		known:        map[string]struct{}{},
		unreachable:  map[string]bool{},
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

// setManifestStatus forces every subsequent registry manifest request to answer
// with status. Zero restores the per-version behaviour.
func (f *fakeFactory) setManifestStatus(status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.manifestStatus = status
}

// setRepoUnreachable makes every subsequent manifest request for repo be
// answered with silence: the connection is hijacked and closed with no HTTP
// response at all.
//
// Settable and clearable per repository, and settable mid-test, because that is
// the only way a re-question can be observed: the cases that matter change one
// repository's behaviour between two calls to InstallerImage against one fake,
// in both directions. A constructor argument could express neither.
func (f *fakeFactory) setRepoUnreachable(repo string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.unreachable[repo] = true
}

// setRepoReachable undoes setRepoUnreachable, so a repository can go from
// silent back to answering within one test.
func (f *fakeFactory) setRepoReachable(repo string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.unreachable, repo)
}

// forgeID makes every subsequent POST /schematics answer with id rather than
// with the hash of the document it was sent.
func (f *fakeFactory) forgeID(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.forgedID = id
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

	case (r.Method == http.MethodGet || r.Method == http.MethodHead) && len(parts) == 5 &&
		parts[0] == "v2" && parts[3] == "manifests":
		f.record("GET /v2/" + parts[1] + "/manifests/" + parts[4])
		f.serveManifest(w, parts[1], parts[4])

	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

// serveManifest answers a registry manifest request the way factory.talos.dev
// does: the platform-prefixed and the legacy repository names resolve for
// different, version-dependent subsets of the supported range, and a name that
// does not resolve is a 404 rather than an empty manifest.
func (f *fakeFactory) serveManifest(w http.ResponseWriter, repo, version string) {
	f.mu.Lock()
	forced := f.manifestStatus
	silent := f.unreachable[repo]
	f.mu.Unlock()

	// Read under the same mutex as every other knob: the connection teardown
	// below races the next call's read of the map under -race otherwise.
	if silent {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			// Every httptest.Server over HTTP/1.1 supplies one; if this ever
			// fires, the knob is silently answering instead of staying silent,
			// which would invert every assertion that depends on it.
			http.Error(w, "the fake cannot hijack this connection", http.StatusInternalServerError)
			return
		}
		conn, _, err := hijacker.Hijack()
		if err != nil {
			return
		}
		_ = conn.Close()
		return
	}

	if forced != 0 {
		w.WriteHeader(forced)
		return
	}
	if !installerRepos[version][repo] {
		http.Error(w, "manifest unknown", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.oci.image.index.v1+json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(mustJSON(map[string]any{"schemaVersion": 2}))
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
	if f.forgedID != "" {
		id = f.forgedID
	}
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
