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
func browserRefusalRanges(t *testing.T) []declaredRange {
	t.Helper()
	source := readSource(t, imagesRoutePath)

	matches := browserRefusalRange.FindAllStringSubmatch(source, -1)
	if len(matches) == 0 {
		t.Fatalf("%s declares no refusal ranges this guard can read.\n"+
			"The browser's set has to be data -- a named array of {from: 0x.., to: 0x..} "+
			"entries -- so that this test can compare it to the server's. A chain of "+
			"comparisons inside an if is unreadable from here, and while it was one, "+
			"the two sets drifted (G-02-11).", imagesRoutePath)
	}

	out := make([]declaredRange, 0, len(matches))
	for _, m := range matches {
		from, err := strconv.ParseUint(m[1], 16, 32)
		if err != nil {
			t.Fatalf("%s: unreadable range start %q: %v", imagesRoutePath, m[1], err)
		}
		to, err := strconv.ParseUint(m[2], 16, 32)
		if err != nil {
			t.Fatalf("%s: unreadable range end %q: %v", imagesRoutePath, m[2], err)
		}
		out = append(out, declaredRange{from: rune(from), to: rune(to)})
	}
	return out
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
