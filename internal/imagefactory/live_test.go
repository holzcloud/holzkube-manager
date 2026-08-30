package imagefactory_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube/internal/imagefactory"
)

// liveEnv opts a run in to the contract test against the real Image Factory.
const liveEnv = "HOLZKUBE_FACTORY_LIVE"

// TestLiveFactory is the fake-drift guard for this package: it checks that the
// recordings in testdata/ and the behaviours reproduced in fake_test.go still
// describe factory.talos.dev.
//
// Every other test in this package runs against a fake, which means every other
// test would keep passing after the upstream changed. This is the one that
// would not -- and it is the counterpart of the method-coverage test that
// guards the talossim fake in the sibling track.
//
// It skips loudly, in the idiom internal/depguard_test.go uses for the same
// reason: a skipped upstream check that prints nothing is indistinguishable
// from a check that passed, and a guard nobody can see is a guard nobody runs.
func TestLiveFactory(t *testing.T) {
	if os.Getenv(liveEnv) != "1" {
		t.Skipf("skipping the live Image Factory contract test: %s is not set to 1. "+
			"Nothing in this run verified that testdata/versions.json, "+
			"testdata/extensions-%s.json, the recorded schematic id %s, or the SecureBoot "+
			"installer matrix -- which of %v answer at which Talos version -- still match "+
			"factory.talos.dev, so a fake that has drifted from the real upstream would "+
			"look exactly like this.", liveEnv, catalogVersion, consoleSchematicID, liveInstallerRepos)
	}

	client, err := imagefactory.New(imagefactory.DefaultBaseURL, imagefactory.WithTimeout(60*time.Second))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := t.Context()

	t.Run("versions still contain the pinned Talos release", func(t *testing.T) {
		versions, err := client.Versions(ctx)
		if err != nil {
			t.Fatalf("Versions: %v", err)
		}
		if !slices.Contains(versions, catalogVersion) {
			t.Errorf("the live version list no longer contains %s; the recorded fixture and "+
				"holzkube's supported range disagree with upstream", catalogVersion)
		}
	})

	t.Run("the version-scoped catalog still lists the recorded extension", func(t *testing.T) {
		catalog, err := client.Extensions(ctx, catalogVersion)
		if err != nil {
			t.Fatalf("Extensions(%s): %v", catalogVersion, err)
		}
		if !slices.Contains(imagefactory.ExtensionNames(catalog), "siderolabs/intel-ucode") {
			t.Errorf("the live catalog for %s no longer lists siderolabs/intel-ucode", catalogVersion)
		}
	})

	t.Run("the recorded payload still produces the recorded id", func(t *testing.T) {
		// The strongest assertion in this file: it proves the canonical
		// serialisation written in schematicid.go still agrees with upstream's,
		// which is the whole of FACT-06. CreateSchematic already refuses a
		// mismatch, so a drift shows up as ErrSchematicIDMismatch.
		created, err := client.CreateSchematic(ctx, goodSchematic())
		if errors.Is(err, imagefactory.ErrSchematicIDMismatch) {
			t.Fatalf("the canonical serialiser has drifted from upstream: %v", err)
		}
		if err != nil {
			t.Fatalf("CreateSchematic: %v", err)
		}
		if created.ID != consoleSchematicID {
			t.Errorf("live id = %s, want the recorded %s", created.ID, consoleSchematicID)
		}
	})

	t.Run("creation is still not validation", func(t *testing.T) {
		// The trap the fake reproduces. If upstream ever starts rejecting a
		// bogus extension at POST time this fails, and the day it does is the
		// day the probe stops being load-bearing -- which is worth knowing
		// rather than assuming in either direction.
		bogus := imagefactory.Schematic{Customization: imagefactory.Customization{
			SystemExtensions: imagefactory.SystemExtensions{
				OfficialExtensions: []string{"siderolabs/totally-not-real-ext"},
			},
		}}
		created, err := client.CreateSchematic(ctx, bogus)
		if err != nil {
			t.Fatalf("upstream refused a bogus extension at creation time (a behaviour change "+
				"worth investigating, not a bug in this test): %v", err)
		}
		probeErr := client.ProbeBuildable(ctx, created.ID, catalogVersion, imagefactory.ArchAMD64)
		if !errors.Is(probeErr, imagefactory.ErrSchematicNotBuildable) {
			t.Errorf("probing a schematic with a non-existent extension returned %v, "+
				"want ErrSchematicNotBuildable", probeErr)
		}
	})

	t.Run("a good schematic still builds", func(t *testing.T) {
		if err := client.ProbeBuildable(ctx, consoleSchematicID, catalogVersion, imagefactory.ArchAMD64); err != nil {
			t.Errorf("ProbeBuildable on the recorded schematic: %v", err)
		}
	})

	installerNameMatrixSubtests(ctx, t, client)
}

