package inventory_test

// Renewing the certificate holzkube-manager dials a cluster with (V2-OPS-02).
//
// The ladder on the cluster card has counted down to this since D-23 shipped:
// "the client certificate for homelab expires within a week. When it does,
// every node in this cluster becomes unreachable at once." There was nothing
// to click. These tests are about the two things that make the button worth
// having rather than worth fearing: the new certificate reaches a node, and
// when it cannot, the old one is still there.

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

func storedCertificate(t *testing.T, f *fixture, id model.ClusterID) (pemBytes []byte, notAfter time.Time) {
	t.Helper()

	sec, err := f.store.ClusterSecrets().Get(t.Context(), id)
	if err != nil {
		t.Fatalf("reading the stored secrets: %v", err)
	}
	block, _ := pem.Decode(sec.ClientCrt)
	if block == nil {
		t.Fatal("the stored client certificate is not PEM")
	}
	crt, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("the stored client certificate does not parse: %v", err)
	}
	return sec.ClientCrt, crt.NotAfter
}

// TestARenewedCertificateIsNewAndWorks.
//
// Both halves. A renewal that produced a different certificate nobody can
// connect with is worse than no renewal at all, and a renewal that quietly
// kept the old one would pass a test that only checked the cluster still
// works.
func TestARenewedCertificateIsNewAndWorks(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixture(t, talossim.Options{ControlPlane: true})
	c := f.importCluster(ctx, t)

	before, beforeExpiry := storedCertificate(t, f, c.ID)

	// Start from the state this feature exists for: the cluster card's
	// countdown says a few days. It is set on the record rather than left at
	// the freshly-minted year, because a test that renews seconds after
	// adopting has an old certificate and a new one expiring in the same
	// second -- so "the countdown moved to the new certificate" and "the
	// countdown was never touched" look identical, and a renewal that forgot
	// to update the record would pass.
	stale, err := f.store.Clusters().Get(ctx, c.ID)
	if err != nil {
		t.Fatalf("reading the cluster: %v", err)
	}
	nearly := time.Now().Add(3 * 24 * time.Hour)
	stale.ClientCertNotAfter = nearly
	if _, err := f.store.Clusters().Put(ctx, stale); err != nil {
		t.Fatalf("writing the cluster: %v", err)
	}

	after, err := f.svc.RenewClientCertificate(ctx, c.ID)
	if err != nil {
		t.Fatalf("RenewClientCertificate: %v", err)
	}

	renewed, renewedExpiry := storedCertificate(t, f, c.ID)
	if string(renewed) == string(before) {
		t.Fatal("the stored certificate did not change")
	}
	// A full fresh lifetime, not merely "at least as long as the old one".
	// The two are indistinguishable in a test that renews seconds after
	// adopting -- both expire at the same second -- and they are very
	// different in the case this feature exists for: renewing a certificate
	// with a week left must buy a year and not a week.
	wantExpiry := time.Now().Add(inventory.ClientCertTTL)
	if drift := renewedExpiry.Sub(wantExpiry); drift < -time.Minute || drift > time.Minute {
		t.Errorf("the renewed certificate expires at %s; a full lifetime from now is %s",
			renewedExpiry, wantExpiry)
	}
	if renewedExpiry.Before(beforeExpiry) {
		t.Errorf("the renewed certificate expires at %s, before the old %s",
			renewedExpiry, beforeExpiry)
	}

	// The cluster record's countdown is against the certificate that is
	// actually stored. A ladder counting down the wrong certificate is worse
	// than no ladder: it is the one thing an operator trusts to tell them when
	// to act.
	if !after.ClientCertNotAfter.Equal(renewedExpiry) {
		t.Errorf("the cluster record says %s and the stored certificate says %s",
			after.ClientCertNotAfter, renewedExpiry)
	}
	if !after.ClientCertNotAfter.After(nearly) {
		t.Errorf("the countdown still says %s after a renewal that bought a year",
			after.ClientCertNotAfter)
	}

	// And it is the record that was written, not only the value returned.
	persisted, err := f.store.Clusters().Get(ctx, c.ID)
	if err != nil {
		t.Fatalf("re-reading the cluster: %v", err)
	}
	if !persisted.ClientCertNotAfter.Equal(renewedExpiry) {
		t.Errorf("the stored cluster still counts down to %s", persisted.ClientCertNotAfter)
	}

	// And the node can still be reached, through a connection opened after the
	// write, which is the only thing that proves the new certificate is in use
	// rather than merely stored.
	machines, err := f.svc.MachinesOf(ctx, c.ID)
	if err != nil || len(machines) == 0 {
		t.Fatalf("MachinesOf: %v (%d machines)", err, len(machines))
	}
	cc, err := f.svc.Connect(ctx, machines[0].ID)
	if err != nil {
		t.Fatalf("the cluster is unreachable after renewing its certificate: %v", err)
	}
	_ = cc.Close()
}

