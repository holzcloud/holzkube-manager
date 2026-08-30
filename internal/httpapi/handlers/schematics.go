package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/holzcloud/holzkube/internal/httpapi"
	"github.com/holzcloud/holzkube/internal/imagefactory"
	"github.com/holzcloud/holzkube/internal/model"
	"github.com/holzcloud/holzkube/internal/store"
)

// versionBuckets is the answer to GET /api/v1/factory/versions.
//
// All three collections encode as [] or {} when empty and never as null: a null
// reads to a client as "the server did not check", which is a different and much
// weaker statement than "there are none".
type versionBuckets struct {
	Stable       []string          `json:"stable"`
	Prerelease   []string          `json:"prerelease"`
	NewestStable string            `json:"newest_stable"`
	Broken       map[string]string `json:"broken"`
}

// extensionCatalog is the answer to GET /api/v1/factory/extensions.
//
// The version is echoed back because the catalog is version-scoped and a client
// that has changed its selection while a request was in flight must be able to
// tell which version the list it is holding belongs to. Serving a catalog that
// does not say what it is a catalog of is how a stale list gets used.
type extensionCatalog struct {
	Version    string                   `json:"version"`
	Extensions []imagefactory.Extension `json:"extensions"`
}

// createdSchematic is the 201 body: the stored record plus the warnings.
//
// The warnings ride alongside the record rather than inside it because they are
// a statement about this authoring attempt, not a property of the stored
// schematic -- they are recomputable from the record at any time and are not
// persisted.
type createdSchematic struct {
	model.Schematic
	Warnings []imagefactory.Warning `json:"warnings"`
}

// assetReferences is the answer to GET /api/v1/schematics/{id}/assets.
//
// Warnings carries the JSON name the 201 body already uses, deliberately: a
// client has one warning shape to learn rather than two, and the same component
// renders both.
//
// Note where the boundary is, because the two are not the same kind of
// statement. The 201 body's warnings are about the *schematic* and are
// recomputable from the record at any time. These are about *this resolution* --
// how the installer repository name was obtained on this request -- and nothing
// holds them. That is why they ride on the response and are not persisted: there
// is no record to put them in and no later moment at which they could be
// derived again.
//
// Installer is a pointer, and that is the shape decision this route turns on.
// The other four references are functions of the request -- see imagefactory's
// URL builders, none of which reaches the registry -- and the installer is the
// one field an upstream can withhold. When it is withheld the field is null and
// InstallerError says why, so a client written before that outcome existed
// fails to decode a null into a string rather than reading an empty one as a
// proven reference. An empty string would have been an affirmative claim of
// proof, because `warnings: []` on this route means exactly that.
type assetReferences struct {
	ISO       string  `json:"iso"`
	PXE       string  `json:"pxe"`
	DiskImage string  `json:"disk_image"`
	Cmdline   string  `json:"cmdline"`
	Installer *string `json:"installer"`

	// InstallerError is present when, and only when, Installer is null. Its
	// absence is the signal on a resolved answer, so a proven response is byte
	// for byte what it was before this member existed.
	InstallerError *installerUnresolved `json:"installer_error,omitempty"`

	Warnings []imagefactory.Warning `json:"warnings"`
}

// installerUnresolved is the problem the route would have answered with, minus
// the envelope.
//
// It carries the code and the detail and nothing else. The code is the part the
// client branches on -- upstream.factory-rejected is a verdict about this
// version and no retry changes it, upstream.factory-unavailable is a registry
// that did not answer and asking again may work -- and the detail is the
// server's own sentence, already naming the schematic, the version and the
// repository names that were asked. A client that re-derived either from a
// status code would be inventing a second, divergent account of one failure.
//
// There is deliberately no status member. This travels inside a 200: the
// request succeeded and four of the five references are in the same body. A
// status here would be a number describing a response that never happened.
type installerUnresolved struct {
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

// schematicInput is the POST body.
//
// Every field name here is also an audit parameter name, because the audit
// middleware captures the decoded body. Only name and talos_version are on the
// allowlist in internal/audit; the rest are written redacted, permanently.
type schematicInput struct {
	Name         string      `json:"name"`
	Cluster      string      `json:"cluster"`
	TalosVersion string      `json:"talos_version"`
	Arch         string      `json:"arch"`
	Extensions   []string    `json:"extensions"`
	KernelArgs   []string    `json:"kernel_args"`
	Meta         []metaInput `json:"meta"`
	SecureBoot   bool        `json:"secureboot"`
}

type metaInput struct {
	Key   uint8  `json:"key"`
	Value string `json:"value"`
}

// SchematicRoutes serves the Image Factory surface: the version and extension
// catalogs an operator picks from, and the schematics they assemble.
//
// The seven routes are the ones contracted in docs/api-contract.md. All of them
// require a session. Only DELETE is Destructive: a schematic id that is neither
// stored here nor readable from a running node is gone, because the Factory
// deliberately offers no way to list schematics back. POST is mutating and is
// not destructive -- the two are different things, and per D-03 the Destructive
// flag is not a dry-run mechanism.
func SchematicRoutes(d httpapi.Deps) []httpapi.Route {
	return []httpapi.Route{
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/factory/versions",
			RequiresSession: true,
			Handler:         handler(factoryVersions(d)),
		},
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/factory/extensions",
			RequiresSession: true,
			Handler:         handler(factoryExtensions(d)),
		},
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/schematics",
			RequiresSession: true,
			Action:          "schematic.create",
			Handler:         handler(createSchematic(d)),
		},
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/schematics",
			RequiresSession: true,
			Handler:         handler(listSchematics(d)),
		},
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/schematics/{id}",
			RequiresSession: true,
			Handler:         handler(getSchematic(d)),
		},
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/schematics/{id}/assets",
			RequiresSession: true,
			Handler:         handler(schematicAssets(d)),
		},
		{
			Method:          http.MethodDelete,
			Pattern:         "/api/v1/schematics/{id}",
			RequiresSession: true,
			Destructive:     true,
			Action:          "schematic.delete",
			Handler:         handler(deleteSchematic(d)),
		},
	}
}

