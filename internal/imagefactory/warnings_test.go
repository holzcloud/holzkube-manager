package imagefactory_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube/internal/imagefactory"
)

// TestWarningsForKernelArgs is FACT-04's core case. A schematic with a kernel
// argument produces an ISO that carries it and an installed system that does
// not, and the only moment an operator can act on that is while they are
// authoring the schematic.
func TestWarningsForKernelArgs(t *testing.T) {
	s := imagefactory.Schematic{
		Customization: imagefactory.Customization{ExtraKernelArgs: []string{"console=ttyS0"}},
	}

	got := imagefactory.Warnings(s)
	if len(got) != 1 {
		t.Fatalf("got %d warnings, want 1: %+v", len(got), got)
	}
	if got[0].Code != imagefactory.WarningInstallerIgnoresKernelArgs {
		t.Errorf("code = %q, want %q", got[0].Code, imagefactory.WarningInstallerIgnoresKernelArgs)
	}
	if !strings.Contains(got[0].Detail, ".machine.install.extraKernelArgs") {
		t.Errorf("the warning does not name the remedy: %q", got[0].Detail)
	}
}

// TestWarningsForMeta covers the other half: META is initial-META-only, applied
// at image build, and is not reapplied on upgrade.
func TestWarningsForMeta(t *testing.T) {
	s := imagefactory.Schematic{
		Customization: imagefactory.Customization{
			Meta: []imagefactory.MetaValue{{Key: 10, Value: "something"}},
		},
	}

	got := imagefactory.Warnings(s)
	if len(got) != 1 {
		t.Fatalf("got %d warnings, want 1: %+v", len(got), got)
	}
	if got[0].Code != imagefactory.WarningInstallerIgnoresMeta {
		t.Errorf("code = %q, want %q", got[0].Code, imagefactory.WarningInstallerIgnoresMeta)
	}
}

// TestWarningsForBoth checks the conditions are independent rather than an
// either/or.
func TestWarningsForBoth(t *testing.T) {
	s := imagefactory.Schematic{
		Customization: imagefactory.Customization{
			ExtraKernelArgs: []string{"console=ttyS0"},
			Meta:            []imagefactory.MetaValue{{Key: 10, Value: "something"}},
		},
	}

	got := imagefactory.Warnings(s)
	if len(got) != 2 {
		t.Fatalf("got %d warnings, want 2: %+v", len(got), got)
	}
	seen := map[string]bool{}
	for _, w := range got {
		seen[w.Code] = true
	}
	if !seen[imagefactory.WarningInstallerIgnoresKernelArgs] || !seen[imagefactory.WarningInstallerIgnoresMeta] {
		t.Errorf("both conditions hold and the warnings are %+v", got)
	}
}

// TestWarningsForNeitherIsEmptyAndNotNil keeps "the server checked and found
// nothing" distinguishable from "the server did not check". A JSON null here is
// the second statement.
func TestWarningsForNeitherIsEmptyAndNotNil(t *testing.T) {
	got := imagefactory.Warnings(imagefactory.Schematic{})

	if got == nil {
		t.Fatal("Warnings returned nil, which encodes as null")
	}
	if len(got) != 0 {
		t.Fatalf("an unadorned schematic produced %+v", got)
	}

	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if string(encoded) != "[]" {
		t.Errorf("encoded as %s, want []", encoded)
	}
}

// TestWarningsNameInstallerAndInitramfs pins the wording against the upstream
// restriction it paraphrases: "installer and initramfs images only support
// system extensions (kernel args and META are ignored)". A warning that does
// not name both assets does not tell an operator which images are affected.
func TestWarningsNameInstallerAndInitramfs(t *testing.T) {
	s := imagefactory.Schematic{
		Customization: imagefactory.Customization{
			ExtraKernelArgs: []string{"console=ttyS0"},
			Meta:            []imagefactory.MetaValue{{Key: 10, Value: "x"}},
		},
	}

	for _, w := range imagefactory.Warnings(s) {
		if !strings.Contains(w.Detail, "installer") {
			t.Errorf("%s does not name installer: %q", w.Code, w.Detail)
		}
		if !strings.Contains(w.Detail, "initramfs") {
			t.Errorf("%s does not name initramfs: %q", w.Code, w.Detail)
		}
		if w.Code == "" {
			t.Error("a warning carries no code")
		}
	}
}

