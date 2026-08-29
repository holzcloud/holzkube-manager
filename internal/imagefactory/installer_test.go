package imagefactory_test

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

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

	ref, _, err := client.InstallerImage(t.Context(), installerRequest(installerModernVersion))
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

	ref, _, err := client.InstallerImage(t.Context(), installerRequest(installerLegacyVersion))
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

	ref, _, err := client.InstallerImage(t.Context(), installerRequest(installerBrokenVersion))
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

	_, _, err := client.InstallerImage(t.Context(), installerRequest(installerModernVersion))
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
		if _, _, err := client.InstallerImage(t.Context(), installerRequest(installerModernVersion)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if n := fake.count("GET /v2/metal-installer/manifests/" + installerModernVersion); n != 1 {
		t.Errorf("two calls at one version issued %d manifest requests, want 1", n)
	}

	if _, _, err := client.InstallerImage(t.Context(), installerRequest(installerLegacyVersion)); err != nil {
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

	if _, _, err := client.InstallerImage(t.Context(), installerRequest(installerModernVersion)); err == nil {
		t.Fatal("the unreachable registry produced no error")
	}

	fake.setManifestStatus(0)
	ref, _, err := client.InstallerImage(t.Context(), installerRequest(installerModernVersion))
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
			if ref, _, err := client.InstallerImage(t.Context(), req); err == nil {
				t.Fatalf("accepted and produced %q", ref)
			}
		})
	}
	if n := fake.count("GET /v2/metal-installer/manifests/" + installerModernVersion); n != 0 {
		t.Errorf("an invalid request still reached the registry %d times", n)
	}
}

// secureBootInstallerRequest is installerRequest with the SecureBoot flag set,
// so the two variants differ in exactly the field under test and nothing else.
func secureBootInstallerRequest(version string) imagefactory.AssetRequest {
	r := installerRequest(version)
	r.SecureBoot = true
	return r
}

// TestInstallerImageResolvesTheSecureBootName closes G-02-4. SecureBoot is not
// a property the schematic carries into its installer: the repository name is
// the switch, and at v1.13.9 the same schematic resolves to two different image
// digests under the SecureBoot and the ordinary name.
func TestInstallerImageResolvesTheSecureBootName(t *testing.T) {
	fake := newFakeFactory(t)
	client := newClient(t, fake.URL)

	ref, _, err := client.InstallerImage(t.Context(), secureBootInstallerRequest(installerModernVersion))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := fakeHost(t, fake.URL) + "/metal-installer-secureboot/" + schematicA + ":" + installerModernVersion
	if ref != want {
		t.Errorf("ref  = %s\nwant = %s", ref, want)
	}
	if n := fake.count("GET /v2/metal-installer-secureboot/manifests/" + installerModernVersion); n != 1 {
		t.Errorf("platform-prefixed SecureBoot manifest requests = %d, want 1", n)
	}
	if n := fake.count("GET /v2/metal-installer/manifests/" + installerModernVersion); n != 0 {
		t.Errorf("a SecureBoot request asked the ordinary repository %d times, want 0", n)
	}
}

// TestInstallerImageFallsBackToTheLegacySecureBootName gives the SecureBoot
// pair the same ordered fallback the ordinary pair already has: upstream's
// registry frontend parses installer-secureboot as the legacy form of
// metal-installer-secureboot.
func TestInstallerImageFallsBackToTheLegacySecureBootName(t *testing.T) {
	fake := newFakeFactory(t)
	client := newClient(t, fake.URL)

	ref, _, err := client.InstallerImage(t.Context(), secureBootInstallerRequest(installerLegacyVersion))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := fakeHost(t, fake.URL) + "/installer-secureboot/" + schematicA + ":" + installerLegacyVersion
	if ref != want {
		t.Errorf("ref  = %s\nwant = %s", ref, want)
	}
	if n := fake.count("GET /v2/metal-installer-secureboot/manifests/" + installerLegacyVersion); n != 1 {
		t.Errorf("the platform-prefixed SecureBoot name was tried %d times, want 1 -- it must be tried first", n)
	}
	if n := fake.count("GET /v2/installer-secureboot/manifests/" + installerLegacyVersion); n != 1 {
		t.Errorf("legacy SecureBoot manifest requests = %d, want 1", n)
	}
}

