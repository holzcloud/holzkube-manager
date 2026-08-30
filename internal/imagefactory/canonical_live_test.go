package imagefactory_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/holzcloud/holzkube/internal/imagefactory"
)

// TestLiveCanonical is the differential behind G-02-16.
//
// schematicid.go is a transcription of upstream's YAML emitter, and a
// transcription cannot be checked against the thing it was transcribed from by
// reading it. Nothing in this repository tested any codepoint above U+007F, and
// the audit that found this sampled nine and broke four. So the oracle here is
// the Factory and only the Factory: for each candidate scalar the local
// canonical document and the local id are compared against Created.Canonical
// and Created.ID, which are the Factory's own normalised document and the hash
// it filed it under.
//
// A second local YAML library would not do. It would replace one transcription
// with a different transcription and prove nothing about upstream, which is why
// this plan adds no module.
//
// Cost, because this test creates schematics on a public service and a reader
// is entitled to weigh that before running it: the corpus is 78 sendable
// scalars per document path, swept in batches of canonicalBatchSize, over two
// document paths -- 16 POSTs when nothing diverges. A batch that the Factory
// refuses outright answers with no document, so it is bisected; each such batch
// costs at most a further 2*canonicalBatchSize-1 POSTs. A batch that the
// Factory answers with a document needs no bisection at all: the document names
// the diverging entries by itself.
//
// Every budget in this package belongs to the still-open
// 02-DECISION-probe-budget.md. There is no retry here and no raised timeout.

// notObservedMarker is the string live_test.go already uses for the same
// reason, and this file reuses it deliberately. A skipped upstream check that
// prints nothing is indistinguishable from a check that passed. Every message
// carrying this marker also names the codepoints or ranges that went
// unmeasured, in U+ form, because "something was skipped" is not information an
// operator or a later plan can act on.
const notObservedMarker = "NOT OBSERVED:"

// canonicalBatchSize is how many corpus scalars ride in a single POST.
//
// One schematic carries many kernel arguments, so a batch is one request
// covering many codepoints. Ten keeps a clean run to tens of requests while
// keeping a bisection over a refused batch short.
const canonicalBatchSize = 10

// canonicalCorpusRanges is what a full run of this test reaches, in U+ form.
// It is a floor and not a ceiling: see canonicalUnreachedRanges.
const canonicalCorpusRanges = "U+0000, U+0009, U+000A, U+000D, U+001F, U+007F, " +
	"U+0080, U+0081, U+0085, U+008D, U+0094, U+009F, U+00A0, U+00E4, U+200B, U+2028, " +
	"U+2029, U+202E, U+4E2D, U+D7FF, U+E000, U+FDD0, U+FEFF, U+FFFD, U+FFFE, U+FFFF, " +
	"U+1F600, U+10FFFF"

// canonicalUnreachedRanges is what no run of this test reaches, whatever the
// outcome. Naming it is the difference between "these classes were measured to
// diverge" and the claim no finite sweep can make.
const canonicalUnreachedRanges = "every other codepoint in U+0020-U+D7FF and U+E000-U+10FFFF, " +
	"in particular the remainder of U+0080-U+009F beyond the six sampled, " +
	"U+FDD1-U+FDEF, and every non-BMP plane above U+1F600 apart from U+10FFFF"

// verdict is the three-way classification. Collapsing any two of these is how
// G-02-16 happened, so they are named apart and never merged.
type verdict string

const (
	// agrees: the local document and the local id equal the Factory's.
	agrees verdict = "AGREES"
	// diverges: both were produced and they differ. This is the finding.
	diverges verdict = "DIVERGES"
	// refusedLocally: Schematic.ID() refused, so nothing was sent. Safe, but
	// not a pass -- a codepoint refused here that the Factory would have
	// carried faithfully is an over-refusal the operator pays for.
	refusedLocally verdict = "REFUSED LOCALLY"
	// notObserved: a transport failure or a throttle. Not one of the three
	// outcomes; a non-observation.
	notObserved verdict = "NOT OBSERVED"
)