// The two repository names installerCandidates asks about first for a metal
// request: the platform-prefixed ordinary name and the platform-prefixed
// SecureBoot one. These are the names the drift guard asserts about, so they
// are spelled once here rather than at each assertion.
const (
	preferredInstallerRepo           = "metal-installer"
	preferredSecureBootInstallerRepo = "metal-installer-secureboot"
)

// liveInstallerRepos is every repository name the resolver can ask about, in
// the two ordered pairs installerCandidates produces.
var liveInstallerRepos = []string{
	preferredInstallerRepo, "installer",
	preferredSecureBootInstallerRepo, "installer-secureboot",
}

// liveOlderVersion is a supported Talos version older than the pinned one. The
// installer repository name is version-dependent (PITFALLS P9(d)), so a matrix
// measured at one version says nothing about the rest of the range.
const liveOlderVersion = "v1.12.0"

// liveExpect is what this file expects the registry to answer for one
// (version, repository) cell.
type liveExpect int

const (
	// liveUnknown: nobody has checked. The run logs what it finds and asserts
	// nothing, which is how a cell stops being unknown.
	liveUnknown liveExpect = iota
	// liveAnswers: observed answering. A contradiction is a real drift.
	liveAnswers
	// liveSilent: observed not answering. A contradiction is a real drift.
	liveSilent
)

// liveInstallerMatrix is what the *registry* is expected to answer, and it is
// deliberately not fake_test.go's installerRepos.
//
// The two tables are about different things and must stay apart. installerRepos
// is a branch-coverage fixture: its installerLegacyVersion row pins v1.9.0 to
// "installer" alone so TestInstallerImageFallsBackToTheLegacyName has a version
// where the legacy fallback is the only path, while 02-04-SUMMARY.md:387 records
// metal-installer@v1.9.0 answering 200 confirmed live. Transcribing this matrix
// into that fixture would therefore break a passing test in order to satisfy a
// check -- so this subtest logs the observed matrix in full whatever it says,
// and only fails where it contradicts the expectations written here. Recording
// the observation is this test's job; rewriting the fixture is nobody's.
var liveInstallerMatrix = map[string]map[string]liveExpect{
	// Measured 2026-08-29 against factory.talos.dev for the recorded schematic:
	// all four names answer, and the two SecureBoot names carry a different
	// image digest than the two ordinary ones (02-UAT.md G-02-4).
	catalogVersion: {
		"metal-installer": liveAnswers, "installer": liveAnswers,
		"metal-installer-secureboot": liveAnswers, "installer-secureboot": liveAnswers,
	},
	// Measured 2026-08-29 at the oldest supported version, in the run that
	// closed G-02-4: all four names answer here too. This is what settles the
	// question T-02-53 asks -- at both ends of the supported range a SecureBoot
	// request resolves, so the "502 with no installer reference" path
	// installerCandidates argues for is not reachable at v1.12.0 or v1.13.9.
	//
	// It took two runs to get this row. The first exceeded the 60s client
	// timeout on both metal-prefixed names without answering, which is
	// factory.talos.dev throttling (WINDOWS entry 5) and not a verdict -- the
	// reason this test logs a transport failure as "not observed" rather than
	// recording it as a name that does not resolve.
	//
	// Still unprobed: v1.13.x below the pin and all of v1.14.
	liveOlderVersion: {
		"metal-installer": liveAnswers, "installer": liveAnswers,
		"metal-installer-secureboot": liveAnswers, "installer-secureboot": liveAnswers,
	},
}

