package diskencryption_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/siderolabs/talos/pkg/machinery/config/configloader"
	"github.com/siderolabs/talos/pkg/machinery/config/container"
	"github.com/siderolabs/talos/pkg/machinery/config/types/block"
	"github.com/siderolabs/talos/pkg/machinery/constants"

	"github.com/holzcloud/holzkube-manager/internal/diskencryption"
)

// metalMode is the runtime mode Talos's validator is given: a bare-metal node
// that requires an install, which is the only kind this product provisions.
type metalMode struct{}

func (metalMode) String() string        { return "metal" }
func (metalMode) RequiresInstall() bool { return true }
func (metalMode) InContainer() bool     { return false }

// encode renders a request the way a machine configuration would carry it, and
// reads it back through machinery's own loader.
//
// Through the loader, deliberately. A test that inspected the Go structs would
// prove this package builds the values it meant to; what has to be true is
// that Talos reads them, and the only thing that can say so is the parser
// Talos uses. A document Talos ignores is the same outcome as no encryption at
// all, and it looks like success.
func encode(t *testing.T, r diskencryption.Request, secureBoot bool) string {
	t.Helper()

	docs, err := r.Documents(secureBoot)
	if err != nil {
		t.Fatalf("Documents: %v", err)
	}
	ctr, err := container.New(docs...)
	if err != nil {
		t.Fatalf("the documents do not form a configuration: %v", err)
	}
	raw, err := ctr.Bytes()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	if _, err := configloader.NewFromBytes(raw); err != nil {
		t.Fatalf("Talos's own loader will not read what this produced: %v\n%s", err, raw)
	}
	return string(raw)
}

// TestTalosValidatesWhatThisProduces is the guard that matters most, and the
// one a hand-written YAML template could not have.
func TestTalosValidatesWhatThisProduces(t *testing.T) {
	for _, tc := range []struct {
		name       string
		req        diskencryption.Request
		secureBoot bool
	}{
		{"nodeID, both volumes", diskencryption.Request{State: true, Ephemeral: true, Kind: diskencryption.KindNodeID}, false},
		{"nodeID, state only", diskencryption.Request{State: true, Kind: diskencryption.KindNodeID}, false},
		{"nodeID, ephemeral only", diskencryption.Request{Ephemeral: true, Kind: diskencryption.KindNodeID}, false},
		{"tpm, both volumes", diskencryption.Request{State: true, Ephemeral: true, Kind: diskencryption.KindTPM}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			docs, err := tc.req.Documents(tc.secureBoot)
			if err != nil {
				t.Fatalf("Documents: %v", err)
			}

			for _, doc := range docs {
				v, ok := doc.(*block.VolumeConfigV1Alpha1)
				if !ok {
					t.Fatalf("a document of type %T reached the configuration", doc)
				}
				warnings, err := v.Validate(metalMode{})
				if err != nil {
					t.Errorf("Talos refuses the %s document: %v", v.MetaName, err)
				}
				for _, w := range warnings {
					t.Logf("%s: %s", v.MetaName, w)
				}
			}

			_ = encode(t, tc.req, tc.secureBoot)
		})
	}
}

// TestTheVolumesAreTheOnesTalosNames. STATE and EPHEMERAL come from Talos's
// own constants rather than from string literals here: a volume name this
// product spells differently is a document Talos accepts and ignores.
func TestTheVolumesAreTheOnesTalosNames(t *testing.T) {
	raw := encode(t, diskencryption.Request{State: true, Ephemeral: true, Kind: diskencryption.KindNodeID}, false)

	for _, want := range []string{
		"kind: VolumeConfig",
		"name: " + constants.StatePartitionLabel,
		"name: " + constants.EphemeralPartitionLabel,
		"provider: luks2",
		"nodeID: {}",
	} {
		if !strings.Contains(raw, want) {
			t.Errorf("the configuration does not carry %q:\n%s", want, raw)
		}
	}
}

// TestEphemeralIsLockedToStateAndStateIsNot.
//
// Talos recommends locking EPHEMERAL to STATE: wiping or replacing STATE then
// leaves EPHEMERAL unreadable rather than leaving workload data recoverable by
// whoever supplies a new STATE. A volume cannot be locked to itself, and
// Talos's own validator refuses it for STATE -- which is why that rule is left
// to the validator rather than restated here.
func TestEphemeralIsLockedToStateAndStateIsNot(t *testing.T) {
	both := encode(t, diskencryption.Request{State: true, Ephemeral: true, Kind: diskencryption.KindNodeID}, false)
	if strings.Count(both, "lockToState: true") != 1 {
		t.Errorf("expected exactly one locked volume:\n%s", both)
	}

	// And the locked one is EPHEMERAL. Asserting the count alone would pass if
	// the two were the wrong way round.
	ephemeral := encode(t, diskencryption.Request{Ephemeral: true, Kind: diskencryption.KindNodeID}, false)
	if !strings.Contains(ephemeral, "lockToState: true") {
		t.Errorf("EPHEMERAL is not locked to STATE:\n%s", ephemeral)
	}

	state := encode(t, diskencryption.Request{State: true, Kind: diskencryption.KindNodeID}, false)
	if strings.Contains(state, "lockToState") {
		t.Errorf("STATE is locked to itself, which Talos refuses:\n%s", state)
	}
}

