package history

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"math"
	"path/filepath"
	"sort"
	"time"
)

// The file.
//
// # One file, rewritten whole, at most once every FlushEvery
//
// The daemon runs on a Raspberry Pi, and its data directory is an SD card or an
// NVMe drive: the first wears out under small frequent writes, and the operator
// decided the history may cost the card one write a minute and no more. So the
// whole history is one file, replaced at most once every FlushEvery (thirty minutes) through the store's own
// atomic write (fsstore.WriteFileAtomic: a temporary, fsync, rename, fsync of
// the directory) -- one file and not one per subject, because fifty-five
// renames a minute is fifty-five directory updates where one would do.
//
// A crash therefore loses at most the last minute and never produces half a
// file. Hourly updates restart the daemon; the shutdown path writes once more,
// so an update loses nothing at all.
//
// # The format
//
// A five-byte header -- "HKMH" and a version -- and then a gzip stream:
//
//	subjects  uvarint
//	per subject:
//	  key     string (uvarint length, bytes)
//	  seen    varint, unix milliseconds
//	  series  uvarint
//	  per series:
//	    name  string
//	    fine  ring
//	    coarse ring
//	ring: first slot varint, count uvarint, count x float32 (little-endian bits)
//
// A ring is written from its first held value to its last, gaps included as
// NaN: a gap is data, and a chart after a restart has to show the same break it
// showed before. Binary rather than JSON because a day of a homelab is a few
// hundred series of 1680 numbers, and as JSON that is ten megabytes a minute
// on the card; see the size test for what it is here.

const (
	// DirName is the history's directory under the data directory.
	DirName = "history"

	// FileName is the one file in it.
	FileName = "metrics.bin"

	// FlushEvery is the least time between two writes of the file.
	//
	// Thirty minutes, the operator's choice on 2026-09-26: the file is ~1.3 MB
	// and rewritten whole, so once a minute was up to 2 GB a day on the Pi's
	// card; this is ~65 MB. A clean stop still writes on the way out, so an
	// update loses nothing -- only a crash loses up to half an hour.
	FlushEvery = 30 * time.Minute

	fileVersion = 1

	// maxKey bounds a string read back from the file, so a corrupt length
	// cannot ask for a gigabyte.
	maxKey = 4096
)

var fileMagic = []byte("HKMH")

// errCorrupt is a file this version cannot read.
var errCorrupt = errors.New("history: the file is not a history this version can read")

// Open loads the history kept at path, and keeps writing it there.
//
// A missing file is an empty history. A file that does not decode is logged and
// set aside as an empty history too, never a refusal to start: the history is
// a convenience for charts, and a daemon that would not come up because its
// charts' past was unreadable would be trading the product for its decoration.
// Everything older than the retention is dropped on the way in, and so is every
// sample older than its tier reaches.
//
// read and write are fsstore.ReadFile and fsstore.WriteFileAtomic in the
// daemon: the data directory is reached through the store package and nowhere
// else, and this package is handed the two functions rather than opening
// files itself.
func Open(
	path string,
	read func(path string) ([]byte, error),
	write func(path string, data []byte) error,
	now time.Time,
	logger *slog.Logger,
) *Store {
	s := NewMemory()
	s.path = path
	s.write = write
	s.lastFlush = now

	raw, err := read(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return s
	case err != nil:
		logger.Warn("the metrics history could not be read; charts start empty",
			slog.String("file", path), slog.Any("error", err))
		return s
	}

	subjects, err := decode(raw, now)
	if err != nil {
		logger.Warn("the metrics history could not be decoded; charts start empty and the "+
			"file is replaced at the next write",
			slog.String("file", path), slog.Any("error", err))
		return s
	}
	s.subjects = subjects
	return s
}

// Writes is how many times the file has been written, for the test that holds
// the once-a-minute promise.
func (s *Store) Writes() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writes
}

// FlushIfDue writes the file if something changed and the last write is at
// least FlushEvery ago. It is what the sampler calls after every pass; the
// minute is kept here and not by the caller so that nothing else calling it
// can make the card write more often.
func (s *Store) FlushIfDue(now time.Time) error {
	s.mu.Lock()
	due := s.dirty && now.Sub(s.lastFlush) >= FlushEvery
	s.mu.Unlock()
	if !due {
		return nil
	}
	return s.Flush(now)
}

// Flush writes the file now, if there is one and anything changed. The
// shutdown path calls it directly: a restart for an update is a write that is
// due whatever the minute says.
func (s *Store) Flush(now time.Time) error {
	s.mu.Lock()
	if s.write == nil || !s.dirty {
		s.mu.Unlock()
		return nil
	}
	data, err := s.encodeLocked(now)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	s.dirty = false
	s.lastFlush = now
	s.writes++
	path, write := s.path, s.write
	s.mu.Unlock()

	if err := write(path, data); err != nil {
		// Dirty again, so the next due flush tries again rather than waiting
		// for a sample to change something.
		s.mu.Lock()
		s.dirty = true
		s.mu.Unlock()
		return fmt.Errorf("history: write %s: %w", path, err)
	}
	return nil
}

// Encode is the file's bytes for the history as it is now. It is exported for
// the size measurement; Flush is what writes them.
func (s *Store) Encode(now time.Time) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.encodeLocked(now)
}

