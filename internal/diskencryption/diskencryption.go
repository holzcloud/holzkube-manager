// Package diskencryption decides how a node's system volumes are encrypted,
// and produces the configuration documents that say so.
//
// It reaches nothing and it applies nothing: the output is two YAML documents
// that travel with a machine configuration, and the encryption happens on the
// node at install time. That boundary is not a limitation of this package, it
// is the shape of the feature, and the next paragraph is why it has to be said
// out loud on every screen that offers this.
//
// **Talos encrypts a system volume only when the volume is empty.** The
// documented behaviour is "before mounting the partition, format and encrypt
// it -- this occurs only if the partition is empty and has no filesystem". So
// handing this configuration to a node that is already installed does not
// encrypt what is on it, does not fail, and does not warn: the node keeps
// running with plaintext partitions and a configuration that says otherwise.
// That is the worst failure a security feature can have -- it is indetectable
// from the configuration, and the operator believes the opposite of the truth
// -- which is why this is offered at provisioning and nowhere else.
//
// # Two key kinds are offered and two are refused
//
// Offered:
//
//   - **nodeID**, derived from the node's UUID and the partition label. It
//     protects a drive that leaves the machine and nothing else. Talos says so
//     directly, and so does the sentence this package produces, because "the
//     disk is encrypted" is what an operator will otherwise remember.
//   - **tpm**, sealed by the TPM. Strong, and only with SecureBoot -- without
//     it the measurements the seal depends on can be produced by a kernel
//     somebody else chose. This package refuses the combination rather than
//     shipping the weaker one quietly, and it can, because holzkube-manager
//     knows which image the node is about to boot.
//
// Refused:
//
//   - **static**, a passphrase written in the configuration. For STATE it is
//     not encryption at all: Talos stores the STATE volume's encryption config
//     in META in cleartext, so the passphrase protecting the disk is on the
//     disk. And a passphrase in a machine configuration is a passphrase in
//     this installation's store, in every backup of it, and in front of anyone
//     who can read a generated configuration.
//   - **kms**, sealed by a network key server. It is the strong one, and
//     taking it would mean this product runs that server -- which inverts the
//     decision the whole product is built on. holzkube-manager runs outside
//     the cluster so that it is available in exactly the failure where the
//     cluster is not; a KMS makes the nodes unable to boot when *it* is down.
//     A management tool you cannot lose is a different product from a
//     management tool whose loss bricks the fleet.
package diskencryption

import (
	"errors"
	"fmt"
	"strings"

	talosconfig "github.com/siderolabs/talos/pkg/machinery/config/config"
	"github.com/siderolabs/talos/pkg/machinery/config/types/block"
	"github.com/siderolabs/talos/pkg/machinery/constants"
	blockres "github.com/siderolabs/talos/pkg/machinery/resources/block"
)

// Kind is an encryption key kind this product offers.
//
// Two values, and the two Talos also has are deliberately absent rather than
// present-and-rejected: a closed type is how a value that must not reach a
// configuration cannot be spelled. The refusals below exist for the strings
// that arrive over HTTP, where the type system is not in the room.
type Kind string

const (
	// KindNodeID derives the key from the node UUID and the partition label.
	KindNodeID Kind = "nodeID"

	// KindTPM seals the key with the TPM. It requires SecureBoot.
	KindTPM Kind = "tpm"
)

// Errors this package refuses with. Each names a condition an operator can
// reach and can act on.
var (
	// ErrNothingSelected reports a request that encrypts no volume.
	ErrNothingSelected = errors.New("diskencryption: no volume was selected, so this encrypts nothing")

	// ErrStaticKey reports a request for a passphrase written in the
	// configuration.
	ErrStaticKey = errors.New("diskencryption: a static passphrase is not offered")

	// ErrKMSKey reports a request for a network key server.
	ErrKMSKey = errors.New("diskencryption: a network key server is not offered")

	// ErrUnknownKind reports anything else.
	ErrUnknownKind = errors.New("diskencryption: that is not an encryption key kind")

	// ErrTPMNeedsSecureBoot reports TPM sealing asked for on a node that is
	// about to boot an image without SecureBoot.
	ErrTPMNeedsSecureBoot = errors.New("diskencryption: TPM sealing without SecureBoot is not the protection it looks like")
)

// Request is what an operator asked for.
type Request struct {
	// State and Ephemeral select the system volumes.
	//
	// Separate, because they hold different things and the answer is not
	// always both: STATE holds the node's secrets and certificates, EPHEMERAL
	// holds whatever the workloads wrote. An operator encrypting one and not
	// the other is making a choice, not making a mistake.
	State     bool `json:"state"`
	Ephemeral bool `json:"ephemeral"`

	// Kind is the key kind, for both volumes. One kind rather than one per
	// volume: two different answers on one machine is a configuration nobody
	// can hold in their head, and there is no case for it that a second
	// machine would not serve better.
	Kind Kind `json:"kind"`
}

// Enabled reports whether this request asks for anything.
func (r Request) Enabled() bool { return r.State || r.Ephemeral }

