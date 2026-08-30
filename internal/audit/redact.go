package audit

// Allowlist redaction of input parameters (D-14).
//
// The direction of this file is the whole point. A list of forbidden fields
// forgets the next secret: it protects against the fields somebody thought of,
// and quietly passes through the one added in phase 7 by an author who never
// read this file. An allowlist fails the other way -- a new parameter shows up
// as a marker until somebody deliberately adds it here -- and the failure is
// visible, recoverable and costs nothing. There is no list of exclusions in
// this package and there is no branch that passes an unrecognised value
// through; both absences are asserted by the plan's gate greps.
//
// The stakes are not ordinary. holzkube-manager holds cluster PKI, and D-16 keeps every
// rotated file forever, so a secret written here is written permanently: there
// is no path that deletes it and no path that could delete it without breaking
// the chain. A log that holds secrets would have to be guarded more carefully
// than the things it is meant to keep an eye on.

import (
	"encoding/json"
	"slices"
	"strings"
	"unicode/utf8"
)

// RedactedMarker replaces every value that is not explicitly permitted.
//
// The key survives, the value does not: knowing that a field was sent is
// forensically useful, and knowing what was in it is exactly the risk.
const RedactedMarker = "<redacted>"

const (
	// maxParamLen caps a permitted value. Without it a caller could park a
	// megabyte in an allowlisted field of an archive nothing ever removes.
	maxParamLen = 256

	truncationMarker = "…(truncated)"
)

// allowlist maps an action token to the parameter paths that may appear in
// clear. Nested paths are dotted; every entry names a leaf, never a branch.
//
// Phase 1 permits exactly two things, both of them identifiers the operator
// chose for themselves and neither of them a credential. Everything else --
// passwords on setup, login and password change, and every field of every
// action added later -- is redacted until listed here on purpose.
//
// The test for whether a field belongs here is not "is it interesting" but "is
// it an identifier the operator chose, and is it certainly not a credential".
// schematic.create is the case that makes the difference concrete. Its body
// carries kernel arguments and META values, and the Image Factory itself
// refuses to enumerate schematics precisely because those may hold secrets. D-16
// keeps every rotated audit file forever and defines no deletion path, so a
// kernel argument written in clear here is written in clear permanently -- there
// is no migration and no redaction pass that could take it back without breaking
// the hash chain. So schematic.create permits the operator's own label and the
// Talos version, and the two fields that could carry a secret are left to the
// fail-closed default along with everything else.
var allowlist = map[string][]string{
	"setup.create": {"username"},
	"auth.login":   {"username"},

	// The schematic's label and the version it targets: an operator-chosen
	// name and a public version string. Deliberately NOT kernel_args, meta,
	// extensions or canonical -- see the paragraph above.
	"schematic.create": {"name", "talos_version"},

	// Listed with nothing permitted, so the table shows the full set of
	// mutations rather than leaving any of them to the default.
	"auth.logout":      {},
	"auth.sudo":        {},
	"account.password": {},

	// A deletion carries its id in the path, not in the body: there is nothing
	// here worth writing in clear.
	"schematic.delete": {},
}

// Params returns the parameters as they may be written to the log.
//
// An action with no entry in the table redacts everything. That is the default
// on purpose: forgetting to extend the allowlist costs a useful record, while
// the opposite default would cost a secret.
func Params(action string, raw map[string]any) map[string]any {
	// Allowlist entries are authored as dotted paths and are split into
	// segments here, because a dot in the table means "descend" while a dot in
	// a JSON key is just a character. Comparing the joined strings conflated
	// the two: an entry "user.name", meant to permit {"user":{"name":...}},
	// also permitted a body sending the flat key {"user.name": "..."}. JSON
	// object keys may contain dots and readBody decodes whatever arrives, so
	// the two namespaces have to be kept apart.
	permitted := make([][]string, 0, len(allowlist[action]))
	for _, f := range allowlist[action] {
		permitted = append(permitted, strings.Split(f, "."))
	}

	out := make(map[string]any, len(raw))
	for k, v := range raw {
		out[k] = redactValue([]string{k}, permitted, v)
	}
	return out
}

