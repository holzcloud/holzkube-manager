package imagefactory_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube/internal/imagefactory"
)

// mustID fails the test rather than returning an error, so the property tests
// below read as statements about ids.
func mustID(t *testing.T, s imagefactory.Schematic) string {
	t.Helper()
	id, err := s.ID()
	if err != nil {
		t.Fatalf("ID: %v", err)
	}
	return id
}

// TestSchematicIDIgnoresTheOrderFieldsWereSet: the canonical form is emitted in
// a fixed order, so two schematics that describe the same thing hash the same
// however they were built up.
func TestSchematicIDIgnoresTheOrderFieldsWereSet(t *testing.T) {
	var a imagefactory.Schematic
	a.Customization.ExtraKernelArgs = []string{"console=ttyS0"}
	a.Customization.SystemExtensions.OfficialExtensions = []string{"siderolabs/intel-ucode"}
	a.Owner = "holzkube"
	a.Overlay = imagefactory.Overlay{Image: "siderolabs/sbc-rockchip", Name: "turingrk1"}

	var b imagefactory.Schematic
	b.Overlay = imagefactory.Overlay{Image: "siderolabs/sbc-rockchip", Name: "turingrk1"}
	b.Owner = "holzkube"
	b.Customization.SystemExtensions.OfficialExtensions = []string{"siderolabs/intel-ucode"}
	b.Customization.ExtraKernelArgs = []string{"console=ttyS0"}

	if mustID(t, a) != mustID(t, b) {
		t.Errorf("two identical schematics built in different orders hashed differently:\n%s\n%s",
			mustID(t, a), mustID(t, b))
	}
}

// TestSchematicIDChangesWithAKernelArg: the id is a fingerprint of the whole
// document, so a customization that differs at all is a different schematic.
func TestSchematicIDChangesWithAKernelArg(t *testing.T) {
	base := goodSchematic()
	id := mustID(t, base)

	changed := goodSchematic()
	changed.Customization.ExtraKernelArgs = append(changed.Customization.ExtraKernelArgs, "quiet")
	if mustID(t, changed) == id {
		t.Error("adding a kernel argument did not change the id")
	}

	removed := goodSchematic()
	removed.Customization.ExtraKernelArgs = nil
	if mustID(t, removed) == id {
		t.Error("removing every kernel argument did not change the id")
	}
}

// TestSchematicIDIsSensitiveToExtensionOrder pins a fact that is easy to assume
// away: the Factory preserves the extension list as given and hashes it in that
// order, so the list is a list and not a set. A caller that sorts the operator's
// selection is composing a different schematic than the one it was handed.
func TestSchematicIDIsSensitiveToExtensionOrder(t *testing.T) {
	forward := imagefactory.Schematic{Customization: imagefactory.Customization{
		SystemExtensions: imagefactory.SystemExtensions{
			OfficialExtensions: []string{"siderolabs/intel-ucode", "siderolabs/iscsi-tools"},
		},
	}}
	reversed := imagefactory.Schematic{Customization: imagefactory.Customization{
		SystemExtensions: imagefactory.SystemExtensions{
			OfficialExtensions: []string{"siderolabs/iscsi-tools", "siderolabs/intel-ucode"},
		},
	}}

	if mustID(t, forward) == mustID(t, reversed) {
		t.Error("reordering the extension list did not change the id; the list is not a set upstream")
	}
}

// TestSchematicIDOfTheEmptyCustomization pins the well-known id. The zero
// schematic is not the hash of an empty document -- customization is always
// emitted, as an empty mapping -- and getting that wrong is the single most
// likely way for this serialiser to be subtly off.
func TestSchematicIDOfTheEmptyCustomization(t *testing.T) {
	if got := mustID(t, imagefactory.Schematic{}); got != emptySchematicID {
		t.Errorf("empty schematic id = %s, want %s", got, emptySchematicID)
	}

	// A false SecureBoot flag is indistinguishable from an absent block
	// upstream, so it must not perturb the well-known id.
	var s imagefactory.Schematic
	s.Customization.SecureBoot.IncludeWellKnownCertificates = false
	s.Customization.ExtraKernelArgs = []string{}
	s.Customization.Meta = []imagefactory.MetaValue{}
	if got := mustID(t, s); got != emptySchematicID {
		t.Errorf("a schematic with only empty values hashed to %s, want the well-known %s", got, emptySchematicID)
	}
}

