package machineconfig

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// A snapshot of a node's configuration, for drift detection.
//
// What is kept of a configuration that is read from a node on a schedule is
// neither a hash nor the configuration itself, and that is deliberate (D-02).
// A hash says "something changed" and nothing else; the operator then has no
// way to tell a deliberate change from a stranger's. The configuration itself
// carries the private keys of the cluster, and a copy of those in the data
// directory, refreshed every few hours, would be the one place they stop being
// guarded by the type system that keeps them inside internal/talos. So the
// thing kept is the output of Redact -- the same text the configuration view
// already shows -- put into a normal form, so that two readings that mean the
// same thing are the same bytes.
//
// One consequence is stated here rather than discovered later: Redact replaces
// every secret by the same constant, so a change of nothing but a secret
// value, a rotated token, is invisible in a snapshot. That is wanted (D-02),
// and a test holds it so nobody takes it for a fault of the comparison.

// MaxSnapshotBytes caps both the raw configuration read from a node and the
// snapshot made of it. It guards against a degenerate node; the configuration
// of a generated control plane measures 6.7 KB, so the cap is two orders of
// magnitude of room.
const MaxSnapshotBytes = 1 << 20

var (
	// ErrSnapshotUnreadable reports a configuration this build could not load.
	// Its text is fixed: the messages of the loader describe the structure of
	// the document and must not be able to carry a piece of it into a log, an
	// API response or a stored record.
	ErrSnapshotUnreadable = errors.New("machineconfig: this version of holzkube-manager could not load the " +
		"node's configuration; a node running a newer Talos than this build knows is the usual reason")

	// ErrSnapshotTooLarge reports a configuration over MaxSnapshotBytes.
	ErrSnapshotTooLarge = errors.New("machineconfig: the node's configuration is larger than a snapshot may be")

	// ErrSnapshotUnsafe reports a PEM private key that survived redaction. It
	// is the belt to Redact's braces and is not reached while Redact holds.
	ErrSnapshotUnsafe = errors.New("machineconfig: a private key survived redaction, so no snapshot was made")
)

// Snap is a snapshot: the normalised, redacted text and its digest.
type Snap struct {
	// Text is the redacted configuration, one document per YAML document,
	// sorted by document key, without comments.
	Text string

	// Digest is the hex sha256 of Text.
	Digest string
}

// Snapshot makes the snapshot of a raw configuration.
//
// It is built from Redact and from nothing else. A configuration that does not
// load gives no snapshot and no text at all: Redact returns bytes alongside
// that error, and those bytes have had only the PEM sweep, which is not a
// snapshot of anything.
func Snapshot(raw []byte) (Snap, error) {
	if len(raw) > MaxSnapshotBytes {
		return Snap{}, ErrSnapshotTooLarge
	}

	redacted, err := Redact(raw)
	if err != nil {
		// Every byte of redacted is dropped, and so is the text of err.
		return Snap{}, ErrSnapshotUnreadable
	}
	if ContainsPrivateKey(redacted) {
		return Snap{}, ErrSnapshotUnsafe
	}

	docs, err := splitDocuments(redacted)
	if err != nil {
		return Snap{}, ErrSnapshotUnreadable
	}

	sort.SliceStable(docs, func(i, j int) bool { return docs[i].key < docs[j].key })

	parts := make([]string, 0, len(docs))
	for _, d := range docs {
		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		if err := enc.Encode(d.node); err != nil {
			return Snap{}, ErrSnapshotUnreadable
		}
		if err := enc.Close(); err != nil {
			return Snap{}, ErrSnapshotUnreadable
		}
		parts = append(parts, buf.String())
	}
	text := strings.Join(parts, "---\n")

	if len(text) > MaxSnapshotBytes {
		return Snap{}, ErrSnapshotTooLarge
	}

	sum := sha256.Sum256([]byte(text))
	return Snap{Text: text, Digest: hex.EncodeToString(sum[:])}, nil
}

// document is one YAML document of a configuration with the key it is
// compared under.
type document struct {
	key  string
	node *yaml.Node
}

// splitDocuments cuts a multi-document text into documents and names each.
//
// The documents stay yaml.Nodes, not decoded values, so that the order of the
// keys inside a document is kept: a snapshot must be a text a person can read
// next to the configuration view, and a map would sort it alphabetically.
func splitDocuments(text []byte) ([]document, error) {
	dec := yaml.NewDecoder(bytes.NewReader(text))

	var docs []document
	seen := map[string]int{}
	for {
		var node yaml.Node
		err := dec.Decode(&node)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if node.Kind != yaml.DocumentNode || len(node.Content) == 0 {
			continue
		}
		stripComments(&node)

		key := documentKey(node.Content[0], len(docs))
		seen[key]++
		if n := seen[key]; n > 1 {
			// Two documents with one key must not collapse into one.
			key += "#" + strconv.Itoa(n)
		}
		docs = append(docs, document{key: key, node: &node})
	}
	return docs, nil
}

