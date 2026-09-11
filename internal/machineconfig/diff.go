package machineconfig

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// A structural diff, not a text diff.
//
// A text diff of two YAML documents answers a question nobody asked: it
// reports reordering, reindentation and comment changes as differences, and it
// reports a list that grew from one element to two as "one line added" -- which
// is exactly the change an operator most needs to see, because a strategic
// merge patch applied twice is how a list of certSANs quietly acquires a
// duplicate.
//
// So the comparison is over the parsed structures, and two things it finds are
// called out by name rather than left in the noise:
//
//   - **list growth**: a list whose length changed, with both lengths;
//   - **duplicates**: a list that contains the same value twice after the
//     merge, which is nearly always a patch that was applied a second time.

// ChangeKind is what happened at one path.
type ChangeKind string

const (
	// ChangeAdded is a path that did not exist before.
	ChangeAdded ChangeKind = "added"

	// ChangeRemoved is a path that no longer exists.
	ChangeRemoved ChangeKind = "removed"

	// ChangeModified is a path whose value differs.
	ChangeModified ChangeKind = "modified"

	// ChangeListGrew is a list whose length changed. It is its own kind
	// because "modified" would hide the one thing worth seeing.
	ChangeListGrew ChangeKind = "list-changed"
)

// Change is one difference.
type Change struct {
	Path string     `json:"path"`
	Kind ChangeKind `json:"kind"`

	// Before and After are rendered values, already redacted. They are strings
	// rather than `any` because they go straight onto a screen and because a
	// typed value would invite a caller to compare them again, differently.
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`

	// LenBefore and LenAfter are set for a list change, so the screen can say
	// "3 entries became 4" rather than printing two lists side by side.
	LenBefore int `json:"len_before,omitempty"`
	LenAfter  int `json:"len_after,omitempty"`

	// Duplicates are values that appear more than once in the list *after* the
	// change. Nearly always a patch applied twice.
	Duplicates []string `json:"duplicates,omitempty"`
}

// Diff is the whole comparison.
type Diff struct {
	Changes []Change `json:"changes"`

	// Paths is every changed path, which is what ModeFor consumes. It is
	// derived here rather than by the caller so that the diff the operator
	// reads and the mode holzkube-manager computes are about the same set.
	Paths []string `json:"paths"`

	// Duplicates is true when any list gained a repeated value. It is hoisted
	// out of the changes because it is the one finding that usually means the
	// operator is about to do something they did not intend.
	Duplicates bool `json:"duplicates"`
}

// Compare diffs two machine configurations.
//
// Both sides are redacted first, so nothing downstream of this -- the API
// response, the screen, the audit record of what was about to be applied --
// can carry a secret. That is the third of the five exits Redact guards.
func Compare(before, after []byte) (Diff, error) {
	redactedBefore, err := Redact(before)
	if err != nil {
		return Diff{}, err
	}
	redactedAfter, err := Redact(after)
	if err != nil {
		return Diff{}, err
	}

	var a, b any
	if err := yaml.Unmarshal(redactedBefore, &a); err != nil {
		return Diff{}, fmt.Errorf("machineconfig: parse the current configuration: %w", err)
	}
	if err := yaml.Unmarshal(redactedAfter, &b); err != nil {
		return Diff{}, fmt.Errorf("machineconfig: parse the proposed configuration: %w", err)
	}

	var d Diff
	walk("", a, b, &d)

	sort.Slice(d.Changes, func(i, j int) bool { return d.Changes[i].Path < d.Changes[j].Path })
	for _, c := range d.Changes {
		d.Paths = append(d.Paths, c.Path)
		if len(c.Duplicates) > 0 {
			d.Duplicates = true
		}
	}
	return d, nil
}

// walk compares two nodes and appends what it finds.
func walk(path string, before, after any, d *Diff) {
	switch {
	case before == nil && after == nil:
		return
	case before == nil:
		d.Changes = append(d.Changes, Change{Path: pathOrRoot(path), Kind: ChangeAdded, After: render(after)})
		return
	case after == nil:
		d.Changes = append(d.Changes, Change{Path: pathOrRoot(path), Kind: ChangeRemoved, Before: render(before)})
		return
	}

	bMap, bIsMap := asMap(before)
	aMap, aIsMap := asMap(after)
	if bIsMap && aIsMap {
		for _, key := range unionKeys(bMap, aMap) {
			walk(path+"."+key, bMap[key], aMap[key], d)
		}
		return
	}

	bList, bIsList := before.([]any)
	aList, aIsList := after.([]any)
	if bIsList && aIsList {
		diffList(path, bList, aList, d)
		return
	}

	if render(before) != render(after) {
		d.Changes = append(d.Changes, Change{
			Path:   pathOrRoot(path),
			Kind:   ChangeModified,
			Before: render(before),
			After:  render(after),
		})
	}
}

// diffList reports a list change as one entry rather than as an element-wise
// walk.
//
// Element-wise would report "element 2 changed" when an element was inserted
// at the front, which is the single most misleading thing a config diff can
// say. The length and the duplicates are the two facts that actually decide
// whether the operator meant this.
func diffList(path string, before, after []any, d *Diff) {
	same := len(before) == len(after)
	if same {
		for i := range before {
			if render(before[i]) != render(after[i]) {
				same = false
				break
			}
		}
	}
	dupes := duplicatesIn(after)
	if same && len(dupes) == 0 {
		return
	}

	kind := ChangeModified
	if len(before) != len(after) {
		kind = ChangeListGrew
	}

	d.Changes = append(d.Changes, Change{
		Path:       pathOrRoot(path),
		Kind:       kind,
		Before:     renderList(before),
		After:      renderList(after),
		LenBefore:  len(before),
		LenAfter:   len(after),
		Duplicates: dupes,
	})
}

// duplicatesIn reports values that appear more than once, in order of first
// appearance.
func duplicatesIn(list []any) []string {
	seen := map[string]int{}
	var out []string
	for _, v := range list {
		r := render(v)
		seen[r]++
		if seen[r] == 2 {
			out = append(out, r)
		}
	}
	return out
}

func asMap(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case map[string]any:
		return m, true
	case map[any]any:
		// yaml.v3 produces map[string]any for string keys, but a document
		// round-tripped through another library can still arrive this way.
		out := make(map[string]any, len(m))
		for k, val := range m {
			out[fmt.Sprint(k)] = val
		}
		return out, true
	default:
		return nil, false
	}
}

func unionKeys(a, b map[string]any) []string {
	seen := map[string]bool{}
	for k := range a {
		seen[k] = true
	}
	for k := range b {
		seen[k] = true
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// render turns a value into the one string both sides are compared as.
//
// Comparing rendered forms rather than Go values means `3` and `"3"` compare
// equal, which is correct for YAML: the two are the same configuration, and
// reporting them as a change would be reporting the parser's opinion.
func render(v any) string {
	if v == nil {
		return ""
	}
	raw, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return strings.TrimRight(string(raw), "\n")
}

func renderList(list []any) string {
	parts := make([]string, 0, len(list))
	for _, v := range list {
		parts = append(parts, strings.ReplaceAll(render(v), "\n", " "))
	}
	return strings.Join(parts, ", ")
}

func pathOrRoot(path string) string {
	if path == "" {
		return "."
	}
	return path
}
