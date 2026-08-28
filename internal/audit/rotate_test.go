package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// clock is the injectable time source. Rotation is a daily event, and a test
// that had to wait for midnight would never be run.
type clock struct{ t time.Time }

func newClock(t *testing.T) *clock {
	t.Helper()
	return &clock{t: time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)}
}

func (c *clock) now() time.Time { return c.t }

func (c *clock) advanceDays(n int) { c.t = c.t.AddDate(0, 0, n) }

func openAt(t *testing.T, dir string, c *clock) *Logger {
	t.Helper()
	l, err := open(dir, c.now)
	if err != nil {
		t.Fatalf("open audit log: %v", err)
	}
	return l
}

// writeDays writes perDay records on each of days consecutive days, advancing
// the clock between them so the logger has to rotate.
func writeDays(t *testing.T, l *Logger, c *clock, days, perDay int) {
	t.Helper()
	for d := range days {
		if d > 0 {
			c.advanceDays(1)
		}
		for i := range perDay {
			_, err := l.Attempt(context.Background(), Record{
				Actor:  "holz",
				Action: fmt.Sprintf("day%d.record%d", d+1, i+1),
			})
			if err != nil {
				t.Fatalf("Attempt on day %d: %v", d+1, err)
			}
		}
	}
}

// dayFiles lists every audit file, compressed or not, in date order.
func dayFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, "audit"))
	if err != nil {
		t.Fatalf("read audit dir: %v", err)
	}
	var out []string
	for _, e := range entries {
		n := e.Name()
		if !strings.HasPrefix(n, filePrefix) {
			continue
		}
		if !strings.HasSuffix(n, fileSuffix) && !strings.HasSuffix(n, gzSuffix) {
			continue
		}
		out = append(out, filepath.Join(dir, "audit", n))
	}
	sort.Strings(out)
	return out
}

func readAll(t *testing.T, path string) []Record {
	t.Helper()
	recs, err := readFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return recs
}

// tamperLine rewrites one 1-based line of a plain audit file, which is exactly
// the edit an attacker with write access to the data directory would make.
func tamperLine(t *testing.T, path string, n int, mut func(*Record)) {
	t.Helper()
	if strings.HasSuffix(path, gzSuffix) {
		t.Fatalf("tamperLine only handles plain files, got %s", path)
	}
	raw, err := os.ReadFile(path) //nolint:gosec // test-owned temp dir
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if n < 1 || n > len(lines) {
		t.Fatalf("line %d out of range: %s has %d lines", n, path, len(lines))
	}
	var rec Record
	if err := json.Unmarshal([]byte(lines[n-1]), &rec); err != nil {
		t.Fatalf("decode line %d of %s: %v", n, path, err)
	}
	mut(&rec)
	edited, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("encode line %d: %v", n, err)
	}
	lines[n-1] = string(edited)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), filePerm); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestRotateKeepsOneFilePerDay is the base case: two records on the same day
// share a file, a sequence and a link in the chain.
func TestRotateKeepsOneFilePerDay(t *testing.T) {
	dir := t.TempDir()
	clk := newClock(t)
	l := openAt(t, dir, clk)
	defer l.Close()

	writeDays(t, l, clk, 1, 2)

	files := dayFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("files = %v, want exactly one", files)
	}
	if got := filepath.Base(files[0]); got != "audit-2026-03-01.jsonl" {
		t.Errorf("file name = %q, want audit-2026-03-01.jsonl", got)
	}

	recs := readAll(t, files[0])
	if len(recs) != 2 {
		t.Fatalf("records = %d, want 2", len(recs))
	}
	if recs[1].Seq != recs[0].Seq+1 {
		t.Errorf("seq %d followed by %d; the sequence is not contiguous", recs[0].Seq, recs[1].Seq)
	}
	if recs[1].PrevHash != recs[0].Hash {
		t.Errorf("second record prev_hash = %q, want the first record's hash %q", recs[1].PrevHash, recs[0].Hash)
	}
}

// TestRotateJSONLIsOneRecordPerLine guards the format itself. The store writes
// indented JSON since plan 02; the audit log must not acquire that habit,
// because a record spread over several lines has no line number to report.
func TestRotateJSONLIsOneRecordPerLine(t *testing.T) {
	dir := t.TempDir()
	clk := newClock(t)
	l := openAt(t, dir, clk)
	defer l.Close()

	writeDays(t, l, clk, 1, 3)

	raw, err := os.ReadFile(l.CurrentFile()) //nolint:gosec // test-owned temp dir
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3 (one per record)", len(lines))
	}
	for i, line := range lines {
		var rec Record
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Errorf("line %d is not a complete record: %v", i+1, err)
		}
		if strings.Contains(line, "\n  ") {
			t.Errorf("line %d is indented; the audit log is JSONL", i+1)
		}
	}
}