// documentKey names a document: `kind`, or `kind/name` for a named one; the
// v1alpha1 document has no kind and a `version`; anything else is doc#<index>.
//
// The name matters: a control plane carries two KubeAuthorizerConfig documents
// (node and rbac), and under one key they could not be told apart.
func documentKey(root *yaml.Node, index int) string {
	if root.Kind == yaml.MappingNode {
		if kind := scalarField(root, "kind"); kind != "" {
			if name := scalarField(root, "name"); name != "" {
				return kind + "/" + name
			}
			return kind
		}
		if scalarField(root, "version") == "v1alpha1" {
			return "v1alpha1"
		}
	}
	return "doc#" + strconv.Itoa(index)
}

func scalarField(m *yaml.Node, name string) string {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == name && m.Content[i+1].Kind == yaml.ScalarNode {
			return m.Content[i+1].Value
		}
	}
	return ""
}

func stripComments(n *yaml.Node) {
	n.HeadComment, n.LineComment, n.FootComment = "", "", ""
	for _, c := range n.Content {
		stripComments(c)
	}
}

// CompareDocuments compares two snapshots over every document.
//
// Compare reads only the first YAML document, and a generated control plane
// has up to 28: through Compare, a change to KubeletConfig.image is no change
// at all, which for drift detection would be the silent "no drift" the feature
// exists to rule out. So this compares per document key and reuses walk for
// the contents; paths carry the key as their prefix (KubeletConfig.image,
// v1alpha1.machine.certSANs), and a document that appeared or went is one
// added or removed change under its key.
//
// Both sides go through Snapshot again, the way Compare sends both sides
// through Redact: a stored text that was altered by hand cannot carry a secret
// into a comparison.
func CompareDocuments(before, after string) (Diff, error) {
	a, err := Snapshot([]byte(before))
	if err != nil {
		return Diff{}, err
	}
	b, err := Snapshot([]byte(after))
	if err != nil {
		return Diff{}, err
	}

	left, err := decodeByKey(a.Text)
	if err != nil {
		return Diff{}, err
	}
	right, err := decodeByKey(b.Text)
	if err != nil {
		return Diff{}, err
	}

	keys := map[string]bool{}
	for k := range left {
		keys[k] = true
	}
	for k := range right {
		keys[k] = true
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)

	// Never nil: a comparison without a change is `[]` in JSON, not `null`,
	// and a screen handed null shows an error instead of a page.
	d := Diff{Changes: []Change{}, Paths: []string{}}
	for _, k := range sorted {
		walk(k, left[k], right[k], &d)
	}

	sort.Slice(d.Changes, func(i, j int) bool { return d.Changes[i].Path < d.Changes[j].Path })
	for _, c := range d.Changes {
		d.Paths = append(d.Paths, c.Path)
		if len(c.Duplicates) > 0 {
			d.Duplicates = true
		}
	}
	return d, nil
}

// decodeByKey reads a snapshot text into its documents' contents by key.
func decodeByKey(text string) (map[string]any, error) {
	docs, err := splitDocuments([]byte(text))
	if err != nil {
		return nil, ErrSnapshotUnreadable
	}
	out := make(map[string]any, len(docs))
	for _, d := range docs {
		var v any
		if err := d.node.Decode(&v); err != nil {
			return nil, ErrSnapshotUnreadable
		}
		out[d.key] = v
	}
	return out, nil
}

// ReadSnapshot reads a node's active configuration and returns its snapshot.
//
// The raw bytes stay in this function: they are not returned, not logged and
// not written into an error.
//
// The class deadline of the read is set here, not by the caller. The caller is
// a background loop on context.Background(), and a reader that waits for
// somebody else to hand it a deadline is the fault of ledger 137: the Talos
// client refuses every call without one.
func (s *Service) ReadSnapshot(ctx context.Context, id model.MachineID) (Snap, error) {
	cc, err := s.deps.Connect(ctx, id)
	if err != nil {
		return Snap{}, err
	}
	defer cc.Close() //nolint:errcheck // the read's verdict is its own

	readCtx, cancel, err := talos.WithClassDeadline(ctx, talos.MethodCOSIGet)
	if err != nil {
		return Snap{}, err
	}
	defer cancel()

	raw, err := cc.MachineConfigYAML(readCtx)
	if err != nil {
		return Snap{}, err
	}
	return Snapshot(raw)
}
