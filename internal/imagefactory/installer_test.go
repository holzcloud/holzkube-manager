package imagefactory_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube/internal/imagefactory"
)

// fakeHost is the host:port half of the fake's URL, which is what an OCI
// reference derived from it must carry. Against the public Factory this is
// factory.talos.dev; deriving it from the client's own base URL is what makes
// a private Factory deployment produce references to itself.
func fakeHost(t *testing.T, url string) string {
	t.Helper()
	host := strings.TrimPrefix(strings.TrimPrefix(url, "http://"), "https://")
	if host == "" || host == url && strings.Contains(url, "://") {
		t.Fatalf("cannot derive a host from %q", url)
	}
	return host
}

func installerRequest(version string) imagefactory.AssetRequest {
	return imagefactory.AssetRequest{
		SchematicID: schematicA,
		Version:     version,
		Arch:        imagefactory.ArchAMD64,
		Platform:    imagefactory.PlatformMetal,
	}
}

// TestInstallerImageResolvesThePlatformPrefixedName is the D-02 happy path: the
// name is proven against the registry manifest, and the modern, platform
// prefixed repository is the one returned when it answers.
func TestInstallerImageResolvesThePlatformPrefixedName(t *testing.T) {
	fake := newFakeFactory(t)
	client := newClient(t, fake.URL)

	ref, err := client.InstallerImage(t.Context(), installerRequest(installerModernVersion))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := fakeHost(t, fake.URL) + "/metal-installer/" + schematicA + ":" + installerModernVersion
	if ref != want {
		t.Errorf("ref  = %s\nwant = %s", ref, want)
	}
	if n := fake.count("GET /v2/metal-installer/manifests/" + installerModernVersion); n != 1 {
		t.Errorf("platform-prefixed manifest requests = %d, want 1", n)
	}
}

// TestInstallerImageFallsBackToTheLegacyName covers the finding in PITFALLS
// P9(d): the repository name is version-dependent, and a version where only the
// legacy name resolves must yield the legacy name rather than an error.
func TestInstallerImageFallsBackToTheLegacyName(t *testing.T) {
	fake := newFakeFactory(t)
	client := newClient(t, fake.URL)

	ref, err := client.InstallerImage(t.Context(), installerRequest(installerLegacyVersion))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := fakeHost(t, fake.URL) + "/installer/" + schematicA + ":" + installerLegacyVersion
	if ref != want {
		t.Errorf("ref  = %s\nwant = %s", ref, want)
	}
	if n := fake.count("GET /v2/metal-installer/manifests/" + installerLegacyVersion); n != 1 {
		t.Errorf("the platform-prefixed name was tried %d times, want 1 -- it must be tried first", n)
	}
	if n := fake.count("GET /v2/installer/manifests/" + installerLegacyVersion); n != 1 {
		t.Errorf("legacy manifest requests = %d, want 1", n)
	}
}

// TestInstallerImageRefusesWhenNeitherAnswers is the whole reason this function
// issues a request at all. The reference it returns is consumed by the upgrade
// RPC, and a guessed one produces an upgrade that reports success while
// dropping every system extension (PITFALLS P9(c)).
func TestInstallerImageRefusesWhenNeitherAnswers(t *testing.T) {
	fake := newFakeFactory(t)
	client := newClient(t, fake.URL)

	ref, err := client.InstallerImage(t.Context(), installerRequest(installerBrokenVersion))
	if err == nil {
		t.Fatalf("no error, and it produced the reference %q", ref)
	}
	if ref != "" {
		t.Errorf("a refusal still returned a reference: %q", ref)
	}
	if !errors.Is(err, imagefactory.ErrSchematicNotBuildable) {
		t.Errorf("error = %v, want ErrSchematicNotBuildable -- the registry answered and refused", err)
	}
	if !strings.Contains(err.Error(), "metal-installer") || !strings.Contains(err.Error(), "installer") {
		t.Errorf("the refusal does not name the repositories it tried: %v", err)
	}
}

