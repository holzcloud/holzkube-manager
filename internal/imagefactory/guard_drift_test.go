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

var refusedRangesDecl = regexp.MustCompile(
	`(?ms)^\s*(?:export\s+)?const\s+` + regexp.QuoteMeta(refusedRangesName) +
		`\s*(?::[^=\n]*)?=\s*\[(.*?)\]`)

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
	ranges := browserRefusalRanges(t)

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

// browserRefusalRanges reads the declared table out of the route.
//
// An empty result is a failure and not a skip. A guard that silently passes
// when it can no longer find what it guards is worse than no guard: it reports
// agreement it never checked.
//
// The reading itself is parseBrowserRefusalRanges, which takes a string and
// returns an error instead of taking a *testing.T. That split is the whole
// point of this round: a guard whose failure nothing checks is exactly the
// class of defect being closed here, and with t.Fatalf inside the reader its
// failure cases cannot be tested at all. Only the file read and the fatal stay
// on this side of the line.
func browserRefusalRanges(t *testing.T) []declaredRange {
	t.Helper()

	ranges, err := parseBrowserRefusalRanges(readSource(t, imagesRoutePath))
	if err != nil {
		// The reason lives in the error, which is where it is testable now.
		// This says only which file was read.
		t.Fatalf("%s: %v", imagesRoutePath, err)
	}
	return ranges
}

// parseBrowserRefusalRanges reads the refusal table out of images.tsx source.
//
// It is pure so that its failure cases are themselves testable; see
// browserRefusalRanges for why that matters.
//
// It reads the declaration and not the file. Renamed, moved or deleted is the
// same as never having been there, and other entry-shaped literals surviving
// elsewhere in the file does not make it better -- that is precisely the state
// round 4 measured as a green run.
func parseBrowserRefusalRanges(source string) ([]declaredRange, error) {
	decl := refusedRangesDecl.FindStringSubmatch(source)
	if decl == nil {
		return nil, fmt.Errorf("no %s declared as an array literal.\n"+
			"The browser's set has to be data -- a named array of {from: 0x.., to: 0x..} "+
			"entries -- so that this guard can compare it to the server's. A chain of "+
			"comparisons inside an if is unreadable from here, and while it was one, the "+
			"two sets drifted (G-02-11). Renamed, moved or deleted is the same as never "+
			"having been there, and entry-shaped literals surviving elsewhere in the file "+
			"do not make it better. A name that merely carries %s as a prefix -- "+
			"%s_LEGACY, %sX -- is a different name and does not satisfy this guard "+
			"either; that leftover is what stays behind when the real table moves away",
			refusedRangesName, refusedRangesName, refusedRangesName, refusedRangesName)
	}
	body := decl[1]

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
	if len(matches) == 0 {
		return nil, fmt.Errorf("%s is declared but carries no entries this guard can read.\n"+
			"This is the declaration being empty, not the declaration being absent; "+
			"the two are separate failures because they call for separate fixes",
			refusedRangesName)
	}

	// The pollution direction, and the truncation direction, in one comparison.
	// An entry-shaped literal outside the declaration is one this guard now
	// correctly ignores but somebody should look at; a body that ended early
	// because an entry carries a closing bracket inside a string produces the
	// same inequality. This guard cannot tell the two apart, so it names both.
	whole := len(browserRefusalRange.FindAllString(source, -1))
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
		// the literal and the check below says what is wrong with it.
		if to > utf8.MaxRune || from > to {
			return nil, fmt.Errorf("this entry of %s has bounds this guard cannot represent: "+
				"from 0x%s to 0x%s.\n"+
				"The upper bound has to be at most utf8.MaxRune and the lower bound at most the "+
				"upper. This guard compares SETS by sweeping every codepoint upwards from "+
				"rune(0), and rune(uint64) is lossy: a bound above utf8.MaxRune lands on a "+
				"negative rune, so such an entry covers no codepoint at all and is passed over. "+
				"An entry it passes over is a codepoint it reports agreement about without "+
				"having compared it -- the same reason the unreadable-entry check above gives",
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

	for _, tc := range []struct {
		name       string
		source     string
		wantErr    string
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
			wantErr: "REFUSED_RANGES",
		},
		{
			name:    "the declaration absent",
			source:  absent,
			wantErr: "REFUSED_RANGES",
		},
		{
			name:    "an entry-shaped literal outside the declaration",
			source:  polluted,
			wantErr: "1 entries inside the REFUSED_RANGES declaration, 2 in the source as a whole",
		},
		{
			name:    "the declaration renamed to a prefixed name",
			source:  prefixed,
			wantErr: "REFUSED_RANGES",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ranges, err := parseBrowserRefusalRanges(tc.source)

			if tc.wantErr == "" {
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
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error does not name %q:\n%v", tc.wantErr, err)
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
			wantErr: []string{"from 0x0000 to 0xFFFFFFFF", "cannot represent"},
		},
		{
			name:    "a real lower bound with an unrepresentable upper",
			source:  realLowerUnrepresentableUpper,
			wantErr: []string{"from 0x0061 to 0xFFFFFFFF", "cannot represent"},
		},
		{
			name:    "an inverted range",
			source:  inverted,
			wantErr: []string{"from 0x001f to 0x0000", "cannot represent"},
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
