package imagefactory

import (
	"fmt"
	"net/url"
)

// Arch is a machine architecture the Factory builds for.
//
// It is a parameter of every asset URL and never a constant inside one. FACT-03
// says "ohne hartkodierte Architektur" and the reason is concrete: holzkube is
// developed on darwin/arm64 and its target hardware is amd64, so an
// architecture baked into a URL is a bug that works perfectly on the machine
// that wrote it and fails on every machine that matters.
type Arch string

// The architectures the Factory publishes assets for.
const (
	ArchAMD64 Arch = "amd64"
	ArchARM64 Arch = "arm64"
)

// Valid reports whether the architecture is one the Factory serves. It is
// checked before the value reaches a URL path.
func (a Arch) Valid() bool { return a == ArchAMD64 || a == ArchARM64 }

// Platform is the boot platform an asset is built for.
//
// Only bare metal exists in this milestone. It is still a named type with a
// closed domain rather than a string literal, because it appears in a URL path
// and in the installer repository name, and both of those are places where an
// unvalidated value addresses a different thing than the code reads as being
// addressed.
type Platform string

// PlatformMetal is bare metal, the only platform holzkube provisions in this
// milestone.
const PlatformMetal Platform = "metal"

// Valid reports whether the platform is one this package builds URLs for.
func (p Platform) Valid() bool { return p == PlatformMetal }

// AssetRequest names one buildable artifact: which schematic, at which Talos
// version, for which architecture and platform.
//
// The same value addresses the ISO, the PXE script, the disk image, the
// resolved kernel command line and the installer reference, so a caller that
// has decided what it wants decides it once.
type AssetRequest struct {
	// SchematicID is the 64-character lowercase hex id the Factory assigned.
	SchematicID string

	// Version is the Talos version, e.g. "v1.13.9".
	Version string

	// Arch is the machine architecture.
	Arch Arch

	// Platform is the boot platform.
	Platform Platform

	// SecureBoot selects the SecureBoot variant of the asset. It suffixes the
	// platform-architecture segment and nothing else.
	SecureBoot bool
}

// ISOURL is the installer ISO an operator writes to a USB stick.
//
// The ISO receives the full customization -- kernel arguments and META
// included -- which the installer and initramfs images do not. See Warnings:
// that asymmetry is the single most common way an ISO and the system it
// installs end up differing.
func ISOURL(base string, r AssetRequest) (string, error) {
	return r.assetURL(base, "image", r.variant()+".iso")
}

// PXEURL is the iPXE boot script for network booting the same image.
func PXEURL(base string, r AssetRequest) (string, error) {
	return r.assetURL(base, "pxe", r.variant())
}

// DiskImageURL is the raw disk image, zstd-compressed.
//
// The Factory serves both .raw.zst and .raw.xz for a current version; zstd is
// the form Talos itself moved to and the one written here, so holzkube offers
// one answer rather than a choice nobody has a basis to make.
func DiskImageURL(base string, r AssetRequest) (string, error) {
	return r.assetURL(base, "image", r.variant()+".raw.zst")
}

// CmdlineURL is the kernel command line the Factory resolves for this
// schematic.
//
// It is the only way to show an operator what a schematic actually produces
// rather than what they typed, which is what makes the ISO/installer
// divergence visible instead of theoretical.
func CmdlineURL(base string, r AssetRequest) (string, error) {
	return r.assetURL(base, "image", "cmdline-"+r.variant())
}

// variant is the platform-architecture segment, with the SecureBoot suffix when
// asked for. Every asset name is built from this one string, so secure boot
// cannot be applied to one asset shape and forgotten on another.
func (r AssetRequest) variant() string {
	v := fmt.Sprintf("%s-%s", r.Platform, r.Arch)
	if r.SecureBoot {
		v += "-secureboot"
	}
	return v
}

// assetURL validates the request and joins the path.
//
// Joining rather than concatenating: a base carrying a trailing slash and one
// without must produce the same URL, and a stray separator in a concatenated
// path produces a URL that still resolves -- to something nobody asked for.
func (r AssetRequest) assetURL(base, prefix, file string) (string, error) {
	u, err := parseAssetBase(base)
	if err != nil {
		return "", err
	}
	if err := r.validate(); err != nil {
		return "", err
	}
	return u.JoinPath(prefix, r.SchematicID, r.Version, file).String(), nil
}

// parseAssetBase turns a Factory base URL into something safe to join onto.
func parseAssetBase(base string) (*url.URL, error) {
	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("imagefactory: base URL %q does not parse: %w", base, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("imagefactory: base URL %q must be http or https, got scheme %q", base, u.Scheme)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("imagefactory: base URL %q has no host", base)
	}
	return u, nil
}

// validate refuses a request rather than deriving a plausible URL from it.
//
// Each refusal names what was refused and why, in the register
// tlsx.LoopbackGuard uses: a message an operator can act on without reading the
// source. The values are validated and not escaped for the reason client.go
// already argues -- a segment containing a slash or a dot-dot addresses a
// different endpoint than this code reads as being addressed, and rejecting the
// shape is checkable where escaping at every call site is not.
func (r AssetRequest) validate() error {
	if !schematicIDPattern.MatchString(r.SchematicID) {
		return fmt.Errorf(
			"imagefactory: %q is not a schematic id; the Factory assigns 64 lowercase hex characters and only that shape reaches an asset URL",
			r.SchematicID)
	}
	if !talosVersionPattern.MatchString(r.Version) {
		return fmt.Errorf(
			"imagefactory: %q is not a Talos version; an asset URL takes an exact version such as v1.13.9 and never a moving target like \"latest\"",
			r.Version)
	}
	if !r.Arch.Valid() {
		return fmt.Errorf(
			"imagefactory: %q is not an architecture the Factory builds for; it publishes %s and %s",
			r.Arch, ArchAMD64, ArchARM64)
	}
	if !r.Platform.Valid() {
		return fmt.Errorf(
			"imagefactory: %q is not a platform holzkube provisions; this milestone builds for %s only",
			r.Platform, PlatformMetal)
	}
	return nil
}