// TestInstallerImageRefusesRatherThanSubstitutingTheOrdinaryInstaller is the
// behaviour change this plan makes deliberately. At a version where neither
// SecureBoot name answers the request is refused with no reference, because
// handing back the ordinary installer is exactly the ISO/installer drift
// G-02-4 exists to stop -- the ordinary installer does not produce a SecureBoot
// node.
func TestInstallerImageRefusesRatherThanSubstitutingTheOrdinaryInstaller(t *testing.T) {
	fake := newFakeFactory(t)
	client := newClient(t, fake.URL)

	ref, _, err := client.InstallerImage(t.Context(), secureBootInstallerRequest(installerNoSecureBootVersion))
	if err == nil {
		t.Fatalf("no error, and it produced the reference %q", ref)
	}
	if ref != "" {
		t.Errorf("a refusal still returned a reference: %q", ref)
	}
	if !errors.Is(err, imagefactory.ErrSchematicNotBuildable) {
		t.Errorf("error = %v, want ErrSchematicNotBuildable -- the registry answered and refused", err)
	}
	if !strings.Contains(err.Error(), "metal-installer-secureboot") || !strings.Contains(err.Error(), "installer-secureboot") {
		t.Errorf("the refusal does not name the SecureBoot repositories it tried: %v", err)
	}
	// The load-bearing half: the ordinary names were never even asked about, so
	// no later change can quietly reintroduce the substitution.
	for _, repo := range []string{"metal-installer", "installer"} {
		if n := fake.count("GET /v2/" + repo + "/manifests/" + installerNoSecureBootVersion); n != 0 {
			t.Errorf("a SecureBoot request fell back to %q %d times, want 0", repo, n)
		}
	}
}

// TestInstallerImageCachesSecureBootSeparately pins T-02-43. The cache key is
// what makes a second request a second question; without the SecureBoot
// selection in it, the second resolution returns the first one's answer and
// nothing else in the codebase would notice.
func TestInstallerImageCachesSecureBootSeparately(t *testing.T) {
	fake := newFakeFactory(t)
	client := newClient(t, fake.URL)

	plain, _, err := client.InstallerImage(t.Context(), installerRequest(installerModernVersion))
	if err != nil {
		t.Fatalf("without SecureBoot: %v", err)
	}
	secure, _, err := client.InstallerImage(t.Context(), secureBootInstallerRequest(installerModernVersion))
	if err != nil {
		t.Fatalf("with SecureBoot: %v", err)
	}

	if plain == secure {
		t.Fatalf("both requests resolved to %s; the SecureBoot flag is not in the cache key", plain)
	}
	if n := fake.count("GET /v2/metal-installer/manifests/" + installerModernVersion); n != 1 {
		t.Errorf("ordinary manifest resolutions = %d, want 1", n)
	}
	if n := fake.count("GET /v2/metal-installer-secureboot/manifests/" + installerModernVersion); n != 1 {
		t.Errorf("SecureBoot manifest resolutions = %d, want 1", n)
	}

	// And each variant still costs one resolution, not one per call.
	for range 2 {
		if _, _, err := client.InstallerImage(t.Context(), secureBootInstallerRequest(installerModernVersion)); err != nil {
			t.Fatalf("repeat SecureBoot resolution: %v", err)
		}
	}
	if n := fake.count("GET /v2/metal-installer-secureboot/manifests/" + installerModernVersion); n != 1 {
		t.Errorf("repeated SecureBoot requests issued %d manifest requests, want 1", n)
	}
}

