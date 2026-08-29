package imagefactory

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// ErrSchematicNotRepresentable reports a schematic carrying a scalar this
// package will not render.
//
// It exists so that a schematic outside the domain the canonical emitter was
// pinned against produces a refusal rather than an id. A wrong id is worse than
// no id: FACT-06 exists so holzkube can recognise "this schematic already
// exists" without a round trip, and a locally computed value that silently
// disagrees with the Factory's turns that optimisation into a lie. Callers that
// hit this can still create the schematic -- CreateSchematic gets the id from
// the Factory -- they just lose the round-trip saving.
var ErrSchematicNotRepresentable = errors.New("imagefactory: schematic contains a value the canonical serialiser will not render")

// NotRepresentableError names which value in which field the canonical
// serialiser refused.
//
// The sentinel says that something was refused; this says what and where, which
// is the difference between an HTTP 400 naming a request field and an HTTP 502
// blaming factory.talos.dev for a failure that never left this process. The
// refusal is raised by Schematic.ID() before the extension catalog is fetched
// and before any POST, so there is no upstream involved and no retry that can
// help -- and telling an operator otherwise is the specific harm this type
// exists to prevent (G-02-6).
//
// It follows UnknownExtensionsError: a struct carrying the specifics, an
// Error() that reads as a sentence, and Unwrap() so the sentinel stays reachable
// through errors.Is.
type NotRepresentableError struct {
	// Path is the canonical document path of the offending scalar, e.g.
	// customization.extraKernelArgs. It is the document's vocabulary, not the
	// HTTP request's; mapping one onto the other belongs to the layer that
	// knows the request shape.
	Path string

	// Index is the zero-based position within a sequence, or -1 for a scalar
	// field that has no position. Error() renders it one-based, because an
	// operator counting rows in a form starts at one.
	Index int

	// Reason names the character class and never the value. Kernel arguments
	// can carry secrets -- that is why the Factory offers no way to enumerate
	// schematics -- and this string ends up in a problem body, which is
	// rendered in a browser, may be logged by a proxy, and outlives the form
	// that produced it (T-02-64).
	Reason string
}

func (e *NotRepresentableError) Error() string {
	where := e.Path
	if e.Index >= 0 {
		where = fmt.Sprintf("%s entry %d", e.Path, e.Index+1)
	}
	return fmt.Sprintf("%s: %s %s", ErrSchematicNotRepresentable.Error(), where, e.Reason)
}

// Unwrap makes errors.Is(err, ErrSchematicNotRepresentable) true, so this type
// is an addition to the sentinel rather than a replacement for it.
func (e *NotRepresentableError) Unwrap() error { return ErrSchematicNotRepresentable }

// canonicalIndent is the number of spaces per nesting level in the Factory's
// canonical form. Four, not two: verified against the live Factory, which
// re-serialises a two-space document into a four-space one and hashes that.
const canonicalIndent = 4

// ID returns the schematic id the Factory will assign to this schematic,
// computed locally.
//
// The id is the lowercase hex SHA-256 of the canonical document -- verified
// against the live Factory across a corpus of recorded documents, every one of
// which satisfies id == sha256(canonical). Computing it locally is what lets
// holzkube recognise a schematic it has seen before without a round trip
// (FACT-06); the Factory deliberately offers no way to list schematics, so an
// id that is neither stored nor recoverable from a running node is gone.
//
// It is a precomputation, never a substitute for the Factory's answer.
// CreateSchematic compares the two and refuses to proceed when they disagree,
// because a disagreement means this serialiser and the upstream one have
// drifted and every id computed here is suspect.
func (s Schematic) ID() (string, error) {
	doc, err := s.Canonical()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(doc)
	return hex.EncodeToString(sum[:]), nil
}

