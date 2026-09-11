package compat_test

import (
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/compat"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// TestEverySupportedTalosVersionHasAWindow is the join between the two tables
// D-25 keeps apart.
//
// They say different things and neither may be derived from the other, but
// they do have to overlap in one direction: a Talos version holzkube-manager claims to
// support and cannot make a compatibility statement about would show an
// operator "no statement is made" on a version the product advertises.
func TestEverySupportedTalosVersionHasAWindow(t *testing.T) {
	t.Parallel()

	for _, v := range []string{talos.MinSupportedVersion, talos.MaxSupportedVersion} {
		if got := compat.Check(v, "v1.34.1"); !got.Known {
			t.Errorf("Talos %s is inside the supported range but has no compatibility window: %s", v, got.Sentence)
		}
	}
}

func TestCheckClassifiesTheWindow(t *testing.T) {
	t.Parallel()

	rows := []struct {
		name      string
		talos     string
		k8s       string
		supported bool
		headroom  int
	}{
		{name: "inside with headroom", talos: "v1.13.9", k8s: "v1.32.4", supported: true, headroom: 2},
		{name: "at the top", talos: "v1.13.9", k8s: "v1.34.1", supported: true, headroom: 0},
		{name: "above the top", talos: "v1.13.9", k8s: "v1.35.0", supported: false, headroom: -1},
		{name: "below the floor", talos: "v1.13.9", k8s: "v1.31.0", supported: false, headroom: 3},
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()

			got := compat.Check(row.talos, row.k8s)
			if !got.Known {
				t.Fatalf("no verdict for %s / %s", row.talos, row.k8s)
			}
			if got.Supported != row.supported {
				t.Errorf("Supported = %v, want %v (%s)", got.Supported, row.supported, got.Sentence)
			}
			if got.HeadroomMinors != row.headroom {
				t.Errorf("HeadroomMinors = %d, want %d", got.HeadroomMinors, row.headroom)
			}
			if got.Sentence == "" {
				t.Error("the verdict carries no sentence; D-24 asks for the number and the words")
			}
		})
	}
}

// TestUnknownVersionsAreNotGuessed is the property that keeps a phase 9
// upgrade from being authorised by an extrapolation.
func TestUnknownVersionsAreNotGuessed(t *testing.T) {
	t.Parallel()

	for _, v := range []string{"v1.99.0", "not-a-version", "", "v2.0.0"} {
		got := compat.Check(v, "v1.34.1")
		if got.Known {
			t.Errorf("Check(%q) claimed a window; an uncurated version has none", v)
		}
		if !strings.Contains(got.Sentence, "no statement") && !strings.Contains(got.Sentence, "could not be read") {
			t.Errorf("Check(%q) said %q, which does not tell the operator that nothing is known", v, got.Sentence)
		}
	}
}

// TestWindowsAreWellFormed walks the whole table so that a row added with its
// bounds the wrong way round, or a default outside its own window, is a red
// test rather than a wrong number on a screen.
func TestWindowsAreWellFormed(t *testing.T) {
	t.Parallel()

	for _, m := range compat.TalosMinors() {
		v := compat.Check("v1."+itoa(m)+".0", "v1.34.0")
		if !v.Known {
			t.Fatalf("Talos minor %d is in the table but Check does not know it", m)
		}
		w := v.Window
		if w.MinMinor > w.MaxMinor {
			t.Errorf("Talos 1.%d: window %d..%d is inverted", m, w.MinMinor, w.MaxMinor)
		}
		if w.DefaultMinor < w.MinMinor || w.DefaultMinor > w.MaxMinor {
			t.Errorf("Talos 1.%d: default 1.%d is outside its own window %d..%d",
				m, w.DefaultMinor, w.MinMinor, w.MaxMinor)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
