package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

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
type assetReferences struct {
	ISO       string `json:"iso"`
	PXE       string `json:"pxe"`
	DiskImage string `json:"disk_image"`
	Cmdline   string `json:"cmdline"`
	Installer string `json:"installer"`
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

		var in schematicInput
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
			Canonical:    authored.Canonical,
			Extensions:   in.Extensions,
			KernelArgs:   in.KernelArgs,
			Meta:         in.meta(),
			Usable:       authored.Usable,
			ProbedAt:     probedAt,
			ProbeReason:  probeReason,
			CreatedAt:    time.Now().UTC(),
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
		// answers with no reference at all rather than a plausible string.
		installer, err := d.Factory.InstallerImage(r.Context(), req)
		if err != nil {
			httpapi.WriteProblem(w, r, factoryProblem(err,
				"resolving the installer image reference for "+req.Version))
			return
		}

		writeJSON(w, http.StatusOK, assetReferences{
			ISO:       iso,
			PXE:       pxe,
			DiskImage: disk,
			Cmdline:   cmdline,
			Installer: installer,
		})
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
	if len(errs) == 0 {
		return nil
	}
	return httpapi.Validation("The schematic is not valid.", errs...)
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
// someone else's machine (FACT-03). version defaults to the version the record
// was authored and probed against, which is the only version this installation
// has any evidence about; platform defaults to the sole member of a closed type.
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
		return "entry " + strconv.Itoa(e.Index+1) + " " + e.Reason
	}
	return e.Reason
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