// TestResolveInstallerRepoClassifiesEveryRegistryAnswer is the mirror of
// TestProbeBuildableClassifiesEveryRegistryAnswer, driven through the manifest
// answer rather than the ISO answer and asserted against the same table. The
// two are what closes G-02-5: one taxonomy, stated once in probe.go, reached
// from both classification sites.
func TestResolveInstallerRepoClassifiesEveryRegistryAnswer(t *testing.T) {
	for _, answer := range registryAnswerTable {
		t.Run(answer.name(), func(t *testing.T) {
			fake := newFakeFactory(t)
			fake.setManifestStatus(answer.status)
			client := newClient(t, fake.URL)

			ref, _, err := client.InstallerImage(t.Context(), installerRequest(installerModernVersion))

			want := answer.wantErr()
			if want == nil {
				if err != nil {
					t.Fatalf("HTTP %d produced %v, want a resolved reference (%s)", answer.status, err, answer.why)
				}
				return
			}
			if !errors.Is(err, want) {
				t.Fatalf("HTTP %d produced %v, want %v (%s) -- the two classification sites disagree",
					answer.status, err, want, answer.why)
			}
			if ref != "" {
				t.Errorf("HTTP %d still produced the reference %q", answer.status, ref)
			}
			other := imagefactory.ErrUpstreamUnavailable
			if errors.Is(want, imagefactory.ErrUpstreamUnavailable) {
				other = imagefactory.ErrSchematicNotBuildable
			}
			if errors.Is(err, other) {
				t.Errorf("HTTP %d produced an error that is also %v", answer.status, other)
			}
			if !strings.Contains(err.Error(), answer.name()) {
				t.Errorf("the error does not carry the status the registry answered (%d): %v", answer.status, err)
			}
		})
	}
}

// TestInstallerImageTreatsAnAllBadRequestCandidateSetAsARefusal is the row
// G-02-5 made unreachable. Before the shared predicate, one non-404 4xx set
// refusedAll to false permanently, so a candidate set answering 400 throughout
// produced ErrUpstreamUnavailable and told the operator to retry something that
// would never succeed.
func TestInstallerImageTreatsAnAllBadRequestCandidateSetAsARefusal(t *testing.T) {
	fake := newFakeFactory(t)
	fake.setManifestStatus(400)
	client := newClient(t, fake.URL)

	ref, _, err := client.InstallerImage(t.Context(), installerRequest(installerModernVersion))
	if !errors.Is(err, imagefactory.ErrSchematicNotBuildable) {
		t.Fatalf("err = %v, want ErrSchematicNotBuildable", err)
	}
	if ref != "" {
		t.Errorf("a refusal still returned a reference: %q", ref)
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("the refusal does not name what the candidates answered: %v", err)
	}
}

// The cases below cover the matrix cell that had never been coverable: the
// platform-prefixed candidate is silent at the transport level and the legacy
// candidate answers 2xx. Before fake_test.go's setRepoUnreachable knob the fake
// could only produce answers, and "the registry did not answer" is precisely
// the state this whole group is about (02-UAT.md G-02-3).

// TestInstallerImageWarnsWhenThePreferredNameWasNeverRuledOut is G-02-3's core
// case. A transport failure on the preferred candidate is not a refusal: the
// name was unheard, not ruled out, so the reference reached past it is usable
// but provisional and must say so on the answer that carries it.
func TestInstallerImageWarnsWhenThePreferredNameWasNeverRuledOut(t *testing.T) {
	fake := newFakeFactory(t)
	fake.setRepoUnreachable("metal-installer")
	client := newClient(t, fake.URL)

	ref, warnings, err := client.InstallerImage(t.Context(), installerRequest(installerModernVersion))
	if err != nil {
		t.Fatalf("the fallback produced an error rather than a warning: %v", err)
	}

	want := fakeHost(t, fake.URL) + "/installer/" + schematicA + ":" + installerModernVersion
	if ref != want {
		t.Errorf("ref  = %s\nwant = %s", ref, want)
	}
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want exactly 1: %+v", len(warnings), warnings)
	}

	w := warnings[0]
	if w.Code != imagefactory.WarningInstallerRepoFallbackUnverified {
		t.Errorf("code = %q, want %q", w.Code, imagefactory.WarningInstallerRepoFallbackUnverified)
	}
	if !strings.Contains(w.Detail, "metal-installer") {
		t.Errorf("the warning does not name the repository that never answered: %q", w.Detail)
	}
	if !strings.Contains(w.Detail, installerModernVersion) {
		t.Errorf("the warning does not name the version: %q", w.Detail)
	}
	// The transport error as the client reported it. Without it an operator
	// reading the warning cannot tell a throttled registry from a broken one.
	if !strings.Contains(w.Detail, "EOF") && !strings.Contains(w.Detail, "connection") {
		t.Errorf("the warning does not carry the transport error: %q", w.Detail)
	}
}