func (s *Store) encodeLocked(now time.Time) ([]byte, error) {
	var body []byte
	keys := make([]string, 0, len(s.subjects))
	for k := range s.subjects {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	nowFine := floorDiv(now.UnixMilli(), FineStep.Milliseconds())
	nowCoarse := floorDiv(now.UnixMilli(), CoarseStep.Milliseconds())

	body = binary.AppendUvarint(body, uint64(len(keys)))
	for _, k := range keys {
		sub := s.subjects[k]
		body = appendString(body, k)
		body = binary.AppendVarint(body, sub.seen.UnixMilli())

		names := make([]string, 0, len(sub.series))
		for n := range sub.series {
			names = append(names, n)
		}
		sort.Strings(names)
		body = binary.AppendUvarint(body, uint64(len(names)))
		for _, n := range names {
			ser := sub.series[n]
			body = appendString(body, n)
			body = appendRing(body, ser.fine, nowFine)
			body = appendRing(body, ser.coarse, nowCoarse)
		}
	}

	var out bytes.Buffer
	out.Write(fileMagic)
	out.WriteByte(fileVersion)
	// BestSpeed: the Pi does this every thirty minutes, and the gain from harder
	// compression on a stream of float32s is a few percent.
	zw, err := gzip.NewWriterLevel(&out, gzip.BestSpeed)
	if err != nil {
		return nil, err
	}
	if _, err := zw.Write(body); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func appendString(b []byte, s string) []byte {
	b = binary.AppendUvarint(b, uint64(len(s)))
	return append(b, s...)
}

// appendRing writes the part of a ring that is still inside its window as of
// now, from the first value held to the last.
func appendRing(b []byte, r *ring, nowSlot int64) []byte {
	n := int64(len(r.vals))
	lo := max(r.head-n+1, nowSlot-n+1)
	hi := min(r.head, nowSlot)
	for lo <= hi && math.IsNaN(float64(r.at(lo))) {
		lo++
	}
	for hi >= lo && math.IsNaN(float64(r.at(hi))) {
		hi--
	}
	if lo > hi {
		b = binary.AppendVarint(b, 0)
		return binary.AppendUvarint(b, 0)
	}
	b = binary.AppendVarint(b, lo)
	b = binary.AppendUvarint(b, uint64(hi-lo+1)) //nolint:gosec // hi >= lo here, so the count is positive
	for slot := lo; slot <= hi; slot++ {
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(r.at(slot)))
	}
	return b
}

// decode reads a file back, dropping what has aged out as of now.
func decode(raw []byte, now time.Time) (map[string]*subject, error) {
	if len(raw) < len(fileMagic)+1 || !bytes.Equal(raw[:len(fileMagic)], fileMagic) {
		return nil, errCorrupt
	}
	if v := raw[len(fileMagic)]; v != fileVersion {
		return nil, fmt.Errorf("%w: version %d", errCorrupt, v)
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw[len(fileMagic)+1:]))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errCorrupt, err)
	}
	body, err := io.ReadAll(zr)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errCorrupt, err)
	}

	d := &decoder{b: body}
	nowFine := floorDiv(now.UnixMilli(), FineStep.Milliseconds())
	nowCoarse := floorDiv(now.UnixMilli(), CoarseStep.Milliseconds())

	out := map[string]*subject{}
	subjects := d.uvarint()
	for range min(subjects, uint64(len(body))) {
		key := d.string()
		seen := time.UnixMilli(d.varint())
		sub := &subject{seen: seen, series: map[string]*series{}}

		nseries := d.uvarint()
		for range min(nseries, uint64(len(body))) {
			name := d.string()
			ser := newSeries()
			fineHeld := d.ring(ser.fine, nowFine)
			coarseHeld := d.ring(ser.coarse, nowCoarse)
			if d.err != nil {
				return nil, d.err
			}
			if fineHeld || coarseHeld {
				sub.series[name] = ser
			}
		}
		if d.err != nil {
			return nil, d.err
		}
		// A subject whose every sample has aged out of both tiers is gone.
		// That is the retention, and not a second test of seen beside it: the
		// last sample is at seen, so "nothing left inside a day" and "not heard
		// from for a day" are the same condition, and two spellings of one
		// condition are one that a fault can hide behind.
		if len(sub.series) == 0 {
			continue
		}
		out[key] = sub
	}
	if d.err != nil {
		return nil, d.err
	}
	return out, nil
}

// decoder reads the body, remembering the first error so the loops above can
// be written straight through.
type decoder struct {
	b   []byte
	err error
}

func (d *decoder) fail() {
	if d.err == nil {
		d.err = fmt.Errorf("%w: truncated", errCorrupt)
	}
	d.b = nil
}

func (d *decoder) uvarint() uint64 {
	v, n := binary.Uvarint(d.b)
	if n <= 0 {
		d.fail()
		return 0
	}
	d.b = d.b[n:]
	return v
}

func (d *decoder) varint() int64 {
	v, n := binary.Varint(d.b)
	if n <= 0 {
		d.fail()
		return 0
	}
	d.b = d.b[n:]
	return v
}

func (d *decoder) string() string {
	n := d.uvarint()
	if n > maxKey || n > uint64(len(d.b)) {
		d.fail()
		return ""
	}
	s := string(d.b[:n])
	d.b = d.b[n:]
	return s
}

// ring reads one ring into r, keeping only the slots still inside its window
// as of nowSlot, and reports whether any value was kept.
func (d *decoder) ring(r *ring, nowSlot int64) bool {
	first := d.varint()
	count := d.uvarint()
	if count > uint64(len(d.b)/4) {
		d.fail()
		return false
	}
	oldest := nowSlot - int64(len(r.vals)) + 1
	held := false
	for i := range int64(count) { //nolint:gosec // count is bounded by len(d.b)/4 above
		v := math.Float32frombits(binary.LittleEndian.Uint32(d.b[4*i:]))
		slot := first + i
		if slot < oldest || slot > nowSlot || math.IsNaN(float64(v)) {
			continue
		}
		r.put(slot, v)
		held = true
	}
	d.b = d.b[4*count:]
	return held
}

// Path is where the history of a data directory is kept.
func Path(dataDir string) string {
	return filepath.Join(dataDir, DirName, FileName)
}
