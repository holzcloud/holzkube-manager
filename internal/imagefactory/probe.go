package imagefactory

import (
	"context"
	"fmt"
	"net/http"
)

// ProbeBuildable reports whether a created schematic can actually be built for
// a specific Talos version and architecture.
//
// This is the mechanism FACT-02 requires, and it exists because creation is not
// validation: the Factory accepts a schematic referencing an extension that
// does not exist, assigns it an id, and only refuses when an image is asked
// for. Verified against the live Factory -- the schematic recorded in
// PITFALLS.md for a non-existent extension returns 400 on its ISO URL while a
// well-formed one returns 200.
//
// A nil error is the only thing that may be reported to an operator as
// "usable". ErrSchematicNotBuildable means the Factory answered and refused;
// ErrUpstreamUnavailable means it did not answer, which is not the same
// statement and must not be shown as a bad schematic.
func (c *Client) ProbeBuildable(ctx context.Context, id, talosVersion string, arch Arch) error {
	// The ISO is probed rather than the installer because the ISO receives the
	// full customization -- kernel arguments and META included -- so it is the
	// asset whose failure covers the most of a schematic. The URL comes from
	// ISOURL rather than being built here, so the probe cannot address a
	// different asset than the one the operator is later handed, and the
	// request validation is the same one in both places.
	u, err := ISOURL(c.BaseURL(), AssetRequest{
		SchematicID: id,
		Version:     talosVersion,
		Arch:        arch,
		Platform:    PlatformMetal,
	})
	if err != nil {
		return err
	}

	status, err := c.probeStatus(ctx, http.MethodHead, u, nil)
	if err != nil {
		return err
	}
	if status == http.StatusMethodNotAllowed || status == http.StatusNotImplemented {
		// Some caches in front of a Factory answer HEAD with a refusal. A
		// single-byte ranged GET asks the same question without downloading an
		// ISO to find out.
		status, err = c.probeStatus(ctx, http.MethodGet, u, http.Header{"Range": []string{"bytes=0-0"}})
		if err != nil {
			return err
		}
	}

	switch {
	case status/100 == 2:
		return nil
	case registryRefused(status):
		// The Factory answered and refused. That is a statement about the
		// schematic, and it is the only thing that may be reported as one.
		return fmt.Errorf("%w: %s at %s/%s answered HTTP %d", ErrSchematicNotBuildable, id, talosVersion, arch, status)
	default:
		// The Factory did not answer the question that was asked. That says
		// nothing about the schematic, and showing it as a bad schematic would
		// send an operator to fix something that is not broken.
		return fmt.Errorf("%w: probing %s at %s/%s answered HTTP %d", ErrUpstreamUnavailable, id, talosVersion, arch, status)
	}
}

// registryRefused reports whether a status is the Factory or the registry
// answering *about this schematic* rather than declining to answer *us*. It is
// this package's only statement of that taxonomy; both classification sites
// call it rather than restating it, which is what G-02-5 found them doing
// differently.
//
// 400 and 404 are answers. A 404 is "no manifest under that name", and a 400 is
// what the Factory returns when an extension the schematic names is not
// available at the requested version -- observed live against factory.talos.dev
// for both an all-zeros schematic id (404) and a nonsense repository name (400).
// Both are reproducible and neither is changed by asking again, so they are the
// only two statuses that may be reported to an operator as a bad schematic.
//
// Everything else is the upstream declining. A 401 is an authentication
// challenge, a 403 a policy refusal and a 429 a rate limit; all three are about
// the caller, and 5xx is about the upstream. factory.talos.dev is known to
// throttle -- 02-04-SUMMARY.md:385 records it doing so without an HTTP response
// at all -- and the probe verdict is written once at creation with no re-probe
// path, so a 429 filed as ErrSchematicNotBuildable becomes a permanent,
// unclearable accusation against a schematic nothing found fault with.
//
// The cost of putting those statuses on the retryable side is that fewer
// answers stamp a verdict, so more records end creation unprobed. That is the
// correct classification, and the size of the resulting unprobed population is
// the open question 02-DECISION-probe-budget.md owns -- not a reason to widen
// what counts as a refusal.
func registryRefused(status int) bool {
	return status == http.StatusBadRequest || status == http.StatusNotFound
}

// probeStatus issues one request and returns its status code, discarding the
// body. The body of an image asset is an ISO; nothing here wants it.
func (c *Client) probeStatus(ctx context.Context, method, u string, header http.Header) (int, error) {
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return 0, fmt.Errorf("imagefactory: build request: %w", err)
	}
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("%w: %s %s: %w", ErrUpstreamUnavailable, method, req.URL.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode, nil
}
