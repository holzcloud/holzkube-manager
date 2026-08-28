package imagefactory_test

import (
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/holzcloud/holzkube/internal/imagefactory"
)

// liveEnv opts a run in to the contract test against the real Image Factory.
const liveEnv = "HOLZKUBE_FACTORY_LIVE"

// TestLiveFactory is the fake-drift guard for this package: it checks that the
// recordings in testdata/ and the behaviours reproduced in fake_test.go still
// describe factory.talos.dev.
//
// Every other test in this package runs against a fake, which means every other
// test would keep passing after the upstream changed. This is the one that
// would not -- and it is the counterpart of the method-coverage test that
// guards the talossim fake in the sibling track.
//
// It skips loudly, in the idiom internal/depguard_test.go uses for the same
// reason: a skipped upstream check that prints nothing is indistinguishable
// from a check that passed, and a guard nobody can see is a guard nobody runs.
func TestLiveFactory(t *testing.T) {
	if os.Getenv(liveEnv) != "1" {
		t.Skipf("skipping the live Image Factory contract test: %s is not set to 1. "+
			"Nothing in this run verified that testdata/versions.json, "+
			"testdata/extensions-%s.json or the recorded schematic id %s still match "+
			"factory.talos.dev, so a fake that has drifted from the real upstream would "+
			"look exactly like this.", liveEnv, catalogVersion, consoleSchematicID)
	}

	client, err := imagefactory.New(imagefactory.DefaultBaseURL, imagefactory.WithTimeout(60*time.Second))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := t.Context()

	t.Run("versions still contain the pinned Talos release", func(t *testing.T) {
		versions, err := client.Versions(ctx)
		if err != nil {
			t.Fatalf("Versions: %v", err)
		}
		if !slices.Contains(versions, catalogVersion) {
			t.Errorf("the live version list no longer contains %s; the recorded fixture and "+
				"holzkube's supported range disagree with upstream", catalogVersion)
		}
	})

	t.Run("the version-scoped catalog still lists the recorded extension", func(t *testing.T) {
		catalog, err := client.Extensions(ctx, catalogVersion)
		if err != nil {
			t.Fatalf("Extensions(%s): %v", catalogVersion, err)
		}
		if !slices.Contains(imagefactory.ExtensionNames(catalog), "siderolabs/intel-ucode") {
			t.Errorf("the live catalog for %s no longer lists siderolabs/intel-ucode", catalogVersion)
		}
	})

	t.Run("the recorded payload still produces the recorded id", func(t *testing.T) {
		// The strongest assertion in this file: it proves the canonical
		// serialisation written in schematicid.go still agrees with upstream's,
		// which is the whole of FACT-06. CreateSchematic already refuses a
		// mismatch, so a drift shows up as ErrSchematicIDMismatch.
		created, err := client.CreateSchematic(ctx, goodSchematic())
		if errors.Is(err, imagefactory.ErrSchematicIDMismatch) {
			t.Fatalf("the canonical serialiser has drifted from upstream: %v", err)
		}
		if err != nil {
			t.Fatalf("CreateSchematic: %v", err)
		}
		if created.ID != consoleSchematicID {
			t.Errorf("live id = %s, want the recorded %s", created.ID, consoleSchematicID)
		}
	})

	t.Run("creation is still not validation", func(t *testing.T) {
		// The trap the fake reproduces. If upstream ever starts rejecting a
		// bogus extension at POST time this fails, and the day it does is the
		// day the probe stops being load-bearing -- which is worth knowing
		// rather than assuming in either direction.
		bogus := imagefactory.Schematic{Customization: imagefactory.Customization{
			SystemExtensions: imagefactory.SystemExtensions{
				OfficialExtensions: []string{"siderolabs/totally-not-real-ext"},
			},
		}}
		created, err := client.CreateSchematic(ctx, bogus)
		if err != nil {
			t.Fatalf("upstream refused a bogus extension at creation time (a behaviour change "+
				"worth investigating, not a bug in this test): %v", err)
		}
		probeErr := client.ProbeBuildable(ctx, created.ID, catalogVersion, imagefactory.ArchAMD64)
		if !errors.Is(probeErr, imagefactory.ErrSchematicNotBuildable) {
			t.Errorf("probing a schematic with a non-existent extension returned %v, "+
				"want ErrSchematicNotBuildable", probeErr)
		}
	})

	t.Run("a good schematic still builds", func(t *testing.T) {
		if err := client.ProbeBuildable(ctx, consoleSchematicID, catalogVersion, imagefactory.ArchAMD64); err != nil {
			t.Errorf("ProbeBuildable on the recorded schematic: %v", err)
		}
	})
}