// TestWarningsCodesAreNamespaced keeps the identifiers plan 02-06 renders and
// the audit allowlist references stable and recognisable as belonging to a
// stated family.
//
// It used to assert a single "schematic." prefix over a hardcoded list of two
// constants, which is a shape that fails silently: a third code outside the
// prefix would not turn it red, it would simply not be iterated, and the
// convention would fork with nothing saying so. That is the worse of the two
// failures, so the question is decided here explicitly rather than inherited.
//
// There are two families and the prefix says which:
//
//   - "schematic." is a statement about a stored schematic. Warnings recomputes
//     these from the record at any time, so they are properties of the thing
//     itself and survive the request that produced them.
//   - "installer." is a statement about one installer-repository resolution
//     attempt -- how a name was obtained on this request. No record holds it and
//     nothing can recompute it later. Filing it under "schematic." would blame
//     the schematic for something the registry did.
//
// Every code the package exports has to choose one of those prefixes, and
// "every" is enumerated rather than listed. The rewrite that widened the prefix
// set left the hand-written []string of three constants in place, which is the
// same silent failure in a new coat: a fourth exported code was still simply not
// iterated, so the sentence promising it was checked was the only thing
// enforcing it. exportedWarningCodes reads the package's own source instead, so
// a constant added tomorrow is covered by this test without anyone remembering
// it exists.
func TestWarningsCodesAreNamespaced(t *testing.T) {
	prefixes := map[string]string{
		"schematic.": "a property of the stored schematic, recomputable from the record",
		"installer.": "a fact about one installer-repository resolution attempt",
	}

	codes := exportedWarningCodes(t)

	// A scan is only a guard while it is finding things: one that matched
	// nothing -- a moved file, a renamed prefix, a constant expressed as
	// something other than a string literal -- would pass this test in silence,
	// which is precisely the shape being removed. These three are named so the
	// compiler checks the reference, and their presence proves the scan read the
	// file it thinks it read.
	found := map[string]bool{}
	for _, code := range codes {
		found[code] = true
	}
	for _, known := range []string{
		imagefactory.WarningInstallerIgnoresKernelArgs,
		imagefactory.WarningInstallerIgnoresMeta,
		imagefactory.WarningInstallerRepoFallbackUnverified,
	} {
		if !found[known] {
			t.Fatalf("scanning this package's source found %d exported warning codes and %q was "+
				"not among them, so the scan is not reading what it is meant to read and "+
				"nothing below is being enforced", len(codes), known)
		}
	}

	for name, code := range codes {
		matched := false
		for prefix := range prefixes {
			if strings.HasPrefix(code, prefix) {
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("%s = %q is in none of the declared warning-code families %v. "+
				"Pick one and say why, or add a family here with its meaning beside it",
				name, code, prefixes)
		}
	}
}

// exportedWarningCodes reads this package's own non-test source and returns
// every exported constant whose name begins with "Warning", keyed by that name.
//
// It parses rather than greps because a grep over the same files would match the
// word in a doc comment and in TestWarningDetailsMatchTheUI's own expectations,
// and would have to be taught the difference. go/ast already knows it. The whole
// directory is walked rather than warnings.go alone: the constants live there
// today, and a guard that only looks where they currently are is the guard that
// misses the one added somewhere else.
//
// Anything it cannot read is a failure and not a skip -- an exported Warning
// constant expressed as something other than a string literal would otherwise
// drop silently out of the check that is the entire point of this file.
func exportedWarningCodes(t *testing.T) map[string]string {
	t.Helper()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package directory: %v", err)
	}

	codes := map[string]string{}
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, ident := range value.Names {
					if !ident.IsExported() || !strings.HasPrefix(ident.Name, "Warning") {
						continue
					}
					if i >= len(value.Values) {
						t.Fatalf("%s: %s has no value of its own, so this guard cannot read it",
							name, ident.Name)
					}
					lit, ok := value.Values[i].(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						t.Fatalf("%s: %s is not a string literal, so this guard cannot read it",
							name, ident.Name)
					}
					unquoted, err := strconv.Unquote(lit.Value)
					if err != nil {
						t.Fatalf("%s: unquoting %s: %v", name, ident.Name, err)
					}
					codes[ident.Name] = unquoted
				}
			}
		}
	}
	return codes
}

