package hostaction

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// The other half of the file root writes. TestHostScriptAsRoot proves the
// script writes what ReadResult accepts; this proves ReadResult accepts
// nothing else. Every refusal plants a marker in the file and asserts the
// error does not repeat it: whatever the reader says ends up on the host page
// and in the log, and a planted file must not be able to put text there.

const resultTime = "2026-09-28T09:37:00Z"

func TestReadResult(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 9, 28, 9, 37, 0, 0, time.UTC)

	tests := []struct {
		name    string
		file    *string // nil: the file does not exist
		dir     bool    // a directory at the path instead of a file
		want    Result
		wantErr string // substring naming the rule; "" means success
		noRes   bool   // the error must be ErrNoResult
		marker  string // planted in the file; the error must not contain it
	}{
		{name: "absent file", noRes: true},

		{
			name: "reboot started",
			file: ptr("0123456789abcdef reboot started " + resultTime + "\n"),
			want: Result{ID: "0123456789abcdef", Action: Reboot, Outcome: OutcomeStarted, At: at},
		},
		{
			name: "poweroff rejected (a valid order out of its window)",
			file: ptr("fedcba9876543210 poweroff rejected " + resultTime + "\n"),
			want: Result{ID: "fedcba9876543210", Action: Poweroff, Outcome: OutcomeRejected, At: at},
		},
		{
			name: "restart-service failed",
			file: ptr("00112233445566ff restart-service failed " + resultTime + "\n"),
			want: Result{ID: "00112233445566ff", Action: RestartService, Outcome: OutcomeFailed, At: at},
		},
		{
			name: "update started",
			file: ptr("a1b2c3d4e5f60718 update started " + resultTime + "\n"),
			want: Result{ID: "a1b2c3d4e5f60718", Action: Update, Outcome: OutcomeStarted, At: at},
		},
		{
			// 13-REVIEW-2 V-01: the helper records the end of a check, so that
			// the end of its wait is its own word, not the update status's.
			name: "check-update done",
			file: ptr("c0ffee00c0ffee11 check-update done " + resultTime + "\n"),
			want: Result{ID: "c0ffee00c0ffee11", Action: CheckUpdate, Outcome: OutcomeDone, At: at},
		},
		{
			name: "a malformed order: - - rejected",
			file: ptr("- - rejected " + resultTime + "\n"),
			want: Result{Outcome: OutcomeRejected, At: at},
		},
		{
			// The script writes this when rm cannot consume the order (a
			// directory by that name): it never read it as valid.
			name: "an order that could not be consumed: - - failed",
			file: ptr("- - failed " + resultTime + "\n"),
			want: Result{Outcome: OutcomeFailed, At: at},
		},
		{
			name: "a time with an offset comes back in UTC",
			file: ptr("0123456789abcdef update started 2026-09-28T11:37:00+02:00\n"),
			want: Result{ID: "0123456789abcdef", Action: Update, Outcome: OutcomeStarted, At: at},
		},

		{
			// A started order was valid, so it has both; "-" here would be
			// the script claiming to act on an order it could not name.
			name:    "- with started",
			file:    ptr("- - started 2026-09-28T09:37:01Z\n"),
			wantErr: "started or done order without an id",
			marker:  "2026-09-28T09:37:01Z",
		},
		{
			name:    "- with done",
			file:    ptr("- - done 2026-09-28T09:37:06Z\n"),
			wantErr: "started or done order without an id",
			marker:  "2026-09-28T09:37:06Z",
		},
		{
			// Only the check waits for its command; the script never writes
			// done for another order, so a file that says so is not its.
			name:    "a reboot done",
			file:    ptr("0123456789abcdef reboot done 2026-09-28T09:37:07Z\n"),
			wantErr: "done for an order other than check-update",
			marker:  "2026-09-28T09:37:07Z",
		},
		{
			name:    "an update done",
			file:    ptr("a1b2c3d4e5f60718 update done 2026-09-28T09:37:08Z\n"),
			wantErr: "done for an order other than check-update",
			marker:  "a1b2c3d4e5f60718",
		},
		{
			name:    "- as the id only",
			file:    ptr("- hkmhalt rejected " + resultTime + "\n"),
			wantErr: "only one of id and action",
			marker:  "hkmhalt",
		},
		{
			name:    "- as the action only",
			file:    ptr("0123456789abcdee - rejected " + resultTime + "\n"),
			wantErr: "only one of id and action",
			marker:  "0123456789abcdee",
		},
		{
			name:    "a 15-hex id",
			file:    ptr("0123456789abcde reboot started " + resultTime + "\n"),
			wantErr: "16 lowercase hex",
			marker:  "0123456789abcde",
		},
		{
			name:    "a 17-hex id",
			file:    ptr("0123456789abcdef0 reboot started " + resultTime + "\n"),
			wantErr: "16 lowercase hex",
			marker:  "0123456789abcdef0",
		},
		{
			name:    "an upper-case id",
			file:    ptr("0123456789ABCDEF reboot started " + resultTime + "\n"),
			wantErr: "16 lowercase hex",
			marker:  "0123456789ABCDEF",
		},
		{
			name:    "an id that is not hex",
			file:    ptr("hkm$(id)xxxxxxxx reboot started " + resultTime + "\n"),
			wantErr: "16 lowercase hex",
			marker:  "hkm$(id)",
		},
		{
			name:    "an unknown action",
			file:    ptr("0123456789abcdef hkmhalt started " + resultTime + "\n"),
			wantErr: "not one of the five host actions",
			marker:  "hkmhalt",
		},
		{
			name:    "an unknown outcome",
			file:    ptr("0123456789abcdef reboot hkmdone " + resultTime + "\n"),
			wantErr: "outcome",
			marker:  "hkmdone",
		},
		{
			name:    "a time that is not RFC 3339",
			file:    ptr("0123456789abcdef reboot started 2026-09-28_09:37:00hkm\n"),
			wantErr: "RFC 3339",
			marker:  "09:37:00hkm",
		},
		{
			name:    "a time with a carriage return",
			file:    ptr("0123456789abcdef reboot started 2026-09-28T09:37:02Z\r\n"),
			wantErr: "RFC 3339",
			marker:  "2026-09-28T09:37:02Z",
		},
		{
			name:    "three fields",
			file:    ptr("0123456789abcdef reboot 2026-09-28T09:37:03Z\n"),
			wantErr: "four fields",
			marker:  "2026-09-28T09:37:03Z",
		},
		{
			name:    "five fields",
			file:    ptr("0123456789abcdef reboot started " + resultTime + " hkmextra\n"),
			wantErr: "four fields",
			marker:  "hkmextra",
		},
		{
			name:    "a double space",
			file:    ptr("0123456789abcdef  reboot started 2026-09-28T09:37:04Z\n"),
			wantErr: "four fields",
			marker:  "2026-09-28T09:37:04Z",
		},
		{
			name:    "two lines",
			file:    ptr("0123456789abcdef reboot started " + resultTime + "\nhkmsecond\n"),
			wantErr: "exactly one line",
			marker:  "hkmsecond",
		},
		{
			name:    "no trailing newline",
			file:    ptr("0123456789abcdef reboot started 2026-09-28T09:37:05Z"),
			wantErr: "exactly one line",
			marker:  "2026-09-28T09:37:05Z",
		},
		{
			name:    "an empty file",
			file:    ptr(""),
			wantErr: "exactly one line",
		},
		{
			// Valid in shape, and too long only by its padding: the size
			// rule, not the parser, has to be what refuses it.
			name:    "above MaxResultSize",
			file:    ptr("0123456789abcdef reboot started " + resultTime + strings.Repeat(" hkmpad", 20) + "\n"),
			wantErr: "larger than",
			marker:  "hkmpad",
		},
		{
			name:    "a directory at the path",
			dir:     true,
			wantErr: "not a regular file",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fsys := fstest.MapFS{}
			switch {
			case tc.dir:
				fsys["state/last/x"] = &fstest.MapFile{Data: []byte("x"), Mode: 0o644}
			case tc.file != nil:
				fsys["state/last"] = &fstest.MapFile{Data: []byte(*tc.file), Mode: 0o644}
			}
			if tc.name == "above MaxResultSize" && len(*tc.file) <= MaxResultSize {
				t.Fatalf("the oversize row is %d bytes, not above %d", len(*tc.file), MaxResultSize)
			}

			got, err := ReadResult(fsys, "/state/last")

			switch {
			case tc.noRes:
				if !errors.Is(err, ErrNoResult) {
					t.Fatalf("err = %v, want ErrNoResult", err)
				}
			case tc.wantErr != "":
				if err == nil {
					t.Fatalf("accepted %+v, want a refusal naming %q", got, tc.wantErr)
				}
				if errors.Is(err, ErrNoResult) {
					t.Errorf("err = %v is ErrNoResult; a file that is there and wrong is not an absent one", err)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("err = %q, want it to name %q", err, tc.wantErr)
				}
				if tc.marker != "" && strings.Contains(err.Error(), tc.marker) {
					t.Errorf("err = %q quotes the file (%q)", err, tc.marker)
				}
				if got != (Result{}) {
					t.Errorf("a refusal returned %+v, want the zero Result", got)
				}
			default:
				if err != nil {
					t.Fatalf("err = %v, want success", err)
				}
				if got != tc.want {
					t.Errorf("got %+v, want %+v", got, tc.want)
				}
				if got.At.Location() != time.UTC {
					t.Errorf("at is in %v, want UTC", got.At.Location())
				}
			}
		})
	}
}

