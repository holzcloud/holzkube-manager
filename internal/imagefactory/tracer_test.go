package imagefactory_test

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube/internal/imagefactory"
)

// The two (payload, id) pairs recorded against the live Factory. They are the
// specification for the local id computation: if the implementation disagrees
// with a fixture, the implementation is wrong and the fixture does not move.
const (
	// emptySchematicID is the well-known id of a schematic with no
	// customization. Its canonical form is "customization: {}\n", which is why
	// it is not the hash of an empty document.
	emptySchematicID = "376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba"

	// consoleSchematicID was captured from a live POST of the console=ttyS0
	// plus intel-ucode plus iscsi-tools payload.
	consoleSchematicID = "20e64852c1be21e6c5e22cafc52c2dcc5add07e66ce62e30fad173d709d5b652"
)

// goodSchematic is the recorded payload behind consoleSchematicID.
func goodSchematic() imagefactory.Schematic {
	return imagefactory.Schematic{
		Customization: imagefactory.Customization{
			ExtraKernelArgs: []string{"console=ttyS0"},
			SystemExtensions: imagefactory.SystemExtensions{
				OfficialExtensions: []string{"siderolabs/intel-ucode", "siderolabs/iscsi-tools"},
			},
		},
	}
}

// TestSchematicIDMatchesRecordedFixtures pins the local computation against
// what the Factory actually assigned. Both pairs were captured from live POSTs;
// a change to the canonical serialiser that breaks either one has broken
// FACT-06, whose whole value is recognising a schematic without a round trip.
func TestSchematicIDMatchesRecordedFixtures(t *testing.T) {
	cases := []struct {
		name      string
		schematic imagefactory.Schematic
		wantID    string
		wantDoc   string
	}{
		{
			name:      "empty schematic",
			schematic: imagefactory.Schematic{},
			wantID:    emptySchematicID,
			wantDoc:   "customization: {}\n",
		},
		{
			name:      "console and two extensions",
			schematic: goodSchematic(),
			wantID:    consoleSchematicID,
			wantDoc: "customization:\n" +
				"    extraKernelArgs:\n" +
				"        - console=ttyS0\n" +
				"    systemExtensions:\n" +
				"        officialExtensions:\n" +
				"            - siderolabs/intel-ucode\n" +
				"            - siderolabs/iscsi-tools\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := tc.schematic.Canonical()
			if err != nil {
				t.Fatalf("Canonical: %v", err)
			}
			if string(doc) != tc.wantDoc {
				t.Errorf("canonical document\n got: %q\nwant: %q", doc, tc.wantDoc)
			}

			id, err := tc.schematic.ID()
			if err != nil {
				t.Fatalf("ID: %v", err)
			}
			if id != tc.wantID {
				t.Errorf("ID = %s, want %s (the fixture is the specification; do not adjust it)", id, tc.wantID)
			}
		})
	}
}

// TestTracerAuthorsAUsableSchematic walks the one path this plan wires end to
// end: compose, precompute, validate against the version-scoped catalog, POST,
// confirm the returned id, probe the build, and only then call it usable.
func TestTracerAuthorsAUsableSchematic(t *testing.T) {
	fake := newFakeFactory(t)
	client := newClient(t, fake.URL)
	ctx := t.Context()

	schematic := goodSchematic()

	// Precomputed before anything leaves the process.
	precomputed, err := schematic.ID()
	if err != nil {
		t.Fatalf("precompute id: %v", err)
	}
	if precomputed != consoleSchematicID {
		t.Fatalf("precomputed id = %s, want the recorded %s", precomputed, consoleSchematicID)
	}

	authored, err := imagefactory.Author(ctx, client, imagefactory.AuthorRequest{
		TalosVersion: catalogVersion,
		Arch:         imagefactory.ArchAMD64,
		Schematic:    schematic,
	})
	if err != nil {
		t.Fatalf("Author: %v", err)
	}

	if authored.ID != precomputed {
		t.Errorf("Factory id %s does not match the locally precomputed %s", authored.ID, precomputed)
	}
	if !authored.Usable {
		t.Error("a schematic whose probe succeeded was not marked usable")
	}
	if authored.Canonical == "" {
		t.Error("the Factory's own document was not returned; it is what a caller persists")
	}

	// Every layer was actually traversed. Without this the test would pass on a
	// client that skipped the catalog and got lucky.
	for _, shape := range []string{"GET /version/*/extensions/official", "POST /schematics", "* /image/*"} {
		if got := fake.count(shape); got != 1 {
			t.Errorf("%s was requested %d times, want exactly 1", shape, got)
		}
	}
}

