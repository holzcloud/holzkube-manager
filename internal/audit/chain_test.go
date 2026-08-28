package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

// TestChainGenesisIsDefined pins the anchor of the chain. The first record of a
// fresh data directory must carry a defined value, not an empty string: an
// empty prev_hash is indistinguishable from a field that was stripped, which is
// exactly the ambiguity tamper-evidence cannot afford.
func TestChainGenesisIsDefined(t *testing.T) {
	if Genesis == "" {
		t.Fatal("Genesis is the empty string")
	}
	sum := sha256.Sum256([]byte(genesisDomain))
	if want := hex.EncodeToString(sum[:]); Genesis != want {
		t.Errorf("Genesis = %q, want sha256(%q) = %q", Genesis, genesisDomain, want)
	}

	dir := t.TempDir()
	clk := newClock(t)
	l := openAt(t, dir, clk)

	if _, err := l.Attempt(context.Background(), Record{Actor: "holz", Action: "auth.login"}); err != nil {
		t.Fatalf("Attempt: %v", err)
	}
	recs := readAll(t, l.CurrentFile())
	if recs[0].PrevHash != Genesis {
		t.Errorf("first record prev_hash = %q, want the genesis constant %q", recs[0].PrevHash, Genesis)
	}
}

// TestChainVerifiesAcrossCompressedAndPlainFiles is the seam this plan exists
// to hold: a chain spread over several days, one of them gzipped, still adds up.
func TestChainVerifiesAcrossCompressedAndPlainFiles(t *testing.T) {
	dir := t.TempDir()
	clk := newClock(t)
	l := openAt(t, dir, clk)
	writeDays(t, l, clk, 3, 2)
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	files := dayFiles(t, dir)
	if !strings.HasSuffix(files[0], gzSuffix) {
		t.Fatalf("expected the oldest file to be compressed, got %s", files[0])
	}

	ok, file, line, err := Verify(files)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !ok {
		t.Fatalf("clean chain did not verify: %s line %d", file, line)
	}
}

// TestChainDetectsTamperingReportsFileAndLine checks that a rewritten line in a
// rotated file is named precisely enough to act on.
func TestChainDetectsTamperingReportsFileAndLine(t *testing.T) {
	dir := t.TempDir()
	clk := newClock(t)
	l := openAt(t, dir, clk)
	writeDays(t, l, clk, 3, 2)
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	files := dayFiles(t, dir)
	// The middle day is plain (only the oldest is compressed), so it can be
	// edited exactly the way an attacker with write access would edit it.
	target := files[1]
	tamperLine(t, target, 2, func(r *Record) { r.Actor = "mallory" })

	ok, file, line, err := Verify(files)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if ok {
		t.Fatal("a rewritten record verified anyway; the chain is not tamper-evident")
	}
	if file != target {
		t.Errorf("broken file = %q, want %q", file, target)
	}
	if line != 2 {
		t.Errorf("broken line = %d, want 2", line)
	}
}

// TestChainVerifyNeverWrites holds the prohibition with teeth: a break is
// reported, never repaired. Recomputing a broken chain would destroy the only
// evidence that it was broken.
func TestChainVerifyNeverWrites(t *testing.T) {
	dir := t.TempDir()
	clk := newClock(t)
	l := openAt(t, dir, clk)
	writeDays(t, l, clk, 2, 2)
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	files := dayFiles(t, dir)
	tamperLine(t, files[1], 1, func(r *Record) { r.Action = "node.wipe" })

	before := snapshot(t, dir)
	if ok, _, _, err := Verify(files); err != nil || ok {
		t.Fatalf("expected a detected break, got ok=%v err=%v", ok, err)
	}
	// Twice, because an idempotent repair would still be a repair.
	if ok, _, _, err := Verify(files); err != nil || ok {
		t.Fatalf("second Verify disagreed with the first: ok=%v err=%v", ok, err)
	}
	after := snapshot(t, dir)

	if len(before) != len(after) {
		t.Fatalf("Verify changed the file set: %d before, %d after", len(before), len(after))
	}
	for name, content := range before {
		if after[name] != content {
			t.Errorf("Verify modified %s", name)
		}
	}
}

// TestChainVerifySeedsFromAPartialWindow covers the startup case: verification
// looks at the current and the last rotated file only, so the oldest record in
// the window has no predecessor to check against and must be accepted as a seed
// rather than reported as a break.
func TestChainVerifySeedsFromAPartialWindow(t *testing.T) {
	dir := t.TempDir()
	clk := newClock(t)
	l := openAt(t, dir, clk)
	writeDays(t, l, clk, 4, 2)
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	files := dayFiles(t, dir)
	ok, file, line, err := Verify(files[len(files)-2:])
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !ok {
		t.Fatalf("a partial window reported a break at %s line %d", file, line)
	}
}

// TestLoggerVerifyCoversTheLastRotatedFile is D-15 as behaviour: the startup
// check reads the current file and the one before it, so damage to yesterday is
// found this morning rather than never.
func TestLoggerVerifyCoversTheLastRotatedFile(t *testing.T) {
	dir := t.TempDir()
	clk := newClock(t)
	l := openAt(t, dir, clk)
	writeDays(t, l, clk, 2, 2)
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	files := dayFiles(t, dir)
	yesterday := files[0]
	tamperLine(t, yesterday, 1, func(r *Record) { r.Actor = "mallory" })

	reopened := openAt(t, dir, clk)
	defer reopened.Close()

	ok, file, line, err := reopened.Verify(context.Background())
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if ok {
		t.Fatal("a break in the last rotated file went unnoticed at startup")
	}
	if file != yesterday {
		t.Errorf("broken file = %q, want %q", file, yesterday)
	}
	if line != 1 {
		t.Errorf("broken line = %d, want 1", line)
	}
}

func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, p := range dayFiles(t, dir) {
		raw, err := os.ReadFile(p) //nolint:gosec // test-owned temp dir
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		out[p] = string(raw)
	}
	return out
}
