package machineconfig

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/siderolabs/talos/pkg/machinery/config/configloader"
	"github.com/siderolabs/talos/pkg/machinery/config/configpatcher"
	"github.com/siderolabs/talos/pkg/machinery/config/encoder"
	"gopkg.in/yaml.v3"
)

var (
	// ErrPatchNotStrategic reports a patch that is not a strategic merge.
	//
	// The refusal is deliberate and it is about lists. RFC 6902 addresses list
	// elements by index -- "replace /machine/certSANs/2" -- and an index is a
	// claim about a list as it happened to be when the patch was written.
	// Applied to a node whose list is one longer, it edits the wrong entry and
	// reports success.
	ErrPatchNotStrategic = errors.New("machineconfig: only strategic merge patches are accepted")

	// ErrPatchInvalid reports a patch that does not parse or does not apply.
	ErrPatchInvalid = errors.New("machineconfig: the patch could not be applied")
)

// ValidatePatch checks that a patch body is a usable strategic merge patch.
//
// It is called before a patch is stored and again before it is applied. Twice,
// because the two moments have different inputs: authoring checks the patch
// against nothing, and applying checks it against a specific node's
// configuration -- and a patch that is fine in the abstract can still be wrong
// for a node.
func ValidatePatch(body string) error {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return fmt.Errorf("%w: it is empty", ErrPatchInvalid)
	}

	// A JSON Patch is a top-level array of operations. It is refused by shape
	// rather than by trying it and seeing, so the message can say why rather
	// than reporting a parse failure.
	var asList []any
	if err := yaml.Unmarshal([]byte(trimmed), &asList); err == nil && len(asList) > 0 {
		if _, isOp := firstOp(asList); isOp {
			return fmt.Errorf("%w: this is an RFC 6902 JSON patch. It addresses list entries by "+
				"index, which means the same patch edits a different entry on a node whose list "+
				"is a different length -- and reports success", ErrPatchNotStrategic)
		}
		return fmt.Errorf("%w: a strategic merge patch is a mapping, not a list", ErrPatchNotStrategic)
	}

	var asMap map[string]any
	if err := yaml.Unmarshal([]byte(trimmed), &asMap); err != nil {
		return fmt.Errorf("%w: %w", ErrPatchInvalid, err)
	}
	if len(asMap) == 0 {
		return fmt.Errorf("%w: it changes nothing", ErrPatchInvalid)
	}
	return nil
}

func firstOp(list []any) (string, bool) {
	m, ok := list[0].(map[string]any)
	if !ok {
		return "", false
	}
	op, ok := m["op"].(string)
	return op, ok
}

// ApplyPatches merges patches into a configuration, in order, and returns the
// result.
//
// The result is **not** redacted: it is the configuration that would be sent
// to a node, and sending a redacted one would send the marker as the CA key.
// Every path that shows it to anybody goes through Redact separately, which is
// why that function is the one thing this package guards rather than a flag on
// this one.
func ApplyPatches(base []byte, patches []string) ([]byte, error) {
	provider, err := configloader.NewFromBytes(base)
	if err != nil {
		return nil, fmt.Errorf("%w: the node's configuration did not parse: %w", ErrPatchInvalid, err)
	}

	for i, body := range patches {
		if err := ValidatePatch(body); err != nil {
			return nil, fmt.Errorf("patch %d: %w", i+1, err)
		}

		loaded, err := configpatcher.LoadPatch([]byte(body))
		if err != nil {
			return nil, fmt.Errorf("%w: patch %d: %w", ErrPatchInvalid, i+1, err)
		}

		strategic, ok := loaded.(configpatcher.StrategicMergePatch)
		if !ok {
			return nil, fmt.Errorf("%w: patch %d", ErrPatchNotStrategic, i+1)
		}

		provider, err = configpatcher.StrategicMerge(provider, strategic)
		if err != nil {
			return nil, fmt.Errorf("%w: patch %d: %w", ErrPatchInvalid, i+1, err)
		}
	}

	out, err := provider.EncodeBytes(encoder.WithComments(encoder.CommentsDisabled))
	if err != nil {
		return nil, fmt.Errorf("machineconfig: encode the patched configuration: %w", err)
	}
	return out, nil
}

// IsIdempotent reports whether applying a patch a second time changes anything
// (CFG-05).
//
// It matters because a strategic merge patch that appends to a list is *not*
// idempotent, and the failure is quiet: the second apply succeeds, the list has
// the entry twice, and nothing says so until something downstream chokes on the
// duplicate. Checking it is one extra merge and a comparison.
func IsIdempotent(base []byte, body string) (bool, error) {
	once, err := ApplyPatches(base, []string{body})
	if err != nil {
		return false, err
	}
	twice, err := ApplyPatches(once, []string{body})
	if err != nil {
		return false, err
	}
	return bytes.Equal(once, twice), nil
}
