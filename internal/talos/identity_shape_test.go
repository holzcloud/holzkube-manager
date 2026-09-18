package talos_test

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// TestAProbesIdentityCarriesOnlyWhatAHandshakeShows is ledger 1, closed by
// removal rather than by implementation.
//
// Identity is what Dialer.Probe returns, and a probe is a TLS handshake and
// nothing more: it has to work against a node in maintenance mode, which has no
// cluster PKI to authenticate to, so it cannot make an RPC. A certificate
// carries a subject, its DNS names and its issuer -- a hostname, and whether
// the node signed its own certificate. It does not carry a version.
//
// The field was there anyway. The direct dialer left it empty, which ledger 1
// recorded, and talossim filled it from the node it models -- so a test could
// have asserted a version that production never returns. That is the hazard
// TRANS-06 names: the simulator passing something the real client could not.
//
// Where an unconfigured machine's version is genuinely wanted, the provisioning
// path asks the node with a Version RPC over a maintenance client. This test
// exists so that a future probe does not quietly grow a field it cannot fill.
func TestAProbesIdentityCarriesOnlyWhatAHandshakeShows(t *testing.T) {
	t.Parallel()

	shown := map[string]bool{
		"Machine":     true, // the caller's own target, echoed back
		"Hostname":    true, // the certificate's subject or first DNS name
		"Maintenance": true, // the certificate is self-signed
	}

	typ := reflect.TypeOf(talos.Identity{})
	for i := range typ.NumField() {
		name := typ.Field(i).Name
		if !shown[name] {
			t.Errorf("Identity carries %q, which a TLS handshake cannot show. A probe makes no RPC "+
				"-- it has to work against a node with no cluster PKI -- so this field will be "+
				"empty in production and filled by the simulator, which is a test asserting what "+
				"hardware never does (ledger 1, TRANS-06). Ask the node for it where it is needed "+
				"instead, as internal/provision does with a Version RPC.", name)
		}
	}

	// And the removal has to stay visible in the type's own documentation,
	// because the next person who wants a version here will read the struct and
	// not this test. The source is read rather than the doc being restated,
	// for the reason internal/inventory's adoption guard reads its source: a
	// copy of the sentence here would be a second thing to keep true.
	src, err := os.ReadFile("talos.go")
	if err != nil {
		t.Fatalf("read talos.go: %v", err)
	}
	if !strings.Contains(string(src), "no Version here") {
		t.Error("the Identity type no longer says why it has no version, so the field comes back " +
			"the next time somebody wants one")
	}
}