// ParseKind turns a string off the wire into a Kind, naming the two Talos has
// that this product does not offer.
//
// Named rather than lumped into "unknown", because they are not typos. An
// operator asking for `static` or `kms` has read Talos's documentation and is
// asking a reasonable question; the answer is a reason, not a validation
// error about an unrecognised value.
func ParseKind(s string) (Kind, error) {
	switch strings.TrimSpace(s) {
	case string(KindNodeID):
		return KindNodeID, nil
	case string(KindTPM):
		return KindTPM, nil

	case "static":
		return "", fmt.Errorf("%w: Talos stores the STATE volume's encryption configuration in "+
			"META in cleartext, so a passphrase written there is a passphrase on the disk it "+
			"protects. It would also be in this installation's store, in every backup of it, and "+
			"in front of anyone who can read a generated configuration", ErrStaticKey)

	case "kms":
		return "", fmt.Errorf("%w: it is the strongest of the four, and offering it would mean "+
			"holzkube-manager runs that server -- and a node whose key server is down does not "+
			"boot. This product runs outside the cluster so that it is there in the failure where "+
			"the cluster is not; a fleet that cannot boot without it would be the opposite "+
			"arrangement. Use tpm with SecureBoot, or run a KMS yourself and write the "+
			"configuration by hand", ErrKMSKey)

	default:
		return "", fmt.Errorf("%w: %q. This product offers %q and %q",
			ErrUnknownKind, s, KindNodeID, KindTPM)
	}
}

// Validate checks a request against the image the node is about to boot.
//
// secureBoot is a fact about the schematic, passed in rather than looked up,
// so that this package reaches nothing and every sentence below is testable
// without a store.
func (r Request) Validate(secureBoot bool) error {
	if !r.Enabled() {
		return ErrNothingSelected
	}

	switch r.Kind {
	case KindNodeID:
	case KindTPM:
		if !secureBoot {
			return fmt.Errorf("%w: the TPM seal is a statement about which kernel booted, and "+
				"without SecureBoot that measurement can be produced by a kernel somebody else "+
				"chose. This node is set to boot a schematic built without SecureBoot. Either "+
				"build the SecureBoot variant of that schematic, or encrypt with %q and know "+
				"what it is worth", ErrTPMNeedsSecureBoot, KindNodeID)
		}
	default:
		return fmt.Errorf("%w: %q", ErrUnknownKind, r.Kind)
	}
	return nil
}

// Documents builds the VolumeConfig documents for this request.
//
// They are machinery's own types rather than YAML this package writes, so the
// field names are the ones Talos reads and the validation is Talos's. A
// hand-written document that Talos ignores is the same failure as no
// encryption at all, and it looks like success.
func (r Request) Documents(secureBoot bool) ([]talosconfig.Document, error) {
	if err := r.Validate(secureBoot); err != nil {
		return nil, err
	}

	var docs []talosconfig.Document
	if r.State {
		// No lockToState on STATE: a volume cannot be locked to itself, and
		// Talos's own validator refuses it. The rule is left to that validator
		// rather than restated here -- two places saying the same thing is two
		// places to disagree.
		docs = append(docs, r.volume(constants.StatePartitionLabel, false))
	}
	if r.Ephemeral {
		// Locked to STATE, which Talos recommends for this volume: it means
		// that wiping or replacing STATE leaves EPHEMERAL unreadable rather
		// than leaving workload data recoverable by whoever supplies a new
		// STATE.
		docs = append(docs, r.volume(constants.EphemeralPartitionLabel, true))
	}
	return docs, nil
}

func (r Request) volume(name string, lockToState bool) *block.VolumeConfigV1Alpha1 {
	key := block.EncryptionKey{KeySlot: 0}
	switch r.Kind {
	case KindNodeID:
		key.KeyNodeID = &block.EncryptionKeyNodeID{}
	case KindTPM:
		key.KeyTPM = &block.EncryptionKeyTPM{}
	}
	if lockToState {
		locked := true
		key.KeyLockToSTATE = &locked
	}

	doc := block.NewVolumeConfigV1Alpha1()
	doc.MetaName = name
	doc.EncryptionSpec = block.EncryptionSpec{
		EncryptionProvider: blockres.EncryptionProviderLUKS2,
		EncryptionKeys:     []block.EncryptionKey{key},
	}
	return doc
}

// Sentence is the request in words, including what it is not worth.
//
// The second half is the part that matters. "The disk is encrypted" is what an
// operator remembers, and for nodeID that sentence is close enough to false to
// be dangerous: it protects a drive that leaves the machine and nothing else.
func (r Request) Sentence() string {
	if !r.Enabled() {
		return "Nothing is encrypted."
	}

	var volumes string
	switch {
	case r.State && r.Ephemeral:
		volumes = "STATE (the node's secrets and certificates) and EPHEMERAL (whatever the workloads write)"
	case r.State:
		volumes = "STATE (the node's secrets and certificates); EPHEMERAL, which is where workload data lands, stays in the clear"
	default:
		volumes = "EPHEMERAL (whatever the workloads write); STATE, which holds the node's secrets and certificates, stays in the clear"
	}

	worth := ""
	switch r.Kind {
	case KindNodeID:
		worth = " The key is derived from this machine's identity, so it protects a drive that " +
			"leaves the machine — a disposal, a warranty return, a theft of the disk. It does " +
			"not protect against anyone who has the machine."
	case KindTPM:
		worth = " The key is sealed by the TPM against the measurements of a SecureBoot chain, " +
			"so it protects the disk and also refuses to unseal under a kernel that is not the " +
			"one this node is supposed to boot."
	}

	return fmt.Sprintf("At install, Talos encrypts %s, with a %s key.%s "+
		"This applies to an empty partition only: a node that is already installed keeps its "+
		"plaintext partitions and says nothing about it.", volumes, r.Kind, worth)
}
