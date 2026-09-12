package talos_test

import (
	"context"
	"crypto/tls"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// TestAPinnedMaintenanceConnectionRefusesTheWrongCertificate is PROV-04's
// actual guarantee, which until now was a comment.
//
// A machine in maintenance mode has no cluster PKI. The connection verifies no
// chain, so the fingerprint the operator read off the machine's own console is
// the only thing standing between "this is the machine on my bench" and "this
// is something else on the same network" -- and the call the path exists to
// make is ApplyConfiguration, which hands over the cluster's secrets.
//
// Both directions are asserted. A test that only checked the refusal would pass
// against a pin that refused everything, which is the same product with a
// different failure.
func TestAPinnedMaintenanceConnectionRefusesTheWrongCertificate(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "bare-1", Maintenance: true})

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	target := simTarget(sim, "00000000-0000-0000-0000-0000000000fe")

	// The value the wizard shows the operator, read the way the wizard reads
	// it: from the certificate the node actually presents.
	fp, err := talos.ServerFingerprint(ctx, talos.NewDirectDialer(sim.Port()), target)
	if err != nil {
		t.Fatalf("ServerFingerprint: %v", err)
	}
	if fp == "" {
		t.Fatal("the probe returned an empty fingerprint, so this test would pin nothing")
	}

	dialer := talos.NewDirectDialer(sim.Port())

	t.Run("the confirmed fingerprint connects", func(t *testing.T) {
		creds := sim.MaintenanceCreds()
		creds.Fingerprint = fp

		mc, err := talos.NewMaintenanceClient(ctx, dialer, target, creds, talos.Mode{})
		if err != nil {
			t.Fatalf("a connection pinned to the node's own certificate was refused: %v", err)
		}
		_ = mc.Close()
	})

	t.Run("a fingerprint that survives a paste still connects", func(t *testing.T) {
		creds := sim.MaintenanceCreds()
		// What a terminal actually hands over: a trailing newline, and the
		// other case. Refusing this teaches an operator that the field is
		// unreliable, and an operator who has learned that stops reading it.
		creds.Fingerprint = "  " + strings.ToLower(fp) + "\n"

		mc, err := talos.NewMaintenanceClient(ctx, dialer, target, creds, talos.Mode{})
		if err != nil {
			t.Fatalf("a pasted fingerprint was refused: %v", err)
		}
		_ = mc.Close()
	})

	t.Run("a different machine's fingerprint is refused", func(t *testing.T) {
		other := newSim(t, talossim.Options{Hostname: "bare-2", Maintenance: true})
		otherFP, err := talos.ServerFingerprint(ctx, talos.NewDirectDialer(other.Port()), simTarget(other, "x"))
		if err != nil {
			t.Fatalf("ServerFingerprint on the second node: %v", err)
		}
		if otherFP == fp {
			t.Fatal("both simulated nodes present the same certificate, so this test cannot tell " +
				"a pin from no pin")
		}

		creds := sim.MaintenanceCreds()
		creds.Fingerprint = otherFP

		mc, err := talos.NewMaintenanceClient(ctx, dialer, target, creds, talos.Mode{})
		if err == nil {
			_ = mc.Close()
			t.Fatal("a node presenting a certificate the operator never confirmed was accepted. " +
				"On the maintenance path there is no PKI behind the connection, so this is the " +
				"whole of PROV-04")
		}

		// And it has to arrive as itself. gRPC turns a rejected handshake into
		// a status with no trailers, which classify reads as "nothing
		// answered" -- so without the pin record this refusal reaches the
		// operator as "the node could not be reached", and they go to check a
		// cable while somebody else is answering on the wire.
		if !errors.Is(err, talos.ErrFingerprintPin) {
			t.Fatalf("a rejected certificate was reported as something else: %v", err)
		}
		if strings.Contains(err.Error(), "unreachable") {
			t.Errorf("the refusal is worded as an unreachable node: %v", err)
		}

		// The operator has to be able to act on it, which means the error has
		// to say what was presented as well as what was expected -- they are
		// comparing two strings by eye against a console.
		if !strings.Contains(err.Error(), "expected") || !strings.Contains(err.Error(), otherFP) {
			t.Errorf("the refusal does not name both fingerprints: %v", err)
		}
	})
}

