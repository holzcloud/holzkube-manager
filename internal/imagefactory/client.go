package imagefactory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultTimeout bounds one Factory request end to end.
	//
	// Named and justified rather than inherited: http.DefaultClient has no
	// timeout at all, so a Factory that accepts a connection and then stops
	// talking would hold a goroutine and a connection for as long as the
	// process runs.
	//
	// What it governs is three different workloads, not one. The three JSON
	// endpoints -- versions, the version-scoped extension catalog, and schematic
	// creation -- are the small ones; the largest answer among them is the
	// catalog at a few tens of kilobytes. It also bounds ProbeBuildable's HEAD
	// of the ISO URL, which makes the Factory build a ~335MB image
	// synchronously, and both installer manifest GETs in installer.go, which a
	// cold resolution issues in series.
	//
	// The value has never been sized against those last two. It was chosen
	// against the JSON endpoints, and the measured cold ISO probe lands in the
	// 30.5-32.7s band -- one to three seconds past this constant, every time.
	// The value is deliberately left alone here: what it should be is the open
	// question in
	// .planning/phases/02-transport-seam-talossim-image-factory/02-DECISION-probe-budget.md,
	// together with how a per-route deadline composes with it. Whoever reads
	// this constant next should find that question rather than a justification
	// that was never about them.
	DefaultTimeout = 30 * time.Second

	// maxResponseBytes caps a response body before it is decoded, for the
	// reason internal/httpapi/handlers already states about inbound bodies:
	// unbounded decoding of input this process does not control is a denial of
	// service with no upside. The largest legitimate answer is the extension
	// catalog, two orders of magnitude below this.
	maxResponseBytes = 1 << 20

	// maxRedirects bounds a redirect chain that stays on the same host.
	maxRedirects = 5
)

