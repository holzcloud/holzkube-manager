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

	// maxParamItems caps a permitted list. An allowlisted field that is a list
	// is a list of identifiers -- patch ids, addresses to scan -- and a
	// hundred of them is not a longer record, it is somebody using an
	// allowlisted name as storage in an archive nothing removes.
	maxParamItems = 32
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

	// The inventory, phase 3.
	//
	// cluster.import is the entry this table was most at risk of getting
	// wrong, and the risk runs in one direction only. Its body carries a
	// talosconfig: a certificate authority, a client certificate and a client
	// *private key*. That field is deliberately absent from this list, so the
	// fail-closed default writes `<redacted>` for it -- which is the whole
	// reason this file is an allowlist. Permitted here are the cluster's name,
	// the address the operator named and the fingerprint they confirmed:
	// an operator-chosen label and two facts that are already public to
	// anybody who can reach the node.
	"cluster.import":      {"name", "endpoint", "fingerprint"},
	"cluster.fingerprint": {"endpoint"},

	// A created cluster names itself and its Kubernetes endpoint. Neither is a
	// secret; what this action generates is, and that never enters a request
	// body at all.
	"cluster.create": {"name", "endpoint"},

	// Whether the lock was opened or closed is the entire content of the
	// event, and it is not a secret. A lock change with a redacted direction
	// would be a record that says something happened and not what.
	"cluster.lock": {"locked"},

	// The cluster this machine was added to and the address it was found at.
	// Neither is a credential; the credentials are the cluster's, and they are
	// not in this body at all.
	"machine.add": {"cluster", "addr"},

	// The three audited *reads*. They are GETs, so there is no body to permit
	// anything out of, and the table lists them with nothing rather than
	// leaving them to the default -- which is the distinction this table
	// claims to make about itself and which cmd/holzkube-managerd's
	// allowlist_test.go is what actually holds.
	"system.status": {},
	"auth.me":       {},
	"audit.list":    {},

	// Listed with nothing permitted, so the table shows the full set of
	// mutations rather than leaving any of them to the default. Each carries
	// its id in the path.
	"cluster.forget":  {},
	"machine.forget":  {},
	"machine.refresh": {},

	// Node actions, phase 6.
	//
	// The *scope of a wipe* is the single most important thing this archive
	// can hold about a reset: "a reset happened on node X" and "every disk on
	// node X was wiped" are different events, and a record that cannot tell
	// them apart is a record nobody can answer a question with. None of these
	// is a credential -- they are the choices an operator made on a screen.
	//
	// `confirmation` is deliberately absent from every one of them. It is an
	// HMAC over the intent, it is the thing that authorises the action, and an
	// archive with no deletion path is the last place it belongs.
	"node.reboot":   {"cluster"},
	"node.shutdown": {"cluster"},
	"node.reset": {
		"cluster",
		"params.mode", "params.graceful", "params.reboot", "params.disks[]",
	},

	// What was confirmed, so that a token issued and never used still leaves a
	// record of what somebody was about to do. `typed` is absent: it is the
	// machine's hostname, which the record already carries, and writing back
	// what a person typed into a box is a habit worth not starting.
	"action.confirm": {"action", "params.mode", "params.graceful", "params.reboot"},

	// A cancel names its job in the path.
	"job.cancel": {},

	// Machine configuration, phase 7.
	//
	// The patch *ids* are permitted and the patch *bodies* are not. A body is
	// arbitrary configuration, configuration is where the secrets are, and
	// this archive is kept forever. A stored patch stays readable at its own
	// id for as long as it exists -- which is also forever -- so the record is
	// complete without the archive holding the bytes.
	"config.plan":  {"cluster", "patch_ids[]"},
	"config.apply": {"cluster", "mode", "patch_ids[]"},

	// The patch's name, and deliberately not its body, for the reason above.
	"patch.create": {"name"},

	// Provisioning, phase 8.
	//
	// Everything here is an address, an identifier or a choice the operator
	// made on a screen, and the one field that could be a credential --
	// `confirmation` -- is absent, as it is for every node action.
	//
	// The scan's own addresses are worth keeping in clear for the reason the
	// reset's disks are: "a scan happened" and "this installation scanned
	// 10.0.0.0/24" are different events, and the second is the one somebody
	// reviewing an unexpected connection on their network is looking for.
	"provision.scan": {"cidr", "addrs[]"},
	"provision.confirm": {
		"cluster", "addr", "uuid", "control_plane", "install_disk",
		"schematic_id", "talos_version", "fingerprint", "hostname", "patch_ids[]",
	},
	"provision.inspect": {"addr", "fingerprint"},

	// The plan and the apply take the same body, so they permit the same
	// fields: what was shown on the screen is what was submitted, and a record
	// where the two differ in shape would be a record that cannot be compared.
	"provision.plan": {
		"cluster", "addr", "uuid", "control_plane", "install_disk",
		"schematic_id", "talos_version", "fingerprint", "hostname", "patch_ids[]",
	},
	"provision.apply": {
		"cluster", "addr", "uuid", "control_plane", "install_disk",
		"schematic_id", "talos_version", "fingerprint", "hostname", "patch_ids[]",
	},

	// The operator's verdict on an unclear bootstrap, and what they looked at.
	// This record is the only account anything will ever have of a bootstrap
	// nothing could decide, and redacting the note would leave the archive
	// saying that somebody decided, without what they decided or why.
	"provision.bootstrap-resolve": {"bootstrapped", "note"},

	// Upgrades and etcd management, phase 9.
	//
	// The version is the whole content of an upgrade record. "A Talos upgrade
	// was run on this cluster" and "this cluster was taken from 1.13.9 to
	// 1.14.0 on the eleventh" are different events, and only the second one
	// answers the question somebody asks six months later about when a
	// behaviour changed.
	//
	// `confirmation` is absent from the two that carry one, as it is
	// everywhere: it is the HMAC that authorises the run.
	"upgrade.plan":            {"to"},
	"upgrade.plan-kubernetes": {"to"},
	"upgrade.confirm":         {"kind", "to"},
	"upgrade.talos":           {"to"},
	"upgrade.kubernetes":      {"to"},

	// The member removal names its member in the path and the cluster in the
	// path; there is no body worth permitting anything out of. It is listed so
	// the table shows it.
	"etcd.remove-member": {},

	// A snapshot is a GET: no body, nothing to permit. It is listed for the
	// same reason -- and because "somebody took a copy of this cluster's etcd"
	// is exactly the kind of event an archive exists to hold, even when the
	// record is only that it happened.
	"etcd.snapshot": {},

	// Whether a node was locked, and why. The reason is the content: a lock
	// record that says a lock happened and not why is a record of nothing, and
	// the reason is an operator's own sentence about their own fleet.
	"machine.lock": {"locked", "reason"},

	// The cluster a node was removed from. The node is in the path.
	"node.remove-from-cluster": {"cluster"},

	// The support bundle, v1.15. A GET naming its cluster in the path, so
	// there is no body to permit anything out of -- and it is listed rather
	// than left to the default because "somebody took a copy of everything
	// this installation knows about a cluster" is exactly the event an archive
	// exists to hold, even when the record is only that it happened.
	"support.bundle": {},
}

