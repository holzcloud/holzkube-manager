package imagefactory_test

import (
	"encoding/json"
	"os"
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
// the audit allowlist references stable and recognisable as one family.
func TestWarningsCodesAreNamespaced(t *testing.T) {
	for _, code := range []string{
		imagefactory.WarningInstallerIgnoresKernelArgs,
		imagefactory.WarningInstallerIgnoresMeta,
	} {
		if !strings.HasPrefix(code, "schematic.") {
			t.Errorf("%q is not in the schematic. namespace", code)
		}
	}
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
}

func readUTF8(t *testing.T, path string) string {
	t.Helper()
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(source)
}