// talosVersionPattern is the shape of a Talos version as it appears in a
// Factory path segment.
//
// Validated rather than escaped: these values reach holzkube-manager from an operator
// and are then interpolated into an upstream URL path, so a segment containing
// a slash or a dot-dot would address a different endpoint than the one this
// code reads as being addressed. Rejecting the shape outright is checkable;
// escaping correctly at every call site is not.
var talosVersionPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$`)

// schematicIDPattern is the shape of a schematic id: SHA-256, lowercase hex.
var schematicIDPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Extension is one entry of the version-scoped official extension catalog.
//
// The field set is exhaustive on purpose: responses are decoded with unknown
// fields rejected, so an upstream that grows a field fails loudly here instead
// of silently dropping it into a schematic nobody checked.
type Extension struct {
	Name        string `json:"name"`
	Ref         string `json:"ref"`
	Digest      string `json:"digest"`
	Author      string `json:"author"`
	Description string `json:"description"`
}

// Created is the Factory's answer to a schematic creation.
type Created struct {
	// ID is the schematic id.
	ID string

	// Canonical is the Factory's own normalised schematic document. This is
	// what callers persist -- not the input. The Factory re-serialises what it
	// receives, and the id is the hash of these bytes, so storing the input
	// stores something that may not hash to the id it is filed under.
	Canonical string
}

// Client talks to one Image Factory.
type Client struct {
	base *url.URL
	http *http.Client

	// installerMu guards installerRepos.
	installerMu sync.Mutex

	// installerRepos caches the resolved installer repository name, keyed by
	// platform, Talos version and the SecureBoot selection. Populated and read
	// only by installer.go, where the reasoning for the key lives.
	//
	// The value carries provenance, not just a name. An entry whose preferred
	// candidate answered is proven and is served forever; an entry reached past
	// a candidate that never answered is provisional, carries the warning that
	// says so, remembers which candidates were never ruled out, and is
	// re-questioned once it is older than installerRetry. installer.go owns
	// every one of those rules.
	installerRepos map[string]installerRepoEntry

	// installerRetry is how long a *provisional* installer-repo entry is served
	// before the candidates it never ruled out are asked again. Zero means
	// re-question on every call. It bounds no request and moves no deadline;
	// see installerRepoRetryInterval in installer.go.
	installerRetry time.Duration
}

// Option configures a Client. Options are applied in the order given.
type Option func(*Client) error

// WithHTTPClient supplies the HTTP client to use.
//
// The supplied client is shallow-copied and its redirect policy replaced rather
// than used as given: refusing a redirect to a different host is a property of
// this package, not a default a caller can accidentally drop by passing a
// client it configured for something else. Copying also means the caller's
// value is not mutated behind its back.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) error {
		if h == nil {
			return errors.New("imagefactory: WithHTTPClient was given a nil client")
		}
		cp := *h
		cp.CheckRedirect = refuseCrossHostRedirect
		c.http = &cp
		return nil
	}
}

// WithTimeout sets the per-request timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) error {
		if d <= 0 {
			return fmt.Errorf("imagefactory: timeout must be positive, got %s", d)
		}
		c.http.Timeout = d
		return nil
	}
}

// WithInstallerRepoRetryInterval sets how long a provisional installer
// repository answer -- one reached past a candidate that never answered -- is
// served before that candidate is asked again.
//
// Zero is legal here and means "re-question on every call", which is what lets
// a test drive both branches deterministically without a fake clock. That is
// the one difference from WithTimeout, where zero would mean "no timeout" and
// is therefore refused. A negative interval is not a shorter one; it is a
// mistake, and it is rejected in the register WithTimeout already uses.
//
// Production passes nothing and gets installerRepoRetryInterval.
func WithInstallerRepoRetryInterval(d time.Duration) Option {
	return func(c *Client) error {
		if d < 0 {
			return fmt.Errorf("imagefactory: the installer repository retry interval must not be negative, got %s", d)
		}
		c.installerRetry = d
		return nil
	}
}

// New returns a client for the Factory at baseURL.
//
// TLS verification is never disabled and there is no option to disable it: the
// schematic documents that cross this connection can carry secrets in kernel
// arguments and META values, which is the reason the Factory itself refuses to
// list schematics back.
func New(baseURL string, opts ...Option) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("imagefactory: parse base URL %q: %w", baseURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("imagefactory: base URL %q must be http or https, got scheme %q", baseURL, u.Scheme)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("imagefactory: base URL %q has no host", baseURL)
	}

	c := &Client{
		base: u,
		http: &http.Client{
			Timeout:       DefaultTimeout,
			CheckRedirect: refuseCrossHostRedirect,
		},
		installerRepos: map[string]installerRepoEntry{},
		installerRetry: installerRepoRetryInterval,
	}
	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// refuseCrossHostRedirect stops a redirect that leaves the origin the caller
// named -- its host or its scheme. A Factory that can bounce this client to an
// arbitrary host is a Factory that can have schematic contents -- kernel
// arguments and META values among them -- delivered somewhere the operator
// never configured, and the same sentence is true of a Factory that can bounce
// it to plaintext.
func refuseCrossHostRedirect(req *http.Request, via []*http.Request) error {
	if len(via) == 0 {
		return nil
	}
	origin := via[0].URL
	if req.URL.Host != origin.Host {
		return fmt.Errorf("imagefactory: refusing a redirect from %s to %s", origin.Host, req.URL.Host)
	}
	// Go re-sends the request body on a 307 and a 308, and the body on the one
	// POST this package makes is the canonical schematic document. A single hop
	// to http therefore puts kernel arguments and META values on the wire in
	// clear, with New's promise that verification is never disabled intact and
	// no option set. The host being unchanged does not make that acceptable.
	//
	// An upgrade is refused on the same terms rather than waved through. The
	// scheme this client speaks is the one the operator configured and New
	// validated; a Factory redirecting it to another one is answering a
	// question nobody asked, and a rule with an exception is a rule with a hole
	// to find.
	if req.URL.Scheme != origin.Scheme {
		return fmt.Errorf("imagefactory: refusing a redirect that changes the scheme from %s to %s",
			origin.Scheme, req.URL.Scheme)
	}
	if len(via) >= maxRedirects {
		return fmt.Errorf("imagefactory: refusing to follow more than %d redirects", maxRedirects)
	}
	return nil
}

// Versions returns every Talos version the Factory can build, newest last.
//
// The list includes pre-releases -- it ends in the current alpha, beta and rc
// tags -- so "the latest version" is not the last element. Filtering is the
// caller's decision and is deliberately not made here.
func (c *Client) Versions(ctx context.Context) ([]string, error) {
	var versions []string
	if err := c.getJSON(ctx, &versions, "versions"); err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("%w: GET /versions returned an empty list", ErrUpstreamUnavailable)
	}
	return versions, nil
}

// Extensions returns the official extension catalog for exactly talosVersion.
//
// The catalog is version-scoped and the scoping is the point: an extension that
// exists at one Talos version may not exist at another, so a list fetched for
// the wrong version produces a schematic that is un-buildable at the moment it
// is used. There is no cached or unscoped fallback; a failure here is a failure
// to validate, never a reason to guess.
//
// An empty catalog is treated as an upstream failure rather than as an answer.
// "This version has no extensions" and "this version is not one I know" are
// different statements, and conflating them offers the operator an empty menu
// and calls it complete.
func (c *Client) Extensions(ctx context.Context, talosVersion string) ([]Extension, error) {
	if !talosVersionPattern.MatchString(talosVersion) {
		return nil, fmt.Errorf("imagefactory: %q is not a Talos version", talosVersion)
	}
	var catalog []Extension
	if err := c.getJSON(ctx, &catalog, "version", talosVersion, "extensions", "official"); err != nil {
		return nil, err
	}
	if len(catalog) == 0 {
		return nil, fmt.Errorf("%w: the extension catalog for %s is empty, which is not an answer this client will act on",
			ErrUpstreamUnavailable, talosVersion)
	}
	return catalog, nil
}

// ErrSchematicIDMismatch reports that the id the Factory assigned is not the id
// computed locally for the same schematic.
//
// This means the canonical serialisation here and the one upstream have
// drifted, so every id this package computes without a round trip is suspect --
// which is the whole of FACT-06. The schematic itself was created: the returned
// Created carries the Factory's id and document, which are authoritative, so a
// caller that knows what it is doing can proceed on them.
var ErrSchematicIDMismatch = errors.New("imagefactory: the Factory assigned a different id than the one computed locally")

// CreateSchematic POSTs a schematic and returns the Factory's answer.
//
// Creation is not validation. A schematic naming an extension that does not
// exist is created successfully and produces a 400 on the first attempt to
// build an image from it, so a caller must have validated the extension names
// against Extensions beforehand and must call ProbeBuildable afterwards before
// showing the operator anything that reads as success.
func (c *Client) CreateSchematic(ctx context.Context, s Schematic) (Created, error) {
	doc, err := s.Canonical()
	if err != nil {
		return Created{}, err
	}
	want, err := s.ID()
	if err != nil {
		return Created{}, err
	}

	u := c.base.JoinPath("schematics")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(doc))
	if err != nil {
		return Created{}, fmt.Errorf("imagefactory: build request: %w", err)
	}
	// The Factory reads YAML and accepts JSON as a subset of it. The canonical
	// document is what is sent, so what is hashed locally is exactly what
	// crosses the wire.
	req.Header.Set("Content-Type", "application/yaml")
	req.Header.Set("Accept", "application/json")

	var out struct {
		ID        string `json:"id"`
		Schematic string `json:"schematic"`
	}
	if err := c.do(req, &out); err != nil {
		return Created{}, err
	}
	if out.ID == "" || out.Schematic == "" {
		return Created{}, fmt.Errorf("%w: POST /schematics answered without an id or a schematic document", ErrUpstreamUnavailable)
	}

	created := Created{ID: out.ID, Canonical: out.Schematic}
	if out.ID != want {
		return created, fmt.Errorf("%w: computed %s, Factory assigned %s", ErrSchematicIDMismatch, want, out.ID)
	}
	return created, nil
}

// getJSON issues a GET against the base URL joined with segments and decodes
// the answer.
func (c *Client) getJSON(ctx context.Context, dst any, segments ...string) error {
	u := c.base.JoinPath(segments...)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("imagefactory: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	return c.do(req, dst)
}

// do performs the request and decodes a JSON answer into dst.
func (c *Client) do(req *http.Request, dst any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %s %s: %w", ErrUpstreamUnavailable, req.Method, req.URL.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode/100 != 2 {
		// Every non-2xx is a failure, never a partial success: there is no
		// status this client acts on other than "the Factory answered what was
		// asked". The code travels in the message because it is the difference
		// between "retry later" and "this request is wrong".
		return fmt.Errorf("%w: %s %s: HTTP %d", ErrUpstreamUnavailable, req.Method, req.URL.Path, resp.StatusCode)
	}
	return decodeCapped(resp.Body, dst)
}

// decodeCapped reads at most maxResponseBytes and decodes them strictly.
//
// Strictly means two things beyond the cap. Unknown fields are rejected, so an
// upstream schema change is loud rather than a field silently dropped on the
// floor; and content after the JSON document is rejected, so a response that is
// two documents is not read as its first one.
func decodeCapped(body io.Reader, dst any) error {
	// One byte past the cap: reading exactly the cap cannot distinguish a body
	// that fits from one that was truncated.
	raw, err := io.ReadAll(io.LimitReader(body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("%w: read response: %w", ErrUpstreamUnavailable, err)
	}
	if len(raw) > maxResponseBytes {
		return fmt.Errorf("%w: response exceeds the %d byte cap and was not decoded", ErrUpstreamUnavailable, maxResponseBytes)
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("%w: decode response: %w", ErrUpstreamUnavailable, err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: response carries trailing content after the JSON document", ErrUpstreamUnavailable)
	}
	return nil
}

// BaseURL returns the Factory this client talks to, for logging and for error
// messages that would otherwise name no host at all.
func (c *Client) BaseURL() string { return strings.TrimSuffix(c.base.String(), "/") }