// The three shapes a divergence takes. The audit found all three, and which one
// a codepoint is decides what the refusal has to prevent.
const (
	shapeUnparseable  = "the Factory would not parse the document"
	shapeRenormalised = "the Factory re-normalised the document"
	shapeAltered      = "the Factory silently altered the scalar"
)

// canonCandidate is one corpus entry: a codepoint, the rule it probes, and the
// scalar carrying it.
type canonCandidate struct {
	cp       rune
	rule     string
	position string
	style    string
	scalar   string
	control  bool // a negative control: it must be accepted and must round-trip
}

func (c canonCandidate) label() string {
	return fmt.Sprintf("%U %s/%s", c.cp, c.position, c.style)
}

// canonResult is one recorded row of the three-way table.
type canonResult struct {
	cand    canonCandidate
	path    string
	verdict verdict
	shape   string
	detail  string
}

// scalarFor places cp at a position inside a scalar of a given quoting style.
//
// Quoting style is decided by renderScalar, and a divergence that only shows up
// in one style is invisible to the other -- so every interesting codepoint is
// probed twice: once inside a scalar that stays plain, once inside a scalar
// whose colon-space already forces the single-quoted form.
func scalarFor(cp rune, position, style string) string {
	base := "console=ttyS0"
	if style == "quoted" {
		base = "a: console=ttyS0"
	}
	switch position {
	case "leading":
		return string(cp) + base
	case "trailing":
		return base + string(cp)
	default:
		i := strings.Index(base, "=") + 1
		return base[:i] + string(cp) + base[i:]
	}
}

// canonicalCorpus is built by class boundary rather than by enumeration, and
// every group says which rule it probes.
func canonicalCorpus() []canonCandidate {
	var out []canonCandidate
	add := func(rule string, control bool, positions, styles []string, cps ...rune) {
		for _, cp := range cps {
			for _, p := range positions {
				for _, s := range styles {
					out = append(out, canonCandidate{
						cp: cp, rule: rule, position: p, style: s,
						scalar: scalarFor(cp, p, s), control: control,
					})
				}
			}
		}
	}

	allPositions := []string{"leading", "interior", "trailing"}
	interior := []string{"interior"}
	leadIn := []string{"leading", "interior"}
	styles := []string{"plain", "quoted"}

	// The C0 range and U+007F, which the current code already refuses. They
	// establish that the REFUSED LOCALLY outcome is reachable and correct; a
	// sweep whose refusal outcome never fires proves nothing about refusal.
	add("C0 control or U+007F: already refused before this plan", false, interior, styles,
		0x00, 0x09, 0x0A, 0x0D, 0x1F, 0x7F)

	// The C1 range. YAML's printable set excludes U+0080-U+009F entirely, and
	// U+0085 is additionally a line break. Three positions, because the audit
	// found the trailing one behaves differently: a trailing U+0085 was eaten
	// rather than refused, so the stored kernel_args and canonical disagreed.
	add("C1 control (U+0080-U+009F), outside YAML's printable set; U+0085 is also a line break",
		false, allPositions, styles, 0x80, 0x81, 0x85, 0x8D, 0x94, 0x9F)

	// The two line breaks above the C1 range.
	add("YAML line break above the C1 range (LS/PS)", false, allPositions, styles, 0x2028, 0x2029)

	// The byte order mark, and specifically at the start of the scalar, where
	// a BOM means something structural to a YAML reader.
	add("byte order mark, outside YAML's printable set", false, leadIn, styles, 0xFEFF)

	// Non-characters: permanently unassigned, valid UTF-8, and a plausible
	// place for two emitters to disagree.
	add("Unicode non-character", false, interior, styles, 0xFDD0, 0xFFFE, 0xFFFF)

	// The boundaries of the printable ranges themselves.
	add("boundary of a printable range", false, interior, styles, 0xD7FF, 0xE000, 0xFFFD, 0x10FFFF)

	// Above the BMP. This was written into the plan as part of the negative
	// control -- an emoji is printable, so it was expected to round-trip -- and
	// the measurement said otherwise: the Factory escapes it to "\U0001F600",
	// exactly as it escapes U+10FFFF. Leaving it in the control group would
	// make this guard fail on its own finding, so it is where the measurement
	// put it and not where the expectation did.
	add("above the BMP: expected to round-trip, measured to be escaped", false, interior, styles, 0x1F600)

	// The negative control. Without it the sweep can only discover that
	// refusing everything is safe.
	add("negative control: printable above U+007F, must be accepted and must round-trip",
		true, interior, styles, 0x00A0, 0x00E4, 0x200B, 0x202E, 0x4E2D)

	return out
}

