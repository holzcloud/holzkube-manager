package imagefactory

// This file is deliberately in package imagefactory and not imagefactory_test.
// It reads installerCandidates, which is unexported and stays that way -- the
// four names it produces are an implementation detail of how a repository is
// resolved, not an API. canonical_live_test.go in the same directory is the
// external test package for the opposite reason: it exercises the package the
// way a caller does. Two test packages in one directory is a choice here, not
// an accident.
//
// What both guards in this file have in common is direction. A browser cannot
// import a Go symbol, so the TypeScript side transcribes what Go decides, and a
// transcription nothing checks is a comment rather than a guarantee. The check
// has to run from Go: vitest is rooted at web/ and refuses to read outside it
// without loosening the bundler's filesystem allowlist, which is a real cost to
// pay for a test-only convenience. This is the same argument
// TestWarningDetailsMatchTheUI makes, and this file follows its idiom.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

const (
	imagesRoutePath   = "../../web/src/routes/images.tsx"
	browserSuitePath  = "../../web/src/routes/images.browser.test.tsx"
	surrogateLow      = 0xD800
	surrogateHigh     = 0xDFFF
	maxReportedDrifts = 12
)

// declaredRange is one entry of the browser's refusal table, read out of the
// source rather than out of a running bundle.
type declaredRange struct {
	from, to rune
}

// browserRefusalRange matches one entry of REFUSED_RANGES in images.tsx.
//
// The browser guard declares its set as data specifically so this can read it.
// It used to be a chain of comparisons inside an if, which no test on either
// side could inspect -- and while it was, it drifted: plan 02-14 widened the
// server's set to four measured classes and the form kept refusing three.
var browserRefusalRange = regexp.MustCompile(
	`\{\s*from:\s*0x([0-9a-fA-F]+),\s*to:\s*0x([0-9a-fA-F]+)`)

// refusedRangesName is the identifier this guard binds to. It is a constant
// rather than a literal inside the pattern so that the error messages name the
// same string the anchor looks for.
const refusedRangesName = "REFUSED_RANGES"

// refusedRangesEntry matches one flat object literal inside the declaration
// body, whatever it is made of.
//
// Deliberately without nesting: the entries are flat, and a nested one would
// itself be a case this guard has to report rather than interpret. Every
// literal it collects has to be readable by browserRefusalRange, which is the
// same decision stringArrayLiteral and exportedWarningCodes already made -- a
// member expressed as something other than a literal is a member the guard
// cannot see, and failing is the honest answer. WR-04 names both as precedent.
var refusedRangesEntry = regexp.MustCompile(`\{[^{}]*\}`)

// refusedRangesDecl cuts the declaration out before anything is read from it.
//
// Two precedents, and this takes one thing from each. stringArrayLiteral
// anchors on the assignment rather than on the first bracket after the name,
// because a type annotation -- here `readonly RefusedRange[]`, which carries a
// bracket pair of its own -- sits between the two and would otherwise be read
// as an empty array. budget_drift_test.go:89-90 anchors on the start of a line
// with regexp.QuoteMeta around the name, "so a number that merely appears
// somewhere in the file cannot satisfy this"; the same sentence is true of an
// object literal, which is the gap this closes.
//
// The identifier ends where the pattern says it ends. What follows the name is
// whitespace, then EITHER a type annotation that has to begin with a colon OR
// the assignment straight away -- the required `:` or `=` IS the word boundary,
// because `_` is neither. This is the correction round 5 measured: the fragment
// that used to sit here allowed any run of characters without an `=`, so every
// prefix extension of the identifier satisfied the anchor. Measured then:
// `REFUSED_RANGES_LEGACY` -> `ranges=[{0 0}] err=<nil>` and `REFUSED_RANGESX`
// -> `ranges=[{0 1}] err=<nil>`, both with no error at all. The damage that
// buys is quiet: REFUSED_RANGES moves into another module, a prefixed leftover
// stays behind in images.tsx, and this guard reports agreement over a set the
// form no longer uses.
//
// The annotation is optional and colon-bound rather than free-form because
// stringArrayLiteral chose assignment anchoring for exactly one reason -- the
// annotation `readonly RefusedRange[]` carries a bracket pair of its own -- and
// in TypeScript such an annotation always begins with a colon. Requiring the
// colon keeps that case and drops every other continuation of the name.
//
// The body is captured non-greedily up to the first closing bracket. An entry
// carrying a bracket inside a string would cut it short -- which is not silent,
// because the count check below compares the entries found in the body against
// the entries found in the whole source and fails on any difference.
//
// That sentence was not true for a body cut short BEFORE its first entry: with
// no entry left inside, the empty branch was reached first and reported the
// declaration as empty, which round 5 measured against a declaration carrying
// six of them. The two are told apart by the SHAPE of the captured body --
// empty once whitespace is removed, or carrying text no entry can be read from
// -- so the cut-short case carries its own message and quotes the body it read,
// while the count check keeps the one question it can answer on its own. Round
// 6 borrowed the whole-source count for that separation instead, which is the
// defect this replaces: a count over the file cannot see where the entries it
// counted are.
var refusedRangesDecl = regexp.MustCompile(
	`(?ms)^\s*(?:export\s+)?const\s+` + regexp.QuoteMeta(refusedRangesName) +
		`\s*(?::[^=\n]*)?=\s*\[(.*?)\]`)

// guardBlindSpot is one mechanism this guard does not see, carried as DATA so
// that TestGuardBlindSpotsAreEachMeasured can bind it to a measured row.
//
// The list is kept by MECHANISM and not by FORM, and that is the whole design.
// A list of forms was incomplete again after each of three verification rounds
// -- round 4 found the rename, round 5 the prefixed leftover, round 6 the
// comment -- because a form is a shape and shapes are unbounded. A mechanism
// covers the shapes it can take, and the coverage test demands at least one
// measured shape per mechanism. Whoever adds an entry here adds a row too, or
// the coverage test goes red at them.
type guardBlindSpot struct {
	// id is the stable name a blindnessRow refers to.
	id string
	// mechanism says in ONE line why the guard is blind here, in terms of what
	// its anchor binds to rather than in terms of a syntax it fails to parse.
	mechanism string
	// measured carries the output that evidences the entry. An entry without a
	// measurement is a claim, and claims are what this file is here to stop.
	measured string
	// rowless is empty for every entry a row can measure. When it is not, it
	// carries the reason why no row can. TestGuardBlindSpotsAreEachMeasured
	// requires there to be EXACTLY ONE such entry -- otherwise this field is a
	// hole through which a future entry escapes being measured at all.
	rowless string
}