// Listed reports whether an action has an entry in the table.
//
// It is not used by the redaction -- an action with no entry redacts
// everything, which is the fail-closed default and is correct. It exists so
// that the composition root can assert the other half of the claim this table
// makes about itself: that it shows the **full set** of mutations, including
// the ones that permit nothing.
//
// The difference matters because the two look identical from inside this
// package. An action deliberately listed as permitting nothing and an action
// whose author never read this file both redact everything; only the table can
// tell them apart, and only something that knows every action there is can
// tell whether the table is complete.
func Listed(action string) bool {
	_, ok := allowlist[action]
	return ok
}

// ListedActions returns every action the table names, in no particular order.
//
// It is for the guard described on Listed, in the other direction: an entry for
// an action nothing emits is a decision about a field that no longer exists.
func ListedActions() []string {
	out := make([]string, 0, len(allowlist))
	for action := range allowlist {
		out = append(out, action)
	}
	return out
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
	permitted := make([]leafRule, 0, len(allowlist[action]))
	for _, f := range allowlist[action] {
		rule := leafRule{}
		if strings.HasSuffix(f, listSuffix) {
			rule.list = true
			f = strings.TrimSuffix(f, listSuffix)
		}
		rule.path = strings.Split(f, ".")
		permitted = append(permitted, rule)
	}

	out := make(map[string]any, len(raw))
	for k, v := range raw {
		out[k] = redactValue([]string{k}, permitted, v)
	}
	return out
}

