package imagefactory_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/imagefactory"
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
	a.Owner = "holzkube-manager"
	a.Overlay = imagefactory.Overlay{Image: "siderolabs/sbc-rockchip", Name: "turingrk1"}

	var b imagefactory.Schematic
	b.Overlay = imagefactory.Overlay{Image: "siderolabs/sbc-rockchip", Name: "turingrk1"}
	b.Owner = "holzkube-manager"
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
		Owner:   "holzkube-manager",
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
// -- FACT-06 exists so holzkube-manager can recognise a schematic without a round trip,
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

// The classes TestLiveCanonical measured to diverge from factory.talos.dev.
//
// This is not a transcription of a list somebody wrote down. Every entry names
// the rows of the recorded three-way table that proved it, and a class the
// table did not prove is not here -- see 02-14-SUMMARY.md for the table itself
// and for the classes it left unmeasured. The negative control below is the
// other half of the same measurement: without it this table could only show
// that refusing everything is safe.
var divergingClasses = []struct {
	class string
	// reason is the substring the refusal must carry. It names the character
	// class; it never carries the value (T-02-64).
	reason string
	// cps are codepoints the table classified DIVERGES.
	cps []rune
}{
	{
		// U+0080, U+0081, U+008D, U+0094 and U+009F each DIVERGES in all six
		// position/style variants on both document paths: the Factory escapes
		// them into "\x80console=ttyS0", so the value survives and only the id
		// moves. U+0085 is worse and is the reason this class is refused rather
		// than escaped -- it DIVERGES as "would not parse" in a plain scalar
		// and as "silently altered" in a quoted one, where the Factory folded
		// it into a space, and it was eaten outright at the end of a plain
		// scalar. That last row is the false 409 G-02-16 recorded.
		class:  "C1 control (U+0080-U+009F)",
		reason: "control character",
		cps:    []rune{0x0080, 0x0081, 0x0085, 0x008D, 0x0094, 0x009F},
	},
	{
		// Five of six variants DIVERGES for each, on both paths: "would not
		// parse" when plain, and "silently altered" when quoted, where the
		// Factory folded the break and inserted ten spaces. U+2028 is the
		// codepoint that produced the 502 UAT test 5(b) exists to exclude.
		//
		// The sixth variant -- trailing, inside an already-quoted scalar --
		// AGREES. It is refused anyway: whether a scalar can be represented is
		// a property of the scalar, and making it depend on where in the
		// surrounding text the codepoint happens to sit would mean the same
		// character is carriable or not according to its neighbours.
		class:  "YAML line break above the C1 range",
		reason: "line separator",
		cps:    []rune{0x2028, 0x2029},
	},
	{
		// All four measured variants DIVERGES on both paths, re-normalised:
		// the Factory escapes the BOM and every character after it. U+FEFF sits
		// inside YAML's printable range and is excluded from it by name, which
		// is why no range test catches it and it needs its own clause.
		class:  "byte order mark",
		reason: "byte order mark",
		cps:    []rune{0xFEFF},
	},
	{
		// U+FFFE and U+FFFF DIVERGES as "would not parse"; U+1F600 and
		// U+10FFFF DIVERGES re-normalised, escaped to "\U0001F600". U+FFFD
		// AGREES, which is what fixes this boundary exactly rather than
		// approximately -- and U+FDD0 AGREES too, so "non-character" is not the
		// rule: a non-character below the ceiling round-trips.
		class:  "above the printable ceiling U+FFFD",
		reason: "above U+FFFD",
		cps:    []rune{0xFFFE, 0xFFFF, 0x1F600, 0x10FFFF},
	},
}

// TestSchematicIDRefusesEveryMeasuredDivergence is the refusal half of
// G-02-16. Each subtest is named for the rule rather than for a codepoint,
// because the rule is what is in the code.
func TestSchematicIDRefusesEveryMeasuredDivergence(t *testing.T) {
	for _, class := range divergingClasses {
		t.Run(class.class, func(t *testing.T) {
			for _, cp := range class.cps {
				t.Run(fmt.Sprintf("%U", cp), func(t *testing.T) {
					s := imagefactory.Schematic{Customization: imagefactory.Customization{
						ExtraKernelArgs: []string{"console=ttyS0", "console=" + string(cp) + "ttyS0"},
					}}
					id, err := s.ID()
					if id != "" {
						t.Errorf("a refused schematic still produced an id: %q", id)
					}
					var typed *imagefactory.NotRepresentableError
					if !errors.As(err, &typed) {
						t.Fatalf("ID() = %q, %v; want a *NotRepresentableError", id, err)
					}
					if !errors.Is(err, imagefactory.ErrSchematicNotRepresentable) {
						t.Error("the typed error no longer satisfies errors.Is against the sentinel")
					}
					if typed.Path != "customization.extraKernelArgs" {
						t.Errorf("Path = %q; want customization.extraKernelArgs", typed.Path)
					}
					if typed.Index != 1 {
						t.Errorf("Index = %d; want 1 -- the first bad entry in document order", typed.Index)
					}
					if !strings.Contains(typed.Reason, class.reason) {
						t.Errorf("Reason = %q; want it to name the class %q", typed.Reason, class.reason)
					}
					// The reason names the class and the codepoint, never the
					// value. A kernel argument can carry a secret (T-02-64).
					if strings.Contains(typed.Reason, "ttyS0") {
						t.Errorf("Reason echoed the offending value: %q", typed.Reason)
					}
					if !strings.Contains(typed.Reason, fmt.Sprintf("%U", cp)) {
						t.Errorf("Reason = %q; want the %%U rendering of the codepoint, which is what makes it actionable", typed.Reason)
					}
				})
			}
		})
	}
}

