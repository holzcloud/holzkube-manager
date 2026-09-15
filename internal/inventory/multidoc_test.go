package inventory

import (
	"errors"
	"strings"
	"testing"

	"github.com/siderolabs/talos/pkg/machinery/config/configloader"

	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// A Talos machine configuration is multi-document YAML, and since v1.14 that is
// where most of it lives: the Kubernetes API-server CA, the cluster identity,
// the volume layout and a dozen others each have their own kind beside the
// v1alpha1 document.
//
// machinery decodes a typed document only if THIS build's machinery has the
// kind registered, and its loader is all-or-nothing. So a node running a Talos
// newer than the pin serves a configuration this build cannot read — which is
// how a real cluster came to be told "Internal error" by a build pinned to
// machinery v1.13.9, over a DiscoveryServiceConfig document that version does
// not have.
//
// The fix that shipped first selected the v1alpha1 document and ignored the
// rest, reasoning that every secret adoption needs lives in it. That was true
// of v1.13 and is false of v1.14. These tests pin the behaviour that replaced
// it, and the second one pins why.

func TestAConfigWithEveryDocumentKnownYieldsItsSecrets(t *testing.T) {
	t.Parallel()

	cluster, err := talossim.NewCluster("holzkube", "https://192.168.0.110:6443")
	if err != nil {
		t.Fatalf("build a simulated cluster: %v", err)
	}

	bundle, err := deriveSecrets(cluster.Config(true))
	if err != nil {
		t.Fatalf("a control-plane configuration yielded no secrets: %v", err)
	}
	if bundle.Certs.OS == nil || len(bundle.Certs.OS.Key) == 0 {
		t.Error("no Talos CA private key")
	}
	// The one that moved out of v1alpha1 in v1.14, and therefore the one a
	// document-selecting parse would silently have dropped.
	if bundle.Certs.K8s == nil || len(bundle.Certs.K8s.Key) == 0 {
		t.Error("no Kubernetes CA private key")
	}
}

// TestSelectingOnlyV1Alpha1WouldLoseTheKubernetesCA is the argument for not
// doing the obvious thing, kept as a measurement rather than as a comment.
//
// Dropping the sibling documents looks safe and reads as robustness. Under
// machinery v1.14 it produces a bundle with the Kubernetes CA missing, which
// `controlPlaneMaterialPresent` then reports as "this is a worker" — a
// confident, wrong answer about a control-plane node, which is worse than the
// Internal error it would have replaced.
//
// If a future machinery moves the material back into v1alpha1, this test fails
// and the reasoning above has to be re-read rather than assumed.
func TestSelectingOnlyV1Alpha1WouldLoseTheKubernetesCA(t *testing.T) {
	t.Parallel()

	cluster, err := talossim.NewCluster("holzkube", "https://192.168.0.110:6443")
	if err != nil {
		t.Fatalf("build a simulated cluster: %v", err)
	}

	full := string(cluster.Config(true))
	v1alpha1, _, found := strings.Cut(full, "\n---")
	if !found {
		t.Fatal("the generated configuration is a single document; this test's premise is gone")
	}

	// Probed on the provider rather than through deriveSecrets, because what
	// the v1alpha1 document alone produces is not a poor bundle: machinery
	// v1.14 PANICS on it. Asserting the absent accessor states the same fact
	// without depending on a crash staying a crash.
	only, err := configloader.NewFromBytes([]byte(v1alpha1))
	if err != nil {
		t.Fatalf("the v1alpha1 document alone does not even load: %v", err)
	}
	if only.RawV1Alpha1() == nil {
		t.Fatal("the cut did not produce the v1alpha1 document; this test is measuring nothing")
	}
	if only.K8sAPIServerCAConfig() != nil {
		t.Fatal("the v1alpha1 document alone still carries the Kubernetes API-server CA, so " +
			"selecting it would be safe after all and the whole-file parse is over-cautious")
	}

	// And the whole file does carry it, so the difference is the documents and
	// not something about this fixture.
	whole, err := configloader.NewFromBytes(cluster.Config(true))
	if err != nil {
		t.Fatalf("the whole configuration does not load: %v", err)
	}
	if whole.K8sAPIServerCAConfig() == nil {
		t.Fatal("the whole configuration carries no Kubernetes API-server CA either")
	}
}

// TestAnUnknownDocumentIsNamedAsTooNew.
//
// The operator's answer has to be actionable, and the action is "upgrade
// holzkube-manager" — not "Internal error", and not a bundle derived from the
// parts this build happened to recognise.
func TestAnUnknownDocumentIsNamedAsTooNew(t *testing.T) {
	t.Parallel()

	cluster, err := talossim.NewCluster("holzkube", "https://192.168.0.110:6443")
	if err != nil {
		t.Fatalf("build a simulated cluster: %v", err)
	}

	// A kind no machinery has or will have, so this keeps reproducing the case
	// after the pin is raised again. Naming a real future kind would make the
	// test expire the moment that kind shipped.
	const fromTheFuture = `---
apiVersion: v1alpha1
kind: ConfigFromAVersionThatDoesNotExistYet
name: default
`
	_, err = deriveSecrets([]byte(string(cluster.Config(true)) + fromTheFuture))
	if err == nil {
		t.Fatal("a document this build cannot read was accepted")
	}
	if !errors.Is(err, talos.ErrUnsupportedVersion) {
		t.Fatalf("an unreadable document is not reported as an unsupported Talos: %v", err)
	}
	if !strings.Contains(err.Error(), "ConfigFromAVersionThatDoesNotExistYet") {
		t.Errorf("the refusal does not name the document that could not be read: %v", err)
	}
	if !strings.Contains(err.Error(), talos.MaxSupportedVersion) {
		t.Errorf("the refusal does not say which versions this build was made for: %v", err)
	}
}

// TestAWorkerIsRefusedWithoutDerivingAnything.
//
// machinery v1.14 made this load-bearing rather than tidy. Deriving a bundle
// from a worker's configuration PANICS there — NewBundleFromConfig reads
// c.K8sAPIServerCAConfig().IssuingCA() and a worker has no API-server CA — so
// asking the configuration what it is has to happen first. On a running
// instance the difference is a crash where a refusal belongs.
func TestAWorkerIsRefusedWithoutDerivingAnything(t *testing.T) {
	t.Parallel()

	cluster, err := talossim.NewCluster("holzkube", "https://192.168.0.110:6443")
	if err != nil {
		t.Fatalf("build a simulated cluster: %v", err)
	}

	_, err = deriveSecrets(cluster.Config(false))
	if !errors.Is(err, ErrNotControlPlane) {
		t.Fatalf("a worker was not refused as one: %v", err)
	}
	if !strings.Contains(err.Error(), "worker") {
		t.Errorf("the refusal does not tell the operator to name a control-plane node: %v", err)
	}
}
