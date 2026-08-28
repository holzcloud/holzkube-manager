// Package audit is holzkube's append-only JSONL log with a hash chain.
//
// It is called by middleware; it never reads an HTTP request. Rotation, gzip of
// older files, verification of rotated files and allowlist redaction of params
// belong to plan 03; this file establishes the format, the chain and the
// intent/outcome pair.
package audit

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	dirPerm  = 0o700
	filePerm = 0o600

	dayLayout  = "2006-01-02"
	filePrefix = "audit-"
	fileSuffix = ".jsonl"
)

// Logger appends records to the current day's file.
type Logger struct {
	mu   sync.Mutex
	dir  string
	file *os.File
	day  string

	seq      uint64
	lastHash string

	// pending links an outcome back to the intent that opened it. The record
	// format is closed, so the outcome repeats the intent's identifying fields
	// rather than carrying a reference field that would change the format.
	pending map[uint64]Record

	closed bool
}

// Open prepares <dir>/audit and returns a Logger positioned after the last
// record already on disk.
func Open(dir string) (*Logger, error) {
	if dir == "" {
		return nil, errors.New("audit: empty data directory")
	}
	logDir := filepath.Join(dir, "audit")
	if err := os.MkdirAll(logDir, dirPerm); err != nil {
		return nil, fmt.Errorf("audit: create log directory: %w", err)
	}

	l := &Logger{dir: logDir, pending: make(map[uint64]Record)}
	if err := l.resume(); err != nil {
		return nil, err
	}
	if err := l.openDay(time.Now().UTC()); err != nil {
		return nil, err
	}
	return l, nil
}

// resume reads the newest existing file to recover the sequence number and the
// tail of the chain, so a restart continues the chain instead of forking it.
func (l *Logger) resume() error {
	files, err := l.files()
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}
	recs, err := readFile(files[len(files)-1])
	if err != nil {
		return err
	}
	if len(recs) == 0 {
		return nil
	}
	last := recs[len(recs)-1]
	l.seq = last.Seq
	l.lastHash = last.Hash
	return nil
}

// files returns the audit files in ascending date order.
func (l *Logger) files() ([]string, error) {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasPrefix(n, filePrefix) || !strings.HasSuffix(n, fileSuffix) {
			continue
		}
		out = append(out, filepath.Join(l.dir, n))
	}
	sort.Strings(out)
	return out, nil
}

func (l *Logger) openDay(now time.Time) error {
	day := now.Format(dayLayout)
	if l.file != nil && l.day == day {
		return nil
	}
	if l.file != nil {
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
	return nil
}

// CurrentFile reports the file records are currently appended to.
func (l *Logger) CurrentFile() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return filepath.Join(l.dir, filePrefix+l.day+fileSuffix)
}

// Close flushes and releases the current file.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed || l.file == nil {
		l.closed = true
		return nil
	}
	l.closed = true
	err := l.file.Close()
	l.file = nil
	return err
}

// append seals a record into the chain and fsyncs it. The caller holds l.mu.
func (l *Logger) append(rec Record) (Record, error) {
	if l.closed {
		return Record{}, errors.New("audit: logger is closed")
	}
	now := time.Now().UTC()
	if err := l.openDay(now); err != nil {
		return Record{}, err
	}

	rec.Seq = l.seq + 1
	rec.TS = now
	rec.PrevHash = l.lastHash
	if rec.Params == nil {
		rec.Params = map[string]any{}
	}

	hash, err := rec.ComputeHash()
	if err != nil {
		return Record{}, fmt.Errorf("audit: hash record: %w", err)
	}
	rec.Hash = hash

	line, err := json.Marshal(rec)
	if err != nil {
		return Record{}, fmt.Errorf("audit: encode record: %w", err)
	}
	line = append(line, '\n')

	if _, err := l.file.Write(line); err != nil {
		return Record{}, fmt.Errorf("audit: append record: %w", err)
	}
	// fsync per record. The volume is a handful per minute at most, and an
	// intent that is not durable before the action happens is not an intent.
	if err := l.file.Sync(); err != nil {
		return Record{}, fmt.Errorf("audit: fsync record: %w", err)
	}

	l.seq = rec.Seq
	l.lastHash = rec.Hash
	return rec, nil
}

// Attempt writes the intent record and returns its sequence number. It must be
// durable before the action it describes is allowed to happen.
func (l *Logger) Attempt(_ context.Context, rec Record) (uint64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	rec.Outcome = OutcomeAttempt
	written, err := l.append(rec)
	if err != nil {
		return 0, err
	}
	l.pending[written.Seq] = written
	return written.Seq, nil
}

// Outcome writes the result record for a previously recorded attempt.
//
// It repeats the attempt's identifying fields. Params on the outcome carry only
// the failure reason, if any: the input parameters were already recorded by the
// attempt, and repeating them would double the surface that plan 03's redaction
// has to cover.
func (l *Logger) Outcome(_ context.Context, seq uint64, outcome string, cause error) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	att, ok := l.pending[seq]
	if !ok {
		return fmt.Errorf("audit: no pending attempt with seq %d", seq)
	}
	delete(l.pending, seq)

	params := map[string]any{}
	if cause != nil {
		params["error"] = cause.Error()
	}

	_, err := l.append(Record{
		Actor:     att.Actor,
		Session:   att.Session,
		SrcIP:     att.SrcIP,
		ClusterID: att.ClusterID,
		MachineID: att.MachineID,
		Action:    att.Action,
		JobID:     att.JobID,
		Params:    params,
		Outcome:   outcome,
	})
	return err
}

// List returns the records of the current day's file, newest first.
// Filtering and pagination land in plan 03.
func (l *Logger) List(_ context.Context) ([]Record, error) {
	path := l.CurrentFile()

	l.mu.Lock()
	defer l.mu.Unlock()

	recs, err := readFile(path)
	if err != nil {
		return nil, err
	}
	for i, j := 0, len(recs)-1; i < j; i, j = i+1, j-1 {
		recs[i], recs[j] = recs[j], recs[i]
	}
	return recs, nil
}

// Verify recomputes the chain over the current day's file. It reports the
// 1-based line number of the first record that does not verify.
//
// A chain nobody checks is theatre, so this runs at startup rather than behind
// a button (D-15). Plan 03 extends it across the most recently rotated file.
func (l *Logger) Verify(_ context.Context) (bool, int, error) {
	path := l.CurrentFile()

	l.mu.Lock()
	defer l.mu.Unlock()

	recs, err := readFile(path)
	if err != nil {
		return false, 0, err
	}

	prev := ""
	for i, rec := range recs {
		if rec.PrevHash != prev {
			return false, i + 1, nil
		}
		want, err := rec.ComputeHash()
		if err != nil {
			return false, i + 1, err
		}
		if want != rec.Hash {
			return false, i + 1, nil
		}
		prev = rec.Hash
	}
	return true, 0, nil
}

// readFile decodes every line of an audit file in write order.
func readFile(path string) ([]Record, error) {
	f, err := os.Open(path) //nolint:gosec // audit owns its own log directory
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var out []Record
	sc := bufio.NewScanner(f)
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
