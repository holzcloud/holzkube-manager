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
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultTimeout bounds one *JSON* Factory request end to end: GET
	// /versions, GET /version/<v>/extensions/official and POST /schematics.
	// Those three and nothing else.
	//
	// Named and justified rather than inherited: http.DefaultClient has no
	// timeout at all, so a Factory that accepts a connection and then stops
	// talking would hold a goroutine and a connection for as long as the
	// process runs.
	//
	// The value was chosen against those three endpoints, whose largest answer
	// is the extension catalog at a few tens of kilobytes -- reasoning that is
	// now true of everything this constant covers, because the two workloads it
	// used to cover by accident have their own constants: ProbeTimeout for the
	// ISO probe and ManifestTimeout for a registry manifest GET. A reader
	// looking for the budget that bounds the probe should look there and not
	// here; that they were the same number is exactly what G-02-1 measured as a
	// defect.
	DefaultTimeout = 30 * time.Second

	// ProbeTimeout bounds ProbeBuildable's HEAD of the ISO URL and the
	// single-byte ranged GET it falls back to. That request makes the Factory
	// build a ~335MB image synchronously, which is a different workload from
	// asking it for a list.
	//
	// The rule, stated so the number can be rederived rather than remembered: a
	// budget is at least twice the slowest observed cold response for the
	// workload it bounds, rounded up to the next thirty seconds. The
	// observations are the five cold probes recorded in
	// .planning/phases/02-transport-seam-talossim-image-factory/02-DECISION-probe-budget.md,
	// measured across two investigators: 30.50, 30.59, 31.18, 31.52 and 32.69
	// seconds. The slowest doubles to 65.38 and rounds to 90.
	//
	// Twice, and not something tighter, for a reason worth writing down. The
	// bounded quantity is work on somebody else's build farm: sampled five
	// times, never controlled, and known to throttle. The constant this
	// replaces sat at 0.92 of the observed maximum and was exceeded on every
	// single one of those five runs. A budget at 1.2 of the maximum would be a
	// prediction about upstream latency, and a prediction is what failed.
	//
	// Widening the sample is how this moves. internal/imagefactory/live_test.go
	// re-applies the rule above to what it measures and fails when the shipped
	// value no longer satisfies it, so a new observation arrives here as a red
	// test rather than as nothing.
	ProbeTimeout = 90 * time.Second

	// ManifestTimeout bounds one registry manifest GET in resolveInstallerRepo.
	//
	// Same rule as ProbeTimeout: twice the slowest observed cold response,
	// rounded up to the next thirty seconds. The observation is the 13.4s cold
	// candidate answer decomposed out of G-02-2's 43.42s serial resolution
	// (30s for the silent candidate plus 13.4s for the one that answered),
	// which doubles to 26.8 and rounds to 30.
	//
	// That it equals DefaultTimeout is what the rule happened to produce from a
	// different input, and not an alias. The two are separate constants that
	// move independently, cmd/holzkube-managerd/budget_test.go reads them as
	// two, and code that means "the manifest budget" must say so -- a manifest
	// GET bounded by the JSON constant is precisely the confusion this plan
	// exists to end, in a new place.
	ManifestTimeout = 30 * time.Second

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

	// The three budgets, one per workload. They live here and not on
	// http.Client.Timeout because that field is one value for every request a
	// client makes, and three budgets cannot be expressed in one value -- which
	// is how the ISO probe came to be bounded by a number sized against a JSON
	// list. budgetClass names which of them a call is spending; withBudget
	// turns that into the call's context deadline.
	jsonBudget     time.Duration
	probeBudget    time.Duration
	manifestBudget time.Duration

	// onDrift is told when a response carried a field this package does not
	// know. It is a callback and not a logger because this package has no
	// logger and should not acquire one to say a single sentence: the
	// composition root already has one, and it is the place that knows whether
	// a Factory addition is worth a line in the operator's log or a metric.
	//
	// Nil is the shipped default and means the drift is decoded past in
	// silence, which is what every caller that has not thought about it gets.
	onDrift func(path, field string)
}