// TestInstallerImageReQuestionsAProvisionalAnswer is the assertion that the
// answer is not frozen for the life of the process, which is the divergence
// G-02-3 observed across two processes.
//
// Its second half is the load-bearing one: the re-question asks *only* the
// candidate that was never ruled out. The candidate that answered is already in
// hand, so re-asking it learns nothing -- and asking it anyway would put the
// 2 x DefaultTimeout = 60.000s composition that equals writeTimeout onto the
// warm path once per interval, on a route that was answering from cache in
// 218us. That is the exact shape G-02-2 recorded a 502 at.
func TestInstallerImageReQuestionsAProvisionalAnswer(t *testing.T) {
	fake := newFakeFactory(t)
	fake.setRepoUnreachable("metal-installer")
	client := newClientWithRetryInterval(t, fake.URL, 0)
	req := installerRequest(installerModernVersion)

	if _, warnings, err := client.InstallerImage(t.Context(), req); err != nil || len(warnings) != 1 {
		t.Fatalf("first call: err = %v, warnings = %+v", err, warnings)
	}

	ref, warnings, err := client.InstallerImage(t.Context(), req)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if !strings.Contains(ref, "/installer/") {
		t.Errorf("ref = %s, want the legacy repository", ref)
	}
	if len(warnings) != 1 || warnings[0].Code != imagefactory.WarningInstallerRepoFallbackUnverified {
		t.Errorf("the answer served after a re-question dropped its warning: %+v", warnings)
	}

	if n := fake.count("GET /v2/metal-installer/manifests/" + installerModernVersion); n != 2 {
		t.Errorf("the never-ruled-out candidate was asked %d times across two calls, want 2 -- "+
			"a stale provisional entry must be re-questioned", n)
	}
	// The narrowing gate. If this reads 2 the re-question is walking the whole
	// candidate list and costs 2 x DefaultTimeout rather than 1.
	if n := fake.count("GET /v2/installer/manifests/" + installerModernVersion); n != 1 {
		t.Errorf("the candidate that already answered was asked %d times, want 1 -- "+
			"a re-question must ask only the candidates that were never ruled out, or it "+
			"composes to 2 x DefaultTimeout = 60.000s against a 60s writeTimeout", n)
	}
}

// TestInstallerImageServesAProvisionalAnswerFromTheCacheWithinTheInterval is
// the other half of the trade-off. The UAT's literal remedy -- never remember a
// name reached past silence -- makes every subsequent request re-pay the silent
// candidate's client timeout on the one route whose worst case already equals
// the response budget, so the operator would see neither the reference nor the
// warning this plan exists to show them.
func TestInstallerImageServesAProvisionalAnswerFromTheCacheWithinTheInterval(t *testing.T) {
	fake := newFakeFactory(t)
	fake.setRepoUnreachable("metal-installer")
	client := newClient(t, fake.URL) // production interval
	req := installerRequest(installerModernVersion)

	for i := range 2 {
		_, warnings, err := client.InstallerImage(t.Context(), req)
		if err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
		if len(warnings) != 1 {
			t.Fatalf("call %d served %d warnings, want 1 -- being remembered must not make a "+
				"provisional answer silent", i+1, len(warnings))
		}
	}

	if n := fake.count("GET /v2/metal-installer/manifests/" + installerModernVersion); n != 1 {
		t.Errorf("the silent candidate was asked %d times, want 1 -- inside the interval its "+
			"client timeout is paid once, not once per request", n)
	}
	if n := fake.count("GET /v2/installer/manifests/" + installerModernVersion); n != 1 {
		t.Errorf("the answering candidate was asked %d times, want 1", n)
	}
}

