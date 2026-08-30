package imagefactory_test

import (
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/imagefactory"
)

// assetBase is a Factory base URL. The tests derive URLs rather than fetch
// them, so nothing here talks to a server.
const assetBase = "https://factory.talos.dev"

// schematicA is the well-known empty schematic id: 64 lowercase hex characters,
// the shape every derivation function requires.
const schematicA = "376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba"

// TestAssetURLs pins every asset shape against the paths recorded in
// STACK.md and re-probed live against factory.talos.dev during this plan.
//
// Table-driven across both architectures and both secure-boot settings,
// because FACT-03's whole point is that the architecture is a parameter: this
// host is darwin/arm64 and its target hardware is amd64, so an architecture
// baked into a URL is a bug that only appears on someone else's machine.
func TestAssetURLs(t *testing.T) {
	type derive func(string, imagefactory.AssetRequest) (string, error)

	cases := []struct {
		name       string
		fn         derive
		arch       imagefactory.Arch
		secureBoot bool
		want       string
	}{
		{
			name: "iso amd64",
			fn:   imagefactory.ISOURL,
			arch: imagefactory.ArchAMD64,
			want: assetBase + "/image/" + schematicA + "/v1.13.9/metal-amd64.iso",
		},
		{
			name: "iso arm64",
			fn:   imagefactory.ISOURL,
			arch: imagefactory.ArchARM64,
			want: assetBase + "/image/" + schematicA + "/v1.13.9/metal-arm64.iso",
		},
		{
			name:       "iso amd64 secureboot",
			fn:         imagefactory.ISOURL,
			arch:       imagefactory.ArchAMD64,
			secureBoot: true,
			want:       assetBase + "/image/" + schematicA + "/v1.13.9/metal-amd64-secureboot.iso",
		},
		{
			name: "pxe arm64",
			fn:   imagefactory.PXEURL,
			arch: imagefactory.ArchARM64,
			want: assetBase + "/pxe/" + schematicA + "/v1.13.9/metal-arm64",
		},
		{
			name:       "pxe arm64 secureboot",
			fn:         imagefactory.PXEURL,
			arch:       imagefactory.ArchARM64,
			secureBoot: true,
			want:       assetBase + "/pxe/" + schematicA + "/v1.13.9/metal-arm64-secureboot",
		},
		{
			name: "disk image arm64",
			fn:   imagefactory.DiskImageURL,
			arch: imagefactory.ArchARM64,
			want: assetBase + "/image/" + schematicA + "/v1.13.9/metal-arm64.raw.zst",
		},
		{
			name: "cmdline arm64",
			fn:   imagefactory.CmdlineURL,
			arch: imagefactory.ArchARM64,
			want: assetBase + "/image/" + schematicA + "/v1.13.9/cmdline-metal-arm64",
		},
		{
			name:       "cmdline amd64 secureboot",
			fn:         imagefactory.CmdlineURL,
			arch:       imagefactory.ArchAMD64,
			secureBoot: true,
			want:       assetBase + "/image/" + schematicA + "/v1.13.9/cmdline-metal-amd64-secureboot",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.fn(assetBase, imagefactory.AssetRequest{
				SchematicID: schematicA,
				Version:     "v1.13.9",
				Arch:        tc.arch,
				Platform:    imagefactory.PlatformMetal,
				SecureBoot:  tc.secureBoot,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got  %s\nwant %s", got, tc.want)
			}
		})
	}
}

// TestAssetURLsDifferOnlyInTheArchitecture is the FACT-03 guard in its most
// direct form: if anything other than the architecture segment changes between
// the two, an architecture has been baked in somewhere it should not be.
func TestAssetURLsDifferOnlyInTheArchitecture(t *testing.T) {
	req := imagefactory.AssetRequest{
		SchematicID: schematicA,
		Version:     "v1.13.9",
		Platform:    imagefactory.PlatformMetal,
	}

	req.Arch = imagefactory.ArchAMD64
	amd, err := imagefactory.ISOURL(assetBase, req)
	if err != nil {
		t.Fatalf("amd64: %v", err)
	}
	req.Arch = imagefactory.ArchARM64
	arm, err := imagefactory.ISOURL(assetBase, req)
	if err != nil {
		t.Fatalf("arm64: %v", err)
	}

	if amd == arm {
		t.Fatal("the two architectures produced the same URL, so the architecture is not in the path at all")
	}
	if normalised := strings.ReplaceAll(arm, "arm64", "amd64"); normalised != amd {
		t.Errorf("the URLs differ in more than the architecture segment:\n amd64: %s\n arm64: %s", amd, arm)
	}
}