// guardBlindSpots is the written-down remainder: what a regular expression over
// TypeScript source does not establish, one entry per mechanism.
//
// It is the replacement for a sentence that claimed the remainder was zero. The
// ARITY of the remainder is what changes: a universally quantified claim has
// infinitely many counterexamples and every one of them is a falsification; a
// written list has finitely many entries and every new shape is an addition.
//
// The list was ATTACKED once when it was written, and the attempt is recorded
// here rather than only in a planning document, because an unrecorded attempt
// cannot be told apart from an omitted one. Six shapes were put through the
// shipped reader looking for one it reads that this list does not name:
// String.raw, a regex literal and a ${...} interpolation were each READ
// (ranges=6 err=nil) and are shapes of text-not-code, so no finding; an object
// property and a re-export were correctly refused with "no REFUSED_RANGES
// declared as an array literal"; two declarations with an indented inner one
// first were refused with "2 declarations of REFUSED_RANGES in this source",
// which is plan 02-29's check -- and that shape WAS a finding when this attempt
// was first run while planning, and was fixed rather than listed.
//
// No shape was found that this list does not name. That is not a closure: an
// attempt that only asks after the shapes already known would close itself. The
// round that closes the ledger entry for this blindness has to run it again.
var guardBlindSpots = []guardBlindSpot{
	{
		id: "text-not-code",
		mechanism: "the anchor matches the identifier wherever it stands IN THE TEXT; " +
			"whether the browser ever executes that text is not a question a regular " +
			"expression can ask",
		measured: "a declaration living only in a /* */ block, only in an indented {/* */} " +
			"JSX comment, only in a template literal, or only as JSX text inside a <pre> " +
			"block each read ranges=6 err=nil with the real import standing beside it; " +
			"String.raw, a regex literal and a ${...} interpolation measured the same",
	},
	{
		id: "literal-not-value",
		mechanism: "the anchor binds to the array LITERAL, while the form uses the VALUE " +
			"the expression around that literal produces -- a filter, a slice or a spread " +
			"between the two is invisible from here",
		measured: "].filter((r) => r.class !== 'byte order mark') behind the literal reads " +
			"ranges=6 err=nil while the form accepts U+FEFF again and the server keeps " +
			"answering 400; [...REFUSED_RANGES, ...PLATFORM_RANGES] as the table the form " +
			"really uses reads ranges=6 err=nil over a set the form does not use",
	},
	{
		id: "declaration-not-use",
		mechanism: "the anchor binds to the DECLARATION and not to the call site that makes " +
			"it live; images.tsx has exactly one living reference to the table, and a table " +
			"nothing calls is a well-formed, compiling, agreeing corpse",
		measured: "an indented leftover declaration inside a function, beside the real " +
			"table's import from another module, reads ranges=2 err=nil -- with no slash, " +
			"backtick or quote anywhere in the shape",
	},
	{
		id: "surrogate-interior",
		mechanism: "the codepoint sweep skips U+D800..U+DFFF and asserts the surrogate set " +
			"at its two endpoints only, and its server-side twin rawBodyRefusal is never " +
			"called from here -- so for the 2046 codepoints between those endpoints " +
			"nothing at all is compared",
		measured: "TestBrowserRefusalSetEqualsTheServers puts every codepoint from 0 to " +
			"0x10FFFF through NotRepresentableReason except 0xD800..0xDFFF, which it " +
			"skips with a continue, and asserts refusedByBrowser at 0xD800 and 0xDFFF only",
		rowless: "it is a property of the SWEEP and not of the reader, and the blindness " +
			"tables drive the reader. Go cannot hold an unpaired surrogate in a string at " +
			"all -- string(rune(0xD800)) is U+FFFD -- so no fixture can make a row measure it",
	},
}

// honestClaim renders what this guard establishes OUT OF the list of what it
// does not, so that the two cannot drift apart.
//
// Rendered and not written beside guardBlindSpots on purpose: two lists of
// different lengths would themselves be a drift risk, which is precisely the
// genus this file guards against.
func honestClaim() string {
	var b strings.Builder
	b.WriteString("What this guard is, stated where it matters:\n")
	b.WriteString("It is a regular expression over TEXT and not over a program. Its anchor " +
		"binds to a LITERAL and not to the VALUE an expression around it produces, and to " +
		"an IDENTIFIER and not to CODE the browser executes. Not finding the declaration " +
		"means the anchor did not match this text; it does not mean the table is gone. " +
		"Finding it does not mean the browser runs it.\n")
	b.WriteString("What it therefore does not see:\n")
	for _, spot := range guardBlindSpots {
		fmt.Fprintf(&b, "  - %s: %s\n    measured: %s\n", spot.id, spot.mechanism, spot.measured)
		if spot.rowless != "" {
			fmt.Fprintf(&b, "    no row can measure this: %s\n", spot.rowless)
		}
	}
	return b.String()
}

// TestBrowserRefusalSetEqualsTheServers is G-02-11's drift guard.
//
// It compares behaviour and not two declarations, which is what makes it a
// guard rather than a second transcription: every codepoint is put through
// NotRepresentableReason on the Go side and through the browser's declared
// ranges on the other, and any disagreement fails.
//
// It fails in BOTH directions, and the second one is the one nobody writes. An
// under-refusing client is a value the operator sends and the server rejects --
// an extra round trip, and the defect this test was written for. An
// over-refusing client is worse: a value the API accepts that the form will not
// let an operator enter, with no way to work around it, which quietly makes the
// form the authority instead of the contract.
func TestBrowserRefusalSetEqualsTheServers(t *testing.T) {
	ranges := browserRefusalRanges(t, imagesRoutePath)

	refusedByBrowser := func(r rune) bool {
		for _, each := range ranges {
			if r >= each.from && r <= each.to {
				return true
			}
		}
		return false
	}

	// The surrogate range is the one entry that is not compared codepoint by
	// codepoint, because Go cannot hold an unpaired surrogate in a string at
	// all -- string(rune(0xD800)) is U+FFFD, so asking NotRepresentableReason
	// about one asks it about a different character. Its server-side twin is
	// rawBodyRefusal in internal/httpapi/handlers/schematics.go, which reads the
	// request body's bytes before the decoder can rewrite the escape. So the
	// range is asserted present here and excluded from the sweep below.
	if !refusedByBrowser(surrogateLow) || !refusedByBrowser(surrogateHigh) {
		t.Errorf("%s no longer refuses the surrogate range U+D800-U+DFFF.\n"+
			"The server refuses an unpaired surrogate on the raw request body "+
			"(handlers.rawBodyRefusal); dropping it here makes the form accept a "+
			"value the API answers 400 to.", imagesRoutePath)
	}

	var underRefused, overRefused []rune
	buf := make([]byte, 4)
	for r := rune(0); r <= 0x10FFFF; r++ {
		if r >= surrogateLow && r <= surrogateHigh {
			continue
		}
		n := utf8.EncodeRune(buf, r)
		server := NotRepresentableReason(string(buf[:n])) != ""
		browser := refusedByBrowser(r)
		switch {
		case server && !browser:
			underRefused = append(underRefused, r)
		case browser && !server:
			overRefused = append(overRefused, r)
		}
	}

	if len(underRefused) > 0 {
		t.Errorf("%d codepoints the server refuses are accepted by %s: %s\n"+
			"The operator learns about these from a 400 instead of from the row "+
			"while they are still looking at it. Widen REFUSED_RANGES.",
			len(underRefused), imagesRoutePath, sampleCodepoints(underRefused))
	}
	if len(overRefused) > 0 {
		t.Errorf("%d codepoints the server accepts are refused by %s: %s\n"+
			"A client refusal of a value the API accepts is a false refusal an "+
			"operator cannot work around. Narrow REFUSED_RANGES; do NOT widen the "+
			"server to match, which would move the precomputed id FACT-06 rests on.",
			len(overRefused), imagesRoutePath, sampleCodepoints(overRefused))
	}
}

// TestBrowserInstallerNamesEqualInstallerCandidates closes the residual plan
// 02-17 recorded against itself.
//
// That plan's browser sweep proves each of the four installer repository names
// occupies one line box at every width from 30px to 280px -- the property
// G-02-10 failed on. The names are declared in TypeScript and derived from a
// construction that lives here, and nothing held the two together: a fifth
// candidate added to installerCandidates would leave the sweep measuring four
// stale strings and still passing green, which is a small instance of exactly
// the defect 02-17 exists to close.
//
// It binds to the literal array rather than to the derived one, because the
// literal is the form a Go test can read out of the source. The browser suite
// asserts its own derivation equals those literals, so pinning the literals
// pins both.
func TestBrowserInstallerNamesEqualInstallerCandidates(t *testing.T) {
	source := readSource(t, browserSuitePath)

	declared := stringArrayLiteral(t, source, "INSTALLER_REPOSITORY_NAMES_LITERAL")

	var want []string
	for _, secureBoot := range []bool{false, true} {
		want = append(want, installerCandidates(AssetRequest{
			Platform:   PlatformMetal,
			SecureBoot: secureBoot,
		})...)
	}

	if len(declared) != len(want) {
		t.Fatalf("%s declares %d installer names, installerCandidates produces %d "+
			"across both SecureBoot states.\ndeclared: %q\nGo:       %q\n"+
			"Copy the Go names into the TypeScript array, never the other way round.",
			browserSuitePath, len(declared), len(want), declared, want)
	}
	for i := range want {
		if declared[i] != want[i] {
			t.Errorf("installer name %d: %s has %q, installerCandidates produces %q",
				i, browserSuitePath, declared[i], want[i])
		}
	}
}