// installerNameOutcome is what one InstallerImage answer entitles a reader to
// say about which repository name the registry resolves.
type installerNameOutcome int

const (
	// installerNameNotObserved -- the answer was reached past a candidate that
	// never answered, so it records which name was *reachable*, not which name
	// the registry serves. Neither a pass nor a defect.
	installerNameNotObserved installerNameOutcome = iota

	// installerNameResolved -- the expected repository answered and every
	// candidate was accounted for.
	installerNameResolved

	// installerNameDrifted -- a repository other than the expected one
	// answered, with nothing unheard to explain it. A real drift.
	installerNameDrifted
)

// installerNameCheck is one answer's verdict together with the sentence to
// report it by.
type installerNameCheck struct {
	outcome installerNameOutcome

	// repo is the repository segment parsed out of the reference, or empty
	// when the reference could not be parsed.
	repo string

	// reason is the message to skip, log or fail with. Always non-empty.
	reason string
}

// checkInstallerName is the drift guard's one assertion about a resolved
// installer reference, in a form that can be exercised without a network.
//
// It answers three questions at once and keeps them apart, which is G-02-18:
//
//  1. Is this an observation at all? A reference carrying any warning was
//     reached past a candidate the resolver never heard from --
//     resolveInstallerRepo returns ErrUpstreamUnavailable only when *no*
//     candidate answered 2xx, so a partial throttle produces a nil error, a
//     usable reference and a warning. Such an answer says nothing about which
//     name the registry prefers, and reporting it as a pass is exactly the
//     failure the guard exists to catch.
//  2. If it is an observation, is it the expected name? By exact repository
//     segment, never by substring: "installer-secureboot" contains
//     "-secureboot/" just as well as "metal-installer-secureboot" does, and a
//     schematic id is 64 hex characters that a substring test would also read.
//  3. If it is not, say both names, because the message is the whole value of
//     a drift guard that fires once a year.
func checkInstallerName(ref string, warnings []imagefactory.Warning, want string) installerNameCheck {
	if len(warnings) > 0 {
		// Deliberately any warning and not one code in particular. Every
		// "installer." warning this package emits is a statement about how the
		// name in this reference was obtained, and every one of them means the
		// same thing here: a candidate earlier in the preference order was
		// never heard from, so the name that answered is the name that was
		// reachable. Keying on a single code would go quietly blind the day a
		// second one is added, which is the shape of failure this whole check
		// exists to remove.
		repo, _ := installerRepoSegment(ref)
		return installerNameCheck{
			outcome: installerNameNotObserved,
			repo:    repo,
			reason: fmt.Sprintf("the reference %q carries %s, so the repository %q answered past a "+
				"candidate that was never ruled out. This run observed which name was reachable, "+
				"not which name the registry serves, and it is neither a pass nor a drift",
				ref, strings.Join(warningCodes(warnings), ", "), repo),
		}
	}

	repo, ok := installerRepoSegment(ref)
	if !ok {
		return installerNameCheck{
			outcome: installerNameDrifted,
			reason: fmt.Sprintf("the reference %q is not host/[prefix/]repository/id:tag, so the "+
				"repository name it carries cannot be read at all", ref),
		}
	}
	if repo != want {
		return installerNameCheck{
			outcome: installerNameDrifted,
			repo:    repo,
			reason: fmt.Sprintf("the reference %q names the repository %q, want %q",
				ref, repo, want),
		}
	}
	return installerNameCheck{
		outcome: installerNameResolved,
		repo:    repo,
		reason:  fmt.Sprintf("%s resolved to the repository %q", ref, repo),
	}
}

// warningCodes is the codes of a warning slice, for a message that has to name
// what it saw without repeating a paragraph of operator-facing prose.
func warningCodes(warnings []imagefactory.Warning) []string {
	codes := make([]string, 0, len(warnings))
	for _, w := range warnings {
		codes = append(codes, w.Code)
	}
	return codes
}

