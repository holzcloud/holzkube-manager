package imagefactory

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// legacyInstallerRepo is the repository name the Factory published before the
// platform-prefixed one existed, documented upstream as the "Legacy installer
// Image". It is still the only name that resolves for part of the supported
// version range.
const legacyInstallerRepo = "installer"

// installerRepoRetryInterval is how long a *provisional* installer repository
// answer -- one obtained without ruling out the preferred candidate -- is
// served before that candidate is asked again. A proven answer never expires.
//
// This is not a request budget and it moves no deadline. It bounds how often an
// unproven answer is re-asked, and it exists because the two obvious
// alternatives are both worse:
//
//   - Freeze it, as this cache used to. That is G-02-3: two processes served
//     two different repository names for one schematic at one version, split
//     exactly at a restart, with nothing saying the preferred name had never
//     been ruled out.
//   - Never remember it, which is the UAT's literal wording ("do not cache a
//     name reached past an unanswered earlier candidate, or re-probe on the next
//     request"). That makes every subsequent assets request re-pay the silent
//     candidate's full client timeout. Get the two measured numbers the right
//     way round here, because an earlier revision of this reasoning did not:
//     43.42s is the *measured success* of one such resolution (30s for the
//     silent candidate plus 13.4s for the one that answered), while the worst
//     case is both candidates silent at 2 x DefaultTimeout = 60.000s -- the same
//     60s writeTimeout the same route has already returned a 502 at
//     (G-02-2: status=502 duration=1m0.002907792s). Either number, paid per
//     request, times out the very panel this warning exists to be rendered on.
//
// Five minutes is short enough that a registry which recovers is noticed within
// one, and long enough that the silent candidate's timeout is amortised across
// every request in between rather than paid by each of them.
//
// What this does not touch is the *cold* path: a first resolution with an empty
// cache still walks both candidates serially. Bounding that needs the per-route
// deadline that
// .planning/phases/02-transport-seam-talossim-image-factory/02-DECISION-probe-budget.md
// owns and G-02-2 is deferred to; cmd/holzkubed/budget_test.go declares the
// route as known-over-budget in the meantime. Do not "simplify" this constant
// away against the UAT's wording without reading that document first.
const installerRepoRetryInterval = 5 * time.Minute

// installerRepoEntry is one cached answer to "which repository name carries the
// installer for this platform, version and SecureBoot selection", together with
// how sure of it we are.
type installerRepoEntry struct {
	// repo is the repository name to serve.
	repo string

	// warning is what an operator should be told about how repo was obtained.
	// Zero-valued when the answer is proven.
	warning Warning

	// unresolved is the candidate names this answer never ruled out: they
	// neither answered 2xx nor refused. Empty means proven.
	//
	// This list, and not the full candidate list, is what a re-question asks.
	// That is the difference between a re-question costing 1 x DefaultTimeout
	// and 2 x DefaultTimeout, and therefore between 30s of headroom under
	// writeTimeout and none at all.
	unresolved []string

	// at is when this entry was last written or re-stamped. It is what the
	// re-question cadence is measured from, and a failed re-question re-stamps
	// it so the next one happens an interval after the failure rather than an
	// interval after the original resolution.
	at time.Time
}

// proven reports whether every candidate was accounted for, in which case the
// entry never expires and carries no warning.
func (e installerRepoEntry) proven() bool { return len(e.unresolved) == 0 }

// installerResolution is what one walk of a candidate list learned: the name to
// use, and what it could not rule out on the way there.
type installerResolution struct {
	repo string

	// unresolved is the candidates tried before repo answered that neither
	// answered nor refused -- the registry did not say.
	unresolved []string

	// unanswered is the last such candidate's error, kept rather than discarded
	// so the warning can name the transport failure as the client reported it.
	// Non-nil exactly when unresolved is non-empty.
	unanswered error
}

// secureBootRepoSuffix is what upstream's registry frontend reads as "this is
// the SecureBoot installer". siderolabs/image-factory's getRequestedImage
// parses the repository name into an image name, a platform and a secureboot
// flag, cutting "-installer-secureboot" before "-installer"; the suffix is the
// switch and there is nothing else that sets it.
const secureBootRepoSuffix = "-secureboot"