// browserRefusalRanges reads the declared table out of a route.
//
// An empty result is a failure and not a skip. A guard that silently passes
// when it can no longer find what it guards is worse than no guard: it reports
// agreement it never checked.
//
// The reading itself is parseBrowserRefusalRanges, which takes a string and
// returns an error instead of taking a *testing.T. That split is the whole
// point of that round: a guard whose failure nothing checks is exactly the
// class of defect being closed here, and with t.Fatalf inside the reader its
// failure cases cannot be tested at all. Only the file read and the fatal stay
// on this side of the line.
//
// The path is a parameter and there is no wrapper that hardcodes one. That is
// the point of this signature and not a side effect of it: the blindness
// tables drive exactly this function, readSource included, so a preprocessing
// step somebody later slides between the file and the expression becomes
// VISIBLE in them instead of passing beside them. A table whose anti-red
// promise can be evaded by moving work to the caller promises nothing.
func browserRefusalRanges(t *testing.T, path string) []declaredRange {
	t.Helper()

	ranges, err := parseBrowserRefusalRanges(readSource(t, path))
	if err != nil {
		// The reason lives in the error, which is where it is testable now.
		// This says only which file was read.
		t.Fatalf("%s: %v", path, err)
	}
	return ranges
}

// parseBrowserRefusalRanges reads the refusal table out of images.tsx source.
//
// It is pure so that its failure cases are themselves testable; see
// browserRefusalRanges for why that matters.
//
// It reads the declaration and not the file, and what it establishes is a
// property of the TEXT it was handed -- nothing beyond that. What it does NOT
// establish about the rest of that text is written down in guardBlindSpots,
// mechanism by mechanism with its measured output, and rendered into the
// no-anchor failure by honestClaim. Three verification rounds each falsified a
// wider sentence that used to stand here; a regular expression over TypeScript
// source has a remainder that does not go to zero, so the claim is cut to what
// is measured and the remainder is written down instead of denied.
func parseBrowserRefusalRanges(source string) ([]declaredRange, error) {
	decls := refusedRangesDecl.FindAllStringSubmatch(source, -1)
	if len(decls) == 0 {
		// honestClaim is appended HERE and to no other exit. This is the exit at
		// which the guard says it found nothing, so it is the one place a reader
		// has to know what not-finding means here and what it does not.
		return nil, fmt.Errorf("no %s declared as an array literal.\n"+
			"The browser's set has to be data -- a named array of {from: 0x.., to: 0x..} "+
			"entries -- so that this guard can compare it to the server's. A chain of "+
			"comparisons inside an if is unreadable from here, and while it was one, the "+
			"two sets drifted (G-02-11). A name that merely carries %s as a prefix -- "+
			"%s_LEGACY, %sX -- is a different name and does not satisfy this guard "+
			"either; that leftover is what stays behind when the real table moves away.\n\n%s",
			refusedRangesName, refusedRangesName, refusedRangesName, refusedRangesName,
			honestClaim())
	}

	// The guard does not choose, it requires uniqueness. Reading one of two
	// declarations of the same name is reporting agreement about the other
	// without ever having compared it -- the readability loop's reason, one
	// level up, at the declaration instead of at the entry.
	//
	// The counterfactual is not theoretical, and it is not exotic either: an
	// indented second declaration is the shape a table has for as long as it
	// exists in two places during a move, and the anchor allows leading
	// whitespace, so it satisfies it exactly as well as the real one does.
	// Measured before this check existed, with an inner declaration standing
	// before the real six-entry table: `1 entries inside the REFUSED_RANGES
	// declaration, 7 in the source as a whole` -- fail closed, with the cause
	// of a different defect, sending the reader after a stray literal while the
	// real table sat unread below. With the inner one empty it was worse after
	// the body-shape branch above: a table of six entries reported as empty.
	// This runs BEFORE the body is read so that neither of those later checks
	// can overwrite the cause with its own.
	if len(decls) > 1 {
		return nil, fmt.Errorf("%d declarations of %s in this source, and this guard reads one.\n"+
			"Which of them the browser actually uses is not this guard's to guess: taking the "+
			"first is reporting agreement about the others without having compared them. Until "+
			"exactly one declaration of this name is left, there is no set to compare",
			len(decls), refusedRangesName)
	}

	body := decls[0][1]

	// Before the count check and not after it, because this case would trip
	// that one too and explain it wrongly: an unreadable entry INSIDE the
	// declaration makes the body count differ from the whole-source count, and
	// the count message speaks of a literal OUTSIDE the declaration. Order the
	// checks so the more specific diagnosis wins.
	for _, literal := range refusedRangesEntry.FindAllString(body, -1) {
		if browserRefusalRange.MatchString(literal) {
			continue
		}
		return nil, fmt.Errorf("this entry of %s cannot be read by this guard: %s\n"+
			"Every bound has to be a hexadecimal literal, because this guard compares "+
			"SETS: an entry it skips is a codepoint it reports agreement about without "+
			"having compared it. Failing is the honest answer, which is what "+
			"stringArrayLiteral and exportedWarningCodes already do",
			refusedRangesName, literal)
	}

	matches := browserRefusalRange.FindAllStringSubmatch(body, -1)

	// Three causes, three messages, and none of them claims another's: the
	// readability loop above catches an unreadable entry INSIDE the
	// declaration, this branch catches an empty declaration and a body cut off
	// BEFORE the first entry, and the count check below catches an entry-shaped
	// literal OUTSIDE the declaration.
	//
	// The quantity that separates the two causes inside this branch is the
	// SHAPE OF THE BODY and not a count over the file. Round 6 used the
	// whole-source count for it, and a count over the file cannot see where the
	// entries it counted are: a declaration that is literally `= []`, standing
	// anywhere near a second entry-shaped table, was reported as cut short and
	// the reader was sent after a bracket that does not exist. A property of
	// the declaration is the only thing that can answer a question about the
	// declaration.
	//
	// What the trim leaves behind, said here rather than in a footnote: a body
	// made of nothing but a comment is not empty to TrimSpace, so it takes the
	// cut-short branch although nothing was cut. That is the same genus of
	// defect inside the change meant to remove it. It is smaller -- the message
	// SHOWS with %q the body it read instead of asserting "the declaration is
	// NOT empty", which was never established but merely inferred -- and it is
	// not nothing. The row `a body carrying only a comment` pins it so it stays
	// measured rather than remembered.
	if len(matches) == 0 {
		if strings.TrimSpace(body) == "" {
			return nil, fmt.Errorf("%s is declared but carries no entries this guard can read.\n"+
				"This is the declaration being empty, not the declaration being absent; "+
				"the two are separate failures because they call for separate fixes",
				refusedRangesName)
		}
		return nil, fmt.Errorf("the %s declaration body was cut short before its first entry.\n"+
			"The captured body is %q -- it carries text, and no entry this guard can read. The "+
			"body is captured non-greedily up to the first closing bracket, so a `]` inside a "+
			"comment or a string before the first entry ends the capture there. Look for that "+
			"bracket in the body quoted above, not for a missing table",
			refusedRangesName, body)
	}

	// Counted here, at its one remaining use, and answering exactly one
	// question: does an entry-shaped literal sit OUTSIDE the declaration. Round
	// 6 had this same number answer a second one -- was the body cut short --
	// and borrowing it was what made the answer wrong for a declaration that
	// really was empty. One quantity, one question.
	whole := len(browserRefusalRange.FindAllString(source, -1))

	// The pollution direction, and the truncation direction that leaves entries
	// inside the body, in one comparison. An entry-shaped literal outside the
	// declaration is one this guard now correctly ignores but somebody should
	// look at; a body that ended early after its first entry, because a later
	// entry carries a closing bracket inside a string, produces the same
	// inequality. This guard cannot tell those two apart, so it names both.
	if whole != len(matches) {
		return nil, fmt.Errorf("%d entries inside the %s declaration, %d in the source as a whole.\n"+
			"Either an entry-shaped literal sits outside the declaration -- this guard "+
			"used to fold those into the browser's set and now ignores them, which is "+
			"correct and still worth a look -- or the declaration body was cut short "+
			"because an entry carries a closing bracket inside a string",
			len(matches), refusedRangesName, whole)
	}

	out := make([]declaredRange, 0, len(matches))
	for _, m := range matches {
		from, err := strconv.ParseUint(m[1], 16, 32)
		if err != nil {
			return nil, fmt.Errorf("unreadable range start %q: %w", m[1], err)
		}
		to, err := strconv.ParseUint(m[2], 16, 32)
		if err != nil {
			return nil, fmt.Errorf("unreadable range end %q: %w", m[2], err)
		}

		// Semantic and not syntactic, which is what separates this from the
		// readability loop above: those bounds parsed, and they still mean
		// nothing. rune(uint64) is lossy -- 0xFFFFFFFF becomes -1 -- and the
		// sweep in TestBrowserRefusalSetEqualsTheServers runs upwards from
		// rune(0), so an entry whose upper bound landed on a negative rune, or
		// whose lower bound sits above its upper, covers not one codepoint and
		// is passed over in silence. That inherits the readability loop's
		// reason word for word: an entry it skips is a codepoint it reports
		// agreement about without having compared it.
		//
		// The expensive direction is the one round 5 names as the worse of the
		// two: { from: 0x0061, to: 0xFFFFFFFF }, measured as {97 -1}. The form
		// in the browser refuses every lowercase letter from it, while Go reads
		// a range covering nothing and compares nothing -- an over-refusing
		// client no operator can work around, and green the whole way.
		//
		// The bit width of ParseUint stays at 32 on purpose. At 21 ParseUint
		// would reject these itself, but with `value out of range`, which names
		// neither the entry, nor the set, nor the reason. Here the message is
		// the point and not the abort, so the width stays wide enough to read
		// the literal and the checks below say what is wrong with it.
		//
		// Two checks and not one, because the two causes are not the same cause
		// and a message that names the wrong one is worse than no message. The
		// round that wrote `none of them claims another's` about the three
		// causes above put these two into a single `to > utf8.MaxRune ||
		// from > to` with a single text: for { from: 0x001f, to: 0x0000 } both
		// bounds are perfectly representable, and the operator was told his
		// value sits above utf8.MaxRune and went looking there. The test row
		// that was supposed to catch that pinned the wrong wording instead --
		// a check written against the message it happens to produce rather than
		// against the cause it is named for.
		if to > utf8.MaxRune {
			return nil, fmt.Errorf("this entry of %s has an upper bound this guard cannot "+
				"represent: from 0x%s to 0x%s.\n"+
				"The upper bound has to be at most utf8.MaxRune. rune(uint64) is lossy -- a "+
				"bound above utf8.MaxRune lands on a negative rune -- so such an entry covers "+
				"no codepoint at all in a sweep from rune(0) upwards and is passed over. An "+
				"entry it passes over is a codepoint it reports agreement about without having "+
				"compared it -- the same reason the unreadable-entry check above gives",
				refusedRangesName, m[1], m[2])
		}
		if from > to {
			return nil, fmt.Errorf("this entry of %s is inverted: from 0x%s to 0x%s.\n"+
				"Both bounds are representable and the entry still covers nothing, because the "+
				"sweep runs upwards from rune(0) and never enters a range whose lower bound "+
				"sits above its upper. Nothing was lost in conversion here; the two numbers are "+
				"in the wrong order. An entry it passes over is a codepoint it reports "+
				"agreement about without having compared it -- the same reason the "+
				"unreadable-entry check above gives",
				refusedRangesName, m[1], m[2])
		}

		out = append(out, declaredRange{from: rune(from), to: rune(to)})
	}
	return out, nil
}

// TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration is round 4's
// falsification of this file's own guard, encoded with the outcome reversed.
//
// The verifier renamed REFUSED_RANGES to RENAMED_BY_VERIFIER throughout
// images.tsx and re-ran TestBrowserRefusalSetEqualsTheServers:
// `ok github.com/holzcloud/holzkube-manager/internal/imagefactory 0.546s`. The
// guard was green against a declaration that no longer existed, which is the
// one property browserRefusalRanges's own comment claims it has.
//
// The table runs over synthetic sources rather than over the file, because a
// copy of the real file would be a second transcription and this file exists to
// stop transcriptions nothing checks. The first row is the real source, so the
// other three are anchored to reality and not merely consistent with each other.
func TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration(t *testing.T) {
	realSource := readSource(t, imagesRoutePath)

	// Renamed throughout, entries left verbatim. This is the verifier's own
	// experiment: `ok ... 0.546s` against a declaration that was gone.
	const renamed = `type RefusedRange = { from: number; to: number; class: string }

const RENAMED_BY_VERIFIER: readonly RefusedRange[] = [
  { from: 0x0000, to: 0x001f, class: 'control character' },
  { from: 0xfeff, to: 0xfeff, class: 'byte order mark' },
]
`

	// The declaration gone altogether, with nothing entry-shaped left behind.
	const absent = `export function hasControlCharacter(value: string): boolean {
  return (value.codePointAt(0) ?? 0) === 0xfeff
}
`

	// A valid declaration plus one entry-shaped literal outside it: the
	// pollution direction. One entry inside, two in the file as a whole.
	const polluted = `const REFUSED_RANGES: readonly RefusedRange[] = [
  { from: 0x0000, to: 0x001f, class: 'control character' },
]

const SOME_OTHER_TABLE: readonly RefusedRange[] = [
  { from: 0x2028, to: 0x2029, class: 'line separator' },
]
`

	// The rename somebody actually performs, as opposed to the one nobody does.
	// RENAMED_BY_VERIFIER above is a name chosen to be unrelated; this is a name
	// that keeps REFUSED_RANGES as its prefix, which is what a leftover table
	// looks like after the real one moves into another module. Round 5 measured
	// this source as `ranges=[{0 0}] err=<nil>`: the guard read the leftover and
	// reported agreement about a set the form no longer uses.
	const prefixed = `const REFUSED_RANGES_LEGACY: readonly RefusedRange[] = [
  { from: 0x0000, to: 0x001f, class: 'control character' },
  { from: 0xfeff, to: 0xfeff, class: 'byte order mark' },
]
`

	// A complete, correct declaration whose body opens with a comment carrying a
	// closing bracket. The non-greedy body capture ends at that bracket, before
	// the first entry: measured, the body is "\n  // see the table in
	// RefusedRange[", so len(matches) == 0 while the source as a whole carries
	// two. Round 5 measured the guard reporting THIS as the declaration being
	// empty -- fail closed, but with a cause that sends the reader to the wrong
	// place.
	const truncated = `const REFUSED_RANGES: readonly RefusedRange[] = [
  // see the table in RefusedRange[] above
  { from: 0x0000, to: 0x001f, class: 'control character' },
  { from: 0xfeff, to: 0xfeff, class: 'byte order mark' },
]
`

	// A declaration that is literally empty, with a second entry-shaped table
	// beside it. Round 6 made the whole-source count the quantity that tells
	// "empty" from "cut short" apart, and a count over the file cannot see
	// where the entries it counted are: measured, this source produced
	// `cut short before its first entry, but 1 entries are present in the
	// source as a whole` -- the instruction to look for a bracket that does
	// not exist, while the actual cause (somebody emptied the table) does not
	// appear in the text at all. The difference to `truncated` above is the
	// whole point: there the bracket is really there, here there is none.
	// SOME_OTHER_TABLE is the same second table the `polluted` row already
	// carries as realistic.
	const emptyBesideASecondTable = `const REFUSED_RANGES: readonly RefusedRange[] = []

const SOME_OTHER_TABLE: readonly RefusedRange[] = [
  { from: 0x2028, to: 0x2029, class: 'line separator' },
]
`

	// The remainder the body-shape distinction leaves behind, pinned rather
	// than mentioned in a footnote. A body made of nothing but a comment is not
	// empty to strings.TrimSpace, so it takes the cut-short branch although
	// nothing was cut -- the same genus of defect inside the change that
	// removes that genus. It is kept, and kept measured, because the new
	// message SHOWS the body it read instead of asserting a property it never
	// established: a reader sees "\n  // nothing yet\n" and needs no further
	// explanation of what the guard found. Today this source reaches the empty
	// branch instead, because no entry-shaped literal exists anywhere in it.
	const commentOnlyBody = `const REFUSED_RANGES: readonly RefusedRange[] = [
  // nothing yet
]
`

	// Two declarations of the guarded identifier, an indented inner one first.
	// The anchor is `^` with leading whitespace allowed, so an indented
	// declaration satisfies it just as well as a top-level one, and the guard
	// took the first match without ever asking whether there was a second.
	// Measured before the count check existed: `1 entries inside the
	// REFUSED_RANGES declaration, 7 in the source as a whole` -- fail closed,
	// but with the cause of a different defect. The reader is sent looking for
	// a stray literal outside the declaration while the real table sits
	// unread below. The outer table carries the six ranges of the real route so
	// the number in that measured message is the one the real file produces.
	const shadowedByAnInnerTable = `function buildRefusalTable() {
  const REFUSED_RANGES: readonly RefusedRange[] = [
    { from: 0x2028, to: 0x2029, class: 'line separator' },
  ]
  return REFUSED_RANGES
}

const REFUSED_RANGES: readonly RefusedRange[] = [
  { from: 0x0000, to: 0x001f, class: 'control character' },
  { from: 0x007f, to: 0x009f, class: 'control character' },
  { from: 0xd800, to: 0xdfff, class: 'unpaired surrogate' },
  { from: 0x2028, to: 0x2029, class: 'line separator' },
  { from: 0xfeff, to: 0xfeff, class: 'byte order mark' },
  { from: 0xfffe, to: 0x10ffff, class: 'above U+FFFD' },
]
`

	// The same shape with an empty inner declaration, and its wrong cause is
	// one this plan created. Before the body-shape distinction of task 1 this
	// source produced the cut-short message; after it, the guard reads the
	// empty inner body, finds nothing in it, and reports `REFUSED_RANGES is
	// declared but carries no entries this guard can read` about a file whose
	// real table carries six. A correction that leaves a new wrong cause behind
	// is the reason this check belongs in the same round as that correction and
	// not in the next one.
	const shadowedByAnEmptyInnerTable = `function buildRefusalTable() {
  const REFUSED_RANGES: readonly RefusedRange[] = []
  return REFUSED_RANGES
}

const REFUSED_RANGES: readonly RefusedRange[] = [
  { from: 0x0000, to: 0x001f, class: 'control character' },
  { from: 0x007f, to: 0x009f, class: 'control character' },
  { from: 0xd800, to: 0xdfff, class: 'unpaired surrogate' },
  { from: 0x2028, to: 0x2029, class: 'line separator' },
  { from: 0xfeff, to: 0xfeff, class: 'byte order mark' },
  { from: 0xfffe, to: 0x10ffff, class: 'above U+FFFD' },
]
`

	for _, tc := range []struct {
		name       string
		source     string
		wantErr    []string
		wantRanges int
	}{
		{
			name:       "the real route",
			source:     realSource,
			wantRanges: 6,
		},
		{
			name:    "the declaration renamed out of existence",
			source:  renamed,
			wantErr: []string{"no REFUSED_RANGES declared as an array literal"},
		},
		{
			name:    "the declaration absent",
			source:  absent,
			wantErr: []string{"no REFUSED_RANGES declared as an array literal"},
		},
		{
			name:    "an entry-shaped literal outside the declaration",
			source:  polluted,
			wantErr: []string{"1 entries inside the REFUSED_RANGES declaration, 2 in the source as a whole"},
		},
		{
			name:   "the declaration renamed to a prefixed name",
			source: prefixed,
			wantErr: []string{
				"no REFUSED_RANGES declared as an array literal",
				"A name that merely carries REFUSED_RANGES as a prefix",
			},
		},
		{
			name:   "the body cut short by a bracket in a comment",
			source: truncated,
			wantErr: []string{
				"cut short before its first entry",
				`"\n  // see the table in RefusedRange["`,
			},
		},
		{
			name:   "a genuinely empty declaration beside a second table",
			source: emptyBesideASecondTable,
			wantErr: []string{
				"REFUSED_RANGES is declared but carries no entries this guard can read",
			},
		},
		{
			name:   "a body carrying only a comment",
			source: commentOnlyBody,
			wantErr: []string{
				"cut short before its first entry",
				`"\n  // nothing yet\n"`,
			},
		},
		{
			name:   "two declarations, an indented inner one first",
			source: shadowedByAnInnerTable,
			wantErr: []string{
				"2 declarations of REFUSED_RANGES in this source",
			},
		},
		{
			name:   "an empty inner declaration before the real one",
			source: shadowedByAnEmptyInnerTable,
			wantErr: []string{
				"2 declarations of REFUSED_RANGES in this source",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ranges, err := parseBrowserRefusalRanges(tc.source)

			if len(tc.wantErr) == 0 {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(ranges) != tc.wantRanges {
					t.Fatalf("read %d ranges, want %d", len(ranges), tc.wantRanges)
				}
				return
			}

			if err == nil {
				t.Fatalf("no error; read %d ranges instead.\n"+
					"A guard that reports a pass here reports agreement it never "+
					"checked -- the property round 4 measured as absent.", len(ranges))
			}
			for _, want := range tc.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error does not name %q:\n%v", want, err)
				}
			}
		})
	}
}

