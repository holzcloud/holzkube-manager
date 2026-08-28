// Package audit is holzkube's append-only JSONL log with a hash chain.
//
// It is called by middleware; it never reads an HTTP request. The format is
// one record per line -- no indentation, ever. A record that spans several
// lines has no line number to report, and reporting the line of the first
// break is how a finding here becomes actionable.
//
// Responsibilities are split across four files: record.go owns the closed field
// set and the canonical form, chain.go the anchor and verification, rotate.go
// the daily file boundary and compression, query.go the read path. This file
// owns the writer.
package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

	// now is the clock. It is a field so a test can turn the day over without
	// waiting for midnight; production always gets UTC wall time.
	now func() time.Time

	// pending links an outcome back to the intent that opened it. The record
	// format is closed, so the outcome repeats the intent's identifying fields
	// rather than carrying a reference field that would change the format.
	pending map[uint64]Record

	// lastVerify memoises the most recent verification verdict. Verify
	// re-reads and re-hashes the whole window under l.mu, so without a floor
	// on how often that happens a polling UI turns every open tab into a
	// repeated full re-read that audit writes queue behind. A zero at means
	// nothing has been verified yet.
	lastVerify verdict

	closed bool
}

// verdict is a remembered verification result.
type verdict struct {
	ok   bool
	file string
	line int
	at   time.Time
}

// verifyCacheTTL is the floor between two live re-verifications. It is short
// enough that damage done while the process runs surfaces within a poll or
// two, and long enough that the cost does not scale with the number of
// clients asking.
const verifyCacheTTL = 30 * time.Second

// Open prepares <dir>/audit and returns a Logger positioned after the last
// record already on disk.
func Open(dir string) (*Logger, error) {
	return open(dir, func() time.Time { return time.Now().UTC() })
}

func open(dir string, now func() time.Time) (*Logger, error) {
	if dir == "" {
		return nil, errors.New("audit: empty data directory")
	}
	logDir := filepath.Join(dir, "audit")
	if err := os.MkdirAll(logDir, dirPerm); err != nil {
		return nil, fmt.Errorf("audit: create log directory: %w", err)
	}

	l := &Logger{
		dir:      logDir,
		now:      now,
		lastHash: Genesis,
		pending:  make(map[uint64]Record),
	}
	if err := l.resume(); err != nil {
		return nil, err
	}
	if err := l.openDay(l.now()); err != nil {
		return nil, err
	}
	return l, nil
}

// resume recovers the sequence number and the tail of the chain, so a restart
// continues the chain instead of forking it.
//
// It walks backwards past empty files: a day on which the process started and
// wrote nothing leaves an empty file behind, and stopping at it would reset the
// chain to the anchor and fork the archive at that seam.
func (l *Logger) resume() error {
	files, err := allFiles(l.dir)
	if err != nil {
		return err
	}
	for i := len(files) - 1; i >= 0; i-- {
		recs, err := readFile(files[i])
		if err != nil {
			return err
		}
		if len(recs) == 0 {
			continue
		}
		last := recs[len(recs)-1]
		l.seq = last.Seq
		l.lastHash = last.Hash
		return nil
	}
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
	now := l.now()
	if err := l.openDay(now); err != nil {
		return Record{}, err
	}

	rec.Seq = l.seq + 1
	rec.TS = now
	rec.PrevHash = l.lastHash
	// Shortening here rather than at the call site is the point: no caller can
	// forget, and the log therefore cannot hold a usable session token.
	rec.Session = ShortSession(rec.Session)
	if rec.Params == nil {
		rec.Params = map[string]any{}
	}

	hash, err := ComputeHash(rec.PrevHash, rec)
	if err != nil {
		return Record{}, fmt.Errorf("audit: hash record: %w", err)
	}
	rec.Hash = hash

	// The written line is JSONL: exactly one record, no indentation, and no
	// HTML escaping -- an operator greps this file, and "\u003credacted\u003e"
	// helps nobody. The written form is free to differ from the canonical form
	// the hash was taken over, because verification decodes the line back into
	// a record and re-canonicalizes it rather than comparing bytes.
	var line bytes.Buffer
	enc := json.NewEncoder(&line)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(rec); err != nil {
		return Record{}, fmt.Errorf("audit: encode record: %w", err)
	}

	if _, err := l.file.Write(line.Bytes()); err != nil {
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
// attempt, and repeating them would double the surface redaction has to cover.
//
// There is no path that completes an attempt after the fact. An attempt with no
// outcome means the process did not survive the action, which is a finding and
// is left standing as one.
func (l *Logger) Outcome(_ context.Context, seq uint64, outcome string, cause error) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	att, ok := l.pending[seq]
	if !ok {
		return fmt.Errorf("audit: no pending attempt with seq %d", seq)
	}
	delete(l.pending, seq)

	// Through the redactor, like every other value that reaches the archive.
	// This used to be the one branch in the package that wrote a string
	// straight into a record, safe only because the single caller happened to
	// pass a shape-checked taxonomy code -- a validation living in a different
	// package from the invariant it upheld.
	params := map[string]any{}
	if cause != nil {
		params["error"] = OutcomeCause(cause.Error())
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

// Verify recomputes the chain over the current day's file and the one rotated
// before it, and reports the file and 1-based line of the first break (D-15).
//
// Two files rather than the whole archive is the deliberate scope: it is what
// can be checked at every start without the cost growing with the archive, and
// it is enough to catch damage done since yesterday. Verifying further back is
// what the file list of Verify is for.
//
// A chain nobody checks is theatre, so this runs at startup rather than behind
// a button. Nothing here repairs what it finds.
func (l *Logger) Verify(ctx context.Context) (ok bool, file string, line int, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.verifyLocked(ctx)
}

// CachedVerify is Verify for the request path: it reuses a verdict newer than
// verifyCacheTTL instead of re-reading the archive.
//
// Verification is the one operation here whose cost grows with the archive,
// and it runs under the same mutex append takes, so an uncached re-verify per
// request lets callers serialise audit writes against a full re-read. The
// audit middleware is fail-closed, so a mutation whose intent cannot be
// recorded is refused — which makes that a way to fail other people's writes,
// not merely a slow endpoint.
//
// The cache is deliberately not invalidated by append. A verdict at most
// verifyCacheTTL old is what this is for; a caller that needs the current
// state of the file on disk wants Verify.
func (l *Logger) CachedVerify(ctx context.Context) (ok bool, file string, line int, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if v := l.lastVerify; !v.at.IsZero() && l.now().Sub(v.at) < verifyCacheTTL {
		return v.ok, v.file, v.line, nil
	}

	ok, file, line, err = l.verifyLocked(ctx)
	if err != nil {
		// Nothing is cached on error: a failed read is not a verdict, and
		// remembering it would suppress the retry that resolves it.
		return false, "", 0, err
	}

	l.lastVerify = verdict{ok: ok, file: file, line: line, at: l.now()}
	return ok, file, line, nil
}

// verifyLocked runs the verification window. The caller holds l.mu, which is
// what keeps a page from being assembled out of a half-written line -- the
// same reason Logger.Query takes it.
func (l *Logger) verifyLocked(ctx context.Context) (ok bool, file string, line int, err error) {
	files, err := allFiles(l.dir)
	if err != nil {
		return false, "", 0, err
	}
	if len(files) > 2 {
		files = files[len(files)-2:]
	}
	return VerifyContext(ctx, files)
}
