package audit

// The read path: a filtered, cursor-paginated view over the day files.
//
// Everything here is shaped by docs/api-contract.md § Audit Query Contract,
// which is binding: plan 05 builds its table against that document without
// talking to this file. Sorting is newest first, always; the parameters are
// from, to, action, limit and cursor; and next_cursor is number-or-null.

import (
	"context"
	"path/filepath"
	"strings"
	"time"
)

const (
	// DefaultLimit applies when a caller asks for no particular page size.
	DefaultLimit = 100

	// MaxLimit is the ceiling. Without one, a single unparameterised request
	// against an archive that is never shortened (D-16) would pull a year of
	// records into memory -- a denial of service that needs no attacker, just
	// an impatient operator (T-01-21).
	MaxLimit = 1000
)

// Filter selects records. The zero value means "everything, newest first,
// one default-sized page".
type Filter struct {
	From   time.Time
	To     time.Time
	Action string
	Limit  int

	// Cursor is the seq of the last record of the previous page; the next page
	// continues strictly below it. A sequence number rather than an offset, so
	// pagination stays stable while records are being appended and does not
	// depend on file names.
	Cursor uint64
}

// Page is one page of results, in the exact wire shape of the contract.
type Page struct {
	Items []Record `json:"items"`

	// NextCursor is a pointer, and the tag carries no flag that would let the
	// encoder drop it, because the contract pins both halves of the wire form:
	// the field is always present, and exhaustion is the value null. A plain
	// uint64 would send 0 -- which is never a valid cursor -- and telling the
	// encoder to skip empty values would drop the field entirely, merging "no
	// more pages" with "I forgot to tell you". A client would then be correct
	// only by the accident that 0 is falsy.
	NextCursor *uint64 `json:"next_cursor"`
}

// Query reads the audit directory dir -- that is <data-dir>/audit -- and returns
// one page of records, newest first.
//
// It walks the day files backwards and stops as soon as the page is full, so
// the cost is bounded by the page size rather than by the size of the archive.
func Query(ctx context.Context, dir string, f Filter) (Page, error) {
	limit := effectiveLimit(f.Limit)

	files, err := allFiles(dir)
	if err != nil {
		return Page{}, err
	}
	files = withinWindow(files, f.From, f.To)

	page := Page{Items: make([]Record, 0, limit)}

	for i := len(files) - 1; i >= 0; i-- {
		if err := ctx.Err(); err != nil {
			return Page{}, err
		}
		recs, err := readFile(files[i])
		if err != nil {
			return Page{}, err
		}
		for j := len(recs) - 1; j >= 0; j-- {
			rec := recs[j]
			if !matches(rec, f) {
				continue
			}
			if len(page.Items) == limit {
				// A further match exists, so this page is not the last. The
				// cursor is the seq of the last record actually delivered,
				// which is what makes the next page begin exactly below it --
				// no overlap, no gap.
				cursor := page.Items[limit-1].Seq
				page.NextCursor = &cursor
				return page, nil
			}
			page.Items = append(page.Items, rec)
		}
	}
	return page, nil
}

// Query is the same read against the live logger. It takes the write lock so a
// page can never be assembled from a half-written line.
func (l *Logger) Query(ctx context.Context, f Filter) (Page, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return Query(ctx, l.dir, f)
}

func effectiveLimit(n int) int {
	switch {
	case n <= 0:
		return DefaultLimit
	case n > MaxLimit:
		return MaxLimit
	default:
		return n
	}
}

func matches(rec Record, f Filter) bool {
	if f.Cursor != 0 && rec.Seq >= f.Cursor {
		return false
	}
	if !f.From.IsZero() && rec.TS.Before(f.From) {
		return false
	}
	if !f.To.IsZero() && rec.TS.After(f.To) {
		return false
	}
	if f.Action != "" && rec.Action != f.Action {
		return false
	}
	return true
}

// withinWindow drops the day files that cannot contain a record in the window,
// so a query over one day does not decompress a year.
//
// Files are named for the UTC day they cover and the timestamps inside are UTC,
// so comparing the formatted day strings is both correct and cheap: the layout
// is fixed-width, which makes lexicographic order chronological order.
func withinWindow(files []string, from, to time.Time) []string {
	if from.IsZero() && to.IsZero() {
		return files
	}
	var lo, hi string
	if !from.IsZero() {
		lo = from.UTC().Format(dayLayout)
	}
	if !to.IsZero() {
		hi = to.UTC().Format(dayLayout)
	}

	out := files[:0:0]
	for _, p := range files {
		day := dayOf(p)
		if lo != "" && day < lo {
			continue
		}
		if hi != "" && day > hi {
			continue
		}
		out = append(out, p)
	}
	return out
}

// dayOf extracts the YYYY-MM-DD a day file covers.
func dayOf(path string) string {
	base := filepath.Base(path)
	base = strings.TrimPrefix(base, filePrefix)
	base = strings.TrimSuffix(base, compressedExt)
	return strings.TrimSuffix(base, fileSuffix)
}
