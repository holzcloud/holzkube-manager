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
// The stakes are not ordinary. holzkube holds cluster PKI, and D-16 keeps every
// rotated file forever, so a secret written here is written permanently: there
// is no path that deletes it and no path that could delete it without breaking
// the chain. A log that holds secrets would have to be guarded more carefully
// than the things it is meant to keep an eye on.

import (
	"encoding/json"
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
var allowlist = map[string][]string{
	"setup.create": {"username"},
	"auth.login":   {"username"},

	// Listed with nothing permitted, so the table shows the full set of
	// phase-1 mutations rather than leaving them to the default.
	"auth.logout":      {},
	"auth.sudo":        {},
	"account.password": {},
}

// Params returns the parameters as they may be written to the log.
//
// An action with no entry in the table redacts everything. That is the default
// on purpose: forgetting to extend the allowlist costs a useful record, while
// the opposite default would cost a secret.
func Params(action string, raw map[string]any) map[string]any {
	permitted := make(map[string]struct{}, len(allowlist[action]))
	for _, f := range allowlist[action] {
		permitted[f] = struct{}{}
	}

	out := make(map[string]any, len(raw))
	for k, v := range raw {
		out[k] = redactValue(k, permitted, v)
	}
	return out
}

// redactValue decides one node of the parameter tree.
//
// There is no branch that returns an unrecognised value unchanged. Everything
// either matches an allowlisted leaf path and is a scalar, or is a container on
// the way to one, or becomes the marker.
func redactValue(path string, permitted map[string]struct{}, v any) any {
	if _, ok := permitted[path]; ok {
		if s, ok := permittedScalar(v); ok {
			return s
		}
		// An allowlist entry names a leaf. A caller that sends an object or a
		// list where a string was expected would otherwise smuggle arbitrary
		// content past the list under a permitted name.
		return RedactedMarker
	}

	if m, ok := v.(map[string]any); ok && leadsToPermitted(path, permitted) {
		out := make(map[string]any, len(m))
		for k, vv := range m {
			out[k] = redactValue(path+"."+k, permitted, vv)
		}
		return out
	}

	// An unknown branch is replaced whole, keys included. Walking it would
	// publish its shape, and the shape of a secrets bundle is itself a map of
	// where to look.
	return RedactedMarker
}

// leadsToPermitted reports whether any allowlisted path continues below here.
func leadsToPermitted(path string, permitted map[string]struct{}) bool {
	prefix := path + "."
	for p := range permitted {
		if strings.HasPrefix(p, prefix) {
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
