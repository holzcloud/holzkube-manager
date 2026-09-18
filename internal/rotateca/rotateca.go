// Package rotateca plans the rotation of a cluster's Talos certificate
// authority, and computes nothing else.
//
// V2-OPS-02's second half, and ledger 103 records why it was left out for as
// long as it was: rotating the authority changes what every node TRUSTS. An
// abort in the middle leaves a cluster that trusts two authorities, which is
// harmless, and the wrong order leaves one that trusts none, which is a
// cluster nobody can reach again. The first is a state to finish, the second
// is a reinstall.
//
// This package reaches nothing. It reads a node's machine configuration, says
// what the next pass should write, and answers whether a pass already
// happened -- the same split internal/scale uses, and for the same reason: an
// interface or a job with its own reading of the rule looks right until
// somebody clicks.
//
// # The order, and why it is four passes and not one
//
//  1. every node accepts the new authority   (machine.acceptedCAs += new)
//  2. every node issues from the new one     (machine.ca = new)
//  3. this installation's own certificate is minted from the new authority
//     and proven against a node before it is stored
//  4. every node stops accepting the old one (machine.acceptedCAs -= old)
//
// Pass 1 before pass 2 is the whole safety property: a node that has not yet
// accepted the new authority but has been switched to issue from it presents a
// certificate its peers -- and this installation -- refuse. Pass 4 last,
// because until this installation holds a certificate from the new authority
// it is the old one that lets it in at all.
//
// Between passes the cluster is in a state that WORKS: two accepted
// authorities is a normal Talos configuration, and a run that stops after pass
// 1 or 2 has left a cluster an operator can still reach and a job they can
// resume. That is the property that makes this rotation something other than
// a coin flip.
//
// # What this does NOT rotate
//
// The Kubernetes certificate authority. Talos keeps the two apart
// (cluster.ca against machine.ca), `talosctl rotate-ca` takes --kubernetes for
// the other one, and this product speaks the Talos machine API and does not
// talk to Kubernetes at all (ledger 86). Rotating the Kubernetes authority
// through machine configuration alone would leave every kubelet holding a
// certificate from an authority the API server no longer has -- so it is not
// offered rather than half offered.
package rotateca

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	cryptox509 "github.com/siderolabs/crypto/x509"
	"gopkg.in/yaml.v3"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

// ErrNotEveryNodeAnswered refuses a rotation that cannot see the whole cluster.
//
// It is the refusal this package exists for. A node that misses pass 1 is a
// node that will refuse the certificate every other node accepts after pass 2,
// and nothing in the rotation can repair it afterwards: reaching it to fix its
// configuration needs the very credential it no longer accepts. A node that is
// merely switched off during a rotation therefore has to stop the rotation,
// not be skipped -- which is the opposite of the rolling upgrade's decision
// about a locked node, and for the opposite reason.
var ErrNotEveryNodeAnswered = errors.New("rotateca: not every node in this cluster answered")

// ErrNoMachines refuses a rotation of a cluster whose inventory is empty.
var ErrNoMachines = errors.New("rotateca: this cluster has no machines in the inventory")

// ErrConfigUnreadable reports a machine configuration this package cannot read.
var ErrConfigUnreadable = errors.New("rotateca: the node's machine configuration could not be read")

// Authority is a Talos certificate authority: the certificate every node
// trusts, and the key that issues from it.
type Authority struct {
	Crt []byte
	Key []byte
}

// NewAuthority generates one.
//
// ECDSA and ten years, which is what Talos generates for itself, because a
// rotation that quietly shortened the authority's life would hand the operator
// a second rotation on a date nobody chose.
func NewAuthority(now time.Time) (Authority, error) {
	ca, err := cryptox509.NewSelfSignedCertificateAuthority(
		cryptox509.ECDSA(true),
		cryptox509.Organization("talos"),
		cryptox509.NotBefore(now.Add(-time.Hour)),
		cryptox509.NotAfter(now.AddDate(10, 0, 0)),
	)
	if err != nil {
		return Authority{}, fmt.Errorf("rotateca: generating an authority: %w", err)
	}
	return Authority{Crt: ca.CrtPEM, Key: ca.KeyPEM}, nil
}