// budgetClass is which of the three workloads a request belongs to.
//
// It is the shape internal/talos/deadline.go's DeadlineClass uses, for the
// reason that file gives: a budget is only reviewable when the classes and
// their memberships can be read in one place rather than inferred from
// whichever constant happened to be in scope at a call site. It is unexported
// because the membership is not a caller's decision -- the three Client options
// are how a caller moves a budget, and which class a given request spends is a
// property of the request.
type budgetClass int

const (
	// classJSON is one of the three JSON endpoints: versions, the
	// version-scoped extension catalog, schematic creation.
	classJSON budgetClass = iota + 1

	// classProbe is the ISO probe: a HEAD (or a ranged GET) that makes the
	// Factory build an image synchronously.
	classProbe

	// classManifest is one registry manifest GET during installer repository
	// resolution.
	classManifest
)

func (b budgetClass) String() string {
	switch b {
	case classJSON:
		return "json"
	case classProbe:
		return "probe"
	case classManifest:
		return "manifest"
	default:
		return fmt.Sprintf("budgetClass(%d)", int(b))
	}
}

// budget is the client's duration for a class, and zero for a class it does not
// know.
func (c *Client) budget(class budgetClass) time.Duration {
	switch class {
	case classJSON:
		return c.jsonBudget
	case classProbe:
		return c.probeBudget
	case classManifest:
		return c.manifestBudget
	default:
		return 0
	}
}

// withBudget derives the context one call is issued on.
//
// It is a ceiling and not a floor: context.WithTimeout takes the earlier of an
// inherited deadline and this one, which is exactly what a shorter route
// deadline above it needs. Do not "fix" that by rebuilding the context from
// context.Background or by wrapping it in context.WithoutCancel -- a call that
// outlives the request it belongs to is the thing the route deadline exists to
// prevent.
//
// A class with no budget gets a cancellable context and no deadline, which
// requireDeadline then refuses at the wire. That is deliberate: the failure
// mode of a new, unbudgeted class must be a named refusal, never an unbounded
// request.
func (c *Client) withBudget(ctx context.Context, class budgetClass) (context.Context, context.CancelFunc) {
	d := c.budget(class)
	if d <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, d)
}

// requireDeadline is the structural gate, and it is the imagefactory sibling of
// internal/talos/deadline.go's function of the same name (D-04).
//
// It matters more here than it looks. This client no longer sets
// http.Client.Timeout -- it cannot, because that field is one number and there
// are three budgets -- so a request issued on a context with no deadline would
// run until the process ends. The removed field used to keep that promise by
// accident; this keeps it on purpose, at the two and only two places that reach
// http.Client.Do.
//
// It is a refusal and it is never retryable. There is no value, option or
// context key that turns it off: the fix for reaching it is a withBudget call
// at the site that forgot one, not a default here.
func requireDeadline(ctx context.Context) error {
	if _, ok := ctx.Deadline(); !ok {
		return ErrNoDeadline
	}
	return nil
}

// ErrNoDeadline reports a request that would have gone to the wire on a context
// with no deadline. Nothing was sent.
var ErrNoDeadline = errors.New("imagefactory: refusing a request with no deadline")

// Option configures a Client. Options are applied in the order given.
type Option func(*Client) error

// WithHTTPClient supplies the HTTP client to use.
//
// The supplied client is shallow-copied and its redirect policy replaced rather
// than used as given: refusing a redirect to a different host is a property of
// this package, not a default a caller can accidentally drop by passing a
// client it configured for something else. Copying also means the caller's
// value is not mutated behind its back.
//
// The copy's Timeout is cleared, and the budgets are not carried on it. They
// live on the Client and are applied per call as context deadlines, so a
// caller's own Timeout is neither honoured nor silently discarded -- it is
// simply not the mechanism any more, and a request through this client is
// bounded by the same three budgets whether or not this option was used. That
// removes the reason .planning/WINDOWS.md entry 8 (WR-01) exists: the option
// used to overwrite a configured timeout with the supplied client's zero value
// and say nothing.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) error {
		if h == nil {
			return errors.New("imagefactory: WithHTTPClient was given a nil client")
		}
		cp := *h
		cp.CheckRedirect = refuseCrossHostRedirect
		cp.Timeout = 0
		c.http = &cp
		return nil
	}
}