// TestWarningDetailsMatchTheUI is the drift guard behind a claim plan 02-06
// makes: the live warning shown while an operator is still typing carries the
// server's own wording, so the sentence read before submitting and the sentence
// read afterwards cannot describe one condition as two problems.
//
// The UI cannot import a Go constant, so the strings are transcribed there. A
// transcription that nothing checks is a comment, not a guarantee -- this test
// is what makes it a guarantee. It reads from the Go side because that is where
// the text is authored: vitest is rooted at web/ and refuses to read outside it
// without loosening the bundler's filesystem allowlist, which is a real cost to
// pay for a test-only convenience.
//
// If this fails, the fix is to copy the Go detail into
// web/src/components/SchematicWarnings.tsx, never the other way round.
func TestWarningDetailsMatchTheUI(t *testing.T) {
	// The sentences are transcribed in the component; the codes are declared
	// once in the API module the component imports them from, which is why the
	// two halves are checked against two different files.
	const (
		uiPath  = "../../web/src/components/SchematicWarnings.tsx"
		apiPath = "../../web/src/api.ts"
	)

	ui := readUTF8(t, uiPath)
	apiModule := readUTF8(t, apiPath)

	full := imagefactory.Warnings(imagefactory.Schematic{
		Customization: imagefactory.Customization{
			ExtraKernelArgs: []string{"console=ttyS0"},
			Meta:            []imagefactory.MetaValue{{Key: 10, Value: "x"}},
		},
	})
	if len(full) != 2 {
		t.Fatalf("got %d warnings, want both of them", len(full))
	}

	for _, w := range full {
		if !strings.Contains(ui, w.Detail) {
			t.Errorf("%s: the UI does not carry this sentence verbatim.\n"+
				"Go:  %q\nCopy it into %s.", w.Code, w.Detail, uiPath)
		}
		if !strings.Contains(apiModule, w.Code) {
			t.Errorf("%s: %s declares no constant for this code", w.Code, apiPath)
		}
	}

	// The installer codes are checked differently, and deliberately: only the
	// code, never a sentence. The other two details are static text authored
	// once in Warnings, so they can be transcribed and compared byte for byte.
	// These are built per incident -- they name the repository that answered,
	// the repository that did not, the version and the transport error as the
	// client reported it -- so there is no sentence to transcribe. The UI renders
	// the server's detail as it arrives; what has to stay in step is the
	// identifier the component keys on.
	//
	// Enumerated rather than listed by name, which is the point of this loop.
	// This check used to name the one installer code it knew about, so plan
	// 02-16 could add installer.secureboot-repo-fallback-unverified, ship the Go
	// half with no TS mirror, and go green -- the drift guard was blind to the
	// exact drift it exists to catch. exportedWarningCodes walks the package's
	// own source, so a code added anywhere in it is covered the moment it is
	// declared.
	for name, code := range exportedWarningCodes(t) {
		if !strings.Contains(apiModule, code) {
			t.Errorf("%s (%s): %s declares no constant for this code. Add the mirror there, "+
				"and a row to docs/api-contract.md's warning table.", code, name, apiPath)
		}
	}
}

func readUTF8(t *testing.T, path string) string {
	t.Helper()
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(source)
}