// httpStatusInError reads the status out of an ErrUpstreamUnavailable message.
//
// The client folds every non-2xx into ErrUpstreamUnavailable and carries the
// code in the message; it exposes no status. That fold is exactly the defect
// G-02-11 is about -- a 400 the Factory answered with becomes "the Image
// Factory did not answer usably" -- and this test must not inherit it, because
// under it a document the Factory refused would be filed as a throttle and the
// finding would disappear into a non-observation.
var httpStatusInError = regexp.MustCompile(`HTTP (\d{3})`)

func answeredStatus(err error) int {
	if err == nil {
		return 0
	}
	m := httpStatusInError.FindStringSubmatch(err.Error())
	if m == nil {
		return 0
	}
	code, convErr := strconv.Atoi(m[1])
	if convErr != nil {
		return 0
	}
	return code
}

// factoryRefused reports whether the Factory read the document and refused it,
// as opposed to never answering. 408 and 429 are 4xx codes that say nothing
// about the document, so they stay non-observations -- the same distinction
// registryRefused draws in installer.go.
func factoryRefused(err error) (int, bool) {
	code := answeredStatus(err)
	return code, code >= 400 && code < 500 && code != 408 && code != 429
}

// canonSweep carries the per-path state of one sweep.
type canonSweep struct {
	t       *testing.T
	client  *imagefactory.Client
	ctx     context.Context
	path    string
	build   func([]canonCandidate) imagefactory.Schematic
	lineFor func(local []string, i int) string
	results *[]canonResult
}

func (s *canonSweep) record(c canonCandidate, v verdict, shape, detail string) {
	*s.results = append(*s.results, canonResult{cand: c, path: s.path, verdict: v, shape: shape, detail: detail})
}

func (s *canonSweep) recordAll(cands []canonCandidate, v verdict, shape, detail string) {
	for _, c := range cands {
		s.record(c, v, shape, detail)
	}
}

// probe classifies a batch, splitting it only when it has to.
func (s *canonSweep) probe(cands []canonCandidate) {
	if len(cands) == 0 {
		return
	}
	schematic := s.build(cands)
	local, err := schematic.Canonical()
	if err != nil {
		// Pre-classification already removed everything ID() refuses, so a
		// refusal here means a batch is refused that none of its members is.
		s.recordAll(cands, refusedLocally, "", "the batch was refused locally: "+err.Error())
		return
	}
	localID, err := schematic.ID()
	if err != nil {
		s.recordAll(cands, refusedLocally, "", "the batch was refused locally: "+err.Error())
		return
	}

	created, err := s.client.CreateSchematic(s.ctx, schematic)

	switch {
	case err == nil:
		// Byte comparison first: an id mismatch says only that something is
		// wrong, the document diff says what.
		if created.Canonical == string(local) && created.ID == localID {
			s.recordAll(cands, agrees, "", "")
			return
		}
		// A document that matches byte for byte cannot hash differently, so
		// this is unreachable in practice; classify rather than assume.
		s.split(cands, created.Canonical, string(local))

	case errors.Is(err, imagefactory.ErrSchematicNotRepresentable):
		s.recordAll(cands, refusedLocally, "", err.Error())

	case errors.Is(err, imagefactory.ErrSchematicIDMismatch):
		// The Factory answered with its own document. That document names the
		// diverging entries by itself, so no bisection is needed.
		s.split(cands, created.Canonical, string(local))

	default:
		if code, refused := factoryRefused(err); refused {
			if len(cands) == 1 {
				s.record(cands[0], diverges, shapeUnparseable,
					fmt.Sprintf("the Factory answered HTTP %d to the document carrying %s; "+
						"local document:\n%s", code, cands[0].label(), string(local)))
				return
			}
			mid := len(cands) / 2
			s.probe(cands[:mid])
			s.probe(cands[mid:])
			return
		}
		// A transport failure, a throttle, a 5xx. factory.talos.dev throttles
		// without an HTTP response (WINDOWS entry 5), and a guard that fails
		// on a throttle is trained out of attention.
		s.recordAll(cands, notObserved, "", err.Error())
		s.t.Logf("%s the %s sweep did not observe %s: %v",
			notObservedMarker, s.path, labels(cands), err)
	}
}