// State is what one node's machine configuration says about authorities.
type State struct {
	// IssuingCrt is machine.ca: the authority this node issues certificates
	// from and presents to callers.
	IssuingCrt []byte

	// AcceptedCrts is machine.acceptedCAs: every authority this node will
	// accept a caller's certificate from. Talos treats machine.ca as accepted
	// implicitly, so this list is the ADDITIONAL ones.
	AcceptedCrts [][]byte
}

// Inspect reads the authorities out of a machine configuration.
//
// It reads YAML rather than loading the configuration through machinery, and
// that is deliberate: the multi-document configurations Talos 1.8 and later
// write carry documents this package has no business decoding, and a loader
// that refuses an unknown document kind would make this step fail on a
// configuration the node itself is running happily.
func Inspect(configYAML []byte) (State, error) {
	var doc struct {
		Machine struct {
			CA struct {
				Crt string `yaml:"crt"`
			} `yaml:"ca"`
			AcceptedCAs []struct {
				Crt string `yaml:"crt"`
			} `yaml:"acceptedCAs"`
		} `yaml:"machine"`
	}
	if err := yaml.Unmarshal(configYAML, &doc); err != nil {
		return State{}, fmt.Errorf("%w: %w", ErrConfigUnreadable, err)
	}

	st := State{IssuingCrt: decodeBase64PEM(doc.Machine.CA.Crt)}
	if len(st.IssuingCrt) == 0 {
		return State{}, fmt.Errorf("%w: it names no machine.ca, so this is not a node "+
			"configuration this rotation can reason about", ErrConfigUnreadable)
	}
	for _, a := range doc.Machine.AcceptedCAs {
		if crt := decodeBase64PEM(a.Crt); len(crt) > 0 {
			st.AcceptedCrts = append(st.AcceptedCrts, crt)
		}
	}
	return st, nil
}

// Accepts reports whether this node already accepts crt, as the issuing
// authority or as one of the accepted ones.
func (s State) Accepts(crt []byte) bool {
	if sameCertificate(s.IssuingCrt, crt) {
		return true
	}
	for _, have := range s.AcceptedCrts {
		if sameCertificate(have, crt) {
			return true
		}
	}
	return false
}

// IssuesFrom reports whether this node issues from crt.
func (s State) IssuesFrom(crt []byte) bool { return sameCertificate(s.IssuingCrt, crt) }

// AcceptPatch is pass 1: the node accepts the new authority, and the one it
// issues from today, in addition to whatever it accepts already.
//
// Two things about this, and both were MEASURED rather than read.
//
// A strategic merge patch APPENDS to a list; it does not replace it. Measured
// against machineconfig.ApplyPatches: patching a two-entry certSANs list with
// one of its own entries produced three entries, the repeated one twice. So a
// patch here lists only the authorities that are missing -- listing the whole
// desired set would duplicate the ones already there -- and pass 4, which has
// to REMOVE one, cannot be a patch at all (see PruneConfig).
//
// And it adds the CURRENT ISSUING AUTHORITY, which looks redundant and is the
// load-bearing part. Talos accepts machine.ca implicitly, so an authority that
// is only the issuer is accepted without being listed -- and pass 2 replaces
// machine.ca, which takes that implicit acceptance away in the same write.
// Without this the old authority becomes unaccepted the instant pass 2 lands,
// which is exactly the cut this rotation exists to avoid. A test found it; the
// first version of this function did not do it.
//
// The second return value is false when nothing would change, so a resumed run
// does not write a configuration it already wrote.
func AcceptPatch(s State, newCrt []byte) (string, bool) {
	var missing [][]byte
	for _, want := range dedupeCertificates([][]byte{s.IssuingCrt, newCrt}) {
		if !listed(s.AcceptedCrts, want) {
			missing = append(missing, want)
		}
	}
	if len(missing) == 0 {
		return "", false
	}
	return acceptedCAsPatch(missing), true
}

