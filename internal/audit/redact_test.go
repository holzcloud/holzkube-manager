package audit

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestRedactKeepsOnlyAllowlistedFields is D-14 in one case: the username of a
// login is useful forever, the password must never reach the file.
func TestRedactKeepsOnlyAllowlistedFields(t *testing.T) {
	got := Params("auth.login", map[string]any{
		"username": "holz",
		"password": "correct-horse-battery-staple",
	})

	if got["username"] != "holz" {
		t.Errorf("username = %v, want it passed through", got["username"])
	}
	if got["password"] != RedactedMarker {
		t.Errorf("password = %v, want %q", got["password"], RedactedMarker)
	}
	assertNoSecret(t, got, "correct-horse-battery-staple")
}

// TestRedactReplacesAFieldNobodyListed is the property a denylist cannot have.
// The field name here is invented and appears on no list anywhere; it must
// still come out redacted, because the function decides by what is permitted,
// not by what is known to be dangerous.
func TestRedactReplacesAFieldNobodyListed(t *testing.T) {
	got := Params("auth.login", map[string]any{
		"username":      "holz",
		"blorptangle42": "hunter2",
	})

	if _, ok := got["blorptangle42"]; !ok {
		t.Error("the unlisted field vanished entirely; the record should show that it was sent")
	}
	if got["blorptangle42"] != RedactedMarker {
		t.Errorf("blorptangle42 = %v, want %q", got["blorptangle42"], RedactedMarker)
	}
	assertNoSecret(t, got, "hunter2")
}

// TestRedactRedactsEverythingForAnUnknownAction covers a route added in a later
// phase whose author forgot the allowlist. The failure mode is a useless record,
// never a leaked one.
func TestRedactRedactsEverythingForAnUnknownAction(t *testing.T) {
	got := Params("node.reset", map[string]any{
		"machine_id": "abc",
		"wipe":       true,
	})

	for k, v := range got {
		if v != RedactedMarker {
			t.Errorf("%s = %v, want %q for an action with no allowlist entry", k, v, RedactedMarker)
		}
	}
}

// TestRedactReplacesAnEntireUnknownBranch checks the recursion: a nested object
// that no allowlisted path reaches is replaced whole, keys included, rather than
// walked and half-copied.
func TestRedactReplacesAnEntireUnknownBranch(t *testing.T) {
	got := Params("setup.create", map[string]any{
		"username": "holz",
		"bundle": map[string]any{
			"ca": map[string]any{"key": "-----BEGIN EC PRIVATE KEY-----"},
		},
	})

	if got["bundle"] != RedactedMarker {
		t.Errorf("bundle = %#v, want the whole branch replaced by %q", got["bundle"], RedactedMarker)
	}
	assertNoSecret(t, got, "BEGIN EC PRIVATE KEY")
	assertNoSecret(t, got, "\"ca\"")
}

// TestRedactRefusesAStructureUnderAnAllowlistedPath is the fail-closed edge: an
// allowlist entry names a leaf. If a caller sends an object where a string was
// expected, passing it through would smuggle arbitrary content past the list.
func TestRedactRefusesAStructureUnderAnAllowlistedPath(t *testing.T) {
	got := Params("auth.login", map[string]any{
		"username": map[string]any{"nested": "surprise"},
	})

	if got["username"] != RedactedMarker {
		t.Errorf("username = %#v, want %q", got["username"], RedactedMarker)
	}
	assertNoSecret(t, got, "surprise")
}

// TestRedactRefusesArrays holds the same line for lists.
func TestRedactRefusesArrays(t *testing.T) {
	got := Params("auth.login", map[string]any{
		"username": []any{"holz", "smuggled"},
	})

	if got["username"] != RedactedMarker {
		t.Errorf("username = %#v, want %q", got["username"], RedactedMarker)
	}
	assertNoSecret(t, got, "smuggled")
}