// TestAnUnpinnedMaintenanceConnectionIsStillPossible pins the other half of the
// decision.
//
// A fingerprint is not mandatory: PROV-04 makes the operator's confirmation the
// trust anchor, and the screen already says what an unconfirmed connection is
// and is not worth. Making it mandatory here would break the scan, which
// reaches a machine precisely in order to learn the value.
func TestAnUnpinnedMaintenanceConnectionIsStillPossible(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "bare-3", Maintenance: true})

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	mc, err := talos.NewMaintenanceClient(ctx, talos.NewDirectDialer(sim.Port()),
		simTarget(sim, "00000000-0000-0000-0000-0000000000fd"), sim.MaintenanceCreds(), talos.Mode{})
	if err != nil {
		t.Fatalf("a connection with no fingerprint was refused: %v", err)
	}
	_ = mc.Close()
}

// TestThePinIsNotInstalledOnTheCallersConfiguration is the aliasing bug this
// would otherwise have.
//
// The composition root builds one tls.Config per request and hands it over on
// Creds. Installing a verifier on that value rather than on a copy would leave
// one node's pin attached to a configuration a later call reuses -- which fails
// closed, and fails closed against the wrong node, which is the hardest kind of
// outage to read.
func TestThePinIsNotInstalledOnTheCallersConfiguration(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "bare-4", Maintenance: true})

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	target := simTarget(sim, "00000000-0000-0000-0000-0000000000fc")
	fp, err := talos.ServerFingerprint(ctx, talos.NewDirectDialer(sim.Port()), target)
	if err != nil {
		t.Fatalf("ServerFingerprint: %v", err)
	}

	creds := sim.MaintenanceCreds()
	creds.Fingerprint = fp

	mc, err := talos.NewMaintenanceClient(ctx, talos.NewDirectDialer(sim.Port()), target, creds, talos.Mode{})
	if err != nil {
		t.Fatalf("NewMaintenanceClient: %v", err)
	}
	_ = mc.Close()

	if creds.TLS.VerifyConnection != nil || creds.TLS.VerifyPeerCertificate != nil {
		t.Error("the pin was installed on the caller's own tls.Config; the next connection built " +
			"from it would carry this node's fingerprint to a different node")
	}
}

// TestThePinSurvivesSessionResumption is the hole gosec found in this pin a few
// minutes after it was written.
//
// Go does not call VerifyPeerCertificate on a resumed TLS session: there is no
// certificate message to hand it. A pin written that way holds on the first
// handshake and stops holding on every one after it -- and passes every test
// that connects once, which is every test somebody would think to write.
// VerifyConnection runs on both.
//
// The assertion is made against a live client session cache, which is what
// makes resumption actually happen rather than merely be possible.
func TestThePinSurvivesSessionResumption(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "bare-5", Maintenance: true})

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	target := simTarget(sim, "00000000-0000-0000-0000-0000000000fb")
	dialer := talos.NewDirectDialer(sim.Port())

	fp, err := talos.ServerFingerprint(ctx, dialer, target)
	if err != nil {
		t.Fatalf("ServerFingerprint: %v", err)
	}

	// One cache shared by both connections, so the second handshake can resume
	// the first's session.
	cache := tls.NewLRUClientSessionCache(8)

	good := sim.MaintenanceCreds()
	good.TLS.ClientSessionCache = cache
	good.Fingerprint = fp

	first, err := talos.NewMaintenanceClient(ctx, dialer, target, good, talos.Mode{})
	if err != nil {
		t.Fatalf("the first connection was refused: %v", err)
	}
	_ = first.Close()

	// Now the same cache, the same node, and the *wrong* fingerprint. If the
	// check only ran on a full handshake, a resumed session would sail past it.
	bad := sim.MaintenanceCreds()
	bad.TLS.ClientSessionCache = cache
	bad.Fingerprint = "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:" +
		"AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88"

	mc, err := talos.NewMaintenanceClient(ctx, dialer, target, bad, talos.Mode{})
	if err == nil {
		_ = mc.Close()
		t.Fatal("a wrong fingerprint was accepted on a connection that could resume an earlier " +
			"session; the pin is not applied to resumed handshakes")
	}
	if !errors.Is(err, talos.ErrFingerprintPin) {
		t.Fatalf("the refusal is not a pin failure: %v", err)
	}
}
