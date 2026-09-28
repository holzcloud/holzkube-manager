package updatestatus

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func ptr(s string) *string { return &s }

// TestReadStatus is the reader's whole contract: a file the script wrote comes
// back as a Status, an absent file as ErrNotRecorded, and everything else as a
// refusal that names what was wrong. Nothing here may come back with a time or
// a version the file did not carry (D-16).
func TestReadStatus(t *testing.T) {
	t.Parallel()

	checked := time.Date(2026, 9, 28, 9, 37, 0, 0, time.UTC)

	tests := []struct {
		name    string
		file    *string // nil: the file does not exist
		want    Status
		wantErr string // substring; "" means success
		notRec  bool   // the error must be ErrNotRecorded
	}{
		{
			name:   "absent file is not recorded",
			file:   nil,
			notRec: true,
		},
		{
			name: "available with both versions",
			file: ptr(`{"checked_at":"2026-09-28T09:37:00Z","installed":"0.1.0","latest":"0.2.0","outcome":"available"}`),
			want: Status{CheckedAt: checked, Installed: ptr("0.1.0"), Latest: ptr("0.2.0"), Outcome: OutcomeAvailable},
		},
		{
			name: "checked_at with an offset comes back in UTC",
			file: ptr(`{"checked_at":"2026-09-28T11:37:00+02:00","installed":"0.2.0","latest":"0.2.0","outcome":"current"}`),
			want: Status{CheckedAt: checked, Installed: ptr("0.2.0"), Latest: ptr("0.2.0"), Outcome: OutcomeCurrent},
		},
		{
			name: "leading v on a version is accepted",
			file: ptr(`{"checked_at":"2026-09-28T09:37:00Z","installed":"v0.1.0","latest":"v0.2.0","outcome":"updated"}`),
			want: Status{CheckedAt: checked, Installed: ptr("v0.1.0"), Latest: ptr("v0.2.0"), Outcome: OutcomeUpdated},
		},
		{
			name: "rolled-back is an outcome",
			file: ptr(`{"checked_at":"2026-09-28T09:37:00Z","installed":"0.1.0","latest":"0.2.0","outcome":"rolled-back"}`),
			want: Status{CheckedAt: checked, Installed: ptr("0.1.0"), Latest: ptr("0.2.0"), Outcome: OutcomeRolledBack},
		},
		{
			name: "failed with latest null",
			file: ptr(`{"checked_at":"2026-09-28T09:37:00Z","installed":"0.1.0","latest":null,"outcome":"failed"}`),
			want: Status{CheckedAt: checked, Installed: ptr("0.1.0"), Latest: nil, Outcome: OutcomeFailed},
		},
		{
			name: "failed with installed null",
			file: ptr(`{"checked_at":"2026-09-28T09:37:00Z","installed":null,"latest":"0.2.0","outcome":"failed"}`),
			want: Status{CheckedAt: checked, Installed: nil, Latest: ptr("0.2.0"), Outcome: OutcomeFailed},
		},
		{
			name: "unknown extra keys are ignored",
			file: ptr(`{"checked_at":"2026-09-28T09:37:00Z","installed":"0.1.0","latest":"0.2.0","outcome":"available","duration_seconds":12}`),
			want: Status{CheckedAt: checked, Installed: ptr("0.1.0"), Latest: ptr("0.2.0"), Outcome: OutcomeAvailable},
		},
		{
			name:    "current with latest null is refused",
			file:    ptr(`{"checked_at":"2026-09-28T09:37:00Z","installed":"0.2.0","latest":null,"outcome":"current"}`),
			wantErr: "latest",
		},
		{
			name:    "available with installed null is refused",
			file:    ptr(`{"checked_at":"2026-09-28T09:37:00Z","installed":null,"latest":"0.2.0","outcome":"available"}`),
			wantErr: "installed",
		},
		{
			name:    "missing checked_at",
			file:    ptr(`{"installed":"0.1.0","latest":"0.2.0","outcome":"available"}`),
			wantErr: "checked_at",
		},
		{
			name:    "missing outcome",
			file:    ptr(`{"checked_at":"2026-09-28T09:37:00Z","installed":"0.1.0","latest":"0.2.0"}`),
			wantErr: "outcome",
		},
		{
			name:    "missing installed key",
			file:    ptr(`{"checked_at":"2026-09-28T09:37:00Z","latest":"0.2.0","outcome":"failed"}`),
			wantErr: "installed",
		},
		{
			name:    "missing latest key",
			file:    ptr(`{"checked_at":"2026-09-28T09:37:00Z","installed":"0.1.0","outcome":"failed"}`),
			wantErr: "latest",
		},
		{
			name:    "checked_at yesterday",
			file:    ptr(`{"checked_at":"yesterday","installed":"0.1.0","latest":"0.2.0","outcome":"available"}`),
			wantErr: "checked_at",
		},
		{
			name:    "checked_at without T and zone",
			file:    ptr(`{"checked_at":"2026-09-28 09:37","installed":"0.1.0","latest":"0.2.0","outcome":"available"}`),
			wantErr: "checked_at",
		},
		{
			name:    "checked_at null",
			file:    ptr(`{"checked_at":null,"installed":"0.1.0","latest":"0.2.0","outcome":"available"}`),
			wantErr: "checked_at",
		},
		{
			name:    "outcome done lists the allowed values",
			file:    ptr(`{"checked_at":"2026-09-28T09:37:00Z","installed":"0.1.0","latest":"0.2.0","outcome":"done"}`),
			wantErr: "current, available, updated, rolled-back, failed",
		},
		{
			name:    "version with a shell command",
			file:    ptr(`{"checked_at":"2026-09-28T09:37:00Z","installed":"0.1.0; rm -rf /","latest":"0.2.0","outcome":"available"}`),
			wantErr: "installed",
		},
		{
			name:    "version of 65 characters",
			file:    ptr(`{"checked_at":"2026-09-28T09:37:00Z","installed":"0.1.0","latest":"` + strings.Repeat("1", 65) + `","outcome":"available"}`),
			wantErr: "latest",
		},
		{
			name:    "empty version",
			file:    ptr(`{"checked_at":"2026-09-28T09:37:00Z","installed":"","latest":"0.2.0","outcome":"available"}`),
			wantErr: "installed",
		},
		{
			name:    "version that is a number",
			file:    ptr(`{"checked_at":"2026-09-28T09:37:00Z","installed":1,"latest":"0.2.0","outcome":"available"}`),
			wantErr: "installed",
		},
		{
			name:    "file over 4 KiB",
			file:    ptr(strings.Repeat(" ", MaxSize+1)),
			wantErr: "larger than 4 KiB",
		},
		{
			name:    "second object after the first",
			file:    ptr(`{"checked_at":"2026-09-28T09:37:00Z","installed":"0.1.0","latest":"0.2.0","outcome":"available"}{"outcome":"current"}`),
			wantErr: "after the JSON object",
		},
		{
			name:    "text after the object",
			file:    ptr(`{"checked_at":"2026-09-28T09:37:00Z","installed":"0.1.0","latest":"0.2.0","outcome":"available"} x`),
			wantErr: "after the JSON object",
		},
		{
			name:    "malformed JSON",
			file:    ptr(`{"checked_at":"2026-09-28T09:37:00Z",`),
			wantErr: "not valid JSON",
		},
		{
			name:    "empty file",
			file:    ptr(``),
			wantErr: "not valid JSON",
		},
		{
			name:    "a list instead of an object",
			file:    ptr(`["current"]`),
			wantErr: "not valid JSON",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fsys := fstest.MapFS{}
			if tc.file != nil {
				fsys["var/lib/holzkube-manager-update/status.json"] = &fstest.MapFile{Data: []byte(*tc.file), Mode: 0o644}
			}

			got, err := Read(fsys, DefaultPath)

			switch {
			case tc.notRec:
				if !errors.Is(err, ErrNotRecorded) {
					t.Fatalf("err = %v, want ErrNotRecorded", err)
				}
				if got != (Status{}) {
					t.Fatalf("status = %+v alongside ErrNotRecorded, want the zero Status", got)
				}
			case tc.wantErr != "":
				if err == nil {
					t.Fatalf("err = nil (status %+v), want an error containing %q", describe(got), tc.wantErr)
				}
				if errors.Is(err, ErrNotRecorded) {
					t.Fatalf("err = %v is ErrNotRecorded; a present but broken file is not 'not recorded'", err)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %q, want it to contain %q", err, tc.wantErr)
				}
				if got != (Status{}) {
					t.Fatalf("status = %+v alongside an error; a refusal carries no values", describe(got))
				}
			default:
				if err != nil {
					t.Fatalf("err = %v, want success", err)
				}
				if !got.CheckedAt.Equal(tc.want.CheckedAt) || got.CheckedAt.Location() != time.UTC {
					t.Errorf("checked_at = %v, want %v in UTC", got.CheckedAt, tc.want.CheckedAt)
				}
				if got.Outcome != tc.want.Outcome {
					t.Errorf("outcome = %q, want %q", got.Outcome, tc.want.Outcome)
				}
				if !samePtr(got.Installed, tc.want.Installed) {
					t.Errorf("installed = %s, want %s", show(got.Installed), show(tc.want.Installed))
				}
				if !samePtr(got.Latest, tc.want.Latest) {
					t.Errorf("latest = %s, want %s", show(got.Latest), show(tc.want.Latest))
				}
			}
		})
	}
}