// manifestAccept is the media-type set an OCI registry expects on a manifest
// request. A registry that receives none of these may answer 404 for a manifest
// that exists, which would look exactly like a repository name that does not
// resolve -- the one thing this function must not get wrong.
const manifestAccept = "application/vnd.oci.image.index.v1+json, " +
	"application/vnd.oci.image.manifest.v1+json, " +
	"application/vnd.docker.distribution.manifest.list.v2+json, " +
	"application/vnd.docker.distribution.manifest.v2+json"

// InstallerImage returns the OCI reference of the installer image for a
// schematic at a Talos version, with the repository name proven against the
// registry rather than assumed (D-02).
//
// The repository name is version-dependent. PITFALLS P9(d) records both
// `factory.talos.dev/v2/installer/<id>/manifests/<v>` and
// `.../metal-installer/<id>/manifests/<v>` answering for one version while only
// one of them answers for another. Hardcoding either name across the supported
// range is a bug with the worst possible failure mode: the reference is
// consumed by the upgrade RPC, and a wrong one produces an upgrade that reports
// success and silently drops every system extension the node was built with
// (P9(c)).
//
// So the platform-prefixed name is tried first and the legacy name second, and
// if neither answers this returns an error and no reference at all. There is no
// guess: a caller that receives an error here must not upgrade.
//
// The returned warnings say how sure of the name we are. A name reached past a
// candidate that never answered is usable but provisional -- the preferred name
// was unheard, not ruled out -- and carries
// WarningInstallerRepoFallbackUnverified on every answer, including the ones
// served from the cache. A proven name carries nothing. The slice is empty and
// never nil, for the reason Warnings already gives: a JSON null reads to a
// client as "the server did not check".
func (c *Client) InstallerImage(ctx context.Context, r AssetRequest) (string, []Warning, error) {
	warnings := make([]Warning, 0, 1)

	if err := r.validate(); err != nil {
		return "", warnings, err
	}

	entry, err := c.installerRepo(ctx, r)
	if err != nil {
		return "", warnings, err
	}
	if entry.warning.Code != "" {
		warnings = append(warnings, entry.warning)
	}
	repo := entry.repo

	// The registry host is the Factory this client was configured with, not a
	// constant: a deployment pointing at a private Factory must produce
	// references to that Factory, or every node it provisions pulls its
	// installer from somewhere the operator never configured.
	//
	// The result is an OCI image reference -- registry, repository, tag -- and
	// not a network endpoint. It is assembled by concatenation rather than by a
	// "%s:%s" format string on purpose: that shape is what the internal/talos
	// seam guard looks for, and a reference built here is exactly the kind of
	// thing the guard should keep looking at rather than learn to ignore.
	ref := c.base.Host
	if prefix := strings.Trim(c.base.Path, "/"); prefix != "" {
		ref += "/" + prefix
	}
	return ref + "/" + repo + "/" + r.SchematicID + ":" + r.Version, warnings, nil
}

// installerCandidates is the ordered list of repository names that may carry
// this request's installer: the platform-prefixed name first, the legacy name
// second, each carrying the SecureBoot suffix when the request asks for it.
//
// SecureBoot is in here because the repository name is the only thing that
// selects it. A schematic does not carry SecureBoot into its installer -- at
// v1.13.9 the same schematic id resolves to sha256:f960382f... under
// metal-installer and to sha256:878b171c... under metal-installer-secureboot,
// two different images picked by name alone (02-UAT.md G-02-4). Talos requires
// the SecureBoot installer for a SecureBoot install: it carries the signed UKI
// and systemd-boot, there is no machine-config flag that substitutes for it,
// and the ordinary installer does not produce a SecureBoot node. Pairing a
// SecureBoot ISO with the ordinary installer is the ISO/installer drift this
// file's own comments warn about, arriving from the one direction nothing
// checked.
//
// So a SecureBoot request asks only about SecureBoot names, and when neither
// answers the caller is refused with no reference rather than handed the
// ordinary installer. That refusal is deliberate and the trade-off is real: at
// a Talos version where only the ordinary names resolve, an asset panel that
// renders five references today would answer 502 with none. Substituting is
// still not an option -- it reintroduces exactly the drift this function exists
// to stop, silently, in the one place an operator cannot see it. Whether such a
// version exists inside the supported range is settled by TestLiveFactory's
// installer-name matrix subtest, not by anything the fakes can tell you.
func installerCandidates(r AssetRequest) []string {
	suffix := ""
	if r.SecureBoot {
		suffix = secureBootRepoSuffix
	}
	return []string{
		string(r.Platform) + "-" + legacyInstallerRepo + suffix,
		legacyInstallerRepo + suffix,
	}
}