// installerRepoSegment returns the repository segment of an OCI reference of
// the shape host[:port]/[prefix/]repository/id:tag.
//
// It splits rather than searches. The repository is the second-to-last path
// element whatever the host, the port and the optional path prefix look like,
// and every substring alternative can be satisfied by the 64 hex characters of
// the schematic id sitting immediately after it.
func installerRepoSegment(ref string) (string, bool) {
	parts := strings.Split(ref, "/")
	if len(parts) < 3 {
		return "", false
	}
	repo := parts[len(parts)-2]
	if repo == "" {
		return "", false
	}
	return repo, true
}

// installerNameMatrixSubtests answers G-02-4's third open item: the version
// range over which each installer repository name resolves.
//
// It has two halves and they must not be confused. The first is an assertion:
// the pinned version must resolve to two different references with and without
// SecureBoot, because that pairing is what this plan exists to settle and a
// live disagreement there is a real defect. The second is a recording: it
// probes every name at two versions, logs the whole matrix, and fails only
// where the observation contradicts liveInstallerMatrix above.
func installerNameMatrixSubtests(ctx context.Context, t *testing.T, client *imagefactory.Client) {
	t.Helper()

	t.Run("SecureBoot resolves to a different installer than the ordinary request", func(t *testing.T) {
		base := imagefactory.AssetRequest{
			SchematicID: consoleSchematicID,
			Version:     catalogVersion,
			Arch:        imagefactory.ArchAMD64,
			Platform:    imagefactory.PlatformMetal,
		}
		// A registry that did not answer is a non-observation, not a defect --
		// the same distinction registryRefused draws, applied to this test.
		// factory.talos.dev throttles (WINDOWS entry 5), and failing the run on
		// a throttle would train a reader to ignore this test. There is no
		// retry and no raised timeout here: every budget in this package is
		// 02-DECISION-probe-budget.md's.
		//
		// The second return value is bound rather than discarded, and that is
		// the whole of G-02-18. resolveInstallerRepo returns
		// ErrUpstreamUnavailable only when *no* candidate answered 2xx, so the
		// errors.Is checks below never fire on a partial throttle: the
		// preferred name times out, the legacy one answers, and a nil error
		// comes back with the fallback warning riding it. checkInstallerName is
		// what reads that warning; cffa851 threw it away.
		plain, plainWarnings, err := client.InstallerImage(ctx, base)
		if errors.Is(err, imagefactory.ErrUpstreamUnavailable) {
			t.Skipf("NOT OBSERVED: the ordinary installer did not resolve at %s, so the "+
				"SecureBoot pairing went unverified in this run: %v", catalogVersion, err)
		}
		if err != nil {
			t.Fatalf("InstallerImage without SecureBoot: %v", err)
		}
		base.SecureBoot = true
		secure, secureWarnings, err := client.InstallerImage(ctx, base)
		if errors.Is(err, imagefactory.ErrUpstreamUnavailable) {
			t.Skipf("NOT OBSERVED: the SecureBoot installer did not resolve at %s, so the "+
				"pairing went unverified in this run (the ordinary one resolved to %s): %v",
				catalogVersion, plain, err)
		}
		if err != nil {
			// ErrSchematicNotBuildable here is the finding T-02-53 names: at
			// this version neither SecureBoot name resolves, so a SecureBoot
			// asset request answers 502 with no installer reference.
			t.Fatalf("InstallerImage with SecureBoot: %v", err)
		}
		t.Logf("live installer references at %s:\n  plain  = %s (warnings: %v)\n  secure = %s (warnings: %v)",
			catalogVersion, plain, warningCodes(plainWarnings), secure, warningCodes(secureWarnings))

		plainCheck := checkInstallerName(plain, plainWarnings, preferredInstallerRepo)
		secureCheck := checkInstallerName(secure, secureWarnings, preferredSecureBootInstallerRepo)

		// A provisional answer is skipped rather than failed, for the reason
		// the transport failures above are: factory.talos.dev throttles
		// (WINDOWS entry 5) and a throttle is not a defect. What changed is
		// that it is no longer reported as a pass.
		for _, c := range []installerNameCheck{plainCheck, secureCheck} {
			if c.outcome == installerNameNotObserved {
				t.Skipf("NOT OBSERVED: %s", c.reason)
			}
		}
		if plainCheck.outcome != installerNameResolved {
			t.Errorf("the ordinary installer name has drifted: %s", plainCheck.reason)
		}
		// By exact repository segment, not by containment: the old check was
		// strings.Contains(secure, "-secureboot/"), which the legacy
		// "installer-secureboot" satisfies exactly as well as the
		// platform-prefixed name does -- and the two are different images
		// (02-UAT.md G-02-13), so the difference is not cosmetic.
		if secureCheck.outcome != installerNameResolved {
			t.Errorf("the SecureBoot installer name has drifted: %s", secureCheck.reason)
		}
		if plain == secure {
			t.Errorf("a SecureBoot request resolved to the same reference as an ordinary one (%s); "+
				"the SecureBoot installer is a different image selected by repository name, and "+
				"pairing a SecureBoot ISO with the ordinary installer is G-02-4", plain)
		}
	})

	t.Run("the installer-name matrix is what this file records", func(t *testing.T) {
		for _, version := range []string{catalogVersion, liveOlderVersion} {
			for _, repo := range liveInstallerRepos {
				m, err := liveManifestProbe(ctx, repo, consoleSchematicID, version)
				if err != nil {
					// A transport failure is not an observation. Say so and
					// move on: factory.talos.dev throttles (WINDOWS entry 5),
					// and a retry loop here would be a budget change this plan
					// may not make.
					t.Logf("MATRIX %s %-28s -> not observed: %v", version, repo, err)
					continue
				}
				answered := m.answered
				// The digest is logged and never asserted on, and it is
				// deliberately not copied into a source comment. A digest in a
				// comment is a fact with an expiry date that nothing in the
				// build checks, which is exactly how installer.go's previous
				// pair went stale (02-UAT.md G-02-13). Re-measuring it is one
				// test run; re-reading a stale comment is a wrong belief.
				t.Logf("MATRIX %s %-28s -> answered=%t digest=%s", version, repo, answered, m.digest)

				switch liveInstallerMatrix[version][repo] {
				case liveAnswers:
					if !answered {
						t.Errorf("%s@%s no longer answers; this file records it as answering", repo, version)
					}
				case liveSilent:
					if answered {
						t.Errorf("%s@%s now answers; this file records it as silent", repo, version)
					}
				case liveUnknown:
					// Nothing to contradict. The log line above is the point.
				}
			}
		}
	})
}

