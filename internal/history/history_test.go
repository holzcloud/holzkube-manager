package history

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

// t0 is minute-aligned, so a test can say "the minute" without arithmetic.
var t0 = time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)

func ms(t time.Time) float64 { return float64(t.UnixMilli()) }

// TestAMinuteIsTheAverageOfItsFourSamples is the rollup: the one-minute tier
// holds the mean of the fifteen-second samples in that minute, not the last of
// them and not their sum.
//
// Fault injected and seen red: Record put the sample itself into the coarse
// ring (ser.coarse.put(minute, float32(v))) instead of the minute's average --
// the minute then read 40, the last sample.
func TestAMinuteIsTheAverageOfItsFourSamples(t *testing.T) {
	s := NewMemory()
	for i, v := range []float64{10, 20, 30, 40} {
		s.Record("machine/a", t0.Add(time.Duration(i)*FineStep), map[string]float64{"cpu": v})
	}

	now := t0.Add(59 * time.Second)
	fine := s.Query("machine/a", Range1h, now).Series["cpu"]
	if len(fine) != 4 {
		t.Fatalf("the hour has %d points, want the 4 samples: %v", len(fine), fine)
	}

	coarse := s.Query("machine/a", Range6h, now).Series["cpu"]
	if len(coarse) != 1 {
		t.Fatalf("six hours have %d points, want the one minute: %v", len(coarse), coarse)
	}
	if coarse[0] != (Point{ms(t0), 25}) {
		t.Errorf("the minute is %v, want [%v 25]: the average of 10, 20, 30 and 40", coarse[0], ms(t0))
	}

	// A minute with a missing sample averages what it has. A node that missed
	// one reading of four was not at zero for a quarter of the minute.
	s.Record("machine/b", t0, map[string]float64{"cpu": 10})
	s.Record("machine/b", t0.Add(30*time.Second), map[string]float64{"cpu": 30})
	if got := s.Query("machine/b", Range6h, now).Series["cpu"]; len(got) != 1 || got[0][1] != 20 {
		t.Errorf("a minute with two of four samples is %v, want 20", got)
	}
}

// TestTheRingsForgetAfterAnHourAndADay is the eviction at both lengths.
//
// Two ways to get it wrong, both held: keeping a 241st point in the hour, and
// -- the one a ring over absolute slots invites -- a slot that the ring moved
// over without writing still holding what it held one lap ago. After an hour
// of silence that is a reading from an hour ago drawn as if it were now.
//
// Faults injected and seen red: the clearing loop in ring.put removed, which
// drew the phantom in both tiers. The 240/1440 counts are held twice -- by
// Query's window and by ring.at's own bound -- and each of those injected alone
// stayed green, because the other held; both together (first := last -
// int64(slots) and slot < r.head-n) went red with 241 and 1441 points. That is
// stated rather than hidden: a single regression there is not caught by this
// test, and does not reach a chart either.
func TestTheRingsForgetAfterAnHourAndADay(t *testing.T) {
	t.Run("an hour of samples is 240 points, the oldest gone", func(t *testing.T) {
		s := NewMemory()
		for i := range FineSlots + 1 {
			s.Record("machine/a", t0.Add(time.Duration(i)*FineStep), map[string]float64{"cpu": float64(i)})
		}
		now := t0.Add(time.Duration(FineSlots) * FineStep)
		got := s.Query("machine/a", Range1h, now).Series["cpu"]
		if len(got) != FineSlots {
			t.Fatalf("the hour has %d points, want %d", len(got), FineSlots)
		}
		if got[0][0] != ms(t0.Add(FineStep)) || got[len(got)-1][0] != ms(now) {
			t.Errorf("the hour runs %v to %v, want %v to %v",
				got[0][0], got[len(got)-1][0], ms(t0.Add(FineStep)), ms(now))
		}
	})

	t.Run("after an hour of silence the fine tier holds no phantom", func(t *testing.T) {
		s := NewMemory()
		s.Record("machine/a", t0, map[string]float64{"cpu": 99})
		later := t0.Add(time.Duration(FineSlots+1) * FineStep)
		s.Record("machine/a", later, map[string]float64{"cpu": 1})

		got := s.Query("machine/a", Range1h, later).Series["cpu"]
		if len(got) != 1 || got[0] != (Point{ms(later), 1}) {
			t.Errorf("the hour after a silence is %v, want only [%v 1]; the 99 is an hour old", got, ms(later))
		}
	})

	t.Run("after a day of silence the coarse tier holds no phantom", func(t *testing.T) {
		s := NewMemory()
		s.Record("machine/a", t0, map[string]float64{"cpu": 99})
		later := t0.Add(time.Duration(CoarseSlots+1) * CoarseStep)
		s.Record("machine/a", later, map[string]float64{"cpu": 1})

		got := s.Query("machine/a", Range24h, later).Series["cpu"]
		if len(got) != 1 || got[0] != (Point{ms(later), 1}) {
			t.Errorf("the day after a silence is %v, want only [%v 1]; the 99 is a day old", got, ms(later))
		}
	})

	t.Run("a day of minutes is 1440 points", func(t *testing.T) {
		s := NewMemory()
		for i := range CoarseSlots + 1 {
			s.Record("machine/a", t0.Add(time.Duration(i)*CoarseStep), map[string]float64{"cpu": 1})
		}
		now := t0.Add(time.Duration(CoarseSlots) * CoarseStep)
		if got := s.Query("machine/a", Range24h, now).Series["cpu"]; len(got) != CoarseSlots {
			t.Errorf("the day has %d points, want %d", len(got), CoarseSlots)
		}
	})
}

