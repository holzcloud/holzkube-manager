package imagefactory

import (
	"context"
	"fmt"
)

// AuthorRequest is one schematic-authoring attempt: a schematic, and the
// version and architecture it is meant to build for.
type AuthorRequest struct {
	TalosVersion string
	Arch         Arch
	Schematic    Schematic
}

// Authored is the outcome of authoring a schematic.
type Authored struct {
	// ID is the Factory's schematic id.
	ID string

	// Canonical is the Factory's own normalised document, which is what a
	// caller persists.
	Canonical string

	// Usable says whether an image for the requested version and architecture
	// actually builds. It is false unless the probe said so; it is never
	// inferred from the creation having succeeded.
	Usable bool
}

// Author walks the whole path: precompute the id, validate every extension name
// against the version-scoped catalog, create the schematic, confirm the
// Factory's id matches the precomputed one, and probe whether the intended
// image builds.
//
// The order is the point, and it lives here rather than in each caller so that
// it is a property of the code rather than of everyone's memory. Two things it
// makes structurally true:
//
//   - There is no route from a failed catalog fetch to a POST. A caller cannot
//     skip validation, and validation cannot fall back to a stale list, so a
//     schematic naming an extension that does not exist at the target version
//     is never created in the first place.
//   - Usable is set from the probe alone. A created schematic with a failing
//     probe comes back with its id, its document and Usable false, together
//     with ErrSchematicNotBuildable -- the record is kept, because the
//     schematic does exist upstream, but nothing in it reads as success.
//
// The same rule governs every failure after the POST, ErrSchematicIDMismatch
// included: once the Factory has created the schematic, the returned Authored
// carries its id and its document alongside the error. A caller that ignores
// the error still stores a record that exists upstream; a caller that ignores
// the Authored loses the only reference to it.
func Author(ctx context.Context, c *Client, req AuthorRequest) (Authored, error) {
	if c == nil {
		return Authored{}, fmt.Errorf("imagefactory: Author was given no client")
	}
	if !talosVersionPattern.MatchString(req.TalosVersion) {
		return Authored{}, fmt.Errorf("imagefactory: %q is not a Talos version", req.TalosVersion)
	}
	if !req.Arch.Valid() {
		return Authored{}, fmt.Errorf("imagefactory: %q is not an architecture the Factory builds for", req.Arch)
	}

	// Precomputed before anything crosses the network. If the schematic cannot
	// be serialised canonically there is no point asking the Factory about it,
	// and the failure names the value rather than an HTTP status.
	if _, err := req.Schematic.ID(); err != nil {
		return Authored{}, err
	}

	catalog, err := c.Extensions(ctx, req.TalosVersion)
	if err != nil {
		return Authored{}, fmt.Errorf("validate extensions for %s: %w", req.TalosVersion, err)
	}
	if err := ValidateExtensions(catalog, req.Schematic.Customization.SystemExtensions.OfficialExtensions); err != nil {
		return Authored{}, err
	}

	created, err := c.CreateSchematic(ctx, req.Schematic)
	if created.ID == "" {
		// Nothing exists upstream, so err is the whole answer.
		return Authored{}, err
	}

	// From here the schematic exists upstream whatever else went wrong, and the
	// caller has to be able to record it: the Factory offers no way to list
	// schematics back, so an id its caller never sees is an id nobody can
	// recover. CreateSchematic returns a populated Created next to
	// ErrSchematicIDMismatch for exactly this reason -- the Factory's id and
	// document are authoritative even when the local computation disagreed
	// about the id -- and dropping it here would make that deliberate design a
	// dead branch.
	//
	// Usable stays false. No probe has run, and a mismatch is not a reason to
	// run one: the schematic has been created and nothing more than that is
	// known about it.
	out := Authored{ID: created.ID, Canonical: created.Canonical}
	if err != nil {
		return out, err
	}

	if err := c.ProbeBuildable(ctx, created.ID, req.TalosVersion, req.Arch); err != nil {
		return out, err
	}
	out.Usable = true
	return out, nil
}