func TestReadResultPath(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{"srv/last": &fstest.MapFile{Data: []byte("- - rejected " + resultTime + "\n"), Mode: 0o644}}

	if _, err := ReadResult(fsys, "/srv/last"); err != nil {
		t.Errorf("an overridden absolute path: err = %v, want success", err)
	}
	// The doc says absolute, and the error says "clean absolute path": a
	// relative one would be read from wherever fsys happens to be rooted.
	for _, path := range []string{"srv/last", "/srv/../srv/last", "/srv/./last", "//srv/last", "/srv/last/"} {
		if _, err := ReadResult(fsys, path); err == nil || errors.Is(err, ErrNoResult) {
			t.Errorf("%q: err = %v, want a refusal that is not ErrNoResult", path, err)
		}
	}
	if _, err := ReadResult(fsys, "/srv"); err == nil || errors.Is(err, ErrNoResult) {
		t.Errorf("a directory: err = %v, want a refusal that is not ErrNoResult", err)
	}
}

func ptr(s string) *string { return &s }

// TestCheckRunning (13-REVIEW-2 WR-01, V-01, V-02): the helper is busy with a
// check while its last record is a check it started -- not done, not failed
// -- younger than its service's limit and not from before the last boot.
// Nobody else's word ends it: there is no update status in the question.
func TestCheckRunning(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 9, 30, 16, 0, 0, 0, time.UTC)
	check := Result{ID: "c0ffee00c0ffee11", Action: CheckUpdate, Outcome: OutcomeStarted, At: at}
	with := func(f func(r *Result)) Result { r := check; f(&r); return r }
	noBoot := time.Time{}
	bootedAt := at.Add(-time.Hour)

	cases := []struct {
		name string
		r    Result
		boot time.Time
		now  time.Time
		want bool
	}{
		{"just started", check, bootedAt, at, true},
		{"5 s in", check, bootedAt, at.Add(5 * time.Second), true},
		{"5 s in, the boot unknown", check, noBoot, at.Add(5 * time.Second), true},
		{"one second short of the limit", check, bootedAt, at.Add(HelperServiceLimit - time.Second), true},
		{"at the limit", check, bootedAt, at.Add(HelperServiceLimit), false},
		{"recorded in the future", check, bootedAt, at.Add(-time.Second), false},
		{"done", with(func(r *Result) { r.Outcome = OutcomeDone }), bootedAt, at.Add(5 * time.Second), false},
		{"failed", with(func(r *Result) { r.Outcome = OutcomeFailed }), bootedAt, at.Add(5 * time.Second), false},
		{"rejected", with(func(r *Result) { r.Outcome = OutcomeRejected }), bootedAt, at.Add(5 * time.Second), false},
		{"an update started", with(func(r *Result) { r.Action = Update }), bootedAt, at.Add(5 * time.Second), false},
		{"a reboot started", with(func(r *Result) { r.Action = Reboot }), bootedAt, at.Add(5 * time.Second), false},
		// V-02: a reboot mid-check ended whatever the helper waited for.
		{"the machine booted 40 s after it began", check, at.Add(40 * time.Second), at.Add(80 * time.Second), false},
		// The record rounds down to its second: written 0.4 s after a boot
		// that is not before it. A record whose second ended at the boot is.
		{"recorded in the second the machine booted", check, at.Add(400 * time.Millisecond), at.Add(5 * time.Second), true},
		{"recorded the second before the boot", check, at.Add(time.Second), at.Add(5 * time.Second), false},
	}
	for _, tc := range cases {
		if got := CheckRunning(tc.r, tc.boot, tc.now); got != tc.want {
			t.Errorf("%s: CheckRunning = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestCheckHeld: the helper was waiting for a check while an order lay in the
// slot when its last record is another order's check without an end (from this
// boot, and less than HelperServiceLimit before the placement), or one whose
// end came after the placement.
func TestCheckHeld(t *testing.T) {
	t.Parallel()

	placed := time.Date(2026, 9, 30, 16, 0, 5, 734e6, time.UTC)
	booted := placed.Add(-time.Hour)
	rec := func(id string, a Action, o Outcome, at time.Time) Result {
		return Result{ID: id, Action: a, Outcome: o, At: at}
	}
	const check, order = "c0ffee00c0ffee11", "0123456789abcdef"

	cases := []struct {
		name string
		r    Result
		boot time.Time
		want bool
	}{
		{"a check started before the placement, no end", rec(check, CheckUpdate, OutcomeStarted, placed.Add(-time.Minute)), booted, true},
		{"a check started in the future (the clock went back)", rec(check, CheckUpdate, OutcomeStarted, placed.Add(time.Hour)), booted, true},
		{"a check started, the boot unknown", rec(check, CheckUpdate, OutcomeStarted, placed.Add(-time.Minute)), time.Time{}, true},
		{"a check started before the last boot", rec(check, CheckUpdate, OutcomeStarted, booted.Add(-time.Minute)), booted, false},
		// 13-REVIEW-2 round 3, I1: systemd ends the helper HelperServiceLimit
		// after it began, whatever the check did; a started record older than
		// that held nothing, however long ago the boot was.
		{"a check started 6 h before the placement", rec(check, CheckUpdate, OutcomeStarted, placed.Add(-6*time.Hour)), placed.Add(-48 * time.Hour), false},
		{"a check started HelperServiceLimit before the placement", rec(check, CheckUpdate, OutcomeStarted, placed.Add(-HelperServiceLimit)), booted, false},
		{"a check started just under HelperServiceLimit before", rec(check, CheckUpdate, OutcomeStarted, placed.Add(-HelperServiceLimit+time.Second)), booted, true},
		{"an old started check, the boot unknown", rec(check, CheckUpdate, OutcomeStarted, placed.Add(-6*time.Hour)), time.Time{}, false},
		{"a check done after the placement", rec(check, CheckUpdate, OutcomeDone, placed.Add(3*time.Second)), booted, true},
		{"a check failed in the placement's second", rec(check, CheckUpdate, OutcomeFailed, placed.Truncate(time.Second)), booted, true},
		{"a check done the second before the placement", rec(check, CheckUpdate, OutcomeDone, placed.Truncate(time.Second).Add(-time.Second)), booted, false},
		{"this order's own record", rec(order, CheckUpdate, OutcomeStarted, placed.Add(time.Second)), booted, false},
		{"a check rejected", rec(check, CheckUpdate, OutcomeRejected, placed.Add(time.Second)), booted, false},
		{"an update started", rec(check, Update, OutcomeStarted, placed.Add(-time.Minute)), booted, false},
	}
	for _, tc := range cases {
		if got := CheckHeld(tc.r, order, placed, tc.boot); got != tc.want {
			t.Errorf("%s: CheckHeld = %v, want %v", tc.name, got, tc.want)
		}
	}
}