// split maps a returned document back onto the entries that produced it.
//
// Each candidate contributes exactly one line to the local document and every
// candidate's scalar is distinct, so a local line missing from the Factory's
// document is that candidate diverging. When the mapping does not hold -- the
// Factory added, dropped or reordered lines -- it falls back to bisection.
func (s *canonSweep) split(cands []canonCandidate, factoryDoc, localDoc string) {
	localLines := strings.Split(localDoc, "\n")
	factoryLines := strings.Split(factoryDoc, "\n")
	present := make(map[string]bool, len(factoryLines))
	for _, l := range factoryLines {
		present[l] = true
	}

	var found int
	pending := make([]canonCandidate, 0, len(cands))
	for i, c := range cands {
		line := s.lineFor(localLines, i)
		if line == "" {
			pending = append(pending, c)
			continue
		}
		if present[line] {
			s.record(c, agrees, "", "")
			continue
		}
		found++
		s.record(c, diverges, shapeOf(c, factoryLines), fmt.Sprintf(
			"local line %q is absent from the Factory's document.\nlocal:\n%s\nfactory:\n%s",
			line, localDoc, factoryDoc))
	}

	if found == 0 && len(pending) == 0 {
		// Every line survived and the documents still disagree: the difference
		// is structural, not per-entry. Bisect rather than guess.
		if len(cands) == 1 {
			s.record(cands[0], diverges, shapeRenormalised, fmt.Sprintf(
				"the documents differ without any entry line changing.\nlocal:\n%s\nfactory:\n%s",
				localDoc, factoryDoc))
			return
		}
		mid := len(cands) / 2
		s.probe(cands[:mid])
		s.probe(cands[mid:])
		return
	}
	if len(pending) > 0 {
		if len(pending) == len(cands) && len(cands) == 1 {
			s.record(cands[0], diverges, shapeRenormalised, "the local line could not be located")
			return
		}
		mid := len(pending) / 2
		if mid == 0 {
			mid = 1
		}
		s.probe(pending[:mid])
		s.probe(pending[mid:])
	}
}

// shapeOf decides which of the two document-bearing shapes a divergence is,
// and it decides it on the value rather than on the bytes.
//
// The distinction is the whole difference between T-02-74 and T-02-75. The
// Factory escapes a scalar it will not write literally -- "console=\x80ttyS0"
// carries exactly the string that was sent, so only the id moved. It also
// folds a line break into a space, and eats a trailing one, and those destroy
// the operator's value: the stored kernel_args and the stored canonical then
// describe different images. A shape test that asked only whether the raw
// bytes reappear reports the first case as the second, which is a lie in the
// direction that matters.
//
// So: if any line of the Factory's document decodes back to the scalar that
// was sent, the value survived and the rendering moved. If none does, the
// value did not survive.
func shapeOf(c canonCandidate, factoryLines []string) string {
	for _, l := range factoryLines {
		if v, ok := decodeRenderedValue(l); ok && v == c.scalar {
			return shapeRenormalised
		}
	}
	return shapeAltered
}

