package history

import (
	"io"
	"log/slog"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
)

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// historyPath is a history file in a data directory as tight as a real one.
func historyPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return Path(dir)
}

func allRanges(s *Store, key string, now time.Time) map[Range]View {
	return map[Range]View{
		Range1h:  s.Query(key, Range1h, now),
		Range6h:  s.Query(key, Range6h, now),
		Range24h: s.Query(key, Range24h, now),
	}
}

// TestTheHistorySurvivesARestart is the file round trip: what a chart showed
// before the daemon stopped is what it shows after, gaps included, in every
// range.
//
// Fault injected and seen red: encodeLocked writing the coarse ring against
// the fine tier's slot (appendRing(body, ser.coarse, nowFine)), which kept
// nothing of the minute tier -- the six-hour and day charts came back empty.
func TestTheHistorySurvivesARestart(t *testing.T) {
	path := historyPath(t)
	now := t0.Add(3 * time.Hour)

	before := Open(path, fsstore.ReadFile, fsstore.WriteFileAtomic, t0, quiet())
	for at := t0; !at.After(now); at = at.Add(FineStep) {
		// A gap from the first half hour of the second hour: a node that
		// was off. It has to come back as a gap, not as zeroes.
		if at.After(t0.Add(time.Hour)) && at.Before(t0.Add(90*time.Minute)) {
			continue
		}
		sec := float64(at.Sub(t0) / time.Second)
		before.Record("machine/a", at, map[string]float64{"cpu": sec / 100, "temp:k10temp/Tctl": 40 + sec/1000})
		before.Record("app/c1/default/Deployment/web", at, map[string]float64{"cpu": 3, "memory": 1 << 27})
	}
	if err := before.Flush(now); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	after := Open(path, fsstore.ReadFile, fsstore.WriteFileAtomic, now, quiet())
	for _, key := range []string{"machine/a", "app/c1/default/Deployment/web"} {
		want, got := allRanges(before, key, now), allRanges(after, key, now)
		for r := range want {
			if len(want[r].Series) == 0 {
				t.Fatalf("%s %s: nothing to compare; the test recorded nothing", key, r)
			}
			if !reflect.DeepEqual(got[r], want[r]) {
				t.Errorf("%s %s after a restart differs from before it:\n got %d series, %v\nwant %d series, %v",
					key, r, len(got[r].Series), summary(got[r]), len(want[r].Series), summary(want[r]))
			}
		}
	}
}

func summary(v View) map[string]int {
	out := map[string]int{}
	for k, p := range v.Series {
		out[k] = len(p)
	}
	return out
}

// TestWhatIsOlderThanADayIsDroppedOnLoad: a daemon that was stopped for longer
// than the retention comes back with nothing for a subject not heard from in a
// day, and keeps what is still inside it.
//
// Fault injected and seen red: the decoder's window check removed
// (slot < oldest no longer skipped), which kept a subject last heard from 25
// hours before the load.
func TestWhatIsOlderThanADayIsDroppedOnLoad(t *testing.T) {
	path := historyPath(t)

	s := Open(path, fsstore.ReadFile, fsstore.WriteFileAtomic, t0, quiet())
	s.Record("machine/old", t0, map[string]float64{"cpu": 1})
	s.Record("machine/recent", t0.Add(2*time.Hour), map[string]float64{"cpu": 2})
	if err := s.Flush(t0.Add(2 * time.Hour)); err != nil {
		t.Fatal(err)
	}

	later := t0.Add(25 * time.Hour)
	loaded := Open(path, fsstore.ReadFile, fsstore.WriteFileAtomic, later, quiet())
	if loaded.Has("machine/old") {
		t.Error("a subject last heard from 25 hours before the load was kept")
	}
	if !loaded.Has("machine/recent") {
		t.Fatal("a subject heard from 23 hours before the load was dropped")
	}
	if got := loaded.Query("machine/recent", Range24h, later).Series["cpu"]; len(got) != 1 {
		t.Errorf("the recent subject's day is %v, want its one minute", got)
	}
}