func factoryVersions(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Factory == nil {
			httpapi.WriteProblem(w, r, factoryNotConfigured())
			return
		}

		versions, err := d.Factory.Versions(r.Context())
		if err != nil {
			httpapi.WriteProblem(w, r, factoryProblem(err, "listing the Talos versions the Factory can build"))
			return
		}

		stable, prerelease := imagefactory.SplitVersions(versions)
		// A list with no stable version at all is an upstream answer holzkube will
		// not act on, not an empty selection: silently promoting a release
		// candidate is what FACT-05 exists to prevent.
		newest, err := imagefactory.NewestStable(versions)
		if err != nil {
			httpapi.WriteProblem(w, r, httpapi.Upstream(httpapi.CodeUpstreamFactoryUnavailable,
				"The Factory listed no stable Talos version."))
			return
		}

		writeJSON(w, http.StatusOK, versionBuckets{
			Stable:       stable,
			Prerelease:   prerelease,
			NewestStable: newest,
			Broken:       imagefactory.BrokenIn(versions),
		})
	}
}

func factoryExtensions(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Factory == nil {
			httpapi.WriteProblem(w, r, factoryNotConfigured())
			return
		}

		version := r.URL.Query().Get("version")
		if !isTalosVersion(version) {
			httpapi.WriteProblem(w, r, httpapi.Validation(
				"The extension catalog is version-scoped and there is no fallback, so the version is required.",
				httpapi.FieldError{Field: "version", Reason: "must be a Talos version such as v1.13.9"}))
			return
		}

		catalog, err := d.Factory.Extensions(r.Context(), version)
		if err != nil {
			httpapi.WriteProblem(w, r, factoryProblem(err, "fetching the extension catalog for "+version))
			return
		}

		writeJSON(w, http.StatusOK, extensionCatalog{Version: version, Extensions: catalog})
	}
}