// TestSchematicIDSurvivesARoundTrip: a schematic that was persisted and read
// back must still name the same image. If it did not, a stored schematic id
// would stop matching the schematic stored beside it.
func TestSchematicIDSurvivesARoundTrip(t *testing.T) {
	original := imagefactory.Schematic{
		Owner:   "holzkube",
		Overlay: imagefactory.Overlay{Image: "siderolabs/sbc-rockchip", Name: "turingrk1"},
		Customization: imagefactory.Customization{
			ExtraKernelArgs:  []string{"console=ttyS0", "--x=1"},
			Meta:             []imagefactory.MetaValue{{Key: 10, Value: "hello"}, {Key: 0, Value: ""}},
			SystemExtensions: imagefactory.SystemExtensions{OfficialExtensions: []string{"siderolabs/intel-ucode"}},
			SecureBoot:       imagefactory.SecureBoot{IncludeWellKnownCertificates: true},
		},
	}

	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var restored imagefactory.Schematic
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if mustID(t, restored) != mustID(t, original) {
		t.Errorf("id changed across a round trip: %s -> %s", mustID(t, original), mustID(t, restored))
	}

	docA, _ := original.Canonical()
	docB, _ := restored.Canonical()
	if string(docA) != string(docB) {
		t.Errorf("canonical document changed across a round trip:\n%q\n%q", docA, docB)
	}
}

// TestSchematicIDRefusesRatherThanGuesses: a value this serialiser was not
// pinned against produces a refusal, not an id. A wrong id is worse than no id
// -- FACT-06 exists so holzkube can recognise a schematic without a round trip,
// and a value that silently disagrees with the Factory turns that into a lie.
func TestSchematicIDRefusesRatherThanGuesses(t *testing.T) {
	for name, arg := range map[string]string{
		"newline":         "console=ttyS0\nquiet",
		"tab":             "console=\tttyS0",
		"null byte":       "console\x00",
		"delete":          "console\x7f",
		"invalid utf-8":   "console=\xff\xfe",
		"carriage return": "console\r",
	} {
		t.Run(name, func(t *testing.T) {
			s := imagefactory.Schematic{Customization: imagefactory.Customization{
				ExtraKernelArgs: []string{arg},
			}}
			id, err := s.ID()
			if !errors.Is(err, imagefactory.ErrSchematicNotRepresentable) {
				t.Fatalf("ID() = %q, %v; want ErrSchematicNotRepresentable", id, err)
			}
			if id != "" {
				t.Errorf("a refused schematic still produced an id: %q", id)
			}

			// The sentinel says that something was refused. The typed error
			// says what and where, which is what a 400 needs in order to name a
			// field instead of blaming the Factory (G-02-6).
			var typed *imagefactory.NotRepresentableError
			if !errors.As(err, &typed) {
				t.Fatalf("ID() error is not a *NotRepresentableError: %v", err)
			}
			if typed.Path != "customization.extraKernelArgs" {
				t.Errorf("Path = %q; want customization.extraKernelArgs", typed.Path)
			}
			if typed.Index != 0 {
				t.Errorf("Index = %d; want 0", typed.Index)
			}
		})
	}
}