// TestInstallerImageRetainsAProvisionalAnswerWhenTheReQuestionFails is the case
// this plan exists to protect, and the obvious implementation regresses it.
// Evicting a provisional entry whose re-question fails turns a throttled
// registry into a 502 on the assets route one interval after the throttling
// starts, for an operator who was being served a usable reference a moment
// earlier -- the hard-error behaviour 02-04-PLAN.md:157-161 deliberately ruled
// out.
func TestInstallerImageRetainsAProvisionalAnswerWhenTheReQuestionFails(t *testing.T) {
	fake := newFakeFactory(t)
	fake.setRepoUnreachable("metal-installer")
	client := newClientWithRetryInterval(t, fake.URL, 0)
	req := installerRequest(installerModernVersion)

	if _, _, err := client.InstallerImage(t.Context(), req); err != nil {
		t.Fatalf("first call: %v", err)
	}
	before, ok := client.InstallerRepoEntryForTest(req)
	if !ok {
		t.Fatal("the provisional answer was not remembered at all")
	}

	// Over-provocation: the candidate that answered goes silent too. It must
	// not matter, because it is not asked.
	fake.setRepoUnreachable("installer")

	ref, warnings, err := client.InstallerImage(t.Context(), req)
	if err != nil {
		t.Fatalf("a failed re-question returned an error rather than the answer already in "+
			"hand: %v", err)
	}
	if !strings.Contains(ref, "/installer/") {
		t.Errorf("ref = %q, want the retained legacy reference", ref)
	}
	if len(warnings) != 1 || warnings[0].Code != imagefactory.WarningInstallerRepoFallbackUnverified {
		t.Errorf("the retained answer lost its warning: %+v", warnings)
	}
	if n := fake.count("GET /v2/installer/manifests/" + installerModernVersion); n != 1 {
		t.Errorf("the candidate that answered was asked %d times, want 1 -- it is not part of "+
			"a re-question", n)
	}

	// Re-stamped, not left stale: the cadence is one interval after the failed
	// re-question, not one interval after the original resolution. Asserted
	// through the entry itself, because at interval zero the re-stamped entry is
	// stale again immediately -- a third call re-questions candidate 1 by design
	// and its counter reads 3. The property under test is the cadence, not the
	// clock, and not a suppressed call.
	after, ok := client.InstallerRepoEntryForTest(req)
	if !ok {
		t.Fatal("the failed re-question evicted the entry")
	}
	if !after.WrittenAt.After(before.WrittenAt) {
		t.Errorf("the entry's timestamp was left at %s after a failed re-question completed; "+
			"leaving it stale makes every subsequent call re-question and re-pays the silent "+
			"candidate's timeout per request", before.WrittenAt)
	}
	if after.Repo != before.Repo {
		t.Errorf("repo = %q, want the retained %q", after.Repo, before.Repo)
	}
	if len(after.Unresolved) != 1 || after.Unresolved[0] != "metal-installer" {
		t.Errorf("unresolved = %v, want the same single name so the next re-question asks it "+
			"and nothing else", after.Unresolved)
	}
	if after.WarningCode != imagefactory.WarningInstallerRepoFallbackUnverified {
		t.Errorf("warning code = %q, want the original warning unchanged", after.WarningCode)
	}
}