// Canonical renders the schematic in the exact form the Factory canonicalises
// it to, which is also the form this package POSTs.
//
// The serialisation is written out explicitly rather than delegated to a YAML
// marshaller. That is the same discipline internal/audit applies to the hash
// chain and for a related reason, though the direction is reversed: the audit
// chain must not move when a library's escaping changes, whereas this one must
// not move when a library's escaping changes *differently from upstream's*.
// Either way, bytes that decide a hash are written by code that can be read.
//
// The rules, all established by observation against factory.talos.dev:
//
//   - Four-space indentation; block sequences are indented one level under
//     their key.
//   - Field order is owner, overlay, customization. Order is not alphabetical
//     and is not the order a caller supplies -- it is the declaration order of
//     the upstream struct.
//   - owner and overlay are omitted when zero; customization is always
//     emitted, as "{}" when empty.
//   - Within a present overlay, image and name are both always emitted.
//   - Sequence order is preserved, so a reordered extension list is a
//     different schematic.
//   - Long scalars are never line-folded.
func (s Schematic) Canonical() ([]byte, error) {
	var w canonWriter

	if s.Owner != "" {
		w.field(0, "owner", s.Owner, "owner")
	}

	if !s.Overlay.IsZero() {
		w.key(0, "overlay")
		w.field(1, "image", s.Overlay.Image, "overlay.image")
		w.field(1, "name", s.Overlay.Name, "overlay.name")
	}

	if s.Customization.IsZero() {
		w.emptyMapping(0, "customization")
	} else {
		w.key(0, "customization")
		c := s.Customization

		if len(c.ExtraKernelArgs) > 0 {
			w.key(1, "extraKernelArgs")
			for i, arg := range c.ExtraKernelArgs {
				w.item(2, arg, "customization.extraKernelArgs", i)
			}
		}
		if len(c.Meta) > 0 {
			w.key(1, "meta")
			for i, m := range c.Meta {
				// A sequence item that is a mapping puts its first key on the
				// dash line and aligns the rest two columns further in.
				w.raw(2, "- key: "+strconv.FormatUint(uint64(m.Key), 10))
				w.continuationField(2, "value", m.Value, "customization.meta.value", i)
			}
		}
		if !c.SystemExtensions.IsZero() {
			w.key(1, "systemExtensions")
			w.key(2, "officialExtensions")
			for i, name := range c.SystemExtensions.OfficialExtensions {
				w.item(3, name, "customization.systemExtensions.officialExtensions", i)
			}
		}
		if !c.SecureBoot.IsZero() {
			w.key(1, "secureboot")
			// Only reachable when the flag is true; IsZero folds false into
			// absent, which is what the Factory does.
			w.raw(2, "includeWellKnownCertificates: true")
		}
	}

	if w.err != nil {
		return nil, w.err
	}
	return w.buf.Bytes(), nil
}

// canonWriter accumulates the document and the first rendering failure. Errors
// are collected rather than returned per call so Canonical reads as a
// description of the format instead of a chain of error checks.
type canonWriter struct {
	buf bytes.Buffer
	err error
}

func (w *canonWriter) pad(level int) {
	w.buf.WriteString(strings.Repeat(" ", level*canonicalIndent))
}

// raw writes an already-rendered line.
func (w *canonWriter) raw(level int, line string) {
	w.pad(level)
	w.buf.WriteString(line)
	w.buf.WriteByte('\n')
}

// key writes a mapping key whose value is the nested block that follows.
func (w *canonWriter) key(level int, name string) {
	w.raw(level, name+":")
}

// emptyMapping writes a key whose value is the empty flow mapping.
func (w *canonWriter) emptyMapping(level int, name string) {
	w.raw(level, name+": {}")
}

// field writes a scalar-valued mapping entry.
func (w *canonWriter) field(level int, name, value, path string) {
	rendered, ok := w.render(value, path, -1)
	if !ok {
		return
	}
	w.raw(level, name+": "+rendered)
}

// continuationField writes a mapping entry that belongs to a sequence item
// whose dash line was already written, so it sits two columns past the dash.
func (w *canonWriter) continuationField(level int, name, value, path string, index int) {
	rendered, ok := w.render(value, path, index)
	if !ok {
		return
	}
	w.pad(level)
	w.buf.WriteString("  " + name + ": " + rendered + "\n")
}

// item writes a scalar sequence entry.
func (w *canonWriter) item(level int, value, path string, index int) {
	rendered, ok := w.render(value, path, index)
	if !ok {
		return
	}
	w.raw(level, "- "+rendered)
}

