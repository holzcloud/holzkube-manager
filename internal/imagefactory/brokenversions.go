package imagefactory

// brokenVersions lists Talos versions holzkube-manager greys out, mapped to the reason
// each one is listed.
//
// Curated and embedded rather than fetched or UI-editable (D-08). A list an
// operator edits in one installation strands the knowledge inside that
// installation: the next deployment, and the next operator, rediscover the same
// broken version the hard way. In the binary it is reviewable in git, arrives
// with an upgrade, and carries its evidence.
//
// The reason string is the whole value of this table. A version greyed out with
// no stated cause is a version nobody can un-grey, because nobody can tell
// whether the cause still holds. An entry without a reviewable reason is worse
// than no entry, which is why the internal guard test refuses one.
//
// The table is currently EMPTY, and that is a finding rather than an omission.
// The research this plan was written from (PITFALLS.md P9(d)) recorded exactly
// one candidate: v1.9.0, where the platform-prefixed installer repository was
// observed not to resolve. That observation was re-probed against
// factory.talos.dev while this file was written and it no longer holds --
// GET /v2/metal-installer/<id>/manifests/v1.9.0 answers 200. Listing v1.9.0
// anyway would put a claim in the table that the table itself is supposed to
// make checkable, and inventing plausible-looking entries to make the feature
// look populated is the failure mode this design is meant to avoid.
//
// What belongs here, when it is found: a version where holzkube-manager has *observed*
// a reproducible failure -- an asset that does not build, an installer
// reference that does not resolve across the supported range, an upgrade path
// that is known to strand a node. Add the version, the reason, and the date
// and method of the observation. The mechanism below is proven by
// brokenversions_internal_test.go against a synthetic table, so the first real
// entry needs nothing but the entry.
var brokenVersions = map[string]string{}

// BrokenReason reports whether a Talos version is known broken, and why.
//
// The reason is returned, not just the verdict: a UI that greys out a version
// without saying why leaves an operator with a dead control and no way to
// judge whether it still applies to them.
func BrokenReason(version string) (string, bool) {
	return brokenReason(brokenVersions, version)
}

// brokenReason is the lookup, taking the table as a parameter so it is testable
// independently of what happens to be curated today.
func brokenReason(table map[string]string, version string) (string, bool) {
	reason, ok := table[version]
	return reason, ok
}

// BrokenIn returns the subset of versions that are known broken, each mapped to
// the reason it is listed.
//
// The result is never nil, so a JSON encoding of "nothing is currently known
// broken" is {} rather than null -- an empty object says the server checked and
// found nothing, a null says nothing at all. The projection lives here rather
// than in the caller because it is a statement about the curated table, and a
// handler that looped over versions itself would be a second place that has to
// be found when the table's shape changes.
func BrokenIn(versions []string) map[string]string {
	broken := make(map[string]string, 0)
	for _, v := range versions {
		if reason, ok := BrokenReason(v); ok {
			broken[v] = reason
		}
	}
	return broken
}