// TestRedactCapsLongValues stops a caller from writing a megabyte into an
// archive that is kept forever (D-16) by putting it in an allowlisted field.
func TestRedactCapsLongValues(t *testing.T) {
	long := strings.Repeat("a", maxParamLen*3)
	got := Params("auth.login", map[string]any{"username": long})

	s, ok := got["username"].(string)
	if !ok {
		t.Fatalf("username = %#v, want a string", got["username"])
	}
	if len(s) > maxParamLen+len(truncationMarker) {
		t.Errorf("username kept %d bytes, want it capped near %d", len(s), maxParamLen)
	}
	if !strings.HasSuffix(s, truncationMarker) {
		t.Errorf("a truncated value must say so, got %q", s[max(0, len(s)-16):])
	}
}

// TestRedactHandlesNothing covers the empty and nil cases, which is what a
// request with no body produces.
func TestRedactHandlesNothing(t *testing.T) {
	if got := Params("auth.logout", nil); len(got) != 0 {
		t.Errorf("Params(nil) = %v, want an empty map", got)
	}
	if got := Params("auth.logout", map[string]any{}); got == nil {
		t.Error("Params returned a nil map; the record format expects an object")
	}
}

// TestRedactOutputIsCanonicalizable makes sure a redacted map can actually be
// hashed. A value that cannot be canonicalized would break the chain at the
// moment it is written, which is the worst possible time to find out.
func TestRedactOutputIsCanonicalizable(t *testing.T) {
	raw := map[string]any{
		"username": "holz",
		"password": "secret",
		"nested":   map[string]any{"a": []any{1, 2, 3}},
	}
	rec := Record{Seq: 1, Action: "auth.login", Params: Params("auth.login", raw), Outcome: OutcomeAttempt}
	if _, err := rec.CanonicalJSON(); err != nil {
		t.Fatalf("redacted params are not canonicalizable: %v", err)
	}
}

// TestRedactAllowlistCarriesNoCredentialFields is the structural assertion
// behind the prohibition: every entry names something that may pass, and no
// entry may name a credential.
func TestRedactAllowlistCarriesNoCredentialFields(t *testing.T) {
	for action, fields := range allowlist {
		for _, f := range fields {
			if strings.Contains(strings.ToLower(f), "password") ||
				strings.Contains(strings.ToLower(f), "secret") ||
				strings.Contains(strings.ToLower(f), "token") ||
				strings.Contains(strings.ToLower(f), "key") {
				t.Errorf("%s allows %q; the allowlist must not carry credential fields", action, f)
			}
		}
	}
}

func assertNoSecret(t *testing.T, params map[string]any, secret string) {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	if strings.Contains(string(raw), secret) {
		t.Errorf("redacted params still contain %q: %s", secret, raw)
	}
}

// TestRedactDoesNotConflateADottedKeyWithANestedPath covers the one mechanism
// the package describes as "failing the other way" by construction.
//
// permitted is keyed by dotted path, and the lookup used to be a direct map hit
// on the raw JSON key. An entry "user.name", meant to permit
// {"user":{"name":...}}, therefore also permitted a body sending the flat key
// {"user.name": "..."}. JSON object keys may contain dots and readBody decodes
// whatever arrives, so the two namespaces were conflated -- a latent hole in an
// archive with no deletion path.
func TestRedactDoesNotConflateADottedKeyWithANestedPath(t *testing.T) {
	const action = "test.nested"
	allowlist[action] = []string{"user.name"}
	t.Cleanup(func() { delete(allowlist, action) })

	t.Run("the nested path it was written for is permitted", func(t *testing.T) {
		out := Params(action, map[string]any{
			"user": map[string]any{"name": "holz", "password": "hunter2"},
		})
		user, ok := out["user"].(map[string]any)
		if !ok {
			t.Fatalf("user = %#v, want the object to be walked", out["user"])
		}
		if user["name"] != "holz" {
			t.Errorf("user.name = %#v, want it permitted in clear", user["name"])
		}
		if user["password"] != RedactedMarker {
			t.Errorf("user.password = %#v, want %q", user["password"], RedactedMarker)
		}
	})

	t.Run("a flat key that merely looks like it is not", func(t *testing.T) {
		out := Params(action, map[string]any{"user.name": "smuggled"})
		if out["user.name"] != RedactedMarker {
			t.Errorf(`params["user.name"] = %#v, want %q: a dot in a JSON key is a `+
				`character, not a descent`, out["user.name"], RedactedMarker)
		}
	})
}