// installerRepo returns the repository name that resolves for this request,
// consulting the cache first.
//
// The cache is keyed on platform, Talos version and the SecureBoot selection,
// because that is what the name depends on -- not on the schematic. Every
// schematic at that combination shares one answer, so a wizard that derives
// assets for several schematics at one version costs one manifest request
// rather than one per schematic (T-02-22); SecureBoot at worst doubles the
// number of combinations, it does not make the saving per-schematic. The
// platform is in the key because the modern name is the platform-prefixed one,
// so two platforms at one version are two different questions, and SecureBoot
// is in it for the same reason: a shared entry would answer a SecureBoot
// request with the ordinary installer it never asked about (T-02-43). The flag
// is formatted rather than conditionally concatenated so two different requests
// cannot produce the same key bytes.
//
// A failed resolution is deliberately not cached. The cache exists to spare
// requests, not to freeze an answer that was never obtained: remembering a
// transient registry outage as "no installer exists" would block upgrades long
// after the registry recovered.
//
// An answer obtained without ruling out the preferred name is neither a failure
// nor a resolution, and it gets its own treatment. Freezing one for the life of
// the process is what produced two processes disagreeing about one schematic at
// one version (G-02-3), so such an entry is marked provisional, is served with
// the warning that says so, and is re-questioned once it is older than
// installerRetry.
//
// A failed *re*-question is not a failed resolution either. A usable answer is
// already in hand, and discarding it would reopen a question the registry never
// asked us to reopen -- turning a throttled registry into a 502 on the assets
// route one interval after the throttling started, for an operator who was
// being served a reference a moment earlier. A first resolution with nothing to
// fall back on still fails and is still not cached; that is unchanged.
func (c *Client) installerRepo(ctx context.Context, r AssetRequest) (installerRepoEntry, error) {
	key := installerRepoKey(r)

	c.installerMu.Lock()
	entry, ok := c.installerRepos[key]
	c.installerMu.Unlock()

	if ok {
		if entry.proven() || time.Since(entry.at) < c.installerRetry {
			return entry, nil
		}
		return c.requestionInstallerRepo(ctx, r, key, entry), nil
	}

	res, err := c.resolveInstallerRepo(ctx, r, installerCandidates(r)...)
	if err != nil {
		return installerRepoEntry{}, err
	}

	entry = installerRepoEntry{repo: res.repo, unresolved: res.unresolved, at: time.Now()}
	if !entry.proven() {
		entry.warning = installerFallbackWarning(r, res)
	}

	return c.storeInstallerRepo(key, entry), nil
}

// installerRepoKey is the cache key. The SecureBoot flag is formatted rather
// than conditionally concatenated so two different requests cannot produce the
// same key bytes.
func installerRepoKey(r AssetRequest) string {
	return fmt.Sprintf("%s/%s/secureboot=%t", r.Platform, r.Version, r.SecureBoot)
}

// storeInstallerRepo publishes an entry under key unless a concurrent resolver
// has already published a better one, and returns the entry to serve.
//
// Every resolution in this file reads the cache, *releases the lock*, asks the
// registry for up to DefaultTimeout, and only then writes what it learned. Two
// requests on one key therefore write from observations made as much as a
// timeout apart, and an unconditional `c.installerRepos[key] = next` makes the
// slowest of them authoritative. That is not merely a lost cache update; it is
// G-02-3 reappearing inside a single process. Concretely: two requests find one
// stale provisional entry, the first proves the preferred name and stores it,
// the second times out on the same name and stores its own stale local copy
// re-stamped fresh -- so the two callers are served two different repository
// names for one schematic at one version at the same moment, and what is left
// in the cache is the fallback that had just been shown to be unnecessary.
// InstallerImage's own doc comment says what a wrong installer reference costs:
// an upgrade that reports success and silently drops every system extension the
// node was built with (P9(c)). The reverted entry is provisional, so it expires
// and can be reverted again; the state does not converge on its own.
//
// The rule is therefore one sentence: a proven entry is never overwritten. A
// proven entry is this cache's end state -- every candidate accounted for, no
// warning, no expiry -- so nothing a slower resolver can learn improves on it,
// and anything it would write in its place is strictly less certain. A caller
// that loses the race is handed the proven entry rather than its own, which is
// what makes two concurrent callers return the same reference instead of two.
//
// Note what this deliberately does not do: it does not compare timestamps. A
// compare-and-set on entry.at -- "write only if nobody moved the entry since I
// read it" -- looks like the more careful rule and is the more dangerous one.
// It makes a resolver that has just *proven* a name discard that proof because
// some concurrent failed re-question re-stamped the entry first, throwing away
// the only observation either goroutine made that ends the question. Among
// unproven entries the last write wins, and that is harmless: on one key they
// carry the same repository name, because the candidate order is fixed and the
// first 2xx wins, so all a later write moves is the re-question cadence, which
// is the one thing it is supposed to move.
//
// This does not collapse the concurrent registry calls themselves into one.
// That is a separate question -- it needs single-flighting around the whole
// resolution rather than around the write -- and it is the load pattern, not
// the correctness of the answer.
func (c *Client) storeInstallerRepo(key string, next installerRepoEntry) installerRepoEntry {
	c.installerMu.Lock()
	defer c.installerMu.Unlock()

	if current, ok := c.installerRepos[key]; ok && current.proven() {
		return current
	}
	c.installerRepos[key] = next
	return next
}