// TestInstallerImagePromotesAProvisionalAnswerWhenTheReQuestionIsRefused is the
// question the warning asked, finally answered. A refusal rules the preferred
// name out, which is the one thing the warning ever claimed had not happened,
// so the entry keeps its name, drops its warning and stops expiring.
func TestInstallerImagePromotesAProvisionalAnswerWhenTheReQuestionIsRefused(t *testing.T) {
	fake := newFakeFactory(t)
	fake.setRepoUnreachable("metal-installer")
	client := newClientWithRetryInterval(t, fake.URL, 0)
	req := installerRequest(installerLegacyVersion)

	if _, warnings, err := client.InstallerImage(t.Context(), req); err != nil || len(warnings) != 1 {
		t.Fatalf("first call: err = %v, warnings = %+v", err, warnings)
	}

	// The registry comes back and says no: at installerLegacyVersion the
	// platform-prefixed name does not resolve, so it answers 404.
	fake.setRepoReachable("metal-installer")

	ref, warnings, err := client.InstallerImage(t.Context(), req)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	want := fakeHost(t, fake.URL) + "/installer/" + schematicA + ":" + installerLegacyVersion
	if ref != want {
		t.Errorf("ref  = %s\nwant = %s", ref, want)
	}
	if len(warnings) != 0 {
		t.Errorf("the preferred name is now ruled out and the warning should be gone: %+v", warnings)
	}
	if warnings == nil {
		t.Error("warnings is nil, which encodes as null")
	}

	metal := fake.count("GET /v2/metal-installer/manifests/" + installerLegacyVersion)
	legacy := fake.count("GET /v2/installer/manifests/" + installerLegacyVersion)

	// A proven entry never expires, so a third call asks nothing at all -- even
	// at interval zero. This is the contrast with the retained-provisional case.
	if _, _, err := client.InstallerImage(t.Context(), req); err != nil {
		t.Fatalf("third call: %v", err)
	}
	if n := fake.count("GET /v2/metal-installer/manifests/" + installerLegacyVersion); n != metal {
		t.Errorf("a promoted entry was re-questioned: %d manifest requests, want %d", n, metal)
	}
	if n := fake.count("GET /v2/installer/manifests/" + installerLegacyVersion); n != legacy {
		t.Errorf("a promoted entry re-asked the answering candidate: %d requests, want %d", n, legacy)
	}
}

// TestInstallerImageDoesNotCacheAResolutionWithNothingToFallBackOn keeps the
// retention rule narrow. Retaining applies to an entry that already exists; a
// first resolution that finds every candidate silent has no usable answer to
// keep, so it still fails and still caches nothing.
func TestInstallerImageDoesNotCacheAResolutionWithNothingToFallBackOn(t *testing.T) {
	fake := newFakeFactory(t)
	fake.setRepoUnreachable("metal-installer")
	fake.setRepoUnreachable("installer")
	client := newClient(t, fake.URL)
	req := installerRequest(installerModernVersion)

	ref, warnings, err := client.InstallerImage(t.Context(), req)
	if err == nil {
		t.Fatalf("no error, and it produced the reference %q", ref)
	}
	if !errors.Is(err, imagefactory.ErrUpstreamUnavailable) {
		t.Errorf("err = %v, want ErrUpstreamUnavailable -- nothing answered", err)
	}
	if ref != "" {
		t.Errorf("a failed resolution still returned a reference: %q", ref)
	}
	if warnings == nil {
		t.Error("warnings is nil, which encodes as null")
	}
	if _, ok := client.InstallerRepoEntryForTest(req); ok {
		t.Error("a failed first resolution was cached")
	}

	// And the recovery path is unchanged: the registry comes back, the answer
	// is proven, and nothing was frozen in between.
	fake.setRepoReachable("metal-installer")
	fake.setRepoReachable("installer")
	ref, warnings, err = client.InstallerImage(t.Context(), req)
	if err != nil {
		t.Fatalf("after the registry recovered: %v", err)
	}
	if !strings.Contains(ref, "/metal-installer/") {
		t.Errorf("ref = %s, want the platform-prefixed repository", ref)
	}
	if len(warnings) != 0 {
		t.Errorf("a proven resolution carried warnings: %+v", warnings)
	}
}

