package audit

import "testing"

// TestOutcomeCauseRedactsFreeText is the guarantee Outcome relies on. Outcome
// is exported, so the next caller may pass a real error rather than a taxonomy
// code, and a filesystem path written here is written permanently.
func TestOutcomeCauseRedactsFreeText(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{"taxonomy code", "sudo.required", "sudo.required"},
		{"status fallback", "http.428", "http.428"},
		{"dashed code", "csrf.precondition-unmet", "csrf.precondition-unmet"},
		{"filesystem path", "open /home/holz/.local/share/holzkube/x: no such file", RedactedMarker},
		{"store message", "store: revision conflict", RedactedMarker},
		{"empty", "", RedactedMarker},
		{"uppercase", "Sudo.Required", RedactedMarker},
		{"overlong but token shaped", longToken(), RedactedMarker},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := OutcomeCause(tc.in); got != tc.want {
				t.Errorf("OutcomeCause(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func longToken() string {
	b := make([]byte, maxCodeLen+1)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}