// TestSchematicRefusalNamesTheFieldAndEntry pins the document path each refusal
// reports. The path is what the HTTP layer maps onto a request field name, so a
// path that drifts turns a 400 naming kernel_args back into the 502 blaming the
// Image Factory that G-02-6 recorded.
func TestSchematicRefusalNamesTheFieldAndEntry(t *testing.T) {
	const bad = "console\x07"

	cases := map[string]struct {
		schematic imagefactory.Schematic
		wantPath  string
		wantIndex int
	}{
		"kernel argument": {
			schematic: imagefactory.Schematic{Customization: imagefactory.Customization{
				ExtraKernelArgs: []string{"console=ttyS0", "quiet", bad},
			}},
			wantPath:  "customization.extraKernelArgs",
			wantIndex: 2,
		},
		"meta value": {
			schematic: imagefactory.Schematic{Customization: imagefactory.Customization{
				Meta: []imagefactory.MetaValue{
					{Key: 10, Value: "fine"},
					{Key: 11, Value: bad},
				},
			}},
			wantPath:  "customization.meta.value",
			wantIndex: 1,
		},
		"official extension": {
			schematic: imagefactory.Schematic{Customization: imagefactory.Customization{
				SystemExtensions: imagefactory.SystemExtensions{
					OfficialExtensions: []string{"siderolabs/intel-ucode", bad},
				},
			}},
			wantPath:  "customization.systemExtensions.officialExtensions",
			wantIndex: 1,
		},
		"owner": {
			schematic: imagefactory.Schematic{Owner: bad},
			wantPath:  "owner",
			// A scalar field has no position within a sequence.
			wantIndex: -1,
		},
		"overlay image": {
			schematic: imagefactory.Schematic{
				Overlay: imagefactory.Overlay{Image: bad, Name: "turingrk1"},
			},
			wantPath:  "overlay.image",
			wantIndex: -1,
		},
		"overlay name": {
			schematic: imagefactory.Schematic{
				Overlay: imagefactory.Overlay{Image: "siderolabs/sbc-rockchip", Name: bad},
			},
			wantPath:  "overlay.name",
			wantIndex: -1,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := tc.schematic.ID()
			var typed *imagefactory.NotRepresentableError
			if !errors.As(err, &typed) {
				t.Fatalf("ID() error is not a *NotRepresentableError: %v", err)
			}
			if !errors.Is(err, imagefactory.ErrSchematicNotRepresentable) {
				t.Error("the typed error no longer satisfies errors.Is against the sentinel")
			}
			if typed.Path != tc.wantPath {
				t.Errorf("Path = %q; want %q", typed.Path, tc.wantPath)
			}
			if typed.Index != tc.wantIndex {
				t.Errorf("Index = %d; want %d", typed.Index, tc.wantIndex)
			}
			if typed.Reason == "" {
				t.Error("Reason is empty; the operator is told nothing about what was wrong")
			}
		})
	}
}

// TestSchematicRefusalReportsTheFirstBadValueInDocumentOrder: two bad values in
// one schematic report the first, as they always have. The collection is
// first-failure-wins and this plan did not change it.
func TestSchematicRefusalReportsTheFirstBadValueInDocumentOrder(t *testing.T) {
	s := imagefactory.Schematic{Customization: imagefactory.Customization{
		ExtraKernelArgs: []string{"ok", "first\x07", "second\x08"},
	}}
	_, err := s.ID()
	var typed *imagefactory.NotRepresentableError
	if !errors.As(err, &typed) {
		t.Fatalf("ID() error is not a *NotRepresentableError: %v", err)
	}
	if typed.Index != 1 {
		t.Errorf("Index = %d; want 1 (the first bad entry in document order)", typed.Index)
	}
}