// listed reports whether crt is one of the EXPLICITLY accepted authorities.
//
// Deliberately not State.Accepts: that one answers the question a caller asks
// about trust, and counts machine.ca. This one answers whether the list has to
// be written, and the list is where pass 2 takes the implicit acceptance away.
func listed(have [][]byte, crt []byte) bool {
	for _, c := range have {
		if sameCertificate(c, crt) {
			return true
		}
	}
	return false
}

// dedupeCertificates keeps the first occurrence of each certificate, in order.
func dedupeCertificates(in [][]byte) [][]byte {
	out := make([][]byte, 0, len(in))
	for _, c := range in {
		if len(c) == 0 {
			continue
		}
		seen := false
		for _, have := range out {
			if sameCertificate(have, c) {
				seen = true
				break
			}
		}
		if !seen {
			out = append(out, c)
		}
	}
	return out
}

// sameCertificateSet compares two lists as sets, because "already done" is a
// question about which authorities are listed and not about their order.
func sameCertificateSet(a, b [][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for _, want := range a {
		found := false
		for _, have := range b {
			if sameCertificate(want, have) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// IssuePatch is pass 2: the node issues from the new authority.
//
// The KEY goes only to a control-plane node, and that is Talos's own split
// rather than caution: trustd runs on the control plane and issues
// certificates to joining nodes, so it needs the authority's key, and a worker
// carries the certificate alone. Handing a worker the key would put the
// cluster's whole Talos PKI on a machine that has no reason to hold it, and
// this rotation would be the thing that put it there.
func IssuePatch(a Authority, role model.MachineRole) string {
	if role == model.RoleControlPlane {
		return "machine:\n" +
			"  ca:\n" +
			"    crt: " + encodeBase64(a.Crt) + "\n" +
			"    key: " + encodeBase64(a.Key) + "\n"
	}
	// An empty key is how Talos itself writes a worker's configuration. It is
	// the field being present and empty rather than absent, because a strategic
	// merge that omits it keeps whatever the node has -- which on a node that
	// was once a control plane is the old authority's key.
	return "machine:\n" +
		"  ca:\n" +
		"    crt: " + encodeBase64(a.Crt) + "\n" +
		"    key: \"\"\n"
}

// PruneConfig is pass 4: the node stops accepting everything but the new
// authority.
//
// It returns a WHOLE machine configuration rather than a patch, and that is
// forced rather than chosen: a strategic merge patch appends to a list, so
// nothing expressed as a patch can shorten acceptedCAs. Measured, not assumed
// -- see AcceptPatch. The alternative would be an RFC 6902 patch, which
// machineconfig refuses on principle because it addresses list entries by
// index, and the whole point here is that the list is a different length on
// different nodes.
//
// So this edits the document that came off the node: it replaces the one
// acceptedCAs node in the v1alpha1 document and leaves every other byte to
// yaml's own round trip. Only the first document is touched, because that is
// where machine lives; the rest are carried through unread.
//
// Refused outright while the node still issues from the old authority. That
// ordering is the difference between a rotation and an outage, and this is the
// second place it is enforced -- the first being the order of the passes --
// because a resumed or hand-driven run is exactly where an order gets skipped.
func PruneConfig(configYAML []byte, newCrt []byte) ([]byte, bool, error) {
	state, err := Inspect(configYAML)
	if err != nil {
		return nil, false, err
	}
	if !state.IssuesFrom(newCrt) {
		return nil, false, fmt.Errorf("rotateca: this node still issues from the old authority, "+
			"so dropping it from the accepted set would cut this installation off from the node "+
			"it is talking to: pass 2 has not happened here yet. %w", ErrOutOfOrder)
	}
	if len(state.AcceptedCrts) == 0 {
		return nil, false, nil
	}
	if len(state.AcceptedCrts) == 1 && sameCertificate(state.AcceptedCrts[0], newCrt) {
		return nil, false, nil
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(configYAML, &doc); err != nil {
		return nil, false, fmt.Errorf("%w: %w", ErrConfigUnreadable, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, false, fmt.Errorf("%w: the first document is not a mapping", ErrConfigUnreadable)
	}

	machine := mappingValue(doc.Content[0], "machine")
	if machine == nil {
		return nil, false, fmt.Errorf("%w: it has no machine section", ErrConfigUnreadable)
	}
	kept := &yaml.Node{Kind: yaml.SequenceNode}
	kept.Content = append(kept.Content, &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "crt"},
			{Kind: yaml.ScalarNode, Value: encodeBase64(newCrt)},
		},
	})
	setMappingValue(machine, "acceptedCAs", kept)

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return nil, false, fmt.Errorf("rotateca: writing the pruned configuration: %w", err)
	}

	// The tail documents, byte for byte. Re-encoding documents this package
	// does not read would be an edit nobody asked for, on a configuration the
	// node is running.
	if i := documentBreak(configYAML); i >= 0 {
		out = append(out, configYAML[i:]...)
	}
	return out, true, nil
}

// mappingValue returns the value node for key in a mapping, or nil.
func mappingValue(m *yaml.Node, key string) *yaml.Node {
	if m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// setMappingValue replaces key's value, or appends the pair when it is absent.
func setMappingValue(m *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = value
			return
		}
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, value)
}

// documentBreak is the index of the first document separator, or -1.
func documentBreak(raw []byte) int {
	for i := 0; i+3 < len(raw); i++ {
		if (i == 0 || raw[i-1] == 0x0a) && string(raw[i:i+3]) == "---" {
			return i
		}
	}
	return -1
}

// ErrOutOfOrder reports a pass asked for before the pass it depends on.
var ErrOutOfOrder = errors.New("rotateca: the passes would run out of order")

// RefuseUnlessEveryNodeAnswered is the precondition, checked before anything
// is written anywhere.
//
// reached carries one entry per machine in the cluster's inventory. The
// refusal names the nodes that did not answer, because "some node" sends an
// operator to look at all of them.
func RefuseUnlessEveryNodeAnswered(reached map[model.MachineID]error) error {
	if len(reached) == 0 {
		return ErrNoMachines
	}

	var silent []string
	for id, err := range reached {
		if err != nil {
			silent = append(silent, string(id))
		}
	}
	if len(silent) == 0 {
		return nil
	}

	return fmt.Errorf("%w: %v. A node that misses the first pass refuses the certificate every "+
		"other node accepts after the second, and nothing here can repair it afterwards -- "+
		"reaching it would need the credential it no longer accepts. Bring it back or remove it "+
		"from the cluster first",
		ErrNotEveryNodeAnswered, silent)
}

func acceptedCAsPatch(crts [][]byte) string {
	if len(crts) == 0 {
		// An empty list rather than an absent key: absent keeps what the node
		// has, which is the opposite of what pass 4 means.
		return "machine:\n  acceptedCAs: []\n"
	}
	out := "machine:\n  acceptedCAs:\n"
	for _, c := range crts {
		out += "    - crt: " + encodeBase64(c) + "\n"
	}
	return out
}

// sameCertificate compares two PEM certificates by what they are rather than
// by their bytes.
//
// Two encodings of one certificate differ in whitespace and in line endings,
// and a rotation that compared bytes would apply pass 1 for ever on a node
// whose configuration came back formatted differently from the one that was
// sent.
func sameCertificate(a, b []byte) bool {
	pa, pb := parseCertificate(a), parseCertificate(b)
	if pa == nil || pb == nil {
		return false
	}
	return pa.Equal(pb)
}

func parseCertificate(pemBytes []byte) *x509.Certificate {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil
	}
	crt, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil
	}
	return crt
}

func encodeBase64(pemBytes []byte) string {
	return base64.StdEncoding.EncodeToString(pemBytes)
}

func decodeBase64PEM(s string) []byte {
	if s == "" {
		return nil
	}
	// Talos writes these fields base64-encoded. A value that is already PEM
	// happens when a configuration was written by hand, and it is accepted
	// rather than refused: what matters is the certificate, not who encoded it.
	if raw, err := base64.StdEncoding.DecodeString(s); err == nil && parseCertificate(raw) != nil {
		return raw
	}
	if parseCertificate([]byte(s)) != nil {
		return []byte(s)
	}
	return nil
}