// TestTPMWithoutSecureBootIsRefused is the cross-check this product can make
// and a hand-written patch cannot: holzkube-manager knows which image the node
// is about to boot.
//
// The TPM seal is a statement about which kernel booted. Without SecureBoot
// that measurement can be produced by a kernel somebody else chose, so the
// configuration reads as the strong option and is not one. Shipping it quietly
// would be worse than refusing: the operator would believe the disk is
// protected against exactly the attack it is not protected against.
func TestTPMWithoutSecureBootIsRefused(t *testing.T) {
	req := diskencryption.Request{State: true, Ephemeral: true, Kind: diskencryption.KindTPM}

	err := req.Validate(false)
	if !errors.Is(err, diskencryption.ErrTPMNeedsSecureBoot) {
		t.Fatalf("TPM on a non-SecureBoot image returned %v", err)
	}
	if !strings.Contains(err.Error(), "SecureBoot variant") {
		t.Errorf("the refusal does not say how to get out of it: %v", err)
	}

	// And nothing is produced. A refusal that still returned documents would
	// be a refusal the caller can ignore by not looking at the error.
	if docs, err := req.Documents(false); err == nil || docs != nil {
		t.Errorf("documents were produced for a refused request: %v, %v", docs, err)
	}

	// With SecureBoot it is allowed, which is what makes the refusal above a
	// statement about the combination rather than about TPM.
	if err := req.Validate(true); err != nil {
		t.Errorf("TPM on a SecureBoot image was refused: %v", err)
	}
}

// TestTheTwoKeyKindsThisProductDoesNotOfferAreAnsweredWithReasons.
//
// Not "unknown value". An operator asking for static or kms has read Talos's
// documentation and is asking a reasonable question, and the answer is a
// reason.
func TestTheTwoKeyKindsThisProductDoesNotOfferAreAnsweredWithReasons(t *testing.T) {
	_, err := diskencryption.ParseKind("static")
	if !errors.Is(err, diskencryption.ErrStaticKey) {
		t.Fatalf("static returned %v", err)
	}
	if !strings.Contains(err.Error(), "META") {
		t.Errorf("the static refusal does not say why it is not encryption: %v", err)
	}

	_, err = diskencryption.ParseKind("kms")
	if !errors.Is(err, diskencryption.ErrKMSKey) {
		t.Fatalf("kms returned %v", err)
	}
	if !strings.Contains(err.Error(), "does not boot") {
		t.Errorf("the kms refusal does not say what it would cost: %v", err)
	}

	if _, err := diskencryption.ParseKind("luks"); !errors.Is(err, diskencryption.ErrUnknownKind) {
		t.Errorf("an actual typo returned %v", err)
	}

	for _, ok := range []string{"nodeID", " tpm "} {
		if _, err := diskencryption.ParseKind(ok); err != nil {
			t.Errorf("%q was refused: %v", ok, err)
		}
	}
}

// TestARequestThatEncryptsNothingIsRefused, rather than silently producing an
// empty document set that reads as "encryption configured".
func TestARequestThatEncryptsNothingIsRefused(t *testing.T) {
	req := diskencryption.Request{Kind: diskencryption.KindNodeID}
	if req.Enabled() {
		t.Error("a request selecting no volume reports itself as enabled")
	}
	if err := req.Validate(true); !errors.Is(err, diskencryption.ErrNothingSelected) {
		t.Fatalf("an empty request returned %v", err)
	}
}

// TestTheSentenceSaysWhatTheEncryptionIsNotWorth.
//
// "The disk is encrypted" is what an operator remembers. For nodeID that is
// close enough to false to be dangerous -- it protects a drive that leaves the
// machine and nothing else -- and for every kind the install-time-only
// property has to be on the same screen as the switch.
func TestTheSentenceSaysWhatTheEncryptionIsNotWorth(t *testing.T) {
	nodeID := diskencryption.Request{State: true, Kind: diskencryption.KindNodeID}.Sentence()
	if !strings.Contains(nodeID, "does not protect against anyone who has the machine") {
		t.Errorf("the nodeID sentence overstates it: %q", nodeID)
	}
	if !strings.Contains(nodeID, "in the clear") {
		t.Errorf("encrypting one volume does not say the other is not: %q", nodeID)
	}

	tpm := diskencryption.Request{State: true, Ephemeral: true, Kind: diskencryption.KindTPM}.Sentence()
	if !strings.Contains(tpm, "SecureBoot") {
		t.Errorf("the TPM sentence does not say what it depends on: %q", tpm)
	}

	for _, s := range []string{nodeID, tpm} {
		if !strings.Contains(s, "already installed") {
			t.Errorf("the sentence does not say this is install-time only: %q", s)
		}
	}
}
