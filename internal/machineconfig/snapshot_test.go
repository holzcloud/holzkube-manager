package machineconfig_test

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/machineconfig"
)

// The edges of the snapshot. The tracer in snapshot_node_test.go covers the
// path through the real client; these cover what a snapshot must never do.

// containsSecret reports whether text holds a secret in the form it is
// generated in or in the base64 form the private keys sit in inside a
// configuration.
func containsSecret(text string, secrets map[string]string) (string, bool) {
	for name, secret := range secrets {
		if strings.Contains(text, secret) ||
			strings.Contains(text, base64.StdEncoding.EncodeToString([]byte(secret))) {
			return name, true
		}
	}
	return "", false
}

func TestNoSecretInTheSnapshot(t *testing.T) {
	t.Parallel()

	cl, raw := fixture(t)
	secrets := secretsOf(t, cl)

	// Precondition: the raw document does hold secrets; otherwise their
	// absence from the snapshot would be a result about nothing.
	var found int
	for _, secret := range secrets {
		if strings.Contains(string(raw), secret) {
			found++
		}
	}
	if found == 0 {
		t.Fatal("the fixture contains none of the secrets, so this test proves nothing")
	}

	snap, err := machineconfig.Snapshot(raw)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if name, leaked := containsSecret(snap.Text, secrets); leaked {
		t.Errorf("the snapshot leaks %s", name)
	}
	assertClean(t, "the snapshot", []byte(snap.Text), secrets)
	if !strings.Contains(snap.Text, machineconfig.RedactedMarker) {
		t.Error("the snapshot carries no redaction marker")
	}
}

func TestAnUnparsableConfigurationYieldsNoSnapshot(t *testing.T) {
	t.Parallel()

	cl, raw := fixture(t)
	const planted = "planted-value-that-must-not-surface-5d2c"
	broken := string(raw) + "\n---\napiVersion: v1alpha1\nkind: SomethingNewUpstream\nplanted: " + planted + "\n"

	// Precondition: Redact itself refuses this one, and hands back bytes
	// alongside the error -- the case that must not become a snapshot.
	partial, rerr := machineconfig.Redact([]byte(broken))
	if rerr == nil {
		t.Fatal("precondition: Redact accepted a document of an unknown kind, so this is not the failing case")
	}
	if len(partial) == 0 {
		t.Fatal("precondition: Redact returned no bytes with its error, so there is nothing to wrongly keep")
	}

	snap, err := machineconfig.Snapshot([]byte(broken))
	if err == nil {
		t.Fatal("a configuration that does not load produced a snapshot")
	}
	if !errors.Is(err, machineconfig.ErrSnapshotUnreadable) {
		t.Errorf("the error is %v, want it to wrap ErrSnapshotUnreadable", err)
	}
	if snap != (machineconfig.Snap{}) {
		t.Errorf("a snapshot came back with the error: %+v", snap)
	}
	if strings.Contains(err.Error(), planted) || strings.Contains(err.Error(), "SomethingNewUpstream") {
		t.Errorf("the error message carries content of the configuration: %q", err.Error())
	}
	if name, leaked := containsSecret(err.Error(), secretsOf(t, cl)); leaked {
		t.Errorf("the error message leaks %s", name)
	}
}

func TestSwappedDocumentsAreTheSameSnapshot(t *testing.T) {
	t.Parallel()

	_, raw := fixture(t)
	docs := strings.Split(string(raw), "\n---\n")
	if len(docs) < 3 {
		t.Fatalf("precondition: the fixture has %d documents, want several", len(docs))
	}
	reversed := make([]string, len(docs))
	for i, d := range docs {
		reversed[len(docs)-1-i] = d
	}
	swapped := strings.Join(reversed, "\n---\n")
	if swapped == string(raw) {
		t.Fatal("precondition: reversing the documents left the bytes unchanged")
	}

	a, err := machineconfig.Snapshot(raw)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	b, err := machineconfig.Snapshot([]byte(swapped))
	if err != nil {
		t.Fatalf("Snapshot of the reversed documents: %v", err)
	}
	if a.Digest != b.Digest {
		t.Error("the same documents in another order are another digest")
	}
	if a.Text != b.Text {
		t.Error("the same documents in another order are another text")
	}
}

func TestCommentsAreNotPartOfTheSnapshot(t *testing.T) {
	t.Parallel()

	_, raw := fixture(t)
	commented := "# written by somebody with talosctl\n" + string(raw)
	if commented == string(raw) {
		t.Fatal("precondition: the comment changed nothing")
	}

	a, err := machineconfig.Snapshot(raw)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	b, err := machineconfig.Snapshot([]byte(commented))
	if err != nil {
		t.Fatalf("Snapshot of the commented configuration: %v", err)
	}
	if a.Digest != b.Digest {
		t.Error("a comment changed the digest")
	}
	if strings.Contains(b.Text, "written by somebody") {
		t.Error("a comment is part of the snapshot text")
	}
}

