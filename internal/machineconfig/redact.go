// Package machineconfig is holzkube-manager's config domain: viewing, patching,
// diffing and applying a node's machine configuration.
//
// One function in this package matters more than the rest, and it is the
// first one below. A machine configuration contains the cluster's certificate
// authority private key. The feature "look at a node's config" is therefore
// the same feature as "hand the cluster over", unless every path out of this
// package goes through Redact -- and "every path" is five of them: the
// rendered view, the raw tab, the diff, any API response, and the audit log.
//
// Redaction is not sequenced after the view. It is in the same package, in
// front of the same five exits, and asserted by a test that walks all five.
package machineconfig

import (
	"fmt"
	"regexp"

	"github.com/siderolabs/talos/pkg/machinery/config/configloader"
	"github.com/siderolabs/talos/pkg/machinery/config/encoder"
)

// RedactedMarker is what replaces a secret.
//
// It is a sentence rather than a row of asterisks, because somebody reading a
// config with it in should know immediately that holzkube-manager removed it rather
// than that the node has an odd value there.
const RedactedMarker = "<redacted by holzkube-manager>"

// pemPrivateKey matches any PEM private-key block.
//
// This is the belt to machinery's braces, and it is here because the two fail
// differently. Machinery's RedactSecrets knows the fields of the schema it was
// compiled against; a future Talos release that adds a secret-bearing field
// would redact nothing of it, and holzkube-manager would leak a key it had never
// heard of. A PEM private key, on the other hand, is unmistakable whatever
// field it sits in.
//
// It is deliberately not an entropy heuristic. Entropy would also flag image
// digests, hashes and UUIDs -- values an operator needs to read -- and a
// redaction that removes things people need is a redaction people turn off.
var pemPrivateKey = regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`)

// Redact returns a machine configuration with every secret removed.
//
// It is the only way config bytes leave this package, and the only function
// the five exits call. Two passes, because they fail in different directions:
//
//  1. machinery's own RedactSecrets, which knows the schema and removes the
//     fields it names -- including ones that are not obviously secrets, like
//     the cluster token;
//  2. a sweep for PEM private keys over the resulting bytes, which knows
//     nothing about the schema and catches a key in a field this build has
//     never heard of.
//
// Neither alone is enough. The first misses what upstream adds after this
// build; the second misses anything that is a secret without looking like one.
//
// A configuration that does not parse is returned as nothing, with the error:
// the first pass cannot run on it, and the second alone is not a redaction.
func Redact(raw []byte) ([]byte, error) {
	provider, err := configloader.NewFromBytes(raw)
	if err != nil {
		// Nothing comes back. This used to return the bytes with the PEM
		// sweep run over them, on the reasoning that an operator has to be
		// able to see what is wrong with a configuration that does not parse.
		// But the sweep finds literal PEM blocks, and a Talos configuration
		// has none: its keys are base64 of the PEM, and its tokens are not
		// PEM at all. A configuration that did not parse -- one document kind
		// this build has never heard of is enough -- therefore came back
		// whole, and the support bundle wrote it into a file meant to be
		// handed to somebody else.
		//
		// There is no text-level redaction that is safe here: without the
		// schema, a secret is any string, and guessing which ones removes
		// either too little or what the reader needed.
		return nil, fmt.Errorf("machineconfig: the configuration did not parse, so it cannot be "+
			"redacted and none of it is shown: %w", err)
	}

	redacted := provider.RedactSecrets(RedactedMarker)

	out, err := redacted.EncodeBytes(encoder.WithComments(encoder.CommentsDisabled))
	if err != nil {
		return nil, fmt.Errorf("machineconfig: encode the redacted configuration: %w", err)
	}

	return pemPrivateKey.ReplaceAll(out, []byte(RedactedMarker)), nil
}

// RedactString is Redact for callers holding a string, which is every UI path.
func RedactString(raw string) (string, error) {
	out, err := Redact([]byte(raw))
	return string(out), err
}

// ContainsPrivateKey reports whether a blob still holds a PEM private key.
//
// It exists so that a test can make the claim this package rests on -- and so
// that a caller which has to hand bytes somewhere irreversible, the audit
// archive above all, can assert it rather than assume it.
func ContainsPrivateKey(b []byte) bool { return pemPrivateKey.Match(b) }