// liveManifest is one registry answer about one (repository, version) cell.
type liveManifest struct {
	// answered reports a 2xx.
	answered bool

	// digest identifies the image the repository name selects, which is the
	// only way to tell two names for one image apart from two names for two
	// images. Empty when the registry did not answer 2xx.
	digest string
}

// liveManifestProbe asks the registry whether a repository name carries a
// manifest for this schematic at this version, with the same Accept header
// installer.go sends -- a registry given none of those media types can answer
// 404 for a manifest that exists, which would look exactly like a name that
// does not resolve.
//
// A non-2xx that the registry actually answered is an observation
// (answered=false, nil); only a transport failure is a non-observation (error).
//
// It also reports the manifest digest, because "is installer-secureboot another
// name for metal-installer-secureboot" is a question only a digest answers, and
// round 1 answered it wrongly from a pair of literals nothing re-measured
// (02-UAT.md G-02-13). The registry's own Docker-Content-Digest is preferred;
// when it is absent the SHA-256 of the body is computed here, which is what
// that header is defined to be.
func liveManifestProbe(ctx context.Context, repo, id, version string) (liveManifest, error) {
	u := imagefactory.DefaultBaseURL + "/v2/" + repo + "/" + id + "/manifests/" + version
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return liveManifest{}, err
	}
	req.Header.Set("Accept", "application/vnd.oci.image.index.v1+json, "+
		"application/vnd.oci.image.manifest.v1+json, "+
		"application/vnd.docker.distribution.manifest.list.v2+json, "+
		"application/vnd.docker.distribution.manifest.v2+json")

	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return liveManifest{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return liveManifest{}, err
	}
	if resp.StatusCode/100 != 2 {
		return liveManifest{}, nil
	}

	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		sum := sha256.Sum256(body)
		digest = "sha256:" + hex.EncodeToString(sum[:]) + " (computed from the body; the registry sent no Docker-Content-Digest)"
	}
	return liveManifest{answered: true, digest: digest}, nil
}
