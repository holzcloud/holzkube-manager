package imagefactory

// The schematic wire shapes, copied rather than imported (D-01).
//
// These mirror siderolabs/image-factory's pkg/schematic. The Factory rejects a
// document carrying a field it does not know -- it answers 400 with
// "field <name> not found in type schematic.Schematic" -- so this struct set is
// the contract, not a convenience. A field added here that upstream does not
// have turns every POST into a 400; a field upstream has and this does not is a
// capability holzkube-manager cannot offer, which is a gap rather than a bug.
//
// Field order is load-bearing. The Factory canonicalises what it receives and
// hashes the result, so the order below is the order it emits -- owner, then
// overlay, then customization -- and reordering these declarations changes every
// computed id. It was established by observation against the live Factory, and
// schematicid_test.go pins it against recorded documents.

// Schematic is a request to the Factory for a customised Talos image.
//
// The zero value is the well-known empty schematic, which canonicalises to
// "customization: {}\n" and has the id
// 376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba.
type Schematic struct {
	// Owner is an opaque label the Factory stores with the schematic. It
	// changes the id, so two schematics that differ only in owner are two
	// schematics.
	Owner string

	// Overlay selects an SBC overlay image. Omitted entirely when zero; when
	// present, both of its fields are always emitted, empty string included.
	Overlay Overlay

	// Customization is the part operators actually fill in. Always emitted --
	// as "{}" when empty -- which is why the empty schematic has a non-trivial
	// canonical form.
	Customization Customization
}

// Overlay names an SBC overlay image and the board within it.
//
// The Factory's overlay also carries a free-form `options` mapping. It is
// deliberately not modelled: holzkube-manager has no SBC provisioning path in this
// milestone, and an unmodelled field cannot be silently dropped from a
// schematic that was never able to express it in the first place. Adding it
// later is a struct field plus an emitter case, and the recorded-document tests
// will catch a mistake.
type Overlay struct {
	Image string
	Name  string
}

// IsZero reports whether the overlay should be omitted from the document.
func (o Overlay) IsZero() bool { return o.Image == "" && o.Name == "" }

// Customization is the operator-facing half of a schematic.
type Customization struct {
	// ExtraKernelArgs applies to the ISO and disk images only. The installer
	// and initramfs ignore it (PITFALLS P9a), so a machine can boot from a USB
	// stick with these arguments and then install a system without them.
	ExtraKernelArgs []string

	// Meta are META partition values written into the image.
	Meta []MetaValue

	// SystemExtensions are the official extensions to bake in. Order is
	// preserved by the Factory and therefore changes the id: these are a list,
	// not a set.
	SystemExtensions SystemExtensions

	// SecureBoot toggles inclusion of the well-known certificates.
	SecureBoot SecureBoot
}

// IsZero reports whether the customization renders as an empty mapping.
func (c Customization) IsZero() bool {
	return len(c.ExtraKernelArgs) == 0 &&
		len(c.Meta) == 0 &&
		c.SystemExtensions.IsZero() &&
		c.SecureBoot.IsZero()
}

// MetaValue is one META partition entry. Key is a uint8 because the META
// partition addresses its slots with a single byte.
type MetaValue struct {
	Key   uint8
	Value string
}

// SystemExtensions lists official extensions by catalog name, e.g.
// "siderolabs/intel-ucode". Every name must exist in the catalog for the target
// Talos version -- see ValidateExtensions, and P9's central finding that the
// Factory itself will not check this for you.
type SystemExtensions struct {
	OfficialExtensions []string
}

// IsZero reports whether the extension block should be omitted.
func (s SystemExtensions) IsZero() bool { return len(s.OfficialExtensions) == 0 }

// SecureBoot carries the SecureBoot customization.
type SecureBoot struct {
	IncludeWellKnownCertificates bool
}

// IsZero reports whether the SecureBoot block should be omitted. A false
// IncludeWellKnownCertificates is indistinguishable from an absent block in the
// canonical form -- verified against the live Factory, which canonicalises
// {"secureboot":{"includeWellKnownCertificates":false}} back to
// "customization: {}".
func (s SecureBoot) IsZero() bool { return !s.IncludeWellKnownCertificates }