// TestSchematicIDRefusesAMeasuredDivergenceInAMetaValue: the differential swept
// both document paths and found the same verdicts on each, so the refusal has
// to name the path it happened in. A meta value reported as a kernel argument
// sends the operator to the wrong input.
func TestSchematicIDRefusesAMeasuredDivergenceInAMetaValue(t *testing.T) {
	for _, class := range divergingClasses {
		t.Run(class.class, func(t *testing.T) {
			s := imagefactory.Schematic{Customization: imagefactory.Customization{
				Meta: []imagefactory.MetaValue{
					{Key: 10, Value: "fine"},
					{Key: 11, Value: "v" + string(class.cps[0])},
				},
			}}
			_, err := s.ID()
			var typed *imagefactory.NotRepresentableError
			if !errors.As(err, &typed) {
				t.Fatalf("ID() = %v; want a *NotRepresentableError", err)
			}
			if typed.Path != "customization.meta.value" {
				t.Errorf("Path = %q; want customization.meta.value", typed.Path)
			}
			if typed.Index != 1 {
				t.Errorf("Index = %d; want 1", typed.Index)
			}
		})
	}
}

// TestSchematicIDStillAcceptsWhatTheFactoryProvedItCarries is the negative
// control, and it is the half that keeps the widening honest.
//
// Every codepoint here was measured AGREES in both quoting styles on both
// document paths: the Factory's own canonical document carried holzkube-manager's line
// back byte for byte. So each of these ids is the hash of a rendering upstream
// confirmed, and asserting the literal rather than merely "no error" is what
// makes this a stability test -- a test that only checked for a nil error would
// pass while the rendering moved underneath it, which is exactly the failure
// WINDOWS entry 29 warns about.
func TestSchematicIDStillAcceptsWhatTheFactoryProvedItCarries(t *testing.T) {
	for _, tc := range []struct {
		cp   rune
		want string
	}{
		{0x00A0, "9ec2f6d3dc3a7837dc31f10c0709374a99f19fdd9ac6b80d8238d8111b166ba3"}, // no-break space
		{0x00E4, "5c2455ed2d6f954bad82dda2c792e6bec1c589dc8235392822ff6c6d8e90470e"}, // a-umlaut
		{0x200B, "291f583fa58bc85ad8223c5bc255fb4ad32fbfee926316b2404cfaaf6cd331de"}, // zero-width space
		{0x202E, "27abc4abc09cd3ab81f67ed83d8bb24b1b2d0d41102a83bea599cd3e43678cfc"}, // right-to-left override
		{0x4E2D, "2c613e9a60fb0072f63b315642f4df8a0067006315a4be897e7a0718b7181480"}, // CJK
		{0xD7FF, "42832d54f32124aacc61c1f50531bbe98c889e167d490062999d2f18b8f1ceb6"}, // last before the surrogates
		{0xE000, "26f35c35573dc70dce9b35f745f9e5118f72d25511a130859d5e7178863230a8"}, // first after the surrogates
		{0xFDD0, "33d13000bbc8c52266b6212a889cffe3dcdf3ca4abf86e24d6a001dc331ef191"}, // a non-character that round-trips
		{0xFFFD, "866118490b38801b7e08a0f4f4ae1a4926d1b74b2481140b6a306281c7e03591"}, // the printable ceiling itself
	} {
		t.Run(fmt.Sprintf("%U", tc.cp), func(t *testing.T) {
			s := imagefactory.Schematic{Customization: imagefactory.Customization{
				ExtraKernelArgs: []string{"console=" + string(tc.cp) + "ttyS0"},
			}}
			got, err := s.ID()
			if err != nil {
				t.Fatalf("ID() refused a codepoint the Factory proved it carries: %v", err)
			}
			if got != tc.want {
				t.Errorf("id = %s, want %s -- the rendering of an accepted scalar moved", got, tc.want)
			}
		})
	}
}