func createSchematic(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Factory == nil {
			httpapi.WriteProblem(w, r, factoryNotConfigured())
			return
		}

		// The body is read whole before it is decoded, because one class of
		// refusal is invisible afterwards. encoding/json rewrites an escaped
		// unpaired surrogate -- and a raw byte that is not valid UTF-8 -- into
		// U+FFFD as it decodes, so a validator handed the decoded string is
		// looking at a repaired value and has nothing to object to. That is how
		// a caller got a 201 and a schematic id computed over a character it
		// never sent (T-02-67, WINDOWS entry 29). The check has to stand here,
		// on the bytes, or it cannot stand anywhere.
		raw, problem := readBody(w, r)
		if problem != nil {
			httpapi.WriteProblem(w, r, problem)
			return
		}
		if problem := rawBodyRefusal(raw); problem != nil {
			httpapi.WriteProblem(w, r, problem)
			return
		}

		var in schematicInput
		r.Body = io.NopCloser(bytes.NewReader(raw))
		if err := decodeJSON(w, r, &in); err != nil {
			httpapi.WriteProblem(w, r, httpapi.Validation(err.Error()))
			return
		}
		if problem := in.validate(); problem != nil {
			httpapi.WriteProblem(w, r, problem)
			return
		}

		// Author owns the order this route's correctness depends on: fetch the
		// version-scoped catalog, reject every unknown extension name before any
		// POST, create, confirm the Factory's id equals the locally precomputed
		// one, and only then probe whether the intended image builds. Reproducing
		// that order here would make it a property of this handler's memory rather
		// than of the package that documents it.
		authored, err := imagefactory.Author(r.Context(), d.Factory, imagefactory.AuthorRequest{
			TalosVersion: in.TalosVersion,
			Arch:         imagefactory.Arch(in.Arch),
			Schematic:    in.schematic(),
		})
		if authored.ID == "" {
			// Nothing was created upstream, so there is nothing to store and err is
			// the whole answer.
			httpapi.WriteProblem(w, r, createProblem(err))
			return
		}

		// The schematic exists upstream, so the record is kept whatever the probe
		// said -- the Factory offers no way to list schematics back, and a record
		// dropped here is a reference an operator cannot recover. What the probe
		// said is recorded in the record rather than in whether it exists.
		//
		// ProbedAt is set only when the probe actually answered. A probe that could
		// not reach the Factory says nothing about the schematic, and leaving
		// ProbedAt zero is what keeps "never probed" distinguishable from "probed
		// and refused" -- two states the contract requires a UI not to merge.
		var probedAt time.Time
		var probeReason string
		if err == nil || errors.Is(err, imagefactory.ErrSchematicNotBuildable) {
			probedAt = time.Now().UTC()
		}
		if errors.Is(err, imagefactory.ErrSchematicNotBuildable) {
			probeReason = probeDetail(err)
		}

		rec := model.Schematic{
			ID:           model.SchematicID(authored.ID),
			Cluster:      model.ClusterID(in.Cluster),
			Name:         in.Name,
			TalosVersion: in.TalosVersion,
			// Arch is stamped unconditionally, unlike ProbedAt three lines
			// above. The two are different statements and the difference
			// matters: ProbedAt records whether an answer arrived, Arch records
			// what the question was about. A record whose probe never answered
			// still has an architecture it was asked about, and withholding it
			// would recreate in miniature the ambiguity G-02-8 is about.
			//
			// The value is in.Arch and not something read back from authored,
			// because in.Arch is exactly what was handed to Author and
			// therefore exactly what the probe used.
			Arch:        in.Arch,
			Canonical:   authored.Canonical,
			Extensions:  in.Extensions,
			KernelArgs:  in.KernelArgs,
			Meta:        in.meta(),
			Usable:      authored.Usable,
			ProbedAt:    probedAt,
			ProbeReason: probeReason,
			CreatedAt:   time.Now().UTC(),
		}
		stored, storeErr := d.Store.Schematics().Put(r.Context(), rec)
		switch {
		case errors.Is(storeErr, store.ErrConflict):
			// rec carries no Rev, so the store reads a Put against an existing
			// record as a compare-and-swap clash. It is not a lost update: the
			// id is the SHA-256 of the Factory's canonical document, so this is
			// the same schematic already stored under whatever label it was
			// first given, and any two authoring attempts sharing a
			// customisation land here regardless of name, cluster or the
			// version they were authored against.
			//
			// Refusing rather than overwriting is the conservative half of that
			// symmetry. The record is the only copy of a reference the Factory
			// will not list back, which is why DELETE is Destructive and behind
			// the sudo window; a POST that replaced the label, the version and
			// the probe verdict of an existing record would do most of that
			// same damage on a route the contract marks Destructive: false.
			// The operator can read the record back by id, or delete it and
			// author again.
			httpapi.WriteProblem(w, r, httpapi.Conflict("store.conflict",
				"This schematic already exists. Its id is the hash of its contents, so the same "+
					"customisation is the same schematic however it is named. Read it back by id, "+
					"or delete it and author it again."))
			return
		case storeErr != nil:
			httpapi.WriteInternal(w, r, d.Logger, storeErr)
			return
		}

		// The record is stored first and the problem written second, in that
		// order and never the other way round. An id the local computation did
		// not predict means the canonical serialisers have drifted, so every
		// id computed here without a round trip is suspect -- but the schematic
		// exists upstream under the Factory's id, and that id is only ever
		// recoverable from this record. Answering 201 would report drift in the
		// one mechanism FACT-06 rests on as success.
		if errors.Is(err, imagefactory.ErrSchematicIDMismatch) {
			httpapi.WriteProblem(w, r, createProblem(err))
			return
		}

		writeJSON(w, http.StatusCreated, createdSchematic{
			Schematic: schematicOut(stored),
			Warnings:  imagefactory.Warnings(in.schematic()),
		})
	}
}