// WithTimeout sets the budget for the three JSON endpoints.
//
// It keeps its old name and its old meaning is now narrower: it is the JSON
// budget, DefaultTimeout's, and it does not move the probe or the manifest one.
// Renaming it would break every caller to say something the doc comment can say
// -- but a caller that means "bound the ISO probe" has to reach for
// WithProbeTimeout, because that is a different workload with a different
// derivation.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) error {
		if d <= 0 {
			return fmt.Errorf("imagefactory: timeout must be positive, got %s", d)
		}
		c.jsonBudget = d
		return nil
	}
}

// WithDriftObserver is told, once per response, when the Factory answered with
// a field this package does not know.
//
// It exists because decodeCapped stopped refusing such a response (see its
// comment for why) and the loudness had to go somewhere. The observer runs on
// the calling goroutine inside the request, so it must not block: a logger
// call is what it is for.
func WithDriftObserver(fn func(path, field string)) Option {
	return func(c *Client) error {
		c.onDrift = fn
		return nil
	}
}

// WithProbeTimeout sets the budget for the ISO probe. See ProbeTimeout for what
// the shipped value is derived from; a caller overriding it is overriding that
// derivation and should have observations of its own.
func WithProbeTimeout(d time.Duration) Option {
	return func(c *Client) error {
		if d <= 0 {
			return fmt.Errorf("imagefactory: the probe timeout must be positive, got %s", d)
		}
		c.probeBudget = d
		return nil
	}
}