// requestionInstallerRepo asks again about a provisional entry that has gone
// stale, and returns the entry to serve.
//
// It asks only the candidates the entry never ruled out -- never the whole
// list. This is the load-bearing constraint of the whole mechanism and it must
// not be simplified into "call resolveInstallerRepo again with everything".
// Walking the full list pays every candidate's timeout: with both names silent
// that is 2 x DefaultTimeout = 60.000s against a 60s writeTimeout, which is the
// exact composition G-02-2 recorded a 502 at, and it would land on the *warm*
// path -- a route that was answering from cache in 218us would acquire a
// periodic 502, and the retention rule below could not save it because the
// answer would arrive after the socket was gone. Asking only the unresolved
// names costs 1 x DefaultTimeout = 30s, thirty seconds under the budget.
//
// The candidate that answered is not re-asked for the plainest of reasons: its
// name is already in hand and its answer is already this entry's value, so
// asking it again cannot change any decision made here.
func (c *Client) requestionInstallerRepo(ctx context.Context, r AssetRequest, key string, entry installerRepoEntry) installerRepoEntry {
	res, err := c.resolveInstallerRepo(ctx, r, entry.unresolved...)

	next := entry
	switch {
	case err == nil:
		// A candidate that was never ruled out has now answered. It is earlier
		// in the preference order than the name we were serving -- that is why
		// it was tried first -- so it wins outright.
		next = installerRepoEntry{repo: res.repo, unresolved: res.unresolved, at: time.Now()}
		if !next.proven() {
			next.warning = installerFallbackWarning(r, res)
		}

	case errors.Is(err, ErrSchematicNotBuildable):
		// Every remaining candidate refused. The name that was never ruled out
		// now is, which is precisely the question the warning asked, so the
		// entry keeps its repository name and is promoted to proven.
		//
		// That looks like trusting a stale observation, so argue it: the 2xx
		// that produced this name is exactly as fresh as any proven entry's own
		// single 2xx, and this cache has always served those forever. Promoting
		// introduces no weakness the proven path did not already have. What it
		// removes is the only thing the warning ever claimed -- that the
		// preferred name was unheard rather than ruled out.
		next = installerRepoEntry{repo: entry.repo, at: time.Now()}

	default:
		// Silent again, which is exactly what a throttling factory.talos.dev
		// produces. Keep the entry, keep its warning, and re-stamp it.
		//
		// Keeping it is prohibition #1 of plan 02-12 and the behaviour
		// 02-04-PLAN.md:157-161 deliberately chose: evicting would turn a
		// throttled registry into a 502 for an operator who was being served a
		// usable reference. Re-stamping is what defines the cadence -- leaving
		// the timestamp stale would make every subsequent call re-question, so
		// the silent candidate's full client timeout would be paid per request
		// again, which is the cost the interval exists to prevent. The next
		// re-question therefore happens one interval after this failure, and
		// asks the same single name.
		//
		// The warning is the one already stored, unchanged. It still names the
		// repository that was never ruled out and that is still true; the
		// entry's provenance is about how the name was obtained, not about the
		// most recent failure to improve on it.
		next.at = time.Now()
	}

	return c.storeInstallerRepo(key, next)
}