// TestReadStatusPath covers the path handling that is not about the content.
func TestReadStatusPath(t *testing.T) {
	t.Parallel()

	good := `{"checked_at":"2026-09-28T09:37:00Z","installed":"0.1.0","latest":"0.2.0","outcome":"available"}`
	fsys := fstest.MapFS{"srv/status.json": &fstest.MapFile{Data: []byte(good), Mode: 0o644}}

	if _, err := Read(fsys, "/srv/status.json"); err != nil {
		t.Errorf("an overridden absolute path: err = %v, want success", err)
	}
	if _, err := Read(fsys, "/srv/../srv/status.json"); err == nil || errors.Is(err, ErrNotRecorded) {
		t.Errorf("a path with ..: err = %v, want a refusal that is not ErrNotRecorded", err)
	}
	if _, err := Read(fsys, "/srv"); err == nil || errors.Is(err, ErrNotRecorded) {
		t.Errorf("a directory: err = %v, want a refusal that is not ErrNotRecorded", err)
	}
}

func samePtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func show(p *string) string {
	if p == nil {
		return "null"
	}
	return `"` + *p + `"`
}

func describe(s Status) string {
	return s.CheckedAt.String() + " " + show(s.Installed) + " " + show(s.Latest) + " " + s.Outcome
}