// WithManifestTimeout sets the budget for one registry manifest GET. See
// ManifestTimeout for its derivation.
func WithManifestTimeout(d time.Duration) Option {
	return func(c *Client) error {
		if d <= 0 {
			return fmt.Errorf("imagefactory: the manifest timeout must be positive, got %s", d)
		}
		c.manifestBudget = d
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
		// No Timeout. One value cannot express three budgets, and the one it
		// used to express bounded the ISO probe with a number sized against a
		// JSON list. The budgets below are applied per call by withBudget, and
		// requireDeadline refuses at the wire if a call path ever forgets to.
		http: &http.Client{
			CheckRedirect: refuseCrossHostRedirect,
		},
		installerRepos: map[string]installerRepoEntry{},
		installerRetry: installerRepoRetryInterval,
		jsonBudget:     DefaultTimeout,
		probeBudget:    ProbeTimeout,
		manifestBudget: ManifestTimeout,
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

	callCtx, cancel := c.withBudget(ctx, classJSON)
	defer cancel()

	u := c.base.JoinPath("schematics")
	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, u.String(), bytes.NewReader(doc))
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
	// Both callers -- Versions and Extensions -- are JSON endpoints, so the
	// class is fixed here rather than passed in. probeStatus takes its class as
	// a parameter for the opposite reason: it serves two workloads.
	callCtx, cancel := c.withBudget(ctx, classJSON)
	defer cancel()

	u := c.base.JoinPath(segments...)
	req, err := http.NewRequestWithContext(callCtx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("imagefactory: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	return c.do(req, dst)
}

// do performs the request and decodes a JSON answer into dst.
func (c *Client) do(req *http.Request, dst any) error {
	// One of the two places in this package that reach http.Client.Do, and
	// therefore one of the two that have to hold the deadline promise.
	if err := requireDeadline(req.Context()); err != nil {
		return fmt.Errorf("%w: %s %s", err, req.Method, req.URL.Path)
	}

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
	drifted, err := decodeCapped(resp.Body, dst)
	if err != nil {
		return err
	}
	if drifted != "" && c.onDrift != nil {
		c.onDrift(req.URL.Path, drifted)
	}
	return nil
}

// unknownFieldPrefix is how encoding/json says a field was not in the struct.
//
// Matching a message is not something to do lightly, and what makes it safe
// here is that nothing depends on the match succeeding: a message that changes
// shape makes decodeCapped fall through to the tolerant decode without a drift
// report, which is the same outcome minus one log line. It is not a path where
// a missed match silently changes what the operator is shown.
const unknownFieldPrefix = "json: unknown field "

// decodeCapped reads at most maxResponseBytes and decodes them.
//
// It returns the name of the first field the response carried that this
// package does not know, or the empty string when there was none.
//
// THIS USED TO REFUSE such a response, and refusing was wrong in a way worth
// writing down, because the reasoning that produced it is good reasoning. The
// argument was: a field silently dropped is a field nobody notices until an
// operator wonders why a value they can see in the Factory's own API is not
// here. That is true. What it missed is the cost on the other side. The
// Factory is a third party that adds fields without telling anyone, the
// extension catalog is read on every visit to the Images screen, and every
// field in an Extension is optional to this product's purpose -- so one
// additive upstream field took the whole screen down, for everybody, until
// somebody shipped a new holzkube-manager. A screen that shows an operator a
// catalog missing a field they did not ask for is strictly better than a
// screen that shows them a 502.
//
// So the loudness moves rather than disappearing: the strict decode still
// runs, it just reports instead of refusing, and do() hands the name to
// whatever the caller wired to onDrift. An addition is still something
// somebody finds out about the same day. It is now a log line rather than an
// outage.
//
// Two things are still refused, and neither is additive drift. A body past the
// cap is not decoded at all -- unbounded decoding of a third party's response
// is a denial of service with no upside. And content after the JSON document
// is refused, because a response that is two documents read as its first one
// is not a tolerated addition, it is a different answer than the one that
// arrived.
func decodeCapped(body io.Reader, dst any) (string, error) {
	// One byte past the cap: reading exactly the cap cannot distinguish a body
	// that fits from one that was truncated.
	raw, err := io.ReadAll(io.LimitReader(body, maxResponseBytes+1))
	if err != nil {
		return "", fmt.Errorf("%w: read response: %w", ErrUpstreamUnavailable, err)
	}
	if len(raw) > maxResponseBytes {
		return "", fmt.Errorf("%w: response exceeds the %d byte cap and was not decoded", ErrUpstreamUnavailable, maxResponseBytes)
	}

	strict := json.NewDecoder(bytes.NewReader(raw))
	strict.DisallowUnknownFields()
	err = strict.Decode(dst)

	var drifted string
	if err != nil && strings.HasPrefix(err.Error(), unknownFieldPrefix) {
		drifted = strings.Trim(strings.TrimPrefix(err.Error(), unknownFieldPrefix), `"`)

		// The strict decode stopped where it found the field, so dst holds
		// however much of the document it had reached. Zeroing it is what
		// makes the tolerant decode below a decode of the whole response and
		// not a merge into a half-filled value.
		reflect.ValueOf(dst).Elem().SetZero()

		tolerant := json.NewDecoder(bytes.NewReader(raw))
		err = tolerant.Decode(dst)
	}
	if err != nil {
		return "", fmt.Errorf("%w: decode response: %w", ErrUpstreamUnavailable, err)
	}

	// Re-read rather than continuing from either decoder: which one consumed
	// the document depends on whether there was drift, and a check that has to
	// ask that question is a check that will eventually be asked it wrongly.
	trailing := json.NewDecoder(bytes.NewReader(raw))
	if err := trailing.Decode(&json.RawMessage{}); err != nil {
		return "", fmt.Errorf("%w: decode response: %w", ErrUpstreamUnavailable, err)
	}
	if err := trailing.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("%w: response carries trailing content after the JSON document", ErrUpstreamUnavailable)
	}
	return drifted, nil
}

// BaseURL returns the Factory this client talks to, for logging and for error
// messages that would otherwise name no host at all.
func (c *Client) BaseURL() string { return strings.TrimSuffix(c.base.String(), "/") }
