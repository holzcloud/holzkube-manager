// The tables in this file and their mirror in installer_test.go are one
// assertion split across two call sites: for every status the live Factory has
// been observed returning, ProbeBuildable and resolveInstallerRepo must reach
// the same verdict. Before G-02-5 they did not -- the probe mapped the whole
// 4xx class to "this schematic cannot be built" while the resolver counted only
// an exact 404 -- so a 400 made the refusal verdict nearly unreachable in one
// place and a 429 became a permanent accusation in the other. A failure here
// reads as the two disagreeing, which is the defect, not as one of them being
// wrong in isolation.
//
// One consequence is worth stating where it will be read rather than only in a
// summary. Moving 401, 403 and 429 out of the refused bucket and into the
// did-not-answer bucket means strictly fewer answers stamp a probe verdict, so
// more records legitimately end their creation with no verdict at all. That is
// the correct classification and it makes the population the open
// 02-DECISION-probe-budget.md is about larger. It is not something to
// compensate for by widening what counts as a refusal: the whole point is that
// a throttled registry has said nothing about the schematic.
package imagefactory_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/imagefactory"
)

// registryAnswer is one row of the taxonomy: a status the Factory or the
// registry answers with, and what that answer means about the schematic.
type registryAnswer struct {
	status int
	// refused is true when the answer is a statement about the schematic
	// (ErrSchematicNotBuildable) and false when it is the upstream declining to
	// answer us (ErrUpstreamUnavailable).
	refused bool
	// why is the reason the row is on the side it is on, so a future reader
	// changing one of these has to argue with a sentence rather than a bool.
	why string
}

// registryAnswerTable is the package's taxonomy, asserted against both
// classification sites. The statuses are the ones observed against
// factory.talos.dev (02-UAT.md G-02-5) plus the ones its documented behaviour
// makes reachable.
//
// It is declared once and consumed twice on purpose: two tables would drift the
// same way the two switches did.
var registryAnswerTable = []registryAnswer{
	{status: 200, refused: false, why: "the answer, and the only one that means usable"},
	{status: 400, refused: true, why: "the Factory answered: an extension in the schematic is not available at this version"},
	{status: 401, refused: false, why: "an authentication challenge is about us, not about the schematic"},
	{status: 403, refused: false, why: "a policy refusal is about us, not about the schematic"},
	{status: 404, refused: true, why: "the registry answered: no manifest under that name"},
	{status: 429, refused: false, why: "factory.talos.dev throttles; a rate limit recorded as a refusal is a permanent, unclearable accusation"},
	{status: 500, refused: false, why: "the upstream failed to answer the question"},
	{status: 503, refused: false, why: "the upstream failed to answer the question"},
}

// wantErr is the sentinel this answer must produce. A 2xx produces none.
func (a registryAnswer) wantErr() error {
	switch {
	case a.status/100 == 2:
		return nil
	case a.refused:
		return imagefactory.ErrSchematicNotBuildable
	default:
		return imagefactory.ErrUpstreamUnavailable
	}
}

func (a registryAnswer) name() string { return strconv.Itoa(a.status) }

// TestProbeBuildableClassifiesEveryRegistryAnswer is half of the agreement; the
// other half is TestResolveInstallerRepoClassifiesEveryRegistryAnswer.
func TestProbeBuildableClassifiesEveryRegistryAnswer(t *testing.T) {
	for _, answer := range registryAnswerTable {
		t.Run(answer.name(), func(t *testing.T) {
			fake := newFakeFactory(t)
			fake.setISOStatus(answer.status)
			client := newClient(t, fake.URL)

			err := client.ProbeBuildable(t.Context(), schematicA, catalogVersion, imagefactory.ArchAMD64)

			want := answer.wantErr()
			if want == nil {
				if err != nil {
					t.Fatalf("HTTP %d produced %v, want no error (%s)", answer.status, err, answer.why)
				}
				return
			}
			if !errors.Is(err, want) {
				t.Fatalf("HTTP %d produced %v, want %v (%s)", answer.status, err, want, answer.why)
			}
			// The two sentinels are mutually exclusive statements; an error
			// wrapping both would let a caller read whichever it looked for.
			other := imagefactory.ErrUpstreamUnavailable
			if errors.Is(want, imagefactory.ErrUpstreamUnavailable) {
				other = imagefactory.ErrSchematicNotBuildable
			}
			if errors.Is(err, other) {
				t.Errorf("HTTP %d produced an error that is also %v", answer.status, other)
			}
			if !strings.Contains(err.Error(), answer.name()) {
				t.Errorf("the error does not carry the status the Factory answered (%d): %v", answer.status, err)
			}
		})
	}
}

// TestProbeBuildableKeepsAThrottledFactoryRetryable is the row that motivated
// the whole change, asserted on its own because it is the one an operator ever
// notices: the probe verdict is written once at creation and there is no
// re-probe path, so a 429 recorded as a refusal is permanent.
func TestProbeBuildableKeepsAThrottledFactoryRetryable(t *testing.T) {
	fake := newFakeFactory(t)
	fake.setISOStatus(429)
	client := newClient(t, fake.URL)

	err := client.ProbeBuildable(t.Context(), schematicA, catalogVersion, imagefactory.ArchAMD64)
	if errors.Is(err, imagefactory.ErrSchematicNotBuildable) {
		t.Fatalf("a throttled Factory was recorded as a schematic that cannot be built: %v", err)
	}
	if !errors.Is(err, imagefactory.ErrUpstreamUnavailable) {
		t.Fatalf("err = %v, want ErrUpstreamUnavailable", err)
	}
}
