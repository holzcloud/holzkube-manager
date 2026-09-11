package upgrade_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/upgrade"
)

func v(t *testing.T, s string) upgrade.Version {
	t.Helper()
	parsed, err := upgrade.ParseVersion(s)
	if err != nil {
		t.Fatalf("ParseVersion(%q): %v", s, err)
	}
	return parsed
}

// TestTheChainNamesEveryIntermediateMinor is UPG-05, and it is the reason
// there is no "latest" button.
//
// Talos upgrades one minor at a time. A button that quietly did two upgrades
// in a row would do the second one against a cluster whose state nobody looked
// at -- and the health gate between them is the thing that matters most.
func TestTheChainNamesEveryIntermediateMinor(t *testing.T) {
	t.Parallel()

	available := []upgrade.Version{
		v(t, "v1.12.3"), v(t, "v1.13.2"), v(t, "v1.13.9"), v(t, "v1.14.0"), v(t, "v1.14.1"),
	}

	steps, err := upgrade.Chain(v(t, "v1.12.3"), v(t, "v1.14.1"), available)
	if err != nil {
		t.Fatalf("Chain: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("the chain from 1.12 to 1.14 has %d step(s), want 2: %+v", len(steps), steps)
	}

	// The newest available patch of the intermediate minor, not the oldest: a
	// bug fixed in a patch is fixed for a reason, and passing through a
	// known-broken release on the way somewhere else is self-inflicted.
	if got := steps[0].To.String(); got != "v1.13.9" {
		t.Errorf("the intermediate step is %s, want v1.13.9", got)
	}
	if got := steps[1].To.String(); got != "v1.14.1" {
		t.Errorf("the final step is %s, want v1.14.1", got)
	}

	// The middle step is the one an operator will ask about, because it is not
	// the version they chose.
	if !strings.Contains(steps[0].Why, "on the way") {
		t.Errorf("the intermediate step does not say why it exists: %q", steps[0].Why)
	}
}

// TestAMissingIntermediateReleaseIsRefusedRatherThanSkipped.
func TestAMissingIntermediateReleaseIsRefusedRatherThanSkipped(t *testing.T) {
	t.Parallel()

	// No 1.13 at all.
	available := []upgrade.Version{v(t, "v1.12.3"), v(t, "v1.14.1")}

	_, err := upgrade.Chain(v(t, "v1.12.3"), v(t, "v1.14.1"), available)
	if !errors.Is(err, upgrade.ErrNoPath) {
		t.Fatalf("a chain with no intermediate release returned %v, want ErrNoPath", err)
	}
	if !strings.Contains(err.Error(), "1.13") {
		t.Errorf("the refusal %q does not name the missing minor", err)
	}
}

// TestAPreReleaseIsNeverOnThePath.
//
// A chain that routed a cluster through a release candidate would be routing
// it through software whose own project does not call it finished.
func TestAPreReleaseIsNeverOnThePath(t *testing.T) {
	t.Parallel()

	available := []upgrade.Version{
		v(t, "v1.13.0-beta.1"), v(t, "v1.13.0-rc.1"), v(t, "v1.14.0"),
	}

	_, err := upgrade.Chain(v(t, "v1.12.3"), v(t, "v1.14.0"), available)
	if !errors.Is(err, upgrade.ErrNoPath) {
		t.Fatalf("the chain used a pre-release as its 1.13 step: %v", err)
	}

	// And the filter says so at the other end too.
	filtered := upgrade.Releases([]string{"v1.13.9", "v1.14.0-rc.1", "v1.13.0-alpha.0"})
	for _, r := range filtered {
		if r.IsPreRelease() {
			t.Errorf("Releases kept the pre-release %s", r)
		}
	}
}

// TestADowngradeIsRefused.
//
// Not because it would fail -- it might appear to work -- but because etcd has
// already written a newer storage version by then.
func TestADowngradeIsRefused(t *testing.T) {
	t.Parallel()

	_, err := upgrade.Chain(v(t, "v1.14.0"), v(t, "v1.13.9"), nil)
	if !errors.Is(err, upgrade.ErrNoPath) {
		t.Fatalf("a downgrade returned %v, want ErrNoPath", err)
	}
	if !strings.Contains(err.Error(), "etcd") {
		t.Errorf("the refusal %q does not say why a downgrade is not merely unsupported", err)
	}
}

// TestATalosUpgradeThatWouldStrandKubernetesIsBlocked is UPG-06, and it is
// blocked rather than warned.
//
// The state it prevents has no good way out: the node comes back on the new
// Talos, the cluster is then running a Kubernetes version that Talos does not
// support, and the Kubernetes upgrade that would fix it is performed by that
// same Talos.
func TestATalosUpgradeThatWouldStrandKubernetesIsBlocked(t *testing.T) {
	t.Parallel()

	// Talos 1.14 supports Kubernetes 1.32 to 1.35. A cluster on 1.31 would be
	// below the floor.
	check := upgrade.CheckStrand(v(t, "v1.14.0"), "v1.31.4")
	if !check.Blocked {
		t.Fatal("a Talos upgrade that drops the running Kubernetes out of support was allowed")
	}
	for _, want := range []string{"1.31", "1.32"} {
		if !strings.Contains(check.Sentence+check.Remedy, want) {
			t.Errorf("the block does not mention %q: %q / %q", want, check.Sentence, check.Remedy)
		}
	}
	if check.Remedy == "" {
		t.Error("the block carries no concrete instruction, which UPG-06 asks for by name")
	}

	// And the ordinary case is allowed, with the sentence that says why.
	ok := upgrade.CheckStrand(v(t, "v1.14.0"), "v1.34.1")
	if ok.Blocked {
		t.Fatalf("a supported pair was blocked: %s", ok.Sentence)
	}
	if ok.Sentence == "" {
		t.Error("an allowed upgrade carries no statement of why it is allowed")
	}
}

// TestAnUnknownTalosVersionIsBlockedRatherThanExtrapolated.
//
// An extrapolated window is an upgrade that is allowed to strand a cluster.
func TestAnUnknownTalosVersionIsBlockedRatherThanExtrapolated(t *testing.T) {
	t.Parallel()

	check := upgrade.CheckStrand(v(t, "v1.99.0"), "v1.34.1")
	if !check.Blocked {
		t.Fatal("an upgrade to a Talos version with no compatibility row was allowed")
	}
	if check.Remedy == "" {
		t.Error("the block says nothing about what to do instead")
	}
}

// TestAnUnknownKubernetesVersionIsBlocked.
//
// "We do not know what this cluster runs" is not a reason to proceed.
func TestAnUnknownKubernetesVersionIsBlocked(t *testing.T) {
	t.Parallel()

	if check := upgrade.CheckStrand(v(t, "v1.14.0"), ""); !check.Blocked {
		t.Fatal("an upgrade was allowed against a cluster whose Kubernetes version is unknown")
	}
}

// TestTheKubernetesChainPassesThroughEveryMinor.
//
// The kubelet may be one minor behind the API server and no more, which is the
// same structural reason Talos upgrades one minor at a time.
func TestTheKubernetesChainPassesThroughEveryMinor(t *testing.T) {
	t.Parallel()

	steps, err := upgrade.KubernetesChain(v(t, "v1.32.4"), v(t, "v1.34.1"), "v1.14.0")
	if err != nil {
		t.Fatalf("KubernetesChain: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("the chain from 1.32 to 1.34 has %d step(s), want 2: %+v", len(steps), steps)
	}
	if steps[0].To.Minor != 33 || steps[1].To.String() != "v1.34.1" {
		t.Errorf("the chain is %s then %s", steps[0].To, steps[1].To)
	}
}

// TestAKubernetesChainBeyondWhatTalosSupportsIsRefused is the mirror of
// UPG-06: the same gate, from the other side.
func TestAKubernetesChainBeyondWhatTalosSupportsIsRefused(t *testing.T) {
	t.Parallel()

	// Talos 1.12 supports Kubernetes up to 1.33.
	_, err := upgrade.KubernetesChain(v(t, "v1.32.4"), v(t, "v1.35.0"), "v1.12.3")
	if !errors.Is(err, upgrade.ErrWouldStrand) {
		t.Fatalf("a Kubernetes upgrade past what the running Talos supports returned %v", err)
	}
}

// TestReleasesFiltersToWhatThisBuildWasTestedAgainst.
func TestReleasesFiltersToWhatThisBuildWasTestedAgainst(t *testing.T) {
	t.Parallel()

	got := upgrade.Releases([]string{
		"v1.9.0", "v1.13.9", "v1.14.0", "v1.99.0", "not-a-version", "v1.14.1",
	})

	for _, r := range got {
		if r.Minor < 12 || r.Minor > 14 {
			t.Errorf("Releases kept %s, which is outside the range this build was tested against", r)
		}
	}

	// Newest first, which is the order an operator reads them in.
	for i := 1; i < len(got); i++ {
		if got[i-1].Less(got[i]) {
			t.Errorf("Releases returned %s before %s", got[i-1], got[i])
		}
	}
}

// TestTheKubernetesPatchWritesControlPlaneImagesOnlyOnAControlPlane.
//
// Writing them onto a worker is writing configuration for components it does
// not run: Talos accepts it and nobody can read it afterwards.
func TestTheKubernetesPatchWritesControlPlaneImagesOnlyOnAControlPlane(t *testing.T) {
	t.Parallel()

	worker, workerPaths := upgrade.KubernetesPatch(v(t, "v1.34.1"), false)
	if strings.Contains(string(worker), "kube-apiserver") {
		t.Errorf("a worker's patch carries control-plane images:\n%s", worker)
	}
	if !strings.Contains(string(worker), "kubelet:v1.34.1") {
		t.Errorf("a worker's patch does not set the kubelet image:\n%s", worker)
	}
	if len(workerPaths) != 1 {
		t.Errorf("a worker's patch declares %d path(s), want 1: %v", len(workerPaths), workerPaths)
	}

	cp, cpPaths := upgrade.KubernetesPatch(v(t, "v1.34.1"), true)
	for _, want := range []string{"kube-apiserver", "kube-controller-manager", "kube-scheduler"} {
		if !strings.Contains(string(cp), want+":v1.34.1") {
			t.Errorf("a control plane's patch does not set %s:\n%s", want, cp)
		}
	}
	if len(cpPaths) != 4 {
		t.Errorf("a control plane's patch declares %d path(s), want 4: %v", len(cpPaths), cpPaths)
	}
}