func listSchematics(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		records, err := d.Store.Schematics().List(r.Context())
		if err != nil {
			httpapi.WriteInternal(w, r, d.Logger, err)
			return
		}
		out := make([]model.Schematic, 0, len(records))
		for _, rec := range records {
			out = append(out, schematicOut(rec))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func getSchematic(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rec, problem := loadSchematic(d, r)
		if problem != nil {
			httpapi.WriteProblem(w, r, problem)
			return
		}
		writeJSON(w, http.StatusOK, schematicOut(rec))
	}
}

// schematicOut replaces the nil collections of a record with empty ones.
//
// A nil slice encodes as null, and a null reads to a client as "the server did
// not say", which is a different statement from "there are none". The record
// itself keeps whatever the store holds; only the wire form is normalised, so
// nothing here changes what is persisted or what the id was computed over.
func schematicOut(rec model.Schematic) model.Schematic {
	if rec.Extensions == nil {
		rec.Extensions = []string{}
	}
	if rec.KernelArgs == nil {
		rec.KernelArgs = []string{}
	}
	if rec.Meta == nil {
		rec.Meta = []model.MetaValue{}
	}
	return rec
}

func schematicAssets(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Factory == nil {
			httpapi.WriteProblem(w, r, factoryNotConfigured())
			return
		}

		rec, problem := loadSchematic(d, r)
		if problem != nil {
			httpapi.WriteProblem(w, r, problem)
			return
		}

		req, problem := assetRequest(r, rec)
		if problem != nil {
			httpapi.WriteProblem(w, r, problem)
			return
		}

		base := d.Factory.BaseURL()
		iso, err := imagefactory.ISOURL(base, req)
		if err != nil {
			httpapi.WriteProblem(w, r, httpapi.Validation(err.Error()))
			return
		}
		pxe, err := imagefactory.PXEURL(base, req)
		if err != nil {
			httpapi.WriteProblem(w, r, httpapi.Validation(err.Error()))
			return
		}
		disk, err := imagefactory.DiskImageURL(base, req)
		if err != nil {
			httpapi.WriteProblem(w, r, httpapi.Validation(err.Error()))
			return
		}
		cmdline, err := imagefactory.CmdlineURL(base, req)
		if err != nil {
			httpapi.WriteProblem(w, r, httpapi.Validation(err.Error()))
			return
		}

		// The installer reference is resolved against the registry, never
		// assembled. It is consumed by the upgrade RPC, and a guessed one produces
		// an upgrade that reports success while silently dropping every system
		// extension the node was built with. If it cannot be resolved this route
		// answers with no installer reference at all rather than a plausible
		// string -- and for a SecureBoot request that rule is absolute, because a
		// substituted ordinary installer would be undetectable forever: SecureBoot
		// is a query parameter that never reaches the stored record, so no later
		// code, log or audit entry can re-derive that a substitution happened.
		//
		// What that rule does *not* license is answering with nothing at all. The
		// four references above are pure string assembly over this request --
		// nothing in ISOURL, PXEURL, DiskImageURL or CmdlineURL touches the
		// registry, and their only failure is a validation error about the
		// request itself, already handled. By the time resolution runs, four
		// correct references exist. Discarding them because a fifth could not be
		// obtained is a denial of service holzkube inflicts on itself, and it is
		// the branch an operator most often meets through a slow registry rather
		// than through a version that genuinely has no installer (02-UAT.md
		// G-02-15). So the installer alone is marked unresolved, carrying the
		// code and the detail the 502 would have carried, and the rest is served.
		//
		// The warnings say how sure of the name we are. A reference reached past
		// a candidate that never answered is usable but provisional, and the
		// operator has to be able to see that on the panel that shows it --
		// which is the whole of G-02-3.
		installer, warnings, err := d.Factory.InstallerImage(r.Context(), req)
		// Normalised the way schematicOut normalises the record's nil
		// collections, for the reason this file already gives: a null reads as
		// "the server did not check".
		if warnings == nil {
			warnings = []imagefactory.Warning{}
		}

		refs := assetReferences{
			ISO:       iso,
			PXE:       pxe,
			DiskImage: disk,
			Cmdline:   cmdline,
			Warnings:  warnings,
		}
		if err != nil {
			// The failure's own words, not a second sentence invented here. The
			// detail already names the schematic, the version and what the
			// registry answered, and the code already separates a refusal from a
			// non-answer -- which is precisely the distinction the operator needs
			// to tell "wait and retry" from "this version has no installer under
			// that name".
			problem := factoryProblem(err,
				"resolving the installer image reference for "+req.Version)
			refs.InstallerError = &installerUnresolved{
				Code:   problem.Code,
				Detail: problem.Detail,
			}
			writeJSON(w, http.StatusOK, refs)
			return
		}

		refs.Installer = &installer
		writeJSON(w, http.StatusOK, refs)
	}
}

func deleteSchematic(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := model.SchematicID(r.PathValue("id"))
		err := d.Store.Schematics().Delete(r.Context(), id)
		switch {
		case errors.Is(err, store.ErrNotFound), errors.Is(err, store.ErrInvalidKey):
			httpapi.WriteProblem(w, r, notFoundSchematic())
		case err != nil:
			httpapi.WriteInternal(w, r, d.Logger, err)
		default:
			writeJSON(w, http.StatusNoContent, nil)
		}
	}
}

// loadSchematic reads the record named by the {id} path segment.
func loadSchematic(d httpapi.Deps, r *http.Request) (model.Schematic, *httpapi.Problem) {
	rec, err := d.Store.Schematics().Get(r.Context(), model.SchematicID(r.PathValue("id")))
	switch {
	case errors.Is(err, store.ErrNotFound), errors.Is(err, store.ErrInvalidKey):
		// An unaddressable id and a missing record are the same answer to a
		// client: there is no such schematic. They are not the same to the
		// store, which is why the two errors exist -- but reporting the
		// difference here would say which ids are well-formed.
		return model.Schematic{}, notFoundSchematic()
	case err != nil:
		return model.Schematic{}, httpapi.Internal(err)
	}
	return rec, nil
}