// TestRotateCarriesTheChainAcrossTheDayBoundary is the seam threat T-01-19
// names: rotation must hand the chain over, not start a second one.
func TestRotateCarriesTheChainAcrossTheDayBoundary(t *testing.T) {
	dir := t.TempDir()
	clk := newClock(t)
	l := openAt(t, dir, clk)
	defer l.Close()

	writeDays(t, l, clk, 2, 2)

	files := dayFiles(t, dir)
	if len(files) != 2 {
		t.Fatalf("files = %v, want two days", files)
	}
	if got := filepath.Base(files[1]); got != "audit-2026-03-02.jsonl" {
		t.Errorf("second day file = %q, want audit-2026-03-02.jsonl", got)
	}

	day1 := readAll(t, files[0])
	day2 := readAll(t, files[1])
	last, first := day1[len(day1)-1], day2[0]

	if first.PrevHash != last.Hash {
		t.Errorf("first record of the new day has prev_hash %q, want %q from the previous day",
			first.PrevHash, last.Hash)
	}
	if first.Seq != last.Seq+1 {
		t.Errorf("sequence restarted at the file boundary: %d followed by %d", last.Seq, first.Seq)
	}
}

// TestRotateCompressesFromTheSecondDayOn covers D-16: the day before last is
// gzipped, the day before this one is not.
func TestRotateCompressesFromTheSecondDayOn(t *testing.T) {
	dir := t.TempDir()
	clk := newClock(t)
	l := openAt(t, dir, clk)
	defer l.Close()

	writeDays(t, l, clk, 3, 2)

	files := dayFiles(t, dir)
	if len(files) != 3 {
		t.Fatalf("files = %v, want three days", files)
	}
	if got := filepath.Base(files[0]); got != "audit-2026-03-01.jsonl"+compressedExt {
		t.Errorf("oldest file = %q, want the compressed form", got)
	}
	if got := filepath.Base(files[1]); got != "audit-2026-03-02.jsonl" {
		t.Errorf("previous day = %q, want it left uncompressed", got)
	}
	if got := filepath.Base(files[2]); got != "audit-2026-03-03.jsonl" {
		t.Errorf("current day = %q, want it left uncompressed", got)
	}

	// A compressed file is still readable as records, or the archive is a
	// write-only heap.
	recs := readAll(t, files[0])
	if len(recs) != 2 {
		t.Fatalf("records in the compressed file = %d, want 2", len(recs))
	}
	if recs[0].Action != "day1.record1" {
		t.Errorf("compressed file lost its content: action = %q", recs[0].Action)
	}

	// No leftover temporary, and nothing wider than 0600 (T-01-20).
	for _, p := range files {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", p, err)
		}
		if info.Mode().Perm() != filePerm {
			t.Errorf("%s is mode %04o, want %04o", p, info.Mode().Perm(), filePerm)
		}
	}
	entries, err := os.ReadDir(filepath.Join(dir, "audit"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("compression left a temporary behind: %s", e.Name())
		}
	}
}

// TestRotateNeverRemovesAFile is D-16 stated as behaviour: the archive only
// ever grows. Losing an old day would break the chain at the seam and make
// "when did this start?" unanswerable, which is the one question it is for.
func TestRotateNeverRemovesAFile(t *testing.T) {
	dir := t.TempDir()
	clk := newClock(t)
	l := openAt(t, dir, clk)
	defer l.Close()

	writeDays(t, l, clk, 5, 1)

	files := dayFiles(t, dir)
	if len(files) != 5 {
		t.Fatalf("files = %d, want 5; no audit file is ever removed", len(files))
	}
	for i, want := range []string{
		"audit-2026-03-01.jsonl" + compressedExt,
		"audit-2026-03-02.jsonl" + compressedExt,
		"audit-2026-03-03.jsonl" + compressedExt,
		"audit-2026-03-04.jsonl",
		"audit-2026-03-05.jsonl",
	} {
		if got := filepath.Base(files[i]); got != want {
			t.Errorf("file %d = %q, want %q", i, got, want)
		}
	}
}

// TestRotateResumesFromACompressedTail covers a restart on a day whose
// predecessor has already been compressed: the sequence and the chain must
// continue rather than fork.
func TestRotateResumesFromACompressedTail(t *testing.T) {
	dir := t.TempDir()
	clk := newClock(t)

	first := openAt(t, dir, clk)
	writeDays(t, first, clk, 3, 1)
	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	files := dayFiles(t, dir)
	tail := readAll(t, files[len(files)-1])
	last := tail[len(tail)-1]

	second := openAt(t, dir, clk)
	defer second.Close()
	seq, err := second.Attempt(context.Background(), Record{Actor: "holz", Action: "auth.login"})
	if err != nil {
		t.Fatalf("Attempt after restart: %v", err)
	}
	if seq != last.Seq+1 {
		t.Errorf("sequence after restart = %d, want %d", seq, last.Seq+1)
	}

	recs := readAll(t, second.CurrentFile())
	resumed := recs[len(recs)-1]
	if resumed.PrevHash != last.Hash {
		t.Errorf("restart forked the chain: prev_hash = %q, want %q", resumed.PrevHash, last.Hash)
	}

	ok, file, line, err := Verify(dayFiles(t, dir))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !ok {
		t.Errorf("chain broke across a restart at %s line %d", file, line)
	}
}
