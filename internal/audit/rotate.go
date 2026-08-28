package audit

// Daily rotation, gzip of older days, and the file-level reading the rest of
// the package builds on.
//
// Two properties are load-bearing here and are each covered by a test:
//
//   - The chain runs *across* the file boundary. The first record of a new day
//     carries the last hash of the previous day as its prev_hash, because the
//     logger keeps that hash in memory and rotation does not touch it. A chain
//     that restarted every midnight would make every seam unverifiable, which
//     is precisely where an attacker would cut.
//   - No file is ever removed, and there is no option that would remove one
//     (D-16). Losing an old day breaks the chain at the seam and makes "when
//     did this start?" -- the one question a forensic log exists to answer --
//     permanently unanswerable. A homelab writes a few KB a day; the archive is
//     not a problem worth a knob.

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// compressedExt is appended to a rotated file once it is gzipped.
	compressedExt = ".gz"
	gzSuffix      = fileSuffix + compressedExt
	tmpExt        = ".tmp"

	// keepPlain is how many of the most recent day files stay uncompressed:
	// today and yesterday. Compression starts from the second day on (D-16).
	keepPlain = 2
)

// openDay makes sure the logger is appending to the file for the given day,
// rotating if the day has turned. The caller holds l.mu.
func (l *Logger) openDay(now time.Time) error {
	day := now.Format(dayLayout)
	if l.file != nil && l.day == day {
		return nil
	}

	rotating := l.file != nil
	if rotating {
		// fsync before letting go: the last record of a day must be on the
		// platter before anything starts compressing that day's neighbours.
		if err := l.file.Sync(); err != nil {
			return fmt.Errorf("audit: fsync %s: %w", l.day, err)
		}
		if err := l.file.Close(); err != nil {
			return err
		}
		l.file = nil
	}

	path := filepath.Join(l.dir, filePrefix+day+fileSuffix)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, filePerm)
	if err != nil {
		return fmt.Errorf("audit: open %s: %w", path, err)
	}
	l.file = f
	l.day = day

	if rotating {
		// A failure here is fatal to the write, and therefore to the mutation
		// that triggered it. That is the same fail-closed stance the rest of
		// this subsystem takes: if holzkube cannot maintain its own archive --
		// disk full, permissions changed underneath it -- the operator has to
		// see that at once, not discover it months later next to the gap it
		// caused.
		if err := CompressOlderThan(l.dir, keepPlain); err != nil {
			return err
		}
	}
	return nil
}

// CompressOlderThan gzips every plain day file except the keep most recent
// ones. It is idempotent and never removes a day: a compressed file replaces
// its plain original, byte for byte recoverable.
func CompressOlderThan(dir string, keep int) error {
	plain, err := plainFiles(dir)
	if err != nil {
		return err
	}
	if keep < 1 || len(plain) <= keep {
		return nil
	}
	for _, p := range plain[:len(plain)-keep] {
		if err := compressFile(p); err != nil {
			return fmt.Errorf("audit: compress %s: %w", filepath.Base(p), err)
		}
	}
	return nil
}

// compressFile writes path.gz through a temporary and a rename, so an
// interruption leaves either the plain file or a whole .gz -- never a truncated
// archive that would read as a broken chain.
func compressFile(path string) error {
	src, err := os.Open(path) //nolint:gosec // audit owns its own log directory
	if err != nil {
		return err
	}
	defer src.Close()

	tmp := path + compressedExt + tmpExt
	dst, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, filePerm)
	if err != nil {
		return err
	}
	// An existing temporary from an earlier interruption keeps its old mode,
	// which O_CREATE does not correct. The data directory must stay owner-only
	// (FOUND-10), and an audit archive readable by group is exactly the leak
	// the guard exists to prevent.
	if err := dst.Chmod(filePerm); err != nil {
		dst.Close()
		os.Remove(tmp)
		return err
	}

	if err := gzipInto(dst, src); err != nil {
		dst.Close()
		os.Remove(tmp)
		return err
	}
	if err := dst.Sync(); err != nil {
		dst.Close()
		os.Remove(tmp)
		return err
	}
	if err := dst.Close(); err != nil {
		os.Remove(tmp)
		return err
	}

	if err := os.Rename(tmp, path+compressedExt); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := syncDir(filepath.Dir(path)); err != nil {
		return err
	}

	// The original goes only after the replacement is durable and named. The
	// order matters: a crash between the rename and this point leaves both
	// files rather than neither, and allFiles collapses that day to the
	// compressed copy so the reader sees one file per day.
	if err := os.Remove(path); err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
}