// leafRule is one allowlist entry, parsed.
//
// list records that the entry was written with a `[]` suffix, which is the
// author saying this field holds a list of identifiers rather than one. It is
// spelled out per entry rather than inferred from what arrives, because
// "username" is a scalar field and a caller sending a list there is a caller
// sending a shape that field does not have -- which is exactly the smuggling
// the allowlist is for.
type leafRule struct {
	path []string
	list bool
}

// listSuffix marks an allowlist entry as naming a list of scalars.
const listSuffix = "[]"

// redactValue decides one node of the parameter tree.
//
// There is no branch that returns an unrecognised value unchanged. Everything
// either matches an allowlisted leaf path and is a scalar, or is a container on
// the way to one, or becomes the marker.
func redactValue(segments []string, permitted []leafRule, v any) any {
	if leaf, ok := permittedLeaf(segments, permitted); ok {
		if s, ok := permittedValue(v, leaf.list); ok {
			return s
		}
		// An allowlist entry names a leaf. A caller that sends an object where
		// a string was expected would otherwise smuggle arbitrary content past
		// the list under a permitted name -- and walking it would publish its
		// shape, which for a secrets bundle is itself a map of where to look.
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

// permittedLeaf returns the rule for an allowlisted leaf named exactly.
func permittedLeaf(segments []string, permitted []leafRule) (leafRule, bool) {
	for _, p := range permitted {
		if slices.Equal(p.path, segments) {
			return p, true
		}
	}
	return leafRule{}, false
}

// leadsToPermitted reports whether any allowlisted path continues below here.
func leadsToPermitted(segments []string, permitted []leafRule) bool {
	for _, p := range permitted {
		if len(p.path) > len(segments) && slices.Equal(p.path[:len(segments)], segments) {
			return true
		}
	}
	return false
}

// permittedValue accepts what a leaf may carry.
//
// list is the entry's own `[]` marking. A field marked as a list accepts a
// list of scalars, and the reason it exists at all is a record that is
// otherwise incomplete: `config.apply` names its patches by id, and a run
// whose ids are `<redacted>` is a record that says a configuration was applied
// and cannot say which one. It is not an exemption from the rule -- every
// element goes through this function itself, an over-long list is refused, and
// a list holding an object or another list is refused whole.
//
// A field not marked as a list refuses one, exactly as before: "username" is a
// scalar field, and a caller sending a list there is sending a shape that
// field does not have.
//
// An object under a permitted name is always the marker, marked or not:
// walking it would publish its shape, and the shape of a secrets bundle is
// itself a map of where to look.
func permittedValue(v any, list bool) (any, bool) {
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
	case []any:
		if !list || len(t) > maxParamItems {
			return nil, false
		}
		out := make([]any, 0, len(t))
		for _, item := range t {
			// Never `list` again: a list of lists is a structure, and this
			// permits one list of scalars rather than a tree of them.
			s, ok := permittedValue(item, false)
			if !ok {
				// One element that is not a scalar refuses the whole list
				// rather than a list with a marker in it: a partially redacted
				// list reads as a complete one.
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
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