// TestBrowserRefusalGuardRefusesAnEntryItCannotRead pins the second half of
// WR-04: an entry the entry pattern cannot read is a failure and not a skip.
//
// stringArrayLiteral and exportedWarningCodes already made this decision -- a
// member computed at runtime is a member the guard cannot see, and failing is
// the honest answer -- and WR-04 names both of them as the precedent. The
// reason it matters here is that this guard compares SETS: an entry it does not
// see is a codepoint it reports agreement about without ever having checked it.
//
// A separate table from
// TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration on purpose. Each
// table's acceptance pins the number of its rows, and that number only holds
// while the tables stay apart. A table test whose rows quietly disappear is the
// same defect as a guard that cannot find what it guards, one level up.
func TestBrowserRefusalGuardRefusesAnEntryItCannotRead(t *testing.T) {
	const decimal = `const REFUSED_RANGES: readonly RefusedRange[] = [
  { from: 0, to: 31, class: 'control character' },
]
`

	const namedConstants = `const REFUSED_RANGES: readonly RefusedRange[] = [
  { from: SURROGATE_LOW, to: SURROGATE_HIGH, class: 'unpaired surrogate' },
]
`

	for _, tc := range []struct {
		name    string
		source  string
		wantErr []string
	}{
		{
			name:    "an entry written in decimal",
			source:  decimal,
			wantErr: []string{`{ from: 0, to: 31, class: 'control character' }`},
		},
		{
			name:   "an entry whose bounds are named constants",
			source: namedConstants,
			wantErr: []string{
				`{ from: SURROGATE_LOW, to: SURROGATE_HIGH, class: 'unpaired surrogate' }`,
				"SURROGATE_LOW",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ranges, err := parseBrowserRefusalRanges(tc.source)
			if err == nil {
				t.Fatalf("no error; read %d ranges instead.\n"+
					"An entry this guard skips is a codepoint it reports agreement "+
					"about without having compared it.", len(ranges))
			}
			for _, want := range tc.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error does not quote %q:\n%v", want, err)
				}
			}
		})
	}
}

