package audit

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// TestCanonicalJSONIsKeySorted pins the property the whole chain rests on: the
// hashed bytes are sorted, unspaced and independent of Go's map iteration order.
func TestCanonicalJSONIsKeySorted(t *testing.T) {
	rec := Record{
		Seq:      7,
		TS:       time.Date(2026, 8, 28, 9, 14, 2, 0, time.UTC),
		Actor:    "holz",
		Action:   "node.reset",
		Params:   map[string]any{"zulu": 1, "alpha": "a", "mike": map[string]any{"z": true, "a": false}},
		Outcome:  OutcomeAttempt,
		PrevHash: "deadbeef",
		Hash:     "this value must not influence the hash",
	}

	canon, err := rec.CanonicalJSON()
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	got := string(canon)

	if strings.Contains(got, `"hash"`) {
		t.Errorf("canonical form includes the hash field: %s", got)
	}
	if strings.Contains(got, " ") {
		t.Errorf("canonical form contains whitespace: %s", got)
	}
	if !strings.Contains(got, `"params":{"alpha":"a","mike":{"a":false,"z":true},"zulu":1}`) {
		t.Errorf("nested params are not key-sorted: %s", got)
	}

	// Re-canonicalizing many times must be byte-identical, or map ordering is
	// leaking into the chain.
	for range 50 {
		again, err := rec.CanonicalJSON()
		if err != nil {
			t.Fatalf("CanonicalJSON: %v", err)
		}
		if string(again) != got {
			t.Fatalf("canonical form is not stable:\n%s\n%s", got, again)
		}
	}
}

// TestHashIgnoresHashFieldAndFollowsPrev checks the chain formula itself.
func TestHashIgnoresHashFieldAndFollowsPrev(t *testing.T) {
	base := Record{Seq: 1, Actor: "holz", Action: "auth.login", Outcome: OutcomeAttempt, PrevHash: "abc"}

	withHash := base
	withHash.Hash = "whatever"
	h1, err := base.ComputeHash()
	if err != nil {
		t.Fatal(err)
	}
	h2, err := withHash.ComputeHash()
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Errorf("the hash field influenced the hash: %s vs %s", h1, h2)
	}

	moved := base
	moved.PrevHash = "abd"
	h3, err := moved.ComputeHash()
	if err != nil {
		t.Fatal(err)
	}
	if h3 == h1 {
		t.Errorf("changing prev_hash did not change the hash")
	}
}

// TestVerifyDetectsTampering is the test that makes the tamper-evidence claim
// real rather than asserted (threat T-01-04).
func TestVerifyDetectsTampering(t *testing.T) {
	dir := t.TempDir()

	l, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for range 3 {
		seq, err := l.Attempt(context.Background(), Record{
			Actor:  "holz",
			Action: "node.reset",
			Params: map[string]any{"graceful": true},
		})
		if err != nil {
			t.Fatalf("Attempt: %v", err)
		}
		if err := l.Outcome(context.Background(), seq, OutcomeSuccess, nil); err != nil {
			t.Fatalf("Outcome: %v", err)
		}
	}
	path := l.CurrentFile()
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if ok, line, err := verifyDir(t, dir); err != nil || !ok {
		t.Fatalf("clean chain did not verify: ok=%v line=%d err=%v", ok, line, err)
	}

	// Rewrite the actor on the third record, exactly the edit an attacker with
	// write access would make to disown an action.
	raw, err := os.ReadFile(path) //nolint:gosec // test-owned temp dir
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 6 {
		t.Fatalf("expected 6 records, got %d", len(lines))
	}
	var rec Record
	if err := json.Unmarshal([]byte(lines[2]), &rec); err != nil {
		t.Fatal(err)
	}
	rec.Actor = "mallory"
	edited, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	lines[2] = string(edited)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ok, line, err := verifyDir(t, dir)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if ok {
		t.Fatalf("a rewritten record verified anyway; the chain is not tamper-evident")
	}
	if line != 3 {
		t.Errorf("broken line = %d, want 3", line)
	}
}

// TestResumeContinuesTheChain guards the restart path: a new process must
// extend the existing chain, not start a second one.
func TestResumeContinuesTheChain(t *testing.T) {
	dir := t.TempDir()

	first, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	seq, err := first.Attempt(context.Background(), Record{Actor: "holz", Action: "auth.login"})
	if err != nil {
		t.Fatal(err)
	}
	if seq != 1 {
		t.Fatalf("first sequence = %d, want 1", seq)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	next, err := second.Attempt(context.Background(), Record{Actor: "holz", Action: "auth.logout"})
	if err != nil {
		t.Fatal(err)
	}
	if next != 2 {
		t.Errorf("sequence after restart = %d, want 2", next)
	}

	ok, line, err := second.Verify(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Errorf("chain broke across a restart at line %d", line)
	}
}

func verifyDir(t *testing.T, dir string) (bool, int, error) {
	t.Helper()
	v, err := Open(dir)
	if err != nil {
		return false, 0, err
	}
	defer v.Close()
	return v.Verify(context.Background())
}
