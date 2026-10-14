package machineconfig_test

import (
	"context"
	"encoding/base64"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/machineconfig"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// TestAChangeInAnotherDocumentIsSeenThroughTheRealClient is the tracer of the
// drift detection: a real node (the simulator) is read through the real
// Talos client from a context WITHOUT a deadline -- which is what main.go
// hands every background loop, and what certwatch failed on (ledger 137) --
// a document that is not the first one changes, and CompareDocuments sees it.
func TestAChangeInAnotherDocumentIsSeenThroughTheRealClient(t *testing.T) {
	t.Parallel()

	cl, err := talossim.NewCluster("homelab", "https://192.168.1.10:6443")
	if err != nil {
		t.Fatalf("NewCluster: %v", err)
	}
	sim, err := talossim.New(talossim.Options{Hostname: "cp-1", Cluster: cl, ControlPlane: true})
	if err != nil {
		t.Fatalf("talossim.New: %v", err)
	}
	t.Cleanup(func() { _ = sim.Close() })

	connect := func(ctx context.Context, id model.MachineID) (*talos.ClusterClient, error) {
		return talos.NewClusterClient(ctx, sim.Dialer(), talos.Target{
			Cluster: "c1", Machine: id, Addr: sim.Host(),
		}, sim.ClientCreds(), talos.Mode{})
	}
	svc := machineconfig.New(machineconfig.Deps{Connect: connect})

	// The context has no deadline: that is the situation in main.go and the
	// point of this test.
	bg := context.Background()
	if _, ok := bg.Deadline(); ok {
		t.Fatal("precondition: the reading context must carry no deadline")
	}

	before, err := svc.ReadSnapshot(bg, "m1")
	if err != nil {
		t.Fatalf("ReadSnapshot from a context without a deadline: %v", err)
	}
	if !sha256Hex.MatchString(before.Digest) {
		t.Errorf("the digest is %q, want 64 hex characters", before.Digest)
	}
	if !strings.Contains(before.Text, machineconfig.RedactedMarker) {
		t.Error("the snapshot carries no redaction marker; it was not made from Redact")
	}

	// Precondition, as in TestNoExitLeaksASecret: the raw configuration does
	// contain the secrets, so their absence from the snapshot says something.
	secrets := secretsOf(t, cl)
	var found int
	for name, secret := range secrets {
		if strings.Contains(string(cl.ControlPlaneConfig), secret) {
			found++
		}
		// The private keys sit in the configuration base64 encoded; look for
		// that form as well. The precondition asks nothing of it.
		encoded := base64.StdEncoding.EncodeToString([]byte(secret))
		if strings.Contains(before.Text, secret) || strings.Contains(before.Text, encoded) {
			t.Errorf("the snapshot leaks %s", name)
		}
	}
	if found == 0 {
		t.Fatal("the raw configuration holds none of the secrets, so the check above proves nothing")
	}

	// The change happens in a document that is not the first, by the way a
	// third party would make it: a second client on the same node.
	const newImage = "ghcr.io/siderolabs/kubelet:v1.34.2"
	patched, err := machineconfig.ApplyPatches(cl.ControlPlaneConfig, []string{
		"apiVersion: v1alpha1\nkind: KubeletConfig\nimage: " + newImage + "\n",
	})
	if err != nil {
		t.Fatalf("ApplyPatches: %v", err)
	}
	applyCtx, cancel := context.WithTimeout(bg, 30*time.Second)
	defer cancel()
	other, err := connect(applyCtx, "m1")
	if err != nil {
		t.Fatalf("second client: %v", err)
	}
	defer func() { _ = other.Close() }()
	if _, err := other.ApplyConfigurationWithMode(applyCtx, patched, "no-reboot"); err != nil {
		t.Fatalf("ApplyConfigurationWithMode: %v", err)
	}

	after, err := svc.ReadSnapshot(bg, "m1")
	if err != nil {
		t.Fatalf("second ReadSnapshot: %v", err)
	}
	if after.Digest == before.Digest {
		t.Fatal("the digest did not change after a change in the KubeletConfig document")
	}

	d, err := machineconfig.CompareDocuments(before.Text, after.Text)
	if err != nil {
		t.Fatalf("CompareDocuments: %v", err)
	}
	if len(d.Changes) != 1 {
		t.Fatalf("CompareDocuments found %d changes, want exactly 1: %+v", len(d.Changes), d.Changes)
	}
	c := d.Changes[0]
	if c.Path != "KubeletConfig.image" || c.Kind != machineconfig.ChangeModified {
		t.Errorf("the change is %q (%s), want KubeletConfig.image (modified)", c.Path, c.Kind)
	}
	if !strings.Contains(c.After, newImage) {
		t.Errorf("After is %q, want it to name %q", c.After, newImage)
	}
	if c.Before == "" || strings.Contains(c.Before, newImage) {
		t.Errorf("Before is %q, want the old image", c.Before)
	}
	if len(d.Paths) != 1 || d.Paths[0] != "KubeletConfig.image" {
		t.Errorf("Paths is %v, want [KubeletConfig.image]", d.Paths)
	}

	// The witness of why CompareDocuments exists, and expressly NOT a guard of
	// this plan: Compare reads only the first YAML document and sees nothing.
	// This line holds the open ledger entry on Compare's blind spot (D-09). If
	// it ever goes red, somebody repaired Compare: close the entry and delete
	// this line.
	old, err := machineconfig.Compare(cl.ControlPlaneConfig, patched)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if len(old.Changes) != 0 {
		t.Errorf("Compare now sees %d change(s) in a later document; its blind spot is repaired -- "+
			"close the ledger entry and delete this check", len(old.Changes))
	}
}