// TestInstallerImageProvenNameCarriesNoWarning is the mirror: when the
// preferred candidate answers, nothing was left unheard, so there is nothing to
// say and the slice is empty rather than nil.
func TestInstallerImageProvenNameCarriesNoWarning(t *testing.T) {
	fake := newFakeFactory(t)
	client := newClient(t, fake.URL)
	req := installerRequest(installerModernVersion)

	_, warnings, err := client.InstallerImage(t.Context(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if warnings == nil {
		t.Fatal("warnings is nil, which encodes as null -- that reads as 'the server did not check'")
	}
	if len(warnings) != 0 {
		t.Fatalf("a proven name produced %+v", warnings)
	}

	encoded, err := json.Marshal(warnings)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if string(encoded) != "[]" {
		t.Errorf("encoded as %s, want []", encoded)
	}

	entry, ok := client.InstallerRepoEntryForTest(req)
	if !ok {
		t.Fatal("the proven answer was not cached")
	}
	if len(entry.Unresolved) != 0 {
		t.Errorf("a proven entry records unresolved candidates %v, so it would expire", entry.Unresolved)
	}
}

// TestWithInstallerRepoRetryIntervalRejectsANegativeValue follows the register
// WithTimeout already uses. Zero is legal here and means "re-question on every
// call", which is what makes both branches drivable without a fake clock; a
// negative interval is not a shorter one, it is a mistake.
func TestWithInstallerRepoRetryIntervalRejectsANegativeValue(t *testing.T) {
	if _, err := imagefactory.New("https://factory.example",
		imagefactory.WithInstallerRepoRetryInterval(-time.Second)); err == nil {
		t.Fatal("a negative re-question interval was accepted")
	}
	if _, err := imagefactory.New("https://factory.example",
		imagefactory.WithInstallerRepoRetryInterval(0)); err != nil {
		t.Fatalf("zero must be legal: %v", err)
	}
}

// newClientWithRetryInterval builds a client whose provisional installer-repo
// entries expire after d. Options apply at construction, so a test that needs
// two different intervals needs two clients -- and a second client has a cold
// cache and cannot see the first one's entries.
func newClientWithRetryInterval(t *testing.T, baseURL string, d time.Duration) *imagefactory.Client {
	t.Helper()
	c, err := imagefactory.New(baseURL, imagefactory.WithInstallerRepoRetryInterval(d))
	if err != nil {
		t.Fatalf("New(%q): %v", baseURL, err)
	}
	return c
}

// concurrentProbeDelay is how long the fake takes to go silent in the cases
// below. It has to outlast an entire competing resolution against the same fake
// -- a 200 over loopback, plus the cache write -- which is microseconds here, so
// it is generous by three orders of magnitude and still leaves the test well
// under a second. It is what turns "these two goroutines might interleave badly"
// into "this goroutine writes second, every run".
const concurrentProbeDelay = 250 * time.Millisecond

// installerImageConcurrently calls InstallerImage n times at once for one
// request and returns the reference each call was served.
//
// It asserts on the way out that no call carried a warning, which is not
// incidental: in both cases below the registry proves the preferred name to at
// least one goroutine, so a provisional answer coming back to any caller means a
// proven one was either discarded at the write or not adopted by the caller that
// lost the race.
func installerImageConcurrently(
	t *testing.T,
	client *imagefactory.Client,
	r imagefactory.AssetRequest,
	n int,
) []string {
	t.Helper()

	ctx := t.Context()
	refs := make([]string, n)
	warningCounts := make([]int, n)
	errs := make([]error, n)

	// Released all at once rather than being raced from the loop, so the calls
	// overlap instead of starting a goroutine-creation apart.
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			ref, warnings, err := client.InstallerImage(ctx, r)
			refs[i], warningCounts[i], errs[i] = ref, len(warnings), err
		}()
	}
	close(start)
	wg.Wait()

	// After the join, never inside the goroutines: t.Fatalf from a goroutine
	// other than the test's own does not stop the test.
	for i := range n {
		if errs[i] != nil {
			t.Fatalf("concurrent call %d: %v", i, errs[i])
		}
		if warningCounts[i] != 0 {
			t.Errorf("concurrent call %d was served %d warnings, want 0 -- the preferred name "+
				"was proven during this window, so every caller must end up with the proven "+
				"answer rather than a provisional one", i, warningCounts[i])
		}
	}
	return refs
}

// assertProvenInstallerEntry fails unless the cache holds repo as a proven
// entry: every candidate accounted for, and therefore no warning and no expiry.
func assertProvenInstallerEntry(
	t *testing.T,
	client *imagefactory.Client,
	r imagefactory.AssetRequest,
	repo string,
) {
	t.Helper()

	entry, ok := client.InstallerRepoEntryForTest(r)
	if !ok {
		t.Fatal("the resolved name was not remembered at all")
	}
	if entry.Repo != repo {
		t.Errorf("the cache holds %q, want the proven %q -- a slower resolver overwrote a "+
			"proven entry with what it had learned before that entry existed", entry.Repo, repo)
	}
	if len(entry.Unresolved) != 0 {
		t.Errorf("the cached entry still lists %v as never ruled out, so it is provisional and "+
			"will be re-questioned; a proven entry was demoted", entry.Unresolved)
	}
	if entry.WarningCode != "" {
		t.Errorf("the cached entry carries the warning %q after the preferred name was proven",
			entry.WarningCode)
	}
}