// decodeRenderedValue reads back the scalar a rendered document line carries.
//
// It covers exactly the three styles this document format produces -- plain,
// single-quoted, and the double-quoted form with the escapes upstream's emitter
// writes. It is deliberately not a YAML parser: a parser would be a second
// transcription of upstream, which is the thing this whole file exists to avoid
// relying on. It is only ever used to label a shape, never to decide a verdict;
// a line it cannot read reports ok=false and the divergence is still recorded.
func decodeRenderedValue(line string) (string, bool) {
	s := strings.TrimLeft(line, " ")
	s = strings.TrimPrefix(s, "- ")
	// A meta value's own line. A scalar that literally began "value: " would
	// have been quoted, so its rendered form starts with a quote and cannot be
	// confused with the key.
	s = strings.TrimPrefix(s, "value: ")
	if s == "" {
		return "", false
	}

	switch s[0] {
	case '\'':
		if len(s) < 2 || s[len(s)-1] != '\'' {
			return "", false
		}
		return strings.ReplaceAll(s[1:len(s)-1], "''", "'"), true
	case '"':
		if len(s) < 2 || s[len(s)-1] != '"' {
			return "", false
		}
		return unescapeDoubleQuoted(s[1 : len(s)-1])
	default:
		return s, true
	}
}

// unescapeDoubleQuoted decodes the escapes upstream's emitter actually writes.
func unescapeDoubleQuoted(s string) (string, bool) {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			i++
			continue
		}
		i++
		if i >= len(s) {
			return "", false
		}
		switch s[i] {
		case '0':
			b.WriteByte(0)
		case 'a':
			b.WriteByte(7)
		case 'b':
			b.WriteByte(8)
		case 't':
			b.WriteByte('\t')
		case 'n':
			b.WriteByte('\n')
		case 'v':
			b.WriteByte(11)
		case 'f':
			b.WriteByte(12)
		case 'r':
			b.WriteByte('\r')
		case 'e':
			b.WriteByte(27)
		case ' ', '"', '\\', '/':
			b.WriteByte(s[i])
		case 'N':
			b.WriteRune(0x85)
		case '_':
			b.WriteRune(0xA0)
		case 'L':
			b.WriteRune(0x2028)
		case 'P':
			b.WriteRune(0x2029)
		case 'x', 'u', 'U':
			width := map[byte]int{'x': 2, 'u': 4, 'U': 8}[s[i]]
			if i+1+width > len(s) {
				return "", false
			}
			cp, err := strconv.ParseUint(s[i+1:i+1+width], 16, 32)
			if err != nil {
				return "", false
			}
			b.WriteRune(rune(cp))
			i += width
		default:
			return "", false
		}
		i++
	}
	return b.String(), true
}

func labels(cands []canonCandidate) string {
	out := make([]string, 0, len(cands))
	for _, c := range cands {
		out = append(out, c.label())
	}
	return strings.Join(out, ", ")
}

// kernelArgsSchematic puts one candidate per extraKernelArgs entry. Each entry
// is one document line at a known index.
func kernelArgsSchematic(cands []canonCandidate) imagefactory.Schematic {
	args := make([]string, 0, len(cands))
	for _, c := range cands {
		args = append(args, c.scalar)
	}
	return imagefactory.Schematic{Customization: imagefactory.Customization{ExtraKernelArgs: args}}
}

// kernelArgsLine: the document is customization / extraKernelArgs / one "- x"
// line per entry, so entry i is line 2+i.
func kernelArgsLine(local []string, i int) string {
	if 2+i >= len(local) {
		return ""
	}
	return local[2+i]
}

// metaSchematic sweeps the same corpus through customization.meta[].value,
// which reaches continuationField rather than item and is the one path with a
// different indentation rule.
func metaSchematic(cands []canonCandidate) imagefactory.Schematic {
	meta := make([]imagefactory.MetaValue, 0, len(cands))
	for i, c := range cands {
		meta = append(meta, imagefactory.MetaValue{Key: uint8(i), Value: c.scalar}) //nolint:gosec // i < canonicalBatchSize
	}
	return imagefactory.Schematic{Customization: imagefactory.Customization{Meta: meta}}
}

// metaLine: two lines per entry -- "- key: N" then "  value: x" -- so entry i's
// value is line 2+2*i+1.
func metaLine(local []string, i int) string {
	idx := 2 + 2*i + 1
	if idx >= len(local) {
		return ""
	}
	return local[idx]
}