// TestTheRangeChoosesTheTier is the contract's range table: 1h is the fifteen-
// second tier, 6h and 24h the minute tier, 6h the last 360 minutes of it.
//
// Fault injected and seen red: tier() answering 6h from the fine ring -- the
// step read 15 and the samples older than an hour were missing.
func TestTheRangeChoosesTheTier(t *testing.T) {
	s := NewMemory()
	now := t0.Add(24 * time.Hour)
	// One sample every minute for the whole day, value = minutes before now,
	// oldest first as a sampler would write them.
	for m := CoarseSlots - 1; m >= 0; m-- {
		s.Record("machine/a", now.Add(-time.Duration(m)*time.Minute), map[string]float64{"cpu": float64(m)})
	}

	for _, tc := range []struct {
		r      Range
		step   int
		points int
		oldest float64 // minutes before now
		span   time.Duration
	}{
		{Range1h, 15, 60, 59, time.Hour},
		{Range6h, 60, 360, 359, 6 * time.Hour},
		{Range24h, 60, 1440, 1439, 24 * time.Hour},
	} {
		v := s.Query("machine/a", tc.r, now)
		if v.Range != tc.r || v.StepSeconds != tc.step {
			t.Errorf("%s: range %q step %d, want %q and %d", tc.r, v.Range, v.StepSeconds, tc.r, tc.step)
		}
		got := v.Series["cpu"]
		if len(got) != tc.points {
			t.Errorf("%s: %d points, want %d", tc.r, len(got), tc.points)
			continue
		}
		if got[0][1] != tc.oldest {
			t.Errorf("%s: the oldest point is %v minutes old, want %v", tc.r, got[0][1], tc.oldest)
		}
		for i := 1; i < len(got); i++ {
			if got[i][0] <= got[i-1][0] {
				t.Fatalf("%s: points are not ascending at %d: %v then %v", tc.r, i, got[i-1], got[i])
			}
		}
		if v.To != now.Format(time.RFC3339) || v.From != now.Add(-tc.span).Format(time.RFC3339) {
			t.Errorf("%s: from %s to %s, want %s to %s", tc.r, v.From, v.To,
				now.Add(-tc.span).Format(time.RFC3339), now.Format(time.RFC3339))
		}
	}

	for in, want := range map[string]Range{"": Range1h, "1h": Range1h, "6h": Range6h, "24h": Range24h} {
		if got, ok := ParseRange(in); !ok || got != want {
			t.Errorf("ParseRange(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, bad := range []string{"2h", "1H", "60m", "all"} {
		if _, ok := ParseRange(bad); ok {
			t.Errorf("ParseRange(%q) accepted a range the contract does not name", bad)
		}
	}
}

// TestTheSeriesAreNeverNull holds the contract's "never null" for the case it
// is easiest to break in: nothing recorded at all.
//
// Fault injected and seen red: Query building the view with a nil Series map,
// which encodes as "series": null.
func TestTheSeriesAreNeverNull(t *testing.T) {
	s := NewMemory()
	s.Record("machine/a", t0, map[string]float64{"cpu": 1})

	for _, key := range []string{"machine/nobody", "machine/a"} {
		// For machine/a, a day later: it has a subject and nothing in range.
		raw, err := json.Marshal(s.Query(key, Range1h, t0.Add(25*time.Hour)))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), `"series":{}`) {
			t.Errorf("%s with no history encodes as %s; want \"series\":{}", key, raw)
		}
	}
}

// TestAValueThatIsNotANumberIsNotRecorded: NaN is how a gap is stored, so a NaN
// sample would be a gap that looks recorded, and an infinity is a bug upstream.
func TestAValueThatIsNotANumberIsNotRecorded(t *testing.T) {
	s := NewMemory()
	s.Record("machine/a", t0, map[string]float64{"cpu": math.NaN(), "memory": math.Inf(1), "rx": 5})
	got := s.Query("machine/a", Range1h, t0).Series
	if len(got) != 1 || len(got["rx"]) != 1 {
		t.Errorf("series = %v, want only rx", got)
	}
}

// TestAFloat32IsServedAsItsShortestDecimal: 18.2 stored as float32 is served as
// 18.2, not as the 18.200000762939453 widening it gives.
func TestAFloat32IsServedAsItsShortestDecimal(t *testing.T) {
	s := NewMemory()
	s.Record("machine/a", t0, map[string]float64{"cpu": 18.2})
	raw, err := json.Marshal(s.Query("machine/a", Range1h, t0))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `,18.2]`) {
		t.Errorf("encoded as %s, want the value as 18.2", raw)
	}
}