// render is where a refusal acquires its location. The path is supplied by the
// call site rather than carried as writer state, so Canonical stays readable as
// a description of the format and every path is visible in the place the format
// puts it.
func (w *canonWriter) render(value, path string, index int) (string, bool) {
	if reason := representable(value); reason != "" {
		w.fail(&NotRepresentableError{Path: path, Index: index, Reason: reason})
		return "", false
	}
	return renderScalar(value), true
}

func (w *canonWriter) fail(err error) {
	if w.err == nil {
		w.err = err
	}
}

// renderScalar chooses the style the upstream emitter would choose.
//
// The upstream rule, in the order it applies: a scalar that would parse back as
// something other than a string is double-quoted, so that reading the document
// returns the string that was written. Otherwise plain is preferred, and single
// quotes are used when plain would be ambiguous.
//
// It is only ever reached for a scalar representable has already cleared; the
// writer checks that first, because that is where the document path is known.
func renderScalar(s string) string {
	if resolvesToNonString(s) {
		return doubleQuote(s)
	}
	if plainAllowed(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// representable rejects the scalars whose rendering this package deliberately
// does not implement.
//
// A newline turns the scalar into a block literal with its own chomping and
// indentation rules; a control character forces a double-quoted form with
// escape sequences. Neither appears in a kernel argument, an extension name or
// a META value, and implementing them from memory rather than from a pinned
// observation is how a serialiser acquires a bug that only shows up as a
// mismatched id.
// The refused set is unchanged from the observation it was pinned against: not
// valid UTF-8, any rune below U+0020, or U+007F. Which scalars this renders
// decides the locally precomputed id, and the id is what FACT-06 rests on, so
// widening or narrowing it here is a change to what holzkube believes a
// schematic hashes to. Only the reporting changed.
//
// The returned reason is written for an operator and names the character class,
// never the value (T-02-64). An empty string means the scalar is representable.
func representable(s string) string {
	if !utf8.ValidString(s) {
		return "is not valid UTF-8"
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7F {
			return fmt.Sprintf("contains the control character %U", r)
		}
	}
	return ""
}

// doubleQuote renders the double-quoted form. Only reachable for scalars that
// representable already cleared, so the escape set is exactly the two
// characters the form itself reserves.
func doubleQuote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(s) + `"`
}

// yamlStyleFloat is the float syntax the upstream resolver accepts, transcribed
// from it rather than from the YAML specification -- the two differ, and it is
// the resolver that decides the quoting.
var yamlStyleFloat = regexp.MustCompile(`^[-+]?(\.[0-9]+|[0-9]+(\.[0-9]*)?)([eE][-+]?[0-9]+)?$`)

// base60Float is the sexagesimal float syntax, which is why "1:30" is a number
// rather than a string and therefore has to be quoted.
var base60Float = regexp.MustCompile(`^[-+]?[0-9][0-9_]*(?::[0-5]?[0-9])+(?:\.[0-9_]*)?$`)

// resolvedNonStrings are the plain scalars that resolve to a null, a boolean, a
// float or a merge key by exact spelling.
var resolvedNonStrings = map[string]struct{}{
	"": {}, "~": {}, "null": {}, "Null": {}, "NULL": {},
	"true": {}, "True": {}, "TRUE": {}, "false": {}, "False": {}, "FALSE": {},
	".nan": {}, ".NaN": {}, ".NAN": {},
	".inf": {}, ".Inf": {}, ".INF": {},
	"+.inf": {}, "+.Inf": {}, "+.INF": {},
	"-.inf": {}, "-.Inf": {}, "-.INF": {},
	"<<": {},
}

// oldBools are the YAML 1.1 booleans. The upstream resolver no longer reads
// them as booleans, but its emitter still quotes them, so that a document it
// writes cannot be misread by a parser that does.
var oldBools = map[string]struct{}{
	"y": {}, "Y": {}, "yes": {}, "Yes": {}, "YES": {},
	"n": {}, "N": {}, "no": {}, "No": {}, "NO": {},
	"on": {}, "On": {}, "ON": {}, "off": {}, "Off": {}, "OFF": {},
}

// resolvesToNonString reports whether the plain form of s would be read back as
// something other than a string, which is what forces the double-quoted style.
func resolvesToNonString(s string) bool {
	if _, ok := resolvedNonStrings[s]; ok {
		return true
	}
	if _, ok := oldBools[s]; ok {
		return true
	}
	if base60Float.MatchString(s) {
		return true
	}

	// Everything below is numeric, and the upstream resolver reaches it only
	// for a first byte that could begin a number.
	if s == "" {
		return false
	}
	switch c := s[0]; {
	case c == '+' || c == '-' || (c >= '0' && c <= '9'):
		if looksLikeTimestamp(s) {
			return true
		}
	case c == '.':
		// The float branch only; a leading dot cannot begin a timestamp or an
		// integer.
		_, err := strconv.ParseFloat(s, 64)
		return err == nil
	default:
		return false
	}

	plain := strings.ReplaceAll(s, "_", "")
	if _, err := strconv.ParseInt(plain, 0, 64); err == nil {
		return true
	}
	if _, err := strconv.ParseUint(plain, 0, 64); err == nil {
		return true
	}
	if yamlStyleFloat.MatchString(plain) {
		if _, err := strconv.ParseFloat(plain, 64); err == nil {
			return true
		}
	}
	for prefix, base := range map[string]int{"0b": 2, "-0b": 2, "0o": 8, "-0o": 8} {
		digits, ok := strings.CutPrefix(plain, prefix)
		if !ok {
			continue
		}
		if strings.HasPrefix(prefix, "-") {
			digits = "-" + digits
		}
		if _, err := strconv.ParseInt(digits, base, 64); err == nil {
			return true
		}
	}
	return false
}

// timestampFormats are the layouts the upstream resolver accepts, and only
// those: it does not implement the whole YAML timestamp type.
var timestampFormats = []string{
	"2006-1-2T15:4:5.999999999Z07:00",
	"2006-1-2t15:4:5.999999999Z07:00",
	"2006-1-2 15:4:5.999999999",
	"2006-1-2",
}

func looksLikeTimestamp(s string) bool {
	// Every accepted layout starts with exactly four digits and a dash.
	i := 0
	for ; i < len(s); i++ {
		if c := s[i]; c < '0' || c > '9' {
			break
		}
	}
	if i != 4 || i == len(s) || s[i] != '-' {
		return false
	}
	for _, format := range timestampFormats {
		if _, err := time.Parse(format, s); err == nil {
			return true
		}
	}
	return false
}

// plainAllowed reports whether s may be written unquoted in block context.
//
// Transcribed from the upstream emitter's scalar analysis. Only the block-context
// verdict is computed: this package emits no flow collections, so the flow rules
// -- which additionally forbid the comma and the brackets anywhere in a scalar --
// would never be consulted.
func plainAllowed(s string) bool {
	if s == "" {
		return false
	}
	// A leading or trailing space is unrecoverable in the plain form: the
	// parser strips it.
	if s[0] == ' ' || s[len(s)-1] == ' ' {
		return false
	}

	// A leading indicator is what makes the parser read a scalar as something
	// structural -- an anchor, an alias, a tag, a comment, a block scalar.
	switch s[0] {
	case '#', ',', '[', ']', '{', '}', '&', '*', '!', '|', '>', '\'', '"', '%', '@', '`':
		return false
	case '?', ':', '-':
		// Only structural when it stands alone as an indicator, i.e. when a
		// blank or the end of the scalar follows. "--console=x" is fine; "-" is
		// not.
		if len(s) == 1 || s[1] == ' ' {
			return false
		}
	}

	for i := range len(s) {
		switch s[i] {
		case ':':
			// A colon ends the scalar when a blank or the end follows it, which
			// is how "key: value" is read at all.
			if i+1 == len(s) || s[i+1] == ' ' {
				return false
			}
		case '#':
			// A hash starts a comment when a blank precedes it.
			if i > 0 && s[i-1] == ' ' {
				return false
			}
		}
	}
	return true
}