func gzipInto(dst io.Writer, src io.Reader) error {
	zw := gzip.NewWriter(dst)
	if _, err := io.Copy(zw, src); err != nil {
		zw.Close()
		return err
	}
	return zw.Close()
}

func syncDir(dir string) error {
	d, err := os.Open(dir) //nolint:gosec // audit owns its own log directory
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// plainFiles lists the uncompressed day files in date order.
func plainFiles(dir string) ([]string, error) {
	return listFiles(dir, func(name string) bool {
		return strings.HasSuffix(name, fileSuffix)
	})
}

// allFiles lists every day file, compressed or not, in date order, with at
// most one file per day. The date is fixed-width and directly follows the
// prefix, so a lexicographic sort is a chronological one.
func allFiles(dir string) ([]string, error) {
	files, err := listFiles(dir, func(name string) bool {
		return strings.HasSuffix(name, fileSuffix) || strings.HasSuffix(name, gzSuffix)
	})
	if err != nil {
		return nil, err
	}
	return dedupeByDay(files), nil
}

// dedupeByDay collapses a day that has both a plain and a compressed file,
// keeping the compressed one.
//
// compressFile renames the gzip into place and fsyncs the directory before it
// removes the plain original, so a crash in that window leaves both. The
// compressed file is therefore the one the rename made durable and the plain
// one is the leftover.
//
// Reading both is not a tolerable outcome, which is what the crash window's
// comment used to claim. sort.Strings puts audit-D.jsonl immediately before
// audit-D.jsonl.gz, because the former is a byte prefix of the latter, so Query
// returned every record of that day twice and Verify processed the day twice --
// the second pass starting from a prev_hash the first pass had already moved
// past, reporting a chain break in a file that is perfectly intact. D-15 makes
// that report permanent until an operator deals with the file by hand, on a
// banner the design deliberately made non-dismissible.
//
// plainFiles is deliberately left alone, so CompressOlderThan still finds the
// leftover and finishes the job on the next rotation.
func dedupeByDay(paths []string) []string {
	seen := make(map[string]int, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		day := dayOf(p)
		if i, ok := seen[day]; ok {
			if strings.HasSuffix(p, compressedExt) {
				out[i] = p
			}
			continue
		}
		seen[day] = len(out)
		out = append(out, p)
	}
	return out
}

func listFiles(dir string, keep func(string) bool) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasPrefix(n, filePrefix) || !keep(n) {
			continue
		}
		out = append(out, filepath.Join(dir, n))
	}
	sort.Strings(out)
	return out, nil
}

// readFile decodes every line of a day file in write order, transparently for
// compressed files.
func readFile(path string) ([]Record, error) {
	f, err := os.Open(path) //nolint:gosec // audit owns its own log directory
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var r io.Reader = f
	if strings.HasSuffix(path, compressedExt) {
		zr, err := gzip.NewReader(f)
		if err != nil {
			return nil, fmt.Errorf("audit: read %s: %w", filepath.Base(path), err)
		}
		defer zr.Close()
		r = zr
	}

	var out []Record
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec Record
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, fmt.Errorf("audit: decode %s line %d: %w", filepath.Base(path), len(out)+1, err)
		}
		out = append(out, rec)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