// validate checks the fields this handler decides on before anything crosses
// the network. It reports every bad field at once.
func (in schematicInput) validate() *httpapi.Problem {
	var errs []httpapi.FieldError
	if in.Name == "" {
		errs = append(errs, httpapi.FieldError{
			Field: "name", Reason: "a schematic needs a label the operator can recognise it by",
		})
	}
	if !isTalosVersion(in.TalosVersion) {
		errs = append(errs, httpapi.FieldError{
			Field: "talos_version", Reason: "must be a Talos version such as v1.13.9",
		})
	}
	if !imagefactory.Arch(in.Arch).Valid() {
		errs = append(errs, httpapi.FieldError{
			Field: "arch", Reason: "must be amd64 or arm64; the build probe needs to know which image to ask for",
		})
	}
	in.refuseUnrepresentable(&errs)
	if len(errs) == 0 {
		return nil
	}
	return httpapi.Validation("The schematic is not valid.", errs...)
}

// refuseUnrepresentable appends a field error for every operator-supplied
// scalar in the request that holzkube will not carry.
//
// It asks imagefactory.NotRepresentableReason rather than restating the rule.
// That function is the single statement of which scalars survive serialisation,
// storage and rendering, and this is its second call site; the canonical writer
// is the first. A copy here would be a second rule that agreed with the first
// only until one of them was edited.
//
// name and cluster are in the list, and their absence from it is what G-02-17
// recorded. Both arrive in the same request body as kernel_args and meta, both
// are stored, and both are rendered -- name in the saved table and in the detail
// dialog's own heading -- so they are exactly as capable of carrying a
// character that cannot be shown as the fields that were already guarded. A
// POST with NUL and a right-to-left override in the name answered 201 and
// rendered the override raw. cluster was worse: nothing read it at all
// (WINDOWS entry 13).
//
// Every scalar is checked and each appends, so an operator fixing a form is
// told about all of their mistakes in one answer rather than one per round
// trip. That is a property validate already had for the three fields it knew
// about, and extending the list must not cost it.
func (in schematicInput) refuseUnrepresentable(errs *[]httpapi.FieldError) {
	scalar := func(field, value string) {
		if reason := imagefactory.NotRepresentableReason(value); reason != "" {
			*errs = append(*errs, httpapi.FieldError{Field: field, Reason: reason})
		}
	}
	sequence := func(field string, values []string) {
		for i, value := range values {
			if reason := imagefactory.NotRepresentableReason(value); reason != "" {
				*errs = append(*errs, httpapi.FieldError{Field: field, Reason: entryReason(i, reason)})
			}
		}
	}

	scalar("name", in.Name)
	scalar("cluster", in.Cluster)
	sequence("kernel_args", in.KernelArgs)
	sequence("extensions", in.Extensions)
	for i, m := range in.Meta {
		if reason := imagefactory.NotRepresentableReason(m.Value); reason != "" {
			*errs = append(*errs, httpapi.FieldError{Field: "meta", Reason: entryReason(i, reason)})
		}
	}
}

// schematic projects the input onto the Factory's own request type.
func (in schematicInput) schematic() imagefactory.Schematic {
	return imagefactory.Schematic{
		Customization: imagefactory.Customization{
			ExtraKernelArgs: in.KernelArgs,
			Meta:            in.factoryMeta(),
			SystemExtensions: imagefactory.SystemExtensions{
				OfficialExtensions: in.Extensions,
			},
			SecureBoot: imagefactory.SecureBoot{IncludeWellKnownCertificates: in.SecureBoot},
		},
	}
}

func (in schematicInput) factoryMeta() []imagefactory.MetaValue {
	if len(in.Meta) == 0 {
		return nil
	}
	out := make([]imagefactory.MetaValue, 0, len(in.Meta))
	for _, m := range in.Meta {
		out = append(out, imagefactory.MetaValue{Key: m.Key, Value: m.Value})
	}
	return out
}

func (in schematicInput) meta() []model.MetaValue {
	if len(in.Meta) == 0 {
		return nil
	}
	out := make([]model.MetaValue, 0, len(in.Meta))
	for _, m := range in.Meta {
		out = append(out, model.MetaValue{Key: m.Key, Value: m.Value})
	}
	return out
}