// TestInstallerImageNeverRevertsAProvenNameUnderConcurrentResolution is the
// regression for the lost update storeInstallerRepo exists to prevent, and it is
// the first concurrency case in this file at all.
//
// Both resolution paths hold no lock while they ask the registry, so two
// requests on one key can be in flight with observations made a full client
// timeout apart. When the slower one's write is unconditional it lands on top of
// the faster one's, and because "slower" here means "the one that did not get an
// answer", the entry left behind is the less certain of the two. The two callers
// are then served two different repository names for one schematic at one
// version, in one process, at one moment -- which is G-02-3 verbatim, and
// InstallerImage's doc comment records what the wrong name costs: an upgrade
// that reports success and drops every system extension the node was built with.
//
// The interleaving is forced rather than hoped for. The fake hands the silence
// to whichever request arrives first and delays it, so the goroutine holding the
// stale observation is always the one that writes last. Run under -race, this
// also covers the map access itself.
func TestInstallerImageNeverRevertsAProvenNameUnderConcurrentResolution(t *testing.T) {
	// The cold path: nothing cached, and the resolution that had to fall back
	// finishes last.
	t.Run("cold cache", func(t *testing.T) {
		fake := newFakeFactory(t)
		// One request finds the preferred name silent -- slowly -- and every
		// request after it finds the name answering. The silent one resolves
		// past it to the legacy name and writes a provisional entry last.
		fake.setRepoSilentForNext("metal-installer", 1, concurrentProbeDelay)
		client := newClient(t, fake.URL)
		req := installerRequest(installerModernVersion)

		want := fakeHost(t, fake.URL) + "/metal-installer/" + schematicA + ":" + installerModernVersion
		for i, ref := range installerImageConcurrently(t, client, req, 2) {
			if ref != want {
				t.Errorf("concurrent call %d was served\n  %s\nwant\n  %s\n"+
					"two concurrent callers must not be handed two repository names for one "+
					"schematic at one version", i, ref, want)
			}
		}
		assertProvenInstallerEntry(t, client, req, "metal-installer")
	})

	// The re-question path: a stale provisional entry, one re-question that
	// proves the preferred name and one that hears nothing.
	t.Run("stale provisional entry", func(t *testing.T) {
		fake := newFakeFactory(t)
		fake.setRepoUnreachable("metal-installer")
		client := newClientWithRetryInterval(t, fake.URL, 0)
		req := installerRequest(installerModernVersion)

		if _, warnings, err := client.InstallerImage(t.Context(), req); err != nil || len(warnings) != 1 {
			t.Fatalf("seeding the provisional entry: err = %v, warnings = %+v", err, warnings)
		}

		// The registry comes back, but not for the request that is already
		// asking: that one still hears nothing, and hears it slowly.
		fake.setRepoReachable("metal-installer")
		fake.setRepoSilentForNext("metal-installer", 1, concurrentProbeDelay)

		want := fakeHost(t, fake.URL) + "/metal-installer/" + schematicA + ":" + installerModernVersion
		for i, ref := range installerImageConcurrently(t, client, req, 2) {
			if ref != want {
				t.Errorf("concurrent call %d was served\n  %s\nwant\n  %s\n"+
					"a re-question that learned nothing must not revert the one that proved "+
					"the preferred name", i, ref, want)
			}
		}
		assertProvenInstallerEntry(t, client, req, "metal-installer")

		// The narrowing gate again, under concurrency: a re-question asks only
		// the candidate that was never ruled out, so the name that already
		// answered is asked once in the whole test -- during the seeding call.
		// Two concurrent re-questions walking the full list would read 3 here,
		// and each would cost 2 x DefaultTimeout.
		if n := fake.count("GET /v2/installer/manifests/" + installerModernVersion); n != 1 {
			t.Errorf("the candidate that already answered was asked %d times, want 1 -- "+
				"a re-question must ask only the candidates that were never ruled out", n)
		}
	})
}
