package talos_test

import (
	"context"
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// This file is the only place in the package that imports internal/httpapi, and
// it is a test. Production code in internal/talos must not: the transport sits
// below HTTP, and an import in the other direction is the shape that later
// becomes a cycle. What the import buys is the one thing a duplicated string
// literal cannot -- a compile error the day somebody renames a reserved code
// token, and a red test the day somebody mints a third one.

// TestErrorKindMapping pins the correspondence between a classified transport
// failure and the reserved upstream.node-* code tokens plan 02-02 minted.
//
// The tokens get no production consumer in this phase: the HTTP adapter that
// turns a *talos.Error into an RFC 9457 problem arrives in Phase 6 with the
// first route that drives a node call. This test is what keeps them honest in
// the meantime -- a reserved token nobody produces is a promise the contract
// made and the code never kept.
func TestErrorKindMapping(t *testing.T) {
	t.Parallel()

	want := map[talos.ErrorKind]string{
		talos.KindUnreachable: httpapi.CodeUpstreamNodeUnreachable,
		talos.KindTimeout:     httpapi.CodeUpstreamNodeTimeout,

		// Deliberately absent from the family. A node that answered is not an
		// upstream failure: it heard the request and refused it, and mapping
		// that onto a 502 "did not answer" would tell the operator to go and
		// check a network that is fine. Phase 6 decides what a refusal becomes;
		// the empty string is this package saying it is not one of these two.
		talos.KindRejected: "",
	}

	seen := map[string]talos.ErrorKind{}

	for _, kind := range talos.ErrorKinds() {
		code, ok := kind.ProblemCode()

		expected, listed := want[kind]
		if !listed {
			t.Fatalf("ErrorKind %v (%d) has no row in this test: a kind added without a decision about "+
				"which reserved token it becomes is a kind that will reach an operator as an anonymous "+
				"internal error", kind, int(kind))
		}

		if code != expected {
			t.Errorf("%v.ProblemCode() = %q, want %q", kind, code, expected)
		}
		if ok != (expected != "") {
			t.Errorf("%v.ProblemCode() reported ok=%v for code %q", kind, ok, code)
		}
		if !ok {
			continue
		}

		if prior, dup := seen[code]; dup {
			t.Errorf("%v and %v both map to %q; two kinds behind one token are two kinds an operator "+
				"cannot tell apart", prior, kind, code)
		}
		seen[code] = kind
	}

	// Totality in the direction that actually rots: every reserved node token
	// declared in the contract has a kind that produces it. Read out of the
	// source rather than from a list here, so a third token minted in
	// problem.go without a matching kind fails this test instead of sitting
	// unreferenced.
	declared := declaredNodeCodes(t)
	if len(declared) != len(seen) {
		t.Errorf("internal/httpapi declares %d upstream.node-* token(s) %v, but ErrorKind produces %d %v",
			len(declared), declared, len(seen), keysOf(seen))
	}
	for _, code := range declared {
		if _, ok := seen[code]; !ok {
			t.Errorf("the contract reserves %q and no ErrorKind produces it", code)
		}
	}
}

// nodeCodeLiteral matches the reserved node tokens as they are spelled in the
// contract's source.
var nodeCodeLiteral = regexp.MustCompile(`"(upstream\.node-[a-z-]+)"`)

// declaredNodeCodes reads the reserved upstream.node-* tokens out of
// internal/httpapi's source.
//
// Go has no way to enumerate the constants of a package at run time, and a
// hand-written list here would be updated by the same edit that forgot to add
// the kind. Reading the source is what makes the totality check mean something.
func declaredNodeCodes(t *testing.T) []string {
	t.Helper()

	const path = "../httpapi/problem.go"

	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var out []string
	for _, m := range nodeCodeLiteral.FindAllStringSubmatch(string(src), -1) {
		if !contains(out, m[1]) {
			out = append(out, m[1])
		}
	}
	if len(out) == 0 {
		t.Fatalf("found no upstream.node-* literal in %s; the detection is broken and this guard is vacuous", path)
	}
	return out
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func keysOf(m map[string]talos.ErrorKind) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestErrorNamesTheMachineAndNeverTheAddress is T-02-32.
//
// A *talos.Error travels outward: Phase 6 turns it into an HTTP problem
// document that reaches a browser. gRPC's own diagnostic for an unreachable
// node embeds the address it tried to dial, which is the same class of string
// audit.ChainStatus.Public() strips before it leaves the process. So the
// rendered message names the operation and the machine id and stops there.
func TestErrorNamesTheMachineAndNeverTheAddress(t *testing.T) {
	t.Parallel()

	// A host that cannot resolve, spelled so that its presence in any message
	// is unambiguous rather than a coincidence.
	const sentinel = "sentinel-node-address-must-not-be-logged.invalid"

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()

	target := talos.Target{
		Machine: model.MachineID("00000000-0000-0000-0000-0000000000aa"),
		Addr:    sentinel,
	}

	sim := newSim(t, talossim.Options{Hostname: "unused"})

	_, err := talos.NewClusterClient(ctx, talos.NewDirectDialer(1), target, sim.ClientCreds(), talos.Mode{})
	if err == nil {
		t.Fatal("NewClusterClient reached a node at an address that does not resolve")
	}

	var te *talos.Error
	if !errors.As(err, &te) {
		t.Fatalf("error is not a *talos.Error: %[1]T: %[1]v", err)
	}

	if te.Kind != talos.KindUnreachable {
		t.Errorf("Kind = %v, want %v", te.Kind, talos.KindUnreachable)
	}
	if !strings.Contains(te.Error(), string(target.Machine)) {
		t.Errorf("Error() = %q does not name the machine %q", te.Error(), target.Machine)
	}
	if strings.Contains(te.Error(), sentinel) {
		t.Errorf("Error() = %q leaks the node address", te.Error())
	}
	if te.Op == "" {
		t.Error("Op is empty; a transport failure whose only record says nothing happened is the one " +
			"that is hardest to diagnose")
	}
	if !strings.Contains(te.Error(), te.Op) {
		t.Errorf("Error() = %q does not name the operation %q", te.Error(), te.Op)
	}

	// The unwrapped cause is still reachable for a developer with a debugger;
	// it is the rendered message that is address-free.
	if te.Unwrap() == nil {
		t.Error("Unwrap() is nil; the cause has been discarded rather than wrapped")
	}
}

// TestClassifyDistinguishesAnsweredFromUnreachable is the distinction the
// breaker in task 4 rests on: "the node is gone" and "the node said no" must
// not open the same circuit.
func TestClassifyDistinguishesAnsweredFromUnreachable(t *testing.T) {
	t.Parallel()

	sim := newSim(t, talossim.Options{Hostname: "classify-node"})

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	cc := newClusterClient(t, sim)

	// etcd_down is the case that matters most. Every Etcd* RPC answers
	// codes.Unavailable -- the same status code a dead connection produces --
	// from a node that is demonstrably reachable, because Version keeps
	// answering. A classifier that read the code alone would call this
	// unreachable, open the node's circuit, and turn one broken subsystem into
	// a node-wide outage.
	restore, err := sim.Inject(talossim.Scenario{Name: talossim.ScenarioEtcdDown})
	if err != nil {
		t.Fatalf("Inject: %v", err)
	}
	defer restore()

	_, err = cc.EtcdMemberList(ctx)
	if err == nil {
		t.Fatal("EtcdMemberList succeeded while etcd_down was injected")
	}

	var te *talos.Error
	if !errors.As(err, &te) {
		t.Fatalf("error is not a *talos.Error: %[1]T: %[1]v", err)
	}
	if te.Kind != talos.KindRejected {
		t.Errorf("Kind = %v, want %v: the node answered, so this is a refusal and not a transport failure",
			te.Kind, talos.KindRejected)
	}
	if te.Status != codes.Unavailable {
		t.Errorf("Status = %v, want %v (the upstream code is carried, not swallowed)", te.Status, codes.Unavailable)
	}

	// And the node is still reachable, which is the other half of the claim.
	if _, err := cc.Version(ctx); err != nil {
		t.Errorf("Version failed while only etcd was down: %v", err)
	}
}