// TestSchematicRefusalDoesNotEchoTheValue is a constraint, not a nicety
// (T-02-64). This message becomes a problem+json body: it is rendered in a
// browser, may be logged by a proxy, and outlives the form it came from. Kernel
// arguments can carry secrets -- that is precisely why the Factory refuses to
// enumerate schematics at all.
func TestSchematicRefusalDoesNotEchoTheValue(t *testing.T) {
	const secret = "talos.config=https://example.invalid/?token=s3cr3t\x07"

	for name, s := range map[string]imagefactory.Schematic{
		"kernel argument": {Customization: imagefactory.Customization{
			ExtraKernelArgs: []string{secret},
		}},
		"meta value": {Customization: imagefactory.Customization{
			Meta: []imagefactory.MetaValue{{Key: 10, Value: secret}},
		}},
		"owner": {Owner: secret},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := s.ID()
			if err == nil {
				t.Fatal("ID() accepted a value carrying a control character")
			}
			if strings.Contains(err.Error(), "s3cr3t") {
				t.Errorf("the refusal echoed the offending value: %q", err.Error())
			}
			var typed *imagefactory.NotRepresentableError
			if !errors.As(err, &typed) {
				t.Fatalf("ID() error is not a *NotRepresentableError: %v", err)
			}
			if strings.Contains(typed.Reason, "s3cr3t") {
				t.Errorf("Reason echoed the offending value: %q", typed.Reason)
			}
		})
	}
}

// TestSchematicCanonicalQuotesWhatMustBeQuoted pins the scalar styles against
// documents the live Factory produced. These were captured, not derived: the
// upstream emitter double-quotes anything that would read back as a non-string
// and single-quotes anything plain would misparse, and a serialiser that gets
// either wrong produces a different hash for the same schematic.
func TestSchematicCanonicalQuotesWhatMustBeQuoted(t *testing.T) {
	cases := []struct {
		arg  string
		want string
	}{
		{"console=ttyS0", "console=ttyS0"}, // plain
		{"--x=1", "--x=1"},                 // a leading dash is only an indicator before a blank
		{"a#b", "a#b"},                     // a hash only starts a comment after a blank
		{`quote'single`, `quote'single`},   // a quote inside a scalar is not an indicator
		{"a: b", `'a: b'`},                 // a colon-space would end the scalar
		{"b #c", `'b #c'`},                 // a blank-hash would start a comment
		{"key:", `'key:'`},                 // a trailing colon would end the scalar
		{"*star", `'*star'`},               // a leading star is an alias
		{"-", `'-'`},                       // a bare dash is a sequence entry
		{"tail ", `'tail '`},               // a trailing blank is unrecoverable plain
		{"yes", `"yes"`},                   // a YAML 1.1 boolean
		{"123", `"123"`},                   // an integer
		{"1:30", `"1:30"`},                 // a sexagesimal number
		{"2020-01-02", `"2020-01-02"`},     // a timestamp
		{".inf", `".inf"`},                 // a float
		{"0o17", `"0o17"`},                 // an octal
	}

	for _, tc := range cases {
		t.Run(tc.arg, func(t *testing.T) {
			s := imagefactory.Schematic{Customization: imagefactory.Customization{
				ExtraKernelArgs: []string{tc.arg},
			}}
			doc, err := s.Canonical()
			if err != nil {
				t.Fatalf("Canonical: %v", err)
			}
			want := "customization:\n    extraKernelArgs:\n        - " + tc.want + "\n"
			if string(doc) != want {
				t.Errorf("canonical document\n got: %q\nwant: %q", doc, want)
			}
		})
	}
}

// TestSchematicCanonicalIndentsFourSpaces guards the shape the id depends on
// most bluntly: two-space indentation would hash differently for every
// schematic in the system.
func TestSchematicCanonicalIndentsFourSpaces(t *testing.T) {
	doc, err := goodSchematic().Canonical()
	if err != nil {
		t.Fatalf("Canonical: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSuffix(string(doc), "\n"), "\n") {
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if indent%4 != 0 {
			t.Errorf("line %q is indented %d spaces, which is not a multiple of four", line, indent)
		}
	}
	if !strings.HasSuffix(string(doc), "\n") {
		t.Error("the canonical document does not end in a newline")
	}
}