// TestRedactKeepsSiblingSegmentsApart guards the recursion itself: the path is
// built by appending, and sharing one backing array across an object's keys
// would let siblings overwrite each other's segment.
func TestRedactKeepsSiblingSegmentsApart(t *testing.T) {
	const action = "test.siblings"
	allowlist[action] = []string{"outer.keep"}
	t.Cleanup(func() { delete(allowlist, action) })

	out := Params(action, map[string]any{
		"outer": map[string]any{
			"keep": "visible",
			"a":    "secret-a",
			"b":    "secret-b",
			"c":    "secret-c",
		},
	})
	outer, ok := out["outer"].(map[string]any)
	if !ok {
		t.Fatalf("outer = %#v, want the object to be walked", out["outer"])
	}
	if outer["keep"] != "visible" {
		t.Errorf("outer.keep = %#v, want it permitted regardless of sibling order", outer["keep"])
	}
	for _, k := range []string{"a", "b", "c"} {
		if outer[k] != RedactedMarker {
			t.Errorf("outer.%s = %#v, want %q", k, outer[k], RedactedMarker)
		}
	}
}

// TestRedactSchematicCreateKeepsOnlyTheNameAndTheVersion is the live half of
// plan 02-04's first prohibition, and it is the one entry in this table whose
// mistake would be permanent.
//
// The Image Factory itself refuses to enumerate schematics on the grounds that
// kernel arguments may carry secrets, and holzkube's archive is append-only and
// kept forever (D-16) with no deletion path. So a kernel argument written in
// clear here is written in clear for good: there is no migration, no redaction
// pass and no delete that could take it back without breaking the hash chain.
// Everything except the operator's own label and the Talos version is redacted.
func TestRedactSchematicCreateKeepsOnlyTheNameAndTheVersion(t *testing.T) {
	const secretArg = "talos.secret=hunter2"

	got := Params("schematic.create", map[string]any{
		"name":          "workers with intel microcode",
		"talos_version": "v1.13.9",
		"extensions":    []any{"siderolabs/intel-ucode"},
		"kernel_args":   []any{secretArg},
		"meta":          []any{map[string]any{"key": 10, "value": "also secret"}},
		"canonical":     "customization: {}\n",
		"cluster":       "homelab",
		"arch":          "amd64",
	})

	if got["name"] != "workers with intel microcode" {
		t.Errorf("name = %v, want it passed through", got["name"])
	}
	if got["talos_version"] != "v1.13.9" {
		t.Errorf("talos_version = %v, want it passed through", got["talos_version"])
	}
	for _, field := range []string{"extensions", "kernel_args", "meta", "canonical", "cluster", "arch"} {
		if got[field] != RedactedMarker {
			t.Errorf("%s = %v, want %q", field, got[field], RedactedMarker)
		}
	}
	assertNoSecret(t, got, secretArg)
	assertNoSecret(t, got, "also secret")
}

// TestRedactSchematicDeleteIsListedWithNothingPermitted keeps the table honest
// about the full set of mutations rather than leaving one to the default. A
// deletion carries an id in its path, not in its body, so there is nothing here
// that is worth writing in clear.
func TestRedactSchematicDeleteIsListedWithNothingPermitted(t *testing.T) {
	fields, listed := allowlist["schematic.delete"]
	if !listed {
		t.Fatal("schematic.delete has no allowlist entry; the table must show every mutation")
	}
	if len(fields) != 0 {
		t.Errorf("schematic.delete permits %v, want nothing permitted", fields)
	}
}