// redactValue decides one node of the parameter tree.
//
// There is no branch that returns an unrecognised value unchanged. Everything
// either matches an allowlisted leaf path and is a scalar, or is a container on
// the way to one, or becomes the marker.
func redactValue(segments []string, permitted [][]string, v any) any {
	if isPermittedLeaf(segments, permitted) {
		if s, ok := permittedScalar(v); ok {
			return s
		}
		// An allowlist entry names a leaf. A caller that sends an object or a
		// list where a string was expected would otherwise smuggle arbitrary
		// content past the list under a permitted name.
		return RedactedMarker
	}

	if m, ok := v.(map[string]any); ok && leadsToPermitted(segments, permitted) {
		out := make(map[string]any, len(m))
		for k, vv := range m {
			// Full slice expression: without capping the capacity, every key
			// of this object would append into the same backing array and
			// siblings would overwrite each other's segment.
			child := append(segments[:len(segments):len(segments)], k)
			out[k] = redactValue(child, permitted, vv)
		}
		return out
	}

	// An unknown branch is replaced whole, keys included. Walking it would
	// publish its shape, and the shape of a secrets bundle is itself a map of
	// where to look.
	return RedactedMarker
}

// isPermittedLeaf reports whether segments names an allowlisted leaf exactly.
func isPermittedLeaf(segments []string, permitted [][]string) bool {
	for _, p := range permitted {
		if slices.Equal(p, segments) {
			return true
		}
	}
	return false
}

// leadsToPermitted reports whether any allowlisted path continues below here.
func leadsToPermitted(segments []string, permitted [][]string) bool {
	for _, p := range permitted {
		if len(p) > len(segments) && slices.Equal(p[:len(segments)], segments) {
			return true
		}
	}
	return false
}

// permittedScalar accepts the leaf kinds a record can carry, capped in length.
func permittedScalar(v any) (any, bool) {
	switch t := v.(type) {
	case nil:
		return nil, true
	case bool:
		return t, true
	case json.Number:
		return t, true
	case float64, int, int64, uint64:
		return t, true
	case string:
		return capLength(t), true
	default:
		return nil, false
	}
}

func capLength(s string) string {
	if len(s) <= maxParamLen {
		return s
	}
	// Cut on a rune boundary. Half a rune is invalid UTF-8, and the canonical
	// form -- which the hash is taken over -- has to stay well-defined.
	cut := maxParamLen
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + truncationMarker
}

// maxCodeLen bounds a taxonomy code. The longest one the taxonomy defines is
// well under this; the cap is here so that a value which merely happens to be
// token-shaped still cannot be arbitrarily long.
const maxCodeLen = 64

// OutcomeCause returns a failure reason as it may be written to the log.
//
// An outcome names a stable taxonomy code -- "sudo.required",
// "validation.failed", "http.500" -- and never free text. This is the door that
// enforces it, rather than whichever call site happened to remember: Outcome is
// exported, and the first caller to pass a real error would otherwise write a
// filesystem path or a store message straight into an archive that is kept
// forever and has no deletion path.
//
// A value that is not code-shaped is redacted rather than truncated. Truncating
// would keep the first 256 characters of exactly the free text this rejects.
func OutcomeCause(s string) string {
	if !isCodeToken(s) {
		return RedactedMarker
	}
	return s
}

// isCodeToken checks the shape of a taxonomy code before it is copied into a
// permanent record: lowercase, digits and the three separators the taxonomy
// uses, and nothing else. It is deliberately a duplicate of the check the audit
// middleware runs on the way out of a problem response; the invariant belongs
// to this package, and a check that lives only in the caller is a check the
// next caller does not have.
func isCodeToken(s string) bool {
	if s == "" || len(s) > maxCodeLen {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}
