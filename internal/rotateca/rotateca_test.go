package rotateca_test

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/machineconfig"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/rotateca"
)

// configWith builds a node configuration carrying one issuing authority and
// any number of accepted ones, the way Talos writes it: base64 in the YAML.
func configWith(t *testing.T, issuing []byte, accepted ...[]byte) []byte {
	t.Helper()

	out := "version: v1alpha1\nmachine:\n  type: controlplane\n  ca:\n    crt: " +
		base64.StdEncoding.EncodeToString(issuing) + "\n"
	if len(accepted) > 0 {
		out += "  acceptedCAs:\n"
		for _, a := range accepted {
			out += "    - crt: " + base64.StdEncoding.EncodeToString(a) + "\n"
		}
	}
	out += "cluster:\n  id: test\n"
	return []byte(out)
}

func authority(t *testing.T) rotateca.Authority {
	t.Helper()

	a, err := rotateca.NewAuthority(time.Now())
	if err != nil {
		t.Fatalf("NewAuthority: %v", err)
	}
	if len(a.Crt) == 0 || len(a.Key) == 0 {
		t.Fatal("the generated authority has no certificate or no key")
	}
	return a
}

// TestPassOneKeepsTheOldAuthorityInTheAcceptedSet is the test this package
// exists for, and the failure it pins is the one that makes a cluster
// unreachable rather than merely wrong.
//
// A strategic merge patch REPLACES a list. A pass-1 patch carrying only the new
// authority therefore performs pass 1 and pass 4 in one write, while every node
// is still issuing from the old authority and this installation still holds a
// certificate from it -- so the next call, to the next node, is refused, with
// the rotation half done and nothing able to finish it.
//
// The assertion goes through machineconfig.ApplyPatches, the same patcher the
// product applies with, and reads the result back with Inspect. A test that
// only searched the patch text would pass on a patch Talos merges differently.
func TestPassOneKeepsTheOldAuthorityInTheAcceptedSet(t *testing.T) {
	t.Parallel()

	old := authority(t)
	extra := authority(t)
	next := authority(t)

	base := configWith(t, old.Crt, extra.Crt)
	before, err := rotateca.Inspect(base)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	patch, changes := rotateca.AcceptPatch(before, next.Crt)
	if !changes {
		t.Fatal("pass 1 reports nothing to do on a node that does not accept the new authority")
	}
	if err := machineconfig.ValidatePatch(patch); err != nil {
		t.Fatalf("the pass-1 patch is not a usable strategic merge patch: %v", err)
	}

	after, err := machineconfig.ApplyPatches(base, []string{patch})
	if err != nil {
		t.Fatalf("ApplyPatches: %v", err)
	}
	state, err := rotateca.Inspect(after)
	if err != nil {
		t.Fatalf("Inspect after pass 1: %v", err)
	}

	if !state.Accepts(next.Crt) {
		t.Error("after pass 1 the node does not accept the new authority, which is the one thing the pass is for")
	}
	if !state.Accepts(old.Crt) {
		t.Error("after pass 1 the node no longer accepts the OLD authority: this write did pass 1 and " +
			"pass 4 at once, and the next node in the run is now unreachable with the rotation half done")
	}
	if !state.Accepts(extra.Crt) {
		t.Error("an authority that was accepted before the rotation was dropped by pass 1; " +
			"a cluster mid-rotation from somewhere else would be cut in half by this")
	}
	if !state.IssuesFrom(old.Crt) {
		t.Error("pass 1 changed which authority the node issues from; that is pass 2")
	}

	// A merge appends, so a pass-1 patch that listed the whole desired set
	// would leave the authorities that were already there in the list twice.
	if got := len(state.AcceptedCrts); got != 3 {
		t.Errorf("acceptedCAs holds %d entries after pass 1, want 3 (the one it had, the issuer, "+
			"the new one) -- a merge patch appends, so a repeated entry means the patch listed "+
			"authorities the node already had", got)
	}
}

