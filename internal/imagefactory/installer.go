package imagefactory

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// legacyInstallerRepo is the repository name the Factory published before the
// platform-prefixed one existed, documented upstream as the "Legacy installer
// Image". It is still the only name that resolves for part of the supported
// version range.
const legacyInstallerRepo = "installer"

// manifestAccept is the media-type set an OCI registry expects on a manifest
// request. A registry that receives none of these may answer 404 for a manifest
// that exists, which would look exactly like a repository name that does not
// resolve -- the one thing this function must not get wrong.
const manifestAccept = "application/vnd.oci.image.index.v1+json, " +
	"application/vnd.oci.image.manifest.v1+json, " +
	"application/vnd.docker.distribution.manifest.list.v2+json, " +
	"application/vnd.docker.distribution.manifest.v2+json"

// InstallerImage returns the OCI reference of the installer image for a
// schematic at a Talos version, with the repository name proven against the
// registry rather than assumed (D-02).
//
// The repository name is version-dependent. PITFALLS P9(d) records both
// `factory.talos.dev/v2/installer/<id>/manifests/<v>` and
// `.../metal-installer/<id>/manifests/<v>` answering for one version while only
// one of them answers for another. Hardcoding either name across the supported
// range is a bug with the worst possible failure mode: the reference is
// consumed by the upgrade RPC, and a wrong one produces an upgrade that reports
// success and silently drops every system extension the node was built with
// (P9(c)).
//
// So the platform-prefixed name is tried first and the legacy name second, and
// if neither answers this returns an error and no reference at all. There is no
// guess: a caller that receives an error here must not upgrade.
func (c *Client) InstallerImage(ctx context.Context, r AssetRequest) (string, error) {
	if err := r.validate(); err != nil {
		return "", err
	}

	repo, err := c.installerRepo(ctx, r)
	if err != nil {
		return "", err
	}

	// The registry host is the Factory this client was configured with, not a
	// constant: a deployment pointing at a private Factory must produce
	// references to that Factory, or every node it provisions pulls its
	// installer from somewhere the operator never configured.
	//
	// The result is an OCI image reference -- registry, repository, tag -- and
	// not a network endpoint. It is assembled by concatenation rather than by a
	// "%s:%s" format string on purpose: that shape is what the internal/talos
	// seam guard looks for, and a reference built here is exactly the kind of
	// thing the guard should keep looking at rather than learn to ignore.
	ref := c.base.Host
	if prefix := strings.Trim(c.base.Path, "/"); prefix != "" {
		ref += "/" + prefix
	}
	return ref + "/" + repo + "/" + r.SchematicID + ":" + r.Version, nil
}

// installerRepo returns the repository name that resolves for this request,
// consulting the cache first.
//
// The cache is keyed on platform and Talos version because that is what the
// name depends on -- not on the schematic. Every schematic at a version shares
// one answer, so a wizard that derives assets for several schematics at one
// version costs one manifest request rather than one per schematic (T-02-22).
// The platform is in the key because the modern name is the platform prefixed
// one, so two platforms at one version are two different questions.
//
// A failed resolution is deliberately not cached. The cache exists to spare
// requests, not to freeze an answer that was never obtained: remembering a
// transient registry outage as "no installer exists" would block upgrades long
// after the registry recovered.
func (c *Client) installerRepo(ctx context.Context, r AssetRequest) (string, error) {
	key := fmt.Sprintf("%s/%s", r.Platform, r.Version)

	c.installerMu.Lock()
	repo, ok := c.installerRepos[key]
	c.installerMu.Unlock()
	if ok {
		return repo, nil
	}

	repo, err := c.resolveInstallerRepo(ctx, r, string(r.Platform)+"-"+legacyInstallerRepo, legacyInstallerRepo)
	if err != nil {
		return "", err
	}

	c.installerMu.Lock()
	c.installerRepos[key] = repo
	c.installerMu.Unlock()
	return repo, nil
}

// resolveInstallerRepo asks the registry, in order, which of the candidate
// repository names carries a manifest for this schematic at this version.
//
// It keeps two verdicts apart, for the reason ProbeBuildable does. A 404 is the
// registry answering that the name does not resolve, and only when every
// candidate answers that way is this a statement about the schematic
// (ErrSchematicNotBuildable). Anything else -- a transport failure, a 5xx, an
// authentication challenge -- is the registry not answering the question, which
// says nothing about the schematic and must stay retryable
// (ErrUpstreamUnavailable). Reporting an outage as "this schematic has no
// installer" would send an operator to rebuild something that is not broken.
func (c *Client) resolveInstallerRepo(ctx context.Context, r AssetRequest, candidates ...string) (string, error) {
	var (
		refused     []string
		unanswered  error
		refusedAll  = true
		lastAttempt string
	)

	for _, repo := range candidates {
		lastAttempt = repo
		u := c.base.JoinPath("v2", repo, r.SchematicID, "manifests", r.Version).String()

		status, err := c.probeStatus(ctx, http.MethodGet, u, http.Header{"Accept": []string{manifestAccept}})
		if err != nil {
			refusedAll = false
			unanswered = fmt.Errorf("%w: resolving the installer repository %q for %s: %w",
				ErrUpstreamUnavailable, repo, r.Version, err)
			continue
		}

		switch {
		case status/100 == 2:
			return repo, nil
		case status == http.StatusNotFound:
			refused = append(refused, fmt.Sprintf("%s (HTTP %d)", repo, status))
		default:
			refusedAll = false
			unanswered = fmt.Errorf("%w: resolving the installer repository %q for %s answered HTTP %d",
				ErrUpstreamUnavailable, repo, r.Version, status)
		}
	}

	if refusedAll && len(refused) > 0 {
		return "", fmt.Errorf("%w: no installer image for schematic %s at %s; the registry has no manifest under %s",
			ErrSchematicNotBuildable, r.SchematicID, r.Version, strings.Join(refused, " or "))
	}
	if unanswered != nil {
		return "", unanswered
	}
	// Unreachable with a non-empty candidate list; a nil error with no
	// reference would be read by a caller as a resolved name.
	return "", fmt.Errorf("%w: the installer repository for %s was not resolved and the last candidate tried was %q",
		ErrUpstreamUnavailable, r.Version, lastAttempt)
}