// TestBrowserRefusalGuardRefusesABoundItCannotRepresent pins round 5's second
// measured hole: the unreadable-entry check above is syntactic, not semantic.
//
// It asks whether browserRefusalRange matches the literal, and nothing asks
// whether the numbers it read mean anything. strconv.ParseUint(..., 16, 32)
// accepts up to 0xFFFFFFFF, rune(uint64) is lossy, and there was neither
// `from <= to` nor `to <= utf8.MaxRune`. Round 5 measured all three rows below
// as `err=<nil>` with a declaredRange that covers no codepoint at all in a
// sweep starting at rune(0) -- which is precisely the property the unreadable
// check exists to remove, one layer deeper: an entry the guard passes over is a
// codepoint it reports agreement about without having compared it.
//
// A separate table from the two above, for the reason
// TestBrowserRefusalGuardRefusesAnEntryItCannotRead already gives: each table's
// acceptance pins the number of its rows, and that number only holds while the
// tables stay apart.
func TestBrowserRefusalGuardRefusesABoundItCannotRepresent(t *testing.T) {
	// Round 5: `{0xFFFFFFFF, 0xFFFFFFFF}` -> `{-1 -1}` and
	// `{0x10FFFF, 0xFFFFFFFF}` -> `{1114111 -1}`.
	const upperOutsideUnicode = `const REFUSED_RANGES: readonly RefusedRange[] = [
  { from: 0x0000, to: 0xFFFFFFFF, class: 'control character' },
]
`

	// Round 5: `{0x0061, 0xFFFFFFFF}` -> `{97 -1}`. A row of its own and not a
	// variant of the one above, because this is the direction the verification
	// text names as the worse one: the form in the browser refuses every
	// lowercase letter, while Go reads {97 -1} and compares nothing. An
	// over-refusing client is a false refusal an operator cannot work around.
	const realLowerUnrepresentableUpper = `const REFUSED_RANGES: readonly RefusedRange[] = [
  { from: 0x0061, to: 0xFFFFFFFF, class: 'control character' },
]
`

	// Round 5: `{0x001f, 0x0000}` -> `{31 0}`. Representable on both ends and
	// still covering nothing, because the sweep runs upwards.
	const inverted = `const REFUSED_RANGES: readonly RefusedRange[] = [
  { from: 0x001f, to: 0x0000, class: 'control character' },
]
`

	for _, tc := range []struct {
		name    string
		source  string
		wantErr []string
	}{
		{
			name:    "an upper bound outside Unicode",
			source:  upperOutsideUnicode,
			wantErr: []string{"from 0x0000 to 0xFFFFFFFF", "has an upper bound this guard cannot represent"},
		},
		{
			name:    "a real lower bound with an unrepresentable upper",
			source:  realLowerUnrepresentableUpper,
			wantErr: []string{"from 0x0061 to 0xFFFFFFFF", "has an upper bound this guard cannot represent"},
		},
		{
			name:    "an inverted range",
			source:  inverted,
			wantErr: []string{"from 0x001f to 0x0000", "is inverted"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ranges, err := parseBrowserRefusalRanges(tc.source)
			if err == nil {
				t.Fatalf("no error; read %d ranges instead: %v\n"+
					"A bound this guard cannot represent covers no codepoint in a sweep "+
					"from rune(0) upwards, so the entry is passed over -- and an entry it "+
					"passes over is a codepoint it reports agreement about without having "+
					"compared it.", len(ranges), ranges)
			}
			for _, want := range tc.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error does not name %q:\n%v", want, err)
				}
			}
		})
	}
}

// blindnessRow is one measured shape of one guardBlindSpot: a synthetic source
// the guard READS although the browser would never run what it read.
type blindnessRow struct {
	// name is the subtest name. It describes the shape, not the mechanism -- the
	// mechanism is named once, in guardBlindSpots.
	name string
	// blindSpot is the id of the guardBlindSpots entry this row measures.
	// TestGuardBlindSpotsAreEachMeasured fails on an id no entry carries.
	blindSpot string
	// source is written into a file under t.TempDir() and read back through the
	// live path. Never a copy of the real route: a copy would be a second
	// transcription, and this file exists to stop transcriptions nothing checks.
	source string
	// wantRanges is what the guard reads out of source today. The number is part
	// of the measurement: a row that only demanded err == nil would stay green
	// over a guard that read a different set.
	wantRanges int
}

// The real table's six entries, copied verbatim out of web/src/routes/images.tsx
// so the measured range count is the one the real route produces. Copied and
// not read: the fixtures describe damage cases, and a fixture that read the
// real file would move with it and stop describing anything.
//
// The declaration lives ONLY inside a plain block comment. The anchor allows
// leading whitespace and asks nothing about what encloses the line, so a
// comment satisfies it exactly as well as code does. The real table is imported
// from another module beside it AND used -- without the use this fixture would
// be a TypeScript error rather than a damage case, and a guard passing over a
// file that does not compile proves nothing.
const declarationOnlyInABlockComment = `import { REFUSED_RANGES } from '../lib/refusal-table'

export function hasControlCharacter(value: string): boolean {
  for (const character of value) {
    const code = character.codePointAt(0) ?? 0
    if (REFUSED_RANGES.some((range) => code >= range.from && code <= range.to)) {
      return true
    }
  }
  return false
}

/*
The table moved to ../lib/refusal-table. This copy stays so the classes are
readable beside the form. It is a comment; nothing executes it.

const REFUSED_RANGES: readonly RefusedRange[] = [
  { from: 0x0000, to: 0x001f, class: 'control character' },
  { from: 0x007f, to: 0x009f, class: 'control character' },
  { from: 0xd800, to: 0xdfff, class: 'unpaired surrogate' },
  { from: 0x2028, to: 0x2029, class: 'line separator' },
  { from: 0xfeff, to: 0xfeff, class: 'byte order mark' },
  { from: 0xfffe, to: 0x10ffff, class: 'above U+FFFD' },
]
*/
`

// textNotCodeRows measures the "text-not-code" mechanism.
var textNotCodeRows = []blindnessRow{
	{
		name:       "a declaration surviving only in a block comment",
		blindSpot:  "text-not-code",
		source:     declarationOnlyInABlockComment,
		wantRanges: 6,
	},
	{
		name:       "a declaration surviving only in an indented JSX comment",
		blindSpot:  "text-not-code",
		source:     declarationOnlyInAnIndentedJSXComment,
		wantRanges: 6,
	},
	{
		name:       "a declaration surviving only in a template literal",
		blindSpot:  "text-not-code",
		source:     declarationOnlyInATemplateLiteral,
		wantRanges: 6,
	},
	{
		name:       "a declaration surviving only as JSX text in a pre block",
		blindSpot:  "text-not-code",
		source:     declarationOnlyAsJSXTextInAPreBlock,
		wantRanges: 6,
	},
}

// allBlindnessRows is every blindness table in one place, for the coverage test.
// A table that is not listed here is a table the coverage test cannot see, so
// adding a table means adding it here -- which is why there is one function and
// not a literal repeated at each use.
func allBlindnessRows() [][]blindnessRow {
	return [][]blindnessRow{textNotCodeRows, literalNotValueRows}
}