// assetRequest reads the asset query parameters.
//
// arch is required and has no default. holzkube is developed on arm64 and
// targets amd64, so a defaulted architecture is a bug that only ever appears on
// someone else's machine (FACT-03). The record now carries an architecture of
// its own (model.Schematic.Arch) and it is deliberately not read here: that
// field describes what was probed, and this parameter asks what to build. Using
// the description as the default is the FACT-03 bug wearing a record's clothes.
//
// version defaults to the version the record was authored and probed against,
// which is the only version this installation has any evidence about; platform
// defaults to the sole member of a closed type.
func assetRequest(r *http.Request, rec model.Schematic) (imagefactory.AssetRequest, *httpapi.Problem) {
	q := r.URL.Query()

	arch := imagefactory.Arch(q.Get("arch"))
	if !arch.Valid() {
		return imagefactory.AssetRequest{}, httpapi.Validation(
			"The architecture is required and is never defaulted.",
			httpapi.FieldError{Field: "arch", Reason: "must be amd64 or arm64"})
	}

	version := q.Get("version")
	if version == "" {
		version = rec.TalosVersion
	}
	if !isTalosVersion(version) {
		return imagefactory.AssetRequest{}, httpapi.Validation(
			"The Talos version is not a version.",
			httpapi.FieldError{Field: "version", Reason: "must be a Talos version such as v1.13.9"})
	}

	platform := imagefactory.Platform(q.Get("platform"))
	if q.Get("platform") == "" {
		platform = imagefactory.PlatformMetal
	}
	if !platform.Valid() {
		return imagefactory.AssetRequest{}, httpapi.Validation(
			"The platform is not one holzkube builds for.",
			httpapi.FieldError{Field: "platform", Reason: "must be metal"})
	}

	// Anything other than an explicit true is false. A parse error here would
	// be a 400 for a parameter whose absence is already meaningful.
	secureBoot, _ := strconv.ParseBool(q.Get("secureboot"))

	return imagefactory.AssetRequest{
		SchematicID: string(rec.ID),
		Version:     version,
		Arch:        arch,
		Platform:    platform,
		SecureBoot:  secureBoot,
	}, nil
}

// probeDetail is the probe's own sentence without the sentinel prefix.
//
// The prefix is the package's, and an operator reading "imagefactory:" learns
// nothing except that Go wrapped an error. What is left is what they can act
// on: the schematic, the version and architecture it was asked for, and the
// status the Factory answered with.
func probeDetail(err error) string {
	detail := strings.TrimPrefix(err.Error(),
		imagefactory.ErrSchematicNotBuildable.Error()+": ")
	if detail == "" {
		// A bare sentinel with nothing wrapped around it. Saying so is better
		// than storing an empty reason, which reads as "never probed".
		return imagefactory.ErrSchematicNotBuildable.Error()
	}
	return detail
}

// refusalReason renders the operator-facing half of a serialiser refusal.
//
// It carries the entry position and the character class and never the value.
// This is the string that actually leaves the process: it is rendered in a
// browser, may be logged by a proxy, and outlives the form that produced it,
// and a kernel argument can carry a secret (T-02-64). The position is
// one-based, matching what an operator counting rows in a form sees.
func refusalReason(e *imagefactory.NotRepresentableError) string {
	if e.Index >= 0 {
		return entryReason(e.Index, e.Reason)
	}
	return e.Reason
}

// entryReason places a refusal in a sequence. One-based, matching the row an
// operator counts in the form, and spelled once so a refusal raised by validate
// and a refusal raised by the serialiser read identically -- they are the same
// statement about the same value, and an operator should not have to notice
// which layer produced it.
func entryReason(index int, reason string) string {
	return "entry " + strconv.Itoa(index+1) + " " + reason
}

// requestFieldForPath maps a canonical document path onto the request's own
// field vocabulary -- the same names schematicInput.validate reports, because
// an operator reading two problems from one route should not have to learn two
// spellings of the same field.
//
// The table is explicit rather than derived. A derived mapping would silently
// invent a field name for a path that gains a new spelling upstream, and a
// wrong field name points the operator at the wrong input.
var requestFieldForPath = map[string]string{
	"customization.extraKernelArgs":                     "kernel_args",
	"customization.meta.value":                          "meta",
	"customization.systemExtensions.officialExtensions": "extensions",
}

// readBody reads the request body whole, under the same cap decodeJSON applies.
//
// It exists because rawBodyRefusal has to see the bytes the caller sent, and a
// decoder consumes the stream. The cap is applied here and again inside
// decodeJSON; MaxBytesReader is idempotent in the way that matters -- the
// second wrapper never sees more than the first allowed.
func readBody(w http.ResponseWriter, r *http.Request) ([]byte, *httpapi.Problem) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, httpapi.Validation(err.Error())
	}
	return raw, nil
}