func TestLiveCanonical(t *testing.T) {
	if os.Getenv(liveEnv) != "1" {
		t.Skipf("%s skipping the live canonical differential: %s is not set to 1. "+
			"Nothing in this run compared Schematic.Canonical() or Schematic.ID() against the "+
			"document and id factory.talos.dev returns, so this run measured none of "+
			"%s -- and no run of this test reaches %s. A serialiser that has drifted from "+
			"upstream above U+007F looks exactly like this.",
			notObservedMarker, liveEnv, canonicalCorpusRanges, canonicalUnreachedRanges)
	}

	client, err := imagefactory.New(imagefactory.DefaultBaseURL, imagefactory.WithTimeout(60*time.Second))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := t.Context()

	corpus := canonicalCorpus()
	var results []canonResult

	for _, path := range []string{"customization.extraKernelArgs", "customization.meta.value"} {
		sweep := &canonSweep{
			t: t, client: client, ctx: ctx, path: path,
			build:   kernelArgsSchematic,
			lineFor: kernelArgsLine,
			results: &results,
		}
		if path == "customization.meta.value" {
			sweep.build = metaSchematic
			sweep.lineFor = metaLine
		}

		// Pre-classification, offline: a candidate ID() refuses never reaches a
		// batch, because one refused member would refuse the whole POST and
		// hide every other member's verdict behind it.
		sendable := make([]canonCandidate, 0, len(corpus))
		for _, c := range corpus {
			single := sweep.build([]canonCandidate{c})
			if _, idErr := single.ID(); idErr != nil {
				if errors.Is(idErr, imagefactory.ErrSchematicNotRepresentable) {
					sweep.record(c, refusedLocally, "", idErr.Error())
					continue
				}
				t.Fatalf("ID() failed for %s in an unexpected way: %v", c.label(), idErr)
			}
			sendable = append(sendable, c)
		}

		for i := 0; i < len(sendable); i += canonicalBatchSize {
			end := min(i+canonicalBatchSize, len(sendable))
			sweep.probe(sendable[i:end])
		}
	}

	reportCanonResults(t, results)
}

// reportCanonResults prints the three-way table and turns it into assertions.
//
// A DIVERGES row is a defect once the refusal set has been widened to cover it:
// it means this serialiser and upstream's have drifted again. A negative
// control that comes back REFUSED LOCALLY is the opposite defect -- an
// over-refusal, which costs the operator a schematic they were entitled to.
func reportCanonResults(t *testing.T, results []canonResult) {
	t.Helper()

	var counts = map[verdict]int{}
	var b strings.Builder
	b.WriteString("live canonical differential against factory.talos.dev\n")
	b.WriteString("| codepoint | position/style | path | verdict | shape | rule |\n")
	for _, r := range results {
		counts[r.verdict]++
		fmt.Fprintf(&b, "| %U | %s/%s | %s | %s | %s | %s |\n",
			r.cand.cp, r.cand.position, r.cand.style, r.path, r.verdict, r.shape, r.cand.rule)
	}
	t.Log(b.String())
	t.Logf("agrees=%d diverges=%d refused-locally=%d not-observed=%d",
		counts[agrees], counts[diverges], counts[refusedLocally], counts[notObserved])

	for _, r := range results {
		switch {
		case r.verdict == diverges:
			t.Errorf("DIVERGES %s in %s (%s): %s\n%s",
				r.cand.label(), r.path, r.cand.rule, r.shape, r.detail)
		case r.verdict == refusedLocally && r.cand.control:
			t.Errorf("OVER-REFUSAL: %s in %s is a negative control and must be accepted: %s",
				r.cand.label(), r.path, r.detail)
		case r.verdict == notObserved:
			t.Logf("%s %s in %s: %s", notObservedMarker, r.cand.label(), r.path, r.detail)
		}
	}
	if counts[notObserved] > 0 {
		t.Logf("%s %d of %d rows went unmeasured in this run; the ranges no run reaches at all are %s",
			notObservedMarker, counts[notObserved], len(results), canonicalUnreachedRanges)
	}
}
