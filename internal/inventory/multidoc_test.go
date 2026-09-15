package inventory

import (
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// A real cluster refused adoption, and it refused over a document adoption does
// not read.
//
//	POST /api/v1/clusters -> 500
//	error decoding document v1alpha1/DiscoveryServiceConfig/default (line 94):
//	  "DiscoveryServiceConfig" "v1alpha1": not registered
//
// A Talos machine configuration is multi-document YAML: one v1alpha1 Config
// carrying `.machine` and `.cluster`, and beside it a growing set of typed
// documents. machinery decodes a typed document only if THIS build's machinery
// has its kind registered, its loader is all-or-nothing, and it offers no
// option to tolerate one. DiscoveryServiceConfig does not exist in machinery
// v1.13.9 at all — it belongs to a Talos newer than the pin.
//
// Every secret adoption needs is in the v1alpha1 document. Not one of the
// siblings is read. So the whole adoption failed on a document nobody looks at,
// and would have failed again on the next kind Talos ships.
//
// talossim could never have found this: it serves a configuration this same
// machinery generated, so by construction every document in it is one this
// build knows. The fixture below is the simulator's own control-plane config
// with the operator's document appended — real secrets, real refusal.
func TestAConfigCarryingAnUnknownDocumentStillYieldsItsSecrets(t *testing.T) {
	t.Parallel()

	cluster, err := talossim.NewCluster("holzkube", "https://192.168.1.110:6443")
	if err != nil {
		t.Fatalf("build a simulated cluster: %v", err)
	}
	base := string(cluster.Config(true))

	// Shaped as the operator's node serves it: apiVersion + kind, which is what
	// every typed document carries and what sends machinery to its registry.
	const unknown = `---
apiVersion: v1alpha1
kind: DiscoveryServiceConfig
name: default
enabled: true
`

	// Asserted rather than assumed: if machinery ever learns this kind, this
	// test stops reproducing anything and should say so instead of passing.
	if _, err := deriveSecretsWithoutSelection(base + "\n" + unknown); err == nil {
		t.Fatal("machinery now decodes DiscoveryServiceConfig; this test no longer " +
			"reproduces the failure it was written for")
	} else if !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("the fixture fails for some other reason than the one under test: %v", err)
	}

	bundle, err := deriveSecrets([]byte(base + "\n" + unknown))
	if err != nil {
		t.Fatalf("a configuration with one unknown document yielded no secrets: %v", err)
	}
	if bundle == nil || bundle.Certs == nil || bundle.Certs.OS == nil || len(bundle.Certs.OS.Key) == 0 {
		t.Fatal("the bundle carries no Talos CA private key, so adoption would refuse the node")
	}
	if bundle.Certs.K8s == nil || len(bundle.Certs.K8s.Key) == 0 {
		t.Fatal("the bundle carries no Kubernetes CA private key")
	}
}

// TestTheDocumentSelectionIsNotOrderDependent.
//
// The v1alpha1 document is first in what talosctl generates and there is
// nothing that guarantees it. A node that carries a typed document ahead of it
// must adopt exactly as well.
func TestTheDocumentSelectionIsNotOrderDependent(t *testing.T) {
	t.Parallel()

	cluster, err := talossim.NewCluster("holzkube", "https://192.168.1.110:6443")
	if err != nil {
		t.Fatalf("build a simulated cluster: %v", err)
	}

	const first = `apiVersion: v1alpha1
kind: DiscoveryServiceConfig
name: default
enabled: true
---
`
	if _, err := deriveSecrets([]byte(first + string(cluster.Config(true)))); err != nil {
		t.Fatalf("an unknown document BEFORE the machine config broke adoption: %v", err)
	}
}

// TestAConfigWithNoMachineDocumentIsTheWrongNode.
//
// The refusal has to stay the one D-05 owns — "that node cannot be adopted
// through", a 400 naming the remedy — rather than becoming a parse failure and
// therefore an Internal error, which is what the operator saw and could do
// nothing with.
func TestAConfigWithNoMachineDocumentIsTheWrongNode(t *testing.T) {
	t.Parallel()

	const onlyTyped = `apiVersion: v1alpha1
kind: DiscoveryServiceConfig
name: default
enabled: true
`
	_, err := deriveSecrets([]byte(onlyTyped))
	if err == nil {
		t.Fatal("a configuration with no machine document was accepted")
	}
	if !strings.Contains(err.Error(), "no v1alpha1 machine configuration") {
		t.Errorf("the refusal does not name what is missing: %v", err)
	}
	// The sentinel is what the HTTP layer maps onto a 400 rather than a 500.
	if !isNotControlPlane(err) {
		t.Errorf("the refusal is not ErrNotControlPlane, so it would reach the "+
			"operator as Internal error: %v", err)
	}
}