// TestACertificateNoNodeAcceptsIsNotKept.
//
// This is the failure the whole ordering exists for. A certificate minted from
// an authority the cluster no longer trusts looks perfectly valid here -- it
// verifies against the store's own CA, because the store's own CA signed it --
// and opens nothing. Writing it would replace a working credential with a dead
// one, and the discovery would come at the moment the old one expired, which
// is exactly when nobody can get in to fix it.
//
// The fault is put in by shutting every node down, which is the reachable way
// to make "no node accepted it" true.
func TestACertificateNoNodeAcceptsIsNotKept(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixture(t, talossim.Options{ControlPlane: true})
	c := f.importCluster(ctx, t)

	before, beforeExpiry := storedCertificate(t, f, c.ID)

	if err := f.sim.Close(); err != nil {
		t.Fatalf("closing the node: %v", err)
	}

	_, err := f.svc.RenewClientCertificate(ctx, c.ID)
	if !errors.Is(err, inventory.ErrCertificateRejected) {
		t.Fatalf("renewing against an unreachable cluster returned %v, want ErrCertificateRejected", err)
	}
	if !strings.Contains(err.Error(), "nothing has changed") {
		t.Errorf("the refusal does not say the old certificate was kept: %v", err)
	}

	kept, keptExpiry := storedCertificate(t, f, c.ID)
	if string(kept) != string(before) || !keptExpiry.Equal(beforeExpiry) {
		t.Fatal("the certificate was replaced even though no node accepted the new one")
	}

	fresh, err := f.store.Clusters().Get(ctx, c.ID)
	if err != nil {
		t.Fatalf("reading the cluster: %v", err)
	}
	if !fresh.ClientCertNotAfter.Equal(c.ClientCertNotAfter) {
		t.Error("the cluster's countdown moved for a renewal that did not happen")
	}
}

// TestAClusterWithNoAuthorityKeySaysSoAndSaysWhatToDo.
//
// A cluster adopted from a talosconfig carrying an admin certificate and no
// authority key can be talked to and cannot be issued anything new. That is a
// specific situation with a specific repair, and "internal error" is the
// answer that sends somebody to the logs for a state this product can name.
func TestAClusterWithNoAuthorityKeySaysSoAndSaysWhatToDo(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixture(t, talossim.Options{ControlPlane: true})
	c := f.importCluster(ctx, t)

	sec, err := f.store.ClusterSecrets().Get(ctx, c.ID)
	if err != nil {
		t.Fatalf("reading the secrets: %v", err)
	}
	sec.OSCAKey = nil
	if _, err := f.store.ClusterSecrets().Put(ctx, sec); err != nil {
		t.Fatalf("writing the secrets: %v", err)
	}

	_, err = f.svc.RenewClientCertificate(ctx, c.ID)
	if !errors.Is(err, inventory.ErrNoCertificateAuthority) {
		t.Fatalf("renewing without an authority key returned %v, want ErrNoCertificateAuthority", err)
	}
	if !strings.Contains(err.Error(), "talosctl config new") {
		t.Errorf("the refusal does not say how to get out of it: %v", err)
	}
}

// TestRenewingAClusterThatIsNotHereIsNotFound, rather than a store error about
// a record nobody asked about by name.
func TestRenewingAClusterThatIsNotHereIsNotFound(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	f := newFixture(t, talossim.Options{ControlPlane: true})

	if _, err := f.svc.RenewClientCertificate(ctx, "no-such-cluster"); !errors.Is(err, inventory.ErrNotFound) {
		t.Fatalf("renewing an unknown cluster returned %v, want ErrNotFound", err)
	}
}