// TestAssetURLsRefuseRatherThanGuess checks every refusal reason. A best-effort
// URL built from a value this package could not make sense of resolves to
// something, and what it resolves to is nobody's intent.
func TestAssetURLsRefuseRatherThanGuess(t *testing.T) {
	valid := imagefactory.AssetRequest{
		SchematicID: schematicA,
		Version:     "v1.13.9",
		Arch:        imagefactory.ArchAMD64,
		Platform:    imagefactory.PlatformMetal,
	}

	cases := []struct {
		name     string
		base     string
		mutate   func(*imagefactory.AssetRequest)
		contains string
	}{
		{
			name:     "empty schematic id",
			mutate:   func(r *imagefactory.AssetRequest) { r.SchematicID = "" },
			contains: "schematic id",
		},
		{
			name:     "schematic id that is not a sha256",
			mutate:   func(r *imagefactory.AssetRequest) { r.SchematicID = "not-a-schematic" },
			contains: "schematic id",
		},
		{
			name:     "schematic id carrying a path separator",
			mutate:   func(r *imagefactory.AssetRequest) { r.SchematicID = "../" + schematicA },
			contains: "schematic id",
		},
		{
			name:     "version that is not semver",
			mutate:   func(r *imagefactory.AssetRequest) { r.Version = "latest" },
			contains: "Talos version",
		},
		{
			name:     "unknown architecture",
			mutate:   func(r *imagefactory.AssetRequest) { r.Arch = imagefactory.Arch("riscv64") },
			contains: "architecture",
		},
		{
			name:     "unknown platform",
			mutate:   func(r *imagefactory.AssetRequest) { r.Platform = imagefactory.Platform("toaster") },
			contains: "platform",
		},
		{
			name:     "base URL that is not a URL",
			base:     "://nope",
			mutate:   func(*imagefactory.AssetRequest) {},
			contains: "base URL",
		},
		{
			name:     "base URL with no host",
			base:     "https://",
			mutate:   func(*imagefactory.AssetRequest) {},
			contains: "base URL",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := valid
			tc.mutate(&req)
			base := assetBase
			if tc.base != "" {
				base = tc.base
			}

			got, err := imagefactory.ISOURL(base, req)
			if err == nil {
				t.Fatalf("no error, and it produced %q", got)
			}
			if got != "" {
				t.Errorf("a refusal still returned a URL: %q", got)
			}
			if !strings.Contains(err.Error(), tc.contains) {
				t.Errorf("the refusal does not name what was refused (want %q): %v", tc.contains, err)
			}
		})
	}
}

// TestAssetURLsRefuseInEveryDerivation stops a validation gap opening in one of
// the four functions. They share a helper today; nothing but this test keeps
// that true.
func TestAssetURLsRefuseInEveryDerivation(t *testing.T) {
	bad := imagefactory.AssetRequest{
		SchematicID: schematicA,
		Version:     "v1.13.9",
		Arch:        imagefactory.Arch("riscv64"),
		Platform:    imagefactory.PlatformMetal,
	}

	for name, fn := range map[string]func(string, imagefactory.AssetRequest) (string, error){
		"ISOURL":       imagefactory.ISOURL,
		"PXEURL":       imagefactory.PXEURL,
		"DiskImageURL": imagefactory.DiskImageURL,
		"CmdlineURL":   imagefactory.CmdlineURL,
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := fn(assetBase, bad); err == nil {
				t.Fatalf("%s accepted an unknown architecture and produced %q", name, got)
			}
		})
	}
}

// TestAssetURLsJoinRatherThanConcatenate checks that a base carrying a trailing
// slash and one without produce the same URL. String concatenation gets this
// wrong in a way that still resolves -- to a different path.
func TestAssetURLsJoinRatherThanConcatenate(t *testing.T) {
	req := imagefactory.AssetRequest{
		SchematicID: schematicA,
		Version:     "v1.13.9",
		Arch:        imagefactory.ArchAMD64,
		Platform:    imagefactory.PlatformMetal,
	}

	with, err := imagefactory.ISOURL(assetBase+"/", req)
	if err != nil {
		t.Fatalf("trailing slash: %v", err)
	}
	without, err := imagefactory.ISOURL(assetBase, req)
	if err != nil {
		t.Fatalf("no trailing slash: %v", err)
	}
	if with != without {
		t.Errorf("a trailing slash on the base changed the URL:\n %s\n %s", with, without)
	}
}
