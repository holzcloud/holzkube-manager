package upgrade_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/compat"
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
// kube-apiserver may not skip a minor when it is upgraded, so every minor
// between the start and the target is passed through, and each one on the
// newest stable patch it has, not on the first release of the minor.
func TestTheKubernetesChainPassesThroughEveryMinor(t *testing.T) {
	t.Parallel()

	steps, err := upgrade.KubernetesChain(v(t, "v1.32.4"), v(t, "v1.34.1"), "v1.14.0",
		map[int]upgrade.Version{33: v(t, "v1.33.7")})
	if err != nil {
		t.Fatalf("KubernetesChain: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("the chain from 1.32 to 1.34 has %d step(s), want 2: %+v", len(steps), steps)
	}
	if steps[0].To.String() != "v1.33.7" || steps[1].To.String() != "v1.34.1" {
		t.Errorf("the chain is %s then %s, want v1.33.7 then v1.34.1", steps[0].To, steps[1].To)
	}
	if !strings.Contains(steps[0].Why, "on the way") {
		t.Errorf("the intermediate step does not say it is on the way: %q", steps[0].Why)
	}
	if strings.Contains(steps[0].Why, "no more") {
		t.Errorf("the intermediate step still gives the skew rule that is wrong: %q", steps[0].Why)
	}
}

// TestAKubernetesChainBeyondWhatTalosSupportsIsRefused is the mirror of
// UPG-06: the same gate, from the other side.
func TestAKubernetesChainBeyondWhatTalosSupportsIsRefused(t *testing.T) {
	t.Parallel()

	// A target far above every window, so the property does not depend on how
	// high the compatibility matrix currently reaches for Talos 1.12.
	_, err := upgrade.KubernetesChain(v(t, "v1.32.4"), v(t, "v1.99.0"), "v1.12.3", nil)
	if !errors.Is(err, upgrade.ErrWouldStrand) {
		t.Fatalf("a Kubernetes upgrade past what the running Talos supports returned %v", err)
	}
}

// TestAKubernetesChainWithAMissingMinorIsRefusedRatherThanFilled.
//
// The function never puts a version in by itself: a minor whose newest patch
// is not known is a refusal that names the minor, not a guess at its first
// release.
func TestAKubernetesChainWithAMissingMinorIsRefusedRatherThanFilled(t *testing.T) {
	t.Parallel()

	steps, err := upgrade.KubernetesChain(v(t, "v1.32.4"), v(t, "v1.35.1"), "v1.14.0",
		map[int]upgrade.Version{33: v(t, "v1.33.7")})
	if !errors.Is(err, upgrade.ErrNoPath) {
		t.Fatalf("a chain with a missing minor returned %v, want ErrNoPath", err)
	}
	if !strings.Contains(err.Error(), "1.34") {
		t.Errorf("the refusal does not name the minor that is missing: %v", err)
	}
	if len(steps) != 0 {
		t.Errorf("a refused chain still carries steps: %+v", steps)
	}
}

// TestAKubernetesChainWithoutIntermediatesNeedsNoReleases.
//
// A patch jump and a jump to the next minor have nothing on the way, and
// neither may need the set of newest patches.
func TestAKubernetesChainWithoutIntermediatesNeedsNoReleases(t *testing.T) {
	t.Parallel()

	steps, err := upgrade.KubernetesChain(v(t, "v1.34.1"), v(t, "v1.34.9"), "v1.14.0", nil)
	if err != nil || len(steps) != 0 {
		t.Errorf("a patch jump gave %+v, %v; want an empty chain", steps, err)
	}

	steps, err = upgrade.KubernetesChain(v(t, "v1.33.4"), v(t, "v1.34.1"), "v1.14.0", nil)
	if err != nil {
		t.Fatalf("the next minor: %v", err)
	}
	if len(steps) != 1 || steps[0].To.String() != "v1.34.1" {
		t.Errorf("the next minor gave %+v, want the target alone", steps)
	}

	steps, err = upgrade.KubernetesChain(v(t, "v1.34.1"), v(t, "v1.34.1"), "v1.14.0", nil)
	if err != nil || steps != nil {
		t.Errorf("the same version gave %+v, %v; want nil", steps, err)
	}
}

// TestAKubernetesChainReachesTheNewestKubernetesTalos114Supports.
//
// The top of the window is computed from compat, not written down here: the
// property is that the chain reaches whatever the newest supported minor is,
// at any height of the matrix.
func TestAKubernetesChainReachesTheNewestKubernetesTalos114Supports(t *testing.T) {
	t.Parallel()

	verdict := compat.Check("v1.14.2", "v1.34.0")
	if !verdict.Known {
		t.Fatal("Talos 1.14 has no row in the compatibility matrix; the matrix quick task " +
			"(D-03) has to have run before this phase's tests mean anything")
	}
	top := verdict.Window.MaxMinor

	from := v(t, fmt.Sprintf("v1.%d.3", top-3))
	to := v(t, fmt.Sprintf("v1.%d.1", top))
	steps, err := upgrade.KubernetesChain(from, to, "v1.14.2", map[int]upgrade.Version{
		top - 2: v(t, fmt.Sprintf("v1.%d.9", top-2)),
		top - 1: v(t, fmt.Sprintf("v1.%d.5", top-1)),
	})
	if err != nil {
		t.Fatalf("a chain up to the top of Talos 1.14's window (1.%d) was refused: %v", top, err)
	}
	want := []string{
		fmt.Sprintf("v1.%d.9", top-2), fmt.Sprintf("v1.%d.5", top-1), fmt.Sprintf("v1.%d.1", top),
	}
	if len(steps) != len(want) {
		t.Fatalf("the chain has %d step(s), want %d: %+v", len(steps), len(want), steps)
	}
	for i, w := range want {
		if steps[i].To.String() != w {
			t.Errorf("step %d is %s, want %s", i, steps[i].To, w)
		}
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

// TestEachKubernetesPatchTouchesOneThing.
//
// The run writes one change at a time so that each can be waited for: the
// kubelet patch carries no control-plane image (a worker gets only it, and
// writing images for components a node does not run is configuration nobody
// can read afterwards), and each component patch carries only its component.
func TestEachKubernetesPatchTouchesOneThing(t *testing.T) {
	t.Parallel()

	kubelet, kubeletPaths := upgrade.KubeletPatch(v(t, "v1.34.1"))
	if strings.Contains(kubelet, "kube-apiserver") || !strings.Contains(kubelet, "kubelet:v1.34.1") {
		t.Errorf("the kubelet patch is not only the kubelet:\n%s", kubelet)
	}
	if len(kubeletPaths) != 1 {
		t.Errorf("the kubelet patch declares %d path(s), want 1: %v", len(kubeletPaths), kubeletPaths)
	}

	for _, c := range []string{"kube-apiserver", "kube-controller-manager", "kube-scheduler"} {
		patch, paths := upgrade.ComponentPatch(c, v(t, "v1.34.1"))
		if !strings.Contains(patch, c+":v1.34.1") {
			t.Errorf("the %s patch does not set its image:\n%s", c, patch)
		}
		for _, other := range []string{"kube-apiserver", "kube-controller-manager", "kube-scheduler", "kubelet"} {
			if other != c && strings.Contains(patch, other+":") {
				t.Errorf("the %s patch also sets %s:\n%s", c, other, patch)
			}
		}
		if len(paths) != 1 {
			t.Errorf("the %s patch declares %d path(s), want 1: %v", c, len(paths), paths)
		}
	}

	proxy, _ := upgrade.ProxyPatch(v(t, "v1.34.1"))
	if !strings.Contains(proxy, "kube-proxy:v1.34.1") {
		t.Errorf("the proxy patch does not set kube-proxy:\n%s", proxy)
	}
}
