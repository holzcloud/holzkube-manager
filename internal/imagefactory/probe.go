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

	switch status / 100 {
	case 2:
		return nil
	case 4:
		// The Factory answered and refused. That is a statement about the
		// schematic, and it is the only thing that may be reported as one.
		return fmt.Errorf("%w: %s at %s/%s answered HTTP %d", ErrSchematicNotBuildable, id, talosVersion, arch, status)
	default:
		// The Factory did not answer usably. That says nothing about the
		// schematic, and showing it as a bad schematic would send an operator
		// to fix something that is not broken.
		return fmt.Errorf("%w: probing %s at %s/%s answered HTTP %d", ErrUpstreamUnavailable, id, talosVersion, arch, status)
	}
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
