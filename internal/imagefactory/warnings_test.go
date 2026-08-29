package imagefactory_test

import (
	"encoding/json"
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
