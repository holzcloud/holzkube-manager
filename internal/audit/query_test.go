package audit

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// seed writes perDay records on each of days days and returns the logger's
// audit directory.
func seed(t *testing.T, days, perDay int) (dir, logDir string, clk *clock) {
	t.Helper()
	dir = t.TempDir()
	clk = newClock(t)
	l := openAt(t, dir, clk)
	writeDays(t, l, clk, days, perDay)
	logDir = l.dir
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return dir, logDir, clk
}

// TestQueryReturnsNewestFirst is the sort order the contract fixes. It is not a
// preference: the UI table and every later phase read it this way.
func TestQueryReturnsNewestFirst(t *testing.T) {
	_, logDir, _ := seed(t, 1, 5)

	page, err := Query(context.Background(), logDir, Filter{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(page.Items) != 5 {
		t.Fatalf("items = %d, want 5", len(page.Items))
	}
	for i := 1; i < len(page.Items); i++ {
		if page.Items[i-1].Seq <= page.Items[i].Seq {
			t.Fatalf("not newest-first: seq %d followed by %d",
				page.Items[i-1].Seq, page.Items[i].Seq)
		}
	}
	if page.NextCursor != nil {
		t.Errorf("next_cursor = %d, want nil when everything fits", *page.NextCursor)
	}
}

// TestQueryPaginatesWithoutOverlapOrGap walks the whole set two at a time and
// checks that every record appears exactly once.
func TestQueryPaginatesWithoutOverlapOrGap(t *testing.T) {
	_, logDir, _ := seed(t, 1, 5)

	seen := map[uint64]int{}
	var cursor uint64
	pages := 0

	for {
		page, err := Query(context.Background(), logDir, Filter{Limit: 2, Cursor: cursor})
		if err != nil {
			t.Fatalf("Query: %v", err)
		}
		pages++
		if pages > 10 {
			t.Fatal("pagination did not terminate")
		}
		for _, rec := range page.Items {
			seen[rec.Seq]++
		}
		if page.NextCursor == nil {
			if len(page.Items) != 1 {
				t.Errorf("last page has %d items, want the remaining 1", len(page.Items))
			}
			break
		}
		if len(page.Items) != 2 {
			t.Fatalf("page %d has %d items, want the limit of 2", pages, len(page.Items))
		}
		if *page.NextCursor != page.Items[len(page.Items)-1].Seq {
			t.Errorf("next_cursor = %d, want the last delivered seq %d",
				*page.NextCursor, page.Items[len(page.Items)-1].Seq)
		}
		cursor = *page.NextCursor
	}

	if pages != 3 {
		t.Errorf("pages = %d, want 3 for 5 records at limit 2", pages)
	}
	for seq := uint64(1); seq <= 5; seq++ {
		switch seen[seq] {
		case 0:
			t.Errorf("seq %d was never delivered: a gap", seq)
		case 1:
		default:
			t.Errorf("seq %d was delivered %d times: an overlap", seq, seen[seq])
		}
	}
}

// TestQueryExhaustionSerializesAsNull checks the produced bytes, not the Go
// value. The contract pins the wire form: the field is always present, is null
// when exhausted, and is never 0 -- so a client that tests !== null is right and
// a client that tests truthiness is only accidentally right.
func TestQueryExhaustionSerializesAsNull(t *testing.T) {
	_, logDir, _ := seed(t, 1, 2)

	page, err := Query(context.Background(), logDir, Filter{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	raw, err := json.Marshal(page)
	if err != nil {
		t.Fatalf("marshal page: %v", err)
	}
	got := string(raw)

	if !strings.Contains(got, `"next_cursor":null`) {
		t.Errorf("exhausted page = %s, want next_cursor null", got)
	}
	if strings.Contains(got, `"next_cursor":0`) {
		t.Errorf("page sent 0 as a cursor: %s", got)
	}
	if !strings.Contains(got, `"next_cursor"`) {
		t.Errorf("page omitted next_cursor entirely: %s", got)
	}
}

// TestQueryEmptyResultIsAnArrayNotNull keeps the client from having to handle
// two shapes of "nothing".
func TestQueryEmptyResultIsAnArrayNotNull(t *testing.T) {
	_, logDir, _ := seed(t, 1, 1)

	page, err := Query(context.Background(), logDir, Filter{Action: "nothing.matches"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	raw, err := json.Marshal(page)
	if err != nil {
		t.Fatalf("marshal page: %v", err)
	}
	if !strings.Contains(string(raw), `"items":[]`) {
		t.Errorf("empty page = %s, want items as an empty array", raw)
	}
}

// TestQueryFiltersByAction matches the token exactly; a prefix is not a match.
func TestQueryFiltersByAction(t *testing.T) {
	_, logDir, _ := seed(t, 2, 3)

	page, err := Query(context.Background(), logDir, Filter{Action: "day2.record2"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("items = %d, want exactly the one matching action", len(page.Items))
	}
	if page.Items[0].Action != "day2.record2" {
		t.Errorf("action = %q, want day2.record2", page.Items[0].Action)
	}

	if page, err = Query(context.Background(), logDir, Filter{Action: "day2"}); err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(page.Items) != 0 {
		t.Errorf("a prefix matched %d records; the filter is exact", len(page.Items))
	}
}

// TestQuerySpansCompressedAndPlainFiles is the point of the read path: the
// archive is one sequence, not one sequence per file format.
func TestQuerySpansCompressedAndPlainFiles(t *testing.T) {
	_, logDir, _ := seed(t, 3, 2)

	files, err := allFiles(logDir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(files[0], gzSuffix) {
		t.Fatalf("fixture is wrong: %s is not compressed", files[0])
	}

	page, err := Query(context.Background(), logDir, Filter{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(page.Items) != 6 {
		t.Fatalf("items = %d, want 6 across three days", len(page.Items))
	}
	if page.Items[0].Action != "day3.record2" {
		t.Errorf("newest = %q, want day3.record2", page.Items[0].Action)
	}
	if page.Items[5].Action != "day1.record1" {
		t.Errorf("oldest = %q, want day1.record1 out of the compressed file", page.Items[5].Action)
	}
}

// TestQueryFiltersByTimeWindowAcrossFiles narrows to the middle day, which lives
// in its own file, and must exclude both neighbours -- one of them compressed.
func TestQueryFiltersByTimeWindowAcrossFiles(t *testing.T) {
	_, logDir, clk := seed(t, 3, 2)

	day2 := clk.t.AddDate(0, 0, -1)
	from := time.Date(day2.Year(), day2.Month(), day2.Day(), 0, 0, 0, 0, time.UTC)
	to := from.Add(24*time.Hour - time.Nanosecond)

	page, err := Query(context.Background(), logDir, Filter{From: from, To: to})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("items = %d, want the 2 records of the middle day", len(page.Items))
	}
	for _, rec := range page.Items {
		if !strings.HasPrefix(rec.Action, "day2.") {
			t.Errorf("record from outside the window: %q at %s", rec.Action, rec.TS)
		}
	}
}

// TestQueryWindowSpansTwoFiles keeps the day-file preselection honest: a window
// that covers two days must return both, compressed or not.
func TestQueryWindowSpansTwoFiles(t *testing.T) {
	_, logDir, clk := seed(t, 3, 2)

	to := clk.t
	from := to.AddDate(0, 0, -2)

	page, err := Query(context.Background(), logDir, Filter{From: from, To: to})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(page.Items) != 6 {
		t.Fatalf("items = %d, want all 6", len(page.Items))
	}
}

// TestQueryLimitHasADefaultAndACeiling stops an unparameterised call from
// pulling a year of records into memory (T-01-21).
func TestQueryLimitHasADefaultAndACeiling(t *testing.T) {
	_, logDir, _ := seed(t, 1, 3)

	if _, err := Query(context.Background(), logDir, Filter{Limit: 0}); err != nil {
		t.Fatalf("Query with no limit: %v", err)
	}
	if DefaultLimit <= 0 || DefaultLimit > MaxLimit {
		t.Fatalf("DefaultLimit = %d, MaxLimit = %d: incoherent", DefaultLimit, MaxLimit)
	}

	page, err := Query(context.Background(), logDir, Filter{Limit: MaxLimit * 10})
	if err != nil {
		t.Fatalf("Query with an absurd limit: %v", err)
	}
	if len(page.Items) != 3 {
		t.Errorf("items = %d, want 3", len(page.Items))
	}
	if got := effectiveLimit(MaxLimit * 10); got != MaxLimit {
		t.Errorf("effectiveLimit(%d) = %d, want the ceiling %d", MaxLimit*10, got, MaxLimit)
	}
	if got := effectiveLimit(0); got != DefaultLimit {
		t.Errorf("effectiveLimit(0) = %d, want the default %d", got, DefaultLimit)
	}
}

// TestLoggerQueryReadsWhatIsBeingWritten checks the method on the live logger,
// which is what the handler actually calls.
func TestLoggerQueryReadsWhatIsBeingWritten(t *testing.T) {
	dir := t.TempDir()
	clk := newClock(t)
	l := openAt(t, dir, clk)
	defer l.Close()

	writeDays(t, l, clk, 1, 3)

	page, err := l.Query(context.Background(), Filter{Limit: 2})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(page.Items))
	}
	if page.NextCursor == nil {
		t.Fatal("next_cursor is nil although a third record exists")
	}
	if *page.NextCursor != page.Items[1].Seq {
		t.Errorf("next_cursor = %d, want %d", *page.NextCursor, page.Items[1].Seq)
	}
}