func TestASnapshotOfASnapshotIsItself(t *testing.T) {
	t.Parallel()

	_, raw := fixture(t)
	a, err := machineconfig.Snapshot(raw)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	b, err := machineconfig.Snapshot([]byte(a.Text))
	if err != nil {
		t.Fatalf("Snapshot of a snapshot: %v", err)
	}
	if a.Text != b.Text || a.Digest != b.Digest {
		t.Error("a snapshot applied to its own text changed it")
	}
}

func TestDocumentsOfTheSameKindAreToldApartByName(t *testing.T) {
	t.Parallel()

	_, raw := fixture(t)
	snap, err := machineconfig.Snapshot(raw)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	const prefix = "kind: KubeAuthorizerConfig\nname: "
	if strings.Count(snap.Text, prefix+"node\n") != 1 || strings.Count(snap.Text, prefix+"rbac\n") != 1 {
		t.Fatal("precondition: the snapshot has no KubeAuthorizerConfig pair named node and rbac")
	}
	swapped := strings.ReplaceAll(snap.Text, prefix+"node\n", prefix+"@@\n")
	swapped = strings.ReplaceAll(swapped, prefix+"rbac\n", prefix+"node\n")
	swapped = strings.ReplaceAll(swapped, prefix+"@@\n", prefix+"rbac\n")
	if swapped == snap.Text {
		t.Fatal("precondition: swapping the names changed nothing")
	}

	d, err := machineconfig.CompareDocuments(snap.Text, swapped)
	if err != nil {
		t.Fatalf("CompareDocuments: %v", err)
	}
	want := []string{"KubeAuthorizerConfig/node.type", "KubeAuthorizerConfig/rbac.type"}
	if strings.Join(d.Paths, ",") != strings.Join(want, ",") {
		t.Errorf("Paths is %v, want %v", d.Paths, want)
	}
}

func TestADocumentAddedOrRemovedIsReportedByItsKey(t *testing.T) {
	t.Parallel()

	_, raw := fixture(t)
	snap, err := machineconfig.Snapshot(raw)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	const doc = "apiVersion: v1alpha1\nkind: HostnameConfig\nauto: stable\n"
	if strings.Count(snap.Text, doc) != 1 {
		t.Fatal("precondition: the HostnameConfig document is not in the snapshot as expected")
	}
	without := strings.Replace(snap.Text, "---\n"+doc, "", 1)
	if without == snap.Text {
		without = strings.Replace(snap.Text, doc+"---\n", "", 1)
	}
	if without == snap.Text {
		t.Fatal("precondition: the document could not be cut out of the text")
	}

	removed, err := machineconfig.CompareDocuments(snap.Text, without)
	if err != nil {
		t.Fatalf("CompareDocuments (removed): %v", err)
	}
	if len(removed.Changes) != 1 || removed.Changes[0].Path != "HostnameConfig" ||
		removed.Changes[0].Kind != machineconfig.ChangeRemoved {
		t.Errorf("removing a document gave %+v, want one removed change at HostnameConfig", removed.Changes)
	}

	added, err := machineconfig.CompareDocuments(without, snap.Text)
	if err != nil {
		t.Fatalf("CompareDocuments (added): %v", err)
	}
	if len(added.Changes) != 1 || added.Changes[0].Path != "HostnameConfig" ||
		added.Changes[0].Kind != machineconfig.ChangeAdded {
		t.Errorf("adding a document gave %+v, want one added change at HostnameConfig", added.Changes)
	}
}

// TestAChangedSecretValueIsInvisibleByDesign holds a blind spot, not a guard.
//
// Redact replaces every secret by the same constant, so a rotated token is the
// same snapshot (D-02). That is wanted: the alternative is a snapshot that
// carries the secret. It is bound to the open ledger entry written in plan 10;
// if this test goes red, the redaction changed and the statement in the guide
// has to be checked again. It does not count as a guard seen red.
func TestAChangedSecretValueIsInvisibleByDesign(t *testing.T) {
	t.Parallel()

	cl, raw := fixture(t)
	token := cl.Secrets.TrustdInfo.Token
	if !strings.Contains(string(raw), token) {
		t.Fatal("precondition: the machine token is not in the raw configuration")
	}
	other := strings.Repeat("x", len(token))
	changed := strings.ReplaceAll(string(raw), token, other)
	if changed == string(raw) {
		t.Fatal("precondition: the raw bytes did not change")
	}

	a, err := machineconfig.Snapshot(raw)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	b, err := machineconfig.Snapshot([]byte(changed))
	if err != nil {
		t.Fatalf("Snapshot of the changed configuration: %v", err)
	}
	if a.Digest != b.Digest {
		t.Error("a changed secret value is visible in the digest; the blind spot is closed, check the guide")
	}
	d, err := machineconfig.CompareDocuments(a.Text, b.Text)
	if err != nil {
		t.Fatalf("CompareDocuments: %v", err)
	}
	if len(d.Changes) != 0 {
		t.Errorf("a changed secret value is a change: %+v", d.Changes)
	}
}