// TestTracerRejectsAnUnknownExtensionBeforePosting is the first half of P9: the
// extension name is checked against the version-scoped catalog, and a name that
// is not in it never reaches a POST at all.
//
// The zero-POST assertion is the load-bearing one. A client that POSTed first
// and validated afterwards would still report an error here and would still be
// wrong, because the Factory would have created the schematic.
func TestTracerRejectsAnUnknownExtensionBeforePosting(t *testing.T) {
	fake := newFakeFactory(t)
	client := newClient(t, fake.URL)

	schematic := imagefactory.Schematic{
		Customization: imagefactory.Customization{
			SystemExtensions: imagefactory.SystemExtensions{
				OfficialExtensions: []string{"siderolabs/intel-ucode", "siderolabs/totally-not-real-ext"},
			},
		},
	}

	_, err := imagefactory.Author(t.Context(), client, imagefactory.AuthorRequest{
		TalosVersion: catalogVersion,
		Arch:         imagefactory.ArchAMD64,
		Schematic:    schematic,
	})
	if !errors.Is(err, imagefactory.ErrExtensionUnknown) {
		t.Fatalf("Author error = %v, want ErrExtensionUnknown", err)
	}
	if got := err.Error(); !strings.Contains(got, "siderolabs/totally-not-real-ext") {
		t.Errorf("the error does not name the offending extension: %s", got)
	}

	if got := fake.count("POST /schematics"); got != 0 {
		t.Errorf("POST /schematics was called %d times for an extension that failed validation; "+
			"validation must happen before creation, not after", got)
	}
}

// TestTracerDoesNotCallACreatedSchematicUsable is the second half of P9, and
// the reason FACT-02 exists. The Factory accepts a schematic naming an
// extension that does not exist and hands back an ordinary id; the ISO for it
// answers 400. Nothing before the probe had any reason to complain, which is
// exactly why the probe decides.
func TestTracerDoesNotCallACreatedSchematicUsable(t *testing.T) {
	fake := newFakeFactory(t)
	client := newClient(t, fake.URL)
	ctx := t.Context()

	bogus := imagefactory.Schematic{
		Customization: imagefactory.Customization{
			SystemExtensions: imagefactory.SystemExtensions{
				OfficialExtensions: []string{"siderolabs/totally-not-real-ext"},
			},
		},
	}

	// Bypass Author deliberately: this reproduces what a client that trusted
	// the POST would have done, and proves the fake really does accept it.
	created, err := client.CreateSchematic(ctx, bogus)
	if err != nil {
		t.Fatalf("the fake refused a bogus schematic; it must accept it, as the real Factory does: %v", err)
	}
	if created.ID == "" {
		t.Fatal("creation returned no id")
	}

	probeErr := client.ProbeBuildable(ctx, created.ID, catalogVersion, imagefactory.ArchAMD64)
	if !errors.Is(probeErr, imagefactory.ErrSchematicNotBuildable) {
		t.Fatalf("probe error = %v, want ErrSchematicNotBuildable", probeErr)
	}

	// And through the composed path, the verdict is the one that matters.
	usable := probeErr == nil
	if usable {
		t.Error("a schematic whose POST succeeded and whose ISO answered 400 was marked usable")
	}
}

// TestTracerVersionsIncludeThePrereleaseTail guards the other half of P9(d):
// the version list ends in pre-releases, so "the latest version" is not the
// last element, and the client does not quietly filter them either.
func TestTracerVersionsIncludeThePrereleaseTail(t *testing.T) {
	fake := newFakeFactory(t)
	client := newClient(t, fake.URL)

	versions, err := client.Versions(t.Context())
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}
	if len(versions) < 100 {
		t.Fatalf("got %d versions, want the full recorded list", len(versions))
	}
	last := versions[len(versions)-1]
	if !strings.Contains(last, "-rc.") {
		t.Errorf("last version is %q; the recording ends in a release candidate, and a client "+
			"that treats the last element as latest would offer it", last)
	}
	for _, want := range []string{"v1.13.9", "v1.14.0-rc.2"} {
		if !slices.Contains(versions, want) {
			t.Errorf("version list does not contain %s", want)
		}
	}
}

func newClient(t *testing.T, baseURL string) *imagefactory.Client {
	t.Helper()
	c, err := imagefactory.New(baseURL)
	if err != nil {
		t.Fatalf("New(%q): %v", baseURL, err)
	}
	return c
}

// TestTracerSeparatesAnUnreachableProbeFromABadSchematic keeps the two verdicts
// apart. A Factory that answers 500 while probing has said nothing about the
// schematic, and reporting that as "not buildable" sends an operator to fix a
// schematic that may be perfectly good.
func TestTracerSeparatesAnUnreachableProbeFromABadSchematic(t *testing.T) {
	fake := newFakeFactory(t)
	client := newClient(t, fake.URL)
	ctx := t.Context()

	created, err := client.CreateSchematic(ctx, goodSchematic())
	if err != nil {
		t.Fatalf("CreateSchematic: %v", err)
	}

	fake.setISOStatus(http.StatusInternalServerError)
	err = client.ProbeBuildable(ctx, created.ID, catalogVersion, imagefactory.ArchAMD64)

	if !errors.Is(err, imagefactory.ErrUpstreamUnavailable) {
		t.Errorf("probe error = %v, want ErrUpstreamUnavailable", err)
	}
	if errors.Is(err, imagefactory.ErrSchematicNotBuildable) {
		t.Error("an upstream 500 was reported as an un-buildable schematic")
	}
}