// TestInstallerImageSeparatesAnUnreachableRegistryFromARefusal keeps the two
// verdicts apart for the same reason ProbeBuildable does: a registry that did
// not answer has said nothing about the schematic, and reporting it as a
// refusal sends an operator to fix something that is not broken.
func TestInstallerImageSeparatesAnUnreachableRegistryFromARefusal(t *testing.T) {
	fake := newFakeFactory(t)
	fake.setManifestStatus(500)
	client := newClient(t, fake.URL)

	_, err := client.InstallerImage(t.Context(), installerRequest(installerModernVersion))
	if !errors.Is(err, imagefactory.ErrUpstreamUnavailable) {
		t.Fatalf("error = %v, want ErrUpstreamUnavailable", err)
	}
	if errors.Is(err, imagefactory.ErrSchematicNotBuildable) {
		t.Error("a registry that did not answer was reported as a schematic that cannot be built")
	}
}

// TestInstallerImageCachesPerTalosVersion pins T-02-22: without the cache the
// resolver issues a registry request per asset request. The key is the Talos
// version because that is what the name depends on, so a second schematic at a
// resolved version costs nothing and a second version costs one request.
func TestInstallerImageCachesPerTalosVersion(t *testing.T) {
	fake := newFakeFactory(t)
	client := newClient(t, fake.URL)

	for range 2 {
		if _, err := client.InstallerImage(t.Context(), installerRequest(installerModernVersion)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if n := fake.count("GET /v2/metal-installer/manifests/" + installerModernVersion); n != 1 {
		t.Errorf("two calls at one version issued %d manifest requests, want 1", n)
	}

	if _, err := client.InstallerImage(t.Context(), installerRequest(installerLegacyVersion)); err != nil {
		t.Fatalf("second version: %v", err)
	}
	if n := fake.count("GET /v2/metal-installer/manifests/" + installerLegacyVersion); n != 1 {
		t.Errorf("a second version issued %d manifest requests, want 1", n)
	}
}

// TestInstallerImageDoesNotCacheAFailure keeps a transient registry outage from
// being remembered as a permanent verdict. The cache exists to spare requests,
// not to freeze an answer that was never obtained.
func TestInstallerImageDoesNotCacheAFailure(t *testing.T) {
	fake := newFakeFactory(t)
	fake.setManifestStatus(503)
	client := newClient(t, fake.URL)

	if _, err := client.InstallerImage(t.Context(), installerRequest(installerModernVersion)); err == nil {
		t.Fatal("the unreachable registry produced no error")
	}

	fake.setManifestStatus(0)
	ref, err := client.InstallerImage(t.Context(), installerRequest(installerModernVersion))
	if err != nil {
		t.Fatalf("after the registry recovered: %v", err)
	}
	if !strings.Contains(ref, "/metal-installer/") {
		t.Errorf("ref = %s, want the platform-prefixed repository", ref)
	}
}

// TestInstallerImageValidatesItsRequest stops an unchecked value reaching a
// registry path, for the reason client.go already argues about version and id
// segments: rejecting the shape is checkable, escaping at every call site is
// not.
func TestInstallerImageValidatesItsRequest(t *testing.T) {
	fake := newFakeFactory(t)
	client := newClient(t, fake.URL)

	cases := map[string]imagefactory.AssetRequest{
		"bad schematic id": {SchematicID: "../etc", Version: installerModernVersion, Arch: imagefactory.ArchAMD64, Platform: imagefactory.PlatformMetal},
		"bad version":      {SchematicID: schematicA, Version: "latest", Arch: imagefactory.ArchAMD64, Platform: imagefactory.PlatformMetal},
		"bad architecture": {SchematicID: schematicA, Version: installerModernVersion, Arch: imagefactory.Arch("riscv64"), Platform: imagefactory.PlatformMetal},
		"bad platform":     {SchematicID: schematicA, Version: installerModernVersion, Arch: imagefactory.ArchAMD64, Platform: imagefactory.Platform("toaster")},
	}

	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			if ref, err := client.InstallerImage(t.Context(), req); err == nil {
				t.Fatalf("accepted and produced %q", ref)
			}
		})
	}
	if n := fake.count("GET /v2/metal-installer/manifests/" + installerModernVersion); n != 0 {
		t.Errorf("an invalid request still reached the registry %d times", n)
	}
}