// TestTheSnapshotHoldsNoMoreAndNoLessThanTheConfigView ties the snapshot to
// what the configuration view already shows (D-14). The second half -- values
// Redact does not know stand in the text -- is a blind spot held as a test,
// bound to the open ledger entry of plan 10, and is not a guard: if it goes
// red, the redaction changed and the guide's statement needs another look.
func TestTheSnapshotHoldsNoMoreAndNoLessThanTheConfigView(t *testing.T) {
	t.Parallel()

	_, raw := fixture(t)
	patched, err := machineconfig.ApplyPatches(raw, []string{
		"machine:\n  env:\n    HOLZ_TEST: planted-env-value\n" +
			"  files:\n    - content: planted-file-content\n      path: /var/etc/planted.conf\n" +
			"      permissions: 0o644\n      op: create\n",
	})
	if err != nil {
		t.Fatalf("ApplyPatches: %v", err)
	}

	view, err := machineconfig.Redact(patched)
	if err != nil {
		t.Fatalf("Redact: %v", err)
	}
	snap, err := machineconfig.Snapshot(patched)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	// No more and no less than the view: the same documents, compared
	// structurally.
	d, err := machineconfig.CompareDocuments(string(view), snap.Text)
	if err != nil {
		t.Fatalf("CompareDocuments: %v", err)
	}
	if len(d.Changes) != 0 {
		t.Errorf("the snapshot differs from the configuration view: %+v", d.Changes)
	}

	// The fields Redact does not know are in the text, as in the view.
	for _, want := range []string{"planted-env-value", "planted-file-content"} {
		if !strings.Contains(snap.Text, want) {
			t.Errorf("%q is not in the snapshot; the unredacted fields are no longer as the view shows them", want)
		}
	}
}

func TestASnapshotOverTheCapIsRefused(t *testing.T) {
	t.Parallel()

	_, err := machineconfig.Snapshot([]byte(strings.Repeat("a", machineconfig.MaxSnapshotBytes+1)))
	if !errors.Is(err, machineconfig.ErrSnapshotTooLarge) {
		t.Errorf("the error is %v, want ErrSnapshotTooLarge", err)
	}
}

func TestCompareDocumentsNeverCarriesASecret(t *testing.T) {
	t.Parallel()

	_, raw := fixture(t)
	snap, err := machineconfig.Snapshot(raw)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	// A stored text altered by hand: a planted value where the marker of
	// machine.token stands.
	const planted = "planted-secret-9c41e7"
	re := regexp.MustCompile(`(?m)^(\s+token: )` + regexp.QuoteMeta(machineconfig.RedactedMarker) + `$`)
	tampered := re.ReplaceAllString(snap.Text, "${1}"+planted)
	if !strings.Contains(tampered, planted) || tampered == snap.Text {
		t.Fatal("precondition: the planted value did not get into the text")
	}

	d, err := machineconfig.CompareDocuments(snap.Text, tampered)
	if err != nil {
		t.Fatalf("CompareDocuments: %v", err)
	}
	for _, c := range d.Changes {
		if strings.Contains(c.Before, planted) || strings.Contains(c.After, planted) {
			t.Errorf("the comparison carries the planted secret at %s", c.Path)
		}
	}
	d, err = machineconfig.CompareDocuments(tampered, snap.Text)
	if err != nil {
		t.Fatalf("CompareDocuments (reversed): %v", err)
	}
	for _, c := range d.Changes {
		if strings.Contains(c.Before, planted) || strings.Contains(c.After, planted) {
			t.Errorf("the reversed comparison carries the planted secret at %s", c.Path)
		}
	}
}

func TestAnEmptyComparisonSerialisesEmptyListsAndNotNull(t *testing.T) {
	t.Parallel()

	_, raw := fixture(t)
	snap, err := machineconfig.Snapshot(raw)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	d, err := machineconfig.CompareDocuments(snap.Text, snap.Text)
	if err != nil {
		t.Fatalf("CompareDocuments: %v", err)
	}
	out, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `"changes":[]`) || !strings.Contains(s, `"paths":[]`) {
		t.Errorf("an empty comparison is %s, want empty lists", s)
	}
	if strings.Contains(s, "null") {
		t.Errorf("an empty comparison carries null: %s", s)
	}
}