// runBlindnessRows drives one blindness table over the LIVE read path.
//
// It calls browserRefusalRanges, which is the same function
// TestBrowserRefusalSetEqualsTheServers calls, readSource included. That is
// deliberate and load-bearing: a preprocessing step somebody later inserts
// between the file and the expression turns these rows RED instead of slipping
// past them, which is the one thing that keeps their promise from being
// evadable.
//
// A failure of the reader falls out of browserRefusalRanges's own t.Fatalf,
// carrying the guard's message -- and under reversed acceptance that message
// is the news, not the noise.
func runBlindnessRows(t *testing.T, rows []blindnessRow) {
	t.Helper()

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "images.tsx")
			if err := os.WriteFile(path, []byte(row.source), 0o600); err != nil {
				t.Fatalf("writing the fixture: %v", err)
			}

			ranges := browserRefusalRanges(t, path)
			if len(ranges) != row.wantRanges {
				t.Fatalf("read %d ranges, want %d.\n"+
					"This row records what the guard reads out of a source the browser "+
					"would never run. A different count is a different blindness, not the "+
					"one guardBlindSpots %q describes -- re-measure before changing the number.",
					len(ranges), row.wantRanges, row.blindSpot)
			}
		})
	}
}

// The declaration lives ONLY inside an indented {/* */} JSX comment. Indented
// on purpose: the anchor starts at a line beginning followed by whitespace, so
// indentation satisfies it -- and this comment form is established convention in
// the real route, which carries five of them.
const declarationOnlyInAnIndentedJSXComment = `import { REFUSED_RANGES } from '../lib/refusal-table'

function refused(code: number): boolean {
  return REFUSED_RANGES.some((range) => code >= range.from && code <= range.to)
}

export function SchematicNameField() {
  return (
    <div>
      {/*
      const REFUSED_RANGES: readonly RefusedRange[] = [
      { from: 0x0000, to: 0x001f, class: 'control character' },
      { from: 0x007f, to: 0x009f, class: 'control character' },
      { from: 0xd800, to: 0xdfff, class: 'unpaired surrogate' },
      { from: 0x2028, to: 0x2029, class: 'line separator' },
      { from: 0xfeff, to: 0xfeff, class: 'byte order mark' },
      { from: 0xfffe, to: 0x10ffff, class: 'above U+FFFD' },
      ]
      */}
      <input onChange={(event) => refused(event.target.value.codePointAt(0) ?? 0)} />
    </div>
  )
}
`

// The declaration lives ONLY inside a template literal, kept as documentation
// beside the form.
//
// Written as an interpreted string with \n escapes and not as a raw string: the
// fixture carries backticks, and a backtick inside a Go raw string would end it.
// A backtick has no special meaning inside an interpreted string, so this is the
// form that survives.
const declarationOnlyInATemplateLiteral = "import { REFUSED_RANGES } from '../lib/refusal-table'\n" +
	"\n" +
	"export const REFUSAL_TABLE_DOC = `\n" +
	"const REFUSED_RANGES: readonly RefusedRange[] = [\n" +
	"{ from: 0x0000, to: 0x001f, class: 'control character' },\n" +
	"  { from: 0x007f, to: 0x009f, class: 'control character' },\n" +
	"  { from: 0xd800, to: 0xdfff, class: 'unpaired surrogate' },\n" +
	"  { from: 0x2028, to: 0x2029, class: 'line separator' },\n" +
	"  { from: 0xfeff, to: 0xfeff, class: 'byte order mark' },\n" +
	"  { from: 0xfffe, to: 0x10ffff, class: 'above U+FFFD' },\n" +
	"]\n" +
	"`\n" +
	"\n" +
	"export function refused(code: number): boolean {\n" +
	"  return REFUSED_RANGES.some((range) => code >= range.from && code <= range.to)\n" +
	"}\n" +
	"\n"

// The declaration lives ONLY inside a <pre> block in the rendered help.
//
// Its children are a template literal and not raw JSX text, and that is a
// correction to this plan's own fixture sketch rather than a convenience: in
// TSX an unescaped { in JSX children opens an expression container, so a table
// of { from: 0x.., to: 0x.. } entries as literal JSX text is a syntax error, not
// a damage case. A fixture that does not compile proves nothing about a guard,
// and the plan's own acceptance criterion says no fixture may be a mere
// TypeScript error. The mechanism measured is unchanged -- the anchor sees text
// the browser renders as characters on a page.
const declarationOnlyAsJSXTextInAPreBlock = "import { REFUSED_RANGES } from '../lib/refusal-table'\n" +
	"\n" +
	"export function RefusalTableHelp() {\n" +
	"  return (\n" +
	"    <pre>{`\n" +
	"const REFUSED_RANGES: readonly RefusedRange[] = [\n" +
	"  { from: 0x0000, to: 0x001f, class: 'control character' },\n" +
	"  { from: 0x007f, to: 0x009f, class: 'control character' },\n" +
	"  { from: 0xd800, to: 0xdfff, class: 'unpaired surrogate' },\n" +
	"  { from: 0x2028, to: 0x2029, class: 'line separator' },\n" +
	"  { from: 0xfeff, to: 0xfeff, class: 'byte order mark' },\n" +
	"  { from: 0xfffe, to: 0x10ffff, class: 'above U+FFFD' },\n" +
	"]\n" +
	"`}</pre>\n" +
	"  )\n" +
	"}\n" +
	"\n" +
	"export function refused(code: number): boolean {\n" +
	"  return REFUSED_RANGES.some((range) => code >= range.from && code <= range.to)\n" +
	"}\n"

// The literal is real, complete and correct, and a filter sits between it and
// the binding. The guard reads six ranges and reports agreement; the form
// accepts U+FEFF again while the server keeps answering 400 to it.
//
// This is the dangerous direction and the reason the list is scoped by
// mechanism: it is GREEN ON REAL DRIFT, on living, compiling, referenced code,
// with no comment, string or template anywhere in it. A list of comment forms
// would be closeable to the letter while this stayed open and unledgered.
const literalFilteredBeforeItIsBound = `const REFUSED_RANGES: readonly RefusedRange[] = [
  { from: 0x0000, to: 0x001f, class: 'control character' },
  { from: 0x007f, to: 0x009f, class: 'control character' },
  { from: 0xd800, to: 0xdfff, class: 'unpaired surrogate' },
  { from: 0x2028, to: 0x2029, class: 'line separator' },
  { from: 0xfeff, to: 0xfeff, class: 'byte order mark' },
  { from: 0xfffe, to: 0x10ffff, class: 'above U+FFFD' },
].filter((range) => range.class !== 'byte order mark')

export function hasControlCharacter(value: string): boolean {
  for (const character of value) {
    const code = character.codePointAt(0) ?? 0
    if (REFUSED_RANGES.some((range) => code >= range.from && code <= range.to)) {
      return true
    }
  }
  return false
}
`

// The declaration is real and the form uses a different value: a spread of it
// together with a derived table. The guard reads the six it can see and reports
// agreement about a set the form does not consult.
//
// PLATFORM_RANGES is derived from a call rather than written as literals on
// purpose -- entry-shaped literals outside the declaration are a case this guard
// already reports, and this row is about the case it does not.
const literalSpreadIntoTheTableTheFormUses = `const REFUSED_RANGES: readonly RefusedRange[] = [
  { from: 0x0000, to: 0x001f, class: 'control character' },
  { from: 0x007f, to: 0x009f, class: 'control character' },
  { from: 0xd800, to: 0xdfff, class: 'unpaired surrogate' },
  { from: 0x2028, to: 0x2029, class: 'line separator' },
  { from: 0xfeff, to: 0xfeff, class: 'byte order mark' },
  { from: 0xfffe, to: 0x10ffff, class: 'above U+FFFD' },
]

const PLATFORM_RANGES: readonly RefusedRange[] = platformRefusals()

const ALL_REFUSED = [...REFUSED_RANGES, ...PLATFORM_RANGES]

export function hasControlCharacter(value: string): boolean {
  for (const character of value) {
    const code = character.codePointAt(0) ?? 0
    if (ALL_REFUSED.some((range) => code >= range.from && code <= range.to)) {
      return true
    }
  }
  return false
}
`

