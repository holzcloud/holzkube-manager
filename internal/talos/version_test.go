package talos_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube/internal/talos"
)

// TestCheckSupportedVersion pins the window and its edges.
//
// The bounds are read from the exported constants rather than written out as
// literals, so widening the range stays a single edit and this test cannot
// disagree with the check about what the range is. What it does assert
// literally is the shape of the answer at each edge.
func TestCheckSupportedVersion(t *testing.T) {
	t.Parallel()

	for _, row := range []struct {
		name      string
		version   string
		supported bool
	}{
		{"the oldest supported minor", talos.MinSupportedVersion + ".0", true},
		{"the newest supported minor", talos.MaxSupportedVersion + ".3", true},
		{"the pinned machinery version", "v1.13.9", true},
		{"one minor below the window", "v1.11.9", false},
		{"one minor above the window", "v1.15.0", false},
		{"a major above the window", "v2.0.0", false},
		{"a release candidate inside the window", "v1.14.0-rc.2", true},
		{"a release candidate above the window", "v1.15.0-rc.1", false},
		{"no version at all", "", false},
		{"not a version", "unknown", false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()

			err := talos.CheckSupportedVersion(row.version)
			if row.supported {
				if err != nil {
					t.Fatalf("CheckSupportedVersion(%q) = %v, want nil", row.version, err)
				}
				return
			}

			if err == nil {
				t.Fatalf("CheckSupportedVersion(%q) = nil, want a refusal", row.version)
			}
			if !errors.Is(err, talos.ErrUnsupportedVersion) {
				t.Errorf("error %v does not satisfy errors.Is(err, ErrUnsupportedVersion)", err)
			}
			// Both halves of the reason, every time: an operator holding a node
			// that will not connect has to be able to tell whether to move the
			// node or to move holzkube.
			for _, want := range []string{talos.MinSupportedVersion, talos.MaxSupportedVersion} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not name the supported range bound %q", err, want)
				}
			}
			if row.version != "" && !strings.Contains(err.Error(), row.version) {
				t.Errorf("error %q does not name the observed version %q", err, row.version)
			}
		})
	}
}