// installerFallbackWarning is the operator-facing sentence for a reference
// reached past a candidate that never answered.
//
// It names the repository that answered, the repository that did not, the
// version and the transport error as the client reported it, and it says that
// asking again may produce a different reference. That last clause is the
// point: it is the sentence that would have made the two-process divergence
// self-explanatory instead of a five-line UAT investigation.
func installerFallbackWarning(r AssetRequest, res installerResolution) Warning {
	return Warning{
		Code: WarningInstallerRepoFallbackUnverified,
		Detail: fmt.Sprintf(
			"This installer reference names the repository %q, which answered for %s. "+
				"The preferred repository %s did not answer at all, so it was never ruled out: %v. "+
				"The reference is usable, but it is provisional rather than proven: "+
				"once the registry is reachable again the preferred name may answer, "+
				"and the reference shown here would then change.",
			res.repo, r.Version, strings.Join(res.unresolved, " or "), res.unanswered),
	}
}

// resolveInstallerRepo asks the registry, in order, which of the candidate
// repository names carries a manifest for this schematic at this version.
//
// It keeps two verdicts apart on exactly the terms ProbeBuildable does, because
// both ask registryRefused rather than each deciding for itself. Only when every
// candidate answers with a refusal is this a statement about the schematic
// (ErrSchematicNotBuildable). Anything else -- a transport failure, a 5xx, an
// authentication challenge, a rate limit -- is the registry not answering the
// question, which says nothing about the schematic and must stay retryable
// (ErrUpstreamUnavailable). Reporting an outage as "this schematic has no
// installer" would send an operator to rebuild something that is not broken.
//
// This comment claimed that parity before G-02-5 while the switch below tested
// status == http.StatusNotFound alone, which made ErrSchematicNotBuildable
// nearly unreachable here: a single 400 anywhere in the candidate loop cleared
// refusedAll for good, and 400 is precisely what the Factory answers for a
// schematic whose extension is unavailable at the requested version.
// It takes the candidate list as an argument rather than deriving it, so the
// cold path and the re-question path are one piece of code with one classifier
// rather than two that can drift apart. The ordering rule is the same either
// way: the first 2xx wins, and the platform-prefixed name is preferred.
//
// The names it could not rule out come back with the answer instead of being
// dropped at the 2xx branch, which is what G-02-3 found it doing. Any value
// `unanswered` holds when a candidate answers 2xx came from a candidate tried
// *earlier* -- one the caller preferred and did not get an answer from -- so it
// is exactly the provenance the returned name needs to carry.
func (c *Client) resolveInstallerRepo(ctx context.Context, r AssetRequest, candidates ...string) (installerResolution, error) {
	var (
		refused     []string
		unresolved  []string
		unanswered  error
		refusedAll  = true
		lastAttempt string
	)

	for _, repo := range candidates {
		lastAttempt = repo
		u := c.base.JoinPath("v2", repo, r.SchematicID, "manifests", r.Version).String()

		status, err := c.probeStatus(ctx, http.MethodGet, u, http.Header{"Accept": []string{manifestAccept}})
		if err != nil {
			refusedAll = false
			unresolved = append(unresolved, repo)
			unanswered = fmt.Errorf("%w: resolving the installer repository %q for %s: %w",
				ErrUpstreamUnavailable, repo, r.Version, err)
			continue
		}

		switch {
		case status/100 == 2:
			return installerResolution{repo: repo, unresolved: unresolved, unanswered: unanswered}, nil
		case registryRefused(status):
			refused = append(refused, fmt.Sprintf("%s (HTTP %d)", repo, status))
		default:
			refusedAll = false
			unresolved = append(unresolved, repo)
			unanswered = fmt.Errorf("%w: resolving the installer repository %q for %s answered HTTP %d",
				ErrUpstreamUnavailable, repo, r.Version, status)
		}
	}

	if refusedAll && len(refused) > 0 {
		return installerResolution{}, fmt.Errorf("%w: no installer image for schematic %s at %s; the registry has no manifest under %s",
			ErrSchematicNotBuildable, r.SchematicID, r.Version, strings.Join(refused, " or "))
	}
	if unanswered != nil {
		return installerResolution{}, unanswered
	}
	// Unreachable with a non-empty candidate list; a nil error with no
	// reference would be read by a caller as a resolved name.
	return installerResolution{}, fmt.Errorf("%w: the installer repository for %s was not resolved and the last candidate tried was %q",
		ErrUpstreamUnavailable, r.Version, lastAttempt)
}