// TestAnUnreadableFileIsAnEmptyHistory: the history is not the record, and a
// daemon that would not start over its charts' past would be trading the
// product for its decoration.
func TestAnUnreadableFileIsAnEmptyHistory(t *testing.T) {
	path := historyPath(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, raw := range [][]byte{
		[]byte("not a history"),
		append([]byte("HKMH\x01"), "not gzip"...),
		append([]byte("HKMH\x09"), 0),
	} {
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		s := Open(path, fsstore.ReadFile, fsstore.WriteFileAtomic, t0, quiet())
		if len(s.Subjects()) != 0 {
			t.Errorf("%q loaded as %v", raw, s.Subjects())
		}
	}
}

// TestTheFileIsWrittenAtMostOnceAMinute counts the writes the sampler's loop
// makes over five minutes of passes: five, one a minute, where a write per
// pass would be twenty.
//
// Fault injected and seen red: FlushIfDue ignoring the minute
// (due := s.dirty), which wrote after every pass -- 20 writes.
func TestTheFileIsWrittenAtMostOnceAMinute(t *testing.T) {
	writes := 0
	s := Open(historyPath(t), fsstore.ReadFile, func(string, []byte) error { writes++; return nil }, t0, quiet())

	for at := t0.Add(FineStep); !at.After(t0.Add(5 * time.Minute)); at = at.Add(FineStep) {
		s.Record("machine/a", at, map[string]float64{"cpu": 1})
		if err := s.FlushIfDue(at); err != nil {
			t.Fatal(err)
		}
	}
	if writes != 5 || s.Writes() != 5 {
		t.Errorf("five minutes of passes wrote the file %d times (store counts %d), want 5", writes, s.Writes())
	}

	// Nothing changed, nothing written, however long it has been.
	if err := s.FlushIfDue(t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if writes != 5 {
		t.Errorf("an unchanged history was written again: %d writes", writes)
	}
}

// TestTheFileSizeOfAFullDay measures the file for the homelab the operator
// sized it for: five nodes of thirty series and fifty apps of two, every
// fifteen seconds for a whole day, with values that move the way gauges do --
// noisy, so the compressor gets no help it would not get from real readings.
//
// The budget is the operator's ~40 MB. The measured number is logged, because
// the number and not the pass is what the commit reports.
func TestTheFileSizeOfAFullDay(t *testing.T) {
	if testing.Short() {
		t.Skip("records a whole day")
	}
	s := NewMemory()
	rng := rand.New(rand.NewPCG(1, 2))
	end := t0.Add(24 * time.Hour)

	for at := t0.Add(FineStep); !at.After(end); at = at.Add(FineStep) {
		for n := range 5 {
			values := map[string]float64{}
			for i := range 30 {
				values["series:"+strconv.Itoa(i)] = 20 + 60*rng.Float64()
			}
			s.Record("machine/node-"+strconv.Itoa(n), at, values)
		}
		for a := range 50 {
			s.Record("app/c1/default/Deployment/app-"+strconv.Itoa(a), at, map[string]float64{
				"cpu":    float64(rng.IntN(2000)),
				"memory": float64(50<<20 + rng.IntN(500<<20)),
			})
		}
	}

	raw, err := s.Encode(end)
	if err != nil {
		t.Fatal(err)
	}
	const budget = 40 << 20
	t.Logf("a full day of 5 nodes x 30 series + 50 apps x 2 series (250 series, %d fine + %d coarse slots each): %d bytes (%.2f MiB)",
		FineSlots, CoarseSlots, len(raw), float64(len(raw))/(1<<20))
	if len(raw) > budget {
		t.Errorf("the file is %d bytes, over the %d the operator sized it for", len(raw), budget)
	}
}
