// Package imagefactory speaks to the Talos Image Factory at factory.talos.dev.
//
// Two rules govern everything here, and both exist because the upstream is
// documented to behave in a way that a naive client reads as success.
//
// First: creation is not validation. POSTing a schematic that references an
// extension which does not exist at the target Talos version returns 201 and a
// perfectly ordinary id; the ISO built from that id then returns 400. A UI that
// says "schematic created" on the strength of the POST is lying, and the
// operator finds out at the moment they try to boot a machine. So a schematic
// becomes usable here only after ProbeBuildable confirms the intended version
// and architecture actually build, and extension names are checked against the
// version-scoped catalog before any POST is issued.
//
// Second: the Factory's own answer is authoritative and is persisted verbatim.
// It re-serialises every schematic it accepts into a canonical form -- indentation,
// field order and scalar quoting are all normalised -- and the id is the SHA-256
// of exactly those bytes. Created.Canonical carries that document, and it is what
// callers store. The input is not the record.
//
// There is deliberately no fallback anywhere in this package. A catalog fetch
// that fails does not fall back to a cached or unscoped list, for the same
// reason internal/tlsx refuses to fall back from a supplied certificate that
// will not load: offering an extension that does not exist at the target
// version produces a schematic that is un-buildable at exactly the moment it
// matters, and nothing in the log would say why.
//
// The client is hand-rolled over net/http rather than using
// siderolabs/image-factory (D-01): that module requires machinery at a
// v1.14.0-rc.2 pseudo-version and lists the Talos root module among its direct
// requires, which would move holzkube-manager off its pin and pull the root module into
// a graph the package-level dependency guard does not cover.
package imagefactory

import "errors"

// DefaultBaseURL is the public Image Factory. It is a constant rather than a
// default buried in New so that a deployment pointing at a private Factory has
// one obvious thing to override.
const DefaultBaseURL = "https://factory.talos.dev"

var (
	// ErrExtensionUnknown reports an extension name that the version-scoped
	// catalog does not contain.
	//
	// Not retryable, and not a transient upstream condition: the catalog is
	// version-scoped, so the same name at the same Talos version will be absent
	// again. Either the name is wrong or the version is.
	ErrExtensionUnknown = errors.New("imagefactory: extension not in the catalog for this Talos version")

	// ErrSchematicNotBuildable reports a schematic the Factory accepted and
	// cannot build for the requested version and architecture.
	//
	// Not retryable. This is the failure P9 exists to surface: the POST
	// succeeded, so nothing before the probe had any reason to complain.
	ErrSchematicNotBuildable = errors.New("imagefactory: schematic is not buildable for this version and architecture")

	// ErrUpstreamUnavailable reports that the Factory did not answer, answered
	// with a status holzkube-manager will not act on, or answered something it will not
	// decode.
	//
	// Retryable, unlike the two above: it says nothing about the request, only
	// that the answer did not arrive. Callers map it to
	// httpapi.CodeUpstreamFactoryUnavailable.
	ErrUpstreamUnavailable = errors.New("imagefactory: upstream did not answer usably")
)