// rawBodyRefusal answers the one question that cannot be asked after decoding.
//
// The contract, decided here rather than left to emerge: an unpaired surrogate
// escape and a byte sequence that is not valid UTF-8 are REFUSED with a 400,
// deterministically, and are never repaired. The alternative -- accept the
// U+FFFD encoding/json substitutes and carry on -- is the one thing every other
// decision on this route rules out: holzkube reports and refuses, it does not
// silently rewrite an operator's value into something they did not write
// (T-02-67). A schematic stored under a name its author would not recognise,
// with an id computed over a character they never sent, is worse than a 400.
//
// It is a raw-bytes check and it must stay one. By the time a Go string exists
// the evidence is gone: json.Decode has already written U+FFFD over both cases,
// so the value is valid UTF-8 with no control character in it and every check
// downstream passes. This is also why the test for it posts a byte slice --
// a test that marshals a Go string cannot reach this branch at all, which is
// precisely why the hole survived commit ec10e08's browser-side refusal.
//
// The field is named when it can be identified and left unnamed when it cannot,
// the same way createProblem treats a document path this handler does not
// recognise: saying less truthfully beats naming the wrong input.
func rawBodyRefusal(raw []byte) *httpapi.Problem {
	reason := rawTextReason(raw)
	if reason == "" {
		return nil
	}

	fieldErr := httpapi.FieldError{Reason: reason}
	// json.RawMessage copies the member's bytes instead of interpreting its
	// escapes, so this walk sees exactly what the caller sent while still
	// getting the request's own field names for free. Sorted, so a body with
	// two offending members always names the same one.
	var members map[string]json.RawMessage
	if err := json.Unmarshal(raw, &members); err == nil {
		for _, name := range slices.Sorted(maps.Keys(members)) {
			if memberReason := rawTextReason(members[name]); memberReason != "" {
				fieldErr.Field = name
				fieldErr.Reason = memberReason
				break
			}
		}
	}

	return httpapi.Validation(
		"The request body carries text holzkube will not repair, so the schematic was not created.",
		fieldErr)
}

// rawTextReason names the class of a value the JSON decoder would rewrite
// rather than reject, or returns the empty string.
//
// Like every other refusal on this route it names the class and never the
// value: name, cluster, kernel_args and meta can all carry a secret, and a
// problem body is rendered in a browser, may be logged by a proxy, and outlives
// the form that produced it (T-02-64).
func rawTextReason(b []byte) string {
	if !utf8.Valid(b) {
		return "contains a byte sequence that is not valid UTF-8, which the JSON decoder would rewrite to U+FFFD"
	}

	for i := 0; i+1 < len(b); {
		if b[i] != '\\' {
			i++
			continue
		}
		if b[i+1] != 'u' {
			// An escaped backslash, quote or control character. Stepping over
			// both bytes is what keeps a literal \\u sequence from being read
			// as an escape.
			i += 2
			continue
		}
		cp, ok := hexQuad(b[i+2:])
		if !ok {
			// Malformed; the decoder refuses it on its own terms and says so
			// better than a guess here would.
			i += 2
			continue
		}
		switch {
		case cp >= 0xD800 && cp <= 0xDBFF:
			// A high half is only half a character. The low one has to follow
			// it immediately and as its own escape; anything else -- the end of
			// the string, an ordinary character, a second high half -- leaves
			// this one unpaired.
			paired := false
			if i+12 <= len(b) && b[i+6] == '\\' && b[i+7] == 'u' {
				if low, ok := hexQuad(b[i+8:]); ok && low >= 0xDC00 && low <= 0xDFFF {
					paired = true
				}
			}
			if !paired {
				return unpairedSurrogateReason(cp)
			}
			// A well-formed pair is an ordinary astral character and is judged
			// as one, by the refusal set and not by its encoding.
			i += 12
		case cp >= 0xDC00 && cp <= 0xDFFF:
			return unpairedSurrogateReason(cp)
		default:
			i += 6
		}
	}
	return ""
}

func unpairedSurrogateReason(cp rune) string {
	return fmt.Sprintf("contains the unpaired surrogate %U, half of a character whose other half never arrived", cp)
}

// hexQuad reads the four hex digits of a \uXXXX escape.
//
// Assembled digit by digit rather than through strconv, so the result is a rune
// by construction and never a widening conversion from a type that could hold
// more than four hex digits' worth.
func hexQuad(b []byte) (rune, bool) {
	if len(b) < 4 {
		return 0, false
	}
	var cp rune
	for _, c := range b[:4] {
		var digit rune
		switch {
		case c >= '0' && c <= '9':
			digit = rune(c - '0')
		case c >= 'a' && c <= 'f':
			digit = rune(c-'a') + 10
		case c >= 'A' && c <= 'F':
			digit = rune(c-'A') + 10
		default:
			return 0, false
		}
		cp = cp<<4 | digit
	}
	return cp, true
}