// TestPassOneIsIdempotentAcrossAReEncoding is about a resumed run.
//
// The configuration comes back from the node re-encoded rather than byte for
// byte, so a comparison on bytes reports "not accepted yet" for ever and pass 1
// writes on every resume. The comparison is on the certificate.
func TestPassOneIsIdempotentAcrossAReEncoding(t *testing.T) {
	t.Parallel()

	old := authority(t)
	next := authority(t)

	// The same certificate, with the line endings a round trip through another
	// encoder can leave behind.
	reEncoded := []byte(strings.ReplaceAll(string(next.Crt), "\n", "\r\n"))

	// A node already through pass 1: both authorities listed, the new one
	// re-encoded on the way back.
	state, err := rotateca.Inspect(configWith(t, old.Crt, old.Crt, reEncoded))
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if _, changes := rotateca.AcceptPatch(state, next.Crt); changes {
		t.Error("pass 1 would write again on a node that already accepts the new authority, " +
			"because the comparison is on bytes rather than on the certificate")
	}
}

// TestPassTwoGivesTheKeyOnlyToAControlPlane pins Talos's own split: trustd
// issues certificates to joining nodes and runs on the control plane, so the
// authority's key belongs there and a worker carries the certificate alone.
func TestPassTwoGivesTheKeyOnlyToAControlPlane(t *testing.T) {
	t.Parallel()

	next := authority(t)
	keyB64 := base64.StdEncoding.EncodeToString(next.Key)

	cp := rotateca.IssuePatch(next, model.RoleControlPlane)
	if !strings.Contains(cp, keyB64) {
		t.Error("a control-plane node is not given the authority's key, so trustd cannot issue " +
			"certificates to a node that joins after this rotation")
	}

	for _, role := range []model.MachineRole{model.RoleWorker, model.RoleUnknown} {
		patch := rotateca.IssuePatch(next, role)
		if strings.Contains(patch, keyB64) {
			t.Errorf("role %q is handed the authority's private key: this rotation would be the "+
				"thing that put the cluster's Talos PKI on a machine with no reason to hold it", role)
		}
		if !strings.Contains(patch, "key: \"\"") {
			t.Errorf("role %q: the key field is absent rather than emptied, so a node that used to "+
				"be a control plane keeps the OLD authority's key after the rotation", role)
		}
	}
}

// TestPassTwoActuallyMovesTheIssuingAuthority applies it and reads it back,
// rather than trusting the string.
func TestPassTwoActuallyMovesTheIssuingAuthority(t *testing.T) {
	t.Parallel()

	old := authority(t)
	next := authority(t)

	// A node that has had pass 1: it lists the old authority explicitly, which
	// is what survives pass 2 replacing machine.ca.
	base := configWith(t, old.Crt, old.Crt, next.Crt)
	after, err := machineconfig.ApplyPatches(base, []string{rotateca.IssuePatch(next, model.RoleControlPlane)})
	if err != nil {
		t.Fatalf("ApplyPatches: %v", err)
	}
	state, err := rotateca.Inspect(after)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if !state.IssuesFrom(next.Crt) {
		t.Fatal("after pass 2 the node still issues from the old authority")
	}
	if !state.Accepts(old.Crt) {
		t.Error("pass 2 dropped the old authority from the accepted set, which is pass 4's job: " +
			"this installation still holds a certificate from it at this point in the run")
	}
}

// TestPassFourRefusesBeforePassTwo is the order, enforced a second time.
//
// The passes run in order, and a resumed or hand-driven run is exactly where an
// order gets skipped. Dropping the old authority from a node that still issues
// from it cuts off the very connection carrying the rotation.
func TestPassFourRefusesBeforePassTwo(t *testing.T) {
	t.Parallel()

	old := authority(t)
	next := authority(t)

	_, _, err := rotateca.PruneConfig(configWith(t, old.Crt, old.Crt, next.Crt), next.Crt)
	if !errors.Is(err, rotateca.ErrOutOfOrder) {
		t.Fatalf("pass 4 on a node that has not had pass 2 = %v, want ErrOutOfOrder", err)
	}
}