// The real table moved into another module and is imported and used; an
// indented leftover declaration stayed behind inside a function. The guard binds
// to the declaration and not to the call site, so it reads the leftover.
//
// TWO ranges and not six, and the shorter number IS the damage: the guard would
// then compare a two-entry set against the server's six and report on a table
// the form never consults. No slash, backtick or quote is involved anywhere in
// this shape -- a list of comment forms would not name it.
const indentedLeftoverDeclaration = `import { REFUSED_RANGES } from '../lib/refusal-table'

export function hasControlCharacter(value: string): boolean {
  for (const character of value) {
    const code = character.codePointAt(0) ?? 0
    if (REFUSED_RANGES.some((range) => code >= range.from && code <= range.to)) {
      return true
    }
  }
  return false
}

export function legacyRefusal(code: number): boolean {
  const REFUSED_RANGES: readonly RefusedRange[] = [
    { from: 0x0000, to: 0x001f, class: 'control character' },
    { from: 0xfeff, to: 0xfeff, class: 'byte order mark' },
  ]
  return REFUSED_RANGES.some((range) => code >= range.from && code <= range.to)
}
`

// literalNotValueRows measures the "literal-not-value" and
// "declaration-not-use" mechanisms.
//
// A table of its own, for the reason every table in this file gives: each
// table's acceptance pins the number of its rows, and that number only holds
// while the tables stay apart.
var literalNotValueRows = []blindnessRow{
	{
		name:       "a declaration whose literal is filtered before it is bound",
		blindSpot:  "literal-not-value",
		source:     literalFilteredBeforeItIsBound,
		wantRanges: 6,
	},
	{
		name:       "a declaration spread into the table the form really uses",
		blindSpot:  "literal-not-value",
		source:     literalSpreadIntoTheTableTheFormUses,
		wantRanges: 6,
	},
	{
		name:       "an indented leftover declaration beside the real table's import",
		blindSpot:  "declaration-not-use",
		source:     indentedLeftoverDeclaration,
		wantRanges: 2,
	},
}

// TestBrowserRefusalGuardBindsToALiteralAndNotToTheValueTheFormUses measures the
// two mechanisms that need no comment, no string and no template at all.
//
// REVERSED ACCEPTANCE, same instruction as the table above:
//
// A RED HERE MEANS THE BLINDNESS HAS ENDED. Delete the row, delete its
// guardBlindSpots entry once nothing measures it, and record both in
// .planning/WINDOWS.md. NEVER soften the guard to make this row green again.
func TestBrowserRefusalGuardBindsToALiteralAndNotToTheValueTheFormUses(t *testing.T) {
	runBlindnessRows(t, literalNotValueRows)
}

// TestBrowserRefusalGuardBindsToAnIdentifierAndNotToCode measures the
// text-not-code mechanism of guardBlindSpots, shape by shape.
//
// REVERSED ACCEPTANCE, and it needs its instruction written here rather than in
// a planning document the next person will not read:
//
// A RED HERE MEANS THE BLINDNESS HAS ENDED. The right answer is then to delete
// the row, delete its guardBlindSpots entry once no row measures it any more,
// and record both in .planning/WINDOWS.md. NEVER soften the guard until this
// row is green again -- a green bought that way is the exact defect this file
// has been chasing since round 3, one level up.
//
// A separate table from the three falsification tables above, for the reason
// TestBrowserRefusalGuardRefusesAnEntryItCannotRead already gives: each table's
// acceptance pins the number of its rows, and that number only holds while the
// tables stay apart.
func TestBrowserRefusalGuardBindsToAnIdentifierAndNotToCode(t *testing.T) {
	runBlindnessRows(t, textNotCodeRows)
}

// TestGuardBlindSpotsAreEachMeasured binds the written claim to the measured
// rows, in BOTH directions.
//
// A listed mechanism no row measures is a claim without evidence. A row naming
// a mechanism the list does not carry is evidence the claim does not mention.
// Either way the honest text and the measured behaviour have come apart, and
// two lists coming apart is the genus this whole file exists to catch.
func TestGuardBlindSpotsAreEachMeasured(t *testing.T) {
	measuredBy := map[string][]string{}
	for _, rows := range allBlindnessRows() {
		for _, row := range rows {
			measuredBy[row.blindSpot] = append(measuredBy[row.blindSpot], row.name)
		}
	}

	listed := map[string]bool{}
	var rowless []string
	for _, spot := range guardBlindSpots {
		if listed[spot.id] {
			t.Errorf("guardBlindSpots lists %q twice; an id is what a row refers to, "+
				"so two entries under one id make the reference ambiguous", spot.id)
		}
		listed[spot.id] = true

		if spot.rowless != "" {
			rowless = append(rowless, spot.id)
			if rows := measuredBy[spot.id]; len(rows) > 0 {
				t.Errorf("guardBlindSpots marks %q as unmeasurable by a row, and %d rows "+
					"measure it: %v.\nOne of the two is wrong: either the reason in "+
					"rowless no longer holds and the field goes, or the rows measure "+
					"something else and name the wrong id",
					spot.id, len(rows), rows)
			}
			continue
		}

		if len(measuredBy[spot.id]) == 0 {
			t.Errorf("guardBlindSpots lists %q and no blindness row measures it.\n"+
				"A listed mechanism without a measured shape is a claim without evidence, "+
				"which is the thing this list replaced. Add a row that measures it, or "+
				"mark it rowless with the reason no row can.", spot.id)
		}
	}

	for id, rows := range measuredBy {
		if !listed[id] {
			t.Errorf("blindness rows %v name the mechanism %q, and guardBlindSpots does "+
				"not carry it.\nThe rendered claim in honestClaim therefore does not "+
				"mention a blindness this file measures -- the guard would understate "+
				"itself, which is the same defect as overstating it with the sign flipped.",
				rows, id)
		}
	}

	if len(rowless) != 1 || rowless[0] != "surrogate-interior" {
		t.Errorf("guardBlindSpots carries %d rowless entries (%v), want exactly one, "+
			"surrogate-interior.\nThe rowless field is the one exemption from being "+
			"measured, and an exemption more than one entry can take is a hole a future "+
			"entry escapes measurement through.", len(rowless), rowless)
	}
}

// stringArrayLiteral reads the single-quoted members of a named TypeScript
// array. It is deliberately literal-only: a member computed at runtime is a
// member this guard cannot see, and failing is the honest answer.
func stringArrayLiteral(t *testing.T, source, name string) []string {
	t.Helper()

	// The assignment, not the first bracket after the name: the type annotation
	// `readonly string[]` sits between the two and would otherwise be read as an
	// empty array.
	body := regexp.MustCompile(regexp.QuoteMeta(name) + `[^=]*=\s*\[([^\]]*)\]`).
		FindStringSubmatch(source)
	if body == nil {
		t.Fatalf("%s declares no %s as an array literal; that array is the seam "+
			"this guard binds to", browserSuitePath, name)
	}

	members := regexp.MustCompile(`'([^']*)'`).FindAllStringSubmatch(body[1], -1)
	if len(members) == 0 {
		t.Fatalf("%s: %s carries no string literals this guard can read", browserSuitePath, name)
	}

	out := make([]string, 0, len(members))
	for _, m := range members {
		out = append(out, m[1])
	}
	return out
}

func readSource(t *testing.T, path string) string {
	t.Helper()
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(source)
}

// sampleCodepoints renders a bounded, deterministic sample. A drift of a whole
// plane would otherwise print a million lines and bury the one fact that
// matters: which range moved.
func sampleCodepoints(cps []rune) string {
	shown := cps
	suffix := ""
	if len(shown) > maxReportedDrifts {
		shown = shown[:maxReportedDrifts]
		suffix = fmt.Sprintf(" ... and %d more", len(cps)-maxReportedDrifts)
	}
	parts := make([]string, 0, len(shown))
	for _, r := range shown {
		parts = append(parts, fmt.Sprintf("%U", r))
	}
	return strings.Join(parts, ", ") + suffix
}