// createProblem maps a failure from the authoring path.
//
// That path has two kinds of failure with two different owners, and the split is
// the whole point of this function.
//
// A value holzkube's own canonical serialiser will not render is the operator's
// input. It is raised by Schematic.ID() before the catalog is fetched and before
// any POST, so no request was made, nothing is known about the Factory, and no
// retry can help: it is a 400 naming the field. The 18ms 502 recorded in
// 02-UAT.md as G-02-6 is exactly what a missing branch here looks like from a
// browser -- a sentence blaming a third party for a refusal that never left this
// process.
//
// An unknown extension name is likewise the operator's mistake and is reported
// per name, so three typos are one round trip rather than three.
//
// Anything else came from the Factory and belongs to factoryProblem's upstream
// family.
//
// The NotRepresentableError branch is a backstop rather than the route's first
// answer, and deliberately kept as one. schematicInput.refuseUnrepresentable now
// asks the same predicate about every scalar the request vocabulary knows, so a
// refusal usually arrives before Author runs. What is left for this branch is
// everything that vocabulary does not enumerate -- a document scalar a future
// field adds without a matching check, and an in-process caller that never went
// through validate at all. Deleting it would make the first of those a 502
// blaming factory.talos.dev for a value that never left the process, which is
// the whole of G-02-6.
func createProblem(err error) *httpapi.Problem {
	var refused *imagefactory.NotRepresentableError
	if errors.As(err, &refused) {
		fieldErr := httpapi.FieldError{Reason: refusalReason(refused)}
		// A path this handler does not recognise is still not the Factory's
		// fault. Falling back to a 502 would tell the operator to retry
		// something that can never succeed, which is the specific harm G-02-6
		// is about; falling back to a 400 with no field name says less, and
		// says it truthfully.
		if field, ok := requestFieldForPath[refused.Path]; ok {
			fieldErr.Field = field
		}
		return httpapi.Validation(
			"The schematic carries a value holzkube cannot serialise, so it was never sent to the Image Factory.",
			fieldErr)
	}

	var unknown *imagefactory.UnknownExtensionsError
	if errors.As(err, &unknown) {
		errs := make([]httpapi.FieldError, 0, len(unknown.Names))
		for _, name := range unknown.Names {
			errs = append(errs, httpapi.FieldError{
				Field:  "extensions",
				Reason: name + " is not in the catalog for this Talos version",
			})
		}
		return httpapi.Validation(
			"Every extension is checked against the catalog for the selected Talos version before the schematic is created.",
			errs...)
	}
	return factoryProblem(err, "creating the schematic")
}

// factoryProblem maps an imagefactory error onto the upstream taxonomy.
//
// The split is the point. ErrSchematicNotBuildable means the Factory answered
// and refused, which is a statement about the request; everything else means it
// did not answer usably, which says nothing about the request and is retryable.
// Merging them sends an operator to fix a schematic that is not broken. Neither
// is ever internal.unexpected -- that would tell the operator their own
// installation is at fault for somebody else's outage.
func factoryProblem(err error, doing string) *httpapi.Problem {
	if errors.Is(err, imagefactory.ErrSchematicNotBuildable) {
		return httpapi.Upstream(httpapi.CodeUpstreamFactoryRejected,
			"The Image Factory refused: "+doing+".")
	}
	if errors.Is(err, imagefactory.ErrSchematicIDMismatch) {
		// The Factory answered, and a retry reproduces the identical mismatch:
		// the canonical serialisers have drifted and nothing about asking
		// again changes that. Reporting it as factory-unavailable would tell
		// the operator to retry, and every retry orphans another schematic.
		return httpapi.Upstream(httpapi.CodeUpstreamFactoryRejected,
			"The Image Factory assigned a different id than the one computed here: "+doing+
				". The schematic was created and has been recorded; do not retry.")
	}
	return httpapi.Upstream(httpapi.CodeUpstreamFactoryUnavailable,
		"The Image Factory did not answer usably: "+doing+".")
}

// factoryNotConfigured is the answer when this instance has no Factory client.
//
// It is a 502 rather than a panic. The composition root always builds one, so
// reaching this is a wiring mistake -- and the shape of that mistake is a Deps
// field assigned after the struct literal, which is copied by value into every
// Routes call and therefore leaves a nil here with no compile error.
func factoryNotConfigured() *httpapi.Problem {
	return httpapi.Upstream(httpapi.CodeUpstreamFactoryUnavailable,
		"This instance has no Image Factory client configured.")
}

func notFoundSchematic() *httpapi.Problem {
	return httpapi.NotFound("notfound.schematic", "No such schematic.")
}

// isTalosVersion is the shape check applied before a version reaches a Factory
// URL path or a stored record. The imagefactory client repeats it at its own
// door; this one exists so a malformed value is a 400 naming the field rather
// than a 502 naming an upstream that was never asked.
func isTalosVersion(v string) bool {
	if v == "" || v[0] != 'v' || len(v) > 64 {
		return false
	}
	digits, dots := 0, 0
	for i := 1; i < len(v); i++ {
		c := v[i]
		switch {
		case c >= '0' && c <= '9':
			digits++
		case c == '.':
			if digits == 0 {
				return false
			}
			dots++
			digits = 0
		case c == '-':
			// The prerelease tail. It is bounded by the same character set the
			// Factory accepts in a path segment, and nothing in it may
			// introduce a new path element.
			return dots == 2 && digits > 0 && isPrereleaseTail(v[i+1:])
		default:
			return false
		}
	}
	return dots == 2 && digits > 0
}

func isPrereleaseTail(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '.':
		default:
			return false
		}
	}
	return true
}