// TestPassFourLeavesOnlyTheNewAuthority applies pass 2 and then pass 4.
func TestPassFourLeavesOnlyTheNewAuthority(t *testing.T) {
	t.Parallel()

	old := authority(t)
	next := authority(t)

	base := configWith(t, old.Crt, old.Crt, next.Crt)
	afterTwo, err := machineconfig.ApplyPatches(base, []string{rotateca.IssuePatch(next, model.RoleControlPlane)})
	if err != nil {
		t.Fatalf("ApplyPatches (pass 2): %v", err)
	}
	afterFour, changes, err := rotateca.PruneConfig(afterTwo, next.Crt)
	if err != nil {
		t.Fatalf("PruneConfig: %v", err)
	}
	if !changes {
		t.Fatal("pass 4 reports nothing to do while the old authority is still accepted")
	}
	final, err := rotateca.Inspect(afterFour)
	if err != nil {
		t.Fatalf("Inspect after pass 4: %v", err)
	}

	if final.Accepts(old.Crt) {
		t.Error("the old authority is still accepted after pass 4, so the rotation changed nothing " +
			"about what the cluster trusts -- which is the whole operation")
	}
	if !final.IssuesFrom(next.Crt) {
		t.Error("pass 4 moved the issuing authority")
	}

	// And again: a resumed run must not write a configuration it already wrote.
	if _, changes, err := rotateca.PruneConfig(afterFour, next.Crt); err != nil || changes {
		t.Errorf("pass 4 is not idempotent: changes=%v err=%v", changes, err)
	}
}

// TestARotationIsRefusedUnlessEveryNodeAnswered names the nodes rather than
// saying "some node", because an operator told "some node did not answer" has
// to check all of them.
func TestARotationIsRefusedUnlessEveryNodeAnswered(t *testing.T) {
	t.Parallel()

	if err := rotateca.RefuseUnlessEveryNodeAnswered(nil); !errors.Is(err, rotateca.ErrNoMachines) {
		t.Errorf("an empty cluster = %v, want ErrNoMachines", err)
	}

	reached := map[model.MachineID]error{"cp-1": nil, "worker-2": errors.New("timed out")}
	err := rotateca.RefuseUnlessEveryNodeAnswered(reached)
	if !errors.Is(err, rotateca.ErrNotEveryNodeAnswered) {
		t.Fatalf("a silent node = %v, want ErrNotEveryNodeAnswered", err)
	}
	if !strings.Contains(err.Error(), "worker-2") {
		t.Errorf("the refusal does not name the node that did not answer: %v", err)
	}
	if strings.Contains(err.Error(), "cp-1") {
		t.Errorf("the refusal names a node that answered, which sends the operator to the wrong machine: %v", err)
	}

	if err := rotateca.RefuseUnlessEveryNodeAnswered(map[model.MachineID]error{"cp-1": nil}); err != nil {
		t.Errorf("a cluster whose every node answered = %v, want nil", err)
	}
}

// TestAConfigurationWithNoIssuingAuthorityIsRefused: a configuration this
// package cannot reason about is not a rotation to guess at.
func TestAConfigurationWithNoIssuingAuthorityIsRefused(t *testing.T) {
	t.Parallel()

	_, err := rotateca.Inspect([]byte("version: v1alpha1\nmachine:\n  type: worker\n"))
	if !errors.Is(err, rotateca.ErrConfigUnreadable) {
		t.Fatalf("a configuration with no machine.ca = %v, want ErrConfigUnreadable", err)
	}
}

// TestInspectReadsAMultiDocumentConfiguration: Talos 1.8 and later append
// further documents after the v1alpha1 one, and a node running such a
// configuration must not make the rotation fail to read it.
func TestInspectReadsAMultiDocumentConfiguration(t *testing.T) {
	t.Parallel()

	old := authority(t)
	raw := append(configWith(t, old.Crt),
		[]byte("---\napiVersion: v1alpha1\nkind: HostnameConfig\nhostname: cp-1\n")...)

	state, err := rotateca.Inspect(raw)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if !state.IssuesFrom(old.Crt) {
		t.Error("the issuing authority was not read out of a multi-document configuration")
	}
}