// TestTheWellKnownIDsDidNotMove restates the two anchors here, beside the
// widening, so that the one prohibition this change had to respect is asserted
// in the file that made the change. Both are also pinned in tracer_test.go
// against their recorded documents; neither literal moved.
func TestTheWellKnownIDsDidNotMove(t *testing.T) {
	if got := mustID(t, imagefactory.Schematic{}); got != emptySchematicID {
		t.Errorf("empty schematic id = %s, want %s", got, emptySchematicID)
	}
	if got := mustID(t, goodSchematic()); got != consoleSchematicID {
		t.Errorf("console schematic id = %s, want %s", got, consoleSchematicID)
	}
}

// TestNotRepresentableReasonIsTheOneStatementOfTheRule pins the shape plan
// 02-20 needed and this package already had a precedent for: one predicate,
// call sites that reference it rather than restate it.
//
// Before it, the rule which decides what holzkube-manager will carry lived in an
// unexported function reachable only through Schematic.ID(), so the HTTP layer
// -- which has to answer the same question about a request field before it has
// a document at all -- had no way to ask it. The alternative was a second copy
// of the rule in internal/httpapi, and two copies of one rule is how a request
// validator and a serialiser end up disagreeing about which values exist.
//
// The binding is the second half of this test: for every diverging codepoint,
// the reason NotRepresentableReason returns and the reason the canonical writer
// puts on its NotRepresentableError are the same string. They cannot drift
// because the writer calls this function.
func TestNotRepresentableReasonIsTheOneStatementOfTheRule(t *testing.T) {
	for _, class := range divergingClasses {
		t.Run(class.class, func(t *testing.T) {
			for _, cp := range class.cps {
				t.Run(fmt.Sprintf("%U", cp), func(t *testing.T) {
					bad := "console=" + string(cp) + "ttyS0"

					reason := imagefactory.NotRepresentableReason(bad)
					if reason == "" {
						t.Fatalf("NotRepresentableReason accepted %U, which the "+
							"differential measured as DIVERGES", cp)
					}
					if !strings.Contains(reason, class.reason) {
						t.Errorf("reason = %q; want it to name the class %q", reason, class.reason)
					}
					if !strings.Contains(reason, fmt.Sprintf("%U", cp)) {
						t.Errorf("reason = %q; want the %%U rendering of the codepoint", reason)
					}
					// The exported form names the class and never the value, for
					// the reason the unexported one already did: this string
					// now reaches an operator through two routes rather than
					// one (T-02-64).
					if strings.Contains(reason, "ttyS0") {
						t.Errorf("reason echoed the offending value: %q", reason)
					}

					// The canonical writer's own refusal carries this exact
					// string. One rule, two call sites.
					s := imagefactory.Schematic{Customization: imagefactory.Customization{
						ExtraKernelArgs: []string{bad},
					}}
					_, err := s.ID()
					var typed *imagefactory.NotRepresentableError
					if !errors.As(err, &typed) {
						t.Fatalf("ID() = %v; want a *NotRepresentableError", err)
					}
					if typed.Reason != reason {
						t.Errorf("the serialiser's reason %q and the predicate's %q differ; "+
							"the rule has acquired a second statement", typed.Reason, reason)
					}
				})
			}
		})
	}

	// The over-refusal half. Widening the set on a guess moves the precomputed
	// id FACT-06 rests on, so the codepoints the differential measured as
	// AGREES are asserted accepted here as explicitly as the diverging ones are
	// asserted refused. U+00A0, U+200B and U+202E are three of the nine the UAT
	// named; all three round-trip.
	for _, cp := range []rune{0x00A0, 0x00E4, 0x200B, 0x202E, 0x4E2D, 0xFDD0, 0xFFFD, 0xD7FF, 0xE000} {
		if reason := imagefactory.NotRepresentableReason("console=" + string(cp) + "ttyS0"); reason != "" {
			t.Errorf("%U is refused (%q) although the differential measured it AGREES", cp, reason)
		}
	}

	// The other half of the rule, which is not a codepoint range at all.
	if reason := imagefactory.NotRepresentableReason("console=\xffttyS0"); !strings.Contains(reason, "valid UTF-8") {
		t.Errorf("reason = %q; want the not-valid-UTF-8 clause", reason)
	}
	if reason := imagefactory.NotRepresentableReason("console=ttyS0"); reason != "" {
		t.Errorf("an ordinary kernel argument was refused: %q", reason)
	}
}
