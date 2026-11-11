package support_test

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/support"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// TestAConfigurationThatDoesNotParseIsNotInTheBundle is the bundle taken from
// a cluster one Talos release ahead of this build.
//
// The configuration then carries a document kind this build cannot decode,
// and the bundle used to write it out with nothing removed but literal PEM
// blocks, of which a Talos configuration has none. The file is left out, and
// the reason given for it carries neither the configuration nor the parser's
// message about it.
func TestAConfigurationThatDoesNotParseIsNotInTheBundle(t *testing.T) {
	t.Parallel()

	cl, err := talossim.NewCluster("homelab", "https://10.0.0.10:6443")
	if err != nil {
		t.Fatalf("NewCluster: %v", err)
	}
	const planted = "planted-value-that-must-not-surface-7c1e"
	raw := string(cl.ControlPlaneConfig) +
		"\n---\napiVersion: v1alpha1\nkind: SomethingThisBuildHasNeverHeardOf\nplanted: " + planted + "\n"

	out, reason := support.ConfigForBundle([]byte(raw))
	if len(out) != 0 {
		t.Errorf("an unparsable configuration goes into the bundle as %d bytes, want none", len(out))
	}
	if reason == "" {
		t.Fatal("the configuration is left out and nothing says why")
	}

	key := strings.TrimSpace(string(cl.OSKeyPEM()))
	for _, secret := range []string{key, base64.StdEncoding.EncodeToString(cl.OSKeyPEM()), planted, "SomethingThisBuildHasNeverHeardOf"} {
		if strings.Contains(string(out), secret) || strings.Contains(reason, secret) {
			t.Errorf("content of the configuration is in what leaves the installation: %.30q…", secret)
		}
	}

	// And a configuration that does parse still goes in, redacted.
	out, reason = support.ConfigForBundle(cl.ControlPlaneConfig)
	if reason != "" || len(out) < 200 {
		t.Errorf("a configuration that parses was left out (%q) or came back as %d bytes", reason, len(out))
	}
}
